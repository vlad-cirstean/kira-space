# Chapter v0.3.4 non-UI full run

Base head: b35e19ec3. Wails bindings regenerated first (`wails3 task common:generate:bindings`), dockerd started, `bun run test:images` pulled.

## Commands

- `bun run typecheck`: pass.
- `bun run lint` (biome, check-tokens/theme/ui/ade/class-conflicts/conventions, copy, catalog, checkprompts, check-docs): pass.
- `bun run lint:go` and `bun run lint:claude`: 0 issues. `go vet` in nested `apps/kira-space/tests/claude`: clean (tests not run).
- `bun run test:unit` (app, package, scripts, site scripts specs): 2014 pass, 0 fail.
- `bun run test:go:rest -count=1`: 2 failures, fixed (below), reruns pass.
- `bun run test:go:containers -count=1` (adapters, dbflow, dockerflow, ipcfixture, real Docker): all ok.
- `bun run test:flows -count=1`: all ok (31 packages).
- Skipped: Playwright suites, real `claude` tests, complete (matrix) suites.

## Failures

1. `internal/conventiontest` TestGoConventions: `internal/shell/menu.go:90` slog call without `scope` key (G3). Added `"scope", "menu"`. Commit 2da5cdef8.
2. `flows/llmpromptsflow` TestList: listed 265 prompts, test wanted 250. Count grew by orchestrator prompts (`create_task`, `list_tasks`, `task_chain` tools plus args, `ade.pass.handover`). Test constant updated to 265. Commit ecb6e7739.

Targeted reruns of both packages plus `internal/shell` pass. Fixes touched no shared code, so no second full run.

## Unfixed

None.
