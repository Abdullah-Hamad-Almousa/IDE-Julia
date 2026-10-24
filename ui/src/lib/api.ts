import type { DocumentView, LSPCompletionResult, LSPDefinitionResult, LSPDiagnostic, LSPHoverResult, LSPReferencesResult, LSPRenameResult, LSPStatusView, SyntaxHighlightsResult, TextSearchResult, WorkspaceSymbolsResult, WorkspaceView } from './types';

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
  searchWorkspaceSymbols: (query: string, limit = 100) => bridge().SearchWorkspaceSymbols(query, limit)
};

export const windowControls = {
  minimize: () => window.runtime?.WindowMinimise(),
  toggleMaximize: () => window.runtime?.WindowToggleMaximise(),
  quit: () => window.runtime?.Quit()
};

export const runtimeEvents = {
  on: (name: string, callback: (...data: unknown[]) => void) => window.runtime?.EventsOn(name, callback)
};
