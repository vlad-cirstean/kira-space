import { expect, test } from './fixtures';
import { type AdeBoardFx, adeBoard, adeFixture, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// A task with no repo: the step runs in the task folder. Contract ade-norepo.
// Backend half: adeflow TestRepolessWorkflow.

const t = (id: string) => `[data-testid="${id}"]`;
const task = (id: string) => `[data-testid="ade-task"][data-task-id="${id}"]`;
const cellAction = (id: string, kind: string) =>
  `${task(id)} ${t('ade-card-head')} ${t(`ade-task-action-${kind}`)}`;

const started = contract<{ runs: Record<string, unknown>[] }>(
  'ade-norepo',
  'AdeTaskService.Board#started',
);

/** T_alerts at `step` of the standard workflow with no branch of its own. */
function alertsWithoutRepo(step: string, runs: Record<string, unknown>[] = []) {
  return (b: AdeBoardFx): void => {
    const alerts = b.tasks.find((x) => x.id === 'T_alerts');
    if (!alerts) throw new Error('fixture tasks missing');
    alerts.entryStepId = step;
    alerts.workflowId = 'standard';
    alerts.branchIds = [];
    alerts.runs = runs;
    b.branches = b.branches.filter((x) => x.taskId !== 'T_alerts');
  };
}

const board = (edit: (b: AdeBoardFx) => void): ControlSnapshot => ({
  channel: IPC.adeTaskBoard,
  response: adeBoard(edit),
});

test('Run on a task with no repo shows the hint and sends no branch names', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [board(alertsWithoutRepo('plan'))]);
  await page.locator(cellAction('T_alerts', 'run')).click();
  const dialog = page.locator(t('ade-run-dialog'));
  await expect(dialog.locator(t('ade-run-no-repo'))).toContainText(
    'No repository on this task yet',
  );
  await expect(dialog.locator(t('ade-run-branch'))).toHaveCount(0);
  await dialog.locator(t('ade-run-send')).click();
  await expect.poll(() => calls(control, IPC.adeTaskStartRun)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskStartRun)[0]?.args).toEqual({
    taskId: 'T_alerts',
    branchNames: {},
    message: '',
  });
});

test('a run without a branch reads as the task folder', async ({ relaunch }) => {
  const run = {
    ...started.runs[0],
    taskId: 'T_alerts',
    stepId: 'plan',
    branchId: '',
    note: 'waiting for branch approval',
  };
  const { window: page } = await openPlan(relaunch, [board(alertsWithoutRepo('plan', [run]))]);
  await page
    .locator(`[data-testid="ade-card"][data-task-id="T_alerts"] [data-testid="ade-card-head"]`)
    .click();
  const line = page.locator(
    `${t('ade-step-block')}[data-step-id="plan"] [data-testid="ade-run-line"][data-branch-id=""]`,
  );
  await expect(line).toContainText('Task folder');
});

test('an outside-repo step on a task with repos shows the task folder', async ({ relaunch }) => {
  const run = {
    ...started.runs[0],
    taskId: 'T_alerts',
    stepId: 'plan',
    branchId: '',
    note: 'waiting for branch approval',
  };
  const { window: page } = await openPlan(relaunch, [
    board((b) => {
      const alerts = b.tasks.find((x) => x.id === 'T_alerts');
      if (!alerts) throw new Error('fixture tasks missing');
      alerts.entryStepId = 'plan';
      alerts.workflowId = 'standard';
      alerts.runs = [run];
    }),
  ]);
  await page
    .locator(`[data-testid="ade-card"][data-task-id="T_alerts"] [data-testid="ade-card-head"]`)
    .click();
  const line = page.locator(
    `${t('ade-step-block')}[data-step-id="plan"] [data-testid="ade-run-line"][data-branch-id=""]`,
  );
  await expect(line).toContainText('Task folder');
});

const sessions = adeFixture<{ sessions: { taskId: string; mode: string }[] }>('sessions');
// No open conversation on the task, or the stage offers Done instead of a session.
const noTalking = {
  ...sessions,
  sessions: sessions.sessions.filter((x) => !(x.taskId === 'T_alerts' && x.mode === 'tui')),
};

test('a manual session stage launches without a repo', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    board(alertsWithoutRepo('spec')),
    { channel: IPC.adeTaskSessions, response: noTalking },
    {
      channel: IPC.adeTaskLaunchStep,
      response: contract('ade-norepo', 'AdeTaskService.LaunchStep'),
    },
  ]);
  await page.locator(cellAction('T_alerts', 'session')).click();
  await expect(page.locator(t('ade-dialog-message'))).toHaveValue(/No repo yet/);
  await page.locator(t('ade-dialog-send')).click();
  await expect.poll(() => calls(control, IPC.adeTaskLaunchStep)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskLaunchStep)[0]?.args).toEqual({
    taskId: 'T_alerts',
    message: '',
  });
});

test('a step that needs a branch shows Needs a repository and no Run', async ({ relaunch }) => {
  const workflows = adeFixture<{
    workflows: { workflow: { id: string; steps: { id: string; runsOn: string }[] } }[];
  }>('workflows');
  const plan = workflows.workflows
    .find((w) => w.workflow.id === 'standard')
    ?.workflow.steps.find((s) => s.id === 'plan');
  if (!plan) throw new Error('fixture workflow missing');
  plan.runsOn = 'only api';
  const { window: page } = await openPlan(relaunch, [
    board(alertsWithoutRepo('plan')),
    { channel: IPC.adeTaskWorkflows, response: workflows },
  ]);
  await expect(page.locator(task('T_alerts'))).toBeVisible();
  await expect(page.locator(cellAction('T_alerts', 'run'))).toHaveCount(0);
  await page
    .locator(`[data-testid="ade-card"][data-task-id="T_alerts"] [data-testid="ade-card-head"]`)
    .click();
  await expect(page.locator(`${t('ade-step-block')}[data-step-id="plan"]`)).toContainText(
    'Needs a repository',
  );
});
