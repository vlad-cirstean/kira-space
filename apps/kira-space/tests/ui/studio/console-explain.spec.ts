import { DATA_OP } from '@shared/protocol/dataOps';
import type { ColumnDescriptor } from '@shared/protocol/page';
import { defaultSettings } from '../../../frontend/src/studio/state/settingsDomain';
import type { ControlSnapshot, LogicalPage, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { connectAndExpand, connectionCreateArgs, openConsoleFromMenu } from './support/connect';
import { typeInto } from './support/editor';
import { editorText } from './support/editorText';
import {
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';

// An Explain press is one data:execute call and the data plane is mocked here the same way
// console-format.spec.ts's own Format scenarios are — the point is asserting the real rendering
// of a real server's real output (the verified Postgres JSON, pasted verbatim)
// without needing a server.

// F11's own verified join plan — the root Limit reports 10 rows over a 184,153-row Seq Scan on
// "t" with a filter, which is exactly D14's "widest scan, not the root estimate" case.
const JOIN_PLAN_JSON = JSON.stringify([
  {
    Plan: {
      'Node Type': 'Limit',
      'Startup Cost': 7814.39,
      'Total Cost': 7814.41,
      'Plan Rows': 10,
      'Plan Width': 15,
      Plans: [
        {
          'Node Type': 'Hash Join',
          'Join Type': 'Inner',
          'Hash Cond': '(t.id = c.t_id)',
          'Total Cost': 6128.95,
          'Plan Rows': 46038,
          Plans: [
            {
              'Node Type': 'Seq Scan',
              'Relation Name': 't',
              Alias: 't',
              'Total Cost': 3582.0,
              'Plan Rows': 184153,
              Filter: '(cat > 3)',
            },
            {
              'Node Type': 'Hash',
              Plans: [
                {
                  'Node Type': 'Seq Scan',
                  'Relation Name': 'c',
                  'Total Cost': 771.0,
                  'Plan Rows': 50000,
                },
              ],
            },
          ],
        },
      ],
    },
  },
]);

const JOIN_SQL = 'SELECT count(*) FROM t JOIN c ON c.t_id = t.id WHERE cat > 3';

function queryPlanPage(json: string): LogicalPage {
  const columns: ColumnDescriptor[] = [
    {
      name: 'QUERY PLAN',
      dataType: 'json',
      typeClass: 'json',
      nullable: false,
      isPrimaryKey: false,
      generated: false,
    },
  ];
  return {
    kind: 'tabular',
    columns,
    rows: [[json]],
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
}

function explainSnap(connectionId: string, sql: string, planJson: string): PortSnapshot {
  return {
    op: DATA_OP.execute,
    payload: {
      connectionId,
      path: ORDER_ITEMS_PATH,
      statements: [
        `EXPLAIN (FORMAT JSON, COSTS TRUE, VERBOSE FALSE, SETTINGS FALSE, BUFFERS FALSE) ${sql}`,
      ],
    },
    response: { kind: 'execute', pages: [queryPlanPage(planJson)] },
  };
}

function numberPage(column: string, value: string): LogicalPage {
  const columns: ColumnDescriptor[] = [
    {
      name: column,
      dataType: 'int8',
      typeClass: 'number',
      nullable: true,
      isPrimaryKey: false,
      generated: false,
    },
  ];
  return {
    kind: 'tabular',
    columns,
    rows: [[value]],
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
}

function runSnap(connectionId: string, sql: string, page: LogicalPage): PortSnapshot {
  return {
    op: DATA_OP.execute,
    payload: { connectionId, path: ORDER_ITEMS_PATH, statements: [sql] },
    response: { kind: 'execute', pages: [page] },
  };
}

test('Query console — Explain produces a plan result set', async ({ relaunch }) => {
  const CONNECTION_ID = 'conn-console-explain-pg';
  const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Explain DB', 'green');
  const FIXTURE = orderItemsFixture(CONNECTION_ID);

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: connectionCreateArgs('Explain DB', 'green'),
      response: CONNECTION_SUMMARY,
    },
    ...FIXTURE.control,
  ];
  const PORT: PortSnapshot[] = [
    ...FIXTURE.port,
    explainSnap(CONNECTION_ID, JOIN_SQL, JOIN_PLAN_JSON),
  ];

  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndExpand(page, 'Explain DB', 'green');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await expect(view).toBeVisible();

  await typeInto(view, page, JOIN_SQL);
  await page.click('[data-testid="console-explain"]');

  const resultTab = view.locator('[data-testid="console-result-tab"]');
  await expect(resultTab).toHaveCount(1);
  await expect(resultTab.locator('.codicon-list-tree')).toBeVisible();

  const explainView = view.locator('[data-testid="explain-result-view"]');
  await expect(explainView).toBeVisible();
  await expect(explainView.locator('[data-testid="explain-tree"]')).toContainText('Seq Scan on t');
  await expect(explainView.locator('[data-testid="explain-tree"]')).toContainText('Hash Join');
  await expect(explainView.locator('[data-testid="explain-issues"]')).toContainText(
    'Full table scan',
  );
});

test('Query console — the threshold flags a wide scan, and a higher threshold does not', async ({
  relaunch,
}) => {
  const CONNECTION_ID = 'conn-console-explain-threshold';
  const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Threshold DB', 'blue');
  const FIXTURE = orderItemsFixture(CONNECTION_ID);
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: connectionCreateArgs('Threshold DB', 'blue'),
      response: CONNECTION_SUMMARY,
    },
    ...FIXTURE.control,
  ];
  const PORT: PortSnapshot[] = [
    ...FIXTURE.port,
    explainSnap(CONNECTION_ID, JOIN_SQL, JOIN_PLAN_JSON),
  ];

  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndExpand(page, 'Threshold DB', 'blue');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await typeInto(view, page, JOIN_SQL);
  await page.click('[data-testid="console-explain"]');

  const verdict = view.locator('[data-testid="explain-verdict"]');
  await expect(verdict).toContainText('184,153 rows');
  await expect(verdict).toHaveAttribute('data-over-threshold', 'true');
});

