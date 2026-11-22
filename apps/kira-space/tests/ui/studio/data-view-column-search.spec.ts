import { DATA_OP } from '@shared/protocol/dataOps';
import { boxCenter, sortableHover } from '@workbench/testing/ui/drag';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { WIDE_TABLE_COLUMNS, WIDE_TABLE_ROWS } from './support/cellEditorCaptures';
import { connectAndExpand, connectionCreateArgs } from './support/connect';
import { headerCell, headerNames } from './support/grid';
import {
  bigRowsFixture,
  postgresConnectionSummary,
  WIDE_TABLE_PATH,
} from './support/postgresFixture';
import { findRow } from './support/tree';

// P725: column search lives in the Columns menu (filters the menu rows only) and each row has a
// button that scrolls the grid to its column. Both are display-only, so the backend half of the
// feature is nothing: this spec asserts that searching and scrolling send no `data:read`. Fixture is
// the real 60-column `app.wide_table` capture.

const CONNECTION_ID = 'conn-column-search';
const FIXTURE = bigRowsFixture(CONNECTION_ID);
const NAMES = WIDE_TABLE_COLUMNS.map((c) => c.name);
const HIDDEN = 'int_a';
const KEPT = WIDE_TABLE_COLUMNS.map((c, i) => ({ c, i })).filter(({ c }) => c.name !== HIDDEN);
const GRID = '[data-testid="data-grid"]';
const ROWS = '[data-testid="columns-menu-row"]';

const WIDE_META = {
  path: WIDE_TABLE_PATH,
  kind: 'table',
  name: 'wide_table',
  qualifiedName: 'app.wide_table',
  columns: WIDE_TABLE_COLUMNS.map((c, i) => ({
    name: c.name,
    position: i + 1,
    dataType: c.dataType,
    nullable: c.nullable,
    defaultExpr: null,
    isPrimaryKey: c.isPrimaryKey,
    comment: null,
  })),
  primaryKey: ['id'],
  foreignKeys: [],
  referencedBy: [],
  indexes: [],
  rowEstimate: WIDE_TABLE_ROWS.length,
  comment: null,
};

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Column Search DB', 'green'),
    response: postgresConnectionSummary(CONNECTION_ID, 'Column Search DB', 'green'),
  },
  ...FIXTURE.control,
  {
    channel: IPC.treeDescribe,
    args: { connectionId: CONNECTION_ID, path: WIDE_TABLE_PATH, refresh: false, tabId: null },
    response: { meta: WIDE_META, source: 'server' },
  },
];

const PORT: PortSnapshot[] = [
  ...FIXTURE.port,
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: WIDE_TABLE_PATH,
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
        columns: WIDE_TABLE_COLUMNS,
        rows: WIDE_TABLE_ROWS,
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
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: WIDE_TABLE_PATH,
      projection: KEPT.map(({ c }) => c.name),
      filter: null,
      sort: null,
      pageSize: 100,
      cursor: { mode: 'offset', offset: 0 },
    },
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: KEPT.map(({ c }) => c),
        rows: WIDE_TABLE_ROWS.map((row) => KEPT.map(({ i }) => row[i])),
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

const LAST = NAMES[NAMES.length - 1];

