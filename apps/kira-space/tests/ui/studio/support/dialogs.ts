import { expect, type Page } from '@playwright/test';

// Ported byte-identically from tests/e2e/support/dialogs.ts — always a Teleported HTML
// dialog, never Electron's own native one; nothing here was Electron-specific to begin with.
//
// ConfirmDialog.vue's replacement for window.confirm() (see state/confirmDialog.ts) — an ordinary
// Teleported HTML dialog rather than a native OS panel, so it's another locator to click
// instead of a page.on('dialog') handler racing Electron's own (unreliable, CDP-invisible) native
// dialog manager.
export async function acceptConfirm(page: Page): Promise<void> {
  const dialog = page.locator('[data-testid="confirm-dialog"]');
  await expect(dialog).toBeVisible();
  await page.click('[data-testid="confirm-dialog-confirm"]');
  await expect(dialog).toHaveCount(0);
}

export async function cancelConfirm(page: Page): Promise<void> {
  const dialog = page.locator('[data-testid="confirm-dialog"]');
  await expect(dialog).toBeVisible();
  await page.click('[data-testid="confirm-dialog-cancel"]');
  await expect(dialog).toHaveCount(0);
}
