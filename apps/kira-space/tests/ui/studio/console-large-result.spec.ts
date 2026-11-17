// P662: a capped, very large result (10,000 documents, or 800 rows of 16 KiB cells plus a page of
// 64 KiB truncated cells) must never freeze the renderer: every frame gap stays under the budget on
// arrival, on scroll, and when the Studio module is left and re-entered. The backend half is
// dbflow/console_large_test.go; sizes mirror the cap it asserts.
import type { Locator, Page } from '@playwright/test';
import { DATA_OP } from '@shared/protocol/dataOps';
import { CELL_DISPLAY_CHARS } from '../../../frontend/src/studio/views/shared/page/display';
import {
  controlSnapshots as kafkaControl,
  portSnapshots as kafkaPorts,
} from '../../ipc/kafka/kafka.fixture';
import { controlSnapshots as redisControl } from '../../ipc/redis/redis.fixture';
import type { ControlSnapshot, LogicalPage, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import {
  expectResponsive,
  installFreezeProbe,
  measureClickLatency,
  resetFreezeProbe,
} from '../support/freezeProbe';
import { IPC } from '../support/ipcChannels';
import { modeTab } from './support/apiMode';
import {
  connectAndExpand,
  connectionCreateArgs,
  createConnection,
  openConsoleFromMenu,
} from './support/connect';
import { mongoCreateArgs } from './support/connectionArgs';
import { executeSnap } from './support/consolePages';
import { typeInto } from './support/editor';
import { editorText } from './support/editorText';
import { largeDocumentPage, largeTabularPage, type StudioPortSnapshot } from './support/mockStream';
import {
  DB_PATH as MONGO_DB_PATH,
  connectAndExpandControl as mongoConnectAndExpandControl,
  mongoConnectionSummary,
  WIDGETS_PATH,
} from './support/mongoFixture';
import {
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { openRedisBrowse, REDIS_DB0 } from './support/redisFixture';
import { connectionRow, expandRow, findRow, openBrowseLeaf, openRowMenu } from './support/tree';

const BUDGET_MS = 250;
const DOC_ROWS = 10_000;
const DOC_BYTES = 2048;
const DELAY_MS = 1500;
const CELL_BYTES = 16 * 1024;

const docPage = () => largeDocumentPage(DOC_ROWS, DOC_BYTES);

async function connectMongoAndExpand(page: Page, name: string): Promise<void> {
  await createConnection(page, 'mongodb', name);
  await expect(connectionRow(page)).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
  await expandRow(page, MONGO_DB_PATH);
}

/** `relaunch` navigates before a spec can add init scripts, so reload once with the probe in. */
async function startProbe(page: Page): Promise<void> {
  await installFreezeProbe(page);
  await page.reload();
  await modeTab(page, 'studio').click();
}

/** Leave Studio and come back: the console remounts onto its cached result page. */
async function roundTripModules(page: Page, label: string, rows: string): Promise<void> {
  await modeTab(page, 'api').click();
  await expect(page.locator('[data-testid="api-start"]')).toBeVisible();
  await resetFreezeProbe(page);
  await modeTab(page, 'studio').click();
  await expect(page.locator(rows).first()).toBeVisible({ timeout: 30_000 });
  await expectResponsive(page, `${label} round trip`, BUDGET_MS);
}

// `away`: stay on the other tab until the reply has landed, so only transport and the page store
// run (H2). Otherwise return to the console and wait for the rows (H1).
for (const away of [false, true]) {
  test(`document console result stays responsive ${away ? 'while the page lands unmounted' : 'on arrival and remount'}`, async ({
    relaunch,
  }) => {
    const CONNECTION_ID = 'conn-large-doc';
    const STATEMENT = 'db.widgets.find()';
    const CONTROL: ControlSnapshot[] = [
      { channel: IPC.connectionsList, response: [] },
      {
        channel: IPC.connectionsCreate,
        args: mongoCreateArgs('Large Doc'),
        response: mongoConnectionSummary(CONNECTION_ID, 'Large Doc', 'green'),
      },
      ...mongoConnectAndExpandControl(CONNECTION_ID),
    ];
    const PORT: PortSnapshot[] = [
      {
        op: DATA_OP.execute,
        payload: { connectionId: CONNECTION_ID, path: WIDGETS_PATH, statements: [STATEMENT] },
        response: { kind: 'execute', pages: [docPage()] },
        delayMs: DELAY_MS,
      },
    ];
    const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
    await startProbe(page);
    await connectMongoAndExpand(page, 'Large Doc');
    await openConsoleFromMenu(page, WIDGETS_PATH);
    const first = page.locator('[data-testid="tab"][data-active="true"]');
    const firstId = (await first.getAttribute('data-tab-id')) as string;
    await openConsoleFromMenu(page, WIDGETS_PATH);
    const second = page.locator('[data-testid="tab"][data-active="true"]');
    const secondId = (await second.getAttribute('data-tab-id')) as string;
    await page.locator(`[data-testid="tab"][data-tab-id="${firstId}"]`).click();

    const view = page.locator('[data-testid="console-view"]');
    await typeInto(view, page, STATEMENT);
    await resetFreezeProbe(page);
    await page.click('[data-testid="console-run-statement"]');
    const ms = await measureClickLatency(
      page,
      page.locator(`[data-testid="tab"][data-tab-id="${secondId}"]`),
      async () =>
        (await page
          .locator(`[data-testid="tab"][data-tab-id="${secondId}"]`)
          .getAttribute('data-active')) === 'true',
    );
    console.log(`tab click latency during run: ${ms} ms`);
    if (away) {
      await page.waitForTimeout(DELAY_MS + 1500);
      await expectResponsive(page, 'documents transport, view unmounted', BUDGET_MS);
      return;
    }
    await expectResponsive(page, 'documents run delay', BUDGET_MS);
    await page.locator(`[data-testid="tab"][data-tab-id="${firstId}"]`).click();

    await resetFreezeProbe(page);
    const rows = '[data-testid="console-result-doc-row"]';
    await expect(page.locator(rows).first()).toBeVisible({ timeout: 60_000 });
    await expectResponsive(page, 'documents arrival', BUDGET_MS);

    await roundTripModules(page, 'documents', rows);
  });
}

// `away` mirrors the document pair above: only transport and the page store run (H2).
for (const away of [false, true]) {
  test(`tabular console result stays responsive ${away ? 'while the pages land unmounted' : 'on arrival, scroll and remount'}`, async ({
    relaunch,
  }) => {
    const CONNECTION_ID = 'conn-large-tab';
    const STATEMENT = 'SELECT big';
    const FIXTURE = orderItemsFixture(CONNECTION_ID);
    const CONTROL: ControlSnapshot[] = [
      { channel: IPC.connectionsList, response: [] },
      {
        channel: IPC.connectionsCreate,
        args: connectionCreateArgs('Large Tab', 'green'),
        response: postgresConnectionSummary(CONNECTION_ID, 'Large Tab', 'green'),
      },
      ...FIXTURE.control,
    ];
    const pages: LogicalPage[] = [
      largeTabularPage(250, 4, CELL_BYTES),
      largeTabularPage(50, 1, 64 * 1024, true),
    ];
    const PORT: PortSnapshot[] = [
      ...FIXTURE.port,
      { ...executeSnap(CONNECTION_ID, [STATEMENT], pages), delayMs: DELAY_MS },
    ];
    const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
    await startProbe(page);
    await connectAndExpand(page, 'Large Tab', 'green');
    await openConsoleFromMenu(page, ORDER_ITEMS_PATH);

    let otherTab = '';
    if (away) {
      const firstTab = page.locator('[data-testid="tab"][data-active="true"]');
      const firstId = (await firstTab.getAttribute('data-tab-id')) as string;
      await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
      otherTab = firstId;
      // Run in the second tab, then return to the first so the result view is unmounted.
    }
    const view = page.locator('[data-testid="console-view"]');
    await typeInto(view, page, STATEMENT);
    await resetFreezeProbe(page);
    await page.click('[data-testid="console-run-statement"]');
    if (away) {
      await page.locator(`[data-testid="tab"][data-tab-id="${otherTab}"]`).click();
      await page.waitForTimeout(DELAY_MS + 2500);
      await expectResponsive(page, 'tabular transport, view unmounted', BUDGET_MS);
      return;
    }
    const cells = '[data-testid="console-result-cell"]';
    await expect(page.locator(cells).first()).toBeVisible({ timeout: 60_000 });
    await expectResponsive(page, 'tabular arrival', BUDGET_MS);

    await resetFreezeProbe(page);
    const grid = view.locator('[data-testid="console-result-grid"]');
    await grid.hover();
    for (let i = 0; i < 8; i++) await page.mouse.wheel(0, 4000);
    await page.waitForTimeout(500);
    await expectResponsive(page, 'tabular scroll', BUDGET_MS);

    // The DOM holds a bounded prefix; the cell editor still shows the whole value.
    const shown = await page
      .locator(cells)
      .first()
      .evaluate((el) => el.textContent?.length ?? 0);
    expect(shown).toBeLessThanOrEqual(CELL_DISPLAY_CHARS + 1);
    await page.locator(cells).first().click();
    const full = await editorText(page.locator('[data-testid="cell-editor-panel"]'));
    expect(full.length).toBe(CELL_BYTES);

    await roundTripModules(page, 'tabular', cells);
  });
}

async function scrollAndExpectResponsive(page: Page, over: Locator, label: string): Promise<void> {
  await resetFreezeProbe(page);
  await over.hover();
  for (let i = 0; i < 8; i++) await page.mouse.wheel(0, 4000);
  await page.waitForTimeout(500);
  await expectResponsive(page, `${label} scroll`, BUDGET_MS);
}

test('data grid tab stays responsive on a page of huge cells', async ({ relaunch }) => {
  const CONNECTION_ID = 'conn-large-grid';
  const FIXTURE = orderItemsFixture(CONNECTION_ID);
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: connectionCreateArgs('Large Grid', 'green'),
      response: postgresConnectionSummary(CONNECTION_ID, 'Large Grid', 'green'),
    },
    ...FIXTURE.control,
  ];
  const PORT: PortSnapshot[] = FIXTURE.port.map((snap) =>
    snap.op === DATA_OP.read && (snap.payload as { path?: string }).path === ORDER_ITEMS_PATH
      ? {
          ...snap,
          delayMs: DELAY_MS,
          response: {
            kind: 'read',
            page: largeTabularPage(150, 4, CELL_BYTES),
            source: 'server',
          },
        }
      : snap,
  );
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await startProbe(page);
  await connectAndExpand(page, 'Large Grid', 'green');
  await (await findRow(page, ORDER_ITEMS_PATH)).dblclick();
  // Reset once the tab shell is up, so the window holds the page's arrival, not the tab opening.
  await page.waitForTimeout(DELAY_MS / 2);
  await resetFreezeProbe(page);
  const grid = page.locator('[data-testid="data-grid"]');
  await expect(grid).toBeVisible({ timeout: 60_000 });
  await expect(grid.locator('.slick-cell').first()).toBeVisible({ timeout: 60_000 });
  await expectResponsive(page, 'data grid load', BUDGET_MS);
  await scrollAndExpectResponsive(page, grid, 'data grid');
});

