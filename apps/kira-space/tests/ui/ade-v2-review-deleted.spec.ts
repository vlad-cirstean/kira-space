import type { Locator, Page } from '@playwright/test';
import { expect, reviewTest } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import {
  GIT,
  type Relaunch,
  renderedLine,
  reviewControl,
  t,
  target,
} from './support/reviewHunkDiff';

// A deleted file reads as its old text in one pane, not as a diff against an empty side.
// Backend half: reviewflow TestDeletedFileReads (committed) and adeflow TestReviewWorking
// (uncommitted), same contracts.

const WORKING = 'ade-review-working';
const workingFiles = contract<{ files: { path: string }[] }>(
  WORKING,
  'AdeTaskService.ReviewWorking',
);
const goneDiff = contract<{ path: string; head: { text: string } }>(
  WORKING,
  'AdeTaskService.ReviewWorkingDiff#deleted',
);
const COMMITTED = 'git-review-deleted';
const committedFiles = contract<{ files: { change: { path: string } }[] }>(
  COMMITTED,
  'git:review.files',
);
const committedLeft = contract<{ content: string }>(COMMITTED, 'git:file.read#left');

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

const openWorking = (relaunch: Relaunch, diff: object = goneDiff) =>
  relaunch({
    control: reviewControl([
      ...REPOS,
      {
        channel: IPC.adeTaskReviewWorking,
        response: { ...workingFiles, worktree: target.worktree },
      },
      { channel: IPC.adeTaskReviewWorkingDiff, response: diff },
    ]),
    gitStream: GIT,
  });

const openCommitted = (relaunch: Relaunch) =>
  relaunch({
    control: reviewControl(REPOS),
    gitStream: {
      ...GIT,
      extraResults: {
        ...GIT.extraResults,
        'review.files': committedFiles,
        'file.read': {
          __sequence: [committedLeft, contract(COMMITTED, 'git:file.read#right')],
        },
        'review.snapshot': { kind: 'none', text: null, reviewedAtSha: null },
        'review.fileDiff': {
          path: 'a.txt',
          deltaSource: 'noSnapshot',
          body: { kind: 'text', hunks: [] },
          reviewedRanges: [],
          lineCount: 0,
          reviewedAtSha: null,
        },
      },
    },
  });

async function expectOldTextAlone(page: Page, oldText: string): Promise<Locator> {
  const host = page.locator(t('repo-diff-editor'));
  await expect(page.locator(t('repo-diff-deleted'))).toHaveText('Deleted', { timeout: 15_000 });
  const firstLine = oldText.split('\n')[0] ?? '';
  await expect(renderedLine(host, firstLine)).toHaveCount(1);
  await expect(host.locator('.monaco-diff-editor')).toHaveCount(0);
  await expect(host.locator('.editor.modified')).toHaveCount(0);
  await expect(host.locator('.char-delete, .line-insert, .line-delete')).toHaveCount(0);
  // One tinted line per content line: the trailing newline's empty line is not a removed line.
  const contentLines = oldText.replace(/\n$/, '').split('\n').length;
  await expect(host.locator('.kira-diff-line-removed')).toHaveCount(contentLines);
  await expect(page.locator(t('repo-diff-layout-side-by-side'))).toHaveCount(0);
  await expect(page.locator(t('repo-diff-go-to-file'))).toHaveCount(0);
  await renderedLine(host, firstLine).hover();
  await expect(host.locator('.kira-review-ln-add')).toHaveCount(0);
  return host;
}

reviewTest('an uncommitted deleted file reads as its old text', async ({ relaunch }) => {
  const { window: page } = await openWorking(relaunch);
  await page.locator(t('ade-review-window')).waitFor();
  await page
    .locator(`${t('review-working-tree')} ${t('file-tree-row')}`, { hasText: goneDiff.path })
    .click();
  await expectOldTextAlone(page, goneDiff.head.text);
  await expect(page.locator(t('repo-diff-uncommitted'))).toHaveText('Uncommitted');
});

reviewTest('a committed deleted file reads as its old text', async ({ relaunch }) => {
  const { window: page } = await openCommitted(relaunch);
  await page.locator(t('ade-review-window')).waitFor();
  const path = committedFiles.files[0]?.change.path ?? '';
  await page.locator(`${t('review-files-tree')} ${t('file-tree-row')}`, { hasText: path }).click();
  await expectOldTextAlone(page, committedLeft.content);
  await expect(page.locator(t('repo-diff-uncommitted'))).toHaveCount(0);
});

reviewTest('a deleted file restored on disk switches to a diff', async ({ relaunch }) => {
  const { window: page, control: mock } = await openWorking(relaunch);
  await page.locator(t('ade-review-window')).waitFor();
  await page
    .locator(`${t('review-working-tree')} ${t('file-tree-row')}`, { hasText: goneDiff.path })
    .click();
  await expectOldTextAlone(page, goneDiff.head.text);

  mock.respond(IPC.adeTaskReviewWorkingDiff, {
    ...goneDiff,
    head: { ...goneDiff.head, text: 'gone.txt\n' },
    worktree: { kind: 'found', text: 'gone.txt\nrestored\n', bytes: 17, limitBytes: 8_388_608 },
    contentHash: 'restored',
  });
  // Two evaluates: the focus watcher sees blur then focus only when a tick separates them.
  await page.evaluate(() => window.dispatchEvent(new Event('blur')));
  await page.evaluate(() => window.dispatchEvent(new Event('focus')));
  const host = page.locator(t('repo-diff-editor'));
  await expect(host.locator('.editor.modified')).toHaveCount(1, { timeout: 15_000 });
  await expect(page.locator(t('repo-diff-deleted'))).toHaveCount(0);
});
