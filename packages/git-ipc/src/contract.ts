/**
 * The type map every transport (real or mock) and the UI client are checked against.
 * Every entry here has a producer and a consumer.
 *
 * `core` and `ipc` both depend on nothing, so the wire shapes below are *structural
 * copies* of the corresponding `@kira/git-core` types, not imports of them — `ipc` may
 * not import `@kira/git-core`. Drift between the two sides is caught by
 * `tests/unit/ipc/wireConformance.spec.ts`, which asserts assignability in both directions,
 * not by an import the lint rule would reject anyway.
 */

// ---------------------------------------------------------------------------------------
// Structural copies of core's wire-relevant types — kept honest by wireConformance.spec.ts.
// ---------------------------------------------------------------------------------------

export type HeadState =
  | { readonly kind: 'branch'; readonly name: string }
  | { readonly kind: 'detached'; readonly sha: string }
  | { readonly kind: 'unborn'; readonly name: string };

/** G19 D11b: the durable review-session snapshot — identifiers only, never commit/diff data (see
 *  `review.session.save`/`.load`'s own doc comments below for why). Named once, here, and reused
 *  by both requests rather than repeated inline. */
export interface ReviewSessionSnapshot {
  readonly branch: string;
  readonly baseOverride: string | null;
  readonly pane: 'commits' | 'files' | 'comments';
  readonly listMode: 'tree' | 'flat';
  readonly filter: string;
  readonly diffMode: 'sinceReview' | 'range';
}

export type DecorationRef =
  | { readonly kind: 'branch'; readonly name: string; readonly isHead: boolean }
  | { readonly kind: 'remoteBranch'; readonly name: string }
  | { readonly kind: 'tag'; readonly name: string }
  | { readonly kind: 'head' }
  | { readonly kind: 'stash'; readonly index: number };

/** G18 D4: the per-repo display settings, server-stored, edited from the new in-app dialog
 *  (`RepoSettingsDialog.vue`) rather than per-window settings — every leaf here is genuinely
 *  scoped by repoId. */
export interface RepoSettingsSnapshot {
  readonly 'kiraSpace.graph.pageSize': number;
  readonly 'kiraSpace.graph.scope': 'all' | 'head';
  /** P9 W6: whether stash entries appear as nodes in the commit graph (OQ5 default: true). */
  readonly 'kiraSpace.stash.showInGraph': boolean;
  /** P9 W6: the Stash dialog's "include untracked files" checkbox default. */
  readonly 'kiraSpace.stash.includeUntracked': boolean;
  /** P7 W7/D43: Branch review's own candidate base branches (§6.8). */
  readonly 'kiraSpace.review.baseCandidates': readonly string[];
  readonly 'kiraSpace.pull.strategy': 'auto' | 'ff-only' | 'merge' | 'rebase';
  /** Which git host supplies change-request status: `auto` (detect from origin, the default),
   *  `off` (no host call, no cache fill: lookups answer `{kind:'disabled'}`) or a provider id.
   *  An unknown id behaves as `auto`. */
  readonly 'kiraSpace.gitHost.provider': string;
  /** The worktree prepare script — one command-line string, never a path, never an argv
   *  array. Empty means the feature is off for this repository: no spawn, no shell, ever. Read
   *  ONLY from this table (`source: 'repo'`) — never from `.git/config`, a tracked file, or any
   *  repo-carried convention, which is the single highest-value safety property in the whole
   *  feature. Written only by Kira Space itself: `RepoSettingsPatch` omits it and
   *  `repoSettings.set` refuses it with `E_READ_ONLY`. */
  readonly 'kiraSpace.worktree.prepareScript': string;
  /** G25 D10: pure UX — pre-fills `WorktreeDialog`'s own path field. Empty means no suggestion
   *  beyond the dialog's own basename default. Never a security boundary. */
  readonly 'kiraSpace.worktree.basePath': string;
  /** Whether a blocked checkout is automatically re-issued with `autoStash: true` instead
   *  of opening the old `CheckoutDialog`. Read CLIENT-SIDE ONLY — the server never consults this
   *  leaf, so a stale or absent value can only ever produce the OLD dialog, never an unexpected
   *  write (the fail-safe direction). Default true. */
  readonly 'kiraSpace.checkout.autoStash': boolean;
}

/** G18: `RepoSettingsSnapshot`'s own `.partial()` shape — `repoSettings.set`'s request, every leaf
 *  optional so the dialog patches only the field the user actually changed. */
export type RepoSettingsPatch = Partial<
  Omit<RepoSettingsSnapshot, 'kiraSpace.worktree.prepareScript'>
>;

export interface RepoSummary {
  readonly repoId: string;
  readonly root: string;
  readonly gitDir: string;
  readonly commonDir: string;
  readonly isBare: boolean;
  readonly isLinkedWorktree: boolean;
  readonly head: HeadState;
}

export interface RepoCandidate {
  readonly path: string;
  readonly label: string;
}

/** A commit's author or committer identity (§4.4) — structural copy of `core`'s own, needed
 *  because `commit.detail`'s result embeds it directly (P5 W4). */
export interface CommitIdentity {
  readonly name: string;
  readonly email: string;
  readonly timestamp: number; // unix seconds
}

/** P5's diff model, wired: structural copies of `core/src/model/diff.ts` and the `FileChange`/
 *  `SignatureStatus` halves of `core/src/model/commit.ts`, kept honest by
 *  `wireConformance.spec.ts`. */
export interface CommitTrailer {
  readonly token: string;
  readonly value: string;
}

export type FileChangeKind =
  | 'added'
  | 'modified'
  | 'deleted'
  | 'renamed'
  | 'copied'
  | 'typeChanged'
  | 'unmerged';

export interface FileChange {
  readonly kind: FileChangeKind;
  readonly path: string;
  /** Set for `renamed`/`copied` only — and load-bearing: a per-file diff of a rename must
   *  name both paths in its pathspec or git renders it as a whole-file add (probe P2). */
  readonly originalPath: string | undefined;
  readonly similarity: number | undefined;
}

/** `%G?`'s raw signature-verification code (§4.4, D20): good, bad, unknown key, expired, etc. */
export type SignatureStatus = 'G' | 'B' | 'U' | 'X' | 'Y' | 'R' | 'E' | 'N';

export type DiffLineKind = 'context' | 'add' | 'del';

export interface DiffLine {
  readonly kind: DiffLineKind;
  /** Without the leading marker character. */
  readonly text: string;
  readonly oldLine: number | undefined;
  readonly newLine: number | undefined;
  /** git's `\ No newline at end of file`, attached to the line it followed. */
  readonly noNewlineAtEof: boolean;
}

export interface DiffHunk {
  readonly oldStart: number;
  readonly oldLines: number;
  readonly newStart: number;
  readonly newLines: number;
  /** Whatever git put after the closing `@@` — rendered in the hunk header row. */
  readonly heading: string;
  readonly lines: readonly DiffLine[];
}

export type FileDiffBody =
  | { readonly kind: 'text'; readonly hunks: readonly DiffHunk[] }
  | {
      readonly kind: 'binary';
      readonly oldBytes: number | undefined;
      readonly newBytes: number | undefined;
    }
  | { readonly kind: 'lfsPointer'; readonly oid: string; readonly bytes: number }
  | { readonly kind: 'tooLarge'; readonly bytes: number; readonly limitBytes: number }
  | { readonly kind: 'empty'; readonly reason: 'modeChangeOnly' | 'identical' };

/** D14a's "Go to file" outcome — wire-only, produced entirely by `rpcHandlers.ts`'s
 *  `editor.goToFile` handler, so it has no `core` counterpart to keep in step with. */
export type GoToFileOutcome =
  | { readonly kind: 'liveFile'; readonly path: string; readonly line: number }
  | {
      readonly kind: 'virtualBlob';
      readonly path: string;
      readonly rev: string;
      readonly line: number;
    }
  | { readonly kind: 'unavailable'; readonly reason: 'notInRevision' | 'binary' | 'tooLarge' };

/**
 * The wire shape for a slice of `CommitStore` rows — the packed, transferable representation
 * `CommitStore.packSlice`/`appendPacked` (`packages/core/src/store/commitStore.ts`) produce and
 * consume. Declared here (structurally, not imported) because it is the payload of
 * `GraphChunk.commits` below; `wireConformance.spec.ts` is what keeps the two in step.
 */
export interface PackedCommitChunk {
  readonly from: number;
  readonly to: number;
  readonly shaWidthBytes: number;
  /** `(to - from) * shaWidthBytes` bytes, binary (§5.5). */
  readonly shas: ArrayBuffer;
  /** `Uint32Array`, `(to - from) + 1` entries, chunk-relative CSR offsets. */
  readonly parentOffsets: ArrayBuffer;
  /** Binary shas in CSR order — parents travel as shas, not row indices (W3). */
  readonly parentShas: ArrayBuffer;
  /** `Uint32Array`, 4 per row (authorName, authorEmail, committerName, committerEmail), into
   *  `dictionary`. */
  readonly identityIds: ArrayBuffer;
  /** `Uint32Array`, 2 per row (authorTime, committerTime). */
  readonly times: ArrayBuffer;
  readonly subjectBytes: ArrayBuffer;
  readonly subjectOffsets: ArrayBuffer;
  /** The first dictionary id this chunk's `dictionary` array defines — the receiver's interner
   *  must be at exactly this size, or the chunk is out of order (W3). */
  readonly dictionaryBase: number;
  /** Only the strings interned since `dictionaryBase` — a delta, not the whole dictionary. */
  readonly dictionary: readonly string[];
  readonly decorations: readonly (readonly [row: number, refs: readonly DecorationRef[]])[];
}

// ---------------------------------------------------------------------------------------
// Refs, status, pre-flight and operations. Structural copies of `packages/core`'s own
// (`model/ref.ts`, `model/operation.ts`, `model/status.ts`, `preflight/types.ts`, `undo/slot.ts`),
// kept honest by `tests/unit/ipc/wireConformance.spec.ts` rather than an import.
// ---------------------------------------------------------------------------------------

export type RefKind = 'branch' | 'remoteBranch' | 'tag';

export interface RefTrack {
  readonly ahead: number;
  readonly behind: number;
}

/** The tagger identity and message subject of an *annotated* tag — `undefined` for a lightweight
 *  one (never an empty annotation: `%(contents:subject)` on a lightweight tag returns the
 *  pointed-at commit's subject, which would read as an annotation that is not there). */
export interface TagAnnotation {
  readonly tagger: string;
  readonly date: number; // unix seconds
  readonly subject: string;
  /** `docs/plans/P11.md` W4/probe 7: the tag message's full body, from `%(contents:body)` —
   *  populated only by the tags-only scope's spawn. */
  readonly body: string;
}

export interface RefRow {
  readonly refname: string; // refs/heads/main
  readonly kind: RefKind;
  readonly shortName: string; // main, origin/main, v1.2.0
  /** For an annotated tag: the TAG OBJECT's sha — which is what undo needs (probe P3). */
  readonly objectId: string;
  readonly peeledObjectId: string | undefined; // %(*objectname); annotated tags only
  readonly upstream: string | undefined;
  readonly track: RefTrack | 'gone' | undefined;
  readonly committerDate: number;
  readonly isHead: boolean;
  /** D12. Absolute path of the worktree holding this branch checked out, when that worktree is
   *  NOT this session's own. `undefined` for every other ref, including the branch checked out
   *  here — `%(worktreepath)` is populated for both and the service subtracts its own toplevel. */
  readonly checkedOutIn: string | undefined;
  /** Present iff this is an annotated tag. */
  readonly annotation: TagAnnotation | undefined;
}

export type InProgressKind =
  | 'merge'
  | 'cherryPick'
  | 'revert'
  | 'rebase'
  | 'bisect'
  | 'unmergedOnly';

export interface InProgressOperation {
  readonly kind: InProgressKind;
  /** MERGE_HEAD / CHERRY_PICK_HEAD / REVERT_HEAD, or rebase's `onto`. */
  readonly otherSha: string | undefined;
  /** rebase only: `rebase-merge/head-name`, e.g. `refs/heads/side`. */
  readonly headName: string | undefined;
  readonly conflictedPaths: readonly string[];
  /** True only where `git <op> --continue` exists AND v1 offers it — false only for bisect
   *  (nothing to continue) and unmergedOnly (no state file to advance). Rebase is true once the
   *  stack.restack executor does start a rebase. */
  readonly canContinue: boolean;
  readonly canAbort: boolean;
  /** `.git/sequencer/` present: a multi-commit revert or cherry-pick mid-run, where --abort is
   *  what delivers §7.10's all-or-nothing. */
  readonly isSequence: boolean;
  /** Continue is *enabled* only when this is 0 (§7.11). Kept separate from
   *  `conflictedPaths.length` so a host that caps the path list cannot accidentally enable it. */
  readonly unmergedCount: number;
  /** P10 probe 6: true for `cherryPick` and `revert` — the two sequencer operations git gives a
   *  `--skip` — and, since G26 D12, `rebase` too (git's own conflict hint names
   *  `git rebase --skip` verbatim). */
  readonly canSkip: boolean;
}

/** P173: set only while background auto-fetch is stopped by a permanent failure (auth, missing
 *  remote, unknown). A successful explicit fetch or pull clears it. `kind` is the classified
 *  fetch error; `message` is its first stderr line. */
export interface AutoFetchStatus {
  readonly state: 'stopped';
  readonly kind: OpErrorKind;
  readonly message: string;
  /** ISO timestamp of the stop. */
  readonly at: string;
}

