# P627 journeys: Studio table grid (read, clipboard, mutations, cell editor)

Split from `journeys-studio-db.md` and `journeys-studio-views.md` (P627 first pass); J-ids unchanged.
Imagined usage and definition of correct. No implementation, no test code.
Conventions: `journeys-studio-connections.md` head.

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
  `null`, empty string is `""`; CSV rows CRLF, formula cells prefixed `'`. TSV formula cells stay raw: verified
  deliberate (`views/shared/clipboardFormats.ts:22` `neutralizeFormulaPrefix`, called only from `csvField` `:107`;
  `tsvField` `:42-45`; rationale `:32-38`). Inventory wording "TSV neutralized" is imprecise, not a defect; test
  pins raw TSV bytes. Truncated cell copy: assumption: copies truncated text, never claims full value (compare
  Copy as INSERT, which writes NULL and comment); OQ-GRID-2.
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

## G. Per-engine sessions (second pass)

Runs in seconds: flow half against real containers of adapter suites (general suite) or sqlite file fixtures; UI
half against mocked bridge with recorded pages. Slow queries simulated with `pg_sleep`-style fixtures under a
statement timeout of 200 ms or a fake adapter that blocks until cancelled; never real waiting.

Facts read from code (CodeGraph): staged changes keyed by full primary key, not page position, so they survive
paging, sort, filter, projection, Refresh (`grid/pendingChanges.ts:15-16`); row with hidden part of composite key
cannot be staged (`:130-131`); keyset paging for structured sort, offset when sort is typed text
(`adapters/relationalpage.go:69-73`); tiebreaker primary key, else first unique key, else sqlite rowid
(`:175-186`); page token bound to fingerprint of path, projection, filter, sort, page size, conditions
(`:114-122`); typed ORDER BY shows header arrows by best-effort parse only (`grid/sortTerms.ts`); NULL copies as
empty field and paste stages string unconditionally (`clipboardFormats.ts:48,98,113`, `grid/paste.ts:80,87`).

### J-GRID-030 Postgres: bulk correction over keyset-paged table, failed commit, retry

- Status: partial (flows studio mutate, ui grid edit; chain with second-client oracle new).
- Persona: support engineer fixing 40 customer rows from spreadsheet.
- Pre: `app.people` 25,000 rows; unique index on `email`; check constraint `score >= 0`; trigger setting
  `updated_at`; second client connected.
- Steps:
  1. Open table; page size 500; sort `name` ascending by header click (keyset).
  2. Filter `email like '%@old.example'` through filter toolbar; 1,240 rows match; count shown.
  3. Page 2; edit 3 `email` cells inline; page 3; edit 2 `score` cells, one to `-1`.
  4. Go back to page 2: 3 edits still marked. Change sort to `id` descending: marks follow rows.
  5. Paste 10x2 TSV block from spreadsheet over `name`, `note` (contains quotes, tab, newline, empty cells).
  6. Delete 2 rows; add 2 rows (one duplicate `email`).
  7. Open preview: read SQL; copy SQL.
  8. Commit: fails on check constraint.
  9. Verify nothing changed with second client. Fix `-1` to `1`; commit: fails on unique `email`.
  10. Discard the duplicate insert only; commit: succeeds.
  11. Second client changes one committed row; Refresh in Kira.
  12. Select edited rows across current page; Copy as CSV; export page to file.
  13. Restart app; reopen tab.
- Oracle: after 8 and 9 second client sees zero changed rows (one transaction, rolled back) and staged set intact
  with failing row marked and engine message quoted; preview SQL copied = statements executed (op log command
  equal); after 10 second client: 15 updates, 2 deletes, 1 insert, `updated_at` set by trigger and shown after
  reload; NULL in pasted empty cells per OQ-GRID-1; CSV on clipboard and file byte-equal for same rows, CRLF,
  formula cells prefixed; after 13 filter, sort, page size restored, no staged edits resurrected.
- Variations: commit with 1,000 staged rows; filter that excludes a staged row before commit.
- Risk: partial commit; staged marks attached to row index instead of key.

### J-GRID-031 MySQL and MariaDB: type matrix read, edit, copy, connection killed mid-commit

- Status: partial (adapter conformance covers reads; grid edit and clipboard far end new).
- Pre: table `t` with `BIGINT UNSIGNED` 18446744073709551615, `DECIMAL(65,30)`, `TINYINT(1)`, `BIT(8)`,
  `ENUM`, `SET`, `YEAR`, `DATETIME(6)`, `TIMESTAMP` (session time zone not UTC), zero date `0000-00-00` (sql_mode
  permitting), `JSON`, `BINARY(16)` uuid, `VARBINARY`, `TEXT` utf8mb4 emoji, `GEOMETRY`; same on MariaDB (JSON
  is alias of LONGTEXT there).
