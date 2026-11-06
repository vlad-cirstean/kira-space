import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import type { Page } from '@playwright/test';
import type { KibanaDiscoverRequest } from '@shared/domain/kibana';
import { closeAllTabs } from '@workbench/testing/ui/heapProbe';
import { diffSnapshotFiles } from '@workbench/testing/ui/heapSnapshot';
import { MemSampler, type MemStep, runMemSteps, SNAPSHOTS } from '@workbench/testing/ui/memProbe';
import { emitWailsEvent } from '@workbench/testing/ui/mockRuntime';
import {
  browserTimeZone,
  resolveTime,
} from '../../../frontend/src/studio/views/console/kibana/time';
import {
  controlSnapshots as redisControl,
  portSnapshots as redisPorts,
} from '../../ipc/redis/redis.fixture';
import {
  controlSnapshots as sqsControl,
  portSnapshots as sqsPorts,
} from '../../ipc/sqs/sqs.fixture';
import { expect, test } from '../../ui/studio/fixtures';
import { grpcTab } from '../../ui/studio/support/apiMode';
import { connectPostgres, ITEMS_CONTROL, ITEMS_PORT } from '../../ui/studio/support/copyScenarios';
import { container, openDocker, setupDocker } from '../../ui/studio/support/dockerFixture';
import { IPC } from '../../ui/studio/support/ipcChannels';
import {
  executeHits,
  kibanaControl,
  logsNode,
  logsView,
} from '../../ui/studio/support/kibanaFixture';
import {
  loc,
  openKind,
  openKubernetes,
  setupKubernetes,
} from '../../ui/studio/support/kubernetesFixture';
import {
  DB_PATH as MONGO_DB,
  mongoConnectionSummary,
  WIDGETS_PATH,
  widgetsFixture,
} from '../../ui/studio/support/mongoFixture';
import { ORDER_ITEMS_PATH } from '../../ui/studio/support/postgresFixture';
import { expandRow, findRow, openBrowseLeaf } from '../../ui/studio/support/tree';
import { CONTROL, FIXTURE, makeDocs, openWidgets, readSnapshot } from './documentsFixture';
import {
  APP,
  connect,
  feedTerminal,
  lastStreamId,
  menuOpen,
  modeTab,
  newTerminal,
  pinClock,
  tabWorld,
  worldEnabled,
} from './support/ramWorlds';

// P480 RAM probe, Studio worlds. Report-only: prints `RAM`/`HEAPSNAP` lines, asserts only on the UI
// states that gate a checkpoint. `bun run perf:ram:studio`; docs/DEV_ENVIRONMENT.md "Perf probes".

const MODES = ['api', 'automations', 'docker', 'kubernetes'] as const;

test('ram: idle and modes', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('idle'));
  const { window: page } = await relaunch({});
  await pinClock(page);
  const steps: MemStep[] = [
    { name: 'boot60', run: () => page.waitForTimeout(60_000) },
    ...MODES.flatMap((m): MemStep[] => [
      {
        name: `mode:${m}`,
        run: async () => {
          await modeTab(page, m as 'api').click();
          await page.waitForTimeout(1500);
        },
        snapshot: m === 'kubernetes',
      },
      {
        name: `back:${m}`,
        run: async () => {
          await modeTab(page, 'studio').click();
          await page.waitForTimeout(1500);
        },
      },
    ]),
  ];
  await runMemSteps({ app: APP, world: 'idle', page, info, steps });
});

test('ram: grid', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('grid'));
  const { window: page } = await relaunch({ control: ITEMS_CONTROL, stream: ITEMS_PORT });
  await connectPostgres(page);
  await pinClock(page);
  await tabWorld({
    world: 'grid',
    page,
    info,
    view: 'data-grid',
    first: async () => {
      await (await findRow(page, ORDER_ITEMS_PATH)).dblclick();
      await expect(page.locator('[data-testid="data-grid"]')).toBeVisible({ timeout: 15_000 });
    },
  });
});

