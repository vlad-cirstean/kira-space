# P627 journeys: cross-module working days

Imagined usage and definition of correct. Conventions: `journeys-studio-connections.md` head. Inputs:
`inventory.md` "Cross-module interactions" and "Bug classes"; `coverage-map.md` section 6 gap 9 (few cross-service
journeys; none spans Studio modules or ADE plus git host plus remote push).

Each journey crosses at least two modules and ends on a far-end oracle. All `new` unless stated.

### J-X-001 Bug report to merged fix in one sitting

- Status: new (parts: `journeyflow` FirstRunJourney, `adeflow` AgentCommitReachesBoard).
- Persona: developer handling Jira bug ABC-77.
- Pre: repo with bare remote, fake host with PR support, fake acli, Postgres connection "staging", workflow
  implement -> review -> finish, `db_mcp` on implement step.
- Steps:
  1. Copy `ABC-77`; capture hotkey; Cmd+Enter: task lands Today with Jira link.
  2. Open issue view from link strip; read; open task; add repo; Start.
  3. Agent (fake): `task_info`, reads DB through session MCP (`run_query mode=read`, approval set allow), tries a
     write (prompt): user denies; agent commits fix; `report_artifacts` PR.
  4. User opens review window; comments 2 lines; sends to agent; agent commits again; review marks files; Approve
     PR from review header.
  5. Push happened from task; PR merged on fake host; fetch; task shows merged; Finish; archive.
- Oracle: bare remote branch tip equals worktree HEAD; `Forge.Calls()` has PR approve; target DB unchanged (denied
  write); `ade_db_audit` has read and denied rows; op log shows git, acli, MCP rows for the task; archived task
  keeps notes, audit, proof; after restart everything reads back.
- Risk: any seam between capture, tracker, ADE, MCP, review, host.

### J-X-002 Query result into API request into terminal

- Status: new.
- Steps:
  1. Studio console: `SELECT id, email FROM people LIMIT 3`; select `id` column cells; Cmd+C.
  2. Api module: new request; paste first id into URL path var; paste all three into JSON body array (manual);
     Send to echo server.
  3. Copy response body; open terminal; paste into `cat > /tmp/x/resp.json` heredoc; Ctrl+D.
  4. Open `/tmp/x/resp.json` via terminal file link in files window.
- Oracle: wire recorder path id equals DB id; file on disk bytes equal response body; files window shows same
  content; clipboard at each hop exact (no trailing newline added by grid copy, no CRLF conversion).
- Risk: clipboard mutation between modules.

### J-X-003 Masked data stays masked everywhere it travels

- Status: new.
- Steps: connection with mask rules; agent in ADE step reads `email` via session MCP; result goes to handover note,
  proof table, step log, phone "Need You" preview, notification text, op log detail; user copies each surface.
- Oracle: real email substring absent from notes DB, proof store, logs, phone API JSON, `notifysink`, `op_log`,
  every clipboard copy; masked token equal across surfaces (deterministic).
- Risk: leak at a surface that renders raw tool output.

### J-X-004 Secret hygiene across launch, run, log, phone

- Status: new (open items pinned: live terminal, phone ring).
- Steps: Runs script secret param; ADE script step uses it; smart script uses it; agent MCP credential; run each;
  view step log, run tail, Ops detail, Copy log, phone terminal ring, notification.
- Oracle: masked in every stored/log/clipboard surface; exceptions exactly those in Known open items; test pins
  exceptions so later fix shows.
- Risk: masker missing for a new surface.

### J-X-005 Runs script as prepare hook, ADE step, schedule and MCP tool at once

- Status: new.
- Steps: script "migrate" referenced as repo prepare script, as ADE script step, scheduled every hour, exposed to
  MCP; trigger all four within 1 s.
- Oracle: concurrency rule explicit: one run per script key (`E_CONFLICT` to the rest) or per-context runs
  (assumption: prepare/step runs are separate contexts; pin actual); each caller gets clear result; no interleaved
  logs in one tab; `script_runs` rows match.
