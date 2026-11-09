# v0.3.6 code review findings

Base commit: `fa9232b6708ec952fe83557c018a85058d6298ef` (main at chapter start, no earlier review). Scope:
`git diff fa9232b..76520d4b4`, phases P580-P589. Plan docs and `docs/v0.3.6/` itself not reviewed.

Counts: High 0, Medium 5, Low 5.

## Group A: config sync repo shared text (P585), `configsync/service.go`, `repocfg/shared.go`, `internal/gitremote`

### 1. Medium (security): shared-text export skips credential gate

- Where: `apps/kira-space/internal/configsync/service.go:535` (`RepoSharedText`), `apps/kira-space/internal/repocfg/shared.go:153` (`SharedText`).
- Defect: export renders `prepare_script` and `environments` into text the user copies and commits to the shared config repo. It never runs `credentialProblem` / `scriptCredentialProblem`. The import side (`configsync/repos.go:557`) refuses the same content.
- Failure: user has a literal token in a prepare script or env value. "Copy shared" puts it on the clipboard, user commits it, secret lands in a shared git repo. The import gate only fires afterwards, on other machines.
- Fix: run the same gate on the rendered entry before returning; refuse with the gate's message and name the offending key.

### 2. Low: `RepoSharedText` matches unnormalized id

- Where: `apps/kira-space/internal/configsync/service.go:535-555`.
- Defect: frontend passes repo `path`; lookup compares it raw against target ids built with `repocfg.Key`. A path differing only in form (trailing slash, case on case-insensitive FS, symlinked root) finds no target.
- Failure: "Copy shared" errors "not found" for a registered repo.
- Fix: normalize input with `repocfg.Key` before matching.

### 3. Low: `gitremote.Key` treats Windows drive paths as scp remotes