test('ram: console', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('console'));
  const { window: page } = await relaunch({ control: ITEMS_CONTROL, stream: ITEMS_PORT });
  await connectPostgres(page);
  await pinClock(page);
  await tabWorld({
    world: 'console',
    page,
    info,
    view: 'console-view',
    first: menuOpen(page, ORDER_ITEMS_PATH, 'open-console', 'console-view'),
  });
});

test('ram: definition', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('definition'));
  const { window: page } = await relaunch({ control: ITEMS_CONTROL, stream: ITEMS_PORT });
  await connectPostgres(page);
  await pinClock(page);
  await tabWorld({
    world: 'definition',
    page,
    info,
    view: 'definition-view',
    first: menuOpen(page, ORDER_ITEMS_PATH, 'open-definition', 'definition-view'),
  });
});

test('ram: document', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('document'));
  const conn = mongoConnectionSummary('conn-ram-doc', 'Mongo DB', 'green');
  const fixture = widgetsFixture(conn.id);
  const { window: page } = await relaunch({
    control: [{ channel: IPC.connectionsList, response: [conn] }, ...fixture.control],
    stream: fixture.port,
  });
  await connect(page);
  await expandRow(page, MONGO_DB);
  await pinClock(page);
  await tabWorld({
    world: 'document',
    page,
    info,
    view: 'document-row',
    first: async () => {
      await (await findRow(page, WIDGETS_PATH)).dblclick();
      await expect(page.locator('[data-testid="document-row"]').first()).toBeVisible({
        timeout: 15_000,
      });
    },
  });
});

test('ram: keyvalue', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('keyvalue'));
  const { window: page } = await relaunch({ control: redisControl, stream: redisPorts });
  await connect(page);
  await pinClock(page);
  const db0 = 'database:db0';
  const ns = `${db0}/namespace:user`;
  const ns1 = `${ns}/namespace:1`;
  const key = `${ns1}/key:user%3A1%3Abighash`;
  await tabWorld({
    world: 'keyvalue',
    page,
    info,
    view: 'keyvalue-view',
    kind: 'keyvalue',
    first: async () => {
      await (await findRow(page, db0)).dblclick();
      const browse = page.locator('[data-testid="browse-view"]');
      await expect(browse).toBeVisible();
      await browse.locator(`[data-testid="browse-row"][data-path="${ns}"]`).dblclick();
      await browse.locator(`[data-testid="browse-row"][data-path="${ns1}"]`).dblclick();
      await openBrowseLeaf(page, key);
      await expect(
        page
          .locator(`[data-testid="keyvalue-view"][data-path="${key}"] [data-testid="keyvalue-row"]`)
          .first(),
      ).toBeVisible({ timeout: 15_000 });
    },
  });
});

test('ram: stream', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('stream'));
  const { window: page } = await relaunch({ control: sqsControl, stream: sqsPorts });
  await connect(page);
  await pinClock(page);
  const poll = async () => {
    const view = page.locator('[data-testid="stream-view"]:visible').first();
    await view.locator('[data-testid="stream-poll"]').click();
    await expect(view.locator('[data-testid="stream-row"]').first()).toBeVisible({
      timeout: 15_000,
    });
  };
  await tabWorld({
    world: 'stream',
    page,
    info,
    view: 'stream-view',
    first: async () => {
      await (await findRow(page, 'queue:orders-queue')).dblclick();
      await poll();
    },
    // A duplicate has an empty buffer until polled.
    next: async (i) => {
      const tabs = page.locator('[data-testid="tab"]');
      await page
        .locator('[data-testid="tab"][data-tab-kind="stream"]')
        .first()
        .click({ button: 'right' });
      await page.click('[data-testid="menu-item-duplicate-tab"]');
      await expect(tabs).toHaveCount(i + 1);
      await poll();
    },
  });
});

const NOW = new Date('2026-01-15T12:00:00.000Z');

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

