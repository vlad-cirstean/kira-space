import { defineStore } from 'pinia';
import { ref } from 'vue';

// Which tree row is selected: the roving stop that single-target actions and the Variables…
// palette command read.
export const useCollectionSelectionStore = defineStore('collectionSelection', () => {
  const focusKey = ref<string | null>(null);

  function isSelected(key: string): boolean {
    return focusKey.value === key;
  }

  function selectOnly(key: string): void {
    focusKey.value = key;
  }

  /** The selected row left the visible list (collapse, search, a remote change). */
  function drop(gone: ReadonlySet<string>): void {
    if (focusKey.value !== null && gone.has(focusKey.value)) focusKey.value = null;
  }

  return { focusKey, isSelected, selectOnly, drop };
});
