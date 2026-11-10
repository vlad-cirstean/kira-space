# v0.3.7

User-requested features after v0.3.6. Chapter stays open: more phases get added later. Branch `chapter/v0.3.7`, tag `v0.3.7`. First `P` is P590 (highest prior: P589). Batch size: 3 phases at once (user request); grouping in `plans/batches.md`.

| Phase | Title | Status |
|---|---|---|
| P590 | Kira Studio approval prompt: skip when the database connection cannot be established | Done |
| P591 | Proof tool: behavior not code, larger explanation, terse guidance | Done |
| P592 | Jira: faster loading | Done |
| P593 | Database MCP: customizable explain threshold | Done |
| P594 | Open GitHub links inside the app: fix | Done |
| P595 | `api_request` rules: default action ask or deny | Done |
| P596 | Masking and the Privacy tab: make it understandable | Todo |
| P597 | Proof: screenshot button saves all to a Desktop folder, location customizable | Todo |
| P598 | Task work start: require a repo choice before an agent opens | Todo |
| P599 | Review changes button: also for uncommitted changes | Todo |
| P600 | Task Details: group the top links by category, more compact and nicer | Todo |
| P601 | Task row: agent indicator at the start, manual status as small text with dropdown | Todo |

Phases run in table order. Batches of 3 consecutive phases run as streams.

## P590 Kira Studio approval prompt: skip when the database connection cannot be established

- Ask (user): the Kira Studio prompt should not show if the database connection cannot be established.
- Context: `internal/dbmcp` approval flow (`ApprovalBroker`, `run_query` path). Planner finds where the prompt is raised relative to connecting, and whether the approval can wait for a successful connect.
- Acceptance: a request against a connection that cannot connect returns the connection error without raising an approval item.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.
- Result: Done. A never-connected failing connection already got no prompt (`connectForQuery` gate). Prompts still showed after a server died while the state read `connected`, or when the read lane could not open. New `adapters.Pinger` (postgres, mysql, mariadb, sqlite, clickhouse, mongodb, redis, kibana), `Router.Probe` and `dbmcp.ensureReachable`, called before each `db.query` and `db.exemption` approval (run_query, browse_table, explain), return `connection "<name>" is not reachable: ...` with no item. `connections.Service.MarkLost` turns a connection whose main adapter fails the probe to `error` (`backend.studio.connection.lost.error`); the next call reconnects. Out of scope: `kube.read`, `script.run`, `api.request`, `db.response`. Plan: `plans/P590.md`. Tests: `dbmcpflow` `TestDbMcpUnreachableRaisesNoApproval` (write, lane read, explain, browse masked, exempted, never connected, recovery; contract `dbmcp-unreachable`), UI spec `studio/dbmcp-unreachable.spec.ts`, `adapterhost` `TestRouter_Probe`, conformance `PingAnswers` plus sqlite `TestSqlite_PingFileRemoved`. Deviations from the plan: unconnected `Ping` returns `E_CONNECT` (shared `RequireConnected`), not `E_ENGINE_DOWN`; a lane failure with not-found is also confirmed on the main adapter (a vanished SQLite file); explain approval moved into `awaitExplainApproval` for the gocognit limit. Real-container adapter suites (postgres, mysql, mariadb, clickhouse, mongodb, redis, kibana `Ping`) not run: no Docker in the sandbox.

## P591 Proof tool: behavior not code, larger explanation, terse guidance

- Ask (user):
  - The proof tool states that a proof is about functionality and behavior, not code.
  - The proof window gives the explanation more space by default and a bigger font; it is the most important part of a proof.
  - Tool text guides the agent to keep the explanation terse but clear.
- Context: `internal/dbmcp/proof_tools.go` (`addProofTools`), proof window UI. Prompt budget rules in `internal/llmprompts`.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.
- Result: `proof_save` text now says a proof shows functionality and behavior, not code, and asks for one terse, clear line per step (soft 120, hard 200 characters; limits unchanged) and 1-3 sentences for the summary; `budget.tsv` ratcheted to 1032/115/192. Step description wraps at `text-kira-lg font-medium` instead of truncating in a fixed-height row; the proof summary, never rendered before, shows above the tabs (`proof-summary`, `max-h-32`, scrolls, in screenshots; new key `studio.proof.summary.aria`). Tests: flow `TestProofExplanation` (contract `proof-explained.json`), UI spec "the explanation and summary show in full at the larger size" plus empty-summary check (14 passed), visual: six baselines re-recorded and `proof-explanation` added (Inter confirmed, rerun green). Real claude `test:claude:db`: `TestProofMcpThroughClaude` and `TestProofChartThroughClaude` pass; `TestSmartScriptRun` fails (user-config MCP server leaks into hermetic list, unrelated to proof, not touched by this phase).

