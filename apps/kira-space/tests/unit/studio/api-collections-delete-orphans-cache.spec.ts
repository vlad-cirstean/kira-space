// A folder or a collection leaves the tree (its file was edited or removed, in this window or
// another) and every descendant request leaves with it. The reconcile after the tree refresh must
// purge every *descendant* request's cached HttpSavedRequest/GrpcSavedRequest, not just one entry.
// The orphan rule
// (HttpRequestView.vue: canSave depends on the apiSavedRequestKey query reading non-null) then
// silently misbehaved for any tab open on one of those orphaned descendants: the stale cache entry
// made the tab look saveable when its row no longer existed at all.
//
// Asserts against the apiSavedRequestKey query cache directly (queryClient.getQueryData), which is
// what reconcileTree writes `null` into for the whole orphaned subtree. The refresh is driven the
// way a `kira:api:dataChanged` {kind:'tree'} broadcast drives it.
import '@workbench/testing/unit/window';

import { afterAll, describe, expect, test } from 'bun:test';
import type { CollectionItemSummary, CollectionSummary } from '@shared/domain/collections';
import { queryClient } from '@workbench/state/queryClient';
import { restoreAfterEach } from '@workbench/testing/unit/restoreAfterEach';
import { setActivePinia } from 'pinia';
import { pinia } from '../../../frontend/src/state/pinia';
import {
  apiCollectionsTreeKey,
  apiSavedRequestKey,
  loadSavedRequest,
  reconcileTree,
  refreshApiQuery,
} from '../../../frontend/src/studio/api/state/apiQueries';

setActivePinia(pinia);

const { control } = await import('../../../frontend/src/studio/bridge/control');
// wailsRuntime.ts's mocked transport deliberately never settles an unmocked call —
// useCollectionsStore below mounts a permanent, eager `useQuery` for the tree the moment the store
// is created, so it needs a resolving default in place *before* that first call, or its own
// initial fetch hangs forever and wedges every later invalidateQueries-triggered refetch
// (refreshApiQuery) behind it. Set before restoreAfterEach's own snapshot,
// so each test's afterEach restores to this benign default rather than reintroducing the hang.
//
// restoreAfterEach's own snapshot is taken *after* this override (deliberately, so the
// per-test reset keeps the benign default) — which never restores the *true* original, and
// `control` is one process-wide singleton (restoreAfterEach.ts's own documented hazard: a spec's
// stub that outlives its file leaks into whatever runs next in the same bun test process).
// Captured here and restored in afterAll, so bridge-unwrap.spec.ts's generic "every control method
// rejects" check never sees this file's own default once this file's own tests are done.
const originalCollectionsList = control.collectionsList;
afterAll(() => {
  control.collectionsList = originalCollectionsList;
});
(control as unknown as { collectionsList: typeof control.collectionsList }).collectionsList =
  async () => ({ collections: [], items: [] });
restoreAfterEach(control);
const { useCollectionsStore } = await import('../../../frontend/src/studio/api/state/collections');

const collectionsStore = useCollectionsStore();
// Lets the store's own eager initial fetch (above) actually settle before any test runs, so
// `refreshApiQuery`'s own in-flight capture never mistakes it for a test's own mocked fetch.
await new Promise((resolve) => setTimeout(resolve, 0));

// P112: collections/items are now a computed view over the TanStack Query cache, not a settable
// field — seed the tree the same way a real List response would land.
function seedTree(collections: CollectionSummary[], items: CollectionItemSummary[]): void {
  queryClient.setQueryData(apiCollectionsTreeKey, { collections, items });
}

function savedRequestCache(itemId: string): unknown {
  return queryClient.getQueryData(apiSavedRequestKey(itemId));
}

function collection(id: string, name: string): CollectionSummary {
  return { id, name, file: `${id}.yaml`, sortOrder: 0, createdAt: '', updatedAt: '' };
}

function folder(
  id: string,
  collectionId: string,
  parentId: string | null,
  name: string,
): CollectionItemSummary {
  return {
    id,
    collectionId,
    parentId,
    kind: 'folder',
    name,
    sortOrder: 0,
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
): CollectionItemSummary {
  return {
    id,
    collectionId,
    parentId,
    kind: 'request',
    name,
    sortOrder: 0,
    method: 'GET',
    url: `https://api.example.com/${name}`,
    protocol: 'http',
    createdAt: '',
    updatedAt: '',
  };
}

// Only the fields httpSavedRequestSchema requires without a default matter here;
// loadSavedRequest's own .parse() fills in the rest from the schema's defaults.
function savedRequest(name: string): Awaited<ReturnType<typeof control.collectionsGetRequest>> {
  return { method: 'GET', url: `https://api.example.com/${name}` } as Awaited<
    ReturnType<typeof control.collectionsGetRequest>
  >;
}

describe('a tree refresh purges the whole removed subtree from the request caches', () => {
  test('a removed folder also drops its nested requests, not just the folder itself', async () => {
    // col-1
    //   folder-1 ("Auth")
    //     req-1 ("Login")           <- direct child
    //     folder-2 ("Nested")
    //       req-2 ("Refresh token") <- grandchild, via a nested folder
    //   req-3 ("Health check")      <- sibling, must survive
    seedTree(
      [collection('col-1', 'Orders API')],
      [
        folder('folder-1', 'col-1', null, 'Auth'),
        request('req-1', 'col-1', 'folder-1', 'Login'),
        folder('folder-2', 'col-1', 'folder-1', 'Nested'),
        request('req-2', 'col-1', 'folder-2', 'Refresh token'),
        request('req-3', 'col-1', null, 'Health check'),
      ],
    );

    (
      control as unknown as { collectionsGetRequest: typeof control.collectionsGetRequest }
    ).collectionsGetRequest = async (id: string) => savedRequest(id);
    await loadSavedRequest('req-1');
    await loadSavedRequest('req-2');
    await loadSavedRequest('req-3');
    expect(savedRequestCache('req-1')).not.toBeNull();
    expect(savedRequestCache('req-2')).not.toBeNull();
    expect(savedRequestCache('req-3')).not.toBeNull();

    (control as unknown as { collectionsList: typeof control.collectionsList }).collectionsList =
      async () => ({
        collections: collectionsStore.collections,
        // Mirrors the real cascade: folder-1, req-1, folder-2 and req-2 are all gone post-removal.
        items: collectionsStore.items.filter((item) => item.id === 'req-3'),
      });

    await refreshApiQuery(apiCollectionsTreeKey);
    reconcileTree();

    // The direct child and the nested folder's own child must both be purged.
    expect(savedRequestCache('req-1')).toBeNull();
    expect(savedRequestCache('req-2')).toBeNull();
    // The sibling outside the deleted subtree must survive.
    expect(savedRequestCache('req-3')).not.toBeNull();
  });

  test('a removed collection purges every item in it, regardless of nesting depth', async () => {
    seedTree(
      [collection('col-2', 'Widgets API')],
      [
        folder('folder-3', 'col-2', null, 'Top'),
        request('req-4', 'col-2', 'folder-3', 'List widgets'),
      ],
    );
    (
      control as unknown as { collectionsGetRequest: typeof control.collectionsGetRequest }
    ).collectionsGetRequest = async (id: string) => savedRequest(id);
    await loadSavedRequest('req-4');
    expect(savedRequestCache('req-4')).not.toBeNull();

    (control as unknown as { collectionsList: typeof control.collectionsList }).collectionsList =
      async () => ({
        collections: [],
        items: [],
      });

    await refreshApiQuery(apiCollectionsTreeKey);
    reconcileTree();

    expect(savedRequestCache('req-4')).toBeNull();
  });
});
