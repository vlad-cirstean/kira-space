import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import type { Page } from '@playwright/test';
import type { KibanaDiscoverRequest } from '@shared/domain/kibana';
import { closeAllTabs } from '@workbench/testing/ui/heapProbe';
import { diffSnapshotFiles } from '@workbench/testing/ui/heapSnapshot';
import { MemSampler, type MemStep, runMemSteps, SNAPSHOTS } from '@workbench/testing/ui/memProbe';
import { emitWailsEvent } from '@workbench/testing/ui/mockRuntime';
import { browserTimeZone, resolveTime } from '../../frontend/src/studio/views/console/kibana/time';
import {
  controlSnapshots as redisControl,
  portSnapshots as redisPorts,
} from '../ipc/redis/redis.fixture';
import { controlSnapshots as sqsControl, portSnapshots as sqsPorts } from '../ipc/sqs/sqs.fixture';
import { expect, filesTest, studioTest, test } from '../ui/fixtures';
import { grpcTab } from '../ui/studio/support/apiMode';
import { connectPostgres, ITEMS_CONTROL, ITEMS_PORT } from '../ui/studio/support/copyScenarios';
import { container, openDocker, setupDocker } from '../ui/studio/support/dockerFixture';
import { executeHits, kibanaControl, logsNode, logsView } from '../ui/studio/support/kibanaFixture';
import {
  loc,
  openKind,
  openKubernetes,
  setupKubernetes,
} from '../ui/studio/support/kubernetesFixture';
import {
  DB_PATH as MONGO_DB,
  mongoConnectionSummary,
  WIDGETS_PATH,
  widgetsFixture,
} from '../ui/studio/support/mongoFixture';
import { ORDER_ITEMS_PATH } from '../ui/studio/support/postgresFixture';
import { expandRow, findRow, openBrowseLeaf } from '../ui/studio/support/tree';
import { adeBoard, adeV2Control, openTask } from '../ui/support/adeV2';
import { installGitStreamMock } from '../ui/support/gitStreamMock';
import {
  BASE_RESULTS,
  manyRowShas,
  manyRows,
  openPortGraph,
  PORT_CONTROL,
  PORT_REPO,
  refsAtTip,
  singleRowChunks,
} from '../ui/support/gitUiPortFixtures';
import { IPC } from '../ui/support/ipcChannels';
import { openMemory } from '../ui/support/memoryNav';
import { treeRow } from '../ui/support/repoRows';
import type { ControlSnapshot } from '../ui/support/types';
import { CONTROL, FIXTURE, makeDocs, openWidgets, readSnapshot } from './studio/documentsFixture';
import {
  APP,
  connect,
  duplicateOpen,
  feedTerminal,
  graphChunks,
  lastStreamId,
  menuOpen,
  modeTab,
  NOW,
  newTerminal,
  openTerminalTab,
  pinClock,
  type TabWorldOptions,
  tabWorld,
  worldEnabled,
} from './support/ramWorlds';

// P480 RAM probe, Space worlds. Report-only: prints `RAM`/`HEAPSNAP` lines, asserts only on the UI
// states that gate a checkpoint. `bun run perf:ram`; docs/DEV_ENVIRONMENT.md "Perf probes".

const repoRow = (page: import('@playwright/test').Page, id: string) =>
  page.locator(`[data-testid="repo-row"][data-repo-id="${id}"]`);

test('ram: idle and modes', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('idle'));
  const { window: page } = await relaunch({ control: PORT_CONTROL });
  await page.clock.install();
  const hop = (m: string, back: string, snapshot = false): MemStep[] => [
    {
      name: `mode:${m}`,
      run: async () => {
        await modeTab(page, m).click();
        await page.waitForTimeout(1500);
      },
      snapshot,
    },
    {
      name: `back:${m}`,
      run: async () => {
        await modeTab(page, back).click();
        await page.waitForTimeout(1500);
      },
    },
  ];
  await runMemSteps({
    app: APP,
    world: 'idle',
    page,
    info,
    steps: [
      { name: 'boot60', run: () => page.waitForTimeout(60_000) },
      ...hop('ade', 'git'),
      ...hop('automations', 'git'),
      ...hop('memory', 'git'),
      { name: 'mode:studio', run: () => modeTab(page, 'studio').click() },
      ...hop('api', 'studio'),
      ...hop('docker', 'studio'),
      ...hop('kubernetes', 'studio', true),
    ],
  });
});

