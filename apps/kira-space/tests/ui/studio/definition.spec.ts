import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { connectAndExpand, connectionCreateArgs } from './support/connect';
import { editorText } from './support/editorText';
import {
  APP_PATH,
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { expandRow, findRow, openRowMenu, settleFrames } from './support/tree';

// Ported from tests/e2e/definition.spec.ts, against real captured
// treeDefinition responses (scripts/capture-postgres-tree.ts). Scenario 10 ("session restore")
// is dropped — real cross-relaunch persistence, no backing store in this tier, same category as
// workbench.spec.ts's five. Scenario 11 (MariaDB) was already explicitly skipped in the original
// (the renderer path is engine-agnostic; mariadb.spec.ts covers that engine's definition()
// directly).
//
// The original's op-count bookkeeping (`window.kira.opsRecent`, dead — no window.kira any more)
// tracked whether a definition open was a real backend round trip or a tree-service cache hit.
// That distinction is already fully visible through `data-source` ('server'/'cache'), which every
// relevant scenario already asserts directly — the op-count checks were redundant with it, not an
// independent fact, so they're dropped rather than rebuilt on a different counting mechanism
// (`kira.control.log()`) that wouldn't mean the same thing anyway (it logs every call this tier's
// mock answers, hit or miss, since the mock doesn't implement real caching).

const CONNECTION_ID = 'conn-definition';
const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Definition DB', 'blue');
const FIXTURE = orderItemsFixture(CONNECTION_ID);
const WIDE_TABLE_PATH = `${APP_PATH}/table:wide_table`;
const ORDER_SUMMARY_PATH = `${APP_PATH}/view:order_summary`;
const CUSTOMER_TOTALS_PATH = `${APP_PATH}/matview:customer_totals`;
const INVOICE_SEQ_PATH = `${APP_PATH}/sequence:invoice_number_seq`;
const FULL_NAME_PATH = `${APP_PATH}/function:full_name`;
const VIEWS_FOLDER_PATH = `${APP_PATH}#view`;
const MATVIEWS_FOLDER_PATH = `${APP_PATH}#matview`;
const SEQUENCES_FOLDER_PATH = `${APP_PATH}#sequence`;
const FUNCTIONS_FOLDER_PATH = `${APP_PATH}#function`;

const ORDER_ITEMS_DEFINITION = {
  path: ORDER_ITEMS_PATH,
  kind: 'table' as const,
  qualifiedName: 'app.order_items',
  statements: [
    'CREATE SEQUENCE app.order_items_id_seq',
    "CREATE TABLE app.order_items (\n  id integer DEFAULT nextval('app.order_items_id_seq'::regclass) NOT NULL,\n  order_id integer NOT NULL,\n  product_id integer NOT NULL,\n  quantity integer DEFAULT 1 NOT NULL\n)",
    'ALTER SEQUENCE app.order_items_id_seq OWNED BY app.order_items.id',
    'ALTER TABLE app.order_items ADD CONSTRAINT order_items_pkey PRIMARY KEY (id)',
    'ALTER TABLE app.order_items ADD CONSTRAINT order_items_quantity_positive CHECK (quantity > 0)',
    'ALTER TABLE app.order_items ADD CONSTRAINT order_items_order_id_fkey FOREIGN KEY (order_id) REFERENCES app.orders(id)',
    'ALTER TABLE app.order_items ADD CONSTRAINT order_items_product_id_fkey FOREIGN KEY (product_id) REFERENCES app.products(id)',
    'CREATE UNIQUE INDEX order_items_order_product_idx ON app.order_items USING btree (order_id, product_id)',
  ],
  language: 'sql' as const,
  origin: 'composed' as const,
  notes: [
    'Composed from catalog metadata: triggers, row-level security policies, grants, ownership, storage parameters, tablespaces and non-default column collations are not included.',
  ],
  constraints: [
    { name: 'order_items_pkey', type: 'primaryKey' as const, definition: 'PRIMARY KEY (id)' },
    {
      name: 'order_items_quantity_positive',
      type: 'check' as const,
      definition: 'CHECK (quantity > 0)',
    },
    {
      name: 'order_items_order_id_fkey',
      type: 'foreignKey' as const,
      definition: 'FOREIGN KEY (order_id) REFERENCES app.orders(id)',
    },
    {
      name: 'order_items_product_id_fkey',
      type: 'foreignKey' as const,
      definition: 'FOREIGN KEY (product_id) REFERENCES app.products(id)',
    },
  ],
  documentSchema: null,
  sections: [],
  generatedAt: '2026-01-01T00:00:00.000Z',
};

const WIDE_TABLE_DEFINITION = {
  path: WIDE_TABLE_PATH,
  kind: 'table' as const,
  qualifiedName: 'app.wide_table',
  statements: [
    'CREATE SEQUENCE app.wide_table_id_seq',
    "CREATE TABLE app.wide_table (\n  id bigint DEFAULT nextval('app.wide_table_id_seq'::regclass) NOT NULL\n)",
    'ALTER TABLE app.wide_table ADD CONSTRAINT wide_table_pkey PRIMARY KEY (id)',
  ],
  language: 'sql' as const,
  origin: 'composed' as const,
  notes: [
    'Composed from catalog metadata: triggers, row-level security policies, grants, ownership, storage parameters, tablespaces and non-default column collations are not included.',
  ],
  constraints: [
    { name: 'wide_table_pkey', type: 'primaryKey' as const, definition: 'PRIMARY KEY (id)' },
  ],
  documentSchema: null,
  sections: [],
  generatedAt: '2026-01-01T00:00:00.000Z',
};

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Definition DB', 'blue'),
    response: CONNECTION_SUMMARY,
  },
  ...FIXTURE.control,
  // Open #1 — real.
  {
    channel: IPC.treeDefinition,
    args: { connectionId: CONNECTION_ID, path: ORDER_ITEMS_PATH, refresh: false, tabId: null },
    response: { definition: ORDER_ITEMS_DEFINITION, source: 'server' },
  },
  // Reopen after close — a tree-service cache hit.
  {
    channel: IPC.treeDefinition,
    args: { connectionId: CONNECTION_ID, path: ORDER_ITEMS_PATH, refresh: false, tabId: null },
    response: { definition: ORDER_ITEMS_DEFINITION, source: 'cache' },
  },
  // Explicit refresh click.
  {
    channel: IPC.treeDefinition,
    args: { connectionId: CONNECTION_ID, path: ORDER_ITEMS_PATH, refresh: true, tabId: null },
    response: { definition: ORDER_ITEMS_DEFINITION, source: 'server' },
  },
  {
    channel: IPC.treeDefinition,
    args: { connectionId: CONNECTION_ID, path: WIDE_TABLE_PATH, refresh: false, tabId: null },
    response: { definition: WIDE_TABLE_DEFINITION, source: 'server' },
  },
];

