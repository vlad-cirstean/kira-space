import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { connectAndExpand, connectionCreateArgs } from './support/connect';
import { gridCell } from './support/grid';
import {
  COMPOSITE_PK_META,
  COMPOSITE_PK_PATH,
  compositePkConnectAndOpen,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { findRow } from './support/tree';
import { wireOps } from './support/wireOps';

// J-GRID-020 and J-BC-051, UI half. Table with a quote-needing column name and a non-ASCII one.
// The Go half (TestGridMutateChangeSet) commits the same typed plan and proves the rows with a
// second client; here the grid must send exactly those ops, show the contract's refusal text on a
// failed commit, and keep the staged change set.

const CONNECTION_ID = 'conn-grid-roundtrip';
const COLUMN_NAMES = ['id', 'select', 'ünï', 'note'];
const TYPED = wireOps(
  contract<{ ops: unknown[] }>('studio-grid-mutate', 'mutate:typed-ops').ops,
  COLUMN_NAMES,
);
const REFUSAL = contract<{ Code: string; Message: string }>('studio-grid-mutate', 'mutate:error');

const COLUMNS = [
  ['id', 'integer', 'number', false, true],
  ['select', 'text', 'text', true, false],
  ['ünï', 'integer', 'number', true, false],
  ['note', 'text', 'text', true, false],
].map(([name, dataType, typeClass, nullable, isPrimaryKey]) => ({
  name: name as string,
  dataType: dataType as string,
  typeClass: typeClass as 'number' | 'text',
  nullable: nullable as boolean,
  isPrimaryKey: isPrimaryKey as boolean,
  generated: false,
}));

const META = {
  ...COMPOSITE_PK_META,
  name: 'we"ird t',
  qualifiedName: 'app."we""ird t"',
  columns: COLUMNS.map((c, i) => ({
    name: c.name,
    position: i + 1,
    dataType: c.dataType,
    nullable: c.nullable,
    defaultExpr: null,
    isPrimaryKey: c.isPrimaryKey,
    comment: null,
  })),
  primaryKey: ['id'],
};

const FIXTURE = compositePkConnectAndOpen(CONNECTION_ID);
const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Roundtrip DB', 'green'),
    response: postgresConnectionSummary(CONNECTION_ID, 'Roundtrip DB', 'green'),
  },
  ...FIXTURE.control.filter((c) => c.channel !== IPC.treeDescribe),
  {
    channel: IPC.treeDescribe,
    args: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, refresh: false, tabId: null },
    response: { meta: META, source: 'server' },
  },
];

const POSITION = {
  offset: 0,
  pageSize: 100,
  hasMore: false,
  nextToken: null,
  prevToken: null,
  strategy: 'keyset' as const,
};
const READ_PAYLOAD = {
  connectionId: CONNECTION_ID,
  path: COMPOSITE_PK_PATH,
  projection: null,
  filter: null,
  sort: null,
  pageSize: 100,
  cursor: { mode: 'offset', offset: 0 },
};
const readSnap = (rows: (string | null)[][]): PortSnapshot => ({
  op: DATA_OP.read,
  payload: READ_PAYLOAD,
  response: {
    kind: 'read',
    page: { kind: 'tabular', columns: COLUMNS, rows, position: POSITION, truncatedCells: 0 },
    source: 'server',
  },
});

const MUTATE_PAYLOAD = { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, ops: TYPED };

test('a typed change set sends the contract ops; a refusal keeps it staged', async ({
  relaunch,
}) => {
  const { window: page, stream } = await relaunch({
    control: CONTROL,
    stream: [
      readSnap([['9', 'again', '3', null]]),
      {
        op: DATA_OP.mutate,
        payload: MUTATE_PAYLOAD,
        error: { code: REFUSAL.Code, message: REFUSAL.Message },
      },
      {
        op: DATA_OP.mutate,
        payload: MUTATE_PAYLOAD,
        response: { kind: 'mutate', affectedRows: 2 },
      },
      {
        op: DATA_OP.invalidate,
        payload: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, scope: 'pages' },
        response: { kind: 'invalidate' },
      },
      readSnap([
        ['5', 'ins', '7', 'typed note'],
        ['9', `x'y"z`, null, null],
      ]),
    ],
  });
  await connectAndExpand(page, 'Roundtrip DB', 'green');
  await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
  await expect(gridCell(page, 0, 'select')).toHaveText('again');

  await gridCell(page, 0, 'select').dblclick();
  await page.locator('[data-testid="grid-cell-input"]').fill(`x'y"z`);
  await page.keyboard.press('Enter');
  await gridCell(page, 0, 'ünï').click();
  await page.keyboard.press('Delete');
  await expect(gridCell(page, 0, 'ünï')).toHaveClass(/pending-edit/);

  await page.click('[data-testid="toolbar-add-row"]');
  const insertInputs = page.locator(
    '[data-testid="grid-row-insert"] [data-testid="grid-cell-insert"] input',
  );
  await insertInputs.nth(0).fill('5');
  await insertInputs.nth(1).fill('ins');
  await insertInputs.nth(2).fill('7');
  await insertInputs.nth(3).fill('typed note');

  await page.click('[data-testid="toolbar-commit-changes"]');
  await expect(page.locator('[data-testid="data-action-error"]')).toContainText(REFUSAL.Message);
  // The refused plan stays staged; committing again sends the same ops.
  await expect(gridCell(page, 0, 'select')).toHaveClass(/pending-edit/);
  await expect(page.locator('[data-testid="grid-row-insert"]')).toHaveCount(1);

  await page.click('[data-testid="toolbar-commit-changes"]');
  await expect(gridCell(page, 0, 'select')).toHaveText('ins', {
    timeout: 15_000,
  });
  await expect(page.locator('[data-testid="toolbar-commit-changes"]')).toHaveCount(0);
  const sent = (await stream.ops()).filter((o) => o.op === DATA_OP.mutate);
  expect(sent).toHaveLength(2);
  for (const call of sent) expect((call.payload as { ops: unknown[] }).ops).toEqual(TYPED);
});