## P592 Jira: faster loading

- Ask (user): Jira is still very slow. Consider a more efficient approach.
- Context: `apps/kira-space/internal/issuetracker/jira`. Planner measures first (calls per view, payload size, pagination, caching, `acli` process spawn cost) and picks the approach with evidence.
- Result: Every Jira read is one `acli` spawn and no timing exists here, so the plan counted spawns on the critical path and cut them. Issue cache keeps the last good copy 30 min (saved detail too, was 10) and serves it `Stale` while one background read refreshes it; an OK status probe re-checks in the background up to 1 h. New optional `IssueBatch` reads 50 issues per `workitem search`, with per-key fallback and a 10 min backoff. The watcher runs issue reads beside host reads, one batch per pass, and reads a detail only where `updated` moved. The task issue line takes `TaskIssueArgs{taskId, fresh}`, shows the copy at once and refreshes in place once. Spawns on the UI path: opening Details on a task read 1 to 30 min ago went 1 blocking (4 when status was past 5 min) to 0; first open after start 1 per task plus probe and discovery to 0 once the start-up pass ran; a board push past 60 s 1 to 0. Watcher with N linked tasks: N heavy detail spawns per 10 min and per Refresh to ceil(N/50) searches plus one detail per changed issue (asserted by `TestTrackedIssueBatch`). Search flags and JSON shape are unverified against a live acli (Known open item); a refused search falls back to per-key reads. A moved `updated` forces a fresh detail read (deviation from plan: avoids a stale saved copy hiding the change). Tests: `TestIssueCacheRules`, `TestIssuesBatch`, `TestStatusStaleWhileRevalidate`, flows `TestTaskIssueFastLoading` and `TestTrackedIssueBatch`, UI `ade-v2-issue-tracker.spec.ts`.

## P593 Database MCP: customizable explain threshold

- Ask (user): the explain threshold cannot be customized.
- Context: `ExplainThreshold` in `bridge/dbmcp.go` `serverConfig`; `ConnectionMcpTab.vue` (`auto_explain`). Planner decides the scope (global, per connection, or both) and where the setting lives.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.
- Result: optional per-connection override `explain_threshold_rows` (file key, column and migration 0095, 1,000 to 1,000,000,000, absent = `advanced.expensiveQueryRows`), shown in the connection Advanced tab for postgres, mysql, mariadb and clickhouse with a reset button and engine note. It drives console explain, auto-explain, the verdict text and every MCP path (`dbmcp.Server.thresholdFor`, read per call, live without restart); `list_connections` reports the effective number. MCP tab says where the number comes from. Settings, Advanced text points to the override; the stale "Takes effect after restart" note is removed (op-log retention is read live at each prune). Flow `TestDbMcpConnectionExplainThreshold` plus a Postgres complete-suite step (not run: no Docker here); UI spec `connection-explain-threshold`; `settings-advanced` visual baseline re-recorded (Inter confirmed). Non-UI flows all pass.

## P594 Open GitHub links inside the app: fix

