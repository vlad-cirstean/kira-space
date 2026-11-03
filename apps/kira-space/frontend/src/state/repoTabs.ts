import { t } from '@theme/i18n';
import { type RevealRequest, requestReveal } from '../views/repo/reveal';
import {
  asRepoDiffTab,
  asRepoFileTab,
  defaultLocalFileTabState,
  defaultRepoDiffTabState,
  defaultRepoFileTabState,
  defaultRepoMultiDiffTabState,
  type ReviewRef,
} from './tabDomain';
import { type OpenTabResult, tabsForWorkspace, useTabsStore } from './tabs';
import { openTerminalTab, type TerminalLaunch } from './terminalTabs';
import { GENERAL_WORKSPACE, repoWorkspaceKey, useWorkspaceStore } from './workspace';

// P91 §6: moved to state/terminalTabs.ts, which openTerminalTab below now shares with the
// Automations module's own (non-repo-scoped) opener — re-exported so no importer of this module
// breaks.
export type { TerminalLaunch };

export interface OpenRepoFileOpts {
  preview: boolean;
  // C7 D12/§7.4: column/endColumn widen the reveal beyond repoFileTabStateSchema's own persisted
  // `revealLine` — a search result (or a go-to-definition match) carries a column worth restoring
  // a cursor to within this session, not across a restart.
  reveal?: { line: number; column?: number; endColumn?: number };
  /** Non-null opens `path` read-only at that revision instead of the worktree file —
   *  part of this tab's own identity (below), so two different revisions of the same path are two
   *  different tabs, never one silently replacing the other. `undefined`/`null` both mean the
   *  worktree file. */
  rev?: string | null;
}

// The repo workspace's own file-open entry point — every tree row click and every
// search/quick-open match routes through this, never openTab directly, so the preview/pin
// rules stay in exactly one place (openTab itself, the unit-tested mechanism).
//
// Does its own existing-tab lookup rather than opening through `openTab`'s `reuse: true`
// branch — `openTab`'s own dedupe key is (workspaceId, kind, connectionId, path), which has no
// room for `rev`; without this, opening the same path at two different revisions would collide
// into one tab (the identical gap `openRepoCommitDiffTab`'s own doc comment names, one kind over).
export function openRepoFileTab(
  repoId: string,
  path: string,
  opts: OpenRepoFileOpts,
): OpenTabResult {
  const tabsStore = useTabsStore();
  const revealLine = opts.reveal?.line ?? null;
  const rev = opts.rev ?? null;
  const workspaceId = repoWorkspaceKey(repoId);
  const existing = tabsStore.tabs.find((t) => {
    if ((t.workspaceId ?? null) !== workspaceId || t.path !== path) return false;
    const file = asRepoFileTab(t);
    return file !== null && file.state.rev === rev;
  });
  let result: OpenTabResult;
  if (existing) {
    tabsStore.activateTab(existing.id);
    // §5.2 rule 1's own "a permanent open promotes the workspace's current preview tab" —
    // `openTab`'s own reuse branch does this; this wrapper's own reuse path needs the identical
    // rule since it never reaches openTab's (same reasoning as openRepoCommitDiffTab's own).
    if (!opts.preview) tabsStore.removeFromPreviewCohort(workspaceId, existing.id);
    result = { id: existing.id, reused: true };
  } else {
    result = tabsStore.openTab(
      'repo-file',
      null,
      path,
      () => defaultRepoFileTabState(revealLine, rev),
      {
        reuse: false,
        workspaceId,
        preview: opts.preview,
      },
    );
  }
  // §5.2 rule 1: "Apply reveal either way" — a fresh tab's makeState() already carries it, so this
  // only does real work for a reused tab (and is a same-value no-op, via skipUnchanged, otherwise).
  if (revealLine !== null) tabsStore.patchRepoFileTabState(result.id, { revealLine });
  // D12: requestReveal is what actually moves the cursor when the tab's editor is already mounted
  // and active (a reused tab this call didn't just create) — patchRepoFileTabState above only ever
  // updates persisted state, which RepoFileView.vue reads on mount, not on an existing mount.
  if (opts.reveal) requestReveal(result.id, opts.reveal);
  return result;
}

// A file opened from Finder that sits outside every imported repository: a read-only tab in the
// general workspace, deduped by absolute path.
export function openLocalFileTab(path: string, reveal?: RevealRequest): OpenTabResult {
  const tabsStore = useTabsStore();
  const revealLine = reveal?.line ?? null;
  const result = tabsStore.openTab(
    'local-file',
    null,
    path,
    () => ({ ...defaultLocalFileTabState(), revealLine }),
    { reuse: true, workspaceId: GENERAL_WORKSPACE, preview: false },
  );
  if (reveal) {
    if (result.reused) tabsStore.patchLocalFileTabState(result.id, { revealLine: reveal.line });
    requestReveal(result.id, reveal);
  }
  return result;
}

