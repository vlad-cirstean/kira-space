# v0.2.36

Branch `chapter/v0.2.36`, based on `chapter/v0.2.35` (v0.2.35 not yet merged to main). First `P` is 528 (highest in v0.2.35 is P527), last is P536.

Source: user request. Nine phases, all on the Studio database MCP (`apps/kira-studio/internal/dbmcp`) and the connection settings. Close duty after the last phase lands: one full suite run, 2 code review rounds (user ask), hand-written close. After the close and push, open the PR to main (CLAUDE.md close rules), subscribe to it, and fix failing checks until it merges. Each phase: Opus plan, Sonnet implementation.

Sequencing: nearly every phase edits the DB MCP server, tool descriptions, the llmprompts registry and `ConnectionMcpTab.vue`, so run them one at a time in table order. Safe to overlap, with a rebase at landing: P531 (throttle, bridge and adapterhost) and P533 (descriptions only) with whichever phase is running.

Limits enforcement model (shared by P529, P530, P531): a limit is enforced by the app or the engine, never by the agent's good will. A call that hits a limit is rejected with a clear reason. The agent may retry with a better query or ask for an exemption (P528). An exemption always raises an approval dialog, even when the connection already allows that mode over MCP without approval.

| Phase | Title | Status |
|---|---|---|
| P528 | DB MCP limit exemptions: agent asks, user always approves | Done |
| P529 | Statement timeout per connection, enforced by the engine | Done |
| P530 | Expensive-query (EXPLAIN) threshold enforced for MCP, with explanation | Done |
| P531 | Ops throttle covers every op from every session, with explanation | Done |
| P532 | Table allow list per connection for DB MCP | Done |
| P533 | Tool descriptions suggest inspecting schemas first | Done |
| P534 | Masking validation: new columns are withheld until masking is reviewed | Pending |
| P535 | Masked-read permission and constrained browse tool as the MCP default | Pending |
| P536 | Review MCP results before delivery to the agent | Pending |

## Rows

### P528 DB MCP limit exemptions: agent asks, user always approves

- Ask: shared mechanism for P529 and P530. When a limit rejects a call, the agent may ask for an exemption in its next call.
- Exemption is a new approval kind on the P520 approvals surface (Go owner plus registered frontend body). It always shows a dialog, even if the connection allows reads over MCP without approval. The dialog says which limit hit, the measured value (elapsed time or estimated rows), the limit, and the statement.
- Rules: an exemption covers that one statement only, never the session. The agent passes an exemption request plus a short reason on `run_query`, never a way to skip the dialog. A denied exemption returns a clear refusal.
- Decide in plan: whether the exemption is a `run_query` field or a separate tool; how the rejected call tells the agent the exemption path exists (rejection text names the field).
- Plan: `plans/P528.md`.
- Result: `run_query` takes `exemption: {id, reason}`; a limit refusal mints a single-use `exm_` id; an exempted call always raises one `db.exemption` approval (also on `allow` modes) that "Approve all" skips. `Limit`/`PreLimit`/`ExecLimit` seams in `dbmcp/limits.go` and `flowharness.WithDbMcpLimits` ready for P529/P530. Five flow tests, a store unit test and a UI spec pass.

### P529 Statement timeout per connection, enforced by the engine

- Ask: DB MCP calls run under a statement timeout, like the P527 read lane sets read-only: engine-enforced, for example Postgres `SET statement_timeout = '5s'`. The value is configurable in the connection settings (MCP tab).
- On timeout the call fails with a clear reason. The agent may then ask for an exemption (P528) to run it with a longer or no timeout.
- Per engine: Postgres, MySQL, MariaDB, ClickHouse and SQLite session or driver-level mechanism where the engine has one. Where none exists (state in plan), enforce by context cancel plus the P518 cancel path and label that enforcement `app`, same labels as P527 `enforcement`.
- Applies to every MCP mode (read, write, ddl). Default value decided in plan.
- Plan: `plans/P529.md`.
- Result: `mcp.statement_timeout_sec` (absent 10, 0 off, 1 to 3600; migration 0060 column). Engine layer: Postgres, MySQL (SELECT), MariaDB, ClickHouse; app backstop timer in `Host.RunOp` through the P518 cancel path (2 s grace when the engine enforces); SQLite, MongoDB, Redis, Kibana read app only. Built-in `statement_timeout` `ExecLimit` in `dbmcp/timeout.go`; an exemption lifts it with no SET issued. `list_connections` carries `statementTimeout`. MCP tab field with who-enforces, on-hit and throttle lines. Real-container calibration passes for Postgres, MySQL, MariaDB, ClickHouse.

### P530 Expensive-query (EXPLAIN) threshold enforced for MCP, with explanation

- Ask: the existing "expensive query rows" setting (`advanced.expensiveQueryRows`, used by `queryplan.FromPages` and the user-side auto-explain) becomes enforceable for MCP the same way as the timeout. Before running, the app runs EXPLAIN. If the estimate passes the threshold, reject the query. The agent improves the query, or asks for an exemption (P528), which always raises a dialog.
- Add a plain explanation next to the setting: it is the estimated row count from the query plan, not the rows returned; above it the app warns on the user side and rejects on the MCP side. Explanation text lives in the i18n catalog and is shown in Settings and in the connection MCP tab.
- Decide in plan: engines without a usable EXPLAIN row estimate (document, key-value, Kafka and similar) skip the check and say so in the MCP tab.
- Plan: `plans/P530.md`.
- Result: built-in `expensive_query` `PreLimit` (`dbmcp/expensive.go`). One EXPLAIN per call (`planForCall`, before limits, read class in any declared mode, `mcp.auto_explain` reused, no migration) feeds the limit, the dialog and the result's `plan`. Estimate at or above `advanced.expensiveQueryRows` refuses with the P528 exemption path; EXPLAIN failure refuses (fail closed). `ApprovalReasonHeavy` and the heavy prompt are gone. `list_connections.expensiveQuery {rowsThreshold, check}`. Settings > Advanced and the MCP tab explain the estimate, who enforces, what a hit does and known limits (stale statistics, LIMIT ignored, SELECT/WITH only, throttle). Flow tests (Postgres complete, SQLite, list_connections), contract `dbmcp-expensive-query` and `connection-mcp-expensive.spec.ts` pass.

