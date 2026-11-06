import { useIntervalFn } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { defineStore } from 'pinia';
import { type ComputedRef, computed, ref, watchEffect } from 'vue';
import { useOpsStore } from './ops';
import { pinia } from './pinia';

export interface RunStateVm {
  status: 'idle' | 'running' | 'error';
  elapsedMs: number | null;
}

const IDLE: RunStateVm = { status: 'idle', elapsedMs: null };

// One shared ticker for the whole app instead of a per-view setInterval: started only while at
// least one op is running, stopped otherwise, so RunState.vue's ring/elapsed-time never leaks a
// timer when a view unmounts mid-run (tests/e2e/leaks.spec.ts).
const useRunStateStore = defineStore('runState', () => {
  const now = ref(Date.now());
  const ticker = useIntervalFn(
    () => {
      now.value = Date.now();
    },
    TIMING.runTick,
    { immediate: false },
  );

  const opsStore = useOpsStore();

  watchEffect(() => {
    const running = opsStore.records.some((r) => r.status === 'running');
    if (running && !ticker.isActive.value) {
      now.value = Date.now();
      ticker.resume();
    } else if (!running && ticker.isActive.value) {
      ticker.pause();
    }
  });

  // A ring + elapsed time in the toolbar that started the work — never a bar across the view.
  // Idle keeps the last op's duration in the same slot instead of blanking it, so the toolbar
  // never reflows when the next run starts. Driven by the most recent op for this tab (opsStore is
  // already live-streamed via control.onOpUpdate).
  function runStateFor(tabId: string): RunStateVm {
    // opsStore.records is newest-started-first (state/ops.ts), so a plain
    // `.find` picks whichever of two concurrent ops on the same tab started last — a fast op
    // (e.g. a page read) finishing first then reads as "idle" while a slower sibling (e.g. Σ) is
    // still running, and the toolbar's ring goes dark mid-query. Preferring a running record over
    // any newer finished one answers the question the ring asks: is *anything* for this
    // tab still waiting on the server. Falls back to the newest record when none is running, so
    // the idle slot's duration is unchanged from before.
    const record =
      opsStore.records.find((r) => r.tabId === tabId && r.status === 'running') ??
      opsStore.records.find((r) => r.tabId === tabId);
    if (!record) return IDLE;
    if (record.status === 'running') {
      return { status: 'running', elapsedMs: now.value - new Date(record.startedAt).getTime() };
    }
    if (record.status === 'error') return { status: 'error', elapsedMs: record.durationMs };
    return { status: 'idle', elapsedMs: record.durationMs };
  }

  return { runStateFor };
});

// Module-level call below runs at import time, before `app.use(pinia)` — the explicit instance is
// required here (state/pinia.ts's own header comment) so the ticker's watchEffect starts as soon
// as this module loads, exactly as it did before this became a store.
useRunStateStore(pinia);

export function useRunState(tabId: () => string | null | undefined): ComputedRef<RunStateVm> {
  return computed(() => {
    const id = tabId();
    return id ? useRunStateStore().runStateFor(id) : IDLE;
  });
}
