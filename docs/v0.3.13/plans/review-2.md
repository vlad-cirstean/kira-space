# v0.3.13 code review round 2 (P632)

Base: `f5a75a5d0` (same base as round 1). Diff: `f5a75a5d0..HEAD`, covering streams A, B, C, full-run
fixes and round-1 fixer commits `f4f0a3561..fa2f6ac82` (reviewed hardest).

Counts: 1 high, 4 medium, 5 low.

Paths are under `apps/kira-space/` unless they start with `packages/` or `docs/`.

Real-DB probes: MySQL 8.4.11, MariaDB 11.4.13, Postgres, SQLite (in-process). Containers removed.

## High

### H1 Lost-ack reconcile settles against the pre-commit page when the reload read fails (CONFIRMED)

- Where: `frontend/src/studio/views/grid/pendingChanges.ts:530-556` (`reconcileAfterUnknownCommit`),
  `frontend/src/studio/views/grid/state.ts:206-237` (`reloadAfterMutation`),
  `frontend/src/studio/views/shared/page/load.ts:47-51` (`runPagedLoad` catch).
- `reconcile.reload()` is `reloadAfterMutation`, which calls `data.invalidate` then `load()`. `load()` goes
  through `runPagedLoad`, whose catch calls `applyLoadFailure` and never rethrows. `applyLoadFailure`
  (`views/shared/viewOp.ts:130`) sets `rt.error` but keeps the old page. So a failed read resolves
  `reload()` normally, and `getPage(tabId)` returns the page from before the commit.
- Scenario: stage an insert into a serial-key table, commit lands, ack lost, read after invalidate fails
  (connection drop, timeout). `settleOnPage` finds no matching row on the stale page, so the insert lands in
  "not applied". `settleUnresolved` with `applied==0 && notApplied>0` keeps it silently and clears
  `unreconciledState`. Next Commit resends the insert: duplicate row. Staged updates likewise read stale
  server text and get re-based or kept wrongly.
- The comment at 527-529 and the UI test "Commit after a failed reload settles first"
  (`tests/ui/studio/grid-lost-ack.spec.ts:221`) cover only a failing `invalidate` (`unreachable` snapshot
  before `invalidated`), which does throw. No test fails the `read`.
- Fix: make the reload report failure. Either have `reloadAfterMutation` rethrow when `rt.error` is set for
  its own `opId`, or have reconcile compare the page identity/fetched marker before and after and treat
  "unchanged" as failed, keeping `unreconciledState`. Add a UI case whose stream fails the `read` after a
  good `invalidate`.

## Medium

### M1 Round-1 M2 not fixed for a tab closed mid-run: open transaction left on shared session (CONFIRMED, code)

- Where: `frontend/src/studio/views/console/state.ts:535` (success: `if (!runtime[tabId]) return;` before
  `trackTransaction` at 558), `:570` (catch: tracks only when `runtime[tabId]` exists), cleanup `:325-361`.
- Cleanup runs `takeOpenTransaction` before the in-flight run finishes. A run containing `BEGIN` that
  completes after close returns at 535 and never records the transaction; a failing run is skipped at 570.
  Adapters pin one connection per (connection, database) (`ConnSet.Acquire`) shared with grid reads and
  mutate, so the open or aborted transaction stays on that session. Postgres: every later grid read errors
  "current transaction is aborted" or sees uncommitted data; grid commits nest inside the stray transaction
  and vanish on disconnect.
- Fix: track the outcome even when the tab is gone and send `ROLLBACK` if it leaves a transaction open, or
  on close await the cancelled run and then roll back whenever its text could open one. Add a UI spec that
  closes the tab during a `BEGIN; ...` run.

### M2 Failed DDL with a syntax error treated as an implicit commit (CONFIRMED on MySQL 8.4 and MariaDB 11.4)

- Where: `frontend/src/studio/views/console/transaction.ts:101-104`;
  `tests/unit/studio/console-transaction.spec.ts` ("a failing DDL statement still commits before it runs").
- Real servers: `BEGIN; INSERT ...; CREATE TABL x` keeps the transaction open (insert visible in session,
  rolled back by `ROLLBACK`). Parse errors (1064) never reach the commit step. Only parse-valid DDL that fails
  later (existing table, missing table) commits. The tracker drops the transaction, so closing the tab skips
  the prompt and the rollback, leaving the insert uncommitted on the shared session (see M1 consequences).
- The unit spec pins the wrong rule with `CREATE TABLE broken (`, itself a parse error. Flow test
  `internal/flows/dbflow/console_mysql_test.go` `TestConsoleMySQLTransactionEnds` has no failing-DDL case.
- Fix: treat `ER_PARSE_ERROR` (1064, carried in `details` or code) as no commit, or read server state (see
  L2). Add a flow case for both parse-error and runtime-error DDL and correct the unit spec.

### M3 `autocommit=0` not modelled (CONFIRMED on MySQL 8.4 and MariaDB 11.4)

