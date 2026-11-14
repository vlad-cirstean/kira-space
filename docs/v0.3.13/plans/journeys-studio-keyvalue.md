# P627 journeys: Studio key/value view (Redis, S3 object)

Split from `journeys-studio-db.md` and `journeys-studio-views.md` (P627 first pass); J-ids unchanged.
Imagined usage and definition of correct. No implementation, no test code.
Conventions: `journeys-studio-connections.md` head.

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
