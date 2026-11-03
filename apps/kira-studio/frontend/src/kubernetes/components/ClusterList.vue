<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import Spinner from '@theme/components/Spinner.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import { type MessageKey, t } from '@theme/i18n';
import { rowIndent, rowMeta, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { copyText } from '@workbench/behaviour/clipboard';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { matchesQuery } from '@workbench/behaviour/match';
import { useTreeShortcuts } from '@workbench/behaviour/treeShortcuts';
import { logError } from '@workbench/bridge/log';
import QueryState from '@workbench/components/QueryState.vue';
import { useRowHeight } from '@workbench/lib/rowHeight';
import {
  type MenuItem,
  runMenuShortcutAsync,
  useContextMenuStore,
} from '@workbench/state/contextMenu';
import { computed, ref, watch } from 'vue';
import { useClusterProbes, useClusters, useRefreshCluster, useReorderClusters } from '../queries';
import { useKubernetesUiStore } from '../state';
import type { KubeCluster } from '../wire';
import ClusterIcon from './ClusterIcon.vue';
import ClusterMcpDialog from './ClusterMcpDialog.vue';
import { useClusterReorder } from './clusterReorder';

type DotState = 'unknown' | 'probing' | 'ok' | 'error';

interface ClusterRow {
  key: string;
  cluster: KubeCluster;
  dot: DotState;
}

const ui = useKubernetesUiStore();
const clusters = useClusters();
const contextMenu = useContextMenuStore();
const refresh = useRefreshCluster();
const reorder = useReorderClusters();
const { single: rowHeight } = useRowHeight();

const list = computed(() => clusters.data.value?.clusters ?? []);
const searching = computed(() => ui.clusterSearch.trim() !== '');
// Only the selected cluster connects; one probed earlier keeps its dot.
const probed = ref<string[]>(ui.clusterId ? [ui.clusterId] : []);
watch(
  () => ui.clusterId,
  (id) => {
    if (id && !probed.value.includes(id)) probed.value = [...probed.value, id];
  },
);
const statuses = useClusterProbes(() => list.value.map((c) => c.id), probed);

const mcpClusterId = ref('');
const mcpOpen = computed({
  get: () => mcpClusterId.value !== '',
  set: (v) => {
    if (!v) mcpClusterId.value = '';
  },
});

function dotOf(index: number): DotState {
  const status = statuses.value[index];
  if (!status) return 'unknown';
  if (status.data) return status.data.state === 'ok' ? 'ok' : 'error';
  if (status.isError) return 'error';
  return status.fetchStatus === 'fetching' ? 'probing' : 'unknown';
}

const rows = computed<ClusterRow[]>(() =>
  list.value.flatMap((cluster, index) =>
    matchesQuery(cluster.context, ui.clusterSearch)
      ? [{ key: `c:${cluster.id}`, cluster, dot: dotOf(index) }]
      : [],
  ),
);

function selectRow(row: ClusterRow): void {
  ui.selectCluster(row.cluster.id);
}

// Disabled at the ends and while a search filters the list: the visible order is then partial.
function moveItems(id: string): MenuItem[] {
  const at = list.value.findIndex((c) => c.id === id);
  return [
    {
      type: 'item',
      id: 'move-up',
      label: t('common.moveUp.tooltip'),
      icon: 'arrow-up',
      shortcut: 'list.moveUp',
      disabled: searching.value || at <= 0,
      run: () => moveCluster(id, -1),
    },
    {
      type: 'item',
      id: 'move-down',
      label: t('common.moveDown.tooltip'),
      icon: 'arrow-down',
      shortcut: 'list.moveDown',
      disabled: searching.value || at < 0 || at >= list.value.length - 1,
      run: () => moveCluster(id, 1),
    },
  ];
}

async function moveCluster(id: string, step: -1 | 1): Promise<void> {
  const ids = list.value.map((c) => c.id);
  const from = ids.indexOf(id);
  const to = from + step;
  if (from < 0 || to < 0 || to >= ids.length) return;
  [ids[from], ids[to]] = [ids[to], ids[from]];
  await reorder.mutateAsync(ids).catch((err) => logError('kubernetes', 'reorder failed', err));
}

function menuFor(row: ClusterRow): MenuItem[] {
  return [
    {
      type: 'item',
      id: 'copy-context',
      label: t('kubernetes.cluster.copyContext.label'),
      icon: 'copy',
      run: () => copyText(row.cluster.context),
    },
    {
      type: 'item',
      id: 'refresh',
      label: t('common.action.refresh.label'),
      icon: 'refresh',
      run: () => void refresh.run(row.cluster.id),
    },
    {
      type: 'item',
      id: 'kube-cluster-mcp',
      label: t('kubernetes.mcp.menu.label'),
      icon: 'settings-gear',
      run: () => {
        mcpClusterId.value = row.cluster.id;
      },
    },
    ...moveItems(row.cluster.id),
  ];
}

const listEl = ref<HTMLElement | null>(null);
const activeKey = ref<string | null>(null);
const nav = useListNavigation<ClusterRow>({
  container: listEl,
  items: () => rows.value,
  key: (r) => r.key,
  activeKey,
  onSelect: selectRow,
  onOpen: selectRow,
  onMenu: (row, rect) => contextMenu.openContextMenuAt(rect.left, rect.bottom, menuFor(row)),
});

const { draggingId, drop } = useClusterReorder({
  container: listEl,
  ids: () => list.value.map((c) => c.id),
  disabled: () => searching.value || list.value.length < 2,
  rowHeight: () => rowHeight.value,
  onReorder: (ids) =>
    void reorder.mutateAsync(ids).catch((err) => logError('kubernetes', 'reorder failed', err)),
});

// Rows are keyed by cluster, but the focused element may show another row after the move: refocus.
function moveViaMenu(id: 'list.moveUp' | 'list.moveDown'): (key: string) => void {
  return (key) => {
    const row = rows.value.find((r) => r.key === key);
    if (!row) return;
    void runMenuShortcutAsync(menuFor(row), id).then(() => nav.focusKey(key));
  };
}
useTreeShortcuts(nav, {
  'list.moveUp': moveViaMenu('list.moveUp'),
  'list.moveDown': moveViaMenu('list.moveDown'),
});

function onContext(row: ClusterRow, e: MouseEvent): void {
  e.preventDefault();
  nav.contextMenu(row, e, menuFor(row));
}

const isSelected = (row: ClusterRow): boolean => row.cluster.id === ui.clusterId;

const DOT_TONE = { unknown: 'muted', ok: 'ok', error: 'error', probing: 'muted' } as const;
const DOT_LABEL: Record<DotState, MessageKey> = {
  unknown: 'kubernetes.cluster.state.unknown.label',
  probing: 'kubernetes.cluster.state.unknown.label',
  ok: 'kubernetes.cluster.state.ok.label',
  error: 'kubernetes.cluster.state.error.label',
};
const rowClass = (selected: boolean) =>
  cn(rowVariants({ layout: 'tree', selected }), 'group/row h-row');
</script>

<template>
  <QueryState :query="clusters" :is-empty="() => list.length === 0">
    <template #empty>
      <EmptyState
        fill
        data-testid="kube-clusters-empty"
        :title="t('kubernetes.panel.none.empty')"
        :description="t('kubernetes.panel.noneHint.emptyHint')"
      />
    </template>
    <div
      ref="listEl"
      role="listbox"
      :aria-label="t('common.product.kubernetes.label')"
      :class="cn('min-h-0 flex-1 overflow-auto', draggingId && 'select-none')"
      data-testid="kube-cluster-list"
    >
      <!-- biome-ignore lint/a11y/useKeyWithClickEvents: Enter comes from useListNavigation on the container. -->
      <div
        v-for="row in rows"
        :key="row.key"
        :class="
          cn(rowClass(isSelected(row)), 'relative', draggingId === row.cluster.id && 'opacity-50')
        "
        :style="rowIndent(0)"
        role="option"
        :aria-selected="isSelected(row)"
        data-testid="kube-cluster-row"
        data-kind="cluster"
        :data-cluster-id="row.cluster.id"
        :data-context="row.cluster.context"
        :data-state="row.dot"
        v-bind="nav.rowProps(row.key)"
        :tabindex="nav.tabindexFor(row.key)"
        @click="selectRow(row)"
        @contextmenu="onContext(row, $event)"
      >
        <div
          v-if="drop?.id === row.cluster.id"
          aria-hidden="true"
          data-testid="kube-drop-indicator"
          :class="
            cn(
              'pointer-events-none absolute inset-x-0 h-0.5 bg-focus',
              drop.edge === 'top' ? 'top-0' : 'bottom-0',
            )
          "
        />
        <ClusterIcon :size="14" />
        <span class="min-w-0 flex-1 truncate">{{ row.cluster.context }}</span>
        <span v-if="row.cluster.current" :class="rowMeta" data-testid="kube-cluster-current">{{
          t('kubernetes.cluster.current.label')
        }}</span>
        <Spinner v-if="row.dot === 'probing'" label="" />
        <StatusDot v-else :tone="DOT_TONE[row.dot]" :label="t(DOT_LABEL[row.dot])" />
      </div>
    </div>
    <ClusterMcpDialog v-if="mcpClusterId" v-model:open="mcpOpen" :cluster-id="mcpClusterId" />
  </QueryState>
</template>
