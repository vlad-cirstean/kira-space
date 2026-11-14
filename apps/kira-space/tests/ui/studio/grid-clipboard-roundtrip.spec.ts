import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { primaryChord } from './support/chord';
import { installClipboardShim } from './support/clipboard';
import { connectionCreateArgs, createConnection } from './support/connect';
import { cellText, gridCell } from './support/grid';
import {
  COMPOSITE_PK_COLUMNS,
  COMPOSITE_PK_PATH,
  compositePkConnectAndOpen,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';

// J-GRID-013 / J-GRID-035, UI half. The Go half (dbflow TestGridPasteNullAndEmpty) commits the ops in
// contract studio-grid-paste and proves, per column type, what the server stores. This half pastes a
// TSV block whose last field is empty and asserts the window sends exactly those ops: an empty field
// in a nullable column is NULL, never ''.

const CONNECTION_ID = 'conn-grid-paste';
const FIXTURE = compositePkConnectAndOpen(CONNECTION_ID);
const SENT = contract<{ ops: unknown[] }>('studio-grid-paste', 'mutate:ops');

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Paste DB'),
    response: postgresConnectionSummary(CONNECTION_ID, 'Paste DB', 'green'),
  },
  ...FIXTURE.control,
];

const PORT: PortSnapshot[] = [
  ...FIXTURE.port,
  {
    op: DATA_OP.mutate,
    payload: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, ops: SENT.ops },
    response: { kind: 'mutate', affectedRows: 2 },
  },
  {
    op: DATA_OP.invalidate,
    payload: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, scope: 'pages' },
    response: { kind: 'invalidate' },
  },
];

test('pasting an empty last field stages NULL and sends the contract ops', async ({ relaunch }) => {
  test.setTimeout(180_000);
  const { window: page, stream } = await relaunch({ control: CONTROL, stream: PORT });
  await installClipboardShim(page);
  await createConnection(page, 'postgres', 'Paste DB');
  await expect(connectionRow(page)).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
  await expandRow(page, 'database:kira_test');
  await expandRow(page, 'database:kira_test/schema:app');
  await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
  const grid = page.locator('[data-testid="data-grid"]');
  await expect(grid).toBeVisible();
  expect(COMPOSITE_PK_COLUMNS.find((c) => c.name === 'name')?.nullable).toBe(true);

  await page.evaluate(() => navigator.clipboard.writeText('2\t1\t\n3\t3\t'));
  await gridCell(page, 2, 'tenant_id').click();
  await grid.focus();
  await page.keyboard.press(await primaryChord(page, 'v'));
  await expect(gridCell(page, 2, 'name')).toHaveClass(/pending-edit/);
  expect(await cellText(page, 2, 'name')).toBe('NULL');
  const insertInputs = page
    .locator('[data-testid="grid-row-insert"]')
    .locator('[data-testid="grid-cell-insert"] input');
  await expect(insertInputs.nth(0)).toHaveValue('3');
  await expect(insertInputs.nth(2)).toHaveValue('');

  await page.click('[data-testid="toolbar-commit-changes"]');
  await expect
    .poll(async () => (await stream.ops()).filter((o) => o.op === DATA_OP.mutate).length)
    .toBe(1);
  const sent = (await stream.ops()).filter((o) => o.op === DATA_OP.mutate);
  expect((sent[0].payload as { ops: unknown[] }).ops).toEqual(SENT.ops);
});
