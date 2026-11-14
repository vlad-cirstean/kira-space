# P627 journeys: shell, settings, Automations, terminal, Operations, notifications, prompts, capture, mobile, memory, config files and sync, secrets, updates, lifecycle

Imagined usage and definition of correct. No implementation. Conventions: see `journeys-studio-db.md` head. Inputs:
`inventory.md` sections 1, 2, 16-18, 28-38; `coverage-map.md` (appflow, windowflow, termflow, notifyflow,
promptflow, captureflow, mobileflow, memoryflow, configflow, configsyncflow, llmpromptsflow).

## A. Shell, windows, tabs

### J-SHELL-001 Two-window working day with restart

- Status: partial (`windowflow` TwoWindowsIsolation, TabsScope; `appflow` WindowScopedState, QuitFlushHandshake).
- Persona: developer with Git on monitor 1, Studio on monitor 2.
- Steps:
  1. Window 1 Git module, 5 tabs (graph pinned, 2 files, diff, terminal). New Window: Studio, 4 tabs (table,
     console, HTTP request, Kibana).
  2. Preview tab by single click, make permanent by double click, drag reorder, Close others, Close to the right,
     Cmd+W on pinned graph (refused).
  3. Switch window 2 to Docker and within 100 ms close window 2 (native close flush). Reopen from Dock.
  4. Quit, relaunch.
- Oracle: `TabsRepo` rows per window key equal final strips (order, permanent flags), terminal tabs absent; mode of
  window 2 after reopen is Docker (flushed within debounce); after relaunch both windows restore own tabs and
  modes; ephemeral windows (review, proof, files, host-view) not restored.
- Variations: close last window (app hides, state kept), second close after re-show, 60 tabs with wheel scroll,
  tab payload at bound.
- Risk: lost write on close (P108, P116).

### J-SHELL-002 Keyboard and menu navigation across modules

- Status: partial (ui `modules`, `mode-switch`, `list-keyboard*`).
- Steps: Cmd+, Settings; Cmd+B panel; Cmd+J dock; Window menu next/prev tab in each module incl Kubernetes
  module-local tabs; command palette: type "new conn", Enter; Shift+Cmd+W; Escape closes palette not dialog under
  it.
- Oracle: each shortcut acts in focused window only; Window menu keys work on Kubernetes tabs (P493); palette
  command opens NewConnectionDialog.
- Risk: shortcut swallowed by Monaco/xterm focus.

### J-SHELL-003 Panel sizes per module and cell editor height

- Status: covered (`appflow` LayoutPanelSizesPerModule, LayoutAdePanelWidth); extend: drag-release outside window.
- Steps: resize left panel in Studio to 420, in Git to 260; release mouse outside window; switch modules; restart.
- Oracle: sizes per module persisted; no stuck drag state.
- Risk: drag-release fallback.

### J-SHELL-004 Status bar truth

- Status: partial (ui `status-agents`, `running-ops-status`, `agent-usage`).
- Steps: run query, fetch, script, 2 agents (1 needs input, 1 finished unseen); ops widget Stop on query; click
  agents item; blame item on file line; update banner.
- Oracle: counts equal real running sets (ops service, sessions); Stop cancels that op at far end (server query
  gone); item clicks focus right tab.
- Risk: stuck running op row.

### J-SHELL-005 Boot failures and single instance

- Status: partial (appflow; BootFailure island).
- Steps: launch second instance (lock): first focuses, second exits; corrupt `kira.db` (boot failure island,
  Retry after restore); schema too old (`SchemaTooOldError` message); read-only home dir.
- Oracle: no second process writing DB; backup file exists before migration; messages name cause; Retry works.
- Risk: silent data reset.

## B. Settings

### J-SET-001 Apply-on-save across panes with invalid draft

- Status: partial (ui `settings-apply-on-save`).
- Steps: change date format, font scale, row height, word wrap, data font to missing font (availability probe);
  enter invalid value in Capture hotkey pane; switch panes; Save disabled; fix; Save; Revert to defaults; restart.
- Oracle: `settings` table rows equal saved values; grid, graph, terminal reflect font/row height without restart;
  invalid hidden draft never re-enables Save (P103); defaults restored exactly; second window updates live.
- Risk: validator lost on pane switch.

### J-SET-002 Experimental memory flag

