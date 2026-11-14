# P629 Stream A: Studio data, console, connections, API collections, gRPC

Implementer plan (Sonnet). Source of decisions: `synthesis.md` §6; journey text: `journeys-studio-*.md`,
`journeys-bug-classes.md`, `journeys-http-grpc*.md`. Read each journey's Steps and Oracle before writing its test.
Committed set: 57 primary journeys (33 P1, 24 P2) plus 40 merged as variations. Work in item order: P1 items first.
Nothing here is optional.

## Rules for this stream

- Worktree: own git worktree off chapter branch head at stream start. Commit per work item (or per logical group of
  product fix plus its test). Conventional Commits (`test:`, `fix:`).
- Split rule (CLAUDE.md Tests): `NEW both` means one scenario, two halves.
  - Flow half drives the bound service or data plane and asserts the far-end oracle.
  - Flow half records request payload and result via `app.Contract(t, "<scenario>", key, v)`. Regenerate with
    `KIRA_CONTRACT=write`.
  - UI half (`tests/ui/studio/...`) drives screen, then asserts outgoing IPC payload equals
    `contract("<scenario>", key)`. It also asserts screen shows contract result.
  - Contract scenario names in this stream start `studio-` or `api-`; never reuse another stream's prefix.
- Seconds per test.
  - No `time.Sleep`, no `waitForTimeout`. Wait on events (`app.Events.WaitAfter`), Playwright auto-wait or
    `expect.poll`.
  - Fake clock via `flowharness.WithClock` / UI `relaunch({clockTime})`.
- Containers:
  - Real-container flow tests call `flowharness.RequireDocker(t)` and start fixtures with
    `testsupport.Start<Kind>(t)`, like `dbflow/kibana_test.go` does.
  - One engine per kind in normal loop. Extra engines (MariaDB beside MySQL, second ClickHouse user) run behind
    `flowharness.Complete(t)`.
  - Adapter-level functional cases go into each adapter's complete suite through `testsupport.Scenario{Name,
    Requires, Run}` (`internal/adapters/testsupport/matrix.go`), never a new mechanism.
- Second client = independent connection outside app (`sql.Open`, mongo/redis/kafka/sqs/s3 SDK client from
  fixture). Never read app state to prove app wrote it.
- Clipboard bytes: `installClipboardSpy` from `tests/ui/studio/support/clipboard.ts` (this stream owns it). Real
  clipboard round trip only in `ui-clipboard` project.
- Product bugs a test exposes:
  - Root-cause and fix in this stream, test and fix in one commit group.
  - Design decisions go to Follow-ups below. Do not half-fix.
  - USER items (`synthesis.md` §5) are never decided here: test only the agreed part.
- Run own new/changed specs only:
  - `go test -p 1 ./apps/kira-space/internal/flows/dbflow/ -run <Test>`.
  - `bun run build:test && bunx playwright test --config=apps/kira-space/playwright.config.ts --project=ui <spec>`.
  - Clipboard specs: `--project=ui-clipboard`.
  - Complete-suite cases: `KIRA_FLOW_COMPLETE=1 KIRA_FLOW_DOCKER=require`.
- Fast checks per commit: typecheck, lint (pre-commit hook). Never `--no-verify` on a commit that ships.

## A0 Harness: data-plane client and second clients

No item below can assert a Studio write at IPC boundary without this. `Router` exposes `Execute`, but grid
read/count/preview/mutate only exist on frame path (`adapterhost/dataframe.go` `HandleDataFrame` to
`Dispatcher.Mutate`, `adapterhost/data.go:154`).

- New `apps/kira-space/internal/flowharness/datastream.go`:
  - `func (a *App) DataStream(t testing.TB) *DataStream`.
  - Builds an in-memory pipe pair like `pipeConn` in `flowharness/stream.go`.
  - Runs `bridge.ServeEngineStream(a.W.Router, server)` (`internal/bridge/studio_stream.go:22`) in a goroutine.
  - Reads responses on client side.
- Methods send JSON request frames `{kind:"req", id, op, payload}` exactly as renderer does
  (`frontend/src/studio/bridge/data.ts`):
  - `Read(payload) (Page, error)`, `Count`, `Preview(payload) ([]string, error)`.
  - `Mutate(payload) (affectedRows int, err error)`, `Execute`.
- Decode FlatBuffers `wire.Frame` (`internal/page/wire`) the way `adapterhost/dataframe_test.go` `readSentFrame`
  does. Expose page rows as `[][]any` with NULL as `nil`, plus error code from `Error().Code()`.
- `Close()` waits for goroutines with a 10 s cap, like `GitStream.Close`.
- Payload structs: reuse `adapterhost.MutateRequestWire` and siblings in `adapterhost/wire.go`. Never hand-roll a
  divergent shape.
- No edit to `harness.go`: method lives in new file.
- New `apps/kira-space/internal/flows/dbflow/second_client_test.go` (package-local helpers):
  - `sqliteRows(t, path, query) [][]any`.
  - `pgRows(t, fx, query)`, `mysqlRows`.
  - `mongoDoc(t, fx, coll, id)`, `redisTTL(t, fx, key)`, `s3Head(t, fx, bucket, key)`.
  - `kafkaConsume(t, fx, topic, partition, from)`, `kafkaGroups(t, fx)`, `sqsAttrs(t, fx, queue)`.
  - Each wraps fixture client from `internal/adapters/testsupport`.
- Commit: `test: add data-plane client and second-client helpers for dbflow`.

## A1 Grid write at IPC boundary (P1)

- Journeys:
  - Primary: J-GRID-020, J-GRID-030, J-GRID-032, J-GRID-021.
  - Merged: J-GRID-022 (delete last row of last page, insert on empty table), J-GRID-025 (multi-line value),
    J-BC-051 (identifiers from hell), J-BC-061 (revert semantics).
- Files:
  - New `dbflow/grid_mutate_test.go`: SQLite. `TestGridMutateChangeSet`, `TestGridMutateSQLiteRowid`.
  - New `dbflow/grid_mutate_pg_test.go`: Postgres. `TestGridMutatePgKeysetRetry`.
  - New ui `tests/ui/studio/grid-commit-roundtrip.spec.ts`.
  - EXT ui `tests/ui/studio/mutations.spec.ts`: J-GRID-021 staged set survives paging, sort, filter, refresh;
    revert of one cell, one row, all. Final commit payload equals contract `studio-grid-mutate` key `mutate:ops`.
- Scenario (contract `studio-grid-mutate`):
  - Table `we"ird t` with columns `id` PK, `"select"` text, `ünï` int null, `note` text. Composite-PK second table.
  - One plan with update, insert (multi-line `a\r\nb\n`), delete; preview, then mutate.
  - Second plan whose 2nd op violates a constraint.
