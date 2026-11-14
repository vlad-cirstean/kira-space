import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { connectAndExpand, connectionCreateArgs } from './support/connect';
import { gridCell } from './support/grid';
import {
  COMPOSITE_PK_PATH,
  compositePkConnectAndOpen,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { findRow } from './support/tree';
import { wireOps } from './support/wireOps';

// Decision 2, UI half of dbflow TestGridLostAck (contract studio-grid-lost-ack). The connection
// dropped between COMMIT and its acknowledgement: the grid shows the contract's text, which says
// the outcome is unknown, and keeps the change set staged. Nothing is retried or dropped on its own.

const CONNECTION_ID = 'conn-grid-lost-ack';
const LOST = contract<{ Code: string; Message: string }>('studio-grid-lost-ack', 'mutate:lostAck');
const OPS = wireOps(contract<{ ops: unknown[] }>('studio-grid-stale', 'mutate:guarded-ops').ops, [
  'tenant_id',
  'entity_id',
  'name',
]);

const FIXTURE = compositePkConnectAndOpen(CONNECTION_ID);
const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Lost ack DB', 'green'),
    response: postgresConnectionSummary(CONNECTION_ID, 'Lost ack DB', 'green'),
  },
  ...FIXTURE.control,
];

test('a commit whose acknowledgement was lost reports the unknown outcome and keeps the edit staged', async ({
  relaunch,
}) => {
  const { window: page, stream } = await relaunch({
    control: CONTROL,
    stream: [
      ...FIXTURE.port,
      {
        op: DATA_OP.mutate,
        payload: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, ops: OPS },
        error: { code: LOST.Code, message: LOST.Message },
      },
    ],
  });
  await connectAndExpand(page, 'Lost ack DB', 'green');
  await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
  await expect(gridCell(page, 0, 'name')).toBeVisible();

  await gridCell(page, 0, 'name').dblclick();
  await page.locator('[data-testid="grid-cell-input"]').fill('mine');
  await page.keyboard.press('Enter');
  await page.click('[data-testid="toolbar-commit-changes"]');

  const error = page.locator('[data-testid="data-action-error"]');
  await expect(error).toContainText('may or may not be saved');
  await expect(error).toContainText(LOST.Message);
  await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);
  await expect(gridCell(page, 0, 'name')).toHaveText('mine');
  await expect(page.locator('[data-testid="toolbar-commit-changes"]')).toBeVisible();
  expect((await stream.ops()).filter((o) => o.op === DATA_OP.mutate)).toHaveLength(1);
});
