// rowValues.ts pulls in grid/menu.ts (FK nav items) -> state/tabs.ts and pendingChanges.ts ->
// bridge/data.ts -> bridge/port.ts, which reaches '/wails/runtime.js' at module scope — the same
// dynamic-import/window-mock need grid-commit-composite-pk-guard.spec.ts's own comment documents.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import type { Selection } from '../../../frontend/src/studio/views/shared/slick/selection';

const { rowsForSelection } = await import('../../../frontend/src/studio/views/grid/slick/rowValues');

// Real-interaction fix (reported bug family — right-click Copy on a multi-row selection was a
// no-op, Delete did nothing for a column or whole-table selection, and the keyboard shortcut and
// the right-click menu could each reach a different answer for the same selection): every
// SlickGridHost.vue/DataToolbar.vue call site that turns "the current selection" into "the rows
// to act on" now goes through this one function. These are the cases that used to diverge across
// those three hand-rolled switches — most importantly, `kind: 'column'` (never handled anywhere)
// and the filter-aware `range`/`column` cases.

describe('rowsForSelection (grid/slick/rowValues.ts)', () => {
  test('1. a row selection returns its own rows, sorted ascending', () => {
    const sel: Selection = { kind: 'row', rows: [5, 1, 3] };
    expect(rowsForSelection(sel, null, 10)).toEqual([1, 3, 5]);
  });

  test('2. a cell selection returns just that one row', () => {
    const sel: Selection = { kind: 'cell', row: 4, col: 2 };
    expect(rowsForSelection(sel, null, 10)).toEqual([4]);
  });

  test('3. a range selection returns every row spanned, unfiltered', () => {
    const sel: Selection = { kind: 'range', anchorRow: 2, anchorCol: 0, row: 5, col: 3 };
    expect(rowsForSelection(sel, null, 10)).toEqual([2, 3, 4, 5]);
  });

  test('4. a range selection under an active filter only returns the rows still visible in it (Finding 3 round 2)', () => {
    const sel: Selection = { kind: 'range', anchorRow: 0, anchorCol: 0, row: 9, col: 1 };
    const displayRows = [1, 2, 5, 8]; // rows 0,3,4,6,7,9 are filtered out
    expect(rowsForSelection(sel, displayRows, 10)).toEqual([1, 2, 5, 8]);
  });

  test('5. a column selection returns every currently-displayed row (never handled before this fix) — unfiltered', () => {
    const sel: Selection = { kind: 'column', cols: [1] };
    expect(rowsForSelection(sel, null, 6)).toEqual([0, 1, 2, 3, 4, 5]);
  });

  test('6. a column selection under an active filter returns only the visible rows, like Copy already does', () => {
    const sel: Selection = { kind: 'column', cols: [1] };
    const displayRows = [0, 2, 4];
    expect(rowsForSelection(sel, displayRows, 6)).toEqual([0, 2, 4]);
  });

  test('7. "select entire table" (a range spanning every row/column) returns every row — the reported "delete does not work" case', () => {
    const sel: Selection = { kind: 'range', anchorRow: 0, anchorCol: 0, row: 2, col: 4 };
    expect(rowsForSelection(sel, null, 3)).toEqual([0, 1, 2]);
  });
});
