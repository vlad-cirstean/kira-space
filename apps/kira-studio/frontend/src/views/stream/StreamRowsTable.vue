<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import Spinner from '@theme/components/Spinner.vue';
import { Badge } from '@theme/components/ui/badge';
import { ResizeHandle } from '@theme/components/ui/resizable';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { type MessageKey, t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { useEventListener } from '@vueuse/core';
import { formatBytes } from '@workbench/behaviour/format';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { useVirtualRows, VIRTUAL_ROW_CLASS } from '@workbench/behaviour/virtualRows';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, ref, watch } from 'vue';
import { type SelectedCell, useCellSelectionStore } from '../../state/cellSelection';
import type { StreamTabRecord } from '../../state/tabDomain';
import { useTabsStore } from '../../state/tabs';
import { datasetNumber } from '../shared/eventCoords';
import SearchFilterNotice from '../shared/page/SearchFilterNotice.vue';
import { setVisibleRows } from '../shared/page/visibleRows';
import { rowMenu } from './menu';
import { pageVersion, setVisibleWindow } from './page';
import StreamSearchToolbar from './StreamSearchToolbar.vue';
import { useStreamSearchStore } from './search';
import { useStreamViewStore } from './state';
import { useStreamGate } from './useStreamGate';

const props = defineProps<{ tab: StreamTabRecord }>();

const cellSelectionStore = useCellSelectionStore();
const contextMenuStore = useContextMenuStore();
const streamSearchStore = useStreamSearchStore();
const streamViewStore = useStreamViewStore();
const tabsStore = useTabsStore();
const { rt, page, isBatch, rowAt } = useStreamGate(() => props.tab);

type CellColumn = 'key' | 'timestamp' | 'headers' | 'attrs' | 'body';
const RESIZABLE = ['key', 'timestamp', 'headers', 'attrs'] as const;
const COLUMN_LABEL_KEYS = {
  key: 'common.field.key.label',
  timestamp: 'studio.stream.column.timestamp.label',
  headers: 'studio.http.pane.headers.label',
  attrs: 'studio.stream.column.attributes.label',
  body: 'common.body.label',
} as const satisfies Record<CellColumn, MessageKey>;
function columnLabel(id: CellColumn): string {
  return t(COLUMN_LABEL_KEYS[id]);
}
function resizeLabel(id: CellColumn): string {
  return t('studio.stream.resizeColumn.aria', { column: columnLabel(id).toLowerCase() });
}

// Filtered rows keep their real row number (the `i + 1` gutter).
const displayRows = computed<number[] | null>(() => streamSearchStore.matchedRows(props.tab.id));
const rowIndices = computed(() => {
  void pageVersion.n;
  if (displayRows.value) return displayRows.value;
  return Array.from({ length: rt.value?.rowCount ?? 0 }, (_, i) => i);
});

const { single: rowHeight } = useRowHeight();
const scrollEl = ref<HTMLElement | null>(null);
const { virtualItems, totalSize, onScroll, scrollToIndex } = useVirtualRows({
  count: () => rowIndices.value.length,
  rowHeight: () => rowHeight.value,
  scrollElement: scrollEl,
});

function onRowClick(i: number): void {
  streamViewStore.selectRow(props.tab.id, i);
}

function rowMenuFor(row: number) {
  return rowMenu(rowAt(row)?.key ?? null, rowAt(row)?.body ?? null, (m) =>
    streamViewStore.setActionError(props.tab.id, m),
  );
}

const nav = useListNavigation<number>({
  container: scrollEl,
  items: () => rowIndices.value,
  key: (row) => String(row),
  activeKey: computed({
    get: () => (rt.value?.selectedRow == null ? null : String(rt.value.selectedRow)),
    set: (key) => {
      if (key !== null) onRowClick(Number(key));
    },
  }),
  onSelect: onRowClick,
  onMenu: (row, rect) =>
    contextMenuStore.openContextMenuAt(rect.left, rect.bottom, rowMenuFor(row), {
      onRestoreFocus: () => void nav.focusKey(String(row)),
    }),
  scrollToIndex: (index) => scrollToIndex(index),
  rendered: () => virtualItems.value,
});

