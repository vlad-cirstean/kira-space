# P627 journeys: Studio project tree, browse panel, definition view

Split from `journeys-studio-db.md` and `journeys-studio-views.md` (P627 first pass); J-ids unchanged.
Imagined usage and definition of correct. No implementation, no test code.
Conventions: `journeys-studio-connections.md` head.

## B. Project tree

### J-TREE-001 Keyboard-only tour of large tree

- Status: partial (ui `tree`; single large test).
- Persona: keyboard user.
- Pre: Postgres with 3 schemas, 2,000 tables in one, partitioned table with 300 partitions.
- Steps:
  1. Focus tree, arrow to connection, Right expands, type-ahead not assumed; Down to schema, Right.
  2. Scroll 2,000 tables with PageDown; sticky headers show at most 3 rows.
  3. Enter on table (preview tab), Enter again on another (preview replaced), Cmd+C on a table, Shift+Alt+C.
  4. Expand partitioned parent; collapse while children still loading.
  5. F2 rename a table to `"Weird ""name"" ü"`; Delete another (confirm).
- Oracle: clipboard after Cmd+C equals table name; after Shift+Alt+C equals qualified path
  (`<connection>/<db>/<schema>/<table>`, exact separator per code); second client sees rename with exact
  identifier and dropped table gone; collapse in step 4 wins (node stays collapsed when load lands).
- Variations: truncated children flag at cap; rename to existing name (error, tree unchanged); rename on read-only
  connection (refused); disconnect during expand (children must not land).
- Risk: collapse ignored, late children after disconnect, identifier quoting.

### J-TREE-002 Metadata cache while disconnected, then reconnect to changed schema

- Status: partial (cache unit tests; journey new).
- Steps:
  1. Connect, expand all of `app`. Disconnect.
  2. Browse tree offline (served from cache), open Definition of `people` (cached or clear "connect to load").
  3. Second client: drop `app.tmp`, add `app.fresh`, add column `people.nick`.
  4. Reconnect, expand `app`.
- Oracle: after step 4 listing shows `fresh`, no `tmp`, columns include `nick` without manual refresh; cache row
  `Since` updated; no stale listing reappears after a second collapse/expand.
- Variations: refresh racing with invalidation; listing truncated on refresh after a complete cached one.
- Risk: stale cache after reconnect (P24, P108).

### J-TREE-003 Same table open in three tabs, drop it from tree

- Status: new.
- Steps:
  1. Open `app.tmp` as permanent tab, as second tab with filter, and a console tab referencing it.
  2. Stage one edit in tab 1. Delete table from tree (confirm).
- Oracle: second client: table gone. Tabs: assumption: data tabs show error state on next load, staged edit
  discardable, no crash, no commit possible; console untouched. Persisted tabs after restart reopen without crash
  (`openTrackedTab` on missing object).
- Risk: crash on deleted object (P173), staged commit against recreated table of same name.

### J-TREE-004 Kafka and Mongo tree specifics

- Status: partial (ipc-frontend kafka; ui mongo fixtures).
- Steps:
  1. Kafka: expand Topics (500 topics) and Consumer groups; open topic; copy name.
  2. Mongo: expand database with 0 collections, with a view, with a time-series collection; Cmd+N new collection
     named `a.b$c` (invalid) then `événements`.
- Oracle: Kafka admin lists unchanged; Mongo `listCollections` shows `événements`, invalid name refused with driver
  text, no partial create.
- Risk: special characters in identifiers.

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

## Second pass: tree, browse and definition sessions

Runs in seconds: adapter containers or fakes; large listings seeded by fixture; no waits.