- Steps:
  1. Open table on MySQL; check every cell text against second client `SELECT CAST(... AS CHAR)`.
  2. Sort by unsigned column descending; page forward and back.
  3. Edit unsigned to max value, decimal with 30 fraction digits, enum to invalid value, set to two members.
  4. Edit `DATETIME(6)` through date picker; edit `TIMESTAMP`.
  5. Edit JSON to invalid JSON; then to valid with unicode escape.
  6. Edit `BINARY(16)` through cell editor hex pane.
  7. Preview; commit: enum error; fix; commit.
  8. Second client `KILL` Kira's connection during next commit (fixture hook).
  9. Read state; reconnect; retry.
  10. Copy row as TSV, JSON, INSERT; run INSERT on empty copy of table.
  11. Repeat 1, 3, 5, 10 on MariaDB.
- Oracle: no precision loss (text equal to second client, no float rounding of 2^64-1 or decimal); TIMESTAMP shown
  and written in one documented zone (pin: session zone of connection; OQ-GRID-3); zero date displayed, not
  crashed, edit of other column in that row does not rewrite it; invalid JSON refused by engine on MySQL, accepted
  as text on MariaDB (engine difference surfaced honestly); binary round trip exact bytes; step 8 result reported
  as failed with staged edits kept, second client shows whole transaction absent; INSERT replay yields rows equal
  by checksum.
- Risk: JS number coercion; time zone drift on edit of untouched cells.

### J-GRID-032 SQLite: rowid fallback, WITHOUT ROWID, generated columns, locked file

- Status: partial (sqlite adapter suite; UI chain new).
- Pre: file DB with: `logs` (no primary key), `kv` (`WITHOUT ROWID`, composite key), `calc` (generated column),
  `loose` (INTEGER column holding text and blob values), view `v_logs`; second process holding sqlite3 CLI.
- Steps:
  1. Open `logs`: paging works (rowid tiebreaker, hidden); sort by non-unique column; page forward 3, back 3.
  2. Edit a cell in `logs`; commit.
  3. Open `kv`; hide one key column through Columns menu; try edit.
  4. Show column again; edit key column value itself; commit.
  5. Open `calc`: generated cell edit attempt; edit base column; commit; generated value refreshes.
  6. Open `loose`: cells show stored text and blob in integer column; sort; copy column.
  7. Open view `v_logs`: edit attempt.
  8. CLI opens write transaction and holds it; Kira commits edit.
  9. CLI commits; retry in Kira.
  10. Replace DB file on disk with other schema while tab open; Refresh.
  11. Move file away; Refresh; move back; reconnect.
- Oracle: step 1 no duplicated or skipped row across 6 page moves (compare concatenated ids with CLI query); 2
  CLI sees change addressed by rowid; 3 edit refused with reason (partial key), nothing staged; 4 one row
  updated, key changed, staged entry addressed original row; 5 generated cell read-only, value recomputed; 6
  clipboard values equal CLI `quote()` semantics as text, sort order equals SQLite ordering (not JS); 7
  read-only state; 8 busy error surfaced after bounded wait, staged edits kept; 10 columns re-read, stale page
  token rejected with reload not crash; 11 clear error then recovery.
- Risk: rowid reuse after delete; affinity surprises in sort.

### J-GRID-033 ClickHouse: analytic types, offset paging, cancel, restricted user

- Status: partial (clickhouse adapter suite).
- Pre: MergeTree table with `UInt64` max, `Int128`, `Decimal(38,18)`, `DateTime64(9,'Asia/Tokyo')`, `Array`,
  `Map`, `Tuple`, `Nullable(String)`, `LowCardinality`, `UUID`, `IPv6`, `Nested`; 50M-row table for slow scan;
  user `ro` with `readonly=1`; user `limited` without grant on one column.
- Steps:
  1. Open table: every cell text equals `clickhouse-client` `FORMAT TSV` output for same row.
  2. Sort by `DateTime64` column; page 5 forward; jump back to first.
  3. Filter with ClickHouse function (`has(arr, 3)`); count.
  4. Open 50M table without filter; sort by unindexed column; press Stop after 100 ms.
  5. Check server `system.processes` through second client.
  6. Try inline edit (capability per `clickhouse/caps.go`); if offered: stage, preview, commit, re-read.
  7. Copy range with Array and Map cells as TSV and JSON.
  8. Connect as `ro`: edit affordances absent; console `INSERT` refused.
  9. Connect as `limited`: open table with denied column.
  10. Kill server connection (fixture); page forward; reconnect; page forward.
