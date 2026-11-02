import type { BaseResolution, CommitRange, FileChange, ReviewFileStatus } from '@kira/git-ipc';
import { TransportError } from '@kira/git-ipc';
import { errorText } from '@workbench/bridge/codedError';
import { type Ref, type ShallowRef, shallowRef } from 'vue';
import type { BridgeClient } from '../bridge/client.ts';
import { DetailState } from './detail.ts';
import { createDetailActions, type DetailActions } from './detailActions.ts';
import { createAnnouncementRef } from './liveAnnouncements.ts';
import { PackedStreamState } from './packedStream.ts';

/**
 * The sidebar review view's whole state machine:
 *
 * ```
 * idle ──setTarget(branch)──> resolving ──> ask | unrelated | empty | listing
 *                                                ^                      |
 *                                                └──── setBase ─────────┘
 * ```
 *
 * Composes `PackedStreamState` with no `onReset` hook — a range walk draws no lanes, so there is
 * nothing beyond the packed columns themselves to reset. Every request is supersede-and-verify,
 * `DetailState`'s own discipline: a response checks the branch/base it was requested for is still
 * current before committing itself, because an abort racing a resolution can still resolve.
 */
export type ReviewPhase =
  | 'idle'
  | 'resolving'
  | 'ask'
  | 'unrelated'
  | 'empty'
  | 'listing'
  | 'error';

export interface ReviewExpansion {
  readonly detail: DetailState;
  readonly actions: DetailActions;
}

/** The cold-bootstrap arm, threaded from `main.ts`'s `MountOptions.target` down to
 *  `ReviewView.vue`'s own `target` prop — kept here, a plain `.ts` module, rather than exported
 *  from `ReviewView.vue` itself: a Vue SFC's `<script setup>` module shim has no named type
 *  exports (only the default component), so `main.ts`/`index.ts` could not otherwise import it
 *  by name. */
export interface ReviewTarget {
  readonly repoId: string;
  readonly branch: string;
  /** Compared against instead of the detected base (resolution reason `override`). */
  readonly base?: string;
  /** Pane to open on; default is the Commits pane. */
  readonly pane?: ReviewPane;
}

/** "The outcome or the count changed" (the mid-review `refsChanged` resolution) — compares the
 *  two things a background re-resolve can find different: which base (and thus which range
 *  shape) applies, and, when both are `ready`, how many commits are in it. `reason` is
 *  deliberately not compared: the same base re-detected via a different reason (say, an upstream
 *  that now happens to match the already-current default branch) is not a change worth a banner. */
function resolutionsDiffer(a: BaseResolution, b: BaseResolution): boolean {
  if (a.base !== b.base) return true;
  if (a.range.kind !== b.range.kind) return true;
  if (a.range.kind === 'ready' && b.range.kind === 'ready') {
    return a.range.commitCount !== b.range.commitCount;
  }
  return false;
}

/** G11 D16, extended G13 D10: which of the review sidebar's three panes is showing — a segmented
 *  control in the existing header, reset (to 'commits') by setTarget/setBase alongside everything
 *  else they already reset. */
export type ReviewPane = 'commits' | 'files' | 'comments';

export class ReviewSessionState {
  readonly phase: ShallowRef<ReviewPhase> = shallowRef('idle');
  readonly repoId: ShallowRef<string | undefined> = shallowRef(undefined);
  readonly branch: ShallowRef<string | undefined> = shallowRef(undefined);
  readonly resolution: ShallowRef<BaseResolution | undefined> = shallowRef(undefined);
  readonly pane: ShallowRef<ReviewPane> = shallowRef('commits');
  readonly resolveError: ShallowRef<string | undefined> = shallowRef(undefined);
  readonly isLoadingMore: ShallowRef<boolean> = shallowRef(false);
  /** D39: a background re-resolve found the outcome or the count changed. Never applied
   *  automatically — `acknowledgeStaleReview()` is the only thing that acts on it. */
  readonly staleReview: ShallowRef<boolean> = shallowRef(false);
  /** One shared announcement feeding one live region (`liveAnnouncements.ts`'s own precedent) —
   *  every expanded row's `DetailActions` writes here rather than each owning its own region. */
  readonly announcement: Ref<string> = createAnnouncementRef();
  readonly expandedShas: ShallowRef<ReadonlySet<string>> = shallowRef(new Set());
  /** The row-level action bundle, one per targeted session rather than one per expanded
   *  row — every bundle `#createRowActions` ever built closed over `repoId` and nothing else, so
   *  N bundles for N expanded rows were N identical closures. `undefined` until `setTarget`, which
   *  is what let a collapsed row's own "Open all changes" reach nothing at all (it read its whole
   *  bundle off `expansionFor(sha)`, `undefined` before the row was ever expanded). Set by
   *  `setTarget`, cleared by `clearTarget`. */
  readonly rowActions: ShallowRef<DetailActions | undefined> = shallowRef(undefined);

