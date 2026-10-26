<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import Editor from './lib/Editor.svelte';
  import DAPPanel from './lib/DAPPanel.svelte';
  import RunPanel from './lib/RunPanel.svelte';
  import FileTree from './lib/FileTree.svelte';
  import { backend } from './lib/api';
  import { buildFileTree } from './lib/fileTree';
  import {
    activeDocumentPathStore,
    activeDocumentStore,
    documentsStore,
    replaceDocument,
    workspaceStore
  } from './lib/stores';
  import { windowControls } from './lib/api';
  import { runtimeEvents } from './lib/api';
  import { clearLSPDiagnostics, registerJuliaEditorActions, setLSPDiagnostics } from './lib/syntaxBridge';
  import type { DocumentView, FileNode, LSPDiagnostic, LSPStatusView, WorkspaceView } from './lib/types';

  let busy = false;
  let errorMessage = '';
  let filterText = '';
  let cursorLine = 1;
  let cursorColumn = 1;
  let updateTimers = new Map<string, ReturnType<typeof setTimeout>>();
  let updateQueues = new Map<string, Promise<void>>();
  let lspStatus: LSPStatusView = { state: 'stopped' };
  let statusTimer: ReturnType<typeof setInterval> | undefined;
  let stopDiagnostics: (() => void) | undefined;
  let stopLSPStatus: (() => void) | undefined;
  let stopLSPError: (() => void) | undefined;
  let lspProviderErrorListener: ((event: Event) => void) | undefined;
  let editorActions: Array<{ dispose(): void }> = [];
  let targetPosition: { path: string; line: number; column: number } | null = null;
  let statusErrorMessage = '';
  let providerErrorMessage = '';
  let statusMessage = '';
  let statusRefreshGeneration = 0;
  let debugVisible = false;
  let runVisible = false;

  $: fileTree = buildFileTree($workspaceStore?.entries ?? []);
  $: filteredTree = filterNodes(fileTree, filterText.trim().toLowerCase());
  $: openPaths = $documentsStore.map((document) => document.path);
  $: activeEnvironment = $workspaceStore?.environments.find(
    (environment) => environment.id === $workspaceStore?.activeEnvironment
  );

  onMount(async () => {
    editorActions = registerJuliaEditorActions();
    try {
      workspaceStore.set(await backend.getWorkspace());
      stopDiagnostics = runtimeEvents.on('lsp:diagnostics', (payload) => {
        const params = payload as { uri: string; diagnostics: LSPDiagnostic[] };
        setLSPDiagnostics(params.uri, params.diagnostics ?? []);
      });
      stopLSPStatus = runtimeEvents.on('lsp:status', () => { void refreshLSPStatus(); });
      stopLSPError = runtimeEvents.on('lsp:error', (payload) => {
        statusErrorMessage = String(payload);
        void refreshLSPStatus();
      });
      lspProviderErrorListener = (event) => {
        const detail = (event as CustomEvent<{ feature: string; message: string }>).detail;
        providerErrorMessage = `Julia ${detail.feature} request failed: ${detail.message}`;
      };
      window.addEventListener('jide:lsp-provider-error', lspProviderErrorListener);
      statusTimer = setInterval(() => void refreshLSPStatus(), 2000);
      await refreshLSPStatus();
      for (const document of $documentsStore) if (document.path.toLowerCase().endsWith('.jl')) await refreshDocumentDiagnostics(document.path);
    } catch (error) {
      showError(error);
    }
  });

  onDestroy(() => {
    for (const timer of updateTimers.values()) clearTimeout(timer);
    if (statusTimer) clearInterval(statusTimer);
    stopDiagnostics?.();
    stopLSPStatus?.();
    stopLSPError?.();
    if (lspProviderErrorListener) window.removeEventListener('jide:lsp-provider-error', lspProviderErrorListener);
    for (const action of editorActions) action.dispose();
  });

  async function openWorkspace(): Promise<void> {
    if (!(await resolveOpenDocuments())) return;
    busy = true;
    errorMessage = '';
    try {
      const next = await backend.openWorkspaceDialog();
      if (next) setWorkspace(next);
    } catch (error) {
      showError(error);
    } finally {
      busy = false;
    }
  }

  async function openFile(node: FileNode): Promise<void> {
    if (node.outsideRoot || node.alias || node.isDirectory) return;
    const existing = $documentsStore.find((document) => document.path === node.absolutePath);
    if (existing) {
      activeDocumentPathStore.set(existing.path);
      return;
    }
    busy = true;
    errorMessage = '';
    try {
      const document = await backend.openFile(node.absolutePath);
      replaceDocument(document);
      activeDocumentPathStore.set(document.path);
      if (document.path.toLowerCase().endsWith('.jl')) await refreshDocumentDiagnostics(document.path);
    } catch (error) {
      showError(error);
    } finally {
      busy = false;
    }
  }

  function handleEditorChange(path: string, text: string): void {
    documentsStore.update((documents) => documents.map((document) =>
      document.path === path ? { ...document, text, dirty: true } : document
    ));
    const previousTimer = updateTimers.get(path);
    if (previousTimer) clearTimeout(previousTimer);
    updateTimers.set(path, setTimeout(() => {
      updateTimers.delete(path);
      void queueUpdate(path, text);
    }, 180));
    void refreshDocumentDiagnostics(path);
  }

  function queueUpdate(path: string, text: string): Promise<void> {
    const previous = updateQueues.get(path) ?? Promise.resolve();
    const next = previous.catch(() => undefined).then(async () => {
      const updated = await backend.updateFile(path, text);
      const current = $documentsStore.find((document) => document.path === path);
      if (current?.text === text) replaceDocument(updated);
    }).catch((error: unknown) => {
      showError(error);
    });
    updateQueues.set(path, next);
    void next.finally(() => {
      if (updateQueues.get(path) === next) updateQueues.delete(path);
    });
    return next;
  }

  async function saveDocument(document = $activeDocumentStore): Promise<void> {
    if (!document) return;
    const timer = updateTimers.get(document.path);
    if (timer) {
      clearTimeout(timer);
      updateTimers.delete(document.path);
    }
    const queued = updateQueues.get(document.path);
    if (queued) await queued;
    try {
      const saved = await backend.saveFile(document.path, document.text);
      const current = $documentsStore.find((item) => item.path === document.path);
      if (current?.text === document.text) replaceDocument(saved);
      errorMessage = '';
    } catch (error) {
      showError(error);
    }
  }

  async function closeDocument(document: DocumentView): Promise<void> {
    let discard = false;
    if (document.dirty) {
      const save = window.confirm(`Save changes to ${document.name}?\n\nChoose Cancel to discard the changes.`);
      if (save) {
        await saveDocument(document);
        const current = $documentsStore.find((item) => item.path === document.path);
        if (current?.dirty) return;
      } else {
        if (!window.confirm(`Discard all unsaved changes to ${document.name}?`)) return;
        discard = true;
      }
    }
    const timer = updateTimers.get(document.path);
    if (timer) {
      clearTimeout(timer);
      updateTimers.delete(document.path);
    }
    const queued = updateQueues.get(document.path);
    if (queued) await queued;
    try {
      await backend.closeFile(document.path, discard);
      const remaining = $documentsStore.filter((item) => item.path !== document.path);
      documentsStore.set(remaining);
      if ($activeDocumentPathStore === document.path) {
        activeDocumentPathStore.set(remaining.at(-1)?.path ?? null);
      }
    } catch (error) {
      showError(error);
    }
  }

  async function resolveOpenDocuments(): Promise<boolean> {
    for (const document of [...$documentsStore]) {
      if (!document.dirty) continue;
      const save = window.confirm(`Save changes to ${document.name} before switching workspaces?\n\nChoose Cancel to discard.`);
      if (save) {
        await saveDocument(document);
        if ($documentsStore.find((item) => item.path === document.path)?.dirty) return false;
      } else if (!window.confirm(`Discard unsaved changes to ${document.name}?`)) {
        return false;
      } else {
        const timer = updateTimers.get(document.path);
        if (timer) clearTimeout(timer);
        updateTimers.delete(document.path);
        const queued = updateQueues.get(document.path);
        if (queued) await queued;
      }
    }
    return true;
  }

  function setWorkspace(next: WorkspaceView): void {
    clearLSPDiagnostics();
    workspaceStore.set(next);
    documentsStore.set([]);
    activeDocumentPathStore.set(null);
    filterText = '';
    errorMessage = '';
    statusErrorMessage = '';
    statusMessage = '';
    void refreshLSPStatus();
  }

  async function openNavigationTarget(path: string): Promise<DocumentView> {
    const normalizedPath = path.replace(/\\/g, '/').replace(/^([A-Z]):/, (_, drive: string) => `${drive.toLowerCase()}:`);
    const existing = $documentsStore.find((document) => document.path.replace(/\\/g, '/').replace(/^([A-Z]):/, (_, drive: string) => `${drive.toLowerCase()}:`) === normalizedPath);
    if (existing) {
      activeDocumentPathStore.set(existing.path);
      return existing;
    }
    const document = await backend.openFile(path);
    replaceDocument(document);
    activeDocumentPathStore.set(document.path);
    if (document.path.toLowerCase().endsWith('.jl')) await refreshDocumentDiagnostics(document.path);
    return document;
  }

  async function navigateToDebugFrame(path: string, line: number): Promise<void> {
    const document = await openNavigationTarget(path);
    targetPosition = { path: document.path, line: Math.max(1, line), column: 1 };
  }

  async function refreshDocumentDiagnostics(path: string): Promise<void> {
    try {
      const uri = await backend.juliaSourceURI(path);
      let diagnostics = await backend.lspDiagnostics(uri);
      setLSPDiagnostics(uri, diagnostics);
    } catch (error) {
      console.error('Could not refresh Julia diagnostics:', error);
    }
  }


  async function selectEnvironment(event: Event): Promise<void> {
    const select = event.currentTarget as HTMLSelectElement;
    try {
      workspaceStore.set(await backend.selectEnvironment(select.value));
    } catch (error) {
      showError(error);
    }
  }

  async function refreshLSPStatus(): Promise<void> {
    const generation = ++statusRefreshGeneration;
    try {
      const status = await backend.lspStatus();
      if (generation !== statusRefreshGeneration) return;
      lspStatus = status;
      if (!providerErrorMessage) {
        statusErrorMessage = status.message && status.state === 'stopped' ? status.message : '';
        statusMessage = status.message && status.state !== 'stopped' ? status.message : '';
      }
      if (status.state === 'restarting') {
        statusMessage = `Julia language server restarting (attempt ${status.restartAttempts ?? 1})…`;
      }
      if (status.state === 'running') {
        for (const document of $documentsStore) {
          if (document.path.toLowerCase().endsWith('.jl')) void refreshDocumentDiagnostics(document.path);
        }
      }
    } catch (error) {
      if (generation !== statusRefreshGeneration) return;
      lspStatus = { state: 'unavailable' };
      statusErrorMessage = error instanceof Error ? error.message : String(error);
    }
  }

  async function retryLanguageServer(): Promise<void> {
    lspStatus = { ...lspStatus, state: 'starting', message: undefined };
    statusErrorMessage = '';
    statusMessage = 'Starting Julia language server…';
    try {
      await backend.retryLanguageServer();
    } catch (error) {
      statusErrorMessage = error instanceof Error ? error.message : String(error);
    }
    await refreshLSPStatus();
    if (statusTimer) clearInterval(statusTimer);
    statusTimer = setInterval(() => void refreshLSPStatus(), 2000);
  }

  async function startLanguageServer(): Promise<void> {
    lspStatus = { ...lspStatus, state: 'starting', message: undefined };
    statusErrorMessage = '';
    statusMessage = 'Starting Julia language server…';
    try {
      await backend.startLanguageServer();
    } catch (error) {
      statusErrorMessage = error instanceof Error ? error.message : String(error);
    }
    await refreshLSPStatus();
    if (statusTimer) clearInterval(statusTimer);
    statusTimer = setInterval(() => void refreshLSPStatus(), 2000);
  }

  function handleGlobalKeydown(event: KeyboardEvent): void {
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'o') {
      event.preventDefault();
      void openWorkspace();
    }
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
      event.preventDefault();
      void saveDocument();
    }
  }

  function closeApplication(): void {
    const dirtyCount = $documentsStore.filter((document) => document.dirty).length;
    if (dirtyCount > 0 && !window.confirm(
      `${dirtyCount} file${dirtyCount === 1 ? ' has' : 's have'} unsaved changes. Close jide and discard them?`
    )) return;
    windowControls.quit();
  }

  function filterNodes(nodes: FileNode[], query: string): FileNode[] {
    if (!query) return nodes;
    return nodes.flatMap((node) => {
      const children = filterNodes(node.children, query);
      if (node.name.toLowerCase().includes(query) || children.length > 0) {
        return [{ ...node, children }];
      }
      return [];
    });
  }

  function showError(error: unknown): void {
    errorMessage = error instanceof Error ? error.message : String(error);
  }

