import { SlickRange } from 'slickgrid';

// P19 D7: moved from views/grid/state.ts — this is the file that already owns the geometry this
// type describes, and views/console/ needs it too (biome.json forbids views/console/** importing
// views/grid/**). views/grid/state.ts re-exports it so nothing there had to change its import.
export type Selection =
  | { kind: 'cell'; row: number; col: number }
  | { kind: 'range'; anchorRow: number; anchorCol: number; row: number; col: number }
  | { kind: 'row'; rows: number[] }
  | { kind: 'column'; cols: number[] };

// Pure functions, unit-testable without a DOM. `SlickHybridSelectionModel`
// owns the geometry (`SlickRange[]`, always in *display-position* space — SlickGrid indexes the
// `CustomDataView` it was handed, never a page row); `Selection` (state.ts) carries the
// `kind` copy/the three menus/the cell-editor publish/Delete all branch on, and its own row
// fields are *page rows*. Neither side can be the sole authority — SlickGridHost.vue translates
// between the two spaces via dataSource.ts's own `rowAtDisplayPosition`/`displayPositionOf`
// (identity today); these functions themselves are
// deliberately position/page-row agnostic, just the structural `SlickRange[]` <-> `Selection`
// shape mapping.
//
// `Selection.col`/`.anchorCol` are *display* column indices; a `SlickRange`'s `cell` fields are
// column *array* indices that include the gutter at 0 (`frozenColumn: 0`). The offset is exactly
// `+1`, named once here in both directions.
const GUTTER_OFFSET = 1;

/** Does the current selection already cover this cell? The gutter menu has always asked this
 *  (onGutterContextMenu's own `inSelection`). Exported here (rather than kept as a private helper
 *  inside the host) so both SlickGridHost.vue's onCellContextMenu (deciding whether a right-click
 *  replaces the selection or acts on it) and its own unit tests can share one copy — the same "pure
 *  Selection logic belongs in this file" rule `selectionFromRanges`/
 *  `rangesFromSelection` above already follow. */
export function selectionCovers(
  sel: Selection | null | undefined,
  row: number,
  displayCol: number,
): boolean {
  if (!sel) return false;
  switch (sel.kind) {
    case 'row':
      return sel.rows.includes(row);
    case 'column':
      return sel.cols.includes(displayCol);
    case 'range':
      return (
        row >= sel.anchorRow &&
        row <= sel.row &&
        displayCol >= sel.anchorCol &&
        displayCol <= sel.col
      );
    default:
      return false;
  }
}

/**
 * `SlickRange[]` -> `Selection`, per §5 D4:
 * - `pendingKind === 'column'` -> `{ kind: 'column', cols }` — set by the header select-zone
 *   handler immediately before it pushes ranges into the model, consumed once (the same one-shot-
 *   flag shape `dragProducedRange` used in the incumbent). `cols` is the union of every range's
 *   own column span, since row-mode-style ctrl/shift accumulation can produce more than one range
 *   for a disjoint multi-column selection.
 * - `rowMode` (`selectionModel.currentSelectionModeIsRow()`) -> `{ kind: 'row', rows: ascending
 *   union }` — `SlickHybridSelectionModel`'s own `rowsToRanges` builds one range *per row* for a
 *   disjoint ctrl/shift selection, so this always unions rather than assuming one contiguous range.
 * - one range, `isSingleCell()` -> `{ kind: 'cell', row, col }`.
 * - one range, multi-cell -> `{ kind: 'range', anchorRow: fromRow, anchorCol: fromCell, row: toRow,
 *   col: toCell }` (F14: `SlickRange` normalises its corners to top-left/bottom-right, so the
 *   anchor here is always the top-left corner regardless of drag direction — §4.1 item 1's
 *   accepted behaviour change, not a bug).
 * - empty -> `null`.
 */
export function selectionFromRanges(
  ranges: readonly SlickRange[],
  rowMode: boolean,
  pendingKind: 'column' | null,
): Selection | null {
  if (ranges.length === 0) return null;

  if (pendingKind === 'column') {
    const cols = new Set<number>();
    for (const r of ranges) {
      for (let c = r.fromCell; c <= r.toCell; c++) {
        const col = c - GUTTER_OFFSET;
        if (col >= 0) cols.add(col);
      }
    }
    return { kind: 'column', cols: [...cols].sort((a, b) => a - b) };
  }

  if (rowMode) {
    const rows = new Set<number>();
    for (const r of ranges) {
      for (let row = r.fromRow; row <= r.toRow; row++) rows.add(row);
    }
    return { kind: 'row', rows: [...rows].sort((a, b) => a - b) };
  }

  // enableMultiSelection: false (§4.1 item 4) — cell mode never produces more than one range, so
  // only the first is ever meaningful.
  const range = ranges[0];
  if (!range) return null;
  if (range.isSingleCell()) {
    return { kind: 'cell', row: range.fromRow, col: range.fromCell - GUTTER_OFFSET };
  }
  return {
    kind: 'range',
    anchorRow: range.fromRow,
    anchorCol: range.fromCell - GUTTER_OFFSET,
    row: range.toRow,
    col: range.toCell - GUTTER_OFFSET,
  };
}

/**
 * The reverse — used when app code SETS the selection (the header select zone, the row/cell
 * context menus' "replace the selection first" rule, `scrollCellIntoView` from search/FK-nav).
 * `rowCount`/`colCount` are display-row-count/display-column-count (never counting the gutter),
 * in whatever row space the caller's own `sel` is already in (page row today; display position
 * once the caller translates first) — this function does no row-space translation of its own,
 * matching `selectionFromRanges`'s own position/page-row-agnostic contract.
 */
export function rangesFromSelection(
  sel: Selection,
  rowCount: number,
  colCount: number,
): SlickRange[] {
  const lastCell = colCount; // colCount display columns + the gutter at index 0.
  switch (sel.kind) {
    case 'cell':
      return [new SlickRange(sel.row, sel.col + GUTTER_OFFSET)];
    case 'range':
      return [
        new SlickRange(
          sel.anchorRow,
          sel.anchorCol + GUTTER_OFFSET,
          sel.row,
          sel.col + GUTTER_OFFSET,
        ),
      ];
    case 'row':
      return sel.rows.map((row) => new SlickRange(row, 0, row, lastCell));
    case 'column': {
      // Grouped into contiguous runs (one range per run) rather than one range per column —
      // fewer ranges for the common contiguous-drag case, and selectionFromRanges' own
      // pendingKind==='column' branch unions every range's span regardless, so a disjoint
      // (ctrl-accumulated) selection round-trips correctly either way.
      const cols = [...sel.cols].sort((a, b) => a - b);
      const ranges: SlickRange[] = [];
      let i = 0;
      while (i < cols.length) {
        let j = i;
        while (j + 1 < cols.length && cols[j + 1] === (cols[j] as number) + 1) j++;
        ranges.push(
          new SlickRange(
            0,
            (cols[i] as number) + GUTTER_OFFSET,
            Math.max(0, rowCount - 1),
            (cols[j] as number) + GUTTER_OFFSET,
          ),
        );
        i = j + 1;
      }
      return ranges;
    }
    default:
      return [];
  }
}
