<script setup lang="ts">
import type { ColumnDescriptor } from '@shared/protocol/page';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { useVirtualRows, VIRTUAL_ROW_CLASS } from '@workbench/behaviour/virtualRows';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, ref, watch } from 'vue';
import { type SelectedCell, useCellSelectionStore } from '../../state/cellSelection';
import { INLINE_CHARS, parseInline } from '../../workers/parse/client';
import { useParseWorker } from '../../workers/parse/useParseWorker';
import DocumentRow from '../shared/document/DocumentRow.vue';
import DocumentTree from '../shared/document/DocumentTree.vue';
import { type DocumentRowView, useDocumentRowsStore } from '../shared/document/rows';
import { datasetNumber } from '../shared/eventCoords';
import { createMatchIndex } from '../shared/page/search';
import { setVisibleRows } from '../shared/page/visibleRows';
import ConsoleSlickGrid from './ConsoleSlickGrid.vue';
import type { CopyAllFormat } from './copyAll';
import { mongoDocumentRowMenu, rowAsJsonMenu } from './menu';
import { documentRow, getPage, keyValueRow, pageVersion, setVisibleWindow } from './resultPages';
import type { ResultViewState } from './resultViewState';
import { type Match, matchedRows, searchState } from './search';
import { useConsoleViewStore } from './state';
import { useResultScrollState } from './useResultScrollState';

const cellSelectionStore = useCellSelectionStore();
const contextMenuStore = useContextMenuStore();
const parse = useParseWorker();
const documentRowsStore = useDocumentRowsStore();
const consoleViewStore = useConsoleViewStore();

// Three-way switch over a console result's kind. Tabular results render through ConsoleSlickGrid.vue
// (same layer as views/grid/SlickGridHost.vue). Document and key-value results stay on VirtualList:
// their rows are Vue component trees or two flex cells, with no column axis to virtualize. The page
// cache is `./resultPages.ts`, a generic-`Page` sibling of `grid/page.ts`: a result can be tabular
// (SQL), document (Mongo shell) or key-value (Redis).
//
// tabId/connectionId/path only publish into cellSelection.ts's shared slot. A console result has no
// row/table to write back to, so published cells leave `onEdit` unset and the cell editor stays read-only.
const props = defineProps<{
  pageKey: string;
  tabId: string;
  connectionId: string | null;
  path: string;
  /** Opt-in view-state persistence (the proof window): restored on mount, reported on every
   *  scroll, click and expansion. `undefined` for the console, which keeps none. */
  viewState?: ResultViewState | null;
}>();
const emit = defineEmits<{
  'update:viewState': [state: ResultViewState];
  ready: [];
}>();

const { single: rowHeight } = useRowHeight();

const page = computed(() => {
  // Establishes the reactive dependency — the page object itself is frozen and non-reactive.
  void pageVersion.n;
  return getPage(props.pageKey);
});

// P30 §3.6 C2 — ConsoleSlickGrid.vue's own defineExpose contract (§3.5), a separate ref from
// the document/key-value virtualizers below since it isn't virtualized and exposes no
// `scrollToIndex`.
const tabularGridRef = ref<{ goToMatch: (match: Match) => void } | null>(null);

// P40 D10/D17: the same "hide non-matching rows" toggle grid/documents/keyvalue share (P24 D2) —
// a filtered row keeps its real page-row number in the gutter, same as those views.
const displayRows = computed<number[] | null>(() => matchedRows(props.tabId));
const rowIndices = computed(() => {
  void pageVersion.n;
  if (displayRows.value) return displayRows.value;
  return Array.from({ length: page.value?.rowCount ?? 0 }, (_, i) => i);
});

// The same head-row/DocumentTree pair the Mongo data tab renders (rowView/rowHeight —
// views/shared/document/rows.ts, registered onto this result's own key in console/state.ts's
// run()) — collapsed by default, per-result expansion state in the console runtime rather than
// persisted tab state (a console result is runtime-only to begin with).
const documentRows = computed<DocumentRowView[]>(() => {
  void pageVersion.n;
  const out: DocumentRowView[] = [];
  for (const i of rowIndices.value) {
    const view = documentRowsStore.rowView(props.pageKey, i);
    if (view) out.push(view);
  }
  return out;
});

