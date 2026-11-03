# v0.2.29 full suite run

Base: dd416364 (after P492). Fixes landed as follow-up commits.

## Ran

- `bun run setup` (scripts/setup.sh), `build:space-mobile`: ok.
- `typecheck`: pass. `lint` (biome + rules): pass. `lint:go`: 0 issues. `lint:dead`: pass.
- `test:unit`: 1993 tests, 1 fail, fixed, rerun pass.
- `test:go:rest`: 1 fail (`TestGoConventions`), fixed, targeted rerun pass.
- `test:go:containers`: all `ok`; no Docker in this sandbox, so container-backed tests skip.
- Studio UI (`ui` + `ui-clipboard`): 673 passed, 19.6 min.
- Space UI (`ui`): 1034 passed, 24.6 min.
- Contract fixtures: unchanged.

## Failures fixed

- Unit `mock-runtime-bindings`: `KubernetesService.Reorder` (P486) missing from mock runtime and IPC channel tables. Added both entries in `apps/kira-studio/tests/ui/support/`. Commit 95962777.
- `TestGoConventions` G1: `ipcerr.BadRequest` literals in `apps/kira-space/internal/bridge/openedfiles.go` (P489). Switched to `ipcerr.Contract` (caller bugs); flow test now expects `CodeContract`. Commit cd819559.

No fix touched shared code (package, fixture, config), so no second full run.
