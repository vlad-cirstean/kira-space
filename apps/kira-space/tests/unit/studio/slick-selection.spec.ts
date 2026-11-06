import { describe, expect, test } from 'bun:test';
import { SlickRange } from 'slickgrid';
import {
  rangesFromSelection,
  selectionCovers,
  selectionFromRanges,
} from '../../../frontend/src/studio/views/shared/slick/selection';

// selectionFromRanges/rangesFromSelection: the four Selection kinds, the
// ±1 gutter column offset (SlickRange's own cell space includes the frozen gutter at index 0;
// Selection.col is a display column index), the empty case, and the row-mode union (a disjoint
// ctrl/shift row selection produces one SlickRange per row, not one contiguous range). Both
// functions are deliberately row-space-agnostic (page row vs. display position is the caller's
// concern, dataSource.ts's own translation) — every row number below is used as an opaque
// integer, never translated.

describe('selectionFromRanges (P22 Pass B C4) — SlickRange[] -> Selection', () => {
  test('1. empty ranges -> null', () => {
    expect(selectionFromRanges([], false, null)).toBeNull();
  });

  test('2. a single-cell range -> kind "cell", with the gutter offset removed', () => {
    // SlickRange(row, cell): cell 1 is the first *data* column (0 is the gutter).
    const sel = selectionFromRanges([new SlickRange(3, 1)], false, null);
    expect(sel).toEqual({ kind: 'cell', row: 3, col: 0 });
  });

  test('3. a multi-cell range -> kind "range", anchor at the top-left (F14: SlickRange normalises corners)', () => {
    // A "drag from bottom-right to top-left" still produces min/max-normalised fromRow/fromCell —
    // constructing the range with the corners already swapped proves the anchor is always
    // top-left regardless of drag direction (§4.1 item 1's accepted behaviour change).
    const sel = selectionFromRanges([new SlickRange(5, 3, 2, 1)], false, null);
    expect(sel).toEqual({ kind: 'range', anchorRow: 2, anchorCol: 0, row: 5, col: 2 });
  });

  test("4. rowMode unions every range's row span, ascending — a disjoint ctrl-selection is several ranges", () => {
    const ranges = [
      new SlickRange(4, 0, 4, 5),
      new SlickRange(1, 0, 1, 5),
      new SlickRange(7, 0, 7, 5),
    ];
    const sel = selectionFromRanges(ranges, true, null);
    expect(sel).toEqual({ kind: 'row', rows: [1, 4, 7] });
  });

  test('5. rowMode with one contiguous range unions its own row span', () => {
    const sel = selectionFromRanges([new SlickRange(2, 0, 5, 5)], true, null);
    expect(sel).toEqual({ kind: 'row', rows: [2, 3, 4, 5] });
  });

  test('6. pendingKind "column" wins over rowMode, and unions every range\'s column span', () => {
    const ranges = [new SlickRange(0, 2, 9, 2), new SlickRange(0, 4, 9, 5)];
    const sel = selectionFromRanges(ranges, true, 'column');
    expect(sel).toEqual({ kind: 'column', cols: [1, 3, 4] });
  });

  test('7. pendingKind "column" drops a column index that resolves to the gutter itself', () => {
    // Only reachable if something pushed a range starting at cell 0 (the gutter) under a pending
    // column push — defensive, since the header select zone never does this in practice.
    const sel = selectionFromRanges([new SlickRange(0, 0, 9, 1)], false, 'column');
    expect(sel).toEqual({ kind: 'column', cols: [0] });
  });
});

