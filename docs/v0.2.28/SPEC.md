# v0.2.28

Branch `chapter/v0.2.28`. First `P` is 472 (highest in v0.2.27 is P471). Open.

Source: user request. Phases run top to bottom, two phases in parallel where file ownership allows (worktree per stream). Scope is closed: any further phases go to v0.2.29. Ends with two code review rounds, then close, then PR.

| Phase | Title | Status |
|---|---|---|
| P472 | Runs config as files in the app folder | Done |
| P473 | API config as files in the app folder | Todo |
| P474 | Git repos config as files in the app folder | Todo |
| P475 | Connections config as files in the app folder | Todo |
| P476 | Git on bound services: connection lifecycle, events, read methods | Todo |
| P477 | Git on bound services: write operations | Todo |
| P478 | Git on bound services: review, change requests, ADE and repo consumers | Todo |
| P479 | Git graph data plane; remove the git stream protocol | Todo |
| P480 | Frontend RAM measurement, both apps | Todo |
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

### P474 Git repos config as files in the app folder

- Per-repo options (prepare worktree and the other repo settings, today behind `guardRepoSettingsSet` and `repoSettings.changed`) become files plus `AGENTS.md`, per the P450 foundation. Runtime state (heads, fetch state, holds) stays in SQLite.
- Runs before the git IPC phases (P476 to P479) so repo settings are not touched twice. The guard and the `repoSettings.changed` event keep their behaviour; the plan decides how a file edit passes the guard.
- Plan decides per field what stays in SQLite, formats, sync, watching, validation, bad edits, migration, Studio/Space split.
- Minimal UI as in P472.
- Test: as P472, plus the guard still refuses what it refuses today.

### P475 Connections config as files in the app folder

- Everything configurable on a connection is file config plus `AGENTS.md`, per the P450 foundation: name, MCP settings, through to the colors of the filters in the left panel. Secrets (passwords, tokens) are keychain references, never plaintext in files.
- Runtime state (connection status, caches, last-used) stays in SQLite.
- Plan decides per field what stays in SQLite, formats, sync, watching, validation, bad edits, migration, Studio/Space split.
- Minimal UI as in P472; filter colors move from the UI editor to the file.
- Test: as P472, plus a color edit in the file shows in the left panel.

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

### P477 Git on bound services: write operations

- Move every write method: preflight, branch, commit writes, stash and global stash, undo, remote (fetch, push, pull through the credential relay), worktree, stack, working-tree writes, op cancel.
- Long ops keep progress events and cancel. The plan picks the cancel path (binding call cancellation or an explicit cancel method) and proves an in-flight write still finishes after window close.
- Test: `flows/gitflow` write flows through the bound service, incl. credential prompt routing (`TestCredentialRoutes`) and cancel. Affected git UI specs unchanged in assertions.

### P478 Git on bound services: review, change requests, ADE and repo consumers

- Move `review.*`, `changeRequest.*`, comments and `gitHost.*`.
- Move every caller outside `packages/git-ui`: review windows (`apps/kira-space/frontend/src/repo/git/reviewSession.ts`), `views/repo/*` (blame line, review threads and decorations), `repo/*` (`RepoList.vue`, `RepoWorktreeRow.vue`, `state/repoHeads.ts`, `state/worktrees.ts`), `state/repoOpenHold.ts`, and ADE (`ade/v2/wire.ts`, `ade/v2/review/*`).
- Test: `flows/reviewflow`, `flows/githostflow`; ADE review and repo list UI specs unchanged in assertions.

### P479 Git graph data plane; remove the git stream protocol

- `graph.stream` sends FlatBuffers `PackedCommitChunk` frames (`KIG1`, up to 32 MiB, credit flow control). The graph bulk payload keeps the FlatBuffers path, as a bulk-only data plane in the style of Studio's engine data plane. Everything else leaves the custom git stream protocol. Measure first and after with the git graph perf probe (`GRAPH_N`) on a large repo; no regression in time to first rows, scroll, or peak memory.
- Delete the custom git stream protocol for everything except the graph bulk payload: `ServeGitStream` request/response handling, the `packages/git-ipc` `rpc.ts`, `validate.ts`, `gitrpc.ContractVersion` and `CONTRACT_VERSION` with their tests, the UI tier's fake git stream. Delete `internal/rpcstream`, `transport.ts` and `streamChannel.ts` only if nothing still uses them after the graph data plane lands. Keep `gitwire.fbs`, the codec and `blobFrame.ts` as the data plane needs.
- Update ARCHITECTURE "Git module" (Transport, Frontend packages) and `docs/DEV_ENVIRONMENT.md` git sections.
- Test: full git flow and UI suites once (shared code); perf probe before and after, numbers in the plan.

### P480 Frontend RAM measurement, both apps

- Real measurement is justified here. Build on P433 (v0.2.24): its findings (`docs/v0.2.24/plans/P433-measurements.md`, read from git once folded), the heap probe (`tests/perf`, `--project=heap`, `packages/workbench/src/testing/ui/heapProbe.ts`) and the RSS sampler (`perfProbe.ts`).
- Per app, mode and view: JS heap with retained size by constructor (CDP heap snapshots, Chromium), process RSS (`ps`), idle after start, after opening N tabs, after closing them all (leaks). Measure on Linux only. If the Linux footprint is lower, no Mac check; no Mac-run step in this chapter.
- Suspects to measure: Monaco instances and models, SlickGrid grids and row caches, TanStack Query retention (`gcTime`), Pinia stores holding large data, kept-alive or mounted-but-hidden panes, xterm instances and scrollback, large lists, images, the whole-catalog i18n bundle (`packages/theme/src/i18n/en.json`), git graph store, ADE board, memory module, Docker and Kubernetes logs.
- Findings file `plans/P480-ram-findings.md`, committed before P481 starts: per candidate the measured cost, the fix, the expected saving, a rank, and a stated saving threshold for "proven". A candidate with no real cost says so.
- Probe additions stay report-only. No app code changes in this phase.

### P481 Frontend RAM: implement the proven wins

- Implement only findings above the P480 threshold. Each fix is re-measured with the same probe; before and after numbers go in the plan and `docs/PERF.md`.
- The planner may split this row into `P481 Part 1`, `Part 2`, … by finding group.
- Behaviour unchanged: switching tabs never loses state or refetches only because of UI interaction (P433 rule).
- Test: heap and RSS probes rerun; affected UI specs. A new spec only where a fix changes when state is released and restored (e.g. an editor disposed while hidden, restored on return).

### P482 Code review (2 rounds)

One row, two rounds. Each round: one Opus review over the full diff since `main`, findings file under `plans/`, then one Sonnet fixer. Then close, then PR, as in v0.2.25.

### P483 Close chapter v0.2.28

Hand-written close per CLAUDE.md "Closing a chapter". Push, open PR to `main`.