const documentRowHeights = computed<number[]>(() => {
  void pageVersion.n;
  void documentRowsStore.rowsVersion.n;
  return documentRows.value.map((view) =>
    documentRowsStore.rowHeight(
      props.pageKey,
      view.index,
      null,
      consoleViewStore.isResultDocExpanded(props.tabId, props.pageKey, view.id),
    ),
  );
});

function onToggleDocExpanded(id: string): void {
  consoleViewStore.toggleResultDocExpanded(props.tabId, props.pageKey, id);
  emitViewState();
}

// VirtualList's own recipe, rebuilt on @tanstack/vue-virtual via the shared
// useVirtualRows composable — one instance per branch (document/key-value), since only one of
// the two is ever mounted at a time (the template's v-else-if/v-else) but each needs its own
// scroll element, count and row-height source.
const docScrollEl = ref<HTMLElement | null>(null);
const docVirtual = useVirtualRows({
  count: () => documentRows.value.length,
  rowHeight: () => rowHeight.value,
  rowHeights: () => documentRowHeights.value,
  scrollElement: docScrollEl,
});

const kvScrollEl = ref<HTMLElement | null>(null);
const kvVirtual = useVirtualRows({
  count: () => rowIndices.value.length,
  rowHeight: () => rowHeight.value,
  scrollElement: kvScrollEl,
});

const selectedKey = computed(() => (selected.value ? String(selected.value.row) : null));
const docNav = useListNavigation<DocumentRowView>({
  container: docScrollEl,
  items: () => documentRows.value,
  key: (view) => String(view.index),
  activeKey: computed({
    get: () => selectedKey.value,
    set: (key) => {
      if (key !== null) selectDocumentRow(Number(key));
    },
  }),
  onOpen: (view) => onToggleDocExpanded(view.id),
  onSelect: (view) => selectDocumentRow(view.index),
  onMenu: (view, rect) =>
    contextMenuStore.openContextMenuAt(rect.left, rect.bottom, documentRowMenu(view.index), {
      onRestoreFocus: () => void docNav.focusKey(String(view.index)),
    }),
  scrollToIndex: (index) => docVirtual.scrollToIndex(index),
  rendered: () => docVirtual.virtualItems.value,
});
const kvNav = useListNavigation<number>({
  container: kvScrollEl,
  items: () => rowIndices.value,
  key: (row) => String(row),
  activeKey: computed({
    get: () => selectedKey.value,
    set: (key) => {
      if (key !== null) selectKeyValueRow(Number(key));
    },
  }),
  onSelect: selectKeyValueRow,
  onMenu: (row, rect) =>
    contextMenuStore.openContextMenuAt(rect.left, rect.bottom, keyValueRowMenu(row), {
      onRestoreFocus: () => void kvNav.focusKey(String(row)),
    }),
  scrollToIndex: (index) => kvVirtual.scrollToIndex(index),
  rendered: () => kvVirtual.virtualItems.value,
});

// Search state is keyed by tabId, like every watcher below. The virtualizers report positions within
// `rowIndices`/`documentRows`: ascending, but non-contiguous while filtering (as DocumentView.vue's
// onVisibleRange). The same bounds prune resultPages.ts's decode cache, keyed by pageKey (a decode
// cache is per result set; search priority is per tab), so resultPages.ts needs no watch of its own.
watch(kvVirtual.virtualItems, (items) => {
  if (items.length === 0) return;
  const list = rowIndices.value;
  const from = list[items[0].index];
  const to = list[items[items.length - 1].index];
  if (from === undefined || to === undefined) return;
  setVisibleRows(props.tabId, from, to + 1);
  setVisibleWindow(props.pageKey, from, to + 1);
});

watch(docVirtual.virtualItems, (items) => {
  if (items.length === 0) return;
  const list = documentRows.value;
  const from = list[items[0].index]?.index;
  const to = list[items[items.length - 1].index]?.index;
  if (from === undefined || to === undefined) return;
  setVisibleRows(props.tabId, from, to + 1);
  setVisibleWindow(props.pageKey, from, to + 1);
  // rows.ts's own parseCache is pruned to the rendered window everywhere else (DocumentView.vue's
  // own onVisibleRange) but had no call site here at all — a Mongo console result's parsed node
  // trees stayed resident for the life of the result instead of the rendered window
  // docs/ARCHITECTURE.md's Caching section already documents for this tier.
  documentRowsStore.pruneRows(props.pageKey, from, to + 1);
});

