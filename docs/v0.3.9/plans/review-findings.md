# v0.3.9 code review findings (round 1)

Base: `6e2c1bfff73ad435d6308ad73c98d374c63c4e93` (merge-base of `chapter/v0.3.9` and `origin/main`; no
earlier review in this chapter). Diff `base..HEAD`, P607-P616.

Severity: High, Medium, Low.

## Architecture, structure, maintainability, security

1. Medium. `.github/workflows/pr.yml:113`, `:328`, `:366` (aggregates `ui`, `test-matrix`,
   `db-compat`). Defect: `if: ${{ !cancelled() }}` skips the aggregate when the run is cancelled, and
   a skipped required check counts as passing. A hand-cancelled run (or one cancelled after the fast
   required checks went green while matrix or shard legs still ran) leaves every required check
   green, so auto-merge can merge a PR whose matrix, db-compat or ui legs never finished. P614/P616
   plans name the hand-cancel case and accept it. Fix: `if: ${{ always() }}` on all three aggregates;
   `test "$RESULT" = success` already fails on `cancelled` or `skipped`. A superseded run then shows a
   red aggregate on an old sha only, which the new run's checks replace.

2. Low. `docs/PACKAGING.md:539-553` ("P616 apply order"). Defect: describes applying
   `docs/pending-changes/.github__workflows__pr.yml.patch`, but that patch is applied (aggregates
   live in `pr.yml`) and `docs/pending-changes/` no longer exists. Stale process text. Fix: delete
   the section; keep the ruleset add-before-remove note only if the ruleset swap is still pending,
   reworded without the patch.

3. Low. `apps/kira-space/internal/mobileweb/writes.go:114-127`. Defect: `markSeen` was inserted between
   `logWrite`'s doc comment and `logWrite`, so `// logWrite is the one audit line ...` now heads
   `markSeen`'s doc and `logWrite` has none. Also `markSeen` runs synchronously inside the
   `authorized` callback (`handleTerminal:289-297`), which `mobileterm.Broker.Serve` calls before
   `websocket.Accept`: a slow DB write delays the attach up to `seenCallLimit` (5 s). Fix: move the
   `logWrite` comment back above `logWrite`; run the mark after the auth check returns, off the
   attach path (`go s.markSeen(...)` with its own detached context, as it already builds one).

## Functional correctness and business logic

4. Medium. `apps/kira-space/internal/dbmcp/browse.go:333-357` (`keyColumns`), used by `stabilize`.
   Defect: index column names are trusted as plain column names. Postgres `listIndexes`
   (`adapters/postgres/catalog.go:358-361`) returns `pg_get_indexdef(..., k, true)` per key, which is
   an expression (`lower(email)`) for an expression index and a quoted identifier (`"MixedCase"`) for
   a mixed-case or special-character column. Such a name is absent from `nullable`, so the
   NOT NULL test passes (`nullable[c]` is false for an unknown key), and `stabilize` appends a
   `SortTerm` the adapter quotes as an identifier: on a table with no primary key whose first unique
   index is an expression or mixed-case index, every row browse (not count) fails with "column does
   not exist". A partial unique index (`WHERE ...`, not exposed in `IndexMeta`) is also taken as a
   key and reported `stableOrder: true` though rows outside its predicate are not unique. Fix: in `keyColumns` accept an index only when every column
   is in `r.known` and is NOT NULL by an explicit lookup (`v, ok := nullable[c]; !ok || v` rejects);
   skip partial indexes (add a partial flag to `IndexMeta` for postgres `indpred IS NOT NULL` and
   sqlite `pragma_index_list.partial`, or skip unique indexes on those engines unless the flag is
   known false). Add a flow or unit case: table with no PK and a unique index on `lower(col)`.

5. Medium. `packages/workbench/src/state/agentActivity.ts:35-40` and
   `apps/kira-space/internal/agentnotify/agentnotify.go:148-150`. Defect: every event carrying
   `agent_id` is dropped, Notification included. The recorded fixture
   (`internal/flows/adeflow/testdata/background-hooks.jsonl`) shows Claude tags a subagent's own
   tool hooks with `agent_id`; a permission prompt raised inside a subagent arrives the same way, so
   the main session never enters `attention`, the status shows `working`, and no needs-input OS
   notification fires while the subagent waits on the user. Before P610 these events moved the
   phase. Fix: let `Notification` (non-idle types) through both filters regardless of `agentId`, and
   let a subagent `PostToolUse` move `attention` back to `working` (phase only, no `runningTools`
   change). Pin the payload in the real-`claude` hooks test (subagent permission prompt carries
   `agent_id` or not) so the decision rests on a recorded fact.

6. Medium. `apps/kira-space/frontend/src/ade/v2/sessions/AdeSessionsRow.vue:39-44`,
   `AdeTuiPane.vue:40-44`; backend `ade/tracker.go:878-905`. Defect: a TUI session is marked unseen on
   a main Stop. If the user does not view it before its PTY ends (app quit or restart, which kills
   every PTY, or the terminal closed elsewhere), the row is stopped and keeps `unseen_at`. Clear paths
   are a held TUI pane (running only), a headless log toggle (`hasLog` is headless-only), archive and
   mobile attach. None applies to a stopped TUI row, so it stays in the status bar count
   (`N unseen`) and the agents popover until the task is archived or the session is resumed. Clicking
   the row in the popover (`useOpenSession` reveals it in the sessions menu) does not clear it either.
   Fix: mark seen when a stopped row is revealed or opened (`AdeSessionsTab` reveal watcher or
   `useOpenSession` for a stopped session), and give stopped TUI rows the same view hook as the
   headless log toggle (row shown in the open sessions menu counts as viewed).

7. Low. `apps/kira-space/frontend/src/ade/v2/sessions/AdeHeadlessPane.vue:20-23`. Defect:
   `useMarkSeenOnView` on the headless pane never fires: `AdeSessionsTab` renders the pane only for
   running sessions (`current` comes from `running`), and `stopHeadlessSession` sets `unseen_at` in the
   same step that marks the row stopped. Dead wiring that reads as a clear path. Fix: remove it, or,
   if a stopped headless session should clear on view, wire that where stopped rows render (see 6).

8. Low. `apps/kira-space/internal/dbmcp/render.go:1074`. Defect: past offset 100000 the note tells the
   agent to "continue with a gt or lt filter on the last row's order value and offset 0". With the
   stabilized order (user terms plus key), a strict filter on the first order value skips every
   remaining row that ties on it, and `browse_table` filters are ANDed, so a correct keyset predicate
   over two columns cannot be expressed. Fix: say it only when the effective order is a single key
   column (then `gt`/`lt` on that key is exact); otherwise tell the agent to narrow with a filter on
   the key column, or use `run_query` where read is allowed.

## Performance and resource efficiency

9. Low. `scripts/test-matrix.sh:153-155`. Defect: the comment says `-timeout 8m` sits under the 12-min
   job cap, but the `rest` group runs six packages in sequence in one 12-min job (`pr.yml`
   `test-matrix-group`): a hang in a later package after about 4 min of earlier packages hits the
   runner kill first, with no goroutine dump, which the timeout exists to give. Fix: derive the
   per-package timeout from the time left (pass a deadline from the workflow, for example
   `MATRIX_DEADLINE`, and use `min(8m, remaining - 1m)`), or split `rest` so no group exceeds one
   package's budget plus margin.

No other performance findings: count paths add one catalog read only when conditions are present;
`Tracker.Background` is a map read under the existing lock; the unseen write per main Stop is one
row update plus one `notifySessions` broadcast.
