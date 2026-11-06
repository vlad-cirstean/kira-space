<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { Input } from '@theme/components/ui/input';
import { useEventListener, useMutationObserver } from '@vueuse/core';
import { useContextMenuStore } from '@workbench/state/contextMenu';
import { type ListGrid, themes } from 'cheetah-grid';
import { markRaw, nextTick, onMounted, onScopeDispose, ref, shallowRef } from 'vue';
import { CGrid, CGridColumn } from 'vue-cheetah-grid';
import { setVisibleWindow } from '../../src/views/grid/page';
import { parseDelimited } from '../../src/views/shared/clipboardFormats';
import {
  columnHeaderTooltip,
  GUTTER_WIDTH,
  headerAwareMinWidth,
  initialWidthsByIndex,
  tooltipAttrs,
} from '../../src/views/shared/page/columns';
import { CellColumn, GutterColumn, type NavKind } from './cellColumn';
import { DataSource } from './cheetahTypes';
import { copySelection, selectionText } from './clipboard';
import { createProtoData, GUTTER_FIELD, readParams } from './data';
import { installDebugHook } from './debugHook';
import { createEditor, EDITOR_CLASS } from './edit';
import { HeaderColumn, SORT_ZONE } from './header';
import { cellMenu, columnMenu, headerMenu, type MenuActions, rangeMenu, rowMenu } from './menus';
import NavPreview, { type NavTarget } from './NavPreview.vue';
import { addInsertRow, deleteRows, stageValue, vetoReason } from './pending';
import { createRecordClass } from './records';
import { computeRowOrder } from './rowOrder';
import { markOf, runSearch, stepMatch } from './search';
import {
  covers,
  dragRows,
  EMPTY,
  rowSelected,
  type SelectionState,
  selectAll,
  selectCell,
  selectColumns,
  selectedRows,
  selectRows,
} from './selection';
import { cycleSort, setSort } from './sort';
import { createState, HEADER_ROWS, pageRowOf, recordOf } from './state';
import TooltipProxy from './TooltipProxy.vue';
import { buildTheme, readPalette } from './theme';
import { cheetahSource, createTrace } from './trace';

const NAV_HIT = 24;
const TRUNCATION_TIP = 'Value truncated at 64 KB';
const COUNTRIES: Record<string, string> = {
  US: 'United States',
  DE: 'Germany',
  RO: 'Romania',
  JP: 'Japan',
  BR: 'Brazil',
};

function referencedRow(kind: NavKind, column: string, value: string): string[] {
  if (kind === 'fk') return [`code: ${value}`, `name: ${COUNTRIES[value] ?? 'Unknown'}`];
  return [`customers.${column}: ${value}`, 'orders: 3 rows'];
}

const params = readParams(window.location.search);
const data = createProtoData(params.fixture);
const state = createState(data, params, readPalette(params.rowHeight ?? undefined));
const Record = createRecordClass(state);
const contextMenu = useContextMenuStore();

const gridRef = ref<{ rawGrid: unknown } | null>(null);
let raw: ListGrid<unknown> | null = null;

function cellBg({ col, row }: { col: number; row: number }): string {
  const p = state.palette;
  if (row < HEADER_ROWS || col === 0) return p.bgChrome;
  const record = row - HEADER_ROWS;
  if (covers(state.selection.sel, record, col - 1)) return p.select;
  if (record >= state.rowCount()) return p.bg;
  const pageRow = pageRowOf(state, record);
  if (state.search.cells.size > 0) {
    const mark = markOf(state, pageRow, state.order[col - 1] as number);
    if (mark === 'current') return p.searchCurrent;
    if (mark === 'match') return p.searchMatch;
  }
  if (pageRow === state.hoverRow) return p.hover;
  if (pageRow >= data.rowCount) return p.bgChrome;
  return state.params.zebra && record % 2 === 1 ? p.bgChrome : p.bg;
}