// Delegated off the container: one listener instead of one per cell.
useEventListener(scrollEl, 'contextmenu', (e: MouseEvent) => {
  const i = datasetNumber(e.target, 'rowIndex');
  if (i === null) return;
  e.preventDefault();
  nav.contextMenu(i, e, rowMenuFor(i));
});

// A cell click selects the row and publishes that column's value read-only to the cell editor
// dock. A stream row has no catalog schema, so a synthetic column descriptor stands in.
function onCellClick(i: number, column: CellColumn): void {
  streamViewStore.selectRow(props.tab.id, i);
  const row = rowAt(i);
  const isBody = column === 'body';
  const value = row?.[column] ?? null;
  const binary = (isBody ? row?.bodyBinary : column === 'key' && row?.keyBinary) ?? false;
  const selected: SelectedCell = {
    tabId: props.tab.id,
    connectionId: props.tab.connectionId,
    path: props.tab.path,
    columnIndex: 0,
    column: {
      name: column,
      dataType: binary ? 'base64' : 'text',
      typeClass: 'text',
      nullable: value === null,
      isPrimaryKey: false,
      generated: false,
    },
    row: i,
    value,
    truncated: isBody && (row?.isTruncated ?? false),
    hasPrimaryKey: true,
  };
  cellSelectionStore.publishSelectedCell(selected);
}

// Stable handlers recover the row and column from data attributes: a v-for-scoped closure would
// be recreated on every render, scroll included.
function cellOf(e: Event): { i: number; column: CellColumn } | null {
  const i = datasetNumber(e.currentTarget, 'rowIndex');
  const column = (e.currentTarget as HTMLElement).dataset.col as CellColumn | undefined;
  return i === null || column === undefined ? null : { i, column };
}
function onGutterClick(e: MouseEvent): void {
  const i = datasetNumber(e.currentTarget, 'rowIndex');
  if (i !== null) onRowClick(i);
}
// Each column is its own role="option" (Biome's useSemanticElements rejects grid markup); the
// gutter is the row's navigation stop. List navigation claims Enter and the arrows on the gutter.
function onGutterKeydown(e: KeyboardEvent): void {
  if (e.key !== ' ') return;
  e.preventDefault();
  const i = datasetNumber(e.currentTarget, 'rowIndex');
  if (i !== null) onRowClick(i);
}
function onCellClickFromEvent(e: MouseEvent): void {
  const cell = cellOf(e);
  if (cell) onCellClick(cell.i, cell.column);
}
function onCellKeydown(e: KeyboardEvent): void {
  if (e.key !== 'Enter' && e.key !== ' ') return;
  e.preventDefault();
  e.stopPropagation();
  const cell = cellOf(e);
  if (cell) onCellClick(cell.i, cell.column);
}

// A new page invalidates the published cell as it does the selected row.
watch(page, () => cellSelectionStore.clearSelectedCellFor(props.tab.id));

const matchSet = computed(
  () => new Set(streamSearchStore.searchState[props.tab.id]?.matches ?? []),
);
const currentMatchRow = computed(() => {
  const s = streamSearchStore.searchState[props.tab.id];
  return s && s.index >= 0 ? (s.matches[s.index] ?? null) : null;
});

// Priority: current match beats match beats selected beats hover.
function streamRowClass(i: number): string {
  const current = currentMatchRow.value === i;
  const match = matchSet.value.has(i);
  return cn(
    rowVariants({ layout: 'tree', selected: current || match || rt.value?.selectedRow === i }),
    'gap-0 border-b border-border px-0 h-row',
    VIRTUAL_ROW_CLASS,
    current ? 'bg-search-match-current text-bg' : match && 'bg-search-match',
  );
}

function onGoToMatch(row: number): void {
  const index = rowIndices.value.indexOf(row);
  if (index >= 0) scrollToIndex(index);
}

// Reports the visible window so page.ts prunes its decode cache and finds prioritise on-screen rows.
watch(virtualItems, (items) => {
  if (items.length === 0) return;
  const list = rowIndices.value;
  const from = list[items[0].index];
  const to = list[items[items.length - 1].index];
  if (from === undefined || to === undefined) return;
  setVisibleRows(props.tab.id, from, to + 1);
  setVisibleWindow(props.tab.id, from, to + 1);
});

