import type { Page } from '@playwright/test';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { dbOp } from '../support/dbOps';
import { IPC } from '../support/ipcChannels';
import { createConnection } from './support/connect';
import {
  DB_PATH,
  mariadbConnectionSummary,
  ORDER_ITEMS_PATH,
  orderItemsFixture,
} from './support/mariadbFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';

// The two guarantees — every genuinely-data surface (36 sites) resolves --kira-font-data,
// every chrome surface (the "no declaration of its own" majority, riding `body`) resolves
// --kira-font-ui, and the two tokens are not secretly aliased back together.

const CONNECTION_ID = 'conn-font-roles';
const CONNECTION_SUMMARY = mariadbConnectionSummary(CONNECTION_ID, 'FontRoles', 'amber');
const FIXTURE = orderItemsFixture(CONNECTION_ID);

const OP_RECORD = {
  id: 'op-font-roles-1',
  connectionId: null,
  tabId: null,
  startedAt: '2026-01-01T00:00:00.000Z',
  durationMs: 12,
  kind: 'read',
  status: 'ok',
  rows: 3,
  command: 'select 1',
  error: null,
};

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: {
      name: 'FontRoles',
      kind: 'mariadb',
      color: 'amber',
      mode: 'fields',
      readOnly: false,
      host: '127.0.0.1',
      port: 3306,
      database: 'kira_test',
      username: 'kira',
      password: null,
      uri: null,
      options: {},
      preconnect: null,
      preconnectSidecar: false,
      preconnectDelaySec: 0,
      autoExplain: false,
      throttlePerSec: 0,
    },
    response: CONNECTION_SUMMARY,
  },
  ...FIXTURE.control,
  { channel: IPC.opsRecent, response: [dbOp(OP_RECORD)] },
];

async function connectAndOpenGrid(page: Page): Promise<void> {
  await createConnection(page, 'mariadb', 'FontRoles');

  const connRow = connectionRow(page);
  await expect(connRow).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);

  const row = await findRow(page, ORDER_ITEMS_PATH);
  await row.dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await expect(page.locator('[data-testid="grid-row"]').first()).toBeVisible({ timeout: 15_000 });
}

function rootFontVar(page: Page, name: string): Promise<string> {
  return page.evaluate(
    (n) => getComputedStyle(document.documentElement).getPropertyValue(n).trim(),
    name,
  );
}

// getComputedStyle(el).fontFamily is a *serialized* CSSOM value (Chromium drops quotes around a
// multi-word family name like "Segoe WPC" once it resolves the property, since font-family's own
// grammar allows unquoted multi-token names) — it is not byte-identical to the raw custom-property
// text rootFontVar() above reads. Comparing an element's computed family against another *real
// element's* computed family (rather than against the raw --kira-font-* text) keeps both sides of
// every comparison serialized the same way.
function computedFontFamily(page: Page, selector: string): Promise<string> {
  return page
    .locator(selector)
    .first()
    .evaluate((el) => getComputedStyle(el).fontFamily);
}

test('font-roles — data surfaces render in the data font, not the interface font (F6)', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: FIXTURE.port });
  await connectAndOpenGrid(page);

  // Anchors: body carries --kira-font-ui (D2); .slick-cell names --kira-font-data directly
  // (F6#9). Comparing against these real elements' own computed style, rather than the raw
  // custom-property text, keeps both sides serialized the same way (see computedFontFamily).
  const uiFont = await computedFontFamily(page, 'body');
  const dataFont = await computedFontFamily(page, '.slick-cell');
  expect(dataFont).not.toBe(uiFont);

  // .view-lines — the SQL console editor. Monaco appends its own built-in fallback list
  // after the configured family (unlike CodeMirror, which set the family verbatim), so the
  // computed value is `dataFont` plus extra fallbacks rather than an exact match — the configured
  // font still wins the actual render since it's first in the list.
  await openRowMenu(page, DB_PATH);
  await page.click('[data-testid="menu-item-open-console"]');
  const sqlConsole = page.locator('[data-testid="console-view"]');
  await expect(sqlConsole).toBeVisible();
  const cmFont = await sqlConsole
    .locator('.view-lines')
    .first()
    .evaluate((el) => getComputedStyle(el).fontFamily);
  expect(cmFont.startsWith(dataFont)).toBe(true);
  expect(cmFont).not.toBe(uiFont);

  // Operations panel's font-data command/duration column (F6, "a query, a duration").
  await page.click('[data-testid="toggle-operations-panel"]');
  const opRow = page.locator('[data-testid="op-row"]').first();
  await expect(opRow).toBeVisible();
  const opMonoFont = await opRow
    .locator('[data-testid="op-time-cell"]')
    .evaluate((el) => getComputedStyle(el).fontFamily);
  expect(opMonoFont).toBe(dataFont);
  expect(opMonoFont).not.toBe(uiFont);
});

test('font-roles — chrome renders in the interface font, and it differs from the data font (F7/D3)', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: FIXTURE.port });
  await connectAndOpenGrid(page);

  const uiFont = await computedFontFamily(page, 'body');
  const dataFont = await computedFontFamily(page, '.slick-cell');
  expect(uiFont).not.toBe(dataFont);

  // The one assertion that would catch --kira-font-ui accidentally aliased back to the data
  // channel: a stack ending in sans-serif can never contain the data font's own families. Read
  // straight off the token here (rootFontVar), not off an element — a substring search doesn't
  // care about quote serialization.
  const uiFontToken = await rootFontVar(page, '--kira-font-ui');
  expect(uiFontToken.toLowerCase()).not.toContain('menlo');
  expect(uiFontToken.toLowerCase()).not.toContain('monospace');

  const readFont = (el: Element) => getComputedStyle(el).fontFamily;

  // A toolbar Button (console's "Run" button, variant="toolbar").
  await openRowMenu(page, DB_PATH);
  await page.click('[data-testid="menu-item-open-console"]');
  const runBtn = page.locator('[data-testid="console-run-statement"]');
  await expect(runBtn).toBeVisible();
  expect(await runBtn.evaluate(readFont)).toBe(uiFont);

  // A TreeRow label — F8(a)'s own most visible consequence.
  const row = await findRow(page, ORDER_ITEMS_PATH);
  expect(await row.locator('.label').evaluate(readFont)).toBe(uiFont);

  // A tab title in TabStrip.
  const tabTitle = page.locator('[data-testid="tab"] .tab-title').first();
  await expect(tabTitle).toBeVisible();
  expect(await tabTitle.evaluate(readFont)).toBe(uiFont);

  // A dialog title (Settings).
  await page.click('[data-testid="open-settings"]');
  const dialogTitle = page.locator('[data-testid="settings-dialog"] [data-slot="dialog-title"]');
  await expect(dialogTitle).toBeVisible();
  expect(await dialogTitle.evaluate(readFont)).toBe(uiFont);
  await page.click('[data-testid="settings-dialog-close"]');
});
