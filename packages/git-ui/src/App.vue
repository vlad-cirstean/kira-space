<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { hostWords, type MessageParams, t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
/**
 * The real shell. `AppToolbar.vue`/`CommitGrid.vue`/`LoadMoreButton.vue` are wired together here
 * for the first time; everything above this file in the dependency order was deliberately dead
 * code in the production bundle until now.
 *
 * The live-data strip and its `data-testid`s are deleted, not hidden, except one: `chunk-source`
 * stays on the list region, because it is a real field of the stream chunk with no other
 * visible surface — everything else the strip showed now has a real UI equivalent (the repo
 * picker's own label, the rendered rows themselves).
 */

import type { CommitRecord, FileChangeKind, TipRef } from '@kira/git-core';
import { describeInProgress, SETTINGS } from '@kira/git-core';
import type { StashEntry, Transport, UiActionKind } from '@kira/git-ipc';
import { TransportError } from '@kira/git-ipc';
import { Button } from '@theme/components/ui/button';
import { Popover, PopoverAnchor, PopoverContent } from '@theme/components/ui/popover';
import { ResizeHandle } from '@theme/components/ui/resizable';
import {
  onClickOutside,
  useDebounceFn,
  useDocumentVisibility,
  useEventListener,
  useResizeObserver,
  useWindowFocus,
} from '@vueuse/core';
import { useLiveRegion } from '@workbench/behaviour/liveRegion';
import { compareNatural } from '@workbench/behaviour/match';
import { TIMING } from '@workbench/behaviour/timing';
import { useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue';
import { BridgeClient } from './bridge/client.ts';
// A .vue default export is a *value* — the component object the template instantiates. `import
// type` erases it, and Vue then renders <CommitGrid> as an unknown element with no grid inside it.
// The script's only reference is `InstanceType<typeof …>`, so biome's useImportType
// cannot tell (it is off for `**/*.vue` in `biome.json` for exactly this class of false positive);
// the template is the real caller.
import AppToolbar from './components/AppToolbar.vue';
import CommitGrid from './components/CommitGrid.vue';
import ConflictBanner from './components/ConflictBanner.vue';
import DetailPane from './components/DetailPane.vue';
import BranchDialog from './components/dialogs/BranchDialog.vue';
import CheckoutDialog from './components/dialogs/CheckoutDialog.vue';
import CherryPickDialog from './components/dialogs/CherryPickDialog.vue';
import ForcePushDialog from './components/dialogs/ForcePushDialog.vue';
import PostCheckoutPullDialog from './components/dialogs/PostCheckoutPullDialog.vue';
import PullDialog from './components/dialogs/PullDialog.vue';
import RenameRefDialog from './components/dialogs/RenameRefDialog.vue';
import RepoSettingsDialog from './components/dialogs/RepoSettingsDialog.vue';
import ResetDialog from './components/dialogs/ResetDialog.vue';
import RevertDialog from './components/dialogs/RevertDialog.vue';
import StackDialog from './components/dialogs/StackDialog.vue';
import StashDialog from './components/dialogs/StashDialog.vue';
import TagDialog from './components/dialogs/TagDialog.vue';
import WorktreeDialog from './components/dialogs/WorktreeDialog.vue';
import EmptyRepositoryPanel from './components/EmptyRepositoryPanel.vue';
import FailureBanner from './components/FailureBanner.vue';
import GitBlockedPanel from './components/GitBlockedPanel.vue';
import GitViewHead, { type RepoHead } from './components/GitViewHead.vue';
import LoadMoreButton from './components/LoadMoreButton.vue';
import NoRepositoryPanel from './components/NoRepositoryPanel.vue';
// The template instantiates this (InstanceType<typeof SearchBox> is the script's only other
// reference) — see AppToolbar.vue's own note on this exact pattern; `biome.json`'s own `**/*.vue`
// override turns `useImportType` off for exactly this class of false positive (P96 §5.2).
import SearchBox from './components/SearchBox.vue';
import StashDetailPane from './components/StashDetailPane.vue';
import UncommittedChangesStrip from './components/UncommittedChangesStrip.vue';
import WorkingDetailPane from './components/WorkingDetailPane.vue';
import { useGraphVisible } from './graphVisibility.ts';
import { localNameForRemoteBranch, remoteCheckoutTarget } from './lib/refListModel.ts';
import { buildRefMenu, buildRowMenu, buildStashMenu } from './lib/rowMenuModel.ts';
import type { SearchOption } from './lib/searchResultsModel.ts';
import { childOf, parentOf } from './lib/stackListModel.ts';
import { renderText, shaLabel } from './lib/text.ts';
import { useSectionMenu } from './lib/useSectionMenu.ts';
import { ASYNC_FAILURES, type AsyncFailure } from './state/asyncFailures.ts';
import { retryBootstrap as sharedRetryBootstrap } from './state/bootstrap.ts';
import { DetailState } from './state/detail.ts';
import { createDetailActions, type DetailActions } from './state/detailActions.ts';
import { composeAsyncFailureNotice, type FailureNotice } from './state/failureNotice.ts';
import { GraphOrderState } from './state/graphOrder.ts';
import { GraphViewState } from './state/graphView.ts';
import {
  composeLoadMoreAnnouncement,
  composeRefreshAnnouncement,
} from './state/liveAnnouncements.ts';
import { RepoState } from './state/repo.ts';
import { createRepoStates } from './state/repoStates.ts';
import { SearchState } from './state/search.ts';
import { SelectionState } from './state/selection.ts';
import { StashState } from './state/stash.ts';
import {
  type ColumnWidths,
  type DateFormat,
  DEFAULT_COLUMN_WIDTHS,
  DEFAULT_DETAIL_WIDTH,
  type PersistedViewState,
  type ViewStateStore,
} from './state/viewState.ts';
import { WorkingDetailState } from './state/working.ts';
import { type WorktreeCreateSeed, WorktreeState } from './state/worktrees.ts';

const props = defineProps<{
  transport: Transport;
  viewState: ViewStateStore;
  /** A palette command that fired while this webview was cold — see `main.ts`'s own
   *  `MountOptions.pendingUiAction` doc comment. `undefined`/`null` means none is pending.
   *  Mirrors `ui.action`'s own shape — this is the extension's own cold-boot document, not the
   *  wire. */
  pendingUiAction?: { action: UiActionKind; target?: { repoId: string; sha: string } } | null;
  /** P72 §9.1: Kira Space's own app-wide `appearance.dateFormat`, mount value, then live through
   *  `MountHandle.update` — see `main.ts`'s own `MountOptions.dateFormat`. Preferred over
   *  `app.init`'s `dateFormat` and the persisted value whenever present. */
  dateFormat?: DateFormat;
  /** P173: opens the host's Operations log (`MountOptions.onShowOperations`); absent where the host has none. */
  showOperations?: () => void;
  /** Repo identity for the Studio-style view head; absent hosts get no head. */
  repoHead?: RepoHead;
}>();

const bridge = new BridgeClient(props.transport);
const connectionState = bridge.connectionState;
// P93 §7: one `GraphOrderState` for the life of this component, exactly like `graphView` itself
// — threaded into it so `graphView.rebuildOrder()` has an order to rebuild against. Reset
// alongside `graphView.reset()` in `handleRepoOpened` below.
const graphOrder = new GraphOrderState();
const graphView = new GraphViewState(bridge, undefined, graphOrder);
// Hidden kept mount: no background re-walk or relayout until shown.
watch(useGraphVisible(), (visible) => graphView.setSuspended(!visible), { immediate: true });
// A fresh CommitStore for the life of this component (graphView is never swapped out from under
// it — a repo switch resets the same GraphViewState instance rather than replacing it, matching
// CommitGrid.vue's own documented assumption), so this can be constructed once, directly.
const selection = new SelectionState(graphView.store);
// One `DetailState` for the life of this component, exactly like
// `graphView`/`selection` above — a repo switch resets it (via `setRepoId` below) rather than
// replacing the instance. `actions` starts `undefined` and is built once, in `bootstrap()`, after
// `app.init` resolves — every template site that reads it is inside the same
// `v-if="repoState"`/`v-else` branches that only render once a repo has opened.
const detailState = new DetailState(bridge);
const actions = shallowRef<DetailActions | undefined>(undefined);

// One instance each of refs/pr/stack/repoSettings/ops for the life of this component, reset via
// `setRepoId` rather than replaced. `createRepoStates` wires `repoSettingsState` into `opsState`
// (checkout.autoStash) and `changeRequestState` into `stackState`.
const { refsState, changeRequestState, stackState, repoSettingsState, opsState } =
  createRepoStates(bridge);
// `docs/plans/P9.md` W13: one `StashState` for the life of this component, exactly like
// `refsState`/`opsState` above — reset via `setRepoId` rather than replaced.
const stashState = new StashState(bridge);
// One `WorkingDetailState` for the life of this component, exactly like `stashState` above — reset
// via `setRepoId` rather than replaced. Independent of `SelectionState` the same way `stashState`
// conceptually is (a stash reuses `SelectionState` only because it is a real graph row; the working
// tree is not one at all).
const workingState = new WorkingDetailState(bridge);
// G25 D1: one `WorktreeState` for the life of this component, exactly like `stashState` above —
// reset via `setRepoId` rather than replaced.
const worktreeState = new WorktreeState(bridge);
// One `SearchState` for the life of this component, exactly like
// `refsState`/`opsState`/`stashState` above — reset via `setRepoId` rather than replaced. Threads
// `refsState`/`graphView` in directly (both already exist above).
const searchState = new SearchState(bridge, refsState, graphView, changeRequestState);
// G18 D13: `repoSettingsState` (from `createRepoStates`) backs `RepoSettingsDialog.vue`.

const repoState = shallowRef<RepoState | undefined>(undefined);
// A failed bootstrap() used to leave repoState undefined forever — the whole template is
// v-if="repoState", so that rendered nothing at all. Set in the catch below, outside that
// v-if, with a Retry that clears it and re-runs bootstrap() — the ordinary case this guards is a
// panel opened before Kira Space's socket is even up, not a rare failure.
const bootError = ref<string | undefined>(undefined);

const detailOpen = ref(true);
const columnWidths = ref<ColumnWidths>(DEFAULT_COLUMN_WIDTHS);
// Drawn graph column width reported by CommitGrid; differs from the persisted one when lanes need more.
const gridGraphWidth = ref(DEFAULT_COLUMN_WIDTHS.graph);
const dateFormat = ref<DateFormat>(props.dateFormat ?? 'relative');
watch(
  () => props.dateFormat,
  (v) => {
    if (v) dateFormat.value = v;
  },
);
const detailWidth = ref(DEFAULT_DETAIL_WIDTH);
const scrollRow = ref(0);
/** G-UX D9: the graph search row's own open/closed state — closed by default, toggled by `/`/
 *  `Ctrl+F`/`Ctrl+Alt+F` (`toggleSearch` below) and persisted like `detailOpen`. */
const searchOpen = ref(false);
/** The collapse-by-default toggle (`AppToolbar.vue`'s own `graph-collapse-toggle`) —
 *  `true` by default (a fresh/discarded persisted blob), persisted like `searchOpen`. Reaches
 *  `graphOrder` (a plain, non-reactive class) through two explicit call sites below, never a
 *  reactive `watch`: the synchronous `graphOrder.setCollapseEnabled(...)` call right after
 *  `setCollapseBranches` applies this default at setup, and `bootstrap()`'s own persisted-restore
 *  applies a real value later — see each call site's own comment for why neither needs an
 *  `immediate: true` watch. */
const collapseBranches = ref(true);
/** First-mount-only rehydration target for `CommitGrid.vue`'s own `initialScrollRow` prop (see
 *  that component's doc comment on why it is one-shot) — `undefined` until `bootstrap()` reads a
 *  persisted value, so a first-ever mount (nothing persisted yet) passes nothing and scrolls
 *  nowhere in particular, which is correct: there is no prior position to restore. */
const initialScrollRow = ref<number | undefined>(undefined);

// `RepoSettingsState`'s own constructor seeds the schema's own default before any repo is open, so
// the `??` fallback below is defence in depth, not the primary path.
const FALLBACK_PAGE_SIZE = SETTINGS['kiraSpace.graph.pageSize'].default;

const pageSize = computed(
  () => repoSettingsState.settings.value['kiraSpace.graph.pageSize'] ?? FALLBACK_PAGE_SIZE,
);

/** `StashDialog.vue`'s create mode default — same "read the schema's own default as the fallback"
 *  shape as `pageSize` above. */
const FALLBACK_INCLUDE_UNTRACKED = SETTINGS['kiraSpace.stash.includeUntracked'].default;
const stashIncludeUntrackedDefault = computed(
  () =>
    repoSettingsState.settings.value['kiraSpace.stash.includeUntracked'] ??
    FALLBACK_INCLUDE_UNTRACKED,
);

/** `WorktreeDialog.vue`'s own path pre-fill default — same "read the schema's own default as the
 *  fallback" shape as `stashIncludeUntrackedDefault` above (G25 D10). */
const FALLBACK_WORKTREE_BASE_PATH = SETTINGS['kiraSpace.worktree.basePath'].default;
const worktreeBasePathDefault = computed(
  () =>
    repoSettingsState.settings.value['kiraSpace.worktree.basePath'] ?? FALLBACK_WORKTREE_BASE_PATH,
);
const commitGridRef = ref<InstanceType<typeof CommitGrid> | null>(null);
const toolbarRef = ref<InstanceType<typeof AppToolbar> | null>(null);

function triggerRefresh(): void {
  toolbarRef.value?.refresh();
}

async function handleRepoOpened(repoId: string): Promise<void> {
  // §6.2: switching repos resets GraphViewState, clears selection, and (via the persistence
  // watch below) writes the new repoId — a genuinely different repo has no sha/scroll position
  // worth re-resolving, unlike a refresh's re-walk of the *same* history.
  pendingSelectionSha.value = null;
  selection.clear();
  // A reveal-in-graph paging through the *old* repo's history must not keep running (or
  // land a stale `selection.select`/`scrollToRow`) once this repo switch has already reset the
  // store out from under it — `revealAndSelectSha`'s own cancel-and-restart shape, applied here
  // for the same reason.
  revealController?.abort();
  graphView.reset();
  graphOrder.reset();
  clearFailure();
  await graphView.openStream(repoId);
  // An open that succeeded disproves a boot-failure banner (a reconnect or a manual pick).
  bootError.value = undefined;
}

// ---------------------------------------------------------------------------------------
// Re-resolving selection by sha once a reset's rows are loaded again — shared by
// both the boot-time rehydration path (bootstrap(), below) and every later refresh
// (GraphViewState.generation bumps on every re-walk reset, the doc comment on `refresh`).
// A *speculative* SelectionState.selectBySha on every partial chunk would be wrong: on a miss it
// clears selection immediately (by design — see SelectionState's own doc comment), so calling it
// before the target's row has streamed back in would discard a selection that was
// still pending, not gone. `CommitStore.rowOfSha` is checked first, non-
// destructively, and the real (clearing-on-miss) call only happens once the row is either found
// or the stream is exhausted, at which point a miss is a real answer.
// ---------------------------------------------------------------------------------------
const pendingSelectionSha = ref<string | null>(null);

watch(graphView.generation, () => {
  const sha = selection.sha.value;
  pendingSelectionSha.value = sha;
  // A staged swap can leave `loadedRows` unchanged, which would never re-resolve the selection.
  resolvePendingSelection();
  // P108 F2: a re-walk (auto-refresh restart, repo switch, reconnect) invalidates any row a menu
  // captured before this point — close both context menus rather than let a select act on
  // whatever now happens to sit at the old row/sha.
  rowMenu.close();
});

const rowMenu = useSectionMenu();

const { text: liveAnnouncement, announce } = useLiveRegion();

/** `main.ts` sets no `app.config.errorHandler`, so a `void`-called (or unawaited)
 *  async operation that rejects becomes a silent, unhandled rejection — no feedback ever reaches
 *  the user. Every fire-and-forget call in this file routes its own rejection through this, into
 *  the same live region `detailState.announcement`/`opsState.announcement`/`graphView.announcement`
 *  below already drive. `transport-closed` (the one code `bridge.dispose()` itself produces, this
 *  file's own `onBeforeUnmount`) is ignored outright — this view is already gone by the time it
 *  lands, so there is nothing left to announce it to. */
function reportAsyncError(err: unknown, failure: AsyncFailure, params: MessageParams = {}): void {
  if (err instanceof TransportError && err.code === 'transport-closed') return;
  announce(t(failure.error, { ...params, error: errorText(err) }));
  // A cancel announces but never raises the banner; a graph stream failure already has its own
  // (with Retry) that this generic one would replace.
  if (err instanceof TransportError && err.code === 'cancelled') return;
  if (graphView.streamFailure.value !== undefined) return;
  showFailure(composeAsyncFailureNotice(t(failure.title, params), err), 'async');
}

// P173: the visible banner. One notice, latest wins; history lives in Kira Space's Operations log.
const failure = shallowRef<FailureNotice | undefined>(undefined);
let failureSource: 'ops' | 'stream' | 'async' | undefined;

function showFailure(notice: FailureNotice, source: 'ops' | 'stream' | 'async'): void {
  failure.value = notice;
  failureSource = source;
}

function clearFailure(source?: 'ops' | 'stream'): void {
  if (source !== undefined && failureSource !== source) return;
  failure.value = undefined;
  failureSource = undefined;
}

watch(opsState.lastFailure, (notice) =>
  notice ? showFailure(notice, 'ops') : clearFailure('ops'),
);
watch(graphView.streamFailure, (notice) =>
  notice ? showFailure(notice, 'stream') : clearFailure('stream'),
);

function retryGraph(): void {
  clearFailure();
  graphView.refresh().catch((err: unknown) => reportAsyncError(err, ASYNC_FAILURES.reloadGraph));
}

function resolvePendingSelection(): void {
  const sha = pendingSelectionSha.value;
  if (sha === null) return;
  const found = graphView.store.rowOfSha(sha) !== -1;
  if (!found && !graphView.exhausted.value) return;
  pendingSelectionSha.value = null;
  // P93 §8.4: `scrollToRow` is `async` now — this callback itself is not, and a page-arrival
  // re-select has nothing further to sequence after the scroll, so fire-and-forget is correct.
  if (selection.selectBySha(sha)) {
    commitGridRef.value
      ?.scrollToRow(selection.row.value)
      .catch((err: unknown) => reportAsyncError(err, ASYNC_FAILURES.scrollToSelected));
  }
}

watch(graphView.loadedRows, resolvePendingSelection);

// ---------------------------------------------------------------------------------------
// P93 §3.2: the priority-ordered tip list driving `graphOrder`'s branch grouping.
// ---------------------------------------------------------------------------------------

/** Checked-out branch (or detached HEAD) first, then every stash entry, then locals+remotes
 *  interleaved by `committerDate` descending, then tags likewise — `buildRowPlan`'s own
 *  `other` group (index `tips.length`) implicitly catches everything left over. Tip order only
 *  decides which group owns a shared commit (collapse labels and keys); it no longer orders
 *  the rows. Rebuilt whole
 *  on every relevant ref/stash change (the watch below) rather than diffed — cheap next to the
 *  relayout it drives, and `buildRowPlan` only cares about each tip's identity, not stability
 *  across rebuilds. */
function buildGraphTips(): TipRef[] {
  const tips: TipRef[] = [];
  const head = refsState.head.value;
  if (head?.kind === 'branch') {
    const branch = refsState.branches.value.find((row) => row.isHead);
    if (branch) {
      tips.push({ sha: branch.objectId, key: `branch:${branch.refname}`, label: branch.shortName });
    }
  } else if (head?.kind === 'detached') {
    tips.push({ sha: head.sha, key: `detached:${head.sha}`, label: shaLabel(head.sha) });
  }
  // `head.kind === 'unborn'` contributes no group 0 tip — no commit exists yet to be one.

  for (const entry of stashState.entries.value) {
    tips.push({ sha: entry.sha, key: `stash:${entry.index}`, label: `stash@{${entry.index}}` });
  }

  // Locals and remotes interleaved by `committerDate` descending; ties break on kind (branch
  // before remoteBranch) then refname ascending, per §3.2's table.
  const branchesAndRemotes = [...refsState.branches.value, ...refsState.remoteBranches.value]
    .filter((row) => !row.isHead)
    .sort(
      (a, b) =>
        b.committerDate - a.committerDate ||
        compareNatural(a.kind, b.kind) ||
        compareNatural(a.refname, b.refname),
    );
  for (const row of branchesAndRemotes) {
    tips.push({ sha: row.objectId, key: `ref:${row.refname}`, label: row.shortName });
  }

  const tags = [...refsState.tags.value].sort(
    (a, b) => b.committerDate - a.committerDate || compareNatural(a.refname, b.refname),
  );
  for (const row of tags) {
    tips.push({
      sha: row.peeledObjectId ?? row.objectId,
      key: `tag:${row.refname}`,
      label: row.shortName,
    });
  }

  return tips;
}

watch(
  [
    refsState.branches,
    refsState.remoteBranches,
    refsState.tags,
    refsState.head,
    stashState.entries,
  ],
  () => {
    graphOrder.setTips(buildGraphTips());
    void graphView.rebuildOrder();
  },
);

/** The toolbar's collapse-branches toggle — the only place `collapseBranches` writes
 *  through to `graphOrder` (a plain, non-reactive class) and asks for a relayout. `bootstrap()`'s
 *  own persisted-restore applies its value to `graphOrder` directly instead of through this
 *  function (see its own comment) — no relayout needed there, since the stream `graphView
 *  .openStream` starts right after already produces the first one. */
function setCollapseBranches(on: boolean): void {
  collapseBranches.value = on;
  graphOrder.setCollapseEnabled(on);
  void graphView.rebuildOrder();
}
// Applies collapseBranches's own default (true) to graphOrder up front — GraphOrderState's own
// field defaults to false (P93 §4.4's doc comment), and nothing else would set it before the
// first real rebuild otherwise. Plain, synchronous, no relayout: the store is still empty here.
graphOrder.setCollapseEnabled(collapseBranches.value);

// ---------------------------------------------------------------------------------------
// The "selection wiring": `DetailState` does not watch `SelectionState` itself (it is kept
// decoupled from it, matching `SelectionState`'s own doc comment on staying decoupled from
// `GraphViewState` — a component wires the two together, per that comment's own precedent), so
// this is that wiring. `selection.sha` only *changes* value (Vue's ref setter is a
// no-op on an unchanged primitive) on a genuine selection change — including to `null` on a
// clear — which is exactly `DetailState.select`'s own precondition ("callers only invoke it on
// an actual change"). A refresh that re-resolves the *same* sha via `selectBySha` above therefore
// never re-triggers this watch at all, which is how the "a refresh must not flicker the pane"
// requirement is satisfied — there is no special-case caching to write, the cache is never
// invalidated because nothing here re-requests when nothing has changed.
// ---------------------------------------------------------------------------------------
/** A stash node is selectable like a commit, but shows its OWN changes
 *  (`StashDetailPane.vue`/`StashState`), not a commit diff — so this watch
 *  first checks whether the newly selected sha's row carries a `"stash"` decoration and
 *  routes to `stashState.select` instead of `detailState.select` when it does, clearing whichever
 *  of the two panes did not win (mirroring `StashState`'s own doc comment: "the two selections are
 *  independent... `App.vue` decides which pane wins when both exist"). */
function isStashSha(sha: string): boolean {
  const row = graphView.store.rowOfSha(sha);
  if (row === -1) return false;
  return graphView.store.decorationAt(row).some((d) => d.kind === 'stash');
}

watch(
  () => selection.sha.value,
  (sha) => {
    if (sha !== null && isStashSha(sha)) {
      stashState.select(sha);
      detailState.select(null);
    } else {
      detailState.select(sha);
      stashState.select(null);
    }
    // P7 (item 2): selecting any real commit/stash row always clears a working-tree selection —
    // mirrors how selecting a commit already clears `stashState` above.
    workingState.select(false);
    // G24 D9: ChangeRequestState.select runs for every selection change, stash entries included — a stash
    // sha simply resolves to "no PR" (or `disabled`) same as any other commit the server has
    // never heard of as a PR head; no special-casing needed here.
    changeRequestState.select(sha);
  },
);

/** Extracted so the reconnect handler below (`handleReconnect`) can re-run this fan-out
 *  explicitly — a reconnect almost always re-opens the *same* `repoId`, so the `watch` below
 *  (keyed on `repoId` identity) never re-fires on its own, even though every one of these states
 *  needs to drop whatever it cached against the old, now-dead connection and re-fetch fresh. */
// `applyRepoIdToStates` below runs via `watch(..., { immediate: true })` — its very
// first call fires synchronously during this component's own setup, before the menu/dialog refs
// further down this file exist yet (a `<script setup>` block is one function body top to bottom;
// referencing a later `const` from an immediately-invoked callback is a genuine TDZ error, not
// just a logical no-op). Nothing needs closing on that first call anyway — every one of those
// refs is freshly created and already in its own default "closed" state — so this flag skips the
// clearing block only for that one call and runs it normally on every real repo switch/reconnect
// after.
let repoIdStatesAppliedOnce = false;

function applyRepoIdToStates(repoId: string | undefined): void {
  detailState.setRepoId(repoId);
  refsState.setRepoId(repoId);
  opsState.setRepoId(repoId);
  stashState.setRepoId(repoId);
  workingState.setRepoId(repoId);
  worktreeState.setRepoId(repoId);
  searchState.setRepoId(repoId);
  repoSettingsState.setRepoId(repoId);
  changeRequestState.setRepoId(repoId);
  // G26: after changeRequestState (StackState.reload calls changeRequestState.ensureSnapshot, F13).
  stackState.setRepoId(repoId);
  if (repoIdStatesAppliedOnce) {
    // The commit/stash context menus resolved against the *old* repo — a genuinely
    // different repo (or a reconnect that re-opens the same one against a fresh `Conn`) makes
    // a captured commit/stash entry stale; closing both here, at the one lifecycle point both a
    // repo switch and a reconnect always run through, catches a reconnect that never bumps
    // `graphView.generation` (the other closing point, above) on its own.
    rowMenu.close();
    // P108 F3: every dialog/menu ref below names a branch/tag/worktree/stash target (or an open
    // flag) from the *old* repo — same stale-ref hazard as the context menus above, same fix,
    // same lifecycle point.
    forceDeleteRefCandidate.value = undefined;
    tagDialogState.value = { open: false, target: '' };
    branchDialogState.value = { open: false, startPoint: '' };
    stashBranchTarget.value = undefined;
    stashCreateOpen.value = false;
    globalStashSaveOpen.value = false;
    globalStashSaveSourceEntry.value = undefined;
    repoSettingsDialogOpen.value = false;
    worktreeCreateRequest.value = undefined;
    stackDialogTarget.value = undefined;
    renameRefDialogState.value = { open: false, currentName: '' };
  }
  repoIdStatesAppliedOnce = true;
}

watch(() => repoState.value?.activeRepo.value?.repoId, applyRepoIdToStates, { immediate: true });

watch(detailState.announcement, announce, { deep: true });

watch(opsState.announcement, announce, { deep: true });

// Re-fetches the open working-tree pane's own file list on the same signal the
// strip's own count already reacts to (`OpsState.statusSummary`, refreshed on every
// `repo.changed`) — a no-op via `WorkingDetailState.refresh`'s own `selected` guard when the pane
// is not the one currently open.
watch(opsState.statusSummary, () => {
  workingState.refresh();
});

// `docs/plans/P11.md` W13/W14: `GraphViewState.revealSha`'s own progress text, forwarded into the
// shared live region exactly like `detailState.announcement`/`opsState.announcement` above.
watch(graphView.announcement, announce, { deep: true });

// ---------------------------------------------------------------------------------------
// `docs/plans/P11.md` W14: `SearchBox.vue`'s two emits, forwarded through `AppToolbar.vue`.
// ---------------------------------------------------------------------------------------
let revealController: AbortController | undefined;

/** "selecting a ref scrolls to and highlights the commit it points at" / "selecting a
 *  commit selects it in the graph" — both funnel through the same `GraphViewState.revealSha`,
 *  since a tail-only commit hit and every ref hit alike may point at a sha outside the
 *  currently loaded window. Supersedes any reveal already in flight — the same cancel-and-restart
 *  shape this file already uses elsewhere (`handleRepoOpened`'s own `pendingSelectionSha` reset,
 *  `GraphViewState`'s own controller swaps): revealing a second sha before the first one finished
 *  paging should abandon that first page-through, not race it. A `"notFound"`/`"cancelled"`
 *  outcome selects nothing — `revealSha`'s own doc comment covers why each is announced (or not)
 *  on its own. Shared by both `handleSearchSelect` (a dropdown pick) and the `activeHit` watcher
 *  below (`Enter`/`Shift+Enter` stepping through commit matches with no dropdown option
 *  highlighted). */
async function revealAndSelectSha(sha: string): Promise<void> {
  revealController?.abort();
  const controller = new AbortController();
  revealController = controller;
  const outcome = await graphView.revealSha(sha, controller.signal);
  if (outcome !== 'found') return;
  const row = graphView.store.rowOfSha(sha);
  if (row === -1) return; // defensive only — revealSha's own contract: "found" means row >= 0
  selection.select(row);
  // P93 §8.4: `scrollToRow` is `async` now — it may need to expand a collapsed group first
  // (`CommitGrid.vue`'s own doc comment) — awaited here so a search reveal genuinely lands on
  // the commit rather than racing the expand.
  await commitGridRef.value?.scrollToRow(row);
}

/** "Open in graph" from the review diff toolbar. Opens the target repo first when it is
 *  not already the active one — the same repo.open + handleRepoOpened path the repo picker uses
 *  — then reveals and selects the commit through the existing revealAndSelectSha above (which
 *  already pages until the sha appears, already announces progress, and already scrolls to it on
 *  a 'found' outcome). */
async function revealCommitInGraph(target: { repoId: string; sha: string }): Promise<void> {
  const repo = repoState.value;
  if (!repo) return;
  if (repo.activeRepo.value?.repoId !== target.repoId) {
    const outcome = await repo.open(target.repoId);
    if (outcome.kind !== 'ok') return;
    await handleRepoOpened(outcome.repo.repoId);
  }
  await revealAndSelectSha(target.sha);
}

/** An annotated tag's own commit is `peeledObjectId`, never `objectId` (§7.8: "for an annotated
 *  tag, the commit it dereferences to") — a lightweight tag or a branch carries no
 *  `peeledObjectId` at all, so `objectId` is what every other ref kind falls back to. */
async function handleSearchSelect(option: SearchOption): Promise<void> {
  const sha =
    option.kind === 'ref'
      ? (option.hit.ref.peeledObjectId ?? option.hit.ref.objectId)
      : option.hit.sha;
  await revealAndSelectSha(sha);
}

// `SearchState.next()`/`previous()` (`SearchBox.vue`'s `Enter`/`Shift+Enter`)
// only move `activeIndex` over `commitHits` — this watcher is the "consumer" `search.ts`'s own
// class doc comment describes ("a consumer watches `activeHit` and drives the reveal-and-select
// side effect"), reusing the exact same helper `handleSearchSelect` uses above. `undefined` means
// either nothing has been stepped to yet (`activeIndex` still `-1`) or `next()`/`previous()` was
// a no-op on an empty hit list — neither reveals anything.
watch(searchState.activeHit, (hit) => {
  if (hit === undefined) return;
  revealAndSelectSha(hit.sha).catch((err: unknown) =>
    reportAsyncError(err, ASYNC_FAILURES.revealSelected),
  );
});

function handleSearchFocusGrid(): void {
  // G-UX D9: SearchBox.vue's own Escape stage 2 (clear the query, focus the grid) now also
  // closes the search row -- focusGrid only ever fires from that one branch (never the
  // dropdown-dismiss stage), so tying the two together here needs no new emit.
  searchOpen.value = false;
  commitGridRef.value?.focusGrid();
}

const searchRowEl = ref<HTMLDivElement | null>(null);
const searchBoxRef = ref<InstanceType<typeof SearchBox> | null>(null);

/** G-UX D9: opens the search row and focuses it on the next tick (`SearchBox.vue`'s own exposed
 *  `focusInput()`) — a tick is needed since the row (and the input inside it) do not exist in the DOM
 *  until this reactive change renders. */
function openSearch(): void {
  searchOpen.value = true;
  void nextTick().then(() => searchBoxRef.value?.focusInput());
}

/** Closing clears the query (`SearchState.clear()`) — a hidden row holding a live query that
 *  still highlights rows in the grid would be a ghost — and hands focus back to the grid. */
function closeSearch(): void {
  searchOpen.value = false;
  searchState.clear();
  commitGridRef.value?.focusGrid();
}

function toggleSearchRow(): void {
  if (searchOpen.value) closeSearch();
  else openSearch();
}

// ---------------------------------------------------------------------------------------
// The per-commit context menu. `CommitGrid.vue` only ever reports which
// row and where to open it (own doc comment on its `contextMenu` emit) — this is the one place
// with both `opsState` and the commit store's own decorations at hand to build the menu itself.
// ---------------------------------------------------------------------------------------
// Captures the commit itself, not the row — an auto-refresh (restart-at-zero), a repo
// switch or a reconnect can land while this menu is open, and a store row is only ever a claim
// about "whichever commit sits there right now". `commit` is `CommitStore.commitAt`'s own plain,
// immutable snapshot (the doc comment on `CommitRecord`), so `commitMenuSections`/
// `onCommitMenuSelect` below never re-resolve `state.row` against a store that may have moved on.
const tagDialogState = ref<{ open: boolean; target: string }>({ open: false, target: '' });
const branchDialogState = ref<{ open: boolean; startPoint: string }>({
  open: false,
  startPoint: '',
});

function handleGridContextMenu(detail: { row: number; x: number; y: number }): void {
  // P108 F2: resolved once, here, at the real click — `CommitGrid.vue`'s own emit fires
  // synchronously off a real DOM event, so the store is exactly as fresh as the click itself.
  const commit = graphView.store.commitAt(detail.row);
  rowMenu.openAtPoint(
    detail.x,
    detail.y,
    buildRowMenu({
      sha: commit.sha,
      decorations: commit.decoration,
      inProgress: opsState.statusSummary.value?.inProgress ?? null,
    }),
    (id) => onCommitMenuSelect(commit, id),
    { onRestoreFocus: () => commitGridRef.value?.focusGrid() },
  );
}

function handleGridOpenChangeRequest(number: number): void {
  actions.value?.openChangeRequest({ number }).catch((err: unknown) =>
    reportAsyncError(err, ASYNC_FAILURES.openChangeRequest, {
      changeRequestNoun: hostWords(changeRequestState.host.value).changeRequestNoun,
    }),
  );
}

// P108 F2: the commit captured when the menu opened, not re-resolved by row — see the doc comment
// above.
async function onCommitMenuSelect(commit: CommitRecord, id: string): Promise<void> {
  switch (id) {
    case 'checkoutDetached':
      await opsState.runCheckout(commit.sha, 'detach');
      return;
    case 'createBranchHere':
      branchDialogState.value = { open: true, startPoint: commit.sha };
      return;
    case 'createTagHere':
      tagDialogState.value = { open: true, target: commit.sha };
      return;
    case 'createWorktreeHere':
      // P76 §9.1: a commit is a point, not a line of development — the dialog's own detach mode.
      worktreeCreateRequest.value = { mode: 'detach', startPoint: commit.sha };
      return;
    case 'revertThisCommit':
      await opsState.runRevert([commit.sha]);
      return;
    case 'resetToThisCommit':
      // OQ1: mixed is the default mode — git's own default, and the only mode destructive to
      // nothing on disk. `ResetDialog.vue` may change it before confirming (`previewResetMode`).
      await opsState.runReset(commit.sha, 'mixed');
      return;
    case 'cherryPickThisCommit':
      await opsState.runCherryPick(commit.sha);
      return;
    case 'copySha':
      actions.value?.copy(commit.sha, t('git.copy.fullSha.announce'));
      return;
    case 'copyMessage':
      // The subject line only — available synchronously from the store. `CommitMeta.vue`'s own
      // dedicated button (P5) copies the full trailer-joined message once its detail has loaded;
      // duplicating that async fetch here for a context-menu convenience is not worth the race.
      actions.value?.copy(commit.subject, t('git.copy.commitMessage.announce'));
      return;
    default:
      return;
  }
}

// ---------------------------------------------------------------------------------------
// The stash-badge context menu — `CommitGrid.vue`'s own hit-test
// (`refBadges.ts`'s `refKind === "stash"`) reports only the row (own doc comment: "the row itself
// IS the stash commit"), so the full `StashEntry` is resolved here by matching the row's sha
// against `stashState.entries` (`commitAt`/`decorationAt` alone only give sha + stack index, not
// `message`/`timestamp`/`baseSubject`/etc.).
// ---------------------------------------------------------------------------------------
// The entry is resolved once, here, at open time — not re-derived from `row` at select
// time. A stash drop/pop between open and select (auto-refresh, another action) must not let a
// stale row resolve to whatever stash now sits there; capturing `entry` itself makes a vanished
// stash not found by `onStashMenuSelect`; further re-checked against `stashState.entries` there.
const stashBranchTarget = ref<StashEntry | undefined>(undefined);

function handleStashContextMenu(detail: { row: number; x: number; y: number }): void {
  const commit = graphView.store.commitAt(detail.row);
  const entry = stashState.entries.value.find((e) => e.sha === commit.sha);
  if (!entry) return; // nothing resolvable at the real click target — no menu, as before.
  const captured = entry;
  rowMenu.openAtPoint(
    detail.x,
    detail.y,
    buildStashMenu(
      opsState.statusSummary.value?.inProgress ?? null,
      entry,
      refsState.currentBranchName.value,
    ),
    (id) => onStashMenuSelect(captured, id),
    { onRestoreFocus: () => commitGridRef.value?.focusGrid() },
  );
}

async function onStashMenuSelect(captured: StashEntry, id: string): Promise<void> {
  // P108 F2: re-check the captured entry still exists — it may have been dropped/popped while
  // the menu was open (auto-refresh between open and select).
  const entry = stashState.entries.value.find((e) => e.sha === captured.sha);
  if (!entry) return;
  switch (id) {
    case 'stashApply':
      await opsState.runStashApply(entry);
      return;
    case 'stashPop':
      await opsState.runStashPop(entry);
      return;
    case 'stashDrop':
      await opsState.runStashDrop(entry);
      return;
    case 'stashBranch':
      stashBranchTarget.value = entry;
      return;
    case 'stashShow':
      stashState.select(entry.sha);
      return;
    default:
      return;
  }
}

/** `StashList.vue`'s own "Create branch from stash…" row action, bubbled through
 *  `RefsPopover.vue` — same target field the badge context menu's `stashBranch` case sets. */
function handleBranchFromStash(entry: StashEntry): void {
  stashBranchTarget.value = entry;
}

const stashCreateOpen = ref(false);

// The global stash section's own header button, the palette's `saveGlobalStash` action,
// and a STACK row's own "Save to global stash…" action all open the SAME dialog mode —
// `globalStashSaveOpen` for the first two (no pre-selected source), `globalStashSaveSourceEntry`
// for the third (opens pre-selected to promote that entry). Same "App.vue owns the
// state, the dialog owns nothing of its own" shape `stashCreateOpen`/`stashBranchTarget` already
// follow.
const globalStashSaveOpen = ref(false);
const globalStashSaveSourceEntry = ref<StashEntry | undefined>(undefined);

function handleSaveEntryToGlobalStash(entry: StashEntry): void {
  globalStashSaveSourceEntry.value = entry;
}

// G18 D13: AppToolbar.vue's own settings gear — same "App.vue owns the boolean, the dialog owns
// nothing of its own" shape stashCreateOpen/tagDialogState above already follow.
const repoSettingsDialogOpen = ref(false);

// WorktreeDialog.vue's own create-phase seed — same "App.vue owns the state, the
// dialog owns nothing of its own" shape stackDialogTarget below already follows. Opened (and
// re-seeded) by the toolbar refs-popover button, the `createWorktree` palette action, and a
// "Create worktree here…" row action; `undefined` closes it.
const worktreeCreateRequest = ref<WorktreeCreateSeed | undefined>(undefined);

// G26 D13/4.8: StackDialog.vue's own two-mode target — same "App.vue owns the state, the dialog
// component owns nothing of its own" shape every other dialog above follows. `undefined` ⇒
// closed.
const stackDialogTarget = ref<
  { readonly mode: 'setParent' | 'restack'; readonly branch: string } | undefined
>(undefined);

function handleOpenSetStackParentDialog(branch: string): void {
  stackDialogTarget.value = { mode: 'setParent', branch };
}

function handleOpenRestackDialog(branch: string): void {
  stackDialogTarget.value = { mode: 'restack', branch };
}

/** `WorktreeList.vue`'s own "Switch to this worktree" row action, bubbled through
 *  `RefsPopover.vue`/`AppToolbar.vue` — the same `repo.open` + `handleRepoOpened` path
 *  `revealCommitInGraph` above already uses (each worktree is already its own `RepoEntry`, so
 *  "switch" needs no worktree-specific request at all). */
async function handleSwitchWorktree(path: string): Promise<void> {
  const repo = repoState.value;
  if (!repo) return;
  const outcome = await repo.open(path);
  if (outcome.kind !== 'ok') return;
  await handleRepoOpened(outcome.repo.repoId);
}

/** `WorktreeList.vue`'s own create button emits no seed (`undefined`); a ref/commit row action
 *  (§9.3) emits one. `{}` keeps `WorktreeDialog.vue`'s existing defaults either way (P76 §9.1). */
function onCreateWorktree(seed?: WorktreeCreateSeed): void {
  worktreeCreateRequest.value = seed ?? {};
}

const viewHeadBindings = computed(() => {
  const repo = repoState.value?.activeRepo.value;
  const head = repo?.head;
  const track = refsState.branches.value.find((b) => b.isHead)?.track;
  const tracked = track !== undefined && track !== 'gone' ? track : undefined;
  const inProgress = opsState.statusSummary.value?.inProgress;
  return {
    headLabel: head === undefined ? undefined : head.kind === 'detached' ? 'detached' : head.name,
    ahead: tracked?.ahead,
    behind: tracked?.behind,
    operation: inProgress ? renderText(describeInProgress(inProgress)) : undefined,
  };
});

// P107 I2-39: the unborn-HEAD and normal branches below bound <AppToolbar> identically — one
// bindings object instead of two copies of the same 16 props/12 listeners. Read only from inside
// the template's own `v-else-if="repoState"` guard, so `repoState.value` is always defined there.
const toolbarBindings = computed(() => ({
  reportError: reportAsyncError,
  graphView,
  repoState: repoState.value as RepoState,
  refsState,
  opsState,
  stashState,
  worktreeState,
  stackState,
  actions: actions.value,
  changeRequestState,
  searchOpen: searchOpen.value,
  collapseBranches: collapseBranches.value,
  onStashChanges: () => {
    stashCreateOpen.value = true;
  },
  onBranchFromStash: handleBranchFromStash,
  onSaveGlobalStash: () => {
    globalStashSaveOpen.value = true;
  },
  onSaveEntryToGlobalStash: handleSaveEntryToGlobalStash,
  onSwitchWorktree: handleSwitchWorktree,
  onCreateWorktree,
  onOpenRestackDialog: handleOpenRestackDialog,
  onOpenSetStackParentDialog: handleOpenSetStackParentDialog,
  onOpenRepoSettings: () => {
    repoSettingsDialogOpen.value = true;
  },
  onToggleSearch: toggleSearchRow,
  onToggleCollapseBranches: () => setCollapseBranches(!collapseBranches.value),
}));

// ---------------------------------------------------------------------------------------
// The ref-badge context menu — a right-click that lands on a
// `refBadges.ts` badge (`CommitGrid.vue`'s own hit-test) instead of bare row space opens this,
// the same `buildRefMenu` table `RefsPopover.vue`'s dropdown already renders, titled with the
// branch name so it "cannot be misread". Renaming has no inline row to swap into here (a
// badge is a DOM fragment inside a commit's message cell, not a list item), so it opens
// `RenameRefDialog.vue` instead of `RefsPopover.vue`'s own inline field.
// ---------------------------------------------------------------------------------------
const renameRefDialogState = ref<{ open: boolean; currentName: string }>({
  open: false,
  currentName: '',
});
const forceDeleteRefCandidate = ref<{ name: string; x: number; y: number } | undefined>(undefined);

function handleGridRefContextMenu(detail: {
  kind: 'branch' | 'remoteBranch';
  name: string;
  x: number;
  y: number;
}): void {
  const isHead =
    detail.kind === 'branch' &&
    refsState.branches.value.some((row) => row.shortName === detail.name && row.isHead);
  rowMenu.openAtPoint(
    detail.x,
    detail.y,
    buildRefMenu({
      kind: detail.kind,
      shortName: detail.name,
      isHead,
      knownRemotes: [],
      inProgress: opsState.statusSummary.value?.inProgress ?? null,
    }),
    (id) => onRefMenuSelect(detail, id),
    { title: detail.name, onRestoreFocus: () => commitGridRef.value?.focusGrid() },
  );
}

async function onRefMenuSelect(
  state: { kind: 'branch' | 'remoteBranch'; name: string; x: number; y: number },
  id: string,
): Promise<void> {
  if (id === 'checkoutRef') {
    const remoteRow =
      state.kind === 'remoteBranch'
        ? refsState.remoteBranches.value.find((row) => row.shortName === state.name)
        : undefined;
    // `remoteRow` should always be found (the badge only exists for a decoration on an
    // already-loaded row) — the `?? state.name` fallback is defensive only, for the same reason
    // `checkoutRef`'s gate never blocks this: a stale badge from a commit rendered just before a
    // `refsChanged` refresh lands is not worth failing the checkout over.
    const target = remoteRow
      ? remoteCheckoutTarget(remoteRow, refsState.branches.value)
      : state.name;
    await opsState.runCheckout(target, 'switch');
    return;
  }
  if (id === 'renameRef') {
    renameRefDialogState.value = { open: true, currentName: state.name };
    return;
  }
  if (id === 'reviewBranch') {
    await opsState.openReview(state.name);
    return;
  }
  if (id === 'deleteRef') {
    const result = await opsState.branchDelete(state.name, false);
    if (!result.ok && result.error?.kind === 'NotFullyMerged') {
      forceDeleteRefCandidate.value = { name: state.name, x: state.x, y: state.y };
    }
    return;
  }
  if (id === 'createWorktreeHere') {
    // P76 §9.1: `existingBranch` needs a branch that exists locally already; a remote-tracking
    // ref's local name may not (`localNameForRemoteBranch`, the same derivation `checkoutRef`
    // above uses for the DWIM checkout), so that arm creates it as a new branch instead.
    worktreeCreateRequest.value =
      state.kind === 'remoteBranch'
        ? {
            mode: 'newBranch',
            branch: localNameForRemoteBranch(state.name),
            startPoint: state.name,
          }
        : { mode: 'existingBranch', branch: state.name };
  }
}

async function confirmForceDeleteRef(): Promise<void> {
  const candidate = forceDeleteRefCandidate.value;
  forceDeleteRefCandidate.value = undefined;
  if (candidate !== undefined) await opsState.branchDelete(candidate.name, true);
}

// ---------------------------------------------------------------------------------------
// W14's own live region: "the Load-more result and Refresh completion are announced through one
// polite live region" — deferred here from W9 (`LoadMoreButton.vue`'s own doc comment: "a second
// region [t]here would fight it"), since both events are really about `GraphViewState.loading`
// leaving `"loadingMore"`/`"refreshing"`, which this file is already the one place watching. The
// row count *before* the operation started is captured on the way *into* one of those two states
// so the completion message can report how many rows the operation itself actually added, not
// just the total the store now holds.
// ---------------------------------------------------------------------------------------
let loadedRowsBeforeLoad = 0;
// Captured together, at the moment `loading` *enters* `'refreshing'` — synchronous with
// `GraphViewState` setting `autoRefreshing` (no `await` between the two), so this is reliable in
// a way re-reading `graphView.autoRefreshing.value` at the *exit* transition would not be (by
// then it may already have been reset). Both the viewport restore and the announcement gate below
// key off this one captured flag rather than the live ref.
let refreshWasAuto = false;
let autoRefreshViewportSha: string | undefined;

watch(graphView.loading, (state, previous) => {
  if (state === 'loadingMore' || state === 'refreshing') {
    loadedRowsBeforeLoad = graphView.loadedRows.value;
    if (state === 'refreshing') {
      refreshWasAuto = graphView.autoRefreshing.value;
      const top = refreshWasAuto ? commitGridRef.value?.getViewportTop() : undefined;
      autoRefreshViewportSha = top === undefined ? undefined : graphView.store.shaAt(top);
    }
    return;
  }
  if (state !== 'idle') return;
  if (previous === 'loadingMore') {
    const added = graphView.loadedRows.value - loadedRowsBeforeLoad;
    announce(
      composeLoadMoreAnnouncement(added, graphView.remaining.value, graphView.exhausted.value),
    );
  } else if (previous === 'refreshing') {
    if (refreshWasAuto) {
      // A background refresh must neither speak on every commit nor move the user's viewport —
      // it wins over the selection's own `scrollRowIntoView` (this watcher runs after the
      // `pendingSelectionSha` re-resolution above, which is what would otherwise re-scroll).
      if (autoRefreshViewportSha !== undefined) {
        const row = graphView.store.rowOfSha(autoRefreshViewportSha);
        if (row >= 0) commitGridRef.value?.scrollToTopRow(row);
      }
    } else {
      announce(composeRefreshAnnouncement(graphView.loadedRows.value));
    }
  }
});

// ---------------------------------------------------------------------------------------
// The palette's own dispatcher — a switch over the same affordances the toolbar or a
// context menu already drives, never a second implementation of an operation. Reached two
// ways: `bridge.on('ui.action', ...)` below, for a command run while this webview is already
// live, and `props.pendingUiAction` at mount (below), for one that fired while it was cold —
// `panelView.ts`'s own two-arm pattern, mirrored from `review.target`'s.
// ---------------------------------------------------------------------------------------
function runUiAction(
  action: UiActionKind,
  target?: { readonly repoId: string; readonly sha: string },
): void {
  switch (action) {
    case 'openBranchPicker':
      toolbarRef.value?.openBranchPicker();
      break;
    case 'openTagPicker':
      toolbarRef.value?.openBranchPicker('tags');
      break;
    case 'openStashPicker':
      toolbarRef.value?.openBranchPicker('stashes');
      break;
    case 'openWorktreePicker':
      toolbarRef.value?.openBranchPicker('worktrees');
      break;
    case 'openStackPicker':
      toolbarRef.value?.openBranchPicker('stacks');
      break;
    case 'createBranch':
      branchDialogState.value = { open: true, startPoint: selection.sha.value ?? 'HEAD' };
      break;
    case 'createTag':
      tagDialogState.value = { open: true, target: selection.sha.value ?? 'HEAD' };
      break;
    case 'revertSelected': {
      const sha = selection.sha.value;
      if (sha) void opsState.runRevert([sha]);
      else announce(t('git.app.noSelection.announce'));
      break;
    }
    case 'continueOperation':
      void opsState.continueOp();
      break;
    case 'abortOperation':
      void opsState.abortOp();
      break;
    case 'skipCommit':
      void opsState.skipOp();
      break;
    case 'undo':
      void opsState.undo();
      break;
    case 'fetch':
      toolbarRef.value?.fetch();
      break;
    case 'pull':
      toolbarRef.value?.pull();
      break;
    case 'push':
      toolbarRef.value?.push();
      break;
    case 'forcePush':
      toolbarRef.value?.forcePush();
      break;
    case 'cancelRemoteOperation':
      toolbarRef.value?.cancelRemote();
      break;
    case 'refresh':
      toolbarRef.value?.refresh();
      break;
    case 'revealCommit':
      if (target) {
        revealCommitInGraph(target).catch((err: unknown) =>
          reportAsyncError(err, ASYNC_FAILURES.revealCommit),
        );
      }
      break;
    case 'stashChanges':
      stashCreateOpen.value = true;
      break;
    case 'saveGlobalStash':
      globalStashSaveOpen.value = true;
      break;
    case 'createWorktree':
      worktreeCreateRequest.value = {};
      break;
    case 'resetSelected': {
      const sha = selection.sha.value;
      if (sha) void opsState.runReset(sha, 'mixed');
      else announce(t('git.app.noSelection.announce'));
      break;
    }
    case 'cherryPickSelected': {
      const sha = selection.sha.value;
      if (sha) void opsState.runCherryPick(sha);
      else announce(t('git.app.noSelection.announce'));
      break;
    }
    // G26 D13: the palette's own route to "Restack this stack" — the CURRENT branch stands in for
    // "this stack" (D14: scope is always the whole stack regardless of which member triggers it).
    case 'restackStack': {
      const branch = refsState.currentBranchName.value;
      if (branch) stackDialogTarget.value = { mode: 'restack', branch };
      else announce(t('git.app.noCurrentBranch.announce'));
      break;
    }
    // G26 D13/§7.3: alt+up/alt+down's own palette equivalents — resolve a target client-side
    // (`stackListModel`) and call the existing checkout op, never a second implementation.
    case 'checkoutStackParent':
    case 'checkoutStackChild': {
      const branch = refsState.currentBranchName.value;
      const stackResult = { stacks: stackState.stacks.value, orphans: stackState.orphans.value };
      const target =
        branch === undefined
          ? undefined
          : action === 'checkoutStackParent'
            ? parentOf(stackResult, branch)
            : childOf(stackResult, branch);
      if (target !== undefined) void opsState.runCheckout(target, 'switch');
      else announce(t('git.app.noStackTarget.announce'));
      break;
    }
    // G-UX D9: the palette's own route to toggling the graph search row — the same assignment
    // the in-webview `/`/`Ctrl+F`/`Ctrl+Alt+F` gestures already make.
    case 'toggleSearch':
      toggleSearchRow();
      break;
  }
}

const unsubscribeUiAction = bridge.on('ui.action', (event) =>
  runUiAction(event.action, event.target),
);

onMounted(() => {
  // requestAnimationFrame so the mark lands after the browser has actually painted this
  // frame, not merely after Vue's synchronous mount work.
  requestAnimationFrame(() => {
    performance.mark('kira:first-paint');
    performance.measure('kira:first-paint', undefined, 'kira:first-paint');
  });

  // `kira:layout-complete` used to be marked right here, in the same frame as
  // `kira:first-paint` — correct only while this shell had no real grid to wait on. Now that one
  // exists, marking it here would always measure ~0ms and hide the exact cost the budget separates
  // out: `GraphViewState`'s own doc comment on `generation` names the reason first-paint and
  // layout-complete are two different budgets at all — "text first, graph a frame later, never a
  // blank list waiting on a worker". This mark now means: the *first* `LayoutChunk` has been
  // applied and the rows it covers re-rendered with their lanes, not merely that the shell
  // mounted. `CommitGrid.vue`'s own `handleChunkLayout` — the one place that event fires — marks
  // it, once, the first time that happens; nothing here needs to know when that is.
  void bootstrap()
    .then(onBootstrapSuccess)
    .catch((err: unknown) => {
      bootError.value = errorText(err);
    });
});

// The cold-bootstrap arm — a palette command that fired before this webview had a live
// RpcServer (panelView.ts's own #pendingUiAction). Shared between the initial mount and every
// retry, so whichever attempt is the one that succeeds still runs it — a failed first attempt
// followed by a successful Retry used to drop it silently. Still runs once: exactly one
// bootstrap() call ever resolves (mount and retry are mutually exclusive — a retry only
// happens after a failure, and success hides the Retry button), and a later hide/reveal of the
// same view starts with no pending action (panelView.ts clears it once consumed), so this never
// replays.
function onBootstrapSuccess(): void {
  if (props.pendingUiAction) {
    runUiAction(props.pendingUiAction.action, props.pendingUiAction.target);
  }
}

function retryBootstrap(): void {
  sharedRetryBootstrap(bootError, bootstrap, onBootstrapSuccess);
}

// The four search toggles/scope below round-trip through every write this file makes (the `watch`
// callback spreads `...lastPersisted`, and a successful `viewState.read()` at boot replaces this
// whole literal with the persisted one), AND are now reactively wired to `searchState` — read into
// it once persisted state is available (`bootstrap()`'s own `if (persisted)` block) and fed back by
// the same persistence `watch` every other field goes through, exactly like `fileListMode` and
// `DetailPane` before it. The query TEXT itself is deliberately never persisted — a remembered term
// silently re-running against a repository that has moved on is the same stale-state argument the
// diff/selected-file omission already made.
let lastPersisted: PersistedViewState = {
  version: 8,
  repoId: null,
  loadedRows: 0,
  detailOpen: true,
  scrollRow: 0,
  selectedSha: null,
  columnWidths: DEFAULT_COLUMN_WIDTHS,
  dateFormat: 'relative',
  detailWidth: DEFAULT_DETAIL_WIDTH,
  fileListMode: 'tree',
  searchCaseSensitive: false,
  searchWholeWord: false,
  searchRegex: false,
  searchScope: 'both',
  searchOpen: false,
  collapseBranches: true,
};

// Registered synchronously in setup so it is bound to the component scope and stopped on unmount;
// `bootstrap()` arms it once the persisted state is loaded.
let persistenceArmed = false;
watch(
  [
    () => repoState.value?.activeRepo.value?.repoId ?? null,
    graphView.loadedRows,
    detailOpen,
    scrollRow,
    () => selection.sha.value,
    columnWidths,
    dateFormat,
    detailWidth,
    detailState.listMode,
    searchState.caseSensitive,
    searchState.wholeWord,
    searchState.regex,
    searchState.scope,
    searchOpen,
    collapseBranches,
  ],
  ([
    repoId,
    loadedRows,
    isDetailOpen,
    row,
    selectedSha,
    widths,
    format,
    dWidth,
    listMode,
    searchCaseSensitive,
    searchWholeWord,
    searchRegex,
    searchScope,
    isSearchOpen,
    isCollapseBranches,
  ]) => {
    if (!persistenceArmed) return;
    lastPersisted = {
      ...lastPersisted,
      repoId,
      loadedRows,
      detailOpen: isDetailOpen,
      scrollRow: row,
      selectedSha,
      columnWidths: widths,
      dateFormat: format,
      detailWidth: dWidth,
      fileListMode: listMode,
      searchCaseSensitive,
      searchWholeWord,
      searchRegex,
      searchScope,
      searchOpen: isSearchOpen,
      collapseBranches: isCollapseBranches,
    };
    props.viewState.write(lastPersisted);
  },
);

async function bootstrap(): Promise<void> {
  // P108 F7: a retry re-enters here after an earlier run already got this far — dispose that
  // run's own `RepoState` (which holds a live `bridge.on(...)` subscription) before
  // replacing it, so a failed run never leaks a subscription forever.
  repoState.value?.dispose();
  const init = await bridge.init();
  const repo = new RepoState(bridge, init.git);
  repoState.value = repo;
  // `actions` is built exactly once, here, rather than reactively re-derived.
  actions.value = createDetailActions(
    bridge,
    (text) => detailState.announce(text),
    () => repoState.value?.activeRepo.value?.repoId,
  );

  const persisted = props.viewState.read();
  // Kira Space's app-wide date format (mount option, else the server's app.init) wins
  // over what this webview persisted for itself.
  const serverDateFormat = props.dateFormat ?? init.dateFormat;
  if (serverDateFormat) dateFormat.value = serverDateFormat;
  // A failure below must not leave persistence off: the persisted state is loaded, so later user
  // changes still write.
  try {
    let openedFromPersisted = false;
    if (persisted) {
      lastPersisted = persisted;
      detailOpen.value = persisted.detailOpen;
      columnWidths.value = persisted.columnWidths;
      dateFormat.value = serverDateFormat ?? persisted.dateFormat;
      detailWidth.value = persisted.detailWidth;
      initialScrollRow.value = persisted.scrollRow;
      detailState.setListMode(persisted.fileListMode);
      // The four search toggles/scope are carried through unchanged (this file's own comment above
      // `lastPersisted`'s literal), but nothing read them into `searchState` until now. The query
      // TEXT itself is deliberately never persisted -- only the widget state, same as `dateFormat`.
      searchState.caseSensitive.value = persisted.searchCaseSensitive;
      searchState.wholeWord.value = persisted.searchWholeWord;
      searchState.regex.value = persisted.searchRegex;
      searchState.scope.value = persisted.searchScope;
      searchOpen.value = persisted.searchOpen;
      // Pushed straight to graphOrder, not through setCollapseBranches — that function
      // also asks for a relayout, and the graphView.openStream(...) call below already produces the
      // first one once the persisted repo's rows arrive; a relayout against the still-empty store
      // here would be redone.
      collapseBranches.value = persisted.collapseBranches;
      graphOrder.setCollapseEnabled(persisted.collapseBranches);

      // The "collapsed by default" below `wide`: a persisted `detailOpen: true` from an earlier,
      // wider session must not reopen the pane/drawer over a mount that starts narrower — without
      // this, the line above would silently clobber the collapse the mount-time `breakpoint` watch
      // (below) already applied moments earlier, since that watch runs synchronously during mount
      // while this restore only lands later, after `bridge.init()`'s own await. Not gated on
      // `breakpoint`'s *previous* value the way that watch is (there is no real "previous" at boot,
      // only that watch's own initial-ref placeholder) — mounting directly into a narrow layout is
      // not merely a special case of resizing into one. A real
      // selection still reopens it once `pendingSelectionSha` resolves, via the selection watch
      // below — nothing here treats a boot with a pending selection any differently.
      collapseIfNarrowWithNoSelection();

      if (persisted.repoId) {
        const outcome = await repo.open(persisted.repoId);
        if (outcome.kind === 'ok') {
          openedFromPersisted = true;
          if (persisted.selectedSha) pendingSelectionSha.value = persisted.selectedSha;
          // §5.4: a freshly (re)mounted GraphViewState's own `loadedRows` starts at 0, so the
          // default `resumeThroughRow` asks the host to replay every row it still has cached —
          // that single round trip is the whole of "rehydrates without re-running git".
          await graphView.openStream(outcome.repo.repoId);
        }
      }
    }

    // Falls back to the workspace's own repository, multi-root aware, only when
    // nothing persisted opened one — a persisted repo B must win over workspace folder A
    // (the one ordering this must not get wrong), and NoRepositoryPanel still owns the case where
    // no candidate is a repository at all. Mirrors extension.ts's own palette-command default
    // rather than inventing a second policy.
    if (!openedFromPersisted) {
      await repo.refreshList();
      // `NoRepositoryPanel.vue`'s own onMounted already calls `refreshList()` and can
      // render candidates a user clicks while this loop is still awaiting an earlier one —
      // `repo.activeRepo.value` is the one shared signal that a pick already won, checked before
      // every candidate `open()` this loop is about to make, not just relied on via the sequence
      // token inside `open()` itself.
      for (const candidate of repo.candidates.value) {
        if (repo.activeRepo.value) break;
        const outcome = await repo.open(candidate.path);
        if (outcome.kind === 'ok') {
          await handleRepoOpened(outcome.repo.repoId);
          break;
        }
        if (outcome.kind === 'gitUnavailable') break; // git itself is blocked; GitBlockedPanel owns it.
        if (outcome.kind === 'superseded') break; // a newer open (a user's pick) already won.
        // 'notARepository' — a workspace folder that simply is not a repo; try the next one.
      }
    }
  } finally {
    persistenceArmed = true;
  }
}

// ---------------------------------------------------------------------------------------
// The breakpoints — measured on the webview's own width via ResizeObserver on the root, not
// `window.matchMedia` (which reports the *window's* width and would be wrong the moment the
// panel is docked to the side or split with another editor group).
// ---------------------------------------------------------------------------------------
type Breakpoint = 'wide' | 'narrow' | 'overlay';

function breakpointFor(width: number): Breakpoint {
  if (width >= 900) return 'wide';
  if (width >= 600) return 'narrow';
  return 'overlay';
}

const rootEl = ref<HTMLDivElement | null>(null);
// `main.ts`'s own `mount()` provides this per instance only for a Kira Space graph tab
// (a kept-mounted sibling among possibly several) — always `true` everywhere else
// (this package's own tests). Gates `onDocumentKeydown`, breakpoint sync and the overlay
// click-outside, so those behave as before wherever only one instance ever exists.
const graphVisible = useGraphVisible();
const overlayDetailRegionEl = ref<HTMLElement | null>(null);
const breakpoint = ref<Breakpoint>('wide');
let breakpointRaf = 0;

/** §6.3's "collapsed by default" below `wide`, with nothing selected — shared by the mount-time
 *  restore in `bootstrap()` (see its own call site's comment) and the live-resize watch just
 *  below, so both a fresh mount into a narrow layout and a later resize into one agree. */
function collapseIfNarrowWithNoSelection(): void {
  if (breakpoint.value !== 'wide' && selection.row.value < 0) detailOpen.value = false;
}

// Entering a narrower breakpoint with nothing selected collapses the pane (the "collapsed by
// default"); entering it *with* a selection, or widening back past 900px, leaves `detailOpen` as
// it is — there is nothing asking a widen to force it back open, and forcing it closed
// on every narrow-to-wide crossing would fight a user who just opened it deliberately.
watch(breakpoint, (kind, previous) => {
  if (kind !== 'wide' && previous === 'wide') collapseIfNarrowWithNoSelection();
});

// §6.3: "collapsed by default, opens on selection" for both sub-900px bands — at the wide
// breakpoint, selecting a row does not by itself open the pane (only CommitGrid.vue's own
// toggle/close events do, §6.4's second-click/Enter/Esc model), so this only applies below it.
watch(
  () => selection.row.value,
  (row) => {
    if (row >= 0 && breakpoint.value !== 'wide') detailOpen.value = true;
  },
);

// P77 §14 (N9): the same rule, for a stash row selected from the picker — `stashState.selected`
// has no row index to gate on (it is never a graph row), so this watches the entry itself instead.
watch(
  () => stashState.selected.value,
  (entry) => {
    if (entry !== undefined && breakpoint.value !== 'wide') detailOpen.value = true;
  },
);

function toggleDetail(): void {
  detailOpen.value = !detailOpen.value;
}

/** G-UX D2: a dedicated one-liner for `CommitGrid.vue`'s new `openDetail` emit (an unselected
 *  row's first click) — never `toggleDetail`, so a click on a *different* row while the pane is
 *  already open never closes it. */
function openDetail(): void {
  detailOpen.value = true;
}

/** §6.6's Esc ordering: an open menu, then the search results dropdown, then the detail pane/
 *  drawer (P11 W12/W13's spec edit 6, restated with this file's own share of it). The first two
 *  stages never reach here at all — `RowContextMenu.vue`'s and `SearchBox.vue`'s own `keydown`
 *  handlers each call `stopPropagation()` on the `Escape` they act on (see either component's own
 *  doc comment), so this file's document-level listener only ever sees an `Escape` that both of
 *  those already declined. Called both by `CommitGrid.vue`'s own `closeDetail` emit (when the
 *  grid has focus) and this file's own document-level listener (when focus is inside the detail
 *  pane/drawer itself, outside the grid's host and so outside its own keydown listener's reach).
 *
 *  G21 D12: the "diff view first" stage this function used to have — `Esc` closing the embedded
 *  diff and going back to the tree, before a second `Esc` closed the whole pane — is gone along
 *  with the embedded diff itself. `Esc` now always closes the pane in one step; the host's native diff
 *  editor has its own, unrelated `Esc` handling. */
const selectionIsStash = computed(() => stashState.selected.value !== undefined);
// P7 (item 2): the strip's own selection wins over a stale row selection — checked first in the
// template's own v-else-if chain, mirroring selectionIsStash's own priority pattern.
const selectionIsWorking = computed(() => workingState.selected.value);

function closeDetail(): void {
  detailOpen.value = false;
}

/** The strip's own click handler — opens the pane (mirrors `openDetail`/a stash
 *  row's click, both of which force `detailOpen` true rather than merely toggling it) and selects
 *  the working tree. `selection.clear()` deselects any highlighted grid row, which fires the
 *  `selection.sha` watch above and clears `detailState`/`stashState` via their own existing
 *  null-selection path — no separate clearing logic needed here. */
function onSelectWorking(): void {
  selection.clear();
  workingState.select(true);
  detailOpen.value = true;
}

/** The working-tree pane's own file-open action — `editor.openWorkingDiff` never
 *  touches the Go server (answered entirely inside the extension), so this is a direct bridge
 *  request, not a `DetailActions` method (whose `openInEditor` is shaped for a commit's sha/
 *  parentIndex, neither of which the working tree has). */
async function openWorkingDiff(params: {
  path: string;
  originalPath: string | undefined;
  status: FileChangeKind;
  pinned: boolean;
}): Promise<void> {
  const repoId = repoState.value?.activeRepo.value?.repoId;
  if (!repoId) return;
  await bridge.request('editor.openWorkingDiff', { repoId, ...params });
}

/** `/`/`Ctrl/Cmd+F` (moved up from `SearchBox.vue`, which used to own this listener for
 *  its own whole lifetime — now that the search row is conditionally mounted, that lifetime is
 *  exactly when it must NOT fire). See `isEditableTarget`'s own doc comment for the exemption
 *  `/` needs and `Ctrl/Cmd+F`'s toggle-closed behaviour. */
function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  // The search row itself (its input, toggles, or scope select) is never treated as "some other
  // editable field to avoid hijacking" — mirrors SearchBox.vue's own original `target ===
  // inputEl.value` exemption, widened to the whole row now that this handler sits outside it.
  if (searchRowEl.value?.contains(target)) return false;
  const tag = target.tagName;
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || target.isContentEditable;
}

function onSearchShortcut(event: KeyboardEvent): boolean {
  const isFindCombo = (event.key === 'f' || event.key === 'F') && (event.ctrlKey || event.metaKey);
  if (event.key !== '/' && !isFindCombo) return false;
  if (event.key === '/' && isEditableTarget(event.target)) return false;
  // `Ctrl/Cmd+F` while the row is already open AND focus is inside it closes it — mirroring
  // the usual find-widget second-press-closes gesture. `/` never closes, only opens/focuses.
  if (
    isFindCombo &&
    searchOpen.value &&
    (searchRowEl.value?.contains(document.activeElement) ?? false)
  ) {
    event.preventDefault();
    closeSearch();
    return true;
  }
  event.preventDefault();
  openSearch();
  return true;
}

const contextMenuStore = useContextMenuStore();
// Menu closes on Escape before the bubble listener runs; capture records it was open.
let escapeClosedMenu = false;
useEventListener(
  document,
  'keydown',
  (event: KeyboardEvent) => {
    escapeClosedMenu = event.key === 'Escape' && contextMenuStore.open;
  },
  { capture: true },
);

function onDocumentKeydown(event: KeyboardEvent): void {
  // Kira Space mounts several git-ui apps into one document — one per graph tab
  // (kept mounted, v-show, while hidden) plus the review sidebar (`v-show`). Every instance's own
  // `document`-level listener otherwise fires for every keydown regardless of which one is
  // visible or focused. Handle it here only when the key landed inside this
  // instance's own DOM, or — for a shortcut typed with focus elsewhere entirely (the host chrome,
  // no graph focused at all) — only the one instance Kira Space currently shows.
  const target = event.target;
  const insideThisRoot = !!rootEl.value && target instanceof Node && rootEl.value.contains(target);
  if (!insideThisRoot && !graphVisible.value) return;
  if (onSearchShortcut(event)) return;
  if (event.key === 'Escape' && detailOpen.value && !escapeClosedMenu) closeDetail();
}

// The overlay drawer's own "dismissible... on a click outside" — only acted on while the
// drawer is showing, and only at the overlay breakpoint (the docked pane at wide/narrow
// has no such behaviour; clicking the grid to select a different row is normal use there, not a
// dismissal). A template ref scoped to this instance's own overlay `<aside>` — the old
// `document.querySelector('[data-testid="detail-region"]')` resolved the first match in the whole
// document, which could belong to a different graph tab's own mount.
onClickOutside(
  overlayDetailRegionEl,
  () => {
    if (!graphVisible.value) return;
    if (breakpoint.value === 'overlay' && detailOpen.value) closeDetail();
  },
  // A FileTree row menu (RowContextMenu, DropdownMenu) can open while the
  // overlay drawer is showing — reka teleports its content to `document.body`, outside
  // `overlayDetailRegionEl`, so picking an item would otherwise read as an outside click and
  // close the drawer mid-action. `[data-reka-popper-content-wrapper]` is Radix parity's own
  // wrapper attribute on every Popper-positioned surface (tooltip, dropdown, popover).
  { ignore: ['[data-reka-popper-content-wrapper]'] },
);

// A hidden kept-mounted host reads 0×0, not a real width (CommitGrid's scheduleResize guard).
function syncBreakpoint(): void {
  if (!graphVisible.value || !rootEl.value) return;
  breakpoint.value = breakpointFor(rootEl.value.clientWidth);
}

function scheduleBreakpointUpdate(): void {
  if (breakpointRaf !== 0) return;
  breakpointRaf = requestAnimationFrame(() => {
    breakpointRaf = 0;
    syncBreakpoint();
  });
}

// ---------------------------------------------------------------------------------------
// The detail pane's own resize handle (≥900px only) — the same drag-and-clamp
// shape `CommitGrid.vue`'s column handles use, kept here rather than factored out: this is the
// only other resizable edge in the app, and the two call sites differ enough (this one persists
// through `viewState` directly, that one round-trips through a prop/emit pair) that a shared
// helper would mostly be parameter-passing.
// ---------------------------------------------------------------------------------------
const MIN_DETAIL_WIDTH = 240;
const MAX_DETAIL_WIDTH = 640;

function setDetailWidth(next: number): void {
  detailWidth.value = Math.max(MIN_DETAIL_WIDTH, Math.min(MAX_DETAIL_WIDTH, Math.round(next)));
}

// The detail pane is right-docked: `direction="reverse"` widens it as the handle moves left.
const detailWidthPx = computed(() => `${detailWidth.value}px`);

// `exactOptionalPropertyTypes` (tsconfig.base.json) treats an explicit `undefined` differently
// from an omitted prop — `CommitGrid.vue`'s `initialScrollRow?: number` wants the latter on a
// first-ever mount (nothing persisted to restore), so this only spreads the prop in once
// `bootstrap()` has set one, rather than always binding a possibly-`undefined` value.
const initialScrollRowProp = computed(() =>
  initialScrollRow.value === undefined ? {} : { initialScrollRow: initialScrollRow.value },
);

// The strip's own selection has no row at all, so it must widen this check directly
// rather than through `selection.row` — mirrors `selectionIsWorking`'s own reasoning.
// A stash row can never be a graph row either (`selectionIsStash` above widens the
// same way) — before this, selecting a stash from the picker left this `false`, so the detail
// pane rendered "Select a commit to see its details." instead of the stash's own pane the
// `v-else-if` chain below already knows how to show.
const hasSelection = computed(
  () => selection.row.value >= 0 || workingState.selected.value || selectionIsStash.value,
);

useEventListener(document, 'keydown', onDocumentKeydown);
useResizeObserver(rootEl, scheduleBreakpointUpdate);
watch(graphVisible, (visible) => {
  if (visible) scheduleBreakpointUpdate();
});

onMounted(syncBreakpoint);

// The server syncs the change request index on its own cadence; a regained focus syncs at once
// and a blur slows it. Debounced so a quick blur/focus pair sends a single request.
const windowFocused = useWindowFocus();
const documentVisibility = useDocumentVisibility();
const appFocused = computed(
  () => windowFocused.value && documentVisibility.value === 'visible' && graphVisible.value,
);
const sendFocus = useDebounceFn(
  (focused: boolean) => changeRequestState.syncFocus(focused),
  TIMING.focusSync,
);
watch(appFocused, (focused) => {
  void sendFocus(focused);
});

onBeforeUnmount(() => {
  if (breakpointRaf !== 0) cancelAnimationFrame(breakpointRaf);
  unsubscribeUiAction();
  graphView.dispose();
  refsState.dispose();
  opsState.dispose();
  stashState.dispose();
  workingState.dispose();
  worktreeState.dispose();
  stackState.dispose();
  searchState.dispose();
  repoSettingsState.dispose();
  changeRequestState.dispose();
  repoState.value?.dispose();
  bridge.dispose();
});
</script>

<template>
  <div
    ref="rootEl"
    class="flex flex-col h-full w-full bg-bg text-fg overflow-hidden text-graph-md font-ui"
    :data-connection-state="connectionState"
  >
    <!-- Unconditional, present from first paint regardless of which of the four content states
         below is showing (or whether bootstrap() has resolved a repoState at all yet) — the old
         live-data strip carried this testid unconditionally too (inside its own always-rendered
         toolbar), and it is a genuine e2e wait/assert target across all three hosts' specs, not
         merely cosmetic duplicate of the root's own data-connection-state attribute above. -->
    <span class="sr-only" data-testid="connection-state">{{ connectionState }}</span>
    <!-- W14's one polite live region (see the `liveAnnouncement` watch above) — unconditional and
         present from first paint, same as connection-state above, since Load-more/Refresh can
         both complete while this file's own v-if chain is on any branch that renders the toolbar. -->
    <div class="sr-only" role="status" aria-live="polite" data-testid="live-announcements">
      {{ liveAnnouncement }}
    </div>
    <!-- P173: the visible half of every failure the live region announces — outside the v-if chain
         below, same host-agnostic slot as the banners around it. -->
    <FailureBanner
      v-if="failure"
      :notice="failure"
      :show-operations="props.showOperations"
      @dismiss="clearFailure()"
      @retry="retryGraph"
    />
    <!-- G12 D6: outside the v-if="repoState" gate below, since bootError means bootstrap() never
         got that far — a blank panel is never an acceptable rendering of a failure. -->
    <div
      v-if="bootError && !repoState"
      class="flex flex-col items-center justify-center gap-2 h-full p-4 text-center text-fg"
      data-testid="boot-error"
    >
      <p class=" max-w-120 text-muted-foreground">{{
        t('git.app.bootError.error', { error: bootError })
      }}</p>
      <Button variant="dialog" size="kira" data-testid="boot-retry" @click="retryBootstrap">
        {{ t('common.action.retry.label') }}
      </Button>
    </div>
    <template v-else-if="repoState">
      <!-- bootstrap() keeps going after repoState is set — through persisted repo.open,
           the auto-open candidate loop and openStream — so a rejection from any of those must
           surface here regardless of which sub-branch below ends up rendering (GitBlockedPanel,
           NoRepositoryPanel, the unborn-head empty state, or the full graph), not only the last of
           them. One banner, above all of them, rather than duplicated into just one branch. -->
      <Alert
        v-if="bootError"
        variant="error"
        strip
        class="shrink-0"
        role="status"
        data-testid="boot-error-banner"
      >
        <AlertDescription>{{
          t('git.app.bootError.error', { error: bootError })
        }}</AlertDescription>
        <AlertAction class="top-1/2 -translate-y-1/2">
          <Button
            variant="dialog"
            size="kira"
            data-testid="boot-error-banner-retry"
            @click="retryBootstrap"
          >
            {{ t('common.action.retry.label') }}
          </Button>
        </AlertAction>
      </Alert>

      <GitBlockedPanel v-if="repoState.git.value.kind !== 'ok'" :status="repoState.git.value" />

      <NoRepositoryPanel
        v-else-if="!repoState.activeRepo.value"
        :repo-state="repoState"
        @repoOpened="handleRepoOpened"
      />

      <template v-else-if="repoState.activeRepo.value.head.kind === 'unborn'">
        <GitViewHead v-if="repoHead" :repo="repoHead" v-bind="viewHeadBindings" />
        <AppToolbar ref="toolbarRef" v-bind="toolbarBindings" />
        <div
          v-if="searchOpen"
          ref="searchRowEl"
          class="flex items-center gap-1 py-1 px-2 border-b border-border shrink-0"
        >
          <SearchBox
            ref="searchBoxRef"
            :search="searchState"
            @select="handleSearchSelect"
            @focus-grid="handleSearchFocusGrid"
            @close="closeSearch"
          />
        </div>
        <EmptyRepositoryPanel :branch-name="repoState.activeRepo.value.head.name" />
      </template>

      <template v-else>
        <GitViewHead v-if="repoHead" :repo="repoHead" v-bind="viewHeadBindings" />
        <AppToolbar ref="toolbarRef" v-bind="toolbarBindings" />
        <div
          v-if="searchOpen"
          ref="searchRowEl"
          class="flex items-center gap-1 py-1 px-2 border-b border-border shrink-0"
        >
          <SearchBox
            ref="searchBoxRef"
            :search="searchState"
            @select="handleSearchSelect"
            @focus-grid="handleSearchFocusGrid"
            @close="closeSearch"
          />
        </div>
        <ConflictBanner :ops="opsState" />
        <main class="flex flex-1 min-h-0 min-w-0">
          <section
            class="relative flex-1 min-w-0 flex flex-col bg-bg"
            data-testid="graph-region"
            :aria-label="t('git.graph.region.aria')"
          >
            <UncommittedChangesStrip
              :graph-view="graphView"
              :ops-state="opsState"
              :graph-width="gridGraphWidth"
              @select="onSelectWorking"
            />
            <CommitGrid
              ref="commitGridRef"
              class="flex-1 min-h-0"
              :graph-view="graphView"
              :order="graphOrder"
              :selection="selection"
              :column-widths="columnWidths"
              :date-format="dateFormat"
              :detail-open="detailOpen"
              :search="searchState"
              :change-requests="changeRequestState"
              :stack="stackState"
              v-bind="initialScrollRowProp"
              @update:column-widths="columnWidths = $event"
              @graph-width="gridGraphWidth = $event"
              @scroll="scrollRow = $event"
              @toggle-detail="toggleDetail"
              @open-detail="openDetail"
              @closeDetail="closeDetail"
              @refresh="triggerRefresh"
              @context-menu="handleGridContextMenu"
              @ref-context-menu="handleGridRefContextMenu"
              @stash-context-menu="handleStashContextMenu"
              @open-change-request="handleGridOpenChangeRequest"
            />
            <LoadMoreButton
              :graph-view="graphView"
              :page-size="pageSize"
              :report-error="reportAsyncError"
            />
            <span class="sr-only" data-testid="chunk-source">{{
              graphView.lastChunkSource.value ?? ''
            }}</span>
          </section>

          <aside
            v-if="detailOpen && breakpoint !== 'overlay'"
            class="relative shrink-0 border-l border-border bg-bg overflow-auto"
            data-testid="detail-region"
            :aria-label="t('git.detail.region.aria')"
            :style="{ width: detailWidthPx }"
          >
            <ResizeHandle
              v-if="breakpoint === 'wide'"
              class="absolute inset-y-0 left-0 -ml-px z-2"
              :label="t('git.detail.resizeHandle.label')"
              direction="reverse"
              :size="detailWidth"
              :min="MIN_DETAIL_WIDTH"
              :max="MAX_DETAIL_WIDTH"
              @live="setDetailWidth"
              @commit="setDetailWidth"
            />
            <EmptyState
              v-if="!hasSelection"
              fill
              icon="git-commit"
              :title="t('git.detail.noSelection.emptyHint')"
            />
            <WorkingDetailPane
              v-else-if="selectionIsWorking && actions"
              :working-state="workingState"
              :store="graphView.store"
              :actions="actions"
              :open-file="openWorkingDiff"
            />
            <StashDetailPane
              v-else-if="selectionIsStash && actions"
              :stash="stashState"
              :store="graphView.store"
              :actions="actions"
            />
            <DetailPane
              v-else-if="actions"
              :detail-state="detailState"
              :store="graphView.store"
              :actions="actions"
              :change-requests="changeRequestState"
            />
          </aside>
        </main>

        <div
          v-if="detailOpen && breakpoint === 'overlay'"
          class="absolute inset-0 flex justify-end bg-scrim z-(--kira-z-dialog)"
        >
          <aside
            ref="overlayDetailRegionEl"
            class="relative shrink-0 border-l border-border bg-bg overflow-auto w-80 max-w-[90vw] shadow-kira"
            data-testid="detail-region"
            :aria-label="t('git.detail.region.aria')"
          >
            <EmptyState
              v-if="!hasSelection"
              fill
              icon="git-commit"
              :title="t('git.detail.noSelection.emptyHint')"
            />
            <WorkingDetailPane
              v-else-if="selectionIsWorking && actions"
              :working-state="workingState"
              :store="graphView.store"
              :actions="actions"
              :open-file="openWorkingDiff"
            />
            <StashDetailPane
              v-else-if="selectionIsStash && actions"
              :stash="stashState"
              :store="graphView.store"
              :actions="actions"
            />
            <DetailPane
              v-else-if="actions"
              :detail-state="detailState"
              :store="graphView.store"
              :actions="actions"
              :change-requests="changeRequestState"
            />
          </aside>
        </div>

        <!-- Popover replaces the hand-rolled fixed-position panel + watch
               — reka's own flip/shift positioning, plus outside-click and Escape
               dismissal it did not have before. Open auto-focus lands on Cancel, never on the
               destructive button. -->
        <Popover
          :open="!!forceDeleteRefCandidate"
          @update:open="(v) => !v && (forceDeleteRefCandidate = undefined)"
        >
          <PopoverAnchor as-child>
            <span
              class="fixed size-0"
              aria-hidden="true"
              :style="{
                left: `${forceDeleteRefCandidate?.x ?? 0}px`,
                top: `${forceDeleteRefCandidate?.y ?? 0}px`,
              }"
            />
          </PopoverAnchor>
          <PopoverContent
            align="start"
            :side-offset="0"
            class="w-auto flex-row items-center gap-1 px-2 py-1"
            destructive
          >
            <span>{{
              t('git.common.forceDelete.body', { name: forceDeleteRefCandidate?.name ?? '' })
            }}</span>
            <Button variant="danger" size="kira" @click="confirmForceDeleteRef">{{
              t('git.common.forceDelete.confirm')
            }}</Button>
            <Button
              data-dialog-dismiss
              variant="toolbar"
              size="kira"
              @click="forceDeleteRefCandidate = undefined"
            >
              {{ t('common.action.cancel.label') }}
            </Button>
          </PopoverContent>
        </Popover>
        <RenameRefDialog
          :open="renameRefDialogState.open"
          :current-name="renameRefDialogState.currentName"
          :ops="opsState"
          @close="renameRefDialogState = { open: false, currentName: '' }"
        />
        <BranchDialog
          :open="branchDialogState.open"
          :start-point="branchDialogState.startPoint"
          :ops="opsState"
          @close="branchDialogState = { open: false, startPoint: '' }"
        />
        <TagDialog
          :open="tagDialogState.open"
          :target="tagDialogState.target"
          :existing-tags="refsState.tags.value"
          :ops="opsState"
          @close="tagDialogState = { open: false, target: '' }"
        />
        <CheckoutDialog :ops="opsState" />
        <RevertDialog :ops="opsState" />
        <ResetDialog :ops="opsState" />
        <CherryPickDialog :ops="opsState" />
        <ForcePushDialog :ops="opsState" />
        <PullDialog :ops="opsState" />
        <PostCheckoutPullDialog :ops="opsState" />
        <StashDialog
          :ops="opsState"
          :create-open="stashCreateOpen"
          :include-untracked-default="stashIncludeUntrackedDefault"
          :branch-target="stashBranchTarget"
          :save-open="globalStashSaveOpen"
          :save-source-entry="globalStashSaveSourceEntry"
          @close="
            stashCreateOpen = false;
            stashBranchTarget = undefined;
            globalStashSaveOpen = false;
            globalStashSaveSourceEntry = undefined;
          "
        />
        <WorktreeDialog
          :worktrees="worktreeState"
          :ops="opsState"
          :refs="refsState"
          :create-request="worktreeCreateRequest"
          :base-path-default="worktreeBasePathDefault"
          @close="worktreeCreateRequest = undefined"
        />
        <StackDialog
          :stack="stackState"
          :ops="opsState"
          :refs="refsState"
          :target="stackDialogTarget"
          @close="stackDialogTarget = undefined"
        />
        <RepoSettingsDialog
          :open="repoSettingsDialogOpen"
          :repo-settings-state="repoSettingsState"
          @close="repoSettingsDialogOpen = false"
        />
      </template>
    </template>
  </div>
</template>
