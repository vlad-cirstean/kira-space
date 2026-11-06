// A server-streaming call's messages arrive over the event
// channel (control.onGrpcCall), while the call's own CallResult returns over the control plane —
// a separate HTTP request (control.grpcCall) — with nothing ordering the two. The return path's
// own guard (`if (rt.opId !== opId) return`) assumed the terminal event always arrives first; when
// the control-plane response lands first instead, the return path used to clear rt.opId before the
// terminal event — carrying the trailing message batch and the terminal status — had been
// delivered, and the event handler's own `rt.opId !== event.callId` check then dropped it outright.
// This drives both orderings directly, the exact interleaving no Playwright test can force (the
// same driven-deferred technique api-server-state-refresh.spec.ts uses).
import '@workbench/testing/unit/window';

import { afterEach, describe, expect, test } from 'bun:test';
import type { GrpcCallEvent, GrpcCallResultWire, GrpcSchemaWire } from '@shared/domain/grpc';
import { deferred, sleep } from '@workbench/testing/unit/async';
import { setActivePinia } from 'pinia';
import { effectScope, isReactive } from 'vue';
import { pinia } from '../../../frontend/src/state/pinia';

setActivePinia(pinia);

const { control } = await import('../../../frontend/src/studio/bridge/control');
// createTabsStore's saveIfChanged now serialises every save through one
// persistent chain — a real (never-settling in this harness) control.tabsSave triggered
// incidentally by opening a tab below would otherwise wedge every later spec's own tabsSave
// assertions for the rest of the process. This spec doesn't test persistence, so give it a
// benign default (not restored via restoreAfterEach — this file's own afterEach below only
// restores grpcCall/grpcHistoryList, matching its existing convention).
(control as unknown as { tabsSave: typeof control.tabsSave }).tabsSave = () => Promise.resolve();
const { openGrpcRequestTab, patchGrpcRequestTabState } = await import(
  '../../../frontend/src/studio/api/tabs'
);
const { useGrpcRequestViewStore } = await import(
  '../../../frontend/src/studio/views/grpcrequest/state'
);
const { useGrpcHistoryList } = await import(
  '../../../frontend/src/studio/views/grpcrequest/history'
);
const { findGrpcRequestTab } = await import('../../../frontend/src/studio/api/tabs');
const { grpcSchemaKey, grpcSchemaSourceFor } = await import(
  '../../../frontend/src/studio/views/grpcrequest/schemaQuery'
);
const { queryClient } = await import('@workbench/state/queryClient');
const grpcRequestViewStore = useGrpcRequestViewStore();

// call()'s pre-flight loads the tree and environments; unstubbed they never settle in this harness.
(
  control as unknown as { variablesListEnvironments: typeof control.variablesListEnvironments }
).variablesListEnvironments = async () => [];
(control as unknown as { collectionsList: typeof control.collectionsList }).collectionsList =
  async () => ({ collections: [], items: [] });

const originalGrpcCall = control.grpcCall;
const originalGrpcHistoryList = control.grpcHistoryList;
afterEach(() => {
  control.grpcCall = originalGrpcCall;
  control.grpcHistoryList = originalGrpcHistoryList;
});

// Counts how many times the gRPC call-history list query fetches — the
// direct, observable signature of noteGrpcCallRecorded firing. Set to `historyListCalls = 0`
// (below) and the tab's `responsePane` to 'history' (so noteRecorded's own eager branch fires a
// real load()) at the top of any test that needs to assert on it.
let historyListCalls = 0;
// biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real grpcHistoryList
(control as any).grpcHistoryList = async () => {
  historyListCalls++;
  return [];
};

// ensureGrpcCallSubscription (state.ts) calls control.onGrpcCall exactly once ever, module-wide
// (a `subscribedToGrpcCall` flag, never reset) — the first call() in this whole test process to
// reach it is the only one that ever registers a callback. Captured once, at module scope, so
// every test below (and any earlier spec file's own call() in this shared bun test process) shares
// the same captured callback rather than each expecting its own fresh registration.
let capturedCallback: ((event: GrpcCallEvent) => void) | null = null;
// biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real onGrpcCall
(control as any).onGrpcCall = (cb: (event: GrpcCallEvent) => void) => {
  capturedCallback = cb;
  return () => {};
};

function streamingSchema(): GrpcSchemaWire {
  return {
    services: [
      {
        name: 'Svc',
        fullName: 'Svc',
        methods: [
          {
            name: 'Stream',
            fullName: 'Svc/Stream',
            clientStreaming: false,
            serverStreaming: true,
            inputType: '.Req',
            outputType: '.Resp',
            requestTemplate: '{}',
          },
        ],
      },
    ],
    mode: 'proto',
    warnings: [],
  };
}

function terminalResult(): GrpcCallResultWire {
  return {
    code: 0,
    codeName: 'OK',
    statusMessage: '',
    elapsedMs: 5,
    header: [],
    trailer: [],
    messageCount: 2,
    messageBytes: 20,
  };
}

