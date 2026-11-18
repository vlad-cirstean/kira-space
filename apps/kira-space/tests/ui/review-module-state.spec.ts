import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import {
  manyReviewFiles,
  manyRowShas,
  manyRows,
  oneChunk,
  openReviewListing,
  REVIEW_BRANCH,
  REVIEW_RESULTS,
  refsAtTip,
  SHA_A,
  SHA_B,
} from './support/gitUiPortFixtures';
import { roundTrip } from './support/moduleRoundTrip';
import { scrollTo, scrollTop } from './support/scroll';

// P660 Part 2: the Git Review segment keeps filter, selection, expanded rows and scroll across a
// module round trip. Renderer-only state, so there is no flow-test half.

const t = (id: string) => `[data-testid="${id}"]`;
const host = (p: Page) => p.locator(t('repo-review-host'));
const commitRows = (p: Page) => host(p).locator(`${t('review-commit-rows')}`);
const filesTree = (p: Page) => host(p).locator(`${t('review-files-tree')} [role="tree"]`);
const firstRow = (p: Page) => host(p).locator(`[data-testid="review-row-${manyRowShas(0)}"]`);
const fileRow = (p: Page) => host(p).locator(t('file-tree-row'), { hasText: 'file-02.ts' });
const toolbar = (p: Page) => p.locator(t('review-toolbar'));

async function open(relaunch: Parameters<typeof openReviewListing>[0]): Promise<Page> {
  const page = await openReviewListing(relaunch, {
    chunks: oneChunk(manyRows(60)),
    results: {
      'refs.list': {
        ...REVIEW_RESULTS['refs.list'],
        branches: refsAtTip(manyRowShas(0)).branches.concat(REVIEW_RESULTS['refs.list'].branches),
      },
      'review.files': {
        branchTip: SHA_A,
        mergeBase: SHA_B,
        files: manyReviewFiles(60),
      },
    },
  });
  await page.setViewportSize({ width: 1200, height: 520 });
  return page;
}

test('review place, filter and scroll survive a module round trip', async ({ relaunch }) => {
  const page = await open(relaunch);
  await firstRow(page).locator(t('review-row-header')).click();
  await expect(firstRow(page)).toHaveAttribute('aria-expanded', 'true');
  const listTop = await scrollTo(commitRows(page), 300);

  await toolbar(page).locator(t('review-pane-files')).click();
  await fileRow(page).click();
  await expect(fileRow(page)).toHaveAttribute('aria-selected', 'true');
  await page.locator(t('review-filter-toggle')).click();
  await page.locator(t('review-file-filter')).fill('src/');
  const treeTop = await scrollTo(filesTree(page), 300);

  await roundTrip(page, 'automations', 'git');

  await expect(page.locator(t('review-branch-name'))).toContainText(REVIEW_BRANCH);
  await expect(filesTree(page)).toBeVisible();
  await expect(page.locator(t('review-file-filter'))).toHaveValue('src/');
  await expect(fileRow(page)).toHaveAttribute('aria-selected', 'true');
  await expect.poll(() => scrollTop(filesTree(page))).toBe(treeTop);

  await toolbar(page).locator(t('review-pane-commits')).click();
  await expect.poll(() => scrollTop(commitRows(page))).toBe(listTop);
  await commitRows(page).evaluate((e) => {
    e.scrollTop = 0;
  });
  await expect(firstRow(page)).toHaveAttribute('aria-expanded', 'true');
});

test("back to branch selection forgets the branch's place", async ({ relaunch }) => {
  const page = await open(relaunch);
  await firstRow(page).locator(t('review-row-header')).click();
  await expect(firstRow(page)).toHaveAttribute('aria-expanded', 'true');
  await toolbar(page).locator(t('review-pane-files')).click();
  await fileRow(page).click();
  await expect(fileRow(page)).toHaveAttribute('aria-selected', 'true');

  await page.locator(t('review-back-button')).click();
  const option = host(page).locator(t(`review-branch-option-${REVIEW_BRANCH}`));
  await expect(option).toBeVisible();
  await roundTrip(page, 'automations', 'git');
  await expect(option).toBeVisible();

  await option.click();
  await expect(firstRow(page)).toHaveAttribute('aria-expanded', 'false');
  await expect.poll(() => scrollTop(commitRows(page))).toBe(0);
  await toolbar(page).locator(t('review-pane-files')).click();
  await expect(host(page).locator('[aria-selected="true"]')).toHaveCount(0);
});

test('closing the workspace drops kept review state', async ({ relaunch }) => {
  const page = await open(relaunch);
  await toolbar(page).locator(t('review-pane-files')).click();
  await page.locator(t('review-filter-toggle')).click();
  await page.locator(t('review-file-filter')).fill('src/');
  await fileRow(page).click();
  await expect(fileRow(page)).toHaveAttribute('aria-selected', 'true');

  await page.locator(t('git-panel-tab-repos')).click();
  const row = page.locator('[data-testid="repo-row"][data-repo-id="repo-port-1"]');
  await row.click({ button: 'right' });
  await page.locator(t('menu-item-close')).click();
  await row.click();
  await page.locator(t('git-panel-tab-review')).click();
  await host(page)
    .locator(t(`review-branch-option-${REVIEW_BRANCH}`))
    .click();
  await expect(host(page).locator(t('review-row-header')).first()).toBeVisible();
  await toolbar(page).locator(t('review-pane-files')).click();
  await expect(page.locator(t('review-file-filter'))).toHaveCount(0);
  await expect(page.locator(t('review-filter-toggle'))).not.toHaveAttribute('aria-pressed', 'true');
  await expect(host(page).locator('[aria-selected="true"]')).toHaveCount(0);
});
