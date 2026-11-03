<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import Spinner from '@theme/components/Spinner.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import { type MessageKey, t } from '@theme/i18n';
import { rowIndent, rowMeta, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { copyText } from '@workbench/behaviour/clipboard';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { compareNatural, matchesQuery } from '@workbench/behaviour/match';
import { useTreeShortcuts } from '@workbench/behaviour/treeShortcuts';
import { logError } from '@workbench/bridge/log';
import QueryState from '@workbench/components/QueryState.vue';
import TreeTwisty from '@workbench/components/TreeTwisty.vue';
import { useRowHeight } from '@workbench/lib/rowHeight';
import {
  type MenuItem,
  runMenuShortcutAsync,
  useContextMenuStore,
} from '@workbench/state/contextMenu';
import { computed, ref } from 'vue';
import { useClusterProbes, useClusters, useRefreshCluster, useReorderClusters } from '../queries';
import { useKubernetesUiStore } from '../state';
import type { KindCategory, KubeCluster, KubeKind } from '../wire';
import ClusterIcon from './ClusterIcon.vue';
import ClusterMcpDialog from './ClusterMcpDialog.vue';
import { useClusterReorder } from './clusterReorder';

type DotState = 'unknown' | 'probing' | 'ok' | 'error';

interface ClusterRow {
  type: 'cluster';
  key: string;
  cluster: KubeCluster;
  dot: DotState;
  open: boolean;
}
interface CategoryRow {
  type: 'category';
  key: string;
  clusterId: string;
  category: KindCategory;
  count: number;
  open: boolean;
}
interface KindRow {
  type: 'kind';
  key: string;
  clusterId: string;
  kind: KubeKind;
}
type Row = ClusterRow | CategoryRow | KindRow;

const ui = useKubernetesUiStore();
const clusters = useClusters();
const contextMenu = useContextMenuStore();
const refresh = useRefreshCluster();
const reorder = useReorderClusters();
const { single: rowHeight } = useRowHeight();

const list = computed(() => clusters.data.value?.clusters ?? []);
const searching = computed(() => ui.treeSearch.trim() !== '');
const active = computed(() => {
  const ids = new Set(ui.expanded);
  if (ui.clusterId) ids.add(ui.clusterId);
  return [...ids];
});
const probes = useClusterProbes(() => list.value.map((c) => c.id), active);

const closedCategories = ref<string[]>([]);
const mcpClusterId = ref('');
const mcpOpen = computed({
  get: () => mcpClusterId.value !== '',
  set: (v) => {
    if (!v) mcpClusterId.value = '';
  },
});

function dotOf(index: number): DotState {
  const status = probes.statuses.value[index];
  if (!status) return 'unknown';
  if (status.data) return status.data.state === 'ok' ? 'ok' : 'error';
  if (status.isError) return 'error';
  return status.fetchStatus === 'fetching' ? 'probing' : 'unknown';
}

const rows = computed<Row[]>(() => {
  const out: Row[] = [];
  list.value.forEach((cluster, index) => {
    const kinds = probes.kinds.value[index]?.data?.kinds ?? [];
    const kindMatches = kinds.filter((k) => matchesQuery(k.kind, ui.treeSearch));
    if (
      searching.value &&
      !matchesQuery(cluster.context, ui.treeSearch) &&
      kindMatches.length === 0
    )
      return;
    const open = searching.value || ui.expanded.includes(cluster.id);
    out.push({ type: 'cluster', key: `c:${cluster.id}`, cluster, dot: dotOf(index), open });
    if (!open) return;
    const visible = matchesQuery(cluster.context, ui.treeSearch) ? kinds : kindMatches;
    const byCategory = new Map<KindCategory, KubeKind[]>();
    for (const k of visible) byCategory.set(k.category, [...(byCategory.get(k.category) ?? []), k]);
    for (const [category, members] of byCategory) {
      const key = `g:${cluster.id}:${category}`;
      const catOpen = searching.value || !closedCategories.value.includes(key);
      out.push({
        type: 'category',
        key,
        clusterId: cluster.id,
        category,
        count: members.length,
        open: catOpen,
      });
      if (!catOpen) continue;
      for (const kind of [...members].sort((a, b) => compareNatural(a.kind, b.kind))) {
        out.push({ type: 'kind', key: `k:${cluster.id}:${kind.key}`, clusterId: cluster.id, kind });
      }
    }
  });
  return out;
});

const depthOf = (row: Row): number =>
  row.type === 'cluster' ? 0 : row.type === 'category' ? 1 : 2;

function toggleCategory(key: string): void {
  closedCategories.value = closedCategories.value.includes(key)
    ? closedCategories.value.filter((k) => k !== key)
    : [...closedCategories.value, key];
}

function selectRow(row: Row): void {
  if (row.type === 'cluster') {
    ui.selectCluster(row.cluster.id);
  } else if (row.type === 'kind') {
    ui.selectKind(row.clusterId, row.kind.key);
  } else {
    toggleCategory(row.key);
  }
}

function toggleRow(row: Row): void {
  if (row.type === 'cluster') ui.toggleExpanded(row.cluster.id);
  else if (row.type === 'category') toggleCategory(row.key);
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

function menuFor(row: Row): MenuItem[] {
  if (row.type !== 'cluster') return [];
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

const treeEl = ref<HTMLElement | null>(null);
const activeKey = ref<string | null>(null);
const nav = useListNavigation<Row>({
  container: treeEl,
  items: () => rows.value,
  key: (r) => r.key,
  activeKey,
  onSelect: selectRow,
  onOpen: selectRow,
  onMenu: (row, rect) => {
    const items = menuFor(row);
    if (items.length > 0) contextMenu.openContextMenuAt(rect.left, rect.bottom, items);
  },
  tree: {
    depth: depthOf,
    isExpanded: (r) => (r.type === 'kind' ? false : r.open),
    hasChildren: (r) => r.type !== 'kind',
    expand: (r) => {
      if (r.type !== 'kind' && !r.open) toggleRow(r);
    },
    collapse: (r) => {
      if (r.type !== 'kind' && r.open) toggleRow(r);
    },
  },
});

const { draggingId, drop } = useClusterReorder({
  container: treeEl,
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
    if (row?.type !== 'cluster') return;
    void runMenuShortcutAsync(menuFor(row), id).then(() => nav.focusKey(key));
  };
}
useTreeShortcuts(nav, {
  'list.moveUp': moveViaMenu('list.moveUp'),
  'list.moveDown': moveViaMenu('list.moveDown'),
});

function onContext(row: Row, e: MouseEvent): void {
  const items = menuFor(row);
  if (items.length === 0) return;
  e.preventDefault();
  nav.contextMenu(row, e, items);
}

function isSelected(row: Row): boolean {
  if (row.type === 'cluster') return row.cluster.id === ui.clusterId && ui.kind === '';
  return row.type === 'kind' && row.clusterId === ui.clusterId && row.kind.key === ui.kind;
}

const DOT_TONE = { unknown: 'muted', ok: 'ok', error: 'error', probing: 'muted' } as const;
const DOT_LABEL: Record<DotState, MessageKey> = {
  unknown: 'kubernetes.cluster.state.unknown.label',
  probing: 'kubernetes.cluster.state.unknown.label',
  ok: 'kubernetes.cluster.state.ok.label',
  error: 'kubernetes.cluster.state.error.label',
};
const CATEGORY_LABEL: Record<KindCategory, () => string> = {
  workloads: () => t('kubernetes.kind.category.workloads.label'),
  network: () => t('docker.resource.network.label'),
  config: () => t('kubernetes.kind.category.config.label'),
  storage: () => t('kubernetes.kind.category.storage.label'),
  access: () => t('kubernetes.kind.category.access.label'),
  cluster: () => t('kubernetes.kind.category.cluster.label'),
  custom: () => t('kubernetes.kind.category.custom.label'),
};
const categoryLabel = (c: KindCategory): string => CATEGORY_LABEL[c]();
const rowClass = (selected: boolean) =>
  cn(rowVariants({ layout: 'tree', selected }), 'group/row h-row');
</script>

<template>
  <QueryState :query="clusters" :is-empty="() => list.length === 0">
    <template #empty>
      <EmptyState
        fill
        data-testid="kube-tree-empty"
        :title="t('kubernetes.panel.none.empty')"
        :description="t('kubernetes.panel.noneHint.emptyHint')"
      />
    </template>
    <div
      ref="treeEl"
      role="tree"
      :class="cn('min-h-0 flex-1 overflow-auto', draggingId && 'select-none')"
      data-testid="kube-tree"
    >
      <template v-for="row in rows" :key="row.key">
        <!-- biome-ignore lint/a11y/useKeyWithClickEvents: Enter comes from useListNavigation on the container. -->
        <div
          v-if="row.type === 'cluster'"
          :class="
            cn(rowClass(isSelected(row)), 'relative', draggingId === row.cluster.id && 'opacity-50')
          "
          :style="rowIndent(0)"
          role="treeitem"
          :aria-expanded="row.open"
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
          <TreeTwisty :expanded="row.open" :has-children="true" @toggle="toggleRow(row)" />
          <ClusterIcon :size="14" />
          <span class="min-w-0 flex-1 truncate">{{ row.cluster.context }}</span>
          <span v-if="row.cluster.current" :class="rowMeta" data-testid="kube-cluster-current">{{
            t('kubernetes.cluster.current.label')
          }}</span>
          <Spinner v-if="row.dot === 'probing'" label="" />
          <StatusDot v-else :tone="DOT_TONE[row.dot]" :label="t(DOT_LABEL[row.dot])" />
        </div>
        <!-- biome-ignore lint/a11y/useKeyWithClickEvents: Enter comes from useListNavigation on the container. -->
        <div
          v-else-if="row.type === 'category'"
          :class="rowClass(false)"
          :style="rowIndent(1)"
          role="treeitem"
          :aria-expanded="row.open"
          data-testid="kube-category-row"
          :data-category="row.category"
          v-bind="nav.rowProps(row.key)"
          :tabindex="nav.tabindexFor(row.key)"
          @click="selectRow(row)"
        >
          <TreeTwisty :expanded="row.open" :has-children="true" @toggle="toggleRow(row)" />
          <span class="min-w-0 flex-1 truncate text-muted-foreground">{{
            categoryLabel(row.category)
          }}</span>
          <span :class="rowMeta">{{ row.count }}</span>
        </div>
        <!-- biome-ignore lint/a11y/useKeyWithClickEvents: Enter comes from useListNavigation on the container. -->
        <div
          v-else
          :class="rowClass(isSelected(row))"
          :style="rowIndent(2)"
          role="treeitem"
          :aria-selected="isSelected(row)"
          data-testid="kube-kind-row"
          :data-kind="row.kind.key"
          v-bind="nav.rowProps(row.key)"
          :tabindex="nav.tabindexFor(row.key)"
          @click="selectRow(row)"
        >
          <span class="size-3.5 shrink-0" />
          <span class="min-w-0 flex-1 truncate">{{ row.kind.kind }}</span>
        </div>
      </template>
    </div>
    <ClusterMcpDialog v-if="mcpClusterId" v-model:open="mcpOpen" :cluster-id="mcpClusterId" />
  </QueryState>
</template>
