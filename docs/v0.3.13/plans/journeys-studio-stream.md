# P627 journeys: Studio stream view (Kafka, SQS)

Split from `journeys-studio-db.md` and `journeys-studio-views.md` (P627 first pass); J-ids unchanged.
Imagined usage and definition of correct. No implementation, no test code.
Conventions: `journeys-studio-connections.md` head.

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

## Second pass: stream sessions (Kafka, SQS)

Runs in seconds: Kafka single-broker container (or `kfake`) with `kcat`-style second client; SQS against
ElasticMQ-style container or fake SQS HTTP. Visibility-timeout cases use queue `VisibilityTimeout` of 0 s or 1 s
set by fixture (receipt cache reads wall clock, `sqs/read.go:115,135`; no clock seam: OQ-STR-3); empty-poll
bounds use adapter constants, no added sleeps.

Facts read from code (CodeGraph): Kafka browse uses `kgo.ConsumePartitions` at explicit offsets, never a
consumer group (`kafka/read.go:342-357`); page token carries per-partition windows `[Next, End)` fixed at first
browse (`:310-329`); rows outside window skipped (`:429-431`); count is offset span, flagged inexact
(`:604-610`); cancel maps to cancelled code (`:386-388`); Kafka accepts insert only, "update" and "delete"
unsupported (`kafka/produce.go:29-33`); sentinels `$key`, `$body`, `$headers`; headers ordered, repeats kept,
values must be strings (`adapters/rowops.go:63-104`); partial produce error states landed count and failed
positions (`produce.go:130-138`). SQS: every poll independent, strategy `batch`, no paging
(`sqs/read.go:200-204`); receive up to 10 per call; receipt handle cache 5,000 FIFO (`:31`); delete of message
never polled "receipt gone", past visibility timeout "receipt expired" (`:130-138`); no update
(`sqs/mutate.go:93-95`); FIFO queue needs `$messageGroupId`, and `$messageDeduplicationId` unless content-based
dedup (`:52-74`); repeated header name keeps last value (`rowops.go:106-118`).

### J-STR-010 Kafka: investigate poison message across partitions without touching groups

- Status: partial (kafka adapter suite; ui stream specs; chain new).
- Persona: developer chasing consumer crash.
- Pre: topic `orders` 6 partitions, 300,000 records; keys null, text, binary; values JSON, Avro bytes, empty,
  tombstone (null), 900 KiB; headers with repeated names; compacted topic `state`; transactional markers present;
  consumer group `billing` with committed offsets; ACL user without `Describe` on other topic.
- Steps:
  1. Open `orders` stream tab: first page of 100 newest-window rows; partition, offset, timestamp, key columns.
  2. Next page 5 times; note partition/offset pairs.
  3. Producer (second client) appends 1,000 records during paging; continue paging to end of window.
  4. Refresh: new window includes appended records.
  5. Filter by partition 3 and offset >= N; then by timestamp range.
  6. Search `"orderId":"A-17"` in page; filter-rows; open row detail; body pretty JSON; headers table.
  7. Copy body; copy key; copy row as JSON; Copy all on page.
  8. Open tombstone row and binary row; copy.
  9. Open 900 KiB record: truncated marker; load full; copy.
  10. Count messages (inexact label).
  11. Open compacted topic `state`: gaps in offsets; count vs rows.
  12. Stop during slow page (broker paused by fixture).
  13. Broker restart; Next page; reconnect; continue.
  14. Open topic without permission.
  15. Restart app; tab restores filter, not position.
- Oracle: second client: consumer groups list unchanged, `billing` committed offsets unchanged, no new group
  (structural no-group promise); step 2 and 3 no duplicate partition/offset pair, none beyond window end;
  appended records appear only after 4; filter 5 rows all satisfy predicate and count scoped to filter; clipboard
  body bytes equal record value (binary through stated encoding, tombstone as explicit null marker, not empty
  string: OQ-STR-1); headers order and repeats kept in JSON copy; 10 labelled approximate; 12 cancelled state, no
  error toast; 13 same token continues or clear "position lost, refresh" (pin); 14 authorization error names
  topic.
- Risk: browsing moving consumer offsets; tombstone indistinguishable from empty.

### J-STR-011 Kafka: produce batch with headers, partial failure, retry without duplicates

