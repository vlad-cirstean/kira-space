# v0.2.28

Branch `chapter/v0.2.28`. First `P` is 472 (highest in v0.2.27 is P471). Open.

Source: user request. Phases run top to bottom, two phases in parallel where file ownership allows (worktree per stream). Scope is closed: any further phases go to v0.2.29. Ends with two code review rounds, then close, then PR.

| Phase | Title | Status |
|---|---|---|
| P472 | Runs config as files in the app folder | Done |
| P473 | API config as files in the app folder | Done |
| P474 | Git repos config as files in the app folder | Done |
| P475 | Connections config as files in the app folder | Done |
| P476 | Git on bound services: connection lifecycle, events, read methods | Done |
| P477 | Git on bound services: write operations | Done |
| P478 | Git on bound services: review, change requests, ADE and repo consumers | Done |
| P479 | Git graph data plane; remove the git stream protocol | Done |
| P480 | Frontend RAM measurement, both apps | Done |
| P481 | Frontend RAM: implement the proven wins | Todo |
| P482 | Code review (2 rounds) | Todo |
| P483 | Close chapter v0.2.28 | Todo |

## Rows

### P472 Runs config as files in the app folder

- Applies the P450 file-config foundation (v0.2.26) to Runs. Data config moves to plain files in a Runs folder in the app folder, with an `AGENTS.md` listing every option and how to update it. Runtime and derived state (run records, history, progress) stays in SQLite.
- Plan decides per field: state or config, file format (yml, json or text), file-to-DB sync direction, watch and external edits, validation errors shown to the user, bad-edit handling, migration of existing rows, conflict with the Studio/Space DB split.
- Secret params keep P447's keychain layer; files hold references only.
- UI becomes minimal: view, open and edit the file; the plan picks plain file edit or a small edit action.
- Test: backend flow (edit file, DB follows, bad edit surfaces an error and leaves state intact) and UI spec on the same scenario.

Result:

- Go: runs are `<id>.yaml` files in the `runs` config folder (Studio and Space), projected into `custom_scripts`; one-shot migration; bad file keeps last good and surfaces a problem. Secrets stay in keychain.
- UI: `RunFileDialog` (raw YAML, conflict Reload/Overwrite, secrets section) replaces `ScriptDialog`. Panel shows broken-file rows, Edit/Open/Reveal file, move up/down, About. Space gets `ScheduleTargetDialog`. Editor-only components and 80 unused i18n keys deleted. Unused `chooseFolder` context method removed.
- Docs: `docs/ARCHITECTURE.md`, `site/.../shared/runs.md`.
- Ran: typecheck, biome, knip, `lint:rules`, `go vet ./apps/... ./internal/...`, pre-commit hooks. Go: full Space and Studio `termflow` packages (TestRunsConfigFile, TestRunsConfigFileStudio, TestRunsFileMigration pass), both per-app run-file migration tests, `internal/configfs`, `internal/scriptruns`. UI: Space `automations-*`, `color-rails`, `ui-leftovers`, `copy-coverage` (86 tests); Studio `automations*`, `script-order`, `color-rails`, `ui-leftovers-docker`, `copy-coverage-docker` (96 tests). Full suites not run, per chapter rule.
- Found by the runs: 8 Space contract fixtures drifted (`collection` for `collectionId`, id is the file stem), regenerated with `KIRA_CONTRACT=write`. Collection header menu from the plan (New script here, New smart script here, Move up/down) was missing; added (`groupMenu`). Two spec fixes (testid, `schedule: null` arg, one data allow-pattern).
- Scheduled run with no task and a built-in variable: new flow `TestScheduleNoTaskWithBuiltinVar`; run is recorded skipped with "no value for task".
- Deviations: Go `ScriptRunsService.McpServers` left bound (used by `tests/claude` and a contract); frontend no longer calls it.
- UI specs: `automations-editor` deleted (both apps); `automations-file` new (both apps); `automations-secrets`, `-smart`, `-recurring`, `-runs`, `-tabs`, `-order`, `script-order`, `automations-module`, `automations-scripts`, leftovers and copy-coverage adapted; dialog, collection CRUD, Choose-folder, drag-between and Move tests deleted; `script-dialog` visual test removed.

