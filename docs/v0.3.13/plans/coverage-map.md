# P626 coverage map: what existing tests actually do

Base: `chapter/v0.3.13` at `fd6694930`. Read-only survey; no test changed.

Method: test names and titles from every file, plus per-file signal scan for far-end oracles
(`os.ReadFile`/`os.Stat`/`ReadDir`, `Repo.Git`/`Bare.Git`/`RevList`, `app.DB()`/`sql.Open`, fake
recorders such as `Browser.Opened`, `FileOpener.OpenedFiles`, `ImageClipboard.Writes`,
`Forge.Calls`, `KubeFake.Requests`), contract writes (`app.Contract`), event waits
(`app.Events.Since`) and, for UI specs, `contract(...)`, control-log reads (`calls(...)`,
`control.log()`), clipboard spy and toast references. Signal counts are a scan, not proof: a file
with zero far-end signals may still assert through a bound read-back. "Self-oracle" below means the
test reads its result back through the same bound service it wrote with.

Counts: 30 flow packages, 326 `_test.go` files, 935 `Test*` functions. UI: 205 Space specs (1,376
tests) plus 124 Studio specs (654 tests) plus 16 shared scenario modules (77 tests). 214 contract
JSON files.

## 1. Tiers at a glance

| Tier | Location | Runner | Backend | CI |
|---|---|---|---|---|
| Flow (backend half) | `apps/kira-space/internal/flows/*` | `bun run test:flows` (`go test -p 1`) | real composition root (`appwire.Build`), real SQLite, git, PTY; OS shell faked | go job; `KIRA_FLOW_COMPLETE=1` cases only on demand (`test:flows:complete`), not in `pr.yml` |
| UI (frontend half) | `tests/ui`, `tests/ui/studio`, `packages/workbench/src/testing/ui/scenarios` | Playwright `ui` (WebKit, 8 shards), `ui-clipboard` (Chromium), `ui-timing` | static `frontend/dist`, every `/wails/*` call mocked | PR CI |
| Contract | `tests/contract/*.json` | written by flows (`KIRA_CONTRACT=write`), read by UI | n/a | both halves |
| Visual | `tests/visual` (27 specs) | Playwright `visual` (WebKit, pixel snapshots) | mocks | PR CI (Linux) |
| IPC frontend | `tests/ipc/<adapter>` (6 specs) | Playwright `ipc-frontend` | mocks from `internal/ipcfixture` captures | PR CI |
| e2e-real | `tests/e2e-real` (8 specs) | Playwright `e2e-real` (Chromium) | `go build -tags server` binary, real SQLite/git/PTY, real DB containers | local only (`test:e2e-real`); not in `pr.yml` |
| Mobile | `tests/mobile` (9 specs) | Playwright `mobile-ios`, `mobile-insecure` | `support/mockServer.ts` scripted `/api` | local only (`test:ui:mobile`); not in `pr.yml` |
| Real claude | `tests/claude` (nested Go module, 31 files) | `bun run test:claude*` | flow harness with real `claude` CLI | never in CI (costs tokens) |
| Perf | `tests/perf` (15 specs) | `playwright.perf.config.ts` | mocks | on demand |
| Screenshots | `tests/screenshots` (6 shot files, 25 shots) | Playwright `screenshots` | mocks + contract | `pages.yml` (site), not a test gate |
| Unit (bun) | `tests/unit` (26 Space, 100 Studio) | `bun run test:unit` | none | PR CI |
| Proto grid | `tests/proto` (6 specs) | `playwright.proto.config.ts` | static proto page | on demand |
| Adapter conformance | `internal/adapters/*/*_test.go` | `go test`; matrix under `KIRA_TEST_MATRIX` | real Testcontainers | containers job; matrix job `test-matrix` |
| IPC fixture capture | `internal/ipcfixture/*_test.go` | `go test` (`KIRA_IPC_FIXTURES=write`) | real containers | containers job |

## 2. Shared harnesses and fixtures

### 2.1 Flow harness (`internal/flowharness`)

- `flowharness.New(t, opts...)` (`harness.go:268`): temp root with `Home`, `SpaceHome`, `MemoryHome`,
  `Work`, `BinDir`, `FakeDir`; sets `HOME`, `KIRA_SPACE_HOME`, `GIT_CONFIG_GLOBAL`, `PATH=BinDir:…`,
  `TZ=UTC`, `KIRA_INSECURE_SECRETS=1`; writes git identity; then `appwire.Build` with real
  `storage.Open`, `repos.New`, `secrets.New`. Teardown in main's order. Skips without git.
- Bound services: `app.W.<Service>.<Method>(…)` (most used: `AdeTask` 787 calls, `ScriptRuns` 140,
  `Connections` 87, `Memory` 70, `Terminal` 65, `CustomScripts` 63, `CodeWorkspace` 55). Git
  requests: `app.OpenGitStream()` / `OpenGitStreamFor(windowKey)` speak the real `gitrpc` stream.
- Lifecycle: `app.Restart()`, `app.Quit(t)` (Wails shutdown order), `app.CloseWindow(key)`.
- Repos: `app.NewRepo`, `app.NewBare` (bare remote; `Bare.Clone`, `Bare.PushFrom`, `Bare.Git`),
  `app.NewHistory(HistorySpec)` (templated large histories), `Repo.Commit/Branch/Merge/Rename/
  Conflict/LinkedWorktree/WriteBinary/BadRemote`.
- Events: `app.Events.Mark()` then `Since(mark, channel)`; `EmitTo` per window.
- Contract: `app.Contract(t, scenario, key, got, Mask/Omit/Replace…)` and `ContractEvent`;
  temp paths replaced by `<work>`, `<home>`, `<space>`, `<tmp>`, `<bin>`, `<fake>`.
- Shell fakes (`shellfakes.go`, `capturefakes.go`), each a recorder:
  `Browser.Opened()`, `FileOpener.OpenedFiles()/RevealedFiles()`, `ImageClipboard.Writes()`
  (reads image bytes at write time; `SetMulti`, `SetFail`), `Clipboard.SetText/SetEmpty`
  (text read side only, for capture), `Shortcuts.Fire(accel)`, `Dialogs.AnswerSave/AnswerFile/
  AnswerDirectory/AnswerFiles` (empty queue = cancel), `WindowManager` (Opened, Closed, Focused,
  Titles, OpenedPopups, OpenedKeyed), `KeepAwakeDriver.Held()`, `McpInstaller.InstallCalls()`,
  `OSAuth.Script(available, outcome)`, `WindowFocus`, `ProofShell`.
- Agent fakes: test binary linked as `claude` and `gh` (`internal/flowtest/fakeagent`).
  `app.Scenario(fakeagent.Scenario{Claude, OpenCode, Prompts, Gh})`; `Action` names `done`,
  `needs_input`, `failed`, `result:<id>`, `fail`, `nofinish`, `lost`, `sleep`; fields `Sh` (agent
  commit), `MCP` (real MCP tool calls, `Parallel`), `Emit`, `WaitFile`. Calls recorded in `FakeDir`.
- Host fakes: `fakehost.Forge` (second code host, every call recorded, `Calls()/Count()`,
  `SetHead`, `SetRateLimited`, `AddComments`), `fakeaz` (Azure CLI argv), `fakeacli` (Jira),
  `WithGitHost`, `WithAzure`, `WithIssueTracker`, `WithoutGh`, `WithoutClaude`.
- Network servers: `flowharness.HTTP/HTTPS/GRPC` (`servers/`, shared CA, `WithGRPCTLS`,
  `WithRequiredMetadata`), `Silent`/`SilentAccepting` (black-hole targets), `NewKubeFake`
  (Kubernetes API, `Requests(sub)`, `AppendLog`, `Followers`, `WriteKubeconfig`).
- Real Docker: `flowharness.RequireDocker(t)` (skip, or fail under `KIRA_FLOW_DOCKER=require`),
  `BootGen` simulates daemon restarts. Real DB containers in flows: `testsupport.Start<Kind>` from
  `dbmcpflow/engines_test.go`, `engine_timeout_test.go`, `dbflow/kibana_test.go`; SQLite files via
  `sql.Open("sqlite", …)` in `dbflow/helpers_test.go`, `dbmcpflow/mcp_test.go`, `review_test.go`.
- Knobs: `WithSandbox` (default `sandboxtest.Fake{NotEnforced:true}`), `WithClock`,
  `WithSmartTimeout`, `WithScheduleTimeout`, `WithMobilePoll`, `WithBranchRequestWait`,
  `WithRebaseTimeout`, `WithTrackedUpdates`, `WithChangeRequestSync`, `WithEmbedSpec`,
  `WithProvider`, `WithOpenCode`, `WithWindowFocus`, `WithWayland`, `WithFileHandlers`,
  `WithDbMcpLimits`, `WithApprovalHeldCaps`, `WithDockerOptions`, `WithKubernetesOptions`.
