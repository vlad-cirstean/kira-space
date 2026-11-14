# P628 synthesis: journey decisions, oracle seams, streams

Inputs: `inventory.md`, `coverage-map.md`, `journeys-*.md` (426 journeys), `journeys-open-questions.md`.
Outputs: this file, `stream-a.md` (Studio, API collections/environments/import, gRPC), `stream-b.md` (ADE,
agents, automations, notify/prompts/mobile, Docker), `stream-c.md` (HTTP send/curl, git, config sync, secrets,
Kubernetes, restart/crash). Each stream file is the full implementer plan.

## 1. Decision codes

- `EXT <file>`: extend named existing test.
- `NEW flow <pkg/file>`: new Go flow test (`apps/kira-space/internal/flows/<pkg>/`).
- `NEW ui <spec>`: new Playwright spec (`apps/kira-space/tests/ui/`).
- `NEW both`: same scenario split at IPC boundary. Flow half drives bound service or data plane, asserts far-end
  result, writes contract (`app.Contract`, `tests/contract/<scenario>.json`). UI half drives screen, asserts
  outgoing IPC payload equals contract (`contract(scenario, key)`) and asserts screen shows contract result.
- `MERGE→J-x`: steps become variations of J-x; J-x test carries them.
- `DROP <reason>`: `dup` (same oracle elsewhere), `cut` (below cut line; named existing test covers core),
  `nondet` (cannot run deterministic in seconds), `absence` (asserts feature not present), `absent-feature`
  (journey assumes feature product lacks).
- Tag after decision: stream work item (`A1`) and priority (`P1`, `P2`).

## 2. Priority and cut line

- P1: user-visible data loss or corruption; clipboard, export, persistence correctness; secret leakage; history bug
  classes (clipboard, paging, reconnect, revert, restart). Far-end oracle mandatory.
- P2: correctness with far-end oracle, lower blast radius, or historical bug class with partial coverage.
- Cut line: committed set = every P1 and P2 item below. Everything tagged `DROP cut` sits below line: not built this
  chapter; named existing test stays the coverage. Implementers run work items in stream-file order (P1 first); no
  item is optional.

Committed counts (primary journeys with own test / merged journeys carried by them):

| Stream | P1 | P2 | Primary total | Merged in | Covered |
|---|---|---|---|---|---|
| A Studio + API collections/gRPC | 33 | 24 | 57 | 40 | 97 |
| B ADE + agents + automations + notify/mobile + Docker | 20 | 35 | 55 | 36 | 91 |
| C HTTP, git, config sync, secrets, Kubernetes, restart | 18 | 31 | 49 | 39 | 88 |
| All | 71 | 90 | 161 | 115 | 276 |

Dropped: 150 (cut 143, dup 2, nondet 2, absence 2, absent-feature 1). Total 426.

## 3. Oracle seams

- Clipboard text:
  - UI only (no backend text clipboard; coverage-map §6 gap 1).
  - Byte assertion via `installClipboardSpy(page, {lazy, image})` in `tests/ui/studio/support/clipboard.ts`
    (`window.__clipboard`).
  - Round trip (copy, then paste reads clipboard) runs in Chromium `ui-clipboard` project with real clipboard
    (`grantPermissions(['clipboard-read'])`). Stream A adds its round-trip spec to that project's `testMatch`.
  - B and C import the spy read-only; never edit `clipboard.ts`.
- Image clipboard: `app.ImageClipboard.Writes()` (flow), `__clipboardImages` (UI).
- File on disk (export, download, save, config):
  - Flow half answers picker with `app.Dialogs.AnswerSave/AnswerFile/AnswerDirectory`; empty queue = cancel.
  - Read bytes with `os.ReadFile`; assert no temp/partial file left in target dir.
  - UI half asserts `filesChooseSave` / save-call args via `calls(control, channel)`.
- DB row:
  - Second client outside app: `sql.Open("sqlite", path)` on fixture file; `testsupport.Start<Kind>` fixtures
    expose DSN/client for Postgres, MySQL, MariaDB, ClickHouse, Mongo, Redis, S3, Kafka, SQS.
  - App DB: `app.DB()`.
- Studio data plane (grid read/count/preview/mutate):
  - New `flowharness/datastream.go` (Stream A) attaches `bridge.ServeEngineStream(app.W.Router, pipe)`.
  - Sends JSON request frames, decodes FlatBuffers `wire.Frame`.
  - Closes coverage-map §6 gap 5: no Studio write at IPC boundary today; `Router` has `Execute`, but `data:mutate`
    only exists on frame path (`adapterhost/data.go` `Dispatcher.Mutate`).
- Remote git: `app.NewBare` / `Bare.Git("rev-parse", ...)`, `Clone`, `PushFrom`; local refs via `NewRepo` git
  calls.
- Restart: `app.Restart()` (same home; old service refs dead). UI: `relaunch({storage, ...})`.
- Crash (SIGKILL): new `flowharness/crash.go` (Stream C), see stream-c C13.
- Wire: `flowharness.HTTP/HTTPS/GRPC` recorders (request bytes, headers); `NewKubeFake().Requests()`;
  `fakehost.Forge.Calls()`; `notifysink` records; `Browser.Opened()`, `FileOpener.OpenedFiles/RevealedFiles`.
- Time: `flowharness.WithClock` (fake clock), `WithBranchRequestWait`, `WithRestoreTiming`, `WithListLimits`; UI
  `relaunch({clockTime})` and `tests/ui/support/clock.ts`. No `time.Sleep` / `waitForTimeout` in new tests.
  Wait on events (`app.Events.WaitAfter`, Playwright auto-wait).
- Network faults: `testsupport.StartPausableProxy` (pause mid-op, drop ack); `flowharness.Silent` (hang).

Rules for every new test:
- Seconds, not minutes.
- Fixtures under reader's `testdata/`, never `docs/v*/`.
- Real-container tests gate with `flowharness.RequireDocker(t)`. Containers are shared per package via
  `main_test.go` when package already does so.
- Adapter-level functional coverage goes into complete suite's `Scenario`/`Requires` seam, never a parallel
  mechanism.
- Never skip, disable or quarantine.
- No real `claude`: fake agent only.
- G5: no credential reads in tests touching `githost/`, `issuetracker/`, `internal/docker`, `internal/harness/claude`.

## 4. Open questions resolved

Legend:
- `ADOPT`: recommended definition is test expectation. Stream fixes product to match, root-caused, same stream.
- `ADOPT-F`: definition adopted. Product change is named follow-up in stream file; test covers safe part now.
- `USER`: needs user (§5). Plan does not depend on answer; test covers agreed part only.

Claims verified:
- TSV formula neutralization: ADOPT refutation. Pin raw TSV bytes, neutralized CSV (A2). A corrects
  `inventory.md` wording.
- NULL to `''` on paste: confirmed defect class. See OQ-GRID-1.