test('contract: Columns menu search and scroll to column, display-only', async ({
  relaunch,
  consoleErrors,
}) => {
  test.setTimeout(90_000);
  const { window: page, stream } = await relaunch({ control: CONTROL, stream: PORT });
  await page.setViewportSize({ width: 1200, height: 700 });
  await connectAndExpand(page, 'Column Search DB', 'green');
  await (await findRow(page, WIDE_TABLE_PATH)).dblclick();
  const search = page.locator('[data-testid="columns-menu-search"]');
  const rows = page.locator(ROWS);
  const row = (name: string) => page.locator(`${ROWS}[data-name="${name}"]`);
  const names = () =>
    rows.evaluateAll((els) => els.map((el) => el.getAttribute('data-name') ?? ''));
  const isWideRead = (o: { payload: unknown }) =>
    (o.payload as { path: string }).path === WIDE_TABLE_PATH;
  const wideReads = async () =>
    (await stream.ops()).filter((o) => o.op === DATA_OP.read).filter(isWideRead);

  // 1. the toolbar field is gone
  await expect(headerCell(page, 'id')).toBeVisible();
  await expect(page.locator('[data-testid="column-filter-input"]')).toHaveCount(0);
  const readsBefore = (await wideReads()).length;

  // 2. opening the menu focuses the search box
  await page.click('[data-testid="toolbar-columns"]');
  await expect(search).toBeFocused();
  await expect(rows).toHaveCount(NAMES.length);

  // 3. search narrows the menu rows, terms OR-ed, no match says so
  await search.fill('text_');
  await expect.poll(names).toEqual(NAMES.filter((n) => n.includes('text_')));
  await search.fill('bool uuid');
  await expect.poll(names).toEqual(NAMES.filter((n) => n.includes('bool') || n.includes('uuid')));
  await search.fill('zzz_none');
  await expect(rows).toHaveCount(0);
  await expect(page.locator('[data-testid="columns-menu-no-match"]')).toBeVisible();
  await search.fill('');
  await expect(rows).toHaveCount(NAMES.length);

  // 4. reveal scrolls the grid to the column; the menu stays open, the checkbox stays checked
  const grid = page.locator(GRID);
  const inGrid = async () => {
    const [g, h] = await Promise.all([grid.boundingBox(), headerCell(page, LAST).boundingBox()]);
    return !!g && !!h && h.x >= g.x && h.x + h.width <= g.x + g.width;
  };
  expect(await inGrid()).toBe(false);
  await search.fill(LAST);
  await expect(rows).toHaveCount(1);
  await rows.locator('[data-testid="columns-menu-reveal"]').click();
  await expect.poll(inGrid).toBe(true);
  await expect(page.locator('[data-testid="columns-menu"]')).toBeVisible();
  await expect(rows.locator('[data-testid="columns-menu-item"]')).toHaveAttribute(
    'data-state',
    'checked',
  );

  // 5. drag while filtered swaps the two rows in the full order
  const [a, b] = ['text_a', 'text_b'];
  await search.fill('text_');
  const src = await rows.locator(`xpath=self::*[@data-name="${a}"]`).boundingBox();
  const dst = await rows.locator(`xpath=self::*[@data-name="${b}"]`).boundingBox();
  if (!src || !dst) throw new Error('columns menu row has no box');
  await sortableHover(page, boxCenter(src), {
    x: dst.x + dst.width / 2,
    y: dst.y + dst.height * 0.75,
  });
  await page.mouse.up();
  await page.keyboard.press('Escape'); // clears the search text first
  await page.keyboard.press('Escape');
  await expect(page.locator('[data-testid="columns-menu"]')).toHaveCount(0);
  // The grid applies the new order at once; the menu keeps the swap relative to the full list.
  await expect
    .poll(async () => {
      const order = await headerNames(page);
      return order.indexOf(b) < order.indexOf(a);
    })
    .toBe(true);
  await page.click('[data-testid="toolbar-columns"]');
  await expect(rows).toHaveCount(NAMES.length);
  const full = await names();
  expect(full.indexOf(b)).toBeLessThan(full.indexOf(a));
  await page.keyboard.press('Escape');
  expect((await wideReads()).length).toBe(readsBefore);

  // 6. a column the grid does not render has a disabled reveal button
  await page.click('[data-testid="toolbar-columns"]');
  await search.fill(HIDDEN);
  await row(HIDDEN).locator('[data-testid="columns-menu-item"]').click();
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');
  await expect(headerCell(page, HIDDEN)).toHaveCount(0);
  await page.click('[data-testid="toolbar-columns"]');
  await search.fill(HIDDEN);
  const reveal = row(HIDDEN).locator('[data-testid="columns-menu-reveal"]');
  await expect(reveal).toBeDisabled();
  await reveal.hover({ force: true });
  await expect(page.getByText('Not shown in the grid')).toBeVisible();
  await row(HIDDEN).locator('[data-testid="columns-menu-item"]').click();
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');
  await expect(headerCell(page, HIDDEN)).toHaveCount(1);

  // 7. the chord opens the menu with the search box focused
  await page.locator('[data-testid="grid-cell"]').first().click();
  await expect(search).toHaveCount(0);
  await page.keyboard.press('Meta+Shift+F');
  await expect(search).toBeFocused();

  // 8. nothing reached the backend before the projection change; those reads are unprojected
  const reads = await wideReads();
  expect(reads.length).toBeGreaterThanOrEqual(readsBefore);
  expect((reads[0].payload as { projection: unknown }).projection).toBeNull();

  expect(consoleErrors).toEqual([]);
});
