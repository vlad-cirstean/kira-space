import {
  cellByteLength,
  cellText,
  type DocumentPage,
  isNull,
  isTruncated,
  type KeyValuePage,
  type TabularPage,
} from '@shared/protocol/page';
import type { PageStore } from './store';

export interface CellView {
  text: string;
  isNull: boolean;
  truncated: boolean;
}

// Shared by every NULL/missing-column return so those paths allocate nothing; callers only read.
export const NULL_CELL: CellView = Object.freeze({ text: '', isNull: true, truncated: false });

export interface DocumentRow {
  id: string;
  body: string;
  isTruncated: boolean;
  bodyByteLength: number;
}

export interface KeyValueRow {
  field: string;
  value: string;
  isTruncated: boolean;
}

type RowStore = Pick<PageStore<{ rowCount: number; byteSize: number }>, 'cached' | 'cachedView'>;

const outOfRange = (page: { rowCount: number }, row: number): boolean =>
  row < 0 || row >= page.rowCount;

/** Hot path: no row bound check. `cachedView` keeps the returned object identical per cell. */
export function tabularCell(
  store: RowStore,
  scope: string,
  page: TabularPage,
  row: number,
  col: number,
): CellView {
  const chunk = page.chunks[col];
  if (!chunk) return NULL_CELL;
  if (isNull(chunk, row)) return NULL_CELL;

  return store.cachedView(scope, row, String(col), () => ({
    text: store.cached(scope, row, String(col), (decoder) => cellText(chunk, row, decoder)),
    isNull: false,
    truncated: isTruncated(chunk, row),
  }));
}

/** One document's id, decoded without its body. */
export function documentIdOf(
  store: RowStore,
  scope: string,
  page: DocumentPage,
  row: number,
): string | null {
  if (outOfRange(page, row)) return null;
  return store.cached(scope, row, 'id', (decoder) => cellText(page.ids, row, decoder));
}

export function documentRowOf(
  store: RowStore,
  scope: string,
  page: DocumentPage,
  row: number,
): DocumentRow | null {
  if (outOfRange(page, row)) return null;
  return store.cachedView(scope, row, 'row', () => ({
    id: store.cached(scope, row, 'id', (decoder) => cellText(page.ids, row, decoder)),
    body: store.cached(scope, row, 'body', (decoder) => cellText(page.bodies, row, decoder)),
    isTruncated: isTruncated(page.bodies, row),
    // Straight from the wire's own offsets: cheaper than TextEncoder().encode(body).length.
    bodyByteLength: cellByteLength(page.bodies, row),
  }));
}

export function keyValueRowOf(
  store: RowStore,
  scope: string,
  page: KeyValuePage,
  row: number,
): KeyValueRow | null {
  if (outOfRange(page, row)) return null;
  return store.cachedView(scope, row, 'row', () => ({
    field: store.cached(scope, row, 'field', (decoder) => cellText(page.fields, row, decoder)),
    value: store.cached(scope, row, 'value', (decoder) => cellText(page.values, row, decoder)),
    isTruncated: isTruncated(page.values, row),
  }));
}
