# Studio twins after P537

Audit only. Base `4308ff61` (P537 Part 2 done). Lists every Studio-only copy that still duplicates a
Space counterpart, so P539 (home fold), P540 (config subfolder) and P541 (DB merge) can finish the
merge. Discovery: `codegraph_explore` (bridge twins, `appcore`, `config` paths, `appwire`
`buildStudio`/`wireStudio*`, layout repos, `configsvc`, ops panels, tab kinds, Monaco hosts), then
shell counts for migrations, contracts, test dirs and catalog the index does not cover.

Phase numbers renumbered after this audit: MERGE-NOW items are P538; home fold P539, config subfolder P540, DB merge P541 (item ids follow).

Verdicts: MERGE-NOW (no data dependency, target named), BLOCKED-BY P539 / P540 / P541 (reason named).
Counts: MERGE-NOW 9, BLOCKED-BY P539 7, BLOCKED-BY P540 1, BLOCKED-BY P541 6. Not twins: last section.

## MERGE-NOW

### M1 Studio layout repo is dead; Space layout drops the cell editor height

- Paths: `internal/studiostorage/repos/layout.go` (+ `layout_test.go`), `internal/studiostorage/model/layout.go`, `repos.go` field `Layout`.
- Counterpart: `internal/storage/repos/layout.go`, `internal/storage/model/layout.go` (the bound `LayoutService` uses only these).
- Why separate: left over from Part 1. No production caller; `appmerge.importLayout` reads `ui_layout` by raw SQL.
- Bug found: frontend `state/layout.ts` `setCellEditorHeight` patches `panel.cellEditor.height`. Space `model.Layout` has no `CellEditor`, so the height never persists since P537.
- Target: delete the Studio repo and model types. Add `PanelCellEditor` to Space `model.Layout`/`LayoutPatch`/`decodeLayout`. Studio `ui_layout` table stays until P541; P541 copies the old `panel.cellEditor.height` row.

### M2 Stale Studio contract fixtures

- Paths: `tests/contract/studio/{link-open,llm-prompts,prompts-route,runs-file,runs-form,runs-lifecycle,runs-restore,schedule-confirm,schedule-now,schedule-overlap,script-order,smart-resume,terminal-link}.json` (13).
- Counterpart: same-name files in `tests/contract/`, written by Space flow tests (`termflow/runs_*_test.go`, `schedule_test.go`, `automation_test.go`, `smart_resume_test.go`, `promptflow`, `llmpromptsflow`, `appflow/misc_test.go`).
- Why separate: Studio flow tests for these merged services were deleted in Stream A; the JSON stayed. No file calling `StudioContract` names them. `link-open` has no reader at all.
- Readers to repoint: `tests/ui/studio/automations-{file,form,lifecycle,mcp}.spec.ts`, `running-ops-status.spec.ts`, `ui-leftovers-docker-automations.spec.ts`, `support/mockRuntime.ts` read `runs-file`, `runs-form`, `schedule-confirm`, `script-order` from the Studio dir.
- Target: delete; readers use `tests/ui/support/contract.ts` (Space dir).

### M3 Contract dir split

- Paths: `flowharness/contract.go` `StudioContract`, `StudioContractEvent`, `contractDir("studio")`; `tests/contract/studio/` (74 files, 61 after M2); `tests/ui/studio/support/contract.ts`.
- Counterpart: `Contract`, `ContractEvent`, `tests/contract/`, `tests/ui/support/contract.ts`.
- Why separate: Part 1 kept two dirs to avoid scenario-name collisions.
- Live collisions after M2: `agent`, `automations`, `automations-mcp-app`, `quit-confirm`, `smart`, `tabs-scope`. `quit-confirm` goes with M4. `automations-mcp-app` and `smart` belong to P539 tests, `tabs-scope` to P541 tests; rename those three now (for example `-db` suffix) or fold them in their phase.
- Target: one `Contract` helper, one `tests/contract/` dir, one UI `contract.ts`.

### M4 Go flow tests duplicating merged services

