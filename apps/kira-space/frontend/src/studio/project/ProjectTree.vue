<script setup lang="ts">
import type { ShortcutId } from '@shared/domain/shortcuts';
import CodiconIcon from '@theme/CodiconIcon.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { useEventListener } from '@vueuse/core';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { useTreeShortcuts } from '@workbench/behaviour/treeShortcuts';
import { useTreeVirtualRows } from '@workbench/behaviour/treeVirtualRows';
import { STICKY_ROW_CLASS, VIRTUAL_ROW_CLASS } from '@workbench/behaviour/virtualRows';
import { logError } from '@workbench/bridge/log';
import { useRowHeight } from '@workbench/lib/rowHeight';
import {
  type MenuItem,
  runMenuShortcut,
  runMenuShortcutAsync,
  useContextMenuStore,
} from '@workbench/state/contextMenu';
import { computed, ref, useTemplateRef, watch } from 'vue';
import { useConnectionConfig } from '../state/connectionConfig';
import { useConnectionDialogStore } from '../state/connectionDialogs';
import { useConnectionsStore } from '../state/connections';
import { useTabsStore } from '../state/tabs';
import { reloadTab } from '../state/viewCommands';
import { useConnectionReorder } from './connectionReorder';
import { emptyBackgroundMenu, menuForRow } from './menu';
import { type TreeRowVm, useTreeStore } from './state/tree';
import TreeRow from './TreeRow.vue';

const contextMenuStore = useContextMenuStore();
const connectionsStore = useConnectionsStore();
const tabsStore = useTabsStore();
const treeStore = useTreeStore();
const connectionDialogStore = useConnectionDialogStore();
const config = useConnectionConfig();

// A file that never held a valid connection has no row; list it so it can be fixed.
const brokenFiles = computed(() =>
  (config.data.value?.files ?? []).filter((f) => f.problem !== null && f.id === ''),
);

// Double-click opens a data tab for a relation (§8.10's "Open data" — the same action) rather
// than toggling the twisty, which the twisty button itself already does.
const OPENABLE_KINDS = new Set(['table', 'view', 'matview']);
// A collection opens the same way, but into a 'document' tab (P8) — not the grid's 'data' tab.
const DOCUMENT_OPENABLE_KINDS = new Set(['collection']);
// A redis key opens into a 'keyvalue' tab (P9) — 'namespace'/'database' stay expand-only, like
// mongo's own 'database' node. An s3 object reuses the exact same tab kind (P17's page.ts doc
// comment explains why) — 'prefix'/'bucket' stay expand-only the same way.
const KEYVALUE_OPENABLE_KINDS = new Set(['key', 'object']);
// A kafka topic or sqs queue opens into a 'stream' tab (P10) — 'partition'/'consumerGroup' stay
// browse-only leaves with nothing to open (onOpen's hasChildren guard makes double-click a no-op
// on them, same as a column/index leaf).
const STREAM_OPENABLE_KINDS = new Set(['topic', 'queue']);
// A redis `database` / s3 `bucket` row, on a connection whose caps say its key space is
// unbounded — double-click/Enter opens a Browse tab instead of the ordinary expand/collapse
// toggle. Gated on the capability, never the row kind alone (`database` is shared by six engines
// that stay expand-only) or the connection kind (Caps is the only thing the UI reads,
// ARCHITECTURE.md).
function isKeyBrowserRow(row: TreeRowVm): boolean {
  if (row.kind !== 'database' && row.kind !== 'bucket') return false;
  return connectionsStore.states[row.connectionId]?.caps?.keyBrowser === true;
}

// A kibana data view opens in Discover, not the document tab other engines' collections use.
function isKibanaConnection(connectionId: string): boolean {
  return connectionsStore.connectionRecord(connectionId)?.kind === 'kibana';
}

const { single: rowHeight } = useRowHeight();

