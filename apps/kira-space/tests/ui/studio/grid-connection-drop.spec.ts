import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, test } from './fixtures';
import { connectAndExpand, connectionCreateArgs } from './support/connect';
import { gridCell } from './support/grid';
import { IPC } from './support/ipcChannels';
import {
  COMPOSITE_PK_PATH,
  compositePkConnectAndOpen,
  connectionsDisconnectSnapshot,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { findRow, openRowMenu } from './support/tree';

// A connection drop keeps the shown page, read-only behind the stale strip; staged edits survive
// and Commit re-enables after the reconnect reload.

const CONNECTION_ID = 'conn-grid-drop';
const FIXTURE = compositePkConnectAndOpen(CONNECTION_ID);
const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Drop DB', 'green'),
    response: postgresConnectionSummary(CONNECTION_ID, 'Drop DB', 'green'),
  },
  ...FIXTURE.control,
  connectionsDisconnectSnapshot(CONNECTION_ID),
];
const PORT: PortSnapshot[] = FIXTURE.port;

test('a dropped connection keeps the page read-only and staged edits survive the reconnect', async ({
  relaunch,
}) => {
  const { window: page, stream } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndExpand(page, 'Drop DB', 'green');
  await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
  await expect(gridCell(page, 0, 'name')).toBeVisible();

  await gridCell(page, 0, 'name').dblclick();
  await page.locator('[data-testid="grid-cell-input"]').fill('first edit');
  await page.keyboard.press('Enter');
  await gridCell(page, 1, 'name').dblclick();
  await page.locator('[data-testid="grid-cell-input"]').fill('open edit');

  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-disconnect"]');

  await expect(page.locator('[data-testid="grid-stale"]')).toBeVisible();
  await expect(page.locator('[data-testid="reconnect-panel"]')).toHaveCount(0);
  await expect(gridCell(page, 0, 'name')).toBeVisible();
  await expect(page.locator('[data-testid="pending-count-chip"]')).toHaveText('2 rows pending');
  await expect(page.locator('[data-testid="grid-cell-input"]')).toHaveCount(0);

  await gridCell(page, 2, 'name').click();
  await expect(page.locator('[data-testid="cell-editor-panel"]')).toContainText('Connection lost');
  await gridCell(page, 2, 'name').dblclick();
  await expect(page.locator('[data-testid="grid-cell-input"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="toolbar-commit-changes"]')).toBeDisabled();
  await expect(page.locator('[data-testid="toolbar-preview-command"]')).toBeDisabled();
  await expect(page.locator('[data-testid="toolbar-add-row"]')).toBeDisabled();
  await expect(page.locator('[data-testid="toolbar-delete-row"]')).toBeDisabled();
  await expect(page.locator('[data-testid="toolbar-discard-changes"]')).toBeEnabled();
  await expect(page.locator('[data-testid="grid-writable-badge"]')).toHaveText('Read-only');

  await page.click('[data-testid="grid-stale-reconnect"]');
  await expect(page.locator('[data-testid="grid-stale"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="toolbar-commit-changes"]')).toBeEnabled();
  await expect(page.locator('[data-testid="pending-count-chip"]')).toHaveText('2 rows pending');

  expect((await stream.ops()).filter((o) => o.op === DATA_OP.mutate)).toHaveLength(0);
});
