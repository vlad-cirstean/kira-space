# P537 Part 2: Finish the merge in three parallel streams

Decisions: `P537.md`. Contract: `P537-part1.md` Result (rename table, tab-kind set, bindings hash).
Starts only after Part 1 is committed and accepted. Three named streams, cap raised to 3 by the user
for this Part. Each stream: one Sonnet implementer, own git worktree, own local branch, same base
commit (Part 1's last commit, written here as `<P1>`).

## Setup

```sh
for s in a b c; do
  git worktree add ../kira-p537-$s -b p537-stream-$s <P1>
  (cd ../kira-p537-$s && sh scripts/prepare-worktree.sh)
done
```

Branches `p537-stream-*` stay local; `.githooks/pre-push` refuses them. Each stream writes its own
notes file (below) and commits per logical group, so a stopped stream resumes from its last commit.

## File ownership (zero overlap)

| Path | A (Go) | B (frontend) | C (build, tests, docs) |
|---|---|---|---|
| `apps/kira-space/*.go` (`main.go`, `notify_*`, `locator_*`) | own | | |
| `apps/kira-space/internal/**`, `apps/kira-space/cmd/**` | own | | |
| `apps/kira-space/tests/claude/**`, `tests/claude-studio/**` | own | | |
| repo-root `internal/**`, `go.mod`, `go.sum`, `.golangci.yml` | own | | |
| `apps/kira-space/frontend/**` except generated `bindings/` | | own | |
| `apps/kira-space/tests/unit/**` | | own | |
| `packages/{shared,theme,workbench,api-core,docker-ui,git-ui,git-core,git-ipc}/**` | | own | |
| `apps/kira-space/{Taskfile.yml,build/**,tools/**,README.md}` | | | own |
| `apps/kira-space/playwright*.config.ts`, `tsconfig.json`, `tsconfig.tests.json` | | | own |
| `apps/kira-space/tests/**` except `unit/`, `claude/`, `claude-studio/` | | | own |
| `packages/db-fixtures/**` | | | own |
| root `package.json`, `bun.lock`, `bunfig.toml`, `biome.json`, `knip.json`, `tsconfig.json` | | | own |
| `scripts/**`, `tools/**`, `.githooks/**`, `site/**`, `README.md`, `NOTICES.md` | | | own |
| `docs/**` except the three notes files below and `docs/v0.3.0/SPEC.md` | | | own |
| `.github/**` (only through `docs/pending-workflows/`, `docs/pending-changes/`) | | | own |
| `docs/v0.3.0/plans/P537-part2-a.md` | own | | |
| `docs/v0.3.0/plans/P537-part2-b.md` | | own | |
| `docs/v0.3.0/plans/P537-part2-c.md` | | | own |

`docs/v0.3.0/SPEC.md` and `CLAUDE.md`: orchestrator only. `apps/kira-space/frontend/package.json`:
B owns the file, but no stream changes its dependencies (Part 1 fixed them). A dep B leaves unused
goes in B's notes; the landing check removes it.

## No ordering dependency

The three couplings that forced Part 1 are frozen at `<P1>`:

1. Bindings. A changes no bound service's method set or signature, no bridge wire type, no event or
   menu-signal channel string. Check: regenerate bindings in A's worktree; file list and content
   match Part 1's snapshot. B builds against the `<P1>` bindings, which stay valid.
2. Paths. No stream moves a top-level directory. Every path a stream's config names exists at
   `<P1>` and stays.
3. Hook targets. Only C edits `package.json` and `hook-scope.sh`. A and B commit against the `<P1>`
   target set, which stays valid in their worktrees.

Further frozen contracts:

- Mode ids (`ade, automations, memory, git, studio, api, docker, kubernetes`) and ModeSwitcher DOM
  (`[data-testid=mode-tab][data-mode=<id>]`). C's Studio fixtures select a mode with it.
- Existing `data-testid` values. B keeps them. Where two merged components carry one testid with
  different meanings, B keeps Space's, renames Studio's, and lists it in B's notes; the landing check
  fixes the specs.
- Contract JSON. A's merged harness writes byte-identical output in `KIRA_CONTRACT` compare mode,
  including `tests/contract/studio/`.
- Claude test names. A keeps every `Test*` name from `tests/claude-studio/*_test.go`; C's
  `test:claude:db` script filters on them.

No stream reads another stream's output. Any order of landing works.

## Stream A: Go

1. One flow harness. Merge `studioflowharness` into `flowharness`: one `App` booting the merged
   app with both homes under temp dirs; Space git, acli, az, host fakes plus Studio HTTP/gRPC
   servers, docker and kube fakes; `flowharness/cmd/flowservers`; contract helper with a per-flow
   subdir (`""` or `"studio"`). Delete `studioflowharness`.
2. One flows tree. Move `studioflows/*` into `flows/`. Merge the seven name clashes (`agentflow`,
   `appflow`, `coverage`, `llmpromptsflow`, `promptflow`, `termflow`, `windowflow`) into one package
   each, deleting duplicated tests of now-merged services. `coverage` checks one bridge. Add the
   Studio tab-kind agreement check from Part 1. Delete `studioflows`. `layering_test.go` exemptions
   drop the `studio*` names.
3. One claude module. Merge `tests/claude-studio` into `tests/claude` (one `main_test.go`, one
   helpers file, Studio test files keep names and `Test*` names). Delete `tests/claude-studio`.
4. First-launch migration, new package `internal/appmerge`, called in `main.go` before
   `appwire.Build`. Marker row in Space DB. First a `VACUUM INTO` backup of both DBs to
   `<space home>/backups/p537-<timestamp>/`. Then, idempotent: re-key Studio tabs whose
   `window_key` has no Space window to Space's primary window, `order` appended; import Studio
   layout rows for Studio modes (copy only columns Space's layout table holds); when Space DB is
   fresh, copy Studio's `Appearance`, `Agent`, `Advanced.NotifyPrompts`, log level; merge Studio
   `llm-prompts.json` overrides for ids Space has not overridden. A half-done run resumes: each
   step checks its own end state. Backend flow test in `flows/appflow` (no UI half: no frontend
   surface).
5. Old Studio guard. Startup check for a process from `Kira Studio.app` (`metrics.AppProcessSet`,
   anchor `Kira Studio.app/Contents/MacOS`); on a hit, a `startupfail` alert asks to quit it, then
   exit. Darwin only; elsewhere a no-op.
6. Copy and names. About description names Kira Space and both module families; `dbmcp`
   instructions and `internal/llmprompts` texts naming Kira Studio say Kira Space (server name
   `kira-db` stays); `g1measure` anchor default `Kira Space`; `datagrip/keychain_other.go`
   messages; remove `appupdate.Studio`; comments naming `apps/kira-studio` in Go.
7. Menu. Part 1 concatenated; order it: one app menu, one Settings item, one Window menu, Studio
   query and view items after Space's. Signal channels unchanged.

Verification (fast only): `go build ./...`; `go build -tags server ./apps/kira-space/...`;
`go vet ./...`; `bun run lint:go`; bindings check (point 1 above); `go test` for packages A changed
or added (`internal/`, `appmerge`, `flowharness`, the merged `flows/*` packages); flows in contract
compare mode for each package touching contract output; `go vet` in `tests/claude`.

## Stream B: frontend

1. One entry. Space `src/main.ts` takes the global setup Studio's `main.ts` did (window hooks,
   Monaco env, workers, theme). Delete `src/studio/{main.ts,App.vue}`.
2. Modes. One mode type (`packages/shared/domain/mode.ts` `AppMode` absorbs `SpaceMode`;
   `state/modeDomain.ts` re-exports or goes). One `workbench/modes.ts`: `MODE_ORDER` per `P537.md`,
   `MODES` holds Space entries and Studio's panel entries, `automations` once. Delete Studio's
   `workbench/{modes,host,WorkbenchShell}`.
3. Stores, one per concern: mode, tabs (one store, `TAB_KINDS` union; Studio modes as workspaces;
   Studio's incognito indicator for its kinds), layout, keepAwake, appUpdate, appMetrics,
   terminals, terminalTabs, customScripts, pinia. Two settings stores stay (`settings` over
   `SettingsService`, `studioSettings` over `StudioSettingsService`): two concerns until P540.
   Operations dock: Space feed for Space modules, Studio feed (`StudioOpsService`) for Studio
   modules.
4. Bridge hub. One `src/bridge/index.ts` exporting merged services; Studio's `control`, `data`,
   `port`, `apiControl` stay under `src/studio/bridge/` and import shared pieces from the hub.
5. Settings dialog. One `SettingsShell`: Space panes, then Studio panes. App-wide keys come from
   Space settings only (`P537.md` table); Studio's duplicate panes for them go.
6. Windows and routing. One `App.vue` routes every window kind (Space review and opened-files
   windows, Studio proof windows, mobile pairing).
7. Shortcuts, palette commands, menu-signal listeners: merged; on a clash Space's binding wins and
   Studio's moves, listed in notes.
8. Copy. App-name strings saying Kira Studio in `packages/theme/src/i18n/en.json` say Kira Space;
   drop keys left unused (`check-catalog --unused`).
9. Unit tests. `tests/unit/studio/` follows moved or deleted code; delete specs of deleted code.

Verification (fast only): `bun run typecheck`; `bun run lint`; `bun run build:space` (desktop,
mobile); proto build; `bun test` for unit specs B changed or added; grep: no import of
`src/studio/{main,App}`; one `defineStore` per store id (`grep -rhoE "defineStore\('[^']+'"
| sort | uniq -d` empty).

## Stream C: build, packaging, CI, tests layout, docs

1. Taskfile and bundle. One app: `APP_NAME` "Kira Space", bundle id `com.kirathecat.kira-space`.
   Merge Studio's plist keys (document types, URL schemes, usage strings) from
   `aa8a899b:apps/kira-studio/build/darwin/Info*.plist` into Space's plists and `tools/gendoctypes`.
2. `package.json` final scripts: `dev`, `build`, `build:test`, `build:mobile`, `package`;
   typecheck targets without app suffixes; test scripts per merged config; `test:claude`
   (`apps/kira-space/tests/claude`) and `test:claude:db` (`-run` over the Studio claude test names,
   read from `aa8a899b`). `hook-scope.sh` matches. Old script names removed, not aliased.
3. Packaging scripts single-app: `verify-packaging.sh`, `sign-bundle.sh`, `release-on-merge.sh`.
4. `install.sh`. `--app=space` unchanged. `--app=studio`: install Kira Space, verify bundle id and
   signature, then move `/Applications/Kira Studio.app` to the Trash and report it; never delete
   `~/.kira-studio`. Contract marker and log path rules (`appupdate/install.go`) kept.
5. CI, as patches under `docs/pending-workflows/` (`.github/workflows/` cannot be pushed from here;
   `docs/DEV_ENVIRONMENT.md`): `pr.yml` builds once, UI shards over the merged config, docker cache
   key path `apps/kira-space/internal/adapters/testsupport/*.go`; `release.yml` one dmg, one
   `assert_app`, install smoke runs both `--app=space` and `--app=studio`; `cache-warm.yml`.
6. Playwright. One `playwright.config.ts` with every project of both old configs (space ui,
   studio ui at `tests/ui/studio`, ipc-frontend, ui-clipboard, ui-timing, visual, mobile,
   screenshots), one perf config, proto config kept. Both run against the merged test build.
   Studio fixtures select their mode through `[data-testid=mode-tab][data-mode=<id>]` before a spec,
   unless the spec opens a window kind directly. e2e-real fixtures launch the one binary. Delete
   `playwright.studio*.config.ts`.
7. Docs per `P537.md` Docs section; `docs/PACKAGING.md` §1 and §8 become one app; root
   `README.md` normal prose; `apps/kira-space/README.md` absorbs Studio's README from `aa8a899b`;
   `site/` pages; `docs/ARCHITECTURE.md` Known open item for two homes and two DBs.
8. `biome.json`, `knip.json` leftovers.

Verification (fast only): `bun run typecheck`; `bun run lint` (includes `site` docs check);
`bunx playwright test --list` per config; `sh -n` on each changed shell script;
`bash scripts/verify-packaging.sh` off-macOS checks; `bun run site:build`; `git grep -l
apps/kira-studio` over C-owned paths lists only `docs/HISTORY.md` and `docs/v0.2.36/`.

## Landing

1. Each stream is implemented and passes its own verification in its worktree. The orchestrator
   checks each against this plan before landing it.
2. Land in order A, B, C (any order works; this one puts Go first). Per stream:
   `git -C ../kira-p537-<s> rebase chapter/v0.3.0`, then in the main checkout
   `git merge --ff-only p537-stream-<s>`. A conflict means an ownership boundary was wrong: stop and
   report, never resolve by editing the other stream's file.
3. Landing check, one Sonnet pass, fixes committed in the chapter branch: `bun run setup`; every
   fast check of all three streams on the merged tree; `bun run lint:dead`; bindings match Part 1's
   snapshot; apply each notes file's follow-ups (unused deps, renamed testids, moved shortcuts);
   `git grep -l apps/kira-studio` lists only `docs/HISTORY.md`, `docs/v0.2.36/`, `CLAUDE.md`.
4. `git worktree remove ../kira-p537-{a,b,c}`; `git branch -D p537-stream-{a,b,c}`; push
   `chapter/v0.3.0`.

UI, visual, e2e-real and full flow suites run once at chapter end. Studio visual baselines will
change (merged chrome and module bar); re-record them there.

## Result

Per stream: commits, verification output (one decisive line each), deviations with reasons.
