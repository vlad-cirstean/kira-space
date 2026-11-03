import { canRunOp, classifyReset } from '@kira/git-core';
import type {
  AutoFetchStatus,
  CheckoutPreflight,
  CherryPickPreflight,
  HeadState,
  InProgressOperation,
  OpErrorKind,
  OpRequest,
  OpResult,
  PullPreflight,
  PullStrategy,
  PullStrategySource,
  PushPreflight,
  RefRow,
  RemoteOpKind,
  RemoteOpParams,
  RemoteOpResult,
  RemoteProgress,
  ResetMode,
  ResetPreflight,
  RestackResult,
  RevertPreflight,
  StashBranchPreflight,
  StashEntry,
  StashPopPreflight,
  StatusSummary,
  UndoSlotSnapshot,
} from '@kira/git-ipc';
import { TransportError } from '@kira/git-ipc';
import { type MessageKey, t } from '@theme/i18n';
import { createLatestRequest } from '@workbench/behaviour/latest';
import { errorText } from '@workbench/bridge/codedError';
import { type Ref, type ShallowRef, shallowRef } from 'vue';
import type { BridgeClient } from '../bridge/client.ts';
import { stashLabel } from '../lib/stashListModel.ts';
import { composeFailureNotice, type FailureNotice } from './failureNotice.ts';
import { isCancelledTransport } from './isCancelled.ts';
import {
  type CherryPickPredictionMismatch,
  composeAutoDetachAnnouncement,
  composeAutoFetchStoppedAnnouncement,
  composeAutoStashAnnouncement,
  composeCheckoutAnnouncement,
  composeCherryPickAnnouncement,
  composeCherryPickMismatchAnnouncement,
  composeOpFailureAnnouncement,
  composeResetAnnouncement,
  composeRestackAnnouncement,
  composeRevertAnnouncement,
  composeStashAnnouncement,
  composeStashPushAnnouncement,
  composeUndoAnnouncement,
  createAnnouncementRef,
  type OpAction,
  type OpFailureError,
  opFailedTitle,
  type StashPredictionMismatch,
  stashRef,
} from './liveAnnouncements.ts';
import { createPendingSlot, type PendingSlot } from './pendingSlot.ts';
import type { RefsState } from './refs.ts';
import { RepoScopedReload } from './repoScopedReload.ts';
import type { RepoSettingsState } from './repoSettings.ts';
import type { StackState } from './stack.ts';

export type { StashPredictionMismatch } from './liveAnnouncements.ts';

/** `StashPopPreflight.prediction`'s own inline shape, named here since `@kira/git-ipc` has no
 *  standalone export for it (it is a structural copy of core's `MergeOutcomePrediction`, inlined
 *  at each of its two call sites rather than given its own top-level name). */
type StashPrediction = StashPopPreflight['prediction'];

/** `StashDialog.vue`'s own pending state for the shared apply/pop confirmation (one dialog,
 *  the verb and one sentence differing) — opened only when `preflight.stashPop`'s verdict is not
 *  `"clean"` (mirrors `RevertDialog`'s own threshold). There is no route data beyond
 *  proceed/cancel: `restoreIndex` is chosen by the caller before the preflight round trip even
 *  starts (`runStashApply`/`runStashPop`'s own parameter), since it is a user preference the
 *  merge-tree prediction never depends on, not a hazard the dialog discovers. */
export interface PendingStashPop {
  readonly verb: 'apply' | 'pop';
  readonly preflight: StashPopPreflight;
}

/** The common subset of `OpResult` and `RemoteOpResult` the `stashAndCarry` route reads —
 *  `checkout` and `pull` return different result shapes (`undo` vs.
 *  `updates`), but both carry exactly this much, which is all `#stashAndCarry` needs from
 *  whichever one its caller ran. */
interface CarryMiddleResult {
  readonly ok: boolean;
  readonly error: { readonly kind: OpErrorKind; readonly message: string } | undefined;
  readonly head: HeadState;
  readonly inProgress: InProgressOperation | null;
}

/** The route a confirmed `ForcePushDialog.vue` takes. `plain: true` selects the second,
 *  differently-worded confirmation (plain `--force`); `plain: false` is the default
 *  `--force-with-lease --force-if-includes` path. `confirmToken` is the typed branch name,
 *  present only when `PushPreflight.protectedBy` was non-null — re-verified host-side, so a
 *  wrong value here surfaces as an ordinary `ProtectedBranch` failure, not a client-side check. */
export interface ForcePushRoute {
  readonly plain: boolean;
  readonly confirmToken: string | undefined;
}

/** `ForcePushDialog.vue`'s own pending state: the preflight it renders from, plus which remote
 *  and branch it was opened for (both already known by the caller, but the dialog needs them
 *  again to build the eventual `RemoteOpParams`). */
export interface PendingForcePush {
  readonly remote: string;
  readonly branch: string;
  readonly preflight: PushPreflight;
}

/** The resolved pull strategy and its provenance, shown by the toolbar's pull-strategy picker
 *  before (and after) a pull runs ("must not discover [a rebase] by watching their history
 *  get rewritten"). Set the moment it is known — from a preflight round trip, or immediately for
 *  an explicit override the user already picked — and left in place once the op finishes so it
 *  stays visible as a record of what just ran. */
export interface PullStrategyInfo {
  readonly strategy: PullStrategy;
  readonly source: PullStrategySource;
}

/** The route a confirmed `blockedByTracked` checkout takes: discard the local changes and
 *  proceed, or (when `preflight.routes` lists it) push them to a stash first and pop them
 *  back afterward — the `stashAndCarry`. `runCheckout` only ever opens the dialog for
 *  `"blocked"` at all (clean and cleanCarry proceed with no prompt). */
export type CheckoutRoute = { readonly kind: 'discard' } | { readonly kind: 'stashAndCarry' };

/** `PostCheckoutPullDialog.vue`'s own pending state (G-UX item 3) — everything the dialog needs
 *  to render "This branch is N commits behind `<upstream>` — pull now?" and, on confirm, for
 *  `resolvePostCheckoutPullDialog`'s caller to run the pull without a second lookup. */
export interface PostCheckoutPullPrompt {
  readonly branch: string;
  readonly remote: string;
  readonly upstreamShortName: string;
  readonly behind: number;
}

/** `RefRow.upstream` is `%(upstream)`'s bare expansion — `refs/remotes/<remote>/<name>` for a
 *  remote-tracking upstream, or `refs/heads/<name>` for the (unusual, but legal) case of a branch
 *  tracking another local branch. Only the first has a *remote* to pull from at all — `undefined`
 *  for the second is deliberate, not a gap: `runPull` has no meaning for a purely local upstream.
 */
export function remoteFromUpstreamRef(upstreamRef: string | undefined): string | undefined {
  if (upstreamRef === undefined) return undefined;
  const match = /^refs\/remotes\/([^/]+)\//.exec(upstreamRef);
  return match?.[1];
}

/** The route a confirmed revert takes: the chosen mainline (present only when the preflight's
 *  `mainlineRequired` was non-empty) and whether to stop short of committing. */
export interface RevertRoute {
  readonly mainline: number | undefined;
  readonly noCommit: boolean;
}

/** The route a confirmed `ResetDialog.vue` takes: `mode` is the radio the dialog settled on
 *  (`runReset`'s own `mode` argument is only the *initial* pre-flight's mode — the mixed
 *  default — the dialog may change it client-side via `previewResetMode`, never a second round
 *  trip). `stashFirst` selects the "stash first" route (stash-then-stop, never the
 *  stash-and-carry); `token` is the typed short sha, present only when the chosen mode is
 *  destructive and `stashFirst` was not taken (requiring both would be theatre once the work is
 *  preserved). */
export interface ResetRoute {
  readonly mode: ResetMode;
  readonly stashFirst: boolean;
  readonly token: string | undefined;
}

/** The route a confirmed `CherryPickDialog.vue` takes — the direct structural sibling of
 *  `RevertRoute`. */
export interface CherryPickRoute {
  readonly mainline: number | undefined;
  readonly noCommit: boolean;
}

/**
 * One exported method per user-facing action, each the same four steps — pre-flight, confirm
 * (only when the pre-flight found a hazard), run, refresh/announce/surface — so no component ever
 * writes them out of order or calls `bridge.request("op.run", …)` itself.
 *
 * The **confirm** step for `checkout`/`revert` is mediated entirely inside this class: a hazard
 * pre-flight sets `pendingCheckout`/`pendingRevert`, a dialog component watches that ref and
 * renders itself, and calling `resolveCheckoutDialog`/`resolveRevertDialog` (Cancel passes
 * `null`) settles the promise `runCheckout`/`runRevert` is awaiting. Every other action
 * (`branchCreate`, `tagDelete`, …) has no pre-flight endpoint of its own — the dialog that
 * collects its input (`TagDialog.vue`, a context-menu confirmation) *is* the confirm step, and
 * has already run by the time it calls one of this class's methods.
 *
 * **The gate.** `canRun(opKind)` is `core`'s own pure `canRunOp` over this class's own
 * `statusSummary.inProgress` — never a component's `v-if` chain. **The
 * busy flag.** One in-flight operation at a time; every method below is a no-op while `busy` is
 * true, on top of the driver's own serialization — belt and braces.
 */
export class OpsState {
  readonly busy: ShallowRef<boolean> = shallowRef(false);
  readonly statusSummary: ShallowRef<StatusSummary | undefined> = shallowRef(undefined);
  /** P173: why background auto-fetch stopped; `null` while it runs. Fed by `status.get` (cold
   *  start, reconnect) and the `autoFetch.changed` event. */
  readonly autoFetch: ShallowRef<AutoFetchStatus | null> = shallowRef(null);
  /** The latest failure, for the visible banner (`FailureBanner.vue`); the live region keeps
   *  announcing through `announcement` unchanged. Cleared when a later op settles OK. */
  readonly lastFailure: ShallowRef<FailureNotice | undefined> = shallowRef(undefined);
  readonly undoSlot: ShallowRef<UndoSlotSnapshot | null> = shallowRef(null);
  /** Set after every action, success or failure — `App.vue` forwards it to the one live region,
   *  the same way it already does for `DetailState.announcement` (P5 W11). */
  readonly announcement: Ref<string> = createAnnouncementRef();

