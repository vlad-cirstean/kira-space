import { expect, test } from '../ui/fixtures';
import { adeV2Control, FIXED_NOW } from '../ui/support/adeV2';
import { contract } from '../ui/support/contract';
import { IPC } from '../ui/support/ipcChannels';
import { emitWailsEvent } from '../ui/support/mockRuntime';

// P414: the status bar with running agents, a running automation, an update and the metrics
// readout, one open tooltip and the open agents popover. Contract ade-sessions-running.
const MINUTE = 60_000;
const PLACES = [
  { id: 'qa01', terminalId: 'term-qa01', taskId: 'T_auth', branchId: 'b_auth' },
  { id: 'qa02', terminalId: '', taskId: 'T_search', branchId: 'b_search' },
  { id: 'qa03', terminalId: '', taskId: 'T_cart', branchId: 'b_cart' },
];

const sessions = {
  sessions: contract<{ sessions: Record<string, unknown>[] }>(
    'ade-sessions-running',
    'AdeTaskService.Sessions#mixed',
  ).sessions.map((s, i) => ({
    ...s,
    ...PLACES[i],
    // The contract's finished run is unseen; these shots keep the viewed state.
    unseen: false,
    stepId: '',
    runId: '',
    cwd: '/tmp/work',
    startedAt: FIXED_NOW - 60 * MINUTE * (i + 1),
    lastActiveAt: FIXED_NOW - 5 * MINUTE * (i + 1),
  })),
};

const RUN = {
  id: 'run-1',
  scriptId: 's1',
  scriptName: 'Review diff',
  color: 'none',
  kind: 'script',
  trigger: 'manual',
  state: 'running',
  terminalId: '',
  cwd: '/tmp/work',
  command: '',
  outcome: null,
  createdAt: 1_000,
  startedAt: 1_000,
  finishedAt: null,
  params: [],
};

async function boot(relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch']) {
  const app = await relaunch({
    control: adeV2Control([
      { channel: IPC.windowsEnsure, response: { mode: 'git' } },
      { channel: IPC.adeTaskSessions, response: sessions },
      { channel: IPC.scriptRunsList, response: [RUN] },
      {
        channel: IPC.updateStatus,
        response: {
          updateAvailable: true,
          currentVersion: '1.2.0',
          latestVersion: '1.3.0',
          installLogPath: '/tmp/kira-space-update.log',
        },
      },
    ]),
    clockTime: FIXED_NOW,
  });
  await emitWailsEvent(app.window, IPC.appMetrics, {
    cpuPercent: 3.4,
    memoryBytes: 512 * 1024 * 1024,
    logicalCPUs: 8,
    processCount: 14,
  });
  await app.window.setViewportSize({ width: 1400, height: 900 });
  await expect(app.window.locator('[data-testid="status-agents"]')).toBeVisible();
  await expect(app.window.locator('[data-testid="status-runs"]')).toBeVisible();
  await expect(app.window.locator('[data-testid="update-available"]')).toBeVisible();
  await expect(app.window.locator('[data-testid="app-metrics"]')).toBeVisible();
  return app.window;
}

test('status bar with agents, runs, update and metrics', async ({ relaunch }) => {
  const window = await boot(relaunch);
  await expect(window.locator('[data-testid="status-bar"]')).toHaveScreenshot('status-bar.png');
});

test('status bar agents tooltip', async ({ relaunch }) => {
  const window = await boot(relaunch);
  await window.mouse.move(0, 0);
  await window.locator('[data-testid="status-agents"]').hover();
  const tip = window.locator('[data-slot="tooltip-content"]');
  await expect(tip).toBeVisible();
  await expect(tip).toHaveScreenshot('status-bar-agents-tooltip.png');
});

test('status bar agents popover', async ({ relaunch }) => {
  const window = await boot(relaunch);
  await window.locator('[data-testid="status-agents"]').click();
  const pop = window.locator('[data-testid="status-agents-popover"]');
  await expect(window.locator('[data-testid="status-agents-row"]')).toHaveCount(2);
  await expect(pop).toHaveScreenshot('status-bar-agents-popover.png');
});
