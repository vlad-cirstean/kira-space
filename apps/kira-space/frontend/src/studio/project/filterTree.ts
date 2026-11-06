import type { ConnectionKind } from '@shared/domain/connection';
import type { NodeKind, TreeNode } from '@shared/domain/tree';
import type { TreeVisibility } from '@shared/domain/treeFilter';
import { t } from '@theme/i18n';
import { compareNatural, matchesQuery } from '@workbench/behaviour/match';
import { useConnectionsStore } from '../state/connections';
import { isVisible, toSets } from './filter';
import { isLeafNode, labelForKind } from './grouping';
import { rowKey, useTreeStore } from './state/tree';

// The dialog's model, derived from the same treeStore.children cache the tree renders from — no
// IPC, no fetch (§0, F9's "cached nodes").

export interface FilterKindRow {
  kind: NodeKind;
  label: string;
  count: number;
  hidden: boolean;
}

/** One row of the expandable object tree. Flat + `depth`, the same shape the real tree uses
 *  (F1) — so the dialog needs no recursive component and no second indentation convention. */
export interface FilterNodeRow {
  path: string;
  name: string;
  kind: NodeKind;
  depth: number;
  hasChildren: boolean;
  childCount: number;
  /** 'partial' => visible, but something under it is hidden. */
  state: 'on' | 'off' | 'partial';
  /** True when this node is already hidden by its *kind*: the checkbox is disabled and says so,
   *  rather than silently disagreeing with the tree (D16). */
  kindHidden: boolean;
  /** kindHidden, or an ancestor row is itself hidden — either way ticking this exact row would
   *  have no visible effect, so the checkbox is disabled. */
  disabled: boolean;
  disabledReason: string | null;
}

function connectionKindFor(connectionId: string): ConnectionKind | undefined {
  return useConnectionsStore().connectionRecord(connectionId)?.kind;
}

export function kindRows(connectionId: string, v: TreeVisibility): FilterKindRow[] {
  const counts = new Map<NodeKind, number>();
  const prefix = `${connectionId}|`;
  for (const [key, nodes] of Object.entries(useTreeStore().children)) {
    if (!key.startsWith(prefix)) continue;
    for (const node of nodes) {
      counts.set(node.kind, (counts.get(node.kind) ?? 0) + 1);
    }
  }
  const connectionKind = connectionKindFor(connectionId);
  const hidden = new Set(v.hiddenKinds);
  return [...counts.entries()]
    .sort((a, b) => compareNatural(a[0], b[0]))
    .map(([kind, count]) => ({
      kind,
      label: connectionKind ? labelForKind(kind, connectionKind) : kind,
      count,
      hidden: hidden.has(kind),
    }));
}

