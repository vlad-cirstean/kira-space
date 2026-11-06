import type { ProtoState } from './state';

export type SortDir = 'asc' | 'desc';

/** Header arrow click: asc, then desc, then off. Shift adds this column as a further term and
 *  keeps the others; a plain click leaves it the only term. */
export function cycleSort(state: ProtoState, pageCol: number, additive: boolean): void {
  const current = state.sorts.find((term) => term.col === pageCol)?.dir;
  const next: SortDir | null = current === undefined ? 'asc' : current === 'asc' ? 'desc' : null;
  setSort(state, pageCol, next, additive);
}

/** Sets one column's direction (null clears it). Without `additive`, other terms are dropped. */
export function setSort(
  state: ProtoState,
  pageCol: number,
  dir: SortDir | null,
  additive: boolean,
): void {
  const others = additive ? state.sorts.filter((term) => term.col !== pageCol) : [];
  const existing = state.sorts.findIndex((term) => term.col === pageCol);
  if (dir === null) {
    state.sorts = additive ? others : [];
    return;
  }
  if (additive && existing >= 0) {
    state.sorts = state.sorts.map((term) => (term.col === pageCol ? { col: pageCol, dir } : term));
    return;
  }
  state.sorts = [...others, { col: pageCol, dir }];
}
