<script setup lang="ts">
import { dbMcpRunQueryResultSchema } from '@shared/domain/dbmcp';
import { useQuery } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import type { SecondaryTabItem } from '@theme/components/SecondaryTabs.vue';
import TooltipDetails from '@theme/components/TooltipDetails.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { FieldError } from '@theme/components/ui/field';
import { Popover, PopoverAnchor, PopoverContent } from '@theme/components/ui/popover';
import { ResizeHandle } from '@theme/components/ui/resizable';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@theme/components/ui/table';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { type MessageKey, t } from '@theme/i18n';
import { useResizeObserver } from '@vueuse/core';
import { useVirtualRows } from '@workbench/behaviour/virtualRows';
import CopyIconButton from '@workbench/components/CopyIconButton.vue';
import QueryState from '@workbench/components/QueryState.vue';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { computed, ref } from 'vue';
import { loadMaskRules, maskRulesQueryKey } from '../state/maskRules';
import FlushEditor from './FlushEditor.vue';
import { usePendingResponse } from './pendingResponse';
import TabbedContent from './TabbedContent.vue';

// The held run_query result of a response-stage approval: the table is a view of the exact bytes
// the AI client receives on Send. Those bytes are shown on the Raw tab, and nothing here
// re-renders or re-masks them. `field` picks the held text: the exact result, or with a script the raw
// result it read.
const props = withDefaults(
  defineProps<{ requestId: string; connectionId: string; field?: 'text' | 'raw' }>(),
  { field: 'text' },
);

const response = usePendingResponse(() => props.requestId);
const maskRules = useQuery({
  queryKey: computed(() => maskRulesQueryKey(props.connectionId)),
  queryFn: () => loadMaskRules(props.connectionId),
});
const maskedColumns = computed(
  () => new Set((maskRules.data.value ?? []).map((r) => r.columnName.toLowerCase())),
);

const text = computed(() => response.data.value?.[props.field] ?? '');
// A script that failed leaves the raw result readable; only the sent text is the error.
const isError = computed(() => props.field === 'text' && response.data.value?.isError === true);

const parsed = computed(() => {
  if (!response.data.value || isError.value) return null;
  try {
    const result = dbMcpRunQueryResultSchema.safeParse(JSON.parse(text.value));
    return result.success ? result.data : null;
  } catch {
    return null;
  }
});

interface Column {
  label: string;
  title?: string;
  masked: boolean;
}
type Cell = string | null;

const grid = computed<{ columns: Column[]; rows: Cell[][] } | null>(() => {
  const r = parsed.value;
  if (!r) return null;
  const plain = (key: MessageKey): Column => ({ label: t(key), masked: false });
  switch (r.kind) {
    case 'empty':
      return null;
    case 'tabular':
      return {
        columns: r.columns.map((c) => ({
          label: c.name,
          title: c.dataType,
          masked: maskedColumns.value.has(c.name.toLowerCase()),
        })),
        rows: r.rows,
      };
    case 'document':
      return {
        columns: [plain('common.field.id.label'), plain('common.body.label')],
        rows: r.documents.map((d) => [d.id, d.body]),
      };
    case 'keyvalue':
      return {
        columns: [plain('studio.keyValue.column.field.label'), plain('common.field.value.label')],
        rows: r.entries.map((e) => [e.field, e.value]),
      };
    case 'stream':
      return {
        columns: [
          plain('common.field.key.label'),
          plain('studio.stream.column.timestamp.label'),
          plain('studio.http.pane.headers.label'),
          plain('studio.stream.column.attributes.label'),
          plain('common.body.label'),
        ],
        rows: r.messages.map((m) => [m.key, m.timestamp, m.headers, m.attrs, m.body]),
      };
  }
});

const view = ref<'table' | 'asSent'>('table');
const items = computed<SecondaryTabItem[]>(() => [
  {
    value: 'table',
    label: t('studio.definition.table.label'),
    testid: 'db-mcp-preview-view-table',
  },
  {
    value: 'asSent',
    label: t('common.raw.label'),
    testid: 'db-mcp-preview-view-as-sent',
  },
]);
// Never hide the bytes: an unparseable non-error result falls back to Raw alone.
const showTable = computed(() => view.value === 'table' && grid.value !== null);
const unreadable = computed(() => response.data.value && !isError.value && parsed.value === null);

