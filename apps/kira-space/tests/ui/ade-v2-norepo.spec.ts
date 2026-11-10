import { expect, test } from './fixtures';
import { adeBoardSnapshot, adeFixture, alertsWithoutRepo, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// A task with no repo: the step runs in the task folder. Contract ade-norepo.
// Backend half: adeflow TestRepolessWorkflow. P598: only outside-repo steps run without a repo.

const t = (id: string) => `[data-testid="${id}"]`;
const task = (id: string) => `[data-testid="ade-task"][data-task-id="${id}"]`;
const cellAction = (id: string, kind: string) =>
  `${task(id)} ${t('ade-card-head')} ${t(`ade-task-action-${kind}`)}`;

const started = contract<{ runs: Record<string, unknown>[] }>(
  'ade-norepo',
  'AdeTaskService.Board#started',
);

test('a run without a branch reads as the task folder', async ({ relaunch }) => {
  const run = {
    ...started.runs[0],
    taskId: 'T_alerts',
    stepId: 'plan',
    branchId: '',
    note: 'waiting for branch approval',
  };
  const { window: page } = await openPlan(relaunch, [
    adeBoardSnapshot(alertsWithoutRepo('plan', [run])),
  ]);
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
    adeBoardSnapshot((b) => {
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

test('an outside-repo session stage launches without a repo', async ({ relaunch }) => {
  const workflows = adeFixture<{
    workflows: { workflow: { id: string; steps: { id: string; outsideRepo: boolean }[] } }[];
  }>('workflows');
  const spec = workflows.workflows
    .find((w) => w.workflow?.id === 'standard')
    ?.workflow.steps.find((s) => s.id === 'spec');
  if (!spec) throw new Error('fixture workflow missing');
  spec.outsideRepo = true;
  const { window: page, control } = await openPlan(relaunch, [
    adeBoardSnapshot(alertsWithoutRepo('spec')),
    { channel: IPC.adeTaskWorkflows, response: workflows },
    { channel: IPC.adeTaskSessions, response: noTalking },
    {
      channel: IPC.adeTaskLaunchStep,
      response: contract('ade-norepo', 'AdeTaskService.LaunchStep'),
    },
  ]);
  await page.locator(cellAction('T_alerts', 'session')).click();
  await expect(page.locator(t('ade-repo-gate'))).toHaveCount(0);
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
    adeBoardSnapshot(alertsWithoutRepo('plan')),
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
