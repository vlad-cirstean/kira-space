import { useIntervalFn } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { computed, ref } from 'vue';

/** Seconds left until `expiresAt` (ms), or null when it never expires. The backend decides expiry. */
export function useCountdown(expiresAt: () => number | null) {
  const now = ref(Date.now());
  useIntervalFn(() => {
    now.value = Date.now();
  }, TIMING.clockTick);
  return computed(() => {
    const at = expiresAt();
    if (!at) return null;
    return Math.max(0, Math.ceil((at - now.value) / 1000));
  });
}
