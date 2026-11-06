// run() captured its runtime record (`rt`) locally and only re-checked
// `rt.opId`/`rt.status` after the await — never whether the tab itself still existed. Closing a
// tab mid-run deletes `runtime[tabId]` (state/tabRuntime.ts's cleanup), but `rt` is still a live
// reference to that now-detached object, so the continuation ran anyway: dropResults() was a
// no-op (the runtime entry was already gone) and setPage() wrote the result page into
// resultPages.ts's module-level store under a key nothing can ever reach again (`nextSeq` never
// repeats) — a permanent leak. This drives run() end to end, triggering the exact runtime-cleanup
// hook `state/tabs.ts`'s real `closeTab` fires (`cleanupTabRuntime`, `state/tabRuntime.ts`) while
// the query is still in flight, and asserts nothing gets written once it resolves. Deliberately
// bypasses `closeTab` itself, which also debounces a real `control.tabsSave` IPC call through
// `bridge/port.ts`'s module-scope singleton — `bridge-port.spec.ts` drives that same singleton
// directly against its own fake socket, so triggering a real send from here would race it.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import type { ConnectionSummary } from '@shared/domain/connection';
import type { ExecuteResponse } from '@shared/protocol/dataOps';
import type { Page } from '@shared/protocol/page';
import { cleanupTabRuntime } from '@workbench/state/tabRuntime';
import { deferred, sleep } from '@workbench/testing/unit/async';
import { bootstrapConsole } from './support/consoleHarness.ts';

const { data, control, connectionsStore, tabsStore, consoleViewStore } = await bootstrapConsole({
  control: true,
});
const { resultPageKey } = await import('../../../frontend/src/studio/views/console/state');
const { getPage } = await import('../../../frontend/src/studio/views/console/resultPages');

function fakePage(): Page {
  // biome-ignore lint/suspicious/noExplicitAny: minimal fake page, run() only reads kind/rowCount
  return { kind: 'tabular', rowCount: 0 } as any;
}

describe('console run() after the tab closes mid-run (P12 round 2 finding #3)', () => {
  test('the in-flight result is never written once the tab is gone', async () => {
    const tabId = tabsStore.openConsoleTab('conn-close-mid-run', 'db');
    const call = deferred<ExecuteResponse>();
    // biome-ignore lint/suspicious/noExplicitAny: a minimal stub, not the real data.execute
    (data as any).execute = (): Promise<ExecuteResponse> => call.promise;

    const running = consoleViewStore.run(tabId, ['SELECT 1']);
    await Promise.resolve(); // let run() reach the await inside data.execute

    cleanupTabRuntime(tabId); // the real closeTab-time signal — deletes runtime[tabId]
    expect(consoleViewStore.runtime[tabId]).toBeUndefined();

    call.resolve({ pages: [fakePage()] });
    await running;

    expect(consoleViewStore.runtime[tabId]).toBeUndefined();
    // The leaked key run() would otherwise have written under, had it ignored the closed tab.
    expect(getPage(resultPageKey(tabId, 0))).toBeNull();
  });
});

// Closing a tab while its auto-explain pre-run EXPLAIN batch is in flight used to
// leave the real statement free to fire once that batch settled — cleanup deleted runtime[tabId]
// but never touched the detached `rt` object run()'s own post-await guard reads, so `rt.status`
// still read 'running' forever and the guard passed. Drives the same auto-explain race
// console-stop-auto-explain.spec.ts covers for a Stop press, but for a tab close instead.
describe('console run() closed mid auto-explain (P108 Part 11 F1)', () => {
  test('cancels the EXPLAIN batch and the real statement never fires', async () => {
    const connectionId = 'conn-close-mid-auto-explain';
    connectionsStore.records.push({
      id: connectionId,
      // biome-ignore lint/suspicious/noExplicitAny: a minimal fixture, not a real ConnectionSummary
      ...({ kind: 'postgres', autoExplain: true, name: 'x', color: 'blue' } as any),
    } as ConnectionSummary);
    const tabId = tabsStore.openConsoleTab(connectionId, 'db');

    const explainCall = deferred<ExecuteResponse>();
    const executeCalls: string[] = [];
    // biome-ignore lint/suspicious/noExplicitAny: a minimal stub, not the real data.execute
    (data as any).execute = (req: { opId: string; statements: string[] }) => {
      executeCalls.push(req.opId);
      return explainCall.promise; // only the EXPLAIN batch's own call should ever happen here
    };
    const cancelledOpIds: string[] = [];
    // biome-ignore lint/suspicious/noExplicitAny: a minimal stub, not the real control.opsCancel
    (control as any).opsCancel = (opId: string) => {
      cancelledOpIds.push(opId);
      return Promise.resolve(true);
    };

    const running = consoleViewStore.run(tabId, ['SELECT * FROM big']);
    await sleep(10); // let run() reach the EXPLAIN batch's own await
    const explainOpId = consoleViewStore.runtime[tabId]?.explainOpId as string;
    const runOpId = consoleViewStore.runtime[tabId]?.opId as string;
    expect(explainOpId).toBeTruthy();

    cleanupTabRuntime(tabId); // the real closeTab-time signal, fired while EXPLAIN is in flight
    expect(consoleViewStore.runtime[tabId]).toBeUndefined();
    // Cleanup must cancel both ops, same as a Stop press (console-stop-auto-explain.spec.ts) —
    // not just delete the runtime entry and leave the backend op running.
    expect(cancelledOpIds).toEqual([explainOpId, runOpId]);

    explainCall.resolve({ pages: [] }); // the EXPLAIN batch settles after the tab is already gone
    await running;

    // The real statement must never fire: only the EXPLAIN batch's own execute() call happened.
    expect(executeCalls).toEqual([explainOpId]);
    expect(getPage(resultPageKey(tabId, 0))).toBeNull();
  });
});
