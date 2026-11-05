import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { gitStreamParams } from './support/gitStreamMock';
import {
  manyRowShas,
  manyRows,
  oneChunk,
  openPortGraph,
  openReviewListing,
  PICKER_RESULTS,
  REVIEW_FILE_FULL,
  REVIEW_FILE_NONE,
  REVIEW_FILE_PARTIAL,
  REVIEW_RESULTS,
  rootRow,
  SHA_A,
  singleRowChunks,
} from './support/gitUiPortFixtures';

// The review sidebar inside Space: row gestures, host-answered actions, the Files pane checkbox and
// the virtualized commit list. Ported from the VS Code webview suite.

const reviewRow = (p: Page, sha = SHA_A) => p.locator(`[data-testid="review-row-${sha}"]`);
const tabOfKind = (p: Page, kind: string) =>
  p.locator(`[data-testid="tab-strip-wrapper"] [data-testid="tab"][data-tab-kind="${kind}"]`);
const base = (name: string) => name.split('/').pop();

test('clicking a file inside an expanded commit does not collapse the row', async ({
  relaunch,
}) => {
  const page = await openReviewListing(relaunch);
  const row = reviewRow(page);
  await row.locator('[data-testid="review-row-header"]').click();
  await expect(row).toHaveAttribute('aria-expanded', 'true');

  await row.locator('[data-testid="file-tree-row"]', { hasText: 'example.ts' }).click();

  await expect(row).toHaveAttribute('aria-expanded', 'true');
  await expect(row.locator('[data-testid="file-tree"]')).toBeVisible();
});

test('Open all changes on a collapsed row opens one multi-diff tab and leaves the row collapsed', async ({
  relaunch,
}) => {
  const page = await openReviewListing(relaunch);
  const row = reviewRow(page);
  await expect(row).toHaveAttribute('aria-expanded', 'false');
  await row.hover();
  await row.locator('button[aria-label="Open all changes"]').click();

  await expect(tabOfKind(page, 'repo-multi-diff')).toHaveCount(1);
  await expect(row).toHaveAttribute('aria-expanded', 'false');
});

test('Open in graph activates the graph tab on that commit', async ({ relaunch }) => {
  const page = await openReviewListing(relaunch);
  const row = reviewRow(page);
  await row.hover();
  await row.locator('button[aria-label="Open in graph"]').click();

  await expect(tabOfKind(page, 'repo-graph')).toHaveAttribute('data-active', 'true');
  await expect(page.locator('[data-testid="commit-grid"] .slick-row[data-row="0"]')).toBeVisible();
});

test('the Files pane reviewed control is a checkbox with three states', async ({ relaunch }) => {
  const page = await openReviewListing(relaunch);
  await page.locator('[data-testid="review-toolbar"] [data-testid="review-pane-files"]').click();

  const box = (path: string) =>
    page
      .locator('[data-testid="review-files-tree"] [data-testid="file-tree-row"]', {
        hasText: base(path),
      })
      .locator('[role="checkbox"]');
  await expect(box(REVIEW_FILE_NONE)).toHaveAttribute('aria-checked', 'false');
  await expect(box(REVIEW_FILE_PARTIAL)).toHaveAttribute('aria-checked', 'mixed');
  await expect(box(REVIEW_FILE_FULL)).toHaveAttribute('aria-checked', 'true');
});

test('Space on the focused reviewed checkbox marks the file', async ({ relaunch }) => {
  const page = await openReviewListing(relaunch);
  await page.locator('[data-testid="review-toolbar"] [data-testid="review-pane-files"]').click();
  const box = page
    .locator('[data-testid="review-files-tree"] [data-testid="file-tree-row"]', {
      hasText: base(REVIEW_FILE_NONE),
    })
    .locator('[role="checkbox"]');
  await box.focus();
  await page.keyboard.press('Space');

  await expect
    .poll(async () =>
      (await gitStreamParams(page, 'review.mark')).map((p) => (p as { path: string }).path),
    )
    .toEqual([REVIEW_FILE_NONE]);
});

test('the back button returns to branch selection', async ({ relaunch }) => {
  const page = await openReviewListing(relaunch);
  await expect(page.locator('[data-testid="review-no-branch"]')).toHaveCount(0);
  await page.locator('[data-testid="review-back-button"]').click();
  await expect(page.locator('[data-testid="review-no-branch"]')).toBeVisible();
});

