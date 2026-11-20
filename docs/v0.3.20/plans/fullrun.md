# Full non-UI run, v0.3.20

## Result

Ran on HEAD 0dc8e460a plus the two fix commits below, after regenerating bindings.

Passed first time:
- `bun run typecheck` (all 10 projects, astro check 0 errors).
- `bun run lint` (biome and `lint:rules`: tokens, theme classes, UI primitives, ADE colours, class conflicts, conventions, copy, catalog, checkprompts, check-docs), `bun run lint:dead` (knip).
- `bun run test:unit`: 2399 pass, 0 fail (covers site scripts, packages, mobile-facing unit specs).
- Go `go test ./...` outside flows, adapters and ipcfixture, `-p 2`.
- `bun run test:flows` (every flow package and flowharness), adapters and ipcfixture. Docker-backed adapter and flow tests skipped: no daemon.

Failures found and fixed:
- biome warning, unused `Button` import in `ResponseDiffDialog.vue`. Commit `5d77ef11d`.
- `lint:go`: gocognit on `ade.(*TaskBoard).RequiredServices` (60) and gocognit and gocyclo on `taskfiles.(*Service).one`. Both split into helpers, behavior unchanged. Commit `dcaf4c0b9`. Reran `golangci-lint` on `ade` and `taskfiles`: 0 issues. Reran `ade`, `taskfiles`, `adeflow`: pass.

Could not run:
- `-race`: disk (about 150 to 300 MB free) fails the cgo link of race test binaries. Not run.
- Full `golangci-lint run` after the fix: ran out of disk (ENOSPC typecheck). The earlier full run listed exactly the three findings above; only the touched packages were relinted.
- `proofflow` after the `taskfiles` refactor: ENOSPC at link. It passed before the refactor; `taskfiles` unit tests pass after it.
- Docker-backed suites (complete flows, adapter matrix): no Docker daemon.
- Nested module `apps/kira-space/tests/claude` (real `claude`): out of scope.
- UI suites: out of scope.