Studio:
- OQ-GRID-1: USER for text columns. ADOPT for non-text nullable columns: empty pasted field stages NULL, never `''`
  or 0. `''` is invalid for those types, so no product choice is involved. Text column keeps `''` (pinned) until
  user answers. A2.
- OQ-GRID-2: ADOPT. Copy of truncated cell fetches full value; if not possible, status says truncated. Same for Copy
  as INSERT. A2.
- OQ-GRID-3: ADOPT. Session zone; edit of other column never rewrites TIMESTAMP value. Tooltip part ADOPT-F. A3.
- OQ-GRID-4: ADOPT. Full sorting key plus projected columns; duplicates under offset paging documented. A3.
- OQ-GRID-5: USER for staged-set retention. ADOPT safe part: lost ack reported as error naming unknown outcome;
  reload shows server truth; retry never double-inserts. A4.
- OQ-GRID-6: USER for warn-with-diff. ADOPT safe part: zero affected rows on update/delete is failure, not success.
  A4.
- OQ-CONS-1: ADOPT-F (strip note "new session" after reconnect). A8 asserts session state loss is observable,
  pinned.
- OQ-CONS-2: ADOPT. Refuse user `FORMAT` clause with message naming it. A3.
- OQ-CONS-3: ADOPT. Refuse `SELECT <db>` in Redis console. A8.
- OQ-CONS-4: USER (concurrent saved-query edit policy). A8 asserts no corruption, last save intact.
- OQ-DOC-1: USER (warn on concurrent change). A5 tests non-concurrent save only.
- OQ-DOC-2: USER (refuse vs force-back). A5 asserts agreed part: server `_id` never changes, no extra document.
- OQ-DOC-3: ADOPT. Copy all under projection copies displayed fields. Label part ADOPT-F. A5.
- OQ-KV-1: ADOPT. HeadObject ETag check before put on stores without conditional put. A6.
- OQ-KV-2: USER (pre-save ACL reset warning). A6 asserts ACL preserved when readable.
- OQ-STR-1: ADOPT. Tombstone: JSON copy `null`, grid explicit marker. TSV marker column ADOPT-F. A7.
- OQ-STR-2: USER (refuse duplicate SQS attribute vs last wins). A7 asserts Kafka keeps repeats; SQS lands one
  message with one value.
- OQ-STR-3: ADOPT. Queue visibility 0 s / 1 s fixture; no clock seam unless flaky. A7.
- OQ-DEF-1: ADOPT. Copied DDL recreates columns, defaults, keys, constraints, indexes, comments; gaps per engine go
  to ARCHITECTURE "Known open items". A11.

API:
- OQ-API-1: ADOPT-F (sent-cookies pane "cookie jar off" hint). C5 asserts no Cookie header sent with jar off.
- OQ-API-2: ADOPT. Block send, list unresolved names; recorder receives zero requests. C5.
- OQ-API-3: ADOPT. `-G` query encoding byte-equal to curl's, expected bytes recorded in testdata. C5.
- OQ-API-4: ADOPT-F (journalled move). A12 asserts item in exactly one file after normal move.
- OQ-API-5: ADOPT. Copy URL is template text, never resolved secret. A12.
- OQ-API-6: USER (base-hash conflict). A12 asserts both windows converge, file valid, no partial write.
- OQ-API-7: ADOPT. Re-import creates suffixed collection; local edits untouched. A12.

Git:
- OQ-GIT-1: ADOPT. Stash pop with `--index`; status byte-equal to pre-stash. C1.
- OQ-GIT-2: ADOPT. Undo of reset refused with file list on collision; nothing changes. C1.
- OQ-GIT-3: ADOPT. Undo of tag move refused when tag moved again. C1.

ADE:
- OQ-ADE-1: ADOPT. Dedup URL: lower-case host, drop fragment and trailing slash, keep query. B8.
- OQ-ADE-2: ADOPT. Resume prompt tells agent to re-acquire leases. B7.
- OQ-ADE-3: USER (TTL vs wall-clock jump). B7 tests heartbeat expiry with fake clock only.
- OQ-ADE-4: USER (confirm before deleting runs on workflow switch). B2 asserts same-id no-op keeps runs; other
  tasks' runs untouched.
- OQ-ADE-5: USER (stale-file detection on workflow save). B2 asserts file valid YAML, bad input never written.
- OQ-ADE-6: ADOPT. Tool calls after `finish_step` refused "step already finished"; outcome unchanged. B3.

Infra:
- OQ-INF-1: ADOPT. Kube Secret values through MCP masked `<N bytes>`. C11.
- OQ-DKR-1: USER (scope). No test; J-DKR-027 dropped (absence). Coverage guard catches any new bound write.
- OQ-K8S-1: USER (scope). Same as OQ-DKR-1; J-K8S-026 dropped.
- OQ-K8S-2: ADOPT. Truncation banner "showing first N; sort and filter cover loaded rows only". C11.

Pins worth decision:
- Results not restored after restart: ADOPT keep (C13 asserts no auto-run).
- Tab close with open transaction or staged edits: USER. A4 asserts quit gate counts staged edits (existing) and
  pins tab-close behaviour.
- Docker double Restart: ADOPT refuse "in progress". B16.
- Docker logs after engine returns: ADOPT stream ends once. B16.
- Kafka token after broker restart: ADOPT continue or "position lost, refresh". A7 asserts no silent duplicate or
  skip.
- Stream compose draft on close: ADOPT-F (prompt). Journey J-STR-013 dropped cut.
- Stage move while headless run live: USER. B6 asserts run state and stage stay consistent (no orphan run).
- Page size change mid-table: ADOPT restart at first page. A10.
- Delete key on grid selection: ADOPT stage NULL for nullable, refuse NOT NULL. A2.

## 5. Questions for user (plan does not depend on them)

1. OQ-GRID-1 text columns: empty pasted field in nullable text column: NULL or `''`? Today `''`; copy/paste round
   trip turns NULL into `''` (data change).
2. OQ-GRID-5: after lost commit ack, keep staged set for rows still differing, or clear?
3. Optimistic concurrency policy (one decision, five surfaces):
   - OQ-GRID-6: grid row changed since staging.
   - OQ-DOC-1: Mongo whole-document replace overwrites concurrent change.
   - OQ-CONS-4: saved query edited in two windows.
   - OQ-API-6: API request edited in two windows.
   - OQ-ADE-5: workflow saved from two windows.

   Warn plus confirm, or last write wins?
4. OQ-DOC-2: edited `_id` refused with message, or forced back silently (today)?
5. OQ-ADE-4: workflow switch deletes task runs and logs: confirm dialog naming count?
6. OQ-KV-2: warn before S3 save that ACL may reset when unreadable?
7. OQ-STR-2: SQS duplicate attribute name: refuse compose, or last wins (today)?
8. Tab close with staged grid edits or open console transaction: prompt and roll back?
9. Stage move while headless run live: refuse until stopped?
10. OQ-ADE-3: lease TTL and run timeouts across sleep: monotonic heartbeat gaps only?
11. OQ-DKR-1 / OQ-K8S-1: Docker remove/pull/prune/compose and kube exec/edit/delete stay out of product?

