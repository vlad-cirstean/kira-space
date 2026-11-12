# P622 audit A2: ADE orchestration, storage, event bus

Base commit: `82cffb85`. Read-only audit, no code changed.

Scope: `apps/kira-space/internal/ade` orchestration (runs, steps, review, route, handover, status,
workflows, recover), ADE storage repos and migrations on hot paths, board/run event bus, bridge ADE
entry points for these. Board, tasks, branches belong to A1; cross-boundary hot spots noted, not
duplicated.

Paths below are relative to `apps/kira-space/` unless they start with `internal/` at repo root
(`internal/sqlitex`, `internal/scriptruns`, `internal/configfs`).

Ranked by user-visible impact. Feedback means: (a) does user see progress and completion today,
(b) blocking call that should go async, (c) surface to use. Surface is always P618's `adeOps`
Pinia store (`frontend/src/ade/v2/state/adeOps.ts`), `AdeOpStatus` chip
(`frontend/src/ade/v2/plan/AdeOpStatus.vue`) and vue-sonner toast it raises. Extend these, never a
parallel mechanism.

## Findings

### A2-1 One SQLite connection serializes every read behind every write

- Where: `internal/sqlitex/sqlitex.go:99` `db.SetMaxOpenConns(1)`; pragmas L49-54 (WAL,
  `synchronous=NORMAL`, `busy_timeout=5000`). `internal/storage/db.go` opens the single `*sql.DB`.
- Slow: WAL allows concurrent readers, but pool of 1 queues every query on one connection. Board
  build (A1), log reads, workflow snapshot reads wait behind log flushes (one tx per running agent
  every 250ms, `ade/logsink.go:16`), `UpdateRun`, `InsertRun`, handover writes. Orchestrator
  completion chain (A2-2) runs 25+ queries back to back on same connection, so a board refetch that
  lands mid-completion stalls for whole chain. Crosses into A1 board latency.
- Fix: second `*sql.DB` opened read-only (`mode=ro` or `_pragma=query_only(1)`), `MaxOpenConns`
  about 4, used by repo read methods. Writer stays at 1. Read-after-write paths inside one operation
  keep using writer or its tx.
- Effort L. Risk medium: each repo read needs classifying; a read that must see an uncommitted
  write in same flow breaks if moved to reader.
- Feedback: (a) n/a, removes stalls; (b) none; (c) none.

### A2-2 `loadTaskCtx` reloads whole space, about 5 times per run completion, under task mutex

- Where: `ade/runs.go:116` `loadTaskCtx`, L138 `loadSpaceFacts` (`BranchesLive` JOIN over every
  live task plus `RepoConfig.List`), L158 `buildTaskCtx` (JSON decode of workflow snapshot).
  Callers on completion path, all under `taskMu`: `completeRun` L1288, `recordOutcomeLocked` L1297,
  `decideRoute` (`ade/route.go:67`), `recordRunHandoverLocked` (`ade/handover.go:124`),
  `advanceLocked` L1324 (own load plus deferred `syncStatus`, `ade/status.go:70`). Each also calls
  `plan()` L191, which runs `LatestRuns` (A2-9). Same pattern in `StartRun` L322 (twice),
  `SetTaskStep` L1643, `reviewRun` (`ade/reviewstep.go:271`), `queueRun` L503 via `syncStatus`.
- Boot: `Recover` (`ade/recover.go:22`) calls `syncLiveStatuses` L64, which runs `syncStatus` per
  live task, so boot does O(tasks x space branches) before first window exists.
- Slow: cost grows with total branches in space, not task size. Estimate: about 5 whole-space branch
  scans and 25+ queries per run completion. Task mutex held throughout, so `StopRun`, `SendReview`,
  agent MCP tool calls on same task wait. Next step start delays by same amount.
- Fix: (1) task-scoped branch query (`BranchesOfTask`, index `ade_task_branches_task` exists);
  nicknames/repo config from a small cache invalidated on repo config change. (2) Load one `taskCtx`
  and `plan` at top of `recordOutcomeLocked` and thread through `decideRoute`,
  `recordRunHandoverLocked`, `advanceLocked`; `syncStatus` takes the ctx; refresh only `tc.runs`
  after `UpdateRun`. (3) `syncLiveStatuses` loads facts once for all tasks.
