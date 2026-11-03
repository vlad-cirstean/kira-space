import {
  closeAllTabs,
  type HeapTab,
  isReadChannel,
  runHeapScenario,
} from '@workbench/testing/ui/heapProbe';
import { expect, filesTest, type KiraApp, test } from '../ui/fixtures';
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
