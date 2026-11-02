import type { Locator, Page } from '@playwright/test';
import { AGENT_PANEL_ON, expect, reviewTest, test } from './fixtures';
import { adeFixture, emitAgentEvent, emitAgentSessions, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { gitStreamEmit, gitStreamParams, gitStreamRequests } from './support/gitStreamMock';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import {
  boxOf,
  change,
  GIT,
  HUNK_COMMENT,
  openHunkDiff,
  REVIEW_FILES,
  renderedLine,
  reviewControl,
  status,
  t,
  target,
} from './support/reviewHunkDiff';
import type { ControlSnapshot } from './support/types';

// The review window: three panes for one branch, and the review agent.

const agent = adeFixture<{ session: { terminalId: string } }>('review-agent');

async function openReview(
  relaunch: Parameters<Parameters<typeof reviewTest>[2]>[0]['relaunch'],
  extra: readonly ControlSnapshot[] = [],
  git: Record<string, unknown> = {},
): Promise<Page> {
  const { window: page } = await relaunch({
    storage: AGENT_PANEL_ON,
    control: reviewControl(extra),
    gitStream: { ...GIT, extraResults: { ...GIT.extraResults, ...git } },
  });
  await page.locator(t('ade-review-window')).waitFor();
  return page;
}

reviewTest('a review window boots three panes for its branch', async ({ relaunch }) => {
  const page = await openReview(relaunch);
  await expect(page.locator(t('ade-review-branch'))).toHaveText('feat/billing-dashboard');
  await expect(page.locator(t('ade-review-base'))).toHaveText('Base main');
  await expect(page.locator(t('ade-review-left'))).toBeVisible();
  await expect(page.locator(t('ade-review-centre'))).toBeVisible();
  await expect(page.locator(t('ade-review-right'))).toBeVisible();
  await expect(page.locator(t('ade-review-empty'))).toBeVisible();
  // One title bar: the identity sits inside the drag strip, no second bar.
  const bar = page.locator('.wails-drag');
  await expect(bar.locator(t('ade-review-header'))).toBeVisible();
  await expect(bar.locator(t('ade-review-title'))).toBeVisible();
  await expect(bar.locator(t('ade-review-branch'))).toBeVisible();
  expect((await bar.boundingBox())?.height).toBe(34);
  await expect(page.getByText('Review · ')).toHaveCount(0);
});

test('any other window boots the normal workbench', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await expect(page.locator(t('ade-review-window'))).toHaveCount(0);
});

reviewTest('the files pane lists every changed file like the git module', async ({ relaunch }) => {
  const page = await openReview(relaunch);
  const left = page.locator(t('ade-review-left'));
  await expect(left).toContainText('pending.ts');
  await expect(left).toContainText('moved.ts');
  await expect(left).toContainText('done.ts');
  await expect(left.getByText('Needs review')).toHaveCount(0);
});

reviewTest("the left bar stays on the window's branch", async ({ relaunch }) => {
  const page = await openReview(relaunch);
  await expect(page.locator(t('review-branch-name'))).toHaveText('feat/billing-dashboard');
  await expect(page.locator(t('review-base-name'))).toHaveText('main');
  for (const id of ['review-back-button', 'review-swap-button', 'base-selector']) {
    await expect(page.locator(t(id))).toHaveCount(0);
  }
  await page.locator(t('review-pane-commits')).click();
  await page.locator(t('review-pane-files')).click();
  expect(await gitStreamRequests(page)).not.toContain('review.session.save');
});

reviewTest('a review diff opens side by side', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    storage: AGENT_PANEL_ON,
    control: reviewControl([
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
    ]),
    gitStream: {
      ...GIT,
      extraResults: {
        ...GIT.extraResults,
        'file.read': { kind: 'found', content: 'export const a = 1;\n' },
        'review.snapshot': { kind: 'none', text: null, reviewedAtSha: null },
        'review.fileDiff': {
          path: 'src/pending.ts',
          deltaSource: 'noSnapshot',
          body: { kind: 'text', hunks: [] },
          reviewedRanges: [],
          lineCount: 1,
          reviewedAtSha: null,
        },
      },
    },
  });
  await page.locator(t('ade-review-window')).waitFor();
  await page
    .locator(`${t('review-files-tree')} ${t('file-tree-row')}`, { hasText: 'pending.ts' })
    .click();
  const editor = page.locator(t('repo-diff-editor'));
  await expect(editor.locator('.monaco-diff-editor .editor.original')).toBeVisible();
  const width = await editor
    .locator('.monaco-diff-editor .editor.original')
    .evaluate((el) => el.getBoundingClientRect().width);
  expect(width).toBeGreaterThan(100);
});

reviewTest('a review diff shows only the hunk and the commented line', async ({ relaunch }) => {
  const { page, modified } = await openHunkDiff(relaunch);
  const lines = modified.locator('.view-lines .view-line');
  const rendered = (text: string) =>
    lines.filter({ hasText: new RegExp(`^\\s*${text.replace(' ', '\\s')}$`) });

  await expect(rendered('CHANGED 100')).toHaveCount(1, { timeout: 15_000 });
  for (const n of [97, 98, 99, 101, 102, 103, 12]) {
    await expect(rendered(`line ${n}`)).toHaveCount(1);
  }
  await expect(modified.locator('.kira-review-ln-comment')).toHaveCount(1);
  for (const n of [50, 150]) await expect(rendered(`line ${n}`)).toHaveCount(0);
  expect(await lines.count()).toBeLessThan(40);
  const placeholders = modified.locator('.diff-hidden-lines');
  await expect(placeholders).toHaveCount(2);

  await page.screenshot({ path: 'test-results/review-hunks-only.png' });

  const before = await lines.count();
  await placeholders.last().locator('a').first().click();
  await expect.poll(() => lines.count()).toBeGreaterThan(before);
});

reviewTest(
  'the review gutter has no glyph margin and a hunk row holds the mark',
  async ({ relaunch }) => {
    const { page, modified } = await openHunkDiff(relaunch);
    const original = page.locator(t('repo-diff-editor')).locator('.editor.original');
    await expect(renderedLine(modified, 'CHANGED 100')).toHaveCount(1, { timeout: 15_000 });
    for (const pane of [original, modified]) {
      const margin = pane.locator('.glyph-margin');
      expect((await margin.count()) === 0 || (await boxOf(margin)).width === 0).toBe(true);
    }
    const mark = modified.getByTestId('review-hunk-mark');
    await expect(mark).toHaveAttribute('data-state', 'actionable');
    const markBox = await boxOf(mark);
    const line99 = await boxOf(renderedLine(modified, 'line 99'));
    const changed = await boxOf(renderedLine(modified, 'CHANGED 100'));
    expect(markBox.y).toBeGreaterThan(line99.y);
    expect(markBox.y + markBox.height).toBeLessThanOrEqual(changed.y + 2);
    // The row is one line tall, and the left pane keeps its line 100 level with CHANGED 100.
    expect(changed.y - line99.y).toBeGreaterThan(line99.height + 8);
    expect((await boxOf(renderedLine(original, 'line 100'))).y).toBe(changed.y);

    await mark.click();
    await expect
      .poll(async () => (await gitStreamParams(page, 'review.mark')).length)
      .toBeGreaterThan(0);
    expect((await gitStreamParams(page, 'review.mark'))[0]).toMatchObject({
      path: 'src/pending.ts',
      reviewed: true,
    });
  },
);

