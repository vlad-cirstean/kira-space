import type { MenuItem } from '@workbench/state/contextMenu';
import {
  columnsToTsv,
  type RowSnapshot,
  rowsToCsv,
  rowsToJson,
  rowsToTsv,
} from '../../src/views/shared/clipboardFormats';
import { type CellText, cellAt, columnName, copyText, rowSnapshot } from './clipboard';
import { vetoReason } from './pending';
import { selectedCols, selectedRows } from './selection';
import type { SortDir } from './sort';
import { type ProtoState, pageRowOf } from './state';

/** Callbacks the menus need from the host; everything else reads `state`. */
export interface MenuActions {
  state: ProtoState;
  hideColumn(displayCol: number): void;
  showAllColumns(): void;
  edit(record: number, displayCol: number): void;
  setNull(record: number, displayCol: number): void;
  paste(record: number, displayCol: number): Promise<void>;
  deleteRows(records: readonly number[]): void;
  insertRow(): void;
  sort(displayCol: number, dir: SortDir | null): void;
}

function copyItem(id: string, label: string, text: () => string): MenuItem {
  return { type: 'item', id, label, icon: 'copy', run: () => copyText(text()) };
}

export function cellMenu(actions: MenuActions, row: number, col: number): MenuItem[] {
  const { state } = actions;
  const name = columnName(state, col);
  const vetoed = vetoReason(state, pageRowOf(state, row), state.order[col] as number) !== null;
  const cell = (): CellText => cellAt(state, row, col);
  const text = (): string => (cell().isNull ? '' : cell().text);
  return [
    copyItem('copy', 'Copy', text),
    copyItem('copy-with-header', 'Copy with header', () => `${name}\n${text()}`),
    copyItem('copy-as-json', 'Copy as JSON', () =>
      JSON.stringify(cell().isNull ? null : cell().text),
    ),
    {
      type: 'item',
      id: 'paste',
      label: 'Paste',
      icon: 'clippy',
      disabled: vetoed,
      run: () => actions.paste(row, col),
    },
    { type: 'separator' },
    {
      type: 'item',
      id: 'edit',
      label: 'Edit',
      icon: 'edit',
      disabled: vetoed,
      run: () => actions.edit(row, col),
    },
    {
      type: 'item',
      id: 'set-null',
      label: 'Set NULL',
      disabled: vetoed,
      run: () => actions.setNull(row, col),
    },
    { type: 'item', id: 'insert-row', label: 'Insert row', run: () => actions.insertRow() },
    {
      type: 'item',
      id: 'delete-row',
      label: 'Delete row',
      icon: 'trash',
      danger: true,
      run: () => actions.deleteRows([row]),
    },
  ];
}

export function rangeMenu({ state }: MenuActions): MenuItem[] {
  const { sel } = state.selection;
  const rows = selectedRows(sel, state.rowCount());
  const cols = selectedCols(sel, state.order.length);
  const snapshots = (): RowSnapshot[] =>
    rows.map((row) => {
      const names = cols.map((c) => columnName(state, c));
      const values: Record<string, string | null> = {};
      cols.forEach((c, i) => {
        const cell = cellAt(state, row, c);
        values[names[i] as string] = cell.isNull ? null : cell.text;
      });
      return { columns: names, values };
    });
  return [
    copyItem('copy', 'Copy', () => columnsToTsv(rows, cols, (r, c) => cellAt(state, r, c))),
    copyItem('copy-as-csv', 'Copy as CSV', () => rowsToCsv(snapshots())),
    copyItem('copy-as-json', 'Copy as JSON', () => rowsToJson(snapshots())),
  ];
}

export function rowMenu(actions: MenuActions, rows: readonly number[]): MenuItem[] {
  const { state } = actions;
  let cached: RowSnapshot[] | null = null;
  const snapshots = (): RowSnapshot[] => {
    cached ??= rows.map((r) => rowSnapshot(state, r));
    return cached;
  };
  return [
    {
      type: 'submenu',
      id: 'copy-rows',
      label: 'Copy row(s)',
      icon: 'copy',
      items: [
        {
          type: 'item',
          id: 'copy-rows-tsv',
          label: 'TSV',
          run: () => copyText(rowsToTsv(snapshots())),
        },
        {
          type: 'item',
          id: 'copy-rows-csv',
          label: 'CSV',
          run: () => copyText(rowsToCsv(snapshots())),
        },
        {
          type: 'item',
          id: 'copy-rows-json',
          label: 'JSON',
          run: () => copyText(rowsToJson(snapshots())),
        },
      ],
    },
    { type: 'separator' },
    {
      type: 'item',
      id: 'delete-row',
      label: rows.length > 1 ? 'Delete rows' : 'Delete row',
      icon: 'trash',
      danger: true,
      run: () => actions.deleteRows(rows),
    },
  ];
}

export function columnMenu({ state }: MenuActions, cols: readonly number[]): MenuItem[] {
  const rows = selectedRows(state.selection.sel, state.rowCount());
  return [
    copyItem('copy-column', 'Copy column', () =>
      columnsToTsv(rows, cols, (r, c) => cellAt(state, r, c)),
    ),
    copyItem('copy-column-name', 'Copy column name', () =>
      cols.map((c) => columnName(state, c)).join('\t'),
    ),
  ];
}

export function headerMenu(actions: MenuActions, displayCol: number): MenuItem[] {
  const { state } = actions;
  const name = columnName(state, displayCol);
  return [
    {
      type: 'item',
      id: 'sort-asc',
      label: 'Sort ascending',
      run: () => actions.sort(displayCol, 'asc'),
    },
    {
      type: 'item',
      id: 'sort-desc',
      label: 'Sort descending',
      run: () => actions.sort(displayCol, 'desc'),
    },
    {
      type: 'item',
      id: 'clear-sort',
      label: 'Clear sort',
      disabled: state.sorts.length === 0,
      run: () => actions.sort(displayCol, null),
    },
    { type: 'separator' },
    {
      type: 'item',
      id: 'hide-column',
      label: 'Hide column',
      run: () => actions.hideColumn(displayCol),
    },
    {
      type: 'item',
      id: 'show-all-columns',
      label: 'Show all columns',
      disabled: state.order.length === state.data.columns.length,
      run: () => actions.showAllColumns(),
    },
    { type: 'separator' },
    copyItem('copy-column-name', 'Copy column name', () => name),
    copyItem('copy-column-values', 'Copy column values', () =>
      Array.from({ length: state.rowCount() }, (_, r) => {
        const cell = cellAt(state, r, displayCol);
        return cell.isNull ? '' : cell.text;
      }).join('\n'),
    ),
  ];
}
