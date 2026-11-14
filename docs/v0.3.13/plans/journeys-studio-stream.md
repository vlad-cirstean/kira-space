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
