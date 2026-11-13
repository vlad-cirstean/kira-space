# Review round 2: backend board (P622 area and round-1 fixes)

Base commit: `82cffb85`. Reviewed head: `535862a64` (code; later commits are findings docs only). Scope:
`git diff 82cffb85..HEAD` over `internal/ade`, `internal/gitsession`, `internal/bridge` (adetask,
adewire), `internal/storage`, `internal/appwire`, `internal/leases`, `docs/PERF.md`,
`docs/ARCHITECTURE.md`; focus on commits after `35a6bbf5` (round-1 fixes, unreviewed). Index stale for
most changed ade files, so `codegraph_explore` served gitsession and callers; changed files read from
disk.

Checked and found sound:

- Remote slot (`remoteop.go`): every holder releases before taking `repoMutex` (sandbox fetch/push,
  `ReviewFetch`), every `repoMutex` holder takes the slot after it (Refresh fetch, `forcePush`,
  `fetchHostLink`, rename push under `taskMu` then `repoMutex`). No path waits for `repoMutex` while
  holding the slot; no cycle. Queued waits end on ctx; the slot is released by defer; one channel per
  code repo, bounded. A Refresh queued behind a sandbox push holds `repoMutex` while it waits, same
  blocking as the pre-chapter base (push under `repoMutex`).
- `joinOp`: tracked on `b.wg` inside the singleflight fn, `errBoardClosed` after Close, panic
  recovered, work ctx keeps values, drops leader cancel, ends with `b.ctx`. No nested joinOp holds a lock
  the inner fn needs (`NextPass` calls `StartRun` after `movePass` unlocks).
- Pair retry (`rebasecheck.go`): attempts carried across `request`, 5/20/80 s backoff, retry timers
  check `closed` before `onDone`, failures bounded at 512, `forgetFailures` clears both branch and
  `pair:` ids on Refresh, `Pair.checkFailed` carried through facts, overlay and wire.
- Deferred marks push: marks, `hadCommits` and `merged` are emitted only when they differ from the
  stored row, so a write's push converges (no mark loop on its own; see F1 for the facts loop it
  feeds).
- `reviewMoved`: compares only stored row fields read twice, no clock or host value; it cannot stay
  true without a real write, so no permanent `ErrStale`.
- Refresh `updating`: count incremented only when the tail goroutine is tracked, decremented after
  `updates.run` on every path (b.ctx cancel returns), push after the decrement. A tail panic crashes
  the process rather than sticking the flag (pre-existing, unchanged).
- Lease flush lock: take, `SendIfIdle` and re-queue under the per-owner `f.mu`; Stop reply, idle
  flush and lease tool notices wait on it, so the lost wakeup and reorder of round 1 F2 (runtime) are
  gone; `ended` stops re-queue for a finished owner. Tracker invokes `replyFn` and `onIdle` with `t.mu`
  released, so `f.mu` then `t.mu` has no inverse order.
- `prepareWorktrees` without errgroup ctx; log trim prefix scan (geometric rescan, early cutoff,
  boundary test kept); task_info channel order; per-cwd learn lock; entry meta cache.

## F1 High: stale-flight rule keys on the board-wide gen; two slow repos recompute forever

`internal/ade/board_deadline.go:109-111`, `:139-142` (stale marks), `:174-177` (`fresh` needs
`bcache.gen == fl.gen` at landing), `:180-182` (late landing calls `scheduleBoard`). Round 1 F2 fix,
commit `03b857e4`.

`bcache.gen` is one counter for the whole board. Every `invalidateBoard` bumps it: another repo's
`repo.changed`, `emitRuns`, lease and task edits, a deferred mark write (`writeBoardMarks` now
`notifyBoard`s), a pair retry timer (`onDone` is `scheduleBoard`), and a late landing's own
`scheduleBoard`. The fix treats any such bump as "a change landed after the flight read its tips", so a
valid late result of an unchanged repo is demoted to overlay-only and its landing schedules another
build, which starts another full facts computation of that repo.