- Oracle:
  - Second client rows equal expected after plan 1 (byte-equal multi-line, quotes).
  - Plan 2: on transactional engines (SQLite, Postgres) nothing changed and error names failing op. Read
    `adapterhost/data.go:151` comment: six adapters mutate without transaction.
  - `Preview` statements equal contract and never interpolate unescaped identifiers.
  - Cache invalidated: `Read` after mutate returns new values (`InvalidateAfterMutation`).
- J-GRID-030 (Postgres): 2,000-row table paged keyset by `Read` cursor. Bulk update across pages via staged ops
  keyed by full PK. First mutate fails (unique violation), retry after fix succeeds. Second client: exactly intended
  rows changed, none twice.
- J-GRID-032 (SQLite):
  - rowid table without PK: mutate keyed by rowid.
  - WITHOUT ROWID table.
  - Generated column: edit refused before engine, or engine error surfaced; never silently dropped.
  - File locked by second client `BEGIN EXCLUSIVE`: mutate returns busy error. After release, retry lands once.
- Expected defects to check: identifier quoting in `adapters/sqlmutate.go` `RunRelationalMutate` /
  `postgres/mutate.go` `literalRenderer`; generated column included in INSERT; partial plan leaving cached stale
  page.

## A2 Clipboard fidelity and paste round trip (P1)

- Journeys:
  - Primary: J-GRID-010, J-GRID-013, J-GRID-035, J-GRID-036 (P2), J-GRID-039 (P2), J-BC-001, J-BC-002 (P2),
    J-BC-003.
  - Merged: J-GRID-011, J-GRID-012, J-GRID-014, J-BC-005, J-BC-006, J-BC-050, J-BC-054.
- Files:
  - New ui `tests/ui/studio/grid-clipboard-roundtrip.spec.ts`. Add to `ui-clipboard` project `testMatch` in
    `apps/kira-space/playwright.config.ts` (real clipboard read needed for paste).
  - New ui `tests/ui/studio/native-copy.spec.ts` (J-BC-003).
  - New ui `tests/ui/studio/grid-keyboard-edit.spec.ts` (J-GRID-039).
  - New flow `dbflow/grid_paste_test.go`.
- Copy bytes (spy, contract `studio-grid-clipboard` holds expected byte strings):
  - Hostile values: `=1+2`, `+x`, `-y`, `@z`, leading TAB, CR, embedded `\t`, `\n`, `"`, emoji, RTL, 64 KiB cell,
    NULL, empty string, whitespace-only, bytea/binary.
  - TSV raw: no neutralization, per `clipboardFormats.ts:42-45` (verified claim).
  - CSV neutralized with `'` prefix, RFC 4180 quoting, CRLF.
  - JSON keeps `null`.
  - Binary shows marker, not mojibake.
  - Row selection across pages and after sort copies rows in displayed order (J-GRID-011).
  - Column copy from header menu (J-GRID-012).
  - Select-all on 10,000-row page then Shift+extend copies exact count (J-BC-006).
