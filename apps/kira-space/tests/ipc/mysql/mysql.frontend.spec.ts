import { expect, test } from '../../ui/studio/fixtures';
import { gridCell } from '../../ui/studio/support/grid';
import { assertTooltipShows } from '../../ui/studio/support/tooltip';
import { connectionRow, expandRow, findRow, openRowMenu } from '../../ui/studio/support/tree';
import type { ControlSnapshot } from '../support/types';
import { controlSnapshots, portSnapshots } from './mysql.fixture';

// P50 §4.4 — mysql's frontend half, same shape as mariadb's (§4.2). The engine-picker/Add
// Connection dialog flow is left to tests/e2e/connections.spec.ts (kept, unchanged, generic
// connection-dialog UI) — this spec starts from an already-listed connection.

interface TreeNodeLike {
  name: string;
  path: string;
  kind: string;
}

function nodePathByName(name: string): string {
  for (const snap of controlSnapshots as ControlSnapshot[]) {
    if (snap.channel !== 'kira:tree:children') continue;
    const nodes = (snap.response as { nodes?: TreeNodeLike[] } | undefined)?.nodes ?? [];
    const node = nodes.find((n) => n.name === name);
    if (node) return node.path;
  }
  throw new Error(`no captured tree node named ${name} in mysql.fixture.ts`);
}

const DB_PATH = nodePathByName('kira_test');
const ORDER_ITEMS_PATH = nodePathByName('order_items');

test('mysql (frontend, mocked IPC) — connect, tree, filter-by-value quoting, console', async ({
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
  await assertTooltipShows(page, statusDot, /^MySQL 8\.4\./);

  await expandRow(page, '');
  const dbRow = await expandRow(page, DB_PATH);
  await expect(dbRow).toHaveAttribute('data-kind', 'database');
  const orderItemsRow = await findRow(page, ORDER_ITEMS_PATH);
  await expect(orderItemsRow).toHaveAttribute('data-kind', 'table');
  // D20: the "Routines" folder heading is a frontend rendering decision over the real
  // function-kind nodes the fixture carries — the one part of this scenario the backend half
  // cannot assert (there is no backend "folder" node kind at all).
  const routinesFolder = page.locator('[data-testid="tree-row"]', { hasText: 'Routines' });
  await expect(routinesFolder).toBeVisible();

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
  // D17: the load-bearing assertion this whole adapter split exists for — backtick, not
  // double-quote.
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
