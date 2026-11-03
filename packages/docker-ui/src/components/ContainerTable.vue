<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import Spinner from '@theme/components/Spinner.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { tableHeadClass, tableHeadRowClass, tableRowClass } from '@theme/components/ui/table';
import { t } from '@theme/i18n';
import { rowActions } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { useResizeObserver } from '@vueuse/core';
import { displayValue, formatBytes, formatPercent } from '@workbench/behaviour/format';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { compareNatural } from '@workbench/behaviour/match';
import { useVirtualRows, VIRTUAL_ROW_CLASS } from '@workbench/behaviour/virtualRows';
import QueryState from '@workbench/components/QueryState.vue';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { computed, onMounted, ref, watch } from 'vue';
import { publishedPorts, stateDotTone } from '../lib/format';
import { useContainerAction, useContainers } from '../queries';
import { useDockerStatsStore } from '../state/dockerStats';
import { useDockerUiStore } from '../state/dockerUi';
import type { DockerContainer } from '../wire';
import UsageBar from './UsageBar.vue';

const props = defineProps<{ scrollEl: HTMLElement | null }>();

const ui = useDockerUiStore();
const stats = useDockerStatsStore();
const containers = useContainers();
const actions = useContainerAction();

const rows = computed(() =>
  [...(containers.data.value ?? [])].sort((a, b) => {
    const ra = a.state === 'running' ? 0 : 1;
    const rb = b.state === 'running' ? 0 : 1;
    return ra - rb || compareNatural(a.name, b.name) || compareNatural(a.id, b.id);
  }),
);

const COLUMNS =
  'grid-cols-[0.5rem_minmax(0,1fr)_6rem_7rem_5.5rem] @4xl:grid-cols-[0.5rem_minmax(8rem,1.2fr)_minmax(8rem,1.4fr)_minmax(5rem,0.8fr)_7rem_8rem_5.5rem]';

function act(c: DockerContainer, action: 'start' | 'stop' | 'restart'): void {
  actions.run(c.id, action).catch(() => {
    // Surfaced through `actions.error`.
  });
}

function open(c: DockerContainer, tab: 'overview' | 'logs' | 'terminal' = 'overview'): void {
  ui.select({ kind: 'container', id: c.id }, tab);
}

const tableEl = ref<HTMLElement | null>(null);
const bodyEl = ref<HTMLElement | null>(null);
const activeKey = ref<string | null>(null);
const activeIndex = computed(() => rows.value.findIndex((c) => c.id === activeKey.value));

// Other overview sections sit above the table inside the scroller, so the virtualizer needs the offset.
const scrollMargin = ref(0);
function updateMargin(): void {
  const scroller = props.scrollEl;
  const body = bodyEl.value;
  if (!scroller || !body) return;
  scrollMargin.value = Math.round(
    body.getBoundingClientRect().top - scroller.getBoundingClientRect().top + scroller.scrollTop,
  );
}
onMounted(updateMargin);
watch([() => props.scrollEl, bodyEl], updateMargin);
useResizeObserver(() => props.scrollEl, updateMargin);
useResizeObserver(() => props.scrollEl?.firstElementChild as HTMLElement | undefined, updateMargin);

const rowHeight = useRowHeight();
const { virtualizer, virtualItems, totalSize, scrollToIndex } = useVirtualRows({
  count: () => rows.value.length,
  rowHeight: () => rowHeight.double.value,
  scrollElement: computed(() => props.scrollEl),
  scrollMargin: () => scrollMargin.value,
  pinned: () => (activeIndex.value < 0 ? null : activeIndex.value),
});
const windowRows = computed(() =>
  virtualItems.value.flatMap((item) => {
    const c = rows.value[item.index];
    return c ? [{ item, c }] : [];
  }),
);
function measure(el: unknown): void {
  if (el instanceof Element) virtualizer.value.measureElement(el);
}
const nav = useListNavigation<DockerContainer>({
  container: tableEl,
  items: () => rows.value,
  key: (c) => c.id,
  activeKey,
  onOpen: (c) => open(c, 'logs'),
  scrollToIndex: (index) => scrollToIndex(index),
  rendered: () => virtualItems.value,
});
</script>

