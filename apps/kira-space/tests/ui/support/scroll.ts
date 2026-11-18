import type { Locator } from '@playwright/test';
import { expect } from '@playwright/test';

export const scrollTop = (el: Locator) => el.evaluate((e) => e.scrollTop);

/** Sets `scrollTop`, waits until it sticks above 0 and returns the settled value. */
export async function scrollTo(el: Locator, top: number): Promise<number> {
  await el.evaluate((e, y) => {
    e.scrollTop = y;
  }, top);
  await expect.poll(() => scrollTop(el)).toBeGreaterThan(0);
  return scrollTop(el);
}