- Oracle: numeric text exact; time shown in column zone with zone visible; paging consistent with sort plus
  tiebreaker (no duplicate rows across pages when sort column has ties; ClickHouse has no enforced unique key:
  assumption: tiebreak on full sort key, duplicates reported honestly, OQ-GRID-4); 4, 5 query gone from server
  within 1 s, UI idle with cancelled state, not error toast; 6 mutation visible on re-read or edit disabled with
  reason, never silent no-op; JSON clipboard keeps arrays and maps as JSON values, not strings; 8 no mutation
  request leaves app (op log); 9 engine error names column, other tables usable; 10 reconnect then same page
  position.
- Risk: offset paging over unstable order; async mutation shown as done.

### J-GRID-034 Pagination style switches under concurrent writes

- Status: partial (adapter keyset tests; UI and concurrency new).
- Pre: Postgres table 10,000 rows, page size 100; second client script inserting and deleting rows on demand.
- Steps:
  1. Sort by header `score` descending (ties present): keyset. Record ids of pages 1 to 5.
  2. Second client inserts 50 rows sorting into page 2 and deletes 20 from page 4.
  3. Page back from 5 to 1, recording ids.
  4. Type `score desc, name` into ORDER BY box: offset mode; header arrows still shown.
  5. Page to 5; second client deletes 100 first rows; page 6.
  6. Type expression sort `length(name)`; page; arrows absent for that term.
  7. Clear typed sort; click header: keyset again from page 1.
  8. Change filter while on page 7.
  9. Change page size on page 7.
  10. Jump to last page; Next disabled; row add on last page; commit; row lands where sort puts it.
  11. Count: exact count on demand vs estimate label.
- Oracle: keyset: no id seen twice within one forward walk and none skipped that existed whole time; back walk
  shows current data, not cached page; offset: skipped rows after deletes acknowledged (inherent) but never
  duplicate crash; page token from old filter never reused (fingerprint mismatch gives reload at page 1, not
  error dialog); 9 restarts at first page with first visible row kept in view if possible (pin); last page
  `hasMore` false; count label distinguishes exact from estimate.
- Risk: off-by-one probe row shown as data; reversed "before" page in wrong order.

### J-GRID-035 NULL, empty string and whitespace round trip (confirmed gap)

- Status: new. Confirms inventory claim: NULL is lost on copy then paste.
- Pre: rows with `note` = NULL, `''`, `' '`, `'NULL'` literal, `'\N'`; nullable int column NULL and 0.
- Steps:
  1. Select 5x2 range; copy TSV. Inspect clipboard.
  2. Paste into same range of other rows; preview.
  3. Commit; read with second client `note IS NULL`.
  4. Copy as JSON; inspect.
  5. Copy as CSV; inspect.
  6. Paste TSV from spreadsheet holding empty cell into nullable int column; commit.
  7. Paste into NOT NULL text column.
  8. Use Set NULL action on pasted cells; commit.
  9. Undo staged paste before commit.
- Oracle: step 1 NULL and `''` both empty fields (current, `clipboardFormats.ts:48`); step 2 preview shows `''`
  for cells that were NULL (current, `grid/paste.ts:80,87`); recommended correct (OQ-GRID-1): paste of empty field
  into nullable non-text column stages NULL, into text column stages `''`, and preview marks which; test records
  current bytes and fails only on undeclared change; JSON keeps `null` vs `""`; step 6 today sends `''` to int
  column and engine rejects or coerces to 0 (MySQL non-strict): must never store 0 silently; 8 NULL stored; 9
  page values restored.
- Risk: silent NULL to empty conversion across thousands of rows.

### J-GRID-036 Large values and truncated cells

- Status: partial (ui truncation marker).
- Pre: Postgres rows with 5 MiB `text`, 2 MiB `bytea`, 1 MiB `jsonb`, 100,000-char single line; MySQL
  `LONGTEXT` 5 MiB.
- Steps:
  1. Open table: cells truncated with marker; page loads under budget.
  2. Copy truncated cell; copy range including it; Copy as INSERT.
  3. Open cell editor dock on truncated cell: full value loads; search inside; copy all.
  4. Edit one character at end; save into staged change; preview.
  5. Commit; second client compares md5.
  6. Inline-edit (no dock) a truncated cell.
  7. Export page to CSV file.
  8. Sort by large column; filter `length(note) > 1000000`.
  9. Open 3 tabs of same table; check retained bytes; close 2.
- Oracle: clipboard for truncated copy per OQ-GRID-2 (recommended: copy fetches full value or states
  truncation in status, never silently partial); dock clipboard length equals `octet_length`; md5 after edit
  equals expected (only last char changed, no truncation written back); inline edit of truncated cell blocked or
  loads full value first, never commits truncated text; CSV export holds full or marked values consistently
  with copy rule; retained bytes drop after tab close (`window.__kiraRetention`).
