import { buildPackedChunk } from '@kira/git-core/testing/packedChunk';
import { CAPTURE_LAUNCH, CAPTURE_NOW } from '@workbench/testing/screenshots/capture';
import { expect, test } from '../ui/fixtures';
import { adeV2Control, FIXED_NOW } from '../ui/support/adeV2';
import { buildGraphStreamChunk } from '../ui/support/graphStreamFixture';
import { IPC } from '../ui/support/ipcChannels';
import { openMemory, withMemory } from '../ui/support/memoryNav';
import type { ControlSnapshot } from '../ui/support/types';
import { shoot } from './support/shoot';

// P280: presentation screenshots of Kira Space, from the production bundle against the mocked
// runtime. Run by `bun run screenshots`; output goes to /screenshots.

test.beforeAll(() => {
  expect(CAPTURE_NOW).toBe(FIXED_NOW);
});

const REPO = {
  id: 'repo-acme-web',
  name: 'acme-web',
  root: '/Users/ana/code/acme-web',
  repoId: '/Users/ana/code/acme-web',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};

// Deterministic hex that reads like a real hash, not a repeated byte.
const sha = (n: number) =>
  Array.from({ length: 5 }, (_, i) =>
    ((Math.imul(n + 1, 2654435761) + Math.imul(i + 1, 40503)) >>> 0).toString(16).padStart(8, '0'),
  ).join('');
const COMMIT_SECONDS = Math.floor(CAPTURE_NOW / 1000) - 3.5 * 3600;
const IDENTITY = {
  name: 'Alice Morgan',
  email: 'alice.morgan@acme.example',
  timestamp: COMMIT_SECONDS,
};

// Row 0 merges rows 1 and 2, which both fork from row 3; the rest is a straight line.
const SUBJECTS = [
  "Merge branch 'feature/search' into main",
  'Add search box to the toolbar',
  'Fix off-by-one in order paging',
  'Refactor the cart store',
  'Add column resize handles to the orders table',
  'Document the keyboard shortcuts',
  'Release 1.4.0',
  'Tighten the checkout lane colours',
  'Add the order detail pane',
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
        : n === 6
          ? [{ kind: 'tag' as const, name: 'v1.4.0' }]
          : [],
}));

const FILES = [
  { kind: 'modified', path: 'src/App.vue' },
  { kind: 'added', path: 'src/components/SearchBox.vue' },
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
    tags: [ref('v1.4.0', 'refs/tags/v1.4.0', 'tag', sha(6))],
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
    body: 'Adds a search box to the toolbar with case, word and regex toggles.\n\nThe order list filters as you type.',
    trailers: [],
    signature: { status: 'N', signer: '' },
    decoration: rows[1]?.decoration,
    parentIndex: 0,
    files: FILES,
  },
};

const VIEW_STATE = {
  version: 8,
  repoId: REPO.repoId,
  loadedRows: rows.length,
  detailOpen: false,
  scrollRow: 0,
  selectedSha: null,
  columnWidths: { author: 110, date: 80, graph: 60 },
  dateFormat: 'relative',
  detailWidth: 320,
  fileListMode: 'tree',
  searchCaseSensitive: false,
  searchWholeWord: false,
  searchRegex: false,
  searchScope: 'both',
  searchOpen: false,
  collapseBranches: false,
};

