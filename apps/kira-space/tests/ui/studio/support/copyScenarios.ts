import { expect, type Locator, type Page } from '@playwright/test';
import { DATA_OP } from '@shared/protocol/dataOps';
import type { ColumnDescriptor } from '@shared/protocol/page';
import type { ControlSnapshot, PortSnapshot } from '../../../ipc/support/types';
import { IPC } from './ipcChannels';
import {
  APP_PATH,
  COMPOSITE_PK_PATH,
  compositePkConnectAndOpen,
  DB_PATH,
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './postgresFixture';
import { connectionRow, expandRow, findRow, openRowMenu, settleFrames } from './tree';

// Shared by copy-wording.spec.ts and copy-coverage.spec.ts: one connected Postgres connection with
// app.composite_pk's three rows, so both specs reach the same grid and menus.

export const PG_CONN = postgresConnectionSummary('conn-copy', 'Copy DB', 'blue');
const FIXTURE = compositePkConnectAndOpen(PG_CONN.id);

export const PG_CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [PG_CONN] },
  ...FIXTURE.control,
];
export const PG_PORT: PortSnapshot[] = FIXTURE.port;

/** Fixture data that legitimately shows without a catalog marker. */
export const PG_ALLOW: RegExp[] = [
  /Copy DB/,
  /kira_test/,
  /composite_pk/,
  /tenant|entity|postgres|127\.0\.0\.1/i,
];

export async function connectPostgres(page: Page): Promise<void> {
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);
}

export async function openCompositeTable(page: Page): Promise<void> {
  await connectPostgres(page);
  await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
}

/** Right-clicks and waits for the menu to stay open; SlickGrid can re-render under the click. */
export async function openContextMenu(target: Locator): Promise<Locator> {
  const page = target.page();
  const menu = page.locator('[data-testid="context-menu"]');
  for (let attempt = 0; attempt < 4; attempt++) {
    try {
      await target.scrollIntoViewIfNeeded();
      await settleFrames(page);
      await target.click({ button: 'right' });
      await expect(menu).toBeVisible();
      await settleFrames(page);
      if (await menu.isVisible()) return menu;
    } catch (error) {
      if (attempt === 3) throw error;
    }
  }
  await expect(menu).toBeVisible();
  return menu;
}

/** Visible item labels of the open context menu, in order. */
export async function menuLabels(page: Page): Promise<string[]> {
  return page
    .locator(
      '[data-testid="context-menu"] [data-testid^="menu-item-"]:not([data-testid$="-shortcut"])',
    )
    .evaluateAll((els) =>
      els.map((el) => (el.querySelector('.flex-1') ?? el).textContent?.trim() ?? ''),
    );
}

// order_items scenario: console with saved queries and an Explain result, plus the definition tab.

export const ITEMS_CONN = postgresConnectionSummary('conn-copy-items', 'Items DB', 'green');
const ITEMS = orderItemsFixture(ITEMS_CONN.id);
export const ITEMS_SQL = 'SELECT * FROM app.order_items';

const PLAN_JSON = JSON.stringify([
  {
    Plan: {
      'Node Type': 'Seq Scan',
      'Relation Name': 'order_items',
      Alias: 'order_items',
      'Total Cost': 3582.0,
      'Plan Rows': 184153,
      Filter: '(quantity > 3)',
    },
  },
]);

const PLAN_COLUMN: ColumnDescriptor = {
  name: 'QUERY PLAN',
  dataType: 'json',
  typeClass: 'json',
  nullable: false,
  isPrimaryKey: false,
  generated: false,
};

const DEFINITION = {
  path: ORDER_ITEMS_PATH,
  kind: 'table' as const,
  qualifiedName: 'app.order_items',
  statements: [
    'CREATE TABLE app.order_items (\n  id integer NOT NULL,\n  quantity integer DEFAULT 1 NOT NULL\n)',
    'ALTER TABLE app.order_items ADD CONSTRAINT order_items_pkey PRIMARY KEY (id)',
    'ALTER TABLE app.order_items ADD CONSTRAINT order_items_quantity_positive CHECK (quantity > 0)',
  ],
  language: 'sql' as const,
  origin: 'composed' as const,
  notes: [
    'Composed from catalog metadata: triggers, row-level security policies, grants and ownership are not included.',
  ],
  constraints: [
    { name: 'order_items_pkey', type: 'primaryKey' as const, definition: 'PRIMARY KEY (id)' },
    {
      name: 'order_items_quantity_positive',
      type: 'check' as const,
      definition: 'CHECK (quantity > 0)',
    },
  ],
  documentSchema: null,
  sections: [],
  generatedAt: '2026-01-01T00:00:00.000Z',
};

export const ITEMS_CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [ITEMS_CONN] },
  ...ITEMS.control,
  {
    channel: IPC.queriesListConsole,
    args: { connectionId: ITEMS_CONN.id, path: ORDER_ITEMS_PATH },
    response: [
      {
        id: 'saved-1',
        connectionId: ITEMS_CONN.id,
        path: ORDER_ITEMS_PATH,
        name: 'Big orders',
        pinned: false,
        createdAt: '2026-01-01T00:00:00.000Z',
        usedAt: null,
        body: { text: ITEMS_SQL },
      },
    ],
  },
  {
    channel: IPC.treeDefinition,
    args: { connectionId: ITEMS_CONN.id, path: ORDER_ITEMS_PATH, refresh: false, tabId: null },
    response: { definition: DEFINITION, source: 'server' },
  },
];

export const ITEMS_PORT: PortSnapshot[] = [
  ...ITEMS.port,
  {
    op: DATA_OP.execute,
    payload: {
      connectionId: ITEMS_CONN.id,
      path: ORDER_ITEMS_PATH,
      statements: [
        `EXPLAIN (FORMAT JSON, COSTS TRUE, VERBOSE FALSE, SETTINGS FALSE, BUFFERS FALSE) ${ITEMS_SQL}`,
      ],
    },
    response: {
      kind: 'execute',
      pages: [
        {
          kind: 'tabular',
          columns: [PLAN_COLUMN],
          rows: [[PLAN_JSON]],
          position: {
            offset: 0,
            pageSize: 1,
            hasMore: false,
            nextToken: null,
            prevToken: null,
            strategy: 'offset',
          },
          truncatedCells: 0,
        },
      ],
    },
  },
];

export const ITEMS_ALLOW: RegExp[] = [
  ...PG_ALLOW,
  /Items DB|order_items|Big orders|SELECT|184,153/,
];

/** Opens a connected connection's own tree root, expanded to the schema holding `path`'s table. */
export async function openItemsConsole(page: Page): Promise<void> {
  await connectPostgres(page);
  await openRowMenu(page, ORDER_ITEMS_PATH);
  await page.click('[data-testid="menu-item-open-console"]');
  await expect(page.locator('[data-testid="console-view"]')).toBeVisible();
}
