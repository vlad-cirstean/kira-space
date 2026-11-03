import type { CommitStore, LayoutChunk, RowPlan } from '@kira/git-core';
import { identityRowPlan, projectLayoutInput } from '@kira/git-core';
import type { StreamChunkOf } from '@kira/git-ipc';
import { TransportError } from '@kira/git-ipc';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { logError } from '@workbench/bridge/log';
import { markRaw, type Ref, type ShallowRef, shallowRef, watch } from 'vue';
import type { BridgeClient } from '../bridge/client.ts';
import {
  createLayoutClient,
  type LayoutClient,
  LayoutClientStaleError,
} from '../graph/layoutClient.ts';
import { LayoutStore } from '../graph/layoutStore.ts';
import { createCancellableTimer } from './cancellableTimer.ts';
import { composeGraphFailureNotice, type FailureNotice } from './failureNotice.ts';
import type { GraphOrderState } from './graphOrder.ts';
import { composeRevealSearchHitAnnouncement, createAnnouncementRef } from './liveAnnouncements.ts';
import { type ChunkSource, PackedStreamState } from './packedStream.ts';
import { AUTO_REFRESH_COALESCE_MS, AUTO_REFRESH_MIN_GAP_MS } from './refreshTiming.ts';

export type { ChunkSource };
export type LoadingState = 'idle' | 'streaming' | 'loadingMore' | 'refreshing';

/**
 * The UI-side half of the cache/rehydration story, the module every component reads. `store` is
 * the client's own `CommitStore`, fed
 * one `graph.stream` chunk at a time via `appendPacked` — which is also where the ordering
 * guarantee lives: `CommitStore.appendPacked` throws if a chunk does not start exactly at the
 * store's current row count, so an out-of-order or duplicated chunk is a loud bug rather than
 * silently corrupted state. This file adds no ordering check of its own; the store's is the
 * only one, and re-implementing it here would be a second place to get it wrong.
 *
 * `store` and `layout` are `markRaw`'d and never become reactive — only the scalars
 * below are.
 *
 * **`generation` is the mechanism that makes a reset visible to the grid.** Nothing else
 * changes when rows are dropped and re-walked from row 0 — `loadedRows` might land on the same
 * number — and SlickGrid's row cache holds rendered rows until it is told they are stale, so it
 * would keep showing the old history. Every consumer that caches anything derived from the
 * store invalidates on `generation`.
 *
 * **Layout is driven from the append, not from the render.** After each chunk lands, the whole
 * list is laid out again and `#publish` swaps `layout`, `plan` and `laneCount` in one synchronous
 * step. `onPlanPublished` is the extension point `CommitGrid.vue` subscribes through to
 * repaint inside that same step — kept as a plain subscribe/unsubscribe callback (mirroring
 * `BridgeClient.on`) rather than a direct reference to the grid, so this file stays ignorant of
 * SlickGrid entirely. Rows can be *listed* before their lanes exist, which is correct and is what
 * makes the ≤ 300 ms first-paint budget reachable: text first, graph a frame later, never a blank
 * list waiting on a worker.
 */
