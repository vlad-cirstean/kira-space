import { DATA_OP } from '@shared/protocol/dataOps';
import {
  controlSnapshots as redisControl,
  portSnapshots as redisPorts,
} from '../../ipc/redis/redis.fixture';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { emitWailsEvent } from '../support/mockRuntime';
import { openHttpModeAndNewRequest } from './support/apiMode';
import { connectAndExpand, connectionCreateArgs } from './support/connect';
import { acceptConfirm } from './support/dialogs';
import type { StudioPortSnapshot } from './support/mockStream';
import {
  APP_PATH,
  BIG_ROWS_PATH,
  bigRowsFixture,
  NULLS_AND_UNICODE_PAGE,
  NULLS_META,
  NULLS_PATH,
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { openRedisBrowse } from './support/redisFixture';
import { readOnlySqsSnapshots, SQS_QUEUE_PATH } from './support/sqsFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';

// P375: tab views unmount on switch; filter inputs and filtered results live in per-tab runtime
// stores, so switching away and back restores them. Frontend half only: no IPC change.

const CONNECTION_ID = 'conn-tab-filter';
const FIXTURE = bigRowsFixture(CONNECTION_ID);

const ORDER_PRODUCTS_PATH = `${APP_PATH}/table:products`;
const ORDER_FIXTURE = orderItemsFixture('conn-tab-filter-def');

function definitionOf(path: string, qualifiedName: string) {
  return {
    path,
    kind: 'table' as const,
    qualifiedName,
    statements: [`CREATE TABLE ${qualifiedName} (\n  id integer NOT NULL\n)`],
    language: 'sql' as const,
    origin: 'composed' as const,
    notes: [],
    constraints: [],
    documentSchema: null,
    sections: [],
    generatedAt: '2026-01-01T00:00:00.000Z',
  };
}

const DEFINITION_CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Tab Filter DB', 'green'),
    response: postgresConnectionSummary('conn-tab-filter-def', 'Tab Filter DB', 'green'),
  },
  ...ORDER_FIXTURE.control,
  ...[
    [ORDER_ITEMS_PATH, 'app.order_items'],
    [ORDER_PRODUCTS_PATH, 'app.products'],
  ].map(([path, name]) => ({
    channel: IPC.treeDefinition,
    args: { connectionId: 'conn-tab-filter-def', path, refresh: false, tabId: null },
    response: { definition: definitionOf(path, name), source: 'server' as const },
  })),
];

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Tab Filter DB', 'green'),
    response: postgresConnectionSummary(CONNECTION_ID, 'Tab Filter DB', 'green'),
  },
  ...FIXTURE.control,
  {
    channel: IPC.treeDescribe,
    args: { connectionId: CONNECTION_ID, path: NULLS_PATH, refresh: false, tabId: null },
    response: { meta: NULLS_META, source: 'server' },
  },
];

const PORT: StudioPortSnapshot[] = [
  ...FIXTURE.port,
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: NULLS_PATH,
      projection: null,
      filter: null,
      sort: null,
      pageSize: 100,
      cursor: { mode: 'offset', offset: 0 },
    },
    response: { kind: 'read', page: NULLS_AND_UNICODE_PAGE, source: 'server' },
  },
];

