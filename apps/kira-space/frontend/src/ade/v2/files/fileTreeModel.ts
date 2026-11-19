import type { FileStatusCode } from '@shared/domain/repo';
import type { StickyRowLike } from '@theme/stickyBand';
import { compareNatural } from '@workbench/behaviour/match';
import { markRaw } from 'vue';

// The flat path list -> row model fold. No single-child directory compaction (VS Code's
// `a/b/c` collapsing) — deliberately declined: it complicates the fold, the expand set and reveal,
// for a cosmetic gain (so a later phase adds it on purpose rather than by accident).
export interface FileNode {
  name: string;
  nameLower: string; // C13-6: precomputed once here rather than on every filter-pass visit
  path: string; // repository-relative, '' only for the synthetic root
  isDir: boolean;
  children: FileNode[];
}

export function buildTree(paths: readonly string[]): FileNode[] {
  const root: FileNode = { name: '', nameLower: '', path: '', isDir: true, children: [] };
  const dirs = new Map<string, FileNode>([['', root]]);

  for (const p of paths) {
    const segments = p.split('/');
    let parent = root;
    let acc = '';
    for (let i = 0; i < segments.length - 1; i++) {
      acc = acc ? `${acc}/${segments[i]}` : segments[i];
      let node = dirs.get(acc);
      if (!node) {
        node = {
          name: segments[i],
          nameLower: segments[i].toLowerCase(),
          path: acc,
          isDir: true,
          children: [],
        };
        dirs.set(acc, node);
        parent.children.push(node);
      }
      parent = node;
    }
    const name = segments[segments.length - 1];
    parent.children.push({
      name,
      nameLower: name.toLowerCase(),
      path: p,
      isDir: false,
      children: [],
    });
  }

  sortChildren(root);
  // The tree is replaced wholesale on every refresh (never field-mutated afterward — the
  // only per-node state that changes live, `expanded`/`status`, lives separately in the files UI store)
  // — markRaw so assigning this into `state.tree` (inside a reactive object)
  // doesn't deep-proxy every node, at every level, of what can be a 200,000-entry tree.
  return markRaw(root.children);
}

// Directories before files, each collator-sorted case-insensitive (§7.2).
function sortChildren(node: FileNode): void {
  node.children.sort((a, b) => {
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
    return compareNatural(a.name, b.name);
  });
  for (const child of node.children) if (child.isDir) sortChildren(child);
}

function subtreeMatches(node: FileNode, query: string): boolean {
  if (node.nameLower.includes(query)) return true;
  return node.isDir && node.children.some((c) => subtreeMatches(c, query));
}

export interface FileTreeRowVm extends StickyRowLike {
  key: string;
  name: string;
  path: string;
  isDir: boolean;
  status: FileStatusCode | undefined;
  matched: boolean;
}

export function flatten(
  nodes: readonly FileNode[],
  depth: number,
  expanded: ReadonlySet<string>,
  status: Readonly<Record<string, string>>,
  query: string,
  out: FileTreeRowVm[],
): void {
  for (const node of nodes) {
    const matched = query !== '' && node.nameLower.includes(query);
    // C13-6a: subtreeMatches is a full recursive subtree walk — computed once and reused for both
    // the skip guard and the expand check below, instead of calling it twice per directory node.
    const dirMatches = query !== '' && node.isDir && subtreeMatches(node, query);
    if (query !== '' && !matched && !dirMatches) continue;
    // §7.2: the search box force-expands matching ancestors.
    const isExpanded = node.isDir && (expanded.has(node.path) || dirMatches);
    out.push({
      key: node.path || '/',
      name: node.name,
      path: node.path,
      depth,
      hasChildren: node.isDir && node.children.length > 0,
      expanded: isExpanded,
      isDir: node.isDir,
      status: status[node.path] as FileStatusCode | undefined,
      matched,
    });
    if (node.isDir && isExpanded) flatten(node.children, depth + 1, expanded, status, query, out);
  }
}