- Effort M. Risk low: pure read refactor, covered by run engine tests.
- Feedback: (a) run row flips to done via runs event, fine; next step appears late with no hint;
  (b) none once cheap; (c) none needed.

### A2-3 `SendReview` does gh/az lookup and git spawns while holding task mutex

- Where: `ade/reviewstep.go:442` `SendReview` (lock held L442-539): `reviewRun`, `liveRound`,
  `reviewTarget` (`ade/review.go:85`: `BranchInventory`, `DefaultRemote`, `baseCtxFor`, `worktreeOf`,
  `linkedChangeRequest` L107), `roundMessage` calling `changeRequestURL` (`ade/review.go:128`), whose
  `ResolveBranchChangeRequest` is a remote host query through gh/az CLI; then `WorktreeHead` (two git
  spawns), `UpsertReview`, `BumpReviewRound`, `MarkSent`, `queueRound` L557. Bridge
  `internal/bridge/adetask.go:912` waits for all of it.
- Slow: network round trip (typically 0.5-3s, worse offline or rate-limited) plus git spawns while
  completion, stop and MCP calls for same task block. Button stays pending until done.
- Fix: resolve target, CR URL and worktree head before taking lock; under lock re-check round
  state (round unchanged, branch head unchanged) and write. Return once round is queued; agent
  launch already async. Cache CR URL per branch with short TTL, or take it from updates watcher's
  last observation (`ade/updates.go`) instead of a live lookup.
- Effort M. Risk low-medium: re-validation must catch a concurrent round change.
- Feedback: (a) only `:pending` on panel buttons (`frontend/src/ade/v2/review/AdeWorkflowReviewPanel.vue:99`),
  no completion toast, failure only via actionError; (b) host lookup out of lock, see fix;
  (c) adeOps kind `sendReview`: chip on review row, toast on queued and on failure. `FinishReview`
  (`ade/reviewstep.go:917`) same, kind `finishReview`.

### A2-4 `StartRun` and advance hold task mutex across worktree creation and setup start

- Where: `ade/runs.go:322` `StartRun` L322-373; `prepareStepWorktrees` L396, `prepareWorktrees` L407
  loops `ensureWorktree` (`ade/setup.go:118`: `repoMutex`, `openRepo` with `GitStatus`, `BranchInventory`,
  `git worktree add`) serially per branch; `startSetup`. Same via `advanceLocked` when next step
  needs worktrees, and `ApproveStep`/`ResumeRun` L1495. Bridge `internal/bridge/adetask.go:833` and
  phone path (`internal/bridge/mobilewrite.go`) wait synchronously.
- Slow: `git worktree add` on a large repo takes seconds per branch; multi-repo task pays sum, not
  max. Task mutex blocks every other op on task meanwhile.
- Fix: insert run rows first as pending with note "preparing worktree" (existing pending/setup gate,
  `noteWaitingSetup`), release lock, return run ids. Prepare worktrees in a tracked goroutine
  (`goTracked`), per repo in parallel (distinct `repoMutex`), then launch through existing held-run
  path. Errors land as stuck run with reason, as setup failures already do.
- Effort M. Risk medium: start errors move from bridge return value into run state; tests asserting
  synchronous errors need updating.
- Feedback: (a) `useTaskAction` (`frontend/src/ade/v2/run/useTaskAction.ts`) only sets `busy`;
  `frontend/src/ade/v2/run/AdeRunDialog.vue:166` awaits `start.mutateAsync` with no toast; failure only actionError;
  (b) worktree prep should go async, see fix; (c) adeOps kinds `startRun`, `approve`, `retry`,
  `nextPass` (`frontend/src/ade/v2/state/adeStageDialog.ts:124`): chip on card, error toast. Completion signal for async part:
  existing runs event (pending to running, or stuck with reason).

### A2-5 `emitRuns` reloads full run history and triggers full board refetch per state change

- Where: `ade/runs.go:288` `emitRuns`; `newWireBatch` `ade/reviewstep.go:106` (badges query,
  `RunsOfTask` full history, `ReviewRounds`), `wireRun` L149 (`GetReview` per run). L308
  `scheduleBoard` fires every time.
- Slow: one run state change costs history scan plus N review reads, then 250ms later frontend
  refetches entire Board (git facts included, A1) although runs event already carries the change.
  Many emits per completion (finish, handover, next step start).
