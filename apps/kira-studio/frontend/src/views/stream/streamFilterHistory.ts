import { newUuid } from '@shared/ids';
import { defineStore } from 'pinia';
import { reactive } from 'vue';

// "with history": mirrors views/shared/FilterHistoryMenu.vue's *shape* (a recency-ordered list,
// pinned entries exempt from the cap) but deliberately not its storage — the SQL grid's filter
// history is a real SQLite table (main/storage/repos/filter-history.ts), and adding a matching
// one here would mean touching this app's own SQLite schema, which is out of scope for this pass.
// Kafka's three positioning filters are a browse convenience, not something that warrants a new
// table for — so this stays session-only, in-memory, gone on reload/restart, same discipline as
// grid/search.ts's own per-tab `searchState`.
export interface StreamFilterHistoryEntry {
  id: string;
  offset: string | null;
  /** Task #61 widened this from a single optional partition to a multiselect — empty means
   *  "every partition", mirroring KafkaStreamFilter's own `partitions` field. */
  partitions: number[];
  timestamp: string | null;
  pinned: boolean;
  usedAt: number;
}

interface StreamFilterInput {
  offset: string | null;
  partitions: number[];
  timestamp: string | null;
}

type StreamFilterEntryLike = Pick<StreamFilterHistoryEntry, 'offset' | 'partitions' | 'timestamp'>;

const HISTORY_LIMIT = 20;

export const useStreamFilterHistoryStore = defineStore('streamFilterHistory', () => {
  // Keyed by connectionId+path (not tabId): two tabs open on the same topic/queue share one
  // history, mirroring the SQL grid's own filter_history table (keyed the same way).
  const store = reactive(new Map<string, StreamFilterHistoryEntry[]>());

  function storeKey(connectionId: string, path: string): string {
    return `${connectionId}\0${path}`;
  }

  function isEmpty(filter: StreamFilterInput): boolean {
    return filter.offset === null && filter.partitions.length === 0 && filter.timestamp === null;
  }

  function samePartitions(a: number[], b: number[]): boolean {
    if (a.length !== b.length) return false;
    const as = [...a].sort((x, y) => x - y);
    const bs = [...b].sort((x, y) => x - y);
    return as.every((v, i) => v === bs[i]);
  }

  function sameFilter(a: StreamFilterInput, b: StreamFilterEntryLike): boolean {
    return (
      a.offset === b.offset &&
      samePartitions(a.partitions, b.partitions) &&
      a.timestamp === b.timestamp
    );
  }

  /** "I cleared the filter" is not history — mirrors main/storage/repos/filter-history.ts's own
   *  early return for a fully-null (where, orderBy). */
  function recordStreamFilterUse(
    connectionId: string,
    path: string,
    filter: StreamFilterInput,
  ): void {
    if (isEmpty(filter)) return;
    const key = storeKey(connectionId, path);
    const existing = store.get(key) ?? [];
    // Re-applying the same filter moves it to the top rather than duplicating it (same rule as the
    // SQL grid's recordFilterUse).
    const prior = existing.find((e) => sameFilter(filter, e));
    const deduped = existing.filter((e) => !sameFilter(filter, e));
    // Keeps the pin: re-applying a pinned filter must not let it age out of the cap.
    deduped.unshift({
      id: newUuid(),
      ...filter,
      pinned: prior?.pinned ?? false,
      usedAt: Date.now(),
    });

    const pinned = deduped.filter((e) => e.pinned);
    const rest = deduped
      .filter((e) => !e.pinned)
      .slice(0, Math.max(0, HISTORY_LIMIT - pinned.length));
    store.set(key, [...pinned, ...rest]);
  }

  function listStreamFilterHistory(connectionId: string, path: string): StreamFilterHistoryEntry[] {
    return store.get(storeKey(connectionId, path)) ?? [];
  }

  function toggleStreamFilterHistoryPin(connectionId: string, path: string, id: string): void {
    const entry = store.get(storeKey(connectionId, path))?.find((e) => e.id === id);
    if (entry) entry.pinned = !entry.pinned;
  }

  function deleteStreamFilterHistoryEntry(connectionId: string, path: string, id: string): void {
    const key = storeKey(connectionId, path);
    const list = store.get(key);
    if (!list) return;
    store.set(
      key,
      list.filter((e) => e.id !== id),
    );
  }

  return {
    recordStreamFilterUse,
    listStreamFilterHistory,
    toggleStreamFilterHistoryPin,
    deleteStreamFilterHistoryEntry,
  };
});
