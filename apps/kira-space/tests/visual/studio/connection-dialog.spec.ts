import { expect, test } from '../../ui/studio/fixtures';
import { IPC } from '../../ui/studio/support/ipcChannels';

// The new-connection dialog at rest (name, kind, color): the only connection form left now that a
// connection's fields live in its file.

test('new-connection dialog at rest', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.connectionsList, response: [] }],
  });

  await page.click('[data-testid="add-connection"]');
  const dialog = page.locator('[data-testid="new-connection-dialog"]');
  await expect(dialog).toBeVisible();
  await page.click('[data-testid="connection-kind-postgres"]');
  await page.fill('[data-testid="new-connection-name"]', 'Visual Connection Dialog');
  await expect(page).toHaveScreenshot('new-connection-dialog.png');
});
