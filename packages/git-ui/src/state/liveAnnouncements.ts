/**
 * The "one polite live region" — pure text composition for the two events it announces
 * (Load-more's own doc comment in `LoadMoreButton.vue` names both: "the Load-more result
 * ... and Refresh completion"). Kept separate from `App.vue`, which only owns *when* to set the
 * region's text (watching `GraphViewState.loading`'s transitions back to `"idle"`), so the exact
 * wording is unit-testable on its own, the same split `rowAccessibility.ts` makes for a row's
 * accessible name.
 *
 * Every announcement `ops.ts` produces — success or failure — is composed here too, per the rule
 * that a destructive action which silently does nothing is the same failure mode already named
 * for the clipboard.
 */
import { shortSha } from '@kira/git-core';
import type {
  CheckoutPreflight,
  OpErrorKind,
  OpResult,
  ResetMode,
  StashEntry,
} from '@kira/git-ipc';
import { joinSentences, type MessageKey, t } from '@theme/i18n';
import { formatCount } from '@workbench/behaviour/format';
import { customRef, type Ref } from 'vue';
import { originLabel, stashLabel } from '../lib/stashListModel.ts';

/** A string ref that notifies on every assignment, equal text included. Watch it with
 *  `{ deep: true }` (which forces the callback): a repeated identical failure must still reach the
 *  live region. */
export function createAnnouncementRef(): Ref<string> {
  let text = '';
  return customRef<string>((track, trigger) => ({
    get() {
      track();
      return text;
    },
    set(next) {
      text = next;
      trigger();
    },
  }));
}

/** The plan's own worked example, generalized: "N more loaded, M remaining" — or, once the
 *  history is fully loaded, "N more loaded, history fully loaded" rather than "0 remaining",
 *  which reads as if nothing happened. */
export function composeLoadMoreAnnouncement(
  added: number,
  remaining: number,
  exhausted: boolean,
): string {
  return exhausted
    ? t('git.announce.loadMoreDone.announce', { added: formatCount(added) })
    : t('git.announce.loadMore.announce', {
        added: formatCount(added),
        remaining: formatCount(remaining),
      });
}

/** §6.2's refresh action, completed: a keyboard user who cannot see the toolbar spinner stop has
 *  no other way to learn a refresh finished (or how many commits it re-walked). */
export function composeRefreshAnnouncement(totalLoaded: number): string {
  return t('git.announce.refreshed.announce', { count: totalLoaded });
}

/** `GraphViewState.revealSha`'s own live-region text — "selecting such a result loads the pages up
 *  to it" can be several seconds of paging on a large repository, which must not read as a hang to
 *  a user who cannot see the toolbar's own loading indicator. There is no `"found"` text: the row
 *  selection that follows a successful reveal already moves real focus onto it, and that row's own
 *  composed accessible name (`rowAccessibility.ts`) is the completion signal — a second
 *  announcement here would only repeat it. A hit that is *not* found (the history paged to
 *  exhaustion without it — most likely a stale hit from before a force-push) gets nothing else at
 *  all otherwise. */
export function composeRevealSearchHitAnnouncement(outcome: 'loading' | 'notFound'): string {
  return outcome === 'loading'
    ? t('git.announce.revealLoading.announce')
    : t('git.announce.revealNotFound.announce');
}

/** A short name for `target`, the way every P6 confirmation reads it: a raw sha is shortened,
 *  anything else (a branch, tag, or remote-branch name) is shown exactly as given. */
function shortTarget(target: string): string {
  return /^[0-9a-f]{20,40}$/i.test(target) ? shortSha(target) : target;
}

/** §7.5's own two silent-success verdicts, given a voice: `cleanCarry` is "proceed with no
 *  prompt", not "proceed with no acknowledgement" — the carried files are still worth a sentence,
 *  just not a dialog. */
export function composeCheckoutAnnouncement(preflight: CheckoutPreflight, target: string): string {
  const short = shortTarget(target);
  const where = preflight.detaches ? t('git.op.detachedTarget.label', { target: short }) : short;
  if (preflight.verdict === 'cleanCarry') {
    return t('git.op.checkoutCarried.announce', { where, count: preflight.carried.length });
  }
  return t('git.op.checkout.announce', { where });
}