// DocumentView.vue's own expand-all/collapse-all pair,
// exposed the same way goToMatch already is (ConsoleView.vue calls through this ref) — its
// toolbar lives one level up since these two buttons only make sense while the active result is
// document-shaped, a fact ConsoleView.vue's own getPage(activeKey) check decides, not this panel.
function expandAll(): void {
  consoleViewStore.setAllResultDocsExpanded(
    props.tabId,
    props.pageKey,
    documentRows.value.map((v) => v.id),
    true,
  );
  emitViewState();
}
function collapseAll(): void {
  consoleViewStore.setAllResultDocsExpanded(props.tabId, props.pageKey, [], false);
  emitViewState();
}

function kvRowAt(row: number) {
  return keyValueRow(props.pageKey, row) ?? { field: '', value: '' };
}

// Local to this one result panel (highlight only) — the actual cross-view selection lives in
// cellSelectionState.current (cellSelection.ts), which this publishes into. Two different
// result panels can each show their own last-clicked cell highlighted at once; only whichever
// published most recently is what the cell editor panel displays.
const selected = ref<{ row: number; col: number } | null>(null);

// Restoring a saved state: expansion goes into the console store before the first row renders; the
// highlighted row is a local ref, so it is set here too. Scroll is applied once mounted (below).
const saved = props.viewState;
if (saved) {
  for (const id of saved.expanded) {
    if (!consoleViewStore.isResultDocExpanded(props.tabId, props.pageKey, id)) {
      consoleViewStore.toggleResultDocExpanded(props.tabId, props.pageKey, id);
    }
  }
  if (saved.selectedRow !== null) selected.value = { row: saved.selectedRow, col: 0 };
}
const { report: emitViewState } = useResultScrollState({
  viewState: () => props.viewState,
  page: () => page.value,
  scrollEl: () => (page.value?.kind === 'document' ? docScrollEl.value : kvScrollEl.value),
  selectedRow: () => selected.value?.row ?? null,
  expandedIds: () =>
    documentRows.value
      .filter((v) => consoleViewStore.isResultDocExpanded(props.tabId, props.pageKey, v.id))
      .map((v) => v.id),
  onState: (state) => emit('update:viewState', state),
  onReady: () => emit('ready'),
});

function onDocScroll(): void {
  docVirtual.onScroll();
  emitViewState();
}

function onKeyValueScroll(): void {
  kvVirtual.onScroll();
  emitViewState();
}

function isSelected(row: number, col: number): boolean {
  return selected.value?.row === row && selected.value?.col === col;
}

// Unlike DataGrid.vue's rt.selection, this panel's "selection" is only ever
// the last click, held in the local `selected` ref above — a row index into a page that has been
// replaced (a new result chip swapping `pageKey` on this same mounted instance, or the same result
// reloading) identifies nothing. Clearing rather than republishing is
// the honest operation: there is no persistent selection concept here to republish against a new
// page in the first place.
// Page identity, not the counter every console tab's load/close bumps.
watch([() => props.pageKey, page], () => {
  selected.value = null;
  cellSelectionStore.clearSelectedCellFor(props.tabId);
});

// P40 D10: rebuilt only when the search result changes (a completed scan or prev/next), not per
// cell — mirrors KeyValueView.vue's own matchIndex.
const matchIndex = createMatchIndex(searchState, () => props.tabId);
function isSearchMatch(row: number, col: number): boolean {
  return matchIndex.value?.has(row, col) ?? false;
}
function isCurrentSearchMatch(row: number, col: number): boolean {
  return matchIndex.value?.isCurrent(row, col) ?? false;
}