async function switchToSource(view: import('@playwright/test').Locator) {
  await view.locator('[data-testid="definition-pane-source"]').click();
  await expect(view.locator('.monaco-host')).toBeVisible();
}

async function switchToStructure(view: import('@playwright/test').Locator) {
  await view.locator('[data-testid="definition-pane-structure"]').click();
}

test('Definition tab — Structure/Source, columns menu, notes, read-only, cache and refresh', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: FIXTURE.port });
  await connectAndExpand(page, 'Definition DB', 'blue');

  // --- scenario 1: open from the menu, Structure is the default pane ----------------------
  await openRowMenu(page, ORDER_ITEMS_PATH);
  await page.click('[data-testid="menu-item-open-definition"]');
  const orderItemsTab = page.locator('[data-testid="tab"][data-active="true"]');
  await expect(orderItemsTab).toHaveAttribute('data-tab-kind', 'definition');
  const orderItemsTabId = (await orderItemsTab.getAttribute('data-tab-id')) as string;

  const definitionView = page.locator('[data-testid="definition-view"]');
  await expect(definitionView).toBeVisible();
  await expect(definitionView).toHaveAttribute('data-path', ORDER_ITEMS_PATH);
  await expect(definitionView).toHaveAttribute('data-origin', 'composed', { timeout: 15_000 });
  await expect(definitionView).toHaveAttribute('data-source', 'server');
  await expect(definitionView.locator('[data-testid="definition-pane-structure"]')).toHaveAttribute(
    'data-state',
    'on',
  );
  await expect(definitionView.locator('.monaco-host')).toHaveCount(0);

  // --- scenario 2: Structure sections, and the relocated Columns menu ---------------------
  const columnsSection = definitionView.locator('[data-testid="definition-columns"]');
  await expect(columnsSection).toBeVisible();
  await expect(columnsSection).toContainText('id');
  await expect(definitionView.locator('[data-testid="definition-indexes"]')).toBeVisible();
  const constraintsSection = definitionView.locator('[data-testid="definition-constraints"]');
  await expect(constraintsSection).toBeVisible();
  await expect(constraintsSection).toContainText('PK');
  await expect(constraintsSection).toContainText('FK');
  await expect(constraintsSection).toContainText('Check');

  const idColumnRow = columnsSection.locator('tr', { has: page.getByText('id', { exact: true }) });
  await idColumnRow.scrollIntoViewIfNeeded();
  await settleFrames(page);
  await idColumnRow.click({ button: 'right' });
  const contextMenu = page.locator('[data-testid="context-menu"]');
  await expect(contextMenu).toBeVisible();
  const menuIds = await contextMenu
    .locator(':scope > div')
    .evaluateAll((els) =>
      els.map((el) => (el.getAttribute('data-testid') ?? '').replace('menu-item-', '')),
    );
  expect(menuIds).toEqual(['copy-name', 'add-to-projection', 'sort-by']);
  await page.click('[data-testid="menu-item-add-to-projection"]');
  const dataTabAfterProjection = page.locator('[data-testid="tab"][data-active="true"]');
  await expect(dataTabAfterProjection).toHaveAttribute('data-tab-kind', 'data');
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();

  await page.locator(`[data-testid="tab"][data-tab-id="${orderItemsTabId}"]`).click();
  await expect(definitionView).toBeVisible();

  // --- scenario 3: menu coverage ------------------------------------------------------------
  await expandRow(page, VIEWS_FOLDER_PATH);
  await expandRow(page, MATVIEWS_FOLDER_PATH);
  await expandRow(page, SEQUENCES_FOLDER_PATH);
  await expandRow(page, FUNCTIONS_FOLDER_PATH);

  await openRowMenu(page, ORDER_ITEMS_PATH);
  await expect(page.locator('[data-testid="menu-item-open-definition"]')).toBeVisible();
  await page.keyboard.press('Escape');

  await openRowMenu(page, ORDER_SUMMARY_PATH);
  await expect(page.locator('[data-testid="menu-item-open-definition"]')).toBeVisible();
  await page.keyboard.press('Escape');

  await openRowMenu(page, CUSTOMER_TOTALS_PATH);
  await expect(page.locator('[data-testid="menu-item-open-definition"]')).toBeVisible();
  await page.keyboard.press('Escape');

  await openRowMenu(page, INVOICE_SEQ_PATH);
  await expect(page.locator('[data-testid="menu-item-open-definition"]')).toHaveCount(0);
  await page.keyboard.press('Escape');

  await openRowMenu(page, FULL_NAME_PATH);
  await expect(page.locator('[data-testid="menu-item-open-definition"]')).toHaveCount(0);
  await page.keyboard.press('Escape');

  // --- scenario 4: highlighting is live (Source pane) ---------------------------------------
  await switchToSource(definitionView);
  await expect(definitionView.locator('[data-testid="definition-pane-source"]')).toHaveAttribute(
    'data-state',
    'on',
  );
  expect(await editorText(definitionView)).toContain('CREATE TABLE app.order_items');
  expect(await definitionView.locator('.view-lines span').count()).toBeGreaterThan(0);

  // --- scenario 4b: search (item U, D14) — find-in-document over Source, a plain substring
  // filter over Structure's own columns/indexes/constraints rows ----------------------------
  await page.click('[data-testid="definition-search-toggle"]');
  const findBar = definitionView.locator('[data-testid="http-find-bar"]');
  await expect(findBar).toBeVisible();
  await page.fill('[data-testid="http-find-input"]', 'PRIMARY KEY');
  await expect(definitionView.locator('[data-testid="http-find-count"]')).toContainText('1 of 1');
  await page.keyboard.press('Escape');
  await expect(findBar).toHaveCount(0);

  await switchToStructure(definitionView);
  await page.click('[data-testid="definition-search-toggle"]');
  const structureFilter = definitionView.locator('[data-testid="definition-structure-filter"]');
  await expect(structureFilter).toBeVisible();
  await structureFilter.fill('quantity');
  await expect(columnsSection).toContainText('quantity');
  await expect(columnsSection).not.toContainText('order_id');
  await structureFilter.fill('');
  await expect(columnsSection).toContainText('order_id');
  await switchToSource(definitionView);

  // --- scenario 5: read-only (Source pane) --------------------------------------------------
  const beforeType = await editorText(definitionView);
  await definitionView.locator('.view-lines').click();
  await page.keyboard.type('DROP TABLE x;');
  expect(await editorText(definitionView)).toBe(beforeType);
  await expect(definitionView).toHaveAttribute('data-read-only-reason', 'definition-not-editable');

  // --- scenario 6: notes (Source pane) -------------------------------------------------------
  await expect(page.locator('[data-testid="definition-notes"]')).toContainText(/trigger/i);

  // --- scenario 7: cache and refresh -----------------------------------------------------------
  await openRowMenu(page, WIDE_TABLE_PATH);
  await page.click('[data-testid="menu-item-open-definition"]');
  await expect(page.locator('[data-testid="definition-view"]')).toHaveAttribute(
    'data-path',
    WIDE_TABLE_PATH,
  );

  await page.locator(`[data-testid="tab"][data-tab-id="${orderItemsTabId}"]`).click();
  await expect(definitionView).toHaveAttribute('data-path', ORDER_ITEMS_PATH);

  await page.locator(`[data-testid="tab"][data-tab-id="${orderItemsTabId}"] .tab-close`).click();
  await openRowMenu(page, ORDER_ITEMS_PATH);
  await page.click('[data-testid="menu-item-open-definition"]');
  await expect(definitionView).toHaveAttribute('data-path', ORDER_ITEMS_PATH);
  await expect(definitionView).toHaveAttribute('data-source', 'cache');

  await page.click('[data-testid="definition-refresh"]');
  await expect(definitionView).toHaveAttribute('data-source', 'server');

  // --- scenario 8: two tabs, one target -------------------------------------------------------
  const tabsBefore = await page.locator('[data-testid="tab"]').count();
  await openRowMenu(page, ORDER_ITEMS_PATH);
  await page.click('[data-testid="menu-item-open-definition"]');
  expect(await page.locator('[data-testid="tab"]').count()).toBe(tabsBefore);
  await expect(page.locator('[data-testid="tab"][data-active="true"]')).toHaveAttribute(
    'data-tab-kind',
    'definition',
  );

  // --- scenario 9: data and definition side by side -------------------------------------------
  await openRowMenu(page, ORDER_ITEMS_PATH);
  await page.click('[data-testid="menu-item-open-data-new-tab"]');
  const dataTab = page.locator('[data-testid="tab"][data-active="true"]');
  await expect(dataTab).toHaveAttribute('data-tab-kind', 'data');
  const dataTabId = (await dataTab.getAttribute('data-tab-id')) as string;
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await expect(page.locator('[data-testid="cell-editor"]')).toHaveCount(0);

  const currentDefinitionTab = page
    .locator('[data-testid="tab"][data-tab-kind="definition"]')
    .first();
  const currentDefinitionTabId = (await currentDefinitionTab.getAttribute('data-tab-id')) as string;
  expect(currentDefinitionTabId).not.toBe(dataTabId);
  await currentDefinitionTab.click();
  await expect(page.locator('[data-testid="definition-view"]')).toBeVisible();
  await expect(page.locator('[data-testid="data-grid"]')).toHaveCount(0);

  await page.locator(`[data-testid="tab"][data-tab-id="${dataTabId}"]`).click();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await expect(page.locator('[data-testid="definition-view"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="cell-editor"]')).toHaveCount(0);

  await currentDefinitionTab.click();
  await expect(page.locator('[data-testid="definition-view"]')).toBeVisible();
});

