# P622 audit A1: board, tasks, branches (backend)

Scope: `apps/kira-space/internal/ade` board assembly (`board.go`, `board_facts.go`, `gitfacts.go`,
`integration.go`, `deploy.go`), task and branch writes (`board_writes.go`, `taskbranches.go`,
`setup.go`), change requests, refresh, tracked updates pass, bridge `AdeTaskService` read/write entry
points. Orchestrator, steps, review, workflows, storage internals belong to A2; cross-boundary spots
are flagged.

Base: `82cffb85` (chapter open). Read-only audit; discovery via `codegraph_explore`.

## Cost model of one `TaskBoard.Board()` (evidence for the findings)

- DB: `load()` runs 11 queries (`board.go:440-475`). Per repo, `collectRepo` adds `RepoConfig.List`,
  `Facts.Marks`, `Facts.EnvStates` (`board_facts.go:411-435`), `CodeRepos.Get`, `Tasks.BranchesLive`
  (`board_facts.go:438-458`). Then `RunSandbox.Badges`, `TaskTitles`, `MarkBranchFacts` tx. Total
  13 + 5R queries plus up to one write tx.
- Git, fixed per repo, uncached: `for-each-ref` inventory + `worktree list`
  (`gitsession/queuefacts.go:26-60`), `git remote` twice (`board_facts.go:179` and again inside `MainRef`,
  `gitsession/queuefacts.go:99`), `symbolic-ref` origin HEAD (`gitsession/review.go:71`), `config --get user.email`
  (`board_facts.go:198`). Six spawns per repo per call.
- Git, per branch, uncached: `git status` in each attached worktree (`board_facts.go:337`);
  `rev-list --left-right` for a branch without upstream but with a remote twin (`gitfacts.go:228`);
  `rev-list` main depth for a branch based on anything but main (`board_facts.go:405`); one
  `for-each-ref --merged` per existing branch in `computeAncestry`, sequential (`gitfacts.go:166-195`)
  whenever two or more pair items exist.
- Git, cached by immutable tips: `rangeFacts`, `contains`, merge-tree pairs, patch ids. Rebase
  conflict check is already async (`applyChecks`, `board.go:533`).
- All reads go through `Repo.Read` (4 slots, `gitclient/repo.go:38`) and never run beside, or after a
  pending, `Repo.Write` (`repo.go:151-170`). `RunOp` (worktree add, checkout, rebase ops) takes Write.
- Quick timing on this repo (6.4k files, Linux, warm): `git status --porcelain=v2 -z
  --untracked-files=all` 19 ms; `for-each-ref`, `worktree list`, `remote`, `config` 2 ms each. One
  repo with 10 attached branches: roughly 30 spawns, 100-150 ms per `Board()` on Linux; spawn cost on
  macOS is several times higher.
- Callers of the full computation: bridge `AdeTaskService.Board` (one per window per board push),
  `updateWatcher.branchObservations` (`updates.go:270`, after each board change, MinGap-limited),
  `taskListOrder` for lease ranking (`leases.go:97`, dropped on every `notifyBoard`).

No backend call blocks the UI thread: Wails runs bound methods off it. The cost shows as latency
(stale board, spinners that last too long) and as repeated CPU and git work.

## Findings (ranked by user-visible impact)

### A1-1 No coalescing: every board push recomputes the whole board once per window, plus extra copies

- Where: `board.go:349-355` (`notifyBoard`), `board.go:493-530` (`Board`), `readSignals.ts:28-32`,
  `updates.go:270`, `leases.go:92-111`. 61 call sites call `notifyBoard` directly, undebounced.
- What: `notifyBoard` broadcasts payload-free `ChannelBoard`. Each window invalidates `boardKey`,
  `changeRequestsKey` and `panelForwardPrefix`, so each window calls `Board()` and `ChangeRequests()`.
  `Board()` has no singleflight or snapshot cache: N windows (plus the phone over SSE) mean N full
  computations. The same push also arms an updates pass that runs `Board()` again, and drops the
  lease order so the next lease grant runs it again. One mutation often emits several pushes:
  `SetTaskBranch` new branch calls `notifyBoard` at `taskbranches.go:214`, again in `createNamed`
  (`taskbranches.go:238`), again in `startSetup` (`setup.go:342` or `setup.go:374`), and the git
  watcher adds a debounced fourth (`board.go:330-347`, 250 ms). That is 4 pushes x windows x
  (Board + ChangeRequests), plus an updates pass. TanStack cancels the in-flight refetch on each new
  invalidation, so the backend finishes computations nobody reads.
