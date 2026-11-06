import { DATA_OP } from '@shared/protocol/dataOps';
import {
  controlSnapshots as redisControl,
  portSnapshots as redisPorts,
} from '../ipc/redis/redis.fixture';
import { controlSnapshots as sqsControl, portSnapshots as sqsPorts } from '../ipc/sqs/sqs.fixture';
import { expect, studioTest, test } from './fixtures';
import { grpcTab, modeTab, openHttpModeAndNewRequest } from './studio/support/apiMode';
import {
  connectAndExpand,
  connectionCreateArgs,
  openConsoleFromMenu,
} from './studio/support/connect';
import { editorText } from './studio/support/editorText';
import {
  mongoConnectionSummary,
  WIDGETS_BODIES,
  WIDGETS_IDS,
  WIDGETS_PATH,
  widgetsFixture,
} from './studio/support/mongoFixture';
import {
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './studio/support/postgresFixture';
import { openRedisBrowse, REDIS_DB0 } from './studio/support/redisFixture';
import {
  connectionRow,
  expandRow,
  findRow,
  openBrowseLeaf,
  openRowMenu,
} from './studio/support/tree';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import type { ControlSnapshot } from './support/types';

// P264: one key set for every list. The Operations dock is the Space list bound through the shared
// useListNavigation today; its rows are virtual, so Home/End also prove focus follows the scroll.

const NOW = '2026-01-01T00:00:00.000Z';

function opRecord(id: string): Record<string, unknown> {
  return {
    id,
    startedAt: NOW,
    durationMs: 12,
    kind: 'fetch',
    status: 'ok',
    command: `git fetch ${id}`,
    error: null,
    category: 'git',
    module: 'git',
    trigger: 'user',
    subject: '',
    repoRoot: '/work/alpha',
    repoName: 'alpha',
    source: 'Kira Space',
    taskId: '',
    parentId: '',
    background: false,
    repeat: 1,
    steps: [],
    stepsDropped: 0,
    cancellable: false,
  };
}

const IDS = ['op-a', 'op-b', 'op-c'];
const CONTROL: ControlSnapshot[] = [{ channel: IPC.opsRecent, response: IDS.map(opRecord) }];

test('operations list — roving tab stop, arrows, Home/End, Enter, Shift+F10 and focus return', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await page.click('[data-testid="toggle-operations-panel"]');
  const rows = page.locator('[data-testid="op-row"]');
  await expect(rows).toHaveCount(IDS.length);
  const ids = await rows.evaluateAll((els) => els.map((el) => el.getAttribute('data-op-id') ?? ''));

  await expect(page.locator('[data-testid="op-row"][tabindex="0"]')).toHaveCount(1);
  const row = (i: number) => page.locator(`[data-testid="op-row"][data-op-id="${ids[i]}"]`);

  await row(0).focus();
  await page.keyboard.press('ArrowDown');
  await expect(row(1)).toBeFocused();
  await expect(row(1)).toHaveAttribute('tabindex', '0');
  await expect(row(0)).toHaveAttribute('tabindex', '-1');

  await page.keyboard.press('End');
  await expect(row(2)).toBeFocused();
  await page.keyboard.press('ArrowDown');
  await expect(row(2)).toBeFocused();
  await page.keyboard.press('Home');
  await expect(row(0)).toBeFocused();
  // Up from the first row returns to the filter field.
  await page.keyboard.press('ArrowUp');
  await expect(page.locator('[data-testid="ops-filter"]')).toBeFocused();
  await row(0).focus();

  // Enter opens the row, same as a click: the detail appears under it, and Enter closes it.
  const detail = page.locator('[data-testid="op-detail"]');
  await page.keyboard.press('Enter');
  await expect(detail).toContainText(`git fetch ${ids[0]}`);
  await page.keyboard.press('Enter');
  await expect(detail).toHaveCount(0);

  // Right opens the detail, Left closes it; focus stays on the row.
  await page.keyboard.press('ArrowRight');
  await expect(detail).toHaveCount(1);
  await page.keyboard.press('ArrowLeft');
  await expect(detail).toHaveCount(0);
  await expect(row(0)).toBeFocused();

  await page.keyboard.press('Shift+F10');
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator('[data-testid="context-menu"]')).toHaveCount(0);
  await expect(row(0)).toBeFocused();
});

test('operations list — keys typed in the filter field do not move the list', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await page.click('[data-testid="toggle-operations-panel"]');
  const rows = page.locator('[data-testid="op-row"]');
  await expect(rows).toHaveCount(IDS.length);
  const filter = page.locator('[data-testid="ops-filter"]');
  await filter.focus();
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('End');
  await expect(filter).toBeFocused();
});

