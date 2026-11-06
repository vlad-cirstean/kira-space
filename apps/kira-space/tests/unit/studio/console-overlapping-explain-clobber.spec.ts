// run()'s two `rt.explainOpId = null` clears (the cancellation catch and
// the post-await settle path) were bare, unlike every other op-id field in this file — so an
// overlapping second run's own explainOpId, stamped *after* the first run started, could be wiped
// out when the first run's own EXPLAIN batch settles. `runStatement`/`runAll` have no `running`
// guard at the command-dispatch layer (only the toolbar buttons do), and run() itself never guarded
// re-entrancy, so two runs against the same tab can genuinely overlap. This drives run() end to end
// (same style as console-stop-auto-explain.spec.ts) to prove: run A's EXPLAIN batch settling must
// not discard run B's explainOpId, so Stop pressed during B's batch still has something to cancel.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import type { ConnectionSummary } from '@shared/domain/connection';
import type { ExecuteResponse } from '@shared/protocol/dataOps';
import { deferred, sleep } from '@workbench/testing/unit/async';
import { bootstrapConsole } from './support/consoleHarness.ts';

const { data, control, connectionsStore, tabsStore, consoleViewStore } = await bootstrapConsole({
  control: true,
});

describe("console overlapping runs must not clobber each other's explainOpId (P12 round 2 F4)", () => {
  test("run A's EXPLAIN batch settling leaves run B's explainOpId tracked for Stop", async () => {
    const connectionId = 'conn-overlap-explain';
    connectionsStore.records.push({
      id: connectionId,
      // biome-ignore lint/suspicious/noExplicitAny: a minimal fixture, not a real ConnectionSummary
      ...({ kind: 'postgres', autoExplain: true, name: 'x', color: 'blue' } as any),
    } as ConnectionSummary);
    const tabId = tabsStore.openConsoleTab(connectionId, 'db');

    type Call = {
      opId: string;
      resolve: (r: ExecuteResponse) => void;
      reject: (e: unknown) => void;
    };
    const calls: Call[] = [];
    // biome-ignore lint/suspicious/noExplicitAny: a minimal stub, not the real data.execute
    (data as any).execute = (req: { opId: string; statements: string[] }) => {
      const d = deferred<ExecuteResponse>();
      calls.push({ opId: req.opId, resolve: d.resolve, reject: d.reject });
      return d.promise;
    };
    const cancelledOpIds: string[] = [];
    // biome-ignore lint/suspicious/noExplicitAny: a minimal stub, not the real control.opsCancel
    (control as any).opsCancel = (opId: string) => {
      cancelledOpIds.push(opId);
      return Promise.resolve(true);
    };

    // Run A starts first — its own EXPLAIN batch is issued and left unresolved.
    const runA = consoleViewStore.run(tabId, ['SELECT * FROM a']);
    await sleep(10);
    expect(calls).toHaveLength(1);
    const explainOpIdA = calls[0]?.opId as string;
    expect(consoleViewStore.runtime[tabId]?.explainOpId).toBe(explainOpIdA);

    // Run B starts before A settles — genuinely overlapping (neither the toolbar's `running` guard
    // nor a re-entrancy guard inside run() itself exists at this layer), so B stamps its own,
    // later explainOpId over A's.
    const runB = consoleViewStore.run(tabId, ['SELECT * FROM b']);
    await sleep(10);
    expect(calls).toHaveLength(2);
    const explainOpIdB = calls[1]?.opId as string;
    expect(explainOpIdB).not.toBe(explainOpIdA);
    expect(consoleViewStore.runtime[tabId]?.explainOpId).toBe(explainOpIdB);

    // A's own EXPLAIN batch now settles (successfully — not a cancellation). Before the fix this
    // unconditionally wiped rt.explainOpId to null, discarding B's still-in-flight id.
    calls[0]?.resolve({ pages: [] });
    await sleep(10);
    expect(consoleViewStore.runtime[tabId]?.explainOpId).toBe(explainOpIdB);

    // Stop, pressed during B's still-in-flight EXPLAIN batch, must still find B's id to cancel.
    consoleViewStore.stop(tabId);
    expect(cancelledOpIds).toContain(explainOpIdB);

    // Simulates the backend's cancellation response reaching B's own EXPLAIN batch.
    calls[1]?.reject(Object.assign(new Error('cancelled'), { code: 'E_CANCELLED' }));
    await Promise.all([runA, runB]);
    expect(calls).toHaveLength(2); // neither run's real query was ever issued
  });
});
