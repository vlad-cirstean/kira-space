// The module owning the collections and variables server-state caches (jar cookies, history
// and gRPC schema live in views/httprequest/cookies.ts, api/state/history.ts and
// views/grpcrequest/schemaQuery.ts) — keys, query options, the
// imperative loaders non-component code uses, the one invalidation helper (refreshApiQuery)
// and the boot-time listener that turns a Go `kira:api:dataChanged` broadcast
// into cache invalidation (initApiDataSync). Precedents: state/schemas.ts (options object,
// never stale because Go pushes, broadcast → invalidateQueries) and state/maskRules.ts.
//
// Query keys follow the flat `[domain, ...ids]` convention (workbench/state/queryClient.ts).
//
// ---- reactivity rule — two reader kinds ----
// notifyManager batches observer notifications on a zero-delay timer, and vue-query never swaps the
// scheduler — a useQuery result's `.data` lags the cache by one macrotask after a fetch resolves.
//   - Reactive readers (computeds, templates, watches) read a useQuery result's `data`.
//   - Imperative code after an `await` (a mutation continuing on, send(), the reconcilers below)
//     reads `queryClient.getQueryData(key)` or awaits a `load*()` reader — never the observer ref.

import type { ApiConfigView } from '@shared/domain/apiConfig';
import type {
  CollectionItemSummary,
  CollectionSummary,
  GrpcSavedRequest,
  HttpSavedRequest,
} from '@shared/domain/collections';
import { grpcSavedRequestSchema, httpSavedRequestSchema } from '@shared/domain/collections';
import type { ApiEnvironment, ApiVariable, VariableScope } from '@shared/domain/variables';
import type { ApiDataChange } from '@shared/protocol/events';
import type { QueryKey } from '@tanstack/vue-query';
import { useQuery } from '@tanstack/vue-query';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { KEEP_WHILE_OPEN, queryClient } from '@workbench/state/queryClient';
import type { MaybeRefOrGetter } from 'vue';
import { toValue } from 'vue';
import { control } from '../../bridge/control';

// ---- keys ----

export const apiCollectionsTreeKey = ['apiCollectionsTree'] as const;
export function apiSavedRequestKey(itemId: string) {
  return ['apiSavedRequest', itemId] as const;
}
export function apiSavedGrpcRequestKey(itemId: string) {
  return ['apiSavedGrpcRequest', itemId] as const;
}
export const apiEnvironmentsKey = ['apiEnvironments'] as const;
export const apiConfigKey = ['apiConfig'] as const;
export function apiVariablesKey(scope: VariableScope, ownerId: string) {
  return ['apiVariables', scope, ownerId] as const;
}

// ---- query options ----

export interface ApiCollectionsTree {
  collections: CollectionSummary[];
  items: CollectionItemSummary[];
}

export function apiCollectionsTreeQueryOptions() {
  return {
    queryKey: apiCollectionsTreeKey,
    gcTime: KEEP_WHILE_OPEN,
    queryFn: async (): Promise<ApiCollectionsTree> => {
      const { collections, items } = await control.collectionsList();
      return { collections, items: items as CollectionItemSummary[] };
    },
  };
}

// `null` means "confirmed orphan" (GetRequest answered E_NOT_FOUND — the row was deleted, in this
// window or another); `undefined` (no data) means "not loaded yet". Keeps the
// orphan/not-loaded split exactly, without two parallel maps. Any other failure rethrows, so a
// transient error never caches as "deleted" under the never-stale default.
function nullOnNotFound(err: unknown): null {
  if ((err as { code?: string }).code === 'E_NOT_FOUND') return null;
  throw err;
}

function apiSavedRequestQueryOptions(itemId: string) {
  return {
    queryKey: apiSavedRequestKey(itemId),
    gcTime: KEEP_WHILE_OPEN,
    queryFn: async (): Promise<HttpSavedRequest | null> => {
      try {
        return httpSavedRequestSchema.parse(await control.collectionsGetRequest(itemId));
      } catch (err) {
        return nullOnNotFound(err);
      }
    },
  };
}

/** apiSavedRequestQueryOptions' own gRPC sibling. */
function apiSavedGrpcRequestQueryOptions(itemId: string) {
  return {
    queryKey: apiSavedGrpcRequestKey(itemId),
    gcTime: KEEP_WHILE_OPEN,
    queryFn: async (): Promise<GrpcSavedRequest | null> => {
      try {
        return grpcSavedRequestSchema.parse(await control.collectionsGetGrpcRequest(itemId));
      } catch (err) {
        return nullOnNotFound(err);
      }
    },
  };
}