Facts read from code (CodeGraph): browse level holds what one `treeChildren` call returns, no paging; `truncated`
flag when adapter stops at round budget, Refresh only retry (`browse/state.ts:14-27`); Redis level may hold up to
200,000 keys, key TYPE fetched only for visible range (`:47-55`); level change clears filter, selection, preview
page, key types (`:149-160`); newer load supersedes older by sequence (`:101-108`); disconnected load unmarks
tab instead of error (`:125-128`); selection and filter are runtime-only, level path persists. Definition view
loads definition and describe independently; describe failure leaves structure body without columns, not error
(`definition/state.ts:59-90`); kafka, sqs, redis, s3 have no describe; source `cache` or `server` shown; FK rows
merged by name, inbound references listed separately (`definition/structure.ts:24-64`); Mongo validator shown
as field table when `$jsonSchema` has `properties`, else raw JSON (`:76-111`).

### J-BRW-010 S3: navigate prefixes at scale, awkward keys, partial permissions, actions

- Status: partial (ui browse S3; s3 catalog tests).
- Persona: developer cleaning upload bucket.
- Pre: bucket `up` with 250,000 objects under `a/`, prefixes 6 deep, keys: `a//b` (empty segment), `a/b/` (zero
  byte folder marker), ` leading space`, `x y+z%20.txt`, unicode NFC and NFD twins, `../etc`, 1,024-byte key,
  `.` and `..` segments; versioned with delete markers; second bucket where `ListBucket` denied but `GetObject`
  allowed; third bucket in other region.
- Steps:
  1. Open bucket browse tab; root lists prefixes and objects; sizes and dates.
  2. Descend `a/`: listing truncated strip appears; type filter text; Refresh.
  3. Descend by keyboard (Enter), ascend (Backspace or Left), breadcrumb jump 4 levels up.
  4. Click three prefixes quickly in a row.
  5. Select object: preview pane shows value page; change preview page size; select other object.
  6. Open each awkward key; copy key; copy `s3://` URI.
  7. Download `x y+z%20.txt`; delete NFD twin only.
  8. Upload file into current prefix; row appears after reload.
  9. Delete zero-byte folder marker `a/b/`; children remain.
  10. Open delete-marked object (hidden) through versions if offered; else absent.
  11. Open bucket with denied list: error state; open known key by path if offered.
  12. Other-region bucket: redirect handled or clear message.
  13. Connection drops; click prefix; reconnect gate; continue.
  14. Restart app: tab reopens at same level.
- Oracle (aws-cli): 2 truncation stated with count shown, filter applies to loaded nodes only and says so; 4
  final listing is of last clicked prefix (supersession), never mixed; level change clears filter and selection;
  6 clipboard key byte-equal (no URL-decoding, no normalisation); 7 downloaded file sha256 equal; NFC twin still
  present; 9 only marker deleted (exact key with trailing slash); 11 error names action, other buckets usable;
  14 level path restored, selection not.
- Risk: key normalisation deleting wrong object; stale listing after fast clicks.

### J-BRW-011 Redis: namespace tree over large keyspace with live changes

- Status: partial (ui browse redis).
- Pre: DB with 1,000,000 keys in namespaces `app:sess:*`, `app:cart:*`, keys without separator, key equal to a
  namespace name (`app:sess` as key and as prefix), keys expiring every 100 ms (fixture), ACL-limited user
  variant.
- Steps:
  1. Open database browse tab: namespaces as containers; counts if shown.
  2. Descend `app` then `sess`: up to 200,000 rows; truncated strip.
  3. Scroll fast through 50,000 rows: type icons fill for visible range only.
  4. Filter substring; select key; preview pane pages hash fields forward.
  5. Key expires while selected: preview shows gone state.
  6. Delete selected key; delete key that is also namespace name; delete namespace container if offered.
  7. Second client adds 10,000 keys; Refresh.
  8. Open leaf in full key/value tab from row; edit there; return: browse shows stale strip.
  9. Arrow keys, Home/End, type-ahead; Enter descends; context menu key.
  10. Switch DB tab; back.
  11. As limited user: SCAN allowed, TYPE denied.
- Oracle (redis-cli `MONITOR` in fixture): TYPE commands issued only for visible paths, count bounded; no `KEYS`
  command ever; 6 exact key deleted, sibling namespace keys intact; namespace delete absent or confirmed with
  count; 5 "key no longer exists" not crash; 11 rows shown without type, action error in strip.
