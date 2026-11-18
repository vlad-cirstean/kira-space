import type { Page } from '@playwright/test';
import { defaultSettings } from '../../frontend/src/state/settingsDomain';
import { expect, test } from './fixtures';
import { FIXED_NOW, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// P239: the Claude Code usage limits item in the ADE status bar, and its Settings switch.

const HOUR = 3_600_000;

function snapshot(five: number | null, seven: number | null, extra: Record<string, unknown> = {}) {
  return {
    state: 'ok',
    source: 'session',
    fiveHour: five === null ? null : { usedPercent: five, resetsAt: FIXED_NOW + 2 * HOUR },
    sevenDay: seven === null ? null : { usedPercent: seven, resetsAt: FIXED_NOW + 50 * HOUR },
    updatedAt: FIXED_NOW - 5 * 60_000,
    ...extra,
  };
}

interface UsageWindow {
  usedPercent: number;
  resetsAt: number;
}

// The backend snapshot (flows/usageflow, contract claude-usage); times are wall-clock there, so
// the spec dates them from FIXED_NOW.
function fromContract(key: string) {
  const snap = contract<{
    fiveHour: UsageWindow | null;
    sevenDay: UsageWindow | null;
    source: string;
    state: string;
  }>('agent-usage', key);
  return {
    ...snap,
    fiveHour: snap.fiveHour && { ...snap.fiveHour, resetsAt: FIXED_NOW + 2 * HOUR },
    sevenDay: snap.sevenDay && { ...snap.sevenDay, resetsAt: FIXED_NOW + 50 * HOUR },
    updatedAt: FIXED_NOW - 5 * 60_000,
  };
}

const pct = (w: UsageWindow | null) => Math.round(w?.usedPercent ?? 0);

const usageControl = (snap: unknown) => [{ channel: IPC.agentUsageGet, response: snap }];
const item = (page: Page) => page.locator('[data-testid="agent-usage-status"]');
const text = (page: Page) => page.locator('[data-testid="agent-usage-text"]');

test('hidden outside the ADE module', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: usageControl(snapshot(23, 41)) });
  await expect(item(page)).toHaveCount(0);
});

test('shows both windows, and the tooltip lists the reset times and the source', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch, usageControl(snapshot(23.4, 41.2)));
  await expect(text(page)).toHaveText('Session 23% · Weekly 41%');
  await expect(item(page)).not.toHaveClass(/(^|\s)text-(warn|error)(\s|$)/);
  await item(page).hover();
  const tip = page.locator('[data-testid="agent-usage-tooltip"]');
  await expect(tip.locator('[data-testid="tooltip-details-title"]')).toHaveText(
    'Claude Code usage limits',
  );
  const row = (label: string) =>
    tip.locator(`[data-testid="tooltip-details-row"][data-label="${label}"]`);
  await expect(row('Session (5-hour)')).toContainText('23% used, resets in 2h');
  await expect(row('Weekly')).toContainText('41% used, resets in');
  await expect(tip.locator('[data-testid="tooltip-details-hint"]')).toHaveText(
    'From a Claude Code session, 5m ago.',
  );
});

test('the readout uses the interface font like the rest of the bar', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, usageControl(snapshot(23, 41)));
  const family = (selector: string) =>
    page
      .locator(selector)
      .first()
      .evaluate((el) => getComputedStyle(el).fontFamily);
  expect(await family('[data-testid="agent-usage-text"]')).toBe(await family('body'));
});

test('contract: a session snapshot shows both windows and its source', async ({ relaunch }) => {
  const snap = fromContract('AgentUsageService.Get#session');
  const { window: page } = await openPlan(relaunch, usageControl(snap));
  await expect(text(page)).toHaveText(
    `Session ${pct(snap.fiveHour)}% · Weekly ${pct(snap.sevenDay)}%`,
  );
  await item(page).hover();
  await expect(page.locator('[data-testid="agent-usage-tooltip"]')).toContainText(
    'From a Claude Code session',
  );
});

