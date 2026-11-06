import type { TabularPage } from '@shared/protocol/page';
import { pageColumnIndexFor } from '../../shared/page/columns';
import type { RowHandle } from '../../shared/slick/dataSource';
import { type CellView, cell } from '../page';
import { usePendingChangesStore } from '../pendingChanges';

// The generic display-position/`CustomDataView` core that used to live in
// this one file moved to `views/shared/slick/dataSource.ts` (`views/console/*` may not
// import `views/grid/*`, and that core has no dependency on a data tab in the first place). This
// file keeps only what genuinely does: `createDisplayValueExtractor`/`pendingRowClasses` both read
// `pendingChanges.ts`. Re-exported verbatim so `SlickGridHost.vue`'s own import
// (`from './slick/dataSource'`) is untouched.
export type { GridDataSourceState, RowHandle } from '../../shared/slick/dataSource';
export {
  createGridDataSource,
  displayPositionOf,
  isRowVisible,
  rowAtDisplayPosition,
} from '../../shared/slick/dataSource';

/**
 * Builds the `dataItemColumnValueExtractor` closure itself (§6 D1's pseudocode) — routes every cell
 * access through the app's EXISTING decode/cache/staged-edit pipeline: `page.ts`'s `cell()` (which
 * memoises both the decoded string and the built `CellView`, `store.ts`'s `cachedView`) layered with
 * `pendingChanges.ts`'s `stagedValue()`, exactly as DataGrid.vue's own `displayCell` does — nothing
 * about that pipeline is rebuilt here, only re-entered. `fieldToPageCol` is resolved once per
 * column-order rebuild (not per cell) via the same `pageColumnIndexFor` DataGrid.vue itself calls,
 * so a repeat lookup for an already-decoded cell is allocation-free (page.ts's own memoisation) and
 * a repeat lookup for a column position is an O(1) map read.
 */
export function createDisplayValueExtractor(
  tabId: string,
  page: TabularPage,
  columnOrder: string[],
  // M5 §6.2: the grid preview's own transform, resolved by SlickGridHost.vue's dataSourceState
  // from the tab's rules and this page's columns — `undefined` when mask preview is off, in
  // which case this extractor is byte-for-byte what it was before M5.
  maskTransform?: (view: CellView, field: string) => CellView,
): (item: RowHandle, field: string) => CellView {
  const fieldToPageCol = new Map<string, number>();
  for (let i = 0; i < columnOrder.length; i++) {
    fieldToPageCol.set(columnOrder[i], pageColumnIndexFor(page, columnOrder, i));
  }
  // An earlier version of this fix snapshotted
  // `pending` once here, at extractor-build time, on the theory that SlickGridHost.vue always
  // rebuilds `dataSource.setState` (and therefore this extractor) on every staging change. That
  // theory was wrong and a real regression (caught by tests/ui/mutations.spec.ts,
  // tests/ui/cell-editor.spec.ts and tests/ui/interaction.spec.ts all failing): SlickGridHost.vue's
  // own watch (the `pendingFor(tabId)?.edits`/`.deletes` signature) invalidates and
  // re-renders the touched rows on every stageEdit/stageNull/stageDelete, but never calls
  // `dataSource.setState` to rebuild this extractor — its own comment says exactly why: "a
  // committed edit renders correctly for free" *because* the old closure re-read stagedValue()
  // live on every cell render. `pendingState[tabId]` is also created lazily (pendingChanges.ts's
  // own `ensure()`), so an extractor built before a tab's first-ever staged change captured
  // `pending = undefined` permanently — no later edit could ever appear, since nothing ever
  // rebuilds this closure to re-observe it. `rawPendingFor` is therefore called *inside* the
  // returned closure, once per cell as before, but through `toRaw` — this still removes 3 of the
  // original 4 proxy traps per cell (the Map wrapper, its `.get()` result wrapper, and the
  // `.changes` field wrapper), leaving only the one unavoidable top-level `pendingState[tabId]`
  // read that must stay live for correctness.
  return (item, field) => {
    const pending = usePendingChangesStore().rawPendingFor(tabId);
    if (item.insertId !== undefined) {
      const value = pending?.inserts.find((i) => i.id === item.insertId)?.values[field];
      return { text: value ?? '', isNull: value === null || value === undefined, truncated: false };
    }
    const staged = usePendingChangesStore().rawStagedValue(tabId, item.row, field);
    if (staged !== undefined) {
      return { text: staged ?? '', isNull: staged === null, truncated: false };
    }
    const pageCol = fieldToPageCol.get(field) ?? -1;
    if (pageCol < 0) return { text: '', isNull: true, truncated: false };
    const view = cell(tabId, item.row, pageCol);
    // M5 §6.2: applied last, over whatever the existing pipeline produced — never mutates `view`
    // itself (maskPreview.ts's own header comment on why that matters: `cell()`'s own return
    // value is memoised by page.ts's `store.cachedView`).
    return maskTransform ? maskTransform(view, field) : view;
  };
}

/** The dirty/deleted-rail source for `GridDataSourceState.rowClasses` (§5 item 18) — a page row
 *  index in, the same two mutually-exclusive rail classes GridRow.vue's own CSS already draws
 *  (`.gutter-cell.dirty`/`.gutter-cell.deleted`, ported verbatim in `slickTheme.css`). */
// The third case, checked first: a pending-insert row is never simultaneously a real, deleted, or
// dirty page row (it has no page row at all — `row` here is `pageRowAt`'s own pseudo-row-number
// past `pageRowCount`, the insert region), so it can never collide with the other two.
// `pending-delete` is DataGrid.vue's own literal class name (`GridRow.vue:41` — the
// strike-through/opacity rule), added alongside `kira-row-deleted` (this file's own rail-only
// class, `slickTheme.css`'s `.kira-gutter::before` selector) rather than replacing it — the two
// mechanisms are independent and this pass ports both.
export function pendingRowClasses(
  tabId: string,
  row: number,
  pageRowCount: number,
): string | undefined {
  if (row >= pageRowCount) return 'kira-row-inserted';
  const change = usePendingChangesStore().rawRowChange(tabId, row);
  if (change === 'delete') return 'kira-row-deleted pending-delete';
  return change === 'edit' ? 'kira-row-dirty' : undefined;
}
