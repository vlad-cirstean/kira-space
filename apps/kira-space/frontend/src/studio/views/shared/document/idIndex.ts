import { cellText, type DocumentPage } from '@shared/protocol/page';

// Keyed by the page object: a reload makes a new page and drops the old index with it.
const indexes = new WeakMap<DocumentPage, ReadonlyMap<string, number>>();

/** Document `_id` text to row, decoded once per page rather than per height recompute. */
export function documentIdRows(page: DocumentPage): ReadonlyMap<string, number> {
  let index = indexes.get(page);
  if (!index) {
    const decoder = new TextDecoder();
    const built = new Map<string, number>();
    for (let row = 0; row < page.rowCount; row++) {
      const id = cellText(page.ids, row, decoder);
      if (!built.has(id)) built.set(id, row);
    }
    index = built;
    indexes.set(page, index);
  }
  return index;
}

/** Rows of `page` whose `_id` is in `ids`; ids not on the page are skipped. */
export function rowsOfIds(page: DocumentPage | null, ids: readonly string[]): Set<number> {
  const index = page && ids.length > 0 ? documentIdRows(page) : null;
  return new Set(ids.flatMap((id) => index?.get(id) ?? []));
}
