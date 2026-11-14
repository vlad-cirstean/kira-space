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
