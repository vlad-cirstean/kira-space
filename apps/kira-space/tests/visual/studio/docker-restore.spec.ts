import { expect, test } from '../../ui/studio/fixtures';
import { contract } from '../../ui/studio/support/contract';
import { openDocker, setupDocker } from '../../ui/studio/support/dockerFixture';
import { settleFonts } from './support/settle';

// P441: the Docker restore dialog on the shared design: standalone, Compose project, waiting,
// already running and deleted rows.

test('docker restore dialog', async ({ relaunch }) => {
  const { page, state } = await setupDocker(relaunch);
  const pending = contract<{ items: Record<string, unknown>[] }>(
    'docker-restore',
    'DockerService.RestoreState#pending',
  );
  const solo = pending.items.find((i) => i.id === 'id-solo');
  state.restore = {
    ...pending,
    items: [
      ...pending.items,
      { ...solo, id: 'id-cache', name: 'kira-flow-cache', status: 'waiting' },
    ],
  };
  await page.setViewportSize({ width: 1400, height: 900 });
  await openDocker(page);
  await page.locator('[data-testid="docker-restore-review"]').click();
  const dialog = page.locator('[data-testid="restore-dialog"]');
  await expect(dialog).toBeVisible();
  await page.locator('[data-testid="restore-unavailable-toggle"]').click();
  await expect(page.locator('[data-testid="restore-row"][data-id="id-doomed"]')).toBeVisible();
  await settleFonts(page);
  await expect(dialog).toHaveScreenshot('docker-restore-dialog.png');
});
