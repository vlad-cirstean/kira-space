# P627 journeys: Studio consoles (SQL, Mongo, Redis, Kibana Discover)

Split from `journeys-studio-db.md` and `journeys-studio-views.md` (P627 first pass); J-ids unchanged.
Imagined usage and definition of correct. No implementation, no test code.
Conventions: `journeys-studio-connections.md` head.

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

## F. Kibana Discover

### J-KIB-001 Investigate incident with KQL, histogram, saved query

- Status: partial (`dbflow` KibanaDiscoverJourney, KibanaSavedQueriesJourney; ui `kibana-discover`).
- Steps:
  1. Open Discover on data view; type `service.name : "api" and http.status >= 500 and not url.path : /health*`
     using field/value/operator autocomplete.
  2. Time range last 15 min; histogram renders; drag-select bucket to narrow (if offered).
  3. Pick columns `@timestamp`, `message`, `trace.id`; expand hit; copy field value with quotes and newlines.
  4. Cmd/Ctrl+S save as "5xx api"; switch tab and back (KEEP_WHILE_OPEN); restart; reopen saved.
  5. Invalid KQL `foo : (` shows compile error with position.
- Oracle: request body sent to Kibana (recorded by container proxy or `Kibana` IPC args) holds compiled ES query
  equal to reference compile of KQL; hit count equals direct ES `_count`; clipboard value exact; saved query present
  after restart with columns and time range (assumption: columns saved too; pin).
- Variations: value with `"` `\` `*`; nested fields; wildcard field; 0 hits; 10k hits; Kibana 401.
- Risk: KQL compiler escapes, state lost on tab switch.
