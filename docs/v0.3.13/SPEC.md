# v0.3.13: functional verification by user journeys

Goal: verify every feature works as a user uses it, via complex journeys with far-end oracles
(clipboard contents, file on disk, DB row, remote git state), not screen-only assertions. Add or
improve flow tests and UI specs per the IPC-boundary split; fix every product bug found.
First `P` is P626.

| P | Phase | Status |
|---|---|---|
| P626 | Inventory of full history and map of existing e2e coverage (Opus) | Done |
| P627 | Journey imagination: new complex scenarios with definition of correct (Fable, no plan, no implementation) | Done |
| P628 | Synthesis: extend/create decisions, 3 streams plan (Opus) | Done |
| P629 | Stream A: tests and fixes | Done |
| P630 | Stream B: tests and fixes | Done |
| P631 | Stream C: tests and fixes | Done |
| P632 | Chapter non-UI full run, 2 code review rounds | Done |
| P633 | Close chapter | Done |

Deliverables of P626-P628 live in `plans/` (inventory.md, coverage-map.md, journeys-*.md, synthesis.md).

## Results

### P629 Stream A: tests and fixes

Done. A0 to A13 landed with flow half and UI half. Decisions built: lost ack keeps staged edits (2), concurrent-edit
guard `E_CONFLICT` in grid, Mongo replace, saved query, API request (3), `_id` edit refused (4), S3 unreadable ACL
warning (6), SQS repeated attribute refused (7), tab close with open transaction rolls back (8). 18 product bugs fixed,
among them: SQLite composite-key editing, empty paste to NULL, lost COMMIT ack message, Redis update resurrecting expired
key, S3 overwrite of replaced object, ClickHouse user FORMAT, console splitter on triggers, page token ignoring key
columns, SQLite Definition DDL indexes, Postman re-import name, gRPC deadline covering dial, non-ASCII metadata. New
contracts `studio-grid-lost-ack`, `api-copy-url`, `api-request-two-windows`. Own Go flow packages and UI specs pass.

### P630 Stream B: tests and fixes

Done. B1 to B16 landed, flow plus UI halves. Bugs fixed: archived finder missed Jira keys, tool calls after
finish_step, loop-back step re-run, stacked rebase abort record, shared draft row across approved branches, resume prompt
lease line, artifact URL normalisation, double Restart of a restarting container, multi-row log copy lost line breaks.
Decisions built: stale-base workflow save conflict, switch confirm counts, stage move refused during live headless run,
lease TTL and silent-tool ceiling count app-running time only. Seams added: `WithLeaseClock`, `WithTrackerGrace`,
`WithMobileClock`. Dropped or pinned: J-HAR-002, J-ADE-072, J-DKR-011, J-DKR-015. Stale contracts regenerated
(`memory-import`, `dbmcp-expensive-query`, `github-links-in-app`). Verified under `KIRA_FLOW_DOCKER=require
KIRA_FLOW_COMPLETE=1`, `-race` clean; `bash_nohup` flaked once at 2.0017s vs 2s bound.

### P631 Stream C: tests and fixes

Done. C1 to C15 landed (C6, C7, C10, C12, C14 not in this stream's plan). Bugs fixed: redirect limit off by one,
unresolved `{{var}}` sent literally (OQ-API-2), curl `-G` with `--data-urlencode`, configsync same-second backup
collision, Kubernetes truncation banner and Copy log. Pinned to existing behaviour: J-REM-011/012, J-GIT-026, J-WT-013,
J-GIT-020, J-REPO-002, J-REV-001, J-BC-009, J-BC-025. Secret hygiene oracle made truthful (store API plus scan of every
other file). `flowharness.Snapshot`/`CrashTo` crash model added; SIGKILL harness stays follow-up. `bun run test:flows`
31 packages pass.

### P632 Chapter non-UI full run, 2 code review rounds

Done. Full run (typecheck, lint, `lint:go`, `test:unit` 2,290 tests, `site:build`, Go non-flow packages, every flow
package with `KIRA_FLOW_COMPLETE=1 KIRA_FLOW_DOCKER=require`) green on the final tree after fixes: stale generated
bindings regenerated; `grid-pending-by-primary-key` specs given the `expect` field; F-A11 import cycle (eager
`useTabsStore` read in `terminalTabs.ts`, now a thunk) and eight specs mocking the wrong `tabsSave` (new
`support/benignTabsSave.ts`); 11 `lint:go` findings; G2 literal `E_UNKNOWN`; `TestConsoleRedis` map-order race
(ordered slice, contract regenerated); `TestDbMcpEngineModes` `waitListed` matching the EXPLAIN pre-check op;
`bash_nohup` bound equal to `closeGracePeriod` (now a var, Close tests widen it). Disk filled twice on the shared host;
those failures were not code faults. Review round 1: 2 high, 6 medium, 6 low, all fixed (lost-ack reconcile, Postgres
guard on `xml`/`point`-class types, console transaction on tab close, API request guard, MySQL FLOAT guard, error
`details` frame, expect-name validation, workflow count recheck, git-ui carry retry). Review round 2 over the full
diff since round 1's base: 1 high, 4 medium, 5 low, all fixed (console transaction state read from the engine and
MySQL implicit-commit rules, tab-closed-mid-run path, forced-overwrite resend routed through lost-ack handling,
reconcile re-bases a guard only when the reloaded text equals the original, `TypeRoundTrip` stale check over every
guarded column). Follow-ups moved to `docs/v0.3.14/SPEC.md` and ARCHITECTURE.md Known open items.

### P633 Close chapter

Done. Result folded into `docs/history/06-v2.8.md`; `docs/v0.3.13/` removed; ARCHITECTURE.md Known open items updated;
PR opened from `chapter/v0.3.13` to `main`.
