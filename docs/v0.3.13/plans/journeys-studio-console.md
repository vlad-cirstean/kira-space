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

## I. Per-engine console sessions (second pass)

Runs in seconds: flow half against adapter containers or fakes; slow statements use engine sleep under Stop or a
blocking fake; Kibana against recorded Elasticsearch and Kibana HTTP fixture (`flowharness.HTTP`); relative time
ranges resolved against injected "now".

Facts read from code (CodeGraph): Run, Run all and Explain reconnect first when connection dropped
(`console/useConsoleRun.ts:31-39,77-79`); Run executes statement at cursor, Run all splits whole text by dialect
(`:69-92`); Explain only for explainable statement at cursor (`:55-60`); console batch cap 10,000 rows and 56 MiB
shared by all statements of batch, exhausted budget still admits 1 row then reports truncated
(`adapters/consolecap.go:12,30-35`); Mongo statement grammar `db.<collection>.<method>(...)`
(`console/mongoStatement.ts:4`); Copy all formats `plain`, `shell` (one document per line), `canonical`,
`relaxed` (`console/copyAll.ts:46-57`); Kibana Discover: hits and histogram send same request (`useDiscover.ts:
89-103`), sample sizes 100, 500, 1000, 5000, 10000, new data view drops sort and columns and reruns once fields
arrive (`:198-203`), histogram needs absolute range (`kibana/discover.go:73-81`), histogram cancel sends
`opsCancel` (`useDiscover.ts:115-121`), value suggestions capped and flagged `Complete`
(`discover.go:161-245`).

### J-CONS-020 Postgres: migration script with transaction, error mid-batch, capped result

- Status: partial (flows console run; UI console specs; chain new).
- Persona: developer rehearsing migration against staging copy.
- Pre: Postgres; second client; table `big` 50,000 rows with 2 KiB text.
- Steps:
  1. Paste script: `BEGIN;` DDL adding column; `UPDATE` with `RETURNING`; `SELECT` with dollar-quoted body holding
     `;`; statement with syntax error; `COMMIT;`.
  2. Run all.
  3. Read result strip: one entry per statement, error on 5th, later statements not run.
  4. Second client checks column absent (transaction open or aborted?).
  5. Run `ROLLBACK;` alone (cursor on it).
  6. Fix error; Run all: completes.
  7. Completion after DDL: type `select new_` and accept new column.
  8. Run `SELECT * FROM big` then `SELECT 1` as batch.
  9. Open result 1: 10,000 rows, truncated note; result 2 present.
  10. Copy result 1 selection as TSV; Copy all as CSV; copy column.
  11. Sort result grid client side; find in results; copy again.
  12. Run `SELECT pg_sleep(30)`; Stop after 100 ms; run next statement at once.
  13. Run `RAISE NOTICE` block and `EXPLAIN ANALYZE`; open plan view.
  14. Close tab with open transaction (`BEGIN` run alone before).
  15. Restart app; reopen console tab.
- Oracle: second client after 3: no schema change visible; after 5 none; after 6 column exists and rows updated
  equal to `RETURNING` count shown; op log has one row per statement with exact text (dollar-quoted body
  unsplit); result 1 row count 10,000 with truncation flag, byte budget shared (result 2 still delivered);
  clipboard TSV equals displayed order after client sort; 12 backend query cancelled (`pg_stat_activity`), next
  statement runs on same session; 14 transaction not left open on server (session closed or rolled back, no
  idle-in-transaction row); 15 text restored, results not restored (pin), no auto-run.
- Risk: statement splitter; dangling transaction holding locks.

### J-CONS-021 MySQL and MariaDB: delimiters, procedures, multiple result sets, session state

- Status: partial (splitter unit tests; engine far end new).
- Steps:
  1. Paste script with `DELIMITER //`, procedure body holding `;`, `DELIMITER ;`, `CALL p()` returning 3 result
     sets and warnings.
  2. Run all.
  3. Check each result set tab; affected-rows line for DML inside.
  4. Run `USE other_db;` then `SELECT DATABASE()` as separate runs.
  5. Run `SET @x := 5;` then `SELECT @x` in separate runs.
  6. Run statement with backtick identifiers, `#` comment, `-- ` comment, `/*! hint */`.
  7. Run `SELECT` returning duplicate column names and 0 rows; copy headers.
  8. Run `INSERT ... ON DUPLICATE KEY UPDATE`; affected count 2 semantics.
  9. Run `SHOW WARNINGS` after truncating insert (non-strict mode).
  10. Run long `SELECT SLEEP(30)`; Stop.
  11. Kill connection from second client; press Run.
  12. Repeat 1 to 5 on MariaDB.
