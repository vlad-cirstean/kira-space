import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// P220: the Git panel's Repos/Review choice persists (localStorage `kira.git.panelTab`),
// defaults to Repos, and is never overwritten by opening a repo or re-mounting the panel.

const REPO = {
  id: 'repo-tab-1',
  name: 'tab-repo',
  root: '/tmp/tab-repo',
  repoId: '/tmp/tab-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};

const CONTROL: ControlSnapshot[] = [{ channel: IPC.codeWorkspaceListRepos, response: [REPO] }];

const panelTab = (page: Page, name: 'repos' | 'files' | 'review') =>
  page.locator(`[data-testid="git-panel-tab-${name}"]`);
const modeTab = (page: Page, mode: 'git' | 'automations') =>
  page.locator(`[data-testid="mode-tab"][data-mode="${mode}"]`);

test('opening a repo from the Repos list leaves the Repos tab selected', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await expect(panelTab(page, 'repos')).toHaveAttribute('aria-pressed', 'true');
  await page.locator(`[data-testid="repo-row"][data-repo-id="${REPO.id}"]`).click();
  await expect(page.locator('[data-testid="tab-strip-wrapper"] [data-testid="tab"]')).toHaveCount(
    1,
  );
  await expect(panelTab(page, 'repos')).toHaveAttribute('aria-pressed', 'true');
  await expect(panelTab(page, 'review')).toHaveAttribute('aria-pressed', 'false');
});

// The project file view moved into agent tasks, so a stored 'files' choice falls back to Repos.
test('a stored Files choice falls back to Repos; the panel has no Files tab', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await page.evaluate(() => localStorage.setItem('kira.git.panelTab', 'files'));
  await page.reload();
  await expect(panelTab(page, 'repos')).toHaveAttribute('aria-pressed', 'true');
  await expect(panelTab(page, 'files')).toHaveCount(0);
});

test('the picked tab survives leaving the Git module and coming back', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await page.locator(`[data-testid="repo-row"][data-repo-id="${REPO.id}"]`).click();
  await panelTab(page, 'review').click();
  await expect(panelTab(page, 'review')).toHaveAttribute('aria-pressed', 'true');

  await modeTab(page, 'automations').click();
  await expect(panelTab(page, 'review')).toHaveCount(0);
  await modeTab(page, 'git').click();
  await expect(panelTab(page, 'review')).toHaveAttribute('aria-pressed', 'true');
});

test('the picked tab survives a reload with a restored repo workspace', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [
      ...CONTROL,
      {
        channel: IPC.tabsList,
        response: [
          {
            id: 'restored-repo-graph',
            kind: 'repo-graph',
            connectionId: null,
            path: '',
            order: 0,
            active: true,
            workspaceId: REPO.id,
            state: { viewState: null, reviewSession: null },
          },
        ],
      },
    ],
  });
  await expect(panelTab(page, 'repos')).toHaveAttribute('aria-pressed', 'true');
  await panelTab(page, 'review').click();
  await expect(panelTab(page, 'review')).toHaveAttribute('aria-pressed', 'true');

  await page.reload();
  await expect(panelTab(page, 'review')).toHaveAttribute('aria-pressed', 'true');
});