const themeDefine = markRaw(buildTheme(state.palette, cellBg));
function makeSource() {
  return markRaw(
    new DataSource({
      length: state.rowCount(),
      get: (index) => new Record(pageRowOf(state, index), index),
    }),
  );
}
const source = makeSource();

const fkNames = new Set(data.fk.keys());
const navFor = (index: number): NavKind | null => {
  const column = data.columns[index];
  if (!column) return null;
  if (fkNames.has(column.name)) return 'fk';
  return column.isPrimaryKey ? 'pk' : null;
};

const widths = initialWidthsByIndex(data.page);
const order = ref([...state.order]);
const gutterType = markRaw(new GutterColumn(state));
const columnTypes = data.columns.map((_, index) =>
  markRaw(new CellColumn(state, index, navFor(index))),
);
const headerTypes = data.columns.map((_, index) =>
  markRaw(new HeaderColumn(state, index, navFor(index))),
);
const editors = data.columns.map((_, index) => markRaw(createEditor(state, index)));

function headerStyle() {
  const p = state.palette;
  return { bgColor: p.bgChrome, color: p.fg, font: p.fontBold };
}

function minWidth(index: number): number {
  const column = data.columns[index];
  if (!column) return 0;
  return headerAwareMinWidth(column.name, {
    padding: 16,
    sortControl: 16,
    keyBadge: navFor(index) ? 20 : 0,
  });
}

function applyAppearance(): void {
  if (!raw) return;
  state.palette = readPalette(params.rowHeight ?? undefined);
  raw.theme = themes.of(buildTheme(state.palette, cellBg));
  raw.font = state.palette.font;
  raw.defaultRowHeight = state.palette.rowHeight;
  raw.headerRowHeight = state.palette.rowHeight;
  raw.underlayBackgroundColor = state.palette.bg;
  raw.updateSize();
  raw.invalidate();
}

useMutationObserver(document.documentElement, applyAppearance, {
  attributes: true,
  attributeFilter: ['style', 'class', 'data-proto-theme'],
});

function setSelection(next: SelectionState): void {
  state.selection = next;
  raw?.invalidate();
}

// Moving the native focus cell fires `selected_cell`, which would overwrite a selection this host
// just built (rows, columns, all). Programmatic moves run under this guard.
let programmatic = false;
function focusNative(record: number, displayCol: number): void {
  if (!raw) return;
  programmatic = true;
  try {
    raw.selection.select = { col: displayCol + 1, row: record + HEADER_ROWS };
  } finally {
    programmatic = false;
  }
  raw.focus();
}

function mirrorNative(): void {
  if (!raw || programmatic) return;
  const { start, end } = raw.selection.range;
  if (start.row < HEADER_ROWS) return;
  const select = raw.selection.select;
  const top = start.row - HEADER_ROWS;
  const bottom = end.row - HEADER_ROWS;
  const left = Math.max(0, start.col - 1);
  const right = Math.max(0, end.col - 1);
  const anchorRow = select.row - HEADER_ROWS;
  const anchorCol = Math.max(0, select.col - 1);
  if (top === bottom && left === right) {
    setSelection(selectCell(top, left));
    return;
  }
  setSelection({
    sel: { kind: 'range', anchorRow: top, anchorCol: left, row: bottom, col: right },
    anchorRow,
    anchorCol,
  });
}

function mods(e: MouseEvent): { shift: boolean; toggle: boolean } {
  return { shift: e.shiftKey, toggle: e.metaKey || e.ctrlKey };
}

let gutterDragFrom: number | null = null;
useEventListener(window, 'mouseup', () => {
  gutterDragFrom = null;
});

const term = ref('');
const matchCount = ref(0);
const hideNonMatching = ref(false);
const tip = ref<InstanceType<typeof TooltipProxy> | null>(null);
const nav = shallowRef<NavTarget | null>(null);

function applyOrder(): void {
  state.rowOrder = computeRowOrder(state, hideNonMatching.value);
  state.selection = EMPTY;
  state.refreshSource();
  raw?.invalidate();
}

