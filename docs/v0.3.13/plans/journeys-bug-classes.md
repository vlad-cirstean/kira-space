# P627 journeys: recurring bug classes, second pass per module

Extra journeys aimed at classes the history shows recurring (`inventory.md` "Bug classes"): clipboard and selection,
pagination, sort/filter/search, reconnect, large data, undo, persistence, concurrency, special characters. Compact
form; same fields and conventions as `journeys-studio-db.md`. All `new` unless a test is cited. Persona for all:
daily user doing ordinary work quickly, not a tester.

## A. Clipboard and selection

### J-BC-001 Console result: select rows after find-filter, copy, paste into grid of other table

- Pre: console result 500 rows; other table open with same columns.
- Steps: 1. Find `2024`, toggle filter-rows. 2. Gutter-select 3 visible rows (page rows 12, 80, 311). 3. Cmd+C.
  4. In data tab select row 1 gutter, Cmd+V, commit.
- Oracle: clipboard has exactly rows 12, 80, 311 in order; second client: target rows equal them; gutter numbers
  stayed real page rows.
- Variations: range selection spanning hidden rows; column selection under filter.
- Risk: display position vs page row mix-up.

### J-BC-002 Copy after tab switch restores selection

- Steps: 1. Select range in console result. 2. Switch to other tab and back (view state restore). 3. Cmd+C without
  clicking. 4. Rerun query returning fewer rows; Cmd+C.
- Oracle: step 3 clipboard equals highlighted range; step 4 stale selection that no longer fits is dropped (no
  copy of ghost cells, clipboard unchanged or empty-range notice).
- Risk: restored highlight differs from copied selection.

### J-BC-003 Native copy inside text controls is never hijacked

- Steps: in each: filter toolbar input, find bar, cell inline editor, Monaco console, HTTP URL field, notes editor,
  terminal: select part of text, Cmd+C, Cmd+X, Cmd+V, Cmd+A.
- Oracle: clipboard equals selected control text, never grid selection behind it; Cmd+A selects within control.
- Risk: grid shortcut steals native copy.

### J-BC-004 Clipboard permission denied and empty clipboard

- Steps: 1. Deny clipboard-read. 2. Cmd+V in grid. 3. Allow, empty clipboard, Cmd+V. 4. Clipboard holds image only.
- Oracle: clear toast per case; nothing staged; no unhandled rejection in console errors.
- Risk: silent failure, stray empty insert row.

### J-BC-005 Right-click decides target by what is shown selected

- Steps: for data grid and console grid: 1. Multi-row select, right-click inside (row menu, selection kept).
  2. Right-click outside selection (selection moves to hit). 3. Right-click header (column menu, selection
  unchanged). 4. Each menu Copy.
- Oracle: clipboard equals menu's stated target each time.
- Risk: P335 class.

### J-BC-006 Select-all then extend then copy on 10,000-row page

- Steps: 1. Page size 10000. 2. Corner select all. 3. Cmd+C. 4. Shift+Up. 5. Cmd+C.
- Oracle: first clipboard 10,000 lines within 1 s (assumption) and equal to second-client page; second has 9,999;
  UI not frozen.
- Risk: spread into `push` overflow past 65k cells; anchor after select-all.

### J-BC-007 Copy from stale (disconnected) grid and from masked preview

- Steps: 1. Drop connection. 2. Copy rows. 3. Reconnect, enable mask preview. 4. Copy same rows as CSV and INSERT.
- Oracle: stale copy equals fetched values; masked copy holds masked values in all formats; Copy as INSERT under
  mask: assumption disabled or masked, never real values.
- Risk: INSERT path reads raw page.

### J-BC-008 Tree multi-select copy in Api and Studio trees

- Steps: 1. Api tree select 3 requests, Cmd+C (assumption: copies names or does nothing; pin). 2. Studio tree
  Cmd+C on schema, on connection. 3. Shift+Alt+C on object with `.` and `"` in name.
- Oracle: clipboard text defined per node kind; path quoting unambiguous.
- Risk: undefined copy writes stale clipboard.

### J-BC-009 Git: copy sha, message, branch, file path, patch

- Steps: commit menu Copy sha (full), Copy message (subject plus body), branch chip copy, file tree Copy path
  (relative and absolute), diff selection copy, blame tooltip copy.
