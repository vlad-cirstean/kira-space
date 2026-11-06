import { DATA_OP } from '@shared/protocol/dataOps';
import type { ColumnDescriptor } from '@shared/protocol/page';
import type { ControlSnapshot, LogicalPage, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { gridCell } from './support/grid';
import {
  POSTGRES_CAPS,
  postgresConnectionSummary,
  SERVER_VERSION,
} from './support/postgresFixture';
import { tabLocator } from './support/tabs';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';

// UI spec: mark a column PII from the grid header menu, verify the preview masks it
// and locks editing everywhere a write could otherwise happen, then turn it off and verify the
// real value and normal editing both return.
//
// A hand-built, minimal tree/table (not postgresFixture.ts's own app.order_items/big_rows/etc) —
// this scenario needs one text column holding a real, redactable-looking value, which none of the
// shared captures were built for, and it needs no other columns' worth of coverage.

const CONNECTION_ID = 'conn-mask-preview';
const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Mask Preview DB', 'green');

const ROOT_CHILDREN = [
  { kind: 'database', name: 'kira_test', path: 'database:kira_test', hasChildren: true },
];
const DB_PATH = 'database:kira_test';
const DB_CHILDREN = [
  { kind: 'schema', name: 'app', path: `${DB_PATH}/schema:app`, hasChildren: true },
];
const APP_PATH = `${DB_PATH}/schema:app`;
const CUSTOMERS_PATH = `${APP_PATH}/table:customers`;
const APP_CHILDREN = [
  { kind: 'table', name: 'customers', path: CUSTOMERS_PATH, hasChildren: false, detail: '3 rows' },
];

const CUSTOMERS_COLUMNS: ColumnDescriptor[] = [
  {
    name: 'id',
    dataType: 'integer',
    typeClass: 'number',
    nullable: false,
    isPrimaryKey: true,
    generated: false,
  },
  {
    name: 'email',
    dataType: 'text',
    typeClass: 'text',
    nullable: true,
    isPrimaryKey: false,
    generated: false,
  },
];

// Row 2 repeats row 0's real email — not asserted on directly here (the correlation tag itself is
// mask-parity.spec.ts's own territory), but present so a reader can see the fixture supports it.
const CUSTOMERS_ROWS: (string | null)[][] = [
  ['1', 'maria.gonzalez@acme.example'],
  ['2', null],
  ['3', 'maria.gonzalez@acme.example'],
];

const CUSTOMERS_PAGE: LogicalPage = {
  kind: 'tabular',
  columns: CUSTOMERS_COLUMNS,
  rows: CUSTOMERS_ROWS,
  position: {
    offset: 0,
    pageSize: 100,
    hasMore: false,
    nextToken: null,
    prevToken: null,
    strategy: 'offset',
  },
  truncatedCells: 0,
};

const CUSTOMERS_META = {
  path: `${CUSTOMERS_PATH}`,
  kind: 'table',
  name: 'customers',
  qualifiedName: 'app.customers',
  columns: [
    {
      name: 'id',
      position: 1,
      dataType: 'integer',
      nullable: false,
      defaultExpr: null,
      isPrimaryKey: true,
      comment: null,
    },
    {
      name: 'email',
      position: 2,
      dataType: 'text',
      nullable: true,
      defaultExpr: null,
      isPrimaryKey: false,
      comment: null,
    },
  ],
  primaryKey: ['id'],
  foreignKeys: [],
  referencedBy: [],
  indexes: [],
  rowEstimate: 3,
  comment: null,
};

const NOW = '2026-01-01T00:00:00.000Z';
const EMAIL_RULE = {
  id: 'rule-email-1',
  connectionId: CONNECTION_ID,
  tableName: 'app.customers',
  columnName: 'email',
  kind: 'email' as const,
  keepHint: true,
  correlate: true,
  createdAt: NOW,
  updatedAt: NOW,
};

test('grid mask preview — mark a column PII, preview masks and locks editing, then unmasks', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [CONNECTION_SUMMARY] },
    {
      channel: IPC.connectionsConnect,
      args: { id: CONNECTION_ID },
      response: {
        connectionId: CONNECTION_ID,
        status: 'connected',
        serverVersion: SERVER_VERSION,
        error: null,
        since: 1735689600000,
        caps: POSTGRES_CAPS,
      },
    },
    {
      channel: IPC.treeChildren,
      args: { connectionId: CONNECTION_ID, path: '', refresh: false },
      response: { nodes: ROOT_CHILDREN, source: 'server', truncated: false },
    },
    {
      channel: IPC.treeChildren,
      args: { connectionId: CONNECTION_ID, path: DB_PATH, refresh: false },
      response: { nodes: DB_CHILDREN, source: 'server', truncated: false },
    },
    {
      channel: IPC.treeChildren,
      args: { connectionId: CONNECTION_ID, path: APP_PATH, refresh: false },
      response: { nodes: APP_CHILDREN, source: 'server', truncated: false },
    },
    {
      channel: IPC.treeDescribe,
      args: { connectionId: CONNECTION_ID, path: CUSTOMERS_PATH, refresh: false, tabId: null },
      response: { meta: CUSTOMERS_META, source: 'server' },
    },
    // Two queued answers for the same (channel, args) key (mockRuntime.ts's own documented
    // "on purpose" duplicate-key support): the grid's own mount-time load (empty — no rules yet),
    // then the reload markColumnMaskKind triggers right after Upsert (the rule now exists).
    { channel: IPC.maskRulesList, args: { connectionId: CONNECTION_ID }, response: [] },
    {
      channel: IPC.maskRulesList,
      args: { connectionId: CONNECTION_ID },
      response: [EMAIL_RULE],
    },
    {
      channel: IPC.maskRulesUpsert,
      args: {
        connectionId: CONNECTION_ID,
        fields: {
          tableName: 'app.customers',
          columnName: 'email',
          kind: 'email',
          keepHint: true,
          correlate: true,
        },
      },
      response: EMAIL_RULE,
    },
    // "" — no correlation key needed for this test: the redaction itself is what's under test,
    // not the correlation tag (mask-parity.spec.ts's own territory). A cache miss here degrades
    // to the redaction alone, never a leak (maskPreview.ts's own fail-closed comment).
    {
      channel: IPC.maskRulesCorrelationKey,
      args: { connectionId: CONNECTION_ID },
      response: '',
    },
  ];
  const PORT: PortSnapshot[] = [
    {
      op: DATA_OP.read,
      payload: {
        connectionId: CONNECTION_ID,
        path: CUSTOMERS_PATH,
        projection: null,
        filter: null,
        sort: null,
        pageSize: 100,
        cursor: { mode: 'offset', offset: 0 },
      },
      response: { kind: 'read', page: CUSTOMERS_PAGE, source: 'server' },
    },
  ];

  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });

  const connRow = connectionRow(page, 'Mask Preview DB');
  await expect(connRow).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);

  const tableRow = await findRow(page, CUSTOMERS_PATH);
  await tableRow.dblclick();
  const grid = page.locator('[data-testid="data-grid"]');
  await expect(grid).toBeVisible();
  await expect(gridCell(page, 0, 'email')).toHaveText('maria.gonzalez@acme.example');

  // Before marking: the toggle is absent entirely (deleteRowTooltip's own standing rule — never a
  // permanently inert control) since this connection has no masked columns yet.
  await expect(page.locator('[data-testid="toolbar-mask-preview"]')).toHaveCount(0);

  // Mark the email column PII via the header menu (§6.7) — this both writes the rule and turns
  // the preview on for this tab immediately (menu.ts's own markColumnMaskKind).
  await page.click('[data-testid="grid-header-cell"][data-column="email"]', { button: 'right' });
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await page.locator('[data-testid="menu-item-mark-pii"]').hover();
  await expect(page.locator('[data-testid="context-submenu"]')).toBeVisible();
  await page.click('[data-testid="menu-item-mask-email"]');
  await expect(page.locator('[data-testid="context-menu"]')).toHaveCount(0);

  // The preview is now on and the redaction shows — kept (email) and destroyed (everything else),
  // per internal/mask's own maskEmail: first grapheme + length of the local part, full domain.
  const toggle = page.locator('[data-testid="toolbar-mask-preview"]');
  await expect(toggle).toBeVisible();
  await expect(toggle).toHaveAttribute('aria-pressed', 'true');
  await expect(gridCell(page, 0, 'email')).toHaveText('m•••••••••••••@acme.example');
  await expect(gridCell(page, 0, 'email')).toHaveClass(/cell-masked/);
  // NULL stays NULL — masking a NULL would invent a value that is not there (§2.3).
  await expect(gridCell(page, 1, 'email')).toHaveText('NULL');
  await expect(page.locator('[data-testid="mask-preview-strip"]')).toBeVisible();
  await expect(page.locator('[data-testid="grid-writable-badge"]')).toHaveText('Read-only');

  // Edit lockout (§6.4): the grid is read-only while masked, and the cell editor panel shows the
  // 'masked' reason first.
  await expect(page.locator('[data-testid="toolbar-add-row"]')).toBeDisabled();
  await expect(page.locator('[data-testid="toolbar-delete-row"]')).toBeDisabled();
  await gridCell(page, 0, 'email').click();
  const panel = page.locator('[data-testid="cell-editor-panel"]');
  await expect(panel).toHaveAttribute('data-read-only-reason', 'masked');
  await gridCell(page, 0, 'email').dblclick();
  await expect(page.locator('[data-testid="grid-cell-input"]')).toHaveCount(0);

  // Turn the preview off: the real value and normal editing both return.
  await toggle.click();
  await expect(toggle).toHaveAttribute('aria-pressed', 'false');
  await expect(gridCell(page, 0, 'email')).toHaveText('maria.gonzalez@acme.example');
  await expect(gridCell(page, 0, 'email')).not.toHaveClass(/cell-masked/);
  await expect(page.locator('[data-testid="mask-preview-strip"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="grid-writable-badge"]')).toHaveText('Read-write');
  await expect(page.locator('[data-testid="toolbar-add-row"]')).toBeEnabled();
  await gridCell(page, 0, 'email').click();
  await expect(panel).not.toHaveAttribute('data-read-only-reason');

  // M7 finding #5: turn the preview back on, then switch away and back to this same tab (a table
  // row's own "Open query console" needs no extra IPC mocking — it registers editor commands only,
  // ConsoleSavedMenu.vue's own queriesListConsole fetch is gated behind a popover this never opens).
  // SlickGridHost remounts on every tab switch (MainView.vue's `:key="activeTab.id"`), resetting
  // its component-scoped mask state — before this fix that reset left the remounted grid showing
  // real values while the toggle and badge still read "masked"/"read-only".
  await toggle.click();
  await expect(toggle).toHaveAttribute('aria-pressed', 'true');
  await expect(gridCell(page, 0, 'email')).toHaveText('m•••••••••••••@acme.example');

  const customersTabId = await page
    .locator('[data-testid="tab"]')
    .first()
    .getAttribute('data-tab-id');
  if (!customersTabId) throw new Error('expected the customers tab to have an id');

  await openRowMenu(page, CUSTOMERS_PATH);
  await page.click('[data-testid="menu-item-open-console"]');
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(2);
  await expect(page.locator('[data-testid="data-grid"]')).toHaveCount(0);

  await tabLocator(page, customersTabId).click();
  await expect(grid).toBeVisible();
  await expect(toggle).toHaveAttribute('aria-pressed', 'true');
  await expect(gridCell(page, 0, 'email')).toHaveText('m•••••••••••••@acme.example');
  await expect(gridCell(page, 0, 'email')).toHaveClass(/cell-masked/);
  await expect(page.locator('[data-testid="grid-writable-badge"]')).toHaveText('Read-only');
});

