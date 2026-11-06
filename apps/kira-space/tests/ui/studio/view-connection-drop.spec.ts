import { DATA_OP } from '@shared/protocol/dataOps';
import {
  controlSnapshots as kafkaControl,
  portSnapshots as kafkaPorts,
} from '../../ipc/kafka/kafka.fixture';
import { controlSnapshots as sqsControl, portSnapshots as sqsPorts } from '../../ipc/sqs/sqs.fixture';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, test } from './fixtures';
import {
  connectAndExpand,
  connectionCreateArgs,
  disconnectConnection,
  openConsoleFromMenu,
} from './support/connect';
import { executeSnap, numberPage } from './support/consolePages';
import { typeInto } from './support/editor';
import { IPC } from './support/ipcChannels';
import {
  connectionsDisconnectSnapshot,
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';

// Stream, Definition and Console results keep their data on a connection drop, behind the stale
// strip. Stream writes lock; Definition and Console have no write path.

const KAFKA_TOPIC_PATH = 'topic:orders';
const SQS_QUEUE_PATH = 'queue:orders-queue';

async function connectFixture(page: import('@playwright/test').Page): Promise<void> {
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
}

test('a kafka stream keeps its messages and locks Produce after a drop', async ({ relaunch }) => {
  const control: ControlSnapshot[] = [
    ...(kafkaControl as ControlSnapshot[]),
    connectionsDisconnectSnapshot('test-kafka'),
  ];
  const { window: page, stream } = await relaunch({ control, stream: kafkaPorts });
  await connectFixture(page);
  await (await findRow(page, KAFKA_TOPIC_PATH)).dblclick();
  const view = page.locator(`[data-testid="stream-view"][data-path="${KAFKA_TOPIC_PATH}"]`);
  await expect(view.locator('[data-testid="stream-row"]').first()).toBeVisible({
    timeout: 15_000,
  });
  await expect(view.locator('[data-testid="stream-add-message"]')).toBeEnabled();

  await disconnectConnection(page);

  await expect(view.locator('[data-testid="stream-stale"]')).toBeVisible();
  await expect(view.locator('[data-testid="stream-reconnect"]')).toHaveCount(0);
  await expect(view.locator('[data-testid="stream-row"]').first()).toBeVisible();
  await expect(view.locator('[data-testid="stream-add-message"]')).toBeDisabled();

  await view.locator('[data-testid="stream-stale-reconnect"]').click();
  await expect(view.locator('[data-testid="stream-stale"]')).toHaveCount(0);
  await expect(view.locator('[data-testid="stream-add-message"]')).toBeEnabled();

  expect((await stream.ops()).filter((o) => o.op === DATA_OP.mutate)).toHaveLength(0);
});

test('an sqs stream locks Send and Delete after a drop and keeps the polled messages', async ({
  relaunch,
}) => {
  const control: ControlSnapshot[] = [
    ...(sqsControl as ControlSnapshot[]),
    connectionsDisconnectSnapshot('test-sqs'),
  ];
  const { window: page, stream } = await relaunch({ control, stream: sqsPorts });
  await connectFixture(page);
  await (await findRow(page, SQS_QUEUE_PATH)).dblclick();
  const view = page.locator(`[data-testid="stream-view"][data-path="${SQS_QUEUE_PATH}"]`);
  await view.locator('[data-testid="stream-poll"]').click();
  await expect(view.locator('[data-testid="stream-row"]').first()).toBeVisible({
    timeout: 15_000,
  });
  await view.locator('[data-testid="stream-body"]').first().click();
  await expect(view.locator('[data-testid="stream-delete-message"]')).toBeEnabled();

  await disconnectConnection(page);

  await expect(view.locator('[data-testid="stream-stale"]')).toBeVisible();
  await expect(view.locator('[data-testid="stream-row"]').first()).toBeVisible();
  await expect(view.locator('[data-testid="stream-add-message"]')).toBeDisabled();
  await expect(view.locator('[data-testid="stream-delete-message"]')).toBeDisabled();
  await view.locator('[data-testid="stream-body"]').nth(1).click();
  await expect(page.locator('[data-testid="cell-editor-panel"]')).toBeVisible();

  expect((await stream.ops()).filter((o) => o.op === DATA_OP.mutate)).toHaveLength(0);
});

const PG_ID = 'conn-view-drop';
const PG_FIXTURE = orderItemsFixture(PG_ID);
const PG_CONTROL_BASE: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('View Drop DB', 'green'),
    response: postgresConnectionSummary(PG_ID, 'View Drop DB', 'green'),
  },
  ...PG_FIXTURE.control,
  connectionsDisconnectSnapshot(PG_ID),
];

