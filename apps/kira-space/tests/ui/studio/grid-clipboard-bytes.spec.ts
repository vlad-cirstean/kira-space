import type { Page } from '@playwright/test';
import type { ControlSnapshot, LogicalPage, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { primaryChord } from './support/chord';
import { clipboardRead, clipboardReset, prepareRealClipboard } from './support/clipboard';
import { connectAndExpand, connectionCreateArgs } from './support/connect';
import { gridCell, gutterCell } from './support/grid';
import {
  COMPOSITE_PK_COLUMNS,
  COMPOSITE_PK_PATH,
  compositePkConnectAndOpen,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { findRow } from './support/tree';

// J-GRID-010 / J-GRID-011, UI half. The Go half (dbflow TestGridClipboardBytesContract) stores the
// hostile values in a real engine, proves they come back byte-for-byte and commits the clipboard text
// in contract studio-grid-clipboard. This half replays those rows and compares what Copy writes.

const CONNECTION_ID = 'conn-grid-clip-bytes';
const ROWS = contract<(string | null)[][]>('studio-grid-clipboard', 'rows');
const TSV = contract<string>('studio-grid-clipboard', 'copy:tsv');
const CSV = contract<string>('studio-grid-clipboard', 'copy:csv');
const JSON_TEXT = contract<string>('studio-grid-clipboard', 'copy:json');

const FIXTURE = compositePkConnectAndOpen(CONNECTION_ID);
const READ = FIXTURE.port[0];
const PAGE: LogicalPage = {
  kind: 'tabular',
  columns: COMPOSITE_PK_COLUMNS,
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
  { ...READ, response: { kind: 'read', page: PAGE, source: 'server' } },
];
const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Clip DB', 'green'),
    response: postgresConnectionSummary(CONNECTION_ID, 'Clip DB', 'green'),
  },
  ...FIXTURE.control,
];

async function menuCopy(
  page: Page,
  browserName: string,
  target: ReturnType<typeof gridCell>,
  id: string,
  submenu: string,
): Promise<string> {
  await clipboardReset(page, browserName);
  await target.click({ button: 'right' });
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await page.locator(`[data-testid="menu-item-${submenu}"]`).hover();
  await expect(page.locator('[data-testid="context-submenu"]')).toBeVisible();
  await page.click(`[data-testid="menu-item-${id}"]`);
  return clipboardRead(page, browserName);
}

test('hostile cells copy as the contract bytes in every format', async ({
  relaunch,
  browserName,
}) => {
  test.setTimeout(180_000);
  const { window: page } = await relaunch({
    control: CONTROL,
    stream: PORT,
    prepare: (p) => prepareRealClipboard(p, browserName),
  });
  await connectAndExpand(page, 'Clip DB', 'green');
  await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
  await expect(gridCell(page, 0, 'name')).toBeVisible();

  const last = ROWS.length - 1;
  await gutterCell(page, 0).click();
  await gutterCell(page, last).click({ modifiers: ['Shift'] });

  await clipboardReset(page, browserName);
  await page
    .locator('[data-testid="data-grid"] .grid-canvas')
    .first()
    .evaluate((el) => (el as HTMLElement).focus());
  await page.keyboard.press(await primaryChord(page, 'c'));
  expect(await clipboardRead(page, browserName)).toBe(TSV);

  const body = gridCell(page, 3, 'name');
  expect(await menuCopy(page, browserName, body, 'copy-rows-tsv', 'copy-rows')).toBe(TSV);
  expect(await menuCopy(page, browserName, body, 'copy-rows-csv', 'copy-rows')).toBe(CSV);
  expect(await menuCopy(page, browserName, body, 'copy-rows-json', 'copy-rows')).toBe(JSON_TEXT);
});
