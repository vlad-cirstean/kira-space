import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { connectAndExpand, connectionCreateArgs } from './support/connect';
import { gridCell, gutterCell, headerCell } from './support/grid';
import {
  COMPOSITE_PK_COLUMNS,
  COMPOSITE_PK_PATH,
  compositePkConnectAndOpen,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { findRow } from './support/tree';

// J-GRID-021: a staged edit is keyed by the row's primary key, so sorting and Refresh move it with
// its row; reverting one cell, one row and everything each clears exactly its own scope.

const CONNECTION_ID = 'conn-grid-survive';
const FIXTURE = compositePkConnectAndOpen(CONNECTION_ID);
const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Survive DB', 'green'),
    response: postgresConnectionSummary(CONNECTION_ID, 'Survive DB', 'green'),
  },
  ...FIXTURE.control,
];

const POSITION = {
  offset: 0,
  pageSize: 100,
  hasMore: false,
  nextToken: null,
  prevToken: null,
  strategy: 'keyset' as const,
};
const SORTED = { kind: 'structured', terms: [{ column: 'entity_id', direction: 'desc' }] };
const SORTED_ROWS = [
  ['1', '2', 'tenant 1 / entity 2'],
  ['1', '1', 'tenant 1 / entity 1'],
  ['2', '1', 'tenant 2 / entity 1'],
];
const sortedRead: PortSnapshot = {
  op: DATA_OP.read,
  payload: {
    connectionId: CONNECTION_ID,
    path: COMPOSITE_PK_PATH,
    projection: null,
    filter: null,
    sort: SORTED,
    pageSize: 100,
    cursor: { mode: 'offset', offset: 0 },
  },
  response: {
    kind: 'read',
    page: {
      kind: 'tabular',
      columns: COMPOSITE_PK_COLUMNS,
      rows: SORTED_ROWS,
      position: POSITION,
      truncatedCells: 0,
    },
    source: 'server',
  },
};

test('staged edits follow their row through sort and refresh, and revert by scope', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: CONTROL,
    stream: [
      ...FIXTURE.port,
      sortedRead,
      {
        op: DATA_OP.invalidate,
        payload: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH },
        response: { kind: 'invalidate' },
      },
      sortedRead,
    ],
  });
  await connectAndExpand(page, 'Survive DB', 'green');
  await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
  await expect(gridCell(page, 0, 'name')).toHaveText('tenant 1 / entity 1');

  // Stage on (1,1): sits on page row 0 unsorted, row 1 once sorted by entity_id descending.
  await gridCell(page, 0, 'name').dblclick();
  await page.locator('[data-testid="grid-cell-input"]').fill('edited');
  await page.keyboard.press('Enter');
  await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);

  await headerCell(page, 'entity_id').click({ button: 'right' });
  await page.click('[data-testid="menu-item-sort-desc"]');
  await expect(gridCell(page, 0, 'name')).toHaveText('tenant 1 / entity 2');
  await expect(gridCell(page, 0, 'name')).not.toHaveClass(/pending-edit/);
  await expect(gridCell(page, 1, 'name')).toHaveText('edited');
  await expect(gridCell(page, 1, 'name')).toHaveClass(/pending-edit/);

  await page.click('[data-testid="toolbar-refresh"]');
  await expect(gridCell(page, 1, 'name')).toHaveClass(/pending-edit/);
  await expect(gridCell(page, 1, 'name')).toHaveText('edited');

  // Reverting one cell is editing it back to the loaded value: nothing stays staged.
  await gridCell(page, 1, 'name').dblclick();
  await page.locator('[data-testid="grid-cell-input"]').fill('tenant 1 / entity 1');
  await page.keyboard.press('Enter');
  await expect(page.locator('[data-testid="toolbar-commit-changes"]')).toHaveCount(0);
  await expect(gridCell(page, 1, 'name')).toHaveText('tenant 1 / entity 1');

  // Revert the row: stage two cells on it, revert-row clears both.
  await gridCell(page, 1, 'name').dblclick();
  await page.locator('[data-testid="grid-cell-input"]').fill('again');
  await page.keyboard.press('Enter');
  await expect(gridCell(page, 1, 'name')).toHaveClass(/pending-edit/);
  await gutterCell(page, 1).click({ button: 'right' });
  await page.click('[data-testid="menu-item-revert-row"]');
  await expect(page.locator('[data-testid="toolbar-commit-changes"]')).toHaveCount(0);

  // Revert everything: a cell edit on one row and a delete on another, then Discard.
  await gridCell(page, 1, 'name').dblclick();
  await page.locator('[data-testid="grid-cell-input"]').fill('third');
  await page.keyboard.press('Enter');
  await gutterCell(page, 2).click();
  await page.click('[data-testid="toolbar-delete-row"]');
  await expect(page.locator('[data-testid="toolbar-discard-changes"]')).toBeVisible();
  await page.click('[data-testid="toolbar-discard-changes"]');
  await expect(page.locator('[data-testid="toolbar-commit-changes"]')).toHaveCount(0);
  await expect(gridCell(page, 1, 'name')).not.toHaveClass(/pending-edit/);
});
