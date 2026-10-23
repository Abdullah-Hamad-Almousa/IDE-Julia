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
