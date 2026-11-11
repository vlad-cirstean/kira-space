# v0.3.8 code review findings (round 1)

Base: `858bdb337` (v0.3.7 merge). Scope: `git diff 858bdb337..HEAD` (P604, P605, P606). Reviewer: Opus.
No tests run (user instruction for this chapter).

5 findings, all Low. None blocks the chapter.

## Dimension summary

- Architecture, structure, security: no findings. ClickHouse read-only enforcement still holds. Per-request
  `readonly=2` is skipped only where the profile itself already enforces read-only (`readonly=1`/`2`).
  Every lane opens through `Connect`, so every lane is probed (`adapterhost/readlane.go` `openLane`).
  The `accessOf` hint repeats only what `access.masked` in the same payload already says. It leaks nothing.
  `browse_table` is registered unconditionally (`dbmcp/server.go:385`). Redis nil handling has no regression:
  `runScan` and `planBounded` callers never relied on `goredis.Nil` as an error.
- Functional correctness: R1, R2 (MySQL schema 2.0 parsers, Go and TS alike, so parity holds).
- Performance and resources: R3, R4.
- Tests: R5. The dbmcpflow and UI spec changes do not weaken coverage. The masking scope narrows table
  listing only. The P606 scoping is justified by a reproduced strict-mode race.

## Findings

### R1. v2 full-scan issue fires on a scan of a materialized derived table or CTE

- File: `apps/kira-space/internal/queryplan/mysql.go:248`, `apps/kira-space/frontend/src/studio/views/console/planParsers/mysql.ts:198`
- Severity: Low (false-positive warning; gates read `EstimatedRowsRead`, not issues)
- What: The rule flags any `access_type: "table"` node whose `table_name` does not start with `<`. Schema 1
  names a derived table `<derived2>`, so the old rule skipped it. Schema 2.0 likely names the scan by alias:
  "Table scan on dt" over a `Materialize` input, like the fixture's "Table scan on <temporary>" over
  "Aggregate using temporary table". The alias does not start with `<`, so a derived table or CTE scan
  reads as "Full table scan on “dt”. No index was used." A base-table scan is always a leaf; this scan has
  `inputs`.
- Fix: Raise `full-scan` only on a leaf (`len(n.Inputs) == 0` / `inputs.length === 0`). Keep the `<` check.
  Add a `mysql-v2-derived` parity fixture captured from MySQL 9 (`SELECT * FROM (SELECT cat, count(*) FROM b
  GROUP BY cat) dt`) to confirm the shape.

### R2. v2 plan loses its native cost whenever the top operator carries none

- File: `apps/kira-space/internal/queryplan/mysql.go:282`, `apps/kira-space/frontend/src/studio/views/console/planParsers/mysql.ts:263`
- Severity: Low (display regression against MySQL 8.x)
- What: Root cost and `nativeCost` come only from `query_plan.estimated_total_cost`. MySQL 9 omits it on
  `Sort` and `Aggregate using temporary table` nodes. Any ORDER BY or temp-table GROUP BY query therefore
  shows no cost. `mysql-v2-join.expected.json` has no `nativeCost`. Schema 1 always had
  `query_block.cost_info.query_cost`, so `ExplainResultView`'s cost label disappears on 9.x for common queries.
- Fix: Fall back to the topmost descendant that carries `estimated_total_cost` (first hit, breadth-first),
  in both languages. Regenerate `mysql-v2-join.expected.json`.

### R3. readonly=1 profile: capped console read keeps running server-side

- File: `apps/kira-space/internal/adapters/clickhouse/console.go:102-111`, `apps/kira-space/internal/adapters/clickhouse/query.go:69`
- Severity: Low (resource use; only `readonly=1` profiles)
- What: For a `readonly=1` profile, `buildURL` drops `cancel_http_readonly_queries_on_client_close` and the
  engine timeout. At the row cap, `streamRows` closes the body and the op ends; no KILL QUERY follows. The
  comment says a hung-up read "stops on its own". That holds only when the server writes output often. A
  sparse-output scan (selective WHERE over a large table) or a late-emitting aggregate runs to completion
  with no `max_execution_time` to stop it.
- Fix: When `!h.settingsAllowed()` and `runRowReturning` stops at the cap, send the existing side-request
  `KILL QUERY WHERE query_id = {qid:String}` (as `Cancel` does) and ignore its result. Correct the comment.

### R4. Catalog decode parses every row three times

- File: `apps/kira-space/internal/adapters/clickhouse/query.go:274`
- Severity: Low
- What: `decodeCatalogRows` unmarshals into `[]map[string]json.RawMessage`, re-marshals, and unmarshals again
  into `[]T`. That costs one map per row plus two full extra passes. `listSchemaColumns` on a database with
  tens of thousands of columns pays it on every schema load. Only three columns need it (`total_rows`,
  `position`, `count()`).
- Fix: Cast the 64-bit columns to strings in the catalog SQL (`toString(position)`, `toString(total_rows)`
  keeping NULL, `toString(count())`), as the Connect probe already does for `readonly`. Then restore the
  direct `Data []T` decode. Alternative: one `json.Decoder` with `UseNumber()` into a string-like type that
  accepts both forms.

### R5. Two readonly=1 matrix scenarios assert less than their names claim

- File: `apps/kira-space/internal/adapters/clickhouse/authmatrix_test.go:199`, `:177`
- Severity: Low (test quality)
- What: "UInt64 max decodes exactly" inserts 18446744073709551615 but checks only `Describe` succeeds and
  `Count == 1`. It never reads the value. A precision loss in the read or catalog path would pass. "statement
  timeout degrades to the app timer" runs only fast statements with a 5 s timeout and asserts success. It never
  shows a slow statement being stopped by the app backstop on a `readonly=1` profile.
- Fix: Read the `p604_u64` first page and assert the cell text `18446744073709551615`. In the timeout case,
  add a slow read (`SELECT sum(cityHash64(number)) FROM numbers(1000000000000)`) with a short
  `StatementTimeout` and assert the typed timeout arrives within timeout plus grace.
