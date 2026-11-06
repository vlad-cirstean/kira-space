import { scriptRunSchema } from '@shared/domain/scriptRuns';
import { customScriptSchema } from '@shared/domain/scripts';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { emitWailsEvent } from '../support/mockRuntime';
import { postgresConnectionSummary } from './support/postgresFixture';

// Status bar running-ops widget. Backend half: dbmcpflow TestDbMcpCancelOperation (the running op
// row) and TestDbMcpCancelScriptRun (the stopped run).

const [op] = contract<Record<string, unknown>[]>('ops-running', 'StudioOpsService.Recent');
const OP = { ...op, id: 'op-live', connectionId: 'conn-1', startedAt: new Date().toISOString() };
const script = contract('automations', 'CustomScriptsService.Create#failed', {
  schema: customScriptSchema,
});
const failed = contract('automations', 'ScriptRunsService.Get#failed', { schema: scriptRunSchema });

function run(state: string) {
  const now = Date.now();
  const live = state === 'running';
  return {
    ...failed,
    id: 'run-live',
    state,
    terminalId: '',
    createdAt: now,
    startedAt: now,
    finishedAt: live ? null : now + 1000,
    outcome: live
      ? null
      : { status: state, reason: 'stopped by you', source: 'user', reported: false },
  };
}

const CONTROL: ControlSnapshot[] = [
  {
    channel: IPC.connectionsList,
    response: [postgresConnectionSummary('conn-1', 'Warehouse DB', 'blue')],
  },
  { channel: IPC.studioOpsRecent, response: [OP] },
  { channel: IPC.studioOpsCancel, response: null },
  { channel: IPC.customScriptsList, response: { collections: [], scripts: [script] } },
  { channel: IPC.scriptRunsList, response: [run('running')] },
  { channel: IPC.scriptRunsStop, response: null },
];

test('contract: the widget lists a running op and run; Stop sends the cancel and the item follows', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: CONTROL });
  const item = page.locator('[data-testid="status-ops"]');
  await expect(item).toContainText('2 running');
  await item.click();
  const rows = page.locator('[data-testid="status-ops-row"]');
  await expect(rows).toHaveCount(2);

  const opRow = page.locator('[data-testid="status-ops-row"][data-op-id="op-live"]');
  await expect(opRow).toContainText('Warehouse DB');
  await opRow.locator('[data-testid="status-ops-stop"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.studioOpsCancel)?.args)
    .toEqual({ opId: 'op-live' });
  await emitWailsEvent(page, IPC.studioOpUpdate, { ...OP, status: 'cancelled', durationMs: 5 });
  await expect(rows).toHaveCount(1);
  await expect(item).toContainText('1 running');

  const runRow = page.locator('[data-testid="status-ops-row"][data-op-id="run:run-live"]');
  await runRow.locator('[data-testid="status-ops-stop"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.scriptRunsStop)?.args)
    .toEqual({ id: 'run-live' });
  await emitWailsEvent(page, IPC.scriptRunsChanged, run('cancelled'));
  await expect(item).toHaveCount(0);
});

test('contract: a panel filter that matches nothing leaves the widget count unchanged', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await page.click('[data-testid="toggle-operations-panel"]');
  await page.locator('[data-testid="ops-filter"]').fill('no-such-text');
  await expect(page.locator('[data-testid="status-ops"]')).toContainText('2 running');
});

test('contract: Show operations opens the Operations dock', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await page.locator('[data-testid="status-ops"]').click();
  await page.locator('[data-testid="status-ops-open"]').click();
  await expect(page.locator('[data-testid="operations-panel"]')).toBeVisible();
});
