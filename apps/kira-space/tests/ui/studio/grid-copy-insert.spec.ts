import type { Page } from '@playwright/test';
import type { ColumnDescriptor } from '@shared/protocol/page';
import type { ControlSnapshot, LogicalPage, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { clipboardRead, clipboardReset, prepareRealClipboard } from './support/clipboard';
import { connectAndExpand, connectionCreateArgs } from './support/connect';
import { gridCell, gutterCell } from './support/grid';
import {
  COMPOSITE_PK_PATH,
  compositePkConnectAndOpen,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { findRow } from './support/tree';

// J-GRID-015, UI half. The Go half (dbflow TestGridCopyInsertReplayPostgres) reads a typed table
// from a real Postgres, executes the INSERT below into an empty clone and proves source and clone
// match. This half replays those columns and rows and asserts Copy as INSERT writes the same bytes,
// bytea included (a quoted "0x.." would store the characters, not the bytes).

const CONNECTION_ID = 'conn-grid-copy-insert';
const COLUMNS = contract<ColumnDescriptor[]>('studio-copy-insert', 'columns');
const ROWS = contract<(string | null)[][]>('studio-copy-insert', 'rows');
const INSERT = contract<string>('studio-copy-insert', 'copy:insert');

const FIXTURE = compositePkConnectAndOpen(CONNECTION_ID);
const PAGE: LogicalPage = {
  kind: 'tabular',
  columns: COLUMNS,
  rows: ROWS,
  position: {
    offset: 0,
    pageSize: 100,
    hasMore: false,
    nextToken: null,
    prevToken: null,
    strategy: 'keyset',
  },
  truncatedCells: 0,
};
const PORT: PortSnapshot[] = [
  { ...FIXTURE.port[0], response: { kind: 'read', page: PAGE, source: 'server' } },
];
const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Insert DB', 'green'),
    response: postgresConnectionSummary(CONNECTION_ID, 'Insert DB', 'green'),
  },
  ...FIXTURE.control,
];

async function copyInsert(page: Page, browserName: string): Promise<string> {
  await clipboardReset(page, browserName);
  await gutterCell(page, 1).click({ button: 'right' });
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await page.locator('[data-testid="menu-item-copy-rows"]').hover();
  await expect(page.locator('[data-testid="context-submenu"]')).toBeVisible();
  await page.click('[data-testid="menu-item-copy-rows-insert"]');
  return clipboardRead(page, browserName);
}

test('Copy as INSERT writes the contract statement for every type', async ({
  relaunch,
  browserName,
}) => {
  test.setTimeout(120_000);
  const { window: page } = await relaunch({
    control: CONTROL,
    stream: PORT,
    prepare: (p) => prepareRealClipboard(p, browserName),
  });
  await connectAndExpand(page, 'Insert DB', 'green');
  await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
  await expect(gridCell(page, 0, 'id')).toBeVisible();

  await gutterCell(page, 0).click();
  await gutterCell(page, ROWS.length - 1).click({ modifiers: ['Shift'] });
  expect(await copyInsert(page, browserName)).toBe(INSERT);
});
