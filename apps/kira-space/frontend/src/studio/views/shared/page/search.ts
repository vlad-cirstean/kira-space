import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';
import { type ComputedRef, computed, shallowReactive } from 'vue';
import type { SearchHandle, SearchQuery } from './scan';
import { usePageSearchFilterStore } from './searchFilter';

// What views/shared/page/SearchToolbar.vue is bound to. Each of grid/search.ts,
// documents/search.ts and keyvalue/search.ts exports one literal of this shape, built from
// its own runSearch/clearSearchState/searchState/matchedRows plus its page module's
// getPage/pageVersion — the toolbar itself never imports a specific view's page or search module.
export interface PageSearchApi<M extends { row: number }> {
  runSearch(
    tabId: string,
    q: SearchQuery,
    onProgress: (
      found: number,
      rowsScanned: number,
      totalRows: number,
      soFar: readonly M[],
    ) => void,
  ): SearchHandle<M>;
  clearSearchState(tabId: string): void;
  /** P42 D38: `pending` is set on every partial (mid-scan) publication and absent on the
   *  completed one — the toolbar's own read of "has this scan actually finished" (matchedRows
   *  below returns null while it's set, so filtering never hides a row the scan hasn't reached
   *  yet). P5 C4/F6: `found` is the scan's true, uncapped match count — present whenever `matches`
   *  is (mid-scan and completed alike), `undefined` only for a tab with no search state at all. */
  searchState: Record<
    string,
    {
      matches: M[];
      index: number;
      pending?: boolean;
      found?: number;
      matchedRows?: number[];
      /** Page the completed scan ran over; a mismatch on remount means rescan. */
      page?: WeakRef<object>;
    }
  >;
  matchedRows(tabId: string): number[] | null;
  pageVersion: { n: number };
  /** This tab's own page object; lets the toolbar ignore `pageVersion` bumps from other tabs. */
  pageOf?(tabId: string): unknown;
  loadedRowCount(tabId: string): number;
}

/** The per-tab match record + its tab-close cleanup registration, once per view module. Moved
 *  here from scan.ts (P39 iter3 F4/D3) — this is about per-tab search *state*, not scanning,
 *  and createPageSearch below is its only caller. */
function createSearchState<M extends { row: number }>(): {
  searchState: PageSearchApi<M>['searchState'];
  clearSearchState(tabId: string): void;
  matchedRows(tabId: string): number[] | null;
} {
  // Every writer (SearchToolbar.vue's startSearch/goNext/goPrev) replaces a tab's whole
  // entry atomically — a scan publishes a brand-new `matches` array wholesale, never pushes into an
  // existing one — so nothing below the top level needs field-by-field reactivity. `reactive()`
  // used to deep-wrap every match object in every tab's array (thousands, for a large scan) in a
  // Proxy for no benefit; shallowReactive tracks exactly what's mutated: `searchState[id]`
  // being set to a new value.
  const searchState = shallowReactive({} as PageSearchApi<M>['searchState']);

  function clearSearchState(tabId: string): void {
    delete searchState[tabId];
    // BrowseView.vue's split keys its own preview pane's keyvalue search state
    // `${tabId}::preview` — not a real tab id, so it never closes on its own; it dies only when
    // the browse tab that owns it does. A no-op for every instance of this factory that never has
    // one (grid/documents/stream), harmless to widen generically here rather than per view.
    delete searchState[`${tabId}::preview`];
  }
  registerTabRuntimeCleanup(clearSearchState);

  // P42 D38: a partial (mid-scan) publication filters nothing — the matches found *so far* say
  // nothing about the rows the scan hasn't reached yet, and hiding on that basis would make rows
  // vanish and reappear as the scan caught up, under a user trying to read them.
  function matchedRows(tabId: string): number[] | null {
    const entry = searchState[tabId];
    if (entry?.pending) return null;
    return usePageSearchFilterStore().matchedRowsOf(tabId, entry?.matches, entry?.matchedRows);
  }

  return { searchState, clearSearchState, matchedRows };
}