test('ram: discover', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('discover'));
  const { window: page } = await relaunch({
    control: kibanaControl(),
    stream: [executeHits(discoverStatement(), 200)],
  });
  await connect(page);
  await pinClock(page);
  const round = (n: number): MemStep => ({
    name: `round${n}`,
    run: async () => {
      await modeTab(page, 'api').click();
      await page.waitForTimeout(500);
      await modeTab(page, 'studio').click();
      await page.waitForTimeout(500);
    },
    snapshot: n === 3 || n === 5 ? 'keep' : false,
  });
  const s = await runMemSteps({
    app: APP,
    world: 'discover',
    page,
    info,
    steps: [
      {
        name: 'open1',
        run: async () => {
          await (await findRow(page, logsNode.path)).dblclick();
          await expect(page.locator('[data-testid="kibana-results"]')).toBeVisible({
            timeout: 15_000,
          });
        },
        snapshot: true,
      },
      ...[1, 2, 3, 4, 5, 6, 7, 8].map(round),
      {
        name: 'closed',
        run: () => closeAllTabs(page),
      },
    ],
  });
  if (SNAPSHOTS) {
    const d = await diffSnapshotFiles(s.snapshotFile('round3'), s.snapshotFile('round5'));
    console.log(`HEAPDIFF ${APP} discover round3-round5 ${JSON.stringify(d)}`);
  }
});

test('ram: api', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('api'));
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.tabsList, response: [grpcTab({ target: 'demo.example.com:443' })] },
      { channel: IPC.grpcHistoryList, args: { itemId: '', tabId: 'tab-grpc-1' }, response: [] },
      { channel: IPC.grpcDescribe, response: { services: [], mode: 'reflection', warnings: [] } },
    ],
  });
  await pinClock(page);
  const s = await tabWorld({
    world: 'api',
    page,
    info,
    snapshotAt: ['closed', 'closed2', 'closed3'],
    enter: async () => {
      await modeTab(page, 'api').click();
      await expect(page.locator('[data-testid="grpc-request-view"]')).toBeVisible();
    },
    first: async () => {
      if ((await page.locator('[data-testid="tab"]').count()) === 0)
        await page.click('[data-testid="new-grpc-request-start"]');
    },
    next: async () => {
      await page.click('[data-testid="new-request"]');
      await expect(page.locator('[data-testid="http-request-view"]:visible').first()).toBeVisible();
    },
  });
  if (SNAPSHOTS) {
    const d = await diffSnapshotFiles(s.snapshotFile('closed2'), s.snapshotFile('closed3'));
    console.log(`HEAPDIFF ${APP} api closed2-closed3 ${JSON.stringify(d)}`);
  }
});

test('ram: terminal', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('terminal'));
  const { window: page } = await relaunch({
    control: [{ channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } }],
  });
  await pinClock(page);
  await tabWorld({
    world: 'terminal',
    page,
    info,
    units: 5,
    enter: async () => {
      await modeTab(page, 'automations').click();
    },
    first: async () => {
      const id = await newTerminal(page);
      await feedTerminal(page, IPC.terminal, id, 10_000);
    },
    next: async () => {
      const id = await newTerminal(page);
      await feedTerminal(page, IPC.terminal, id, 10_000);
    },
  });
});

const NCONTAINERS = Number(process.env.NCONTAINERS ?? 400);

async function feedLines<T>(
  page: Page,
  channel: string,
  streamId: string,
  from: number,
  to: number,
  line: (i: number) => T,
): Promise<void> {
  const chunk = 500;
  for (let at = from; at < to; at += chunk) {
    const lines: T[] = [];
    for (let i = at; i < Math.min(at + chunk, to); i++) lines.push(line(i));
    await emitWailsEvent(page, channel, { streamId, lines, ended: false });
  }
}

const logText = (i: number) =>
  `2026-01-15T12:00:00Z INFO request ${i} handled in ${i % 97} ms id=${i * 7919}`;

test('ram: docker', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('docker'));
  const list = [
    container('c-solo', 'solo', 'running'),
    ...Array.from({ length: NCONTAINERS }, (_, i) =>
      container(`c-${String(i).padStart(4, '0')}`, `svc-${String(i).padStart(4, '0')}`, 'running'),
    ),
  ];
  const { page, docker } = await setupDocker(relaunch, { handlers: { Containers: () => list } });
  await pinClock(page);
  let streamId = '';
  const logsTo = (name: string, to: number, from: number): MemStep => ({
    name,
    run: () =>
      feedLines(page, 'kira:docker:logs', streamId, from, to, (i) => ({
        stream: 'stdout',
        text: logText(i),
      })),
    snapshot: name === 'logs50k',
  });
  await runMemSteps({
    app: APP,
    world: 'docker',
    page,
    info,
    steps: [
      { name: 'list', run: () => openDocker(page), snapshot: true },
      {
        name: 'logs0',
        run: async () => {
          await page.locator('[data-testid="docker-row"][data-name="solo"]').click();
          await page.locator('[data-testid="docker-tab-logs"]').click();
          await expect.poll(() => docker.calls('LogsOpen').length).toBeGreaterThan(0);
          streamId = lastStreamId(docker.calls('LogsOpen'));
        },
      },
      logsTo('logs1k', 1000, 0),
      logsTo('logs10k', 10_000, 1000),
      logsTo('logs50k', 50_000, 10_000),
      {
        name: 'left',
        run: async () => {
          await modeTab(page, 'studio').click();
          await page.waitForTimeout(1000);
        },
        snapshot: true,
      },
    ],
  });
});

test('ram: kubernetes', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('kubernetes'));
  const { page, kube } = await setupKubernetes(relaunch);
  await pinClock(page);
  let streamId = '';
  const logsTo = (name: string, to: number, from: number): MemStep => ({
    name,
    run: () =>
      feedLines(page, 'kira:kubernetes:logs', streamId, from, to, (i) => ({ text: logText(i) })),
    snapshot: name === 'logs50k',
  });
  await runMemSteps({
    app: APP,
    world: 'kubernetes',
    page,
    info,
    steps: [
      { name: 'panel', run: () => openKubernetes(page), snapshot: true },
      {
        name: 'logs0',
        run: async () => {
          await openKind(page, 'kind-dev', 'v1/pods');
          await page.locator('[data-testid="kube-row"][data-name="web-1"]').click();
          await loc(page, 'kube-detail-tab-logs').click();
          await expect.poll(() => kube.calls('LogsOpen').length).toBeGreaterThan(0);
          streamId = lastStreamId(kube.calls('LogsOpen'));
        },
      },
      logsTo('logs1k', 1000, 0),
      logsTo('logs10k', 10_000, 1000),
      logsTo('logs50k', 50_000, 10_000),
      {
        name: 'left',
        run: async () => {
          await modeTab(page, 'studio').click();
          await page.waitForTimeout(1000);
        },
        snapshot: true,
      },
    ],
  });
});

test('ram: documents-list', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('documents-list'));
  const docs = makeDocs(Number(process.env.NDOCS ?? 5000));
  const { window: page } = await relaunch({
    control: CONTROL,
    stream: [
      readSnapshot(100, docs, true),
      readSnapshot(10000, docs, false),
      ...FIXTURE.port.slice(1),
    ],
  });
  await pinClock(page);
  await runMemSteps({
    app: APP,
    world: 'documents-list',
    page,
    info,
    steps: [
      { name: 'open100', run: () => openWidgets(page), snapshot: true },
      {
        name: 'open5000',
        run: async () => {
          await page.click('[data-testid="document-page-size-10000"]');
          await page.waitForTimeout(2000);
        },
        snapshot: true,
      },
      { name: 'closed', run: () => closeAllTabs(page), snapshot: true },
    ],
  });
});

test('ram: i18n catalog parse', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('i18n'));
  const { window: page } = await relaunch({});
  await page.goto('about:blank');
  const s = await MemSampler.attach(APP, 'i18n', page, info);
  await s.sample('blank');
  const json = readFileSync(
    resolve(__dirname, '../../../../../packages/theme/src/i18n/en.json'),
    'utf8',
  );
  await page.evaluate((text) => {
    (window as unknown as { __catalog: unknown }).__catalog = JSON.parse(text);
  }, json);
  await s.sample('parsed');
});
