# P627 journeys: open questions on correct behaviour

Every point where journeys could not define "correct" from inventory, architecture doc or code alone. Each entry:
question, where it is used, recommended definition. Until answered, tests record current behaviour and fail only
on undeclared change. Items marked `pin` inside journeys (search `pin` in `journeys-*.md`) are smaller cases of
same kind: behaviour read as either-or; test pins what build does today. Section "Pins worth a decision" lists
those with user-visible cost.

## Claims verified against code

- TSV formula neutralization (J-GRID-010): REFUTED as defect. CSV copy prefixes `'` to cells starting with
  `= + - @ TAB CR`; TSV copy leaves them raw on purpose.
  `apps/kira-space/frontend/src/studio/views/shared/clipboardFormats.ts:22` (`neutralizeFormulaPrefix`), sole
  caller `csvField` at `:107`; `tsvField` at `:42-45`; rationale comment `:32-38`. Inventory wording that TSV is
  neutralized is imprecise. Recommended: correct inventory; pin raw TSV bytes in test.
- NULL versus empty string on paste (J-GRID-035): CONFIRMED. Copy writes NULL as empty field
  (`clipboardFormats.ts:48,98,113`); paste stages string for every field (`grid/paste.ts:80,87`); JSON copy
  keeps `null` (`clipboardFormats.ts:118`). Copy then paste turns NULL into `''`. See OQ-GRID-1.

## Studio grid

- OQ-GRID-1 (J-GRID-030, 035): empty pasted field: NULL or `''`? Recommended: nullable non-text column gets
  NULL; text column gets `''`; preview marks each; Set NULL action stays explicit path. Never store 0 for empty
  field in numeric column.
- OQ-GRID-2 (J-GRID-010, 036): copy of truncated cell. Recommended: copy fetches full value; if not possible,
  status line says value was truncated. Same rule for CSV export and Copy as INSERT.
- OQ-GRID-3 (J-GRID-031): zone for MySQL `TIMESTAMP` display and edit. Recommended: connection session zone,
  shown in column header tooltip; edit of other column never rewrites value.
- OQ-GRID-4 (J-GRID-033): paging tiebreak on ClickHouse tables without unique key. Recommended: order by full
  sorting key plus all projected columns; state that duplicates across pages are possible under offset paging.
- OQ-GRID-5 (J-GRID-038): commit acknowledgement lost. Recommended: report "outcome unknown", reload page, keep
  staged set only for rows whose server value still differs from staged value.
- OQ-GRID-6 (J-GRID-040): row changed on server since staging. Recommended: preview shows current server value
  as "before" and warns when it differs from value seen at staging; zero affected rows is failure.

## Studio console

- OQ-CONS-1 (J-CONS-021): session state (`USE`, `SET @x`, temp tables, open transaction) after automatic
  reconnect. Recommended: result strip notes "new session" on first run after reconnect.
- OQ-CONS-2 (J-CONS-023): user-written `FORMAT` clause in ClickHouse console. Recommended: refuse with message
  naming clause, or strip it and say so; never render raw payload as grid.
- OQ-CONS-3 (J-CONS-025): `SELECT <db>` in Redis console. Recommended: refuse; tab is bound to one database.
- OQ-CONS-4 (J-CONS-027): same saved query edited in two windows. Recommended: second save warns that saved
  text changed since load, offers overwrite or save as copy.

## Studio documents, key/value, stream, definition

- OQ-DOC-1 (J-DOC-010): whole-document replace overwrites concurrent change. Recommended: compare document read
  at edit start with server document at save; differ means warn with diff, save needs confirm.
- OQ-DOC-2 (J-DOC-010): `_id` edited in editor is silently forced back. Recommended: refuse save with message
  "_id cannot change; insert new document and delete old one".
- OQ-DOC-3 (J-DOC-010): Copy all under projection. Recommended: copies what is displayed (projected fields),
  toolbar label says "projected".
- OQ-KV-1 (J-KV-012): S3-compatible store without conditional put. Recommended: fall back to ETag check by
  `HeadObject` just before put and say check is best effort.
- OQ-KV-2 (J-KV-012): ACL not readable before edit. Recommended: tell user before save that ACL may reset to
  owner-only, not only log line.
- OQ-STR-1 (J-STR-010): tombstone on clipboard. Recommended: JSON copy `null`; TSV copy empty field plus
  distinct marker column; grid shows explicit marker.
