import { DATA_OP } from '@shared/protocol/dataOps';
import {
  controlSnapshots as redisControl,
  portSnapshots as redisPorts,
} from '../../ipc/redis/redis.fixture';
import {
  controlSnapshots as sqsControl,
  portSnapshots as sqsPorts,
} from '../../ipc/sqs/sqs.fixture';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, test } from './fixtures';
import { grpcTab, modeTab, openHttpModeAndNewRequest } from './support/apiMode';
import { connectAndExpand, connectionCreateArgs, openConsoleFromMenu } from './support/connect';
import { editorText } from './support/editorText';
import { IPC } from './support/ipcChannels';
import {
  mongoConnectionSummary,
  WIDGETS_BODIES,
  WIDGETS_IDS,
  WIDGETS_PATH,
  widgetsFixture,
} from './support/mongoFixture';
import {
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { openRedisBrowse, REDIS_DB0 } from './support/redisFixture';
import { connectionRow, expandRow, findRow, openBrowseLeaf, openRowMenu } from './support/tree';

// P264: one key set for every list. The project tree is the Studio list bound through the shared
// useListNavigation today (Left/Right and expansion are covered in tree.spec.ts); rows here are
// disconnected connections so no tree IPC is needed.

const CONNS = [
  postgresConnectionSummary('conn-k1', 'Alpha DB', 'blue'),
  postgresConnectionSummary('conn-k2', 'Bravo DB', 'cyan'),
  postgresConnectionSummary('conn-k3', 'Charlie DB', 'green'),
];
const CONTROL: ControlSnapshot[] = [{ channel: IPC.connectionsList, response: CONNS }];

test('project tree — arrows, Home/End, right-click select, Shift+F10 and focus return', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL });
  const rows = page.locator('[data-testid="tree-row"][data-kind="connection"]');
  await expect(rows).toHaveCount(CONNS.length);
  const row = (i: number) => rows.filter({ hasText: CONNS[i].name });

  await row(1).focus();
  await expect(row(1)).toHaveAttribute('aria-selected', 'true');
  await page.keyboard.press('Shift+F10');
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator('[data-testid="context-menu"]')).toHaveCount(0);

  await row(0).focus();
  await page.keyboard.press('ArrowDown');
  await expect(row(1)).toBeFocused();
  await expect(row(1)).toHaveAttribute('aria-selected', 'true');
  await expect(page.locator('[data-testid="tree-row"][tabindex="0"]')).toHaveCount(1);

  await page.keyboard.press('End');
  await expect(row(2)).toBeFocused();
  await page.keyboard.press('ArrowDown');
  await expect(row(2)).toBeFocused();
  await page.keyboard.press('Home');
  await expect(row(0)).toBeFocused();
  await page.keyboard.press('ArrowUp');
  await expect(row(0)).toBeFocused();

  // Right-click selects the row under the pointer, then the menu opens; Esc returns focus to it.
  await row(2).click({ button: 'right' });
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await expect(row(2)).toHaveAttribute('aria-selected', 'true');
  await page.keyboard.press('Escape');
  await expect(page.locator('[data-testid="context-menu"]')).toHaveCount(0);
  await expect(row(2)).toBeFocused();

  await page.keyboard.press('ArrowUp');
  await expect(row(1)).toBeFocused();
  await page.keyboard.press('Shift+F10');
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator('[data-testid="context-menu"]')).toHaveCount(0);
  await expect(row(1)).toBeFocused();
});

// --- collections tree -------------------------------------------------------------------------

