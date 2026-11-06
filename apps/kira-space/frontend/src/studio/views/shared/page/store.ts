import { reactive } from 'vue';

// One factory behind all five page modules: a two-level decode cache (row -> subKey -> text) and
// visible-window pruning. A module that never calls setVisibleWindow never prunes.
interface Entry<P> {
  page: P;
  decodeCache: Map<number, Map<string, string>>;
  viewCache: Map<number, Map<string, unknown>>;
  windowStart: number;
  windowEnd: number;
}

export interface PageStore<P extends { rowCount: number; byteSize: number }> {
  readonly pageVersion: { n: number };
  /** state.ts's setActiveResult (P40 D9) raises "the page this scope resolves to changed"
   *  without a page of its own to setPage/drop. */
  bumpPageVersion(): void;
  setPage(scope: string, page: P): void;
  getPage(scope: string): P | null;
  drop(scope: string): void;
  /** console/resultPages.ts's own dropForTab: every scope equal to, or prefixed by, `${prefix}:`. */
  dropForPrefix(prefix: string): void;
  totalRetainedBytes(): number;
  /** row -> subKey -> decoded text, memoized. `subKey` is what was decoded *within* a row: a
   *  tabular cell's column index (as a string), or one of 'id'/'body'/'field'/'value'/etc. */
  cached(
    scope: string,
    row: number,
    subKey: string,
    decode: (decoder: TextDecoder) => string,
  ): string;
  /** Every page/*.ts row/cell accessor built above `cached`'s memoized text
   *  (grid/page.ts's `cell()`, console/resultPages.ts's `cell()`/`documentRow()`/`keyValueRow()`,
   *  documents/page.ts's `documentRow()`, keyvalue/page.ts's `keyValueRow()`, stream/page.ts's
   *  `streamRow()`) still allocated a fresh return object on *every* call, even though the decoded
   *  text underneath was already cached — and some of those accessors (`cellAt` in the deleted
   *  DataGrid.vue/ConsoleResultGrid.vue in particular) get called several times per cell per
   *  render straight from a template. This memoizes the accessor's own built object the same way
   *  `cached` memoizes decoded text, so a repeat call for the same row/subKey returns the identical
   *  object reference instead of a new one. Shares `cached`'s row-level invalidation story
   *  (setPage/drop/setVisibleWindow) via its own parallel per-row map. */
  cachedView<V>(scope: string, row: number, subKey: string, build: () => V): V;
  /** Prunes the decode cache to the visible row window (P29 D7) instead of clearing it outright,
   *  so a fling doesn't re-decode a window that mostly overlaps the last one. A no-op for a scope
   *  that never calls this — the cache simply never prunes. */
  setVisibleWindow(scope: string, startRow: number, endRow: number): void;
}

/** One page store's internals, read-only — what `retentionEntries` below reports for a
 *  single scope (a tab id, or a console result key). `decodeCacheChars` sums the decoded string
 *  lengths `cached()` has memoized; `page` is handed back whole so a caller that knows its own
 *  concrete page kind (a page module, or main.ts's `__kiraRetention`) can read `byteSize` or walk
 *  its chunks without this generic module needing to know either. */
export interface RetentionEntry<P> {
  page: P;
  decodeCacheRows: number;
  decodeCacheChars: number;
  viewCacheRows: number;
}

// Keyed by the store object itself, not exposed on `PageStore<P>` — the interface every
// view module already imports `cached`/`cachedView`/`setVisibleWindow` through stays exactly as
// wide as it always was, so ordinary view code reaching for `store.<something>` can never stumble
// onto a probe-only accessor. Only `retentionEntries` below (and, transitively, each page module's
// own one-line re-export of it) ever looks this up.
const retentionSources = new WeakMap<object, () => RetentionEntry<unknown>[]>();

