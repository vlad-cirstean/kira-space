# Chapter v0.3.6 non-UI full run

Worktree `/home/user/kira-wt-full`, base 81a0eb7e5 (P589 excluded, UI-only).

Ran:
- `sh scripts/prepare-worktree.sh`, `go build ./...`, `go vet ./...`: clean.
- `bun run lint:go` (pinned golangci-lint, root and `apps/kira-space/tests/claude`): 0 issues.
- `go test -p 1` over every root-module package except the Docker-needing set (see Skipped).
- `bun run typecheck:web`, `typecheck:tests`, `typecheck:unit`, `typecheck:site`: clean.
- `bun run lint` (biome plus lint:rules), `bun run lint:dead` (knip): clean.
- `bun run test:unit`: 2014 pass, 0 fail.

Failures found: none. All Go packages passed (flows, conventiontest/guards included), no timeouts.

Fixes: none needed. Added the missing P585 test (f70dfd53b): `TestConfigSyncReposWatchFolderRescan`
in `flows/configsyncflow/repos_watch_test.go`. A watched folder imports a repo matching the shared
origin; `Status().Changes` rises with no manual fetch. Verified it fails with the `onReposChanged()`
call removed from `appwire/wire.go`. No contract needed.

Shared code touched: none, so no second full run.

Skipped:
- `internal/adapters/...`, `internal/ipcfixture`, `flows/dockerflow`, `flows/dbflow`: need Docker, CI covers them.
- Nested module `apps/kira-space/tests/claude`: real `claude` tests, opt-in, lint only.
- Playwright suites and `packages/workbench/src/approvals`: P589 stream.
