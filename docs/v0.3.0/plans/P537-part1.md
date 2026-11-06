# P537 Part 1: Relocate Studio into apps/kira-space and freeze the merged bridge

Decisions: `P537.md`. This Part is the enabling step for Part 2's three streams. One sequential
Sonnet implementer. Base: `aa8a899b`.

Goal at the end: `apps/kira-studio` gone; every Studio file at its final path; one Go binary that
binds every service of both apps; one bindings tree; Studio frontend source typechecks inside
Space's frontend project (not mounted yet); every dev, test and hook path rewired; fast checks
green; rename table written below.

Not here (Part 2): mounting Studio modules in the UI, flow harness merge, first-launch migration,
packaging, CI, docs, playwright config merge.

## Steps

Commit after each step (`refactor:` or `chore:`). Go may not build between steps 1 and 4; every
commit still passes the pre-commit hook (it checks TS and lint only). Step 4's commit is the first
with green Go again.

### 1. Move Go packages

- `git mv` each Studio package with no Space twin to `apps/kira-space/internal/<same name>` (list in
  `P537.md` Target layout).
- `git mv` Studio twins: `storage` to `internal/studiostorage`, `oplog` to `internal/studiooplog`,
  `flowharness` to `internal/studioflowharness`, `flows` to `internal/studioflows`.
- `git mv` Studio `appcore`, `appshell`, `appwire`, `bridge`, `config`, `buildinfo` to
  `apps/kira-space/internal/studiotmp/<name>`. Steps 2 to 4 fold each into its Space package and
  delete `studiotmp`.
- `git mv apps/kira-studio/cmd/g1measure apps/kira-space/cmd/g1measure`.
- `git mv apps/kira-studio/tests/claude apps/kira-space/tests/claude-studio`; module path becomes
  `github.com/kirathecat/kira-studio/apps/kira-space/tests/claude-studio` (the `replace ../../../..`
  depth is unchanged). Stream A merges it into `tests/claude`.
- Rewrite imports in `*.go` (sed, renamed paths first, then the generic
  `apps/kira-studio/internal/` to `apps/kira-space/internal/`), then `gofmt -w`.
- Move `apps/kira-studio/main.go`, `notify_darwin.go`, `notify_other.go` to
  `apps/kira-space/internal/studiotmp/mainsrc/` as reference input for step 4 (not compiled: add
  `//go:build ignore`). Step 4 deletes it.

### 2. Fold `config`, `buildinfo`, `appcore`

- `config`: add `StudioHome()` (was `KiraHome`, still `kirapaths.Home("KIRA_HOME", ".kira-studio")`),
  `StudioDbPath`, `StudioDbPathAt`. `EnsureLayout` creates both homes. Logs: Space `LogsDir` only.
  Rewrite every Studio caller (`codegraph_explore KiraHome` lists 38 callers plus tests).
- `buildinfo`: keep Space's; delete Studio's; rewrite imports.
- `appcore`: add Studio struct as `StudioDeps` beside `Deps`, import aliases for the two `repos`
  packages. Rewrite Studio services' embed to `appcore.StudioDeps`.

### 3. Fold `bridge`

- Move Studio bridge files into `apps/kira-space/internal/bridge`. Twin files that collide get a
  `studio_` prefix (`studio_settings.go`).
- Apply the bridge table in `P537.md`:
  - Merged services (Agent, Approvals, Prompts, Files, KeepAwake, Layout, Lifecycle, Link,
    Windows, Update, Terminal, LlmPrompts, Tabs): one struct each. Union of methods; on a method name
    clash with different signatures, keep Space's name and give Studio's a descriptive new name.
  - Tabs: `TabsService` routes by kind. Studio tab kinds are the kinds Studio's frontend registers
    (`apps/kira-studio/frontend/src/state/tabKinds.ts` at base); hold the Go set in
    `studiostorage/model` with one flow-level check in Stream A that the two lists agree. Studio
    records go to `studiostorage/repos.TabsRepo` (keeps `connection_id`), the rest to Space's. `List`
    returns both, ordered by `order`.
  - CustomScripts, ScriptRuns: one bound service each over a Go router of both homes' stacks (list
    union; route by script id; create in Space home). Both schedulers keep running, each over its
    own scripts.
  - Twins: `StudioConfigFilesService`, `StudioOpsService`, `StudioSettingsService`.
- `events.go`: one `Events`. Channel strings that collide with different payloads get a `studio:`
  prefix on the Studio side.
- Delete `studiotmp/bridge`.

### 4. Fold `appwire`, `appshell`, `main.go`

- `appwire`: one `Options`, one `Build`, one `Wired`. Studio's build moves into `buildStudio`
  called from `Build` after the Space services, over Studio's own DB and repos (`studiostorage.Open`
  at `config.StudioDbPath()`). Process-wide singletons built once, shared: approvals queue, prompt
  router, keep-awake controller, window registry, close-flush coordinator, quitter, PTY registry,
  updater (`appupdate.Space`), harness registry and selector (Space settings' `Agent.Provider`),
  `llmprompts` store (Space home). One teardown: Studio's order inside it is kept (DB MCP stops
  before `connectionsSvc.Shutdown`).
- `appshell`: one menu. Space items, then Studio-only items; shared roles once. Same signal
  channel strings as today.
- `apps/kira-space/main.go`: one boot. Space's `RunArgvShim`, instance lock and startup reporter
  first; Studio's cipher and `localauth` authorizer from `openCore` join Space's boot; one
  `application.New` with `w.Bound()` plus both apps' notify services; Studio proof-window purge
  next to Space's review-window purge.