test('key/value tab stays responsive on a page of huge values', async ({ relaunch }) => {
  const KEY_PATH = `${REDIS_DB0}/namespace:session/key:session%3Aabc`;
  const NS = `${REDIS_DB0}/namespace:session`;
  const fields = Array.from({ length: 150 }, (_, i) => `field-${i}`);
  const values = fields.map((_, i) =>
    `v${i}-`.repeat(Math.ceil(CELL_BYTES / 4)).slice(0, CELL_BYTES),
  );
  const read: StudioPortSnapshot = {
    op: DATA_OP.read,
    payload: {
      connectionId: 'test-redis',
      path: KEY_PATH,
      projection: null,
      filter: null,
      sort: null,
      pageSize: 100,
      cursor: { mode: 'offset', offset: 0 },
    },
    delayMs: DELAY_MS,
    response: {
      kind: 'read',
      page: {
        kind: 'keyvalue',
        redisType: 'hash',
        ttlMs: null,
        memoryBytes: null,
        fields,
        values,
        position: {
          offset: 0,
          pageSize: fields.length,
          hasMore: false,
          nextToken: null,
          prevToken: null,
          strategy: 'offset',
        },
      },
      source: 'server',
    },
  };
  const { window: page } = await relaunch({ control: redisControl, stream: [read, read] });
  await startProbe(page);
  await openRedisBrowse(page);
  const browse = page.locator('[data-testid="browse-view"]');
  await browse.locator(`[data-testid="browse-row"][data-path="${NS}"]`).dblclick();
  await openBrowseLeaf(page, KEY_PATH);
  await page.waitForTimeout(DELAY_MS / 2);
  await resetFreezeProbe(page);
  const view = page.locator(`[data-testid="keyvalue-view"][data-path="${KEY_PATH}"]`);
  await expect(view.locator('[data-testid="keyvalue-row"]').first()).toBeVisible({
    timeout: 60_000,
  });
  await expectResponsive(page, 'key/value load', BUDGET_MS);
  await scrollAndExpectResponsive(page, view, 'key/value');
});

