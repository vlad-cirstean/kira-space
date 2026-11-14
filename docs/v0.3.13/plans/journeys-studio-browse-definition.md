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
