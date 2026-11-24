import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { adeBoardSnapshot, openPlan, openTask } from './support/adeV2';

// Panel header title never touches the colour line, at any title length, width or app font size.
// Pure layout: no IPC or state change, so no backend half.

const t = (id: string) => `[data-testid="${id}"]`;
const LONG = 'Gypsy hypergraph typography query pyramid '.repeat(8).trim();

async function expectClear(page: Page): Promise<void> {
  const [head, line, title, tab] = await Promise.all([
    page.locator(t('ade-panel-head')).boundingBox(),
    page.locator(`${t('ade-panel-head')} ${t('ade-task-line')}`).boundingBox(),
    page.locator(t('ade-panel-title')).boundingBox(),
    page.locator('[data-testid^="ade-panel-tab-"]').first().boundingBox(),
  ]);
  if (!head || !line || !title || !tab) throw new Error('panel header boxes missing');
  expect(line.y).toBeGreaterThanOrEqual(title.y + title.height + 2);
  expect(Math.abs(line.y + line.height - (head.y + head.height))).toBeLessThanOrEqual(1);
  expect(tab.y).toBeGreaterThanOrEqual(line.y + line.height - 1);
  const lineHeight = await page
    .locator(t('ade-panel-title'))
    .evaluate((el) => Number.parseFloat(getComputedStyle(el).lineHeight));
  expect(title.height).toBeGreaterThanOrEqual(lineHeight);
}

const setFont = (page: Page, px: number) =>
  page.evaluate(
    (v) => document.documentElement.style.setProperty('--kira-font-size', `${v}px`),
    px,
  );

test('a long task title stays clear of the colour line at every width and font size', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch, [
    adeBoardSnapshot((b) => {
      const task = b.tasks.find((x) => x.id === 'T_alerts');
      if (task) task.title = LONG;
    }),
  ]);
  for (const width of [1440, 900]) {
    await page.setViewportSize({ width, height: 900 });
    for (const font of [12, 20]) {
      await setFont(page, font);
      await openTask(page, 'T_alerts');
      const title = page.locator(t('ade-panel-title'));
      await expect(title).toHaveValue(LONG);
      await expectClear(page);
      expect(await title.evaluate((el) => el.scrollWidth > el.clientWidth)).toBe(true);
      expect(await title.evaluate((el) => getComputedStyle(el).textOverflow)).toBe('ellipsis');
      await title.focus();
      await expectClear(page);
      await title.evaluate((el) => (el as HTMLInputElement).blur());
    }
  }
});

test('a wrapped long branch name stays clear of the colour line', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    adeBoardSnapshot((b) => {
      const branch = b.branches.find((x) => x.id === 'b_cart');
      if (branch) branch.name = `feature/${LONG.replaceAll(' ', '-')}`;
    }),
  ]);
  await page.setViewportSize({ width: 900, height: 900 });
  await page.locator(`${t('ade-branch-row')}[data-branch-id="b_cart"]`).click();
  await expect(page.locator(t('ade-panel-title'))).toContainText('feature/Gypsy');
  await expectClear(page);
});
