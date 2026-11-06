import type { ItemMetadata } from 'slickgrid';

// The load-bearing decision the grid's SlickGrid migration lives or dies on:
// SlickGrid's own `CustomDataView` seam over the app's EXISTING frozen-page/decode-cache/staged-
// edit pipeline, never a materialised row array (that is `SlickDataView`'s job, declined).
// Nothing here is Vue-reactive; `docs/ARCHITECTURE.md`'s Invariants section (no Vue reactivity on
// row data) holds by construction because `RowHandle` is a plain frozen object and
// `GridDataSourceState` is a plain object the host swaps by reassignment, never a `ref`/`reactive`.
//
// This is the generic *core* of what was one file
// (`views/grid/slick/dataSource.ts`) — display-position <-> page-row translation and the
// `CustomDataView` bridge itself, with no dependency on a data tab's pending-changes state. That
// dependency (`createDisplayValueExtractor`/`pendingRowClasses`, which read `pendingChanges.ts`)
// stayed behind in `views/grid/slick/dataSource.ts`, which re-exports everything here verbatim
// (SlickGridHost.vue's own import is unaffected) and adds those two grid-only functions.
// `views/console/ConsoleSlickGrid.vue` imports this module directly — SPEC forbids it
// reaching into `views/grid/*` at all, and this core never needed to live there in the first
// place.

/** What SlickGrid hands a formatter/extractor as the rendered row's "item" — never a materialised
 *  row. `row` is the page-row index every other subsystem (selection, pending changes, search, the
 *  gutter number) addresses a row by; `pos` is display position only (pixel placement / SlickGrid's
 *  own row index), equal to `row` unless a filter is hiding non-matching rows. Frozen: nothing
 *  downstream may treat this as a mutable row. */
export interface RowHandle {
  readonly row: number;
  readonly pos: number;
  /** A pending insert's id, when this position is past the loaded page (D1's insert region). */
  readonly insertId?: string;
}

/** The display-position space this grid renders in: `displayRows` (ascending page-row indices)
 *  when a filter is hiding non-matching rows, `null` when unfiltered — the
 *  `displayPositionOf`/`rowAtDisplayPosition` split, originally written against the
 *  now-deleted DataGrid.vue and reused here rather than re-derived. */
export interface DisplayRowIndex {
  readonly displayRows: readonly number[] | null;
  readonly pageRowCount: number;
}

function displayRowCount(idx: DisplayRowIndex): number {
  return idx.displayRows ? idx.displayRows.length : idx.pageRowCount;
}

/** Total display length: real (possibly filtered) rows plus any pending inserts past the end. */
export function dataLength(idx: DisplayRowIndex, insertCount: number): number {
  return displayRowCount(idx) + insertCount;
}

/**
 * Display position -> `RowHandle`, across the filter and into the pending-insert region past the
 * last display row — the arithmetic the deleted DataGrid.vue's own `rowAtDisplayPosition` used,
 * plus its own `page.rowCount + idx` insert-identity rule (`onPaste`'s own comment).
 * The one place this translation lives for the Slick engine: `getLength`/`getItem` below are its
 * only callers, replacing `displayPositionOf`/`rowAtDisplayPosition`'s use inside the old
 * virtualizer.
 */
/** The row-identity arithmetic alone, without allocating/freezing a
 *  RowHandle. `rowHandleAt` below is defined in terms of this so the two paths can never drift
 *  apart; `getItemMetadata` (createGridDataSource, below) uses this directly so a rendered row
 *  only pays for one frozen RowHandle allocation (getItem's), not two (appendRowHtml calls
 *  getDataItem AND getItemMetadaWhenExists per newly-built row, and the latter never needed a full
 *  handle, only the row number). */
function pageRowAt(idx: DisplayRowIndex, pos: number): number {
  const count = displayRowCount(idx);
  if (pos >= count) return idx.pageRowCount + (pos - count);
  return idx.displayRows ? (idx.displayRows[pos] ?? pos) : pos;
}

/** Display position -> page row, exported: `selection.ts`'s own
 *  `SlickRange`<->`Selection` translation needs exactly this arithmetic (a `SlickRange`'s
 *  row fields are always display positions — SlickGrid indexes the `CustomDataView` it was
 *  handed, never a page row), and it must be the SAME arithmetic `getItem`/`getItemMetadata`
 *  use, not a second implementation that could drift from it. Identity while nothing is
 *  filtered (`idx.displayRows === null`) — this function is correct for that case by
 *  construction, not by luck. */