function openEditor(record: number, displayCol: number): void {
  if (!raw) return;
  const pageCol = state.order[displayCol] as number;
  const reason = vetoReason(state, pageRowOf(state, record), pageCol);
  if (reason) {
    state.vetoReason = reason;
    return;
  }
  focusNative(record, displayCol);
  // The closing menu hands focus back to its trigger, which would blur and detach a new editor.
  requestAnimationFrame(() =>
    requestAnimationFrame(() =>
      editors[pageCol]?.onOpenCellInternal(raw as never, {
        col: displayCol + 1,
        row: record + HEADER_ROWS,
      }),
    ),
  );
}

function pasteBlock(text: string, record: number, displayCol: number): void {
  parseDelimited(text).forEach((cells, r) => {
    if (record + r >= state.rowCount()) return;
    cells.forEach((value, c) => {
      const pageCol = state.order[displayCol + c];
      if (pageCol !== undefined) stageValue(state, pageRowOf(state, record + r), pageCol, value);
    });
  });
}

const actions: MenuActions = {
  state,
  hideColumn(displayCol) {
    state.order = state.order.filter((_, i) => i !== displayCol);
    order.value = [...state.order];
    setSelection(EMPTY);
  },
  showAllColumns() {
    state.order = data.columns.map((_, i) => i);
    order.value = [...state.order];
    setSelection(EMPTY);
  },
  sort(displayCol, dir) {
    const pageCol = state.order[displayCol] as number;
    setSort(state, pageCol, dir, false);
    applyOrder();
  },
  edit: openEditor,
  setNull(record, displayCol) {
    stageValue(state, pageRowOf(state, record), state.order[displayCol] as number, null);
  },
  async paste(record, displayCol) {
    try {
      pasteBlock(await navigator.clipboard.readText(), record, displayCol);
    } catch {
      // Clipboard read refused: nothing to paste.
    }
  },
  deleteRows(records) {
    deleteRows(
      state,
      records.map((r) => pageRowOf(state, r)),
    );
    setSelection(EMPTY);
  },
  insertRow() {
    const record = addInsertRow(state);
    const name = data.columns[state.order[0] as number]?.name as string;
    raw?.makeVisibleGridCell(name as never, record);
    setSelection(selectCell(record, 0));
    focusNative(record, 0);
  },
};

function find(): void {
  runSearch(state, term.value);
  matchCount.value = state.search.list.length;
  if (hideNonMatching.value) applyOrder();
  else raw?.invalidate();
}

function goToMatch(step: 1 | -1): void {
  const target = stepMatch(state, step);
  raw?.invalidate();
  if (!raw || !target) return;
  const name = data.columns[state.order[target.displayCol] as number]?.name as string;
  raw.makeVisibleGridCell(name as never, target.record);
  setSelection(selectCell(target.record, target.displayCol));
  focusNative(target.record, target.displayCol);
}

function setHideNonMatching(value: boolean): void {
  hideNonMatching.value = value;
  applyOrder();
}

function openMenu(ev: MouseEvent, record: number, displayCol: number, header: boolean): void {
  ev.preventDefault();
  if (header) {
    const cols = state.selection.sel?.kind === 'column' ? state.selection.sel.cols : [];
    if (!cols.includes(displayCol)) setSelection(selectColumns(EMPTY, displayCol, mods(ev)));
    contextMenu.openContextMenu(ev, headerMenu(actions, displayCol));
    return;
  }
  if (displayCol < 0) {
    if (!rowSelected(state.selection.sel, record)) {
      setSelection(selectRows(state.selection, record, { shift: false, toggle: false }));
      focusNative(record, 0);
    }
    contextMenu.openContextMenu(
      ev,
      rowMenu(actions, selectedRows(state.selection.sel, state.rowCount())),
    );
    return;
  }
  const { sel } = state.selection;
  const inside = covers(sel, record, displayCol);
  if (inside && sel?.kind === 'row') {
    contextMenu.openContextMenu(ev, rowMenu(actions, sel.rows));
  } else if (inside && sel?.kind === 'column') {
    contextMenu.openContextMenu(ev, columnMenu(actions, sel.cols));
  } else if (inside && sel && sel.kind !== 'cell') {
    contextMenu.openContextMenu(ev, rangeMenu(actions));
  } else {
    setSelection(selectCell(record, displayCol));
    focusNative(record, displayCol);
    contextMenu.openContextMenu(ev, cellMenu(actions, record, displayCol));
  }
}

