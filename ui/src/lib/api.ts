import type { DocumentView, SyntaxHighlightsResult, WorkspaceView } from './types';

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
}

interface WailsRuntime {
  WindowMinimise(): void;
  WindowToggleMaximise(): void;
  Quit(): void;
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
  syntaxHighlights: (uri: string, text: string) => bridge().GetSyntaxHighlights(uri, text)
};

export const windowControls = {
  minimize: () => window.runtime?.WindowMinimise(),
  toggleMaximize: () => window.runtime?.WindowToggleMaximise(),
  quit: () => window.runtime?.Quit()
};