export function apiEnvironmentsQueryOptions() {
  return {
    queryKey: apiEnvironmentsKey,
    gcTime: KEEP_WHILE_OPEN,
    queryFn: (): Promise<ApiEnvironment[]> => control.variablesListEnvironments(),
  };
}

export function apiConfigQueryOptions() {
  return {
    queryKey: apiConfigKey,
    gcTime: KEEP_WHILE_OPEN,
    queryFn: (): Promise<ApiConfigView> => control.apiConfigView(),
  };
}

function apiVariablesQueryOptions(scope: VariableScope, ownerId: string) {
  return {
    queryKey: apiVariablesKey(scope, ownerId),
    gcTime: KEEP_WHILE_OPEN,
    queryFn: (): Promise<ApiVariable[]> => control.variablesList(scope, ownerId),
  };
}

// ---- imperative loaders (non-component code) ----
//
// Cache-first, refetch when invalidated — exactly queryClient.query's own contract with
// the never-stale default. fetchQuery/ensureQueryData are deprecated in this app's TanStack Query
// version; query() is their replacement (query-core@5.103.2's own queryClient.js).

export function loadCollectionsTree(): Promise<ApiCollectionsTree> {
  return queryClient.query(apiCollectionsTreeQueryOptions());
}

export function loadEnvironments(): Promise<ApiEnvironment[]> {
  return queryClient.query(apiEnvironmentsQueryOptions());
}

/** Returns `[]` for `ownerId === ''` with no fetch — preserves ensureVariablesLoaded's own '' no-op
 *  (a scratch tab's collection, or no active environment). */
export function loadVariableRows(scope: VariableScope, ownerId: string): Promise<ApiVariable[]> {
  if (ownerId === '') return Promise.resolve([]);
  return queryClient.query(apiVariablesQueryOptions(scope, ownerId));
}

export function loadSavedRequest(itemId: string): Promise<HttpSavedRequest | null> {
  return queryClient.query(apiSavedRequestQueryOptions(itemId));
}

export function loadSavedGrpcRequest(itemId: string): Promise<GrpcSavedRequest | null> {
  return queryClient.query(apiSavedGrpcRequestQueryOptions(itemId));
}

// ---- refreshApiQuery — the one invalidation helper ----
//
// Every invalidation goes through this: the event handler below and every mutation's onSuccess
// where a re-list is needed.
//
// The second invalidate is the race, restated for TanStack: a fetch that was already running
// when a change landed is never trusted as the post-change answer (query.fetch joins an in-flight
// fetch with no data yet rather than cancelling it, and invalidateQueries only refetches an
// `active` query by default — an observer-less in-flight `queryClient.query` caller, e.g. send(),
// would otherwise have its own invalidation cleared by that in-flight fetch landing afterward).
// Worst case here is one extra local SQLite read.
export async function refreshApiQuery(queryKey: QueryKey): Promise<void> {
  const inFlight = queryClient.getQueryCache().find({ queryKey, exact: true })?.promise;
  await queryClient.invalidateQueries({ queryKey, exact: true });
  if (inFlight) {
    // Settling is all that matters; the fetch's own caller reports a failure.
    await inFlight.then(
      () => undefined,
      () => undefined,
    );
    await queryClient.invalidateQueries({ queryKey, exact: true });
  }
}

// ---- reconcilers — F9's successor: cross-window-aware cache eviction ----

/** After the tree refreshes: drop any cached saved-request query whose item no longer exists (the
 *  tab reads as orphan — D14's rule, now cross-window and cascade-aware for free), and evict any
 *  cached collection-scope variables query whose collection no longer exists. */
