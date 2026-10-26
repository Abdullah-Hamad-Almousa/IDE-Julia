<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { backend, runtimeEvents } from './api';
  import type { DocumentView, RunExitedParams, RunOutputParams, TestRunEventParams } from './types';

  export let visible = false;
  export let workspacePath = '';
  export let projectPath = '';
  export let activeDocument: DocumentView | null = null;
  export let onSave: () => Promise<void>;
  export let onNavigate: (path: string, line: number) => void;

  let configurationType = 'julia-script';
  let moduleName = '';
  let testFile = 'test/runtests.jl';
  let runID = '';
  let testRunID = '';
  let runState = 'idle';
  let testState = 'idle';
  let output = '';
  let testOutput = '';
  let testEvents: TestRunEventParams[] = [];
  let errorMessage = '';
  let busy = false;
  let replInput = '';
  let stopEventHandlers: Array<() => void> = [];

  $: canRun = Boolean(workspacePath && (
    configurationType === 'julia-repl' ||
    (configurationType === 'julia-script' && activeDocument?.path.toLowerCase().endsWith('.jl')) ||
    (configurationType === 'julia-module' && moduleName.trim())
  ));
  $: canTest = Boolean(workspacePath && testFile.trim().toLowerCase().endsWith('.jl'));

  onMount(() => {
    stopEventHandlers.push(runtimeEvents.on('run:output', (payload) => {
      const event = payload as RunOutputParams;
      if (!event?.runId) return;
      runID = event.runId;
      runState = 'running';
      output = appendOutput(output, `[${event.stream}] ${event.data}`);
    }) ?? (() => undefined));
    stopEventHandlers.push(runtimeEvents.on('run:exited', (payload) => {
      const event = payload as RunExitedParams;
      if (!event?.runId) return;
      runID = event.runId;
      runState = event.exitCode === 0 ? 'passed' : 'failed';
      if (event.error) errorMessage = event.error;
    }) ?? (() => undefined));
    stopEventHandlers.push(runtimeEvents.on('testrun:event', (payload) => handleTestEvent(payload)) ?? (() => undefined));
  });

  onDestroy(() => stopEventHandlers.forEach((stop) => stop()));

  async function startRun(): Promise<void> {
    if (!canRun || busy) return;
    busy = true;
    errorMessage = '';
    output = '';
    runState = 'starting';
    try {
      if (configurationType === 'julia-script' && activeDocument?.dirty) {
        await onSave();
        if (activeDocument?.dirty) throw new Error('Save the active Julia file before running it.');
      }
      const result = await backend.startRunConfiguration({
        name: configurationType === 'julia-script' ? 'Active Julia file' : configurationType === 'julia-module' ? moduleName : 'Julia REPL',
        type: configurationType,
        projectPath: projectPath || undefined,
        workingDirectory: workspacePath,
        program: configurationType === 'julia-script' ? activeDocument?.path : undefined,
        module: configurationType === 'julia-module' ? moduleName.trim() : undefined
      });
      runID = result.runId;
      runState = 'running';
    } catch (error) {
      runState = 'failed';
      errorMessage = error instanceof Error ? error.message : String(error);
    } finally {
      busy = false;
    }
  }

  async function stopRun(): Promise<void> {
    if (!runID || busy) return;
    busy = true;
    try {
      await backend.stopRun(runID);
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : String(error);
    } finally {
      busy = false;
    }
  }

  async function sendREPLInput(): Promise<void> {
    if (!runID || !replInput) return;
    const input = `${replInput}\n`;
    replInput = '';
    try {
      await backend.writeRunInput(runID, input);
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : String(error);
    }
  }

  async function startTests(): Promise<void> {
    if (!canTest || busy) return;
    busy = true;
    errorMessage = '';
    testOutput = '';
    testEvents = [];
    testState = 'starting';
    try {
      const result = await backend.startTestRun({
        projectPath: projectPath || undefined,
        workingDirectory: workspacePath,
        testFile
      });
      testRunID = result.runId;
      testState = 'running';
    } catch (error) {
      testState = 'failed';
      errorMessage = error instanceof Error ? error.message : String(error);
    } finally {
      busy = false;
    }
  }

  async function stopTests(): Promise<void> {
    if (!testRunID || busy) return;
    busy = true;
    try {
      await backend.stopTestRun(testRunID);
      testState = 'stopping';
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : String(error);
    } finally {
      busy = false;
    }
  }

  function handleTestEvent(payload: unknown): void {
    const event = payload as TestRunEventParams;
    if (!event?.runId) return;
    testRunID = event.runId;
    if (event.kind === 'output') {
      testOutput = appendOutput(testOutput, `[${event.stream ?? 'stdout'}] ${event.data ?? ''}`);
      return;
    }
    testEvents = [...testEvents, event].slice(-500);
    if (event.kind === 'started' || event.kind === 'testset-start') testState = 'running';
    if (event.kind === 'finished') {
      testState = event.exitCode === 0 ? 'passed' : 'failed';
      if (event.message) errorMessage = event.message;
    }
  }

  function navigateFailure(event: TestRunEventParams): void {
    if (event.sourcePath && event.line) void Promise.resolve(onNavigate(event.sourcePath, event.line)).catch((error) => {
      errorMessage = error instanceof Error ? error.message : String(error);
    });
  }

  function appendOutput(current: string, next: string): string {
    const combined = current + next;
    return combined.length > 120_000 ? combined.slice(-120_000) : combined;
  }
