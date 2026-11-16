import type { Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { emitPrompts, routed } from '@workbench/testing/ui/prompts';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// Prompt dialogs never take the screen while the user types, and answer to two-key chords only.
// The quiet time is PROMPT_QUIET_MS (4 s); the page clock jumps it.

const t = (id: string) => `[data-testid="${id}"]`;
const QUIET = 4100;

function approval(): ApprovalItem {
  const [it] = contract<ApprovalItem[]>('ade-orchestrator', 'ApprovalsService.List#taskCreate');
  if (!it) throw new Error('contract has no item');
  return liveItem(it);
}

// A text field the user is typing into.
async function startTyping(page: Page): Promise<void> {
  await page.evaluate(() => {
    const input = document.createElement('input');
    input.setAttribute('data-testid', 'typing-field');
    document.body.appendChild(input);
    input.focus();
  });
  await page.keyboard.type('hello');
}

test('an approval that arrives while typing shows a toast; the dialog opens after the pause', async ({
  relaunch,
}) => {
  const it = approval();
  const { window: page } = await relaunch({});
  await page.clock.install();
  await startTyping(page);
  await pushApprovals(page, [it]);
  await expect(page.locator(t('approvals-toast'))).toBeVisible();
  await expect(page.locator(t('approvals-toast-typing'))).toBeVisible();
  await expect(page.locator(t('approvals-dialog'))).toHaveCount(0);
  await page.clock.runFor(QUIET);
  await expect(page.locator(t('approvals-dialog'))).toBeVisible();
});

test('Show on the toast opens the dialog at once', async ({ relaunch }) => {
  const it = approval();
  const { window: page } = await relaunch({});
  await page.clock.install();
  await startTyping(page);
  await pushApprovals(page, [it]);
  await page.locator(t('approvals-toast-review')).click();
  await expect(page.locator(t('approvals-dialog'))).toBeVisible();
});

test('single keys do nothing in an open approval; Mod+Enter approves', async ({ relaunch }) => {
  const it = approval();
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk([it.id]) }],
  });
  await pushApprovals(page, [it]);
  const dialog = page.locator(t('approvals-dialog'));
  await expect(dialog).toBeVisible();
  for (const key of ['Enter', 'Space', 'a', 'Backspace', 'Escape']) await page.keyboard.press(key);
  await expect(dialog).toBeVisible();
  expect(calls(control, IPC.approvalsAnswer)).toHaveLength(0);
  await page.keyboard.press('Meta+Enter');
  await expect.poll(() => calls(control, IPC.approvalsAnswer)).toHaveLength(1);
  expect(calls(control, IPC.approvalsAnswer)[0]?.args).toMatchObject({
    ids: [it.id],
    approve: true,
  });
});

test('Mod+Backspace denies and Shift+Escape minimises', async ({ relaunch }) => {
  const it = approval();
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk([it.id]) }],
  });
  await pushApprovals(page, [it]);
  await expect(page.locator(t('approvals-dialog'))).toBeVisible();
  await page.keyboard.press('Shift+Escape');
  await expect(page.locator(t('approvals-toast'))).toBeVisible();
  await page.locator(t('approvals-toast-review')).click();
  await page.keyboard.press('Meta+Backspace');
  await expect.poll(() => calls(control, IPC.approvalsAnswer)).toHaveLength(1);
  expect(calls(control, IPC.approvalsAnswer)[0]?.args).toMatchObject({
    ids: [it.id],
    approve: false,
  });
});

test('a routed prompt waits for the pause while the user types', async ({ relaunch }) => {
  const { window: page } = await relaunch({});
  await page.clock.install();
  await startTyping(page);
  await emitWailsEvent(page, IPC.gitCredential, [
    {
      requestId: 'req-1',
      source: 'ADE board',
      repoLabel: 'demo-repo',
      prompt: "Password for 'https://example.com':",
      masked: true,
    },
  ]);
  await emitPrompts(page, [routed('git-credential', 'req-1')]);
  await expect(page.locator(t('prompts-toast'))).toBeVisible();
  await expect(page.locator(t('git-credential-dialog'))).toHaveCount(0);
  await page.clock.runFor(QUIET);
  await expect(page.locator(t('git-credential-dialog'))).toBeVisible();
  await expect(page.locator(t('prompts-toast'))).toHaveCount(0);
});
