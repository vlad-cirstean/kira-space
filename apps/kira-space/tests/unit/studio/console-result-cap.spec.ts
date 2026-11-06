// console/state.ts's `run()` used to push every run's result set(s) onto `rt.results`
// unconditionally, with nothing ever evicting an old one short of the user's own close/close-others
// action or closing the tab — a tab left open for a long session of many small queries in append
// mode (the default, consoleTabStateSchema's own `newResultSet: true`) retained every one of their
// full decoded pages forever. This file drives the real `run()` end to end (not just the pure
// eviction helper) — data.execute is overridden the same way view-state.spec.ts does for
// data.read/control.treeChildren, so the exercised code is state.ts's actual push+evict sequence,
// not a hand-rolled stand-in for it.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import type { ExecuteResponse } from '@shared/protocol/dataOps';
import type { Page } from '@shared/protocol/page';
import { restoreAfterEach } from '@workbench/testing/unit/restoreAfterEach';
import { setActivePinia } from 'pinia';
import { pinia } from '../../../frontend/src/state/pinia';

setActivePinia(pinia);

const { data } = await import('../../../frontend/src/studio/bridge/data');
restoreAfterEach(data);
const { control } = await import('../../../frontend/src/studio/bridge/control');
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
const { useTabsStore } = await import('../../../frontend/src/state/tabs');
const tabsStore = useTabsStore();
const { useConsoleViewStore } = await import('../../../frontend/src/studio/views/console/state');
const consoleViewStore = useConsoleViewStore();
const { getPage } = await import('../../../frontend/src/studio/views/console/resultPages');

// A minimal fake — state.ts's run() only reads page.kind and page.rowCount, never anything
// shape-specific to a real tabular/document page.
function fakePage(): Page {
  // biome-ignore lint/suspicious/noExplicitAny: minimal fake page, see comment above
  return { kind: 'tabular', rowCount: 0 } as any;
}

describe('console result cap (P2 R1)', () => {
  test('1. append mode evicts the oldest results once a tab holds more than the cap', async () => {
    const tabId = tabsStore.openConsoleTab('conn-cap-1', 'db');
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real ExecuteResponse
    (data as any).execute = (): Promise<ExecuteResponse> =>
      Promise.resolve({ pages: [fakePage()] });

    for (let i = 0; i < 55; i++) await consoleViewStore.run(tabId, [`SELECT ${i}`]);

    const results = consoleViewStore.runtime[tabId]?.results ?? [];
    expect(results.length).toBeLessThanOrEqual(50);

    // The very first run's result must be gone from both the list and the underlying page store —
    // otherwise this is just testing that the array stopped growing while still leaking pages.
    const firstKey = `${tabId}:result:0`;
    expect(results.some((r) => r.key === firstKey)).toBe(false);
    expect(getPage(firstKey)).toBeNull();

    // The most recent run's result must still be there.
    const lastKey = results[results.length - 1]?.key;
    expect(lastKey).toBeDefined();
    expect(getPage(lastKey as string)).not.toBeNull();
  });

  test("2. a single run's own result set is never evicted, however many statements it produced", async () => {
    const tabId = tabsStore.openConsoleTab('conn-cap-2', 'db');
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real ExecuteResponse
    (data as any).execute = (): Promise<ExecuteResponse> =>
      Promise.resolve({ pages: [fakePage()] });
    for (let i = 0; i < 10; i++) await consoleViewStore.run(tabId, [`SELECT ${i}`]);
    expect(consoleViewStore.runtime[tabId]?.results.length).toBe(10);

    // One "Run all" producing more result sets in a single call than the cap itself.
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real ExecuteResponse
    (data as any).execute = (): Promise<ExecuteResponse> =>
      Promise.resolve({ pages: Array.from({ length: 60 }, () => fakePage()) });
    await consoleViewStore.run(
      tabId,
      Array.from({ length: 60 }, (_, i) => `SELECT ${i}`),
    );

    // All 10 pre-existing results are evicted to make room, but none of this run's own 60 are —
    // the cap only ever trims *earlier* runs, never truncates the run that just completed.
    expect(consoleViewStore.runtime[tabId]?.results.length).toBe(60);
  });
});