// P104 §3.4: TreeHost's own recipe (virtualization + pinned ancestor band + reveal-scroll),
// inlined via the shared useTreeVirtualRows composable rather than kept as a wrapper component.
const scrollEl = ref<HTMLElement | null>(null);
const { virtualItems, totalSize, band, onScroll, revealKey } = useTreeVirtualRows({
  rows: () => treeStore.visibleRows,
  rowHeight: () => rowHeight.value,
  scrollElement: scrollEl,
});

// Raw `search` (not the debounced query) so typing pauses drag at once.
const { draggingId, drop } = useConnectionReorder({
  container: scrollEl,
  ids: () => connectionsStore.records.map((r) => r.id),
  disabled: () => treeStore.search.trim() !== '' || connectionsStore.records.length < 2,
  rowHeight: () => rowHeight.value,
  onReorder: (ids) =>
    void connectionsStore
      .reorderConnections(ids)
      .catch((err) => logError('projectTree', 'reorder failed', err)),
});

function dragProps(row: TreeRowVm) {
  if (row.kind !== 'connection') return {};
  return {
    dragging: draggingId.value === row.connectionId,
    dropEdge: drop.value?.id === row.connectionId ? drop.value.edge : null,
  };
}

// initTreeSync/initSchemaSync/initSchemaColumnsSync used to be wired from here,
// on mount — moved to main.ts's bootstrap() so they're live whether or not this panel ever
// mounts (a window in API/Terminal mode, or one with no connections at boot, used to get none of
// them for the whole session). See bootstrap()'s own comment for the full reasoning.

// revealPath() (Step 7b) sets pendingScrollKey once its expansion/selection work is done;
// scrolling happens here, one tick later, once treeStore.visibleRows reflects the newly expanded
// nodes — revealKey() does the animation-frame wait, the index lookup and the band-inset scroll.
watch(
  () => treeStore.pendingScrollKey,
  async (key) => {
    if (!key) return;
    treeStore.pendingScrollKey = null;
    await revealKey(key);
  },
  // A reveal requested while the panel was hidden leaves its key set; scroll once the tree mounts.
  { immediate: true },
);

function onSelect(row: TreeRowVm): void {
  treeStore.selectRow(row.key);
}

// The twisty always expands/collapses. A group row has no adapter path behind it — it's a
// pure view over its parent's already-fetched children — so it toggles treeStore.expanded
// directly rather than going through treeStore.expand()/treeStore.collapse(), which would connect
// the connection and issue an IPC call for a synthetic path no adapter has ever heard of.
function onToggle(row: TreeRowVm): void {
  if (row.kind === 'group') {
    treeStore.toggleGroup(row.connectionId, row.path);
    return;
  }
  if (row.expanded) treeStore.collapse(row.connectionId, row.path);
  else void treeStore.expand(row.connectionId, row.path);
}

// Task 62: double-clicking a tree row that already has an open tab for the same
// (connectionId, path) used to just refocus it — no refetch. A freshly created tab is about to
// fetch on mount anyway, so only the reused case needs an explicit reload here.
function onOpen(row: TreeRowVm): void {
  // A group folder only ever toggles on double-click — it opens nothing (P19 D4).
  if (row.kind === 'group') {
    treeStore.toggleGroup(row.connectionId, row.path);
    return;
  }
  if (OPENABLE_KINDS.has(row.kind)) {
    const opened = tabsStore.openDataTab(row.connectionId, row.path);
    if (opened?.reused) reloadTab('data', opened.id);
    return;
  }
  if (row.kind === 'collection' && isKibanaConnection(row.connectionId)) {
    tabsStore.openDiscoverTab(row.connectionId, row.path, { title: row.name });
    return;
  }
  if (DOCUMENT_OPENABLE_KINDS.has(row.kind)) {
    const opened = tabsStore.openDocumentTab(row.connectionId, row.path);
    if (opened?.reused) reloadTab('document', opened.id);
    return;
  }
  if (KEYVALUE_OPENABLE_KINDS.has(row.kind)) {
    const opened = tabsStore.openKeyValueTab(row.connectionId, row.path);
    if (opened?.reused) reloadTab('keyvalue', opened.id);
    return;
  }
  if (STREAM_OPENABLE_KINDS.has(row.kind)) {
    const opened = tabsStore.openStreamTab(row.connectionId, row.path);
    if (opened?.reused) reloadTab('stream', opened.id);
    return;
  }
  if (isKeyBrowserRow(row)) {
    const { id, reused } = tabsStore.openBrowseTab(row.connectionId, row.path);
    if (reused) reloadTab('browse', id);
    return;
  }
  // A childless, non-openable leaf (column, index) has nothing to open or expand — TreeRow.vue
  // now emits 'open' unconditionally (P9 fix), so this guard is what keeps dblclick a no-op there.
  if (!row.hasChildren) return;
  if (row.expanded) treeStore.collapse(row.connectionId, row.path);
  else void treeStore.expand(row.connectionId, row.path);
}