test('stream tab stays responsive on a page of huge bodies', async ({ relaunch }) => {
  const TOPIC_PATH = 'topic:orders';
  const rows = 150;
  const ordersRead = kafkaPorts.find(
    (s) => s.op === DATA_OP.read && (s.payload as { path: string }).path === TOPIC_PATH,
  ) as PortSnapshot;
  const big: PortSnapshot = {
    ...ordersRead,
    response: {
      kind: 'read',
      page: {
        kind: 'stream',
        keys: Array.from({ length: rows }, (_, i) => `k${i}`),
        headers: Array.from({ length: rows }, () => '[]'),
        attrs: Array.from({ length: rows }, () => '{}'),
        timestamps: Array.from({ length: rows }, () => '2026-01-01T00:00:00.000Z'),
        bodies: Array.from({ length: rows }, (_, i) =>
          `b${i}-`.repeat(Math.ceil(CELL_BYTES / 4)).slice(0, CELL_BYTES),
        ),
        position: {
          offset: null,
          pageSize: 100,
          hasMore: false,
          nextToken: null,
          prevToken: null,
          strategy: 'offsetWindow',
        },
        visibilityTimeoutSeconds: null,
      },
      source: 'server',
    },
  } as PortSnapshot;
  const { window: page } = await relaunch({
    control: kafkaControl as ControlSnapshot[],
    stream: [big, ...kafkaPorts.filter((s) => s !== ordersRead)],
  });
  await startProbe(page);
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
  await resetFreezeProbe(page);
  await (await findRow(page, TOPIC_PATH)).dblclick();
  const view = page.locator(`[data-testid="stream-view"][data-path="${TOPIC_PATH}"]`);
  await expect(view.locator('[data-testid="stream-row"]').first()).toBeVisible({ timeout: 60_000 });
  await expectResponsive(page, 'stream load', BUDGET_MS);
  await scrollAndExpectResponsive(page, view, 'stream');
});

