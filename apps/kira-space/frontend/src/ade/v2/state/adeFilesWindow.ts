import { defineStore } from 'pinia';
import { ref } from 'vue';

// Which task this window's Files view shows; set once at boot (main.ts). A window with a task is a
// task Files window.
export const useAdeFilesWindowStore = defineStore('adeFilesWindow', () => {
  const taskId = ref<string | null>(null);
  return { taskId };
});
