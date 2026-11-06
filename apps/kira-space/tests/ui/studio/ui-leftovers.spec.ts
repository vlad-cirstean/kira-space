import type { Locator, Page } from '@playwright/test';
import { DATA_OP } from '@shared/protocol/dataOps';
import {
  controlSnapshots as redisControl,
  portSnapshots as redisPorts,
} from '../../ipc/redis/redis.fixture';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, test } from './fixtures';
import { modeTab } from './support/apiMode';
import { collectionRow } from './support/apiRows';
import { primaryChord } from './support/chord';
import { installClipboardSpy, lastClipboardWrite } from './support/clipboard';
import {
  connectAndExpand,
  connectionCreateArgs,
  createConnection,
  openConsoleFromMenu,
} from './support/connect';
import { acceptConfirm, cancelConfirm } from './support/dialogs';
import { gridCell } from './support/grid';
import { IPC } from './support/ipcChannels';
import { mongoConnectionSummary, WIDGETS_PATH, widgetsFixture } from './support/mongoFixture';
import {
  COMPOSITE_PK_PATH,
  compositePkConnectAndOpen,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { openRedisBrowse, REDIS_DB0 } from './support/redisFixture';
import { expandRow, findRow, openBrowseLeaf } from './support/tree';

// P267 Stream A: UI leftover unification. Written, not run in the authoring session.

const MENU = '[data-testid="context-menu"]';
const CONFIRM = '[data-testid="confirm-dialog"]';
const NOW = '2026-01-01T00:00:00.000Z';

// --- dirty guards -----------------------------------------------------------------------------

const GRID_ID = 'conn-ul-grid';
const GRID_FIXTURE = compositePkConnectAndOpen(GRID_ID);
const GRID_CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Leftover DB', 'green'),
    response: postgresConnectionSummary(GRID_ID, 'Leftover DB', 'green'),
  },
  ...GRID_FIXTURE.control,
];

async function openGridWithStagedEdit(page: Page): Promise<void> {
  await connectAndExpand(page, 'Leftover DB', 'green');
  await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await gridCell(page, 0, 'name').dblclick();
  await page.locator('[data-testid="grid-cell-input"]').fill('staged value');
  await page.keyboard.press('Enter');
  await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);
}

test('closing a data tab with a staged edit asks to discard; Cancel keeps the tab and the edit', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: GRID_CONTROL, stream: GRID_FIXTURE.port });
  await openGridWithStagedEdit(page);

  await page.locator('[data-testid="tab"]').first().locator('[data-testid="tab-close"]').click();
  await expect(page.locator(CONFIRM)).toBeVisible();
  await expect(page.locator('[data-testid="confirm-dialog-confirm"]')).toHaveText('Discard');
  await cancelConfirm(page);
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(1);
  await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);

  await page.locator('[data-testid="tab"]').first().locator('[data-testid="tab-close"]').click();
  await expect(page.locator('[data-testid="confirm-dialog-confirm"]')).toHaveText('Discard');
  await acceptConfirm(page);
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(0);
});

test('Close others over a dirty tab asks once', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: GRID_CONTROL, stream: GRID_FIXTURE.port });
  await openGridWithStagedEdit(page);
  await openConsoleFromMenu(page, '');
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(2);

  await page.locator('[data-testid="tab"]').nth(1).click({ button: 'right' });
  await page.click('[data-testid="menu-item-close-others"]');
  await expect(page.locator('[data-testid="confirm-dialog-confirm"]')).toHaveText('Discard');
  await acceptConfirm(page);
  await expect(page.locator(CONFIRM)).toHaveCount(0);
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(1);
});

// --- connection dialog ------------------------------------------------------------------------

async function openNewPostgresDialog(page: Page): Promise<void> {
  await page.click('[data-testid="add-connection"]');
  await expect(page.locator('[data-testid="new-connection-dialog"]')).toBeVisible();
  await page.click('[data-testid="connection-kind-postgres"]');
}