// The find toolbar's go-to-match (P40 D10) — rowIndices is the *filtered* array when the filter
// toggle is on, so a match's page-row number has to be looked up by position rather than assumed
// to equal it, same as DocumentView.vue's own onGoToMatch.
function goToMatch(match: Match): void {
  // P30 §3.6 C2/C5 — the tabular branch delegates to ConsoleSlickGrid.vue's own contract; it
  // addresses a row by display *position*, not by an index into this file's own `rowIndices`.
  if (page.value?.kind === 'tabular') {
    tabularGridRef.value?.goToMatch(match);
    return;
  }
  const index = rowIndices.value.indexOf(match.row);
  if (index < 0) return;
  if (page.value?.kind === 'document') docVirtual.scrollToIndex(index);
  else kvVirtual.scrollToIndex(index);
}
defineExpose({ goToMatch, expandAll, collapseAll });

function publish(selectedCell: Omit<SelectedCell, 'tabId' | 'connectionId' | 'path'>): void {
  cellSelectionStore.publishSelectedCell({
    tabId: props.tabId,
    connectionId: props.connectionId,
    path: props.path,
    ...selectedCell,
  });
}

// A document row no longer publishes into the shared cellSelection slot — the expanded
// DocumentTree right below it already shows exactly this same body, so the cell editor dock used to
// pop up and show it a second time for no reason the real Mongo data tab's own DocumentView.vue
// doesn't (it has no cell editor at all). `selected` still tracks the last-clicked row for its own
// highlight, the same convention DocumentView.vue's rows use independent of any editor.
function selectDocumentRow(row: number): void {
  selected.value = { row, col: 0 };
  emitViewState();
}

function selectKeyValueRow(row: number): void {
  selected.value = { row, col: 0 };
  emitViewState();
  const kv = kvRowAt(row);
  const column: ColumnDescriptor = {
    name: kv.field || 'value',
    dataType: 'text',
    typeClass: 'text',
    nullable: false,
    isPrimaryKey: false,
    generated: false,
  };
  publish({
    columnIndex: 0,
    column,
    row,
    value: kv.value,
    truncated: false,
    hasPrimaryKey: false,
  });
}

// Same closure-per-render problem DataGrid.vue found first — a template
// handler that calls out with the v-for's row/column index can never be cached by Vue's compiler
// (hasScopeRef), so every visible row got a fresh wrapper closure on every render, scroll
// included. This recovers the row index from a data-* attribute on the element the event
// fired on instead, so the template can bind a stable, module-scope function. The document-row
// branch below is left as-is: DocumentRow.vue's `toggle`/`select` emits carry no native Event
// (defineEmits<{ toggle: []; select: [] }>()), so there is no currentTarget to read a data-*
// attribute off without also changing that shared component's public contract (which
// DocumentView.vue also depends on, outside this fix's scope).
function selectKeyValueRowFromEvent(e: MouseEvent): void {
  const r = datasetNumber(e.currentTarget, 'row');
  if (r !== null) selectKeyValueRow(r);
}

// The list navigation claims Enter and the arrows; Space mirrors a single click.
function selectKeyValueRowFromKeydown(e: KeyboardEvent): void {
  if (e.key !== ' ') return;
  e.preventDefault();
  const r = datasetNumber(e.currentTarget, 'row');
  if (r !== null) selectKeyValueRow(r);
}

// The document and key-value branches' own row menu — the document branch
// gets the Shell/Canonical/Relaxed submenu pair (mongoDocumentRowMenu); key-value has no
// shell/EJSON format of its own and keeps the plain "Copy as JSON"/"Copy all as JSON" pair
// (rowAsJsonMenu). Both cover every row currently displayed (i.e. rowIndices/documentRows — under
// an active find-filter that's the filtered subset, same rule columnsToTsv's own callers already
// follow). A copy failure lands here (component-local) since the
// console has no actionError field the way documents/menu.ts's copyOrReportError writes into.
const copyError = ref<string | null>(null);
function onCopyError(message: string): void {
  copyError.value = message;
}

