// Per-scope lifecycle of the `doc.pageStats` worker job: line counts for the row-height model and
// the page's field names, computed off the main thread so a 10,000-document page costs the
// renderer only the rows in its window.

import type { DocumentPage } from '@shared/protocol/page';
import { compareNatural } from '@workbench/behaviour/match';
import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';
import { defineStore } from 'pinia';
import { shallowReactive } from 'vue';
import { parseClient } from '../../../workers/parse/instance';
import { useDocumentRowsStore } from './rows';

interface Request {
  page: DocumentPage;
  ctrl: AbortController;
}

// A worker message clones the whole buffer behind a typed array, and one 20 MiB clone blocks the
// renderer for tens of milliseconds. Bodies go over in slices of about this size, one job each,
// so the main thread never holds more than one slice's copy and frames land between jobs.
const BATCH_BYTES = 1 << 20;

export const useDocumentPageStatsStore = defineStore('documentPageStats', () => {
  const fieldNames = shallowReactive<Record<string, readonly string[]>>({});
  const requests = new Map<string, Request>();

  async function compute(scope: string, page: DocumentPage, signal: AbortSignal): Promise<void> {
    const rows = useDocumentRowsStore();
    const generation = rows.generationOf(scope);
    const { data, offsets } = page.bodies;
    const names = new Set<string>();
    let from = 0;
    while (from < page.rowCount) {
      const lo = offsets[from];
      let to = from + 1;
      while (to < page.rowCount && offsets[to + 1] - lo <= BATCH_BYTES) to++;
      const batchOffsets = offsets.slice(from, to + 1).map((o) => o - lo);
      const out = await parseClient.run(
        'doc.pageStats',
        { data: data.slice(lo, offsets[to]), offsets: batchOffsets },
        { signal },
      );
      rows.primeLineCounts(scope, generation, from, out.lineCounts);
      for (const name of out.fieldNames) names.add(name);
      from = to;
    }
    fieldNames[scope] = [...names].sort(compareNatural);
  }

  /** Starts the job for `scope`'s current page; a no-op while the same page object is in flight or done. */
  function request(scope: string, page: DocumentPage): void {
    if (requests.get(scope)?.page === page) return;
    requests.get(scope)?.ctrl.abort();
    delete fieldNames[scope];
    const ctrl = new AbortController();
    requests.set(scope, { page, ctrl });
    compute(scope, page, ctrl.signal).catch((err: unknown) => {
      if (err instanceof DOMException && err.name === 'AbortError') return;
      // The client already falls back inline when the worker dies, so a rejection is a handler
      // bug: surface it as an unhandled rejection. The window still counts its own rows.
      throw err;
    });
  }

  /** `null` until the worker has answered for the scope's current page. */
  function fieldNamesFor(scope: string): readonly string[] | null {
    return fieldNames[scope] ?? null;
  }

  function drop(scope: string): void {
    requests.get(scope)?.ctrl.abort();
    requests.delete(scope);
    delete fieldNames[scope];
  }

  registerTabRuntimeCleanup(drop);

  return { request, fieldNamesFor, drop };
});
