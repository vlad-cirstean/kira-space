import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';
import { defineStore } from 'pinia';
import { reactive } from 'vue';

// Per-tab console UI state that a tab switch (unmount) must not lose. Session only, never
// persisted (P375).
export interface ConsoleUiState {
  cursor: number;
  scrollTop: number;
  /** Discover field sidebar. */
  fieldSearch: string;
  fieldCap: number | null;
}

export const useConsoleUiStore = defineStore('consoleUi', () => {
  const byTab = reactive({} as Record<string, ConsoleUiState>);

  registerTabRuntimeCleanup((tabId) => {
    delete byTab[tabId];
  });

  function ui(tabId: string): ConsoleUiState {
    if (!byTab[tabId]) byTab[tabId] = { cursor: 0, scrollTop: 0, fieldSearch: '', fieldCap: null };
    return byTab[tabId];
  }

  return { ui };
});
