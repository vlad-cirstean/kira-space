import type { QueryClient, QueryKey } from '@tanstack/vue-query';

/**
 * Single-flight refetch of one query for a stream of push signals. Idle, a signal refetches at once;
 * while a fetch runs, signals only mark it dirty and one more fetch follows when it settles. A running
 * fetch is never cancelled (the default `invalidateQueries` restarts it, and the backend still
 * finishes the discarded call).
 */
export function createRefetchGate(queryClient: QueryClient, queryKey: QueryKey) {
  let running = false;
  let dirty = false;

  async function pump(): Promise<void> {
    running = true;
    try {
      do {
        dirty = false;
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
