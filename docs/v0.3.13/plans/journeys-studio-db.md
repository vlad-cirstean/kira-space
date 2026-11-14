# P627 journeys: Studio databases (connections, tree, grid, cell editor, console, masking)

Imagined usage and definition of correct. No implementation, no test code. Inputs: `inventory.md` sections 3-6, 8,
9; `coverage-map.md`; `docs/ARCHITECTURE.md`; source read through CodeGraph (`selectionCopy.ts`,
`clipboardFormats.ts`, `grid/paste.ts`, `ConsoleSlickGrid.vue`, `SlickGridHost.vue`).

Conventions for every journeys file:

- Status: `new` (no test), `partial` (a test touches it, far end missing; cite), `covered` (cite).
- Oracle: far end first (clipboard text, file on disk, target DB row read by second client, app DB row, IPC result,
  state after restart). Screen checks are secondary.
- "Assumption:" marks correct behaviour where inventory is silent.
- Seed table `app.people(id int PK, name text, email text, note text, score numeric, born date, meta jsonb)` unless
  stated. "Second client" = independent driver session (psql, sqlite3, mongosh), never Kira.
- Clipboard facts read from source: `tsvField` quotes a value holding `"`, TAB, LF or CR and doubles `"`; NULL copies
  as empty field in TSV and CSV, as `null` in JSON; `neutralizeFormulaPrefix` (prefix `'` for leading `= + - @ TAB CR`)
  is called from `csvField` only; rows join with LF (TSV) or CRLF (CSV); repeated column names become `x`, `x_2`.

## A. Connections

### J-CONN-001 First Postgres connection from pasted `.env`, then restart

- Status: partial (`dbflow` PostgresJourney, ConnectionSecrets; ui `connection-paste-credentials`). New: file plus
  secret plus restart in one chain.
- Persona: backend developer on day one, copies `.env` block from team wiki.
- Pre: empty `<home>/config/connections/`, Postgres container up, password `p/a?s#s w0rd'"`.
- Steps:
  1. New connection, Postgres tile, open Paste credentials, paste 6-line `.env` (`DB_HOST`, `DB_PORT`, `DB_USER`,
     `DB_PASSWORD="p/a?s#s w0rd'\""`, `DB_NAME`, `SSLMODE=disable`).
  2. Review panel shows password masked; accept. Test. Save as "Orders (prod-ro)".
  3. Connect, expand tree to `app.people`, open table.
  4. Quit app, start again, connect without retyping password.
- Oracle: `<home>/config/connections/<stem>.yaml` holds `${secret:<stem>.password}`, never the literal; grep of whole
  `<home>` (YAML, `kira.db` strings, logs) finds no `w0rd`; `secret_values` has one sealed row for the stem; after
  restart `Connections.List` returns same id/stem and connect succeeds; server `pg_stat_activity` shows user from
  paste. Pasted blob absent from every store.
- Variations: DSN `postgres://u:p%2Fa@h/db?sslmode=require`; CLI flags `psql -h .. -U ..`; quoted value with `#`
  inside quotes; `.env` with `export` prefix; CRLF line ends; duplicate key (last wins, assumption).
- Risk: password with `/ ? #` stored unencrypted (P108 class); blob persisted; secret lost on restart.

### J-CONN-002 Duplicate connection, rotate password on copy only

- Status: partial (`dbflow` ConnectionDuplicateOwnsSecret; no reconnect proof per copy).
- Persona: developer keeps "staging" copy of "prod" with other credentials.
- Pre: connected "prod" with password A; server has second role with password B.
- Steps:
  1. Tree, Cmd+D on "prod". Rename copy "staging" (F2).
  2. Edit copy: user and password B. Save. Connect both.
  3. Rename "prod" to "prod-eu" without revealing password. Disconnect, reconnect "prod-eu".
  4. Delete "staging". Restart.
- Oracle: two YAML files with distinct stems and distinct secret names; after step 3 `prod-eu` still authenticates
  with A (server log shows role 1); after step 4 `staging` YAML gone, its secret row gone, `prod-eu` secret row
  intact; throttle, `mcp` block, colour, filters, mask rules copied field for field (diff YAML minus name/secret
  refs).
- Variations: duplicate URI-mode connection; duplicate twice (names "copy", "copy 2"); duplicate while original is
  mid-connect; copy name collides with existing stem differing only by case (assumption: refused or suffixed, never
  overwrite).
- Risk: shared secret names (P482), rename wipes unrevealed secret (P108), misaligned copied fields (P313).

### J-CONN-003 Pre-connect tunnel script with slow start, cancel, quit

- Status: partial (`dbflow` SQLitePreconnectOp/Delay/KilledOnQuit; ui `preconnect`). New: cancel then real
  connect, two windows.
- Persona: SRE reaching DB through `ssh -L` tunnel script.
- Pre: connection with pre-connect `sleep 3; nc -l 15432 &` style script, settle 2 s, non-loopback delay 1 s.
- Steps:
  1. Connect. Ops panel shows pre-connect row with timer.
  2. Press Cancel in Ops at 1 s. Connection ends disconnected, no error popover stuck.
  3. Connect again, let it finish. Open table.
  4. In second window press Test on same connection dialog while window 1 stays connected.
  5. Quit app during a third Test.
