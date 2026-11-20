import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import type { Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import type { KibanaDiscoverRequest } from '@shared/domain/kibana';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
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
import {
  adeBoard,
  adeV2Control,
  emitLog,
  emitOpenSession,
  emitSessions,
  openTask,
  openTaskFile,
  taskFilesControl,
} from '../ui/support/adeV2';
import { contract } from '../ui/support/contract';
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
import { restoredRepoTabs } from '../ui/support/repoFiles';
import { repoTab } from '../ui/support/repoRows';
import type { ControlSnapshot } from '../ui/support/types';
import { CONTROL, FIXTURE, makeDocs, openWidgets, readSnapshot } from './studio/documentsFixture';
import {
  AGENTS_WORLD,
  APP,
  agentFrames,
  agentsBoard,
  agentsSessions,
  agentsSessionsStopped,
  connect,
  duplicateOpen,
  feedFrames,
  feedTerminal,
  graphChunks,
  lastStreamId,
  menuOpen,
  modeTab,
  NOW,
  newTerminal,
  openTerminalTab,
  pinClock,
  pssAfter,
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
  const control: ControlSnapshot[] = [{ channel: IPC.codeWorkspaceListRepos, response: repos }];
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

// No project tree opens a file or diff tab any more: the world's tabs restore, `open(i)` activates
// the i-th, so the heap cost measured is the mount, not the open.
function fileControl(kind: 'repo-file' | 'repo-diff'): ControlSnapshot[] {
  const out: ControlSnapshot[] = [
    { channel: IPC.codeWorkspaceListRepos, response: [PORT_REPO] },
    restoredRepoTabs(
      PORT_REPO.id,
      (kind === 'repo-file' ? FILES : DIFFS).map((path) => ({ kind, path })),
    ),
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

async function openGraphWithFiles(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  kind: 'repo-file' | 'repo-diff',
) {
  return openPortGraph(async (o) => relaunch(o), {
    chunks: singleRowChunks(manyRows(300)),
    results: { 'refs.list': refsAtTip(manyRowShas(0)) },
    control: fileControl(kind),
  });
}

test('ram: repo-file', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('repo-file'));
  const page = await openGraphWithFiles(relaunch, 'repo-file');
  await page.clock.install();
  await tabWorld({
    world: 'repo-file',
    page,
    info,
    keep: 1,
    open: async (i) => {
      await repoTab(page, 'repo-file').nth(i).click();
      await expect(page.locator('[data-testid="repo-file-editor"] .view-lines')).toContainText(
        'value0',
      );
      await expect(page.locator('[data-testid="tab"]')).toHaveCount(FILES.length + 1);
    },
  });
});

test('ram: diff', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('diff'));
  const page = await openGraphWithFiles(relaunch, 'repo-diff');
  await page.clock.install();
  const s = await tabWorld({
    world: 'diff',
    page,
    info,
    keep: 1,
    snapshotAt: ['mode', 'closed', 'closed2', 'closed3'],
    open: async (i) => {
      await repoTab(page, 'repo-diff').nth(i).click();
      await expect(page.locator('[data-testid="repo-diff-editor"]')).toBeVisible();
      await expect(page.locator('[data-testid="tab"]')).toHaveCount(DIFFS.length + 1);
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

/** F4: where ADE DOM, xterm, style and font cost sits. Report-only; one JSON line per checkpoint. */
async function logAdeBreakdown(page: Page, label: string): Promise<void> {
  const c = await page.evaluate(() => {
    const count = (sel: string) =>
      Array.from(document.querySelectorAll(sel)).reduce(
        (n, e) => n + e.querySelectorAll('*').length + 1,
        0,
      );
    const regions: Record<string, number> = {};
    for (const id of ['ade-shell-bar', 'ade-plan', 'ade-panel', 'ade-tui-pane']) {
      regions[id] = count(`[data-testid="${id}"]`);
    }
    const shell = count('[data-testid="ade-shell"]');
    const styles = Array.from(document.querySelectorAll('style'));
    return {
      total: document.querySelectorAll('*').length,
      shell,
      regions,
      xtermHosts: document.querySelectorAll('.xterm').length,
      xtermRows: document.querySelectorAll('.xterm-rows > div').length,
      canvases: document.querySelectorAll('canvas').length,
      styleTags: styles.length,
      styleChars: styles.reduce((n, e) => n + (e.textContent?.length ?? 0), 0),
      fontFaces: document.fonts.size,
    };
  });
  let counters: unknown = null;
  try {
    const cdp = await page.context().newCDPSession(page);
    counters = await cdp.send('Memory.getDOMCounters' as never);
    await cdp.detach();
  } catch {
    // WebKit has no CDP.
  }
  console.log(`ADEDOM ${label} ${JSON.stringify({ ...c, counters })}`);
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
    const pss = pssAfter(page, `ade${tasks}`);
    const hopTo = (name: string, mode: string, label: string, snapshot?: 'keep'): MemStep => ({
      name,
      run: async () => {
        await modeTab(page, mode).click();
        await page.waitForTimeout(1500);
        if (label) await logAdeDom(page, `ade${tasks} ${label}`);
      },
      snapshot,
      after: pss(name),
    });
    const s = await runMemSteps({
      app: APP,
      world: `ade${tasks}`,
      page,
      info,
      steps: [
        { name: 'board', run: async () => {}, snapshot: true, after: pss('board') },
        { name: 'task', run: () => openTask(page, first), snapshot: true, after: pss('task') },
        hopTo('left', 'git', 'left', 'keep'),
        hopTo('back', 'ade', 'back', 'keep'),
        ...[2, 3].flatMap((n): MemStep[] => [
          hopTo(`left${n}`, 'git', ''),
          hopTo(`back${n}`, 'ade', ''),
        ]),
      ],
    });
    if (SNAPSHOTS) {
      const d = await diffSnapshotFiles(s.snapshotFile('left'), s.snapshotFile('back'));
      console.log(`HEAPDIFF ${APP} ade${tasks} left-back ${JSON.stringify(d)}`);
    }
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

/** Selects task `id`'s card, then its panel tab. A selected card toggles closed on a second click, and
 *  a running session sends the panel to Sessions: both seen after `switch10`. */
async function openPanel(page: Page, id: string, tab: string): Promise<void> {
  const panel = page.locator('[data-testid="ade-panel"]');
  const head = page.locator(
    `[data-testid="ade-card"][data-task-id="${id}"] [data-testid="ade-card-head"]`,
  );
  await head.click();
  await panel.waitFor({ timeout: 3000 }).catch(() => head.click());
  await page.locator(`[data-testid="ade-panel-tab-${tab}"]`).click();
}

// P661: agents module only. 10 active tasks, 200 archived, 300 stopped session rows, 4 running TUI
// sessions fed Claude-like ED2 redraws; 2 panes opened, 10 task switches, 50 board and sessions
// pushes. Report-only: RAM, PSS and HEAPSNAP lines. `KIRA_RAM_WORLD=ade-agents`,
// `KIRA_RAM_SNAPSHOTS=1` for the Terminal/BufferLine/Query constructor rows.
test('ram: ade-agents', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('ade-agents'));
  const board = agentsBoard();
  const ids = board.tasks.map((t) => t.id);
  const { window: page, control: mock } = await relaunch({
    control: adeV2Control([
      { channel: IPC.adeTaskBoard, response: board },
      { channel: IPC.adeTaskSessions, response: agentsSessions(ids) },
      { channel: IPC.mobileLaunchOpened, response: null },
      ...taskFilesControl(),
    ]),
  });
  await page.clock.install();
  await expect(page.locator('[data-testid="ade-card"]').first()).toBeVisible({ timeout: 30_000 });
  const pss = pssAfter(page, 'ade-agents');
  const pushes = (n: number): MemStep => ({
    name: `push${n}`,
    run: async () => {
      for (let i = 0; i < 10; i++) {
        await emitWailsEvent(page, IPC.adeTaskBoardChanged, null);
        await emitSessions(page);
        await page.waitForTimeout(150);
      }
    },
    after: pss(`push${n}`),
  });
  await runMemSteps({
    app: APP,
    world: 'ade-agents',
    page,
    info,
    steps: [
      {
        name: 'board',
        run: () => logAdeBreakdown(page, 'agents board'),
        snapshot: true,
        after: pss('board'),
      },
      {
        name: 'launch4',
        run: async () => {
          for (let i = 0; i < AGENTS_WORLD.running; i++) {
            await emitWailsEvent(page, IPC.mobileOpenLaunch, {
              launch: {
                terminalId: `term-ag${i}`,
                sessionId: `ag${i}`,
                command: 'claude',
                cwd: `/tmp/ag${i}`,
              },
              taskId: ids[i],
              branchId: '',
            });
            await page.waitForTimeout(300);
            await feedFrames(
              page,
              IPC.terminal,
              `term-ag${i}`,
              agentFrames(i, AGENTS_WORLD.bytesPerSession),
            );
          }
          await page.waitForTimeout(1500);
          await logAdeBreakdown(page, 'agents launch4');
        },
        snapshot: true,
        after: pss('launch4'),
      },
      {
        name: 'panes2',
        run: async () => {
          for (const i of [0, 1]) {
            await emitOpenSession(page, { taskId: ids[i], branchId: '', sessionId: `ag${i}` });
            await expect(page.locator('[data-testid="ade-tui-pane"]')).toBeVisible();
            await page.waitForTimeout(500);
          }
          await logAdeBreakdown(page, 'agents panes2');
        },
        after: pss('panes2'),
      },
      {
        name: 'switch10',
        run: async () => {
          const shown = await page
            .locator('[data-testid="ade-card"]')
            .evaluateAll((els) => els.map((e) => e.getAttribute('data-task-id') ?? ''));
          for (let i = 0; i < 10; i++) {
            await page
              .locator(
                `[data-testid="ade-card"][data-task-id="${shown[i % shown.length]}"] [data-testid="ade-card-head"]`,
              )
              .click();
            await expect(page.locator('[data-testid="ade-panel"]')).toBeVisible();
            await page.waitForTimeout(200);
          }
          await logAdeBreakdown(page, 'agents switch10');
        },
        snapshot: true,
        after: pss('switch10'),
      },
      ...[10, 20, 30, 40, 50].map(pushes),
      {
        name: 'final',
        run: async () => {
          await page.waitForTimeout(5000);
          await logAdeBreakdown(page, 'agents final');
        },
        snapshot: true,
        after: pss('final'),
      },
      {
        // An agent approval pending: whatever the surface loads for it (Monaco, P661 F5) shows here.
        name: 'approval',
        run: async () => {
          const [item] = contract<ApprovalItem[]>(
            'ade-orchestrator',
            'ApprovalsService.List#taskCreate',
          );
          await pushApprovals(page, [liveItem(item, 1800)]);
          await expect(page.locator('[data-testid="approvals-dialog"]')).toBeVisible();
          await page.waitForTimeout(3000);
        },
        snapshot: true,
        after: pss('approval'),
      },
      {
        // P693 E3: 3 MB of run log on the push channel, the log visible (S3).
        name: 'log3m',
        run: async () => {
          await pushApprovals(page, []);
          await expect(page.locator('[data-testid="approvals-dialog"]')).toBeHidden();
          await openPanel(page, 'T_bill', 'main');
          await page
            .locator('[data-testid="ade-step-block"][data-step-id="impl"]')
            .locator('[data-testid="ade-run-line"][data-branch-id="b_bill"]')
            .locator('[data-testid="ade-run-log-toggle"]')
            .click();
          await expect(page.locator('[data-testid="ade-run-log"]')).toBeVisible();
          const PUSHES = 30;
          const PER_PUSH = 100;
          const text = (i: number) => `log line ${i} `.padEnd(1000, 'x');
          for (let p = 0; p < PUSHES; p++) {
            await emitLog(
              page,
              'run',
              'r_T_bill_impl_b_bill',
              Array.from({ length: PER_PUSH }, (_, k) => ({
                seq: 10_000 + p * PER_PUSH + k,
                at: 1_790_067_600_000 + p * PER_PUSH + k,
                stream: 'event',
                text: text(p * PER_PUSH + k),
              })),
            );
            await page.waitForTimeout(50);
          }
          await page.waitForTimeout(1500);
        },
        snapshot: true,
        after: pss('log3m'),
      },
      {
        // P693 E3 (P691): task Files tab and one file; Monaco shows here (S7).
        name: 'files1',
        run: async () => {
          await openPanel(page, 'T_cart', 'files');
          await openTaskFile(page, 'README.md');
          await page.waitForTimeout(1500);
        },
        snapshot: true,
        after: pss('files1'),
      },
      {
        // P693 E3: the four running sessions stop; adeTerminals must reap their xterms (S4). The exit
        // events matter: reap waits for local `exited`, and a probe without them kept all four.
        name: 'stopped4',
        run: async () => {
          // Real order: a stop ends the pty first, so each terminal's exit event precedes the push.
          for (let i = 0; i < AGENTS_WORLD.running; i++) {
            await emitWailsEvent(page, IPC.terminal, {
              terminalId: `term-ag${i}`,
              data: '',
              exited: true,
              exitCode: 0,
            });
          }
          mock.respond(IPC.adeTaskSessions, agentsSessionsStopped(ids));
          await emitSessions(page);
          await page.waitForTimeout(2000);
        },
        snapshot: true,
        after: pss('stopped4'),
      },
      {
        name: 'left',
        run: async () => {
          await modeTab(page, 'git').click();
          await page.waitForTimeout(1500);
        },
        snapshot: 'keep',
        after: pss('left'),
      },
      {
        name: 'back',
        run: async () => {
          await modeTab(page, 'ade').click();
          await page.waitForTimeout(1500);
        },
        snapshot: 'keep',
        after: pss('back'),
      },
    ],
  });
});

// P693 E1: same board and sessions list as ade-agents (10 live, 200 archived, 300 stopped rows) but
// no launched session: the idle floor of the module. `idle60` is a 60 s wait; `left`/`back` leave
// and re-enter the ade mode.
test('ram: ade-idle', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('ade-idle'));
  const board = agentsBoard();
  const { window: page } = await relaunch({
    control: adeV2Control([
      { channel: IPC.adeTaskBoard, response: board },
      { channel: IPC.adeTaskSessions, response: { sessions: [] } },
      { channel: IPC.mobileLaunchOpened, response: null },
    ]),
  });
  await page.clock.install();
  await expect(page.locator('[data-testid="ade-card"]').first()).toBeVisible({ timeout: 30_000 });
  const pss = pssAfter(page, 'ade-idle');
  const hop = (name: string, mode: string): MemStep => ({
    name,
    run: async () => {
      await modeTab(page, mode).click();
      await page.waitForTimeout(1500);
    },
    snapshot: 'keep',
    after: pss(name),
  });
  await runMemSteps({
    app: APP,
    world: 'ade-idle',
    page,
    info,
    steps: [
      { name: 'board', run: async () => {}, snapshot: true, after: pss('board') },
      {
        name: 'idle60',
        run: () => page.waitForTimeout(60_000),
        snapshot: true,
        after: pss('idle60'),
      },
      hop('left', 'git'),
      hop('back', 'ade'),
    ],
  });
});
