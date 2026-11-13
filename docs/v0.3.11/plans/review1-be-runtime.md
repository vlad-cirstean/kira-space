# Review round 1: backend runtime (P623)

Base commit: `82cffb85`. Scope: `git diff 82cffb85..HEAD` for `internal/harness` (event queue, hooks
hub, CLI cache), `internal/terminal`, `internal/appevent`, `internal/agentmcp`, `internal/llmprompts`,
`apps/kira-space/internal/oplog`, ade tracker, leases, agent ops, branch request, sandbox, artifacts
quick scope, appwire shutdown, ARCHITECTURE notes. Three dimensions: structure/security, correctness,
performance.

Checked and clean (no finding): `eventQueue` close/enqueue/barrier (senders WaitGroup gated by the
`closed` check, no Add-after-Wait; drain then exit; panic recovered); Stop/UserPromptSubmit barrier vs
`suppressIdle` (reply runs after earlier events, `ReplyFailed` before its own enqueue); shutdown order
`hooks.Stop` then `tracker.Close`; `Registry.notifyChange` (changeAgain keeps the last change; unregister
precedes `done`, so `CloseAll`'s `changeWG.Wait` never races an Add); concurrent `CloseWindow`
(`Session.closeFor` idempotent); `Coalescer` flush order (flushMu taken under mu) and `Finish` at
shutdown; `learnAsync` Close wait (`closed` checked under `t.mu` before `learnWG.Add`); `SendIfIdle`
busy mark before unlock and guarded rollback; `callCtx` (release happens after process exit, when the
HTTP ctx ends anyway); agent op lifecycle (approval op finished on approve/deny/timeout/abandon in both
`awaitBranchRequest` and `SpawnTask`; rename/change ops finished by defer on every return); parallel
slot creation (per-repo clone of `tc.branches`, no shared positional writes, failures listed in the op
and per slot); progress notifications (counter monotonic, no-op without token); task_info wait loop
(negative `time.Until` fires at once, loop exits). B1-8 reasoning holds: MCP 2025-06-18 asks for the
text copy beside `structuredContent` and go-sdk typed handlers emit both; dropping it needs transcript
evidence.

## F1 (Medium): sandbox fetch/push outside `repoMutex` now fail fast on a busy remote slot

`apps/kira-space/internal/ade/sandbox.go:401`, `:437`; `gitsession/remote.go:314` (`remoteOp.claim`).

`RunRemote` allows one remote op per repo entry and returns `OperationInProgress` at once, never waits.
Before P623, every ADE remote op on a repo (sandbox fetch/push, `refreshFetch` `board.go:985-1012`,
`ForcePush` `board.go:1122-1135`, rename remote push `branchrename.go:409`) ran under `repoMutex`, so
they queued. Now sandbox `fetch_remote`/`push_branch` run unlocked.

Scenario: two step runs on different branches of one repo finish together and both call
`push_branch`; or a run pushes while the user clicks Refresh (refresh fetch holds the slot). The
second call gets tool error "another remote operation is already running on this repository". Before,
it waited a few seconds and succeeded. An agent may give up, report a failed push, or loop.

Fix: keep the network step off `repoMutex` but serialize ADE remote ops per repo with a separate
`remoteMutex(codeRepoID)` taken by sandbox fetch/push, `refreshFetch`'s fetch, `ForcePush`,
`ReviewFetch` and the rename remote push. Alternative: on `OperationInProgress`, wait and retry under
`ctx` (bounded) inside `FetchRemote`/`PushBranch`. Add an adeflow test with two concurrent
`push_branch` calls on one repo.

## F2 (Medium): `flushLease` can strand a lease message and reorder messages

`apps/kira-space/internal/ade/leases.go:190-214`.

`flushLease` now deletes `pending[key]`, drops `lease.mu`, calls `SendIfIdle`, and re-queues on
failure. While messages are in flight nobody else sees them.

1. Lost wakeup. Grant arrives (`deliverLease`), flush A takes `[grant]`, `SendIfIdle` reads turn busy
   and returns false. Before A re-locks, the hook worker handles the turn's Stop: turn goes idle,
   `onIdle` runs flush C, which finds `pending` empty and returns. A then re-queues `[grant]`. The
   agent is idle, the message waits, and no trigger fires until the user types or the agent calls a
   lease tool. Before P623, C blocked on `lease.mu` until A finished, then sent. The same hole hits
   `takeLeaseNotices` (Stop reply and lease tool calls read nothing while A holds the messages).
2. Reorder. A takes `[m1]`; `deliverLease(m2)` starts flush B, which takes `[m2]` and sends while A is
   still before its `SendIfIdle`; A then sees busy and re-queues `m1`, delivered after `m2` (e.g.
   "revoked" before "you now hold").
3. `endLeases` (`leases.go:257`) runs while A is in flight: it deletes `pending[key]`, then A re-queues
   for an ended key. The entry never drains; `LeaseSnapshot` `NoticePending` also reads false during
   every flush.

Fix: serialize flushes per key without holding `lease.mu` across the PTY write: a per-key (or single)
`flushMu` taken around take, `SendIfIdle` and re-queue, so C waits for A and then sees the re-queued
messages. Skip the re-queue when the key was ended meanwhile (track ended keys or a generation).

## F3 (Low): one global hook worker gives cross-session head-of-line blocking

`internal/harness/hooks.go:151-170`, `internal/harness/eventqueue.go`.

All sessions share one worker, and the Stop/UserPromptSubmit barrier waits for every earlier event of
every session. `OnEvent` includes `Notifier.HandleEvent`, whose `notifyTerminal` calls `AppFocused`
(blocks on the main thread, `agentnotify.go:193`) and sends the OS note synchronously. Scenario: the
main thread is busy (large render, modal) while session A finishes; session B's user prompt submit
then waits on A's notification before claude gets its hook answer. Before, each event ran on its own
handler goroutine, so B did not wait on A.

Fix: shard the queue by `TerminalID` (one ordered worker per terminal, or a fixed pool hashed by
terminal) and make the barrier per terminal; `suppressIdle` is per record, so per-terminal order is
all it needs. Or move the notifier's main-thread probe off the worker.

## F4 (Low): CLI path cache pins an older binary after a reinstall

`internal/harness/locate.go:78-103`.

The cache stores any successful resolve, including the cheap `toolexec.Locate` PATH/candidate hit, and
only drops it when the cached path stops being executable. Scenario: user moves from the npm `claude`
to the native installer (`~/.local/bin`, earlier on PATH) and keeps the old one; Kira keeps launching
the old binary until restart. Before, every launch found the new one. The expensive step the cache
targets is only the login-shell probe (seconds); `Locate` costs microseconds.

Fix: run `Locate` fresh every time; cache only the login-shell probe result (same key, same
stat check).

## F5 (Low): `task_info` wait can miss the setup-end wake by up to 5 s

`apps/kira-space/internal/ade/agenttools.go:416-429`, `:441-448`.

The loop reads the setup row, then fetches `setupDone`. If the setup finishes between the two (row
written, `endSetup` deletes the `setupLive` entry and closes `done`), `setupDone` returns nil and the
select waits for the 5 s fallback tick. Fix: take the channel first, then read the row; select on the
channel taken before the read.

## F6 (Low): concurrent session-id learning breaks the "one id per record" exclusion

`apps/kira-space/internal/ade/tracker.go:513-546` (`learnAsync`), `:472-507` (`learnSessionID`).

Dedupe is per record only. `Reconcile` stopping two OpenCode records in one cwd now learns both in
parallel; each reads `AgentSessionIDsExcept` before the other stores, so both can pick the same single
candidate (e.g. one record's own session never got created) and store the same id. A resume of one
then opens the other's conversation. Sequential `learnStopped` stored the first, and the second
excluded it. A sync `learnSessionID` from take over can race a background one of another record the
same way.

Fix: hold one tracker-level learn mutex (or one per cwd) across the taken read, `FindSession` and
`SetAgentSessionID`; the background goroutines still keep `Stop`/`Reconcile` unblocked.

## F7 (Low): docs and comments that no longer match the code

- `docs/ARCHITECTURE.md` (agent ops bullet): says `agentApproval` ends "error on timeout or abandon";
  `finishApprovalOp` (`agentops.go:41-52`) finishes both as `cancelled` (`timeout`, `abandoned`).
  The P624 mirror and toast policy read the status, so fix the doc (or the code, if error was meant).
- `docs/ARCHITECTURE.md` (finish reply bullet): "The panel (`ListArtifacts`) stays on the full path".
  `ListArtifacts` is the agent's quick path (`artifacts.go:538-540`); the panel is `TaskArtifacts`.
- `docs/v0.3.11/SPEC.md` P623 Result: "quick artifact scope ... stays DB rows only"; the code also
  reads `RepoEntry.CachedBranchChangeRequest` (`artifacts.go:291`), as ARCHITECTURE says. Same Result
  says "every tool handler runs under `callCtx`"; `finish_step` (`mcp.go:335`) and
  `reply_review_comment` (`review.go:38`) do not (fine, both are short; fix the wording).
- `apps/kira-space/internal/appwire/appwire.go:737-739`: comment says shutdownTracker "flushes ...
  and stops the hooks listener"; the order is now stop, then flush.
- `internal/appevent/coalescer.go:26-28`: "Push and the timer never wait on an emit in progress" is
  false when a size-triggered Push meets a running flush (it blocks on `flushMu` while holding `mu`).
  Say "a Push that does not flush never waits on an emit".