- Oracle: process table (`pgrep -f` of script marker) empty after cancel and after quit; op log rows: first
  `cancelled`, second `ok` with duration at least settle; window 1 connection state unchanged by window 2 Test; no
  orphan listener on 15432 after quit.
- Variations: script exits non-zero (error text shown, capped 2000); script backgrounds a helper that holds stdout;
  empty PATH element; script prints 5,000 lines; delay 0 on loopback.
- Risk: Test survives quit, race connecting vs script, shared key across windows, unbounded wait.

### J-CONN-004 Connection drop mid-edit, reconnect, commit

- Status: partial (ui `grid-connection-drop`, `immediate-write-connection-drop`; mock only). New: real server kill.
- Persona: analyst editing rows when VPN flaps.
- Pre: Postgres connected, `app.people` open, 100 rows.
- Steps:
  1. Edit `name` in rows 3 and 7, delete row 9, add one row. Do not commit.
  2. Kill server-side backend (`pg_terminate_backend`) or stop container.
  3. Observe `StaleDataStrip`; try Commit (refused "Can’t save. The connection was lost."), try paste (refused).
  4. Select range of stale rows, Cmd+C.
  5. Start container, press Reconnect. Commit.
- Oracle: clipboard after step 4 equals TSV of fetched values; second client after step 5 sees exactly 2 updates,
  1 delete, 1 insert, nothing else; between step 2 and 5 second client sees zero changes; console tab on same
  connection restamped, statement not rerun (server statement log shows no extra SELECT).
- Variations: drop during Commit in flight (assumption: transaction rolled back, staged set intact, error shown);
  drop while count-all runs; SQLite file deleted on disk; Mongo primary step-down; Redis restart.
- Risk: staged edits lost, console rerun, half-applied commit.

### J-CONN-005 URI mode connection lifecycle

- Status: partial (ui `credential-reveal`; `dbflow` SecretsStatusAndReveal).
- Persona: developer with Mongo SRV URI.
- Steps:
  1. New Mongo connection, URI mode, paste `mongodb+srv://u:p%40ss@cluster/db?retryWrites=true`.
  2. Save, reopen dialog: URI masked. Press Show (OS auth fake approves). Press Copy.
  3. Within 5 min press Show on another connection (grace applies). After fake clock +6 min press Show (auth asked
     again).
  4. Switch connection from URI mode to fields mode; save.
- Oracle: clipboard after Copy equals exact pasted URI; YAML holds `${secret:<stem>.uri}` in URI mode and after
  step 4 no `uri` secret row remains (assumption: stale secret removed); OS auth fake records 2 prompts (step 2,
  step 3 second half).
- Variations: OS auth denied (no reveal, no clipboard write); auth unavailable; URI with IPv6 host `[::1]`; password
  with space.
- Risk: reveal bypass, stale secret left behind, clipboard written on denied reveal.

### J-CONN-006 Read-only connection end to end

- Status: partial (adapter suites; no flow at IPC boundary).
- Persona: support engineer with prod read-only flag.
- Steps:
  1. Mark connection read-only, connect. Open table: no add/delete/edit affordances; paste refused.
  2. Console: `UPDATE app.people SET name='x' WHERE id=1`. Run.
  3. Console: `SELECT ... ; DELETE ...` run all.
  4. Toggle read-only off in dialog, Update (reconnects), rerun UPDATE.
- Oracle: second client sees row unchanged after steps 2-3 and changed after step 4; error text names read-only;
  open tabs after Update show Reconnect-and-load state then load.
- Variations: per engine (Postgres wrap, MySQL, ClickHouse readonly profile, Mongo, Redis `SET`); S3 Download still
  allowed while read-only; write hidden in function call.
- Risk: read-only bypass through console or paste.

### J-CONN-007 Reorder, colour, filter exclusions persist across windows and restart

- Status: partial (`dbflow` ConnectionOrder; ui `connection-order`).
- Steps:
  1. With 6 connections drag #5 to top; set colour red on it, "none" on #2.
  2. Open connection panel filter on #5, untick schema `audit` and table `app.tmp`.
  3. Open second window: order, colours, exclusions match without refresh.
  4. Second client creates `app.newtable`; refresh tree (new object visible, never hidden).
  5. Restart.
- Oracle: YAML of #5 lists exclusions and colour; order persisted (list order equal after restart); window 2 got
  broadcast (no manual refresh call in its IPC log).
- Variations: reorder while filter text active; drag onto itself; 200 connections.
- Risk: cross-window drift, new objects hidden, order lost.

### J-CONN-008 Edit connection YAML by hand while app runs

- Status: partial (`dbflow` ConnectionConfigFile; ui `connection-files`).
- Persona: power user edits files in editor.
- Steps:
  1. With connection open and connected, edit its YAML outside Kira: change `host` to bad host, save.
  2. Introduce YAML syntax error on line 7, save.
  3. In Kira open ConnectionFileDialog YAML tab: `ConfigFileProblem` with line 7, Show in YAML.
  4. Fix in Kira YAML tab while external editor also saves again (base-hash conflict).
  5. Delete the file outside Kira.