- Oracle: sha equals `git rev-parse`; message equals `git log -1 --format=%B` exact incl trailing lines policy
  (pin); paths correct for unicode NFC/NFD names.
- Risk: abbreviated sha, normalized path mismatch.

### J-BC-010 ADE: copy session log while session still writes

- Steps: 1. Session printing continuously. 2. Copy log. 3. Again 2 s later.
- Oracle: each clipboard is a prefix-consistent snapshot (second starts with first), no torn chunk, no duplicate
  chunk at page seam (500-chunk paging).
- Risk: paging seam duplicates.

### J-BC-011 Operations: Copy details of op with huge and binary output

- Steps: op with 5 MB error text and NUL bytes; Copy details; Copy error; Copy command with quotes.
- Oracle: clipboard bounded with truncation note; command shell-safe; secrets redacted.
- Risk: clipboard write failure on huge text.

### J-BC-012 Proof and host view: selection inside read-only renders

- Steps: select text across Markdown blocks in PR description, issue callout, proof explanation; Cmd+C.
- Oracle: clipboard text non-empty and equals rendered text (body `select-none` must not block).
- Risk: not selectable (P577, P582 class).

## B. Pagination and paging

### J-BC-020 Page controls under rapid input

- Steps: grid: press Next 5 times in 300 ms, then Prev 2, during slow loads; then change page size mid-flight.
- Oracle: final page index = start +3 at new size mapping (pin rule: page resets to first on size change,
  assumption); server saw no more page queries than needed; rows match second client.
- Risk: double advance.

### J-BC-021 Paging with concurrent deletes and inserts by another client

- Steps: keyset table; page 1; second client deletes rows of page 2 and inserts new rows sorted into page 1;
  Next; Prev.
- Oracle: no row shown twice on consecutive pages; Prev returns consistent window; count refresh on demand.
- Risk: keyset anchor on deleted row.

### J-BC-022 Redis and S3 cursors across refresh, filter, tab switch

- Steps: hash 5,000 fields: scroll to 3,000; type filter; clear; switch tab; back; Refresh.
- Oracle: every field reachable exactly once after Refresh; scroll position restored on tab switch (assumption);
  filter does not reset server cursor into duplicates.
- Risk: cursor reset duplicates.

### J-BC-023 Git Load more at exact multiples and after refs change

- Steps: history of exactly 2 pages; Load more twice; new commit arrives; refresh; Load more.
- Oracle: never "Load the last 0"; total rows equal `git rev-list --count`; refresh keeps loaded depth.
- Risk: off-by-one at page multiple.

### J-BC-024 Kubernetes list truncation and log tail paging

- Steps: 5,000 pods (list truncates with notice); filter narrows server-side or client-side (pin); logs tail 2,000
  then scroll up to request older (if offered).
- Oracle: truncation notice count equals fake's total; no silent cut.
- Risk: silent truncation.

### J-BC-025 Host comments and checks past caps

- Steps: PR with 400 comments, 150 checks; issue with 200 comments.
- Oracle: newest N shown with note naming cap; order chronological; reply targets right thread id.
- Risk: reply to wrong thread at cap edge.

### J-BC-026 Runs log paging and tail

- Steps: run with 100k lines; open finished run; scroll to top; Copy log; Operations tail shows last 40 lines.
- Oracle: first line and last line equal file; tail equals `tail -n 40`.
- Risk: tail off by one, copy partial.

### J-BC-027 Archived finder and board history with 2,000 archived tasks

- Steps: type in finder; scroll; open oldest; restore.
- Oracle: result set equals DB filter; UI responsive; open item (history rides wire) measured, pinned.
- Risk: board slow or truncated silently.

## C. Sort, filter, search

### J-BC-030 Multi-sort then filter then page then export

- Steps: sort by `score DESC`, `name ASC`; filter `score IS NOT NULL`; page 3; copy page; export CSV.
- Oracle: SQL sent holds ORDER BY both in badge order; page rows equal second client; ties stable by PK.
- Risk: sort order lost on filter change.

### J-BC-031 Sort on NULLs, mixed case, unicode, numeric-as-text

- Steps: sort column with NULLs, `a`, `B`, `ä`, `10`, `9` on Postgres, MySQL, SQLite.
- Oracle: order equals engine order (server-side sort), not client locale sort; console result client sort (if
  offered) labelled as such.
