<script setup lang="ts">
import type { ShortcutId } from '@shared/domain/shortcuts';
import { t } from '@theme/i18n';
import { useEventListener } from '@vueuse/core';
import { copyOrReportError } from '@workbench/behaviour/clipboard';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { announceApp } from '@workbench/behaviour/liveRegion';
import { useTreePointerDrag } from '@workbench/behaviour/treePointerDrag';
import { useTreeShortcuts } from '@workbench/behaviour/treeShortcuts';
import { useTreeVirtualRows } from '@workbench/behaviour/treeVirtualRows';
import { STICKY_ROW_CLASS, VIRTUAL_ROW_CLASS } from '@workbench/behaviour/virtualRows';
import { errorText } from '@workbench/bridge/codedError';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { isPrimaryModifier } from '@workbench/shortcuts/keys';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import {
  runMenuShortcut,
  runMenuShortcutAsync,
  useContextMenuStore,
} from '@workbench/state/contextMenu';
import { computed, ref, useTemplateRef, watch } from 'vue';
import { control } from '../bridge/control';
import CollectionRow from './CollectionRow.vue';
import {
  type DropTarget,
  type DropZone,
  type Placement,
  placementFor,
  topmost,
  zoneFor,
} from './collectionDrop';
import { backgroundMenu, type CollectionMenuActions, menuForRow, menuForSelection } from './menu';
import { useApiVariablesDialogStore } from './state/apiVariablesDialog';
import { useCollectionSelectionStore } from './state/collectionSelection';
import { type CollectionRowVm, useCollectionsStore } from './state/collections';
import { useImportCurlStore } from './state/curl';
import { useDynamicValuesStore } from './state/dynamicValues';
import { useVariablesStore } from './state/variables';
import { hasOpenRequestTab, openVariableSetTab } from './tabs';

const confirmDialogStore = useConfirmDialogStore();
const contextMenuStore = useContextMenuStore();
const dynamicValuesStore = useDynamicValuesStore();
const variablesDialogStore = useApiVariablesDialogStore();
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
const rowByKey = computed(() => new Map(collectionsStore.visibleRows.map((r) => [r.key, r])));
const selectedRows = computed(() =>
  collectionsStore.visibleRows.filter((r) => selection.isSelected(r.key)),
);

// A click picks, Ctrl/Cmd toggles, Shift extends a range, both add a range. A select with no event
// is the keyboard (Space) or focus landing on an unselected row.
function onSelect(row: CollectionRowVm, event?: MouseEvent): void {
  if (!event) {
    if (!selection.isSelected(row.key) || selection.focusKey !== row.key) {
      selection.selectOnly(row.key);
    }
    return;
  }
  if (event.shiftKey) selection.extendTo(row.key, visibleKeys.value, isPrimaryModifier(event));
  else if (isPrimaryModifier(event)) selection.toggle(row.key);
  else selection.selectOnly(row.key);
}

// A row that leaves the visible list (collapse, search, delete, a change from another window)
// leaves the selection too, so a bulk action never reaches a row the user cannot see. A row just
// selected but not yet listed stays: only rows seen before and gone now are dropped.
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
  rename: (row) => contextMenuStore.afterClose(() => collectionsStore.beginRename(row)),
  duplicate: (row) => void collectionsStore.duplicateRow(row),
  remove: (rows) => void confirmAndDeleteRows(rows),
  collections: () => collectionsStore.collections,
  moveTo: (rows, collectionId) => void collectionsStore.moveRowsToCollection(rows, collectionId),
  moveToNewCollection: (rows) => void collectionsStore.moveRowsToNewCollection(rows),
  moveStep: async (row, step) => {
    await collectionsStore.moveStep(row, step);
  },
  canMoveStep: (row, step) => collectionsStore.canMoveStep(row, step),
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
  editVariables: (row) => {
    const file = fileOf(row);
    if (file) variablesDialogStore.open(file);
  },
  environments: () => variablesStore.openEnvironments(),
  dynamicValues: () => dynamicValuesStore.openDynamicValuesDialog(),
  openFile: (row) => void runFileAction(row, control.configFilesOpen),
  revealFile: (row) => void runFileAction(row, control.configFilesReveal),
};

/** Creating *into* a collection row means the root; into a folder row means that folder. */
function folderTarget(row: CollectionRowVm): string | null {
  return row.kind === 'collection' ? null : row.id;
}

const DELETE_CONFIRM = {
  collection: {
    title: 'common.deleteCollectionConfirm.title',
    body: 'studio.api.collections.deleteTreeConfirm.body',
  },
  folder: {
    title: 'studio.api.collections.deleteFolderConfirm.title',
    body: 'studio.api.collections.deleteTreeConfirm.body',
  },
  request: {
    title: 'studio.api.collections.deleteRequestConfirm.title',
    body: 'studio.api.collections.deleteRequestConfirm.body',
  },
} as const;

