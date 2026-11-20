import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { gitStreamParams } from './support/gitStreamMock';
import {
  openPortGraph,
  PICKER_RESULTS,
  rootRow,
  SHA_A,
  singleRowChunks,
} from './support/gitUiPortFixtures';

// The Git module removes a worktree inline, with no dialog (P702): a clean one goes on one op, a dirty
// one shows an inline refusal with a button that names the loss. Contract git-worktree-remove;
// backend half: flows/gitflow TestWorktreeRemoveInline (worktree_remove_test.go).

const tid = (page: Page, id: string) => page.locator(`[data-testid="${id}"]`);

const WORKTREE = '/tmp/port-repo-wt';

type Preflight = { verdict: string; confirmToken?: string };
const preflight = (key: 'clean' | 'dirty'): Preflight =>
  contract<Preflight>('git-worktree-remove', `git:preflight.worktreeRemove#${key}`);

const OP_OK = {
  ok: true,
  error: undefined,
  undo: null,
  head: { kind: 'branch', name: 'main' },
  inProgress: null,
};

async function openWorktrees(relaunch: Parameters<typeof openPortGraph>[0], verdict: Preflight) {
  const page = await openPortGraph(relaunch, {
    chunks: singleRowChunks([rootRow(SHA_A, 'A commit')]),
    results: {
      ...PICKER_RESULTS,
      'worktree.list': {
        worktrees: [
          ...PICKER_RESULTS['worktree.list'].worktrees,
          {
            ...PICKER_RESULTS['worktree.list'].worktrees[0],
            path: WORKTREE,
            branch: 'refs/heads/wt',
            isMain: false,
            isCurrent: false,
          },
        ],
      },
      'preflight.worktreeRemove': { ...verdict, path: WORKTREE },
      'op.run': OP_OK,
    },
  });
  await tid(page, 'branch-trigger').click();
  await page.getByRole('button', { name: /^Worktrees/ }).click();
  return page;
}

test('contract: a clean worktree is removed on one op with no dialog and no token', async ({
  relaunch,
}) => {
  const page = await openWorktrees(relaunch, preflight('clean'));
  await page.getByRole('button', { name: 'Remove worktree' }).click();
  await expect
    .poll(async () => gitStreamParams(page, 'op.run'))
    .toEqual([
      expect.objectContaining({
        op: expect.objectContaining({ kind: 'worktreeRemove', path: WORKTREE, force: false }),
      }),
    ]);
  await expect(tid(page, 'git-worktree-refusal')).toHaveCount(0);
  await expect(page.getByRole('alertdialog')).toHaveCount(0);
});

test('contract: a dirty worktree shows an inline refusal and removes only on Discard changes and remove', async ({
  relaunch,
}) => {
  const dirty = preflight('dirty');
  const page = await openWorktrees(relaunch, dirty);
  await page.getByRole('button', { name: 'Remove worktree' }).click();
  await expect(tid(page, 'git-worktree-refusal')).toBeVisible();
  await expect(tid(page, 'git-worktree-discard-remove')).toHaveText('Discard changes and remove');
  expect(await gitStreamParams(page, 'op.run')).toHaveLength(0);
  await tid(page, 'git-worktree-discard-remove').click();
  await expect
    .poll(async () => gitStreamParams(page, 'op.run'))
    .toEqual([
      expect.objectContaining({
        op: expect.objectContaining({
          kind: 'worktreeRemove',
          path: WORKTREE,
          force: true,
          confirmToken: dirty.confirmToken,
        }),
      }),
    ]);
  await expect(tid(page, 'git-worktree-refusal')).toHaveCount(0);
});

test('a blocked worktree shows its reason and offers no discard', async ({ relaunch }) => {
  const page = await openWorktrees(relaunch, {
    verdict: 'blocked',
    blockers: [{ kind: 'locked', reason: 'usb' }],
  } as unknown as Preflight);
  await page.getByRole('button', { name: 'Remove worktree' }).click();
  await expect(tid(page, 'git-worktree-refusal')).toContainText('usb');
  await expect(tid(page, 'git-worktree-discard-remove')).toHaveCount(0);
  expect(await gitStreamParams(page, 'op.run')).toHaveLength(0);
});