- Risk: client re-sort contradicting engine.

### J-BC-032 Filter by value from cell menu with hostile value

- Steps: cell value `it's 100% "x"\y`, NULL, empty, date, uuid, jsonb; menu Filter by value on each engine.
- Oracle: filter returns that row (NULL uses IS NULL); dialect quoting right.
- Risk: dialect quoting.

### J-BC-033 Find bars keep state per tab across module switch

- Steps: open find in grid tab (regex on), HTTP response (case on), repo file, review diff; switch modules; back.
- Oracle: each tab's query/toggles/current match restored; closing tab clears.
- Risk: find state lost (class 1, 13).

### J-BC-034 Tree and list filters with special characters

- Steps: filter boxes (project tree, collections, runs, clusters, containers, sessions menu, branch picker, history)
  with `[`, `*`, `\`, emoji, leading space.
- Oracle: literal substring match (no regex errors), case rule consistent across lists.
- Risk: regex injection in filter.

### J-BC-035 Branch picker ranking with 5,000 branches

- Steps: type `main`; exact match first; cap 50 with Show more; disabled reasons; keyboard pick 51st after Show
  more.
- Oracle: picked ref equals highlighted; ranking deterministic.
- Risk: index shift after Show more.

### J-BC-036 KQL and SQL autocomplete insert position

- Steps: accept completion mid-token, inside quotes, after dot, with multi-cursor (Monaco), in filter toolbar.
- Oracle: resulting text equals intended; no duplicated prefix.
- Risk: replace range wrong.

## D. Reconnect and drop

### J-BC-040 Laptop sleep and wake with everything connected

- Steps: connected: Postgres, Redis, Kafka follow, kube log follow, docker logs, gRPC stream, mobile server,
  git auto-fetch; suspend 10 min (drop all sockets); wake.
- Oracle: each surface shows stale/ended state with one-click resume; none spins forever; no duplicate streams
  after resume; no auto-rerun of statements.
- Risk: zombie streams, silent stale data.

### J-BC-041 Reconnect to changed server identity

- Steps: connection host now points to other DB (different schema); Reconnect with open tabs and staged edits.
- Oracle: tabs reload or error clearly; staged edits not committed against wrong DB without user seeing change
  (assumption: commit re-reads row by PK and fails on mismatch or warns; pin).
- Risk: commit into wrong database.

### J-BC-042 Bounded reconnect and in-flight cap

- Steps: fire 70 operations on one connection (cap 64) then drop link.
- Oracle: extra ops refused with clear code; all in-flight end with error; reconnect attempts bounded; UI idle
  afterwards.
- Risk: unbounded retries.

### J-BC-043 Pre-connect plus reconnect

- Steps: connection with tunnel script; tunnel dies; Reconnect.
- Oracle: pre-connect runs again once; old script process gone; Ops shows both.
- Risk: leaked tunnel processes.

### J-BC-044 MCP session server across app restart and step restart

- Steps: step with `db_mcp` running; restart session; restart app; resume run.
- Oracle: new per-launch token; old token dead (401); audit continues under same task.
- Risk: stale token accepted.

### J-BC-045 Git bound service after window close and reopen

- Steps: start long graph stream in window 2; close window; reopen; open same repo.
- Oracle: old Conn released (no goroutine leak, stream ended); new stream full.
- Risk: Conn leak (P477).

## E. Large and messy data

### J-BC-050 Wide and long values in grid

- Steps: table 600 columns; row with 10 MB text, 1 MB json, 64 KiB+1 boundary value; scroll horizontally; open
  cell editor; copy cell.
- Oracle: truncation flag exactly at cap boundary on UTF-8 char boundary; editor loads full value on demand or
  states truncated; no mid-codepoint cut in clipboard.
- Risk: Latin-1/UTF-8 truncate boundary.

### J-BC-051 Identifiers from hell

- Steps: schema/table/column named with space, `"`, backtick, `.`, emoji, reserved word, 63-byte name, mixed case,
  leading digit; open, filter, sort, edit, FK navigate, copy as INSERT, MCP browse.
- Oracle: every generated statement runs; second client sees edits.
- Risk: quoting gaps per dialect and path.

### J-BC-052 Duplicate names everywhere