// allJson used to be a materialized array, built eagerly at
// right-click time -- a whole-page decode (plus, for the key-value branch, a full
// JSON.stringify(..., null, 2) per row) on a click that should cost ~0 ms, and one that re-
// populates the decode/view caches for every row, undoing the visible-window pruning until the
// next scroll. Both are thunks now, invoked only inside "Copy all as JSON"'s own run().
function documentRowMenu(index: number) {
  const body = documentRow(props.pageKey, index)?.body ?? '';
  const allBodies = () =>
    documentRows.value.map((v) => documentRow(props.pageKey, v.index)?.body ?? '');
  // Over the inline threshold the encode runs in the shared parse worker: a 10 000-document page
  // otherwise blocks the main thread for hundreds of ms.
  const allText = (format: CopyAllFormat): Promise<string> => {
    const bodies = allBodies();
    const chars = bodies.reduce((n, b) => n + b.length, 0);
    return chars <= INLINE_CHARS
      ? Promise.resolve(parseInline('ejson.copyAll', { format, bodies }))
      : parse.run('ejson.copyAll', { format, bodies });
  };
  return mongoDocumentRowMenu({ body, allText, onError: onCopyError });
}

function onDocumentRowContextMenu(e: MouseEvent, index: number): void {
  e.preventDefault();
  contextMenuStore.openContextMenu(e, documentRowMenu(index), {
    onRestoreFocus: () => void docNav.focusKey(String(index)),
  });
}

function keyValueRowMenu(row: number) {
  const kv = kvRowAt(row);
  const json = JSON.stringify({ [kv.field]: kv.value }, null, 2);
  const allJson = () =>
    rowIndices.value.map((r) => {
      const entry = kvRowAt(r);
      return JSON.stringify({ [entry.field]: entry.value }, null, 2);
    });
  return rowAsJsonMenu({ json, allJson, onError: onCopyError });
}

function onKeyValueRowContextMenu(e: MouseEvent, row: number): void {
  e.preventDefault();
  contextMenuStore.openContextMenu(e, keyValueRowMenu(row), {
    onRestoreFocus: () => void kvNav.focusKey(String(row)),
  });
}

function onKeyValueRowContextMenuFromEvent(e: MouseEvent): void {
  const r = datasetNumber(e.currentTarget, 'row');
  if (r !== null) onKeyValueRowContextMenu(e, r);
}
</script>

