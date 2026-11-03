import type { EventKey, RequestKey, StreamKey } from './contract.ts';

/**
 * Boundary validation. Per §3.5, a contract mismatch must fail loudly rather than
 * half-work — so this throws, it does not degrade.
 */
// G10 D9: 16 -> 17 for one new event, 'ui.action' — the palette's own route into an already-
// mounted webview (see contract.ts's own doc comment on UiActionKind). This is the one place G10
// touches the wire contract; every other change this phase makes is packaging mechanics.
// G11 D1: 17 -> 18 for three new requests (review.files, review.fileDiff, review.mark) and one new
// UiActionKind member ('toggleFileReviewed').
// G12 D1: 18 -> 19 for one new request, 'editor.openRangeDiff' — a two-revision diff for the
// review sidebar's Files pane, answered entirely inside the extension (never emitted or parsed by
// the Go server), the same "extension-answered but the sole compatibility authority still moves"
// precedent G10 D9 set for 'ui.action'.
// G13 D1: 19 -> 20 for five new requests ('review.comment.add/list/remove/clear/export'),
// 'editor.openRangeDiff's reshaped params (D8), and two new UiActionKind members
// ('copyReviewComments', 'refreshReviewComments').
// G14 D6/D10: 20 -> 21, one bump for two reasons landing in the same phase. D6:
// SettingsSnapshot gains the host-owned 'workbench.tree.indent' member. D10: UiActionKind gains
// 'revealCommit' and 'ui.action' gains an optional 'target' naming the commit to reveal — both
// extension-side only (the Go server neither emits nor parses either), the same "the sole
// compatibility authority still moves" precedent G10 D9/G12 D1 set.
// G18 D5: 21 -> 22, for three new requests (repoSettings.get/set, settings.setGitPath) and one
// new event (repoSettings.changed) — seven kiraSpace.* settings move out of
// contributes.configuration into their own per-repo store (D1/D3/D4); git.path's own dead
// server-side wiring is fixed in the same phase (D15) and its one-time migration leg needs its
// own tiny server-only request (D11) since git.path was never part of the per-repo store
// repoSettings.set writes. SettingsSnapshot narrows to its one remaining member
// (workbench.tree.indent); the seven moved keys now live in the new RepoSettingsSnapshot.
// G19 D11b: 22 -> 23, for two new requests ('review.session.save'/'review.session.load') — the
// review sidebar's durable "back to branch selection" resume point, additive only, answered
// entirely inside the extension against `context.workspaceState` and never reaching the Go
// backend (the same "extension-answered but the sole compatibility authority still moves"
// precedent G10 D9/G12 D1/G14 D6/D10 already established). No existing method's shape changes.
// G21 D8/D12/D13: 23 -> 24, one bump for three additive changes landing across this phase's own
// commits (stated here so it is never discovered piecemeal in a diff): one new request,
// 'editor.openAllChanges' (D8, the "Open all changes" multi-file diff, extension-answered like
// every 'editor.*' request before it); 'editor.openDiff' gains optional 'pinned' (D13) and
// 'fallbackSha' (D12, the stash-untracked-file retry); 'editor.openRangeDiff' gains the same
// optional 'pinned' (D13). No existing method's shape changes — every addition is optional.
// G22 D10: 24 -> 25, for two new UiActionKind members ('resetSelected', 'cherryPickSelected'),
// the palette's own route into ResetDialog.vue/CherryPickDialog.vue, neither of which had one.
// This is the only wire change this phase makes: every result/param/error shape
// 'preflight.reset'/'preflight.cherryPick'/'op.run's reset/cherryPick kinds serve already existed
// at CONTRACT_VERSION 24. Moves for the same reason 'ui.action' first did (G10 D9): this constant
// is the sole compatibility authority, even though the Go server neither emits nor parses
// 'ui.action' at all.
// G23 D6/D13: 25 -> 26, for one new 'SearchRunResult' member, 'unsupportedPattern' -- a
// `regex`-mode pattern using lookahead/lookbehind/a backreference, syntax the Go tail scan's RE2
// engine cannot express at all (docs/HISTORY.md (v1.3)'s own "a hit's presence must not depend
// on which page happens to be loaded"). Unlike every earlier bump in this history, the Go server
// now actually serves 'search.run' for the first time -- this is the phase that ports upstream's
// P11 tail scan into internal/gitsearch, not merely a contract-shape change for a method the
// extension already answered on its own.
// G24 D14: 26 -> 27, for two new Go-served requests ('commit.resolvePr', 'branch.resolvePr'), four
// new wire types (GhStatus/PrRecord/PrLookupResult and the state string union it carries), and one
// new RepoSettingsSnapshot member ('kiraSpace.github.enabled'). Additive only -- SearchMatchField
// is untouched (F9: the wire's own search-field union is commits-only, the PR fields live entirely
// in git-core's client-side SearchField instead).
// G25 D16 (2026-09-09): 27 -> 28, for worktree support -- no upstream design existed for this
// phase at all (this chapter's own SPEC row was a placeholder). Six new requests ('worktree.list',
// 'preflight.worktreeAdd', 'preflight.worktreeRemove', 'worktree.prepare',
// 'worktree.cancelPrepare', 'worktree.openWindow' -- the last answered entirely inside the
// extension, the same "editor.*-shaped" precedent 'editor.openDiff' already set), one new event
// ('worktree.progress'), two new 'OpRequest' kinds ('worktreeAdd', 'worktreeRemove'), one new
// 'OpErrorKind' member ('WorktreeLocked' -- 'worktree.prepare's own four synthetic refusals live
// in a SEPARATE, dedicated 'WorktreePrepareErrorKind' instead, spending none of this budget), two
// new capabilities ('openWorktreeWindow', 'runPrepareScript'), one new 'UiActionKind' member
// ('createWorktree'), and two new 'RepoSettingsSnapshot' members ('kiraSpace.worktree.
// prepareScript', 'kiraSpace.worktree.basePath'). The prepare script is written only in-process by
// Kira Space (P172); 'repoSettings.set' refuses it.
// G26 D17 (2026-09-09): 28 -> 29, for four new Go-served requests ('stack.list',
// 'preflight.restack', 'stack.restack', 'stack.cancelRestack'), one new event ('stack.progress'),
// nine new wire types ('StackBranchState', 'StackBranch', 'StackSummary', 'StackListResult',
// 'RestackBlocker', 'RestackPlanEntry', 'RestackPreflight', 'RestackResult', 'RestackProgress'),
// one new 'OpRequest' kind ('stackSet'), one new 'OpErrorKind' member ('StackCycle' -- produced
// exclusively by 'stackSet's own cycle check, never by rebase itself), and three new
// 'UiActionKind' members ('restackStack', 'checkoutStackParent', 'checkoutStackChild'). No new
// capability, no new setting, no SQL migration (this phase's own §8 explicit non-goals).
// G28 D17 (2026-09-09): 29 -> 30, for branch-scoped stash -- auto-stash on checkout, cross-branch
// apply, auto-detach on worktree conflict, and a durable global stash bucket. One new Go-served
// request ('globalStash.list' -> {entries: StashEntry[]}, reusing the existing result shape
// verbatim); three requests widen their params with an optional 'scope' ('stack'|'global', absent
// = 'stack') -- 'stash.show', 'preflight.stashPop', 'preflight.stashBranch'; two new 'OpRequest'
// kinds ('globalStashSave', 'globalStashRemove'); one new 'OpRequest' field
// ('checkout.autoStash'); two new 'CheckoutPreflight.routes' members ('autoStash', 'detachHere')
// and one new 'WorktreeAddPreflight.routes' member ('detachHere') -- all three additive to an
// EXISTING string-array field, no new wire type; one new 'OpErrorKind' member
// ('NothingToStash'); one new 'UiActionKind' member ('saveGlobalStash'); one new
// 'RepoSettingsSnapshot' member ('kiraSpace.checkout.autoStash', read client-side only). 'StashEntry'
// itself widens by two fields ('scope', 'ref') rather than forking a parallel 'GlobalStashEntry'
// type -- so, notably, ZERO new wire interfaces. No SQL migration, no watcher change, no new
// 'ClassifyOpError' stderr row (this phase's own explicit non-goals).
// G-UX D4/D9 (2026-09-09): 30 -> 31, for two graph/review UX fixes' wire impact. 'repo.pick' is
// REMOVED from 'Contract["requests"]' -- the workspace's own folders are now the only source of
// repositories, so the native-folder-picker fallback has no wire method left to reach (D4). One
// new 'UiActionKind' member, 'toggleSearch' -- the palette's route to toggling the graph panel's
// search row (D9). No new capability, no new setting, no SQL migration.
// G30 round-1 code review (2026-09-09): 31 -> 32, one new 'OpErrorKind' member, 'BranchChanged' --
// remote.run's pull integrate phase now refuses (rather than silently writing to the wrong
// branch) when HEAD changed out from under it between the fetch and the merge/rebase (finding
// #2). No new request, no new capability, no SQL migration.
// G-UX D13 (item 13): 32 -> 33, one new event, 'connection.changed' -- pushes the connection
// state that used to reach only the extension's own status bar into both webviews themselves, so
// a panel that stays open through a drop shows it too. No new request, no new capability, no SQL
// migration.
// P5 (2026-09-11): 33 -> 34, for one new Go-served request, 'blame.line' -- the status-bar blame
// widget's own one-line-at-a-time query (params: repoId/path/line; result: sha/author/
// authorTimeSeconds/summary, 'sha' the all-zero sentinel for an uncommitted line). No webview
// caller exists yet, only the extension's own host-side status bar -- 'proxyHandlers.ts' still
// gained a plain forward entry since 'ServerHandlers.requests' is total over 'RequestKey'. No new
// event, no new capability, no new 'UiActionKind' member, no SQL migration.
// P7 item 2 (2026-09-12): 34 -> 35, for one new Go-served request, 'working.detail' -- the
// uncommitted-changes strip's click-through file list (params: repoId; result: {files:
// FileChange[]}, composed server-side the same way 'commit.detail' composes its own). A real
// webview caller exists this time (the new WorkingDetailPane), so this is a plain forward in
// 'proxyHandlers.ts' too, no different in kind from 'blame.line's own bump. One new
// extension-only request, 'editor.openWorkingDiff' (never reaches the Go server, answered
// entirely inside the extension exactly like 'editor.openRangeDiff') -- included in
// 'REQUEST_KEY_MAP' below for the same total-over-'RequestKey' exhaustiveness reason every request
// key is, but does not itself require this version bump (extension-only methods never did). No new
// event, no new capability, no new 'UiActionKind' member, no SQL migration.
// P74 §3.3 (2026-09-15): 35 -> 36, for one new Go-served request, 'pr.browserUrl' (composes a
// PR's github.com URL server-side; params: repoId/number, result: {url: string|null}) and one new
// extension-answered request, 'pr.openExternal' (params: repoId/number, result: {}), which
// requests 'pr.browserUrl' over the socket and hands the URL to the host's own browser-open path
// -- never answered by the Go server itself, the same "editor.*-shaped" precedent 'editor.openDiff'
// already set. One new 'app.init' capability, 'openExternal' (both hosts report true). No SQL
// migration.
// P75 §2.3 (2026-09-15): 36 -> 37, for one new host-answered request, 'graph.revealCommit'
// (params: repoId/sha, result: {revealed: boolean}) -- replaces the review row's VS Code-only
// `command:` URI anchor with a real request both hosts answer locally, never reaching Go (the
// same 'editor.*-shaped' precedent above). No new event, no new capability, no SQL migration.
// P77 §13 (2026-09-16): 37 -> 38, four new 'UiActionKind' members -- 'openTagPicker',
// 'openStashPicker', 'openWorktreePicker', 'openStackPicker' -- routing the palette's tag/stash/
// worktree/stack commands to the branch picker's own matching tab (D22's five-tab redesign)
// instead of funnelling all of them into plain 'openBranchPicker'. No new request, no new event,
// no new capability, no SQL migration.
// P79 finding 4 (2026-09-16): 38 -> 39, one new host-answered request, 'link.openExternal'
// (params: {url: string}, result: {}) -- opens an arbitrary URL 'linkify.ts' found in a commit
// message body in the OS browser, replacing 'CommitMeta.vue's own raw `<a href>` (a real
// whole-window navigation under Wails, which has no anchor-click interception the way
// 'pr.openExternal' already gets one from Go-side re-validation). Never reaches the Go server --
// unlike 'pr.openExternal' this URL is untrusted renderer-visible content already (the commit
// message itself), not something the Go side can compose, so each host still refuses anything but
// a well-formed http(s) URL before its own OS-open call. No new event, no new capability
// ('openExternal' already gates this the same way it gates 'pr.openExternal'), no SQL migration.
// P100 Part 3: 39 -> 40, no shape change at all -- the eleven 'RepoSettingsSnapshot' keys
// (§ below) are renamed 'kiraVersion.*' -> 'kiraSpace.*' as the extension itself is renamed to
// Kira Space. A wire-compatible extension built before this rename would otherwise silently
// write and read settings under the old key names against a server that has migrated its stored
// rows to the new ones -- deliberately rejected by the version gate instead, the same reasoning
// every version-only bump in this history has already established (G10 D9): the mismatch must be
// a loud, blocking panel, not a settings value that quietly stops round-tripping.
// P111: 40 -> 41, one new PullPreflight field (rebaseMerges) and one new remote.run param
// (rebaseMerges) -- the pull executor stops re-deriving --rebase-merges from config and takes it
// from the preflight the client already ran. No new request, no new event, no SQL migration.
// P150: 41 -> 42, one new request (review.snapshot) -- the stored text a file had when it was last
// reviewed, so a review window's since-review left side survives a rewritten branch. No new
// event, no SQL migration beyond review.db's own pinned column.
// P172: 42 -> 43, two breaking changes. 'repoSettings.set' refuses the
// 'kiraSpace.worktree.prepareScript' patch leaf with E_READ_ONLY ('RepoSettingsPatch' omits it:
// the prepare script is written only in-process by Kira Space), and 'settings.setGitPath' is
// removed (the git path is set only in Kira Space's own Settings).
// P173: 43 -> 44, one new request ('graph.reportFailure'), one new event ('autoFetch.changed') and
// one new field on 'StatusSummary' ('autoFetch'): the toolbar's "Auto-fetch stopped" marker and the
// server-side record of a corrupted graph stream. The frame cap rises 8 -> 32 MiB on both sides.
// P178: 44 -> 45, behaviour changes. A socket client never receives 'credential.request' (its
// prompts open in Kira Space) and 'git.sock' refuses 'credential.provide' with E_READ_ONLY. Only
// the native stream still uses both. 'git.sock' also refuses 'repoSettings.set' with E_READ_ONLY
// (repository settings are edited in Kira Space only), and 'app.init' gains the
// 'capabilities.editRepoSettings' flag (Kira Space's native window true, VS Code false). The 'kiraSpace.log.level' leaf is removed
// from 'RepoSettingsSnapshot'/'RepoSettingsPatch' (it was never read). 'app.init' gains 'dateFormat'
// (Kira Space's app-wide appearance.dateFormat), which the VS Code graph follows.
// P243 Part 2: 45 -> 46, breaking. The VS Code extension is gone, so 'worktree.prepare',
// 'worktree.cancelPrepare', 'worktree.openWindow', 'editor.resolveConflict', the events
// 'worktree.progress', 'settings.changed' and 'connection.changed', and 'app.init''s 'host',
// 'settings' and 'capabilities' fields are removed. The extension-injected params ('scope' and
// 'pageSize' on 'graph.loadMore'/'graph.stream', 'baseCandidates' on 'review.resolveBase',
// 'strategySetting' on 'remote.pullPreflight') are removed; the server reads the stored
// 'kiraSpace.*' repo settings.
// P246: 46 -> 47, breaking. 'credential.request' and 'credential.provide' are removed: native git
// stream credential prompts go through Kira Space's prompt router and credential relay instead.
// P257: 47 -> 48, breaking. 'FileChange' loses 'additions', 'deletions' and 'isBinary'.
// P286: 48 -> 49, additive. New request 'review.comment.setState'; 'review.comment.add' gains an
// optional 'kind'; 'ReviewComment' gains 'kind', 'state', 'reply', 'repliedAt' and 'sentAt'.
// P318: 49 -> 50, breaking. The PR requests are renamed for any code host: 'commit.resolvePr' ->
// 'commit.resolveChangeRequests', 'branch.resolvePr' -> 'branch.resolveChangeRequest', 'pr.browserUrl'
// -> 'changeRequest.browserUrl', 'pr.openExternal' -> 'changeRequest.openExternal'. 'GhStatus',
// 'PrRecord' and 'PrLookupResult' become 'HostStatus', 'ChangeRequestRecord' and
// 'ChangeRequestLookup'; the lookup's 'prs' and 'gh' fields become 'changeRequests' and 'status'.
// 'kiraSpace.github.enabled' (boolean) becomes 'kiraSpace.gitHost.provider' (string). Two new
// requests, 'gitHost.info' and 'gitHost.providers', name a repository's host and every host.
// P352: 50 -> 51, additive. New requests 'branch.changeRequestIndex' and 'branch.syncChangeRequests';
// 'repo.changed' gains kind 'changeRequestsChanged'; 'ChangeRequestRecord' gains 'draft',
// 'reviewDecision' and 'checksState'.
// P358: 51 -> 52, additive. New request 'review.markMany' (batch whole-file mark).
// P369: 52 -> 53, breaking. 'GitHostInfo' loses 'fileReviewSync' (GitHub viewed sync removed).
// P370: 53 -> 54, additive. New requests 'changeRequest.reviewState', 'changeRequest.submitReview' and 'changeRequest.comment' (review a change request, comment on its conversation).
// P392: 54 -> 55, additive. 'changeRequest.reviewState' gains optional 'number' (a known change request skips the branch lookup).
// P413: 55 -> 56, additive. New request 'review.comment.edit' (replace a comment's body and kind).
// P418: 56 -> 57, additive. 'ReviewComment' gains 'origin' ('commit' | 'working'); 'CommentAnchor' gains
// 'uncommitted' (a comment written on uncommitted worktree content).
// P464: 57 -> 58, additive. New request 'changeRequest.lineComment' (comment on lines of a file at a
// change request's head).
export const CONTRACT_VERSION = 58;

