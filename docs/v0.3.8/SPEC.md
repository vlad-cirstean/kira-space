# v0.3.8

Fixes after the v0.3.7 release failed. Chapter stays open: more phases may be added. Branch `chapter/v0.3.8`, tag `v0.3.8`. First `P` is P604 (highest prior: P603). Batch size: 3 phases at once; grouping in `plans/batches.md`. Docker works in the sandbox: start `dockerd` per `docs/DEV_ENVIRONMENT.md`, then run real-container tests for real, never reason from SQL alone.

| Phase | Title | Status |
|---|---|---|
| P604 | Release container failures: ClickHouse `readonly=1` connect and MySQL 9.7 explain row estimate | Todo |
| P605 | Real `claude` test `TestSmartScriptRun`: user MCP server leaks into the hermetic list | Todo |
| P606 | UI spec `dbmcp-script-long.spec.ts`: ambiguous `run-stop` locator race | Todo |
| P607 | Proof explanation text: larger than 13px | Todo |
| P608 | Real-container sweep: run every container suite P590-P603 added and fix failures | Todo |

Phases run in table order. Batches of 3 consecutive phases run as streams (P608 runs alone, last).

## P604 Release container failures: ClickHouse `readonly=1` connect and MySQL 9.7 explain row estimate

Two independent failures from the v0.3.7 release, one phase, one plan.

- Ask (user): fix the failing v0.3.7 release jobs `test-matrix (Linux)` and `db-compat (Linux, max)`; run both here to be sure they are fixed.
- Context, ClickHouse: `TestClickHouse_AuthMatrix/kira_ro1_(profile_readonly=1),_database=kira_test` fails with `Connect: want success, got DB::Exception: Cannot modify 'output_format_json_quote_64bit_integers' setting in readonly mode. (READONLY)` (ClickHouse 26.3.58.5). The adapter sets that setting on connect, which a `readonly=1` user cannot change. P602 added the matrix case; it exposed a real adapter bug. Lane reads send `readonly=2` (H1 in P602). Adapter: `internal/adapters/clickhouse`.
- Context, MySQL: `TestMySQL/explain:_the_composed_EXPLAIN_runs_on_the_lane_and_parses` fails with `mysql plan has no row estimate: full <nil>, point <nil>` on `mysql:9.7.2` (MariaDB 12.3.3 passes in the same run). Adapter or P602 `ExplainPlan` scenario: find which is wrong for 9.7. Adapter: `internal/adapters/mysqlfamily`; also check the min image still passes.
- Acceptance: a `readonly=1` user connects and reads; settings the adapter needs are sent in a way that user may use, or the adapter degrades without them; every ClickHouse matrix row passes against the real container. The explain scenario passes on the min and max MySQL and MariaDB images; if the parser was wrong for 9.7, the MCP explain threshold still works on 9.7.
- Scope: every other failure in those suites is in this phase too. Run all of `scripts/test-matrix.sh` and `scripts/db-compat.sh --min` and `--max` end to end, fix every failing test at its root (adapter or test, never weaken), rerun to green.
- Tests: `go test ./internal/adapters/clickhouse/...` against a real ClickHouse container plus `internal/flows/dbmcpflow` ClickHouse rows; `scripts/test-matrix.sh`, and `scripts/db-compat.sh --min` and `--max` for mysqlfamily. All run here and passing.
- Result:

## P605 Real `claude` test `TestSmartScriptRun`: user MCP server leaks into the hermetic list

- Ask (user): fix the failing real `claude` test.
- Context: `bun run test:claude:db` (nested module `apps/kira-space/tests/claude/`): `TestSmartScriptRun` fails because a user-config MCP server appears in the hermetic MCP list. Failing since P591 notes, untouched. Verify auth with the probe in `docs/DEV_ENVIRONMENT.md` "Real `claude` tests" first. Workflow agent step launch is in `internal/harness/claude/headless.go` and `internal/ade/runs.go`.
- Acceptance: the test passes against the real `claude`; if the leak is a product bug (hermetic or isolated runs load user servers), fix the product, else the test.
- Tests: the matching `test:claude*` row, run here and passing.
- Result:

## P606 UI spec `dbmcp-script-long.spec.ts`: ambiguous `run-stop` locator race

- Ask (user): fix the open item.
- Context: `tests/ui/studio/dbmcp-script-long.spec.ts:58` locates `run-stop` without scoping, so it can match a second element. Scope it to `script-run-view`.
- Acceptance: the locator is unambiguous; the spec passes repeatedly.
- Tests: the spec, run here several times with `--repeat-each`.
- Result:

## P607 Proof explanation text: larger than 13px

- Ask (user, P591): proof explanation should be bigger. P591 only reached 13px because a larger size needs a new type token and a change to the font-scale guard.
- Context: `ProofView` step description and summary use `text-kira-lg`. Planner finds the type-token scale, the font-scale guard (lint) and the visual baselines it touches.
- Acceptance: the step description and summary render visibly larger than before through a proper token; the guard allows it; the six affected baselines re-recorded on Linux.
- Tests: UI spec plus visual baselines (Linux, `fc-match sans-serif` = Inter).
- Result:

## P608 Real-container sweep: run every container suite P590-P603 added and fix failures

- Ask (user): run the real-container tests here to be sure everything is fixed.
- Context: v0.3.7 landed with container tests that never ran in the sandbox (P590 `Ping` suites, P593 Postgres step, P602 `TestDbMcpEngine*`, adapter scenarios, complete suites, `TestKubeMcp`). Run `bun run test:go:containers`, `bun run test:flows:complete` and `scripts/test-matrix.sh --mirror` / `scripts/db-compat.sh --min --mirror` / `--max --mirror` here (mirror pulls per `docs/DEV_ENVIRONMENT.md`; Docker Hub blob downloads may be blocked). Fix every failure at its root, adapter or test, never weaken a test.
- Acceptance: every one of those runs passes here; anything that cannot run here (image pull blocked) is listed with the blocking host.
- Tests: the commands above.
- Result:
