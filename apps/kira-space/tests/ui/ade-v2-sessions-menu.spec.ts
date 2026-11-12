import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { emitOpenSession, openPlan, openSessions } from './support/adeV2';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// The Sessions dropdown of a task: every session in scope, Running then Finished or stopped, with
// a filter from 6 rows, keyboard, a row menu and a reveal. UI half of P437: no backend change.

const t = (id: string) => `[data-testid="${id}"]`;
const rowOf = (id: string) => `${t('ade-sessions-row')}[data-session-id="${id}"]`;

async function rowIds(page: Page): Promise<(string | null)[]> {
  return page
    .locator(`${t('ade-sessions-menu-popover')} ${t('ade-sessions-row')}`)
    .evaluateAll((els) => els.map((e) => e.getAttribute('data-session-id')));
}

test('the trigger counts every session; the popover groups Running, then Stopped', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  await openSessions(page, 'T_bill');
  await expect(page.locator(t('ade-sessions-menu-count'))).toHaveText('6');
  await page.locator(t('ade-sessions-menu-trigger')).click();
  await expect(page.locator(t('ade-sessions-menu-popover'))).toBeVisible();
  expect(await rowIds(page)).toEqual(['tk01', 'b1c2', 'm111', '9d10', 'rv01', 'sp11']);
  await expect(page.locator('[data-slot="command-group-heading"]')).toHaveText([
    'Running 4',
    'Stopped 2',
  ]);
});

test('no session in scope, no dropdown', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskSessions, response: { sessions: [] } },
  ]);
  await openSessions(page, 'T_bill');
  await expect(page.locator(t('ade-sessions-menu-trigger'))).toHaveCount(0);
  await expect(page.locator(t('ade-sessions-empty'))).toBeVisible();
});

test('the dropdown works from the keyboard and Take over runs on Enter', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await openSessions(page, 'T_bill');
  await page.locator(t('ade-sessions-menu-trigger')).focus();
  await page.keyboard.press('Enter');
  await expect(page.locator(t('ade-sessions-menu-popover'))).toBeVisible();
  await expect(page.locator(t('ade-sessions-menu-filter'))).toBeFocused();
  await page.keyboard.press('ArrowDown');
  await expect(page.locator(`${t('ade-sessions-row')}[data-highlighted]`)).toHaveCount(1);
  await page.keyboard.press('End');
  await expect(page.locator(rowOf('sp11'))).toHaveAttribute('data-highlighted', '');
  await expect(page.locator(`${rowOf('sp11')} ${t('ade-sessions-row-hint')}`)).toHaveText(
    'Take over',
  );
  await page.keyboard.press('Enter');
  await expect(page.locator(t('ade-sessions-menu-popover'))).toHaveCount(0);
  await expect.poll(() => calls(control, IPC.adeTaskTakeOver)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskTakeOver)[0]?.args).toMatchObject({ sessionId: 'sp11' });
});

test('Escape closes the dropdown and returns focus to the trigger', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openSessions(page, 'T_bill');
  await page.locator(t('ade-sessions-menu-trigger')).click();
  await expect(page.locator(t('ade-sessions-menu-popover'))).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator(t('ade-sessions-menu-popover'))).toHaveCount(0);
  await expect(page.locator(t('ade-sessions-menu-trigger'))).toBeFocused();
});

test('a running row selects its strip tab and closes the dropdown', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openSessions(page, 'T_bill');
  await page.locator(t('ade-sessions-menu-trigger')).click();
  await page.locator(rowOf('m111')).click();
  await expect(page.locator(t('ade-sessions-menu-popover'))).toHaveCount(0);
  await expect(page.locator(t('ade-session-tab-m111'))).toHaveAttribute('data-selected', 'true');
  await page.locator(t('ade-sessions-menu-trigger')).click();
  await page.locator(rowOf('b1c2')).hover();
  await page.keyboard.press('Enter');
  await expect(page.locator(t('ade-session-tab-b1c2'))).toHaveAttribute('data-selected', 'true');
});

test('the filter shows from 6 rows and narrows by id; no match says so', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openSessions(page, 'T_bill');
  await page.locator(t('ade-sessions-menu-trigger')).click();
  const filter = page.locator(t('ade-sessions-menu-filter'));
  await filter.fill('sp11');
  expect(await rowIds(page)).toEqual(['sp11']);
  await filter.fill('zzzz');
  await expect(page.locator(t('ade-sessions-menu-empty'))).toHaveText('No matches');
  expect(await rowIds(page)).toEqual([]);
});

