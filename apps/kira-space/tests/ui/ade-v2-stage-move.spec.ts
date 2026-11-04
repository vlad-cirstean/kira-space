import { expect, test } from './fixtures';
import { adeBoard, adeFixture, openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// Stage move: Next stage / Finish, and the close-sessions choice in the stage dialog. Contract
// ade-stage-move; backend half: flows/adeflow TestStageMoveSessions (stagemove_test.go).

const t = (id: string) => `[data-testid="${id}"]`;
const task = (id: string) => `[data-testid="ade-task"][data-task-id="${id}"]`;
const cellAction = (id: string, kind: string) =>
  `${task(id)} ${t('ade-card-head')} ${t(`ade-task-action-${kind}`)}`;

interface SessionFx extends Record<string, unknown> {
  id: string;
  taskId: string;
  mode: string;
  state: string;
  stepId: string;
}

const recorded = contract<{ sessions: SessionFx[] }>(
  'ade-stage-move',
  'AdeTaskService.Sessions#stage-open',
).sessions;

/** A recorded running terminal session placed on `taskId` and `stepId` ('' = a branch session). */
function session(id: string, taskId: string, stepId: string): SessionFx {
  const base = recorded[0];
  if (!base) throw new Error('contract has no sessions');
  return { ...base, id, taskId, stepId };
}

/** The fixture sessions with `taskId`'s terminal sessions replaced by `own`. */
function sessionsOf(taskId: string, own: SessionFx[]): ControlSnapshot {
  const fx = adeFixture<{ sessions: SessionFx[] }>('sessions');
  const rest = fx.sessions.filter((s) => !(s.taskId === taskId && s.mode === 'tui'));
  return { channel: IPC.adeTaskSessions, response: { ...fx, sessions: [...rest, ...own] } };
}

const board = (taskId: string, stepId: string): ControlSnapshot => ({
  channel: IPC.adeTaskBoard,
  response: adeBoard((b) => {
    const row = b.tasks.find((x) => x.id === taskId);
    if (!row) throw new Error(`fixture task ${taskId} missing`);
    row.entryStepId = stepId;
    row.workflowId = 'standard';
  }),
});

/** The standard workflow without its script tail, so Review is the last stage. */
function reviewLast(): ControlSnapshot {
  const fx = adeFixture<{
    workflows: { workflow: { id: string; steps: { id: string; skip: boolean }[] } | null }[];
  }>('workflows');
  const std = fx.workflows.find((w) => w.workflow?.id === 'standard')?.workflow;
  const release = std?.steps.find((s) => s.id === 'release');
  if (!release) throw new Error('fixture workflow missing');
  release.skip = true;
  return { channel: IPC.adeTaskWorkflows, response: fx };
}

const stageOne = [session('st1', 'T_alerts', 'spec'), session('st2', 'T_alerts', 'review')];

test('another stage’s open session does not hide Start', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    board('T_auth', 'review'),
    sessionsOf('T_auth', [session('st1', 'T_auth', 'spec')]),
  ]);
  await expect(page.locator(cellAction('T_auth', 'session'))).toBeVisible();
  await expect(page.locator(cellAction('T_auth', 'advance'))).toHaveCount(0);
  await expect(page.locator(cellAction('T_auth', 'finish'))).toHaveCount(0);
});

test('Next stage asks to close the open sessions; Close is the default', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    board('T_alerts', 'spec'),
    sessionsOf('T_alerts', stageOne),
  ]);
  const action = page.locator(cellAction('T_alerts', 'advance'));
  await expect(action).toHaveAttribute('aria-label', 'Next stage ›');
  await action.click();
  await expect(page.locator(t('ade-stage-dialog'))).toBeVisible();
  await expect(page.locator(t('ade-stage-sessions'))).toContainText('2 sessions still open');
  await expect(page.locator(t('ade-stage-close'))).toBeChecked();
  await page.locator(t('ade-stage-move')).click();
  await expect.poll(() => calls(control, IPC.adeTaskStepDone)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskStepDone)[0]?.args).toEqual({
    taskId: 'T_alerts',
    closeSessions: true,
  });
});

test('Keep open moves without closing', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    board('T_alerts', 'spec'),
    sessionsOf('T_alerts', stageOne),
  ]);
  await page.locator(cellAction('T_alerts', 'advance')).click();
  await page.locator(t('ade-stage-keep')).click();
  await page.locator(t('ade-stage-move')).click();
  await expect.poll(() => calls(control, IPC.adeTaskStepDone)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskStepDone)[0]?.args).toEqual({ taskId: 'T_alerts' });
});

test('Finish on the last stage closes its session too', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    board('T_auth', 'review'),
    reviewLast(),
    sessionsOf('T_auth', [session('st1', 'T_auth', 'review')]),
  ]);
  const action = page.locator(cellAction('T_auth', 'finish'));
  await expect(action).toHaveAttribute('aria-label', 'Finish');
  await action.click();
  await expect(page.locator(t('ade-stage-target'))).toHaveValue('done');
  await expect(page.locator(t('ade-stage-sessions'))).toContainText('1 session still open');
  await page.locator(t('ade-stage-move')).click();
  await expect.poll(() => calls(control, IPC.adeTaskStepDone)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskStepDone)[0]?.args).toEqual({
    taskId: 'T_auth',
    closeSessions: true,
  });
});

test('a branch session alone is not offered for closing', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    board('T_alerts', 'spec'),
    sessionsOf('T_alerts', [session('br1', 'T_alerts', '')]),
  ]);
  await page.locator(cellAction('T_alerts', 'advance')).click();
  await expect(page.locator(t('ade-stage-dialog'))).toBeVisible();
  await expect(page.locator(t('ade-stage-sessions'))).toHaveCount(0);
  await page.locator(t('ade-stage-move')).click();
  await expect.poll(() => calls(control, IPC.adeTaskStepDone)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskStepDone)[0]?.args).toEqual({ taskId: 'T_alerts' });
});

test('the step mover sends closeSessions when a stage session is open', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    board('T_auth', 'review'),
    sessionsOf('T_auth', [session('st1', 'T_auth', 'review')]),
  ]);
  await openTask(page, 'T_auth');
  await page.locator(t('ade-step-back')).click();
  await expect(page.locator(t('ade-stage-sessions'))).toBeVisible();
  await page.locator(t('ade-stage-move')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSetTaskStep)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskSetTaskStep)[0]?.args).toMatchObject({
    taskId: 'T_auth',
    closeSessions: true,
  });
});
