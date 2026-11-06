import { cellByteLength, cellText, type DocumentPage, isTruncated } from '@shared/protocol/page';
import { compareNatural } from '@workbench/behaviour/match';
import { useDocumentRowsStore } from '../shared/document/rows';
import { createPageStore, type RetentionEntry, retentionEntries } from '../shared/page/store';

// P27 D21: a new page has new rows — every memoized parse and every nested-expansion path
// views/shared/document/rows.ts holds for this tab is stale.
const store = createPageStore<DocumentPage>({
  onSet: (tabId) => useDocumentRowsStore().resetRows(tabId),
});

export const pageVersion = store.pageVersion;
export const setPage = store.setPage;
export const getPage = store.getPage;
export const drop = store.drop;
export const totalRetainedBytes = store.totalRetainedBytes;
export const setVisibleWindow = store.setVisibleWindow;
/** Playwright-only (main.ts's `window.__kiraRetention`, C1). */
export function pageStoreEntries(): RetentionEntry<DocumentPage>[] {
  return retentionEntries(store);
}

export interface DocumentRow {
  id: string;
  body: string;
  isTruncated: boolean;
  bodyByteLength: number;
}

export function documentRow(tabId: string, row: number): DocumentRow | null {
  const page = getPage(tabId);
  if (!page || row < 0 || row >= page.rowCount) return null;
  // P2 R2 (task #99): see grid/page.ts's cell() for why this is wrapped in cachedView.
  return store.cachedView(tabId, row, 'row', () => ({
    id: store.cached(tabId, row, 'id', (decoder) => cellText(page.ids, row, decoder)) ?? '',
    body: store.cached(tabId, row, 'body', (decoder) => cellText(page.bodies, row, decoder)) ?? '',
    isTruncated: isTruncated(page.bodies, row),
    // Straight from the wire's own offsets — see cellByteLength's own doc comment for why this is
    // preferred over TextEncoder().encode(body).length (views/shared/document/rows.ts's byteLabel).
    bodyByteLength: cellByteLength(page.bodies, row),
  }));
}

// The projection picker's candidate list (ProjectionMenu.vue), the filter/sort autocomplete
// (views/shared/mongoFieldSample.ts) and the toolbar's Fields tooltip (DocumentView.vue) all need
// "every top-level field name seen on the loaded page" — a document collection has no catalog to
// read a field list from, so this is the closest equivalent, shared so those call sites can't
// drift on how they read a body's field names. `_id` is left out: it is always returned regardless
// of projection (mongo/read.ts's own comment) and already has its own column, so it is never a
// real projection choice.
//
// No longer routes through views/shared/document/rows.ts's memoized parse (the reasoning for doing
// so — "the tree renderer already walks every body on the page" — was true only while
// DocumentView.vue's own `rows` computed eagerly parsed every row; that is now lazy, so reusing
// the shared cache here would have made this the one thing silently re-materialising it, on every
// load, for every row). A plain top-level `JSON.parse` + `Object.keys` per row is transient
// (nothing retained past this call) instead of building and caching a full recursive DocNode tree
// per row the way `rowView`'s own `parseDocument` does — this still has to walk every row
// (there is no smaller correct answer for "every field name on the page"), but it no longer grows
// `rows.ts`'s `parseCache` to the size of the whole page in the process.
//
// Four computeds plus ProjectionMenu call this per page load, so the result is memoized per page
// object (setPage freezes and replaces the page, so identity is the page version).
const fieldNamesCache = new WeakMap<DocumentPage, string[]>();

export function fieldNamesOnPage(tabId: string): string[] {
  const page = getPage(tabId);
  if (!page) return [];
  const cached = fieldNamesCache.get(page);
  if (cached) return cached;
  const names = new Set<string>();
  for (let row = 0; row < page.rowCount; row++) {
    const doc = documentRow(tabId, row);
    if (!doc) continue;
    let parsed: unknown;
    try {
      parsed = JSON.parse(doc.body);
    } catch {
      continue;
    }
    if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) continue;
    for (const key of Object.keys(parsed as Record<string, unknown>)) {
      if (key !== '_id') names.add(key);
    }
  }
  const sorted = [...names].sort(compareNatural);
  fieldNamesCache.set(page, sorted);
  return sorted;
}