export interface StatusSummary {
  readonly head: HeadState;
  readonly upstream:
    | { readonly name: string; readonly ahead: number; readonly behind: number }
    | undefined;
  readonly counts: {
    readonly staged: number;
    readonly unstaged: number;
    readonly untracked: number;
    readonly unmerged: number;
  };
  readonly isClean: boolean;
  /** Bounded (W8's 200-entry display cap). §7.5 requires naming the exact files; it does not
   *  require naming ten thousand of them. `dirtyTruncated` says the list was cut. */
  readonly dirtyPaths: readonly string[];
  readonly dirtyTruncated: boolean;
  readonly inProgress: InProgressOperation | null;
  /** P173: `null` while auto-fetch runs, is paused or was never armed. */
  readonly autoFetch: AutoFetchStatus | null;
}

export type CheckoutBlocker =
  | { readonly kind: 'blockedByTracked'; readonly paths: readonly string[] }
  | { readonly kind: 'blockedByUntracked'; readonly paths: readonly string[] }
  | { readonly kind: 'inProgressOperation'; readonly operation: InProgressOperation }
  | { readonly kind: 'worktreeConflict'; readonly branch: string; readonly worktreePath: string };

export interface CheckoutPreflight {
  readonly target: { readonly kind: RefKind | 'sha'; readonly name: string };
  /** True when the result is a detached HEAD: a tag, a raw sha, or an explicitly detached
   *  checkout of a remote-tracking ref. */
  readonly detaches: boolean;
  /** Set when the only way to land ON A BRANCH is to create one tracking a remote — the DWIM
   *  case (probe P7), surfaced as an explicit choice instead of happening silently. */
  readonly createsTracking: { readonly branch: string; readonly upstream: string } | undefined;
  /** D \ T: the local changes that will survive the switch. Empty on a clean tree. */
  readonly carried: readonly string[];
  readonly blockers: readonly CheckoutBlocker[];
  /** "clean" = nothing local at all. "cleanCarry" = §7.5's carry case; still no prompt, but the
   *  confirmation copy differs and the UI announces what carried. */
  readonly verdict: 'clean' | 'cleanCarry' | 'blocked';
  /** Which routes the UI may offer for a `blocked` verdict. Emits `["discard"]` (or `[]` when
   *  an untracked block is also present); adds `"stashAndCarry"`. Adds two more:
   *  `"autoStash"` (whole-tree `stash push [-u]`, tagged with the CURRENT branch, never popped
   *  back — offered whenever EITHER dirty-blocker kind is present, unlike the two routes above,
   *  which the untracked case suppresses) and `"detachHere"` (offered whenever a `worktreeConflict`
   *  blocker is present for a `switch`-mode request to a branch/remote-branch target — composes
   *  freely with `"autoStash"` when both blocker kinds are present at once). */
  readonly routes: readonly ('discard' | 'stashAndCarry' | 'autoStash' | 'detachHere')[];
}

export interface RevertParentChoice {
  readonly parentNumber: number; // 1-based, as `-m` takes it
  readonly sha: string;
  readonly subject: string;
}

export interface RevertPreflight {
  readonly shas: readonly string[];
  /** Non-empty ⇒ the user MUST pick before the op is offered (§7.10: "rather than guessing
   *  -m 1"). One entry per merge commit among `shas` whose mainline is not already resolved. */
  readonly mainlineRequired: readonly {
    readonly sha: string;
    readonly parents: readonly RevertParentChoice[];
  }[];
  readonly dirtyPaths: readonly string[];
  readonly inProgress: InProgressOperation | null;
  /** §7.10's merge-tree prediction. Scoped to `shas[0]` — `predictedFor` says so, and the UI
   *  quotes it when `shas.length > 1`. `unknown` when the prediction itself could not run. */
  readonly prediction:
    | { readonly kind: 'clean' }
    | { readonly kind: 'conflicts'; readonly paths: readonly string[] }
    | { readonly kind: 'unknown'; readonly reason: string };
  readonly predictedFor: string | null;
  /** §7.10: allowed, with a note. Not a blocker. */
  readonly detachedHead: boolean;
  readonly verdict: 'clean' | 'willConflict' | 'blocked';
  readonly blockers: readonly ('dirtyWorktree' | 'inProgressOperation' | 'mainlineRequired')[];
}

// ---------------------------------------------------------------------------------------
// Stash. Structural copies of `@kira/git-core`'s own; `RefKind` stays
// untouched — a stash is its own request, never part of `refs.list`.
// ---------------------------------------------------------------------------------------

export interface StashEntry {
  /** The entry's own `stash@{N}` position — meaningful only for `scope: 'stack'`. `-1` for a
   *  `scope: 'global'` entry, which has no stack position at all (the sentinel; guarded
   *  structurally, not by convention — `stashPop`/`stashDrop` are never offered for a global entry,
   *  and `stashBranch` takes a sha-addressed arm for one instead of ever reading this field). */
  readonly index: number;
  readonly sha: string;
  readonly baseSha: string;
  /** `baseSha`'s own commit subject — see `@kira/git-core`'s own `StashEntry.baseSubject`
   *  doc comment (P9 W14). */
  readonly baseSubject: string;
  readonly indexSha: string;
  readonly untrackedSha: string | undefined;
  readonly message: string;
  readonly branch: string | null;
  readonly timestamp: number;
  readonly fileCount: number;
  readonly includedUntracked: boolean;
  /** Which bucket this entry lives in — `'stack'` (an ordinary `refs/stash` entry,
   *  `stash.list`) or `'global'` (`refs/kira/globalstash/<sha>`, `globalStash.list`). Widening this
   *  one field, rather than a parallel `GlobalStashEntry` interface, is the closing
   *  argument: a fork would touch `StashDetailPane.vue`, `stash.show`, `preflight.stashPop`,
   *  `preflight.stashBranch` and every announcement helper for one field's worth of real
   *  difference. */
  readonly scope: 'stack' | 'global';
  /** G28 D8/D17: `''` for a stack entry (addressed by position, never by ref); the entry's own
   *  `refs/kira/globalstash/<sha>` for a global one. */
  readonly ref: string;
}

export type StashPopBlocker =
  | { readonly kind: 'untrackedCollision'; readonly paths: readonly string[] }
  | { readonly kind: 'localChangesWouldBeOverwritten'; readonly paths: readonly string[] }
  | { readonly kind: 'inProgressOperation'; readonly operation: InProgressOperation };

export interface StashPopPreflight {
  readonly stashSha: string;
  readonly stashIndex: number;
  /** The commit the stash would be applied onto — HEAD today, or the checkout target when this
   *  is the `stashAndCarry` route's second step. */
  readonly targetSha: string;
  /** §7.6's exact prediction. Covers the WORKTREE MERGE ONLY — never `--index` restoration and
   *  never the two blockers below. */
  readonly prediction:
    | { readonly kind: 'clean' }
    | { readonly kind: 'conflicts'; readonly paths: readonly string[] }
    | { readonly kind: 'unknown'; readonly reason: string };
  readonly blockers: readonly StashPopBlocker[];
  readonly verdict: 'clean' | 'willConflict' | 'blocked';
}

export interface StashBranchPreflight {
  readonly name: {
    readonly valid: boolean;
    readonly error: string | undefined;
    readonly exists: boolean;
  };
  /** The `checkout -b <name> <stash>^` half. No pop prediction exists: the branch starts at the
   *  stash's own base, so the apply is clean by construction (probe 11). */
  readonly checkout: CheckoutPreflight;
  readonly verdict: 'clean' | 'invalidName' | 'blocked';
}

// ---------------------------------------------------------------------------------------
// Reset and cherry-pick pre-flight. Structural copies of
// `@kira/git-core`'s own; `tests/unit/ipc/wireConformance.spec.ts` keeps the two in step.
// ---------------------------------------------------------------------------------------

export type ResetMode = 'soft' | 'mixed' | 'hard';

export interface ResetPreflight {
  readonly target: string;
  readonly targetSubject: string;
  readonly mode: ResetMode;
  readonly currentHead: string;
  /** `null` on a detached HEAD. */
  readonly branch: string | null;
  readonly leaving: number;
  readonly gaining: number;
  readonly leavingCommits: readonly { readonly sha: string; readonly subject: string }[];
  readonly leavingTruncated: boolean;
  readonly dirty: {
    readonly staged: readonly string[];
    readonly unstaged: readonly string[];
    readonly untracked: readonly string[];
  };
  /** What `--hard` will actually destroy. Empty for `soft`/`mixed`, always. */
  readonly destroys: readonly string[];
  readonly inProgress: InProgressOperation | null;
  readonly requiresTypedConfirmation: boolean;
  readonly routes: readonly 'stashFirst'[];
  readonly verdict: 'clean' | 'destructive' | 'blocked';
  readonly blockers: readonly ('inProgressOperation' | 'unknownTarget')[];
}

export type CherryPickBlocker =
  | { readonly kind: 'inProgressOperation'; readonly operation: InProgressOperation }
  | { readonly kind: 'mainlineRequired'; readonly parents: readonly RevertParentChoice[] }
  | { readonly kind: 'stagedChanges'; readonly paths: readonly string[] }
  | { readonly kind: 'localChangesWouldBeOverwritten'; readonly paths: readonly string[] }
  | { readonly kind: 'untrackedWouldBeOverwritten'; readonly paths: readonly string[] };

export interface CherryPickPreflight {
  readonly sha: string;
  readonly subject: string;
  readonly mainlineRequired: readonly RevertParentChoice[];
  readonly prediction:
    | { readonly kind: 'clean' }
    | { readonly kind: 'conflicts'; readonly paths: readonly string[] }
    | { readonly kind: 'unknown'; readonly reason: string };
  readonly alreadyApplied: boolean;
  readonly inProgress: InProgressOperation | null;
  readonly detachedHead: boolean;
  readonly verdict: 'clean' | 'willConflict' | 'blocked';
  readonly blockers: readonly CherryPickBlocker[];
}

// ---------------------------------------------------------------------------------------
// Worktree support. No upstream to structurally copy from — these are the original design, ported
// to `@kira/git-core`'s `preflight/worktree.ts` verbatim so the two sides never drift (the same
// "authoritative Go twin, ported TS copy" split `preflight/reset.ts` already established).
// ---------------------------------------------------------------------------------------

/** `worktree.list`'s own per-entry shape — a direct read of `git worktree list --porcelain
 *  -z`, crossed with this session's own identity (`isCurrent`) and every other connection's
 *  (`openElsewhere`). Never cached: the watcher's own `commonDir/worktrees` blind spot
 *  is exactly the kind of staleness a cache would make the first thing a user notices. */
export interface WorktreeEntry {
  readonly path: string;
  readonly head: string | null;
  readonly branch: string | null;
  readonly isBare: boolean;
  readonly isDetached: boolean;
  readonly isMain: boolean;
  readonly isCurrent: boolean;
  readonly locked: { readonly reason: string } | null;
  readonly prunable: { readonly reason: string } | null;
  readonly openElsewhere: boolean;
}

/** `preflight.worktreeAdd`'s own blocker union — five kinds, in the exact order the Go
 *  classifier reports them: `invalidPath`, `pathExists`, `branchCheckedOutElsewhere`,
 *  `branchExists`, `unknownStartPoint`. Every applicable blocker is reported at once — an earlier
 *  one never suppresses a later one. */
export type WorktreeAddBlocker =
  | { readonly kind: 'invalidPath'; readonly path: string }
  | { readonly kind: 'pathExists'; readonly path: string }
  | {
      readonly kind: 'branchCheckedOutElsewhere';
      readonly branch: string;
      readonly worktreePath: string;
    }
  | { readonly kind: 'branchExists'; readonly branch: string }
  | { readonly kind: 'unknownStartPoint'; readonly startPoint: string };

/** `preflight.worktreeAdd`'s own note union — informational only, never gates `verdict`.
 *  `pathInsideRepo` is the named example (git allows nesting a worktree inside the
 *  repository it belongs to; refusing a legal action would be this app deciding for the user).
 *  `parentDirectoryMissing`/`detachedHead` are a reasonable fill-in, flagged here as a genuine
 *  ambiguity resolved locally. */
export type WorktreeAddNote =
  | { readonly kind: 'pathInsideRepo'; readonly path: string }
  | { readonly kind: 'parentDirectoryMissing'; readonly path: string }
  | { readonly kind: 'detachedHead'; readonly path: string };

export interface WorktreeAddPreflight {
  readonly path: string;
  readonly mode: 'existingBranch' | 'newBranch' | 'detach';
  readonly branch: string | undefined;
  readonly blockers: readonly WorktreeAddBlocker[];
  readonly notes: readonly WorktreeAddNote[];
  readonly verdict: 'clean' | 'blocked';
  /** `["detachHere"]` iff `branchCheckedOutElsewhere` is the SOLE blocker.
   *  Unlike `CheckoutPreflight.routes`' own `"detachHere"`, this one is only ever
   *  OFFERED (a dialog button), never taken automatically: `worktree add` has no "never blocks"
   *  promise to keep, and a dialog is already open to ask in. */
  readonly routes: readonly 'detachHere'[];
}

/** `preflight.worktreeRemove`'s own blocker union, verbatim. `notAWorktree` is ALSO the
 *  security check that guarantees the app never runs a destructive command against a
 *  caller-supplied arbitrary path — checked, and reported, first. `mainWorktree`/`currentWorktree`
 *  are unconditional (no force route ever unlocks them); `openInAnotherWindow` names its own
 *  remedy, no force route; `locked` gets no force route either (git itself needs a
 *  second, undelivered `-f` to override a lock — this app never sends it). */
