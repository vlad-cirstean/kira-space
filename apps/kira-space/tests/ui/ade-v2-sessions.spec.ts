import { expect, test } from './fixtures';
import { adeBoard, emitOpenSession, openPlan, openSessions } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { dragSelect, longLog } from './support/logSpecs';

// The Sessions tab of a task and of a branch, Take over and the terminal pane.

const t = (id: string) => `[data-testid="${id}"]`;
const row = (id: string) => `[data-testid="ade-branch-row"][data-branch-id="${id}"]`;

test('the task strip lists interactive sessions first, then the most recent runs', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  await openSessions(page, 'T_bill');
  const tabs = page.locator(`${t('ade-session-strip')} button`);
  await expect(tabs).toHaveCount(4);
  const ids = await tabs.evaluateAll((els) =>
    els.map((e) => e.getAttribute('data-testid')?.replace('ade-session-tab-', '')),
  );
  expect(ids).toEqual(['tk01', 'b1c2', 'm111', '9d10']);
  const badges = await page.locator(t('ade-session-badge')).allTextContents();
  expect(badges).toEqual(['TUI', 'claude -p', 'claude -p', 'claude -p']);
  await expect(page.locator(t('ade-session-tab-tk01'))).toContainText('(resumed)');
  await expect(page.locator(t('ade-session-tab-tk01'))).toHaveAttribute('data-selected', 'true');
});

test('tabs are named after the step; the tab menu copies the full Claude session id', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  await openSessions(page, 'T_bill');
  await expect(page.locator(t('ade-session-tab-tk01'))).toContainText('Implement · ');
  await expect(page.locator(t('ade-session-tab-b1c2'))).toContainText('Implement · ');
  await expect(page.locator(t('ade-session-tab-tk01'))).not.toContainText('claude ');
  await expect(page.locator(t('ade-session-id'))).toHaveCount(0);

  await page.evaluate(() => {
    const w = window as unknown as { __clipboard: string[] };
    w.__clipboard = [];
    navigator.clipboard.writeText = async (text: string) => {
      w.__clipboard.push(text);
    };
  });
  await page.locator(t('ade-session-tab-tk01')).click({ button: 'right' });
  await page.locator(t('menu-item-ade-session-copy-id')).click();
  await expect
    .poll(() => page.evaluate(() => (window as unknown as { __clipboard: string[] }).__clipboard))
    .toEqual(['claude-tk01']);
});

test('a TUI tab shows its state and age on hover', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openSessions(page, 'T_bill');
  await page.locator(t('ade-session-tab-tk01')).hover();
  await expect(page.locator(t('ade-session-tip'))).toContainText('Interactive · ');
  await expect(page.locator(`${t('ade-tui-pane')} ${t('ade-tui-label')}`)).toHaveCount(0);
});

test('contract: Restart closes the session and reopens it in a new terminal', async ({
  relaunch,
}) => {
  const launch = contract<Record<string, string>>(
    'ade-session-restart',
    'AdeTaskService.RestartSession',
  );
  expect(Object.keys(launch).sort()).toEqual(['command', 'cwd', 'sessionId', 'terminalId']);
  const { window: page, control } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskRestartSession,
      response: { ...launch, sessionId: 'tk01', terminalId: 'term-tk01-r' },
    },
  ]);
  await openSessions(page, 'T_bill');
  await page.locator(t('ade-session-tab-tk01')).click({ button: 'right' });
  await page.locator(t('menu-item-ade-session-restart')).click();
  await expect.poll(() => calls(control, IPC.adeTaskRestartSession)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskRestartSession)[0]?.args).toEqual({ sessionId: 'tk01' });
  await expect.poll(() => calls(control, IPC.terminalOpen)).toHaveLength(1);
  expect(JSON.stringify(calls(control, IPC.terminalOpen)[0]?.args)).toContain('term-tk01-r');
  await expect(page.locator(t('ade-session-tab-tk01'))).toHaveAttribute('data-selected', 'true');
});

