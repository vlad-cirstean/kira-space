# P623 audit B1: agent-facing MCP tool latency

Scope: `internal/agentmcp` handlers, `internal/llmprompts`, tool registration and MCP transport,
plus the `ade` backends they call (agenttools, branchrequest, branchrename, branchchange,
artifacts, taskcreate, orchestrator, sandbox). Lease/approval backend internals, harness, hooks,
terminal and session tracking belong to B2; only cross-boundary hot spots appear here.

Read-only audit. Discovery through `codegraph_explore`. Line numbers are at `b64bed79`.

Ranked by user-visible impact. Feedback surface for every fix: extend P618's `useAdeOpsStore`
(`frontend/src/ade/v2/state/adeOps.ts`), `AdeOpStatus.vue` chip and vue-sonner toast. Never add a
parallel mechanism.

## Shared feedback proposal (referenced as FB below)

The adeOps store today is client-initiated only. `run(op, fn)` wraps a frontend promise, and
`AdeOpKind` is `removeBranch | removeRepo | archive`. Agent-started work has no frontend promise,
so it never shows. Backend ops already reach the frontend: `oplog.Log.Start` publishes, and
`state/ops.ts` (`createOpLogStore` over `onOpUpdate`/`onOpBatch`) holds them with `TaskID`.

Fix:

- Add backend-sourced entries to adeOps. A watcher maps running ops from the ops store (category
  MCP or setup, with `taskId`) to adeOps entries keyed by task.
- Add `AdeOpKind` values `agentBranches`, `agentRename`, `agentChange`, `agentFinish`.
- Add an `ade` oplog op around each slow post-approval phase (B1-2), not only the MCP call op.
  Its subject is the task. `AdeOpStatus` renders it on the card as is.
- Toast on finish or failure only for ops over a threshold (about 2s), so fast calls stay quiet.
  Reuse `toast.loading`/`success`/`error` from `run`.

Agent side: results that start async work say so in the result body ("preparing", plus the next
call to make). `BranchInfo.Next` already does this for setup; extend the same field, never add a
second hint channel.

## Findings

### B1-1 finish_step / handover(finished) reply blocks on full artifact gather

- Where:
  - `internal/agentmcp/mcp.go:335` `finishStep` calls `finishContent`.
  - `internal/agentmcp/handover.go:62` does the same for `finished: true`.
  - `internal/agentmcp/artifacts.go:126` `artifactsReply` runs `ListArtifacts` with a 10s timeout
    (`artifactsReplyTimeout`).
  - It leads to `apps/kira-space/internal/ade/artifacts.go:76` `TaskArtifacts`, which fans out
    sources with a 5s timeout each (`artifactSourceTimeout`).
- What is slow:
  - `repoArtifacts` (`artifacts.go:286`) runs `resolveRepoHost` once per branch, in sequence.
    Each call is `BranchURL` plus `ResolveBranchChangeRequest`, which are gh/az CLI network round
    trips.
  - It also runs `repoFacts` over every live row of the repo, which means board-level git facts
    for branches the task does not own.
  - `trackerSource` makes Jira network calls.
  - This sits on the critical path of every step end. The agent waits for this reply before it
    exits, so run end, card state and the next chained step all wait too.
  - Typical cost: one CLI spawn plus network per branch, about 0.3–1.5s each. With 3 repos it is
    several seconds, capped at 10s.
- Fix:
  - Build the finish reply from cheap sources only: stored artifacts, task links, branch rows and
    a change request already cached on the row or the board cache.
  - Skip `repoFacts` and live host resolution in this path.
  - Alternatively, ack at once with a one-line "step finished", and let the agent fetch
    artifacts through `operations`/the artifacts tool when needed.
  - Keep the full gather for the UI artifacts panel, where it already runs off the critical path.
- Effort: S (cheap-source variant) / M (board-cache read).
- Risk: low. The agent sees a change request created seconds earlier only on its next query,
  which is acceptable.
