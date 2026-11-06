import { cellText, isBinary, isNull, isTruncated, type StreamPage } from '@shared/protocol/page';
import { createPageStore, type RetentionEntry, retentionEntries } from '../shared/page/store';

const store = createPageStore<StreamPage>();

export const pageVersion = store.pageVersion;
export const setPage = store.setPage;
export const getPage = store.getPage;
export const drop = store.drop;
export const totalRetainedBytes = store.totalRetainedBytes;
export const setVisibleWindow = store.setVisibleWindow;
/** Playwright-only (main.ts's `window.__kiraRetention`, C1). */
export function pageStoreEntries(): RetentionEntry<StreamPage>[] {
  return retentionEntries(store);
}

export interface StreamRow {
  key: string | null;
  headers: string;
  attrs: string;
  timestamp: string | null;
  /** `null` is a Kafka tombstone: distinct from the empty string. */
  body: string | null;
  isTruncated: boolean;
  /** Cell text is base64 of non-UTF-8 bytes (Kafka); never a decode of them. */
  keyBinary: boolean;
  bodyBinary: boolean;
}

export function streamRow(tabId: string, row: number): StreamRow | null {
  const page = getPage(tabId);
  if (!page || row < 0 || row >= page.rowCount) return null;

  // P2 R2 (task #99): see grid/page.ts's cell() for why this is wrapped in cachedView.
  return store.cachedView(tabId, row, 'row', () => {
    const cached = (subKey: string, chunk: Parameters<typeof cellText>[0]): string =>
      store.cached(tabId, row, subKey, (decoder) => cellText(chunk, row, decoder)) ?? '';

    return {
      key: isNull(page.keys, row) ? null : cached('key', page.keys),
      headers: cached('headers', page.headers),
      attrs: cached('attrs', page.attrs),
      timestamp: isNull(page.timestamps, row) ? null : cached('timestamp', page.timestamps),
      body: isNull(page.bodies, row) ? null : cached('body', page.bodies),
      isTruncated: isTruncated(page.bodies, row),
      keyBinary: isBinary(page.keys, row),
      bodyBinary: isBinary(page.bodies, row),
    };
  });
}
