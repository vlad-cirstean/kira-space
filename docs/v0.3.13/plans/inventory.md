# P626 feature inventory

Whole-app inventory distilled from `docs/HISTORY.md`, every `docs/history/*.md` block (v1 to v0.3.12, P1 to P625) and
`docs/ARCHITECTURE.md`. Input for later phases; implements nothing.

Conventions:

- One app since P537: Kira Space hosts former Kira Studio modules. Module order (`frontend/src/workbench/modes.ts`
  `MODE_ORDER`): `ade`, `automations`, `memory`, `git`, `studio`, `api`, `docker`, `kubernetes`. Memory hidden unless
  `experimental.memory` (P556).
- Tag `flow:` names Go flow package under `apps/kira-space/internal/flows/`. Packages: `adeflow`, `agentflow`, `apiflow`,
  `appflow`, `captureflow`, `claudeflow`, `configflow`, `configsyncflow`, `dbflow`, `dbmcpflow`, `dockerflow`, `gitflow`,
  `githostflow`, `grpcflow`, `harnessflow`, `httpflow`, `journeyflow`, `kubeflow`, `llmpromptsflow`, `memoryflow`,
  `mobileflow`, `notifyflow`, `promptflow`, `proofflow`, `repoflow`, `reviewflow`, `termflow`, `usageflow`, `windowflow`;
  gate `coverage` (0 exemptions, P236).
- Tag `ui:` names Playwright spec under `apps/kira-space/tests/ui/` (mocked bridge, WebKit); `ui:studio/` under
  `tests/ui/studio/`; `mobile:` under `tests/mobile/`; `visual:`, `perf`, `e2e-real`, `claude` (real CLI, opt-in) beside.
  Contract fixtures in `tests/contract/` join flow and UI halves.
- "Bug:" lines record fixed defects, phrased as class plus instance. "Open:" lines record limits still listed in
  ARCHITECTURE "Known open items".

## 1. Shell, windows, modules

Features:

- Title bar with icon-only module tabs (tooltip, `aria-label`, P438), app title "Kira Space" at traffic-light inset
  (P576), keep-awake toggle, 34px drag region (P369). ui: `modules`, `window-chrome`, `mode-switch`.
- Per-window mode persisted, debounced write, flushed on native close handshake (`createModeStore`, P128). Default start
  module `git` (P390).
- Full-area module kind (`ade` layout `full`) vs panel module (panel, start, newTab, optional tab strip).
- Tab strip: preview (single click, italic) vs permanent (double click), pinned unclosable first tab (git graph), drag
  reorder, close/close others/to right, Cmd+W, horizontal wheel scroll, tab-kind icons. ui: `tabs-scope-db`,
  `ui:studio/tabs`.
- Tabs persisted per window (`TabsRepo` upsert/prune, serialized saves, payload bounded); terminal tabs never persisted.
- `keepMounted` tab kind (v-show) keeps git graph state across tab switch (P342). Studio per-view runtime stores keep
  filters, find bar, results across tab switch, cleared on tab close (P375). `KEEP_WHILE_OPEN` keeps Discover, keyvalue
  page, mask rules, Monaco view state (P433). ui: `ui:studio/tab-filter-state`, `ui:studio/tab-switch-retention`.
- Multi-window: New Window, cascade, per-window tabs, cross-window broadcasts (connections, settings, tabs, tree, layout,
  schemas, mask rules, api data, clusters). Closing last window hides app. flow: `windowflow`, `journeyflow`
  (`TestTwoWindowsIsolation`).
- Ephemeral windows: review windows (per branch), proof windows (`proof-`), host-view windows (PR, Jira issue), files
  window (`files-<uuid>`, P490). Never restored; purged at boot.
- Native menu: Settings Cmd+, , Toggle Project Panel Cmd+B, Window tab navigation, Shift+Cmd+W close window, Check for
  updates (P328), Window > Validation proof (P569).
- Command palette (shadcn CommandDialog).
- Status bar 22px interface font: CPU/memory, running agents ("N agents · M needs input", "N working" pulse, "M unseen"),
  running ops widget with Stop (P518), usage limits, blame item, update banner. `StatusBarItem` with `tip` slot.
  ui: `status-agents`, `running-ops-status`, `agent-usage`.
- Panel layout: left panel, dock (Operations, Cmd+J), resize handles with drag-release fallback (P185), per-module panel
  sizes (P312). ui: `panel-resize`, `ui:studio/panel-size-per-module`.
- Scrollbars hidden until pointer in panel, no layout shift (P391). ui: `scrollbars`.
- Window sizing: default 1280x800, first-launch clamp (open: first window unclamped, macOS screen cache).
- Single instance lock (`app.lock`); boot failure native message pre-window; BootFailure island with Retry.
- About pane with repo link (P572). flow: `appflow` (`TestAboutGitHubLink`); ui: `settings-about`.
- Quit confirm "N agents running" (QuitGate, agents, runs, scripts, P279). ui: `quit-confirm`.

Bugs:

- Lost write on close: mode write within debounce of close dropped; fixed with native flush (P108, P116).
- Second close after Dock re-show dropped pending flush (P108).
- Circular import left store unbound on repo-row click (P116).
- Window menu tab keys ignored Kubernetes tabs (P493).
- macOS crash: `NSApp unhide` off main thread on second Review code press (P354).
- Mission Control three-finger swipe moves focus to another Space. Open (P396).

## 2. Settings, theme, design system, i18n

Features:

- One `SettingsService`, one `settings` table (P544); apply-on-save, revert to defaults; panes kept mounted so validator
  survives tab switch (P103). ui: `ui:studio/settings-apply-on-save`, `settings-ops-merged`.
- Panes: General (date format, log level), fonts (data font customizable, interface fixed, availability probe, font scale
  sm/md/lg/xl), row height, word wrap, row coloring, Agent, Agent notifications, Capture, CLI tools, File types,
  Experimental, LLM prompts, Memory, Mobile access, Shared config, Studio MCP (5 sections, P619), Updates, About.
  ui: `settings-*`.
- Dark-only flat palette (P505, P508, P525): outline buttons only, `dialog-primary` accent outline, semantic
  `dialog-approve`, `dialog-deny`, `dialog-danger` (guard N17), dialog surface `--kira-bg-dialog` AA contrast (P579).
  ui: `dialog-surface`, `focus-ring`. Open: no light theme.
- Shared primitives: shadcn-vue (Button, Dialog resizable opt-in P371, DropdownMenu, Popover, Tooltip one elevated
  ground, Command, Checkbox, RadioGroup, Switch only in settings panes U28), `ProjectPicker`/`BranchPicker` (cap 50,
  Show more, disabled reasons, P398), `ValueChip`/`BranchChip`/`PathChip`, `SecondaryTabs` with count badge (P466),
  `MenuIconButton` (P497), `SecretInput` reveal (P516), `ToolbarSeparator`, vue-sonner toasts.
- i18n: one vue-i18n catalog, glossary, copy lint, copy coverage specs (P265). Open: backend, git-core, vue-flow text
  English only. ui: `copy-coverage`, `copy-wording`, `consistency`.
- Guard scripts: `check-ui-primitives.sh` (U-rules), `check-conventions.ts` (G, N rules), CodeGraph, knip, Biome.

Bugs:

- Invalid hidden settings draft re-enabled Save (panes unmounted, P103).
- Focus ring white flash on transition (P130); Deny button blue focus edge (P624).
- Tooltip root between DropdownMenu and trigger stole Popper anchor: "+" menu off-screen (P461, again P601).
- NativeSelect `v-model` overrode `:value`, blank selects at 9 sites (P110); Tailwind `z-[1]` generated no CSS, sticky band
  stacking (P96).
- Disabled-tile attribute `data-off="false"` still matched selector, greyed New connection tiles (P164).
- Proof screenshot menu label clipped and overprinted (P621).

## 3. Studio connections and project tree

Features:

- Connection kinds: PostgreSQL, MySQL, MariaDB, SQLite, ClickHouse, MongoDB, Redis, Kafka, SQS, S3, Kibana (P424).
  RabbitMQ dropped.
- Connections are YAML files `<home>/config/connections/<stem>.yaml` projected by `connfiles.Loader` (P475); secrets
  `${secret:<stem>.password}`, `<stem>.uri`; colour, filters, mask rules in file. `NewConnectionDialog`,
  `ConnectionFileDialog` (form plus YAML, P514), lifecycle duplicate/delete/rename/move. ui: `ui:studio/connection-files`,
  `connection-lifecycle`, `connection-form`.
- Colour swatches (none shows diagonal slash), kind icons, error popover, green connected dot, drag reorder (P308,
  `TestConnectionOrder`). ui: `ui:studio/connection-order`, `row-coloring`.
- URI mode: URI encrypted whole, masked by default, Show decrypts, Copy via gated reveal (P181). Paste credentials panel
  parses .env, DSN, CLI flags, quoted values; review masked password; blob never stored (P248).
  ui: `ui:studio/connection-paste-credentials`, `credential-reveal`.
