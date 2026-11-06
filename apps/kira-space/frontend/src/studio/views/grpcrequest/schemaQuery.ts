import { loadDynamicGenerator } from '@kira/api-core';
import type { GrpcMetadataState, GrpcMetaPairWire, GrpcSchemaWire } from '@shared/domain/grpc';
import { newUuid } from '@shared/ids';
import { useQuery } from '@tanstack/vue-query';
import { refDebounced } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { KEEP_WHILE_OPEN, queryClient } from '@workbench/state/queryClient';
import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';
import { computed, type MaybeRefOrGetter, toValue, watch } from 'vue';
import { loadVariableRows } from '../../api/state/apiQueries';
import { useCollectionsStore } from '../../api/state/collections';
import { mergeVariableRows, useVariablesStore } from '../../api/state/variables';
import { control } from '../../bridge/control';
import type { GrpcRequestTabRecord } from '../../state/tabDomain';
import { resolveGrpcTabState } from './resolve';

// The resolved schema for a request's
// *source* — never for a tab. Reflection keys carry the raw `target` template (never a resolved
// value, so no secret enters a cache key) plus the owner ids that give the template its meaning;
// Go's descriptor cache (grpcclient/descriptors.go cacheKey) hashes the resolved target instead, so
// a variables change invalidates every reflection key (apiQueries.ts handleApiDataChange). Metadata
// *values* are read at fetch time, because Go ignores them on a non-reload Describe.

export interface GrpcSchemaSource {
  mode: 'reflection' | 'proto';
  target: string;
  tlsEnabled: boolean;
  caFile: string;
  serverName: string;
  protoPath: string;
  importPaths: string[];
  metadata: GrpcMetadataState[];
  collectionId: string;
  environmentId: string;
  /** False while a saved tab's collection id is unknown (tree not loaded): fetching then would key
   *  on '' and refetch the moment the tree lands. */
  ready: boolean;
}

export function grpcSchemaSourceFor(tab: GrpcRequestTabRecord): GrpcSchemaSource {
  const { state } = tab;
  const collections = useCollectionsStore();
  return {
    mode: state.descriptorMode,
    target: state.target,
    tlsEnabled: state.tlsMode === 'tls',
    caFile: state.caFile,
    serverName: state.serverName,
    protoPath: state.protoPath,
    importPaths: state.importPaths,
    metadata: state.metadata,
    collectionId: collections.collectionIdFor(state),
    environmentId: useVariablesStore().environmentIdForTab(tab.id),
    ready: state.itemId === null || collections.loaded,
  };
}

export function grpcSchemaKey(source: GrpcSchemaSource) {
  if (source.mode === 'proto') {
    return ['grpcSchema', 'proto', source.protoPath, source.importPaths.join('\0')] as const;
  }
  const names = source.metadata
    .filter((m) => m.enabled && m.name.trim() !== '')
    .map((m) => m.name)
    .join('\0');
  return [
    'grpcSchema',
    'reflection',
    source.target,
    source.tlsEnabled,
    source.caFile,
    source.serverName,
    names,
    source.collectionId,
    source.environmentId,
  ] as const;
}

function isComplete(source: GrpcSchemaSource): boolean {
  return source.mode === 'reflection' ? source.target !== '' : source.protoPath !== '';
}

/** Describe has no message field, so only target and metadata are resolved (stage 1, as send()). */
async function describeSchema(
  source: GrpcSchemaSource,
  reload: boolean,
  signal?: AbortSignal,
): Promise<GrpcSchemaWire> {
  let target = source.target;
  let metadata: GrpcMetaPairWire[] = [];
  if (source.mode === 'reflection') {
    const [collectionRows, environmentRows] = await Promise.all([
      loadVariableRows('collection', source.collectionId),
      loadVariableRows('environment', source.environmentId),
    ]);
    const { values, secretNames } = mergeVariableRows(collectionRows, environmentRows);
    const fields = { target: source.target, metadata: source.metadata, message: '' };
    const first = resolveGrpcTabState(fields, values, secretNames);
    const resolved = first.refs.some((r) => r.kind === 'dynamic')
      ? resolveGrpcTabState(fields, values, secretNames, await loadDynamicGenerator())
      : first;
    target = resolved.target;
    metadata = resolved.metadata;
  }
  // A listener added after an abort never fires, so bail out before dialling.
  signal?.throwIfAborted();
  const opId = newUuid();
  signal?.addEventListener(
    'abort',
    () => fireAndForget(control.opsCancel(opId), 'cancelSchemaQuery'),
    { once: true },
  );
  return control.grpcDescribe({
    descriptorMode: source.mode,
    target,
    tls: { enabled: source.tlsEnabled, caFile: source.caFile, serverName: source.serverName },
    metadata,
    protoPath: source.protoPath,
    importPaths: source.importPaths,
    collectionId: source.collectionId,
    environmentId: source.environmentId,
    reload,
    opId,
  });
}

// Schemas are kept while their tab is open (a tab switch or 5 min away refetches nothing). A key
// belongs to the source, so it goes when its tab leaves that source or closes, unless another
// observer still reads it.
const keyByTab = new Map<string, readonly unknown[]>();

function dropSchemaKey(key: readonly unknown[]): void {
  const query = queryClient.getQueryCache().find({ queryKey: key, exact: true });
  if (query && query.getObserversCount() === 0) {
    queryClient.removeQueries({ queryKey: key, exact: true });
  }
}

registerTabRuntimeCleanup((tabId) => {
  const key = keyByTab.get(tabId);
  keyByTab.delete(tabId);
  if (key) dropSchemaKey(key);
});

/** The tab's schema, fetched once its source is complete; debounced like a fast typist (the
 *  schema watcher this replaces). A source change is a new query — nothing to supersede. */
export function useGrpcSchema(tab: MaybeRefOrGetter<GrpcRequestTabRecord>) {
  const source = computed(() => grpcSchemaSourceFor(toValue(tab)));
  const debounced = refDebounced(source, TIMING.search);
  const query = useQuery(() => {
    const s = debounced.value;
    return {
      queryKey: grpcSchemaKey(s),
      queryFn: ({ signal }) => describeSchema(s, false, signal),
      enabled: s.ready && isComplete(s),
      gcTime: KEEP_WHILE_OPEN,
    };
  }, queryClient);
  watch(
    () => JSON.stringify(grpcSchemaKey(debounced.value)),
    () => {
      const id = toValue(tab).id;
      const prev = keyByTab.get(id);
      const key = grpcSchemaKey(debounced.value);
      keyByTab.set(id, key);
      if (prev && JSON.stringify(prev) !== JSON.stringify(key)) dropSchemaKey(prev);
    },
    { immediate: true, flush: 'post' },
  );

  /** Go invalidates its own descriptor cache on `reload`; the reply replaces this key's data. A
   *  failure lands in the query's own error state. */
  async function reload(): Promise<void> {
    const s = debounced.value;
    await queryClient
      .fetchQuery({
        queryKey: grpcSchemaKey(s),
        queryFn: ({ signal }) => describeSchema(s, true, signal),
        staleTime: 0,
      })
      .catch(() => {
        // the failure lands in the query's own error state
      });
  }

  return {
    schema: computed(() => query.data.value ?? null),
    loading: computed(() => query.isFetching.value),
    error: computed(() => query.error.value?.message ?? null),
    reload,
  };
}