- Fix: Returns from `LatestRuns` or a `COUNT(*) ... GROUP BY step_id, branch_id`; `GetReviews(ids)`
  batch read. Stop `scheduleBoard` for run-only changes once frontend patches board cache from runs
  event (coordinate C1); keep it where status or step actually changed.
- Effort M. Risk medium: frontend must patch board cache correctly, or card goes stale.
- Feedback: n/a.

### A2-6 `notifyBoard` undebounced at about 75 call sites, double fires per op

- Where: `ade/board.go:349` `notifyBoard`; e.g. `ade/status.go:109` inside `syncStatus`, then caller
  fires again (`ade/runs.go:1701-1703` in `SetTaskStep`; `ade/rebase.go:770` per recovered rebase).
  Each call: `OnBoard` broadcast (frontend board refetch), `dropLeaseOrder` (A2-11),
  `updates.boardChanged` (`ade/updates.go:112`).
- Slow: two to three board refetches per user action; each drops lease order cache.
- Fix: route through `scheduleBoard` (250ms coalesce, `ade/board.go:340`), or coalesce per locked
  operation (mark dirty, fire once on unlock).
- Effort S. Risk low: tests waiting on immediate board event need a flush or wait.
- Feedback: n/a.

### A2-7 Workflow and step-library reads parse files and hit DB on every call

- Where: `internal/adeflow/reader.go:97` `Get`, `internal/configfs/folder.go:96` (repo root) `Get`, L178
  `entryFor`, L159 `lastGood` (`Projection.Snapshot` SELECT); `internal/adeflow/steps.go:155` `Lookup`
  reads and parses each referenced library step file. Callers: `taskWorkflow`
  (`ade/taskwf.go:25`) per unstarted task in every `buildTaskCtx`, `snapshotWorkflow` L41,
  `liveWorkflows` L101 (memo per call only), Workflows page `List`.
- Slow: stat, read, YAML parse, N step files, JSON marshal and a DB SELECT per call; multiplied by
  A2-2 load count for unstarted tasks and by board build.
- Fix: in-memory cache in configfs keyed by file (mtime, size, hash), invalidated by existing
  `Watch` callback (`ade/workflows.go:75` `workflowsFileChanged`, L82 `stepsFileChanged`); load
  Snapshot once and refresh on write. Started tasks already use snapshot JSON; cache decoded
  snapshot per task keyed by snapshot hash.
- Effort M. Risk low: watcher already exists; stat check covers missed events.
- Feedback: n/a.

### A2-8 `AdeLogsRepo.Append` trims oldest chunks one by one per flush

- Where: `internal/storage/repos/adelogs.go:75` `Append`, trim loop L106-124: SELECT oldest plus
  DELETE per dropped chunk while bytes exceed `AdeLogTailBytes`.
- Slow: once a log reaches tail cap (long agent run, steady state), every 250ms flush per running
  agent does 2 x dropped-chunk statements inside write tx, holding the single connection (A2-1).
- Fix: mirror `internal/scriptruns/repo.go:446` `AppendLogs`: prepared insert, then one
  `DELETE ... WHERE kind=? AND id=? AND seq <= ?` with cutoff from a running-sum window
  (`SUM(bytes) OVER (ORDER BY seq DESC)`), or keep total bytes on the parent row.
- Effort S. Risk low.
- Feedback: n/a.

### A2-9 `LatestRuns`/`LatestRunsOf` correlated subquery lacks covering index

- Where: `internal/storage/repos/adetask.go:1645`/`1651`: `attempt = (SELECT MAX(attempt) ... WHERE
  task_id, step_id, branch_id, purpose)`. Index `ade_runs_task (task_id, purpose, attempt)` in
  `internal/storage/migrations/0090_p557_baseline.sql` has no `step_id`, `branch_id`.
- Slow: each outer row scans all task runs of same purpose: O(n^2) per task. Runs on every
  `plan()`, i.e. 5+ times per completion (A2-2) and in board `ListTasks`.
- Fix: migration `0100_p622_ade_runs_latest.sql`:
  `CREATE INDEX ade_runs_latest ON ade_runs(task_id, step_id, branch_id, purpose, attempt)`; or
  rewrite with `ROW_NUMBER() OVER (PARTITION BY ...)`.
- Effort S. Risk low. Small write cost per insert.
- Feedback: n/a.

### A2-10 `queueRun`/`launch` read whole tables and rebuild git inventory per call