reviewTest('the hunk row stays above the deleted lines inline', async ({ relaunch }) => {
  const { page, modified } = await openHunkDiff(relaunch);
  await expect(renderedLine(modified, 'CHANGED 100')).toHaveCount(1, { timeout: 15_000 });
  await page.locator(t('repo-diff-layout-inline')).click();
  const mark = page.getByTestId('review-hunk-mark');
  await expect(mark).toBeVisible();
  const markBox = await boxOf(mark);
  const deleted = await boxOf(page.locator('.view-lines.line-delete .view-line'));
  expect(markBox.y + markBox.height).toBeLessThanOrEqual(deleted.y + 2);
  expect(markBox.y).toBeGreaterThan((await boxOf(renderedLine(modified, 'line 99'))).y);
});

reviewTest(
  'the add mark replaces the hovered line number and opens the composer',
  async ({ relaunch }) => {
    const { page, modified } = await openHunkDiff(relaunch, { comments: [] });
    const line = renderedLine(modified, 'CHANGED 100');
    await expect(line).toHaveCount(1, { timeout: 15_000 });
    const number = modified.locator('.line-numbers').filter({ hasText: /^100$/ });
    const before = await boxOf(number);
    const contentLeft = (await boxOf(modified.locator('.view-lines'))).x;

    await line.hover();
    const add = modified.locator('.kira-review-ln-add');
    await expect(add).toHaveCount(1);
    expect(await add.evaluate((el) => getComputedStyle(el).color)).toBe('rgba(0, 0, 0, 0)');
    expect(await boxOf(add)).toEqual(before);
    expect((await boxOf(modified.locator('.view-lines'))).x).toBe(contentLeft);

    await add.click();
    const thread = page.locator(t('review-thread'));
    await expect(thread).toBeVisible();
    await expect(thread.getByTestId('review-thread-input')).toBeFocused();
    await page.mouse.move(0, 0);
    await expect(modified.locator('.kira-review-ln-add')).toHaveCount(0);
  },
);

reviewTest('a commented line number is tinted and opens its thread', async ({ relaunch }) => {
  const { page, modified } = await openHunkDiff(relaunch);
  await expect(modified.locator('.kira-review-ln-comment')).toHaveText('12', { timeout: 15_000 });
  await modified.locator('.kira-review-ln-comment').click();
  const thread = page.locator(t('review-thread'));
  await expect(thread.getByTestId('review-thread-body')).toHaveText('Why this?');
  await expect(thread.getByTestId('review-thread-kind-chip')).toHaveText('Change');
  await thread.getByRole('button', { name: 'Close' }).click();
  await expect(thread).toHaveCount(0);
});

async function openDiffWindow(
  relaunch: Parameters<Parameters<typeof reviewTest>[2]>[0]['relaunch'],
  files: readonly string[],
  extraResults: Record<string, unknown>,
): Promise<Page> {
  const { window: page } = await relaunch({
    storage: AGENT_PANEL_ON,
    control: reviewControl([
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
    ]),
    gitStream: {
      ...GIT,
      extraResults: {
        ...GIT.extraResults,
        'review.files': {
          ...REVIEW_FILES,
          files: files.map((path) => ({ change: change(path), review: status('none', false) })),
        },
        'review.snapshot': { kind: 'none', text: null, reviewedAtSha: null },
        'review.fileDiff': {
          path: files[0],
          deltaSource: 'noSnapshot',
          body: { kind: 'text', hunks: [] },
          reviewedRanges: [],
          lineCount: 200,
          reviewedAtSha: null,
        },
        ...extraResults,
      },
    },
  });
  await page.locator(t('ade-review-window')).waitFor();
  return page;
}

reviewTest('a folder checkbox marks every file under it', async ({ relaunch }) => {
  const marked = contract<{ results: { path: string; review: object }[] }>(
    'git-review-folder',
    'git:review.markMany#mark',
  );
  const fullReview = marked.results[0]?.review ?? {};
  const noneReview = { kind: 'none', changedSinceReview: false };
  const result = (reviewed: boolean, paths: string[]) => ({
    results: paths.map((path) => ({ path, review: reviewed ? fullReview : noneReview })),
  });
  const all = ['src/pending.ts', 'src/done.ts', 'src/moved.ts'];
  const { window: mixed } = await relaunch({
    storage: AGENT_PANEL_ON,
    control: reviewControl(),
    gitStream: {
      ...GIT,
      extraResults: {
        ...GIT.extraResults,
        'review.markMany': {
          __sequence: [result(true, ['src/moved.ts', 'src/pending.ts']), result(false, all)],
        },
      },
    },
  });
  await mixed.locator(t('ade-review-window')).waitFor();
  const tree = mixed.locator(t('review-files-tree'));
  const folder = tree.locator(t('review-folder-check'));
  const files = tree.locator(`${t('file-tree-row')} button[role="checkbox"]`);
  await expect(folder).toHaveAttribute('aria-checked', 'mixed');

  await folder.click();
  await expect(folder).toHaveAttribute('aria-checked', 'true');
  await expect(files).toHaveCount(4);
  for (const i of [0, 1, 2, 3]) await expect(files.nth(i)).toHaveAttribute('aria-checked', 'true');
  await mixed.screenshot({ path: 'test-results/review-folder-marked.png' });

  await folder.click();
  await expect(folder).toHaveAttribute('aria-checked', 'false');
  const params = (await gitStreamParams(mixed, 'review.markMany')) as {
    paths: string[];
    reviewed: boolean;
  }[];
  expect(params.map((p) => [p.paths, p.reviewed])).toEqual([
    // The edited-since-review file is unreviewed, so a folder click marks it again.
    [['src/moved.ts', 'src/pending.ts'], true],
    [['src/done.ts', 'src/moved.ts', 'src/pending.ts'], false],
  ]);
});

reviewTest('next and previous change walk hunks and cross files', async ({ relaunch }) => {
  const read = (file: 'a' | 'b', side: 'left' | 'right') =>
    contract('git-review-nav', `git:file.read#${file}-${side}`);
  const page = await openDiffWindow(relaunch, ['src/a.txt', 'src/b.txt'], {
    'file.read': {
      __sequence: [
        read('a', 'left'),
        read('a', 'right'),
        read('b', 'left'),
        read('b', 'right'),
        read('a', 'left'),
        read('a', 'right'),
      ],
    },
  });
  await page
    .locator(`${t('review-files-tree')} ${t('file-tree-row')}`, { hasText: 'a.txt' })
    .click();
  const modified = page.locator(t('repo-diff-editor')).locator('.editor.modified');
  const active = modified.locator('.line-numbers.active-line-number');
  const next = page.locator(t('repo-diff-next-change'));
  const previous = page.locator(t('repo-diff-prev-change'));
  await expect(modified.locator('.view-lines')).toContainText('CHANGED 20');
  await expect(previous).toBeDisabled();

  await next.click();
  await expect(active).toHaveText('20');
  await next.click();
  await expect(active).toHaveText('150');
  await next.click();
  await expect(modified.locator('.view-lines')).toContainText('CHANGED 60');
  await expect(active).toHaveText('60');
  await expect(next).toBeDisabled();
  await page.screenshot({ path: 'test-results/review-change-nav.png' });

  await previous.click();
  await expect(modified.locator('.view-lines')).toContainText('CHANGED 150');
  await expect(active).toHaveText('150');
  await previous.click();
  await expect(active).toHaveText('20');
  await expect(previous).toBeDisabled();
});