- Paths: `flows/termflow/studioshell_test.go` (`TestShellTab`, `TestOpenRefusals`, `TestWindowCloseKillsOnlyItsShells`, `TestQuitTearsDownTerminals`, `TestLargeOutput`), `studioterminalid_test.go`, `studioclaudemissing_test.go`, `studiohelpers_test.go`; `flows/windowflow/studioquit_test.go`.
- Counterpart: `termflow/terminal_test.go`, `appflow/quit_test.go` (Terminal and quit are one service since Part 1).
- Why separate: moved verbatim with the Studio flows tree.
- Target: fold unique cases into `termflow/terminal_test.go` and `appflow/quit_test.go`; drop cases both cover; contract `quit-confirm` becomes one.

### M5 UI spec twins for merged surfaces

- Paths: `tests/ui/studio/{approvals,prompts,quit-confirm,terminal-links,update-dialog,settings-llm-prompts,panel-resize,focus-ring,scrollbars,list-keyboard,consistency,copy-coverage,copy-wording,color-rails,ui-leftovers}.spec.ts` (15).
- Counterpart: same names in `tests/ui/`.
- Why separate: Studio suite moved whole; each service or sweep behind them is now one.
- Target: one spec per name in `tests/ui/`; Studio-only cases (DB approval bodies, Studio module chrome) move in as cases.

### M6 UI harness twins and split Playwright projects

- Paths: `tests/ui/studio/fixtures.ts`, `tests/ui/studio/support/{mockRuntime,bootSnapshots,clock,ipcChannels}.ts`; projects `ui-studio`, `visual-studio`, `screenshots-studio` in `playwright.config.ts` (`ui-clipboard`, `ui-timing` point at `tests/ui/studio`).
- Counterpart: `tests/ui/fixtures.ts`, `tests/ui/support/*`; projects `ui`, `visual`, `screenshots`.
- Why separate: two mock runtimes and boot snapshots from two apps; the projects exist only because the fixtures differ.
- Target: one mock runtime and fixture; Studio service mocks become entries in it (the `StudioSettings*`/`StudioOps*`/`StudioConfigFiles*` entries stay until P539/P541). Then one project per tier; `test:ui`, `test:visual`, `screenshots` scripts in root `package.json` drop the `-studio` projects.

### M7 e2e-real spec twins

- Paths: `tests/e2e-real/studio/prompts-two-windows-real.spec.ts` (byte-identical), `terminal-real.spec.ts` (same test, adds temp HOME), `multiwindow-real.spec.ts` (different scenario: tabs per window).
- Counterpart: same names in `tests/e2e-real/`.
- Target: delete the identical one, merge the temp-HOME setup into Space's `terminal-real`, move the multiwindow tab case into Space's `multiwindow-real`. The fixture twin is P539 (B539-1).

### M8 Perf probe twins

- Paths: `tests/perf/studio/{heap-ram,heap-tabs}.spec.ts`, `tests/perf/studio/support/ramWorlds.ts`.
- Counterpart: `tests/perf/{heap-ram,heap-tabs}.spec.ts`, `tests/perf/support/ramWorlds.ts`.
- Why separate: same probe (`@workbench/testing/ui/heapProbe`), different worlds (repo vs grid tabs).
- Target: one spec per probe, worlds from one `ramWorlds.ts`.

### M9 AGENTS.md header says "Kira Studio"

- Paths: `internal/connfiles/docs.go:19`, `internal/apifiles/docs.go:24-26` (`App: "Studio"`); rendered by `internal/configfs/docs.go:258` as "Generated by Kira Studio".
- Counterpart: every Space folder doc uses `App: "Space"`.
- Target: `"Space"`. Header text only; no layout change.

## BLOCKED-BY P539 (home fold)

### B539-1 Two homes in path config and test harnesses