- Oracle: procedure created once (second client `SHOW CREATE PROCEDURE` body byte-equal); 3 result sets shown in
  order; 4 and 5 session state persists between runs of same tab (one session per tab: pin) and is not shared
  with other tabs; duplicate headers copied as `x`, `x_2`; affected rows shown as engine reports; 10 `SLEEP`
  interrupted (returns 1) and UI cancelled; 11 reconnect first, then run; session state (`@x`, `USE`) lost and
  user told (OQ-CONS-1).
- Risk: silent loss of session state after reconnect.

### J-CONS-022 SQLite: pragmas, attach, write lock, file-level effects

- Status: new.
- Steps:
  1. Run `PRAGMA journal_mode`, `PRAGMA table_info(t)`: results as grids.
  2. `ATTACH DATABASE '<tmp>/other.db' AS o;` then `SELECT` joining both.
  3. `CREATE TABLE`, insert 3 rows, `SELECT changes()`.
  4. `BEGIN IMMEDIATE;` run alone; in grid tab of same connection commit an edit.
  5. `COMMIT;`.
  6. Second process locks DB; run `INSERT`.
  7. Run `VACUUM`; file size on disk changes.
  8. Run statement with trigger body (`BEGIN ... END;`) through Run all.
  9. Read-only connection: `INSERT` refused; `PRAGMA query_only`.
  10. Explain: `EXPLAIN QUERY PLAN` view.
- Oracle: file on disk read by sqlite3 CLI after each write; 4 grid commit either waits and fails busy or shares
  session (pin), never deadlocks UI; 6 busy error within bounded time; 8 trigger created whole; 9 no write to
  file (mtime unchanged).
- Risk: splitter on `BEGIN ... END`; lock between app's own connections.

### J-CONS-023 ClickHouse: settings, formats, long query cancel, numeric fidelity

- Status: partial (adapter console tests).
- Steps:
  1. `SELECT number FROM system.numbers LIMIT 100000`: capped at 10,000, note shown.
  2. Query with `SETTINGS max_threads=1` and trailing `FORMAT JSON` clause.
  3. `SELECT toUInt64(18446744073709551615), toDecimal128('1.000000000000000001', 18), [1,2], map('a',1)`.
  4. `INSERT INTO t VALUES` 3 rows; `SELECT count()`.
  5. `ALTER TABLE t UPDATE` mutation; immediate `SELECT`.
  6. Long aggregation over 50M rows; Stop.
  7. Multi-statement text with `;` in string literal; Run all.
  8. `SHOW CREATE TABLE`; copy cell (multi-line) as TSV.
  9. Read-only user: `INSERT`.
  10. Explain: `EXPLAIN PLAN`, `EXPLAIN PIPELINE` in plan view.
- Oracle: 2 `FORMAT` clause handled (stripped or refused with message, never garbage grid: OQ-CONS-2); 3 text
  exact; 5 UI does not claim rows updated synchronously; 6 server `system.processes` empty for query id within 1
  s; clipboard of DDL cell quoted per `tsvField` and pastes back as one cell.
- Risk: user `FORMAT` clause breaking result parsing.

### J-CONS-024 Mongo console: statement grammar, EJSON results, Copy all in 4 formats

- Status: partial (ui console mongo copy; lint unit tests).
- Pre: collection `orders` 3,000 docs with ObjectId, Date, Decimal128, Long, Binary, UUID, regex, nested arrays,
  `null`, missing field, key with dot and `$`, 2 MiB document.