// What sits under the table: truncation, shortened cells, Redis type and TTL, the engine's note.
const footnote = computed(() => {
  const r = parsed.value;
  if (!r) return '';
  if (r.kind === 'empty')
    return [t('studio.common.noRows.empty'), r.note].filter(Boolean).join(' · ');
  const parts: string[] = [];
  if (r.truncated) parts.push(t('studio.dbMcp.preview.truncated.status'));
  if (r.kind === 'tabular' && r.truncatedCells > 0) {
    parts.push(t('studio.dbMcp.preview.truncatedCells.status', { count: r.truncatedCells }));
  }
  if (r.kind === 'keyvalue') {
    parts.push(t('studio.dbMcp.preview.redisType.status', { type: r.redisType }));
    if (r.ttlMs !== undefined) parts.push(t('studio.dbMcp.preview.ttl.status', { ms: r.ttlMs }));
  }
  if (r.note) parts.push(r.note);
  return parts.join(' · ');
});

const { single: rowHeight } = useRowHeight();
// Heights a row's drag handle set; other rows keep the single height, or their natural one while expanded.
const heights = ref<Record<number, number>>({});
// A click on a row expands it in place: every cell wraps and nothing is cropped. The natural height
// is measured once rendered, so the virtualizer keeps known heights.
const expanded = ref<Record<number, number>>({});
const heightOf = (r: number): number => heights.value[r] ?? expanded.value[r] ?? rowHeight.value;
const isOpenRow = (r: number): boolean => r in expanded.value || r in heights.value;
function toggleRow(r: number): void {
  if (isOpenRow(r)) {
    const { [r]: _a, ...restE } = expanded.value;
    const { [r]: _b, ...restH } = heights.value;
    expanded.value = restE;
    heights.value = restH;
    return;
  }
  expanded.value = { ...expanded.value, [r]: rowHeight.value };
}
function setHeight(r: number, h: number): void {
  heights.value = { ...heights.value, [r]: h };
}
function resetHeight(r: number): void {
  const { [r]: _, ...rest } = heights.value;
  heights.value = rest;
}
const rowHeights = computed(() =>
  Object.keys(heights.value).length || Object.keys(expanded.value).length
    ? Array.from({ length: grid.value?.rows.length ?? 0 }, (_, r) => heightOf(r))
    : undefined,
);
// One popover for the whole table, anchored on the cell that asked for it.
const cellOpen = ref(false);
const cellAnchor = ref<HTMLElement | null>(null);
const cellValue = ref('');
const cellLabel = ref('');
function showCell(e: MouseEvent, r: number, c: number): void {
  const cell = grid.value?.rows[r]?.[c];
  if (cell === null || cell === undefined) return;
  cellAnchor.value = (e.currentTarget as HTMLElement).closest('td');
  cellLabel.value = t('studio.cellEditor.target.label', {
    target: grid.value?.columns[c]?.label ?? '',
    row: r + 1,
  });
  cellValue.value = cell;
  cellOpen.value = true;
}
// The one expand button mounts in the hovered (or, for keyboard, focused-row) cell only.
const activeCell = ref<{ row: number; col: number } | null>(null);
function firstFilledCol(r: number): number {
  return Math.max(
    0,
    (grid.value?.rows[r] ?? []).findIndex((cell) => cell !== null),
  );
}
function trackCell(e: Event, r: number): void {
  const td = (e.target as HTMLElement).closest<HTMLElement>('td[data-col]');
  if (td) activeCell.value = { row: r, col: Number(td.dataset.col) };
  else if (activeCell.value?.row !== r) activeCell.value = { row: r, col: firstFilledCol(r) };
}
function releaseCell(e: Event, r: number): void {
  // The open popover is anchored to this cell; its focus and the pointer leaving must not drop it.
  if (activeCell.value?.row !== r || cellOpen.value) return;
  const row = e.currentTarget as HTMLElement;
  const focused =
    e instanceof FocusEvent ? (e.relatedTarget as Node | null) : document.activeElement;
  if (focused && row.contains(focused)) return;
  activeCell.value = null;
}
const scrollRef = ref<HTMLElement | null>(null);
const {
  virtualItems: virtualRows,
  totalSize,
  onScroll,
} = useVirtualRows({
  count: () => grid.value?.rows.length ?? 0,
  rowHeight: () => rowHeight.value,
  rowHeights: () => rowHeights.value,
  scrollElement: scrollRef,
  overscan: 8,
});
const shownRows = computed(() => virtualRows.value.map((vr) => vr.index));
// An expanded row's natural height: re-read whenever the table resizes (a row opens, the width wraps).
useResizeObserver(
  () => scrollRef.value?.querySelector('table') ?? null,
  () => {
    const next = { ...expanded.value };
    let changed = false;
    for (const tr of scrollRef.value?.querySelectorAll<HTMLElement>('tr[data-expanded]') ?? []) {
      const r = Number(tr.dataset.row);
      if (r in heights.value || !(r in next)) continue;
      const h = Math.max(rowHeight.value, tr.offsetHeight);
      if (next[r] !== h) {
        next[r] = h;
        changed = true;
      }
    }
    if (changed) expanded.value = next;
  },
);
const padTop = computed(() => virtualRows.value[0]?.start ?? 0);
const padBottom = computed(() => {
  const last = virtualRows.value[virtualRows.value.length - 1];
  return last ? totalSize.value - last.end : 0;
});
</script>

