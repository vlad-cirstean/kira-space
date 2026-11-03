import { expect, test } from './fixtures';
import { adeFixture, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// A stopped stage session is the step's default action: Resume. Go half: TestStageSessionResume.

const t = (id: string) => `[data-testid="${id}"]`;
const taskEl = (id: string) => `[data-testid="ade-task"][data-task-id="${id}"]`;
const head = (id: string) =>
  `[data-testid="ade-card"][data-task-id="${id}"] [data-testid="ade-card-head"]`;

interface SessionFx {
  id: string;
}

/** The fixture snapshot, optionally without the running T_auth session, plus stopped stage rows. */
function sessions(withRunning: boolean) {
  const fx = adeFixture<{ sessions: SessionFx[] }>('sessions');
  const stopped = contract<Record<string, unknown>>(
    'ade-stage-resume',
    'AdeTaskService.Sessions#stopped-step',
  );
  const row = (id: string, at: number, agentSessionId: string) => ({
    ...stopped,
    id,
    taskId: 'T_auth',
    branchId: 'b_auth',
    stepId: 'review',
    agentSessionId,
    startedAt: at,
    lastActiveAt: at,
  });
  return {
    ...fx,
    sessions: [
      ...fx.sessions.filter((s) => withRunning || s.id !== '9ab0'),
      row('rs01', 1_789_000_000_000, 'claude-rs01'),
      row('rs02', 1_789_500_000_000, 'claude-rs02'),
      row('rs00', 1_789_900_000_000, ''),
    ],
  };
}

const open = (relaunch: Parameters<typeof openPlan>[0], withRunning = false) =>
  openPlan(relaunch, [{ channel: IPC.adeTaskSessions, response: sessions(withRunning) }]);

test('contract: Resume is the default and takes over the newest session', async ({ relaunch }) => {
  const { window: page, control } = await open(relaunch);
  await expect(page.locator(`${taskEl('T_auth')} ${t('ade-task-action-resume')}`)).toBeVisible();
  await expect(page.locator(`${taskEl('T_auth')} ${t('ade-task-action-session')}`)).toHaveCount(0);
  await page.locator(`${taskEl('T_auth')} ${t('ade-task-action-resume')}`).click();
  await expect.poll(() => calls(control, IPC.adeTaskTakeOver)).toHaveLength(1);
  const sent = calls(control, IPC.adeTaskTakeOver)[0]?.args as Record<string, unknown>;
  const want = contract<Record<string, unknown>>(
    'ade-stage-resume',
    'args:AdeTaskService.TakeOver',
  );
  expect(Object.keys(sent).sort()).toEqual(Object.keys(want).sort());
  expect(sent).toMatchObject({ sessionId: 'rs02', stopIfRunning: true });
});

test('Start new sits in the task menu from right-click and 3-dots', async ({ relaunch }) => {
  const { window: page, control } = await open(relaunch);
  await page.locator(head('T_auth')).click({ button: 'right' });
  await expect(page.locator(t('menu-item-ade-task-action'))).toContainText('Resume Review');
  await page.locator(t('menu-item-ade-task-start-new')).click();
  await expect(page.locator(t('ade-dialog'))).toBeVisible();
  await page.locator(t('ade-dialog-send')).click();
  await expect.poll(() => calls(control, IPC.adeTaskLaunchStep)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskLaunchStep)[0]?.args).toEqual({ taskId: 'T_auth', message: '' });

  await page.locator(head('T_auth')).click();
  await expect(page.locator(t('ade-panel'))).toBeVisible();
  await page.locator(t('ade-panel-more')).click();
  await expect(page.locator(t('menu-item-ade-task-start-new'))).toBeVisible();
  await page.keyboard.press('Escape');
  await page.locator(t('ade-panel-title')).click({ button: 'right' });
  await expect(page.locator(t('menu-item-ade-task-start-new'))).toBeVisible();
});

test('Archive and Backlog live in the menu, not the header', async ({ relaunch }) => {
  const { window: page } = await open(relaunch);
  await page.locator(head('T_auth')).click();
  await expect(page.locator(t('ade-panel'))).toBeVisible();
  await expect(page.locator(t('ade-panel-archive'))).toHaveCount(0);
  await expect(page.locator(t('ade-panel-shelve'))).toHaveCount(0);
  await page.locator(t('ade-panel-more')).click();
  await expect(page.locator(t('menu-item-ade-task-archive'))).toBeVisible();
  await expect(page.locator(t('menu-item-ade-task-backlog'))).toBeVisible();
});

test("a running stage session keeps today's action", async ({ relaunch }) => {
  const { window: page } = await open(relaunch, true);
  await expect(page.locator(`${taskEl('T_auth')} ${t('ade-task-action-resume')}`)).toHaveCount(0);
  await page.locator(head('T_auth')).click({ button: 'right' });
  await expect(page.locator(t('context-menu'))).toBeVisible();
  await expect(page.locator(t('menu-item-ade-task-start-new'))).toHaveCount(0);
});