## 6. Per-journey decisions

### Studio grid (`journeys-studio-grid.md`)

- J-GRID-001 MERGE→J-GRID-034 (A10).
- J-GRID-002 DROP cut: ui `studio/tab-filter-state.spec.ts`, dbflow `sqlite_test.go` history snapshot.
- J-GRID-003 DROP cut: FK navigation read-only; no far-end effect beyond J-GRID-020 edit.
- J-GRID-004 DROP cut: ui `studio/interaction.spec.ts` find bar.
- J-GRID-005 MERGE→J-BC-070 (C13).
- J-GRID-006 MERGE→J-GRID-040 (A4).
- J-GRID-010 NEW ui · A2 · P1 · `studio/grid-clipboard-roundtrip.spec.ts` (TSV raw, CSV neutralized, JSON null,
  binary marker).
- J-GRID-011 MERGE→J-GRID-010 (A2).
- J-GRID-012 MERGE→J-GRID-010 (A2).
- J-GRID-013 NEW both · A2 · P1 · ui `studio/grid-clipboard-roundtrip.spec.ts`; flow `dbflow/grid_paste_test.go`.
- J-GRID-014 MERGE→J-GRID-035 (A2).
- J-GRID-015 NEW both · A3 · P1 · ui `studio/grid-copy-insert.spec.ts`; flow `dbflow/grid_types_test.go`.
- J-GRID-016 DROP absent-feature: grid has no export-to-file; CSV only via clipboard (J-GRID-010).
- J-GRID-017 DROP cut: ui `studio/interaction.spec.ts`.
- J-GRID-020 NEW both · A1 · P1 · flow `dbflow/grid_mutate_test.go`; ui `studio/grid-commit-roundtrip.spec.ts`.
- J-GRID-021 EXT ui `studio/mutations.spec.ts` · A1 · P1 (staged set is renderer-only until commit; final payload =
  A1 contract).
- J-GRID-022 MERGE→J-GRID-020 (A1).
- J-GRID-023 MERGE→J-GRID-031 (A3).
- J-GRID-024 DROP cut: ui `studio/fake-data.spec.ts`.
- J-GRID-025 MERGE→J-GRID-020 (A1).
- J-GRID-026 NEW ui · A4 · P2 · `studio/grid-staged-close.spec.ts` (safe part; USER 8).
- J-CELL-001 DROP cut: ui `studio/cell-editor.spec.ts`.
- J-CELL-002 DROP cut: ui `studio/cell-editor.spec.ts`.
- J-CELL-003 MERGE→J-BC-070 (C13).
- J-GRID-030 NEW flow · A1 · P1 · `dbflow/grid_mutate_pg_test.go`.
- J-GRID-031 EXT `internal/adapters/mysqlfamily/*_test.go` complete suite (Scenario) · A3 · P1; kill mid-commit
  variation carried by J-GRID-038 (A4).
- J-GRID-032 NEW flow · A1 · P1 · `dbflow/grid_mutate_test.go` (SQLite rowid, WITHOUT ROWID, generated, locked).
- J-GRID-033 EXT `internal/adapters/clickhouse/*_test.go` complete suite · A3 · P2.
- J-GRID-034 NEW flow · A10 · P2 · `dbflow/grid_paging_test.go`.
- J-GRID-035 NEW both · A2 · P1 · flow `dbflow/grid_paste_test.go`; ui `studio/grid-clipboard-roundtrip.spec.ts`.
- J-GRID-036 NEW ui · A2 · P2 · `studio/grid-clipboard-roundtrip.spec.ts` (truncated cell copy).
- J-GRID-037 MERGE→J-CONN-006 (A9).
- J-GRID-038 NEW flow · A4 · P1 · `dbflow/grid_faults_test.go` (PausableProxy).
- J-GRID-039 NEW ui · A2 · P2 · `studio/grid-keyboard-edit.spec.ts`.
- J-GRID-040 NEW both · A4 · P1 · flow `dbflow/grid_faults_test.go`; ui `studio/grid-stale-row.spec.ts`.

### Studio console (`journeys-studio-console.md`)

- J-CONS-001 NEW both · A8 · P1 · flow `dbflow/console_script_test.go`; ui `studio/console-script.spec.ts`.
- J-CONS-002 MERGE→J-CONS-024 (A8).
- J-CONS-003 DROP cut: ui `studio/console.spec.ts`, `adapterhost/dataframe_cancel_test.go`.
- J-CONS-004 DROP cut: ui `studio/console-format.spec.ts`.
- J-CONS-005 MERGE→J-CONS-020 (A8).
- J-CONS-006 MERGE→J-CONS-027 (A8).
- J-CONS-007 DROP cut: ui `studio/autocomplete.spec.ts`.
- J-CONS-008 DROP cut: ui `studio/console-explain.spec.ts`, dbmcpflow `explain_threshold_test.go`.
- J-CONS-009 MERGE→J-CONS-025 (A8).
- J-KIB-001 MERGE→J-KIB-010 (A8).
- J-CONS-020 NEW flow · A8 · P1 · `dbflow/console_script_test.go` (Postgres).
- J-CONS-021 NEW flow · A8 · P2 · `dbflow/console_mysql_test.go`.
- J-CONS-022 NEW flow · A8 · P2 · `dbflow/console_script_test.go` (SQLite).
- J-CONS-023 EXT `internal/adapters/clickhouse/*_test.go` complete suite · A3 · P2.
- J-CONS-024 NEW both · A8 · P1 · flow `dbflow/console_mongo_test.go`; ui `studio/console-copy-all.spec.ts`.
- J-CONS-025 NEW flow · A8 · P2 · `dbflow/console_redis_test.go`.
- J-CONS-026 DROP cut: ui `studio/console-explain.spec.ts`.
- J-CONS-027 EXT `dbflow/sqlite_test.go` + ui `studio/saved-queries-sync.spec.ts` · A8 · P2.
- J-KIB-010 EXT `dbflow/kibana_test.go` + ui `studio/kibana-discover.spec.ts` · A8 · P2.
- J-KIB-011 DROP cut: `dbflow/kibana_test.go`.

### Studio connections (`journeys-studio-connections.md`)

- J-CONN-001 EXT `dbflow/connection_form_test.go` + ui `studio/connection-paste-credentials.spec.ts` · A9 · P1.
- J-CONN-002 NEW both · A9 · P1 · flow `dbflow/connection_secrets_test.go`; ui `studio/secrets.spec.ts` EXT
  (reader of contract `connection-secrets`).