test('new connection dialog with a typed name: outside click keeps it open; Esc and X ask to discard; Cancel closes without asking', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.connectionsList, response: [] }],
  });
  const dialog = page.locator('[data-testid="new-connection-dialog"]');
  await openNewPostgresDialog(page);
  await page.fill('[data-testid="new-connection-name"]', 'Draft DB');

  await page.mouse.click(4, 4);
  await expect(dialog).toBeVisible();
  await expect(page.locator(CONFIRM)).toHaveCount(0);

  await page.keyboard.press('Escape');
  await expect(page.locator(CONFIRM)).toBeVisible();
  await expect(page.locator('[data-testid="confirm-dialog-confirm"]')).toHaveText('Discard');
  await cancelConfirm(page);
  await expect(dialog).toBeVisible();

  await page.click('[data-testid="new-connection-close"]');
  await expect(page.locator(CONFIRM)).toBeVisible();
  await cancelConfirm(page);
  await expect(dialog).toBeVisible();

  await page.click('[data-testid="new-connection-cancel"]');
  await expect(dialog).toHaveCount(0);
  await expect(page.locator(CONFIRM)).toHaveCount(0);
});

test('new connection dialog: Create stays off until the connection has a name', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.connectionsList, response: [] }],
  });
  await openNewPostgresDialog(page);

  const create = page.locator('[data-testid="new-connection-create"]');
  await expect(create).toBeDisabled();
  await page.fill('[data-testid="new-connection-name"]', 'Named DB');
  await expect(create).toBeEnabled();
  await page.fill('[data-testid="new-connection-name"]', '   ');
  await expect(create).toBeDisabled();
});

test('creating a connection: Create is aria-busy while the create call is pending', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: [] },
      {
        channel: IPC.connectionsCreate,
        args: connectionCreateArgs('Busy DB'),
        response: postgresConnectionSummary('conn-ul-busy', 'Busy DB', 'green'),
        hold: true,
      },
    ],
  });
  await openNewPostgresDialog(page);
  await page.fill('[data-testid="new-connection-name"]', 'Busy DB');

  const create = page.locator('[data-testid="new-connection-create"]');
  await create.click();
  await expect(create).toHaveAttribute('aria-busy', 'true');
  control.release(IPC.connectionsCreate);
  await expect(page.locator('[data-testid="new-connection-dialog"]')).toHaveCount(0);
});

// --- shortcuts --------------------------------------------------------------------------------

const COLLECTION_TREE = {
  collections: [{ id: 'col-1', name: 'Orders API', sortOrder: 0, createdAt: NOW, updatedAt: NOW }],
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
      createdAt: NOW,
      updatedAt: NOW,
    },
  ],
};
const HEALTH_REQUEST = {
  method: 'GET',
  url: 'https://api.example.com/healthz',
  headers: [],
  bodyMode: 'none',
  body: '',
  code: '',
  codeLanguage: 'json',
  urlEncoded: [],
  formData: [],
  binaryFile: null,
};

test('Ctrl/Cmd+S in an HTTP request tab calls the save mock', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.collectionsList, response: COLLECTION_TREE },
      { channel: IPC.collectionsGetRequest, response: HEALTH_REQUEST },
      {
        channel: IPC.collectionsSaveRequest,
        response: COLLECTION_TREE.items[0],
      },
    ],
  });
  await modeTab(page, 'api').click();
  await collectionRow(page, 'col-1').locator('[data-testid="tree-twisty"]').click();
  await collectionRow(page, 'item-health').dblclick();
  await expect(page.locator('[data-testid="http-request-view"]')).toBeVisible();

  await page.fill('[data-testid="http-url"]', 'https://api.example.com/healthz?x=1');
  await expect(page.locator('[data-testid="http-dirty"]')).toBeVisible();
  await page.locator('[data-testid="http-url"]').press(await primaryChord(page, 'S'));
  await expect(page.locator('[data-testid="http-dirty"]')).toHaveCount(0);
  expect(control.log().filter((e) => e.channel === IPC.collectionsSaveRequest)).toHaveLength(1);
});

