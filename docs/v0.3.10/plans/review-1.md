# v0.3.10 code review round 1

Base: `8165b1d3` (v0.3.9 merge). Head: `db34761e`. Scope: `git diff 8165b1d3..HEAD`, phases P617-P621
plus full-run fixes. One Opus reviewer, three dimensions. Findings only; fixer works from this file.

Counts: High 2, Medium 7, Low 5.

## High

### R1-H1 Routed handover dialog resets typed notes on every board push

- File: `apps/kira-space/frontend/src/ade/v2/stage/AdeHandoverPrompt.vue:22-49`, with
  `apps/kira-space/frontend/src/ade/queries.ts:82-85` and
  `apps/kira-space/frontend/src/ade/v2/state/adeStageDialog.ts:93-101`.
- Description: `ready` requires `!handovers.isFetching`. Every `onAdeTaskBoard` push invalidates
  `handoversPrefix`, so `isFetching` turns true on any board change. `ready` drops to false,
  `v-if="ready"` unmounts `AdeStageDialog`; when the refetch lands, the `watch(ready)` fires again
  and calls `store.open(...)`, which resets `typed`, `picked`, `closeChoice` and `seeded`.
- Failure scenario: agent finishes a stage, handover prompt opens, user types notes. Any git ref
  move, session event or other board push (frequent while agents work) unmounts and reopens the
  dialog with the draft text; user's typed notes and chosen target stage are lost.
- Fix: gate only the first open. Use `handovers.isSuccess` (or `isFetched`) without `isFetching`,
  and open once (`watch` with `once: true`, or a local `opened` flag); keep `AdeStageDialog` mounted
  after first ready. Extend `approvals-first-frame`/handover spec: board push while dialog open keeps
  typed text.

### R1-H2 User interrupt leaves turn state busy forever: lease TTL never fires, grant never typed

- File: `apps/kira-space/internal/ade/tracker.go:902-944` (`trackStatusLocked`),
  `apps/kira-space/internal/ade/leases.go:107-109` (`leaseAlive`).
- Description: turn turns busy on any prompt or tool event and returns to idle only on a main
  `Stop`. Claude Code does not run the `Stop` hook when the user interrupts (Esc). After an
  interrupt the record stays `turnBusy` until the next prompt. `leaseAlive` treats `turnBusy` as
  mid-turn, so `Sweep` never expires the holder; `SendIfIdle` never fires for a waiter.
- Failure scenario: agent holds lease `push`, user presses Esc mid-turn and leaves. Session sits at
  an idle prompt holding the lease indefinitely; TTL (spec: "TTL with heartbeat") never applies;
  every waiter blocks until user force-releases or closes the session. Same for a waiter: grant
  message waits for the user's next prompt instead of being typed.
