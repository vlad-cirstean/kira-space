import { buildPackedChunk } from '@kira/git-core/testing/packedChunk';
import { expect, test } from '../ui/fixtures';
import { buildGraphStreamChunk } from '../ui/support/graphStreamFixture';
import { IPC } from '../ui/support/ipcChannels';
import type { ControlSnapshot } from '../ui/support/types';

// P229: the Git module's look (surfaces, row density, badges, detail pane, dialogs) against the
// rest of the app. Mocked git transport; commit times sit a fixed 3.5h before the run, so the
// relative date column always reads "3h".

const REPO = {
  id: 'repo-visual-1',
  name: 'visual-repo',
  root: '/tmp/visual-repo',
  repoId: '/tmp/visual-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};

const sha = (n: number) => (n + 1).toString(16).padStart(2, '0').repeat(20);
const NOW_SECONDS = Math.floor(Date.now() / 1000) - 3.5 * 3600;
const IDENTITY = {
  name: 'Fake Author',
  email: 'fake@example.com',
  timestamp: Math.floor(NOW_SECONDS),
};

// Row 0 merges rows 1 and 2, which both fork from row 3; the rest is a straight line.
const SUBJECTS = [
  "Merge branch 'feature/search' into main",
  'Add search box to the toolbar',
  'Fix off-by-one in paging',
  'Refactor the commit store',
  'Add column resize handles',
  'Document the keyboard shortcuts',
  'Release 1.0.0',
  'Tighten the lane palette',
  'Add the detail pane',
  'Initial commit',
];

const rows = SUBJECTS.map((subject, n) => ({
  sha: sha(n),
  subject,
  parents: n === 0 ? [sha(1), sha(2)] : n === 1 || n === 2 ? [sha(3)] : n < 9 ? [sha(n + 1)] : [],
  decoration:
    n === 0
      ? [
          { kind: 'branch' as const, name: 'main', isHead: true },
          { kind: 'remoteBranch' as const, name: 'origin/main' },
        ]
      : n === 1
        ? [{ kind: 'branch' as const, name: 'feature/search', isHead: false }]
        : n === 4
          ? [
              {
                kind: 'branch' as const,
                name: 'feature/column-widths-persist-per-tab',
                isHead: false,
              },
              {
                kind: 'remoteBranch' as const,
                name: 'origin/feature/column-widths-persist-per-tab',
              },
              {
                kind: 'branch' as const,
                name: 'release/2026-10-candidate-with-long-name',
                isHead: false,
              },
              { kind: 'tag' as const, name: 'v2.0.0-rc.1' },
              { kind: 'remoteBranch' as const, name: 'origin/release/2026-10-candidate' },
            ]
          : n === 6
            ? [{ kind: 'tag' as const, name: 'v1.0.0' }]
            : [],
}));

const FILES = [
  { kind: 'modified', path: 'src/App.vue' },
  {
    kind: 'added',
    path: 'src/components/SearchBox.vue',
  },
  { kind: 'deleted', path: 'src/legacy/search.ts' },
  { kind: 'modified', path: 'README.md' },
].map((f) => ({ ...f, originalPath: undefined, similarity: undefined }));

const ref = (
  shortName: string,
  refname: string,
  kind: string,
  objectId: string,
  isHead = false,
) => ({
  refname,
  kind,
  shortName,
  objectId,
  peeledObjectId: undefined,
  upstream: undefined,
  track: undefined,
  committerDate: IDENTITY.timestamp,
  isHead,
  checkedOutIn: undefined,
  annotation: undefined,
});

const MOCK_RESULTS = {
  'repo.open': {
    kind: 'ok',
    repo: {
      repoId: REPO.repoId,
      root: REPO.root,
      gitDir: `${REPO.root}/.git`,
      commonDir: `${REPO.root}/.git`,
      isBare: false,
      isLinkedWorktree: false,
      head: { kind: 'branch', name: 'main' },
    },
  },
  'refs.list': {
    branches: [
      ref('main', 'refs/heads/main', 'branch', sha(0), true),
      ref('feature/search', 'refs/heads/feature/search', 'branch', sha(1)),
    ],
    remoteBranches: [ref('origin/main', 'refs/remotes/origin/main', 'remoteBranch', sha(0))],
    tags: [ref('v1.0.0', 'refs/tags/v1.0.0', 'tag', sha(6))],
    head: { kind: 'branch', name: 'main' },
  },
  'graph.refresh': {},
  'graph.status': { loaded: rows.length, remaining: 0, exhausted: true },
  'status.get': {
    head: { kind: 'branch', name: 'main' },
    upstream: { name: 'origin/main', ahead: 0, behind: 0 },
    counts: { staged: 1, unstaged: 2, untracked: 0, unmerged: 0 },
    isClean: false,
    dirtyPaths: ['src/App.vue', 'README.md', 'src/components/SearchBox.vue'],
    dirtyTruncated: false,
    inProgress: null,
    autoFetch: null,
  },
  'stash.list': { entries: [] },
  'working.detail': { files: FILES.slice(0, 3) },
  'commit.detail': {
    sha: sha(1),
    parents: [sha(3)],
    author: IDENTITY,
    committer: IDENTITY,
    subject: SUBJECTS[1],
    body: 'Adds a search box to the toolbar with case, word and regex toggles.\n\nThe row list filters as you type.',
    trailers: [{ token: 'Co-authored-by', value: 'Fake Author <fake@example.com>' }],
    signature: { status: 'N', signer: '' },
    decoration: rows[1].decoration,
    parentIndex: 0,
    files: FILES,
  },
};

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
  {
    channel: IPC.codeWorkspaceListFiles,
    args: { id: REPO.id },
    response: { paths: ['a.ts'], status: {}, truncated: false },
  },
];

