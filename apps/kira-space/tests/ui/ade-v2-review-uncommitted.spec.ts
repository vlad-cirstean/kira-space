import { expect, reviewTest, test } from './fixtures';
import { adeBoard, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import { GIT, reviewControl, t, target } from './support/reviewHunkDiff';
import type { ControlSnapshot } from './support/types';

// Review code on a branch with only uncommitted changes, and the window that opens.
// Backend half: adeflow TestReviewUncommittedOnly, same contract.

const SCENARIO = 'ade-review-uncommitted';
const card = (id: string) => `[data-testid="ade-card"][data-task-id="${id}"]`;
const tip = '[data-slot="tooltip-content"]';
const OPEN: ControlSnapshot = { channel: IPC.adeTaskOpenReviewWindow, response: true };

type Facts = { name: string; ahead: number; base: string; dirty: unknown[] };
// Only the backend's own facts: the rest of the fixture branch (its ids) stays.
const boardWith = (facts: Pick<Facts, 'ahead' | 'dirty'> & Partial<Facts>): ControlSnapshot => ({
  channel: IPC.adeTaskBoard,
  response: adeBoard((b) => {
    for (const r of b.tasks.find((x) => x.id === 'T_deps')?.runs ?? []) r.state = 'done';
    const br = b.branches.find((x) => x.id === 'b_deps');
    if (!br) throw new Error('fixture branch missing');
    const { name, ahead, base, dirty } = facts;
    Object.assign(br, { ...(name ? { name, base } : {}), ahead, dirty });
  }),
});
const openCalls = (control: { log(): { channel: string; args?: unknown }[] }) =>
  control.log().filter((e) => e.channel === IPC.adeTaskOpenReviewWindow);

test('Review code opens a branch with only uncommitted changes', async ({ relaunch }) => {
  const facts = contract<Facts>(SCENARIO, 'AdeTaskService.Branch#uncommittedOnly');
  const { window: page, control } = await openPlan(relaunch, [OPEN, boardWith(facts)]);
  const btn = page.locator(`${card('T_deps')} ${t('ade-card-review')}`);
  await expect(btn).toBeEnabled();
  await btn.locator('xpath=..').hover();
  const tooltip = page.locator(tip);
  await expect(tooltip).toContainText('uncommitted change');
  await expect(tooltip).toContainText('no commits on top of');
  await expect(tooltip.locator('[data-var="branch"]')).toHaveText(facts.name);
  await expect(tooltip.locator('[data-var="base"]')).toHaveText(facts.base);
  await page.mouse.move(0, 0);

  await page.locator(`${card('T_deps')} ${t('ade-card-head')}`).click();
  await expect(page.locator(t('ade-panel-review'))).toBeEnabled();
  await btn.click();
  await expect.poll(() => openCalls(control)).toHaveLength(1);
  // The contract's args carry the backend's id; the fixture branch keeps its own.
  const sent = openCalls(control)[0]?.args as object;
  expect(Object.keys(sent)).toEqual(
    Object.keys(contract(SCENARIO, 'args:AdeTaskService.OpenReviewWindow')),
  );
  expect(sent).toEqual({ branchId: 'b_deps' });
});

test('a mixed branch says it adds uncommitted changes', async ({ relaunch }) => {
  const facts = contract<Facts>(SCENARIO, 'AdeTaskService.Branch#mixed');
  const { window: page } = await openPlan(relaunch, [OPEN, boardWith(facts)]);
  const btn = page.locator(`${card('T_deps')} ${t('ade-card-review')}`);
  await expect(btn).toBeEnabled();
  await btn.locator('xpath=..').hover();
  await expect(page.locator(tip)).toContainText('plus 1 uncommitted change');
});

test('a branch with nothing to review stays disabled', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    OPEN,
    boardWith({ ahead: 0, dirty: [] }),
  ]);
  const btn = page.locator(`${card('T_deps')} ${t('ade-card-review')}`);
  await expect(btn).toBeDisabled();
  await btn.locator('xpath=..').hover();
  await expect(page.locator(tip)).toContainText('no uncommitted changes');
  await emitWailsEvent(page, IPC.reviewCode, null);
  expect(openCalls(control)).toHaveLength(0);
});

const working = contract<{ files: { path: string }[]; contentHashes: Record<string, string> }>(
  SCENARIO,
  'AdeTaskService.ReviewWorking#uncommittedOnly',
);
// The contract masks the base, so the fixture names it.
const emptyRange = {
  ...contract<object>(SCENARIO, 'git:review.resolveBase#uncommittedOnly'),
  base: 'main',
};
const git = (range: object) => ({
  ...GIT,
  extraResults: { ...GIT.extraResults, 'review.resolveBase': range },
});
const sideText = { kind: 'found', text: 'x\n', bytes: 2, limitBytes: 8_388_608 };
const diff = {
  path: working.files[0]?.path ?? '',
  language: 'plaintext',
  head: sideText,
  worktree: { ...sideText, text: 'y\n' },
  contentHash: working.contentHashes[working.files[0]?.path ?? ''],
};
const REPOS: ControlSnapshot[] = [
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
const control = (files: object[]) =>
  reviewControl([
    ...REPOS,
    {
      channel: IPC.adeTaskReviewWorking,
      response: { ...working, files, worktree: target.worktree },
    },
    { channel: IPC.adeTaskReviewWorkingComments, response: { comments: [] } },
    { channel: IPC.adeTaskReviewWorkingDiff, response: diff },
  ]);

reviewTest('the review window shows uncommitted work with no commits', async ({ relaunch }) => {
  const { window: page, control: mock } = await relaunch({
    control: control(working.files),
    gitStream: git(emptyRange),
  });
  await page.locator(t('ade-review-window')).waitFor();
  const section = page.locator(t('review-working'));
  await expect(section).toBeVisible();
  const path = working.files[0]?.path ?? '';
  await expect(section.locator(t('file-tree-row'))).toContainText(path.split('/').pop() ?? path);
  await expect(page.locator(t('review-empty'))).toContainText('adds no commits to “main”');
  await expect(page.locator(t('review-pane-files'))).toBeVisible();
  await expect(page.locator(t('review-pane-comments'))).toBeVisible();
  await expect(page.locator(t('review-pane-commits'))).toHaveCount(0);
  await expect(page.locator(t('review-committed-header'))).toHaveCount(0);

  await section.locator(t('file-tree-row')).first().click();
  await expect(page.locator(t('repo-diff-uncommitted'))).toHaveText('Uncommitted');
  expect(calls(mock, IPC.adeTaskReviewWorking).length).toBeGreaterThan(0);
});

reviewTest('no commits and a clean worktree keep the empty text', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: control([]), gitStream: git(emptyRange) });
  await page.locator(t('ade-review-window')).waitFor();
  await expect(page.locator(t('review-empty'))).toBeVisible();
  await expect(page.locator(t('review-toolbar'))).toHaveCount(0);
  await expect(page.locator(t('review-working'))).toHaveCount(0);
});
