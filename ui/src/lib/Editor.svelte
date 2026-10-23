<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import type * as monaco from 'monaco-editor';
  import {
    applyDocument,
    createJuliaEditor,
    disposeEditor,
    removeDocumentModel,
    setEditorOpener,
    updateJuliaHighlights
  } from './syntaxBridge';
  import type { DocumentView } from './types';

  export let document: DocumentView | null = null;
  export let openPaths: string[] = [];
  export let onChange: (path: string, text: string) => void;
  export let onSave: (path: string, text: string) => void;
  export let onCursor: (line: number, column: number) => void;
  export let onOpen: (path: string) => Promise<DocumentView>;
  export let onActivate: (path: string) => void;
  export let targetPosition: { path: string; line: number; column: number } | null = null;

  let host: HTMLDivElement;
  let editor: monaco.editor.IStandaloneCodeEditor;
  let syncing = false;
  let activePath = '';
  let knownPaths = new Set<string>();
  let editorOpener: { dispose(): void } | undefined;

  onMount(() => {
    editor = createJuliaEditor(host, (_path, text) => {
      if (!syncing && activePath) onChange(activePath, text);
    }, (_path, text) => {
      if (activePath) onSave(activePath, text);
    });
    editor.onDidChangeCursorPosition((event) => onCursor(event.position.lineNumber, event.position.column));
    editorOpener = setEditorOpener(onOpen, onActivate);
    if (document) showDocument(document);
  });

  $: if (editor && document) showDocument(document);
  $: if (editor) removeClosedModels(openPaths);

  function showDocument(next: DocumentView): void {
    if (!editor) return;
    const documentChanged = activePath !== next.path;
    const current = editor.getModel();
    syncing = true;
    try {
      if (current?.uri.fsPath === next.path && current.getValue() === next.text) {
        // Keep the existing model and its cursor/undo history.
      } else {
        applyDocument(editor, next);
      }
      activePath = next.path;
      knownPaths.add(next.path);
    } finally {
      syncing = false;
    }
    if (documentChanged) editor.focus();
    if (targetPosition?.path === next.path) {
      editor.setPosition({ lineNumber: targetPosition.line, column: targetPosition.column });
      editor.revealPositionInCenter({ lineNumber: targetPosition.line, column: targetPosition.column });
    }
    updateJuliaHighlights();
  }

  function removeClosedModels(paths: string[]): void {
    const open = new Set(paths);
    for (const path of knownPaths) {
      if (!open.has(path)) {
        removeDocumentModel(path);
        knownPaths.delete(path);
      }
    }
  }

  onDestroy(() => {
    editorOpener?.dispose();
    if (editor) disposeEditor(editor);
    for (const path of knownPaths) removeDocumentModel(path);
  });
</script>

<div class="editor-host" bind:this={host} aria-label="Julia source editor"></div>

<style>
  .editor-host { height: 100%; min-height: 0; width: 100%; }
</style>
