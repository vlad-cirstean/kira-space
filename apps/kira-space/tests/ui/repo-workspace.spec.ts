import { dragTab } from '@workbench/testing/ui/drag';
import { defaultSettings } from '../../frontend/src/state/settingsDomain';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { gitStreamRequests, installGitStreamMock } from './support/gitStreamMock';
import {
  buildGraphStreamChunk,
  buildMultiBranchChunk,
  buildMultiBranchRefsList,
  buildOneCommitChunk,
  MULTI_BRANCH_FEATURE_NEWER_HIDDEN_COUNT,
  MULTI_BRANCH_FEATURE_NEWER_NAME,
} from './support/graphStreamFixture';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import { repoRowById, repoTab, treeRow } from './support/repoRows';
import type { ControlSnapshot } from './support/types';

// The cheapest proof of the one real claim a unit test can't reach — a repo
// workspace's own isolated, pinned-plus-preview tab strip, and that studio's own strip never shows
// a repo tab. "Import a fixture repository" here means seeding codeWorkspaceListRepos with one
// already-imported repo (the same shortcut mode-switch.spec.ts's own createAndConnect takes for a
// connection) — the import dialog itself is a thin pass-through CLAUDE.md's own bar gives nothing.

const REPO = {
  id: 'repo-1',
  name: 'demo-repo',
  root: '/tmp/demo-repo',
  repoId: '/tmp/demo-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};

const FILE_LISTING = { paths: ['a.ts', 'b.ts'], status: {}, truncated: false };

// C6 §10/§16: 'c.ts' carries a status glyph, so its row gets an "Open changes" entry — the other
// two rows above stay unchanged, gating the entry off for anything the tree doesn't color.
const FILE_LISTING_WITH_STATUS = {
  paths: ['a.ts', 'b.ts', 'c.ts'],
  status: { 'c.ts': 'M' },
  truncated: false,
};

const DIFF_SNAP: ControlSnapshot = {
  channel: IPC.codeWorkspaceReadDiff,
  args: { id: REPO.id, path: 'c.ts' },
  response: {
    path: 'c.ts',
    language: 'typescript',
    head: { kind: 'found', text: 'export const c = 1;\n', bytes: 20, limitBytes: 8 * 1024 * 1024 },
    worktree: {
      kind: 'found',
      text: 'export const c = 2;\n',
      bytes: 20,
      limitBytes: 8 * 1024 * 1024,
    },
  },
};

function readFileSnap(path: string, text: string): ControlSnapshot {
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

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
  { channel: IPC.codeWorkspaceListFiles, args: { id: REPO.id }, response: FILE_LISTING },
  readFileSnap('a.ts', 'export const a = 1;\n'),
  readFileSnap('b.ts', 'export const b = 2;\n'),
];

// P82 §12.3: a linked worktree and the row it becomes once switched to.
const WORKTREE_PATH = '/tmp/demo-repo-feature';
const WORKTREE_REPO = {
  id: 'repo-2',
  name: 'demo-repo-feature',
  root: WORKTREE_PATH,
  repoId: WORKTREE_PATH,
  sortOrder: 2,
  color: 'none',
  createdAt: '2026-01-02T00:00:00.000Z',
};
const WORKTREE_LIST_RESULT = {
  worktrees: contract<{ worktrees: Record<string, unknown>[] }>(
    'git-worktree',
    'git:worktree.list#after-add',
  ).worktrees.map((w, i) => ({
    ...w,
    path: i === 0 ? REPO.repoId : WORKTREE_PATH,
    head: String(i).repeat(40),
  })),
};

// P72 §7: the pinned graph tab now renders in `tab-strip-pinned`, a sibling of `tab-strip-row`
// rather than a descendant of it — `tab-strip-wrapper` is the common ancestor of both.

test('a repo workspace: pinned graph tab, preview-slot reuse, and promotion', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL });

  // Import a fixture repository (seeded above) — visible in the Git panel's repo list (the Git
  // module's own left panel, mounted while `git` mode is active — the app boots into it, and this
  // suite never switches away, P128 §2.6).
  await expect(repoRowById(page, REPO.id)).toBeVisible();

  // Opening it activates its own workspace and the row itself. A single click (onRowClick's own
  // open-or-activate semantics), not a double click: the auto-switch-to-Files already fires
  // on this same click, so a genuine second click at the same fixed point (what `.dblclick()`
  // sends) now lands on the Files tab's own relocated view-strip instead of the vanished row — a
  // real double-click's second physical click racing the same instant re-layout, not a test
  // artifact.
  await repoRowById(page, REPO.id).click();
  // P84 §8.3: opening the workspace auto-switched the panel to Files — switch back to see the row
  // itself.
  await page.locator('[data-testid="git-panel-tab-repos"]').click();
  await expect(repoRowById(page, REPO.id)).toHaveClass(/active/);

  // The pinned graph tab exists, alone, and has no close button.
  await expect(repoTab(page)).toHaveCount(1);
  const graphTab = repoTab(page, 'repo-graph');
  await expect(graphTab).toHaveCount(1);
  await expect(graphTab.locator('[data-testid="tab-close"]')).toHaveCount(0);

  // Single click opens a preview tab (§5.2 rule 1/4) — the strip now shows two tabs: pinned +
  // one preview, italic.
  await page.locator('[data-testid="git-panel-tab-files"]').click();
  await expect(treeRow(page, 'a.ts')).toBeVisible();
  await treeRow(page, 'a.ts').click();
  await expect(repoTab(page)).toHaveCount(2);
  const previewTab = repoTab(page, 'repo-file');
  await expect(previewTab).toHaveCount(1);
  await expect(previewTab).toHaveAttribute('data-preview', 'true');

  // A second single click on a *different* file replaces the preview slot's own tab — still two
  // tabs, never three (§5.2 rule 3).
  await treeRow(page, 'b.ts').click();
  await expect(repoTab(page)).toHaveCount(2);
  await expect(repoTab(page, 'repo-file')).toHaveCount(1);
  await expect(repoTab(page, 'repo-file')).toHaveAttribute('data-preview', 'true');
  await expect(repoTab(page, 'repo-file')).toContainText('b.ts');

  // Double-click promotes the current preview tab to permanent (§5.2's own promotion trigger).
  await treeRow(page, 'b.ts').dblclick();
  await expect(repoTab(page, 'repo-file')).toHaveAttribute('data-preview', 'false');
  await expect(repoTab(page)).toHaveCount(2); // still pinned + the one (now permanent) file tab.
});

