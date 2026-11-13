import { cleanupTabRuntime } from '@workbench/state/tabRuntime';
import { defineStore } from 'pinia';
import { reactive, watch } from 'vue';
import { useTerminalsStore } from '../../../state/terminals';
import { useSessions } from '../queries';

const sameSet = (a: Set<string>, b: Set<string>): boolean =>
  a.size === b.size && [...a].every((x) => b.has(x));
const sameIds = (
  a: { id: string; local?: string }[],
  b: { id: string; local?: string }[],
): boolean =>
  a.length === b.length && a.every((x, i) => x.id === b[i]?.id && x.local === b[i]?.local);

// Reaps the terminals this window opened for ade sessions. Nothing else closes them, so a stopped
// session would keep its terminal entry, drain queue and xterm instance for the window's life.
export const useAdeTerminalsStore = defineStore('adeTerminals', () => {
  const terminalsStore = useTerminalsStore();
  const sessions = useSessions();
  const tracked = reactive(new Set<string>());

  /** Every terminal id this window opens for an ade session. */
  function track(terminalId: string): void {
    tracked.add(terminalId);
  }

  // A stopped session reports `terminalId: ''`, so "no running session holds this id" is the reap
  // signal; a terminal still starting a moment before its first sessions refetch is left alone.
  watch(
    () => ({
      running: new Set(
        (sessions.data.value?.sessions ?? [])
          .filter((s) => s.state === 'running')
          .map((s) => s.terminalId),
      ),
      // Read here so a local exit after the sessions push re-runs the reap.
      ids: [...tracked].map((id) => ({ id, local: terminalsStore.terminalSession(id)?.status })),
    }),
    ({ running, ids }, prev) => {
      // A push that changes neither the running set nor a tracked terminal's status has nothing to reap.
      if (prev && sameSet(running, prev.running) && sameIds(ids, prev.ids)) return;
      for (const { id, local } of ids) {
        if (running.has(id)) continue;
        if (local && local !== 'exited' && local !== 'failed') continue;
        terminalsStore.closeTerminalSession(id);
        cleanupTabRuntime(id);
        tracked.delete(id);
      }
    },
  );

  return { track };
});