- Risk: writing truncated display text back to DB.

### J-GRID-037 Permissions and read-only paths

- Status: partial (complete suite least-privilege matrix; UI new).
- Pre: Postgres role `viewer` (SELECT on 2 of 3 tables, column-level SELECT without `email`), role `editor`
  (UPDATE on `name` only, no DELETE); row-level security policy hiding rows; MySQL user without `SHOW VIEW`.
- Steps:
  1. As `viewer` open allowed table; open denied table.
  2. Open table with denied column (`SELECT *` fails).
  3. Hide denied column through Columns menu; reload.
  4. As `editor` edit `name` and `score` in one row; commit.
  5. Remove `score` edit; commit.
  6. Delete row; commit.
  7. Table under RLS: count vs visible rows; edit row then policy hides it after update.
  8. Admin grants DELETE through second client; retry 6 without reconnect.
  9. Connection marked read-only in Kira with admin credentials: try edit, paste, delete, fake data.
- Oracle: denied table shows engine error with object name, tree still usable; 3 projection lets table load
  (query lists columns explicitly); 4 nothing committed (transaction), error names column; 5 one row changed; 6
  permission error, row intact; 7 affected-rows mismatch reported, not counted as success; 8 succeeds; 9 no
  mutation statement reaches server (second client `pg_stat_statements` or op log), UI affordances disabled.
- Risk: UI-only read-only guard.

### J-GRID-038 Cancel, timeout and reconnect during read and commit

- Status: partial (flows cancel; adapter abort tests).
- Pre: Postgres with view `slow_v` sleeping 10 s per row; connection statement timeout 300 ms on second
  connection entry; proxy fixture able to drop TCP.
- Steps:
  1. Open `slow_v`; Stop while loading.
  2. Open again on timeout connection.
  3. Open normal table; stage 5 edits; drop TCP; page forward.
  4. Reconnect prompt; reconnect; page loads.
  5. Commit staged edits.
  6. Stage edits; start commit; drop TCP after statements sent, before commit ack (fixture hook).
  7. Reconnect; Refresh.
  8. Start count on big table; close tab.
  9. Quit app during page load.
- Oracle: 1 backend pid no longer running query (second client `pg_stat_activity`), UI neutral cancelled state; 2
  timeout message names limit; 3 staged edits kept through drop; 5 applied; 6 outcome reported unknown or
  failed, never success; after 7 grid shows server truth and staged set either cleared (if applied) or kept (if
  not), never both applied and still staged (OQ-GRID-5); 8, 9 no orphan query on server.
- Risk: double apply on retry after ambiguous commit.

### J-GRID-039 Keyboard-only editing and selection session

- Status: partial (ui grid keyboard specs).
- Steps:
  1. Focus grid from tree with keyboard; arrows, PageDown, Home/End, Mod+Home.
  2. Shift+arrows select 4x3; Mod+C.
  3. Enter edits; type; Tab commits and moves; Shift+Tab back; Escape cancels edit.
  4. F2 or Enter on JSON cell opens dock; edit; Mod+Enter saves; focus returns to cell.
  5. Delete key on selection: sets NULL or empties (pin).
  6. Mod+Z undoes last staged change; Mod+Shift+Z redoes.
  7. Mod+F find; Enter cycles; Escape returns focus to grid cell.
  8. Context menu key: Copy as JSON.
  9. Add row shortcut; fill 3 cells; duplicate row; delete row.
  10. Open preview by shortcut; commit by shortcut; confirm.
  11. Next page by shortcut: focus stays in grid, same column.
- Oracle: clipboard at 2 and 8 exact; staged set after 6 equals expectation at each undo depth; second client
  rows after 10; focus location asserted after each step (no focus loss to body).
- Risk: focus trap after dock close.

### J-GRID-040 Same row edited in two tabs, two windows and by second client

- Status: new.
- Steps:
  1. Tab A and tab B (other window) on same table; both stage edit of row 7 `name`.
  2. A commits. B still shows own staged value and old original.
  3. B refreshes: staged edit kept, original updated.
  4. B commits.
  5. Second client deletes row 9; A stages edit on row 9; commits.
  6. Second client changes PK of row 3; A edits row 3; commits.
  7. A stages delete of row 5; B stages edit of row 5; A commits; B commits.
- Oracle: 4 last write wins and preview showed true before value (current server value), or conflict warning
  (OQ-GRID-6: recommended warn when original differs from server); 5, 6, 7 update affecting 0 rows reported as
  failure with row marked, transaction rolled back, never counted as success.
- Risk: silent lost update; zero-row update treated as success.