- DB access: `app.DB()` gives the app's SQLite (`termflow` seeds `script_runs`, counts
  `secret_values`).
- Gates: `flowharness.Complete(t)` (`KIRA_FLOW_COMPLETE=1`): permutation matrices, large histories,
  slow cases. Used in 18 places (gitflow 5, dockerflow 7, dbmcpflow 4, dbflow 3, …).
- Guard: `flows/coverage` `TestEveryBoundMethodAndRequestHasAFlow` fails when a bound method or git
  request name has no call in any flow file (name match only; `exempt.txt` holds 1 entry).

### 2.2 UI harness (Playwright)

- `createUiFixtures` (`packages/workbench/src/testing/ui/fixtures.ts`): worker static server over
  `frontend/dist`; `relaunch(options)` opens a fresh page, optional `clockTime`, `prepare`, mocks
  installed before first navigation; `consoleErrors` collected. `/wails/*` not served (501 on miss).
- App fixtures (`tests/ui/fixtures.ts`): `test`, `studioTest` (selects Studio mode), `apiTest`,
  `reviewTest`, `filesTest`, `configSyncReviewTest`; plus `support/hostView.ts` `hostViewTest`,
  `support/capture.ts` `captureTest`. Options: `control` snapshots, `stream` (data grid pages),
  `gitStream`, `storage` (localStorage seed).
- Bridge mock: `installControlMocks` (`packages/workbench/src/testing/ui/mockRuntime.ts`) answers
  bound calls from `ControlSnapshot[]` keyed by channel + canonical args; miss gives
  `E_FIXTURE_MISS` 422. Handle: `log()` (every answered call, in order), `release(channel)` (held
  calls), `respond(channel, value)` (change answer mid-test). `support/ipcCalls.ts` `calls(control,
  channel)` filters the log. `emulateAdeTaskPush` pushes board events.
- Channel catalog: `tests/ui/support/ipcChannels.ts` (580 entries) mapped to Wails FQNs by
  `mockRuntime.ts` `CHANNEL_TO_FQN`; `tests/unit/mock-runtime-bindings.spec.ts` checks it.
- Contract loader: `tests/ui/support/contract.ts` `contract(scenario, key, {schema, origins})`
  hydrates placeholders and optionally zod-parses. Missing key throws "regenerate with
  KIRA_CONTRACT=write".
- Git stream: `support/gitStreamMock.ts` (`installGitStreamMockOnInit`), graph fixtures
  (`graphStreamFixture`, `graphPagingFixture`, `graphBranchHistoryFixture`); data grid stream:
  `studio/support/mockStream.ts`.
- Domain helpers: `support/adeV2.ts`, `adeReviewStep.ts`, `approvals.ts`, `approvalItems.ts`,
  `branchMenu.ts`, `hostView.ts`, `memoryNav.ts`, `bootSnapshots.ts` (`mergeBootSnapshots`),
  `clock.ts`, `firstFrame.ts`; Studio: `connect.ts`, `grid.ts`, `tree.ts`, `editor.ts`,
  `dialogs.ts`, `proof.ts`, `settings.ts`, `apiRows.ts`.
- Engine captures (not live DBs): `studio/support/postgresFixture.ts`, `mariadbFixture.ts`,
  `mongoFixture.ts`, `redisFixture.ts`, `s3Fixture.ts`, `sqsFixture.ts`, `kibanaFixture.ts`,
  `dockerFixture.ts`, `kubernetesFixture.ts` hold responses captured once from real containers
  (`scripts/capture-postgres-tree.ts`). `tests/fixtures/ade-v2/*.json`, `tests/fixtures/studio/*`
  are static fixtures; `tests/support/seeds/md5Rows.ts` seeds rows.
- Clipboard: `studio/support/clipboard.ts` `installClipboardSpy(page, {lazy, image})` stubs
  `navigator.clipboard.writeText` (and `write` for ClipboardItems / PNG blobs) into
  `window.__clipboard` / `__clipboardImages`. 26 call sites, 103 `__clipboard` reads. Real clipboard
  only in `ui-clipboard` project (Chromium, `grantPermissions(['clipboard-read'])`) for
  `studio/grid-copy-menu.spec.ts`.
- Shared scenarios: `packages/workbench/src/testing/ui/scenarios/*` registered by thin spec files
  (`automations-*.spec.ts`, `quit-confirm`, `scrollbars`, `terminal-links`, `script-order`), taking
  `{test, contract, ipc}`.
- Files and real DBs: none in `tests/ui`. File-system and DB effects are visible only as the mocked
  call's args in `control.log()`.

### 2.3 e2e-real (`tests/e2e-real/fixtures.ts`)

- Builds `go build -tags server` once per worker (600 s), starts it per test on a scratch root,
  plain browser page. `kira.call(service, method, args)` calls a bound method on the real server;
  `reload()`, `relaunch()` (SIGKILL + restart on same home/port). Fake agent for `claude`/`gh`
  (`fakeDir`). `flowServers` gives the flow harness HTTP/HTTPS/gRPC servers. `serverEnv`
  (e.g. `DOCKER_HOST`), `startMode`.
- Support: `support/postgres.ts`, `mariadb.ts`, `kafka.ts`, `sqlite.ts` (real containers / files),
  `gitRepo.ts`, `connections.ts`, `bound.ts`, `routes.ts`, `passthrough.ts`.
- No dialogs, keep-awake or window manager (no server-build counterpart).

### 2.4 Mobile (`tests/mobile/fixtures.ts`, `support/mockServer.ts`)

Built phone app (`frontend/dist-mobile`) plus scripted `/api` in one process; SSE stream and
pairing long-poll run for real; state reset per test. No Go behind it; backend half is
`mobileflow`, linked by contract (`mobile-*` scenarios).

### 2.5 Adapter suites and IPC fixtures

- `internal/adapters/testsupport`: one memoized Testcontainers instance per kind per test binary
  (`StartPostgres`, `StartMariadb`, `StartMysql`, `StartClickHouse`, `StartMongo`, `StartRedis`,
  `StartKibana`, localstack for S3/SQS, Kafka), seeded from `packages/db-fixtures/fixtures/
  0001_seed.sql` plus 1M-row `app.big_rows`; skip without Docker. `RunMatrix(t, kind, fixture,
  base, cases)` with `Scenario{Name, Requires(Caps), Run}`; complete suite under
  `KIRA_TEST_MATRIX`.
- `internal/ipcfixture/<adapter>_test.go` (`TestFixture_ClickHouse/Kafka/MariaDB/MySQL/Redis/SQS`)
  capture real responses into `testdata/<adapter>.fixture.json` and `tests/ipc/<adapter>/*.
  fixture.ts`; backend spec asserts each run against the capture;
  `tests/unit/studio/ipc-fixture-sync.spec.ts` fails if the two differ.

### 2.6 Real claude (`tests/claude/helpers_test.go`)

`newRealApp(t, trustDirs…)` boots the flow harness then swaps the fake `claude` for the real one
under a temp HOME; `preflight` runs one cheap `claude -p` (auth, haiku pin, not parent session),
skips on auth problems; `claudeP` adds haiku + budget cap; `recordSpend` totals cost;
`parseStream`, `toolUses`, `toolResult` read stream-json.

### 2.7 Adding a journey cheaply

Backend half (one flow test):

1. `app := flowharness.New(t, opts…)`; seed `repo := app.NewRepo("x")` (or `NewBare` + `Clone`
   for remote state); register via `app.W.CodeWorkspace`/repos service.
2. Script the agent: `app.Scenario(fakeagent.Scenario{Claude: {"x": {{Sh: "git commit …"},
   "done"}}})`; for host/tracker add `WithGitHost(fakehost.New())`, `WithIssueTracker(fakeacli…)`.
3. Drive `app.W.<Service>.<Method>`; wait with `app.Events.Since(mark, channel)`.
4. Assert the far end: `repo.Git("rev-parse", …)`, `bare.Git(…)`, `os.ReadFile`, `app.DB()`
   query, `app.Browser.Opened()`, `app.FileOpener.OpenedFiles()`, `app.ImageClipboard.Writes()`,
   `forge.Calls()`, `kube.Requests()`; then `app.Restart()` and read back.
5. `app.Contract(t, "<scenario>", "<Service.Method>", got)`; write once with `KIRA_CONTRACT=write`.

Frontend half (one UI spec): `relaunch({control: [{channel: IPC.x, args, response:
contract('<scenario>', '<Service.Method>')}]})`, act, assert DOM, assert `calls(kira.control,
IPC.y)` args equal what the flow sent. Name the flow test in a header comment (207 of 329 specs do).
Clipboard: `installClipboardSpy(page)` then read `window.__clipboard`.

### 2.8 Far-end oracle availability