reviewTest('the diff layout toggle switches to inline and persists', async ({ relaunch }) => {
  const page = await openDiffWindow(relaunch, ['src/a.txt'], {
    'file.read': {
      __sequence: [
        contract('git-review-nav', 'git:file.read#a-left'),
        contract('git-review-nav', 'git:file.read#a-right'),
      ],
    },
  });
  const open = () =>
    page.locator(`${t('review-files-tree')} ${t('file-tree-row')}`, { hasText: 'a.txt' }).click();
  const diff = page.locator(t('repo-diff-editor')).locator('.monaco-diff-editor');
  await open();
  await expect(diff).toHaveClass(/side-by-side/);

  await page.locator(t('repo-diff-layout-inline')).click();
  await expect(diff).not.toHaveClass(/side-by-side/);
  await expect(page.locator(t('repo-diff-layout-inline'))).toHaveAttribute('data-state', 'on');
  expect(await page.evaluate(() => localStorage.getItem('kira.repo.diffLayout'))).toBe('inline');
  await page.screenshot({ path: 'test-results/review-diff-inline.png' });

  await page.reload();
  await page.locator(t('ade-review-window')).waitFor();
  await open();
  await expect(diff).not.toHaveClass(/side-by-side/);

  await page.locator(t('repo-diff-layout-side-by-side')).click();
  await expect(diff).toHaveClass(/side-by-side/);
});

const working = contract<{ files: object[]; head: string; worktree: string }>(
  'ade-review-working',
  'AdeTaskService.ReviewWorking',
);
const workingResponse = { ...working, worktree: target.worktree };
const workingDiff = contract<{ path: string }>(
  'ade-review-working',
  'AdeTaskService.ReviewWorkingDiff',
);
const withWorking = [
  { channel: IPC.adeTaskReviewWorking, response: workingResponse },
  { channel: IPC.adeTaskReviewWorkingDiff, response: workingDiff },
];

// Contract ade-review-working, backend half: adeflow TestReviewWorking.
reviewTest('a new commit reloads the files pane', async ({ relaunch }) => {
  const page = await openReview(relaunch);
  const loads = async () => (await gitStreamRequests(page)).filter((r) => r === 'review.files');
  await expect.poll(async () => (await loads()).length).toBeGreaterThan(0);
  const before = (await loads()).length;
  await gitStreamEmit(page, 'repo.changed', {
    repoId: target.gitRepoId,
    kind: 'refsChanged',
  });
  await expect.poll(async () => (await loads()).length).toBeGreaterThan(before);
});

reviewTest('uncommitted changes sit in their own section', async ({ relaunch }) => {
  const page = await openReview(relaunch, withWorking);
  const section = page.locator(t('review-working'));
  await expect(section.locator(t('file-tree-row'))).toHaveCount(working.files.length);
  await expect(section.locator(t('file-tree-row'), { hasText: 'keep.txt' })).toBeVisible();
  await expect(page.locator(t('review-files-tree'))).not.toContainText('keep.txt');
  await expect(page.locator(t('ade-review-uncommitted'))).toHaveText(
    `${working.files.length} uncommitted`,
  );
  // Each row carries a review checkbox, in the contract's state.
  await expect(section.getByRole('checkbox')).toHaveCount(working.files.length);
  await expect(section.getByRole('checkbox', { checked: true })).toHaveCount(0);
});

reviewTest('no uncommitted section for a clean worktree', async ({ relaunch }) => {
  const page = await openReview(relaunch);
  await expect(page.locator(t('review-working'))).toHaveCount(0);
  await expect(page.locator(t('ade-review-uncommitted'))).toHaveCount(0);
});

reviewTest('an uncommitted file opens a worktree diff', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    storage: AGENT_PANEL_ON,
    control: reviewControl([
      ...withWorking,
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
    ]),
    gitStream: GIT,
  });
  await page.locator(t('ade-review-window')).waitFor();
  await page
    .locator(`${t('review-working-tree')} ${t('file-tree-row')}`, {
      hasText: 'keep.txt',
    })
    .click();
  await expect
    .poll(() => calls(control, IPC.adeTaskReviewWorkingDiff).map((c) => c.args))
    .toContainEqual({ branchId: target.branchId, path: 'keep.txt' });
  await expect(page.locator(t('repo-diff-editor')).locator('.editor.modified')).toContainText(
    'unstaged edit',
  );
});

reviewTest(
  'agent activity in the worktree refreshes the uncommitted list',
  async ({ relaunch }) => {
    const { window: page, control } = await relaunch({
      storage: AGENT_PANEL_ON,
      control: reviewControl(withWorking),
      gitStream: GIT,
    });
    await page.locator(t('ade-review-window')).waitFor();
    const reads = () => calls(control, IPC.adeTaskReviewWorking).length;
    await expect.poll(reads).toBeGreaterThan(0);
    const before = reads();
    await emitAgentEvent(page, agent.session.terminalId, 'PostToolUse', `${target.worktree}/src`);
    await expect.poll(reads).toBeGreaterThan(before);
    const after = reads();
    await emitAgentEvent(page, agent.session.terminalId, 'PostToolUse', '/tmp/elsewhere');
    // The matching event is the barrier: exactly one more read proves the elsewhere one caused none.
    await emitAgentEvent(page, agent.session.terminalId, 'PostToolUse', `${target.worktree}/src`);
    await expect.poll(reads).toBeGreaterThan(after);
    expect(reads()).toBe(after + 1);
  },
);

reviewTest(
  'a board push that leaves the branch alone does not re-read the uncommitted list',
  async ({ relaunch }) => {
    const { window: page, control } = await relaunch({
      storage: AGENT_PANEL_ON,
      control: reviewControl(withWorking),
      gitStream: GIT,
    });
    await page.locator(t('ade-review-window')).waitFor();
    const reads = () => calls(control, IPC.adeTaskReviewWorking).length;
    await expect.poll(reads).toBeGreaterThan(0);
    const before = reads();
    await emitWailsEvent(page, IPC.adeTaskBoardChanged, null);
    // Absence of a read: this event has no matching one to use as a barrier.
    await page.waitForTimeout(600);
    expect(reads()).toBe(before);
  },
);