- Reveal saved password through Touch ID with 5 min grace (P14).
- Pre-connect scripts: shell command, settle window, per-connection delay for non-loopback targets, visible in Ops panel
  with timer (P307); limit 2000. ui: `ui:studio/preconnect`.
- Connect/disconnect; disconnect returns open tabs to Reconnect and load; Update reconnects on destination or read-only
  change.
- Read-only flag guard and capability gates.
- Stale data on connection drop (P325): fetched data visible and selectable, `StaleDataStrip` with Reconnect, no edit or
  commit, staged edits survive, Discard enabled, editors keep buffer with Save disabled, upload refused, stream
  Send/Delete disabled, console restamped never rerun. Error "Can’t save. The connection was lost.".
  ui: `ui:studio/grid-connection-drop`, `view-connection-drop`, `immediate-write-connection-drop`,
  `browse-s3-connection-drop`.
- Lazy cached tree: tables first, per-kind folders, Kafka topics and Consumer groups, Postgres partitions under parent
  collapsed (P328), row estimate number only; sticky group headers band (max 3 rows); truncated children flag.
  ui: `ui:studio/tree`.
- Tree menus and shortcuts: F2 rename, Delete, Cmd+C copy, Cmd+N new, Enter open, Shift+Alt+C copy path, Cmd+D duplicate.
- Connection panel filter: checkbox tree exclusions; new objects never hidden; dialog focuses invoked row.
- Metadata cache (SQLite L1, staleness by connection `Since`, served while disconnected, 4 MiB/row, 200 rows/conn).
- DataGrip import (macOS Keychain only, KeePass refused, MCP modes imported). ui: `ui:studio/datagrip-import`.
- Statement timeout per connection, throttle, explain threshold, MCP tab, Privacy tab (sections 9, 10).

Bugs:

- Duplicate-connection INSERT misaligned placeholders put throttle into `mcp_enabled` (P313).
- Duplicated connection shared secret names; connection migration deleted rows on failed file write (P482).
- Preconnect: dialog Test survived app quit; cancelled preconnect did not end disconnected; race "connecting" vs script;
  concurrent Test across windows shared key; empty PATH element; unbounded Wait on backgrounded helper (P108, P307, P313).
- Password with unencoded `/ ? #` stored unencrypted; rename of unrevealed secret wiped it (P108).
- Postgres `PG*` env fallbacks silently reached other host (P108).
- Cache: stale listing after reconnect (`putIfSinceUnchanged`); result racing invalidation got cached (generation
  tracker); truncated refresh left stale complete row (P24, P108).
- Large data: spread into `Array.push` overflowed past 65k rows; Browse 200k nodes needed `markRaw`.
- Tree: collapse during expand ignored; children loaded after disconnect landed (P108).
- `openTrackedTab` crashed on deleted connection (P173, P176).

## 4. Studio data grid (SQL tabular)

Features:

- SlickGrid virtual grid; same table in N tabs; page sizes 10/100/1000/10000; count all; projection (never zero columns);
  multi-sort badges; Stop; prefetch; L2/L3 caches. ui: `ui:studio/slick-grid`, `data-view`, `interaction`.
- Filter toolbar WHERE/ORDER BY with history, saved and recent menus, filter autocomplete from column metadata; Escape
  reverts; filter change invalidates count. Go to PK/FK writes plain filter (P328).
- Find bar (case, word, regex, regex scan cap 10k), filter-rows toggle, priority window, rescan on page replace.
- Mutations: add/delete/duplicate row, duplicate as insert, cell edit, staged change set with exact command preview,
  before/after Monaco diff with Changes and SQL tabs (P326, P367), commit in one transaction, rollback, rails (yellow
  edit, red delete), generated columns gated. ui: `ui:studio/mutations`, `grid-commit-in-flight`.
- Staged edits survive paging, sort, filter, projection, Refresh, keyed by PK; staging refused without full PK (P174).
- Copy/paste: TSV copy raw (quoted only for tab, quote or line break), CSV copy neutralizes formula starters with a leading ' (CWE-1236), multi-line paste splits rows, quoted value one cell,
  paste into open editor one value, Cmd+C cell/row/column, CSV export, Copy as JSON; shared `selectionCopy` for keyboard
  and menu (P335). ui: `ui:studio/grid-copy-menu` (Chromium, real clipboard).
- Selection: drag with autoscroll, corner selects all, whole row/column, Shift+arrow extends from active cell, fill-only
  selection (P324), right-click keeps multi-row selection.
- PK/FK navigation: FK graph, cell buttons, popover preview with Open in new tab, Edit referenced row (Postgres, MySQL,
  MariaDB, SQLite).
- Column widths: header-aware min, 96px initial, 64px resize floor, kept across reloads keyed by column set (P195), font
  change re-measures, keyboard column resize.
- Header tooltip stacked sections: name/type, PK/NOT NULL chips, explanation, comment (P310). ui: `ui:studio/tooltips`.
- Sticky header and gutter in one scroller (P182).
- Fake data generator (Faker, type bounds, excludes read-only). ui: `ui:studio/fake-data`.
- Mask preview disables edits.

Bugs:

- Pagination: next/prev double advance while load in flight; delete last row on last page did not step back (P108).
- Selection: right-click collapsed multi-row selection; console right-click on row selection fell to single-cell menu
  (P335).
- Stale state: filter idle blur re-read; superseded scan republished; preview SQL served stale (gcTime 0).
- Data fidelity: truncated cell staged as real data; `numeric(p,p)` fake overflow; `$numberLong` beyond safe integer;
  MySQL BIT edit; strict timestamp parse.
- Commit: failed commit error lost; committed mutation not reloading other tabs; Revert disabled after blur-stage
  republish (P327); edits staged during commit dropped (P168).
- Layout: pending-changes diff rendered 0px tall (`h-full` under `max-height`, P367); cell editor footer scrolled out of
  view (P309); header one frame behind body (P182).
- Perf: per-row compositing layers (P162); cached tab switch rebuilt per-grid styles (P139).
- Open: quit discards staged edits; insert-row cell single-line input flattens line breaks; tab-switch remount above
  50 ms budget in WebKit sandbox.

## 5. Cell editor

Features:

- Monaco dock per data view (CodeMirror removed P64); format autodetect and override (text, json, xml, csv, sql,
  iso8601, epochSeconds, epochMillis, base64, hex); beautify indented/compact; timestamp translate pane with calendar;
  hex/base64 decoded pane; validate chip; generators UUID v4, ULID, hex token, Now; word wrap; undo/redo; "Show changes"
  diff toggle (P327); read-only mode for console and proof (P519). Height persists (P538,
  `TestLayoutCellEditorHeight`). ui: `ui:studio/cell-editor`.

Bugs:

- Lost edit: dirty buffer not staged on tab switch or selection change (fixed: stage on unmount and selection change).
- Truncated value staged as full write or reported broken JSON.
- ULID timestamp wrong; format then Cmd+Z wiped query (P69).

## 6. Query console, explain, format

Features:

- Monaco console: run statement/all, result tabs (new-result toggle per tab), close others/to right, Stop, Run statement
  menu item. ui: `ui:studio/console`.
- Autocomplete SQL, Mongo, Redis: schema-aware (DDL doc, schema columns, tree relations), Schema (DDL) dialog, Mongo field
  sampling, hover escapes DB text. ui: `ui:studio/autocomplete`, `sql-schema`.
- Format per statement (Shift+Alt+F, worker parse), "Already formatted". ui: `ui:studio/console-format`.
- Explain: 5 kinds, parsed plan, auto-explain toggle and threshold per connection (skips past 10 statements), per-
  connection `explain_threshold_rows` (P593). ui: `ui:studio/console-explain`, `connection-explain-threshold`.
- Result grid hybrid selection, Copy as JSON/all, Mongo copy Shell/Canonical/Relaxed EJSON, duplicate column names
  disambiguated, column virtualization (200 cols).
- Saved queries with cross-window sync. ui: `ui:studio/saved-queries-sync`.
- Op log stores console path; Re-run reopens at path, reconnects.
- Row/byte cap with truncated flag, early cursor close (P174).

Bugs:

- Splitter: `#` and `//` comments, `E''` strings, nested block comments, `[ident]`, `$` in identifiers mis-split (fixed);
  compound bodies (`BEGIN ... END`) still split. Open.
- Format: undo no-op (shallowRef), trailing comment separator, text changed during await discarded, final semicolon.
- Double click Run during reconnect ran twice (starting guard).
- Closing tab did not cancel run and auto-explain.
- Open: Postgres/MySQL reads drain past cap; some Redis commands read whole before cap; ClickHouse `ᴺᵁᴸᴸ` string shows
  as NULL.

## 7. Definition, documents, key/value, browse, stream, Kibana

Features:

- Definition view: Structure/Source, Columns/Indexes/Constraints, Mongo Indexes/Validation, Kafka partitions/config/
  consumer groups, SQS attributes, ClickHouse engine/keys; column menu copy name, add to projection, sort by.
  ui: `ui:studio/definition`.
