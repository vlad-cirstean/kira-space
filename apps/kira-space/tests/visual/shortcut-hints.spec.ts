import { expect, studioTest, test } from '../ui/fixtures';
import {
  connectAndExpand,
  connectionCreateArgs,
  openConsoleFromMenu,
} from '../ui/studio/support/connect';
import {
  BIG_ROWS_PATH,
  bigRowsFixture,
  postgresConnectionSummary,
} from '../ui/studio/support/postgresFixture';
import { IPC } from '../ui/support/ipcChannels';
import { repoRowById } from '../ui/support/repoRows';

// P712: the shortcut chip in a button tooltip, and the right-aligned chip column in a context menu.

const TIP = '[data-slot="tooltip-content"]';

test('title bar tooltip: label then chip', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: [] });
  await page.locator('[data-testid="toggle-project-panel"]').hover();
  await expect(page.locator(TIP).first()).toHaveScreenshot('shortcut-tooltip-title-bar.png');
});

test('git repo row menu: chips in one right-aligned column', async ({ relaunch }) => {
  const repo = {
    id: 'repo-1',
    name: 'demo-repo',
    root: '/tmp/demo-repo',
    repoId: '/tmp/demo-repo',
    sortOrder: 1,
    color: 'none',
    createdAt: '2026-01-01T00:00:00.000Z',
  };
  const { window: page } = await relaunch({
    control: [{ channel: IPC.codeWorkspaceListRepos, response: [repo] }],
  });
  await repoRowById(page, repo.id).click({ button: 'right' });
  await expect(page.locator('[data-testid="context-menu"]')).toHaveScreenshot(
    'shortcut-menu-repo-row.png',
  );
});

studioTest('console Run tooltip: label then chip', async ({ relaunch }) => {
  const id = 'conn-hints-visual';
  const fixture = bigRowsFixture(id);
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: [] },
      {
        channel: IPC.connectionsCreate,
        args: connectionCreateArgs('Hints DB', 'green'),
        response: postgresConnectionSummary(id, 'Hints DB', 'green'),
      },
      ...fixture.control,
    ],
    stream: fixture.port,
  });
  await connectAndExpand(page, 'Hints DB', 'green');
  await openConsoleFromMenu(page, BIG_ROWS_PATH);
  await page.locator('[data-testid="console-run-statement"]').hover();
  await expect(page.locator(TIP).first()).toHaveScreenshot('shortcut-tooltip-console-run.png');
});