- Where: `frontend/src/studio/views/console/transaction.ts:52-70` (`transactionEffect`).
- `SET autocommit=0` then `COMMIT` then `INSERT`: server holds an open transaction (`innodb_trx` count 1,
  `ROLLBACK` removes the row). Tracker sees `COMMIT` close and nothing reopen. Closing the tab skips the
  prompt. Even when a transaction is tracked, the closing `ROLLBACK` leaves `autocommit=0` on the pinned
  session, so later grid commits on that session never commit until disconnect.
- Fix: model session autocommit state (or query `@@autocommit` / `@@in_transaction`), and reset
  `autocommit=1` after the close rollback.

### M4 `#` comments and nested Postgres block comments not stripped (CONFIRMED, code)

- Where: `frontend/src/studio/views/console/transaction.ts:28-39` (`stripLeadingComments`).
- Handles `--` and one `/* ... */` up to the first `*/`. MySQL `# note\nBEGIN` and Postgres
  `/* a /* b */ c */ BEGIN` are not recognised, so the transaction is never tracked and close skips prompt
  and rollback. Same gap hides implicit-commit DDL behind a `#` comment.
- Fix: strip `#` line comments for the MySQL family and nest block comments for Postgres; add unit cases.

## Low

### L1 Lexer gaps beyond M2-M4 (CONFIRMED for CHECK TABLE and RESET on MySQL; rest PLAUSIBLE)

- `transaction.ts:43` `IMPLICIT_COMMIT` misses `CHECK TABLE` (confirmed commits), `RESET`, `START REPLICA`,
  `CREATE/ALTER/DROP USER` variants via other verbs, `XA` statements and `CALL` of a procedure that commits.
- Failed `COMMIT`/`ROLLBACK` still close per `transactionEffect`; SQLite `SAVEPOINT` outside a transaction
  opens one and `RELEASE` closes it, both untracked.
- `USE other_db` moves later statements to another session key; the entry for the old key stays orphaned
  until disconnect.
- Fix: extend the list or, better, take L2.

### L2 Server transaction state exists but lexing is used everywhere (architecture)

- Postgres console already reads `conn.PgConn().TxStatus()` (`internal/adapters/postgres/console.go:376`).
  MariaDB has `@@in_transaction`; MySQL can enable `session_track_transaction_info`. Returning an
  `inTransaction` flag on the console result would replace M2-M4 and L1 for those engines; lexing stays only
  as SQLite fallback (`sqlite3_get_autocommit` is also available).

### L3 Forced-overwrite resend lacks lost-ack handling (PLAUSIBLE)

- Where: `frontend/src/studio/views/grid/pendingChanges.ts:610`.
- After the user confirms overwrite, the second `data.mutate` sits outside the try. An `outcomeUnknown`
  error there skips `failAfterUnknownOutcome`, so the staged set is not reconciled and the next Commit
  resends a plan that may have landed (duplicate insert on a serial key).
- Fix: route the forced resend through the same outcome-unknown branch.

### L4 Reconcile silently re-bases guards past a concurrent change (PLAUSIBLE)

- Where: `frontend/src/studio/views/grid/reconcile.ts:58-66`.
- For a "not applied" update, settle re-bases `expect` to the reloaded server text. If another writer changed
  the row in the window, the next Commit overwrites that change without the decision-3 confirm.
- Fix: re-base only when reloaded text equals the original `expect`; otherwise keep the old guard so the
  conflict prompt fires.

### L5 TypeRoundTrip stale check covers only the edit column (PLAUSIBLE)

- Where: `internal/adapters/testsupport/typeround.go:~60-75`.
- Stale-guard check asserts `E_CONFLICT` only for `EditColumn`. Guards on other column types (MySQL FLOAT
  dropped, DOUBLE, temporal) are never proven to detect change; a guard that always matches would pass.
- Fix: loop the stale check over every guarded type in the round-trip table.

## Round-1 findings status

- H1 lost ack: built (reload, settle, ask), but see H1 and L3.
- H2 Postgres guard on xml/point/etc: fixed (`::text` for non-native-equality types), probe clean.
- M2 console transaction on tab close: fixed for the common path; tab-closed-mid-run path still open (M1).
- Remaining round-1 items (error `details` frame, MySQL FLOAT guard, expect-name validation, concurrent-edit
  prompts, workflow count recheck, git-ui carry retry): fixed, no regression found.
- `details` field: encode/decode symmetric (`adapterhost` `encodeError`, `port.ts` `parseDetails`),
  `AtStatement` and `CarryDetails` preserve it through timeouts; `scripts/generate-wire.sh` shows no drift.

## Decisions check

- 1, 3, 4, 5, 6, 7, 9, 10, 11: enforced in code and covered by flow plus UI tests.
- 2 (lost ack): enforced except H1 and L3.
- 8 (close prompts and rolls back): grid side enforced; console side gaps M1-M4.

## Dimensions

- Architecture/security: nothing real beyond L2. No new credential handling, no injection path in the new
  `::text` casts (identifiers quoted).
- Correctness: H1, M1-M4, L1, L3-L5.
- Performance: nothing real. Reconcile reload is one page read; `::text` casts apply only to guard columns.

## Tests

- No fixed sleeps found in the diff's new tests. No hand-edited contracts; wire code regenerates identically.
- Weak spots: H1 (no failed-read case), M2 (unit spec pins wrong rule), L5.
