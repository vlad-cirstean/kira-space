import { describe, expect, test } from 'bun:test';
import {
  covers,
  dragRows,
  EDGE_BOTTOM,
  EDGE_LEFT,
  EDGE_RIGHT,
  EDGE_TOP,
  EMPTY,
  edgesOf,
  extendTo,
  selectAll,
  selectCell,
  selectColumns,
  selectedCols,
  selectedRows,
  selectRows,
} from '../../../frontend/proto/grid/selection';

const plain = { shift: false, toggle: false };

describe('proto grid selection model', () => {
  test('gutter click, shift range and ctrl toggle accumulate like SlickHybridSelectionModel', () => {
    let s = selectRows(EMPTY, 0, plain);
    s = selectRows(s, 2, { shift: true, toggle: false });
    expect(s.sel).toEqual({ kind: 'row', rows: [0, 1, 2] });
    s = selectRows(s, 0, plain);
    expect(s.sel).toEqual({ kind: 'row', rows: [0] });
    s = selectRows(s, 2, { shift: false, toggle: true });
    expect(s.sel).toEqual({ kind: 'row', rows: [0, 2] });
    s = selectRows(s, 0, { shift: false, toggle: true });
    expect(s.sel).toEqual({ kind: 'row', rows: [2] });
    s = selectRows(s, 2, { shift: false, toggle: true });
    expect(s.sel).toBeNull();
  });

  test('a shift range after a toggle spans from the toggled row, replacing the disjoint set', () => {
    let s = selectRows(EMPTY, 5, plain);
    s = selectRows(s, 9, { shift: false, toggle: true });
    s = selectRows(s, 7, { shift: true, toggle: false });
    expect(s.sel).toEqual({ kind: 'row', rows: [7, 8, 9] });
  });

  test('gutter drag spans either direction', () => {
    expect(dragRows(EMPTY, 3, 1).sel).toEqual({ kind: 'row', rows: [1, 2, 3] });
  });

  test('cell then shift-extend normalises corners and keeps the anchor', () => {
    let s = selectCell(4, 3);
    s = extendTo(s, 2, 5);
    expect(s.sel).toEqual({ kind: 'range', anchorRow: 2, anchorCol: 3, row: 4, col: 5 });
    s = extendTo(s, 4, 3);
    expect(s.sel).toEqual({ kind: 'cell', row: 4, col: 3 });
  });

  test('column select follows the same shift and toggle rules', () => {
    let s = selectColumns(EMPTY, 2, plain);
    s = selectColumns(s, 4, { shift: true, toggle: false });
    expect(s.sel).toEqual({ kind: 'column', cols: [2, 3, 4] });
    s = selectColumns(s, 6, { shift: false, toggle: true });
    expect(s.sel).toEqual({ kind: 'column', cols: [2, 3, 4, 6] });
  });

  test('edges mark only the outer perimeter of a disjoint selection', () => {
    const { sel } = selectRows(selectRows(EMPTY, 0, plain), 2, { shift: false, toggle: true });
    expect(covers(sel, 1, 0)).toBe(false);
    expect(edgesOf(sel, 0, 0)).toBe(EDGE_TOP | EDGE_BOTTOM | EDGE_LEFT);
    expect(edgesOf(sel, 0, 3)).toBe(EDGE_TOP | EDGE_BOTTOM);
    const range = extendTo(selectCell(1, 1), 3, 3).sel;
    expect(edgesOf(range, 2, 2)).toBe(0);
    expect(edgesOf(range, 1, 3)).toBe(EDGE_TOP | EDGE_RIGHT);
    expect(edgesOf(range, 3, 1)).toBe(EDGE_BOTTOM | EDGE_LEFT);
  });

  test('row and column extents for menus', () => {
    expect(selectedRows(selectAll().sel, 4)).toEqual([0, 1, 2, 3]);
    expect(selectedCols(selectRows(EMPTY, 1, plain).sel, 3)).toEqual([0, 1, 2]);
    expect(selectedRows(selectColumns(EMPTY, 1, plain).sel, 3)).toEqual([0, 1, 2]);
  });
});
