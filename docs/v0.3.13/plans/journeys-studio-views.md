# P627 journeys: Studio non-tabular views, Studio MCP, approvals, proof, Kubernetes, Docker

Imagined usage and definition of correct. No implementation. Conventions: see `journeys-studio-db.md` head. Inputs:
`inventory.md` sections 7, 10, 11, 12, 34; `coverage-map.md`; source read through CodeGraph
(`internal/approvals/queue.go`, `ade/branchrequest.go`, `console/copyAll.ts`).

Approvals facts read from source: item id `<kind>:<ref>` stable for item life; `Queue.Answer` answers ids one by
one, per-id failure never aborts rest; answering resolved item returns NotFound `approval`; list sorted oldest
first by `CreatedAt` then id; branch request answer carries `shownItemIds`, an item joined after view rendered
stays pending.

## A. Definition view

### J-DEF-001 Definition as working reference while writing a query

- Status: partial (ui `definition`).
- Persona: developer checks indexes before writing WHERE.
- Steps:
  1. Open Definition of `orders` (Structure). Columns tab: column menu Copy name, Add to projection, Sort by.
  2. Indexes and Constraints tabs; Source tab: copy full DDL.
  3. Second client adds index; press Refresh.
  4. Switch to data tab of same table: projection and sort from step 1 applied.
- Oracle: clipboard column name exact (quoted identifier unquoted form, assumption); Source DDL runs on empty DB
  via second client and yields same columns, PK, indexes; new index listed after Refresh; data tab SQL (IPC args)
  holds projection and ORDER BY.
- Variations: Mongo Indexes/Validation (validator JSON copy parses); Kafka topic partitions/config; SQS attributes;
  ClickHouse engine and keys; view, materialized view, partition child; table with 600 columns.
- Risk: stale definition cache, DDL not round-trippable.

## B. Documents (MongoDB)

### J-DOC-001 Edit, insert, delete documents with rich EJSON types

- Status: partial (ui `document-view-readonly`, `document-new-save-double-submit`; adapter suites). New: target DB
  oracle through IPC (coverage gap 5).
- Persona: developer fixing bad records.
- Pre: collection with `_id` kinds ObjectId, string, int, compound document; fields NumberLong 2^53+1, Decimal128,
  Date before 1970, Binary, nested 6 levels, 3 MB document, field names with `.` and `$`.
- Steps:
  1. Open collection; first layer expanded; expand nested; virtual heights stable while scrolling 5,000 docs.
  2. Edit document: change Date and NumberLong, save. Double-click Save quickly.
  3. New document with explicit string `_id` already existing (duplicate key error), then unique id.
  4. Delete document with compound `_id`. Copy `_id`, Copy document.
  5. Page next past mixed `_id` types.
- Oracle: `mongosh` shows edited doc with types preserved (`$numberLong` exact, Date exact ms, untouched fields
  byte-equal BSON types); one insert only after double submit; failed duplicate insert leaves count unchanged;
  delete removes exactly compound-id doc; copied `_id` text pastes into `mongosh` filter and matches; paging visits
  each doc exactly once (count of distinct `_id` over all pages = collection count).
- Variations: read-only connection (edit/delete hidden); connection drop with editor open (buffer kept, Save
  disabled); document changed by second client before save (assumption: last write wins on `_id` replace, or
  conflict notice; pin actual).
- Risk: precision loss, double submit, mixed `_id` keyset gaps.

### J-DOC-002 Find and filter-rows over documents then Copy all

- Status: new.
- Steps: find `"status": "failed"`, toggle filter-rows, Copy all as Relaxed EJSON, switch tab and back, next page.
- Oracle: copied array holds only visible docs, valid JSON array, order as shown; find state kept across tab switch;
  rescan on page replace.
- Risk: copy all ignores filter.

## C. Key/value (Redis, S3 object)

### J-KV-001 Redis types tour with large collections

- Status: partial (ipc-frontend redis; adapter suites).
- Pre: string 2 MB, list 1,200 items, hash 5,000 fields, set 10k, zset with equal scores, stream 3k entries, key
  with TTL 60 s, key name `user:1:"quoted"\nline`, binary-unsafe key bytes.
