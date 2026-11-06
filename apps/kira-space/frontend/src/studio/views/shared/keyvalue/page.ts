import { cellText, isTruncated, type KeyValuePage } from '@shared/protocol/page';
import { createPageStore, type RetentionEntry, retentionEntries } from '../page/store';

const store = createPageStore<KeyValuePage>();

export const pageVersion = store.pageVersion;
export const setPage = store.setPage;
export const getPage = store.getPage;
export const drop = store.drop;
export const totalRetainedBytes = store.totalRetainedBytes;
export const setVisibleWindow = store.setVisibleWindow;

/** Playwright-only (main.ts's `window.__kiraRetention`, C1). */
export function pageStoreEntries(): RetentionEntry<KeyValuePage>[] {
  return retentionEntries(store);
}

export interface KeyValueRow {
  field: string;
  value: string;
  isTruncated: boolean;
}

export function keyValueRow(tabId: string, row: number): KeyValueRow | null {
  const page = getPage(tabId);
  if (!page || row < 0 || row >= page.rowCount) return null;
  // P2 R2 (task #99): see grid/page.ts's cell() for why this is wrapped in cachedView.
  return store.cachedView(tabId, row, 'row', () => ({
    field:
      store.cached(tabId, row, 'field', (decoder) => cellText(page.fields, row, decoder)) ?? '',
    value:
      store.cached(tabId, row, 'value', (decoder) => cellText(page.values, row, decoder)) ?? '',
    isTruncated: isTruncated(page.values, row),
  }));
}
