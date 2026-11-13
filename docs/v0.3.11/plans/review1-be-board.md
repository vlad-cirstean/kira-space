# Review round 1: backend board (P622 area)

Base commit: `82cffb85`. Scope: `git diff 82cffb85..HEAD` over `internal/ade`, `internal/gitsession`,
`internal/bridge` (adetask, adewire), `internal/storage`, `internal/appwire`, `docs/PERF.md` §2.16,
`docs/ARCHITECTURE.md` (ADE board facts, Known open items), plus touched Go tests. Lookups through
`codegraph_explore` (index stale for `board.go`, `leases.go`, `tracker.go`, so those were read from disk).

Checked and found sound: board snapshot gen/flight logic (`boardcache.go`: build keyed by gen, older gen
never replaces newer snapshot, caller cancel only ends its wait), push coalescer and `notifyBoardNow`
(stopped-timer race handled by `takeBoardPush`), `scheduleBoardWith` merge, lease order
stale-while-revalidate (`leaseorder.go`), repo config cache drops (`ReposChanged`, `notifyRepos`,
`RepoConfigChanged`), history memo gen, workflow memo sharing (no writer into shared step maps), migration
0100 (`IF NOT EXISTS`, registered in `embed.go`, schema snapshot updated), `CountReturns` matches old
`countReturns`, `loadTaskCtx` through `BranchesOfTask` (same `queryBranches` order, detached rows
filtered), ahead/behind and ancestry caches (immutable tip keys), entry meta cache, overlay clone
discipline (no shared slice mutation), Refresh tail tracking (`track` plus `refreshTails`).

## Findings

### F1 High: log trim scans the whole tail on every flush (perf regression)

`internal/storage/repos/adelogs.go:107-113`, `trimLogTail` `:129-155`.

Old loop cost scaled with chunks dropped (one indexed `LIMIT 1` select plus one delete each). New
cutoff query runs `SUM(length(CAST(text AS BLOB))) OVER (ORDER BY seq)` over every chunk of the log,
reading the full 2 MiB text tail, on every `Append` once the log is past `AdeLogTailBytes`. A chunk
is one output piece from `logSink.add` (usually a line), so a 2 MiB log holds ~20k rows, and
`logSink.flush` appends every 250 ms. Steady state for any verbose run past 2 MiB: a full-tail scan
per flush on the single shared SQLite connection (Known open item: every read waits behind it).

Measured (scratch sqlite, 21k rows of 100 bytes, 20 rows appended then trimmed, median of 30):
window cutoff plus delete 11.5 ms, old loop of 20 select-plus-delete 0.07 ms. Four verbose runs
hold the shared connection ~45 ms per 250 ms. ARCHITECTURE and SPEC list log trim among the cuts
that reduce queueing; this direction is reversed at steady state. The boundary test passes; the
cost model is wrong.

Fix: bound the scan to the dropped prefix. One query `SELECT seq, length(CAST(text AS BLOB)) ...
ORDER BY seq LIMIT n` with n = appended count plus a margin, compute the cutoff in Go, loop with a
larger n only when the prefix falls short, then one `DELETE ... seq <= ?`. Keep the existing
boundary test.

### F2 Medium: late facts result reused as fresh after a write that landed during the stall

`internal/ade/board_deadline.go:93-98` (late reuse), `:100` (join past-deadline flight of any gen),
`:163-170` (late store plus `scheduleBoard`).

`repoFactsWith` reads `BranchInventory` first, so a flight's tips are from its start. Scenario:
flight F starts at gen 1 and stalls 3 s (large worktree status, slow disk). At 1.5 s an agent
commits on branch X; `repo.changed` bumps to gen 2. The gen-2 build joins F (older than the
deadline), overlays at once with `updating`. F lands at 3 s with pre-commit tips, `late=true`,
`scheduleBoard`. The next build takes the late result "as is" (`updating` cleared) and snapshots it.
No further push follows, so the UI shows X's old tip, ahead/behind and integration until some
unrelated change. Second effect: the late result is single-use and goes to whichever `Board`
caller comes first, including the lease-order rebuild or the updates pass between the two
invalidations of `scheduleBoardWith` (immediate) and its timer (`notifyBoardWith`). The UI build
then starts a fresh flight; on a repo that always takes over 1 s that flight overlays again and the
cycle repeats while such a caller keeps arriving.

