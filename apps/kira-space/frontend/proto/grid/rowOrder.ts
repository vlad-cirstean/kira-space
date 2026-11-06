import { matchRows } from './search';
import type { ProtoState } from './state';

function compare(state: ProtoState, a: number, b: number): number {
  for (const { col, dir } of state.sorts) {
    const left = state.data.viewAt(a, col);
    const right = state.data.viewAt(b, col);
    let result = 0;
    if (left.isNull !== right.isNull) {
      result = left.isNull ? -1 : 1;
    } else if (!left.isNull) {
      const numeric = state.data.columns[col]?.typeClass === 'number';
      if (numeric) result = Number(left.text) - Number(right.text);
      else result = left.text < right.text ? -1 : left.text > right.text ? 1 : 0;
    }
    if (result !== 0) return dir === 'asc' ? result : -result;
  }
  return 0;
}

/** Client-side stand-in for Studio's re-query: the base rows after hide-non-matching and sort.
 *  Null when neither applies, so the page order stays free of an index array. */
export function computeRowOrder(state: ProtoState, hideNonMatching: boolean): Uint32Array | null {
  const hide = hideNonMatching && state.search.list.length > 0;
  if (!hide && state.sorts.length === 0) return null;
  const rows = hide
    ? matchRows(state)
    : Uint32Array.from({ length: state.data.rowCount }, (_, i) => i);
  if (state.sorts.length > 0) rows.sort((a, b) => compare(state, a, b));
  return rows;
}
