# v0.3.8

Fixes after the v0.3.7 release failed. Chapter stays open: more phases may be added. Branch `chapter/v0.3.8`, tag `v0.3.8`. First `P` is P604 (highest prior: P603). Batch size: 3 phases at once; grouping in `plans/batches.md`. Docker works in the sandbox: start `dockerd` per `docs/DEV_ENVIRONMENT.md`, then run real-container tests for real, never reason from SQL alone.

| Phase | Title | Status |
|---|---|---|
| P604 | Release container failures: ClickHouse `readonly=1` connect and MySQL 9.7 explain row estimate | Done |
| P605 | Real `claude` test `TestSmartScriptRun`: user MCP server leaks into the hermetic list | Todo |
| P606 | UI spec `dbmcp-script-long.spec.ts`: ambiguous `run-stop` locator race | Done |

Phases run in table order. Batches of 3 consecutive phases run as streams.

## P604 Release container failures: ClickHouse `readonly=1` connect and MySQL 9.7 explain row estimate

Two independent failures from the v0.3.7 release, one phase, one plan.

- Ask (user): fix the failing v0.3.7 release jobs `test-matrix (Linux)` and `db-compat (Linux, max)`; run both here to be sure they are fixed.
- Context, ClickHouse: `TestClickHouse_AuthMatrix/kira_ro1_(profile_readonly=1),_database=kira_test` fails with `Connect: want success, got DB::Exception: Cannot modify 'output_format_json_quote_64bit_integers' setting in readonly mode. (READONLY)` (ClickHouse 26.3.58.5). The adapter sets that setting on connect, which a `readonly=1` user cannot change. P602 added the matrix case; it exposed a real adapter bug. Lane reads send `readonly=2` (H1 in P602). Adapter: `internal/adapters/clickhouse`.
- Context, MySQL: `TestMySQL/explain:_the_composed_EXPLAIN_runs_on_the_lane_and_parses` fails with `mysql plan has no row estimate: full <nil>, point <nil>` on `mysql:9.7.2` (MariaDB 12.3.3 passes in the same run). Adapter or P602 `ExplainPlan` scenario: find which is wrong for 9.7. Adapter: `internal/adapters/mysqlfamily`; also check the min image still passes.
- Acceptance: a `readonly=1` user connects and reads; settings the adapter needs are sent in a way that user may use, or the adapter degrades without them; every ClickHouse matrix row passes against the real container. The explain scenario passes on the min and max MySQL and MariaDB images; if the parser was wrong for 9.7, the MCP explain threshold still works on 9.7.
- Scope: every other failure in those suites is in this phase too. Run all of `scripts/test-matrix.sh` and `scripts/db-compat.sh --min` and `--max` end to end, fix every failing test at its root (adapter or test, never weaken), rerun to green.
- Tests: `go test ./internal/adapters/clickhouse/...` against a real ClickHouse container plus `internal/flows/dbmcpflow` ClickHouse rows; `scripts/test-matrix.sh`, and `scripts/db-compat.sh --min` and `--max` for mysqlfamily. All run here and passing.
- Result: ClickHouse: Connect reads `getSetting('readonly')`; `buildURL` sends settings and per-request `readonly=2` only as the profile allows (0: all; 2: settings, no `readonly`; 1: nothing). `output_format_json_quote_64bit_integers` and `output_format_json_validate_utf8` dropped; `RunCatalogQuery` re-quotes bare numbers. MySQL: both parsers (Go `queryplan/mysql.go`, TS `planParsers/mysql.ts`) read schema 2.0 (`query_plan`), pinned by three parity fixtures captured from a real `mysql:9.7.2`; the adapter and `ExplainPlan` scenario were right, the parser was not. Extra failures found and fixed at root: Redis console turned a nil reply (missing key, timed-out `XREAD BLOCK`) into `E_QUERY: redis: nil` (adapter fix); `TestDbMcpEngineTimeout` (Postgres CTAS of a void column, bool rendered `t`, MySQL `select sleep(2)` never errors under `max_execution_time`) and `TestDbMcpEngineMasking` (column review ran over every fixture table, name-based drop hid `id`; scoped to the table under test) were test defects, all failing before this phase. Ran here, all passed: `KIRA_TEST_MATRIX=1 go test ./internal/adapters/clickhouse/... -count=1` (rows kira_ro1, kira_ro2 included); `go test ./internal/queryplan/...`; `bun test apps/kira-space/tests/unit/studio/explain-plan.spec.ts`; `KIRA_FLOW_COMPLETE=1 KIRA_FLOW_DOCKER=require go test -p 1 ./apps/kira-space/internal/flows/dbmcpflow/ -run TestDbMcpEngine`; `sh scripts/test-matrix.sh --no-pull` (images were already pulled with `--mirror`; its own pull step fails on disk at `kibana`, unused by the matrix); `sh scripts/db-compat.sh --min --mirror`; `sh scripts/db-compat.sh --max --mirror` (mysql 9.7.2 `explain:` included); `KIRA_TEST_MATRIX=1 sh scripts/db-compat.sh --only clickhouse --min --mirror` and `--max --mirror`; `go test ./internal/adapters/mysqlfamily/... ./internal/adapters/redis/...` on pinned images. Not run: none.

## P605 Real `claude` test `TestSmartScriptRun`: user MCP server leaks into the hermetic list

- Ask (user): fix the failing real `claude` test.
- Context: `bun run test:claude:db` (nested module `apps/kira-space/tests/claude/`): `TestSmartScriptRun` fails because a user-config MCP server appears in the hermetic MCP list. Failing since P591 notes, untouched. Verify auth with the probe in `docs/DEV_ENVIRONMENT.md` "Real `claude` tests" first. Workflow agent step launch is in `internal/harness/claude/headless.go` and `internal/ade/runs.go`.
- Acceptance: the test passes against the real `claude`; if the leak is a product bug (hermetic or isolated runs load user servers), fix the product, else the test.
- Scope: every other failure in the real `claude` tests is in this phase too. Run all `bun run test:claude*` rows, fix each failure at its root (product or test, never weaken), rerun to green.
- Tests: all `test:claude*` rows, run here and passing.
- Result:

## P606 UI spec `dbmcp-script-long.spec.ts`: ambiguous `run-stop` locator race

- Ask (user): fix the open item.
- Context: `tests/ui/studio/dbmcp-script-long.spec.ts:58` locates `run-stop` without scoping, so it can match a second element. Scope it to `script-run-view`.
- Acceptance: the locator is unambiguous; the spec passes repeatedly.
- Tests: the spec, run here several times with `--repeat-each`.
- Result: Cause: `run-stop` also renders in `RunsSection` `run-row` for live runs, so the unscoped locator matched 2 elements (strict-mode race). Fix: tests 2 and 3 scope `run-log`, `run-stop`, `script-run-result`, `run-outcome-exit` to `script-run-view`; no assertion weakened. Before: `--repeat-each=20 --workers=4` failed 12/60. After `bun run build:test`: 60/60 pass. Siblings `automations-mcp`, `ui-leftovers-docker-automations`, `dbmcp-approval-script` specs: 19/19 pass.