- Paths: `internal/config/paths.go` (`StudioHome`, `StudioDbPath`, `StudioDbPathAt`, `EnsureLayout` creating both homes, `KIRA_HOME`, `.kira-studio`); `internal/testx/apphome.go` `RunWithTempHomes`; `flowharness/harness.go` `KiraHome` field, `KIRA_HOME` setenv, `<kira>` replace in `contract.go:55`; `ipcfixture/harness.go`; `tests/e2e-real/fixtures.ts:84` (`KIRA_HOME`); `tests/e2e-real/studio/fixtures.ts` (`kiraHome` fixture, twin of `tests/e2e-real/fixtures.ts`); Go unit tests setting `KIRA_HOME` (`studiooplog`, `maskrules`, `connections`, `tree`, `apivars`, `bridge/http_test.go`, `bridge/grpc_test.go`, `gitsession/incremental_test.go`, `studiostorage/*_test.go`); comments in `internal/logging/log.go`, `internal/startupfail/step.go`, `internal/mcpauth/token.go`, `internal/harness/claude/mcpinstall/install.go`.
- Counterpart: `KiraSpaceHome`, `KIRA_SPACE_HOME`, `flowharness.SpaceHome`, Space e2e-real fixture.
- Why blocked: user data lives in `~/.kira-studio`; collapsing the resolver before the move loses it.

### B539-2 Second runs stack (custom scripts, script runs)

- Paths: `appwire/studio_wire.go:145-175` (second `scriptruns.Service`, `App: "Studio"`; second `scripts.Loader` on `<studio home>/runs`, no `SpaceKeys`); `bridge/customscripts.go` (`Studio *scripts.Service`, `owner`, `Publish(studio bool)`, `mergeSnapshots`, split `Reorder`); `bridge/scriptruns.go` (`Studio *scriptruns.Bound`, owner routing); `appwire/studio_build.go:41-42`.
- Tests: `flows/termflow/studio{context,settings,hermetic,smart,script}_test.go`; contracts `tests/contract/studio/{automations-mcp-app,smart}.json`; `tests/ui/studio/automations-{file,form,lifecycle,ops,recurring,restore,resume,runs,secrets,smart,tabs}.spec.ts` (twins of `tests/ui/automations-*`).
- Counterpart: Space runs stack in `appwire.go:480,543`.
- Why blocked: two `runs/` folders, one per home; the router is deleted when they merge (P537 plan).
- Data caveat for the P539 plan: the Studio DB also holds `custom_scripts`, `custom_script_collections`, `script_runs`, `script_run_logs`, `secret_values`, `config_last_valid` rows for that stack. P539 either copies them into the Space DB with the folder, or the router stays until P541. Decide in plan.

### B539-3 `StudioConfigFilesService`

- Paths: `bridge/studio_configfiles.go`; `appwire/studio_build.go:173` (folders `runs`, `api`, `connections`); `appwire.go:246`; binding `studioconfigfilesservice.js`; overrides `configFilesOpen`, `configFilesReveal` in `frontend/src/studio/bridge/control.ts`; callers `studio/api/CollectionsTree.vue`, `ApiConfigProblems.vue`, connection dialogs.
- Counterpart: `bridge/configfiles.go` `ConfigFilesService` (`appwire.go:566`: `workflows`, `steps`, `agent`, `runs`, repos).
- Why blocked: folder key `runs` names a different dir per home. `api` and `connections` do not collide.
- Target after P539: register `api` and `connections` in `ConfigFilesService`; delete the twin, its binding and both control overrides.

### B539-4 DB MCP token and helper under Studio home

- Paths: `internal/dbmcp/server.go:130,224` (`cfg.Home = config.StudioHome()`); `bridge/dbmcp.go:235,324` (`mcpauth.HelperTokenPathNamed`/`PathNamed` on `config.StudioHome()`).
- Counterpart: Space home tokens (agent port, memory).
- Why blocked: token files and helper script move with the home; agent registrations need rewrite (P539 scope).

### B539-5 `AppService.Info` reports the Studio home

- Path: `bridge/app.go:46,54` (`Deps appcore.StudioDeps`, `KiraHome: config.StudioHome()`); test `flows/appflow/studioapp_test.go` `TestAppInfo`.
- Target: one home field from the Space home; `TestAppInfo` moves to `appflow`.

