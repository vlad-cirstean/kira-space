import type { Page } from '@playwright/test';
import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { emitWailsEvent } from '../support/mockRuntime';
import { WIDE_TABLE_COLUMNS, WIDE_TABLE_ROWS } from './support/cellEditorCaptures';
import { connectAndExpand, connectionCreateArgs } from './support/connect';
import { cellText, headerCell, headerNames } from './support/grid';
import {
  BIG_ROWS_PATH,
  bigRowsFixture,
  postgresConnectionSummary,
  WIDE_TABLE_PATH,
} from './support/postgresFixture';
import { findRow } from './support/tree';

// P658: the column-name filter in the filter toolbar narrows the grid's visible columns. It is
// display-only, so the backend half of the feature is nothing: this spec asserts that no `data:read`
// request changes while filtering. Fixture is the real 60-column `app.wide_table` capture.

const CONNECTION_ID = 'conn-column-filter';
const FIXTURE = bigRowsFixture(CONNECTION_ID);
const NAMES = WIDE_TABLE_COLUMNS.map((c) => c.name);
const HEADERS = '[data-testid="grid-header-cell"]';

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
    args: connectionCreateArgs('Column Filter DB', 'green'),
    response: postgresConnectionSummary(CONNECTION_ID, 'Column Filter DB', 'green'),
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
];

function expectedNames(...prefixes: string[]): string[] {
  return NAMES.filter((n) => n === 'id' || prefixes.some((p) => n.includes(p)));
}

async function expectHeaders(page: Page, names: string[]): Promise<void> {
  await expect.poll(() => headerNames(page)).toEqual(names);
}

test('contract: column filter narrows grid columns, display-only', async ({
  relaunch,
  consoleErrors,
}) => {
  test.setTimeout(90_000);
  const { window: page, stream } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndExpand(page, 'Column Filter DB', 'green');
  await (await findRow(page, WIDE_TABLE_PATH)).dblclick();
  const filter = page.locator('[data-testid="column-filter-input"]');
  const status = page.locator('[data-testid="column-filter-status"]');
  const isWideRead = (o: { payload: unknown }) =>
    (o.payload as { path: string }).path === WIDE_TABLE_PATH;
  const reads = async () => (await stream.ops()).filter((o) => o.op === DATA_OP.read);

  // 1. all columns
  await expect(page.locator(HEADERS)).toHaveCount(NAMES.length);
  const wideReadsBefore = (await reads()).filter(isWideRead).length;

  // 2. substring filter keeps the PK, maps display columns to page values
  await filter.fill('text_');
  const text = expectedNames('text_');
  await expectHeaders(page, text);
  await expect(status).toHaveText(`${text.length} of ${NAMES.length}`);
  expect(await cellText(page, 0, 'text_a')).toBe(WIDE_TABLE_ROWS[0][NAMES.indexOf('text_a')]);

  // 3. several terms, whitespace-split, OR-ed
  await filter.fill('bool uuid');
  await expectHeaders(page, expectedNames('bool', 'uuid'));

  // 4. no match keeps every column and flags the field
  await filter.fill('zzz_none');
  await expect(page.locator(HEADERS)).toHaveCount(NAMES.length);
  await expect(filter).toHaveAttribute('aria-invalid', 'true');
  await expect(status).toHaveText('No match');

  // 5. the filter survives a tab switch (P660 seam)
  await filter.fill('date');
  await expectHeaders(page, expectedNames('date'));
  await (await findRow(page, BIG_ROWS_PATH)).dblclick();
  await expect(headerCell(page, 'hash')).toBeVisible();
  await page.locator('[data-testid="tab"]').first().click();
  await expect(filter).toHaveValue('date');
  await expectHeaders(page, expectedNames('date'));

  // 6. Escape clears
  await filter.focus();
  await page.keyboard.press('Escape');
  await expect(filter).toHaveValue('');
  await expect(page.locator(HEADERS)).toHaveCount(NAMES.length);

  // 7. nothing reached the backend: every wide-table read is the original unprojected one
  const wideReads = (await reads()).filter(isWideRead);
  expect(wideReads.length).toBeGreaterThanOrEqual(1);
  for (const r of wideReads) {
    expect((r.payload as { projection: unknown }).projection).toBeNull();
  }
  // at most the remount re-read after the tab switch, never one per keystroke
  expect(wideReads.length).toBeLessThanOrEqual(wideReadsBefore + 1);

  // 8. palette command focuses the field
  await emitWailsEvent(page, IPC.commandPalette, null);
  await page
    .locator('[data-testid="command-palette-item"][data-command-id="data.filterColumns"]')
    .click();
  await expect(filter).toBeFocused();

  // 9. a search match in a hidden column clears the filter and reveals the cell
  await filter.fill('bool');
  await expectHeaders(page, expectedNames('bool'));
  await page.click('[data-testid="toolbar-search"]');
  await page.fill('[data-testid="search-input"]', 'row one');
  await page.click('[data-testid="search-next"]');
  await expect(filter).toHaveValue('');
  await expect(page.locator(HEADERS)).toHaveCount(NAMES.length);
  await expect(page.locator('[data-column="text_a"]').first()).toBeVisible();

  // 10. a staged insert keeps its typed value while its column is filtered out
  await page.click('[data-testid="toolbar-add-row"]');
  const insertInput = page.locator('[data-testid="grid-row-insert"] [data-column="text_a"] input');
  await insertInput.fill('typed value');
  await filter.fill('bool');
  await expectHeaders(page, expectedNames('bool'));
  await filter.fill('');
  await expect(page.locator(HEADERS)).toHaveCount(NAMES.length);
  await expect(insertInput).toHaveValue('typed value');
  await page.click('[data-testid="toolbar-discard-changes"]');

  expect(consoleErrors).toEqual([]);
});
