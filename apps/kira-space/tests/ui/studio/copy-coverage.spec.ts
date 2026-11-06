import type { Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import {
  expectCatalogText,
  setPseudoLocale,
} from '../../../../../packages/workbench/src/testing/ui/copyCoverage';
import {
  controlSnapshots as redisControl,
  portSnapshots as redisPorts,
} from '../../ipc/redis/redis.fixture';
import {
  controlSnapshots as sqsControl,
  portSnapshots as sqsPorts,
} from '../../ipc/sqs/sqs.fixture';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { emitWailsEvent } from '../support/mockRuntime';
import { grpcTab, modeTab, openHttpModeAndNewRequest } from './support/apiMode';
import {
  connectPostgres,
  ITEMS_ALLOW,
  ITEMS_CONTROL,
  ITEMS_PORT,
  ITEMS_SQL,
  openCompositeTable,
  openContextMenu,
  openItemsConsole,
  PG_ALLOW,
  PG_CONTROL,
  PG_PORT,
} from './support/copyScenarios';
import { typeInto } from './support/editor';
import { cellText, gridCell, gutterCell, headerCell } from './support/grid';
import { openKind, openKubernetes, setupKubernetes } from './support/kubernetesFixture';
import {
  DB_PATH as MONGO_DB_PATH,
  mongoConnectionSummary,
  WIDGETS_PATH,
  widgetsFixture,
} from './support/mongoFixture';
import { APP_PATH, COMPOSITE_PK_PATH } from './support/postgresFixture';
import { openSettings } from './support/settings';
import { connectionRow, expandRow, findRow, openBrowseLeaf, openRowMenu } from './support/tree';

// P265: with the pseudo-locale on, every catalog message shows wrapped in ⟦…⟧, so any letter
// outside a marker is a literal that skipped t(). Fixture names go through `allow`. Surfaces reuse the fixtures of the existing UI specs.

// Product and typeface names are proper nouns; `1k`/`10k` are page-size numerals.
const NAMES =
  /^(PostgreSQL|MariaDB|MySQL|SQLite|ClickHouse|MongoDB|Redis|Kafka|SQS|S3|Kibana|Courier New|Menlo|SF Mono|Monaco|Andale Mono|PT Mono|JetBrains Mono|Fira Code|Source Code Pro|IBM Plex Mono|Cascadia Code|Hack|Inconsolata|Roboto Mono|\d+k)$/;
// Column, table and key names from the fixtures: data, not chrome.
const SCHEMA = /^(id|email|name|text|integer|order_id|product_id|quantity|orders|products)$/;
const REDIS = /^(Test Redis \/|events|queue|session|tags|counter|leaderboard)$/;

// Palette ids are raw by decision (Stream A); byte sizes are formatted data; the definition scope
// note is Go-originated (backend text, P266).
const PALETTE = /^(red|amber|green|cyan|blue|magenta|grey)$/;
const BYTES = /^[\d.,]+ (B|KB|MB|GB)$/;
const BACKEND_NOTE = /^Composed from catalog metadata:/;

// The DB MCP review fixtures (dbmcp.json, dbmcp-preview.json): connection name, file, columns.
const DBMCP_FIXTURE = /^(notes db|\/data\/mcp\.db|id|body)$/;

const ALLOW: RegExp[] = [...PG_ALLOW, NAMES, BYTES, DBMCP_FIXTURE];
const loc = (page: Page, testid: string) => page.locator(`[data-testid="${testid}"]`);

test('title bar, mode switcher and status bar', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.connectionsList, response: [] }],
  });
  await setPseudoLocale(page);
  await expectCatalogText(page.locator('.h-titlebar'), { allow: ALLOW });
  await expectCatalogText(page.locator('[data-testid="mode-tab"]'), { allow: ALLOW });
  await expectCatalogText(loc(page, 'status-bar'), { allow: ALLOW });
});

test('project panel header and empty state', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.connectionsList, response: [] }],
  });
  await setPseudoLocale(page);
  await expectCatalogText(loc(page, 'project-panel'), { allow: ALLOW });
});

