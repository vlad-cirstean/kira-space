import type { ControlSnapshot } from '../ipc/support/types';
import { expect, studioTest as test } from './fixtures';
import { postgresConnectionSummary } from './studio/support/postgresFixture';
import { openSettings } from './studio/support/settings';
import { dbOp } from './support/dbOps';
import { IPC } from './support/ipcChannels';

// P544: one settings dialog and one Operations panel serve the git, database, API and Kubernetes
// modules. Backend half: appflow TestSettingsCarryDatabaseSections and dbmcpflow TestDbMcpCancelOperation.

const NOW = '2026-01-01T00:00:00.000Z';

const GIT_OP = {
  id: 'op-git',
  startedAt: NOW,
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
};

function dbRecord(id: string, connectionId: string, command: string) {
  return dbOp({
    id,
    connectionId,
    tabId: null,
    startedAt: NOW,
    durationMs: 5,
    kind: 'read',
    status: 'ok',
    rows: 3,
    command,
    error: null,
    source: 'user',
  });
}

test('the Advanced pane saves the op log retention through the one settings call', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch();
  await openSettings(page);
  await page.click('[data-testid="settings-section-Advanced"]');
  await page.locator('[data-testid="settings-oplog-retention"]').fill('7');
  await page.click('[data-testid="settings-save"]');

  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.settingsSet))
    .toHaveLength(1);
  const call = control.log().find((e) => e.channel === IPC.settingsSet);
  expect(call?.args).toEqual({ patch: { advanced: { opLogRetentionDays: 7 } } });
});

test('one Operations panel lists git and database ops, filtered by connection name', async ({
  relaunch,
}) => {
  const warehouse = postgresConnectionSummary('conn-w', 'Warehouse DB', 'blue');
  const analytics = postgresConnectionSummary('conn-a', 'Analytics DB', 'green');
  const control: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [warehouse, analytics] },
    {
      channel: IPC.opsRecent,
      response: [
        GIT_OP,
        dbRecord('op-w', 'conn-w', 'select 1'),
        dbRecord('op-a', 'conn-a', 'select 2'),
      ],
    },
  ];
  const { window: page } = await relaunch({ control });

  await page.click('[data-testid="toggle-operations-panel"]');
  const rows = page.locator('[data-testid="op-row"]');
  await expect(rows).toHaveCount(3);

  await page.locator('[data-testid="ops-filter"]').fill('Warehouse');
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText('select 1');
  await expect(rows.first()).toContainText('Warehouse DB');
});
