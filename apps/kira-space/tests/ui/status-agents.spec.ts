import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { adeV2Control, emitAgentEvent, emitAgentSessions, FIXED_NOW } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// Contract ade-sessions-running, backend half: adeflow TestRunningSessionsList. The status bar
// counts the running ADE agents of every module and lists them in a popover.

const t = (id: string) => `[data-testid="${id}"]`;
const MINUTE = 60_000;

// The contract lists a running interactive session, a running background run and a stopped run;
// the ids are placed on tasks of the committed board.
const PLACES = [
  { id: 'qa01', terminalId: 'term-qa01', taskId: 'T_auth', branchId: 'b_auth' },
  { id: 'qa02', terminalId: '', taskId: 'T_search', branchId: 'b_search' },
  { id: 'qa03', terminalId: '', taskId: 'T_cart', branchId: 'b_cart' },
];

function sessions(running = true, idle = false): unknown {
  const fx = contract<{ sessions: Record<string, unknown>[] }>(
    'ade-sessions-running',
    'AdeTaskService.Sessions#mixed',
  );
  return {
    sessions: (running ? fx.sessions : []).map((s, i) => ({
      ...s,
      ...PLACES[i],
      ...(idle && s.activity ? { activity: 'idle' } : {}),
      stepId: '',
      runId: '',
      cwd: '/tmp/work',
      startedAt: FIXED_NOW - 60 * MINUTE * (i + 1),
      lastActiveAt: FIXED_NOW - 5 * MINUTE * (i + 1),
    })),
  };
}

const control = (extra: readonly ControlSnapshot[] = []): ControlSnapshot[] => {
  const base: ControlSnapshot[] = [
    { channel: IPC.windowsEnsure, response: { mode: 'git' } },
    { channel: IPC.adeTaskSessions, response: sessions() },
    { channel: IPC.adeTaskFocusSession, response: false },
  ];
  const overridden = new Set(extra.map((e) => e.channel));
  return adeV2Control([...base.filter((e) => !overridden.has(e.channel)), ...extra]);
};

const item = (page: Page) => page.locator(t('status-agents'));
const dot = (page: Page) => page.locator(t('status-agents-working'));

test('contract: the bar counts the running agents in git mode and the tooltip counts them by activity', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: control(), clockTime: FIXED_NOW });
  await expect(item(page)).toHaveText('2 agents');
  await expect(item(page)).toHaveAttribute('data-kind', 'working');
  await expect(dot(page)).toBeVisible();
  await expect(dot(page)).toHaveAttribute('aria-label', '1 working');
  await expect(dot(page)).toHaveClass(/(^|\s)animate-pulse(\s|$)/);
  await expect(item(page)).toHaveText('2 agents');
  await item(page).hover();
  const tip = page.locator(t('tooltip-details'));
  await expect(tip.locator(t('tooltip-details-title'))).toHaveText('Running agents');
  await expect(tip.locator(t('tooltip-details-row'))).toHaveCount(2);
  await expect(tip.locator(`${t('tooltip-details-row')}[data-label="Working"]`)).toHaveText(
    /Working\s*1/,
  );
  await expect(tip.locator(`${t('tooltip-details-row')}[data-label="Idle"]`)).toHaveText(
    /Idle\s*1/,
  );
  await expect(tip.locator(t('tooltip-details-hint'))).toHaveText('Click to list them.');
});

test('contract: an agent asking for input shows in the label and turns the item to a warning', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: control(), clockTime: FIXED_NOW });
  await expect(item(page)).toHaveText('2 agents');
  await emitAgentSessions(page, ['term-qa01']);
  await emitAgentEvent(page, 'term-qa01', 'Notification');
  await expect(item(page)).toHaveText('2 agents · 1 needs input');
  await expect(item(page)).toHaveAttribute('data-kind', 'input');
  await expect(item(page)).toHaveClass(/(^|\s)text-warn(\s|$)/);
  await expect(dot(page)).toBeVisible();
});