const COUNT_LABEL = {
  request: 'studio.api.importReport.requests.label',
  folder: 'studio.api.importReport.folders.label',
  collection: 'studio.api.collections.deleteManyConfirm.collections.label',
} as const;

async function confirmAndDeleteRows(rows: readonly CollectionRowVm[]): Promise<void> {
  const plan = collectionsStore.planDelete(rows);
  if (plan.top.length === 0) return;
  let message: string;
  let title: string;
  if (plan.top.length === 1) {
    // A folder and a collection take their whole subtree with them, which the prompt says out loud.
    const [row] = plan.top;
    const keys = DELETE_CONFIRM[row.kind];
    title = t(keys.title);
    message = [t(keys.body, { name: row.name }), t('common.confirm.irreversible.body')].join(' ');
  } else {
    const counts = (['request', 'folder', 'collection'] as const)
      .map((kind) => [kind, plan.top.filter((r) => r.kind === kind).length] as const)
      .filter(([, count]) => count > 0)
      .map(([kind, count]) => t(COUNT_LABEL[kind], { count }));
    const nested = plan.itemIds.size > plan.top.filter((r) => r.kind !== 'collection').length;
    title = t('studio.api.collections.deleteManyConfirm.title', { count: plan.top.length });
    message = [
      t('studio.api.collections.deleteManyConfirm.body', { list: counts.join(', ') }),
      nested ? t('studio.api.collections.deleteManyConfirm.nested.body') : '',
      hasOpenRequestTab(plan.itemIds)
        ? t('studio.api.collections.deleteManyConfirm.openTabs.body')
        : '',
      t('common.confirm.irreversible.body'),
    ]
      .filter(Boolean)
      .join(' ');
  }
  const confirmed = await confirmDialogStore.confirm({
    title,
    message,
    confirmLabel: t('common.action.delete.label'),
    tone: 'danger',
  });
  if (!confirmed) return;
  await collectionsStore.deleteRows(plan.top);
}

/** The menu a right-click on `row` opens: the set's when `row` is one of several selected. */
function menuItemsFor(row: CollectionRowVm) {
  return selection.isSelected(row.key) && selection.count > 1
    ? menuForSelection(
        selectedRows.value,
        collectionsStore.planDelete(selectedRows.value).top.length,
        actions,
      )
    : menuForRow(row, actions);
}

function onContextMenu(row: CollectionRowVm, event: MouseEvent): void {
  if (selection.isSelected(row.key) && selection.count > 1) {
    event.preventDefault();
    contextMenuStore.openContextMenu(event, menuItemsFor(row), {
      onRestoreFocus: () => void nav.focusKey(selection.focusKey ?? row.key),
    });
    return;
  }
  nav.contextMenu(row, event, menuForRow(row, actions));
}

function onBackgroundContextMenu(event: MouseEvent): void {
  event.preventDefault();
  contextMenuStore.openContextMenu(event, backgroundMenu(actions));
}

function onRename(row: CollectionRowVm, name: string): void {
  void collectionsStore.renameRow(row, name);
}

