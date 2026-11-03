import { expect, test } from './fixtures';
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