- Documents (Mongo): PK preview, first layer expanded, per-row edit/expand/delete, EJSON v2, virtual row heights, copy
  `_id`/document. ui: `ui:studio/document-view-readonly`, `document-new-save-double-submit`.
- Key/value (Redis, S3 object): virtual list, Redis list paging past 500, cursor-paged hash/set/zset/stream, 5000-field
  hash, Download never blocked by read-only, object editor 1 MB edit / 4 MB preview.
- Browse panel (Redis, S3): one level, Up and breadcrumb, filter, vertical split preview; opening leaf previews in split,
  not replacing tab (P340). ui: `ui:studio/browse-folder-nav`.
- Stream (Kafka, SQS): partition multiselect, since-timestamp, offset window, compose gated on capabilities, binary
  payload marker, Kafka tombstone NULL marker, SQS poll with visibility timeout setting (default 1s, confirm names
  redrive limit), poll stops at duplicate batch, dedupe by MessageId. ui: `ui:studio/sqs-poll-visibility`.
- Kibana Discover: Go KQL compiler, field/value/operator autocomplete, hits histogram (ECharts), saved queries
  Cmd/Ctrl+S, column selection (P424, P425). ui: `ui:studio/kibana-discover`, `kibana-saved`. Open: internal
  `/internal/search/es` route; Discover remount heap growth.

Bugs:

- SQS browse hid messages and bumped receive count (DLQ risk); stale receipt handle delete past visibility timeout
  refused (P108, P174).
- Kafka tombstone coalesced to empty string in mock (P180); Kafka 1 MiB per-partition fetch window clamp.
- Browse supersession: late level load landed over newer one.
- Documents flick scroll misses 50 ms frames. Open (P161).

## 8. Per-engine adapter facts

Features:

- Go-native adapters behind `adapters` contract, Router with RunOp, bounded reconnect, in-flight op cap 64, cancel per
  connection. Conformance suites per capability (`internal/adapters/*/*_test.go`); general vs complete real-container
  suites; `test-matrix`, `db-compat` (min/max) in PR CI (P614).
- Postgres/MySQL/MariaDB: sslmode semantics, read-only wrap, SET search_path, explain JSON (MySQL 9.7 schema 2.0, P604).
- SQLite: file never modified, rowid keyset, ATTACH/TEMP shadow-safe catalog, locked maps to `E_TIMEOUT`.
- ClickHouse: insert only, `KILL QUERY` cancel, readonly profile detection (P604), `withStatementTimeout` (P607).
- MongoDB: EJSON, mixed `_id` keyset tiers, killOps on disconnect, `listDatabases` on connect detects auth failure.
- Redis: string edits, any delete, denies connection-state commands, bounded forms (`ZREVRANGE`).
- Kafka produce; SQS send/delete, FIFO fields; S3 transfer, SSE-KMS preserved, conditional writes, tagged refuse.
- `Pinger` per adapter, `Router.Probe` (P590).

Bugs:

- Special characters: NUL identifier, `/` in S3 key, IPv6 host joins, Mongo password with space, KIRA_HOME path with
  `? # %` in DSN, Latin-1 UTF-8 truncate boundary.
- Cancel: ClickHouse client timeout killed long queries without KILL; SQLite cancel; abort race waited per-conn in-flight.
- Redis nil reply surfaced as `E_QUERY: redis: nil` (P604).
- ClickHouse readonly=1 ignored statement timeout; row-cap kill never fired with gzip buffering (P607).
- MySQL 9.7 plan schema unparsed (P604). Mongo 8.x/9.x unverified in CI (kernel range). Open.

## 9. Masking and privacy

Features:

- Per-column rules per connection, kinds name/email/text/number/date plus redact, deterministic per value, correlation
  tag, applied over MCP (Go render boundary) and grid preview toggle.
- Rules match by column name; Privacy tab overview, Masked columns card with example, New columns card (Allow, Undo,
  Mask as, Allow all) (P596). ui: `ui:studio/connection-privacy`, `mask-preview`.
- Column review `mask_review`: new columns withheld until reviewed, 30 s recheck, `withheldColumns` (P534).
  ui: `ui:studio/connection-mask-review`.

Bugs:

- Masked column leaked through unaliased/quoted alias projection; plan metrics leaked on masked connection; explain plan
  `attached_condition` leaked (P108, P168).
- Column review matched rule table while masking ignores table: masked column stayed "Needs review" (P596).
- Go vs TS grapheme count differs for Indic conjuncts. Open.
- Open: view renaming masked column; JSON/array columns over-refused; tag provenance accepted.

## 10. Studio MCP (database, scripts, API, Kubernetes) and approvals

Features:

- External server `kira-studio` (loopback 8766, bearer 7-day TTL, Regenerate, install into Claude) plus per-step session
  server `kira-studio-session` (ephemeral port, per-launch hashed tokens, P548). Tools merged (18, P560).
  flow: `dbmcpflow`; ui: `ui:studio/studio-mcp-settings`, `studio-mcp-copy`.
- Database tools: `list_connections` (access, enforcement, statementTimeout), describe, `run_query` with required `mode`
  read/write/ddl and `path`, `explain_query`, `browse_table` (masked default, count, paging, stable order, P535, P609).
- Permissions per connection read/write/ddl/masked deny/allow/prompt; unclassified uses strictest; table allow list
  `mcp.tables` (P532); read lane engine-enforced read-only (P527); MCP reads refused with open user transaction (P330);
  statement timeout (absent 10 s, P529); expensive-query EXPLAIN gate (P530); ops throttle one paced gate per connection
  (P531); limit exemptions `exm_` always approved (P528); unreachable connection raises no prompt (P590).
  ui: `ui:studio/connection-mcp-*`, `connection-throttle`, `dbmcp-unreachable`, `approvals-exemption`.
- Result review before delivery per mode (`mcp.review.read|write|ddl`, held in memory, caps 8/64 MiB, P536); preview
  responses (P333). ui: `ui:studio/dbmcp-preview`, `dbmcp-review-*`.
- Ops tools `list_operations`, `cancel_operation` (P518).
- Scripts tools `list_scripts`, `run_script`, `get_script_run`, `get_script_logs`; opt-in `mcp` block; short/long mode
  (P521, P587). ui: `ui:studio/dbmcp-script-long`, `dbmcp-approval-script`.
- API tools `api_list_environments`, `api_request`: per-host agent credentials, redirect guard, rules allow/deny hosts and
  methods, default ask/deny, allow rules skip approval (P522, P580, P595). flow: `TestApiAccessRules`,
  `TestApiAccessDefault`; ui: `ui:studio/dbmcp-api-*`, `api-agent-access`.
- Kubernetes tools (section 11).
- Aggregated approvals queue (`internal/approvals`): one dialog, one item at a time with position badge, X/Escape
  minimises to toast never denies, Deny all, no bulk approve (P520, P618); target banner (colour, read-only, host,
  database, class); read-only Monaco with Formatted/As sent toggle; Deny initial focus; window raise per kind 2 s
  (`dbMcp.focusOnApproval`). ui: `approvals`, `approvals-one-at-a-time`, `approvals-first-frame`,
  `approvals-alignment`, `ui:studio/dbmcp-approval*`, `dbmcp-focus`.
- Audit trail per task `ade_db_audit` (masked statements, codes only, P547). ui: `ade-v2-db-audit`.

Bugs:

- Bulk approve approved unseen prompts; removed (P618). Stacked items from `v-for` over all items.
- Approval prompts shown for dead server while state read connected (P590); `MarkLost` check and teardown race (P603).
- Panic text leaked masked data; helper path with space gave 401; port bind failure fell back silently (P108).
- `run_query` forced full reconnect; settings hydration lost after restart (v1.7).
- Same-host redirect of hand-approved `api_request`; plan-to-send race (P595, P603).
- ClickHouse EXPLAIN misclassified so explain gate never fired (P108).
- Open: read lane does not stop all side effects; write vs ddl not DB-enforced; Mongo/Redis read app-checked; HTTP server
  without write timeout; external server not audited; `TestDbMcpBrowsePreferredThroughClaude` flaky.

## 11. Kubernetes module

Features:

- Read-only module (client-go): clusters from kubeconfig (stored ids, migration), namespaces, kinds (kind bar, recent,
  categorized picker), resources table, describe with Secret redaction, logs with follow (P439, P486).
  flow: `kubeflow`; ui: `ui:studio/kubernetes-module`, `kubernetes-tabs`.
- Module-local tabs: click previews, double-click keeps, pod tab keeps log stream.
- Cluster reorder drag, Alt+Up/Down, menu (disabled while searching), cross-window refetch (P485).
  ui: `ui:studio/kubernetes-order`.
- Every cluster call recorded in op log (op kind `kubernetes`), log stream row Cancel ends stream, background refetch
  skips log (P484). ui: `ui:studio/kubernetes-ops`.
- MCP: one switch per cluster plus prompt/allow, description, off cluster refused (P440, P603).
  ui: `ui:studio/kubernetes-mcp`.