export function reconcileTree(): void {
  const tree = queryClient.getQueryData<ApiCollectionsTree>(apiCollectionsTreeKey);
  if (!tree) return;
  const itemIds = new Set(tree.items.map((item) => item.id));
  const collectionIds = new Set(tree.collections.map((c) => c.id));

  for (const query of queryClient.getQueryCache().findAll({ queryKey: ['apiSavedRequest'] })) {
    const itemId = query.queryKey[1] as string;
    if (!itemIds.has(itemId)) queryClient.setQueryData(query.queryKey, null);
  }
  for (const query of queryClient.getQueryCache().findAll({ queryKey: ['apiSavedGrpcRequest'] })) {
    const itemId = query.queryKey[1] as string;
    if (!itemIds.has(itemId)) queryClient.setQueryData(query.queryKey, null);
  }
  // Item-scope history lists (`[domain, itemId, '']`) live until their item goes.
  for (const domain of ['httpHistory', 'grpcHistory']) {
    for (const query of queryClient.getQueryCache().findAll({ queryKey: [domain] })) {
      const itemId = query.queryKey[1] as string;
      if (itemId !== '' && !itemIds.has(itemId)) {
        queryClient.removeQueries({ queryKey: query.queryKey, exact: true });
      }
    }
  }
  for (const query of queryClient
    .getQueryCache()
    .findAll({ queryKey: ['apiVariables', 'collection'] })) {
    const collectionId = query.queryKey[2] as string;
    if (!collectionIds.has(collectionId)) {
      queryClient.removeQueries({ queryKey: query.queryKey, exact: true });
    }
  }
}

/** After the environments list refreshes: evict any cached environment-scope variables query
 *  whose environment no longer exists. */
export function reconcileEnvironments(): void {
  const list = queryClient.getQueryData<ApiEnvironment[]>(apiEnvironmentsKey);
  if (!list) return;
  const ids = new Set(list.map((e) => e.id));
  for (const query of queryClient
    .getQueryCache()
    .findAll({ queryKey: ['apiVariables', 'environment'] })) {
    const environmentId = query.queryKey[2] as string;
    if (!ids.has(environmentId)) {
      queryClient.removeQueries({ queryKey: query.queryKey, exact: true });
    }
  }
}

// ---- the listener ----

async function handleApiDataChange(change: ApiDataChange): Promise<void> {
  switch (change.kind) {
    case 'tree':
      await refreshApiQuery(apiCollectionsTreeKey);
      reconcileTree();
      return;
    case 'savedRequest':
      // Only one of the two exists for a given itemId; the other refresh is a no-op.
      await Promise.all([
        refreshApiQuery(apiSavedRequestKey(change.itemId)),
        refreshApiQuery(apiSavedGrpcRequestKey(change.itemId)),
      ]);
      return;
    case 'variables':
      await refreshApiQuery(apiVariablesKey(change.scope, change.ownerId));
      // A reflection schema is keyed by the unresolved target, so a variable edit can change its host.
      await queryClient.invalidateQueries({ queryKey: ['grpcSchema', 'reflection'] });
      return;
    case 'environments':
      await refreshApiQuery(apiEnvironmentsKey);
      reconcileEnvironments();
      return;
    case 'config':
      await refreshApiQuery(apiConfigKey);
      return;
  }
}

let unsubscribeApiDataChanged: (() => void) | null = null;

/** Called once in main.ts's bootstrap(), synchronously, before its Promise.all — it needs no data,
 *  and it must be live before any query exists, whether or not the Api panel ever mounts. Same
 *  unsubscribe?.() then resubscribe shape as state/schemas.ts's initSchemaSync. */
export function initApiDataSync(): void {
  unsubscribeApiDataChanged?.();
  unsubscribeApiDataChanged = control.onApiDataChanged((event) => {
    // A broadcast has no user action to attach an error strip to: log only.
    fireAndForget(Promise.all(event.changes.map(handleApiDataChange)), 'apiDataChanged');
  });
}

// ---- composables (components only — dynamic keys, so not observed from a store) ----

export function useVariableRows(scope: VariableScope, ownerId: MaybeRefOrGetter<string>) {
  return useQuery(
    () => ({
      ...apiVariablesQueryOptions(scope, toValue(ownerId)),
      enabled: toValue(ownerId) !== '',
    }),
    queryClient,
  );
}

export function useSavedRequest(itemId: MaybeRefOrGetter<string | null>) {
  return useQuery(
    () => ({
      ...apiSavedRequestQueryOptions(toValue(itemId) ?? ''),
      enabled: !!toValue(itemId),
    }),
    queryClient,
  );
}

/** useSavedRequest's own gRPC sibling. */
export function useSavedGrpcRequest(itemId: MaybeRefOrGetter<string | null>) {
  return useQuery(
    () => ({
      ...apiSavedGrpcRequestQueryOptions(toValue(itemId) ?? ''),
      enabled: !!toValue(itemId),
    }),
    queryClient,
  );
}