// grid/search.ts, documents/search.ts and keyvalue/search.ts each assembled this
// same six-ingredient PageSearchApi literal by hand from createSearchState()'s three return values
// plus their own runSearch/pageVersion/getPage. One factory in one place instead of the same
// assembly written out three times.
export function createPageSearch<M extends { row: number }>(opts: {
  runSearch: PageSearchApi<M>['runSearch'];
  pageVersion: { n: number };
  pageOf?: PageSearchApi<M>['pageOf'];
  loadedRowCount(tabId: string): number;
}): {
  searchState: PageSearchApi<M>['searchState'];
  clearSearchState(tabId: string): void;
  matchedRows(tabId: string): number[] | null;
  api: PageSearchApi<M>;
} {
  const { searchState, clearSearchState, matchedRows } = createSearchState<M>();
  return {
    searchState,
    clearSearchState,
    matchedRows,
    api: {
      runSearch: opts.runSearch,
      clearSearchState,
      searchState,
      matchedRows,
      pageVersion: opts.pageVersion,
      pageOf: opts.pageOf,
      loadedRowCount: opts.loadedRowCount,
    },
  };
}

// The deleted DataGrid.vue, KeyValueView.vue and ConsoleResultGrid.vue each built this same
// plus current-match computed, rebuilt only when the search result changes (a completed scan or
// prev/next), not per cell — differing only in `col`'s type ('field' | 'value' for a key/value row,
// a page column index everywhere else). `tabId` is a function, not a plain string, so the computed
// re-tracks when a caller's own prop changes.
//
// This used to build a Set<string> of `${row}:${col}` template-literal keys — a string
// allocation and concatenation per match, on every rebuild, purely to give `has` an O(1) lookup a
// nested Map/Set gets for free with no string keys at all (row is already a native number; col is
// whatever primitive type C already is). Mirrors DocumentView.vue's own docMatchIndex, which never
// had this problem since it never used createMatchIndex.
export function createMatchIndex<C>(
  state: Record<string, { matches: { row: number; col: C }[]; index: number }>,
  tabId: () => string,
): ComputedRef<{
  has(row: number, col: C): boolean;
  isCurrent(row: number, col: C): boolean;
} | null> {
  // byRow used to rebuild from scratch on *every* call —
  // including a bare Next/Prev, which only moves `index` by one (SearchToolbar.vue's
  // goNext/goPrev replace the whole entry with `{ ...e, index }`, so this computed's own tracked
  // dependency — `state[tabId()]`'s reference — changes even though `entry.matches` itself is the
  // *same* array both times, spread-copied by reference, not cloned). Vue's computed caching can
  // only invalidate at the granularity of "did any dependency I read change", not "did this one
  // field change" — reactive() has no visibility into a plain object's own nested fields once
  // `searchState` is only shallowReactive — so this adds
  // its own manual memo keyed on `entry.matches`' identity: pressing Next/Prev rebuilds nothing
  // (byRow is reused as-is), and only a genuinely new match array (a fresh scan tick or a
  // completed scan) pays the O(matches) rebuild.
  let lastMatches: { row: number; col: C }[] | null = null;
  let lastByRow: Map<number, Set<C>> | null = null;

  return computed(() => {
    const entry = state[tabId()];
    if (!entry) return null;

    let byRow: Map<number, Set<C>>;
    if (entry.matches === lastMatches && lastByRow) {
      byRow = lastByRow;
    } else {
      byRow = new Map<number, Set<C>>();
      for (const m of entry.matches) {
        let cols = byRow.get(m.row);
        if (!cols) {
          cols = new Set<C>();
          byRow.set(m.row, cols);
        }
        cols.add(m.col);
      }
      lastMatches = entry.matches;
      lastByRow = byRow;
    }

    const current = entry.index >= 0 ? entry.matches[entry.index] : undefined;
    return {
      has: (row: number, col: C) => byRow.get(row)?.has(col) ?? false,
      isCurrent: (row: number, col: C) => !!current && current.row === row && current.col === col,
    };
  });
}