/** Playwright-only (main.ts's `window.__kiraRetention`, C1): a page store's per-scope internals —
 *  the decode/view caches `totalRetainedBytes()` cannot see, since that sums `page.byteSize` only
 *  (F2's own finding: the app's retention accounting is blind to the largest thing it retains). */
export function retentionEntries<P extends { rowCount: number; byteSize: number }>(
  store: PageStore<P>,
): RetentionEntry<P>[] {
  const source = retentionSources.get(store as object);
  return (source?.() ?? []) as RetentionEntry<P>[];
}

export function createPageStore<P extends { rowCount: number; byteSize: number }>(opts?: {
  onSet?(scope: string): void;
}): PageStore<P> {
  const pages = new Map<string, Entry<P>>();
  const decoder = new TextDecoder();
  const pageVersion = reactive({ n: 0 });

  const store: PageStore<P> = {
    pageVersion,

    bumpPageVersion() {
      pageVersion.n++;
    },

    setPage(scope, page) {
      // A tripwire: any code that tries to mutate this fails loudly in dev instead of silently
      // diverging from `byteSize`.
      Object.freeze(page);
      pages.set(scope, {
        page,
        decodeCache: new Map(),
        viewCache: new Map(),
        windowStart: 0,
        windowEnd: 0,
      });
      pageVersion.n++;
      opts?.onSet?.(scope);
    },

    getPage(scope) {
      return pages.get(scope)?.page ?? null;
    },

    drop(scope) {
      if (pages.delete(scope)) pageVersion.n++;
    },

    dropForPrefix(prefix) {
      let changed = false;
      const withColon = `${prefix}:`;
      for (const scope of pages.keys()) {
        if (scope === prefix || scope.startsWith(withColon)) {
          pages.delete(scope);
          changed = true;
        }
      }
      if (changed) pageVersion.n++;
    },

    totalRetainedBytes() {
      let total = 0;
      for (const entry of pages.values()) total += entry.page.byteSize;
      return total;
    },

    cached(scope, row, subKey, decode) {
      const entry = pages.get(scope);
      if (!entry) return decode(decoder);
      let rowCache = entry.decodeCache.get(row);
      if (!rowCache) {
        rowCache = new Map();
        entry.decodeCache.set(row, rowCache);
      }
      let value = rowCache.get(subKey);
      if (value === undefined) {
        value = decode(decoder);
        rowCache.set(subKey, value);
      }
      return value;
    },

    cachedView<V>(scope: string, row: number, subKey: string, build: () => V): V {
      const entry = pages.get(scope);
      if (!entry) return build();
      let rowCache = entry.viewCache.get(row);
      if (!rowCache) {
        rowCache = new Map();
        entry.viewCache.set(row, rowCache);
      }
      if (rowCache.has(subKey)) return rowCache.get(subKey) as V;
      const value = build();
      rowCache.set(subKey, value);
      return value;
    },

    setVisibleWindow(scope, startRow, endRow) {
      const entry = pages.get(scope);
      if (!entry) return;
      if (entry.windowStart === startRow && entry.windowEnd === endRow) return;
      for (const row of entry.decodeCache.keys()) {
        if (row < startRow || row >= endRow) entry.decodeCache.delete(row);
      }
      for (const row of entry.viewCache.keys()) {
        if (row < startRow || row >= endRow) entry.viewCache.delete(row);
      }
      entry.windowStart = startRow;
      entry.windowEnd = endRow;
    },
  };

  retentionSources.set(store, () => {
    const out: RetentionEntry<P>[] = [];
    for (const entry of pages.values()) {
      let decodeCacheChars = 0;
      for (const rowCache of entry.decodeCache.values()) {
        for (const text of rowCache.values()) decodeCacheChars += text.length;
      }
      out.push({
        page: entry.page,
        decodeCacheRows: entry.decodeCache.size,
        decodeCacheChars,
        viewCacheRows: entry.viewCache.size,
      });
    }
    return out as RetentionEntry<unknown>[];
  });

  return store;
}
