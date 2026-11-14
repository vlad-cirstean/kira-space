# P627 journeys: Docker module

Dedicated Docker file. Earlier entries stay valid and are not repeated: J-DKR-001..003
(`journeys-studio-mcp-approvals.md`), J-DKR-010..016 (`journeys-infra-misc-2.md`). Ids here start at J-DKR-020.
Conventions: `journeys-studio-db` head (now `journeys-studio-connections.md`).

Runs in seconds: flow half against the fake engine of `dockerflow` (in-process Engine API); UI half against
mocked bridge. Time seams: `docker.WithRestoreTiming(tick, grace, policyWait)`, `docker.WithBootProbe`,
`flowharness.BootGen` (simulated daemon restart), `docker.WithHubAPI` (fake Hub). No real waiting: restore ticks,
stop grace, `recreateTimeout` 3 min and `actionTimeout` are shortened or the fake answers at once.

Surface read from code (`internal/docker/bound.go`, CodeGraph): `Status`, `Contexts`, `UseContext`, `Containers`,
`Images`, `ImageRegistry`, `ConfirmImageRegistry`, `Volumes`, `Networks`, `InspectContainer`, `Inspect` (image,
volume, network raw JSON), `Start`, `Stop`, `Restart`, `Watch/Unwatch`, `StatsSubscribe/Unsubscribe`,
`LogsOpen/Close`, `ExecOpen/Write/Resize/Close`, `DiskUsage`, `ContainerSize`, `ContainerEditSpec`,
`UpdateContainer`, `RecreateContainer`, `RestoreState`, `RestoreContainers`, `RestoreDismiss`. No remove, pull,
build, create-from-scratch, compose up/down, volume or network write. Journeys asking for those assert absence
(OQ-DKR-1).

Other facts: endpoint order selected UI context, `DOCKER_HOST`, `DOCKER_CONTEXT`, `docker context show`, default
socket, socket probe; streams per window and only while module visible; stats one engine stream per container
shared by windows; log buffer 20,000 lines, wrap shows last 2,000; edit lock per container; stale `baseHash`
gives `E_CONFLICT`; clearing a limit needs recreate; recreate pulls no image, detaches anonymous volumes; Swarm
and Kubernetes containers read-only; restore snapshot never written while engine unreachable, stopped container
leaves after grace, testcontainers/buildx/kind/kubernetes/swarm/`--rm` never snapshotted, compose project
restored as unit in `depends_on` order (healthy wait up to 60 s), deleted containers listed, never recreated;
registry link lazy, Hub asked only on click.

### J-DKR-020 Local stack session: inspect everything, tune, break, recover

- Status: partial (`dockerflow` containers, streams, edit; chain new).
- Persona: backend developer with compose project `shop` (db, cache, api, web) plus 2 standalone containers.
- Pre: fake engine seeded with 6 containers (1 stopped, 1 restarting), 9 images (2 dangling), 4 volumes, 3 networks.
- Steps:
  1. Open Docker module. Overview lists containers grouped by compose project with state dots; CPU and RAM
     columns fill from stats.
  2. Toggle "all" to include stopped; sort by name, then by memory.
  3. Select `shop-api`; header shows ports `0.0.0.0:8080->80/tcp`, image, origin compose.
  4. Open Images: select image; click registry button (Hub fake answers 200). Second image: Hub 404.
  5. Open Volumes: inspect named volume; copy raw JSON. Open Networks: inspect `shop_default`; copy raw JSON.
  6. Back to `shop-api`: Stop. State updates by event, not by poll.
  7. Start. Restart. Press Restart twice fast.
  8. Open Stats tab; request container size (on demand).
  9. Open Disk usage (on demand); switch tab while it computes; return.
  10. Open edit: raise memory, add network alias; apply in place.
  11. Open edit again: change published port to one used by `shop-web`; apply recreate.
  12. Read error; fix port; apply.
  13. Close window; reopen.