// C6 §10/§16: the diff tab's own vocabulary and a real createDiffEditor mount under WebKit — the
// one thing neither typecheck nor a Go test can reach. The diff algorithm itself is Monaco's, so
// this asserts only that "Open changes" opens a second, permanent tab with the diff host rendered.
test('a repo workspace: "Open changes" opens a diff tab', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
      {
        channel: IPC.codeWorkspaceListFiles,
        args: { id: REPO.id },
        response: FILE_LISTING_WITH_STATUS,
      },
      DIFF_SNAP,
    ],
  });

  await repoRowById(page, REPO.id).click();
  await page.locator('[data-testid="git-panel-tab-files"]').click();
  await expect(treeRow(page, 'c.ts')).toBeVisible();

  await treeRow(page, 'c.ts').click({ button: 'right' });
  const menu = page.locator('[data-testid="context-menu"]');
  await expect(menu).toBeVisible();
  await menu.locator('[data-testid="menu-item-open-changes"]').click();

  // A permanent tab (never the preview slot) — the pinned graph tab plus this one, two total.
  await expect(repoTab(page)).toHaveCount(2);
  const diffTab = repoTab(page, 'repo-diff');
  await expect(diffTab).toHaveCount(1);
  await expect(diffTab).toHaveAttribute('data-preview', 'false');
  await expect(diffTab).toContainText('c.ts (Working Tree)');
  await expect(page.locator('[data-testid="repo-diff-editor"]')).toBeVisible();

  // A row with no status glyph gets no "Open changes" entry at all (§8.4's own gate).
  await page.keyboard.press('Escape');
  await treeRow(page, 'a.ts').click({ button: 'right' });
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await expect(page.locator('[data-testid="menu-item-open-changes"]')).toHaveCount(0);
});

// The one UI case a Go test or typecheck can't reach — the CHANNEL.codeSearch
// subscription, the out-of-order merge-by-path, and the result-click-to-preview-tab path.
// Everything upstream of the event stream (the scanner itself, the coalescer) has its own
// coverage (search_test.go, the Go package's own layering); this only exercises what those can't.
test('a repo workspace: search streams results out of order and opens a match', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
      { channel: IPC.codeWorkspaceListFiles, args: { id: REPO.id }, response: FILE_LISTING },
      readFileSnap('a.ts', 'export const a = 1;\n'),
      { channel: IPC.codeWorkspaceStartSearch, response: { searchId: 'search-1' }, hold: true },
    ],
  });

  await repoRowById(page, REPO.id).click();
  await page.locator('[data-testid="git-panel-tab-files"]').click();
  await page.locator('[data-testid="repo-view-search"]').click();

  const queryInput = page.locator('[data-testid="repo-search-query"]');
  await queryInput.fill('const');
  await queryInput.press('Enter');

  // 'b.ts' arrives first — enumeration order, not path order — but the store's own binary insert
  // (D11) must still land it after 'a.ts' once both are in. The start's reply is held until after
  // this first batch, pinning the order that used to drop it: event before reply.
  await emitWailsEvent(page, IPC.codeSearch, {
    searchId: 'search-1',
    seq: 0,
    files: [
      {
        path: 'b.ts',
        matches: [
          {
            line: 1,
            column: 8,
            endColumn: 13,
            preview: 'export const b = 2;',
            previewMatchStart: 7,
            previewMatchEnd: 12,
            truncatedStart: false,
            truncatedEnd: false,
          },
        ],
        truncated: false,
      },
    ],
    done: false,
  });
  control.release(IPC.codeWorkspaceStartSearch);
  await expect(page.locator('[data-testid="repo-search-file-row"]')).toHaveCount(1);

  await emitWailsEvent(page, IPC.codeSearch, {
    searchId: 'search-1',
    seq: 1,
    files: [
      {
        path: 'a.ts',
        matches: [
          {
            line: 1,
            column: 8,
            endColumn: 13,
            preview: 'export const a = 1;',
            previewMatchStart: 7,
            previewMatchEnd: 12,
            truncatedStart: false,
            truncatedEnd: false,
          },
        ],
        truncated: false,
      },
    ],
    done: true,
    stats: { filesScanned: 2, filesMatched: 2, filesSkipped: 0, matches: 2, truncated: false },
  });

  const fileRows = page.locator('[data-testid="repo-search-file-row"]');
  await expect(fileRows).toHaveCount(2);
  await expect(fileRows.nth(0)).toHaveAttribute('data-path', 'a.ts');
  await expect(fileRows.nth(1)).toHaveAttribute('data-path', 'b.ts');

  await expect(page.locator('[data-testid="repo-search-status"]')).toContainText(
    '2 results in 2 files',
  );

  // A single click on a match row opens a preview tab (§7.3's own onSelect/onOpen split).
  await page.locator('[data-testid="repo-search-match-row"][data-path="a.ts"]').click();
  const previewTab = repoTab(page, 'repo-file');
  await expect(previewTab).toHaveCount(1);
  await expect(previewTab).toHaveAttribute('data-preview', 'true');
  await expect(previewTab).toContainText('a.ts');
});

// Deliberately shallow: the deep review behaviour (a real branch comparison, marking, comments)
// needs a real repository and is the manual recipe's job (docs/HISTORY.md step 7), not a UI spec.
// This asserts only that switching the panel to Review mounts the real @kira/git-ui ReviewView.vue
// bundle over the native git stream and renders something real (its own branch picker) rather than
// an empty container — the one thing neither typecheck nor a Go test can reach for this phase,
// the same role the diff-tab/search/quick-open specs above already play.
test('a repo workspace: switching the panel to Review mounts the review sidebar', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL });
  // Must land before the repo workspace ever opens (ensureWorkspaceShell mounts the pinned graph
  // tab immediately, which is what first calls Stream('gitgraph') — repo/git/transport.ts's
  // gitTransportFor is lazy, unlike bridge/port.ts's own module-scope Stream('engine'), so
  // page.evaluate (not page.addInitScript) is both sufficient and correct here.
  // The transport opens the repo before the first request that names it.
  await installGitStreamMock(page, REPO.repoId, { 'repo.open': null });

  await repoRowById(page, REPO.id).click();
  // P92 item 6: Review moved from the Files body's own segment to GitPanel's top-level tab row.
  await page.locator('[data-testid="git-panel-tab-review"]').click();

  await expect(page.locator('[data-testid="repo-review-host"]')).toBeVisible();
  // The "no branch chosen yet" picker — ReviewView.vue's own branch selector — with a real branch
  // row from the mocked refs.list, not an empty container.
  const picker = page.locator('[data-testid="review-no-branch"]');
  await expect(picker).toBeVisible();
  await expect(picker.getByText('main', { exact: true })).toBeVisible();
});

