# v0.3.21 code review, round 2

Base commit: `a69a180e1` (same as round 1). Scope: `git diff a69a180e1..HEAD` at `84e696290`, round 1 fixes
`c0be6f455`, `ea5dd3a34`, `be9851d48` included. One Opus reviewer, three dimensions. Findings only, no fixes.

Counts: 3 important, 2 nit.

Checked, no finding: watcher `SkipDir` one-level `worktrees/<name>` admit (fsnotify adds each dir; FSEvents
stream on `commonDir` covers nested `gitDir`); fswatch scope, sink, backends, Debouncer stop and Close order;
taskwatch and folderwatch teardown (no goroutine or watcher leak found); exclusive backup write (`os.Link`
`EEXIST` maps to `fs.ErrExist`, temp name removed on both paths; home volumes have hard links); agent paths
never discard (`worktree.remove` and `UpdateTaskRepo` pass `""`, refused on dirty); lock order task then repo
in `detachLinked` and archive; restore marks (quit `MarkRestore`, crash `StopAllTaskRunning`, one-shot
`TakeRestoreSessions`) and migrations 0103/0104 on upgrade; P703 author rules against SPEC; PR glyph mapping and
upsert change predicate (mergeable refreshed by sweep); mobile run projection; file service containment;
every new or changed `.vue` is `<script setup lang="ts">` with no `<style>` block; new unit tests within bar.

## R2-1 (important) Board discard token goes stale and nothing refreshes it: discard refused in a loop

Files: `apps/kira-space/internal/ade/board_facts.go:380`, `apps/kira-space/internal/ade/taskbranches.go:136-145`,
`apps/kira-space/internal/gitsession/worktree.go:234-273`,
`apps/kira-space/frontend/src/ade/v2/panel/AdeTaskBranchMenu.vue:104`,
`apps/kira-space/frontend/src/ade/v2/panel/AdeTaskBranchForm.vue:61`.

Defect: branch menu and change-branch form send `Branch.dirtyToken` from the board snapshot. The token hashes
size and mtime of every dirty file (and every file under an untracked dir). The desktop board is push-only
(`staleTime: Infinity`, no focus refetch, `queries.ts:93`), and pushes come from Kira writes and the git-dir
watcher (`HEAD`, `index`, refs). Editing a file that is already modified, or adding a file inside an untracked
dir, writes neither, so no push and no rebuild. `discardRefusal` then returns "changed since you reviewed them"
without `notifyBoard`, and the client neither refetches nor invalidates on that error.

Failure: worktree shows `M src/a.go`. User saves `src/a.go` again in an editor, then picks "Discard changes and
remove" (or change branch with discard). Server token differs, refusal. Menu still shows the same list and
the same old token; every retry is refused until some unrelated board push (manual Refresh, a commit) happens.
Same for a new untracked file: list and token both stale, same loop.

Fix: on a token mismatch push the board (`notifyBoard`) before returning the refusal, so the row reloads
with the current list and token; and/or invalidate `boardKey` in the two mutations' `onError`. Consider also
R2-2's narrower token, which makes edits to listed files stop mismatching at all.

## R2-2 (important) Archive with a live agent: token check before stopping work refuses in a loop, or aborts mid-teardown

Files: `apps/kira-space/internal/ade/archive.go:183-208`, `apps/kira-space/internal/ade/archive.go:317-327`,
`apps/kira-space/frontend/src/ade/v2/dialog/flow.ts:309-320`.

Defect: `ArchiveTask` compares each `BranchRisk.dirtyToken` from the dialog while the task's runs and TUI
agents still run, then stops work, then `removeWorktreeLocked` compares the same token again against a fresh
preflight. The token moves with every agent save (size and mtime), so it is a moving target until work stops.

Failure, case A: an agent edits files every few seconds. User opens Archive, dialog lists `M a.go`, clicks
"Delete anyway" five seconds later: refused. Reopen, same again. Archiving such a task needs the user to stop
the agent by hand first, though archive exists to stop it. Case B: the agent saves between the pre-check
(`archive.go:184`) and `stopTaskWork` (`:194`). Requests are already abandoned, runs and setups stopped,
terminals closed, maybe an earlier repo's worktree removed, then `removeWorktree` refuses and `ArchiveTask`
returns the error with the task live and half torn down. A dirty file that appears in a worktree the dialog saw
clean hits case B too (refusal with count 0 at removal).

Fix: stop work (and close terminals) before the token comparison, then compare once and, on mismatch, return
the refusal (work already stopped, so the reopened dialog's token holds). Narrowing the token to what the
dialog shows (kind, XY, path, original path; untracked dir as one path) also stops edits to listed files from
invalidating it; new paths would still refuse.

## R2-3 (important) Site docs still describe `submit` and user-approved branch requests

Files: `site/src/content/docs/docs/reference/mcp-tools.md:36,38,44`, `site/src/content/docs/docs/space/mcp.md:24`,
`site/src/content/docs/docs/space/agents.mdx:95`, `site/src/content/docs/docs/space/proof.mdx:17,37`.

Defect: P703 removed the `submit` tool and its `reason`; P702 made `branch.request` apply new branches and
worktrees at once (only adopt asks) and added `worktree.remove`. The public site still lists `submit` with
`paths` and `reason`, says agents call `submit` for links and proofs, says "you approve each request in a
dialog" and that `branch.request` asks the user to approve. No site file changed in either phase.

Failure: a user reading the docs expects a branch approval dialog that never comes and a `submit` tool that
does not exist; a proof page tells them refusal reasons arrive as a `submit` answer.

Fix: reword those lines to the shipped behaviour (watcher applies saves, approvals only for another agent's
content; `branch.request` asks only for adopt; list `worktree.remove`).

## R2-4 (nit) `DirtyToken` walks every file under untracked dirs and modified submodules on each board build

Files: `apps/kira-space/internal/gitsession/worktree.go:251-273`, `apps/kira-space/internal/ade/board_facts.go:380`.

Defect: `diskStamp` runs `filepath.WalkDir` plus `Info` for every file below an untracked directory entry
(and below a modified submodule path, which is a dir). `git status --untracked-files=normal` deliberately
avoids that walk. The board build computes the token for every linked worktree on every rebuild (any
`notifyBoard`, snapshot fresh 2 s), though it is only needed when the user discards.

Failure: a worktree with an untracked, unignored `node_modules/`, `.venv/` or vendored clone: tens of
thousands of `lstat` calls per board build, repeated through an agent's busy turn.

Fix: compute the token over the status entries only (see R2-2), or stamp an untracked dir by its own entry,
or compute it lazily when the discard UI opens.

## R2-5 (nit) Removal recomputes the same preflight up to three times

Files: `apps/kira-space/internal/ade/archive.go:116,318`, `apps/kira-space/internal/gitsession/worktree.go:412`.

Defect: `branchRisk` runs `WorktreeStatus` and then `WorktreeRemovePreflight` (a second status plus token walk);
`removeWorktreeLocked` runs the preflight again only to compare tokens, and `RunOp` -> `prepareWorktreeRemove`
runs it once more and compares again. Archive per branch: pre-check `branchRisk` (2), lock preflight (1),
op preflight (1).

Failure: four `git status` spawns plus four token walks per worktree per archive or detach, with R2-4's walk
cost each time.

Fix: in `removeWorktreeLocked` pass `discardToken` straight as `OpRequest.ConfirmToken` and let
`prepareWorktreeRemove` be the one check (map its `ConfirmationRequired` to the same refusal text); in
`branchRisk` take the token from the preflight instead of hashing `status` a second time.
