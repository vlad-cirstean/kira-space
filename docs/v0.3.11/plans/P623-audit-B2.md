# P623 audit B2: agent runtime backend performance

Base commit: `82cffb85`. Read-only audit. Discovery via `codegraph_explore` against `/home/user/kira-space`
(same tree).

Scope: harness (claude/opencode hooks server, CLI locate, session tracking), `internal/terminal` (PTY I/O,
output coalescing, close), `apps/kira-space/internal/leases` + `ade/leases.go`, approvals backend
(`internal/approvals`), session lifecycle (launch, restart, close, reconcile), activity detection. MCP tool
handlers and llmprompts belong to B1; cross-boundary spots marked.

Feedback surface for every user-triggered slow op: `adeOps` Pinia store
(`apps/kira-space/frontend/src/ade/v2/state/adeOps.ts`) + `AdeOpStatus` chip + vue-sonner `toast.loading` then
success/error by id (P618). Extend `adeOps` kinds; never a parallel mechanism.

Findings ranked by user-visible impact.

## B2-1 Hook HTTP reply waits for all OnEvent work

- Where: `internal/harness/claude/hooks/http.go:170-176` (`handleHook`); same shape in
  `internal/harness/opencode/hooks/server.go:148-154`.
- Slow: when no reply is due (common case), `s.onEvent(ev)` runs before `w.WriteHeader(200)`. Claude's hook
  shim (`hooks/shim.go`, curl `--max-time 2`) blocks the agent until the 200. OnEvent chain
  (`appwire/wire.go:104-142`): `Tracker.HandleEvent` (global `Tracker.mu`, sync DB writes
  `SetAgentSessionID`/`SetLastActive`/`SetUnseen` on TurnEnd/SessionEnd, `OnChange` broadcast, deferred
  `onIdle` → `flushLease` → PTY write), then `bridge.EmitAgentEvent` (Emit to every window), then
  `agentnotify.HandleEvent` (`agentnotify.go:147-218`: `Prefs()` settings DB read, `AppFocused` which blocks
  on main thread, `Describe`, OS notification send). Every PreToolUse/PostToolUse/Stop pays this. Under
  main-thread or DB contention it can exceed the 2s curl cap: hook then fails silently and the event is
  lost to the agent side.
- Fix: write the 200 (or reply) first, then enqueue the event to one ordered worker per server (buffered
  channel, single goroutine, preserves order). Drain the worker in `Close`. Reply path keeps `s.reply(ev)`
  synchronous since its body is the reply.
- Effort: S/M. Risk: ordering already guaranteed by single worker; tests that assert state right after
  the HTTP call need a sync point (drain hook or `Flush` for tests).
- Feedback: none needed; removes agent-side stall. Adds nothing to UI.

## B2-2 CLI resolve uncached, login-shell probe up to twice per launch

- Where: `internal/harness/locate.go:63-91` (`ResolveCLI`, `shellProbeTimeout=5s` at :19). Callers:
  `harness/claude/locate.go:61` `Locate`, `ade/tracker.go:362` (`Tracker.Prepare`),
  `internal/terminal/bound.go:316-322` (`requireAgent`, every agent `Open`, `context.Background()`), opencode
  `FindSession` (`harness/opencode/session.go`).
- Slow: PATH miss (common in GUI-launched app: minimal PATH) spawns `$SHELL -l -i -c "command -v claude"`.
  Interactive login shell sources rc files: often 0.3-2s, cap 5s. Nothing cached, so a launch runs it in
  Prepare and again in requireAgent before the PTY exists. Terminal sits in `starting` the whole time.
- Fix: cache resolved path per (spec, bin, configured path override) in `harness`; invalidate on agent
  settings change; `os.Stat` the cached path on use and re-resolve on miss. Drop `requireAgent`'s second
  `Locate` when launch already resolved (pass resolved path through the open token/args).
- Effort: S. Risk: stale path after reinstall, covered by stat check.
- Feedback: launch already shows terminal `starting`; no new surface. After fix, first launch only.

## B2-3 Close and restart block seconds under the task lock, no feedback