export class ContractVersionMismatchError extends Error {
  readonly received: number;

  constructor(received: number) {
    super(
      `ipc contract version mismatch: this build expects ${CONTRACT_VERSION}, received ${received}`,
    );
    this.name = 'ContractVersionMismatchError';
    this.received = received;
  }
}

export function validateVersion(received: number): void {
  if (received !== CONTRACT_VERSION) {
    throw new ContractVersionMismatchError(received);
  }
}

export interface VersionedEnvelope<T> {
  readonly version: number;
  readonly body: T;
}

export function wrapVersioned<T>(body: T): VersionedEnvelope<T> {
  return { version: CONTRACT_VERSION, body };
}

export function unwrapVersioned<T>(envelope: VersionedEnvelope<T>): T {
  validateVersion(envelope.version);
  return envelope.body;
}

// ---------------------------------------------------------------------------------------
// assertContractShape — a per-key structural check on arrival.
// ---------------------------------------------------------------------------------------

/**
 * The complete method-name lists, mirroring `Contract`'s keys. TypeScript's own exhaustiveness
 * checking cannot reach across a wire, so these are the runtime half of the same guarantee — but
 * a plain `Set<RequestKey>` literal is only checked for *extra* keys, not missing ones (adding a
 * key to `Contract["requests"]` and forgetting it here compiles cleanly, and previously did:
 * all four `remote.*` requests and `remote.progress` were missing from these three sets, silently
 * failing every `assertContractShape` call for them since they were added to `Contract`
 * (`RpcError: ipc contract shape error … unknown request method`) with nothing short of an E2E
 * test through the real codec ever exercising this path to catch it — `mockBridge` calls its
 * handlers directly, `repoService`'s own integration tests never round-trip through
 * `assertContractShape`). Built through a `Record<Key, true>` rather than an array literal so a
 * *missing* key is a compile error too — TypeScript's mapped-type checker requires every key of
 * `RequestKey`/`EventKey`/`StreamKey` to be present, which a bare array can never enforce.
 */