- Risk: namespace/key ambiguity; unbounded TYPE calls.

### J-TREE-010 Project tree across engines: reconnect, limited catalogs, concurrent DDL

- Status: partial (ui tree specs; flows tree cache).
- Pre: connections: Postgres (300 schemas, 20,000 tables), MySQL, SQLite, ClickHouse, Mongo, Redis, Kafka, SQS,
  S3, Kibana; Postgres user seeing only 2 schemas.
- Steps:
  1. Expand each connection once: node kinds per engine as designed (schemas, tables, views, collections,
     databases, topics, queues, buckets, data views).
  2. Filter tree text `ord`; exclusions applied; clear.
  3. Expand Postgres schema with 20,000 tables: virtualized; type-ahead.
  4. Second client creates table, renames another, drops third; refresh node.
  5. Open tabs for renamed and dropped tables from before refresh.
  6. Disconnect connection with 5 tabs open; tree shows cached nodes dimmed; open cached node.
  7. Reconnect: tabs rehydrate lazily (only visible one loads).
  8. Limited user: tree shows 2 schemas; no error for others.
  9. Context menu per node kind by keyboard: open data, definition, console, copy name, copy qualified name.
  10. Drag connection to reorder; collapse all; restart.
- Oracle: request log: 7 exactly one load for visible tab; clipboard qualified name quoted per dialect
  (`"Order Items"`, backticks on MySQL); dropped table tab shows engine not-found with close hint, renamed tab
  same (no silent retarget); expansion state and order persisted.
- Risk: thundering reload on reconnect.

### J-DEF-010 Definition as source of truth: copy DDL and rebuild elsewhere, per engine

- Status: partial (definition adapter tests; ui definition specs).
- Persona: developer cloning schema to scratch database.
- Pre: Postgres table with identity, defaults with functions, generated column, composite PK, 2 FKs (one
  inbound), partial and expression indexes, check constraints, comments, partitioned parent; MySQL table with
  charset, collation, fulltext index, `ON UPDATE CURRENT_TIMESTAMP`; SQLite table with `STRICT`, triggers;
  ClickHouse `ReplacingMergeTree` with `ORDER BY`, `TTL`, codecs; Mongo collection with `$jsonSchema`
  validator and non-schema validator, 4 indexes (TTL, partial, text, compound); Kafka topic (partitions, configs);
  SQS queue (attributes, redrive).
- Steps:
  1. Open Postgres definition: sections Columns, Constraints, Indexes, Properties, DDL; source `server`.
  2. Filter structure `email`; find bar inside DDL; switch tab away and back: both kept.
  3. Click FK row: referenced table opens; inbound reference row opens referencing table.
  4. Copy DDL; run on empty scratch database through second client.
  5. Copy column list; copy single index definition.
  6. Repeat 1 and 4 for MySQL, SQLite, ClickHouse.
  7. Mongo: Validation section table for `$jsonSchema`; raw JSON for other validator; Indexes with options; copy.
  8. Kafka and SQS: properties only; no Columns section; copy config.
  9. Second client alters table; definition shows source `cache` until Refresh; stale strip after reconnect.
  10. Revoke catalog permission for describe only: definition still renders, structure columns empty with note.
  11. Drop object; Refresh.
  12. Restart app.
- Oracle: scratch database after 4: schema dump (`pg_dump -s`, `SHOW CREATE TABLE`, `.schema`, `SHOW CREATE`)
  equal to source for that object modulo owner (definition of faithful DDL; differences listed are defects or
  documented gaps: OQ-DEF-1); FK appears once in Constraints; clipboard index text executable; Mongo index
  options (expireAfterSeconds, partialFilterExpression) present in copy; 10 no blank tab; 11 engine not-found
  shown; 12 tab restored, loads from cache then server per design.
- Risk: DDL missing constraint or index silently.