- J-CONN-003 DROP cut: `dbflow/preconnect_test.go`, ui `studio/preconnect.spec.ts`.
- J-CONN-004 MERGE→J-GRID-038 (A4).
- J-CONN-005 DROP cut: `dbflow/connection_lifecycle_test.go`.
- J-CONN-006 NEW both · A9 · P1 · flow `dbflow/readonly_test.go`; ui `studio/readonly-connection.spec.ts`.
- J-CONN-007 DROP cut: `dbflow/order_test.go`, ui `studio/connection-order.spec.ts`.
- J-CONN-008 EXT `dbflow/connfiles_test.go` + ui `studio/connection-files.spec.ts` · A9 · P2.
- J-CONN-009 DROP cut: ui `studio/datagrip-import.spec.ts`.
- J-CONN-010 DROP cut: dbmcpflow `engine_throttle_test.go`, `engine_timeout_test.go`.
- J-MASK-001 NEW both · A9 · P1 · flow `dbflow/mask_test.go`; ui `studio/mask-copy.spec.ts`.
- J-MASK-002 DROP cut: dbmcpflow `maskreview_test.go`, ui `studio/connection-mask-review.spec.ts`.

### Studio documents (`journeys-studio-documents.md`)

- J-DOC-001 MERGE→J-DOC-010 (A5).
- J-DOC-002 MERGE→J-DOC-010 (A5).
- J-DOC-010 NEW both · A5 · P1 · flow `dbflow/document_test.go`; ui `studio/document-edit.spec.ts`.
- J-DOC-011 MERGE→J-DOC-010 (A5).
- J-DOC-012 MERGE→J-CONN-006 (A9) read-only part; replica step-down part is not deterministic.
- J-DOC-013 DROP cut: ui `studio/document-view-readonly.spec.ts`, `studio/tab-filter-state.spec.ts`.

### Studio key/value (`journeys-studio-keyvalue.md`)

- J-KV-001 MERGE→J-KV-010 (A6).
- J-KV-002 MERGE→J-KV-012 (A6).
- J-KV-010 NEW both · A6 · P1 · flow `dbflow/redis_edit_test.go`; ui `studio/redis-edit.spec.ts`.
- J-KV-011 DROP cut: cluster/replica topology absent from fixture; ACL-denied path in redis adapter suite.
- J-KV-012 NEW flow · A6 · P1 · `dbflow/s3_object_test.go`.
- J-KV-013 NEW flow · A6 · P1 · `dbflow/s3_transfer_test.go`.

### Studio stream (`journeys-studio-stream.md`)

- J-STR-001 MERGE→J-STR-010 (A7).
- J-STR-002 MERGE→J-STR-012 (A7).
- J-STR-010 NEW both · A7 · P1 · flow `dbflow/kafka_browse_test.go`; ui `studio/stream-copy.spec.ts`.
- J-STR-011 NEW flow · A7 · P1 · `dbflow/kafka_produce_test.go`.
- J-STR-012 NEW flow + EXT ui `studio/sqs-poll-visibility.spec.ts` · A7 · P1 · flow `dbflow/sqs_test.go`.
- J-STR-013 DROP cut: ui stream filter-history specs; draft pin ADOPT-F.

### Studio browse and definition (`journeys-studio-browse-definition.md`)

- J-TREE-001 DROP cut: ui `studio/tree.spec.ts`.
- J-TREE-002 DROP cut: ui `studio/tree.spec.ts`.
- J-TREE-003 DROP cut: read-only error path; no far-end effect.
- J-TREE-004 DROP cut: ui `studio/tree.spec.ts`.
- J-DEF-001 MERGE→J-DEF-010 (A11).
- J-DEF-010 NEW both · A11 · P2 · flow `dbflow/definition_ddl_test.go`; ui `studio/definition.spec.ts` EXT.
- J-BRW-001 DROP cut: ui `studio/browse-folder-nav.spec.ts`.
- J-BRW-010 MERGE→J-KV-013 (A6).
- J-BRW-011 DROP cut: redis adapter suite scan.
- J-TREE-010 DROP cut: ui `studio/tree.spec.ts`, `studio/sql-schema.spec.ts`.

### Studio MCP and approvals (`journeys-studio-mcp-approvals.md`)

- J-MCP-001 EXT dbmcpflow `approvals_test.go` · A10 · P1.
- J-MCP-002 EXT dbmcpflow `tables_test.go` · A10 · P1.
- J-MCP-003 DROP cut: `expensive_test.go`, `exemption_test.go`, `throttle_test.go`, `timeout_test.go`.
- J-MCP-004 EXT dbmcpflow `review_test.go` · A10 · P2.
- J-MCP-005 MERGE→J-GRID-034 (A10).
- J-MCP-006 DROP cut: dbmcpflow `unreachable_test.go`.
- J-MCP-007 DROP cut: dbmcpflow `ops_test.go`.
- J-MCP-010 DROP cut: dbmcpflow `scripts_test.go`.
- J-MCP-011 EXT dbmcpflow `api_test.go` · C5 · P1 (shares httpclient redirect fix with J-HTTP-041).
- J-MCP-012 MERGE→J-K8S-025 (C11).
- J-APR-001 DROP cut: ui `approvals-one-at-a-time.spec.ts`, dbmcpflow `approvals_test.go`.
- J-APR-002 MERGE→J-ADE-053 (B5).
- J-APR-003 DROP cut: dbmcpflow `focus_test.go`, ui `studio/dbmcp-focus.spec.ts`.
- J-PRF-001 MERGE→J-PRF-020 (B8).
- J-PRF-002 DROP cut: ui `studio/proof-window.spec.ts`.

### ADE first pass (`journeys-ade.md`)

- J-ADE-001 DROP cut: adeflow `newtask_test.go`, `boardfacts_test.go`.
- J-ADE-002 DROP cut: ui `list-keyboard.spec.ts`.
- J-ADE-003 MERGE→J-ADE-041 (B1).
- J-ADE-004 DROP cut: adeflow `statuses_test.go`.
- J-ADE-005 EXT adeflow `repoupdating_test.go` + ui `ade-v2-repo-updating.spec.ts` · B8 · P2 (reader of contract
  `ade-repo-updating`).
- J-ADE-010 MERGE→J-ADE-071 (B13).
- J-ADE-011 MERGE→J-ADE-053 (B5).
- J-ADE-012 MERGE→J-ADE-055 (B4).
- J-ADE-013 DROP cut: adeflow `norepo_test.go`, `reporequired_test.go`.
- J-ADE-020 MERGE→J-RUN-026 (B6).
- J-ADE-021 MERGE→J-LINK-010 (B8).
- J-ADE-022 DROP cut: adeflow `trackedupdates_test.go`.
- J-ADE-023 MERGE→J-PRF-020 (B8).
- J-WF-001 MERGE→J-WF-011 (B2).
- J-WF-002 MERGE→J-WF-011 (B2).
- J-RUN-001 MERGE→J-RUN-021 (B3).
- J-RUN-002 MERGE→J-RUN-022 (B3).
- J-RUN-003 DROP cut: adeflow `sessions_test.go`.
- J-RUN-004 DROP cut: terminal renderer, ui `ade-tui-takeover.spec.ts`.
- J-RUN-005 MERGE→J-LEASE-001 (B7).
- J-RUN-006 MERGE→J-RUN-027 (B10).
- J-RUN-007 MERGE→J-X-005 (B14).
- J-RUN-008 DROP cut: ui `ade-v2-background-ops.spec.ts`.
- J-HAR-001 EXT harnessflow `provider_test.go` · B12 · P2.
- J-HAR-002 NEW both · B12 · P2 · flow `harnessflow/config_io_test.go`; ui `ade-agent-settings.spec.ts` EXT.
- J-HAR-003 DROP cut: adeflow `sessionstatus_test.go`.
- J-HAR-004 EXT adeflow `sandbox_test.go` · B12 · P2.
- J-ADE-030 DROP cut: adeflow `capturereview_test.go`.
- J-ADE-031 MERGE→J-REV-030 (B9).

