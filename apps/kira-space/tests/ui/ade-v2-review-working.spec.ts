import { AGENT_PANEL_ON, expect, reviewTest } from './fixtures';
import { contract } from './support/contract';
import { gitStreamParams } from './support/gitStreamMock';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { GIT, renderedLine, reviewControl, t, target } from './support/reviewHunkDiff';

// Uncommitted review: marks and comments on the worktree, apart from the committed review.
// Backend half: adeflow TestReviewWorkingReview, same contract.

const SCENARIO = 'ade-review-working-review';
const working = contract<{
  files: { path: string }[];
  reviews: Record<string, object>;
  contentHashes: Record<string, string>;
}>(SCENARIO, 'AdeTaskService.ReviewWorking');
const file = (variant: 'reviewed' | 'unreviewed') =>
  contract<{
    path: string;
    body: { hunks: { lines: { kind: string; text: string }[] }[] };
    reviewedRanges: { start: number; end: number }[];
  }>(SCENARIO, `AdeTaskService.ReviewWorkingFile#${variant}`);
const shown = contract<{ contentHash: string }>(SCENARIO, 'AdeTaskService.ReviewWorkingDiff');
const markResult = contract<{ files: { path: string; review: object }[] }>(
  SCENARIO,
  'AdeTaskService.ReviewWorkingMark',
);
const markArgs = contract<object>(SCENARIO, 'args:AdeTaskService.ReviewWorkingMark');
const addArgs = contract<object>(SCENARIO, 'args:AdeTaskService.ReviewWorkingCommentAdd');
const added = contract<object>(SCENARIO, 'AdeTaskService.ReviewWorkingCommentAdd');
const listed = contract<{ comments: { path: string; range: { start: number } }[] }>(
  SCENARIO,
  'AdeTaskService.ReviewWorkingComments',
);

const PATH = file('unreviewed').path;
const texts = (kinds: string[]) =>
  file('unreviewed')
    .body.hunks.flatMap((h) => h.lines)
    .filter((l) => kinds.includes(l.kind))
    .map((l) => `${l.text}\n`)
    .join('');
const diff = {
  path: PATH,
  language: 'plaintext',
  head: { kind: 'found', text: texts(['context', 'del']), bytes: 1, limitBytes: 8_388_608 },
  worktree: { kind: 'found', text: texts(['context', 'add']), bytes: 1, limitBytes: 8_388_608 },
  contentHash: shown.contentHash,
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

const control = (comments: object[] = []) =>
  reviewControl([
    ...REPOS,
    { channel: IPC.adeTaskReviewWorking, response: { ...working, worktree: target.worktree } },
    { channel: IPC.adeTaskReviewWorkingDiff, response: diff },
    { channel: IPC.adeTaskReviewWorkingFile, response: file('unreviewed') },
    { channel: IPC.adeTaskReviewWorkingMark, response: markResult },
    { channel: IPC.adeTaskReviewWorkingComments, response: { comments } },
    { channel: IPC.adeTaskReviewWorkingCommentAdd, response: added },
  ]);

reviewTest('the agent panel is off by default and the header toggles it', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: reviewControl(), gitStream: GIT });
  await page.locator(t('ade-review-window')).waitFor();
  await expect(page.locator(t('ade-review-right'))).toHaveCount(0);
  await page.locator(t('ade-review-toggle-agent')).click();
  await expect(page.locator(t('ade-review-right'))).toBeVisible();
  await page.locator(t('ade-review-toggle-agent')).click();
  await expect(page.locator(t('ade-review-right'))).toHaveCount(0);
});

reviewTest('a seeded panel preference shows the agent panel', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: reviewControl(),
    gitStream: GIT,
    storage: AGENT_PANEL_ON,
  });
  await page.locator(t('ade-review-window')).waitFor();
  await expect(page.locator(t('ade-review-right'))).toBeVisible();
});