### B539-6 Studio DB file and config dirs located under Studio home

- Paths: `internal/studiostorage/db.go:31` (`OpenAt(config.StudioHome())`); `appwire/studio_build.go:58-80` (`connections/`, `api/` under `studioHome`).
- Why blocked: P539 moves the files into the Space home. The DB stays a second file until P541; dirs reach the subfolder in P540.

### B539-7 First-launch carry-over reads the Studio home

- Paths: `internal/appmerge/appmerge.go` (`Options.StudioHome`, `hasStudioData`, `mergePrompts` on `<studio home>/llm-prompts.json`); `main.go:309` `carryOverStudio`.
- Why blocked: P539's fold owns home-level files (`llm-prompts.json`); the DB parts of `appmerge` go with P541 (B541-6).

## BLOCKED-BY P540 (config subfolder)

### B540-1 Two config-folder registries and inline folder paths

- Paths: `appwire.go:566` (`ConfigFilesService` folder map, `adeflow.Dir`, `adeflow.StepsDir`, `filepath.Join(deps.Home, "agent")`, `filepath.Join(deps.Home, "runs")`), `appwire.go:543,795`; `appwire/studio_build.go:60,80,168,173`.
- Why blocked: after B539-3 one registry remains, but each module still joins its own folder onto the home. P540 moves every module under one subfolder; one resolver for `<home>/<subfolder>/<module>` removes the scattered joins.

## BLOCKED-BY P541 (DB merge)

### B541-1 Second storage package and migration sequence

- Paths: `internal/studiostorage/` (`db.go`, 63 migrations, `model/`, `repos/`).
- Counterpart: `internal/storage/` (72 migrations).
- Live table names in both DBs: `config_last_valid`, `custom_script_collections`, `custom_scripts`, `op_log`, `script_run_logs`, `script_runs`, `secret_values`, `settings`, `tabs`, `ui_layout`, `windows`.
- Colliding model files: `studiostorage/model/{settings,tabs,window,ops}.go` (`layout.go` goes in M1). The rest of `studiostorage/model` is the database-module domain model imported by `adapters/*`, `adapterhost`, `dbmcp`, `tree`, `connections`; P541 moves or renames it so no `studiostorage` import remains.

### B541-2 `StudioDeps` and the second build stage

- Paths: `appcore/deps.go` `StudioDeps`; `appwire/appwire.go` `Options.StudioDB`, `Options.StudioRepos`, Teardown closing two DBs; `appwire/studio_build.go` `buildStudio`, `studio_wire.go` `studioAdapters`/`studioEmbedded`; `main.go` second `studiostorage.Open`; `flowharness/harness.go` `studioDB`, `StudioDB()` (used by 22 flow test files); `ipcfixture/harness.go` (hand-wires a Studio stack on `StudioDeps` and `StudioOpsService`).
- Counterpart: `appcore.Deps`, `Wired.Build`.
- Why blocked: `StudioDeps.DB`/`Repos` point at the second DB.

### B541-3 Settings twin

- Go: `bridge/studio_settings.go` (`StudioSettingsService`, `StudioSettingsSetArgs`); `bridge/studio_events.go` `StudioSettingsChanged`, `ChannelStudioSettingsChanged` (`studio:kira:settings:changed`); `studiorepos.SettingsRepo`, `studiomodel.Settings` (still carries the unread app-wide keys); `appwire.go:384` prompts `RaiseOnOpen` reads `opts.StudioRepos.Settings`.
- Frontend: `studio/state/settings.ts` (`useStudioSettingsStore`, id `studioSettings`), `studio/state/settingsDomain.ts`; `workbench/SettingsDialog.vue` draft split and `studioAdvanced` leaves; `workbench/settings/types.ts`; `studio/workbench/settings/StudioAdvancedFields.vue` inside `AdvancedPane.vue`; panes `ApiPane`, `CachePane`, `DataPane`, `DatabaseMcpPane`, `ScriptsMcpSection` bound to the Studio store; `packages/workbench/src/state/createSettingsStore.ts` branch for a store without appearance; overrides `settingsGetAll`, `settingsSet`, `onSettingsChanged` in `studio/bridge/control.ts`; `CHANNEL.studioSettingsChanged` in `packages/shared/protocol/events.ts`; binding `studiosettingsservice.js`.
- Tests: `tests/visual/studio/settings.spec.ts` (twin of `tests/visual/settings.spec.ts`).
- Counterpart: `SettingsService`, `useSettingsStore`, `state/settingsDomain.ts`.
- Why blocked: two `settings` tables, two models.