const MONGO_ID = 'conn-ul-doc';
const MONGO_FIXTURE = widgetsFixture(MONGO_ID);

test('Ctrl/Cmd+S in a document edit saves it', async ({ relaunch }) => {
  const { window: page, stream } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: [] },
      {
        channel: IPC.connectionsCreate,
        response: mongoConnectionSummary(MONGO_ID, 'Mongo UL', 'red'),
      },
      ...MONGO_FIXTURE.control,
    ],
    stream: MONGO_FIXTURE.port,
  });
  await createConnection(page, 'mongodb', 'Mongo UL');
  await expandRow(page, '');
  await expandRow(page, 'database:kira_test');
  await (await findRow(page, WIDGETS_PATH)).dblclick();
  await expect(page.locator('[data-testid="document-view"]')).toBeVisible();

  await page.locator('[data-testid="document-edit"]').first().click();
  await page.locator('.view-lines').first().click();
  await page.keyboard.type(' ');
  await page.keyboard.press(await primaryChord(page, 'S'));
  await expect
    .poll(async () => (await stream.ops()).some((op) => op.op === DATA_OP.mutate))
    .toBe(true);
});

// --- redis browse / key-value -----------------------------------------------------------------

const REDIS_USER_NS = `${REDIS_DB0}/namespace:user`;
const REDIS_USER1_NS = `${REDIS_USER_NS}/namespace:1`;
const REDIS_BIGHASH = `${REDIS_USER1_NS}/key:user%3A1%3Abighash`;

test('right-click on a KeyValue row selects it before the menu shows', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: redisControl, stream: redisPorts });
  await openRedisBrowse(page);
  const browse = page.locator('[data-testid="browse-view"]');
  await browse.locator(`[data-testid="browse-row"][data-path="${REDIS_USER_NS}"]`).dblclick();
  await browse.locator(`[data-testid="browse-row"][data-path="${REDIS_USER1_NS}"]`).dblclick();
  await openBrowseLeaf(page, REDIS_BIGHASH);
  const view = page.locator(`[data-testid="keyvalue-view"][data-path="${REDIS_BIGHASH}"]`);
  const rows = view.locator('[data-testid="keyvalue-row"]');
  await expect(rows.nth(2)).toBeVisible({ timeout: 15_000 });

  // Records the row's aria-selected at the instant the menu node is inserted.
  await page.evaluate(
    ({ menu }) => {
      const w = window as unknown as { __selectedAtMenu: string | null };
      w.__selectedAtMenu = null;
      new MutationObserver(() => {
        if (w.__selectedAtMenu !== null || !document.querySelector(menu)) return;
        w.__selectedAtMenu =
          document
            .querySelectorAll('[data-testid="keyvalue-row"]')[2]
            ?.getAttribute('aria-selected') ?? 'missing';
      }).observe(document.body, { childList: true, subtree: true });
    },
    { menu: MENU },
  );
  await rows.nth(2).click({ button: 'right' });
  await expect(page.locator(MENU)).toBeVisible();
  await expect(rows.nth(2)).toHaveAttribute('aria-selected', 'true');
  expect(
    await page.evaluate(() => (window as unknown as { __selectedAtMenu: string }).__selectedAtMenu),
  ).toBe('true');
});

test('Browse: the tree.copyName chord copies the name', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: redisControl, stream: redisPorts });
  await openRedisBrowse(page);
  const browse = page.locator('[data-testid="browse-view"]');
  await browse.locator(`[data-testid="browse-row"][data-path="${REDIS_USER_NS}"]`).dblclick();
  await browse.locator(`[data-testid="browse-row"][data-path="${REDIS_USER1_NS}"]`).dblclick();
  const keyRow = browse.locator(`[data-testid="browse-row"][data-path="${REDIS_BIGHASH}"]`);
  await expect(keyRow).toBeVisible();

  await installClipboardSpy(page);
  await keyRow.focus();
  await page.keyboard.press(await primaryChord(page, 'C'));
  await expect.poll(() => lastClipboardWrite(page)).toBe('user:1:bighash');
});