Bugs:

- `LogsOpen` could hang on recorded-call panic; cluster-change event subscribed per caller (P493).
- Migration 0097 never widens permissions (deny becomes ask is residual change).
- Open: interactive exec credential plugins fail.

## 12. Docker module

Features:

- Engine resolution (`DOCKER_HOST`, contexts via `docker context show`, TLS via PEM paths, socket probe Desktop/Colima/
  OrbStack), states not-installed/daemon-down/unreachable; no credential reads (P429). flow: `dockerflow`;
  ui: `ui:studio/docker-module`.
- Containers grouped by Compose project, origin icons (devcontainer, Testcontainers, kind, Kubernetes, Swarm, buildx),
  images, volumes, networks; start/stop/restart; logs follow/filter/timestamps/wrap, last 2000 lines; Stats; exec via
  terminal with explicit New session; storage `/system/df` on demand with Refresh (P252); registry button.
  ui: `ui:studio/docker-disk`.
- Edit in place (resources, restart policy, rename, networks) vs recreate with loss confirm, tabs with dirty dot
  (P253, P260). ui: `ui:studio/docker-edit`.
- Restore after engine/VM restart: tracker, snapshot excluding ephemeral containers, click-only restore in dependency
  order with health wait, banner in Docker only, shared restore dialog (P334, P336, P441). ui: `ui:studio/docker-restore`.
- Virtualized overview table (P481).

Bugs:

- Scroll sluggish: rows recreated per range change, tooltip per row (P311).
- Heap 64 MB at 400 containers before virtualization (P480).
- Exec open unbounded, logs close not awaited (P209).
- Banner text overlap (P340).
- Open: tcp without TLS allowed; exec chips persist; restore marker unverified on Desktop/Colima/OrbStack.

## 13. Api: HTTP requests

Features:

- Request builder: method, URL, params, headers, body modes (raw, code, JSON, urlencoded, form-data with files, binary),
  auto-grow fields, descriptions side-car, header autocomplete, variable colouring/hover/completion, find bar.
  flow: `httpflow`; ui: `ui:studio/http-request`, `http-request-body`.
- Response viewer: status hint, headers, body highlight, beautify in worker, chunked fill (freeze fix P160), raw
  inspector and editor, timeline per hop (DNS, connect, TLS, TTFB, download). ui: `ui:studio/http-raw`, `http-timeline`.
- Request settings global and per request: HTTP version, timeout, max response MB (default 5), SSL verify, redirects,
  cookie jar toggle.
- Cookies: process jar with public suffix, incognito jar, request/response panes. Incognito per tab skips history and op
  log.
- History 30 per scope, compare/diff, masked secrets. ui: `ui:studio/http-history`.
- curl paste/parse/generate (`-u`, `--json`, `--data-urlencode`). ui: `ui:studio/http-curl`.
- Dynamic values `{{$...}}`, `fake.<module>.<method>`, pipes (base64, case, url). ui: `ui:studio/http-dynamic-values`,
  `http-pipes`.

Bugs:

- Secret leakage: redirect kept auth headers cross-host and on https-to-http downgrade; wire masked after truncation;
  protojson error echoed secrets (P108).
- Large data: response viewer froze on big bodies (P160).
- Closing tab mid-send leaked op and recorded history.
- Stuck loading: history load supersede retry loop.
- Open: response bodies, gRPC messages, jar cookies reach history unmasked.

## 14. Api: gRPC

Features:

- Reflection or `.proto`, unary and server streaming (no client streaming), metadata vocabulary, status code hint,
  history (256 KiB cap), live buffer 10k, deadline `api.requestTimeoutMs` (0 none), wait connect 5 s, reflection 15 s,
  Describe cancellable. flow: `grpcflow`; ui: `ui:studio/grpc-request`.

Bugs:

- Reflection retried on bare `io.EOF` transport glitch; cyclic proto deps crash; Describe timeout missing.
- Call before method resolved errored unclearly; terminal event race; service fullName lacked package.
- Postman export skips gRPC (warned).

## 15. Api: collections, variables, environments, import

Features:

- Collections and environments are YAML (`<id>.collection.yaml`, `<id>.environment.yaml`, `apifiles.Store`, P473);
  secret variables `${secret:name}`. flow: `apiflow`; ui: `ui:studio/api-config-files`, `collections`.
- Tree: folders, requests, multi-select (click, Ctrl/Cmd toggle, Shift range, Shift+arrows, Ctrl/Cmd+A, Escape), batch
  move/delete in one transaction, invalid move refused, bulk delete one confirm with counts, deleted request tabs stay
  unsaved (P362); manual order drag/Move up/down/Alt+Arrow for collections and environments (P381, P524).
  ui: `ui:studio/api-order`, `api-lifecycle`.
- Variables `{{name}}`, secrets masked with reveal, value history, descriptions, bulk `.env` editor (refuses typed
  secrets), environment colour, environment as tab, variables form (P514). ui: `ui:studio/http-variables`,
  `api-variables-form`, `api-secret-reveal-isolation`.
- Postman v2.1 import/export: environments and globals, many files atomic per file, API envelopes unwrapped, disabled
  variables skipped, partial data imported with warnings (`empty_values`, `unnamed_<n>`, `missing_request`) (P328, P361,
  P385). flow: `TestPostmanImportCollectionAndEnvironment`.
- Cross-window sync via TanStack plus `kira:api:dataChanged` (P112). ui: `ui:studio/api-cross-window-sync`.

Bugs:

- Partial import regression: P361 strictness dropped valid entries (P385); generated names collided with real keys
  (P386).
- API migration rerun lost promoted variables (P482).
- Undecryptable env secret shadowed other scope; plaintext auth kept in origin JSON; export followed symlinks.
- Unsaved draft clobbered by remote refetch; restored tab Save created duplicate row (P112, P108).

## 16. Automations (Runs), quick commands, scripts

Features:

- Module renamed Terminal to Automations (P242), UI copy "Runs" (P438). Runs are `<id>.yaml` in `config/runs`, bad file
  keeps last good (P472). ui: `automations-file`, `automations-form`, `automations-module`.
- Normal scripts (shell `-c`, working dir default `<home>/automations/<id>`), params text/select/multi-select/secret
  (secret write-only saved value, P447), collections with drag between, Escape cancels cross-list drop (P381).
  ui: `automations-scripts`, `automations-order`, `automations-secrets`.
- Smart scripts (`claude -p` schema-forced report, outcome done/failed/blocked/cancelled), ADE variables, run dialog with
  task/branch preview. ui: `automations-smart`.
- Recurring schedules (cron 5 fields, timezone, next 3 fires), confirm popup or run without prompt, overlap skip with
  reason, missed runs skipped. ui: `automations-recurring`.
- Runs list: one tab per script key (live focuses, finished replaced in place), server refuses concurrent run
  (`E_CONFLICT`, P380, P386); Stop; Copy for agent; finished runs in Operations panel with 40-line output tail (P416);
  log selectable, Copy log over all chunks (P577). ui: `automations-tabs`, `automations-runs`, `automations-runs-layout`,
  `automations-ops`.
- Resume stuck smart runs; restore banner for scripts running at quit (P279, P441). ui: `automations-resume`,
  `automations-restore`.
- Repo scripts reference Runs scripts (prepare, deployed SHA; smart refused, P513, P515). ui: `repo-script-picker`.
- MCP exposure (section 10). ui: `automations-mcp`.

Bugs:

- Log clipped (`flex-1` inert) and not selectable (body `select-none`) (P577); copy log stopped at first page (v0.3.5 review).
- Strict-mode locator race matched two Stop buttons (P606).
- Open: run file deleted outside Kira leaves folder; saved secrets show in live terminal.

## 17. Terminal

Features:

- PTY (creack/pty, Setsid, login shell `-l -i`), SIGHUP group then SIGKILL 2 s, tabs not persisted, xterm lazy, one
  renderer `terminalRenderer.ts` for every terminal. flow: `termflow`.
- Shift+Enter inserts newline (`ESC CR`, P376); Cmd/Ctrl+click opens http(s) links incl OSC 8 with hover hint (P393);
  file path links `path:line:col` open file with cursor, relative to live cwd (P489). ui: `terminal-links`,
  `terminal-file-links`.
- `CLAUDE_CODE_SCROLL_SPEED=3`, scrollback 5000, writes coalesced one in flight per tab.

Bugs:

- Hangs: Linux pty blocking Close never unblocked read loop; zombie treated alive; Session.Close unbounded after SIGKILL;
  SIGHUP ignored at start (nohup) so Close never hung up shells (P153, P294).
- `window.open` no-op in Wails webview broke link clicks (P393).
- Terminal unregister ordering flaked `TestTwoWindowsIsolation` (P435).
- Open: Claude redraw after phone resize unobserved.

## 18. Operations panel and op log

Features:

- Shared `OpLogPanel` (both former apps, P432): toolbar, facets (module, connection), detail pane, steps tree, background
  toggle, "Via <tool>", Copy details, Cancel, Re-run (Studio). ui: `operations`, `operations-db`, `operations-memory`.
- Persisted `op_log` (30 days / 5000 rows), lanes foreground 500 and background 300, batched events, dedupe, redaction of
  URL credentials (P415). Every git spawn recorded with Origin; acli calls; kube calls; MCP tool calls; memory-mcp rows
  ingested from foreign process (P415 Part 2); db category folded (P544). flow: `TestOpsLogAndCancel`,
  `TestOpsEveryGitSpawn`, `TestMemoryMcpOps`.
- Source user/MCP recorded (P328).

Bugs:

- Dropped `op:end` left row stuck running (unbounded queue fix); phantom "test" op kinds (P108).
- Orphaned running foreign row becomes error.
- Open: `gh`, `az` calls and agent's own shell git not in log.

## 19. Git module: repositories and panel

Features:

- One repo list for Git and ADE: import single repo, scan folders (add, watch, remove), hide repos (persisted, P259),
  remove; per-repo colour (Colour submenu); drag reorder (P140, P308). flow: `repoflow`; ui: `repo-order`,
  `git-panel-hidden`.
- Git panel tabs Repos/Files/Review (`kira.git.panelTab`, default Repos); nested worktrees under repo; Open terminal on
  repo/worktree rows; repo-head branch label.
- Repositories page as ADE top-bar tab (P558): nickname, prepare script and timeout (or Runs script ref), integration
  branches, deployment environments, worktree base path, review base candidates. Repo config in
  `~/.kira-space/repos/<stem>.yaml` (P474). ui: `ade-repos`, `repo-config-file`, `repo-config-form`.
- Native git process layer: porcelain parsers, NUL separators, `cat-file --batch`, SHA-256 repos, env scrubbed of
  `GIT_DIR`, NFC/NFD path normalization, argv flag injection guard.
- Bound `GitService` (65 methods) per window plus bulk `gitgraph` stream (credit 2, 32 MiB frames) replacing git stream
  protocol (P476-P479). flow: `gitflow`; ui: `git-bound`, `git-bound-writes`, `git-bound-review`, `git-graph-plane`.
- Credential prompts via askpass broker relayed to focused window (opens one if none). ui: `git-credential-relay`.

Bugs:

- Duplicate worktree listing (worktree imported as top-level repo) (P84).
- Repo list synced only while ADE subscription installed (P190).
- Native transport opened repo late; Review pane, diff tab needed Retry (P231).
- Bound call after window close leaked Conn; data race in `newRepoEntry` (P477, P482).
- Unhide during folder scan re-hidden (`hidden_rev` CAS, P277).
- Adding deployment environment always failed (empty script rejected, P222).

## 20. Git graph

Features:

- Streamed graph (FlatBuffers chunks), lanes, branch-ordered layout, `--date-order` walk with local topo order (P347),
  collapse runs of non-checked-out branches with tip plus placeholder (P350), badges line above subject, `+N` chip,
  PR badges at head from change request index, uncommitted strip, Load more/Show more, column resize with lane floor,
  widths persisted. ui: `repo-graph-*`, `graph-blank-rows`.
- Commit detail panel (Show more, refs row, ancestry PR, files tree, Go to file, Open in graph), commit body links
  external. ui: `repo-commit-detail`, `repo-commit-meta`.
- Search (Go tail scan plus client matcher, PR number/title in ref scope).
- Branch picker/RefsPopover 5 tabs (Branches, Tags, Stashes, Worktrees, Stacks), ranking, pins. ui: `repo-refs-popover`.

Bugs (recurring area):

- Blank/vanishing rows: stale plan vs store row count (P203); graph width seeded before lanes clipped outer lanes (P220,
  P225); branch-out edge lane overwrite after Load more (P225); plan watcher microtask gap blanked rows on click/scroll
  (P278); collapsed row count after new commits; malformed tip sha threw.
- Load more exhaustion "Load the last 0" at exact page multiple (v1.3).
- Reload from scratch after tab switch (P342); commit detail vanished when opening file diff (hidden tab measured 0x0,
  P343).
- Branches start from nowhere under topo order (P347).
- Column resize: message column floor overflowed viewport, drag froze below lane need (P228).
- Server nil slices marshalled null so Show more dead; root commit tree never rendered (P124).
- Scroll flicker on PR badge resolution; checkout misalignment mid-stream (P71-P83).
- Open: keeps pre-refresh scroll offset; first paint times out under load; display order not final on page load.

## 21. Git operations, files, blame, code workspace

Features:

- Checkout with Go preflight (protected branches, auto-stash opt-out honoured), revert, reset 3 modes, cherry-pick,
  undo slot, in-progress banner Continue/Skip/Abort (Skip not for merge), stash with merge-tree pop prediction, stash
  apply cross-branch, create worktree here, worktree create/switch/remove with prepare script progress, stacked branches
  restack. flow: `gitflow`.
- Remote: fetch, push, force-with-lease, decomposed pull (ff-only/merge/rebase, `--rebase-merges` only from config),
  auto-fetch with backoff stopping on credential errors, error banner with retry. Open: no standalone merge or rebase.
- Code workspace per repo: project tree (tracked plus untracked not ignored), Monaco viewer, diff tab worktree vs HEAD,
  "Open all changes" multi-diff tab, repo-wide streaming search, Quick Open Cmd+P, markdown reading view, seti icons.
  ui: `repo-workspace`, `repo-file-tree`. Open: tree not live, no search include/exclude, no multi-line search.
- File editing (P419): read only by default, Edit on worktree file, Cmd/Ctrl+S, base hash conflict with Refresh/Overwrite,
  atomic write keeping mode/BOM/EOL, 8 MiB cap; stat-gated reads (P433). flow: `TestEditFile`; ui: `repo-file-edit`,
  `repo-file-gate`. Open: quit discards unsaved edit.
- Diff layout toggle per device (`kira.repo.diffLayout`), next/previous change crosses files (P358).
- Blame inline and status bar item (author, age, subject, click reveals commit); revision-pinned tabs never blamed.
- External opens and default app (P332, P490): Kira as default for 41 code and 14 data extensions, Finder opens coalesced
  300 ms into one files window; repo path opens repo-file tab, else read-only local file. flow:
  `TestOpenedFilesWindow`; ui: `opened-files`, `settings-file-types`. Open: macOS default-app switch unverified.

Bugs:

- Commit run twice; edits staged during commit dropped; auto-stash opt-out ignored (P168).
- Pull config regex not QuoteMeta'd; rebase synonyms missed; prepare script incomplete on wait delay; protected gate used
  local name not upstream (P108).
- catfile deadlock; circuit breaker tripped by cancels; healthy process killed after post-success cancel (P152, P168).
- Askpass shim single-quote injection (P108).
- Line counts dropped files lacking numstat row; counts removed (P257).

## 22. Code hosts, change requests, host view, issue tracker

Features:

- Host abstraction `githost` (GitHub via `gh`, Azure Repos via `az`, fake host in tests, P318, P423). GHES host support.
  flow: `githostflow`; ui: `git-host-provider`, `git-host-azure`.
- Persisted change request index per repo: GraphQL pages, ETag probe, resumable backfill, delta walk, checks refresh,
  rate-limit gate, focus-regain sync (P352). ui: `repo-graph-change-request-index`.
- One-PR host-view window: description, comments, checks; post comments/replies with Markdown composer; PR line comment
  with head-moved check; no attachments (P427, P429, P464). ui: `host-view`, `host-view-comments`, `github-pr-open`.
- GitHub link parsing (trailing slash, `/changes`, sub-paths), browser fallback when view refuses (P594).
  flow: `TestGitHubLinksInApp`; ui: `github-links-in-app`.
- Issue tracker abstraction, Jira via `acli` (P417): issue view (fields, comments, callouts, chips, ADF to Markdown),
  cache 30 min stale-while-revalidate, batch search 50 issues, PR count only (P428, P460, P592). ui: `issue-view`,
  `issue-view-fetch`, `ade-v2-issue-tracker`.
- Settings CLI tools pane: install/login steps for gh, acli (Homebrew tap trust, P488), az. ui: `settings-cli-tools`.
- Tracked-item update indicators per concept (P512). ui: `ade-tracked-updates`.

Bugs:

- `gh` output truncated at 4 MiB gave "unreadable response" (cap 64 MiB, P169).
- GraphQL document invalid (field merge conflict `Team.name` vs `User.name`), PR open failed (P619); schema test added.
- PR cache "no PR" stale; facts lost across refs change; PR badge on non-head commits (P189, P209).
- Link misclassification: changes tab and sub-path links filed as change requests, rejection swallowed (P594).
- Jira detail save raced in-flight reads across post (P470).
- Open: Jira PR rows and images; Azure comment/assignee updates, reactivated PR, no head ref; live acli/az unverified;
  PR view caps, no live update.

## 23. Code review (review window)

Features:

- Per-branch review window reusing git-ui review module, title bar header with sync badges (P150, P369).
  flow: `reviewflow`; ui: `ade-v2-review*`, `repo-review-interaction`.
- Since-review diff content-based snapshot (survives rebase), range marking, folder tri-state mark (P358), reviewed file
  keeps diff, review attention unreviewed/partial/reviewed/changed with Alt+F8 navigation (P465), removed-since-review
  section with dismiss (P499).
- Uncommitted work reviewed like committed (hashed snapshots, promote on commit, P418, P599).
- Diff hides unchanged regions with 3 context lines, comment lines revealed (P357); side-by-side not collapsing when
  narrow; gutter plus replaces line number; hunk mark view zone; thread overlay capped height (P413).
- AI comments kinds change request/question, edit, Send review comments N to agent, Copy as prompt, Send clears sent
  (P354, P464).
- PR actions: Comment, Approve, Request changes, Comment on PR, resizable submit dialog (P370, P371); foreign PR lookup by
  host head name or number (P392).
- Ref freshness: Fetch, Update branch, Update base (ff-only), Diverged/not pushed badges (P581). ui: `ade-v2-review-sync`.
- Head badge "At <sha>" or Stale (P412); file list follows shown file (P561).

Bugs:

- Review stale on new commit (files target unchanged, P384).
- Hover glyph lane shifted margin (P371).
- Whole files rendered (`hideUnchangedRegions` off for review, P357).
- `changedSinceReview` ignored by UI (P465).
- Ref spelling (`origin/feat/x`, fork) missed PR lookup, no actions shown (P392).
- Nondeterministic removed-entry order from map range (P507).
- Diff-compute race in navigation test (P358).
- Open: oversized deltas miss removed lines; taken-over review round lacks reply tools; branch rename keeps old review
  rows; comment reload fan-out N+1.

## 24. ADE board and plan

Features:

- Board: tasks with branches across repos, Plan day bands (7-day window, later work joins Later, P401), Later and Backlog
  shelves (P405), weekend hatch, Today marker, history toggle (P491), archived inline with finder (P404).
  flow: `adeflow`; ui: `ade-v2-plan`, `ade-v2-later`, `ade-v2-archived`.
- Task card: 3px task-colour edge, status text menu, agent indicator slot, step action icons, tag glyphs, at most 2
  actions plus More (P500, P601, P625). ui: `ade-v2-task-row`, `ade-v2-card-icons`.
- Add popover: new task (today or next work day), existing branch, GitHub link (fork PR as `<owner>/<head>`), kind
  (task/orchestrator) (P353, P383, P571). ui: `ade-v2-add`, `ade-v2-add-link`.
- Drag between day bands (incl review items, P368), estimates set/change/clear any time, off days move.
- Task context menu (right-click, Shift+F10, More): stage actions, move, skip, stop agents, open session, plan
  today/next/Later, open Jira/GitHub, copy title/id/branches/session id, convert review to work item (P377), archive
  with confirm. ui: `ade-v2-task-menu`.
- Task statuses: editor on Workflows page, default slots new/step/review/finished, manual move sticks until step change,
  delete needs reassign (P379, P411). ui: `ade-v2-task-status`.
- Conflict pairs via `merge-tree`, ahead/behind, merged/stale via patch-id, repo updating chip (P622).
  ui: `ade-v2-pair-check`, `ade-v2-repo-updating`.
- Capture box on shell bar centre.

Bugs:

- Click flash from Sortable `chosenClass` (P402); drag spec flaky without two-frame wait (P368).
- Estimate extend-only rule removed (P353); estimate flake (P371).
- Card head overflow at narrow pane (P625).
- `%(authoremail:trim)` misclassified mine as review (P129).
- Open: archived data grows unbounded; history rides board wire; rename/delete conflicts dropped from pairs; linked
  worktree dirty state not watched; ADE first mount heap retained.

## 25. ADE tasks, panel, notes, links

Features:

- Task panel tabs Main, Details, Sessions (P501); Main branch list in Plan row look with PR line and checks glyph; panel
  tabs kept per task (`adePanelTabs`, P463). ui: `ade-v2-task-panel`, `ade-v2-main-branches`, `ade-v2-panel-tabs`.
- Task repos and branches edited independently (add/change/remove), refused while in use, dirty/unmerged confirm
  (P397); worktree checked-out branch vs assigned mismatch chip (P617). ui: `ade-v2-task-branches`,
  `ade-v2-branch-change`.
- Base branch pick, Change base rebases with stacked children, Rebase/Queue after headless runs, Abort rebase (P241).
  ui: `ade-v2-base-rebase`.
- Repoless tasks: task folder `<home>/ade/tasks/<id>`, outside-repo steps; Start on no-repo task returns
  `E_TASK_NO_REPO` with repo gate dialog (P467, P598). ui: `ade-v2-norepo`, `ade-v2-repo-required`.
- Notes chain: user notes and handovers one chain, Markdown editor, composer draft store, copy per entry, selectable
  (P542, P582). ui: `ade-v2-notes`, `ade-v2-handover`.
- Details: links (free-form add/edit, classified, grouped Issues/Change requests/Misc), `AdeLinkStrip` (click in app,
  Cmd-click external), proofs chain, timeline toggle, Kira DB audit (P509, P545, P600, P613, P615).
  ui: `ade-v2-details-*`, `ade-v2-links-strip`, `ade-v2-proof-chain`.
- Artifacts derived and reported (tracker link, branches, commits, change requests, deployments) (P409).
  ui: `ade-v2-artifacts`.
- Panel edits forwarded as diffs to running TUI agents with send/discard bar (P546). ui: `ade-v2-panel-forward`.
- Archive keeps data; restore refuses live branch name (P399).
- Readable file names for workflows/steps with rename journal (P584). ui: `ade-readable-names`.

Bugs:

- Late-branch bug: attach did not fill not-created row (P397).
- Notes not selectable (TipTap contenteditable, body `select-none`, P582).
- Handover dialog empty first frame (event order, P618).
- Repoless workflow test race: fake agent created branch before snapshot (P506).
- Removed-lines capture with no hunks; Jira slot took non-Jira URLs (P503).
- Open: pending "Send then archive" lives in renderer; headless runs take no forwarded edits.

## 26. ADE workflows, runs, sessions, agents

Features:

- Workflows as YAML config files with step library (`<home>/steps/*.yaml`, overrides, routes, `prompt_extra`), tabs
  Statuses/Steps/Workflows, Form and YAML, graph editor (dagre, Vue Flow), explicit Save/Discard, last-valid kept,
  started task snapshots workflow with outdated badge (P196, P255, P526). ui: `ade-v2-workflows`,
  `ade-v2-workflow-steps`, `ade-v2-workflow-layout`.
- Steps: flat list, kinds agent/script/smart_script/manual/review, results ok/not ok routes with loop limits,
  `max_returns`, skip, stage settings (auto compact), `db_mcp` key, sandbox block, `outside_repo` (P247, P288, P293,
  P431, P510, P548). ui: `ade-v2-flat-steps`, `ade-v2-return-limit`, `ade-v2-stage-settings`,
  `ade-v2-workflow-db-mcp`, `ade-v2-sandbox`.
- Run engine: headless `claude -p` stream-json, finish_step outcome done/failed/needs_input, retry, approve, send back,
  Take over (TUI resume), resume stuck runs after restart, stage dialog with handover notes and Close/Keep open, Next
  stage/Finish, resume stage action (P146-P148, P279, P469, P495). ui: `ade-v2-run`, `ade-v2-stage-move`,
  `ade-v2-stage-resume`, `ade-v2-resume`, `ade-v2-task-flow`.
- Worktree setup: prepare script progress and failure reason everywhere, launch waits on setup (P202). Multiple worktrees
  per repo (P517). ui: `ade-v2-multi-worktree`.
- Agent MCP `kira-space` (13 tools): `task_info`, `declare_repos`, `request_branches` (1-20 repos, blocking dialog,
  optional base, existing checkout/adopt), `rename_branch`, `change_branch`, `handover`, `report_artifacts`,
  `attach_proof`, `create_task`, `list_tasks`, `task_chain`, leases (P199, P408, P517, P570, P583, P617).
  ui: `ade-v2-branch-request`, `ade-v2-branch-adopt`, `ade-v2-branch-rename`.
- Orchestrator tasks: children, next pass, child mirror read-only, create_task approval (P571).
  ui: `ade-v2-orchestrator`.
- Resource leases: named queues, ranked by task order, 10 min TTL, wake via `SendIfIdle`, Force release (P620).
  ui: `ade-v2-leases`.
- Sessions: TUI and headless tabs, sessions menu with filter (P437), tooltip state/age, Restart, Close, Copy session id,
  ad hoc sessions per worktree (P410, P436), unseen-finished status (P610), status running while subagents/background
  work. ui: `ade-v2-sessions*`, `ade-v2-session-status`, `ade-tui-takeover`, `ade-v2-task-launch`.
