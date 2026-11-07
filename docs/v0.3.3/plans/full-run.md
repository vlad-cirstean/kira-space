# v0.3.3 full non-UI run

Base HEAD 77ce72f8. Linux sandbox, no Docker daemon.

## Passed
- `bun run lint` (biome + lint:rules, check-docs 25 files): pass.
- `bun run typecheck` (12 projects incl. astro check, 0 errors): pass.
- `bun run test:unit`: 2014 pass, 0 fail, 193 files (includes site/scripts).
- `go test` all packages except adapters, ipcfixture, flows, flowharness: 104 ok, 0 fail.
- `bun run lint:go`: 0 issues. `bun run lint:dead` (knip): clean.
- `bun run test:flows`: first run 1 fail (below); rerun after fix 31 ok, 0 fail.

## Failure
- `appflow` `TestStartupSweepsLeftovers`: waited for git.sock sweep, removed in cc1a39bf (P557/P559). Test left behind when leftovers_test.go was deleted. Predates nothing: introduced this chapter. Fix `52fcde80`: delete test plus unused `flowharness.WithSeed`.

## Not runnable
- Real-container suites (`internal/adapters/*`, `ipcfixture`, Docker flows, `test:flows:complete`, `test:compat`, e2e-real): `failed to connect to the docker API at unix:///var/run/docker.sock ... no such file or directory`. Adapter packages ran but container tests skip: not counted as passed.
- `tests/claude` real-claude suites: opt-in, excluded.
- Playwright ui/visual/ui-timing/screenshots: excluded by rule (PR CI).

## Second full run
Not needed. Fix touched flowharness (shared by flows); reran all flows, plus go vet/build, golangci-lint and knip. No package, fixture or config shared with other suites.
