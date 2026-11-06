// P433: switching away from a tab and back keeps its state and re-reads nothing. The backend never
// changes behaviour here, so the scenarios assert what crosses the mock IPC boundary and what the
// user sees: kept key-value page, kept console cursor, kept Discover sidebar search and data view.
import type { Page } from '@playwright/test';
import type { KibanaDiscoverRequest } from '@shared/domain/kibana';
import {
  browserTimeZone,
  resolveTime,
} from '../../../frontend/src/studio/views/console/kibana/time';
import {
  controlSnapshots as redisControl,
  portSnapshots as redisPorts,
} from '../../ipc/redis/redis.fixture';
import { expect, test } from './fixtures';
import { grpcTab, modeTab } from './support/apiMode';
import { openConsoleFromMenu } from './support/connect';
import { connectPostgres, ITEMS_CONTROL, ITEMS_PORT } from './support/copyScenarios';
import { typeInto } from './support/editor';
import { editorText } from './support/editorText';
import { IPC } from './support/ipcChannels';
import { executeHits, kibanaControl, logsNode, logsView } from './support/kibanaFixture';
import { ORDER_ITEMS_PATH } from './support/postgresFixture';
import { connectionRow, expandRow, findRow, openBrowseLeaf, openRowMenu } from './support/tree';

const NOW = new Date('2026-01-15T12:00:00.000Z');

async function connect(page: Page): Promise<void> {
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
}

test('a key-value tab re-reads nothing on a tab switch', async ({ relaunch }) => {
  const { window: page, stream } = await relaunch({ control: redisControl, stream: redisPorts });
  await connect(page);
  const db0 = 'database:db0';
  const ns = `${db0}/namespace:user`;
  const ns1 = `${ns}/namespace:1`;
  const key = `${ns1}/key:user%3A1%3Abighash`;

  await (await findRow(page, db0)).dblclick();
  const browse = page.locator('[data-testid="browse-view"]');
  await expect(browse).toBeVisible();
  await browse.locator(`[data-testid="browse-row"][data-path="${ns}"]`).dblclick();
  await browse.locator(`[data-testid="browse-row"][data-path="${ns1}"]`).dblclick();
  await openBrowseLeaf(page, key);
  const rows = page.locator(
    `[data-testid="keyvalue-view"][data-path="${key}"] [data-testid="keyvalue-row"]`,
  );
  await expect(rows.first()).toBeVisible({ timeout: 15_000 });
  const shown = await rows.count();

  const tabs = page.locator('[data-testid="tab"]');
  await tabs.first().click();
  await expect(browse).toBeVisible();
  const before = (await stream.ops()).length;

  await tabs.nth(1).click();
  await expect(rows.first()).toBeVisible();
  // Absence of a stream op: no event to await.
  await page.waitForTimeout(500);
  expect(await rows.count()).toBe(shown);
  expect((await stream.ops()).length).toBe(before);
});

test('a console tab keeps its cursor across a tab switch', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: ITEMS_CONTROL, stream: ITEMS_PORT });
  await connectPostgres(page);

  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await expect(view).toBeVisible();
  await typeInto(view, page, 'SELECT 1;\nSELECT 2;');

  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  await expect(page.locator('[data-testid="console-view"]')).toBeVisible();
  await page.locator('[data-testid="tab"][data-tab-kind="console"]').first().click();

  // The caret returns to the end of line 2, not the start of the buffer.
  await expect.poll(() => editorText(view)).toContain('SELECT 2;');
  await view.locator('textarea.inputarea').focus();
  await page.keyboard.type('X');
  await expect.poll(() => editorText(view)).toContain('SELECT 2;X');
});

function discoverStatement(): string {
  const { columns, ...request }: KibanaDiscoverRequest = {
    dataView: logsView.id,
    query: '',
    language: 'kuery',
    timeRange: resolveTime({ mode: 'preset', preset: 'last15m' }, NOW),
    filters: [],
    sort: [],
    size: 500,
    timeZone: browserTimeZone(),
  };
  return JSON.stringify(columns ? { ...request, columns } : request);
}

test('Discover keeps its field search and re-reads no data view or histogram after four minutes away', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: kibanaControl(),
    stream: [executeHits(discoverStatement(), 200)],
  });
  await page.clock.install({ time: NOW });
  await page.clock.setFixedTime(NOW);
  await connect(page);
  await (await findRow(page, logsNode.path)).dblclick();
  await expect(page.locator('[data-testid="kibana-results"]')).toBeVisible({ timeout: 15_000 });

  const search = page.locator('[data-testid="kibana-field-search"]');
  await search.fill('mess');
  // The data view definition has no gcTime override (P434 R1-F3): the default 5 min bounds it, so the
  // away time stays under that. The data view picker's list (kibanaDataViews) stays on a one-minute staleTime on purpose: Kibana
  // owns it and others add views. The view's own definition and the histogram are Kira's to keep.
  const kibanaReads = () =>
    control
      .log()
      .filter((e) => [IPC.kibanaDataView, IPC.kibanaHistogram].includes(e.channel as never)).length;
  const before = kibanaReads();

  await modeTab(page, 'api').click();
  await page.clock.fastForward(4 * 60_000);
  await modeTab(page, 'studio').click();

  await expect(page.locator('[data-testid="kibana-results"]')).toBeVisible();
  await expect(search).toHaveValue('mess');
  expect(kibanaReads()).toBe(before);
});

test('a gRPC tab keeps its schema and history after six minutes away', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.tabsList, response: [grpcTab({ target: 'demo.example.com:443' })] },
      { channel: IPC.grpcHistoryList, args: { itemId: '', tabId: 'tab-grpc-1' }, response: [] },
      { channel: IPC.grpcDescribe, response: { services: [], mode: 'reflection', warnings: [] } },
    ],
  });
  await page.clock.install({ time: NOW });
  await page.clock.setFixedTime(NOW);
  await modeTab(page, 'api').click();
  await expect(page.locator('[data-testid="grpc-request-view"]')).toBeVisible();
  const reads = () =>
    control.log().filter((e) => e.channel === IPC.grpcDescribe || e.channel === IPC.grpcHistoryList)
      .length;
  await expect.poll(reads).toBeGreaterThan(0);
  // The mount reads land in a burst with no DOM signal for its end; the baseline needs it done.
  await page.waitForTimeout(500);
  const before = reads();

  await modeTab(page, 'studio').click();
  await page.clock.fastForward(6 * 60_000);
  await modeTab(page, 'api').click();

  await expect(page.locator('[data-testid="grpc-request-view"]')).toBeVisible();
  // Absence of a read: no event to await.
  await page.waitForTimeout(500);
  expect(reads()).toBe(before);
});