const NOW = '2026-01-01T00:00:00.000Z';
const COLLECTION_TREE = {
  collections: [{ id: 'col-1', name: 'Orders API', sortOrder: 0, createdAt: NOW, updatedAt: NOW }],
  items: [
    {
      id: 'item-folder',
      collectionId: 'col-1',
      parentId: null,
      kind: 'folder',
      name: 'Orders',
      sortOrder: 0,
      method: '',
      url: '',
      createdAt: NOW,
      updatedAt: NOW,
    },
    {
      id: 'item-create',
      collectionId: 'col-1',
      parentId: 'item-folder',
      kind: 'request',
      name: 'Create order',
      sortOrder: 0,
      method: 'POST',
      url: 'https://api.example.com/v2/orders',
      createdAt: NOW,
      updatedAt: NOW,
    },
    {
      id: 'item-health',
      collectionId: 'col-1',
      parentId: null,
      kind: 'request',
      name: 'Health check',
      sortOrder: 1,
      method: 'GET',
      url: 'https://api.example.com/healthz',
      createdAt: NOW,
      updatedAt: NOW,
    },
  ],
};
const HEALTH_REQUEST = {
  method: 'GET',
  url: 'https://api.example.com/healthz',
  headers: [],
  bodyMode: 'code',
  body: '',
  code: '',
  codeLanguage: 'json',
  urlEncoded: [],
  formData: [],
  binaryFile: null,
};

test('collections tree — arrows, expand and collapse, Enter opens, Shift+F10 and focus return', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.collectionsList, response: COLLECTION_TREE },
      { channel: IPC.collectionsGetRequest, response: HEALTH_REQUEST },
    ],
  });
  await modeTab(page, 'api').click();
  const row = (id: string) => page.locator(`[data-testid="collection-row"][data-id="${id}"]`);

  await row('col-1').focus();
  await page.keyboard.press('ArrowRight');
  await expect(row('item-folder')).toBeVisible();
  await expect(row('item-health')).toBeVisible();

  await page.keyboard.press('ArrowRight');
  await expect(row('item-folder')).toBeFocused();
  await expect(row('item-folder')).toHaveAttribute('aria-selected', 'true');
  await page.keyboard.press('ArrowRight');
  await expect(row('item-create')).toBeVisible();
  await page.keyboard.press('ArrowRight');
  await expect(row('item-create')).toBeFocused();
  await page.keyboard.press('ArrowLeft');
  await expect(row('item-folder')).toBeFocused();
  await page.keyboard.press('ArrowLeft');
  await expect(row('item-create')).toHaveCount(0);

  await page.keyboard.press('End');
  await expect(row('item-health')).toBeFocused();
  await page.keyboard.press('Home');
  await expect(row('col-1')).toBeFocused();
  await expect(page.locator('[data-testid="collection-row"][tabindex="0"]')).toHaveCount(1);

  await row('item-health').click({ button: 'right' });
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await expect(row('item-health')).toHaveAttribute('aria-selected', 'true');
  await page.keyboard.press('Escape');
  await expect(row('item-health')).toBeFocused();
  await page.keyboard.press('Shift+F10');
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(row('item-health')).toBeFocused();

  await page.keyboard.press('Enter');
  await expect(page.locator('[data-testid="http-request-view"]')).toBeVisible();
});

// --- request history lists --------------------------------------------------------------------

function httpEntry(id: string, status: number, sentAt: string) {
  return {
    id,
    itemId: null,
    tabId: 't1',
    sentAt,
    method: 'GET',
    url: 'https://api.example.com/orders',
    environment: '',
    status,
    statusText: status === 200 ? 'OK' : 'Not Found',
    elapsedMs: 10,
    bodyBytes: 5,
    storedBytes: 20,
  };
}

test('http history — arrows, Home/End and Enter view the focused entry', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [
      {
        channel: IPC.historyList,
        response: [
          httpEntry('e3', 200, '2024-06-01T10:10:00.000Z'),
          httpEntry('e2', 404, '2024-06-01T10:05:00.000Z'),
          httpEntry('e1', 200, '2024-06-01T10:00:00.000Z'),
        ],
      },
    ],
  });
  await openHttpModeAndNewRequest(page);
  await page.click('[data-testid="http-response-pane-history"]');
  const rows = page.locator('[data-testid="http-history-row"]');
  await expect(rows).toHaveCount(3);

  await rows.nth(0).focus();
  await page.keyboard.press('ArrowDown');
  await expect(rows.nth(1)).toBeFocused();
  await expect(page.locator('[data-testid="http-history-row"][tabindex="0"]')).toHaveCount(1);
  await page.keyboard.press('End');
  await expect(rows.nth(2)).toBeFocused();
  await page.keyboard.press('ArrowDown');
  await expect(rows.nth(2)).toBeFocused();
  await page.keyboard.press('Home');
  await expect(rows.nth(0)).toBeFocused();

  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Enter');
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.historyGet).length)
    .toBeGreaterThan(0);
});

