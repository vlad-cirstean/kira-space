import { type ProtoState, recordOf, stageKey } from './state';

/** Case-insensitive substring search over every page cell. Studio's find runs server-side; the
 *  prototype scans the decoded page, which is enough to exercise the highlight layers. */
export function runSearch(state: ProtoState, term: string): void {
  const needle = term.trim().toLowerCase();
  const list: number[] = [];
  if (needle) {
    const cols = state.data.columns.length;
    for (let row = 0; row < state.data.rowCount; row++) {
      for (let col = 0; col < cols; col++) {
        const view = state.data.viewAt(row, col);
        if (!view.isNull && view.text.toLowerCase().includes(needle)) list.push(stageKey(row, col));
      }
    }
  }
  state.search = { term, cells: new Set(list), list, current: -1 };
}

export type SearchMark = 'none' | 'match' | 'current';

export function markOf(state: ProtoState, pageRow: number, pageCol: number): SearchMark {
  const { cells, list, current } = state.search;
  if (cells.size === 0) return 'none';
  const key = stageKey(pageRow, pageCol);
  if (!cells.has(key)) return 'none';
  return list[current] === key ? 'current' : 'match';
}

/** Rows the search leaves visible when non-matching rows are hidden. */
export function matchRows(state: ProtoState): Uint32Array {
  const rows = new Set<number>();
  for (const key of state.search.list) rows.add(Math.floor(key / 256));
  return Uint32Array.from([...rows].sort((a, b) => a - b));
}

export interface MatchTarget {
  record: number;
  displayCol: number;
}

/** Moves `current` by `step` (wrapping) and resolves its grid position. Null when nothing
 *  matches or the cell's column is hidden. */
export function stepMatch(state: ProtoState, step: 1 | -1): MatchTarget | null {
  const { list } = state.search;
  if (list.length === 0) return null;
  const next = (state.search.current + step + list.length) % list.length;
  state.search.current = next;
  const key = list[next] as number;
  const record = recordOf(state, Math.floor(key / 256));
  const displayCol = state.order.indexOf(key % 256);
  return record < 0 || displayCol < 0 ? null : { record, displayCol };
}