export type WorktreeRemoveBlocker =
  | { readonly kind: 'notAWorktree'; readonly path: string }
  | { readonly kind: 'mainWorktree' }
  | { readonly kind: 'currentWorktree' }
  | { readonly kind: 'openInAnotherWindow' }
  | { readonly kind: 'locked'; readonly reason: string };

export interface WorktreeRemovePreflight {
  readonly path: string;
  readonly blockers: readonly WorktreeRemoveBlocker[];
  readonly requiresTypedConfirmation: boolean;
  /** The worktree's own basename — set only when `requiresTypedConfirmation` is true. Echoed back
   *  as `op.run`'s own `confirmToken` field; the server always re-derives and re-checks this fresh
   *  against a freshly re-read status, never trusting a client-echoed value. */
  readonly confirmToken: string | undefined;
  readonly routes: readonly 'force'[];
  readonly verdict: 'clean' | 'dirty' | 'blocked';
}

// ---------------------------------------------------------------------------------------
// Stacked branches: the parent pointer, the restack executor, and stack navigation.
// No upstream to structurally copy from — these nine types are original design.
// Unlike `preflight/reset.ts`'s own "authoritative Go twin, ported TS copy" split (needed there
// for `previewResetMode`'s no-round-trip mode-radio recompute), `ClassifyRestack` itself is NOT
// ported to `@kira/git-core`: nothing in `StackDialog.vue` recomputes a preflight verdict
// client-side ahead of a round trip, so `gitpreflight.ClassifyRestack` (Go) is the ONLY
// implementation — `packages/git-ui/src/lib/stackListModel.ts` is a pure presentation
// layer over whatever `stack.list`/`preflight.restack` already returned, never a second
// classifier.
//
// These nine types ARE structural copies of `internal/gitpreflight/stack.go`'s wire types
// (`StackBranch`/`StackSummary`/`RestackPreflight`) and `internal/gitsession/stack.go`'s
// (`RestackResult`/`RestackProgress`) — kept honest by hand (this repo carries no
// `wireConformance.spec.ts`) rather than by a
// conformance test.
// ---------------------------------------------------------------------------------------

export type StackBranchState = 'upToDate' | 'needsRestack' | 'parentMissing';

/** `stack.list`'s own per-branch row (D3) — `depth` carries a fork's own tree shape without a
 *  recursive wire type; `track`/`checkedOutIn`/`isHead` are reused VERBATIM from `RefRow` (F14),
 *  never re-derived. */
export interface StackBranch {
  readonly name: string;
  /** The recorded parent — a local branch in this same stack, or the stack's own base. Still
   *  named here even when it no longer resolves (an orphan's own remedy needs to say what was
   *  recorded). */
  readonly parent: string;
  /** 0 for a branch sitting directly on the stack's base; meaningless (0) for an orphan. */
  readonly depth: number;
  readonly tip: string;
  /** `undefined` ⇒ `state === 'parentMissing'`. */
  readonly parentTip: string | undefined;
  /** `branch.<name>.kirastackbase`, when recorded — regardless of whether it still resolves as a
   *  commit (D14 is where that resolvability actually matters, for `RestackPlanEntry.baseSource`). */
  readonly recordedBase: string | undefined;
  /** Commits on the parent this branch does not have — `> 0` ⇒ `needsRestack` (D4), and nothing
   *  else ever decides staleness. */
  readonly behind: number;
  /** This branch's own commits since the merge base with its parent. */
  readonly ahead: number;
  readonly state: StackBranchState;
  readonly checkedOutIn: string | undefined;
  readonly track: RefTrack | 'gone' | undefined;
  readonly isHead: boolean;
}

export interface StackSummary {
  /** The branch (or remote-tracking branch) every root in this stack sits on — itself never a
   *  member of `branches` (D1). */
  readonly base: string;
  /** `undefined` only when `base` is a remote-tracking branch that has since been pruned. */
  readonly baseTip: string | undefined;
  /** Pre-order, bottom-to-top (D3): a parent always precedes every one of its children. A stack
   *  is a FOREST, not strictly a chain (§10.6) — a parent may have more than one child. */
  readonly branches: readonly StackBranch[];
  readonly needsRestack: boolean;
}

export interface StackListResult {
  readonly stacks: readonly StackSummary[];
  /** Branches whose recorded parent no longer resolves to any ref, or that sit in a cycle — never
   *  silently dropped, always surfaced with a remedy. Always `state: 'parentMissing'`. */
  readonly orphans: readonly StackBranch[];
}

/** `preflight.restack`'s own blocker union — six kinds, in the exact fixed order the Go
 *  classifier reports them: `inProgressOperation`, `notStacked`, `cycle`, `parentMissing`,
 *  `checkedOutElsewhere`, `dirtyWorktree`. The first is the dialog's own headline, the same
 *  convention `CheckoutBlocker`/`CherryPickBlocker` already established. */
export type RestackBlocker =
  | { readonly kind: 'inProgressOperation'; readonly operation: InProgressOperation }
  | { readonly kind: 'notStacked'; readonly branch: string }
  | { readonly kind: 'cycle'; readonly branches: readonly string[] }
  | { readonly kind: 'parentMissing'; readonly branch: string; readonly parent: string }
  | {
      readonly kind: 'checkedOutElsewhere';
      readonly branch: string;
      readonly worktreePath: string;
    }
  | { readonly kind: 'dirtyWorktree'; readonly paths: readonly string[] };

export interface RestackPlanEntry {
  readonly branch: string;
  readonly parent: string;
  readonly base: string;
  /** `'recorded'` when `kirastackbase` still resolves as a commit; `'mergeBase'` otherwise (D14/
   *  F4 — merge-base is the honest FALLBACK, never the primary source: it is provably wrong the
   *  moment a parent is amended, probe P6). */
  readonly baseSource: 'recorded' | 'mergeBase';
  readonly commits: number;
  /** `'stale'`: this branch's own `behind > 0`. `'ancestorRestacked'`: it is itself up to date,
   *  but an ancestor of it in the stack is in the plan (D14's own cascade rule — a branch cannot
   *  be stale without its ancestors being at least as stale). */
  readonly reason: 'stale' | 'ancestorRestacked';
}

export interface RestackPreflight {
  readonly base: string;
  /** Always the WHOLE stack (D14) — never "from this branch up": a branch cannot be stale
   *  without its ancestors being at least as stale, so a partial plan would rebase onto a parent
   *  that is itself about to move. */
  readonly plan: readonly RestackPlanEntry[];
  readonly blockers: readonly RestackBlocker[];
  readonly verdict: 'clean' | 'noop' | 'blocked';
  /** Where HEAD is returned to (F5: rebase always moves HEAD, even on a no-op) — a branch name,
   *  or a short sha for a detached HEAD. */
  readonly restoresHead: string;
  /** Branches with an upstream that this restack will make diverge — a force-push-with-lease is
   *  needed afterwards (F14). This phase never acts on it automatically (§8's own non-goal). */
  readonly needsForcePush: readonly string[];
  readonly routes: readonly 'stashFirst'[];
}

/** `stack.restack`'s own result — a `Conflict`/dirty-tree/base-write failure pauses
 *  the restack at `stoppedAt` with NO undo record set (replaying `update-ref`s underneath a
 *  running `rebase-merge` sequencer would leave it pointing at commits the refs no longer name).
 *  A cancellation stops between branches, never mid-rebase, and also sets no undo record. */
export interface RestackResult {
  readonly ok: boolean;
  readonly error: { readonly kind: OpErrorKind; readonly message: string } | undefined;
  readonly restacked: readonly string[];
  readonly stoppedAt: string | undefined;
  readonly remaining: readonly string[];
  readonly undo: UndoSlotSnapshot | undefined;
  readonly head: HeadState;
  readonly inProgress: InProgressOperation | null;
}

/** `stack.progress`'s own event payload — emitted BEFORE each branch's own rebase
 *  spawn, the same "coalesce, never withhold past a bound" family `remote.progress`
 *  already established, though a restack's own per-branch granularity needs
 *  no throttling of its own (there is at most one event per branch, never a stream within one). */
export interface RestackProgress {
  readonly repoId: string;
  readonly branch: string;
  readonly index: number; // 1-based
  readonly total: number;
}

// ---------------------------------------------------------------------------------------
// Remote-op vocabulary, and pull/push pre-flight. Structural copies of `@kira/git-core`'s own (core
// and ipc both depend on nothing, so neither imports the other);
// `tests/unit/ipc/wireConformance.spec.ts` keeps the two in step.
// ---------------------------------------------------------------------------------------

export type PullStrategy = 'ff-only' | 'merge' | 'rebase';

/** Where a resolved pull strategy came from, so the UI can say so before running it (§7.3). */
export type PullStrategySource =
  | 'explicit' // the user picked it for this invocation
  | 'setting' // kiraSpace.pull.strategy
  | 'branchConfig' // branch.<name>.rebase
  | 'pullConfig' // pull.rebase / pull.ff
  | 'default'; // kira-space's own ff-only fallback

/** Which remote operation `remote.run` is being asked to perform. One key, five kinds — see
 *  `docs/plans/P8.md`'s D51 for why this is not a fifth arm of `op.run`'s union. */
export type RemoteOpKind = 'fetch' | 'push' | 'pull' | 'forcePush' | 'deleteRemoteBranch';

/** One ref an operation moved, for the UI's "what happened" summary. */
export interface RefUpdate {
  readonly ref: string;
  readonly from: string | null; // null = created
  readonly to: string | null; // null = deleted
  readonly forced: boolean;
}

/** P9's autostash seam, empty at P8 — mirrors `CheckoutPreflight.routes`'s own precedent. */
export type PullRoute = 'stashAndCarry';

export type PullBlocker = 'dirtyNonFastForward';

export interface PullPreflight {
  readonly strategy: PullStrategy;
  readonly source: PullStrategySource;
  /** `true` only when `strategy` is `rebase`, `source` is `branchConfig` or `pullConfig`, and the
   *  key that won the ladder holds `merges`/`m`. `false` otherwise, including every
   *  `explicit`/`setting`/`default` source. */
  readonly rebaseMerges: boolean;
  readonly upstream: string | null;
  readonly ahead: number;
  readonly behind: number;
  readonly dirty: boolean;
  /** Empty at P8 — see `PullRoute`'s own doc comment. */
  readonly routes: readonly PullRoute[];
  readonly blockers: readonly PullBlocker[];
}

export interface PushPreflight {
  readonly upstream: string | null;
  readonly wouldSetUpstream: boolean;
  readonly ahead: number;
  readonly behind: number;
  /** The remote-tracking sha the lease will be checked against — read here and shown in the
   *  dialog, so "you are about to overwrite <sha>" is a fact, not a guess. `null` when the
   *  remote-tracking ref does not exist yet (nothing to overwrite). */
  readonly remoteTip: string | null;
  /** The matched protected pattern, or `null` — never a bare boolean (D52). */
  readonly protectedBy: string | null;
  readonly fastForward: boolean;
  /** The branch a force-push lands on: the resolved UPSTREAM remote branch name, which
   *  can differ from the LOCAL branch name a stacked/renamed branch tracks. `ForcePushDialog.vue`
   *  must gate its typed-confirmation on this name, not the local one, to ever match what the
   *  server's own `ConfirmToken` check requires. */
  readonly resolvedBranch: string;
}

/** `remote.run`'s params — one request key for all five `RemoteOpKind`s. `confirmToken`
 *  is present only for a protected-branch force-push/delete: the typed branch name, checked
 *  server-side against Kira Space's own server-owned `protectedBranches` setting (superseding
 *  the `kiraSpace.protectedBranches` — moved out of per-window settings because two windows
 *  disagreeing about it is a safety issue, not a preference) — never trusted from the
 *  UI alone. */
export interface RemoteOpParams {
  readonly repoId: string;
  readonly kind: RemoteOpKind;
  readonly remote: string;
  readonly branch: string | undefined;
  readonly setUpstream: boolean;
  readonly prune: boolean;
  readonly pruneTags: boolean;
  readonly strategy: PullStrategy | undefined;
  /** `pull` + `strategy: "rebase"` only. `true` runs `git rebase --rebase-merges`; `false`/
   *  `undefined` runs a plain, linearizing `git rebase`. `undefined` for every other kind. */
  readonly rebaseMerges: boolean | undefined;
  /** `forcePush` only: the PushPreflight.remoteTip value the confirmation dialog showed the
   *  user, or null when the dialog showed "nothing to overwrite". The server re-reads the
   *  remote-tracking ref immediately before spawning and compares against this value, failing
   *  with LeaseViolation on a mismatch even when git's own bare --force-with-lease
   *  --force-if-includes would not itself object (a background auto-fetch can silently satisfy
   *  git's own lease between dialog-open and spawn, without the user — still looking at the
   *  dialog's now-stale remoteTip — ever finding out).
   *  undefined for every other kind. */
  readonly expectedRemoteTip: string | null | undefined;
  /** forcePush only: true selects plain --force; false/undefined selects the default
   *  lease-based --force-with-lease --force-if-includes (D48). undefined for every other kind. */
  readonly plainForce: boolean | undefined;
  readonly confirmToken: string | undefined;
}