- Oracle: step 1 live connection keeps working until reconnect (assumption), dialog shows new host; step 2 keeps
  last good projection (connection still listed); step 4 Save refused with conflict, no silent overwrite, file on
  disk equals external content; step 5 connection row leaves list in all windows, open tabs show stale/closed
  state, secret row removed.
- Variations: file renamed outside (stem change, secret refs dangling); literal password typed into YAML
  (assumption: accepted locally, flagged by config-sync gate only); unicode stem.
- Risk: bad file wipes connection, lost secret, overwrite of external edit.

### J-CONN-009 DataGrip import with partial failures

- Status: partial (`appflow` DataGripScan; ui `datagrip-import`).
- Steps:
  1. Import from DataGrip project holding 5 data sources: Postgres (keychain password), MySQL (no password), one
     KeePass-backed, one unsupported driver, one with same name as existing Kira connection.
  2. Review report, apply. Run import again.
- Oracle: 2-3 YAML files created as report says; KeePass one refused with reason; second run creates nothing new
  (idempotent, assumption) or suffixes names, never overwrites existing file; imported MCP modes equal source.
- Risk: duplicate rows on re-import, overwrite of existing connection.

### J-CONN-010 Statement timeout and throttle set per connection reach engine

- Status: partial (`dbmcpflow` timeouts for MCP; UI path new).
- Steps:
  1. Set statement timeout 2 s. Console `SELECT pg_sleep(10)`.
  2. Clear timeout (absent). Run again, press Stop at 1 s.
  3. Close tab during a third run.
- Oracle: step 1 ends about 2 s with timeout error; `pg_stat_activity` has no leftover backend query after each
  step (cancel reached server); op log rows end states `error`, `cancelled`, `cancelled`.
- Variations: ClickHouse (`KILL QUERY` visible in `system.query_log`), ClickHouse readonly=1, SQLite busy,
  MySQL `KILL QUERY`.
- Risk: cancel abandons reply only; server keeps running.

## B. Project tree

### J-TREE-001 Keyboard-only tour of large tree

- Status: partial (ui `tree`; single large test).
- Persona: keyboard user.
- Pre: Postgres with 3 schemas, 2,000 tables in one, partitioned table with 300 partitions.
- Steps:
  1. Focus tree, arrow to connection, Right expands, type-ahead not assumed; Down to schema, Right.
  2. Scroll 2,000 tables with PageDown; sticky headers show at most 3 rows.
  3. Enter on table (preview tab), Enter again on another (preview replaced), Cmd+C on a table, Shift+Alt+C.
  4. Expand partitioned parent; collapse while children still loading.
  5. F2 rename a table to `"Weird ""name"" ü"`; Delete another (confirm).
- Oracle: clipboard after Cmd+C equals table name; after Shift+Alt+C equals qualified path
  (`<connection>/<db>/<schema>/<table>`, exact separator per code); second client sees rename with exact
  identifier and dropped table gone; collapse in step 4 wins (node stays collapsed when load lands).
- Variations: truncated children flag at cap; rename to existing name (error, tree unchanged); rename on read-only
  connection (refused); disconnect during expand (children must not land).
- Risk: collapse ignored, late children after disconnect, identifier quoting.

### J-TREE-002 Metadata cache while disconnected, then reconnect to changed schema

- Status: partial (cache unit tests; journey new).
- Steps:
  1. Connect, expand all of `app`. Disconnect.
  2. Browse tree offline (served from cache), open Definition of `people` (cached or clear "connect to load").
  3. Second client: drop `app.tmp`, add `app.fresh`, add column `people.nick`.
  4. Reconnect, expand `app`.
- Oracle: after step 4 listing shows `fresh`, no `tmp`, columns include `nick` without manual refresh; cache row
  `Since` updated; no stale listing reappears after a second collapse/expand.
- Variations: refresh racing with invalidation; listing truncated on refresh after a complete cached one.
- Risk: stale cache after reconnect (P24, P108).

### J-TREE-003 Same table open in three tabs, drop it from tree

- Status: new.
- Steps:
  1. Open `app.tmp` as permanent tab, as second tab with filter, and a console tab referencing it.
  2. Stage one edit in tab 1. Delete table from tree (confirm).
- Oracle: second client: table gone. Tabs: assumption: data tabs show error state on next load, staged edit
  discardable, no crash, no commit possible; console untouched. Persisted tabs after restart reopen without crash
  (`openTrackedTab` on missing object).
- Risk: crash on deleted object (P173), staged commit against recreated table of same name.

### J-TREE-004 Kafka and Mongo tree specifics

- Status: partial (ipc-frontend kafka; ui mongo fixtures).
- Steps:
  1. Kafka: expand Topics (500 topics) and Consumer groups; open topic; copy name.
  2. Mongo: expand database with 0 collections, with a view, with a time-series collection; Cmd+N new collection
     named `a.b$c` (invalid) then `événements`.
- Oracle: Kafka admin lists unchanged; Mongo `listCollections` shows `événements`, invalid name refused with driver
  text, no partial create.
- Risk: special characters in identifiers.

## C. Data grid: read, page, sort, filter

### J-GRID-001 Page through 1M rows forward and back with sort and count