const REPO = {
  id: 'repo-1',
  name: 'demo-repo',
  root: '/tmp/demo-repo',
  repoId: '/tmp/demo-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};
const REPO_2 = {
  ...REPO,
  id: 'repo-2',
  name: 'other-repo',
  root: '/tmp/other-repo',
  repoId: '/tmp/other-repo',
  sortOrder: 2,
};
const FILES = { paths: ['a.ts', 'dir/b.ts', 'dir/c.ts'], status: {}, truncated: false };

function readFile(path: string): ControlSnapshot {
  const text = `export const x = '${path}';\n`;
  return {
    channel: IPC.codeWorkspaceReadFile,
    args: { id: REPO.id, path },
    response: {
      kind: 'found',
      text,
      bytes: text.length,
      limitBytes: 8 * 1024 * 1024,
      language: 'typescript',
    },
  };
}

const REPO_CONTROL: ControlSnapshot[] = [
  { channel: IPC.codeWorkspaceListRepos, response: [REPO, REPO_2] },
  { channel: IPC.codeWorkspaceListFiles, args: { id: REPO.id }, response: FILES },
  readFile('a.ts'),
  readFile('dir/b.ts'),
  readFile('dir/c.ts'),
];

test('repo tree — arrows, Home/End, Left/Right parent and child, Shift+F10 menu and focus return', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: REPO_CONTROL });
  await page.locator('[data-testid="repo-row"][data-repo-id="repo-1"]').click();
  await page.locator('[data-testid="git-panel-tab-files"]').click();
  const row = (path: string) => page.locator(`[data-testid="repo-tree-row"][data-path="${path}"]`);
  await expect(row('a.ts')).toBeVisible();

  // Rows: dir, a.ts (folders first). Selecting by click makes the row the roving stop.
  await row('dir').click();
  await expect(row('dir')).toBeFocused();
  await page.keyboard.press('ArrowRight');
  await expect(row('dir/b.ts')).toBeVisible();
  await expect(row('dir')).toBeFocused();
  await page.keyboard.press('ArrowDown');
  await expect(row('dir/b.ts')).toBeFocused();
  await page.keyboard.press('End');
  await expect(row('a.ts')).toBeFocused();
  await page.keyboard.press('Home');
  await expect(row('dir')).toBeFocused();

  // Right on an expanded folder enters its first child, Left on a child goes back to the parent.
  await page.keyboard.press('ArrowRight');
  await expect(row('dir/b.ts')).toBeFocused();
  await page.keyboard.press('ArrowLeft');
  await expect(row('dir')).toBeFocused();
  // Left on an expanded folder collapses it.
  await page.keyboard.press('ArrowLeft');
  await expect(row('dir/b.ts')).toHaveCount(0);

  await page.keyboard.press('Shift+F10');
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator('[data-testid="context-menu"]')).toHaveCount(0);
  await expect(row('dir')).toBeFocused();

  // Right-click selects and focuses the row it opens the menu on.
  await row('a.ts').click({ button: 'right' });
  await expect(row('a.ts')).toHaveAttribute('aria-selected', 'true');
  await page.keyboard.press('Escape');
  await expect(row('a.ts')).toBeFocused();
});

test('repo tree — Enter on a file opens a permanent tab', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: REPO_CONTROL });
  await page.locator('[data-testid="repo-row"][data-repo-id="repo-1"]').click();
  await page.locator('[data-testid="git-panel-tab-files"]').click();
  const a = page.locator('[data-testid="repo-tree-row"][data-path="a.ts"]');
  await a.click();
  await a.press('Enter');
  const fileTab = page.locator(
    '[data-testid="tab-strip-wrapper"] [data-testid="tab"][data-tab-kind="repo-file"]',
  );
  await expect(fileTab).toHaveCount(1);
  await expect(fileTab).toHaveAttribute('data-preview', 'false');
});

test('git panel repo list — arrows, Home/End, Enter, Shift+F10 and inline rename with Esc', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: REPO_CONTROL });
  const row = (id: string) => page.locator(`[data-testid="repo-row"][data-repo-id="${id}"]`);
  await expect(row('repo-1')).toBeVisible();
  await expect(page.locator('[data-testid="repo-row"][tabindex="0"]')).toHaveCount(1);

  await row('repo-1').focus();
  await page.keyboard.press('ArrowDown');
  await expect(row('repo-2')).toBeFocused();
  await page.keyboard.press('ArrowDown');
  await expect(row('repo-2')).toBeFocused();
  await page.keyboard.press('Home');
  await expect(row('repo-1')).toBeFocused();
  await page.keyboard.press('End');
  await expect(row('repo-2')).toBeFocused();

  await page.keyboard.press('Shift+F10');
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await page.locator('[data-testid="menu-item-rename"]').click();
  const input = page.locator('[data-testid="repo-rename-input"]');
  await expect(input).toBeFocused();
  await input.fill('renamed');
  await page.keyboard.press('Escape');
  await expect(input).toHaveCount(0);
  await expect(row('repo-2')).toContainText('other-repo');
});

test('repos dialog — arrows move the selected repository', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: REPO_CONTROL });
  await page.locator('[data-testid="manage-repos"]').click();
  const rows = page.locator('[data-testid="repos-dialog-repo"]');
  await expect(rows).toHaveCount(2);
  await expect(rows.first()).toHaveAttribute('aria-selected', 'true');
  await rows.first().focus();
  await page.keyboard.press('ArrowDown');
  await expect(rows.nth(1)).toBeFocused();
  await expect(rows.nth(1)).toHaveAttribute('aria-selected', 'true');
  await expect(rows.first()).toHaveAttribute('aria-selected', 'false');
  await page.keyboard.press('Home');
  await expect(rows.first()).toHaveAttribute('aria-selected', 'true');
});

