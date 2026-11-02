import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { adeBoard, adeFixture, archivedTaskFx, openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// The task plus: a terminal in a worktree, or an agent session with no first message.
// Contract ade-adhoc. Backend half: adeflow TestAdHocSession.

const t = (id: string) => `[data-testid="${id}"]`;
const card = (id: string) => `[data-testid="ade-card"][data-task-id="${id}"]`;
const plus = (id: string) => `${card(id)} ${t('ade-task-launch')}`;

/** The menu floats off-screen in this runtime, so the item is picked with the keyboard. */
async function choose(page: Page, id: string): Promise<void> {
  await page.locator(t(id)).focus();
  await page.keyboard.press('Enter');
}

const launch = {
  ...contract<Record<string, unknown>>('ade-adhoc', 'AdeTaskService.StartAdHoc'),
  ...adeFixture<Record<string, unknown>>('launch'),
};

test('one worktree: Terminal opens a shell there', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await page.locator(plus('T_cart')).click();
  await expect(page.locator(t('ade-task-launch-menu'))).toBeVisible();
  await expect(page.locator(t('ade-task-launch-reason'))).toHaveCount(0);
  await choose(page, 'ade-task-launch-terminal-b_cart');
  await expect.poll(() => calls(control, IPC.terminalOpen)).toHaveLength(1);
  expect(calls(control, IPC.terminalOpen)[0]?.args).toMatchObject({
    cwd: '~/wt/web-app/cart-flaky',
    launchKind: 'shell',
  });
  expect(calls(control, IPC.adeTaskStartAdHoc)).toHaveLength(0);
});

test('Agent session starts with no message and opens in the branch panel', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskStartAdHoc, response: launch },
  ]);
  await page.locator(plus('T_cart')).click();
  await choose(page, 'ade-task-launch-agent-b_cart');
  await expect.poll(() => calls(control, IPC.adeTaskStartAdHoc)).toHaveLength(1);
  const sent = calls(control, IPC.adeTaskStartAdHoc)[0]?.args as Record<string, unknown>;
  expect(sent).toEqual({ branchId: 'b_cart', provider: '' });
  expect(Object.keys(sent)).toEqual(
    Object.keys(contract<Record<string, unknown>>('ade-adhoc', 'args:AdeTaskService.StartAdHoc')),
  );
  await expect.poll(() => calls(control, IPC.terminalOpen)).toHaveLength(1);
  expect(calls(control, IPC.terminalOpen)[0]?.args).toMatchObject({
    launchKind: 'agent',
    command: launch.command,
  });
  await expect(page.locator(t('ade-sessions-tab'))).toBeVisible();
});

test('several worktrees: grouped items, one per branch, and a busy one says why', async ({
  relaunch,
}) => {
  const board = adeBoard((b) => {
    const br = b.branches.find((x) => x.id === 'b_authcb');
    if (br) br.worktree = '';
  });
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  await page.locator(plus('T_auth')).click();
  for (const id of ['b_auth', 'b_authui', 'b_authcb']) {
    await expect(page.locator(t(`ade-task-launch-terminal-${id}`))).toBeVisible();
    await expect(page.locator(t(`ade-task-launch-agent-${id}`))).toBeVisible();
  }
  await expect(page.locator(t('ade-task-launch-agent-b_auth'))).toContainText('Session open');
  await expect(page.locator(t('ade-task-launch-agent-b_auth'))).toHaveAttribute(
    'data-disabled',
    '',
  );
  await expect(page.locator(t('ade-task-launch-agent-b_authcb'))).toContainText('No worktree yet');
  await expect(page.locator(t('ade-task-launch-terminal-b_authcb'))).toContainText(
    'No worktree yet',
  );
  await expect(page.locator(t('ade-task-launch-terminal-b_auth'))).not.toHaveAttribute(
    'data-disabled',
    '',
  );
  await choose(page, 'ade-task-launch-agent-b_authui');
  await expect.poll(() => calls(control, IPC.adeTaskStartAdHoc)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskStartAdHoc)[0]?.args).toEqual({
    branchId: 'b_authui',
    provider: '',
  });
});

test('setup states disable only the agent item', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator(plus('T_search')).click();
  await expect(page.locator(t('ade-task-launch-agent-b_searchui'))).toContainText('Setup failed');
  await expect(page.locator(t('ade-task-launch-terminal-b_searchui'))).not.toHaveAttribute(
    'data-disabled',
    '',
  );
  await page.keyboard.press('Escape');
  await page.locator(plus('T_push')).click();
  await expect(page.locator(t('ade-task-launch-agent-b_push'))).toContainText('Preparing worktree');
});

test('a refusal lands in the panel action error', async ({ relaunch }) => {
  const refusal = JSON.parse(
    contract<string>('ade-adhoc', 'error:AdeTaskService.StartAdHoc#open'),
  ) as { code: string; message: string };
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskStartAdHoc, error: refusal },
  ]);
  await page.locator(plus('T_cart')).click();
  await choose(page, 'ade-task-launch-agent-b_cart');
  await expect.poll(() => calls(control, IPC.adeTaskStartAdHoc)).toHaveLength(1);
  await expect(page.getByText('a session is already open on feat/api-adhoc')).toBeVisible();
  expect(calls(control, IPC.terminalOpen)).toHaveLength(0);
});

test('a task without a branch name has no plus on its card', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await expect(page.locator(card('T_cart'))).toBeVisible();
  await expect(page.locator(plus('T_alerts'))).toHaveCount(0);
});

test('the Sessions tab bar has a plus even with no session, scoped to its branch', async ({
  relaunch,
}) => {
  const fx = adeFixture<{ sessions: { taskId: string }[] }>('sessions');
  const { window: page, control } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskSessions,
      response: { ...fx, sessions: fx.sessions.filter((s) => s.taskId !== 'T_cart') },
    },
  ]);
  await openTask(page, 'T_cart');
  await page.locator(t('ade-panel-tab-sessions')).click();
  const bar = page.locator(`${t('ade-session-bar-row')} ${t('ade-task-launch')}`);
  await expect(bar).toBeVisible();
  await bar.click();
  await choose(page, 'ade-task-launch-agent-b_cart');
  await expect.poll(() => calls(control, IPC.adeTaskStartAdHoc)).toHaveLength(1);
});

test('an archived task panel has no plus', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskArchivedTask, response: archivedTaskFx() },
  ]);
  await page.locator(t('ade-archived-find')).click();
  await page.locator(`${t('ade-archived-find-item')}[data-task-id="H_1"]`).click();
  await page.locator(t('ade-panel-tab-sessions')).click();
  await expect(page.locator(t('ade-sessions-tab'))).toBeVisible();
  await expect(page.locator(`${t('ade-sessions-tab')} ${t('ade-task-launch')}`)).toHaveCount(0);
});
