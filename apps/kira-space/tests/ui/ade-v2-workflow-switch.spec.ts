import { expect, test } from './fixtures';
import { adeFixture, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// OQ-ADE-4: a workflow switch that would delete runs and logs asks first, naming the counts.
// Backend half: flows/adeflow TestWorkflowSwitchKeepsOtherTasksRuns writes the refusal contract.

const t = (id: string) => `[data-testid="${id}"]`;

const refusal = () =>
  JSON.parse(
    contract<string>('ade-workflow-switch', 'error:AdeTaskService.SetTaskWorkflow#loses-work'),
  ) as { code: string; message: string };

async function pickBugfix(relaunch: Parameters<typeof openPlan>[0]) {
  const first = { taskId: 'T_bill', workflowId: 'bugfix' };
  const opened = await openPlan(relaunch, [
    { channel: IPC.adeTaskSetTaskWorkflow, args: first, error: refusal() },
    {
      channel: IPC.adeTaskSetTaskWorkflow,
      args: { ...first, confirmed: true },
      response: adeFixture('task'),
    },
  ]);
  await opened.window
    .locator('[data-testid="ade-card"][data-task-id="T_bill"] [data-testid="ade-card-head"]')
    .click();
  await opened.window.locator(t('ade-workflow-select')).selectOption('bugfix');
  return opened;
}

test('a switch that deletes runs names the counts and sends confirmed on Yes', async ({
  relaunch,
}) => {
  const { window: page, control } = await pickBugfix(relaunch);
  await expect(page.locator(t('confirm-dialog-message'))).toContainText('runs (1)');
  await expect(page.locator(t('confirm-dialog-message'))).toContainText('run logs (1)');
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSetTaskWorkflow)).toHaveLength(2);
  const sent = calls(control, IPC.adeTaskSetTaskWorkflow).map((c) => c.args);
  expect(sent[1]).toEqual({ taskId: 'T_bill', workflowId: 'bugfix', confirmed: true });
});

test('declining the confirm sends no second switch', async ({ relaunch }) => {
  const { window: page, control } = await pickBugfix(relaunch);
  await page.locator(t('confirm-dialog-cancel')).click();
  await expect(page.locator(t('confirm-dialog-root'))).toHaveCount(0);
  expect(calls(control, IPC.adeTaskSetTaskWorkflow)).toHaveLength(1);
});