test('repo search results — arrows, Enter on a file header and Home/End', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [
      ...REPO_CONTROL,
      { channel: IPC.codeWorkspaceStartSearch, response: { searchId: 'search-1' } },
    ],
  });
  await page.locator('[data-testid="repo-row"][data-repo-id="repo-1"]').click();
  await page.locator('[data-testid="git-panel-tab-files"]').click();
  await page.locator('[data-testid="repo-view-search"]').click();
  const query = page.locator('[data-testid="repo-search-query"]');
  await query.fill('const');
  await query.press('Enter');
  const match = (line: number) => ({
    line,
    column: 8,
    endColumn: 13,
    preview: 'export const a = 1;',
    previewMatchStart: 7,
    previewMatchEnd: 12,
    truncatedStart: false,
    truncatedEnd: false,
  });
  await emitWailsEvent(page, IPC.codeSearch, {
    searchId: 'search-1',
    seq: 0,
    files: [
      { path: 'a.ts', matches: [match(1)], truncated: false },
      { path: 'dir/b.ts', matches: [match(2)], truncated: false },
    ],
    done: true,
    stats: { filesScanned: 2, filesMatched: 2, filesSkipped: 0, matches: 2, truncated: false },
  });
  const fileRows = page.locator('[data-testid="repo-search-file-row"]');
  await expect(fileRows).toHaveCount(2);
  await expect(page.locator('[data-testid="repo-search-match-row"]')).toHaveCount(2);

  await fileRows.first().focus();
  await page.keyboard.press('ArrowDown');
  await expect(page.locator('[data-testid="repo-search-match-row"]').first()).toBeFocused();
  await page.keyboard.press('End');
  await expect(page.locator('[data-testid="repo-search-match-row"]').last()).toBeFocused();
  await page.keyboard.press('Home');
  await expect(fileRows.first()).toBeFocused();
  // Enter on a file header collapses its matches.
  await page.keyboard.press('Enter');
  await expect(page.locator('[data-testid="repo-search-match-row"]')).toHaveCount(1);
});

function memoryRecord(id: string, fact: string): Record<string, unknown> {
  return {
    id,
    lineageId: id,
    version: 1,
    fact,
    kind: 'fact',
    reason: 'Because.',
    keywords: [],
    origin: 'user',
    sourceTag: null,
    scope: 'personal',
    project: null,
    sessionId: null,
    status: 'current',
    historical: false,
    supersedesId: null,
    supersededById: null,
    createdAt: NOW,
    supersededAt: null,
    versions: 1,
  };
}

test('memory list — focus on mount, arrows, Home/End, Enter selects and a typed key lands in search', async ({
  relaunch,
}) => {
  const first = memoryRecord('ma', 'First fact.');
  const second = memoryRecord('mb', 'Second fact.');
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.memoryRecent, response: [first, second] },
      { channel: IPC.memoryHistory, response: { memories: [second], events: [] } },
      { channel: IPC.memoryContext, response: { memory: second, groups: [] } },
    ],
  });
  await page.locator('[data-testid="mode-tab"][data-mode="memory"]').click();
  const search = page.locator('[data-testid="memory-search"]');
  await expect(search).toBeFocused();

  const row = (id: string) => page.locator(`[data-testid="memory-row-${id}"]`);
  await expect(page.locator('[data-testid^="memory-row-"][tabindex="0"]')).toHaveCount(1);
  await row('ma').focus();
  await page.keyboard.press('ArrowDown');
  await expect(row('mb')).toBeFocused();
  await page.keyboard.press('ArrowDown');
  await expect(row('mb')).toBeFocused();
  await page.keyboard.press('Home');
  await expect(row('ma')).toBeFocused();
  await page.keyboard.press('End');
  await expect(row('mb')).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(page.locator('[data-testid="memory-detail-fact"]')).toContainText('Second fact.');

  await row('ma').focus();
  await page.keyboard.press('z');
  await expect(search).toBeFocused();
  await expect(search).toHaveValue('z');
});

test('memory list — 200 memories virtualize, End and Home reach the ends', async ({ relaunch }) => {
  const records = Array.from({ length: 200 }, (_, i) => memoryRecord(`m${i}`, `Fact ${i}.`));
  const { window: page } = await relaunch({
    control: [{ channel: IPC.memoryRecent, response: records }],
  });
  await page.locator('[data-testid="mode-tab"][data-mode="memory"]').click();

  const rows = page.locator('[data-testid^="memory-row-m"]');
  await expect(rows.first()).toBeVisible();
  expect(await rows.count()).toBeLessThan(60);
  await rows.first().focus();
  await page.keyboard.press('End');
  await expect(page.locator('[data-testid="memory-row-m199"]')).toBeFocused();
  await page.keyboard.press('Home');
  await expect(page.locator('[data-testid="memory-row-m0"]')).toBeFocused();
});

