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

## Second pass: key/value sessions (Redis, S3 object)

Runs in seconds: Redis container with redis-cli as second client; S3 against MinIO-style container or fake S3
HTTP; TTL cases use keys with `PX` of 100 ms to 300 ms set by fixture or already expired, not long waits;
uploads use sparse temp files.

Facts read from code (CodeGraph): Redis edit is `SET key value KEEPTTL`, string keys only, other types refused
"only string-type keys are editable" (`redis/mutate.go:52,91-100,117`); insert is `SET NX`, collision "key
already exists" (`:149-155`); delete is `DEL` for any type (`:125-135`); key gone at read is `E_QUERY` "key no
longer exists" (`redis/read.go:34-38`); hash, set, zset read by SCAN family, forward only (`:143-145`); set rows
get running index carried in page token (`:139-141`); memory usage best effort (`:57-60`). S3 edit: `HeadObject`
first, object with tags refused (`s3/mutate.go:255-261`), ACL grants read and re-sent, unreadable ACL logged and
edit proceeds (`:173-184`), conditional put used with `PreconditionFailed` detection (`:239`); download writes
sibling temp `<dest>.kira-partial-<id>`, renames last, removes temp on any error (`s3/transfer.go:47-88`);
upload refuses non-regular file and file over single-upload limit, no multipart (`:102-120`).

### J-KV-010 Redis: session cache triage with TTL, type limits, concurrent expiry

- Status: partial (ui keyvalue edit; redis adapter suite; chain new).
- Persona: developer debugging stale sessions.
- Pre: DB 0: 200,000 keys `sess:<id>` (string, TTL), `cart:<id>` (hash 5,000 fields), `tags` (set 30,000),
  `rank` (zset 30,000), `queue` (list 50,000), `events` (stream), `bin` (binary string 1 MiB), key with
  spaces, newline and unicode in name; DB 3 has other data.
- Steps:
  1. Open DB 0 browse; filter pattern `sess:*`; open one key: value, TTL countdown, memory shown.
  2. Edit value; Save. Check TTL.
  3. Add key `sess:new` with value; add again same name.
  4. Open `cart:<id>` hash: page through 5,000 fields forward; try Previous.
  5. Try edit hash field value.
  6. Open `tags` set: page 3 times; row indexes continue; second client `SADD` during paging.
  7. Open `rank`, `queue`, `events`: values shown per type; copy rows.
  8. Open `bin`: binary shown escaped or hex; copy; download if offered.
  9. Open key that expires between list and open (fixture `PX 100`).
  10. Delete `cart:<id>` (hash) and key with newline in name.
  11. Switch tab to DB 3; open key with same name as DB 0 key; edit.
  12. Search within value rows; filter-rows; Copy all.
  13. Server restarts (fixture); Refresh; reconnect; continue paging from start.
  14. Restart app.
- Oracle (redis-cli): 2 value changed and `PTTL` still positive, within original; 3 second add fails "key
  already exists", first value kept; 4 forward tokens visit all 5,000 fields at least once, Previous disabled
  with reason (forward-only); 5 refused with type message, no write; 6 indexes monotonic without restart at 0,
  possible duplicates under concurrent write tolerated and never crash; 8 clipboard round-trips to same bytes
  through documented encoding; 9 "key no longer exists" message, list entry removed on refresh; 10 `EXISTS` 0
  for both, exact key name sent (newline preserved); 11 write lands in DB 3 only; 12 clipboard rows equal
  filtered rows; 14 tab reopens on same DB and key.
- Risk: TTL wiped by edit; write to wrong database.

### J-KV-011 Redis: restricted ACL user, replica, cluster redirect, huge value

- Status: new.
- Pre: ACL user `app` (`~sess:* +get +set +ttl +type +scan`), replica endpoint (read-only), key 512 MiB (fixture
  via `SETRANGE`), key of unknown module type.
- Steps:
  1. As `app`: browse shows only what SCAN returns; open `cart:1` (no permission).
  2. Edit `sess:1` (allowed); delete `sess:1` (no `+del`).
  3. Memory usage denied: value still loads without memory figure.
  4. Connect to replica: edit attempt.
  5. Open 512 MiB key: bounded read with truncation marker; edit attempt on truncated value.
  6. Open module-type key.
  7. Cancel slow SCAN over 10M keys.
- Oracle: NOPERM text names command; best-effort fields absent, not error; replica write refused by server and
  shown; 5 never `SET`s truncated value (redis-cli `STRLEN` unchanged); 6 unsupported type message; 7 no
  further SCAN commands after cancel (`MONITOR` capture in fixture).
- Risk: truncated value written back.

### J-KV-012 S3 object: edit small config object preserving metadata, conflicts, tags

- Status: partial (s3 adapter suite; ui keyvalue S3).
- Pre: bucket `cfg` versioned; `app.json` (content-type `application/json`, cache-control, user metadata,
  public-read ACL); `tagged.txt` with 2 tags; `big.bin` 200 MiB; `enc.txt` with SSE-KMS; bucket with ACLs
  disabled; user without `s3:GetObjectAcl`.
- Steps:
  1. Open `app.json`: body shown formatted; metadata pane.
  2. Edit one value; preview shows `PutObject s3://cfg/app.json (<size>)`; Save.
  3. Second client overwrites `app.json`; user edits stale view; Save.
  4. Refresh; edit; Save.
  5. Open `tagged.txt`; edit; Save.
  6. As user without `GetObjectAcl`: edit `app.json`; Save; read app log.
  7. In ACL-disabled bucket: edit; Save.
  8. Open `big.bin`: not loaded inline; edit unavailable.
  9. Open `enc.txt` without KMS permission.
  10. Delete `app.json` on versioned bucket; list shows gone; second client sees delete marker.
  11. Read-only connection: edit and delete.
  12. Copy object URI, key, body; copy presigned URL if offered.
- Oracle (aws-cli as second client): 2 content-type, cache-control, user metadata, ACL public-read all preserved,
  new version id; 3 precondition failure reported "changed remotely", remote content kept (or, where store lacks
  conditional put, documented fallback: OQ-KV-1); 5 refused with tags message, object unchanged; 6 edit applied,
  warning logged once, ACL reset risk stated to user (OQ-KV-2); 9 access error names KMS; 10 delete marker, older
  versions intact; 11 no request sent; clipboard exact.
- Risk: silent metadata or ACL loss on edit.

### J-KV-013 S3 transfer: download and upload with cancel, bad paths, limits

- Status: partial (flows s3 transfer).
- Steps:
  1. Download 50 MiB object to `~/Downloads/a.bin` (fixture throttled stream).
  2. Cancel mid-stream.
  3. Download again to completion.
  4. Download to existing file path; to directory without write permission; to path on full disk (fixture).
  5. Download key deleted after listing.
  6. Upload local file into prefix `in/`; content-type guessed or chosen.
  7. Upload path that is directory, named pipe, `/dev/zero`, missing file.
  8. Upload file over single-upload limit (sparse file).
  9. Upload over existing key.
  10. Cancel upload mid-way.
  11. Quit app during download.
- Oracle: 2 destination absent and no `.kira-partial-*` sibling left; 3 file sha256 equals object; 4 existing
  file replaced only at final rename (old content intact until then), permission and disk errors leave no
  partial file; 5 not-found before any local file created; 7 each refused before network call (wire recorder
  empty) with message naming path; 8 refused naming limit; 9 overwrite confirmed first (pin) and new version on
  versioned bucket; 10 no object or incomplete upload left; 11 no partial file after relaunch (or cleaned at
  start).
- Risk: partial file presented as complete download.
