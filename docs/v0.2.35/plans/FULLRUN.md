# Full suite run, v0.2.35

Resumable log. Env: 4 cores, CGO_ENABLED=1.

## Go (`rest` split: all except adapters, ipcfixture, dockerflow, dbflow)
- 142 packages ran; 1 failure.
- F1 `apps/kira-space/internal/flows/adeflow` TestAdeIDs: contract `ade-ids.json` held unmasked timestamps (addedAt, startedAt, createdAt, ...). Fix: mask the `*At` keys in the test, regenerate fixture. Commit "test: mask timestamps in ade-ids contract". Rerun x3 green.
- `apps/kira-studio/internal/flows/gitflow`: no such package in Studio; Space `flows/gitflow` passes (36.9s).
- F2 `apps/kira-studio/internal/flows/dockerflow` TestRestoreShutdownCasualty / TestRestoreDismissAndConflict flaked ~1/50: `kill` returned before the container stopped, a tick saw it running and swallowed the boot change. Fix: wait for stopped state in `restoreEnv.kill`. Commit "test: wait for killed container to stop in restore flows". Rerun: 10 min of -count=60 loop, zero failures (hit default 10m timeout, no test failed).
- Not run (real-container): internal/adapters/*, internal/ipcfixture (fail without containers: Kafka etc.), dbflow real DBs (ran, ok 6s).

- Env: `bun install --frozen-lockfile` needed (nanoid, uuid missing in node_modules); bindings regenerated (both apps) for AgentAccess. Then typecheck green, lint green, lint:dead green.
- F3 `apps/kira-studio/tests/unit/bridge-unwrap.spec.ts` sweep: apiConfigForm maps arg 3 before unwrap, threw TypeError with string args. Fix: pass array as arg 3. Unit: 2013 pass, 0 fail (194 files).
- Studio UI (ui + ui-clipboard, 767 tests): 4 failed first run, all fixed:
  - F4 cell-editor.spec UUID regex expected v4; generator is v7 since P523. Spec now expects v7.
  - F5 mask-preview.spec asserted class is-active on toolbar-mask-preview; button is shadcn with aria-pressed. Spec asserts aria-pressed.
  - F6/F7 grid-copy-menu (ui-clipboard) env: chromium_headless_shell-1248 missing; installed with `playwright install chromium-headless-shell`. Specs pass.
  - Rerun of cell-editor, mask-preview, grid-copy-menu: 10 passed.
- Space UI (ui, 1139 tests): all passed first run (31.4m).
- Mobile (mobile-ios + mobile-android, 84): 83 passed, 1 skipped.
- Studio ipc-frontend (+visual): ipc-frontend all pass. Visual baselines stale from this chapter's intended UI:
  - F8 Studio visual settings-database-mcp: pane gained 'Bring Kira Studio forward for approvals' and 'Let agents send API requests' (P520). Re-recorded.
  - F9 Space visual workflow-graph x5 (review, graph, branches, manual, script): step panel gained 'Save to library' (P526). Re-recorded. Fonts: fc-match sans-serif = Inter.
  - Rerun without update: all pass (Studio settings, Space visual 27).
- Space e2e-real (4): prompts-two-windows-real failed; F10 spec used pre-file-dialog testids (script-dialog, script-dialog-tab-schedule, script-dialog-save) and expected Save to close the dialog; now run-file-dialog, run-file-tab-schedule, run-file-save, then Close. Fixed in both apps' copies. Space rerun passes; other 3 passed first run.
- Studio e2e-real (10): 6 failed. F11 sqlite-real, multiwindow-real, postgres-real x2, mariadb-real x2 drove the pre-v0.2.28 connection dialog (connection-dialog, connection-name, connection-save...). The dialog now writes engine defaults and a file. Fix: new support/connections.ts creates via ConnectionsService.Create with the full config; specs use it. Rerun of sqlite, multiwindow, postgres, mariadb, prompts-two (7 tests) passed. Remaining Studio e2e-real (docker-real, terminal-real, prompts-two) passed first run. Note tests/perf/documentsFixture.ts still uses the old dialog (perf, out of scope).
