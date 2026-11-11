# v0.3.9

Fixes after the v0.3.8 release failed. Chapter stays open: more phases may be added, and it is not closed or PR'd until the user says so. Branch `chapter/v0.3.9`, tag `v0.3.9`. First `P` is P607 (highest prior: P606). Batch size: 3 phases at once; grouping in `plans/batches.md`. Docker works in the sandbox: start `dockerd` per `docs/DEV_ENVIRONMENT.md`, then run real-container tests for real, never reason from SQL alone. Tests run for real this chapter: no fix counts as done unless its suite ran and passed here.

| Phase | Title | Status |
|---|---|---|
| P607 | ClickHouse `readonly=1`: `Execute` ignores the app statement timeout | Todo |
| P608 | Release `test-matrix (Linux)`: split into parallel jobs with tight timeouts | Todo |

Phases run in table order. Batches of 3 consecutive phases run as streams.

## P607 ClickHouse `readonly=1`: `Execute` ignores the app statement timeout

- Ask (user): fix the failing v0.3.8 release job `test-matrix (Linux)`; run it here to be sure it is fixed.
- Context: `TestClickHouse_AuthMatrix/kira_ro1_(profile_readonly=1),_database=kira_test/statement_timeout_degrades_to_the_app_timer` fails with `authmatrix_test.go:205: slow Execute = <nil>, want a statement timeout`. The subtest ran 976.8s (query `SELECT sum(cityHash64(number)) FROM numbers(1000000000000)`, 1s `StatementTimeout`) and `Execute` returned nil. On a `readonly=1` profile the server-side `max_execution_time` setting cannot be sent, so the app timer must cancel the statement (context cancel plus `KILL QUERY`). Works on `Read` (v0.3.8 R3), not on `Execute`. The scenario came from v0.3.8 review fix R5 and had never run. Adapter: `internal/adapters/clickhouse`; test: `authmatrix_test.go`. Also check that the cancel path really stops the query on the server, and that the test's own bound is tight (it must fail fast, not after 16 min).
- Acceptance: on `readonly=1`, `Execute` and `Read` stop at the app timeout and report a statement-timeout error; the matrix test fails within seconds when it regresses.
- Scope: run `scripts/test-matrix.sh` end to end here, plus `scripts/db-compat.sh --only clickhouse --min` and `--max` with `KIRA_TEST_MATRIX=1`, and the other scenarios v0.3.8 R3/R4/R5 added that never ran (UInt64 max read-back, `KILL QUERY` at the row cap, `toString` catalog SQL). Fix every other failure found at its root, never weaken a test.
- Tests: the commands above, run here and passing.
- Result:

## P608 Release `test-matrix (Linux)`: split into parallel jobs with tight timeouts

- Ask (user): consider splitting slow release tests into more GitHub Actions jobs.
- Context: `test-matrix (Linux)` runs 9 adapter packages sequentially in one job (about 22 min when a test hangs, 30 min job timeout, `go test -timeout 30m` per package equals the job timeout). `db-compat` is already split min/max at about 5 min each. `scripts/test-matrix.sh` supports `--only <kind>`; check that image pulls are filtered by kind (`pull_images`). Workflow files cannot be pushed from this session: deliver the change as `docs/pending-changes/` patch per `docs/DEV_ENVIRONMENT.md`.
- Acceptance: release `test-matrix` becomes a matrix job over kinds (grouped by weight: postgres, mysqlfamily, clickhouse, the small kinds together), `fail-fast: false`, each job pulls only its images, job timeout about 12 min, per-package `go test -timeout` about 8 min; an aggregate job keeps the name `test-matrix (Linux)` so `publish`'s `needs` stays valid. Same for `pr.yml` only if it has an equivalently slow job. Script changes (`--only` pull filtering, timeouts) are real commits; the workflow edit is the pending patch.
- Tests: `scripts/test-matrix.sh --only <kind> --mirror` for each group run here and passing; a dry check of the workflow YAML (actionlint if available, else YAML parse).
- Result:
