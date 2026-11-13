# Review round 2: backend runtime (P623 area plus CI/docs head)

Base commit: `82cffb85`. Reviewed head: `535862a64`. Scope: `git diff 82cffb85..HEAD` for
`internal/harness`, `internal/terminal`, `internal/appevent`, `internal/agentmcp`, `internal/llmprompts`,
`apps/kira-space/internal/oplog`, ade leases flush, tracker learning, ADE remote queue (`remoteop.go` and
its callers), appwire shutdown, `.github/workflows`, `scripts`, docs. Focus: round-1 fixes since
`35a6bbf5` (unreviewed) and the workflow/docs commits `386c3dd1d`, `ac213099d`, `535862a64`. Lookups
through `codegraph_explore`; index stale for `hooks.go`, `tracker.go`, so changed files were read from disk.

Checked and clean (no finding):

- `eventQueue` per-terminal lanes: the worker deletes its lane under `mu` only when empty and after the
  last popped item returned, and `enqueue` creates a lane under the same `mu`, so an event enqueued as a
  lane drains starts a fresh worker; no lost event, no stuck lane, order kept (the old worker finished
  every earlier item before deleting). `barrier` on an absent lane returns at once, correct because the
  lane outlives its in-flight item. Full-lane `enqueue` wakes on `space.Broadcast` and re-resolves the
  lane after `Wait`. `barrierAll` covers every live lane. `close` gates senders by `closed` under `mu`
  before `senders.Wait`, then `workers.Wait`. Lanes and goroutines bounded by terminals with events
  waiting. `-race` on `internal/harness`, `internal/appevent` passes.
- Reply barrier only for main-session Stop/UserPromptSubmit; shutdown `hooks.Stop` then `tracker.Close`
  (comment at `appwire.go:737-740` now matches).
- `flushLease` per-owner lock: closes round-1 F2 lost wakeup (idle flush waits for the in-flight one, then
  sees the re-queue) and its reorder between two flushes; `takeLeaseNotices` waits too; no lock cycle
  (`f.mu` then `lease.mu` or `Tracker.mu`/terminal write; nothing takes `f.mu` under those). Snapshot
  counts in-flight messages.
- CLI path lookup: `Locate` fresh every call, only the login-shell probe cached, stat-checked, misses not
  cached, cache off when any seam is set. A login-shell result pinned while the old binary still exists is
  the accepted trade-off (documented, ARCHITECTURE 1540-1542).
- `task_info` takes the setup channel before the row read (round-1 F5 fixed).
- Per-cwd learn mutex spans taken read, `FindSession`, store (round-1 F6 fixed; growth below as F3).
- `runRemote` ctx handling: wait ends on ctx, slot released by defer on every path, no slot holder takes
  `repoMutex` (sandbox post-network sections at `sandbox.go:411`, `:450` lock after the slot drops).
- Workflows: `pr.yml` and `cache-warm.yml` parse as YAML. `cache-warm` matrix-images `gtk: "true"` matches
  `pr.yml`'s test-matrix setup (`pr.yml:299-303`). The macOS step `go test -tags imagecliptest
  ./apps/kira-space/internal/imageclip/...` matches the build tags (`imageclip_darwin_test.go`,
  `readback_darwin.go`: `darwin && cgo && imagecliptest`); no `CGO_ENABLED` override in `.github`; same
  `run:` block, so bash `-e` stops on the first failing `go test`. Matches DEV_ENVIRONMENT step 11.
- Docs commit: the workflow-push limitation is gone in fact (`386c3dd1d`, `ac213099d` reached
  `origin/chapter/v0.3.11` from this sandbox). The deleted ARCHITECTURE item "`pr.yml` has no `test:ui`
  step" is false now (`pr.yml:80-100` ui shards, visual, ui-extra), so deleting it was right; the PACKAGING
  "pending workflow patch" (aggregates on `always()`, matrix deadline) was applied in `872249caa`
  (`pr.yml:114`, `:332`, `:370`). `bun site/scripts/check-docs.ts` (0 violations), `checkprompts`,
  `check-ui-primitives.sh`, `go test ./scripts/docshistory/...` pass.

## F1 (Medium): callers holding `repoMutex` wait for the ADE remote slot, so a push again blocks repo ops

`apps/kira-space/internal/ade/board.go:1031-1057` (`refreshFetch`), `board.go:1169-1185` (`forcePush`),
`hostlink.go:88-96` (`fetchHostLink`), `branchrename.go:328-350` (`applyRename` then
`renameRemoteBranch`); `remoteop.go:10-17`.

Round-1 F1 fix queues ADE remote ops on a per-repo slot with lock order `repoMutex` then slot. Four of
the seven callers take `repoMutex` first and then wait for the slot while holding it. The slot holder can
be a sandbox `push_branch`/`fetch_remote` or a `ReviewFetch`, which run without `repoMutex` precisely so
that an askpass prompt or slow host does not block the repo (`review_sync.go:133-134`, `remoteop.go:12-13`:
"a sandbox push no longer blocks adopt, detach or setup on the repo").

Scenario: an agent calls `push_branch`; the push waits on an askpass credential prompt or a slow host (or a
`ReviewFetch` runs, up to its 10 min timeout). The user clicks Refresh: `refreshFetch` takes `repoMutex`,
reads inventory, then blocks in `runRemote` on the slot. From now until the push ends, adopt, detach,
worktree setup, archive, rename, and the post-network locked sections of other agents' `fetch_remote` /
`push_branch` (`sandbox.go:411`, `:450`) all wait on `repoMutex`. Same with `ForcePush`, host-link fetch
or a rename push queued behind the slot. That is the pre-P623 blocking the queue claims to avoid, now
reached transitively. No deadlock (slot holders never take `repoMutex`).

Fix: invert the order for these four callers: take the slot first (ctx-aware), then `repoMutex`, so a
queued caller waits without the repo lock (forcePush keeps preflight and push atomic, both inside).
Offer a `withRemoteSlot(ctx, codeRepoID, fn)` helper and let `runRemote` skip the acquire when the caller
already holds it, or split each caller so its network step runs after `repoMutex` drops, as sandbox does.
Update the lock-order sentence in `remoteop.go` and ARCHITECTURE:2632. Test: slot held by a blocked fake
push, Refresh queued, `AdoptBranch` (or any `repoMutex` user) on the repo completes.

## F2 (Low): `requeueLeaseReply` and a stale flush state bypass the per-owner flush lock

`apps/kira-space/internal/ade/leases.go:279-290` (`requeueLeaseReply`), `:218-225` (`flushLease`).

1. Reorder (round-1 F2 part 2, incomplete). `requeueLeaseReply` prepends to `pending` without `f.mu`.
   Stop reply takes `[m1]`; grant `m2` arrives, flush A takes `[m2]` and is in `SendIfIdle`; the hook reply
   is not delivered, `ReplyFailed` prepends `m1` (pending `[m1]`); A finds the turn busy and re-queues as
   `append([m2], [m1]...)`: `m2` now precedes `m1` (e.g. "you now hold" after a later "revoked", or the
   reverse). Fix: in `requeueLeaseReply`, look up `lease.flush[key]` and hold its `mu` across the prepend,
   like `takeLeaseNotices`; then A's re-queue lands first and `m1` is prepended ahead of it.
2. Ended mark reset. `flushLease` reads `f` from `flushOf`, releases `lease.mu`, then waits on `f.mu`. If
   `endLeases` runs in that gap it sets `f.ended` and drops `f` from the map; the flush then sets
   `f.ended = false` (`:224`). Any message queued for the key afterwards (a `ReplyFailed` re-queue of the
   session's last Stop, which also lands on an ended key with no flush at all) is taken by an orphan `f`
   that no later caller locks, and a busy send re-queues for an ended key: the round-1 F2.3 leak, narrowed
   but open. Fix: after `f.mu.Lock()`, under `lease.mu` check `b.lease.flush[key] == f`; if not, unlock and
   return (the owner ended or a newer state exists). Never reset `ended`. Drop re-queues for keys that
   ended (`requeueLeaseReply` skips a key with no live tracker record, or `endLeases` keeps a short
   ended-key set).

## F3 (Low): per-cwd learn mutex map never shrinks

`apps/kira-space/internal/ade/tracker.go:224-225`, `:494`.

`learnCwd` gains one `*sync.Mutex` per distinct cwd ever learned and never drops it. ADE worktrees are
per branch, so the map grows with every task branch for the app's lifetime. Magnitude is small (one key
string plus a mutex per worktree), so impact is memory hygiene, not a user-visible failure. `LoadOrStore`
also allocates a mutex on every call. Fix: fixed stripe array (e.g. 64 mutexes indexed by a hash of the
cwd), which keeps the per-cwd exclusion with no growth and no allocation; or a refcounted map under
`t.mu` that deletes on last unlock.

## F4 (Low): stale docs, comments and a leftover pending patch

- `internal/harness/hooks.go:17-19`: OnEvent "runs on the hub's single worker in arrival order"; it is
  per-terminal lanes now, ordered per terminal only. `hooks.go:182`: "drains the event worker" (lanes).
- `docs/v0.3.11/SPEC.md:33` P623 Result still describes pre-round-1 code: "processes events on one
  ordered worker (queue 1024)" (per-terminal lanes, 1024 per lane); "Resolved agent CLI paths cached per
  environment" (only the login-shell probe); "`fetch_remote` and `push_branch` run the network step
  outside `repoMutex`" (plus the per-repo ADE remote queue); "`flushLease` write the PTY outside ...
  `lease.mu`" (true, but a per-owner flush lock now spans the write). The close phase distills history
  from this Result, so the stale text would ship into `docs/history/`. Add the round-1 changes to the
  Result (P622's Result already records its review-round deviations).
- `docs/ARCHITECTURE.md:1538-1539`: "writes lease and queued messages to the PTY outside `t.mu` and the
  lease lock": name the per-owner flush lock held across the write, so nobody adds a wait on it from a
  PTY-write path.
- `docs/ARCHITECTURE.md:4119` (Known open item, multi-image clipboard): "its `go test -tags imagecliptest`
  line needs the pending workflow patch". `ac213099d` applied it; say PR CI `checks (macOS)` runs the test
  and the item stays until it passes there and steps 11-12 pass on a Mac.
- `docs/pending-changes/.github__workflows__pr.yml.patch` is still tracked (`git ls-files`) although
  `ac213099d` applied exactly that diff and `535862a64` removed the workaround that said to delete an
  applied patch in the same commit. Since `repoint.go:89` no longer skips `docs/pending-changes/`,
  `scripts/docshistory` repointing would now also rewrite text inside it. Fix: `git rm` the file (and the
  directory with it).

No other stale reference to the removed paths outside `docs/history`, `docs/HISTORY.md` and this
chapter's plans (grep for `pending-changes`, `pending-workflows`, `workflow scope`).
