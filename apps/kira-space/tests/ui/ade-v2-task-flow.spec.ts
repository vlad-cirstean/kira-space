import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { adeFixture, openPlan } from './support/adeV2';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// Task flow: archive confirmation, estimate set/change/clear, inline panel title, branch tabs.
// Backend half: adeflow TestNewTaskLandsTodayWithoutRepos, TestEstimateSetChangeClear. The
// new-task case (Today, no repos) lives in ade-v2-add.spec.ts.

const t = (id: string) => `[data-testid="${id}"]`;

interface BoardFx {
  tasks: { id: string; est: string }[];
}

async function openTask(page: Page, id: string): Promise<void> {
  await page
    .locator(`[data-testid="ade-card"][data-task-id="${id}"] [data-testid="ade-card-head"]`)
    .click();
  await expect(page.locator(t('ade-panel'))).toBeVisible();
}

const CLEAR_RISK = {
  taskId: 'T_cart',
  branches: [
    {
      branchId: 'b_cart',
      worktree: '~/kira-space-wts/web-app/cart',
      dirty: [],
      unmerged: 0,
      blocked: '',
    },
  ],
};

test('Archive asks first: Cancel sends nothing, Archive sends it', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskArchiveRisk, response: CLEAR_RISK },
  ]);
  await openTask(page, 'T_cart');
  await page.locator(t('ade-panel-more')).click();
  await page.locator(t('menu-item-ade-task-archive')).click();
  await expect(page.locator(t('confirm-dialog'))).toBeVisible();
  await page.locator(t('confirm-dialog-cancel')).click();
  await expect(page.locator(t('confirm-dialog'))).toHaveCount(0);
  expect(calls(control, IPC.adeTaskArchiveTask)).toHaveLength(0);

  await page.locator(t('ade-panel-more')).click();
  await page.locator(t('menu-item-ade-task-archive')).click();
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect.poll(() => calls(control, IPC.adeTaskArchiveTask)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskArchiveTask)[0]?.args).toEqual({ taskId: 'T_cart' });
});

test('estimate: set, change and clear at any time', async ({ relaunch }) => {
  const board = adeFixture<BoardFx>('board');
  const notif = board.tasks.find((x) => x.id === 'T_notif');
  if (notif) notif.est = '';
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  const patches = () => calls(control, IPC.adeTaskUpdateTask).map((c) => c.args);

  await openTask(page, 'T_notif');
  await expect(page.locator(t('ade-estimate-clear'))).toHaveCount(0);
  await page.locator(t('ade-estimate-num')).fill('3');
  await page.locator(t('ade-estimate-unit-d')).click();
  await expect.poll(patches).toHaveLength(1);
  expect(patches()[0]).toMatchObject({ taskId: 'T_notif', patch: { est: '3d' } });

  await openTask(page, 'T_bill');
  await expect(page.locator(t('ade-estimate-num'))).toHaveValue('2');
  await page.locator(t('ade-estimate-num')).fill('1');
  await page.locator(t('ade-estimate-num')).press('Tab');
  await expect.poll(patches).toHaveLength(2);
  expect(patches()[1]).toMatchObject({ taskId: 'T_bill', patch: { est: '1d' } });
  // The board refetch after a save resets the field; wait for it before typing again.
  await expect(page.locator(t('ade-estimate-num'))).toHaveValue('2');

  await page.locator(t('ade-estimate-num')).fill('1');
  await page.locator(t('ade-estimate-unit-h')).click();
  await expect.poll(patches).toHaveLength(3);
  expect(patches()[2]).toMatchObject({ patch: { est: '1h' } });

  await page.locator(t('ade-estimate-clear')).click();
  await expect.poll(patches).toHaveLength(4);
  expect(patches()[3]).toMatchObject({ patch: { est: '' } });
});

test('the panel title is the one editable name field', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await openTask(page, 'T_notif');
  await expect(page.locator(t('ade-task-name'))).toHaveCount(0);
  const title = page.locator(t('ade-panel-title'));
  await expect(title).toHaveValue('In-app notifications');
  await title.fill('Notify on mention');
  await title.press('Enter');
  await expect.poll(() => calls(control, IPC.adeTaskUpdateTask)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskUpdateTask)[0]?.args).toMatchObject({
    taskId: 'T_notif',
    patch: { title: 'Notify on mention' },
  });
});

test('a branch panel has Details and Sessions tabs only', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openTask(page, 'T_auth');
  await page.locator('[data-testid="ade-task-branch"][data-branch-id="b_auth"]').click();
  await expect(page.locator(t('ade-panel-tab-details'))).toBeVisible();
  await expect(page.locator(t('ade-panel-tab-sessions'))).toBeVisible();
  await expect(page.locator(t('ade-panel-tab-changes'))).toHaveCount(0);
});