- Fix: in `TaskBoard`, keep a board generation counter bumped by `notifyBoard` and by `handleEmit`
  repo events. `Board()` goes through `singleflight` keyed by generation and returns the cached
  snapshot while the generation is unchanged (add a short TTL, about 2 s, as a guard for git changes
  no event announces). Route direct `notifyBoard` calls through a trailing coalescer (about 30 ms,
  with max wait) so one mutation emits one push. `branchObservations` and `taskListOrder` read the
  cached snapshot. The update pass and lease order then stop costing extra computations.
- Effort M. Risk: medium. A write path that changes board state without bumping the generation would
  serve a stale board. Mitigate: bump inside `notifyBoard` only (every write already calls it) plus
  the TTL. The coalescer delays pushes by up to 30 ms; keep it below perceptible delay.
- Feedback: none needed (latency fix). Board query keeps showing the last data while it refetches.

### A1-2 One busy repo stalls the whole board

- Where: `board.go:501-514` (errgroup waits for every repo), `gitsession/queries.go:50-63` (`runOne` through
  `Repo.Read`), `gitclient/repo.go:111-170`, `setup.go:291` (`RunOp` takes `Repo.Write`).
- What: `Board()` returns only after the slowest repo. Any `Repo.Write` on a repo blocks all its
  reads, and a pending Write blocks new ones: worktree add (full checkout, seconds on big repos), an
  in-app commit, checkout or rebase op. During that time every window's board refetch hangs for
  every repo, not only the busy one. No timeout, no partial result, no signal.
- Fix: keep the last good `repoResult` per repo (keyed by repo id, stored with the snapshot from
  A1-1). Give each repo's `repoFacts` a deadline (about 1 s). On expiry, assemble with the previous
  result, set a new `RepoState.Updating` flag, let the computation finish in the background and
  `scheduleBoard()` when it lands. First load with no previous result waits as today.
- Effort M. Risk: medium. Stale branch facts show briefly; the flag makes that explicit. Writes that
  need fresh facts (`wireRow`, `attachExistingBranch`) must keep calling `repoFacts` directly, without
  the deadline.
- Feedback: `AdeRepoChip` shows its existing pending spinner while `updating` is set (extend the chip
  props, no new mechanism). No toast: background refresh.

### A1-3 Per-call git spawns that immutable keys or the entry could cache

- Where: `board_facts.go:179`, `gitsession/queuefacts.go:80-117` (`MainRef`, `DefaultRemote`),
  `board_facts.go:198`, `gitsession/queuefacts.go:40-60` (`worktree list` in every inventory read),
  `gitfacts.go:166-195` (`computeAncestry`), `gitfacts.go:228` (`remoteTwin`),
  `board_facts.go:398-407` (`mainDepth`), `board_facts.go:337` (`WorktreeStatus`).
- What: six fixed spawns per repo and one to three per branch run on every call although most
  answers only change on a ref move or a config edit. `computeAncestry` is O(N) sequential spawns per
  repo. Each holds one of 4 read slots, so it also queues behind and delays user git reads in the
  same repo.
- Fix:
  - `AheadBehind` in `remoteTwin` and `mainDepth`: cache by `tipPair` (same LRU pattern as `since`).
  - `computeAncestry`: cache the result keyed by the sorted (item, tip) set; recompute only when a
    tip moves.
  - Remote list, `user.email`, origin HEAD: cache on `RepoEntry`, dropped on the existing
    `refsChanged`/config invalidation (`invalidateAfterWrite`). `MainRef` reuses the remote already
    read by `repoFacts`.
  - `git status` per worktree: cache keyed by worktree path, invalidated by the board generation
    bump plus agent `PostToolUse`/`Stop` hook events whose cwd is inside the worktree (tracker already
    sees them), with a short TTL (2 s) for edits nobody announces.
- Effort M. Risk: low for tip-keyed caches (immutable inputs); medium for `git status`, where a
  missed invalidation shows a wrong dirty list for up to the TTL.
- Feedback: none (latency fix).

### A1-4 Refresh: long lock hold and a slow tail the spinner waits for

- Where: `board.go:764-785` (`Refresh`), `board.go:792-880` (`refreshRepo`),
  `AdePlanView.vue:49-72`.
