// Package gitrpc is the server half of the git wire contract ("Port of rpcHandlers.ts,
// minus the seven host-capability methods the extension now answers locally"). It serves exactly
// three of them — app.init, repo.open, repo.close — with a uniform E_UNKNOWN_METHOD default
// for everything else, rather than 30 placeholder entries later phases would each have to revisit.
package gitrpc

// ContractVersion is the sole compatibility authority (SPEC §3.4, D20) — a structural copy of
// packages/git-ipc/src/validate.ts:7's CONTRACT_VERSION, kept honest by both sides reading the
// same upstream source rather than by codegen (a single integer does not justify one). A mismatch
// in the stream handshake is a hard, loud stop, not a silent drop.
// G7 D2: 15 -> 16, for exactly three additions — credential.request (a new event), credential.
// provide (a new request), and remote.pullPreflight's own optional strategySetting param.
// G10 D9: 16 -> 17, for one new event, ui.action — the palette's route into an already-mounted
// webview. This constant moves only because it is the single compatibility authority (SPEC §3.4):
// ui.action is a JSON control frame on the extension<->webview channel and never crosses the
// socket, so the Go server neither emits nor parses it.
// G11 D1: 17 -> 18, for three new requests (review.files, review.fileDiff, review.mark) and one
// new UiActionKind member (toggleFileReviewed) — no event, no stream, no change to any existing
// request's params or result.
// G12 D1: 18 -> 19, for one new request, editor.openRangeDiff — the review sidebar's two-revision
// diff, answered entirely inside the extension. This constant moves for the same reason ui.action
// did (G10 D9): it is the sole compatibility authority, even though the Go server neither emits
// nor parses this method.
// G13 D1: 19 -> 20, for five new requests (review.comment.add/list/remove/clear/export),
// editor.openRangeDiff's params reshaped (D8: both sides sha-addressed, the right-hand document
// carries the review branch — extension-answered, but this constant is still the sole
// compatibility authority, same precedent as G10/G12) and two new UiActionKind members
// (copyReviewComments, refreshReviewComments).
// G14 D6/D10: 20 -> 21, one bump for two reasons landing in the same phase — SettingsSnapshot's
// host-owned 'workbench.tree.indent' member (D6) and ui.action's optional 'target' plus the new
// 'revealCommit' UiActionKind member (D10). This constant moves for the same reason ui.action
// first did (G10 D9): it is the sole compatibility authority, even though the Go server neither
// emits nor parses either addition — both are extension<->webview only.
// G18 D5: 21 -> 22, for three new requests (repoSettings.get/set, settings.setGitPath) and one
// new event (repoSettings.changed) — seven kiraSpace.* settings move out of
// contributes.configuration into their own per-repo store (D1/D3/D4); SettingsSnapshot narrows to
// its one remaining member (workbench.tree.indent) and a new RepoSettingsSnapshot carries the
// seven moved keys.
// G19 D11b (2026-09-08): 22 -> 23, for two new requests (review.session.save/.load) — the review
// sidebar's durable "back to branch selection" resume point (F11/D11a/D11b), stored in the
// extension's own context.workspaceState and never reaching this server. This constant moves for
// the same reason ui.action first did (G10 D9): it is the sole compatibility authority, even
// though the Go server neither emits nor parses either addition.
// G21 D8/D12/D13 (2026-09-08): 23 -> 24, one bump for three additive changes landing across that
// phase's own commits — one new request, editor.openAllChanges (D8, the "Open all changes"
// multi-file diff), and two optional params, editor.openDiff's pinned/fallbackSha (D13/D12) and
// editor.openRangeDiff's own pinned (D13). This constant moves for the same reason ui.action
// first did (G10 D9): it is the sole compatibility authority, even though the Go server neither
// emits nor parses any of the three — every 'editor.*' request is answered entirely inside the
// extension, the same precedent editor.openRangeDiff itself set at G12 D1.
// G22 D10 (2026-09-09): 24 -> 25, for two new UiActionKind members (resetSelected,
// cherryPickSelected) — the palette's own route into ResetDialog.vue/CherryPickDialog.vue, neither
// of which had one. This is the only wire change this phase makes: every result/param/error shape
// preflight.reset/preflight.cherryPick/op.run's reset/cherryPick kinds serve already existed at
// CONTRACT_VERSION 24. This constant moves for the same reason ui.action first did (G10 D9): it is
// the sole compatibility authority, even though the Go server neither emits nor parses ui.action.
// G23 D6/D13 (2026-09-09): 25 -> 26, for one new SearchRunResult member, unsupportedPattern — a
// `regex`-mode pattern using lookahead/lookbehind/a backreference, syntax the Go tail scan's RE2
// engine cannot run at all (SPEC's own "a hit's presence must not depend on which page happens to
// be loaded", docs/HISTORY.md (v1.3)). Unlike most of this constant's history, this bump also
// lands the first real Go implementation of the method it accompanies (search.run) rather than
// only the wire shape for an extension-answered method.
// G24 D14 (2026-09-09): 26 -> 27, for two new Go-served requests (commit.resolvePr,
// branch.resolvePr), four new wire types (GhStatus, PrRecord, PrLookupResult, and the state string
// union it carries) and one new RepoSettingsSnapshot member (kiraSpace.github.enabled). Every
// addition is additive; SearchMatchField/CommitSearchHit/internal/gitsearch are untouched (F9 —
// the wire's own search-field union is commits-only, the PR fields live entirely in git-core's
// client-side SearchField instead).
// G25 D16 (2026-09-09): 27 -> 28, for six new requests (worktree.list, preflight.worktreeAdd,
// preflight.worktreeRemove, worktree.prepare, worktree.cancelPrepare, worktree.openWindow), one new
// event (worktree.progress), two new OpRequest kinds (worktreeAdd, worktreeRemove), one new
// OpErrorKind (WorktreeLocked), two new host capabilities (openWorktreeWindow, runPrepareScript),
// one new UiActionKind, and two new RepoSettingsSnapshot members (kiraSpace.worktree.
// prepareScript, kiraSpace.worktree.basePath). worktree.openWindow is answered entirely inside
// the extension (D6) — the same "editor.*-shaped" precedent editor.openDiff/editor.openRangeDiff
// already set (this constant still moves, for the same reason ui.action first did at G10 D9: it is
// the sole compatibility authority, even for an addition the Go server neither emits nor parses).
// The prepare script is written only in-process by Kira Space (P172); repoSettings.set refuses it.
// G26 D17 (2026-09-09): 28 -> 29, for four new Go-served requests (stack.list, preflight.restack,
// stack.restack, stack.cancelRestack), one new event (stack.progress), nine new wire types
// (StackBranchState, StackBranch, StackSummary, StackListResult, RestackBlocker, RestackPlanEntry,
// RestackPreflight, RestackResult, RestackProgress), one new OpRequest kind (stackSet), one new
// OpErrorKind (StackCycle — produced exclusively by stackSet, never by rebase itself, D5), and three
// new UiActionKind members (restackStack, checkoutStackParent, checkoutStackChild). No new
// capability, no new setting, no SQL migration (§8's own explicit non-goals for this phase).
// G28 D17 (2026-09-09): 29 -> 30, for branch-scoped stash — auto-stash on checkout, cross-branch
// apply, auto-detach on worktree conflict, and a durable global stash bucket. One new Go-served
// request (globalStash.list -> {entries: StashEntry[]}, reusing StashListResult verbatim); three
// requests widen their params with an optional scope ('stack'|'global', absent = 'stack') --
// stash.show, preflight.stashPop, preflight.stashBranch; two new OpRequest kinds (globalStashSave,
// globalStashRemove, D10/D11); one new OpRequest field (checkout.autoStash, D3); two new
// CheckoutPreflight routes (autoStash, detachHere, D2) and one new WorktreeAddPreflight route
// (detachHere, D7) — both additive to an existing string-array field, no new wire type; one new
// OpErrorKind (NothingToStash); one new UiActionKind (saveGlobalStash); one new
// RepoSettingsSnapshot leaf (kiraSpace.checkout.autoStash, boolean, default true, read
// client-side only, D16). StashEntry itself widens by two fields (scope, ref; index's own doc
// comment gains the -1 sentinel for a global entry) rather than forking a parallel
// GlobalStashEntry type (D17's own closing argument) -- so, notably, ZERO new wire interfaces.
// No SQL migration (kiraSpace.checkout.autoStash lives in the existing key-value
// git_repo_settings table); no watcher change (refs/kira/** already falls under
// commonDir/refs/**'s existing first classify rule); no new ClassifyOpError stderr row (every
// failure this phase can produce is either already classified or refused host-side before git can
// produce it).
// G-UX D4/D9 (2026-09-09): 30 -> 31, for two graph/review UX fixes' wire impact. repo.pick is
// REMOVED -- the workspace's own folders are now the only source of repositories, so the native
// folder-picker fallback has no wire method left to reach (D4); the Go server never served it
// (grep -rn "repo.pick" apps/kira-studio/ returned nothing before this bump too). One new
// UiActionKind member, toggleSearch (D9) -- extension<->webview only, the Go server neither emits
// nor parses it, the same reason this constant moves for every ui.action-only addition since G10
// D9. No new capability, no new setting, no SQL migration.
// G30 round-1 code review (2026-09-09): 31 -> 32, one new OpErrorKind member, BranchChanged --
// remote.run's pull integrate phase now refuses, rather than silently writing to the wrong
// branch, when HEAD changed between the fetch and the merge/rebase (finding #2). No new request,
// no new capability, no SQL migration.
// G-UX D13 (item 13): 32 -> 33, one new event, connection.changed -- extension<->webview only,
// the Go server neither emits nor parses it, the same reason this constant moves for every
// ui.action-only addition since G10 D9. No new request, no new capability, no SQL migration.
// P5 (2026-09-11): 33 -> 34, one new Go-served request, blame.line (RepoEntry.BlameLine). No new
// event, no new capability, no new UiActionKind member, no SQL migration.
// P7 item 2 (2026-09-12): 34 -> 35, one new Go-served request, working.detail
// (RepoEntry.WorkingDetail), the uncommitted-changes strip's click-through file list. No new event,
// no new capability, no new UiActionKind member, no SQL migration.
// P74 §3.3 (2026-09-15): 35 -> 36, for one new Go-served request, pr.browserUrl (composes a PR's
// github.com URL server-side; params: repoId/number, result: {url: string|null}) and one new
// extension-answered request, pr.openExternal (params: repoId/number, result: {}), which requests
// pr.browserUrl over the socket and hands the URL to the host's own browser-open path -- never
// answered by this Go server itself, the same "editor.*-shaped" precedent editor.openDiff already
// set. One new app.init capability, openExternal (both hosts report true). No SQL migration.
// P75 §2.3: 36 -> 37, for one new host-answered request, graph.revealCommit (params: repoId/sha,
// result: {revealed: boolean}) -- replaces the review row's VS Code-only command: URI anchor with
// a real request both hosts answer locally, never reaching this server. No SQL migration.
// P77 §13: 37 -> 38, four new UiActionKind members -- openTagPicker, openStashPicker,
// openWorktreePicker, openStackPicker -- extension<->webview only, this Go server neither emits
// nor parses ui.action payloads, the same reason this constant moves for every ui.action-only
// addition since G10 D9. No new request, no new capability, no SQL migration.
// P79 finding 4: 38 -> 39, one new host-answered request, link.openExternal (params: {url:
// string}, result: {}) -- opens a commit message body's own URL (linkify.ts) in the OS browser,
// never reaching this server: unlike pr.openExternal the URL is untrusted renderer-visible
// content already, not something this server can compose, so each host only validates the URL's
// own shape before its own OS-open call. This constant moves for the same reason ui.action-only
// additions always have (G10 D9): it is the sole compatibility authority, even for a method this
// server neither emits nor parses. No new event, no new capability (openExternal already gates
// this the same way it gates pr.openExternal), no SQL migration.
// P100 Part 3: 39 -> 40, no shape change at all -- the eleven RepoSettingsSnapshot keys are
// renamed kiraVersion.* -> kiraSpace.* as the extension itself is renamed to Kira Space. A
// wire-compatible extension built before this rename would otherwise silently write and read
// settings under the old key names against a server that has migrated its stored rows to the new
// ones -- deliberately rejected by the version gate instead, the same reasoning every
// version-only bump in this history has already established (G10 D9).
// P111: 40 -> 41, one new PullPreflight field (rebaseMerges) and one new remote.run param
// (rebaseMerges) -- the pull executor stops re-deriving --rebase-merges from config and takes it
// from the preflight the client already ran. No new request, no new event, no SQL migration.
// P150: 41 -> 42, one new request (review.snapshot) -- the stored text a file had when it was last
// reviewed, so a review window's since-review left side survives a rewritten branch. No new
// event, no SQL migration beyond review.db's own pinned column.
// P172: 42 -> 43, two breaking changes. repoSettings.set refuses the kiraSpace.worktree.prepareScript
// patch leaf with E_READ_ONLY (the prepare script is written only in-process by Kira Space), and
// settings.setGitPath is removed (the git path is set only in Kira Space's own Settings).
// P173: 43 -> 44, one new request (graph.reportFailure), one new event (autoFetch.changed) and one
// new StatusSummary field (autoFetch) -- the toolbar's "Auto-fetch stopped" marker and the
// server-side record of a corrupted graph stream. The frame cap rises 8 -> 32 MiB on both sides.
// P178: 44 -> 45, behaviour changes. A socket client never receives credential.request (its
// prompts open in Kira Space) and git.sock refuses credential.provide with E_READ_ONLY. Only the
// native stream still uses both. git.sock also refuses repoSettings.set with E_READ_ONLY
// (repository settings are edited in Kira Space only), and app.init's capabilities (composed
// host-side) gain editRepoSettings. The kiraSpace.log.level repoSettings leaf is removed (it was never read). app.init gains dateFormat (Kira Space's appearance.dateFormat).
// P243 Part 2: 45 -> 46, breaking. The VS Code extension is gone: worktree.prepare,
// worktree.cancelPrepare, worktree.openWindow, editor.resolveConflict, the worktree.progress,
// settings.changed and connection.changed events, and app.init's host, settings and capabilities
// fields are removed. The extension-injected params (scope and pageSize on graph.loadMore and
// graph.stream, baseCandidates on review.resolveBase, strategySetting on remote.pullPreflight)
// are removed; the server reads the stored kiraSpace.* repo settings.
// P246: 46 -> 47, breaking. credential.request and credential.provide are removed: a native stream
// connection's credential prompts go to the app's credential relay, tagged with its window.
// P257: 47 -> 48, breaking. FileChange loses additions, deletions and isBinary; the file lists
// (commit.detail, stash.show, working.detail) read --name-status only.
// P286: 48 -> 49, additive. One new request, review.comment.setState; review.comment.add takes an
// optional kind; every comment carries kind, state, reply, repliedAt and sentAt.
// P318: 49 -> 50, breaking. The PR requests are renamed for any code host: commit.resolvePr ->
// commit.resolveChangeRequests, branch.resolvePr -> branch.resolveChangeRequest, pr.browserUrl ->
// changeRequest.browserUrl, pr.openExternal -> changeRequest.openExternal. GhStatus, PrRecord and
// PrLookupResult become HostStatus, ChangeRequestRecord and ChangeRequestLookup; the lookup's prs
// and gh fields become changeRequests and status. kiraSpace.github.enabled (boolean) becomes
// kiraSpace.gitHost.provider (string). Two new requests, gitHost.info and gitHost.providers, name
// the host a repository resolves to and every registered host.
// P352: 50 -> 51, additive. Two new requests, branch.changeRequestIndex (the persisted change
// request index of a repository) and branch.syncChangeRequests (sync it now). repo.changed gains
// the kind changeRequestsChanged. ChangeRequestRecord gains draft, reviewDecision and checksState.
// P358: 51 -> 52, additive. One new request, review.markMany (batch whole-file mark).
// P369: 52 -> 53, breaking. GitHostInfo loses fileReviewSync (GitHub viewed sync removed).
// P370: 53 -> 54, additive. New requests changeRequest.reviewState, changeRequest.submitReview and
// changeRequest.comment (review a change request, comment on its conversation).
// P392: 54 -> 55, additive. 'changeRequest.reviewState' gains optional 'number' (a known change
// request skips the branch lookup).
// P413: 55 -> 56, additive. New request review.comment.edit (replace a comment's body and kind).
// P418: 56 -> 57, additive. ReviewComment gains origin (commit | working); CommentAnchor gains
// uncommitted (a comment written on uncommitted worktree content).
// P464: 57 -> 58, additive. New request changeRequest.lineComment (comment on lines of a file at a
// change request's head).
const ContractVersion = 58

// Protocol is the handshake envelope's own version (SPEC §3.3's "protocol":1), distinct from
// ContractVersion — it never changes unless the hello/ready exchange itself is redesigned.
const Protocol = 1
