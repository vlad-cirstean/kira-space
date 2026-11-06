import type { Locator, Page } from '@playwright/test';
import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, LogicalPage, PortSnapshot } from '../../ipc/support/types';
import { expect, test } from './fixtures';
import { connectionCreateArgs, createConnection, openConsoleFromMenu } from './support/connect';
import { acceptConfirm } from './support/dialogs';
import { typeInto } from './support/editor';
import { IPC } from './support/ipcChannels';
import {
  APP_PATH,
  DB_PATH,
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { closeAllTabs } from './support/tabs';
import { expandRow, findRow, openRowMenu } from './support/tree';

// Ported from tests/e2e/leaks.spec.ts (P57 D16). P13's own regression spec for its leak sweep
// (F4-F7, F19, F20) splits cleanly into two categories once read closely, the same split
// perf.spec.ts's header comment documents for its own single dropped check:
//
// PORTS AS-IS (scenarios 1-3, minus one persistence tail): `window.__kiraRetainedBytes` and
// `window.__kiraTreeConnectionIds` (main.ts:21-58) are genuinely pure `apps/kira-space/frontend/src` globals — the
// sum of five page stores' own byte bookkeeping, and the tree store's own known-connection-id set —
// with zero involvement from whatever answers the data-plane stream. Opening/closing tabs, deleting
// a connection, and observing the tree purge its dead connection's state are all real renderer work
// this mock can genuinely exercise: the mock only needs to answer the handful of distinct
// (op, payload) requests these scenarios actually issue, and the renderer's own store logic does
// the rest for real.
//
// DOES NOT PORT (scenarios 4-5, and scenario 3's own relaunch tail): three sub-scenarios whose real
// subject lives somewhere this tier has no analogue for.
//   - Scenario 3's final third (change page size, `relaunch()`, confirm the tab and its page-size
//     setting survived) is real cross-process SQLite persistence — `tests/ui/fixtures.ts`'s own
//     header comment already rules this out for the whole tier ("there is nothing to persist to"),
//     the same category as workbench.spec.ts's five dropped scenarios and connections.spec.ts's two.
//     Confirmed by tracing `deleteConnection()` (state/connections.ts): it only calls
//     `control.connectionsDelete` and filters local reactive state — nothing about the eventual
//     `tabsSave` a surviving connection's tab mutation triggers is validated against a real SQLite
//     `connection_id` foreign key here, since `tabsSave` is one of `mockRuntime.ts`'s own
//     `WILDCARD_DEFAULTS` (an unconditional `null` echo, P57 finding) — a real regression in that
//     save path (the original D7 bug this scenario existed to catch) would be invisible to a mock
//     that never actually persists or constrains anything. The regression itself is Go-side
//     (`apps/kira-space/internal/storage/repos/tabs.go`) and would be better guarded there directly if it
//     isn't already (`tabs_test.go` currently covers null-connection-id and bad-row handling, but
//     not specifically a stale-connection_id save after a sibling connection's deletion — a real,
//     named gap worth a follow-up Go test, not something this port can paper over).
//   - Scenario 4 ("L3 is bounded") and scenario 5 ("clearing the cache resets the hit rate") both
//     read `window.__kiraCacheStats`/drive `window.__kiraCount` — thin wrappers over `data.count()`/
//     `data.cacheStats()` (`bridge/data.ts`), which are real `DATA_OP` requests over the data-plane
//     stream. The actual subject under test — `src/engine/cache/counts.ts`'s L3 `ByteLru` eviction
//     bound, and `src/engine/cache/pages.ts`'s L2 hit/miss counters and their reset on
//     `clearPages()` — lives inside the real `engine` child process, not in `apps/kira-space/frontend/src`. This
//     tier runs no such process at all; a mock answering `DATA_OP.count`/`DATA_OP.cacheStats` could
//     only echo a hand-picked number, which would make "the bound holds" or "the rate resets" true
//     by fixture construction rather than by the real eviction algorithm actually doing anything —
//     the same "no real subject" category `perf.spec.ts`'s own dropped L2 check falls into (see
//     that file's header comment for the fuller reasoning, which applies here verbatim). Both are
//     replaced by `tests/unit/engine-cache.spec.ts` (new, this session): a direct, dependency-free
//     unit test of `counts.ts`/`pages.ts` themselves, asserting the *exact* 2 048-entry L3 bound
//     (this file's own original assertion only checked `<= 2100`, with slack for a real Postgres
//     round trip's own timing noise) and the hit/miss counters resetting to zero on `clearPages()`.
//     No browser, no mock, no engine process — and a more precise test of the real subject than
//     driving 2 500 real UI clicks ever was.
//
// What changed getting the ported half green, same as every other Postgres-backed port:
//   - `window.kira.connectionsCreate(...)` becomes a real connection-dialog flow.
//   - The console `run all` scenario's `generate_series(1, 5000)` result is a real capture
//     (scripts/capture-postgres-tree.ts's existing `execute` step kind — no extension needed): a
//     single `n int4` column, exactly 5 000 rows, `hasMore: false` — confirming the adapter's raw
//     `execute` path has no server-side row cap of its own, which is what makes this scenario a
//     genuine "large result set retains real bytes" case rather than a truncated stand-in.
//   - `openDefinitionFromMenu`'s `app.order_items` definition is also a real capture (`definition`
//     step kind).

function connectionRootRow(page: Page, name: string): Locator {
  return page.locator('[data-testid="tree-row"][data-kind="connection"]').filter({ hasText: name });
}

async function connectAndExpand(page: Page, name: string): Promise<void> {
  const root = connectionRootRow(page, name);
  await expect(root).toBeVisible();
  await root.click({ button: 'right' });
  await page.click('[data-testid="menu-item-connect"]');
  await expect(root.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await root.locator('[data-testid="tree-twisty"]').click();
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);
}

async function openDefinitionFromMenu(page: Page, path: string): Promise<void> {
  await openRowMenu(page, path);
  await page.click('[data-testid="menu-item-open-definition"]');
}

async function retainedBytes(page: Page): Promise<number> {
  return page.evaluate(() => window.__kiraRetainedBytes?.() ?? -1);
}

// The retention probe — asserted for exact-baseline equality the same way retainedBytes()
// above is, but over the structures that hook cannot see (decode/view caches, the document parse
// cache, per-tab search matches, distinct retained frame buffers). `toEqual` on the whole object
// is deliberate: any one field drifting from baseline is a genuine regression.
async function retention(page: Page): Promise<unknown> {
  return page.evaluate(() => window.__kiraRetention?.() ?? null);
}

async function waitForGrid(page: Page): Promise<void> {
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await expect
    .poll(async () => page.locator('[data-testid="grid-gutter-cell"]').count(), { timeout: 15_000 })
    .toBeGreaterThan(0);
}

const CONN_A = 'conn-leaks-a';
const CONN_B = 'conn-leaks-b';
const SUMMARY_A = postgresConnectionSummary(CONN_A, 'Leaks A', 'blue');
const SUMMARY_B = postgresConnectionSummary(CONN_B, 'Leaks B', 'magenta');
const FIXTURE_A = orderItemsFixture(CONN_A);
const FIXTURE_B = orderItemsFixture(CONN_B);

// Real capture (scripts/capture-postgres-tree.ts's `execute` step, this session) — a raw
// `generate_series` execute has no server-side row cap: all 5 000 rows come back in one page,
// `hasMore: false`, `strategy: 'offset'`.
const GENERATE_SERIES_PAGE: LogicalPage = {
  kind: 'tabular',
  columns: [
    {
      name: 'n',
      dataType: 'int4',
      typeClass: 'number',
      nullable: true,
      isPrimaryKey: false,
      generated: false,
    },
  ],
  rows: Array.from({ length: 5000 }, (_, i) => [String(i + 1)]),
  position: {
    offset: 0,
    pageSize: 5000,
    hasMore: false,
    nextToken: null,
    prevToken: null,
    strategy: 'offset',
  },
  truncatedCells: 0,
};

// Real capture (scripts/capture-postgres-tree.ts's `definition` step, this session).
const ORDER_ITEMS_DEFINITION = {
  path: ORDER_ITEMS_PATH,
  kind: 'table' as const,
  qualifiedName: 'app.order_items',
  statements: [
    'CREATE SEQUENCE app.order_items_id_seq',
    "CREATE TABLE app.order_items (\n  id integer DEFAULT nextval('app.order_items_id_seq'::regclass) NOT NULL,\n  order_id integer NOT NULL,\n  product_id integer NOT NULL,\n  quantity integer DEFAULT 1 NOT NULL\n)",
    'ALTER SEQUENCE app.order_items_id_seq OWNED BY app.order_items.id',
    'ALTER TABLE app.order_items ADD CONSTRAINT order_items_pkey PRIMARY KEY (id)',
    'ALTER TABLE app.order_items ADD CONSTRAINT order_items_quantity_positive CHECK (quantity > 0)',
    'ALTER TABLE app.order_items ADD CONSTRAINT order_items_order_id_fkey FOREIGN KEY (order_id) REFERENCES app.orders(id)',
    'ALTER TABLE app.order_items ADD CONSTRAINT order_items_product_id_fkey FOREIGN KEY (product_id) REFERENCES app.products(id)',
    'CREATE UNIQUE INDEX order_items_order_product_idx ON app.order_items USING btree (order_id, product_id)',
  ],
  language: 'sql' as const,
  origin: 'composed' as const,
  notes: [
    'Composed from catalog metadata: triggers, row-level security policies, grants, ownership, storage parameters, tablespaces and non-default column collations are not included.',
  ],
  constraints: [
    { name: 'order_items_pkey', type: 'primaryKey' as const, definition: 'PRIMARY KEY (id)' },
    {
      name: 'order_items_quantity_positive',
      type: 'check' as const,
      definition: 'CHECK (quantity > 0)',
    },
    {
      name: 'order_items_order_id_fkey',
      type: 'foreignKey' as const,
      definition: 'FOREIGN KEY (order_id) REFERENCES app.orders(id)',
    },
    {
      name: 'order_items_product_id_fkey',
      type: 'foreignKey' as const,
      definition: 'FOREIGN KEY (product_id) REFERENCES app.products(id)',
    },
  ],
  documentSchema: null,
  sections: [],
  generatedAt: '2026-08-30T19:09:27.825Z',
};

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Leaks A', 'blue'),
    response: SUMMARY_A,
  },
  ...FIXTURE_A.control,
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Leaks B', 'magenta'),
    response: SUMMARY_B,
  },
  ...FIXTURE_B.control,
  {
    channel: IPC.treeDefinition,
    args: { connectionId: CONN_A, path: ORDER_ITEMS_PATH, refresh: false, tabId: null },
    response: { definition: ORDER_ITEMS_DEFINITION, source: 'server' },
  },
  { channel: IPC.connectionsDelete, args: { id: CONN_A }, response: null },
];