- Status: partial (kafka produce tests).
- Pre: topic `in` (max message bytes 1 MiB), topic `ro` without write ACL, read-only connection entry.
- Steps:
  1. Compose message: key `k1`, body JSON, headers `{"trace":"a","trace":"b","type":"x"}`; preview shows
     `ProduceSync in key=k1`.
  2. Send. Refresh: row present.
  3. Compose without key; with empty body; with body absent.
  4. Headers not an object (`[1]`), value not string (`{"n":1}`), malformed JSON.
  5. Compose 5 messages in one batch where 3rd exceeds 1 MiB.
  6. Read error text; resend only failed one after shrinking.
  7. Send to `ro`.
  8. Read-only connection: compose.
  9. Try edit and delete on existing row.
  10. Send while broker down (fixture); broker returns; resend.
  11. Double-click Send fast.
- Oracle (second client consuming `in`): 2 one record, headers in order `trace=a, trace=b, type=x`; 3 null key
  record; empty body stored as empty value, absent body refused "requires a $body"; 4 each refused before
  broker (no record), message names problem; 5 result says 4 of 5 landed and failed position 3; after 6 topic
  holds exactly 5 new records, none twice; 7 authorization error, nothing landed; 8, 9 refused, no request; 10
  no duplicate after resend (record count); 11 one record.
- Risk: retry of partially landed batch duplicating messages.

### J-STR-012 SQS: inspect queue without harming consumers, delete, FIFO rules

- Status: partial (sqs adapter suite; ui stream SQS).
- Pre: standard queue `jobs` (visibility 1 s for test, redrive max receive 3 to `jobs-dlq`), 25 messages with
  attributes (String, Number, Binary); FIFO queue `tx.fifo` (no content dedup) and `cb.fifo` (content dedup);
  second client able to read `ApproximateReceiveCount`.
- Steps:
  1. Open `jobs`: poll returns batch; no pager; banner explains each poll is snapshot and counts as receive.
  2. Poll again twice; watch receive counts; warning before message reaches max receive (DLQ risk).
  3. Open row: body, attributes JSON, sent timestamp; copy body and attributes.
  4. Delete one message right after poll.
  5. Delete message after visibility expiry (fixture visibility 0 s or wait-free expired handle).
  6. Delete message shown in list restored from before app restart (never polled this session).
  7. Send message with attributes `{"a":"1","a":"2"}`.
  8. Try edit of body.
  9. Send to `tx.fifo` without group id; with group id only; with both ids; same dedup id twice.
  10. Send to `cb.fifo` with group id only.
  11. Poll `jobs-dlq`; move nothing (no redrive feature): absence.
  12. Credentials expire (fixture 403 `ExpiredToken`); poll; refresh credentials; poll.
  13. Read-only connection: send, delete.
  14. Purge or batch delete if offered; else absent.
- Oracle (second client): 2 receive count rises per poll and UI states it; app never polls on timer or on tab
  focus (request log: `ReceiveMessage` only on user action); no message reaches DLQ through opening tab once; 4
  message gone; 5 "receipt expired" message, message still in queue; 6 "receipt gone", nothing deleted; 7
  attribute `a` = `2` (last wins) and user told or documented (OQ-STR-2); 8 unsupported with hint delete plus
  resend; 9 errors name missing field, accepted send appears once, duplicate dedup id yields one message; 10
  accepted; 12 expired-credential message, no retry storm; 13 no request.
- Risk: viewer pushing messages to DLQ; deleting wrong message through stale handle.

### J-STR-013 Stream view state: filter history, search, compose draft, windows, restart

- Status: partial (ui stream filter history).
- Steps:
  1. Run 12 distinct filters on Kafka tab; open history menu: newest first, capped, no duplicates.
  2. Pick old entry with keyboard; edit; run; pin if offered.
  3. Search toolbar: match count, next/prev, filter-rows; then Next page: search persists, matches recomputed.
  4. Start composing message; switch tab; return: draft kept.
  5. Open same topic in second window; send there; first window shows stale strip until refresh.
  6. Column resize and order; row height with wrap toggle.
  7. Select 30 rows with Shift+click across virtual scroll; copy as TSV and JSON.
  8. Close tab with draft: prompt or silent drop (pin).
  9. Restart app.
- Oracle: history rows in app DB per connection and topic; clipboard has 30 rows exact order; after restart
  filter text, columns, history restored; draft not restored unless documented; no network request at restore
  until tab visible.
- Risk: copy of virtualized selection; history leaking between connections.
