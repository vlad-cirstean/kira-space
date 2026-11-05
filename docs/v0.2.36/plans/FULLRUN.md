# Full suite run, v0.2.36

Env: 4 cores, CGO_ENABLED=1, tip d473bf022 plus the two fix commits below. Docker daemon up; most test images already local.

## Go
- `rest` split (223 packages minus adapters, ipcfixture, dockerflow, dbflow), `go test -p 1`: 145 ok, 78 no test files, 0 failures. Includes dbmcpflow (TestThrottleSerialAcrossUserAndMcp passed) and Space flows.
- dbflow, dockerflow: pass (5.9s, 45.6s).
- Containers split: all `internal/adapters/*` (clickhouse, kafka, kibana, kibana/kql, mysqlfamily, postgres, redis, s3, sqs, sqlite, testsupport) and `internal/ipcfixture` pass.
- F1 `adapters/mongo` all tests failed: image `mongo:7.0.45` not local, Docker Hub 429. Env, not code. Pulled via `mirror.gcr.io` and re-tagged (DEV_ENVIRONMENT recipe); package passes on rerun.
- lint:go 0 issues. `go vet ./...` clean.

## Frontend static
- lint (biome + lint:rules + check-docs), lint:dead, typecheck: green.
- test:unit: 2013 pass, 0 fail (194 files).

## UI
- Studio ui + ui-clipboard: 822 passed (16.6m).
- Space ui: 1141 passed (20.6m).
- Space mobile (mobile-ios + mobile-insecure, Android project dropped in d071d3ab3): 42 passed.
- Studio ipc-frontend: 7 passed. Proto: 6 passed.
- ui-timing: 4 passed. The two budget failures documented on v0.2.35 did not recur.
- Space visual: 26 passed.
- Studio visual: 3 failed first run, all intended UI changes (diff images checked):
  - F2 approvals-dialog: result-review option row and restructured approval body (P536).
  - F3 settings-database-mcp: description now ends "per mode." (P536).
  - F4 settings-advanced: expensive-query threshold description (P530).
  Re-recorded in dca7deec0; full visual project reran without update: 22 passed.

## e2e-real
- Studio (9): mariadb-real x2 failed first run. F5 fixture image `mariadb:11.4` was a floating tag never pulled by `test:images`, Docker Hub 429. Fix: pin `packages/db-fixtures/support/mariadb.ts` to `mariadb:11.4.13`, the tag the Go side pins and the mirror pull provides (4d139523f). Rerun: 2 passed. Other 7 passed first run.
- Space (4): passed.

## Not run
- `bun run test:claude*` (real claude, no API key).
- Complete adapter matrix (`KIRA_TEST_MATRIX=1`, `KIRA_FLOW_COMPLETE=1`): CI covers it.
- Perf probes, screenshots, site:check.

No fix touched shared code beyond one fixture constant and three baselines; no second full run needed.
