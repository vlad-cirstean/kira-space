<script setup lang="ts">
import { t } from '@theme/i18n';
import { useDebounceFn, useEventListener } from '@vueuse/core';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { TIMING } from '@workbench/behaviour/timing';
import { useTreeVirtualRows } from '@workbench/behaviour/treeVirtualRows';
import { STICKY_ROW_CLASS, VIRTUAL_ROW_CLASS } from '@workbench/behaviour/virtualRows';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, onMounted, onUnmounted, ref, useTemplateRef, watch } from 'vue';
import { openRepoFileTab } from '../state/repoTabs';
import { menuForRepoRow } from './menu';
import RepoTreeRow from './RepoTreeRow.vue';
import { type RepoTreeRowVm, useFileTreeStore } from './state/fileTree';

const contextMenuStore = useContextMenuStore();
const fileTreeStore = useFileTreeStore();

const props = defineProps<{ repoId: string; search: string }>();

const { single: rowHeight } = useRowHeight();
const selected = ref<string | null>(null);

onMounted(() => fileTreeStore.ensureRepoTreeLoaded(props.repoId));
watch(
  () => props.repoId,
  (id) => fileTreeStore.ensureRepoTreeLoaded(id),
);

// project/state/tree.ts's own debounce for the Studio schema tree's identical shape of
// problem — the input itself (PanelShell's own model-value, bound above props.search) stays
// instantly responsive, only the expensive filter recompute (a full tree walk) waits for
// typing to settle. 150ms matches that precedent. Measured (200k files, the cap): 538ms per
// keystroke reactive vs 200ms plain filter cost — debouncing turns "every keystroke pays this"
// into "one pause pays this", which is what fixes the perceived lag.
const debouncedSearch = ref(props.search);
// SchemaDialog.vue's own useDebounceFn precedent (Part 2) — cancel() in onUnmounted/the repoId
// watch replaces the clearTimeout below.
const applySearchDebounced = useDebounceFn((value: string) => {
  debouncedSearch.value = value;
}, TIMING.search);
watch(
  () => props.search,
  (value) => {
    void applySearchDebounced(value);
  },
);
// A workspace/repo switch must not show the new repo's tree filtered by a stale debounce timer
// still counting down from the previous one.
watch(
  () => props.repoId,
  () => {
    applySearchDebounced.cancel();
    debouncedSearch.value = props.search;
  },
);
onUnmounted(() => applySearchDebounced.cancel());

const rows = computed(() => fileTreeStore.visibleRepoRows(props.repoId, debouncedSearch.value));

const scrollEl = ref<HTMLElement | null>(null);
const { virtualItems, totalSize, band, onScroll, revealKey } = useTreeVirtualRows({
  rows: () => rows.value,
  rowHeight: () => rowHeight.value,
  scrollElement: scrollEl,
  keepScroll: () => `git:files:${props.repoId}`,
});

function onSelect(row: RepoTreeRowVm): void {
  selected.value = row.key;
}

function onToggle(row: RepoTreeRowVm): void {
  fileTreeStore.toggleRepoDir(props.repoId, row.path);
}

// §7.2: single click (row's own onClick) opens a preview tab, double click/Enter a permanent one
// — RepoTreeRow.vue emits `open` with the `preview` flag already resolved.
function onOpen(row: RepoTreeRowVm, preview: boolean): void {
  if (row.isDir) return;
  void openRepoFileTab(props.repoId, row.path, { preview });
}

// Right-click selects the row (without opening anything) before its menu opens.
function onContextMenu(row: RepoTreeRowVm, event: MouseEvent): void {
  selected.value = row.key;
  contextMenuStore.openContextMenu(event, menuForRepoRow(props.repoId, row), {
    onRestoreFocus: () => void nav.focusKey(row.key),
  });
}

// Arrows, Home/End, Right/Left expand and parent, Enter (permanent open), Space (select, toggle or
// preview) and Shift+F10 come from the shared list navigation; the selection is the roving stop.
const nav = useListNavigation<RepoTreeRowVm>({
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
    else onOpen(row, false);
  },
  onSelect: (row) => {
    onSelect(row);
    if (row.isDir) onToggle(row);
    else onOpen(row, true);
  },
  onMenu: (row, rect) =>
    contextMenuStore.openContextMenuAt(rect.left, rect.bottom, menuForRepoRow(props.repoId, row), {
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
  <div ref="treeBodyEl" class="h-full" data-testid="tree-background">
    <div
      ref="scrollEl"
      class="virtual-list h-full overflow-auto"
      data-testid="virtual-list"
      role="tree"
      :aria-label="t('space.repo.tree.files.aria')"
      @scroll="onScroll"
    >
      <div class="sticky top-0 z-2 h-0" data-testid="tree-sticky-band">
        <template v-for="slot in band" :key="slot.row.key">
          <RepoTreeRow
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
          <RepoTreeRow
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
  <!-- P110 I2-15: `.sticky-row`/`.virtual-row` moved to VIRTUAL_ROW_CLASS/STICKY_ROW_CLASS
       (packages/workbench/src/lib/virtualRows.ts) -- see kira-studio's ProjectTree.vue's
       identical note. -->
</template>