const REQUEST_KEY_MAP: Record<RequestKey, true> = {
  'app.init': true,
  'repo.list': true,
  'repo.open': true,
  'repo.close': true,
  'graph.status': true,
  'graph.loadMore': true,
  'graph.refresh': true,
  'graph.reportFailure': true,
  'commit.detail': true,
  'commit.fileDiff': true,
  'editor.openDiff': true,
  'editor.openRangeDiff': true,
  'editor.openAllChanges': true,
  'editor.goToFile': true,
  'clipboard.write': true,
  'refs.list': true,
  'status.get': true,
  'preflight.checkout': true,
  'preflight.revert': true,
  'op.run': true,
  'undo.peek': true,
  'undo.run': true,
  'review.resolveBase': true,
  'review.open': true,
  'review.files': true,
  'review.fileDiff': true,
  'review.mark': true,
  'review.markMany': true,
  'review.snapshot': true,
  'review.comment.add': true,
  'review.comment.list': true,
  'review.comment.remove': true,
  'review.comment.setState': true,
  'review.comment.edit': true,
  'review.comment.clear': true,
  'review.comment.export': true,
  'remote.pullPreflight': true,
  'remote.pushPreflight': true,
  'remote.run': true,
  'remote.cancel': true,
  'stash.list': true,
  'stash.show': true,
  'preflight.stashPop': true,
  'preflight.stashBranch': true,
  'globalStash.list': true,
  'preflight.reset': true,
  'preflight.cherryPick': true,
  'search.run': true,
  'file.read': true,
  'file.goToTarget': true,
  'blame.line': true,
  'working.detail': true,
  'editor.openWorkingDiff': true,
  'repoSettings.get': true,
  'repoSettings.set': true,
  'review.session.save': true,
  'review.session.load': true,
  'commit.resolveChangeRequests': true,
  'branch.resolveChangeRequest': true,
  'branch.changeRequestIndex': true,
  'branch.syncChangeRequests': true,
  'gitHost.info': true,
  'gitHost.providers': true,
  'changeRequest.browserUrl': true,
  'changeRequest.reviewState': true,
  'changeRequest.submitReview': true,
  'changeRequest.comment': true,
  'changeRequest.lineComment': true,
  'changeRequest.openExternal': true,
  'link.openExternal': true,
  'worktree.list': true,
  'preflight.worktreeAdd': true,
  'preflight.worktreeRemove': true,
  'stack.list': true,
  'preflight.restack': true,
  'stack.restack': true,
  'stack.cancelRestack': true,
  'graph.revealCommit': true,
};
const EVENT_KEY_MAP: Record<EventKey, true> = {
  'repo.changed': true,
  'review.target': true,
  'remote.progress': true,
  'ui.action': true,
  'repoSettings.changed': true,
  'autoFetch.changed': true,
  'stack.progress': true,
};
const STREAM_KEY_MAP: Record<StreamKey, true> = {
  'graph.stream': true,
};
const REQUEST_KEYS: ReadonlySet<RequestKey> = new Set(Object.keys(REQUEST_KEY_MAP) as RequestKey[]);
const EVENT_KEYS: ReadonlySet<EventKey> = new Set(Object.keys(EVENT_KEY_MAP) as EventKey[]);
const STREAM_KEYS: ReadonlySet<StreamKey> = new Set(Object.keys(STREAM_KEY_MAP) as StreamKey[]);