for (const rows of [10_000, 50_000]) {
  test(`ram: graph ${rows / 1000}k`, async ({ relaunch }, info) => {
    test.skip(!worldEnabled('graph'));
    test.setTimeout(900_000);
    const { chunks, tip } = graphChunks(PORT_REPO.repoId, rows);
    const { window: page } = await relaunch({ control: PORT_CONTROL });
    await installGitStreamMock(
      page,
      PORT_REPO.repoId,
      { ...BASE_RESULTS, 'refs.list': refsAtTip(tip) },
      chunks,
    );
    await page.clock.install();
    await runMemSteps({
      app: APP,
      world: `graph${rows / 1000}k`,
      page,
      info,
      steps: [
        {
          name: 'open',
          run: async () => {
            await repoRow(page, PORT_REPO.id).click();
            await expect(
              page.locator('[data-testid="commit-grid"] .slick-row[data-row="5"]'),
            ).toBeVisible({ timeout: 400_000 });
            await page.waitForTimeout(5000);
          },
          snapshot: true,
        },
        {
          name: 'left',
          run: async () => {
            await modeTab(page, 'automations').click();
            await page.waitForTimeout(1500);
          },
          snapshot: true,
        },
        {
          name: 'back',
          run: async () => {
            await modeTab(page, 'git').click();
            await page.waitForTimeout(1500);
          },
        },
      ],
    });
  });
}

test('ram: kept-graphs', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('kept-graphs'));
  const repos = Array.from({ length: 5 }, (_, i) => ({
    ...PORT_REPO,
    id: `repo-kept-${i}`,
    name: `kept-repo-${i}`,
    root: `/tmp/kept-repo-${i}`,
    sortOrder: i + 1,
  }));
  const control: ControlSnapshot[] = [
    { channel: IPC.codeWorkspaceListRepos, response: repos },
    ...repos.map((r) => ({
      channel: IPC.codeWorkspaceListFiles,
      args: { id: r.id },
      response: { paths: ['a.ts'], status: {}, truncated: false },
    })),
  ];
  const { window: page } = await relaunch({ control });
  await installGitStreamMock(
    page,
    PORT_REPO.repoId,
    { ...BASE_RESULTS, 'refs.list': refsAtTip(manyRowShas(0)) },
    singleRowChunks(manyRows(300)),
  );
  await page.clock.install();
  const open = (i: number): MemStep => ({
    name: `graph${i + 1}`,
    run: async () => {
      await repoRow(page, repos[i].id).click();
      await expect(
        page.locator('[data-testid="commit-grid"] .slick-row:visible').first(),
      ).toBeVisible({ timeout: 30_000 });
      await page.waitForTimeout(1000);
    },
    snapshot: i === 3 || i === 4,
  });
  await runMemSteps({
    app: APP,
    world: 'kept-graphs',
    page,
    info,
    steps: [0, 1, 2, 3, 4].map(open),
  });
});

const FILE_LINES = Number(process.env.KIRA_HEAP_FILE_LINES ?? 40_000);
const LARGE_TEXT = Array.from(
  { length: FILE_LINES },
  (_, i) => `export const value${i} = ${i}; // generated line ${i}\n`,
).join('');
const LIMIT = 8 * 1024 * 1024;
const FILES = Array.from({ length: 10 }, (_, i) => `f${i}.ts`);
const DIFFS = Array.from({ length: 10 }, (_, i) => `d${i}.ts`);

