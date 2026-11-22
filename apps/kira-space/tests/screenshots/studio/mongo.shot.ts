import { CAPTURE_LAUNCH, CAPTURE_NOW } from '@workbench/testing/screenshots/capture';
import { expect, studioTest as test } from '../../ui/fixtures';
import { connectionSummary } from '../../ui/studio/support/engineFixture';
import { connectionRow, expandRow, findRow } from '../../ui/studio/support/tree';
import { IPC } from '../../ui/support/ipcChannels';
import { shoot } from '../support/shoot';
import { acmeMongo, MONGO_DB, MONGO_ID, MONGO_NAME, ORDERS_PATH } from './support/acmeMongo';

// P734: the MongoDB document view. Run by `bun run screenshots`; output goes to /screenshots/framed.

test('mongodb documents', async ({ relaunch, consoleErrors }) => {
  const fixture = acmeMongo();
  const summary = {
    ...connectionSummary('mongodb', 27017, 'acme_shop', 'shop_app', MONGO_ID, MONGO_NAME, 'green'),
    host: 'mongo.acme.example',
  };
  const { window: page } = await relaunch({
    ...CAPTURE_LAUNCH,
    control: [
      { channel: IPC.connectionsList, response: [summary] },
      { channel: IPC.repoConfigList, response: { dir: '~/.kira-space/repos', files: [] } },
      { channel: IPC.adeTaskRepos, response: { repos: [], folders: [] } },
      { channel: IPC.adeTaskSessions, response: { sessions: [] } },
      ...fixture.control,
    ],
    stream: fixture.port,
  });
  const row = connectionRow(page);
  await expect(row).toBeVisible();
  await row.click({ button: 'right' });
  await page.click('[data-testid="menu-item-connect"]');
  await expect(row.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, MONGO_DB);
  await (await findRow(page, ORDERS_PATH)).dblclick();
  await expect(page.locator('[data-testid="document-view"]')).toBeVisible();
  await expect(page.locator('[data-testid="document-row"]').first()).toBeVisible({
    timeout: 15_000,
  });
  await page.clock.setFixedTime(CAPTURE_NOW + 1000);
  await shoot(page, 'studio-mongo-documents', consoleErrors);
});