function setUpStreamingTab(): string {
  const id = openGrpcRequestTab();
  // responsePane: 'history' plus a mounted list observer, so noteGrpcCallRecorded's refresh
  // (api/state/history.ts's noteRecorded) refetches the active list — the observable signal the F1
  // test below counts via historyListCalls, rather than the lazy stale mark a hidden pane gets.
  patchGrpcRequestTabState(id, { service: 'Svc', method: 'Stream', responsePane: 'history' });
  const tab = findGrpcRequestTab(id) as NonNullable<ReturnType<typeof findGrpcRequestTab>>;
  effectScope().run(() => useGrpcHistoryList(() => tab));
  queryClient.setQueryData(grpcSchemaKey(grpcSchemaSourceFor(tab)), streamingSchema());
  historyListCalls = 0;
  return id;
}

describe("a server-streaming call's terminal event race (F5)", () => {
  test('the control-plane response arriving before the terminal event does not lose the trailing batch', async () => {
    const id = setUpStreamingTab();
    await sleep(); // the observer's own initial list fetch is not the signal under test
    historyListCalls = 0;

    const grpcCallDeferred = deferred<GrpcCallResultWire>();
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real grpcCall
    (control as any).grpcCall = () => grpcCallDeferred.promise;

    const callPromise = grpcRequestViewStore.call(id);
    // Wait a tick so call() has run its synchronous setup (subscribing, setting opId/lastCallId)
    // and reached the awaited control.grpcCall.
    await Promise.resolve();
    await Promise.resolve();

    const opId = grpcRequestViewStore.runtime[id]?.opId;
    expect(opId).toBeTruthy();

    // The control-plane response lands FIRST — the reverse of the order the code used to assume.
    grpcCallDeferred.resolve(terminalResult());
    await callPromise;
    expect(grpcRequestViewStore.runtime[id]?.opId).toBeNull();
    expect(grpcRequestViewStore.runtime[id]?.status).toBe('idle');

    // The terminal event — carrying the trailing batch — is delivered only now, after opId has
    // already been cleared.
    expect(capturedCallback).not.toBeNull();
    capturedCallback?.({
      callId: opId as string,
      seq: 0,
      messages: [
        { seq: 0, json: '{"n":0}', wireBytes: 10, offsetMs: 1 },
        { seq: 1, json: '{"n":1}', wireBytes: 10, offsetMs: 2 },
      ],
      done: true,
      status: terminalResult(),
    });

    // The fix: the event is still matched (via lastCallId, which opId-clearing does not touch)
    // and its messages are applied — this is exactly what used to be silently dropped.
    expect(grpcRequestViewStore.runtime[id]?.messages).toHaveLength(2);
    expect(grpcRequestViewStore.runtime[id]?.trueMessageCount).toBe(2);
    expect(grpcRequestViewStore.runtime[id]?.messageBytes).toBe(20);

    // Both the control-plane return (above) and this terminal event observed "the call
    // finished" — each used to call noteGrpcCallRecorded unconditionally, recording this one call
    // twice and feeding history's own load() two overlapping refreshes close enough together to
    // retry-loop forever. `notifiedCallId` must have deduped them to exactly one history list
    // fetch.
    expect(historyListCalls).toBe(1);
  });

  test('the terminal event arriving before the control-plane response still works (the order the code always handled)', async () => {
    const id = setUpStreamingTab();

    const grpcCallDeferred = deferred<GrpcCallResultWire>();
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real grpcCall
    (control as any).grpcCall = () => grpcCallDeferred.promise;

    const callPromise = grpcRequestViewStore.call(id);
    await Promise.resolve();
    await Promise.resolve();
    const opId = grpcRequestViewStore.runtime[id]?.opId;

    capturedCallback?.({
      callId: opId as string,
      seq: 0,
      messages: [{ seq: 0, json: '{"n":0}', wireBytes: 10, offsetMs: 1 }],
      done: true,
      status: terminalResult(),
    });
    expect(grpcRequestViewStore.runtime[id]?.opId).toBeNull();
    expect(grpcRequestViewStore.runtime[id]?.messages).toHaveLength(1);

    grpcCallDeferred.resolve(terminalResult());
    await callPromise;

    // The return path's own supersession guard bails out — the event already finalized this call.
    expect(grpcRequestViewStore.runtime[id]?.messages).toHaveLength(1);
  });
});