- What: `refreshRepo` holds the repo mutex across the network fetch, `repoFacts`, `checkPrMerges`
  (host calls per mine branch, `board.go:891`) and waits for every rebase-conflict check
  (`board.go:865-878`). Then `Refresh` runs `updates.run(ctx, true)` synchronously: a fresh pass with
  forced host calls (CR snapshot, Jira) and another full `Board()`. The repo chip spinner lasts until
  all of that ends. Any write on that repo (`SetTaskBranch`, `ForcePush`, attach, worktree add) blocks
  on the mutex meanwhile, with no feedback in its own surface.
- Fix: release the repo mutex once the fetch returns (the remote-op slot already serializes remote
  ops; `repoFacts` takes no lock). Run `checkPrMerges` and the updates pass in `goTracked` and let
  them `notifyBoard`. Stop awaiting conflict checks: the checker already calls `scheduleBoard` when one
  lands and branches show `checking` meanwhile. `Refresh` returns after fetch plus facts.
- Effort S-M. Risk: low-medium. The returned `MergedInto` summary no longer includes PR-merged
  branches; report those via the next board push, or keep `checkPrMerges` inline but outside the
  mutex.
- Feedback: chip spinner (exists) now ends with the fetch. Conflict `checking` chip (exists) covers
  the tail. Add a vue-sonner error toast when a refresh fails on a repo whose chip is scrolled out of
  view (`AdePlanView` sets only an inline chip error today).

### A1-5 Branch set/create/attach run worktree add synchronously; the form shows nothing

- Where: `taskbranches.go:176-251` (`SetTaskBranch`, `createNamed`, `wireRow`),
  `board_writes.go:395-415` (`prepareAttached`), `board_writes.go:483-548`
  (`attachExistingBranch`), `setup.go:118-197`; frontend `AdeTaskBranchForm.vue:42-66`,
  `AdeAddPopover.vue:143`.
- What: the bridge call returns only after `git worktree add` (a checkout of the whole tree, seconds
  on large repos) under task mutex + repo mutex + `Repo.Write`, then `wireRow` recomputes repo facts
  (inventory, main ref, config, owners) for one row. `AdeTaskBranchForm` binds no pending state to
  `setBranch`: the user clicks, nothing changes until the call returns. Meanwhile the Write blocks
  every board read of that repo (A1-2).
- Fix: backend: write the row and a setup row with state `running` and note "creating worktree",
  notify once, return; run `ensureWorktree` and `startSetup` in `goTracked`, recording failure via
  `recordSetupFailure` (the path already exists for prepare scripts). Frontend: route the submit
  through `adeOps.run` with a new kind (`setBranch`), so the form closes at once, `AdeOpStatus` shows
  on the branch row, and a toast reports done or the git refusal. Keep name validation
  (`checkFreshName`) synchronous so a bad name still errors inline.
- Effort M. Risk: medium. Callers that expect the created branch name in the reply (agent
  `request_branches`, A2/B1 boundary) must keep the synchronous path or wait on the setup row.
- Feedback: `adeOps` kind `setBranch` (and `attachBranch` for `AddExistingBranch`), `AdeOpStatus`
  chip on the row, vue-sonner loading/success/error toast. Setup progress keeps its existing
  `WorktreeSetup` state on the row.

### A1-6 Board payload grows without bound

- Where: `board.go:608` (`History: toWireHistory(d.archived, d.archBranch)`), `board.go:467-472`,
  `board.go:613-652` (`toWireTask` sends every run attempt with note and summary),
  `taskwf.go:71-98` (`decorateTask`).
- What: every push ships the whole archive (every archived task ever, with its repos) and runs two
  extra queries for it. Each started task also carries its full workflow snapshot; `decorateTask`
  unmarshals it, re-marshals it and hashes it per task per call. `countReturns` and `latestRoundOf`
  scan the run list per run (O(R^2) per task). The bridge serializes all of it per window per push.
- Fix: move history to its own query (`adeTaskHistory`), invalidated only by archive, restore and
  delete; the board push stops refetching it. Memoize `decorateTask` output per (task id,
  `WorkflowHash`, live hash) on `TaskBoard`. Run payload belongs to A2: flag it there (send latest
  attempt per step, page older ones).
- Effort M. Risk: low-medium; frontend readers of `board.history` (C1) move to the new query.
- Feedback: none.

### A1-7 Change requests refetched on every board push, resolved sequentially

- Where: `readSignals.ts:30`, `board.go:695-760` (`ChangeRequests`, `resolveRepoHost`),
  `gitsession/changerequests.go:539-624`.