test('a definition keeps its content read-only after a drop', async ({ relaunch }) => {
  const definition = {
    path: ORDER_ITEMS_PATH,
    kind: 'table' as const,
    qualifiedName: 'app.order_items',
    statements: ['CREATE TABLE app.order_items (\n  id integer NOT NULL\n)'],
    language: 'sql' as const,
    origin: 'composed' as const,
    notes: [],
    constraints: [],
    documentSchema: null,
    sections: [],
    generatedAt: '2026-01-01T00:00:00.000Z',
  };
  const control: ControlSnapshot[] = [
    ...PG_CONTROL_BASE,
    {
      channel: IPC.treeDefinition,
      args: { connectionId: PG_ID, path: ORDER_ITEMS_PATH, refresh: false, tabId: null },
      response: { definition, source: 'server' },
    },
  ];
  const { window: page } = await relaunch({ control, stream: PG_FIXTURE.port });
  await connectAndExpand(page, 'View Drop DB', 'green');
  await openRowMenu(page, ORDER_ITEMS_PATH);
  await page.click('[data-testid="menu-item-open-definition"]');
  const view = page.locator('[data-testid="definition-view"]');
  await expect(view).toHaveAttribute('data-origin', 'composed', { timeout: 15_000 });
  await view.locator('[data-testid="definition-pane-source"]').click();
  await expect(view.locator('.monaco-host')).toContainText('order_items');

  await disconnectConnection(page);

  await expect(view.locator('[data-testid="definition-stale"]')).toBeVisible();
  await expect(view.locator('[data-testid="definition-reconnect"]')).toHaveCount(0);
  await expect(view.locator('.monaco-host')).toContainText('order_items');

  await view.locator('[data-testid="definition-stale-reconnect"]').click();
  await expect(view.locator('[data-testid="definition-stale"]')).toHaveCount(0);
});

test('console results stay selectable after a drop and a new run clears the strip', async ({
  relaunch,
}) => {
  const port = [
    ...PG_FIXTURE.port,
    executeSnap(PG_ID, ['SELECT 10 AS a'], [numberPage('a', '10')]),
  ];
  const { window: page } = await relaunch({ control: PG_CONTROL_BASE, stream: port });
  await connectAndExpand(page, 'View Drop DB', 'green');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await typeInto(view, page, 'SELECT 10 AS a;');
  await page.click('[data-testid="console-run-statement"]');
  const grid = view.locator('[data-testid="console-result-grid"]');
  await expect(grid.first()).toContainText('10');

  await disconnectConnection(page);

  await expect(view.locator('[data-testid="console-stale"]')).toBeVisible();
  await expect(grid.first()).toContainText('10');
  await grid.locator('[data-testid="console-result-cell"][data-col-index="0"]').first().click();
  await expect(page.locator('[data-testid="cell-editor-panel"]')).toBeVisible();

  await page.click('[data-testid="console-run-statement"]');
  await expect(view.locator('[data-testid="console-stale"]')).toHaveCount(0);
  await expect(grid.first()).toContainText('10');
});

// A restored tab was never loaded: it shows only the reconnect gate, never the stale strip.
function restoredTab(kind: string, connectionId: string, path: string, state: object) {
  return { id: `tab-restored-${kind}`, connectionId, path, kind, order: 0, active: true, state };
}

test('a restored stream tab shows the reconnect gate without the stale strip', async ({
  relaunch,
}) => {
  const control: ControlSnapshot[] = [
    ...(kafkaControl as ControlSnapshot[]),
    {
      channel: IPC.tabsList,
      response: [restoredTab('stream', 'test-kafka', KAFKA_TOPIC_PATH, {})],
    },
  ];
  const { window: page } = await relaunch({ control, stream: kafkaPorts });
  const view = page.locator(`[data-testid="stream-view"][data-path="${KAFKA_TOPIC_PATH}"]`);
  await expect(view.locator('[data-testid="stream-reconnect"]')).toBeVisible();
  await expect(view.locator('[data-testid="stream-stale"]')).toHaveCount(0);
});

test('a restored definition tab shows the reconnect gate without the stale strip', async ({
  relaunch,
}) => {
  const control: ControlSnapshot[] = [
    {
      channel: IPC.connectionsList,
      response: [postgresConnectionSummary(PG_ID, 'View Drop DB', 'green')],
    },
    { channel: IPC.tabsList, response: [restoredTab('definition', PG_ID, ORDER_ITEMS_PATH, {})] },
  ];
  const { window: page } = await relaunch({ control, stream: PG_FIXTURE.port });
  const view = page.locator('[data-testid="definition-view"]');
  await expect(view.locator('[data-testid="definition-reconnect"]')).toBeVisible();
  await expect(view.locator('[data-testid="definition-stale"]')).toHaveCount(0);
});

test('console reconnect clears the strip and keeps the results without rerunning', async ({
  relaunch,
}) => {
  const port = [
    ...PG_FIXTURE.port,
    executeSnap(PG_ID, ['SELECT 10 AS a'], [numberPage('a', '10')]),
  ];
  const { window: page, stream } = await relaunch({ control: PG_CONTROL_BASE, stream: port });
  await connectAndExpand(page, 'View Drop DB', 'green');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await typeInto(view, page, 'SELECT 10 AS a;');
  await page.click('[data-testid="console-run-statement"]');
  const grid = view.locator('[data-testid="console-result-grid"]');
  await expect(grid.first()).toContainText('10');
  const runs = async () => (await stream.ops()).filter((o) => o.op === DATA_OP.execute).length;
  const before = await runs();

  await disconnectConnection(page);
  await expect(view.locator('[data-testid="console-stale"]')).toBeVisible();
  await expect(view.locator('[data-testid="console-stale-reconnect"]')).toHaveText('Reconnect');

  await view.locator('[data-testid="console-stale-reconnect"]').click();
  await expect(view.locator('[data-testid="console-stale"]')).toHaveCount(0);
  await expect(grid.first()).toContainText('10');
  expect(await runs()).toBe(before);
});
