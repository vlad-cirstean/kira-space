# Chapter v0.3.17 non-UI full run

Base: `chapter/v0.3.17` at bb174d8f8, clean tree. Suites ran sequentially, one at a time.
Setup: started `dockerd` by hand; `bun run test:images` found every image present.

| Suite | Command | Result |
|---|---|---|
| Go rest | `bun run test:go:rest` | pass (142 packages ok) |
| Go containers | `KIRA_FLOW_DOCKER=require bun run test:go:containers` | 3 packages failed on startup timeouts; pass on lone rerun |
| Typecheck | `bun run typecheck` | pass, 0 errors |
| Lint | `bun run lint` | pass |
| Lint Go | `bun run lint:go` | pass, 0 issues |
| Dead code | `bun run lint:dead` | pass |
| Unit, packages, site scripts | `bun run test:unit` | pass, 2386 tests, 201 files |
| Flows | `KIRA_FLOW_DOCKER=require bun run test:flows` | pass (31 packages ok) |

## Failures

`clickhouse`, `kafka`, `mysqlfamily` adapter packages failed in `test:go:containers`:
container readiness wait hit its deadline (`wait until ready: context deadline exceeded`, kafka
`Transitioning from RECOVERY to RUNNING matched 0 times`, MySQL `port: 3306 MySQL Community Server`
matched 0 times). Cause: about 9 containers started at once by parallel packages on 4 cores
(load average about 6). Not a code fault. Lone rerun
`CGO_ENABLED=1 go test -p 1 -count=1 ./internal/adapters/{clickhouse,kafka,mysqlfamily}/`: all ok
(28 s, 24 s, 35 s). No fixes, no code commits. No shared code changed, so no second full run.

## Not run

- Complete container suites (`KIRA_TEST_MATRIX`, `test:flows:complete`): on demand and in CI only.
- UI suites (Playwright `ui`, `visual`, `ui-timing`, screenshots, mobile) and real-`claude` tests: out of scope.