type HeaderZone = 'resize' | 'sort' | 'body';

// Cheetah starts a column resize only when `mousedown_cell` is not cancelled, so a press on a
// header border has to fall through to it. The library's own hit width is 5 px.
const RESIZE_EDGE = 5;

function headerZone(
  grid: ListGrid<unknown>,
  col: number,
  row: number,
  event: MouseEvent,
): HeaderZone {
  const origin = grid.getElement().getBoundingClientRect();
  const cell = grid.getCellRelativeRect(col, row);
  const x = event.clientX - origin.left - cell.left;
  if (x >= cell.width - RESIZE_EDGE || x <= RESIZE_EDGE) return 'resize';
  return x >= cell.width - SORT_ZONE && col > 0 ? 'sort' : 'body';
}

function setHoverHeader(grid: ListGrid<unknown>, pageCol: number, col: number): void {
  if (state.hoverHeader === pageCol) return;
  const previous = state.hoverHeader;
  state.hoverHeader = pageCol;
  if (previous >= 0) {
    const at = state.order.indexOf(previous);
    if (at >= 0) grid.invalidateCell(at + 1, 0);
  }
  if (col > 0) grid.invalidateCell(col, 0);
}

function bindGrid(grid: ListGrid<unknown>): void {
  grid.listen('mousedown_cell', (e) => {
    if (e.event.button !== 0) return false;
    const record = e.row - HEADER_ROWS;
    if (e.row < HEADER_ROWS) {
      const zone = headerZone(grid, e.col, e.row, e.event);
      if (zone === 'resize') return true;
      if (zone === 'sort') {
        cycleSort(state, state.order[e.col - 1] as number, e.event.shiftKey);
        applyOrder();
        return false;
      }
      setSelection(
        e.col === 0 ? selectAll() : selectColumns(state.selection, e.col - 1, mods(e.event)),
      );
      // The browser's own mousedown default parks focus on the body after this handler returns.
      requestAnimationFrame(() => grid.focus());
      return false;
    }
    if (e.col === 0) {
      setSelection(selectRows(state.selection, record, mods(e.event)));
      gutterDragFrom = record;
      focusNative(record, 0);
      return false;
    }
    return true;
  });
  grid.listen('mouseenter_cell', (e) => {
    showTip(grid, e.col, e.row);
    if (e.row < HEADER_ROWS) {
      setHoverHeader(grid, e.col > 0 ? (state.order[e.col - 1] as number) : -1, e.col);
      return;
    }
    setHoverHeader(grid, -1, 0);
    const record = e.row - HEADER_ROWS;
    if (gutterDragFrom !== null && e.col === 0) {
      setSelection(dragRows(state.selection, gutterDragFrom, record));
    }
    const pageRow = pageRowOf(state, record);
    if (pageRow === state.hoverRow) return;
    const previous = state.hoverRow;
    state.hoverRow = pageRow;
    const last = grid.colCount - 1;
    grid.invalidateGridRect(0, e.row, last, e.row);
    const prevRecord = previous < 0 ? -1 : recordOf(state, previous);
    if (prevRecord >= 0)
      grid.invalidateGridRect(0, prevRecord + HEADER_ROWS, last, prevRecord + HEADER_ROWS);
  });
  grid.listen('mouseleave_cell', (e) => {
    tip.value?.hide();
    if (e.row < HEADER_ROWS) setHoverHeader(grid, -1, 0);
  });
  grid.listen('click_cell', (e) => {
    if (e.row < HEADER_ROWS || e.col === 0) return;
    const pageCol = state.order[e.col - 1] as number;
    const kind = navFor(pageCol);
    if (!kind) return;
    const view = state.viewAt(pageRowOf(state, e.row - HEADER_ROWS), pageCol);
    if (view.isNull) return;
    const origin = grid.getElement().getBoundingClientRect();
    const cell = grid.getCellRelativeRect(e.col, e.row);
    if (e.event.clientX - origin.left - cell.left > NAV_HIT) return;
    const name = data.columns[pageCol]?.name as string;
    nav.value = {
      kind,
      rect: {
        left: origin.left + cell.left,
        top: origin.top + cell.top,
        width: cell.width,
        height: cell.height,
      },
      column: name,
      value: view.text,
      lines: referencedRow(kind, name, view.text),
    };
  });
  grid.listen('paste_cell', (e) => {
    if (!e.multi) return;
    e.event.preventDefault();
    const record = e.row - HEADER_ROWS;
    if (record >= 0 && e.col > 0) pasteBlock(e.normalizeValue, record, e.col - 1);
  });
  grid.listen('selected_cell', (e) => {
    if (e.selected) mirrorNative();
  });
  grid.listen('contextmenu_cell', (e) => {
    const record = e.row - HEADER_ROWS;
    openMenu(e.event, record, e.col - 1, e.row < HEADER_ROWS);
  });
  grid.listen('keydown', (e) => {
    const chord = e.event.metaKey || e.event.ctrlKey;
    if (chord && e.event.key.toLowerCase() === 'c') {
      e.event.preventDefault();
      void copySelection(state);
    } else if (chord && e.event.key.toLowerCase() === 'a') {
      e.event.preventDefault();
      setSelection(selectAll());
    }
  });
  grid.listen('copydata', () => selectionText(state));
  const scrollId = grid.listen('scroll', () => {
    const top = Math.max(0, grid.topRow - HEADER_ROWS);
    const bottom = Math.min(state.rowCount() - 1, top + grid.visibleRowCount);
    setVisibleWindow(data.scope, pageRowOf(state, top), pageRowOf(state, Math.max(top, bottom)));
  });
  onScopeDispose(() => grid.unlisten(scrollId));
}

