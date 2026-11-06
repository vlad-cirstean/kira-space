import { expect, studioTest as test } from '../../ui/fixtures';
import { gridCell } from '../../ui/studio/support/grid';
import { assertTooltipShows } from '../../ui/studio/support/tooltip';
import { connectionRow, expandRow, findRow, openRowMenu } from '../../ui/studio/support/tree';
import type { ControlSnapshot } from '../support/types';
import { controlSnapshots, portSnapshots } from './clickhouse.fixture';

// Clickhouse's frontend half, same shape as mysql's. The
// engine-picker/Add Connection dialog flow is left to tests/e2e/connections.spec.ts (kept,
// unchanged, generic connection-dialog UI, full-stack) — this spec starts from an already-listed
// connection, both wire protocols mocked from the exact fixture the backend half asserts against.

interface TreeNodeLike {
  name: string;
  path: string;
  kind: string;
}

function nodePathByName(name: string, kind?: string): string {
  for (const snap of controlSnapshots as ControlSnapshot[]) {
    if (snap.channel !== 'kira:tree:children') continue;
    const nodes = (snap.response as { nodes?: TreeNodeLike[] } | undefined)?.nodes ?? [];
    const node = nodes.find((n) => n.name === name && (!kind || n.kind === kind));
    if (node) return node.path;
  }
  throw new Error(`no captured tree node named ${name} in clickhouse.fixture.ts`);
}

const DB_PATH = nodePathByName('kira_test', 'database');
const ORDER_ITEMS_PATH = nodePathByName('order_items', 'table');

test('clickhouse (frontend, mocked IPC) — tree, filter-by-value quoting, delete gating, definition, console', async ({
  relaunch,
  consoleErrors,
}) => {
  const { window: page, stream } = await relaunch({
    control: controlSnapshots,
    stream: portSnapshots,
  });

  const connRow = connectionRow(page);
  await expect(connRow).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  const statusDot = connRow.locator('.status-dot');
  await expect(statusDot).toHaveAttribute('data-status', 'connected', { timeout: 10_000 });
  // P104 §6: the status-dot hint moved off `data-kira-tip` onto the real Tooltip system.
  await assertTooltipShows(page, statusDot, /^ClickHouse 2\d\./);

  // --- tree: tables ungrouped, Views/Materialized views folders, no INFORMATION_SCHEMA row ------
  await expandRow(page, '');
  const dbRow = await expandRow(page, DB_PATH);
  await expect(dbRow).toHaveAttribute('data-kind', 'database');
  const orderItemsRow = await findRow(page, ORDER_ITEMS_PATH);
  await expect(orderItemsRow).toHaveAttribute('data-kind', 'table');
  // D15: "Views"/"Materialized views" are frontend-only grouping headings over the real
  // view/matview-kind nodes the fixture carries — same reasoning as mysql's own "Routines"
  // finding (D20); there is no backend "folder" node kind at all.
  const viewsFolder = await findRow(page, `${DB_PATH}#view`);
  await expect(viewsFolder).toBeVisible();
  await expect(viewsFolder).toContainText('Views');
  const matviewsFolder = await findRow(page, `${DB_PATH}#matview`);
  await expect(matviewsFolder).toBeVisible();
  await expect(matviewsFolder).toContainText('Materialized views');
  await expect(
    page.locator('[data-testid="tree-row"]').filter({ hasText: 'INFORMATION_SCHEMA' }),
  ).toHaveCount(0);

  // --- Filter by this value: backtick-quoted, and it really narrows the grid (D29) --------------
  await (await findRow(page, ORDER_ITEMS_PATH)).dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await expect(page.locator('[data-testid="grid-header-cell"][data-column="id"]')).toBeVisible();

  // `[data-testid="grid-cell"][data-row="N"]` alone is never valid for SlickGrid's DOM shape
  // (data-row lives on the ancestor `.slick-row`, never the cell — tests/ui/support/grid.ts's own
  // doc comment, P22 Pass B) — gridCell() scopes into the row correctly.
  const idCell = gridCell(page, 0, 'id');
  await expect(idCell).toBeVisible();
  const idValue = (await idCell.innerText()).trim();
  await idCell.click({ button: 'right' });
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await page.click('[data-testid="menu-item-filter-by-value"]');
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(1, { timeout: 10_000 });
  const whereInput = page.locator('[data-testid="filter-where-input"]');
  await expect(whereInput).toHaveValue(`\`id\` = '${idValue}'`);

  // D7: the read request the filter actually produced, not only what it rendered.
  const ops = await stream.ops();
  const filteredRead = ops.find(
    (o) =>
      o.op === 'data:read' && (o.payload as { filter?: string }).filter === `\`id\` = '${idValue}'`,
  );
  expect(filteredRead).toBeTruthy();

  await whereInput.fill('');
  await whereInput.press('Enter');

  // --- + row is enabled; − row is disabled with an engine-specific tooltip; double-clicking a
  //     cell does not start an inline edit (D31/D26 — canUpdate/canDelete permanently false) -----
  const addRowButton = page.locator('[data-testid="toolbar-add-row"]');
  await expect(addRowButton).toBeEnabled();
  const deleteRowButton = page.locator('[data-testid="toolbar-delete-row"]');
  await expect(deleteRowButton).toBeDisabled();
  // P104 §6.3: a disabled <button> receives no pointer events in Blink, so the never-disabled
  // wrapper <span> is the real trigger and hit target.
  await assertTooltipShows(
    page,
    deleteRowButton.locator('xpath=..'),
    /doesn’t support deleting rows/,
  );

  await idCell.dblclick();
  await expect(page.locator('[data-testid="grid-cell-input"]')).toHaveCount(0);

  // --- the definition tab's Table properties section, and no PK badge anywhere (D18/D22) --------
  await openRowMenu(page, ORDER_ITEMS_PATH);
  await page.click('[data-testid="menu-item-open-definition"]');
  const definitionView = page.locator('[data-testid="definition-view"]');
  await expect(definitionView).toBeVisible();
  const tableSection = page.locator(
    '[data-testid="definition-properties"][data-title="Table properties"]',
  );
  await expect(tableSection).toBeVisible();
  await expect(tableSection).toContainText('MergeTree');
  await expect(tableSection).toContainText('Sorting key');
  await expect(page.locator('.header-key', { hasText: 'PK' })).toHaveCount(0);

  // --- the console tab is really in SQL mode, and runs a statement (D30) -------------------------
  await openRowMenu(page, DB_PATH);
  await page.click('[data-testid="menu-item-open-console"]');
  const consoleView = page.locator('[data-testid="console-view"]');
  await expect(consoleView).toBeVisible();
  await consoleView.locator('.view-lines').click();
  await page.keyboard.type('SELECT 1;');
  await page.click('[data-testid="console-run-statement"]');
  const results = consoleView.locator('[data-testid="console-result-grid"]');
  await expect(results).toHaveCount(1);
  await expect(results.first()).toContainText('1');

  expect(consoleErrors).toEqual([]);
});
