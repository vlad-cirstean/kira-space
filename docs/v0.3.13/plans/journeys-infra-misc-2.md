# P627 journeys: infra and misc, second pass

Continues `journeys-infra-misc.md` (ids continue). One scenario per journey. Conventions: `journeys-studio-db.md`
head. Far end: docker engine state (`docker inspect`, flow fake engine), `KubeFake.Requests()`, `notifysink`
notes, mobile HTTP responses and `mobile_devices` rows, files under `<home>` (`.sync/apply.json`, `backups/`,
`agent-usage.json`, `memory/memory.db`), `Browser.Opened()`.

Code and architecture facts used (CodeGraph reads, `docs/ARCHITECTURE.md` 2670-3283, 3749-3766):

- `internal/docker/edit_apply.go`: one edit per container (`lockEdit`, second gets Conflict "edit in progress");
  `loadForEdit` refuses when container changed since load (`BaseHash`, Conflict "changed since load"); in-place
  steps rename, limits (resources plus restart policy), networks run in order, stop at first failure, no
  rollback, error carries `step` and `applied`; recreate: create temp-named new, connect extra networks, stop
  old, rename old to `<name>-kira-old-<id>`, rename new, start; failure rolls back (remove new, restore name,
  restart old), code `RecreateFailed`, details `restored` and leftover containers; missing image NotFound;
  timeout 3 min; old container remove failure only warns "old container kept as ...".
- Docker module: log buffer cap 20,000 lines, wrap mode renders last 2,000; streams per window, run only while
  module visible; stats shared per container across windows; disk usage on demand only.
- `configsync/apply.go`: preview token single use, 10 min; apply refuses when fetched commit or any local target
  changed since preview (Conflict "changed"), expired token NotFound; conflicted row default choice mine
  (`Kept`), gate-blocked rows `Blocked`; journal `.sync/apply.json` (`applying` then `committed`); failure rolls
  all files back from `backups/configsync-<UTC stamp>`; boot `Recover` rolls back `applying`, finishes
  `committed`; interrupted journal blocks Preview and Apply with "interrupted"; old backups pruned to newest N.
- Agent notifications: kinds finished, needs-input, run-ended; finished suppressed while any app window is key;
  others suppressed while owning window shows that terminal or task; cooldown 10 s per (terminal or run, kind);
  note id `<kind>:<id>` replaces older; Stop after `Monitor`/`ScheduleWakeup` gives none; SendTest bypasses.
- Prompt routing: target = claimed window, else origin, else main (lowest open order), else queued; kinds
  schedule, approval, git-credential, mobile-pairing, update, handover; hidden prompt stays listed.
- Usage: feeds `session` (statusline) and `run` (stream-json); newest reading wins per window; passed reset
  drops window; persisted to `agent-usage.json` (0600); `agent.usageEnabled` off hides item and injects no
  statusline in later launches; tones at 80 and 95 percent; push at most 1 per 10 s.
- Mobile: off by default, port 7790, plain HTTP, binds trusted interface IPv4 only; Host must be bound IP
  literal; pairing 120 s timeout, 60 s deny cooldown per IP, 4-digit code; device expiry 30 days (401
  `E_EXPIRED`); permissions `can_write`, `can_agent_input` plus global `mobile.agentInput`; refusals 403
  `E_WRITE_OFF`, `E_AGENT_INPUT_OFF`; writes need `Idempotency-Key` (replay same response, LRU 1024, 10 min);
  stale guard `FromStepID` gives 409 `E_STALE`; second device on terminal 409 `E_TERMINAL_BUSY`; rate limits
  pairing 1 per 10 s burst 3, write 1/s burst 10; supervisor every 10 s stops server off trusted network.
- Memory: versions immutable (current/superseded), FTS prefix search, user input never FTS syntax; search limit
  default 40 max 200; embeddings optional (model download on explicit click, SHA-256 pinned); embedder failure
  leaves keyword results.

## H. Docker

### J-DKR-010 Edit running container in place: rename, limits, networks

- Status: partial (`dockerflow` TestUpdateContainerInPlace).
- Persona: developer tuning local stack.
- Pre: running container `api` on networks `front`, `back` (alias `svc`), restart `no`.
- Steps: 1. Open edit; change name `api2`, memory 512 MiB, restart `on-failure:3`, add alias `svc2` on `back`, detach
  `front`. 2. Review diff; apply. 3. Reopen edit.
- Oracle: `docker inspect api2`: name, `HostConfig.Memory` 536870912, restart policy and retry 3, networks only
  `back` with both aliases and same IP; result `Applied` lists rename, resources, restart, network:back,
  network:front; container never stopped (`StartedAt` unchanged).
- Risk: alias change dropping static IP; container restart on in-place edit.

