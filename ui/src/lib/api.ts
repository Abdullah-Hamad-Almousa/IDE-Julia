import type { DAPBreakpointsResult, DAPContinueResult, DAPEvaluateResult, DAPScopesResult, DAPSource, DAPSourceBreakpoint, DAPStackTraceResult, DAPStartResult, DAPThreadsResult, DAPVariablesResult, DocumentView, LSPCompletionResult, LSPDefinitionResult, LSPDiagnostic, LSPHoverResult, LSPReferencesResult, LSPRenameResult, LSPStatusView, RunConfiguration, RunExecuteResult, RunExitedParams, RunOutputParams, SyntaxHighlightsResult, TestRunEventParams, TestRunExecuteParams, TestRunExecuteResult, TextSearchResult, WorkspaceSymbolsResult, WorkspaceView } from './types';

interface WailsApp {
  OpenWorkspaceDialog(): Promise<WorkspaceView | null>;
  OpenWorkspace(path: string): Promise<WorkspaceView>;
  GetWorkspace(): Promise<WorkspaceView | null>;
  RefreshWorkspace(): Promise<WorkspaceView>;
  SelectEnvironment(id: string): Promise<WorkspaceView>;
  OpenFile(path: string): Promise<DocumentView>;
  UpdateFile(path: string, text: string): Promise<DocumentView>;
  SaveFile(path: string, text: string): Promise<DocumentView>;
  CloseFile(path: string, discard: boolean): Promise<void>;
  GetSyntaxHighlights(uri: string, text: string): Promise<SyntaxHighlightsResult>;
  GetLSPStatusView(): Promise<LSPStatusView>;
  GetLSPDiagnostics(uri: string): Promise<LSPDiagnostic[]>;
  GetJuliaSourceURI(path: string): Promise<string>;
  GetPathFromURI(uri: string): Promise<string>;
  RetryLanguageServer(): Promise<void>;
  StartLanguageServer(): Promise<void>;
  LSPHover(uri: string, line: number, character: number): Promise<LSPHoverResult | null>;
  LSPCompletion(uri: string, line: number, character: number): Promise<LSPCompletionResult>;
  LSPReferences(uri: string, line: number, character: number, includeDeclaration: boolean): Promise<LSPReferencesResult>;
  LSPDefinition(uri: string, line: number, character: number): Promise<LSPDefinitionResult>;
  LSPRename(uri: string, line: number, character: number, newName: string): Promise<LSPRenameResult>;
  SearchText(query: string, limit: number): Promise<TextSearchResult>;
  SearchWorkspaceSymbols(query: string, limit: number): Promise<WorkspaceSymbolsResult>;
  StartRunConfiguration(configuration: RunConfiguration): Promise<RunExecuteResult>;
  StopRun(runId: string): Promise<void>;
  WriteRunInput(runId: string, input: string): Promise<void>;
  StartTestRun(params: TestRunExecuteParams): Promise<TestRunExecuteResult>;
  StopTestRun(runId: string): Promise<void>;
  StartDebugSession(): Promise<DAPStartResult>;
  LaunchDebuggee(programPath: string, workingDirectory: string, args: string[], stopOnEntry: boolean, breakpoints: DAPSourceBreakpoint[]): Promise<void>;
  SetSourceBreakpoints(source: DAPSource, breakpoints: DAPSourceBreakpoint[]): Promise<DAPBreakpointsResult>;
  DebugThreads(): Promise<DAPThreadsResult>;
  DebugStackTrace(threadId: number, startFrame: number, levels: number): Promise<DAPStackTraceResult>;
  DebugContinue(threadId: number): Promise<DAPContinueResult>;
  DebugNext(threadId: number): Promise<void>;
  DebugStepIn(threadId: number, targetId: number): Promise<void>;
  DebugStepOut(threadId: number): Promise<void>;
  DebugPause(threadId: number): Promise<void>;
  DebugScopes(frameId: number): Promise<DAPScopesResult>;
  DebugVariables(reference: number, start: number, count: number): Promise<DAPVariablesResult>;
  DebugEvaluate(expression: string, frameId: number, contextName: string): Promise<DAPEvaluateResult>;
  EndDebugSession(terminate: boolean): Promise<void>;
}

interface WailsRuntime {
  WindowMinimise(): void;
  WindowToggleMaximise(): void;
  Quit(): void;
  EventsOn(name: string, callback: (...data: unknown[]) => void): () => void;
}

declare global {
  interface Window {
    runtime?: WailsRuntime;
    go?: {
      main?: {
        App?: WailsApp;
      };
    };
  }
}