</script>

<svelte:window on:keydown={handleGlobalKeydown} />

<svelte:head>
  <title>{$workspaceStore ? `${$workspaceStore.name} — jide` : 'jide — Julia IDE'}</title>
</svelte:head>

<div class="app-frame">
  <header class="titlebar">
    <div class="brand-lockup">
      <div class="brand-mark" aria-hidden="true"><span></span><span></span><span></span></div>
      <span class="brand-name">jide</span>
      <span class="brand-divider"></span>
      <span class="brand-context">Julia IDE</span>
    </div>
    <div class="titlebar-center">
      {#if $workspaceStore}
        <span class="project-dot"></span>
        <span class="project-title">{$workspaceStore.name}</span>
        <span class="project-path">{$workspaceStore.root}</span>
      {:else}
        <span class="project-title muted-title">No workspace open</span>
      {/if}
    </div>
    <div class="titlebar-actions">
      <button class="quiet-button" on:click={openWorkspace} disabled={busy} title="Open workspace (Ctrl+O)">
        <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M2 4.5h4l1.4 1.6H14v6.4H2z"/><path d="M2 6.1h12"/></svg>
        <span>Open folder</span>
      </button>
      <div class="window-controls" role="group" aria-label="Window controls">
        <button class="window-control" on:click={() => windowControls.minimize()} aria-label="Minimize" title="Minimize">
          <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M4 8h8"/></svg>
        </button>
        <button class="window-control" on:click={() => windowControls.toggleMaximize()} aria-label="Maximize or restore" title="Maximize or restore">
          <svg viewBox="0 0 16 16" aria-hidden="true"><rect x="4" y="4" width="8" height="8" rx=".5"/></svg>
        </button>
        <button class="window-control window-close" on:click={closeApplication} aria-label="Close jide" title="Close">
          <svg viewBox="0 0 16 16" aria-hidden="true"><path d="m4 4 8 8M12 4l-8 8"/></svg>
        </button>
      </div>
    </div>
  </header>

  <div class="workbench">
    <aside class="activity-rail" aria-label="Activity bar">
      <button class="rail-button selected" aria-label="Explorer" title="Explorer">
        <svg viewBox="0 0 20 20" aria-hidden="true"><rect x="3" y="3" width="14" height="14" rx="2"/><path d="M8 3v14M11 7h3M11 10h3M11 13h3"/></svg>
      </button>
      <div class="rail-spacer"></div>
      <button class="rail-button rail-settings" aria-label="Settings (coming later)" title="Settings arrive in WP10" disabled>
        <svg viewBox="0 0 20 20" aria-hidden="true"><circle cx="10" cy="10" r="3"/><path d="m16.2 11.8 1.1.8-1.5 2.6-1.3-.5a7 7 0 0 1-1.5.9l-.2 1.4h-3l-.2-1.4a7 7 0 0 1-1.5-.9l-1.3.5-1.5-2.6 1.1-.8a6 6 0 0 1 0-1.7l-1.1-.8 1.5-2.6 1.3.5a7 7 0 0 1 1.5-.9l.2-1.4h3l.2 1.4a7 7 0 0 1 1.5.9l1.3-.5 1.5 2.6-1.1.8a6 6 0 0 1 0 1.7Z"/></svg>
      </button>
    </aside>

    <aside class="explorer-panel">
      <div class="panel-heading">
        <div>
          <div class="eyebrow">Workspace</div>
          <div class="panel-title">Explorer</div>
        </div>
        <button class="icon-button" aria-label="Refresh files" title="Refresh files" on:click={async () => {
          try { if ($workspaceStore) workspaceStore.set(await backend.refreshWorkspace()); }
          catch (error) { showError(error); }
        }} disabled={!$workspaceStore || busy}>
          <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M13 5V2l-1.5 1.5A5 5 0 1 0 14 8"/><path d="M13 2v3H10"/></svg>
        </button>
      </div>

      {#if $workspaceStore}
        <label class="filter-box">
          <svg viewBox="0 0 16 16" aria-hidden="true"><circle cx="7" cy="7" r="4.2"/><path d="m10.2 10.2 3 3"/></svg>
          <input bind:value={filterText} placeholder="Filter files" aria-label="Filter workspace files" />
          {#if filterText}<button class="clear-filter" aria-label="Clear filter" on:click={() => (filterText = '')}>×</button>{/if}
        </label>
        <div class="tree-caption">
          <span>{$workspaceStore.name}</span>
          <span class="entry-count">{$workspaceStore.entries.length}</span>
        </div>
        <div class="file-tree-scroll">
          {#if filteredTree.length}
            <FileTree entries={filteredTree} selectedPath={$activeDocumentPathStore ?? ''} on:open={(event) => openFile(event.detail)} />
          {:else}
            <div class="empty-tree">{filterText ? 'No matching files' : 'This folder is empty'}</div>
          {/if}
        </div>
        {#if $workspaceStore.environmentWarning}
          <div class="workspace-warning" title={$workspaceStore.environmentWarning}>Project.toml could not be parsed</div>
        {/if}
      {:else}
        <div class="explorer-empty">
          <div class="empty-folder-icon"><svg viewBox="0 0 34 30" aria-hidden="true"><path d="M3 6h10l3 4h15v15H3z"/><path d="M3 10h28"/></svg></div>
          <p>Open a folder to explore your Julia project.</p>
          <button class="text-button" on:click={openWorkspace}>Choose workspace <span>↗</span></button>
        </div>
      {/if}

      <div class="panel-footer">
        <span class="footer-label">JULIA PROJECT</span>
        {#if $workspaceStore?.projectPath}
          <span class="project-file" title={$workspaceStore.projectPath}>Project.toml detected</span>
        {:else}
          <span class="project-file subdued">No Project.toml</span>
        {/if}
      </div>
    </aside>

    <main class="main-area">
      {#if $documentsStore.length > 0}
        <div class="tab-strip" role="tablist" aria-label="Open files">
          {#each $documentsStore as document (document.path)}
            <button
              class:tab-active={$activeDocumentPathStore === document.path}
              class="editor-tab"
              role="tab"
              aria-selected={$activeDocumentPathStore === document.path}
              title={document.path}
              on:click={() => activeDocumentPathStore.set(document.path)}
            >
              <span class="tab-language">{document.name.endsWith('.jl') ? 'λ' : '·'}</span>
              <span class="tab-name">{document.name}</span>
              {#if document.dirty}<span class="dirty-dot" title="Unsaved changes"></span>{/if}
              <span
                role="button"
                tabindex="0"
                class="tab-close"
                aria-label={`Close ${document.name}`}
                on:click|stopPropagation={() => closeDocument(document)}
                on:keydown|stopPropagation={(event) => event.key === 'Enter' && closeDocument(document)}
              >×</span>
            </button>
          {/each}
          <div class="tab-flex"></div>
          <button class="save-button" disabled={!$activeDocumentStore || !$activeDocumentStore.dirty || busy} on:click={() => saveDocument()} title="Save (Ctrl+S)">
            <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M3 2.5h8.5L14 5v8.5H2V2.5z"/><path d="M5 2.5v4h6v-4M5 13.5V9h6v4.5"/></svg>
            Save
          </button>
        </div>
        <div class="editor-heading">
          <div class="breadcrumbs">
            <span class="breadcrumb-root">{$workspaceStore?.name ?? 'Workspace'}</span>
            {#if $activeDocumentStore}
              <span class="breadcrumb-separator">/</span>
              <span class="breadcrumb-file">{$activeDocumentStore.path.replace($workspaceStore?.root + '\\', '').replace($workspaceStore?.root + '/', '')}</span>
            {/if}
          </div>
          <div class="editor-context">
            <span class="language-chip">{$activeDocumentStore?.path.endsWith('.jl') ? 'Julia' : 'Text'}</span>
            {#if $activeDocumentStore?.dirty}<span class="unsaved-label">Unsaved</span>{/if}
            {#if $activeDocumentStore?.path.toLowerCase().endsWith('.jl')}
              <button class="debug-toggle" class:active={runVisible} on:click={() => { runVisible = !runVisible; if (runVisible) debugVisible = false; }} aria-expanded={runVisible}>Run / Test</button>
              <button class="debug-toggle" class:active={debugVisible} on:click={() => { debugVisible = !debugVisible; if (debugVisible) runVisible = false; }} aria-expanded={debugVisible}>Debug</button>
            {/if}
          </div>
        </div>
        <section class="editor-surface" aria-label="Editor">
          <div class="editor-region">
            <Editor
              document={$activeDocumentStore}
              {openPaths}
              onOpen={openNavigationTarget}
              onActivate={(path) => activeDocumentPathStore.set(path)}
              {targetPosition}
              onChange={handleEditorChange}
              onSave={(path, text) => {
                const current = $documentsStore.find((item) => item.path === path);
                if (current) void saveDocument({ ...current, text });
              }}
              onCursor={(line, column) => { cursorLine = line; cursorColumn = column; }}
            />
          </div>
          <DAPPanel
            bind:visible={debugVisible}
            workspacePath={$workspaceStore?.root ?? ''}
            activeDocument={$activeDocumentStore}
            onSave={() => saveDocument()}
            onNavigate={navigateToDebugFrame}
          />
          <RunPanel
            bind:visible={runVisible}
            workspacePath={$workspaceStore?.root ?? ''}
            projectPath={$workspaceStore?.projectPath ?? ''}
            activeDocument={$activeDocumentStore}
            onSave={() => saveDocument()}
            onNavigate={navigateToDebugFrame}
          />
        </section>
      {:else}
        <section class="welcome-view">
          <div class="welcome-glow"></div>
          <div class="welcome-content">
            <div class="welcome-mark"><span></span><span></span><span></span></div>
            <div class="welcome-kicker">{$workspaceStore ? 'Workspace ready' : 'A workspace for Julia'}</div>
            {#if $workspaceStore}
              <h1>Open a file to<br /><span>start exploring.</span></h1>
              <p class="welcome-copy">Choose a Julia source file from Explorer to open it in the editor.</p>
              <button class="quiet-button welcome-secondary" on:click={openWorkspace} disabled={busy}>
                <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M2 4.5h4l1.4 1.6H14v6.4H2z"/><path d="M2 6.1h12"/></svg>
                Open another workspace
              </button>
            {:else}
              <h1>Make room for<br /><span>good ideas.</span></h1>
              <p class="welcome-copy">A focused place to read, shape, and save your Julia code.</p>
              <button class="primary-button" on:click={openWorkspace} disabled={busy}>
                <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M2 4.5h4l1.4 1.6H14v6.4H2z"/><path d="M2 6.1h12"/></svg>
                Open a Julia workspace
                <span class="button-shortcut">Ctrl O</span>
              </button>
            {/if}
            <div class="welcome-meta"><span class="meta-dot"></span> Tree-sitter Julia highlighting <span class="meta-separator">·</span> Local files stay local</div>
          </div>
          <div class="welcome-side-note"><span>01</span><span class="note-line"></span><span>EDIT</span></div>
        </section>
      {/if}
    </main>
  </div>

  <footer class="statusbar">
    <div class="status-left">
      <span class="status-indicator" title={lspStatus.message ?? (lspStatus.julia ? `Julia ${lspStatus.julia}` : 'Julia language server')}><span></span> {lspStatus.state === 'running' ? 'Julia LS' : lspStatus.state}</span>
      {#if $workspaceStore && (lspStatus.state === 'stopped' || lspStatus.state === 'unavailable')}
        <button class="quiet-button" title={statusErrorMessage || 'Start Julia language server'} on:click={lspStatus.state === 'stopped' && lspStatus.message ? retryLanguageServer : startLanguageServer}>{lspStatus.state === 'stopped' && lspStatus.message ? 'Retry Julia server' : 'Start Julia server'}</button>
      {/if}
      {#if $workspaceStore}
        <span class="status-path" title={$workspaceStore.root}>{$workspaceStore.root}</span>
      {/if}
    </div>
    <div class="status-right">
      {#if $activeDocumentStore}
        <span>Ln {cursorLine}, Col {cursorColumn}</span>
        <span class="status-divider"></span>
        <span>UTF-8</span>
        <span class="status-divider"></span>
        <span>Spaces: 4</span>
      {/if}
      <label class="environment-picker" title={activeEnvironment?.details ?? 'Select Julia environment'}>
        <svg viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="5.7"/><path d="M5.1 8h5.8M8 2.3c1.4 1.5 2.1 3.4 2.1 5.7S9.4 12.2 8 13.7C6.6 12.2 5.9 10.3 5.9 8S6.6 3.8 8 2.3Z"/></svg>
        <select
          aria-label="Julia environment"
          disabled={!$workspaceStore}
          value={$workspaceStore?.activeEnvironment ?? 'default'}
          on:change={selectEnvironment}
        >
          {#each $workspaceStore?.environments ?? [] as environment (environment.id)}
            <option value={environment.id}>{environment.name}</option>
          {/each}
          {#if !$workspaceStore}<option value="default">Julia environment</option>{/if}
        </select>
        <svg class="select-chevron" viewBox="0 0 12 12" aria-hidden="true"><path d="m3 4.5 3 3 3-3"/></svg>
      </label>
    </div>
  </footer>


  {#if providerErrorMessage}
    <section class="lsp-status-panel lsp-status-error" role="alert" aria-live="assertive">
      <strong>Julia editor request failed</strong>
      <p>{providerErrorMessage}</p>
      <button class="quiet-button" on:click={() => (providerErrorMessage = '')}>Dismiss</button>
    </section>
  {:else if statusErrorMessage && (lspStatus.state === 'stopped' || lspStatus.state === 'unavailable' || lspStatus.state === 'restarting')}
    <section class="lsp-status-panel lsp-status-error" role="alert" aria-live="assertive">
      <strong>Julia language server stopped</strong>
      <p>{statusErrorMessage}</p>
      {#if $workspaceStore}
        <button class="quiet-button" on:click={retryLanguageServer}>Retry Julia server</button>
      {/if}
    </section>
  {:else if statusMessage}
    <section class="lsp-status-panel" role="status" aria-live="polite">{statusMessage}</section>
  {/if}


  {#if errorMessage}
    <div class="error-toast" role="alert">
      <span class="error-icon">!</span><span>{errorMessage}</span>
      <button aria-label="Dismiss error" on:click={() => (errorMessage = '')}>×</button>
    </div>
  {/if}
</div>