- Paste (J-GRID-013, J-GRID-035):
  - Paste TSV block larger than range: overflow rows become staged inserts.
  - Commit payload equals contract `studio-grid-paste` `mutate:ops`.
  - Flow half `TestGridPasteNullAndEmpty` sends those ops through `DataStream.Mutate`. Second client asserts per
    column type, per OQ-GRID-1 ADOPT part:
    - nullable int and timestamp columns hold NULL for empty field, never `''`, never 0;
    - text column holds `''` (pinned until user answers §5 Q1);
    - whitespace kept exactly.
  - Product fix: `frontend/src/studio/views/shared/grid/paste.ts:80,87` stages a string for every field. Stage NULL
    for empty field in nullable non-text column. Column type comes from describe metadata already on grid. Text
    columns unchanged.
- J-BC-001: console result filtered by find-filter, select rows, copy, paste into grid of other table. Staged values
  equal source rows; flow half shares `studio-grid-paste`.
- J-BC-002/005:
  - Copy after tab switch copies restored selection.
  - Right-click copy targets the selection shown, not a stale one.
- J-BC-003: Ctrl+C inside filter input, cell editor, console editor, find bar copies the control's selected text.
  Grid copy handler never hijacks.
- J-GRID-036 (OQ-GRID-2 ADOPT):
  - Copy of truncated cell yields full value fetched, or status line says truncated.
  - Never silent truncated bytes. Same for Copy as INSERT.
  - Fix in `slick/selectionCopy.ts` path if silent.
- J-GRID-039 (pin ADOPT): keyboard-only edit session. Delete key stages NULL on nullable cells and refuses on NOT
  NULL with visible reason. Assert staged payload.
- Docs: correct `inventory.md` TSV neutralization wording (verified claim). One `docs:` commit.
- Expected defects: NULL to `''` (confirmed); stale-selection copy after tab switch (history class); hijacked native
  copy.

## A3 Type fidelity per engine (P1/P2)

- Journeys:
  - Primary: J-GRID-015 (P1), J-GRID-031 (P1), J-GRID-033 (P2), J-CONS-023 (P2).
  - Merged: J-GRID-023.
- Files:
  - New flow `dbflow/grid_types_test.go`: Copy as INSERT replay on SQLite and Postgres.
  - New ui `tests/ui/studio/grid-copy-insert.spec.ts`.
  - EXT complete suites:
    - `internal/adapters/mysqlfamily/*_test.go`: new `Scenario` "type round trip".
    - `internal/adapters/clickhouse/*_test.go`: new `Scenario` "analytic types and paging tiebreak", "console FORMAT
      refused".
- J-GRID-015:
  - UI copies selected rows as INSERT (spy bytes, contract `studio-copy-insert`).
  - Flow half executes those exact bytes with `app.W.Router.Execute` into an empty clone table.
  - Second client: clone rows equal source rows for every type (numeric precision, timestamp with zone, bool, json,
    bytea, NULL vs `''`, unicode).
- J-GRID-031 (MySQL; MariaDB behind `Complete`):
  - Read, edit, copy across type matrix: DECIMAL(38,10), BIGINT UNSIGNED max, BIT, ENUM, SET, JSON, TIMESTAMP vs
    DATETIME, zero dates, BLOB.
  - Edit of other column never rewrites TIMESTAMP (OQ-GRID-3 ADOPT; tooltip is follow-up F-A1).
- J-GRID-033: ClickHouse Decimal256, UInt64 max, LowCardinality, Nullable, Array, Map, DateTime64. Offset paging on
  table without unique key orders by sorting key plus projected columns (OQ-GRID-4).
- J-CONS-023: ClickHouse console numeric fidelity (UInt64 above 2^53 exact as text). User `FORMAT JSON` clause
  refused with message naming clause (OQ-CONS-2 ADOPT). Fix in `adapters/clickhouse` console path if raw payload
  rendered.
- Expected defects: float64 coercion of big integers; zone shift on TIMESTAMP edit; Copy as INSERT literal escaping
  for bytea/JSON.

## A4 Concurrency, cancel, lost ack, tab close (P1/P2)

- Journeys:
  - Primary: J-GRID-038 (P1), J-GRID-040 (P1), J-GRID-026 (P2).
  - Merged: J-GRID-006, J-CONN-004, J-GRID-031 kill-mid-commit variation.
- Files:
  - New flow `dbflow/grid_faults_test.go` (Postgres via `testsupport.StartPausableProxy`).
  - New ui `tests/ui/studio/grid-stale-row.spec.ts`.
  - New ui `tests/ui/studio/grid-staged-close.spec.ts`.
- J-GRID-038 (OQ-GRID-5 safe part):
  - Cancel during read and commit via op id (`Router.Cancel`).
  - Proxy paused after statement reaches server, connection killed before ack.
  - Mutate returns error naming unknown outcome. Read after reconnect returns server truth.
  - Retry of same plan never double-inserts: second client count.
  - Cancel during commit on transactional engine leaves nothing applied.