function bridge(): WailsApp {
  const app = window.go?.main?.App;
  if (!app) {
    throw new Error('The jide desktop bridge is unavailable. Start the app with Wails.');
  }
  return app;
}

export const backend = {
  openWorkspaceDialog: () => bridge().OpenWorkspaceDialog(),
  openWorkspace: (path: string) => bridge().OpenWorkspace(path),
  getWorkspace: () => bridge().GetWorkspace(),
  refreshWorkspace: () => bridge().RefreshWorkspace(),
  selectEnvironment: (id: string) => bridge().SelectEnvironment(id),
  openFile: (path: string) => bridge().OpenFile(path),
  updateFile: (path: string, text: string) => bridge().UpdateFile(path, text),
  saveFile: (path: string, text: string) => bridge().SaveFile(path, text),
  closeFile: (path: string, discard: boolean) => bridge().CloseFile(path, discard),
  syntaxHighlights: (uri: string, text: string) => bridge().GetSyntaxHighlights(uri, text),
  lspStatus: () => bridge().GetLSPStatusView(),
  lspDiagnostics: (uri: string) => bridge().GetLSPDiagnostics(uri),
  juliaSourceURI: (path: string) => bridge().GetJuliaSourceURI(path),
  pathFromURI: (uri: string) => bridge().GetPathFromURI(uri),
  retryLanguageServer: () => bridge().RetryLanguageServer(),
  startLanguageServer: () => bridge().StartLanguageServer(),
  lspHover: (uri: string, line: number, character: number) => bridge().LSPHover(uri, line, character),
  lspCompletion: (uri: string, line: number, character: number) => bridge().LSPCompletion(uri, line, character),
  lspReferences: (uri: string, line: number, character: number, includeDeclaration: boolean) => bridge().LSPReferences(uri, line, character, includeDeclaration),
  lspDefinition: (uri: string, line: number, character: number) => bridge().LSPDefinition(uri, line, character),
  lspRename: (uri: string, line: number, character: number, newName: string) => bridge().LSPRename(uri, line, character, newName),
  searchText: (query: string, limit = 100) => bridge().SearchText(query, limit),
  searchWorkspaceSymbols: (query: string, limit = 100) => bridge().SearchWorkspaceSymbols(query, limit),
  startRunConfiguration: (configuration: RunConfiguration) => bridge().StartRunConfiguration(configuration),
  stopRun: (runId: string) => bridge().StopRun(runId),
  writeRunInput: (runId: string, input: string) => bridge().WriteRunInput(runId, input),
  startTestRun: (params: TestRunExecuteParams) => bridge().StartTestRun(params),
  stopTestRun: (runId: string) => bridge().StopTestRun(runId),
  startDebugSession: () => bridge().StartDebugSession(),
  launchDebuggee: (programPath: string, workingDirectory: string, args: string[], stopOnEntry: boolean, breakpoints: DAPSourceBreakpoint[]) => bridge().LaunchDebuggee(programPath, workingDirectory, args, stopOnEntry, breakpoints),
  setSourceBreakpoints: (source: DAPSource, breakpoints: DAPSourceBreakpoint[]) => bridge().SetSourceBreakpoints(source, breakpoints),
  debugThreads: () => bridge().DebugThreads(),
  debugStackTrace: (threadId: number, startFrame = 0, levels = 20) => bridge().DebugStackTrace(threadId, startFrame, levels),
  debugContinue: (threadId: number) => bridge().DebugContinue(threadId),
  debugNext: (threadId: number) => bridge().DebugNext(threadId),
  debugStepIn: (threadId: number, targetId = 0) => bridge().DebugStepIn(threadId, targetId),
  debugStepOut: (threadId: number) => bridge().DebugStepOut(threadId),
  debugPause: (threadId: number) => bridge().DebugPause(threadId),
  debugScopes: (frameId: number) => bridge().DebugScopes(frameId),
  debugVariables: (reference: number, start = 0, count = 100) => bridge().DebugVariables(reference, start, count),
  debugEvaluate: (expression: string, frameId: number, contextName = 'repl') => bridge().DebugEvaluate(expression, frameId, contextName),
  endDebugSession: (terminate: boolean) => bridge().EndDebugSession(terminate)
};

export const windowControls = {
  minimize: () => window.runtime?.WindowMinimise(),
  toggleMaximize: () => window.runtime?.WindowToggleMaximise(),
  quit: () => window.runtime?.Quit()
};

export const runtimeEvents = {
  on: (name: string, callback: (...data: unknown[]) => void) => window.runtime?.EventsOn(name, callback)
};
