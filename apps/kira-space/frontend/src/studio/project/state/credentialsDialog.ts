import { defineStore } from 'pinia';
import { ref } from 'vue';

// Holds only which connection the Update-credentials dialog targets. The pasted text and any
// secret stay inside the dialog's own panel and are never put here.
export const useCredentialsDialogStore = defineStore('credentialsDialog', () => {
  const open = ref(false);
  const connectionId = ref<string | null>(null);

  function openFor(id: string): void {
    connectionId.value = id;
    open.value = true;
  }

  function close(): void {
    open.value = false;
    connectionId.value = null;
  }

  return { open, connectionId, openFor, close };
});