// Review is a top-level tab alongside Repos/Files, so all three read as peers — this asserts that
// structure directly (labels, order, and that the Files body's own inner strip is back down to two
// segments) rather than only the "Review mounts the review sidebar" behaviour the test above
// already covers.
test('a repo workspace: the panel has three top-level tabs (Repos/Files/Review), and the Files tab keeps its own two-segment Files/Search strip', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await installGitStreamMock(page, REPO.repoId);

  await repoRowById(page, REPO.id).click();

  const topTabs = page.locator(
    '[data-testid="git-panel-tab-repos"], [data-testid="git-panel-tab-files"], [data-testid="git-panel-tab-review"]',
  );
  await expect(topTabs).toHaveCount(3);
  const labels = await topTabs.allTextContents();
  expect(labels.map((l) => l.trim())).toEqual(['Repositories', 'Files', 'Review']);

  // Files tab: its own inner strip is Files/Search only — Review is no longer one of its values.
  await page.locator('[data-testid="git-panel-tab-files"]').click();
  await expect(page.locator('[data-testid="git-panel-tab-files"]')).toHaveClass(/on/);
  const innerStrip = page.locator(
    '[data-testid="repo-view-files"], [data-testid="repo-view-search"]',
  );
  await expect(innerStrip).toHaveCount(2);
  expect((await innerStrip.allTextContents()).map((l) => l.trim())).toEqual(['Files', 'Search']);

  // Review shows the real ReviewView.vue bundle, not an empty container.
  await page.locator('[data-testid="git-panel-tab-review"]').click();
  await expect(page.locator('[data-testid="repo-review-host"]')).toBeVisible();
  await expect(page.locator('[data-testid="repo-view-files"]')).toHaveCount(0);
});

// Deliberately shallow: tests/ui/ has no git-stream mock that can answer blame.line, so asserting
// a real blame answer would need a fixture that is its own piece of work. What this tier CAN
// assert: the gitRepoIdFor === undefined guard is honest (no crash, no annotation) rather than
// assumed, and toggling the setting leaves the editor mounted and healthy either way.
test('a repo workspace: the blame annotation stays off with no git record, and its setting toggles safely', async ({
  relaunch,
}) => {
  // A repository this window has no git record for — repoId '' (never populated), the same "no
  // git identification" shape a plain-folder import would produce. gitRepoIdFor(codeRepoId) reads
  // this back as falsy, exactly the guard RepoFileView.vue/blameAnnotation.ts checks.
  const NO_GIT_REPO = { ...REPO, id: 'repo-no-git', repoId: '' };
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.codeWorkspaceListRepos, response: [NO_GIT_REPO] },
      {
        channel: IPC.codeWorkspaceListFiles,
        args: { id: NO_GIT_REPO.id },
        response: FILE_LISTING,
      },
      {
        channel: IPC.codeWorkspaceReadFile,
        args: { id: NO_GIT_REPO.id, path: 'a.ts' },
        response: {
          kind: 'found',
          text: 'export const a = 1;\n',
          bytes: 21,
          limitBytes: 8 * 1024 * 1024,
          language: 'typescript',
        },
      },
      { channel: IPC.settingsSet, response: defaultSettings },
    ],
  });

  const noGitRepoRow = page.locator(`[data-testid="repo-row"][data-repo-id="${NO_GIT_REPO.id}"]`);
  await noGitRepoRow.click();
  await page.locator('[data-testid="git-panel-tab-files"]').click();
  await expect(treeRow(page, 'a.ts')).toBeVisible();
  await treeRow(page, 'a.ts').click();

  const editor = page.locator('[data-testid="repo-file-editor"]');
  await expect(editor).toBeVisible();
  // §4.1's guard is honest, not merely absent because nothing ever requested a blame line: no
  // annotation renders at all.
  await expect(page.locator('.kira-blame-inline')).toHaveCount(0);

  // Toggling the setting off and back on, then saving, doesn't disturb the still-mounted editor —
  // the live watch (RepoFileView.vue) and the dispose/reattach cycle both run cleanly with no git
  // record behind them.
  await page.click('[data-testid="open-settings"]');
  await expect(page.locator('[data-testid="settings-dialog"]')).toBeVisible();
  await page.click('[data-testid="settings-inline-blame"]');
  await page.click('[data-testid="settings-inline-blame"]');
  await page.click('[data-testid="settings-save"]');
  await expect(page.locator('[data-testid="settings-dialog"]')).toHaveCount(0);

  await expect(editor).toBeVisible();
  await expect(page.locator('.kira-blame-inline')).toHaveCount(0);
});