/** The auto-stash announcement — mandatory, never decoration, since the working tree
 *  reads as clean on the new branch and this sentence is the only place the user learns where
 *  their work went. `fileCount` is summed from the blocked-tracked/blocked-untracked blocker
 *  paths the pre-flight already reported (the exact set the server's own `stash push` swept).
 *  `fromBranch` is the ORIGIN branch (where the switch started, not the target) — the entry is
 *  tagged with the branch it came FROM. */
export function composeAutoStashAnnouncement(fileCount: number, fromBranch: string): string {
  return t('git.op.autoStash.body', { count: fileCount, branch: fromBranch });
}

/** G28 D6: the auto-detach announcement — mandatory, the entire safety story for landing a user
 *  in a detached HEAD they did not directly ask for. Names both the branch and the worktree path
 *  that already holds it. */
export function composeAutoDetachAnnouncement(target: string, worktreePath: string): string {
  return t('git.op.autoDetach.body', { target: shortTarget(target), path: worktreePath });
}

/** §7.10: a `noCommit` revert stages rather than commits, and the toolbar's live region is the
 *  only way a keyboard/screen-reader user learns which one just happened. */
export function composeRevertAnnouncement(shas: readonly string[], noCommit: boolean): string {
  if (shas.length === 1) {
    return t(noCommit ? 'git.op.revertedOneStaged.announce' : 'git.op.revertedOne.announce', {
      sha: shortTarget(shas[0] ?? ''),
    });
  }
  return t(noCommit ? 'git.op.revertedManyStaged.announce' : 'git.op.revertedMany.announce', {
    count: shas.length,
  });
}

/** `docs/plans/P10.md` W10: §7.7's own reset, given a voice — the target's subject is not carried
 *  here (the live region names *what was done*, not the full advisory the dialog already showed
 *  and the user already read before confirming). */
export function composeResetAnnouncement(mode: ResetMode, target: string): string {
  return t('git.op.reset.announce', { mode, target: shortTarget(target) });
}

/** `docs/plans/P10.md` W10, §7.10's own `noCommit` wording reused verbatim for cherry-pick
 *  (§7.13 states the same "staged, not committed" outcome for `--no-commit`). */
export function composeCherryPickAnnouncement(sha: string, noCommit: boolean): string {
  return t(noCommit ? 'git.op.cherryPickedStaged.announce' : 'git.op.cherryPicked.announce', {
    sha: shortTarget(sha),
  });
}

/** `CherryPickPreflight`'s `merge-tree` prediction inherits the whole posture (the stash-pop
 *  precedent) — exact about the merge, reconciled after the fact by `runOp`'s read-back, never
 *  swallowed. No `stashKept` half exists here (unlike `StashPredictionMismatch`): a cherry-pick
 *  never touches the stash, so this is its own, one-field-simpler shape rather than a forced
 *  reuse. */
export interface CherryPickPredictionMismatch {
  readonly predicted: 'clean' | 'conflicts';
  readonly actual: 'clean' | 'conflicts' | 'refused';
}

const CHERRY_PICK_MISMATCH: Record<
  CherryPickPredictionMismatch['predicted'],
  Partial<Record<CherryPickPredictionMismatch['actual'], MessageKey>>
> = {
  clean: {
    conflicts: 'git.op.cherryPickMismatch.cleanConflicts.body',
    refused: 'git.op.cherryPickMismatch.cleanRefused.body',
  },
  conflicts: {
    clean: 'git.op.cherryPickMismatch.conflictsClean.body',
    refused: 'git.op.cherryPickMismatch.conflictsRefused.body',
  },
};

export function composeCherryPickMismatchAnnouncement(
  mismatch: CherryPickPredictionMismatch,
): string {
  const key = CHERRY_PICK_MISMATCH[mismatch.predicted][mismatch.actual];
  // predicted and actual never agree on a mismatch
  return key === undefined ? t('common.done.status') : t(key);
}