test('a held console run cancels on Stop without a freeze', async ({ relaunch }) => {
  const CONNECTION_ID = 'conn-large-cancel';
  const STATEMENT = 'SELECT pg_sleep(60)';
  const FIXTURE = orderItemsFixture(CONNECTION_ID);
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: connectionCreateArgs('Large Cancel', 'green'),
      response: postgresConnectionSummary(CONNECTION_ID, 'Large Cancel', 'green'),
    },
    ...FIXTURE.control,
  ];
  const PORT: StudioPortSnapshot[] = [
    ...FIXTURE.port,
    {
      op: DATA_OP.execute,
      payload: { connectionId: CONNECTION_ID, path: ORDER_ITEMS_PATH, statements: [STATEMENT] },
      error: { code: 'E_CANCELLED', message: 'operation was cancelled' },
      untilCancel: true,
    },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL, stream: PORT });
  await startProbe(page);
  await connectAndExpand(page, 'Large Cancel', 'green');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await typeInto(view, page, STATEMENT);
  await page.click('[data-testid="console-run-statement"]');
  await expect(page.locator('[data-testid="console-stop"]')).toBeEnabled();
  await resetFreezeProbe(page);
  await page.waitForTimeout(1000);
  await page.click('[data-testid="console-stop"]');
  await expect.poll(() => control.log().filter((e) => e.channel === IPC.opsCancel).length).toBe(1);
  // No result exists to carry a status label: the run ending is the Stop button going idle.
  await expect(page.locator('[data-testid="console-stop"]')).toBeDisabled();
  await expectResponsive(page, 'cancel', BUDGET_MS);
});