// The status bar's own blame item, plus the revision-pinned guard proven the same
// way the test above proves its guard — a resolvable trap, not an absent one. Both `repo.open` and
// `blame.line` answer in each relaunch below; if `blameable` ever again omitted `rev === null`
// (the fix), the revision-pinned half would call them too and the item would wrongly appear.
test('contract: a repo workspace: the status bar blame item follows the cursor, and never shows on a revision-pinned tab (P76)', async ({
  relaunch,
}) => {
  const BLAME_RESULT = {
    ...contract<{ author: string; summary: string }>('git-blame', 'git:blame.line#line-2'),
    sha: 'a'.repeat(40),
    authorTimeSeconds: 1_700_000_000,
  };

  {
    const { window: page } = await relaunch({ control: CONTROL });
    // See :364's own comment: lazy `gitTransportFor`, so this lands before Stream('gitgraph') is ever
    // called as long as it precedes the click that opens the workspace.
    await installGitStreamMock(page, REPO.repoId, {
      'repo.open': undefined,
      'blame.line': BLAME_RESULT,
    });

    await repoRowById(page, REPO.id).click();
    await page.locator('[data-testid="git-panel-tab-files"]').click();
    await treeRow(page, 'a.ts').click();

    const editor = page.locator('[data-testid="repo-file-editor"]');
    await expect(editor).toBeVisible();
    const blameStatus = page.locator('[data-testid="blame-status"]');
    await expect(blameStatus).toHaveCount(0); // nothing resolved until the cursor actually moves

    await editor.locator('.view-lines').click();
    await page.keyboard.press('ArrowDown');

    await expect(blameStatus).toBeVisible();
    await expect(blameStatus).toContainText(BLAME_RESULT.author);
    await expect(blameStatus).toContainText(BLAME_RESULT.summary);
    await blameStatus.hover();
    await expect(page.locator('[data-testid="tooltip-details-title"]')).toHaveText(
      BLAME_RESULT.summary,
    );
    await expect(
      page.locator('[data-testid="tooltip-details-row"][data-label="Author"]'),
    ).toContainText(BLAME_RESULT.author);
  }

  {
    // A revision-pinned repo-file tab (`rev` set), seeded directly as a restored tab — the real
    // navigation paths to one (a commit's "Go to file", a diff tab's own go-to-file command) both
    // need `graph.stream`/`file.goToTarget`, which this mock deliberately never answers.
    // Marking it `active` here (unlike the restored-tab test above, which leaves its tab inactive
    // to exercise ensureWorkspaceShell's own fallback) means it renders as soon as the workspace
    // itself activates, with no tree click needed.
    //
    // Unlike the first half, boot itself opens the git transport here — main.ts's own
    // post-hydrate fall-forward mounts this active tab's RepoFileView before app.mount, so before
    // any click is even possible. The mock must exist before that first page script runs, which
    // `installGitStreamMock`'s `page.evaluate` (installed after `relaunch()` resolves) is too late
    // for. Use `relaunch`'s own `gitStream` option (`page.addInitScript`) instead. `'repo.open':
    // null`, not `undefined` — `addInitScript` JSON-serializes its args, and an `undefined` value
    // would drop the key and hang `repo.open` forever (see gitStreamMock.ts's own doc comment).
    const REV = 'b'.repeat(40);
    const { window: page } = await relaunch({
      control: [
        { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
        { channel: IPC.codeWorkspaceListFiles, args: { id: REPO.id }, response: FILE_LISTING },
        {
          channel: IPC.tabsList,
          response: [
            {
              id: 'restored-repo-graph',
              kind: 'repo-graph',
              connectionId: null,
              path: '',
              order: 0,
              active: false,
              // state/workspace.ts's own WorkspaceKey is the bare code_repos id, not a `repo:`
              // prefixed one (Studio's own format, never adopted here).
              workspaceId: REPO.id,
              state: { viewState: null, reviewSession: null },
            },
            {
              id: 'restored-repo-file-rev',
              kind: 'repo-file',
              connectionId: null,
              path: 'a.ts',
              order: 1,
              active: true,
              workspaceId: REPO.id,
              state: { revealLine: null, markdownReading: false, rev: REV },
            },
          ],
        },
      ],
      gitStream: {
        repoId: REPO.repoId,
        extraResults: {
          'repo.open': null,
          'blame.line': BLAME_RESULT,
          // Two lines, so a click plus ArrowDown below moves to a real second line.
          'file.read': { kind: 'found', content: 'export const a = 1;\nexport const b = 2;\n' },
        },
      },
    });

    // Content only renders from state = 'found', which RepoFileView.vue's mount() reaches only
    // after synchronously evaluating the `blameable` guard (`rev === null`) earlier in the same
    // function — so this assertion (unlike a bare toBeVisible(), which an empty 'loading' host
    // would also satisfy) proves the guard ran.
    const editor = page.locator('[data-testid="repo-file-editor"]');
    await expect(editor.locator('.view-lines')).toContainText('export const a = 1;');
    await expect(page.locator('[data-testid="blame-status"]')).toHaveCount(0);

    // Arm the trap the same way the first half does: move the cursor, so a regressed `blameable`
    // guard (one that dropped `rev === null`) would actually call `blame.line` and show the item,
    // instead of this half passing vacuously because the cursor never moved.
    await editor.locator('.view-lines').click();
    await page.keyboard.press('ArrowDown');

    // Bounded negative window: >3x blameLine.ts's 150ms DEBOUNCE_MS plus the mock's own
    // setTimeout(0) round trips. A guard regression has no event this test could otherwise wait
    // on, and toHaveCount(0) alone would pass at t=0, before a regressed controller even fires.
    await page.waitForTimeout(500);

    const requests = await gitStreamRequests(page);
    expect(requests).toContain('file.read'); // the mock was live
    expect(requests).not.toContain('blame.line'); // the guard held
    await expect(page.locator('[data-testid="blame-status"]')).toHaveCount(0);
  }
});

// P67b §4.2/§9: closing the active repo workspace from its row menu falls back to the empty state
// (GitStart.vue) — "falls back" just means no repo is active any more within the Git module, not a
// switch to another mode: the mode switcher (P128 §2.6) stays on `git` throughout this test.
test('closing the active repo workspace from its row menu falls back to the empty state', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL });

  await repoRowById(page, REPO.id).click();
  await expect(repoTab(page, 'repo-graph')).toHaveCount(1);

  // P82: the row's hover × is gone — closing is the row menu's job now.
  await expect(page.locator('[data-testid="workspace-repo-close"]')).toHaveCount(0);

  // P84 §8.3: the panel auto-switched to Files when the workspace opened — switch back to reach
  // the repo row's own context menu.
  await page.locator('[data-testid="git-panel-tab-repos"]').click();
  await repoRowById(page, REPO.id).click({ button: 'right' });
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await page.locator('[data-testid="menu-item-close"]').click();

  await expect(page.locator('[data-testid="git-start"]')).toBeVisible();
  await expect(repoTab(page, 'repo-graph')).toHaveCount(0);
});

// Expanding a row lists its worktrees without opening it (the twisty's own
// @click.stop), and clicking a linked worktree switches to it — importing it as its own
// code_repos row first, since this app has none for that root yet — the same premise
// WorktreeList.vue's own switch rests on, not a second worktree-switching path.
test('contract: a repo workspace: expanding a row lists its worktrees, and switching to one opens its own workspace', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [
      ...CONTROL,
      {
        channel: IPC.codeWorkspaceImportRepo,
        args: { path: WORKTREE_PATH },
        response: WORKTREE_REPO,
      },
      {
        channel: IPC.codeWorkspaceListFiles,
        args: { id: WORKTREE_REPO.id },
        response: FILE_LISTING,
      },
    ],
  });

  // Installed before the expanding click — repo/git/transport.ts's gitTransportFor is lazy, so
  // Stream('gitgraph') is first called only once the row actually expands (repo-workspace.spec.ts:485's
  // own comment gives the identical reason).
  await installGitStreamMock(page, REPO.repoId, {
    'repo.open': undefined,
    'worktree.list': WORKTREE_LIST_RESULT,
  });

  await repoRowById(page, REPO.id).locator('[data-testid="repo-row-expand"]').click();

  // Expanding must not open the workspace — the assertion that guards the twisty's own @click.stop.
  await expect(repoTab(page, 'repo-graph')).toHaveCount(0);

  const worktreeRows = page.locator('[data-testid="repo-worktree-row"]');
  await expect(worktreeRows).toHaveCount(2);
  const linkedRow = page.locator(
    `[data-testid="repo-worktree-row"][data-worktree-path="${WORKTREE_PATH}"]`,
  );
  await expect(linkedRow).toContainText('wt-branch');

  await linkedRow.click();

  // The switch actually happened, not just that a call was made.
  expect(
    control
      .log()
      .some(
        (e) =>
          e.channel === IPC.codeWorkspaceImportRepo &&
          JSON.stringify(e.args) === JSON.stringify({ path: WORKTREE_PATH }),
      ),
  ).toBe(true);

  // P84: the opened worktree renders nowhere at the top level — only nested under its parent's
  // twisty. Without §4.5's click-time hint this would be racy-then-green (the row appears for one
  // round trip before the batched RepoWorktreeLinks answer removes it); toHaveCount(0) catches that.
  const flatDuplicateRow = page.locator(
    `[data-testid="repo-row"][data-repo-id="${WORKTREE_REPO.id}"]`,
  );
  await expect(flatDuplicateRow).toHaveCount(0);

  // P84 §8.3: switching opened the workspace, which flipped the panel to Files — switch back to
  // see the nested row's own active marking.
  await page.locator('[data-testid="git-panel-tab-repos"]').click();
  await expect(linkedRow).toHaveClass(/active/);
  await expect(repoTab(page, 'repo-graph')).toHaveCount(1);

  // Collapsing again drops the worktree list.
  await repoRowById(page, REPO.id).locator('[data-testid="repo-row-expand"]').click();
  await expect(page.locator('[data-testid="repo-worktrees"]')).toHaveCount(0);
});