// correlationKeyFor's own IPC call can reject (an unwrapped business-rule failure,
// tests/ipc's own `error` snapshot shape) — before the fix this "failed open": the preview stayed
// on (toggle active, badge "read-only") while the grid's own dataSource was never updated to apply
// any transform at all, since the render tail after the failed await never ran. The fix turns the
// preview back off and surfaces the failure instead.
test('grid mask preview — turns itself back off when the correlation key fetch fails', async ({
  relaunch,
}) => {
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [CONNECTION_SUMMARY] },
    {
      channel: IPC.connectionsConnect,
      args: { id: CONNECTION_ID },
      response: {
        connectionId: CONNECTION_ID,
        status: 'connected',
        serverVersion: SERVER_VERSION,
        error: null,
        since: 1735689600000,
        caps: POSTGRES_CAPS,
      },
    },
    {
      channel: IPC.treeChildren,
      args: { connectionId: CONNECTION_ID, path: '', refresh: false },
      response: { nodes: ROOT_CHILDREN, source: 'server', truncated: false },
    },
    {
      channel: IPC.treeChildren,
      args: { connectionId: CONNECTION_ID, path: DB_PATH, refresh: false },
      response: { nodes: DB_CHILDREN, source: 'server', truncated: false },
    },
    {
      channel: IPC.treeChildren,
      args: { connectionId: CONNECTION_ID, path: APP_PATH, refresh: false },
      response: { nodes: APP_CHILDREN, source: 'server', truncated: false },
    },
    {
      channel: IPC.treeDescribe,
      args: { connectionId: CONNECTION_ID, path: CUSTOMERS_PATH, refresh: false, tabId: null },
      response: { meta: CUSTOMERS_META, source: 'server' },
    },
    { channel: IPC.maskRulesList, args: { connectionId: CONNECTION_ID }, response: [] },
    {
      channel: IPC.maskRulesList,
      args: { connectionId: CONNECTION_ID },
      response: [EMAIL_RULE],
    },
    {
      channel: IPC.maskRulesUpsert,
      args: {
        connectionId: CONNECTION_ID,
        fields: {
          tableName: 'app.customers',
          columnName: 'email',
          kind: 'email',
          keepHint: true,
          correlate: true,
        },
      },
      response: EMAIL_RULE,
    },
    // The one difference from the scenario above: this fetch fails instead of degrading to "".
    {
      channel: IPC.maskRulesCorrelationKey,
      args: { connectionId: CONNECTION_ID },
      error: { code: 'E_INTERNAL', message: 'key store unavailable' },
    },
  ];
  const PORT: PortSnapshot[] = [
    {
      op: DATA_OP.read,
      payload: {
        connectionId: CONNECTION_ID,
        path: CUSTOMERS_PATH,
        projection: null,
        filter: null,
        sort: null,
        pageSize: 100,
        cursor: { mode: 'offset', offset: 0 },
      },
      response: { kind: 'read', page: CUSTOMERS_PAGE, source: 'server' },
    },
  ];

  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });

  const connRow = connectionRow(page, 'Mask Preview DB');
  await expect(connRow).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);

  const tableRow = await findRow(page, CUSTOMERS_PATH);
  await tableRow.dblclick();
  const grid = page.locator('[data-testid="data-grid"]');
  await expect(grid).toBeVisible();
  await expect(gridCell(page, 0, 'email')).toHaveText('maria.gonzalez@acme.example');

  await page.click('[data-testid="grid-header-cell"][data-column="email"]', { button: 'right' });
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await page.locator('[data-testid="menu-item-mark-pii"]').hover();
  await expect(page.locator('[data-testid="context-submenu"]')).toBeVisible();
  await page.click('[data-testid="menu-item-mask-email"]');
  await expect(page.locator('[data-testid="context-menu"]')).toHaveCount(0);

  // The toggle exists (this connection has a masked column now) but must not read "on" — the
  // fetch behind it failed — and the grid must show the real value, never a half-applied mask.
  const toggle = page.locator('[data-testid="toolbar-mask-preview"]');
  await expect(toggle).toBeVisible();
  await expect(toggle).toHaveAttribute('aria-pressed', 'false');
  await expect(gridCell(page, 0, 'email')).toHaveText('maria.gonzalez@acme.example');
  await expect(gridCell(page, 0, 'email')).not.toHaveClass(/cell-masked/);
  await expect(page.locator('[data-testid="grid-writable-badge"]')).toHaveText('Read-write');
  await expect(page.locator('[data-testid="data-action-error"]')).toBeVisible();
});