- Where: `ade/launches.go:211-230` (`closeTUILocked`, comment "Blocks until the process exits"),
  `ade/launches.go:269-309` (`RestartSession`), `internal/terminal/session.go:297-337` (`closeFor`: grace 2s
  at :31, kill wait 2s at :41, exit drain 200ms at :46).
- Slow: worst case ~4.2s per session, held on the bridge call goroutine with the task lock held, so other
  task operations on same task queue behind it. UI gets no running indicator; button looks dead.
  `Registry.CloseWindow` (`session.go:591-607`) closes sessions one after another: N agent tabs in a closing
  window cost up to N × 4.2s; `CloseAll` (:613-633) already closes concurrently.
- Fix: keep sync wait where conversation-file safety needs it. Add `adeOps` kinds `restartSession` and
  `closeSession` with chip + `toast.loading` → success/error. Send SIGTERM to the agent process group at
  close start so most exits finish well under the grace period. Make `CloseWindow` concurrent like
  `CloseAll` (WaitGroup).
- Effort: S (ops kinds, CloseWindow), M (signal path). Risk: concurrent close ordering in CloseWindow is
  independent per session; low.
- Feedback: required. Chip while running, toast on finish or error.

## B2-4 learnStopped spawns opencode CLI synchronously on stop and reconcile

- Where: `ade/tracker.go:507-513` (`learnStopped`), called in `Stop` (:790) and `Reconcile` (:846);
  `learnSessionID` (:467-504, `locateTimeout=10s` at :462) → opencode `FindSession`
  (`opencode session list --format json -n 20`, plus uncached `Locate`, see B2-2).
- Slow: process spawn up to 10s on Restart/Close path and on `Registry.OnChange` (reader exit goroutine and
  `Registry.Open`, see B2-6). Claude path is cheap (id from hooks); opencode only.
- Fix: run `learnStopped` on a goroutine with per-record dedupe set; write id back under `Tracker.mu` when
  done. Or defer to take-over, which already calls `learnSessionID`.
- Effort: S. Risk: take-over racing the background learn; guard with the dedupe set and re-check.
- Feedback: covered by B2-3 ops; after fix nothing user-visible blocks.

## B2-5 Terminal output flush runs under coalescer mutex on reader goroutine

- Where: `internal/appevent/coalescer.go:83-95` (`flushLocked` calls `c.flush(batch...)` with `c.mu` held);
  `internal/terminal/service.go:134-135` (`coalesceInterval=16ms`, `coalesceMaxBytes=16KiB`), flush at
  :178-201 (concat, base64, `EmitTo`); `session.go:26` (`readBufSize=32KiB`), `readLoop` :170-224
  (allocate + copy per chunk).
- Slow: under heavy output (build logs, `cat`), each 32KiB read exceeds the 16KiB max, so every read
  flushes immediately: one Wails event per read, no batching. Base64 (+33%), JSON marshal and Wails dispatch
  run on the PTY reader goroutine with the coalescer lock held, so reading stalls while dispatching;
  timer-path flushes block `Push` too. About 4 copies per byte (read copy, concat, base64, JSON). Shows as
  output stutter and CPU spikes on bursty agent output.
- Fix: swap batch out under lock, flush outside it; keep order with a single flusher (flushing flag or
  one flush goroutine fed by channel). Raise `coalesceMaxBytes` to 64KiB. Reuse read buffer via
  `sync.Pool` or concat directly into one buffer.
- Effort: M. Risk: output ordering and Exit-after-last-data; cover with existing service tests plus one
  ordering test (concurrency earns a unit test per working agreement).
- Feedback: N/A (throughput).

## B2-6 Registry.Open runs OnChange (Reconcile) before returning

- Where: `internal/terminal/session.go:468-470` (`Registry.Open`, agent sessions); OnChange wiring in
  `appwire/appwire.go` → `Tracker.Reconcile` (`ade/tracker.go:809-861`) and `KeepAwakeRecompute`
  (`bridge/keepawake.go:69-84`, settings DB read).
- Slow: Reconcile walks `LiveAgents`, may write DB, runs `learnStopped` (B2-4). All before `terminalOpen`
  resolves, adding to spawn-to-visible time.