### P473 API config as files in the app folder

- Same treatment for API config (requests, variables, environments): files plus `AGENTS.md`, per the P450 foundation. Secret variables are keychain references; Studio's `apivars` two-stage substitution stays the interpolation rule.
- Plan decides per field what stays in SQLite, formats, sync, watching, validation, bad edits, migration, Studio/Space split.
- Minimal UI as in P472.
- Test: as P472.

Result:

- Go: collections and environments are `<id>.collection.yaml` and `<id>.environment.yaml` files in the `api` config folder, projected into the `api_*` tables by `apifiles.Store` (the only writer, except `SetActiveEnvironment`). Migration 0055 and a one-shot `MigrateOnce` export. Secret variables are `${secret:name}` (scope `variable`, owner `api`); sealed envelopes move verbatim. Bad file keeps last good and surfaces a line problem. `ApiConfigService` (`View`, `SetSecret`, `DeleteSecret`) is new; event `kira:api:dataChanged` gains kind `config`.
- Removed bound methods: collection and item rename, delete, duplicate, move, reorder; environment update, delete, duplicate, reorder; variable CRUD, bulk and restore. Kept: see `docs/ARCHITECTURE.md` "Api config files (P473)".
- UI: tree rows Open file and Reveal file; `ApiConfigProblems`, `ApiSecrets`; variable sets and environments read-only; variable-set tab names follow the file (`syncVariableSetTabNames`). Deleted: `EnvironmentFields`, `BulkVariablesEditor`, `collectionDrop`, `collectionOrder`, `draftMerge`.
- Ran: `go vet ./apps/kira-studio/... ./internal/...` (Space vet needs its dist, not built here), `golangci-lint run` (0 issues), `bun run lint`, `lint:dead`, all typechecks, unit suite (1126 pass). Go: `apifiles`, `flows/apiflow` (`TestApiConfigFiles` and siblings), `storage/...` (migration test), `internal/configfs`. UI after `build:test:studio`: new `api-config-files`, plus the Studio api, http, variables, consistency, grpc, list-keyboard, ui-leftovers specs (51 + 191 + 22 + 103 pass). Full suites not run, per chapter rule.
- Specs: kept `api-cross-window-sync`. Deleted `api-order`, unit `api-collection-drop`, `api-draft-merge`. Adapted `api-collections-delete-orphans-cache`, `api-variables-delete-eviction`, `api-collections-search-debounce`, collections, http-variables, api-secret-reveal-isolation, api-ui-consistency, consistency, grpc-request, list-keyboard, ui-leftovers (move, reorder, rename, bulk, duplicate and delete tests removed).
- Deviation: `configfs` doc walker gets a cycle guard (the item schema recurses through folders). Shared by P474 and P475.
- Found: `golangci-lint` gocognit on `syncItems`, `syncVariables` and the migration; split into helpers.
- Docs: `docs/ARCHITECTURE.md`, `site/.../studio/api-client.mdx`.

### P474 Git repos config as files in the app folder

- Per-repo options (prepare worktree and the other repo settings, today behind `guardRepoSettingsSet` and `repoSettings.changed`) become files plus `AGENTS.md`, per the P450 foundation. Runtime state (heads, fetch state, holds) stays in SQLite.
- Runs before the git IPC phases (P476 to P479) so repo settings are not touched twice. The guard and the `repoSettings.changed` event keep their behaviour; the plan decides how a file edit passes the guard.
- Plan decides per field what stays in SQLite, formats, sync, watching, validation, bad edits, migration, Studio/Space split.
- Minimal UI as in P472.
- Test: as P472, plus the guard still refuses what it refuses today.

Result:

