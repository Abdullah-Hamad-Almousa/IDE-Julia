import EditorWorker from 'monaco-editor/editor/editor.worker.js?worker';

// Monaco reads its worker hook while the editor API module is initialized, so
// this module must execute before syntaxBridge imports that API.
globalThis.MonacoEnvironment = {
  getWorker: () => new EditorWorker()
};
