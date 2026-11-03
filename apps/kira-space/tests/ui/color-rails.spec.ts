import type { Locator } from '@playwright/test';
import { expect, test } from './fixtures';
import { installGitStreamMock } from './support/gitStreamMock';
import { IPC } from './support/ipcChannels';

// P226: one colour mark across modules. Pins the same literal as
// apps/kira-studio/tests/ui/color-rails.spec.ts, so Studio and Space rails match by construction.

const RAIL_CLASS = 'absolute inset-y-0 left-0 w-1 bg-conn-cyan';

const REPO = {
  id: 'repo-rail-1',
  name: 'rail-repo',
  root: '/tmp/rail-repo',
  repoId: '/tmp/rail-repo',
  sortOrder: 1,
  color: 'cyan',
  createdAt: '2026-01-01T00:00:00.000Z',
};

const SCRIPT = {
  id: 'script-rail',
  name: 'Rail script',
  command: 'npm run dev',
  workingDir: '',
  color: 'cyan',
  collection: '',
  sortOrder: 0,
  createdAt: '2026-01-01T00:00:00.000Z',
  updatedAt: '2026-01-01T00:00:00.000Z',
};

const WORKTREE_LIST_RESULT = {
  worktrees: [
    {
      path: REPO.repoId,
      head: '0'.repeat(40),
      branch: 'refs/heads/main',
      isBare: false,
      isDetached: false,
      isMain: true,
      isCurrent: true,
      locked: null,
      prunable: null,
      openElsewhere: false,
    },
    {
      path: '/tmp/rail-repo-feature',
      head: '1'.repeat(40),
      branch: 'refs/heads/feature',
      isBare: false,
      isDetached: false,
      isMain: false,
      isCurrent: false,
      locked: null,
      prunable: null,
      openElsewhere: false,
    },
  ],
};

async function expectRail(rail: Locator, row: Locator): Promise<void> {
  await expect(rail).toHaveAttribute('class', RAIL_CLASS);
  const box = await rail.boundingBox();
  const rowBox = await row.boundingBox();
  expect(box?.width).toBe(4);
  expect(box?.x).toBe(rowBox?.x);
  expect(box?.height).toBe(rowBox?.height);
}

const paintOf = (el: Locator) => el.evaluate((n) => getComputedStyle(n).backgroundColor);

test('repo, worktree, quick-command and dialog rails share one class, geometry and paint', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
      {
        channel: IPC.codeWorkspaceListFiles,
        args: { id: REPO.id },
        response: { paths: ['a.ts'], status: {}, truncated: false },
      },
      {
        channel: IPC.codeWorkspaceReadFile,
        args: { id: REPO.id, path: 'a.ts' },
        response: {
          kind: 'found',
          text: 'export const a = 1;\n',
          bytes: 20,
          limitBytes: 8 * 1024 * 1024,
          language: 'typescript',
        },
      },
      { channel: IPC.customScriptsList, response: { collections: [], scripts: [SCRIPT] } },
    ],
  });
  await installGitStreamMock(page, REPO.repoId, {
    'repo.open': undefined,
    'worktree.list': WORKTREE_LIST_RESULT,
  });

  const repoRow = page.locator(`[data-testid="repo-row"][data-repo-id="${REPO.id}"]`);
  const repoRail = repoRow.locator('[data-testid="repo-rail"]');
  await expectRail(repoRail, repoRow);
  const paint = await paintOf(repoRail);

  await repoRow.locator('[data-testid="repo-row-expand"]').click();
  const worktreeRow = page.locator('[data-testid="repo-worktree-row"]').first();
  await expectRail(worktreeRow.locator('[data-testid="repo-worktree-rail"]'), worktreeRow);

  await page.locator('[data-testid="manage-repos"]').click();
  const navRow = page.locator(`[data-testid="repos-dialog-repo"][data-repo-id="${REPO.id}"]`);
  const navRail = navRow.locator('[data-testid="repos-dialog-repo-rail"]');
  await expect(navRail).toHaveAttribute('class', RAIL_CLASS);
  const navBox = await navRail.boundingBox();
  expect(navBox?.width).toBe(4);
  expect(navBox?.x).toBe((await navRow.boundingBox())?.x);
  expect(await paintOf(navRail)).toBe(paint);
  await page.keyboard.press('Escape');

  await page.locator('[data-testid="mode-tab"][data-mode="automations"]').click();
  const scriptRow = page.locator(`[data-testid="script-${SCRIPT.id}"]`);
  const scriptRail = scriptRow.locator('[data-testid="script-rail"]');
  await expectRail(scriptRail, scriptRow);
  expect(await paintOf(scriptRail)).toBe(paint);

  await page.locator('[data-testid="mode-tab"][data-mode="git"]').click();
  await repoRow.click();
  await page.locator('[data-testid="git-panel-tab-files"]').click();
  await page.locator('[data-testid="repo-tree-row"][data-path="a.ts"]').click();
  const tab = page.locator(
    '[data-testid="tab-strip-wrapper"] [data-testid="tab"][data-tab-kind="repo-file"]',
  );
  await expect(tab).toHaveAttribute('data-color', 'cyan');
  expect(await paintOf(tab.locator('span').first())).toBe(paint);
});

test('a linked worktree tab takes its anchor repo colour', async ({ relaunch }) => {
  const worktree = {
    ...REPO,
    id: 'repo-rail-wt',
    name: 'rail-repo-feature',
    root: '/tmp/rail-repo-feature',
    repoId: '/tmp/rail-repo-feature',
    sortOrder: 2,
    color: 'amber',
  };
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.codeWorkspaceListRepos, response: [REPO, worktree] },
      {
        channel: IPC.codeWorkspaceRepoWorktreeLinks,
        response: [
          { id: REPO.id, parentId: '' },
          { id: worktree.id, parentId: REPO.id },
        ],
      },
      { channel: IPC.codeWorkspaceImportRepo, args: { path: worktree.repoId }, response: worktree },
      {
        channel: IPC.codeWorkspaceListFiles,
        args: { id: worktree.id },
        response: { paths: ['a.ts'], status: {}, truncated: false },
      },
      {
        channel: IPC.codeWorkspaceReadFile,
        args: { id: worktree.id, path: 'a.ts' },
        response: {
          kind: 'found',
          text: 'export const a = 1;\n',
          bytes: 20,
          limitBytes: 8 * 1024 * 1024,
          language: 'typescript',
        },
      },
    ],
  });
  await installGitStreamMock(page, REPO.repoId, {
    'repo.open': undefined,
    'worktree.list': WORKTREE_LIST_RESULT,
  });

  const repoRow = page.locator(`[data-testid="repo-row"][data-repo-id="${REPO.id}"]`);
  await repoRow.locator('[data-testid="repo-row-expand"]').click();
  await page
    .locator(`[data-testid="repo-worktree-row"][data-worktree-path="${worktree.repoId}"]`)
    .click();
  await page.locator('[data-testid="git-panel-tab-files"]').click();
  await page.locator('[data-testid="repo-tree-row"][data-path="a.ts"]').click();
  const tab = page.locator(
    '[data-testid="tab-strip-wrapper"] [data-testid="tab"][data-tab-kind="repo-file"]',
  );
  await expect(tab).toHaveAttribute('data-color', 'cyan');
});