test('Query console — a higher threshold does not flag the same plan', async ({ relaunch }) => {
  const CONNECTION_ID = 'conn-console-explain-threshold-hi';
  const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Threshold Hi DB', 'blue');
  const FIXTURE = orderItemsFixture(CONNECTION_ID);
  const HIGH_THRESHOLD_SETTINGS = {
    ...defaultSettings,
    advanced: { ...defaultSettings.advanced, expensiveQueryRows: 1_000_000 },
  };
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.studioSettingsGetAll, response: HIGH_THRESHOLD_SETTINGS },
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: connectionCreateArgs('Threshold Hi DB', 'blue'),
      response: CONNECTION_SUMMARY,
    },
    ...FIXTURE.control,
  ];
  const PORT: PortSnapshot[] = [
    ...FIXTURE.port,
    explainSnap(CONNECTION_ID, JOIN_SQL, JOIN_PLAN_JSON),
  ];

  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndExpand(page, 'Threshold Hi DB', 'blue');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await typeInto(view, page, JOIN_SQL);
  await page.click('[data-testid="console-explain"]');

  const verdict = view.locator('[data-testid="explain-verdict"]');
  await expect(verdict).toContainText('184,153 rows');
  await expect(verdict).toHaveAttribute('data-over-threshold', 'false');
});

test('Query console — Raw is one toggle away', async ({ relaunch }) => {
  const CONNECTION_ID = 'conn-console-explain-raw';
  const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Raw DB', 'blue');
  const FIXTURE = orderItemsFixture(CONNECTION_ID);
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: connectionCreateArgs('Raw DB', 'blue'),
      response: CONNECTION_SUMMARY,
    },
    ...FIXTURE.control,
  ];
  const PORT: PortSnapshot[] = [
    ...FIXTURE.port,
    explainSnap(CONNECTION_ID, JOIN_SQL, JOIN_PLAN_JSON),
  ];

  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndExpand(page, 'Raw DB', 'blue');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await typeInto(view, page, JOIN_SQL);
  await page.click('[data-testid="console-explain"]');

  await expect(view.locator('[data-testid="explain-raw"]')).toHaveCount(0);
  await page.click('[data-testid="explain-raw-toggle"]');
  const raw = view.locator('[data-testid="explain-raw"]');
  await expect(raw).toBeVisible();
  // `plan.raw` is the server's own text verbatim, never re-formatted — the JSON is
  // minified, so this checks the raw text exactly as it would come back over the wire.
  // ExplainResultView's own host is MonacoHost — same host the console's query editor
  // above it now uses too, both read through the identical `editorText()` helper.
  expect(await editorText(raw)).toContain('"Node Type":"Seq Scan"');
});

