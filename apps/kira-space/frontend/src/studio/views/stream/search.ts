import { cellText, isNull, type StreamPage } from '@shared/protocol/page';
import { matchesQuery } from '@workbench/behaviour/match';
import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';
import { defineStore } from 'pinia';
import { reactive } from 'vue';
import { usePageSearchFilterStore } from '../shared/page/searchFilter';
import { getPage } from './page';

// The precedent (grid/search.ts): filters purely client-side against the already-fetched
// page, never a fresh server call — `getPage`'s rowCount is the only thing this ever iterates.
// Kept deliberately simpler than grid/search.ts's own module: one case-insensitive substring
// match across all five columns (no whole-word/regex toggles, no requestAnimationFrame chunking)
// — a stream page's five fixed short-text columns are a much smaller, much more uniform search
// surface than a SQL grid's arbitrary N-column, arbitrary-width page, so the extra machinery grid/
// search.ts needs to stay off the frame budget for a big page isn't earning its keep here. Applies
// identically to Kafka and SQS pages (search is read-only, no protocol difference).
interface StreamSearchState {
  query: string;
  matches: number[]; // row indices, ascending
  index: number; // position within `matches`, or -1 when there are none
  page?: WeakRef<StreamPage>; // page the scan ran over; a mismatch on remount means rescan
}

export const useStreamSearchStore = defineStore('streamSearch', () => {
  const searchState = reactive({} as Record<string, StreamSearchState>);

  function clearSearchState(tabId: string): void {
    delete searchState[tabId];
  }

  // D4: closeTab has no way to import this leaf module directly (reality 18) — registers here,
  // mirrors grid/search.ts's own registration.
  registerTabRuntimeCleanup(clearSearchState);

  // rowMatches used to be `streamRow(tabId, row)` — every field
  // read that way, page.ts's own comment explains, goes through store.cachedView/store.cached (the
  // same memoizing decode/view cache the grid/documents/console hosts use), so the very first
  // search keystroke on a page permanently decoded and cached *every* row, undoing the visible-
  // window pruning that otherwise only re-runs on a scroll event (setVisibleWindow). This reads
  // straight off the page's own chunks with a plain TextDecoder instead, the same way scan.ts's own
  // tabularRowScanner/keyValueRowScanner do for every other paged view's search — nothing here is
  // retained past this one call.
  //
  // Each column is checked independently rather than concatenated into one
  // haystack string first: a hit in an early, typically-short column (key/headers) skips decoding
  // and lowering the row's own body at all, and a miss lowercases only that one column's own
  // text rather than the whole row's five fields joined together.
  function rowMatches(
    page: StreamPage,
    row: number,
    needle: string,
    decoder: TextDecoder,
  ): boolean {
    if (!isNull(page.keys, row) && matchesQuery(cellText(page.keys, row, decoder), needle)) {
      return true;
    }
    if (matchesQuery(cellText(page.headers, row, decoder), needle)) return true;
    if (matchesQuery(cellText(page.attrs, row, decoder), needle)) return true;
    if (
      !isNull(page.timestamps, row) &&
      matchesQuery(cellText(page.timestamps, row, decoder), needle)
    ) {
      return true;
    }
    return !isNull(page.bodies, row) && matchesQuery(cellText(page.bodies, row, decoder), needle);
  }

  function runSearch(tabId: string, query: string): void {
    if (query.trim() === '') {
      clearSearchState(tabId);
      return;
    }
    const page = getPage(tabId);
    const needle = query;
    const matches: number[] = [];
    if (page) {
      const decoder = new TextDecoder();
      for (let row = 0; row < page.rowCount; row++) {
        if (rowMatches(page, row, needle, decoder)) matches.push(row);
      }
    }
    searchState[tabId] = {
      query,
      matches,
      index: matches.length > 0 ? 0 : -1,
      ...(page ? { page: new WeakRef(page) } : {}),
    };
  }

  // P31 D16: this module's own matches are already ascending, distinct row indices (one entry
  // per matching row, built by a single `row` loop) — filtering just gates them on the toggle,
  // with no de-dup pass needed the way matchedRowsOf's Match[]-shaped callers need one.
  function matchedRows(tabId: string): number[] | null {
    if (!usePageSearchFilterStore().isSearchFiltering(tabId)) return null;
    return searchState[tabId]?.matches ?? null;
  }

  function goToNextMatch(tabId: string): number | null {
    const s = searchState[tabId];
    if (!s || s.matches.length === 0) return null;
    s.index = (s.index + 1) % s.matches.length;
    return s.matches[s.index];
  }

  function goToPrevMatch(tabId: string): number | null {
    const s = searchState[tabId];
    if (!s || s.matches.length === 0) return null;
    s.index = (s.index - 1 + s.matches.length) % s.matches.length;
    return s.matches[s.index];
  }

  return {
    searchState,
    clearSearchState,
    runSearch,
    matchedRows,
    goToNextMatch,
    goToPrevMatch,
  };
});