// --- InlineState, find bar --------------------------------------------------------------------

function httpRequestTab(state: Record<string, unknown>): Record<string, unknown> {
  return {
    id: 'tab-ul-1',
    connectionId: null,
    path: 'request',
    kind: 'http-request',
    order: 0,
    active: true,
    state: {
      method: 'GET',
      url: 'https://api.example.com/users',
      headers: [],
      bodyMode: 'none',
      body: '',
      code: '',
      codeLanguage: 'json',
      urlEncoded: [],
      formData: [],
      binaryFile: null,
      itemId: null,
      name: '',
      requestPane: 'body',
      responsePane: 'body',
      responseView: 'pretty',
      requestPaneHeight: 0,
      ...state,
    },
  };
}

function httpResponseOf(body: string): Record<string, unknown> {
  return {
    status: 200,
    statusText: 'OK',
    proto: 'HTTP/1.1',
    headers: [{ name: 'Content-Type', value: 'application/json' }],
    body,
    bodyEncoding: 'utf8',
    bodyBytes: body.length,
    bodyTruncated: false,
    elapsedMs: 10,
    finalUrl: 'https://api.example.com/users',
    redirects: [],
  };
}

test('pane InlineState: response formatting is hidden at 100 ms and visible within 1 s', async ({
  relaunch,
}) => {
  // Over INLINE_CHARS (65 536), so the format pass runs in the worker and stays pending.
  const big = JSON.stringify({
    rows: Array.from({ length: 200_000 }, (_, i) => ({ id: i, v: 'x' })),
  });
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.tabsList, response: [httpRequestTab({})] },
      { channel: IPC.httpSend, response: httpResponseOf(big) },
    ],
  });
  await page.click('[data-testid="http-send"]');
  const pending = page.locator('[data-testid="http-response-formatting"]');
  // Behavior under test: the formatting indicator shows only after a delay.
  await page.waitForTimeout(100);
  await expect(pending).toHaveCount(0);
  await expect(pending).toBeVisible({ timeout: 1_000 });
});

test('response find bar: count text, Enter and Shift+Enter move the current match', async ({
  relaunch,
}) => {
  const body = '{"id":1,"name":"Ada","note":"Ada Lovelace"}';
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.tabsList, response: [httpRequestTab({})] },
      { channel: IPC.httpSend, response: httpResponseOf(body) },
    ],
  });
  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');
  await page.click('[data-testid="http-find-toggle"]');
  await page.fill('[data-testid="http-find-input"]', 'Ada');
  const count = page.locator('[data-testid="http-find-count"]');
  await expect(count).toHaveText('Match 1 of 2');

  await page.locator('[data-testid="http-find-input"]').press('Enter');
  await expect(count).toHaveText('Match 2 of 2');
  await expect(page.locator('.kira-ed-find-match-current')).toHaveCount(1);
  await page.locator('[data-testid="http-find-input"]').press('Shift+Enter');
  await expect(count).toHaveText('Match 1 of 2');
  await expect(page.locator('.kira-ed-find-match-current')).toHaveCount(1);
});

// --- trees ------------------------------------------------------------------------------------

test('project tree search renders matches as highlighted marks', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [
      {
        channel: IPC.connectionsList,
        response: [
          postgresConnectionSummary('conn-ul-1', 'Alpha DB', 'blue'),
          postgresConnectionSummary('conn-ul-2', 'Bravo DB', 'cyan'),
        ],
      },
    ],
  });
  await page.click('[data-testid="toggle-search"]');
  await page.fill('[data-testid="tree-search"]', 'Alpha');
  const mark = page.locator('[data-slot="highlighted-text"] mark').first();
  await expect(mark).toHaveText('Alpha');
  await expect(mark).toHaveClass(/bg-search-match/);
});

