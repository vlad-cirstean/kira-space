import {
  type QueryClient,
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from '@tanstack/vue-query';
import { useDocumentVisibility } from '@vueuse/core';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { TIMING } from '@workbench/behaviour/timing';
import { computed, type MaybeRefOrGetter, toValue } from 'vue';
import { kubeControl } from './control';
import type { KubeClusters, KubeMcpSettings, KubeStatus } from './wire';

const LIST_POLL = TIMING.pollFast * 2;
const LIST_POLL_LARGE = TIMING.pollSlow / 2;
const LARGE_LIST_ROWS = 1000;

const kubeKeys = {
  clusters: ['kubernetes', 'clusters'] as const,
  cluster: (id: string) => ['kubernetes', id] as const,
  status: (id: string) => ['kubernetes', id, 'status'] as const,
  namespaces: (id: string) => ['kubernetes', id, 'namespaces'] as const,
  kinds: (id: string) => ['kubernetes', id, 'kinds'] as const,
};

/** A refetch of loaded data is background work: the backend keeps it out of the Operations panel. */
const isRefetch = (qc: QueryClient, key: readonly unknown[]) => qc.getQueryData(key) !== undefined;

export function useClusters() {
  return useQuery({ queryKey: kubeKeys.clusters, queryFn: kubeControl.clusters });
}

const statusOptions = (qc: QueryClient, id: string) => ({
  queryKey: kubeKeys.status(id),
  queryFn: () => kubeControl.status(id, false, isRefetch(qc, kubeKeys.status(id))),
  staleTime: TIMING.pollSlow,
});

const kindsOptions = (qc: QueryClient, id: string) => ({
  queryKey: kubeKeys.kinds(id),
  queryFn: () => kubeControl.kinds(id, isRefetch(qc, kubeKeys.kinds(id))),
  staleTime: TIMING.pollSlow,
});

/** Probe/discovery for every listed cluster; only the ones in `active` ever connect. */
export function useClusterProbes(
  ids: MaybeRefOrGetter<readonly string[]>,
  active: MaybeRefOrGetter<readonly string[]>,
) {
  const qc = useQueryClient();
  const statuses = useQueries({
    queries: computed(() =>
      toValue(ids).map((id) => ({
        ...statusOptions(qc, id),
        enabled: toValue(active).includes(id),
      })),
    ),
  });
  const kinds = useQueries({
    queries: computed(() =>
      toValue(ids).map((id) => ({
        ...kindsOptions(qc, id),
        enabled: toValue(active).includes(id),
      })),
    ),
  });
  return { statuses, kinds };
}

export function useClusterStatus(id: MaybeRefOrGetter<string>) {
  const qc = useQueryClient();
  return useQuery({
    queryKey: computed(() => kubeKeys.status(toValue(id))),
    queryFn: () =>
      kubeControl.status(toValue(id), false, isRefetch(qc, kubeKeys.status(toValue(id)))),
    staleTime: TIMING.pollSlow,
    enabled: computed(() => toValue(id) !== ''),
  });
}

export function useKinds(id: MaybeRefOrGetter<string>) {
  const qc = useQueryClient();
  return useQuery({
    queryKey: computed(() => kubeKeys.kinds(toValue(id))),
    queryFn: () => kubeControl.kinds(toValue(id), isRefetch(qc, kubeKeys.kinds(toValue(id)))),
    staleTime: TIMING.pollSlow,
    enabled: computed(() => toValue(id) !== ''),
  });
}

export function useNamespaces(id: MaybeRefOrGetter<string>, enabled: MaybeRefOrGetter<boolean>) {
  const qc = useQueryClient();
  return useQuery({
    queryKey: computed(() => kubeKeys.namespaces(toValue(id))),
    queryFn: () =>
      kubeControl.namespaces(toValue(id), isRefetch(qc, kubeKeys.namespaces(toValue(id)))),
    staleTime: TIMING.pollSlow,
    enabled: computed(() => toValue(id) !== '' && toValue(enabled)),
  });
}

export function useResourceList(args: {
  clusterId: MaybeRefOrGetter<string>;
  kind: MaybeRefOrGetter<string>;
  namespace: MaybeRefOrGetter<string>;
  labelSelector: MaybeRefOrGetter<string>;
  enabled: MaybeRefOrGetter<boolean>;
}) {
  const visibility = useDocumentVisibility();
  const qc = useQueryClient();
  const queryKey = computed(() => [
    'kubernetes',
    toValue(args.clusterId),
    'list',
    toValue(args.kind),
    toValue(args.namespace),
    toValue(args.labelSelector),
  ]);
  return useQuery({
    queryKey,
    queryFn: () =>
      kubeControl.list(
        toValue(args.clusterId),
        toValue(args.kind),
        toValue(args.namespace),
        toValue(args.labelSelector),
        isRefetch(qc, queryKey.value),
      ),
    refetchInterval: (query) => {
      if (visibility.value !== 'visible') return false;
      const data = query.state.data;
      return data && (data.truncated || data.rows.length > LARGE_LIST_ROWS)
        ? LIST_POLL_LARGE
        : LIST_POLL;
    },
    enabled: computed(() => toValue(args.clusterId) !== '' && toValue(args.enabled)),
  });
}

export function useDescribe(args: {
  clusterId: MaybeRefOrGetter<string>;
  kind: MaybeRefOrGetter<string>;
  namespace: MaybeRefOrGetter<string>;
  name: MaybeRefOrGetter<string>;
}) {
  const qc = useQueryClient();
  const queryKey = computed(() => [
    'kubernetes',
    toValue(args.clusterId),
    'describe',
    toValue(args.kind),
    toValue(args.namespace),
    toValue(args.name),
  ]);
  return useQuery({
    queryKey,
    queryFn: () =>
      kubeControl.describe(
        toValue(args.clusterId),
        toValue(args.kind),
        toValue(args.namespace),
        toValue(args.name),
        isRefetch(qc, queryKey.value),
      ),
  });
}

/** Reconnects one cluster (fresh client) and republishes its status; drops its cached reads. */
export function useRefreshCluster() {
  const qc = useQueryClient();
  return useAsyncAction(async (id: string) => {
    const status = await kubeControl.status(id, true, false);
    await qc.invalidateQueries({ queryKey: kubeKeys.cluster(id) });
    qc.setQueryData<KubeStatus>(kubeKeys.status(id), status);
  });
}

/** Saves one cluster's MCP settings and patches the clusters cache with what the backend stored. */
export function useSetClusterMcp() {
  const qc = useQueryClient();
  return useMutation({
    mutationKey: ['kubernetes', 'setMcp'],
    mutationFn: (v: { clusterId: string; mcp: KubeMcpSettings }) =>
      kubeControl.setMcp(v.clusterId, v.mcp),
    onSuccess: (mcp, v) => {
      qc.setQueryData<KubeClusters>(kubeKeys.clusters, (old) =>
        old
          ? {
              ...old,
              clusters: old.clusters.map((c) => (c.id === v.clusterId ? { ...c, mcp } : c)),
            }
          : old,
      );
      void qc.invalidateQueries({ queryKey: kubeKeys.clusters });
    },
  });
}
