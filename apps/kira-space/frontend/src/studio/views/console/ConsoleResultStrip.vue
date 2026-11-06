<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { tabChipVariants } from '@theme/components/ui/tabs';
import { t } from '@theme/i18n';
import { rowActions, rowMeta } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { wheelToHorizontal } from '@workbench/lib/wheelScroll';
import { useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, useTemplateRef } from 'vue';
import { getPage } from './resultPages';
import { useConsoleViewStore } from './state';

// The result tab strip of a console tab, with its status line and document expand/collapse.
const props = defineProps<{ tabId: string }>();
const emit = defineEmits<{ expandAll: []; collapseAll: [] }>();

const consoleViewStore = useConsoleViewStore();
const contextMenuStore = useContextMenuStore();

const rt = computed(() => consoleViewStore.runtime[props.tabId]);
const activeResultIsDocument = computed(
  () => getPage(rt.value?.activeKey ?? '')?.kind === 'document',
);

const RESULT_KIND_ICON: Record<string, string> = {
  tabular: 'table',
  document: 'json',
  keyvalue: 'symbol-key',
};
function iconForResult(key: string): string {
  if (rt.value?.results.find((r) => r.key === key)?.kind === 'plan') return 'list-tree';
  return RESULT_KIND_ICON[getPage(key)?.kind ?? ''] ?? 'table';
}

function onResultContextMenu(e: MouseEvent, key: string, index: number): void {
  const total = rt.value?.results.length ?? 0;
  contextMenuStore.openContextMenu(e, [
    {
      type: 'item',
      id: 'close',
      label: t('common.action.close.label'),
      icon: 'close',
      run: () => consoleViewStore.closeResult(props.tabId, key),
    },
    {
      type: 'item',
      id: 'close-other-results',
      label: t('common.action.closeOthers.label'),
      disabled: total <= 1,
      run: () => consoleViewStore.closeOtherResults(props.tabId, key),
    },
    {
      type: 'item',
      id: 'close-results-to-the-right',
      label: t('common.action.closeToTheRight.label'),
      disabled: index >= total - 1,
      run: () => consoleViewStore.closeResultsToTheRight(props.tabId, key),
    },
  ]);
}

const stripEl = useTemplateRef<HTMLElement>('stripEl');
function onWheel(e: WheelEvent): void {
  if (wheelToHorizontal(stripEl.value, e)) e.preventDefault();
}

const statusLine = computed(() => {
  const r = rt.value;
  if (!r) return '';
  if (r.status === 'running') return t('common.state.running.status');
  if (r.status === 'cancelled') return t('common.cancelled.status');
  if (r.status === 'idle' && r.results.length > 0) {
    return t('studio.console.status.results.status', { count: r.results.length });
  }
  return '';
});
</script>

<template>
  <ViewToolbar v-if="rt">
    <div
      ref="stripEl"
      class="flex items-center flex-1 min-w-0 overflow-x-auto gap-0.5 scrollbar-none"
      data-testid="console-result-strip"
      @wheel="onWheel"
    >
      <div
        v-for="(result, i) in rt.results"
        :key="result.key"
        class="group"
        :class="cn(tabChipVariants({ active: result.key === rt.activeKey }), 'max-w-36')"
        data-testid="console-result-tab"
        :data-active="result.key === rt.activeKey"
      >
        <button
          type="button"
          class="flex flex-1 min-w-0 items-center gap-1 border-0 bg-transparent p-0"
          @click="consoleViewStore.setActiveResult(tabId, result.key)"
          @auxclick.middle="consoleViewStore.closeResult(tabId, result.key)"
          @contextmenu.prevent="onResultContextMenu($event, result.key, i)"
        >
          <CodiconIcon :name="iconForResult(result.key)" :size="13" class="shrink-0" />
          <span class="min-w-0 truncate">{{
            t('studio.console.result.tab.label', { n: i + 1 })
          }}</span>
        </button>
        <Button
          variant="toolbar"
          size="kira-icon"
          class="size-4"
          :class="result.key === rt.activeKey ? 'opacity-100' : rowActions"
          :aria-label="t('studio.console.result.close.aria')"
          data-testid="console-result-close"
          @click="consoleViewStore.closeResult(tabId, result.key)"
        >
          <CodiconIcon name="close" :size="13" />
        </Button>
      </div>
    </div>
    <span class="ml-auto" :class="rowMeta" data-testid="console-status">{{ statusLine }}</span>
    <template v-if="activeResultIsDocument">
      <TooltipIconButton
        icon="expand-all"
        :label="t('common.action.expandAll.label')"
        data-testid="console-expand-all"
        @click="emit('expandAll')"
      />
      <TooltipIconButton
        icon="collapse-all"
        :label="t('common.action.collapseAll.label')"
        data-testid="console-collapse-all"
        @click="emit('collapseAll')"
      />
    </template>
  </ViewToolbar>
</template>