export class GraphViewState {
  readonly store: CommitStore;
  readonly layout: LayoutStore;
  /** The plan `layout` was built for. Display rows move between plans in grouped mode, so a
   *  mismatch means every lane read from `layout` belongs to a different commit. */
  #layoutPlan: RowPlan;
  readonly loadedRows: ShallowRef<number>;
  readonly remaining: ShallowRef<number>;
  readonly exhausted: ShallowRef<boolean>;
  readonly lastChunkSource: ShallowRef<ChunkSource | undefined>;
  readonly laneCount: ShallowRef<number> = shallowRef(0);
  /** The current `RowPlan` (branch-grouped, or `identityRowPlan` with no `#order` attached) —
   *  `CommitGrid.vue`'s own translation sites and `columns.ts`/`graphColumn.ts` read this to
   *  convert between display rows (SlickGrid's own indexing) and store rows
   *  (`CommitStore`/`SelectionState`'s). A fresh object on every publish. */
  readonly plan: ShallowRef<RowPlan> = shallowRef(identityRowPlan(0));
  readonly loading: ShallowRef<LoadingState> = shallowRef('idle');
  readonly generation: ShallowRef<number>;
  /** W13's `revealSha` own live-region text — `App.vue` forwards it into the shared region
   *  exactly as it already does for `DetailState.announcement`/`OpsState.announcement`. */
  readonly announcement: Ref<string> = createAnnouncementRef();
  /** The latest stream failure, for the visible banner (`FailureBanner.vue`); the live region
   *  keeps announcing through `announcement`. Cleared by the next good chunk and on `reset()`. */
  readonly streamFailure: ShallowRef<FailureNotice | undefined> = shallowRef(undefined);
  /** True for exactly the duration of an auto-triggered `refresh()` — `App.vue`'s viewport
   *  capture/restore and its refresh announcement are both gated on this (D10): a background
   *  refresh must neither move the user's scroll position nor speak on every commit. */
  readonly autoRefreshing: ShallowRef<boolean> = shallowRef(false);

  readonly #packed: PackedStreamState;
  readonly #bridge: BridgeClient;
  readonly #layoutClient: LayoutClient;
  readonly #order: GraphOrderState | undefined;
  readonly #planListeners = new Set<(plan: RowPlan) => void>();
  #abortController: AbortController | undefined;
  #loadController: AbortController | undefined;
  #repoId: string | undefined;
  #layoutSubmitMarked = false;
  /** Bumped by `reset()` (repo switch). `#runLoad` captures this at entry and checks it again
   *  before its post-request resync — a bump in between means the repo switched out from under
   *  it, so its captured `repoId` is now stale and its resync must be skipped rather than
   *  reopening/resyncing the *old* repo's stream over the new one. */
  #loadGeneration = 0;
  /** F11: a chunk folded in since the last relayout started — `#queueLayoutRebuild`'s own doc
   *  comment. */
  #layoutPending = false;
  #layoutDraining = false;
  #layoutDrain: Promise<void> | undefined;