async function rowMenuItems(row: TreeRowVm): Promise<MenuItem[]> {
  // The "Saved filters ▸" submenu (Step 13) is built synchronously by menuForRow() from
  // treeStore.savedQueries, so it must already be populated by the time the menu opens.
  if (OPENABLE_KINDS.has(row.kind)) {
    try {
      await treeStore.loadSavedQueries(row.connectionId, row.path);
    } catch {
      // P108 Part 12 F11: a queriesList rejection (DB failure) must not leave the menu unopened
      // and an unhandled rejection behind — menuForRow() just renders an empty "Saved filters ▸"
      // submenu (treeStore.savedQueries has no entry for this row) instead.
    }
  }
  return menuForRow(row);
}

async function onContextMenu(row: TreeRowVm, event: MouseEvent): Promise<void> {
  // Right-click selects the row (without opening anything) before its menu opens.
  nav.contextMenu(row, event, await rowMenuItems(row));
}

function onBackgroundContextMenu(event: MouseEvent): void {
  // TreeRow.vue stops propagation on its own contextmenu handler, so only a right-click on
  // the empty area below/around the rows (the virtual list's spacer divs) ever reaches here.
  event.preventDefault();
  contextMenuStore.openContextMenu(event, emptyBackgroundMenu());
}

// Arrows, Home/End, Right/Left expand and parent, Enter (the row's primary action, the same one
// double-click performs), Space and Shift+F10 come from the shared list navigation. The selection
// is the roving stop. The pinned sticky-band rows carry no tabindex of their own.
const activeKey = computed({
  get: () => treeStore.selected,
  set: (key) => {
    if (key) treeStore.selectRow(key);
  },
});
const nav = useListNavigation<TreeRowVm>({
  container: scrollEl,
  items: () => treeStore.visibleRows,
  key: (row) => row.key,
  activeKey,
  tree: {
    depth: (row) => row.depth,
    isExpanded: (row) => row.expanded,
    hasChildren: (row) => row.hasChildren,
    expand: onToggle,
    collapse: onToggle,
  },
  onOpen,
  onSelect,
  onMenu: async (row, rect) =>
    contextMenuStore.openContextMenuAt(rect.left, rect.bottom, await rowMenuItems(row), {
      onRestoreFocus: () => void nav.focusKey(row.key),
    }),
  scrollToIndex: (_index, key) => revealKey(key),
  rendered: () => virtualItems.value,
  rowSelector: () => '[role="treeitem"][tabindex="0"]',
});