| Oracle | Flow tests | UI specs | e2e-real |
|---|---|---|---|
| Text clipboard | none: copy is frontend-only (`navigator.clipboard`, 55 frontend files); `flowharness.Clipboard` is read side for capture | spy `window.__clipboard`; real only in `ui-clipboard` grid copy | none |
| Image clipboard | `ImageClipboard.Writes()` bytes (`proofflow/clip_test.go`) | spy `__clipboardImages` (proof window) | none |
| File on disk | `os.ReadFile/Stat/ReadDir` (adeflow 32/37, harnessflow 25, memoryflow 15, repoflow 11) | mocked call args only | real server writes; specs rarely read |
| App DB row | `app.DB()` (adeflow 12, termflow 12, harnessflow 8) | none | via `kira.call` read-back |
| Target DB row | `sql.Open` on SQLite file (`dbmcpflow/mcp_test.go` count of `notes`, `review_test.go` insert) ; containers via adapter reads | captured fixtures only | real Postgres/MariaDB/SQLite reads |
| Local git state | `Repo.Git` (gitflow 135, adeflow 101, reviewflow 39) | none | `boot-real` graph only |
| Remote git state | `Bare.Git("rev-parse", …)` (`gitflow/remote_test.go`, `adeflow/agentwork_test.go` force push) | none | none |
| Code host / tracker | `fakehost.Forge.Calls()`, fake `gh` rules + `FakeDir` | args in log | none |
| Browser / opener | `Browser.Opened()`, `FileOpener` (windowflow 38) | `linkOpenExternal` args | none |
| OS notification | `internal/flowtest/notifysink` recorder (notifyflow) | `SendTest` call only | none |
| Docker engine | real engine via `d.Cli` (`dockerflow/edit_test.go` 13, `restore_test.go` 5) | captured fixtures | `docker-real` real engine |
| Kubernetes | `KubeFake.Requests()` | contract | none |
| Window manager | `WindowManager` recorder | `windowsEnsure` etc. args | real server, two pages |

## 3. Flow packages

Per package: files: tests; scenarios; depth; IPC; fakes vs real; gaps. Edge-named = test names
containing Refus/Error/Fail/Invalid/Missing/Without/Cancel/Conflict/Stale/Lost/Timeout/Limit etc.
(name-level only; journeys hide edge cases inside).

### adeflow (79 files, 152 tests, edge-named 22)

- Tasks and board: `newtask` NewTaskLandsTodayWithoutRepos, EstimateSetChangeClear; `shelf`
  Shelves; `statuses` TaskStatuses; `boardfacts` FolderImportAndFacts, RefreshDefaultSettings,
  TaskBranches; `ids` AdeIDs; `readablenames` StepRenameFollowsName, RenameRefusedOnBrokenWorkflow,
  WorkflowRenameFollowsName, SharedFileKeepsStem, ReadableNamesMigration; `folders_hidden`
  HideFolder; `archived` ArchivedTaskKeepsData, ResumeArchivedSession; `lifecycle` ArchiveRisk,
  ReviewWindowAndAgent, RestartRecovery, GitHubWithoutGh, GitHubChangeRequestsWithFakeGh;
  `taskpanel` TaskPanelOrigins; `taskinfowait` TaskInfoWaitsForSetup; `reporequired`
  TaskStartNeedsRepo; `norepo` RepolessWorkflow, OutsideRepoStep.
- Branches: `branching` Branching; `branchadopt` BranchAdopt; `branchchange` BranchChange;
  `branchrename` BranchRename; `branchrequest` BranchRequest; `branchmerge` BranchRequestMerge,
  BranchRequestsTwoTasks; `taskbranches` TaskReposAndBranches, TaskBranchEditRefusals,
  TaskBranchDetachKeepsRunHistory; `multiworktree` MultiWorktree; `rebase` RebaseRun; `queue`
  QueueMergeTree; `leases` Leases; `agentwork` AgentCommitReachesBoard; `repoupdating`
  RepoUpdatingWhileGitBusy.
- Runs and steps: `runs` RunLifecycle; `outcome` RunOutcomes; `returnlimit` ReturnLimit;
  `flatsteps` FlatSteps; `handover` StageHandover; `stagemove` StageMoveSessions; `stagesettings`
  StageSettingsHeadless/ManualSession/Editor/OtherProvider; `stepagent` StepAgentKey;
  `automation` AutomationRun, AutomationStep, RunOutcomeAutomation; `schedule` ScheduleADE,
  SchedulePickerListsTasks; `errors` RunWithoutClaudeOnPath, RunClaudeExitsNonZero,
  WorkflowYamlInvalidThenFixed, StaleWorkflowSaveReportsCatalogText; `claude_missing`
  LaunchWithoutClaude; `llmprompts` RunUsesEditedFinishInstruction, RunDialogPreview;
  `panelforward` PanelForward, PanelForwardHeadless; `spaceops` AgentCancelsTaskOperation;
  `opslog` OpsAde, OpsBackgroundDedupe, OpsCheckRefFormat.
- Sessions: `sessions` InteractiveSessions, RestartSession, CloseSession, RunningSessionsList,
  StageSessionResume; `sessionstatus` SessionStatusBackgroundAndUnseen; `adhoc` AdHocSession;
  `resume` ResumeAfterRestart, ResumeLostConversation, ResumeRefused.
- Review: `review` ReviewOpenFacts; `review_working` ReviewWorking,
  ReviewWorkingRefusesUncreatedBranch; `review_working_review`; `review_uncommitted`;
  `review_removed`; `review_rereview` ReviewRereviewOnEdit; `review_sync` ReviewSync,
  ReviewSyncStackedBase; `reviewconvert`; `reviewforeign` ReviewForeignChangeRequest; `reviewplan`
  ReviewPlanDay; `reviewstep` ReviewStepWaits, ReviewSendImplementer, ReviewSendNew, ReviewFinish,
  ReviewLostFallback, ReviewQuitResume, ReviewRefusals, ReviewForwardComments; `capturereview`
  CaptureChangeRequestForReview.
- Orchestrator: OrchestratorSpawn, Pass, ChildChain, ToolGrants, ChildChainAutomaticFinish,
  ArchivedChildStepName.
- Sandbox: SandboxStepWrite, ReadStep, Off, Denial, NotEnforced, PrepareFails, Tools, ReviewRound,
  SmartRunSandboxed, ScriptStep, ManualSession.
- Workflows/config: `workflows` WorkflowsYaml; `workflowfiles` WorkflowFilesSync,
  WorkflowSaveWritesAtomicallyOnce, ConfigFilesOpenReveal; `workflow_editor` 4 tests;
  `steplibrary` StepLibrary, StepLibraryYaml; `agentsdoc` WorkflowsAgentsDoc; `toolnames`;
  `env` RepoEnvironments; `reposcript` RepoScriptPrepareRun(+Errors, ScriptDir, DeployedShaRun).
- Links/details: `artifacts` TaskArtifacts; `detailslinks`; `detailslist`; `detailsopen`
  DetailsOpenExternal; `hostlink` HostLink, ReviewAgentLinksPR, HostLinkUnauthenticated;
  `issuetracker` TaskIssueTracker, TaskIssueFastLoading; `trackedupdates` TrackedUpdates,
  TrackingSettings, TrackedIssueBatch; `notes` UnifiedNotes; `proofchain` ProofChain; `dbaudit`
  DbAuditTrail; `dbmcpstep` DbMcpStep.
- Depth: strongest package. Real repos and worktrees (101 git signals), bare remotes
  (`agentwork_test.go` compares `bare.Git("rev-parse", "refs/heads/feat/api-work")` to worktree
  HEAD after `ForcePush`), files (workflow YAML atomic write, agents doc), app DB, fake agent
  commits/MCP calls, `Browser.Opened` (details open), `FileOpener` (config files reveal). 276
  contract writes feed most `ade-v2-*` specs.
- IPC: `AdeTaskService` (787 calls), `AdeBoard`, `Leases`, `Tracker`, `IssueTracker`, `GitHost`,
  `ProofStore`, `AgentHooks`, `TermRegistry`.
- Fakes: fake `claude`/`gh`, `fakehost`, `fakeacli`, `sandboxtest.Fake` (never real Seatbelt),
  `WithTrackedUpdates`, `WithBranchRequestWait`, `WithRebaseTimeout`.
- Gaps: 27 files show no far-end signal (archived, branching, detailslinks, detailslist,
  flatsteps, folders_hidden, llmprompts, multiworktree, newtask, notes, proofchain, repoupdating,
  all six review_* window files, stagemove, stagesettings, statuses, stepagent, toolnames,
  workflow_editor, workflows, claude_missing, errors): board read-back plus contract only.
  Sandbox enforcement only through the fake enforcer; real enforcement only in
  `tests/claude` AdeSandbox. Text copies (copy task id, copy session id, Copy for agent) have no
  backend oracle. Two-window board interplay not covered here (windowflow covers terminals/tabs).