<template>
  <QueryState :query="containers" :is-empty="() => rows.length === 0">
    <template #empty>
      <EmptyState
        fill
        data-testid="docker-list-empty"
        icon="server"
        :title="t('docker.container.none.empty')"
        :description="t('docker.container.none.emptyHint')"
      />
    </template>
    <div ref="tableEl" class="@container" role="listbox" data-testid="docker-table">
      <div
        :class="cn(tableHeadRowClass, tableHeadClass, 'grid items-center gap-x-3 px-3', COLUMNS)"
      >
        <span />
        <span>{{ t('common.field.name.label') }}</span>
        <span class="hidden @4xl:block">{{ t('docker.field.image.label') }}</span>
        <span class="hidden @4xl:block">{{ t('docker.field.ports.label') }}</span>
        <span class="text-right">{{ t('docker.metric.cpu.label') }}</span>
        <span class="text-right">{{ t('common.module.memory.label') }}</span>
        <span />
      </div>
      <div ref="bodyEl" class="relative" :style="{ height: `${totalSize}px` }">
        <!-- biome-ignore lint/a11y/useKeyWithClickEvents: Enter comes from useListNavigation on the container. -->
        <div
          v-for="{ item, c } in windowRows"
          :key="c.id"
          :ref="measure"
          :data-index="item.index"
          :class="
            cn(
              tableRowClass,
              VIRTUAL_ROW_CLASS,
              'group/row grid h-auto min-h-row cursor-default select-none items-center gap-x-3 px-3 py-1',
              item.index === rows.length - 1 && 'border-b-0',
              COLUMNS,
            )
          "
          :style="{ transform: `translateY(${item.start - scrollMargin}px)` }"
          data-testid="docker-table-row"
          :data-name="c.name"
          :data-state="c.state"
          role="option"
          :aria-selected="false"
          v-bind="nav.rowProps(c.id)"
          :tabindex="nav.tabindexFor(c.id)"
          @click="open(c)"
          @dblclick="open(c, 'logs')"
        >
          <StatusDot :tone="stateDotTone(c.state)" />
          <span class="flex min-w-0 flex-col">
            <span class="flex min-w-0 items-baseline gap-1.5">
              <span
                class="truncate"
                :class="c.state === 'running' ? 'font-medium' : 'text-muted-foreground'"
                >{{
                  c.name
                }}</span
              >
              <span v-if="c.composeProject" class="shrink-0 text-kira-sm text-muted-foreground">{{
                c.composeProject
              }}</span>
            </span>
            <span class="truncate text-kira-sm text-muted-foreground @4xl:hidden">{{
              c.image
            }}</span>
          </span>
          <span class="hidden truncate text-muted-foreground @4xl:block">{{ c.image }}</span>
          <span class="hidden truncate font-data text-kira-sm text-muted-foreground @4xl:block">{{
            displayValue(publishedPorts(c.ports).join(' '))
          }}</span>
          <template v-if="c.state === 'running' && stats.latest.get(c.id)">
            <span class="flex flex-col items-end gap-0.5">
              <span class="font-data text-kira-sm">{{
                formatPercent(stats.latest.get(c.id)!.cpuPercent)
              }}</span>
              <UsageBar :percent="stats.latest.get(c.id)!.cpuPercent" />
            </span>
            <span class="flex flex-col items-end gap-0.5">
              <span class="font-data text-kira-sm">{{
                formatBytes(stats.latest.get(c.id)!.memUsage)
              }}</span>
              <UsageBar :percent="stats.latest.get(c.id)!.memPercent" />
            </span>
          </template>
          <template v-else>
            <span class="text-right text-muted-foreground">{{
              c.state === 'running' ? '…' : displayValue(null)
            }}</span>
            <span class="text-right text-muted-foreground">{{
              c.state === 'running' ? '…' : displayValue(null)
            }}</span>
          </template>
          <span class="flex items-center justify-end">
            <Spinner v-if="actions.isBusy(c.id)" label="" />
            <span v-else :class="cn('flex', rowActions)">
              <TooltipIconButton
                icon="output"
                :label="t('docker.tab.logs.label')"
                @click.stop="open(c, 'logs')"
              />
              <TooltipIconButton
                v-if="c.state === 'running'"
                icon="debug-stop"
                :label="t('common.action.stop.label')"
                tone="error"
                data-testid="docker-table-stop"
                @click.stop="act(c, 'stop')"
              />
              <TooltipIconButton
                v-else
                icon="play"
                :label="t('common.action.start.label')"
                tone="ok"
                data-testid="docker-table-start"
                @click.stop="act(c, 'start')"
              />
            </span>
          </span>
        </div>
      </div>
    </div>
  </QueryState>
</template>