// Dispatches through the menu builder, so disabled gating and the printed chord stay in one place.
function viaMenu(id: ShortcutId): (key: string) => void {
  return (key) => {
    const row = treeStore.visibleRows.find((r) => r.key === key);
    if (row) runMenuShortcut(menuForRow(row), id);
  };
}
// Rows are index-keyed, so after a move the focused element would show another row: wait for the
// move to land, scroll to the moved row and focus it again.
function moveViaMenu(id: ShortcutId): (key: string) => void {
  return (key) => {
    const row = treeStore.visibleRows.find((r) => r.key === key);
    if (!row) return;
    void runMenuShortcutAsync(menuForRow(row), id)
      .then(() => revealKey(key))
      .then(() => nav.focusKey(key));
  };
}
useTreeShortcuts(nav, {
  'tree.copyName': viaMenu('tree.copyName'),
  'tree.copyUri': viaMenu('tree.copyUri'),
  'tree.duplicate': viaMenu('tree.duplicate'),
  'tree.delete': viaMenu('tree.delete'),
  'list.moveUp': moveViaMenu('list.moveUp'),
  'list.moveDown': moveViaMenu('list.moveDown'),
});

// The tree background has no interactive role of its own: bind its listener via VueUse.
const treeBodyEl = useTemplateRef<HTMLElement>('treeBodyEl');
useEventListener(treeBodyEl, 'contextmenu', onBackgroundContextMenu);
</script>

<template>
  <div class="h-full flex flex-col min-h-0">
    <div ref="treeBodyEl" class="flex-1 min-h-0" data-testid="tree-background">
      <div
        ref="scrollEl"
        :class="['virtual-list h-full overflow-auto', draggingId && 'select-none']"
        data-testid="virtual-list"
        role="tree"
        :aria-label="t('studio.project.tree.aria')"
        @scroll="onScroll"
      >
        <div class="virtual-list-sticky sticky top-0 z-2 h-0" data-testid="tree-sticky-band">
          <template v-for="slot in band" :key="slot.row.key">
            <TreeRow
              :class="STICKY_ROW_CLASS"
              :style="{ top: `${slot.top}px`, height: `${rowHeight}px` }"
              :row="slot.row"
              :selected="treeStore.selected === slot.row.key"
              :sticky="true"
              v-bind="dragProps(slot.row)"
              @select="onSelect"
              @toggle="onToggle"
              @open="onOpen"
              @contextmenu="onContextMenu"
            />
          </template>
        </div>
        <div class="relative" :style="{ height: `${totalSize}px` }">
          <template v-for="item in virtualItems" :key="String(item.key)">
            <TreeRow
              :class="VIRTUAL_ROW_CLASS"
              :style="{ transform: `translateY(${item.start}px)`, height: `${item.size}px` }"
              :row="treeStore.visibleRows[item.index]"
              :selected="treeStore.selected === treeStore.visibleRows[item.index].key"
              :sticky="false"
              v-bind="dragProps(treeStore.visibleRows[item.index])"
              @select="onSelect"
              @toggle="onToggle"
              @open="onOpen"
              @contextmenu="onContextMenu"
            />
          </template>
        </div>
      </div>
    </div>
    <ul
      v-if="brokenFiles.length"
      class="m-0 flex list-none flex-col gap-0.5 p-1"
      data-testid="tree-connection-broken"
    >
      <li v-for="f in brokenFiles" :key="f.file">
        <Button
          variant="link"
          size="inline"
          class="w-full min-w-0 justify-start text-error"
          :data-testid="`tree-connection-broken-${f.file}`"
          @click="connectionDialogStore.openFile(f.file)"
        >
          <CodiconIcon name="warning" :size="13" />
          <TruncatedText
            :text="`${f.file}: ${f.problem?.line ? t('common.config.lineError.error', { line: f.problem.line, text: f.problem.message }) : f.problem?.message}`"
          />
        </Button>
      </li>
    </ul>
    <Alert v-if="treeStore.searchIncomplete" variant="info" data-testid="search-incomplete-note">
      <AlertDescription>
        {{ t('studio.project.tree.searchIncomplete.body') }}
      </AlertDescription>
    </Alert>
  </div>
</template>
