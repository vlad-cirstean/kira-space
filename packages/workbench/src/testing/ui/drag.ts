import type { Locator, Page } from '@playwright/test';

export interface Point {
  x: number;
  y: number;
}

export function boxCenter(box: { x: number; y: number; width: number; height: number }): Point {
  return { x: box.x + box.width / 2, y: box.y + box.height / 2 };
}

/**
 * Presses at `from`, drags to `to` and hovers there without releasing; the caller releases with
 * `page.mouse.up()`. `up` too early drops before Sortable has swapped.
 */
export async function sortableHover(
  page: Page,
  from: Point,
  to: Point,
  { start = { x: 0, y: 10 } }: { start?: Point } = {},
): Promise<void> {
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(from.x + start.x, from.y + start.y, { steps: 5 });
  await page.mouse.move(to.x, to.y, { steps: 15 });
  await page.mouse.move(to.x, to.y, { steps: 2 });
  // Sortable's fallback mode emulates dragover on a 50ms interval, one swap per tick; no DOM
  // signal marks the swap done.
  await page.waitForTimeout(300);
}

/** Drags `source` past the window edge (client `to`, negative x is left of the viewport) and
 *  releases there; the pointer stays captured, so the release still reaches the page. */
export async function dragTabOut(page: Page, source: Locator, to: Point): Promise<void> {
  const from = await source.boundingBox();
  if (!from) throw new Error('dragTabOut: chip has no box');
  const start = boxCenter(from);
  await page.mouse.move(start.x, start.y);
  await page.mouse.down();
  await page.mouse.move(start.x + 10, start.y, { steps: 5 });
  await page.mouse.move(to.x, to.y, { steps: 15 });
  await page.waitForTimeout(150);
  await page.mouse.up();
}

export async function dragTab(page: Page, source: Locator, target: Locator): Promise<void> {
  const from = await source.boundingBox();
  const to = await target.boundingBox();
  if (!from || !to) throw new Error('dragTab: chip has no box');
  // Aim past the target's midpoint in the drag direction so Sortable's swap threshold is met.
  const dropX = to.x + to.width * (to.x < from.x ? 0.25 : 0.75);
  await sortableHover(
    page,
    boxCenter(from),
    { x: dropX, y: to.y + to.height / 2 },
    { start: { x: 10, y: 0 } },
  );
  await page.mouse.up();
}