test('connection row menu', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: PG_CONTROL, stream: PG_PORT });
  await setPseudoLocale(page);
  await openRowMenu(page, '');
  await expectCatalogText(loc(page, 'context-menu'), { allow: ALLOW });
});

test('new connection dialog, both steps', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.connectionsList, response: [] }],
  });
  await setPseudoLocale(page);
  await page.click('[data-testid="add-connection"]');
  const dialog = loc(page, 'new-connection-dialog');
  await expect(dialog).toBeVisible();
  await expectCatalogText(dialog, { allow: [...ALLOW, PALETTE] });
  await page.click('[data-testid="connection-kind-postgres"]');
  await expectCatalogText(dialog, { allow: [...ALLOW, PALETTE] });
});

test('settings dialog, every pane', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: [] });
  await setPseudoLocale(page);
  await openSettings(page);
  for (const section of ['Appearance', 'Data', 'Cache', 'Api', 'Database MCP', 'Advanced']) {
    await loc(page, `settings-section-${section}`).click();
    await expectCatalogText(loc(page, 'settings-dialog'), { allow: [...ALLOW, SCHEMA] });
  }
});

test('data view toolbar, column menu, cell menu and cell editor dock', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: PG_CONTROL, stream: PG_PORT });
  await setPseudoLocale(page);
  await openCompositeTable(page);
  await expectCatalogText(loc(page, 'data-toolbar'), { allow: ALLOW });

  await openContextMenu(headerCell(page, 'name'));
  await expectCatalogText(loc(page, 'context-menu'), { allow: ALLOW });
  await page.keyboard.press('Escape');

  await openContextMenu(gridCell(page, 0, 'name'));
  await expectCatalogText(loc(page, 'context-menu'), { allow: ALLOW });
  await page.keyboard.press('Escape');

  await openContextMenu(gutterCell(page, 0));
  await expectCatalogText(loc(page, 'context-menu'), { allow: ALLOW });
  await page.keyboard.press('Escape');

  await gridCell(page, 0, 'name').click();
  expect(await cellText(page, 0, 'name')).not.toBe('');
  await expectCatalogText(loc(page, 'cell-editor'), { allow: ALLOW });
});

test('generate data dialog', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: PG_CONTROL, stream: PG_PORT });
  await setPseudoLocale(page);
  await openCompositeTable(page);
  await page.click('[data-testid="toolbar-generate-data"]');
  const dialog = loc(page, 'generate-data-dialog');
  await expect(dialog).toBeVisible();
  await expectCatalogText(dialog, { allow: [...ALLOW, SCHEMA] });
});

test('console toolbar', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: PG_CONTROL, stream: PG_PORT });
  await setPseudoLocale(page);
  await connectPostgres(page);
  await openRowMenu(page, COMPOSITE_PK_PATH);
  await page.click('[data-testid="menu-item-open-console"]');
  await expect(loc(page, 'console-view')).toBeVisible();
  await expectCatalogText(loc(page, 'console-view'), { allow: ALLOW });
});

test('command palette', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: [] });
  await setPseudoLocale(page);
  await emitWailsEvent(page, IPC.commandPalette, null);
  await expect(loc(page, 'command-palette-input')).toBeVisible();
  await expectCatalogText(page.locator('[data-slot="dialog-content"]'), { allow: ALLOW });
});

test('operations panel', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: [] });
  await setPseudoLocale(page);
  await page.click('[data-testid="toggle-operations-panel"]');
  await expect(loc(page, 'operations-panel')).toBeVisible();
  await expectCatalogText(loc(page, 'operations-panel'), { allow: ALLOW });
});

test('DB MCP approval dialog', async ({ relaunch }) => {
  const [item] = contract<ApprovalItem[]>('dbmcp', 'ApprovalsService.List');
  if (!item) throw new Error('contract has no item');
  const { window: page } = await relaunch({ control: [] });
  await setPseudoLocale(page);
  await pushApprovals(page, [liveItem(item)]);
  const dialog = loc(page, 'approvals-dialog');
  await expect(dialog).toBeVisible();
  await expectCatalogText(dialog, { allow: ALLOW });
});

