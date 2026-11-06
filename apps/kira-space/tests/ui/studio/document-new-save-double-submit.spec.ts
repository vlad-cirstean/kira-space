import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, test } from './fixtures';
import { createConnection } from './support/connect';
import { IPC } from './support/ipcChannels';
import {
  DB_PATH as MONGO_DB_PATH,
  mongoConnectionSummary,
  WIDGETS_PATH,
  widgetsFixture,
} from './support/mongoFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';

// Two Save clicks on the new-document panel used to run two `insertOne`s with no `_id`, so the
// server assigned two ids and both documents landed.

const CONNECTION_ID = 'conn-doc-save';
const FIXTURE = widgetsFixture(CONNECTION_ID);
const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: {
      name: 'Mongo Save',
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
    response: mongoConnectionSummary(CONNECTION_ID, 'Mongo Save', 'green'),
  },
  ...FIXTURE.control,
];
const PORT: PortSnapshot[] = [
  ...FIXTURE.port,
  {
    op: DATA_OP.mutate,
    payload: {
      connectionId: CONNECTION_ID,
      path: WIDGETS_PATH,
      ops: [{ kind: 'insert', values: { $document: '{\n  \n}' } }],
    },
    response: { kind: 'mutate', affectedRows: 1 },
    delayMs: 1500,
  },
  {
    op: DATA_OP.invalidate,
    payload: { connectionId: CONNECTION_ID, path: WIDGETS_PATH },
    response: { kind: 'invalidate' },
  },
];

test('a double click on the new-document Save inserts once', async ({ relaunch }) => {
  const { window: page, stream } = await relaunch({ control: CONTROL, stream: PORT });

  await createConnection(page, 'mongodb', 'Mongo Save');

  const connRow = connectionRow(page);
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, MONGO_DB_PATH);
  await (await findRow(page, WIDGETS_PATH)).dblclick();
  await expect(page.locator('[data-testid="document-row"]').first()).toBeVisible({
    timeout: 15_000,
  });

  await page.click('[data-testid="document-add"]');
  await page.locator('[data-testid="document-new-save"]').dblclick();
  await expect(page.locator('[data-testid="document-new"]')).toHaveCount(0, { timeout: 15_000 });

  const mutates = (await stream.ops()).filter((op) => op.op === DATA_OP.mutate);
  expect(mutates).toHaveLength(1);
});
