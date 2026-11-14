# P627 journeys: Studio connections, masking, shared conventions

Split from `journeys-studio-db.md` and `journeys-studio-views.md` (P627 first pass); J-ids unchanged.
Imagined usage and definition of correct. No implementation, no test code.

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