export function nodeRows(
  connectionId: string,
  v: TreeVisibility,
  expandedPaths: ReadonlySet<string>,
  nameFilter: string,
): FilterNodeRow[] {
  const hiddenKinds = new Set(v.hiddenKinds);
  const hiddenPaths = new Set(v.hiddenPaths);
  const connectionKind = connectionKindFor(connectionId);
  // P41 D7: a keyBrowser connection's database/bucket rows are leaves here too — its Objects
  // section stops at the container, and its Object types section only ever lists kinds still
  // cached under one (F24), same as the tree itself.
  const keyBrowser = useConnectionsStore().states[connectionId]?.caps?.keyBrowser === true;
  const query = nameFilter.trim().toLowerCase();
  const treeStore = useTreeStore();
  const rootChildren = treeStore.children[rowKey(connectionId, '')] ?? [];

  const childrenOf = (node: TreeNode): TreeNode[] | undefined =>
    isLeafNode(node, keyBrowser) ? undefined : treeStore.children[rowKey(connectionId, node.path)];

  // Whether `node` or anything cached beneath it matches the name filter — used only when a
  // filter is active, to decide which branches survive and are force-expanded (D17).
  function matches(node: TreeNode): boolean {
    if (matchesQuery(node.name, query)) return true;
    return childrenOf(node)?.some(matches) ?? false;
  }

  // Effective visibility, independent of the dialog's own expand/collapse state — a container's
  // tri-state depends on its whole cached subtree, not just the rows currently shown.
  function stateOf(node: TreeNode, ancestorHidden: boolean): 'on' | 'off' | 'partial' {
    const ownHidden = hiddenKinds.has(node.kind) || hiddenPaths.has(node.path);
    if (ownHidden || ancestorHidden) return 'off';
    const children = childrenOf(node);
    if (!children || children.length === 0) return 'on';
    return children.every((c) => stateOf(c, false) === 'on') ? 'on' : 'partial';
  }

  function reasonFor(node: TreeNode, ancestorHiddenName: string | null): string | null {
    if (hiddenKinds.has(node.kind)) {
      const label = connectionKind ? labelForKind(node.kind, connectionKind) : node.kind;
      return t('studio.project.filtersDialog.reason.kindHidden.tooltip', { label });
    }
    if (ancestorHiddenName)
      return t('studio.project.filtersDialog.reason.ancestorHidden.tooltip', {
        name: ancestorHiddenName,
      });
    return null;
  }

  const rows: FilterNodeRow[] = [];

  function emit(
    nodes: TreeNode[],
    depth: number,
    ancestorHidden: boolean,
    ancestorHiddenName: string | null,
  ): void {
    for (const node of nodes) {
      if (query && !matches(node)) continue;

      const kindHidden = hiddenKinds.has(node.kind);
      const ownPathHidden = hiddenPaths.has(node.path);
      const state = stateOf(node, ancestorHidden);
      const children = childrenOf(node);

      rows.push({
        path: node.path,
        name: node.name,
        kind: node.kind,
        depth,
        hasChildren: !isLeafNode(node, keyBrowser) && node.hasChildren,
        childCount: children?.length ?? 0,
        state,
        kindHidden,
        disabled: kindHidden || ancestorHidden,
        disabledReason: reasonFor(node, ancestorHiddenName),
      });

      const expand = query ? true : expandedPaths.has(node.path);
      if (expand && children) {
        const nowHiddenName = kindHidden || ownPathHidden ? node.name : ancestorHiddenName;
        emit(children, depth + 1, ancestorHidden || kindHidden || ownPathHidden, nowHiddenName);
      }
    }
  }

  emit(rootChildren, 0, false, null);
  return rows;
}

/** The mockup's live-consequence strip (F9): nodes the tree will show, of nodes cached. Flat over
 *  every cached node under the connection, matching the tree's own per-node isVisible() check —
 *  same shape the old dialog's preview computed. */
export function previewCounts(
  connectionId: string,
  v: TreeVisibility,
): { shown: number; total: number } {
  const sets = toSets(v);
  const prefix = `${connectionId}|`;
  let shown = 0;
  let total = 0;
  for (const [key, nodes] of Object.entries(useTreeStore().children)) {
    if (!key.startsWith(prefix)) continue;
    for (const node of nodes) {
      total += 1;
      if (isVisible(node, sets)) shown += 1;
    }
  }
  return { shown, total };
}

export function toggleKind(v: TreeVisibility, kind: NodeKind): TreeVisibility {
  const hidden = new Set(v.hiddenKinds);
  if (hidden.has(kind)) hidden.delete(kind);
  else hidden.add(kind);
  return { hiddenKinds: [...hidden], hiddenPaths: v.hiddenPaths };
}

/** Ticking/unticking one node, returning the next set — hiding a container drops the
 *  now-redundant entries beneath it, so the persisted set stays minimal (D15); un-hiding one
 *  restores its whole subtree exactly as it was, for the same reason. */
export function toggleNode(v: TreeVisibility, row: FilterNodeRow): TreeVisibility {
  const paths = new Set(v.hiddenPaths);
  const prefix = `${row.path}/`;
  for (const p of paths) {
    if (p.startsWith(prefix)) paths.delete(p);
  }
  if (row.state === 'on') paths.add(row.path);
  else paths.delete(row.path);
  return { hiddenKinds: v.hiddenKinds, hiddenPaths: [...paths] };
}