reviewTest('committed and uncommitted files sit in separate sections', async ({ relaunch }) => {
  const { window: page, control: mock } = await relaunch({ control: control(), gitStream: GIT });
  await page.locator(t('ade-review-window')).waitFor();
  const committed = page.locator(t('review-committed-header'));
  await expect(committed).toContainText('Committed');
  await expect(committed).toContainText('2 of 3 reviewed');
  const header = page.locator(t('review-working-header'));
  await expect(header).toContainText('Uncommitted');
  await expect(header).toContainText(`0 of ${working.files.length} reviewed`);

  const rows = page.locator(`${t('review-working-tree')} ${t('file-tree-row')}`);
  await expect(rows).toHaveCount(working.files.length);
  const box = rows.first().getByRole('checkbox');
  await expect(box).toHaveCount(1);
  // The contract's file is partly reviewed: a click on its box clears it.
  await expect(box).toHaveAttribute('data-state', 'indeterminate');
  await box.click();
  await expect
    .poll(() => calls(mock, IPC.adeTaskReviewWorkingMark).map((c) => c.args))
    .toContainEqual({
      branchId: target.branchId,
      paths: [working.files[0]?.path],
      reviewed: false,
      ranges: null,
      contentHashes: {
        [working.files[0]?.path ?? '']: working.contentHashes[working.files[0]?.path ?? ''],
      },
    });
});

reviewTest('a working tab is badged and its hunk mark reviews the lines', async ({ relaunch }) => {
  const { window: page, control: mock } = await relaunch({ control: control(), gitStream: GIT });
  await page.locator(t('ade-review-window')).waitFor();
  await page
    .locator(`${t('review-working-tree')} ${t('file-tree-row')}`)
    .first()
    .click();
  await expect(page.locator(t('repo-diff-uncommitted'))).toHaveText('Uncommitted');
  const modified = page.locator(t('repo-diff-editor')).locator('.editor.modified');
  await expect(renderedLine(modified, 'C!')).toHaveCount(1, { timeout: 15_000 });
  const mark = modified.getByTestId('review-hunk-mark');
  await expect(mark).toHaveAttribute('data-state', 'actionable');
  await mark.click();
  await expect.poll(() => calls(mock, IPC.adeTaskReviewWorkingMark).length).toBeGreaterThan(0);
  expect(calls(mock, IPC.adeTaskReviewWorkingMark)[0]?.args).toMatchObject({
    branchId: target.branchId,
    paths: [PATH],
    reviewed: true,
    ranges: (markArgs as { ranges: object[] }).ranges,
    contentHashes: { [PATH]: shown.contentHash },
  });
  // The committed review never hears about it.
  expect(await gitStreamParams(page, 'review.mark')).toHaveLength(0);
});

reviewTest(
  'a working comment is composed on the worktree and listed as uncommitted',
  async ({ relaunch }) => {
    const { window: page, control: mock } = await relaunch({ control: control(), gitStream: GIT });
    await page.locator(t('ade-review-window')).waitFor();
    await page
      .locator(`${t('review-working-tree')} ${t('file-tree-row')}`)
      .first()
      .click();
    const modified = page.locator(t('repo-diff-editor')).locator('.editor.modified');
    const line = renderedLine(modified, 'C!');
    await expect(line).toHaveCount(1, { timeout: 15_000 });
    await line.hover();
    await modified.locator('.kira-review-ln-add').click();
    const thread = page.locator(t('review-thread'));
    await expect(thread.getByTestId('review-thread-kind-question')).toBeVisible();
    await expect(thread.getByTestId('review-thread-kind-host')).toHaveCount(0);
    await thread.getByTestId('review-thread-input').fill((addArgs as { body: string }).body);
    await thread.getByTestId('review-thread-send').click();
    await expect
      .poll(() => calls(mock, IPC.adeTaskReviewWorkingCommentAdd).length)
      .toBeGreaterThan(0);
    expect(calls(mock, IPC.adeTaskReviewWorkingCommentAdd)[0]?.args).toMatchObject({
      branchId: target.branchId,
      path: PATH,
      body: (addArgs as { body: string }).body,
      contentHash: shown.contentHash,
    });
    expect(await gitStreamParams(page, 'review.comment.add')).toHaveLength(0);
  },
);

