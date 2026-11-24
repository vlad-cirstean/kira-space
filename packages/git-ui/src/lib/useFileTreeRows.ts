import { rowIndent } from '@theme/lib/rowVariants';
import type { useCappedList } from '@workbench/behaviour/cappedList';
import { revealElement } from '@workbench/behaviour/focus';
import { useTreeVirtualRows } from '@workbench/behaviour/treeVirtualRows';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { KEPT_VIEW_STATE_KEY } from '@workbench/state/viewState';
import { computed, inject, nextTick, type Ref, type StyleValue } from 'vue';
import {
  buildFlatList,
  type FileTreeNode,
  type FileTreeRow,
  type filterFiles,
  flattenTreeRows,
} from './fileTreeModel.ts';
import { keptScrollIf } from './reviewKeep.ts';

/** A row plus what `useTreeVirtualRows` needs: a stable `key` and the sticky-band row shape. */
export type KeyedRow = FileTreeRow & { key: string; hasChildren: boolean; expanded: boolean };

export interface RenderRow {
  row: KeyedRow;
  /** Index in the full row list: the capped page is a prefix, so it holds there too. */
  index: number;
  style: StyleValue;
}

interface RowListInput {
  indexed: Readonly<Ref<ReturnType<typeof filterFiles>>>;
  tree: Readonly<Ref<readonly FileTreeNode[]>>;
  listMode: () => 'tree' | 'flat';
  isExpanded: (path: string) => boolean;
}

/** The flat keyed row list of the filtered tree (or flat list). */
export function useFileTreeRowList(input: RowListInput) {
  return computed<KeyedRow[]>(() => {
    const raw: readonly FileTreeRow[] =
      input.listMode() === 'flat'
        ? buildFlatList(input.indexed.value).map((node) => ({
            kind: 'file' as const,
            node,
            depth: 0,
          }))
        : flattenTreeRows(input.tree.value, input.isExpanded);
    return raw.map((row) => ({
      ...row,
      key: `${row.kind === 'directory' ? 'dir' : 'file'}:${row.node.path}`,
      hasChildren: row.kind === 'directory',
      expanded: row.kind === 'directory' && row.expanded,
    }));
  });
}

interface WindowInput {
  rows: Readonly<Ref<readonly KeyedRow[]>>;
  /** The capped page over `rows` (`useCappedList`), used only when `capped`. */
  cap: ReturnType<typeof useCappedList<KeyedRow>>;
  /** No bounded parent (an expanded review commit row): first page plus "Show more", no window. */
  capped: () => boolean;
  keepScroll: () => string | null;
  treeEl: Ref<HTMLElement | null>;
}

/** Either the virtual window over `rows` or the capped page, whichever the mount asks for. */
export function useFileTreeWindow(input: WindowInput) {
  const { rows, cap } = input;
  const cappedRows = cap.visible;
  const hiddenCount = computed(() => (input.capped() ? cap.hiddenCount.value : 0));

  const { single: rowHeight } = useRowHeight();
  // Scroll survives an unmount only when the host keeps view state.
  const kept = inject(KEPT_VIEW_STATE_KEY, null);
  const { virtualItems, totalSize, onScroll, revealKey } = useTreeVirtualRows({
    rows: () => (input.capped() ? [] : rows.value),
    rowHeight: () => rowHeight.value,
    scrollElement: input.treeEl,
    keepScroll: kept ? () => (input.capped() ? null : input.keepScroll()) : null,
  });
  if (input.capped()) {
    keptScrollIf(input.keepScroll, input.treeEl, { ready: () => cappedRows.value.length > 0 });
  }

  /** What keyboard navigation and the cursor see: every row when virtual, the shown page when capped. */
  const navRows = computed<readonly KeyedRow[]>(() =>
    input.capped() ? cappedRows.value : rows.value,
  );

  /** What the template paints: the virtual window, or the capped page in normal flow. */
  const renderRows = computed<RenderRow[]>(() => {
    if (input.capped()) {
      return cappedRows.value.map((row, index) => ({ row, index, style: rowIndent(row.depth) }));
    }
    const out: RenderRow[] = [];
    for (const item of virtualItems.value) {
      const row = rows.value[item.index];
      if (!row) continue;
      out.push({
        row,
        index: item.index,
        style: {
          ...rowIndent(row.depth),
          transform: `translateY(${item.start}px)`,
          height: `${item.size}px`,
        },
      });
    }
    return out;
  });

  /** Virtual mode scrolls the window to the row; capped mode raises the cap past it, then scrolls. */
  async function revealRow(key: string): Promise<void> {
    if (!input.capped()) return revealKey(key);
    const at = rows.value.findIndex((row) => row.key === key);
    if (at < 0) return;
    while (at >= cappedRows.value.length) cap.showMore();
    await nextTick();
    const el = [...(input.treeEl.value?.querySelectorAll('[data-nav-key]') ?? [])].find(
      (node) => node.getAttribute('data-nav-key') === key,
    );
    if (el) revealElement(el);
  }

  return {
    navRows,
    renderRows,
    hiddenCount,
    totalSize,
    onScroll,
    virtualItems,
    revealKey,
    revealRow,
  };
}