<template>
  <div
    class="h-full min-h-0 flex flex-col text-kira-md font-data"
    data-testid="console-result-grid"
  >
    <Alert strip v-if="copyError" variant="error" data-testid="console-copy-error">
      <AlertDescription>{{ copyError }}</AlertDescription>
    </Alert>
    <div
      v-if="!page || page.rowCount === 0"
      class="no-rows absolute inset-0 h-full flex items-center justify-center text-muted-foreground text-kira-sm"
      >{{
        page ? t('studio.common.noRows.empty') : ''
      }}</div
    >
    <!-- P31 D19/P24 D8 precedent: filtering to zero matches is a distinct empty state from "no
         data loaded" — same discipline as KeyValueView.vue's own EmptyState pair. -->
    <div
      v-else-if="rowIndices.length === 0"
      class="no-rows absolute inset-0 h-full flex items-center justify-center text-muted-foreground text-kira-sm"
      data-testid="console-no-matching-rows"
    >
      {{ t('studio.common.noMatchingRows.empty') }}
    </div>
    <ConsoleSlickGrid
      v-else-if="page.kind === 'tabular'"
      :key="pageKey"
      ref="tabularGridRef"
      :page-key="pageKey"
      :tab-id="tabId"
      :connection-id="connectionId"
      :path="path"
      class="flex-1 min-h-0"
      :view-state="viewState"
      @copy-error="onCopyError"
      @update:view-state="emit('update:viewState', $event)"
      @ready="emit('ready')"
    />
    <div
      v-else-if="page.kind === 'document'"
      ref="docScrollEl"
      class="flex-1 min-h-0 p-1 overflow-auto"
      data-testid="virtual-list"
      role="listbox"
      :aria-label="t('studio.common.documents.aria')"
      @scroll="onDocScroll"
    >
      <div class="relative" :style="{ height: `${docVirtual.totalSize.value}px` }">
        <!-- P48 F10-F12: this read-only copy of DocumentRow (no edit/delete affordance) wants no
             pointer cursor outside its head; DocumentRow.vue itself doesn't opt into that. -->
        <DocumentRow
          v-for="vi in docVirtual.virtualItems.value"
          :key="String(vi.key)"
          :class="[VIRTUAL_ROW_CLASS, 'cursor-default']"
          data-testid="console-result-doc-row"
          :data-row="documentRows[vi.index]?.index"
          :view="documentRows[vi.index]!"
          :row-scope="pageKey"
          :nav-props="docNav.rowProps(String(documentRows[vi.index]!.index))"
          :expanded="
            consoleViewStore.isResultDocExpanded(tabId, pageKey, documentRows[vi.index]!.id)
          "
          :selected="isSelected(documentRows[vi.index]!.index, 0)"
          :search-match="isSearchMatch(documentRows[vi.index]!.index, 0)"
          :search-match-current="isCurrentSearchMatch(documentRows[vi.index]!.index, 0)"
          :style="{ transform: `translateY(${vi.start}px)` }"
          @toggle="onToggleDocExpanded(documentRows[vi.index]!.id)"
          @select="selectDocumentRow(documentRows[vi.index]!.index)"
          @contextmenu="onDocumentRowContextMenu($event, documentRows[vi.index]!.index)"
        >
          <template #body>
            <div
              v-if="
                consoleViewStore.isResultDocExpanded(tabId, pageKey, documentRows[vi.index]!.id)
              "
              class="flex-1 min-h-0 bg-elevated overflow-hidden"
              data-testid="document-body"
            >
              <DocumentTree
                v-if="documentRows[vi.index]!.root"
                :tab-id="pageKey"
                :row="documentRows[vi.index]!.index"
                @toggle-path="
                  (path) =>
                    documentRowsStore.togglePath(pageKey, documentRows[vi.index]!.index, path)
                "
              />
              <pre
                v-else
                class="m-0 whitespace-pre-wrap break-words font-data py-1 px-2"
              >{{ documentRow(pageKey, documentRows[vi.index]!.index)?.body }}</pre>
            </div>
          </template>
        </DocumentRow>
      </div>
    </div>
    <div
      v-else
      ref="kvScrollEl"
      class="flex-1 min-h-0 overflow-auto"
      data-testid="virtual-list"
      role="listbox"
      :aria-label="t('studio.console.result.rows.aria')"
      @scroll="onKeyValueScroll"
    >
      <div class="relative" :style="{ height: `${kvVirtual.totalSize.value}px` }">
        <div
          v-for="vi in kvVirtual.virtualItems.value"
          :key="String(vi.key)"
          class="flex w-(--total-width)"
          data-testid="console-result-kv-row"
          :data-row="rowIndices[vi.index]"
          :class="VIRTUAL_ROW_CLASS"
          :style="{ height: `${vi.size}px`, transform: `translateY(${vi.start}px)` }"
          role="option"
          v-bind="kvNav.rowProps(String(rowIndices[vi.index]))"
          :tabindex="kvNav.tabindexFor(String(rowIndices[vi.index]))"
          :aria-selected="isSelected(rowIndices[vi.index]!, 0)"
          @click="selectKeyValueRowFromEvent"
          @keydown="selectKeyValueRowFromKeydown"
          @contextmenu="onKeyValueRowContextMenuFromEvent"
        >
          <div
            class="cell"
            :class="
              cn(
                rowVariants({
                  layout: 'tree',
                  selected: isSelected(rowIndices[vi.index]!, 0),
                  muted: true,
                }),
                'w-52 select-text overflow-hidden text-ellipsis px-2',
                {
                  'search-match bg-search-match': isSearchMatch(rowIndices[vi.index]!, 0),
                  'search-match-current bg-search-match-current': isCurrentSearchMatch(
                    rowIndices[vi.index]!,
                    0,
                  ),
                },
                isCurrentSearchMatch(rowIndices[vi.index]!, 0) && 'text-bg',
              )
            "
          >
            {{ kvRowAt(rowIndices[vi.index]!).field }}
          </div>
          <div
            class="cell"
            :class="
              cn(
                rowVariants({ layout: 'tree', selected: isSelected(rowIndices[vi.index]!, 1) }),
                'flex-1 select-text overflow-hidden whitespace-pre-wrap break-words px-2',
                {
                  'search-match bg-search-match': isSearchMatch(rowIndices[vi.index]!, 1),
                  'search-match-current bg-search-match-current text-bg': isCurrentSearchMatch(
                    rowIndices[vi.index]!,
                    1,
                  ),
                },
              )
            "
          >
            {{ kvRowAt(rowIndices[vi.index]!).value }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
