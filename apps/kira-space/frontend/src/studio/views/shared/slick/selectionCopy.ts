import {
  columnsToTsv,
  disambiguateNames,
  type RowSnapshot,
  rowsToCsv,
  rowsToInsert,
  rowsToJson,
  rowsToTsv,
  tsvField,
} from '../clipboardFormats';
import type { SqlDialect } from '../sqlIdent';
import type { Selection } from './selection';

export type CopyFormat = 'tsv' | 'csv' | 'json' | 'insert';

export interface InsertTarget {
  qualifiedName: string;
  dialect: SqlDialect | undefined;
}

export interface CopySource {
  /** Displayed cell (masking-aware in the data grid). */
  cellAt(row: number, col: number): { text: string; isNull: boolean };
  /** Whole row in display column order (masked in the data grid). */
  rowSnapshot(row: number): RowSnapshot;
  /** Display name of a display column, before disambiguation. */
  columnName(col: number): string;
  /** Visible page rows in [r0, r1] under the active filter. */
  rowsInSpan(r0: number, r1: number): number[];
  /** Every visible page row (column-scoped ops). */
  allRows(): number[];
  insertTarget?: InsertTarget;
}

// Both functions stay synchronous: WebKit grants `writeText` only inside the user gesture.

/** Text for already-built row snapshots. The one format switch. */
export function rowsText(rows: RowSnapshot[], format: CopyFormat, target?: InsertTarget): string {
  switch (format) {
    case 'tsv':
      return rowsToTsv(rows);
    case 'csv':
      return rowsToCsv(rows);
    case 'json':
      return rowsToJson(rows);
    case 'insert':
      if (!target) throw new Error('insert format needs an insert target');
      return rowsToInsert(target.qualifiedName, rows, target.dialect);
  }
}

function cellSnapshots(
  rows: readonly number[],
  cols: readonly number[],
  src: CopySource,
): RowSnapshot[] {
  // A result can repeat a column name; keep every occurrence under its own key.
  const names = disambiguateNames(cols.map((c) => src.columnName(c)));
  return rows.map((row) => {
    const values: Record<string, string | null> = {};
    cols.forEach((c, i) => {
      const dc = src.cellAt(row, c);
      values[names[i] as string] = dc.isNull ? null : dc.text;
    });
    return { columns: names, values };
  });
}

function cellsText(
  rows: readonly number[],
  cols: readonly number[],
  format: CopyFormat,
  src: CopySource,
): string {
  if (format === 'tsv') return columnsToTsv(rows, cols, (r, c) => src.cellAt(r, c));
  if (format === 'insert')
    return rowsText(
      rows.map((r) => src.rowSnapshot(r)),
      format,
      src.insertTarget,
    );
  return rowsText(cellSnapshots(rows, cols, src), format);
}

/** What `sel` copies as `format`. The one "selection to clipboard text" function. */
export function selectionText(sel: Selection, format: CopyFormat, src: CopySource): string {
  switch (sel.kind) {
    case 'cell': {
      if (format === 'tsv' || format === 'json') {
        const dc = src.cellAt(sel.row, sel.col);
        if (format === 'tsv') return dc.isNull ? '' : tsvField(dc.text);
        return JSON.stringify(dc.isNull ? null : dc.text);
      }
      return cellsText([sel.row], [sel.col], format, src);
    }
    case 'range': {
      const cols = Array.from({ length: sel.col - sel.anchorCol + 1 }, (_, i) => sel.anchorCol + i);
      return cellsText(src.rowsInSpan(sel.anchorRow, sel.row), cols, format, src);
    }
    case 'column':
      return cellsText(src.allRows(), sel.cols, format, src);
    case 'row': {
      const rows = [...new Set(sel.rows)].sort((a, b) => a - b);
      return rowsText(
        rows.map((r) => src.rowSnapshot(r)),
        format,
        src.insertTarget,
      );
    }
  }
}
