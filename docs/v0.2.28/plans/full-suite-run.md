# Full suite run (chapter end)

Base tip 14713a38. One full run, failures fixed, targeted reruns.

## Ran
- `bun run lint`, `typecheck`, `lint:dead`, `test:unit` (twice more at end; no repeat of the P477 one-off), `lint:go`.
- Go: `go-test-split.sh rest` and `containers`, `CGO_ENABLED=1`, no `-race` (DEV_ENVIRONMENT asks it only for the Docker module and mobile mock). Docker daemon down; container adapter tests skipped. No real-claude suites.
- `test:ui:space` (1023), `test:ui:studio` (655), `test:visual:studio` (21), `test:visual:space` (26), `site:check`.

## First attempt lost
First `go test -race ./...` filled the disk (cache 22 GB, 100% full), producing spurious failures. `go clean -cache` freed 24 GB; reran without `-race`.

## Failures and fixes
- biome `noExplicitAny` memProbe.ts:159: second `biome-ignore`.
- Go `TestGoConventions`: gitservice.go G1 (use `ipcerr.Wrap(fmt.Errorf)`), scriptstest G4 (`renameio.WriteFile`).
- Go i18n catalog test: key-parameter wrappers (scripts, repocfg, apifiles, connfiles) added to `wrappers`.
- Space UI: stale spec for removed script editor Switch (`script-use-ade-dir`) deleted; `use_ade_dir` covered by termflow run file test.
- Studio UI (5): create fixtures predated the new-connection dialog (`connectionCreateArgs`); `confirm-focus` reveal now from the file dialog secrets; tree menu list gained `open-file`, `reveal-file`, lost `color`, `readonly`. `DEFAULT_PORT` now drives `newConnectionArgs` (adds mariadb).
- Visual Studio: console, data-view, schema-dialog and connection-dialog specs drove the removed tabbed dialog; now use `createConnection`. Baselines re-recorded after viewing diffs: new-connection dialog (new), Runs restore (paths now by file name), Database MCP settings (copy change). Space repositories dialog re-recorded (repo config file UI). No baseline hid a real diff.
- site:check phone `visual.spec.ts` timeout: real, not environment. Reproduced on unchanged spec under load (9 of 30 timed out): WebKit leaves `img.decode()` pending, even on a loaded image. Spec now awaits image load/error; 30 of 30 under load. Spec is unchanged since main, so main behaves the same; no separate main worktree needed.
- `graphChunkFrame.*` testdata: no incidental rewrite present.

## Reruns
- Failing Studio UI specs, then all 36 Studio spec files sharing `support/connect` or `connectionArgs` (191 passed), visual both apps full (21, 26 passed), site visual idle and under load, bridge/gitflow/scripts/convention/i18n Go packages, lint, lint:go, lint:dead, typecheck (via hooks).
- No second full run: fixes touched test code and shared test support only, covered by the targeted reruns above.