- Oracle: engine state after each step (`docker inspect` equivalent on fake): step 6 `State.Running=false`, 7
  running with new `StartedAt`, exactly 2 restart calls or second refused while first runs (pin); step 4
  `Browser.Opened()` has Hub URL once, second shows "No registry page found" and no browser call; no Hub request
  before click (`fake Hub` request log); clipboard of step 5 parses as JSON equal to engine inspect; step 9
  exactly one `/system/df` call (singleflight) and none triggered by Refresh; step 11 `E_RECREATE_FAILED`, step
  `start`, `restored: true`, old container running under original name; step 12 new id, port mapping present;
  after 13 no stream left open for closed window (fake engine open-connection count).
- Variations: container that is part of Swarm: edit disabled with reason.
- Risk: action on stale row after event reorder; disk walk triggered by routine refresh.

### J-DKR-021 Name collisions and rename races

- Status: new.
- Pre: containers `api`, `api-old`, `worker`.
- Steps:
  1. Edit `worker`: rename to `api`. Apply.
  2. Rename to `api2`; before apply, terminal renames `api-old` to `api2`.
  3. Apply.
  4. Rename `worker` to name with invalid characters (`my api`, `-x`, 300 chars).
  5. Recreate `api` while container `api-kira-old-deadbeef` from an earlier failed cleanup exists.
  6. Recreate `api` twice at once from two windows.
  7. After success, look for leftover temp names.
- Oracle: 1 and 3 `E_CONFLICT` with engine text naming the taken name, `applied: []`, container still `worker`; 4
  refused by validation before engine call (fake request log empty for those); 5 uses fresh random suffix, never
  touches the stale container, warns about it or leaves it listed; 6 one runs, other "edit in progress"; 7 no
  `-kira-new-`/`-kira-old-` container remains after success.
- Risk: recreate removing a container it did not create.

### J-DKR-022 Logs: follow, search, copy, huge output, container restart

- Status: partial (`dockerflow` streams; UI logs spec).
- Pre: fake container emitting scripted output: 60,000 lines; includes ANSI colour, CRLF, tab, invalid UTF-8, one
  line of 200 KiB, stderr and stdout interleaved, timestamps.
- Steps:
  1. Open Logs with tail 1000, follow on. View pinned to bottom.
  2. Scroll up 500 lines; new lines arrive: view does not jump; "new lines" marker appears.
  3. Search `ERROR`; step through matches with Enter and Shift+Enter; count shown.
  4. Toggle timestamps; toggle wrap.
  5. Fake emits remaining 59,000 lines in burst.
  6. Select from a match to end with Shift+click; copy.
  7. Select all (Mod+A) inside log; copy.
  8. Container restarts (new stream epoch); follow continues or shows ended marker.
  9. Stop follow; change tail to all; reopen.
  10. Switch to Git module and back; open same logs in second window.
  11. Clear view; copy with nothing selected.
- Oracle: buffer never over 20,000 lines, truncation note shown; match count equals count in retained buffer;
  clipboard step 6 = exact retained lines in order, long line complete or cut with marker as displayed (pin),
  no ANSI escape bytes, invalid bytes replaced not dropped; step 7 clipboard line count = buffer size; step 8 no
  duplicate lines across restart; step 10 stream closed while hidden and reopened once (fake stream-open count),
  windows have independent stream ids; step 11 clipboard unchanged.
- Risk: selection across virtualized rows copies only rendered rows; search index stale after eviction.

### J-DKR-023 Exec terminal sessions

- Status: partial (`dockerflow` exec; UI docker exec).
- Steps:
  1. Open Terminal tab: no session until New session clicked.
  2. New session: shell picker (`sh`); type `echo hi`; output shown.
  3. Paste 2,000-char multi-line text (bracketed paste); resize window.
  4. Open second session in same container; switch between them.
  5. Run full-screen program (alternate screen); copy selection; exit program.
  6. Container stops while session open.
  7. New session on stopped container.
  8. Start container; new session; close tab with session running; confirm.
  9. Quit app with session open.
- Oracle: fake engine exec log: one exec per session with tty, resize calls carry cols/rows, input bytes equal
  typed and pasted bytes (base64 decoded); clipboard equals selected cells text without trailing spaces; step 6
  session ends with message, no reconnect loop; 7 refused with clear state; 8, 9 exec streams closed (open exec
  count 0), no orphan.
