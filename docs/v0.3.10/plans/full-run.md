# v0.3.10 full non-UI run

Branch `chapter/v0.3.10`, after P621. Docker daemon started by hand for the docker flows.

## Fixes

- `flowharness` `TestHarnessBootsWithDefaultSettings`: bound services 60 to 61. P620 `LeasesService` is
  bound on purpose (`appwire.go` `application.NewService(w.Leases)`). ARCHITECTURE has no current count
  (old counts are per-phase history).
- `llmpromptsflow` `TestList`: 271 to 277. P618/P620 registered 10 prompts (`budget.tsv` +10 rows: 4
  `change_branch`, 6 lease tools).
- `conventiontest` `TestGoConventions`: G5 flagged `internal/harness/claude/hooks/http.go` reading the
  `Authorization` header. It is the bearer check on Kira's own hook listener, not a CLI credential.
  Added a permanent exemption with that reason.

## Ran

- `bun run test:go:rest`: 2 failures above, fixed, rerun green.
- `bun run test:go:containers`: pass (docker-gated tests skip without daemon); with daemon:
  `dockerflow` and `internal/docker` pass.
- `bun run test:unit`: 2258 pass.
- `bun run typecheck`, `bun run lint` (incl. `lint:rules`, check-docs), `lint:go`, `lint:dead`,
  `lint:claude`: pass.
- Not run: `site:build` needs generated screenshots (`bun run screenshots`, out of scope); site astro
  check ran in `typecheck`, site specs in `test:unit`. Complete flow suite, UI and real-claude out of scope.

No fix touched shared code, so no second full run.