- J-GRID-040 (OQ-GRID-6 safe part; contract `studio-grid-stale`):
  - Second client deletes, or updates key of, row staged in app. App mutate update/delete affecting zero rows
    returns failure naming row, not success.
  - Two windows: commit in window 1. Window 2 `Read` sees new value after invalidation; stale strip shown in UI half.
  - Product fix if relational mutate reports success on 0 affected rows (`adapters/sqlmutate.go`).
  - Warn-with-diff is USER §5 Q3: not built.
- J-GRID-026 (USER §5 Q8 safe part):
  - Quit with staged edits triggers quit gate (existing `quit-confirm` path) naming staged count.
  - Tab close behaviour pinned as today; spec comment links §5 Q8.
- Expected defects: success toast on zero affected rows; retry duplicating inserts after lost ack.

## A5 Documents (P1)

- Journeys:
  - Primary: J-DOC-010.
  - Merged: J-DOC-001, J-DOC-002, J-DOC-011.
- Files:
  - New flow `dbflow/document_test.go` (Mongo via `testsupport.StartMongo`).
  - New ui `tests/ui/studio/document-edit.spec.ts`.
- Scenario (contract `studio-document`):
  - Filter, project, edit rich EJSON types: ObjectId, Decimal128, Int64, Date before 1970, Binary subtype 4, nested
    arrays, field order.
  - Insert new document, delete one.
  - Copy all in JSON/EJSON under projection (OQ-DOC-3 ADOPT: projected fields only). One 4 MiB document
    (J-DOC-011).
- Oracle (second Mongo client):
  - Saved document byte-equal in BSON types: no Int64 to double, no Decimal128 to string.
  - Untouched fields preserved, field order kept.
  - `_id` unchanged and no extra document when editor changed `_id`: agreed part of OQ-DOC-2; refuse vs force is
    USER §5 Q4.
  - Deleted document gone; inserted document present once.
  - Copy all bytes (spy) equal contract.
- Concurrent replace (OQ-DOC-1) is USER §5 Q3: no test asserts overwrite.
- Expected defects: EJSON type loss on round trip through editor; projection leaking hidden fields into Copy all.

## A6 Key/value: Redis and S3 (P1)

- Journeys:
  - Primary: J-KV-010, J-KV-012, J-KV-013.
  - Merged: J-KV-001, J-KV-002, J-BRW-010.
- Files:
  - New flow `dbflow/redis_edit_test.go`, `dbflow/s3_object_test.go`, `dbflow/s3_transfer_test.go`.
  - New ui `tests/ui/studio/redis-edit.spec.ts`.
- J-KV-010 (contract `studio-redis-edit`):
  - Edit string, hash field, list item, set member, zset score with TTL set.
  - Second client: `TTL` kept (SET KEEPTTL path), value exact bytes.
  - Key expired between read and save: error, no resurrected key.
  - Large hash beyond type limit shows cap.
- J-KV-012:
  - Edit small object.
  - Second client `HeadObject`: Content-Type, Cache-Control, user metadata, tags kept. ACL kept when readable
    (OQ-KV-2 warning is USER §5 Q6).
  - Object changed by second client between open and save: conditional put refused. Without conditional put, ETag
    HeadObject check (OQ-KV-1 ADOPT) refuses.
  - Versioned bucket: new version created, old intact.
- J-KV-013 (+ J-BRW-010 keys):
  - Download via `app.Dialogs.AnswerSave(path)`: file bytes equal object.
  - Cancel mid-download leaves no target and no temp file (temp plus rename path).
  - Upload from `AnswerFile`. Keys with spaces, unicode, `a//b`, trailing slash, `%2F`.
  - Downloaded file name safe; no path traversal (`../x` key never writes outside chosen dir).
  - Unwritable target: error, no partial.
- Expected defects: metadata dropped on put; temp file left on cancel; traversal via key name.

## A7 Stream: Kafka and SQS (P1)

- Journeys:
  - Primary: J-STR-010, J-STR-011, J-STR-012.
  - Merged: J-STR-001, J-STR-002.
- Files:
  - New flow `dbflow/kafka_browse_test.go`, `dbflow/kafka_produce_test.go`, `dbflow/sqs_test.go`.
  - New ui `tests/ui/studio/stream-copy.spec.ts`.
  - EXT ui `tests/ui/studio/sqs-poll-visibility.spec.ts`.
- J-STR-010:
  - Browse 6 partitions with paging. Second client produces 1,000 records mid-paging.
  - Oracle: no duplicate partition/offset pair; none beyond window end; appended rows only after refresh.
  - `kafkaGroups` before and after equal; group `billing` offsets unchanged; no new group.
  - Tombstone copies as JSON `null` and shows marker (OQ-STR-1 ADOPT; TSV marker column is follow-up F-A2).
  - Headers order and repeats kept in JSON copy.
  - Broker restart via fixture: next page continues or returns "position lost, refresh" (pin ADOPT), never silent
    skip or duplicate.
  - Count labelled inexact.
