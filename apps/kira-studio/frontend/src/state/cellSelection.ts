import type { ColumnDescriptor } from '@shared/protocol/page';
import { defineStore } from 'pinia';
import { reactive, toRefs } from 'vue';

// The only thing that crosses between the grid and the cell editor (D1): `views/shared/celleditor/`
// imports nothing from `views/grid/` — §11 forbids sideways view imports, and this publication
// is the seam that keeps it that way. P8/P10 publish into the same slot for their own views.
export interface SelectedCell {
  tabId: string;
  connectionId: string | null;
  /** Encoded NodePath of the tab's target — the override key's second component (D12). */
  path: string;
  /** Index into the page's own `columns`/`chunks`, never a display position (§0 note 4). */
  columnIndex: number;
  column: ColumnDescriptor;
  /** Row index within the loaded page, 0-based, as the grid's gutter shows it minus one. */
  row: number;
  /** The decoded server text. `null` means SQL NULL — an empty string is `''` (D14). */
  value: string | null;
  /** Stored value under a staged edit of this cell; absent when nothing is staged. The cell editor
   *  diffs its buffer against it. */
  original?: string | null;
  /** The engine cut this value at MAX_CELL_BYTES; the rest was never fetched (D14). */
  truncated: boolean;
  /** True when `value` is the grid's own mask-preview redaction, not the stored value —
   *  `ReadOnlyReason`'s `'masked'` arm (views/shared/celleditor/state.ts) reads this first, ahead
   *  of every other reason, since it is the one the user can fix by toggling the preview off.
   *  `undefined`/absent means "this view has no masking concept at all" (every publisher other
   *  than the grid), which `readOnlyReasonFor` treats identically to `false`. */
  masked?: boolean;
  /** Whether the page has a primary key at all (P5 D14) — computed once here, since whether a
   *  page has one is grid-only knowledge and `views/shared/celleditor/` may not import `views/grid/`. */
  hasPrimaryKey: boolean;
  /** The server computes a generated column's value — an UPDATE into one is
   *  refused at commit. `readOnlyReasonFor` reads this to block the dock the same way the grid's
   *  own `onBeforeEditCell` does; `undefined` means "this view has no generated-column concept"
   *  (every publisher other than the grid), treated identically to `false`. */
  generated?: boolean;
  /** F3: this row is staged for delete right now. Republished whenever that changes (not just
   *  when the selection itself moves) so the dock can't go on offering Save for a row the grid
   *  would refuse it for — `pendingChanges.ts`'s own `stageEdit` already refuses it silently. */
  pendingDelete?: boolean;
  /** The shown data outlived its connection (useStaleDataStore) — `readOnlyReasonFor` returns
   *  `'connection-lost'` first. */
  stale?: boolean;
  /** Set only by a publisher that can genuinely stage a write for this exact cell — today only
   *  `DataGrid.vue`, closing over `stageEdit(tabId, row, columnName, newValue)`, and
   *  `KeyValueView.vue`, for an S3 object's own editable `Body` row. `StreamView.vue` and
   *  `ConsoleResultGrid.vue` publish too but never set this (both are viewers — the
   *  dock mounting them passes `readOnly`). `undefined` means "this view never lets the panel edit
   *  its cells" — the panel forces read-only whenever this is absent, regardless of
   *  `readOnlyReasonFor()`, so a publisher that never sets it keeps its cells read-only in the
   *  panel by default rather than needing to opt out. `views/documents/` publishes nothing at all
   *  (a document's own row is already the read/write surface) and mounts no dock. */
  onEdit?: (newValue: string) => void;
  /** Sibling to `onEdit`, closing over `discardCellEdit(tabId, row, columnName)` — what the panel's
   *  Revert button calls to un-stage a pending edit, not just visually reset its own buffer.
   *  Always set alongside `onEdit` by the same publisher; never set without it. */
  onRevert?: () => void;
}

export const useCellSelectionStore = defineStore('cellSelection', () => {
  // P26 D2/D3: one record per tab rather than one global slot — the dock that renders a tab's
  // selection is now mounted by the view that owns that tab, so "which cell is selected" is
  // per-tab state like every other piece of tab state (views/*/state.ts's runtimes).
  const state = reactive<{ byTab: Record<string, SelectedCell> }>({ byTab: {} });

  function selectedCellFor(tabId: string): SelectedCell | null {
    return state.byTab[tabId] ?? null;
  }

  // Replaces the tab's record wholesale — never mutates the existing object. CellEditorView
  // compares on `cellKey` plus `value`, so an in-place mutation would be invisible to it.
  function publishSelectedCell(cell: SelectedCell): void {
    state.byTab[cell.tabId] = cell;
  }

  /** No cell is selected in this tab any more — a cleared selection, a row/column selection with
   *  no single cell in it, a page whose rows no longer reach the selected index, or a closed tab
   *  (state/tabs.ts's four close paths). All the same operation. */
  function clearSelectedCellFor(tabId: string): void {
    delete state.byTab[tabId];
  }

  /** Drops every selection whose tab id matches, e.g. a window's tabs for one finished session. */
  function clearSelectedCellsWhere(match: (tabId: string) => boolean): void {
    for (const tabId of Object.keys(state.byTab)) {
      if (match(tabId)) delete state.byTab[tabId];
    }
  }

  return {
    ...toRefs(state),
    selectedCellFor,
    publishSelectedCell,
    clearSelectedCellFor,
    clearSelectedCellsWhere,
  };
});

/** Stable identity of a selection, for `data-cell-key` and for change detection. */
export function cellKey(cell: SelectedCell): string {
  return `${cell.tabId}:${cell.row}:${cell.column.name}`;
}
