import type { Page } from '@playwright/test';
import { TIMING } from '@workbench/behaviour/timing';
import { expect, test } from './fixtures';
import { adeBoardSnapshot, adeFixture, alertsWithoutRepo, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// Starting work on a task with no repo asks for a repo first. Contract ade-repo-required.
// Backend half: adeflow TestTaskStartNeedsRepo.

const t = (id: string) => `[data-testid="${id}"]`;
const task = (id: string) => `[data-testid="ade-task"][data-task-id="${id}"]`;
const cellAction = (id: string, kind: string) =>
  `${task(id)} ${t('ade-card-head')} ${t(`ade-task-action-${kind}`)}`;

const SCENARIO = 'ade-repo-required';
const added = contract(SCENARIO, 'AdeTaskService.AddTaskRepo');
const noRepo = contract<{ code: string; message: string }>(
  SCENARIO,
  'AdeTaskService.StartRun#noRepo',
);
const waiting = contract<{ runs: Record<string, unknown>[] }>(
  SCENARIO,
  'AdeTaskService.Board#waitingRepo',
);

const addRepo: ControlSnapshot = { channel: IPC.adeTaskAddTaskRepo, response: added };

const sessions = adeFixture<{ sessions: { taskId: string; mode: string }[] }>('sessions');
// No open conversation on the task, or the stage offers Done instead of a session.
const noTalking = {
  ...sessions,
  sessions: sessions.sessions.filter((x) => !(x.taskId === 'T_alerts' && x.mode === 'tui')),
};

/** Picks `repo` in the open gate. */
async function choose(page: Page, repo = 'repo-api'): Promise<void> {
  await page.locator(t('ade-repo-gate-pick')).click();
  await page.locator(t(`ade-repo-gate-pick-option-${repo}`)).click();
  await expect(page.locator(t('ade-repo-gate-chip'))).toHaveCount(1);
}

test('Run on a task with no repo asks for a repo and Cancel starts nothing', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    adeBoardSnapshot(alertsWithoutRepo('plan')),
  ]);
  await page.locator(cellAction('T_alerts', 'run')).click();
  const gate = page.locator(t('ade-repo-gate'));
  await expect(gate).toBeVisible();
  await expect(gate.locator(t('ade-repo-gate-continue'))).toBeDisabled();
  await expect(page.locator(t('ade-run-dialog'))).toHaveCount(0);
  await gate.locator(t('ade-repo-gate-cancel')).click();
  await expect(gate).toBeHidden();
  await expect(page.locator(t('ade-run-dialog'))).toHaveCount(0);
  expect(calls(control, IPC.adeTaskStartRun)).toHaveLength(0);
  expect(calls(control, IPC.adeTaskAddTaskRepo)).toHaveLength(0);
});

test('choosing a repo adds it to the task, then the Run dialog opens', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    adeBoardSnapshot(alertsWithoutRepo('plan')),
    addRepo,
  ]);
  await page.locator(cellAction('T_alerts', 'run')).click();
  await choose(page);
  await page.locator(t('ade-repo-gate-continue')).click();
  await expect.poll(() => calls(control, IPC.adeTaskAddTaskRepo)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskAddTaskRepo)[0]?.args).toEqual({
    taskId: 'T_alerts',
    codeRepoId: 'repo-api',
    base: null,
  });
  await expect(page.locator(t('ade-repo-gate'))).toBeHidden();
  const dialog = page.locator(t('ade-run-dialog'));
  await expect(dialog).toBeVisible();
  await dialog.locator(t('ade-run-send')).click();
  await expect.poll(() => calls(control, IPC.adeTaskStartRun)).toHaveLength(1);
});

test('a refusal from a stale board asks for a repo and retries the start', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    adeBoardSnapshot((b) => {
      const alerts = b.tasks.find((x) => x.id === 'T_alerts');
      if (!alerts) throw new Error('fixture tasks missing');
      alerts.entryStepId = 'plan';
      alerts.workflowId = 'standard';
      alerts.runs = [];
    }),
    { channel: IPC.adeTaskStartRun, error: noRepo },
    addRepo,
  ]);
  await page.locator(cellAction('T_alerts', 'run')).click();
  await page.locator(t('ade-run-dialog')).locator(t('ade-run-send')).click();
  await expect(page.locator(t('ade-repo-gate'))).toBeVisible();
  expect(calls(control, IPC.adeTaskStartRun)).toHaveLength(1);
  await choose(page, 'repo-mobile');
  await page.locator(t('ade-repo-gate-continue')).click();
  await expect.poll(() => calls(control, IPC.adeTaskStartRun)).toHaveLength(2);
  expect(calls(control, IPC.adeTaskAddTaskRepo)).toHaveLength(1);
});

