import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { connectAndExpand, connectionCreateArgs, openConsoleFromMenu } from './support/connect';
import { contract } from './support/contract';
import { typeInto } from './support/editor';
import { IPC } from './support/ipcChannels';
import {
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';

// A malformed op id is refused with E_QUERY and leaves no row; a reused id keeps the first op's
// history row. Contract op-id-guard. Backend half: dbmcpflow TestOpIDGuard.

const [recorded] = contract<Record<string, unknown>[]>('op-id-guard', 'OpsService.Recent');
const ID = 'conn-op-guard';

test('a refused op shows the error, leaves no running row, and history keeps the first op', async ({
  relaunch,
}) => {
  const summary = postgresConnectionSummary(ID, 'Guard DB', 'green');
  const fixture = orderItemsFixture(ID);
  const history = { ...recorded, id: '0192f0a8-1b2c-7000-8000-00000000aaaa', connectionId: ID };
  const control: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: connectionCreateArgs('Guard DB', 'green'),
      response: summary,
    },
    ...fixture.control,
    { channel: IPC.opsRecent, response: [history] },
  ];
  const port: PortSnapshot[] = [
    ...fixture.port,
    {
      op: DATA_OP.execute,
      payload: { connectionId: ID, path: ORDER_ITEMS_PATH, statements: ['SELECT 1'] },
      error: { code: 'E_QUERY', message: 'The operation ID is not valid.' },
    },
  ];
  const { window: page } = await relaunch({ control, stream: port });
  await connectAndExpand(page, 'Guard DB', 'green');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await typeInto(view, page, 'SELECT 1;');
  await page.click('[data-testid="console-run-all"]');
  await expect(view.locator('[data-testid="console-error"]')).toContainText(
    'The operation ID is not valid.',
  );
  await expect(page.locator('[data-testid="status-ops"]')).toHaveCount(0);

  await page.click('[data-testid="toggle-operations-panel"]');
  const rows = page.locator('[data-testid="op-row"]');
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText(String(recorded?.command));
});
