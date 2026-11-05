# P520 research: standard async result delivery to the agent

Question (user): is there a standard MCP way for a server to deliver a tool result after the call
returns, esp. after approvals finish? Alternative to `await_results` long-poll in `P520.md`.

Answer: **yes. MCP Tasks extension (SEP-2663, `io.modelcontextprotocol/tasks`), and Claude Code
2.1.318 implements it end to end over stateless Streamable HTTP.** Verified live here (section 2).
Server answers `tools/call` at once with a task handle; Claude moves the call to the background,
polls `tasks/get`, and injects the result into the conversation as a new turn when the task
completes, even while the agent sits idle. No `await_results` tool, no model-side polling.

## 1. Evidence base

- Discovery: `codegraph_explore` over `internal/harness/claude` (`Script`, `RunHeadless`,
  `InteractiveCommand`, `hooks.MCPToolTimeoutEnv`), `agentmcp.Server.Register`, `dbmcp`
  `bindHTTP`/`withPanicRecovery`/`bindRequestCancellation`, `mcpinstall.Installer`,
  `ade.Tracker.Send`/`pasteBytes`, `ForwardReviewComments`.
- Installed `claude` 2.1.318 (build 2026-11-03): binary strings, plus live probes with a mock
  Anthropic API (`ANTHROPIC_BASE_URL` to a local stub, fake key, no real credential touched) and a
  Python stateless MCP server. Clean env (`env -i`) required: this container sets
  `CLAUDE_CODE_REMOTE`/`CLAUDE_CODE_ENTRYPOINT=remote`, which turns Tasks off.
- go-sdk v1.8.0 source in module cache (latest published, 2026-09-04).
- SEP-2663 text: https://modelcontextprotocol.io/seps/2663-tasks-extension (Final, 2026-04-27).

## 2. Findings per question

### 2.1 MCP Tasks

Spec (SEP-2663; the 2025-11-25 core `tasks` of SEP-1686 is superseded):

- Server-directed. Client lists `extensions["io.modelcontextprotocol/tasks"]: {}` in per-request
  `_meta["io.modelcontextprotocol/clientCapabilities"]`; server advertises the same key in
  `server/discover` capabilities. Server decides per call whether to make a task.
- `tools/call` result, flat (not nested under `task`):
  `{"resultType":"task","taskId","status":"working","statusMessage","createdAt","lastUpdatedAt","ttlMs","pollIntervalMs"}`.
  Server MUST NOT return it before `tasks/get` would resolve that id.
- `tasks/get` returns the task with `resultType:"complete"`; `status` `working` |
  `input_required` (with `inputRequests`, MRTR) | `completed` (with `result`, a CallToolResult) |
  `failed` (JSON-RPC error) | `cancelled`. `tasks/update` (input responses), `tasks/cancel`
  (eventually consistent). Optional `notifications/tasks` via `subscriptions/listen`.
- Polling only: works on stateless HTTP, no session, no GET stream. Fits `agentmcp` and `dbmcp`
  exactly as they are.

Claude Code 2.1.318 (verified live, interactive TUI, clean env):

- Negotiates protocol `2026-07-28` via `server/discover` (headers `mcp-method`,
  `mcp-protocol-version`), declares the tasks extension on every request.
- Server returned the flat task result: tool_result to the model was
  `MCP tool "probe/slow" started server-side task t…. It was moved to the background as task k… and keeps running on the server; you'll receive a notification with the result when it completes…`.
  Claude polled `tasks/get` every `pollIntervalMs` (1 s). On `completed` it started a new turn:
  `<task-notification><status>completed</status>…<result>TASK-RESULT-42</result>`.
- 170 s task, agent idle after its reply: 170 polls, result delivered as new turn. No 60 s cut
  (each poll is a short request).