- Steps:
  1. Type `db.orders.find({status: "open"}).limit(5)`; lint clean; Run.
  2. Type unterminated string; lint shows single problem; Run blocked or engine error.
  3. `db.orders.aggregate([{$group: {_id: "$status", n: {$sum: 1}}}])`.
  4. `db.orders.find({_id: ObjectId("...")})`, then `{created: {$gt: ISODate("2026-01-01")}}`.
  5. `db.orders.updateMany({status:"open"},{$set:{flag:true}})` on read-only connection, then on writable.
  6. `db.orders.drop()` typed: confirm path (pin) or refuse.
  7. `db.nope.find({})` (missing collection): empty result, no error.
  8. `db.orders.find({})`: result capped; page through result strip.
  9. Copy all as plain, shell, canonical, relaxed.
  10. Filter-rows find `open`; Copy all again.
  11. Copy single document from tree view; copy one field value.
  12. Explain `find` with sort without index: plan view shows COLLSCAN issue.
  13. Stop long `$where` sleep query.
- Oracle: mongosh as second client: step 5 read-only: zero modified; writable: `modifiedCount` equals shown;
  Copy all: `plain` parses as JSON array with dates as strings, `shell` has one document per line and each
  line evaluates in mongosh to equal document, `canonical` round-trips through `EJSON.parse` to BSON-equal
  documents (Decimal128, Long, Binary subtype kept), `relaxed` parses; count of items equals displayed documents
  (step 10: only matching ones); 2 MiB document present whole or with truncation stated, array still valid JSON;
  13 `currentOp` shows operation killed.
- Risk: type loss in "plain"; Copy all ignoring filter-rows.

### J-CONS-025 Redis console: command set, bounds, binary replies, database scope

- Status: partial (redis console tests).
- Steps:
  1. `SET k v`, `GET k`, `HSET h a 1 b 2`, `HGETALL h`, `LRANGE l 0 -1`, `ZRANGE z 0 -1 WITHSCORES`.
  2. `SCAN 0 COUNT 100` then `KEYS *` on 1M-key DB.
  3. `SELECT 3` then `GET k`.
  4. `MULTI`, `INCR c`, `EXEC` as 3 runs and as one Run all.
  5. `SUBSCRIBE ch`, `MONITOR`, `BLPOP l 0`.
  6. `FLUSHDB` on writable and on read-only connection.
  7. `GET bin` (binary value with NUL and invalid UTF-8); copy result.
  8. Quoted args: `SET "a b" "x\ny"`; `GET "a b"`.
  9. Unknown command; wrong arity.
  10. `EVAL` script returning nested array.
  11. `DEBUG SLEEP 30`; Stop.
- Oracle: redis-cli as second client sees writes in expected database (3: tab's own DB scope kept or `SELECT`
  refused with reason, never writes to other DB silently: OQ-CONS-3); blocking and streaming commands refused
  up front with message (per `redis/consolebound.go`); `KEYS *` bounded or refused; read-only refuses writes
  before send; binary shown escaped and clipboard round-trips through `redis-cli` to same bytes; 4 transaction
  semantic preserved or refused, never half applied.
- Risk: destructive command on wrong DB.

### J-CONS-026 Explain across engines with plan issues

- Status: partial (plan parser unit tests; UI explain spec).
- Steps:
  1. Postgres: cursor in `SELECT` with seq scan on 1M rows: Explain enabled; run; plan tree; issue badges.
  2. Cursor on `UPDATE`: Explain (no execution of write: `EXPLAIN` without `ANALYZE`, or transaction rolled back).
  3. Cursor on `CREATE TABLE`: Explain disabled with tooltip.
  4. Cursor between two statements; in comment; in empty line.
  5. MySQL: `EXPLAIN FORMAT=JSON` plan view; MariaDB variant.
  6. SQLite, ClickHouse, Mongo plan views.
  7. Copy plan as text; copy node detail.
  8. Run while Explain in flight; Explain while run in flight.
  9. Explain on dropped connection: reconnects first.
  10. Auto-explain threshold: slow statement shows hint; open plan from hint.
- Oracle: second client row counts unchanged after 2 (no write executed); disabled states match
  `isExplainable`; op log shows exact explain statement sent; clipboard plan text equals engine's raw plan; 8
  second action refused while running, no interleaved results.
- Risk: explain executing DML.

### J-CONS-027 Saved queries, format, completion and reconnect in one working session

