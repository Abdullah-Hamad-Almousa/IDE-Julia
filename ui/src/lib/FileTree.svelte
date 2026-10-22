<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { FileNode } from './types';

  export let entries: FileNode[] = [];
  export let selectedPath = '';

  const dispatch = createEventDispatcher<{ open: FileNode }>();
  let expanded = new Set<string>();

  function toggle(node: FileNode): void {
    const next = new Set(expanded);
    if (next.has(node.path)) next.delete(node.path);
    else next.add(node.path);
    expanded = next;
  }

  function activate(node: FileNode): void {
    if (node.isDirectory) {
      toggle(node);
      return;
    }
    if (!node.outsideRoot && !node.alias) dispatch('open', node);
  }
</script>

<ul class="tree-list" role="tree">
  {#each entries as entry (entry.path)}
    <li role="treeitem" aria-selected={selectedPath === entry.absolutePath} aria-expanded={entry.isDirectory ? expanded.has(entry.path) : undefined}>
      <button
        class:active={selectedPath === entry.absolutePath}
        class:muted={entry.outsideRoot || entry.alias}
        class="tree-entry"
        style={`--depth: ${entry.path.split('/').length - 1}`}
        title={entry.outsideRoot ? 'Link points outside this workspace' : entry.alias ? 'Already shown at another path' : entry.path}
        on:click={() => activate(entry)}
      >
        {#if entry.isDirectory}
          <svg class:rotated={expanded.has(entry.path)} class="disclosure" viewBox="0 0 12 12" aria-hidden="true">
            <path d="m4 2 4 4-4 4" />
          </svg>
          <svg class="tree-icon folder-icon" viewBox="0 0 16 16" aria-hidden="true">
            <path d="M1.8 4.2h4l1.4 1.5h7v6.8H1.8z" />
            <path d="M1.8 5.7h12.4" />
          </svg>
        {:else}
          <span class="disclosure-spacer"></span>
          <svg class="tree-icon file-icon" viewBox="0 0 16 16" aria-hidden="true">
            <path d="M4 1.8h5l3 3v9.4H4z" />
            <path d="M9 1.8v3h3M6 8h4M6 10.5h4" />
          </svg>
        {/if}
        <span class="tree-name">{entry.name}</span>
        {#if entry.symlink}<span class="link-badge" aria-label="symbolic link">↗</span>{/if}
      </button>
      {#if entry.isDirectory && expanded.has(entry.path) && entry.children.length > 0}
        <svelte:self entries={entry.children} {selectedPath} on:open={(event) => dispatch('open', event.detail)} />
      {/if}
    </li>
  {/each}
</ul>

<style>
  .tree-list { list-style: none; margin: 0; padding: 0; }
  li { margin: 0; padding: 0; }
  .tree-entry {
    align-items: center;
    background: transparent;
    border: 0;
    border-radius: 5px;
    color: var(--text-secondary);
    cursor: pointer;
    display: flex;
    font: inherit;
    gap: 7px;
    height: 27px;
    min-width: 0;
    padding: 0 8px 0 calc(7px + var(--depth) * 13px);
    text-align: left;
    width: 100%;
  }
  .tree-entry:hover { background: var(--surface-hover); color: var(--text-primary); }
  .tree-entry.active { background: #172638; color: #eaf6ff; }
  .tree-entry.muted { cursor: not-allowed; opacity: .42; }
  .tree-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .disclosure, .tree-icon { fill: none; flex: 0 0 14px; stroke: currentColor; stroke-linecap: round; stroke-linejoin: round; stroke-width: 1.3; }
  .disclosure { color: #65748b; height: 10px; transition: transform 120ms ease; width: 10px; }
  .disclosure.rotated { transform: rotate(90deg); }
  .disclosure-spacer { flex: 0 0 10px; }
  .folder-icon { color: #82aaff; height: 15px; width: 15px; }
  .file-icon { color: #718096; height: 14px; width: 14px; }
  .link-badge { color: var(--accent); font-size: 11px; margin-left: auto; }
</style>
