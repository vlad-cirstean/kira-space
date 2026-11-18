import type { Page } from '@playwright/test';

/** Reka's radio checks a card from a `setTimeout(0)` after arrow focus, and only while the arrow is
 *  still down. A zero-hold `press` can release first and skip the check; a real key is held longer. */
export async function pressArrow(page: Page, key: 'ArrowUp' | 'ArrowDown'): Promise<void> {
  await page.keyboard.press(key, { delay: 40 });
}