test('Definition tab — tree grouping: folders collapsed by default, zero-IPC expand', async ({
  relaunch,
}) => {
  const RO_CONNECTION_ID = 'conn-definition-grouping';
  const RO_FIXTURE = orderItemsFixture(RO_CONNECTION_ID);
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: [] },
      {
        channel: IPC.connectionsCreate,
        args: connectionCreateArgs('Grouping DB', 'green'),
        response: postgresConnectionSummary(RO_CONNECTION_ID, 'Grouping DB', 'green'),
      },
      ...RO_FIXTURE.control,
    ],
    stream: RO_FIXTURE.port,
  });
  await connectAndExpand(page, 'Grouping DB', 'green');

  // Tables render first, ungrouped, ahead of any folder.
  const wideTableRow = await findRow(page, WIDE_TABLE_PATH);
  await expect(wideTableRow).toBeVisible();
  await expect(wideTableRow.locator('[data-testid="tree-twisty"]')).not.toBeVisible();

  for (const path of [
    VIEWS_FOLDER_PATH,
    MATVIEWS_FOLDER_PATH,
    SEQUENCES_FOLDER_PATH,
    FUNCTIONS_FOLDER_PATH,
  ]) {
    const folder = await findRow(page, path);
    await expect(folder).toBeVisible();
    await expect(folder).toHaveAttribute('data-kind', 'group');
  }

  // Collapsed by default — the Sequences folder's own member isn't rendered until toggled,
  // and toggling costs zero network calls: a pure render over the schema's already-fetched
  // children (no treeChildren fixture entry exists for the folder path itself — a real call here
  // would 422 as E_FIXTURE_MISS, so a passing test already proves the claim).
  expect(await page.locator(`[data-path="${INVOICE_SEQ_PATH}"]`).count()).toBe(0);
  await (await findRow(page, SEQUENCES_FOLDER_PATH)).locator('[data-testid="tree-twisty"]').click();
  await expect(await findRow(page, INVOICE_SEQ_PATH)).toBeVisible();
});