### agentflow (1 file, 1 test)

`agent_test.go` InfoNamesTheProvider: `AgentService` display facts, contract only. Gap: one
read-only check; provider switching lives in harnessflow.

### apiflow (11 files, 17 tests, edge-named 0)

- Scenarios: `collections` SavedRequestHistory, VariablePrecedence, PostmanImportSendExport,
  GrpcItemSaveAndRestart, PostmanImportCollectionAndEnvironment; `configfiles` ApiConfigFiles,
  ApiAgentsDocCoversSchemas; `history` HistoryClear; `lifecycle` ApiLifecycle; `order` ApiOrder;
  `traces` IncognitoLeavesNoTrace, DynamicValues, RestartKeepsApiState; `variables_form`
  ApiVariablesForm; `variables` EnvironmentFiles, ActiveEnvironmentResolvesBase; `agentaccess`
  ApiAgentAccess.
- Depth: files (7 ReadFile: config files, Postman export via `Dialogs.AnswerSave`), DB 2
  (incognito leaves no row), real `flowharness.HTTP` server for sends; 48 contract writes.
- IPC: `Collections`, `Variables`, `ResponseHistory`, `ApiConfig`, `ApiFiles`, `Http`, `Grpc`.
- Gaps: no edge-named tests; 6 files self-oracle only (agentaccess, history, lifecycle, order,
  variables_form, variables). Corrupt/invalid config file, secret store off, conflicting save hash
  not in names.

### appflow (16 files, 30 tests, edge-named 9)

- Scenarios: `appinfo` AppInfo, AboutGitHubLink; `clitools` CliTools; `experimental`
  ExperimentalMemory; `filehandlers` FileHandlersSetAndRestore, UnsupportedByDefault,
  SharedTypeRestoresFromSibling, StaleRowReplacedWhenFinderChangedHandler; `keepawake`
  KeepAwakeManualAndAgents; `layout` LayoutAdePanelWidth, LayoutCellEditorHeight,
  LayoutPanelSizesPerModule; `misc` DateFormatReachesGit, OpenLinks, TerminalLinkOpen; `ops`
  OpsLogAndCancel, OpsEveryGitSpawn; `quit` QuitConfirm, QuitWithoutLiveWorkAsksNothing; `settings`
  GitPathSettingEverywhere; `settingsops` SettingsCarryDatabaseSections; `studioapp` DataGripScan;
  `studioerrors` RelativeSQLitePathRefused, DeletedSavedQueryIsNotFound,
  TabsSaveWithoutWindowKeyIsContract, AdapterFailureKeepsDriverText; `update`
  UpdateRefusedInDevBuild, CheckForUpdatesRefusedInDevBuild; `window` WindowScopedState,
  QuitFlushHandshake.
- Depth: `Browser.Opened` (links, about), `KeepAwakeDriver.Held`, file handler registry fake, git
  spawn op rows; restarts in 5 files.
- IPC: `Settings`, `Layout`, `Ops`, `Lifecycle`, `Quitter`, `KeepAwake`, `Link`, `Update`,
  `FileHandlersSvc`, `DataGrip`, `Deps`.
- Gaps: update install path only refused-in-dev-build (no successful update flow); 8 files
  self-oracle (experimental, filehandlers, layout, settings, settingsops, studioerrors, update,
  window).

### captureflow (1 file, 12 tests)

- `capture_test.go`: BootRegistersDefaultChord, ChangeRequestLinks, IssueKeyAndLink, TextToLater,
  TodayLandsOnDay, ChangeRequestUnknownRepo, EmptyClipboard, SecondPressReplacesItem,
  LongTextTruncated, SettingsReRegister, WaylandRequested, RelaunchKeepsChord.
- Depth: fake OS hotkey (`Shortcuts.Fire`) and text clipboard (`Clipboard.SetText`) drive real
  `CaptureService`; result read on ADE board + popup `WindowManager.OpenedPopups`; 12 contracts.
- Gaps: board is the oracle (fine); OS registration failure only via `Shortcuts.FailNext`.

### claudeflow (2 files, 2 tests)

`hermetic_test.go` HermeticClaude; `isolation_test.go` ClaudeSettingsUntouched: fake HOME seeded
with user Claude settings, `os.ReadFile` asserts byte-identical after agent terminal and ADE
launches. Strong file oracle. Gap: no UI half by design.

### configflow (1 file, 1 test)

`secretref_test.go` SecretReferenceResolution through a test kind (`configfs`). ReadFile 1. Gap:
foundation only; no UI half, no contract.

### configsyncflow (3 files, 5 tests)

ConfigSync, ApplyRollsBackOnWriteFailure, RecoverRollsBackInterruptedApply, ConfigSyncRepos,
ConfigSyncReposWatchFolderRescan. Depth: git (fetch from bare), `os.Stat`/`ReadDir` for backup and
rollback; 8 contracts. Gaps: credential gate and conflict review covered inside `ConfigSync` only;
`repos_watch` self-oracle.

### coverage (1 test)

Name-match guard, see 2.1. Gap: call-site presence, not assertion quality.

### dbflow (10 files, 17 tests, edge-named 0)

- Scenarios: `sqlite` SQLiteJourney, SecretsStatusAndReveal, SchemaDDLSetGet; `postgres`
  PostgresJourney, PostgresUpdateCredentials; `kibana` KibanaDiscoverJourney; `kibana_saved`
  KibanaSavedQueriesJourney; `connection_form` ConnectionForm; `connection_lifecycle`
  ConnectionLifecycle; `connfiles` ConnectionConfigFile, ConnectionSecrets,
  ConnectionDuplicateOwnsSecret, ConnectionDuplicateKeepsRenamedFileSecret; `order`
  ConnectionOrder; `preconnect` SQLitePreconnectOp, SQLitePreconnectDelay,
  SQLiteTestPreconnectKilledOnQuit.
- Depth: `journey.run` (`sqlite_test.go`) covers create, Test (good and bad target), connect,
  browse tree, `Router.Execute` row count, saved queries, history, filters, masks, disconnect,
  duplicate/reorder/update, restart. Real Postgres/Kibana containers (7 docker signals, 3
  `Complete`). Connection files via ReadFile/Stat.
- IPC: `Connections`, `ConnectionFiles`, `ConnectionConfig`, `Tree`, `Queries`, `MaskRules`,
  `Filters`, `Schema`, `Router.Execute`, `Kibana`.
- Gaps: no grid edit/commit/insert/delete through the IPC boundary (no `Router` mutate op in any
  flow; writes proven only by adapter suites and UI `studio/mutations.spec.ts` on captures). Data
  view paging/sort/filter over the data stream not driven at flow level. MySQL, MariaDB, Mongo,
  Redis, S3, SQS, Kafka, ClickHouse have no dbflow journey (dbmcpflow engines cover reads for some).
  No edge-named tests.

### dbmcpflow (31 files, 102 tests, edge-named 25)

- Scenarios: approval flow, per-mode review, exemptions, expensive query (EXPLAIN), throttle,
  statement timeout, table allow-list, column mask review, browse (16 tests), scripts, API
  requests (16 tests: approved, blocked host, redirect blocked/no credential, unavailable, denied
  and replanned, variable changed, approval modes, tools toggle, environments, response caps,
  bodies, access rules, TLS), Kubernetes MCP, engine matrix (`engines_test.go` `engineTest` over
  real containers: Browse, Expensive, NoExplain, Masking, Modes, Throttle, Timeout*), focus,
  approval routes, op ids/guards, unreachable connection, edited prompts reaching a session.
- Depth: real MCP client over HTTP against the embedded server; SQLite target row checks
  (`mcp_test.go:143` counts `notes` after approval; `review_test.go:64` inserts marker);
  `McpInstaller.InstallCalls` (InstallAgentWithoutCli); 13 DB signals; real containers via
  `testsupport.Start*` (9 docker signals, 4 `Complete`).
- IPC: `DbMcp`, `ApprovalsSvc`, `Router.ReadLaneOpen/Cancel`, `PromptsSvc`, `Connections`.
- Gaps: 24 of 31 files self-oracle (assert MCP tool result text, approval items, contract) without
  checking the target DB after a write (only `mcp_test.go`, `review_test.go` do). Engine tests
  cover reads; write-mode refusal on non-SQLite engines not checked at target.

### dockerflow (11 files, 32 tests, edge-named 5)

- Scenarios: `containers` ContainerActions, ImagesVolumesNetworks; `disk` DiskUsage,
  ContainerSize; `edit` ContainerEditSpec, UpdateContainerInPlace, UpdateContainerRejects,
  UpdateStoppedContainer, RecreateContainer, RecreateStoppedContainer, RecreateRollback,
  RecreateRefusals; `errors` StartMissingContainer; `origin` ContainerOrigin; `registry`
  ImageRegistryURL; `restore` RestoreAfterEngineRestart, SnapshotSurvivesIdleEngine, AtLaunch,
  IgnoresUserStop, ShutdownCasualty, WaitingPolicy, DismissAndConflict, RealDaemonRestart;
  `status` StatusClasses; `streams` EngineEvents, LogsStream, ExecSession, StatsTwoWindows,
  WindowCloseEndsStreams, Contexts, QuitEndsStreams, ComposeGrouping.