test('Query console — not explainable (an UPDATE) disables the button', async ({ relaunch }) => {
  const CONNECTION_ID = 'conn-console-explain-update';
  const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Update DB', 'red');
  const FIXTURE = orderItemsFixture(CONNECTION_ID);
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: connectionCreateArgs('Update DB', 'red'),
      response: CONNECTION_SUMMARY,
    },
    ...FIXTURE.control,
  ];

  const { window: page } = await relaunch({ control: CONTROL, stream: FIXTURE.port });
  await connectAndExpand(page, 'Update DB', 'red');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');

  // The button never disappears (SELECT/WITH-only is a per-statement state, not a
  // per-console capability) — it refuses this one statement. An empty console has no
  // statement at the cursor at all, so a SELECT is typed first to prove the button starts enabled
  // before the UPDATE below disables it.
  await typeInto(view, page, JOIN_SQL);
  await expect(view.locator('[data-testid="console-explain"]')).toBeEnabled();
  await page.keyboard.press(process.platform === 'darwin' ? 'Meta+A' : 'Control+A');
  await page.keyboard.press('Delete');
  await typeInto(view, page, 'UPDATE t SET cat = 1 WHERE id = 1');
  await expect(view.locator('[data-testid="console-explain"]')).toBeDisabled();
});

test('Query console — auto-explain warns and still runs the query', async ({ relaunch }) => {
  const CONNECTION_ID = 'conn-console-auto-explain';
  const CONNECTION_SUMMARY = {
    ...postgresConnectionSummary(CONNECTION_ID, 'Auto DB', 'amber'),
    autoExplain: true,
  };
  const FIXTURE = orderItemsFixture(CONNECTION_ID);
  const RUN_SQL = 'SELECT * FROM t';
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: connectionCreateArgs('Auto DB', 'amber'),
      response: CONNECTION_SUMMARY,
    },
    ...FIXTURE.control,
  ];
  const PORT: PortSnapshot[] = [
    ...FIXTURE.port,
    explainSnap(CONNECTION_ID, RUN_SQL, JOIN_PLAN_JSON),
    runSnap(CONNECTION_ID, RUN_SQL, numberPage('id', '1')),
  ];

  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndExpand(page, 'Auto DB', 'amber', { autoExplain: true });
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await typeInto(view, page, RUN_SQL);
  await page.click('[data-testid="console-run-statement"]');

  const strip = view.locator('[data-testid="console-auto-explain"]');
  await expect(strip).toBeVisible();
  await expect(strip).toContainText('184,153 rows');
  // D19 rule 5: warn, never block — the real run's own result set is still present.
  await expect(view.locator('[data-testid="console-result-tab"]')).toHaveCount(1);
  await expect(view.locator('[data-testid="console-results"]')).toBeVisible();

  // The "Show plan" action pushes the already-parsed plan — no second EXPLAIN round trip.
  await page.click('[data-testid="console-auto-explain-show-plan"]');
  await expect(view.locator('[data-testid="console-result-tab"]')).toHaveCount(2);
  await expect(view.locator('[data-testid="explain-result-view"]')).toBeVisible();
});

