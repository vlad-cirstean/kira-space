import { defineStore } from 'pinia';
import { reactive, toRefs } from 'vue';

// Mirrors state/objectStore.ts's uploadDialogState shape — a tiny reactive open/close
// flag plus the one identity the dialog needs (the tab), so project/menus.ts-style callers (and
// the command-palette entry) never have to import a views/ module. Every
// other fact the dialog needs — the tab's connection/path, its page's columns, its meta — is read
// live from the tab itself once open, not copied in here.
export interface FakeDataDialogState {
  open: boolean;
  tabId: string | null;
}

export const useFakeDataStore = defineStore('fakeData', () => {
  const state = reactive<FakeDataDialogState>({ open: false, tabId: null });

  function openGenerateDataDialog(tabId: string): void {
    state.tabId = tabId;
    state.open = true;
  }

  function closeGenerateDataDialog(): void {
    state.open = false;
  }

  return { ...toRefs(state), openGenerateDataDialog, closeGenerateDataDialog };
});