- OQ-STR-2 (J-STR-012): repeated SQS attribute name keeps last value. Recommended: refuse compose with duplicate
  name (SQS attributes are unique); Kafka keeps repeats.
- OQ-STR-3 (J-STR-012 header): SQS receipt expiry reads wall clock, no seam. Recommended: P628 tests use queue
  visibility 0 s or 1 s; clock seam only if that proves flaky.
- OQ-DEF-1 (J-DEF-010): how faithful is copied DDL. Recommended: copied DDL recreates columns, defaults, keys,
  constraints, indexes, comments; known gaps listed per engine in architecture doc "Known open items".

## API module

- OQ-API-1 (J-API second pass, cookie jar off): hint why no cookie was sent. Recommended: sent-cookies pane shows
  "cookie jar off" hint.
- OQ-API-2: unresolved `{{variable}}` at send. Recommended: block send, list unresolved names.
- OQ-API-3: curl import of `-G` with data: query encoding. Recommended: byte-equal to real curl.
- OQ-API-4: move of requests between collections writes two files. Recommended: journalled; crash leaves item in
  exactly one file.
- OQ-API-5: Copy URL of templated request. Recommended: template text as saved, never resolved secret.
- OQ-API-6: same request edited in two windows. Recommended: base-hash conflict on second save.
- OQ-API-7: re-import of same collection file. Recommended: new collection with suffix, local edits untouched.

## Git module

- OQ-GIT-1: stash pop restoring index state. Recommended: `--index`; status byte-equal to state before stash.
- OQ-GIT-2: undo of reset when local change collides. Recommended: refuse with file list, change nothing.
- OQ-GIT-3: undo of tag move when tag changed again meanwhile. Recommended: refuse, never move newer tag.

## ADE

- OQ-ADE-1 (J-LINK-011): URL normalisation for artifact dedup. Recommended: lower-case host, drop fragment and
  trailing slash, keep query.
- OQ-ADE-2 (J-LEASE-003): leases after restart are gone while resumed agent believes it holds. Recommended:
  resume prompt line tells agent to re-acquire.
- OQ-ADE-3 (J-ADE-060): wall-clock jump (sleep) against lease TTL and run timeouts. Recommended: TTL counts
  heartbeat gaps only while process ran (monotonic clock); busy turn never expires.
- OQ-ADE-4 (J-WF-010): switching task workflow deletes its runs and run logs. Recommended: confirm dialog naming
  run count; no-op for same id stays.
- OQ-ADE-5 (J-WF-011): same workflow saved from two windows or changed on disk under dirty editor.
  Recommended: detect newer file version at save, offer reload or overwrite.
- OQ-ADE-6 (J-RUN-032): tool calls after `finish_step`. Recommended: refuse with "step already finished";
  outcome unchanged.

## Infrastructure, Docker, Kubernetes

- OQ-INF-1 (J-K8S-010..012): Secret values through MCP. Recommended: same `<N bytes>` masking as UI, no bypass.
- OQ-DKR-1 (J-DKR-027): remove, pull, prune, compose up/down, volume and network writes are outside bound
  surface. Recommended: keep absent; journeys assert absence until product decides.
- OQ-K8S-1 (J-K8S-026): exec, attach, port-forward, edit, delete are outside surface (GET only). Recommended:
  keep absent; any write path arrives with approval surface.
- OQ-K8S-2 (J-K8S-021): wording when list is truncated at cap. Recommended: banner "showing first N; sort and
  filter cover loaded rows only".

## Pins worth a decision

- Results after app restart: console results and Discover hits not restored, no auto-run (J-CONS-020,
  J-KIB-010). Recommended: keep.
- Tab close with open transaction or staged edits (J-CONS-020, J-GRID-026): Recommended: prompt for staged
  edits; roll back open transaction on close.
- Docker: second Restart while first runs (J-DKR-020): Recommended: refuse with "in progress".
- Docker logs after engine returns (J-DKR-024): Recommended: stream ends once, user reopens.
- Kafka page token after broker restart (J-STR-010): Recommended: continue when offsets still valid, else
  "position lost, refresh".
- Stream compose draft on tab close (J-STR-013): Recommended: prompt when body not empty.
- Stage move while headless run is live (J-RUN-031): Recommended: refuse until run stopped.
- Page size change mid-table (J-GRID-034): Recommended: restart at first page.
- Delete key on grid selection (J-GRID-039): Recommended: stage NULL for nullable, refuse for NOT NULL.