function fileControl(): ControlSnapshot[] {
  const out: ControlSnapshot[] = [
    { channel: IPC.codeWorkspaceListRepos, response: [PORT_REPO] },
    {
      channel: IPC.codeWorkspaceListFiles,
      args: { id: PORT_REPO.id },
      response: {
        paths: [...FILES, ...DIFFS],
        status: Object.fromEntries(DIFFS.map((d) => [d, 'M'])),
        truncated: false,
      },
    },
  ];
  for (const path of FILES) {
    const stat = { size: LARGE_TEXT.length, hash: `hash-${path}` };
    out.push(
      {
        channel: IPC.codeWorkspaceReadFile,
        args: { id: PORT_REPO.id, path },
        response: {
          kind: 'found',
          text: LARGE_TEXT,
          bytes: LARGE_TEXT.length,
          limitBytes: LIMIT,
          language: 'typescript',
          stat,
        },
      },
      {
        channel: IPC.codeWorkspaceReadFile,
        args: { id: PORT_REPO.id, path, ifStat: stat },
        response: { kind: 'unchanged', text: '', bytes: 0, limitBytes: LIMIT, stat },
      },
    );
  }
  for (const path of DIFFS) {
    const stat = { size: 20, hash: `hash-${path}` };
    const head = {
      kind: 'found',
      text: 'export const c = 1;\n',
      bytes: 20,
      limitBytes: LIMIT,
      oid: 'h1',
    };
    const worktree = {
      kind: 'found',
      text: 'export const c = 2;\n',
      bytes: 20,
      limitBytes: LIMIT,
      stat,
    };
    out.push(
      {
        channel: IPC.codeWorkspaceReadDiff,
        args: { id: PORT_REPO.id, path, ifStat: stat, ifHeadOid: 'h1' },
        response: {
          path,
          language: 'typescript',
          head: { kind: 'unchanged', text: '', bytes: 0, limitBytes: LIMIT, oid: 'h1' },
          worktree: { kind: 'unchanged', text: '', bytes: 0, limitBytes: LIMIT, stat },
        },
      },
      {
        channel: IPC.codeWorkspaceReadDiff,
        args: { id: PORT_REPO.id, path },
        response: { path, language: 'typescript', head, worktree },
      },
    );
  }
  return out;
}

async function openGraphWithFiles(relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch']) {
  return openPortGraph(async (o) => relaunch(o), {
    chunks: singleRowChunks(manyRows(300)),
    results: { 'refs.list': refsAtTip(manyRowShas(0)) },
    control: fileControl(),
  });
}

test('ram: repo-file', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('repo-file'));
  const page = await openGraphWithFiles(relaunch);
  await page.clock.install();
  await page.locator('[data-testid="git-panel-tab-files"]').click();
  await tabWorld({
    world: 'repo-file',
    page,
    info,
    keep: 1,
    open: async (i) => {
      await treeRow(page, FILES[i]).dblclick();
      await expect(page.locator('[data-testid="repo-file-editor"] .view-lines')).toContainText(
        'value0',
      );
      await expect(page.locator('[data-testid="tab"]')).toHaveCount(i + 2);
    },
  });
});

test('ram: diff', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('diff'));
  const page = await openGraphWithFiles(relaunch);
  await page.clock.install();
  await page.locator('[data-testid="git-panel-tab-files"]').click();
  const s = await tabWorld({
    world: 'diff',
    page,
    info,
    keep: 1,
    snapshotAt: ['mode', 'closed', 'closed2', 'closed3'],
    open: async (i) => {
      await treeRow(page, DIFFS[i]).click({ button: 'right' });
      await page.locator('[data-testid="menu-item-open-changes"]').click();
      await expect(page.locator('[data-testid="repo-diff-editor"]')).toBeVisible();
      await expect(page.locator('[data-testid="tab"]')).toHaveCount(i + 2);
    },
  });
  if (SNAPSHOTS) {
    const d = await diffSnapshotFiles(
      s.snapshotFile('closed2'),
      s.snapshotFile('closed3'),
      /Detached/,
    );
    console.log(`HEAPDIFF ${APP} diff closed2-closed3 ${JSON.stringify(d)}`);
    const m = await diffSnapshotFiles(s.snapshotFile('mode'), s.snapshotFile('closed'), /Detached/);
    console.log(`HEAPDIFF ${APP} diff mode-closed ${JSON.stringify(m)}`);
  }
});

filesTest('ram: local-file', async ({ relaunch }, info) => {
  filesTest.skip(!worldEnabled('local-file'));
  const LOCAL = { path: '/tmp/outside/b.json', relPath: '', repoId: '' };
  const { window: page } = await relaunch({
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
          limitBytes: LIMIT,
          language: 'json',
        },
      },
    ],
  });
  await expect(page.locator('[data-testid="tab"][data-tab-kind="local-file"]')).toHaveCount(1);
  await page.clock.install();
  await runMemSteps({
    app: APP,
    world: 'local-file',
    page,
    info,
    steps: [
      {
        name: 'idle:6m',
        run: async () => {
          await page.clock.fastForward(6 * 60_000);
          await page.waitForTimeout(1500);
        },
      },
      { name: 'closed', run: () => closeAllTabs(page) },
    ],
  });
});

