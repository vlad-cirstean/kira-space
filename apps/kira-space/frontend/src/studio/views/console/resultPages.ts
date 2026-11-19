import type { Page } from '@shared/protocol/page';
import {
  type CellView,
  type DocumentRow,
  documentIdOf,
  documentRowOf,
  type KeyValueRow,
  keyValueRowOf,
  NULL_CELL,
  tabularCell,
} from '../shared/page/rowViews';
import { createPageStore, pageModuleApi } from '../shared/page/store';

// The console's own page store, widened to hold either page kind — a console
// result can be tabular (SQL) or document (Mongo shell), unlike `views/grid/page.ts`'s store,
// which stays tabular-only because a 'data' tab is never anything else. Kept as a sibling file
// rather than widening grid/page.ts's `cell()`/DataGrid.vue's call sites, which assume tabular
// throughout.
//
// Rebuilt on the shared factory — the two-level decode cache and visible-window pruning
// this file grew separately now live in views/shared/page/store.ts, shared with
// views/grid/page.ts's own copy of both.
const store = createPageStore<Page>();

export const {
  pageVersion,
  /** state.ts's setActiveResult (P40 D9) raises the same "the page this scope resolves to changed"
   *  signal without reaching into the counter directly. */
  bumpPageVersion,
  setPage,
  getPage,
  drop,
  dropForPrefix: dropForTab,
  totalRetainedBytes,
  /** Called by ConsoleResultGrid.vue and ConsoleSlickGrid.vue on scroll. */
  setVisibleWindow,
  pageStoreEntries,
} = pageModuleApi(store);

export type { CellView };

/** Tabular-only, same contract as `views/grid/page.ts`'s `cell()`. */
export function cell(key: string, row: number, col: number): CellView {
  const page = getPage(key);
  return page?.kind === 'tabular' ? tabularCell(store, key, page, row, col) : NULL_CELL;
}

/** Document-only: one document's id, decoded without its body (window-only callers). */
export function documentId(key: string, row: number): string | null {
  const page = getPage(key);
  return page?.kind === 'document' ? documentIdOf(store, key, page, row) : null;
}

/** Document-only: one document's id/body text, decoded from a DocumentPage's chunks. */
export function documentRow(key: string, row: number): DocumentRow | null {
  const page = getPage(key);
  return page?.kind === 'document' ? documentRowOf(store, key, page, row) : null;
}

/** Key/value-only: one field/value pair, decoded from a KeyValuePage's chunks. */
export function keyValueRow(key: string, row: number): KeyValueRow | null {
  const page = getPage(key);
  return page?.kind === 'keyvalue' ? keyValueRowOf(store, key, page, row) : null;
}
