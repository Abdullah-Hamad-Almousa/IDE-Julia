export interface SyntaxPosition {
  line: number;
  column: number;
}

export interface SyntaxRange {
  startByte: number;
  endByte: number;
  start: SyntaxPosition;
  end: SyntaxPosition;
}

export interface SyntaxToken {
  kind: string;
  text: string;
  range: SyntaxRange;
}

export interface SyntaxDiagnostic {
  message: string;
  range: SyntaxRange;
}

export interface SyntaxHighlightsResult {
  uri: string;
  tokens: SyntaxToken[];
  diagnostics: SyntaxDiagnostic[];
}

export interface WorkspaceEntryView {
  path: string;
  absolutePath: string;
  isDirectory: boolean;
  symlink?: boolean;
  outsideRoot?: boolean;
  alias?: boolean;
}

export interface EnvironmentView {
  id: string;
  name: string;
  path?: string;
  details?: string;
}

export interface WorkspaceView {
  root: string;
  name: string;
  projectPath?: string;
  juliaCompat?: string;
  environmentWarning?: string;
  entries: WorkspaceEntryView[];
  environments: EnvironmentView[];
  activeEnvironment: string;
}

export interface DocumentView {
  path: string;
  name: string;
  text: string;
  version: number;
  dirty: boolean;
  highlights: SyntaxHighlightsResult;
}

export interface LSPPosition { line: number; character: number }
export interface LSPRange { start: LSPPosition; end: LSPPosition }
export interface LSPDiagnostic {
  range: LSPRange;
  severity?: number;
  source?: string;
  message: string;
}
export interface LSPStatusView { state: string; julia?: string; message?: string; restartAttempts?: number }
export interface LSPHoverResult { uri: string; hover?: { contents: { kind: string; value: string }; range?: LSPRange } }
export interface LSPCompletionItem { label: string; kind?: number; detail?: string; documentation?: unknown; insertText?: string; insertTextFormat?: number; textEdit?: { range: LSPRange; text: string }; additionalTextEdits?: Array<{ range: LSPRange; text: string }> }
export interface LSPCompletionResult { uri: string; isIncomplete?: boolean; items: LSPCompletionItem[] }
export interface LSPLocation { uri: string; range: LSPRange }
export interface LSPReferencesResult { locations: LSPLocation[] }
export interface LSPDefinitionResult { locations: LSPLocation[] }
export interface TextSearchMatch { uri: string; line: number; column: number; preview: string }
export interface TextSearchResult { matches: TextSearchMatch[]; truncated?: boolean }
export interface WorkspaceSymbol { name: string; kind: string; location: LSPLocation }
export interface WorkspaceSymbolsResult { symbols: WorkspaceSymbol[]; truncated?: boolean }
export interface LSPTextChange { range?: LSPRange; text: string }
export interface LSPDocumentChange { uri: string; version?: number; edits: LSPTextChange[] }
export interface LSPRenameResult { changes?: Record<string, LSPTextChange[]>; documentChanges?: LSPDocumentChange[] }

export interface DAPSource { name?: string; path?: string; sourceReference?: number }
export interface DAPSourceBreakpoint { line: number; column?: number; condition?: string; hitCondition?: string; logMessage?: string }
export interface DAPBreakpoint { id?: number; verified: boolean; message?: string; source?: DAPSource; line?: number; column?: number }
export interface DAPBreakpointsResult { breakpoints: DAPBreakpoint[] }
export interface DAPCapabilities { supportsFunctionBreakpoints?: boolean; supportsSetVariable?: boolean; supportsTerminateRequest?: boolean; exceptionBreakpointFilters?: Array<{ filter: string; label: string; description?: string; default?: boolean }> }
export interface DAPStartResult { capabilities: DAPCapabilities }
export interface DAPContinueResult { allThreadsContinued?: boolean }
export interface DAPThread { id: number; name: string }
export interface DAPThreadsResult { threads: DAPThread[] }
export interface DAPStackFrame { id: number; name: string; source?: DAPSource; line: number; column: number }
export interface DAPStackTraceResult { stackFrames: DAPStackFrame[]; totalFrames?: number }
export interface DAPScope { name: string; variablesReference: number; namedVariables?: number; indexedVariables?: number; expensive: boolean }
export interface DAPScopesResult { scopes: DAPScope[] }
export interface DAPVariable { name: string; value: string; type?: string; evaluateName?: string; variablesReference: number; namedVariables?: number; indexedVariables?: number }
export interface DAPVariablesResult { variables: DAPVariable[] }
export interface DAPEvaluateResult { result: string; type?: string; variablesReference: number }
export interface RunConfiguration {
  id?: string;
  name: string;
  type: string;
  projectPath?: string;
  workingDirectory?: string;
  program?: string;
  module?: string;
  arguments?: string[];
  environment?: Record<string, string>;
  properties?: Record<string, unknown>;
}
export interface RunExecuteResult { runId: string; pid: number }
export interface RunOutputParams { runId: string; stream: string; data: string }
export interface RunExitedParams { runId: string; exitCode: number; error?: string }
export interface TestRunExecuteParams { projectPath?: string; workingDirectory?: string; testFile: string; arguments?: string[] }
export interface TestRunExecuteResult { runId: string }
export interface TestRunEventParams {
  runId: string;
  kind: string;
  name?: string;
  status?: 'passed' | 'failed' | 'error' | 'skipped' | 'broken' | string;
  message?: string;
  sourcePath?: string;
  line?: number;
  passed?: number;
  failed?: number;
  errors?: number;
  skipped?: number;
  broken?: number;
  exitCode?: number;
  stream?: string;
  data?: string;
}

export interface FileNode {
  name: string;
  path: string;
  absolutePath: string;
  isDirectory: boolean;
  symlink: boolean;
  outsideRoot: boolean;
  alias: boolean;
  children: FileNode[];
}
