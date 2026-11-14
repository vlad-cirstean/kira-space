# P627 journeys: Studio documents view (MongoDB)

Split from `journeys-studio-db.md` and `journeys-studio-views.md` (P627 first pass); J-ids unchanged.
Imagined usage and definition of correct. No implementation, no test code.
Conventions: `journeys-studio-connections.md` head.

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

## Second pass: document view sessions (MongoDB)

Runs in seconds: flow half against Mongo container (replica set for step-down case) with mongosh as second
client; UI half on mocked bridge.

Facts read from code (CodeGraph): document mutations apply at once, no staged plan, no preview
(`documents/mutations.ts:6`); edit is whole-document `replaceOne` keyed by `_id`, with `_id` forced back to key
value (`adapters/mongo/mutate.go:80-109`); matched count other than 1 is error "expected update to affect exactly
one document" (`:104-107`), same for delete (`:127-130`); key must be exactly `{_id}`, any EJSON value
(`:22-38`); insert without `_id` gets server ObjectId; non-relational mutate loops ops one by one with no
transaction (`adapters/rowops.go:16-32`).

### J-DOC-010 Catalogue clean-up: filter, project, edit rich types, insert, delete, copy

- Status: partial (ui documents edit; flows mongo mutate; chain new).
- Persona: developer repairing product documents.
- Pre: `shop.products` 12,000 docs; `_id` kinds: ObjectId, string, int, compound document `{sku, region}`,
  one `null`; fields Date, Decimal128, Long, Binary UUID, regex, nested 6 levels, array of 2,000; unique index
  on `sku`; JSON-schema validator requiring `price`.
- Steps:
  1. Open collection; page size 50; filter row `{category: "tools", price: {$gt: 10}}`; count shown.
  2. Projection menu: only `sku`, `price`, `stock`; sort `price` desc; next page twice; back.
  3. Open document editor on doc with ObjectId; change `price` Decimal128; Save.
  4. Edit doc with compound `_id`; change nested field; Save.
  5. Edit doc with `_id: null`; Save.
  6. In editor change `_id` itself to other value; Save.
  7. Remove `price` (validator fails); read error; restore; Save.
  8. Set `sku` equal to another doc (unique index); Save.
  9. New document without `_id`; Save. New document with existing `_id`.
  10. Delete 2 documents with row action; one already deleted by mongosh.
  11. Second client updates field `stock` of doc open in editor; user saves own change to `price`.
  12. Find in page `hammer`; filter-rows on; Copy all as canonical; copy one doc as shell.
  13. Paste copied shell document into New document; Save.
  14. Restart app.
- Oracle (mongosh): 3 only `price` differs, type still Decimal128, key order kept; 4, 5 exactly one doc replaced;
  6 `_id` unchanged (forced), user told `_id` cannot change (not silently dropped: OQ-DOC-2); 7, 8 engine error
  text shown, document unchanged, editor keeps user text; 9 ObjectId assigned and shown after reload; duplicate
  key error; 10 second delete reports "deleted 0" error, list reloads without it; 11 whole-document replace
  overwrites `stock` back to stale value today: recommended correct (OQ-DOC-1): stale strip blocks or warns
  before save when server document changed since load; 12 clipboard documents BSON-equal to server documents
  under projection in force (projection applies or not: pin, OQ-DOC-3); 13 inserted doc equal except `_id`; 14
  filter, projection, sort, page size restored.
- Risk: lost update through replaceOne; type downgrade on save (Long to double, Date to string).

### J-DOC-011 Large and awkward documents

- Status: new.
- Pre: docs of 15.9 MiB, 10,000 top-level keys, depth 100, key names `a.b`, `$x`, empty string, unicode,
  duplicate-looking keys differing by case; string with 1 MiB single line; NaN, Infinity, -0, MinKey, MaxKey,
  Timestamp, DBRef, JS code.
- Steps:
  1. Page containing 15.9 MiB doc loads within byte budget; row shows truncated marker.
  2. Expand tree to depth 100 with keyboard; collapse all.
  3. Open editor on truncated doc.
  4. Edit small field in it; Save.
  5. Copy all on page as relaxed and plain.
  6. Sort by field missing in half of docs; filter `{"a.b": 1}` and `{$expr: ...}`.
  7. Insert document over 16 MiB.
  8. Edit doc holding NaN, -0, MinKey: change other field; Save.
- Oracle: 3 editor loads full body or refuses edit; never saves truncated body (mongosh `bsonsize` before and
  after differs only by edit); 5 array valid even with truncated member (`copyAll.ts` keeps raw text) and
  truncation stated; 7 engine size error; 8 special values intact (mongosh type checks).
- Risk: saving truncated body over real document.

### J-DOC-012 Read-only, permissions, primary step-down, cancel

- Status: partial (complete suite permission matrix).
- Pre: replica set; users: `reader` (read only), `writer` without `remove`; connection flagged read-only.
- Steps:
  1. Read-only connection: editor opens read-only; row actions absent; New disabled.
  2. `reader` on writable-flag connection: Save edit.
  3. `writer`: edit ok; delete denied.
  4. Start save; primary steps down (fixture `rs.stepDown`) before reply.
  5. Retry save after election.
  6. Open collection with slow filter (`$where` sleep); Stop.
  7. Network drop; next page; reconnect; state kept (filter, projection, expanded rows).
  8. Collection dropped by second client; Refresh; tab state.
  9. Count on 50M collection: estimate vs exact; cancel exact.
- Oracle: 1 no write command reaches server (profiler); 2, 3 unauthorized text with action name, document
  intact; 4 outcome failed or unknown, never success without ack; after 5 exactly one replace applied (document
  version field incremented once); 6 `currentOp` clean; 8 empty state with message, no stale rows editable.
- Risk: retry after ambiguous write applying twice (harmless for replace, harmful for insert: verify insert
  variant gives one document, not two).

### J-DOC-013 Keyboard-only and search session with view state persistence

- Status: partial (ui documents search specs).
- Steps:
  1. Focus list; arrows move rows; Right expands, Left collapses; Enter opens editor.
  2. In editor: format, fold, find/replace, Mod+S saves, Escape closes with dirty prompt.
  3. Mod+F find across page; next/previous; match count; filter-rows toggle.
  4. Filter row history: recall 3rd entry with keyboard; edit; run; bad JSON shows inline error, old results stay.
  5. Projection menu by keyboard; sort by nested field path.
  6. Row tip (hover/focus) shows `_id` and size; copy `_id`.
  7. Next page while a row is expanded; back: expansion restored or reset (pin).
  8. Switch tab away and back; split window; restart.
- Oracle: mongosh sees edits of 2; clipboard `_id` in shell form evaluates to same value; filter history in app
  DB capped and de-duplicated; focus never lost to body; state after restart equal.
- Risk: find matches desynchronised from rendered rows after reload.