- Status: covered (`appflow` ExperimentalMemory; ui `settings-experimental`); extend: open tabs.
- Steps: enable flag (module appears), open memory tab, disable flag with tab open, restart.
- Oracle: module, settings pane, Distill selection hidden; `kira-memory` MCP stays registered; persisted mode
  `memory` falls back to default module on restart.
- Risk: restored hidden mode blank screen.

### J-SET-003 File types default app

- Status: covered (`appflow` filehandlers); platform unverified.
- Steps: set Kira default for `.sql`, `.json`; restore; shared type restore from sibling; Finder changed handler.
- Oracle: registry fake rows; restore returns previous handler exactly.
- Risk: previous handler lost.

### J-SET-004 LLM prompts editing

- Status: partial (`llmpromptsflow`; ui `settings-llm-prompts`). Far end new: stored file.
- Steps: edit finish instruction, remove required template var (refused), add unknown var (refused), save, reset,
  conflict with external edit of `config/prompts/` file.
- Oracle: file in `config/prompts/` equals editor; next run launch args contain edited text; reset deletes
  override file.
- Risk: invalid template breaks every run.

## C. Automations (Runs)

### J-AUTO-001 Script with params from form to finished run

- Status: partial (`termflow` script runs, secret params; ui `automations-*`).
- Persona: developer automating local deploy.
- Steps:
  1. New script "deploy" in collection "Ops": command with `$ENV`, params: text, select (dev/prod), multi-select,
     secret. Save (YAML tab check).
  2. Run: dialog asks params; secret saved (write-only). Output streams 20k lines with ANSI and progress bars.
  3. Select log text, copy; Copy log (all chunks); Copy for agent.
  4. Run again while live: focuses live tab (no second run). Stop. Rerun: finished tab replaced in place.
  5. Restart app during run: restore banner offers rerun.
- Oracle: `<home>/config/runs/<id>.yaml` has params, no secret value; `secret_values` row; process env had secret
  (script writes sha256 to file; compare); Copy log clipboard equals full log file with secret masked; run rows in
  `script_runs` with exit code; concurrent run refused `E_CONFLICT` at server; Operations panel shows finished run
  with 40-line tail.
- Variations: exit 137, binary output, 1 MB single line, working dir missing, script deleted while running.
- Risk: log clipped/not selectable (P577), copy log first page only.

### J-AUTO-002 Recurring schedule lifecycle

- Status: partial (`termflow` schedules).
- Steps: schedule cron `*/5 9-17 * * 1-5` timezone Berlin; preview next 3 fires across DST change; confirm popup
  mode: popup in main window only, approve; next fire overlaps running one (skipped with reason); app closed
  during 3 fires (missed, skipped); run-without-prompt mode with secret param; Run now.
- Oracle: fire times equal reference cron evaluation in tz; one run per approved fire; skip rows with reason;
  no catch-up storm at launch; schedule persisted in run YAML.
- Risk: DST double fire, catch-up storm.

### J-AUTO-003 Smart script outcomes

- Status: partial (`termflow` smart scripts).
- Steps: smart script with ADE task variable; run dialog previews task/branch; outcomes done, failed, blocked,
  cancelled; resume stuck after restart; notification click opens run.
- Oracle: report JSON matches schema; outcome stored; fake agent prompt contains variables; notification sink
  entry per outcome kind toggles.
- Risk: stuck run never resumable.

### J-AUTO-004 Collections drag and bad file

- Status: partial (`termflow` collections; ui `automations-order`).
- Steps: drag script between collections; Escape mid-drag; reorder; hand-edit YAML to invalid (last good kept,
  problem shown); delete file outside (row gone, folder stays: open item pinned).
- Oracle: files reflect moves; cancelled drag no write.
- Risk: cross-list drop on Escape.

## D. Terminal

### J-TERM-001 Shell tab working session

- Status: partial (`termflow` terminal tests; e2e-real terminal-real).
- Steps: open terminal on repo row; `cd sub && git status`; run `yes | head -200000`; resize; select 3 screens of
  scrollback and copy; paste 5 KiB multi-line; `nohup sleep 1000 &`; vim alternate screen; close tab; close
  window; quit.
- Oracle: PTY output hash equals expected for large output (no dropped bytes, coalesced writes in order);
  clipboard equals selection; after tab close shell process group gone incl nohup child after SIGHUP then SIGKILL
  2 s (process table); window close kills only its shells; tabs never persisted.
