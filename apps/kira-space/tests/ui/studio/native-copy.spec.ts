import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { primaryChord } from './support/chord';
import { clipboardRead, clipboardReset, prepareRealClipboard } from './support/clipboard';
import { connectAndExpand, connectionCreateArgs } from './support/connect';
import { gridCell, gutterCell } from './support/grid';
import {
  BIG_ROWS_PATH,
  bigRowsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { findRow } from './support/tree';

// J-BC-003: Ctrl/Cmd+C inside a text control (filter editor, cell editor) copies that control's own
// selected text. The grid's selection copy never takes the keystroke over. Real clipboard, so
// Chromium only (project `ui-clipboard`).

const CONNECTION_ID = 'conn-native-copy';
const FIXTURE = bigRowsFixture(CONNECTION_ID);
const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Native Copy DB', 'green'),
    response: postgresConnectionSummary(CONNECTION_ID, 'Native Copy DB', 'green'),
  },
  ...FIXTURE.control,
];

test('copy inside the filter editor and the cell editor copies the control text', async ({
  relaunch,
  browserName,
}) => {
  test.skip(browserName !== 'chromium', 'native copy needs the real Chromium clipboard');
  test.setTimeout(120_000);
  const { window: page } = await relaunch({
    control: CONTROL,
    stream: FIXTURE.port,
    prepare: (p) => prepareRealClipboard(p, browserName),
  });
  await connectAndExpand(page, 'Native Copy DB', 'green');
  await (await findRow(page, BIG_ROWS_PATH)).dblclick();
  await expect(gridCell(page, 0, 'id')).toBeVisible();
  const copy = await primaryChord(page, 'c');
  const selectAll = await primaryChord(page, 'a');

  // The cell editor: type, select all, copy.
  await gridCell(page, 1, 'hash').click();
  await page.keyboard.press('Enter');
  const editor = page.locator('[data-testid="grid-cell-input"]');
  await expect(editor).toBeVisible();
  await editor.fill('cell editor text');
  // Ctrl/Cmd+A selects the editor text and keeps the edit open (it once extended the grid range).
  await page.keyboard.press(selectAll);
  await expect(editor).toBeVisible();
  await clipboardReset(page, browserName);
  await page.keyboard.press(copy);
  expect(await clipboardRead(page, browserName)).toBe('cell editor text');
  await page.keyboard.press('Escape');

  // Last: leaving the filter editor applies it and refetches a page the mock holds no snapshot for.
  // A grid row is selected, then the filter editor takes focus and copies its own text.
  await gutterCell(page, 0).click();
  const where = page.locator('[data-testid="filter-where-input"]');
  await where.click();
  await page.keyboard.type('id > 5 AND hash <> 1');
  await page.keyboard.press(selectAll);
  await clipboardReset(page, browserName);
  await page.keyboard.press(copy);
  expect(await clipboardRead(page, browserName)).toBe('id > 5 AND hash <> 1');
});