### ADE second pass (`journeys-ade-2.md`)

- J-ADE-040 MERGE→J-ADE-071 (B13).
- J-ADE-041 NEW both · B1 · P1 · flow `adeflow/archive_safety_test.go`; ui `ade-v2-archive.spec.ts` EXT.
- J-ADE-042 NEW flow · B1 · P1 · `adeflow/archive_safety_test.go`.
- J-ADE-043 NEW flow · B1 · P1 · `adeflow/archive_safety_test.go`.
- J-ADE-044 NEW flow · B11 · P2 · `adeflow/twowindows_test.go`.
- J-ADE-050 EXT adeflow `rebase_test.go` · B4 · P1.
- J-ADE-051 MERGE→J-ADE-050 (B4).
- J-ADE-052 EXT adeflow `rebase_test.go` · B4 · P1.
- J-ADE-053 EXT adeflow `branchrequest_test.go` + ui `ade-v2-branch-request.spec.ts` · B5 · P1.
- J-ADE-054 EXT adeflow `branchrequest_test.go` · B5 · P2.
- J-ADE-055 EXT adeflow `branchadopt_test.go` · B4 · P2.
- J-ADE-056 MERGE→J-RUN-032 (B3).
- J-ADE-057 NEW both · B4 · P1 · flow `adeflow/forcepush_test.go`; ui `ade-v2-force-push.spec.ts` EXT.
- J-RUN-020 MERGE→J-RUN-030 (B3).
- J-RUN-021 EXT adeflow `outcome_test.go` · B3 · P1.
- J-RUN-022 EXT adeflow `resume_test.go` · B3 · P1.
- J-RUN-023 DROP cut: adeflow `norepo_test.go`.
- J-RUN-024 MERGE→J-RUN-031 (B6).
- J-RUN-025 EXT adeflow `handover_test.go` · B6 · P1.
- J-RUN-026 NEW both · B6 · P2 · flow `adeflow/notes_test.go` EXT; ui `ade-v2-notes.spec.ts` EXT.
- J-RUN-027 EXT adeflow `orchestrator_test.go` · B10 · P2.
- J-REV-020 MERGE→J-REV-030 (B9).
- J-REV-021 DROP cut: adeflow `review_uncommitted_test.go`.
- J-PRF-010 MERGE→J-PRF-020 (B8).
- J-LINK-010 EXT adeflow `detailslinks_test.go` + ui `ade-v2-links-strip.spec.ts` · B8 · P2 (contract `link-open`
  writer and reader).
- J-LINK-011 EXT adeflow `artifacts_test.go` · B8 · P2.
- J-LEASE-001 EXT adeflow `leases_test.go` · B7 · P2.
- J-LEASE-002 EXT adeflow `leases_test.go` · B7 · P2.
- J-LEASE-003 EXT adeflow `leases_test.go` · B7 · P2.
- J-HAR-010 DROP cut: adeflow `sessions_test.go`.
- J-ADE-060 DROP cut: composite of B1-B9 items; 6-hour day not deterministic in seconds.

### ADE third pass (`journeys-ade-3.md`)

- J-WF-010 NEW flow · B2 · P1 · `adeflow/workflow_switch_test.go`.
- J-RUN-030 NEW flow · B3 · P1 · `adeflow/routing_test.go`.
- J-WF-011 EXT adeflow `workflowfiles_test.go`, `workflow_editor_test.go` · B2 · P1.
- J-RUN-031 EXT adeflow `stagemove_test.go` · B6 · P1.
- J-RUN-032 NEW flow · B3 · P1 · `adeflow/agent_tools_test.go`.
- J-PRF-020 EXT adeflow `proofchain_test.go` + proofflow `shots_test.go` · B8 · P1.
- J-REV-030 EXT adeflow `reviewstep_test.go` · B9 · P2.
- J-LINK-020 EXT adeflow `issuetracker_test.go`, `hostlink_test.go` · B8 · P2.
- J-ADE-070 NEW flow · B11 · P2 · `adeflow/board_scale_test.go`.
- J-ADE-071 NEW flow · B13 · P2 · `journeyflow/ade_task_test.go`.
- J-ADE-072 MERGE→J-RUN-027 (B10); mobile approval part MERGE→J-MOB-014 (B15).

### HTTP and gRPC first pass (`journeys-http-grpc.md`)

- J-HTTP-001 DROP cut: apiflow `lifecycle_test.go`, `collections_test.go`.
- J-HTTP-002 EXT httpflow `wire_test.go` · C5 · P2.
- J-HTTP-003 MERGE→J-HTTP-048 (C5).
- J-HTTP-004 DROP cut: ui `studio/http-pipes.spec.ts`, `studio/http-dynamic-values.spec.ts`.
- J-HTTP-005 MERGE→J-HTTP-041 (C5).
- J-HTTP-006 MERGE→J-HTTP-040 (C5).
- J-HTTP-007 MERGE→J-HTTP-045 (C5).
- J-HTTP-008 DROP cut: httpflow `send_test.go`.
- J-HTTP-010 MERGE→J-HTTP-045 (C5).
- J-HTTP-011 DROP cut: ui `studio/http-raw.spec.ts`, `studio/http-timeline.spec.ts`.
- J-HTTP-020 DROP cut: apiflow `history_test.go`.
- J-HTTP-030 MERGE→J-HTTP-065 (C5).
- J-GRPC-001 DROP cut: grpcflow `call_test.go`.
- J-GRPC-002 DROP cut: ui `studio/grpc-request.spec.ts`.
- J-COLL-001 MERGE→J-COLL-011 (A12).
- J-COLL-002 MERGE→J-ENV-011 (A12).
- J-COLL-003 EXT apiflow `configfiles_test.go` · A12 · P2.
- J-ENV-001 MERGE→J-ENV-010 (A12).
- J-ENV-002 DROP cut: ui `studio/api-cross-window-sync.spec.ts`.
- J-IMP-001 MERGE→J-IMP-011 (A12).

### HTTP and gRPC second pass (`journeys-http-grpc-2.md`)

