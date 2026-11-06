# v0.2.36 code review, round 1

Base: `f0cd476d9` (v0.2.35 squash on main). Tip reviewed: `aa8a899b5`. Scope: `git diff f0cd476d9..HEAD`, minus generated files, snapshots, lockfiles and plan docs.

Covered: DB MCP gate chain (tighten, table allow list, column review, exemption, EXPLAIN limit, pre-limits, approval, result review, throttle, statement timeout), `browse.go`, `internal/maskreview`, `ReadRequest.Conditions` and `BuildConditionsSQL`, per-dialect table extraction, migrations 0060-0064, `ConnectionMcp*.vue`, approvals UI, i18n, contracts.

Nothing real found in: `BuildConditionsSQL` and the four adapter read paths (values always bound, identifiers from the catalog), browse literal preview quoting, exemption store, held-result cap accounting under `mu`, throttle `admit` and reentrancy, statement timeout backstop, migrations, connfiles validation, mongo and kibana extraction, Vue rules (all `<script setup lang="ts">`, no scoped styles, no hard-coded copy).

## Findings

### 1. High: ClickHouse `in(x, table)` function form bypasses the table allow list

- Where: `apps/kira-studio/internal/adapters/clickhouse/tablerefs.go:175-193`.
- Defect: the walker refuses `x IN secret` only as a `BinaryOperation`. The function forms `in`, `notIn`, `globalIn`, `globalNotIn`, `nullIn`, `notNullIn` (and the `...NullIn` variants) parse as `FunctionExpr`. Their names land in `Funcs`, the probe sees a System function, and the call passes. ClickHouse resolves a bare identifier as the second argument of these functions as a table, same as the operator.
- Failure: list holds `default.listed` only. `SELECT count() FROM listed WHERE in(id, secret)` passes the gate (checked: refs `[default.listed]`, refusal empty). The agent gets a membership oracle over `secret`, one guess per call or a batch per `IN` tuple.
- Also check: `id IN (secret)` (parenthesised identifier) passes the walker too. Confirm against a real server whether ClickHouse reads it as a table; refuse it if so.
- Fix: in the `FunctionExpr` case, for any name in the `in` family, refuse when the second argument is an `Ident`, `NestedIdentifier` or `Path`. Add both forms to `tablerefs_parse_test.go`.

### 2. Medium: column review is a deny list, so a column added between checks reaches the agent

- Where: `apps/kira-studio/internal/maskreview/service.go:151-159` (`fresh`, 30 s `MaxAge`), `:282` (`Connected` checks with `opts{}`), `apps/kira-studio/internal/dbmcp/render.go:959-974` (`keptColumns`).
- Defect: render drops only names in `Withheld.Names`, the pending columns of the last check. A column the last check never saw is not in that set and passes. Two windows:
  - Any column added outside MCP (console, migration, other client) is returned for up to 30 s after the last check. `MarkStale` covers only MCP write/DDL.
  - `Connected` runs its check on the persisted SQLite `metadata_cache` (no refresh). That listing can be days old, yet the entry is stored as fresh for 30 s. Columns added since the cache was filled are not withheld right after connect.
- Failure: user turns on review, reviews all columns, quits. Overnight a migration adds `users.ssn`. Next day the app connects; within 30 s the agent runs `SELECT * FROM users` and gets `ssn` unmasked. SPEC P534 ask: a new column not covered is not returned.
- Fix: make render an allow list. Keep a result column only when its lowercased name is a known live column of the last check (or covered by a rule); an unknown name forces an inline `For` refresh with `columns: true`, and is withheld if still unknown. Run the `Connected` check with `columns: true`.

### 3. Medium: held-result refusals tell the agent to retry a statement that already ran

- Where: `apps/kira-studio/internal/dbmcp/tools.go:425-432` (`heldRefusal`, cases `ApprovalHeldFull` and `ApprovalQueueFull`).
- Defect: the result-review step runs after execution. `ApprovalHeldFull` returns "too many results are waiting for the user's review; retry after the user answers them" with no word that the statement ran. `ApprovalQueueFull` says "query ran; ... retry after the user answers them". Neither says changes stay, unlike the `ApprovalDenied` text.
- Failure: write mode with review on; the held total is full. `INSERT INTO orders ...` runs and commits, the agent reads "retry", reruns it, and inserts the row twice.
- Fix: both texts say the statement ran, its changes stay for non-read classes, and not to rerun it; reuse the `rv.class != ClassRead` suffix from the `ApprovalDenied` case.

