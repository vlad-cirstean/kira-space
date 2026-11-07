import type { Locator, Page } from '@playwright/test';
import { expect, reviewTest } from './fixtures';
import { contract } from './support/contract';
import { openDiffWindow, type Relaunch, t } from './support/reviewHunkDiff';

// The review window's Files list follows the file the centre shows: cross-file next/previous
// change and a tab click move the highlight, expand its folder and scroll it into view.

const FILLER = Array.from({ length: 40 }, (_, i) => `lib/f${String(i + 1).padStart(2, '0')}.txt`);
const FILES = [...FILLER, 'src/a.txt', 'zz/b.txt'];

const read = (file: 'a' | 'b', side: 'left' | 'right') =>
  contract('git-review-nav', `git:file.read#${file}-${side}`);
const reads = (order: ('a' | 'b')[]) => ({
  __sequence: order.flatMap((f) => [read(f, 'left'), read(f, 'right')]),
});

const open = (relaunch: Relaunch, order: ('a' | 'b')[]) =>
  openDiffWindow(relaunch, FILES, { 'file.read': reads(order) });

const tree = (page: Page) => page.locator(t('review-files-tree'));
const row = (page: Page, name: string) => tree(page).locator(t('file-tree-row'), { hasText: name });
const scroller = (page: Page) => tree(page).locator('[role="tree"]');

async function inView(page: Page, target: Locator): Promise<boolean> {
  const [box, area] = await Promise.all([target.boundingBox(), scroller(page).boundingBox()]);
  if (!box || !area) return false;
  // 1px absorbs sub-pixel layout.
  return box.y >= area.y - 1 && box.y + box.height <= area.y + area.height + 1;
}
const expectInView = (page: Page, target: Locator) =>
  expect.poll(() => inView(page, target)).toBe(true);

const scrollTree = (page: Page, to: 'top' | 'bottom') =>
  scroller(page).evaluate((el, edge) => {
    el.scrollTop = edge === 'top' ? 0 : el.scrollHeight;
  }, to);

reviewTest('next and previous change reveal the file in the list', async ({ relaunch }) => {
  const page = await open(relaunch, ['a', 'b', 'a']);
  await row(page, 'a.txt').click();
  await row(page, 'zz').click();
  await expect(row(page, 'zz')).toHaveAttribute('aria-expanded', 'false');
  await scrollTree(page, 'top');
  const modified = page.locator(t('repo-diff-editor')).locator('.editor.modified');
  const next = page.locator(t('repo-diff-next-change'));
  await expect(modified.locator('.view-lines')).toContainText('CHANGED 20');

  await next.click();
  await next.click();
  await next.click();
  await expect(modified.locator('.view-lines')).toContainText('CHANGED 60');
  await expect(row(page, 'zz')).toHaveAttribute('aria-expanded', 'true');
  await expect(row(page, 'b.txt')).toHaveAttribute('aria-selected', 'true');
  await expect(row(page, 'a.txt')).toHaveAttribute('aria-selected', 'false');
  await expectInView(page, row(page, 'b.txt'));

  await scrollTree(page, 'bottom');
  const previous = page.locator(t('repo-diff-prev-change'));
  await previous.click();
  await previous.click();
  await expect(modified.locator('.view-lines')).toContainText('CHANGED 20');
  await expect(row(page, 'a.txt')).toHaveAttribute('aria-selected', 'true');
  await expectInView(page, row(page, 'a.txt'));
});

reviewTest('a tab click moves the list to its file without opening a tab', async ({ relaunch }) => {
  const page = await open(relaunch, ['a', 'b', 'a', 'b', 'a']);
  await row(page, 'a.txt').dblclick();
  await row(page, 'b.txt').dblclick();
  const tabs = page.locator(t('ade-review-tabs')).locator(t('tab'));
  await expect(tabs).toHaveCount(2);
  await expect(row(page, 'b.txt')).toHaveAttribute('aria-selected', 'true');
  await scrollTree(page, 'bottom');
  await scrollTree(page, 'top');

  await tabs.filter({ hasText: 'a.txt' }).click();
  await expect(row(page, 'a.txt')).toHaveAttribute('aria-selected', 'true');
  await expect(row(page, 'b.txt')).toHaveAttribute('aria-selected', 'false');
  await expectInView(page, row(page, 'a.txt'));
  await expect(tabs).toHaveCount(2);
});