// The hydration path — a worktree imported in an *earlier* session has both rows already
// in codeWorkspaceListRepos on boot, with no click to supply the hint. This is what proves the
// batched RepoWorktreeLinks call itself is wired: the test would pass even if it were never
// called, since the click-time hint alone is enough there.
test('a worktree imported in an earlier session lists only under its parent', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.codeWorkspaceListRepos, response: [REPO, WORKTREE_REPO] },
      {
        channel: IPC.codeWorkspaceRepoWorktreeLinks,
        response: [
          { id: REPO.id, parentId: '' },
          { id: WORKTREE_REPO.id, parentId: REPO.id },
        ],
      },
    ],
  });

  await installGitStreamMock(page, REPO.repoId, {
    'repo.open': undefined,
    'worktree.list': WORKTREE_LIST_RESULT,
  });

  // Exactly one top-level row — the anchor. The worktree's own row never renders flat.
  await expect(page.locator('[data-testid="repo-row"]')).toHaveCount(1);
  await expect(repoRowById(page, REPO.id)).toHaveCount(1);

  await repoRowById(page, REPO.id).locator('[data-testid="repo-row-expand"]').click();
  const worktreeRows = page.locator('[data-testid="repo-worktree-row"]');
  await expect(worktreeRows).toHaveCount(2);
  // Main worktree first (worktrees.ts:44-52's own sort).
  await expect(worktreeRows.first()).toContainText('main');

  // §6.2's reachability guarantee: the nested row still offers Rename/Close/Remove, the thing that
  // makes hiding its flat duplicate safe.
  const linkedRow = page.locator(
    `[data-testid="repo-worktree-row"][data-worktree-path="${WORKTREE_PATH}"]`,
  );
  await linkedRow.click({ button: 'right' });
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await expect(page.locator('[data-testid="menu-item-remove"]')).toBeVisible();
});

// A .ts row and a .go row carry different per-language icon styles (ported from the
// diff tree's own setiIconFor rule, repo/fileIcon.ts), and a directory row still carries a real
// codicon (folder/folder-opened) — no directory-icon rule was ported, since the diff
// tree has none.
test('a repo workspace: file-tree rows carry per-language icons, directories keep their codicon', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
      {
        channel: IPC.codeWorkspaceListFiles,
        args: { id: REPO.id },
        response: {
          paths: ['main.go', 'app.ts', 'src/util.ts'],
          status: {},
          truncated: false,
        },
      },
      readFileSnap('main.go', 'package main\n'),
    ],
  });

  await repoRowById(page, REPO.id).click();
  await page.locator('[data-testid="git-panel-tab-files"]').click();

  const goIcon = treeRow(page, 'main.go').locator('.node-icon');
  const tsIcon = treeRow(page, 'app.ts').locator('.node-icon');
  await expect(goIcon).toBeVisible();
  await expect(tsIcon).toBeVisible();

  const [goStyle, tsStyle] = await Promise.all([
    goIcon.getAttribute('style'),
    tsIcon.getAttribute('style'),
  ]);
  expect(goStyle).toContain('mask-image');
  expect(tsStyle).toContain('mask-image');
  expect(goStyle).not.toBe(tsStyle);

  // The directory row synthesized from 'src/util.ts' keeps a real codicon glyph — not a mask icon.
  const dirRow = treeRow(page, 'src');
  await expect(dirRow.locator('.codicon-folder, .codicon-folder-opened')).toHaveCount(1);
  await expect(dirRow.locator('.node-icon[style*="mask-image"]')).toHaveCount(0);

  // P73 §2: opening main.go carries the same seti icon on its tab as on its tree row.
  await treeRow(page, 'main.go').click();
  const tabFileIcon = repoTab(page, 'repo-file').locator('.tab-file-icon');
  await expect(tabFileIcon).toBeVisible();
  expect(goStyle).not.toBeNull();
  await expect(tabFileIcon).toHaveAttribute('style', goStyle as string);
});

// terminalId is a client-generated UUID (state/tabs.ts's own newUuid()) — unpredictable
// ahead of time, so this carries no `args` at all and relies on mockRuntime.ts's own single-
// snapshot shortcut ("a channel called with the same args every time … answers regardless of the
// exact args it was called with"). Without a real answer here, the mocked call 422s and
// openTerminalSession's own catch marks the session 'failed' — terminalCountAtPath excludes that
// status, so the indicator test below would flip invisible again once the call resolves.
const TERMINAL_OPEN_OK: ControlSnapshot = {
  channel: IPC.terminalOpen,
  response: { shell: '/bin/zsh' },
};

// The embedded terminal's own wiring — real shell I/O is out of scope for this
// tier (the stated boundary; internal/terminal/session_test.go covers that). This is a
// wiring test, not a call-count test: it proves subscribe -> decode -> term.write -> DOM by
// rendering a synthesized `kira:terminal:data` chunk inside the real xterm.js DOM it mounts.
//
// The "+" opens a terminal directly, no dropdown — TabStrip.vue's own onNewTab calls
// openRepoTerminalTab unconditionally; Studio's own launch-kind menu (Terminal/Claude Code/a
// configured script) was never ported (no AgentSessions/CustomScripts store here, main.ts's own
// doc comment).
test("the tab strip's + opens a terminal tab at the active repository's root", async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: [...CONTROL, TERMINAL_OPEN_OK] });

  await repoRowById(page, REPO.id).click();

  await page.locator('[data-testid="tab-strip-new"]').click();

  const terminalTab = repoTab(page, 'terminal');
  await expect(terminalTab).toHaveCount(1);
  const terminalId = await terminalTab.getAttribute('data-tab-id');
  expect(terminalId).not.toBeNull();

  // .xterm-rows appearing proves the dynamic import of terminalRenderer.ts settled and xterm
  // mounted; the terminalOpen call itself is fire-and-forget from the shared terminal host's own
  // mount() (the one component every terminal tab, repo-scoped or module-scoped, now
  // renders through), so a plain synchronous check can race its still-in-flight mocked round trip
  // — expect.poll rather than a fixed wait.
  await expect(page.locator('.xterm-rows')).toBeVisible();

  await expect
    .poll(() =>
      control
        .log()
        .some(
          (e) =>
            e.channel === IPC.terminalOpen &&
            (e.args as { cwd?: string } | undefined)?.cwd === REPO.root,
        ),
    )
    .toBe(true);

  await emitWailsEvent(page, IPC.terminal, {
    terminalId,
    data: Buffer.from('hello\r\n').toString('base64'),
    exited: false,
  });
  await expect(page.locator('.xterm-rows')).toContainText('hello');
});