test('grpc history — arrows and Home/End move between calls', async ({ relaunch }) => {
  const entry = (id: string) => ({
    id,
    itemId: null,
    tabId: 'tab-grpc-1',
    calledAt: '2026-01-01T00:00:00.000Z',
    target: 'demo.example.com:443',
    method: 'demo.Echo/SayHello',
    streaming: 'unary',
    environment: '',
    code: 0,
    codeName: 'OK',
    statusMessage: '',
    elapsedMs: 5,
    messageCount: 1,
    messageBytes: 20,
    storedBytes: 20,
  });
  const scope = { itemId: '', tabId: 'tab-grpc-1' };
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.tabsList, response: [grpcTab({})] },
      {
        channel: IPC.grpcHistoryList,
        args: scope,
        response: [entry('c3'), entry('c2'), entry('c1')],
      },
    ],
  });
  const rows = page.locator('[data-testid="grpc-history-row"]');
  await expect(rows).toHaveCount(3);

  await rows.nth(0).focus();
  await page.keyboard.press('ArrowDown');
  await expect(rows.nth(1)).toBeFocused();
  await page.keyboard.press('End');
  await expect(rows.nth(2)).toBeFocused();
  await page.keyboard.press('Home');
  await expect(rows.nth(0)).toBeFocused();
  await expect(page.locator('[data-testid="grpc-history-row"][tabindex="0"]')).toHaveCount(1);
});

// --- redis Browse and KeyValue ----------------------------------------------------------------

const REDIS_USER_NS = `${REDIS_DB0}/namespace:user`;
const REDIS_USER1_NS = `${REDIS_USER_NS}/namespace:1`;
const REDIS_BIGHASH = `${REDIS_USER1_NS}/key:user%3A1%3Abighash`;
const MENU = '[data-testid="context-menu"]';

test('browse — arrows, Home/End, Space selects, Enter opens, Shift+F10 menu and focus return', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: redisControl, stream: redisPorts });
  await openRedisBrowse(page);
  const view = page.locator('[data-testid="browse-view"]');
  const rows = view.locator('[data-testid="browse-row"]');
  await expect(rows).toHaveCount(7);

  await rows.first().focus();
  await page.keyboard.press('ArrowDown');
  await expect(rows.nth(1)).toBeFocused();
  await expect(view.locator('[data-testid="browse-row"][tabindex="0"]')).toHaveCount(1);
  await page.keyboard.press('End');
  await expect(rows.nth(6)).toBeFocused();
  await page.keyboard.press('ArrowDown');
  await expect(rows.nth(6)).toBeFocused();
  await page.keyboard.press('Home');
  await expect(rows.nth(0)).toBeFocused();
  await page.keyboard.press('ArrowUp');
  await expect(rows.nth(0)).toBeFocused();

  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Space');
  await expect(rows.nth(1)).toHaveAttribute('aria-selected', 'true');

  await page.keyboard.press('Shift+F10');
  await expect(page.locator(MENU)).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator(MENU)).toHaveCount(0);
  await expect(rows.nth(1)).toBeFocused();

  const userRow = view.locator(`[data-testid="browse-row"][data-path="${REDIS_USER_NS}"]`);
  await userRow.focus();
  await page.keyboard.press('Enter');
  await expect(view).toHaveAttribute('data-level', REDIS_USER_NS);
});

