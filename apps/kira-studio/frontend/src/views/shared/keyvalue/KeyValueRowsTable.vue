<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import Spinner from '@theme/components/Spinner.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Badge } from '@theme/components/ui/badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { formatBytes } from '@workbench/behaviour/format';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { useVirtualRows, VIRTUAL_ROW_CLASS } from '@workbench/behaviour/virtualRows';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { type MenuItem, useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, ref, watch } from 'vue';
import { datasetNumber } from '../eventCoords';
import SearchFilterNotice from '../page/SearchFilterNotice.vue';
import SearchToolbar from '../page/SearchToolbar.vue';
import { createMatchIndex } from '../page/search';
import { setVisibleRows } from '../page/visibleRows';
import { pageVersion, setVisibleWindow } from './page';
import { type Match, matchedRows, pageSearchApi, searchState } from './search';
import { useKeyValueViewStore } from './state';
import { useKeyValueGate } from './useKeyValueGate';

const props = defineProps<{
  viewKey: string;
  menuFor: (field: string, value: string) => MenuItem[] | null;
}>();
const emit = defineEmits<{ select: [row: number] }>();

const keyValueViewStore = useKeyValueViewStore();
const contextMenuStore = useContextMenuStore();
const { rt, page, rowAt } = useKeyValueGate(() => props.viewKey);

// Filtered rows keep their real row number (the `i + 1` gutter).
const displayRows = computed<number[] | null>(() => matchedRows(props.viewKey));
const rowIndices = computed(() => {
  void pageVersion.n;
  if (displayRows.value) return displayRows.value;
  return Array.from({ length: rt.value?.rowCount ?? 0 }, (_, i) => i);
});

const { single: rowHeight } = useRowHeight();
const scrollRef = ref<HTMLElement | null>(null);
const {
  virtualItems: virtualRows,
  totalSize,
  scrollToIndex,
} = useVirtualRows({
  count: () => rowIndices.value.length,
  rowHeight: () => rowHeight.value,
  scrollElement: scrollRef,
  overscan: 8,
});
// A virtual item's `index` is its position in rowIndices; `i` is the page-row number.
const visibleRows = computed(() =>
  virtualRows.value.map((row) => ({ row, i: rowIndices.value[row.index] })),
);

const activeRow = ref<string | null>(null);
const nav = useListNavigation<number>({
  container: scrollRef,
  items: () => rowIndices.value,
  key: (row) => String(row),
  activeKey: activeRow,
  onSelect: (i) => emit('select', i),
  onMenu: (i, rect) => {
    const row = rowAt(i);
    const items = row ? props.menuFor(row.field, row.value) : null;
    if (items) {
      contextMenuStore.openContextMenuAt(rect.left, rect.bottom, items, {
        onRestoreFocus: () => void nav.focusKey(String(i)),
      });
    }
  },
  scrollToIndex: (index) => scrollToIndex(index),
  rendered: () => virtualRows.value,
});

// Stable module-scope handlers: a v-for-scoped closure defeats Vue's handler cache on every render.
function onRowClickFromEvent(e: MouseEvent): void {
  const i = datasetNumber(e.currentTarget, 'row');
  if (i !== null) emit('select', i);
}
// The list navigation claims Enter and the arrows; Space mirrors a single click.
function onRowKeydownFromEvent(e: KeyboardEvent): void {
  if (e.key !== ' ') return;
  e.preventDefault();
  const i = datasetNumber(e.currentTarget, 'row');
  if (i !== null) emit('select', i);
}
function onRowContextMenuFromEvent(e: MouseEvent): void {
  const i = datasetNumber(e.currentTarget, 'row');
  if (i === null) return;
  e.preventDefault();
  const row = rowAt(i);
  const items = row ? props.menuFor(row.field, row.value) : null;
  if (items) nav.contextMenu(i, e, items);
}

function onGoToMatch(match: Match): void {
  const index = rowIndices.value.indexOf(match.row);
  if (index >= 0) scrollToIndex(index);
}

// Reports the visible window so a find scan prioritizes on-screen rows, and prunes the decode cache.
watch(
  virtualRows,
  (rows) => {
    if (rows.length === 0) return;
    const list = rowIndices.value;
    const from = list[rows[0].index];
    const to = list[rows[rows.length - 1].index];
    if (from === undefined || to === undefined) return;
    setVisibleRows(props.viewKey, from, to + 1);
    setVisibleWindow(props.viewKey, from, to + 1);
  },
  { immediate: true },
);

const matchIndex = createMatchIndex(searchState, () => props.viewKey);
function isSearchMatch(row: number, col: 'field' | 'value'): boolean {
  return matchIndex.value?.has(row, col) ?? false;
}
function isCurrentSearchMatch(row: number, col: 'field' | 'value'): boolean {
  return matchIndex.value?.isCurrent(row, col) ?? false;
}

