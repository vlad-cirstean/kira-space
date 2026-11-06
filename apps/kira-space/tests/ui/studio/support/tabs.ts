import { expect, type Page } from '@playwright/test';
import { settleFrames } from './tree';

// Same race openRowMenu() guards against, but against the tab strip's own scroll.
export async function closeAllTabs(page: Page): Promise<void> {
  const firstTab = page.locator('[data-testid="tab"]').first();
  await firstTab.scrollIntoViewIfNeeded();
  await settleFrames(page);
  await firstTab.click({ button: 'right' });
  await page.click('[data-testid="menu-item-close-all"]');
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(0);
}

export function tabLocator(page: import('@playwright/test').Page, tabId: string) {
  return page.locator(`[data-testid="tab"][data-tab-id="${tabId}"]`);
}
