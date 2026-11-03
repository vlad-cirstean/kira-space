import type { Page } from '@playwright/test';
import { expect, reviewTest } from './fixtures';
import { contract } from './support/contract';
import { gitStreamParams } from './support/gitStreamMock';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { change, GIT, reviewControl, status, t, target } from './support/reviewHunkDiff';

// Re-review on edit: a reviewed file edited afterwards reads as unreviewed, one click re-marks it,
// an edit with no agent event shows within one poll, and next/previous walk the files to review.
// Backend half: adeflow TestReviewRereviewOnEdit, same contract.

const SCENARIO = 'ade-review-rereview';
interface WorkingList {
  files: { path: string }[];
  reviews: Record<string, object>;
  contentHashes: Record<string, string>;
}
const reviewed = contract<WorkingList>(SCENARIO, 'AdeTaskService.ReviewWorking#reviewed');
const changed = contract<WorkingList>(SCENARIO, 'AdeTaskService.ReviewWorking#changed');
const movedFile = contract<{
  files: { change: { path: string }; review: { kind: string; changedSinceReview: boolean } }[];
}>(SCENARIO, 'review.files#changed').files[0];

const WORKING = 'work.txt';
const COMMITTED = movedFile?.change.path ?? '';
const withWorktree = (list: WorkingList) => ({ ...list, worktree: target.worktree });

const filesFor = (moved: boolean) => ({
  branchTip: 'a'.repeat(40),
  mergeBase: 'b'.repeat(40),
  files: [
    { change: change('a.txt'), review: status('none', false) },
    { change: change('b.txt'), review: status('full', false) },
    { change: change(COMMITTED), review: status('full', moved) },
  ],
});

const markedFile = { path: WORKING, review: { kind: 'full', changedSinceReview: false } };
const diffText = 'one\n';
const workingDiff = {
  path: WORKING,
  language: 'plaintext',
  head: { kind: 'absent' },
  worktree: { kind: 'found', text: diffText, bytes: diffText.length, limitBytes: 8_388_608 },
  contentHash: changed.contentHashes[WORKING],
};
const workingFile = {
  ...contract<object>('ade-review-working-review', 'AdeTaskService.ReviewWorkingFile#unreviewed'),
  path: WORKING,
};

const REPOS = [
  { channel: IPC.windowsEnsure, response: { mode: 'git' } },
  {
    channel: IPC.codeWorkspaceListRepos,
    response: [
      {
        id: target.codeRepoId,
        name: 'web-app',
        root: '/tmp/web-app',
        repoId: target.gitRepoId,
        sortOrder: 1,
        color: 'none',
        createdAt: '2026-01-01T00:00:00.000Z',
      },
    ],
  },
];

const control = (list: WorkingList) =>
  reviewControl([
    ...REPOS,
    { channel: IPC.adeTaskReviewWorking, response: withWorktree(list) },
    { channel: IPC.adeTaskReviewWorkingStamp, response: { stamp: 'before' } },
    { channel: IPC.adeTaskReviewWorkingDiff, response: workingDiff },
    { channel: IPC.adeTaskReviewWorkingFile, response: workingFile },
    { channel: IPC.adeTaskReviewWorkingMark, response: { files: [markedFile] } },
  ]);

const git = (files: object) => ({
  ...GIT,
  extraResults: {
    ...GIT.extraResults,
    'review.files': files,
    'review.mark': { review: { kind: 'full', changedSinceReview: false } },
  },
});

const committedRow = (page: Page, name: string) =>
  page.locator(`${t('review-files-tree')} ${t('file-tree-row')}`).filter({ hasText: name });
const workingRow = (page: Page) =>
  page.locator(`${t('review-working-tree')} ${t('file-tree-row')}`).filter({ hasText: WORKING });

reviewTest(
  'edited reviewed files read as unreviewed and count as changed',
  async ({ relaunch }) => {
    const { window: page } = await relaunch({
      control: control(changed),
      gitStream: git(filesFor(true)),
    });
    await page.locator(t('ade-review-window')).waitFor();

    await expect(committedRow(page, 'a.txt')).toHaveAttribute('data-review-state', 'unreviewed');
    await expect(committedRow(page, 'b.txt')).toHaveAttribute('data-review-state', 'reviewed');
    const moved = committedRow(page, COMMITTED);
    await expect(moved).toHaveAttribute('data-review-state', 'changed');
    await expect(moved.getByRole('checkbox')).toHaveAttribute('aria-checked', 'false');
    await expect(page.locator(t('review-committed-header'))).toContainText('1 of 3 reviewed');
    await expect(page.locator(t('review-committed-changed-count'))).toHaveText(
      '1 changed since review',
    );

    const work = workingRow(page);
    await expect(work).toHaveAttribute('data-review-state', 'changed');
    await expect(work.getByRole('checkbox')).toHaveAttribute('aria-checked', 'false');
    await expect(page.locator(t('review-working-header'))).toContainText('0 of 1 reviewed');
    await expect(page.locator(t('review-working-changed-count'))).toHaveText(
      '1 changed since review',
    );
  },
);

