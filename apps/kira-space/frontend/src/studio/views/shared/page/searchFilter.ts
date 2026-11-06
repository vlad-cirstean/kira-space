import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';
import { defineStore } from 'pinia';
import { reactive } from 'vue';

// P31 D16: hoisted out of views/grid/search.ts, whose own P24 D2 toggle four other search
// modules (documents/keyvalue/stream) were about to duplicate verbatim — the exact drift P24's
// F5 already documented in these same four files. One module, one cleanup registration, one
// semantic. P375: also holds each tab's find-bar input (query + option toggles), so a tab switch
// that unmounts the bar can restore it.
export interface FindInput {
  query: string;
  matchCase: boolean;
  wholeWord: boolean;
  regex: boolean;
}

const EMPTY_FIND: Readonly<FindInput> = {
  query: '',
  matchCase: false,
  wholeWord: false,
  regex: false,
};

export const usePageSearchFilterStore = defineStore('pageSearchFilter', () => {
  const state = reactive<Record<string, boolean>>({});
  const find = reactive<Record<string, FindInput>>({});

  function findInput(tabId: string): FindInput {
    return find[tabId] ?? EMPTY_FIND;
  }

  function setFindInput(tabId: string, patch: Partial<FindInput>): void {
    find[tabId] = { ...findInput(tabId), ...patch };
  }

  function clearFind(tabId: string): void {
    delete find[tabId];
    delete state[tabId];
  }

  function isSearchFiltering(tabId: string): boolean {
    return state[tabId] === true;
  }

  function setSearchFiltering(tabId: string, on: boolean): void {
    if (on) state[tabId] = true;
    else delete state[tabId];
  }

  function clearSearchFilterState(tabId: string): void {
    delete state[tabId];
    // P63: widens the same way search.ts's own clearSearchState does — see its comment.
    delete state[`${tabId}::preview`];
    delete find[tabId];
    delete find[`${tabId}::preview`];
  }

  registerTabRuntimeCleanup(clearSearchFilterState);

  // Ascending, de-duplicated page-row indices with at least one match, or `null` when
  // the filter is off or there's no completed scan to filter by (an empty query shows every
  // row). Every scanner emits matches in ascending row order (the outer loop is always `row`), so
  // this is one de-duplicating pass with no sort and no Set.
  function matchedRowsOf(
    tabId: string,
    matches: ReadonlyArray<{ row: number }> | undefined,
    scannedRows?: number[],
  ): number[] | null {
    if (!isSearchFiltering(tabId) || !matches) return null;
    if (scannedRows) return scannedRows;
    const rows: number[] = [];
    let last = -1;
    for (const m of matches) {
      if (m.row !== last) {
        rows.push(m.row);
        last = m.row;
      }
    }
    return rows;
  }

  return {
    isSearchFiltering,
    setSearchFiltering,
    matchedRowsOf,
    findInput,
    setFindInput,
    clearFind,
  };
});
