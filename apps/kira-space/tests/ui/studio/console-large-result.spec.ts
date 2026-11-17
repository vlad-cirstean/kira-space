// P662: a capped, very large result (10,000 documents, or 800 rows of 16 KiB cells plus a page of
// 64 KiB truncated cells) must never freeze the renderer: every frame gap stays under the budget on
// arrival, on scroll, and when the Studio module is left and re-entered. The backend half is
// dbflow/console_large_test.go; sizes mirror the cap it asserts.
import type { Page } from '@playwright/test';
import { DATA_OP } from '@shared/protocol/dataOps';
import { CELL_DISPLAY_CHARS } from '../../../frontend/src/studio/views/shared/page/display';
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
import { largeDocumentPage, largeTabularPage } from './support/mockStream';
import {
  DB_PATH as MONGO_DB_PATH,
  connectAndExpandControl as mongoConnectAndExpandControl,
  mongoConnectionSummary,
  WIDGETS_PATH,
  widgetsFixture,
} from './support/mongoFixture';
import {
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';

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
      largeTabularPage(500, 4, CELL_BYTES),
      largeTabularPage(100, 1, 64 * 1024, true),
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
    for (let i = 0; i < 20; i++) await page.mouse.wheel(0, 4000);
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

test('documents tab stays responsive on a 10,000-document page', async ({ relaunch }) => {
  const CONNECTION_ID = 'conn-large-doctab';
  const FIXTURE = widgetsFixture(CONNECTION_ID);
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: mongoCreateArgs('Large DocTab'),
      response: mongoConnectionSummary(CONNECTION_ID, 'Large DocTab', 'green'),
    },
    ...FIXTURE.control,
  ];
  const [unfiltered, ...rest] = FIXTURE.port;
  const PORT: PortSnapshot[] = [
    {
      ...unfiltered,
      response: { kind: 'read', page: docPage(), source: 'server' },
    } as PortSnapshot,
    ...rest,
  ];
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await startProbe(page);
  await connectMongoAndExpand(page, 'Large DocTab');
  await resetFreezeProbe(page);
  await (await findRow(page, WIDGETS_PATH)).dblclick();
  await expect(page.locator('[data-testid="document-row"]').first()).toBeVisible({
    timeout: 60_000,
  });
  await expectResponsive(page, 'documents tab load', BUDGET_MS);
});
