import { defineStore } from 'pinia';
import { reactive, toRefs } from 'vue';

// P6 D11: the dynamic-values reference dialog's own open/close state — mirrors
// state/fakeData.ts's fakeDataDialogState shape (P15 D11), minus the tab identity that dialog
// carries: this one is read-only and global, not scoped to any particular tab.
export interface DynamicValuesDialogState {
  open: boolean;
}

export const useDynamicValuesStore = defineStore('dynamicValues', () => {
  const state = reactive<DynamicValuesDialogState>({ open: false });

  function openDynamicValuesDialog(): void {
    state.open = true;
  }

  function closeDynamicValuesDialog(): void {
    state.open = false;
  }

  return { ...toRefs(state), openDynamicValuesDialog, closeDynamicValuesDialog };
});
