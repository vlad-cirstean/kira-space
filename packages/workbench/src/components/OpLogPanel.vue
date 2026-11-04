<script setup lang="ts" generic="R extends OpLogRecord">
import type { EditorLanguageId } from '@shared/domain/editor';
import type { OpLogRecord } from '@shared/domain/ops';
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import HighlightedText from '@theme/components/HighlightedText.vue';
import Spinner from '@theme/components/Spinner.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Skeleton } from '@theme/components/ui/skeleton';
import { tableHeadClass, tableHeadRowClass, tableRowClass } from '@theme/components/ui/table';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { cn } from '@theme/lib/utils';
import { useElementSize, useFocus, useIntervalFn } from '@vueuse/core';
import { computed, ref, watch } from 'vue';
import { useCopy } from '../behaviour/clipboard';
import {
  displayValue,
  formatClockTime,
  formatDateTime,
  formatDuration,
  formatElapsed,
} from '../behaviour/format';
import { useListNavigation } from '../behaviour/listNavigation';
import { splitMatches } from '../behaviour/match';
import { TIMING } from '../behaviour/timing';
import { useVirtualRows, VIRTUAL_ROW_CLASS } from '../behaviour/virtualRows';
import {
  fitColumns,
  formatOpTime,
  gridTemplateFor,
  type OpLogColumn,
  type OpLogDetailLine,
  type OpLogField,
  type OpLogStatusFilter,
  type OpLogStep,
  opLogMenuItems,
  STATUS_ICON,
  statusLabel,
} from '../lib/opLog';
import { type OpTreeRow, orderOpTree } from '../lib/opLogTree';
import { type MenuItem, useContextMenuStore } from '../state/contextMenu';
import OpLogDetail from './OpLogDetail.vue';
import OpLogToolbar from './OpLogToolbar.vue';

// Store-agnostic (props/emits only, no Pinia import here), generic over each app's own record
// shape so Kira Space's op log and Kira Studio's reuse it unchanged. `columns` states the column
// set as data; the panel fits it to its own width, orders parent/child ops as a tree and renders
// each open record as one measured detail item.
interface Props {
  records: readonly R[];
  runningCount: number;
  /** False while the first snapshot is in flight: the panel shows skeleton rows. */
  loading: boolean;
  filterText: string;
  statusFilter: OpLogStatusFilter;
  columns: readonly OpLogColumn[];
  /** Clear button tooltip — each app states its own retention story. */
  clearHint: string;
  /** One line of what lands here, under "No operations yet". */
  emptyHint: string;
  canCancel?: (record: R) => boolean;
  menuFor?: (record: R) => MenuItem[];
  /** Replaces the command cell of a running row while non-null; `now` is the shared clock. */
  runningNote?: (record: R, now: number) => string | null;
  /** Replaces the raw `kind` in the kind cell. */
  kindLabel?: (record: R) => string;
  /** Short text after the status icon, e.g. a repeat count. */
  statusNote?: (record: R) => string | null;
  /** Extra lines under an expanded record. */
  detailLines?: (record: R) => readonly OpLogDetailLine[];
  /** Id of the op this one ran under; a child renders under its parent when both are listed. */
  parentOf?: (record: R) => string | null;
  /** Labelled values in the expanded record, after Time and Duration. */
  fieldsFor?: (record: R) => readonly OpLogField[];
  stepsFor?: (record: R) => readonly OpLogStep[];
  /** Syntax colouring of the expanded command; null renders plain text. */
  codeLanguage?: (record: R) => EditorLanguageId | null;
  facet?: { label: string; options: readonly { value: string; label: string }[] };
  facetValues?: string[];
}
const props = defineProps<Props>();

const emit = defineEmits<{
  'update:filterText': [value: string];
  'update:statusFilter': [value: OpLogStatusFilter];
  'update:facetValues': [value: string[]];
  clear: [];
  cancel: [record: R];
  resetFilters: [];
}>();

defineSlots<{
  cell(props: { column: OpLogColumn; record: R; depth: number; viaParent: boolean }): unknown;
  toolbar?(): unknown;
}>();

const contextMenuStore = useContextMenuStore();
const { copy } = useCopy();