- Feedback:
  - UI: none needed once the call is fast. Until then, the MCP op shows "running" in the ops
    store only, and nothing on the card.
  - Agent: none today; the reply is late, not marked pending.

### B1-2 Post-approval branch work runs serial, under taskMu, with no card feedback

- Where:
  - `apps/kira-space/internal/ade/branchrequest.go:771` `finishBranchRequest` calls
    `createApproved` for each slot.
  - `createApproved` leads to `createBranch`, then `ensureWorktree` (`setup.go:118`, takes
    `repoMutex`, calls `BranchInventory`), then `namedWorktree`/`rowWorktree` (`setup.go:140`,
    calls `BranchInventory` again), then `git worktree add`, then `startSetup`.
  - `branchrename.go:303` `applyRename` holds `taskMu` and `repoMutex`. With `renameRemote` it
    also pushes the new name and deletes the old one (`renameRemoteBranch` L390), both network.
  - `branchchange.go:231` `applyChange` re-runs the git checks from `changeRefusal`
    (L112) under `taskMu`.
- What is slow:
  - The user clicks Approve and the dialog closes. Then one `git worktree add` runs per slot,
    serially across repos. Each also costs two `BranchInventory` calls (two git spawns each:
    `for-each-ref` over all heads and remotes, plus `worktree list`), and remote push/delete for
    a rename.
  - All of it holds `taskMu`, so every other op on that task waits: task_info, declare_repos,
    UI branch actions.
  - `awaitBranchRequest` (L459–485) releases the run note before this phase. The card shows
    nothing until the final `notifyBoard`.
  - On a large repo, `worktree add` plus checkout takes seconds. Three repos run back to back.
- Fix:
  - Create slots for different repos in parallel (bounded, reuse `boardFanOut`). Slots in the
    same repo stay serial under `repoMutex`.
  - Do not hold `taskMu` across git. Plan under `taskMu`, run git unlocked, re-take `taskMu` to
    commit rows, and re-check for a conflicting change.
  - Pass the inventory from `ensureWorktree` into `namedWorktree`/`rowWorktree` instead of
    fetching it twice.
  - Hold the run note until creation ends, or replace it with the FB chip
    (`agentBranches`/`agentRename`/`agentChange`). Toast on failure always, and on success only
    when the op ran over the threshold.
  - Call `notifyBoard` per created slot, so rows appear as they land.
- Effort: M.
- Risk: medium. Dropping `taskMu` across git needs the re-check, or a concurrent rename or
  change can race a create on the same row.
- Feedback:
  - UI: none today between approval and completion. Needs the FB chip plus a toast.
  - Agent: the tool result already carries per-branch `Next` (setup pending). Keep that.

### B1-3 task_info(waitSeconds) returns stale state and polls

- Where:
  - `internal/agentmcp/space.go:337–374` `addTaskInfoTool` calls `TaskInfo` before
    `BranchStatus`.
  - `apps/kira-space/internal/ade/agenttools.go:385–404` `BranchStatus` polls `GetSetup` every
    1s (`time.After`) up to `maxWaitSeconds` (600).
- What is slow:
  - The agent waits up to 10 min for setup, then gets a `Repos` block computed before the wait.
    Setup state in `Repos` is the pre-wait value, which contradicts the branch status. That
    prompts another task_info call: one more full `spaceInfo` (see B1-4).
  - The 1s poll adds 0–1s latency after setup ends, plus one DB read per second per waiter.
  - The agent gets no progress during the wait. A client timeout looks like a hang.
- Fix:
  - Compute `TaskInfo` after the wait returns.
  - Replace the poll with a per-branch done signal: a channel map in the setup tracker, closed by
    `runSetup` on exit or by the `UpsertSetup` terminal write. Keep a long fallback tick for
    safety.
  - If the request carries a progress token, send `NotifyProgress` per state change (setup
    started, script running, done). Pattern exists at `internal/memory/mcpserver/server.go:260`.
