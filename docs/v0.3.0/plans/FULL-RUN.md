# v0.3.0 full run

Base: aa01ade14. Host: 4 CPUs, Linux sandbox, dockerd started by hand, images from `bun run test:images`.

## Ran

- `bun run setup` (bindings regenerate): ok.
- `bun run test:go:rest`: 1 failure, fixed (below), rerun of the failing packages green.
- `bun run test:go:containers` with `KIRA_FLOW_DOCKER=require`: green.
- `go vet ./...`: clean. `bun run lint:go`: 0 issues. `bun run lint:dead`: clean.
- `bun run lint` (biome, rules, check-copy/catalog, check-docs, N1-N15): clean.
- `bun run typecheck`: clean. `bun run build`: ok.
- `bun run site:build` with `KIRA_SITE_SHOTS=placeholder`: ok (20 pages).
- `bun run test:visual:update` (only the `visual` project): 41 passed, baselines re-recorded.

## Failures found and fixed

- `TestGoConventions` G5: `mcpinstall.Registered` ran `claude mcp get` (prints stored headers). Now reads
  `claude mcp list` stdout for a `<name>: ` line. Commit f1eb9d73e.
- Visual re-record exposed a real regression: git graph rows drew dotted cell borders. Studio grid views
  bundle unlayered `slick.grid.css`; `git.css` rules sat in `@layer components` and lost. Rules are now
  unlayered. Commit a94e0a4ac. After the fix `git-graph*` and `git-row-menu` baselines are byte-identical to HEAD.

## Visual baselines

19 PNGs changed, commit 995b66a08. Each diff reviewed as expected:

- Studio shell (5): mode tab strip now lists the merged modules.
- Settings (4), approvals, runs-restore: new Data, Cache, API, Database MCP panes and fields.
- Operations (2): header controls.
- git-repos-dialog, workflow graph (5): file path now under `config/`.

Nothing surprising left.

## UI suites deferred to PR CI (per user)

Not run here: Playwright `ui`, `ui-clipboard`, studio, `ipc-frontend`, `e2e-real`, `mobile`, `ui-timing`,
`screenshots`, site Playwright specs. Known-risk items:

- `apiTest` fixture (fixtures.ts) landed in d00e89196. Specs restoring gRPC/HTTP tabs: grpc-request,
  api-ui-consistency, copy-coverage, list-keyboard, operations-db use it or click the API mode tab themselves.
  Read check of the remaining http-*, tab-switch-retention and api-cross-window-sync specs found each switching
  to the API mode before asserting; unverified by a run.
- `ade-v2-proof-chain` "a proof changed push refetches the task proofs": no race found by reading.
  `installAdeSignals` runs in main.ts before mount and subscribes `onProofChanged` to invalidate
  `['adetask','proofs']`, the key `useTaskProofs` uses; TanStack's invalidate cancels and restarts an
  in-flight fetch. Cause stays open until a run shows the failing step.
- `site/tests` specs that need real shot images (22 failures here, 404 on the hero shot) pass only after
  CI generates the shots; `visual.spec.ts` landing baselines were not re-recorded.

## Not run

- Real `claude` tests (`test:claude:db` rows P543 attach_proof, P548 db_mcp): no login in the sandbox
  (no `~/.claude/.credentials.json`; env holds only the proxy base URL).

## pages.yml review (origin 3f72d7f6c)

Matches plans/P550.md D6 step for step (key, restore, fallback set-aside, setup with bindings, WebKit cache and
retried install, capture with continue-on-error, fill, save gated on capture and fill outputs, configure,
build, upload, 30 min timeout). Deviation: the cache key hashes `ui/studio/support`, `ipc/support`,
`tests/support` and `ui/fixtures.ts` (the shot specs' real imports) in place of the plan's `tests/contract`
and `tests/fixtures`; the shot imports do not touch those two, so the key is adequate. No fault found.