function canCancelRecord(record: R): boolean {
  return props.canCancel ? props.canCancel(record) : true;
}
function menuForRecord(record: R): MenuItem[] {
  return props.menuFor
    ? props.menuFor(record)
    : opLogMenuItems(record, () => emit('cancel', record), canCancelRecord(record), copy);
}

const selectedFacets = computed(() => props.facetValues ?? []);
const hasFilters = computed(
  () =>
    props.filterText.trim() !== '' ||
    props.statusFilter !== 'all' ||
    selectedFacets.value.length > 0,
);
const filterNeedle = computed(() => props.filterText.trim().toLowerCase());

const filterTextModel = computed({
  get: () => props.filterText,
  set: (value: string) => emit('update:filterText', value),
});

// Open details, by record id. Replaced rather than mutated, so a watcher sees each change.
const expandedIds = ref<ReadonlySet<string>>(new Set());
function setExpanded(id: string, open: boolean): void {
  if (expandedIds.value.has(id) === open) return;
  const next = new Set(expandedIds.value);
  if (open) next.add(id);
  else next.delete(id);
  expandedIds.value = next;
}
function toggleExpanded(record: R): void {
  setExpanded(record.id, !expandedIds.value.has(record.id));
}
watch(
  () => props.records,
  (records) => {
    if (expandedIds.value.size === 0) return;
    const present = new Set(records.map((r) => r.id));
    const kept = [...expandedIds.value].filter((id) => present.has(id));
    if (kept.length !== expandedIds.value.size) expandedIds.value = new Set(kept);
  },
);

const treeRows = computed(() => orderOpTree(props.records, props.parentOf));

type ListItem =
  | { key: string; kind: 'op'; row: OpTreeRow<R> }
  | { key: string; kind: 'detail'; record: R };

const listItems = computed<ListItem[]>(() => {
  const out: ListItem[] = [];
  for (const row of treeRows.value) {
    out.push({ key: row.record.id, kind: 'op', row });
    if (expandedIds.value.has(row.record.id)) {
      out.push({ key: `${row.record.id}:detail`, kind: 'detail', record: row.record });
    }
  }
  return out;
});

const OP_ROW_PX = 18;
const DETAIL_LINE_PX = 16;
const DETAIL_PAD_PX = 16;

// Space estimate for an unmeasured detail; the real height comes from measureElement.
function detailEstimate(record: R): number {
  const lines =
    3 +
    (props.fieldsFor?.(record).length ?? 0) +
    (record.command ? 3 : 0) +
    (record.error ? 3 : 0) +
    (props.stepsFor?.(record).length ?? 0) +
    (props.detailLines?.(record).length ?? 0);
  return lines * DETAIL_LINE_PX + DETAIL_PAD_PX;
}

const rootEl = ref<HTMLElement | null>(null);
const { width } = useElementSize(rootEl);
const fitted = computed(() => fitColumns(props.columns, width.value));
const gridTemplateColumns = computed(() => gridTemplateFor(fitted.value));

const scrollEl = ref<HTMLElement | null>(null);
const activeId = ref<string | null>(null);
const menuRowId = ref<string | null>(null);
const { virtualizer, virtualItems, totalSize, onScroll } = useVirtualRows({
  count: () => listItems.value.length,
  rowHeight: () => OP_ROW_PX,
  rowHeights: () =>
    listItems.value.map((item) => (item.kind === 'op' ? OP_ROW_PX : detailEstimate(item.record))),
  scrollElement: scrollEl,
  // The roving row stays mounted when it scrolls away, so it keeps focus.
  pinned: () => {
    const index = listItems.value.findIndex((item) => item.key === activeId.value);
    return index < 0 ? null : index;
  },
});

function measureRef(el: unknown): void {
  if (el instanceof HTMLElement) virtualizer.value.measureElement(el);
}

const rendered = computed(() =>
  virtualItems.value.flatMap((vi) => {
    const item = listItems.value[vi.index];
    return item ? [{ vi, item }] : [];
  }),
);

const filterInput = computed(() =>
  rootEl.value?.querySelector<HTMLInputElement>('[data-testid="ops-filter"]'),
);
const { focused: filterFocused } = useFocus(filterInput);