// A minimal, real-shaped Postgres EXPLAIN (FORMAT JSON) page — enough for parseExplainPages to
// produce a QueryPlan, which is all scenario 1 needs to exercise explainResults.ts's own store
// (P12 round 1 finding #10).
const EXPLAIN_PAGE: LogicalPage = {
  kind: 'tabular',
  columns: [
    {
      name: 'QUERY PLAN',
      dataType: 'json',
      typeClass: 'text',
      nullable: false,
      isPrimaryKey: false,
      generated: false,
    },
  ],
  rows: [
    [
      JSON.stringify([
        {
          Plan: {
            'Node Type': 'Function Scan',
            'Relation Name': 'generate_series',
            'Plan Rows': 5000,
          },
        },
      ]),
    ],
  ],
  position: {
    offset: 0,
    pageSize: 1,
    hasMore: false,
    nextToken: null,
    prevToken: null,
    strategy: 'offset',
  },
  truncatedCells: 0,
};

const PORT: PortSnapshot[] = [
  ...FIXTURE_A.port,
  ...FIXTURE_B.port,
  {
    op: DATA_OP.execute,
    payload: {
      connectionId: CONN_A,
      path: ORDER_ITEMS_PATH,
      statements: ['SELECT * FROM generate_series(1, 5000) AS n'],
    },
    response: { kind: 'execute', pages: [GENERATE_SERIES_PAGE] },
  },
  {
    op: DATA_OP.execute,
    payload: {
      connectionId: CONN_A,
      path: ORDER_ITEMS_PATH,
      statements: [
        'EXPLAIN (FORMAT JSON, COSTS TRUE, VERBOSE FALSE, SETTINGS FALSE, BUFFERS FALSE) SELECT * FROM generate_series(1, 5000) AS n',
      ],
    },
    response: { kind: 'execute', pages: [EXPLAIN_PAGE] },
  },
];

