import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { adeBoard, emitAgentEvent, emitAgentSessions, openPlan } from './support/adeV2';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// Rebase / queue after / merge: the Claude dialog, its blocks, delivery and what is recorded.

const t = (id: string) => `[data-testid="${id}"]`;
const row = (id: string) => `[data-testid="ade-branch-row"][data-branch-id="${id}"]`;

async function openCard(page: Page, taskId: string): Promise<void> {
  await page
    .locator(`[data-testid="ade-card"][data-task-id="${taskId}"] [data-testid="ade-card-head"]`)
    .click();
  await expect(page.locator(t('ade-panel'))).toBeVisible();
}

test('Rebase on ↓N main shows the server prompt and starts a background run', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch);
  await page
    .locator(`${row('b_auth')} >> xpath=ancestor::*[@data-testid="ade-task"][1]`)
    .locator(t('ade-rebase'))
    .first()
    .click();
  const dialog = page.locator(t('ade-dialog'));
  await expect(dialog).toBeVisible();
  await expect(page.locator(t('ade-dialog-title'))).toHaveText('Rebase onto main');
  await expect(page.locator(t('ade-dialog-message'))).toHaveValue(
    'Rebase feat/billing onto origin/main.',
  );
  await expect(page.locator(t('ade-dialog-suffix'))).toHaveText(/finish_step/);
  await expect(page.locator(t('ade-dialog-target'))).toHaveCount(0);
  await expect(page.locator(t('ade-dialog-send'))).toHaveText('Run in background');
  await page.locator(t('ade-dialog-send')).click();
  await expect(dialog).toBeHidden();
  const sent = calls(control, IPC.adeTaskRebase);
  expect(sent).toHaveLength(1);
  expect(sent[0]?.args).toMatchObject({ branchId: 'b_auth', message: '', onto: null });
  expect(calls(control, IPC.adeTaskSend)).toHaveLength(0);
});

test('a blank message disables Send', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page
    .locator(`${row('b_auth')} >> xpath=ancestor::*[@data-testid="ade-task"][1]`)
    .locator(t('ade-rebase'))
    .first()
    .click();
  await page.locator(t('ade-dialog-message')).fill('   ');
  await expect(page.locator(t('ade-dialog-send'))).toBeDisabled();
  await page.locator(t('ade-dialog-reset')).click();
  await expect(page.locator(t('ade-dialog-send'))).toBeEnabled();
});

test('queue after a review branch sends Rebase with queueWith', async ({ relaunch }) => {
  const board = adeBoard((bd) => {
    (bd.pairs as unknown[]).push({
      a: 'b_authui',
      b: 'b_sara',
      shared: ['src/payments/client.ts'],
      conflicts: ['src/payments/client.ts'],
    });
  });
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  await page.locator(row('b_authui')).click();
  await page.locator(t('ade-panel-action-queue')).click();
  await page.locator(t('ade-dialog-send')).click();
  await expect.poll(() => calls(control, IPC.adeTaskRebase)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskRebase)[0]?.args).toMatchObject({ queueWith: 'b_sara' });
  expect(calls(control, IPC.adeTaskSetQueuedAfter)).toHaveLength(0);
});

test('a TUI session at work lists a busy block with an override', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await emitAgentSessions(page, ['term-tk01']);
  await emitAgentEvent(page, 'term-tk01', 'UserPromptSubmit');
  await page.locator(row('b_billdash')).click();
  await page.locator(t('ade-panel-action-start')).count();
  await page.locator(row('b_billdash')).click({ button: 'right' });
  await page.getByRole('menuitem', { name: /Merge into develop/ }).click();
  await expect(page.locator(t('ade-dialog'))).toBeVisible();
});

test('a running background run disables the rebase act with a tip', async ({ relaunch }) => {
  const board = adeBoard((bd) => {
    const br = bd.branches.find((x) => x.id === 'b_bill');
    if (br) br.behind = 2;
  });
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  await page.locator(row('b_bill')).click({ button: 'right' });
  await expect(page.getByRole('menuitem', { name: /Rebase onto main/ })).toBeDisabled();
});

