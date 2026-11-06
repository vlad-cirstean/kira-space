import type { Page } from '@playwright/test';
import type { ColumnDescriptor } from '@shared/protocol/page';
import type { ControlSnapshot, LogicalPage, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { primaryChord } from './support/chord';
import { clipboardRead, clipboardReset, prepareRealClipboard } from './support/clipboard';
import { connectAndExpand, connectionCreateArgs, openConsoleFromMenu } from './support/connect';
import { executeSnap } from './support/consolePages';
import { typeInto } from './support/editor';
import { gridCell, gutterCell } from './support/grid';
import {
  BIG_ROWS_PATH,
  bigRowsFixture,
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { findRow } from './support/tree';

// Right-click "Copy as" over a multi-row selection must write what the selection copies with the
// keyboard. Real clipboard on Chromium (project `ui-clipboard`); on WebKit (project `ui`) a
// pass-through recorder around the real `writeText` (support/clipboard.ts).

const SQL = 'SELECT a, b FROM t;';

function col(name: string): ColumnDescriptor {
  return {
    name,
    dataType: 'text',
    typeClass: 'text',
    nullable: true,
    isPrimaryKey: false,
    generated: false,
  };
}

const CONSOLE_ROWS: [string, string | null][] = [
  ['1', '2'],
  ['3', 'x,y'],
  ['5', null],
  ['7', 'q"t'],
];

const CONSOLE_PAGE: LogicalPage = {
  kind: 'tabular',
  columns: [col('a'), col('b')],
  rows: CONSOLE_ROWS,
  position: {
    offset: 0,
    pageSize: CONSOLE_ROWS.length,
    hasMore: false,
    nextToken: null,
    prevToken: null,
    strategy: 'offset',
  },
  truncatedCells: 0,
};

async function openMenuItem(page: Page, id: string, submenu?: string): Promise<void> {
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  if (submenu) {
    await page.locator(`[data-testid="menu-item-${submenu}"]`).hover();
    await expect(page.locator('[data-testid="context-submenu"]')).toBeVisible();
  }
  await page.click(`[data-testid="menu-item-${id}"]`);
}

test('console grid — right-click copy acts on the selection and matches the keyboard', async ({
  relaunch,
  browserName,
}) => {
  const CONNECTION_ID = 'conn-grid-copy-console';
  const FIXTURE = orderItemsFixture(CONNECTION_ID);
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: connectionCreateArgs('Grid Copy DB', 'cyan'),
      response: postgresConnectionSummary(CONNECTION_ID, 'Grid Copy DB', 'cyan'),
    },
    ...FIXTURE.control,
  ];
  const PORT: PortSnapshot[] = [
    ...FIXTURE.port,
    executeSnap(CONNECTION_ID, [SQL.replace(/;$/, '')], [CONSOLE_PAGE]),
  ];
  const { window: page } = await relaunch({
    control: CONTROL,
    stream: PORT,
    prepare: (p) => prepareRealClipboard(p, browserName),
  });
  await connectAndExpand(page, 'Grid Copy DB', 'cyan');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const consoleView = page.locator('[data-testid="console-view"]');
  const results = consoleView.locator('[data-testid="console-result-grid"]');
  await typeInto(consoleView, page, SQL);
  await page.click('[data-testid="console-run-statement"]');
  await expect(results.locator('[data-testid="console-result-row"]')).toHaveCount(4);

  const cellAt = (row: number, c: number) =>
    results.locator(
      `[data-testid="console-result-row"][data-row="${row}"] [data-testid="console-result-cell"][data-col-index="${c}"]`,
    );
  const gutterAt = (row: number) =>
    results.locator(`.slick-row[data-row="${row}"] [data-testid="console-result-gutter-cell"]`);
  const headerB = results.locator('[data-testid="console-result-header-cell"][data-column="b"]');
  const copyChord = await primaryChord(page, 'c');

  const keyboardCopy = async (): Promise<string> => {
    await clipboardReset(page, browserName);
    await results
      .locator('.grid-canvas')
      .first()
      .evaluate((el) => (el as HTMLElement).focus());
    await page.keyboard.press(copyChord);
    return clipboardRead(page, browserName);
  };
  const menuCopy = async (
    target: ReturnType<typeof cellAt>,
    id: string,
    submenu?: string,
  ): Promise<string> => {
    await clipboardReset(page, browserName);
    await target.click({ button: 'right' });
    await openMenuItem(page, id, submenu);
    return clipboardRead(page, browserName);
  };

  // Gutter rows 0-1, right-click on a body cell: the row menu, acting on both rows.
  await gutterAt(0).click();
  await gutterAt(1).click({ modifiers: ['Shift'] });
  const kb01 = await keyboardCopy();
  expect(kb01).toBe('1\t2\n3\tx,y');
  expect(await menuCopy(cellAt(0, 0), 'copy-rows-tsv', 'copy-rows')).toBe(kb01);
  expect(await menuCopy(cellAt(1, 1), 'copy-rows-csv', 'copy-rows')).toBe('1,2\r\n3,"x,y"');
  expect(JSON.parse(await menuCopy(cellAt(0, 1), 'copy-rows-json', 'copy-rows'))).toEqual([
    { a: '1', b: '2' },
    { a: '3', b: 'x,y' },
  ]);

  // Disjoint gutter rows 0 and 2 (NULL in row 2), right-click the gutter.
  await gutterAt(0).click();
  await gutterAt(2).click({ modifiers: ['Control'] });
  const kb02 = await keyboardCopy();
  expect(kb02).toBe('1\t2\n5\t');
  expect(await menuCopy(gutterAt(2), 'copy-rows-tsv', 'copy-rows')).toBe(kb02);

  // Right-click outside the row selection: the clicked cell becomes the selection first.
  await gutterAt(0).click();
  await gutterAt(1).click({ modifiers: ['Shift'] });
  expect(await menuCopy(cellAt(2, 0), 'copy-as-json')).toBe('"5"');
  await expect(cellAt(2, 0)).toHaveClass(/kira-cell-selected/);
  await expect(cellAt(0, 0)).not.toHaveClass(/kira-cell-selected/);

  // Range over rows 0-2, both columns.
  await cellAt(0, 0).click();
  await cellAt(2, 1).click({ modifiers: ['Shift'] });
  const kbRange = await keyboardCopy();
  expect(kbRange).toBe('1\t2\n3\tx,y\n5\t');
  expect(await menuCopy(cellAt(1, 0), 'copy')).toBe(kbRange);
  expect(await menuCopy(cellAt(1, 0), 'copy-as-csv')).toBe('1,2\r\n3,"x,y"\r\n5,');
  expect(JSON.parse(await menuCopy(cellAt(1, 0), 'copy-as-json'))).toEqual([
    { a: '1', b: '2' },
    { a: '3', b: 'x,y' },
    { a: '5', b: null },
  ]);

  // One column, two rows.
  await cellAt(0, 0).click();
  await cellAt(1, 0).click({ modifiers: ['Shift'] });
  expect(await menuCopy(cellAt(1, 0), 'copy-as-csv')).toBe('1\r\n3');

  // Single cells, including a value that needs TSV quoting.
  await cellAt(1, 0).click();
  expect(await keyboardCopy()).toBe('3');
  expect(await menuCopy(cellAt(1, 0), 'copy')).toBe('3');
  expect(await menuCopy(cellAt(1, 0), 'copy-as-json')).toBe('"3"');
  await cellAt(3, 1).click();
  const kbQuoted = await keyboardCopy();
  expect(kbQuoted).toBe('"q""t"');
  expect(await menuCopy(cellAt(3, 1), 'copy')).toBe(kbQuoted);

  // Column b via its header: NULL is an empty line.
  await headerB.click();
  const kbCol = await keyboardCopy();
  expect(kbCol).toBe('2\nx,y\n\n"q""t"');
  expect(await menuCopy(cellAt(0, 1), 'copy-column')).toBe(kbCol);
});

