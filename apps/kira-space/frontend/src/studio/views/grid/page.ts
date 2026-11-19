import type { TabularPage } from '@shared/protocol/page';
import { type CellView, NULL_CELL, tabularCell } from '../shared/page/rowViews';
import { createPageStore, pageModuleApi } from '../shared/page/store';

export type { CellView };

const store = createPageStore<TabularPage>();

export const {
  pageVersion,
  setPage,
  getPage,
  drop,
  totalRetainedBytes,
  setVisibleWindow,
  pageStoreEntries,
} = pageModuleApi(store);

export function cell(tabId: string, row: number, col: number): CellView {
  const page = getPage(tabId);
  return page ? tabularCell(store, tabId, page, row, col) : NULL_CELL;
}