test('grid find: query, options and filtered rows survive a tab switch; closing clears them', async ({
  relaunch,
}) => {
  test.setTimeout(90_000);
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndExpand(page, 'Tab Filter DB', 'green');

  const tabs = page.locator('[data-testid="tab"]');
  await (await findRow(page, BIG_ROWS_PATH)).dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await (await findRow(page, NULLS_PATH)).dblclick();
  await expect(tabs).toHaveCount(2);

  await tabs.nth(0).click();
  await page.click('[data-testid="toolbar-search"]');
  await page.click('[data-testid="search-regex"]');
  await page.fill('[data-testid="search-input"]', '^(2|5|9)$');
  await expect(page.locator('[data-testid="search-count"]')).toContainText('1 of 3');
  await page.click('[data-testid="search-filter-rows"]');
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(3);

  await tabs.nth(1).click();
  await expect(page.locator('[data-testid="search-bar"]')).toHaveCount(0);
  await tabs.nth(0).click();

  await expect(page.locator('[data-testid="search-input"]')).toHaveValue('^(2|5|9)$');
  await expect(page.locator('[data-testid="search-regex"]')).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await expect(page.locator('[data-testid="search-filter-rows"]')).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await expect(page.locator('[data-testid="search-count"]')).toContainText('1 of 3');
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(3);
  await expect(page.locator('[data-testid="search-scope"]')).toContainText(
    'Showing 3 of 100 loaded rows',
  );

  // Esc closes: rows return and the next open starts empty.
  await page.press('[data-testid="search-input"]', 'Escape');
  await expect(page.locator('[data-testid="search-bar"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="grid-gutter-cell"]').nth(9)).toBeVisible();
  await page.click('[data-testid="toolbar-search"]');
  await expect(page.locator('[data-testid="search-input"]')).toHaveValue('');
  await expect(page.locator('[data-testid="search-filter-rows"]')).toHaveAttribute(
    'aria-pressed',
    'false',
  );

  // Closing the tab drops the state: a reopened table starts with the bar closed.
  await page.fill('[data-testid="search-input"]', '7');
  await page.click('[data-testid="search-filter-rows"]');
  await tabs.nth(0).hover();
  await tabs.nth(0).locator('[data-testid="tab-close"]').click();
  await expect(tabs).toHaveCount(1);
  await (await findRow(page, BIG_ROWS_PATH)).dblclick();
  await expect(tabs).toHaveCount(2);
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await expect(page.locator('[data-testid="search-bar"]')).toHaveCount(0);
  await page.click('[data-testid="toolbar-search"]');
  await expect(page.locator('[data-testid="search-input"]')).toHaveValue('');
  await expect(page.locator('[data-testid="search-filter-rows"]')).toHaveAttribute(
    'aria-pressed',
    'false',
  );
});

test('stream find: query and filtered rows survive a tab switch', async ({ relaunch }) => {
  test.setTimeout(90_000);
  const { control, stream: port } = readOnlySqsSnapshots([1]);
  const { window: page } = await relaunch({ control, stream: port });
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');

  const tabs = page.locator('[data-testid="tab"]');
  await (await findRow(page, SQS_QUEUE_PATH)).dblclick();
  await page.click('[data-testid="stream-poll"]');
  await acceptConfirm(page);
  await expect(page.locator('[data-testid="stream-row"]').first()).toBeVisible({ timeout: 15_000 });
  await (await findRow(page, 'queue:empty-queue')).dblclick();
  await expect(tabs).toHaveCount(2);

  await tabs.nth(0).click();
  await page.click('[data-testid="stream-search-toggle"]');
  await page.fill('[data-testid="stream-search-input"]', 'msg-1');
  await expect(page.locator('[data-testid="stream-search-count"]')).toContainText('1 of 1');
  await page.click('[data-testid="stream-search-filter-rows"]');
  await expect(page.locator('[data-testid="stream-row"]')).toHaveCount(1);

  await tabs.nth(1).click();
  await expect(page.locator('[data-testid="stream-search-input"]')).toHaveCount(0);
  await tabs.nth(0).click();

  await expect(page.locator('[data-testid="stream-search-input"]')).toHaveValue('msg-1');
  await expect(page.locator('[data-testid="stream-search-filter-rows"]')).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await expect(page.locator('[data-testid="stream-search-count"]')).toContainText('1 of 1');
  await expect(page.locator('[data-testid="stream-row"]')).toHaveCount(1);
});

test('browse filter: open bar, text and filtered list survive a tab switch', async ({
  relaunch,
}) => {
  test.setTimeout(90_000);
  const { window: page } = await relaunch({ control: redisControl, stream: redisPorts });
  await openRedisBrowse(page);

  const tabs = page.locator('[data-testid="tab"]');
  await (await findRow(page, 'database:db1')).dblclick();
  await expect(tabs).toHaveCount(2);

  await tabs.nth(0).click();
  await page.click('[data-testid="browse-filter-toggle"]');
  await page.fill('[data-testid="browse-filter"]', 'queue');
  await expect(page.locator('[data-testid="browse-row"]')).toHaveCount(1);

  await tabs.nth(1).click();
  await expect(page.locator('[data-testid="browse-view"]')).toHaveAttribute(
    'data-level',
    'database:db1',
  );
  await tabs.nth(0).click();

  await expect(page.locator('[data-testid="browse-filter"]')).toHaveValue('queue');
  await expect(page.locator('[data-testid="browse-row"]')).toHaveCount(1);
});