reviewTest('a file already marked reviewed still opens its diff', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    storage: AGENT_PANEL_ON,
    control: reviewControl([
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
    ]),
    gitStream: {
      ...GIT,
      extraResults: {
        ...GIT.extraResults,
        'file.read': { kind: 'found', content: 'export const a = 1;\n' },
        'review.snapshot': { kind: 'none', text: null, reviewedAtSha: null },
        'review.fileDiff': {
          path: 'src/done.ts',
          deltaSource: 'noSnapshot',
          body: { kind: 'text', hunks: [] },
          reviewedRanges: [],
          lineCount: 1,
          reviewedAtSha: null,
        },
      },
    },
  });
  await page.locator(t('ade-review-window')).waitFor();
  await page
    .locator(`${t('review-files-tree')} ${t('file-tree-row')}`, {
      hasText: 'done.ts',
    })
    .click();
  const editor = page.locator(t('repo-diff-editor'));
  await expect(editor.locator('.monaco-diff-editor .editor.original')).toBeVisible();
  await expect(editor.locator('.monaco-diff-editor .editor.modified')).toBeVisible();
});

reviewTest('the agent panel hides and returns', async ({ relaunch }) => {
  const page = await openReview(relaunch);
  const right = page.locator(t('ade-review-right'));
  const toggle = page.locator(t('ade-review-toggle-agent'));
  await expect(right).toBeVisible();
  await expect(toggle).toHaveAttribute('aria-pressed', 'true');
  await toggle.click();
  await expect(right).toHaveCount(0);
  await expect(toggle).toHaveAttribute('aria-pressed', 'false');
  await emitWailsEvent(page, IPC.toggleOperationsPanel, undefined);
  await expect(right).toBeVisible();
});

reviewTest('with no review agent the panel offers to start one', async ({ relaunch }) => {
  const page = await openReview(relaunch);
  await expect(page.locator(t('ade-review-start'))).toHaveText('Start review agent');
  await expect(page.locator(t('ade-review-compose'))).toHaveCount(0);
});

const openComment = (id: number, state: 'open' | 'addressed' = 'open') => ({
  id,
  path: 'src/billing.ts',
  range: { start: 10 * id, end: 10 * id },
  body: `Comment ${id}`,
  anchor: 'exact',
  anchorSha: 'a'.repeat(40),
  createdAt: 1_790_036_500_000,
  kind: 'change',
  state,
  reply: '',
  repliedAt: null,
  sentAt: null,
});

async function openForward(
  relaunch: Parameters<Parameters<typeof reviewTest>[2]>[0]['relaunch'],
  comments: readonly unknown[] | { readonly __sequence: readonly unknown[] },
  extra: readonly ControlSnapshot[] = [],
): Promise<{ page: Page; control: { log(): { channel: string; args?: unknown }[] } }> {
  const { window: page, control } = await relaunch({
    storage: AGENT_PANEL_ON,
    control: reviewControl([{ channel: IPC.adeTaskReviewAgent, response: agent }, ...extra]),
    gitStream: {
      ...GIT,
      extraResults: {
        ...GIT.extraResults,
        'review.comment.list': Array.isArray(comments)
          ? { at: 'a'.repeat(40), comments }
          : comments,
      },
    },
  });
  await page.locator(t('ade-review-window')).waitFor();
  await emitAgentSessions(page, [agent.session.terminalId]);
  return { page, control };
}

reviewTest('the running review agent is a plain terminal', async ({ relaunch }) => {
  const { page } = await openForward(
    relaunch,
    [],
    [{ channel: IPC.mobileLaunchOpened, response: null }],
  );
  await emitWailsEvent(page, IPC.mobileOpenLaunch, {
    launch: {
      ...adeFixture<object>('launch'),
      terminalId: agent.session.terminalId,
      sessionId: 'rv01',
    },
    taskId: 'T_bill',
    branchId: 'b_billdash',
  });
  await expect(page.locator(t('ade-tui-pane'))).toBeVisible();
  await expect(page.locator(t('ade-review-question'))).toHaveCount(0);
  await expect(page.locator(t('ade-review-compose'))).toHaveCount(0);
});

// Contract ade-review-forward, backend half: adeflow TestReviewForwardComments.
reviewTest('contract: forward review comments', async ({ relaunch }) => {
  const args = contract<{ branchId: string }>(
    'ade-review-forward',
    'args:AdeTaskService.ForwardReviewComments',
  );
  const result = contract<{ count: number }>(
    'ade-review-forward',
    'AdeTaskService.ForwardReviewComments',
  );
  const addressed = openComment(3, 'addressed');
  const forwardedList = contract<{ at: string; comments: readonly unknown[] }>(
    'ade-review-forward',
    'git:review.comment.list#forwarded',
  );
  const { page, control } = await openForward(
    relaunch,
    {
      __sequence: [
        { at: 'a'.repeat(40), comments: [openComment(1), openComment(2), addressed] },
        { at: 'a'.repeat(40), comments: forwardedList.comments },
      ],
    },
    [{ channel: IPC.adeTaskForwardReviewComments, response: result }],
  );
  const button = page.locator(t('ade-review-forward'));
  await expect(button).toHaveText('Send review comments · 2');
  await expect(button).toBeEnabled();
  await button.click();
  const forwarded = () =>
    control.log().filter((e) => e.channel === IPC.adeTaskForwardReviewComments);
  await expect.poll(forwarded).toHaveLength(1);
  expect(Object.keys(args)).toEqual(Object.keys(forwarded()[0]?.args as object));
  expect(forwarded()[0]?.args).toEqual({ branchId: target.branchId });
  await expect(button).toHaveText('Send review comments · 0');
});

// Contract ade-review-forward, backend half: adeflow TestReviewForwardComments.
reviewTest(
  'contract: copy puts the forwarded message on the clipboard and sends nothing',
  async ({ relaunch }) => {
    const result = contract<{ text: string }>(
      'ade-review-forward',
      'AdeTaskService.ReviewForwardText',
    );
    const { page, control } = await openForward(
      relaunch,
      [openComment(1), openComment(2)],
      [{ channel: IPC.adeTaskReviewForwardText, response: result }],
    );
    await page.evaluate(() => {
      const w = window as unknown as { __clipboard: string[] };
      w.__clipboard = [];
      navigator.clipboard.writeText = async (text: string) => {
        w.__clipboard.push(text);
      };
    });
    const button = page.locator(t('ade-review-copy'));
    await expect(button).toBeEnabled();
    await button.click();
    await expect(button).toHaveText('Copied');
    await expect
      .poll(() => page.evaluate(() => (window as unknown as { __clipboard: string[] }).__clipboard))
      .toEqual([result.text]);
    const calls = (channel: string) => control.log().filter((e) => e.channel === channel);
    expect(calls(IPC.adeTaskReviewForwardText)).toHaveLength(1);
    expect(calls(IPC.adeTaskForwardReviewComments)).toHaveLength(0);
  },
);

reviewTest('copy is disabled with no open comment', async ({ relaunch }) => {
  const { page } = await openForward(relaunch, [openComment(1, 'addressed')]);
  await expect(page.locator(t('ade-review-copy'))).toBeDisabled();
});