const rootRef = ref<HTMLElement | null>(null);
useEventListener(rootRef, 'mouseleave', onLeave);
useEventListener(
  rootRef,
  'keydown',
  (e: KeyboardEvent) => {
    const input = e.target;
    if (e.key !== 'Escape' || !(input instanceof HTMLInputElement) || !state.editing) return;
    if (!input.classList.contains(EDITOR_CLASS)) return;
    input.value = state.editing.original;
    input.blur();
    raw?.focus();
  },
  { capture: true },
);
// An insert row's value is staged per keystroke, as Studio's live inputs do.
useEventListener(rootRef, 'input', (e: Event) => {
  const input = e.target;
  const editing = state.editing;
  if (!(input instanceof HTMLInputElement) || !editing) return;
  if (!input.classList.contains(EDITOR_CLASS) || editing.pageRow < data.rowCount) return;
  stageValue(state, editing.pageRow, editing.col, input.value);
});

function showTip(grid: ListGrid<unknown>, col: number, row: number): void {
  let attrs: Record<string, string> | null = null;
  const pageCol = state.order[col - 1] as number;
  const descriptor = data.columns[pageCol];
  if (col === 0) {
    attrs = null;
  } else if (row < HEADER_ROWS) {
    if (descriptor) attrs = tooltipAttrs(columnHeaderTooltip(descriptor, descriptor.dataType));
  } else if (state.viewAt(pageRowOf(state, row - HEADER_ROWS), pageCol).truncated) {
    attrs = tooltipAttrs({ title: TRUNCATION_TIP });
  }
  if (!attrs) {
    tip.value?.hide();
    return;
  }
  tip.value?.show({ rect: grid.getCellRelativeRect(col, row), attrs });
}