- Nested shape (`{"task":{…}}`) is rejected: `Unsupported result type 'task' for tools/call`.
- Gate (binary): Tasks on when `!CLAUDE_CODE_DISABLE_MCP_TASKS` and GrowthBook
  `tengu_glowing_lark`, whose default is `entrypoint === "cli" && !CLAUDE_CODE_REMOTE`. Live:
  headless `claude -p` (also with `--input-format stream-json`, also with
  `CLAUDE_CODE_ENTRYPOINT=cli`) declares no tasks extension; a forced task result there fails with
  the same `Unsupported result type` error. So: **TUI yes, `-p` no** under default flags.
- Status line in UI: "runs on the server; survives exiting this session". Restored after restart
  ("sep2663 sidecar" records). A per-user-message cap exists ("N tasks have been moved there since
  the user last wrote"); value not found.

go-sdk:

- v1.8.0 has no Tasks: `resultType` is an unexported type with only `complete`/`input_required`.
  Upstream: issue #626 (SEP-1686, open), #942 (SEP-2663, closed 2026-05), PRs #755 (tasks
  capability negotiation, open) and #1335 (Task type, open). Titles only; no release with it.
- v1.8.0 does have the seams to add it in Kira without a fork:
  - `ServerCapabilities.AddExtension("io.modelcontextprotocol/tasks", nil)` (advertised in
    `server/discover`).
  - `mcp.AddReceivingCustomMethod` registers `tasks/get`, `tasks/update`, `tasks/cancel` (not
    standard methods in v1.8.0's table).
  - `ServerRequest.ClientCapabilities()` reads per-request client caps (SEP-2575).
  - `Server.AddReceivingMiddleware` on `tools/call` can return a Kira result type. `Result` has
    unexported `isResult()`; a Kira struct embedding an SDK result type plus its own `MarshalJSON`
    should satisfy it. **Not built here: needs a spike** (see open item 3).

### 2.2 Progress, timeouts, holding a call open

From binary, verified where marked:

- Per-call timeout: per-server `timeout` (ms, ≥1000) in the MCP config, else `MCP_TOOL_TIMEOUT`.
  HTTP transport: default 60 s (verified: backgrounded 75 s call failed
  `tool "slow" timed out after 60s`). With `"timeout": 600000` in the server config the same call
  completed (verified). Studio's `mcpinstall` already uses `claude mcp add-json`, so it can write
  `timeout`; `agentmcp` writes its own MCP config file.
- Idle timeout: `CLAUDE_CODE_MCP_TOOL_IDLE_TIMEOUT`, default 300 s HTTP, 1800 s stdio; raised by
  per-server `timeout`; reset by progress ("sent no response or progress for Ns").
- A held synchronous call blocks the turn. Claude TUI auto-backgrounds any MCP call still running
  after 120 s (`tengu_mcp_auto_background`, default on; `CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS`
  overrides, `0` disables). Also backgrounds early when the user types a message. Result arrives as
  task notification. In `-p` only with `CLAUDE_AUTO_BACKGROUND_TASKS=1`, and verified: plain `-p`
  exits after its reply and kills the task (`status: stopped`); with `--input-format stream-json`
  and stdin held open, the result arrived as a new turn.
- Holding a call open for minutes works when the timeout is lifted; that is P520's current Kira
  launch posture (`MCP_TOOL_TIMEOUT=2100000`).

### 2.3 Other notifications

- Resource subscriptions: SDK method exists; no app-level `subscribeResource` call site found.
  Nothing reaches the model.
- `list_changed`: refreshes tool lists only.
- Logging `notifications/message`, sampling, elicitation: elicitation is a user dialog (form/url),
  not model context. Logging: not traced to model context; unverified.
- Channels: still gated (`tengu_harbor` default false, `allow_channels`, `channelsEnabled`), and
  "this connection's protocol version has no channel delivery path" on the modern era. P520 §1
  holds.

### 2.4 Hooks and session injection (Kira-launched only)

- Headless runs: `Script` = `claude -p --output-format stream-json --verbose`, prompt via
  `strings.NewReader` on stdin, closed after. No mid-run injection today. Switching to
  `--input-format stream-json` with stdin held open would allow injecting user turns, but that is a
  run-lifecycle redesign (run ends on stdin EOF, not on result).
- TUI sessions (Space): `Tracker.Send` already pastes into a running agent's PTY (bracketed paste +
  Enter), used by `ForwardReviewComments` and `agentnotify`. Works only for Kira-tracked Space
  TUI records, races user typing, and lands as user input. Not idiomatic for tool results.
