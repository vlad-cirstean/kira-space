import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, test } from './fixtures';

// Real PTY keystroke order; shell tab and script-in-folder moved to termflow + automations-module.spec.ts (P249). HOME is a temp dir so the
// default cwd and the login shell's rc files are isolated from the machine running the suite.

const home = mkdtempSync(join(tmpdir(), 'kira-e2e-home-'));
test.use({ serverEnv: { HOME: home } });
// Not afterAll: with fullyParallel it runs between tests of one worker, deleting the HOME the next test uses.
process.once('exit', () => rmSync(home, { recursive: true, force: true }));

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