- Depth: real engine (`RequireDocker`); `edit_test.go` inspects containers through `d.Cli` (13),
  `restore_test.go` (5), `registry_test.go` (3, Hub API stub via `WithDockerHubAPI`); 26 event
  waits; 7 `Complete`.
- Gaps: `containers_test.go` (start/stop/remove actions) and `streams_test.go` assert through the
  service and events, not `ContainerInspect`; skipped entirely without an engine.

### gitflow (25 files, 61 tests, edge-named 14)

- Scenarios: graph paging/refresh/fork order, stream cache resume, credit backpressure, per-window
  walks, commit detail/diff, blame, search, stash/checkout autostash/cherry-pick/revert/reset/undo,
  worktree add/remove, stack restack (+conflict), remote fetch/pull/push, force-push lease guards,
  push refusals, pull preflight, credential prompt (HTTP remote, routes, relay), remote cancel,
  repo deleted while open, push rejected non-fast-forward, worktree branch checked out elsewhere,
  repo settings in file, concurrency matrix (13 tests in `matrix_test.go`: undo slot,
  disconnect during remote op/credential prompt, two repos share nothing, stalled stream), bound
  `GitService` (bound, bound_review, bound_writes), open shapes, graph plane, perf baselines.
- Depth: strongest git oracle: 135 git signals; bare remote tips (`remote_test.go:88`
  `bare.Git("rev-parse", "main")`), worktree state files.
- IPC: git stream requests (`gitrpc`), `GitSvc`, `GitCredential`, `GitWindows`.
- Gaps: no restart test (persistence of repo settings/review across restart lives in
  reviewflow/journeyflow). `bound_test.go`, `graph_plane_test.go` self-oracle. Perf tests
  (`perf_test.go`) gated.

### githostflow (12 files, 25 tests, edge-named 2)

- Scenarios: `host` SecondGitHostRegistered, OriginPicksTheProvider; `hostview` Forge/Azure/GitHub;
  `comments` HostViewCommentsForge/Azure/GitHub, IssueComments; `review`
  ChangeRequestReviewActions, ReviewAgentLinksChangeRequest; `sync` ChangeRequestIndex* (8: cold
  and restart, delta, checks and deletion, account change, rate limit, deleted merged branch,
  delta overflow, timer walk); `azure` AzureRepos, AzureReposFailures; `issueview`,
  `issueview_fetch`; `githublinks` GitHubLinksInApp; `githubpropen` OpenGitHubPullRequest.
- Depth: `fakehost.Forge.Calls()` records posts and reviews; `fakeaz`, `fakeacli`, fake `gh`
  rules; `Browser.Opened` (3); 53 contracts.
- Gaps: comment post and review submit checked as recorded calls on the fake, not as host state
  read back on a second fetch in every case; `githublinks`, `issueview_fetch` self-oracle.

### grpcflow (3 files, 13 tests, edge-named 5)

UnaryMetadataAndStatus, ServerStreamAndCancel, ClientAndBidiRefused, TLSTargets,
SecretsInTargetAndMetadata, LargeMessages, DescribeReflection, DescribeProtoAndCall,
ReloadAfterSchemaChange, DescribeSilentServer, DescribeCancel, CallSilentServer, UnaryDeadline.
Real `flowharness.GRPC` server records metadata (far end); `Silent` targets. Gap: no restart
(history persistence in apiflow `GrpcItemSaveAndRestart`).

### harnessflow (6 files, 12 tests)

AgentContextReachesEveryLaunch, HermeticOpenCode, OpenCodeNotFound, OpenCodeProvider,
OpenCodeHeadless, AgentContextOpenCode, AgentPortClaude, AgentPortOpenCode,
SecondProviderRegistered, LeanProviderSeams, AgentSettingsReachEveryLaunch, AgentSettingsOpenCode.
Depth: 25 ReadFile (launch args/config written for the agent), DB 8, fake opencode. Good file
oracle.

### httpflow (3 files, 10 tests)

CurlReplayMatchesSend, SendResolvesSecretsOnlyOnTheWire, Redirects, CookieJar, TLSAndHTTPVersion,
ConcurrentSendsSharedJar, AuthHeaders, Bodies, TimeoutAndCancel, TransportErrors. Real HTTP/HTTPS
servers record wire requests (far end: secret only on the wire). Gap: no restart.

### journeyflow (3 files, 3 tests)

FirstRunJourney (import repo, graph subjects, ADE task, terminal, archive, relaunch: everything
reads back), RestartMidRun, RestartKeepsUserData. Git 3, Stat 1. Gap: only three cross-service
journeys; no Studio (DB/API/Docker) journey; no UI half.

### kubeflow (9 files, 19 tests)

Clusters, DescribePod, DescribeSecretHidesValues, DescribeClusterScoped, DescribeUnknown,
Namespaces, NamespacesRestricted, Kinds, List, ListTruncates, LogsTail, LogsOptions, LogsFollow,
LogsCloseWindowEndsOnlyItsStreams, LogsQuitEndsStreams, OpsRecordEveryAction, ClusterOrder,
Status, StatusExecPlugin. `KubeFake` API server with `Requests()`; writes one shared
`tests/contract/kubernetes.json`. Gaps: read-only module (no write ops exist); edge-named 1.

### llmpromptsflow (2 files, 5 tests)

List, Preview, SaveAndReset, SaveInvalid, SaveConflict. Self-oracle (no file read of the stored
prompt). Use of an edited prompt proven in adeflow `RunUsesEditedFinishInstruction`, memoryflow
`StoreUsesEditedGatePrompt`, dbmcpflow `EditedPromptsReachNewSession`.

### memoryflow (15 files, 49 tests, edge-named 10)

- Scenarios: distill (MCP and bridge, cross-checks, spend cap, cancel, to import), rewrite (6),
  check hook, search format/ranking/origin filter/score meta, lazy home, import lifecycle/retries,
  ingest (12: external, provenance doc write, locator heading, formatted doc parse, retired doc
  fails, verifies questionable facts, chunk cap, memdoc header claim, idempotent, runner outlives
  app), models, ops, provenance (4), semantic model install (checksum, cancelled), store through
  gate, rule, gate rejects/fails, semantic not installed, ConnectClaudeCode.
- Depth: doc write-back via ReadFile (15); fake agent prompt rules answer the gate; 49 contracts.
- Gaps: 5 files self-oracle (llmprompts, model, provenance, ranking, rewrite).

### mobileflow (10 files, 12 tests)

EnableTrustPort, PhonePairing, PhoneExpiredDevice, PhoneAndDesktopWriteRace,
PairedDeviceSurvivesRestart, PhoneEventStream, MobilePairingRoutes, PhoneReadsRealAde,
PhoneReadsArchivedTask, PhoneTerminalAttach, PhoneWrites, PhoneIdempotencyKeys. Real phone HTTP
server (`phone_test.go`, `terminal_test.go` use an HTTP client); loopback LAN fakes
(`MobileDetect`, `MobileFind`). Gap: phone write results read on desktop board (self-oracle);
mobile UI runs against a mock server, not this backend.

### notifyflow (3 files, 20 tests, edge-named 9)

StopNotifies, NeedsInputNotifies, SubagentNeedsInputNotifies, IgnoredNotifications,
WakeArmedStopSilent, FinishedWaitsForBackground, FocusedAppSkipsFinished, FocusedTabSuppresses,
Cooldown, PrefsToggles, AdeManualSessionNote, RunEndedNotifies, ClickRevealsTerminal,
ClickRevealsAdeSession, ClickRevealsTask, SendTest, AutomationNotifies, AutomationQuietCases,
ClickRevealsScriptRun, RecurringScriptNotifies. Hook events through the real shim over the hooks
socket; OS sink is `notifysink` recorder (far end). Strong.

### promptflow (2 files, 3 tests)

ScheduleConfirmRoutes, UpdatePrompt, NotifyPromptsSetting. Which window gets a popup. Thin.

### proofflow (9 files, 18 tests)

ProofCharts, ProofShotCopyAll, ProofShotClipLeavesNoFiles, ProofDiagram, ProofExplanation,
ProofImage, ProofImageRefusals, ProofImageEarlyCap, ProofImageSessionCap, ProofMenuOpensWindow,
ProofCloseAndRead, ProofNameCollision, ProofEditFlow, ProofRetentionAndDelete, ProofParallelSave,
ProofSaveIsAtomic, ProofWindow, ProofShotSaveAll. Far ends: `ImageClipboard.Writes()` bytes
(`clip_test.go:99`), shot files via Stat/ReadDir, `ProofShell` window records. Gap: 5 files
self-oracle (chart, diagram, explanation, image, proof).

