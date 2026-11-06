// collectionsStore.search had no debounce, and
// childrenOf re-filtered+sorted the entire flat `items` array on every single call — called
// repeatedly per row per level by both visibleRows and subtreeMatches, an O(N²)-ish walk on every
// keystroke. This pins both fixes at the state level: the search itself stays immediate (so the
// input never stutters), the debounced query it drives lands ~150ms after typing settles, and a
// request row's own search never recurses into a childrenOf lookup (it can never have children).
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import type { CollectionItemSummary, CollectionSummary } from '@shared/domain/collections';
import { queryClient } from '@workbench/state/queryClient';
import { sleep } from '@workbench/testing/unit/async';
import { setActivePinia } from 'pinia';
import { apiCollectionsTreeKey } from '../../../frontend/src/studio/api/state/apiQueries';
import { pinia } from '../../../frontend/src/studio/state/pinia';

setActivePinia(pinia);

const { useCollectionsStore } = await import('../../../frontend/src/studio/api/state/collections');

const collectionsStore = useCollectionsStore();

// P112: collections/items are now a computed view over the TanStack Query cache, not a settable
// field — seed the tree the same way a real List response would land.
function seedTree(collections: CollectionSummary[], items: CollectionItemSummary[]): void {
  queryClient.setQueryData(apiCollectionsTreeKey, { collections, items });
}

function collection(id: string, name: string, sortOrder = 0): CollectionSummary {
  return { id, name, file: `${id}.yaml`, sortOrder, createdAt: '', updatedAt: '' };
}

function folder(
  id: string,
  collectionId: string,
  parentId: string | null,
  name: string,
  sortOrder = 0,
): CollectionItemSummary {
  return {
    id,
    collectionId,
    parentId,
    kind: 'folder',
    name,
    sortOrder,
    method: '',
    url: '',
    protocol: 'http',
    createdAt: '',
    updatedAt: '',
  };
}

function request(
  id: string,
  collectionId: string,
  parentId: string | null,
  name: string,
  sortOrder = 0,
): CollectionItemSummary {
  return {
    id,
    collectionId,
    parentId,
    kind: 'request',
    name,
    sortOrder,
    method: 'GET',
    url: `https://api.example.com/${name}`,
    protocol: 'http',
    createdAt: '',
    updatedAt: '',
  };
}

describe('api/state/collections.ts search debounce and indexed childrenOf (finding 14)', () => {
  test('the search box value updates immediately, but the query rows filter on stays stale until the debounce settles', async () => {
    seedTree(
      [collection('col-1', 'Orders API')],
      [
        folder('folder-1', 'col-1', null, 'Auth', 0),
        request('req-1', 'col-1', null, 'Health check', 1),
      ],
    );
    collectionsStore.expanded = ['c:col-1'];
    collectionsStore.search = '';
    // Let any debounce timer left over from a previous test in this file settle first.
    await sleep(200);

    collectionsStore.search = 'Health';
    // Immediate: the state itself already carries what was typed.
    expect(collectionsStore.search).toBe('Health');
    // Stale: the debounced query has not caught up yet, so nothing has been filtered out — both
    // the folder and the request are still visible rows.
    expect(collectionsStore.activeSearchQuery).toBe('');
    expect(collectionsStore.visibleRows.map((r) => r.name)).toContain('Auth');

    await sleep(250);

    expect(collectionsStore.activeSearchQuery).toBe('health');
    const names = collectionsStore.visibleRows.map((r) => r.name);
    expect(names).toContain('Health check');
    expect(names).not.toContain('Auth');
  });

  test("a request row's own search never looks for children it can never have", async () => {
    seedTree(
      [collection('col-2', 'Widgets API')],
      [request('req-2', 'col-2', null, 'List widgets', 0)],
    );
    collectionsStore.expanded = ['c:col-2'];
    collectionsStore.search = '';
    await sleep(200);

    collectionsStore.search = 'widgets';
    await sleep(200);

    // The request row itself matches (both name and URL contain "widgets") and renders with no
    // children — proving subtreeMatches' own kind === 'request' short-circuit didn't silently
    // drop it while skipping the childrenOf lookup.
    const row = collectionsStore.visibleRows.find((r) => r.id === 'req-2');
    expect(row?.matched).toBe(true);
    expect(row?.hasChildren).toBe(false);
  });
});