- `studioflowharness` compiles against the merged `Build`. `layering_test.go`: one file, union of
  exemptions (`ipcfixture`, `studioflowharness`, `studioflows`).
- Delete `studiotmp`. Then: `go build ./...`, `go vet ./...`, `bun run setup` (bindings).

### 5. Move the Studio frontend

- `git mv apps/kira-studio/frontend/src apps/kira-space/frontend/src/studio`;
  `frontend/proto` to `apps/kira-space/frontend/proto`; `vite.proto.config.ts`,
  `tsconfig.proto.json` to `apps/kira-space/frontend/`. Fix their relative paths.
- Studio `index.html`, `components.json`, `wails/`, `vite.config.ts`, `tsconfig.json`: delete
  (Space's own copies serve; check `wails/runtime.js` has no unique content first).
- Merge Studio frontend `package.json` deps into `apps/kira-space/frontend/package.json`; delete the
  Studio one; `bun install` (updates `bun.lock`). This is the final dep set: Part 2 Stream B adds or
  removes no dependency.
- Space `tsconfig.json` already includes `src/**`, so `src/studio/**` typechecks there. Fix:
  `@bindings` call sites for renamed services and channels (`src/studio/bridge/*`,
  `@bindings-internal/storage/model` to `@bindings-internal/studiostorage/model`), clashing
  `declare global` blocks (`src/studio/main.ts`, `kiraSlickGrid.ts`) against Space and
  `packages/workbench/env.d.ts`.

### 6. Move Studio tests and playwright configs

- `git mv` each Studio suite to `apps/kira-space/tests/<suite>/studio/` (contract, e2e-real,
  fixtures, perf, screenshots, ui, unit, visual); `ipc`, `proto`, `support` to
  `apps/kira-space/tests/<suite>/`. Rewrite relative imports (unit specs reach
  `frontend/src/studio/...`).
- Playwright: `apps/kira-studio/playwright.config.ts` to `apps/kira-space/playwright.studio.config.ts`,
  `playwright.perf.config.ts` to `playwright.studio-perf.config.ts`, `playwright.proto.config.ts`
  to `apps/kira-space/playwright.proto.config.ts`; fix `testDir` and paths so `--list` works.
  Stream C merges them.
- `apps/kira-space/tsconfig.tests.json` includes the moved suites (union of Studio's includes);
  `tests/unit/tsconfig.json` includes `studio/**`. Delete Studio's tsconfigs.
- Contract dir: `studioflowharness` `contractDir` and `tests/ui/studio/support/contract.ts` point
  at `tests/contract/studio/`.

### 7. Rewire tooling, delete `apps/kira-studio`

- Root `package.json`: typecheck targets point at the final paths (studio web and tests targets
  dropped, proto points at `apps/kira-space/frontend/tsconfig.proto.json`); `build:studio`,
  `dev:studio`, `package:studio`, `build:test:studio` removed; scripts that ran Studio specs point at
  the moved configs; `test:claude:studio` points at `tests/claude-studio`. Stream C renames the rest.
- `scripts/setup.sh`, `prepare-worktree.sh`, `lib.sh`: one app dir for bindings.
  `hook-scope.sh`: Studio targets removed, paths under `apps/kira-space`.
- Every other dev, lint and test script naming `apps/kira-studio`: `check-*.sh`, `check-*.ts`,
  `commentsweep`, `copy`, `mutation`, `generate-wire.sh`, `test-matrix.sh`, `test-ui-changed.sh`,
  `go-test-split.sh`, `db-compat.sh`, `demo-dbs`, `pull-test-images.sh`; plus `tools/mutation`,
  `biome.json`, `knip.json`, root `tsconfig.json`, `.golangci.yml`, `apps/kira-space/tsconfig.json`.
- Left for Stream C on purpose: `scripts/verify-packaging.sh`, `scripts/sign-bundle.sh`,
  `scripts/install.sh`, `scripts/release-on-merge.sh`, `.github/`, `docs/`, `site/`, READMEs.
- `git rm -r apps/kira-studio` (Taskfile, `build/`, README, leftovers). Stream C reads plist keys and
  README text from `aa8a899b`.

## Verification (orchestrator re-runs before accepting)

- `go build ./...`; `go build -tags server ./apps/kira-space/...`; `go vet ./...`; `bun run lint:go`.
- `bun run setup` in a fresh worktree (`sh scripts/prepare-worktree.sh`) generates one bindings tree,
  no Studio app dir.
- `bun run typecheck`; `bun run lint`; `bun run lint:dead`; `bun run build:space` (desktop and
  mobile); proto build.
- `go test ./apps/kira-space/internal/ ./apps/kira-space/internal/bridge/... ./apps/kira-space/internal/appwire/...`.
- `bunx playwright test --list` for each config under `apps/kira-space`.
- `git grep -l apps/kira-studio` lists only `docs/`, `site/`, `.github/`, `README.md`,
  `CLAUDE.md`, `scripts/verify-packaging.sh`, `scripts/sign-bundle.sh`, and Go or TS comments
  Stream A or B owns.
- Bound service count equals Space's bound count plus Studio's minus the 15 merged ones (twins
  renamed, not dropped). Base has 38 and 39 `*Service` structs, so 62 if every one is bound; count
  `w.Bound()` entries at base and after, and reconcile any difference in Result.
- Snapshot the bindings tree: `tar` of `apps/kira-space/frontend/bindings` into the scratchpad, and
  write its file list hash below. Stream A's bindings check compares against it.

## Result

Write here before Part 2 starts: final service rename table (Studio name, merged or twin name),
renamed bridge types, renamed event channel strings, the Studio tab-kind set, the bindings file-list
hash, and any decision taken differently from `P537.md` with its reason.
