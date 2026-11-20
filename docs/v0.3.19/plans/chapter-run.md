# Chapter v0.3.19 non-UI full run

Base: `07778a23e` (P693 part 2 results).

| Suite | Result |
|---|---|
| `bun run typecheck` (10 projects) | pass |
| `bun run lint` (biome, `lint:rules`, copy guards, ui-primitives, docs) | pass |
| `bun run lint:go` | pass |
| `bun run lint:dead` (knip, from pre-push) | failed first run, pass after fix |
| `bun run test:unit` (unit, packages, site scripts) | 2403 tests, 12 failed first run, 0 after fix |
| `sh scripts/go-test-split.sh rest` | 135 packages ok, 2 failed first run, 0 after fix |
| `bun run test:flows` | 28 packages ok, same 2 failures, 0 after fix |

Not run:
- `go-test-split.sh containers` group (adapters, ipcfixture, dockerflow, dbflow, dbmcpflow): `dockerd` down, images need 7.5 GB, 3.2 GB disk free. Runs in PR CI.
- Real `claude` tests (`apps/kira-space/tests/claude`): opt-in, cost tokens.
- UI suites: PR CI.

## Failures and fixes

- 12 unit tests: P693 hooks read bare `__KIRA_DEBUG_HOOKS__` at module scope (`createTerminalsStore.ts`, `terminalRenderer.ts`); Bun has no vite `define`, so `ReferenceError`. Fix: define it `false` in `packages/workbench/src/testing/unit/window.ts` preload. Commit `5276bbdcd`.
- `flowharness` `TestHarnessBootsWithDefaultSettings` (61 services) and `llmpromptsflow` `TestList` (233 prompts): P690 memory removal dropped 3 services and 64 prompts. Fix: counts 58 and 169. Commit `cb37c7747`.
- knip: `.github/workflows/pr.yml` line 148 step name held an unquoted `: `, invalid YAML (from `36c8e2df1`); would break PR CI. Fix: quote the name. Commit `1126c7665`.

Reran only the failing specs and packages; full `test:unit` rerun after the preload fix (shared preload): 2403 pass.
