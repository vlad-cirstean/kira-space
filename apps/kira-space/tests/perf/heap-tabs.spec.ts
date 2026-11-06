import type { KibanaDiscoverRequest } from '@shared/domain/kibana';
import {
  closeAllTabs,
  type HeapTab,
  isReadChannel,
  runHeapScenario,
} from '@workbench/testing/ui/heapProbe';
import { browserTimeZone, resolveTime } from '../../frontend/src/studio/views/console/kibana/time';
import {
  controlSnapshots as redisControl,
  portSnapshots as redisPorts,
} from '../ipc/redis/redis.fixture';
import { controlSnapshots as sqsControl, portSnapshots as sqsPorts } from '../ipc/sqs/sqs.fixture';
import { expect, filesTest, type KiraApp, studioTest, test } from '../ui/fixtures';
import { grpcTab } from '../ui/studio/support/apiMode';
import { connectPostgres, ITEMS_CONTROL, ITEMS_PORT } from '../ui/studio/support/copyScenarios';
import { executeHits, kibanaControl, logsNode, logsView } from '../ui/studio/support/kibanaFixture';
import {
  DB_PATH as MONGO_DB,
  mongoConnectionSummary,
  WIDGETS_PATH,
  widgetsFixture,
} from '../ui/studio/support/mongoFixture';
import { ORDER_ITEMS_PATH } from '../ui/studio/support/postgresFixture';
import { expandRow, findRow, openBrowseLeaf } from '../ui/studio/support/tree';
import { gitStreamRequests } from '../ui/support/gitStreamMock';
import {
  manyRowShas,
  manyRows,
  openPortGraph,
  PORT_REPO,
  refsAtTip,
  singleRowChunks,
} from '../ui/support/gitUiPortFixtures';
import { IPC } from '../ui/support/ipcChannels';
import { treeRow } from '../ui/support/repoRows';
import type { ControlSnapshot } from '../ui/support/types';
import { connect, menuOpen, modeTab, NOW, pinClock } from './support/ramWorlds';

// P433 heap probe: per tab kind, JS heap per open tab, switch rounds, six minutes away, close all.
// Chromium only (CDP); report-only. `bunx playwright test -c playwright.perf.config.ts heap-tabs
// --project heap`. A Finder-opened local file lives in the files window (no modes, so no away step), so it is its own test.

const LARGE_LINES = Number(process.env.KIRA_HEAP_FILE_LINES ?? 40_000);
const LARGE_TEXT = Array.from(
  { length: LARGE_LINES },
  (_, i) => `export const value${i} = ${i}; // generated line ${i}\n`,
).join('');
const LOCAL = { path: '/tmp/outside/b.json', relPath: '', repoId: '' };

const A_STAT = { size: LARGE_TEXT.length, hash: 'hash-a' };
const C_STAT = { size: 20, hash: 'hash-c' };
const LIMIT = 8 * 1024 * 1024;

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.codeWorkspaceListRepos, response: [PORT_REPO] },
  {
    channel: IPC.codeWorkspaceListFiles,
    args: { id: PORT_REPO.id },
    response: { paths: ['a.ts', 'c.ts'], status: { 'c.ts': 'M' }, truncated: false },
  },
  {
    channel: IPC.codeWorkspaceReadFile,
    args: { id: PORT_REPO.id, path: 'a.ts' },
    response: {
      kind: 'found',
      text: LARGE_TEXT,
      bytes: LARGE_TEXT.length,
      limitBytes: 8 * 1024 * 1024,
      language: 'typescript',
      stat: A_STAT,
    },
  },
  // The backend answers a matching ifStat with no content.
  {
    channel: IPC.codeWorkspaceReadFile,
    args: { id: PORT_REPO.id, path: 'a.ts', ifStat: A_STAT },
    response: { kind: 'unchanged', text: '', bytes: 0, limitBytes: LIMIT, stat: A_STAT },
  },
  {
    channel: IPC.codeWorkspaceReadDiff,
    args: { id: PORT_REPO.id, path: 'c.ts', ifStat: C_STAT, ifHeadOid: 'h1' },
    response: {
      path: 'c.ts',
      language: 'typescript',
      head: { kind: 'unchanged', text: '', bytes: 0, limitBytes: LIMIT, oid: 'h1' },
      worktree: { kind: 'unchanged', text: '', bytes: 0, limitBytes: LIMIT, stat: C_STAT },
    },
  },
  {
    channel: IPC.codeWorkspaceReadDiff,
    args: { id: PORT_REPO.id, path: 'c.ts' },
    response: {
      path: 'c.ts',
      language: 'typescript',
      head: {
        kind: 'found',
        text: 'export const c = 1;\n',
        bytes: 20,
        limitBytes: 8 * 1024 * 1024,
        oid: 'h1',
      },
      worktree: {
        kind: 'found',
        text: 'export const c = 2;\n',
        bytes: 20,
        limitBytes: 8 * 1024 * 1024,
        stat: C_STAT,
      },
    },
  },
];