- J-STR-011 (second client consumes):
  - Headers `trace=a, trace=b, type=x` in order.
  - Null key; empty body stored empty; absent body refused.
  - Bad headers refused before broker.
  - 5-message batch with 3rd over 1 MiB: result says 4 of 5 landed, failed position 3.
  - Resend failed one: exactly 5 records, none twice.
  - Read-only connection: refused, no request.
- J-STR-012 (queue `VisibilityTimeout` 0 s / 1 s, OQ-STR-3 ADOPT; no sleeps, wait on visibility via receive
  result):
  - `ApproximateReceiveCount` rises once per user poll, never on timer or tab focus.
  - DLQ depth unchanged after opening tab once.
  - Expired receipt delete gives "receipt expired", message still present.
  - Never-polled message delete gives "receipt gone".
  - FIFO rules: missing group id, dedup id, content dedup.
  - Duplicate attribute name: SQS lands one message with one value. Refuse vs last-wins is USER §5 Q7: assert only
    that one message lands.
- Expected defects: consumer group side effect; receive on focus; duplicate after partial batch resend.

## A8 Console (P1/P2)

- Journeys:
  - Primary: J-CONS-001 (P1), J-CONS-020 (P1), J-CONS-024 (P1), J-CONS-021, J-CONS-022, J-CONS-025, J-CONS-027,
    J-KIB-010 (P2).
  - Merged: J-CONS-002, J-CONS-005, J-CONS-006, J-CONS-009, J-KIB-001.
- Files:
  - New flow `dbflow/console_script_test.go` (SQLite, Postgres), `dbflow/console_mysql_test.go`,
    `dbflow/console_mongo_test.go`, `dbflow/console_redis_test.go`.
  - EXT `dbflow/sqlite_test.go` (J-CONS-027 restart snapshot), `dbflow/kibana_test.go`.
  - New ui `tests/ui/studio/console-script.spec.ts`, `tests/ui/studio/console-copy-all.spec.ts`.
  - EXT ui `tests/ui/studio/saved-queries-sync.spec.ts`, `tests/ui/studio/kibana-discover.spec.ts`.
- J-CONS-001 (contract `studio-console-script`):
  - Script with splitter traps: `;` in strings, dollar quotes, comments, `BEGIN ... END` blocks, empty statements.
  - UI half asserts statement list sent equals contract.
  - Flow half executes list: statement count and second-client DB effects.
- J-CONS-020: Postgres migration in transaction with error at statement 3. Second client: nothing applied after
  rollback. Capped result shows cap flag (J-CONS-005).
- J-CONS-021: MySQL `DELIMITER`, procedure create and call, multiple result sets. Session state after reconnect
  observable (pin; strip note is follow-up F-A3, OQ-CONS-1).
- J-CONS-022: SQLite `ATTACH`, pragma, write lock from second client: busy error, no partial write.
- J-CONS-024:
  - Mongo statements, EJSON results.
  - Copy all in 4 formats (spy bytes equal contract `studio-console-mongo`).
  - Duplicate column names kept distinct (J-CONS-002).
- J-CONS-025: Redis command set, binary reply marker. `SELECT 2` refused (OQ-CONS-3 ADOPT; fix if accepted).
- J-CONS-027:
  - Saved query create, edit, format; app restart; list equal.
  - Two windows save same query: file and DB valid, last save intact. Conflict policy is USER §5 Q3.
- J-KIB-010: Discover filters, histogram bucket sum equals hit count, saved search persists across restart. Results
  not restored and no auto-run after restart (pin ADOPT).
- Expected defects: splitter mis-split; Redis SELECT changing tab's db; histogram/hit mismatch.

## A9 Connections, secrets, read-only, masks (P1/P2)

- Journeys:
  - Primary: J-CONN-001, J-CONN-002, J-CONN-006, J-MASK-001 (P1), J-CONN-008 (P2).
  - Merged: J-GRID-037, J-DOC-012 (read-only part), J-BC-007, J-X-003.
- Files:
  - EXT `dbflow/connection_form_test.go`, `dbflow/connfiles_test.go`.
  - New `dbflow/connection_secrets_test.go`, `dbflow/readonly_test.go`, `dbflow/mask_test.go`.
  - EXT ui `tests/ui/studio/connection-paste-credentials.spec.ts`, `tests/ui/studio/secrets.spec.ts`,
    `tests/ui/studio/connection-files.spec.ts`.
  - New ui `tests/ui/studio/readonly-connection.spec.ts`, `tests/ui/studio/mask-copy.spec.ts`.
- J-CONN-001:
  - Paste `.env` with password; save; restart.
  - Connection YAML under `app.SpaceHome` contains no password bytes (scan file).
  - Reconnect works after `app.Restart()`.