reviewTest('one click re-marks a changed file reviewed', async ({ relaunch }) => {
  const { window: page, control: mock } = await relaunch({
    control: control(changed),
    gitStream: git(filesFor(true)),
  });
  await page.locator(t('ade-review-window')).waitFor();

  await committedRow(page, COMMITTED).getByRole('checkbox').click();
  await expect
    .poll(async () => await gitStreamParams(page, 'review.mark'))
    .toContainEqual(expect.objectContaining({ path: COMMITTED, reviewed: true }));

  await workingRow(page).getByRole('checkbox').click();
  await expect
    .poll(() => calls(mock, IPC.adeTaskReviewWorkingMark).map((c) => c.args))
    .toContainEqual({
      branchId: target.branchId,
      paths: [WORKING],
      reviewed: true,
      ranges: null,
      contentHashes: { [WORKING]: changed.contentHashes[WORKING] },
    });
});

reviewTest('an edit no agent announced shows within one poll', async ({ relaunch }) => {
  const { window: page, control: mock } = await relaunch({
    control: control(reviewed),
    gitStream: git(filesFor(false)),
  });
  await page.locator(t('ade-review-window')).waitFor();
  await expect(workingRow(page)).toHaveAttribute('data-review-state', 'reviewed');
  await expect.poll(() => calls(mock, IPC.adeTaskReviewWorkingStamp).length).toBeGreaterThan(0);
  expect(calls(mock, IPC.adeTaskReviewWorkingStamp)[0]?.args).toMatchObject({
    branchId: target.branchId,
    paths: expect.arrayContaining([WORKING]),
  });

  mock.respond(IPC.adeTaskReviewWorkingStamp, { stamp: 'after' });
  mock.respond(IPC.adeTaskReviewWorking, withWorktree(changed));
  await expect(workingRow(page)).toHaveAttribute('data-review-state', 'changed', {
    timeout: 10_000,
  });
});

reviewTest('next and previous walk the files still to review', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: control(changed),
    gitStream: git(filesFor(true)),
  });
  await page.locator(t('ade-review-window')).waitFor();
  const next = page.locator(t('review-next-unreviewed'));
  const previous = page.locator(t('review-prev-unreviewed'));

  await next.click();
  await expect(committedRow(page, 'a.txt')).toHaveAttribute('aria-selected', 'true');
  await next.click();
  await expect(committedRow(page, COMMITTED)).toHaveAttribute('aria-selected', 'true');
  await next.click();
  await expect(workingRow(page)).toHaveAttribute('aria-selected', 'true');
  await expect(page.locator(t('repo-diff-uncommitted'))).toHaveText('Uncommitted');
  await next.click();
  await expect(committedRow(page, 'a.txt')).toHaveAttribute('aria-selected', 'true');
  await previous.click();
  await expect(workingRow(page)).toHaveAttribute('aria-selected', 'true');
  await previous.click();
  await expect(committedRow(page, COMMITTED)).toHaveAttribute('aria-selected', 'true');

  // The shortcuts do the same, with focus inside the diff editor.
  await workingRow(page).click();
  await page.locator(t('repo-diff-editor')).locator('.editor.modified').click();
  await page.keyboard.press('Alt+F8');
  await expect(committedRow(page, 'a.txt')).toHaveAttribute('aria-selected', 'true');
  await page.keyboard.press('Shift+Alt+F8');
  await expect(workingRow(page)).toHaveAttribute('aria-selected', 'true');
});

reviewTest('next and previous are disabled when every file is reviewed', async ({ relaunch }) => {
  const allDone = {
    ...filesFor(false),
    files: filesFor(false).files.map((f) => ({ ...f, review: status('full', false) })),
  };
  const { window: page } = await relaunch({
    control: control(reviewed),
    gitStream: git(allDone),
  });
  await page.locator(t('ade-review-window')).waitFor();
  await expect(workingRow(page)).toHaveAttribute('data-review-state', 'reviewed');
  await expect(page.locator(t('review-next-unreviewed'))).toBeDisabled();
  await expect(page.locator(t('review-prev-unreviewed'))).toBeDisabled();
});
