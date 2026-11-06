import type { Page } from '@playwright/test';
import { openMainWindow } from '@workbench/testing/e2eReal';
import { expect, test } from './fixtures';

// P246: a due confirm popup shows in the app's main window only. Two pages stand in for two
// windows: the main window's key and a random one. `Not now` in the main page skips the run, and
// both pages list it in Operations.

async function addRecurring(page: Page, name: string): Promise<void> {
  await page.locator('[data-testid="mode-tab"][data-mode="automations"]').click();
  await page.locator('[data-testid="automations-add"]').click();
  await page.locator('[data-testid="menu-item-new-script"]').click();
  const dialog = page.locator('[data-testid="run-file-dialog"]');
  await dialog.locator('[data-testid="script-dialog-name"]').fill(name);
  await dialog.locator('[data-testid="script-dialog-command"]').fill('echo prompt-routing');
  await dialog.locator('[data-testid="run-file-tab-schedule"]').click();
  await dialog.locator('[data-testid="script-schedule"]').click();
  await dialog.locator('[data-testid="schedule-cron"]').fill('* * * * *');
  await expect(dialog.locator('[data-testid="schedule-next"]')).toContainText('Next:');
  await dialog.locator('[data-testid="run-file-save"]').click();
  // Save keeps the dialog open; Close reads "Close" once the draft is clean.
  await expect(dialog.locator('[data-testid="run-file-cancel"]')).toHaveText('Close');
  await dialog.locator('[data-testid="run-file-cancel"]').click();
  await expect(dialog).toHaveCount(0);
}

test('the confirm popup shows in the main window only; Not now skips in both', async ({
  kira,
  browser,
}) => {
  test.setTimeout(150_000);
  const main = kira.window;
  await openMainWindow(kira);
  const other = await browser.newPage();
  await other.goto(`${kira.baseURL}/?window=other-${Date.now()}`);
  await other.waitForSelector('[data-testid="status-bar"]');

  await addRecurring(main, 'Routed echo');
  await other.locator('[data-testid="mode-tab"][data-mode="automations"]').click();

  const popup = main.locator('[data-testid="schedule-confirm"]');
  await expect(popup).toBeVisible({ timeout: 75_000 });
  await expect(other.locator('[data-testid="schedule-confirm"]')).toHaveCount(0);

  await popup.locator('[data-testid="schedule-confirm-decline"]').click();
  await expect(popup).toHaveCount(0);
  for (const page of [main, other]) {
    await page.locator('[data-testid="toggle-operations-panel"]').click();
    await expect(page.locator('[data-testid="op-row"][data-op-id^="run:"]').first()).toBeVisible();
  }
});
