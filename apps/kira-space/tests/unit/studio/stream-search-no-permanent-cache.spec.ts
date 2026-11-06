// runSearch used to read every row through streamRow(), which
// goes through the page store's own memoizing decode/view cache (store.cached/cachedView) — so
// the very first search keystroke on a page permanently decoded and cached *every* row, undoing
// the visible-window pruning that otherwise only re-runs on a scroll event (setVisibleWindow).
// This pins that a search no longer populates that cache at all: pageStoreEntries() (the same
// Playwright-exposed retention probe main.ts's own __kiraRetention uses) must report zero decode-
// cache rows for a tab that has only ever been searched, never rendered.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import { createStreamPageBuilder } from '@shared/protocol/page';
import { setActivePinia } from 'pinia';
import { pinia } from '../../../frontend/src/state/pinia';

setActivePinia(pinia);

const { getPage, pageStoreEntries, setPage } = await import(
  '../../../frontend/src/studio/views/stream/page'
);
const { useStreamSearchStore } = await import('../../../frontend/src/studio/views/stream/search');
const { runSearch, searchState } = useStreamSearchStore();

function buildPage(
  rows: { key: string; headers: string; attrs: string; timestamp: string; body: string }[],
) {
  const builder = createStreamPageBuilder({ visibilityTimeoutSeconds: null });
  for (const row of rows) builder.push(row);
  return builder.finish({
    offset: 0,
    pageSize: rows.length,
    hasMore: false,
    nextToken: null,
    prevToken: null,
    strategy: 'offset',
  });
}

describe('stream/search.ts runSearch reads chunks directly, never through the memoizing store (finding 1)', () => {
  test('a search does not populate the page store decode/view cache at all', () => {
    const tabId = 'stream-tab-perf-1';
    const rows = Array.from({ length: 50 }, (_, i) => ({
      key: `key-${i}`,
      headers: `header-${i}`,
      attrs: `attrs-${i}`,
      timestamp: `2024-01-01T00:00:${String(i).padStart(2, '0')}Z`,
      body: i === 7 ? '{"needle":"FOUND-ME"}' : `{"seq":${i}}`,
    }));
    setPage(tabId, buildPage(rows));

    runSearch(tabId, 'found-me');

    expect(searchState[tabId]?.matches).toEqual([7]);

    const entry = pageStoreEntries().find((e) => e.page === getPage(tabId));
    expect(entry).toBeDefined();
    // The regression this guards against: before the fix, this was 50 (every row's own streamRow()
    // call decoded and cached the whole row through store.cachedView/store.cached).
    expect(entry?.decodeCacheRows).toBe(0);
    expect(entry?.viewCacheRows).toBe(0);
  });

  test('matching is case-insensitive across all five columns, still finding exactly the matching rows', () => {
    const tabId = 'stream-tab-perf-2';
    setPage(
      tabId,
      buildPage([
        { key: 'order-1', headers: 'h', attrs: 'a', timestamp: 't', body: 'nothing here' },
        { key: 'k', headers: 'X-Trace-Id: ABC', attrs: 'a', timestamp: 't', body: 'nothing here' },
        { key: 'k', headers: 'h', attrs: 'source=SEED', timestamp: 't', body: 'nothing here' },
        {
          key: 'k',
          headers: 'h',
          attrs: 'a',
          timestamp: '2024-06-01T00:00:00Z',
          body: 'nothing here',
        },
        { key: 'k', headers: 'h', attrs: 'a', timestamp: 't', body: 'the NEEDLE is in the body' },
        { key: 'k', headers: 'h', attrs: 'a', timestamp: 't', body: 'no match anywhere' },
      ]),
    );

    runSearch(tabId, 'needle');
    expect(searchState[tabId]?.matches).toEqual([4]);

    runSearch(tabId, 'seed');
    expect(searchState[tabId]?.matches).toEqual([2]);

    runSearch(tabId, 'abc');
    expect(searchState[tabId]?.matches).toEqual([1]);

    runSearch(tabId, '2024-06-01');
    expect(searchState[tabId]?.matches).toEqual([3]);
  });
});
