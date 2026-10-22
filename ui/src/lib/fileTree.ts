import type { FileNode, WorkspaceEntryView } from './types';

export function buildFileTree(entries: WorkspaceEntryView[]): FileNode[] {
  const roots: FileNode[] = [];
  const byPath = new Map<string, FileNode>();
  const sorted = [...entries].sort((left, right) => left.path.localeCompare(right.path));

  for (const entry of sorted) {
    const parts = entry.path.split('/').filter(Boolean);
    const parentPath = parts.slice(0, -1).join('/');
    const node: FileNode = {
      name: parts.at(-1) ?? entry.path,
      path: entry.path,
      absolutePath: entry.absolutePath,
      isDirectory: entry.isDirectory,
      symlink: entry.symlink ?? false,
      outsideRoot: entry.outsideRoot ?? false,
      alias: entry.alias ?? false,
      children: []
    };
    const parent = byPath.get(parentPath);
    if (parent?.isDirectory) parent.children.push(node);
    else roots.push(node);
    byPath.set(entry.path, node);
  }

  const sortChildren = (nodes: FileNode[]): void => {
    nodes.sort((left, right) => {
      if (left.isDirectory !== right.isDirectory) return left.isDirectory ? -1 : 1;
      return left.name.localeCompare(right.name);
    });
    nodes.forEach((node) => sortChildren(node.children));
  };
  sortChildren(roots);
  return roots;
}