export function rowAtDisplayPosition(idx: DisplayRowIndex, pos: number): number {
  return pageRowAt(idx, pos);
}

/** The inverse of `rowAtDisplayPosition` — page row -> display position.
 *  `displayRows` is always ascending (matchedRows' own contract), so an exact hit (the common case
 *  — the row is visible) is a binary search, and
 *  a miss (the row was just filtered out from under a live selection) falls through to the
 *  position it would sort into, landing on the nearest visible row instead of doing nothing. */
export function displayPositionOf(idx: DisplayRowIndex, row: number): number {
  const count = displayRowCount(idx);
  if (row >= idx.pageRowCount) return count + (row - idx.pageRowCount); // pending insert
  const dr = idx.displayRows;
  if (!dr) return row; // unfiltered: identity
  let lo = 0;
  let hi = dr.length;
  while (lo < hi) {
    const mid = (lo + hi) >>> 1;
    if ((dr[mid] as number) < row) lo = mid + 1;
    else hi = mid;
  }
  return Math.min(lo, Math.max(0, dr.length - 1));
}

/** `displayPositionOf` deliberately falls through to the nearest visible row
 *  on a miss (its own doc comment: correct for scroll-into-view, wrong for a highlight/selection
 *  that must not silently jump to a neighboring row when its own row gets filtered out). Callers
 *  that need to know "is this page row still on screen" before deciding whether to translate or
 *  clear use this instead — an exact-match check, never a nearest-match fallback. Pending inserts
 *  (`row >= idx.pageRowCount`) are never filtered, so they're always visible. */
export function isRowVisible(idx: DisplayRowIndex, row: number): boolean {
  if (row >= idx.pageRowCount) return true; // pending insert region
  const dr = idx.displayRows;
  if (!dr) return true; // unfiltered: identity
  let lo = 0;
  let hi = dr.length;
  while (lo < hi) {
    const mid = (lo + hi) >>> 1;
    if ((dr[mid] as number) < row) lo = mid + 1;
    else hi = mid;
  }
  return dr[lo] === row;
}

/** A minimal structural stand-in for a pending insert — only `.id` is ever read on this path
 *  (Vue `:key` / discard identity, `views/grid/pendingChanges.ts`'s own `PendingInsert.id`
 *  comment). Kept local, rather than importing that grid-specific type, so this shared module
 *  never depends on a data tab's own pending-changes state; a real `PendingInsert[]` (a superset)
 *  is assignable here structurally, so `views/grid/slick/dataSource.ts`'s own callers pass one
 *  unchanged. */
export interface InsertHandle {
  id: string;
}

export function rowHandleAt(
  idx: DisplayRowIndex,
  inserts: readonly InsertHandle[],
  pos: number,
): RowHandle {
  const count = displayRowCount(idx);
  const row = pageRowAt(idx, pos);
  if (pos >= count) {
    const insert = inserts[pos - count];
    return Object.freeze({ row, pos, insertId: insert?.id });
  }
  return Object.freeze({ row, pos });
}

/** The mutable state a `KiraGridDataSource` reads on every `getItem`/`getLength`/`getItemMetadata`/
 *  `extractValue` call — a plain object the host (SlickGridHost.vue/ConsoleSlickGrid.vue)
 *  reassigns wholesale via `setState` on a relevant change (page reload, filter, a staged insert),
 *  and never a Vue `ref`/`reactive` (the rule: the extractor and formatter run *during*
 *  SlickGrid's own render, and must never touch Vue reactivity whose mutation could re-enter it).
 *
 *  `extractValue` itself lives in this state (not a separate, one-time constructor argument) for a
 *  load-bearing reason: SlickGrid's own `dataItemColumnValueExtractor` grid option is captured
 *  *once*, at construction — routing it through this mutable state (via `KiraGridDataSource.
 *  extractValue`, below) is what lets a page reload's new decode/staged-edit closure reach
 *  the grid without reconstructing the whole instance. A first version of this file passed
 *  `extractValue` as a separate constructor argument instead; every cell after the first page
 *  reload silently kept reading the *original* (often `page === null`, "everything is NULL")
 *  closure, because the grid's own option object is fixed at construction and was never the one
 *  being updated — confirmed against a real render, not merely reasoned about. */
