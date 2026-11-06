import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';
import { defineStore } from 'pinia';
import { reactive } from 'vue';
import { tabNeedsReconnect } from '../views/shared/useConnectionGate';
import { useConnectionsStore } from './connections';

export const useStaleDataStore = defineStore('staleData', () => {
  // dataKey -> connection session (`ConnectionState.since`) the shown data was read under.
  const sessions = reactive<Record<string, number | null>>({});

  function stamp(dataKey: string, connectionId: string): void {
    sessions[dataKey] = useConnectionsStore().states[connectionId]?.since ?? null;
  }

  /** Stale: the gate needs a reconnect, or the data came from another connection session. */
  function isStale(dataKey: string, gateTabId: string, connectionId: string): boolean {
    if (tabNeedsReconnect(gateTabId, connectionId)) return true;
    const stamped = sessions[dataKey];
    if (stamped === undefined || stamped === null) return false;
    return stamped !== useConnectionsStore().states[connectionId]?.since;
  }

  registerTabRuntimeCleanup((id) => {
    for (const key of Object.keys(sessions)) {
      if (key === id || key.startsWith(`${id}::`)) delete sessions[key];
    }
  });

  return { stamp, isStale };
});