- Status: partial (e2e-real postgres/mariadb keyset; local only). New: count plus sort plus oracle per page.
- Persona: analyst scanning `app.big_rows`.
- Steps:
  1. Open table, page size 1000. Next x5 clicking fast (double click each).
  2. Count all. Jump last page (if offered) or page size 10000 then Next.
  3. Sort by `created_at DESC` then Shift-click `id ASC` (badges 1, 2). Prev x2.
  4. Change page size to 10 mid-load. Press Stop during load.
- Oracle: after step 1 first row id equals row 5001 of `ORDER BY pk` from second client (no double advance, no
  skipped page); count equals `SELECT count(*)`; every shown page equals second client query with same ORDER BY
  and window; after Stop grid shows previous page intact, not half page.
- Variations: table without PK (offset paging, order not stable: assumption: stated in UI or ordered by ctid);
  composite PK; PK with NULL-able unique; rows inserted by second client between pages (keyset: no duplicate row).
- Risk: pagination double advance, stale count, superseded load landing late.

### J-GRID-002 Filter toolbar with history, autocomplete, bad SQL, Escape

- Status: partial (ui `data-view`; mock).
- Steps:
  1. Type `name LIKE 'A%' AND score > 5` in WHERE, pick column from autocomplete, Enter.
  2. Type broken `name = ` Enter (error). Escape (reverts to last applied text).
  3. ORDER BY `born DESC NULLS LAST`. Open recent menu, pick first entry. Save current as "A-people".
  4. Count all, then change filter: count shown must reset.
  5. Close tab, reopen table: saved filter in menu; restart: still there.
- Oracle: rows equal second-client `SELECT ... WHERE ... ORDER BY ...`; after step 4 count display cleared or
  recomputed, never old number; saved filter persisted in connection YAML or filter store, present after restart;
  history entries truncate on rune boundary for 500-char unicode filter.
- Variations: filter containing `;` and `--`; filter with `'` in value; unicode column name needing quotes; filter
  on masked-preview column.
- Risk: stale count, idle blur re-read, history truncation mid-rune.

### J-GRID-003 Go to FK, back, edit referenced row

- Status: partial (ui `interaction`).
- Pre: `orders.customer_id -> customers.id`, customer id is text `O'Brien/7`.
- Steps:
  1. Hover FK cell, open popover preview, Open in new tab.
  2. New tab filter text is plain filter; edit referenced row's `name`, commit.
  3. Return to orders tab; popover preview again.
- Oracle: generated filter quoted for dialect (`customer_id = 'O''Brien/7'`); second client sees one row updated;
  preview in step 3 shows new name (no stale preview cache).
- Variations: composite FK; NULL FK (no button); FK to table in other schema; MySQL backtick dialect; SQLite.
- Risk: dialect quoting in Go to PK/FK.

### J-GRID-004 Find bar over paged data with filter-rows toggle

- Status: partial (ui `tab-filter-state`).
- Steps:
  1. Page of 1000 rows. Find `ann` case-insensitive: N matches. Toggle regex `^Ann\b`, toggle word.
  2. Enable filter-rows: only matching rows shown, gutter keeps real row numbers.
  3. Select all visible (corner), Cmd+C.
  4. Next page with find bar open (rescan). Switch tab and back.
  5. Enter catastrophic regex `(a+)+$` on 10k page.
- Oracle: clipboard holds only visible rows, in order, full column set; match count equals independent regex
  count over second-client page values; find text and toggle survive tab switch; step 5 stops at scan cap without
  freezing UI over 1 s (assumption) and reports cap.
- Risk: copy includes hidden rows, superseded scan republished, ReDoS.

### J-GRID-005 Projection, column widths, font change, restart

- Status: partial (ui `slick-grid`; widths P195).
- Steps:
  1. Hide 3 columns via projection; try to hide the last one (refused: never zero columns).
  2. Resize `note` to 400px, keyboard-resize `name`. Reload. Change data font size in Settings.
  3. Copy row: only projected columns. Close tab, reopen, restart.
- Oracle: clipboard row has exactly visible columns in display order; widths after reload equal set values (keyed
  by column set); after restart widths kept if persisted (assumption: per inventory "kept across reloads", restart
  behaviour stated in test as observed contract); SQL sent (IPC args) lists only projected columns.
- Risk: zero-column query, widths lost, copy of hidden columns.

### J-GRID-006 Same table in two tabs and two windows after commit

- Status: partial (bug "committed mutation not reloading other tabs" fixed; ui mock).
- Steps:
  1. Tab A and tab B on `people`; window 2 tab C on same.
  2. In A edit row 1 `name`, commit. In B stage edit on row 1 `email` before A's commit, commit after.
- Oracle: second client row has both changes; B and C show A's new `name` after commit without manual refresh
  (same window; assumption for other window: refresh on focus or stale marker); B's staged edit survives A's
  reload.
- Variations: A deletes row B has staged edit on (assumption: B commit reports 0 rows affected error, not silent
  success).
- Risk: lost update, silent zero-row commit.

## D. Data grid: selection and clipboard

### J-GRID-010 Range copy fidelity with hostile values

