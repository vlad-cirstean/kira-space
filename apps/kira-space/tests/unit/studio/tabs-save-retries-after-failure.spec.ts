// saveIfChanged (state/tabs.ts) used to assign lastSavedSnapshot *before* awaiting
// control.tabsSave, so a rejected write (the FK case
// onConnectionsChanged exists to prevent, or any transient failure) was still recorded as
// "already persisted". The next call to saveIfChanged, even for the byte-identical snapshot the
// failed write never stored, would then see `snapshot === lastSavedSnapshot` and skip the
// retry outright — the tab list silently never gets saved again until something changes it to a
// genuinely different shape.
//
// activateTab(id) on an already-active tab is the reproduction vehicle: setActiveTabId sets each
// tab's own `.active` field, which is already `true` for the target, so tabsState.tabs serializes
// to the exact same JSON both times it is called — the same "identical snapshot recurs" shape the
// bug needs.
import '@workbench/testing/unit/window';

import { describe, expect, spyOn, test } from 'bun:test';
import { restoreAfterEach } from '@workbench/testing/unit/restoreAfterEach';
import { setActivePinia } from 'pinia';
import { pinia } from '../../../frontend/src/state/pinia';
import { seedConnectionRecord } from './support/connectionRecord.ts';

setActivePinia(pinia);

const { control } = await import('../../../frontend/src/bridge/control');
restoreAfterEach(control);
const { useConnectionsStore } = await import('../../../frontend/src/studio/state/connections');
const connectionsStore = useConnectionsStore();
const { useTabsStore } = await import('../../../frontend/src/state/tabs');
const tabsStore = useTabsStore();

function flush(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

describe('state/tabs.ts saveIfChanged retries a snapshot a failed save never actually persisted', () => {
  test('a save that fails does not permanently block a later, identical-looking save', async () => {
    const logged = spyOn(console, 'error').mockImplementation(() => {});
    let calls = 0;
    let shouldFail = true;
    (control as unknown as { tabsSave: typeof control.tabsSave }).tabsSave = () => {
      calls += 1;
      return shouldFail ? Promise.reject(new Error('simulated FK failure')) : Promise.resolve();
    };

    seedConnectionRecord(connectionsStore, 'conn-tabs-1');
    const { id } = tabsStore.openStreamTab('conn-tabs-1', 'topic:orders', { newTab: true })!;
    await flush();
    expect(calls).toBe(1); // the open itself triggers saveNow -> saveIfChanged's first (failing) call

    // activateTab on the tab openStreamTab just made active is a no-op in terms of resulting
    // state — tabsState.tabs serializes identically both times — so this reproduces "the exact
    // same snapshot the failed save above never actually stored".
    shouldFail = false;
    tabsStore.activateTab(id);
    await flush();

    expect(calls).toBe(2);
    logged.mockRestore();
  });
});