const OP_ERROR_REASON: Record<OpErrorKind, MessageKey> = {
  AuthFailed: 'git.op.reason.authFailed.error',
  NonFastForward: 'git.op.reason.nonFastForward.error',
  Conflict: 'git.op.reason.conflict.error',
  DirtyWorktree: 'git.op.reason.dirtyWorktree.error',
  UntrackedWouldBeOverwritten: 'git.op.reason.untrackedWouldBeOverwritten.error',
  LockHeld: 'git.op.reason.lockHeld.error',
  NotFound: 'git.op.reason.notFound.error',
  AlreadyExists: 'git.op.reason.alreadyExists.error',
  NotFullyMerged: 'git.op.reason.notFullyMerged.error',
  WorktreeConflict: 'git.op.reason.worktreeConflict.error',
  OperationInProgress: 'git.op.reason.operationInProgress.error',
  RemoteRefMissing: 'git.op.reason.remoteRefMissing.error',
  HookRejected: 'git.op.reason.hookRejected.error',
  LeaseViolation: 'git.op.reason.leaseViolation.error',
  RemoteRefUpdated: 'git.op.reason.remoteRefUpdated.error',
  NetworkFailed: 'git.op.reason.networkFailed.error',
  RemoteNotFound: 'git.op.reason.remoteNotFound.error',
  ProtectedBranch: 'git.op.reason.protectedBranch.error',
  Cancelled: 'git.op.reason.cancelled.error',
  StashConflict: 'git.op.reason.stashConflict.error',
  StashIndexConflict: 'git.op.reason.stashIndexConflict.error',
  StashUntrackedCollision: 'git.op.reason.stashUntrackedCollision.error',
  ConfirmationRequired: 'git.op.reason.confirmationRequired.error',
  EmptyCherryPick: 'git.op.reason.emptyCherryPick.error',
  MainlineRequired: 'git.op.reason.mainlineRequired.error',
  WorktreeLocked: 'git.op.reason.worktreeLocked.error',
  StackCycle: 'git.op.reason.stackCycle.error',
  NothingToStash: 'git.op.reason.nothingToStash.error',
  BranchChanged: 'git.op.reason.branchChanged.error',
  Unknown: 'git.op.reason.unknown.error',
};

/** Every user-facing operation a failure can name; each has a `git.op.failed.<action>.error` message. */
export type OpAction =
  | 'checkout'
  | 'revert'
  | 'stash'
  | 'reset'
  | 'cherryPick'
  | 'stashPop'
  | 'stashApply'
  | 'dropStash'
  | 'stashBranch'
  | 'saveGlobalStash'
  | 'removeGlobalStash'
  | 'createWorktree'
  | 'removeWorktree'
  | 'setStackParent'
  | 'restack'
  | 'createBranch'
  | 'deleteBranch'
  | 'renameBranch'
  | 'createTag'
  | 'deleteTag'
  | 'pushTag'
  | 'deleteRemoteTag'
  | 'continue'
  | 'abort'
  | 'skip'
  | 'undo'
  | 'fetch'
  | 'pull'
  | 'push'
  | 'forcePush';

/** What was attempted, as its own sentence ("Couldn’t check out."). */
export function opFailedTitle(action: OpAction): string {
  return t(`git.op.failed.${action}.error`);
}

/** Every op failure the live region reports — including one that never reached git at all (a
 *  gated action, `op.run`'s own guard) — reads the same way: what was attempted, then why it
 *  didn't happen. A silent failure is the clipboard's own failure mode (§6.4), applied here. */
export function composeOpFailureAnnouncement(
  action: OpAction,
  error: OpFailureError | undefined,
): string {
  const { title, detail } = composeOpFailureParts(action, error);
  return detail === undefined ? title : `${title} ${detail}`;
}

export interface OpFailureError {
  readonly kind: OpErrorKind;
  readonly message: string;
  readonly remoteMessage?: string | undefined;
}

/** The announcement split into its sentences and the optional first server line, so the visible
 *  failure banner and the live region read from one composition. */
export function composeOpFailureParts(
  action: OpAction,
  error: OpFailureError | undefined,
): { readonly title: string; readonly detail: string | undefined } {
  const what = opFailedTitle(action);
  if (!error) return { title: what, detail: undefined };
  const title = joinSentences(what, t(OP_ERROR_REASON[error.kind]));
  const detail =
    error.kind === 'HookRejected'
      ? error.remoteMessage
      : error.kind === 'Unknown'
        ? error.message
        : undefined;
  return { title, detail: firstLine(detail) };
}