const VIEW_STATE = {
  version: 8,
  repoId: REPO.repoId,
  loadedRows: rows.length,
  detailOpen: false,
  scrollRow: 0,
  selectedSha: null,
  columnWidths: { author: 140, date: 152, graph: 60 },
  dateFormat: 'relative',
  detailWidth: 380,
  fileListMode: 'tree',
  searchCaseSensitive: false,
  searchWholeWord: false,
  searchRegex: false,
  searchScope: 'both',
  searchOpen: false,
  collapseBranches: false,
};

async function openGraph(relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch']) {
  const { window: win } = await relaunch({
    control: [
      ...CONTROL,
      {
        channel: IPC.tabsList,
        response: [
          {
            id: 'visual-repo-graph',
            kind: 'repo-graph',
            connectionId: null,
            path: '',
            order: 0,
            active: true,
            workspaceId: REPO.id,
            state: { viewState: VIEW_STATE, reviewSession: null },
          },
        ],
      },
    ],
    gitStream: {
      repoId: REPO.repoId,
      extraResults: MOCK_RESULTS,
      graphStreamChunks: [
        buildGraphStreamChunk(
          REPO.repoId,
          0,
          buildPackedChunk(rows, { timestamp: IDENTITY.timestamp }),
        ),
      ],
    },
  });
  await win.setViewportSize({ width: 1400, height: 820 });
  await expect(win.locator('[data-testid="commit-grid"] .slick-row[data-row="9"]')).toBeVisible();
  await expect(win.locator('[data-testid="uncommitted-strip"]')).toBeVisible();
  return win;
}

test('git module: graph (P229)', async ({ relaunch }) => {
  const win = await openGraph(relaunch);
  await expect(win.locator('[data-testid="repo-graph-host"]')).toHaveScreenshot('git-graph.png');
});

test('git module: graph with the detail pane (P229)', async ({ relaunch }) => {
  const win = await openGraph(relaunch);
  await win.locator('[data-testid="commit-grid"] .slick-row[data-row="1"]').click();
  await expect(win.locator('[data-testid="detail-region"]')).toBeVisible();
  await expect(
    win.locator('[data-testid="detail-region"] [data-testid="file-tree-row"]').first(),
  ).toBeVisible();
  await expect(win.locator('[data-testid="repo-graph-host"]')).toHaveScreenshot(
    'git-graph-detail.png',
  );
});

test('git module: stash dialog (P229)', async ({ relaunch }) => {
  const win = await openGraph(relaunch);
  await win.locator('[data-testid="stash-changes-button"]').click();
  const dialog = win.getByRole('dialog');
  await expect(dialog).toBeVisible();
  await expect(dialog).toHaveScreenshot('git-stash-dialog.png');
});

test('git module: repository settings dialog (P256)', async ({ relaunch }) => {
  const win = await openGraph(relaunch);
  await win.locator('[data-testid="repo-settings-button"]').click();
  const dialog = win.getByRole('dialog');
  await expect(dialog).toBeVisible();
  await expect(dialog).toHaveScreenshot('git-repo-settings-dialog.png');
});

test('git module: branch picker (P256)', async ({ relaunch }) => {
  const win = await openGraph(relaunch);
  await win.locator('[data-testid="branch-trigger"]').click();
  const picker = win.getByRole('dialog');
  await expect(picker).toBeVisible();
  await expect(picker).toHaveScreenshot('git-refs-popover.png');
});

test('git module: row context menu (P258)', async ({ relaunch }) => {
  const win = await openGraph(relaunch);
  await win
    .locator('[data-testid="commit-grid"] .slick-row[data-row="1"]')
    .click({ button: 'right' });
  const menu = win.getByRole('menu');
  await expect(menu).toBeVisible();
  await expect(menu).toHaveScreenshot('git-row-menu.png');
});