- Effort: S (reorder) + M (signal, progress).
- Risk: low.
- Feedback:
  - UI: setup already has its own op (`opSetup`, `setup.go:324`) and shows. Fine.
  - Agent: MCP progress notifications, plus a fresh `Repos` block.

### B1-4 task_info / declare_repos per-call cost and result size

- Where:
  - `apps/kira-space/internal/ade/agenttools.go:147` `spaceInfo`, with
    `registeredCandidates` at L105.
- What is slow, for each call:
  - `loadTaskCtx`: `BranchesLive` for the whole space, plus `RepoConfig.List`.
  - `RepoConfig.List` again.
  - `attachCache.load` per repo: `BranchInventory` (two git spawns), plus `BranchesLive` again,
    plus `CodeRepos.Get`.
  - `GetSetup` per branch.
  - The result carries every registered repo with its path on every call. That is a token cost
    when agents poll it, and it scales with the space, not the task.
- Fix:
  - Reuse the `loadTaskCtx` live rows and repo list inside `attachCache`. Drop the second
    `RepoConfig.List` and the per-repo `BranchesLive`.
  - Batch setup reads with `SetupByBranch` (already exists for artifacts).
  - Include `registered` only on the first call per run, or when `declare_repos` refuses
    (unknown repo), or behind an `includeRegistered` arg.
- Effort: M.
- Risk: low.
- Cross-boundary: `loadTaskCtx` and `BranchInventory` cost is shared with board recompute (A1).
  Fix the inventory cache there once, not twice.
- Feedback: none needed. The call is synchronous and should be fast.

### B1-5 Approval waits give no card feedback for session callers

- Where:
  - `branchrequest.go:459` `awaitBranchRequest`.
  - `taskcreate.go:198` `SpawnTask`.
  - Both hold the run note through `holdRunNote` only when `RunID` is set.
- What is slow:
  - The wait itself is by design (`defaultBranchRequestWait` and `defaultTaskRequestWait`, 30m
    each). It runs unlocked, which is correct.
  - The approvals queue push (`internal/approvals/queue.go:139`, then `OrderedEmitter`, then
    `setQueryData` in `approvalsQueries.ts`) is fast.
  - Problem: an interactive session (no `RunID`) waiting on approval shows nothing on the card.
    The user sees an agent "stuck" with no hint that it waits on them.
- Fix:
  - Hold the note, or post an FB chip ("waiting for approval"), for session callers too, keyed
    by task.
  - The approvals panel already lists the request. The chip links to it.
- Effort: S.
- Risk: low.
- Feedback:
  - UI: chip, plus the existing approval dialog.
  - Agent: the result already reports approved or denied.

### B1-6 fetch_remote / push_branch hold repoMutex across network and credential prompt

- Where:
  - `apps/kira-space/internal/ade/sandbox.go:391` `FetchRemote` and `sandbox.go:418`
    `PushBranch` hold `repoMutex` across `RunRemote`.
  - The tools are in `internal/agentmcp/sandboxgit.go`.
- What is slow:
  - A fetch or push over a slow link holds the repo lock for its whole duration.
  - So does an askpass credential prompt, which waits on the user indefinitely.
  - Meanwhile every other agent's worktree creation (B1-2), rename and UI branch op on that repo
    blocks.
- Fix:
  - Run network git outside `repoMutex`. Fetch and push do not touch worktree layout.
  - If a ref-update race matters, use a separate per-repo remote lock that worktree ops do not
    take.
- Effort: S.
- Risk: low to medium. Check that the push reads branch state once before unlocking.
- Cross-boundary: the askpass prompt flow is B2's.
- Feedback:
  - UI: the credential prompt is visible. The blocked ops on other tasks show nothing, which
    B1-2's chip would cover.

### B1-7 sharedServer stats prompts per request and rebuilds under the server lock

