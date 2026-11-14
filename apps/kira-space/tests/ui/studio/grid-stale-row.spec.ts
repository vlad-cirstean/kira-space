import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { connectAndExpand, connectionCreateArgs } from './support/connect';
import { gridCell } from './support/grid';
import {
  COMPOSITE_PK_COLUMNS,
  COMPOSITE_PK_PATH,
  compositePkConnectAndOpen,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { findRow } from './support/tree';
import { wireOps } from './support/wireOps';

// J-GRID-040 + decision 3, UI half. The Go half (dbflow TestGridStaleRow) shows the engine refuses a
// guarded update of a row another client changed (E_CONFLICT naming the row) and lands the same plan
// without the guard. This half replays those exact ops and errors: the grid sends the loaded value
// with the edit, asks before overwriting, sends the unguarded plan only after Overwrite, and keeps
// the staged edit on Cancel.

const CONNECTION_ID = 'conn-grid-stale';
const COLUMNS = ['tenant_id', 'entity_id', 'name'];
const GUARDED = wireOps(
  contract<{ ops: unknown[] }>('studio-grid-stale', 'mutate:guarded-ops').ops,
  COLUMNS,
);
const OVERWRITE = wireOps(
  contract<{ ops: unknown[] }>('studio-grid-stale', 'mutate:overwrite-ops').ops,
  COLUMNS,
);
const CONFLICT = contract<{ Code: string; Message: string }>(
  'studio-grid-stale',
  'mutate:conflict',
);

const FIXTURE = compositePkConnectAndOpen(CONNECTION_ID);
const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Stale DB', 'green'),
    response: postgresConnectionSummary(CONNECTION_ID, 'Stale DB', 'green'),
  },
  ...FIXTURE.control,
];

const CONFLICT_MUTATE: PortSnapshot = {
  op: DATA_OP.mutate,
  payload: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, ops: GUARDED },
  error: { code: CONFLICT.Code, message: CONFLICT.Message },
};
const OVERWRITE_PORT: PortSnapshot[] = [
  {
    op: DATA_OP.mutate,
    payload: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, ops: OVERWRITE },
    response: { kind: 'mutate', affectedRows: 1 },
  },
  {
    op: DATA_OP.invalidate,
    payload: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, scope: 'pages' },
    response: { kind: 'invalidate' },
  },
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: COMPOSITE_PK_PATH,
      projection: null,
      filter: null,
      sort: null,
      pageSize: 100,
      cursor: { mode: 'offset', offset: 0 },
    },
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: COMPOSITE_PK_COLUMNS,
        rows: [
          ['1', '1', 'mine'],
          ['1', '2', 'tenant 1 / entity 2'],
          ['2', '1', 'tenant 2 / entity 1'],
        ],
        position: {
          offset: 0,
          pageSize: 100,
          hasMore: false,
          nextToken: null,
          prevToken: null,
          strategy: 'keyset',
        },
        truncatedCells: 0,
      },
      source: 'server',
    },
  },
];

async function stageAndCommit(page: import('@playwright/test').Page): Promise<void> {
  await connectAndExpand(page, 'Stale DB', 'green');
  await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
  await expect(gridCell(page, 0, 'name')).toBeVisible();
  await gridCell(page, 0, 'name').dblclick();
  await page.locator('[data-testid="grid-cell-input"]').fill('mine');
  await page.keyboard.press('Enter');
  await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);
  await page.click('[data-testid="toolbar-commit-changes"]');
  const dialog = page.locator('[data-testid="confirm-dialog"]');
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText(CONFLICT.Message);
}

test('a row changed on the server asks before it is overwritten', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: CONTROL,
    stream: [...FIXTURE.port, CONFLICT_MUTATE, ...OVERWRITE_PORT],
  });
  await stageAndCommit(page);
  await page.click('[data-testid="confirm-dialog-confirm"]');
  // The mutate snapshot for the unguarded plan answers only after the confirm.
  await expect(gridCell(page, 0, 'name')).toHaveText('mine', { timeout: 15_000 });
  await expect(page.locator('[data-testid="toolbar-commit-changes"]')).toHaveCount(0, {
    timeout: 15_000,
  });
});

test('cancelling the overwrite keeps the staged edit and sends nothing more', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: CONTROL,
    stream: [...FIXTURE.port, CONFLICT_MUTATE],
  });
  await stageAndCommit(page);
  await page.click('[data-testid="confirm-dialog-cancel"]');
  await expect(page.locator('[data-testid="confirm-dialog"]')).toHaveCount(0);
  await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);
  await expect(page.locator('[data-testid="toolbar-commit-changes"]')).toBeVisible();
  await expect(page.locator('[data-testid="toolbar-commit-changes"]')).toBeEnabled();
  await expect(page.locator('[data-testid="data-action-error"]')).toHaveCount(0);
});