test('keyvalue — arrows, Home/End, Space selects, Shift+F10 menu and focus return', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: redisControl, stream: redisPorts });
  await openRedisBrowse(page);
  const browse = page.locator('[data-testid="browse-view"]');
  await browse.locator(`[data-testid="browse-row"][data-path="${REDIS_USER_NS}"]`).dblclick();
  await browse.locator(`[data-testid="browse-row"][data-path="${REDIS_USER1_NS}"]`).dblclick();
  await openBrowseLeaf(page, REDIS_BIGHASH);
  const view = page.locator(`[data-testid="keyvalue-view"][data-path="${REDIS_BIGHASH}"]`);
  const rows = view.locator('[data-testid="keyvalue-row"]');
  await expect(rows.first()).toBeVisible({ timeout: 15_000 });

  await rows.first().focus();
  await page.keyboard.press('ArrowDown');
  await expect(rows.nth(1)).toBeFocused();
  await expect(view.locator('[data-testid="keyvalue-row"][tabindex="0"]')).toHaveCount(1);
  await page.keyboard.press('ArrowUp');
  await expect(rows.nth(0)).toBeFocused();
  await page.keyboard.press('ArrowUp');
  await expect(rows.nth(0)).toBeFocused();

  // 100 rows are virtualized: End jumps past the rendered window and renders the last row.
  await page.keyboard.press('End');
  await expect(rows.last()).toBeFocused();
  await expect(rows.last()).toContainText('100');
  await page.keyboard.press('Home');
  await expect(rows.first()).toBeFocused();
  await expect(rows.first()).toContainText('1');

  // Selecting a row publishes it to the cell editor panel.
  await expect(page.locator('[data-testid="cell-editor-panel"]')).toHaveCount(0);
  await page.keyboard.press('Space');
  await expect(page.locator('[data-testid="cell-editor-panel"]')).toBeVisible();
  await page.keyboard.press('Shift+F10');
  await expect(page.locator(MENU)).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator(MENU)).toHaveCount(0);
  await expect(rows.first()).toBeFocused();
});

// --- documents ----------------------------------------------------------------------------------

test('documents — arrows, Home/End, Space selects, Enter expands, Shift+F10 menu and focus return', async ({
  relaunch,
}) => {
  const fixture = widgetsFixture('conn-doc-kb');
  const { window: page } = await relaunch({
    control: [
      {
        channel: IPC.connectionsList,
        response: [mongoConnectionSummary('conn-doc-kb', 'Mongo KB', 'red')],
      },
      ...fixture.control,
    ],
    stream: fixture.port,
  });
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
  await expandRow(page, 'database:kira_test');
  await (await findRow(page, WIDGETS_PATH)).dblclick();
  const view = page.locator('[data-testid="document-view"]');
  const rows = view.locator('[data-testid="document-row"] [role="option"][data-nav-key]');
  await expect(rows.first()).toBeVisible({ timeout: 15_000 });

  await rows.first().focus();
  await page.keyboard.press('ArrowDown');
  await expect(rows.nth(1)).toBeFocused();
  await expect(view.locator('[role="option"][data-nav-key][tabindex="0"]')).toHaveCount(1);
  await expect(rows.nth(1)).toHaveAttribute('aria-selected', 'true');
  await page.keyboard.press('ArrowUp');
  await expect(rows.first()).toBeFocused();
  await page.keyboard.press('ArrowUp');
  await expect(rows.first()).toBeFocused();

  await page.keyboard.press('End');
  await expect(rows.last()).toBeFocused();
  await page.keyboard.press('Home');
  await expect(rows.first()).toBeFocused();

  const toggle = rows.first().locator('[data-testid="document-toggle-expand"]');
  await expect(toggle).toHaveAttribute('aria-label', 'Collapse');
  await page.keyboard.press('Enter');
  await expect(toggle).toHaveAttribute('aria-label', 'Expand');
  await page.keyboard.press('Enter');
  await expect(toggle).toHaveAttribute('aria-label', 'Collapse');

  await page.keyboard.press('Space');
  await expect(rows.first()).toHaveAttribute('aria-selected', 'true');
  await page.keyboard.press('Shift+F10');
  await expect(page.locator(MENU)).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator(MENU)).toHaveCount(0);
  await expect(rows.first()).toBeFocused();
});

// --- stream ---------------------------------------------------------------------------------------

