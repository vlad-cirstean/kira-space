import { expect, test } from './fixtures';
import { openPlan } from './support/adeV2';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// The task and branch panels reopen on the tab they were left on, with the same picked session.

const t = (id: string) => `[data-testid="${id}"]`;
const head = (id: string) =>
  `[data-testid="ade-card"][data-task-id="${id}"] [data-testid="ade-card-head"]`;
const row = (id: string) => `[data-testid="ade-branch-row"][data-branch-id="${id}"]`;

test('each task reopens on the tab it was left on', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await page.locator(head('T_bill')).click();
  await page.locator(t('ade-panel-tab-sessions')).click();
  await page.locator(t('ade-session-tab-b1c2')).click();
  await expect(page.locator(t('ade-session-tab-b1c2'))).toHaveAttribute('data-selected', 'true');
  const boards = calls(control, IPC.adeTaskBoard).length;
  const sessions = calls(control, IPC.adeTaskSessions).length;

  await page.locator(head('T_notif')).click();
  await expect(page.locator(t('ade-task-tab'))).toBeVisible();
  await page.locator(t('ade-panel-tab-notes')).click();
  await expect(page.locator(t('ade-notes-tab'))).toBeVisible();

  await page.locator(head('T_bill')).click();
  await expect(page.locator(t('ade-sessions-tab'))).toBeVisible();
  await expect(page.locator(t('ade-session-tab-b1c2'))).toHaveAttribute('data-selected', 'true');

  await page.locator(head('T_notif')).click();
  await expect(page.locator(t('ade-notes-tab'))).toBeVisible();

  expect(calls(control, IPC.adeTaskBoard)).toHaveLength(boards);
  expect(calls(control, IPC.adeTaskSessions)).toHaveLength(sessions);
});

test('each branch reopens on the tab it was left on', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator(row('b_billdash')).click();
  await page.locator(t('ade-panel-tab-sessions')).click();
  await expect(page.locator(t('ade-sessions-tab'))).toBeVisible();

  await page.locator(row('b_search')).click();
  await expect(page.locator(t('ade-worktree-setup'))).toBeVisible();
  await expect(page.locator(t('ade-sessions-tab'))).toHaveCount(0);

  await page.locator(row('b_billdash')).click();
  await expect(page.locator(t('ade-sessions-tab'))).toBeVisible();

  await page.locator(t('ade-panel-back')).click();
  await expect(page.locator(t('ade-task-tab'))).toBeVisible();
});

test('tab counts show in a grey circle badge', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  const badge = (id: string) => page.locator(`${t(id)} [data-slot="badge"][data-variant="count"]`);
  await page.locator(head('T_bill')).click();
  const sessions = badge('ade-panel-tab-sessions');
  await expect(sessions).toHaveText('4');
  const box = await sessions.boundingBox();
  expect(Math.abs((box?.width ?? 0) - (box?.height ?? 1))).toBeLessThanOrEqual(1);

  await page.locator(t('ade-tab-needs')).click();
  await page.locator(t('ade-needs-all')).click();
  await expect(badge('ade-all-running')).toHaveText('13');
  await expect(badge('ade-all-stopped')).toHaveText('7');
});