- J-CONN-002 (contract `connection-secrets`, existing, no UI reader today):
  - Duplicate, then rotate password on copy only. Original still connects with old secret, copy with new.
  - `secrets.spec.ts` reads contract (stale-contract fix).
- J-CONN-006 (contract `studio-readonly`):
  - Read-only connection: `DataStream.Mutate` refused before engine. Second client: row count and values
    unchanged.
  - Console write statement refused. Mongo and Redis edit refused (J-DOC-012, J-GRID-037).
- J-CONN-008:
  - Hand-edit connection YAML while app runs: list reflects change; invalid YAML leaves last good state and reports
    error.
  - Contract `connection-files-migration` has neither writer nor reader. Add Go writer in `connfiles_test.go` and
    reader in `connection-files.spec.ts`. If scenario is obsolete (`git log -S connection-files-migration`), delete
    JSON instead and say so in commit.
- J-MASK-001 (contract `studio-mask`):
  - Mask rule on column. Grid copy (spy) contains masked text, never raw.
  - Disconnected (stale) grid copy also masked (J-BC-007).
  - DbMcp read (`app.W.DbMcp` tool call through dbmcpflow helper pattern) returns masked.
  - Preview toggle reveals only in UI, never in clipboard unless explicit reveal.
- Expected defects: password in YAML; mask bypass in clipboard path; read-only check only in UI.

## A10 Paging, filter by value, Studio MCP (P1/P2)

- Journeys:
  - Primary: J-BC-032 (P1), J-MCP-001 (P1), J-MCP-002 (P1), J-GRID-034 (P2), J-MCP-004 (P2), J-BC-044 (P2).
  - Merged: J-GRID-001, J-MCP-005, J-BC-020, J-BC-021.
- Files:
  - New flow `dbflow/grid_paging_test.go`.
  - New ui `tests/ui/studio/grid-filter-by-value.spec.ts`.
  - EXT dbmcpflow `approvals_test.go`, `tables_test.go`, `review_test.go`, `mcp_test.go`.
- J-GRID-034 (SQLite plus Postgres):
  - Keyset paging forward/back while second client inserts and deletes: no duplicate or skipped PK.
  - Offset mode duplicates/skips documented by result flag.
  - Page size change mid-table restarts at first page (pin ADOPT).
  - Rapid page clicks (J-BC-020) settle on last requested page: cancel of superseded ops.
  - DbMcp `browse_table` paging stable under same inserts (J-MCP-005).
  - Page-token fingerprint mismatch after schema change gives clear error (`relationalpage.go`).
- J-BC-032 (contract `studio-filter-by-value`):
  - Filter by value from cell menu with `O'Brien`, backslash, `%`, `_`, NULL, unicode.
  - UI asserts generated filter sent. Flow asserts rows returned equal second-client exact-match query.
  - Product fix if quoting wrong.
- J-MCP-001: agent installs, runs write through MCP, approval granted. Second client sees row; op log entry names
  statement; rejected approval leaves DB unchanged.
- J-MCP-002: allow-list bypass attempts via JOIN, subquery, view, schema-qualified name, CTE, quoted identifier. Each
  refused; DB untouched.
- J-MCP-004: result review rejects delivery; agent receives refusal, never rows.
- J-BC-044: MCP server across `app.Restart()`: same port reused when free (`dbMcpPortRe` in `harness.go`); client
  reconnect works; no orphan listener.
- Expected defects: allow-list bypass through view or CTE; filter literal escaping.

## A11 Definition DDL fidelity (P2)

- Journeys:
  - Primary: J-DEF-010.
  - Merged: J-DEF-001.
- Files:
  - New flow `dbflow/definition_ddl_test.go` (SQLite, Postgres).
  - EXT ui `tests/ui/studio/definition.spec.ts` (copy DDL bytes = contract `studio-definition`).
- Oracle:
  - `Router.Definition` DDL executed in fresh DB recreates table.
  - Describe of clone equals source: columns, defaults, PK, FK, unique, check, indexes, comments (OQ-DEF-1 ADOPT).
  - Engine gaps found go to `docs/ARCHITECTURE.md` "Known open items" as one entry per engine. Do not hide gaps.

## A12 API collections, environments, import (P1/P2)

- Journeys:
  - Primary: J-COLL-011, J-COLL-013, J-IMP-011 (P1); J-COLL-003, J-COLL-012, J-ENV-010, J-ENV-011, J-IMP-010 (P2).
  - Merged: J-COLL-001, J-COLL-002, J-ENV-001, J-IMP-001.
- Files:
  - EXT apiflow `collections_test.go`, `configfiles_test.go`, `variables_test.go`.
  - New apiflow `twowindows_test.go`.
  - New ui `tests/ui/studio/api-copy-url.spec.ts`.