test('contract: a run snapshot names the run source', async ({ relaunch }) => {
  const snap = fromContract('AgentUsageService.Get#run');
  const { window: page } = await openPlan(relaunch, usageControl(snap));
  await expect(text(page)).toHaveText(
    `Session ${pct(snap.fiveHour)}% · Weekly ${pct(snap.sevenDay)}%`,
  );
  await item(page).hover();
  await expect(page.locator('[data-testid="agent-usage-tooltip"]')).toContainText(
    'From a background ADE run',
  );
});

test('contract: one window only shows only that window', async ({ relaunch }) => {
  const snap = fromContract('AgentUsageService.Get#expired');
  expect(snap.sevenDay).toBeNull();
  const { window: page } = await openPlan(relaunch, usageControl(snap));
  await expect(text(page)).toHaveText(`Session ${pct(snap.fiveHour)}%`);
});

test('the usage item sits in the left group with the dashboard icon', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, usageControl(snapshot(23, 41)));
  const usage = await item(page).boundingBox();
  const viewport = page.viewportSize();
  if (!(usage && viewport)) throw new Error('status bar item not laid out');
  expect(usage.x).toBeLessThan(24);
  expect(usage.x + usage.width).toBeLessThan(viewport.width / 2);
  await expect(item(page).locator('.codicon-dashboard')).toHaveCount(1);
});

test('warns at 80 percent and errors at 95 percent', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, usageControl(snapshot(85, 10)));
  await expect(item(page)).toHaveAttribute('data-tone', 'warn');
  await emitWailsEvent(page, IPC.agentUsage, snapshot(85, 97));
  await expect(text(page)).toHaveText('Session 85% · Weekly 97%');
  await expect(item(page)).toHaveAttribute('data-tone', 'error');
});

test('shows a dash and the hint before any session reported', async ({ relaunch }) => {
  const { window: page } = await openPlan(
    relaunch,
    usageControl({
      state: 'waiting',
      source: '',
      fiveHour: null,
      sevenDay: null,
      updatedAt: 0,
    }),
  );
  await expect(text(page)).toHaveText('Usage: –');
  await item(page).hover();
  await expect(page.locator('[data-testid="agent-usage-tooltip"]')).toContainText(
    'Start a Claude Code session to see usage',
  );
});

test('shows unavailable when the read fails', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.agentUsageGet, error: { code: 'internal', message: 'boom' } },
  ]);
  await expect(text(page)).toHaveText('Usage unavailable');
});

test('a pushed snapshot replaces the shown values', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, usageControl(snapshot(10, 20)));
  await expect(text(page)).toHaveText('Session 10% · Weekly 20%');
  await emitWailsEvent(page, IPC.agentUsage, snapshot(30, 40));
  await expect(text(page)).toHaveText('Session 30% · Weekly 40%');
});

test('hidden while the setting is off', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    ...usageControl(snapshot(23, 41)),
    {
      channel: IPC.settingsGetAll,
      response: {
        ...defaultSettings,
        agent: { ...defaultSettings.agent, usageEnabled: false },
      },
    },
  ]);
  await expect(page.locator('[data-testid="ade-plan"]')).toBeVisible();
  await expect(item(page)).toHaveCount(0);
});

test('the Settings switch saves agent.usageEnabled', async ({ relaunch }) => {
  const { window: page, control } = await relaunch();
  await emitWailsEvent(page, IPC.openSettings, undefined);
  await page.locator('[data-testid="settings-section-Agent"]').click();
  const sw = page.locator('[data-testid="settings-agent-usage"]');
  await expect(sw).toHaveAttribute('aria-checked', 'true');
  await sw.click();
  await page.locator('[data-testid="settings-save"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.settingsSet)?.args)
    .toEqual({ patch: { agent: { usageEnabled: false } } });
});
