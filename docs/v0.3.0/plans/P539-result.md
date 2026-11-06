# P539 Result (draft)

Not copied into SPEC.md yet. Orchestrator marks the row Done and pastes this.

Studio home folded into Space home. One data home `~/.kira-space/`; `studio.db` beside `kira.db`.

- `internal/homefold`: only code naming `KIRA_HOME` / `.kira-studio`. `Files` (flock, move to
  `backups/p539-<ts>/kira-studio`, `VACUUM INTO studio.db`, copy `connections/`, `api/`, DB MCP tokens,
  merge prompt overrides) and `Runs` (Space-wins id map, one-transaction row copy, run files, automations
  dirs, delete copied Studio rows). Marker row `homefold.p539`, resumable at every step. `TestHomeFold`
  covers interrupt at each step, EXDEV, absent, empty, studio-running, LegacyHome rules, idempotency.
- Paths: `config.StudioHome`, `StudioDbPath*` gone; `ModuleDbPathAt`. Callers, harnesses (`flowharness`,
  `ipcfixture`, `testx`) use `KIRA_SPACE_HOME` only. `AppService.Info` reports one home. `appmerge`
  reads the Space home.
- One runs stack: second `scriptruns.Service`, loader, scheduler, `scriptLaunchers`, Studio routing in
  `bridge/{customscripts,scriptruns,dbmcp_scripts}.go` deleted. Studio script repos removed.
- `StudioConfigFilesService` folded into `ConfigFilesService` (`api`, `connections` registered); binding
  gone; apiflow/dbflow assert Open and Reveal resolve under `SpaceHome`.
- DB MCP: `mcpinstall.Registered`, `bridge.ReregisterAfterFold`, appwire goroutine clears the flag;
  `dbmcpflow` `TestDbMcpReregisterAfterFold`; real claude `TestDbMcpInstallRegisters` passes.
- Tests: termflow `studio{context,settings,hermetic}_test.go` deleted (twins in harnessflow/claudeflow
  cover the same service); `studiosmart` became `smart_rules_test.go`, `studioscript` became
  `script_test.go`. Contracts `automations-mcp-app-db`, `smart-db` deleted. 11 UI wrappers deleted,
  `automations-{mcp,module,runs-layout}` moved to `tests/ui` on the `test` fixture. One e2e-real fixture
  and project; `tests/e2e-real/studio/` gone.
- Docs: ARCHITECTURE (Home fold section, storage, two-homes and `kira.sqlite` open items removed),
  PACKAGING, DEV_ENVIRONMENT, PERF, README, install.sh, site pages.

Not run (chapter-end suites): full Go, UI, e2e-real. Moved e2e-real specs only listed.