test('DB MCP response review dialog', async ({ relaunch }) => {
  const [item] = contract<ApprovalItem[]>('dbmcp-preview', 'ApprovalsService.List');
  if (!item) throw new Error('contract has no item');
  const { window: page } = await relaunch({
    control: [
      {
        channel: IPC.dbMcpPendingResponse,
        response: contract('dbmcp-preview', 'DbMcpService.PendingResponse'),
      },
    ],
  });
  await setPseudoLocale(page);
  await pushApprovals(page, [liveItem(item)]);
  const dialog = loc(page, 'approvals-dialog');
  await expect(loc(page, 'db-mcp-preview-table')).toBeVisible();
  await expectCatalogText(dialog, { allow: ALLOW });
});

test('HTTP request view, every tab, empty response', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: [] });
  await setPseudoLocale(page);
  await openHttpModeAndNewRequest(page);
  await expectCatalogText(loc(page, 'http-request-view'), { allow: ALLOW });
  for (const pane of ['params', 'headers', 'body', 'settings', 'cookies']) {
    await loc(page, `http-request-pane-${pane}`).click();
    await expectCatalogText(loc(page, 'http-request-view'), { allow: ALLOW });
  }
});

test('collections panel empty state', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.collectionsList, response: { collections: [], items: [] } }],
  });
  await setPseudoLocale(page);
  await modeTab(page, 'api').click();
  await expect(loc(page, 'collections-empty')).toBeVisible();
  await expectCatalogText(loc(page, 'project-panel'), { allow: ALLOW });
});

test('environments view', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.collectionsList, response: { collections: [], items: [] } }],
  });
  await setPseudoLocale(page);
  await modeTab(page, 'api').click();
  await page.click('[data-testid="api-environments"]');
  await expect(loc(page, 'environments-dialog')).toBeVisible();
  await expectCatalogText(loc(page, 'environments-dialog'), { allow: ALLOW });
});

test('saved queries menu', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: ITEMS_CONTROL, stream: ITEMS_PORT });
  await setPseudoLocale(page);
  await openItemsConsole(page);
  await page.click('[data-testid="console-saved-toggle"]');
  await expect(loc(page, 'console-saved-entry').first()).toBeVisible();
  await expectCatalogText(loc(page, 'console-saved-menu'), {
    allow: [...ITEMS_ALLOW, NAMES, SCHEMA],
  });
});

test('explain result', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: ITEMS_CONTROL, stream: ITEMS_PORT });
  await setPseudoLocale(page);
  await openItemsConsole(page);
  const view = loc(page, 'console-view');
  await typeInto(view, page, ITEMS_SQL);
  await page.click('[data-testid="console-explain"]');
  const result = view.locator('[data-testid="explain-result-view"]');
  await expect(result).toBeVisible();
  await expectCatalogText(result, { allow: [...ITEMS_ALLOW, NAMES, SCHEMA] });
});

test('definition view, structure and source', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: ITEMS_CONTROL, stream: ITEMS_PORT });
  await setPseudoLocale(page);
  await connectPostgres(page);
  await openRowMenu(page, `${APP_PATH}/table:order_items`);
  await page.click('[data-testid="menu-item-open-definition"]');
  const view = loc(page, 'definition-view');
  await expect(view).toHaveAttribute('data-origin', 'composed', { timeout: 15_000 });
  await expectCatalogText(view, { allow: [...ITEMS_ALLOW, NAMES, SCHEMA, BACKEND_NOTE] });
  await view.locator('[data-testid="definition-pane-source"]').click();
  await expect(view.locator('.monaco-host')).toBeVisible();
  await expectCatalogText(view, { allow: [...ITEMS_ALLOW, NAMES, SCHEMA, BACKEND_NOTE] });
});

test('documents toolbar', async ({ relaunch }) => {
  const conn = mongoConnectionSummary('conn-copy-doc', 'Mongo DB', 'green');
  const fixture = widgetsFixture(conn.id);
  const { window: page } = await relaunch({
    control: [{ channel: IPC.connectionsList, response: [conn] }, ...fixture.control],
    stream: fixture.port,
  });
  await setPseudoLocale(page);
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
  await expandRow(page, MONGO_DB_PATH);
  await (await findRow(page, WIDGETS_PATH)).dblclick();
  await expect(loc(page, 'document-row').first()).toBeVisible({ timeout: 15_000 });
  await expectCatalogText(loc(page, 'document-view'), {
    allow: [...ALLOW, /Mongo DB|widget|kira_test|\d/i],
  });
});

