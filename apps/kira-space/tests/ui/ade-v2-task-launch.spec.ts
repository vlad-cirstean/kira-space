import type { Locator, Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { adeBoard, adeFixture, archivedTaskFx, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// The task plus on the Sessions tab bar: a terminal in a worktree, or an agent session with no first message.
// Contract ade-adhoc. Backend half: adeflow TestAdHocSession.

const t = (id: string) => `[data-testid="${id}"]`;
const item = (id: string) => t(`menu-item-ade-task-launch-${id}`);
const card = (id: string) => `[data-testid="ade-card"][data-task-id="${id}"]`;
const bar = (): string => `${t('ade-session-bar-row')} ${t('ade-task-launch')}`;

/** Opens the task panel's Sessions tab and returns its plus. */
async function plusOf(page: Page, id: string): Promise<Locator> {
  await page.locator(`${card(id)} ${t('ade-card-head')}`).click();
  await page.locator(t('ade-panel-tab-sessions')).click();
  return page.locator(bar());
}

/** Clicks the plus; the menu must sit inside the viewport so a mouse pick works. */
async function openMenu(page: Page, id: string): Promise<void> {
  await (await plusOf(page, id)).click();
  await expect(page.locator(t('context-menu'))).toBeInViewport();
}

const launch = {
  ...contract<Record<string, unknown>>('ade-adhoc', 'AdeTaskService.StartAdHoc'),
  ...adeFixture<Record<string, unknown>>('launch'),
};

test('one worktree: Terminal opens a shell there', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await openMenu(page, 'T_cart');
  await expect(page.locator(item('terminal-b_cart'))).not.toHaveAttribute('aria-describedby');
  await expect(page.locator(item('agent-b_cart'))).not.toHaveAttribute('aria-describedby');
  await page.locator(item('terminal-b_cart')).click();
  await expect.poll(() => calls(control, IPC.terminalOpen)).toHaveLength(1);
  expect(calls(control, IPC.terminalOpen)[0]?.args).toMatchObject({
    cwd: '~/kira-space-wts/web-app/cart-flaky',
    launchKind: 'shell',
  });
  await expect(
    page.locator('[data-testid="tab-strip-wrapper"] [data-testid="tab"][data-tab-kind="terminal"]'),
  ).toBeVisible();
  expect(calls(control, IPC.adeTaskStartAdHoc)).toHaveLength(0);
});

test('Agent session starts with no message and opens in the branch panel', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskStartAdHoc, response: launch },
  ]);
  await openMenu(page, 'T_cart');
  await page.locator(item('agent-b_cart')).click();
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
  await openMenu(page, 'T_auth');
  for (const id of ['b_auth', 'b_authui', 'b_authcb']) {
    await expect(page.locator(item(`terminal-${id}`))).toBeVisible();
    await expect(page.locator(item(`agent-${id}`))).toBeVisible();
  }
  await expect(page.locator(item('agent-b_auth'))).toContainText('Session open');
  await expect(page.locator(item('agent-b_auth'))).toHaveAttribute('data-disabled', '');
  await expect(page.locator(item('agent-b_authcb'))).toContainText('No worktree yet');
  await expect(page.locator(item('terminal-b_authcb'))).toContainText('No worktree yet');
  await expect(page.locator(item('terminal-b_auth'))).not.toHaveAttribute('data-disabled', '');
  await page.locator(item('agent-b_authui')).click();
  await expect.poll(() => calls(control, IPC.adeTaskStartAdHoc)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskStartAdHoc)[0]?.args).toEqual({
    branchId: 'b_authui',
    provider: '',
  });
});

test('a disabled item explains itself on hover, like the Runs menu', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openMenu(page, 'T_auth');
  await expect(page.locator(bar())).toHaveAttribute('aria-haspopup', 'menu');
  await page.locator(item('agent-b_auth')).hover();
  await expect(page.getByRole('tooltip')).toContainText('Session open');
});

test('setup states disable only the agent item', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openMenu(page, 'T_search');
  await expect(page.locator(item('agent-b_searchui'))).toContainText('Setup failed');
  await expect(page.locator(item('terminal-b_searchui'))).not.toHaveAttribute('data-disabled', '');
  await page.keyboard.press('Escape');
  await openMenu(page, 'T_push');
  await expect(page.locator(item('agent-b_push'))).toContainText('Preparing worktree');
});

test('a refusal lands in the panel action error', async ({ relaunch }) => {
  const refusal = JSON.parse(
    contract<string>('ade-adhoc', 'error:AdeTaskService.StartAdHoc#open'),
  ) as { code: string; message: string };
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskStartAdHoc, error: refusal },
  ]);
  await openMenu(page, 'T_cart');
  await page.locator(item('agent-b_cart')).click();
  await expect.poll(() => calls(control, IPC.adeTaskStartAdHoc)).toHaveLength(1);
  await expect(page.getByText('a session is already open on feat/api-adhoc')).toBeVisible();
  expect(calls(control, IPC.terminalOpen)).toHaveLength(0);
});

test('no card in the plan has a plus', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await expect(page.locator(card('T_cart'))).toBeVisible();
  await expect(page.locator(`${t('ade-card')} ${t('ade-task-launch')}`)).toHaveCount(0);
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
  const plus = await plusOf(page, 'T_cart');
  await expect(plus).toBeVisible();
  await plus.click();
  await page.locator(item('agent-b_cart')).click();
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