/** Why auto-fetch stopped, in the same wording the failure announcements use. */
export function composeAutoFetchStoppedAnnouncement(kind: OpErrorKind): string {
  const stopped = t('git.op.autoFetchStopped.error', { reason: t(OP_ERROR_REASON[kind]) });
  return joinSentences(stopped, t('git.op.autoFetchResume.hint'));
}

const MAX_DETAIL_LENGTH = 200;

/** First non-empty line of a server message, capped, or undefined when there is none. */
export function firstLine(text: string | undefined): string | undefined {
  const line = text
    ?.split('\n')
    .map((l) => l.trim())
    .find((l) => l !== '');
  if (line === undefined) return undefined;
  return line.length > MAX_DETAIL_LENGTH ? `${line.slice(0, MAX_DETAIL_LENGTH)}…` : line;
}

/** git's own `No local changes to save` no-op exits 0 with no
 *  error, so a plain "succeeded" announcement would be the silent-no-op failure mode already
 *  named for the clipboard — `pushed` is `ops.ts`'s own before/after stash-count comparison,
 *  the only way to tell the two outcomes apart. */
export function composeStashPushAnnouncement(pushed: boolean): string {
  return t(pushed ? 'git.op.stashed.announce' : 'git.op.nothingToStash.announce');
}

/** Non-null only when an executed pop/apply disagreed with the prediction the user was shown
 *  (a race between pre-flight and write, or a gap pre-flight
 *  could not see — never a false positive, since `composeStashAnnouncement` never compares at all
 *  when the prediction itself was `"unknown"`). `stashKept` is a fact for every failure mode
 *  probed (conflict, untracked collision, local-overwrite refusal) — the stash always survives —
 *  except a genuinely CLEAN `pop` (as opposed to `apply`), which removes it exactly as intended
 *  even when that clean outcome is itself the surprise (`predicted: "conflicts"`). */
export interface StashPredictionMismatch {
  readonly predicted: 'clean' | 'conflicts';
  readonly actual: 'clean' | 'conflicts' | 'refused';
  readonly stashKept: boolean;
}

// The backend owns this label (`RepoService`'s stash-drop undo capture) and it stays English.
const STASH_DROP_UNDO_LABEL_PREFIX = 'Dropped stash@{';

/** Undoing a dropped stash does not restore it to its old stack position
 *  — it always lands back at `stash@{0}` — so the generic "Undone: `<label>`" text
 *  would read as if `stash@{2}` (say) came back exactly as it was, which a user watching for that
 *  position would take as a sign the undo failed. Detected from the slot's own `label` rather
 *  than a dedicated field: `UndoSlotSnapshot` carries no "kind" to switch on. */
export function composeUndoAnnouncement(label: string): string {
  return label.startsWith(STASH_DROP_UNDO_LABEL_PREFIX)
    ? t('git.op.undoStash.announce', { ref: stashRef(0) })
    : t('git.op.undone.announce', { label });
}

/** A literal `stash@{N}` cannot sit in a catalog message (`{0}` is a positional placeholder). */
export function stashRef(index: number | string): string {
  return `stash@{${index}}`;
}

/** The undo button's tooltip is the one place a user learns *what a reset's undo restores* before
 *  they need it, and the three modes are not interchangeable (a mixed reset's undo does not bring
 *  back what was staged, because that index state was never written to the object database at all).
 *  Detected from `UndoRecord.label`'s own text (backend-owned, English), the same way
 *  `composeUndoAnnouncement` detects a dropped stash above. */
const RESET_UNDO_LABEL_PATTERN = /^Reset \((soft|mixed|hard)\) to /;

const RESET_UNDO_TOOLTIP: Record<ResetMode, MessageKey> = {
  soft: 'git.undo.tooltipSoft.tooltip',
  mixed: 'git.undo.tooltipMixed.tooltip',
  hard: 'git.undo.tooltipHard.tooltip',
};

export function composeUndoTooltip(label: string): string {
  const resetMatch = label.match(RESET_UNDO_LABEL_PATTERN);
  const mode = resetMatch?.[1] as ResetMode | undefined;
  return t(mode ? RESET_UNDO_TOOLTIP[mode] : 'git.undo.tooltipDefault.tooltip', { label });
}

