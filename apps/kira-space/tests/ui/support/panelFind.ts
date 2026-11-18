import type { Locator, Page } from '@playwright/test';
import { expect } from '@playwright/test';

// The app picks Cmd or Ctrl from the UA, which a Linux WebKit run reports as macOS.
export async function pressFind(page: Page): Promise<void> {
  const mac = await page.evaluate(() => navigator.userAgent.includes('Mac'));
  await page.keyboard.press(mac ? 'Meta+F' : 'Control+F');
}

/** Cmd/Ctrl+F with focus on `from` reveals `field`, focuses it, and a second press selects its text. */
export async function expectPanelFind(page: Page, from: Locator, field: Locator): Promise<void> {
  await from.focus();
  await pressFind(page);
  await expect(field).toBeVisible();
  await expect(field).toBeFocused();
  await page.keyboard.type('ab');
  await expect(field).toHaveValue('ab');
  await pressFind(page);
  await expect(field).toBeFocused();
  await page.keyboard.type('c');
  await expect(field).toHaveValue('c');
}