test('definition structure filter survives a tab switch', async ({ relaunch }) => {
  test.setTimeout(90_000);
  const { window: page } = await relaunch({
    control: DEFINITION_CONTROL,
    stream: ORDER_FIXTURE.port,
  });
  await connectAndExpand(page, 'Tab Filter DB', 'green');

  const tabs = page.locator('[data-testid="tab"]');
  await openRowMenu(page, ORDER_ITEMS_PATH);
  await page.click('[data-testid="menu-item-open-definition"]');
  await expect(page.locator('[data-testid="definition-view"]')).toBeVisible();
  await openRowMenu(page, ORDER_PRODUCTS_PATH);
  await page.click('[data-testid="menu-item-open-definition"]');
  await expect(tabs).toHaveCount(2);

  await tabs.nth(0).click();
  await page.click('[data-testid="definition-search-toggle"]');
  const filter = page.locator('[data-testid="definition-structure-filter"]');
  await filter.fill('quantity');
  const columns = page.locator('[data-testid="definition-columns"]');
  await expect(columns).not.toContainText('order_id');

  await tabs.nth(1).click();
  await expect(page.locator('[data-testid="definition-view"]')).toHaveAttribute(
    'data-path',
    ORDER_PRODUCTS_PATH,
  );
  await tabs.nth(0).click();

  await expect(filter).toHaveValue('quantity');
  await expect(columns).toContainText('quantity');
  await expect(columns).not.toContainText('order_id');
});

test('http request field filter survives a tab switch', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: [] });
  await openHttpModeAndNewRequest(page);
  await page.click('[data-testid="new-request"]');
  const tabs = page.locator('[data-testid="tab"]');
  await expect(tabs).toHaveCount(2);

  await tabs.nth(0).click();
  await page.click('[data-testid="http-request-pane-headers"]');
  await page.click('[data-testid="http-field-filter-toggle"]');
  await page.fill('[data-testid="http-field-filter"]', 'accept');

  await tabs.nth(1).click();
  await expect(page.locator('[data-testid="http-field-filter"]')).toHaveCount(0);
  await tabs.nth(0).click();

  await expect(page.locator('[data-testid="http-field-filter"]')).toHaveValue('accept');
  await expect(page.locator('[data-testid="http-field-filter-toggle"]')).toHaveAttribute(
    'aria-pressed',
    'true',
  );
});

test('grid WHERE draft survives a shortcut tab switch without being applied', async ({
  relaunch,
}) => {
  test.setTimeout(90_000);
  const { window: page, stream } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndExpand(page, 'Tab Filter DB', 'green');

  const tabs = page.locator('[data-testid="tab"]');
  await (await findRow(page, BIG_ROWS_PATH)).dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await (await findRow(page, NULLS_PATH)).dblclick();
  await expect(tabs).toHaveCount(2);
  await tabs.nth(0).click();

  const where = page.locator('[data-testid="filter-where-input"]');
  const readsBefore = (await stream.ops()).filter((o) => o.op === DATA_OP.read).length;
  await where.fill('id < 4');

  // A shortcut switch fires no blur, so the draft is never applied.
  await emitWailsEvent(page, IPC.tabNext, null);
  await expect(tabs.nth(1)).toHaveAttribute('data-active', 'true');
  await emitWailsEvent(page, IPC.tabPrev, null);
  await expect(tabs.nth(0)).toHaveAttribute('data-active', 'true');

  await expect(where).toHaveValue('id < 4');
  expect((await stream.ops()).filter((o) => o.op === DATA_OP.read)).toHaveLength(readsBefore);
});