### P531 Ops throttle covers every op from every session, with explanation

- Ask: the per-connection throttle (`throttle_per_sec`, 0.01 to 1000) must apply to all ops on that connection from this app: user ops and MCP ops, across all windows and sessions. Audit every path that reaches an adapter (grid, console, tree, exports, scripts, MCP, background loads) and route all through the one limiter. A bypass found is a defect to fix.
- Add an explanation to the setting: what the value means (ops per second admitted to the database for that connection), that ops run one at a time, so a slow or blocking query makes the following ops wait, and what happens at 0 (off). Same text in the connection dialog and the MCP tab where it matters.
- Plan must state the real current semantics first (read the limiter, state whether it is serial and per connection or per session), then the change.
- Plan: `plans/P531.md`.
- Result: one serial, paced gate per connection id (`Host.admit`, exempt-list kinds, live rate change keeps the slot, redis classify gated), app-wide across windows, MCP and the read lane. Explained in the Advanced and MCP tabs. Flow test `dbmcpflow/throttle_test.go` and `tests/ui/connection-throttle.spec.ts` pass.

### P532 Table allow list per connection for DB MCP

- Ask: a table allow list on the connection. A table not on the list cannot be queried over MCP: any MCP statement that touches it is refused, whatever the mode. Global per connection for all MCP permissions (read, write, ddl, and later P535). A select-all checkbox sets or clears the whole list.
- Needs table extraction from each statement. Plan decides the parser approach and fails closed: if a statement cannot be parsed and the list is not "all", refuse it. Plan also covers views, CTEs, subqueries, schema-qualified names, and `list_connections` reporting the allowed tables.
- Default for existing connections decided in plan (all allowed, so nothing changes silently).
- UI: list in `ConnectionMcpTab.vue` with search and the select-all checkbox.
- Plan: `plans/P532.md`.
- Result: file key `mcp.tables` (absent = all, `[]` = none), per-dialect cgo-free table extraction, fail-closed gate as the last step of `resolveVerdict` plus explain_query, list_children, describe_table, describe_schema and list_connections, live catalog probe as throttled op kind `tables`, MCP tab list with enforcement lines. Flow test `dbmcpflow/tables_test.go`, `tests/ui/connection-mcp-tables.spec.ts`, adapter probes and the real `claude` suite pass.

### P533 Tool descriptions suggest inspecting schemas first

- Ask: the DB MCP tool descriptions and the server instructions suggest the agent first inspect the schema of the tables it will query. A suggestion only, nothing enforced.
- Touches the llmprompts registry text and budget, plus the docs listing the prompts.
- Plan: `plans/P533.md`.
- Result: dbmcp.instructions and describe_table, describe_schema, run_query, explain_query descriptions suggest inspecting table schemas before a query; budget lines and contract hash updated; docs bullets added; real `claude` Studio suite passes.

### P534 Masking validation: new columns are withheld until masking is reviewed

- Ask: the user configures masking per connection and is happy with it. A column that disappears is fine. A column that appears and is not covered by the reviewed set is not returned to the agent, and the user sees a notification to update the masking.
- Needs a stored snapshot of the reviewed columns per connection (user confirms it), a diff against the live schema on connect and refresh, and enforcement in the MCP result path (the new column is dropped from results or the query refused; plan decides, the agent is told why).
- Notification uses the existing notification mechanism; a link opens the masking editor.
- Plan: `plans/P534.md`.

### P535 Masked-read permission and constrained browse tool as the MCP default

- Ask: new MCP permission "read masked data", lower sensitivity than read, toggled independently. It does not allow arbitrary queries. It exposes only what the SQL viewer (grid browse) allows: filter (where), order, and removing columns. No transforms, no renames, no joins, no expressions. That guarantees masking applies. Result limit is at most 100 rows.
- This browse tool becomes the default way for the agent to read rows. A real query (`run_query`) is the fallback; tool descriptions and server instructions tell the agent to use the browse tool first and a custom query only when it does not fit. Running a real query needs a distinct tool or an explicit field (decide in plan).
- Browse input is a structured spec (table, columns subset, filters, order, limit), never SQL text, so the app builds the statement and masking cannot be bypassed by aliases or expressions. This closes the name-matching gaps of the current masking for this path. Filters on masked columns: decide in plan (inference risk).
- Depends on P532 (allow list) and P534 (new-column gate).
- UI: new permission toggle and note in `ConnectionMcpTab.vue`.
- Plan: `plans/P535.md`.

### P536 Review MCP results before delivery to the agent

- Ask: the user can inspect DB results before the agent receives them. A toggle per permission, so reads can require review even when the agent may run them freely. The approval prompt gets a button to request result review for that one call, even when the toggle is off.
- Flow: query runs, the result is held, the user sees it in the approvals surface (P520 kind with a result body, grid or table, with masking already applied), then Deliver or Withhold. Withheld returns a refusal to the agent. Long approval timeouts as in P520.
- Held results must not be logged or kept longer than needed; plan covers memory, size caps and the op log.
- Plan: `plans/P536.md`.

## Close

2 code review rounds (user ask). After P536: one full suite run with all failures fixed, review rounds, hand-written close per CLAUDE.md, PR.