// rt.messages is a deep-reactive array (createRuntimeStore's own reactive()), and every arriving
// message used to be pushed straight in with no markRaw,
// letting Vue wrap each one in its own per-field reactive Proxy on top of the array's own
// reactivity — wasted work for an immutable wire value this view never mutates in place.
describe('live-stream message buffer (P21 round 2 performance finding 6)', () => {
  test('an arriving message is not itself made reactive', async () => {
    const id = setUpStreamingTab();
    const grpcCallDeferred = deferred<GrpcCallResultWire>();
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real grpcCall
    (control as any).grpcCall = () => grpcCallDeferred.promise;
    const callPromise = grpcRequestViewStore.call(id);
    await Promise.resolve();
    await Promise.resolve();
    const opId = grpcRequestViewStore.runtime[id]?.opId as string;

    capturedCallback?.({
      callId: opId,
      seq: 0,
      messages: [{ seq: 0, json: '{"n":0}', wireBytes: 10, offsetMs: 1 }],
      done: false,
    });

    // This fails against the pre-fix handler, which pushed event.messages straight into the
    // reactive array with no markRaw — reactive()'s own array getter wraps every plain-object
    // element it returns, so the pushed message would come back as a reactive Proxy.
    expect(isReactive(grpcRequestViewStore.runtime[id]?.messages[0])).toBe(false);

    grpcCallDeferred.resolve(terminalResult());
    await callPromise;
  });

  // 10_000 (MAX_LIVE_MESSAGES) is state.ts's own private constant — restated here as a literal
  // rather than imported. An amortized 90%-of-cap trim target was tried and reverted (it broke
  // the "exactly the most recent 10,000" contract, caught by grpc-request.spec.ts's own
  // "caps at 10,000" UI test); this pins the reverted, exact-cap behavior instead.
  test('crossing the cap trims back to exactly the cap', async () => {
    const id = setUpStreamingTab();
    const grpcCallDeferred = deferred<GrpcCallResultWire>();
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real grpcCall
    (control as any).grpcCall = () => grpcCallDeferred.promise;
    const callPromise = grpcRequestViewStore.call(id);
    await Promise.resolve();
    await Promise.resolve();
    const opId = grpcRequestViewStore.runtime[id]?.opId as string;

    let seq = 0;
    function pushBatch(n: number): void {
      const messages = Array.from({ length: n }, () => ({
        seq: seq++,
        json: '{}',
        wireBytes: 1,
        offsetMs: 0,
      }));
      capturedCallback?.({ callId: opId, seq: 0, messages, done: false });
    }

    for (let i = 0; i < 156; i++) pushBatch(64); // 156 * 64 = 9 984
    pushBatch(16); // exactly 10 000 — right at the cap, no trim triggered yet
    expect(grpcRequestViewStore.runtime[id]?.messages.length).toBe(10_000);

    pushBatch(1); // crosses the cap: this is the splice this test pins
    expect(grpcRequestViewStore.runtime[id]?.messages.length).toBe(10_000);
    expect(grpcRequestViewStore.runtime[id]?.trueMessageCount).toBe(10_001);

    grpcCallDeferred.resolve(terminalResult());
    await callPromise;
  });
});

// P168 Part 10 F21: the variable load ran outside call()'s try, so a rejection wedged the tab in
// 'running', Stop cancelled an op Go never saw, and a tab closed mid-load still called.
describe('call() pre-flight (P168 Part 10 F21)', () => {
  test('a closed tab or a Stop during pre-flight issues no call', async () => {
    const gate = deferred<Awaited<ReturnType<typeof control.variablesListEnvironments>>>();
    const original = control.variablesListEnvironments;
    (
      control as unknown as { variablesListEnvironments: typeof control.variablesListEnvironments }
    ).variablesListEnvironments = () => gate.promise;
    const { queryClient } = await import('@workbench/state/queryClient');
    void queryClient.invalidateQueries({ refetchType: 'none' });
    let called = 0;
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real grpcCall
    (control as any).grpcCall = async () => {
      called++;
      return terminalResult();
    };

    try {
      const closedTab = setUpStreamingTab();
      const stoppedTab = setUpStreamingTab();
      const closed = grpcRequestViewStore.call(closedTab);
      const stopped = grpcRequestViewStore.call(stoppedTab);
      await Promise.resolve();

      const { useTabsStore } = await import('../../../frontend/src/state/tabs');
      useTabsStore().closeTab(closedTab);
      grpcRequestViewStore.stop(stoppedTab);
      expect(grpcRequestViewStore.runtime[stoppedTab]?.status).toBe('cancelled');

      gate.resolve([]);
      await Promise.all([closed, stopped]);
      expect(called).toBe(0);
      expect(grpcRequestViewStore.runtime[closedTab]).toBeUndefined();
    } finally {
      (
        control as unknown as {
          variablesListEnvironments: typeof control.variablesListEnvironments;
        }
      ).variablesListEnvironments = original;
    }
  });

  test('a failed pre-flight load surfaces as an error and a retry can call', async () => {
    const { queryClient } = await import('@workbench/state/queryClient');
    void queryClient.invalidateQueries({ refetchType: 'none' });
    const id = setUpStreamingTab();
    let failing = true;
    (
      control as unknown as { variablesListEnvironments: typeof control.variablesListEnvironments }
    ).variablesListEnvironments = async () => {
      if (failing) throw new Error('bridge down');
      return [];
    };
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real grpcCall
    (control as any).grpcCall = async () => terminalResult();

    await grpcRequestViewStore.call(id);
    expect(grpcRequestViewStore.runtime[id]?.status).toBe('error');
    expect(grpcRequestViewStore.runtime[id]?.opId).toBeNull();

    failing = false;
    await grpcRequestViewStore.call(id);
    expect(grpcRequestViewStore.runtime[id]?.status).toBe('idle');
  });
});