export interface RemoteOpResult {
  readonly ok: boolean;
  readonly error:
    | {
        readonly kind: OpErrorKind;
        readonly message: string;
        /** HookRejected only: the hook's own remote:-prefixed output, prefix stripped. undefined
         *  for every other kind, and for a HookRejected with no such lines. */
        readonly remoteMessage: string | undefined;
      }
    | undefined;
  readonly updates: readonly RefUpdate[];
  readonly head: HeadState;
  readonly inProgress: InProgressOperation | null;
}

/** `remote.progress`'s event payload — one op's live stderr, parsed (`git/src/progress.ts`). */
export interface RemoteProgress {
  readonly repoId: string;
  readonly phase: string;
  readonly percent: number | undefined;
  readonly done: number | undefined;
  readonly total: number | undefined;
  readonly remote: boolean;
}

export type OpRequest =
  | {
      readonly kind: 'checkout';
      readonly target: string;
      readonly mode: 'switch' | 'detach';
      /** §7.5's "discard" route: `git switch --discard-changes`. Cannot clear an untracked
       *  block (probe P9) — the UI never offers it for one. */
      readonly discardLocalChanges: boolean;
      /** Prepend a whole-tree `stash push [-u]` tagged with the CURRENT branch, so the
       *  switch cannot be blocked by a dirty tree. Never popped back — the entry stays in the
       *  stash list; cross-branch apply is the deliberate recovery path. Mutually exclusive with
       *  `discardLocalChanges` — the server refuses both at once rather than guessing. */
      readonly autoStash: boolean;
    }
  | {
      readonly kind: 'branchCreate';
      readonly name: string;
      readonly startPoint: string;
      readonly checkout: boolean;
      readonly track: string | undefined;
    }
  | { readonly kind: 'branchDelete'; readonly name: string; readonly force: boolean }
  | { readonly kind: 'branchRename'; readonly from: string; readonly to: string }
  | {
      readonly kind: 'tagCreate';
      readonly name: string;
      readonly target: string;
      /** Present ⇒ annotated (`-a -m`). Absent ⇒ lightweight. On `force`, an annotated tag MUST
       *  re-supply this or `-f` downgrades it to lightweight (probe P3). */
      readonly message: string | undefined;
      readonly force: boolean;
    }
  | { readonly kind: 'tagDelete'; readonly name: string }
  | { readonly kind: 'tagPush'; readonly remote: string; readonly names: readonly string[] | 'all' }
  | { readonly kind: 'tagDeleteRemote'; readonly remote: string; readonly name: string }
  | {
      readonly kind: 'revert';
      readonly shas: readonly string[];
      readonly mainline: number | undefined;
      readonly noCommit: boolean;
    }
  | { readonly kind: 'opContinue' }
  | { readonly kind: 'opAbort' }
  | {
      readonly kind: 'stashPush';
      /** Undefined → git's own `WIP on <branch>: …`. */
      readonly message: string | undefined;
      readonly includeUntracked: boolean;
      readonly keepIndex: boolean;
      /** Joined after a literal `--`. Empty ⇒ the whole worktree. */
      readonly paths: readonly string[];
    }
  /** `apply` accepts a raw sha, so this addresses by sha and needs no index guard (probe 8). */
  | { readonly kind: 'stashApply'; readonly sha: string; readonly restoreIndex: boolean }
  /** `pop` REFUSES a raw sha, so the argv must use `stash@{index}` and the service verifies
   *  `rev-parse stash@{index} === sha` immediately before writing (probe 8). */
  | {
      readonly kind: 'stashPop';
      readonly sha: string;
      readonly index: number;
      readonly restoreIndex: boolean;
    }
  | { readonly kind: 'stashDrop'; readonly sha: string; readonly index: number }
  /** Addressed by `stash@{index}`: given a raw sha, `stash branch` applies but silently never
   *  drops. `scope: 'global'` skips the stack-position verification entirely
   *  (there is no position) and applies by sha instead — `index` is then ignored server-side
   *  (still present on the wire for shape uniformity; the client sends the entry's own `-1`
   *  sentinel). */
  | {
      readonly kind: 'stashBranch';
      readonly branch: string;
      readonly sha: string;
      readonly index: number;
      readonly scope?: 'stack' | 'global';
    }
  | {
      readonly kind: 'reset';
      readonly mode: ResetMode;
      /** A sha, always — resolved by the caller from the graph row. */
      readonly target: string;
      /** §7.7's typed confirmation, required (and re-checked host-side) exactly when
       *  `mode === "hard"` and the pre-flight's `destroys` was non-empty. */
      readonly confirmToken: string | undefined;
    }
  | {
      readonly kind: 'cherryPick';
      readonly sha: string;
      /** Required for a merge commit — probe 8: `is a merge but no -m option was given`. */
      readonly mainline: number | undefined;
      readonly noCommit: boolean;
    }
  /** §7.11's third sequencer verb, for cherry-pick and revert only (probe 6). */
  | { readonly kind: 'opSkip' }
  /** Three explicit creation modes, always with an explicit commit-ish — bare DWIM
   *  (`worktree add <path>` with no commit-ish, which silently names a branch after the path's own
   *  basename) is never relied on. `branch`/`startPoint` are populated per mode:
   *  `existingBranch` needs only `branch` (itself the commit-ish); `newBranch` needs both;
   *  `detach` needs only `startPoint`. Deliberately no `force` — its only real use (forcing a
   *  branch already checked out elsewhere onto a worktree) enables exactly the hazard
   *  `branchCheckedOutElsewhere` exists to prevent. */
  | {
      readonly kind: 'worktreeAdd';
      readonly path: string;
      readonly mode: 'existingBranch' | 'newBranch' | 'detach';
      readonly branch: string | undefined;
      readonly startPoint: string | undefined;
    }
  /** `force`/`confirmToken` are BOTH re-derived and re-checked host-side immediately
   *  before the write, never trusted as the client's own claim (the fail-safe-over-fail-open
   *  principle for this destructive path) — `force` becomes true only when the server's own fresh
   *  preflight verdict is `"dirty"`, and `confirmToken` must then equal that same fresh preflight's
   *  own `confirmToken` (the worktree's basename) exactly. */
  | {
      readonly kind: 'worktreeRemove';
      readonly path: string;
      readonly force: boolean;
      readonly confirmToken: string | undefined;
    }
  /** G26 D10: sets or clears a branch's stack parent — always exactly two `git config --local`
   *  writes (D2), always exit 0. `parent: undefined` removes `branch` from its stack (D2 writes
   *  `""` to both `kirastack*` keys server-side, never `git config --unset`, per F15/probe P5). */
  | { readonly kind: 'stackSet'; readonly branch: string; readonly parent: string | undefined }
  /** Saves an entry into the global stash bucket — ALWAYS COPIES, never drops its
   *  source. `sha: undefined` snapshots the CURRENT working tree (tracked changes only — `git
   *  stash create` cannot include untracked files); otherwise the sha of an existing
   *  stash-stack OR global entry to promote, resolved across BOTH buckets server-side. */
  | { readonly kind: 'globalStashSave'; readonly label: string; readonly sha: string | undefined }
  /** G28 D11: removes one entry from the global bucket — the cleanest undo in the whole table
   *  (an exact single `update-ref <ref> <sha>` replay). */
  | { readonly kind: 'globalStashRemove'; readonly sha: string };

/** Every `WireError.code` the git RPC emits: Go (`internal/ipcerr`, `gitrpc` `mapGitError`,
 *  `rpcstream`) and this package's server. `E_GIT_*` is `gitclient`'s error kind in
 *  SCREAMING_SNAKE_CASE. A locally thrown JS error crosses as its class name instead. Keep in
 *  step with `apps/kira-space/internal/gitrpc/contract.go`. */
export type WireErrorCode =
  | 'E_BAD_REQUEST'
  | 'E_INTERNAL'
  | 'E_UNKNOWN_METHOD'
  | 'E_READ_ONLY'
  | 'E_TOO_LARGE'
  | 'E_FRAME_TOO_LARGE'
  | 'E_GIT_NOT_A_REPOSITORY'
  | 'E_GIT_PERMISSION_DENIED'
  | 'E_GIT_CANCELLED'
  | 'E_GIT_TIMEOUT'
  | 'E_GIT_UNKNOWN'
  | 'E_GIT_UNAVAILABLE';

export type OpErrorKind =
  | 'AuthFailed'
  | 'NonFastForward'
  | 'Conflict'
  | 'DirtyWorktree'
  | 'UntrackedWouldBeOverwritten'
  | 'LockHeld'
  | 'NotFound'
  | 'AlreadyExists'
  | 'NotFullyMerged'
  | 'WorktreeConflict'
  | 'OperationInProgress'
  | 'RemoteRefMissing'
  | 'HookRejected'
  /** P8: `git`'s own `(stale info)` — the bare `--force-with-lease` lease was violated because
   *  the remote moved and we never fetched it. Probe 1, rows 1-2. */
  | 'LeaseViolation'
  /** P8: `git`'s own `(remote ref updated since checkout)` — `--force-if-includes` caught a
   *  remote move we DID fetch but have not integrated. Probe 1, row 3. Kept distinct from
   *  `LeaseViolation`: the remedies differ (fetch-and-look vs. you-already-saw-this). */
  | 'RemoteRefUpdated'
  /** P8: a transport-level failure (`Could not resolve host`, `Connection refused/timed out`) —
   *  never git's own decision, always the network. */
  | 'NetworkFailed'
  /** P8: the remote itself does not exist (`Repository not found`, "does not appear to be a
   *  git repository"). */
  | 'RemoteNotFound'
  /** P8: neither git says this nor could it — the confirmation token `remote.run` requires for
   *  a protected-branch force-push/delete was absent or did not match (D52). */
  | 'ProtectedBranch'
  /** P8: a remote op was cancelled mid-flight (D50) — never a git-reported failure either. */
  | 'Cancelled'
  /** A pop/apply merged with conflicts. Deliberately NOT detected from a stderr pattern — a
   *  conflicting pop writes to stdout and leaves stderr empty — the service classifies
   *  it from `exitCode !== 0` plus a post-op status read-back finding unmerged paths. The stash
   *  is ALWAYS kept; the message says so. */
  | 'StashConflict'
  /** P9: `apply --index`/`pop --index` onto an already-conflicted index —
   *  `error: conflicts in index. Try without --index.` (probe 10). Distinct from `StashConflict`:
   *  git names its own remedy exactly (retry the same op with `restoreIndex: false`). */
  | 'StashIndexConflict'
  /** P9: untracked files in the way of restoring the stash's own untracked half — the ONE
   *  non-atomic failure in the phase: the tracked half was already applied and the stash was
   *  kept (probe 3). */
  | 'StashUntrackedCollision'
  /** P10: a cherry-pick/revert whose change is already present — `CHERRY_PICK_HEAD`/
   *  `REVERT_HEAD` is set, the worktree is clean and NO paths are unmerged (probe 6). Detected by
   *  exit code + read-back, never by pattern: the whole message goes to stdout. The remedy is
   *  `--skip`. */
  | 'EmptyCherryPick'
  /** P10: the typed confirmation a destructive `reset --hard` requires was absent or did not
   *  match, re-checked host-side. Deliberately NOT `ProtectedBranch` (D52's pattern is reused;
   *  its kind is not). */
  | 'ConfirmationRequired'
  /** P10, probe 8: `commit <sha> is a merge but no -m option was given.` */
  | 'MainlineRequired'
  /** `fatal: cannot remove a locked working tree, lock reason: <reason>` —
   *  the ONE new member here (exactly one new `OpErrorKind`).
   *  Kept distinct from `LockHeld` (which means "another git process holds index.lock" — an
   *  entirely different remedy: `LockHeld` says wait/retry, `WorktreeLocked` says unlock the
   *  worktree first). */
  | 'WorktreeLocked'
  /** G26 D5/D10: this phase's own ONE new `OpErrorKind` — produced EXCLUSIVELY by `stackSet`
   *  (D10's cycle check), never by rebase itself (D5: rebase's own two new stderr patterns both
   *  map onto the EXISTING `DirtyWorktree`/`NotFound` kinds above). */
  | 'StackCycle'
  /** G28 D10: `globalStashSave` from the CURRENT WORKING TREE with nothing dirty — `git stash
   *  create` answers exit 0 with empty output on a clean tree (probe P4), which becomes this named
   *  refusal rather than a silent no-op write. This phase's own ONE new `OpErrorKind`. */
  | 'NothingToStash'
  /** `remote.run`'s pull integrate phase (merge/rebase) re-checks HEAD is still the branch the pull
   *  was started for, immediately before that write — a fetch can take arbitrary wall-clock time,
   *  during which another window/terminal can check out a different branch, and without this the
   *  merge/rebase would silently land on whatever is checked out now instead. */
  | 'BranchChanged'
  | 'Unknown';

export interface UndoSlotSnapshot {
  readonly id: string;
  /** "Deleted branch feature" — §7.12's "labelled with what it will undo". */
  readonly label: string;
  /** "was d657c6e" — §7.12's "captured recovery sha is shown alongside the button, so the user
   *  can recover manually even after the slot is cleared". */
  readonly recoverySha: string;
  readonly createdAt: number;
}

export interface OpResult {
  readonly ok: boolean;
  readonly error: { readonly kind: OpErrorKind; readonly message: string } | undefined;
  /** The slot AFTER this op: a new record for an undoable op, `null` for any other (which clears
   *  it — §7.12's "performing another operation clears the undo slot"). */
  readonly undo: UndoSlotSnapshot | null;
  /** Read back after the op, success or failure — the reconcile step, closing the window before
   *  the watcher's debounce. */
  readonly head: HeadState;
  readonly inProgress: InProgressOperation | null;
}