reviewTest('a working comment paints on its line and carries the badge', async ({ relaunch }) => {
  const comment = listed.comments[0];
  if (!comment) throw new Error('contract has no comment');
  const { window: page } = await relaunch({
    control: control(listed.comments),
    gitStream: {
      ...GIT,
      extraResults: {
        ...GIT.extraResults,
        'review.comment.list': {
          at: 'a'.repeat(40),
          comments: listed.comments.map((c, i) => ({ ...c, id: i + 1 })),
        },
      },
    },
  });
  await page.locator(t('ade-review-window')).waitFor();
  await page
    .locator(`${t('review-working-tree')} ${t('file-tree-row')}`)
    .first()
    .click();
  const modified = page.locator(t('repo-diff-editor')).locator('.editor.modified');
  await expect(modified.locator('.kira-review-ln-comment')).toHaveText(
    String(comment.range.start),
    { timeout: 15_000 },
  );
  await page.locator(t('review-pane-comments')).click();
  await expect(page.locator(t('review-comment-uncommitted'))).toHaveText('Uncommitted');
});

reviewTest('a refused hunk mark reloads the file and tells the user', async ({ relaunch }) => {
  const refusal = JSON.parse(
    contract<string>(SCENARIO, 'error:AdeTaskService.ReviewWorkingMark#changed'),
  ) as { code: string; message: string };
  const base = control();
  const { window: page, control: mock } = await relaunch({
    control: [
      ...base.filter((e: { channel: string }) => e.channel !== IPC.adeTaskReviewWorkingMark),
      { channel: IPC.adeTaskReviewWorkingMark, error: refusal },
    ],
    gitStream: GIT,
  });
  await page.locator(t('ade-review-window')).waitFor();
  await page
    .locator(`${t('review-working-tree')} ${t('file-tree-row')}`)
    .first()
    .click();
  const modified = page.locator(t('repo-diff-editor')).locator('.editor.modified');
  await expect(renderedLine(modified, 'C!')).toHaveCount(1, { timeout: 15_000 });
  const diffReads = () => calls(mock, IPC.adeTaskReviewWorkingDiff).length;
  const before = diffReads();
  await modified.getByTestId('review-hunk-mark').click();
  await expect(page.locator(t('repo-diff-mark-refused'))).toBeVisible();
  // The refusal re-reads the file, so text, hunks and hash come from one fresh read.
  await expect.poll(diffReads).toBeGreaterThan(before);
});

reviewTest('a refused list mark tells the user and reloads the list', async ({ relaunch }) => {
  const refusal = JSON.parse(
    contract<string>(SCENARIO, 'error:AdeTaskService.ReviewWorkingMark#changed'),
  ) as { code: string; message: string };
  const { window: page, control: mock } = await relaunch({
    control: [
      ...control().filter((e) => e.channel !== IPC.adeTaskReviewWorkingMark),
      { channel: IPC.adeTaskReviewWorkingMark, error: refusal },
    ],
    gitStream: GIT,
  });
  await page.locator(t('ade-review-window')).waitFor();
  const listReads = () => calls(mock, IPC.adeTaskReviewWorking).length;
  await expect.poll(listReads).toBeGreaterThan(0);
  const before = listReads();
  await page
    .locator(`${t('review-working-tree')} ${t('file-tree-row')}`)
    .first()
    .getByRole('checkbox')
    .click();
  await expect(page.locator(t('ade-review-files-mark-refused'))).toBeVisible();
  await expect.poll(listReads).toBeGreaterThan(before);
});
