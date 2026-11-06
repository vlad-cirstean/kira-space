import { defineStore } from 'pinia';
import { computed, ref, shallowRef } from 'vue';

// Which tree rows are selected. The store never knows row order: callers pass the visible key
// order, so a range or select-all can only ever cover rows the user can see.
export const useCollectionSelectionStore = defineStore('collectionSelection', () => {
  /** The roving stop: single-target actions and the Variables… palette command read it. */
  const focusKey = ref<string | null>(null);
  /** Where a Shift range starts. */
  const anchorKey = ref<string | null>(null);
  const keys = shallowRef<ReadonlySet<string>>(new Set());
  const count = computed(() => keys.value.size);

  function isSelected(key: string): boolean {
    return keys.value.has(key);
  }

  function selectOnly(key: string): void {
    focusKey.value = key;
    anchorKey.value = key;
    keys.value = new Set([key]);
  }

  function toggle(key: string): void {
    const next = new Set(keys.value);
    anchorKey.value = key;
    if (next.delete(key)) {
      keys.value = next;
      if (focusKey.value === key) focusKey.value = [...next].at(-1) ?? null;
      return;
    }
    next.add(key);
    keys.value = next;
    focusKey.value = key;
  }

  /** Shift: the range anchor..key in `order`. `additive` keeps the keys already selected. */
  function extendTo(key: string, order: readonly string[], additive: boolean): void {
    const from = order.indexOf(anchorKey.value ?? focusKey.value ?? key);
    const to = order.indexOf(key);
    if (from < 0 || to < 0) {
      selectOnly(key);
      return;
    }
    const next = new Set(additive ? keys.value : []);
    for (const k of order.slice(Math.min(from, to), Math.max(from, to) + 1)) next.add(k);
    keys.value = next;
    focusKey.value = key;
    anchorKey.value ??= key;
  }

  function selectAll(order: readonly string[]): void {
    if (order.length === 0) return;
    keys.value = new Set(order);
    if (focusKey.value === null || !keys.value.has(focusKey.value)) focusKey.value = order[0];
    anchorKey.value ??= focusKey.value;
  }

  function selectMany(next: readonly string[], focus: string): void {
    keys.value = new Set(next);
    focusKey.value = focus;
    anchorKey.value = focus;
  }

  /** Escape: drop the extra rows, keep the focused one. */
  function collapseToFocus(): void {
    if (focusKey.value !== null) selectOnly(focusKey.value);
  }

  /** Rows that left the visible list (collapse, search, delete, a remote change) leave the set. */
  function drop(gone: ReadonlySet<string>): void {
    if (gone.size === 0) return;
    const next = new Set([...keys.value].filter((k) => !gone.has(k)));
    if (next.size !== keys.value.size) keys.value = next;
    if (anchorKey.value !== null && gone.has(anchorKey.value)) anchorKey.value = null;
    if (focusKey.value !== null && gone.has(focusKey.value)) {
      focusKey.value = [...next].at(0) ?? null;
    }
    anchorKey.value ??= focusKey.value;
  }

  return {
    focusKey,
    anchorKey,
    keys,
    count,
    isSelected,
    selectOnly,
    toggle,
    extendTo,
    selectAll,
    selectMany,
    collapseToFocus,
    drop,
  };
});
