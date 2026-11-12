# v0.3.10 code review round 2

Base: `8165b1d3` (v0.3.9 merge). Head: `54fd8511`. Scope: `git diff 8165b1d3..HEAD`, phases P617-P621,
full-run fixes and round 1 fixes (`1642d4c2`..`54fd8511`). One Opus reviewer, three dimensions.
Findings only; fixer works from this file.

Counts: High 0, Medium 3, Low 4.

## Medium

### R2-M1 SendIfIdle still submits a draft typed while the turn was busy

- File: `apps/kira-space/internal/ade/tracker.go:1051-1054` (`SendIfIdle`), `:968-971` (`idleAt`);
  test `apps/kira-space/internal/ade/tracker_test.go` `TestTracker_SendIfIdleHoldsBackAfterUserTyping`.
- Description: hold-back fires only when `LastInput` is after `idleAt` (the Stop that made the turn
  idle). Claude Code keeps its input box editable while it works; text typed there without Enter
  stays as a draft after the turn ends. Those keystrokes are older than `idleAt`, so `SendIfIdle`
  pastes the lease text after the draft and presses Enter. The test pins this as intended
  ("Typing before the turn went idle ... does not block"). R1-M4 is fixed only for typing after
  the Stop.
- Failure scenario: agent works; user starts typing the next instruction into the box, stops midway.
  Turn ends (Stop, idle). A waiter grant or "Request release" fires: Kira appends "Kira: you now
  hold lease ..." to the half-written instruction and submits both as one prompt.
- Fix: decide on "draft present", not on time since idle. Track in `terminal.Session` the time of
  the last key press and of the last submit (a write carrying `\r` outside a bracketed paste), and
  hold back while the last key press is after the last submit. Keep `idleAt` only as a floor. Change
  the test: keystrokes during the busy turn with no Enter must hold back; a submitted prompt must not.

### R2-M2 change_branch does not see task-level agent sessions working in the folder

- File: `apps/kira-space/internal/ade/branchchange.go:199-224` (`otherWorkOn`, the `runningTUI`
  filter), with `apps/kira-space/internal/ade/launches.go:671` (step launch) and
  `apps/kira-space/internal/ade/review_agent.go:69-71` (review agent).
- Description: the round 1 fix (R1-M3) refuses when another running TUI has
  `s.BranchID == sb.ID`. Step sessions and the review agent are task-level: `PrepareArgs` sets no
  `BranchID`, `launchDirs` makes the first own worktree their cwd and adds the others with
  `--add-dir`. Their rows carry `BranchID ""`, so the filter never matches them. Only sessions opened
  on one branch (`launches.go:788`) are caught. The new flow case inserts a branch-bound session, so
  the gap is untested.
- Failure scenario: the implement step session and the review agent of one task are both open. The
  implementer calls `change_branch`, user approves, `git switch` swaps files under the review agent,
  which then reads and comments on the wrong branch. Same with two step sessions of one task.
- Fix: also match running TUI rows of the same task with `BranchID == ""` (they work in every own
  worktree of the task), excluding the caller's session. Add a `TestBranchChange` case with a
  task-level session row (`branch_id` empty).

### R2-M3 Lease holder waiting at a permission prompt expires, then runs the tool without the lease

- File: `apps/kira-space/internal/ade/tracker.go:942-945` (`EventNotification` sets `turnInput`),
  `:1030-1038` (`Working` requires `turnBusy`), `apps/kira-space/internal/ade/leases.go:139-152`
  (`leaseAlive`).
- Description: Claude Code runs `PreToolUse` before its permission check, then sends a
  permission Notification, so the record sits in `turnInput` with one open tool call. `Working`
  returns false for any state but `turnBusy`, so `Sweep` expires the holder after the TTL. The
  expiry notice stays pending until the next Stop or prompt hook, after the tool ran.
- Failure scenario: agent holds `push`, asks permission for `git push`; user is away 12 minutes.
  At 10 minutes the lease expires and the next waiter is granted and pushes. User returns, approves;
  the first push runs too. The collision leases exist to prevent.
- Fix: count `turnInput` with an open tool call as working, under the same `leaseToolCeiling` bound,
  so an abandoned prompt still frees the lease. Add a tracker case: tool start, needs-input
  notification, TTL passes, still working; past the ceiling, not working.

## Low

### R2-L1 Open tool calls are counted, not paired

- File: `apps/kira-space/internal/ade/tracker.go:925-935` (`tools` counter).
- Description: `tools` is a bare counter. A `PreToolUse` with no `PostToolUse` keeps it above zero
  for the rest of the turn. The listener drops a hook body over its size cap with 413
  (`internal/harness/claude/hooks/http.go:16-19`); a `PostToolUse` carrying a large
  `tool_response` is the documented case, and the UI reducer pairs by `tool_use_id` for that
  reason. The event carries `ToolUseID`, unused here.
- Failure scenario: holder runs the full test suite; its `PostToolUse` (large output) is dropped.
  Later in the same turn the user presses Esc: `Working` still sees an open call, so the holder
  stays alive for 3x TTL (30 minutes) instead of expiring at the TTL.