reviewTest(
  'forward is disabled while the agent waits on input or nothing is open',
  async ({ relaunch }) => {
    const { page } = await openForward(relaunch, [openComment(1)]);
    const button = page.locator(t('ade-review-forward'));
    await expect(button).toBeEnabled();
    await emitAgentEvent(page, agent.session.terminalId, 'Notification');
    await expect(button).toBeDisabled();

    const empty = await openForward(relaunch, [openComment(1, 'addressed')]);
    await expect(empty.page.locator(t('ade-review-forward'))).toBeDisabled();
    await expect(empty.page.locator(t('ade-review-forward'))).toHaveText(
      'Send review comments · 0',
    );
  },
);

reviewTest(
  'dragging a pane handle stores the dragged width; a click stores none',
  async ({ relaunch }) => {
    const page = await openReview(relaunch);
    const left = page.locator(t('ade-review-left'));
    const handle = page.locator(t('ade-panel-resize-handle')).first();
    const start = (await left.boundingBox())?.width ?? 0;
    const box = await handle.boundingBox();
    if (!box) throw new Error('no handle');
    const x = box.x + box.width / 2;
    const y = box.y + 200;
    const stored = () => page.evaluate(() => localStorage.getItem('kira.ade.review.leftWidth'));
    await page.mouse.click(x, y);
    expect(await stored()).toBeNull();
    await page.mouse.move(x, y);
    await page.mouse.down();
    await page.mouse.move(x + 60, y, { steps: 6 });
    await page.mouse.up();
    await expect.poll(stored).toBe(String(Math.round(start + 60)));
  },
);

reviewTest('the agent pane widens past the old cap', async ({ relaunch }) => {
  const { page } = await openForward(relaunch, []);
  await page.setViewportSize({ width: 2000, height: 900 });
  const right = page.locator(t('ade-review-right'));
  const centre = page.locator(t('ade-review-centre'));
  const handle = page.locator(t('ade-panel-resize-handle')).nth(1);
  const box = await handle.boundingBox();
  if (!box) throw new Error('no handle');
  const x = box.x + box.width / 2;
  const y = box.y + 200;
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.mouse.move(x - 500, y, { steps: 10 });
  await page.mouse.up();
  await expect.poll(async () => (await right.boundingBox())?.width ?? 0).toBeGreaterThan(720);
  const stored = await page.evaluate(() => localStorage.getItem('kira.ade.review.rightWidth'));
  expect(Number(stored)).toBeGreaterThan(720);
  expect((await centre.boundingBox())?.width ?? 0).toBeGreaterThanOrEqual(320);
});

reviewTest(
  'a stored width wider than the window leaves the centre its minimum',
  async ({ relaunch }) => {
    const { page } = await openForward(relaunch, []);
    await page.evaluate(() => localStorage.setItem('kira.ade.review.rightWidth', '5000'));
    await page.reload();
    await page.locator(t('ade-review-window')).waitFor();
    const centre = page.locator(t('ade-review-centre'));
    await expect
      .poll(async () => (await centre.boundingBox())?.width ?? 0)
      .toBeGreaterThanOrEqual(320);
    expect(await page.evaluate(() => localStorage.getItem('kira.ade.review.rightWidth'))).toBe(
      '5000',
    );
  },
);

reviewTest('arrow keys on a pane handle resize and store the left width', async ({ relaunch }) => {
  const page = await openReview(relaunch);
  const left = page.locator(t('ade-review-left'));
  const handle = page.locator(t('ade-panel-resize-handle')).first();
  const start = (await left.boundingBox())?.width ?? 0;
  await handle.focus();
  await page.keyboard.press('ArrowRight');
  await expect.poll(async () => (await left.boundingBox())?.width ?? 0).toBeGreaterThan(start);
  const stored = await page.evaluate(() => localStorage.getItem('kira.ade.review.leftWidth'));
  expect(Number(stored)).toBeGreaterThan(start);
});

const ACTIONS = 'ade-review-actions';
const actionsGit = (own = false, state = '') => ({
  'changeRequest.reviewState': contract(
    ACTIONS,
    `git:changeRequest.reviewState${own ? '#own' : state}`,
  ),
  'changeRequest.submitReview': contract(
    ACTIONS,
    `git:changeRequest.submitReview${own ? '#own' : ''}`,
  ),
  'changeRequest.comment': contract(ACTIONS, 'git:changeRequest.comment'),
});
const actionsHead = (contract(ACTIONS, 'git:changeRequest.reviewState') as { headSha: string })
  .headSha;
const staleHead = (contract(ACTIONS, 'git:changeRequest.reviewState#stale') as { headSha: string })
  .headSha;

reviewTest('contract: approving a change request submits the review', async ({ relaunch }) => {
  const page = await openReview(relaunch, [], actionsGit());
  await page.locator(t('ade-review-actions-trigger')).click();
  await page.locator(t('ade-review-action-approve')).click();
  await expect(page.getByRole('dialog')).toContainText('Approve MR #7');
  await page.locator(t('ade-review-submit-body')).fill('LGTM');
  await page.locator(t('ade-review-submit')).click();
  await expect(page.locator(t('ade-review-posted'))).toContainText('Review submitted');
  expect(await gitStreamParams(page, 'changeRequest.submitReview')).toEqual([
    { repoId: target.gitRepoId, number: 7, event: 'approve', body: 'LGTM', headSha: actionsHead },
  ]);
});

// Contract ade-review-actions, backend half: githostflow TestChangeRequestReviewActions.
reviewTest(
  'contract: the title bar shows the local branch at the host head',
  async ({ relaunch }) => {
    const page = await openReview(relaunch, [], actionsGit());
    const badge = page.locator(t('ade-review-head'));
    await expect(badge).toHaveAttribute('data-head', 'current');
    await expect(badge).toHaveText('At MR head');
    await badge.focus();
    await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(
      actionsHead.slice(0, 7),
    );
  },
);

reviewTest(
  'contract: the title bar flags a stale head and the dialog agrees',
  async ({ relaunch }) => {
    const page = await openReview(relaunch, [], actionsGit(false, '#stale'));
    const badge = page.locator(t('ade-review-head'));
    await expect(badge).toHaveAttribute('data-head', 'stale');
    await expect(badge).toHaveText('Stale');
    await badge.focus();
    const tip = page.locator('[data-slot="tooltip-content"]').first();
    await expect(tip).toContainText('is not at the MR head');
    await expect(tip).toContainText(actionsHead.slice(0, 7));
    await expect(tip).toContainText(staleHead.slice(0, 7));
    await page.locator(t('ade-review-actions-trigger')).click();
    await page.locator(t('ade-review-action-approve')).click();
    await expect(page.locator(t('ade-review-submit-head'))).toBeVisible();
  },
);

reviewTest('a ref change of the repo rechecks the head', async ({ relaunch }) => {
  const page = await openReview(relaunch, [], actionsGit());
  const checks = async () =>
    (await gitStreamRequests(page)).filter((r) => r === 'changeRequest.reviewState').length;
  await expect(page.locator(t('ade-review-head'))).toBeVisible();
  const before = await checks();
  await gitStreamEmit(page, 'repo.changed', { repoId: 'other-repo', kind: 'refsChanged' });
  // The matching event is the barrier: exactly one more check proves the other repo's caused none.
  await gitStreamEmit(page, 'repo.changed', { repoId: target.gitRepoId, kind: 'refsChanged' });
  await expect.poll(checks).toBeGreaterThan(before);
  expect(await checks()).toBe(before + 1);
});

