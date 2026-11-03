<script setup lang="ts">
import { t } from '@theme/i18n';
import { useEventListener } from '@vueuse/core';
import { copyOrReportError } from '@workbench/behaviour/clipboard';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { announceApp } from '@workbench/behaviour/liveRegion';
import { useTreeVirtualRows } from '@workbench/behaviour/treeVirtualRows';
import { STICKY_ROW_CLASS, VIRTUAL_ROW_CLASS } from '@workbench/behaviour/virtualRows';
import { errorText } from '@workbench/bridge/codedError';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, ref, useTemplateRef, watch } from 'vue';
import { control } from '../bridge/control';
import CollectionRow from './CollectionRow.vue';
import { backgroundMenu, type CollectionMenuActions, menuForRow } from './menu';
import { useCollectionSelectionStore } from './state/collectionSelection';
import { type CollectionRowVm, useCollectionsStore } from './state/collections';
import { useImportCurlStore } from './state/curl';
import { useDynamicValuesStore } from './state/dynamicValues';
import { useVariablesStore } from './state/variables';
import { openVariableSetTab } from './tabs';

const contextMenuStore = useContextMenuStore();
const dynamicValuesStore = useDynamicValuesStore();
const collectionsStore = useCollectionsStore();
const selection = useCollectionSelectionStore();
const importCurlStore = useImportCurlStore();
const variablesStore = useVariablesStore();

// P104 §3.4: TreeHost's own recipe (virtualization + the pinned ancestor band + reveal-scroll),
// inlined via the shared useTreeVirtualRows composable rather than kept as a wrapper component —
// the same swap ProjectTree.vue's own call site already made.
const { single: rowHeight } = useRowHeight();
const scrollEl = ref<HTMLElement | null>(null);
const { virtualItems, totalSize, band, onScroll, revealKey } = useTreeVirtualRows({
  rows: () => collectionsStore.visibleRows,
  rowHeight: () => rowHeight.value,
  scrollElement: scrollEl,
});

/** Called by the panel after a mutation adds a row worth scrolling to. */
async function reveal(key: string): Promise<void> {
  await revealKey(key);
}
defineExpose({ reveal });

const visibleKeys = computed(() => collectionsStore.visibleRows.map((r) => r.key));

function onSelect(row: CollectionRowVm): void {
  if (selection.focusKey !== row.key) selection.selectOnly(row.key);
}

// A row that leaves the visible list (collapse, search, a change from another window) leaves the
// selection too. A row just selected but not yet listed stays: only rows seen before and gone now
// are dropped.
watch(visibleKeys, (next, prev) => {
  const now = new Set(next);
  selection.drop(new Set(prev.filter((key) => !now.has(key))));
});

// The twisty always expands/collapses; unlike Studio's tree there is no adapter call behind it and
// no synthetic group row to special-case (F15) — the whole tree is already in memory.
function onToggle(row: CollectionRowVm): void {
  collectionsStore.toggleRow(row);
}

// A collection and a folder toggle on double-click; a request opens into the existing
// 'http-request' tab kind (D14), reusing an already-open tab bound to the same row.
async function onOpen(row: CollectionRowVm): Promise<void> {
  if (row.kind !== 'request') {
    collectionsStore.toggleRow(row);
    return;
  }
  await collectionsStore.openRequestRow(row);
}

/** The config file that holds `row`: its collection's. */
function fileOf(row: CollectionRowVm): string {
  return collectionsStore.collectionRecord(row.collectionId)?.file ?? '';
}

async function runFileAction(
  row: CollectionRowVm,
  call: (args: { folder: string; file: string }) => Promise<void>,
): Promise<void> {
  const file = fileOf(row);
  if (!file) return;
  try {
    await call({ folder: 'api', file });
  } catch (err) {
    collectionsStore.error = errorText(err);
  }
}

// The menu is a description (menu.ts) and this is the one place that knows how to perform any of
// it — so the actions are injected rather than imported there, which also keeps that module free
// of both the store's mutation half and the tab-opening path.
const actions: CollectionMenuActions = {
  open: (row) => void onOpen(row),
  newRequest: (row) =>
    void collectionsStore.createItem(row.collectionId, folderTarget(row), 'request'),
  newGrpcRequest: (row) =>
    void collectionsStore.createGrpcItem(row.collectionId, folderTarget(row)),
  newFolder: (row) =>
    void collectionsStore.createItem(row.collectionId, folderTarget(row), 'folder'),
  newCollection: () => void collectionsStore.createCollection(),
  copyUrl: (row) =>
    void copyOrReportError(
      row.url,
      (message) => {
        collectionsStore.error = message;
      },
      () => announceApp(t('studio.common.copiedUrl.announce')),
    ),
  importPostman: () => void collectionsStore.importPostman(),
  importCurl: () => importCurlStore.openImportCurlDialog(),
  exportCollection: (row) => void collectionsStore.exportCollection(row.id, row.name),
  variables: (row) => openVariableSetTab('collection', row.id, row.name),
  environments: () => variablesStore.openEnvironments(),
  dynamicValues: () => dynamicValuesStore.openDynamicValuesDialog(),
  openFile: (row) => void runFileAction(row, control.configFilesOpen),
  revealFile: (row) => void runFileAction(row, control.configFilesReveal),
};