test('key-value view', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: redisControl, stream: redisPorts });
  await setPseudoLocale(page);
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
  const db0 = 'database:db0';
  const ns = `${db0}/namespace:user`;
  const ns1 = `${ns}/namespace:1`;
  const key = `${ns1}/key:user%3A1%3Abighash`;
  await (await findRow(page, db0)).dblclick();
  const browse = loc(page, 'browse-view');
  await expectCatalogText(browse, { allow: [...ALLOW, REDIS, /user|db0|key|\d/i] });
  await browse.locator(`[data-testid="browse-row"][data-path="${ns}"]`).dblclick();
  await browse.locator(`[data-testid="browse-row"][data-path="${ns1}"]`).dblclick();
  await openBrowseLeaf(page, key);
  const view = page.locator(`[data-testid="keyvalue-view"][data-path="${key}"]`);
  await expect(view.locator('[data-testid="keyvalue-row"]').first()).toBeVisible({
    timeout: 15_000,
  });
  await expectCatalogText(view, { allow: [...ALLOW, /user|bighash|field|value|\d/i] });
});

test('stream view', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: sqsControl, stream: sqsPorts });
  await setPseudoLocale(page);
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
  await (await findRow(page, 'queue:orders-queue')).dblclick();
  const view = page.locator('[data-testid="stream-view"][data-path="queue:orders-queue"]');
  await view.locator('[data-testid="stream-poll"]').click();
  await expect(view.locator('[data-testid="stream-row"]').first()).toBeVisible({
    timeout: 15_000,
  });
  await expectCatalogText(view, { allow: [...ALLOW, /orders|queue|sqs|message|\d/i] });
});

test('gRPC request view, every tab', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.tabsList, response: [grpcTab({})] },
      {
        channel: IPC.grpcHistoryList,
        args: { itemId: '', tabId: 'tab-grpc-1' },
        response: [],
      },
    ],
  });
  await setPseudoLocale(page);
  const view = loc(page, 'grpc-request-view');
  await expect(view).toBeVisible();
  await expectCatalogText(view, { allow: ALLOW });
  for (const pane of ['message', 'metadata', 'schema']) {
    await loc(page, `grpc-request-pane-${pane}`).click();
    await expectCatalogText(view, { allow: ALLOW });
  }
  for (const pane of ['messages', 'metadata', 'history']) {
    await loc(page, `grpc-response-pane-${pane}`).click();
    await expectCatalogText(view, { allow: ALLOW });
  }
});

test('collections panel menu', async ({ relaunch }) => {
  const now = '2026-01-01T00:00:00.000Z';
  const { window: page } = await relaunch({
    control: [
      {
        channel: IPC.collectionsList,
        response: {
          collections: [
            { id: 'col-1', name: 'Orders API', sortOrder: 0, createdAt: now, updatedAt: now },
          ],
          items: [
            {
              id: 'item-health',
              collectionId: 'col-1',
              parentId: null,
              kind: 'request',
              name: 'Health check',
              sortOrder: 0,
              method: 'GET',
              url: 'https://api.example.com/healthz',
              createdAt: now,
              updatedAt: now,
            },
          ],
        },
      },
    ],
  });
  await setPseudoLocale(page);
  await modeTab(page, 'api').click();
  const fixtureNames = [...ALLOW, /Orders API|Health check|api\.example\.com|GET/];
  await expectCatalogText(loc(page, 'project-panel'), { allow: fixtureNames });

  await page.locator('[data-testid="collection-row"][data-id="col-1"]').click({ button: 'right' });
  await expect(loc(page, 'context-menu')).toBeVisible();
  await expectCatalogText(loc(page, 'context-menu'), { allow: fixtureNames });
  await page.keyboard.press('Escape');

  await page.locator('[data-testid="collection-row"]').first().focus();
  await page.keyboard.press('ArrowRight');
  await page
    .locator('[data-testid="collection-row"][data-id="item-health"]')
    .click({ button: 'right' });
  await expect(loc(page, 'context-menu')).toBeVisible();
  await expectCatalogText(loc(page, 'context-menu'), { allow: fixtureNames });
});

