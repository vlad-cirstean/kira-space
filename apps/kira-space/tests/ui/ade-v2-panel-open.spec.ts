import { expect, test } from './fixtures';
import { adeBoard, openPlan, selectTask } from './support/adeV2';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// The task panel never closes beside the Plan. Frontend only: no IPC or backend change.

const t = (id: string) => `[data-testid="${id}"]`;
const card = (id: string) => `[data-testid="ade-card"][data-task-id="${id}"]`;

test('the plan opens with the top task in the panel', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  const first = page.locator(t('ade-card')).first();
  await expect(first).toHaveAttribute('aria-current', 'true');
  await expect(page.locator(t('ade-panel'))).toBeVisible();
  await expect(page.locator(t('ade-panel-empty'))).toHaveCount(0);
});

test('the panel has no close control and stays after every switch', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await expect(page.locator(`${t('ade-panel')} [aria-label*="lose" i]`)).toHaveCount(0);
  await expect(page.locator(`${t('ade-panel')} [data-testid*="close" i]`)).toHaveCount(0);
  await selectTask(page, 'T_bill');
  await expect(page.locator(t('ade-panel'))).toBeVisible();
  await page.locator(`${t('ade-branch-row')}[data-branch-id="b_cart"]`).click();
  await page.locator(t('ade-panel-back')).click();
  await expect(page.locator(t('ade-panel'))).toBeVisible();
  await page.locator(t('ade-tab-later')).click();
  await page.locator(t('ade-tab-plan')).click();
  await expect(page.locator(t('ade-panel'))).toBeVisible();
});

test('a selected task that leaves the board falls back to the top task', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await selectTask(page, 'T_bill');
  const gone = adeBoard((b) => {
    b.tasks = b.tasks.filter((x) => x.id !== 'T_bill');
    b.branches = b.branches.filter((x) => x.taskId !== 'T_bill');
    const plan = b.plan as { day: Record<string, string>; order: string[] };
    plan.day = Object.fromEntries(Object.entries(plan.day).filter(([k]) => k !== 'T_bill'));
    plan.order = plan.order.filter((x) => x !== 'T_bill');
  });
  control.respond(IPC.adeTaskBoard, gone);
  await emitWailsEvent(page, IPC.adeTaskBoardChanged, null);
  await expect(page.locator(t('ade-card')).first()).toHaveAttribute('aria-current', 'true');
  await expect(page.locator(card('T_bill'))).toHaveCount(0);
  await expect(page.locator(t('ade-panel'))).toBeVisible();
});

test('an empty plan shows the empty panel', async ({ relaunch }) => {
  const empty = adeBoard((b) => {
    b.tasks = [];
    b.branches = [];
    b.pairs = [];
    b.history = [];
    b.plan = { day: {}, order: [], queuedAfter: {}, unpushed: {}, backlog: [] };
  });
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: empty },
  ]);
  await expect(page.locator(t('ade-panel'))).toBeVisible();
  const state = page.locator(t('ade-panel-empty'));
  await expect(state).toContainText('No tasks yet');
  await expect(state).toContainText('capture box');
});
