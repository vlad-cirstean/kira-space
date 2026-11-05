# v0.2.35 code review, round 2 of 2

Base `d0541228b` (same as round 1). HEAD `5dd1f16f1`. Scope: `git diff d0541228b..HEAD`, focus on round-1 fixer
commits after `ac766957e` (`93267b5c6` to `5dd1f16f1`). docs/v0.3.0 and docs/v0.2.36 excluded. Three dimensions:
architecture and security, functional correctness, performance. Findings only; a fixer applies them.

Counts: High 0, Medium 3, Low 4.

## Medium

### F-R2-1 Partial branch approval leaves the late call's card disabled until it times out

- Files: `apps/kira-space/internal/ade/branchrequest.go:371-380` (`ApproveBranchRequest`),
  `packages/workbench/src/state/approvals.ts` (`begin`, `prune`),
  `packages/workbench/src/approvals/ApprovalItemCard.vue:21`, `:101`, `:112`.
- Problem: the F-R1-8 fix keeps a late-joined call pending and re-puts the same approvals item
  (`branch.request:<reqID>`). The client marked that id `answering` in `store.begin` before the call.
  `answering` clears only on `fail` or when `prune` sees the id gone; a success with the item still listed does
  neither. The card's Deny and Approve buttons get `:pending="busy"`, which disables them. Scenario: call A
  renders, call B joins, user clicks Approve. A resolves, B stays pending, its card shows a spinner with both
  buttons disabled. B can only time out after 30 min; the agent waits the whole time.
- Fix: clear `answering` for ids whose answer succeeded (add a `done(id)` store action called on a result
  without error), or prune `answering` when a push changes an item's payload. Add a UI spec: late join, approve
  the first call, then approve the second from the same card.

### F-R2-2 Migration 0007 orphans the comment ids of review rounds stored before it

- Files: `apps/kira-space/internal/gitreview/migrations/0007_p523_comment_id.sql:22-31`,
  `apps/kira-space/internal/storage/model/adetask.go` (`CommentIDList`),
  `apps/kira-space/internal/ade/reviewstep.go:801`, `:866`, `:891-904`, `internal/agentmcp/review.go:47`.
- Problem: 0007 gives each old row a fresh random v7 id and keeps no link to the old integer. `CommentIDList`
  decodes a stored round's `[3,4]` to `["3","4"]`, which now matches no row. A round running across the upgrade
  (the upgrade is a restart, so `resumeReviewRound` runs) finds no open comments and resumes without the
  follow-up. `Reply` and the MCP grant refuse every id ("comment X was not sent to you"). The fallback route
  (`:801`) queues a new round with zero comments. The comments stay open with no agent able to answer them.
  The P523 Result says legacy ids "still decode", which reads as working.
- Fix: keep the mapping. Either add a nullable `legacy_id INTEGER` column filled from the old id in 0007, and
  map numeric entries through it when a spec is read (`roundSpec`), or derive the v7 random part
  deterministically from the old id so the reader can compute it. Add a flow case: round stored with numeric
  ids, migrate, resume, reply succeeds.

### F-R2-3 `review_removed.id` is an autoincrement integer that crosses the bridge

- Files: `apps/kira-space/internal/gitsession/removed.go:31` (`ReviewRemovedEntry.ID int64`),
  `apps/kira-space/internal/gitrpc/wire.go:427` (`IDs []int64`), `apps/kira-space/internal/gitreview/removed.go:219`,
  `packages/git-ipc/src/contract.ts:1325`, `:1626`, `apps/kira-space/frontend/src/ade/v2/review/useReviewRemoved.ts:69`;
  `docs/v0.2.35/plans/P523.md:294`.
- Problem: same rule F-R1-5 enforced for `review_comment.id`. P523 rule 5 allows autoincrement only with no
  bridge exposure. The plan lists `review_removed.id` as DB-internal, but the renderer receives it in
  `ReviewRemovedEntry` and sends it back in `review.removed.dismiss`. A grep of every `ID int64 json:"id"` in Go
  finds this one site left.
- Fix: convert like 0007 (TEXT UUIDv7 id, `review_removed` has no referrers), Go and TS types to string,
  dismiss ids `[]string`. Correct P523.md:294. Or record the user's explicit keep decision on the P523 row.

## Low

### F-R2-4 Deny answers late-joined calls the user never saw

- Files: `apps/kira-space/internal/ade/branchrequest.go:384` (`DenyBranchRequest`), `:142-148`.
- Problem: F-R1-8 fixed approve only. Deny still calls `resolveBranchRequest`, which answers every merged call.
  A call that joined after the view rendered is told "denied" for branches the user never saw. The client
  already sends `shownItemIds` in the options for both answers.
- Fix: in `branchOwner.Answer`, decode options for deny too and remove only calls whose items are all in
  `shownItemIds`, as approve does; keep the rest pending.

### F-R2-5 Review prompt still calls the UUID id a "number" and prefixes it with `#`

- Files: `internal/llmprompts/registry.go:888` (`The comment number, from the list.`), `:1205`, `:1246`
  (`#{{.ID}}`), `internal/agentmcp/review.go:46-48`.