test('a refused Restart shows its error on the panel', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskRestartSession,
      error: { code: 'E_BAD_REQUEST', message: 'the session is not running' },
    },
  ]);
  await openSessions(page, 'T_bill');
  await page.locator(t('ade-session-tab-tk01')).click({ button: 'right' });
  await page.locator(t('menu-item-ade-session-restart')).click();
  await expect(page.locator(t('ade-panel-action-error'))).toContainText(
    'the session is not running',
  );
});

test('contract: Restart of a background run starts a new attempt', async ({ relaunch }) => {
  const launch = contract<Record<string, string>>(
    'ade-session-restart-headless',
    'AdeTaskService.RestartSession',
  );
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskRestartSession, response: { ...launch, sessionId: 'b1c2' } },
  ]);
  await openSessions(page, 'T_bill');
  await page.locator(t('ade-session-tab-b1c2')).click({ button: 'right' });
  await page.locator(t('menu-item-ade-session-restart')).click();
  await expect.poll(() => calls(control, IPC.adeTaskRestartSession)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskRestartSession)[0]?.args).toEqual({ sessionId: 'b1c2' });
  expect(calls(control, IPC.terminalOpen)).toHaveLength(0);
});

test('the tab menu lists Restart, Close and Copy session ID in order', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openSessions(page, 'T_bill');
  await page.locator(t('ade-session-tab-tk01')).click({ button: 'right' });
  const ids = await page
    .locator('[data-testid^="menu-item-ade-session-"]')
    .evaluateAll((els) => els.map((el) => el.getAttribute('data-testid')));
  expect(ids).toEqual([
    'menu-item-ade-session-restart',
    'menu-item-ade-session-close',
    'menu-item-ade-session-copy-id',
  ]);
});

test('Close sends the session id, for a TUI and a background tab', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await openSessions(page, 'T_bill');
  await page.locator(t('ade-session-tab-tk01')).click({ button: 'right' });
  await page.locator(t('menu-item-ade-session-close')).click();
  await expect.poll(() => calls(control, IPC.adeTaskCloseSession)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskCloseSession)[0]?.args).toEqual({ sessionId: 'tk01' });
  await page.locator(t('ade-session-tab-b1c2')).click({ button: 'right' });
  await page.locator(t('menu-item-ade-session-close')).click();
  await expect.poll(() => calls(control, IPC.adeTaskCloseSession)).toHaveLength(2);
  expect(calls(control, IPC.adeTaskCloseSession)[1]?.args).toEqual({ sessionId: 'b1c2' });
});

test('a refused Close shows its error on the panel', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskCloseSession,
      error: { code: 'E_BAD_REQUEST', message: 'the session is not running' },
    },
  ]);
  await openSessions(page, 'T_bill');
  await page.locator(t('ade-session-tab-tk01')).click({ button: 'right' });
  await page.locator(t('menu-item-ade-session-close')).click();
  await expect(page.locator(t('ade-panel-action-error'))).toContainText(
    'the session is not running',
  );
});

test('a branch scope shows only that branch', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator(row('b_billdash')).click();
  await page.locator(t('ade-panel-tab-sessions')).click();
  const tabs = page.locator(`${t('ade-session-strip')} button`);
  await expect(tabs).toHaveCount(2);
  await expect(page.locator(t('ade-session-tab-9d10'))).toBeVisible();
  await expect(page.locator(t('ade-session-tab-tk01'))).toBeVisible();
});

test('a headless tab shows its status on hover, its log, and Stop sends the run id', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch);
  await openSessions(page, 'T_bill');
  await page.locator(t('ade-session-tab-b1c2')).click();
  await page.mouse.move(0, 0);
  await page.locator(t('ade-session-tab-b1c2')).hover();
  await expect(page.locator(t('ade-session-tip'))).toContainText('Background run · step');
  await expect(page.locator(t('ade-run-log-line')).first()).toBeVisible();
  await page.locator(t('ade-session-stop')).click();
  await expect.poll(() => calls(control, IPC.adeTaskStopRun)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskStopRun)[0]?.args).toEqual({ runId: 'r_T_bill_impl_b_bill' });
});

