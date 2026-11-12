import type { Locator, Page } from '@playwright/test';

/** Drags from the box's top-left to its bottom-right and returns the page selection. */
export async function dragAcross(page: Page, target: Locator): Promise<string> {
  await target.scrollIntoViewIfNeeded();
  const box = await target.boundingBox();
  if (!box) throw new Error('drag target has no box');
  await page.mouse.move(box.x + 1, box.y + 1);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width - 1, box.y + box.height - 1, { steps: 5 });
  await page.mouse.up();
  return page.evaluate(() => window.getSelection()?.toString() ?? '');
}