reviewTest('no head indicator without a change request', async ({ relaunch }) => {
  const page = await openReview(relaunch, [], { 'changeRequest.reviewState': { kind: 'none' } });
  await expect(page.locator(t('ade-review-actions-trigger'))).toBeVisible();
  await expect(page.locator(t('ade-review-head'))).toHaveCount(0);
});

reviewTest('contract: a change request comment posts to the conversation', async ({ relaunch }) => {
  const page = await openReview(relaunch, [], actionsGit());
  await page.locator(t('ade-review-cr-comment')).click();
  await page.locator(t('ade-review-submit-body')).fill('@alice thanks');
  await page.locator(t('ade-review-submit')).click();
  await expect(page.locator(t('ade-review-posted'))).toContainText('Comment posted');
  expect(await gitStreamParams(page, 'changeRequest.comment')).toEqual([
    { repoId: target.gitRepoId, number: 7, body: '@alice thanks' },
  ]);
});

const LINE_ARGS = contract<Record<string, unknown>>(ACTIONS, 'args:changeRequest.lineComment');

async function openHostCompose(
  relaunch: Parameters<Parameters<typeof reviewTest>[2]>[0]['relaunch'],
  extraResults: Record<string, unknown>,
  hostReady = true,
) {
  const { page, modified } = await openHunkDiff(relaunch, { comments: [], extraResults });
  await expect(renderedLine(modified, 'CHANGED 100')).toHaveCount(1, { timeout: 15_000 });
  if (hostReady) await expect(page.locator(t('ade-review-head'))).toBeVisible();
  else {
    await expect
      .poll(async () => (await gitStreamRequests(page)).includes('changeRequest.reviewState'))
      .toBe(true);
  }
  return { page, thread: await openComposeFromGutter(page, modified) };
}

// Contract ade-review-actions, backend half: githostflow TestChangeRequestReviewActions.
reviewTest('contract: a PR comment posts on its line', async ({ relaunch }) => {
  const { page, thread } = await openHostCompose(relaunch, {
    ...actionsGit(),
    'changeRequest.lineComment': contract(ACTIONS, 'git:changeRequest.lineComment'),
  });
  await thread.getByTestId('review-thread-kind-host').click();
  await expect(thread.getByTestId('review-thread-host-hint')).toHaveText(
    'Posts to MR #7. Kira keeps no copy.',
  );
  await thread.getByTestId('review-thread-input').fill('tighten this');
  await thread.getByTestId('review-thread-send').click();
  await expect(thread.getByTestId('review-thread-posted')).toHaveText('Posted to MR #7');
  const posts = await gitStreamParams(page, 'changeRequest.lineComment');
  expect(posts).toHaveLength(1);
  expect(Object.keys(LINE_ARGS).sort()).toEqual(Object.keys(posts[0] as object).sort());
  expect(posts[0]).toEqual({
    repoId: target.gitRepoId,
    number: 7,
    path: 'src/pending.ts',
    range: { start: 100, end: 100 },
    headSha: actionsHead,
    body: 'tighten this',
  });
  expect(await gitStreamParams(page, 'review.comment.add')).toHaveLength(0);
});

reviewTest('contract: a moved PR head refuses the post', async ({ relaunch }) => {
  const { thread } = await openHostCompose(relaunch, {
    ...actionsGit(),
    'changeRequest.lineComment': contract(ACTIONS, 'git:changeRequest.lineComment#headMoved'),
  });
  await thread.getByTestId('review-thread-kind-host').click();
  await thread.getByTestId('review-thread-input').fill('tighten this');
  await thread.getByTestId('review-thread-send').click();
  await expect(thread.getByTestId('review-thread-error')).toContainText(
    'The MR head is not the commit shown here',
  );
  await expect(thread.getByTestId('review-thread-posted')).toHaveCount(0);
});

reviewTest('a stale head blocks the PR comment before any post', async ({ relaunch }) => {
  const { page, thread } = await openHostCompose(relaunch, actionsGit(false, '#stale'));
  await thread.getByTestId('review-thread-kind-host').click();
  await thread.getByTestId('review-thread-input').fill('tighten this');
  await expect(thread.getByTestId('review-thread-host-hint')).toContainText(
    'The MR head is not the commit shown here',
  );
  await expect(thread.getByTestId('review-thread-send')).toBeDisabled();
  expect(await gitStreamParams(page, 'changeRequest.lineComment')).toHaveLength(0);
});

reviewTest('no change request hides the PR comment tab', async ({ relaunch }) => {
  const { thread } = await openHostCompose(
    relaunch,
    { 'changeRequest.reviewState': { kind: 'none' } },
    false,
  );
  await expect(thread.getByTestId('review-thread-kind-question')).toBeVisible();
  await expect(thread.getByTestId('review-thread-kind-host')).toHaveCount(0);
});

reviewTest(
  'contract: your own change request disables approve and request changes',
  async ({ relaunch }) => {
    const page = await openReview(relaunch, [], actionsGit(true));
    await page.locator(t('ade-review-actions-trigger')).click();
    for (const event of ['approve', 'requestChanges']) {
      await expect(page.locator(t(`ade-review-action-${event}`))).toHaveAttribute(
        'data-disabled',
        '',
      );
      await expect(page.locator(t(`ade-review-action-${event}-reason`))).toContainText(
        'your own MR',
      );
    }
    await expect(page.locator(t('ade-review-action-comment'))).not.toHaveAttribute(
      'data-disabled',
      '',
    );
  },
);

reviewTest('contract: a refused review shows the host reason', async ({ relaunch }) => {
  const page = await openReview(relaunch, [], actionsGit(true));
  await page.locator(t('ade-review-actions-trigger')).click();
  await page.locator(t('ade-review-action-comment')).click();
  await page.locator(t('ade-review-submit-body')).fill('please look');
  await page.locator(t('ade-review-submit')).click();
  await expect(page.locator(t('ade-review-submit-error'))).toContainText(
    'you cannot approve your own merge request',
  );
  await expect(page.getByRole('dialog')).toBeVisible();
});

const FOREIGN = 'ade-review-foreign';
const foreignTarget = (key: string) => {
  const { branch, changeRequest } = contract(
    FOREIGN,
    `AdeTaskService.ReviewWindowTarget#${key}`,
  ) as { branch: string; changeRequest: number };
  return { ...target, branch, changeRequest };
};
const foreignHead = (contract(FOREIGN, 'git:changeRequest.reviewState#fork') as { headSha: string })
  .headSha;