- J-HTTP-040 EXT httpflow `send_test.go` · C5 · P2.
- J-HTTP-041 NEW flow · C5 · P1 · `httpflow/redirect_test.go`.
- J-HTTP-042 MERGE→J-HTTP-041 (C5).
- J-HTTP-043 MERGE→J-HTTP-041 (C5).
- J-HTTP-044 NEW flow · C5 · P2 · `httpflow/redirect_test.go`.
- J-HTTP-045 EXT httpflow `send_test.go` · C5 · P2.
- J-HTTP-046 DROP cut: header flood bounded by 5 MiB cap covered in J-HTTP-045.
- J-HTTP-047 DROP cut: protocol selection in httpclient unit tests; recorder cannot assert HTTP/2 framing cheaply.
- J-HTTP-048 NEW both · C5 · P1 · flow `httpflow/unresolved_test.go`; ui `studio/http-variables.spec.ts` EXT.
- J-HTTP-049 EXT httpflow `wire_test.go` · C5 · P2.
- J-HTTP-050 DROP cut: jar and incognito covered by J-HTTP-040.
- J-HTTP-051 DROP cut: ui `studio/api-lifecycle.spec.ts`.
- J-HTTP-060 EXT `packages/api-core/test/http-curl.spec.ts` + `curl-cases.json` · C5 · P2 (parser with interacting
  rules: unit cases are the right tier; renderer parses, no IPC).
- J-HTTP-061 MERGE→J-HTTP-060 (C5).
- J-HTTP-062 MERGE→J-HTTP-060 (C5).
- J-HTTP-063 MERGE→J-HTTP-060 (C5).
- J-HTTP-064 MERGE→J-HTTP-060 (C5).
- J-HTTP-065 NEW both · C5 · P1 · flow `httpflow/curl_test.go` EXT; ui `studio/http-curl.spec.ts` EXT.
- J-HTTP-066 MERGE→J-HTTP-065 (C5).
- J-GRPC-010 DROP cut: grpcflow `describe_test.go`.
- J-GRPC-011 EXT grpcflow `call_test.go` · A13 · P2.
- J-GRPC-012 EXT grpcflow `call_test.go` · A13 · P2.
- J-GRPC-013 DROP cut: grpcflow `call_test.go` streaming.
- J-GRPC-014 EXT grpcflow `call_test.go` · A13 · P2.
- J-COLL-010 DROP cut: apiflow `collections_test.go`.
- J-COLL-011 EXT apiflow `collections_test.go` · A12 · P1.
- J-COLL-012 NEW ui · A12 · P2 · `studio/api-copy-url.spec.ts`.
- J-COLL-013 EXT apiflow `collections_test.go` · A12 · P1.
- J-ENV-010 EXT apiflow `variables_test.go` · A12 · P2.
- J-ENV-011 NEW flow · A12 · P2 · `apiflow/twowindows_test.go`.
- J-IMP-010 EXT apiflow `collections_test.go` · A12 · P2.
- J-IMP-011 EXT apiflow `collections_test.go` · A12 · P1.

### Repos and git first pass (`journeys-repos-git.md`)

- J-REPO-001 DROP cut: repoflow `import_test.go`, `hidden_test.go`.
- J-REPO-002 EXT repoflow `configfile_test.go` · C15 · P2.
- J-REPO-003 DROP cut: gitflow `credential_routes_test.go`.
- J-GRAPH-001 DROP cut: perf; gitflow `graph_test.go`, `perf_test.go`.
- J-GRAPH-002 DROP cut: ui `repo-graph-toolbar.spec.ts`.
- J-GRAPH-003 DROP cut: ui `repo-refs-popover.spec.ts`.
- J-GRAPH-004 EXT githostflow `sync_test.go` + ui `git-graph-plane.spec.ts` · C15 · P2 (reader of contract
  `git-graph-plane`).
- J-GIT-001 DROP cut: composite of C1/C2 items.
- J-GIT-002 MERGE→J-GIT-028 (C4).
- J-GIT-003 MERGE→J-GIT-026 (C1).
- J-GIT-004 MERGE→J-WT-010 (C3).
- J-GIT-005 MERGE→J-STACK-001 (C3).
- J-GIT-006 DROP cut: gitflow `remote_test.go`, `matrix_test.go`.
- J-GIT-007 DROP cut: gitflow `misc_test.go`.
- J-WS-001 EXT repoflow `edit_test.go` · C15 · P2.
- J-WS-002 DROP cut: ui diff specs.
- J-WS-003 DROP cut: gitflow `blame_test.go`.
- J-WS-004 DROP cut: windowflow `openedfiles_test.go`.
- J-HOST-001 EXT githostflow `comments_test.go` · C15 · P2.
- J-HOST-002 DROP cut: githostflow `issueview_test.go`.
- J-HOST-003 DROP cut: appflow `clitools_test.go`.
- J-REV-001 EXT reviewflow `incremental_test.go` · C15 · P2 (ui `git-bound-review.spec.ts` reads contract
  `git-review-edit`).
- J-REV-002 MERGE→J-HOST-001 (C15).
- J-REV-003 DROP cut: reviewflow `review_test.go`.
- J-REV-004 DROP cut: ui `repo-review-interaction.spec.ts`.

### Repos and git second pass (`journeys-repos-git-2.md`)

- J-GIT-020 EXT gitflow `bound_writes_test.go` · C1 · P1.
- J-GIT-021 NEW flow · C4 · P2 · `gitflow/preflight_test.go`.
- J-GIT-022 MERGE→J-GIT-021 (C4).
- J-GIT-023 EXT gitflow `ops_local_test.go` · C4 · P2.
- J-GIT-024 EXT gitflow `ops_local_test.go` · C1 · P1.
- J-GIT-025 MERGE→J-GIT-024 (C1).
- J-GIT-026 EXT gitflow `bound_writes_test.go` · C1 · P1.
- J-GIT-027 NEW flow · C1 · P1 · `gitflow/undo_test.go`.
- J-GIT-028 EXT gitflow `ops_local_test.go` · C4 · P2.
- J-GIT-029 NEW flow · C1 · P1 · `gitflow/undo_test.go`.
- J-GIT-030 NEW flow · C1 · P1 · `gitflow/undo_test.go`.
- J-GIT-031 EXT gitflow `concurrency_test.go` · C1 · P2.
- J-WT-010 EXT gitflow `worktree_state_test.go` · C3 · P2.
- J-WT-011 EXT gitflow `worktree_state_test.go` · C3 · P1.
- J-WT-012 MERGE→J-WT-011 (C3).
- J-WT-013 MERGE→J-WT-010 (C3).
- J-WT-014 DROP cut: gitflow `concurrency_test.go`.
- J-STACK-001 NEW flow · C3 · P2 · `gitflow/stack_test.go`.
- J-STACK-002 MERGE→J-STACK-001 (C3).
- J-STACK-003 MERGE→J-STACK-001 (C3).
- J-REM-010 EXT gitflow `remote_test.go` · C2 · P2.
- J-REM-011 EXT gitflow `remote_test.go` · C2 · P1.
- J-REM-012 EXT gitflow `remote_test.go` · C2 · P2.
- J-REM-013 DROP cut: gitflow `remote_test.go` fetch.
- J-REM-014 EXT gitflow `remote_test.go` · C2 · P2.
- J-GIT-040 DROP cut: composite.
- J-GIT-041 DROP cut: perf; reviewflow `incremental_test.go`.
- J-GIT-042 DROP cut: read-only navigation.