// "Open changes" opens a permanent tab, never the preview slot — a single tree click
// already owns the preview slot for the file viewer, so this context-menu action behaves like the
// menu's existing "Open" (permanent). openTab's dedupe key is (workspaceId, kind, connectionId,
// path), so a diff tab and a file tab for the same path coexist, and a second "Open changes"
// activates the existing one rather than opening a duplicate.
export function openRepoDiffTab(repoId: string, path: string): OpenTabResult {
  return useTabsStore().openTab('repo-diff', null, path, defaultRepoDiffTabState, {
    reuse: true,
    workspaceId: repoWorkspaceKey(repoId),
    preview: false,
  });
}

// openRepoCommitDiffTab/openRepoReviewDiffTab share this reuse-or-open frame — find an
// existing tab over the same (path, left, right, review-or-not) key, replay the preview-cohort
// fix on a hit, else delegate to openTab with reuse:false so the dedupe key (workspaceId,
// kind, connectionId, path) never collides two different revision pairs of the same file.
// `review: null` selects openRepoCommitDiffTab's own commit-diff match/build; non-null selects
// openRepoReviewDiffTab's review-diff ones — the two are otherwise identical.
//
// Reuse branches skip openTab, so they replicate its cohort rules: a pinned open promotes the
// tab out of the cohort; a preview open of a current preview member makes it the sole member; a
// permanent tab is only activated, never demoted into the cohort.
function reuseOrOpenRepoDiffTab(opts: {
  repoId: string;
  path: string;
  left: string;
  right: string;
  labels: { left: string; right: string };
  review: ReviewRef | null;
  pinned: boolean;
  // P74 §5.2: set only by a bulk caller ("Open all changes") — every file after the first in that
  // same loop, so the cohort it started is joined rather than each file evicting the last.
  previewCohort?: boolean;
}): OpenTabResult {
  const tabsStore = useTabsStore();
  const workspaceId = repoWorkspaceKey(opts.repoId);
  const existing = tabsStore.tabs.find((t) => {
    if ((t.workspaceId ?? null) !== workspaceId || t.path !== opts.path) return false;
    const diff = asRepoDiffTab(t);
    if (diff === null || diff.state.left !== opts.left || diff.state.right !== opts.right) {
      return false;
    }
    // C11 §7.5: without matching on review too, a single-commit branch whose merge base equals
    // the commit's own parent would have the identical (left, right) pair as its review diff and
    // reuse that tab, rendering no review layer at all.
    return opts.review === null
      ? diff.state.review === null
      : diff.state.review !== null && diff.state.review.branch === opts.review.branch;
  });
  if (existing) {
    if (opts.pinned) {
      // §5.2 rule 1's own "a permanent open promotes the workspace's current preview tab" —
      // openTab's own reuse branch does this; this wrapper's own reuse path needs the identical
      // rule since it never reaches openTab's.
      tabsStore.removeFromPreviewCohort(workspaceId, existing.id);
    } else if (!opts.previewCohort && tabsStore.isPreview(existing.id)) {
      // A permanent tab reused by a preview open stays permanent: only a current preview member
      // becomes the sole cohort member. Ordered before activateTab so its saveNow() persists this.
      tabsStore.evictPreviewCohort(workspaceId, existing.id);
    }
    tabsStore.activateTab(existing.id);
    return { id: existing.id, reused: true };
  }
  return tabsStore.openTab(
    'repo-diff',
    null,
    opts.path,
    () =>
      defaultRepoDiffTabState(
        {
          left: opts.left,
          right: opts.right,
          leftLabel: opts.labels.left,
          rightLabel: opts.labels.right,
        },
        opts.review ?? undefined,
      ),
    { reuse: false, workspaceId, preview: !opts.pinned, previewCohort: opts.previewCohort },
  );
}

// C10 §6.1 (S15): a *commit* diff — two revisions of one path, neither of which is the worktree.
export function openRepoCommitDiffTab(
  repoId: string,
  path: string,
  left: string,
  right: string,
  labels: { left: string; right: string },
  pinned: boolean,
  previewCohort?: boolean,
): OpenTabResult {
  return reuseOrOpenRepoDiffTab({
    repoId,
    path,
    left,
    right,
    labels,
    review: null,
    pinned,
    previewCohort,
  });
}

