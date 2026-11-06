// Postgres/MySQL/MariaDB's JSON-format EXPLAIN returns the whole plan as
// one cell. A plan over MAX_CELL_BYTES (64 KiB) arrives clipped mid-JSON on the wire, with
// `isTruncated` set on the cell — parseExplainPages used to ignore that flag entirely and hand
// the (invalid, truncated) JSON straight to JSON.parse, which fails with an opaque "Unexpected end
// of JSON input" instead of a real, actionable message.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import type { ConnectionSummary } from '@shared/domain/connection';
import { createTabularPageBuilder, MAX_CELL_BYTES } from '@shared/protocol/page';
import { restoreAfterEach } from '@workbench/testing/unit/restoreAfterEach';
import { setActivePinia } from 'pinia';
import { pinia } from '../../../frontend/src/state/pinia';
import {
  ExplainTruncatedError,
  parseExplainPages,
} from '../../../frontend/src/studio/views/console/plan';

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

const PLAN_COLUMN = [
  {
    name: 'QUERY PLAN',
    dataType: 'json',
    typeClass: 'text' as const,
    nullable: false,
    isPrimaryKey: false,
    generated: false,
  },
];

// Deliberately not valid JSON once truncated — a well-formed plan under the limit stays whole and
// parses normally; this is what a real over-the-wire clip looks like.
function truncatedPlanPage() {
  const builder = createTabularPageBuilder(PLAN_COLUMN);
  const oversized = `[{"Plan":${JSON.stringify({ x: 'y'.repeat(MAX_CELL_BYTES) })}}]`;
  builder.appendRow([oversized]);
  return builder.finish({
    offset: 0,
    pageSize: 1,
    hasMore: false,
    nextToken: null,
    prevToken: null,
    strategy: 'offset',
  });
}

describe('parseExplainPages — truncated cell (P12 round 1 F8)', () => {
  for (const kind of ['postgres', 'mysql', 'mariadb', 'clickhouse'] as const) {
    test(`${kind}: a truncated plan cell throws ExplainTruncatedError, not a JSON parse error`, () => {
      const page = truncatedPlanPage();
      expect(page.truncatedCells).toBe(1); // sanity: the fixture really did clip on the wire
      expect(() => parseExplainPages(kind, [page], 100_000)).toThrow(ExplainTruncatedError);
    });
  }
});

describe('a truncated plan end to end (P12 round 1 F8)', () => {
  test('auto-explain shows a "could not check" strip instead of nothing, and the real run still executes', async () => {
    const connectionId = 'conn-explain-truncated-auto';
    connectionsStore.records.push({
      id: connectionId,
      // biome-ignore lint/suspicious/noExplicitAny: a minimal fixture, not a real ConnectionSummary
      ...({ kind: 'postgres', autoExplain: true, name: 'x', color: 'blue' } as any),
    } as ConnectionSummary);
    const tabId = tabsStore.openConsoleTab(connectionId, 'db');

    let call = 0;
    // biome-ignore lint/suspicious/noExplicitAny: a minimal stub, not the real data.execute
    (data as any).execute = () => {
      call++;
      // The pre-run EXPLAIN batch's own page comes back truncated; the real run's own page
      // (issued afterward, since D19 rule 5 says a check failure never blocks the query) is fine.
      if (call === 1) return Promise.resolve({ pages: [truncatedPlanPage()] });
      return Promise.resolve({ pages: [{ kind: 'tabular', rowCount: 0 }] });
    };

    await consoleViewStore.run(tabId, ['SELECT * FROM big']);

    expect(consoleViewStore.runtime[tabId]?.autoExplain).toEqual({ kind: 'truncated' });
    expect(consoleViewStore.runtime[tabId]?.status).toBe('idle'); // the real query still ran
    expect(call).toBe(2);
  });

  test('the manual Explain button surfaces a real message, not a raw JSON parse error', async () => {
    const connectionId = 'conn-explain-truncated-manual';
    connectionsStore.records.push({
      id: connectionId,
      // biome-ignore lint/suspicious/noExplicitAny: a minimal fixture, not a real ConnectionSummary
      ...({ kind: 'postgres', autoExplain: false, name: 'y', color: 'blue' } as any),
    } as ConnectionSummary);
    const tabId = tabsStore.openConsoleTab(connectionId, 'db');

    // biome-ignore lint/suspicious/noExplicitAny: a minimal stub, not the real data.execute
    (data as any).execute = () => Promise.resolve({ pages: [truncatedPlanPage()] });

    const result = await consoleViewStore.explain(tabId, 'postgres', 'SELECT * FROM big');
    expect(result).toEqual({ ok: false, reason: 'The query plan was too large to display.' });
  });
});
