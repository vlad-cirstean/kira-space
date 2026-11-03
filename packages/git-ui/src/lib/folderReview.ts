import type { ReviewFileStatus } from '@kira/git-ipc';
import type { FileTreeNode } from './fileTreeModel.ts';
import { reviewAttention } from './reviewAttention.ts';

export interface FolderReview {
  readonly state: boolean | 'indeterminate';
  /** Every file under the directory in the (filtered) tree. */
  readonly paths: readonly string[];
  readonly full: readonly string[];
  readonly notNone: readonly string[];
}

/** One post-order walk: each directory's files and tri-state (`true` all reviewed, `false` none reviewed or partial; a changed file counts as unreviewed). */
export function buildFolderReview(
  tree: readonly FileTreeNode[],
  states: ReadonlyMap<string, ReviewFileStatus> | undefined,
): Map<string, FolderReview> {
  const map = new Map<string, FolderReview>();
  if (!states) return map;
  const walk = (node: FileTreeNode): Omit<FolderReview, 'state'> => {
    if (node.kind === 'file') {
      const path = node.change.path;
      const attention = reviewAttention(states.get(path));
      return {
        paths: [path],
        full: attention === 'reviewed' ? [path] : [],
        notNone: attention === 'reviewed' || attention === 'partial' ? [path] : [],
      };
    }
    const paths: string[] = [];
    const full: string[] = [];
    const notNone: string[] = [];
    for (const child of node.children) {
      const part = walk(child);
      paths.push(...part.paths);
      full.push(...part.full);
      notNone.push(...part.notNone);
    }
    let state: FolderReview['state'] = 'indeterminate';
    if (full.length === paths.length) state = true;
    else if (notNone.length === 0) state = false;
    map.set(node.path, { state, paths, full, notNone });
    return { paths, full, notNone };
  };
  for (const node of tree) walk(node);
  return map;
}

/** The files a folder click changes: unmark every touched file when all are full, else mark the rest. */
export function folderToggle(folder: FolderReview): { paths: string[]; reviewed: boolean } {
  if (folder.state === true) return { paths: [...folder.notNone], reviewed: false };
  const full = new Set(folder.full);
  return { paths: folder.paths.filter((p) => !full.has(p)), reviewed: true };
}