test('Shift+Enter in a terminal sends meta-Enter (ESC CR); Cmd+Enter and plain Enter stay CR', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [...CONTROL, TERMINAL_OPEN_OK, { channel: IPC.terminalWrite, response: null }],
  });

  await repoRowById(page, REPO.id).click();
  await page.locator('[data-testid="tab-strip-new"]').click();
  await expect(page.locator('.xterm-rows')).toBeVisible();
  await page.locator('.xterm-helper-textarea').focus();

  const writes = () =>
    control
      .log()
      .filter((e) => e.channel === IPC.terminalWrite)
      .map((e) => (e.args as { data?: string } | undefined)?.data);

  await page.keyboard.press('Meta+Enter');
  await expect.poll(writes).toEqual(['DQ==']);

  await page.keyboard.press('Shift+Enter');
  await expect.poll(writes).toEqual(['DQ==', 'Gw0=']);

  await page.keyboard.press('Enter');
  await expect.poll(writes).toEqual(['DQ==', 'Gw0=', 'DQ==']);
});

// A worktree row's own context menu (nothing needed one until a terminal could be opened at a
// worktree that isn't the active workspace) plus the indicator's own row-specificity: it must key
// off the terminal's cwd, not merely "a terminal is open somewhere in this repository".
test("a worktree row's menu opens a terminal there, and both rows show the indicator", async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: [...CONTROL, TERMINAL_OPEN_OK] });

  // Installed before the expanding click — gitTransportFor is lazy (repo-workspace.spec.ts's own
  // comment on the identical setup above).
  await installGitStreamMock(page, REPO.repoId, {
    'repo.open': undefined,
    'worktree.list': WORKTREE_LIST_RESULT,
  });

  await repoRowById(page, REPO.id).locator('[data-testid="repo-row-expand"]').click();

  const worktreeRows = page.locator('[data-testid="repo-worktree-row"]');
  await expect(worktreeRows).toHaveCount(2);
  const linkedRow = page.locator(
    `[data-testid="repo-worktree-row"][data-worktree-path="${WORKTREE_PATH}"]`,
  );
  const mainRow = page.locator(
    `[data-testid="repo-worktree-row"][data-worktree-path="${REPO.repoId}"]`,
  );

  await linkedRow.click({ button: 'right' });
  const menu = page.locator('[data-testid="context-menu"]');
  await expect(menu).toBeVisible();
  await menu.locator('[data-testid="menu-item-open-terminal"]').click();

  await expect
    .poll(() =>
      control
        .log()
        .some(
          (e) =>
            e.channel === IPC.terminalOpen &&
            (e.args as { cwd?: string } | undefined)?.cwd === WORKTREE_PATH,
        ),
    )
    .toBe(true);

  // openRepoTerminalTab opens repo.id's own workspace as a side effect (it calls
  // openRepoWorkspace), so the auto-switch already flipped the panel to Files — switch
  // back to see the worktree rows again (the same migration applies at :652/:673).
  await page.locator('[data-testid="git-panel-tab-repos"]').click();

  // Both row kinds share the same indicator markup (data-testid="repo-terminal-indicator",
  // GitPanel.vue) — this proves it lands only on the row whose own path matches the terminal's
  // cwd, on neither the sibling worktree row nor the collapsed repo row above them.
  await expect(linkedRow.locator('[data-testid="repo-terminal-indicator"]')).toHaveCount(1);
  await expect(mainRow.locator('[data-testid="repo-terminal-indicator"]')).toHaveCount(0);
  await expect(
    repoRowById(page, REPO.id).locator('[data-testid="repo-terminal-indicator"]'),
  ).toHaveCount(0);
});

// P83 §12/§13: a repo row's checked-out branch (RepoHeads, batched) and §13.2's sort, proven
// together — the mock deliberately answers worktree.list linked-first, so a first-row assertion
// that only ever matched the server's own order would pass vacuously here.
test('every repo row shows its checked-out branch, main worktree first', async ({ relaunch }) => {
  const REPO2 = {
    id: 'repo-2',
    name: 'demo-repo-2',
    root: '/tmp/demo-repo-2',
    repoId: '/tmp/demo-repo-2',
    sortOrder: 2,
    color: 'none',
    createdAt: '2026-01-03T00:00:00.000Z',
  };
  const REPO2_SHA = `abc1234${'0'.repeat(33)}`;

  const { window: page } = await relaunch({
    control: [
      { channel: IPC.codeWorkspaceListRepos, response: [REPO, REPO2] },
      {
        channel: IPC.codeWorkspaceRepoHeads,
        args: { ids: null },
        response: [
          { id: REPO.id, head: { kind: 'branch', name: 'main' } },
          { id: REPO2.id, head: { kind: 'detached', sha: REPO2_SHA } },
        ],
      },
    ],
  });

  // worktree.list answers linked-first, deliberately — §13.2's sort must reorder it, not merely
  // pass through the server's own order.
  await installGitStreamMock(page, REPO.repoId, {
    'repo.open': undefined,
    'worktree.list': {
      worktrees: [
        {
          path: WORKTREE_PATH,
          head: '1'.repeat(40),
          branch: 'refs/heads/feature',
          isBare: false,
          isDetached: false,
          isMain: false,
          isCurrent: false,
          locked: null,
          prunable: null,
          openElsewhere: false,
        },
        {
          path: REPO.repoId,
          head: '0'.repeat(40),
          branch: 'refs/heads/main',
          isBare: false,
          isDetached: false,
          isMain: true,
          isCurrent: true,
          locked: null,
          prunable: null,
          openElsewhere: false,
        },
      ],
    },
  });

  await expect(repoRowById(page, REPO.id).locator('.repo-head')).toHaveText('main');
  const repo2Row = page.locator(`[data-testid="repo-row"][data-repo-id="${REPO2.id}"]`);
  await expect(repo2Row.locator('.repo-head')).toHaveText('Detached at abc1234');

  await repoRowById(page, REPO.id).locator('[data-testid="repo-row-expand"]').click();
  const worktreeRows = page.locator('[data-testid="repo-worktree-row"]');
  await expect(worktreeRows).toHaveCount(2);
  await expect(worktreeRows.first()).toHaveAttribute('data-worktree-path', REPO.repoId);
  await expect(worktreeRows.first().locator('.worktree-badge')).toHaveText('Main');
});

