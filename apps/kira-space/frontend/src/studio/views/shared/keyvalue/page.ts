import type { KeyValuePage } from '@shared/protocol/page';
import { type KeyValueRow, keyValueRowOf } from '../page/rowViews';
import { createPageStore, pageModuleApi } from '../page/store';

const store = createPageStore<KeyValuePage>();

export const {
  pageVersion,
  setPage,
  getPage,
  drop,
  totalRetainedBytes,
  setVisibleWindow,
  pageStoreEntries,
} = pageModuleApi(store);

export type { KeyValueRow };

export function keyValueRow(tabId: string, row: number): KeyValueRow | null {
  const page = getPage(tabId);
  return page ? keyValueRowOf(store, tabId, page, row) : null;
}