### Infra and misc first pass (`journeys-infra-misc.md`)

- J-SHELL-001 MERGE→J-BC-070 (C13).
- J-SHELL-002 DROP cut: ui `modules.spec.ts`.
- J-SHELL-003 DROP cut: ui `studio/panel-size-per-module.spec.ts`.
- J-SHELL-004 DROP cut: ui `running-ops-status.spec.ts`.
- J-SHELL-005 DROP cut: appflow `misc_test.go`.
- J-SET-001 DROP cut: ui `studio/settings-apply-on-save.spec.ts`.
- J-SET-002 DROP cut: appflow `experimental_test.go`.
- J-SET-003 DROP cut: appflow `filehandlers_test.go`.
- J-SET-004 DROP cut: llmpromptsflow.
- J-AUTO-001 EXT termflow `runs_form_test.go` · B14 · P2.
- J-AUTO-002 EXT termflow `schedule_test.go` · B14 · P2.
- J-AUTO-003 DROP cut: termflow `smart_test.go`.
- J-AUTO-004 EXT termflow `runs_file_test.go` · B14 · P2 (reader of contract `script-folders`).
- J-TERM-001 DROP cut: termflow `terminal_test.go`.
- J-TERM-002 DROP cut: windowflow `terminallinks_test.go`.
- J-OPS-001 DROP cut: appflow `ops_test.go`.
- J-NOTE-001 MERGE→J-NOTE-010 (B15).
- J-PROMPT-001 MERGE→J-PROMPT-010 (B15).
- J-CAP-001 DROP cut: captureflow.
- J-MOB-001 MERGE→J-MOB-010 (B15).
- J-MEM-001 DROP cut: memoryflow `store_test.go`, `ingest_test.go`, `distill_test.go`.
- J-CFG-001 MERGE→J-CFG-010 (C8).
- J-CFG-002 EXT configsyncflow `configsync_test.go` · C8 · P2.
- J-SEC-001 NEW flow · C9 · P1 · `journeyflow/secret_hygiene_test.go`.
- J-SEC-002 DROP cut: needs versioned old-schema DB fixtures not in repo; follow-up F-C3.
- J-UPD-001 DROP cut: appflow `update_test.go`.
- J-LIFE-001 DROP cut: appflow `keepawake_test.go`, `quit_test.go`.

### Infra and misc second pass (`journeys-infra-misc-2.md`)

- J-DKR-010 EXT dockerflow `edit_test.go` · B16 · P2.
- J-DKR-011 EXT dockerflow `edit_test.go` · B16 · P1.
- J-DKR-012 EXT dockerflow `edit_test.go` · B16 · P2.
- J-DKR-013 EXT dockerflow `edit_test.go` · B16 · P1.
- J-DKR-014 MERGE→J-DKR-013 (B16).
- J-DKR-015 EXT dockerflow `streams_test.go` · B16 · P2.
- J-DKR-016 DROP cut: dockerflow `origin_test.go`.
- J-K8S-010 MERGE→J-K8S-024 (C11).
- J-K8S-011 MERGE→J-K8S-025 (C11).
- J-K8S-012 EXT kubeflow `clusters_test.go` · C11 · P2.
- J-NOTE-010 EXT notifyflow `notify_test.go` · B15 · P2.
- J-NOTE-011 EXT notifyflow `notify_test.go` · B15 · P2.
- J-NOTE-012 EXT notifyflow `notify_test.go` · B15 · P2.
- J-PROMPT-010 EXT promptflow `prompts_test.go` · B15 · P2.
- J-USE-010 DROP cut: usageflow `usage_test.go`.
- J-MOB-010 EXT mobileflow `access_test.go` · B15 · P2.
- J-MOB-011 EXT mobileflow `access_test.go` · B15 · P1.
- J-MOB-012 EXT mobileflow `concurrency_test.go` · B15 · P2.
- J-MOB-013 DROP cut: mobileflow `terminal_test.go`.
- J-MOB-014 EXT mobileflow `writes_test.go` · B15 · P1.
- J-CFG-010 EXT configsyncflow `configsync_test.go` · C8 · P1.
- J-CFG-011 EXT configsyncflow `configsync_test.go` · C8 · P2.
- J-CFG-012 EXT configsyncflow `configsync_test.go` · C8 · P1.
- J-CFG-013 EXT configsyncflow `configsync_test.go` · C8 · P1.
- J-MEM-010 DROP cut: memoryflow `store_test.go`, `import_test.go`.
- J-MEM-011 DROP cut: memoryflow `semantic_test.go`.

### Docker (`journeys-docker.md`)

- J-DKR-020 EXT dockerflow `containers_test.go` · B16 · P2.
- J-DKR-021 MERGE→J-DKR-010 (B16).
- J-DKR-022 MERGE→J-DKR-015 (B16).
- J-DKR-023 DROP cut: termflow exec terminal, ui docker specs.
- J-DKR-024 EXT dockerflow `restore_test.go` · B16 · P2.
- J-DKR-025 DROP cut: dockerflow `origin_test.go`.
- J-DKR-026 DROP cut: read-only lists, dockerflow `disk_test.go`.
- J-DKR-027 DROP absence: `TestEveryBoundMethodAndRequestHasAFlow` already fails on any new bound method.
- J-DKR-001 MERGE→J-DKR-020 (B16).
- J-DKR-002 MERGE→J-DKR-024 (B16).
- J-DKR-003 DROP cut: dockerflow `status_test.go`.

### Kubernetes (`journeys-kubernetes.md`)

- J-K8S-020 MERGE→J-K8S-024 (C11).
- J-K8S-021 EXT kubeflow `list_test.go` · C11 · P2.
- J-K8S-022 EXT kubeflow `clusters_test.go` · C11 · P2.
- J-K8S-023 DROP cut: kubeflow `clusters_test.go`.
- J-K8S-024 EXT kubeflow `logs_test.go` · C11 · P2.
- J-K8S-025 EXT dbmcpflow `kube_test.go` · C11 · P1.
- J-K8S-026 DROP absence: as J-DKR-027.
- J-K8S-001 MERGE→J-K8S-020 (C11, via J-K8S-024).
- J-K8S-002 DROP cut: ui `studio/kubernetes-module.spec.ts`.

### Bug classes (`journeys-bug-classes.md`)

