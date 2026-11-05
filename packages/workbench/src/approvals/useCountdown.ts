import { useIntervalFn } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { computed, ref } from 'vue';

/** mm:ss. */
export function formatClock(totalSeconds: number): string {
  const m = Math.floor(totalSeconds / 60);
  const s = totalSeconds % 60;
  return `${m}:${String(s).padStart(2, '0')}`;
}

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