- Where: `internal/gitremote/gitremote.go:12` (scp-form branch).
- Defect: `C:/work/repo` or `C:\work\repo` parses as host `c`, path `work/repo`. A local-path origin becomes a shareable origin key.
- Failure: two machines with unrelated local repos at the same drive path share one repo config entry (scripts, environments) through config sync.
- Fix: reject a single-letter host followed by `:/` or `:\`, and any input containing a backslash, as not shareable.

## Group B: config sync refresh cost (P585), `configsync/service.go`, `configsync/repos.go`

### 4. Medium (perf): `RefreshAsync` spawns uncoalesced full plans

- Where: `apps/kira-space/internal/configsync/service.go:510`; wiring in `apps/kira-space/internal/appwire/appwire.go` (CodeWorkspace `OnReposChanged`, ADE `OnRepos`).
- Defect: each call starts a goroutine that queues on `opMu` and runs a full plan. `OnRepos` fires per repo-config file change (folderwatch, `RepoConfigChanged`); `Apply` itself calls `Repos.Reload()`, which echoes back into `RefreshAsync`. Each plan also calls `originsOf` (`configsync/repos.go:208`), which spawns one `git remote get-url` process per registered repo; `takeSession` repeats it.
- Failure: bulk change (branch switch in repos-config folder, apply of N entries, sync pull) queues N goroutines, each running R git processes serially: N x R process spawns, UI refresh lag, goroutines pile up under `opMu`.
- Fix: coalesce: one pending flag or short debounce timer, so at most one running plus one queued refresh. Cache origins per refresh pass (or per repo with mtime of `.git/config`) and reuse in `takeSession`.

## Group C: rename journal (P584), `adeflow/rename.go`

### 5. Medium: single journal slot overwritten after failed finish

- Where: `apps/kira-space/internal/adeflow/rename.go:213` (`writeJournal`), `:399` (`finishStepRename`).
- Defect: on retarget failure `finishStepRename` keeps the journal for recovery. Journal is one file; next rename calls `writeJournal` and overwrites it unconditionally. Recovery only runs at startup (`RecoverRename`, `:543`).
- Failure: step rename A fails mid-retarget (old file present, some workflows rewritten). User renames step B in same session. A's journal is gone; on restart nothing finishes A: duplicate step file, workflows split between old and new id.
- Fix: refuse a new rename while a journal exists, or run recovery first and fail if it cannot complete. Alternative: per-rename journal files, all recovered at startup.

### 6. Low (security/robustness): journal workflow names not validated

- Where: `apps/kira-space/internal/adeflow/rename.go:555` validates `Old`/`New` with `fileNamePattern`; `:401` joins each `j.Workflows` entry with `r.Dir` unchecked.
- Defect: a corrupted or hand-edited `rename.json` with `../x` in `workflows` makes recovery rewrite a file outside the workflows dir.
- Failure: low likelihood (file under `<home>/ade`), but recovery writes to an arbitrary path the journal names.
- Fix: in `RecoverRename`, reject the journal unless every `Workflows` entry matches `fileNamePattern`.

## Group D: review sync (P581), `ade/review_sync.go`

### 7. Medium: `ReviewFetch` holds repo mutex across network fetch with askpass

- Where: `apps/kira-space/internal/ade/review_sync.go:119-124`.
- Defect: `repoMutex(sb.CodeRepoID)` is held while fetching from remote, which can block on an interactive askpass prompt or a slow host.
- Failure: while a credential prompt waits, adopt/detach, worktree setup, archive `removeWorktree` and rebase on the same repo all block; the UI looks hung.
- Fix: fetch outside the repo mutex (git's own ref locking covers concurrent fetch); take the mutex only for the local state read afterwards. Or bound the fetch with a timeout.

### 8. Medium: `ReviewUpdateLocal` fast-forwards a busy task worktree

- Where: `apps/kira-space/internal/ade/review_sync.go:170` and `checkFastForward` (`:207`).
- Defect: check covers in-progress git ops and HEAD position, not task activity. `merge --ff-only` rewrites files in a worktree where an agent run, agent TUI session, prepare script or smart script may be working.
- Failure: agent is editing in the worktree; user clicks "Update local"; files change under the agent, which then commits or reasons against mixed state. A dirty-free but busy worktree passes every current check.
- Fix: reuse the `donorBusy` check from `ade/branchadopt.go` (running run, setup, claim, live session) for task-owned worktrees and refuse with a clear message.

## Group E: ADE attachment lookups (P583), `ade/worktreedir.go` and callers

### 9. Low (perf): `rowWorktree` recomputes repo-wide data per row

- Where: `apps/kira-space/internal/ade/worktreedir.go:118` (`rowWorktree` / `resolveAttachment`); callers `apps/kira-space/internal/ade/archive.go:34` (`linkedBranches` loop), `apps/kira-space/internal/ade/rebase.go:219` and `:355` (`planRebase` then `rebaseBlockers`, same rows twice).
- Defect: each call runs `BranchInventory` (git), `CodeRepos.Get`, the full `BranchesLive` query and an `Lstat`. Loops call it per row; rebase planning calls it twice per row.
- Failure: task with many branches or a stack rebase: O(rows) git inventory calls and full-table queries per action; slow archive-risk dialog and rebase plan.
- Fix: load inventory and live-row ownership once per repo per call (as `board_facts.go` `loadOwners` does) and pass it into a pure resolver.

## Group F: frontend

### 10. Low: unhandled rejection in shared-text copy

- Where: `apps/kira-space/frontend/src/repo/RepoFilePane.vue:37` (`onCopyShared`).
- Defect: `await sharedText.mutateAsync(...)` has no catch; mutation error is already shown via the query's error state.
- Failure: every failed copy (e.g. finding 2's not-found) also raises an unhandled promise rejection; global handler logs/toasts twice.
- Fix: use `mutate` with `onSuccess`, or wrap in try/catch and rely on the mutation's error state.

## Areas with no findings

- P580 API rules: `apiaccess` deny-wins precedence, allow-required for non-empty policy, per-hop redirect check with original method (`agentapi/resolve.go` `redirectGuard`), fail-closed reads in `bridge/dbmcp_api.go` and `storage/repos/settings.go`. No defect.
- P581 fast-forward: `FastForwardRef` uses `update-ref` with old-value check; ff-only semantics hold (busy gap is finding 8).
- P583 adopt/detach ownership: `takeLocked` re-checks holder under repo mutex; migration 0094 unique index on `adopted = 0` matches code paths. No defect beyond perf in finding 9.
- P584 crash-safety otherwise: hash-checked `removeOld`, workflow rename backs out on retarget failure, `MigrateNames` marker idempotent, started tasks protected by snapshots.
- P587 MCP script `mode` short/long and timeout defaults: no defect.
- P588 default flips (sandbox off, `db_mcp` absent = on, hermetic false): consistent with spec; explicit values in existing files keep their meaning; running tasks read settings per launch (`TaskBoard.hermetic`). No defect.
- P589 registry and kinds: no defect.
- U79 external-open guard: backend validates http(s) scheme. No defect.
- Repo frontend guidelines (shadcn-vue, Tailwind, VueUse, TanStack Query, `<script setup lang="ts">`): no new violations in changed files.