  readonly #unsubscribeChanged: () => void;
  readonly #unsubscribeLoading: () => void;
  readonly #autoRefreshTimer = createCancellableTimer(() => {
    void this.#runAutoRefresh();
  }, AUTO_REFRESH_COALESCE_MS);
  #autoRefreshPending = false;
  #suspended = false;
  #lastAutoRefreshAt = 0;

  /** `order` is the `GraphOrderState` — optional, since not every host of this class needs
   *  branch grouping (a caller that omits it gets the identity plan). When given, `#rebuildLayout`
   *  (below) rebuilds its plan on every call —
   *  see `rebuildOrder()`'s own doc comment for why that is a plain method, not a reactive watch
   *  over `order.revision`. */
  constructor(
    bridge: BridgeClient,
    layoutClient: LayoutClient = createLayoutClient(),
    order?: GraphOrderState,
  ) {
    this.#bridge = bridge;
    this.#layoutClient = layoutClient;
    this.#order = order;
    this.#layoutPlan = this.plan.value;
    this.#packed = new PackedStreamState({ stageRestarts: true });
    this.store = this.#packed.store;
    this.loadedRows = this.#packed.loadedRows;
    this.remaining = this.#packed.remaining;
    this.exhausted = this.#packed.exhausted;
    this.lastChunkSource = this.#packed.lastChunkSource;
    this.generation = this.#packed.generation;
    this.layout = markRaw(new LayoutStore());

    this.#unsubscribeChanged = bridge.on('repo.changed', (event) => {
      // `worktreeChanged` (an index write) cannot change committed history — nothing to refresh.
      if (event.kind !== 'refsChanged') return;
      if (event.repoId !== this.#repoId) return;
      this.#scheduleAutoRefresh();
    });
    // Rule: a signal that arrives while a load-shaped operation is running (loadMore/loadAll/
    // revealSha/a manual refresh) is never dropped — once `loading` returns to idle, whatever
    // auto-refresh was deferred runs. This is what makes auto-refresh "always", not "usually".
    this.#unsubscribeLoading = watch(this.loading, (state) => {
      if (state !== 'idle' || !this.#autoRefreshPending) return;
      this.#autoRefreshPending = false;
      this.#scheduleAutoRefresh();
    });
  }

  /**
   * Opens `graph.stream` for `repoId`. `resumeThroughRow` defaults to this store's own current
   * row count, which is exactly what a post-remount rehydration needs: a freshly constructed
   * `GraphViewState` (the only kind that exists right after a remount) starts at 0, so the default
   * asks the host to replay every row it still has cached from row 0 — the single round trip that
   * is "rehydrates without re-running git" from the UI's side. The same default also makes a
   * same-session reconnect (the store already holds N rows) resume from N instead of
   * re-fetching them.
   *
   * Supersedes any still-open stream on this instance: aborting the previous
   * `#abortController` ends it. A restart at row 0 streams into a staging store; the list swaps
   * to it in one step once it holds as many rows as before or the stream ends.
   */
  async openStream(
    repoId: string,
    resumeThroughRow: number = this.loadedRows.value,
  ): Promise<void> {
    this.#repoId = repoId;
    this.#abortController?.abort();
    const controller = new AbortController();
    this.#abortController = controller;
    if (this.loading.value === 'idle') this.loading.value = 'streaming';
    let ended = false;
    try {
      await this.#bridge.stream(
        'graph.stream',
        { repoId, resumeThroughRow },
        (chunk) => this.#applyChunk(chunk),
        controller.signal,
      );
      ended = !controller.signal.aborted;
    } finally {
      if (this.#abortController === controller) {
        if (ended) {
          if (this.#packed.finishStaging()) this.#queueLayoutRebuild();
        } else {
          this.#packed.discardStaging();
        }
      }
      // The plan lags the store by the in-flight relayout; going idle first would let the
      // viewport restore on `loading` -> 'idle' (App.vue) run against a plan that is too short.
      await this.#layoutDrain;
      if (this.#abortController === controller) {
        this.#abortController = undefined;
        if (this.loading.value === 'streaming') this.loading.value = 'idle';
      }
    }
  }

  /**
   * Loads `pages` more pages (default 1) and folds them into the store — the flow the
   * `LoadMoreButton.vue` drives: `graph.loadMore` reads pages into the *host's* store and
   * returns without pushing rows; the rows only arrive by re-opening `graph.stream` from
   * `loadedRows`, which the host then answers entirely from cache. Two round trips, and the
   * right shape: the alternative (one stream kept open for the session) makes the host hold a
   * stream across a webview disposal it cannot observe.
   *
   * Idempotent while already loading ("a second press is a no-op, not a queued second
   * page") — including while `loadAll()`'s own loop is running, since it calls this directly.
   */
  async loadMore(pages = 1): Promise<void> {
    const repoId = this.#repoId;
    if (!repoId || this.loading.value !== 'idle') return;
    const controller = new AbortController();
    this.#loadController = controller;
    try {
      await this.#runLoad('loadingMore', () =>
        this.#bridge.request('graph.loadMore', { repoId, pages }, controller.signal),
      );
    } finally {
      if (this.#loadController === controller) this.#loadController = undefined;
    }
  }

  /** Loops `loadMore` until the host reports the history exhausted (the Alt-click "loads
   *  everything"). Appends progressively — each `loadMore` call still folds its page in via the
   *  usual re-open-and-stream path — so the list stays live throughout rather than jumping once
   *  at the end. `cancelLoad()` stops it between pages; the page in flight when cancelled still
   *  completes and is kept ("rows already read are kept") — the same one
   *  `AbortController` spans the whole loop so a cancel reaches whichever page is currently in
   *  flight, not just the next one. */
  async loadAll(): Promise<void> {
    const repoId = this.#repoId;
    if (!repoId || this.loading.value !== 'idle') return;
    const controller = new AbortController();
    this.#loadController = controller;
    try {
      while (!this.exhausted.value && !controller.signal.aborted) {
        const before = this.loadedRows.value;
        await this.#runLoad('loadingMore', () =>
          this.#bridge.request('graph.loadMore', { repoId, pages: 1 }, controller.signal),
        );
        // A termination guard independent of the exhaustion signal — a page that
        // appended no rows means there are no more rows, whatever `exhausted` says. Without
        // this, a walk whose re-stream never reports exhausted spins forever, repacking
        // and re-emitting the whole history every iteration.
        if (this.loadedRows.value === before) break;
      }
    } finally {
      // F1: same identity guard loadMore()/revealSha() already use — reset() (repo switch) may
      // have already replaced #loadController with a newer one (or cleared it), which this call
      // must not clobber.
      if (this.#loadController === controller) this.#loadController = undefined;
    }
  }

  /**
   * A tail search hit's `row` is `-1` (`SearchState.CommitHit`'s own doc comment) until this
   * resolves it into a real, loaded row ("selecting such a result loads the pages up to it"),
   * implemented as `loadAll()`'s exact loop shape (one shared `AbortController`, so a single
   * cancel reaches whichever page is currently in flight) but exiting the moment the sha appears
   * rather than only at exhaustion. `signal` is the caller's own (`SearchState`/`App.vue`) —
   * aborting it stops the loop between pages, the same "the page already in flight still completes
   * and is kept" guarantee `cancelLoad()` gives `loadMore`/`loadAll`, since it aborts the very
   * controller the in-flight request was given.
   *
   * Returns `"cancelled"` rather than `"notFound"` when the signal — not the history — is what
   * stopped the loop, so a caller does not misreport a cancellation as "this commit does not
   * exist"; also `"cancelled"` if another load-shaped operation is already running (the same
   * idempotency `loadMore`/`loadAll` already enforce) or there is no open repo, since neither is
   * this call's own failure to report. Announces through `liveAnnouncements.ts` — see
   * `composeRevealSearchHitAnnouncement`'s own doc comment for why only "loading" and "notFound"
   * get text, never "found".
   */
  async revealSha(sha: string, signal: AbortSignal): Promise<'found' | 'notFound' | 'cancelled'> {
    if (this.store.rowOfSha(sha) >= 0) return 'found';
    const repoId = this.#repoId;
    if (!repoId || this.loading.value !== 'idle' || signal.aborted) return 'cancelled';
    const controller = new AbortController();
    this.#loadController = controller;
    const onExternalAbort = (): void => controller.abort();
    signal.addEventListener('abort', onExternalAbort);
    this.announcement.value = composeRevealSearchHitAnnouncement('loading');
    try {
      while (this.store.rowOfSha(sha) < 0 && !this.exhausted.value && !controller.signal.aborted) {
        const before = this.loadedRows.value;
        await this.#runLoad('loadingMore', () =>
          this.#bridge.request('graph.loadMore', { repoId, pages: 1 }, controller.signal),
        );
        // G16 D8: same no-progress termination guard as loadAll() — see its comment.
        if (this.loadedRows.value === before) break;
      }
    } finally {
      signal.removeEventListener('abort', onExternalAbort);
      if (this.#loadController === controller) this.#loadController = undefined;
    }
    if (controller.signal.aborted) return 'cancelled';
    const found = this.store.rowOfSha(sha) >= 0;
    if (!found) this.announcement.value = composeRevealSearchHitAnnouncement('notFound');
    return found ? 'found' : 'notFound';
  }

  /**
   * The refresh action: forces a full re-query bypassing every cache. `graph.refresh` only
   * marks the host's session stale — the actual re-walk happens on the `graph.stream` re-open
   * that follows, whose first chunk lands with `from: 0`, which starts a staged restart: the
   * old history stays on screen until the re-walked one is ready, then replaces it at once
   * (`PackedStreamState.commitStaged`, bumping `generation`). Idempotent while already refreshing
   * ("a second press while running is a no-op").
   * Not cancellable — unlike `loadMore`/`loadAll`, refresh has no cancel affordance, only a
   * spinner, so this does not touch `#loadController`.
   */
  async refresh(): Promise<void> {
    const repoId = this.#repoId;
    if (!repoId || this.loading.value !== 'idle') return;
    await this.#runLoad('refreshing', () => this.#bridge.request('graph.refresh', { repoId }));
  }

  /** Runs one load-shaped operation (`graph.loadMore`/`graph.refresh`) followed by the
   *  cache-only stream re-open that folds the new rows in, under one `loading` state.
   *  Shared by `loadMore`/`loadAll`/`refresh` so each keeps its own idempotency check but none
   *  duplicates the "request, then reopen, then always resync" shape — including on
   *  cancellation, where the resync is what turns "rows already read are kept" into the client
   *  seeing them. */
  async #runLoad(state: LoadingState, request: () => Promise<unknown>): Promise<void> {
    try {
      await this.#runLoadSteps(state, request);
    } catch (error) {
      if (!(error instanceof TransportError && error.code === 'cancelled')) {
        const detail = errorText(error);
        this.streamFailure.value = composeGraphFailureNotice(
          t('git.graph.loadFailed.error'),
          detail,
          false,
        );
      }
      throw error;
    }
  }

  async #runLoadSteps(state: LoadingState, request: () => Promise<unknown>): Promise<void> {
    const repoId = this.#repoId;
    if (!repoId) return;
    const generation = this.#loadGeneration;
    this.loading.value = state;
    try {
      await request();
    } catch (error) {
      if (!(error instanceof TransportError && error.code === 'cancelled')) throw error;
      // Cancelled mid-request: fall through to the resync below anyway, so whatever the host
      // already read before the abort lands on the client instead of being silently dropped.
    } finally {
      // A reset() (repo switch) landed while `request()` was in flight — `repoId` above is
      // now the *old* repo's. This call no longer owns the load: the new repo's own reset() +
      // openStream() already drives `loading` and the stream, so resyncing here would reopen the
      // old repo's stream over it and stomp `loading` out from under it. Skip silently.
      if (generation === this.#loadGeneration) {
        try {
          await this.openStream(repoId, this.loadedRows.value);
          // A re-stream that emits nothing (the client
          // already holds every row the host has) never calls #applyChunk, so
          // exhausted/remaining would otherwise be stuck at whatever the last real chunk said —
          // which is exactly "Load the last 0". graph.status already exists and answers exactly
          // this (`{loaded, remaining, exhausted}` off the walk's own cached count), so this needs
          // no new method. A failed status call must not mask the load's own outcome
          // or leave `loading` stuck, so it is caught and logged rather than rethrown.
          try {
            const status = await this.#bridge.request('graph.status', { repoId });
            this.#packed.applyStatus(status.remaining, status.exhausted);
          } catch (statusError) {
            logError('graphView', 'graph.status failed after load', statusError);
          }
        } finally {
          if (generation === this.#loadGeneration) this.loading.value = 'idle';
        }
      }
    }
  }

  /** Aborts whichever of `loadMore`/`loadAll` is currently in flight. The resync each of them
   *  always performs in its `finally` (see `#runLoad`) is what keeps this a true
   *  cancel-and-keep-what-was-read rather than a cancel-and-lose-it. */
  cancelLoad(): void {
    this.#loadController?.abort();
  }

  /** Registers a handler called synchronously inside every publish of `plan`, `layout` and
   *  `laneCount` — W6's `CommitGrid.vue` extension point (see this class's own doc comment).
   *  Returns an unsubscribe function, mirroring `BridgeClient.on`. */
  onPlanPublished(handler: (plan: RowPlan) => void): () => void {
    this.#planListeners.add(handler);
    return () => this.#planListeners.delete(handler);
  }

  /** Clears every loaded row. Call before opening a stream for a newly *selected* repo — never
   *  needed for a fresh mount or remount, whose store already starts empty.
   *
   *  Aborts whichever `loadMore`/`loadAll`/`revealSha` is in flight and bumps
   *  `#loadGeneration`, so that call's own `#runLoad` resync (still keyed on the *old* repo)
   *  skips itself instead of reopening/resyncing the old repo's stream over the caller's own
   *  `openStream(newRepoId)` that always follows this (`App.vue`'s `handleRepoOpened`). Forces
   *  `loading` back to `'idle'` too — a superseded `#runLoad` deliberately leaves it alone now
   *  (see above), so this is what hands the state machine back to `openStream`, which otherwise
   *  only flips `'idle'` -> `'streaming'` and would leave `loading` stuck at whatever load state
   *  was running at switch time. */
  reset(): void {
    this.#loadController?.abort();
    this.#loadController = undefined;
    this.#loadGeneration++;
    this.loading.value = 'idle';
    this.streamFailure.value = undefined;
    // `#resetLayout()`'s own `#layoutClient.reset()` below turns any relayout still in flight
    // into a no-op `LayoutClientStaleError` (`#rebuildLayout`'s own catch), so a
    // `#drainLayoutRebuilds` loop already running sees nothing pending once that settles and
    // exits; a fresh one starts the moment the new repo's own stream applies a chunk.
    this.#layoutPending = false;
    this.#resetLayout();
    this.#packed.reset();
    this.#cancelAutoRefresh();
  }

  /** The stream can no longer be trusted: announce, show the banner, and ask the host to log it
   *  (fire-and-forget; a failed report must never mask the failure itself). */
  #failCorrupted(): void {
    const title = t('git.graph.corrupted.error');
    const detail = t('git.graph.corruptedDetail.error');
    this.announcement.value = `${title} ${detail}`;
    this.streamFailure.value = composeGraphFailureNotice(title, detail, true);
    const repoId = this.#repoId;
    if (repoId) {
      this.#bridge
        .request('graph.reportFailure', { repoId, reason: 'corrupted' })
        .catch((error: unknown) => {
          logError('graphView', 'reporting a corrupted stream failed', error);
        });
    }
  }

  /** Coalesces arrivals within `AUTO_REFRESH_COALESCE_MS` into one run, and enforces
   *  `AUTO_REFRESH_MIN_GAP_MS` since the last auto-refresh *completed* — a single delay covers
   *  both, since whichever bound is larger is the one that matters. A timer already pending
   *  absorbs further arrivals for free (the same "leading window" shape the server's own watcher
   *  debounce already uses). */
  #scheduleAutoRefresh(): void {
    if (this.#suspended) {
      this.#autoRefreshPending = true;
      return;
    }
    if (this.#autoRefreshTimer.pending) return;
    const sinceLastRefresh = Date.now() - this.#lastAutoRefreshAt;
    const delay = Math.max(AUTO_REFRESH_COALESCE_MS, AUTO_REFRESH_MIN_GAP_MS - sinceLastRefresh);
    this.#autoRefreshTimer.start(delay);
  }

  /** A backgrounded (hidden, kept-mounted) host defers auto-refresh: signals collapse into one
   *  pending run that starts when the host shows again. */
  setSuspended(suspended: boolean): void {
    this.#suspended = suspended;
    if (suspended || !this.#autoRefreshPending) return;
    this.#autoRefreshPending = false;
    this.#scheduleAutoRefresh();
  }

  async #runAutoRefresh(): Promise<void> {
    if (this.loading.value !== 'idle') {
      // Deferred, not dropped — the `loading` watcher above re-schedules the moment it clears.
      this.#autoRefreshPending = true;
      return;
    }
    this.autoRefreshing.value = true;
    try {
      // `graph.refresh` is nominally redundant (the host already marked the walk stale on
      // `refsChanged`), but `MarkRefresh`/`MarkStale` are not the same guarantee — reusing the
      // one refresh path is worth the one extra ~1ms request (D10).
      await this.refresh();
    } catch (error) {
      // `#scheduleAutoRefresh`'s own `void this.#runAutoRefresh()` has no caller left to hand
      // a rejection to — logging here, not in `refresh()` itself, is what stands between a
      // disconnect/git error on an auto-refresh and a silent unhandled rejection, while any future
      // direct caller of `refresh()` still sees it thrown.
      logError('graphView', '#runAutoRefresh failed', error);
    } finally {
      this.autoRefreshing.value = false;
      this.#lastAutoRefreshAt = Date.now();
    }
  }

  #cancelAutoRefresh(): void {
    this.#autoRefreshTimer.stop();
    this.#autoRefreshPending = false;
  }

  /** False in grouped mode when `plan` and `layout` differ (a collapse reset, never a rebuild:
   *  rebuilds publish both together): `graphColumn.ts` draws no lanes then rather than the other
   *  plan's. Identity mode never moves rows, so its older layout stays valid for the rows it
   *  covers. */
  get layoutCurrent(): boolean {
    return this.#order === undefined || this.#layoutPlan === this.plan.value;
  }

  #resetLayout(): void {
    this.#layoutClient.reset();
    this.#publish(identityRowPlan(0), undefined);
  }

  /** The one place `layout`, `#layoutPlan`, `plan` and `laneCount` change, and the grid repaints
   *  inside it, never later: a click or scroll in the same task as a publish always meets a grid
   *  that already shows the new plan. `clear()` and `append()` in the same synchronous block, so
   *  `laneCount` never observably passes through 0 the way it would if `clear()` ran on its own. */
  #publish(plan: RowPlan, layoutChunk: LayoutChunk | undefined): void {
    this.layout.clear();
    if (layoutChunk) this.layout.append(layoutChunk);
    this.#layoutPlan = plan;
    this.plan.value = plan;
    this.laneCount.value = this.layout.laneCount;
    for (const listener of this.#planListeners) listener(plan);
  }

  /**
   * Rebuilds `#order`'s plan (if attached — otherwise the identity plan) against
   * the store's current rows, projects it into display-row coordinates, and re-lays-out the
   * *whole* visible list in one worker pass. Replaces the old per-chunk incremental
   * `submit(store.layoutInput(from, to))`/`layout.append(...)` pair — under
   * display order a new page's rows scatter into existing groups, so there is no contiguous
   * `[from, to)` range left to append incrementally; every plan-affecting change (a page
   * landing, a group toggled, tips changing) relays out from scratch.
   *
   * Called from `#applyChunk` after every chunk, and from the public `rebuildOrder()` after
   * `App.vue` mutates `#order`'s own inputs. `#order.rebuild()` is only ever called from here —
   * one path from "an input changed" to "the plan is rebuilt", not a call site racing a reactive
   * watch over `#order.revision` to do the same work twice.
   */
  async #rebuildLayout(): Promise<void> {
    const pending = this.#packed.pendingStore;
    const source = pending ?? this.store;
    const pendingRows = source.rowCount;
    this.#order?.rebuild(source, this.generation.value);
    const plan = this.#order?.plan.value ?? identityRowPlan(source.rowCount);
    const input = projectLayoutInput(plan, source.layoutInput(0, source.rowCount));

    // The `layoutSubmitMs` — the worker round trip for the *first* relayout only, so a
    // first-paint `firstPageMs`/`worstFrameMs` miss is attributable to this hop or not in one
    // line rather than re-derived. Marked here, not measured externally, because this `await` is
    // the only place that round trip is ever isolated from the rest of this method's own work.
    const markSubmit = !this.#layoutSubmitMarked;
    if (markSubmit) performance.mark('kira:layout-submit-start');
    this.#layoutClient.reset(); // §5.3: every rebuild starts a fresh pass, never a resumed one
    let layoutChunk: LayoutChunk;
    try {
      layoutChunk = await this.#layoutClient.submit(input);
    } catch (error) {
      // A newer rebuild's own `reset()` (this method, re-entered) marked this submit stale —
      // that newer rebuild's own result is what should land, not this one. Nothing to apply.
      if (error instanceof LayoutClientStaleError) return;
      throw error;
    }
    if (markSubmit) {
      this.#layoutSubmitMarked = true;
      performance.mark('kira:layout-submit-end');
      performance.measure(
        'kira:layout-submit',
        'kira:layout-submit-start',
        'kira:layout-submit-end',
      );
    }
    // Plan and layout publish together: a row never renders against a plan whose lanes are not
    // yet in `layout` (it drew an empty graph column for the whole stream otherwise).
    if (pending === undefined) {
      this.#publish(plan, layoutChunk);
      return;
    }
    // A chunk or a newer restart changed staging meanwhile; the rebuild it queued lays out the new one.
    if (this.#packed.pendingStore !== pending || pending.rowCount !== pendingRows) return;
    this.#packed.commitStaged(() => this.#publish(plan, layoutChunk));
  }

  /** `App.vue`'s own entry point after `#order.setTips`/`.toggleGroup`/`.setCollapseEnabled` —
   *  the "a collapse toggle, a refs/HEAD change" triggers §5.1 names beside a page landing. A
   *  no-op when this instance has no `#order` attached. */
  async rebuildOrder(): Promise<void> {
    await this.#rebuildLayout();
  }

  async #applyChunk(chunk: StreamChunkOf<'graph.stream'>): Promise<void> {
    const range = await this.#packed.applyChunk(chunk, {
      onReset: () => {
        this.#layoutPending = false;
        this.#resetLayout();
      },
      onCorrupted: () => {
        // The re-open aborts this stream's own controller (`openStream`), so nothing else from
        // the corrupted sequence is applied. It runs detached: awaited here, its failure would
        // reject into the old stream's already-finished queue and never reach anyone.
        const repoId = this.#repoId;
        if (!repoId) return;
        this.openStream(repoId, 0).catch((error: unknown) => {
          if (error instanceof TransportError && error.code === 'cancelled') return;
          logError('graphView', 're-open after a corrupted chunk failed', error);
          this.#failCorrupted();
        });
      },
      onUnrecoverable: () => this.#failCorrupted(),
    });
    if (!range) return; // corrupted — already re-opening from row 0, nothing to lay out
    this.streamFailure.value = undefined;
    if (range.staged && this.#packed.pendingStore === undefined) return; // still filling

    // Folds `range` in and returns WITHOUT awaiting its own relayout — the graph plane
    // client grants the next chunk's credit only after awaiting this method's promise, so a fast
    // return here is what lets a large load (a 200k-row
    // rehydration is 400 500-row chunks) pipeline many wire chunks per relayout instead of paying
    // one full main-thread plan rebuild plus worker round trip per chunk.
    this.#queueLayoutRebuild();
  }

  /** F11: marks a relayout owed and starts a drain loop if none is already running. A relayout
   *  always lays out the WHOLE store (`#rebuildLayout`'s own doc comment), so chunks folded in
   *  while one runs need no bookkeeping beyond this flag. */
  #queueLayoutRebuild(): void {
    this.#layoutPending = true;
    if (this.#layoutDraining) return; // an already-running drain picks it up on its next loop
    this.#layoutDrain = this.#drainLayoutRebuilds();
  }

  /** F11: keeps relaying out until no further chunk landed while the last relayout was running —
   *  a stream burst that outpaces the layout worker (chunk after 500-row chunk, `loadAll`'s own
   *  loop; a cached rehydration replaying hundreds of chunks) coalesces into however many
   *  relayouts the worker actually had time to run, never one per wire chunk. */
  async #drainLayoutRebuilds(): Promise<void> {
    this.#layoutDraining = true;
    try {
      while (this.#layoutPending) {
        this.#layoutPending = false;
        await this.#rebuildLayout();
      }
    } catch (error) {
      // `#applyChunk` no longer awaits this loop (that is the whole point — see its own
      // comment), so a `dispose()` racing a relayout still in flight (the worker itself torn
      // down mid-`submit`, a non-`LayoutClientStaleError` rejection) has no caller left to hand
      // this to. Logged, matching this file's own `graph.status failed after load` precedent —
      // never rethrown, since nothing here would catch it.
      logError('graphView', '#drainLayoutRebuilds failed', error);
    } finally {
      this.#layoutDraining = false;
    }
  }

  dispose(): void {
    this.#abortController?.abort();
    this.#loadController?.abort();
    this.#layoutClient.dispose();
    this.#unsubscribeChanged();
    this.#unsubscribeLoading();
    this.#cancelAutoRefresh();
  }
}
