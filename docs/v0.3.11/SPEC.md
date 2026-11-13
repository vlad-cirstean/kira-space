# v0.3.11

Agents module performance: speed, latency and responsiveness, backend and frontend, including Kira Space MCP tools. Phases P622 to P624 run as parallel streams A, B, C (batch requested by user). Highest prior phase: P621. Two code review rounds after the full non-UI run.

Size survey (non-test): BE `ade` 24k lines/70 files, `bridge` 15k, `harness` 6.4k, `terminal` 1.4k, `leases` 0.5k; FE `ade/v2` 34k lines/290 files, approvals 0.6k.

| Phase | Title | Status |
|-------|-------|--------|
| P622 | Backend board, tasks, orchestrator, storage | Done |
| P623 | Kira Space MCP tools and agent runtime | Open |
| P624 | Frontend agents UI | Open |

Each phase: several Opus auditors run in parallel (read-only, `codegraph_explore` mandatory), findings go into the phase plan, then one Sonnet implementer per stream fixes them.

Cross-cutting requirement (user): cover latency, not only throughput. Slow work (git, DB, MCP tools, agent steps) may move async, but every such operation shows feedback while running and when finished (running status where the user looks, completion and failure notice). Each auditor lists: slow operations with no feedback, blocking calls that should go async, feedback surface to use. Reuse `adeOps` store, `AdeOpStatus` chip and vue-sonner toasts from P618; extend, never build a parallel mechanism. Measure before and after only where a claim needs it.

## P622 Backend board, tasks, orchestrator, storage (Stream A)

- Scope: `apps/kira-space/internal/ade`, repos, bridge ADE services. Auditors: A1 board, tasks, branches; A2 orchestrator, steps, review, workflows, storage.
- Targets: query counts, git facts per call (`Board()` cost), lock hold times, event fan-out, DB writes on read paths, calls that block the UI and need async plus feedback.

Result: Done. Plan `plans/P622.md`, 16 planned commits plus two test fixes. Storage: migration 0100 index `ade_runs_latest` (plan shows a covering index), `AdeTaskRepo.CountReturns` replaces full run scans in `wireRun`, `AdeLogsRepo.Append` trims with one window-function cutoff and one delete (boundary test against a brute-force sum). Board: `Board()` serves a shared snapshot keyed by a gen (fresh 2 s) and joins one tracked build per gen; pushes coalesce per 30 ms into `adewire.BoardChangedEvent{refs, taskIds}` (bridge, appwire, mobile SSE pass it through; `notifyBoardNow` keeps handover's board-before-prompt order); lease order reads the snapshot stale-while-revalidate; one shared load of configs, code repos and live branches per build and a repo-config cache; task-scoped `loadTaskCtx` through `BranchesOfTask`; board marks written after assembly; ahead/behind, ancestry and `gitsession` entry-meta caches; per-repo 1 s facts deadline with last-good overlay (`RepoState.updating`, contract `ade-repo-updating`, flow `repoupdating_test.go`); pair conflict checks in the background (`Pair.checking`); workflow decode, history and per-task index memos plus `adeflow.Reader.Get` memo; change requests resolved in parallel per repo. Tasks: `Refresh` holds the repo lock for the fetch only and returns after branch facts (PR merge marks, conflict checks and tracked updates finish in a tracked goroutine, then push; `WaitRefreshTails` for flow tests); `queueRun` reads one setup; `ensureWorktree` seeds the attach cache with its inventory; worktrees prepare in parallel across repos; singleflight (`opdedupe.go`) joins identical in-flight `StartRun`, `Approve`, `RetryRun`, `ResumeRun`, `SetTaskBranch`, `AddExistingBranch`, `ForcePush`, `SendReview`, `FinishReview`; `SendReview` gathers outside the task lock, rechecks under it, retries once, then `ErrStale`. Benchmarks in `docs/PERF.md` section 2.16: `BenchmarkBoardWarm` 34.8 ms and 21 spawns/op at baseline, 9.1 ms and 7 spawns/op at head; `BenchmarkBoardHit` 0 spawns/op, about 70 ns.

Deviations: commit 6 invalidates the snapshot after a clean mark write but does not push the board, where the plan calls `notifyBoard()`; wire content does not depend on the stored marks, so a push would only refetch identical data. Dedupe keys carry the full JSON args after the kind (plan: `<kind>:<target>`), because `StartRun`, `SetTaskBranch` and others take inputs beyond the target; `SendReview` and `FinishReview` key on the args (run id, route, target) not the task id. Lease order also rebuilds after 5 s without a board change, for settings that never announce one. Two tests relied on the old synchronous behavior and now poll: `board_test.go` conflict checks after `Refresh`, `integration_test.go` out-of-band git moves (invalidates like the repo watcher). Benchmark reports spawns only (harness hides its `*sql.DB`). Checks: `go test -race ./internal/ade/...`, storage, gitsession, adeflow reader, bridge, mobileweb, flows `adeflow` and `mobileflow` pass; hook clean per commit.

Left out, with reasons: SQLite read pool (shared `sqlitex` with Studio, read-after-write audit of every repo method, no tested rollback; Known open item added). History off the board wire (changes contract JSONs and Stream C readers: ordering dependency; the history build is memoized instead; follow-up phase: lazy history fetch, A and C together). Dropping `repoMutex` around `ForcePush` (`RunRemote` refuses instead of waiting; only double-submit dedupe lands). One `taskCtx` threaded through the completion chain (goes stale between steps that write; a reload is now three indexed reads). Full async `StartRun` with errors on the run row (36 flow files and the agent `request_branches` path read its error synchronously). Stopping `scheduleBoard` in `emitRuns` (board fields derive from runs; `taskIds` narrows the refetch instead). `RunningRebaseOn` SQL filter (rows bounded by running rebases). Pruning `taskMus` (about 100 bytes per task; needs refcounting). New oplog ops for async user calls (`oplog.Record` has no `BranchID`; needs a migration; calls stay awaitable).

## P623 Kira Space MCP tools and agent runtime (Stream B)

- Scope: `agentmcp`, `llmprompts`, `harness`, hooks, `terminal`, `leases`, approvals backend. Auditors: B1 MCP tool handlers, B2 harness, hooks, terminal, session tracking.
- Targets: tool call latency, blocking waits, approval and prompt round trips, hook reply timing, tool calls that need async plus feedback.

## P624 Frontend agents UI (Stream C)

- Scope: `frontend/src/ade/v2`, `packages/workbench` approvals and agent code. Auditors: C1 plan, board, cards, panel; C2 sessions, terminal, agents, workflows, dialogs.
- Targets: first paint and interaction latency, reactivity cost, list rendering, query refetch storms, lazy loading, dialog and prompt responsiveness, missing running and finished feedback for slow actions.