### J-DKR-011 In-place edit fails midway

- Status: new (no-rollback rule read from code).
- Steps: 1. Edit name plus memory below current usage minimum (engine rejects) plus network change. 2. Apply.
- Oracle: error names step `resources`, details `applied: ["rename"]`; container is renamed, memory unchanged,
  networks unchanged; UI shows what landed and reloads draft from fresh inspect (no stale base hash on retry).
- Risk: UI claims nothing changed; retry hits "changed since load" loop.

### J-DKR-012 Edit conflict: container changed outside, and two windows editing

- Status: partial (`dockerflow` TestUpdateContainerRejects).
- Steps: 1. Open edit in window A. 2. `docker rename` from terminal. 3. Apply in A. 4. Reload draft (3-way merge
  keeps A's memory change). 5. Window B applies edit at same moment as A.
- Oracle: 3 Conflict "changed since load", nothing applied; 4 draft shows outside rename plus own pending change;
  5 exactly one succeeds, other Conflict "edit in progress"; final inspect consistent with one edit.
- Risk: lost update.

### J-DKR-013 Recreate with new image and failed start, automatic rollback

- Status: partial (`dockerflow` edit tests).
- Steps: 1. Edit `api`: image tag to one whose entrypoint fails at start; add env var and port. 2. Apply (recreate).
  3. Observe error. 4. Fix image; apply again.
- Oracle: after 3 container `api` is old id, running, same name; no `api-kira-new-*` or `api-kira-old-*` left
  (`docker ps -a`); error code `RecreateFailed`, step `start`, `restored: true`; after 4 new id, name `api`, env
  and port present, old removed; volumes and network aliases carried.
- Variations: image absent locally: NotFound naming image, nothing created.
- Risk: service left stopped with temp name.

### J-DKR-014 Recreate when rollback itself partly fails

- Status: new.
- Steps: 1. Recreate where new container start fails and old container start also fails (port now taken).
- Oracle: error details `restored: false` and list of leftover containers with ids and current names; UI shows
  them with actions; nothing silently deleted.
- Risk: user left with stopped stack and no pointer to pieces.

### J-DKR-015 Live logs: 20k cap, wrap, filter, copy, window switch

- Status: partial (`journeys-infra-misc` J-DKR; ui docker logs).
- Steps: 1. Container emits 50k lines with ANSI colour, CRLF, 100 KiB single line. 2. Filter text; toggle wrap.
  3. Select 300 lines across virtual window; copy. 4. Switch module 1 min; return. 5. Second window opens same
  logs.
- Oracle: buffer holds newest 20,000 with truncation note; wrap shows last 2,000 filtered; clipboard = selected
  lines in order without ANSI codes (pin) and with full long line; stream stopped while module hidden (engine
  connection count) and resumes without duplicate lines; windows independent.
- Risk: copy of virtualized range missing unrendered rows.

### J-DKR-016 Engine endpoint changes under the app

- Status: partial.
- Steps: 1. Use default context. 2. `docker context use remote-ssh` in terminal. 3. Refresh. 4. Select context in UI;
  engine goes down; comes back. 5. Exec terminal open during loss.
- Oracle: UI-selected context wins over CLI change; list shows containers of selected engine only; down state
  banner, no stale rows actionable; exec session ends with message, new session works after return; only
  non-secret context `meta.json` read (guard G5).
- Risk: action sent to wrong engine after context switch.

## I. Kubernetes

### J-K8S-010 Browse cluster with 5,000 pods and follow logs across restarts

- Status: partial (`kubeflow` discovery, list, logs).
- Steps: 1. Pick context; namespaces load. 2. List pods (5,000): scroll, filter by name, sort by age. 3. Describe one.
  4. Open logs follow; pod restarts (new container id). 5. Switch namespace while logs open. 6. Copy 200 log lines.
- Oracle: `KubeFake.Requests()` show list with limit/continue paging, not one unbounded call; filter/sort
  client-side keep selection by uid; log stream reconnects to new container or reports ended (pin); op log has
  one record per read with cluster and namespace; clipboard lines exact.
- Risk: selection jumps on refresh, log stream leak after namespace switch.

### J-K8S-011 Agent kube read needs approval per cluster

- Status: partial (`dbmcpflow`; J-K8S pass 1).
- Steps: 1. Agent calls kube list on prod context. 2. `kube.read` approval shows cluster, namespace, kind. 3. Deny.
  4. Agent retries on dev; approve with remember option. 5. Agent reads secret kind.
- Oracle: denied call returns denial text, zero requests to API server; remembered scope applies to dev only;
  secret values masked or refused per rule (OQ-INF-1); item id `kube.read:<ref>` answered once.
- Risk: remember scope too wide.

### J-K8S-012 Kubeconfig edge cases

- Status: new.
- Steps: 1. `KUBECONFIG` with 2 files and duplicate context names. 2. Context with exec credential plugin missing
  binary. 3. Expired client cert. 4. Kubeconfig edited while app open.
- Oracle: first definition wins as kubectl does; errors name cause (plugin not found, cert expired) without
  printing token or key material; edit picked up on refresh.
- Risk: credential material in error text or op log.

## J. Notifications and prompts

### J-NOTE-010 Notification matrix across focus states

- Status: partial (`notifyflow`).
- Steps: 1. Agent in terminal tab finishes while window focused on that tab. 2. Finishes while app in background.
  3. Needs input (permission prompt) while other tab active in same window. 4. Same while that tab active. 5. Run
  ends failed while review window is key. 6. Two finishes of same terminal within 5 s.
- Oracle: `notifysink`: none, finished (title names task, else repo, else folder), needs-input, none, run-ended,
  exactly one for step 6 (cooldown); ids `<kind>:<id>` so newer replaces older; body present only when
  `notifyIncludeMessage` on.
- Risk: notification spam, or silence when needed.

### J-NOTE-011 Agent waiting on Monitor does not notify, later really finishes

- Status: partial.
- Steps: 1. Agent calls `Monitor` then Stop. 2. Wake fires, agent works, Stop.
- Oracle: no note after first Stop; finished note after second; arm cleared by UserPromptSubmit.
- Risk: false "finished" for sleeping agent.

### J-NOTE-012 Click notification lands on right window and task

- Status: partial.
- Steps: 1. Three windows; task session lives in window 2 (minimised). 2. Click finished note. 3. Click run-ended note
  of archived task.
- Oracle: window 2 un-minimised and focused, terminal revealed (`kira:agent:revealTerminal` to that window only);
  archived task click opens archived view or no-ops with message, never wrong task.
- Risk: reveal sent to all windows.

### J-PROMPT-010 Prompt routing with windows opening and closing

- Status: partial (`promptflow`).
- Steps: 1. No window open (app in tray): approval raised. 2. Open window. 3. Open second window; raise git
  credential prompt from window 2's stream. 4. Close window 2. 5. Claim approval into window 1, hide with Escape.
  6. Raise 3 approvals quickly.
- Oracle: 1 queued (target ""); 2 prompt appears in new main window; 3 shows in window 2 only; 4 reroutes to main;
  5 hidden prompt still in list and reopenable; 6 oldest shown first, notification title counts 3, removed when
  last closes.
- Risk: prompt lost when origin window closes.

## K. Usage

### J-USE-010 Usage item through a working day

- Status: partial (`usageflow`).
- Steps: 1. Fresh home: item shows waiting hint. 2. Interactive session reports 5h 23%, week 41%. 3. Headless run
  reports 5h 0.81. 4. Clock passes 5h reset. 5. Restart app. 6. Turn `agent.usageEnabled` off; launch new session.
- Oracle: item text `5h 23% · wk 41%` then 81% with warn tone; tooltip source and age; after reset 5h window
  dropped; after restart values reload from `agent-usage.json` (mode 0600); off: item hidden, new launch settings
  file has no statusLine, existing session unaffected; user's own statusline output still printed by shim.
- Risk: stale percentage shown after reset; user statusline broken.

## L. Mobile

### J-MOB-010 Pair phone, lose network, come back, expire

- Status: partial (`mobileflow`).
- Steps: 1. Enable mobile on trusted LAN; phone opens `http://<ip>:7790`. 2. Pair: code shown both sides; approve.
  3. Laptop moves to cafe Wi-Fi. 4. Returns home. 5. 31 days later phone opens page.
- Oracle: `mobile_devices` row with hash only; after 3 server stopped within 10 s (reason shown), port closed;
  after 4 server back on same port, phone cookie still valid; step 5 401 `E_EXPIRED`, re-pair flow; request with
  `Host: localhost` or other host name refused.
- Risk: server listening on untrusted network.

### J-MOB-011 Pairing abuse

- Status: partial.
- Steps: 1. Phone requests pairing; user denies. 2. Same IP retries within 60 s. 3. 9 devices request at once.
  4. Request left unanswered 120 s.
- Oracle: 2 refused by cooldown without desktop prompt; queue holds 8, ninth refused; 4 times out both sides, no
  device row; rate limit 429 after burst.
- Risk: prompt flood.

### J-MOB-012 Phone moves task stage while desktop also moves it

- Status: partial (stale guard).
- Steps: 1. Phone shows task at step 2. 2. Desktop moves to step 3. 3. Phone taps Next stage (FromStepID step 2).
  4. Phone retries same tap with same Idempotency-Key. 5. Phone sends without Idempotency-Key.
- Oracle: 3 gives 409 `E_STALE`, task stays step 3; 4 replays stored 409; 5 refused; desktop sessions stay open
  on any phone move.
- Risk: double advance.

### J-MOB-013 Phone takes over terminal, desktop reclaims

- Status: partial (`mobileterm`).
- Steps: 1. Phone attaches to running agent terminal (agent input on globally and per device). 2. Types; rotates
  (resize). 3. Desktop types meanwhile. 4. Second phone attaches. 5. Desktop presses Reconnect. 6. Global agent
  input turned off during a later hold.
- Oracle: pty input = phone bytes only while phone controls (desktop writes dropped), size follows phone; second
  device 409 `E_TERMINAL_BUSY`; after reclaim socket close code 4001, pty resized to desktop size; turning input
  off ends hold; audit log lines carry no keystrokes.
- Risk: interleaved input from two controllers.

### J-MOB-014 Permissions matrix

- Status: partial.
- Steps: device with write off; write on, agent input off; both on but global agent input off; revoke device
  mid-SSE.
- Oracle: 403 `E_WRITE_OFF`, 403 `E_AGENT_INPUT_OFF` twice, SSE ends on revoke and next call 401; reads keep
  working where allowed; `cwd` absent from session payloads.
- Risk: local path disclosure.

## M. Config sync

### J-CFG-010 Pull with conflicts, choose per row, verify backup

- Status: partial (`configsyncflow`).
- Pre: shared repo configured; local edits to 2 workflows and 1 prompt; shared changed same prompt and 1 workflow.
- Steps: 1. Fetch; toast appears; open review window. 2. Preview: rows clean, conflict, blocked. 3. Choose shared
  for prompt, keep mine for workflow. 4. Apply. 5. Open backup folder.
- Oracle: files on disk: prompt equals shared text, workflow equals local; result `Applied`, `Kept` 1, `Blocked`
  count; `backups/configsync-<stamp>/` holds pre-apply copy of every replaced file byte-identical; journal file
  gone; next preview shows kept conflict still pending.
- Risk: merge base moved for kept row so conflict vanishes later.

### J-CFG-011 Preview goes stale

- Status: partial.
- Steps: 1. Preview. 2. Edit one target file in app. 3. Apply. 4. Preview again; wait 11 min; apply. 5. Preview;
  background fetch brings newer commit; apply. 6. Apply same token twice.
- Oracle: 3, 5 Conflict "changed", nothing written; 4 NotFound "preview expired"; 6 second NotFound; no backup
  folder created on refusals.
- Risk: applying plan computed for other content.

### J-CFG-012 Crash in middle of apply

- Status: new (journal read from code).
- Steps: 1. Apply 6 files; kill process after 3 written (journal `applying`). 2. Relaunch. 3. Repeat with kill after
  journal `committed` before base swap.
- Oracle: case 1: boot recover restores all 6 from backup (hash equal to pre-apply), journal removed, state says
  not applied; case 2: files stay new, base swapped, applied commit recorded; in neither case mixed state; if
  recover cannot run, Preview/Apply refuse "interrupted" instead of proceeding.
- Risk: half-applied config with moved base.

### J-CFG-013 Write failure rolls back

- Status: new.
- Steps: 1. Make one target read-only. 2. Apply 5 files.
- Oracle: error; all 5 equal pre-apply bytes and modes; backup folder may remain; journal gone.
- Risk: partial write.

## N. Memory

### J-MEM-010 Memory lifecycle: add, supersede, search, import

- Status: partial (`journeys-infra-misc` J-MEM-001; memory flows).
- Steps: 1. Agent stores 30 facts over a session (5 near-duplicates). 2. User edits one in module (new version).
  3. Search `"quote` `AND OR NEAR(` `*`, unicode, 500-char query. 4. Filter scope and origin. 5. Import document.
  6. Restart; search again.
- Oracle: `memories` has one current row per lineage, superseded kept; update/delete of old version blocked by
  trigger; hostile queries return results or empty, never FTS syntax error; results ordered by score; filters
  with unknown value refused; imported file under `distilled/`; same results after restart.
- Risk: FTS injection error surfaced to agent.

### J-MEM-011 Embedding model install, failure and fallback

- Status: partial.
- Steps: 1. Search with no model (keyword only). 2. Click download; cut network at 50%. 3. Retry; checksum of
  tampered file fails. 4. Clean install; backfill runs. 5. Kill embed worker during search.
- Oracle: nothing downloaded before click; partial file not used (manifest written last); checksum failure
  message and no model dir activated; after install results carry `match` semantic or both; worker kill leaves
  keyword results within timeout, next search respawns worker.
- Risk: corrupted model loaded.