// One commit's whole changed-file set in one tab — `editor.openAllChanges`'s
// own replacement for its old one-`repo-diff`-tab-per-file loop. Unlike openRepoCommitDiffTab
// above, the dedupe key needs no extra revision-pair lookup of its own: `right` (the commit sha)
// IS the tab's `path`, already unique per commit, so openTab's own `reuse: true` is enough — a
// second "See commit changes" on the same commit reuses the tab, a different commit opens a
// second one.
export function openRepoMultiDiffTab(
  repoId: string,
  files: string[],
  left: string,
  right: string,
  labels: { left: string; right: string },
  review?: ReviewRef,
): OpenTabResult {
  return useTabsStore().openTab(
    'repo-multi-diff',
    null,
    right,
    () =>
      defaultRepoMultiDiffTabState(
        files,
        { left, right, leftLabel: labels.left, rightLabel: labels.right },
        review,
      ),
    { reuse: true, workspaceId: repoWorkspaceKey(repoId), preview: false },
  );
}

// The review-diff counterpart to openRepoCommitDiffTab above — same shape.
// `review` turns on `reviewDecorations.ts`'s comment/mark layer over the same left/right
// pair; `left` is the merge base or `reviewedAtSha` (the two diff modes), `right` is always
// `review.branchTip`.
export function openRepoReviewDiffTab(
  repoId: string,
  path: string,
  left: string,
  right: string,
  labels: { left: string; right: string },
  review: ReviewRef,
  pinned: boolean,
  // P74 §5.2: mirrors openRepoCommitDiffTab's own trailing param — see its doc comment.
  previewCohort?: boolean,
): OpenTabResult {
  return reuseOrOpenRepoDiffTab({
    repoId,
    path,
    left,
    right,
    labels,
    review,
    pinned,
    previewCohort,
  });
}

// An uncommitted file of a task branch's worktree, for the review window. Keyed by (path, branch):
// the same path in another branch's worktree is another tab. `left`/`right` are model keys only.
export function openRepoWorkingReviewTab(
  repoId: string,
  path: string,
  branchId: string,
  pinned: boolean,
): OpenTabResult {
  const tabsStore = useTabsStore();
  const workspaceId = repoWorkspaceKey(repoId);
  const existing = tabsStore.tabs.find(
    (t) =>
      (t.workspaceId ?? null) === workspaceId &&
      t.path === path &&
      asRepoDiffTab(t)?.state.working?.branchId === branchId,
  );
  if (existing) {
    if (pinned) tabsStore.removeFromPreviewCohort(workspaceId, existing.id);
    tabsStore.activateTab(existing.id);
    return { id: existing.id, reused: true };
  }
  return tabsStore.openTab(
    'repo-diff',
    null,
    path,
    () =>
      defaultRepoDiffTabState(
        {
          left: `working-head:${branchId}`,
          right: `working:${branchId}`,
          leftLabel: 'HEAD',
          rightLabel: t('space.ade.review.working.right.label'),
        },
        undefined,
        { branchId },
      ),
    { reuse: false, workspaceId, preview: !pinned },
  );
}

// Opens a terminal tab in codeRepoId's own workspace, rooted at `cwd` — the tab-strip
// "+" (active workspace's root) and both GitPanel.vue row menus (a repo's root or a worktree's own
// path) all funnel through this. openRepoWorkspace runs first: a tab must belong to a workspace
// whose strip is on screen, and right-clicking a closed repository's row would otherwise open an
// invisible tab. `launch`, when given, seeds the session's initial command/title/rail
// colour — omitted, this is the plain terminal, unchanged.
//
// Delegates to state/terminalTabs.ts's openTerminalTab for the actual tab-open — this
// function's own remaining job is exactly "which workspace does the tab belong to", the
// openRepoWorkspace side effect that the Automations module's own opener must NOT have.
export function openRepoTerminalTab(
  codeRepoId: string,
  cwd: string,
  launch?: TerminalLaunch,
): OpenTabResult {
  useWorkspaceStore().openRepoWorkspace(codeRepoId);
  return openTerminalTab({ workspaceId: repoWorkspaceKey(codeRepoId), cwd, codeRepoId, launch });
}

// Creates repoId's own pinned graph tab if it has none, and activates it only when the
// workspace currently has no active tab at all — called by openRepoWorkspace (state/workspace.ts)
// and once per restored repo after hydrateTabs (main.ts's own bootstrap), never from inside
// hydrateTabs itself (state/tabs.ts already depends on this module for closeWorkspaceTabs's own
// call graph; hydrateTabs calling back in here would add a third link to that cycle for no need).
export function ensureWorkspaceShell(repoId: string): void {
  const tabsStore = useTabsStore();
  const key = repoWorkspaceKey(repoId);
  const tabs = tabsForWorkspace(key);
  if (!tabs.some((t) => t.kind === 'repo-graph')) {
    tabsStore.createPinnedRepoGraphTab(key);
  }
  if (!tabs.some((t) => t.active)) {
    const graph = tabsForWorkspace(key).find((t) => t.kind === 'repo-graph');
    if (graph) tabsStore.activateTab(graph.id);
  }
}