// "Open all changes" (CommitMeta.vue) on a multi-file commit opens exactly one
// repo-multi-diff tab, not one repo-diff tab per file — hostHandlers.ts's own 'editor
// .openAllChanges' handler, driven end to end through a real graph.stream chunk (no prior UI test
// has driven this method through gitStreamMock.ts — see graphStreamFixture.ts's own doc comment
// for why this needs real FlatBuffers encoding, not a hand-rolled one). `file.read` is
// deliberately left unanswered: RepoMultiDiffView.vue's own section headers (`.path`/`.path-dir`)
// render from the tab's own file list unconditionally, with no dependency on a section's diff
// editor ever finishing its load (RepoMultiDiffView.vue's own template) — that's what this test
// asserts, not diff content.
test('a repo workspace: "Open all changes" on a commit opens one multi-diff tab listing every changed path', async ({
  relaunch,
}) => {
  const SHA = 'a'.repeat(40);
  const { window: page } = await relaunch({ control: CONTROL });

  const commitDetail = {
    sha: SHA,
    parents: [],
    author: { name: 'Ada Lovelace', email: 'ada@example.com', timestamp: 1_700_000_000 },
    committer: { name: 'Ada Lovelace', email: 'ada@example.com', timestamp: 1_700_000_000 },
    subject: 'Change two files',
    body: '',
    trailers: [],
    signature: { status: 'N', signer: '' },
    decoration: [],
    parentIndex: 0,
    files: [
      {
        kind: 'modified',
        path: 'a.ts',
        originalPath: undefined,
        similarity: undefined,
      },
      {
        kind: 'modified',
        path: 'b.ts',
        originalPath: undefined,
        similarity: undefined,
      },
    ],
  };

  await installGitStreamMock(
    page,
    REPO.repoId,
    {
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
      'commit.detail': commitDetail,
    },
    [buildGraphStreamChunk(REPO.repoId, 0, buildOneCommitChunk(SHA, 'Change two files'))],
  );

  await repoRowById(page, REPO.id).click();
  await expect(page.locator('[data-testid="repo-graph-host"]')).toBeVisible();

  const row = page.locator('[data-testid="commit-grid"] .slick-row[data-row="0"]');
  await expect(row).toBeVisible();
  await row.click();
  const openAllChanges = page.locator('[data-testid="open-all-changes-button"]');
  await expect(openAllChanges).toBeVisible();

  await expect(repoTab(page, 'repo-multi-diff')).toHaveCount(0);
  await openAllChanges.click();
  await expect(repoTab(page, 'repo-multi-diff')).toHaveCount(1);

  const multiDiffView = page.locator('[data-testid="repo-multi-diff-view"]');
  await expect(multiDiffView).toBeVisible();
  const paths = await multiDiffView.locator('.path-dir').allTextContents();
  expect(paths).toEqual(['a.ts', 'b.ts']);
});

// P121 §6.2: T1's own `pt-0.5` removal applies through the shared TabStrip.vue -- both the pinned
// graph tab and a promoted file tab should centre in the 34px `h-tabbar` bar, same as Studio's own
// T1 assertion.
test('the pinned graph tab and a promoted file tab sit vertically centred in the tab-strip bar (P121 T1)', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await repoRowById(page, REPO.id).click();
  await page.locator('[data-testid="git-panel-tab-repos"]').click();

  const bar = page.locator('[data-testid="tab-strip"]');

  async function assertCentred(tabLocator: import('@playwright/test').Locator) {
    const [barBox, tabBox] = await Promise.all([bar.boundingBox(), tabLocator.boundingBox()]);
    if (!barBox || !tabBox) throw new Error('expected both the bar and its tab to be measurable');
    const barBottomInterior = barBox.y + barBox.height;
    const topGap = tabBox.y - barBox.y;
    const bottomGap = barBottomInterior - (tabBox.y + tabBox.height);
    expect(Math.abs(topGap - bottomGap)).toBeLessThanOrEqual(0.5);
  }

  await assertCentred(repoTab(page, 'repo-graph'));

  await page.locator('[data-testid="git-panel-tab-files"]').click();
  await treeRow(page, 'a.ts').click();
  await assertCentred(repoTab(page, 'repo-file'));
});

// P121 §6.2: the Git panel's own top-level tabs (S1, a default-variant ToggleGroup) get the same
// 4px full-corner radius as every other default-variant instance; its label keeps normal letter
// spacing, same as the Files/Search segmented control beside it (S2, unaffected by this phase).
test('the Git panel top-level tabs render at 4px radius with normal letter spacing (P121 S1)', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await repoRowById(page, REPO.id).click();
  await page.locator('[data-testid="git-panel-tab-repos"]').click();

  const repos = page.locator('[data-testid="git-panel-tab-repos"]');
  expect(await repos.evaluate((el) => getComputedStyle(el).borderRadius)).toBe('4px');
  const reposSpacing = await repos.evaluate((el) => getComputedStyle(el).letterSpacing);

  await page.locator('[data-testid="git-panel-tab-files"]').click();
  const filesView = page.locator('[data-testid="repo-view-files"]');
  const filesSpacing = await filesView.evaluate((el) => getComputedStyle(el).letterSpacing);

  expect(reposSpacing).toBe('normal');
  expect(filesSpacing).toBe('normal');
});

