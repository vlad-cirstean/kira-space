// Per-scope lifecycle of the `doc.pageStats` worker job: line counts for the row-height model and
// the page's field names, computed off the main thread so a 10,000-document page costs the
// renderer only the rows in its window.

import type { DocumentPage } from '@shared/protocol/page';
import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';
import { defineStore } from 'pinia';
import { shallowReactive } from 'vue';
import { parseClient } from '../../../workers/parse/instance';
import { useDocumentRowsStore } from './rows';

interface Request {
  page: DocumentPage;
  ctrl: AbortController;
}

export const useDocumentPageStatsStore = defineStore('documentPageStats', () => {
  const fieldNames = shallowReactive<Record<string, readonly string[]>>({});
  const requests = new Map<string, Request>();

  /** Starts the job for `scope`'s current page; a no-op while the same page object is in flight or done. */
  function request(scope: string, page: DocumentPage): void {
    if (requests.get(scope)?.page === page) return;
    requests.get(scope)?.ctrl.abort();
    delete fieldNames[scope];
    const rows = useDocumentRowsStore();
    const generation = rows.generationOf(scope);
    const ctrl = new AbortController();
    requests.set(scope, { page, ctrl });
    parseClient
      .run(
        'doc.pageStats',
        { data: page.bodies.data, offsets: page.bodies.offsets },
        { signal: ctrl.signal },
      )
      .then((out) => {
        if (ctrl.signal.aborted) return;
        rows.primeLineCounts(scope, generation, out.lineCounts);
        fieldNames[scope] = out.fieldNames;
      })
      .catch((err: unknown) => {
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