- Async feedback: `adeOps` chips and toasts (always or past 2 s), background branch removal and task close (P618, P624).
  ui: `ade-v2-background-ops`, `ade-v2-op-kinds`, `ade-v2-agent-ops`.
- Performance: shared board snapshot by generation, coalesced pushes `BoardChangedEvent`, singleflight duplicate ops
  (P622-P624). ui: `ade-v2-board-coalesce`.

Bugs:

- Refresh "never fetched" plus "exec: no command" from empty git path (P230); unset git path broke StartRun (P147).
- Finish counted previous-stage session as talking (P469).
- `request_branch` names collided; unnamed branches created before request (P199, P406).
- Agent paused on first `next` text; handover paging placeholder escaping (P571).
- Concurrent Take over during Restart started second resume (P394).
- Flake: stale refresh observer re-claimed key (P353).
- Workflow route to manual/skipped step accepted; stale `review.implementer` after edit (P302).
- Snapshot hash flagged outdated on added wire fields (P510).
- Open: `taskMu` held across git in branch create/change/rename; SQLite reads queue behind writes; no detached runner
  (quit stops agents); `declare_repos` untested live; unsaved workflow edits lost on module switch.

## 27. Agent harness: Claude, OpenCode, hooks, context, settings

Features:

- Harness abstraction with optional Structured/Headless roles, providers Claude and OpenCode (late session id, plugin
  hooks, agent key), agent pickers per launch (P316, P448, P449). flow: `harnessflow`, `agentflow`; ui:
  `agent-provider`, `ade-agent-missing`.
- Hooks via per-session `--settings` temp file, never user settings (P233); shim inert without session env; per-terminal
  lanes (P623); events Stop, SubagentStop, Notification, PreToolUse/PostToolUse paired by id; activity reducer phases
  waiting/working/idle. flow: `claudeflow`.
- Agent context shared (32 KiB, per-provider render), Agent config tab Context/Settings with leave guard (P452, P498).
  ui: `ade-agent-context`, `ade-agents-tab`.
- Agent settings `settings.yaml` common plus per provider, MCP credentials write-only in kira.db `secret_values` (P453,
  P557). ui: `ade-agent-settings`, `settings-agent`.
- Import/export agent config per provider with previews, backups, masked diffs (P454). ui: `ade-agent-port`.
- Hermetic launches `agent.hermetic` (default false since P588) (P455).
- User MCP servers picker from `claude mcp list`, no `~/.claude.json` reads (P444).
- `MCP_TOOL_TIMEOUT=2100000` on every launch (CLI cut calls at 60 s, P408).
- Agent sandbox macOS Seatbelt per step (read-only folders, worktree writes, keychain readable writes denied), Linux
  flags only; default off since P588 (P287, P341, P510, P511).
- Missing `claude` detected before launch (`ResolveClaude`, P277); CLI path fresh per launch (P623). ui:
  `ui:studio/terminal-claude-missing`.

Bugs:

- Stop did not clear toolName (P127).
- Hooks writing user settings risk (fixed by temp file; `TestClaudeSettingsUntouched`).
- Hermetic test never set flag after default flip; read-denied reason wording (P605).
- Open: many hermetic and OpenCode gaps (managed config, profile exports, skills leak, AGENTS.md, `OPENCODE_CONFIG`
  collision), sandbox unverified on Mac, network-off loopback, background count sticks, backgrounded Bash cannot arm
  waiting.

## 28. Notifications

Features:

- Desktop notifications: agent finished, needs input, ADE run ended, automation (smart run done/blocked/failed, scheduled
  failure), test button; per-kind toggles; click focuses window and tab or run; suppressed while that tab focused;
  cooldown; finished skipped while any app window is key, no cooldown burn (P238, P359). flow: `notifyflow`; ui:
  `settings-agent-notify`.
- Prompt notifications for popups (section 29).

Bugs:

- Finished note fired while user looked at app (P359). Subagent hooks moving main session (P610).
- Open: macOS notifications unverified; screen locked keeps window key so notes skip.

## 29. Prompt routing and LLM prompts

Features:

- Prompt router by origin: window-triggered prompt in that window; generic only main window, queued if none; system
  notification click focuses popup (P246). Kinds: schedule confirm, approvals, git credential, mobile pairing, update,
  masking review, proof ready, branch request, task create. flow: `promptflow`; ui: `prompts`.
- LLM prompts registry (`internal/llmprompts`), overrides in `config/prompts/`, Settings LLM prompts with template-var
  validation (P271). flow: `llmpromptsflow`; ui: `settings-llm-prompts`.

Bugs:

- Approval dialog lacked requestId keying across 2 windows; pairing countdown not re-armed per request (P108, P113).
- Proof ready Open did not close dialog (P611).

## 30. Usage limits

