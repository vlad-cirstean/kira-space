import type { Page } from '@playwright/test';
import { closeAllTabs } from '@workbench/testing/ui/heapProbe';
import { diffSnapshotFiles } from '@workbench/testing/ui/heapSnapshot';
import { type MemStep, runMemSteps, SNAPSHOTS } from '@workbench/testing/ui/memProbe';
import { expect, test } from '../ui/fixtures';
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
import {
  APP,
  feedTerminal,
  graphChunks,
  modeTab,
  openTerminalTab,
  tabWorld,
  worldEnabled,
} from './support/ramWorlds';

// P480 RAM probe, Space worlds. Report-only: prints `RAM`/`HEAPSNAP` lines, asserts only on the UI
// states that gate a checkpoint. `bun run perf:ram:space`; docs/DEV_ENVIRONMENT.md "Perf probes".

const repoRow = (page: import('@playwright/test').Page, id: string) =>
  page.locator(`[data-testid="repo-row"][data-repo-id="${id}"]`);

test('ram: idle and modes', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('idle'));
  const { window: page } = await relaunch({ control: PORT_CONTROL });
  await page.clock.install();
  const hop = (m: 'ade' | 'automations' | 'memory'): MemStep[] => [
    {
      name: `mode:${m}`,
      run: async () => {
        await modeTab(page, m).click();
        await page.waitForTimeout(1500);
      },
    },
    {
      name: `back:${m}`,
      run: async () => {
        await modeTab(page, 'git').click();
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
      ...hop('ade'),
      ...hop('automations'),
      ...hop('memory'),
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

test('ram: local-file', async ({ relaunch }, info) => {
  test.skip(!worldEnabled('local-file'));
  const LOCAL = { path: '/tmp/outside/b.json', relPath: '', repoId: '' };
  const { window: page } = await relaunch({
    control: [
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
        name: 'mode:automations',
        run: async () => {
          await modeTab(page, 'automations').click();
          await page.waitForTimeout(1500);
        },
      },
      {
        name: 'back:git',
        run: async () => {
          await modeTab(page, 'git').click();
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