- Steps:
  1. Open list key: page past 500 to item 1,200; copy item 1,001.
  2. Hash: scroll to field 4,999, edit not allowed (strings only): try; delete whole key (allowed).
  3. String key: edit, save. Key with TTL: edit after expiry.
  4. zset: page forward/back with cursor; refresh resets cursor.
- Oracle: `redis-cli LINDEX key 1000` equals clipboard; `GET` equals saved string, TTL preserved after edit
  (assumption: `SET ... KEEPTTL`; pin actual); edit of expired key reports gone, does not recreate silently
  (assumption); deleted key absent; zset pages cover each member once.
- Risk: list paging past 500, cursor reset, TTL wiped by edit.

### J-KV-002 S3 object edit, download, upload on versioned bucket

- Status: partial (ui `browse-s3-connection-drop`; adapter suite s3).
- Steps:
  1. Open 200 KiB JSON object, edit, save (conditional write). Second client overwrites object first, then save.
  2. Open 3 MB object (preview only, no edit); 6 MB object (no preview); Download both to disk.
  3. Read-only connection: Download still works, upload/edit refused.
  4. Object with SSE-KMS and tags: edit and save.
- Oracle: saved object bytes via `aws s3api get-object` equal editor buffer; conflicting save refused (ETag
  mismatch), remote keeps second client's bytes; downloaded file SHA-256 equals object ETag/sha; SSE-KMS key id
  preserved; tagged object refused per rule ("tagged refuse") with clear reason; no partial upload on drop.
- Variations: key with `/`, `%`, space, unicode, trailing slash "folder" marker; zero-byte object; cancel Save
  dialog on download (no file).
- Risk: overwrite race, metadata loss, `/` in key.

## D. Browse panel (Redis, S3)

### J-BRW-001 Navigate deep prefix tree with fast clicks

- Status: partial (ui `browse-folder-nav`).
- Steps:
  1. S3 bucket with 3 levels, one level of 200k keys. Click folder A then immediately Up then folder B (supersede).
  2. Type filter `2024-`; open leaf: preview in split, tab not replaced; resize split.
  3. Breadcrumb jump to root; Redis browse with `:` separator, same flow.
  4. Switch tab and back: level, filter, split kept.
