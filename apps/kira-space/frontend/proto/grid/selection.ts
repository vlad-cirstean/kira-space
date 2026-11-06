// Pure selection model for the prototype grid. Cheetah's native model is one range plus a focus
// cell; Studio also needs disjoint row sets, column sets and select-all. Rows are display positions
// (record indices), columns are display columns (grid column minus the gutter).

export type Selection =
  | { kind: 'cell'; row: number; col: number }
  /** Normalised: `anchorRow`/`anchorCol` are the top-left corner, `row`/`col` the bottom-right. */
  | { kind: 'range'; anchorRow: number; anchorCol: number; row: number; col: number }
  /** Ascending, unique. */
  | { kind: 'row'; rows: number[] }
  | { kind: 'column'; cols: number[] }
  | { kind: 'all' };

export interface SelectionState {
  sel: Selection | null;
  /** Origin of the next shift extension: the last plain or toggled click. */
  anchorRow: number;
  anchorCol: number;
}

export interface Modifiers {
  shift: boolean;
  toggle: boolean;
}

export const EMPTY: SelectionState = { sel: null, anchorRow: 0, anchorCol: 0 };

export const EDGE_TOP = 1;
export const EDGE_RIGHT = 2;
export const EDGE_BOTTOM = 4;
export const EDGE_LEFT = 8;

function span(a: number, b: number): number[] {
  const lo = Math.min(a, b);
  const hi = Math.max(a, b);
  return Array.from({ length: hi - lo + 1 }, (_, i) => lo + i);
}

function toggled(list: readonly number[], value: number): number[] {
  const at = list.indexOf(value);
  if (at < 0) return [...list, value].sort((a, b) => a - b);
  return list.filter((_, i) => i !== at);
}

export function selectCell(row: number, col: number): SelectionState {
  return { sel: { kind: 'cell', row, col }, anchorRow: row, anchorCol: col };
}

/** Shift-click or drag from the anchor to `(row, col)`. */
export function extendTo(state: SelectionState, row: number, col: number): SelectionState {
  if (!state.sel) return selectCell(row, col);
  const { anchorRow, anchorCol } = state;
  if (anchorRow === row && anchorCol === col) return { ...selectCell(row, col) };
  return {
    sel: {
      kind: 'range',
      anchorRow: Math.min(anchorRow, row),
      anchorCol: Math.min(anchorCol, col),
      row: Math.max(anchorRow, row),
      col: Math.max(anchorCol, col),
    },
    anchorRow,
    anchorCol,
  };
}

/** Gutter click: plain replaces, shift spans from the anchor row, toggle flips one row in or out. */
export function selectRows(state: SelectionState, row: number, mods: Modifiers): SelectionState {
  const current = state.sel?.kind === 'row' ? state.sel.rows : [];
  if (mods.toggle) {
    const rows = toggled(current, row);
    return {
      sel: rows.length > 0 ? { kind: 'row', rows } : null,
      anchorRow: row,
      anchorCol: state.anchorCol,
    };
  }
  if (mods.shift && state.sel) {
    return {
      sel: { kind: 'row', rows: span(state.anchorRow, row) },
      anchorRow: state.anchorRow,
      anchorCol: state.anchorCol,
    };
  }
  return { sel: { kind: 'row', rows: [row] }, anchorRow: row, anchorCol: state.anchorCol };
}

/** Gutter drag: every row between the press row and the row under the pointer. */
export function dragRows(state: SelectionState, from: number, to: number): SelectionState {
  return {
    sel: { kind: 'row', rows: span(from, to) },
    anchorRow: from,
    anchorCol: state.anchorCol,
  };
}

/** Header select zone: same rules as rows, on columns. */
export function selectColumns(state: SelectionState, col: number, mods: Modifiers): SelectionState {
  const current = state.sel?.kind === 'column' ? state.sel.cols : [];
  if (mods.toggle) {
    const cols = toggled(current, col);
    return {
      sel: cols.length > 0 ? { kind: 'column', cols } : null,
      anchorRow: state.anchorRow,
      anchorCol: col,
    };
  }
  if (mods.shift && state.sel) {
    return {
      sel: { kind: 'column', cols: span(state.anchorCol, col) },
      anchorRow: state.anchorRow,
      anchorCol: state.anchorCol,
    };
  }
  return { sel: { kind: 'column', cols: [col] }, anchorRow: state.anchorRow, anchorCol: col };
}

export function selectAll(): SelectionState {
  return { sel: { kind: 'all' }, anchorRow: 0, anchorCol: 0 };
}

function has(sorted: readonly number[], value: number): boolean {
  let lo = 0;
  let hi = sorted.length - 1;
  while (lo <= hi) {
    const mid = (lo + hi) >>> 1;
    const v = sorted[mid] as number;
    if (v === value) return true;
    if (v < value) lo = mid + 1;
    else hi = mid - 1;
  }
  return false;
}

export function rowSelected(sel: Selection | null, row: number): boolean {
  return sel?.kind === 'all' || (sel?.kind === 'row' && has(sel.rows, row));
}

export function covers(sel: Selection | null, row: number, col: number): boolean {
  if (!sel || row < 0 || col < 0) return false;
  switch (sel.kind) {
    case 'cell':
      return sel.row === row && sel.col === col;
    case 'range':
      return row >= sel.anchorRow && row <= sel.row && col >= sel.anchorCol && col <= sel.col;
    case 'row':
      return has(sel.rows, row);
    case 'column':
      return has(sel.cols, col);
    case 'all':
      return true;
  }
}

/** Sides of `(row, col)` that sit on the selection's outer perimeter, as an `EDGE_*` bitmask. */
export function edgesOf(sel: Selection | null, row: number, col: number): number {
  if (!covers(sel, row, col)) return 0;
  let mask = 0;
  if (!covers(sel, row - 1, col)) mask |= EDGE_TOP;
  if (!covers(sel, row, col + 1)) mask |= EDGE_RIGHT;
  if (!covers(sel, row + 1, col)) mask |= EDGE_BOTTOM;
  if (!covers(sel, row, col - 1)) mask |= EDGE_LEFT;
  return mask;
}

export function selectedRows(sel: Selection | null, rowCount: number): number[] {
  if (!sel) return [];
  switch (sel.kind) {
    case 'cell':
      return [sel.row];
    case 'range':
      return span(sel.anchorRow, sel.row);
    case 'row':
      return sel.rows;
    case 'column':
    case 'all':
      return span(0, rowCount - 1);
  }
}

export function selectedCols(sel: Selection | null, colCount: number): number[] {
  if (!sel) return [];
  switch (sel.kind) {
    case 'cell':
      return [sel.col];
    case 'range':
      return span(sel.anchorCol, sel.col);
    case 'column':
      return sel.cols;
    case 'row':
    case 'all':
      return span(0, colCount - 1);
  }
}
