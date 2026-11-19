import type { DocumentPage } from '@shared/protocol/page';
import { compareNatural } from '@workbench/behaviour/match';
import { useDocumentPageStatsStore } from '../shared/document/pageStatsStore';
import { useDocumentRowsStore } from '../shared/document/rows';
import { type DocumentRow, documentIdOf, documentRowOf } from '../shared/page/rowViews';
import { createPageStore, pageModuleApi } from '../shared/page/store';

// P27 D21: a new page has new rows — every memoized parse and every nested-expansion path
// views/shared/document/rows.ts holds for this tab is stale. The reset comes first so the page-stats
// request captures the new page's generation.
const store = createPageStore<DocumentPage>({
  onSet: (tabId) => {
    useDocumentRowsStore().resetRows(tabId);
    const page = store.getPage(tabId);
    if (page) useDocumentPageStatsStore().request(tabId, page);
  },
});

export const {
  pageVersion,
  setPage,
  getPage,
  drop,
  totalRetainedBytes,
  setVisibleWindow,
  pageStoreEntries,
} = pageModuleApi(store);

export type { DocumentRow };

/** One document's id, decoded without its body. */
export function documentId(tabId: string, row: number): string | null {
  const page = getPage(tabId);
  return page ? documentIdOf(store, tabId, page, row) : null;
}

export function documentRow(tabId: string, row: number): DocumentRow | null {
  const page = getPage(tabId);
  return page ? documentRowOf(store, tabId, page, row) : null;
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
// Reactive readers (toolbar, filter row) use `pageFieldNames`, which the parse worker fills off the
// main thread. This synchronous pass is the explicit-action fallback (the projection menu opened
// before the worker answered) and is memoized per page object (setPage freezes and replaces the
// page, so identity is the page version).
const fieldNamesCache = new WeakMap<DocumentPage, string[]>();

/** The worker's field names for the current page; `null` until it has answered. */
export function pageFieldNames(tabId: string): readonly string[] | null {
  return useDocumentPageStatsStore().fieldNamesFor(tabId);
}

export function fieldNamesOnPage(tabId: string): readonly string[] {
  const page = getPage(tabId);
  if (!page) return [];
  const ready = pageFieldNames(tabId);
  if (ready) return ready;
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