function onLeave(): void {
  if (state.hoverRow < 0 || !raw) return;
  const previous = state.hoverRow;
  state.hoverRow = -1;
  const record = recordOf(state, previous);
  if (record >= 0)
    raw.invalidateGridRect(0, record + HEADER_ROWS, raw.colCount - 1, record + HEADER_ROWS);
}

onMounted(async () => {
  await nextTick();
  raw = (gridRef.value?.rawGrid ?? null) as ListGrid<unknown> | null;
  if (!raw) return;
  state.invalidateRow = (pageRow) => {
    const record = recordOf(state, pageRow);
    if (record >= 0)
      raw?.invalidateGridRect(0, record + HEADER_ROWS, raw.colCount - 1, record + HEADER_ROWS);
  };
  state.refreshSource = () => {
    if (raw) raw.dataSource = makeSource() as never;
  };
  raw.allowRangePaste = false;
  bindGrid(raw);
  if (__KIRA_DEBUG_HOOKS__) {
    installDebugHook({ grid: raw, state, navOf: navFor });
    window.__kiraProtoTrace = createTrace(cheetahSource(raw));
  }
});
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <div
      v-if="params.fixture === 'features'"
      class="flex shrink-0 items-center justify-end gap-1 border-b border-border bg-bg-chrome px-3 py-1"
      data-testid="proto-toolbar"
    >
      <Input
        v-model="term"
        class="h-6 w-40"
        placeholder="Find"
        data-testid="find-input"
        @keydown.enter="find"
      />
      <Button size="kira" variant="toolbar" data-testid="find-run" @click="find">Find</Button>
      <Button size="kira" variant="toolbar" data-testid="find-prev" @click="goToMatch(-1)"
        >Prev</Button
      >
      <Button size="kira" variant="toolbar" data-testid="find-next" @click="goToMatch(1)"
        >Next</Button
      >
      <span class="text-fg-muted" data-testid="find-count">{{ matchCount }}</span>
      <Button
        size="kira"
        variant="toolbar"
        data-testid="find-hide"
        @click="setHideNonMatching(!hideNonMatching)"
      >
        {{ hideNonMatching ? 'Show all' : 'Hide others' }}
      </Button>
      <Button size="kira" variant="toolbar" data-testid="add-row" @click="actions.insertRow()"
        >Add row</Button
      >
    </div>
    <div
      ref="rootRef"
      class="relative min-h-0 flex-1 overflow-hidden"
      data-testid="data-grid"
      :style="{
        width: params.width ? `${params.width}px` : undefined,
        height: params.height ? `${params.height}px` : undefined,
        flex: params.height ? 'none' : undefined,
      }"
    >
      <TooltipProxy ref="tip" :container="rootRef" />
      <NavPreview :target="nav" @close="nav = null" />
      <CGrid
        ref="gridRef"
        :data="source"
        :theme="themeDefine"
        :font="state.palette.font"
        :underlay-background-color="state.palette.bg"
        :frozen-col-count="1"
        :default-row-height="state.palette.rowHeight"
        :header-row-height="state.palette.rowHeight"
        :move-cell-on-tab-key="true"
        :move-cell-on-enter-key="true"
      >
        <CGridColumn
          :field="GUTTER_FIELD"
          :width="GUTTER_WIDTH"
          :min-width="GUTTER_WIDTH"
          :max-width="GUTTER_WIDTH"
          :column-type="gutterType"
          :header-style="headerStyle()"
        >
          {{ '' }}
        </CGridColumn>
        <CGridColumn
          v-for="index in order"
          :key="data.columns[index]?.name"
          :field="data.columns[index]?.name"
          :width="widths[index]"
          :min-width="minWidth(index)"
          :column-type="columnTypes[index]"
          :header-type="headerTypes[index]"
          :action="editors[index]"
          :header-style="headerStyle()"
        >
          {{ data.columns[index]?.name }}
        </CGridColumn>
      </CGrid>
    </div>
  </div>
</template>