export interface GridDataSourceState {
  index: DisplayRowIndex;
  inserts: readonly InsertHandle[];
  /** Per-page-row CSS classes (the dirty/deleted/inserted gutter rails, §5 item 18) — `undefined`
   *  renders no metadata for that row, matching `getItemMetadata`'s own `| null` contract. */
  rowClasses?: (row: number) => string | undefined;
  /** Per-row column metadata: `undefined` for a normal row (the overwhelming
   *  majority), the insert region's own `{ editor: null, focusable: false }` override for the
   *  handful of pending-insert rows (`getEditor`/`canCellBeActive` both consult this before
   *  the column's own flag, which is what keeps SlickGrid's own editor from ever opening over the
   *  insert row's real `<input>`, and keeps Tab/arrow-key cell navigation off the region). Takes
   *  the handle, not just the row number `pageRowAt` alone could give `getItemMetadata` for free,
   *  because it keys on `insertId` — a full RowHandle is the only thing that carries it.
   *  SlickGridHost.vue only assigns this callback while there is at least one pending insert, so
   *  the common (no staged insert) case never pays for the second handle allocation this adds. */
  rowColumns?: (handle: RowHandle) => ItemMetadata['columns'] | undefined;
  /** The current `dataItemColumnValueExtractor` body — see this interface's own header comment. */
  extractValue: (item: RowHandle, field: string) => unknown;
}

export interface KiraGridDataSource {
  getLength(): number;
  getItem(pos: number): RowHandle;
  getItemMetadata(pos: number): { cssClasses?: string; columns?: ItemMetadata['columns'] } | null;
  /**
   * The current state's own `extractValue`, keyed by `RowHandle` directly — the exact shape
   * SlickGrid's own `dataItemColumnValueExtractor` option calls with (`item`, not a position; that
   * grid option receives the *already-resolved* item, not an index to resolve). The host's grid
   * option is set to this method once, at construction, and stays correct across every later
   * `setState` because this method reads live `state`, not a captured closure — see
   * `GridDataSourceState`'s own header comment for why that distinction is load-bearing.
   */
  extractValue(item: RowHandle, field: string): unknown;
  /**
   * The insurance, not a real render-path seam (the render path is `extractValue`/
   * `dataItemColumnValueExtractor`, above): a future call site — or SlickGrid's own
   * `autosizeColumns`, left off via `autosizeColsMode: LegacyOff` but still a public method someone
   * could call — that reaches for `getCellValue` gets the extractor's own answer instead of
   * silently falling through to `getDataItem(i)[field]`, which would read `undefined` for every
   * column on a `RowHandle` that only ever has `row`/`pos`/`insertId`.
   */
  getCellValue(index: number, field: string): unknown;
}

/**
 * A stable `CustomDataView` over frozen pages. Never materialises a row, never allocates one for a
 * row SlickGrid isn't building — a retained row is never revisited, so this object's
 * own identity, and the `state` it closes over, are the only things that change across a render.
 */
export function createGridDataSource(
  initialState: GridDataSourceState,
): KiraGridDataSource & { setState(next: GridDataSourceState): void } {
  let state = initialState;
  return {
    setState(next) {
      state = next;
    },
    getLength() {
      return dataLength(state.index, state.inserts.length);
    },
    getItem(pos) {
      return rowHandleAt(state.index, state.inserts, pos);
    },
    getItemMetadata(pos) {
      // P22 iter2-pacing D6: the row number alone, not a second frozen RowHandle allocation — see
      // pageRowAt's own comment.
      const cssClasses = state.rowClasses
        ? state.rowClasses(pageRowAt(state.index, pos))
        : undefined;
      // C9/§5 D9 — the second handle allocation `rowColumns`'s own header comment describes,
      // gated behind the callback actually being assigned (the common no-insert case never sets
      // it, so this branch is a single `undefined` check there, not a RowHandle build).
      if (state.rowColumns) {
        const columns = state.rowColumns(rowHandleAt(state.index, state.inserts, pos));
        if (columns) return cssClasses ? { cssClasses, columns } : { columns };
      }
      return cssClasses ? { cssClasses } : null;
    },
    extractValue(item, field) {
      return state.extractValue(item, field);
    },
    getCellValue(pos, field) {
      return state.extractValue(rowHandleAt(state.index, state.inserts, pos), field);
    },
  };
}
