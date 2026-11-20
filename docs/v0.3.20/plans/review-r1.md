# v0.3.20 code review, round 1

Base: `754f1ac0d` (main, merged v0.3.19, reviewed in two rounds). Scope: `git diff 754f1ac0d..HEAD`
(P694 to P698, release.yml install retry, full-run fixes). Reviewer: Opus. Findings only, no fixes.

Counts: 3 important, 0 pre_existing, 3 nit.

## R1-1 (important): watcher files the user's external edits as the live agent's, then hides them

Files: `apps/kira-space/internal/ade/taskwatch.go:303-313` (`run`), `apps/kira-space/internal/ade/detailssubmit.go:399`
(`DiskChanges` skips paths with a `fileErrors` entry).

Scenario: manual TUI session (grant lives for hours) or headless run on a task. User opens the task folder
(UI open/reveal action) in an editor and changes an older note, a handed handover, another author's link
or an accepted proof. fsnotify fires; with one live caller the pass runs `SyncTaskFiles` as that agent with
reason `""`. Ask-first change: result `invalid`, `markInvalid` marks the row, `recordFileErrors` writes a
`fileErrors` entry naming the agent, and `DiskChanges` now drops the path from the disk banner. The user
sees no banner for own edit. The agent is told by prompt and README to fix `fileErrors`: it rewrites the
file to the accepted copy (user's edit lost) or calls `submit` with an invented reason (user's edit sent
to approval under the agent's name). Free kinds (latest note, own links, draft) are accepted silently with
`AcceptedBy` set to the agent session. Plan P695 "(a)" only covers user edits while no agent runs and
says attribution needs one writer; one live grant does not prove the write came from it.

Suggested fix: attribute only writes the agent can be tied to. Options: (1) keep the watch pass but never
hide or record `fileErrors` for an ask-first result whose path the agent has not touched through its own
tools; (2) skip the banner hide (`hasFileError`) so a misfiled user edit still shows, and word the
`fileErrors` entry so the agent does not revert content it did not write; (3) record per-session writes
from harness PostToolUse paths where available (Claude hooks drop `tool_input`, so check other sources)
before attributing. Add a flow test: user writes an older note while a run is live; assert banner shows
it and the agent's `fileErrors` stays empty (or the chosen behavior).

## R1-2 (important): `fileErrors` entry under a path that no longer exists never clears

Files: `apps/kira-space/internal/ade/detailssubmit.go:243-278` (`recordFileErrors`),
`apps/kira-space/internal/ade/detailssubmit.go:339-344` (`SyncTaskFiles` drops gone, unindexed paths),
`apps/kira-space/internal/taskfiles/submit.go` `skipPath` (hint results carry `hintPath`, never a row).

Scenario: agent writes `proofs/Orders_1/proof.md`. Pass: `skipPath` returns `invalid` at
`proofs/Orders_1/` (hint path, no index row). `recordFileErrors` stores it; named path was
`proofs/Orders_1/proof.md`, so the hint key is only cleared by a later result for that exact key. Agent
renames the folder to `proofs/orders-1/` as the hint says (or deletes it). Events queue
`proofs/Orders_1` and its files; `SyncTaskFiles` drops them (Lstat fails, no row). New folder is
accepted. The `proofs/Orders_1/` entry stays in `state.yaml` `fileErrors` and in the banner skip list
until the last grant releases; the finish flush (`paths` nil) does not clear it either. The agent, told
to read `fileErrors` after each edit, keeps seeing an error for a folder that is gone. Same for a file
`proofs/<slug>` (no slash) removed again, and for an explicit `submit` path such as `../x` (stored as an
entry the agent cannot clear). The comment on `recordFileErrors` ("a file removed again ... loses its
entry too") does not hold, since `SyncTaskFiles` filters such paths out before they are named.

Suggested fix: after each submit, drop every entry of the task whose path is neither present on disk nor
indexed (one `Lstat` per entry, entries are few), or keep queued gone paths in `SyncTaskFiles` when they
have a `fileErrors` entry (and map a removed folder to its `hintPath`). Do not record entries for
explicit-submit paths that fail `ValidRel`; the call reply already answers them.

## R1-3 (important): required-services gate fails closed and silent on take-over paths

Files: `apps/kira-space/frontend/src/ade/v2/state/adeTakeOver.ts:82`,
`apps/kira-space/frontend/src/ade/v2/state/useServicesGate.ts:11`,
`apps/kira-space/internal/ade/services.go:58-61,107-110`.

Scenario: `RequiredServices` returns an error when any repo with a worktree service fails `openRepo` or
`BranchInventory` (repo folder moved, git failure), or `CodeRepos.Get` / `listScripts` fails: one bad
row fails the whole call. `fetchRequiredServices` throws and `ensureServices` rejects. Plan P697 line
194 says callers already turn errors into `ui.actionError`. True for `useTaskAction.run` (try),
`useTaskLaunch`, `useNeedsAction`, `AdeBranchRow`, `AdeBranchPanel`, `AdeReviewAgentPanel`. Not for
`takeOver.request`, now throwing at line 82: callers `AdeSessionActions.vue:45`, `AdeSessionsList.vue:55`,
`AdeAllSessionRow.vue:74`, `AdeStepBlock.vue:381`, `useSessionMenu.ts:98` and `useTaskAction.ts:98`
(returned before the try) do not catch. Take over and Resume then do nothing, no inline error, no
toast (P698 left toasts to ops), plus an unhandled rejection. Before P697 those buttons could not fail
this way.

Suggested fix: make the gate fail open on a read error (log, show the error in the dialog or inline,
let the start go on), or catch in `takeOver.request` and route to `ui.actionError` like `run`. In
`RequiredServices`, turn a per-branch worktree read error into that row's `invalid` state with the
error as `problem` instead of failing every row.

## R1-4 (nit): `fileErrors` not cleared at release when the watcher never started

File: `apps/kira-space/internal/ade/taskwatch.go:84-86,118-123`.

`newTaskWatch` failure (fd limit, missing folder) leaves no `watches` entry, so `unwatchTask` returns
early and `clearFileErrors` never runs. Errors recorded by explicit submits and the finish flush stay in
`state.yaml` until restart or archive, and stay hidden from the disk banner.

Suggested fix: in `unwatchTask`, when no watcher exists and no other grant of the task is live, call
`clearFileErrors` (track live grants per task separately from watcher creation).

## R1-5 (nit): debounce has no max wait

File: `apps/kira-space/internal/ade/taskwatch.go:263-267`.

Every event resets the 300 ms timer. An agent that keeps writing within 300 ms (a script writing proof
data in a loop, an editor autosave) defers every pass until it stops, so `fileErrors` stays stale for
the whole burst. Suggested fix: cap the delay (for example fire at most 2 s after the first queued path).

## R1-6 (nit): kqueue fd cost per watched task folder

File: `apps/kira-space/internal/ade/taskwatch.go:108`.

On macOS fsnotify uses kqueue: one fd per watched folder and per file in it, up to the 500-file task cap,
per task with a live grant. Several busy tasks reach thousands of fds. Architecture notes the cost; G9
(v1.3) moved the git watcher to FSEvents for this exact reason (`gitclient/watcher_fsevents_darwin.go`).
Suggested fix: reuse the FSEvents backend on darwin for task folders, or one shared watcher, before task
counts grow.

## Checked, nothing real

- Watcher concurrency: `fire`/`stop` ordering under `t.mu` with `passes`, final pass off the task lock,
  `stopTaskWatches` after `agent.Close` (no new grants), concurrent passes serialized by `taskMu`: no
  race or deadlock found.
- Path traversal and symlinks: `relOf` rejects `..`, store `lstat` refuses a symlink at any component,
  `O_NOFOLLOW`, `ValidRel` on explicit paths: no escape.
- Approval bypass: latest note/handover/proof freedom matches the documented "any author" decision;
  create and delete stay ask-first; collision still forces a reason.
- P696: spot-checked removed footer handlers against each dialog's `update:open` path (Stash, Stack,
  WorktreeList, Update, Stage, Run, ConfirmDialogView): X and Esc reach the same handler; hidden-X
  dialogs keep their dismiss button.
- P698: every removed toast was loading/success; client and slow backend failures still toast.
- Frontend rules: new files are `<script setup lang="ts">`, Tailwind, shadcn-vue primitives, Pinia store
  `adeServicesGate` holds one concern, server state via TanStack Query.
- CLI-credential rule, secrets, minor-bump legacy rule, release.yml retry: no issue.
