import { expect, test } from '../ui/fixtures';
import { IPC } from '../ui/support/ipcChannels';

// P432: the Operations dock at rest, then with one failed op and one step list expanded.

const T0 = '2026-01-01T00:00:00.000Z';

function op(overrides: Record<string, unknown>): Record<string, unknown> {
  return {
    id: 'op-1',
    startedAt: T0,
    durationMs: 12,
    kind: 'branchCreate',
    status: 'ok',
    command: 'git branch feat main',
    error: null,
    category: 'git',
    module: 'git',
    trigger: 'user',
    subject: '',
    repoRoot: '/work/alpha',
    repoName: 'alpha',
    source: 'Kira Space',
    taskId: '',
    parentId: '',
    background: false,
    repeat: 1,
    steps: [],
    stepsDropped: 0,
    cancellable: false,
    ...overrides,
  };
}

const RECORDS = [
  op({ id: 'op-a', startedAt: '2026-01-01T00:03:00.000Z', command: 'git fetch origin' }),
  op({
    id: 'op-b',
    startedAt: '2026-01-01T00:02:00.000Z',
    status: 'error',
    kind: 'push',
    durationMs: 480,
    command: 'git push origin feat',
    error: 'fatal: unable to access remote\nhint: check credentials',
    steps: [
      {
        argv: 'git push origin feat',
        readOnly: false,
        exitCode: 1,
        durationMs: 480,
        status: 'error',
        stderr: 'fatal: unable to access remote',
      },
    ],
  }),
];

test('operations dock: list and expanded failed op', async ({ relaunch }) => {
  const { window } = await relaunch({
    control: [{ channel: IPC.opsRecent, response: RECORDS }],
  });
  await window.setViewportSize({ width: 1400, height: 800 });
  await window.click('[data-testid="toggle-operations-panel"]');
  const handle = window.getByRole('separator', { name: 'Resize operations panel' });
  await handle.focus();
  for (let i = 0; i < 20; i++) await window.keyboard.press('ArrowUp');
  const dock = window.locator('[data-testid="operations-panel"]');
  await expect(window.locator('[data-testid="op-row"]')).toHaveCount(2);
  await expect(dock).toHaveScreenshot('operations-list.png');
  await window.locator('[data-testid="op-row"][data-op-id="op-b"]').click();
  await expect(window.locator('[data-testid="op-detail"]')).toBeVisible();
  await expect(dock).toHaveScreenshot('operations-detail.png');
});
