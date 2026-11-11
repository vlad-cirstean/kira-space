# v0.3.9

Fixes after the v0.3.8 release failed. Chapter stays open: more phases may be added, and it is not closed or PR'd until the user says so. Branch `chapter/v0.3.9`, tag `v0.3.9`. First `P` is P607 (highest prior: P606). Batch size: 3 phases at once; grouping in `plans/batches.md`. Docker works in the sandbox: start `dockerd` per `docs/DEV_ENVIRONMENT.md`, then run real-container tests for real, never reason from SQL alone. Tests run for real this chapter: no fix counts as done unless its suite ran and passed here.

| Phase | Title | Status |
|---|---|---|
| P607 | ClickHouse `readonly=1`: `Execute` ignores the app statement timeout | Todo |
| P608 | Release `test-matrix (Linux)`: split into parallel jobs with tight timeouts | Todo |
| P609 | DB MCP: `browse_table` gets `in`, count and paging; descriptions steer reads to it | Todo |
| P610 | Agent status: running while subagents or background jobs run; new unseen-finished status | Todo |

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

## P609 DB MCP: `browse_table` gets `in`, count and paging; descriptions steer reads to it

- Ask (user): agents run SQL for plain reads instead of `browse_table`. Update the tool descriptions and make `browse_table` support `in`, count and paging.
- Context: `browse_table` (`apps/kira-space/internal/dbmcp/browse.go`, schema in `server.go`, text in `internal/llmprompts/registry.go` `DbmcpToolBrowseTable`/`DbmcpToolRunQuery`) reads at most 100 rows with ANDed filters and order, so agents fall back to `run_query`. Masked columns, column review, expensiveQuery plan check and statement timeout must keep applying to every new feature. No `run_query` refusal in this phase (user chose descriptions plus capability only).
- Acceptance: filter operator `in` (list of values); count mode returning the matching row count; paging (offset or cursor, planner decides per engine) with a stable order and a next-page marker. `run_query` description leads with "not for plain row reads: use browse_table"; `browse_table` description leads with a one-line example and lists the new options. Adapter kinds without structured browse keep refusing as before. Arg descriptions, Settings prompt catalog, ARCHITECTURE and any MCP docs updated.
- Tests: flow test (backend, real DB where the browse suite already runs) and matching UI spec only if a UI surface changes; masking and limit interactions covered. Run for real.
- Result:

## P610 Agent status: running while subagents or background jobs run; new unseen-finished status

- Ask (user): agent status detection is good but misses cases. (1) A session whose main turn stopped while its subagents still work, or while background jobs run, shows as "no agent running". It must show running. (2) New status: finished but not yet seen. An agent that just finished, whose result the user has not opened, must look different from one that finished hours ago.
- Context: find where the app derives agent/session status (ADE sessions, session list, badges, any status enum shared with the frontend) and where it detects subagents and background jobs from the Claude harness stream. Seen state needs a defined moment (user opens the session or its result) and persistence across restarts.
- Acceptance: planner decides exact signals. Status is running while the main turn runs, or any subagent or background job of that session is live; never idle-looking while work continues. New `unseen` status (finished, not yet viewed) with its own visual, cleared when the user views the session, persisted, shown wherever status shows (lists, badges, tabs, notifications if any). Catalog copy, ARCHITECTURE and any status docs updated.
- Tests: flow test (backend derives status from a recorded stream with a live subagent and background job after the main turn ends, plus unseen set and cleared) and matching UI spec (same scenario shows the statuses). Run for real.
- Result:
