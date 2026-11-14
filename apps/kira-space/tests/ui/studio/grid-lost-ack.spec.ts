import type { Page } from '@playwright/test';
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

// Decision 2, UI half of dbflow TestGridLostAck (contract studio-grid-lost-ack). The connection
// dropped between COMMIT and its acknowledgement: the error carries details.outcomeUnknown, the grid
// shows the contract's text and reloads the table, then settles its staged set against it. A staged
// edit the table already holds is cleared; one it does not hold stays staged. Rows the reload cannot
// match (a keyless insert, which a blind resend would duplicate) are put to the user, and Commit
// never resends before that is settled.

const CONNECTION_ID = 'conn-grid-lost-ack';
const LOST = contract<{ Code: string; Message: string; Details: string }>(
  'studio-grid-lost-ack',
  'mutate:lostAck',
);
const COLUMNS = ['tenant_id', 'entity_id', 'name'];
const EDIT_OPS = wireOps(
  contract<{ ops: unknown[] }>('studio-grid-stale', 'mutate:guarded-ops').ops,
  COLUMNS,
);
const INSERT_OPS = wireOps(
  [{ kind: 'insert', values: { tenant_id: null, entity_id: null, name: 'serial' } }],
  COLUMNS,
);

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

const ORIGINAL = [
  ['1', '1', 'tenant 1 / entity 1'],
  ['1', '2', 'tenant 1 / entity 2'],
  ['2', '1', 'tenant 2 / entity 1'],
];
const POSITION = {
  offset: 0,
  pageSize: 100,
  hasMore: false,
  nextToken: null,
  prevToken: null,
  strategy: 'keyset' as const,
};

const lostAck = (ops: unknown[]): PortSnapshot => ({
  op: DATA_OP.mutate,
  payload: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, ops },
  error: { code: LOST.Code, message: LOST.Message, details: LOST.Details },
});
const invalidated: PortSnapshot = {
  op: DATA_OP.invalidate,
  payload: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, scope: 'pages' },
  response: { kind: 'invalidate' },
};
const unreachable: PortSnapshot = {
  ...invalidated,
  response: undefined,
  error: { code: 'E_CONNECT', message: 'connection refused' },
};
// The second read of the table: what the server holds after the lost acknowledgement.
const reloaded = (rows: string[][]): PortSnapshot => ({
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
      rows,
      position: POSITION,
      truncatedCells: 0,
    },
    source: 'server',
  },
});

const COMMIT = '[data-testid="toolbar-commit-changes"]';
const DIALOG = '[data-testid="confirm-dialog"]';

async function openGrid(page: Page): Promise<void> {
  await connectAndExpand(page, 'Lost ack DB', 'green');
  await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
  await expect(gridCell(page, 0, 'name')).toBeVisible();
}

async function stageEdit(page: Page): Promise<void> {
  await gridCell(page, 0, 'name').dblclick();
  await page.locator('[data-testid="grid-cell-input"]').fill('mine');
  await page.keyboard.press('Enter');
  await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);
}

async function stageInsert(page: Page): Promise<void> {
  await page.click('[data-testid="toolbar-add-row"]');
  await page
    .locator('[data-testid="grid-row-insert"] [data-testid="grid-cell-insert"] input')
    .nth(2)
    .fill('serial');
}

async function expectUnknownOutcomeShown(page: Page): Promise<void> {
  const error = page.locator('[data-testid="data-action-error"]');
  await expect(error).toContainText('may or may not be saved');
  await expect(error).toContainText(LOST.Message);
}

const sentMutates = async (stream: { ops(): Promise<{ op: string }[]> }) =>
  (await stream.ops()).filter((o) => o.op === DATA_OP.mutate);

test('a lost acknowledgement whose commit the table already holds clears the staged edit', async ({
  relaunch,
}) => {
  const { window: page, stream } = await relaunch({
    control: CONTROL,
    stream: [
      ...FIXTURE.port,
      lostAck(EDIT_OPS),
      invalidated,
      reloaded([['1', '1', 'mine'], ORIGINAL[1], ORIGINAL[2]]),
    ],
  });
  await openGrid(page);
  await stageEdit(page);
  await page.click(COMMIT);

  await expectUnknownOutcomeShown(page);
  await expect(gridCell(page, 0, 'name')).toHaveText('mine');
  await expect(gridCell(page, 0, 'name')).not.toHaveClass(/pending-edit/);
  await expect(page.locator(COMMIT)).toHaveCount(0);
  await expect(page.locator(DIALOG)).toHaveCount(0);
  expect(await sentMutates(stream)).toHaveLength(1);
});

test('a lost acknowledgement whose commit the table does not hold keeps the staged edit', async ({
  relaunch,
}) => {
  const { window: page, stream } = await relaunch({
    control: CONTROL,
    stream: [...FIXTURE.port, lostAck(EDIT_OPS), invalidated, reloaded(ORIGINAL)],
  });
  await openGrid(page);
  await stageEdit(page);
  await page.click(COMMIT);

  await expectUnknownOutcomeShown(page);
  await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);
  await expect(gridCell(page, 0, 'name')).toHaveText('mine');
  await expect(page.locator(COMMIT)).toBeVisible();
  await expect(page.locator(DIALOG)).toHaveCount(0);
  expect(await sentMutates(stream)).toHaveLength(1);
});

test('a staged insert the reload cannot match is put to the user; keeping it resends nothing', async ({
  relaunch,
}) => {
  const { window: page, stream } = await relaunch({
    control: CONTROL,
    stream: [...FIXTURE.port, lostAck(INSERT_OPS), invalidated, reloaded(ORIGINAL)],
  });
  await openGrid(page);
  await stageInsert(page);
  await page.click(COMMIT);

  const dialog = page.locator(DIALOG);
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText('may have been saved');
  await page.click('[data-testid="confirm-dialog-cancel"]');
  await expectUnknownOutcomeShown(page);
  await expect(page.locator('[data-testid="grid-row-insert"]')).toHaveCount(1);
  await expect(page.locator(COMMIT)).toBeVisible();
  expect(await sentMutates(stream)).toHaveLength(1);
});

test('discarding the unmatched insert after a lost acknowledgement clears it', async ({
  relaunch,
}) => {
  const { window: page, stream } = await relaunch({
    control: CONTROL,
    stream: [...FIXTURE.port, lostAck(INSERT_OPS), invalidated, reloaded(ORIGINAL)],
  });
  await openGrid(page);
  await stageInsert(page);
  await page.click(COMMIT);

  await expect(page.locator(DIALOG)).toBeVisible();
  await page.click('[data-testid="confirm-dialog-confirm"]');
  await expectUnknownOutcomeShown(page);
  await expect(page.locator('[data-testid="grid-row-insert"]')).toHaveCount(0);
  await expect(page.locator(COMMIT)).toHaveCount(0);
  expect(await sentMutates(stream)).toHaveLength(1);
});

test('Commit after a failed reload settles first and never resends the insert', async ({
  relaunch,
}) => {
  const { window: page, stream } = await relaunch({
    control: CONTROL,
    stream: [...FIXTURE.port, lostAck(INSERT_OPS), unreachable, invalidated, reloaded(ORIGINAL)],
  });
  await openGrid(page);
  await stageInsert(page);
  await page.click(COMMIT);

  const error = page.locator('[data-testid="data-action-error"]');
  await expect(error).toContainText('couldn’t be reloaded');
  await expect(page.locator('[data-testid="grid-row-insert"]')).toHaveCount(1);

  // This Commit only settles the earlier one: it asks, keeps the insert and sends nothing.
  await page.click(COMMIT);
  await expect(page.locator(DIALOG)).toBeVisible();
  await page.click('[data-testid="confirm-dialog-cancel"]');
  await expect(page.locator('[data-testid="grid-row-insert"]')).toHaveCount(1);
  expect(await sentMutates(stream)).toHaveLength(1);
});