test('collections tree expansion survives a reload', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.collectionsList, response: COLLECTION_TREE }],
  });
  await modeTab(page, 'api').click();
  await collectionRow(page, 'col-1').locator('[data-testid="tree-twisty"]').click();
  await expect(collectionRow(page, 'item-health')).toBeVisible();

  await page.reload();
  await modeTab(page, 'api').click();
  await expect(collectionRow(page, 'item-health')).toBeVisible();
});

// --- layering, cursor -------------------------------------------------------------------------

test('a tooltip layer sits above TextPromptDialog', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [...GRID_CONTROL, { channel: IPC.queriesList, response: [] }],
    stream: GRID_FIXTURE.port,
  });
  await connectAndExpand(page, 'Leftover DB', 'green');
  await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await page.click('[data-testid="filter-history-button"]');
  await page.click('[data-testid="save-current-filter"]');
  const prompt = page.locator('[data-testid="text-prompt"]');
  await expect(prompt).toBeVisible();

  // Tooltip content cannot be hovered behind the scrim; probe the layer token it renders with.
  const [promptZ, tooltipZ] = await page.evaluate(() => {
    const scrim = document.querySelector('[data-testid="text-prompt"]') as HTMLElement;
    const probe = document.createElement('div');
    probe.style.cssText = 'position:fixed;z-index:var(--kira-z-tooltip)';
    document.body.appendChild(probe);
    const z = getComputedStyle(probe).zIndex;
    probe.remove();
    return [getComputedStyle(scrim).zIndex, z];
  });
  expect(Number(tooltipZ)).toBeGreaterThan(Number(promptZ));
});

test('tree rows and toolbar buttons use the default cursor; a link button uses pointer', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [
      {
        channel: IPC.connectionsList,
        response: [postgresConnectionSummary('conn-ul-1', 'Alpha DB', 'blue')],
      },
      { channel: IPC.tabsList, response: [httpRequestTab({ requestPane: 'settings' })] },
    ],
  });
  const cursor = (loc: Locator) => loc.evaluate((el) => getComputedStyle(el).cursor);
  await modeTab(page, 'studio').click();
  await expect.poll(() => cursor(page.locator('[data-testid="tree-row"]').first())).toBe('default');
  await expect.poll(() => cursor(page.locator('[data-testid="add-connection"]'))).toBe('default');

  await modeTab(page, 'api').click();
  await page.click('[data-testid="http-request-pane-settings"]');
  await expect
    .poll(() => cursor(page.locator('[data-testid="http-settings-edit-global"]')))
    .toBe('pointer');
});

// --- filter history ---------------------------------------------------------------------------

test('filter history: pin toggles the row without reopening the menu', async ({ relaunch }) => {
  const SAVED = {
    id: 'q-1',
    connectionId: GRID_ID,
    name: 'Small ids',
    kind: 'filter',
    pinned: false,
    body: { where: 'id <= 5', orderBy: '' },
    createdAt: NOW,
    updatedAt: NOW,
  };
  const { window: page, control } = await relaunch({
    control: [
      ...GRID_CONTROL,
      { channel: IPC.queriesList, response: [SAVED] },
      { channel: IPC.queriesUpdate, response: { ...SAVED, pinned: true } },
    ],
    stream: GRID_FIXTURE.port,
  });
  await connectAndExpand(page, 'Leftover DB', 'green');
  await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();

  await page.click('[data-testid="filter-history-button"]');
  const panel = page.locator('[data-testid="filter-history"]');
  await expect(panel).toBeVisible();
  const pin = panel.locator('[data-testid="saved-entry"] [data-testid="saved-entry-pin"]').first();
  await expect(pin).toHaveClass(/text-subtle/);

  await pin.click();
  // The mocked list stays unpinned after the update, so the toggle shows as the call it makes.
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.queriesUpdate))
    .toHaveLength(1);
  expect(control.log().find((e) => e.channel === IPC.queriesUpdate)?.args).toEqual({
    id: 'q-1',
    name: null,
    pinned: true,
  });
  await expect(panel).toBeVisible();
});