  readonly #packed: PackedStreamState;
  readonly #bridge: BridgeClient;
  readonly #expansions = new Map<string, ReviewExpansion>();
  readonly #unsubscribeChanged: () => void;
  #resolveController: AbortController | undefined;
  #streamController: AbortController | undefined;
  #loadController: AbortController | undefined;
  #checkController: AbortController | undefined;
  /** Bumped by every `setTarget`/`setBase`/`dispose` (`swapBaseAndBranch` bumps it twice,
   *  transitively, since it calls both) — `#checkForChange`'s own session-identity guard,
   *  alongside `resolution`'s object identity below. A background re-resolve started before one
   *  of those landed must never apply (or even flag a banner for) a comparison the user has
   *  already moved past. */
  #sessionGeneration = 0;
  /** The base in effect — `undefined` until a `ready`/`unrelated`/`empty` resolution
   *  lands (never set while `reason: "none"`, since there is no base at all then). Distinct
   *  from `resolution.value.base` only in that this is what `loadMore`/re-open/the background
   *  check reuse, so an override survives a background re-check
   *  ("the override holds for the session"). */
  #base: string | undefined;
  #pendingResolution: BaseResolution | undefined;

  readonly #autoApply: boolean;

  /** `autoApply`: a changed background resolution is applied at once instead of raising the D39
   *  banner — for a locked window that has no Commits pane to show the banner in. */
  constructor(bridge: BridgeClient, opts: { autoApply?: boolean } = {}) {
    this.#bridge = bridge;
    this.#autoApply = opts.autoApply ?? false;
    this.#packed = new PackedStreamState();
    this.#unsubscribeChanged = bridge.on('repo.changed', (event) => {
      if (event.kind !== 'refsChanged') return;
      if (this.repoId.value !== event.repoId) return;
      if (this.phase.value === 'idle' || this.phase.value === 'resolving') return;
      void this.#checkForChange();
    });
  }

  get store(): PackedStreamState['store'] {
    return this.#packed.store;
  }

  get loadedRows(): ShallowRef<number> {
    return this.#packed.loadedRows;
  }

  get remaining(): ShallowRef<number> {
    return this.#packed.remaining;
  }

  get exhausted(): ShallowRef<boolean> {
    return this.#packed.exhausted;
  }

  get generation(): ShallowRef<number> {
    return this.#packed.generation;
  }

  /** Targets a new branch — aborts everything in flight, clears rows and expansions, and
   *  requests `review.resolveBase` with no override. The entry point for all three of the
   *  "how the view learns which branch to review" arms: a cold bootstrap target, a
   *  `review.target` push, and the palette's own branch picker once it has one to hand over. */
  async setTarget(
    repoId: string,
    branch: string,
    open?: { readonly base?: string; readonly pane?: ReviewPane },
  ): Promise<void> {
    this.#sessionGeneration++;
    this.#abortAll();
    this.#clearExpansions();
    this.#packed.reset();
    this.repoId.value = repoId;
    this.rowActions.value = this.#createRowActions(repoId);
    this.branch.value = branch;
    this.#base = undefined;
    this.resolution.value = undefined;
    this.resolveError.value = undefined;
    this.staleReview.value = false;
    this.#pendingResolution = undefined;
    this.pane.value = open?.pane ?? 'commits';
    this.phase.value = 'resolving';
    await this.#resolve(repoId, branch, open?.base);
  }

  /** The header's swap button — re-targets the session through the two RPCs it
   *  already has, the same two-round-trip sequence a manual re-pick-then-override would already
   *  produce. Only meaningful once a `ready` comparison exists (a `null` base has nothing to
   *  swap in); a no-op otherwise. */
  async swapBaseAndBranch(): Promise<void> {
    const repoId = this.repoId.value;
    const branch = this.branch.value;
    const resolution = this.resolution.value;
    if (!repoId || !branch) return;
    if (resolution?.range.kind !== 'ready' || resolution.base === null) return;
    const oldBase = resolution.base;
    await this.setTarget(repoId, oldBase);
    await this.setBase(branch);
  }

  /** "back to branch selection" — mirrors `setTarget`'s own reset lines, but
   *  sets `phase.value = 'idle'` and, critically, clears `branch.value` back to `undefined` — the
   *  one field `setTarget` never touches, and the one flip that makes the template's own
   *  `v-else-if="!review.branch.value"` true again. `repoId.value` is left as-is: the user is
   *  still browsing the same repo, just picking a different (or the same) branch to compare. */
  clearTarget(): void {
    this.#abortAll();
    this.#clearExpansions();
    this.#packed.reset();
    this.#base = undefined;
    this.resolution.value = undefined;
    this.resolveError.value = undefined;
    this.staleReview.value = false;
    this.#pendingResolution = undefined;
    this.pane.value = 'commits';
    this.phase.value = 'idle';
    this.branch.value = undefined;
    this.rowActions.value = undefined;
  }

  /** The header picker's override (§6.8): re-resolves **with** an explicit base, exactly the
   *  same request shape a detected base gets, so a user-chosen base is checked (`merge-base`,
   *  `rev-list --count`) rather than trusted blind. */
  async setBase(base: string): Promise<void> {
    const repoId = this.repoId.value;
    const branch = this.branch.value;
    if (!repoId || !branch) return;
    this.#sessionGeneration++;
    this.#resolveController?.abort();
    this.#streamController?.abort();
    this.#loadController?.abort();
    this.#checkController?.abort();
    this.#clearExpansions();
    this.#packed.reset();
    this.resolveError.value = undefined;
    this.staleReview.value = false;
    this.#pendingResolution = undefined;
    this.pane.value = 'commits';
    this.phase.value = 'resolving';
    await this.#resolve(repoId, branch, base);
  }

  /** The Files/Commits segmented control's own setter (D16). */
  setPane(pane: ReviewPane): void {
    this.pane.value = pane;
  }

  /** §6.8's "Load more" (D42) — pages the review walk, then re-opens the stream exactly as
   *  `GraphViewState.loadMore` does for the panel: the same two-round-trip host contract, one
   *  `range` param threaded through both requests instead of none. */
  async loadMore(pages = 1): Promise<void> {
    const repoId = this.repoId.value;
    const branch = this.branch.value;
    const base = this.#base;
    if (!repoId || !branch || base === undefined || this.isLoadingMore.value) return;
    const range: CommitRange = { base, branch };
    const controller = new AbortController();
    this.#loadController = controller;
    this.isLoadingMore.value = true;
    try {
      await this.#bridge.request('graph.loadMore', { repoId, pages, range }, controller.signal);
    } catch (error) {
      if (!(error instanceof TransportError && error.code === 'cancelled')) throw error;
    } finally {
      try {
        if (this.repoId.value === repoId && this.branch.value === branch && this.#base === base) {
          await this.#open(repoId, branch, base);
        }
      } finally {
        if (this.#loadController === controller) this.#loadController = undefined;
        this.isLoadingMore.value = false;
      }
    }
  }

  /** The "comparison has changed" banner's own click handler (D39). Applies the resolution the
   *  background check already found and re-walks — the only path by which a mid-review
   *  `refsChanged` ever changes what is on screen. */
  async acknowledgeStaleReview(): Promise<void> {
    const repoId = this.repoId.value;
    const branch = this.branch.value;
    const pending = this.#pendingResolution;
    if (!repoId || !branch || !pending) return;
    this.staleReview.value = false;
    this.#pendingResolution = undefined;
    await this.#applyFresh(repoId, branch, pending);
  }

  async #applyFresh(repoId: string, branch: string, fresh: BaseResolution): Promise<void> {
    this.#abortAll();
    this.#clearExpansions();
    this.#packed.reset();
    this.resolution.value = fresh;
    this.#base = fresh.base ?? undefined;
    await this.#applyResolution(repoId, branch, fresh);
  }

  /** Re-checks the resolution now (window focus); a change follows `autoApply`. */
  checkNow(): void {
    if (this.phase.value === 'idle' || this.phase.value === 'resolving') return;
    void this.#checkForChange();
  }

  /** Expands a row (first expansion fetches `commit.detail`; a later re-expansion reuses the
   *  same, still-live `DetailState` — §6.8 step 2's "fetched on first expansion and kept for the
   *  session"). No-op with no active repo. */
  expand(sha: string): void {
    const repoId = this.repoId.value;
    const actions = this.rowActions.value;
    if (!repoId || !actions) return;
    if (this.expandedShas.value.has(sha)) return;
    this.expandedShas.value = new Set(this.expandedShas.value).add(sha);
    if (!this.#expansions.has(sha)) {
      const detail = new DetailState(this.#bridge);
      detail.setRepoId(repoId);
      this.#expansions.set(sha, { detail, actions });
      detail.select(sha);
    }
  }

  /** Collapses a row without discarding its `DetailState` — re-expanding does not re-fetch. */
  collapse(sha: string): void {
    if (!this.expandedShas.value.has(sha)) return;
    const next = new Set(this.expandedShas.value);
    next.delete(sha);
    this.expandedShas.value = next;
  }

  toggle(sha: string): void {
    if (this.expandedShas.value.has(sha)) this.collapse(sha);
    else this.expand(sha);
  }

  expansionFor(sha: string): ReviewExpansion | undefined {
    return this.#expansions.get(sha);
  }

  async #resolve(repoId: string, branch: string, base: string | undefined): Promise<void> {
    this.#resolveController?.abort();
    const controller = new AbortController();
    this.#resolveController = controller;
    const stillCurrent = (): boolean =>
      this.repoId.value === repoId && this.branch.value === branch;
    try {
      const result = await this.#bridge.request(
        'review.resolveBase',
        { repoId, branch, ...(base !== undefined ? { base } : {}) },
        controller.signal,
      );
      if (!stillCurrent()) return;
      this.resolution.value = result;
      this.#base = result.base ?? undefined;
      await this.#applyResolution(repoId, branch, result);
    } catch (error) {
      if (error instanceof TransportError && error.code === 'cancelled') return;
      if (!stillCurrent()) return;
      this.phase.value = 'error';
      this.resolveError.value = errorText(error);
    } finally {
      if (this.#resolveController === controller) this.#resolveController = undefined;
    }
  }

  async #applyResolution(repoId: string, branch: string, result: BaseResolution): Promise<void> {
    switch (result.range.kind) {
      case 'ask':
        this.phase.value = 'ask';
        return;
      case 'unrelated':
        this.phase.value = 'unrelated';
        return;
      case 'empty':
        this.phase.value = 'empty';
        return;
      case 'ready': {
        this.phase.value = 'listing';
        const base = result.base;
        if (base !== null) await this.#open(repoId, branch, base);
        return;
      }
    }
  }

  /** `resumeThroughRow` defaults to this session's own currently-loaded row count — mirrors
   *  `GraphViewState.openStream`'s own default exactly: `loadMore`'s own re-open must resume past
   *  what it already applied, not restart the
   *  whole ranged walk from row 0 (the server now honors this for a ranged request the same way
   *  it always has for the graph's own). A genuine fresh open (`setTarget`/`setBase`/
   *  `acknowledgeStaleReview`, each after its own `#packed.reset()`) passes the default
   *  unchanged — 0, since there is nothing loaded yet to resume past. `#applyChunk`'s own
   *  corrupted-stream recovery passes 0 explicitly instead, the same distrust-everything restart
   *  `GraphViewState`'s own recovery already uses. */
  async #open(
    repoId: string,
    branch: string,
    base: string,
    resumeThroughRow: number = this.#packed.loadedRows.value,
  ): Promise<void> {
    this.#streamController?.abort();
    const controller = new AbortController();
    this.#streamController = controller;
    const range: CommitRange = { base, branch };
    try {
      await this.#bridge.stream(
        'graph.stream',
        { repoId, range, resumeThroughRow },
        (chunk) => this.#applyChunk(repoId, branch, base, chunk),
        controller.signal,
      );
    } finally {
      if (this.#streamController === controller) this.#streamController = undefined;
    }
  }

  async #applyChunk(
    repoId: string,
    branch: string,
    base: string,
    chunk: Parameters<PackedStreamState['applyChunk']>[0],
  ): Promise<void> {
    await this.#packed.applyChunk(chunk, {
      onCorrupted: () => {
        if (this.repoId.value === repoId && this.branch.value === branch && this.#base === base) {
          // Distrust everything and restart from row 0 — the same recovery GraphViewState's own
          // onCorrupted uses, not the default (current loadedRows), since what's already applied
          // is exactly what a corrupted chunk casts doubt on. Detached so a failure of the
          // re-open is not swallowed by this (finished) stream's queue.
          this.#open(repoId, branch, base, 0).catch((error: unknown) => {
            if (error instanceof TransportError && error.code === 'cancelled') return;
            this.#failStream(repoId, branch, base, error);
          });
        }
      },
      onUnrecoverable: (error) => this.#failStream(repoId, branch, base, error),
    });
  }

  #failStream(repoId: string, branch: string, base: string, error: unknown): void {
    if (this.repoId.value !== repoId || this.branch.value !== branch || this.#base !== base) return;
    this.phase.value = 'error';
    this.resolveError.value = errorText(error);
  }

  async #checkForChange(): Promise<void> {
    const repoId = this.repoId.value;
    const branch = this.branch.value;
    const current = this.resolution.value;
    if (!repoId || !branch || !current) return;
    // A real setTarget/setBase (a user picking a different base than the one this check
    // started against) landing while this request is in flight must never have its OWN resolve's
    // fresh-but-now-stale result flag a banner for, or (via a since-superseded #pendingResolution)
    // silently apply on click, a comparison the user has already moved past. The session token
    // catches "the target/base changed"; `current`'s own object identity is the narrower guard —
    // `setBase` reassigns `resolution.value` to a NEW object on the SAME repoId/branch, which the
    // token alone already covers, but checking identity too costs nothing and matches this
    // class's own "supersede and verify" discipline (the class doc comment) exactly.
    const generation = this.#sessionGeneration;
    this.#checkController?.abort();
    const controller = new AbortController();
    this.#checkController = controller;
    try {
      const fresh = await this.#bridge.request(
        'review.resolveBase',
        { repoId, branch, ...(this.#base !== undefined ? { base: this.#base } : {}) },
        controller.signal,
      );
      if (generation !== this.#sessionGeneration) return;
      if (this.resolution.value !== current) return;
      if (this.repoId.value !== repoId || this.branch.value !== branch) return;
      if (!resolutionsDiffer(current, fresh)) return;
      if (this.#autoApply) {
        await this.#applyFresh(repoId, branch, fresh);
        return;
      }
      this.#pendingResolution = fresh;
      this.staleReview.value = true;
    } catch (error) {
      if (error instanceof TransportError && error.code === 'cancelled') return;
      // A background check nobody asked for failing is not itself surfaced — the view stays
      // exactly as it was, silently, until the next `refsChanged` tries again.
    } finally {
      if (this.#checkController === controller) this.#checkController = undefined;
    }
  }

  // P115 H6: was a hand-built duplicate of `createDetailActions` (`detailActions.ts`) — every
  // method here closed over the same fixed `repoId` `createDetailActions` reads through a
  // function, and never passed `fallbackSha` (stash-only, so sharing this way is safe).
  #createRowActions(repoId: string): DetailActions {
    return createDetailActions(
      this.#bridge,
      (text) => {
        this.announcement.value = text;
      },
      () => repoId,
    );
  }

  #clearExpansions(): void {
    for (const { detail } of this.#expansions.values()) detail.dispose();
    this.#expansions.clear();
    this.expandedShas.value = new Set();
  }

  #abortAll(): void {
    this.#resolveController?.abort();
    this.#streamController?.abort();
    this.#loadController?.abort();
    this.#checkController?.abort();
  }

  dispose(): void {
    this.#sessionGeneration++;
    this.#abortAll();
    this.#clearExpansions();
    this.#unsubscribeChanged();
  }
}

/** What a host gets from `onReviewApi`: open a file's diff, and walk the list in its shown order. */
export interface ReviewApi {
  selectFile(path: string): void;
  /** The path after/before `path` in the list as shown (tree order or flat, filter applied), or
   *  null at either end or when `path` is not listed. */
  adjacentFile(path: string, dir: 'next' | 'previous'): string | null;
  /** Every committed list path, unfiltered; reactive. */
  committedPaths(): readonly string[];
  /** Re-reads the file list and re-checks the base: for a host that may have missed a ref event. */
  refresh(): void;
}

/** Uncommitted changes a host shows under the committed list (review window), reviewed apart from
 *  the committed ones. `onOpen` opens the host's own diff for the path. */
export interface ReviewWorkingSection {
  readonly files: readonly FileChange[];
  readonly reviewStates: ReadonlyMap<string, ReviewFileStatus>;
  onOpen(path: string, pinned: boolean): void;
  onToggleReviewed(path: string, reviewed: boolean): void;
  onToggleReviewedFolder(paths: readonly string[], reviewed: boolean): void;
}