// One roving tab stop for the whole list: Tab enters at the current row, arrows move within it.
// Right opens a row's detail, Left closes it or goes to the parent op. The nested buttons keep their
// own Enter and Space.
const nav = useListNavigation<OpTreeRow<R>>({
  container: scrollEl,
  items: () => treeRows.value,
  key: (row) => row.record.id,
  activeKey: activeId,
  onOpen: (row) => toggleExpanded(row.record),
  onSelect: (row) => toggleExpanded(row.record),
  onMenu: (row, rect) =>
    openMenuAt(row.record, (items, options) =>
      contextMenuStore.openContextMenuAt(rect.left, rect.bottom, items, options),
    ),
  onExitTop: () => {
    filterFocused.value = true;
  },
  tree: {
    depth: (row) => row.depth,
    isExpanded: (row) => expandedIds.value.has(row.record.id),
    hasChildren: () => true,
    expand: (row) => setExpanded(row.record.id, true),
    collapse: (row) => setExpanded(row.record.id, false),
  },
  scrollToIndex: (_index, key) => {
    const at = listItems.value.findIndex((item) => item.key === key);
    if (at >= 0) virtualizer.value.scrollToIndex(at, { align: 'auto' });
  },
  rendered: () => virtualItems.value,
});

function openMenuAt(
  record: R,
  open: (items: MenuItem[], options: { onRestoreFocus: () => void }) => void,
): void {
  menuRowId.value = record.id;
  open(menuForRecord(record), { onRestoreFocus: () => void nav.focusKey(record.id) });
}

function onRowContextMenu(record: R, event: MouseEvent): void {
  void nav.focusKey(record.id);
  openMenuAt(record, (items, options) => contextMenuStore.openContextMenu(event, items, options));
}

function isSelected(id: string): boolean {
  return activeId.value === id || (contextMenuStore.open && menuRowId.value === id);
}

// One shared clock for every running row and the time column (rows are virtualized, so a per-row
// timer would churn).
const now = ref(Date.now());
const { pause, resume } = useIntervalFn(
  () => {
    now.value = Date.now();
  },
  TIMING.clockTick,
  { immediate: false },
);
watch(
  () => props.runningCount,
  (count) => {
    if (count > 0) {
      now.value = Date.now();
      resume();
    } else pause();
  },
  { immediate: true },
);

function runningNoteFor(record: R): string | null {
  return record.status === 'running' ? (props.runningNote?.(record, now.value) ?? null) : null;
}
function elapsedText(record: R): string {
  return formatElapsed(Math.max(0, now.value - Date.parse(record.startedAt)));
}
function durationText(ms: number | null): string {
  return displayValue(ms === null ? null : formatDuration(ms));
}
function timeTitle(record: R): string {
  return t('common.dotPair.label', {
    first: formatDateTime(record.startedAt),
    second: formatClockTime(record.startedAt),
  });
}
function commandText(record: R): string {
  if (record.status === 'error') return (record.error ?? '').split('\n')[0] ?? '';
  return displayValue(record.command);
}
function highlightParts(text: string) {
  return splitMatches(text, filterNeedle.value);
}
function alignClass(column: OpLogColumn): string {
  return column.align === 'end' ? 'justify-self-end text-right tabular-nums' : '';
}
</script>

