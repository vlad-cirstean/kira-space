import type { Page } from '@playwright/test';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../../ui/fixtures';
import { createConnection } from '../../ui/studio/support/connect';
import { mariadbCreateArgs } from '../../ui/studio/support/connectionArgs';
import {
  DB_PATH,
  mariadbConnectionSummary,
  ORDER_ITEMS_PATH,
  orderItemsFixture,
} from '../../ui/studio/support/mariadbFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from '../../ui/studio/support/tree';
import { IPC } from '../../ui/support/ipcChannels';
import { settleFonts } from './support/settle';

// The data grid at rest on a small, representative table — mixed column types, no scroll/edit in
// progress. Same boot shape font-roles.spec.ts's own connectAndOpenGrid already established (a
// handful of rows, not the heavier 1M-row postgres fixture data-view.spec.ts's own interaction
// tests use — this spec only needs one stable rendered frame, not real pagination).

const CONNECTION_ID = 'conn-visual-data-view';
const CONNECTION_SUMMARY = mariadbConnectionSummary(CONNECTION_ID, 'VisualDataView', 'amber');
const FIXTURE = orderItemsFixture(CONNECTION_ID);

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: mariadbCreateArgs('VisualDataView'),
    response: CONNECTION_SUMMARY,
  },
  ...FIXTURE.control,
];

async function connectAndOpenGrid(page: Page): Promise<void> {
  await createConnection(page, 'mariadb', 'VisualDataView');

  const connRow = connectionRow(page);
  await expect(connRow).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);

  const row = await findRow(page, ORDER_ITEMS_PATH);
  await row.dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await expect(page.locator('[data-testid="grid-row"]').first()).toBeVisible({ timeout: 15_000 });
}

test('data grid at rest (P6)', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: FIXTURE.port });
  await connectAndOpenGrid(page);

  await settleFonts(page);
  await expect(page).toHaveScreenshot('data-grid.png');
});
