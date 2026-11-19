<script setup lang="ts" generic="T extends { key: string }">
import { type ListNavigationOptions, useListNavigation } from '@workbench/behaviour/listNavigation';
import { useVirtualRows, VIRTUAL_ROW_CLASS } from '@workbench/behaviour/virtualRows';
import { useRowHeight } from '@workbench/lib/rowHeight';
import type { MenuItem } from '@workbench/state/contextMenu';
import { computed, ref, watch } from 'vue';

const props = defineProps<{
  rows: readonly T[];
  rowHeight?: number;
  rowHeights?: readonly number[];
  testid: string;
  /** Enter on a row. */
  open?: (row: T) => void;
  /** ContextMenu key and Shift+F10 on a row. */
  menu?: (row: T, rect: DOMRect) => void;
  /** Click, Space and right-click select the row. */
  select?: (row: T) => void;
  /** Right-click menu of a row; none (or empty) leaves the browser menu alone. */
  menuFor?: (row: T) => MenuItem[];
  /** Group rows that expand and collapse (Right and Left). */
  tree?: ListNavigationOptions<T>['tree'];
  /** View-state key: list scroll survives an unmount. */
  keepScrollKey?: string;
}>();

defineSlots<{
  row(props: {
    row: T;
    nav: {
      'data-nav-key': string;
      tabindex: 0 | -1;
      onFocus: () => void;
      onContextmenu: (e: MouseEvent) => void;
    };
  }): unknown;
}>();

const { single: defaultRowHeight } = useRowHeight();
const scrollEl = ref<HTMLElement | null>(null);
// Bound by a host that tracks the active row itself; local when unbound.
const activeKey = defineModel<string | null>('activeKey', { default: null });
const activeIndex = computed(() => props.rows.findIndex((r) => r.key === activeKey.value));

const { virtualItems, totalSize, onScroll, scrollToIndex } = useVirtualRows({
  count: () => props.rows.length,
  rowHeight: () => props.rowHeight ?? defaultRowHeight.value,
  rowHeights: () => props.rowHeights,
  scrollElement: scrollEl,
  keepScroll: () => props.keepScrollKey ?? null,
  pinned: () => (activeIndex.value < 0 ? null : activeIndex.value),
});

const nav = useListNavigation<T>({
  container: scrollEl,
  items: () => props.rows,
  key: (row) => row.key,
  activeKey,
  onSelect: (row) => props.select?.(row),
  onOpen: (row) => props.open?.(row),
  onMenu: (row, rect) => props.menu?.(row, rect),
  tree: props.tree && {
    depth: (row) => props.tree?.depth(row) ?? 0,
    isExpanded: (row) => props.tree?.isExpanded(row) ?? false,
    hasChildren: (row) => props.tree?.hasChildren(row) ?? false,
    expand: (row) => props.tree?.expand(row),
    collapse: (row) => props.tree?.collapse(row),
  },
  scrollToIndex: (index) => scrollToIndex(index),
  rendered: () => virtualItems.value,
});

// Stable handlers per row key let rows skip re-render when the virtual range changes.
const handlers = new Map<string, { onFocus: () => void; onContextmenu: (e: MouseEvent) => void }>();
watch(
  () => props.rows,
  () => handlers.clear(),
);

function rowNav(row: T) {
  let h = handlers.get(row.key);
  if (!h) {
    h = {
      onFocus: nav.rowProps(row.key).onFocus,
      onContextmenu: (e: MouseEvent) => {
        const items = props.menuFor?.(row) ?? [];
        if (items.length === 0) return;
        e.preventDefault();
        nav.contextMenu(row, e, items);
      },
    };
    handlers.set(row.key, h);
  }
  return { 'data-nav-key': row.key, tabindex: nav.tabindexFor(row.key), ...h };
}
</script>

<template>
  <div
    ref="scrollEl"
    role="listbox"
    class="min-h-0 flex-1 overflow-auto"
    :data-testid="testid"
    @scroll="onScroll"
  >
    <div class="relative" :style="{ height: `${totalSize}px` }">
      <div
        v-for="vi in virtualItems"
        :key="rows[vi.index]!.key"
        :class="VIRTUAL_ROW_CLASS"
        :style="{ height: `${vi.size}px`, transform: `translateY(${vi.start}px)` }"
      >
        <slot name="row" :row="rows[vi.index]!" :nav="rowNav(rows[vi.index]!)" />
      </div>
    </div>
  </div>
</template>