<template>
  <div data-testid="db-mcp-preview">
    <QueryState :query="response" variant="inline" :is-empty="() => false">
      <FieldError v-if="isError" class="whitespace-pre-wrap" data-testid="db-mcp-preview-error">{{
        text
      }}</FieldError>
      <TabbedContent
        v-else
        v-model="view"
        :tabs="grid !== null"
        :first-line="showTable ? rowHeight : undefined"
        :items="items"
        :label="t('studio.dbMcp.preview.view.aria')"
      >
        <div class="flex flex-col gap-1.5">
          <span v-if="unreadable" class="text-kira-sm text-subtle">{{
            t('studio.dbMcp.preview.unreadable.hint')
          }}</span>
          <div
            v-if="showTable && grid"
            class="flex flex-col gap-1.5"
            data-testid="db-mcp-preview-table"
          >
            <div ref="scrollRef" class="max-h-72 overflow-auto" @scroll.passive="onScroll">
              <Table class="w-auto">
                <TableHeader class="static bg-transparent">
                  <TableRow>
                    <TableHead class="w-5 px-0" aria-hidden="true" />
                    <TableHead
                      v-for="(col, c) in grid.columns"
                      :key="c"
                      data-testid="db-mcp-preview-column"
                    >
                      <Tooltip :disabled="!col.title && !col.masked">
                        <TooltipTrigger as-child>
                          <span class="inline-flex items-center gap-1">
                            {{ col.label }}
                            <CodiconIcon v-if="col.masked" name="lock" :size="12" />
                          </span>
                        </TooltipTrigger>
                        <TooltipContent>
                          <TooltipDetails
                            :title="col.label"
                            :rows="
                              col.title
                                ? [{ label: t('common.field.type.label'), value: col.title }]
                                : []
                            "
                            :hint="col.masked ? t('studio.dbMcp.preview.masked.hint') : undefined"
                          />
                        </TooltipContent>
                      </Tooltip>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <TableRow
                    v-if="padTop > 0"
                    aria-hidden="true"
                    class="border-0 hover:bg-transparent"
                    :style="{ height: `${padTop}px` }"
                  >
                    <TableCell :colspan="grid.columns.length + 1" class="p-0" />
                  </TableRow>
                  <TableRow
                    v-for="r in shownRows"
                    :key="r"
                    class="group/row relative"
                    :style="heights[r] ? { height: `${heights[r]}px` } : undefined"
                    tabindex="0"
                    :aria-expanded="isOpenRow(r)"
                    :data-row="r"
                    :data-expanded="isOpenRow(r) ? '' : undefined"
                    data-testid="db-mcp-preview-row"
                    @click="toggleRow(r)"
                    @mouseover="trackCell($event, r)"
                    @focusin="trackCell($event, r)"
                    @mouseleave="releaseCell($event, r)"
                    @focusout="releaseCell($event, r)"
                    @keydown.enter.self.prevent="toggleRow(r)"
                    @keydown.space.self.prevent="toggleRow(r)"
                  >
                    <TableCell class="w-6 px-0 align-top">
                      <TooltipIconButton
                        :icon="isOpenRow(r) ? 'chevron-down' : 'chevron-right'"
                        :icon-size="12"
                        variant="toolbar"
                        size="title"
                        class="group-hover/row:opacity-100 focus-visible:opacity-100"
                        :class="isOpenRow(r) ? 'opacity-100' : 'opacity-0'"
                        :aria-expanded="isOpenRow(r)"
                        :label="
                          t(
                            isOpenRow(r)
                              ? 'studio.dbMcp.preview.rowCollapse.tooltip'
                              : 'studio.dbMcp.preview.rowExpand.hint',
                          )
                        "
                        data-testid="db-mcp-preview-row-toggle"
                        @click.stop="toggleRow(r)"
                      />
                      <ResizeHandle
                        axis="y"
                        :size="heightOf(r)"
                        :min="rowHeight"
                        :max="480"
                        :label="t('studio.dbMcp.preview.rowResize.aria')"
                        class="absolute inset-x-0 -bottom-px z-1"
                        :tabindex="0"
                        data-testid="db-mcp-preview-row-resize"
                        @live="(h: number) => setHeight(r, h)"
                        @commit="(h: number) => setHeight(r, h)"
                        @click.stop
                        @dblclick.stop="resetHeight(r)"
                      />
                    </TableCell>
                    <TableCell
                      v-for="(cell, c) in grid.rows[r]"
                      :key="c"
                      :data-col="c"
                      class="relative max-w-80 align-top font-data"
                      :data-null="cell === null ? '' : undefined"
                    >
                      <span v-if="cell === null" class="italic text-muted-foreground">{{
                        t('studio.common.nullValue.label')
                      }}</span>
                      <div
                        v-else-if="heights[r]"
                        class="overflow-hidden break-words whitespace-pre-wrap"
                        :style="{ maxHeight: `${heights[r] - 8}px` }"
                      >
                        {{ cell }}
                      </div>
                      <div v-else-if="r in expanded" class="break-words whitespace-pre-wrap pr-4">
                        {{ cell }}
                      </div>
                      <span v-else class="block truncate pr-4">{{ cell }}</span>
                      <TooltipIconButton
                        v-if="cell !== null && activeCell?.row === r && activeCell.col === c"
                        icon="screen-full"
                        :icon-size="12"
                        variant="toolbar"
                        size="title"
                        class="absolute top-0.5 right-0.5 bg-elevated"
                        :label="t('studio.dbMcp.preview.cellShow.tooltip')"
                        data-testid="db-mcp-preview-expand"
                        @click.stop="(e: MouseEvent) => showCell(e, r, c)"
                      />
                    </TableCell>
                  </TableRow>
                  <TableRow
                    v-if="padBottom > 0"
                    aria-hidden="true"
                    class="border-0 hover:bg-transparent"
                    :style="{ height: `${padBottom}px` }"
                  >
                    <TableCell :colspan="grid.columns.length + 1" class="p-0" />
                  </TableRow>
                </TableBody>
              </Table>
            </div>
            <Popover v-model:open="cellOpen">
              <PopoverAnchor :reference="cellAnchor ?? undefined" />
              <PopoverContent
                align="start"
                class="w-120 max-w-[80vw]"
                data-testid="db-mcp-preview-cell"
              >
                <div
                  class="flex items-center justify-between gap-2 text-kira-sm text-muted-foreground"
                >
                  <span>{{ cellLabel }}</span>
                  <span class="flex items-center gap-1">
                    {{ t('studio.dbMcp.preview.cellChars.status', { count: cellValue.length }) }}
                    <CopyIconButton
                      :text="cellValue"
                      :label="t('common.action.copy.label')"
                      variant="toolbar"
                      size="title"
                      data-testid="db-mcp-preview-cell-copy"
                    />
                  </span>
                </div>
                <pre
                  class="m-0 max-h-80 overflow-auto font-data text-kira-md break-words whitespace-pre-wrap"
                >{{ cellValue }}</pre>
              </PopoverContent>
            </Popover>
          </div>
          <FlushEditor v-else :doc="text" language="json" data-testid="db-mcp-preview-raw" />
          <span
            v-if="footnote"
            class="text-kira-sm text-subtle"
            data-testid="db-mcp-preview-summary"
            >{{
              footnote
            }}</span
          >
        </div>
      </TabbedContent>
    </QueryState>
  </div>
</template>