- Ask (user): opening GitHub links in the app itself is broken.
- Context: link handling for GitHub URLs (issues, pull requests) in the task panel and elsewhere; P586 moved Details links to the system browser. Planner reproduces first.
- Tests: UI spec; backend half only if one exists.
- Result: Cause: classification (`ParseLink`) is prefix-only but the PR view parser (`github.ParseBranchLink`) allowed only `/pull/<n>` plus one of `files`, `commits`, `checks`. A link ending in `/changes` (GitHub's current Files tab), a trailing slash, `files/a..b` or `commits/<sha>` filed as a change request, then `HostViewService.Open` answered bad request and the click's rejection was swallowed. Fix: parser accepts a trailing slash, the `changes` tab and sub-paths under `files`, `commits`, `changes`; unknown suffixes still miss. New `useOpenChangeRequest` opens the browser when the view refuses or fails, used by the Details title, branch panel View and Main tab PR chip. Tests: flow `TestGitHubLinksInApp` (written first, failed on `/pull/12/changes`) and `tests/ui/github-links-in-app.spec.ts` share contract `github-links-in-app`; parser table extended. Not fixed: GitHub Enterprise Server PR links classify as plain links (`ParseLink` hardcodes `github.com`; needs a signature change across providers), and timeline, archived, notes and PR-body links keep opening the browser by P586 design.

## P595 `api_request` rules: default action ask or deny

- Ask (user): API allow lists block everything else once a rule exists. With the default "ask for everything", the model is allow and deny rules plus a default case that is ask or deny.
- Context: P580 rules (`internal/apiaccess/policy.go`, `dbMcp.apiRules`, `apiApproval`). Today any rule makes unmatched requests refused; replace by an explicit default.
- Acceptance: default is `ask` or `deny`; existing installs keep behavior.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.
- Result: Rules are allow (runs without approval), deny (refuses), and new leaf `dbMcp.apiDefault` (`ask` | `deny`, default `ask`) for requests no rule matches; it applies with zero rules too. Deliberate user-approved change: allow rules now skip approval, so installs with allow rules and approval on stop prompting for allowed hosts. Migration 0096 writes `deny` for installs with stored rules (unmatched stay refused); an unreadable value reads as `deny`. `Policy.Decide` and `CheckHop` replace `Check`; a redirect hop must go to an allow-matched host unless there are no rules and the default is `ask`; `Planned.AllowedByRule` plus `ErrPlanChanged` in `Send` close the plan-to-send race. Prompt text and budget (363 to 453) updated. UI: default select, hints, allow-any warning, dialog effect hint and host-forms line (key `apiRules.hostForms.body`, catalog caps depth at 6). Plan: `plans/P595.md`. Tests: `TestPolicyDecide`, `TestPolicyAllowsAll`, `TestPolicyCheckHop`, `TestMigration0096APIDefault`, flows `TestApiAccessDefault` and revised `TestApiAccessRules` (contract `dbmcp-api-default`), UI specs `dbmcp-api-default` and `dbmcp-api-rules`. Real `claude` run: REALCLAUDE

## P596 Masking and the Privacy tab: make it understandable

- Ask (user): masking is confusing; the Privacy tab is not understandable.
- Context: Privacy tab, `MaskRules`, `MaskReview` in `serverConfig`, column review flow. Planner explains the current model first, then proposes a redesign the user can read.
- Tests: flow test plus UI spec where behavior changes.
- Result:

## P597 Proof: screenshot button saves all to a Desktop folder, location customizable

- Ask (user):
  - Export all screenshots creates a folder by default and saves it on the Desktop. The save location is customizable from the export window.
  - The proof window's screenshot button is the save-all-to-folder action: it replaces the single-screenshot save, with no separate export-all control.
- Context: proof screenshot export (`ProofSaveJpegArgs`, save flow).
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.
- Result:

## P598 Task work start: require a repo choice before an agent opens

- Ask (user): when starting work in a task, first ask the user to at least select a repo. Today, if they forget, the agent opens in a random run folder. Applies from inside a task when starting work in a workflow.
- Context: `AddTaskRepo` (`internal/ade/board_writes.go`), run launch (`internal/ade/runs.go` `queueRun`, `launches.go`), task `AdeTaskRepo`. Planner finds every start path (Run dialog, workflow step start, TUI launch, retry) and what folder an agent gets when the task has no repo.
- Acceptance: a task with no repo cannot start work without choosing at least one repo in a prompt; no start path falls back to a run folder.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.
- Result:

## P599 Review changes button: also for uncommitted changes

- Ask (user): the review changes button shows not only for commits but also for uncommitted changes.
- Context: Review tool (`internal/gitsession/crreview.go`, P581), the review changes button in the task panel and Git sidebar. Planner finds when the button shows today and how a review of the working tree (staged and unstaged) would be modeled.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.
- Result:

## P600 Task Details: group the top links by category, more compact and nicer

- Ask (user): in the task Details, group the top links by category, with issue, change request (CR) and misc types. Make it more space efficient and nicer.
- Context: Details tab top links (`AdeDetailsTab.vue` and children), link kinds on the task (issue links, change request links, other). Planner finds the link model, how misc links are stored and added, and the current layout.
- Acceptance: links appear under issue, CR and misc groups; the block uses less vertical space than today; add and remove still work.
- Tests: UI spec; flow test only if the link model changes.
- Result:

## P601 Task row: agent indicator at the start, manual status as small text with dropdown

- Ask (user):
  - Keep the agent indicators in the task (green circle and the others) but move them from the middle to the very beginning of the task row, replacing the colored square.
  - The manual status the user can set shows as small text in the task itself; clicking it opens a dropdown to change it.
- Context: task row in the board and lists (`AdeTaskRow`/card components under `apps/kira-space/frontend/src/ade/v2/`), the colored square (task color mark), the agent state indicator, manual status control. Planner finds every place the row renders and whether the color is still needed elsewhere.
- Tests: UI spec; flow test only if status writes change. Visual baselines move: plan which.
- Result:
