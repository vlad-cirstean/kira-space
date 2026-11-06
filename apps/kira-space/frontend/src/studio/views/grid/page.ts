import { cellText, isNull, isTruncated, type TabularPage } from '@shared/protocol/page';
import { createPageStore, type RetentionEntry, retentionEntries } from '../shared/page/store';

// P48 F19: rebuilt on the shared factory — the two-level decode cache and P29 D7's visible-window
// pruning this file grew on its own now live in views/shared/page/store.ts, shared with
// console/resultPages.ts's own copy of both.
const store = createPageStore<TabularPage>();

export const pageVersion = store.pageVersion;
export const setPage = store.setPage;
export const getPage = store.getPage;
export const drop = store.drop;
export const totalRetainedBytes = store.totalRetainedBytes;
export const setVisibleWindow = store.setVisibleWindow;
/** Playwright-only (main.ts's `window.__kiraRetention`, C1). */
export function pageStoreEntries(): RetentionEntry<TabularPage>[] {
  return retentionEntries(store);
}

export interface CellView {
  text: string;
  isNull: boolean;
  truncated: boolean;
}

// P2 R2 (task #99): shared across every NULL/missing-column return below so those paths don't
// each allocate their own throwaway object — same object every time, safe since callers only read.
const NULL_CELL: CellView = Object.freeze({ text: '', isNull: true, truncated: false });

export function cell(tabId: string, row: number, col: number): CellView {
  const page = getPage(tabId);
  if (!page) return NULL_CELL;
  const chunk = page.chunks[col];
  if (!chunk) return NULL_CELL;
  if (isNull(chunk, row)) return NULL_CELL;

  return store.cachedView(tabId, row, String(col), () => ({
    text: store.cached(tabId, row, String(col), (decoder) => cellText(chunk, row, decoder)),
    isNull: false,
    truncated: isTruncated(chunk, row),
  }));
}
