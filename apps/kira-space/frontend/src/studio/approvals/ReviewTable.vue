<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipDetails from '@theme/components/TooltipDetails.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
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
import { t } from '@theme/i18n';
import { useResizeObserver } from '@vueuse/core';
import { useVirtualRows } from '@workbench/behaviour/virtualRows';
import CopyIconButton from '@workbench/components/CopyIconButton.vue';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { computed, nextTick, ref, watch } from 'vue';

// A held result as a grid, every row: click expands a row in place, its handle resizes it, a cell opens a
// popover with Copy. Rows are virtualized. Whatever the MCP, the grid shows cells verbatim.
interface ReviewColumn {
  label: string;
  title?: string;
  masked?: boolean;
}
const props = defineProps<{
  columns: readonly ReviewColumn[];
  rows: readonly (string | null)[][];
  /** Test id prefix: `${testid}-table`, `-row`, `-column`, `-expand`, `-cell`. */
  testid: string;
  maskHint?: string;
}>();

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
    ? Array.from({ length: props.rows.length }, (_, r) => heightOf(r))
    : undefined,
);
// One popover for the whole table, anchored on the cell that asked for it.
const cellOpen = ref(false);
const cellAnchor = ref<HTMLElement | null>(null);
const cellValue = ref('');
const cellLabel = ref('');
function showCell(e: MouseEvent, r: number, c: number): void {
  const cell = props.rows[r]?.[c];
  if (cell === null || cell === undefined) return;
  cellAnchor.value = (e.currentTarget as HTMLElement).closest('td');
  cellLabel.value = t('studio.cellEditor.target.label', {
    target: props.columns[c]?.label ?? '',
    row: r + 1,
  });
  cellValue.value = cell;
  cellOpen.value = true;
}
// The one expand button mounts in the hovered cell and in the keyboard-focused cell. Separate refs:
// pointer motion must not unmount the button that holds focus or opened the popover.
type CellPos = { row: number; col: number };
const hoverCell = ref<CellPos | null>(null);
const focusCell = ref<CellPos | null>(null);
const isActiveCell = (r: number, c: number): boolean =>
  (hoverCell.value?.row === r && hoverCell.value.col === c) ||
  (focusCell.value?.row === r && focusCell.value.col === c);
function firstFilledCol(r: number): number {
  return Math.max(
    0,
    (props.rows[r] ?? []).findIndex((cell) => cell !== null),
  );
}
// Cell under the event, else the row's first filled cell (chevron, handle, row gap).
function cellAt(e: Event, r: number, current: CellPos | null): CellPos | null {
  const td = (e.target as HTMLElement).closest<HTMLElement>('td[data-col]');
  if (td) return { row: r, col: Number(td.dataset.col) };
  return current?.row === r ? null : { row: r, col: firstFilledCol(r) };
}
function trackCell(e: Event, r: number, focus: boolean): void {
  // The open popover is anchored on one cell; pointer motion elsewhere must not move the button.
  if (cellOpen.value && !focus) return;
  const target = focus ? focusCell : hoverCell;
  const next = cellAt(e, r, target.value);
  if (!next || (target.value?.row === next.row && target.value.col === next.col)) return;
  target.value = next;
}
const rowEl = (r: number): HTMLElement | null =>
  scrollRef.value?.querySelector<HTMLElement>(`tr[data-row="${r}"]`) ?? null;
