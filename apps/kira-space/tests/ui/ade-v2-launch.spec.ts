import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { adeBoard, adeFixture, openPlan } from './support/adeV2';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// Step and start launches, the task's Take over and the `!` targets.

const t = (id: string) => `[data-testid="${id}"]`;
const row = (id: string) => `[data-testid="ade-branch-row"][data-branch-id="${id}"]`;
const taskEl = (id: string) => `[data-testid="ade-task"][data-task-id="${id}"]`;

interface SessionFx {
  id: string;
  branchId: string;
}

/** The sessions snapshot without the given session ids and without any session of `branches`. */
function sessionsWithout(ids: string[], branches: string[] = []) {
  const fx = adeFixture<{ sessions: SessionFx[] }>('sessions');
  return {
    ...fx,
    sessions: fx.sessions.filter((s) => !ids.includes(s.id) && !branches.includes(s.branchId)),
  };
}

async function editedSend(page: Page, text: string): Promise<void> {
  await page.locator(t('ade-dialog-message')).fill(text);
  await page.locator(t('ade-dialog-send')).click();
}

test('▶ <Step> opens the dialog and an unedited message goes as an empty string', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskSessions, response: sessionsWithout(['9ab0']) },
  ]);
  await page.locator(`${taskEl('T_auth')} ${t('ade-task-action-session')}`).click();
  await expect(page.locator(t('ade-dialog'))).toBeVisible();
  await expect(page.locator(t('ade-dialog-message'))).not.toHaveValue('');
  await page.locator(t('ade-dialog-send')).click();
  await expect.poll(() => calls(control, IPC.adeTaskLaunchStep)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskLaunchStep)[0]?.args).toEqual({
    taskId: 'T_auth',
    message: '',
  });
  await expect.poll(() => calls(control, IPC.terminalOpen)).toHaveLength(1);
  await expect(page.locator(t('ade-dialog'))).toBeHidden();
});

test('▶ <Step> sends an edited message as written', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskSessions, response: sessionsWithout(['9ab0']) },
  ]);
  await page.locator(`${taskEl('T_auth')} ${t('ade-task-action-session')}`).click();
  await editedSend(page, 'Review only the auth module.');
  await expect.poll(() => calls(control, IPC.adeTaskLaunchStep)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskLaunchStep)[0]?.args).toEqual({
    taskId: 'T_auth',
    message: 'Review only the auth module.',
  });
});

test('▶ Start launches the branch with the default and with an edited message', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskSessions, response: sessionsWithout([], ['b_authui']) },
  ]);
  await page.locator(row('b_authui')).click();
  await page.locator(t('ade-panel-action-start')).click();
  await page.locator(t('ade-dialog-send')).click();
  await expect.poll(() => calls(control, IPC.adeTaskStartBranch)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskStartBranch)[0]?.args).toEqual({
    branchId: 'b_authui',
    message: '',
  });
  await page.locator(t('ade-panel-action-start')).click();
  await editedSend(page, 'Start with the tests.');
  await expect.poll(() => calls(control, IPC.adeTaskStartBranch)).toHaveLength(2);
  expect(calls(control, IPC.adeTaskStartBranch)[1]?.args).toEqual({
    branchId: 'b_authui',
    message: 'Start with the tests.',
  });
});

function stuckBoard() {
  return adeBoard((b) => {
    for (const task of b.tasks) {
      for (const run of task.runs)
        if (String(run.id).startsWith('r_T_bill_impl_')) run.state = 'stuck';
    }
  });
}

test('a stuck step puts Take over on the task, taken over directly', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: stuckBoard() },
  ]);
  await page.locator(`${taskEl('T_bill')} ${t('ade-task-action-takeOver')}`).click();
  await expect.poll(() => calls(control, IPC.adeTaskTakeOver)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskTakeOver)[0]?.args).toMatchObject({ stopIfRunning: true });
});

test('a double click on Take over launches once', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: stuckBoard() },
  ]);
  await page.locator(`${taskEl('T_bill')} ${t('ade-task-action-takeOver')}`).dblclick();
  await expect.poll(() => calls(control, IPC.adeTaskTakeOver)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskTakeOver)).toHaveLength(1);
});

test('the ! circle on a stuck branch takes the run over', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: stuckBoard() },
  ]);
  await page.locator(`${row('b_bill')} ${t('ade-attention')}`).click();
  await expect.poll(() => calls(control, IPC.adeTaskTakeOver)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskTakeOver)[0]?.args).toMatchObject({ sessionId: 'b1c2' });
});

const setupOf = (state: string, note = '') => ({
  state,
  startedAt: 1_790_069_900_000,
  finishedAt: state === 'running' ? null : 1_790_069_990_000,
  exitCode: state === 'running' ? null : 1,
  note,
});

function authBoard(state: string, note = '') {
  return adeBoard((b) => {
    for (const br of b.branches as unknown as {
      taskId: string;
      kind: string;
      name: string;
      setup: unknown;
    }[]) {
      if (br.taskId === 'T_auth' && br.kind === 'mine' && br.name !== '')
        br.setup = setupOf(state, note);
    }
  });
}

test('a launch behind a running prepare script shows its progress and waits', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: authBoard('running') },
    { channel: IPC.adeTaskSessions, response: sessionsWithout(['9ab0']) },
    {
      channel: IPC.adeTaskLaunchStep,
      error: {
        code: 'E_PREPARING',
        message: 'preparing the worktree of feat/oauth-login; try again when it is ready',
      },
    },
  ]);
  await page.locator(`${taskEl('T_auth')} ${t('ade-task-action-session')}`).click();
  await expect(page.locator(t('ade-dialog-setup')).first()).toContainText('Preparing worktree');
  await page.locator(t('ade-dialog-send')).click();
  await expect(page.locator(t('ade-dialog-send'))).toHaveText('Starts when ready…');
  await expect(page.locator(t('ade-dialog-send'))).toBeDisabled();
  await expect(page.locator(t('ade-dialog-error'))).toHaveCount(0);
  await expect.poll(() => calls(control, IPC.adeTaskLaunchStep).length).toBeGreaterThan(1);
  await page.locator(t('ade-dialog-close')).click();
  await expect(page.locator(t('ade-dialog'))).toBeHidden();
  const sent = calls(control, IPC.adeTaskLaunchStep).length;
  // Absence of further launch polls: spans more than one poll interval of real time.
  await page.waitForTimeout(2600);
  expect(calls(control, IPC.adeTaskLaunchStep)).toHaveLength(sent);
});

test('a launch behind a failed prepare script shows why and Retry setup', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: authBoard('failed', 'exited with status 1') },
    { channel: IPC.adeTaskSessions, response: sessionsWithout(['9ab0']) },
  ]);
  await page.locator(`${taskEl('T_auth')} ${t('ade-task-action-session')}`).click();
  const block = page.locator(t('ade-dialog-setup')).first();
  await expect(block).toContainText('Worktree setup failed');
  await expect(block.locator(t('script-progress-reason'))).toHaveText('exited with status 1');
  await block.locator(t('ade-setup-retry')).click();
  await expect.poll(() => calls(control, IPC.adeTaskRetrySetup)).toHaveLength(1);
});