reviewTest(
  "contract: another person's change request can be approved and commented on",
  async ({ relaunch }) => {
    const page = await openReview(
      relaunch,
      [{ channel: IPC.adeTaskReviewWindowTarget, response: foreignTarget('fork') }],
      {
        'changeRequest.reviewState': contract(FOREIGN, 'git:changeRequest.reviewState#fork'),
        'changeRequest.submitReview': contract(FOREIGN, 'git:changeRequest.submitReview#fork'),
        'changeRequest.comment': contract(FOREIGN, 'git:changeRequest.comment#fork'),
      },
    );
    await page.locator(t('ade-review-actions-trigger')).click();
    for (const event of ['approve', 'requestChanges', 'comment']) {
      await expect(page.locator(t(`ade-review-action-${event}`))).not.toHaveAttribute(
        'data-disabled',
        '',
      );
      await expect(page.locator(t(`ade-review-action-${event}-reason`))).toHaveCount(0);
    }
    await page.locator(t('ade-review-action-approve')).click();
    await page.locator(t('ade-review-submit')).click();
    await expect(page.locator(t('ade-review-posted'))).toContainText('Review submitted');
    expect((await gitStreamParams(page, 'changeRequest.reviewState'))[0]).toEqual({
      repoId: target.gitRepoId,
      branch: 'forker/main',
      number: 12,
    });
    expect(await gitStreamParams(page, 'changeRequest.submitReview')).toEqual([
      { repoId: target.gitRepoId, number: 12, event: 'approve', body: '', headSha: foreignHead },
    ]);
    await page.locator(t('ade-review-cr-comment')).click();
    await page.locator(t('ade-review-submit-body')).fill('Looks good, one nit');
    await page.locator(t('ade-review-submit')).click();
    await expect(page.locator(t('ade-review-posted'))).toContainText('Comment posted');
    expect(await gitStreamParams(page, 'changeRequest.comment')).toEqual([
      { repoId: target.gitRepoId, number: 12, body: 'Looks good, one nit' },
    ]);
  },
);

reviewTest(
  'contract: a remote-only branch finds its change request by head name',
  async ({ relaunch }) => {
    const page = await openReview(
      relaunch,
      [{ channel: IPC.adeTaskReviewWindowTarget, response: foreignTarget('remote') }],
      { 'changeRequest.reviewState': contract(FOREIGN, 'git:changeRequest.reviewState#remote') },
    );
    await page.locator(t('ade-review-actions-trigger')).click();
    await expect(page.locator(t('ade-review-action-approve'))).not.toHaveAttribute(
      'data-disabled',
      '',
    );
    const [params] = await gitStreamParams(page, 'changeRequest.reviewState');
    expect(params).toEqual({ repoId: target.gitRepoId, branch: 'origin/feat/x' });
  },
);

reviewTest('a branch with no change request shows why Review is off', async ({ relaunch }) => {
  const page = await openReview(relaunch, [], { 'changeRequest.reviewState': { kind: 'none' } });
  const trigger = page.locator(t('ade-review-actions-trigger'));
  await expect(trigger).toBeDisabled();
  await trigger.locator('..').hover();
  await expect(page.locator('[data-slot="tooltip-content"]')).toContainText(
    'No CR found for this branch.',
  );
});

reviewTest(
  'the agent panel button sits at the far right of the title bar',
  async ({ relaunch }) => {
    const page = await openReview(relaunch, [], actionsGit());
    await page.locator(t('ade-review-actions-trigger')).waitFor();
    const bar = await boxOf(page.locator('.wails-drag'));
    const comment = await boxOf(page.locator(t('ade-review-cr-comment')));
    const toggle = await boxOf(page.locator(t('ade-review-toggle-agent')));
    expect(toggle.x).toBeGreaterThanOrEqual(comment.x + comment.width);
    expect(bar.x + bar.width - (toggle.x + toggle.width)).toBeLessThanOrEqual(8);
  },
);

const openCommentDialog = async (
  relaunch: Parameters<Parameters<typeof reviewTest>[2]>[0]['relaunch'],
  viewport?: { width: number; height: number },
): Promise<Page> => {
  const page = await openReview(relaunch, [], actionsGit());
  if (viewport) await page.setViewportSize(viewport);
  await page.locator(t('ade-review-cr-comment')).click();
  await expect(page.getByRole('dialog')).toBeVisible();
  return page;
};

reviewTest('the review dialog fits a small window', async ({ relaunch }) => {
  const viewport = { width: 560, height: 420 };
  const page = await openCommentDialog(relaunch, viewport);
  const d = await boxOf(page.getByRole('dialog'));
  expect(d.x).toBeGreaterThanOrEqual(0);
  expect(d.y).toBeGreaterThanOrEqual(0);
  expect(d.x + d.width).toBeLessThanOrEqual(viewport.width);
  expect(d.y + d.height).toBeLessThanOrEqual(viewport.height);
  await expect(page.locator(t('ade-review-submit'))).toBeInViewport();
  await expect(page.locator(t('ade-review-submit-body'))).toBeInViewport();
  expect((await boxOf(page.locator(t('ade-review-submit-body')))).height).toBeGreaterThanOrEqual(
    96,
  );
});

reviewTest('the review dialog resizes from its edges', async ({ relaunch }) => {
  const page = await openCommentDialog(relaunch);
  const dialog = page.getByRole('dialog');
  const body = page.locator(t('ade-review-submit-body'));
  const drag = async (id: string, dx: number, dy: number) => {
    const h = await boxOf(page.locator(t(id)));
    const x = h.x + h.width / 2;
    const y = h.y + h.height / 2;
    await page.mouse.move(x, y);
    await page.mouse.down();
    await page.mouse.move(x + dx, y + dy, { steps: 8 });
    await page.mouse.up();
  };
  const d0 = await boxOf(dialog);
  const t0 = await boxOf(body);
  const handleX = page.locator(t('dialog-resize-x'));
  await expect(handleX).toHaveAttribute('aria-valuenow', String(Math.round(d0.width)));
  await expect(handleX).toHaveAttribute('aria-valuemin', '320');

  // An aborted drag (window blur) drops the live size.
  const hb = await boxOf(handleX);
  await page.mouse.move(hb.x + hb.width / 2, hb.y + hb.height / 2);
  await page.mouse.down();
  await page.mouse.move(hb.x + hb.width / 2 + 80, hb.y + hb.height / 2, { steps: 6 });
  await expect.poll(async () => (await boxOf(dialog)).width).toBeGreaterThan(d0.width + 100);
  await page.evaluate(() => window.dispatchEvent(new Event('blur')));
  await expect.poll(async () => (await boxOf(dialog)).width).toBeCloseTo(d0.width, -1);
  await page.mouse.up();

  await drag('dialog-resize-x', 100, 0);
  await expect.poll(async () => (await boxOf(dialog)).width).toBeGreaterThan(d0.width + 196);
  const d1 = await boxOf(dialog);
  expect(Math.abs(d1.x + d1.width - (d0.x + d0.width) - 100)).toBeLessThanOrEqual(2);
  expect(Math.abs(d1.width - d0.width - 200)).toBeLessThanOrEqual(4);

  await drag('dialog-resize-y', 0, 80);
  await expect.poll(async () => (await boxOf(body)).height).toBeGreaterThanOrEqual(t0.height + 150);

  const viewportWidth = page.viewportSize()?.width ?? 0;
  await drag('dialog-resize-x', 2000, 0);
  await expect
    .poll(async () => (await boxOf(dialog)).width)
    .toBeGreaterThan(viewportWidth * 0.9 - 2);
  const wide = (await boxOf(dialog)).width;
  expect(wide).toBeLessThanOrEqual(viewportWidth * 0.9 + 1);

  await page.locator(t('dialog-resize-x')).focus();
  await page.keyboard.press('ArrowLeft');
  await expect.poll(async () => (await boxOf(dialog)).width).toBeLessThan(wide);

  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
  await page.locator(t('ade-review-cr-comment')).click();
  await expect(dialog).toBeVisible();
  await expect.poll(async () => (await boxOf(dialog)).width).toBeCloseTo(d0.width, -1);
});