</script>

<section class:hidden={!visible} class="run-panel" aria-label="Run configurations and Test Explorer">
  <div class="run-toolbar">
    <div class="run-title"><span class="run-indicator" class:active={runState === 'running' || testState === 'running'}></span>Run & Test <span class="run-status">{runState !== 'idle' ? `run ${runState}` : `tests ${testState}`}</span></div>
    <div class="run-actions">
      <select bind:value={configurationType} aria-label="Run configuration type">
        <option value="julia-script">Julia script</option>
        <option value="julia-module">Julia module</option>
        <option value="julia-repl">Julia REPL</option>
      </select>
      {#if configurationType === 'julia-module'}
        <input class="module-name" bind:value={moduleName} placeholder="Module.Name" aria-label="Julia module name" />
      {/if}
      <button class="run-button primary" on:click={startRun} disabled={!canRun || busy || runState === 'running'}>▶ Run</button>
      <button class="run-button" on:click={stopRun} disabled={!runID || busy || runState !== 'running'}>■ Stop</button>
      <button class="run-button test-button" on:click={startTests} disabled={!canTest || busy || testState === 'running'}>✓ Run tests</button>
      <button class="run-button" on:click={stopTests} disabled={!testRunID || busy || testState !== 'running'}>Stop tests</button>
    </div>
    <button class="run-close" aria-label="Hide run panel" on:click={() => (visible = false)}>×</button>
  </div>
  <div class="run-content">
    <section class="run-column run-config-column" aria-label="Run configuration">
      <div class="run-section-title">CONFIGURATION</div>
      {#if configurationType === 'julia-script'}
        <div class="run-detail">{activeDocument?.path ?? 'Open a Julia source file.'}</div>
      {:else if configurationType === 'julia-module'}
        <div class="run-detail">Loads the selected module in the workspace project.</div>
      {:else}
        <div class="run-detail">Interactive stdin/stdout process in the workspace project.</div>
      {/if}
      <label class="test-file-label" for="test-file">Test file</label>
      <input id="test-file" class="test-file-input" bind:value={testFile} placeholder="test/runtests.jl" />
      {#if configurationType === 'julia-repl' && runID && runState === 'running'}
        <form class="repl-input" on:submit|preventDefault={sendREPLInput}>
          <input bind:value={replInput} placeholder="Julia input" aria-label="Julia REPL input" />
          <button class="run-button" type="submit" disabled={!replInput}>Send</button>
        </form>
      {/if}
      {#if errorMessage}<div class="run-error" role="alert">{errorMessage}</div>{/if}
    </section>
    <section class="run-column test-results-column" aria-label="Test Explorer results">
      <div class="run-section-title">TEST EXPLORER</div>
      <div class="test-result-list">
        {#each testEvents as event, index (`${event.kind}-${index}`)}
          {#if event.kind === 'testset-start'}
            <div class="testset-row"><span class="testset-chevron">⌄</span>{event.name}</div>
          {:else if event.kind === 'testset-finish'}
            <div class="testset-summary">{event.name}: {event.passed ?? 0} passed · {event.failed ?? 0} failed · {event.skipped ?? 0} skipped</div>
          {:else if event.kind === 'test'}
            <button class="test-result-row" class:failure={event.status === 'failed' || event.status === 'error'} class:skipped={event.status === 'skipped' || event.status === 'broken'} on:click={() => navigateFailure(event)} disabled={!event.sourcePath || !event.line} title={event.sourcePath ? `${event.sourcePath}:${event.line}` : event.message}>
              <span class="result-status">{event.status === 'passed' ? '✓' : event.status === 'skipped' || event.status === 'broken' ? '◌' : '×'}</span>
              <span class="result-expression">{event.name}</span>
              {#if event.sourcePath && event.line}<span class="result-location">{event.line}</span>{/if}
            </button>
            {#if event.status === 'failed' || event.status === 'error'}<div class="failure-message">{event.message}</div>{/if}
          {/if}
        {:else}
          <div class="run-empty">Run tests to populate results. Failed assertions are clickable and navigate to their source line.</div>
        {/each}
      </div>
    </section>
    <section class="run-column output-column" aria-label="Process output">
      <div class="run-section-title">OUTPUT</div>
      <pre>{output}{testOutput}</pre>
    </section>
  </div>
</section>

<style>
  .run-panel { background: #0e151e; border-top: 1px solid #263342; color: #c7d2df; display: flex; flex: 0 0 250px; flex-direction: column; min-height: 170px; overflow: hidden; }
  .run-panel.hidden { display: none; }
  .run-toolbar { align-items: center; border-bottom: 1px solid #202c39; display: flex; flex: 0 0 38px; gap: 10px; padding: 0 12px; }
  .run-title { align-items: center; color: #d7e1ed; display: flex; font-size: 10px; font-weight: 600; gap: 7px; white-space: nowrap; }
  .run-indicator { background: #647386; border-radius: 50%; height: 6px; width: 6px; }
  .run-indicator.active { background: #72e0c2; box-shadow: 0 0 7px #72e0c288; }
  .run-status { color: #7f8da0; font-family: var(--mono); font-size: 9px; font-weight: 400; }
  .run-actions { align-items: center; display: flex; flex: 1; gap: 5px; min-width: 0; }
  .run-actions select, .module-name, .test-file-input, .repl-input input { background: #111b26; border: 1px solid #293848; border-radius: 4px; color: #c7d2df; font-size: 9px; height: 24px; padding: 0 6px; }
  .module-name { max-width: 150px; }
  .run-button { background: #172330; border: 1px solid #293949; border-radius: 4px; color: #aebccc; cursor: pointer; font-size: 9px; height: 24px; padding: 0 8px; white-space: nowrap; }
  .run-button:hover:not(:disabled) { background: #203142; color: #e2ebf5; }
  .run-button:disabled { cursor: default; opacity: .42; }
  .run-button.primary { background: #163126; border-color: #315846; color: #9cdec0; }
  .run-button.test-button { color: #a8c9e0; }
  .run-close { background: transparent; border: 0; color: #708095; cursor: pointer; font-size: 18px; height: 25px; width: 25px; }
  .run-content { display: grid; flex: 1; grid-template-columns: minmax(160px, .8fr) minmax(300px, 1.3fr) minmax(180px, .9fr); min-height: 0; overflow: hidden; }
  .run-column { border-right: 1px solid #202b38; min-width: 0; overflow: auto; padding: 9px 10px; }
  .run-column:last-child { border-right: 0; }
  .run-section-title { color: #748398; font-size: 8px; font-weight: 600; letter-spacing: .11em; margin-bottom: 8px; }
  .run-detail, .run-empty { color: #8190a3; font-size: 9px; line-height: 1.5; overflow-wrap: anywhere; }
  .test-file-label { color: #748398; display: block; font-size: 8px; margin: 10px 0 4px; }
  .test-file-input { width: 100%; }
  .repl-input { display: flex; gap: 4px; margin-top: 9px; }
  .repl-input input { min-width: 0; width: 100%; }
  .run-error { color: #e59a91; font-size: 9px; margin-top: 8px; overflow-wrap: anywhere; }
  .test-result-list { display: flex; flex-direction: column; gap: 2px; }
  .testset-row { color: #a9c4d4; font-size: 9px; font-weight: 600; margin-top: 4px; }
  .testset-chevron { color: #6e8193; display: inline-block; margin-right: 5px; }
  .testset-summary { color: #718297; font-size: 8px; margin: 0 0 4px 13px; }
  .test-result-row { align-items: center; background: transparent; border: 0; color: #aebccc; cursor: pointer; display: flex; font-family: var(--mono); font-size: 8px; gap: 6px; min-height: 18px; padding: 2px 3px 2px 15px; text-align: left; width: 100%; }
  .test-result-row:hover:not(:disabled) { background: #172330; }
  .test-result-row:disabled { cursor: default; }
  .result-status { color: #70c29d; width: 9px; }
  .test-result-row.failure .result-status { color: #e37d82; }
  .test-result-row.skipped .result-status { color: #d6b778; }
  .result-expression { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .result-location { color: #78889d; margin-left: auto; }
  .failure-message { color: #d7948b; font-family: var(--mono); font-size: 8px; line-height: 1.4; margin: 2px 4px 6px 30px; max-height: 3.5em; overflow: hidden; }
  .output-column pre { color: #a8b8c9; font-family: var(--mono); font-size: 8px; line-height: 1.5; margin: 0; overflow-wrap: anywhere; white-space: pre-wrap; }
</style>
