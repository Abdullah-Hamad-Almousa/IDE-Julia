/// <reference types="vite/client" />

declare global {
  // Wails lets the host application provide workers for Monaco's editor core.
  var MonacoEnvironment: {
    getWorker: (moduleId: string, label: string) => Worker;
  };
}

export {};
