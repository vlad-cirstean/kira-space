import type { Page } from '@playwright/test';
import { DATA_OP } from '@shared/protocol/dataOps';
import { CAPTURE_LAUNCH } from '@workbench/testing/screenshots/capture';
import type { ControlSnapshot, LogicalPage, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../../ui/fixtures';
import { openHttpModeAndNewRequest } from '../../ui/studio/support/apiMode';
import { typeInto } from '../../ui/studio/support/editor';
import { gutterCell } from '../../ui/studio/support/grid';
import { connectionRow, expandRow, findRow, openRowMenu } from '../../ui/studio/support/tree';
import { IPC } from '../../ui/support/ipcChannels';
import { shoot } from '../support/shoot';
import {
  ACME,
  CONNECTION_ID,
  col,
  DB_PATH,
  ORDERS_PATH,
  SCHEMA_PATH,
  SUMMARY,
} from './support/acme';

// P280: presentation screenshots of Kira Studio, from the production bundle against the mocked
// runtime. Run by `bun run screenshots`; output goes to /screenshots.

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [SUMMARY] },
  { channel: IPC.repoConfigList, response: { dir: '~/.kira-space/repos', files: [] } },
  { channel: IPC.adeTaskRepos, response: { repos: [], folders: [] } },
  { channel: IPC.adeTaskSessions, response: { sessions: [] } },
  ...ACME.control,
];

async function connectAndExpand(page: Page): Promise<void> {
  const row = connectionRow(page);
  await expect(row).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(row.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, SCHEMA_PATH);
}

test('studio data grid', async ({ relaunch, consoleErrors }) => {
  const { window: page } = await relaunch({
    ...CAPTURE_LAUNCH,
    control: CONTROL,
    stream: ACME.port,
  });
  await connectAndExpand(page);
  await (await findRow(page, ORDERS_PATH)).dblclick();

  const grid = page.locator('[data-testid="data-grid"]');
  await expect(grid).toBeVisible();
  const rows = page.locator('[data-testid="grid-row"]');
  await expect(rows.first()).toBeVisible({ timeout: 15_000 });
  await expect(grid).toContainText('Alice Morgan');
  await rows.nth(2).click();

  await shoot(page, 'studio-data-grid', consoleErrors);

  await gutterCell(page, 4).click({ button: 'right' });
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await shoot(page, 'studio-grid-row-menu', consoleErrors);
});

const QUERY = [
  'select c.name as customer, count(*) as orders, sum(o.total) as revenue',
  'from orders o',
  'join customers c on c.id = o.customer_id',
  "where o.status in ('paid', 'shipped', 'delivered')",
  'group by c.name',
  'order by revenue desc;',
].join('\n');

const RESULT_PAGE: LogicalPage = {
  kind: 'tabular',
  columns: [
    col('customer', 'text', 'text'),
    col('orders', 'bigint', 'number'),
    col('revenue', 'numeric', 'number'),
  ],
  rows: [
    ['Alice Morgan', '14', '2318.50'],
    ['Ben Carter', '11', '1904.25'],
    ['Chloe Hughes', '9', '1650.00'],
    ['Daniel Brooks', '8', '1388.75'],
    ['Emma Wright', '6', '972.40'],
    ['Oliver Grant', '5', '815.10'],
  ],
  position: {
    offset: 0,
    pageSize: 6,
    hasMore: false,
    nextToken: null,
    prevToken: null,
    strategy: 'offset',
  },
  truncatedCells: 0,
};

test('studio sql console', async ({ relaunch, consoleErrors }) => {
  const execute: PortSnapshot = {
    op: DATA_OP.execute,
    payload: {
      connectionId: CONNECTION_ID,
      path: ORDERS_PATH,
      statements: [QUERY.replace(/;$/, '')],
    },
    response: { kind: 'execute', pages: [RESULT_PAGE] },
  };
  const { window: page } = await relaunch({
    ...CAPTURE_LAUNCH,
    control: CONTROL,
    stream: [...ACME.port, execute],
  });
  await connectAndExpand(page);
  await openRowMenu(page, ORDERS_PATH);
  await page.click('[data-testid="menu-item-open-console"]');

  const view = page.locator('[data-testid="console-view"]');
  await expect(view).toBeVisible();
  await typeInto(view, page, QUERY, { paste: true });
  await expect(view.locator('.view-lines')).toContainText('order by revenue desc');
  await page.click('[data-testid="console-run-all"]');
  await expect(view.locator('[data-testid="console-result-grid"]')).toContainText('Alice Morgan');

  // Monaco's caret blink is JS-driven: blur the editor so two runs match.
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
  await expect(view.locator('.monaco-editor.focused')).toHaveCount(0);

  await shoot(page, 'studio-sql-console', consoleErrors);
});