// Arrows, Home/End, Right/Left expand and parent, Enter (the row's primary action, the same one
// double-click performs), Space and Shift+F10 come from the shared list navigation. The selection
// is the roving stop. The tree.* chords dispatch through the menu builder a right-click would call,
// so the printed shortcut and the executed action are the same object.
// Setting the key a range already focuses is a no-op, or every Shift+arrow would reset its range.
const activeKey = computed({
  get: () => selection.focusKey,
  set: (key) => {
    if (!key || (key === selection.focusKey && selection.isSelected(key))) return;
    selection.selectOnly(key);
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
  onExtend: (row) => selection.extendTo(row.key, visibleKeys.value, false),
  onMenu: (row, rect) =>
    contextMenuStore.openContextMenuAt(rect.left, rect.bottom, menuItemsFor(row), {
      onRestoreFocus: () => void nav.focusKey(selection.focusKey ?? row.key),
    }),
  scrollToIndex: (_index, key) => revealKey(key),
  rendered: () => virtualItems.value,
  rowSelector: () => '[role="treeitem"][tabindex="0"]',
});

// Dispatches through the menu builder, so disabled gating and the printed chord stay in one place.
function viaMenu(id: ShortcutId): (key: string) => void {
  return (key) => {
    const row = rowByKey.value.get(key);
    if (row) runMenuShortcut(menuItemsFor(row), id);
  };
}
// Rows are index-keyed, so after a move the focused element would show another row: wait for the
// move to land, scroll to the moved row and focus it again. A multi-selection has no move items,
// so the chord does nothing.
function moveViaMenu(id: ShortcutId): (key: string) => void {
  return (key) => {
    const row = rowByKey.value.get(key);
    if (!row || selection.count > 1) return;
    void runMenuShortcutAsync(menuItemsFor(row), id)
      .then(() => revealKey(key))
      .then(() => nav.focusKey(key));
  };
}
useTreeShortcuts(nav, {
  'tree.selectAll': () => selection.selectAll(visibleKeys.value),
  'tree.rename': viaMenu('tree.rename'),
  'tree.delete': viaMenu('tree.delete'),
  'tree.duplicate': viaMenu('tree.duplicate'),
  'list.moveUp': moveViaMenu('list.moveUp'),
  'list.moveDown': moveViaMenu('list.moveDown'),
});

const hasVisibleFocus = computed(() =>
  collectionsStore.visibleRows.some((r) => r.key === selection.focusKey),
);

// Escape collapses a multi-selection to the focused row. A rename input or a drag claims its own
// Escape first.
useEventListener(scrollEl, 'keydown', (e: KeyboardEvent) => {
  if (e.key !== 'Escape' || e.defaultPrevented || selection.count < 2 || drag.dragging.value)
    return;
  if ((e.target as HTMLElement | null)?.closest('input, textarea')) return;
  e.preventDefault();
  selection.collapseToFocus();
});

// Pointer drag of the selection onto a row: before/after reorders, into a folder or collection
// re-parents. The pressed row joins the selection when it is not in it. Collections do not drag:
// they list by name.
type RowDrop = { key: string; zone: DropZone; placement: Placement };
let dragIds: string[] = [];
const dragCount = ref(0);
const pointer = ref({ x: 0, y: 0 });
const searching = computed(() => collectionsStore.activeSearchQuery !== '');

const drag = useTreePointerDrag<RowDrop>({
  container: scrollEl,
  pressable: (target) => {
    if (target.closest('input, textarea')) return null;
    const row = target.closest<HTMLElement>('[data-testid="collection-row"]');
    return row ?? null;
  },
  disabled: () => collectionsStore.renamingKey !== null,
  rowHeight: () => rowHeight.value,
  onStart: (pressed) => {
    const key = pressed.dataset.key;
    if (!key) return false;
    if (!selection.isSelected(key)) selection.selectOnly(key);
    const rows = selectedRows.value;
    if (rows.some((r) => r.kind === 'collection')) return false;
    dragIds = topmost(rows, collectionsStore.items).map((r) => r.id);
    dragCount.value = dragIds.length;
    return dragIds.length > 0;
  },
  resolve: (hit, y) => {
    const el = hit?.closest<HTMLElement>('[data-testid="collection-row"]');
    const row = el?.dataset.key ? rowByKey.value.get(el.dataset.key) : undefined;
    if (!el || !row) return null;
    const rect = el.getBoundingClientRect();
    const zone = zoneFor(row, (y - rect.top) / rect.height, searching.value);
    if (!zone) return null;
    const target: DropTarget = { key: row.key, zone };
    const placement = placementFor(target, new Set(dragIds), collectionsStore.items);
    return placement && { ...target, placement };
  },
  onDrop: (_pressed, drop) => void collectionsStore.moveRows(dragIds, drop.placement),
});
useEventListener(document, 'pointermove', (e: PointerEvent) => {
  if (drag.dragging.value) pointer.value = { x: e.clientX, y: e.clientY };
});

function rowDragging(key: string): boolean {
  return drag.dragging.value !== null && selection.isSelected(key);
}
function rowDropZone(key: string): DropZone | null {
  return drag.drop.value?.key === key ? drag.drop.value.zone : null;
}

// The tree background has no interactive role of its own: bind its listener via VueUse.
const treeBodyEl = useTemplateRef<HTMLElement>('treeBodyEl');
useEventListener(treeBodyEl, 'contextmenu', onBackgroundContextMenu);
</script>

<template>
  <div class="h-full flex flex-col min-h-0">
    <div ref="treeBodyEl" class="flex-1 min-h-0" data-testid="tree-background">
      <div
        ref="scrollEl"
        :class="['virtual-list h-full overflow-auto', drag.dragging.value && 'select-none']"
        data-testid="virtual-list"
        role="tree"
        aria-multiselectable="true"
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
              @rename="onRename"
              @cancel-rename="collectionsStore.cancelRename"
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
              :dragging="rowDragging(collectionsStore.visibleRows[item.index].key)"
              :drop-zone="rowDropZone(collectionsStore.visibleRows[item.index].key)"
              @select="onSelect"
              @toggle="onToggle"
              @open="onOpen"
              @contextmenu="onContextMenu"
              @rename="onRename"
              @cancel-rename="collectionsStore.cancelRename"
            />
          </template>
        </div>
      </div>
    </div>
    <div
      v-if="drag.dragging.value && dragCount > 1"
      class="pointer-events-none fixed z-(--kira-z-popover) min-w-5 rounded-full bg-focus px-1.5 text-center text-kira-sm text-primary-foreground"
      :style="{ left: `${pointer.x + 12}px`, top: `${pointer.y + 12}px` }"
      :aria-label="t('studio.api.collections.dragCount.aria', { count: dragCount })"
      data-testid="collection-drag-count"
    >
      {{ dragCount }}
    </div>
  </div>
</template>