// The desktop app's own DOM-level check for the collapse feature —
// `repo-graph-branch-order.spec.ts` already covers the toolbar toggle, the
// fork lanes and the no-overlap/no-scrollbar cases in detail; this only exercises what is
// specific to the desktop host: `RepoGraphView.vue`'s kept-mounted model (a tab switch away and back
// keeps the mounted `git-ui` app alive, hidden with v-show, `MainView.vue`) and `revealAndSelectSha`'s auto-expand-on-search-reveal path
// (`App.vue:490`). `buildMultiBranchChunk`/`buildMultiBranchRefsList` (`graphStreamFixture.ts`)
// port `fakeGraphHost.ts`'s own fixture table — see that file's doc comment for the row/parent
// layout: `main` (HEAD, 3 commits, never collapses), `feature-newer` (5 commits, collapses by
// default), `feature-older` (2 commits, always full). Display follows store order.
// Collapsed-by-default: `F0, [placeholder], G0, G1, M0, M1, M2` (7 rows, row 6 last); fully
// expanded: 10 rows (row 9 last).
test.describe('a repo workspace: graph branch collapse (P93 §8.4)', () => {
  async function bootMultiBranchGraph(
    page: import('@playwright/test').Page,
    extraResults?: Record<string, unknown>,
  ): Promise<void> {
    await installGitStreamMock(
      page,
      REPO.repoId,
      {
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
        'refs.list': buildMultiBranchRefsList(),
        ...extraResults,
      },
      [buildGraphStreamChunk(REPO.repoId, 0, buildMultiBranchChunk())],
    );
    await repoRowById(page, REPO.id).click();
    await expect(page.locator('[data-testid="repo-graph-host"]')).toBeVisible();
    await expect(
      page.locator('[data-testid="commit-grid"] .slick-row[data-row="6"]'),
    ).toBeVisible();
  }

  function messageCell(page: import('@playwright/test').Page, row: number) {
    return page
      .locator(`[data-testid="commit-grid"] .slick-row[data-row="${row}"] .kira-cell-message`)
      .first();
  }

  test('the default view collapses feature-newer, main and feature-older render in full', async ({
    relaunch,
  }) => {
    const { window: page } = await relaunch({ control: CONTROL });
    await bootMultiBranchGraph(page);

    await expect(messageCell(page, 4)).toContainText('main tip (M0)');
    const placeholder = messageCell(page, 1);
    await expect(placeholder).toHaveAttribute('data-testid', 'graph-collapsed-row');
    await expect(placeholder).toContainText(
      `${MULTI_BRANCH_FEATURE_NEWER_HIDDEN_COUNT} more commits on ${MULTI_BRANCH_FEATURE_NEWER_NAME}`,
    );
    await expect(page.locator('[data-testid="commit-grid"] .slick-row[data-row="7"]')).toHaveCount(
      0,
    );
  });

  // MainView keeps RepoGraphView mounted (hidden with v-show) while a sibling tab is active, so
  // in-memory state (a manually expanded group; the toolbar's own collapse-off choice) survives
  // a switch away and back without `PersistedViewState`.
  test('a manual expand and the toolbar collapse-off choice both survive a tab switch and back', async ({
    relaunch,
  }) => {
    const { window: page } = await relaunch({ control: CONTROL });
    await bootMultiBranchGraph(page);

    await messageCell(page, 1).click(); // expand feature-newer
    await expect(
      page.locator('[data-testid="commit-grid"] .slick-row[data-row="9"]'),
    ).toBeVisible();
    await expect(page.locator('[data-testid="graph-collapse-toggle"]')).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    await page.locator('[data-testid="graph-collapse-toggle"]').click(); // collapse-off
    await expect(page.locator('[data-testid="graph-collapse-toggle"]')).toHaveAttribute(
      'aria-pressed',
      'false',
    );

    // Switch to the Files tab (a real, unkept tab) and back to the pinned graph tab.
    await page.locator('[data-testid="git-panel-tab-files"]').click();
    await expect(treeRow(page, 'a.ts')).toBeVisible();
    await treeRow(page, 'a.ts').click();
    await expect(repoTab(page, 'repo-file')).toHaveCount(1);
    await repoTab(page, 'repo-graph').click();

    await expect(
      page.locator('[data-testid="commit-grid"] .slick-row[data-row="9"]'),
    ).toBeVisible();
    await expect(page.locator('[data-testid="graph-collapsed-row"]')).toHaveCount(0);
    await expect(page.locator('[data-testid="graph-collapse-toggle"]')).toHaveAttribute(
      'aria-pressed',
      'false',
    );
  });

  // App.vue's `revealAndSelectSha` (search.ts's `activeHit` watcher) auto-expands a collapsed
  // group before scrolling — `CommitGrid.vue`'s own `scrollToRow` doc comment. `'F2'` is a unique
  // substring of exactly one commit ('feature-newer F2', row 2 once expanded, hidden inside the
  // placeholder while collapsed) — the loaded-only scan (`searchLoadedCommits`) is synchronous
  // over this fixture's one, fully-exhausted chunk, so no `search.run` tail request is needed.
  test('a search reveal into a collapsed group expands it and selects the hit', async ({
    relaunch,
  }) => {
    const { window: page } = await relaunch({ control: CONTROL });
    await bootMultiBranchGraph(page);

    await expect(page.locator('[data-testid="graph-collapsed-row"]')).toHaveCount(1);

    await page.locator('[data-testid="search-toggle-button"]').click();
    // `[data-testid="search-input"]` lands on `KuiSearchInput.vue`'s own root `<div>` (Vue's
    // attrs fallthrough) — the real `<input>` is nested one level inside it.
    const searchInput = page.locator('[data-testid="search-input"] input');
    await expect(searchInput).toBeVisible();
    await searchInput.fill('F2');
    await expect(page.locator('[data-testid="search-count"]')).toBeVisible();
    await searchInput.press('Enter');

    await expect(page.locator('[data-testid="graph-collapsed-row"]')).toHaveCount(0);
    // Expanded order: F0, F1, F2, F3, F4, G0, G1, M0, M1, M2 — 'F2' is row 2.
    const targetRow = page.locator('[data-testid="commit-grid"] .slick-row[data-row="2"]');
    await expect(targetRow).toContainText('feature-newer F2');
    await expect(targetRow).toBeVisible();
    await expect(targetRow).toHaveClass(/kira-row-selected/);
  });
});

test('tab strip: drag reorders file tabs; the pinned graph tab never moves (P137)', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await repoRowById(page, REPO.id).click();
  await page.locator('[data-testid="git-panel-tab-files"]').click();
  await treeRow(page, 'a.ts').click();
  await treeRow(page, 'a.ts').dblclick();
  await treeRow(page, 'b.ts').click();
  await treeRow(page, 'b.ts').dblclick();

  const row = page.locator('[data-testid="tab-strip-row"] [data-testid="tab"]');
  await expect(row).toHaveCount(2);
  const rowOrder = () => row.evaluateAll((els) => els.map((el) => el.textContent?.trim() ?? ''));
  await expect
    .poll(rowOrder)
    .toEqual([expect.stringContaining('a.ts'), expect.stringContaining('b.ts')]);
  const graphChip = page.locator('[data-testid="tab-strip-pinned"] [data-testid="tab"]');
  await expect(graphChip).toHaveCount(1);

  await dragTab(page, row.nth(1), row.nth(0));
  await expect
    .poll(rowOrder)
    .toEqual([expect.stringContaining('b.ts'), expect.stringContaining('a.ts')]);

  // A drag started on the pinned chip moves nothing; a file tab dragged over it never enters or
  // displaces the pinned slot.
  await dragTab(page, graphChip, row.nth(1));
  await expect
    .poll(rowOrder)
    .toEqual([expect.stringContaining('b.ts'), expect.stringContaining('a.ts')]);
  await dragTab(page, row.nth(1), graphChip);
  await expect(row).toHaveCount(2);
  await expect(graphChip).toHaveCount(1);
  await expect(graphChip).toHaveAttribute('data-tab-kind', 'repo-graph');
  await expect(
    page.locator('[data-testid="tab-strip-row"] [data-tab-kind="repo-graph"]'),
  ).toHaveCount(0);
});
