import type { Page } from '@playwright/test';
import type { KibanaDiscoverRequest } from '@shared/domain/kibana';
import {
  closeAllTabs,
  type HeapTab,
  isReadChannel,
  runHeapScenario,
} from '@workbench/testing/ui/heapProbe';
import { browserTimeZone, resolveTime } from '../../../frontend/src/studio/views/console/kibana/time';
import {
  controlSnapshots as redisControl,
  portSnapshots as redisPorts,
} from '../../ipc/redis/redis.fixture';
import { controlSnapshots as sqsControl, portSnapshots as sqsPorts } from '../../ipc/sqs/sqs.fixture';
import { expect, type KiraApp, test } from '../../ui/studio/fixtures';
import { grpcTab, modeTab } from '../../ui/studio/support/apiMode';
import { connectPostgres, ITEMS_CONTROL, ITEMS_PORT } from '../../ui/studio/support/copyScenarios';
import { IPC } from '../../ui/studio/support/ipcChannels';
import { executeHits, kibanaControl, logsNode, logsView } from '../../ui/studio/support/kibanaFixture';
import {
  DB_PATH as MONGO_DB,
  mongoConnectionSummary,
  WIDGETS_PATH,
  widgetsFixture,
} from '../../ui/studio/support/mongoFixture';
import { ORDER_ITEMS_PATH } from '../../ui/studio/support/postgresFixture';
import { connectionRow, expandRow, findRow, openBrowseLeaf, openRowMenu } from '../../ui/studio/support/tree';

// P433 heap probe: per tab kind, JS heap per open tab, switch rounds, six minutes away, close all.
// Chromium only (CDP); report-only. `bunx playwright test -c playwright.perf.config.ts heap-tabs
// --project heap`. Each world is one fixture set, so kinds sharing a connection run together.

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

async function pinClock(page: Page): Promise<void> {
  await page.clock.install({ time: NOW });
  await page.clock.setFixedTime(NOW);
}

async function run(app: KiraApp, tabs: HeapTab[], away = false, rounds?: number): Promise<void> {
  const { window: page, control, stream } = app;
  await pinClock(page);
  await runHeapScenario({
    page,
    tabs,
    rounds,
    closeAll: () => closeAllTabs(page),
    away: away
      ? {
          leave: () => modeTab(page, 'api').click(),
          back: () => modeTab(page, 'studio').click(),
        }
      : undefined,
    reads: async () => [
      ...control
        .log()
        .map((e) => e.channel)
        .filter(isReadChannel),
      ...(await stream.ops()).map((o) => `port:${o.op}`),
    ],
  });
}

const menuOpen = (page: Page, path: string, item: string, view: string) => async () => {
  await openRowMenu(page, path);
  await page.click(`[data-testid="menu-item-${item}"]`);
  await expect(page.locator(`[data-testid="${view}"]`)).toBeVisible({ timeout: 15_000 });
};

test('heap: postgres grid, definition, console', async ({ relaunch }) => {
  const app = await relaunch({ control: ITEMS_CONTROL, stream: ITEMS_PORT });
  const page = app.window;
  await connectPostgres(page);
  await run(app, [
    {
      name: 'data-grid',
      open: async () => {
        await (await findRow(page, ORDER_ITEMS_PATH)).dblclick();
        await expect(page.locator('[data-testid="data-grid"]')).toBeVisible({ timeout: 15_000 });
      },
    },
    {
      name: 'definition',
      open: menuOpen(page, ORDER_ITEMS_PATH, 'open-definition', 'definition-view'),
    },
    { name: 'console', open: menuOpen(page, ORDER_ITEMS_PATH, 'open-console', 'console-view') },
  ]);
});