test('ram: terminal', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('terminal'));
  const { window: page } = await relaunch({
    control: [...PORT_CONTROL, { channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } }],
  });
  await page.clock.install();
  await repoRow(page, PORT_REPO.id).click();
  const s = await tabWorld({
    world: 'terminal',
    page,
    info,
    units: 5,
    keep: 1,
    snapshotAt: ['closed', 'closed2', 'closed3'],
    open: async () => {
      const id = await openTerminalTab(page);
      await feedTerminal(page, IPC.terminal, id, 10_000);
    },
  });
  if (SNAPSHOTS) {
    const d = await diffSnapshotFiles(s.snapshotFile('closed2'), s.snapshotFile('closed3'));
    console.log(`HEAPDIFF ${APP} terminal closed2-closed3 ${JSON.stringify(d)}`);
  }
});

/** Mounted ADE roots and cards: tells a duplicate mount from a retained one. */
async function logAdeDom(page: Page, label: string): Promise<void> {
  const c = await page.evaluate(() => ({
    views: document.querySelectorAll('[data-testid="ade-view"]').length,
    cards: document.querySelectorAll('[data-testid="ade-card"]').length,
    nodes: document.querySelectorAll('*').length,
  }));
  console.log(`ADEDOM ${label} ${JSON.stringify(c)}`);
}

for (const tasks of [20, 200]) {
  test(`ram: ade ${tasks} tasks`, async ({ relaunch }, info) => {
    test.skip(!worldEnabled('ade'));
    const board = adeBoard((b) => {
      const base = b.tasks.slice();
      let k = 0;
      while (b.tasks.length < tasks) {
        const src = base[k % base.length];
        const copy = structuredClone(src) as typeof src;
        const id = `${src.id}_x${k}`;
        copy.id = id;
        copy.title = `${String(src.title)} #${k}`;
        b.tasks.push(copy);
        k++;
      }
    });
    const first = board.tasks[0].id;
    const { window: page } = await relaunch({
      control: adeV2Control([{ channel: IPC.adeTaskBoard, response: board }]),
    });
    await page.clock.install();
    await expect(page.locator('[data-testid="ade-card"]').first()).toBeVisible({ timeout: 30_000 });
    const s = await runMemSteps({
      app: APP,
      world: `ade${tasks}`,
      page,
      info,
      steps: [
        { name: 'board', run: async () => {}, snapshot: true },
        { name: 'task', run: () => openTask(page, first), snapshot: true },
        {
          name: 'left',
          run: async () => {
            await modeTab(page, 'git').click();
            await page.waitForTimeout(1500);
            await logAdeDom(page, `ade${tasks} left`);
          },
          snapshot: 'keep',
        },
        {
          name: 'back',
          run: async () => {
            await modeTab(page, 'ade').click();
            await page.waitForTimeout(1500);
            await logAdeDom(page, `ade${tasks} back`);
          },
          snapshot: 'keep',
        },
        ...[2, 3].flatMap((n): MemStep[] => [
          {
            name: `left${n}`,
            run: async () => {
              await modeTab(page, 'git').click();
              await page.waitForTimeout(1500);
            },
          },
          {
            name: `back${n}`,
            run: async () => {
              await modeTab(page, 'ade').click();
              await page.waitForTimeout(1500);
            },
          },
        ]),
      ],
    });
    if (SNAPSHOTS) {
      const d = await diffSnapshotFiles(s.snapshotFile('left'), s.snapshotFile('back'));
      console.log(`HEAPDIFF ${APP} ade${tasks} left-back ${JSON.stringify(d)}`);
    }
  });
}

function memoryRow(i: number): Record<string, unknown> {
  return {
    id: `m${i}`,
    lineageId: `m${i}`,
    version: 1,
    fact: `Fact ${i}: the billing service number ${i} listens on port ${9000 + (i % 100)}.`,
    kind: 'fact',
    reason: `Reason ${i}`,
    keywords: ['billing', `k${i % 20}`],
    origin: i % 2 ? 'agent' : 'user',
    sourceTag: null,
    scope: 'personal',
    project: null,
    sessionId: null,
    status: 'current',
    historical: false,
    supersedesId: null,
    supersededById: null,
    createdAt: '2026-03-01T10:00:00.000Z',
    supersededAt: null,
    versions: 1,
  };
}