test('600 commits mount only the viewport rows; End scrolls to and focuses the last', async ({
  relaunch,
}) => {
  const total = 600;
  const page = await openReviewListing(relaunch, { chunks: oneChunk(manyRows(total)) });
  const rows = page.locator(
    '[data-testid^="review-row-"]:not([data-testid$="-header"]):not([data-testid$="-actions"])',
  );
  await expect(reviewRow(page, manyRowShas(0))).toBeVisible();
  expect(await rows.count()).toBeLessThan(100);

  await reviewRow(page, manyRowShas(0)).focus();
  await page.keyboard.press('End');
  const last = reviewRow(page, manyRowShas(total - 1));
  await expect(last).toBeVisible();
  await expect(last).toBeFocused();
  expect(await rows.count()).toBeLessThan(100);
});

test('the focused commit row stays mounted and keyboard navigation works after a wheel scroll', async ({
  relaunch,
}) => {
  const total = 600;
  const page = await openReviewListing(relaunch, { chunks: oneChunk(manyRows(total)) });
  const first = reviewRow(page, manyRowShas(0));
  await expect(first).toBeVisible();
  await expect(first).toHaveAttribute('aria-setsize', String(total));
  await expect(first).toHaveAttribute('aria-posinset', '1');

  await first.focus();
  // Under load the list can lay out after the first scroll and clamp it; scroll again until the row renders.
  await expect(async () => {
    await page
      .locator('[role="tree"][aria-label="Commits"]')
      .first()
      .evaluate((el) => {
        el.scrollTop = 20000;
      });
    await expect(reviewRow(page, manyRowShas(455))).toBeAttached({ timeout: 500 });
  }).toPass();
  await expect(first).toBeFocused();

  await page.keyboard.press('ArrowDown');
  await expect(reviewRow(page, manyRowShas(1))).toBeFocused();
});

test('Review branch changes from the picker opens the Review tab on that branch', async ({
  relaunch,
}) => {
  const page = await openPortGraph(relaunch, {
    chunks: singleRowChunks([rootRow(SHA_A, 'A commit')]),
    results: { ...PICKER_RESULTS, ...REVIEW_RESULTS, 'refs.list': PICKER_RESULTS['refs.list'] },
  });
  await expect(page.locator('[data-testid="commit-grid"] .slick-row[data-row="0"]')).toBeVisible();
  await page.locator('[data-testid="branch-trigger"]').click();
  await page
    .locator('[data-row-id="branch:refs/heads/feature-auth"]')
    .getByRole('button', { name: 'More actions' })
    .click();
  await page.getByText('Review branch changes').click();

  await expect(page.locator('[data-testid="review-branch-name"]')).toHaveText('feature-auth');
});

// Contract git-review. Backend half: reviewflow TestReviewSessionRoundTrip. The comments the
// server returns after a relaunch show in the Review pane without opening the graph tab first.
test('contract: stored review comments come back in the Review pane', async ({ relaunch }) => {
  const list = contract<{ comments: { body: string }[] }>(
    'git-review',
    'git:review.comment.list#after-restart',
  );
  const page = await openReviewListing(relaunch, { results: { 'review.comment.list': list } });
  const host = page.locator('[data-testid="repo-review-host"]');
  await host.locator('[data-testid="review-pane-comments"]').click();
  await expect(host.getByText(list.comments[0].body)).toBeVisible();
});

test('the Review pane shows each comment kind, state and the agent reply', async ({ relaunch }) => {
  const base = {
    path: 'src/a.ts',
    anchor: 'exact',
    anchorSha: SHA_A,
    createdAt: 1,
    sentAt: 1,
  };
  const comments = [
    {
      ...base,
      id: '1',
      range: { start: 3, end: 3 },
      body: 'Rename it.',
      kind: 'change',
      state: 'addressed',
      reply: 'Renamed.',
      repliedAt: 2,
    },
    {
      ...base,
      id: '2',
      range: { start: 9, end: 9 },
      body: 'Why cents?',
      kind: 'question',
      state: 'open',
      reply: '',
      repliedAt: null,
    },
  ];
  const page = await openReviewListing(relaunch, {
    results: { 'review.comment.list': { at: SHA_A, comments } },
  });
  const host = page.locator('[data-testid="repo-review-host"]');
  await host.locator('[data-testid="review-pane-comments"]').click();
  const chips = host.locator('[data-testid="review-comment-chips"]');
  await expect(chips.nth(0)).toHaveText('Change · Addressed');
  await expect(chips.nth(1)).toHaveText('Question · Open');
  await expect(host.getByText('Renamed.')).toBeVisible();
});