- Go: per-repo settings are `~/.kira-space/repos/<stem>.yaml`, bound to a repo by the `path:` key (not the stem). New `internal/repocfg` (`Store` with an in-memory index, last-good projection, hot reload, `Patch` through the new shared `configfs.EditYAML`, `Service`, `repocfgtest`). All 11 `git_repo_settings` leaves plus nickname, integration branches and environments live in the file. `git_repo_settings`, `ade_repo_config.nickname`, `ade_repo_integration` and `ade_repo_envs` are gone; migration 0065 repoints `ade_env_state` at `code_repos`, a `MigrateOnce("repos")` export writes the files first and drops the legacy tables in one transaction. `AdeTaskService.UpdateRepo` and the `SetRepoSettings` dep are removed; `RepoConfigService` (`List`, `File`, `SaveFile`) and event `kira:repoConfig:changed` are new.
- Guard: `guardRepoSettingsSet` and `bridge/gitstream.go` are unchanged (also `packages/git-ui`, `packages/git-ipc`). The stream still refuses the prepare script and base path with `E_READ_ONLY` before `Patch`; a file edit never crosses the stream. A file edit emits `repoSettings.changed` (a stream patch does not twice, `FromPatch`).
- UI: `RepoFilePane` (file editor with hash, Reload and Overwrite, template and Create settings file) plus `RepoIdentity` (colour, path, source) replace `RepoConfigForm` and `RepoEnvRow`; an error dot on repo rows; an Other files tab for unbound or duplicate files. `useUpdateRepo` and `RepoPatch` are deleted.
- Ran: `go vet` (Space `./internal/...`, root `./internal/...`), `golangci-lint run ./internal/...` in both modules (0 issues; Space `./...` only fails on the missing `frontend/dist-mobile` embed), `bun run lint`, `lint:dead`, `typecheck`, `test:unit` (1965 pass). Go: `internal/configfs` (`TestEditYAML`, `TestAgentsDocKeyedBy`), Space `storage/...` (`TestMigration65RepoFiles`), `ade`, `gitrpc`, `gitsession`, `bridge` (guard tests), `appwire`, `flows/repoflow` (new `TestRepoConfigFile`, `TestRepoConfigMigration`), `flows/adeflow`, `flows/gitflow` (new `TestRepoSettingsLiveInFile`). UI after `build:test:space`: new `repo-config-file`, `repos-dialog`, `ade-v2-panel`, `color-rails`, `consistency`, `copy-coverage`, `copy-wording`, `list-keyboard`, `ade-agent-context`; `site:check` green. Full suites not run, per chapter rule.
- Keep/adapt/delete: kept the colour, hide, remove, folder and import tests of `repos-dialog`; adapted `selecting a repo`, `a repo row menu` and the `copy-coverage` repositories test; deleted `each field sends a one-leaf patch`, `Choose... writes the picked folder`, `a rejected timeout`, `Add environment writes nothing` (the form is gone). Go: adapted board, integration, run-engine and adeflow harnesses to write repo files; deleted the `git_repo_settings` repository test; contract `repo-env.json` regenerated without the UpdateRepo keys, `ade-automation.json` regenerated (stale since P472, found failing before this phase).
- Deviations: the Base path "Choose..." button is gone (the file holds the path); an integration branch equal to main stays in the file and in the Repos wire, it is ignored where targets are used; `configfs.DocModel.KeyedBy` added for the path-keyed folder; the migration drops legacy values the file could not hold (invalid provider, oversize script, relative base path, blank review candidates).
- Docs: `docs/ARCHITECTURE.md` ("Repo config files (P474)" and the git facts), `site/.../space/git.mdx`.

### P475 Connections config as files in the app folder

- Everything configurable on a connection is file config plus `AGENTS.md`, per the P450 foundation: name, MCP settings, through to the colors of the filters in the left panel. Secrets (passwords, tokens) are keychain references, never plaintext in files.
- Runtime state (connection status, caches, last-used) stays in SQLite.
- Plan decides per field what stays in SQLite, formats, sync, watching, validation, bad edits, migration, Studio/Space split.
- Minimal UI as in P472; filter colors move from the UI editor to the file.
- Test: as P472, plus a color edit in the file shows in the left panel.

Result:

- Go: each connection is `~/.kira-studio/connections/<stem>.yaml`, bound to a surrogate row by `stem` and projected by `connfiles.Loader` (only writer). `connections.config_json` holds the last valid file JSON and is the record source. Secrets are `${secret:<stem>.password}` and `<stem>.uri` in `secret_values`; sealed envelopes move verbatim. Bad file keeps last good, publishes a line problem. Edits from the app go through `Loader.Patch` (`configfs.EditYAML`). Connection `color` (row rail) and per-connection visibility filters both live in the file. `ConnectionConfigService` (`View`, `File`, `SaveFile`, `SetSecret`, `DeleteSecret`) and event `kira:connectionConfig:changed` are new. Update, duplicate, delete and field setters are removed. Mask rules key by stem. Migration `MigrateOnce("connections")` plus schema migration.
- UI: `NewConnectionDialog`, `ConnectionFileDialog`, `ConnectionSecrets`; tree menu Edit connection file, Open file, Reveal file; problem marker on rows; broken-file entries in the panel; MCP pane Edit button. Deleted: `ConnectionDialog` and its tabs, `CheckboxRow`, connection `form.ts` and `reveal.ts`, Privacy tab and mask-rule management UI, key regeneration. 80 unused i18n keys pruned.
- Ran: `go test ./internal/...`, `go vet`, `golangci-lint` (0 issues), `bun run lint`, `lint:dead`, `typecheck`, `test:unit`, `site:check`. Contracts regenerated (`connection-files`, `connection-secrets`, `connection-files-migration`, `connection-order`, `dbmcp-review-connection`). UI after `build:test:studio`: new `connection-files`; rewritten `connections`, `secrets`, `credential-reveal`, `preconnect`; adapted `connection-paste-credentials`, `dbmcp-review-connection`, `tabs`, `consistency`, `interaction`, `control-sizing`, `copy-coverage`, `focus-ring`, `ui-leftovers`, `cell-editor` and about 20 specs moved to `createConnection`. Full suites not run, per chapter rule.
- Keep/adapt/delete: deleted `connection-dialog-tabs.spec.ts`; dropped the submenu-flip test in `tabs`, the dialog-only control-sizing tests, the preconnect dialog tests, and the Privacy tab, mask-rule management and key regeneration coverage. The Filters... save path stays covered by `tree.spec`.
- Deviations: no `yamledit.go` (`configfs.EditYAML` exists); `config_json` is the record source; no `connfilestest` package (tests call `Loader.Patch`).
- Docs: `docs/ARCHITECTURE.md` ("Connection config files (P475)"), `site/.../studio/databases.md`.

### P476 Git on bound services: connection lifecycle, events, read methods

- Today git has its own IPC: `packages/git-ipc` (`rpc.ts`, `transport.ts`, `streamChannel.ts`, `validate.ts` with `CONTRACT_VERSION` 56, `blobFrame.ts`) over one Wails stream per window, served by `apps/kira-space/internal/bridge/gitstream.go` `ServeGitStream` through `internal/rpcstream` into `gitrpc.Router.ForConn` (57 methods in `apps/kira-space/internal/gitrpc/handlers.go`, events, one `graph.stream`).
- Every other module calls a bound service in `internal/bridge` through generated Wails bindings, gets `kira:<module>:<event>` pushes, and holds state in TanStack Query or Pinia (ARCHITECTURE "Process model", "UI architecture").
- Goal for P476 to P479: git follows that convention. Behaviour unchanged: same results, errors, events, credential prompts, op log records, guards (`guardRepoSettingsSet`).
- P476 adds the bound git service (name by plan) and moves the per-window `gitsession.Conn` behind it: created on a window's first call, released on window close. Holds, walks, credential routing to `gitcred.Relay` with the window key, auto-fetch off, writes detached on disconnect: all as today.
- Events (`repo.changed`, status, refs, `autoFetch.changed`, `repoSettings.changed`, op progress) become window-addressed app events.
- `packages/git-ui` `BridgeClient` keeps its typed `request`/`on`/`stream` surface. A binding-backed path replaces the stream method by method, so `state/` modules barely change. The plan decides adapter versus direct calls and lists what moves to TanStack Query.
- P476 moves the read-only methods; the plan lists the exact set from `handlers.go` (app, repo, status, refs, detail, file, blame, search, settings and host reads). Both transports coexist until P479.
- Surface ownership, in migration order: P476 bridge service, connection lifecycle, events, reads. P477 write handlers (`ops.go`, `stash.go`, `stack.go`, `worktree.go`, `remote.go`, `reset.go`) and their git-ui callers. P478 `review.go`, `changerequests.go`, `comments.go` and every consumer outside `packages/git-ui`. P479 `graph.go`, `incremental.go` and the deletions. Each phase touches the shared service and `BridgeClient`, so they run in order, not as parallel streams.
- Test: Go flow tests (`flows/gitflow`, `flows/repoflow`) call the bound service as other modules' flows do. UI tier: migrated methods answer from `/wails/runtime` fixtures (`CHANNEL_TO_FQN`) instead of the fake stream socket; existing git UI specs pass with unchanged assertions. Contract fixtures regenerated.

