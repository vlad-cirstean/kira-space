import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { connectAndExpand, connectionCreateArgs, openConsoleFromMenu } from './support/connect';
import { executeSnap, numberPage } from './support/consolePages';
import { typeInto } from './support/editor';
import {
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';

// An op id is a UUIDv7 minted by the renderer for a console run, and absent from the requests Go
// mints for. Backend half: dbmcpflow TestOpIDs and TestOpIDGuard.

const V7 = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const ID = 'conn-op-ids';

const [recorded] = contract<Record<string, unknown>[]>('op-id-guard', 'OpsService.Recent');

test('a console run sends a v7 opId; a v4 op row still shows in the widget', async ({
  relaunch,
}) => {
  const summary = postgresConnectionSummary(ID, 'Op DB', 'green');
  const fixture = orderItemsFixture(ID);
  const legacy = {
    ...recorded,
    id: '3f2504e0-4f89-41d3-9a0c-0305e82c3301',
    status: 'running',
    cancellable: true,
    connectionId: ID,
    startedAt: new Date().toISOString(),
  };
  const control: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: connectionCreateArgs('Op DB', 'green'),
      response: summary,
    },
    ...fixture.control,
    { channel: IPC.opsRecent, response: [legacy] },
    { channel: IPC.opsCancel, response: null },
  ];
  const port: PortSnapshot[] = [
    ...fixture.port,
    executeSnap(ID, ['SELECT 10 AS a'], [numberPage('a', '10')]),
  ];
  const { window: page, stream, control: log } = await relaunch({ control, stream: port });
  await connectAndExpand(page, 'Op DB', 'green');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await typeInto(view, page, 'SELECT 10 AS a;');
  await page.click('[data-testid="console-run-statement"]');
  await expect(view.locator('[data-testid="console-result-grid"]')).toHaveCount(1);
  const runs = (await stream.ops()).filter((o) => o.op === DATA_OP.execute);
  expect(runs).toHaveLength(1);
  expect(runs[0]?.opId).toMatch(V7);

  const row = page.locator(
    '[data-testid="status-ops-row"][data-op-id="3f2504e0-4f89-41d3-9a0c-0305e82c3301"]',
  );
  await page.locator('[data-testid="status-ops"]').click();
  await row.locator('[data-testid="status-ops-stop"]').click();
  await expect
    .poll(() => log.log().find((e) => e.channel === IPC.opsCancel)?.args)
    .toEqual({ opId: '3f2504e0-4f89-41d3-9a0c-0305e82c3301' });
});