### repoflow (10 files, 13 tests, edge-named 0)

RepoConfigFile, RepoConfigForm, EditFile, HideRepo, ImportViaFolderPicker, ImportEdgeCases,
RepoScriptPicker, RepoScriptRefFile, HeadsAndWorktreeLinks, RemoveRepoClosesSessions,
BrowseReadDiff, CodeSearch, CloseWorkspaceStopsSearch. Far ends: `EditFile` reads file back and
checks mode bits (`edit_test.go:60,90`); `Dialogs.AnswerDirectory` for import. Gaps: configfile,
configform, hidden, scriptpicker self-oracle; editing a file changed on disk (hash conflict) not
in names.

### reviewflow (6 files, 31 tests, edge-named 4)

Comments (9: ordered, export text, remove scoped/idempotent, clear keeps marks, add refusals,
shared across connections, refs changed keeps, kind/state export, edit), incremental marks (7),
ranged base resolution and walk (9: RangedWalkMatchesGitLog compares to real `git log`), removed
(2), session round trip, large file hunks, mark folder, navigation. 39 git signals. Gap: export
text checked as returned string; clipboard copy of export is frontend-only.

### termflow (13 files, 44 tests, edge-named 4)

- Scenarios: script runs (lifecycle, twice, non-zero exit, stop/close, window close, quit and
  restart, folders, order, forged command ignored, retention/remove), runs restore, runs config
  file/form/lifecycle, schedules (validation, headless, overlap, confirm, confirm secret, restart,
  run now, missing task, builtin var), scripts in picked folder/collections/update-remove, secret
  params (smart run, schedule, terminal run, resume, cleanup, storage off), smart scripts (space,
  failed, resume, rules), terminal (shell tab runs git, collections move/delete, agent tab keep
  awake, open refusals, window close kills only its shells, quit tears down, large output, Claude
  launch without claude).
- Depth: real PTY; `app.DB()` seeds/counts `script_runs`, `secret_values`; restarts in 8 files;
  78 contracts.
- Gaps: `runs_form`, `schedule`, `script` self-oracle; PTY output content asserted on a few
  scenarios only.

### usageflow (3 files, 7 tests)

StatusLineFeed, UserStatusLineFromAppSettings, StatusLineThrottle, UsageOffNoInjection,
ExpiredWindowDropped, SnapshotSurvivesRestart, RunStreamFeed. Generated `--settings` wrapper read
from disk (ReadFile 5); hooks socket. Good.

### windowflow (6 files, 10 tests)

OpenedFilesWindow, StudioWindows, QuitHandshake, DockerStatsEndWithTheirWindow,
LayoutWidthUserSetSurvivesRestart, TabsScope, TerminalFileLinks, TwoWindowsIsolation,
FocusSessionAcrossWindows, OpenNewWindow. `FileOpener` (38) and `WindowManager` recorders;
per-window events. Gap: studiowindows, tabs_scope self-oracle.

## 4. UI specs by area

Columns per area: specs; scenarios; oracle depth; IPC; gaps. "ipc-log" = asserts call args
through `control.log()`/`calls()`. Every UI spec is mock-backed; its far end is the mocked call
args at best.

### ADE board, tasks, branches, runs (`ade-*`, `ade-v2-*`, 85 specs)

- Specs: ade-agent-context, ade-agent-missing, ade-agent-port, ade-agent-settings, ade-agents-tab,
  ade-ids, ade-readable-names, ade-repos, ade-tracked-updates, ade-tui-takeover, ade-v2-add,
  -add-link, -agent-ops, -archive, -archived, -artifacts, -automations, -background-ops,
  -base-rebase, -board-coalesce, -branch-adopt, -branch-change, -branch-rename, -branch-request,
  -card-icons, -db-audit, -details-links, -details-list, -details-open, -dialogs, -flat-steps,
  -force-push, -handover, -issue-tracker, -later, -launch, -leases, -links-strip, -main-branches,
  -multi-worktree, -needs, -norepo, -notes, -op-kinds, -orchestrator, -pair-check, -panel,
  -panel-forward, -panel-tabs, -plan, -proof-chain, -repo-required, -repo-script, -repo-updating,
  -resume, -return-limit, -run, -sandbox, -session-status, -sessions, -sessions-menu, -shell,
  -shell-bar, -stage-move, -stage-resume, -stage-settings, -task-branches, -task-flow,
  -task-launch, -task-menu, -task-panel, -task-row, -task-status, -workflow-db-mcp,
  -workflow-layout, -workflow-steps, -workflows.
- Depth: most assert DOM plus ipc-log args (ade-v2-links-strip 37 log reads, ade-v2-review-open
  30, ade-v2-panel 23, ade-v2-run 23, ade-v2-workflows 25). Contract-fed through `support/adeV2.ts`
  helpers. Clipboard spy: ade-v2-base-rebase (Copy for agent), ade-v2-details-links (card copy),
  ade-v2-handover, ade-v2-sessions (Copy session ID, Copy log incl. 500-chunk paging),
  ade-v2-task-menu (Copy task id).
- IPC: `AdeTaskService.*` (create, update, shelve, set step, step done, rebase, force push,
  archive, restore, resume, take over, add link, remove artifact, forward/dismiss panel edits,
  handover), board pushes via `emulateAdeTaskPush`.
- Gaps:
  - Stop at chip/toast timing: ade-v2-op-kinds (17 toast refs), ade-v2-force-push (8),
    ade-v2-background-ops (9), ade-v2-agent-ops (8): they check chip and toast choreography plus
    call args; backend effect lives in adeflow (force push remote tip verified in
    `agentwork_test.go`; branch removal / stopRuns effect not traced to a remote).
  - Render-only (no ipc-log, no direct contract): ade-agent-missing, ade-ids, ade-v2-card-icons,
    ade-v2-main-branches, ade-v2-multi-worktree (1 test), ade-v2-pair-check, ade-v2-repo-script,
    ade-v2-repo-updating, ade-v2-review-removed, ade-v2-shell, ade-v2-shell-bar,
    ade-v2-task-panel, ade-v2-workflow-layout, ade-v2-stage-settings (contract only).
  - `ade-repo-updating` contract has no UI reader; `ade-v2-repo-updating` renders hand-built data.

### ADE review window (`ade-v2-review*`, `repo-review-interaction`, 10 specs)

- ade-v2-review (54 tests, 16 contract): three panes, diffs inline/side, comments, forward to
  agent, approve change request, PR comment post, moved head refusal, host reason; review-open
  (12), review-step (16), review-working (10), review-uncommitted (5), review-rereview (5),
  review-sync (4), review-removed (3), review-follow (2).
- Depth: ipc-log for `review.comment.*`, `AdeTaskService` review calls, host post args; clipboard
  spy in ade-v2-review (copy comments).
- Gaps: PR post success shown as UI state; host-side state verified only in githostflow fake
  calls. `git-review-edit` contract has no UI reader.

### Agents, providers, usage, notify (`agent-*`, `settings-agent*`, `status-agents`, 6 specs)

agent-provider (8), agent-usage (13), settings-agent (1), settings-agent-notify (6), status-agents
(10), ade-agent-port/settings/context above. Depth: ipc-log of settings writes. Gaps:
notification delivery only as `AgentNotifyService.SendTest` call; reveal routing via contract.

### Git module (`repo-graph-*`, `repo-commit-*`, `git-*`, `graph-blank-rows`, `consistency-git`, `copy-*-git`, `list-keyboard-git`, `ui-leftovers-git`, `repo-refs-popover`, `repo-resize`, `repo-floating-geometry`, ~38 specs)

- Scenarios: graph columns, lanes, paging, refresh, rewalk, badges, branch order, keep state,
  failures banner, change request badge/index, commit meta/detail (rename, binary, merge parent),
  refs popover, bound GitService reads/writes, credential relay prompt, graph plane stream credit.
- Depth: mostly DOM geometry and render; git stream mock answers; ipc-log rare (git-panel-hidden
  10, git-credential-relay 1). Bound specs (git-bound, git-bound-writes, git-bound-review,
  git-graph-plane) assert call shapes on the transport.
- Gaps: no UI spec checks a write's resulting graph against a flow-produced contract except
  repo-graph-refresh (branch behind) and repo-refs-popover (auto-stash entry). Contracts with no UI
  reader: git-graph-plane (UI uses own mock), git-review-edit. Checkout/stash/cherry-pick dialogs
  covered for wording (copy-coverage-git, copy-wording-git) more than for call args.

### Code hosts and issues (`host-view*`, `issue-view*`, `git-host-*`, `github-*`, 8 specs)

