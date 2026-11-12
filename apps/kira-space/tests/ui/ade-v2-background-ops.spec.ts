import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { adeFixture, openPlan, openTask } from './support/adeV2';
import { pick } from './support/branchMenu';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// Remove branch and archive run in the background: the dialog closes at once, a chip and a toast
// show progress. Backend half: adeflow TestTaskReposAndBranches and TestArchiveRisk.

const t = (id: string) => `[data-testid="${id}"]`;
const toast = (page: Page) => page.locator('[data-sonner-toast]');
const chip = (page: Page) => page.locator(t('ade-op-status'));

const dirty = { channel: IPC.adeTaskTaskBranchRisk, response: adeFixture('branch-risk-dirty') };

test('a confirmed branch removal closes the dialog and runs behind a chip and a toast', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    dirty,
    { channel: IPC.adeTaskRemoveTaskBranch, hold: true },
  ]);
  await openTask(page, 'T_cart');
  await pick(page, 'b_cart', 'ade-task-branch-remove');
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect.poll(() => calls(control, IPC.adeTaskRemoveTaskBranch)).toHaveLength(1);
  await expect(page.locator(t('confirm-dialog'))).toHaveCount(0);
  await expect(chip(page).first()).toHaveAttribute('data-op', /removeBranch|removeRepo/);
  await expect(toast(page)).toContainText('Removing');
  control.release(IPC.adeTaskRemoveTaskBranch);
  await expect(chip(page)).toHaveCount(0);
  await expect(toast(page)).toContainText('Removed');
});

test('a repo removal with nothing at risk also runs in the background', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskRemoveTaskRepo, hold: true },
  ]);
  await openTask(page, 'T_alerts');
  await pick(page, 'd_alerts_web', 'ade-task-repo-remove');
  await expect.poll(() => calls(control, IPC.adeTaskRemoveTaskRepo)).toHaveLength(1);
  await expect(chip(page).first()).toHaveAttribute('data-op', 'removeRepo');
  control.release(IPC.adeTaskRemoveTaskRepo);
  await expect(chip(page)).toHaveCount(0);
});

test('a refused removal ends the chip and shows an error toast', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    dirty,
    {
      channel: IPC.adeTaskRemoveTaskBranch,
      hold: true,
      error: { code: 'E_BAD_REQUEST', message: 'worktree is locked' },
    },
  ]);
  await openTask(page, 'T_cart');
  await pick(page, 'b_cart', 'ade-task-branch-remove');
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect(chip(page).first()).toBeVisible();
  control.release(IPC.adeTaskRemoveTaskBranch);
  await expect(chip(page)).toHaveCount(0);
  await expect(toast(page)).toContainText('worktree is locked');
});

test('Delete anyway closes the dialog and archives behind a chip', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskArchiveRisk,
      response: {
        taskId: 'T_cart',
        branches: [
          {
            branchId: 'b_cart',
            worktree: '~/kira-space-wts/web-app/cart',
            dirty: [{ code: 'M', path: 'src/cart.ts' }],
            unmerged: 2,
            blocked: '',
          },
        ],
      },
    },
    { channel: IPC.adeTaskArchiveTask, hold: true },
  ]);
  await page
    .locator('[data-testid="ade-card"][data-task-id="T_cart"] [data-testid="ade-card-head"]')
    .click();
  await page.locator(t('ade-panel-more')).click();
  await page.locator(t('menu-item-ade-task-archive')).click();
  await page.locator(t('ade-dialog-delete')).click();
  await expect.poll(() => calls(control, IPC.adeTaskArchiveTask)).toHaveLength(1);
  await expect(page.locator(t('ade-dialog'))).toBeHidden();
  await expect(chip(page).first()).toHaveAttribute('data-op', 'archive');
  await expect(toast(page)).toContainText('Closing task');
  control.release(IPC.adeTaskArchiveTask);
  await expect(chip(page)).toHaveCount(0);
  await expect(toast(page)).toContainText('Closed');
});
