# Chapter run (v0.3.23)

Head at start: `269b3ee83`. One full non-UI run.

## Ran, all green after fixes

- `bun run typecheck` (all 10 projects), `bun run lint` (biome + `lint:rules`), `bun run lint:dead` (knip).
- `bun run test:unit` (2409 pass), `packages/api-core` tests (277 pass), `bun run verify:packaging`.
- `go mod tidy -diff`, `go vet ./...`, `go build ./...`.
- `go test ./...` (root module and `apps/kira-space`), then `bun run test:flows` (30 packages ok).

## Failures found

1. `internal/conventiontest` `TestGoConventions`: G1 at `bridge/apiconfig.go:92`, a literal `ipcerr.BadRequest` message
   in `EditVariables`. Real defect. Fix: `ipcerr.Contract` (UI always sends 1-500 edits). Commit `497c0a1e7`.
2. `flows/adeflow` `TestAgentBranchCreateNoPrompt`: contract snapshot read `conflictCheck: "checking"`, the async
   check had not settled. Test race, seen under load in the full run; passed alone 3 times before the fix. Fix:
   wait until every row is `done` before the snapshot. Commit `bec514450`. Passed 3 repeats plus the
   `-p 1` flows run.
3. `flows/dockerflow` `TestStatusClasses/daemon-down` and `gitaskpass`
   `TestRunHelper_BrokerNeverAnswersExitsOneWithinItsOwnTimeout`: unix socket path too long under the
   per-worktree `GOTMPDIR` (scratchpad path). Environment only, not a code defect; pass with `GOTMPDIR=/tmp/gt`.

No `dbmcpflow` or other known flake recurred.

## Not run

- `golangci-lint` (`lint:go`): binary built with go1.25, repo targets 1.27.3, cannot load here. `go vet` and the repo
  rule scripts ran instead.
- Docker-dependent tests: no `dockerd` started, images absent and disk (~3 GB free) too small to pull. Container
  adapter and `dockerflow`/`dbmcpflow` container paths skip. Run in PR CI.
- Playwright `ui`, `visual`, `ui-timing`, screenshots; real `claude` tests; complete-suite container tests (out of scope).
- Packaging artifact checks A1/A3/A4/A5/N2/N3 (no `.app`/`.dmg`) and shellcheck (not installed).
- Second full suite: not needed; fixes touched one error call in `bridge` and one test.