  // P107 I2-23: each slot below backs one of the six confirm-dialog quadruples this class used to
  // hand-roll separately (`pendingSlot.ts`'s own doc comment) — `pendingX` stays the public ref a
  // dialog component watches, now `#xSlot.pending` rather than its own `shallowRef`.
  readonly #checkoutSlot = createPendingSlot<CheckoutPreflight, CheckoutRoute | null>();
  readonly pendingCheckout = this.#checkoutSlot.pending;
  readonly #revertSlot = createPendingSlot<RevertPreflight, RevertRoute | null>();
  readonly pendingRevert = this.#revertSlot.pending;
  /** `docs/plans/P10.md` W10: `ResetDialog.vue`'s own pending state — unlike `pendingRevert`,
   *  opened for EVERY reset, not only a hazardous one (judgment call 18's single entry point IS
   *  the mode picker, so there is no "clean, skip the dialog" fast path to mirror here). */
  readonly #resetSlot = createPendingSlot<ResetPreflight, ResetRoute | null>();
  readonly pendingReset = this.#resetSlot.pending;
  /** `CherryPickDialog.vue`'s own pending state — a near-sibling of `pendingRevert`: opened only
   *  when the pre-flight found something worth a dialog (a blocker, a mainline choice, or the
   *  non-blocking `alreadyApplied` advisory — `runCherryPick`'s own trigger condition). */
  readonly #cherryPickSlot = createPendingSlot<CherryPickPreflight, CherryPickRoute | null>();
  readonly pendingCherryPick = this.#cherryPickSlot.pending;
  /** P9 W13: the shared apply/pop confirmation's own pending state — see `PendingStashPop`'s own
   *  doc comment on why one field, not two, covers both verbs (OQ7). */
  readonly #stashPopSlot = createPendingSlot<PendingStashPop, boolean>();
  readonly pendingStashPop = this.#stashPopSlot.pending;

  // -------------------------------------------------------------------------------------
  // Remote ops. Deliberately a sibling to the four steps above, not folded into `#runSimple` —
  // cancellation and progress are genuinely different concerns, and merging them would make the
  // local path pay for the remote path's complexity. `#applyRemoteResult` below (not
  // `#applyResult`) is the other half of that split: `RemoteOpResult` carries no `undo` field
  // at all, by construction (no remote operation ever touches the undo slot in either direction).
  // -------------------------------------------------------------------------------------

  /** Which `RemoteOpKind` is currently running against this repo, or `undefined` — the toolbar's
   *  progress affordance and its cancel button both key off this, not `busy` (which is also true
   *  for local ops the toolbar renders no progress bar for). */
  readonly activeRemoteOp: ShallowRef<RemoteOpKind | undefined> = shallowRef(undefined);
  /** Latest throttled `remote.progress` for the op named by `activeRemoteOp` — `undefined` before
   *  the first chunk arrives (a trivially small fetch/push may emit none at all; the toolbar must
   *  not read that as a stall, matching probe 6). Cleared when the op finishes. */
  readonly remoteProgress: ShallowRef<RemoteProgress | undefined> = shallowRef(undefined);
  readonly #forcePushSlot = createPendingSlot<PendingForcePush, ForcePushRoute | null>();
  readonly pendingForcePush = this.#forcePushSlot.pending;
  /** Set the moment a strategy is known (preflight response, or an explicit override) and left
   *  in place after the pull finishes — see `PullStrategyInfo`'s own doc comment. */
  readonly pullStrategy: ShallowRef<PullStrategyInfo | undefined> = shallowRef(undefined);
  /** `runPull`'s own confirm step for a `dirtyNonFastForward` blocker — mirrors
   *  `pendingCheckout`'s shape one field simpler, since `PullPreflight.routes` only ever offers
   *  `"stashAndCarry"` (its own doc comment): `resolvePullDialog`'s `boolean` says only whether
   *  to take that one route, there being no second one (no pull analogue of "discard"). */
  readonly pendingPull: ShallowRef<PullPreflight | undefined> = shallowRef(undefined);
  /** G-UX (item 3): `PostCheckoutPullDialog.vue`'s own pending state — "when checking out a
   *  branch that advanced, ask if I want to pull it too". Set only after `runCheckout` itself has
   *  finished (its `busy` hold released) — see `#maybePromptPostCheckoutPull`'s own doc comment. */
  readonly pendingPostCheckoutPull: ShallowRef<PostCheckoutPullPrompt | undefined> =
    shallowRef(undefined);

  readonly #bridge: BridgeClient;
  readonly #refs: RefsState;
  /** `runRestack`/`cancelRestack` DELEGATE to this instance's own execution
   *  (`StackState.runRestack`/`cancelRestack` already hold `restacking`/`progress` reactively,
   *  mirroring `WorktreeState`'s own "domain class holds the list and the read preview" scope) —
   *  this class only layers `busy`/`announcement` on top, the same uniform lifecycle every other
   *  mutating action here gets. Optional so a test can construct `OpsState` without one, mirroring
   *  `ChangeRequestState`'s own optional threading through `StackState`/`SearchState`. */
  readonly #stack: StackState | undefined;
  /** G28 D16: `kiraSpace.checkout.autoStash`'s own read point. Required: an omitted instance
   *  would silently pin the schema default and ignore the user's opt-out (P168 Part 18 F13). */
  readonly #repoSettings: RepoSettingsState;
  readonly #repo: RepoScopedReload;
  #resolvePull: ((proceed: boolean) => void) | undefined;
  #resolvePostCheckoutPull: ((proceed: boolean) => void) | undefined;
  readonly #unsubscribeProgress: () => void;
  readonly #unsubscribeAutoFetch: () => void;
  /** The server runs one goroutine per request, so two `refreshStatus`/`refreshUndo` calls in
   *  quick succession (two `repo.changed` events back to back) can reply out of order — each its
   *  own tracker, so a stale status reply can never be mistaken for a stale undo reply or vice
   *  versa. */
  readonly #statusRequest = createLatestRequest<StatusSummary>(isCancelledTransport);
  readonly #undoRequest = createLatestRequest<UndoSlotSnapshot | null>(isCancelledTransport);

