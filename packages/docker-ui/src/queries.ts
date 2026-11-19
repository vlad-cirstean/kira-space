import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query';
import { useDocumentVisibility, useLocalStorage } from '@vueuse/core';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { TIMING } from '@workbench/behaviour/timing';
import {
  computed,
  type MaybeRefOrGetter,
  onScopeDispose,
  type Ref,
  reactive,
  ref,
  toValue,
  watch,
} from 'vue';
import { useDocker } from './context';
import type {
  DockerDiskUsage,
  DockerRecreateArgs,
  DockerRestoreState,
  DockerStatus,
  DockerUpdateArgs,
  InspectKind,
  ResourceKind,
} from './wire';

const STATUS_KEY = ['docker', 'status'] as const;
/** Outside ['docker'] so refresh and live-sync invalidation do not run an engine check each time. */
const RESTORE_KEY = ['docker-restore'] as const;

export function useDockerStatus() {
  const { control } = useDocker();
  return useQuery({
    queryKey: STATUS_KEY,
    queryFn: () => control.status(false),
    refetchInterval: (query) => (query.state.data?.state === 'ok' ? false : TIMING.pollFast),
    staleTime: 0,
  });
}

/** Forces a fresh engine probe and publishes the result to the status cache. */
export function useRetryStatus() {
  const { control } = useDocker();
  const qc = useQueryClient();
  const { run, pending: retrying } = useAsyncAction(async () => {
    qc.setQueryData<DockerStatus>(STATUS_KEY, await control.status(true));
  });
  return { retrying, retry: run };
}

/** Scope for every resource query: a context switch must not serve the old engine's cache. */
function useScope() {
  const status = useDockerStatus();
  const scope = computed(() => {
    const ep = status.data.value?.endpoint;
    return ep ? `${ep.context}|${ep.host}` : '';
  });
  const ready = computed(() => status.data.value?.state === 'ok');
  return { status, scope, ready };
}

export function useContainers() {
  const { control } = useDocker();
  const { scope, ready } = useScope();
  return useQuery({
    queryKey: computed(() => ['docker', scope.value, 'containers'] as const),
    queryFn: () => control.containers(true),
    staleTime: 0,
    enabled: ready,
  });
}

export function useImages() {
  const { control } = useDocker();
  const { scope, ready } = useScope();
  return useQuery({
    queryKey: computed(() => ['docker', scope.value, 'images'] as const),
    queryFn: () => control.images(),
    staleTime: 0,
    enabled: ready,
  });
}

export function useVolumes() {
  const { control } = useDocker();
  const { scope, ready } = useScope();
  return useQuery({
    queryKey: computed(() => ['docker', scope.value, 'volumes'] as const),
    queryFn: () => control.volumes(),
    staleTime: 0,
    enabled: ready,
  });
}

export function useNetworks() {
  const { control } = useDocker();
  const { scope, ready } = useScope();
  return useQuery({
    queryKey: computed(() => ['docker', scope.value, 'networks'] as const),
    queryFn: () => control.networks(),
    staleTime: 0,
    enabled: ready,
  });
}

export function useContainerDetail(id: Ref<string>) {
  const { control } = useDocker();
  const { scope, ready } = useScope();
  return useQuery({
    queryKey: computed(() => ['docker', scope.value, 'container', id.value] as const),
    queryFn: () => control.inspectContainer(id.value),
    staleTime: 0,
    enabled: computed(() => ready.value && id.value !== ''),
  });
}

/** The registry page of one image; the client default never refetches it. */
function useImageRegistry(imageId: MaybeRefOrGetter<string>) {
  const { control } = useDocker();
  const { scope, ready } = useScope();
  return useQuery({
    queryKey: computed(() => ['docker', scope.value, 'image-registry', toValue(imageId)] as const),
    queryFn: () => control.imageRegistry(toValue(imageId)),
    enabled: computed(() => ready.value && toValue(imageId) !== ''),
  });
}

/**
 * The registry button of one image. An implied Docker Hub name is resolved only on click, so opening
 * a detail view sends no local image name to Docker Hub; `missing` is set when Hub has no page.
 */