describe('rangesFromSelection (P22 Pass B C4) — Selection -> SlickRange[]', () => {
  test('1. cell -> a single-cell range, gutter offset applied', () => {
    const ranges = rangesFromSelection({ kind: 'cell', row: 3, col: 0 }, 100, 5);
    expect(ranges).toEqual([new SlickRange(3, 1)]);
  });

  test('2. range -> one range spanning anchor to focus, both offset', () => {
    const ranges = rangesFromSelection(
      { kind: 'range', anchorRow: 2, anchorCol: 0, row: 5, col: 2 },
      100,
      5,
    );
    expect(ranges).toEqual([new SlickRange(2, 1, 5, 3)]);
  });

  test('3. row -> one full-width range per row, spanning the gutter through the last column', () => {
    const ranges = rangesFromSelection({ kind: 'row', rows: [1, 4] }, 100, 5);
    expect(ranges).toEqual([new SlickRange(1, 0, 1, 5), new SlickRange(4, 0, 4, 5)]);
  });

  test('4. column, contiguous -> one range spanning the whole page height', () => {
    const ranges = rangesFromSelection({ kind: 'column', cols: [1, 2, 3] }, 10, 5);
    expect(ranges).toEqual([new SlickRange(0, 2, 9, 4)]);
  });

  test('5. column, disjoint -> one range per contiguous run', () => {
    const ranges = rangesFromSelection({ kind: 'column', cols: [0, 1, 3] }, 10, 5);
    expect(ranges).toEqual([new SlickRange(0, 1, 9, 2), new SlickRange(0, 4, 9, 4)]);
  });

  test('6. column selection on a zero-row page still clamps to one row rather than an inverted range', () => {
    const ranges = rangesFromSelection({ kind: 'column', cols: [0] }, 0, 5);
    expect(ranges).toEqual([new SlickRange(0, 1, 0, 1)]);
  });

  test('7. a column selection round-trips through selectionFromRanges with the pendingKind flag', () => {
    const original = { kind: 'column' as const, cols: [0, 2, 3] };
    const ranges = rangesFromSelection(original, 10, 5);
    const roundTripped = selectionFromRanges(ranges, false, 'column');
    expect(roundTripped).toEqual(original);
  });

  test('8. a row selection round-trips through selectionFromRanges in row mode', () => {
    const original = { kind: 'row' as const, rows: [0, 3, 7] };
    const ranges = rangesFromSelection(original, 10, 5);
    const roundTripped = selectionFromRanges(ranges, true, null);
    expect(roundTripped).toEqual(original);
  });
});

// selectionCovers is what SlickGridHost.vue's onCellContextMenu
// now uses to decide whether a right-click acts on the whole current selection (rowMenu(), via
// rowsForSelection) or replaces it with just the clicked cell (cellMenu()). null/undefined and the
// 'cell' kind (never covers anything but its own exact cell, which onCellContextMenu checks via
// `priorSel.kind !== 'cell'` rather than calling this at all) are exercised by the call site, not
// here — these cover the three multi-cell kinds selectionCovers itself branches on.
describe('selectionCovers (P28 D8, promoted for reuse — §4a/§4d)', () => {
  test('1. no selection never covers anything', () => {
    expect(selectionCovers(null, 3, 1)).toBe(false);
    expect(selectionCovers(undefined, 3, 1)).toBe(false);
  });

  test('2. a row selection covers every column of a selected row, no others', () => {
    const sel = { kind: 'row' as const, rows: [2, 5] };
    expect(selectionCovers(sel, 5, 9)).toBe(true); // any column, row 5 is selected
    expect(selectionCovers(sel, 3, 0)).toBe(false); // row 3 is not selected
  });

  test('3. a column selection covers every row of a selected column, no others', () => {
    const sel = { kind: 'column' as const, cols: [1, 3] };
    expect(selectionCovers(sel, 100, 3)).toBe(true); // any row, column 3 is selected
    expect(selectionCovers(sel, 0, 2)).toBe(false); // column 2 is not selected
  });

  test('4. a range selection covers exactly the rectangle between its two corners, inclusive', () => {
    const sel = { kind: 'range' as const, anchorRow: 2, anchorCol: 1, row: 5, col: 3 };
    expect(selectionCovers(sel, 2, 1)).toBe(true); // top-left corner
    expect(selectionCovers(sel, 5, 3)).toBe(true); // bottom-right corner
    expect(selectionCovers(sel, 3, 2)).toBe(true); // interior
    expect(selectionCovers(sel, 1, 2)).toBe(false); // one row above
    expect(selectionCovers(sel, 3, 4)).toBe(false); // one column right
  });

  test('5. the reported "select several rows, right-click Copy does not work" case: a right-click cell inside a multi-row range still covers it', () => {
    // A user drags from (row 1, col 0) to (row 4, col 0) — a 'range' selection spanning several
    // rows in a single column, the shape a cell-to-cell drag produces (not the gutter's own 'row'
    // kind). Right-clicking any cell inside it must read as "covered" so onCellContextMenu opens
    // rowMenu() over the whole span instead of replacing the selection with one cell.
    const sel = { kind: 'range' as const, anchorRow: 1, anchorCol: 0, row: 4, col: 0 };
    for (let row = 1; row <= 4; row++) expect(selectionCovers(sel, row, 0)).toBe(true);
  });
});