// Open popover is anchored to this cell; its focus and the pointer leaving must not drop it.
function releaseHover(r: number): void {
  if (hoverCell.value?.row !== r || cellOpen.value) return;
  hoverCell.value = null;
}
let movingFocus = false;
function releaseFocus(e: FocusEvent, r: number): void {
  if (focusCell.value?.row !== r || cellOpen.value || movingFocus) return;
  const next = e.relatedTarget as Node | null;
  if (next && (e.currentTarget as HTMLElement).contains(next)) return;
  focusCell.value = null;
}
// Closing the popover re-evaluates what the pointer and focus left behind.
watch(cellOpen, async (open) => {
  if (open) return;
  await nextTick();
  const hr = hoverCell.value?.row;
  if (hr !== undefined && !rowEl(hr)?.matches(':hover')) hoverCell.value = null;
  const fr = focusCell.value?.row;
  if (fr !== undefined && !rowEl(fr)?.contains(document.activeElement)) focusCell.value = null;
});
// Focus the popover itself, not its Copy button: a focus-opened tooltip would take the first Esc.
function focusPopover(e: Event): void {
  e.preventDefault();
  (e.currentTarget as HTMLElement | null)?.focus();
}
// ArrowLeft/Right on the expand button roves across the row's filled cells.
async function moveCell(r: number, from: number, dir: 1 | -1): Promise<void> {
  const cells = props.rows[r] ?? [];
  for (let c = from + dir; c >= 0 && c < cells.length; c += dir) {
    if (cells[c] === null) continue;
    movingFocus = true;
    focusCell.value = { row: r, col: c };
    await nextTick();
    rowEl(r)
      ?.querySelector<HTMLElement>(`td[data-col="${c}"] [data-testid="${props.testid}-expand"]`)
      ?.focus();
    movingFocus = false;
    return;
  }
}
const scrollRef = ref<HTMLElement | null>(null);
const {
  virtualItems: virtualRows,
  totalSize,
  onScroll,
} = useVirtualRows({
  count: () => props.rows.length,
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
  <div class="flex flex-col gap-1.5" :data-testid="`${testid}-table`">
    <div ref="scrollRef" class="max-h-72 overflow-auto" @scroll.passive="onScroll">
      <Table class="w-auto">
        <TableHeader class="static bg-transparent">
          <TableRow>
            <TableHead class="w-5 px-0" aria-hidden="true" />
            <TableHead v-for="(col, c) in columns" :key="c" :data-testid="`${testid}-column`">
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
                      col.title ? [{ label: t('common.field.type.label'), value: col.title }] : []
                    "
                    :hint="col.masked ? maskHint : undefined"
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
            <TableCell :colspan="columns.length + 1" class="p-0" />
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
            :data-testid="`${testid}-row`"
            @click="toggleRow(r)"
            @mouseover="trackCell($event, r, false)"
            @focusin="trackCell($event, r, true)"
            @mouseleave="releaseHover(r)"
            @focusout="releaseFocus($event, r)"
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
                :data-testid="`${testid}-row-toggle`"
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
                :data-testid="`${testid}-row-resize`"
                @live="(h: number) => setHeight(r, h)"
                @commit="(h: number) => setHeight(r, h)"
                @click.stop
                @dblclick.stop="resetHeight(r)"
              />
            </TableCell>
            <TableCell
              v-for="(cell, c) in rows[r]"
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
                v-if="cell !== null && isActiveCell(r, c)"
                icon="screen-full"
                :icon-size="12"
                variant="toolbar"
                size="title"
                class="absolute top-0.5 right-0.5 bg-elevated"
                :label="t('studio.dbMcp.preview.cellShow.tooltip')"
                :data-testid="`${testid}-expand`"
                @click.stop="(e: MouseEvent) => showCell(e, r, c)"
                @keydown.left.prevent="moveCell(r, c, -1)"
                @keydown.right.prevent="moveCell(r, c, 1)"
              />
            </TableCell>
          </TableRow>
          <TableRow
            v-if="padBottom > 0"
            aria-hidden="true"
            class="border-0 hover:bg-transparent"
            :style="{ height: `${padBottom}px` }"
          >
            <TableCell :colspan="columns.length + 1" class="p-0" />
          </TableRow>
        </TableBody>
      </Table>
    </div>
    <Popover v-model:open="cellOpen">
      <PopoverAnchor :reference="cellAnchor ?? undefined" />
      <PopoverContent
        align="start"
        class="w-120 max-w-[80vw]"
        :data-testid="`${testid}-cell`"
        @open-auto-focus="focusPopover"
      >
        <div class="flex items-center justify-between gap-2 text-kira-sm text-muted-foreground">
          <span>{{ cellLabel }}</span>
          <span class="flex items-center gap-1">
            {{ t('studio.dbMcp.preview.cellChars.status', { count: cellValue.length }) }}
            <CopyIconButton
              :text="cellValue"
              :label="t('common.action.copy.label')"
              variant="toolbar"
              size="title"
              :data-testid="`${testid}-cell-copy`"
            />
          </span>
        </div>
        <pre
          class="m-0 max-h-80 overflow-auto font-data text-kira-md break-words whitespace-pre-wrap"
        >{{ cellValue }}</pre>
      </PopoverContent>
    </Popover>
  </div>
</template>