- Oracle: shown level equals `list-objects-v2` of last clicked prefix (never A's late result); 200k level stays
  responsive (scroll frame budget, heap bounded); preview bytes equal object head.
- Risk: late level load lands over newer one, 200k nodes reactivity.

## E. Stream (Kafka, SQS)

### J-STR-001 Kafka: read window, tombstones, binary, produce

- Status: partial (ipc-frontend kafka; adapter suite).
- Persona: developer debugging a consumer.
- Pre: topic 6 partitions, messages incl tombstone (null value), binary Avro payload, 1.5 MiB message, key with
  unicode, headers.
- Steps:
  1. Open topic; select partitions 0, 3; since-timestamp 10 min ago; load.
  2. Offset window: jump to offset N on partition 3; scroll live buffer to 10k cap.
  3. Select tombstone row and binary row; copy value; copy row.
  4. Compose: produce message with key, headers, JSON value to partition 2; produce tombstone (if offered).
  5. Disconnect mid-follow; reconnect.
- Oracle: second consumer (kcat) sees produced message on partition 2 with exact key/headers/value bytes; tombstone
  shows NULL marker and copies as empty/NULL not `""` (pin), binary marker not mojibake; consumer group offsets
  untouched by browsing (`kafka-consumer-groups --describe` equal before/after); 1.5 MiB message flagged, not
  silently dropped by 1 MiB fetch clamp; Send disabled while dropped.
- Risk: tombstone coalesced, group offset commit side effect.

### J-STR-002 SQS: poll without harming queue, send, delete

- Status: partial (ui `sqs-poll-visibility`; adapter suite).
- Persona: on-call engineer inspecting DLQ source queue with redrive `maxReceiveCount` 3.
- Steps:
  1. Open queue; visibility timeout setting default 1 s; confirm dialog names redrive limit. Poll.
  2. Poll 3 more times slowly; duplicates deduped by MessageId; poll stops at duplicate batch.
  3. Send message (FIFO queue: group id, dedup id). Delete a message after waiting past visibility timeout.
- Oracle: `ApproximateReceiveCount` of each message after 4 polls: assert no message moved to DLQ (DLQ depth
  unchanged); sent message visible to second client with FIFO fields; stale receipt handle delete refused with
  clear text, message still present; deleted fresh message gone.
- Variations: empty queue; 10k messages; message body 256 KiB; binary attributes.
- Risk: browsing bumps receive count into DLQ (P108).

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

## G. Studio MCP: database tools

### J-MCP-001 Agent session from install to audited write

- Status: partial (`dbmcpflow` approval flow, `mcp_test.go` row count; ui `dbmcp-approval*`). New: one chain with
  restart and target check per step.
- Persona: developer lets Claude fix data on staging.
- Pre: SQLite or Postgres target, connection `mcp`: read allow, write prompt, ddl deny, masked allow.
- Steps:
  1. Settings Studio MCP: enable, copy install command, Regenerate token (old token must die).
  2. MCP client `list_connections`; `describe`; `run_query mode=read` SELECT.
  3. `run_query mode=write` UPDATE one row: approval dialog (target banner colour, host, database, class write),
     toggle Formatted/As sent; approve.
  4. `run_query mode=read` with an UPDATE inside (refused by classifier and read lane).
  5. `run_query mode=ddl` DROP: denied without prompt.
  6. `run_query mode=write` with DDL text: refused (more powerful than declared).
  7. Restart app; client reconnects with same token.
- Oracle: second client DB state: exactly step 3 row changed; old token gets 401 after Regenerate; new token works
  after restart (settings hydration); op log rows carry source MCP and tool name; statement shown in dialog "As
  sent" byte-equals statement executed (server log).
- Variations: user has open transaction on connection (MCP read refused, P330); unclassified statement (strictest);
  `SELECT ... INTO`, CTE with DML, `EXPLAIN ANALYZE UPDATE`, multiple statements in one call.
- Risk: classifier bypass, token hydration, dialog shows other text than executed.

### J-MCP-002 Table allow list under joins, views, schemas

- Status: partial (`dbmcpflow` table allow-list).
- Steps: set `mcp.tables: [app.people, app.orders]`; agent queries `people`, `app.people JOIN app.secrets`, view
  over `secrets`, `"APP"."People"`, CTE named `people` shadowing, subquery in WHERE on `secrets`, `browse_table
  secrets`, `describe secrets`.
- Oracle: only allowed relations return rows; every refusal names relation; describe/list hide others (assumption);
  open-item cases (view definition server-side) pinned as documented, not silently "safe".
- Risk: parse gaps leak tables.

### J-MCP-003 Expensive query gate, exemption, throttle, timeout together

- Status: partial (`dbmcpflow` Expensive, exemptions, Throttle, Timeout*).
- Steps:
  1. Agent `run_query SELECT * FROM big_rows` (over threshold): refused with EXPLAIN estimate and exemption hint.
  2. Agent requests exemption `exm_`: user sees exemption item, approves; agent reruns with exemption.
  3. Agent fires 20 reads in parallel (throttle gate paces one per interval).
  4. Agent runs `pg_sleep(30)` with absent timeout (10 s default).
- Oracle: step 1 no rows shipped, server ran EXPLAIN only; step 2 exemption single-use or scoped as documented
  (pin), always raises approval even when read is allow; step 3 server log timestamps spaced at least throttle
  interval; step 4 killed about 10 s, backend gone.
- Variations: EXPLAIN fails (refuse, fail closed); ClickHouse; engine without EXPLAIN (NoExplain path).
- Risk: fail-open on explain error.

### J-MCP-004 Result review before delivery

- Status: partial (`dbmcpflow` review; ui `dbmcp-review-*`).
- Steps: set `mcp.review.read: review`; agent reads 500 rows incl masked column; user sees held result in dialog
  with grid, selects cells, copies, denies; agent retries, user approves; agent sends 80 MiB result (cap).
- Oracle: denied result never reaches client (MCP response is refusal, no row text); approved response equals
  reviewed rows; clipboard from review grid holds masked values; over-cap result refused with clear code, memory
  released (held bytes 0 after).
- Variations: unknown review value in YAML coerces to review; preview responses (P333); app quit with held result
  (client gets error, nothing delivered).
- Risk: delivery before approval, unmasked review copy.

### J-MCP-005 browse_table paging stability

- Status: partial (`dbmcpflow` browse 16 tests).
- Steps: agent pages `browse_table people` 50 rows at a time to end while second client inserts rows; requests
  count; filter value with `'` and `;`; offset 100000+; table without PK; masked default.
- Oracle: union of pages has no duplicates and misses no pre-existing row (stable order by PK); count equals
  `count(*)`; `;` filter refused per open item (pin); offset limit error explicit; no-PK table states unstable
  order in result meta.
- Risk: pagination duplicates/gaps.

### J-MCP-006 Unreachable and lost connections raise no prompts

- Status: covered (`dbmcpflow` unreachable; ui `dbmcp-unreachable`); extend: mid-approval loss.
- Steps: agent write request opens approval; server dies while dialog open; user approves.
- Oracle: approval answer returns error to agent (connection lost), DB unchanged, item removed, no second prompt;
  later requests while down return error without prompt.
- Risk: prompt for dead server, approve executes on reconnected different host.

### J-MCP-007 Operations tools

- Status: partial (ui/flow P518).
- Steps: agent starts long `run_query`; second MCP call `list_operations`; `cancel_operation` id; user also presses
  Stop in status bar ops widget at same time.
- Oracle: one cancel reaches server; both callers get clean result (one ok, other already-ended); op row
  `cancelled`; first tool call returns cancelled error, not hang.
- Risk: double cancel race, stuck running row.

## H. Studio MCP: scripts, API, Kubernetes tools

### J-MCP-010 Scripts tools short and long mode

- Status: partial (`dbmcpflow` scripts; ui `dbmcp-script-long`, `dbmcp-approval-script`).
- Steps:
  1. Run "deploy-check" has `mcp` block opt-in; another script has none.
  2. Agent `list_scripts` (only opted-in), `run_script` with params incl secret param and select param with invalid
     value.
  3. Approval dialog shows command and params (secret masked); approve.
  4. Long mode: agent polls `get_script_run`, `get_script_logs` paging to end; user presses Stop in Runs tab.
  5. Agent runs same script again while first live.
- Oracle: non-opted script refused by name; invalid select refused before prompt; process ran with secret in env
  (script writes hash of it to file; file hash matches), logs returned to agent show masked secret; Stop yields
  outcome cancelled to agent; concurrent run refused `E_CONFLICT`; Runs tab shows same run (one tab per script key).
- Risk: secret in logs to agent, concurrent run.

### J-MCP-011 api_request with rules, credentials, redirects

- Status: partial (`dbmcpflow` API 16 tests; ui `dbmcp-api-*`).
- Steps:
  1. Rules: allow GET `api.internal`, deny `*.prod`, default ask. Per-host agent credential for `api.internal`.
  2. Agent GET allowed host (no prompt); POST same host (ask); GET `x.prod` (deny).
  3. Approved request answers 302 to other host; to same host other path; https to http.
  4. Agent uses `{{secret}}` variable of active environment; user changes variable between plan and send.
  5. Response 20 MB.
- Oracle: wire recorder (`flowharness.HTTP`): credential header present only on `api.internal` requests, absent
  after cross-host or downgrade redirect; hand-approved request does not follow redirect to unapproved target
  without new decision; changed variable invalidates approval (replan); response capped with flag; secret value
  never in MCP result or op log.
- Risk: redirect credential leak, plan-to-send race.

### J-MCP-012 Kubernetes MCP per cluster

- Status: partial (`dbmcpflow` Kubernetes MCP; ui `kubernetes-mcp`).
- Steps: cluster A MCP off, B prompt, C allow with description; agent lists pods on each; describes Secret on C;
  tails logs on B (approve), user reorders clusters meanwhile.
- Oracle: `KubeFake.Requests()`: none for A; B only after approval; Secret values redacted in tool result; op log
  kind `kubernetes` rows with "Via" tool; cluster ids stable across reorder.
- Risk: off cluster reachable, secret values leak.

## I. Approvals queue

### J-APR-001 Mixed queue from four sources, one at a time

- Status: partial (ui `approvals-one-at-a-time`, `approvals`; flows per kind).
- Persona: user returns from lunch to 7 pending approvals.
- Pre: pending: 2 db.query (different connections), 1 db.exemption, 1 script.run, 1 api.request, 1 branch.request,
  1 task.create.
- Steps:
  1. Dialog shows item 1 of 7 (oldest), Deny focused. Press Escape: minimised to toast, nothing denied.
  2. Reopen from toast. Approve item 1; badge 1 of 6.
  3. New db.query arrives while reading item 2 (joins end, current item not replaced).
  4. Item 3's agent times out/abandons while shown (item vanishes, next shown, no error modal).
  5. Deny all (confirm): remaining denied.
- Oracle: each agent/tool call returns matching outcome (approved ran; denied got denial text; none approved
  unseen); target DB changed only by item 1; order oldest first by created time; answering vanished item gives
  NotFound handled silently; after Deny all queue empty, toast gone.
- Variations: Enter key with Deny focused denies (never approves by reflex); double-click Approve (second click
  must not approve next item); two windows: answer in window 2 clears window 1; app restart with pending items
  (agents get error, queue empty on boot).
- Risk: approving unseen item (P618), requestId keying across windows.

### J-APR-002 Branch request item joined after render

- Status: partial (`adeflow` BranchRequest; ui `ade-v2-branch-request`).
- Steps: agent requests branches in repo A and B; dialog shows 2 items; second agent of same task adds repo C
  request while open; user approves with A ticked, B unticked.
- Oracle: worktree created for A only (`git worktree list`), B denied, C stays pending in new dialog round (not
  auto-approved); agent 1 gets result naming A approved, B denied.
- Risk: late-joined item approved unseen.

### J-APR-003 Focus on approval across windows and modules

- Status: partial (`dbmcpflow` focus; ui `dbmcp-focus`).
- Steps: user types in terminal tab of window 2 (Git module); approval arrives from session started in window 1;
  `dbMcp.focusOnApproval` on; five requests in 1 s; then setting off.
- Oracle: `WindowFocus` fake: one raise for the burst, target window per prompt routing (origin window, else main);
  keystrokes typed during raise not delivered into dialog buttons (no accidental approve: Deny focus plus 2 s
  guard); with setting off zero raises, toast only.
- Risk: keystroke hijack approves.

## J. Validation proof

### J-PRF-001 Proof session from agent to screenshots on disk

- Status: partial (`proofflow` ProofWindow, ProofShotSaveAll, ProofShotCopyAll; ui `proof-*`).
- Persona: reviewer checks agent's validation evidence.
- Steps:
  1. Agent `proof_save` with 4 steps: table view (ints 2^63-1, NULLs, unicode), chart (1,000 points), Mermaid
     diagram, JPEG image.
  2. Proof ready prompt: Open (dialog closes, window opens). Step tabs; sort table; select cells; Cmd+C; open
     cell in read-only editor.
  3. Change panel: accept step 1, reject step 2 with note.
  4. Save all screenshots to chosen folder with name containing spaces; Reveal. Copy all (macOS multi-image).
  5. Agent saves same session name again (collision); 31st session (retention 30).
- Oracle: clipboard TSV of table cells equals agent's raw args (int precision exact); JPEG files exist one per
  step, decodable, dimensions > 0, folder named `<session>`; `ImageClipboard.Writes()` count = steps; accept/reject
  delivered to agent tool result; collision yields distinct session (suffix), oldest of 31 purged from disk and
  list; proof attached to task chain entry.
- Variations: chart 1,001 points (refused/capped), invalid Mermaid (source plus "Loading error."), image over cap,
  Save cancelled (no files), clip leaves no temp files.
- Risk: integer precision, PNG/JPEG clipboard, ready dialog stays open.

### J-PRF-002 Proof window view state and restart

- Status: partial.
- Steps: per step set sort, column widths, chart kind; switch steps; close window; reopen via Window > Validation
  proof; restart app.
- Oracle: view state per step kept while session open; proof windows never restored at boot (purged), session
  history list still holds session; reopening shows saved content byte-equal.
- Risk: ephemeral window restored, state bleed between steps.

## K. Kubernetes module

### J-K8S-001 Incident triage across clusters

- Status: partial (`kubeflow` 19 tests; ui `kubernetes-*`).
- Persona: SRE following a crashlooping pod.
- Pre: kubeconfig with 3 contexts (one exec plugin non-interactive, one unreachable), namespace list restricted on
  one.
- Steps:
  1. Pick cluster 1, namespace via typed name (list forbidden), kind Pods from kind bar; filter `api-`.
  2. Click pod (preview tab), double-click (kept). Describe; copy describe text.
  3. Logs: tail 2,000, follow, previous container, timestamps; select 300 lines, copy; scroll up while following
     (autoscroll pauses, assumption).
  4. Open Secret describe: values hidden. Switch kind via categorized picker to CRD; recent kinds updated.
  5. Second window: reorder clusters with Alt+Up; window 1 list follows. Cancel log stream from Ops row.
  6. Unreachable cluster: status reason shown, no hang; restart app: tabs, cluster order, recent kinds.
- Oracle: `KubeFake.Requests()` has no secret value served into DOM/clipboard; copied log text equals appended
  lines in order, no duplicate lines at follow reconnect; Ops Cancel closes follower (`Followers` 0); every user
  action one op row, background refetch none; order persisted by stored id after restart.
- Variations: 5,000 pods (virtual list), pod deleted while tab open, log line 1 MB, ANSI colours in logs, cluster
  removed from kubeconfig while app runs.
- Risk: secret leak, log duplication, stream leak on window close.

### J-K8S-002 Keyboard-only resource navigation with search active

- Status: partial (ui `kubernetes-order`).
- Steps: type in cluster search; try Alt+Down reorder (disabled while searching); clear search; reorder; Enter
  opens; Cmd+W closes module-local tab; Window menu next/prev tab.
- Oracle: order unchanged while searching; Window menu keys move Kubernetes tabs (P493).
- Risk: reorder applied to filtered index.

## L. Docker module

### J-DKR-001 Day with Compose stack: logs, exec, stats, edit

- Status: partial (`dockerflow`; e2e-real docker-real; ui `docker-*`).
- Persona: developer running local stack.
- Pre: Compose project `shop` (web, db, worker), one Testcontainers container, one devcontainer.
- Steps:
  1. Docker module: containers grouped by project with origin icons. Stop `worker`, Start, Restart `web`.
  2. Logs of `web`: follow, filter `ERROR`, timestamps, wrap; select and copy 50 lines; 2000-line cap.
  3. Exec into `db`: New session; run `psql -c 'select 1'`; close tab; open again (new explicit session).
  4. Stats tab open in two windows; close window 2.
  5. Edit `web`: memory limit in place (no recreate); rename; then change env (recreate confirm lists loss); cancel
     once, then confirm.
  6. Images/volumes/networks tabs; storage `/system/df` Refresh; registry button opens URL.
- Oracle: `docker inspect`: state after each action; memory limit applied without new container id; rename
  applied; recreate yields new id with same name, volumes, networks, labels, ports, and old container gone;
  cancel changes nothing; exec process in container ends when tab closes (`docker top`); stats stream for window 2
  ended (`DockerStatsEndWithTheirWindow`); clipboard log lines equal `docker logs` subset; `Browser.Opened` has
  registry URL.
- Variations: recreate fails midway (rollback: old container restored and running); container removed by CLI while
  edit tab dirty; log line with ANSI and 100 KiB; 400 containers (virtualized list heap).
- Risk: recreate data loss, exec leak, stream leak.

### J-DKR-002 Engine restart and restore

- Status: partial (`dockerflow` restore 8 tests; ui `docker-restore`).
- Steps:
  1. Stack running; user stops `worker` by hand. Engine restarts (BootGen).
  2. Banner in Docker module only; open restore dialog; restore.
  3. Engine restarts again mid-restore; dismiss; relaunch app (AtLaunch offer).
- Oracle: `docker ps`: `db` then `web` started in dependency order after health; `worker` (user-stopped) and
  ephemeral Testcontainers not started; dismiss persists (no re-offer for same boot generation); snapshot survives
  idle engine.
- Risk: restoring user-stopped containers, wrong order.

### J-DKR-003 Engine states and context switch

- Status: partial (`dockerflow` StatusClasses, Contexts).
- Steps: no docker binary (not-installed), daemon down, `DOCKER_HOST` tcp with TLS paths, switch context via CLI
  while module open, socket permission denied.
- Oracle: state label per case; after context switch list equals `docker --context X ps`; no credential file read
  (guard G5; strace-free: `config.json` atime unchanged, assumption test via fake HOME file access log).
- Risk: stale engine after context switch.
