<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import HighlightedText from '@theme/components/HighlightedText.vue';
import { tableHeadClass, tableHeadRowClass, tableRowClass } from '@theme/components/ui/table';
import { t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { useElementSize } from '@vueuse/core';
import { copyText } from '@workbench/behaviour/clipboard';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { compareNatural, matchesQuery, splitMatches } from '@workbench/behaviour/match';
import { useVirtualRows, VIRTUAL_ROW_CLASS } from '@workbench/behaviour/virtualRows';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { type MenuItem, useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, ref } from 'vue';
import { useKubernetesUiStore } from '../state';
import {
  type KubeResourceRef,
  type KubeTabView,
  refKey as tabRefKey,
  useKubeTabsStore,
} from '../tabs';
import type { KubeKind, KubeList, KubeRow } from '../wire';

const props = defineProps<{ data: KubeList; clusterId: string; context: string; kind: KubeKind }>();

const ui = useKubernetesUiStore();
const tabs = useKubeTabsStore();
const contextMenu = useContextMenuStore();
const { single: rowHeight } = useRowHeight();

const sortBy = ref<{ index: number; dir: 1 | -1 } | null>(null);
const needle = computed(() => ui.listFilter.trim().toLowerCase());

const rows = computed(() => {
  const filtered = props.data.rows.filter((r) => matchesQuery(r.cells.join('\n'), ui.listFilter));
  const sort = sortBy.value;
  if (!sort) return filtered;
  return [...filtered].sort(
    (a, b) => sort.dir * compareNatural(a.cells[sort.index] ?? '', b.cells[sort.index] ?? ''),
  );
});

function toggleSort(index: number): void {
  const cur = sortBy.value;
  sortBy.value =
    cur?.index !== index ? { index, dir: 1 } : cur.dir === 1 ? { index, dir: -1 } : null;
}

const rootEl = ref<HTMLElement | null>(null);
const { width } = useElementSize(rootEl);
const COMPACT_BELOW = 512;
const SHOWN_WHEN_COMPACT = ['Ready', 'Status', 'Age'];
const compact = computed(() => width.value > 0 && width.value < COMPACT_BELOW);

// Compact: the name plus the first two of Ready, Status, Age; else the first other column.
const visibleColumns = computed<number[]>(() => {
  const all = props.data.columns.map((_, i) => i);
  if (!compact.value) return all;
  const named = SHOWN_WHEN_COMPACT.flatMap((n) => {
    const i = props.data.columns.findIndex((c, ci) => ci > 0 && c.name === n);
    return i < 0 ? [] : [i];
  }).sort((a, b) => a - b);
  const extra = named.length > 0 ? named.slice(0, 2) : all.slice(1, 2);
  return [0, ...extra].filter((i) => i < all.length);
});

const gridStyle = computed(() => ({
  gridTemplateColumns: `repeat(${visibleColumns.value.length}, minmax(5rem, 1fr))`,
}));

const rowKey = (r: KubeRow): string => `${r.namespace}/${r.name}`;
const refOf = (r: KubeRow): KubeResourceRef => ({
  clusterId: props.clusterId,
  context: props.context,
  kind: props.kind,
  namespace: r.namespace,
  name: r.name,
});
const activeRef = computed(() => {
  const r = tabs.activeTab?.state.ref;
  return r ? tabRefKey(r) : null;
});
const isSelected = (r: KubeRow): boolean => tabRefKey(refOf(r)) === activeRef.value;

function openRow(r: KubeRow, preview: boolean, view?: KubeTabView): void {
  tabs.open(refOf(r), { preview, view });
}

function menuFor(r: KubeRow): MenuItem[] {
  return [
    {
      type: 'item',
      id: 'kube-open',
      label: t('common.action.open.label'),
      icon: 'go-to-file',
      run: () => openRow(r, false),
    },
    ...(props.kind.logs
      ? [
          {
            type: 'item',
            id: 'kube-open-logs',
            label: t('kubernetes.row.openLogs.label'),
            icon: 'output',
            run: () => openRow(r, false, 'logs'),
          } as const,
        ]
      : []),
    {
      type: 'item',
      id: 'copy-name',
      label: t('common.action.copyName.label'),
      icon: 'copy',
      run: () => copyText(r.name),
    },
  ];
}

const scrollEl = ref<HTMLElement | null>(null);
const activeKey = ref<string | null>(null);
const activeIndex = computed(() => rows.value.findIndex((r) => rowKey(r) === activeKey.value));
const { virtualItems, totalSize, onScroll, scrollToIndex } = useVirtualRows({
  count: () => rows.value.length,
  rowHeight: () => rowHeight.value,
  scrollElement: scrollEl,
  pinned: () => (activeIndex.value < 0 ? null : activeIndex.value),
});
const nav = useListNavigation<KubeRow>({
  container: scrollEl,
  items: () => rows.value,
  key: rowKey,
  activeKey,
  onOpen: (r) => openRow(r, false),
  onMenu: (r, rect) => contextMenu.openContextMenuAt(rect.left, rect.bottom, menuFor(r)),
  scrollToIndex: (index) => scrollToIndex(index),
  rendered: () => virtualItems.value,
});
</script>

<template>
  <div
    ref="rootEl"
    class="flex min-h-0 flex-1 flex-col"
    :data-compact="compact"
    data-testid="kube-table"
  >
    <EmptyState
      v-if="data.rows.length === 0"
      fill
      data-testid="kube-list-empty"
      icon="symbol-misc"
      :title="t('kubernetes.list.empty.empty')"
      :description="t('kubernetes.list.emptyHint.emptyHint')"
    />
    <EmptyState
      v-else-if="rows.length === 0"
      fill
      data-testid="kube-list-no-match"
      :title="t('common.state.noMatches.empty')"
    />
    <template v-else>
      <div
        :class="cn(tableHeadRowClass, tableHeadClass, 'grid shrink-0 items-center gap-x-3 px-3')"
        :style="gridStyle"
        data-testid="kube-table-head"
      >
        <button
          v-for="i in visibleColumns"
          :key="data.columns[i]!.name"
          type="button"
          class="flex min-w-0 cursor-default items-center gap-1 border-0 bg-transparent p-0 text-left font-inherit text-inherit"
          :data-testid="`kube-sort-${data.columns[i]!.name}`"
          @click="toggleSort(i)"
        >
          <span class="truncate">{{ data.columns[i]!.name }}</span>
          <CodiconIcon
            v-if="sortBy?.index === i"
            :name="sortBy.dir === 1 ? 'arrow-up' : 'arrow-down'"
            :size="12"
          />
        </button>
      </div>
      <div
        ref="scrollEl"
        role="listbox"
        class="min-h-0 flex-1 overflow-auto"
        data-testid="kube-rows"
        @scroll="onScroll"
      >
        <div class="relative" :style="{ height: `${totalSize}px` }">
          <!-- biome-ignore lint/a11y/useKeyWithClickEvents: Enter comes from useListNavigation on the container. -->
          <div
            v-for="vi in virtualItems"
            :key="rowKey(rows[vi.index]!)"
            :class="
              cn(
                tableRowClass,
                rowVariants({ layout: 'tree', selected: isSelected(rows[vi.index]!) }),
                VIRTUAL_ROW_CLASS,
                'grid gap-x-3 px-3',
              )
            "
            :style="{
              ...gridStyle,
              height: `${vi.size}px`,
              transform: `translateY(${vi.start}px)`,
            }"
            role="option"
            :aria-selected="isSelected(rows[vi.index]!)"
            data-testid="kube-row"
            :data-name="rows[vi.index]!.name"
            :data-namespace="rows[vi.index]!.namespace"
            v-bind="nav.rowProps(rowKey(rows[vi.index]!))"
            :tabindex="nav.tabindexFor(rowKey(rows[vi.index]!))"
            @click="openRow(rows[vi.index]!, true)"
            @dblclick="openRow(rows[vi.index]!, false)"
            @contextmenu.prevent="
              nav.contextMenu(rows[vi.index]!, $event, menuFor(rows[vi.index]!))
            "
          >
            <span v-for="ci in visibleColumns" :key="ci" class="min-w-0 truncate"
              ><HighlightedText
                :parts="splitMatches(rows[vi.index]!.cells[ci] ?? '', needle)"
              /></span
            >
          </div>
        </div>
      </div>
      <div
        v-if="data.truncated"
        class="shrink-0 bg-chrome px-2 py-0.5 text-kira-sm text-muted-foreground"
        data-testid="kube-truncated"
      >
        {{ t('kubernetes.list.truncated.status', { count: data.rows.length }) }}
      </div>
    </template>
  </div>
</template>
