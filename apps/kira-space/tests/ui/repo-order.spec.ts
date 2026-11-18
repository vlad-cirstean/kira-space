import type { Page } from '@playwright/test';
import { boxCenter, sortableHover } from '@workbench/testing/ui/drag';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { installGitStreamMock } from './support/gitStreamMock';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { repoRowById } from './support/repoRows';

// P308: top-level repos in the Git panel reorder by drag only (P669: no Alt+Up/Down); worktree rows stay in git
// order. Backend half: repoflow import_test "rename, colour and reorder persist".

const t = (id: string) => `[data-testid="${id}"]`;

function record(id: string, name: string, order: number) {
  return {
    id,
    name,
    root: `/tmp/${name}`,
    repoId: `/tmp/${name}`,
    sortOrder: order,
    color: 'none',
    createdAt: '2026-01-01T00:00:00.000Z',
    hidden: false,
  };
}

const A = record('repo-a', 'alpha-repo', 0);
const B = record('repo-b', 'bravo-repo', 1);
const LIST = { channel: IPC.codeWorkspaceListRepos, response: [A, B] };

const WORKTREES = {
  worktrees: ['/tmp/alpha-repo', '/tmp/alpha-repo-feature'].map((path, i) => ({
    path,
    head: String(i).repeat(40),
    branch: i === 0 ? 'refs/heads/main' : 'refs/heads/feature',
    isBare: false,
    isDetached: false,
    isMain: i === 0,
    isCurrent: i === 0,
    locked: null,
    prunable: null,
    openElsewhere: false,
  })),
};

const orderOf = (page: Page) =>
  page
    .locator(t('repo-row'))
    .evaluateAll((els) => els.map((el) => el.getAttribute('data-repo-id')));

async function dragAbove(page: Page, from: string, to: string): Promise<void> {
  const src = await repoRowById(page, from).boundingBox();
  const dst = await repoRowById(page, to).boundingBox();
  if (!src || !dst) throw new Error('repo row has no box');
  await sortableHover(
    page,
    boxCenter(src),
    { x: dst.x + dst.width / 2, y: dst.y + dst.height * 0.25 },
    { start: { x: 0, y: -10 } },
  );
  await page.mouse.up();
}

test('drag a repo above another sends ReorderRepos and shows the new order', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [LIST, { channel: IPC.codeWorkspaceReorderRepos, response: [B, A] }],
  });
  await expect(page.locator(t('repo-row'))).toHaveCount(2);

  await dragAbove(page, B.id, A.id);
  await expect.poll(() => orderOf(page)).toEqual([B.id, A.id]);
  expect(calls(control, IPC.codeWorkspaceReorderRepos)).toHaveLength(1);
  expect(calls(control, IPC.codeWorkspaceReorderRepos)[0]?.args).toEqual({
    ...contract<object>('repos', 'args:CodeWorkspaceService.ReorderRepos'),
    ids: [B.id, A.id],
  });
});

test('Alt+ArrowUp on a repo or worktree row moves nothing', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [LIST, { channel: IPC.codeWorkspaceReorderRepos, response: [B, A] }],
  });
  await installGitStreamMock(page, A.repoId, {
    'repo.open': undefined,
    'worktree.list': WORKTREES,
  });

  await repoRowById(page, A.id).locator(t('repo-row-expand')).click();
  const worktree = page.locator(t('repo-worktree-row')).last();
  await expect(worktree).toBeVisible();
  await worktree.focus();
  await page.keyboard.press('Alt+ArrowUp');
  await repoRowById(page, B.id).focus();
  await page.keyboard.press('Alt+ArrowUp');

  // Barrier: one drag lands exactly one call, so the chords sent none.
  await dragAbove(page, B.id, A.id);
  await expect.poll(() => orderOf(page)).toEqual([B.id, A.id]);
  expect(calls(control, IPC.codeWorkspaceReorderRepos)).toHaveLength(1);
});

test('an active search pauses drag', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: [LIST] });
  await page.locator(t('toggle-search')).click();
  await page.locator(t('tree-search')).fill('repo');
  await expect(page.locator(t('repo-row'))).toHaveCount(2);

  await dragAbove(page, B.id, A.id);
  // Absence of a call: nothing to await, no later event on the same channel.
  await page.waitForTimeout(100);
  expect(calls(control, IPC.codeWorkspaceReorderRepos)).toHaveLength(0);
  expect(await orderOf(page)).toEqual([A.id, B.id]);
});
