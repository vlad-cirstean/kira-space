import { expect, test } from '../ui/fixtures';
import { adeV2Control } from '../ui/support/adeV2';
import { IPC } from '../ui/support/ipcChannels';

// The ADE Repositories page: list pane beside the picked repo's settings.

function repoRecord(id: string, name: string, sortOrder: number, hidden: boolean) {
  return {
    id,
    name,
    root: `/tmp/${name}`,
    repoId: `/tmp/${name}`,
    sortOrder,
    color: 'blue',
    createdAt: '2026-01-01T00:00:00.000Z',
    hidden,
  };
}

test('ade: repositories page', async ({ relaunch }) => {
  const web = repoRecord('repo-web-app', 'acme-customer-dashboard-web-frontend', 1, false);
  const api = repoRecord('repo-api', 'acme-platform-core-api-service', 2, true);
  const { window: win } = await relaunch({
    control: adeV2Control([
      { channel: IPC.windowsEnsure, response: { mode: 'ade' } },
      { channel: IPC.codeWorkspaceListRepos, response: [web, api] },
    ]),
  });
  await win.setViewportSize({ width: 1400, height: 820 });
  await win.locator('[data-testid="ade-tab-repos"]').click();
  const page = win.locator('[data-testid="ade-repos"]');
  await expect(page.locator('[data-testid="ade-repos-repo-head"]')).toBeVisible();
  await expect(page.locator('[data-testid="ade-repos-repo"][data-hidden="true"]')).toHaveCount(1);
  await expect(page).toHaveScreenshot('ade-repos-page.png');
});
