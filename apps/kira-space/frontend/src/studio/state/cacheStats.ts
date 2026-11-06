import type { CacheStats } from '@shared/protocol/dataOps';
import { defineStore } from 'pinia';
import { reactive, toRefs } from 'vue';
import { data } from '../bridge/data';

export const useCacheStatsStore = defineStore('cacheStats', () => {
  const state = reactive({
    // Stays null until the first event arrives — the status bar readout is hidden until then, so
    // a fresh launch never flashes a misleading "0 B / 0%" before the engine has reported anything.
    stats: null as CacheStats | null,
  });

  let unsubscribe: (() => void) | null = null;

  function initCacheStats(): void {
    if (unsubscribe) return;
    unsubscribe = data.onCacheStats((stats) => {
      state.stats = stats;
    });
  }

  return { ...toRefs(state), initCacheStats };
});