test('stream — arrows, Home/End, Space selects, Shift+F10 menu and focus return', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: sqsControl, stream: sqsPorts });
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
  const rows = view.locator('[data-testid="stream-row"] [role="option"][data-nav-key]');
  await expect(rows).toHaveCount(5, { timeout: 15_000 });

  await rows.first().focus();
  await page.keyboard.press('ArrowDown');
  await expect(rows.nth(1)).toBeFocused();
  await expect(view.locator('[data-testid="stream-row"] [data-nav-key][tabindex="0"]')).toHaveCount(
    1,
  );
  await page.keyboard.press('End');
  await expect(rows.nth(4)).toBeFocused();
  await page.keyboard.press('ArrowDown');
  await expect(rows.nth(4)).toBeFocused();
  await page.keyboard.press('Home');
  await expect(rows.first()).toBeFocused();
  await page.keyboard.press('ArrowUp');
  await expect(rows.first()).toBeFocused();

  await page.keyboard.press('Space');
  await expect(rows.first()).toHaveAttribute('aria-selected', 'true');
  await page.keyboard.press('Shift+F10');
  await expect(page.locator(MENU)).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator(MENU)).toHaveCount(0);
  await expect(rows.first()).toBeFocused();
});

// --- console result grid ----------------------------------------------------------------------

test('console key-value result — arrows, Home/End, Space selects, Shift+F10 menu and focus return', async ({
  relaunch,
}) => {
  const ports = structuredClone(redisPorts);
  const exec = ports.find((p) => p.op === DATA_OP.execute);
  if (!exec) throw new Error('redis fixture has no execute snapshot');
  const page0 = (exec.response as { pages: { fields: string[]; values: string[] }[] }).pages[0];
  page0.fields = ['alpha', 'bravo', 'charlie', 'delta'];
  page0.values = ['1', '2', '3', '4'];
  const { window: page } = await relaunch({ control: redisControl, stream: ports });
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
  await openRowMenu(page, REDIS_DB0);
  await page.click('[data-testid="menu-item-open-console"]');
  const consoleView = page.locator('[data-testid="console-view"]');
  await consoleView.locator('.view-lines').click();
  await page.keyboard.type('DBSIZE');
  await page.click('[data-testid="console-run-statement"]');
  const rows = consoleView.locator('[data-testid="console-result-kv-row"]');
  await expect(rows).toHaveCount(4);

  await rows.first().focus();
  await page.keyboard.press('ArrowDown');
  await expect(rows.nth(1)).toBeFocused();
  await expect(
    consoleView.locator('[data-testid="console-result-kv-row"][tabindex="0"]'),
  ).toHaveCount(1);
  await page.keyboard.press('End');
  await expect(rows.nth(3)).toBeFocused();
  await page.keyboard.press('ArrowDown');
  await expect(rows.nth(3)).toBeFocused();
  await page.keyboard.press('Home');
  await expect(rows.nth(0)).toBeFocused();
  await page.keyboard.press('ArrowUp');
  await expect(rows.nth(0)).toBeFocused();

  await page.keyboard.press('Space');
  await expect(rows.first()).toHaveAttribute('aria-selected', 'true');
  await page.keyboard.press('Shift+F10');
  await expect(page.locator(MENU)).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator(MENU)).toHaveCount(0);
  await expect(rows.first()).toBeFocused();
});

