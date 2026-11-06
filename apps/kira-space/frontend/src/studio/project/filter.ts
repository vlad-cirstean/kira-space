import type { NodeKind, TreeNode } from '@shared/domain/tree';
import type { TreeVisibility } from '@shared/domain/treeFilter';

// P28 D10/D11: two set lookups replace the old glob/regex rule evaluator. No compiled-pattern
// cache, no glob translation, no FILTERABLE_KINDS gate — every kind and every path is filterable
// now.
export interface VisibilitySets {
  kinds: ReadonlySet<NodeKind>;
  paths: ReadonlySet<string>;
}

export function toSets(v: TreeVisibility): VisibilitySets {
  return { kinds: new Set(v.hiddenKinds), paths: new Set(v.hiddenPaths) };
}

// Filters are applied in the renderer, at render time, over cached nodes. They never change
// what is fetched or cached. Hiding a container hides its subtree implicitly — buildRows() only
// recurses into rows it renders, so a hidden node's children are never reached, and their own
// paths never need an entry in `paths`.
export function isVisible(node: TreeNode, sets: VisibilitySets): boolean {
  return !sets.kinds.has(node.kind) && !sets.paths.has(node.path);
}
