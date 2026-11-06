import type { Column, SlickEventHandler, SlickHybridSelectionModel, SlickRange } from 'slickgrid';
import { type Ref, watch } from 'vue';
import { GUTTER_WIDTH } from '../page/columns';
import type { EdgeHash } from './cssLayers';
import type { Selection } from './selection';

// Pieces views/console/ConsoleSlickGrid.vue and views/grid/SlickGridHost.vue shared as
// byte-identical (or near-identical) copies. Pure
// functions/callbacks only, no reference to either host's own `grid`/`dataSource` module-scope
// `let`s — biome.json's own views/<kind>/* boundary is why this lives here rather than being
// imported cross-kind.

/** The gutter column definition both hosts build first in their own `buildColumns` — identical
 *  shape apart from the formatter (console: plain row number; host: `+` for a pending insert),
 *  the cell testid, and the host's own header "select all" affordance (absent on the read-only
 *  console). `field` is passed rather than hardcoded so each caller keeps its own `GUTTER_FIELD`
 *  constant (identical value today, `'__kira_gutter'`, but not this function's business).
 *  `Column<any>` matches each caller's own `KiraColumn` alias — `Column<T>`'s `field` is a
 *  recursive `PathsToStringProps<T>` no non-`any` type parameter can satisfy for an arbitrary db
 *  column name (same escape hatch kiraSlickGrid.ts/both hosts already use). */
export function gutterColumn(
  field: string,
  // biome-ignore lint/suspicious/noExplicitAny: see doc comment above.
  formatter: Column<any>['formatter'],
  cellTestid: string,
  headerCellAttrs?: Record<string, string>,
  // biome-ignore lint/suspicious/noExplicitAny: see doc comment above.
): Column<any> {
  return {
    id: field,
    field,
    name: '',
    width: GUTTER_WIDTH,
    minWidth: GUTTER_WIDTH,
    maxWidth: GUTTER_WIDTH,
    resizable: false,
    sortable: false,
    focusable: true,
    selectable: true,
    cssClass: 'kira-gutter',
    formatter,
    cellAttrs: { 'data-testid': cellTestid },
    ...(headerCellAttrs ? { headerCellAttrs } : {}),
  };
}

/** The drag-in-progress fill preview. Every caller hands in its own rendered-bounds/
 *  page-row/field mapping. `fieldAtDisplayCol` returning `undefined` (SlickGridHost.vue's
 *  own case, whose columns can be reordered/hidden) skips that cell rather than writing a
 *  `undefined` field key — a no-op for ConsoleSlickGrid.vue's own `colField`, which never returns
 *  undefined. */
export function computeCellFillHash(
  sel: Selection | null,
  renderedBounds: { start: number; end: number },
  pageRowAt: (pos: number) => number,
  fieldAtDisplayCol: (displayCol: number) => string | undefined,
): EdgeHash {
  const hash: EdgeHash = {};
  if (!sel) return hash;
  const { start, end } = renderedBounds;
  if (end < start) return hash;
  if (sel.kind !== 'cell' && sel.kind !== 'range') return hash;
  const anchorRow = sel.kind === 'range' ? sel.anchorRow : sel.row;
  const anchorCol = sel.kind === 'range' ? sel.anchorCol : sel.col;
  const r0 = Math.min(anchorRow, sel.row);
  const r1 = Math.max(anchorRow, sel.row);
  const c0 = Math.min(anchorCol, sel.col);
  const c1 = Math.max(anchorCol, sel.col);
  for (let pos = start; pos <= end; pos++) {
    const pageRow = pageRowAt(pos);
    if (pageRow < r0 || pageRow > r1) continue;
    const row: Record<string, string> = {};
    for (let c = c0; c <= c1; c++) {
      const f = fieldAtDisplayCol(c);
      if (f) row[f] = 'kira-cell-selected';
    }
    hash[pos] = row;
  }
  return hash;
}

/** `grid.onRendered`'s own visible-page-row-band arithmetic — both hosts' own `onGridRendered`
 *  open with this exact sequence (bounds -> length guard -> first/last item's own page row ->
 *  lo/hi) before branching into their own, genuinely different, follow-up work (the host's own
 *  staged-layer refresh and nav-button placement have no console equivalent). Returns `null` where
 *  either host's own guard clause returned early. */
export function renderedPageRowBand(
  grid: { lastRenderedRowBounds: { start: number; end: number } },
  ds: { getLength: () => number; getItem: (pos: number) => { row: number } },
): { start: number; end: number; lo: number; hi: number } | null {
  const { start, end } = grid.lastRenderedRowBounds;
  const length = ds.getLength();
  if (length <= 0 || end < start) return null;
  const first = ds.getItem(Math.max(0, Math.min(start, length - 1)));
  const last = ds.getItem(Math.max(0, Math.min(end, length - 1)));
  return { start, end, lo: Math.min(first.row, last.row), hi: Math.max(first.row, last.row) };
}

/** Both hosts wire their own cell-range-drag preview through this identical
 *  `getCellRangeSelector()` guard — `SlickHybridSelectionModel.getCellRangeSelector()` is
 *  `undefined` under row-only selection mode, in which case there is nothing to subscribe. */
export function subscribeRangeSelecting(
  eventHandler: SlickEventHandler,
  selectionModel: SlickHybridSelectionModel,
  handler: (e: unknown, args: { range: SlickRange }) => void,
): void {
  const cellRangeSelector = selectionModel.getCellRangeSelector();
  if (cellRangeSelector) {
    eventHandler.subscribe(cellRangeSelector.onCellRangeSelecting, handler);
  }
}

/** The `--kira-header-row-height` live row-density watch — byte-identical in both hosts.
 *  `getGrid` reads the caller's own module-scope `grid` fresh on fire, matching the plain `let`
 *  (never a ref) rule every other piece here already follows. */
export function subscribeRowHeight(
  rowHeight: Ref<number>,
  getGrid: () => {
    setOptions: (opts: { rowHeight: number }) => void;
    updateRowCount: () => void;
    render: () => void;
  } | null,
  rootRef: Ref<HTMLElement | null>,
): void {
  watch(rowHeight, (h) => {
    const grid = getGrid();
    if (!grid) return;
    rootRef.value?.style.setProperty('--kira-header-row-height', `${h}px`);
    grid.setOptions({ rowHeight: h });
    grid.updateRowCount();
    grid.render();
  });
}