- Risk: conflated runs across contexts.

### J-X-006 Approvals storm from three modules while typing

- Status: new.
- Steps: user types in console; within 3 s arrive: db.query (ADE step), script.run (external MCP), branch.request
  (other task), kube.read, api.request; window raise setting on.
- Oracle: typed characters all land in console buffer (none eaten as dialog keys); one dialog, 5 items oldest
  first; none approved by stray Enter (Deny focus plus guard); each owner gets its own answer.
- Risk: keystroke approves unseen item.

### J-X-007 Quit with everything running, relaunch, restore

- Status: partial (`journeyflow` RestartMidRun; `termflow` restore; `dockerflow` AtLaunch).
- Steps: live: 2 agents, 1 script, 1 recurring schedule due in 1 min, DB query, HTTP send, kube log follow, docker
  log follow, staged grid edits, dirty file buffer, unsaved request draft, pending approval; Quit (confirm);
  relaunch.
- Oracle: all child processes gone; restore offers: agents resume, script rerun, docker (if engine restarted);
  accepted losses exactly: staged edits, dirty file buffer; request draft per tab persistence (pin); no pending
  approval resurrected; no schedule catch-up; DB server has no leftover query; tabs restored per window.
- Risk: zombie processes, phantom state.

### J-X-008 Shared repo list between Git and ADE under change

- Status: partial (`repoflow`; bug P190).
- Steps: with ADE never opened this session, import repo in Git module; open ADE Add popover (repo listed); hide
  repo in Git panel; ADE picker reflects; remove repo with ADE task using it (refused or task flagged).
- Oracle: one repo list for both; removal closes sessions and leaves task with clear missing-repo state.
- Risk: list synced only while ADE subscribed.

### J-X-009 Review window, git graph and host view agree on PR state

- Status: new.
- Steps: PR head moves on host (new push); open git graph (badge), ADE card PR line, review window header, host
  view; press refresh in one.
- Oracle: all four show same head sha/state after one sync (shared index); stale ones marked stale, never a
  different head presented as current.
- Risk: PR cache divergence (P189, P209).

### J-X-010 Config sync changes modules under the user's hands

- Status: new.
- Steps: user edits workflow (unsaved), run script form (unsaved), connection dialog open, request tab on
  collection; teammate change to each arrives; apply sync.
- Oracle: unsaved local drafts not clobbered; base-hash conflict on Save; modules reload other entries; running
  task keeps snapshot; live connection keeps working until reconnect.
- Risk: draft clobber on reload.

### J-X-011 Phone and desktop on same task

- Status: partial (`mobileflow` PhoneAndDesktopWriteRace).
- Steps: desktop drags task to Later while phone moves it to Today; phone sends TUI input while desktop types;
  phone stage Next while desktop opens stage dialog; desktop archives task while phone views it.
- Oracle: final board consistent on both (last write, one winner, loser told); PTY bytes not interleaved mid
  multi-byte sequence; stage advances once; phone shows archived read view.
- Risk: double stage advance.

### J-X-012 Proof from Studio MCP into task chain and screenshots for PR

- Status: partial (`proofflow`, `adeflow` ProofChain).
- Steps: agent in ADE step queries DB (masked), builds proof (table, chart), `attach_proof`; user opens from task
  Details; saves screenshots; pastes image into PR comment composer (no attachments: refused with hint).
- Oracle: proof table holds masked values; JPEGs on disk; composer states attachments unsupported, nothing
  uploaded (`Forge.Calls()` none).
- Risk: unmasked proof, silent drop.

### J-X-013 Terminal agent, Docker exec and Runs share terminal renderer

- Status: new.
- Steps: change data font and font scale in Settings with 4 terminal kinds open (shell, agent TUI, docker exec, run
  tab); resize; select and copy in each; Cmd+click link in each; Shift+Enter in each.