test('Merge from the fix menu: path, push line, recorded only after a Stop', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch);
  await page.locator(row('b_authui')).click({ button: 'right' });
  await page.getByRole('menuitem', { name: 'Merge into develop' }).click();
  await expect(page.locator(t('ade-dialog-title'))).toHaveText('Merge into develop');
  const message = page.locator(t('ade-dialog-message'));
  await expect(message).toHaveValue(
    /~\/kira-space-wts\/acme-customer-dashboard-web-frontend\/_develop/,
  );
  await expect(page.locator(t('ade-dialog-push-label'))).toHaveText('Also push develop');
  await page.locator(t('ade-dialog-push')).click();
  await expect(message).toHaveValue(/git push origin develop/);
  await page
    .locator(t('ade-dialog-target-0'))
    .locator('button', { hasText: 'new session' })
    .click();
  await page.locator(t('ade-dialog-send')).click();
  await expect.poll(() => calls(control, IPC.adeTaskStartBranch)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskRecordMerge)).toHaveLength(0);
  await emitAgentSessions(page, ['term-tk01']);
  await emitAgentEvent(page, 'term-tk01', 'Stop');
  await expect.poll(() => calls(control, IPC.adeTaskRecordMerge)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskRecordMerge)[0]?.args).toEqual({
    branchId: 'b_authui',
    target: 'develop',
  });
});

test('a session that ends first records nothing and says so', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await page.locator(row('b_authui')).click({ button: 'right' });
  await page.getByRole('menuitem', { name: 'Merge into develop' }).click();
  await page
    .locator(t('ade-dialog-target-0'))
    .locator('button', { hasText: 'new session' })
    .click();
  await page.locator(t('ade-dialog-send')).click();
  await expect.poll(() => calls(control, IPC.adeTaskStartBranch)).toHaveLength(1);
  await openCard(page, 'T_auth');
  await emitAgentEvent(page, 'term-tk01', 'SessionEnd');
  await expect(page.locator(t('ade-panel-action-error'))).toContainText(
    'The session ended before the merge finished; nothing recorded.',
  );
  expect(calls(control, IPC.adeTaskRecordMerge)).toHaveLength(0);
});

test('Details lists Merge and Re-merge buttons and the stale tip invites a right click', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator(row('b_auth')).click();
  const develop = page.locator('[data-testid="ade-branch-into"][data-target="develop"]');
  await expect(develop.locator(t('ade-branch-merge'))).toHaveText('Re-merge');
  // reka tooltips open on pointermove; a pointer already resting on the target dispatches none.
  await page.mouse.move(0, 0);
  await page.locator(row('b_auth')).getByText('dev ⚠').hover();
  await expect(page.getByText('Right-click to re-merge.').first()).toBeVisible();
  await develop.locator(t('ade-branch-merge')).click();
  await expect(page.locator(t('ade-dialog-title'))).toHaveText('Re-merge into develop');
});

test('the fix menu offers Nothing to fix on a clean merged branch', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator(row('b_cart')).click({ button: 'right' });
  await expect(page.getByRole('menuitem', { name: 'Nothing to fix' })).toBeDisabled();
});

test('the fix menu lists re-merge, rebase onto main and force push', async ({ relaunch }) => {
  const board = adeBoard((b) => {
    (b.plan as { unpushed: Record<string, boolean> }).unpushed = { b_auth: true };
  });
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  await page.locator(row('b_auth')).click({ button: 'right' });
  for (const name of ['Re-merge into develop (stale)', 'Rebase onto main', 'Force push']) {
    await expect(page.getByRole('menuitem', { name })).toBeVisible();
  }
  await page.keyboard.press('Escape');
  await openCard(page, 'T_auth');
});
