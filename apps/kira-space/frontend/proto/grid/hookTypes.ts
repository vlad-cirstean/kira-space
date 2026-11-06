// Shape of `window.__kiraGridProto`, shared by the debug hook and the Playwright helper
// (apps/kira-studio/tests/proto/support/grid.ts). Pure types: no DOM, Vue or cheetah imports.

interface ViewportRect {
  x: number;
  y: number;
  width: number;
  height: number;
}

interface HookSelection {
  kind: 'cell' | 'range' | 'row' | 'column' | 'all' | null;
  /** Page rows, ascending. */
  rows: number[];
  /** Column names in display order. */
  columns: string[];
  /** Active cell: page row and column name. */
  active: { row: number; column: string } | null;
}

interface HookCellState {
  text: string;
  isNull: boolean;
  truncated: boolean;
  masked: boolean;
  staged: boolean;
  search: 'none' | 'match' | 'current';
  category: string;
  align: 'left' | 'right';
  nav: 'fk' | 'pk' | null;
  /** Viewport rect of the nav glyph's hit area; null without a nav affordance. */
  navRect: ViewportRect | null;
}

interface HookEditor {
  open: boolean;
  pageRow: number | null;
  column: string | null;
  value: string | null;
  /** Reason of the last refused edit attempt. */
  vetoReason: string | null;
}

interface HookHeader {
  sort: 'asc' | 'desc' | null;
  /** 1-based position among the sort terms. */
  sortOrder: number | null;
  key: 'PK' | 'FK' | null;
}

interface HookRow {
  inserted: boolean;
  deleted: boolean;
  dirty: boolean;
  hovered: boolean;
  gutterLabel: string;
}

export interface KiraGridProtoHook {
  /** Record index of a page row, or null when sort or hide filtered it out. */
  recordIndex(pageRow: number): number | null;
  pageRow(recordIndex: number): number;
  cellState(pageRow: number, column: string): HookCellState;
  selection(): HookSelection;
  editor(): HookEditor;
  header(column: string): HookHeader;
  rowState(pageRow: number): HookRow;
  insertRowCount(): number;
  /** Grid column index of a column name (the gutter is 0), or null when hidden. */
  gridCol(column: string): number | null;
}

declare global {
  interface Window {
    __kiraGridProto?: KiraGridProtoHook;
  }
}
