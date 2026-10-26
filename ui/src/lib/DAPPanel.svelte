<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { backend, runtimeEvents } from './api';
  import type { DAPCapabilities, DAPScope, DAPSourceBreakpoint, DAPStackFrame, DAPThread, DAPVariable, DocumentView } from './types';

  export let visible = false;
  export let workspacePath = '';
  export let activeDocument: DocumentView | null = null;
  export let onSave: () => Promise<void>;
  export let onNavigate: (path: string, line: number) => void;

  type ScopeView = { scope: DAPScope; variables: DAPVariable[] };

  let state: 'stopped' | 'starting' | 'running' | 'paused' | 'exited' | 'error' = 'stopped';
  let message = '';
  let capabilities: DAPCapabilities = {};
  let threads: DAPThread[] = [];
  let selectedThread = 0;
  let frames: DAPStackFrame[] = [];
  let selectedFrame = 0;
  let scopeViews: ScopeView[] = [];
  let breakpointLines: Record<string, number[]> = {};
  let lineToAdd = 1;
  let expression = '';
  let evaluation = '';
  let busy = false;
  let stopEvents: Array<() => void> = [];

  $: currentBreakpoints = activeDocument ? (breakpointLines[activeDocument.path] ?? []) : [];
  $: canStart = Boolean(activeDocument?.path.toLowerCase().endsWith('.jl') && workspacePath);
  $: controlsDisabled = busy || state !== 'paused' || selectedThread < 1;

  onMount(() => {
    stopEvents.push(runtimeEvents.on('dap:event', (payload) => handleDAPEvent(payload)) ?? (() => undefined));
    stopEvents.push(runtimeEvents.on('dap:status', (payload) => handleDAPStatus(payload)) ?? (() => undefined));
  });

  onDestroy(() => stopEvents.forEach((stop) => stop()));

  async function startDebugging(): Promise<void> {
    if (!activeDocument || !canStart || busy) return;
    busy = true;
    message = '';
    state = 'starting';
    try {
      await onSave();
      if (activeDocument?.dirty) throw new Error('Save the active Julia file before starting the debugger.');
      const started = await backend.startDebugSession();
      capabilities = started.capabilities ?? {};
      const initialBreakpoints = (breakpointLines[activeDocument.path] ?? []).map((line): DAPSourceBreakpoint => ({ line }));
      await backend.launchDebuggee(activeDocument.path, workspacePath, [], true, initialBreakpoints);
      if (state === 'starting') state = 'running';
    } catch (error) {
      state = 'error';
      message = error instanceof Error ? error.message : String(error);
    } finally {
      busy = false;
    }
  }

  async function stopDebugging(): Promise<void> {
    busy = true;
    try {
      await backend.endDebugSession(false);
      state = 'stopped';
      resetInspection();
    } catch (error) {
      showError(error);
    } finally {
      busy = false;
    }
  }

  async function addBreakpoint(): Promise<void> {
    if (!activeDocument || !Number.isInteger(lineToAdd) || lineToAdd < 1) return;
    const lines = [...new Set([...(breakpointLines[activeDocument.path] ?? []), lineToAdd])].sort((a, b) => a - b);
    await replaceBreakpoints(activeDocument.path, lines);
  }

  async function removeBreakpoint(line: number): Promise<void> {
    if (!activeDocument) return;
    await replaceBreakpoints(activeDocument.path, (breakpointLines[activeDocument.path] ?? []).filter((item) => item !== line));
  }

  async function replaceBreakpoints(path: string, lines: number[]): Promise<void> {
    breakpointLines = { ...breakpointLines, [path]: lines };
    if (state === 'stopped' || state === 'exited' || state === 'error') return;
    try {
      const result = await backend.setSourceBreakpoints({ path }, lines.map((line) => ({ line })));
      const rejected = result.breakpoints.filter((breakpoint) => !breakpoint.verified);
      if (rejected.length) message = rejected.map((breakpoint) => breakpoint.message ?? `Breakpoint at line ${breakpoint.line} was rejected`).join('; ');
    } catch (error) {
      showError(error);
    }
  }

  async function perform(action: () => Promise<unknown>): Promise<void> {
    if (busy) return;
    busy = true;
    message = '';
    try {
      await action();
    } catch (error) {
      showError(error);
    } finally {
      busy = false;
    }
  }

  async function refreshInspection(): Promise<void> {
    try {
      const result = await backend.debugThreads();
      threads = result.threads ?? [];
      if (!threads.some((thread) => thread.id === selectedThread)) selectedThread = threads[0]?.id ?? 0;
      if (selectedThread > 0) {
        const stack = await backend.debugStackTrace(selectedThread);
        frames = stack.stackFrames ?? [];
        if (!frames.some((frame) => frame.id === selectedFrame)) selectedFrame = frames[0]?.id ?? 0;
        if (selectedFrame > 0) await loadScopes(selectedFrame);
      }
    } catch (error) {
      showError(error);
    }
  }

  async function selectFrame(frame: DAPStackFrame): Promise<void> {
    selectedFrame = frame.id;
    if (frame.source?.path) void Promise.resolve(onNavigate(frame.source.path, frame.line)).catch(showError);
    await loadScopes(frame.id);
  }

  async function loadScopes(frameID: number): Promise<void> {
    const result = await backend.debugScopes(frameID);
    const next: ScopeView[] = [];
    for (const scope of result.scopes ?? []) {
      const variables = scope.variablesReference > 0
        ? (await backend.debugVariables(scope.variablesReference)).variables ?? []
        : [];
      next.push({ scope, variables });
    }
    scopeViews = next;
  }

  async function evaluate(): Promise<void> {
    if (!expression.trim()) return;
    await perform(async () => {
      const result = await backend.debugEvaluate(expression, selectedFrame, 'repl');
      evaluation = result.result;
      if (result.variablesReference > 0) message = `Result can be expanded (reference ${result.variablesReference}).`;
    });
  }

  function handleDAPEvent(payload: unknown): void {
    const event = payload as { event?: string; body?: { threadId?: number; allThreadsStopped?: boolean } };
    switch (event?.event) {
      case 'initialized':
        if (state === 'starting') state = 'running';
        break;
      case 'stopped':
        state = 'paused';
        if (event.body?.threadId) selectedThread = event.body.threadId;
        void refreshInspection();
        break;
      case 'continued':
        state = 'running';
        break;
      case 'terminated':
      case 'exited':
        state = 'exited';
        resetInspection();
        break;
      case 'breakpoint':
        void refreshInspection();
        break;
    }
  }

  function handleDAPStatus(payload: unknown): void {
    const status = payload as { state?: string; message?: string };
    if (!status?.state) return;
    if (status.state === 'stopped' || status.state === 'exited') {
      state = status.state;
      resetInspection();
    } else if (status.state === 'error') {
      state = 'error';
    }
    if (status.message) message = status.message;
  }

  function resetInspection(): void {
    threads = [];
    frames = [];
    scopeViews = [];
    selectedThread = 0;
    selectedFrame = 0;
  }

  function showError(error: unknown): void {
    message = error instanceof Error ? error.message : String(error);
  }