<template>
  <div ref="rootEl" class="h-full flex flex-col min-h-0 text-kira-md">
    <OpLogToolbar
      v-model:filter-text="filterTextModel"
      :status-filter="statusFilter"
      :running-count="runningCount"
      :clear-hint="clearHint"
      :facet="facet"
      :facet-values="selectedFacets"
      @update:status-filter="(v) => emit('update:statusFilter', v)"
      @update:facet-values="(v) => emit('update:facetValues', v)"
      @clear="emit('clear')"
    >
      <slot name="toolbar" />
    </OpLogToolbar>

    <div v-if="loading" class="flex flex-col gap-px p-2" data-testid="ops-loading">
      <Skeleton v-for="n in 4" :key="n" class="h-4.5" />
    </div>
    <div v-else-if="records.length === 0" class="flex-1 min-h-0">
      <EmptyState
        v-if="hasFilters"
        fill
        icon="search"
        :title="t('workbench.opLog.noMatch.empty')"
        :description="t('workbench.opLog.noMatch.hint')"
        data-testid="ops-no-match"
      >
        <Button
          variant="dialog"
          size="kira-lg"
          data-testid="ops-clear-filters"
          @click="emit('resetFilters')"
        >
          <span>{{ t('workbench.opLog.clearFilters.label') }}</span>
        </Button>
      </EmptyState>
      <EmptyState
        v-else
        fill
        icon="checklist"
        :title="t('workbench.opLog.none.empty')"
        :description="emptyHint"
        data-testid="ops-empty"
      />
    </div>
    <template v-else>
      <div
        :class="
          cn(tableHeadRowClass, tableHeadClass, 'grid items-center gap-2 px-2 shrink-0 h-4.5')
        "
        :style="{ gridTemplateColumns }"
      >
        <span v-for="column in fitted" :key="column.id" :class="alignClass(column)">{{
          column.label
        }}</span>
      </div>
      <div
        ref="scrollEl"
        class="flex-1 min-h-0 overflow-auto"
        data-testid="virtual-list"
        @scroll="onScroll"
      >
        <ul
          :aria-label="t('common.module.operations.label')"
          class="relative m-0 p-0"
          :style="{ height: `${totalSize}px` }"
        >
          <template v-for="entry in rendered" :key="String(entry.vi.key)">
            <!-- biome-ignore lint/a11y/useKeyWithClickEvents: Enter and Space come from useListNavigation on the list container. -->
            <li
              v-if="entry.item.kind === 'op'"
              :style="{
                transform: `translateY(${entry.vi.start}px)`,
                height: `${OP_ROW_PX}px`,
                gridTemplateColumns,
              }"
              :class="
                cn(
                  VIRTUAL_ROW_CLASS,
                  tableRowClass,
                  'group/row grid items-center gap-2 px-2 select-text h-4.5',
                  entry.item.row.record.status === 'error' &&
                    'bg-error/15 shadow-[inset_2px_0_0_var(--color-error)]',
                )
              "
              data-testid="op-row"
              :data-status="entry.item.row.record.status"
              :data-op-id="entry.item.row.record.id"
              :data-depth="entry.item.row.depth"
              :aria-level="entry.item.row.depth + 1"
              :aria-expanded="expandedIds.has(entry.item.row.record.id)"
              :data-state="isSelected(entry.item.row.record.id) ? 'selected' : undefined"
              v-bind="nav.rowProps(entry.item.row.record.id)"
              @click="toggleExpanded(entry.item.row.record)"
              @contextmenu.prevent="onRowContextMenu(entry.item.row.record, $event)"
            >
              <template v-for="column in fitted" :key="column.id">
                <span v-if="column.id === 'status'" class="flex items-center gap-1 min-w-0">
                  <Tooltip>
                    <TooltipTrigger as-child>
                      <span
                        role="img"
                        :aria-label="statusLabel(entry.item.row.record.status)"
                        data-testid="op-status-icon"
                        class="inline-flex shrink-0"
                      >
                        <Spinner v-if="entry.item.row.record.status === 'running'" label="" />
                        <CodiconIcon
                          v-else
                          :name="STATUS_ICON[entry.item.row.record.status].icon"
                          :size="13"
                          :class="STATUS_ICON[entry.item.row.record.status].tone"
                        />
                      </span>
                    </TooltipTrigger>
                    <TooltipContent>{{ statusLabel(entry.item.row.record.status) }}</TooltipContent>
                  </Tooltip>
                  <Badge
                    v-if="statusNote?.(entry.item.row.record)"
                    class="min-w-0 truncate px-0.5"
                    data-testid="op-status-note"
                    >{{
                      statusNote(entry.item.row.record)
                    }}</Badge
                  >
                </span>
                <Tooltip v-else-if="column.id === 'time'">
                  <TooltipTrigger as-child>
                    <span
                      class="font-data tabular-nums whitespace-nowrap"
                      data-testid="op-time-cell"
                      >{{
                        formatOpTime(entry.item.row.record.startedAt, now)
                      }}</span
                    >
                  </TooltipTrigger>
                  <TooltipContent>{{ timeTitle(entry.item.row.record) }}</TooltipContent>
                </Tooltip>
                <span
                  v-else-if="column.id === 'kind'"
                  class="truncate min-w-0 font-data text-muted-foreground"
                  >{{
                    kindLabel?.(entry.item.row.record) ?? entry.item.row.record.kind
                  }}</span
                >
                <span
                  v-else-if="column.id === 'duration' && entry.item.row.record.status === 'running'"
                  :class="cn('font-data', alignClass(column))"
                  data-testid="op-elapsed"
                  >{{
                    elapsedText(entry.item.row.record)
                  }}</span
                >
                <span
                  v-else-if="column.id === 'duration'"
                  :class="cn('font-data', alignClass(column))"
                  >{{
                    durationText(entry.item.row.record.durationMs)
                  }}</span
                >
                <span
                  v-else-if="column.id === 'command'"
                  class="flex items-center gap-1 min-w-0"
                  :style="
                    entry.item.row.depth > 0
                      ? { paddingLeft: `${(entry.item.row.depth - 1) * 12}px` }
                      : undefined
                  "
                >
                  <CodiconIcon
                    v-if="entry.item.row.depth > 0"
                    name="indent"
                    :size="12"
                    class="shrink-0 text-muted-foreground"
                  />
                  <span
                    v-if="runningNoteFor(entry.item.row.record)"
                    class="block min-w-0 truncate text-muted-foreground"
                    data-testid="op-running-note"
                    >{{
                      runningNoteFor(entry.item.row.record)
                    }}</span
                  >
                  <span
                    v-else
                    :class="
                      cn(
                        'block min-w-0 flex-1 font-data',
                        entry.item.row.record.status === 'error' && 'text-error',
                      )
                    "
                    ><TruncatedText :text="commandText(entry.item.row.record)"
                      ><HighlightedText
                        :parts="highlightParts(commandText(entry.item.row.record))"
                      /></TruncatedText
                    ></span
                  >
                </span>
                <slot
                  v-else
                  name="cell"
                  :column="column"
                  :record="entry.item.row.record"
                  :depth="entry.item.row.depth"
                  :via-parent="entry.item.row.viaParent"
                />
              </template>
              <span
                class="absolute right-1 top-0 flex h-full items-center gap-0.5 group-hover/row:bg-elevated group-focus-within/row:bg-elevated"
              >
                <Button
                  v-if="
                    entry.item.row.record.status === 'running' &&
                    canCancelRecord(entry.item.row.record)
                  "
                  variant="toolbar"
                  size="inline"
                  :aria-label="t('workbench.opLog.cancel.aria')"
                  data-testid="op-cancel"
                  @click.stop="emit('cancel', entry.item.row.record)"
                >
                  <CodiconIcon name="debug-stop" :size="13" />
                </Button>
                <TooltipIconButton
                  v-if="entry.item.row.record.command"
                  icon="copy"
                  :label="t('common.copyCommand.label')"
                  size="inline"
                  :icon-size="13"
                  tabindex="-1"
                  class="opacity-0 group-hover/row:opacity-100 group-focus-within/row:opacity-100"
                  data-testid="op-copy"
                  @click.stop="
                    copy(entry.item.row.record.command ?? '', {
                      message: t('workbench.opLog.copiedCommand.announce'),
                    })
                  "
                />
              </span>
            </li>
            <li
              v-else
              :ref="measureRef"
              :data-index="entry.vi.index"
              :class="VIRTUAL_ROW_CLASS"
              :style="{ transform: `translateY(${entry.vi.start}px)` }"
            >
              <OpLogDetail
                :record="entry.item.record"
                :fields="fieldsFor?.(entry.item.record) ?? []"
                :steps="stepsFor?.(entry.item.record) ?? []"
                :lines="detailLines?.(entry.item.record) ?? []"
                :command-language="codeLanguage?.(entry.item.record) ?? null"
                :actions="menuForRecord(entry.item.record)"
                @escape="nav.focusKey(entry.item.record.id)"
              />
            </li>
          </template>
        </ul>
      </div>
    </template>
  </div>
</template>