test('heap: repo graph, repo file, diff', async ({ relaunch }) => {
  let control: KiraApp['control'] | undefined;
  const page = await openPortGraph(
    async (o) => {
      const app = await relaunch(o);
      control = app.control;
      return app;
    },
    {
      chunks: singleRowChunks(manyRows(300)),
      results: { 'refs.list': refsAtTip(manyRowShas(0)) },
      control: CONTROL,
    },
  );
  await page.clock.install();
  const tabs: HeapTab[] = [
    {
      name: 'repo-graph',
      open: async () => {
        await expect(page.locator('[data-testid="commit-grid"] .slick-row').first()).toBeVisible({
          timeout: 15_000,
        });
      },
    },
    {
      name: 'repo-file',
      open: async () => {
        await page.locator('[data-testid="git-panel-tab-files"]').click();
        await treeRow(page, 'a.ts').click();
        await expect(page.locator('[data-testid="repo-file-editor"] .view-lines')).toContainText(
          'value0',
        );
      },
    },
    {
      name: 'diff',
      open: async () => {
        await treeRow(page, 'c.ts').click({ button: 'right' });
        await page.locator('[data-testid="menu-item-open-changes"]').click();
        await expect(page.locator('[data-testid="repo-diff-editor"]')).toBeVisible();
      },
    },
  ];
  await runHeapScenario({
    page,
    tabs,
    rounds: 5,
    closeAll: () => closeAllTabs(page, 1) /* the graph tab is pinned */,
    reads: async () => [
      ...(control?.log() ?? []).map((e) => e.channel).filter(isReadChannel),
      ...(await gitStreamRequests(page)).map((m) => `git:${m}`),
    ],
  });
});

filesTest('heap: local file', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.openedFilesIsFilesWindow, response: true },
      { channel: IPC.openedFilesPending, response: [LOCAL] },
      {
        channel: IPC.openedFilesRead,
        args: { path: LOCAL.path },
        response: {
          kind: 'found',
          text: '{"a":1}\n',
          bytes: 8,
          limitBytes: 8 * 1024 * 1024,
          language: 'json',
        },
      },
    ],
  });
  await expect(page.locator('[data-testid="tab"][data-tab-kind="local-file"]')).toHaveCount(1);
  await page.clock.install();
  await runHeapScenario({
    page,
    tabs: [{ name: 'local-file', open: async () => {}, bootOnly: true }],
    closeAll: () => closeAllTabs(page),
    reads: async () =>
      control
        .log()
        .map((e) => e.channel)
        .filter(isReadChannel),
  });
});

studioTest.describe('Studio tab kinds', () => {
  // P433 heap probe: per tab kind, JS heap per open tab, switch rounds, six minutes away, close all.
  // Chromium only (CDP); report-only. `bunx playwright test -c playwright.perf.config.ts heap-tabs
  // --project heap`. Each world is one fixture set, so kinds sharing a connection run together.

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

  studioTest('heap: postgres grid, definition, console', async ({ relaunch }) => {
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

  studioTest('heap: mongo document', async ({ relaunch }) => {
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

  studioTest('heap: redis browse and keyvalue', async ({ relaunch }) => {
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

  studioTest('heap: sqs stream', async ({ relaunch }) => {
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
            const view = page.locator(
              '[data-testid="stream-view"][data-path="queue:orders-queue"]',
            );
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

  studioTest('heap: kibana discover', async ({ relaunch }) => {
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

  studioTest('heap: api grpc and http request', async ({ relaunch }) => {
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
});