// Cluster, kind and pod names, versions and table cells are data, not chrome.
const KUBE_DATA = [
  /kind-dev|bad-token|plugin-ctx|down|dev-user|web-\d|db-\d|db-creds|default|kube-system|apps|ReplicaSet|kubelet|v1\.\d+/,
  /^(Deployment|Pod|Service|Secret|Event|Namespace|Node|Widget|v1|apps\/v1|example\.com\/v1|Ready|Status|Restarts|Age|Name|Namespace|Running|Opaque|True|False|Normal|PodScheduled|Scheduled|Started|Pulled|app|web)$/,
  /^\s*[\d/.,]+\s*(m|s|h|d)?\s*$/,
  /container started|image pulled|assigned to node|\bUTC\b|\d{4}-\d{2}-\d{2}|apiVersion|metadata|creationTimestamp|uid/,
];

test('Kubernetes panel, list, describe and logs use catalog text', async ({ relaunch }) => {
  const { page } = await setupKubernetes(relaunch);
  await setPseudoLocale(page);
  await openKubernetes(page);
  await expectCatalogText(loc(page, 'kubernetes-panel'), { allow: KUBE_DATA });

  await openKind(page, 'kind-dev', 'v1/pods');
  const view = loc(page, 'kubernetes-view');
  await expect(loc(page, 'kube-row').first()).toBeVisible();
  await expectCatalogText(view, { allow: KUBE_DATA });

  await loc(page, 'kube-row').first().click();
  await expect(loc(page, 'kube-describe-section-events')).toBeVisible();
  const panel = page.locator('[data-testid="kube-detail"][data-active="true"]');
  await expectCatalogText(panel, { allow: KUBE_DATA });
  await expectCatalogText(loc(page, 'kube-tabs'), { allow: KUBE_DATA });

  await loc(page, 'kube-kind-more').click();
  await expect(loc(page, 'kube-kind-more-content')).toBeVisible();
  await expectCatalogText(loc(page, 'kube-kind-more-content'), { allow: KUBE_DATA });
  await page.keyboard.press('Escape');

  await panel.locator('[data-testid="kube-detail-tab-logs"]').click();
  await expect(loc(page, 'kube-logs')).toBeVisible();
  await expectCatalogText(loc(page, 'kube-logs'), {
    allow: [...KUBE_DATA, /^app$|^init$|^sidecar$/],
  });

  await page.locator('[data-testid="kube-cluster-row"][data-context="bad-token"]').click();
  await expect(loc(page, 'kube-unavailable')).toBeVisible();
  await expectCatalogText(loc(page, 'kube-unavailable'), { allow: KUBE_DATA });
});

test('Kubernetes MCP dialog and approval use catalog text', async ({ relaunch }) => {
  const { page } = await setupKubernetes(relaunch);
  await setPseudoLocale(page);
  await openKubernetes(page);
  await page
    .locator('[data-testid="kube-cluster-row"][data-context="kind-dev"]')
    .click({ button: 'right' });
  await page.locator('[data-testid="menu-item-kube-cluster-mcp"]').click();
  await expect(loc(page, 'kube-mcp-dialog')).toBeVisible();
  await expectCatalogText(loc(page, 'kube-mcp-dialog'), { allow: KUBE_DATA });
});

test('Kubernetes MCP approval dialog', async ({ relaunch }) => {
  const [item] = contract<ApprovalItem[]>('dbmcp-kube', 'ApprovalsService.List');
  if (!item) throw new Error('contract has no item');
  const { window: page } = await relaunch({ control: [] });
  await setPseudoLocale(page);
  await pushApprovals(page, [liveItem(item)]);
  const dialog = loc(page, 'approvals-dialog');
  await expect(dialog).toBeVisible();
  await expectCatalogText(dialog, { allow: [...ALLOW, ...KUBE_DATA, /<kube>|kubectl/] });
});
