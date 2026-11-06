import { usePersistedState } from '@workbench/behaviour/persisted';
import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import { z } from 'zod';
import { POD_KIND } from './kinds';

const RECENT_MAX = 3;

interface PerCluster {
  kind: string;
  namespace: string | null;
  recent: string[];
}

interface Remembered {
  clusterId: string;
  perCluster: Record<string, PerCluster>;
}

const EMPTY: Remembered = { clusterId: '', perCluster: {} };

const rememberedSchema = z.object({
  clusterId: z.string(),
  perCluster: z.record(
    z.string(),
    z.object({
      kind: z.string(),
      namespace: z.string().nullable(),
      recent: z.array(z.string()).max(RECENT_MAX).default([]),
    }),
  ),
});

export const useKubernetesUiStore = defineStore('kubernetesUi', () => {
  const remembered = usePersistedState<Remembered>('kira.kubernetes.selection', EMPTY, {
    schema: rememberedSchema,
  });
  const listWidth = usePersistedState('kira.kubernetes.listWidth', 440);
  const listHidden = usePersistedState('kira.kubernetes.listHidden', false);

  const clusterId = computed(() => remembered.value.clusterId);
  /** Empty until the user picks one; the browser then shows the default kind. */
  const kind = computed(() => remembered.value.perCluster[clusterId.value]?.kind ?? '');
  const recent = computed(() => remembered.value.perCluster[clusterId.value]?.recent ?? []);
  /** Null until the user picks one; the browser then falls back to the context's namespace. */
  const namespace = computed(() => remembered.value.perCluster[clusterId.value]?.namespace ?? null);

  const labelSelector = ref('');
  const listFilter = ref('');
  const clusterSearch = ref('');

  function remember(id: string, patch: Partial<PerCluster>): void {
    const prev = remembered.value.perCluster[id] ?? { kind: '', namespace: null, recent: [] };
    remembered.value = {
      clusterId: remembered.value.clusterId,
      perCluster: { ...remembered.value.perCluster, [id]: { ...prev, ...patch } },
    };
  }

  function resetView(): void {
    labelSelector.value = '';
    listFilter.value = '';
  }

  function selectCluster(id: string): void {
    if (id === clusterId.value) return;
    remembered.value = { ...remembered.value, clusterId: id };
    resetView();
  }

  function selectKind(key: string): void {
    const changed = key !== kind.value;
    const patch: Partial<PerCluster> = { kind: key };
    if (key !== POD_KIND && !recent.value.includes(key)) {
      patch.recent = [...recent.value, key].slice(-RECENT_MAX);
    }
    remember(clusterId.value, patch);
    if (changed) resetView();
  }

  function setNamespace(next: string | null): void {
    if (clusterId.value === '') return;
    remember(clusterId.value, { namespace: next });
  }

  return {
    clusterId,
    kind,
    recent,
    namespace,
    labelSelector,
    listFilter,
    clusterSearch,
    listWidth,
    listHidden,
    selectCluster,
    selectKind,
    setNamespace,
  };
});
