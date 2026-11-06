// worstIndex used to be plans.findIndex(...) — the *first* flagged
// statement in a Run-all, not the worst one. On a multi-statement run where an early statement
// trips a mild warning and a later one is far more expensive, the strip's "Show plan" action
// pointed at the mild one. This drives run() end to end (same style as
// console-auto-explain-race.spec.ts) with two flagged postgres statements, the first far cheaper
// than the second, and asserts worstIndex lands on the expensive one.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import type { ConnectionSummary } from '@shared/domain/connection';
import type { ExecuteResponse } from '@shared/protocol/dataOps';
import { createTabularPageBuilder, type Page, unpagedPosition } from '@shared/protocol/page';
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
const { useConnectionsStore } = await import('../../../frontend/src/studio/state/connections');
const connectionsStore = useConnectionsStore();
const { useTabsStore } = await import('../../../frontend/src/state/tabs');
const tabsStore = useTabsStore();
const { useConsoleViewStore } = await import('../../../frontend/src/studio/views/console/state');
const consoleViewStore = useConsoleViewStore();

// Both clear the default 100,000-row threshold (packages/shared/domain/settings.ts) — flagged, but
// by a wide margin apart, so worstIndex has a real "which one is actually worse" to get right.
function scanPlanPage(rows: number): Page {
  const builder = createTabularPageBuilder([
    {
      name: 'QUERY PLAN',
      dataType: 'json',
      typeClass: 'text',
      nullable: false,
      isPrimaryKey: false,
      generated: false,
    },
  ]);
  builder.appendRow([
    JSON.stringify([
      { Plan: { 'Node Type': 'Seq Scan', 'Relation Name': 't', 'Plan Rows': rows } },
    ]),
  ]);
  return builder.finish(unpagedPosition(1));
}

function fakeResultPage(): Page {
  // biome-ignore lint/suspicious/noExplicitAny: minimal fake page, real run() only reads kind/rowCount
  return { kind: 'tabular', rowCount: 0 } as any;
}

describe('auto-explain worstIndex points at the worst flagged plan, not the first (P12 round 2 F13)', () => {
  test('a cheap-but-flagged statement first, a far more expensive one second', async () => {
    const connectionId = 'conn-auto-explain-worst';
    connectionsStore.records.push({
      id: connectionId,
      // biome-ignore lint/suspicious/noExplicitAny: a minimal fixture, not a real ConnectionSummary
      ...({ kind: 'postgres', autoExplain: true, name: 'x', color: 'blue' } as any),
    } as ConnectionSummary);
    const tabId = tabsStore.openConsoleTab(connectionId, 'db');

    // biome-ignore lint/suspicious/noExplicitAny: a minimal stub, not the real data.execute
    (data as any).execute = (): Promise<ExecuteResponse> =>
      Promise.resolve({
        pages: [
          scanPlanPage(150_000), // statement 0: flagged, mild
          scanPlanPage(9_000_000), // statement 1: flagged, far worse
          fakeResultPage(), // the real run's own result
        ],
      });

    await consoleViewStore.run(tabId, ['SELECT * FROM small_but_over', 'SELECT * FROM huge']);

    const state = consoleViewStore.runtime[tabId]?.autoExplain;
    expect(state?.kind).toBe('plans');
    if (state?.kind !== 'plans') throw new Error('expected a plans state');
    expect(state.plans).toHaveLength(2);
    expect(state.worstIndex).toBe(1);
    expect(state.plans[1]?.plan.estimatedRowsRead).toBe(9_000_000);
  });
});