Self-sustaining case: two repos A and B whose facts take over 1 s. Both flights are abandoned at the
deadline and land close together. A lands first: fresh, `scheduleBoard` bumps the gen at once. B lands:
gen moved, so not fresh. The next build consumes A's late result and starts a new flight for B (no
consumable result); B overlays with `updating`, lands, schedules; that build consumes B and starts a new
flight for A (A's late result was single-use); and so on. The board never settles: both repos run full
facts back to back, a board push every flight, a UI refetch per push, an `updating` chip that never
clears for one of them.

Measured (scratch test in package `ade`, removed afterwards: two repos, `facts.deadline` 100 ms, hook
sleeps 300 ms per computation, a stand-in UI that refetches after every push, 8 s): head 15
computations and 14 pushes, still rising about 2 per second. Same test with `fresh := late` (pre-fix
rule): 2 computations, 2 pushes, settled. One slow repo alone settles on both. On a real board "slow"
means status plus facts over 1 s on a large repo, a cost the loop then pays continuously next to running
agents.

Smaller effect with one slow repo: each unrelated bump during its flight (a run state change, a mark
write of the build that abandoned it) discards the result and costs one more full computation plus about
one more second of `updating`.

The second half of round 1 F2 is also still open: the late result stays single-use (`l.late = false` on
first read), so a `Board` caller between `scheduleBoardWith`'s immediate invalidate and its timer still
takes it, and the UI build after the push starts a new flight.

Fix: track change per repo, not per board. Keep a per-repo counter bumped where a repo's facts can move
(`handleEmit` `repo.changed` for that repo, Refresh, writes that name the repo, row signature change),
record it on the flight at start, and mark stale only when that repo's counter moved. Keep a landed
result valid for its row signature and repo counter, not single-use, and let a build reuse it while both
match (also stops recomputing an unchanged slow repo on every unrelated gen bump). Add a test with two
stalled repos and a refetch-on-push loop that asserts the computation count settles.

## F2 Low: a newer gen inside the deadline runs a second computation of the same repo concurrently

`internal/ade/board_deadline.go:102-108`; `docs/ARCHITECTURE.md` "Per-repo facts deadline" bullet.

When `fl.gen != gen` and the flight is younger than the deadline, a new flight replaces it in
`f.flight`, but the old one keeps running on `b.ctx` (nothing cancels it, `repoFactsWith` takes no
per-repo lock). A burst of invalidations each followed by a `Board` call within one second of the
previous flight's start (push window 30 ms; run state changes, mark writes, retries) starts one
computation per bump: three bumps in a second on a 1.5 s repo means three concurrent full facts runs of
the same repo, each slower for the others. ARCHITECTURE says "At most one computation per repo and row
signature", which the code does not hold; the type comment's "a restart queues behind the same lock"
names a lock that does not exist.

Fix: with F1's per-repo counter, a newer board gen whose repo counter did not move joins the running
flight; one that did move may start a new flight only after the running one lands (one queued
successor per repo), or cancel the old flight through a per-flight ctx. Correct the ARCHITECTURE line
either way.

## F3 Low: docs still describe the pre-fix behavior

- `docs/ARCHITECTURE.md` "Board marks (P622)": "A clean write drops the snapshot without a push (wire
  content is unchanged)." Since `31945416` the write pushes (`writeBoardMarks` calls `notifyBoard`),
  and SPEC records why.
- `docs/ARCHITECTURE.md` "Op dedupe (P622)": "The first caller's ctx drives the work." Since `a4302df0`
  the work runs tracked on a ctx with the first caller's values but not its cancel or deadline, stopped
  by the board ctx; a caller's cancel only ends its wait.
- `docs/v0.3.11/SPEC.md` P622 "Left out": "Dropping `repoMutex` around `ForcePush` (`RunRemote` refuses
  instead of waiting ...)". `runRemote` now waits on the per-repo ADE slot, so that reason no longer
  holds; say why the lock stays (for example the preflight-to-push lease window), or open the follow-up.

Fix: reword the three lines to match the code.

## Claims checked against code

- ARCHITECTURE remote slot bullet: callers, lock order and the outside-ADE refusal match
  `remoteop.go` and its six callers.
- ARCHITECTURE pair check bullet (3 retries, 5 s then 4x, `checkFailed`, 512 cap, Refresh forgets):
  matches `rebasecheck.go`.
- ARCHITECTURE Refresh bullet (`updating` while the tail runs, `refreshTail` oplog error): matches
  `board.go` `markTails`, `refreshTail`.
- PERF log trim paragraph: code reads only the prefix (`adeLogTrimScan` 64, 4x rescan, one delete).
- SPEC P622 Result: deferred-mark deviation and log trim wording match the code; the facts deadline
  claims hold for one slow repo only (F1).