Fix: mark a flight `stale` when a build of a newer gen joins or abandons it. A late result from a
stale flight becomes last good for overlays only, not the consumable answer, and its landing
schedules a build that starts a fresh flight. Without that mark, keep the late result valid for its
row signature until the next gen bump instead of single-use.

### F3 Medium: parallel worktree prep cancels other repos' in-flight git writes

`internal/ade/runs.go:443` (`errgroup.WithContext`), `prepareWorktree` `ensureWorktree` calls.

The first repo error cancels `gctx`, which kills the other repos' running `git worktree add`
(gitsession spawns honor ctx). The plan states "already-created worktrees stay (same as today's
serial failure)", but the serial loop never cut a write in half. A killed `worktree add` leaves a
registered worktree with an `initializing` lock and a partial checkout. `ensureWorktree` runs a
branch "checked out anywhere ... there as is", so the next `StartRun` can launch the agent in a
half-populated tree, or fail on the stale lock. Example: repo A refuses a fresh name (taken) while
repo B is checking out.

Fix: plain `errgroup.Group` (or keep the caller's `ctx`) so in-flight repos finish; return the first
error after `Wait`.

### F4 Medium: failed pair check silently shows "no conflicts" until Refresh

`internal/ade/gitfacts.go:351` (`conflictFailed` returns error, `facts.go` maps it to no conflicts and
`Checking: false`), `rebasecheck.go:98-100,148` (failure kept until `forgetFailures`).

Before P622 a merge-tree error degraded the whole repo visibly. Now a failed pair (for example
`RebaseTimeout` under load with several agents) renders as checked and conflict-free. `adewire.Pair`
has no failed state, unlike branch `ConflictCheck: failed`. The failure stays in `checker.failed` for
that tip pair until the user presses Refresh, so a real conflict between two branches stays hidden
for as long as their tips are unchanged. `failed` also grows per failing tip pair with no bound
other than Refresh.

Fix: carry the failure on the wire (`Pair.checkFailed` or a reason, `omitempty`, contract update for
Stream C) or retry failed pair checks after a backoff instead of only on Refresh.

### F5 Medium: sandbox `fetch_remote`/`push_branch` now collide with other remote ops (P623 commit `48dfcaf3`)

`internal/ade/sandbox.go:396-405` (`FetchRemote`), `:433-444` (`PushBranch`).

`RunRemote` claims a per-entry slot and refuses a second op with `OperationInProgress`; it does not
wait (`gitsession/remote.go:314-318`). P622 plan, decision A1-11, declined exactly this change for
`ForcePush` for this reason. With the network step outside `repoMutex`, an agent's push now fails
instead of queueing when the user's Refresh fetch (still under `repoMutex`), a `ForcePush`, a rename's
remote push, or a sibling agent on the same repo holds the slot. Two parallel tasks on one repo
pushing near the same moment is a normal case; one agent gets "another remote operation is already
running on this repository" as a tool error. ARCHITECTURE line on sandbox `fetch_remote`/`push_branch`
documents the lock drop but not the refusal.

Fix: keep the network step outside `repoMutex` but wait for the slot: a per-entry remote-op queue
(or a `RunRemote` option that waits with ctx), else restore the lock around the network step.

### F6 Low: deferred marks drop the snapshot without a push; deviation premise does not hold

`internal/ade/board.go:558-583` (`writeBoardMarks`), `board_facts.go:388-399`, `integration.go:191-222`.

SPEC deviation: "wire content does not depend on the stored marks". It does for builds that run
before the write lands: `integrationRow` reads the stored target mark (`hasMark`, `MergedTip`,
`Recorded`), `mergedRule` reads stored `HadCommits`, and an unstored merge shows
`MergedAt = sc.nowMs` of that build. A build in the write window (another gen bump from `emitRuns`
or a watcher event) can show "not merged" where the stored mark gives "stale", or a moving
`mergedAt`. The write then invalidates silently and that board stays on screen until an unrelated
push. Writes happen only when a mark changes, so a push costs little.

Fix: `notifyBoard()` when a write changed a row, as the plan specified.

### F7 Low: `joinOp` runs the work on an untracked singleflight goroutine

`internal/ade/opdedupe.go:13-23`.

`DoChan` runs `fn` on its own goroutine: (a) a panic in `fn` re-panics on a fresh goroutine and kills
the process, where a panic in a bound bridge method was recoverable by the host; (b) the goroutine is
outside `b.wg`, so `Close` does not wait for a `StartRun`/`SetTaskBranch` already running; (c) the
leader's `ctx` drives the work, so a leader that closes its window cancels the operation and every
joiner gets `context.Canceled` even though its own ctx is live; the leader itself returns
`ctx.Err()` at once while the work may still commit.

Fix: run `fn` through `b.track()` with `recover` converting a panic to an error; drive the work with a
ctx detached from the leader (`context.WithoutCancel` plus the board ctx) so joiners do not inherit
the leader's cancel.

### F8 Low: SendReview recheck misses inputs gathered outside the lock

`internal/ade/reviewstep.go:516-520`.

The recheck compares rounds, branch id and name, run id, route and dedicated session. `g.tgt` also
carries `Worktree` (from `worktreeOf`), `Base` (from `reviewSpellings`) and `ChangeRequest` (from the
task's `HostURL`), and `changeRequestURL` comes from `HostURL` too. A worktree relocation, base change
or task link edit between the phases sends a round with a stale worktree path or wrong change request
link, with no `ErrStale`.

Fix: add `rc.sb.Base`, `rc.sb.WorktreeDir` (or the resolved worktree) and `rc.tc.task.HostURL` to the
recheck.

### F9 Low: Refresh tail work has no running or failure feedback

`internal/ade/board.go:1026-1041` (`refreshTail`), `:917` (`runRefreshTails`).

Cross-cutting requirement: async slow work shows running and finished feedback. Conflict checks show
`checking`, but PR merge checks (`checkPrMerges`, one host call per branch) and the tracked-updates
pass carry no running state on the wire, and a `MarkBranchFacts` failure in the tail only logs. After
Refresh returns, the UI has no signal that merge detection is still pending or that it failed.

Fix: a per-repo flag on `RepoState` while the tail runs (the `updating` chip already exists for
exactly this) and an oplog or toast-visible error on tail failure.

### F10 Low: Known open item on `taskMu` understates the lock scope

`docs/ARCHITECTURE.md` Known open items, "`taskMu` is held across git in branch change and rename".

Verified: create plans under `taskMu` with git reads (`planBranchRequest` `branchrequest.go:501`,
`planNew` `BranchInventory`, `agentBase`), so "branch create has the same shape" holds for planning.
The item omits the apply phases: branch create holds `taskMu` across worktree creation for every
approved repo (`finishBranchRequest` then `createApprovedSlots`); change holds `taskMu` plus
`repoMutex` across `SwitchWorktree` (`branchchange.go:244-270`); rename holds `taskMu` across a host
change request lookup in planning (`remoteRefusal` `branchrename.go:276`, a `gh`/`az` call) and
`taskMu` plus `repoMutex` across `RunOp` and the network push and delete of the remote branch
(`branchrename.go:318-409`).

Fix: reword the item: all three hold `taskMu` across git in plan and apply; rename also across a host
call and remote push/delete. Delete once a shared re-check design ships.

## Claims checked against code

- PERF §2.16 numbers are consistent with the code paths: `BoardHit` is a pure snapshot hit; `BoardWarm`
  re-runs facts on an unchanged repo, where every new cache hits. Fine as stated ("unchanged repo").
  The head row label says "chapter head (commits 4 to 15)"; it is the P622 stream head, not the
  chapter head.
- SPEC P622 Result: every listed mechanism exists with a real caller (`joinOp` at all nine entry
  points, `notifyBoardNow` at `handover.go:93`, `BoardChangedEvent` emitted from
  `bridge/adetask.go:59`, `Updating`/`Checking` set, `WaitRefreshTails` used by flow tests).
- Log trim claim: see F1.
