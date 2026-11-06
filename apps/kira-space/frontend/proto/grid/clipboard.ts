import { copyText } from '@workbench/behaviour/clipboard';
import {
  columnsToTsv,
  type RowSnapshot,
  rowsToTsv,
  tsvField,
} from '../../src/views/shared/clipboardFormats';
import { selectedCols, selectedRows } from './selection';
import { type ProtoState, pageRowOf } from './state';

export interface CellText {
  text: string;
  isNull: boolean;
}

/** One displayed cell, addressed by record index and display column. */
export function cellAt(state: ProtoState, record: number, displayCol: number): CellText {
  const view = state.viewAt(pageRowOf(state, record), state.order[displayCol] as number);
  return { text: view.text, isNull: view.isNull };
}

export function columnName(state: ProtoState, displayCol: number): string {
  return state.data.columns[state.order[displayCol] as number]?.name ?? '';
}

export function rowSnapshot(state: ProtoState, record: number): RowSnapshot {
  const columns = state.order.map((_, i) => columnName(state, i));
  const values: Record<string, string | null> = {};
  columns.forEach((name, i) => {
    const cell = cellAt(state, record, i);
    values[name] = cell.isNull ? null : cell.text;
  });
  return { columns, values };
}

/** What Cmd/Ctrl+C puts on the clipboard for the current selection: the same displayed text a
 *  row, range or column copy produces everywhere else in the app. */
export function selectionText(state: ProtoState): string {
  const { sel } = state.selection;
  if (!sel) return '';
  const at = (row: number, col: number): CellText => cellAt(state, row, col);
  if (sel.kind === 'cell') {
    const cell = at(sel.row, sel.col);
    return cell.isNull ? '' : tsvField(cell.text);
  }
  if (sel.kind === 'row') return rowsToTsv(sel.rows.map((r) => rowSnapshot(state, r)));
  const rows = selectedRows(sel, state.rowCount());
  return columnsToTsv(rows, selectedCols(sel, state.order.length), at);
}

export function copySelection(state: ProtoState): Promise<void> {
  return copyText(selectionText(state));
}

export { copyText };
