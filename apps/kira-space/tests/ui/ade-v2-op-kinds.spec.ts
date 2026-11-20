import type { Page } from '@playwright/test';
import { TIMING } from '@workbench/behaviour/timing';
import { expect, test } from './fixtures';
import { adeFixture, openPlan, openTask } from './support/adeV2';
import { pick } from './support/branchMenu';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// Op kinds show progress as a chip and toast only a failure. One kind per surface. Backend half: the
// adeflow bridge calls these wrap stay awaitable until done.

const t = (id: string) => `[data-testid="${id}"]`;
const toast = (page: Page) => page.locator('[data-sonner-toast]');
const chip = (page: Page, kind: string) =>
  page.locator(`[data-testid="ade-op-status"][data-op="${kind}"]`);

const candidates = { channel: IPC.adeTaskCandidateBranches, response: adeFixture('candidates') };

test('setBranch: the form closes at once, the row shows a chip, no toast even past 2 s', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    candidates,
    { channel: IPC.adeTaskSetTaskBranch, response: adeFixture('board'), hold: true },
  ]);
  await openTask(page, 'T_alerts');
  await pick(page, 'd_alerts_api', 'ade-task-branch-add');
  await page.locator(t('ade-branch-form-search')).fill('feat/alerts-api');
  await page.clock.install();
  await page.locator(t('ade-branch-form-custom')).click();
  await expect(page.locator(t('ade-branch-form'))).toHaveCount(0);
  await expect(chip(page, 'setBranch').first()).toBeVisible();
  await page.clock.runFor(TIMING.slowOp + 500);
  await expect(toast(page)).toHaveCount(0);
  control.release(IPC.adeTaskSetTaskBranch);
  await expect(chip(page, 'setBranch')).toHaveCount(0);
  await expect(toast(page)).toHaveCount(0);
  expect(calls(control, IPC.adeTaskSetTaskBranch)).toHaveLength(1);
});

test('attachBranch: a refusal from Add task toasts the error', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    candidates,
    {
      channel: IPC.adeTaskAddExistingBranch,
      error: { code: 'E_BAD_REQUEST', message: 'branch is already in a task' },
    },
  ]);
  await page.locator(t('ade-add')).click();
  await page.locator(t('ade-add-tab-branch')).click();
  await page.locator(t('ade-branch-search')).fill('invoice');
  await page.locator('[data-testid^="ade-branch-option-"]').first().click();
  await expect(page.locator(t('ade-add-popover'))).toHaveCount(0);
  await expect(toast(page)).toContainText('branch is already in a task');
});

test('refresh: a failing fetch also toasts the repo and the error', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskRefresh, error: { code: 'E_INTERNAL', message: 'network down' } },
  ]);
  await page.locator(t('ade-refresh-all')).click();
  await expect(toast(page).first()).toContainText('network down');
  await expect(toast(page).first()).toContainText(/(web-app|mobile) — network down/);
});

test('stopRuns: the card shows a chip and no toast even past 2 s', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskStopRun, response: undefined, hold: true },
  ]);
  await page
    .locator('[data-testid="ade-card"][data-task-id="T_deps"]')
    .click({ button: 'right', position: { x: 60, y: 30 } });
  await page.clock.install();
  await page.locator('[data-testid="menu-item-ade-task-stop"]').click();
  await expect(chip(page, 'stopRuns').first()).toBeVisible();
  await page.clock.runFor(TIMING.slowOp + 500);
  await expect(toast(page)).toHaveCount(0);
  control.release(IPC.adeTaskStopRun);
  await expect(chip(page, 'stopRuns')).toHaveCount(0);
  await expect(toast(page)).toHaveCount(0);
  expect(calls(control, IPC.adeTaskStopRun)).toHaveLength(1);
});

test('stopRuns: a refusal toasts the error', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskStopRun, error: { code: 'E_INTERNAL', message: 'run already ended' } },
  ]);
  await page
    .locator('[data-testid="ade-card"][data-task-id="T_deps"]')
    .click({ button: 'right', position: { x: 60, y: 30 } });
  await page.locator('[data-testid="menu-item-ade-task-stop"]').click();
  await expect(toast(page)).toContainText('run already ended');
});