</script>

<section class:hidden={!visible} class="debug-panel" aria-label="Julia debugger">
  <div class="debug-toolbar">
    <div class="debug-title"><span class="debug-dot" class:paused={state === 'paused'}></span>Debugger <span class="debug-state">{state}</span></div>
    <div class="debug-actions">
      {#if state === 'stopped' || state === 'exited' || state === 'error'}
        <button class="debug-button primary" on:click={startDebugging} disabled={!canStart || busy} title={!canStart ? 'Open a Julia source file in a workspace first' : 'Start DebugAdapter.jl and launch this file'}>▶ Start</button>
      {:else}
        <button class="debug-button" on:click={() => perform(() => backend.debugContinue(selectedThread))} disabled={controlsDisabled} title="Continue">▶</button>
        <button class="debug-button" on:click={() => perform(() => backend.debugNext(selectedThread))} disabled={controlsDisabled} title="Step over">↷</button>
        <button class="debug-button" on:click={() => perform(() => backend.debugStepIn(selectedThread))} disabled={controlsDisabled} title="Step into">↓</button>
        <button class="debug-button" on:click={() => perform(() => backend.debugStepOut(selectedThread))} disabled={controlsDisabled} title="Step out">↑</button>
        <button class="debug-button" on:click={() => perform(() => backend.debugPause(selectedThread))} disabled={busy || state !== 'running'} title="Pause">Ⅱ</button>
        <button class="debug-button stop" on:click={stopDebugging} disabled={busy} title="Stop debugging">■</button>
      {/if}
      {#if state === 'stopped' && !canStart}<span class="debug-hint">Open a .jl file to begin</span>{/if}
      {#if state === 'paused' && threads.length > 1}
        <select aria-label="Debug thread" bind:value={selectedThread} on:change={() => void refreshInspection()}>
          {#each threads as thread (thread.id)}<option value={thread.id}>{thread.name}</option>{/each}
        </select>
      {/if}
    </div>
    <button class="debug-close" aria-label="Hide debugger panel" on:click={() => (visible = false)}>×</button>
  </div>

  <div class="debug-content">
    <section class="debug-column breakpoints-column" aria-label="Breakpoints">
      <div class="debug-section-title">BREAKPOINTS</div>
      {#if activeDocument?.path.toLowerCase().endsWith('.jl')}
        <div class="breakpoint-add">
          <input type="number" min="1" bind:value={lineToAdd} aria-label="Breakpoint line number" />
          <button class="debug-button" on:click={addBreakpoint} disabled={busy}>Add line</button>
        </div>
        {#each currentBreakpoints as line (line)}
          <button class="breakpoint-row" on:click={() => removeBreakpoint(line)} title="Remove breakpoint"><span class="breakpoint-marker"></span><span>Line {line}</span><span class="remove-breakpoint">×</span></button>
        {:else}
          <div class="debug-empty">No breakpoints in this file.</div>
        {/each}
      {:else}
        <div class="debug-empty">Select a Julia file.</div>
      {/if}
      {#if capabilities.supportsFunctionBreakpoints}
        <div class="debug-capability">Function breakpoints supported</div>
      {/if}
    </section>

    <section class="debug-column stack-column" aria-label="Call stack">
      <div class="debug-section-title">CALL STACK</div>
      {#if frames.length}
        {#each frames as frame (frame.id)}
          <button class="stack-row" class:selected={selectedFrame === frame.id} on:click={() => void selectFrame(frame)}>
            <span>{frame.name}</span><span>{frame.source?.name ?? ''}:{frame.line}</span>
          </button>
        {/each}
      {:else}
        <div class="debug-empty">{state === 'paused' ? 'No stack frames.' : 'Pause execution to inspect the stack.'}</div>
      {/if}
    </section>

    <section class="debug-column variables-column" aria-label="Variables">
      <div class="debug-section-title">VARIABLES</div>
      {#if scopeViews.length}
        {#each scopeViews as group (group.scope.variablesReference)}
          <div class="scope-title">{group.scope.name}</div>
          {#each group.variables as variable (variable.name)}
            <div class="variable-row"><span>{variable.name}</span><span>{variable.value}</span><span>{variable.type ?? ''}</span></div>
          {:else}
            <div class="debug-empty">No variables.</div>
          {/each}
        {/each}
      {:else}
        <div class="debug-empty">{state === 'paused' ? 'Select a stack frame.' : 'Variable scopes appear when paused.'}</div>
      {/if}
    </section>

    <section class="debug-column evaluate-column" aria-label="Evaluate expression">
      <div class="debug-section-title">EVALUATE</div>
      <form class="evaluate-form" on:submit|preventDefault={evaluate}>
        <input bind:value={expression} placeholder="Julia expression" aria-label="Julia expression to evaluate" disabled={state !== 'paused'} />
        <button class="debug-button" type="submit" disabled={state !== 'paused' || busy || !expression.trim()}>↵</button>
      </form>
      {#if evaluation}<pre class="evaluation-result">{evaluation}</pre>{/if}
      {#if message}<div class="debug-message" role="status">{message}</div>{/if}
    </section>
  </div>
</section>

<style>
  .debug-panel { background: #0e151e; border-top: 1px solid #263342; color: #c7d2df; display: flex; flex: 0 0 220px; flex-direction: column; min-height: 150px; overflow: hidden; }
  .debug-panel.hidden { display: none; }
  .debug-toolbar { align-items: center; border-bottom: 1px solid #202c39; display: flex; flex: 0 0 38px; gap: 12px; padding: 0 12px; }
  .debug-title { align-items: center; color: #d7e1ed; display: flex; font-size: 10px; font-weight: 600; gap: 7px; white-space: nowrap; }
  .debug-dot { background: #637386; border-radius: 50%; height: 6px; width: 6px; }
  .debug-dot.paused { background: #e9b86a; box-shadow: 0 0 7px #e9b86a88; }
  .debug-state, .debug-hint { color: #718196; font-family: var(--mono); font-size: 9px; font-weight: 400; }
  .debug-actions { align-items: center; display: flex; flex: 1; gap: 5px; }
  .debug-actions select { background: #141f2b; border: 1px solid #293848; border-radius: 4px; color: #c7d2df; font-size: 10px; height: 24px; max-width: 180px; }
  .debug-button { background: #172330; border: 1px solid #293949; border-radius: 4px; color: #aebccc; cursor: pointer; font-size: 10px; height: 24px; min-width: 27px; padding: 0 8px; }
  .debug-button:hover:not(:disabled) { background: #203142; color: #e2ebf5; }
  .debug-button:disabled { cursor: default; opacity: .42; }
  .debug-button.primary { background: #163126; border-color: #315846; color: #9cdec0; }
  .debug-button.stop { color: #e59a91; }
  .debug-close { background: transparent; border: 0; color: #708095; cursor: pointer; font-size: 18px; height: 25px; width: 25px; }
  .debug-content { display: grid; flex: 1; grid-template-columns: minmax(130px, .8fr) minmax(170px, 1fr) minmax(190px, 1.2fr) minmax(150px, .9fr); min-height: 0; overflow: auto; }
  .debug-column { border-right: 1px solid #202b38; min-width: 0; overflow: auto; padding: 9px 10px; }
  .debug-column:last-child { border-right: 0; }
  .debug-section-title { color: #748398; font-size: 8px; font-weight: 600; letter-spacing: .11em; margin-bottom: 8px; }
  .breakpoint-add, .evaluate-form { display: flex; gap: 4px; margin-bottom: 7px; }
  .breakpoint-add input { background: #0b1119; border: 1px solid #273545; border-radius: 3px; color: #cbd6e3; font-family: var(--mono); font-size: 10px; min-width: 0; padding: 0 5px; width: 54px; }
  .evaluate-form input { background: #0b1119; border: 1px solid #273545; border-radius: 3px; color: #cbd6e3; font-family: var(--mono); font-size: 10px; min-width: 0; padding: 0 6px; width: 100%; }
  .breakpoint-row, .stack-row { align-items: center; background: transparent; border: 0; color: #aab8c8; cursor: pointer; display: flex; font-size: 9px; gap: 7px; min-height: 22px; padding: 2px 4px; text-align: left; width: 100%; }
  .breakpoint-row:hover, .stack-row:hover, .stack-row.selected { background: #172330; color: #e1e9f2; }
  .breakpoint-marker { background: #dd777c; border-radius: 50%; height: 7px; width: 7px; }
  .remove-breakpoint { color: #738195; font-size: 13px; margin-left: auto; }
  .stack-row { justify-content: space-between; }
  .stack-row span:first-child, .stack-row span:last-child { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .stack-row span:last-child { color: #748398; font-family: var(--mono); font-size: 8px; }
  .scope-title { color: #9db6c8; font-size: 9px; font-weight: 600; margin: 6px 0 3px; }
  .variable-row { display: grid; font-family: var(--mono); font-size: 8px; grid-template-columns: minmax(45px, .7fr) minmax(60px, 1.4fr) minmax(35px, .6fr); gap: 6px; line-height: 1.6; }
  .variable-row span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .variable-row span:first-child { color: #bed4e3; }
  .variable-row span:nth-child(2) { color: #d8c38d; }
  .variable-row span:last-child { color: #75869b; }
  .debug-empty, .debug-message { color: #6c7c90; font-size: 9px; line-height: 1.5; }
  .debug-capability { color: #6e9d8a; font-size: 8px; margin-top: 8px; }
  .debug-message { color: #e4b579; margin-top: 8px; overflow-wrap: anywhere; }
  .evaluation-result { color: #b9d4c0; font-family: var(--mono); font-size: 9px; margin: 8px 0 0; overflow-wrap: anywhere; white-space: pre-wrap; }
</style>