test('cancelling the repo prompt after a refusal shows no error toast and keeps the dialog input', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    adeBoardSnapshot(alertsWithoutRepo('plan')),
    { channel: IPC.adeTaskStartRun, error: noRepo },
    addRepo,
  ]);
  await page.clock.install();
  // The board says the task has no repo, so the gate asks first; pick, then the bridge still refuses.
  await page.locator(cellAction('T_alerts', 'run')).click();
  await choose(page);
  await page.locator(t('ade-repo-gate-continue')).click();
  const dialog = page.locator(t('ade-run-dialog'));
  await dialog.locator(t('ade-run-message')).fill('Plan it my way.');
  await dialog.locator(t('ade-run-send')).click();
  await expect(page.locator(t('ade-repo-gate'))).toBeVisible();
  // While the prompt waits no op runs: no chip, and no loading toast even past the slow-op mark.
  await page.clock.runFor(TIMING.slowOp + 500);
  await expect(page.locator('[data-testid="ade-op-status"]')).toHaveCount(0);
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(0);
  await page.locator(t('ade-repo-gate-cancel')).click();
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(0);
  expect(calls(control, IPC.adeTaskStartRun)).toHaveLength(1);
  await expect(dialog.locator(t('ade-run-message'))).toHaveValue('Plan it my way.');
});

test('a manual session step asks for a repo before the session dialog', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    adeBoardSnapshot(alertsWithoutRepo('spec')),
    { channel: IPC.adeTaskSessions, response: noTalking },
    addRepo,
    {
      channel: IPC.adeTaskLaunchStep,
      response: contract(SCENARIO, 'AdeTaskService.LaunchStep'),
    },
  ]);
  await page.locator(cellAction('T_alerts', 'session')).click();
  await expect(page.locator(t('ade-repo-gate'))).toBeVisible();
  await expect(page.locator(t('ade-dialog-send'))).toHaveCount(0);
  await choose(page);
  await page.locator(t('ade-repo-gate-continue')).click();
  await page.locator(t('ade-dialog-send')).click();
  await expect.poll(() => calls(control, IPC.adeTaskLaunchStep)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskLaunchStep)[0]?.args).toEqual({
    taskId: 'T_alerts',
    message: '',
  });
});

test('an outside-repo step runs without asking for a repo', async ({ relaunch }) => {
  const workflows = adeFixture<{
    workflows: { workflow: { id: string; steps: { id: string; outsideRepo: boolean }[] } }[];
  }>('workflows');
  const plan = workflows.workflows
    .find((w) => w.workflow?.id === 'standard')
    ?.workflow.steps.find((s) => s.id === 'plan');
  if (!plan) throw new Error('fixture workflow missing');
  plan.outsideRepo = true;
  const { window: page } = await openPlan(relaunch, [
    adeBoardSnapshot(alertsWithoutRepo('plan')),
    { channel: IPC.adeTaskWorkflows, response: workflows },
  ]);
  await page.locator(cellAction('T_alerts', 'run')).click();
  await expect(page.locator(t('ade-run-dialog'))).toBeVisible();
  await expect(page.locator(t('ade-repo-gate'))).toHaveCount(0);
});

test('a step that waits for a repo reads Choose a repository and offers Run', async ({
  relaunch,
}) => {
  const planRun = { ...waiting.runs[0], taskId: 'T_alerts', stepId: 'plan', branchId: '' };
  const { window: page } = await openPlan(relaunch, [
    adeBoardSnapshot(alertsWithoutRepo('plan', [planRun])),
  ]);
  await expect(page.locator(cellAction('T_alerts', 'run'))).toBeVisible();
  await page
    .locator(`[data-testid="ade-card"][data-task-id="T_alerts"] [data-testid="ade-card-head"]`)
    .click();
  await expect(page.locator(`${t('ade-step-block')}[data-step-id="impl"]`)).toContainText(
    'Choose a repository to start',
  );
});