test('data grid — right-click copy matches the keyboard per selection kind', async ({
  relaunch,
  browserName,
}) => {
  const CONNECTION_ID = 'conn-grid-copy-data';
  const FIXTURE = bigRowsFixture(CONNECTION_ID);
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: connectionCreateArgs('Grid Copy Data DB', 'green'),
      response: postgresConnectionSummary(CONNECTION_ID, 'Grid Copy Data DB', 'green'),
    },
    ...FIXTURE.control,
  ];
  const { window: page } = await relaunch({
    control: CONTROL,
    stream: FIXTURE.port,
    prepare: (p) => prepareRealClipboard(p, browserName),
  });
  await connectAndExpand(page, 'Grid Copy Data DB', 'green');
  await (await findRow(page, BIG_ROWS_PATH)).dblclick();
  await expect(gridCell(page, 0, 'id')).toBeVisible();
  const copyChord = await primaryChord(page, 'c');

  const keyboardCopy = async (): Promise<string> => {
    await clipboardReset(page, browserName);
    await page
      .locator('[data-testid="data-grid"] .grid-canvas')
      .first()
      .evaluate((el) => (el as HTMLElement).focus());
    await page.keyboard.press(copyChord);
    return clipboardRead(page, browserName);
  };
  const menuCopy = async (
    target: ReturnType<typeof gridCell>,
    id: string,
    submenu?: string,
  ): Promise<string> => {
    await clipboardReset(page, browserName);
    await target.click({ button: 'right' });
    await openMenuItem(page, id, submenu);
    return clipboardRead(page, browserName);
  };

  // Gutter rows 0-1: ⌘C, then every Copy rows format from a gutter and from a body right-click.
  await gutterCell(page, 0).click();
  await gutterCell(page, 1).click({ modifiers: ['Shift'] });
  const kbRows = await keyboardCopy();
  expect(kbRows.split('\n')).toHaveLength(2);
  expect(await menuCopy(gridCell(page, 0, 'id'), 'copy-rows-tsv', 'copy-rows')).toBe(kbRows);
  expect(await menuCopy(gutterCell(page, 1), 'copy-rows-csv', 'copy-rows')).toMatch(
    /^1,[0-9a-f]{32}\r\n2,[0-9a-f]{32}$/,
  );
  expect(
    JSON.parse(await menuCopy(gridCell(page, 1, 'hash'), 'copy-rows-json', 'copy-rows')),
  ).toHaveLength(2);
  const insert = await menuCopy(gridCell(page, 0, 'id'), 'copy-rows-insert', 'copy-rows');
  expect(insert.startsWith('INSERT INTO ')).toBe(true);
  expect(insert.match(/^ {2}\(/gm)).toHaveLength(2);

  // A row selection prints the shortcut on "TSV", since ⌘C copies exactly that.
  await gridCell(page, 0, 'id').click({ button: 'right' });
  await page.locator('[data-testid="menu-item-copy-rows"]').hover();
  await expect(page.locator('[data-testid="context-submenu"]')).toBeVisible();
  await expect(page.locator('[data-testid="menu-item-copy-rows-tsv-shortcut"]')).toHaveCount(1);
  await page.keyboard.press('Escape');

  // Range over one column: ⌘C copies cells, "TSV" copies whole rows and prints no shortcut.
  await gridCell(page, 0, 'id').click();
  await gridCell(page, 2, 'id').click({ modifiers: ['Shift'] });
  expect(await keyboardCopy()).toBe('1\n2\n3');
  await clipboardReset(page, browserName);
  await gridCell(page, 1, 'id').click({ button: 'right' });
  await page.locator('[data-testid="menu-item-copy-rows"]').hover();
  await expect(page.locator('[data-testid="context-submenu"]')).toBeVisible();
  await expect(page.locator('[data-testid="menu-item-copy-rows-tsv-shortcut"]')).toHaveCount(0);
  await page.click('[data-testid="menu-item-copy-rows-tsv"]');
  const wholeRows = (await clipboardRead(page, browserName)).split('\n');
  expect(wholeRows).toHaveLength(3);
  for (const line of wholeRows) expect(line).toMatch(/^\d+\t[0-9a-f]{32}$/);

  // Single cell.
  await gridCell(page, 1, 'id').click();
  expect(await keyboardCopy()).toBe('2');
  expect(await menuCopy(gridCell(page, 1, 'id'), 'copy')).toBe('2');
  expect(await menuCopy(gridCell(page, 1, 'id'), 'copy-as-json')).toBe('"2"');
});