- J-COLL-011:
  - Multi-select 3 requests (J-COLL-001 scale: 200 seeded); move to other and to new collection.
  - Collection files on disk: each item in exactly one file; YAML parses; no duplicate ids.
  - Journalled move is follow-up F-A4 (OQ-API-4).
- J-COLL-013:
  - Export collection: `app.Dialogs.AnswerSave(path)`, file bytes parse back to same items.
  - Cancel writes nothing.
  - Reveal recorded in `app.FileOpener.RevealedFiles`.
- J-IMP-011 (OQ-API-7 ADOPT):
  - Import same Postman file twice: second import creates suffixed collection.
  - Local edits to first collection untouched (file bytes unchanged).
  - Messy export (J-IMP-001): import, send, export, re-import stable.
- J-IMP-010: Postman environment with unnamed and empty entries. Import report lists skipped; stored variables equal
  expected.
- J-COLL-003: hand-edited collection YAML with bad entry. Good items load, bad reported, file never rewritten by app.
- J-COLL-012 (OQ-API-5 ADOPT): Copy URL from tree row gives template text `{{base}}/x`; secret never resolved into
  clipboard (spy).
- J-ENV-010: variable value history and revert. File on disk after revert equals pre-edit bytes.
- J-ENV-011 (USER §5 Q3 safe part): globals, collection and environment edited in two windows. Both converge
  through events; file valid YAML; no partial write; last save intact.
- Expected defects: duplicate item after move; re-import overwriting local edits; secret in Copy URL.

## A13 gRPC (P2)

- Journeys: J-GRPC-011, J-GRPC-012, J-GRPC-014.
- Files: EXT grpcflow `call_test.go`.
- J-GRPC-011: deadline vs cancel vs connect wait against `flowharness.Silent` and `GRPC` recorder. Deadline error
  code exact; cancel leaves no in-flight call (recorder count).
- J-GRPC-012: metadata edge cases (binary `-bin` keys, repeated keys, unicode values). Recorder sees exact metadata.
- J-GRPC-014: TLS target matrix (plaintext, TLS with CA, wrong host, self-signed refused). `flowharness.HTTPS`-style
  CA from servers package.

## File ownership (Stream A)

| Path | Owner |
|---|---|
| `apps/kira-space/internal/flowharness/datastream.go` (new) | A |
| `apps/kira-space/internal/flows/dbflow/**` | A |
| `apps/kira-space/internal/flows/dbmcpflow/approvals_test.go`, `tables_test.go`, `review_test.go`, `mcp_test.go` | A |
| `apps/kira-space/internal/flows/apiflow/**` | A |
| `apps/kira-space/internal/flows/grpcflow/**` | A |
| `apps/kira-space/internal/adapters/mysqlfamily/*_test.go`, `clickhouse/*_test.go` (complete-suite scenarios) | A |
| `apps/kira-space/tests/ui/studio/**` except `http-curl.spec.ts`, `http-variables.spec.ts` | A |
| `apps/kira-space/tests/ui/studio/support/clipboard.ts` | A (B, C import only) |
| `apps/kira-space/tests/contract/studio-*.json`, `api-*.json` except `api-curl.json`, `connection-*.json` | A |
| Product: `internal/adapters/**`, `internal/adapterhost/**`, `internal/connections`, `internal/connfiles`, `internal/mask*`, `internal/dbmcp` | A |
| Product: `internal/apifiles`, `internal/apivars`, `internal/postman`, `internal/grpcclient` | A |
| Product: `frontend/src/studio/**` except `frontend/src/studio/api/state/curl.ts` and HTTP send path | A |
| `docs/v0.3.13/plans/inventory.md` (TSV wording) | A |

## Shared files (additive hunks only)

| File | A edit | B edit | C edit |
|---|---|---|---|
| `apps/kira-space/playwright.config.ts` | add `studio/grid-clipboard-roundtrip.spec.ts` to `ui-clipboard` `testMatch`, configured exactly as `grid-copy-menu.spec.ts` is | none | none |
| `packages/theme/src/i18n/*.json` | new keys under `studio.*` / `backend.studio.*` only | `ade.*` / `backend.ade.*`, `docker.*` | `api.http.*`, `git.*`, `backend.space.*`, `kubernetes.*` |
| `docs/ARCHITECTURE.md` "Known open items" | append entries (DDL gaps per engine) | append own | append own |
| `apps/kira-space/internal/flows/coverage/exempt.txt` | none expected; append line only if guard demands | same | same |
| `apps/kira-space/tests/ui/support/ipcChannels.ts` | append own block only if a fix adds a channel | same | same |

## No ordering dependency

- A's tests and fixes touch none of B's or C's owned paths.
- The httpclient redirect fix (C5) is not needed by any A test: J-MCP-011 moved to C.
- A12 storage packages and C5 send packages are disjoint.
- Shared-file edits are append-only in separate key namespaces and rebase cleanly in any order.

## Follow-ups (design decisions, not built here)

