# v0.2.35 code review, round 1 of 2

Base `d0541228b` (v0.2.34 merge on main, last review session). HEAD `1799c69f5`. Scope: `git diff d0541228b..HEAD`
(P512 to P527 plus full-run fixes), docs/v0.3.0 and docs/v0.2.36 excluded. Three dimensions: architecture and
security, functional correctness, performance. Findings only; a fixer applies them.

Counts: High 0, Medium 8, Low 5.

## Medium

### F-R1-1 Cell editor "UUID (v4)" generator now emits UUIDv7

- Files: `apps/kira-studio/frontend/src/views/shared/celleditor/generate.ts:70`,
  `packages/theme/src/i18n/en.json:12810-12812`, `apps/kira-studio/tests/ui/cell-editor.spec.ts:877`.
- Problem: P523 (`f6df6ac22`) swapped `crypto.randomUUID()` for `newUuid()` (v7). This generator writes user data
  into the user's own column. P523 plan §3.6 lists it as "user data ... keep" and §5 exempts the file from N14. The
  label still reads "UUID (v4)", hint "A random 36-character dashed UUID". A user now gets a time-ordered v7 that
  leaks its creation time. Full-run F4 rewrote the spec to expect v7, locking the regression in.
- Fix: restore a v4 value (`crypto.randomUUID()` is fine here; the file is N14-exempt per plan, or add `v4` from
  `uuid` behind a named user-data helper in `@shared/ids`). Revert `cell-editor.spec.ts:877` to the v4 pattern.

### F-R1-2 Merged branch request outlives the agent's MCP call timeout

- Files: `apps/kira-space/internal/ade/branchrequest.go:203` (`joinBranchRequest`), `:439-469`
  (`awaitBranchRequest`), `internal/harness/mcp.go:12` (`MCPCallTimeout = 35 min`).
- Problem: every join resets `ExpiresAt` to now + 30 min for the whole request, and every waiting call re-arms to
  that new expiry. Call A at t=0, call B joins at t=20: expiry t=50. A's client cuts the call at t=35
  (`MCPCallTimeout`, written into the per-server timeout and `MCP_TOOL_TIMEOUT`). The agent sees a tool timeout
  while the dialog still counts down 15 more minutes; A's items are withdrawn or, if the client does not cancel the
  request, A's approved branches get created after the agent already gave up. The `MCPCallTimeout` comment assumes
  one 30 min wait.
- Fix: give each call its own deadline (join time + wait) and time out that call's items alone, or cap the request
  expiry at the oldest live call's start + wait. Add a flow case with an injected clock: two joins 20 min apart,
  first call answers by 30 min after its own start.

### F-R1-3 `isReadOnlyRefusal` substring match steers the agent to mode=write

- Files: `apps/kira-studio/internal/dbmcp/tools.go:244-246`, `:452-460`.
- Problem: any mode=read error whose text contains `readonly`, `read only`, `read-only`, `1792`, `1290` or `(164)`
  becomes "the database refused a write on the read-only connection ... Use mode=write or mode=ddl". Example:
  `SELECT * FROM readonly_users` on a missing table, or a cast error naming the literal `1290`. The agent is told to
  rerun a plain read under mode=write, on the main connection, with no engine read-only session. That nudges the
  agent around the P527 enforcement it was meant to respect.
- Fix: match the engine's structured code, not message text: postgres SQLSTATE `25006` via `pgconn.PgError.Code`,
  mysql `*mysql.MySQLError.Number` in {1792, 1290}, sqlite result code 8, clickhouse exception code 164, and Kira's
  own app-lane refusal as a typed error (or an `adapters.Error` code). Fall back to the normal tool error otherwise.

### F-R1-4 Bulk `.env` editor shows typed secret values in clear text

- Files: `apps/kira-studio/frontend/src/api/BulkVariablesEditor.vue:38-45`,
  `apps/kira-studio/frontend/src/api/variablesForm.ts:127-129`.
- Problem: the bulk view is a plain `CodeTextEditor`. A value typed on a secret's line (`typed[...] = e.value`) is
  sent to the write-only secret call on Save but stays visible on screen, in the editor's undo history, and in any
  screenshot. P516's ask: every place a secret is entered or shown has a reveal toggle, default hidden. Every row
  editor uses `SecretInput`; this path bypasses it.
- Fix: do not accept secret values in bulk mode. A non-empty value on a secret line becomes a parse problem pointing
  the user to the row's `SecretInput` (keep the `KEY=` line round-trip). Alternatively mask the value span with a
  Monaco decoration plus a reveal toggle, but the refusal is simpler and leaves no plaintext in the model.

### F-R1-5 `review_comment.id` stays an integer against the P523 rules as written

- Files: `docs/v0.2.35/plans/P523.md:611-617` (D1), `docs/ARCHITECTURE.md:213`, `apps/kira-space/internal/gitreview`
  (`Comment.ID int64`), git-ui `reviewComments.remove(c.id)`.