- Usage 5h/weekly percent and reset in status bar from injected status-line command (keeps user's) and headless
  `rate_limit_event`; no credential reads (P239). flow: `usageflow`; ui: `agent-usage`. Open: only Kira-started sessions,
  Pro/Max only.

## 31. Capture

- Global hotkey (`CmdOrCtrl+Shift+K`, customisable, live re-register with conflict status) opens quick capture popup;
  clipboard classified PR link, Jira key, text; Enter adds to Later, Cmd/Ctrl+Enter to Today; pasted PR head attached as
  review branch, partial outcome notice (P430, P462). flow: `captureflow`; ui: `capture`, `settings-capture`.
- Open: hotkey unverified on real desktops; Wayland stays `requested`; text only.

## 32. Mobile agents web

- Local HTTP server on trusted LAN only (subnet, router IP/MAC), one address bind with rebind, plaintext warning, device
  pairing approval, per-device cookie 30 days with CSRF, expiry enforced mid-request (P212, P223, P227). flow:
  `mobileflow`; mobile: `pairing`, `insecure-origin`; ui: `mobile-access`.
- Screens: Need You, Plan, Later (shelf moves), agents; writes: capture, reorder, TUI input, stage Back/Next with notes,
  terminal attach with desktop overlay and Reconnect, archived task read (P212, P399, P405, P407). mobile: `agents`,
  `writes`, `terminal`, `tabs`.
- Status codes: `E_TASK_NO_REPO` 409; idempotency keys; stale-stage guard.
- Bugs: reloaded deep link redirected to Need You; phone input lock deadlock across PTY write; Serve skipped device
  recheck after Attach; session route needed cwd (P212, P217).
- Open: unverified on real phones; plaintext LAN; phone sees desktop worktree paths; secrets unmasked in phone terminal
  ring.

## 33. Memory (experimental)

- Memory MCP (`kira-memory` stdio): store with gate (atomic facts, challenges, questions), reconcile add/update/noop with
  history, search hybrid FTS5 plus local embedding (download pinned, keyword-only while indexing), ranking bm25 plus
  cosine, ids `km_` lineage, scopes, origins, groups, `memory_context` (P201, P210, P272, P275). flow: `memoryflow`;
  ui: `memory-module`, `memory-ranking`, `memory-provenance`.
- Ingest files/folders by path with jobs (chunk, extract, cross-check, store), estimate confirm, gitignore, write back,
  quit-mid-run recovery; distill (paste, file, selection, rewrite mode) (P211, P274, P285, P297, P315).
  ui: `memory-import`, `memory-ingest`, `memory-distill`.
- Memory home under Space home, lazy open (P559); Settings Memory (connect Claude Code, model download with Cancel).
  ui: `settings-memory`.
- Bugs: duplicates across parts survived; stale revision re-decides; scan vs auto-start race; prompt via variadic flag
  swallowed; failed-download message lost.
- Open: no delete; re-import never retracts; English-oriented semantic search; Mac unverified; compression target unmet.

## 34. Validation proof

- Proof window (Studio MCP `proof_save` one call): sessions history 30, steps tabs, views table/key-value grid with cell
  editor, chart (5 kinds, 1000 points), Mermaid diagram (validated, strict), JPEG image (caps), change panel accept/
  reject, summary and one-line step text, view state per step (P420, P426, P541, P569, P591). flow: `proofflow`;
  ui: `ui:studio/proof-*`.
- Screenshots: save all as JPEG to Desktop `<session>` folder or chosen location, Reveal, macOS copy-all multi-image
  clipboard (P597, P621). ui: `ui:studio/proof-copy-all`.
- Proof attached to task chain (P543).
- Bugs: raw args lost integer precision (fixed by raw read); image memory and decode cap; ready dialog stayed open.
- Open: Mermaid grammar checked only in window; multi-image clipboard unverified on Mac.

## 35. Config files and config sync

- `internal/configfs`: folder store per module under `<home>/config/<module>/`, atomic save with base hash, debounced
  watch, line-located YAML errors, JSON Schema, `${NAME}` and `${secret:name}` refs, generated `AGENTS.md`; config file
  bar only on YAML tab, `ConfigFileProblem` with Show in YAML (P450, P540, P578). flow: `configflow`.
- Modules on files: workflows, steps, runs, api, repos, connections, agent settings/context, prompts.
- Config sync GitOps (P554, P585): pull-only shared git repo, three-way merge per file and entry, deactivate per entry,
  gate (YAML, validators, literal-credential scan), preview, backup, journal, rollback, boot Recover, repos module keyed
  by normalized origin. flow: `configsyncflow`; ui: `config-sync`, `config-sync-repos`.
- Bugs: doc walker cycle; rename journal overwrite; `RefreshAsync` coalescing; credential gate on shared text.
- Open: generic gate only for connections/api/runs/workflows; repos deactivation per unit, comments lost, origin change
  seen late.

## 36. Secrets, credentials, storage, ids

- `internal/secrets`: one keychain item, AES-256-GCM with AAD scope, sealed `secret_values`, masker for log sink,
  outcome fields, terminal tail (P447). ui: `ui:studio/secrets`, `secret-reveal`.
- CLI credentials never touched (gh, az, acli, docker, claude), guard G5.
- Storage: one `kira.db` (P544), STRICT tables (P552), go-sqlbuilder runtime SQL (P553), baseline per DB with
  `SchemaTooOldError` (P557), `review.db`, memory DB; ids UUIDv7 keys plus prefixed nanoid handles (P523).
  ui: `ade-ids`.
- Bugs: keyring wrong-length key panic; filter history rune-boundary truncation; storage growth bounds.

## 37. Updates, packaging, site

- Update check (GitHub latest release), dialog, install.sh detached, dev builds refuse, short error cause (raw logged)
  (P119, P328, P330). ui: `update-dialog`, `settings-updates`.
- DMG packaging, macOS min 26, release guards, chapter branch pre-push guard.
- Presentation site (Astro, Starlight docs, WebP shots generated in CI).

## 38. Keep-awake, restore, lifecycle

- Keep-awake titlebar toggle, with agents setting (terminal agents plus running headless), caffeinate on macOS.
- Restore offers: Docker containers after engine restart, Runs scripts running at quit (P441).
- Startup: leftover sweeps, review/proof/files windows purged, migrations with backups.
- Bug: keep-awake Rearm race reported lost; init blocked boot on stuck assertion (P108, P116).

## Bug classes

Each class lists areas (section numbers above) with representative instances.

- Clipboard and copy-paste: grid TSV formula injection, multi-line paste split, native Ctrl/Cmd+C/V inside text
  controls, right-click on selection used single-cell menu (4, 6); permission-denied `readText` (4); Mongo EJSON copy
  forms (6); log copy past first page (16); terminal Cmd+Enter/Shift+Enter (17); notes/handover/log text not selectable
  under body `select-none` (16, 25); proof multi-image clipboard, PNG vs JPEG support (34).
- Selection and focus: multi-row selection collapsed on right-click, Shift+arrow anchor after select-all (4); tooltip
  root stole menu anchor (2, 24); Sortable click flash (24); focus ring flash, Deny focus edge (2); approval Deny initial
  focus and window raise (10); review file list not following shown file (23); stale nav status after fast tab switch
  (21).
- Pagination and paging: double advance while loading, last-row delete on last page (4); Redis list past 500, cursor
  reset (7); git Load more exhaustion at page multiple (20); browse offset limit 100000, stable order (10); acli comment
  caps (22); handover paging placeholders (26).
- Sort, filter, search: stale count after filter change, idle blur re-read (4); dialect quoting in Go to PK/FK and
  Filter by value (4); regex dialect JS vs RE2, ReDoS cap (4, 20); table allow list parse gaps (10); find bar and filter
  lost on tab switch (1, 13); cluster reorder while searching disabled (11).
- Reconnect and connection drop: stale data rules, console never rerun (3); preconnect races and cancel (3); MCP prompt
  for dead server (10); auto-fetch disabled for life on failure (21); mobile rebind on address change (32); bound Conn
  leak after window close (19).
- Stale state and caching: metadata cache vs reconnect, generation trackers (3); PR cache facts (22); review window stale
  on new commit (23); board snapshot and refetch gates (26); Jira cache stale-while-revalidate (22); restored tab dirty
  state (15); superseded async loads landing late (3, 7, 13).
- Large data and performance: 65k push, 200k tree nodes, 10k stream buffer (3, 7, 14); response viewer freeze (13); git
  graph 200k rows, chunk relayout (20); Docker list heap, Memory list, RepoDiffView retention (12, 33, 21); docs flick
  frames, tab switch budget (7, 4); console drain past cap (6); ADE board spawns (26); `gh` output cap (22).
- Special characters and encoding: NUL identifiers and separators, `/` in S3 keys, IPv6, passwords with `/ ? #` or space,
  DSN path escapes, UTF-8 truncate boundaries, NFC/NFD paths, Indic grapheme counts, BOM/EOL preservation (8, 9, 19, 21).
- Cancel and abort: SQLite and ClickHouse cancel, `KILL QUERY` paths, abort race (8); closing tab cancels run (6, 13);
  preconnect cancel on quit (3); binding-promise cancel only abandons reply (19); cancelled git run cause (21); log stream
  cancel (11); ops cancel via MCP (10).
- Undo and revert: Format then Cmd+Z wiped query, undo no-op (6); Revert disabled after blur-stage (4); git undo slot
  cleared before write (21); adopt undo restores donor (26).
- Persistence across restart: mode and tab saves (1); settings hydration (10); column widths (4); run resume and stuck
  runs (16, 26); review windows purged (23); restore offers (12, 16); migrations with backups and resumable folds (36);
  plan persists (24). Accepted loss: quit discards staged grid edits and unsaved file edits.
- Concurrency and races: single-flight and duplicate ops (26); RestartSession vs Take over (26); data race in repo entry
  (19); OrderedEmitter, rpcstream id reuse (18); PTY write locks (17, 32); map-range nondeterminism (23); test races with
  fake agent (26); flaky UI waits needing animation frames (24).
- Secrets and data leakage: redirect header stripping, history masking, op log URL redaction (13, 18); masked column via
  alias, plan leaks (9); panic text (10); live terminal and phone ring unmasked (16, 27, 32); update error raw text (37);
  credential never read from CLIs (36).
- Layout and rendering: 0px-tall diff (4); footer scrolled away (4); log clipped by inert flex (16); hidden tab measured
  0x0 (20); glyph lane shift (23); overprinted menu label (2); narrow pane overflow (24); WebKit blur stall (37).
- Fail-closed policy gaps: unclassified statements use strictest mode, unknown review value coerces to review, unreadable
  api rules read deny, EXPLAIN failure refuses, migration never widens (10, 11).
- Cross-window sync: api data, connections, clusters, settings, mask rules, approvals over two windows (1, 10, 11, 15).
- Platform unverified: macOS-only paths (notifications, sandbox, default apps, approval focus, multi-image clipboard,
  capture hotkey, lannet) checked on Linux or by fakes only (multiple).

## Cross-module interactions

- Git module and ADE share repo list, repo config, `gitsession` registry, worktrees and review windows; ADE git spawns
  ride the shared op-log runner (19, 21, 23, 26).
- Code host index feeds git graph PR badges, ADE PR lines, review actions, host-view window, capture classification and
  add-from-link (20, 22, 24, 31).
- Issue tracker (Jira) feeds ADE Details links, task issue line, tracked updates, capture, host view (22, 25, 31).
- Studio MCP session server per ADE step (`db_mcp`) ties database permissions, approvals, audit trail and task details
  (10, 25, 26).
- Approvals queue serves database queries, exemptions, responses, scripts, API requests, Kubernetes reads, branch
  requests, branch rename/change, task create (10, 11, 26).
- Runs scripts run from Automations, ADE steps (`smart_script`, script steps), repo prepare/deployed-SHA hooks, schedules
  and Studio MCP scripts tools (16, 19, 26, 10).
- Secrets store spans connections, api variables, Runs params, agent MCP credentials and config-file `${secret:}` refs;
  masker spans op log, run tails, headless output (36, 3, 15, 16, 27, 35).
- Config files and config sync cover workflows, steps, runs, api, repos, connections, agent settings, prompts; module
  reloads follow sync applies (35).
- Harness hooks drive status bar agent counts, notifications, keep-awake, usage, ADE session states, leases wake-up and
  quit confirm (27, 28, 30, 38, 26, 1).
- Operations panel aggregates git, ADE, Studio connections, Kubernetes, acli, MCP tools, memory-mcp, finished runs (18).
- Proof tool links Studio MCP output to ADE task chain and proof window (34, 25).
- Prompt router and notifications deliver every approval and popup to correct window (29, 28).
- Mobile web reads board, writes shelves, stages, terminal input; shares terminal PTY with desktop overlay (32, 17, 24).
- Terminal renderer shared by Git/ADE terminals, Docker exec, Runs tabs, phone terminal; file links open in files window
  or repo workspace (17, 21).
- External file opens (Finder, default app, terminal links) land in files window or repo workspace (21, 17).
- Experimental flag hides Memory module, settings pane, Distill selection; memory-mcp stays registered (33, 2).
- Settings agent provider and hermetic switch change every launch site: ADE, smart scripts, review agent, proof, memory
  (27, 16, 23, 33).
