import { expect, test } from './fixtures';
import { adeFixture, openPlan, openTask } from './support/adeV2';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// A burst of board pushes costs one refetch behind the running one, never a restart per push. A push
// names the tasks it touches and whether refs moved, and the refetches follow. Backend half: adeflow
// board-changed payload and coalescing tests (P622).

const forTask = (control: Parameters<typeof calls>[0], taskId: string) =>
  calls(control, IPC.adeTaskPanelForward).filter(
    (c) => (c.args as { taskId?: string } | undefined)?.taskId === taskId,
  );

test('five pushes during one slow board fetch give two fetches and the final board', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, hold: true, response: adeFixture('board') },
  ]);
  control.release(IPC.adeTaskBoard);
  await expect(page.locator('[data-testid="ade-card"]').first()).toBeVisible();
  const before = calls(control, IPC.adeTaskBoard).length;
  for (let i = 0; i < 5; i++) await emitWailsEvent(page, IPC.adeTaskBoardChanged, null);
  await expect.poll(() => calls(control, IPC.adeTaskBoard).length).toBe(before + 1);
  control.release(IPC.adeTaskBoard);
  await expect.poll(() => calls(control, IPC.adeTaskBoard).length).toBe(before + 2);
  control.release(IPC.adeTaskBoard);
  await expect(page.locator('[data-testid="ade-card"]').first()).toBeVisible();
  await page.waitForTimeout(300);
  expect(calls(control, IPC.adeTaskBoard)).toHaveLength(before + 2);
});

test('a push naming other tasks leaves this panel and the change requests alone', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch);
  await openTask(page, 'T_alerts');
  await expect.poll(() => forTask(control, 'T_alerts').length).toBeGreaterThan(0);
  const panel = forTask(control, 'T_alerts').length;
  const board = calls(control, IPC.adeTaskBoard).length;
  const requests = calls(control, IPC.adeTaskChangeRequests).length;
  await emitWailsEvent(page, IPC.adeTaskBoardChanged, { refs: false, taskIds: ['T_cart'] });
  await expect.poll(() => calls(control, IPC.adeTaskBoard).length).toBe(board + 1);
  await page.waitForTimeout(300);
  expect(forTask(control, 'T_alerts')).toHaveLength(panel);
  expect(calls(control, IPC.adeTaskChangeRequests)).toHaveLength(requests);
  await emitWailsEvent(page, IPC.adeTaskBoardChanged, { refs: true, taskIds: ['T_alerts'] });
  await expect.poll(() => forTask(control, 'T_alerts').length).toBe(panel + 1);
  await expect.poll(() => calls(control, IPC.adeTaskChangeRequests).length).toBe(requests + 1);
});