test('a branch with fewer than 6 sessions has no filter', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator('[data-testid="ade-branch-row"][data-branch-id="b_billdash"]').click();
  await page.locator(t('ade-panel-tab-sessions')).click();
  await page.locator(t('ade-sessions-menu-trigger')).click();
  await expect(page.locator(t('ade-sessions-menu-popover'))).toBeVisible();
  await expect(page.locator(t('ade-sessions-menu-filter'))).toHaveCount(0);
});

test('opening a stopped session reveals it checked and highlighted', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await emitOpenSession(page, { taskId: 'T_bill', branchId: '', sessionId: 'sp11' });
  await expect(page.locator(t('ade-sessions-menu-popover'))).toBeVisible();
  await expect(page.locator(rowOf('sp11'))).toHaveAttribute('data-selected', 'true');
  await expect(page.locator(rowOf('sp11'))).toHaveAttribute('data-highlighted', '');
});

test('the row menu opens by right-click and Shift+F10', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openSessions(page, 'T_bill');
  const menuIds = () =>
    page
      .locator('[data-testid^="menu-item-ade-session-"]')
      .evaluateAll((els) => els.map((e) => e.getAttribute('data-testid')));
  await page.locator(t('ade-sessions-menu-trigger')).click();
  await page.locator(rowOf('sp11')).click({ button: 'right' });
  expect(await menuIds()).toEqual([
    'menu-item-ade-session-takeover-menu',
    'menu-item-ade-session-copy-id',
  ]);
  await page.keyboard.press('Escape');
  await page.locator(t('ade-sessions-menu-trigger')).click();
  await page.locator(rowOf('tk01')).click({ button: 'right' });
  expect(await menuIds()).toEqual([
    'menu-item-ade-session-restart',
    'menu-item-ade-session-close',
    'menu-item-ade-session-copy-id',
  ]);
  await page.keyboard.press('Escape');
  await page.locator(t('ade-sessions-menu-trigger')).click();
  await page.locator(rowOf('rv01')).hover();
  await page.keyboard.press('Shift+F10');
  await expect(page.locator(t('menu-item-ade-session-takeover-menu'))).toBeVisible();
});

test('a headless row toggles its log below the row', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openSessions(page, 'T_bill');
  await page.locator(t('ade-sessions-menu-trigger')).click();
  const popover = page.locator(t('ade-sessions-menu-popover'));
  const toggle = popover.locator(`${t('ade-sessions-row-log')}[data-session-id="b1c2"]`);
  await expect(popover.locator(t('ade-run-log-line'))).toHaveCount(0);
  await page.locator(rowOf('b1c2')).hover();
  await toggle.click();
  await expect(popover.locator(t('ade-run-log-line')).first()).toBeVisible();
  await toggle.click();
  await expect(popover.locator(t('ade-run-log-line'))).toHaveCount(0);
});

test('a running session that needs input flags the trigger until its tab is open', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  await openSessions(page, 'T_bill');
  await expect(page.locator(t('ade-sessions-menu-attention'))).toBeVisible();
  await page.locator(t('ade-session-tab-9d10')).click();
  await expect(page.locator(t('ade-sessions-menu-attention'))).toHaveCount(0);
});

const toast = (page: Page) => page.locator('[data-sonner-toast]');
const closeChip = `${t('ade-op-status')}[data-op="closeSession"]`;

test('Close shows a chip on the strip tab, and past 2 s a loading then a success toast', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskCloseSession, response: undefined, hold: true },
  ]);
  await openSessions(page, 'T_bill');
  await page.locator(t('ade-session-tab-tk01')).click({ button: 'right' });
  await page.locator(t('menu-item-ade-session-close')).click();
  await expect(page.locator(`${t('ade-session-tab-tk01')} ${closeChip}`)).toBeVisible();
  await page.locator(t('ade-sessions-menu-trigger')).click();
  await expect(page.locator(`${rowOf('tk01')} ${closeChip}`)).toBeVisible();
  await expect(toast(page)).toContainText('Closing session', { timeout: 5000 });
  control.release(IPC.adeTaskCloseSession);
  await expect(toast(page)).toContainText('Closed session');
  await expect(page.locator(closeChip)).toHaveCount(0);
});

test('a refused Close toasts the error', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskCloseSession, error: { code: 'E_INTERNAL', message: 'session is busy' } },
  ]);
  await openSessions(page, 'T_bill');
  await page.locator(t('ade-session-tab-tk01')).click({ button: 'right' });
  await page.locator(t('menu-item-ade-session-close')).click();
  await expect(toast(page)).toContainText('session is busy');
});