- F-A1: TIMESTAMP column header tooltip naming session zone (OQ-GRID-3 display part).
- F-A2: TSV tombstone marker column for Kafka copy (OQ-STR-1 TSV part).
- F-A3: console strip note "new session" after automatic reconnect (OQ-CONS-1).
- F-A4: journalled move of requests between collection files (OQ-API-4 crash safety).
- F-A5: Copy all label "projected" under projection (OQ-DOC-3 label part).
- F-A7: Redis edit is string-only. Hash, list, set, zset edits unsupported (J-KV-010).
- F-A8: SQLite no-PK rowid tables refuse keyed writes by design (D23).
- F-A9: mutate errors do not name the failing op index.
- F-A11: 39 of 759 unit tests in `apps/kira-space/tests/unit/studio` fail in isolation with an import cycle (`useTabsStore` before initialization). Identical with this stream's changes stashed.
- F-A12: MySQL console has no `DELIMITER` directive.
- F-A13: a `CALL`'s extra result sets are dropped (first set only).
- F-A14: Mongo console parses numbers as doubles; no cursor chain (`.sort().limit()`).
- F-A15: a hand-edited collection file with one bad item serves no rows on its first load (validation is whole-file, last-good rows only after a good load). J-COLL-003 "good items load" oracle unmet. Listed in ARCHITECTURE.md Known open items.
- F-A16: Kafka and SQS composer: body required, no FIFO group/dedup fields, single message only.
- F-A17: SQLite read-only console refusal shows raw driver text.
- F-A18: table allow list (J-MCP-002) gates statement text, not triggers.
- F-A19: Browse offset paging can duplicate or skip rows when rows are inserted or deleted ahead of the cursor.
- F-A20: guard coverage limits (see A9 notes).
- No global variable scope exists in the app; J-ENV-011 covers collection and environment variables.
- Add each bug found needing design decision here with journey id; orchestrator turns list into SPEC rows.

## Done when

- Every work item's tests pass in a normal run, plus `Complete` gate for complete-suite scenarios.
- Each `NEW both` UI half and flow half read same contract. Stale contracts above have reader and writer.
- `grep -rn "time.Sleep\|waitForTimeout" <new files>` is empty.
- Pre-commit hook green on every commit; worktree rebases cleanly on chapter branch.

## Result

Done. A0 to A13 landed, each with flow half and UI half where the plan names both. Own specs pass; no full UI suite run.

Decisions built and tested (flow and UI):
- Decision 2 (lost ack keeps staged edits, "may or may not be saved"): `TestGridLostAck`, contract `studio-grid-lost-ack`, `grid-lost-ack.spec.ts`.
- Decision 3 (concurrent-edit guard, `E_CONFLICT` with `changed:true`, UI confirm then resend unguarded) in all four places: grid row update and delete, Mongo whole-document replace, saved query (console, Discover), API request (`Expect` on `collections:saveRequest`, `useSeenSaved`; `TestSavedRequestTwoWindows`, `api-request-conflict.spec.ts`). `confirmOverwrite` and `guardedSavedWrite` moved to `studio/state/`.
- Decision 4 (refuse `_id` edit), 6 (S3 unreadable ACL warning), 7 (SQS repeated attribute name refused), 8 (tab close with open transaction asks, rolls back).

Product bugs fixed:
1. SQLite composite or nullable-key tables not editable, key lost in DDL.
2. Empty paste staged `''`; now NULL. Unchanged pasted cells skipped.
3. Lost COMMIT ack message; now `commitOutcomeUnknown`.
4. Redis update resurrected expired key and dropped TTL.
5. S3 edit overwrote a replaced object.
6. Truncated-copy notice, Delete clears to NULL, Ctrl+A in cell editor, binary INSERT literals.
7. ClickHouse console accepted a user FORMAT clause.
8. Grid update and delete overwrote server-changed rows.
9. Console splitter on trigger and compound bodies; SQLite trigger refusal.
10. Saved-query concurrent edit; Redis console binary reply flag; Discover Run on a restored tab; Kibana test op id.
11. Page token fingerprint ignored key columns.
12. SQLite Definition DDL omitted explicit indexes.
13. Kafka browse accepted a trimmed position; SQS repeated attribute names.
14. API request two-window overwrite.
15. Postman re-import reused the display name; now suffixed (OQ-API-7).
16. gRPC unary deadline did not cover the dial (300 ms call took 5 s).
17. gRPC non-ASCII metadata value gave a raw transport error; now `BadRequest` naming the key.
18. Flaky busy-timeout in a test helper.

New contracts: `studio-grid-lost-ack`, `api-copy-url`, `api-request-two-windows`, plus earlier `studio-*` set.
Tests run: new and changed Go flow packages (dbflow, apiflow, grpcflow, dbmcpflow, kafka, sqs, adapters touched), grpcclient, and the new UI specs on project `ui`.
Follow-ups: see list above.
