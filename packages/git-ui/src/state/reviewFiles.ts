import type {
  FileChangeKind,
  LineRange,
  ResultOf,
  ReviewDeltaSource,
  ReviewDiffMode,
  ReviewFileEntry,
} from '@kira/git-ipc';
import { t } from '@theme/i18n';
import { createLatestRequest, runLatest } from '@workbench/behaviour/latest';
import { errorText } from '@workbench/bridge/codedError';
import { type ShallowRef, shallowRef } from 'vue';
import type { BridgeClient } from '../bridge/client.ts';
import { createCancellableTimer } from './cancellableTimer.ts';
import { isCancelledTransport } from './isCancelled.ts';
import { AUTO_REFRESH_COALESCE_MS } from './refreshTiming.ts';

export type ReviewFileDiffBody = ResultOf<'review.fileDiff'>['body'];

/** editor.openRangeDiff's status is a four-way enum; `copied`/`typeChanged`/`unmerged`
 *  fold to `modified` because the handler treats them identically — both sides present, `path`
 *  (and `originalPath` when set) name them. Only `added` (no base side) and `deleted` (no branch
 *  side) change which side is `{kind: 'empty'}`. */
function rangeDiffStatus(kind: FileChangeKind): 'added' | 'deleted' | 'modified' | 'renamed' {
  switch (kind) {
    case 'added':
    case 'deleted':
    case 'renamed':
      return kind;
    default:
      return 'modified';
  }
}

/** The (repo, branch, base) triple review.files/review.fileDiff/review.mark need — base only for
 *  the two read methods (G11 D5: review.mark carries no base at all, a write is a fact about
 *  (repo, branch, path) only). */
export interface ReviewFilesTarget {
  readonly repoId: string;
  readonly branch: string;
  readonly base: string;
}

/**
 * The review sidebar's Files pane — `DetailState`'s own supersede-and-verify discipline,
 * applied to review.files/review.fileDiff/review.mark instead of commit.detail/commit.fileDiff.
 * Kept as its own class (not folded into `ReviewSessionState`) since it answers a different
 * question (which files has THIS branch's diff, and what state do they carry) than the commit
 * list does, and the two panes are mutually exclusive tabs, never shown together.
 */
export class ReviewFilesState {
  readonly files: ShallowRef<readonly ReviewFileEntry[]> = shallowRef([]);
  readonly loading: ShallowRef<boolean> = shallowRef(false);
  readonly loadError: ShallowRef<string | undefined> = shallowRef(undefined);
  // G13 D8/F15: kept, not discarded — the two shas editor.openRangeDiff's reshaped params need.
  // `branchTip` is the right-hand revision; `mergeBase` is `range` mode's correct left-hand one
  // (F7: review.files' own three-dot set, not a two-dot diff against `target.base`).
  #branchTip: string | undefined;
  #mergeBase: string | undefined;

  /** `null` ⇒ the list is showing; a path ⇒ that file's diff has taken over the pane. */
  readonly selectedPath: ShallowRef<string | null> = shallowRef(null);
  readonly diffMode: ShallowRef<ReviewDiffMode> = shallowRef('sinceReview');
  readonly deltaSource: ShallowRef<ReviewDeltaSource | undefined> = shallowRef(undefined);
  readonly body: ShallowRef<ReviewFileDiffBody | undefined> = shallowRef(undefined);
  readonly reviewedRanges: ShallowRef<readonly LineRange[]> = shallowRef([]);
  /** G12 D12: the base editor.openRangeDiff uses in `sinceReview` mode — `null` means "never
   *  reviewed", the fallback-to-`range` case. */
  readonly reviewedAtSha: ShallowRef<string | null | undefined> = shallowRef(undefined);
  readonly diffError: ShallowRef<string | undefined> = shallowRef(undefined);

  /** A review.mark request in flight or queued — the two header buttons disable themselves while
   *  true rather than let a double-click race two writes against the same file. */
  readonly pending: ShallowRef<boolean> = shallowRef(false);
  // mark()'s own request had no catch at
  // all — a rejection propagated straight out of mark() as a rejected promise, and every caller
  // (ReviewFilesPane.vue's onToggleReviewed among them) calls it as `void mark(...)`, discarding
  // that promise outright. The failure became an unhandled rejection with nothing user-visible:
  // the checkbox just silently reverted to its pre-click state on the next render with no
  // explanation. Mirrors loadError/diffError's own pattern exactly.
  readonly markError: ShallowRef<string | undefined> = shallowRef(undefined);