- J-BC-001 NEW ui · A2 · P1 · `studio/grid-clipboard-roundtrip.spec.ts` (console result to grid paste).
- J-BC-002 NEW ui · A2 · P2 · `studio/grid-clipboard-roundtrip.spec.ts`.
- J-BC-003 NEW ui · A2 · P1 · `studio/native-copy.spec.ts`.
- J-BC-004 DROP cut: denial path in ui `studio/grid-copy-menu.spec.ts`.
- J-BC-005 MERGE→J-BC-002 (A2).
- J-BC-006 MERGE→J-GRID-010 (A2).
- J-BC-007 MERGE→J-MASK-001 (A9).
- J-BC-008 DROP cut: ui `studio/collections.spec.ts`, `studio/tree.spec.ts`.
- J-BC-009 NEW both · C15 · P2 · flow `gitflow/copy_values_test.go`; ui `copy-coverage-git.spec.ts` EXT.
- J-BC-010 DROP cut: ui `ade-v2-sessions.spec.ts`.
- J-BC-011 DROP cut: ui `operations.spec.ts`.
- J-BC-012 DROP cut: ui `studio/proof-copy-all.spec.ts`.
- J-BC-020 MERGE→J-GRID-034 (A10).
- J-BC-021 MERGE→J-GRID-034 (A10).
- J-BC-022 DROP cut: redis/s3 adapter suite cursors.
- J-BC-023 EXT gitflow `graph_test.go` · C15 · P2.
- J-BC-024 MERGE→J-K8S-021 (C11).
- J-BC-025 EXT githostflow `comments_test.go` · C15 · P2.
- J-BC-026 EXT termflow `runs_lifecycle_test.go` · B14 · P2.
- J-BC-027 EXT adeflow `archived_test.go` · B1 · P2.
- J-BC-030 DROP cut: sort/filter/page covered by J-GRID-034; no export-to-file.
- J-BC-031 DROP cut: engine collation, adapter suites.
- J-BC-032 NEW both · A10 · P1 · flow `dbflow/grid_paging_test.go`; ui `studio/grid-filter-by-value.spec.ts`.
- J-BC-033 DROP cut: ui `studio/tab-filter-state.spec.ts`.
- J-BC-034 DROP cut: ui `studio/tree.spec.ts`.
- J-BC-035 DROP cut: perf; ui `branchMenu` specs.
- J-BC-036 DROP cut: ui `studio/autocomplete.spec.ts`.
- J-BC-040 DROP nondet: real sleep/wake.
- J-BC-041 DROP cut: `dbflow/connection_lifecycle_test.go`.
- J-BC-042 DROP cut: `adapterhost/dataframe_cancel_test.go`.
- J-BC-043 DROP cut: `dbflow/preconnect_test.go`.
- J-BC-044 EXT dbmcpflow `mcp_test.go` · A10 · P2.
- J-BC-045 EXT gitflow `bound_test.go` · C4 · P2.
- J-BC-050 MERGE→J-GRID-036 (A2).
- J-BC-051 MERGE→J-GRID-020 (A1).
- J-BC-052 DROP cut: naming collisions read-only; J-CONS-024 covers duplicate columns.
- J-BC-053 DROP cut: ui empty-state specs.
- J-BC-054 MERGE→J-GRID-010 (A2).
- J-BC-055 DROP cut: perf.
- J-BC-060 DROP cut: ui `studio/console-format.spec.ts`.
- J-BC-061 MERGE→J-GRID-021 (A1).
- J-BC-062 MERGE→J-GIT-027 (C1).
- J-BC-063 DROP cut: ui `repo-config-form.spec.ts`.
- J-BC-070 EXT journeyflow `restart_test.go` · C13 · P1.
- J-BC-071 NEW flow · C13 · P2 · `journeyflow/crash_test.go`.
- J-BC-080 DROP cut: ui `studio/document-new-save-double-submit.spec.ts`, `studio/grid-commit-in-flight.spec.ts`.
- J-BC-081 DROP cut: same as J-BC-080.

### Cross-module (`journeys-cross-module.md`)

- J-X-001 DROP cut: composite.
- J-X-002 DROP cut: composite; clipboard legs covered by A2, C5.
- J-X-003 MERGE→J-MASK-001 (A9).
- J-X-004 MERGE→J-SEC-001 (C9).
- J-X-005 NEW flow · B14 · P2 · `termflow/script_reuse_test.go`.
- J-X-006 DROP cut: ui `approvals-one-at-a-time.spec.ts`.
- J-X-007 MERGE→J-BC-070 (C13).
- J-X-008 DROP cut: repoflow, adeflow repo list tests.
- J-X-009 MERGE→J-GRAPH-004 (C15).
- J-X-010 MERGE→J-CFG-011 (C8).
- J-X-011 MERGE→J-MOB-012 (B15).
- J-X-012 MERGE→J-PRF-020 (B8).
- J-X-013 DROP cut: shared renderer, ui terminal specs.
- J-X-014 DROP cut: composite.
- J-X-015 MERGE→J-HAR-001 (B12).
- J-X-016 DROP cut: windowflow `openedfiles_test.go`.
- J-X-017 DROP dup: every copy action carried by A2, A5, A7, A8, A9, B6, C5, A12, B16, C15 items and
  `copy-coverage*.spec.ts`.
- J-X-018 DROP nondet: OS network state.
- J-X-019 DROP cut: perf.
- J-X-020 DROP dup: J-GRID-040, J-ADE-044, J-ENV-011, J-DKR-012.

## 7. Stale contracts (coverage-map §6 gap 4)

- No UI reader:
  - `connection-secrets`: A9.
  - `connection-files-migration`: A9; Go writer missing too. A adds writer in `dbflow/connfiles_test.go`, or
    deletes stale JSON if scenario is obsolete.
  - `ade-repo-updating`: B8.
  - `link-open`: B8; Go writer missing too.
  - `automations-mcp-app`: B14.
  - `script-folders`: B14.
  - `git-graph-plane`: C15.
  - `git-review-edit`: C15.
- Each stream adds reader (and writer where missing) for its contracts.

## 8. Streams

- A: Studio (grid, console, documents, key/value, stream, definition, connections, masks, Studio MCP). Also API
  collections, environments, import and gRPC: same `frontend/src/studio` tree and `tests/ui/studio` specs. Moved
  from C to balance load.
- B: ADE (tasks, board, runs, review step, branches, stages, handover, proof, links, workflows, leases, harness).
  Also Runs/automations, notifications, prompts, mobile and Docker: moved from C to balance load.
- C: HTTP send, redirects, curl (plus `dbmcpflow/api_test.go` for J-MCP-011, which shares the httpclient fix). Also
  git (ops, undo, worktrees, stacks, remote, host, review), config sync, secrets, Kubernetes, restart/crash
  checklist.
- No ordering dependency between streams:
  - Each owns disjoint test files and disjoint product subsystems.
  - Shared files take additive hunks only (stream files list them).
  - A12 (collections) and C5 (send) both touch the API module, but in disjoint packages: A12/A13 own
    `internal/apifiles`, `internal/apivars`, `internal/postman`, `internal/grpcclient`, apiflow and grpcflow; C5
    owns `internal/httpclient`, `packages/api-core/src/http/curl/` and httpflow. Product fixes stay
    inside owned package.