- Fix: keep a per-record set of open `ToolUseID`s (fall back to the counter when the id is empty);
  delete on `PostToolUse`; clear on main prompt, Stop and session start/end as now.

### R2-L2 Board-change notifications wait behind an in-flight lease order build

- File: `apps/kira-space/internal/ade/leases.go:95-122` (`taskListOrder` holds `orderMu` across
  `b.Board`; `dropLeaseOrder` takes it), `apps/kira-space/internal/ade/board.go:349-355`.
- Description: `notifyBoard` now calls `dropLeaseOrder` first. While a lease call is building the
  board (git fan-out per repo, a `MarkBranchFacts` write), every `notifyBoard` caller blocks on
  `orderMu`, including writers that call it while holding a repo mutex (`ForcePush`, board.go:957-978).
  No deadlock found (Board takes no repo mutex), but board pushes to the UI and unrelated writers
  stall for one full board build.
- Failure scenario: two waiters ranked during a lease grant on a machine with several large repos;
  a concurrent force push holds its repo mutex until the board build ends, and the UI's board push
  lags by the same time.
- Fix: invalidate without the lock: an atomic generation counter bumped by `dropLeaseOrder`;
  `taskListOrder` stores its result only when the generation it read at build start is unchanged.
  Keep `orderMu` (or `singleflight`) only to collapse concurrent builds.

### R2-L3 G5 exemption for hooks/http.go names the wrong reason and covers the whole file

- File: `internal/conventiontest/conventions_test.go:23`, guard at `:234-236` and `isHeaderWrite`
  (`:345-358`).
- Description: the reason says "checks the bearer on Kira's own hook listener". The bearer read
  predates P620 and never tripped G5. The hit is `isHeaderWrite` on `w.Header().Set` in `writeReply`
  (response headers, `http.go:205,207`). The exemption is file-wide, so a real request-header
  injection added to `http.go` later passes G5 silently.
- Failure scenario: a later change in `hooks/http.go` builds an outbound request and sets an auth
  header read from Claude's config; G5 does not flag it.
- Fix: drop the exemption and make `isHeaderWrite` skip calls on an `http.ResponseWriter`'s
  `Header()` (receiver named by the handler's `w http.ResponseWriter` parameter, or match
  `w.Header()` where `w` is that type via `go/types`). If kept, correct the reason to "sets response
  headers on Kira's own hook listener".

### R2-L4 Doc and guard leftovers from round 1 fixes

- File: `docs/ARCHITECTURE.md:3250`, `docs/ARCHITECTURE.md:2927`, `scripts/check-conventions.ts:552`,
  `scripts/check-conventions.spec.ts:275`.
- Description: (a) the P589 anatomy line still lists `individualOnly` among `ApprovalKindDef`
  fields; the field is gone. (b) The Rank line says the book reads the order only with two or more
  waiters; `Acquire` reads it once any queue has one (`lockRanked(1)`). (c) N17's approve word list
  keeps `approveAll` (redundant: `approve` plus an uppercase letter already matches) and the N17
  spec fixture uses the deleted key `workbench.approvals.approveAll.label`.
- Failure scenario: a reader trusts the doc and adds `individualOnly` to a kind def, or expects no
  board build on a first waiter's acquire.
- Fix: remove `individualOnly` from line 3250; state "one or more waiters on acquire, two or more on
  release, sweep and snapshot"; drop `approveAll` from the word list and use a live key (for example
  `workbench.approvals.approve.label`) in the spec fixture.

## Checked, nothing found

- Round 1 fixes hold: R1-H1 (`AdeHandoverPrompt` opens once per `PromptHost` key, stays mounted
  across refetch), R1-H2 (holder with no open tool call expires at the TTL; open call bounded by 3x
  TTL), R1-M1 (reply written and flushed before `OnEvent`; failed write re-queues and clears
  `suppressIdle` before `OnEvent` sees the Stop, so the idle path types it), R1-M2 (live headless run
  alive via `leaseState.runs`, cleared by `endLeases`), R1-M5 (rank read lazily, cached 5 s, dropped
  on board change; a drop during a build waits for it, so no stale order survives), R1-M6, R1-M7,
  R1-L1, R1-L2 (eviction, shutdown via Wails `ServiceShutdown`, commit and swap under one lock),
  R1-L3 (files read in Go, only pasteboard calls on the main thread; cgo pointer rules respected:
  the Go slices hold C pointers only), R1-L4, R1-L5 (TanStack awaits the `onSettled` invalidate, so
  `isPending` covers the refetch; backend `saveMu`).
- Reply-first ordering: `OnEvent` for a Stop runs microseconds after the flush, while the agent's
  next hook needs a model round trip; no inversion found.
- Lock order: `leaseAlive` and `flushLease` take `lease.mu` before the tracker lock, never the
  reverse; tracker callbacks run unlocked.
- "Approve all" removal: no current or shipped requirement names bulk approve (P520's row is
  "Aggregated MCP approvals: one dialog, minimise to toast, long approval timeouts"); site, i18n,
  specs and the registry are cleaned apart from R2-L4. Removal does not narrow a user requirement.
- SPEC rows P617-P621 against their wording and CLAUDE.md frontend and test rules: nothing new
  beyond round 1.