test('contract: the working dot shows only while an agent works and follows the TUI activity', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: control([{ channel: IPC.adeTaskSessions, response: sessions(true, true) }]),
    clockTime: FIXED_NOW,
  });
  await expect(item(page)).toHaveText('2 agents');
  await expect(dot(page)).toHaveCount(0);
  await emitAgentSessions(page, ['term-qa01']);
  await emitAgentEvent(page, 'term-qa01', 'PreToolUse');
  await expect(dot(page)).toBeVisible();
  await expect(dot(page)).toHaveAttribute('aria-label', '1 working');
  await expect(item(page)).toHaveAttribute('data-kind', 'working');
  await emitAgentEvent(page, 'term-qa01', 'Stop');
  await expect(dot(page)).toHaveCount(0);
  await expect(item(page)).toHaveText('2 agents');
});

test('contract: reduced motion keeps the working dot but stops its pulse', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: control(), clockTime: FIXED_NOW });
  const animation = () => dot(page).evaluate((el) => getComputedStyle(el).animationName);
  await page.emulateMedia({ reducedMotion: 'no-preference' });
  await expect(dot(page)).toBeVisible();
  expect(await animation()).not.toBe('none');
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await expect(dot(page)).toBeVisible();
  expect(await animation()).toBe('none');
});

test('contract: no running agent hides the item', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: control([{ channel: IPC.adeTaskSessions, response: sessions(false) }]),
    clockTime: FIXED_NOW,
  });
  await expect(item(page)).toHaveCount(0);
});

test('contract: the popover lists the agents most urgent first and a row opens its session', async ({
  relaunch,
}) => {
  const { window: page, control: ctl } = await relaunch({
    control: control(),
    clockTime: FIXED_NOW,
  });
  await item(page).click();
  const rows = page.locator(t('status-agents-row'));
  await expect(rows).toHaveCount(2);
  expect(
    await rows.evaluateAll((els) => els.map((e) => e.getAttribute('data-session-id'))),
  ).toEqual(['qa02', 'qa01']);
  await rows.nth(1).click();
  await expect(page.locator(t('status-agents-popover'))).toHaveCount(0);
  await expect.poll(() => calls(ctl, IPC.adeTaskFocusSession)).toHaveLength(1);
  expect(calls(ctl, IPC.adeTaskFocusSession)[0]?.args).toMatchObject({
    sessionId: 'qa01',
    taskId: 'T_auth',
  });
  await expect(page.locator(t('ade-sessions-tab'))).toBeVisible();
  await expect(page.locator(t('ade-session-tab-qa01'))).toHaveAttribute('data-selected', 'true');
});

test('contract: a background run row opens its session too', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: control(), clockTime: FIXED_NOW });
  await item(page).click();
  await page.locator(`${t('status-agents-row')}[data-session-id="qa02"]`).click();
  await expect(page.locator(t('ade-sessions-tab'))).toBeVisible();
  await expect(page.locator(t('ade-session-tab-qa02'))).toHaveAttribute('data-selected', 'true');
});

test('contract: a session another window holds is raised there, not opened here', async ({
  relaunch,
}) => {
  const { window: page, control: ctl } = await relaunch({
    control: control([{ channel: IPC.adeTaskFocusSession, response: true }]),
    clockTime: FIXED_NOW,
  });
  await item(page).click();
  await page.locator(`${t('status-agents-row')}[data-session-id="qa01"]`).click();
  await expect.poll(() => calls(ctl, IPC.adeTaskFocusSession)).toHaveLength(1);
  await expect(page.locator(t('ade-session-tab-qa01'))).toHaveCount(0);
});

test('contract: Show all sessions opens the Needs page with All sessions', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: control(), clockTime: FIXED_NOW });
  await item(page).click();
  await page.locator(t('status-agents-all')).click();
  await expect(page.locator(t('ade-needs-page'))).toBeVisible();
  await expect(page.locator(t('ade-all-session')).first()).toBeVisible();
});
