import { defineStore } from 'pinia';
import { ref } from 'vue';

// Whether this webview is the shared-config review window; set once at boot (main.ts).
export const useConfigSyncWindowStore = defineStore('configSyncWindow', () => {
  const active = ref(false);
  return { active };
});