- Fix: dispatch OnChange on a goroutine (coalesced: one pending recompute at a time) so Open returns once
  the PTY exists.
- Effort: S. Risk: Reconcile sees session slightly later; it is idempotent.
- Feedback: none.

## B2-7 PTY write under Tracker.mu, nested under lease mu

- Where: `ade/tracker.go:1073-1093` (`SendIfIdle` writes PTY holding `Tracker.mu`), called from
  `ade/leases.go:189-211` (`flushLease` holding lease `mu`).
- Slow: a PTY write blocks when the child stops reading (full kernel buffer). Then every hook event (B2-1
  path) and every lease op waits. Messages are short, so rare; impact is a full stall when it hits.
- Fix: mark record busy under lock, write outside, roll back busy on error. Release lease mu before
  `SendIfIdle`.
- Effort: S. Risk: double-send race; busy flag prevents it.
- Feedback: none.

## B2-8 Lease ranking rebuilds full task board (cross-boundary B1, stream A)

- Where: `leases/leases.go:143-154` (`lockRanked` calls ranker when waiters >= min; `Acquire` uses
  `lockRanked(1)`), `ade/leases.go:92-108` (`taskListOrder` → `Board(ctx)`), `ade/leaseorder.go`
  (`orderCache.get` holds its mutex during build), `leaseOrderFresh=5s` (`ade/leases.go:34`), cache dropped on
  every board change.
- Slow: any Acquire on a queue with 1+ waiters, and every UI `Snapshot` refetch after payload-free
  `AdeLeasesChanged`, can trigger a full Board build (DB + git facts). Concurrent callers serialize on
  cache mutex. Acquire is an MCP tool call (B1 owns the handler latency).
- Fix: rank from a cheap DB-only task order query (no git facts); or serve stale order while one rebuild
  runs in background (singleflight).
- Effort: M. Risk: order momentarily stale; acceptable for queue ranking.
- Feedback: none (agent-side wait).

## B2-9 Approvals Put opens router prompt under queue mutex (minor)

- Where: `internal/approvals/queue.go:139-150` (`Put` calls `prompts.Open` under `q.mu`); prompt deps in
  `appwire/appwire.go:364-380` read settings (`Enabled`, `RaiseOnOpen`) per open.
- Slow: settings DB read and possible window raise while approvals mutex held; concurrent `Put`/`Remove`
  and `answerOne` wait. Low volume (cap 50 pending), so small.
- Fix: cache the two settings flags (invalidate on settings change) instead of reading per prompt. Keep
  Open under mu (ordering vs Close is deliberate).
- Effort: S. Risk: low.
- Feedback: none.

## B2-10 Per-hook-event broadcast fan-out (note only)

- Where: `bridge.EmitAgentEvent` emits to every window per hook event; `HandleEvent` OnChange broadcasts
  `AdeTaskSessionsChanged`, each window refetches sessions.
- Cheap per event today. Becomes visible only with many windows and chatty tools. Fold into B2-1 worker
  (coalesce `AdeTaskSessionsChanged` per 100ms) if B2-1 lands.

## Nothing real

- `mobileterm` broker output ring (`mobileterm/broker.go`): append under lock, cheap.
- `leases.Book` `Touch`/`Sweep` (30s ticker, `ade/leases.go:28`): O(queues), cheap.
- `ComposeInto` per-launch settings file rewrite (`hooks/agenthooks.go:159-200`): one small file.
- Transcript/JSONL watching: none for claude (ids come from hooks); no per-tick scans found.
- Goroutines per session: reader + waiter + exit drain, plus coalescer timer. Bounded, fine.
- Approvals broker (`dbmcp/approval.go`): bounded queue, one timer per request, fine.
- Statusline hook: throttled 30s, backgrounded in shim.

## Feedback summary

- Needs `adeOps` extension: B2-3 (`restartSession`, `closeSession`). Optionally `launch` kind covering
  Prepare (B2-2) if first-launch probe stays slow after caching.
- Blocking calls to move async: B2-1 (OnEvent after reply), B2-4 (learnStopped), B2-6 (OnChange in Open),
  B2-5 (flush outside lock), B2-7 (PTY write outside lock).
