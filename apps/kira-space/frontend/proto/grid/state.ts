import type { ProtoCellView, ProtoData, ProtoParams } from './data';
import { EMPTY, type SelectionState } from './selection';
import type { Palette } from './theme';

export const HEADER_ROWS = 1;

/** Cell view plus the staged flag the edit layer adds. */
export type ProtoView = ProtoCellView & { staged?: boolean };

interface SearchState {
  term: string;
  /** Matching cells as `stageKey(pageRow, pageCol)`, ascending. */
  cells: Set<number>;
  list: number[];
  /** Index into `list`, or -1. */
  current: number;
}

interface SortTerm {
  /** Page column. */
  col: number;
  dir: 'asc' | 'desc';
}

interface EditingCell {
  pageRow: number;
  /** Page column. */
  col: number;
  original: string;
}

/** Mutable render state shared by the draw path, the host and the debug hook. Deliberately not
 *  reactive (a ref here would proxy every read inside the per-cell draw loop). */
export interface ProtoState {
  data: ProtoData;
  params: ProtoParams;
  palette: Palette;
  /** Record index to page row for the base rows. Null while the record order is the page order. */
  rowOrder: Uint32Array | null;
  /** Display column to page column index (hide and reorder rebuild it). */
  order: number[];
  selection: SelectionState;
  /** Page row under the pointer, or -1. */
  hoverRow: number;
  /** First page row's number minus one, as the gutter shows it. */
  rowNumberBase: number;
  /** Staged edits on existing rows, by `stageKey`. Null stages a NULL. */
  staged: Map<number, string | null>;
  dirty: Set<number>;
  deleted: Set<number>;
  /** Insert rows appended after the base rows: page column to staged text. */
  inserts: Map<number, string | null>[];
  search: SearchState;
  sorts: SortTerm[];
  editing: EditingCell | null;
  vetoReason: string | null;
  /** Page column under the pointer in the header, or -1. */
  hoverHeader: number;
  /** Host callbacks: repaint one page row, rebuild the grid's data source. */
  invalidateRow: (pageRow: number) => void;
  refreshSource: () => void;
  /** Display view of a cell. Folds staged and insert values in. */
  viewAt(pageRow: number, col: number): ProtoView;
  /** Records on display: the base rows plus insert rows. */
  rowCount(): number;
  baseCount(): number;
}

const INSERT_EMPTY: ProtoView = Object.freeze({ text: '', isNull: false, truncated: false });
const STAGED_NULL: ProtoView = Object.freeze({
  text: '',
  isNull: true,
  truncated: false,
  staged: true,
});

export const stageKey = (pageRow: number, pageCol: number): number => pageRow * 256 + pageCol;

export function createState(data: ProtoData, params: ProtoParams, palette: Palette): ProtoState {
  const state: ProtoState = {
    data,
    params,
    palette,
    rowOrder: null,
    order: data.columns.map((_, index) => index),
    selection: EMPTY,
    hoverRow: -1,
    rowNumberBase: 0,
    staged: new Map(),
    dirty: new Set(),
    deleted: new Set(),
    inserts: [],
    search: { term: '', cells: new Set(), list: [], current: -1 },
    sorts: [],
    editing: null,
    vetoReason: null,
    hoverHeader: -1,
    invalidateRow: () => {},
    refreshSource: () => {},
    viewAt(pageRow, col) {
      if (pageRow >= data.rowCount) {
        const value = state.inserts[pageRow - data.rowCount]?.get(col);
        if (value === undefined) return INSERT_EMPTY;
        return value === null
          ? STAGED_NULL
          : { text: value, isNull: false, truncated: false, staged: true };
      }
      if (state.staged.size > 0) {
        const value = state.staged.get(stageKey(pageRow, col));
        if (value === null) return STAGED_NULL;
        if (value !== undefined)
          return { text: value, isNull: false, truncated: false, staged: true };
      }
      return data.viewAt(pageRow, col);
    },
    baseCount: () => state.rowOrder?.length ?? data.rowCount,
    rowCount: () => state.baseCount() + state.inserts.length,
  };
  return state;
}

export function pageRowOf(state: ProtoState, recordIndex: number): number {
  const base = state.baseCount();
  if (recordIndex >= base) return state.data.rowCount + (recordIndex - base);
  return state.rowOrder ? (state.rowOrder[recordIndex] as number) : recordIndex;
}

/** Record index of a page row, or -1 when sort or hide filtered it out. */
export function recordOf(state: ProtoState, pageRow: number): number {
  if (pageRow >= state.data.rowCount) return state.baseCount() + (pageRow - state.data.rowCount);
  if (!state.rowOrder) return pageRow;
  return state.rowOrder.indexOf(pageRow);
}