test('console document result — arrows, Home/End, Enter expands, Shift+F10 menu and focus return', async ({
  relaunch,
}) => {
  const fixture = widgetsFixture('conn-doc-kb');
  const statement = 'db.widgets.find()';
  const { window: page } = await relaunch({
    control: [
      {
        channel: IPC.connectionsList,
        response: [mongoConnectionSummary('conn-doc-kb', 'Mongo KB', 'red')],
      },
      ...fixture.control,
    ],
    stream: [
      ...fixture.port,
      {
        op: DATA_OP.execute,
        payload: {
          connectionId: 'conn-doc-kb',
          path: 'database:kira_test',
          statements: [statement],
        },
        response: {
          kind: 'execute',
          pages: [
            {
              kind: 'document',
              ids: WIDGETS_IDS.slice(0, 4),
              bodies: WIDGETS_BODIES.slice(0, 4),
              position: {
                offset: 0,
                pageSize: 4,
                hasMore: false,
                nextToken: null,
                prevToken: null,
                strategy: 'offset',
              },
            },
          ],
        },
      },
    ],
  });
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
  await openRowMenu(page, 'database:kira_test');
  await page.click('[data-testid="menu-item-open-console"]');
  const consoleView = page.locator('[data-testid="console-view"]');
  await consoleView.locator('.view-lines').click();
  await page.keyboard.type(statement);
  await page.click('[data-testid="console-run-statement"]');
  const rows = consoleView.locator(
    '[data-testid="console-result-doc-row"] [role="option"][data-nav-key]',
  );
  await expect(rows).toHaveCount(4);

  await rows.first().focus();
  await page.keyboard.press('ArrowDown');
  await expect(rows.nth(1)).toBeFocused();
  await page.keyboard.press('End');
  await expect(rows.nth(3)).toBeFocused();
  await page.keyboard.press('Home');
  await expect(rows.first()).toBeFocused();

  const toggle = rows.first().locator('[data-testid="document-toggle-expand"]');
  const before = await toggle.getAttribute('aria-label');
  await page.keyboard.press('Enter');
  await expect(toggle).not.toHaveAttribute('aria-label', before ?? '');
  await page.keyboard.press('Space');
  await expect(rows.first()).toHaveAttribute('aria-selected', 'true');
  await page.keyboard.press('Shift+F10');
  await expect(page.locator(MENU)).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator(MENU)).toHaveCount(0);
  await expect(rows.first()).toBeFocused();
});

// --- saved list menu type-ahead -----------------------------------------------------------------

test('saved queries popover — type-ahead jumps by first letter and prefix, Enter applies', async ({
  relaunch,
}) => {
  const id = 'conn-saved-kb';
  const saved = ['Alpha', 'Apple', 'Bravo', 'Beta'].map((name, i) => ({
    kind: 'console' as const,
    id: `saved-${i}`,
    connectionId: id,
    path: ORDER_ITEMS_PATH,
    name,
    pinned: false,
    createdAt: '2026-01-01T00:00:00.000Z',
    usedAt: null,
    body: { text: `SELECT '${name}';` },
  }));
  const fixture = orderItemsFixture(id);
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: [] },
      {
        channel: IPC.connectionsCreate,
        args: connectionCreateArgs('Saved DB', 'green'),
        response: postgresConnectionSummary(id, 'Saved DB', 'green'),
      },
      ...fixture.control,
      {
        channel: IPC.queriesListConsole,
        args: { connectionId: id, path: ORDER_ITEMS_PATH },
        response: saved,
      },
      { channel: IPC.queriesTouch, args: { id: 'saved-3' }, response: undefined },
    ],
    stream: fixture.port,
  });
  await connectAndExpand(page, 'Saved DB', 'green');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  await page.click('[data-testid="console-saved-toggle"]');
  const entries = page.locator('[data-testid="console-saved-entry"]');
  await expect(entries).toHaveCount(4);

  await entries.first().focus();
  await page.keyboard.press('b');
  await expect(entries.nth(2)).toBeFocused();
  await page.keyboard.press('b');
  await expect(entries.nth(3)).toBeFocused();
  await page.keyboard.press('b');
  await expect(entries.nth(2)).toBeFocused();
  // After the buffer resets, a two-letter prefix picks the matching row.
  // Behavior under test: the type-ahead buffer resets after a real-time idle window.
  await page.waitForTimeout(600);
  await page.keyboard.type('be');
  await expect(entries.nth(3)).toBeFocused();
  // Behavior under test: the type-ahead buffer resets after a real-time idle window.
  await page.waitForTimeout(600);
  await page.keyboard.press('a');
  await expect(entries.nth(0)).toBeFocused();

  await page.keyboard.press('Enter');
  await expect
    .poll(() => editorText(page.locator('[data-testid="console-view"]')))
    .toContain("SELECT 'Alpha';");
});