- Oracle: same behaviour in all four (one `terminalRenderer.ts`): clipboard equals selection, link opens, bytes
  `ESC CR`; font applied without restart; no tab lost output on refit.
- Risk: one host diverging from shared renderer.

### J-X-014 Kubernetes pod to Docker to DB during incident

- Status: new.
- Steps: kube logs show DB error; copy connection host from log; Studio: open connection, run diagnostic query;
  Docker: restart local replica; rerun query after reconnect; Ops panel shows trail; Copy details of all three.
- Oracle: ops rows in order with module facets; connection auto state after container restart: stale strip then
  Reconnect works; clipboard details exact.
- Risk: reconnect after local container restart.

### J-X-015 Experimental flag, provider switch and hermetic toggle mid-day

- Status: new.
- Steps: with sessions running on Claude, switch provider to OpenCode, toggle hermetic, enable memory; start new
  step, smart script, review agent.
- Oracle: running sessions unaffected; each new launch uses new provider/hermetic (launch files); memory MCP
  available to new launches only where configured.
- Risk: settings applied to some sites only.

### J-X-016 External file open while modal dialog is up

- Status: new.
- Steps: approval dialog open; Finder opens `.sql` file; terminal link clicked; update prompt arrives.
- Oracle: files window opens without dismissing or answering approval; prompts queue in order; no focus trap
  deadlock.
- Risk: prompt routing vs ephemeral window.

### J-X-017 Day of copy actions: every "Copy" in app

- Status: new (coverage gap 1: text clipboard has no backend oracle; 26 spy sites).
- Steps: walk every copy affordance: grid cell/row/column/JSON/CSV/INSERT, console Copy all, Mongo forms, tree
  name/path, DDL, definition column name, HTTP body/raw/curl, gRPC message, history diff, variable value (masked
  vs revealed), op details/error/command, run log, Copy for agent, session id, task id/title/branches, notes entry,
  handover, review comments as prompt, commit sha/message, branch name, file path, blame, PR link, issue link,
  kube describe/logs, docker logs, CLI tool commands, MCP install command/token, proof cells, proof images,
  config-sync text, memory distill.
- Oracle: per affordance: clipboard text equals source of truth read from backend (not DOM); no trailing
  whitespace/newline unless source has it; masked where source is secret; keyboard shortcut and menu item give
  identical bytes.
- Risk: recurring clipboard class; one inventory of all copy sites prevents drift.

### J-X-018 Offline laptop morning

- Status: new.
- Steps: launch with no network: git auto-fetch, PR index sync, Jira cache, update check, kube, remote DB, mobile
  server, embedding model download all fail; work locally (SQLite, local repo, terminal); network returns.
- Oracle: no modal error storm (each failure one quiet state); local features unaffected; on return each
  subsystem recovers without restart (auto-fetch resumes, index syncs on focus, stale Jira revalidates).
- Risk: backoff that never recovers.

### J-X-019 Large everything at once

- Status: new (perf tripwires exist per area).
- Steps: open 200k-commit graph, 1M-row table page 10000, 5 MB HTTP response, 10k gRPC stream, 400 containers,
  5,000 pods, 300 tasks, 20k-line run log in separate tabs; switch tabs round-robin 3 times; copy from each.
- Oracle: heap under budget; tab switch keeps state (filters, selection, scroll) per `KEEP_WHILE_OPEN` rules; copies
  correct after switches; no blank rows.
- Risk: state loss or leak across heavy tabs.

### J-X-020 Two windows, same entity edited in both, every module

- Status: partial (ui cross-window specs per module).
- Steps: for connection, saved query, mask rule, collection request, environment, cluster order, settings, workflow,
  run script, repo config, task: open in both windows; edit in A; observe B; edit stale B; save.
- Oracle: B refreshes or flags stale; stale save conflicts (base hash) where file-backed; DB-backed last-write
  rules stated per entity; never silent mixed state on disk.
- Risk: cross-window sync gaps.