const API_STAMP = '2026-09-20T09:00:00Z';
const apiCollection = (id: string, name: string, order: number) => ({
  id,
  name,
  file: `${name.toLowerCase().replace(/ /g, '-')}.collection.yaml`,
  sortOrder: order,
  createdAt: API_STAMP,
  updatedAt: API_STAMP,
});
const apiItem = (
  collectionId: string,
  id: string,
  kind: 'request' | 'folder',
  name: string,
  method: string,
  url: string,
  order: number,
  parentId: string | null = null,
) => ({
  collectionId,
  id,
  kind,
  name,
  method,
  url,
  parentId,
  protocol: 'http',
  sortOrder: order,
  createdAt: API_STAMP,
  updatedAt: API_STAMP,
});
const API_TREE = {
  collections: [
    apiCollection('col-orders', 'Orders API', 0),
    apiCollection('col-customers', 'Customers API', 1),
    apiCollection('col-billing', 'Billing', 2),
  ],
  items: [
    apiItem('col-orders', 'o-list', 'request', 'List orders', 'GET', '/v1/orders', 0),
    apiItem('col-orders', 'o-get', 'request', 'Get order', 'GET', '/v1/orders/4217', 1),
    apiItem('col-orders', 'o-create', 'request', 'Create order', 'POST', '/v1/orders', 2),
    apiItem('col-orders', 'o-cancel', 'request', 'Cancel order', 'DELETE', '/v1/orders/4217', 3),
    apiItem('col-customers', 'c-list', 'request', 'List customers', 'GET', '/v1/customers', 0),
    apiItem('col-customers', 'c-get', 'request', 'Get customer', 'GET', '/v1/customers/88', 1),
    apiItem(
      'col-customers',
      'c-update',
      'request',
      'Update customer',
      'PUT',
      '/v1/customers/88',
      2,
    ),
    apiItem('col-billing', 'b-invoices', 'request', 'List invoices', 'GET', '/v1/invoices', 0),
    apiItem('col-billing', 'b-refund', 'request', 'Refund payment', 'POST', '/v1/refunds', 1),
  ],
};

test('studio api request', async ({ relaunch, consoleErrors }) => {
  const body = JSON.stringify(
    {
      id: 4217,
      customer: 'Alice Morgan',
      status: 'shipped',
      items: [
        { sku: 'KB-104', qty: 1, price: 89.9 },
        { sku: 'MS-220', qty: 2, price: 24.5 },
      ],
      total: 138.9,
    },
    null,
    2,
  );
  const response = {
    status: 200,
    statusText: 'OK',
    proto: 'HTTP/1.1',
    headers: [
      { name: 'content-type', value: 'application/json; charset=utf-8' },
      { name: 'cache-control', value: 'no-cache' },
      { name: 'x-request-id', value: '9f3c2a71' },
    ],
    body,
    bodyEncoding: 'utf8',
    bodyBytes: body.length,
    bodyTruncated: false,
    elapsedMs: 84,
    finalUrl: 'https://api.acme.example/v1/orders/4217?status=shipped&expand=items&limit=25',
    redirects: [],
  };
  const { window: page } = await relaunch({
    ...CAPTURE_LAUNCH,
    control: [
      { channel: IPC.collectionsList, response: API_TREE },
      { channel: IPC.apiConfigView, response: { files: [], secrets: [] } },
      { channel: IPC.repoConfigList, response: { dir: '~/.kira-space/repos', files: [] } },
      { channel: IPC.adeTaskRepos, response: { repos: [], folders: [] } },
      { channel: IPC.adeTaskSessions, response: { sessions: [] } },
      { channel: IPC.httpSend, response },
    ],
  });
  await openHttpModeAndNewRequest(page);
  for (const id of ['col-orders', 'col-customers']) {
    await page
      .locator(`[data-testid="collection-row"][data-id="${id}"] [data-testid="tree-twisty"]`)
      .click();
  }
  await expect(page.getByText('Get customer')).toBeVisible();
  await page.fill(
    '[data-testid="http-url"]',
    'https://api.acme.example/v1/orders/4217?status=shipped&expand=items&limit=25',
  );
  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toHaveText('200 OK');
  await expect(page.locator('[data-testid="http-request-view"]')).toContainText('Alice Morgan');
  // Monaco tokenizes JSON asynchronously; an uncoloured body would differ between runs.
  await expect
    .poll(() =>
      page
        .locator('[data-testid="http-request-view"] .view-line span[class^="mtk"]')
        .evaluateAll((els) => new Set(els.map((el) => el.className)).size),
    )
    .toBeGreaterThan(2);

  await shoot(page, 'studio-api', consoleErrors);
});