- Risk: hang on Close (P153, P294), dropped output.

### J-TERM-002 Links in terminal output

- Status: covered (ui `terminal-links`, `terminal-file-links`; `windowflow` TerminalFileLinks); extend: wrapped
  lines.
- Steps: URL wrapped over 2 lines, URL with `)` and `,` at end, OSC 8 link, path with spaces quoted, relative path
  after `cd`, path:line:col in compiler output.
- Oracle: `Browser.Opened` exact URL without trailing punctuation; file opens at position.
- Risk: trailing punctuation in URL.

## E. Operations panel

### J-OPS-001 Mixed-module op log audit

- Status: partial (`appflow` OpsLogAndCancel, OpsEveryGitSpawn; ui `operations*`).
- Steps: generate ops: git fetch (URL with credentials), DB query, HTTP send, kube list, acli read, MCP tool call,
  script run, memory-mcp row; filter facets module/connection; toggle background; open detail steps tree; Copy
  details; Cancel running; Re-run (Studio console op); restart.
- Oracle: `op_log` rows one per action with Origin/source; credentials redacted in row and clipboard; cancelled
  op ended at far end; Re-run opens console path; retention 30 days / 5000 rows enforced after seeding 6000;
  orphan running rows from crash become error on boot.
- Risk: stuck running row, credential in log.

## F. Notifications and prompts

### J-NOTE-001 Notification matrix while multitasking

- Status: covered mostly (`notifyflow` 20 tests); extend: click routing across windows.
- Steps: agent in window 1 tab A finishes while user is in window 2 (no note: app key); app in background (note);
  click note focuses window 1 tab A; needs-input while that tab focused (suppressed); cooldown; per-kind toggles;
  test button.
- Oracle: `notifysink` entries exact; click reveals right window/tab (`WindowManager.Focused`).
- Risk: note while looking at app (P359).

### J-PROMPT-001 Prompt routing with zero, one, two windows

- Status: partial (`promptflow` 3 tests; e2e-real prompts-two-windows).
- Steps: for each kind (schedule confirm, approval, git credential, pairing, update, masking review, proof ready,
  branch request, task create): trigger with origin window open; with origin closed; with no window (queued);
  open window later; system notification click.
- Oracle: popup appears once, in right window; queued prompt delivered on first window; answering in one window
  clears everywhere; pairing countdown re-armed per request.
- Risk: duplicate prompt in two windows, lost queued prompt.

## G. Capture

### J-CAP-001 Hotkey capture variants

- Status: covered (`captureflow` 12 tests; ui `capture`); extend: unicode and huge clipboard.
- Steps: clipboard: PR URL, Jira key in sentence, 2 MB text, empty, image only, text with NUL/RTL/emoji; press
  hotkey twice fast; Enter vs Cmd+Enter; Escape; change hotkey to conflicting chord in Settings.
- Oracle: task title truncated on rune boundary; second press replaces popup item; Escape adds nothing; conflict
  status shown and old chord kept or released as stated; board row lands Later/Today.
- Risk: rune-boundary truncation.

## H. Mobile web

### J-MOB-001 Pair phone and work from couch

- Status: partial (`mobileflow`; mobile specs on mock server). New: phone UI against real backend.
- Steps:
  1. Settings Mobile access: enable on trusted network, QR; phone opens URL; desktop pairing approval (countdown);
     approve.
  2. Phone: Need You list; reply to waiting agent (text with newline and emoji); Plan reorder; move to Later;
     capture text; stage Next with notes.
  3. Attach terminal: desktop shows overlay; type; Reconnect after phone sleep; release.
  4. Desktop moves stage meanwhile (stale-stage guard on phone write); repeated POST with same idempotency key.
  5. Laptop changes Wi-Fi (untrusted): server unbinds; back: rebind. Revoke device. Cookie expiry mid-request.
- Oracle: desktop board equals phone writes; PTY received phone input bytes once; idempotent replay creates one
  effect; untrusted network: port closed (connect refused); revoked/expired cookie gets 401 mid-session; CSRF
  missing gets 403; reloaded deep link stays on page.
- Risk: input lock deadlock, write race, stale stage.

## I. Memory (experimental)

### J-MEM-001 Store, search, ingest, distill

