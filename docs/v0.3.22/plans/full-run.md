# v0.3.22 non-UI full run

Base: chapter/v0.3.22 at 9df511850. Scope: Go (rest, containers, flows), typecheck, lint, knip, site, package, bun unit tests.

## Results

| Suite | Result |
| --- | --- |
| `bun run lint` (biome, N1-N19, copy, prompts, docs) | pass |
| `bun run typecheck` (10 projects) | pass, 0 errors |
| `bun run test:unit` | 2410 pass, 0 fail, 200 files |
| `bun run verify:packaging` | pass (artifact checks skip: no .app/.dmg) |
| `bun run lint:dead` (knip) | fail, fixed (F1) |
| `bun run lint:go` + `lint:claude` | fail, fixed (F2), then 0 issues |
| Go `rest` group (`go-test-split.sh rest`, incl. all flows) | 140 ok packages, 0 fail |
| Go `containers` group, `KIRA_FLOW_DOCKER=require` | first run 6 packages failed (F3), all pass on lone rerun |
| `site:build` (astro check, build, icons, share image) | pass |
| site Playwright (webkit, phone+desktop, visual excluded) | 173 pass, 1 skip, 50 fail: 2 fixed (F4), 48 placeholder shots (F5) |
| `verify:packaging` | pass |

## Findings

### F1 knip: unused export `MODULE_ACTIONS`

`apps/kira-space/frontend/src/shortcuts/moduleActions.ts` exported `MODULE_ACTIONS`, used only inside the file. Cause: P712 (`0c266bcfb`). Fix: drop `export`.

### F2 golangci-lint gocritic x2

`internal/releaseline/index_test.go` (`stringXbytes`) and `scripts/releaseindex/main.go` (`preferStringWriter`). Cause: release index commits `afbfc29e5`, `42751b4be`, not this chapter. Fix: `bytes.Equal`, `os.Stdout.WriteString`.

### F3 containers group: 6 packages failed on first run

clickhouse, kafka: container start timeout (wait until ready) under load beside kibana. dbflow, dbmcpflow, dockerflow, ipcfixture: `no space left on device` (disk 99%). No code cause. Each passed on a lone rerun with GOTMPDIR cleared. Freed space by deleting `/tmp/ksf*`, `/tmp/node-compile-cache` and merged stream worktree `kira-space-v0.3.22-p711/node_modules` (regenerable by `bun install`).

### F4 site docs.spec "page list is not empty"

Floor `>= 25` but 23 doc pages exist since v0.3.19 (`754f1ac0d` removed memory pages). Not this chapter. Fix: floor 20.

### F5 site shot specs: 48 failures, open (environment)

Every failure is `shot-placeholder.svg` instead of the real shot, plus the hero 404 at `shots/<id>.webp` and the link-count check that dedupes the shared placeholder URL. Real shots need `bun run screenshots` (Playwright `screenshots` project, out of scope). CI pages job builds them. Not a code fault.

## Second full run

Not needed: no fix touched shared Go code, packages, fixtures or config. Targeted reruns covered each fix.
