# v0.3.26 non-UI full run

Branch `chapter/v0.3.26`, phases P756 to P760 landed. Start commit `d238bd870`. Env: 4 CPU, no Docker daemon.
Prep: `bun run setup`, `bun run build` (not `build:test`), `touch apps/kira-space/frontend/dist/.keep`,
`PATH` with `$(go env GOPATH)/bin`, `CGO_ENABLED=1`, `fs.inotify.max_user_instances=8192`.

## Commands and results

- `sh scripts/go-test-split.sh rest -timeout 15m` (every Go package of both modules except the
  containers group; includes `flows/...` bar dockerflow, dbflow, dbmcpflow): 2 failures, both fixed below.
  - `TestHarnessBootsWithDefaultSettings`: bound services 56, test wanted 57.
  - `TestGoConventions`: `ade/artifacts.go:119` G3 (slog call without literal `"scope"` key).
- `bun run typecheck`: pass.
- `bun run lint` (biome + all `lint:rules` guards): pass.
- `bun run lint:dead` (knip): pass.
- `bun run lint:go`: 0 issues.
- `bun run test:unit`: 2402 pass, 0 fail (198 files; includes site/scripts and package tests).
- `bun run verify:packaging` (after `bun run build`): pass.
- `bun run test:flows`: pass (flows and flowharness; docker flows skip without a daemon).
- `bun run site:build`: fails at Astro prerender, `Missing screenshot framed/build-start-board.webp. Run: bun run screenshots`.
  Needs the screenshots UI suite output, so not runnable here. Site typecheck (`typecheck:site`) and
  site unit tests passed above.

## Fixes

1. `fix(ade): give the slow-artifacts log a literal scope key`. Chapter-introduced (`cdae0fe3f`); G3 reads only
   literal slog args and the spread `attrs...` hid the key. Now `slog.With("scope", "ade").With(attrs...).Info(...)`.
2. `test(flowharness): expect 56 bound services after the kibana removal`. P758/P759 dropped `KibanaService`.
3. `test(dbmcpflow): scale the throttle test's slow query under -race`. Root cause confirmed: pure-Go SQLite under
   the race detector runs the 2.5M-row CTE in over 30 s, so `run_query` waited out the throttle's 30 s
   `E_TIMEOUT` (failed alone, `-race`, 33 s). Added `slowRows` to `race_test.go` (250,000) and `norace_test.go`
   (2,500,000), the existing race-aware const pair. `-race -run TestThrottle` passes in 26 s; non-race unchanged.
4. `test(adeflow): let the second step attach the worktree before TestBranchChange switches it`. Real ordering
   race in the test, not a product bug. `createFirst` returns when step one is done; step two then attaches the
   worktree to `feat/old`. Under load that came after the test's out-of-band `git switch`, undoing it: diagnostic
   showed own-folder HEAD back at `refs/heads/feat/old` with the board never showing the mismatch, so no wait length
   helped (120 s waits also failed, 2 of 3 runs under 6 CPU hogs with `-race`). `newChangeFixture` now waits for
   step two to run. After: `-race -count=5` of the first subtest and `-race -count=3` of the whole test plus
   `TestSmartRunSandboxed`, all under 6 CPU hogs, pass. `TestSmartRunSandboxed` never reproduced (lone and
   under-load reruns pass); no change made.

None of the fixes touch shared code (one log call, three test files), so no full-suite rerun. Reruns after fixes:
conventiontest, flowharness, ade, dbmcpflow (whole package, non-race), adeflow targeted.

## Intermittent, not fixed

`TestAutomationRun` (`gate both ways` at `automation_test.go:292`, then `normal script from a task` at `:398`,
both 20 s `waitFor`) failed twice back to back right after the heavy load runs above, in a 5-package parallel
`go test` and a lone rerun immediately after. Machine idle afterwards: lone run passes, `-count=8` passes
(68 s), the earlier rest-group run passed. Treated as load timing. Same terminal-output wait family as the
other adeflow load timeouts; no ordering race found, not root-caused.

## Not run

- Docker and real-container suites (containers group of `go-test-split.sh`: adapters, dbflow, dbmcpflow engines other
  than SQLite, dockerflow, ipcfixture; `test:flows:complete`; `test:images`): no daemon.
- UI suites: Playwright `ui`, `visual`, `ui-timing`, `screenshots`, mobile, ipc-frontend, `e2e-real`.
- Real `claude` tests (`test:claude*`), perf suites, `site:check`.
- A whole-repo `go test -race`: only the suspects ran under `-race`.