- Fix: do not treat busy as alive without bound. Options: cap the alive exemption (e.g. busy counts
  as alive only while the last hook event is younger than a busy ceiling, such as 2x TTL, using the
  tracker's `lastActive`); or treat a long silence after the last `PreToolUse` with no matching
  `PostToolUse` differently from silence after `PostToolUse`/`UserPromptSubmit`. Add a tracker test:
  prompt then no Stop, TTL passes with no events, holder expires.

## Medium

### R1-M1 Hook reply removes lease notices before delivery is certain; reply waits on OnEvent work

- File: `internal/harness/claude/hooks/http.go:160-168`, `apps/kira-space/internal/ade/leases.go:175-185`,
  `apps/kira-space/internal/ade/tracker.go:1050-1068`.
- Description: `Reply` takes (deletes) pending notices and sets `suppressIdle`, then `OnEvent` runs
  synchronously (tracker DB writes `SetLastActive`/`SetUnseen`, `onIdle`, PTY write, bridge emit)
  before `writeReply`. The shim gives curl `--max-time 2`. Before P620 this latency was harmless
  (output discarded); now the reply body carries the only copy of the notice.
- Failure scenario: SQLite writer busy (sync, import) delays `SetLastActive` past 2s on a `Stop`
  carrying a grant. curl times out, Claude never sees the block, the grant notice is gone, and
  `suppressIdle` already turned the Stop into busy, so the session is not idle and gets nothing
  typed; holder sits with a granted lease it never heard about until TTL (and only if R1-H2 does not
  hold it alive).
- Fix: write the reply before running `OnEvent` (flush response, then run `onEvent`), or run
  `onEvent` work after the response; keep notices pending until the reply is written (re-queue on
  write error). Keep reply path free of DB writes.

### R1-M2 Headless lease holders expire during a long tool call

- File: `apps/kira-space/internal/ade/leases.go:107-109`, `apps/kira-space/internal/ade/opsrecord.go:123`.
- Description: `leaseAlive` returns false for every `run:` key, and headless runs have no hook
  events, so their only heartbeat is a Kira MCP tool call (`mcpRecorder`). A Bash or other non-Kira
  tool never touches the lease. Spec: "TTL with heartbeat (any holder tool call extends)".
- Failure scenario: headless step run acquires `full-suite`, then runs a 15 minute test command in
  one Bash call. At 10 minutes `Sweep` expires it and grants the next waiter, which starts a second
  full suite on the same resource: the collision leases exist to prevent.
- Fix: a live headless run is alive (`b.live[runID]` present); run end already calls `endLeases`.
  For a run key with a session id, check run liveness too.

### R1-M3 `change_branch` switches a worktree while other work runs in it

- File: `apps/kira-space/internal/ade/branchchange.go:134-188`, `apps/kira-space/internal/ade/branchrename.go:212-226`.
- Description: `changeRefusal` uses `rowBusyRefusal` (sandbox, rebase, prepare script) and dirty
  count. It does not refuse an active step run on the row, a script-run claim (`claimOn`) or a
  second open agent session on the same row. `donorBusy` has those checks but `holderBlock` runs it
  only for `holderTask`, never for `holderOwn`.
- Failure scenario: smart script run (tests) works in the row's folder; the TUI agent calls
  `change_branch`, user approves; `git switch` swaps files under the running script, which then
  tests or commits on the wrong branch. Same with a headless step run or a second agent session
  writing into that folder.
- Fix: in `changeRefusal`, refuse when `HasActiveStepRunOn(sb.ID)`, `claimOn(sb.ID) != ""`, or a
  running TUI other than the caller's session is bound to the row. Add flow cases to
  `TestBranchChange`.

### R1-M4 Lease injection types into an idle prompt the user may be typing in

- File: `apps/kira-space/internal/ade/tracker.go:1009-1025` (`SendIfIdle`).
- Description: idle means "last main Stop had no background work". It does not mean the input box
  is empty. `SendIfIdle` pastes the message and presses Enter.
- Failure scenario: user has a half-typed prompt in an idle agent session when a lease grant or
  "Request release" fires. The lease text is appended to the user's draft and submitted at once,
  sending an unfinished prompt.
- Fix: skip injection when the terminal received user keystrokes since the last Stop (the terminal
  registry sees input writes), or when the terminal is focused, falling back to the hook reply. At
  minimum, prefix the paste with a clear-line sequence only after confirming nothing was typed.

### R1-M5 Lease rank recomputes the full board on every lease call and snapshot

- File: `apps/kira-space/internal/leases/leases.go:124-136,271,311,345,419,439`,
  `apps/kira-space/internal/ade/leases.go:78-90`, `apps/kira-space/internal/ade/board.go:492-529`.
- Description: `Order` is `taskListOrder`, which calls `TaskBoard.Board`: git facts per repo (fan
  out), rebase-check queueing and a `MarkBranchFacts` DB write. `Acquire`, `Release`, `ForceRelease`
  and `Snapshot` call `b.ranker()` eagerly, even with zero or one waiter or a holder heartbeat. Each
  lease push makes every window refetch `LeaseSnapshot`, each costing one Board.
- Failure scenario: three agents polling `acquire_lease`/`lease_status` (the tool text tells
  headless runs to call again later) plus two open windows: each call runs a full board build with
  git spawns and a DB write; tool latency and CPU climb with repo count.
- Fix: compute rank lazily, only when sorting two or more waiters (`promote`, `status`, `Snapshot`
  with `len(waiters) > 1`). Derive order from DB-only task data (statuses, plan, dates) without
  `repoFacts`, or cache the last board order and refresh it on `OnBoard`.

### R1-M6 Removing a queue drops its waiters silently

- File: `apps/kira-space/internal/leases/leases.go:245-253`.
- Description: `SetQueues` sends a revoked grant only to the holder of a removed queue. Waiters are
  deleted without a notice, yet `acquire_lease` told them "Kira messages you in this session when the
  lease is granted. Do other work meanwhile."
- Failure scenario: user removes queue `port-3000` while two agents wait. Both keep waiting for a
  message that never comes, stalling their tasks until a human notices.
- Fix: emit a notice to each dropped waiter ("queue X was removed; you no longer wait for it"),
  through the same `deliverLease` path.

### R1-M7 Approve all approves prompts the user never saw

- File: `packages/workbench/src/approvals/ApprovalsDialog.vue:36-41,60-82`.
- Description: P618 made the dialog show one prompt at a time, but kept the bulk footer. "Approve
  all (N)" approves every queued item, N-1 of which are not on screen. Before, all items were in a
  scrollable list.
- Failure scenario: an agent queues a harmless query and a destructive one (`DELETE`, branch
  change, kube write). User reads the first, clicks "Approve all", and approves the destructive one
  unseen. The approvals gate exists to stop exactly that.
- Fix: drop "Approve all" (keep "Deny all", which is safe), or require a confirm listing every
  item's kind and target before bulk approve. Update `approvals-one-at-a-time` spec.

## Low

### R1-L1 Tracker never clears turn state when a record stops

- File: `apps/kira-space/internal/ade/tracker.go:731-750` (`forgetLocked`), `:920-921`.
- Description: `Abort` deletes `turn` and `suppressIdle`; `forgetLocked` (used by `Stop` and
  `Reconcile`) does not, and `SessionStart` clears `turn` but not `suppressIdle`.
- Failure scenario: session killed after a Stop block reply; user resumes the same record later.
  The stale `suppressIdle` makes the first real Stop of the resumed session read as busy, so a
  lease message waits for hooks instead of being typed. Entries also accumulate per record for the
  app's lifetime.
- Fix: delete `turn` and `suppressIdle` in `forgetLocked`; clear `suppressIdle` on main
  `SessionStart`.

### R1-L2 Clipboard batch temp dirs leak and concurrent commits can delete the live batch

- File: `apps/kira-space/internal/bridge/proofclip.go:59-68,103-127,132-142`,
  `apps/kira-space/internal/bridge/proofshots.go:39-53`.
- Description: (a) the batch on the clipboard (`lastClip`) is never removed at shutdown; (b) a batch
  evicted from `proofShotFolders` past `maxShotFolders`, or abandoned when the window closes
  mid-walk, keeps its dir because `CloseShotClip` sees an unknown id; (c) `WriteImages` and the
  `lastClip` swap are not under one lock: commit A writes, commit B writes, B swaps first, then A
  removes B's dir while B's items are on the clipboard.
- Failure scenario: (a, b) proof screenshots (may show DB rows or secrets) persist in TMPDIR after
  exit; (c) two proof windows copy at once and the clipboard's `public.file-url` items point at
  deleted files, so pasting into Finder fails.
- Fix: remove `lastClip` on service close; remove the dir on eviction of an uncommitted batch; hold
  `clipMu` across `WriteImages` and the swap.

### R1-L3 Darwin pasteboard: test readback compiled into the app; file reads on the main thread

- File: `apps/kira-space/internal/imageclip/readback_darwin.go:1-85`,
  `apps/kira-space/internal/imageclip/imageclip_darwin.go:21-43,93-95`.
- Description: `readback_darwin.go` has only test helpers but no test-only build tag, so it ships
  in the macOS binary. `kira_write_images` reads every PNG (`dataWithContentsOfFile`) inside
  `InvokeSync`, on the AppKit main thread. Memory and thread use otherwise look correct (ARC,
  autorelease pool, C strings freed after the synchronous invoke).
- Failure scenario: copy-all of 10 full-resolution tabs (several MB each) stalls the UI while the
  main thread reads files; dead helper code ships to users.
- Fix: read the files into `NSData` off the main thread (or pass bytes from Go) and do only
  `clearContents`/`writeObjects` on main; move the helpers behind a build tag such as
  `darwin && cgo && imagecliptest`, or into an internal test-only package.

### R1-L4 Expiry message hardcodes 10 minutes

- File: `apps/kira-space/internal/ade/leases.go:118`.
- Description: TTL comes from `deps.LeaseTTL`; the agent-facing text says "expired after 10 minutes
  of silence" whatever the TTL is.
- Failure scenario: a configured or test TTL other than 10 minutes tells the agent a wrong window.
- Fix: format the message from `b.lease.book.TTL()`.

### R1-L5 Queue editor: lost updates and remove without confirm

- File: `apps/kira-space/frontend/src/ade/v2/state/adeLeases.ts:33-48`,
  `apps/kira-space/internal/ade/leases.go:284-295`.
- Description: `removeQueue` builds the new list from the cached query; two removes in quick
  succession send stale lists, so the second re-adds the first. Backend `Save` then `SetQueues` is
  not atomic, so concurrent saves can leave DB and book order apart. Removing a queue revokes its
  holder with no confirm, while Force release asks.
- Failure scenario: user removes `a` then `b` quickly: `a` comes back. Removing a held queue revokes
  a running agent's lease with one click.
- Fix: send a remove-by-name mutation (or disable the buttons while a save is pending); serialize
  `SaveLeaseQueues` with a mutex; confirm remove when the queue has a holder.

## Dimensions with nothing found

- GraphQL fix (`githost/github/detail.go`): aliases are correct, every GitHub document in the package
  is validated by `TestGraphQLDocumentsValidate`. Nothing found.
- Background ops store (`adeOps.ts`): one op per id, toasts and chips clear on settle. Nothing
  found beyond normal backend serialization.
- Standing rules: new components are `<script setup lang="ts">`, use shadcn-vue primitives,
  Tailwind, VueUse (`useTimeout`, `useDebounceFn`), TanStack Query for lease and clip state, one
  concern per new store (`adeOps`, `adeLeases`); each feature has a flow test plus UI spec. Nothing
  found.
