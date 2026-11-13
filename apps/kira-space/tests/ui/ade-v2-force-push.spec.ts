import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { adeFixture, openPlan } from './support/adeV2';
import { IPC } from './support/ipcChannels';

// Force push: a branch rebased locally (`plan.unpushed`) offers it; the dialog names the repo.

interface BoardFx {
  plan: { unpushed: Record<string, boolean> };
}

function unpushedBoard(): BoardFx {
  const board = adeFixture<BoardFx>('board');
  board.plan.unpushed = { b_deps: true };
  return board;
}

test('asks first, names the repo and sends the branch', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: unpushedBoard() },
    { channel: IPC.adeTaskForcePush, response: adeFixture('force-push') },
  ]);
  await page.locator('[data-testid="ade-force-push"]').click();
  const dialog = page.locator('[data-testid="ade-confirm-dialog"]');
  await expect(dialog).toContainText('Force push?');
  await expect(dialog).toContainText('“chore/deps-bump” in web-app');
  await expect(dialog.locator('[data-testid="ade-confirm-yes"]')).toHaveText('Force push');
  expect(control.log().filter((e) => e.channel === IPC.adeTaskForcePush)).toHaveLength(0);
  await dialog.locator('[data-testid="ade-confirm-yes"]').click();
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.adeTaskForcePush))
    .toHaveLength(1);
  expect(control.log().find((e) => e.channel === IPC.adeTaskForcePush)?.args).toEqual({
    branchId: 'b_deps',
  });
  await expect(dialog).toHaveCount(0);
});

test('the danger dialog opens with Cancel focused and Escape closes it unsent', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: unpushedBoard() },
  ]);
  await page.locator('[data-testid="ade-force-push"]').click();
  const dialog = page.locator('[data-testid="ade-confirm-dialog"]');
  await expect(dialog.locator('[data-testid="ade-confirm-no"]')).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
  expect(control.log().filter((e) => e.channel === IPC.adeTaskForcePush)).toHaveLength(0);
});

const toast = (page: Page) => page.locator('[data-sonner-toast]');
const forcePushSnapshot = (response: unknown, hold = false) => ({
  channel: IPC.adeTaskForcePush,
  response,
  hold,
});

test('the backend error closes the dialog and lands in an error toast', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: unpushedBoard() },
    forcePushSnapshot({
      branchId: 'b_deps',
      error: { kind: 'stale-lease', message: 'remote has newer commits', remoteMessage: null },
    }),
  ]);
  await page.locator('[data-testid="ade-force-push"]').click();
  const dialog = page.locator('[data-testid="ade-confirm-dialog"]');
  await dialog.locator('[data-testid="ade-confirm-yes"]').click();
  await expect(dialog).toHaveCount(0);
  await expect(toast(page)).toContainText('remote has newer commits');
});

test('a push in flight shows the chip, and past 2 s a loading then a success toast', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: unpushedBoard() },
    forcePushSnapshot(adeFixture('force-push'), true),
  ]);
  await page.locator('[data-testid="ade-force-push"]').click();
  await page.locator('[data-testid="ade-confirm-yes"]').click();
  await expect(page.locator('[data-testid="ade-confirm-dialog"]')).toHaveCount(0);
  const chip = page.locator('[data-testid="ade-op-status"][data-op="forcePush"]');
  await expect(chip.first()).toBeVisible();
  await expect(toast(page)).toContainText('Force pushing', { timeout: 5000 });
  control.release(IPC.adeTaskForcePush);
  await expect(toast(page)).toContainText('Force pushed chore/deps-bump');
  await expect(chip).toHaveCount(0);
});

test('a force push from the panel branch list shows the chip on that row', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: unpushedBoard() },
    forcePushSnapshot(adeFixture('force-push'), true),
  ]);
  await page
    .locator('[data-testid="ade-card"][data-task-id="T_deps"] [data-testid="ade-card-head"]')
    .click();
  const row = page.locator(
    '[data-testid="ade-task-branch-item"][data-branch-id="b_deps"] [data-testid="ade-force-push"]',
  );
  await row.click();
  await page.locator('[data-testid="ade-confirm-yes"]').click();
  const chip = page.locator(
    '[data-testid="ade-task-branch-item"][data-branch-id="b_deps"] [data-testid="ade-op-status"][data-op="forcePush"]',
  );
  await expect(chip.first()).toBeVisible();
  control.release(IPC.adeTaskForcePush);
  await expect(chip).toHaveCount(0);
});

test('a push that finishes within 2 s shows no toast', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: unpushedBoard() },
    forcePushSnapshot(adeFixture('force-push')),
  ]);
  await page.locator('[data-testid="ade-force-push"]').click();
  await page.locator('[data-testid="ade-confirm-yes"]').click();
  await expect(page.locator('[data-testid="ade-confirm-dialog"]')).toHaveCount(0);
  await page.waitForTimeout(2600);
  await expect(toast(page)).toHaveCount(0);
});