- What: each board push refetches `ChangeRequests` in every window. Inside one repo, branches resolve
  one after another; a cache miss or a branch with no open CR past its 5 min TTL falls through to
  `ForBranch` (a `gh`/`az` CLI spawn, 0.5-2 s) inline on first load. PR badges therefore appear late
  on cold start, one repo's branches at a time.
- Fix: invalidate `changeRequestsKey` on the existing `SignalChangeRequestsChanged` and on branch-set
  changes, not on every board push (needs a payload or a separate channel from the backend). Resolve
  branches in parallel within a repo (bounded, 4).
- Effort S. Risk: low; stale-while-revalidate in `gitsession` already guards correctness.
- Feedback: PR badge area shows a loading state while the query has no data (C1 owns rendering).

### A1-8 Redundant per-repo DB reads (N+1)

- Where: `board_facts.go:411-458`.
- What: per repo, `RepoConfig.List()` reads every repo's config, `BranchesLive()` re-reads rows
  `load()` already holds, `CodeRepos.Get` runs again. 5 extra queries per repo per call.
- Fix: read configs, code repos and live branches once in `load()`, pass them into `repoFacts`.
  `wireRow` and `attachExistingBranch` read them once for their single repo.
- Effort S. Risk: low.
- Feedback: none.

### A1-9 DB writes on the board read path

- Where: `board.go:526` (`MarkBranchFacts` tx), `integration.go:220`, `deploy.go:245` (`saveMark`
  from branch goroutines).
- What: a read takes the SQLite writer. With A1-1's parallel computations, several `Board()` calls
  race to write the same rows, and each waits on the single writer behind log sinks of running
  agents. Writes are rare (only on a new fact), so cost is tail latency, not throughput.
- Fix: collect facts and marks during the read and apply them in one `goTracked` write after the
  snapshot returns. A1-1's singleflight already caps writers at one.
- Effort S. Risk: low; a crash between read and write loses only a fact the next build recomputes.
- Feedback: none.

### A1-10 Pair conflicts computed inline on a cold cache

- Where: `board_facts.go:267-280`, `gitfacts.go:259-300`.
- What: unlike the rebase check, merge-tree for pairs with shared files runs inside `Board()`. After
  a restart or after many commits, a board with N overlapping branches spawns up to N(N-1)/2
  merge-trees (each 50-500 ms on big repos) before the first board renders.
- Fix: route pair merge-trees through the rebase checker queue (or a sibling queue), report the pair
  as pending, `scheduleBoard` on completion. Warm the cache in the background at startup.
- Effort M. Risk: low-medium; the frontend must tolerate a pending pair (render "checking").
- Feedback: existing conflict `checking` chip; no toast.

### A1-11 Force push: modal blocks for the whole network push

- Where: `board.go:947-980` (`ForcePush`, repo mutex held across the push),
  `AdeForcePushDialog.vue:14-24`.
- What: the confirm dialog stays open with a spinner until the push finishes; slow remotes keep the
  user stuck in it. The repo mutex blocks other writes on the repo during the push (the remote-op slot
  already serializes remote ops).
- Fix: run through `adeOps.run` with kind `forcePush`, close the dialog on confirm; drop the repo
  mutex around `RunRemote` (keep it for the preflight read only).
- Effort S. Risk: low.
- Feedback: `adeOps` kind `forcePush`, `AdeOpStatus` on the branch row, vue-sonner toast with the
  remote error text on failure.

## Areas checked with nothing real

- Lock hold inside `Board()`: only `b.mu` for map lookups (`cachesFor`, `openRepo`); no repo or task
  mutex taken. Fine.
- Rebase-conflict check: already async with peek/queue and `scheduleBoard` on completion.
- `GitStatus` discovery: cached 30 s (`gitclient/discovery.go:213`). Fine.
- Host CR lookups: stale-while-revalidate with breaker and persisted index already in place; only
  the call pattern (A1-7) is off.
- Integration and deployment facts: `contains` cached by immutable tips; the few uncached
  `IsAncestor`/`CountRange` calls in `deploymentRow` run only for marked, rebased branches. Not worth
  a finding.

## Cross-boundary notes for A2/B1/C1

- A2: run list payload (A1-6) and `RunsByLiveTask` returning every attempt; any step/run write that
  calls `notifyBoard` in a burst benefits from A1-1.
- B1: agent `request_branches` relies on synchronous `SetTaskBranch`/`createNamed` (A1-5).
- C1: `board.history` readers (A1-6), PR badge loading state (A1-7), `updating` flag on repo chips
  (A1-2), new `adeOps` kinds `setBranch`, `attachBranch`, `forcePush` (A1-5, A1-11).