test('space git graph', async ({ relaunch, consoleErrors }) => {
  const control: ControlSnapshot[] = [
    { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
    { channel: IPC.repoConfigList, response: { dir: '~/.kira-space/config/repos', files: [] } },
    { channel: IPC.adeTaskRepos, response: { repos: [], folders: [] } },
    { channel: IPC.adeTaskSessions, response: { sessions: [] } },
    {
      channel: IPC.codeWorkspaceRepoHeads,
      args: { ids: [REPO.id] },
      response: [{ id: REPO.id, head: { kind: 'branch', name: 'main' } }],
    },
    {
      channel: IPC.codeWorkspaceListFiles,
      args: { id: REPO.id },
      response: { paths: ['src/App.vue'], status: {}, truncated: false },
    },
    {
      channel: IPC.tabsList,
      response: [
        {
          id: 'acme-web-graph',
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
  ];
  const { window: win } = await relaunch({
    ...CAPTURE_LAUNCH,
    control,
    gitStream: {
      repoId: REPO.repoId,
      extraResults: MOCK_RESULTS,
      graphStreamChunks: [
        buildGraphStreamChunk(
          REPO.repoId,
          0,
          buildPackedChunk(rows, {
            timestamp: IDENTITY.timestamp,
            dictionary: [IDENTITY.name, IDENTITY.email],
          }),
        ),
      ],
    },
  });
  const grid = win.locator('[data-testid="commit-grid"]');
  await expect(grid.locator('.slick-row[data-row="9"]')).toBeVisible();
  await expect(grid).toContainText(SUBJECTS[1] as string);
  await grid.locator('.slick-row[data-row="1"]').click();
  const detail = win.locator('[data-testid="detail-region"]');
  await expect(detail).toBeVisible();
  await expect(detail.locator('[data-testid="file-tree-row"]').first()).toBeVisible();

  await shoot(win, 'space-git-graph', consoleErrors);
  await shoot(win, 'space-commit-detail', consoleErrors);
});

test('space agents board', async ({ relaunch, consoleErrors }) => {
  const { window: win } = await relaunch({ ...CAPTURE_LAUNCH, control: adeV2Control() });
  await win.locator('[data-testid="ade-plan"]').waitFor();
  await expect(win.locator('[data-testid="ade-task"]').first()).toBeVisible();
  await expect(
    win.locator('[data-testid="ade-card"][data-task-id="T_search"] [data-testid="ade-step-label"]'),
  ).toHaveText('Make CI green 3/6');

  await shoot(win, 'space-agents-board', consoleErrors);
});

interface Fact {
  fact: string;
  origin: 'user' | 'agent' | 'external';
  scope: 'project' | 'company';
  reason: string;
  keywords: string[];
}

const PROJECT = 'github.com/acme/web';
const FACTS: Fact[] = [
  {
    fact: 'Deploys to staging run every weekday at 09:00 UTC.',
    origin: 'user',
    scope: 'company',
    reason: 'The release calendar says so.',
    keywords: ['deploy', 'staging', 'schedule'],
  },
  {
    fact: 'The web app uses PostgreSQL 16 for orders and customers.',
    origin: 'user',
    scope: 'project',
    reason: 'Stated in the migration notes.',
    keywords: ['postgres', 'database', 'orders'],
  },
  {
    fact: 'Run bun run test:unit before opening a pull request.',
    origin: 'agent',
    scope: 'project',
    reason: 'Found in CONTRIBUTING.md.',
    keywords: ['tests', 'pull request'],
  },
  {
    fact: 'Order totals round half up to two decimals.',
    origin: 'agent',
    scope: 'project',
    reason: 'Confirmed by the checkout tests.',
    keywords: ['orders', 'rounding', 'checkout'],
  },
  {
    fact: 'The search API rate limit is 60 requests per minute.',
    origin: 'external',
    scope: 'company',
    reason: 'Imported from the platform handbook.',
    keywords: ['search', 'api', 'rate limit'],
  },
  {
    fact: 'Feature flags live in config/flags.yaml, never in code.',
    origin: 'user',
    scope: 'project',
    reason: 'Stated in the engineering handbook.',
    keywords: ['flags', 'config'],
  },
  {
    fact: 'Integration tests use the seeded orders fixture.',
    origin: 'agent',
    scope: 'project',
    reason: 'Found in tests/README.md.',
    keywords: ['tests', 'fixtures'],
  },
  {
    fact: 'Releases are cut from main on the first Monday of the month.',
    origin: 'user',
    scope: 'company',
    reason: 'The release calendar says so.',
    keywords: ['release', 'schedule'],
  },
  {
    fact: 'Customer emails are masked in every log line.',
    origin: 'external',
    scope: 'company',
    reason: 'Imported from the security policy.',
    keywords: ['privacy', 'logging'],
  },
];

const MEMORIES = FACTS.map((f, i) => ({
  id: `m${i + 1}.1`,
  lineageId: `m${i + 1}`,
  createdAt: new Date(CAPTURE_NOW - (i + 1) * 3_600_000 * 7).toISOString(),
  fact: f.fact,
  historical: false,
  keywords: f.keywords,
  kind: 'fact',
  origin: f.origin,
  project: f.scope === 'project' ? PROJECT : null,
  reason: f.reason,
  scope: f.scope,
  sessionId: f.origin === 'agent' ? `session-${i + 1}` : null,
  sourceTag: null,
  status: 'current',
  supersededAt: null,
  supersededById: null,
  supersedesId: null,
  version: 1,
  versions: 1,
}));

test('space memory', async ({ relaunch, consoleErrors }) => {
  const { window: win } = await relaunch({
    ...CAPTURE_LAUNCH,
    control: withMemory([
      { channel: IPC.memoryRecent, response: MEMORIES },
      { channel: IPC.repoConfigList, response: { dir: '~/.kira-space/config/repos', files: [] } },
      { channel: IPC.memoryProjects, response: [PROJECT] },
      { channel: IPC.adeTaskRepos, response: { repos: [], folders: [] } },
      { channel: IPC.adeTaskSessions, response: { sessions: [] } },
      { channel: IPC.memoryImportJobs, response: [] },
      { channel: IPC.memoryContext, response: { memory: MEMORIES[0], groups: [] } },
      { channel: IPC.memoryHistory, response: { memories: [MEMORIES[0]], events: [] } },
      {
        channel: IPC.memorySemanticStatus,
        response: { state: 'installed', message: '', model: '', done: 0, total: 0 },
      },
    ]),
  });
  await openMemory(win);
  const first = win.locator('[data-testid="memory-row-m1.1"]');
  await expect(first).toBeVisible();
  await expect(first.locator('[data-testid="memory-row-origin"]')).toHaveText('User');
  await expect(win.locator('[data-testid="memory-row-m2.1"]')).toContainText('PostgreSQL 16');

  await first.click();
  await expect(win.locator('[data-testid="memory-detail"]')).toBeVisible();

  await shoot(win, 'space-memory', consoleErrors);
});
