import { defineStore } from 'pinia';
import { ref } from 'vue';

// Which collection or environment file the variables dialog edits, or null when it is closed.
export const useApiVariablesDialogStore = defineStore('apiVariablesDialog', () => {
  const file = ref<string | null>(null);
  /** The tab the dialog opens on. */
  const tab = ref<'environment' | 'variables' | 'agent'>('variables');

  function open(name: string, on: 'environment' | 'variables' | 'agent' = 'variables'): void {
    file.value = name;
    tab.value = on;
  }

  function close(): void {
    file.value = null;
  }

  return { file, tab, open, close };
});
