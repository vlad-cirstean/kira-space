import { expect, test } from './fixtures';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// Status bar running-ops widget. Backend half: appflow TestOpsLogAndCancel (cancel path and the
// cancellable flag).

function opRecord(overrides: Record<string, unknown>): Record<string, unknown> {
  return {
    id: 'op-1',
    startedAt: new Date().toISOString(),
    durationMs: null,
    kind: 'push',
    status: 'running',
    command: 'git push',
    error: null,
    category: 'git',
    module: 'git',
    trigger: 'user',
    subject: 'origin',
    repoRoot: '/work/alpha',
    repoName: 'alpha',
    source: 'Kira Space',
    taskId: '',
    parentId: '',
    background: false,
    repeat: 1,
    steps: [],
    stepsDropped: 0,
    cancellable: true,
    ...overrides,
  };
}

const CONTROL: ControlSnapshot[] = [
  {
    channel: IPC.opsRecent,
    response: [
      opRecord({ id: 'op-push' }),
      opRecord({ id: 'op-merge', kind: 'merge', cancellable: false }),
      opRecord({ id: 'op-bg', kind: 'status', background: true }),
    ],
  },
  { channel: IPC.opsCancel, response: null },
];

test('a cancellable op stops through OpsService.Cancel; a fixed one cannot; background rows are not listed', async ({
  relaunch,
}) => {
  const { window, control } = await relaunch({ control: CONTROL });
  const item = window.locator('[data-testid="status-ops"]');
  await expect(item).toContainText('2 running');
  await item.click();
  const rows = window.locator('[data-testid="status-ops-row"]');
  await expect(rows).toHaveCount(2);
  await expect(window.locator('[data-op-id="op-bg"]')).toHaveCount(0);

  await expect(
    rows.filter({ hasText: 'merge' }).locator('[data-testid="status-ops-stop"]'),
  ).toBeDisabled();
  await rows.filter({ hasText: 'push' }).locator('[data-testid="status-ops-stop"]').click();
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.opsCancel))
    .toEqual([{ channel: IPC.opsCancel, args: { opId: 'op-push' } }]);
});
