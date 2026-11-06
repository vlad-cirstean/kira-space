import { type ProtoState, stageKey } from './state';

/** Why an existing or new cell refuses an edit, or null when it takes one. The same rules as
 *  SlickGridHost's `onBeforeEditCell`: read-only table, deleted row, generated column, truncated
 *  value; plus a masked cell, whose real value is not on screen to edit. */
export function vetoReason(state: ProtoState, pageRow: number, pageCol: number): string | null {
  if (state.params.readOnly) return 'read-only';
  if (pageRow >= state.data.rowCount) {
    return state.data.columns[pageCol]?.generated ? 'generated' : null;
  }
  if (state.deleted.has(pageRow)) return 'deleted';
  if (state.data.columns[pageCol]?.generated) return 'generated';
  const view = state.data.viewAt(pageRow, pageCol);
  if (view.truncated) return 'truncated';
  if (view.masked) return 'masked';
  return null;
}

/** The one write path: the grid's own editor, paste and range delete all land here through the
 *  record's field setter. A refused cell records its reason and stays untouched. */
export function stageValue(
  state: ProtoState,
  pageRow: number,
  pageCol: number,
  value: string | null,
): boolean {
  const reason = vetoReason(state, pageRow, pageCol);
  if (reason) {
    state.vetoReason = reason;
    return false;
  }
  if (pageRow >= state.data.rowCount) {
    const insert = state.inserts[pageRow - state.data.rowCount];
    if (!insert) return false;
    insert.set(pageCol, value);
  } else {
    const current = state.data.viewAt(pageRow, pageCol);
    const before = current.isNull ? null : current.text;
    const key = stageKey(pageRow, pageCol);
    if (value === before || (value === '' && before === null)) {
      state.staged.delete(key);
    } else {
      state.staged.set(key, value);
      state.dirty.add(pageRow);
    }
  }
  state.invalidateRow(pageRow);
  return true;
}

export function deleteRows(state: ProtoState, pageRows: readonly number[]): void {
  for (const row of pageRows) {
    if (row >= state.data.rowCount || state.params.readOnly) continue;
    state.deleted.add(row);
    state.invalidateRow(row);
  }
}

export function addInsertRow(state: ProtoState): number {
  state.inserts.push(new Map());
  state.refreshSource();
  return state.rowCount() - 1;
}

export type RowRail = 'inserted' | 'deleted' | 'dirty' | null;

export function railOf(state: ProtoState, pageRow: number): RowRail {
  if (pageRow >= state.data.rowCount) return 'inserted';
  if (state.deleted.has(pageRow)) return 'deleted';
  return state.dirty.has(pageRow) ? 'dirty' : null;
}
