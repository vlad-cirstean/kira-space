import type { Page } from '@playwright/test';

/** The app's primary-modifier chord for `key`, read from the page's own UA like `keys.ts`:
 *  Playwright's WebKit reports a Mac userAgent on every host, so `ControlOrMeta` is wrong here. */
export async function primaryChord(page: Page, key: string): Promise<string> {
  const isMac = await page.evaluate(() => navigator.userAgent.includes('Mac'));
  return `${isMac ? 'Meta' : 'Control'}+${key}`;
}

/** `tree.delete`: Cmd+Backspace on mac, Delete elsewhere. */
export async function deleteChord(page: Page): Promise<string> {
  const isMac = await page.evaluate(() => navigator.userAgent.includes('Mac'));
  return isMac ? 'Meta+Backspace' : 'Delete';
}
