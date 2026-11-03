// An environment's file is removed (in this window or another) and the list refreshes. A leftover
// `incognitoEnvByTab` entry holding the removed id would keep an incognito tab substituting its
// plain values in stage 1 while Go resolved no secrets for the missing environment, and the
// environment-scope variables query would stay cached forever. This pins both evictions.
//
// Eviction is `reconcileEnvironments`/`reconcileTree` (apiQueries.ts), run after a
// `kira:api:dataChanged` {kind:'environments'|'tree'} refresh, which these tests drive directly.
import '@workbench/testing/unit/window';

import { afterAll, describe, expect, test } from 'bun:test';
import type { CollectionItemSummary, CollectionSummary } from '@shared/domain/collections';
import type { ApiEnvironment, ApiVariable } from '@shared/domain/variables';
import { queryClient } from '@workbench/state/queryClient';
import { restoreAfterEach } from '@workbench/testing/unit/restoreAfterEach';
import { setActivePinia } from 'pinia';
import { pinia } from '../../frontend/src/state/pinia';

setActivePinia(pinia);

const { control } = await import('../../frontend/src/bridge/control');
// wailsRuntime.ts's mocked transport deliberately never settles an unmocked call — useVariablesStore
// and useCollectionsStore below each mount a permanent, eager `useQuery` (apiQueries.ts) the moment
// the store is created, so both need a resolving default in place *before* that first call, or its
// own initial fetch hangs forever and wedges every later invalidateQueries-triggered refetch behind
// it. Set before restoreAfterEach's own snapshot, so each test's afterEach restores to this benign
// default rather than reintroducing the hang.
//
// P112: restoreAfterEach's own snapshot is taken *after* these two overrides (deliberately, so the
// per-test reset keeps the benign default rather than reintroducing the hang) — which means it
// never restores the *true* originals, and `control` is one process-wide singleton (restoreAfterEach.ts's
// own documented hazard: a spec's stub that outlives its file leaks into whatever runs next in the
// same bun test process). Captured here and restored in afterAll, so bridge-unwrap.spec.ts's
// generic "every control method rejects" check never sees this file's own defaults once this file's
// own tests are done.
const originalVariablesListEnvironments = control.variablesListEnvironments;
const originalCollectionsList = control.collectionsList;
const originalTabsSave = control.tabsSave;
afterAll(() => {
  control.tabsSave = originalTabsSave;
  control.variablesListEnvironments = originalVariablesListEnvironments;
  control.collectionsList = originalCollectionsList;
});
(
  control as unknown as { variablesListEnvironments: typeof control.variablesListEnvironments }
).variablesListEnvironments = async () => [];
(control as unknown as { collectionsList: typeof control.collectionsList }).collectionsList =
  async () => ({ collections: [], items: [] });
// Closing tabs on delete triggers tabsSave; unmocked it never settles and wedges the shared
// tabs store's saveChain for every later spec.
(control as unknown as { tabsSave: typeof control.tabsSave }).tabsSave = () => Promise.resolve();
restoreAfterEach(control);
const { useVariablesStore } = await import('../../frontend/src/api/state/variables');
const {
  apiVariablesKey,
  apiEnvironmentsKey,
  apiCollectionsTreeKey,
  loadVariableRows,
  reconcileEnvironments,
  reconcileTree,
  refreshApiQuery,
} = await import('../../frontend/src/api/state/apiQueries');
const { useTabIncognitoStore } = await import('../../frontend/src/state/tabIncognito');
const { useCollectionsStore } = await import('../../frontend/src/api/state/collections');

const variablesStore = useVariablesStore();
useCollectionsStore();
// Lets both stores' own eager initial fetch (above) actually settle before any test runs, so
// `refreshApiQuery`'s own in-flight capture never mistakes it for a test's own mocked fetch.
await new Promise((resolve) => setTimeout(resolve, 0));

function environment(id: string): ApiEnvironment {
  return {
    id,
    name: id,
    file: `${id}.yaml`,
    description: '',
    color: 'none',
    sortOrder: 0,
    isActive: false,
  };
}

function collection(id: string, name: string): CollectionSummary {
  return { id, name, file: `${id}.yaml`, sortOrder: 0, createdAt: '', updatedAt: '' };
}

function folder(id: string, collectionId: string): CollectionItemSummary {
  return {
    id,
    collectionId,
    parentId: null,
    kind: 'folder',
    name: id,
    sortOrder: 0,
    method: '',
    url: '',
    protocol: 'http',
    createdAt: '',
    updatedAt: '',
  };
}