// ---------------------------------------------------------------------------------------
// Discriminated unions the UI renders explicitly rather than infers.
// ---------------------------------------------------------------------------------------

export type GitStatus =
  | { readonly kind: 'ok'; readonly path: string; readonly version: string }
  | { readonly kind: 'notFound'; readonly probed: readonly string[] }
  | {
      readonly kind: 'tooOld';
      readonly path: string;
      readonly detected: string;
      readonly required: string;
      readonly settingId: string;
    }
  | { readonly kind: 'unusable'; readonly path: string; readonly reason: string };

// The Go server's real `app.init` result — `ResultOf<'app.init'>` is the client-visible shape,
// which drops `serverVersion`. Mirrors internal/gitrpc/wire.go's AppInitResult field for field.
export interface ServerAppInitResult {
  readonly contractVersion: number;
  readonly serverVersion: string;
  readonly git: GitStatus;
  /** Kira Space's app-wide `appearance.dateFormat`. */
  readonly dateFormat: 'relative' | 'absolute';
}

// ---------------------------------------------------------------------------------------
// Change-request links — a structural copy of internal/githost.Status/ChangeRequest, kept honest
// by hand (this repo carries no wireConformance.spec.ts — see the commit message for why).
// ---------------------------------------------------------------------------------------

/** D5's own four-kind actionability union: "ok" ⇒ the host's client is installed, authenticated and
 *  answered; "notFound" ⇒ it could not be used at all; "unauthenticated" ⇒ it works but the host does
 *  not know who you are; "forbidden" ⇒ it works, the host knows who you are, and refused or could not
 *  answer (rate limit, SSO, scope, a 404, or a 5xx — `reason` carries which). */
export interface HostStatus {
  readonly kind: 'ok' | 'notFound' | 'unauthenticated' | 'forbidden';
  readonly path?: string;
  readonly version?: string;
  readonly host?: string;
  readonly account?: string;
  readonly reason?: string;
}

/** One change request (a pull request or merge request), trimmed to exactly the fields D4 names as
 *  read. `state` is derived server-side (D4): the host's own state is only ever open or closed —
 *  "merged" and "draft" are computed before this record is ever built. */
export interface ChangeRequestRecord {
  readonly number: number;
  readonly title: string;
  /** P79 finding 6: never hand this straight to a host/renderer as a clickable link — go through
   *  `changeRequest.openExternal` instead. */
  readonly url: string;
  readonly state: 'open' | 'draft' | 'merged' | 'closed';
  readonly headRef: string;
  readonly headSha: string;
  readonly baseRef: string;
  /** Unix milliseconds. */
  readonly updatedAt: number;
  /** Set by the persisted index only. */
  readonly draft?: boolean;
  readonly reviewDecision?: 'APPROVED' | 'CHANGES_REQUESTED' | 'REVIEW_REQUIRED';
  readonly checksState?: 'SUCCESS' | 'FAILURE' | 'PENDING' | 'ERROR' | 'EXPECTED';
}

/** One head branch's newest cached change request. */
export interface BranchChangeRequest {
  readonly name: string;
  readonly changeRequest: ChangeRequestRecord;
}

/** branch.changeRequestIndex's result: the persisted change request index of a repository, served
 *  from SQLite. `complete` means every change request was ingested, so a branch not listed has
 *  none; incomplete lists what is cached so far. Only branches in refs, plus open requests. */
export interface ChangeRequestIndex {
  readonly complete: boolean;
  /** Last sync, Unix milliseconds; 0 when never synced. */
  readonly syncedAt: number;
  readonly branches: readonly BranchChangeRequest[];
}

/** commit.resolveChangeRequests / branch.resolveChangeRequest's own shared result shape.
 *  `"disabled"` covers both `kiraSpace.gitHost.provider === 'off'` and
 *  "no remote on a known code host" — the grid/badge/detail pane render nothing for either, so the
 *  two need no further distinction on the wire. */
/** What the UI needs to name one code host provider (`githost.Info`). */
export interface GitHostInfo {
  readonly id: string;
  /** "GitHub". */
  readonly label: string;
  /** "Pull request" — sentence start. */
  readonly changeRequest: string;
  /** "pull request" — mid-sentence. */
  readonly changeRequestNoun: string;
  /** "PR". */
  readonly changeRequestShort: string;
  /** "GitHub CLI (gh)"; empty when the host needs no CLI. */
  readonly cli: string;
}

export type ChangeRequestLookup =
  | { readonly kind: 'ok'; readonly changeRequests: readonly ChangeRequestRecord[] }
  | { readonly kind: 'disabled' }
  | { readonly kind: 'unavailable'; readonly status: HostStatus };

export type ChangeRequestReviewEvent = 'approve' | 'comment' | 'requestChanges';

/** changeRequest.reviewState's result. `disabled`: host off, no provider, or one that cannot review.
 *  `none`: the branch has no change request. */
export type ChangeRequestReviewState =
  | { readonly kind: 'disabled' }
  | { readonly kind: 'none' }
  | { readonly kind: 'unavailable'; readonly status: HostStatus }
  | {
      readonly kind: 'ok';
      readonly host: GitHostInfo;
      readonly number: number;
      readonly title: string;
      readonly state: ChangeRequestRecord['state'];
      /** The head the host shows now; a submitted review applies to it. */
      readonly headSha: string;
      /** The local branch tip; '' when the branch is gone. */
      readonly localHeadSha: string;
      readonly author: string;
      readonly viewer: string;
      /** The viewer opened it: the host refuses approve and request changes. */
      readonly own: boolean;
    };

export type ChangeRequestPostResult =
  | { readonly kind: 'ok' }
  | { readonly kind: 'disabled' }
  | { readonly kind: 'unavailable'; readonly status: HostStatus };

/** changeRequest.lineComment's result. `headMoved`: the host head is not `headSha`; nothing posted. */
export type ChangeRequestLineCommentResult =
  | ChangeRequestPostResult
  | { readonly kind: 'headMoved'; readonly headSha: string };

export type RepoOpenResult =
  | { readonly kind: 'ok'; readonly repo: RepoSummary }
  | { readonly kind: 'notARepository'; readonly path: string }
  | { readonly kind: 'gitUnavailable'; readonly git: GitStatus };

// ---------------------------------------------------------------------------------------
// Branch review. Structural copies of `core`'s `model/review.ts`, kept honest by
// `tests/unit/ipc/wireConformance.spec.ts` rather than an import.
// ---------------------------------------------------------------------------------------

/** A `<base>..<branch>` two-dot range (§6.8/D30). Both are short ref names as the UI shows them
 *  (`main`, `origin/develop`, `feature-x`) — never full refnames, never object ids: the range is
 *  a question about two *refs*, and a sha in `base` would make "how it was resolved" unanswerable. */
export interface CommitRange {
  readonly base: string;
  readonly branch: string;
}

export type BaseResolutionReason =
  /** §6.8 step 1: the branch's upstream, and it names a *different* branch. */
  | 'upstream'
  /** §6.8 step 2: `origin/HEAD`, or the first existing `review.baseCandidates` member. */
  | 'defaultBranch'
  /** The user picked it from the header picker — resolution was skipped entirely. */
  | 'override'
  /** §6.8 step 3: nothing detected. `base` is null and no walk is opened. */
  | 'none';

/** What a `<base>..<branch>` comparison *is*, decided before the first row is painted — see
 *  `docs/plans/P7.md`'s "Base resolution has four outcomes" for why this cannot be inferred from
 *  an empty walk. */
export type ReviewRangeState =
  | { readonly kind: 'ready'; readonly commitCount: number }
  /** `rev-list --count` is 0: fully merged, or the branch IS the base. §6.8's "nothing to
   *  review — `<branch>` adds no commits to `<base>`", which the UI renders naming both refs. */
  | { readonly kind: 'empty' }
  /** `merge-base` found nothing: `<base>..<branch>` would list the branch's entire history as
   *  though it were all new (§6.8). */
  | { readonly kind: 'unrelated' }
  /** No base at all. Only ever paired with `reason: "none"`. */
  | { readonly kind: 'ask' };

/** One entry in the header picker's shortlist. The picker's *full* branch list comes from
 *  `refs.list`, which the review view already loads — this is only the handful worth ranking to
 *  the top, each with the reason it is offered. */
export interface BaseCandidate {
  readonly ref: string;
  readonly kind: RefKind;
  /** Never "override" — a candidate is a thing we detected, not a thing the user chose. */
  readonly reason: Exclude<BaseResolutionReason, 'override' | 'none'>;
}

export interface BaseResolution {
  readonly branch: string;
  /** `null` iff `reason === "none"`. */
  readonly base: string | null;
  readonly reason: BaseResolutionReason;
  readonly range: ReviewRangeState;
  readonly candidates: readonly BaseCandidate[];
}

// ---------------------------------------------------------------------------------------
// Incremental review.
// review.db's own per-file "last reviewed" state, the three-tier delta selection, and partial
// review ranges — all new to v1.3, no upstream equivalent.
// ---------------------------------------------------------------------------------------

/** 1-based, inclusive, both ends. Always new-side (branch-tip) line numbers on the wire — stored
 *  ranges live in the snapshot's own coordinates server-side, but are always projected forward
 *  before crossing here (G11 D10). */
export interface LineRange {
  readonly start: number;
  readonly end: number;
}

export interface ReviewFileStatus {
  readonly kind: 'none' | 'partial' | 'full';
  /** The branch tip's content for this path differs from the snapshot the state was recorded
   *  against. Always `false` for `'none'`. */
  readonly changedSinceReview: boolean;
  readonly reviewedAt: number | undefined; // unix millis
  readonly reviewedAtSha: string | undefined;
}

export interface ReviewFileEntry {
  readonly change: FileChange;
  readonly review: ReviewFileStatus;
}

/** Which mechanism answered "what changed since the snapshot" (G11 D7). Reported even in
 *  `mode: "range"`, because it is also what produced `reviewedRanges`' own projection. */
export type ReviewDeltaSource =
  /** Never reviewed — the delta IS the whole range diff. */
  | 'noSnapshot'
  /** Blob-oid equality: nothing changed, no diff ran. */
  | 'unchanged'
  /** The snapshot commit is still an ancestor; an ordinary git diff ran. */
  | 'fast'
  /** History was rewritten; the stored blob was diffed with `diff --no-index`. */
  | 'slow'
  /** Reviewed, but no content was stored and the sha no longer resolves. */
  | 'snapshotUnavailable';

export type ReviewDiffMode = 'range' | 'sinceReview';

// ---------------------------------------------------------------------------------------
// Inline AI review comments. A flat, non-threaded annotation table anchored to the revision the
// reviewer was reading and projected forward on read — no upstream equivalent.
// ---------------------------------------------------------------------------------------

/** How a stored comment's line range relates to the revision it was asked about (D7). */
export type CommentAnchor =
  /** The file is byte-identical to when the comment was written; the lines are current. */
  | 'exact'
  /** The lines moved and were mapped forward through a real diff; the lines are current. */
  | 'projected'
  /** The commented lines no longer exist; `range` is as of `anchorSha`. */
  | 'removed'
  /** History was rewritten and no mapping exists; `range` is as of `anchorSha`. */
  | 'stale'
  /** Written against uncommitted worktree content; `range` is as of that content. */
  | 'uncommitted';

export type ReviewCommentOrigin = 'commit' | 'working';

export interface ReviewComment {
  readonly id: number;
  readonly path: string;
  /** In the requested revision's coordinates for `exact`/`projected`, in `anchorSha`'s own for
   *  `removed`/`stale` — which is what `anchor` is for. */
  readonly range: LineRange;
  readonly body: string;
  readonly anchor: CommentAnchor;
  readonly anchorSha: string;
  readonly createdAt: number; // unix millis
  /** P286: a change request or a question to answer. */
  readonly kind: ReviewCommentKind;
  readonly state: ReviewCommentState;
  /** An agent's answer; '' until one replies. */
  readonly reply: string;
  readonly repliedAt: number | null; // unix millis
  /** Last send to an agent, unix millis. */
  readonly sentAt: number | null;
  /** P418: `working` = written on uncommitted worktree content. */
  readonly origin: ReviewCommentOrigin;
}

export type ReviewCommentKind = 'change' | 'question';
export type ReviewCommentState = 'open' | 'addressed';

// ---------------------------------------------------------------------------------------
// Search. Structural copy of `packages/core`'s `search/query.ts` (minus `scope`: the
// tail scan is commits-only, and Refs/Both are resolved entirely client-side against
// `RefsState`, no RPC), kept honest by `tests/unit/ipc/wireConformance.spec.ts`.
// ---------------------------------------------------------------------------------------

export interface SearchQueryParams {
  readonly text: string;
  readonly caseSensitive: boolean;
  readonly wholeWord: boolean;
  readonly regex: boolean;
}

export type SearchMatchField =
  | 'subject'
  | 'body'
  | 'authorName'
  | 'authorEmail'
  | 'committerName'
  | 'committerEmail'
  | 'sha';

/** Enough to render a dropdown row without a second round trip, and nothing more — the scan
 *  reads gigabytes of records (probe 5) and none of it crosses the wire beyond these capped
 *  hits. */