- Hooks: `async`/`asyncRewake` command hooks exist ("wakes the model on exit code 2"). A Kira
  PostToolUse `asyncRewake` hook could wait for a result and wake the model. Kira-launched only,
  hook-timeout bound, non-standard. Inferior to Tasks.

### 2.5 OpenCode

Not installed here; not checked. Assume no Tasks support. It gets the synchronous fallback.

## 3. Comparison

| Approach | Kira TUI sessions (Space ADE TUI, Continue in terminal) | Kira headless `-p` runs | User's own Claude sessions (Studio `kira-db`) | OpenCode | Complexity | Failure modes |
|---|---|---|---|---|---|---|
| A. SEP-2663 Tasks | yes, verified | no (client declares nothing); fallback | yes (TUI), verified on same binary | fallback | medium: Kira-side Tasks layer on go-sdk v1.8.0 seams; swap to SDK later | flag `tengu_glowing_lark` flipped off remotely; per-message task cap; task store lost on Kira restart (return `failed`/unknown id) |
| B. P520 `await_results` long-poll | yes | yes | yes, but 50 s cap per await and model must re-call | yes | high: new tool, prompt texts, model discipline | model forgets to await; results lost if session ends; extra turns and tokens |
| C. Sync hold + lifted timeouts (+ Claude auto-background at 120 s) | yes; blocks turn until 120 s then background | yes; blocks (agent waits anyway) | yes once Kira writes per-server `timeout`; else 60 s cut | yes, if its own timeout allows | low: mostly today | 60 s cut where config not written; idle 300 s without progress; plain `-p` kills backgrounded calls |
| D. Channels | gated | no | needs user flags | no | high | org policy, era block |
| E. PTY paste / asyncRewake hook / stream-json stdin | partial (Space TUI) / Kira-launched / headless redesign | E3 only | no | no | high | races, non-standard, lands as user input |

## 4. Recommendation

Replace the `await_results` design with **A plus C as fallback**, one code path in
`internal/mcpasync`:

1. Every Kira MCP server advertises `io.modelcontextprotocol/tasks` and serves `tasks/get`,
   `tasks/update` (reject: no input requests), `tasks/cancel`.
