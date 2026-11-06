// Closing an HTTP tab mid-send leaked both the in-flight op and a history runtime entry.
// Unlike gRPC's own cleanup (views/grpcrequest/state.ts, stopOp before delete), HTTP's cleanup was
// only `delete runtime[tabId]` — the send kept running in Go and recording history, and on
// completion `rt.opId !== opId` still passed (rt is a captured reference cleanup never mutates),
// so noteSendRecorded ran for the closed tab, recreating a history UI entry
// (api/state/history.ts) nothing ever cleans up again. This pins both halves: the op is cancelled
// on close, and the post-await path is a no-op once the tab is gone.
import '@workbench/testing/unit/window';

import { beforeEach, describe, expect, test } from 'bun:test';
import type { HttpResponseWire } from '@shared/domain/http';
import { deferred, sleep } from '@workbench/testing/unit/async';
import { restoreAfterEach } from '@workbench/testing/unit/restoreAfterEach';
import { setActivePinia } from 'pinia';
import { pinia } from '../../../frontend/src/studio/state/pinia';

setActivePinia(pinia);

const { control } = await import('../../../frontend/src/studio/bridge/control');
const { queryClient } = await import('@workbench/state/queryClient');
// createTabsStore's saveIfChanged now serialises every save through one
// persistent chain — a real (never-settling in this harness) control.tabsSave triggered
// incidentally by opening a tab below would otherwise wedge every later spec's own tabsSave
// assertions for the rest of the process. This spec doesn't test persistence, so give it a
// benign default -- set *before* restoreAfterEach snapshots control, not in beforeEach: the
// enqueued save this triggers can settle on a tick past this file's own afterEach (confirmed
// via stream-count-honors-filter.spec.ts's identical hazard: only reproduces with ~90+ other
// spec files loaded, never in isolation), so a beforeEach/afterEach-scoped override races the
// chain back onto the hanging default and wedges it permanently for every later spec in the
// process.
(control as unknown as { tabsSave: typeof control.tabsSave }).tabsSave = () => Promise.resolve();
restoreAfterEach(control);
beforeEach(() => {
  // send()'s pre-flight loads the tree and environments; unstubbed they never settle in this harness.
  void queryClient.invalidateQueries({ refetchType: 'none' });
  (control as unknown as { collectionsList: typeof control.collectionsList }).collectionsList =
    async () => ({ collections: [], items: [] });
  (
    control as unknown as { variablesListEnvironments: typeof control.variablesListEnvironments }
  ).variablesListEnvironments = async () => [];
});
const { openApiRequestTab } = await import('../../../frontend/src/studio/api/tabs');
const { useTabsStore } = await import('../../../frontend/src/studio/state/tabs');
const { useHttpRequestViewStore } = await import('../../../frontend/src/studio/views/httprequest/state');
const { useHttpHistoryStore } = await import('../../../frontend/src/studio/views/httprequest/history');

function response(): HttpResponseWire {
  return {
    status: 200,
    statusText: 'OK',
    headers: [],
    body: '',
    bodyEncoding: 'utf8',
    bodyTruncated: false,
    durationMs: 0,
    sizeBytes: 0,
  } as unknown as HttpResponseWire;
}

describe('closing an HTTP tab mid-send stops the op and leaks no history runtime (P108 F8)', () => {
  test('cleanup cancels the op, and the send that was already in flight is a no-op on completion', async () => {
    const tabId = openApiRequestTab();
    const httpSend = deferred<HttpResponseWire>();
    let cancelledOpId: string | undefined;
    (control as unknown as { httpSend: typeof control.httpSend }).httpSend = () => httpSend.promise;
    (control as unknown as { opsCancel: typeof control.opsCancel }).opsCancel = async (
      opId: string,
    ) => {
      cancelledOpId = opId;
    };

    const httpRequestViewStore = useHttpRequestViewStore();
    const sendPromise = httpRequestViewStore.send(tabId);
    await sleep(); // let pre-flight finish so httpSend is in flight

    const opId = httpRequestViewStore.runtime[tabId]?.opId ?? undefined;
    expect(opId).toBeTruthy();

    useTabsStore().closeTab(tabId);
    // Cleanup must cancel the in-flight op, not just drop the runtime silently.
    expect(cancelledOpId).toBe(opId);
    expect(httpRequestViewStore.runtime[tabId]).toBeUndefined();

    // The send's own IPC call resolves after the tab is already gone.
    httpSend.resolve(response());
    await sendPromise;

    // Neither runtime was resurrected by the post-await path.
    expect(httpRequestViewStore.runtime[tabId]).toBeUndefined();
    expect(useHttpHistoryStore().ui[tabId]).toBeUndefined();
  });
});

// P168 Part 10 F15: the variable/tree load ran outside send()'s try, so a rejection wedged the tab
// in 'running', Stop cancelled an op Go never saw, and a tab closed mid-load still sent.
describe('send() pre-flight (P168 Part 10 F15)', () => {
  function stubSend() {
    const sent: string[] = [];
    (control as unknown as { httpSend: typeof control.httpSend }).httpSend = async (args) => {
      sent.push(args.opId);
      return response();
    };
    return sent;
  }

  test('a failed pre-flight load surfaces as an error and a retry can send', async () => {
    const tabId = openApiRequestTab();
    const sent = stubSend();
    let failing = true;
    (control as unknown as { collectionsList: typeof control.collectionsList }).collectionsList =
      async () => {
        if (failing) throw new Error('bridge down');
        return { collections: [], items: [] };
      };
    const store = useHttpRequestViewStore();

    await store.send(tabId);
    expect(store.runtime[tabId]?.status).toBe('error');
    expect(store.runtime[tabId]?.opId).toBeNull();
    expect(sent).toEqual([]);

    failing = false;
    await store.send(tabId);
    expect(sent).toHaveLength(1);
    expect(store.runtime[tabId]?.status).toBe('idle');
  });

  test('closing the tab or pressing Stop during pre-flight sends nothing', async () => {
    const sent = stubSend();
    const gate = deferred<Awaited<ReturnType<typeof control.collectionsList>>>();
    (control as unknown as { collectionsList: typeof control.collectionsList }).collectionsList =
      () => gate.promise;
    const store = useHttpRequestViewStore();

    const closedTab = openApiRequestTab();
    const stoppedTab = openApiRequestTab();
    const closedSend = store.send(closedTab);
    const stoppedSend = store.send(stoppedTab);
    await Promise.resolve();

    useTabsStore().closeTab(closedTab);
    store.stop(stoppedTab);
    expect(store.runtime[stoppedTab]?.status).toBe('cancelled');

    gate.resolve({ collections: [], items: [] } as Awaited<
      ReturnType<typeof control.collectionsList>
    >);
    await Promise.all([closedSend, stoppedSend]);

    expect(sent).toEqual([]);
    expect(store.runtime[closedTab]).toBeUndefined();
    expect(store.runtime[stoppedTab]?.status).toBe('cancelled');
  });
});
