# v0.3.19 code review, round 2

Base: `5022eddb3` (v0.3.18 on `origin/main`, same base as round 1). Head: `7018b342c`. Scope: `git diff 5022eddb3..HEAD`, round 1 fix commits `b236253a6..2ad289944` first, then a re-sweep of P690 to P693, `ui-heap` project, `pr.yml`, close and history edits, memory leftovers. Reviewer: Opus, one pass over three dimensions.

Counts: 1 important (carried, round 1 R4 still open), 0 pre_existing, 4 nit.

## Important

### R1. Archive tag still not on origin; PR #52 open (round 1 R4, unfixed)

`docs/ARCHITECTURE.md:3112`, `docs/history/07-v0.3.15.md` (v0.3.19 P690 entry).

`git ls-remote --tags origin 'refs/tags/archive/*'` at review time: empty. Round 1 fix was "push the tag before the PR opens; hold the PR otherwise". PR #52 is open with auto-merge, and the folded history entry itself says the push "must precede the chapter PR".

Failure: PR squash-merges, `chapter/v0.3.19` is deleted, `a08665c6a` becomes unreachable on origin. ARCHITECTURE and HISTORY both point readers at a restore path that does not exist outside this container; the memory module, eval corpus and site pages are lost once the container goes.

Fix: push `archive/memory-v0.3.18` (user action if the proxy keeps answering 403) before PR #52 merges; until then disable auto-merge on #52 and report it held.

## Nit

### N1. Recreated replay ring restarts offsets at 0: a reconnecting phone resumes at a wrong position without reset

`apps/kira-space/internal/mobileterm/broker.go:156`, `:199`, `:271`, `:480`; `apps/kira-space/internal/mobileterm/serve.go:142-147`; client `apps/kira-space/frontend/mobile/terminal/useRemoteTerminal.ts:95-98`.

Before F3 the ring lived as long as the entry, so absolute offsets were monotonic per terminal. Now `SetRecording(false)` and `toDesktop` drop it and `Output`/`Attach` build a new one from offset 0. The phone page keeps `offset` and `known` across reconnects and sends `from=<old offset>`.

Scenario: phone holds a busy terminal, Wi-Fi drops (offline grace, client in `reconnecting`), user turns phone access off and on within the client's ~60 s retry window. Server: `ReleaseAll` drops the ring; output after re-enable fills a new ring from 0. Reconnect with `from=50000`: if new `End() < 50000`, `pump` sets `off = 0`, `ReadFrom` reports no overrun, hello carries `reset: false`; the client keeps its old screen and appends the replay. If new `End() > 50000`, the phone skips the first 50000 bytes of the new ring. Either way a garbled terminal, no error.

Fix: keep a per-entry `ringBase int64` set to the dropped ring's `End()` and have `phoneConn` translate offsets (or give `newRing` a start offset), so an old `from` lands below `start()` and reports overrun; minimum: send `Reset: true` in hello when `from >= 0 && from != start`.

### N2. Stale search start closes the session of a remounted Files tab and cancels its new search

`apps/kira-space/frontend/src/ade/v2/files/state/fileSearch.ts:203-209` (fix `2ad289944`).

The dropped-state check compares state identity, but the cleanup acts by branch id: `cancelSearch(branchId)` and `adeTaskFilesClose(branchId)` reach whatever session the branch has now.

Scenario: Enter in Search, switch tab, switch straight back to Files on the same branch, press Enter again, all inside the first `adeTaskFilesSearch` round-trip (`taskWorktree` plus `openRepo` plus `worktreeOf`). The new mount's state is a different object, so the stale start resolves into the dropped branch: it cancels the new search (cancel is a clean finish, so the partial result shows as complete with no error) and closes the session the new tab just opened.

Fix: in the dropped branch, act only when `!byBranch.has(branchId)` (no new mount owns the branch). When a new mount exists, skip both calls: its next `BeginSearch` supersedes the stale search and its events are dropped by unknown `searchId`.

### N3. Missing parent directory still maps to an internal error instead of "missing"

`apps/kira-space/internal/ade/taskfiles.go:63-69`, `:88`, `:117`; `internal/pathsafe/pathsafe.go:70-73`.

`badPath` (fix `548f197a0`) now passes non-escape errors through as internal. `ValidateRelPath` returns a wrapped `fs.ErrNotExist` from the parent `EvalSymlinks` when a directory on the path is gone.

Scenario: viewer has `src/feature/a.ts` open; the agent deletes `src/feature/`; the next refetch shows "Couldn't open the file" with a raw `pathsafe: resolve ...` message instead of the `missing` empty state. Reveal or Open in VS Code on that path answers E_INTERNAL instead of `ErrTargetMissing`.

Fix: in `badPath` callers, map `errors.Is(err, fs.ErrNotExist)` to `codeworkspace.FileContent{Kind: missing}` in `TaskFileRead` and to `ErrTargetMissing` in `TaskFileTarget`.

### N4. DEV_ENVIRONMENT still lists removed memory tests and eval round

`docs/DEV_ENVIRONMENT.md:544`.

The "Real `claude` tests" spend line names `TestMemoryDistill`, `TestMemoryDistillBridge`, `TestMemoryMcpFollowsDocs`, `TestMemoryGate`, the memory docs test, eval round `round-p297-01` and `KIRA_EVAL_PHASE_CAP_USD`. None exist after P690; the doc describes the current sandbox.

Fix: drop the P296 memory-docs clause and the P297 memory and eval clauses from that line.

## Checked, no finding

- `b236253a6` argv shim: handled before any Wails or lock work in both `main.go` and `flowharness/main.go`; `memory-check` exits 0, others 1; marked v0.3.x-only.
- `56c4b2d3a`: binary untracked, `.gitignore` entry added, double blank line gone. Binary stays in branch history only; squash merge drops it.
- `e34f34686` and `4d360a687`: every `e.ring` access is under `e.mu` and nil-guarded or preceded by creation. `Output` re-checks `recording` under `e.mu`, so no ring survives the sweep; `toDesktop` drop covers the disable order (round 1 N1). Lock order unchanged (`b.mu` released before `e.mu`). A pump on a finished conn reads empty and exits on `Done`; no leak, no busy loop.
- `c986cb855` Monaco: both `mountEditor` calls await the same `loadMonaco` promise and resume in order, so the newer text is applied last.
- P693 F1: `activateAdeStores` runs in `app.runWithContext` before mount or from a sync mode watcher outside any instance; `agentControlKey` provided in `mountApp` covers every window; `gcTime: 0` on candidates and file read; `usePaneResize` flag reset in `onEnd` and `abort`. Task files listing query closes over a string, not a getter, so no new pin.
- F3 wiring: recording set at boot from settings and in `SetEnabled`; the server starts only through those paths, so no running server with recording off.
- `ui-heap` project: excluded from `ui`, Chromium-only, in `test:ui` and `pr.yml` `ui-extra`; step name quoted. Retention spec uses CDP GC plus a WeakRef and a node-count tolerance.
- Close: history block has 5 rows matching SPEC, file at 225 lines, index range updated; guard comment repointed to HISTORY. Visual re-record touches one baseline with a fixed clock.
- Memory leftovers: no code, catalog, go.mod or package.json references outside the argv shim and settings-legacy tests.
- Performance: one atomic load per output chunk; sweep is O(entries) on toggle only. Nothing else found.
