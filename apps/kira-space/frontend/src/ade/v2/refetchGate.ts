import type { QueryClient, QueryKey } from '@tanstack/vue-query';

/**
 * Single-flight refetch of one query for a stream of push signals. Idle, a signal refetches at once;
 * while a fetch runs, signals only mark it dirty and one more fetch follows when it settles. A running
 * fetch is never cancelled (the default `invalidateQueries` restarts it, and the backend still
 * finishes the discarded call). A fetch the gate did not start (mutation settle, mount) may have read
 * its snapshot before this signal, so the gate waits it out and fetches again rather than joining it.
 */
export function createRefetchGate(queryClient: QueryClient, queryKey: QueryKey) {
  let running = false;
  let dirty = false;

  async function pump(): Promise<void> {
    running = true;
    try {
      do {
        dirty = false;
        const foreign = queryClient.getQueryCache().find({ queryKey, exact: true });
        if (foreign?.state.fetchStatus === 'fetching') {
          try {
            await foreign.promise;
          } catch {
            // Its failure is the query's to report; the gate still owes a fetch.
          }
          dirty = true;
          continue;
        }
        await queryClient.invalidateQueries({ queryKey, exact: true }, { cancelRefetch: false });
      } while (dirty);
    } finally {
      running = false;
    }
  }

  return {
    signal(): void {
      if (running) {
        dirty = true;
        return;
      }
      void pump();
    },
  };
}