host-view (6), host-view-comments (8), issue-view (8), issue-view-fetch (3), git-host-azure (3),
git-host-provider (3), github-links-in-app (3), github-pr-open (2). Depth: contract plus ipc-log;
clipboard spy for Copy link (host-view, issue-view). Gap: comment post asserted as pending UI then
cleared; no read-back.

### Repositories, workspace, files (`ade-repos`, `repo-config-*`, `repo-file-*`, `repo-workspace`, `repo-script-picker`, `repo-order`, `repo-last-active`, `opened-files`, `terminal-file-links`, `git-panel-*`, ~16 specs)

Depth: ade-repos (20 log reads), repo-config-file (10), repo-file-edit (save sends hash; conflict
offers Refresh/Overwrite), repo-script-picker (contract), terminal-file-links (13, contract).
Gaps: file content on disk never visible (mock); `repo-file-edit` relies on repoflow `EditFile`
for disk truth.

### Runs, automations, terminal (`automations-*` + scenario modules, `terminal-links`, `script-order`, ~20 specs)

Scenario modules: automationsFile (8), Form (7), Lifecycle (4), Ops (9), Order (2), Recurring (7),
Restore (5), Resume (2), Runs (7), Secrets (4), Smart (9), Tabs (5); spec files: automations-mcp
(10), -module (6), -scripts (3), -runs-layout (2), -tabs (1), -recurring (1). Depth: contract-heavy
(termflow 78 contracts); clipboard spy for Copy for agent and Copy log (automationsOps,
automationsRuns, automations-mcp). Gap: terminal output content only from mocked chunks.

### Memory (`memory-*`, `settings-memory`, `operations-memory`, 9 specs)

memory-distill (13, 8 clipboard refs), memory-import (7), memory-ingest (1), memory-module (7),
memory-provenance (3), memory-ranking (2), settings-memory (7), operations-memory (3),
settings-experimental (2). Contract-fed. Gap: memory-ingest one test.

### Mobile settings and phone (`mobile-access` + `tests/mobile`)

mobile-access (12): trust network, QR, port, revoke, permissions, pairing prompt (contract). Phone
specs in §5.

### Capture, prompts, popups (`capture`, `settings-capture`, `prompts`, `update-dialog`, `quit-confirm`, 5 specs)

capture (11, ipc-log 12), settings-capture (5), prompts (6), update-dialog (8: install call
logged), quitConfirm scenario (2). Gap: update install success never reached in flows (dev build
refuses).

### App shell, settings, consistency (`window-chrome`, `modules`, `panel-resize`, `settings-*`, `consistency`, `copy-coverage`, `copy-wording`, `color-rails`, `dialog-surface`, `focus-ring`, `scrollbars`, `list-keyboard`, `ui-leftovers`, `tabs-scope-db`, ~20 specs)

Mostly render, geometry, keyboard and catalog-text checks; copy-coverage (35) and copy-wording
(18) assert catalog strings; settings-cli-tools (8, clipboard copy of commands), settings-file-
types (5), settings-llm-prompts (6), settings-updates (4), settings-about (2). Gap: these are
UI-only by nature; no flow pairing needed except settings writes (covered in appflow).

### Config sync (`config-sync`, `config-sync-repos`, 2 specs)

config-sync (5, toast 13: fetched commit toast offers Review/Dismiss, pull applies, conflict opens
review), config-sync-repos (3, clipboard: Copy for shared config writes backend text). Gap:
toast-led; apply result on disk only in configsyncflow.

### Operations log (`operations`, `operations-db`, `running-ops-*`, `settings-ops-merged`, 5 specs)

operations (17, Copy details/error/command clipboard), operations-db (10), running-ops-db (3),
running-ops-status (1), settings-ops-merged (2). Cancel sends id (ipc-log). Gap: operations-db no
ipc-log.

### Studio databases (`studio/connection-*`, `tree`, `console*`, `data-view`, `mutations`, `slick-grid`, `cell-editor`, `definition`, `sql-schema`, `autocomplete`, `grid-*`, `*-connection-drop`, `browse-*`, `kibana-*`, `datagrip-import`, `fake-data`, `mask-preview`, `row-coloring`, `preconnect`, `secrets`, `secret-reveal`, `credential-reveal`, ~55 specs)

- Depth: captured engine fixtures (`postgresFixture` etc.) through `mockStream`; ipc-log for
  connection lifecycle, tree (19), connection-files, connection-form (contract: form edit lands in
  YAML, Save sends text + hash, typed password via write-only call). Clipboard spy: console (17),
  autocomplete (8, Mongo copy document), slick-grid, grid-copy-menu (real clipboard in Chromium),
  data-view, interaction.
- Gaps: no live DB; mutations.spec, grid-commit-in-flight, document-new-save-double-submit verify
  staged/commit call args only; the matching backend write is proven only in adapter suites, not a
  flow test at the IPC boundary. Many big multi-step tests (`console`, `data-view`, `tree`,
  `mutations`, `interaction`, `fake-data` are single tests covering many steps), so a failure
  localises poorly. Contracts with no UI reader: connection-secrets, connection-files-migration
  (also no Go writer).

### Studio DB MCP and approvals (`studio/dbmcp-*`, `approvals-*`, `connection-mcp-*`, `connection-privacy`, `connection-mask-review`, `connection-throttle`, `connection-explain-threshold`, `studio-mcp-*`, `approvals*`, ~30 specs)

Contract-fed approval dialogs (dbmcp-approval, -approval-script, -approval-mode, -preview,
-review-mode, -review-connection, -script-long, -unreachable, approvals-api, approvals-exemption,
approvals (Space)). Depth: answer by item id (ipc-log). Gaps: many connection-mcp-* specs render
only (connection-mcp-access, -expensive, -tables, -timeout, connection-mask-review, -privacy,
-explain-threshold have 0 ipc-log).

### Studio proof (`studio/proof-*`, `ade-v2-proof-chain`, 7 specs)

proof-window (15, clipboard image spy: copied screenshot matches screen), proof-chart (5),
proof-diagram (5), proof-image (4), proof-copy-all (4), proof-ready (2). Strong pairing with
proofflow (`ImageClipboard.Writes`).

### Studio API/HTTP/gRPC (`studio/api-*`, `http-*`, `grpc-request`, `collections`, `script-order`, ~20 specs)

api-lifecycle (19 log), api-order (19), http-request (16), http-curl (9, contract command),
http-variables (secret stays literal in send args), http-pipes (masked reference reaches httpSend),
grpc-request (25), collections (8, contract). Gap: wire effect (secret resolved only on the wire)
proven in httpflow; UI checks args.

### Studio Docker (`studio/docker-*`, `list-keyboard-docker`, `*-docker-automations`, 8 specs)

docker-module (17, 27 log), docker-edit (8), docker-restore (8), docker-disk (4). Contract-fed.
Gap: engine effect only in dockerflow/e2e-real.

### Studio Kubernetes (`studio/kubernetes-*`, 5 specs)

kubernetes-module (9), -tabs (10), -order (6), -ops (3), -mcp (6); all contract-fed from
`kubernetes.json`.

## 5. Other tiers

### Visual (`tests/visual`, 27 specs, 88 files incl. snapshots)

Space: ade-links, ade-plan-branches, ade-repo-gate, ade-repos, ade-review-diff, ade-review-step,
ade-review-uncommitted, ade-sandbox, ade-sessions-menu, ade-workflow-graph (3), git-module (6),
memory, operations, settings (per pane), status-bar (3). Studio: approvals, automations-module,
connection-dialog, connection-privacy (2), console, data-view, docker-restore, http-request-view,
proof-window (9), schema-dialog, workbench. Pixel oracle only; mock-backed.

### IPC frontend (`tests/ipc`)

clickhouse, kafka, mariadb, mysql, redis (2), sqs: one long test per adapter on captured
fixtures (tree, filter-by-value quoting, data tab, console, stream tab). Backend half:
`internal/ipcfixture`. Gap: no postgres, mongo, s3, sqlite, kibana ipc-frontend spec (covered by
Studio UI captures instead).

### Contract (`tests/contract`, 214 files)

- Written by flows, read by UI and screenshots. No UI reader (half-split missing UI side):
  `ade-repo-updating`, `automations-mcp-app`, `connection-files-migration`, `connection-secrets`,
  `git-graph-plane`, `git-review-edit`, `link-open`, `script-folders`.
- No Go writer (stale, never regenerated): `connection-files-migration`, `link-open`.

### e2e-real (8 specs, 12 tests)

boot-real (real server, real repo import, graph), docker-real (compose container, logs, exec,
stop), mariadb-real (native kind, keyset paging over `big_rows`; two kinds survive reload),
postgres-real (round trip; keyset paging forward and back), sqlite-real (connect, tree, rows),
multiwindow-real (terminal output per page; tabs per window), prompts-two-windows-real (confirm
popup in main window only), terminal-real (keystrokes in order). Gaps: not in PR CI; no ADE run
end to end through the browser; no clipboard, no file-on-disk or remote git check; no
API/HTTP/gRPC spec despite `flowServers`.