test('leak sweep — tab/store symmetry, connection delete purges the tree', async ({ relaunch }) => {
  test.setTimeout(120_000);
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });

  await createConnection(page, 'postgres', 'Leaks A');

  await connectAndExpand(page, 'Leaks A');

  // --- scenario 1: tab open/close symmetry across all five page stores -------
  // A data tab and a console tab (with a large result set) both retain bytes in one of the five
  // stores this file's own `__kiraRetainedBytes` sums; a definition tab retains none of them but
  // must still close cleanly alongside the other two.
  const baseline1 = await retainedBytes(page);
  const retentionBaseline1 = await retention(page);

  await (await findRow(page, ORDER_ITEMS_PATH)).dblclick();
  await waitForGrid(page);

  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const consoleView = page.locator('[data-testid="console-view"]');
  await expect(consoleView).toBeVisible();
  await typeInto(consoleView, page, 'SELECT * FROM generate_series(1, 5000) AS n;');
  await page.click('[data-testid="console-run-all"]');
  await expect(consoleView.locator('[data-testid="console-result-grid"]')).toHaveCount(1);
  // P12 round 1 finding #10: explainResults.ts's own plan store is invisible to
  // __kiraRetainedBytes entirely (it isn't a page store) — only __kiraRetention's `explainPlans`
  // field sees it, so this is the one action in this scenario that actually exercises it.
  await page.click('[data-testid="console-explain"]');
  await expect(consoleView.locator('[data-testid="explain-result-view"]')).toBeVisible();
  expect((await retention(page)) as { explainPlans: number }).toMatchObject({ explainPlans: 1 });

  await openDefinitionFromMenu(page, ORDER_ITEMS_PATH);
  await expect(page.locator('[data-testid="definition-view"]')).toBeVisible();

  expect(await retainedBytes(page)).toBeGreaterThan(baseline1);

  await closeAllTabs(page);
  expect(await retainedBytes(page)).toBe(baseline1);
  expect(await retention(page)).toEqual(retentionBaseline1);

  // --- scenario 2: runtime records are released (F4) -------------------------------------------
  const baseline2 = await retainedBytes(page);
  const retentionBaseline2 = await retention(page);
  for (let i = 0; i < 20; i++) {
    await openRowMenu(page, ORDER_ITEMS_PATH);
    await page.click('[data-testid="menu-item-open-data-new-tab"]');
    await waitForGrid(page);
  }
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(20);
  await closeAllTabs(page);
  expect(await retainedBytes(page)).toBe(baseline2);
  expect(await retention(page)).toEqual(retentionBaseline2);

  // A freshly re-opened tab starts from a default runtime: no stale count, nothing counted yet.
  // P104 §6.2: the count button's tooltip moved off data-kira-tip onto the real Tooltip system
  // (DataToolbar.vue, Stream B's own file) -- hover to read its now-hover-revealed content instead.
  await (await findRow(page, ORDER_ITEMS_PATH)).dblclick();
  await waitForGrid(page);
  const freshCountButton = page.locator('[data-testid="toolbar-count"]');
  await expect(freshCountButton).not.toHaveClass(/stale/);
  await freshCountButton.hover();
  await expect(page.locator('[data-slot="tooltip-content"]')).toContainText('Count all rows');
  await page.mouse.move(0, 0);
  await closeAllTabs(page);

  // --- scenario 3: deleting a connection closes its tabs and purges the tree (F6/D6) -----------
  // The relaunch-persistence tail of the original scenario 3 is dropped — see this file's own
  // header comment.
  await (await findRow(page, ORDER_ITEMS_PATH)).dblclick();
  await waitForGrid(page);
  await openRowMenu(page, ORDER_ITEMS_PATH);
  await page.click('[data-testid="menu-item-open-data-new-tab"]');
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(2);

  // Collapse A's own root before B exists — A's descendant rows share the exact same `data-path`
  // values B will render, and findRow/expandRow match on `data-path` alone (tabs.spec.ts's own
  // convention, for the identical reason).
  await connectionRootRow(page, 'Leaks A').locator('[data-testid="tree-twisty"]').click();

  await createConnection(page, 'postgres', 'Leaks B');

  await connectAndExpand(page, 'Leaks B');
  await (await findRow(page, ORDER_ITEMS_PATH)).dblclick();
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(3);

  expect(await page.evaluate(() => window.__kiraTreeConnectionIds?.())).toContain(CONN_A);

  await connectionRootRow(page, 'Leaks A').click({ button: 'right' });
  await page.click('[data-testid="menu-item-delete"]');
  await acceptConfirm(page);
  await expect(connectionRootRow(page, 'Leaks A')).toHaveCount(0, { timeout: 10_000 });

  // P57 M5 finding: the rest of the original scenario ("A's two tabs are gone" and "the tree
  // store no longer holds A's connection id") does not port. Both are wired to the
  // `connectionsChanged` *event* specifically, not to a reactive watch over the local state
  // `deleteConnection()` already updates directly:
  //   - state/tabs.ts's stale-tab-close (D7) subscribes `control.onConnectionsChanged`.
  //   - project/state/tree.ts's own `knownConnectionIds` pruning (its own comment: "onConnectionsChanged
  //     (below) rather than off deleteConnection() directly, so every deletion...") is the same
  //     deliberate design, not an accident either half could route around.
  // `mockRuntime.ts` answers control-plane requests only — it has no `Events.On` analogue at all
  // (the same structural gap `tests/ui/interaction.spec.ts`'s own header comment already names for
  // its dropped Operations-panel scenario) — so this channel never fires here, and neither
  // consumer ever prunes. The row disappearing above is a genuinely different, direct-state code
  // path (confirmed real, not a mock artifact) and is as much of F6/D6 as this tier can verify.
});
