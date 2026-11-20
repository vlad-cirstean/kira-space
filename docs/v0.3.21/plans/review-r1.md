# v0.3.21 code review, round 1

Base commit: `a69a180e1`. Scope: `git diff a69a180e1..HEAD` (P699-P706 plus non-UI full-run fixes). All
commits in range unreviewed, fix commits included. Discovery via `codegraph_explore` (index at
`/home/user/kira-space`), every hit re-read in worktree.

Counts: 1 important, 3 nits.

## Findings

### R1-1 (important): discard token is static basename, not what the user saw

- Where: `apps/kira-space/internal/gitpreflight/worktree.go:211` (`ConfirmToken: filepath.Base(in.Path)`),
  checked at `apps/kira-space/internal/gitsession/worktree.go:390`. Callers:
  `packages/git-ui/src/components/WorktreeList.vue:103`,
  `apps/kira-space/frontend/src/ade/v2/panel/AdeTaskBranchMenu.vue:103-114`,
  `apps/kira-space/internal/ade/archive.go:306` (re-preflights and echoes server's own token).
- Defect: token never changes for a worktree, so a confirmation proves only the path, not the dirty
  set. Discard removes whatever is dirty at execution time. `AdeTaskBranchMenu` derives
  `discardChanges` from board state and sends a flag with no token at all; `archive.go` fetches a fresh
  token server-side, so it always matches. Safety rule (never delete uncommitted work unless explicit
  user action) holds only for work the user saw.
- Scenario: agent keeps writing in the task worktree. Menu shows "Discard 2 changes and remove". User
  clicks; meanwhile agent wrote 3 more files. All 5 are deleted. Same in `WorktreeList`: stale refusal
  from earlier preflight still carries a valid token after new edits.
- Fix: token = hash of dirty set (path, status, size, mtime) computed in preflight. UI sends token it
  displayed (menu included, from its own preflight); server recomputes and refuses on mismatch.
  Archive path passes token from the dialog the user confirmed, not a fresh one.

### R1-2 (nit): row detached before worktree remove can refuse

- Where: `apps/kira-space/internal/ade/worktreeremove.go:70-96` with
  `apps/kira-space/internal/ade/taskbranches.go:101-112`.
- Defect: `RemoveWorktree` checks `branchRisk` clean, then `removeRowBranch(..., false)`; `detachRow`
  calls `DetachBranch` first and `removeWorktree` second. `removeWorktree` re-preflights; tree turning
  dirty in between makes it refuse after the row is already gone.
- Scenario: agent asks removal; another agent or user edit lands in that window. Task loses the row;
  worktree with uncommitted work stays on disk, unlisted in any task. No data loss, but orphan only
  findable via `git worktree list`.
- Fix: preflight and remove under repo lock before `DetachBranch`, or re-attach row when
  `removeWorktree` refuses.

### R1-3 (nit): backup no-clobber now racy

- Where: `internal/agentport/backup.go:34-47`.
- Defect: previously `O_EXCL`. Now `Write` with `IfVersion: &absent`; `fileservice` compares before
  rename (documented race). Doc comment "It never replaces a backup" no longer strictly true.
- Scenario: two concurrent ports of same agent config pick same backup name; both pass absent check,
  second rename replaces first backup.
- Fix: add exclusive-create primitive to `fileservice` (link temp to target, or `RENAME_NOREPLACE`
  where available) and use it here.

### R1-4 (nit): macOS repo watcher no longer sees other worktrees' state

- Where: `apps/kira-space/internal/gitclient/watcher.go:131-141`; comment `:85-95`.
- Defect: `SkipDir` admits `worktrees` but skips `worktrees/<name>`. Old darwin backend was recursive,
  so FSEvents saw `worktrees/<name>/HEAD` and `index`. Now no backend does; comment implies those
  writes are classified.
- Scenario: checkout or commit in a linked worktree (HEAD/index under `worktrees/<name>/`) does not
  refresh worktree list status until another event or rescan.
- Fix: admit depth-1 `worktrees/<name>` dirs (not deeper), or correct comment and accept the gap.

## Dimensions with nothing further

- Architecture/security: beyond R1-1, none. `fileservice` path policy (rooted `os.Root`, Contain/Refuse,
  PlainOnly), G8 allowlist and G7 watch-lib guard sound. No credential touched (G5 scope unchanged).
- Correctness: P701 session restore (migration 0103, quit/crash marks, takeover guard), P703 approval
  table and collision rule, P704 Files window, P705 phone projection (keep-list; no cwd, command,
  prompt or output), P706 PR glyph (migration 0104, schema registry, githost mapping): none found.
- Performance: none worth a finding. `fswatch` scope `forget` scans decided map per dir event; fine at
  current tree sizes.
- Standing rules: Vue files `<script setup lang="ts">`, Tailwind/shadcn-vue/TanStack/Pinia used, no new
  dependency, unit tests meet bar (`fswatch/sink_test.go` guards overflow rules). None found.