- Status: partial (ui `grid-copy-menu` real clipboard on fixtures; hostile matrix new).
- Persona: analyst copying to spreadsheet.
- Pre: rows with values: `=1+1`, `+49 30`, `-5`, `@user`, TAB inside, LF inside, CRLF inside, `"quoted"`, empty
  string, NULL, 70 KiB text (truncated in grid), emoji ZWJ sequence, RTL text, `ᴺᵁᴸᴸ` literal, leading/trailing
  spaces, numeric `1e3`, boolean, bytea, jsonb `{"a":[1,2]}`.
- Steps:
  1. Drag-select 6x5 range crossing those cells; Cmd+C. Paste into plain text sink.
  2. Same range via context menu Copy; via Copy as CSV; via Copy as JSON.
  3. Single cell copy of the LF value, then one-cell range copy of it.
- Oracle: keyboard and menu TSV byte-identical; each field equals grid display text passed through `tsvField`;
  NULL yields empty field; single cell and one-cell range identical; JSON parses, keys in display order, NULL is
  `null`, empty string is `""`; CSV rows CRLF, formula cells prefixed `'`. TSV formula cells: inventory states
  neutralization for TSV, source applies it in CSV only. Assumption to settle in P628: TSV must also neutralize
  `= + - @`, or inventory wording is corrected; test records actual TSV bytes either way. Truncated cell copy:
  assumption: copies truncated text plus never claims full value (compare with Copy as INSERT which writes NULL
  and comment).
- Variations: selection under filter-rows (only visible rows); range dragged bottom-right to top-left; column
  order after projection; console grid with duplicate column names (`id`, `id_2` in JSON).
- Risk: example bug of chapter: right rows on screen, wrong clipboard.

### J-GRID-011 Row selection across pages and after sort

- Status: new.
- Steps:
  1. Ctrl-click gutter rows 2, 7, 9; Shift-click to 15. Right-click inside selection: menu is row menu, selection
     intact. Copy.
  2. Sort by `name`; selection: assumption cleared or follows rows; either way copy must equal what is shown
     selected.
  3. Next page: selection cleared (assumption). Prev page.
  4. Corner select all, Shift+Down (anchor from active cell), Cmd+C.
- Oracle: clipboard rows equal second-client values for exactly highlighted PKs, ascending display order; never
  rows from previous page or pre-sort positions.
- Risk: selection indices reused after page/sort replace, right-click collapses selection.

### J-GRID-012 Column copy and header menu

- Status: partial (ui `grid-copy-menu`).
- Steps:
  1. Click header `email` (column selection), Cmd+C.
  2. Ctrl-click second header; copy. Right-click a third, unselected header: Copy column.
  3. With filter-rows active repeat step 1.
- Oracle: step 1 clipboard = one value per line for every loaded row of page (not whole table), NULL as empty line;
  step 2 two columns tab-separated in display order; step 3 header right-click copies clicked column, selection
  unchanged; filtered copy holds visible rows only.
- Risk: header right-click copies selection instead of clicked column.

### J-GRID-013 Paste TSV block from spreadsheet into range, overflow into inserts

- Status: partial (unit `grid-paste-row-cell-kind-target`; ui `mutations` mock). New: DB oracle.
- Pre: page with 5 rows, PK `id`, generated column `full_name`.
- Steps:
  1. Copy 8x3 block in sheet (trailing newline, one cell quoted with LF, one with `""`).
  2. Select cell (row 2, `name`), Cmd+V.
  3. Preview pending changes: Changes and SQL tabs. Commit.
- Oracle: rows 2-5 staged as UPDATE of 3 columns from `name` rightwards; clipboard rows 5-8 become 4 inserts;
  generated column never in SQL; no bogus 9th row from trailing newline; quoted LF cell lands as one value;
  second client after commit: exact values. Paste that runs past last column: extra cells dropped (assumption).
- Variations: paste with Ctrl-selected rows 2, 7, 9 (maps to those rows, remainder inserts); paste under
  filter-rows (hidden rows skipped); paste single value `Main St, Apt 4` (one cell, no comma split); paste
  multi-line prose (ragged CSV: one cell); paste into open cell editor (one value); paste on table without PK
  (staging refused, message); paste into read-only/masked-preview grid (refused); clipboard permission denied.
- Risk: off-by-one rows, hidden-row writes, generated column UPDATE, comma split.

### J-GRID-014 Round trip copy from Kira, paste back in Kira

- Status: new.
- Steps:
  1. Select rows 1-3 fully, Cmd+C. Add nothing; select row 10 cell 1; Cmd+V. Commit.
  2. Compare rows 10-12 to rows 1-3.
- Oracle: second client: rows 10-12 equal rows 1-3 in every non-PK, non-generated column except: NULL source cells.
  NULL copies as empty field, so paste stages empty string, not NULL. Assumption to settle: either paste of empty
  field into nullable non-text column stages NULL, or test documents the loss and PK conflict error is shown
  cleanly (PK pasted over existing PK: commit fails, whole transaction rolled back, staged set kept).
- Variations: numeric/date/bool/jsonb columns receiving empty string (engine error surfaced, nothing committed);
  MySQL strict mode; values with tabs.