Result:

- Go: `bridge.GitWindows` (`gitwindows.go`) holds one `gitsession.Conn` per window key, created on the first call and released by `Close(key)` (OnWindowClosing) or when the window's last stream closes. Credential routing to `gitcred.Relay` with the key, auto-fetch off and the `Module` tag are set once at creation. `bridge.GitService` (`gitservice.go`) carries 14 methods through generic `callGit[T]` over `handlers.Request`: AppInit, RepoOpen, RepoClose, StatusGet, RefsList, CommitDetail, CommitFileDiff, WorkingDetail, FileRead, FileGoToTarget, BlameLine, SearchRun, RepoSettingsGet, RepoSettingsSet. `guardRepoSettingsSet` moved to `gitwindows.go` and guards both paths. `ServeGitStream` attaches to the window's Conn; the stream keeps serving every method until P479. Events push as `EmitTo(windowKey, "kira:git:<event>")`. Bound services: 37. `gitrpc.ContractVersion` stays 58 (no method or shape change).
- Frontend: `repo/git/boundGit.ts` plus a rewritten `createNativeGitTransport`: `BOUND_METHODS` go through the bindings (abort and dispose call `CancellablePromise.cancel()`; `RpcError`/`TransportError` mapping kept), the rest stay on the stream. Repos open on both transports; events are filtered by held repoIds; dispose sends a bound `repo.close` per held repo. `BridgeClient` and `packages/git-ui` unchanged.
- Tests. Keep: all `packages/git-ui` and `packages/git-ipc` specs, unchanged assertions (262 pass); all 25 UI specs using the git mock (211 pass after the mock change). Adapt: `flowharness` `GitStream` routes bound methods through `GitSvc` and reads events from `app.Events`; `gitstream_test.go` unchanged and passing; UI mock `gitStreamMock.ts` answers `/wails/runtime` GitService calls (its own `BOUND_GIT_WIRE` map, not `CHANNEL_TO_FQN`), emits `kira:git:*` app events, and answers the runtime cancel call. Add: `flows/gitflow/bound_test.go` (all 14 methods, event addressing, guard parity with the stream, error-code parity, window close; contract `git-bound.json`); `tests/ui/git-bound.spec.ts` (window key on bound calls, repo.changed own repo vs other); `tests/unit/git-transport-bound.spec.ts` (cancel, dispose release, stream fallback). Delete: none.
- Ran: go vet and golangci-lint (apps/kira-space and root), `bun run lint`, `bun run lint:dead`, typecheck, `bun run test:unit` (1967 pass), `go test ./internal/flows/...` with `KIRA_CONTRACT` write then compare, bridge, gitrpc, flowharness; UI specs for the git mock and the new spec. Full e2e suites not run (chapter rule).
- Fixed on the spot: claudeflow `TestHermeticClaude/user_server_blocks` contract (`automations-mcp-app.json`) drifted since scripts took the name as id (`<space>/automations/ask`); regenerated.
- Deviations: UI mock uses a self-contained `BOUND_GIT_WIRE` map instead of 14 new `ipcChannels.ts`/`mockRuntime.ts` entries. The restricted-setting refusal has no UI assertion (git-ui cannot reach it); the Go flow test covers it. Under `-tags server` the stream key falls back to "main", so a non-main tab double-holds and gets doubled `repo.changed` until P479.

### P477 Git on bound services: write operations

