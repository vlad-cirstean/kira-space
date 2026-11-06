import type { Page } from '@playwright/test';
import { expect, studioTest as test } from '../../ui/fixtures';
import { assertTooltipShows } from '../../ui/studio/support/tooltip';
import { connectionRow, expandRow, findRow, openRowMenu } from '../../ui/studio/support/tree';
import { controlSnapshots, portSnapshots } from './mariadb.fixture';

// The pilot adapter's frontend half. Real Vue and real bridge/{control,
// port}.ts; both wire protocols mocked from the exact same fixture mariadb.backend.spec.ts asserts
// against (the vital rule). No container, no adapter, no network, and no Electron
// process either — this file needs no Docker at all and is expected to run in this sandbox.

const DB_PATH = 'database:kira_test';
const ORDER_ITEMS_PATH = `${DB_PATH}/table:order_items`;
const BIG_ROWS_PATH = `${DB_PATH}/table:big_rows`;

async function firstGutterNumber(page: Page): Promise<string> {
  const grid = page.locator('[data-testid="data-grid"]');
  await grid.evaluate((el) => {
    el.scrollTop = 0;
  });
  await page.waitForTimeout(50);
  return (await page.locator('[data-testid="grid-gutter-cell"]').first().innerText()).trim();
}

test('mariadb (frontend, mocked IPC) — connect, tree, data tab, count, filter, stop button', async ({
  relaunch,
  consoleErrors,
}) => {
  const { window: page, control } = await relaunch({
    control: controlSnapshots,
    stream: portSnapshots,
  });

  const connRow = connectionRow(page);
  await expect(connRow).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });

  // --- tree: database -> table, no schema level, no twisty on a table (row 3) --------------
  await expandRow(page, '');
  const dbRow = await expandRow(page, DB_PATH);
  await expect(dbRow).toHaveAttribute('data-kind', 'database');
  const orderItemsRow = await findRow(page, ORDER_ITEMS_PATH);
  await expect(orderItemsRow).toHaveAttribute('data-kind', 'table');
  await expect(orderItemsRow.locator('[data-testid="tree-twisty"]')).not.toBeVisible();

  // --- open a data tab, read a page, count (rows 4-5) ---------------------------------------
  await (await findRow(page, ORDER_ITEMS_PATH)).dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await expect(page.locator('[data-testid="grid-header-cell"][data-column="id"]')).toBeVisible();
  await expect.poll(() => firstGutterNumber(page), { timeout: 15_000 }).toBe('1');

  await page.click('[data-testid="toolbar-count"]');
  // P104 §6: the count tooltip moved off `data-kira-tip` onto the real Tooltip system.
  await assertTooltipShows(page, page.locator('[data-testid="toolbar-count"]'), /3/, 15_000);

  // --- filter (row 6) ------------------------------------------------------------------------
  await page.fill('[data-testid="filter-where-input"]', 'quantity > 1');
  await page.press('[data-testid="filter-where-input"]', 'Enter');
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(2, { timeout: 10_000 });

  // --- stop button (row 7): the button's enablement and the opsCancel payload it sends, per D7 -
  // — no real slow query needed; the mocked big_rows read replies after portSnapshot's own
  // delayMs, standing in for the real SLEEP()-based filter the backend half already exercised.
  await (await findRow(page, BIG_ROWS_PATH)).dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await expect(page.locator('[data-testid="toolbar-stop"]')).toBeEnabled({ timeout: 2_000 });
  await page.click('[data-testid="toolbar-stop"]');
  await expect(page.locator('[data-testid="toolbar-stop"]')).toBeDisabled({ timeout: 2_000 });

  const controlLog = control.log();
  const cancelCall = controlLog.find((entry) => entry.channel === 'kira:ops:cancel');
  expect(cancelCall).toBeTruthy();
  expect(typeof (cancelCall?.args as { opId?: unknown } | undefined)?.opId).toBe('string');

  expect(consoleErrors).toEqual([]);
});
