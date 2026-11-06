import { cellByteLength, cellText, isNull, isTruncated, type Page } from '@shared/protocol/page';
import { createPageStore, type RetentionEntry, retentionEntries } from '../shared/page/store';

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

export const pageVersion = store.pageVersion;
/** state.ts's setActiveResult (P40 D9) raises the same "the page this scope resolves to changed"
 *  signal without reaching into the counter directly. */
export const bumpPageVersion = store.bumpPageVersion;
export const setPage = store.setPage;
export const getPage = store.getPage;
export const drop = store.drop;
export const dropForTab = store.dropForPrefix;
/** D5: mirrors `views/grid/page.ts`'s `totalRetainedBytes()` — feeds `window.__kiraRetainedBytes`. */
export const totalRetainedBytes = store.totalRetainedBytes;
/** Called by ConsoleResultGrid.vue on scroll (from the same bounds its own search-priority report
 *  already computes, P42 D39 — see `visibleRows.ts`'s own handoff note). */
export const setVisibleWindow = store.setVisibleWindow;
/** Playwright-only (main.ts's `window.__kiraRetention`, C1). */
export function pageStoreEntries(): RetentionEntry<Page>[] {
  return retentionEntries(store);
}

export interface CellView {
  text: string;
  isNull: boolean;
  truncated: boolean;
}

// P2 R2 (task #99): see grid/page.ts's own copy of this constant for why.
const NULL_CELL: CellView = Object.freeze({ text: '', isNull: true, truncated: false });

/** Tabular-only, same contract as `views/grid/page.ts`'s `cell()`. */
export function cell(key: string, row: number, col: number): CellView {
  const page = getPage(key);
  if (page?.kind !== 'tabular') return NULL_CELL;
  const chunk = page.chunks[col];
  if (!chunk) return NULL_CELL;
  if (isNull(chunk, row)) return NULL_CELL;

  return store.cachedView(key, row, String(col), () => ({
    text: store.cached(key, row, String(col), (d) => cellText(chunk, row, d)),
    isNull: false,
    truncated: isTruncated(chunk, row),
  }));
}

/** Document-only: one document's id/body text, decoded from a DocumentPage's chunks. Mirrors
 *  views/documents/page.ts's own documentRow() — the divergence (this one
 *  hard-coded `isTruncated: false`) meant the console's own cell-editor publish could never know
 *  a Mongo console document's body was cut, unlike the data-tab equivalent. */
export function documentRow(
  key: string,
  row: number,
): { id: string; body: string; isTruncated: boolean; bodyByteLength: number } | null {
  const page = getPage(key);
  if (page?.kind !== 'document') return null;
  return store.cachedView(key, row, 'row', () => ({
    id: store.cached(key, row, 'id', (d) => cellText(page.ids, row, d)),
    body: store.cached(key, row, 'body', (d) => cellText(page.bodies, row, d)),
    isTruncated: isTruncated(page.bodies, row),
    bodyByteLength: cellByteLength(page.bodies, row),
  }));
}

/** Key/value-only: one field/value pair, decoded from a KeyValuePage's chunks. */
export function keyValueRow(key: string, row: number): { field: string; value: string } | null {
  const page = getPage(key);
  if (page?.kind !== 'keyvalue') return null;
  return store.cachedView(key, row, 'row', () => ({
    field: store.cached(key, row, 'field', (d) => cellText(page.fields, row, d)),
    value: store.cached(key, row, 'value', (d) => cellText(page.values, row, d)),
  }));
}