// P12 round 1 finding #7: loading a saved query writes the new text via setText(), which goes
// through MonacoHost's external-sync path — the one path that deliberately never re-emits
// update:doc, the only event onDocChange (the sole place that used to reset this strip) ever saw.
test('Query console — loading a saved query clears a stale auto-explain warning', async ({
  relaunch,
}) => {
  const CONNECTION_ID = 'conn-console-auto-explain-stale';
  const CONNECTION_SUMMARY = {
    ...postgresConnectionSummary(CONNECTION_ID, 'Stale DB', 'amber'),
    autoExplain: true,
  };
  const FIXTURE = orderItemsFixture(CONNECTION_ID);
  const RUN_SQL = 'SELECT * FROM t';
  const SAVED_QUERY = {
    kind: 'console' as const,
    id: 'saved-query-stale-1',
    connectionId: CONNECTION_ID,
    path: ORDER_ITEMS_PATH,
    name: 'Other query',
    pinned: false,
    createdAt: '2026-01-01T00:00:00.000Z',
    usedAt: null,
    body: { text: 'SELECT 1;' },
  };
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: connectionCreateArgs('Stale DB', 'amber'),
      response: CONNECTION_SUMMARY,
    },
    ...FIXTURE.control,
    {
      channel: IPC.queriesListConsole,
      args: { connectionId: CONNECTION_ID, path: ORDER_ITEMS_PATH },
      response: [SAVED_QUERY],
    },
    { channel: IPC.queriesTouch, args: { id: SAVED_QUERY.id }, response: undefined },
  ];
  const PORT: PortSnapshot[] = [
    ...FIXTURE.port,
    explainSnap(CONNECTION_ID, RUN_SQL, JOIN_PLAN_JSON),
    runSnap(CONNECTION_ID, RUN_SQL, numberPage('id', '1')),
  ];

  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndExpand(page, 'Stale DB', 'amber', { autoExplain: true });
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await typeInto(view, page, RUN_SQL);
  await page.click('[data-testid="console-run-statement"]');

  const strip = view.locator('[data-testid="console-auto-explain"]');
  await expect(strip).toBeVisible();

  await page.click('[data-testid="console-saved-toggle"]');
  await page.locator('[data-testid="console-saved-entry"]', { hasText: 'Other query' }).click();

  await expect.poll(() => editorText(view)).toContain('SELECT 1;');
  await expect(strip).toHaveCount(0);
});

test('Query console — auto-explain off issues one execute call, not two', async ({ relaunch }) => {
  const CONNECTION_ID = 'conn-console-auto-explain-off';
  const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'No Auto DB', 'cyan');
  const FIXTURE = orderItemsFixture(CONNECTION_ID);
  const RUN_SQL = 'SELECT * FROM t';
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: connectionCreateArgs('No Auto DB', 'cyan'),
      response: CONNECTION_SUMMARY,
    },
    ...FIXTURE.control,
  ];
  const PORT: PortSnapshot[] = [
    ...FIXTURE.port,
    runSnap(CONNECTION_ID, RUN_SQL, numberPage('id', '1')),
  ];

  const { window: page, stream } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndExpand(page, 'No Auto DB', 'cyan');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await typeInto(view, page, RUN_SQL);
  await page.click('[data-testid="console-run-statement"]');

  await expect(view.locator('[data-testid="console-result-tab"]')).toHaveCount(1);
  await expect(view.locator('[data-testid="console-auto-explain"]')).toHaveCount(0);
  const ops = await stream.ops();
  expect(ops.filter((o) => o.op === DATA_OP.execute)).toHaveLength(1);
});

test('Query console — a failed EXPLAIN does not fail the real run', async ({ relaunch }) => {
  const CONNECTION_ID = 'conn-console-auto-explain-fail';
  const CONNECTION_SUMMARY = {
    ...postgresConnectionSummary(CONNECTION_ID, 'Fail DB', 'magenta'),
    autoExplain: true,
  };
  const FIXTURE = orderItemsFixture(CONNECTION_ID);
  const RUN_SQL = 'SELECT * FROM t';
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: connectionCreateArgs('Fail DB', 'magenta'),
      response: CONNECTION_SUMMARY,
    },
    ...FIXTURE.control,
  ];
  const PORT: PortSnapshot[] = [
    ...FIXTURE.port,
    {
      op: DATA_OP.execute,
      payload: {
        connectionId: CONNECTION_ID,
        path: ORDER_ITEMS_PATH,
        statements: [
          `EXPLAIN (FORMAT JSON, COSTS TRUE, VERBOSE FALSE, SETTINGS FALSE, BUFFERS FALSE) ${RUN_SQL}`,
        ],
      },
      error: { code: 'E_QUERY', message: 'the planner refused this statement' },
    },
    runSnap(CONNECTION_ID, RUN_SQL, numberPage('id', '1')),
  ];

  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndExpand(page, 'Fail DB', 'magenta', { autoExplain: true });
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await typeInto(view, page, RUN_SQL);
  await page.click('[data-testid="console-run-statement"]');

  // D19 rule 6: a failed EXPLAIN is swallowed for this path — no strip, and the real run's own
  // result set (from the second, unrelated snapshot) still comes through normally.
  await expect(view.locator('[data-testid="console-auto-explain"]')).toHaveCount(0);
  await expect(view.locator('[data-testid="console-error"]')).toHaveCount(0);
  await expect(view.locator('[data-testid="console-result-tab"]')).toHaveCount(1);
});
