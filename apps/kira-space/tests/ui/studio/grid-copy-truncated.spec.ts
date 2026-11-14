import type { ControlSnapshot, LogicalPage, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import {
  NULLS_AND_UNICODE_COLUMNS,
  NULLS_AND_UNICODE_ROWS,
  NULLS_AND_UNICODE_TRUNCATED_CELLS,
  NULLS_AND_UNICODE_TRUNCATED_ROWS,
} from './support/cellEditorCaptures';
import { primaryChord } from './support/chord';
import { connectAndExpand, connectionCreateArgs } from './support/connect';
import { gridCell } from './support/grid';
import {
  NULLS_PATH,
  nullsAndUnicodeFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { findRow } from './support/tree';

// J-GRID-036 (OQ-GRID-2): copying a cell the engine cut at 64 KiB says so; the success message never
// claims a plain copy. The Go half is dbflow TestGridReadFlagsClippedCell.

const CONNECTION_ID = 'conn-copy-truncated';
const FIXTURE = nullsAndUnicodeFixture(CONNECTION_ID);
const PAGE: LogicalPage = {
  kind: 'tabular',
  columns: NULLS_AND_UNICODE_COLUMNS,
  rows: NULLS_AND_UNICODE_ROWS,
  position: {
    offset: 0,
    pageSize: 100,
    hasMore: false,
    nextToken: null,
    prevToken: null,
    strategy: 'keyset',
  },
  truncatedCells: NULLS_AND_UNICODE_TRUNCATED_CELLS,
  truncatedRows: NULLS_AND_UNICODE_TRUNCATED_ROWS,
};
const PORT: PortSnapshot[] = [
  { ...FIXTURE.port[0], response: { kind: 'read', page: PAGE, source: 'server' } },
];
const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Truncated DB', 'green'),
    response: postgresConnectionSummary(CONNECTION_ID, 'Truncated DB', 'green'),
  },
  ...FIXTURE.control,
];

test('copying a truncated cell announces the truncation', async ({ relaunch }) => {
  test.setTimeout(120_000);
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await page.evaluate(() => {
    navigator.clipboard.writeText = () => Promise.resolve();
  });
  await connectAndExpand(page, 'Truncated DB', 'green');
  await (await findRow(page, NULLS_PATH)).dblclick();
  const live = page.locator('[data-testid="app-live-region"]');
  const chord = await primaryChord(page, 'c');
  const grid = page.locator('[data-testid="data-grid"] .grid-canvas').first();

  await gridCell(page, 3, 'big_text').click();
  await grid.evaluate((el) => (el as HTMLElement).focus());
  await page.keyboard.press(chord);
  await expect(live).toContainText('64 kilobytes');
  await expect(live).toContainText('1 of these cells');

  await gridCell(page, 0, 'label').click();
  await grid.evaluate((el) => (el as HTMLElement).focus());
  await page.keyboard.press(chord);
  await expect(live).toHaveText('Copied selection');
});
