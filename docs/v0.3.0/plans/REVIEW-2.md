# v0.3.0 code review, round 2

Base: `736caa3e5` (v0.2.36 squash merge, PR #31), same as round 1. Diff `736caa3e5..5ce3b978d`, whole diff. Round-1 fixer commits (`e18b9abe3`..`03d4e9381`, `0117c3a39`, `922e38b57`, `9eb4f82a3`, `ac57ff687`, `5ce3b978d`) reviewed as new code.

Severity: High = data loss or security hole on a normal path; Medium = real failure on a reachable path; Low = narrow edge, hygiene or cost.

No High or Medium findings.

## Low

### L1. homefold never finishes when the legacy home has no `kira.db`

- Where: `apps/kira-space/internal/dbfold/dbfold.go:78-81` with `apps/kira-space/internal/homefold/runs.go:27-85`.
- Failure: `homefold.Runs` is called only from `dbfold.importStudio`. When the legacy folder holds `runs/` or `automations/` but no `kira.db`, `landDB` writes no `studio.db`. `dbfold.Run` then sees no marker and no file, writes `{done, fresh}`, and never calls `Runs`. The homefold marker stays at `files` for good. The Studio run files and automation folders stay in the backup and never reach Space, with no log. Same end state when `studio.db` disappears mid-import (`dbfold.go:107-108` warns, then marks done).
- Fix: let `Runs` run with no Studio database (empty row set, `buildMaps` over the run files only), and call it from `dbfold.Run` whenever the homefold marker is `files` or `runs`, not only inside `importStudio`. Or have `dbfold` log a warning naming the backup when it leaves homefold short of `done`.

### L2. Resumed `copyRows` logs every Studio row as dropped

- Where: `apps/kira-space/internal/homefold/runs.go:370-376`, flow at `runs.go:58-78`.
- Failure: `copyRows` commits, then `copyRunFiles`, `copyAutomations` and `deleteStudioRows` run outside that transaction. A crash or failure in those steps resumes at `copyRows`. Every row now clashes with its own earlier copy, so `insertRows` counts all of them and the warning reads "Studio rows dropped on a key clash with Space" with `dropped == read`. Nothing was lost; the log says the opposite and would mislead support.
- Fix: record the copy in the marker inside the `copyRows` transaction (e.g. `m.RowsCopied = true`, written with `tx`), and skip `copyRows` on resume when it is set.

### L3. `ade_db_audit.call_id` has no index; every trim and cascade scans the table

- Where: `apps/kira-space/internal/storage/migrations/0081_p547_db_audit.sql:6,19`; trim at `apps/kira-space/internal/storage/repos/dbaudit.go:35-38`.
- Failure: `call_id REFERENCES ade_db_audit(id) ON DELETE CASCADE` has no index on the child column. Once a task passes 2000 calls, every `StartCall` deletes one call, and SQLite looks up its approvals with a full scan of `ade_db_audit`. The table holds every task's rows with no retention (round-1 L3, now a Known open item), so each audited tool call past the cap pays a scan that grows with all history.
- Fix: new migration `CREATE INDEX ade_db_audit_call ON ade_db_audit (call_id) WHERE call_id IS NOT NULL;`.

## Round-1 fixes checked, nothing real found

- `ade/paste.go` `normalizeMessage`: drops every C0 (except `\n`, `\t`), DEL and C1 rune, so ESC cannot survive and the nested end-of-paste cannot reassemble. 8-bit CSI (U+009B) is covered. Invalid UTF-8 becomes U+FFFD, harmless. Also used for the initial CLI prompt (`tracker.go:572`), where dropping controls is fine.
- `scripts/install.sh:335-339`: the Space-running check for `--app=studio` self-update runs before the hand-off, so Studio still owns the error. Interactive mode is still covered by step 10.
- 0078 comment and ARCHITECTURE rule: accurate. `PRAGMA foreign_keys` is a no-op in the migration tx; memory `0005_p274_ingest.sql` does use the TEMP-copy pattern. No table references `tabs` or `ade_task_handovers`, so 0077 and 0079 stay safe.
- `homefold.Reappeared`: after a fresh fold the legacy folder is gone, so no false warning. A legacy folder recreated by Studio warns every launch, as documented.
- `insertRows` counting: `RowsAffected` per `INSERT OR IGNORE` is correct (see L2 for the resume case).
- `AdeSessionsRepo.ListRunning`: served by `ade_sessions_task`. Dropping the `ListTask` scan also stops finished headless rows being counted unreachable; `match` already required `running`, so behaviour is unchanged.
- `dbaudit.refusalText`: no dialect guess; fixed text when the adapter cannot mask.
- 0077 comment: matches the data.
- `MemoryPanel.vue`: `VIRTUAL_ROW_CLASS` last lets `absolute` beat `rowVariants`' `relative` in tailwind-merge. Other call sites either put it last or merge with classes that carry no position.
- `crop.ts` region top/bottom trim and `shoot.ts`: arithmetic keeps the bottom on a top trim and sets height from the last bottom match.
- Shot specs and `tests/claude` helper fixes: test data only; `mustRaw` is still used elsewhere.

## Round-1 areas re-read, nothing new found

- Architecture, structure and security: nothing real beyond the above. Migrations 0078-0081, dbfold (pinned connection, deferred keys, marker in the import tx), configfold, DB MCP `SessionPool` (token lifecycle, release and close ordering), panel forward (size caps, `taskMu` serialization, sanitized paste) and the `install.sh` `--app` paths hold up.
- Functional correctness: L1 and L2 only.
- Performance: L3 only. Op log `db` rows keep their own prune (`PruneCategory`); 0080 indexes `op_log.connection_id` for its `SET NULL` key.