export interface CommitSearchHit {
  readonly sha: string;
  readonly subject: string;
  readonly authorName: string;
  readonly authorEmail: string;
  readonly authorTime: number;
  readonly fields: readonly SearchMatchField[];
}

export type SearchRunResult =
  | {
      readonly kind: 'ok';
      /** Walk order (probe 11), capped at `limit`. */
      readonly hits: readonly CommitSearchHit[];
      /** EXACT, counted over every commit scanned — not `hits.length`. */
      readonly total: number;
      readonly truncated: boolean;
      readonly scanned: number;
      /** `false` ⇒ the host-side time box fired before git's own end (hard part 4). */
      readonly complete: boolean;
    }
  /** Never thrown (probe 4): a pattern the UI would not send still comes back as data. */
  | { readonly kind: 'invalidPattern'; readonly message: string }
  /** The pattern is valid JavaScript (the client compiled it before sending) but uses
   *  syntax the Go tail scan's RE2 engine cannot run — lookahead, lookbehind, or a backreference.
   *  The loaded-commit half of the search is unaffected and still complete; only the not-yet-
   *  walked tail (and therefore any body-only match) is missing. */
  | { readonly kind: 'unsupportedPattern'; readonly message: string };

/** G10 D9: the host's route into an already-mounted git-ui — every mutating command that isn't a
 *  fresh host-side implementation of `OpsState`'s own logic funnels through this one event, so a
 *  command is an entry point, never a second implementation. */
export type UiActionKind =
  | 'openBranchPicker'
  /** The same panel, opened on its Tags / Stashes / Worktrees / Stacks tab. Before this, all
   *  twelve of `commands.ts`'s `MUTATING_COMMANDS` entries for those rows funnelled into
   *  `openBranchPicker` alone, which opens a five-tab panel on whichever tab it was last left on
   *  and no closer to the row the command names. One member per tab, the same shape used
   *  for `resetSelected`/`cherryPickSelected`. */
  | 'openTagPicker'
  | 'openStashPicker'
  | 'openWorktreePicker'
  | 'openStackPicker'
  | 'createBranch'
  | 'createTag'
  | 'revertSelected'
  | 'continueOperation'
  | 'abortOperation'
  | 'skipCommit'
  | 'undo'
  | 'fetch'
  | 'pull'
  | 'push'
  | 'forcePush'
  | 'cancelRemoteOperation'
  | 'refresh'
  /** G11 D17: toggles the file currently open in the review sidebar's Files pane. Not a
   *  MUTATING_COMMANDS member (it maps to no OpRequest/RemoteOpParams kind) — commands.ts's own
   *  OTHER_COMMANDS carries it instead. */
  | 'toggleFileReviewed'
  /** G13 D19: the palette's own route to the Comments pane's copy-for-AI action. */
  | 'copyReviewComments'
  /** G13 D19: host -> the Comments pane, emitted after an editor-side comment add/delete so
   *  the sidebar's list updates without the user switching panes. */
  | 'refreshReviewComments'
  /** G14 D10: "Open in graph" from the review diff toolbar — reveals and selects a commit named
   *  by `ui.action`'s optional `target`. Host -> webview only. */
  | 'revealCommit'
  /** The palette's own route to `StashDialog.vue`'s create mode — the same assignment the
   *  toolbar's own "Stash changes…" button already makes (`App.vue`'s `@stash-changes` handler),
   *  so this is a second entry point into the same dialog, never a second implementation. The
   *  other four stash commands (`stashApply`/`stashPop`/`stashDrop`/`stashBranch`) reuse
   *  `openStashPicker` instead — no new member for those (`BranchPicker.vue`'s own Stashes tab
   *  already has row-level Apply/Pop/Drop/Branch actions). */
  | 'stashChanges'
  /** The palette's own route into `ResetDialog.vue` — the same assignment the graph row
   *  menu's own "Reset to This Commit…" entry already makes (`App.vue`'s
   *  `resetToThisCommit`/`runReset` handler), so this is a second entry point into the same
   *  dialog, never a second implementation. */
  | 'resetSelected'
  /** G22 D10: the palette's own route into `CherryPickDialog.vue` — the same assignment the graph
   *  row menu's own "Cherry-pick This Commit…" entry already makes (`App.vue`'s
   *  `cherryPickThisCommit`/`runCherryPick` handler). */
  | 'cherryPickSelected'
  /** The palette's own route into `WorktreeDialog.vue`'s create mode — the same assignment
   *  the branch picker's own worktree section "Create Worktree…" button already makes, so this is
   *  a second entry point into the same dialog, never a second implementation. The other worktree
   *  actions (switch, open in new window, remove) reuse `openWorktreePicker` instead, the same
   *  convention the five stash commands already established — `BranchPicker.vue`'s own Worktrees
   *  tab already has row-level actions for all three. */
  | 'createWorktree'
  /** The palette's own route to "Restack this stack" — resolves the CURRENT branch's own
   *  stack client-side (`stackListModel`) and calls the same `runRestack` the row menu's
   *  "Restack this stack" entry and `StackList.vue`'s own header button already call, never a
   *  second implementation. */
  | 'restackStack'
  /** The `alt+up`/`alt+down` stack navigation — resolves the current branch's parent
   *  (`checkoutStackParent`) or child (`checkoutStackChild`) client-side and calls the existing
   *  `opsState.runCheckout`, exactly the same "resolve a target, call the existing op" shape
   *  `revertSelected`/`resetSelected`/`cherryPickSelected` already established. */
  | 'checkoutStackParent'
  | 'checkoutStackChild'
  /** The palette's own route into `StashDialog.vue`'s fourth mode, save-to-global-stash —
   *  the same assignment the global stash section's own "Save to global stash…" header button
   *  already makes, so this is a second entry point into the same dialog, never a second
   *  implementation. `globalStashRemove`/`stashBranch`-for-a-global-entry reuse `openStashPicker`
   *  instead, the same convention the five ordinary stash commands already established. */
  | 'saveGlobalStash'
  /** G-UX D9: the palette's own route to toggling the graph panel's search row — the same
   *  assignment the in-webview `/`/`Ctrl+F` shortcuts already make. It maps to no
   *  OpRequest/RemoteOpParams kind. */
  | 'toggleSearch';

// ---------------------------------------------------------------------------------------
// The contract.
// ---------------------------------------------------------------------------------------