- Where: `ade/runs.go:531` `SetupByBranch()` (`adetask.go:1130`, every setup row) per queued run;
  L576 `RunningRebaseOn` (`adetask.go:1938`) loads all running rebases and JSON-decodes each spec
  per launch; `worktreeOf` (`ade/setup.go:308`) via `rowWorktree` builds a fresh `attachCache`
  (`ade/worktreedir.go:144-211`), so each call re-reads repo inventory (git spawn).
- Slow: per-run cost grows with space history; multi-branch steps repeat inventory read per branch.
- Fix: `SetupOf(branchID)`; `RunningRebaseOn` filtered by branch in SQL (store branch ids in a
  column or join table) or one load per operation; pass one `attachCache` through operation.
- Effort S. Risk low.
- Feedback: n/a.

### A2-11 Lease order rebuilds full `Board()` after every board change

- Where: `ade/leases.go:92` `taskListOrder` builds `b.Board(b.ctx)` on cache miss;
  `notifyBoard` drops cache (A2-6). `internal/leases/leases.go` `lockRanked` calls order inside
  `Acquire`, `Release`, `Sweep`, `Snapshot`. Cross-boundary with Stream B (B1, MCP `acquire_lease`).
- Slow: agent MCP lease call after any board change pays a full board build including git facts.
- Fix: order from cheap query (live tasks by board position from `ade_tasks`) instead of `Board()`;
  or drop cache only on task order change, not every notify.
- Effort S-M. Risk low.
- Feedback: n/a (agent-facing latency).

### A2-12 `reviewWorktree` loads full task ctx per review read request

- Where: `ade/review.go:248` `reviewWorktree` calls `loadTaskCtx` (A2-2) for `ReviewWorking` and
  `ReviewWorkingDiff`; panel requests diff per file.
- Slow: whole-space branch scan per file opened in review panel.
- Fix: `GetTask` plus branch-of-task check (`GetBranch`, compare `TaskID`), no plan.
- Effort S. Risk low.
- Feedback: n/a (TanStack loading state exists).

### A2-13 `taskMus` map never pruned

- Where: `ade/runs.go:66-74` `taskMu` adds entries, nothing deletes.
- Slow: not latency; small unbounded growth over app life (one mutex per task ever touched).
- Fix: delete on archive/delete under `taskMusMu` when unlocked, or `sync.Map` with refcount.
- Effort S. Risk low: deleting a mutex another goroutine holds breaks exclusion; delete only after
  archive completes and no run is live.
- Feedback: n/a.

## Feedback surfaces to add

Extend `AdeOpKind` in `frontend/src/ade/v2/state/adeOps.ts` (today `removeBranch | removeRepo |
archive`) with `startRun`, `approve`, `retry`, `nextPass`, `setStep`, `sendReview`, `finishReview`,
`rebase`. Each gets i18n loading/status text and goes through `ops.run`, so `AdeOpStatus` chip and
vue-sonner loading/success/error toast come for free. Wire callers: `useTaskAction.ts` (run,
approve, retry), `frontend/src/ade/v2/run/AdeRunDialog.vue:166`, `AdeWorkflowReviewPanel.vue:99`, `frontend/src/ade/v2/state/adeStageDialog.ts:124`,
step-change menu. Backend work moved async (A2-4) reports completion through existing runs and
board events; op resolves when bridge returns (run queued), and run row carries the rest.

## Areas with nothing real

- `ListTasks` (`ade/orchestrator.go:142`): batched (`LatestRunsOf`, `LatestHandovers`); only shared
  `loadSpaceFacts` cost from A2-2.
- Log sink cadence (`ade/logsink.go`): 250ms batching fine; cost is in A2-8.
- `sweepLeases` ticker (`ade/leases.go:114`): fine.
- Script runs (`internal/scriptruns`): `AppendLogs` efficient; `startMu` serializes only start.
- Updates watcher (`ade/updates.go`): gh/az passes rate-limited by 30s min gap, off request path.
- Goroutines: `goTracked`/`runTracked` tied to board ctx; no leak found.

## Cross-boundary notes

- A1: A2-1 (single connection) and A2-5/A2-6 (board refetch fan-out) drive board latency.
- B1: A2-11 lease order on MCP lease path.
- C1: A2-5 needs frontend to patch board cache from runs event before `scheduleBoard` is dropped;
  feedback surface section is C-side work.
