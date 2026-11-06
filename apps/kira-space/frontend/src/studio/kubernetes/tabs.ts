import { newUuid } from '@shared/ids';
import type { TabLike } from '@workbench/host';
import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import type { KubeKind } from './wire';

export interface KubeResourceRef {
  clusterId: string;
  /** Shown in the breadcrumb and the collision title. */
  context: string;
  /** Snapshot at open: the tab never needs the Kinds query. */
  kind: KubeKind;
  /** Empty for a cluster-scoped kind. */
  namespace: string;
  name: string;
}

export type KubeTabView = 'describe' | 'logs';

export interface KubeTab extends TabLike {
  kind: 'resource';
  state: { ref: KubeResourceRef; view: KubeTabView };
}

type StoredTab = Omit<KubeTab, 'active'>;

export const refKey = (r: KubeResourceRef): string =>
  `${r.clusterId}|${r.kind.key}|${r.namespace}|${r.name}`;

/** Open resource tabs of this window. In memory only: pod names are ephemeral, so no restore. */
export const useKubeTabsStore = defineStore('kubeTabs', () => {
  const list = ref<StoredTab[]>([]);
  const activeId = ref<string | null>(null);
  const previewId = ref<string | null>(null);

  const tabs = computed<KubeTab[]>(() =>
    list.value.map((t) => ({ ...t, active: t.id === activeId.value })),
  );
  const count = computed(() => list.value.length);
  const activeTab = computed(() => tabs.value.find((t) => t.active) ?? null);

  const indexOf = (id: string): number => list.value.findIndex((t) => t.id === id);

  function open(r: KubeResourceRef, opts: { preview: boolean; view?: KubeTabView }): void {
    const key = refKey(r);
    const existing = list.value.find((t) => refKey(t.state.ref) === key);
    if (existing) {
      activeId.value = existing.id;
      if (!opts.preview && previewId.value === existing.id) previewId.value = null;
      if (opts.view) existing.state = { ...existing.state, view: opts.view };
      return;
    }
    const tab: StoredTab = {
      id: `kube-tab:${newUuid()}`,
      kind: 'resource',
      state: { ref: r, view: opts.view ?? 'describe' },
    };
    const at = previewId.value === null ? -1 : indexOf(previewId.value);
    if (opts.preview && at >= 0) {
      list.value.splice(at, 1, tab);
    } else {
      const after = activeId.value === null ? -1 : indexOf(activeId.value);
      list.value.splice(after < 0 ? list.value.length : after + 1, 0, tab);
    }
    previewId.value = opts.preview ? tab.id : previewId.value;
    activeId.value = tab.id;
  }

  const activate = (id: string): void => {
    if (indexOf(id) >= 0) activeId.value = id;
  };

  function promote(id: string): void {
    if (previewId.value === id) previewId.value = null;
  }

  function setView(id: string, view: KubeTabView): void {
    const tab = list.value.find((t) => t.id === id);
    if (!tab) return;
    tab.state = { ...tab.state, view };
    promote(id);
  }

  function close(id: string): void {
    const at = indexOf(id);
    if (at < 0) return;
    list.value.splice(at, 1);
    if (previewId.value === id) previewId.value = null;
    if (activeId.value === id) {
      activeId.value = (list.value[at] ?? list.value[at - 1])?.id ?? null;
    }
  }

  function keepOnly(keep: (index: number) => boolean, anchorId: string): void {
    list.value = list.value.filter((_, i) => keep(i));
    if (previewId.value && indexOf(previewId.value) < 0) previewId.value = null;
    if (activeId.value === null || indexOf(activeId.value) < 0) activeId.value = anchorId;
  }

  function closeOthers(id: string): void {
    const at = indexOf(id);
    if (at >= 0) keepOnly((i) => i === at, id);
  }

  function closeToTheRight(id: string): void {
    const at = indexOf(id);
    if (at >= 0) keepOnly((i) => i <= at, id);
  }

  function closeAll(): void {
    list.value = [];
    activeId.value = null;
    previewId.value = null;
  }

  function move(fromId: string, toId: string): void {
    const from = indexOf(fromId);
    const to = indexOf(toId);
    if (from < 0 || to < 0 || from === to) return;
    const [tab] = list.value.splice(from, 1);
    if (tab) list.value.splice(to, 0, tab);
  }

  /** A context left the kubeconfig: its tabs go. */
  function dropClusters(ids: readonly string[]): void {
    const gone = list.value.filter((t) => ids.includes(t.state.ref.clusterId));
    for (const t of gone) close(t.id);
  }

  function step(delta: 1 | -1): void {
    const n = list.value.length;
    if (n === 0) return;
    const at = activeId.value === null ? -1 : indexOf(activeId.value);
    activeId.value = list.value[(at + delta + n) % n]?.id ?? null;
  }

  const activateNext = (): void => step(1);
  const activatePrev = (): void => step(-1);
  function closeActive(): void {
    if (activeId.value !== null) close(activeId.value);
  }

  /** The name; qualified with context and namespace when another open tab shares it. */
  function titleOf(tab: KubeTab): string {
    const r = tab.state.ref;
    const clash = list.value.some((t) => t.id !== tab.id && t.state.ref.name === r.name);
    if (!clash) return r.name;
    return `${r.name} · ${r.namespace ? `${r.context}/${r.namespace}` : r.context}`;
  }

  return {
    tabs,
    count,
    activeId,
    previewId,
    activeTab,
    open,
    activate,
    promote,
    setView,
    close,
    closeOthers,
    closeToTheRight,
    closeAll,
    move,
    dropClusters,
    activateNext,
    activatePrev,
    closeActive,
    titleOf,
  };
});