test('heap: mongo document', async ({ relaunch }) => {
  const conn = mongoConnectionSummary('conn-heap-doc', 'Mongo DB', 'green');
  const fixture = widgetsFixture(conn.id);
  const app = await relaunch({
    control: [{ channel: IPC.connectionsList, response: [conn] }, ...fixture.control],
    stream: fixture.port,
  });
  const page = app.window;
  await connect(page);
  await expandRow(page, MONGO_DB);
  await run(
    app,
    [
      {
        name: 'document',
        open: async () => {
          await (await findRow(page, WIDGETS_PATH)).dblclick();
          await expect(page.locator('[data-testid="document-row"]').first()).toBeVisible({
            timeout: 15_000,
          });
        },
      },
    ],
    true,
  );
});

test('heap: redis browse and keyvalue', async ({ relaunch }) => {
  const app = await relaunch({ control: redisControl, stream: redisPorts });
  const page = app.window;
  await connect(page);
  const db0 = 'database:db0';
  const ns = `${db0}/namespace:user`;
  const ns1 = `${ns}/namespace:1`;
  const key = `${ns1}/key:user%3A1%3Abighash`;
  await run(app, [
    {
      name: 'browse',
      open: async () => {
        await (await findRow(page, db0)).dblclick();
        await expect(page.locator('[data-testid="browse-view"]')).toBeVisible();
      },
    },
    {
      name: 'keyvalue',
      open: async () => {
        const browse = page.locator('[data-testid="browse-view"]');
        await browse.locator(`[data-testid="browse-row"][data-path="${ns}"]`).dblclick();
        await browse.locator(`[data-testid="browse-row"][data-path="${ns1}"]`).dblclick();
        await openBrowseLeaf(page, key);
        await expect(
          page
            .locator(
              `[data-testid="keyvalue-view"][data-path="${key}"] [data-testid="keyvalue-row"]`,
            )
            .first(),
        ).toBeVisible({ timeout: 15_000 });
      },
    },
  ]);
});

test('heap: sqs stream', async ({ relaunch }) => {
  const app = await relaunch({ control: sqsControl, stream: sqsPorts });
  const page = app.window;
  await connect(page);
  await run(
    app,
    [
      {
        name: 'stream',
        open: async () => {
          await (await findRow(page, 'queue:orders-queue')).dblclick();
          const view = page.locator('[data-testid="stream-view"][data-path="queue:orders-queue"]');
          await view.locator('[data-testid="stream-poll"]').click();
          await expect(view.locator('[data-testid="stream-row"]').first()).toBeVisible({
            timeout: 15_000,
          });
        },
      },
    ],
    true,
  );
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

test('heap: kibana discover', async ({ relaunch }) => {
  const app = await relaunch({
    control: kibanaControl(),
    stream: [executeHits(discoverStatement(), 200)],
  });
  const page = app.window;
  await connect(page);
  await run(
    app,
    [
      {
        name: 'discover',
        open: async () => {
          await (await findRow(page, logsNode.path)).dblclick();
          await expect(page.locator('[data-testid="kibana-results"]')).toBeVisible({
            timeout: 15_000,
          });
        },
      },
    ],
    true,
    5,
  );
});

test('heap: api grpc and http request', async ({ relaunch }) => {
  const app = await relaunch({
    control: [
      { channel: IPC.tabsList, response: [grpcTab({ target: 'demo.example.com:443' })] },
      { channel: IPC.grpcHistoryList, args: { itemId: '', tabId: 'tab-grpc-1' }, response: [] },
      {
        channel: IPC.grpcDescribe,
        response: { services: [], mode: 'reflection', warnings: [] },
      },
    ],
  });
  const page = app.window;
  await modeTab(page, 'api').click();
  await expect(page.locator('[data-testid="grpc-request-view"]')).toBeVisible();
  await run(app, [
    {
      name: 'grpc-request',
      open: async () => {
        // Restored from the tab list on first open; the start screen after close-all.
        if ((await page.locator('[data-testid="tab"]').count()) === 0)
          await page.click('[data-testid="new-grpc-request-start"]');
      },
    },
    {
      name: 'http-request',
      open: async () => {
        await page.click('[data-testid="new-request"]');
        await expect(page.locator('[data-testid="http-request-view"]')).toBeVisible();
      },
    },
  ]);
});
