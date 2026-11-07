# v0.3.2 full non-UI run

Base: `bee193a32`. No failures found, no fix commits, no reruns. Second full run not needed.

## Ran

- Go: `go test -p 1 ./...` (root module, includes flows): 148 packages ok, 0 fail. No Docker daemon in sandbox: Docker-backed adapter, `dockerflow` and `dbflow` tests skipped, not passed. Not run: `test:flows:complete` (needs Docker).
- Lint: `bun run lint` (biome, token/theme/primitive/colour/class/convention checks, check-copy, check-catalog `--unused`, checkprompts, check-docs: 25 files 0 violations), `lint:go` 0 issues, `lint:claude` 0 issues, `knip` clean.
- `bun run typecheck`: 0 errors.
- `bun run test:unit`: 2014 pass, 0 fail (193 files; covers packages/*, site/scripts).
- Site: `astro check` and `astro build` pass with `KIRA_SITE_SHOTS=placeholder`. Playwright site suite: 174 passed, 1 skipped, failures only on tests needing generated screenshots (placeholder image names, 404 placeholder asset) and visual baselines. Cause: gitignored `screenshots/` output needs `bun run screenshots` (UI project, out of scope). Not run: real-screenshot site pass.
- Real `claude` (probe `ok`, haiku, `-count=1`): `TestMemoryMcpThroughClaude`, `TestMemoryMcpFollowsDocs` (5 subtests), `TestMemoryInstallRegisters`, `TestMemoryImport`, `TestMemoryIngestThroughClaude`, `TestMemoryGate`, `TestMemoryDistill`, `TestMemoryDistillBridge`, `TestMemoryDistillTrivia`, `TestMemoryDistillRewrite`: all pass, 299 s. `TestMemoryIngestEval` skipped (opt-in eval).
- Env note: isolated bun install keeps per-package `node_modules` symlink dirs; copy them too, else typecheck shows ~1000 false errors.