- Move every write method: preflight, branch, commit writes, stash and global stash, undo, remote (fetch, push, pull through the credential relay), worktree, stack, working-tree writes, op cancel.
- Long ops keep progress events and cancel. The plan picks the cancel path (binding call cancellation or an explicit cancel method) and proves an in-flight write still finishes after window close.
- Test: `flows/gitflow` write flows through the bound service, incl. credential prompt routing (`TestCredentialRoutes`) and cancel. Affected git UI specs unchanged in assertions.

Result:

- Go: `bridge/gitservice_writes.go` adds 23 `GitService` methods over `callGit[T]` (all preflights, stash, global stash, `op.run`, undo, remote incl. `remote.cancel`, worktree, stack incl. `stack.cancelRestack`); 37 methods in total. No handler change, no contract version bump (58). Cancel path: explicit cancel methods; binding-promise cancel only abandons the reply, writes stay detached.
- Frontend: 23 more `BOUND_METHODS` in `repo/git/boundGit.ts`. git-ui drops `remote.progress` outside an active remote op and `stack.progress` outside a restack, since bound replies and events no longer share one ordered stream.
- Tests. Keep: all `packages/git-ui` and `git-ipc` specs, all UI specs using the git mock (208 pass), every existing gitflow, repoflow, appflow, reviewflow, githostflow and journeyflow test now running writes bound; existing `tests/contract/*.json` byte-identical after a `KIRA_CONTRACT=write` run. Adapt: harness bound table (+23) and `GitStream.Key()`; `gitStreamMock.ts` `BOUND_GIT_WIRE` (+23); `TestWriteSurvivesCancelAndDisconnect` gains a `window close` case. Add: `flows/gitflow/bound_writes_test.go` (`TestGitServiceBoundWrites`: all 23 methods, progress addressing, `RemoteCancel`, binding-ctx cancel, window close mid-write with op log record, window close mid-credential-prompt freeing the slot; contract `git-bound-writes.json`); `tests/ui/git-bound-writes.spec.ts`; one late-progress case each in `ops.spec.ts` and `stack.spec.ts`. Delete: none.
- Ran: go vet (`./internal/...`; `main.go` needs `frontend/dist-mobile`), golangci-lint (apps/kira-space and root `./internal/...`), `bun run lint`, `bun run lint:dead`, typecheck, `bun run test:unit` (1969 pass), the flow packages above plus bridge, gitrpc and flowharness, gitflow under `-race`, git-mock UI specs and the new spec. Full e2e suites not run (chapter rule).
- Fixed on the spot: data race in `gitsession.newRepoEntry` (`pump` read `crSync` before it was set; `pump` now starts last); `TestCredentialRoutes` read the credential note before it showed, now waits. One `test:unit` failure on the first run did not reproduce in 3 reruns.
- Deviations: commits 2 and 3 of the plan landed as one (`feat: git transport routes writes bound`, with the git-ui guards). The `-tags server` Wails call lifetime check with a slow remote was not run in the native app; the code path is unchanged.

### P478 Git on bound services: review, change requests, ADE and repo consumers

- Move `review.*`, `changeRequest.*`, comments and `gitHost.*`.
- Move every caller outside `packages/git-ui`: review windows (`apps/kira-space/frontend/src/repo/git/reviewSession.ts`), `views/repo/*` (blame line, review threads and decorations), `repo/*` (`RepoList.vue`, `RepoWorktreeRow.vue`, `state/repoHeads.ts`, `state/worktrees.ts`), `state/repoOpenHold.ts`, and ADE (`ade/v2/wire.ts`, `ade/v2/review/*`).
- Test: `flows/reviewflow`, `flows/githostflow`; ADE review and repo list UI specs unchanged in assertions.

Result:

- Go: `bridge/gitservice_review.go` adds 23 `GitService` methods over `callGit[T]` (review.*, comments, commit/branch change request methods, `gitHost.info`, `changeRequest.*`); 60 in total. `gitHost.providers` goes to the existing `GitHostService.Providers`. `gitWindow` tracks bound repo holds (`noteBoundOpen`/`noteBoundClose`): a window's Conn outlives its last stream while any remain. No handler change, contract version stays 58.
- Frontend: 24 more `BOUND_METHODS`. New `repo/git/repoHolds.ts` (window-level refcounted holds, ordered per-path open/close), `gitCall.ts` (`gitCall`, `onGitRepoEvent`), `reviewRepaint.ts` (review-write repaint fan-out and comment refresh, moved out of `transport.ts`). `state/repoOpenHold.ts` deleted. Consumers off the transport: `blameLine`, `fileContent`, `reviewSource`, `useDiffEditor` (snapshot), `ade/v2/review/*`, `repoHeads`, `worktrees`. `transport.ts` keeps git-ui mounts and host-local methods; its repo hold is the shared registry. Dead `onTransportEvicted` and `RELEASE_RETRY_MS` removed.
- Worktree lists are TanStack Query (`state/worktreeQueries.ts`); `worktrees.ts` keeps expand state, holds and `repo.changed` invalidation.
- Tests. Add: `flows/gitflow/bound_review_test.go` (`TestGitServiceBoundReview`: 23 methods, stream byte parity, error parity, hold outlives last stream), contract `git-bound-review.json`, `tests/ui/git-bound-review.spec.ts`, `tests/unit/git-repo-holds.spec.ts`. Adapt: harness bound table (+24), `RequestWire`, `GitStream.Close` releases bound-opened repos; `gitStreamMock.ts` `BOUND_GIT_WIRE` (+24, `repo.close` answers by default); unit Wails stub keeps `Events.On` handlers (`emitWailsEvent`); `blame-line-controller.spec.ts` mocks. Existing contract files byte-identical.
- Ran: go vet, golangci-lint (root and apps/kira-space, 0 issues), `bun run lint`, `lint:dead`, typecheck, `test:unit` (1973 pass), flow packages (gitflow under `-race`, repoflow, reviewflow, githostflow, appflow, journeyflow, adeflow), bridge, gitrpc, flowharness, related UI specs (219 plus the new spec). Full e2e suites not run (chapter rule).
- Fixed on the spot: gocritic `stringXbytes` in the new flow test; knip unused exports left by the move.

### P479 Git graph data plane; remove the git stream protocol

- `graph.stream` sends FlatBuffers `PackedCommitChunk` frames (`KIG1`, up to 32 MiB, credit flow control). The graph bulk payload keeps the FlatBuffers path, as a bulk-only data plane in the style of Studio's engine data plane. Everything else leaves the custom git stream protocol. Measure first and after with the git graph perf probe (`GRAPH_N`) on a large repo; no regression in time to first rows, scroll, or peak memory.
- Delete the custom git stream protocol for everything except the graph bulk payload: `ServeGitStream` request/response handling, the `packages/git-ipc` `rpc.ts`, `validate.ts`, `gitrpc.ContractVersion` and `CONTRACT_VERSION` with their tests, the UI tier's fake git stream. Delete `internal/rpcstream`, `transport.ts` and `streamChannel.ts` only if nothing still uses them after the graph data plane lands. Keep `gitwire.fbs`, the codec and `blobFrame.ts` as the data plane needs.
- Update ARCHITECTURE "Git module" (Transport, Frontend packages) and `docs/DEV_ENVIRONMENT.md` git sections.
- Test: full git flow and UI suites once (shared code); perf probe before and after, numbers in the plan.

**Result:**