/** Creating *into* a collection row means the root; into a folder row means that folder. */
function folderTarget(row: CollectionRowVm): string | null {
  return row.kind === 'collection' ? null : row.id;
}

function onContextMenu(row: CollectionRowVm, event: MouseEvent): void {
  nav.contextMenu(row, event, menuForRow(row, actions));
}

function onBackgroundContextMenu(event: MouseEvent): void {
  event.preventDefault();
  contextMenuStore.openContextMenu(event, backgroundMenu(actions));
}

// Arrows, Home/End, Right/Left expand and parent, Enter (the row's primary action, the same one
// double-click performs), Space and Shift+F10 come from the shared list navigation. The selection
// is the roving stop.
const activeKey = computed({
  get: () => selection.focusKey,
  set: (key) => {
    if (key && key !== selection.focusKey) selection.selectOnly(key);
  },
});
const nav = useListNavigation<CollectionRowVm>({
  container: scrollEl,
  items: () => collectionsStore.visibleRows,
  key: (row) => row.key,
  activeKey,
  tree: {
    depth: (row) => row.depth,
    isExpanded: (row) => row.expanded,
    hasChildren: (row) => row.hasChildren,
    expand: collectionsStore.expandRow,
    collapse: collectionsStore.collapseRow,
  },
  onOpen,
  onSelect: (row) => onSelect(row),
  onMenu: (row, rect) =>
    contextMenuStore.openContextMenuAt(rect.left, rect.bottom, menuForRow(row, actions), {
      onRestoreFocus: () => void nav.focusKey(selection.focusKey ?? row.key),
    }),
  scrollToIndex: (_index, key) => revealKey(key),
  rendered: () => virtualItems.value,
  rowSelector: () => '[role="treeitem"][tabindex="0"]',
});

const hasVisibleFocus = computed(() =>
  collectionsStore.visibleRows.some((r) => r.key === selection.focusKey),
);

// The tree background has no interactive role of its own: bind its listener via VueUse.
const treeBodyEl = useTemplateRef<HTMLElement>('treeBodyEl');
useEventListener(treeBodyEl, 'contextmenu', onBackgroundContextMenu);
</script>

<template>
  <div class="h-full flex flex-col min-h-0">
    <div ref="treeBodyEl" class="flex-1 min-h-0" data-testid="tree-background">
      <div
        ref="scrollEl"
        class="virtual-list h-full overflow-auto"
        data-testid="virtual-list"
        role="tree"
        :aria-label="t('studio.api.collections.tree.aria')"
        @scroll="onScroll"
      >
        <div class="virtual-list-sticky sticky top-0 z-2 h-0" data-testid="tree-sticky-band">
          <template v-for="slot in band" :key="slot.row.key">
            <CollectionRow
              :class="STICKY_ROW_CLASS"
              :style="{ top: `${slot.top}px`, height: `${rowHeight}px` }"
              :row="slot.row"
              :selected="selection.isSelected(slot.row.key)"
              :focused="selection.focusKey === slot.row.key"
              :sticky="true"
              @select="onSelect"
              @toggle="onToggle"
              @open="onOpen"
              @contextmenu="onContextMenu"
            />
          </template>
        </div>
        <div class="relative" :style="{ height: `${totalSize}px` }">
          <template v-for="item in virtualItems" :key="String(item.key)">
            <CollectionRow
              :class="VIRTUAL_ROW_CLASS"
              :style="{ transform: `translateY(${item.start}px)`, height: `${item.size}px` }"
              :row="collectionsStore.visibleRows[item.index]"
              :selected="selection.isSelected(collectionsStore.visibleRows[item.index].key)"
              :focused="selection.focusKey === collectionsStore.visibleRows[item.index].key"
              :tabbable="!hasVisibleFocus && item.index === 0"
              :sticky="false"
              @select="onSelect"
              @toggle="onToggle"
              @open="onOpen"
              @contextmenu="onContextMenu"
            />
          </template>
        </div>
      </div>
    </div>
  </div>
</template>