function cellClass(row: number, col: 'field' | 'value') {
  return [
    {
      'search-match bg-search-match': isSearchMatch(row, col),
      'search-match-current bg-search-match-current': isCurrentSearchMatch(row, col),
    },
    isCurrentSearchMatch(row, col) ? 'text-bg' : 'text-fg',
  ];
}
</script>

<template>
  <SearchToolbar
    v-if="rt?.searchOpen"
    :tab-id="viewKey"
    :is-open="() => !!keyValueViewStore.runtime[viewKey]?.searchOpen"
    testid-prefix="keyvalue-"
    kind="rows"
    :api="pageSearchApi"
    @go-to-match="onGoToMatch"
    @close="keyValueViewStore.setSearchOpen(viewKey, false)"
  />

  <div class="bg-bg overflow-hidden flex flex-col flex-1 min-h-0 border-0 rounded-none">
    <div class="h-control-lg shrink-0 flex bg-elevated border-b border-border">
      <div
        class="flex items-center gap-1 px-2 border-r border-border text-kira-sm text-muted-foreground overflow-hidden whitespace-nowrap w-10 shrink-0"
      ></div>
      <div
        class="flex items-center gap-1 px-2 border-r border-border text-kira-sm text-muted-foreground overflow-hidden whitespace-nowrap w-56 shrink-0"
      >
        <span class="text-fg truncate">{{
          page?.redisType === 'string'
            ? ''
            : page?.redisType === 'list'
              ? t('studio.keyValue.column.index.label')
              : t('studio.keyValue.column.field.label')
        }}</span>
      </div>
      <div
        class="flex items-center gap-1 px-2 border-r border-border text-kira-sm text-muted-foreground overflow-hidden whitespace-nowrap flex-1 min-w-0"
      >
        <span class="text-fg truncate">{{
          page?.redisType === 'zset'
            ? t('studio.keyValue.column.score.label')
            : t('common.field.value.label')
        }}</span>
      </div>
    </div>
    <div class="flex-1 min-h-0 flex flex-col overflow-hidden" data-testid="keyvalue-list">
      <EmptyState
        v-if="!rt || rt.rowCount === 0"
        :icon="rt ? 'database' : undefined"
        :title="rt ? t('studio.keyValue.noData.empty') : t('common.state.loading.loading')"
      >
        <template v-if="!rt" #media><Spinner :size="24" /></template>
      </EmptyState>
      <SearchFilterNotice
        v-else-if="displayRows && displayRows.length === 0"
        data-testid="keyvalue-no-matching-rows"
        :filter-key="viewKey"
        show-all-testid="keyvalue-show-all-rows"
      />
      <div
        v-else
        ref="scrollRef"
        class="flex-1 min-h-0 overflow-auto"
        data-testid="virtual-list"
        role="listbox"
        :aria-label="t('studio.keyValue.rows.aria')"
      >
        <div class="relative w-full" :style="{ height: `${totalSize}px` }">
          <template v-for="entry in visibleRows" :key="entry.row.index">
            <div
              :class="cn(rowVariants({ layout: 'tree' }), 'gap-0 h-row px-0', VIRTUAL_ROW_CLASS)"
              data-testid="keyvalue-row"
              :data-row="entry.i"
              :style="{ transform: `translateY(${entry.row.start}px)` }"
              role="option"
              :aria-selected="activeRow === String(entry.i)"
              v-bind="nav.rowProps(String(entry.i))"
              :tabindex="nav.tabindexFor(String(entry.i))"
              @click="onRowClickFromEvent"
              @keydown="onRowKeydownFromEvent"
              @contextmenu="onRowContextMenuFromEvent"
            >
              <div
                class="flex items-center justify-end px-2 border-r border-b border-border border-r-border bg-elevated font-data text-kira-sm text-subtle truncate relative w-10 shrink-0"
                >{{
                  entry.i + 1
                }}</div
              >
              <div
                class="flex items-center px-2 border-r border-b border-border font-data text-kira-md w-56 shrink-0"
                :class="cellClass(entry.i, 'field')"
                data-testid="keyvalue-field"
              >
                <TruncatedText :text="rowAt(entry.i)?.field ?? ''" />
              </div>
              <div
                class="flex items-center px-2 border-r border-b border-border font-data text-kira-md flex-1 min-w-0"
                :class="cellClass(entry.i, 'value')"
                data-testid="keyvalue-value"
              >
                <TruncatedText :text="rowAt(entry.i)?.value ?? ''" />
                <Tooltip v-if="rowAt(entry.i)?.isTruncated">
                  <TooltipTrigger as-child>
                    <Badge variant="chip" class="bg-field text-subtle ml-1.5 shrink-0">{{
                      t('studio.common.truncated.status')
                    }}</Badge>
                  </TooltipTrigger>
                  <TooltipContent>{{
                    t('studio.common.cellTruncated.tooltip', { size: formatBytes(64 * 1024) })
                  }}</TooltipContent>
                </Tooltip>
              </div>
            </div>
          </template>
        </div>
      </div>
    </div>
  </div>
</template>