- Steps: two connections same name; two requests same name in folder; two scripts same name; two tasks same title;
  two branches differing by case on case-insensitive FS; two environments same name.
- Oracle: each addressed by id (operations hit the clicked one); files get distinct stems; no overwrite.
- Risk: name-keyed lookup hits wrong entity.

### J-BC-053 Empty states

- Steps: empty DB, empty table, zero-column result (`SELECT` nothing on Postgres), empty repo (no commits), repo
  with only orphan branch, empty collection, empty board, no clusters, no containers.
- Oracle: each shows empty state, no error, actions that need content disabled with reason.
- Risk: crash on zero rows/cols.

### J-BC-054 Binary and non-UTF-8 text

- Steps: bytea/blob cells, Redis binary values, Kafka binary, HTTP binary body, git binary diff, file with invalid
  UTF-8 in workspace editor (Edit refused or bytes preserved), terminal output invalid bytes.
- Oracle: no replacement-char corruption written back anywhere; edit paths refuse or round-trip bytes.
- Risk: mojibake saved to disk/DB.

### J-BC-055 Very many of each list entity

- Steps: 500 connections, 2,000 saved queries, 5,000 collection items, 1,000 scripts, 300 repos, 100 windows tabs.
- Oracle: lists virtualized or responsive; reorder/drag still hits right item; persistence payload bounded.
- Risk: drag index mismatch in long lists.

## F. Undo, redo, revert

### J-BC-060 Monaco undo boundaries after programmatic edits

- Steps: console: format, autocomplete accept, paste, Run; request body: beautify; cell editor: generator insert;
  each followed by Cmd+Z x2, Cmd+Shift+Z.
- Oracle: one undo reverts one programmatic edit exactly; buffer never empties unexpectedly.
- Risk: P69 class.

### J-BC-061 Grid revert semantics

- Steps: edit cell twice; Revert cell; edit inserted row then delete it; delete existing row then undelete; Revert
  all with 3 kinds staged.
- Oracle: pending set empty after Revert all; SQL preview empty; rails cleared; commit sends nothing.
- Risk: Revert disabled after blur-stage (P327).

### J-BC-062 Git undo slot across operations and repos

- Steps: checkout (undo available), commit (slot replaced), switch repo (slot per repo), fetch (slot kept),
  failed op (slot kept).
- Oracle: Undo acts on last undoable op of current repo only; after undo slot empty.
- Risk: slot cleared before write, cross-repo undo.

### J-BC-063 Form Discard in config forms

- Steps: workflows, runs, repo config, connection file dialog, agent settings: edit, switch Form/YAML, Discard.
- Oracle: disk file byte-identical (mtime unchanged); form equals disk.
- Risk: Discard leaves YAML tab dirty.

## G. Persistence across restart

### J-BC-070 What survives restart, one checklist journey

- Steps: set and then restart: module per window, tabs, panel sizes, column widths (grid, graph), diff layout,
  page size (assumption: per tab), filters saved/recent, find state (not kept), result tabs (not kept), console
  text (kept per saved path), request drafts, active environment, cluster order, hidden repos, pins, settings,
  schedules, plan, panel tabs per task, notification prefs, hotkey, MCP token, paired devices.
- Oracle: each item compared to explicit expected (kept or reset) from inventory; silent items stated as
  assumptions and pinned.
- Risk: unintended reset or unintended resurrection.

### J-BC-071 Crash (SIGKILL) instead of quit

- Steps: SIGKILL during: tab save debounce, commit of grid changes, file save, config sync apply, migration, script
  run, pull.
- Oracle: DB commit atomic at server; file either old or new bytes; config sync recovered; `op_log` orphans become
  error; git state resumable with banner; no lock file blocks relaunch.
- Risk: torn writes.

## H. Concurrency

### J-BC-080 Same action double-fired

- Steps: double-click or Enter-repeat on: Commit, Send, Run, Save, Create task, Start step, Approve, Push, Post
  comment, Import, Pair.
- Oracle: far end sees one effect each.
- Risk: duplicate side effects.

### J-BC-081 Action during in-flight opposite action

- Steps: Stop while starting; Disconnect while connecting; Cancel while committing; Close tab while saving; Archive
  while agent starts; Restart session while Take over.
- Oracle: defined winner; end state consistent (not "connecting" forever); far end matches end state.
- Risk: race leaves limbo state.
