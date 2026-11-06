import type { Page } from '@playwright/test';
import { DATA_OP } from '@shared/protocol/dataOps';
import { CAPTURE_LAUNCH, captureShot } from '@workbench/testing/screenshots/capture';
import type { ControlSnapshot, LogicalPage, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../../ui/fixtures';
import { openHttpModeAndNewRequest } from '../../ui/studio/support/apiMode';
import { typeInto } from '../../ui/studio/support/editor';
import { connectionRow, expandRow, findRow, openRowMenu } from '../../ui/studio/support/tree';
import { IPC } from '../../ui/support/ipcChannels';
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

  await captureShot(page, { id: 'studio-data-grid', detail: { x: 265, y: 130 }, consoleErrors });
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

  await captureShot(page, { id: 'studio-sql-console', detail: { x: 265, y: 150 }, consoleErrors });
});

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
    finalUrl: 'https://api.acme.example/v1/orders/4217',
    redirects: [],
  };
  const { window: page } = await relaunch({
    ...CAPTURE_LAUNCH,
    control: [
      { channel: IPC.apiConfigView, response: { files: [], secrets: [] } },
      { channel: IPC.httpSend, response },
    ],
  });
  await openHttpModeAndNewRequest(page);
  await page.fill('[data-testid="http-url"]', 'https://api.acme.example/v1/orders/4217');
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

  await captureShot(page, { id: 'studio-api', detail: { x: 270, y: 310 }, consoleErrors });
});