// P413: the review thread over the hunk diff. Backend halves: reviewflow TestCommentEdit (edit).

const longComment = (extra: object = {}) => ({
  ...HUNK_COMMENT,
  range: { start: 100, end: 100 },
  body: Array.from({ length: 80 }, (_, i) => `body line ${i + 1}`).join('\n'),
  ...extra,
});

const ADDED = {
  comment: { ...HUNK_COMMENT, id: 2, range: { start: 100, end: 100 }, body: 'x' },
};

/** The gutter route: hover the line, click the add mark that replaces its number. */
async function openComposeFromGutter(page: Page, modified: Locator) {
  await renderedLine(modified, 'CHANGED 100').hover();
  await modified.locator('.kira-review-ln-add').click();
  const thread = page.locator(t('review-thread'));
  await expect(thread).toBeVisible();
  return thread;
}

reviewTest('a long thread stays inside the editor and scrolls by wheel', async ({ relaunch }) => {
  const { page, modified } = await openHunkDiff(relaunch, {
    comments: [longComment()],
    line101: `LONGLINE ${'x'.repeat(400)}`,
  });
  await expect(renderedLine(modified, 'CHANGED 100')).toHaveCount(1, { timeout: 15_000 });
  await modified.locator('.kira-review-ln-comment').click();
  const thread = page.locator(t('review-thread'));
  await expect(thread).toBeVisible();

  const editor = await boxOf(modified);
  const box = await boxOf(thread);
  expect(box.x + box.width).toBeLessThanOrEqual(editor.x + editor.width + 1);
  expect(box.y + box.height).toBeLessThanOrEqual(editor.y + editor.height + 1);
  expect(box.height).toBeLessThanOrEqual(editor.height * 0.8 + 1);
  await expect(thread.getByTestId('review-thread-state')).toBeVisible();

  const probe = renderedLine(modified, 'line 99');
  const probeBefore = (await boxOf(probe)).y;
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await expect
    .poll(async () => {
      await page.mouse.wheel(0, 300);
      return thread.evaluate((el) => el.scrollTop);
    })
    .toBeGreaterThan(0);
  expect((await boxOf(probe)).y).toBe(probeBefore);
});

reviewTest(
  'the compose form grows to a cap, then scrolls, and never overlaps',
  async ({ relaunch }) => {
    const { page, modified } = await openHunkDiff(relaunch, { comments: [] });
    await expect(renderedLine(modified, 'CHANGED 100')).toHaveCount(1, { timeout: 15_000 });
    const thread = await openComposeFromGutter(page, modified);
    const input = thread.getByTestId('review-thread-input');
    const below = renderedLine(modified, 'line 101');
    const editor = await boxOf(modified);

    const small = await boxOf(thread);
    await input.fill(Array.from({ length: 40 }, (_, i) => `row ${i + 1}`).join('\n'));
    await expect.poll(async () => (await boxOf(thread)).height).toBeGreaterThan(small.height);
    const grown = await boxOf(thread);
    expect(grown.height).toBeLessThanOrEqual(editor.height * 0.8 + 1);
    expect(grown.y + grown.height).toBeLessThanOrEqual((await boxOf(below)).y + 1);
    await expect.poll(() => thread.evaluate((el) => el.scrollHeight > el.clientHeight)).toBe(true);
  },
);

reviewTest('compose opens on Question', async ({ relaunch }) => {
  const { page, modified } = await openHunkDiff(relaunch, { comments: [] });
  await expect(renderedLine(modified, 'CHANGED 100')).toHaveCount(1, { timeout: 15_000 });
  const thread = await openComposeFromGutter(page, modified);
  const kinds = thread
    .getByTestId('review-thread-kind')
    .locator('[data-testid^="review-thread-kind-"]');
  await expect(kinds.first()).toHaveAttribute('data-testid', 'review-thread-kind-question');
  await expect(kinds.first()).toHaveAttribute('data-state', 'on');
});

reviewTest('keystrokes in the composer stay in the composer', async ({ relaunch }) => {
  const { page, modified } = await openHunkDiff(relaunch, {
    comments: [],
    extraResults: { 'review.comment.add': ADDED },
  });
  await expect(renderedLine(modified, 'CHANGED 100')).toHaveCount(1, { timeout: 15_000 });
  const thread = await openComposeFromGutter(page, modified);
  const input = thread.getByTestId('review-thread-input');
  await expect(input).toBeFocused();

  await input.click();
  const kb = page.keyboard;
  await kb.type('abc');
  await kb.press('ArrowLeft');
  await kb.press('Backspace');
  await kb.press('Enter');
  await kb.type('d');
  await kb.press('Home');
  await kb.type('x');
  await expect(input).toHaveValue('a\nxdc');
  await kb.press('ControlOrMeta+A');
  await kb.type('z');
  await expect(input).toHaveValue('z');

  await kb.press('Escape');
  await expect(thread).toHaveCount(0);

  await openComposeFromGutter(page, modified);
  await input.fill('typed');
  await kb.press('Meta+Enter');
  await expect
    .poll(async () => (await gitStreamRequests(page)).includes('review.comment.add'))
    .toBe(true);
  expect((await gitStreamParams(page, 'review.comment.add'))[0]).toMatchObject({
    path: 'src/pending.ts',
    range: { start: 100, end: 100 },
    body: 'typed',
    kind: 'question',
  });
});

reviewTest(
  'the context menu adds a review comment from the keyboard route',
  async ({ relaunch }) => {
    const { page, modified } = await openHunkDiff(relaunch, { comments: [] });
    await expect(renderedLine(modified, 'CHANGED 100')).toHaveCount(1, { timeout: 15_000 });
    // The menu item exists only once the review layer attached; wait for that, not for a retry.
    await expect(page.locator(t('repo-diff-editor'))).toHaveAttribute('data-review-ready', 'true');
    await renderedLine(modified, 'CHANGED 100').click({ button: 'right' });
    await page.getByRole('menuitem', { name: 'Add review comment' }).click();
    const thread = page.locator(t('review-thread'));
    await expect(thread).toBeVisible();
    await thread.getByTestId('review-thread-input').fill('from the menu');
    await thread.getByTestId('review-thread-send').click();
    await expect
      .poll(async () => (await gitStreamParams(page, 'review.comment.add'))[0])
      .toMatchObject({ range: { start: 100, end: 100 }, body: 'from the menu' });
  },
);
