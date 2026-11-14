# v0.3.13 code review round 1 (P632)

Base: `f5a75a5d0` (v0.3.12 squash merge on `origin/main`, holds last review's fixes `d7d0bf5db`). Local
`git merge-base HEAD main` (`a025b8b9e`) is stale and was not used. Diff: `f5a75a5d0..3a28c77fa`, streams A,
B, C and full-run fixes, product code and tests.

Counts: 2 high, 6 medium, 6 low.

Paths below are under `apps/kira-space/` unless they start with `packages/`.

## High

### H1 Decision 2 (lost ack) only half built: staged set never reconciled with server (CONFIRMED)

- Where: `frontend/src/studio/views/grid/pendingChanges.ts:508-540` (`commitPending`),
  `frontend/src/studio/views/grid/DataView.vue:127` (`onCommit`).
- Decision 2: "keep staged edits for rows that still differ from server; clear matching ones". Code keeps the
  whole staged set on any error and only shows `commitOutcomeUnknown`. No reload, no compare. Stream A's
  Result restates the decision as "lost ack keeps staged edits": silent narrowing.
- Scenario: user inserts a row into a table with a serial or no primary key, commit applies but ack is lost.
  User presses Commit again. Insert runs twice (duplicate row). Staged updates now carry stale `expect`, so
  each raises a false "row changed on server" prompt; confirming overwrites.
- `TestGridLostAck` (`internal/flows/dbflow/grid_faults_test.go:21`) proves no double insert only because
  the test uses an explicit integer key that refuses the retry. `tests/ui/studio/grid-lost-ack.spec.ts`
  asserts message plus "edit stays staged" only.
- Fix: after `commitOutcomeUnknown`, reload the page and drop each staged edit whose server value already
  equals the staged value (inserts: match by key if known, else ask). Add flow plus UI case on a serial-key
  table.

### H2 Postgres guard breaks grid delete and update on xml, point, polygon, path and similar columns (CONFIRMED)

- Where: `internal/adapters/postgres/read.go:28-49` (`typeClassFor` maps every unknown type to `text`);
  `frontend/src/studio/views/grid/pendingChanges.ts:43,286,424-430` (text class is guarded; `guardedRow`
  puts every guarded column of the row into a delete's `expect`);
  `internal/adapters/sqlmutate.go:124` (`whereFromKey` emits `col = $n`).
- Verified on Postgres 17: `... AND x = $1` with `x xml` fails `operator does not exist: xml = unknown`;
  same for `point`. Delete of any row in a table holding such a column now fails, where it worked before
  this chapter. Updating such a column fails the same way.
- Fix: guard only types with a known equality operator (adapter-side allow-list or a `guardable` flag on
  the column), or compare via `col::text = $n` for text-class fallbacks. Add an xml or point column to the
  Postgres type round-trip guard matrix.

## Medium

### M1 MySQL FLOAT guard never matches: false conflict every time (CONFIRMED)

- Where: `internal/adapters/mysqlfamily/read.go:35` (`float` in number class, so guarded);
  `internal/adapters/sqlmutate.go:124`.
- Verified on MySQL 8.4: FLOAT column holding 0.1 reads back as text `0.1`, but `WHERE f = '0.1'` and
  `WHERE f = 0.1` both match 0 rows. Every delete of a row with a FLOAT value, and every update of a FLOAT
  column, answers E_CONFLICT; user must confirm an overwrite each time, which trains them to ignore the
  prompt.
- `internal/adapters/mysqlfamily/typeroundtrip_test.go:43` guard list has no FLOAT (nor DOUBLE) column, so
  the matrix never saw it.
- Fix: leave single-precision FLOAT out of the guard, or compare `CAST(col AS CHAR) = ?`. Add FLOAT to the
  guard matrix.

### M2 Decision 8 console: transaction tracked only after a successful run (CONFIRMED)

- Where: `frontend/src/studio/views/console/state.ts:558` (`trackTransaction` in the success branch only;
  also skipped when the tab closed mid-run), `frontend/src/studio/views/console/transaction.ts:40`.
- Scenarios:
  - `BEGIN; <failing statement>` leaves a Postgres transaction open (aborted) on the connection. Tab closes
    with no prompt, no rollback.
  - Transaction lives on the connection, not the tab (`TestConsolePostgresTransactionAndCap`,
    `internal/flows/dbflow/console_script_test.go:141`). Two console tabs on one connection: closing tab A,
    which ran `BEGIN`, rolls back work tab B did; closing tab B prompts nothing.
  - MySQL DDL implicit commit and `COMMIT AND CHAIN` are not modelled: stale prompt or missed one.
  - `open` map is not cleared on disconnect.
- Fix: track on failed runs too (statements up to the failure ran), key the open state by connection, clear
  it on disconnect. Better: ask the adapter for real transaction status (`pg` `TxStatus`, MySQL
  `@@in_transaction`) after each execute instead of lexing statements.

### M3 Saved HTTP request guard ignores item name: concurrent rename silently lost (CONFIRMED)

- Where: `internal/apifiles/edit.go:99-125` (`requestDiffers` compares the `http` node only);
  `internal/apifiles/store.go:593-610` (`SaveRequest` writes `name` too).
- Scenario (journey J-COLL-002, "never silent loss of rename"): window 2 renames request R and saves.
  Window 1, holding an unsaved body edit read before the rename, saves. `expect` matches the http node, so
  no conflict; `replaceRequest` writes window 1's old name. Rename gone, no prompt.
- Fix: include the item name in the compared state (send `expectName`, or compare name plus http).

### M4 Decision 3 narrowed: gRPC saved requests have no concurrent-edit guard (CONFIRMED absence)

- Where: `internal/bridge/collections.go:138-159` (`CollectionsSaveGrpcRequestArgs` has no `Expect`),
  `internal/apifiles/store.go:613-619`.
- Decision 3 names "API request two windows". HTTP got base compare; gRPC items in the same collection files
  still last-write-wins without warning. Neither stream plan nor Result records this as out of scope.
- Fix: mirror the HTTP path (`Expect *model.SavedGrpcRequest`, `grpcDiffers`), plus flow and UI halves.

### M5 Stash carry flow now fails where it used to succeed (CONFIRMED)

- Where: `packages/git-ui/src/state/ops.ts:1350` (carry flow pops with `restoreIndex: true`), plus default
  flip at `ops.ts:959,966` (commit `996a5b09d`).
- Carry flow stashes, runs the middle op (moves HEAD), preflights, then pops. Preflight predicts the
  worktree merge only (`ops.ts` doc comment). Verified with git: staged change on line 5, HEAD then changes
  line 3; `git stash apply --index` fails `conflicts in index. Try without --index.`, plain apply succeeds.
  So a clean preflight is followed by `StashIndexConflict`: changes stay stashed, carry aborted. Before this
  chapter the same carry completed.
- Fix: on `StashIndexConflict` in the carry flow, retry once with `restoreIndex: false` and say staged state
  was not restored; or preflight the index half with `git apply --cached --check`.

### M6 `TestGridGuardTypesPostgres` stale-guard assertion is vacuous (CONFIRMED)

- Where: `internal/flows/dbflow/grid_faults_test.go:217-236`.
- `expect` holds every loaded column, `txt` included. First update sets `txt = 'edited'`, so `expect`'s
  `txt` is already stale. Second update would answer E_CONFLICT even if the `big` guard did nothing. Test
  claims to prove "another client changed a guarded column" is caught; it proves only that the `txt` guard
  works.
- Fix: rebuild `expect` from a fresh read after the first update (or exclude `txt`), then assert the
  conflict after the `big` change. Repeat per guarded type class to make the matrix real.

## Low

### L1 Grid and immediate-mutation conflict paths ignore `details.changed` (CONFIRMED)

- Where: `frontend/src/studio/views/grid/pendingChanges.ts:528`,
  `frontend/src/studio/views/shared/immediateMutation.ts`.
- They treat any E_CONFLICT as "changed on server"; `state/guardedSavedWrite.ts` and
  `state/confirmOverwrite.ts` check `details.changed`. An adapter E_CONFLICT for another reason (S3
  unreadable ACL, `configfs.ErrConflict`) would show the overwrite prompt and resend unguarded. Today S3
  uses its own confirm, so no live bug; inconsistency invites one.
- Fix: one helper that checks `code === 'E_CONFLICT' && details.changed`, used everywhere.

### L2 Expect column names not validated (CONFIRMED)

- Where: `internal/adapters/sqlmutate.go` `ValidateMutationOps` checks keys and changes, not `expect`.
- Unknown column in `expect` reaches the engine and returns a raw SQL error instead of a contract error.
  Identifiers are quoted, so no injection.
- Fix: validate `expect` names against the table's columns like `changes`.

### L3 Mongo guarded miss swallows CountDocuments error (CONFIRMED)

- Where: `internal/adapters/mongo/mutate.go:124-132`.
- If the count fails (timeout, auth), user gets "matched 0" query error, not the real cause.
- Fix: return the count error.

### L4 Stale `FinishFunc` doc comment (CONFIRMED)

- Where: `internal/agentmcp/mcp.go:42` says "The last call of a run wins"; OneShot grants (line 80) now
  refuse every call after the first accepted `finish_step`.
- Fix: reword for OneShot.

### L5 Decision 5 counts can go stale before confirm (PLAUSIBLE)

- Where: `internal/ade/taskworkflow.go` `SwitchLosesWorkError`; `internal/bridge/adetask.go`.
- Counts come from refusal time; confirmed resend deletes whatever exists then. A run finishing between
  prompt and confirm is deleted without being counted. Narrow window.
- Fix: send the counted ids or counts back with the confirm and refuse again if they grew.

### L6 Fixed sleep in negative assertion (CONFIRMED)

- Where: `tests/ui/studio/kibana-saved.spec.ts:476` (`page.waitForTimeout(1000)` then asserts no execute
  op).
- Costs 1 s every run and passes vacuously if an auto-run fires after 1 s.
- Fix: assert on a deterministic signal (e.g. results-empty state after the query store settles, or install
  the clock and run pending timers) instead of wall time.

## Per dimension

- Architecture, structure, maintainability, security: H2, M2 (statement lexing instead of server state),
  L1, L2, L4. No security finding: guard values are bound parameters, identifiers quoted; agentmcp
  `refuseAfterFinish` and lease monotonic clock look sound.
- Functional correctness: H1, H2, M1-M5, L3, L5.
- Performance and resource efficiency: nothing real. Kafka `assertPositionKept` adds one
  `ListStartOffsets` per "after" page, acceptable. `LogLines.vue` document-level copy listener is cheap and
  scoped by `contains`.
- Tests: M6, L6. Contract JSON files are checked by flow tests in read mode, so hand edits would fail the
  full run; none found. One polling loop with `time.Sleep(20ms)` in `s3_transfer_test.go` has a 15 s
  deadline, fine. No new unit spec breaks the unit-test bar.

## Decisions checklist

1. Paste NULL into nullable text: enforced (`paste.ts` `pastedValue`), tested flow and UI. OK.
2. Lost ack reconcile: not enforced (H1).
3. Concurrent edit, five places: grid enforced but broken for some types (H2, M1); Mongo, saved query,
   discover, workflow OK; API request partial (M3, M4).
4. `_id` edit refused: enforced (`mongo/mutate.go` `idEdit`), tested. OK.
5. Workflow switch confirm with counts: enforced, tested (L5 minor).
6. S3 ACL unreadable warning: enforced, tested. OK.
7. Duplicate SQS attribute refused: enforced (`adapters/rowops.go` `ParseHeaderJSON`), tested. OK.
8. Close prompts: grid staged edits OK; console transaction partial (M2).
9. Stage move refused while run live: enforced (pre-existing), flow test `TestStageMoveRefusedWhileRunLive`.
   OK.
10. Monotonic leases and timeouts: enforced (`leases.Elapsed`, `fakeclock.Suspend`), tested. OK.
11. Docker and Kubernetes destructive ops out: no such feature added; absence journeys dropped; coverage
    guard untouched. OK.

Follow-ups F-A*, F-B*, F-C* are SPEC rows in `docs/v0.3.14/SPEC.md`, as required.

## Result

Fixed on `chapter/v0.3.13`, one commit per group. Every finding fixed in full, none narrowed.

Shared transport: the data-plane error frame (`wire.fbs` `Error`) now carries optional `details` JSON
(`adapters.Error.Details`, `WithDetails`; decoded to `err.details` in `bridge/port.ts`). Flags in use:
`{"changed":true}` (row changed), `{"outcomeUnknown":true}` (lost ack), `{"aclUnreadable":true}` (S3),
`{"statementIndex":n}` (console). The flow harness `DataError.Details` reads it back.

- H1: `commitFailure` marks a lost ack `outcomeUnknown`. `pendingChanges.commitPending` now reloads the
  table and settles the staged set against it (`views/grid/reconcile.ts`): edits already stored are
  cleared, edits not stored are re-based on the server value, deletes are re-guarded, inserts are
  cleared only when the page holds the row by its full key. A keyless or serial-key insert the reload
  cannot match is put to the user (`confirmKeepUnmatched`; Cancel keeps it) and is never resent blind.
  A failed reload keeps the staged set, says so, and the next Commit settles first without sending.
  Verified: `TestGridLostAck` over explicit key, serial key and no key (details flag, one read, a blind
  resend refused only for the explicit key and a duplicate otherwise); UI half `grid-lost-ack.spec.ts`
  (5 tests) on contract `studio-grid-lost-ack`.
- H2: `postgres/mutate.go` `expectColumn` guards types outside the native-equality list through
  `col::text`, so xml, point, polygon and path no longer compare with `=`. `TestGridGuardTypesPostgres`
  runs xml, point, polygon, path, char(5), enum, uuid, inet and the earlier types on a real Postgres.
  Reverting the fix fails it on xml.
- M1: `mysqlfamily/mutate.go` leaves single-precision FLOAT out of the guard (its `=` is not stable);
  `typeroundtrip_test.go` gained FLOAT and DOUBLE columns in the guard matrix on a real MySQL. Reverting
  fails it.
- M2: transaction state is keyed by the connection session (`console/transaction.ts`), not the tab: the
  last console tab on a session ends its transaction, an earlier close prompts nothing and rolls
  nothing back. Failed runs are tracked through the error's `statementIndex` (statements before it ran,
  `BEGIN; bad` stays open). MySQL implicit commits (DDL, grants, table maintenance, locks, autocommit
  on) close it, `COMMIT AND CHAIN` keeps it, temporary-table DDL does not close it. Disconnect and
  connection removal clear it (`forgetConnectionTransactions`). Lexing stays: MySQL 8.4 has no
  `@@in_transaction`, so a server read would not cover it; the rules are pinned on real servers instead.
  Verified: `TestConsoleMySQLTransactionEnds` (real MySQL: implicit commit, chain, failing script index)
  and `TestConsolePostgresTransactionAndCap` (failure index) write contract `studio-console-script`;
  unit spec `console-transaction.spec.ts` replays those lists plus the per-connection and disconnect
  rules; UI half in `grid-staged-close.spec.ts` (script failing past its BEGIN, close asks, confirm sends
  ROLLBACK).
- M3: `apifiles.ItemExpect{Name, HTTP, GRPC}` compares the name and the whole saved item; the bridge
  takes `expectName`; the client sends the item name it holds. `TestSavedRequestTwoWindows` gained a
  concurrent-rename step (conflict, rename kept). UI: `api-request-conflict.spec.ts`.
- M4: `SaveGrpcRequest` takes `expect` and `expectName` with the same guard and
  `E_CONFLICT {"changed":true}`; `GrpcRequestView` guards through `useSeenSaved` and
  `guardedSavedWrite`. Flow `TestSavedGrpcRequestTwoWindows` (contract `api-grpc-two-windows`), UI
  `grpc-request-conflict.spec.ts`.
- M5: the carry flow retries once with `restoreIndex: false` on `StashIndexConflict` and announces that
  the staged changes came back unstaged. Test in `ops.spec.ts` (retry sent, announcement; no retry when
  the index restores).
- M6: `TestGridGuardTypesPostgres` rebuilds `expect` from a fresh read per column, changes only that
  column on the server, asserts E_CONFLICT with `changed`, and a control with that column refreshed
  must pass. Per type class, so the matrix is real.
- L1: `isConflictWith(err, flag)` and `isChangedConflict` in `state/confirmOverwrite.ts` are the one
  check; `guardedSavedWrite`, `immediateMutation` and S3 use them.
- L2: `ValidateMutationOps` rejects expect column names that are not table columns, for update and
  delete; covered in `TestGridGuardTypesPostgres`.
- L3: the Mongo guarded miss returns the `CountDocuments` error instead of reporting a conflict.
- L4: `agentmcp.FinishFunc` comment says a OneShot grant accepts one call.
- L5: `SetTaskWorkflowArgs` gained `confirmedRuns` and `confirmedLogs`; the backend recounts under the
  task lock and refuses again when the task holds more; the refusal carries the counts in `details` and
  the dialog sends them back. Tests: `runengine_test.go`, `TestWorkflowSwitchKeepsOtherTasksRuns`,
  `ade-v2-workflow-switch.spec.ts`.
- L6: `kibana-saved.spec.ts` advances the installed fake clock (`page.clock.runFor`) instead of
  `waitForTimeout(1000)`.

Contracts regenerated with `KIRA_CONTRACT=write` and stable on rerun: `studio-grid-lost-ack`,
`api-request-two-windows`, `api-grpc-two-windows`, `studio-console-script`, `studio-document`,
`studio-grid-stale`, `studio-s3-edit`, `ade-workflow-switch`. `DataError.Details` is `omitempty` so
contracts without details are unchanged.

Ran after the last change: Go `apifiles`, `bridge`, `adapters/...`, `adapterhost`, `flowharness`,
`agentmcp`, `ade`, `flows/apiflow`, `flows/adeflow`, `flows/dbflow` (real Postgres, MySQL, Mongo, Redis,
Kafka, SQS, S3); bun `ops.spec.ts` and `console-transaction.spec.ts`; Playwright `ui` specs
`grpc-request-conflict`, `api-request-conflict`, `grid-lost-ack`, `grid-staged-close`,
`grid-staged-survive`, `grid-stale-row`, `document-edit`, `s3-object-edit`, `ade-v2-workflow-switch`,
`kibana-saved`, `saved-queries-conflict`. Full UI suites are left to PR CI.