- Problem: the agent now sees `#0193...-...` and is told to pass "the comment number". An agent that passes
  `#0193...` or a number gets "was not sent to you" (string compare) or a schema error, then retries.
- Fix: word the arg as "The comment id from the list, without #" and render ids without `#` (or `id <uuid>`);
  optionally `strings.TrimPrefix(args.ID, "#")` in the handler. Run the matching `bun run test:claude*` row
  per `docs/DEV_ENVIRONMENT.md` after the change.

### F-R2-6 Space `list_operations`/`cancel_operation` have no flow test or UI spec

- Files: `apps/kira-space/internal/ade/agenttools_test.go` (`TestSpaceTools_operationsAreTaskScopedAndGitOnly`),
  `apps/kira-space/internal/flows/` (no case).
- Problem: CLAUDE.md requires a new feature to be proven by a flow test plus a UI spec of the same scenario;
  F-R1-6's fix text asked for both. The feature landed with a unit test inside `internal/ade` (a fake oplog,
  not a real agent call through the loopback server) and none of either. Studio's counterpart has
  `dbmcpflow/ops_test.go`.
- Fix: add a Space flow case (fake agent calls `list_operations` during a task git op started under an MCP
  call, then `cancel_operation`; assert the op row ends cancelled and another task's op is unknown) and a UI
  spec asserting the cancelled row in the Operations panel. Drop the unit test if the flow covers it.

### F-R2-7 Docs out of date after round-1 fixes

- Files: `docs/ARCHITECTURE.md:213`, `docs/v0.2.35/plans/P523.md:720`.
- Problem: ARCHITECTURE still says "`review_comment.id` stays an integer". The Ids section does not mention the
  user-data helper `newUserUuidV4`. P523.md "Deviations" says G6 flags imports only, not call sites; it now also
  flags `crypto/rand.Text` calls.
- Fix: replace the line with the TEXT UUIDv7 fact (and F-R2-2's legacy mapping); add one line for
  `newUserUuidV4` (user data only); update the Deviations line.

## Not findings / clean areas

- F-R1-1: `newUserUuidV4` (`uuid` v4) lives in `packages/shared/ids.ts`, the one N14-allowed file; spec back to the
  v4 pattern. Correct.
- F-R1-2: per-call deadline, request expiry = earliest live deadline, timeout withdraws only that call's items.
  Timer/answer race is safe: a withdrawn-after-answer call finds itself gone and reads its buffered answer.
  Within `MCPCallTimeout` (35 min).
- F-R1-3: tagging via `adapters.RefusedWrite` on postgres 25006, mysql 1792/1290, sqlite readonly, clickhouse
  164 (`tableIsReadOnly` 242 correctly untagged), and Kira's own `AssertWritable`/`ReadOnlyViolation` (Mongo,
  Redis app lanes, per-request wrap). `adapters.Error.Unwrap` reaches the sentinel; the dbmcp call is
  in-process, so the chain survives. Message text no longer matters.
- F-R1-4: refusing a value on a secret line satisfies P516. Bulk text is no longer a place where a secret is
  entered; every secret entry goes through `SecretInput` with its reveal toggle, and nothing typed in bulk
  reaches `apiConfigSetSecret`. Line numbers map correctly (the parser is single-line). `SECRET_MARKER` lives
  only in the in-memory bulk text, so changing its wording breaks no stored file.
- F-R1-5 consumers: Go store, gitrpc validation, gitsession, ADE (review, archive snapshot carries no id),
  agentmcp grant and reply, llmprompts vars, git-ipc contract, git-ui state, Space frontend all moved to
  string; regenerated bindings show `id: string`. Migration SQL builds valid v7-shaped ids (`printf('%012x')`
  timestamp, version 7, RFC variant), keeps every column of 0002/0004/0005 and the index; nothing references
  `review_comment`. Same-millisecond comments now tie-break at random instead of insertion order; cosmetic.
- F-R1-6 security: rows filtered to the caller's task (`Origin.TaskID` is set only by the MCP recorder, so user
  window ops never appear), foreground git rows that registered a cancel; sessions, runs, setups and MCP rows
  stay out; an id outside the list reads as unknown. Not offered to rebase runs or review rounds.
- F-R1-7: perf fixtures use `createConnection` and `mongoCreateArgs`, the same helpers the UI specs use.
- F-R1-9: G6 covers `oklog/ulid` (v1 and v2) and `crypto/rand.Text` calls under any import name; N14 covers
  `Math.random().toString(36)` with a spec case.
- F-R1-10: `sameRequest` compares the display plan (method, URL, headers, query, body, content type, form)
  built from plain variables, so a changed plain variable refuses with `ErrPlanChanged`; flow test proves the
  server saw no request.
- F-R1-11: rollback removes only files this workflow wrote and frees its `taken` ids on every failure path,
  including the final write.
- F-R1-12: SPEC P523 Done with Result; every commit id in P523.md is on the branch.
- F-R1-13: lane connect and teardown run under `OpSourceMCP`; the Operations panel labels them MCP. No frontend
  code reacts to op kind `disconnect`.
- Performance: nothing new on a hot path. 0007 is a one-time table copy; `refreshBranchExpiry` and the approve
  scan are linear in a request's calls and items.