### Mobile (9 specs, 46 tests)

agents (5: Reply pastes into waiting session), consistency (2), copy-wording (4), insecure-origin
(1), pairing (7), tabs (6), terminal (8: key bar bytes, compose, resize, reclaim, release),
ui-leftovers (6), writes (7: idempotency key, shelf drag, stale move). Mock server only; contract
for `mobile-*`. Gap: not in PR CI.

### Real claude (`tests/claude`, 31 files)

ade (AdeHeadlessRun, AdeStepResults, AdeManualSession, AdeAdHocSession, AdeRebaseRun,
AdeReportArtifacts), agentsettings, automation (SmartScriptRun), branchrequest, context, dbmcp
(7), hermetic, hooks (TerminalAgentHooks, HookPayloadContract, HookBackgroundTasks,
ShimInertForPlainClaude, HookStopBlock), kube, memory (6) plus memory eval suite (eval, judge,
corpus, footprint, retrieval, p291), memory_docs, memory_gate, memory_import, memory_ingest,
orchestrator, port, proof (2), resume (3), review (2), sandbox (AdeSandbox, AdeSandboxReadOnly:
real enforcer), settings (RealClaudeSettingsUntouched), stagesettings (2). Far end: real CLI
behaviour, files, git. Opt-in only.

### Perf (15 specs)

graph-scroll, heap-ram (45 parametrised), heap-tabs (9), Studio: console-grid-scroll,
docker-scroll, documents-flick, documents-scroll, grid-scroll, http-response(-body),
parse-callers (7), proto-grid-scroll (2), tree-scroll. Tripwires, not correctness.

### Screenshots (25 shots)

space.shot (5), build/branchesHandover (5), dbPrompt (3), proof (3), review (1), startTask (3),
studio.shot (5). Site imagery; no assertions.

### Unit (bun, 126 specs)

Space: ade-v2 logic (dialog, progress, plan model memo, board parity, task order parity, workflow
graph, slow notice, dropplan, refetch gate, timeline), repo/git (review diff tab, holds, blame
controller, workspace close drops caches, tab slots, tree sort, go-to-file, file tab revision,
transport bound), host-view markdown, notes markdown, mock-runtime-bindings, go-ts-vocabulary
parity, search insert reactivity, editors shared model refcount, terminal-file-paths. Studio:
100 specs on grid selection, SQL split/lint/tokens, explain, ejson, fake data bounds, console
races, api variables races, tabs save, mask parity, ipc-fixture-sync.

### Proto grid (6 specs)

edit-veto, header-sort, nav-button, null-vs-empty, parity, row-selection-copy on the prototype
grid page.

### Adapter conformance (`internal/adapters/*`)

Test counts: clickhouse 25, kafka 34, kibana 9, mongo 77, mysqlfamily 19 (mysql + mariadb), postgres
53, redis 65, s3 42, sqlite 16, sqs 33, testsupport 7. Each kind: `authmatrix_test.go` (complete
matrix: root vs least-privilege, password, database field, error handling), client/catalog/
tablerefs/console tests, real container reads and writes (`testsupport.InsertDeleteRoundTrip`),
1M-row paging. Package-level: abort, classify, conditions, connguard, connset, consolecap, errors,
rowops, sqlmask, sqlmutate, sqltext. Far end: the real database. Gap: sqlite 16 tests (lowest of
the SQL kinds); write proof never crosses the IPC boundary (see dbflow gap).

## 6. Cross-cutting gaps

1. Text clipboard has no backend oracle; only the UI spy sees it, and only in specs that install
   it (26 call sites). Real clipboard read only in `ui-clipboard` for grid copy.
2. UI specs never see files, DB rows or remote git: they stop at mocked call args. 134 of 329 spec
   files read no control log, so they assert render only.
3. Toast-led specs (ade-v2-op-kinds, -force-push, -background-ops, -agent-ops, config-sync,
   approvals 10 toast refs) assert choreography; backend outcomes rely on the flow half.
4. 2 contracts stale (no Go writer), 8 contracts unread by UI.
5. Studio data writes (grid commit/insert/delete, document save) have no flow test at the IPC
   boundary; adapter suites prove them below the router.
6. Flow tests with self-oracle only: about 110 of 296 non-helper files (list in §3 gaps).
7. e2e-real and mobile Playwright suites run locally only; complete flow suite and claude suite on
   demand only.
8. Sandbox enforcement in flows is always the fake (`NotEnforced:true`).
9. Few cross-service journeys: journeyflow has 3; none spans Studio modules or ADE plus git host
   plus remote push end to end.
10. Packages with no edge-named tests: apiflow, dbflow, repoflow, windowflow, journeyflow,
    agentflow, configflow, claudeflow (journeys may hold edge cases inside).
11. Single large UI tests (`studio/console`, `data-view`, `tree`, `mutations`, `interaction`,
    `fake-data`, `smoke`, every `tests/ipc` spec) mix many steps per test.

## 7. Summary table

| Area | Flow tests | UI specs | Far-end oracles | Gap notes |
|---|---|---|---|---|
| ADE tasks/branches/runs | adeflow (152) | 77 `ade-*`/`ade-v2-*` | git worktree + bare remote, files, app DB, fake gh/claude, Browser, FileOpener | 27 files board-only; toast-led UI specs; copy actions frontend-only; fake sandbox |
| ADE review window | adeflow review_* (17), reviewflow (31) | 10 | git log compare (reviewflow), fakehost calls | review_* adeflow files no far end; `git-review-edit` unread |
| Agent providers/hooks | agentflow (1), harnessflow (12), claudeflow (2), usageflow (7) | agent-provider, agent-usage, ade-agent-* | settings files byte-identical, launch config files | agentflow one test |
| Notifications | notifyflow (20) | settings-agent-notify | notifysink recorder | UI sees SendTest only |
| Git module | gitflow (61) | ~38 repo-graph/git specs | local + bare remote git | no restart; UI mostly render; `git-graph-plane` unread |
| Code hosts/issues | githostflow (25) | 8 | fakehost/fakeaz/fakeacli calls, Browser | post/review as recorded call only |
| Repos/workspace/files | repoflow (13), windowflow openedfiles/terminallinks | ~16 | file content and mode bits, Dialogs | hash-conflict edit not in flow names |
| Runs/automations/terminal | termflow (44) | ~20 incl. 12 scenario modules | app DB rows, PTY, restart | `script-folders` unread; PTY content thin |
| Memory | memoryflow (49) | 9 | doc write-back files | 5 files self-oracle; memory-ingest 1 UI test |
| Mobile | mobileflow (12) | mobile-access + 9 phone specs | real phone HTTP server | phone UI on mock server; not in CI |
| Capture | captureflow (12) | capture, settings-capture | fake hotkey/clipboard into board | board is oracle |
| Prompts/popups | promptflow (3) | prompts, update-dialog, quit-confirm | WindowManager | update success untested |
| App shell/settings/layout | appflow (30), windowflow (10) | ~20 | Browser, KeepAwake, FileOpener, WindowManager | many UI specs render-only by nature |
| Config sync | configsyncflow (5) | 2 | git + backup files | UI toast-led |
| Config as files | configflow (1) | none | file | foundation only |
| LLM prompts | llmpromptsflow (5) | settings-llm-prompts | none direct; use proven elsewhere | stored file not read |
| Studio DB connections | dbflow (17) | ~55 + 6 ipc | real Postgres/Kibana/SQLite reads | no grid write at IPC boundary; 8 kinds no journey; no edge names |
| DB MCP/approvals | dbmcpflow (102) | ~30 | MCP over HTTP, SQLite row count, InstallCalls, containers | 24 files no target-DB check; connection-mcp-* UI render-only |
| Proof | proofflow (18) | 7 | image clipboard bytes, shot files | 5 files self-oracle |
| API/HTTP/gRPC | apiflow (17), httpflow (10), grpcflow (13) | ~20 | real HTTP/HTTPS/gRPC servers record wire | apiflow no edge names |
| Docker | dockerflow (32) | 8 | real engine (`d.Cli` inspect in edit/restore) | actions/streams via service only; skips without engine |
| Kubernetes | kubeflow (19) | 5 | KubeFake requests | read-only module |
| Operations log | appflow ops, adeflow opslog, kubeflow ops | 5 | op rows via service | operations-db no ipc-log |
| Cross-service journeys | journeyflow (3) | none | git, restart read-back | no Studio or host-push journey |
| e2e-real | n/a | 8 specs | real server, real DB containers, real engine | local only; no ADE/clipboard/file checks |
| Real claude | tests/claude (31 files) | none | real CLI, real sandbox | opt-in |
| Adapters | adapter suites (~380 tests), ipcfixture (6) | ipc-frontend (6) | real databases | sqlite thinnest; no IPC-boundary write |
