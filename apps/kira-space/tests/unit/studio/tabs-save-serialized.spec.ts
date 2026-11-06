// createTabsStore's saveIfChanged/saveNow/flushPendingTabState used to fire an
// independent tabsSave call every time (guarded only against re-sending an exact snapshot already
// in flight), with no ordering guarantee between two overlapping calls at all — Wails v3 dispatches
// each bound call on its own goroutine, so whichever HTTP round trip happened to land last in the
// DB won, not whichever was issued last client-side. enqueueSave's saveChain now serialises every
// save behind whatever is already queued, and nextSnapshot coalesces any burst of intervening
// changes down to just the latest one by the time each queued link runs.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import { deferred } from '@workbench/testing/unit/async';
import { restoreAfterEach } from '@workbench/testing/unit/restoreAfterEach';
import { setActivePinia } from 'pinia';
import { pinia } from '../../../frontend/src/studio/state/pinia';
import { seedConnectionRecord } from './support/connectionRecord.ts';

setActivePinia(pinia);

const { control } = await import('../../../frontend/src/studio/bridge/control');
restoreAfterEach(control);
const { useConnectionsStore } = await import('../../../frontend/src/studio/state/connections');
const connectionsStore = useConnectionsStore();
const { useTabsStore } = await import('../../../frontend/src/studio/state/tabs');
const tabsStore = useTabsStore();

describe('state/tabs.ts saveIfChanged serialises and coalesces overlapping saves (F12)', () => {
  test('a save queued while another is in flight waits, and skips a since-superseded snapshot', async () => {
    const calls: unknown[][] = [];
    const gates: ReturnType<typeof deferred<void>>[] = [];
    (control as unknown as { tabsSave: typeof control.tabsSave }).tabsSave = (...args) => {
      calls.push(args);
      const gate = deferred<void>();
      gates.push(gate);
      return gate.promise;
    };

    // S1: open a tab -- triggers saveNow() -> the first tabsSave call, held open on gates[0].
    seedConnectionRecord(connectionsStore, 'conn-tabs-2');
    const { id } = tabsStore.openStreamTab('conn-tabs-2', 'topic:orders', { newTab: true })!;
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(calls.length).toBe(1);

    // S2: while S1's write is still in flight, change the tabs again and save -- this must NOT
    // fire a second tabsSave call yet; it queues behind S1.
    tabsStore.activateTab(id);
    tabsStore.saveNow();
    expect(calls.length).toBe(1);

    // S3: another change lands before S1 even resolves, coalescing S2's own queued target away --
    // only the latest state (S3) should ever reach the network once S1 finally settles.
    tabsStore.closeTab(id);
    tabsStore.saveNow();
    expect(calls.length).toBe(1);

    // S1 resolves -- the queued link now runs and should send S3 directly, never S2.
    gates[0]?.resolve();
    await new Promise((resolve) => setTimeout(resolve, 0));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(calls.length).toBe(2);
    const s1Snapshot = JSON.stringify(calls[0]?.[0]);
    const s3Snapshot = JSON.stringify(calls[1]?.[0]);
    expect(s3Snapshot).not.toBe(s1Snapshot);

    // No further link is queued behind S3 -- letting it settle produces no third call.
    gates[1]?.resolve();
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(calls.length).toBe(2);
  });
});
