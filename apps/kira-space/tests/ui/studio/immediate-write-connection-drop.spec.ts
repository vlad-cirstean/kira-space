import { DATA_OP } from '@shared/protocol/dataOps';
import {
  controlSnapshots as redisControl,
  portSnapshots as redisPorts,
} from '../../ipc/redis/redis.fixture';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, test } from './fixtures';
import { createConnection, disconnectConnection } from './support/connect';
import { IPC } from './support/ipcChannels';
import {
  DB_PATH as MONGO_DB_PATH,
  mongoConnectionSummary,
  WIDGETS_PATH,
  widgetsFixture,
} from './support/mongoFixture';
import { connectionsDisconnectSnapshot } from './support/postgresFixture';
import { openRedisBrowse, REDIS_DB0 } from './support/redisFixture';
import { connectionRow, expandRow, findRow, openBrowseLeaf, openRowMenu } from './support/tree';

// Documents, KeyValue and Browse keep their data on a connection drop, read-only behind the stale
// strip; open editors keep their buffer and no write reaches the server before the reconnect.

const MONGO_ID = 'conn-doc-drop';
const MONGO_FIXTURE = widgetsFixture(MONGO_ID);
const MONGO_CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: {
      name: 'Mongo Drop',
      kind: 'mongodb',
      color: 'green',
      mode: 'fields',
      readOnly: false,
      host: '127.0.0.1',
      port: 27017,
      database: 'kira_test',
      username: 'kira',
      password: null,
      uri: null,
      options: {},
      preconnect: null,
      preconnectSidecar: false,
      preconnectDelaySec: 0,
      autoExplain: false,
      throttlePerSec: 0,
    },
    response: mongoConnectionSummary(MONGO_ID, 'Mongo Drop', 'green'),
  },
  ...MONGO_FIXTURE.control,
  connectionsDisconnectSnapshot(MONGO_ID),
];

const REDIS_CONTROL: ControlSnapshot[] = [
  ...redisControl,
  connectionsDisconnectSnapshot('test-redis'),
];
const SESSION_NS = `${REDIS_DB0}/namespace:session`;
const SESSION_KEY = `${SESSION_NS}/key:session%3Aabc`;

test('documents keep rows and the open editor read-only after a drop', async ({ relaunch }) => {
  const { window: page, stream } = await relaunch({
    control: MONGO_CONTROL,
    stream: MONGO_FIXTURE.port,
  });
  await createConnection(page, 'mongodb', 'Mongo Drop');
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
  await expandRow(page, MONGO_DB_PATH);
  await (await findRow(page, WIDGETS_PATH)).dblclick();
  await expect(page.locator('[data-testid="document-row"]').first()).toBeVisible({
    timeout: 15_000,
  });

  await page.locator('[data-testid="document-edit"]').first().click();
  await expect(page.locator('[data-testid="document-edit-save"]')).toBeEnabled();

  await disconnectConnection(page);

  await expect(page.locator('[data-testid="document-stale"]')).toBeVisible();
  await expect(page.locator('[data-testid="document-reconnect"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="document-row"]').first()).toBeVisible();
  await expect(page.locator('[data-testid="document-edit-save"]')).toBeDisabled();
  await expect(page.locator('[data-testid="document-add"]')).toBeDisabled();
  await expect(page.locator('[data-testid="document-delete"]').first()).toBeDisabled();

  await page.click('[data-testid="document-stale-reconnect"]');
  await expect(page.locator('[data-testid="document-stale"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="document-edit-save"]')).toBeEnabled();

  expect((await stream.ops()).filter((o) => o.op === DATA_OP.mutate)).toHaveLength(0);
});

test('a key/value tab keeps its value read-only after a drop', async ({ relaunch }) => {
  const { window: page, stream } = await relaunch({ control: REDIS_CONTROL, stream: redisPorts });
  await openRedisBrowse(page);
  const browse = page.locator('[data-testid="browse-view"]');
  await browse.locator(`[data-testid="browse-row"][data-path="${SESSION_NS}"]`).dblclick();
  await openBrowseLeaf(page, SESSION_KEY);
  const view = page.locator(`[data-testid="keyvalue-view"][data-path="${SESSION_KEY}"]`);
  await expect(view.locator('[data-testid="keyvalue-row"]').first()).toBeVisible({
    timeout: 15_000,
  });

  await disconnectConnection(page);

  await expect(page.locator('[data-testid="keyvalue-stale"]')).toBeVisible();
  await expect(view.locator('[data-testid="keyvalue-row"]').first()).toContainText('token-abc');
  await expect(page.locator('[data-testid="keyvalue-edit"]')).toBeDisabled();
  await expect(page.locator('[data-testid="keyvalue-add"]')).toBeDisabled();
  await expect(page.locator('[data-testid="keyvalue-delete"]')).toBeDisabled();
  await view.locator('[data-testid="keyvalue-row"]').first().click();
  await expect(page.locator('[data-testid="cell-editor-panel"]')).toContainText('Connection lost');

  await page.click('[data-testid="keyvalue-stale-reconnect"]');
  await expect(page.locator('[data-testid="keyvalue-stale"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="keyvalue-edit"]')).toBeEnabled();
  await expect(page.locator('[data-testid="keyvalue-add"]')).toBeEnabled();
  await expect(page.locator('[data-testid="keyvalue-delete"]')).toBeEnabled();

  expect((await stream.ops()).filter((o) => o.op === DATA_OP.mutate)).toHaveLength(0);
});

test('browse keeps the level and preview read-only after a drop', async ({ relaunch }) => {
  const { window: page, stream } = await relaunch({ control: REDIS_CONTROL, stream: redisPorts });
  await openRedisBrowse(page);
  const browse = page.locator('[data-testid="browse-view"]');
  await browse.locator(`[data-testid="browse-row"][data-path="${SESSION_NS}"]`).dblclick();
  await browse.locator(`[data-testid="browse-row"][data-path="${SESSION_KEY}"]`).click();
  const detail = page.locator('[data-testid="browse-detail-pane"]');
  await expect(detail.locator('[data-testid="keyvalue-row"]').first()).toBeVisible({
    timeout: 15_000,
  });

  await disconnectConnection(page);

  await expect(page.locator('[data-testid="browse-stale"]')).toBeVisible();
  await expect(page.locator('[data-testid="browse-reconnect"]')).toHaveCount(0);
  await expect(browse.locator('[data-testid="browse-row"]').first()).toBeVisible();
  await expect(detail.locator('[data-testid="keyvalue-row"]').first()).toBeVisible();
  await expect(detail.locator('[data-testid="keyvalue-delete"]')).toBeDisabled();
  await expect(detail.locator('[data-testid="keyvalue-add"]')).toBeDisabled();

  expect((await stream.ops()).filter((o) => o.op === DATA_OP.mutate)).toHaveLength(0);
});