### 4. Low: run_query does not recheck the table allow list after an approval wait

- Where: `apps/kira-studio/internal/dbmcp/tools.go:633-645` (`recheckAfterApproval`), against `browse.go:451-457`.
- Defect: `recheckAfterApproval` re-resolves exposure and mode only. `browseTable` also rechecks the allow list after the wait, with the comment "the allow list may have changed". run_query and the exemption path skip it. The two tools disagree on the same TOCTOU.
- Failure: a prompt sits for minutes; meanwhile the user removes `secret` from the list. A later Approve click (or a second window) runs the statement against the now-unlisted table.
- Fix: in `recheckAfterApproval`, rerun `checkStatementTables` on the re-resolved summary (no catalog probe when the list is unchanged), so both tools share one recheck.

### 5. Low: postgres probe treats any user overload of a built-in name as built in

- Where: `apps/kira-studio/internal/adapters/postgres/tablerefs.go:395-421`.
- Defect: an unqualified call passes when any `pg_catalog` function has that name. Postgres overload resolution can still pick a user function with the same name and different argument types in a search_path schema.
- Failure: an extension or earlier DDL left `public.format(int)` that reads an unlisted table. `SELECT format(1)` passes the gate and runs it.
- Fix: refuse when any function of that name exists outside `pg_catalog`: query `pg_proc` joined to `pg_namespace` for `nspname <> 'pg_catalog' AND proname = ANY($1)`, instead of the built-in set difference.

### 6. Low: unticking "All tables" gives an empty or a full list depending on cache

- Where: `apps/kira-studio/frontend/src/project/connection/ConnectionMcpTables.vue:39-43` and `:67-73` (`setAllTables`).
- Defect: `tablesQuery` is enabled only while `!allTables`, so on the first untick `data` is undefined and `setAllTables(false)` emits `tables: []`. After a later re-tick and untick, cached data exists and it emits every live table. Same click, two results.
- Failure: user unticks "All tables" once: nothing is allowed. Ticks and unticks again: everything is listed explicitly. Neither is stated in the UI.
- Fix: pick one meaning. Emitting `[]` always is the safe one; drop the `tablesQuery.data` read so the code says so.

### 7. Low: column review truncation text names the wrong cap

- Where: `apps/kira-studio/frontend/src/project/connection/ConnectionMaskReview.vue:39` and `:296-303`, against `apps/kira-studio/internal/maskreview/service.go:24-25`.
- Defect: the note says "Showing the first 100000 columns." The server stops at 50000 columns or 50 schemas, whichever comes first, and then the check is incomplete.
- Failure: a 60000-column connection shows "first 100000 columns". The user cannot tell the list is cut short, nor why MCP refuses every call (incomplete check).
- Fix: drop the client constant; have `Listing` carry the cap that tripped (columns or schemas, with its number) and show that.

### 8. Low: a held result costs twice its counted bytes

- Where: `apps/kira-studio/internal/dbmcp/tools.go:370-395` (`reviewResponse`) and `:439-447` (`resultText`).
- Defect: `resultText` copies the text into a new string, while `res` keeps the original for delivery. The broker caps count `held.Bytes` once, so 8 MiB per result and 64 MiB total are really about 16 MiB and 128 MiB.
- Fix: when `res.Content` holds one `TextContent`, return its `Text` without copying (Go strings share backing memory); else keep the builder.

### 9. Low: dead and duplicated code in the new gate code

- `apps/kira-studio/internal/adapters/clickhouse/tablerefs.go:209-229`: `names` is built and then discarded with `_ = names`. Remove it.
- `apps/kira-studio/internal/dbmcp/browse.go:345-348` (`previewOps`) duplicates `adapters.conditionOperators` (`adapters/conditions.go:11-14`). Export one map and use it in both places, so preview and the bound statement cannot drift.
- `apps/kira-studio/internal/dbmcp/browse.go:156-161` (`browseDepth`) is `containerDepth(kind)+1` (`tables_gate.go:181-186`). Define one in terms of the other.