for (const count of [50, 500]) {
  test(`ram: memory ${count}`, async ({ relaunch }, info) => {
    test.skip(!worldEnabled('memory'));
    const all = Array.from({ length: count }, (_, i) => memoryRow(i));
    const { window: page } = await relaunch({
      control: [
        { channel: IPC.memoryRecent, response: all },
        { channel: IPC.memoryHistory, response: { memories: [all[0]], events: [] } },
        { channel: IPC.memoryContext, response: { memory: all[0], groups: [] } },
      ],
    });
    await page.clock.install();
    await runMemSteps({
      app: APP,
      world: `memory${count}`,
      page,
      info,
      steps: [
        { name: 'list', run: () => openMemory(page), snapshot: true },
        {
          name: 'detail',
          run: async () => {
            await page.locator('[data-testid="memory-row-m0"]').click();
            await page.waitForTimeout(1000);
          },
          snapshot: true,
        },
        {
          name: 'left',
          run: async () => {
            await modeTab(page, 'git').click();
            await page.waitForTimeout(1500);
          },
          snapshot: true,
        },
      ],
    });
  });
}

studioTest.describe('Studio worlds', () => {
  const studioWorld = (o: TabWorldOptions) =>
    tabWorld({ ...o, activateAll: false, idleSample: true });

  // P480 RAM probe, Studio worlds. Report-only: prints `RAM`/`HEAPSNAP` lines, asserts only on the UI
  // states that gate a checkpoint. `bun run perf:ram`; docs/DEV_ENVIRONMENT.md "Perf probes".

  studioTest('ram: grid', async ({ relaunch }, info) => {
    studioTest.skip(!worldEnabled('grid'));
    const { window: page } = await relaunch({ control: ITEMS_CONTROL, stream: ITEMS_PORT });
    await connectPostgres(page);
    await pinClock(page);
    await studioWorld({
      world: 'grid',
      page,
      info,
      open: duplicateOpen(
        page,
        async () => {
          await (await findRow(page, ORDER_ITEMS_PATH)).dblclick();
          await expect(page.locator('[data-testid="data-grid"]')).toBeVisible({ timeout: 15_000 });
        },
        'data-grid',
      ),
    });
  });

  studioTest('ram: console', async ({ relaunch }, info) => {
    studioTest.skip(!worldEnabled('console'));
    const { window: page } = await relaunch({ control: ITEMS_CONTROL, stream: ITEMS_PORT });
    await connectPostgres(page);
    await pinClock(page);
    await studioWorld({
      world: 'console',
      page,
      info,
      open: duplicateOpen(
        page,
        menuOpen(page, ORDER_ITEMS_PATH, 'open-console', 'console-view'),
        'console-view',
      ),
    });
  });

  studioTest('ram: definition', async ({ relaunch }, info) => {
    studioTest.skip(!worldEnabled('definition'));
    const { window: page } = await relaunch({ control: ITEMS_CONTROL, stream: ITEMS_PORT });
    await connectPostgres(page);
    await pinClock(page);
    await studioWorld({
      world: 'definition',
      page,
      info,
      open: duplicateOpen(
        page,
        menuOpen(page, ORDER_ITEMS_PATH, 'open-definition', 'definition-view'),
        'definition-view',
      ),
    });
  });

  studioTest('ram: document', async ({ relaunch }, info) => {
    studioTest.skip(!worldEnabled('document'));
    const conn = mongoConnectionSummary('conn-ram-doc', 'Mongo DB', 'green');
    const fixture = widgetsFixture(conn.id);
    const { window: page } = await relaunch({
      control: [{ channel: IPC.connectionsList, response: [conn] }, ...fixture.control],
      stream: fixture.port,
    });
    await connect(page);
    await expandRow(page, MONGO_DB);
    await pinClock(page);
    await studioWorld({
      world: 'document',
      page,
      info,
      open: duplicateOpen(
        page,
        async () => {
          await (await findRow(page, WIDGETS_PATH)).dblclick();
          await expect(page.locator('[data-testid="document-row"]').first()).toBeVisible({
            timeout: 15_000,
          });
        },
        'document-row',
      ),
    });
  });

  studioTest('ram: keyvalue', async ({ relaunch }, info) => {
    studioTest.skip(!worldEnabled('keyvalue'));
    const { window: page } = await relaunch({ control: redisControl, stream: redisPorts });
    await connect(page);
    await pinClock(page);
    const db0 = 'database:db0';
    const ns = `${db0}/namespace:user`;
    const ns1 = `${ns}/namespace:1`;
    const key = `${ns1}/key:user%3A1%3Abighash`;
    await studioWorld({
      world: 'keyvalue',
      page,
      info,
      open: duplicateOpen(
        page,
        async () => {
          await (await findRow(page, db0)).dblclick();
          const browse = page.locator('[data-testid="browse-view"]');
          await expect(browse).toBeVisible();
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
        'keyvalue-view',
        'keyvalue',
      ),
    });
  });

  studioTest('ram: stream', async ({ relaunch }, info) => {
    studioTest.skip(!worldEnabled('stream'));
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
    await studioWorld({
      world: 'stream',
      page,
      info,
      open: async (i) => {
        if (i === 0) {
          await (await findRow(page, 'queue:orders-queue')).dblclick();
          await poll();
        } else {
          // A duplicate has an empty buffer until polled.
          const tabs = page.locator('[data-testid="tab"]');
          await page
            .locator('[data-testid="tab"][data-tab-kind="stream"]')
            .first()
            .click({ button: 'right' });
          await page.click('[data-testid="menu-item-duplicate-tab"]');
          await expect(tabs).toHaveCount(i + 1);
          await poll();
        }
      },
    });
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

  studioTest('ram: discover', async ({ relaunch }, info) => {
    studioTest.skip(!worldEnabled('discover'));
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

  studioTest('ram: api', async ({ relaunch }, info) => {
    studioTest.skip(!worldEnabled('api'));
    const { window: page } = await relaunch({
      control: [
        { channel: IPC.tabsList, response: [grpcTab({ target: 'demo.example.com:443' })] },
        { channel: IPC.grpcHistoryList, args: { itemId: '', tabId: 'tab-grpc-1' }, response: [] },
        { channel: IPC.grpcDescribe, response: { services: [], mode: 'reflection', warnings: [] } },
      ],
    });
    await pinClock(page);
    const s = await studioWorld({
      world: 'api',
      page,
      info,
      snapshotAt: ['closed', 'closed2', 'closed3'],
      enter: async () => {
        await modeTab(page, 'api').click();
        await expect(page.locator('[data-testid="grpc-request-view"]')).toBeVisible();
      },
      open: async (i) => {
        if (i === 0) {
          if ((await page.locator('[data-testid="tab"]').count()) === 0)
            await page.click('[data-testid="new-grpc-request-start"]');
        } else {
          await page.click('[data-testid="new-request"]');
          await expect(
            page.locator('[data-testid="http-request-view"]:visible').first(),
          ).toBeVisible();
        }
      },
    });
    if (SNAPSHOTS) {
      const d = await diffSnapshotFiles(s.snapshotFile('closed2'), s.snapshotFile('closed3'));
      console.log(`HEAPDIFF ${APP} api closed2-closed3 ${JSON.stringify(d)}`);
    }
  });

  studioTest('ram: terminal automations', async ({ relaunch }, info) => {
    studioTest.skip(!worldEnabled('terminal-automations'));
    const { window: page } = await relaunch({
      control: [{ channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } }],
    });
    await pinClock(page);
    await studioWorld({
      world: 'terminal-automations',
      page,
      info,
      units: 5,
      enter: async () => {
        await modeTab(page, 'automations').click();
      },
      open: async (i) => {
        if (i === 0) {
          const id = await newTerminal(page);
          await feedTerminal(page, IPC.terminal, id, 10_000);
        } else {
          const id = await newTerminal(page);
          await feedTerminal(page, IPC.terminal, id, 10_000);
        }
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

  studioTest('ram: docker', async ({ relaunch }, info) => {
    studioTest.skip(!worldEnabled('docker'));
    const list = [
      container('c-solo', 'solo', 'running'),
      ...Array.from({ length: NCONTAINERS }, (_, i) =>
        container(
          `c-${String(i).padStart(4, '0')}`,
          `svc-${String(i).padStart(4, '0')}`,
          'running',
        ),
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

  studioTest('ram: kubernetes', async ({ relaunch }, info) => {
    studioTest.skip(!worldEnabled('kubernetes'));
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

  studioTest('ram: documents-list', async ({ relaunch }, info) => {
    studioTest.skip(!worldEnabled('documents-list'));
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

  studioTest('ram: i18n catalog parse', async ({ relaunch }, info) => {
    studioTest.skip(!worldEnabled('i18n'));
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
});
