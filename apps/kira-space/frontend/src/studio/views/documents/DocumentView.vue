<script setup lang="ts">
import { pathTail } from '@shared/domain/tree';
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import HighlightedText from '@theme/components/HighlightedText.vue';
import Spinner from '@theme/components/Spinner.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { colorMarkClass, connTextClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { useVirtualRows, VIRTUAL_ROW_CLASS } from '@workbench/behaviour/virtualRows';
import { errorText } from '@workbench/bridge/codedError';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { registerCommand } from '@workbench/shortcuts/commands';
import { useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue';
import MonacoHost from '../../editor/MonacoHost.vue';
import { useConnectionsStore } from '../../state/connections';
import { nodeKindLabel } from '../../state/nodeKindLabel';
import { useRunState } from '../../state/runState';
import type { DocumentTabRecord } from '../../state/tabDomain';
import EngineIcon from '../../theme/EngineIcon.vue';
import DocumentRow from '../shared/document/DocumentRow.vue';
import DocumentTree from '../shared/document/DocumentTree.vue';
import { type DocumentRowView, useDocumentRowsStore } from '../shared/document/rows';
import SearchFilterNotice from '../shared/page/SearchFilterNotice.vue';
import SearchToolbar from '../shared/page/SearchToolbar.vue';
import { setVisibleRows } from '../shared/page/visibleRows';
import StaleDataStrip from '../shared/StaleDataStrip.vue';
import { ancestorPathPrefix } from '../shared/targetPath';
import { refreshOrReconnect, useConnectionGate } from '../shared/useConnectionGate';
import DocumentEditor from './DocumentEditor.vue';
import DocumentFilterRow from './DocumentFilterRow.vue';
import DocumentToolbar from './DocumentToolbar.vue';
import { confirmDeleteDocument, rowMenu } from './menu';
import { deleteDocument } from './mutations';
import { documentRow, pageVersion, setVisibleWindow } from './page';
import RowActionButton from './RowActionButton.vue';
import { type Match, matchedRows, pageSearchApi } from './search';
import { useDocumentViewStore } from './state';
import { useDocumentEdit } from './useDocumentEdit';
import { useDocumentGate } from './useDocumentGate';
import { useDocumentSearchMatches } from './useDocumentSearchMatches';
import { useRowTip } from './useRowTip';

const contextMenuStore = useContextMenuStore();
const documentRowsStore = useDocumentRowsStore();
const connectionsStore = useConnectionsStore();
const documentViewStore = useDocumentViewStore();

// MainView.vue keys this component by tab.id — same discipline as DefinitionView.vue/ConsoleView.vue.
const props = defineProps<{ tab: DocumentTabRecord }>();

documentRowsStore.registerDocumentRows(props.tab.id, (row) => documentRow(props.tab.id, row));

const { needsReconnect, reconnecting, onReconnectAndLoad } = useConnectionGate(
  () => props.tab,
  () => documentViewStore.load(props.tab.id),
);

const rt = computed(() => documentViewStore.runtime[props.tab.id]);
const _running = computed(() => rt.value?.status === 'loading');

const {
  caps,
  connRecord,
  stale,
  hasPage,
  saveDisabledReason,
  canInsert,
  canDelete,
  insertTitle,
  deleteTitle,
  editGateFor,
} = useDocumentGate(() => props.tab);

const targetTail = computed(() => pathTail(props.tab.path));

const connectionColor = computed(
  () => connectionsStore.connectionRecord(props.tab.connectionId)?.color,
);

const iconClass = computed(() => connTextClass(connectionColor.value) ?? 'text-muted-foreground');

const railColor = computed(() => (connRecord.value ? (connRecord.value.color ?? null) : undefined));
const _runState = useRunState(() => props.tab.id);

// The view-head's breadcrumb prefix ("connection / database / "): derived from the already
// loaded connection record and the tab's own path, purely for display — no new state.
const pathPrefix = computed(() => ancestorPathPrefix(props.tab.connectionId, props.tab.path));

const {
  creatingNew,
  newBuffer,
  editingRow,
  editBuffer,
  saving,
  onAddDocument,
  cancelCreate,
  commitCreate,
  startEdit,
  cancelEdit,
  commitEdit,
} = useDocumentEdit(
  () => props.tab.id,
  (row) => editGateFor(row).editable,
  () => stale.value,
);
const { isSearchMatch, isCurrentSearchMatch, previewSegments } = useDocumentSearchMatches(
  () => props.tab.id,
);

function onToggleSearch(): void {
  documentViewStore.toggleSearchOpen(props.tab.id);
}

function onCloseSearch(): void {
  documentViewStore.setSearchOpen(props.tab.id, false);
}

interface DocumentRowEntry {
  view: DocumentRowView;
  body: string;
}

const displayRows = computed<number[] | null>(() => matchedRows(props.tab.id));

const rows = computed<number[]>(() => {
  void pageVersion.n;
  return displayRows.value ?? Array.from({ length: rt.value?.rowCount ?? 0 }, (_, i) => i);
});

/** Resolves one page row for rendering — id/body decode plus the body's own parsed tree, both
 *  memoized by page.ts/rows.ts's own caches (C3 prunes them to the visible window), so repeat
 *  calls for the same row within one render are cheap Map lookups, not a re-decode/re-parse. */
function rowAt(row: number): DocumentRowEntry | null {
  const doc = documentRow(props.tab.id, row);
  const view = documentRowsStore.rowView(props.tab.id, row);
  if (!doc || !view) return null;
  return { view, body: doc.body };
}

/** Expand all/Collapse all and the row context menu's own "every id on this page" (menu.ts's
 *  `allIds`) need every row's `_id` — a decode, not `rowAt`'s own parse — over whichever index
 *  list is current (the full page, or the filtered subset while a find's filter toggle is on,
 *  matching `rows`' own scope exactly). */
function idsOf(indices: readonly number[]): string[] {
  const out: string[] = [];
  for (const i of indices) {
    const doc = documentRow(props.tab.id, i);
    if (doc) out.push(doc.id);
  }
  return out;
}

const rowHeights = computed<number[]>(() => {
  void pageVersion.n;
  void documentRowsStore.rowsVersion.n;
  return rows.value.map((row) => {
    const id = documentRow(props.tab.id, row)?.id ?? null;
    const expanded = id !== null && documentViewStore.isDocumentExpanded(props.tab.id, id);
    return documentRowsStore.rowHeight(
      props.tab.id,
      row,
      editingRow.value,
      expanded,
      !expanded && isSearchMatch(row),
    );
  });
});

const { single: rowHeight } = useRowHeight();
const scrollEl = ref<HTMLElement | null>(null);

const OVERSCAN_PX = 320;
const overscanRows = computed(() => {
  const heights = rowHeights.value;
  if (heights.length === 0) return 8;
  let sum = 0;
  for (const h of heights) sum += h;
  return Math.min(8, Math.max(2, Math.ceil(OVERSCAN_PX / (sum / heights.length))));
});

const { virtualItems, totalSize, onScroll, scrollToIndex } = useVirtualRows({
  count: () => rows.value.length,
  rowHeight: () => rowHeight.value,
  rowHeights: () => rowHeights.value,
  scrollElement: scrollEl,
  overscan: () => overscanRows.value,
  pinned: () => rt.value?.selectedRow ?? null,
});

const nav = useListNavigation<number>({
  container: scrollEl,
  items: () => rows.value,
  key: (row) => String(row),
  activeKey: computed({
    get: () => {
      const position = rt.value?.selectedRow;
      const row = position === null || position === undefined ? undefined : rows.value[position];
      return row === undefined ? null : String(row);
    },
    set: (key) => {
      if (key !== null) onRowClick(rows.value.indexOf(Number(key)));
    },
  }),
  onOpen: (row) => {
    const entry = rowAt(row);
    if (entry) documentViewStore.toggleExpanded(props.tab.id, entry.view.id);
  },
  onSelect: (row) => onRowClick(rows.value.indexOf(row)),
  onMenu: (row, rect) => {
    const items = rowMenuFor(row);
    if (items) {
      contextMenuStore.openContextMenuAt(rect.left, rect.bottom, items, {
        onRestoreFocus: () => void nav.focusKey(String(row)),
      });
    }
  },
  scrollToIndex: (index) => scrollToIndex(index),
  rendered: () => virtualItems.value,
});

const { rowTip, rowTipStyle, showRowTip, hideRowTip, clearRowTip } = useRowTip();
function onListScroll(): void {
  clearRowTip();
  onScroll();
}

watch(virtualItems, (items) => {
  if (items.length === 0) return;
  const list = rows.value;
  const from = list[items[0].index];
  const to = list[items[items.length - 1].index];
  if (from === undefined || to === undefined) return;
  setVisibleRows(props.tab.id, from, to + 1);
  setVisibleWindow(props.tab.id, from, to + 1);
  documentRowsStore.pruneRows(props.tab.id, from, to + 1);
});

function onGoToMatch(match: Match): void {
  const row = match.row;
  const view = documentRowsStore.rowView(props.tab.id, row);
  if (!view) return;
  if (!documentViewStore.isDocumentExpanded(props.tab.id, view.id))
    documentViewStore.toggleExpanded(props.tab.id, view.id);
  void nextTick().then(() => {
    const index = rows.value.indexOf(row);
    if (index >= 0) scrollToIndex(index);
  });
}

function rowMenuFor(row: number) {
  const entry = rowAt(row);
  if (!entry) return null;
  return rowMenu(
    props.tab.id,
    entry.view.id,
    entry.body,
    () => idsOf(rows.value),
    () => startEdit(row, entry.view.id, entry.body),
    editGateFor(row),
    { deletable: canDelete.value, label: deleteTitle.value },
  );
}

function onRowContextMenu(e: MouseEvent, row: number): void {
  e.preventDefault();
  const items = rowMenuFor(row);
  if (items) nav.contextMenu(row, e, items);
}

async function onDeleteRow(id: string): Promise<void> {
  if (!(await confirmDeleteDocument(id))) return;
  if (stale.value) {
    documentViewStore.setActionError(props.tab.id, t('studio.common.connectionLost.error'));
    return;
  }
  deleteDocument(props.tab.id, id)
    .then(() => documentViewStore.setActionError(props.tab.id, null))
    .catch((err: unknown) => {
      documentViewStore.setActionError(props.tab.id, errorText(err));
    });
}

function _onStop(): void {
  documentViewStore.stop(props.tab.id);
}

function onRefresh(): void {
  refreshOrReconnect(needsReconnect.value, onReconnectAndLoad, () =>
    documentViewStore.reload(props.tab.id),
  );
}

function onExpandAll(): void {
  documentViewStore.setAllExpanded(props.tab.id, [], true);
}

function onCollapseAll(): void {
  documentViewStore.setAllExpanded(props.tab.id, idsOf(rows.value), false);
}

function onRowClick(i: number): void {
  documentViewStore.selectRow(props.tab.id, i);
}

let unregisterCommand: (() => void) | null = null;
let unregisterFindCommand: (() => void) | null = null;

onMounted(() => {
  if (!needsReconnect.value && !documentViewStore.runtime[props.tab.id]) {
    void documentViewStore.load(props.tab.id);
  }
  unregisterCommand = registerCommand('view.refresh', onRefresh);
  unregisterFindCommand = registerCommand('view.find', onToggleSearch);
});

onUnmounted(() => {
  documentRowsStore.unregisterDocumentRows(props.tab.id);
  unregisterCommand?.();
  unregisterFindCommand?.();
});
</script>

<template>
  <div class="h-full flex flex-col min-h-0" data-testid="document-view" :data-path="tab.path">
    <ViewToolbar>
      <span v-if="railColor !== undefined" :class="colorMarkClass('dot', railColor)" />
      <span v-if="connRecord?.kind" class="size-4 flex items-center justify-center shrink-0">
        <EngineIcon :kind="connRecord.kind" :size="13" />
      </span>
      <span class="size-4 flex items-center justify-center shrink-0" :class="iconClass">
        <CodiconIcon name="json" :size="13" />
      </span>
      <span class="text-kira-md text-fg truncate" data-testid="document-target"
        ><span v-if="pathPrefix" class="text-subtle">{{ pathPrefix }}</span>
        {{ targetTail?.name ?? tab.path }}</span
      >
      <Badge>{{ nodeKindLabel('collection') }}</Badge>
    </ViewToolbar>

    <div :class="colorMarkClass('band', railColor)" />
    <DocumentToolbar
      :tab="tab"
      :caps="caps"
      :can-insert="canInsert"
      :insert-title="insertTitle"
      @refresh="onRefresh"
      @expand-all="onExpandAll"
      @collapse-all="onCollapseAll"
      @add="onAddDocument"
    />
    <!-- The Mongo dialect of the filter row: one filter box, permanent, never closed — plus a
         SORT box beside it (read.ts's structured-sort-only rule, see ./sortDocument.ts's
         sortSpecToText/parseSortText), FilterToolbar.vue's ORDER BY box narrowed to the one form
         Mongo can execute and reworded to Mongo's own sort-document syntax rather than SQL's.
         History button and Clear button match FilterToolbar.vue's own layout exactly — this row
         used to have neither. -->
    <DocumentFilterRow :tab="tab" />

    <StaleDataStrip
      v-if="stale && hasPage"
      testid="document-stale"
      :reconnecting="reconnecting"
      @reconnect="onReconnectAndLoad"
    />
    <Alert
      strip
      v-if="rt?.status === 'error' && rt.error"
      variant="error"
      data-testid="document-error"
    >
      <AlertDescription>{{ rt.error.message }}</AlertDescription>
    </Alert>
    <Alert strip v-if="rt?.actionError" variant="error" data-testid="document-action-error">
      <AlertDescription>{{ rt.actionError }}</AlertDescription>
    </Alert>
    <SearchToolbar
      v-if="rt?.searchOpen"
      :tab-id="tab.id"
      :is-open="() => !!documentViewStore.runtime[tab.id]?.searchOpen"
      testid-prefix="document-"
      kind="documents"
      :api="pageSearchApi"
      @go-to-match="onGoToMatch"
      @close="onCloseSearch"
    />

    <!-- The reconnect gate used to replace this whole ViewChrome (header, toolbar and
         all) — every other view but the grid's DataView.vue did the same, the one inconsistency
         this fixes. The chrome above (and so its toolbar rows) now always renders; only the body
         — the part that needs a live connection — swaps for the gate. -->
    <EmptyState
      v-if="needsReconnect && !hasPage"
      data-testid="document-reconnect"
      icon="plug"
      :title="t('studio.common.disconnected.empty')"
    >
      <Button
        variant="dialog-primary"
        size="kira-lg"
        data-testid="document-reconnect-load"
        :pending="reconnecting"
        @click="onReconnectAndLoad"
      >
        {{ t('studio.common.reconnectAndLoad.label') }}
      </Button>
    </EmptyState>
    <template v-else>
      <div
        v-if="creatingNew"
        class="h-55 shrink-0 flex flex-col bg-elevated"
        data-testid="document-new"
      >
        <DocumentEditor
          :buffer="newBuffer"
          testid-prefix="document-new"
          :saving="saving"
          :save-disabled-reason="saveDisabledReason"
          @save="commitCreate"
          @cancel="cancelCreate"
        />
      </div>

      <div class="flex-1 min-h-0 overflow-y-auto" data-testid="document-list">
        <EmptyState
          v-if="!rt || rt.rowCount === 0"
          fill
          :icon="rt ? 'json' : undefined"
          :title="rt ? t('studio.documents.empty') : t('common.state.loading.loading')"
        >
          <template v-if="!rt" #media><Spinner :size="24" /></template>
        </EmptyState>
        <SearchFilterNotice
          v-else-if="displayRows && displayRows.length === 0"
          class="h-full"
          data-testid="document-no-matching-rows"
          :filter-key="tab.id"
          show-all-testid="document-show-all-rows"
        />
        <div
          v-else
          ref="scrollEl"
          class="overflow-y-auto h-full"
          data-testid="virtual-list"
          role="listbox"
          :aria-label="t('studio.common.documents.aria')"
          @scroll="onListScroll"
          @pointerover="showRowTip"
          @pointerout="hideRowTip"
          @focusin="showRowTip"
          @focusout="hideRowTip"
        >
          <Tooltip
            :open="rowTip !== null"
            @update:open="
              (o) => {
                if (!o) rowTip = null;
              }
            "
          >
            <TooltipTrigger as-child>
              <span
                aria-hidden="true"
                class="pointer-events-none fixed"
                :style="rowTipStyle"
              ></span>
            </TooltipTrigger>
            <TooltipContent>{{ rowTip?.label }}</TooltipContent>
          </Tooltip>
          <div class="relative" :style="{ height: `${totalSize}px` }">
            <template v-for="vi in virtualItems" :key="String(vi.key)">
              <DocumentRow
                v-if="rowAt(rows[vi.index])"
                data-testid="document-row"
                :class="VIRTUAL_ROW_CLASS"
                :style="{ transform: `translateY(${vi.start}px)`, height: `${vi.size}px` }"
                @contextmenu="onRowContextMenu($event, rows[vi.index])"
                :nav-props="nav.rowProps(String(rows[vi.index]))"
                :view="rowAt(rows[vi.index])!.view"
                :row-scope="tab.id"
                :expanded="
                  documentViewStore.isDocumentExpanded(tab.id, rowAt(rows[vi.index])!.view.id)
                "
                :selected="rt?.selectedRow === vi.index"
                :search-match="isSearchMatch(rows[vi.index])"
                :search-match-current="isCurrentSearchMatch(rows[vi.index])"
                @toggle="documentViewStore.toggleExpanded(tab.id, rowAt(rows[vi.index])!.view.id)"
                @select="onRowClick(vi.index)"
              >
                <template #actions>
                  <span class="flex-1 min-w-0"></span>
                  <div class="flex shrink-0 items-center gap-1">
                    <Badge v-if="editingRow === rows[vi.index]" variant="warn">{{
                      t('studio.documents.editing.status')
                    }}</Badge>
                    <RowActionButton
                      :icon="ICON.edit"
                      :label="editGateFor(rows[vi.index]).label"
                      :aria-label="t('common.action.edit.label')"
                      :aria-pressed="editingRow === rows[vi.index]"
                      :disabled="!editGateFor(rows[vi.index]).editable"
                      data-testid="document-edit"
                      @click.stop="
                        startEdit(
                          rows[vi.index],
                          rowAt(rows[vi.index])!.view.id,
                          rowAt(rows[vi.index])!.body,
                        )
                      "
                    />
                    <RowActionButton
                      :icon="ICON.delete"
                      :label="deleteTitle"
                      :aria-label="t('common.action.delete.label')"
                      :disabled="!canDelete"
                      data-testid="document-delete"
                      @click.stop="onDeleteRow(rowAt(rows[vi.index])!.view.id)"
                    />
                  </div>
                </template>
                <template #body>
                  <div
                    v-if="
                      isSearchMatch(rows[vi.index]) &&
                      !documentViewStore.isDocumentExpanded(tab.id, rowAt(rows[vi.index])!.view.id)
                    "
                    class="truncate text-muted-foreground text-kira-sm font-data px-2 pb-1 in-[.search-match-current]:text-bg"
                    data-testid="document-search-preview"
                  >
                    <HighlightedText
                      :parts="previewSegments(rows[vi.index], rowAt(rows[vi.index])!.body)"
                    />
                  </div>
                  <div
                    v-if="
                      documentViewStore.isDocumentExpanded(tab.id, rowAt(rows[vi.index])!.view.id)
                    "
                    class="flex-1 min-h-0 flex flex-col overflow-hidden bg-elevated"
                    data-testid="document-body"
                  >
                    <!-- The editor is the same code surface the definition view and the console views
                       use — the only difference is the language. -->
                    <DocumentEditor
                      v-if="editingRow === rows[vi.index]"
                      :buffer="editBuffer"
                      testid-prefix="document-edit"
                      :saving="saving"
                      :save-disabled-reason="saveDisabledReason"
                      @save="commitEdit"
                      @cancel="cancelEdit"
                    />
                    <DocumentTree
                      v-else-if="rowAt(rows[vi.index])!.view.root"
                      :tab-id="tab.id"
                      :row="rows[vi.index]"
                      @toggle-path="
                        (path) => documentRowsStore.togglePath(tab.id, rows[vi.index], path)
                      "
                    />
                    <MonacoHost
                      v-else
                      :doc="rowAt(rows[vi.index])!.body"
                      language="json"
                      readonly
                    />
                  </div>
                </template>
              </DocumentRow>
            </template>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