const DEFAULT_COLUMN_WIDTHS: Record<string, number> = {
  key: 160,
  timestamp: 160,
  headers: 140,
  attrs: 140,
};

// A resize drag previews through this local ref and writes tab state once on commit: a per-pixel
// patch would re-render the header and re-arm the debounced save of every tab.
const liveResizeWidth = ref<{ column: string; width: number } | null>(null);
function widthFor(column: string): number {
  if (liveResizeWidth.value?.column === column) return liveResizeWidth.value.width;
  return props.tab.state.columnWidths[column] ?? DEFAULT_COLUMN_WIDTHS[column] ?? 96;
}
function onResizeCommit(column: string, width: number): void {
  tabsStore.patchStreamTabState(props.tab.id, {
    columnWidths: { ...props.tab.state.columnWidths, [column]: width },
  });
  liveResizeWidth.value = null;
}
// A drag ended by pointercancel never commits: clear only this column's preview, not another drag's.
function onResizeCancel(column: string): void {
  if (liveResizeWidth.value?.column === column) liveResizeWidth.value = null;
}

const CELL_BASE =
  'flex items-center px-2 border-r border-b border-border font-data text-kira-md text-fg truncate';
</script>

<template>
  <StreamSearchToolbar
    v-if="rt?.searchOpen"
    :tab-id="tab.id"
    :is-open="() => !!streamViewStore.runtime[tab.id]?.searchOpen"
    @go-to-match="onGoToMatch"
    @close="streamViewStore.setSearchOpen(tab.id, false)"
  />

  <div class="relative flex-1 min-h-0 flex flex-col overflow-hidden" data-testid="stream-list">
    <EmptyState
      v-if="isBatch && !rt?.polled"
      class="no-rows absolute inset-0"
      icon="arrow-swap"
      :title="t('studio.stream.pollPrompt.empty')"
    />
    <EmptyState
      v-else-if="!rt || rt.rowCount === 0"
      class="no-rows absolute inset-0"
      :icon="rt ? 'inbox' : undefined"
      :title="rt ? t('studio.stream.noMessages.empty') : t('common.state.loading.loading')"
    >
      <template v-if="!rt" #media><Spinner :size="24" /></template>
    </EmptyState>
    <SearchFilterNotice
      v-else-if="displayRows && displayRows.length === 0"
      class="no-rows absolute inset-0"
      data-testid="stream-no-matching-rows"
      :filter-key="tab.id"
      show-all-testid="stream-show-all-rows"
    />
    <template v-else>
      <div class="h-control-lg shrink-0 flex bg-elevated border-b border-border">
        <div
          class="flex items-center gap-1 px-2 border-r border-border text-kira-sm text-muted-foreground overflow-hidden whitespace-nowrap relative w-10"
        />
        <div
          v-for="column in RESIZABLE"
          :key="column"
          class="flex items-center gap-1 px-2 border-r border-border text-kira-sm text-muted-foreground overflow-hidden whitespace-nowrap relative"
          :style="{ width: `${widthFor(column)}px` }"
        >
          <span class="text-fg truncate">{{ columnLabel(column) }}</span>
          <ResizeHandle
            class="absolute inset-y-0 right-0 z-1"
            draggable="false"
            :data-testid="`stream-column-resize-${column}`"
            :label="resizeLabel(column)"
            :size="widthFor(column)"
            :min="40"
            @live="(w) => (liveResizeWidth = { column, width: w })"
            @commit="(w) => onResizeCommit(column, w)"
            @cancel="() => onResizeCancel(column)"
            @click.stop
          />
        </div>
        <div
          class="flex items-center gap-1 px-2 border-r border-border text-kira-sm text-muted-foreground overflow-hidden whitespace-nowrap relative flex-1"
        >
          <span class="text-fg truncate">{{ columnLabel('body') }}</span>
        </div>
      </div>
      <div
        ref="scrollEl"
        class="flex-1 min-h-0 overflow-auto"
        data-testid="virtual-list"
        role="listbox"
        :aria-label="t('studio.stream.rows.aria')"
        @scroll="onScroll"
      >
        <div class="relative" :style="{ height: `${totalSize}px` }">
          <div
            v-for="vi in virtualItems"
            :key="String(vi.key)"
            data-testid="stream-row"
            :data-row-index="rowIndices[vi.index]"
            :style="{ transform: `translateY(${vi.start}px)`, height: `${vi.size}px` }"
            :class="streamRowClass(rowIndices[vi.index])"
          >
            <div
              class="flex items-center justify-end px-2 border-r border-b border-border bg-elevated font-data text-kira-sm text-subtle truncate relative w-10"
              role="option"
              v-bind="nav.rowProps(String(rowIndices[vi.index]))"
              :tabindex="nav.tabindexFor(String(rowIndices[vi.index]))"
              :aria-selected="rt?.selectedRow === rowIndices[vi.index]"
              @click="onGutterClick"
              @keydown="onGutterKeydown"
            >
              {{ rowIndices[vi.index] + 1 }}
            </div>
            <div
              :class="[
                CELL_BASE,
                rowAt(rowIndices[vi.index])?.key === null && 'italic text-subtle',
              ]"
              :style="{ width: `${widthFor('key')}px` }"
              data-testid="stream-key"
              data-col="key"
              role="option"
              tabindex="0"
              :aria-selected="rt?.selectedRow === rowIndices[vi.index]"
              @click="onCellClickFromEvent"
              @keydown="onCellKeydown"
            >
              <Badge
                v-if="rowAt(rowIndices[vi.index])?.keyBinary"
                data
                class="mr-1"
                data-testid="stream-key-binary"
                >{{
                  t('studio.cellEditor.format.base64.label')
                }}</Badge
              >
              {{ rowAt(rowIndices[vi.index])?.key ?? t('common.state.none.option') }}
            </div>
            <div
              v-for="column in ['timestamp', 'headers', 'attrs'] as const"
              :key="column"
              :class="CELL_BASE"
              :style="{ width: `${widthFor(column)}px` }"
              :data-testid="`stream-${column}`"
              :data-col="column"
              role="option"
              tabindex="0"
              :aria-selected="rt?.selectedRow === rowIndices[vi.index]"
              @click="onCellClickFromEvent"
              @keydown="onCellKeydown"
            >
              {{ rowAt(rowIndices[vi.index])?.[column] ?? '' }}
            </div>
            <div
              class="flex items-center px-2 border-r border-b border-border font-data truncate flex-1 text-muted-foreground text-kira-sm"
              data-testid="stream-body"
              data-col="body"
              role="option"
              :aria-label="
                rowAt(rowIndices[vi.index])?.body === null
                  ? t('studio.stream.tombstone.tooltip')
                  : undefined
              "
              tabindex="0"
              :aria-selected="rt?.selectedRow === rowIndices[vi.index]"
              @click="onCellClickFromEvent"
              @keydown="onCellKeydown"
            >
              <Tooltip v-if="rowAt(rowIndices[vi.index])?.body === null">
                <TooltipTrigger as-child>
                  <span class="text-subtle italic" data-testid="stream-body-null">{{
                    t('studio.common.nullValue.label')
                  }}</span>
                </TooltipTrigger>
                <TooltipContent>{{ t('studio.stream.tombstone.tooltip') }}</TooltipContent>
              </Tooltip>
              <template v-else>
                <Badge
                  v-if="rowAt(rowIndices[vi.index])?.bodyBinary"
                  class="mr-1"
                  data-testid="stream-body-binary"
                  >{{
                    t('studio.cellEditor.format.base64.label')
                  }}</Badge
                >
                {{ rowAt(rowIndices[vi.index])?.body }}
              </template>
              <Tooltip v-if="rowAt(rowIndices[vi.index])?.isTruncated">
                <TooltipTrigger as-child>
                  <span class="text-kira-sm text-muted-foreground">{{
                    t('studio.common.truncated.status')
                  }}</span>
                </TooltipTrigger>
                <TooltipContent>{{
                  t('studio.common.cellTruncated.tooltip', { size: formatBytes(64 * 1024) })
                }}</TooltipContent>
              </Tooltip>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