- Status: partial (`memoryflow` 49 tests; ui `memory-*`).
- Steps: agent stores facts through gate (accept, reject, update of earlier fact); search before embedding model
  installed (keyword only) and after; ingest folder of 200 docs with `.gitignore`, estimate confirm, quit mid-run,
  relaunch (runner outlives app, recovery); distill pasted text in rewrite mode, copy result; cancel model
  download.
- Oracle: store rows with lineage ids `km_`; ingest idempotent (second run adds nothing); written-back docs on
  disk carry provenance header; clipboard distill equals result text; cancelled download leaves no partial model
  file; no delete (open) pinned.
- Risk: duplicates across parts, scan vs auto-start race.

## J. Config files and sync

### J-CFG-001 Shared config GitOps round trip

- Status: partial (`configsyncflow` 5 tests; ui `config-sync*`).
- Persona: team shares workflows, runs, API collections, connections.
- Steps:
  1. Connect shared repo (bare). Teammate pushes new workflow, changed run, connection with literal password, and
     invalid YAML.
  2. Fetch: toast Review/Dismiss; preview per file; credential gate blocks literal-password file; invalid file
     blocked; deactivate one entry; apply.
  3. Local edit of same workflow entry then teammate edit of other entry (three-way merge per entry); conflicting
     edit of same entry (review).
  4. Kill app mid-apply; relaunch (boot Recover rolls back); apply again; Rollback from journal.
- Oracle: files under `<home>/config/*` after apply equal merge result; blocked files absent; backup dir holds
  pre-apply bytes; after kill disk equals pre-apply exactly; rollback restores backup bytes; modules reloaded
  (new workflow listed) without restart; local secrets untouched.
- Risk: half-applied state, credential on disk from shared file.

### J-CFG-002 `${NAME}` and `${secret:}` references across modules

- Status: partial (`configflow` SecretReferenceResolution).
- Steps: use `${HOME}` and `${secret:x}` in connection, run, api env, agent settings; secret missing; secret store
  unavailable (keychain locked); rename secret.
- Oracle: resolved only at use (wire, env), never written back resolved; missing ref gives named error at use,
  file stays loadable; generated `AGENTS.md` documents schema, no values.
- Risk: resolved secret persisted.

## K. Secrets and storage

### J-SEC-001 Secret never leaves sealed store

- Status: partial (`termflow` secret params; `dbflow` ConnectionSecrets).
- Steps: create secrets in 5 places (connection password, URI, API variable, run param, agent MCP credential);
  use each; grep whole `<home>` tree, logs, op log, history, run tails for the 5 values; reveal one with OS auth.
- Oracle: zero plaintext hits outside sealed blobs (documented exceptions pinned: response bodies, live terminal);
  AAD scope prevents decrypting one scope's blob as another (swap rows in DB: decrypt fails, clear error).
- Risk: leakage (P108 class).

### J-SEC-002 Migrations and backups on upgrade

- Status: partial (unit; `journeyflow` RestartKeepsUserData).
- Steps: start with v0.3.12-shaped home (seeded DB, config files, readable-names migration pending); launch;
  interrupt migration (kill); relaunch.
- Oracle: backup of DB exists; resumed migration completes; user data equal (tasks, connections, tabs); no file
  deleted on failed write (P482).
- Risk: migration deletes rows on failed file write.

## L. Updates, keep-awake, lifecycle

### J-UPD-001 Update check and install

- Status: partial (`appflow` update refused in dev; ui `update-dialog`). Success path untested.
- Steps: Check for updates (newer release on stub); dialog; Install with agents running (quit confirm first,
  assumption); network error (short cause, raw logged); dev build refuses.
- Oracle: installer script launched detached with expected args (recorded by fake), app quits cleanly after
  flush; error text has no raw response body.
- Risk: update kills running agents without confirm.

### J-LIFE-001 Keep-awake and quit gate

- Status: covered (`appflow` KeepAwakeManualAndAgents, QuitConfirm); extend: combined.
- Steps: toggle keep-awake manual; "with agents" setting; start agent, script, headless run; quit: confirm lists
  N; cancel; stop all; quit without prompt.
- Oracle: `KeepAwakeDriver.Held()` timeline; no caffeinate child after quit; quit count equals live work.
- Risk: assertion leak after quit.
