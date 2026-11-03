import { defineStore } from 'pinia';
import { ref } from 'vue';

// Which connection dialog is open. The new-connection dialog hands over to the file dialog once it
// has created the file.
export const useConnectionDialogStore = defineStore('connectionDialog', () => {
  const newOpen = ref(false);
  /** The connection file open in the file dialog, or null. */
  const file = ref<string | null>(null);

  function openNew(): void {
    newOpen.value = true;
  }

  function closeNew(): void {
    newOpen.value = false;
  }

  function openFile(name: string): void {
    file.value = name;
  }

  function closeFile(): void {
    file.value = null;
  }

  return { newOpen, file, openNew, closeNew, openFile, closeFile };
});