- Status: partial (ui console saved, format specs).
- Steps:
  1. Type lower-case multi-join query with comment; Format; Mod+Z restores exactly; Format again.
  2. Save as `weekly`; edit text; dirty marker; Save; Save as copy.
  3. Open second console on other connection of same engine; open `weekly`.
  4. Rename and delete saved entries from menu with keyboard.
  5. Completion: schema-qualified table, alias column, keyword case; accept with Tab.
  6. Hover table name: columns shown; go to definition.
  7. Drop connection (fixture); press Run: reconnect then run.
  8. Reconnect fails (bad password now): error shown inline; fix password in connection dialog; Run.
  9. Open same saved query in second window; edit both; save both.
  10. Restart app.
- Oracle: formatted text re-parses to same statement (second client `EXPLAIN` identical plan), comment kept;
  saved rows in app DB (name, body, connection kind) after each step; clipboard none; 7 exactly one reconnect
  and one execution (op log); 9 last save wins with warning or both kept (OQ-CONS-4); after 10 tabs, text,
  cursor, saved link, dirty state restored.
- Risk: format changing semantics; double run after reconnect.

### J-KIB-010 Kibana Discover: incident window, filters, histogram agreement, saved search

- Status: partial (ui kibana discover; adapter kql tests).
- Persona: engineer chasing error burst.
- Time: relative ranges ("last 15 minutes") resolved against injected now; fixture indices stamped relative to it.
- Pre: fixture with 2 data views (`logs-*` with time field, `users` without), 200,000 hits, fields text plus
  keyword sub-field, ip, boolean, nested object, scripted or runtime field.
- Steps:
  1. Open Discover console; pick `logs-*`; time last 15 minutes; Run.
  2. Histogram and hit count agree; bucket interval auto.
  3. Type KQL `level:error and service.name:"check` : completion offers field names, then values; accept.
  4. Run; add filter from field sidebar (+ on value), then negate it, then pin/disable (per `FilterBar`).
  5. Drag-select histogram range: time becomes absolute; rerun.
  6. Change interval; histogram refetches, hits do not.
  7. Add 3 columns from sidebar; sort by `@timestamp` desc then `duration`.
  8. Sample size 10,000; scroll to end; expand document; copy document JSON; copy cell.
  9. Switch language to Lucene; query `level:error AND message:/time.*out/`.
  10. Switch data view to `users`: sort and columns dropped; time picker hidden; reruns once.
  11. Back to `logs-*`; Save as `errors-checkout`; change filter: dirty; Save; Reset.
  12. Open Operations; re-run earlier Discover op: controls hydrate from it.
  13. Start slow search (fixture delay); Stop; histogram request cancelled too.
  14. Fixture returns 401, then shard failures with partial hits, then `timed_out: true`.
  15. Restart app; reopen tab.
- Oracle: wire recorder: hits request and histogram request carry identical query, filters, time range, zone;
  histogram total equals hits total (same relation `eq` or `gte` shown); step 5 range ISO strings equal bucket
  bounds; value suggestions request has `terminate_after` and prefix regex escaped (`check` with special chars
  variation); 10 no request with stale sort field; saved query body in app DB equals controls; 12 controls equal
  re-run request; 13 `opsCancel` sent, no late histogram painted; 14 distinct messages, partial results labelled;
  clipboard document equals `_source` JSON; after 15 state restored, no auto-run (pin).
- Risk: histogram and table disagreeing; stale data view fields.

### J-KIB-011 Kibana Discover edge data and permissions

- Status: new.
- Steps:
  1. Data view list of 500 entries: search, keyboard pick.
  2. Data view whose index pattern matches nothing; data view deleted in Kibana after selection.
  3. Field with 10,000-char value, array of objects, geo point, `_id` with slash, missing time field value.
  4. KQL with nested field, wildcard field name, quoted phrase with escaped quote, invalid syntax.
  5. Time range `to` before `from`; time zone other than UTC; DST boundary day.
  6. Space-restricted user: data views forbidden; index read forbidden.
  7. Copy 50 selected rows; export if offered.
  8. Field sidebar counts (top values) on sampled hits.
- Oracle: errors map to plain messages (`kibanaAbsoluteRange`, forbidden, not found) without stack text; invalid
  KQL flagged before send (zero requests); DST: bucket keys continuous, no duplicate hour label; clipboard rows
  equal displayed columns in order with raw (not truncated) values; sidebar states sample basis.
- Risk: time zone off-by-one-hour windows.