### B541-4 Op log twin

- Go: `internal/studiooplog/`; `studiorepos.OpsRepo`; `bridge/studio_ops.go` (`StudioOpsService`, `StudioOpsRecentArgs`, `StudioOpsCancelArgs`, `StudioCanceller`); `StudioSources.Oplog`, `ChannelStudioOpUpdate` (`studio:kira:op:update`); Studio setting `Advanced.OpLogRetentionDays`.
- Frontend: `studio/state/ops.ts` (`useStudioOpsStore`, `storeId: 'studioOpLog'`); `studio/workbench/panels/OperationsPanel.vue`; `workbench/OperationsDock.vue` feed switch on `STUDIO_MODES`; `@shared/domain/ops` `OpRecord` vs `state/opsDomain.ts` `SpaceOpRecord`; `createOpLogStore` `storeId` option; `createCoreControl` `channels` arg; overrides `opsRecent`, `opsCancel`, `onOpUpdate`; `CHANNEL.studioOpUpdate`; binding `studioopsservice.js`.
- Tests: `tests/ui/studio/{operations,running-ops-status}.spec.ts`, `tests/visual/studio/operations.spec.ts` (twins of the Space specs).
- Counterpart: `internal/oplog`, `storage/repos/oplog.go`, `OpsService`, `useOpsStore`, `workbench/OperationsPanel.vue`.
- Why blocked: two `op_log` tables with different schemas; P541 decides one table (P537 plan).

### B541-5 Tabs and windows split across DBs

- Paths: `bridge/tabs.go` (`Studio appcore.StudioDeps`, routing by `studiomodel.IsStudioTabKind`, `Windows.EnsureExists` for the FK); `studiorepos.{TabsRepo,WindowsRepo}`; `internal/appstorage` `ListWindowTabs`/`SaveWindowTabs` called over both repo sets.
- Tests: `flows/windowflow/studio{tabkinds,tabsscope,windows}_test.go`, contract `tests/contract/studio/tabs-scope.json`; `tests/unit/studio/go-ts-vocabulary-parity.spec.ts` (twin of `tests/unit/go-ts-vocabulary-parity.spec.ts`).
- Why blocked: Studio tabs keep `connection_id` and its FK in the Studio DB.

### B541-6 `appmerge` first-launch import

- Paths: `internal/appmerge/` (re-key tabs, layout rows, settings, marker `appmerge.p537`), `flows/appflow/appmerge_test.go`.
- Why blocked: interim copy between two DBs; P541's data migration replaces or retires it.

## Not twins (checked)

- `bridge/studio_stream.go` (engine stream), `studio_events.go` connection channels, `src/studio/` module code, `studio.*` catalog keys (7 string overlaps, all frontend vs backend namespaces), `STUDIO_TAB_KINDS` (already merged into `TAB_KINDS`), approval kind registries (one `provideApprovalKinds` call), Monaco (one shared `loadMonaco`; `MonacoHost.vue` is the database-module host), `studio/kubernetes/tabs.ts` (Kubernetes-internal tabs, no Space counterpart), `startupfail.StepOldStudio`/`oldstudio_*.go` (old-bundle guard), keychain service `Kira Studio Secrets` (keys existing secrets), `tests/claude` (already one module), `appflow/studioerrors_test.go` and `studioapp_test.go` `TestDataGripScan` (database-module tests; file rename optional).
- Shared repo-root packages with an `App` field (`scripts`, `scriptruns`, `agentctx`, `agentcfg`, `configfs`, `prompts`): one value after B539-2 and M9; dropping the field is optional.

