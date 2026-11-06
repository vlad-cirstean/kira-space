import type { Page } from '@playwright/test';
import { expect } from '../fixtures';

// P107 I2-25: openSettings was copied, byte-identically, across 3 spec files. One copy here,
// imported everywhere.

export async function openSettings(page: Page): Promise<void> {
  await page.click('[data-testid="open-settings"]');
  await expect(page.locator('[data-testid="settings-dialog"]')).toBeVisible();
}