async function openHeadlessLog(relaunch: Parameters<typeof openPlan>[0]) {
  const { window: page } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskReadLog,
      response: {
        kind: 'run',
        id: 'r_T_bill_impl_b_bill',
        chunks: longLog(300),
        nextSeq: 301,
        truncated: false,
        done: false,
      },
    },
  ]);
  await openSessions(page, 'T_bill');
  await page.locator(t('ade-session-tab-b1c2')).click();
  const log = page.locator(`${t('ade-headless-pane')} ${t('ade-run-log')}`);
  await expect(log).toContainText('line 2 of the log');
  return { page, log };
}

test("a headless session's log scrolls and its text selects", async ({ relaunch }) => {
  const { page, log } = await openHeadlessLog(relaunch);
  const pane = (await page.locator(t('ade-headless-pane')).boundingBox())!;
  const box = (await log.boundingBox())!;
  expect(box.y + box.height).toBeLessThanOrEqual(pane.y + pane.height + 1);
  await log.hover();
  await page.mouse.wheel(0, 2000);
  await expect.poll(() => log.evaluate((el) => el.scrollTop)).toBeGreaterThan(0);
  await log.evaluate((el) => {
    el.scrollTop = 0;
  });
  const rows = log.locator(t('ade-run-log-line'));
  await expect(rows.nth(2)).toBeVisible();
  const text = await dragSelect(page, rows);
  expect(text).toContain('line 2 of the log');
  expect(text).toContain('line 3 of the log');
});

test('Take over of a running run confirms, stops it and selects the resumed session', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch);
  await openSessions(page, 'T_bill');
  await page.locator(t('ade-session-tab-9d10')).click();
  await page.locator(t('ade-session-takeover')).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog).toContainText('Take over run?');
  await expect(dialog).toContainText(
    'is still running on feat/billing-dashboard. Taking over stops it, marks it stuck and continues it in Claude Code.',
  );
  await dialog.getByRole('button', { name: 'Cancel' }).click();
  expect(calls(control, IPC.adeTaskTakeOver)).toHaveLength(0);
  await page.locator(t('ade-session-takeover')).click();
  await page.locator(t('ade-confirm-yes')).click();
  await expect.poll(() => calls(control, IPC.adeTaskTakeOver)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskTakeOver)[0]?.args).toEqual({
    sessionId: '9d10',
    stopIfRunning: true,
  });
  await expect(page.locator(t('ade-session-tab-tk01'))).toHaveAttribute('data-selected', 'true');
});

test('Take over of a stuck run skips the confirm', async ({ relaunch }) => {
  const board = adeBoard((b) => {
    for (const task of b.tasks) {
      for (const run of task.runs) if (run.id === 'r_T_bill_impl_b_bill') run.state = 'stuck';
    }
  });
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  await openSessions(page, 'T_bill');
  await page.locator(t('ade-session-tab-b1c2')).click();
  await expect(page.locator(t('ade-session-stop'))).toHaveCount(0);
  await page.locator(t('ade-session-takeover')).click();
  await expect.poll(() => calls(control, IPC.adeTaskTakeOver)).toHaveLength(1);
  await expect(page.getByRole('dialog')).toHaveCount(0);
});

test('contract: an open-session event selects that session in its Sessions tab', async ({
  relaunch,
}) => {
  const open = contract<{ taskId: string; branchId: string; sessionId: string }>(
    'notify-reveal',
    'event:kira:adetask:openSession',
  );
  expect(Object.keys(open).sort()).toEqual(['branchId', 'sessionId', 'taskId']);
  const { window: page } = await openPlan(relaunch);
  await emitOpenSession(page, {
    ...open,
    taskId: 'T_bill',
    branchId: 'b_meter',
    sessionId: 'm111',
  });
  await expect(page.locator(t('ade-session-tab-m111'))).toHaveAttribute('data-selected', 'true');
});

test('Show focuses the window holding the terminal, or says none does', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskFocusSession, response: false },
  ]);
  await openSessions(page, 'T_bill');
  await page.locator(t('ade-tui-show')).click();
  await expect.poll(() => calls(control, IPC.adeTaskFocusSession)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskFocusSession)[0]?.args).toMatchObject({ sessionId: 'tk01' });
  await expect(page.locator(t('ade-tui-missing'))).toHaveText(
    'This session’s terminal is not open in any window.',
  );
});
