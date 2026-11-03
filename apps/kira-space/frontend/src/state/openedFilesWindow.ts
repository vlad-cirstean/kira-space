import { defineStore } from 'pinia';
import { ref } from 'vue';

// Whether this webview is the files window; set once at boot (main.ts), never changes.
export const useOpenedFilesWindowStore = defineStore('openedFilesWindow', () => {
  const active = ref(false);
  return { active };
});