2. Per call: if the request's client caps declare the extension, run the job; answer inline when
   done within the inline window (keep P520's 1 s), else return a flat `CreateTaskResult`
   (`statusMessage` e.g. "waiting for your approval in Kira Studio", `pollIntervalMs` ~1000–2000,
   `ttlMs` ≥ approval timeout plus margin). Approval-gated calls return a task at once.
3. Client without the extension (headless `-p`, OpenCode, Tasks flag off): hold the call open as
   today and finish synchronously. Send progress when the client gave a progress token (keeps idle
   timer alive). Kira writes per-server `"timeout"` into every config it controls: Studio
   `mcpinstall` add-json payload and `agentmcp` per-run config (`harness.MCPServer` gains a
   timeout), so user sessions lose the 60 s cut too. Keep `MCP_TOOL_TIMEOUT` for Kira launches.
4. Batching unchanged in spirit: tasks of a batch stay `working` until the batch's last approval;
   then all complete together. Sync-fallback calls wait on `WaitIdle` exactly as P520 §2.4 says.
5. `tasks/cancel` and Claude's TaskStop map to the job cancel (same seam P520 gives the op widget).

## 5. What changes in P520.md

- §1: replace "Real push … exists only as channels" and the Tasks bullet ("go-sdk has no server
  support … still blocks the turn": wrong for SEP-2663 in 2.1.318 TUI). State Tasks facts and the
  TUI-only gate. Drop the "held long-poll" consequence. Keep the channels bullet as rejected
  alternative; drop sub-point 5's implication that custom methods are impossible (go-sdk v1.8.0 has
  `AddReceivingCustomMethod`; outbound custom notifications remain unsupported).
- §2.1: `Pending`/`mj_` handle becomes a SEP-2663 task (`taskId` = handle). Remove
  `AddAwaitTool`, `await_results`, `waitSeconds`, delivered-once and handle-less await rules.
  Registry keeps: owner bounds, TTL, eviction, Drain/Close, op-log deferral, panic/nil
  normalisation. Add: capability check per request, flat task JSON, `tasks/*` handlers, sync
  fallback path, progress in fallback. Outputs: no `outputSchema` rule may relax (a task result is
  not structured content), re-check with spike.
- §2.2 owner table: `dbmcp` handles stay unguessable capabilities (`tasks/get` by id, no caller
  identity), same as now.
- §2.3: memory MCP is stdio; Tasks over stdio unverified (open item 4). Fallback covers it.
- §2.6: drop all `await_results` prompt entries and the "call await_results" instruction. Claude's
  own tool_result text already tells the model a notification follows. Tool descriptions may keep
  "waits for the user" wording for the sync fallback.
- §2.4/§2.5/§2.7: unchanged.
- §3 tests: replace await-driven assertions with `tasks/get` polling in flow tests (Go client from
  go-sdk cannot speak Tasks; flow helper posts raw JSON-RPC, or uses `CallCustomMethod` with
  `AddSendingCustomMethod`). `fakeagent`: on task result, poll `tasks/get` until terminal. Add a
  flow case per server for the no-extension path (sync, held). Real `claude` row: TUI-only
  feature; headless real tests exercise the fallback. Add a probe-style test only if the real-claude
  harness can drive a TUI.
- New §: `mcpinstall` and `agentmcp` config write per-server `timeout`.
- §4 docs / Known open items: replace "No unsolicited push…" with "Async delivery needs a client
  with MCP Tasks (Claude TUI); `claude -p` and OpenCode get synchronous calls."
- §9 decision 1: reopen; recommend Tasks.

## 6. Open questions for the user

1. Accept Tasks plus synchronous fallback, dropping `await_results`? Headless Kira runs then stay
   synchronous (agent waits on approvals, as today, under the 35 min `MCP_TOOL_TIMEOUT`).
2. Headless runs: is blocking acceptable, or should a later phase move `RunHeadless` to
   `--input-format stream-json` with `CLAUDE_AUTO_BACKGROUND_TASKS=1` (verified to deliver
   backgrounded results there) so headless agents keep working during approvals?
3. Spike first (own commit, before the plan rewrite): go-sdk v1.8.0 receiving middleware returning
   a Kira `Result` with flat task JSON, and `tasks/get` via `AddReceivingCustomMethod`, against
   real `claude` 2.1.318 TUI. Or wait for go-sdk PRs #755/#1335 to ship.
4. Memory MCP (stdio): include in Tasks or leave synchronous?
5. Inline window: keep 1 s, or task every slow-capable call at once (simpler, more notifications)?

## 7. Not verified

- Real accounts' GrowthBook values for `tengu_glowing_lark` (Tasks), `tengu_mcp_auto_background`;
  probes ran with flags at local defaults (no network flags). Could differ per account or change
  remotely.
- go-sdk spike (open item 3): not built.
- Tasks over stdio; OpenCode behaviour; logging notifications reaching model context.
- Concurrency: whether Claude runs several MCP calls of one assistant message in parallel (matters
  for the sync-fallback batch).
- Exact per-message background task cap value; `tasks/cancel` on TaskStop (not exercised).
- Upstream go-sdk issue/PR contents (403 on repo API; only search titles seen).