- Problem: P523 rule 5 allows autoincrement only for keys with no bridge exposure; rule 2/3 then require UUIDv7. D1
  admits the id crosses the bridge (git-ui and ADE review tools echo it) and keeps the integer as "plan's call; the
  user can overrule". The SPEC row has no Result and records no user sign-off, so this is a requirement narrowed by
  the planner, not the user.
- Fix: either convert per D1's own path (gitreview migration to TEXT ids, old rows mapped to fresh v7 with fks
  rewritten in the same statement, Go and TS types to string) or get the user's explicit keep decision and record it
  on the P523 SPEC row.

### F-R1-6 P518 cancel via MCP covers Studio only; the Space scope cut is unconfirmed

- Files: `docs/v0.2.35/plans/P518.md:30-32`, `internal/agentmcp` (no `list_operations`/`cancel_operation`),
  `apps/kira-studio/internal/dbmcp/ops_tools.go`.
- Problem: the ask says an agent can stop "any request listed in the ops panel". Space has an ops panel and agents
  (ADE `agentmcp`), yet only Studio's `kira-db` server got the tools. The plan marks this `[confirm]` with the user;
  neither SPEC nor the plan records the answer.
- Fix: add `list_operations` and `cancel_operation` to Space `agentmcp` over Space's oplog (`cancellable` rows only,
  background rows hidden, task-scoped like the other Space tools), with a paired flow test and UI spec; or record the
  user's confirmation of the Studio-only scope on the P518 row.

### F-R1-7 Perf documents fixture still drives the pre-v0.2.28 connection dialog

- Files: `apps/kira-studio/tests/perf/documentsFixture.ts:101-117` (`connectMongo`), callers
  `tests/perf/documents-scroll.spec.ts`, `heap-ram.spec.ts`, `parse-callers.spec.ts`;
  helper `apps/kira-studio/tests/e2e-real/support/connections.ts`.
- Problem: `connectMongo` clicks `connection-kind-mongodb`, fills `connection-name`/`-host`/`-port`, saves through
  `connection-save` and waits for `connection-dialog` to close. Those testids belong to the dialog the v0.2.28 file
  dialog replaced (same breakage full-run F11 fixed in e2e-real). Every documents perf probe fails at setup.
  FULLRUN.md parked it as out of scope.
- Fix: create the connection through the bound `ConnectionsService.Create` call, as
  `e2e-real/support/connections.ts` `createConnection` does. The perf specs run on mocked IPC (`CONTROL` snapshot
  for `IPC.connectionsCreate`), so move the helper's argument builder to a shared support module (or give it a
  `Page`-level variant that invokes the bound call through the page bridge) and make the `CONTROL` snapshot args
  match it. Then connect from the tree as today.

### F-R1-8 Approving a branch request can deny items the user never saw