function variable(id: string, ownerId: string): ApiVariable {
  return {
    id,
    scope: 'environment',
    ownerId,
    name: 'token',
    value: 'value',
    isSecret: false,
    sortOrder: 0,
    description: '',
  };
}

/** §3.5's reactivity rule: notifyManager batches with setTimeout(0), so a reactive reader
 *  (the `environments` computed, and the incognito-eviction watch over it) lags a resolved
 *  refetch by one macrotask — this waits that tick out. */
function flush(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

describe('a removed environment evicts the incognito override and the cached variables query', () => {
  test('an incognito tab pointing at the removed environment falls back to the app-wide selection', async () => {
    const tabId = 'tab-incognito-1';
    const envId = 'env-to-delete-1';
    // Seeds the environments list with the id about to be removed — TanStack's default structural
    // sharing would otherwise reuse the same (empty) array reference across the refresh below,
    // since an empty-to-empty transition has no content change for it to detect, and the
    // eviction watch (variables.ts) would then have nothing to react to.
    queryClient.setQueryData(apiEnvironmentsKey, [environment(envId)]);
    await flush();
    useTabIncognitoStore().setIncognito(tabId, true);
    await variablesStore.selectEnvironmentForTab(tabId, envId);
    expect(variablesStore.environmentIdForTab(tabId)).toBe(envId);

    (
      control as unknown as { variablesListEnvironments: typeof control.variablesListEnvironments }
    ).variablesListEnvironments = async () => [];

    await refreshApiQuery(apiEnvironmentsKey);
    reconcileEnvironments();
    await flush();

    // Falls back to the app-wide selection ('' — no environment active) rather than keeping the
    // removed id.
    expect(variablesStore.environmentIdForTab(tabId)).toBe('');
  });

  test('a removed environment evicts its cached variables query, not just the environments list', async () => {
    const envId = 'env-to-delete-2';
    (control as unknown as { variablesList: typeof control.variablesList }).variablesList =
      async () => [variable('v1', envId)];
    await loadVariableRows('environment', envId);
    expect(queryClient.getQueryData(apiVariablesKey('environment', envId))).toHaveLength(1);

    (
      control as unknown as { variablesListEnvironments: typeof control.variablesListEnvironments }
    ).variablesListEnvironments = async () => [];

    await refreshApiQuery(apiEnvironmentsKey);
    reconcileEnvironments();

    // reconcileEnvironments reads the query cache directly (an imperative reader, §3.5) —
    // no reactivity lag to wait out here.
    expect(queryClient.getQueryData(apiVariablesKey('environment', envId))).toBeUndefined();
    expect(queryClient.getQueryData<ApiEnvironment[]>(apiEnvironmentsKey)).toEqual([]);

    // A later loadVariableRows call for the same (now-evicted) id must re-fetch rather than
    // silently keep returning nothing forever because some stale cache entry still answers it.
    let calls = 0;
    (control as unknown as { variablesList: typeof control.variablesList }).variablesList =
      async () => {
        calls++;
        return [variable('v2', envId)];
      };
    const rows = await loadVariableRows('environment', envId);
    expect(calls).toBe(1);
    expect(rows).toHaveLength(1);
  });
});

describe('a removed collection evicts the collection-scope variables query (reconcileTree)', () => {
  test('a removed collection evicts its cached variables query, not just the tree', async () => {
    const collectionId = 'col-to-delete-1';
    queryClient.setQueryData(apiCollectionsTreeKey, {
      collections: [collection(collectionId, 'Orders API')],
      items: [folder('folder-1', collectionId)],
    });

    (control as unknown as { variablesList: typeof control.variablesList }).variablesList =
      async () => [variable('v1', collectionId)];
    await loadVariableRows('collection', collectionId);
    expect(queryClient.getQueryData(apiVariablesKey('collection', collectionId))).toHaveLength(1);

    (control as unknown as { collectionsList: typeof control.collectionsList }).collectionsList =
      async () => ({ collections: [], items: [] });

    await refreshApiQuery(apiCollectionsTreeKey);
    reconcileTree();

    // reconcileTree reads the query cache directly (an imperative reader, §3.5) — no reactivity
    // lag to wait out here.
    expect(queryClient.getQueryData(apiVariablesKey('collection', collectionId))).toBeUndefined();
    expect(
      queryClient.getQueryData<{ collections: CollectionSummary[] }>(apiCollectionsTreeKey)
        ?.collections,
    ).toEqual([]);
  });
});
