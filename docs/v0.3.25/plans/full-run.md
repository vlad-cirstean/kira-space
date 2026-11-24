# Chapter v0.3.25 non-UI full run

Base: `50f914674`. Host: 4 cores, no Docker daemon. No code failures; no fix commits.

## Commands

| Command | First run | Final |
|---|---|---|
| `bun run typecheck` | fail (stale bindings) | pass |
| `bun run lint` (biome + `lint:rules`) | pass | pass |
| `bun run lint:go` | pass | pass |
| `bun run lint:dead` (knip) | pass | pass |
| `bun run test:unit` | 2409 pass, 2 fail (stale bindings) | 2411 pass, 0 fail (199 files) |
| `bun run verify:packaging` | fail (test bundle in `frontend/dist`) | pass |
| `bun run test:go:rest` | pass (136 packages ok) | pass |
| `bun run test:go:containers` | pass (16 packages ok; container tests skip without Docker) | pass |
| `bun run test:flows` | pass (30 packages ok) | pass |

## Failures

1. `typecheck:unit` TS2339 `GitHostService.Tools` (`bridge/index.ts:669`) and `mock-runtime-bindings.spec.ts` (2 tests: `HostViewService.ChangeRequest`/`Issue`, `GitHostService.Tools` unbound or unmapped).
   Root cause: gitignored `frontend/bindings/**` generated before P751 removed viewers and issue tracker. No source fault.
   Fix: `wails3 task common:generate:bindings` per DEV_ENVIRONMENT. No tracked file changed. Reran `typecheck`, `test:unit`, `lint:dead`: green.
2. `verify:packaging` "debug hooks in packaged bundle".
   Root cause: `frontend/dist` held a `build:test` bundle. Fix: `bun run build`, rerun green. No tracked file changed.

## Not run

- `site:build` and site Playwright specs (`build`, `docs`, `site.spec`): `astro build` fails with `Missing screenshot framed/build-start-board.webp`. The gitignored `screenshots/framed` comes from the excluded `screenshots` project. Site typecheck (`astro check`, 0 errors) and site script specs (inside `test:unit`) passed.
- Docker-backed container tests (adapters, `dbflow`, `dbmcpflow`, `dockerflow`) skipped by design without a daemon; `test:flows:complete` not run.
- `verify:packaging` artifact checks A1/A3/A4/A5/N2/N3 skipped (no `.app`/`.dmg`); shellcheck absent.
- Nested module `apps/kira-space/tests/claude` (real `claude`, opt-in) excluded.

## Open

None.
