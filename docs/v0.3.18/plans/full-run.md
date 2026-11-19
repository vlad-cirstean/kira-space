# v0.3.18 non-UI full run

Base: chapter tip `ff939429e`. Worktree `kira-space-full`, branch `stream/v0.3.18-full`. Linux sandbox, dockerd up, test images preloaded.

## Commands and results

- `bun run typecheck`: pass (11 projects, astro check 0 errors).
- `bun run lint` (biome plus lint:rules incl. check-ui-primitives, check-catalog, checkprompts, check-docs): pass after fix 1.
- `bun run lint:dead` (knip): pass.
- `bun run lint:go` (golangci-lint): 0 issues.
- `bun run test:unit` (unit, packages, scripts, site scripts): 2404 pass, 0 fail, 203 files.
- `bun run site:build` (placeholder shots): pass, 28 pages, icon and share-image checks pass.
- Go non-flow: `go test -p 3` on 203 packages (every package outside `internal/flows` and `internal/flowharness`, adapters general suites included): 127 ok, 73 no test files, 3 FAIL (below).
- Go flows: `go test -p 1` on `internal/flows/...` and `internal/flowharness/...`: 31 ok, 5 no test files, 0 fail. Whole `dbmcpflow` (99 s), `llmpromptsflow`, `dbflow` (102 s), `adeflow` (235 s) ok.
- Contract tests: Go flow halves ran in the flows run; `git status` clean afterward (no fixture rewritten).
- Not run, per brief: Playwright ui, visual, ui-timing, screenshots, e2e-real, tests/claude.

## Failures and fixes

1. `internal/conventiontest` TestGoConventions: 4 G1 hits, literal or `fmt.Sprintf` messages in `adapters.New` from the bound-parameter phases (`adapters/clickhouse/params.go` x2, `adapters/mysqlfamily/console.go`, `adapters/sqlparams.go`). Fix `550b08216`: four catalog keys `backend.studio.adapter.{boundNameRequired,boundValuesTooLarge,boundListTooLarge,noBoundParams}.body` in `en.json`, callers use `adapters.T`. The ClickHouse size text now says bytes (check-catalog C6 rejects mid-sentence "KiB"). Reran conventiontest, i18n catalog, adapters, clickhouse, mysqlfamily, lint, lint:go, lint:dead, typecheck, test:unit: pass. Pre-commit hook ran full and passed. The fix touched the shared catalog (`en.json`); consumers are covered by lint, typecheck and test:unit above, and the flows ran after the fix.
2. `adapters/clickhouse` and `adapters/kafka`: container start timeouts (`wait until ready: context deadline exceeded`) under `-p 3` load. Lone rerun with `-p 1`: both ok (28.7 s, 25.0 s). No code change.

`adeflow` TestArchivedTaskKeepsData flake did not occur.
