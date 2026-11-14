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
