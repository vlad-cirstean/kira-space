# v0.3.24 full non-UI run

Branch `chapter/v0.3.24`, base `a4074c9a2`. No failures needed code fixes; no fix commits, so no second full run.

## Ran, all green

- `bun run lint` (biome, lint:rules incl. check-docs 24 files): pass.
- `bun run lint:go` (golangci-lint): 0 issues. `bun run lint:claude`: pass. `bun run lint:dead` (knip): pass. `go build ./...`: pass.
- `bun run typecheck` (all 10 projects, astro check 0 errors): pass.
- Go: `sh scripts/go-test-split.sh rest` 139 packages ok; `containers` 16 packages ok (Docker daemon up, images via `bun run test:images`); `bun run test:flows`-equivalent (`go test -p 1 ./flows/... ./flowharness/...`) ok; `-race` on `internal/docker`, `internal/shell` (TestEngine PASS, not SKIP) and `mobileterm`: ok. CI runs no repo-wide `-race`.
- `bun run test:unit`: 2411 pass, 0 fail (200 files; includes `site/scripts` and package specs).
- `bun run verify:packaging`: all checks passed (shellcheck and .app/.dmg checks skipped, not present here).
- `bun run build:test`, then `ade-v2-artifacts`, `ade-v2-links-strip`, `ade-v2-issue-tracker`, `ade-v2-details-links` (ui project): 48 passed.
- `bun run test:claude:db -count=1` (auth probe printed `ok`): 14 of 16 passed, spend 0.0923 USD.

## Not run

- `bun run site:check`: `site:build` fails at `/build` with "Missing screenshot framed/build-start-board.webp. Run: bun run screenshots". Screenshots come from the excluded screenshots suite (generated in `pages.yml` CI). Site typecheck, check-docs and site bun specs passed above; site Playwright specs not run.

## Real-claude failures, not reproducible

First run: `TestDbMcpBrowsePreferredThroughClaude` and `TestKubeMcpKeptChainThroughClaude` failed (haiku model behavior).
- BrowsePreferred: lone rerun passed.
- KubeKeptChain: "kept = true, read through prepare = false" (model did not read logs through the kept name). Three lone reruns passed. Test and code under it untouched by this chapter. Model flake; no change made. `TestDbMcpThroughClaude` (earlier flake) passed.
