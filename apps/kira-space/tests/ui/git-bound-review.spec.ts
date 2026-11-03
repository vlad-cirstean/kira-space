import type { Page } from '@playwright/test';
import { expect, reviewTest, test } from './fixtures';
import { openWindow } from './support/adeReviewStep';
import { contract } from './support/contract';
import {
  gitBoundWindowKeys,
  gitStreamEmit,
  gitStreamParams,
  gitStreamRequests,
  installGitStreamMock,
} from './support/gitStreamMock';
import {
  manyRowShas,
  manyRows,
  openPortGraph,
  PORT_CONTROL,
  PORT_REPO,
  refsAtTip,
  singleRowChunks,
} from './support/gitUiPortFixtures';
import { IPC } from './support/ipcChannels';
import { repoRowById, treeRow } from './support/repoRows';
import { GIT, reviewControl, t } from './support/reviewHunkDiff';

// Frontend half of flows/gitflow/bound_review_test.go: review, change-request and worktree calls
// leave the page as bound GitService calls with this page's window key. Answers come from the flow
// test's contract file.

const SCENARIO = 'git-bound-review';
const SHA = 'a'.repeat(40);
const REVIEW_STATE = contract<Record<string, unknown>>(
  SCENARIO,
  'git:changeRequest.reviewState#bound',
);
const pageKey = (page: Page) =>
  page.evaluate(() => new URLSearchParams(location.search).get('window') ?? 'main');
const count = async (page: Page, method: string) =>
  (await gitStreamRequests(page)).filter((m) => m === method).length;

async function openReview(relaunch: Parameters<Parameters<typeof reviewTest>[2]>[0]['relaunch']) {
  const { window: page } = await relaunch({
    control: reviewControl(),
    gitStream: {
      ...GIT,
      extraResults: {
        ...GIT.extraResults,
        'repo.open': {
          kind: 'ok',
          repo: { repoId: GIT.repoId, root: GIT.repoId, head: { kind: 'branch', name: 'main' } },
        },
        'changeRequest.reviewState': { ...REVIEW_STATE, headSha: SHA, localHeadSha: SHA },
      },
    },
  });
  await page.locator(t('ade-review-window')).waitFor();
  return page;
}

reviewTest(
  'the review window lists and edits comments through GitService',
  async ({ relaunch }) => {
    const { page } = await openWindow(relaunch, 'answered');
    const key = await pageKey(page);
    await expect(page.locator(t('ade-review-comment'))).not.toHaveCount(0);
    expect(await count(page, 'repo.open')).toBeGreaterThan(0);
    expect(new Set(await gitBoundWindowKeys(page))).toEqual(new Set([key]));

    const listed = await count(page, 'review.comment.list');
    await page
      .locator(t('ade-review-comment'))
      .first()
      .locator(t('ade-review-comment-state'))
      .click();
    await expect
      .poll(() => gitStreamParams(page, 'review.comment.setState'))
      .toEqual([expect.objectContaining({ id: 1, state: 'open' })]);
    await expect.poll(() => count(page, 'review.comment.list')).toBeGreaterThan(listed);
  },
);

reviewTest('the change request head pill reads changeRequest.reviewState', async ({ relaunch }) => {
  const page = await openReview(relaunch);
  await expect(page.locator(t('ade-review-head'))).toHaveAttribute('data-head', 'current');
  expect(await gitStreamParams(page, 'changeRequest.reviewState')).not.toHaveLength(0);
});

const REPO = {
  id: 'repo-a',
  name: 'alpha-repo',
  root: '/tmp/alpha-repo',
  repoId: '/tmp/alpha-repo',
  sortOrder: 0,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
  hidden: false,
};
const WORKTREES = {
  worktrees: [
    {
      path: REPO.root,
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
  ],
};

test('expanding worktrees holds the repo; refsChanged refetches; collapse releases it', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.codeWorkspaceListRepos, response: [REPO] }],
  });
  await installGitStreamMock(page, REPO.repoId, {
    'repo.open': {
      kind: 'ok',
      repo: { repoId: REPO.repoId, root: REPO.root, head: { kind: 'branch', name: 'main' } },
    },
    'worktree.list': WORKTREES,
  });
  const expand = repoRowById(page, REPO.id).locator(t('repo-row-expand'));

  await expand.click();
  await expect(page.locator(t('repo-worktree-row'))).toHaveCount(1);
  const requests = await gitStreamRequests(page);
  expect(requests.indexOf('repo.open')).toBeGreaterThanOrEqual(0);
  expect(requests.indexOf('repo.open')).toBeLessThan(requests.indexOf('worktree.list'));
  expect(new Set(await gitBoundWindowKeys(page))).toEqual(new Set([await pageKey(page)]));

  const listed = await count(page, 'worktree.list');
  await gitStreamEmit(page, 'repo.changed', { repoId: '/tmp/elsewhere', kind: 'refsChanged' });
  // The matching event is the barrier: exactly one more list proves the other repo's caused none.
  await gitStreamEmit(page, 'repo.changed', { repoId: REPO.repoId, kind: 'refsChanged' });
  await expect.poll(() => count(page, 'worktree.list')).toBeGreaterThan(listed);
  expect(await count(page, 'worktree.list')).toBe(listed + 1);

  await expand.click();
  await expect(page.locator(t('repo-worktree-row'))).toHaveCount(0);
  await expect.poll(() => count(page, 'repo.close')).toBe(1);
  expect(await gitStreamParams(page, 'repo.close')).toEqual([{ repoId: REPO.repoId }]);
});

test('the file blame goes out as a bound BlameLine with the page window key', async ({
  relaunch,
}) => {
  const page = await openPortGraph(relaunch, {
    chunks: singleRowChunks(manyRows(300)),
    results: {
      'refs.list': refsAtTip(manyRowShas(0)),
      'blame.line': {
        sha: manyRowShas(250),
        author: 'Fake Author',
        summary: 'Row 250',
        authorTimeSeconds: 1_700_000_000,
      },
    },
    control: [
      ...PORT_CONTROL,
      {
        channel: IPC.codeWorkspaceReadFile,
        args: { id: PORT_REPO.id, path: 'a.ts' },
        response: {
          kind: 'found',
          text: 'export const a = 1;\nexport const b = 2;\n',
          bytes: 40,
          limitBytes: 8 * 1024 * 1024,
          language: 'typescript',
        },
      },
    ],
  });
  await page.locator('[data-testid="git-panel-tab-files"]').click();
  await treeRow(page, 'a.ts').click();
  await page.locator('[data-testid="repo-file-editor"] .view-lines').click();
  await page.keyboard.press('ArrowDown');
  await expect.poll(() => count(page, 'blame.line')).toBeGreaterThan(0);
  expect((await gitBoundWindowKeys(page)).length).toBeGreaterThan(0);
  expect(new Set(await gitBoundWindowKeys(page))).toEqual(new Set([await pageKey(page)]));
});