  constructor(
    bridge: BridgeClient,
    refs: RefsState,
    repoSettings: RepoSettingsState,
    stack?: StackState,
  ) {
    this.#bridge = bridge;
    this.#refs = refs;
    this.#stack = stack;
    this.#repoSettings = repoSettings;
    // Any change at all (a ref write OR a worktree/index touch) can move `inProgress`,
    // `dirtyPaths` or the upstream ahead/behind counts — unlike `RefsState`, which only cares
    // about `refsChanged`, this refreshes on both event kinds (no `kind` filter below).
    this.#repo = new RepoScopedReload(bridge, 'gitOps', {
      onChanged: () => {
        this.#repo.fireAndForget(() => this.refreshStatus());
        // The server's undo slot is per `RepoEntry`, shared by every surface holding this
        // repo (another webview, the native app, a review view). An op run from one of those
        // replaces the slot; without this, this surface kept showing its own stale slot and got
        // NotFound on click. Previously refreshed only from `setRepoId` (this surface's own repo
        // switch), which never observes another surface's write.
        this.#repo.fireAndForget(() => this.refreshUndo());
      },
    });
    this.#unsubscribeProgress = bridge.on('remote.progress', (event) => {
      // Bound replies and events travel apart, so an event can land after the reply.
      if (this.#repo.repoId !== event.repoId || this.activeRemoteOp.value === undefined) return;
      this.remoteProgress.value = event;
    });
    this.#unsubscribeAutoFetch = bridge.on('autoFetch.changed', (event) => {
      if (this.#repo.repoId !== event.repoId) return;
      this.#setAutoFetch(event.autoFetch);
    });
  }

  setRepoId(repoId: string | undefined): void {
    this.#repo.setRepoId(repoId);
    this.activeRemoteOp.value = undefined;
    this.remoteProgress.value = undefined;
    this.pullStrategy.value = undefined;
    // Every `run*` method that opens a confirm dialog sets `busy = true` then awaits its own
    // slot's `ask()` — nothing else ever settles that Promise. Without abandoning it here, a repo
    // switch mid-dialog leaves the OLD repo's dialog open over the new repo, and `busy` stuck true
    // (every op button disabled) until the user dismisses it — confirming then does nothing (the
    // post-`ask()` repo guard each of those methods already has returns), which reads as a silent
    // failure. `this.#repo.repoId` is already the NEW repoId above, so each guard sees the mismatch
    // and its own `finally` clears `busy` the same way a real cancel would.
    this.#abandonPending();
    this.autoFetch.value = null;
    this.lastFailure.value = undefined;
    if (repoId === undefined) {
      this.statusSummary.value = undefined;
      this.undoSlot.value = null;
      return;
    }
    this.#repo.fireAndForget(() => this.refreshStatus());
    this.#repo.fireAndForget(() => this.refreshUndo());
  }

  /** F6: settles every pending confirm dialog and pull prompt with its own cancel value — shared
   *  by `setRepoId` (a repo switch) and `dispose` (the whole class going away). */
  #abandonPending(): void {
    this.#checkoutSlot.abandon(null);
    this.#revertSlot.abandon(null);
    this.#resetSlot.abandon(null);
    this.#cherryPickSlot.abandon(null);
    this.#stashPopSlot.abandon(false);
    this.#forcePushSlot.abandon(null);
    this.resolvePullDialog(false);
    this.resolvePostCheckoutPullDialog(false);
  }

  /** Pure predicate over `(inProgress, opKind)` (§7.11) — the gate every dialog and menu entry's
   *  disabled state reads, never a component-local re-derivation. */
  canRun(kind: OpRequest['kind']): boolean {
    return canRunOp(this.statusSummary.value?.inProgress ?? null, kind);
  }

  async refreshStatus(): Promise<void> {
    const repoId = this.#repo.repoId;
    if (repoId === undefined) return;
    const outcome = await this.#statusRequest.run(
      (signal) => this.#bridge.request('status.get', { repoId }, signal),
      () => this.#repo.repoId === repoId,
    );
    if (outcome.status === 'ok') {
      this.statusSummary.value = outcome.value;
      this.#setAutoFetch(outcome.value.autoFetch ?? null);
    } else if (outcome.status === 'error') throw new Error(outcome.message);
  }

  /** Announces only the running -> stopped transition, never a status refresh that repeats it. */
  #setAutoFetch(next: AutoFetchStatus | null): void {
    const was = this.autoFetch.value;
    this.autoFetch.value = next;
    if (next !== null && was === null) {
      this.announcement.value = composeAutoFetchStoppedAnnouncement(next.kind);
    }
  }

  async refreshUndo(): Promise<void> {
    const repoId = this.#repo.repoId;
    if (repoId === undefined) return;
    const outcome = await this.#undoRequest.run(
      (signal) =>
        this.#bridge.request('undo.peek', { repoId }, signal).then((result) => result.slot),
      () => this.#repo.repoId === repoId,
    );
    if (outcome.status === 'ok') this.undoSlot.value = outcome.value;
    else if (outcome.status === 'error') throw new Error(outcome.message);
  }

  /** "Review branch changes", from either menu `buildRefMenu` puts it on
   *  (`RefsPopover.vue`'s own row menu, and `App.vue`'s ref-badge menu). This is a read, not an
   *  operation — no pre-flight, no hazard, no `canRunOp` gate, no `op.run` envelope, so it does
   *  not go through `#runSimple` below; it is the one method on this class that is a
   *  named `bridge.request` (the `review.open` reveals the sidebar view and either seeds a
   *  cold resolve or pushes `review.target` to an already-open one, host-side — this class does
   *  not wait on, or need to know, which). */
  async openReview(branch: string): Promise<void> {
    const repoId = this.#repo.repoId;
    if (repoId === undefined) return;
    await this.#bridge.request('review.open', { repoId, branch });
  }

  // -------------------------------------------------------------------------------------
  // checkout
  // -------------------------------------------------------------------------------------

  /** The routing table, checked in this exact order:
   *
   *  1. `inProgressOperation` among `blockers` -> fall through to the dialog below; neither new
   *     route is ever offered by the server for that state, so this branch is "the
   *     table below finds neither route and falls through on its own", stated here for clarity.
   *  2. `routes` has BOTH `'detachHere'` and `'autoStash'` -> re-issue with
   *     `{ mode: 'detach', autoStash: true }` — both blockers (a worktree conflict AND a dirty
   *     tree) clear in one composed re-issue (the "the two routes compose client-side" case).
   *  3. `routes` has `'detachHere'` only -> `{ mode: 'detach', autoStash: false }`.
   *  4. `routes` has `'autoStash'` (only) AND the `kiraSpace.checkout.autoStash` SETTING is on
   *     -> `{ mode: <the originally requested mode>, autoStash: true }`.
   *  5. Otherwise -> the old dialog (discard / stashAndCarry / cancel), completely unchanged.
   */
  #resolveAutoCheckoutRoute(
    preflight: CheckoutPreflight,
    requestedMode: 'switch' | 'detach',
  ): { readonly mode: 'switch' | 'detach'; readonly autoStash: boolean } | null {
    if (preflight.verdict !== 'blocked') return null;
    if (preflight.blockers.some((b) => b.kind === 'inProgressOperation')) return null;
    const hasDetachHere = preflight.routes.includes('detachHere');
    const hasAutoStash = preflight.routes.includes('autoStash');
    if (hasDetachHere && hasAutoStash) return { mode: 'detach', autoStash: true };
    if (hasDetachHere) return { mode: 'detach', autoStash: false };
    const autoStashSetting = this.#repoSettings.settings.value['kiraSpace.checkout.autoStash'];
    if (hasAutoStash && autoStashSetting) return { mode: requestedMode, autoStash: true };
    return null;
  }

  /** The blocked-tracked/blocked-untracked path counts a pre-flight already reported — the exact
   *  set an auto-stash's own whole-tree `stash push` sweeps (whole-tree, never scoped), so
   *  this is the same number `composeAutoStashAnnouncement` names, computed once here rather than
   *  re-derived at the announcement call site. */
  #blockedPathCount(preflight: CheckoutPreflight): number {
    let n = 0;
    for (const b of preflight.blockers) {
      if (b.kind === 'blockedByTracked' || b.kind === 'blockedByUntracked') n += b.paths.length;
    }
    return n;
  }

  async runCheckout(target: string, mode: 'switch' | 'detach'): Promise<void> {
    const repoId = this.#repo.repoId;
    if (repoId === undefined || this.busy.value) return;
    this.busy.value = true;
    // Captured here, before ANY write below, so the post-checkout pull prompt
    // (run once `busy` is released, at the very bottom of this method) always reads the target
    // branch's ahead/behind exactly as it stood before the switch — `RefsState.branches`'s own
    // `track` field is only as fresh as the next `refs.list` reload, which the checkout's own
    // `repo.changed` event triggers moments later (the same staleness window `RefsState
    // .applyHead`'s doc comment already names for `head` itself), so there is no reason to read it
    // again after the write only to land back in that window.
    const targetTrack = this.#refs.branches.value.find((b) => b.shortName === target)?.track;
    // Set at each point below where a switch-mode checkout actually lands on `target` as a branch
    // (never a detach, never a route this method cannot cleanly resolve `result.ok` for — see
    // `#maybePromptPostCheckoutPull`'s own doc comment on why `stashAndCarry` is out of scope).
    let landedOnBranch = false;
    try {
      const preflight = await this.#bridge.request('preflight.checkout', { repoId, target, mode });
      const auto = this.#resolveAutoCheckoutRoute(preflight, mode);
      if (auto !== null) {
        // The origin branch — where the switch starts FROM, which is what an auto-stash entry is
        // tagged with (D1) — read BEFORE the write, from whatever HEAD currently is.
        const fromBranch = this.#refs.currentBranchName.value;
        const worktreePath = preflight.blockers.find(
          (b) => b.kind === 'worktreeConflict',
        )?.worktreePath;
        const result = await this.#bridge.request('op.run', {
          repoId,
          op: {
            kind: 'checkout',
            target,
            mode: auto.mode,
            discardLocalChanges: false,
            autoStash: auto.autoStash,
          },
        });
        this.#applyResult(repoId, result);
        if (!result.ok) {
          this.#fail('checkout', result.error, true);
          return;
        }
        landedOnBranch = auto.mode === 'switch';
        const parts: string[] = [];
        if (auto.autoStash && fromBranch !== undefined) {
          parts.push(composeAutoStashAnnouncement(this.#blockedPathCount(preflight), fromBranch));
        }
        if (auto.mode === 'detach' && mode !== 'detach' && worktreePath !== undefined) {
          parts.push(composeAutoDetachAnnouncement(target, worktreePath));
        }
        this.announcement.value =
          parts.length > 0 ? parts.join(' ') : composeCheckoutAnnouncement(preflight, target);
        return;
      }

      let discardLocalChanges = false;
      if (preflight.verdict === 'blocked') {
        const route = await this.#checkoutSlot.ask(preflight);
        // The active repo can change
        // while this dialog is open (e.g. "Open in graph" calls repo.open directly, independent
        // of this method's own busy hold) — proceeding on a route the user chose for a DIFFERENT
        // repo than the one now active would run this repo's write against the right repo (repoId
        // is still passed explicitly below) but apply its result to whatever repo IS now active
        // (#applyResult's own guard would in fact refuse that), so the honest outcome is to
        // abandon the route rather than silently completing an action the user has since
        // navigated away from.
        if (this.#repo.repoId !== repoId) return;
        if (route === null) {
          this.announcement.value = t('git.op.cancelled.checkout.announce');
          return;
        }
        if (route.kind === 'stashAndCarry') {
          // The `stashAndCarry` route — `discardLocalChanges: false` for the checkout half below;
          // the stash push `#stashAndCarry` runs first is what clears the tree. Handled inline
          // (not via a public `runCheckoutStashAndCarry`) so it shares this call's own `busy`
          // hold rather than needing a second one of its own. UNTOUCHED — `autoStash: false`
          // on the checkout half is a new required field, never a new behavior:
          // this route never auto-stashes, it stash-and-carries, exactly as it always has.
          await this.#stashAndCarry(
            repoId,
            () =>
              this.#bridge.request('op.run', {
                repoId,
                op: {
                  kind: 'checkout',
                  target,
                  mode,
                  discardLocalChanges: false,
                  autoStash: false,
                },
              }),
            'checkout',
            () =>
              t('git.op.checkout.announce', {
                where: mode === 'detach' ? t('git.op.detachedTarget.label', { target }) : target,
              }),
          );
          return;
        }
        discardLocalChanges = true;
      }
      const result = await this.#bridge.request('op.run', {
        repoId,
        op: { kind: 'checkout', target, mode, discardLocalChanges, autoStash: false },
      });
      this.#applyResult(repoId, result);
      if (result.ok) this.announcement.value = composeCheckoutAnnouncement(preflight, target);
      else this.#fail('checkout', result.error, true);
      landedOnBranch = result.ok && mode === 'switch';
    } catch (error) {
      this.#announceRejection('checkout', error);
    } finally {
      this.busy.value = false;
    }
    if (landedOnBranch) await this.#maybePromptPostCheckoutPull(target, targetTrack);
  }

  /** "when checking out a branch that advanced, ask if I want to pull it too" —
   *  offered only once `runCheckout`'s own `busy` hold has already been released (`runPull` below
   *  is itself gated on `!busy`, and this method may call it). `targetTrack` is the pre-switch
   *  snapshot `runCheckout` captured — `'gone'`/`undefined`/`behind <= 0` all mean nothing to ask
   *  about (no upstream, an already-deleted one, or already caught up). The remote name comes from
   *  the branch's own `upstream` ref (`refs/remotes/<remote>/<branch>`), not the toolbar's
   *  `defaultRemote` (the first remote name found at all) — a multi-remote repo could otherwise
   *  offer to pull from the wrong one. Deliberately does not cover the `stashAndCarry` route above:
   *  that route's own success is reported by `#stashAndCarry` internally, with no `result.ok` this
   *  method could key off without a larger, riskier change to a helper `runPull` also shares. */
  async #maybePromptPostCheckoutPull(
    target: string,
    targetTrack: RefRow['track'] | undefined,
  ): Promise<void> {
    if (targetTrack === undefined || targetTrack === 'gone' || targetTrack.behind <= 0) return;
    const upstreamRef = this.#refs.branches.value.find((b) => b.shortName === target)?.upstream;
    const remote = remoteFromUpstreamRef(upstreamRef);
    if (remote === undefined || upstreamRef === undefined) return;
    const proceed = await this.#confirmPostCheckoutPull({
      branch: target,
      remote,
      upstreamShortName: upstreamRef.replace(/^refs\/remotes\//, ''),
      behind: targetTrack.behind,
    });
    // G32 round-3 functional-correctness review, finding #7: unlike the other #confirm* dialogs
    // above, this method never captures its own `repoId` and needs no re-check here — its only
    // effect after the dialog resolves is `runPull(remote, target)`, which independently
    // re-validates identity via its own fresh `this.#repo.repoId` capture at entry. If the active repo
    // changed while this dialog was open, `runPull` simply pulls (or no-ops) against whatever repo
    // is active now, exactly as if the user had triggered it directly from the toolbar.
    if (!proceed) return;
    await this.runPull(remote, target);
  }

  #confirmPostCheckoutPull(prompt: PostCheckoutPullPrompt): Promise<boolean> {
    this.pendingPostCheckoutPull.value = prompt;
    return new Promise((resolve) => {
      this.#resolvePostCheckoutPull = resolve;
    });
  }

  /** The prompt's own Pull now/Not now buttons call this — `false` for Not now, matching every
   *  other confirm dialog's own `false`/`null`-for-decline convention as closely as a boolean
   *  route can. */
  resolvePostCheckoutPullDialog(proceed: boolean): void {
    this.pendingPostCheckoutPull.value = undefined;
    const resolve = this.#resolvePostCheckoutPull;
    this.#resolvePostCheckoutPull = undefined;
    resolve?.(proceed);
  }

  /** `CheckoutDialog.vue`'s own Discard/Cancel buttons call this — `null` for Cancel, matching
   *  `RevertDialog`'s own convention below. */
  resolveCheckoutDialog(route: CheckoutRoute | null): void {
    this.#checkoutSlot.resolve(route);
  }

  // -------------------------------------------------------------------------------------
  // revert
  // -------------------------------------------------------------------------------------

  /**
   * `runRevert`/`runCherryPick` share this exact flow — preflight already in hand,
   * an optional dialog for `mainline`/`noCommit` (skipped when nothing needs deciding), the
   * repo-changed guard, a `null` route cancelling with `cancelText`, then
   * `op.run` + `#applyResult` + an announce step. Push/force-push keep their own `#runRemote` (no
   * `mainline`/`noCommit`, a differently-shaped route) and use their slot directly, per the doc.
   */
  async #runPreflighted<
    TPreflight,
    TRoute extends { mainline: number | undefined; noCommit: boolean },
  >(opts: {
    repoId: string;
    preflight: TPreflight;
    needsDialog: (preflight: TPreflight) => boolean;
    slot: PendingSlot<TPreflight, TRoute | null>;
    cancelText: MessageKey;
    action: OpAction;
    buildOp: (mainline: number | undefined, noCommit: boolean) => OpRequest;
    /** `undefined` for a plain failure, which `#fail` composes. */
    announce: (result: OpResult, noCommit: boolean) => string | undefined;
  }): Promise<void> {
    const { repoId } = opts;
    let mainline: number | undefined;
    let noCommit = false;
    if (opts.needsDialog(opts.preflight)) {
      const route = await opts.slot.ask(opts.preflight);
      // G32 round-3 functional-correctness review, finding #7: same guard as runCheckout's own
      // #checkoutSlot.ask above — the active repo can change while this dialog is open.
      if (this.#repo.repoId !== repoId) return;
      if (route === null) {
        this.announcement.value = t(opts.cancelText);
        return;
      }
      mainline = route.mainline;
      noCommit = route.noCommit;
    }
    const result = await this.#bridge.request('op.run', {
      repoId,
      op: opts.buildOp(mainline, noCommit),
    });
    this.#applyResult(repoId, result);
    const text = opts.announce(result, noCommit);
    if (text !== undefined) this.announcement.value = text;
    if (result.ok) return;
    if (text === undefined) this.#fail(opts.action, result.error, true);
    else this.#noteFailure(opts.action, result.error, true);
  }

  async runRevert(shas: readonly string[]): Promise<void> {
    const repoId = this.#repo.repoId;
    if (repoId === undefined || this.busy.value || shas.length === 0) return;
    this.busy.value = true;
    try {
      const preflight = await this.#bridge.request('preflight.revert', { repoId, shas });
      await this.#runPreflighted({
        repoId,
        preflight,
        needsDialog: (p) => p.verdict !== 'clean' || p.mainlineRequired.length > 0,
        slot: this.#revertSlot,
        cancelText: 'git.op.cancelled.revert.announce',
        action: 'revert',
        buildOp: (mainline, noCommit) => ({ kind: 'revert', shas, mainline, noCommit }),
        announce: (result, noCommit) =>
          result.ok ? composeRevertAnnouncement(shas, noCommit) : undefined,
      });
    } catch (error) {
      this.#announceRejection('revert', error);
    } finally {
      this.busy.value = false;
    }
  }

  /** `RevertDialog.vue` calls this once a mainline is picked (`mainlineRequired.length > 0`) so
   *  the dialog's own prediction re-runs against the newly-known mainline before the user
   *  decides on `--no-commit` — the *same* pending promise stays open; only the displayed
   *  `pendingRevert` snapshot changes. */
  async previewRevertMainline(mainline: number): Promise<void> {
    const repoId = this.#repo.repoId;
    const shas = this.pendingRevert.value?.shas;
    if (repoId === undefined || shas === undefined) return;
    const preflight = await this.#bridge.request('preflight.revert', { repoId, shas, mainline });
    if (this.pendingRevert.value !== undefined) this.pendingRevert.value = preflight;
  }

  resolveRevertDialog(route: RevertRoute | null): void {
    this.#revertSlot.resolve(route);
  }

  // -------------------------------------------------------------------------------------
  // reset (`docs/plans/P10.md` W10, §7.7)
  // -------------------------------------------------------------------------------------

  /** `mode` is only the *initial* pre-flight's mode (OQ1's mixed default, chosen by
   *  `ResetDialog.vue`'s caller before this even runs) — the dialog itself may change it via
   *  `previewResetMode` with no second round trip (OQ11), and whatever it settles on travels back
   *  in `ResetRoute.mode`. The dialog opens for every reset, hazardous or not (judgment call 18):
   *  choosing a mode IS the confirm step here, unlike `runCheckout`/`runRevert`'s "skip the dialog
   *  when nothing is wrong" shape. A `blocked` verdict should never actually reach a user (the row
   *  menu's own `canRun("reset")` gate gets there first for an in-progress operation, and
   *  `unknownTarget` cannot happen for a sha taken from a real graph row) — handled defensively,
   *  with no dialog, rather than assumed unreachable. */
  async runReset(target: string, mode: ResetMode): Promise<void> {
    const repoId = this.#repo.repoId;
    if (repoId === undefined || this.busy.value) return;
    this.busy.value = true;
    let stashedFirst = false;
    try {
      const preflight = await this.#bridge.request('preflight.reset', { repoId, target, mode });
      if (preflight.verdict === 'blocked') {
        this.announcement.value = preflight.blockers.includes('inProgressOperation')
          ? `${opFailedTitle('reset')} ${t('git.op.reason.operationInProgress.error')}`
          : t('git.op.reset.unresolved.error', { target });
        return;
      }
      const route = await this.#resetSlot.ask(preflight);
      // G32 round-3 functional-correctness review, finding #7: same guard as runCheckout's own
      // #checkoutSlot.ask above — the active repo can change while this dialog is open.
      if (this.#repo.repoId !== repoId) return;
      if (route === null) {
        this.announcement.value = t('git.op.cancelled.reset.announce');
        return;
      }
      if (route.stashFirst) {
        const push = await this.#bridge.request('op.run', {
          repoId,
          op: {
            kind: 'stashPush',
            message: undefined,
            includeUntracked: false,
            keepIndex: false,
            paths: [],
          },
        });
        this.#applyResult(repoId, push);
        if (!push.ok) {
          this.#fail('stash', push.error, true);
          return;
        }
        stashedFirst = true;
      }
      const result = await this.#bridge.request('op.run', {
        repoId,
        op: {
          kind: 'reset',
          mode: route.mode,
          target,
          // Dropped when the stash-first route ran: the tree is clean by then, nothing is
          // destroyed, and the confirmation would be theatre (judgment call 19).
          confirmToken: route.stashFirst ? undefined : route.token,
        },
      });
      this.#applyResult(repoId, result);
      if (result.ok) this.announcement.value = composeResetAnnouncement(route.mode, target);
      else this.#fail('reset', result.error, true, stashedFirst);
    } catch (error) {
      this.#announceRejection('reset', error, stashedFirst);
    } finally {
      this.busy.value = false;
    }
  }

  /** `ResetDialog.vue`'s own mode-radio recompute: `destroys`/`requiresTypedConfirmation`/
   *  `routes`/`verdict` all depend only on `mode` and the ONE pre-flight's own `dirty` breakdown,
   *  already in hand — no round trip. Re-derived via `core`'s own `classifyReset`, never a hand-
   *  rolled duplicate, holding every other field fixed. `stagedNew: []` is not a loss of fidelity:
   *  a staged-but-new file is already counted in `dirty.staged` by construction
   *  (`repoService.ts`'s own `dirtySplitFrom`/`stagedNewPathsFrom` — the latter is a subset of the
   *  former), so the union `classifyReset` forms is identical either way. */
  previewResetMode(mode: ResetMode): void {
    const current = this.pendingReset.value;
    if (current === undefined) return;
    this.pendingReset.value = classifyReset({
      target: current.target,
      targetSubject: current.targetSubject,
      mode,
      currentHead: current.currentHead,
      branch: current.branch,
      leaving: current.leaving,
      gaining: current.gaining,
      leavingCommits: current.leavingCommits,
      leavingTruncated: current.leavingTruncated,
      dirty: current.dirty,
      stagedNew: [],
      inProgress: current.inProgress,
      targetResolves: !current.blockers.includes('unknownTarget'),
    });
  }

  /** `ResetDialog.vue`'s Reset/Stash first/Cancel buttons call this — `null` for Cancel, matching
   *  `resolveCheckoutDialog`/`resolveRevertDialog`'s own convention. */
  resolveResetDialog(route: ResetRoute | null): void {
    this.#resetSlot.resolve(route);
  }

  // -------------------------------------------------------------------------------------
  // cherry-pick (`docs/plans/P10.md` W10, §7.13)
  // -------------------------------------------------------------------------------------

  /** Mirrors `runRevert` exactly, including the "skip the dialog when there is nothing to show"
   *  shape — with one addition: the non-blocking `alreadyApplied` advisory also opens the
   *  dialog, since it is a fact the user should read before proceeding, not one
   *  `classifyCherryPick` folds into `verdict`/`blockers` (it never blocks anything, the
   *  "guessing is not honest" reasoning applied to the dialog's own trigger). */
  async runCherryPick(sha: string): Promise<void> {
    const repoId = this.#repo.repoId;
    if (repoId === undefined || this.busy.value) return;
    this.busy.value = true;
    try {
      const preflight = await this.#bridge.request('preflight.cherryPick', { repoId, sha });
      await this.#runPreflighted({
        repoId,
        preflight,
        needsDialog: (p) =>
          p.verdict !== 'clean' || p.mainlineRequired.length > 0 || p.alreadyApplied,
        slot: this.#cherryPickSlot,
        cancelText: 'git.op.cancelled.cherryPick.announce',
        action: 'cherryPick',
        buildOp: (mainline, noCommit) => ({ kind: 'cherryPick', sha, mainline, noCommit }),
        announce: (result, noCommit) => {
          const mismatch = this.#reconcileCherryPick(preflight.prediction, result);
          return mismatch
            ? composeCherryPickMismatchAnnouncement(mismatch)
            : result.ok
              ? composeCherryPickAnnouncement(sha, noCommit)
              : undefined;
        },
      });
    } catch (error) {
      this.#announceRejection('cherryPick', error);
    } finally {
      this.busy.value = false;
    }
  }

  /** `CherryPickDialog.vue` calls this once a mainline is picked, same shape as
   *  `previewRevertMainline` — the *same* pending promise stays open; only the displayed
   *  `pendingCherryPick` snapshot changes, re-predicted against the newly-known mainline before
   *  the user decides on `--no-commit`. */
  async previewCherryPickMainline(mainline: number): Promise<void> {
    const repoId = this.#repo.repoId;
    const sha = this.pendingCherryPick.value?.sha;
    if (repoId === undefined || sha === undefined) return;
    const preflight = await this.#bridge.request('preflight.cherryPick', {
      repoId,
      sha,
      mainline,
    });
    if (this.pendingCherryPick.value !== undefined) this.pendingCherryPick.value = preflight;
  }

  resolveCherryPickDialog(route: CherryPickRoute | null): void {
    this.#cherryPickSlot.resolve(route);
  }

  /** `CherryPickPreflight.prediction` is exact about the merge alone, reconciled
   *  after the fact exactly like `#reconcileStashPop` — a disagreement is announced, never
   *  swallowed. `unknown` predictions are never compared against, for the same reason
   *  `#reconcileStashPop` never does: there is nothing to disagree WITH. No `stashKept` half
   *  exists here (`CherryPickPredictionMismatch`'s own doc comment) — a cherry-pick never touches
   *  the stash. */
  #reconcileCherryPick(
    predicted: CherryPickPreflight['prediction'],
    result: OpResult,
  ): CherryPickPredictionMismatch | null {
    if (predicted.kind === 'unknown') return null;
    const actual: 'clean' | 'conflicts' | 'refused' = result.ok
      ? 'clean'
      : result.error?.kind === 'Conflict'
        ? 'conflicts'
        : 'refused';
    if (actual === predicted.kind) return null;
    return { predicted: predicted.kind, actual };
  }

  // -------------------------------------------------------------------------------------
  // stash (`docs/plans/P9.md` W13, §7.6)
  // -------------------------------------------------------------------------------------

  /** `StashDialog.vue`'s create mode IS the confirm step (no pre-flight endpoint exists for
   *  `stashPush` — there is nothing to classify before the fact), so this runs directly. The one
   *  thing it still cannot take on faith is git's own `No local changes to save` no-op: that spawn
   *  exits 0 with no error, so a stash-count comparison before/after is the only way to tell
   *  "stashed" from "there was nothing to stash" apart — reporting the latter as a
   *  plain success would be exactly the silent-no-op failure mode. */
  async runStashPush(input: {
    readonly message: string | undefined;
    readonly includeUntracked: boolean;
    readonly keepIndex: boolean;
    readonly paths: readonly string[];
  }): Promise<OpResult> {
    const repoId = this.#repo.repoId;
    if (repoId === undefined) return this.#failedResult(t('git.op.noRepository.error'));
    if (this.busy.value) {
      this.announcement.value = `${opFailedTitle('stash')} ${t('git.op.busy.error')}`;
      return this.#failedResult(t('git.op.busy.error'));
    }
    this.busy.value = true;
    try {
      const before = await this.#bridge.request('stash.list', { repoId });
      const result = await this.#bridge.request('op.run', {
        repoId,
        op: { kind: 'stashPush', ...input },
      });
      this.#applyResult(repoId, result);
      let pushed = true;
      if (result.ok) {
        const after = await this.#bridge.request('stash.list', { repoId });
        pushed = after.entries.length > before.entries.length;
      }
      if (result.ok) this.announcement.value = composeStashPushAnnouncement(pushed);
      else this.#fail('stash', result.error, true);
      return result;
    } catch (error) {
      this.#announceRejection('stash', error);
      return this.#failedResult(errorText(error));
    } finally {
      this.busy.value = false;
    }
  }

  /** `apply` — the stash survives regardless of outcome (it accepts a raw sha, so this
   *  needs no `stash@{index}` addressing or position re-verification at all). `restoreIndex` is a
   *  plain caller preference (`StashList.vue`'s own row-menu choice), never discovered by the
   *  pre-flight — the merge-tree prediction covers the worktree half only. */
  async runStashApply(entry: StashEntry, restoreIndex = false): Promise<OpResult | undefined> {
    return this.#runStashPopLike('apply', entry, restoreIndex);
  }

  /** `pop` — REFUSES a raw sha (probe 8), so the executed op addresses `stash@{index}`; the
   *  service re-verifies that position immediately before writing and the write removes the
   *  entry on success. */
  async runStashPop(entry: StashEntry, restoreIndex = false): Promise<OpResult | undefined> {
    return this.#runStashPopLike('pop', entry, restoreIndex);
  }

  async #runStashPopLike(
    verb: 'apply' | 'pop',
    entry: StashEntry,
    restoreIndex: boolean,
  ): Promise<OpResult | undefined> {
    const repoId = this.#repo.repoId;
    if (repoId === undefined || this.busy.value) return undefined;
    this.busy.value = true;
    try {
      // entry.scope threads through to the pre-flight so a global entry's own
      // merge-tree prediction resolves it from the bucket — `op.run`'s own 'stashApply'/'stashPop'
      // kinds need NO scope at all (`apply`/`pop` already address by sha/stack-position, the
      // "zero server plumbing" claim).
      const preflight = await this.#bridge.request('preflight.stashPop', {
        repoId,
        sha: entry.sha,
        index: entry.index,
        scope: entry.scope,
      });
      if (preflight.verdict !== 'clean') {
        const proceed = await this.#stashPopSlot.ask({ verb, preflight });
        // G32 round-3 functional-correctness review, finding #7: same guard as runCheckout's own
        // #checkoutSlot.ask above — the active repo can change while this dialog is open.
        if (this.#repo.repoId !== repoId) return undefined;
        if (!proceed) {
          this.announcement.value = t(
            verb === 'apply'
              ? 'git.op.cancelled.stashApply.announce'
              : 'git.op.cancelled.stashPop.announce',
          );
          return undefined;
        }
      }
      const op: OpRequest =
        verb === 'apply'
          ? { kind: 'stashApply', sha: entry.sha, restoreIndex }
          : { kind: 'stashPop', sha: entry.sha, index: entry.index, restoreIndex };
      const result = await this.#bridge.request('op.run', { repoId, op });
      this.#applyResult(repoId, result);
      const mismatch = this.#reconcileStashPop(verb, preflight.prediction, result);
      this.announcement.value = composeStashAnnouncement(
        verb,
        entry,
        result,
        mismatch,
        this.#refs.currentBranchName.value ?? null,
      );
      if (!result.ok)
        this.#noteFailure(verb === 'apply' ? 'stashApply' : 'stashPop', result.error, true);
      return result;
    } catch (error) {
      this.#announceRejection(verb === 'apply' ? 'stashApply' : 'stashPop', error);
      return undefined;
    } finally {
      this.busy.value = false;
    }
  }

  /** `StashDialog.vue`'s shared apply/pop confirmation calls this — `false` for Cancel, matching
   *  `resolveCheckoutDialog`/`resolveRevertDialog`'s own `null`-for-cancel convention as closely
   *  as a boolean route can (there is no route data to withhold on cancel here, per
   *  `PendingStashPop`'s own doc comment). */
  resolveStashPopDialog(proceed: boolean): void {
    this.#stashPopSlot.resolve(proceed);
  }

  /** Only ever non-null when reality disagreed with the prediction the user was shown. `unknown`
   *  predictions are never compared against — there is nothing to disagree WITH when pre-flight
   *  itself could not predict. `actual` is read from `result.ok`/`result.error.kind` — `runOp`'s
   *  own read-back already turns a real conflicting pop into `StashConflict` (never a thrown
   *  error), so no second git read is needed here. Every non-clean outcome keeps the stash;
   *  a genuinely clean `pop` is the one outcome that does not, `apply` never drops
   *  it either way. */
  #reconcileStashPop(
    verb: 'apply' | 'pop',
    predicted: StashPrediction,
    result: OpResult,
  ): StashPredictionMismatch | null {
    if (predicted.kind === 'unknown') return null;
    const actual: 'clean' | 'conflicts' | 'refused' = result.ok
      ? 'clean'
      : result.error?.kind === 'StashConflict'
        ? 'conflicts'
        : 'refused';
    if (actual === predicted.kind) return null;
    const stashKept = verb === 'apply' || actual !== 'clean';
    return { predicted: predicted.kind, actual, stashKept };
  }

  /** The undo slot has no stash-specific exception — the very next operation clears
   *  it exactly like any other undoable op — so the one honest thing to promise here is the bound
   *  itself, not a guarantee. No pre-flight endpoint exists for `stashDrop` (`git stash drop`
   *  cannot fail in a way worth predicting), so — like `tagDelete`/`branchDelete` — this runs
   *  directly with no confirm dialog; the announcement below stands in as the "confirmation". */
  async runStashDrop(entry: StashEntry): Promise<OpResult> {
    return this.#runSimple(
      { kind: 'stashDrop', sha: entry.sha, index: entry.index },
      (ok) =>
        ok
          ? t('git.op.dropStash.body', { ref: stashRef(entry.index), message: entry.message })
          : undefined,
      'dropStash',
    );
  }

  /** `StashDialog.vue`'s branch mode — the live preflight it re-renders as the user types a name,
   *  mirroring `previewRevertMainline`'s own read-only-round-trip shape. Not gated by `busy`: a
   *  read, like `previewPullStrategy`/`previewRevertMainline`, not an operation. */
  async previewStashBranch(
    entry: StashEntry,
    name: string,
  ): Promise<StashBranchPreflight | undefined> {
    const repoId = this.#repo.repoId;
    if (repoId === undefined) return undefined;
    return this.#bridge.request('preflight.stashBranch', {
      repoId,
      sha: entry.sha,
      branch: name,
      scope: entry.scope,
    });
  }

  /** `stash branch` gets no pop prediction (probe 11: clean by construction) but IS non-atomic on
   *  a mid-way failure (OQ6) — `StashDialog.vue`'s own branch mode has already run the classifier
   *  via `previewStashBranch` and is the confirm step, so this runs directly like `branchCreate`. */
  async runStashBranch(entry: StashEntry, name: string): Promise<OpResult> {
    return this.#runSimple(
      { kind: 'stashBranch', branch: name, sha: entry.sha, index: entry.index, scope: entry.scope },
      (ok) =>
        ok ? t('git.op.stashBranch.announce', { name, label: stashLabel(entry) }) : undefined,
      'stashBranch',
    );
  }

  // -------------------------------------------------------------------------------------
  // The global stash bucket. Both are `#runSimple` one-liners like every
  // branch/tag mutation above — `StashDialog.vue`'s own fourth mode has already collected the
  // label/source by the time `runGlobalStashSave` is called, and `GlobalStashList.vue`'s own row
  // menu is the confirm step for `runGlobalStashRemove`, the same "no separate pre-flight, the
  // dialog/menu IS the confirm step" shape `runStashDrop`/`runStashBranch` already follow.
  // -------------------------------------------------------------------------------------

  /** `sha: undefined` snapshots the CURRENT working tree; otherwise the sha of an existing
   *  stash-stack or global entry to promote (D10). Always COPIES — never drops the source. */
  async runGlobalStashSave(label: string, sha: string | undefined): Promise<OpResult> {
    return this.#runSimple(
      { kind: 'globalStashSave', label, sha },
      (ok) => (ok ? t('git.op.saveGlobalStash.announce', { label }) : undefined),
      'saveGlobalStash',
    );
  }

  /** D11: the cleanest undo in the whole table — the entry stays fully recoverable via the
   *  ordinary undo slot for one more operation, same as every other undoable action here. */
  async runGlobalStashRemove(entry: StashEntry): Promise<OpResult> {
    return this.#runSimple(
      { kind: 'globalStashRemove', sha: entry.sha },
      (ok) => (ok ? t('git.op.removeGlobalStash.body', { label: stashLabel(entry) }) : undefined),
      'removeGlobalStash',
    );
  }

  // -------------------------------------------------------------------------------------
  // Worktree support. runWorktreeAdd/runWorktreeRemove are `#runSimple` one-liners like
  // every branch/tag mutation above — `WorktreeDialog.vue`'s own create phase and the Remove
  // confirmation have already collected/confirmed their input by the time either is called.
  // -------------------------------------------------------------------------------------

  async runWorktreeAdd(params: {
    readonly path: string;
    readonly mode: 'existingBranch' | 'newBranch' | 'detach';
    readonly branch: string | undefined;
    readonly startPoint: string | undefined;
  }): Promise<OpResult> {
    return this.#runSimple(
      { kind: 'worktreeAdd', ...params },
      (ok) => (ok ? t('git.op.createWorktree.announce', { path: params.path }) : undefined),
      'createWorktree',
    );
  }

  /** `confirmToken` is present only for the dirty route — WorktreeDialog's own Remove
   *  confirmation reads it straight off a fresh `preflight.worktreeRemove` call, never typed blind.
   *  `force` is derived here from whether a token was supplied at all: the server re-derives it
   *  fresh from its own preflight regardless (the fail-safe-over-fail-open principle), so a
   *  mismatched or stale claim here can only ever be REFUSED, never accepted. */
  async runWorktreeRemove(path: string, confirmToken?: string): Promise<OpResult> {
    return this.#runSimple(
      { kind: 'worktreeRemove', path, force: confirmToken !== undefined, confirmToken },
      (ok) => (ok ? t('git.op.removeWorktree.announce', { path }) : undefined),
      'removeWorktree',
    );
  }

  // -------------------------------------------------------------------------------------
  // stackSet is an ordinary #runSimple one-liner (StackDialog's own set-parent mode
  // has already collected the target branch/parent by the time this is called). runRestack/
  // cancelRestack delegate to StackState (see #stack's own doc comment above) and layer this
  // class's own busy/announcement lifecycle on top — StackState holds the reactive progress.
  // -------------------------------------------------------------------------------------

  async runStackSet(branch: string, parent: string | undefined): Promise<OpResult> {
    return this.#runSimple(
      { kind: 'stackSet', branch, parent },
      (ok) =>
        ok
          ? parent === undefined
            ? t('git.op.removeFromStack.announce', { branch })
            : t('git.op.setStackParent.announce', { branch, parent })
          : undefined,
      'setStackParent',
    );
  }

  /** `stack.restack`'s own client-facing entry point — NOT an `op.run` kind, so this bypasses
   *  `#runSimple` entirely and delegates the actual execution to
   *  `StackState.runRestack`, which already holds `restacking`/`progress` reactively. `busy` is
   *  set for the same duration `#runSimple` would set it, so every other button this app disables
   *  while `busy` is true (checkout, reset, …) is disabled during a restack too — a restack
   *  rewrites the very branches those buttons would act on. */
  async runRestack(branch: string): Promise<RestackResult | undefined> {
    if (this.#stack === undefined) return undefined;
    if (this.busy.value) {
      this.announcement.value = `${opFailedTitle('restack')} ${t('git.op.busy.error')}`;
      return undefined;
    }
    // F3: captured for the same reason `#applyResult`'s own doc comment gives every other
    // caller's identity guard — `#stack.runRestack` awaits real git work (one rebase per
    // branch), long enough for the active repo to have changed underneath it.
    const repoId = this.#repo.repoId;
    this.busy.value = true;
    try {
      const result = await this.#stack.runRestack(branch);
      if (result !== undefined && repoId !== undefined) {
        // Route through the same reconcile every other mutating op uses — this used to
        // apply only `head` via `#refs.applyHead`, so a restack's own undo record never
        // appeared (the Undo button kept showing the previous op's, now-superseded, label) and
        // a restack paused on conflict left `statusSummary.inProgress` null (wrong `canRun`
        // conflict gating) until the next `repo.changed` round trip. `#applyResult` itself
        // re-checks repo identity, so this is safe even if the repo changed mid-restack.
        this.#applyResult(repoId, {
          ok: result.ok,
          error: result.error,
          undo: result.undo ?? null,
          head: result.head,
          inProgress: result.inProgress,
        });
        this.announcement.value = result.ok
          ? composeRestackAnnouncement(result.restacked, undefined, [])
          : composeRestackAnnouncement(result.restacked, result.stoppedAt, result.remaining);
        if (!result.ok && result.error) this.#noteFailure('restack', result.error, true);
      }
      return result;
    } catch (error) {
      this.#announceRejection('restack', error);
      return undefined;
    } finally {
      this.busy.value = false;
    }
  }

  /** `stack.cancelRestack` — always safe to call, mirroring `cancelRemote`'s/
   *  `cancelRemote`'s own doc comment: `false` (never an error) when there was
   *  nothing to cancel. */
  async cancelRestack(): Promise<boolean> {
    return (await this.#stack?.cancelRestack()) ?? false;
  }

  /**
   * The `stashAndCarry` route — shared
   * between `runCheckout`'s and `runPull`'s own inline calls below (each confirms via its own
   * dialog first, so there is no separate public `run*StashAndCarry` entry point), which differ
   * only in `runMiddle` (the blocked operation itself) and its own success wording.
   *
   * **Push, then run, then pop only if predicted clean.** The middle op ALWAYS runs once
   * the stash push succeeds — carrying the user's changes across is the entire point of the
   * route, and a predicted-conflict pop is a reason to leave the stash for the user to resolve
   * by hand, never a reason to also withhold the checkout/pull they explicitly asked for. A
   * predicted conflict (or a blocker) therefore ends the route with the stash intentionally
   * still in the stack, not an error — the middle op's own success is what gets announced.
   *
   * **`targetSha` is omitted deliberately.** `preflight.stashPop`'s own contract documents this
   * field as the `stashAndCarry` route's way to predict against a target the walk has not
   * switched to yet — accurate for `checkout`, whose target is already a known revision before
   * the middle op runs. `pull` has no such revision to offer: its target is whatever the fetch
   * half of a single atomic `remote.run` resolves to, not something this class learns ahead of
   * running it. Predicting AFTER `runMiddle` (the default `targetSha` ⇒ current HEAD, which by
   * then already reflects whatever the middle op did) is correct for both — even for `checkout`,
   * it is the ACTUAL resulting tree rather than a hypothetical one — so both routes share this one
   * predict-after shape rather than checkout alone taking the more elaborate predict-before path
   * the field's doc comment names. This is a deliberate, documented deviation from that comment's
   * literal reading.
   */
  /** `repoId` is the CALLER's own captured repo id (`runCheckout`/`runPull`), a required
   *  parameter rather than a fresh `this.#repo.repoId` read (G32 round-3 functional-correctness
   *  review, finding #7): reading it fresh here used to let this whole sequence — stash push,
   *  the caller's own `runMiddle` (checkout/pull), stash pop — silently disagree with the
   *  identity `runMiddle`'s own closure was built against, if the active repo changed between
   *  the caller's confirm dialog opening and this method actually running (e.g. "Open in graph"
   *  switching repos mid-dialog). One shared identity for the whole sequence, checked again after
   *  each await below — via `#applyResult`'s own guard for the push/pop halves, and explicitly
   *  for the `runMiddle` half, which writes `#refs`/`statusSummary` directly. */
  async #stashAndCarry(
    repoId: string,
    runMiddle: () => Promise<CarryMiddleResult>,
    action: OpAction,
    announceMiddleOk: () => string,
  ): Promise<void> {
    // The stash-list snapshot taken AFTER stashPush is not, by itself, proof that stashPush created
    // anything — an
    // untracked-only dirty tree makes `git stash push` (no `-u`) print "No local changes to
    // save" and exit 0 (`pushResult.ok` true) without touching the stack at all. The OLD check
    // below ("is the post-push list empty?") only caught that when the user had NO stash of
    // their own; with a pre-existing stash, `entries[0]` after such a no-op push is that
    // pre-existing, UNRELATED entry, and the code used to run stashPop on it — applying and then
    // deleting a stash this operation never created. Same before/after entry-count comparison
    // runStashPush already uses (above) for the identical probe-10 no-op.
    const beforePush = await this.#bridge.request('stash.list', { repoId });

    const pushResult = await this.#bridge.request('op.run', {
      repoId,
      op: {
        kind: 'stashPush',
        message: undefined,
        includeUntracked: false,
        keepIndex: false,
        paths: [],
      },
    });
    this.#applyResult(repoId, pushResult);
    if (!pushResult.ok) {
      this.#fail('stash', pushResult.error, true);
      return;
    }

    let middle: CarryMiddleResult;
    try {
      middle = await runMiddle();
    } catch (error) {
      this.#announceRejection(action, error, true);
      return;
    }
    // Same identity guard as #applyResult's own (this write isn't routed through it, since
    // CarryMiddleResult isn't an OpResult) — a repo switch during runMiddle must not land its
    // head/inProgress on whatever repo is displayed now.
    if (this.#repo.repoId === repoId) {
      this.#refs.applyHead(middle.head);
      const current = this.statusSummary.value;
      if (current)
        this.statusSummary.value = { ...current, head: middle.head, inProgress: middle.inProgress };
    }
    if (!middle.ok) {
      this.#fail(action, middle.error, true, true);
      return;
    }

    const { entries } = await this.#bridge.request('stash.list', { repoId });
    const top = entries[0];
    if (top === undefined || entries.length <= beforePush.entries.length) {
      // `stashPush` above was itself a no-op — either the worktree was clean
      // (top === undefined), or it was dirty only in a way `stashPush`'s own args don't capture
      // (untracked-only, no -u — the list is unchanged, and `top` is the caller's own
      // pre-existing, unrelated entry). Either way nothing THIS operation stashed, so nothing to
      // carry back or pop.
      this.announcement.value = announceMiddleOk();
      return;
    }

    const preflight = await this.#bridge.request('preflight.stashPop', {
      repoId,
      sha: top.sha,
      index: top.index,
    });
    if (preflight.verdict !== 'clean') {
      this.announcement.value = `${announceMiddleOk()}. ${t('git.op.stashKept.body', { ref: stashRef(top.index) })}`;
      return;
    }

    const popResult = await this.#bridge.request('op.run', {
      repoId,
      op: { kind: 'stashPop', sha: top.sha, index: top.index, restoreIndex: false },
    });
    this.#applyResult(repoId, popResult);
    const mismatch = this.#reconcileStashPop('pop', preflight.prediction, popResult);
    if (mismatch) {
      this.announcement.value = composeStashAnnouncement('pop', top, popResult, mismatch);
      if (!popResult.ok) this.#noteFailure('stashPop', popResult.error, true);
    } else if (popResult.ok) this.announcement.value = announceMiddleOk();
    else this.#fail('stashPop', popResult.error, true);
  }

  // -------------------------------------------------------------------------------------
  // Branch/tag mutations and the in-progress op controls: none of these have a pre-flight
  // endpoint of their own (only checkout and revert do) — the dialog or menu confirmation that
  // collects their input has already run by the time one of these is called, so there is no
  // second "confirm" step here, only run → refresh/announce/surface.
  // -------------------------------------------------------------------------------------

  async branchCreate(req: {
    readonly name: string;
    readonly startPoint: string;
    readonly checkout: boolean;
    readonly track: string | undefined;
  }): Promise<OpResult> {
    return this.#runSimple(
      { kind: 'branchCreate', ...req },
      (ok) => (ok ? t('git.op.createBranch.announce', { name: req.name }) : undefined),
      'createBranch',
    );
  }

  async branchDelete(name: string, force: boolean): Promise<OpResult> {
    return this.#runSimple(
      { kind: 'branchDelete', name, force },
      (ok) => (ok ? t('git.op.deleteBranch.announce', { name }) : undefined),
      'deleteBranch',
    );
  }

  async branchRename(from: string, to: string): Promise<OpResult> {
    return this.#runSimple(
      { kind: 'branchRename', from, to },
      (ok) => (ok ? t('git.op.renameBranch.announce', { from, to }) : undefined),
      'renameBranch',
    );
  }

  async tagCreate(req: {
    readonly name: string;
    readonly target: string;
    readonly message: string | undefined;
    readonly force: boolean;
  }): Promise<OpResult> {
    return this.#runSimple(
      { kind: 'tagCreate', ...req },
      (ok) => (ok ? t('git.op.createTag.announce', { name: req.name }) : undefined),
      'createTag',
    );
  }

  async tagDelete(name: string): Promise<OpResult> {
    return this.#runSimple(
      { kind: 'tagDelete', name },
      (ok) => (ok ? t('git.op.deleteTag.announce', { name }) : undefined),
      'deleteTag',
    );
  }

  async tagPush(remote: string, names: readonly string[] | 'all'): Promise<OpResult> {
    return this.#runSimple(
      { kind: 'tagPush', remote, names },
      (ok) =>
        ok
          ? names === 'all'
            ? t('git.op.pushAllTags.announce', { remote })
            : t('git.op.pushTag.announce', { names: names.join(', '), remote })
          : undefined,
      'pushTag',
    );
  }

  async tagDeleteRemote(remote: string, name: string): Promise<OpResult> {
    return this.#runSimple(
      { kind: 'tagDeleteRemote', remote, name },
      (ok) => (ok ? t('git.op.deleteRemoteTag.announce', { name, remote }) : undefined),
      'deleteRemoteTag',
    );
  }

  async continueOp(): Promise<OpResult> {
    return this.#runSimple(
      { kind: 'opContinue' },
      (ok) => (ok ? t('git.op.continued.announce') : undefined),
      'continue',
    );
  }

  async abortOp(): Promise<OpResult> {
    return this.#runSimple(
      { kind: 'opAbort' },
      (ok) => (ok ? t('common.aborted.status') : undefined),
      'abort',
    );
  }

  /** The third sequencer verb — `ConflictBanner.vue`'s Skip button, rendered only
   *  when `inProgress.canSkip`. Joins `continueOp`/`abortOp` as a `#runSimple` one-liner: like
   *  them, the confirmation already happened (the banner itself), so there is no separate confirm
   *  step here either. */
  async skipOp(): Promise<OpResult> {
    return this.#runSimple(
      { kind: 'opSkip' },
      (ok) => (ok ? t('common.skipped.status') : undefined),
      'skip',
    );
  }

  async undo(): Promise<OpResult | undefined> {
    const repoId = this.#repo.repoId;
    const slot = this.undoSlot.value;
    if (repoId === undefined || slot === null || this.busy.value) return undefined;
    this.busy.value = true;
    try {
      const result = await this.#bridge.request('undo.run', { repoId, id: slot.id });
      this.#applyResult(repoId, result);
      if (result.ok) this.announcement.value = composeUndoAnnouncement(slot.label);
      else this.#fail('undo', result.error, true);
      return result;
    } catch (error) {
      this.#announceRejection('undo', error);
      return undefined;
    } finally {
      this.busy.value = false;
    }
  }

  // -------------------------------------------------------------------------------------
  // remote ops (P8 W17)
  // -------------------------------------------------------------------------------------

  /** `PullStrategyPicker.vue`'s own read: what the ladder would resolve to *right now*,
   *  with no override — a read, not an operation (mirrors `openReview`'s own doc comment above),
   *  so the popover can show "follow your configuration (would run: rebase)" before the user
   *  commits to an override. */
  async previewPullStrategy(branch: string): Promise<PullPreflight | undefined> {
    const repoId = this.#repo.repoId;
    if (repoId === undefined) return undefined;
    return this.#bridge.request('remote.pullPreflight', { repoId, branch });
  }

  /** §7.1: `--prune` on, `--prune-tags` off, both by default (D49 — an unpushed tag is user
   *  work and fetch offers no undo). Neither is exposed as a toolbar option at P8; a future
   *  phase can surface them without changing this method's shape. */
  async runFetch(remote: string): Promise<void> {
    await this.#runRemote(
      {
        kind: 'fetch',
        remote,
        branch: undefined,
        setUpstream: false,
        prune: true,
        pruneTags: false,
        strategy: undefined,
        rebaseMerges: undefined,
        expectedRemoteTip: undefined,
        plainForce: undefined,
        confirmToken: undefined,
      },
      (result) => (result.ok ? t('git.op.fetched.announce', { remote }) : undefined),
      'fetch',
    );
  }

  /**
   * The resolved strategy and its provenance are shown before the operation runs, not
   * after — `remote.pullPreflight` always runs first (its `blockers`/`routes` are the only
   * way to learn about a dirty tree that would rewrite history, so this can no longer be
   * short-circuited the way a bare strategy resolution once was) and `pullStrategy` is set from
   * it before `remote.run` is ever called, so what the toolbar displays is exactly what is about
   * to run, not a guess. `explicitStrategy` overrides the *strategy* the preflight resolved (the
   * user's own choice is authoritative, ladder step 1, `source: "explicit"` by construction) but
   * not the dirty-tree check: `blockers` is evaluated against the ladder's own default-resolved
   * strategy regardless, since `remote.pullPreflight` has no way to ask "would this be blocked
   * under strategy X instead" — a known imprecision that
   * only matters for the narrow case of an explicit override changing whether history would be
   * rewritten at all.
   *
   * The `stashAndCarry` route: a `dirtyNonFastForward` blocker opens
   * `pendingPull` (`PullDialog.vue`'s own confirm step, mirroring `pendingCheckout`) — cancelling
   * ends the pull with nothing run; confirming pushes a stash, runs the pull, and pops it back
   * exactly like `runCheckout`'s own inline call above (`#stashAndCarry`'s shared shape).
   *
   * `rebaseMerges` travels with the preflight-resolved strategy (`preflight.rebaseMerges`,
   * the server's own once-computed decision); an explicit override always sends `false` — never
   * derived here from `strategy === 'rebase'` or from `source`.
   */
  async runPull(remote: string, branch: string, explicitStrategy?: PullStrategy): Promise<void> {
    try {
      await this.#runPull(remote, branch, explicitStrategy);
    } catch (error) {
      this.#announceRejection('pull', error);
    }
  }

  async #runPull(remote: string, branch: string, explicitStrategy?: PullStrategy): Promise<void> {
    const repoId = this.#repo.repoId;
    if (repoId === undefined || this.busy.value) return;
    const preflight: PullPreflight = await this.#bridge.request('remote.pullPreflight', {
      repoId,
      branch,
    });
    if (this.#repo.repoId !== repoId) return;
    const strategy = explicitStrategy ?? preflight.strategy;
    const source: PullStrategySource =
      explicitStrategy !== undefined ? 'explicit' : preflight.source;
    this.pullStrategy.value = { strategy, source };
    // An explicit pick is a plain strategy choice; only the config ladder carries git's own "merges".
    const rebaseMerges = explicitStrategy === undefined ? preflight.rebaseMerges : false;

    if (preflight.blockers.length > 0) {
      const proceed = await this.#confirmPull(preflight);
      // G32 round-3 functional-correctness review, finding #7: same guard as runCheckout's own
      // #checkoutSlot.ask above — the active repo can change while this dialog is open.
      if (this.#repo.repoId !== repoId) return;
      if (!proceed) {
        this.announcement.value = t('git.op.cancelled.pull.announce');
        return;
      }
      if (this.busy.value) return; // another op started while the dialog was open
      this.busy.value = true;
      try {
        await this.#stashAndCarry(
          repoId,
          () =>
            this.#bridge.request('remote.run', {
              repoId,
              kind: 'pull',
              remote,
              branch,
              setUpstream: false,
              prune: false,
              pruneTags: false,
              strategy,
              rebaseMerges,
              expectedRemoteTip: undefined,
              plainForce: undefined,
              confirmToken: undefined,
            }),
          'pull',
          () => t('git.op.pulled.announce', { remote, branch, strategy }),
        );
      } finally {
        this.busy.value = false;
      }
      return;
    }

    await this.#runRemote(
      {
        kind: 'pull',
        remote,
        branch,
        setUpstream: false,
        prune: false,
        pruneTags: false,
        strategy,
        rebaseMerges,
        expectedRemoteTip: undefined,
        plainForce: undefined,
        confirmToken: undefined,
      },
      (result) =>
        result.ok ? t('git.op.pulled.announce', { remote, branch, strategy }) : undefined,
      'pull',
    );
  }

  #confirmPull(preflight: PullPreflight): Promise<boolean> {
    this.pendingPull.value = preflight;
    return new Promise((resolve) => {
      this.#resolvePull = resolve;
    });
  }

  /** `PullDialog.vue`'s own Stash-and-pull/Cancel buttons call this — `false` for Cancel. */
  resolvePullDialog(proceed: boolean): void {
    this.pendingPull.value = undefined;
    const resolve = this.#resolvePull;
    this.#resolvePull = undefined;
    resolve?.(proceed);
  }

  /** Plain push is never gated (D52) — no confirm step here, only the upstream question a
   *  preflight already answers: §7.2's "offered, not silent" for `--set-upstream`. */
  async runPush(remote: string, branch: string): Promise<void> {
    try {
      await this.#runPush(remote, branch);
    } catch (error) {
      this.#announceRejection('push', error);
    }
  }

  async #runPush(remote: string, branch: string): Promise<void> {
    const repoId = this.#repo.repoId;
    if (repoId === undefined || this.busy.value) return;
    const preflight: PushPreflight = await this.#bridge.request('remote.pushPreflight', {
      repoId,
      branch,
      remote,
    });
    if (this.#repo.repoId !== repoId) return;
    await this.#runRemote(
      {
        kind: 'push',
        remote,
        branch,
        setUpstream: preflight.wouldSetUpstream,
        prune: false,
        pruneTags: false,
        strategy: undefined,
        rebaseMerges: undefined,
        expectedRemoteTip: undefined,
        plainForce: undefined,
        confirmToken: undefined,
      },
      (result) => (result.ok ? t('git.op.pushed.announce', { branch, remote }) : undefined),
      'push',
    );
  }

  /**
   * `ForcePushDialog.vue`'s own entry point: a preflight round trip (so the dialog can show the
   * remote tip it is about to overwrite and the matched protected pattern, if any), then the
   * dialog's own confirm/cancel promise, mirroring `#checkoutSlot.ask`/`#revertSlot.ask` above.
   * `expectedRemoteTip` is the preflight's `remoteTip` — re-read and compared host-side
   * immediately before spawning, so a change between here and
   * the actual spawn fails with `LeaseViolation` rather than silently overwriting more than the
   * dialog showed.
   */
  async runForcePush(remote: string, branch: string): Promise<void> {
    try {
      await this.#runForcePush(remote, branch);
    } catch (error) {
      this.#announceRejection('forcePush', error);
    }
  }

  async #runForcePush(remote: string, branch: string): Promise<void> {
    const repoId = this.#repo.repoId;
    if (repoId === undefined || this.busy.value) return;
    const preflight: PushPreflight = await this.#bridge.request('remote.pushPreflight', {
      repoId,
      branch,
      remote,
    });
    if (this.#repo.repoId !== repoId) return;
    const route = await this.#forcePushSlot.ask({ remote, branch, preflight });
    // G32 round-3 functional-correctness review, finding #7: same guard as runCheckout's own
    // #checkoutSlot.ask above — the active repo can change while this dialog is open.
    if (this.#repo.repoId !== repoId) return;
    if (route === null) {
      this.announcement.value = t('git.op.cancelled.forcePush.announce');
      return;
    }
    await this.#runRemote(
      {
        kind: 'forcePush',
        remote,
        branch,
        setUpstream: false,
        prune: false,
        pruneTags: false,
        strategy: undefined,
        rebaseMerges: undefined,
        expectedRemoteTip: preflight.remoteTip,
        plainForce: route.plain,
        confirmToken: route.confirmToken,
      },
      (result) => (result.ok ? t('git.op.forcePushed.announce', { branch, remote }) : undefined),
      'forcePush',
    );
  }

  /** `ForcePushDialog.vue`'s Cancel/confirm buttons call this — `null` for Cancel, matching
   *  `resolveCheckoutDialog`/`resolveRevertDialog`'s own convention. */
  resolveForcePushDialog(route: ForcePushRoute | null): void {
    this.#forcePushSlot.resolve(route);
  }

  /** `remote.cancel` — always safe to call: `false` (never an error) when there was nothing to
   *  cancel, including when the toolbar's cancel button is clicked against a push or a pull
   *  already past its fetch phase (D50; W19's "cancel is refused mid-push"). Does not throw and
   *  does not touch `busy`/`activeRemoteOp` itself — the in-flight `#runRemote` call unwinds
   *  those the moment `remote.run` actually settles, whether that is `Cancelled` or, for an
   *  unkillable op, its ordinary result. */
  async cancelRemote(): Promise<boolean> {
    const repoId = this.#repo.repoId;
    if (repoId === undefined) return false;
    const { cancelled } = await this.#bridge.request('remote.cancel', { repoId });
    return cancelled;
  }

  async #runRemote(
    op: Omit<RemoteOpParams, 'repoId'>,
    announceOk: (result: RemoteOpResult) => string | undefined,
    action: OpAction,
  ): Promise<RemoteOpResult | undefined> {
    const repoId = this.#repo.repoId;
    if (repoId === undefined || this.busy.value) return undefined;
    this.busy.value = true;
    this.activeRemoteOp.value = op.kind;
    this.remoteProgress.value = undefined;
    try {
      const result = await this.#bridge.request('remote.run', { repoId, ...op });
      if (this.#repo.repoId !== repoId) return result;
      this.#applyRemoteResult(result);
      if (result.ok) this.announcement.value = announceOk(result) ?? t('common.done.status');
      else this.#fail(action, result.error, true);
      return result;
    } catch (error) {
      this.#announceRejection(action, error);
      return undefined;
    } finally {
      this.busy.value = false;
      this.activeRemoteOp.value = undefined;
      this.remoteProgress.value = undefined;
    }
  }

  /** The remote-op half of `#applyResult`: reconciles `head`/`inProgress` exactly the same way
   *  (so a conflicting pull lands in the existing in-progress banner, unchanged), but — unlike
   *  `#applyResult` — never touches `undoSlot` in either direction. `RemoteOpResult` carries no
   *  `undo` field at all, by construction; this method's whole reason to exist separately
   *  is that the answer must be structurally impossible to get wrong, not merely remembered. */
  #applyRemoteResult(result: RemoteOpResult): void {
    if (result.ok) this.lastFailure.value = undefined;
    this.#refs.applyHead(result.head);
    const current = this.statusSummary.value;
    this.statusSummary.value = current
      ? { ...current, head: result.head, inProgress: result.inProgress }
      : {
          head: result.head,
          upstream: undefined,
          counts: { staged: 0, unstaged: 0, untracked: 0, unmerged: 0 },
          isClean: true,
          dirtyPaths: [],
          dirtyTruncated: false,
          inProgress: result.inProgress,
          autoFetch: null,
        };
  }

  /** A rejected request (reconnect race, socket drop, `E_GIT_UNAVAILABLE`) never reaches an
   *  `OpResult`, so no failure path above announces it, and every caller treats these methods as
   *  fire-and-forget. Announce instead of rethrowing; a local cancel stays silent. */
  #announceRejection(action: OpAction, error: unknown, stashed = false): void {
    if (error instanceof TransportError && error.code === 'cancelled') return;
    const message = errorText(error);
    this.#fail(action, { kind: 'Unknown', message }, false, stashed);
  }

  /** Every failure the user should see: the live-region text (unchanged wording) plus the visible
   *  banner notice. `logged` says whether the Operations log recorded it. A user cancel announces
   *  but never raises the banner. */
  #fail(
    action: OpAction,
    error: OpFailureError | undefined,
    logged: boolean,
    stashed = false,
  ): void {
    const note = stashed ? ` ${t('git.op.stashedNote.body')}` : '';
    this.announcement.value = `${composeOpFailureAnnouncement(action, error)}${note}`;
    this.#noteFailure(action, error, logged);
  }

  /** The banner half of `#fail`, for outcomes whose announcement text is composed elsewhere. */
  #noteFailure(action: OpAction, error: OpFailureError | undefined, logged: boolean): void {
    if (error?.kind === 'Cancelled') return;
    this.lastFailure.value = composeFailureNotice(action, error, logged);
  }

  /** The `OpResult` a caller that reads `.ok` sees when no request completed. Never applied to
   *  state (`#applyResult` is not called with it); `head`/`inProgress` echo what is displayed. */
  #failedResult(message: string): OpResult {
    return {
      ok: false,
      error: { kind: 'Unknown', message },
      undo: this.undoSlot.value,
      head: this.#refs.head.value ?? { kind: 'unborn', name: '' },
      inProgress: this.statusSummary.value?.inProgress ?? null,
    };
  }

  async #runSimple(
    op: OpRequest,
    announceOk: (ok: true) => string | undefined,
    action: OpAction,
  ): Promise<OpResult> {
    const repoId = this.#repo.repoId;
    if (repoId === undefined) return this.#failedResult(t('git.op.noRepository.error'));
    if (this.busy.value) {
      this.announcement.value = `${opFailedTitle(action)} ${t('git.op.busy.error')}`;
      return this.#failedResult(t('git.op.busy.error'));
    }
    this.busy.value = true;
    try {
      const result = await this.#bridge.request('op.run', { repoId, op });
      this.#applyResult(repoId, result);
      if (result.ok) this.announcement.value = announceOk(true) ?? t('common.done.status');
      else this.#fail(action, result.error, true);
      return result;
    } catch (error) {
      this.#announceRejection(action, error);
      return this.#failedResult(errorText(error));
    } finally {
      this.busy.value = false;
    }
  }

  /** Step 4's synchronous half: applies `head`/`inProgress`/`undo`
   *  before the matching `repo.changed` event — which always follows a real op — has a chance to
   *  arrive and trigger `RefsState.reload()`/`refreshStatus()`'s own, fuller reconcile.
   *
   *  `repoId` is the CALLING op's own captured repo id, not read fresh here: every caller already
   *  awaited at least one request or dialog before reaching this point, during which the active
   *  repo can have changed (`App.vue`'s "Open in graph" calls `repo.open` directly, independent of
   *  any op's own `busy` hold). Applying a stale op's own head/inProgress/undo into
   *  `RefsState`/`statusSummary` after such a switch would silently corrupt the NOW-displayed
   *  repo's state with another repo's own result — the same identity check
   *  `refreshStatus`/`refreshUndo`/`#runRemote` already apply to their own writes. */
  #applyResult(repoId: string, result: OpResult): void {
    if (this.#repo.repoId !== repoId) return;
    if (result.ok) this.lastFailure.value = undefined;
    this.#refs.applyHead(result.head);
    const current = this.statusSummary.value;
    this.statusSummary.value = current
      ? { ...current, head: result.head, inProgress: result.inProgress }
      : {
          head: result.head,
          upstream: undefined,
          counts: { staged: 0, unstaged: 0, untracked: 0, unmerged: 0 },
          isClean: true,
          dirtyPaths: [],
          dirtyTruncated: false,
          inProgress: result.inProgress,
          autoFetch: null,
        };
    this.undoSlot.value = result.undo;
  }

  dispose(): void {
    this.#repo.dispose();
    this.#unsubscribeProgress();
    this.#unsubscribeAutoFetch();
    this.#statusRequest.abort();
    this.#undoRequest.abort();
    this.#abandonPending();
  }
}
