// Stop was a no-op while auto-explain's pre-run EXPLAIN batch was in
// flight — rt.opId held the *real* run's future id, which the backend had never registered, so
// stopOp's control.opsCancel(rt.opId) call landed on nothing. The EXPLAIN batch then always ran to
// completion and the real (possibly expensive) query fired regardless of the Stop press. This
// drives run()/stop() end to end (same style as console-result-cap.spec.ts) to prove: pressing
// Stop while the EXPLAIN batch is in flight cancels *that* op, and the real query is never issued
// at all — the one ordering guarantee no non-Docker Playwright test can force deterministically.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import type { ConnectionSummary } from '@shared/domain/connection';
import type { ExecuteResponse } from '@shared/protocol/dataOps';
import { deferred, sleep } from '@workbench/testing/unit/async';
import { bootstrapConsole } from './support/consoleHarness.ts';

const { data, control, connectionsStore, tabsStore, consoleViewStore } = await bootstrapConsole({
  control: true,
});

describe('console Stop during the auto-explain pre-run batch (P12 round 1 F5)', () => {
  test('cancels the EXPLAIN batch and never issues the real run', async () => {
    const connectionId = 'conn-stop-auto-explain';
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
      // Only the EXPLAIN batch's own call should ever happen in this test.
      return explainCall.promise;
    };
    const cancelledOpIds: string[] = [];
    // biome-ignore lint/suspicious/noExplicitAny: a minimal stub, not the real control.opsCancel
    (control as any).opsCancel = (opId: string) => {
      cancelledOpIds.push(opId);
      return Promise.resolve(true);
    };

    const running = consoleViewStore.run(tabId, ['SELECT 1']);
    // Let run() reach the point where the EXPLAIN batch's own op id is registered.
    await sleep(10);
    expect(consoleViewStore.runtime[tabId]?.explainOpId).not.toBeNull();
    const explainOpId = consoleViewStore.runtime[tabId]?.explainOpId as string;

    const runOpId = consoleViewStore.runtime[tabId]?.opId as string;
    consoleViewStore.stop(tabId);
    // Both the EXPLAIN batch's own op id and the real run's (not-yet-issued, unregistered on the
    // backend) opId are cancelled, best-effort — the explain one is what actually stops anything.
    expect(cancelledOpIds).toEqual([explainOpId, runOpId]);

    // Simulates the backend's cancellation response reaching the EXPLAIN batch's own execute call.
    explainCall.reject(Object.assign(new Error('cancelled'), { code: 'E_CANCELLED' }));
    await running;

    expect(executeCalls).toEqual([explainOpId]); // the real run's own execute() never fired
    expect(consoleViewStore.runtime[tabId]?.status).toBe('cancelled');
    expect(consoleViewStore.runtime[tabId]?.opId).toBeNull();
    expect(consoleViewStore.runtime[tabId]?.explainOpId).toBeNull();
  });
});
