<script setup lang="ts">
import { t } from '@theme/i18n';
import { useDebounceFn, useEventListener } from '@vueuse/core';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { TIMING } from '@workbench/behaviour/timing';
import { useTreeVirtualRows } from '@workbench/behaviour/treeVirtualRows';
import { STICKY_ROW_CLASS, VIRTUAL_ROW_CLASS } from '@workbench/behaviour/virtualRows';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, onUnmounted, ref, useTemplateRef, watch } from 'vue';
import AdeFileTreeRow from './AdeFileTreeRow.vue';
import { buildTree, type FileTreeRowVm, flatten } from './fileTreeModel';
import { menuForFilePath } from './menu';
import { useAdeFilesUiStore } from './state/filesUi';
import { useTaskFilesListing } from './useTaskFiles';

const contextMenuStore = useContextMenuStore();
const filesUi = useAdeFilesUiStore();

const props = defineProps<{ branchId: string; worktree: string; filter: string }>();
const emit = defineEmits<{ open: [path: string] }>();

const { single: rowHeight } = useRowHeight();
const selected = computed({
  get: () => filesUi.selected[props.branchId] ?? null,
  set: (v: string | null) => {
    filesUi.selected[props.branchId] = v;
  },
});

const listing = useTaskFilesListing(() => props.branchId);
const tree = computed(() => buildTree(listing.data.value?.paths ?? []));
// First load of a branch opens the root's own directories; a later Refresh never re-expands
// whatever the user has since collapsed.
watch(
  tree,
  (nodes) =>
    filesUi.initExpanded(
      props.branchId,
      nodes.filter((n) => n.isDir).map((n) => n.path),
    ),
  { immediate: true },
);

// The filter field stays instantly responsive; only the filter fold (a full tree walk, 538ms per
// keystroke at the 200k-file cap) waits for typing to settle.
const debouncedFilter = ref(props.filter);
const applyFilterDebounced = useDebounceFn((value: string) => {
  debouncedFilter.value = value;
}, TIMING.search);
watch(
  () => props.filter,
  (value) => {
    void applyFilterDebounced(value);
  },
);
// A branch switch must not show the new tree filtered by a stale timer from the previous one.
watch(
  () => props.branchId,
  () => {
    applyFilterDebounced.cancel();
    debouncedFilter.value = props.filter;
  },
);
onUnmounted(() => applyFilterDebounced.cancel());

const rows = computed(() => {
  const out: FileTreeRowVm[] = [];
  flatten(
    tree.value,
    0,
    filesUi.expandedOf(props.branchId) ?? new Set<string>(),
    listing.data.value?.status ?? {},
    debouncedFilter.value.trim().toLowerCase(),
    out,
  );
  return out;
});

const scrollEl = ref<HTMLElement | null>(null);
const { virtualItems, totalSize, band, onScroll, revealKey } = useTreeVirtualRows({
  rows: () => rows.value,
  rowHeight: () => rowHeight.value,
  scrollElement: scrollEl,
  keepScroll: () => `ade:files:${props.branchId}`,
});

function onSelect(row: FileTreeRowVm): void {
  selected.value = row.key;
}

function onToggle(row: FileTreeRowVm): void {
  filesUi.toggleDir(props.branchId, row.path);
}

// A row click opens the file in the viewer; the row emits `open` with its preview flag, which a
// single viewer has no use for.
function onOpen(row: FileTreeRowVm): void {
  if (row.isDir) return;
  emit('open', row.path);
}

function menuFor(row: FileTreeRowVm) {
  return menuForFilePath({
    path: row.path,
    worktree: props.worktree,
    isDir: row.isDir,
    expanded: row.expanded,
    onToggle: () => onToggle(row),
  });
}

// Right-click selects the row (without opening anything) before its menu opens.
function onContextMenu(row: FileTreeRowVm, event: MouseEvent): void {
  selected.value = row.key;
  contextMenuStore.openContextMenu(event, menuFor(row), {
    onRestoreFocus: () => void nav.focusKey(row.key),
  });
}

// Arrows, Home/End, Right/Left expand and parent, Enter (permanent open), Space (select, toggle or
// preview) and Shift+F10 come from the shared list navigation; the selection is the roving stop.
const nav = useListNavigation<FileTreeRowVm>({
  container: scrollEl,
  items: () => rows.value,
  key: (row) => row.key,
  activeKey: selected,
  tree: {
    depth: (row) => row.depth,
    isExpanded: (row) => row.expanded,
    hasChildren: (row) => row.hasChildren,
    expand: onToggle,
    collapse: onToggle,
  },
  onOpen: (row) => {
    if (row.isDir) onToggle(row);
    else onOpen(row);
  },
  onSelect: (row) => {
    onSelect(row);
    if (row.isDir) onToggle(row);
    else onOpen(row);
  },
  onMenu: (row, rect) =>
    contextMenuStore.openContextMenuAt(rect.left, rect.bottom, menuFor(row), {
      onRestoreFocus: () => void nav.focusKey(row.key),
    }),
  scrollToIndex: (_index, key) => revealKey(key),
  rendered: () => virtualItems.value,
  rowSelector: () => '[role="treeitem"][tabindex="0"]',
});

// P105 §5.1: suppresses the browser context menu over the empty tree background (rows stop
// propagation on their own contextmenu handler) -- the tree background has no interactive role.
const treeBodyEl = useTemplateRef<HTMLElement>('treeBodyEl');
useEventListener(treeBodyEl, 'contextmenu', (e) => e.preventDefault());
</script>

<template>
  <div ref="treeBodyEl" class="h-full" data-testid="ade-files-tree">
    <div
      ref="scrollEl"
      class="virtual-list h-full overflow-auto"
      data-testid="ade-files-tree-list"
      role="tree"
      :aria-label="t('space.ade.files.tree.aria')"
      @scroll="onScroll"
    >
      <div class="sticky top-0 z-2 h-0" data-testid="tree-sticky-band">
        <template v-for="slot in band" :key="slot.row.key">
          <AdeFileTreeRow
            :class="STICKY_ROW_CLASS"
            :style="{ top: `${slot.top}px`, height: `${rowHeight}px` }"
            :row="slot.row"
            :selected="selected === slot.row.key"
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
          <AdeFileTreeRow
            :class="VIRTUAL_ROW_CLASS"
            :style="{ transform: `translateY(${item.start}px)`, height: `${item.size}px` }"
            :row="rows[item.index]"
            :selected="selected === rows[item.index].key"
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
</template>