- Risk: NULL/empty conflation, partial commit.

### J-GRID-015 Copy as INSERT and run it in console

- Status: new (unit level only).
- Steps:
  1. Select 3 rows incl one with truncated 70 KiB cell, value `O'Brien`, NULL, backslash, `$$`.
  2. Copy as INSERT. Open console on empty clone table, paste, run.
- Oracle: statement runs without syntax error per dialect (Postgres, MySQL backslash escapes, SQLite); second
  client rows equal originals; truncated column NULL and leading SQL comment names it; identifiers quoted.
- Risk: quoting per dialect, truncated value written as real data.

### J-GRID-016 CSV export of filtered, sorted, projected page to disk

- Status: new (far end file).
- Steps:
  1. Filter, sort, hide 2 columns. Export CSV (Save dialog to `/tmp/x/people.csv`). Cancel once, then save.
  2. Open file with strict RFC 4180 parser.
- Oracle: cancel writes nothing; file has header row (assumption) and rows equal second-client query; CRLF line
  ends; formula prefixes neutralized; UTF-8 without mangling; NULL as empty; export scope stated (page vs all
  rows) matches file row count.
- Variations: 10000-row page; path with spaces and unicode; target not writable (error, no partial file).
- Risk: export ignores filter/projection, encoding.

### J-GRID-017 Drag selection with autoscroll then keyboard extend

- Status: partial (ui `interaction`).
- Steps:
  1. Mouse down row 5, drag below viewport (autoscroll to row 300), release.
  2. Shift+Up x3, Shift+Right x2. Cmd+C.
  3. Click elsewhere; Shift+click; Escape.
- Oracle: clipboard line count = 293 (rows 5-297) and column count = dragged + 2; first/last line match
  second-client rows 5 and 297; selection highlight equals copied range after virtualization re-render.
- Risk: rendered-band clipping changes copied range, anchor drift.

## E. Data grid: mutations

### J-GRID-020 Mixed change set, preview, partial failure, rollback

- Status: partial (ui `mutations`, `grid-commit-in-flight`). New: DB oracle at IPC boundary (coverage gap 5).
- Steps:
  1. Edit 3 cells in 2 rows, delete 2 rows, add 2 rows (one violates NOT NULL), duplicate a row.
  2. Open pending changes: Changes diff, SQL tab. Copy SQL.
  3. Commit: fails on NOT NULL. Fix cell. Commit.
  4. While commit in flight (slow server) edit another cell.
- Oracle: after failed commit second client sees zero changes (one transaction); error text visible and kept;
  staged set intact; after success: exact final rows; SQL tab text equals statements observed in server log
  (order: assumption deletes, updates, inserts or as previewed; test asserts preview equals executed); edit made
  during commit stays staged, not dropped, not committed.
- Variations: Revert single cell, Revert all, Revert after blur-stage; rails colours; commit with 500 staged rows.
- Risk: partial commit, lost error, dropped edits during commit (P168).

### J-GRID-021 Staged edits survive paging, sort, filter, refresh

- Status: partial (P174 unit/ui).
- Steps:
  1. Edit row PK 42 on page 1. Next page, edit PK 1042. Sort desc, filter `id > 40`, Refresh, change projection to
     hide edited column, show again.
  2. Second client updates PK 42's other column meanwhile.
  3. Commit.
- Oracle: both edits committed; PK 42 other column keeps second client's value (UPDATE touches only edited
  column, WHERE by PK); yellow rail on both rows whenever visible.
- Variations: edit PK value itself then page; table without full PK (staging refused with reason).
- Risk: staged edits keyed by row index.

### J-GRID-022 Delete last row of last page, add row on empty table

- Status: partial (P108 fix).
- Steps:
  1. Go to last page holding 1 row. Delete, commit.
  2. Truncate table by deleting all rows on page, commit. Add row on now-empty table, commit.
- Oracle: after step 1 grid steps back to previous page (not blank page); row counts match second client; insert
  into empty table works with all-default columns (assumption: `DEFAULT VALUES` or omitted columns).
- Risk: blank page, insert SQL with empty column list.

### J-GRID-023 Type edge edits per engine

- Status: partial (adapter suites; bug list section 4).
- Steps: edit and commit each: MySQL `BIT(8)` to `b'101'`; Postgres `numeric(5,5)` 0.99999; `timestamptz` with
  offset; `bigint` 9007199254740993; `jsonb` with unicode escapes; `bytea`; `uuid`; array `{a,"b,c"}`; `text`
  empty vs NULL (Set NULL action); MySQL `ENUM`; ClickHouse (insert only: edit refused).
- Oracle: second client value equals typed value exactly (no float rounding of bigint, no timezone shift, bit
  preserved); invalid input gives engine error and nothing committed.
- Risk: data fidelity (`$numberLong`, BIT, strict timestamp).

### J-GRID-024 Fake data generator into constrained table

- Status: partial (ui `fake-data`).
- Steps:
  1. Generate 500 rows for table with UNIQUE email, FK, CHECK (score between 0 and 10), generated column,
     `varchar(5)`.
  2. Preview, commit. Regenerate with other seed, commit again.
- Oracle: second client: 1000 rows, all constraints hold; generated column untouched; FK values exist in parent
  (assumption: sampled from parent or column excluded with notice); failure rolls back whole batch.
- Risk: type bound overflow, read-only columns written.

### J-GRID-025 Multi-line value into insert-row cell

- Status: new (open item: insert-row cell single-line input flattens line breaks).
- Steps: add row, paste 3-line text into `note` cell inline, commit; then same through cell editor dock.
- Oracle: defines current contract: inline input flattens (documented open item) and UI must not claim otherwise;
  via dock the DB value holds exact LFs. Test pins both so regression or fix is visible.
- Risk: silent data change on paste.

### J-GRID-026 Tab close and quit with staged edits

- Status: partial (open item: quit discards).
- Steps: stage edits; Cmd+W (guard dialog: Cancel keeps, Discard closes); stage again; switch module and back
  (kept); close window (assumption: same guard); quit app (accepted loss, no prompt).
- Oracle: second client never sees uncommitted values; after restart tab restored without staged set and without
  dirty marker.
- Risk: phantom dirty state after restart.

## F. Cell editor

### J-CELL-001 Edit JSON cell through dock with format, validate, undo

- Status: partial (ui `cell-editor`).
- Steps:
  1. Select `meta` cell holding compact JSON 20 KiB. Dock autodetects json. Beautify indented.
  2. Break JSON (delete brace): validate chip red. Cmd+Z until valid; Cmd+Shift+Z once; fix.
  3. Toggle Show changes diff. Select another cell (stage on selection change). Commit.
- Oracle: second client `meta::text` parses and equals edited document semantically; if user only beautified and
  reverted to compact, no change staged (assumption: beautify alone on jsonb stages nothing or stages equal
  value; test asserts no spurious UPDATE when text returns to original); undo stack never empties buffer.
- Variations: truncated cell (64 KiB cap): editor read-only or loads full value before edit, never stages prefix;
  xml, csv formats; override format to text.
- Risk: truncated value staged, lost dirty buffer on tab switch.

### J-CELL-002 Timestamp and encodings panes

- Status: partial.
- Steps:
  1. `born` cell: translate pane, pick date in calendar; epochMillis column: pane shows ISO; edit via pane.
  2. base64 column: decoded pane shows text; edit decoded text; hex column likewise with non-UTF-8 bytes.
  3. Generators: UUID v4, ULID, hex token, Now, each into new row cells. Commit.
- Oracle: second client: epoch value equals chosen instant (UTC, not local shift); base64 decodes to edited text;
  non-UTF-8 hex round trips byte-exact; ULID timestamp part decodes to commit time +/- 5 s; UUID version nibble 4.
- Risk: ULID timestamp bug, timezone shift.

### J-CELL-003 Dock height and state across tabs and restart

- Status: partial (`appflow` LayoutCellEditorHeight).
- Steps: resize dock to 300px; type unsaved text; switch tab (stages); back; switch module; restart.
- Oracle: height persisted (layout row); staged value present after tab switch, visible as yellow cell; after
  restart no staged value.
- Risk: lost edit on unmount.

## G. Query console

### J-CONS-001 Multi-statement script with every splitter trap

- Status: partial (unit sql split; ui `console`).
- Persona: developer pastes migration script.
- Steps:
  1. Paste script: `--` and `#` comments, `/* nested /* */ */`, `E'it\'s;'`, `$tag$ ; $tag$`, `[weird;ident]`,
     `"a;b"`, statement ending without semicolon, `DELIMITER`-free `CREATE FUNCTION ... BEGIN ...; END`.
  2. Run all. Then caret in statement 3, Run statement.
- Oracle: server statement log receives each statement exactly once, boundaries as intended; compound body case is
  documented open item: test pins that failure is reported per statement and nothing after failing statement runs
  (assumption) rather than silent half-run; result tabs count equals statements returning rows.
- Variations: per dialect (Postgres, MySQL, SQLite, ClickHouse); 10 statements with auto-explain on (skip past
  10); script of 2 MB.
- Risk: mis-split executing partial statements.

### J-CONS-002 Result grid copy with duplicate columns and Mongo formats

- Status: partial (ui `console`, `autocomplete`).
- Steps:
  1. `SELECT a.id, b.id, 1 AS x, 2 AS x FROM a JOIN b ...`. Select 2 rows, Copy as JSON; select range, Cmd+C;
     right-click on row selection (row menu, not cell menu).
  2. Mongo console `db.people.find({})`: Copy all as Shell, Canonical, Relaxed, plain; single document copy.
- Oracle: JSON keys `id`, `id_2`, `x`, `x_2` with correct values per position; TSV positional; Mongo Shell copy
  pastes back into `mongosh` and inserts equal documents (ObjectId, NumberLong beyond 2^53, Date, Decimal128,
  binary, nested arrays); Canonical parses with EJSON strict; one document per line in Shell form.
- Risk: duplicate names overwrite, `$numberLong` precision.

### J-CONS-003 Run, stop, close, rerun around reconnect

- Status: partial (unit console races).
- Steps:
  1. Disconnect connection. In console double-click Run (reconnect starts).
  2. Run long query, press Stop. Run again, close tab mid-run. Reopen console from op log Re-run.
- Oracle: server log: first statement executed once; cancelled queries absent from `pg_stat_activity` within 2 s;
  auto-explain not issued after tab close; Re-run reopens console at stored path with text, connection
  reconnected, does not auto-execute (assumption) or executes once if Re-run means run: pin actual.
- Risk: double run, leaked run after close.

### J-CONS-004 Format, undo, edit during format

- Status: partial (ui `console-format`).
- Steps: 400-line ugly SQL with trailing comment and no final semicolon; Shift+Alt+F; Cmd+Z; Shift+Alt+F and type
  during worker await; format again ("Already formatted").
- Oracle: buffer after undo byte-equal to original; typed characters during await never lost (format result
  discarded); formatted script still splits into same statements and returns same results as original.
- Risk: format then undo wipes query (P69).

### J-CONS-005 Caps and huge results

- Status: partial.
- Steps: `SELECT * FROM big_rows` (1M); result shows truncated flag at row/byte cap; Copy all; `SELECT repeat('x',
  5e6)`; 250-column select (column virtualization), scroll right, copy last column.
- Oracle: copy all row count equals shown rows, clipboard notes nothing beyond cap; truncated cell flagged; memory
  stays bounded (heap under threshold); last column values correct after horizontal virtualization.
- Risk: virtualization mis-maps column index on copy.

### J-CONS-006 Saved queries across windows and restart

- Status: partial (ui `saved-queries-sync`; `dbflow` journey).
- Steps: save "top customers" in window 1; rename in window 2; edit text in window 1 without save while window 2
  deletes it; restart.
- Oracle: window 1 draft not clobbered; Save after remote delete recreates or reports not found
  (`DeletedSavedQueryIsNotFound` contract), never silently lost; list after restart consistent in both.
- Risk: unsaved draft clobbered by remote refetch.

### J-CONS-007 Autocomplete and schema dialog after DDL

- Status: partial (ui `autocomplete`, `sql-schema`).
- Steps: run `ALTER TABLE people ADD nick text`; type `SELECT p.n` with alias: `nick` offered; open Schema (DDL)
  dialog, copy DDL; hover column whose comment holds `<script>` and backticks.
- Oracle: completion list includes new column without reconnect (assumption: after refresh trigger defined in
  test); copied DDL applies cleanly on empty database via second client; hover renders text, no HTML execution.
- Risk: stale schema cache, unescaped DB text.

### J-CONS-008 Explain and auto-explain threshold

- Status: partial (ui `console-explain`).
- Steps: set `explain_threshold_rows`; run query over threshold with auto-explain on; view plan; copy plan; run on
  MySQL 9.7 and ClickHouse; masked connection.
- Oracle: plan node count equals engine EXPLAIN JSON; exactly one EXPLAIN per statement in server log; on masked
  connection plan shows no literal values from masked columns.
- Risk: plan leak, duplicate explain.

### J-CONS-009 Redis and ClickHouse console quirks

- Status: partial (ipc-frontend).
- Steps: Redis `GET missing` (nil shown as nil, not error), `SELECT 1` (denied connection-state command), `KEYS *`
  over 100k keys (bounded), `SET k "a b"`; ClickHouse `SELECT 'ᴺᵁᴸᴸ'` vs `SELECT NULL`; copy both cells.
- Oracle: Redis value via `redis-cli`; nil reply no `E_QUERY`; ClickHouse literal string case is open item: test
  pins displayed/copied text so fix is visible.
- Risk: nil mapping, NULL marker confusion in clipboard.

## H. Masking and privacy

### J-MASK-001 Mask rules, preview toggle, copy, MCP read

- Status: partial (ui `mask-preview`, `connection-privacy`; `dbmcpflow` Masking).
- Persona: lead sets masking before enabling agent access.
- Steps:
  1. Privacy tab: mask `email` as email, `name` as name, `score` as number, `note` redact.
  2. Open grid, toggle mask preview: edits disabled. Select rows, Cmd+C, Copy as JSON, CSV export.
  3. Toggle preview off: real values, edits enabled.
  4. Agent calls `browse_table people` and `run_query SELECT email AS e, "email", lower(email) FROM people`.
- Oracle: with preview on every clipboard/export form holds masked values only (no real email substring);
  same input value masks to same output across rows and across grid vs MCP (deterministic); MCP result has no real
  value through alias, quoted alias or expression (refused or masked); rule saved in connection YAML; second
  window shows rules.
- Variations: column added by second client (New columns card, withheld until reviewed, 30 s recheck); view
  renaming masked column (open item: pin behaviour); Indic conjunct names (grapheme count parity open).
- Risk: masked value leak through copy path or alias.

### J-MASK-002 Column review lifecycle

- Status: partial (ui `connection-mask-review`).
- Steps: enable `mask_review`; second client adds `ssn`; agent `run_query SELECT * FROM people`; user gets masking
  review prompt; Allow `ssn`... then Undo; Mask as redact; Allow all on another table.
- Oracle: MCP results never include `ssn` values before review; after Mask as, values redacted; YAML records
  decisions; Undo returns column to withheld; masked-by-name column in other table not shown as "Needs review".
- Risk: review/mask mismatch (P596).