- Where:
  - `internal/agentmcp/mcp.go:374–402` `startLocked`/`getServer`: every POST calls `registered`
    (a linear scan under `s.mu`) and then `sharedServer` (L405–418).
  - `sharedServer` holds `s.mu`, calls `s.prompts.Revision()` (an `os.Stat` in
    `llmprompts.Store`), and after any prompt edit runs a full `buildMCPServer` (L265) under
    `s.mu`.
  - `Register` (L462) builds one per-run server per run.
  - `llmprompts.Render` (`render.go:28`) re-parses the template set on every call, and
    `InputSchema` reflects per build.
- What is slow:
  - One stat per request, serialized across all concurrent agents. That is microseconds, so it
    is minor.
  - After a prompt edit, the first request pays template parse plus schema reflection for every
    tool while holding `s.mu`. Every concurrent call stalls for that window, about ms-level.
  - Stateless transport also builds a session per POST. That is inherent to `Stateless: true`
    and cheap.
- Fix:
  - Build outside the lock: read the revision, build unlocked, then swap under the lock if the
    revision has not changed (double-checked).
  - Cache the parsed template per `(id, revision)` in `llmprompts`.
  - Optionally cache `Revision` for about 1s, or watch the file.
- Effort: S.
- Risk: low.
- Impact: low. Listed because it is the only lock every MCP call takes.
- Feedback: none needed.

### B1-8 Result size: typed outputs doubled, git output up to 64KiB

- Where:
  - go-sdk typed tool handlers fill `StructuredContent` and also a `TextContent` JSON copy.
  - `internal/agentmcp/gittool.go`: `maxGitOutput` is 64KiB.
- What is slow:
  - Every typed result goes on the wire twice. Whether Claude Code feeds both copies to the model
    is unverified; if it does, token cost doubles for task_info and list_tasks.
  - One git call can return about 16k tokens.
- Fix:
  - Return explicit compact `Content` for large structured results (summary text, structured
    data kept).
  - Lower the git cap to about 16KiB, with a truncation note telling the agent to narrow the
    command (path, `-n`).
  - Measure first: check one real transcript for the duplicate before changing.
- Effort: S.
- Risk: low.
- Feedback: agent-side only; the truncation note.

### B1-9 Some tools ignore callCtx

- Where:
  - `internal/agentmcp/mcp.go:542` `callCtx` ties a call to its registration.
  - Lease tools (`leases.go`), handover, git (`gittool.go`), sandbox git (`sandboxgit.go`) and
    review (`review.go`) use the raw request context.
- What is slow: when a run is released or killed, an in-flight git or network call keeps running
  until its own timeout (`gitTimeout` is 2m; the transport timeout `harness.MCPCallTimeout` is
  35 min). The process and lock stay held after the run ended.
- Fix: route every handler through `callCtx`.
- Effort: S.
- Risk: low.
- Feedback: none.

## Nothing real

- Token verify (`mcp.go:362`): salted SHA-256 per token, cheap.
- Approval push round trip: push-based, no polling.
- Tool descriptions: headless tool text about 4.8KB, argument text about 3.1KB, space
  instructions 378B. These are terse and built once per server, so no per-call prompt cost.
- Lease tools: non-blocking (queued state), no waits held in handlers.
- `create_task` post-approval: `createChild` (`taskcreate.go:318`) runs under `taskMu` with DB
  work only, so it is cheap. The wait before it is B1-5.
- `mcpRecorder` (`opsrecord.go:122`): one `GetTask` read plus async persist per call. It could
  cache the subject, but the gain is marginal.
- `reply_review_comment`: cheap.

## Cross-boundary notes

- `notifyBoard` (`board.go:349`) fires on every request open and close, triggering a board
  recompute. Its cost is A1's. B1-2's per-slot notify must ride whatever coalescing A1 adds.
- `BranchInventory` duplication (B1-2, B1-4) overlaps A1's board git-facts cache.
- Askpass under `repoMutex` (B1-6): the prompt mechanism is B2's.
- `list_tasks` (`orchestrator.go:142`) runs `loadSpaceFacts` over the whole space, plus
  `buildTaskCtx` per child. Its cost is the same as board load (A1). It is not hot unless an
  orchestrator polls it.