export type ContractChannel = 'request' | 'event' | 'stream';

export class ContractShapeError extends Error {
  readonly channel: ContractChannel;
  readonly method: string;

  constructor(channel: ContractChannel, method: string, reason: string) {
    super(`ipc contract shape error on ${channel} '${method}': ${reason}`);
    this.name = 'ContractShapeError';
    this.channel = channel;
    this.method = method;
  }
}

function keysForChannel(channel: ContractChannel): ReadonlySet<string> {
  switch (channel) {
    case 'request':
      return REQUEST_KEYS;
    case 'event':
      return EVENT_KEYS;
    case 'stream':
      return STREAM_KEYS;
  }
}

/**
 * A per-key structural check on arrival, not a schema library: the wire is trusted-but-
 * versioned between two halves of one build. `validateVersion` rules out a stale build
 * talking to a fresh one; this rules out the one thing a version number alone cannot catch — a
 * method name or a `kind` discriminant that could not have come from this contract at all.
 * It does not re-validate every field, since a single build's own type-checker already
 * guarantees that; it exists for the boundary between two different builds.
 */
export function assertContractShape(
  channel: ContractChannel,
  method: string,
  payload: unknown,
): void {
  if (!keysForChannel(channel).has(method)) {
    throw new ContractShapeError(channel, method, `unknown ${channel} method`);
  }
  if (payload === null || typeof payload !== 'object') {
    throw new ContractShapeError(channel, method, 'payload is not an object');
  }
  const record = payload as Record<string, unknown>;
  if ('kind' in record && typeof record.kind !== 'string') {
    throw new ContractShapeError(channel, method, "'kind' discriminant is not a string");
  }
}