- Go: `GitService` has 65 bound methods plus `ResetWindow` (drops the window's Conn; the frontend calls it once per page load). New `internal/graphplane`: bulk-only `gitgraph` stream (initial credit 2, 32 MiB frame cap, `E_FRAME_TOO_LARGE`, duplicate live id refused, missing window `E_CONFLICT`). `GitWindows` conns live until `Close(key)` or `ResetWindow`; stream/bound refcounting gone.
- Deleted: `internal/rpcstream`, `bridge/gitstream.go`, `gitrpc.ContractVersion`/`Protocol`, `AppInitResult.ContractVersion`, `GitRouter`, `guardRepoSettingsSet` (patch check stays in `patchTouchesRestrictedField`, now covered by a reflection test), `packages/git-ipc` `rpc.ts`/`validate.ts`, `CONTRACT_VERSION`, `git-transport-late-close.spec.ts`.
- Frontend: every git request is `callBound` after `gitWindowReady()`; `repo/git/graphStream.ts` dials `Stream('gitgraph')` (lazy, redial after close) over `packages/git-ipc` `graphPlane.ts`. UI mock rewritten for the `gitgraph` socket; new `git-graph-plane.spec.ts` (5 tests).
- Contract fixtures: only `app.init` entries changed (`contractVersion` removed) plus new `git-graph-plane.json`.
- Ran: Go flows (`KIRA_CONTRACT=write`, gitflow under `-race`), bridge, gitrpc, graphplane, flowharness; go vet and golangci-lint (root and nested modules, 0 issues); `bun run lint`, `lint:dead`, typecheck, `test:unit` (1972 pass); 324 git/repo/ADE-review UI specs pass.
- Perf (median of 3): Go first chunk 306 to 253 ms; UI first rows 6149 to 6763 ms (within noise, range overlaps), flick p95 unchanged, RSS peak within 5%. Details in `plans/P479.md`.
- Deviation: old rows in this file still name removed symbols; left as history.

### P480 Frontend RAM measurement, both apps

- Real measurement is justified here. Build on P433 (v0.2.24): its findings (`docs/v0.2.24/plans/P433-measurements.md`, read from git once folded), the heap probe (`tests/perf`, `--project=heap`, `packages/workbench/src/testing/ui/heapProbe.ts`) and the RSS sampler (`perfProbe.ts`).
- Per app, mode and view: JS heap with retained size by constructor (CDP heap snapshots, Chromium), process RSS (`ps`), idle after start, after opening N tabs, after closing them all (leaks). Measure on Linux only. If the Linux footprint is lower, no Mac check; no Mac-run step in this chapter.
- Suspects to measure: Monaco instances and models, SlickGrid grids and row caches, TanStack Query retention (`gcTime`), Pinia stores holding large data, kept-alive or mounted-but-hidden panes, xterm instances and scrollback, large lists, images, the whole-catalog i18n bundle (`packages/theme/src/i18n/en.json`), git graph store, ADE board, memory module, Docker and Kubernetes logs.
- Findings file `plans/P480-ram-findings.md`, committed before P481 starts: per candidate the measured cost, the fix, the expected saving, a rank, and a stated saving threshold for "proven". A candidate with no real cost says so.
- Probe additions stay report-only. No app code changes in this phase.

**Result:** Probe (`memProbe.ts`, `heapSnapshot.ts`, per-app `heap-ram.spec.ts`) ran Pass A (3 repeats, Chromium and WebKit) and Pass B (unminified snapshots) on both apps; findings in `plans/P480-ram-findings.md`. 4 candidates clear the threshold: Studio docker list virtualization (+53 MB heap, +90 MB RSS at 400 containers), xterm scrollback (about 4 MB array bytes per full terminal), Space memory list virtualization (+9 MB heap at 500 rows), Space `RepoDiffView` retaining closed diff editors (3.5 MB). Space ADE mount retained after leaving (+13 MB heap) is real but unattributed, so not proven; TanStack `gcTime`, Pinia, images, i18n and the Discover growth (V8 JIT) have no cost.

### P481 Frontend RAM: implement the proven wins

- Implement only findings above the P480 threshold. Each fix is re-measured with the same probe; before and after numbers go in the plan and `docs/PERF.md`.
- The planner may split this row into `P481 Part 1`, `Part 2`, … by finding group.
- Behaviour unchanged: switching tabs never loses state or refetches only because of UI interaction (P433 rule).
- Test: heap and RSS probes rerun; affected UI specs. A new spec only where a fix changes when state is released and restored (e.g. an editor disposed while hidden, restored on return).

### P482 Code review (2 rounds)

One row, two rounds. Each round: one Opus review over the full diff since `main`, findings file under `plans/`, then one Sonnet fixer. Then close, then PR, as in v0.2.25.

### P483 Close chapter v0.2.28

Hand-written close per CLAUDE.md "Closing a chapter". Push, open PR to `main`.