## Acceptance checklist (git-grep-able end state)

Run from repo root. Each command must print nothing, unless an allowance is named.

P539:

- `git grep -nE 'StudioHome|StudioDbPath|KIRA_HOME|\.kira-studio' -- apps internal ':!**/testdata/**'` prints only the one-time fold code and `internal/secrets/cipher.go` history comment.
- `git grep -n 'App: *"Studio"' -- '*.go'`
- `git grep -nE 'Studio +\*script(s|runs)\.' apps/kira-space/internal/bridge`
- `git grep -nE 'StudioConfigFiles|studioconfigfilesservice'`
- `git grep -n 'kiraHome\|KiraHome' apps/kira-space/internal/flowharness apps/kira-space/tests/e2e-real`
- `ls apps/kira-space/internal/flows/termflow/studio*_test.go` lists none of `studio{context,settings,hermetic,smart,script}_test.go`.
- `ls apps/kira-space/tests/ui/studio/automations-*.spec.ts` lists nothing.
- `git grep -n 'configFilesOpen\|configFilesReveal' apps/kira-space/frontend/src/studio/bridge/control.ts`

P540:

- `git grep -nE 'filepath\.Join\([^)]*(Home|home)[^)]*"(connections|api|runs|workflows|steps|agent)"' apps/kira-space/internal` prints nothing outside the one config-root resolver.

P541:

- `git grep -nE 'studiostorage|studiorepos|studiomodel|studiooplog'`
- `git grep -nE 'StudioDeps|StudioOps|StudioSettings|StudioCanceller|StudioSources|StudioDB|StudioRepos|buildStudio|IsStudioTabKind|studioAdapters|studioEmbedded'`
- `git grep -nE 'studio:kira:|studioSettingsChanged|studioOpUpdate'`
- `git grep -nE "useStudioSettingsStore|useStudioOpsStore|'studioSettings'|'studioOpLog'|studioAdvanced|StudioOperationsPanel|StudioAdvancedFields|STUDIO_MODES"`
- `git grep -n 'storeId' packages/workbench/src/state/createOpLogStore.ts`
- `ls apps/kira-space/frontend/bindings/github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge | grep -i studio`
- `ls apps/kira-space/internal/studiostorage apps/kira-space/internal/appmerge apps/kira-space/tests/unit/studio/go-ts-vocabulary-parity.spec.ts apps/kira-space/tests/visual/studio/operations.spec.ts apps/kira-space/tests/visual/studio/settings.spec.ts` all fail (removed or folded).
- `ls apps/kira-space/internal/bridge/studio_{ops,settings,configfiles}.go` all fail.

MERGE-NOW items (P538; SPEC row carries the allowances for twins later phases own):

- `ls apps/kira-space/internal/studiostorage/repos/layout.go` fails; `git grep -n CellEditor apps/kira-space/internal/storage/model/layout.go` matches.
- `git grep -nE 'StudioContract|contract/studio'`
- `ls apps/kira-space/tests/contract/studio` fails.
- `ls apps/kira-space/internal/flows/termflow/studio{shell,terminalid,claudemissing,helpers}_test.go apps/kira-space/internal/flows/windowflow/studioquit_test.go` all fail.
- `comm -12 <(ls apps/kira-space/tests/ui) <(ls apps/kira-space/tests/ui/studio)` prints nothing; same for `tests/e2e-real`, `tests/perf`, `tests/visual` (visual allows the two P541 twins until P541).
- `git grep -nE "'(ui|visual|screenshots|e2e-real)-studio'" apps/kira-space/playwright.config.ts package.json` (e2e-real allowed until B539-1).
- `git grep -n 'App, m.Title.*"Studio"\|c.App.*"Studio"\|App: "Studio"' apps/kira-space/internal/connfiles apps/kira-space/internal/apifiles`