- Risk: keystrokes sent to wrong session after tab switch.

### J-DKR-024 Daemon down, restart, restore prompt

- Status: partial (`dockerflow` restore with `BootGen`).
- Time: `WithRestoreTiming(10 ms, 20 ms, 20 ms)`; `BootGen` bumps boot identity; no real 15 s tick.
- Pre: running: compose `shop` (db healthy-gated, api depends_on db), standalone `redis`, testcontainers `tc-1`,
  `--rm` container `tmp`; `old` stopped before snapshot grace.
- Steps:
  1. Module open; logs following `shop-api`; exec session open.
  2. Engine becomes unreachable (fake refuses). Banner shows down; rows not actionable.
  3. Press Start on a visible stale row.
  4. Engine returns with new boot identity; all containers exited.
  5. Restore banner appears inside Docker module only; open dialog.
  6. Dialog lists `shop` as unit and `redis`; not `tc-1`, `tmp`, `old`.
  7. Untick `redis`; Restore.
  8. During restore `db` never becomes healthy (health wait shortened).
  9. Retry after health fixed.
  10. Dismiss remaining.
  11. Repeat cycle; this time delete `redis` container before restore.
- Oracle: step 3 refused with engine-down state, no request; snapshot not overwritten while unreachable (table
  `docker_restore_containers` unchanged); step 7 start order db then api (fake call order), redis untouched; step
  8 api not started, result names db health timeout; step 10 pending cleared; step 11 `redis` listed as deleted,
  never recreated; log stream and exec of step 1 ended once with message, logs resume only after user reopens or
  automatically (pin).
- Risk: restore starting containers user stopped on purpose; snapshot wiped by outage.

### J-DKR-025 Context switch mid-flow

- Status: partial.
- Pre: contexts `default` (local fake A) and `staging` (fake B over tcp), same container names on both.
- Steps:
  1. On A open `api` logs and edit dialog with unsaved change.
  2. Switch context to `staging` in UI.
  3. Apply the edit left open.
  4. Press Stop on `api` in list.
  5. Change `DOCKER_CONTEXT` in environment; Status refresh.
  6. Select "automatic" (empty context).
  7. Pick context whose endpoint is invalid.
  8. Second window open during all steps.
- Oracle: after 2 lists show B's objects, A's log stream closed; 3 refused (`baseHash` of other engine or stale
  draft discarded), nothing changed on A or B; 4 stop hits B only (request logs of A and B); 5 UI selection wins
  until 6, then env applies; 7 `BadRequest`, previous context kept; both windows switch together.
- Risk: destructive action on wrong engine.

### J-DKR-026 Volumes, networks, ports and images read paths at scale

- Status: new.
- Pre: fake engine with 2,000 containers, 5,000 images, 1,000 volumes, 300 networks; names with unicode, 200-char
  names, labels with newlines.
- Steps:
  1. Open each list; type-to-filter; scroll to end; keyboard navigate (arrows, Home, End, PageDown).
  2. Open container with 40 published ports (tcp, udp, IPv6, ranges) and 30 mounts.
  3. Copy a port mapping, a mount path, container id (short and full), image digest.
  4. Open image used by 50 containers; follow link to a container; back.
  5. Open volume used by no container; network with 100 attached containers.
  6. Events storm: 500 create/destroy events in burst while list is filtered and row selected.
- Oracle: virtualized DOM row count bounded; clipboard values exact (full 64-char id when "full" chosen); filter
  kept and selection kept by id through storm; list equals engine set after storm (no ghost rows); request
  count bounded (coalesced refetch, not 500 list calls).
- Risk: selection drift under live sync, unbounded refetch.

### J-DKR-027 Features outside the surface

- Status: new (defines absence).
- Steps: look for remove container, pull image, prune, compose up/down, create volume, port-forward in menus,
  shortcuts and MCP tools.
- Oracle: none offered; no bound method reachable (contract JSON lists only surface above). If product intends
  them, open question OQ-DKR-1.
- Risk: half-wired menu item calling missing method.
