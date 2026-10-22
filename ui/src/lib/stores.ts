import { derived, writable } from 'svelte/store';
import type { DocumentView, WorkspaceView } from './types';

// These stores contain presentation state for the shell. File and workspace
// content is kept by the Go workspace/VFS services and mirrored for rendering.
export const workspaceStore = writable<WorkspaceView | null>(null);
export const documentsStore = writable<DocumentView[]>([]);
export const activeDocumentPathStore = writable<string | null>(null);
export const activeDocumentStore = derived(
  [documentsStore, activeDocumentPathStore],
  ([$documents, $activePath]) => $documents.find((document) => document.path === $activePath) ?? null
);

export function replaceDocument(document: DocumentView): void {
  documentsStore.update((documents) => {
    const index = documents.findIndex((item) => item.path === document.path);
    if (index < 0) return [...documents, document];
    const next = [...documents];
    next[index] = document;
    return next;
  });
}
