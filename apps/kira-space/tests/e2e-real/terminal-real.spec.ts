import { expect, test } from './fixtures';

// Fixtures already set HOME per test (Studio's variant only added a temp HOME).
// P232: Space's Terminal module shares Studio's terminal store, so per-key writes must reach a real
// shell in order here too.

test('keystrokes typed one by one reach the shell in order', async ({ kira }) => {
  const page = kira.window;
  await page.locator('[data-testid="mode-tab"][data-mode="automations"]').click();
  await page.locator('[data-testid="automations-start-new"]').click();
  await expect(page.locator('.xterm-rows')).toBeVisible();
  await page.locator('.xterm-helper-textarea').focus();
  await page.keyboard.type('echo kira-$((1+1))', { delay: 0 });
  await expect(page.locator('.xterm-rows')).toContainText('echo kira-$((1+1))');
  await page.keyboard.press('Enter');
  await expect(page.locator('.xterm-rows')).toContainText('kira-2');
});