export type Contract = {
  requests: {
    'app.init': {
      params: Record<string, never>;
      result: {
        contractVersion: number;
        git: GitStatus;
        /** Kira Space's app-wide date format; the UI falls back to its own persisted preference
         *  when absent. */
        dateFormat?: 'relative' | 'absolute';
      };
    };
    'repo.list': {
      params: Record<string, never>;
      result: { candidates: readonly RepoCandidate[]; activeRepoId: string | null };
    };
    'repo.open': {
      params: { path: string };
      result: RepoOpenResult;
    };
    'repo.close': {
      params: { repoId: string };
      result: Record<string, never>;
    };
    'graph.status': {
      /** `range` present ⇒ the review walk's own counters (P7 W5), not the panel's. */
      params: { repoId: string; range?: CommitRange };
      result: { loaded: number; remaining: number; exhausted: boolean };
    };
    'graph.loadMore': {
      /** `range` present ⇒ pages the review walk instead of the panel's own (P7 W5). Scope and
       *  page size come from the repository's stored `kiraSpace.graph.*` settings. */
      params: { repoId: string; pages?: number; range?: CommitRange };
      result: { started: boolean };
    };
    'graph.refresh': {
      params: { repoId: string };
      result: { restarted: boolean };
    };
    /** P173: the client detected an unrecoverable graph stream (a corrupted chunk). Records one
     *  Operations-log entry server-side; the message is composed there, never taken from the
     *  client. Writes nothing to git. */
    'graph.reportFailure': {
      params: { repoId: string; reason: 'corrupted' };
      result: Record<string, never>;
    };
    /**
     * Called once when the review view targets a branch (no `base`), and again with
     * an explicit `base` each time the header picker overrides it — the `merge-base` and
     * range-count checks must run for a chosen base exactly as they do for a detected one, so
     * both paths land here rather than the override taking a shortcut through the candidate
     * list the first call returned.
     */
    'review.resolveBase': {
      params: {
        repoId: string;
        branch: string;
        base?: string;
      };
      result: BaseResolution;
    };
    /**
     * §6.8's entry point, from the *panel* webview: reveal the sidebar view on this branch. The
     * host reveals the view and pushes `review.target` to it. Returns nothing — the panel does not wait
     * on, and is not told about, what the review view then finds.
     */
    'review.open': {
      params: { repoId: string; branch: string };
      result: Record<string, never>;
    };
    /**
     * G11 D1/D6: the range's file list — the three-dot (merge-base) diff's own file set, each
     * joined against its stored review record (if any) via blob-oid equality (D7 tier 0) so a
     * per-file "has this changed since you reviewed it" answer never spawns a diff.
     */
    'review.files': {
      params: { repoId: string; branch: string; base: string };
      result: {
        readonly branchTip: string;
        readonly mergeBase: string;
        readonly files: readonly ReviewFileEntry[];
      };
    };
    /**
     * G11 D1/D7/D13: one file's delta since it was last reviewed, plus its projected reviewed
     * ranges — the delta selection runs in BOTH modes (reviewedRanges needs the projection either
     * way); `mode` decides only which patch becomes `body`.
     */
    'review.fileDiff': {
      params: {
        repoId: string;
        branch: string;
        base: string;
        path: string;
        mode: ReviewDiffMode;
      };
      result: {
        readonly path: string;
        readonly deltaSource: ReviewDeltaSource;
        readonly body: FileDiffBody;
        readonly reviewedRanges: readonly LineRange[];
        readonly lineCount: number;
        readonly reviewedAtSha: string | null;
      };
    };
    /**
     * Marks (or unmarks) a file, in whole or in part. No `base` — a write is a fact
     * about `(repo, branch, path)` only. `ranges` omitted (not merely empty) means the whole file.
     * Returns the resulting status so the file list updates from the response rather than
     * re-requesting `review.files` after every checkbox.
     */
    'review.mark': {
      params: {
        repoId: string;
        branch: string;
        path: string;
        reviewed: boolean;
        ranges?: readonly LineRange[];
      };
      result: { readonly review: ReviewFileStatus };
    };
    /**
     * Marks (or unmarks) whole files in one call: one branch-tip resolution, one result per distinct
     * path in first-seen order. Backs a folder checkbox.
     */
    'review.markMany': {
      params: { repoId: string; branch: string; paths: readonly string[]; reviewed: boolean };
      result: {
        readonly results: readonly { readonly path: string; readonly review: ReviewFileStatus }[];
      };
    };
    /**
     * The stored text of `path` as it was when last reviewed, read from review.db's own
     * compressed snapshot (no git object read, so it survives a rewritten or unreachable tip).
     * `absent` = no record or no stored snapshot; `reviewedAtSha` is the tip at that review.
     */
    'review.snapshot': {
      params: { repoId: string; branch: string; path: string };
      result: {
        readonly kind: 'none' | 'text' | 'binary' | 'tooLarge' | 'absent';
        readonly text: string | null;
        readonly reviewedAtSha: string | null;
      };
    };
    /** Anchors a new comment to `at` — the revision the caller says it was reading,
     *  required with no default (a default of "the tip" is exactly the silent mis-anchor).
     *  Returns the whole comment, id included, so the caller can render its thread from the
     *  response instead of re-listing. */
    'review.comment.add': {
      params: {
        repoId: string;
        branch: string;
        path: string;
        at: string;
        range: LineRange;
        body: string;
        /** P286: defaults to 'change'. */
        kind?: ReviewCommentKind;
      };
      result: { readonly comment: ReviewComment };
    };
    /** P286: reopens or addresses one comment. `changed` is false when the id matches nothing. */
    'review.comment.setState': {
      params: { repoId: string; branch: string; id: number; state: ReviewCommentState };
      result: { readonly changed: boolean };
    };
    /** P413: replaces body and kind; keeps range, anchor, state and reply; clears `sentAt` (the edited
     *  text was never sent). `changed` is false when the id matches nothing. */
    'review.comment.edit': {
      params: { repoId: string; branch: string; id: number; body: string; kind: ReviewCommentKind };
      result: { readonly changed: boolean };
    };
    /** G13 D11: `at` defaults to the branch tip when omitted — the resolved value is echoed back
     *  so a caller never has to guess which coordinates it is holding. */
    'review.comment.list': {
      params: { repoId: string; branch: string; at?: string };
      result: { readonly at: string; readonly comments: readonly ReviewComment[] };
    };
    /** G13 D11: idempotent and scoped — an id from another (repo, branch) session matches
     *  nothing, and a row already gone (another window deleted it) answers `false` rather than an
     *  error. */
    'review.comment.remove': {
      params: { repoId: string; branch: string; id: number };
      result: { readonly removed: boolean };
    };
    /** G13 D14: removes every comment for this (repo, branch) session and nothing else — never
     *  `review_file`/`review_range`, never the session row itself. Returns the removed count so
     *  the pane can announce it. */
    'review.comment.clear': {
      params: { repoId: string; branch: string };
      result: { readonly removed: number };
    };
    /** G13 D12: the AI-paste plain text, produced server-side by `gitreview.FormatComments` — `""`
     *  for a session with no comments. `at` defaults to the branch tip, same as list. */
    'review.comment.export': {
      params: { repoId: string; branch: string; at?: string };
      result: { readonly at: string; readonly text: string };
    };
    /**
     * The durable half of "back to branch selection" — a small, additive pair. Not commit or diff
     * data, only the identifiers needed to re-ask `setTarget`/`setBase`'s own question fresh
     * on the next resume (every resume re-runs `review.resolveBase` for real — there is no
     * cached resolution to go stale). Answered entirely host-side, exactly
     * like `editor.openDiff` itself never reaching the Go backend for its own local concerns.
     * `session: null` clears the stored session for `repoId` — sent by `clearTarget()` itself, so
     * an explicit "go back" never leaves a stale resume-point the next cold boot would silently
     * jump back into.
     */
    'review.session.save': {
      params: {
        repoId: string;
        session: ReviewSessionSnapshot | null;
      };
      result: Record<string, never>;
    };
    /** The read half — its own round trip, not carried on the bootstrap island, because
     *  a cold reveal's `repoId` is only known once the view's own `repo.list`
     *  call resolves — see `ReviewView.vue`'s own doc comment on `bootstrap()`'s resume path. A
     *  snapshot older than 14 days (`savedAt`, matching the `review.db` idle-purge number)
     *  is treated as expired and answered as `{session: null}`. */
    'review.session.load': {
      params: { repoId: string };
      result: { session: ReviewSessionSnapshot | null };
    };
    'commit.detail': {
      params: { repoId: string; sha: string; parentIndex?: number };
      result: {
        readonly sha: string;
        readonly parents: readonly string[];
        readonly author: CommitIdentity;
        readonly committer: CommitIdentity;
        readonly subject: string;
        /** `%b` with the trailer paragraph removed (W1) — the trailers travel structured,
         *  below. */
        readonly body: string;
        readonly trailers: readonly CommitTrailer[];
        readonly signature: { readonly status: SignatureStatus; readonly signer: string };
        /** `%D`, already parsed by `parse/log.ts` — "all refs pointing at this commit" (§6.4). */
        readonly decoration: readonly DecorationRef[];
        /** Which parent `files` is diffed against. Always 0 for a non-merge; always
         *  < parents.length. */
        readonly parentIndex: number;
        readonly files: readonly FileChange[];
      };
    };
    /** The status bar's one-line-at-a-time query — never a whole-file blame (no such request
     *  exists). Always blames the working tree, never a historical revision (no `atSha` param — no
     *  caller needs one). `sha` is the all-zero sentinel
     *  (`'0000000000000000000000000000000000000000'`) for a line whose content isn't in any commit
     *  yet (an unsaved-but-on-disk edit) — the caller checks this before treating `sha` as a real
     *  commit to route `revealCommit` at. */
    'blame.line': {
      params: { repoId: string; path: string; line: number };
      result: {
        readonly sha: string;
        readonly author: string;
        readonly authorTimeSeconds: number;
        readonly summary: string;
      };
    };
    /** The uncommitted-changes strip's click-through — every staged/unstaged/
     *  untracked change against HEAD (or the empty tree, when HEAD is unborn), in the identical
     *  `FileChange` shape `commit.detail` already produces, so `FileTree.vue` renders it with no
     *  new component logic. No `sha`, `parents`, `author` or `subject` — the working tree is not a
     *  commit, which is exactly why this is its own request rather than a `commit.detail` call
     *  with a synthetic sha. */
    'working.detail': {
      params: { repoId: string };
      result: { readonly files: readonly FileChange[] };
    };
    /** Opens the working-tree diff for one file — the one path this whole contract
     *  intentionally sends a LIVE on-disk path for (every other `editor.*`
     *  method's own left/right documents are immutable history). Answered entirely host-side,
     *  exactly like `editor.openRangeDiff` — the Go server never sees this method.
     *  `status === 'deleted'` (no live file exists to diff against) opens no diff. */
    'editor.openWorkingDiff': {
      params: {
        repoId: string;
        path: string;
        originalPath?: string;
        status: FileChangeKind;
        pinned?: boolean;
      };
      result: Record<string, never>;
    };
    'commit.fileDiff': {
      params: {
        repoId: string;
        sha: string;
        path: string;
        /** From the same `FileChange` — passed so the argv can name both sides (probe P2). */
        originalPath?: string;
        parentIndex?: number;
      };
      result: {
        readonly sha: string;
        readonly parentIndex: number;
        /** The pre-image revision, or null for a root commit (diffed against the empty tree). */
        readonly baseSha: string | null;
        /** Echoed so the view has status, rename arrow and counts without a second lookup. */
        readonly change: FileChange;
        readonly body: FileDiffBody;
      };
    };
    /** "Open in editor" — hands the same two blobs to the host's native diff.
     *
     *  Two additive params. `pinned` (optional so a caller that forgets it is a
     *  type error at the transport-building call site, never a silent default) —
     *  `true` opens a real, permanent tab and `false` a preview tab. `fallbackSha`
     *  is the stash tree's own need: a stash's `-u` untracked files live only in its third
     *  parent (`entry.untrackedSha`), which has no `baseSha` of its own — the handler retries the
     *  whole `commit.detail`-composition against `fallbackSha` when `path` is not among `sha`'s
     *  own changed files, instead of throwing, mirroring the retry `state/stash.ts` already
     *  implemented for the now-deleted in-webview diff path. */
    'editor.openDiff': {
      params: {
        repoId: string;
        sha: string;
        path: string;
        originalPath?: string;
        parentIndex?: number;
        pinned?: boolean;
        fallbackSha?: string;
      };
      result: Record<string, never>;
    };
    /** The review sidebar's own diff request: a two-revision comparison
     *  for one path, which (unlike editor.openDiff) is not one commit's parent-child pair.
     *  Answered entirely host-side, exactly like editor.openDiff — the server never
     *  sees this method. `status` is carried rather than re-derived because the caller
     *  (review.files) already knows which side is `{kind: 'empty'}` (an added file has no
     *  base-side blob).
     *
     *  Both sides are now sha-addressed (never a branch name), and `branch`
     *  is carried so the right-hand document can be marked as that branch's tip — the fourth
     *  virtual-key field (`virtualKey.ts`) that is what lets a comment, and a hunk mark, be
     *  anchored exactly rather than approximately. This closes three defects at once: a
     *  branch-addressed document's content used to be cached per URI and never
     *  invalidated (a stale tab could silently mis-anchor a comment against content that had
     *  since moved); `range` mode's left side used to be `base` even though the file list beside
     *  it is the three-dot (merge-base) set, so the diff and the file list could disagree about
     *  what the branch changed. */
    'editor.openRangeDiff': {
      params: {
        repoId: string;
        /** The review session's branch — carried so the right-hand document can be marked as
         *  its tip (G13 D8a), never itself a revision on either side of the diff. */
        branch: string;
        /** The branch tip's own commit sha: the right-hand document's revision. */
        branchTip: string;
        /** The left-hand document's revision — the merge base in `range` mode (G13 F7), the
         *  file's own `reviewedAtSha` in `sinceReview` mode when changed since review. Always a
         *  commit sha, never a ref name. */
        leftRev: string;
        /** What to call the left side in the tab title (`main`, `your last review`). Display
         *  only. */
        leftLabel: string;
        path: string;
        originalPath?: string;
        status: 'added' | 'deleted' | 'modified' | 'renamed';
        /** G21 D13 — same meaning and same mapping as `editor.openDiff`'s own `pinned`. */
        pinned?: boolean;
      };
      result: Record<string, never>;
    };
    /**
     * "Open all changes" — composes the whole file list from **one**
     * `commit.detail` (collapsing what used to be N separate `editor.openDiff` round trips, one
     * per file, into one), reusing the exact `DocumentRef` derivation `editor.openDiff`'s own
     * handler already performs. Space's `editor.openAllChanges` handler (`hostHandlers.ts`) opens
     * every file as a preview-cohort tab — a bulk open the original bug was about, and that bug is
     * never regressed by the per-file preview/pin split, but a whole commit's files permanently
     * pinning the strip was never the ask either.
     */
    'editor.openAllChanges': {
      params: { repoId: string; sha: string; parentIndex?: number };
      result: {
        readonly opened: number;
        readonly failed: number;
        readonly mode: 'multiDiff' | 'tabs';
      };
    };
    /**
     * D14a. `line` in is 1-based **in `rev`'s version of `path`** — the UI maps the cursor row
     * to the historical revision (`mapDiffLineToRevision`) and stops there. `line` out, on the
     * outcome, is the line actually revealed. The two are equal on every branch except
     * `liveFile`, where the handler re-maps across the commit→worktree drift (`worktreeDiff` +
     * `mapLineAcrossDiff`) so the UI never has to run a second mapping or a second round trip.
     */
    'editor.goToFile': {
      params: { repoId: string; rev: string; path: string; line: number };
      result: GoToFileOutcome;
    };
    /** `label` is for the host's log line only — never the content, which may be a whole
     *  message. */
    'clipboard.write': {
      params: { text: string; label: string };
      result: Record<string, never>;
    };
    // ---- P6: refs, status, pre-flight, operations -----------------------------------------
    'refs.list': {
      params: { repoId: string };
      /** Two spawns: heads+remotes `--sort=-committerdate`, tags `--sort=-v:refname` (§7.9's
       *  version-aware sort, which git does correctly and JS does not). */
      result: {
        branches: readonly RefRow[];
        remoteBranches: readonly RefRow[];
        tags: readonly RefRow[];
        head: HeadState;
      };
    };
    'status.get': {
      params: { repoId: string };
      result: StatusSummary;
    };
    'preflight.checkout': {
      params: { repoId: string; target: string; mode: 'switch' | 'detach' };
      result: CheckoutPreflight;
    };
    'preflight.revert': {
      params: { repoId: string; shas: readonly string[]; mainline?: number };
      result: RevertPreflight;
    };
    'stash.list': {
      params: { repoId: string };
      /** One spawn: `stash list -z --numstat -M -C --format=…` (probe 12). */
      result: { entries: readonly StashEntry[] };
    };
    /** The stash's own file list for the detail pane — tracked and, with `-u`, untracked, from
     *  one `stash show --numstat/--name-status -z -u -M -C` pair. `scope`: absent or `'stack'`
     *  resolves against the ordinary stack; `'global'` resolves against the bucket instead — the
     *  same query either way, since a stash commit's own tree diffed against its base means the
     *  same thing regardless of which bucket the ref lives in. */
    'stash.show': {
      params: { repoId: string; sha: string; scope?: 'stack' | 'global' };
      result: { sha: string; changes: readonly FileChange[] };
    };
    /** `targetSha` omitted ⇒ HEAD. Supplied by the `stashAndCarry` route, which predicts against
     *  the commit it is about to switch to. `scope`: same optional 'stack'/'global' shape
     *  as `stash.show` — the prediction machinery itself (`stashPopPrediction`,
     *  `ClassifyStashPop`) is UNCHANGED either way: only which bucket the entry is
     *  resolved from differs. */
    'preflight.stashPop': {
      params: {
        repoId: string;
        sha: string;
        index: number;
        targetSha?: string;
        scope?: 'stack' | 'global';
      };
      result: StashPopPreflight;
    };
    /** `scope` (G28 D17): same optional shape. For `scope: 'global'` there is no stack position to
     *  verify (`ClassifyStashBranch` is unaffected; `op.run`'s own `stashBranch` kind takes a
     *  sha-addressed arm instead, D12). */
    'preflight.stashBranch': {
      params: { repoId: string; sha: string; branch: string; scope?: 'stack' | 'global' };
      result: StashBranchPreflight;
    };
    /** G28 D9: the global stash bucket's own listing — `refs/kira/globalstash/<sha>`, one ref per
     *  entry (D8). Reuses `stash.list`'s own result shape verbatim (D17: zero new wire interfaces —
     *  the bucket's entries are `StashEntry` rows with `scope: 'global'`, not a parallel type). */
    'globalStash.list': {
      params: { repoId: string };
      result: { entries: readonly StashEntry[] };
    };
    'preflight.reset': {
      params: { repoId: string; target: string; mode: ResetMode };
      result: ResetPreflight;
    };
    'preflight.cherryPick': {
      params: { repoId: string; sha: string; mainline?: number };
      result: CherryPickPreflight;
    };
    'op.run': {
      params: { repoId: string; op: OpRequest };
      result: OpResult;
    };
    'undo.peek': {
      params: { repoId: string };
      result: { slot: UndoSlotSnapshot | null };
    };
    'undo.run': {
      params: { repoId: string; id: string };
      result: OpResult;
    };
    'remote.pullPreflight': {
      /** The strategy resolves from this repo's stored `kiraSpace.pull.strategy`. */
      params: { repoId: string; branch: string };
      result: PullPreflight;
    };
    'remote.pushPreflight': {
      params: { repoId: string; branch: string; remote: string };
      result: PushPreflight;
    };
    /** One key for all five `RemoteOpKind`s (D51) — never routed through `op.run`: a remote op
     *  is killable/streaming/progress-reporting and reuses the write queue's `busy` flag, none of
     *  which `op.run`'s local, synchronous ops need (§4.3, §7.1). */
    'remote.run': {
      params: RemoteOpParams;
      result: RemoteOpResult;
    };
    /** No-op if the named op already finished or was never running (OQ7: reject a second
     *  concurrent op with `OperationInProgress` rather than queue it — this cancels the one
     *  that's already in flight, it does not enqueue a second). */
    'remote.cancel': {
      params: { repoId: string };
      /** false when there was nothing to cancel (already finished, never running, or the op is
       *  past its killable phase — push/forcePush/deleteRemoteBranch/pull's merge-rebase phase)
       *  — never an error: a cancel racing a just-finished op is an ordinary outcome, not a
       *  fault. */
      result: { readonly cancelled: boolean };
    };
    // ---- P11: search -----------------------------------------------------------------------
    /** One request, one cancellable read — no cancel key, no second walk session.
     *  Superseded by the caller's own `AbortSignal`, which `rpc.ts` already threads to the spawn.
     *  Refs/Both are resolved entirely client-side against `RefsState`;
     *  this request is commits-only. */
    'search.run': {
      params: { repoId: string; query: SearchQueryParams; limit?: number };
      result: SearchRunResult;
    };
    // ---- G4 (v1.3 chapter): server-only methods -------------------------------------------
    /** The blob at `<rev>:<path>`, for read-only text documents (G4 D14). Text only: a binary
     *  blob is refused rather than encoded. */
    'file.read': {
      params: { repoId: string; rev: string; path: string };
      result:
        | { readonly kind: 'found'; readonly content: string }
        | { readonly kind: 'missing' }
        | { readonly kind: 'binary' }
        | { readonly kind: 'tooLarge'; readonly bytes: number; readonly limitBytes: number };
    };
    /** D14a's decision procedure, minus the part only the editor host can do (G4 D4). The host maps
     *  `line` through `hunks` with `@kira/git-core`'s `mapLineAcrossDiff` and then reveals; it
     *  never asks the filesystem or the object database anything itself. */
    'file.goToTarget': {
      params: { repoId: string; rev: string; path: string };
      result:
        | {
            readonly kind: 'live';
            readonly absPath: string;
            /** `null` ⇒ do not re-map: identical file, a path git cannot see, over the patch
             *  cap, or a spawn that failed. A refinement declining to fire is never an error. */
            readonly hunks: readonly DiffHunk[] | null;
          }
        | { readonly kind: 'historical'; readonly rev: string; readonly path: string }
        | {
            readonly kind: 'unavailable';
            readonly reason: 'notInRevision' | 'binary' | 'tooLarge';
          };
    };
    // ---- G18: the per-repo settings dialog (D4) --------------------------------------------
    /** Reads repoId's own stored settings — every leaf is scoped by repoId. */
    'repoSettings.get': {
      params: { repoId: string };
      result: RepoSettingsSnapshot;
    };
    /** Writes only the leaves the caller patched and returns the resulting snapshot —
     *  the same "patch, don't replace" discipline `RepoSettingsPatch` states at its own type.
     *  Also triggers `repoSettings.changed` (below) to every connected client, not only this
     *  one. */
    'repoSettings.set': {
      params: { repoId: string; patch: RepoSettingsPatch };
      result: RepoSettingsSnapshot;
    };
    // ---- G24: change request links (D9/D14) ------------------------------------------------------
    /** Per-commit PR lookup, driven by the graph indicator and the detail pane's own selection
     *  (D9): `ChangeRequestState.select`'s 300ms-debounced, abort-and-recheck request. Answered entirely by
     *  the Go server */
    'commit.resolveChangeRequests': {
      params: { repoId: string; sha: string };
      result: ChangeRequestLookup;
    };
    /** Per-branch PR lookup (D8), upstream's own branch-tip badge plus the reaper's own eager
     *  purge trigger — `state=all` server-side, so this is the one lookup that can report a PR
     *  that has since closed or merged. */
    'branch.resolveChangeRequest': {
      params: { repoId: string; branch: string };
      result: ChangeRequestLookup;
    };
    /** The persisted change request index (P352): every branch's change requests in one read,
     *  from SQLite and so answering before any host call. Same-repo heads only. Go-served. */
    'branch.changeRequestIndex': {
      params: { repoId: string };
      result: ChangeRequestIndex;
    };
    /** Syncs the index now (within the server's min gap and rate limit gates). `focused: false`
     *  only records that the window blurred, slowing the periodic cadence. `wait` blocks until the
     *  run ends. Go-served. */
    'branch.syncChangeRequests': {
      params: { repoId: string; focused?: boolean; wait?: boolean };
      result: Record<string, never>;
    };
    // ---- P74 §3.3: opening a PR in the external browser, never in place -------------------
    /** Server-composed: the renderer names a PR by number, never a URL (the boundary
     *  `GitHostService.OpenChangeRequestURL`'s own host/path check already defends). `null` when
     *  the host is off or no provider matches the remote — the same "disabled collapses to nothing"
     *  posture `ChangeRequestLookup` already takes. Go-served. */
    'changeRequest.browserUrl': {
      params: { repoId: string; number: number };
      result: { readonly url: string } | { readonly url: null };
    };
    /** Fresh review facts of the branch's change request. Go-served. */
    'changeRequest.reviewState': {
      params: {
        repoId: string;
        branch: string;
        /** > 0: the known change request; skips the branch lookup. */
        number?: number;
      };
      result: ChangeRequestReviewState;
    };
    /** Submits a review on the host, applied to `headSha`. Go-served. */
    'changeRequest.submitReview': {
      params: {
        repoId: string;
        number: number;
        event: ChangeRequestReviewEvent;
        body: string;
        headSha: string;
      };
      result: ChangeRequestPostResult;
    };
    /** Posts a conversation comment on the change request. Go-served. */
    'changeRequest.comment': {
      params: { repoId: string; number: number; body: string };
      result: ChangeRequestPostResult;
    };
    /** Posts a comment on lines of a file at the change request's head (right side). `headSha` is
     *  the commit the line numbers refer to; the server refuses with `headMoved` when the host
     *  head differs. Go-served. */
    'changeRequest.lineComment': {
      params: {
        repoId: string;
        number: number;
        path: string;
        range: LineRange;
        headSha: string;
        body: string;
      };
      result: ChangeRequestLineCommentResult;
    };
    /** The provider a repository's change-request calls go to under its `kiraSpace.gitHost.provider`
     *  setting; `host: null` when the setting is `off` or no provider matches the origin remote. */
    'gitHost.info': {
      params: { repoId: string };
      result: { readonly host: GitHostInfo | null };
    };
    /** Every registered provider, in registration order — the repository settings choice. */
    'gitHost.providers': {
      params: Record<string, never>;
      result: { readonly providers: readonly GitHostInfo[] };
    };
    /** Host-answered (like every `editor.*` request before it, D3.3):
     *  requests `changeRequest.browserUrl` and hands the URL to the host's own
     *  browser-open path — never answered by the Go server itself. */
    'changeRequest.openExternal': {
      params: { repoId: string; number: number };
      result: Record<string, never>;
    };
    /** Opens an arbitrary URL `linkify.ts` found in a commit message body, in the
     *  OS browser — `CommitMeta.vue`'s message-body links, unlike the PR row above, cannot be
     *  server-composed (the URL *is* the untrusted renderer-visible content, a commit message).
     *  Host-answered like `changeRequest.openExternal`, never proxied to the Go server; each host
     *  still refuses anything that is not a well-formed http(s) URL before its own OS-open call
     *  ever sees it — the same "boundary is the thing being defended" posture, applied to a URL
     *  this process can only validate, never compose. */
    'link.openExternal': {
      params: { url: string };
      result: Record<string, never>;
    };
    // ---- G25: worktree support (D1/D4/D8/D9-D14) -------------------------------------------
    /** D1: one spawn (`git worktree list --porcelain -z`), never cached — see `WorktreeEntry`'s
     *  own doc comment for why. */
    'worktree.list': {
      params: { repoId: string };
      result: { readonly worktrees: readonly WorktreeEntry[] };
    };
    'preflight.worktreeAdd': {
      params: {
        repoId: string;
        path: string;
        mode: 'existingBranch' | 'newBranch' | 'detach';
        branch?: string;
        startPoint?: string;
      };
      result: WorktreeAddPreflight;
    };
    'preflight.worktreeRemove': {
      params: { repoId: string; path: string };
      result: WorktreeRemovePreflight;
    };
    // ---- G26: stacked branches (D1-D17) -----------------------------------------------------
    /** One `git config --local --null --get-regexp` spawn, always; one `rev-list
     *  --left-right --count` spawn per stacked branch beyond that, bounded by
     *  `MaxStackedBranches` (64) — a repository with no stacked branches costs exactly the first
     *  spawn and nothing else. Cache-reading (dropped on `refsChanged`/any write). */
    'stack.list': {
      params: { repoId: string };
      result: StackListResult;
    };
    /** D14: always FRESH (never `stack.list`'s own cache) — this decision precedes a write, the
     *  same discipline `preflight.checkout`/`preflight.worktreeAdd` already follow. */
    'preflight.restack': {
      params: { repoId: string; branch: string };
      result: RestackPreflight;
    };
    /** A long, cancellable, streaming method modelled exactly on `remote.run` — its own
     *  dedicated executor (`gitsession.RunRestack`), NOT server via the shared `op.run` table
     *  (recording each child's new base needs the parent's post-rebase tip, which no static
     *  argv list can express). Emits `stack.progress` before each branch. */
    'stack.restack': {
      params: { repoId: string; branch: string };
      result: RestackResult;
    };
    /** No-op if the run already finished or was never running — the same "never an error, a
     *  cancel racing a just-finished op is ordinary" shape `remote.cancel`
     *  already uses. No palette command serves this directly: cancel is a button on the
     *  surface that started the work. */
    'stack.cancelRestack': {
      params: { repoId: string };
      result: { readonly cancelled: boolean };
    };
    // ---- P75 §2.3: reveal a review commit in the graph, from either host -------------------
    /** Replaces the review row's old `command:kiraSpace.openCommitInGraph?…` anchor (a webview link
     *  escape hatch — `RepoReviewView.vue` mounts the row component in a Wails WebView, where no
     *  `command:` handler exists at any layer). Host-answered, never reaching Go
     *  (like `review.open`/`editor.openRangeDiff`): the desktop answers by emitting/stashing
     *  `ui.action`'s own `revealCommit` onto the pinned graph tab. `revealed: false` is a
     *  real, non-stub answer — the desktop genuinely cannot reveal when this window has no record
     *  of the repository, or its workspace has no pinned graph tab open. */
    'graph.revealCommit': {
      params: { repoId: string; sha: string };
      result: { readonly revealed: boolean };
    };
  };
  events: {
    'repo.changed': {
      repoId: string;
      kind: 'refsChanged' | 'worktreeChanged' | 'changeRequestsChanged';
    };
    /** G18 D4/D7: fanned out to every connected client whenever `repoSettings.set` succeeds
     *  anywhere, not only to the connection that made the change. `repoId` names which repo's own
     *  write triggered the emit; a viewer decides for itself whether it is relevant. */
    'repoSettings.changed': { repoId: string; settings: RepoSettingsSnapshot };
    /** P173: auto-fetch stopped (non-null) or re-armed (`null`) for a held repository. */
    'autoFetch.changed': { repoId: string; autoFetch: AutoFetchStatus | null };
    /** Host -> the review view only: "review this branch instead". */
    'review.target': { repoId: string; branch: string };
    /** Throttled to 100ms (OQ10) — live progress for whichever `remote.run` is in flight. */
    'remote.progress': RemoteProgress;
    /** G26 D6 step 4: one event per planned branch (never a stream within one) for whichever
     *  `stack.restack` is in flight. */
    'stack.progress': RestackProgress;
    /** Host -> whichever webview the action targets — the graph panel for every mutating op
     *  (the review sidebar renders no operation UI), and the review sidebar for
     *  'toggleFileReviewed'. One palette command's action, routed to the affordance the toolbar or
     *  a context menu
     *  already drives — the palette is an entry point, never a second implementation. */
    'ui.action': {
      readonly action: UiActionKind;
      /** G14 D10: present only for actions that name a commit ('revealCommit'). Host ->
       *  webview only; the Go server neither emits nor parses ui.action. */
      readonly target?: { readonly repoId: string; readonly sha: string };
    };
  };
  streams: {
    'graph.stream': {
      params: {
        repoId: string;
        /** Honored the same way with `range` present or absent (this field used to be structurally
         *  documented as ignored for a ranged
         *  request, but the server reuses the same walk, and so the same row cache, across every
         *  `graph.stream` call for one range spec exactly like the graph's own non-ranged walk;
         *  `review.ts`'s own `loadMore` needs this to resume past what it already applied instead
         *  of re-streaming the whole range from row 0 on every page). */
        resumeThroughRow?: number;
        /** Present ⇒ walk `<base>..<branch>` instead of the repo's `graph.scope` rev set,
         *  against this repo's own separate review walk. Chunk shape is byte-for-byte the same. */
        range?: CommitRange;
      };
      chunk: {
        readonly repoId: string;
        readonly seq: number;
        /** Absolute row indices, not chunk-relative. */
        readonly from: number;
        readonly to: number;
        /** §5.4 made observable; W9 renders it, P4 keeps it. */
        readonly source: 'git' | 'cache';
        readonly remaining: number;
        readonly exhausted: boolean;
        readonly commits: PackedCommitChunk;
      };
    };
  };
};

export type RequestKey = keyof Contract['requests'];
export type EventKey = keyof Contract['events'];
export type StreamKey = keyof Contract['streams'];

export type ParamsOf<K extends RequestKey> = Contract['requests'][K]['params'];
export type ResultOf<K extends RequestKey> = Contract['requests'][K]['result'];
export type EventPayload<K extends EventKey> = Contract['events'][K];
export type StreamParamsOf<K extends StreamKey> = Contract['streams'][K]['params'];
export type StreamChunkOf<K extends StreamKey> = Contract['streams'][K]['chunk'];