  readonly #bridge: BridgeClient;
  #target: ReviewFilesTarget | undefined;
  /** Marks run one at a time, in call order; `#runMark` never rejects. */
  #markChain: Promise<void> = Promise.resolve();
  #markState = { count: 0 };
  #lastQueuedKey: string | undefined;
  readonly #filesRequest = createLatestRequest<ResultOf<'review.files'>>(isCancelledTransport);
  readonly #diffRequest = createLatestRequest<ResultOf<'review.fileDiff'>>(isCancelledTransport);

  readonly #unsubscribeChanged: () => void;
  readonly #reloadTimer = createCancellableTimer(() => this.reload(), AUTO_REFRESH_COALESCE_MS);

  constructor(bridge: BridgeClient) {
    this.#bridge = bridge;
    // A new commit on the branch keeps the (repo, branch, base) triple, so `setTarget` never
    // re-runs; the ref write is the only signal the list and shas went stale.
    this.#unsubscribeChanged = bridge.on('repo.changed', (event) => {
      if (event.kind !== 'refsChanged') return;
      if (this.#target?.repoId !== event.repoId) return;
      this.#reloadTimer.start();
    });
  }

  /** Called whenever the review session's own target/base changes — resets everything and, when a
   *  target is given, loads the file list. */
  setTarget(target: ReviewFilesTarget | undefined): void {
    this.#abortAll();
    this.#reloadTimer.stop();
    this.#target = target;
    this.files.value = [];
    this.#branchTip = undefined;
    this.#mergeBase = undefined;
    this.loadError.value = undefined;
    this.markError.value = undefined;
    this.selectedPath.value = null;
    this.#clearDiff();
    // mark()'s own `finally` only clears
    // `pending` when `this.#target === target` still holds for the SAME target identity that was
    // current when the request started — a `setTarget` call that lands while a mark() is in
    // flight (a base-resolution change, the stale-review banner, clicking Refresh review) swaps
    // that identity, so the guard never matches and `pending` stays latched true forever: every
    // review checkbox in the panel silently stops responding until the webview reloads. `pending`
    // is a pane-wide "is a mark in flight" flag, not a per-target one, so a fresh target — which
    // already discards everything else about the in-flight request above — discards this too.
    this.pending.value = false;
    this.#markState = { count: 0 };
    this.#markChain = Promise.resolve();
    this.#lastQueuedKey = undefined;
    if (target) void this.#loadFiles();
  }

  async #loadFiles(): Promise<void> {
    const target = this.#target;
    if (!target) return;
    await runLatest(this.#filesRequest, {
      request: (signal) =>
        this.#bridge.request(
          'review.files',
          { repoId: target.repoId, branch: target.branch, base: target.base },
          signal,
        ),
      stillCurrent: () => this.#target === target,
      onResult: (result) => {
        this.files.value = result.files;
        this.#branchTip = result.branchTip;
        this.#mergeBase = result.mergeBase;
        this.loadError.value = undefined;
        const selected = this.selectedPath.value;
        if (selected === null) return;
        if (result.files.some((e) => e.change.path === selected)) void this.#loadDiff();
        else {
          this.selectedPath.value = null;
          this.#clearDiff();
        }
      },
      onError: (message) => {
        this.loadError.value = message;
      },
      setLoading: (loading) => {
        this.loading.value = loading;
      },
    });
  }

  /** Opens path in the host's native diff — a no-op re-selection of the file already
   *  open re-opens it anyway, since the editor tab may since have been closed. `#loadDiff` runs
   *  alongside, independently: it fetches metadata this pane still renders (the delta status
   *  line, the per-file reviewed marks), never the diff body the host owns.
   *
   *  `opts.pinned` — `false`/omitted for a click or arrow-key move (navigational, the
   *  default here matches every other "selecting = opening" caller in this file), `true` for a
   *  double click/`Enter` (`FileTree.vue`'s own `openFile` emit). */
  selectFile(path: string, opts: { pinned?: boolean } = {}): void {
    const changed = this.selectedPath.value !== path;
    this.selectedPath.value = path;
    if (changed) void this.#loadDiff(); // metadata only re-fetches when the file actually changes.
    void this.#openInEditor(opts.pinned ?? false); // always re-opens: the tab may since have closed.
  }

  /** Moves the selection to a file the host already shows; never opens it. */
  followFile(path: string): void {
    if (this.selectedPath.value === path) return;
    this.selectedPath.value = path;
    void this.#loadDiff();
  }

  /** Returns to the file list without discarding which file was selected. */
  showList(): void {
    this.selectedPath.value = null;
  }

  setDiffMode(mode: ReviewDiffMode): void {
    if (this.diffMode.value === mode) return;
    this.diffMode.value = mode;
    if (this.selectedPath.value !== null) {
      void this.#loadDiff();
      // G21 D13: a mode change re-showing the same file is navigational, not a new commitment —
      // never pinned.
      void this.#openInEditor(false);
    }
  }

  /** Composes the `editor.openRangeDiff` params from data this class already holds —
   *  `sinceReview` uses the file's own `reviewedAtSha` only when it changed since that review,
   *  else `#mergeBase` (never reviewed, or reviewed and unchanged); `range` mode always uses `#mergeBase` (the
   *  three-dot set `review.files` already returns, not a two-dot diff against `target.base`).
   *  A fire-and-forget host action, not a fetch: nothing here is superseded the way `#loadDiff`'s
   *  reactive state is. */
  async #openInEditor(pinned: boolean): Promise<void> {
    const target = this.#target;
    const path = this.selectedPath.value;
    if (!target || path === null) return;
    if (this.#branchTip === undefined || this.#mergeBase === undefined) return;
    const entry = this.files.value.find((e) => e.change.path === path);
    if (!entry) return;
    const change = entry.change;
    // The file's own sha from the list, not `#loadDiff`'s shared ref, which lags the click.
    // A reviewed file with no change since keeps the full branch diff (as on GitHub): left =
    // `reviewedAtSha` would equal the tip and show the plain file.
    const review = entry.review;
    const sinceReview =
      this.diffMode.value === 'sinceReview' && review.kind !== 'none' && review.changedSinceReview
        ? review.reviewedAtSha
        : undefined;
    const leftRev = sinceReview ?? this.#mergeBase;
    const leftLabel = sinceReview ? t('git.review.lastReview.label') : target.base;
    await this.#bridge.request('editor.openRangeDiff', {
      repoId: target.repoId,
      branch: target.branch,
      branchTip: this.#branchTip,
      leftRev,
      leftLabel,
      path,
      ...(change.originalPath !== undefined ? { originalPath: change.originalPath } : {}),
      status: rangeDiffStatus(change.kind),
      pinned,
    });
  }

  async #loadDiff(): Promise<void> {
    const target = this.#target;
    const path = this.selectedPath.value;
    if (!target || path === null) return;
    const mode = this.diffMode.value;
    this.diffError.value = undefined;
    await runLatest(this.#diffRequest, {
      request: (signal) =>
        this.#bridge.request(
          'review.fileDiff',
          { repoId: target.repoId, branch: target.branch, base: target.base, path, mode },
          signal,
        ),
      stillCurrent: () =>
        this.#target === target && this.selectedPath.value === path && this.diffMode.value === mode,
      onResult: (result) => {
        this.deltaSource.value = result.deltaSource;
        this.body.value = result.body;
        this.reviewedRanges.value = result.reviewedRanges;
        this.reviewedAtSha.value = result.reviewedAtSha;
      },
      onError: (message) => {
        this.diffError.value = message;
      },
    });
  }

  #clearDiff(): void {
    this.deltaSource.value = undefined;
    this.body.value = undefined;
    this.reviewedRanges.value = [];
    this.reviewedAtSha.value = undefined;
    this.diffError.value = undefined;
  }

  /**
   * Marks (or unmarks) path — the whole file when ranges is omitted, a sub-range otherwise.
   * Applies the returned status to the in-memory file list directly, so the list updates
   * from the response rather than a second review.files round trip; when path is the file
   * currently open, its diff is re-fetched too (a mark can change reviewedRanges/deltaSource for
   * the very diff the user is looking at).
   */
  async mark(path: string, reviewed: boolean, ranges?: readonly LineRange[]): Promise<void> {
    const target = this.#target;
    if (!target) return;
    // Queued, never dropped: a second tick while one is in flight (the tree's checkboxes are not
    // disabled on `pending`) must still reach the server, in order. Only an identical whole-file
    // repeat of the newest queued mark is a no-op.
    const key = ranges === undefined ? `${path}\0${reviewed}` : undefined;
    if (key !== undefined && key === this.#lastQueuedKey) return;
    this.#lastQueuedKey = key;
    const state = this.#markState;
    state.count++;
    this.pending.value = true;
    const run = this.#markChain.then(() => this.#runMark(target, state, path, reviewed, ranges));
    this.#markChain = run;
    await run;
  }

  /** Whole-file mark of several paths in one `review.markMany` call, queued with `mark()`. A failure
   *  reloads the list: earlier files in the batch may have been written. */
  async markMany(paths: readonly string[], reviewed: boolean): Promise<void> {
    const target = this.#target;
    if (!target || paths.length === 0) return;
    this.#lastQueuedKey = undefined;
    const state = this.#markState;
    state.count++;
    this.pending.value = true;
    const run = this.#markChain.then(() => this.#runMarkMany(target, state, paths, reviewed));
    this.#markChain = run;
    await run;
  }

  async #runMarkMany(
    target: ReviewFilesTarget,
    state: { count: number },
    paths: readonly string[],
    reviewed: boolean,
  ): Promise<void> {
    try {
      if (this.#target !== target) return;
      this.markError.value = undefined;
      try {
        const { results } = await this.#bridge.request('review.markMany', {
          repoId: target.repoId,
          branch: target.branch,
          paths,
          reviewed,
        });
        if (this.#target !== target) return;
        const byPath = new Map(results.map((r) => [r.path, r.review]));
        this.files.value = this.files.value.map((entry) => {
          const review = byPath.get(entry.change.path);
          return review ? { ...entry, review } : entry;
        });
        const selected = this.selectedPath.value;
        if (selected !== null && byPath.has(selected)) await this.#loadDiff();
      } catch (error) {
        if (this.#target !== target) return;
        this.markError.value = errorText(error);
        this.reload();
      }
    } finally {
      state.count--;
      if (state === this.#markState && state.count === 0) {
        this.pending.value = false;
        this.#lastQueuedKey = undefined;
      }
    }
  }

  async #runMark(
    target: ReviewFilesTarget,
    state: { count: number },
    path: string,
    reviewed: boolean,
    ranges: readonly LineRange[] | undefined,
  ): Promise<void> {
    try {
      if (this.#target !== target) return;
      this.markError.value = undefined;
      const result = await this.#bridge.request('review.mark', {
        repoId: target.repoId,
        branch: target.branch,
        path,
        reviewed,
        ...(ranges !== undefined ? { ranges } : {}),
      });
      if (this.#target !== target) return;
      this.files.value = this.files.value.map((entry) =>
        entry.change.path === path ? { ...entry, review: result.review } : entry,
      );
      if (this.selectedPath.value === path) await this.#loadDiff();
    } catch (error) {
      if (this.#target !== target) return;
      this.markError.value = errorText(error);
    } finally {
      state.count--;
      if (state === this.#markState && state.count === 0) {
        this.pending.value = false;
        this.#lastQueuedKey = undefined;
      }
    }
  }

  /** Re-runs `#loadFiles` against the current target — the review sidebar's own answer to
   *  an editor-side range mark, which emits no `repo.changed` (it touches `review.db`, not refs)
   *  and would otherwise leave the file list stale until an unrelated repo change. A no-op with no
   *  target set. */
  reload(): void {
    if (this.#target) void this.#loadFiles();
  }

  dispose(): void {
    this.#reloadTimer.stop();
    this.#unsubscribeChanged();
    this.#abortAll();
  }

  #abortAll(): void {
    this.#filesRequest.abort();
    this.#diffRequest.abort();
  }
}
