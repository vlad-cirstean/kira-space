// Contract ade-session-status, backend half: adeflow TestSessionStatusBackgroundAndUnseen.
import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import {
  adeFixture,
  emitAgentEvent,
  emitAgentSessions,
  emitSessions,
  openPlan,
  openSessions,
} from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// An agent session with background tasks left still works; a finished session nobody viewed is
// unseen until its pane is shown in a focused window.

const t = (id: string) => `[data-testid="${id}"]`;
const TERMINAL = 'term-tk01';

interface WireSession extends Record<string, unknown> {
  id: string;
}

/** The one interactive session of T_bill, with the status fields the backend reported for `key`. */
function sessionsAt(key: 'background' | 'unseen' | 'seen'): unknown {
  const fx = contract<{ sessions: WireSession[] }>(
    'ade-session-status',
    `AdeTaskService.Sessions#${key}`,
  ).sessions[0];
  const base = adeFixture<{ sessions: WireSession[] }>('sessions').sessions.find(
    (s) => s.id === 'tk01',
  );
  if (!fx || !base) throw new Error('fixture session missing');
  return {
    sessions: [
      {
        ...base,
        activity: fx.activity,
        background: fx.background,
        unseen: fx.unseen,
      },
    ],
  };
}

const sessionsControl = (key: 'background' | 'unseen' | 'seen') => [
  { channel: IPC.adeTaskSessions, response: sessionsAt(key) },
  { channel: IPC.mobileLaunchOpened, response: null },
];

const tabIcon = (page: Page) => page.locator(`${t('ade-session-tab-tk01')} [data-activity]`);
const item = (page: Page) => page.locator(t('status-agents'));
const cardState = (page: Page) =>
  page.locator(`${t('ade-card')}[data-task-id="T_bill"] ${t('ade-card-sessions')}`);

test('contract: a Stop with background tasks left keeps the session working until the last Stop', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch, sessionsControl('seen'));
  await openSessions(page, 'T_bill');
  await emitAgentSessions(page, [TERMINAL]);
  await emitAgentEvent(page, TERMINAL, 'SessionStart');
  await emitAgentEvent(page, TERMINAL, 'UserPromptSubmit');
  await emitAgentEvent(page, TERMINAL, 'Stop', '', { background: 2 });
  await expect(tabIcon(page)).toHaveAttribute('data-activity', 'working');
  await expect(item(page)).toHaveAttribute('data-kind', 'working');
  await expect(cardState(page)).toHaveAttribute('data-state', 'running');

  // A subagent's own tool hooks arrive after the main Stop and never move the phase.
  await emitAgentEvent(page, TERMINAL, 'PreToolUse', '', { agentId: 'a1' });
  await emitAgentEvent(page, TERMINAL, 'SubagentStop', '', { agentId: 'a1', background: 1 });
  await expect(tabIcon(page)).toHaveAttribute('data-activity', 'working');

  await emitAgentEvent(page, TERMINAL, 'UserPromptSubmit');
  await emitAgentEvent(page, TERMINAL, 'Stop', '', { background: 0 });
  await expect(tabIcon(page)).toHaveAttribute('data-activity', 'idle');
  await expect(item(page)).toHaveAttribute('data-kind', 'idle');
});

test('contract: a window opened after the last event reads the background count from the wire', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch, sessionsControl('background'));
  await openSessions(page, 'T_bill');
  await expect(tabIcon(page)).toHaveAttribute('data-activity', 'working');
  await expect(item(page)).toHaveAttribute('data-kind', 'working');
});

test('contract: a finished session nobody viewed shows unseen everywhere until its pane is shown', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, sessionsControl('unseen'));
  await expect(item(page)).toHaveText('1 agent · 1 unseen');
  await expect(item(page)).toHaveAttribute('data-kind', 'unseen');
  await expect(cardState(page)).toHaveAttribute('data-state', 'unseen');
  await openSessions(page, 'T_bill');
  await expect(tabIcon(page)).toHaveAttribute('data-activity', 'unseen');
  await page.locator(t('ade-session-tab-tk01')).hover();
  await expect(page.locator(t('ade-session-tip'))).toContainText('Finished, not viewed');

  // The terminal is held in no window yet: nothing marks the session seen.
  await page.waitForTimeout(800);
  expect(calls(control, IPC.adeTaskMarkSessionSeen)).toHaveLength(0);

  // Its pane opens here: after the dwell the window reports the view.
  await emitWailsEvent(page, IPC.mobileOpenLaunch, {
    launch: adeFixture('launch'),
    taskId: 'T_bill',
    branchId: 'b_billdash',
  });
  await expect(page.locator(t('ade-tui-pane'))).toBeVisible();
  await expect.poll(() => calls(control, IPC.adeTaskMarkSessionSeen).length).toBeGreaterThan(0);
  expect(calls(control, IPC.adeTaskMarkSessionSeen)[0]?.args).toEqual({ sessionId: 'tk01' });

  // The backend cleared the marker; the pushed list drops it.
  control.respond(IPC.adeTaskSessions, sessionsAt('seen'));
  await emitSessions(page);
  await expect(tabIcon(page)).toHaveAttribute('data-activity', 'idle');
  await expect(item(page)).toHaveText('1 agent');
  await expect(cardState(page)).toHaveAttribute('data-state', 'finished');
});