export function useRegistryLink(imageId: MaybeRefOrGetter<string>) {
  const { control, openExternal } = useDocker();
  const registry = useImageRegistry(imageId);
  const missingFor = ref('');
  const confirm = useMutation({
    mutationFn: (id: string) => control.confirmImageRegistry(id),
  });
  const url = computed(() => registry.data.value?.url ?? '');
  const missing = computed(() => missingFor.value !== '' && missingFor.value === toValue(imageId));
  async function open(): Promise<void> {
    const id = toValue(imageId);
    missingFor.value = '';
    let target = url.value;
    if (registry.data.value?.unconfirmed) {
      try {
        target = (await confirm.mutateAsync(id)).url;
      } catch {
        target = '';
      }
    }
    if (target === '') {
      missingFor.value = id;
      return;
    }
    await openExternal(target);
  }
  return { url, missing, checking: computed(() => confirm.isPending.value), open };
}

export function useContainerEditSpec(id: Ref<string>) {
  const { control } = useDocker();
  const { scope, ready } = useScope();
  return useQuery({
    queryKey: computed(() => ['docker', scope.value, 'edit-spec', id.value] as const),
    queryFn: () => control.editSpec(id.value),
    staleTime: 0,
    enabled: computed(() => ready.value && id.value !== ''),
  });
}

/** In-place apply; a partial failure still refetches so the spec shows what landed. */
export function useUpdateContainer() {
  const { control } = useDocker();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (args: DockerUpdateArgs) => control.updateContainer(args),
    onSettled: () => qc.invalidateQueries({ queryKey: ['docker'] }),
  });
}

export function useRecreateContainer() {
  const { control } = useDocker();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (args: DockerRecreateArgs) => control.recreateContainer(args),
    onSettled: () => qc.invalidateQueries({ queryKey: ['docker'] }),
  });
}

export function useInspect(kind: Ref<InspectKind>, id: Ref<string>) {
  const { control } = useDocker();
  const { scope, ready } = useScope();
  return useQuery({
    queryKey: computed(() => ['docker', scope.value, 'inspect', kind.value, id.value] as const),
    queryFn: () => control.inspect(kind.value, id.value),
    staleTime: 0,
    enabled: computed(() => ready.value && id.value !== ''),
  });
}

const DISK_CACHE_MAX_SCOPES = 5;

/** `map` plus `usage` for `scope`, keeping only the newest scopes by `takenAt`. */
function pruneDiskCache(
  map: Record<string, DockerDiskUsage>,
  scope: string,
  usage: DockerDiskUsage,
): Record<string, DockerDiskUsage> {
  const at = (u: DockerDiskUsage) => Date.parse(u.takenAt) || 0;
  const newest = Object.entries({ ...map, [scope]: usage })
    .sort(([, a], [, b]) => at(b) - at(a))
    .slice(0, DISK_CACHE_MAX_SCOPES);
  return Object.fromEntries(newest);
}

const storedDisk = useLocalStorage<Record<string, DockerDiskUsage>>('kira.docker.diskUsage', {});

/**
 * Engine-wide disk usage, measured only on `refresh()`. Keys sit outside the `['docker']` prefix so
 * no refresh, container action or live-sync invalidation ever triggers a df walk. The last result
 * per engine persists in localStorage with its own timestamp.
 */
export function useEngineDiskUsage() {
  const { control } = useDocker();
  const { scope } = useScope();
  const query = useQuery({
    queryKey: computed(() => ['docker-disk', scope.value, 'engine'] as const),
    queryFn: () => control.diskUsage(),
    enabled: false,
    gcTime: Infinity,
    initialData: () => storedDisk.value[scope.value],
    initialDataUpdatedAt: () => {
      const at = Date.parse(storedDisk.value[scope.value]?.takenAt ?? '');
      return Number.isNaN(at) ? 0 : at;
    },
  });
  watch(query.data, (d) => {
    if (d && storedDisk.value[scope.value]?.takenAt !== d.takenAt) {
      storedDisk.value = pruneDiskCache(storedDisk.value, scope.value, d);
    }
  });
  return { query };
}