- Files: `apps/kira-space/internal/ade/branchrequest.go:309-339` (`ApproveBranchRequest`), `:210-225`.
- Problem: approval reads a snapshot (`BranchRequest`), builds `approved` from it (or from the client's item ids),
  then `resolveBranchRequest` answers every call merged in by then. A call that joins between the user's last
  render and the click (or between snapshot and resolve) is answered "denied" for items the user never decided on.
  The agent then treats a never-shown branch as refused.
- Fix: resolve only the calls whose items were all known at approval time; keep later-joined calls pending in a
  fresh request (re-put the item), or refuse the approval with a stale-view error so the dialog re-renders.

## Low

### F-R1-9 G6 and N14 guard less than P523 §5 specifies

- Files: `internal/conventiontest/conventions_test.go:42-43`, `:132-139`; `scripts/check-conventions.ts:225-236`.
- Problem: G6 flags imports of `google/uuid` and `go-nanoid` only. The plan also bans `crypto/rand.Text()` and
  `github.com/oklog/ulid`; N14 should flag `Math.random().toString(36)`. None is in use today, so this is a
  regression gap only. Import-level G6 (stricter than call-level for uuid) is fine.
- Fix: add a `rand.Text` call check (selector `rand.Text` on `crypto/rand`) and `oklog/ulid` to `idGenerators`; add
  the `Math.random\(\)\.toString\(36\)` regex to `rawIdHits` with a spec case.

### F-R1-10 `api_request` sends a re-plan that may differ from what the user approved

- Files: `apps/kira-studio/internal/dbmcp/api_tools.go:100-109`, `apps/kira-studio/internal/agentapi/send.go:28-49`.
- Problem: `Send` re-plans from the raw input against the environment as it is now. If a plain variable (for
  example `base_url`) changes during the up to 10 min approval wait, the request goes to a different URL, path or
  body than the approval showed. Credential host rules still apply, so no secret leaks, but an approved GET to host
  A can become a GET to host B.
- Fix: pass the approved `Planned` into `Send` and refuse when the fresh plan's method, URL, headers, query and body
  differ ("environment changed while waiting for approval; send again").

### F-R1-11 Step library migration leaves orphan library files when a workflow is skipped

- Files: `apps/kira-space/internal/adeflow/migrate.go:141-169`.
- Problem: `workflow()` creates library files (`m.r.Steps.create`) before the resolve check. When the check fails,
  the workflow file stays inline but the new library steps remain and `m.taken` keeps them, so later workflows may
  reference them and the Steps tab shows steps nobody uses.
- Fix: build `out` and run the resolve check against an in-memory lookup that includes `created` first; write the
  library files and then the workflow only after the check passes (remove created files on a later write error).

### F-R1-12 P523 docs out of date

- Files: `docs/v0.2.35/SPEC.md:22` and the P523 row; `docs/v0.2.35/plans/P523.md:709-720`.
- Problem: the table still says P523 Pending and the row has no Result, though the phase landed (`5496754cc`,
  `95f76d089`, UI specs `5d8e24c09`). The plan Result names pre-rebase commits (`8260ed560`, `46988939c`, ...) that
  are on no branch, and says the UI specs are deferred while `ade-ids`, `op-ids`, `op-id-guard`, `tabs-scope` now
  exist.
- Fix: mark P523 Done with a Result on the SPEC row (including the D1 outcome from F-R1-5); refresh the plan Result
  with the current commit ids and drop the stale "deferred" line.

### F-R1-13 `run_query` read lane connect and idle close log as user connect/disconnect ops

- Files: `apps/kira-studio/internal/adapterhost/readlane.go:173-176`, `:218-226`; `router.go:158-168`.
- Problem: lane open runs `RunOp` with kind `connect`, lane teardown runs `disconnectAdapter` (kind `disconnect`),
  both under the user's connection id. The Operations panel and op log then show "connect"/"disconnect" rows for a
  connection the user never touched; the 10 min idle close shows a disconnect while the connection stays connected.
- Fix: give lane ops their own kind (`laneConnect`/`laneDisconnect`, added to `model/ops.go`'s kind set and the
  panel labels) or mark them `source: mcp` so the panel can label them as the agent's read lane.

## Not findings / clean areas

- P519 "Copy all tabs" removal: no code defect. The stacked copy was the long PNG the ask rejects; residue is clean
  (no `copyAll` shot keys, no `stackCanvases`). A multi-file clipboard copy stays a possible new phase (plan D6).
  `proofshots.go` keeps renderer paths out (opaque `shot_` ids, fresh folder, no overwrite).
- P522 defaults verified: `dbMcp.apiRequests` false, `apiApproval` "always" (Go `model/settings.go:95`, TS
  `settingsDomain.ts:134-135`), per-variable `agent` off, `agentAutoApproveReads` off; settings read failure fails
  closed. Credential handling: host rules check scheme, host and port; IDNA and IP normalised; secrets cannot sit in
  the host (`url.Parse` rejects `{`); `sameTarget` re-checks after substitution; redirect guard per hop; responses,
  errors, op log and header echoes masked; truncated bodies trimmed before masking. No secret reaches the agent.
- P527 read-lane known limits: Mongo and Redis are app-checked and labelled `app`; ARCHITECTURE Known open items
  lists them plus postgres/mysql side effects, write vs ddl and SQLite `ATTACH`. Lane fails closed, a stale lane
  (replaced primary) is discarded, cancel reaches the lane adapter, exposure off closes the lane. Mongo console
  grammar is a closed method set.
- P523 mislabeled commit (`8021e3961` and its sibling hold mixed Studio/ADE edits): history is pushed; rewriting it
  on the shared chapter branch costs more than the label. Not a code defect; F-R1-12 covers the stale Result text.
- P523 op id gate, optional op ids, appendFailed history guard and tab scoping (`WHERE tabs.window_key =
  excluded.window_key` plus `RowsAffected`) match the plan; no cross-window tab move exists to break.
- P518: agent cancel uses `CancelRunning` (no early-cancel memory); hidden connections and unexposed scripts read as
  unknown alike.
- P521: secret params never come from the agent; approval payload and logs carry masked text; `contains` filters
  masked text, so no probing oracle; plan hash refuses a script changed after approval.
- P520 queue: per-id failures do not abort the batch; prompts open under the lock; Space `leaveBranchRequest`
  prefix match cannot hit `c10i` for `c1`.
- P517 branch names: git `check-ref-format`, leading `-`, `@{`, `HEAD`, main and integration branches refused.
- P524 order: dense reindex in one transaction, unknown or repeated ids refused.
- P512 watcher: board-change passes rate-limited (30 s), timer 5 min, Jira 10 min, failed reads keep previous
  snapshots.
- P514 config form draft, P516 `SecretInput` and saved-password reveal (existing gated call), P525 styling and P526
  step library engine: no defect found beyond F-R1-4 and F-R1-11.
- Performance: the DB MCP server reads settings through a prepared statement per HTTP request to decide the API
  tool set; cheap, rebuild only on change. No other hot path added.