const STASH_MISMATCH: Record<
  'apply' | 'pop',
  Record<
    StashPredictionMismatch['predicted'],
    Partial<Record<StashPredictionMismatch['actual'], MessageKey>>
  >
> = {
  apply: {
    clean: {
      conflicts: 'git.op.applyMismatch.cleanConflicts.body',
      refused: 'git.op.applyMismatch.cleanRefused.body',
    },
    conflicts: {
      clean: 'git.op.applyMismatch.conflictsClean.body',
      refused: 'git.op.applyMismatch.conflictsRefused.body',
    },
  },
  pop: {
    clean: {
      conflicts: 'git.op.popMismatch.cleanConflicts.body',
      refused: 'git.op.popMismatch.cleanRefused.body',
    },
    conflicts: {
      clean: 'git.op.popMismatch.conflictsClean.body',
      refused: 'git.op.popMismatch.conflictsRefused.body',
    },
  },
};

/** The worked example, given a voice: *"This pop conflicted, though the pre-flight
 *  predicted a clean apply. The tree changed in between. Your stash was kept."* A `mismatch`
 *  takes priority over the plain success/failure text — it is the more specific, more surprising
 *  fact. With no mismatch, this reads exactly like `composeOpFailureAnnouncement`'s own output
 *  for a failure, or a short "Applied"/"Popped" sentence for a plain, agreeing success.
 *
 *  `currentBranch` (optional — omitted means "the caller does not know", which suppresses
 *  the origin clause rather than comparing against `undefined`) adds the origin whenever `entry`'s
 *  own origin branch differs from it — the cross-branch apply's own "you're not where this came
 *  from" reminder. `label` reads `stashLabel(entry)` (the user's own text) rather than the
 *  position-addressed `stash@{N}` — correct for a global entry too, whose `index` is the `-1`
 *  sentinel. */
export function composeStashAnnouncement(
  verb: 'apply' | 'pop',
  entry: StashEntry,
  result: OpResult,
  mismatch: StashPredictionMismatch | null,
  currentBranch?: string | null,
): string {
  const label = stashLabel(entry);
  const origin = currentBranch === undefined ? undefined : originLabel(entry, currentBranch);
  if (mismatch) {
    const key = STASH_MISMATCH[verb][mismatch.predicted][mismatch.actual];
    const parts = key === undefined ? [] : [t(key)];
    if (origin !== undefined) parts.push(t('git.op.mismatchOrigin.body', { origin }));
    if (mismatch.stashKept) parts.push(t('git.op.mismatchKept.body'));
    // A bare "Done" is no sentence: it stands alone only when no sentence follows.
    return parts.length === 0 ? t('common.done.status') : joinSentences(...parts);
  }
  if (!result.ok)
    return composeOpFailureAnnouncement(verb === 'apply' ? 'stashApply' : 'stashPop', result.error);
  if (origin === undefined)
    return t(verb === 'apply' ? 'git.op.applied.announce' : 'git.op.popped.announce', { label });
  return t(verb === 'apply' ? 'git.op.appliedFrom.announce' : 'git.op.poppedFrom.announce', {
    label,
    origin,
  });
}

/** `stack.restack`'s own live-region text — a full success, a paused conflict
 *  (`stoppedAt` set), and a cancellation (`stoppedAt` undefined but `remaining` non-empty) all read
 *  differently. `StackList.vue`'s own paused-restack strip renders the same "paused on X" text
 *  inline, not just the live region — kept as one function so the two can never disagree. */
export function composeRestackAnnouncement(
  restacked: readonly string[],
  stoppedAt: string | undefined,
  remaining: readonly string[],
): string {
  if (stoppedAt !== undefined) {
    return restacked.length > 0
      ? t('git.op.restackPausedDone.body', {
          branch: stoppedAt,
          done: restacked.length,
          count: remaining.length,
        })
      : t('git.op.restackPaused.body', { branch: stoppedAt, count: remaining.length });
  }
  if (remaining.length > 0) {
    return t('git.op.restackCancelled.body', { done: restacked.length, count: remaining.length });
  }
  if (restacked.length === 0) return t('git.op.restackUpToDate.body');
  return restacked.length === 1
    ? t('git.op.restackedOne.announce', { branch: restacked[0] })
    : t('git.op.restackedMany.announce', { count: restacked.length });
}