/** One container's size, measured only on `refresh()` and kept for the session, never persisted. */
export function useContainerSize(id: MaybeRefOrGetter<string>) {
  const { control } = useDocker();
  const { scope } = useScope();
  const query = useQuery({
    queryKey: computed(() => ['docker-disk', scope.value, 'container', toValue(id)] as const),
    queryFn: () => control.containerSize(toValue(id)),
    enabled: false,
    gcTime: Infinity,
  });
  return { query };
}

export type ContainerAction = 'start' | 'stop' | 'restart';

export function useContainerAction() {
  const { control } = useDocker();
  const qc = useQueryClient();
  const busy = reactive(new Set<string>());
  const mutation = useMutation({
    mutationFn: async ({ id, action }: { id: string; action: ContainerAction }) => {
      busy.add(id);
      try {
        await control[action](id);
      } finally {
        busy.delete(id);
      }
    },
    onSettled: () => qc.invalidateQueries({ queryKey: ['docker'] }),
  });
  return {
    run: (id: string, action: ContainerAction) => mutation.mutateAsync({ id, action }),
    isBusy: (id: string) => busy.has(id),
    error: mutation.error,
    reset: mutation.reset,
  };
}

/** The restart-restore state; the engine is checked on mount, pushes keep it current. */
export function useRestoreState() {
  const { control } = useDocker();
  return useQuery({
    queryKey: RESTORE_KEY,
    queryFn: () => control.restoreState(),
    staleTime: 0,
    refetchOnMount: 'always',
  });
}

/** Re-reads the restore state after an action so the banner follows what the engine now holds. */
function useSettleRestore() {
  const { control } = useDocker();
  const qc = useQueryClient();
  return async () => {
    qc.setQueryData<DockerRestoreState>(RESTORE_KEY, await control.restoreState());
    void qc.invalidateQueries({ queryKey: ['docker'] });
  };
}

export function useRestoreContainers() {
  const { control } = useDocker();
  const settle = useSettleRestore();
  return useMutation({
    mutationFn: (args: { engineId: string; ids: string[] }) =>
      control.restoreContainers(args.engineId, args.ids),
    onSettled: settle,
  });
}

export function useDismissRestore() {
  const { control } = useDocker();
  const settle = useSettleRestore();
  return useMutation({
    mutationFn: (engineId: string) => control.restoreDismiss(engineId),
    onSettled: settle,
  });
}

function invalidateKinds(qc: ReturnType<typeof useQueryClient>, kinds: readonly ResourceKind[]) {
  const wanted = new Set<string>();
  for (const k of kinds) {
    if (k === 'container') {
      for (const w of ['containers', 'container', 'edit-spec', 'images', 'volumes', 'networks']) {
        wanted.add(w);
      }
    } else {
      wanted.add(`${k}s`);
      wanted.add('inspect');
      if (k === 'network') wanted.add('edit-spec');
    }
  }
  void qc.invalidateQueries({
    predicate: (q) => q.queryKey[0] === 'docker' && wanted.has(String(q.queryKey[2])),
  });
}

function liveSyncBestEffort(): void {
  // Live sync is best effort; status polling and refetch on show still keep lists current.
}

/** While mounted and the document is visible: engine events invalidate lists, status pushes land
 *  in the cache. Hidden or unmounted: the Go watcher is released. */
export function useDockerLiveSync() {
  const { control } = useDocker();
  const qc = useQueryClient();
  const visibility = useDocumentVisibility();
  let offs: Array<() => void> = [];
  let wasHidden = false;

  function start(): void {
    if (offs.length > 0) return;
    offs = [
      control.onChanged((e) => invalidateKinds(qc, e.kinds)),
      control.onStatus((s) => qc.setQueryData<DockerStatus>(STATUS_KEY, s)),
      control.onRestore((s) => qc.setQueryData<DockerRestoreState>(RESTORE_KEY, s)),
    ];
    control.watch().catch(liveSyncBestEffort);
  }

  function stop(): void {
    if (offs.length === 0) return;
    for (const off of offs) off();
    offs = [];
    control.unwatch().catch(liveSyncBestEffort);
  }

  watch(
    visibility,
    (v) => {
      if (v === 'visible') {
        start();
        if (wasHidden) void qc.invalidateQueries({ queryKey: ['docker'] });
        wasHidden = false;
      } else {
        wasHidden = true;
        stop();
      }
    },
    { immediate: true },
  );
  onScopeDispose(stop);
}
