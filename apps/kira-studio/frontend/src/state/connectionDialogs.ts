import { defineStore } from 'pinia';
import { ref } from 'vue';
import type { ConnectionSection } from '../project/connection/form';

// Which connection dialog is open. The new-connection dialog hands over to the file dialog once it
// has created the file.
export const useConnectionDialogStore = defineStore('connectionDialog', () => {
  const newOpen = ref(false);
  /** The connection file open in the file dialog, or null. */
  const file = ref<string | null>(null);
  /** The section the file dialog opens on, once; the dialog clears it. */
  const section = ref<ConnectionSection | null>(null);

  function openNew(): void {
    newOpen.value = true;
  }

  function closeNew(): void {
    newOpen.value = false;
  }

  function openFile(name: string, on: ConnectionSection | null = null): void {
    section.value = on;
    file.value = name;
  }

  function closeFile(): void {
    file.value = null;
  }

  return { newOpen, file, section, openNew, closeNew, openFile, closeFile };
});