studioTest.describe('databases module', () => {
  // P264: one key set for every list. The project tree is the Studio list bound through the shared
  // useListNavigation today (Left/Right and expansion are covered in tree.spec.ts); rows here are
  // disconnected connections so no tree IPC is needed.

  const CONNS = [
    postgresConnectionSummary('conn-k1', 'Alpha DB', 'blue'),
    postgresConnectionSummary('conn-k2', 'Bravo DB', 'cyan'),
    postgresConnectionSummary('conn-k3', 'Charlie DB', 'green'),
  ];
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.connectionsList, response: CONNS }];

  studioTest(
    'project tree — arrows, Home/End, right-click select, Shift+F10 and focus return',
    async ({ relaunch }) => {
      const { window: page } = await relaunch({ control: CONTROL });
      const rows = page.locator('[data-testid="tree-row"][data-kind="connection"]');
      await expect(rows).toHaveCount(CONNS.length);
      const row = (i: number) => rows.filter({ hasText: CONNS[i].name });

      await row(1).focus();
      await expect(row(1)).toHaveAttribute('aria-selected', 'true');
      await page.keyboard.press('Shift+F10');
      await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
      await page.keyboard.press('Escape');
      await expect(page.locator('[data-testid="context-menu"]')).toHaveCount(0);

      await row(0).focus();
      await page.keyboard.press('ArrowDown');
      await expect(row(1)).toBeFocused();
      await expect(row(1)).toHaveAttribute('aria-selected', 'true');
      await expect(page.locator('[data-testid="tree-row"][tabindex="0"]')).toHaveCount(1);

      await page.keyboard.press('End');
      await expect(row(2)).toBeFocused();
      await page.keyboard.press('ArrowDown');
      await expect(row(2)).toBeFocused();
      await page.keyboard.press('Home');
      await expect(row(0)).toBeFocused();
      await page.keyboard.press('ArrowUp');
      await expect(row(0)).toBeFocused();

      // Right-click selects the row under the pointer, then the menu opens; Esc returns focus to it.
      await row(2).click({ button: 'right' });
      await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
      await expect(row(2)).toHaveAttribute('aria-selected', 'true');
      await page.keyboard.press('Escape');
      await expect(page.locator('[data-testid="context-menu"]')).toHaveCount(0);
      await expect(row(2)).toBeFocused();

      await page.keyboard.press('ArrowUp');
      await expect(row(1)).toBeFocused();
      await page.keyboard.press('Shift+F10');
      await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
      await page.keyboard.press('Escape');
      await expect(page.locator('[data-testid="context-menu"]')).toHaveCount(0);
      await expect(row(1)).toBeFocused();
    },
  );

  // --- collections tree -------------------------------------------------------------------------

  const NOW = '2026-01-01T00:00:00.000Z';
  const COLLECTION_TREE = {
    collections: [
      { id: 'col-1', name: 'Orders API', sortOrder: 0, createdAt: NOW, updatedAt: NOW },
    ],
    items: [
      {
        id: 'item-folder',
        collectionId: 'col-1',
        parentId: null,
        kind: 'folder',
        name: 'Orders',
        sortOrder: 0,
        method: '',
        url: '',
        createdAt: NOW,
        updatedAt: NOW,
      },
      {
        id: 'item-create',
        collectionId: 'col-1',
        parentId: 'item-folder',
        kind: 'request',
        name: 'Create order',
        sortOrder: 0,
        method: 'POST',
        url: 'https://api.example.com/v2/orders',
        createdAt: NOW,
        updatedAt: NOW,
      },
      {
        id: 'item-health',
        collectionId: 'col-1',
        parentId: null,
        kind: 'request',
        name: 'Health check',
        sortOrder: 1,
        method: 'GET',
        url: 'https://api.example.com/healthz',
        createdAt: NOW,
        updatedAt: NOW,
      },
    ],
  };
  const HEALTH_REQUEST = {
    method: 'GET',
    url: 'https://api.example.com/healthz',
    headers: [],
    bodyMode: 'code',
    body: '',
    code: '',
    codeLanguage: 'json',
    urlEncoded: [],
    formData: [],
    binaryFile: null,
  };

  studioTest(
    'collections tree — arrows, expand and collapse, Enter opens, Shift+F10 and focus return',
    async ({ relaunch }) => {
      const { window: page } = await relaunch({
        control: [
          { channel: IPC.collectionsList, response: COLLECTION_TREE },
          { channel: IPC.collectionsGetRequest, response: HEALTH_REQUEST },
        ],
      });
      await modeTab(page, 'api').click();
      const row = (id: string) => page.locator(`[data-testid="collection-row"][data-id="${id}"]`);

      await row('col-1').focus();
      await page.keyboard.press('ArrowRight');
      await expect(row('item-folder')).toBeVisible();
      await expect(row('item-health')).toBeVisible();

      await page.keyboard.press('ArrowRight');
      await expect(row('item-folder')).toBeFocused();
      await expect(row('item-folder')).toHaveAttribute('aria-selected', 'true');
      await page.keyboard.press('ArrowRight');
      await expect(row('item-create')).toBeVisible();
      await page.keyboard.press('ArrowRight');
      await expect(row('item-create')).toBeFocused();
      await page.keyboard.press('ArrowLeft');
      await expect(row('item-folder')).toBeFocused();
      await page.keyboard.press('ArrowLeft');
      await expect(row('item-create')).toHaveCount(0);

      await page.keyboard.press('End');
      await expect(row('item-health')).toBeFocused();
      await page.keyboard.press('Home');
      await expect(row('col-1')).toBeFocused();
      await expect(page.locator('[data-testid="collection-row"][tabindex="0"]')).toHaveCount(1);

      await row('item-health').click({ button: 'right' });
      await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
      await expect(row('item-health')).toHaveAttribute('aria-selected', 'true');
      await page.keyboard.press('Escape');
      await expect(row('item-health')).toBeFocused();
      await page.keyboard.press('Shift+F10');
      await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
      await page.keyboard.press('Escape');
      await expect(row('item-health')).toBeFocused();

      await page.keyboard.press('Enter');
      await expect(page.locator('[data-testid="http-request-view"]')).toBeVisible();
    },
  );

  // --- request history lists --------------------------------------------------------------------

  function httpEntry(id: string, status: number, sentAt: string) {
    return {
      id,
      itemId: null,
      tabId: 't1',
      sentAt,
      method: 'GET',
      url: 'https://api.example.com/orders',
      environment: '',
      status,
      statusText: status === 200 ? 'OK' : 'Not Found',
      elapsedMs: 10,
      bodyBytes: 5,
      storedBytes: 20,
    };
  }

  studioTest(
    'http history — arrows, Home/End and Enter view the focused entry',
    async ({ relaunch }) => {
      const { window: page, control } = await relaunch({
        control: [
          {
            channel: IPC.historyList,
            response: [
              httpEntry('e3', 200, '2024-06-01T10:10:00.000Z'),
              httpEntry('e2', 404, '2024-06-01T10:05:00.000Z'),
              httpEntry('e1', 200, '2024-06-01T10:00:00.000Z'),
            ],
          },
        ],
      });
      await openHttpModeAndNewRequest(page);
      await page.click('[data-testid="http-response-pane-history"]');
      const rows = page.locator('[data-testid="http-history-row"]');
      await expect(rows).toHaveCount(3);

      await rows.nth(0).focus();
      await page.keyboard.press('ArrowDown');
      await expect(rows.nth(1)).toBeFocused();
      await expect(page.locator('[data-testid="http-history-row"][tabindex="0"]')).toHaveCount(1);
      await page.keyboard.press('End');
      await expect(rows.nth(2)).toBeFocused();
      await page.keyboard.press('ArrowDown');
      await expect(rows.nth(2)).toBeFocused();
      await page.keyboard.press('Home');
      await expect(rows.nth(0)).toBeFocused();

      await page.keyboard.press('ArrowDown');
      await page.keyboard.press('Enter');
      await expect
        .poll(() => control.log().filter((e) => e.channel === IPC.historyGet).length)
        .toBeGreaterThan(0);
    },
  );

  studioTest('grpc history — arrows and Home/End move between calls', async ({ relaunch }) => {
    const entry = (id: string) => ({
      id,
      itemId: null,
      tabId: 'tab-grpc-1',
      calledAt: '2026-01-01T00:00:00.000Z',
      target: 'demo.example.com:443',
      method: 'demo.Echo/SayHello',
      streaming: 'unary',
      environment: '',
      code: 0,
      codeName: 'OK',
      statusMessage: '',
      elapsedMs: 5,
      messageCount: 1,
      messageBytes: 20,
      storedBytes: 20,
    });
    const scope = { itemId: '', tabId: 'tab-grpc-1' };
    const { window: page } = await relaunch({
      control: [
        { channel: IPC.tabsList, response: [grpcTab({})] },
        {
          channel: IPC.grpcHistoryList,
          args: scope,
          response: [entry('c3'), entry('c2'), entry('c1')],
        },
      ],
    });
    const rows = page.locator('[data-testid="grpc-history-row"]');
    await expect(rows).toHaveCount(3);

    await rows.nth(0).focus();
    await page.keyboard.press('ArrowDown');
    await expect(rows.nth(1)).toBeFocused();
    await page.keyboard.press('End');
    await expect(rows.nth(2)).toBeFocused();
    await page.keyboard.press('Home');
    await expect(rows.nth(0)).toBeFocused();
    await expect(page.locator('[data-testid="grpc-history-row"][tabindex="0"]')).toHaveCount(1);
  });

  // --- redis Browse and KeyValue ----------------------------------------------------------------

  const REDIS_USER_NS = `${REDIS_DB0}/namespace:user`;
  const REDIS_USER1_NS = `${REDIS_USER_NS}/namespace:1`;
  const REDIS_BIGHASH = `${REDIS_USER1_NS}/key:user%3A1%3Abighash`;
  const MENU = '[data-testid="context-menu"]';

  studioTest(
    'browse — arrows, Home/End, Space selects, Enter opens, Shift+F10 menu and focus return',
    async ({ relaunch }) => {
      const { window: page } = await relaunch({ control: redisControl, stream: redisPorts });
      await openRedisBrowse(page);
      const view = page.locator('[data-testid="browse-view"]');
      const rows = view.locator('[data-testid="browse-row"]');
      await expect(rows).toHaveCount(7);

      await rows.first().focus();
      await page.keyboard.press('ArrowDown');
      await expect(rows.nth(1)).toBeFocused();
      await expect(view.locator('[data-testid="browse-row"][tabindex="0"]')).toHaveCount(1);
      await page.keyboard.press('End');
      await expect(rows.nth(6)).toBeFocused();
      await page.keyboard.press('ArrowDown');
      await expect(rows.nth(6)).toBeFocused();
      await page.keyboard.press('Home');
      await expect(rows.nth(0)).toBeFocused();
      await page.keyboard.press('ArrowUp');
      await expect(rows.nth(0)).toBeFocused();

      await page.keyboard.press('ArrowDown');
      await page.keyboard.press('Space');
      await expect(rows.nth(1)).toHaveAttribute('aria-selected', 'true');

      await page.keyboard.press('Shift+F10');
      await expect(page.locator(MENU)).toBeVisible();
      await page.keyboard.press('Escape');
      await expect(page.locator(MENU)).toHaveCount(0);
      await expect(rows.nth(1)).toBeFocused();

      const userRow = view.locator(`[data-testid="browse-row"][data-path="${REDIS_USER_NS}"]`);
      await userRow.focus();
      await page.keyboard.press('Enter');
      await expect(view).toHaveAttribute('data-level', REDIS_USER_NS);
    },
  );

  studioTest(
    'keyvalue — arrows, Home/End, Space selects, Shift+F10 menu and focus return',
    async ({ relaunch }) => {
      const { window: page } = await relaunch({ control: redisControl, stream: redisPorts });
      await openRedisBrowse(page);
      const browse = page.locator('[data-testid="browse-view"]');
      await browse.locator(`[data-testid="browse-row"][data-path="${REDIS_USER_NS}"]`).dblclick();
      await browse.locator(`[data-testid="browse-row"][data-path="${REDIS_USER1_NS}"]`).dblclick();
      await openBrowseLeaf(page, REDIS_BIGHASH);
      const view = page.locator(`[data-testid="keyvalue-view"][data-path="${REDIS_BIGHASH}"]`);
      const rows = view.locator('[data-testid="keyvalue-row"]');
      await expect(rows.first()).toBeVisible({ timeout: 15_000 });

      await rows.first().focus();
      await page.keyboard.press('ArrowDown');
      await expect(rows.nth(1)).toBeFocused();
      await expect(view.locator('[data-testid="keyvalue-row"][tabindex="0"]')).toHaveCount(1);
      await page.keyboard.press('ArrowUp');
      await expect(rows.nth(0)).toBeFocused();
      await page.keyboard.press('ArrowUp');
      await expect(rows.nth(0)).toBeFocused();

      // 100 rows are virtualized: End jumps past the rendered window and renders the last row.
      await page.keyboard.press('End');
      await expect(rows.last()).toBeFocused();
      await expect(rows.last()).toContainText('100');
      await page.keyboard.press('Home');
      await expect(rows.first()).toBeFocused();
      await expect(rows.first()).toContainText('1');

      // Selecting a row publishes it to the cell editor panel.
      await expect(page.locator('[data-testid="cell-editor-panel"]')).toHaveCount(0);
      await page.keyboard.press('Space');
      await expect(page.locator('[data-testid="cell-editor-panel"]')).toBeVisible();
      await page.keyboard.press('Shift+F10');
      await expect(page.locator(MENU)).toBeVisible();
      await page.keyboard.press('Escape');
      await expect(page.locator(MENU)).toHaveCount(0);
      await expect(rows.first()).toBeFocused();
    },
  );

  // --- documents ----------------------------------------------------------------------------------

  studioTest(
    'documents — arrows, Home/End, Space selects, Enter expands, Shift+F10 menu and focus return',
    async ({ relaunch }) => {
      const fixture = widgetsFixture('conn-doc-kb');
      const { window: page } = await relaunch({
        control: [
          {
            channel: IPC.connectionsList,
            response: [mongoConnectionSummary('conn-doc-kb', 'Mongo KB', 'red')],
          },
          ...fixture.control,
        ],
        stream: fixture.port,
      });
      await openRowMenu(page, '');
      await page.click('[data-testid="menu-item-connect"]');
      await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
        'data-status',
        'connected',
        { timeout: 10_000 },
      );
      await expandRow(page, '');
      await expandRow(page, 'database:kira_test');
      await (await findRow(page, WIDGETS_PATH)).dblclick();
      const view = page.locator('[data-testid="document-view"]');
      const rows = view.locator('[data-testid="document-row"] [role="option"][data-nav-key]');
      await expect(rows.first()).toBeVisible({ timeout: 15_000 });

      await rows.first().focus();
      await page.keyboard.press('ArrowDown');
      await expect(rows.nth(1)).toBeFocused();
      await expect(view.locator('[role="option"][data-nav-key][tabindex="0"]')).toHaveCount(1);
      await expect(rows.nth(1)).toHaveAttribute('aria-selected', 'true');
      await page.keyboard.press('ArrowUp');
      await expect(rows.first()).toBeFocused();
      await page.keyboard.press('ArrowUp');
      await expect(rows.first()).toBeFocused();

      await page.keyboard.press('End');
      await expect(rows.last()).toBeFocused();
      await page.keyboard.press('Home');
      await expect(rows.first()).toBeFocused();

      const toggle = rows.first().locator('[data-testid="document-toggle-expand"]');
      await expect(toggle).toHaveAttribute('aria-label', 'Collapse');
      await page.keyboard.press('Enter');
      await expect(toggle).toHaveAttribute('aria-label', 'Expand');
      await page.keyboard.press('Enter');
      await expect(toggle).toHaveAttribute('aria-label', 'Collapse');

      await page.keyboard.press('Space');
      await expect(rows.first()).toHaveAttribute('aria-selected', 'true');
      await page.keyboard.press('Shift+F10');
      await expect(page.locator(MENU)).toBeVisible();
      await page.keyboard.press('Escape');
      await expect(page.locator(MENU)).toHaveCount(0);
      await expect(rows.first()).toBeFocused();
    },
  );

  // --- stream ---------------------------------------------------------------------------------------

  studioTest(
    'stream — arrows, Home/End, Space selects, Shift+F10 menu and focus return',
    async ({ relaunch }) => {
      const { window: page } = await relaunch({ control: sqsControl, stream: sqsPorts });
      await openRowMenu(page, '');
      await page.click('[data-testid="menu-item-connect"]');
      await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
        'data-status',
        'connected',
        { timeout: 10_000 },
      );
      await expandRow(page, '');
      await (await findRow(page, 'queue:orders-queue')).dblclick();
      const view = page.locator('[data-testid="stream-view"][data-path="queue:orders-queue"]');
      await view.locator('[data-testid="stream-poll"]').click();
      const rows = view.locator('[data-testid="stream-row"] [role="option"][data-nav-key]');
      await expect(rows).toHaveCount(5, { timeout: 15_000 });

      await rows.first().focus();
      await page.keyboard.press('ArrowDown');
      await expect(rows.nth(1)).toBeFocused();
      await expect(
        view.locator('[data-testid="stream-row"] [data-nav-key][tabindex="0"]'),
      ).toHaveCount(1);
      await page.keyboard.press('End');
      await expect(rows.nth(4)).toBeFocused();
      await page.keyboard.press('ArrowDown');
      await expect(rows.nth(4)).toBeFocused();
      await page.keyboard.press('Home');
      await expect(rows.first()).toBeFocused();
      await page.keyboard.press('ArrowUp');
      await expect(rows.first()).toBeFocused();

      await page.keyboard.press('Space');
      await expect(rows.first()).toHaveAttribute('aria-selected', 'true');
      await page.keyboard.press('Shift+F10');
      await expect(page.locator(MENU)).toBeVisible();
      await page.keyboard.press('Escape');
      await expect(page.locator(MENU)).toHaveCount(0);
      await expect(rows.first()).toBeFocused();
    },
  );

  // --- console result grid ----------------------------------------------------------------------

  studioTest(
    'console key-value result — arrows, Home/End, Space selects, Shift+F10 menu and focus return',
    async ({ relaunch }) => {
      const ports = structuredClone(redisPorts);
      const exec = ports.find((p) => p.op === DATA_OP.execute);
      if (!exec) throw new Error('redis fixture has no execute snapshot');
      const page0 = (exec.response as { pages: { fields: string[]; values: string[] }[] }).pages[0];
      page0.fields = ['alpha', 'bravo', 'charlie', 'delta'];
      page0.values = ['1', '2', '3', '4'];
      const { window: page } = await relaunch({ control: redisControl, stream: ports });
      await openRowMenu(page, '');
      await page.click('[data-testid="menu-item-connect"]');
      await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
        'data-status',
        'connected',
        { timeout: 10_000 },
      );
      await expandRow(page, '');
      await openRowMenu(page, REDIS_DB0);
      await page.click('[data-testid="menu-item-open-console"]');
      const consoleView = page.locator('[data-testid="console-view"]');
      await consoleView.locator('.view-lines').click();
      await page.keyboard.type('DBSIZE');
      await page.click('[data-testid="console-run-statement"]');
      const rows = consoleView.locator('[data-testid="console-result-kv-row"]');
      await expect(rows).toHaveCount(4);

      await rows.first().focus();
      await page.keyboard.press('ArrowDown');
      await expect(rows.nth(1)).toBeFocused();
      await expect(
        consoleView.locator('[data-testid="console-result-kv-row"][tabindex="0"]'),
      ).toHaveCount(1);
      await page.keyboard.press('End');
      await expect(rows.nth(3)).toBeFocused();
      await page.keyboard.press('ArrowDown');
      await expect(rows.nth(3)).toBeFocused();
      await page.keyboard.press('Home');
      await expect(rows.nth(0)).toBeFocused();
      await page.keyboard.press('ArrowUp');
      await expect(rows.nth(0)).toBeFocused();

      await page.keyboard.press('Space');
      await expect(rows.first()).toHaveAttribute('aria-selected', 'true');
      await page.keyboard.press('Shift+F10');
      await expect(page.locator(MENU)).toBeVisible();
      await page.keyboard.press('Escape');
      await expect(page.locator(MENU)).toHaveCount(0);
      await expect(rows.first()).toBeFocused();
    },
  );

  studioTest(
    'console document result — arrows, Home/End, Enter expands, Shift+F10 menu and focus return',
    async ({ relaunch }) => {
      const fixture = widgetsFixture('conn-doc-kb');
      const statement = 'db.widgets.find()';
      const { window: page } = await relaunch({
        control: [
          {
            channel: IPC.connectionsList,
            response: [mongoConnectionSummary('conn-doc-kb', 'Mongo KB', 'red')],
          },
          ...fixture.control,
        ],
        stream: [
          ...fixture.port,
          {
            op: DATA_OP.execute,
            payload: {
              connectionId: 'conn-doc-kb',
              path: 'database:kira_test',
              statements: [statement],
            },
            response: {
              kind: 'execute',
              pages: [
                {
                  kind: 'document',
                  ids: WIDGETS_IDS.slice(0, 4),
                  bodies: WIDGETS_BODIES.slice(0, 4),
                  position: {
                    offset: 0,
                    pageSize: 4,
                    hasMore: false,
                    nextToken: null,
                    prevToken: null,
                    strategy: 'offset',
                  },
                },
              ],
            },
          },
        ],
      });
      await openRowMenu(page, '');
      await page.click('[data-testid="menu-item-connect"]');
      await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
        'data-status',
        'connected',
        { timeout: 10_000 },
      );
      await expandRow(page, '');
      await openRowMenu(page, 'database:kira_test');
      await page.click('[data-testid="menu-item-open-console"]');
      const consoleView = page.locator('[data-testid="console-view"]');
      await consoleView.locator('.view-lines').click();
      await page.keyboard.type(statement);
      await page.click('[data-testid="console-run-statement"]');
      const rows = consoleView.locator(
        '[data-testid="console-result-doc-row"] [role="option"][data-nav-key]',
      );
      await expect(rows).toHaveCount(4);

      await rows.first().focus();
      await page.keyboard.press('ArrowDown');
      await expect(rows.nth(1)).toBeFocused();
      await page.keyboard.press('End');
      await expect(rows.nth(3)).toBeFocused();
      await page.keyboard.press('Home');
      await expect(rows.first()).toBeFocused();

      const toggle = rows.first().locator('[data-testid="document-toggle-expand"]');
      const before = await toggle.getAttribute('aria-label');
      await page.keyboard.press('Enter');
      await expect(toggle).not.toHaveAttribute('aria-label', before ?? '');
      await page.keyboard.press('Space');
      await expect(rows.first()).toHaveAttribute('aria-selected', 'true');
      await page.keyboard.press('Shift+F10');
      await expect(page.locator(MENU)).toBeVisible();
      await page.keyboard.press('Escape');
      await expect(page.locator(MENU)).toHaveCount(0);
      await expect(rows.first()).toBeFocused();
    },
  );

  // --- saved list menu type-ahead -----------------------------------------------------------------

  studioTest(
    'saved queries popover — type-ahead jumps by first letter and prefix, Enter applies',
    async ({ relaunch }) => {
      const id = 'conn-saved-kb';
      const saved = ['Alpha', 'Apple', 'Bravo', 'Beta'].map((name, i) => ({
        kind: 'console' as const,
        id: `saved-${i}`,
        connectionId: id,
        path: ORDER_ITEMS_PATH,
        name,
        pinned: false,
        createdAt: '2026-01-01T00:00:00.000Z',
        usedAt: null,
        body: { text: `SELECT '${name}';` },
      }));
      const fixture = orderItemsFixture(id);
      const { window: page } = await relaunch({
        control: [
          { channel: IPC.connectionsList, response: [] },
          {
            channel: IPC.connectionsCreate,
            args: connectionCreateArgs('Saved DB', 'green'),
            response: postgresConnectionSummary(id, 'Saved DB', 'green'),
          },
          ...fixture.control,
          {
            channel: IPC.queriesListConsole,
            args: { connectionId: id, path: ORDER_ITEMS_PATH },
            response: saved,
          },
          { channel: IPC.queriesTouch, args: { id: 'saved-3' }, response: undefined },
        ],
        stream: fixture.port,
      });
      await connectAndExpand(page, 'Saved DB', 'green');
      await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
      await page.click('[data-testid="console-saved-toggle"]');
      const entries = page.locator('[data-testid="console-saved-entry"]');
      await expect(entries).toHaveCount(4);

      await entries.first().focus();
      await page.keyboard.press('b');
      await expect(entries.nth(2)).toBeFocused();
      await page.keyboard.press('b');
      await expect(entries.nth(3)).toBeFocused();
      await page.keyboard.press('b');
      await expect(entries.nth(2)).toBeFocused();
      // After the buffer resets, a two-letter prefix picks the matching row.
      // Behavior under test: the type-ahead buffer resets after a real-time idle window.
      await page.waitForTimeout(600);
      await page.keyboard.type('be');
      await expect(entries.nth(3)).toBeFocused();
      // Behavior under test: the type-ahead buffer resets after a real-time idle window.
      await page.waitForTimeout(600);
      await page.keyboard.press('a');
      await expect(entries.nth(0)).toBeFocused();

      await page.keyboard.press('Enter');
      await expect
        .poll(() => editorText(page.locator('[data-testid="console-view"]')))
        .toContain("SELECT 'Alpha';");
    },
  );
});
