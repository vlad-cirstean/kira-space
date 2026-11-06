# v0.3.0 code review, round 1

Base: `736caa3e5` (v0.2.36 squash merge, PR #31). It carries the last review session: v0.2.36's two review rounds and their fixes (`docs/HISTORY.md`, chapter v0.2.36 block). Diff `736caa3e5..HEAD`: 149 commits, 2621 files, mostly the Space/Studio relocation.

Scope, by risk: homefold, configfold, dbfold, migrations 0077–0081, DB MCP session pool and audit trail, panel forward, `scripts/install.sh`, proof links and delete cascades, new stores. Relocated files (`apps/kira-studio/...` to `apps/kira-space/...`, rename-only hunks) were not re-reviewed.

Severity: High = data loss or security hole on a normal path; Medium = real failure on a reachable path; Low = narrow edge, hygiene or cost.

No High findings.

## Medium

### M1. Bracketed-paste guard can be bypassed; panel forward now pastes agent-authored and remote text

- Where: `apps/kira-space/internal/ade/paste.go:17-21` (`normalizeMessage`), reached from `apps/kira-space/internal/ade/panelforward.go:351` (`tr.Send`).
- Failure: `strings.ReplaceAll(s, "\x1b[201~", "")` makes one pass. The input `"\x1b[20\x1b[201~1~"` becomes `"\x1b[201~"` after the replace, which ends the paste block early. All bytes after it reach the Claude TUI as typed keys, including ESC sequences and the newline that submits. Other C0 controls (ESC, 0x03, 0x04) also pass through unchanged.
- Why it matters now: P546 forwards diffs whose text other parties wrote. Handover drafts come from agents (`handover.go:320`, `before` is the agent's body). Proof name and summary come from agents (`proof.go:95`). Link titles can come from `autoLinkTitle` on a fetched page (`links.go:121`). An agent or a remote page can plant the sequence. The user's next forward then injects keystrokes into another agent's session, for example a mode toggle or an extra prompt.
- Fix: strip every ESC (0x1b) and every other C0 control except `\n` and `\t` before wrapping, or loop the replace until the string stops changing. Add a unit test with the nested sequence. This is parser-shaped and hard to get right, so it meets the unit-test bar.

### M2. `--app=studio` self-update fails whenever Kira Space is running

- Where: `scripts/install.sh:353-361`.
- Failure: the old Studio updater runs with `--wait-pid/--notify-fd`, hands off, and Studio quits. Step 10 then runs `pgrep -xq "Kira Space"`. In the two-app world, users who still run Studio usually have Space open too. The script dies with "Kira Space was reopened before the update finished", which is false, and the cleanup trap reopens Kira Studio with a failure alert. The retired app's update path fails on the common case, and the message gives the user nothing to act on.
- Fix: for `--app=studio`, check `pgrep "Kira Space"` before the hand-off (step 9), while Studio can still show the error in its own dialog. Use a separate message ("Quit Kira Space, then update again"). Keep the post-hand-off "reopened" wording only for the `--app=space` self-update.

### M3. Migration 0078 tells future authors to use a foreign-key escape the runner does not have

- Where: `apps/kira-space/internal/storage/migrations/0078_p544_module_tables.sql:5-7` and `internal/sqlitex/sqlitex.go:174-193`.
- Failure: the comment says a later rebuild of `ade_tasks` "must run PRAGMA foreign_keys=OFF first (sqlitex runs it outside the tx), as the P543 rebuilds do". `sqlitex.Migrate` runs every step inside `db.Begin()`, and `PRAGMA foreign_keys` is a no-op inside a transaction. No migration uses the pragma; grep finds it only in this comment. The DSN sets `_foreign_keys=1`. A future `ade_tasks` rebuild that follows the comment would run `DROP TABLE ade_tasks` with keys on. That cascades and deletes every `proof_task_links`, `ade_db_audit`, `ade_task_handovers`, `ade_task_artifacts` and `ade_runs` row, and the migration still reports success.
- Fix: correct the comment now, either way. Option one: say rebuilds of a parent table are not possible through `sqlitex.Migrate`. Option two: add a real mechanism, such as a migration flag that makes the runner run `PRAGMA foreign_keys=OFF` before `Begin`, `PRAGMA foreign_key_check` before commit, and turn keys back on after. 0077 (drops and rebuilds `ade_task_handovers`) and 0079 (`tabs`) are safe today only because no table references them. Document that as the rule.

### M4. An old Kira Studio launched after the fold silently starts a new, never-folded home

- Where: `apps/kira-space/internal/homefold/files.go:27-62`, `homefold.go:28` (one-shot marker), `scripts/install.sh:393-423`.
- Failure: the fold renames `~/.kira-studio` into `~/.kira-space/backups/p539-*` and sets the marker to `done`. Only the `--app=studio` install path trashes `Kira Studio.app`. A user who updates through Space's own updater, or installs Space fresh, keeps Studio.app. Launching it recreates an empty `~/.kira-studio` and Studio works there normally. Space never folds again because the marker is `done`. Connections, scripts and saved queries made in that copy never reach Space, and nothing tells the user.
- Fix: at startup, after the marker is `done`, check whether `LegacyHome()` exists again and is non-empty. If so, show a one-time warning naming the folder, or fold it again into a new backup under the same rules. At minimum, log at warn level and add a Known open item in `docs/ARCHITECTURE.md`.

## Low

### L1. `homefold.Runs` merges Studio script collections into Space collections of the same name without telling the user

- Where: `apps/kira-space/internal/homefold/runs.go:351-367` with `INSERT OR IGNORE` (`runs.go:267`).
- Failure: `custom_script_collections` is keyed by `name` (migration 0064). A Studio collection whose name Space already has is ignored, and its `sort_order` offset is lost. Its scripts land in Space's collection. Rows are not lost, but the grouping changes silently. The ignore also hides any other key clash in the five tables, because a dropped row raises no error and no log.
- Fix: count `RowsAffected` per table against the rows read, and log a warning with the per-table difference, the way dbfold does for `proof_task_links` (`dbfold.go:405-412`).

### L2. Panel forward scans the full `ade_sessions` table on every edit and every refetch

- Where: `apps/kira-space/internal/ade/panelforward.go:89-97` (`ListTask()` returns every session row, all tasks, all history). Callers: `recordPanelEdit` on each user edit, and `PanelForward`, which the frontend refetches on every board and sessions push (`frontend/src/ade/v2/readSignals.ts:31,38`).
- Failure: cost grows with total session history, never pruned, and runs on each board push during agent activity.
- Fix: add a repo query `ListRunning(taskID)`: `WHERE task_id = ? AND state = 'running'`, which index use can serve. Use it for both the headless and the TUI halves.

### L3. Linked proofs and audit rows have no retention; tasks are never deleted

- Where: `apps/kira-space/internal/storage/repos/proof.go:403` (`pruneClosed` skips linked sessions), `repos/adetask.go:1609` (archive sets `archived_at` only), `repos/dbaudit.go:11,35` (cap per task, none global).
- Failure: every proof attached to a task keeps its `payload` BLOBs forever. Each task keeps up to 2000 audit calls plus their approvals. The cascades added in 0078/0081 never fire, because no code path deletes an `ade_tasks` row. The DB grows without bound over months of use.
- Fix: decide a retention rule for archived tasks, for example dropping proof payloads and audit rows N days after `archived_at`. Record it in `docs/ARCHITECTURE.md` "Known open items" if it is deferred.

### L4. Audit refusal masking falls back to Postgres literal rules for every dialect

- Where: `apps/kira-space/internal/dbaudit/recorder.go:319-321`.
- Failure: when the connection's redactor cannot mask (connection not live), `MaskSQLLiterals(text, PostgresDialect)` treats `"..."` as an identifier. In MySQL and MariaDB default mode, `"..."` is a string literal, so a value the refusal quotes inside double quotes is stored unmasked in `ade_db_audit.error`.
- Fix: pick the dialect from the connection kind (`ConnName` already resolves the connection). If the kind is unknown, store `"refused (message not recorded)"` rather than guess.

### L5. Migration 0077 mints note ids with `randomblob`, outside the id rule

- Where: `apps/kira-space/internal/storage/migrations/0077_p543_unified_notes.sql:25`.
- Failure: migrated note rows get `lower(hex(randomblob(16)))` ids. Kira keys are UUIDv7 (`internal/ids`, guards G6/N14). Legacy v4-shaped ids are accepted, so nothing breaks today. But these rows do not sort by time and do not match any shape the guards know.
- Fix: none needed for shipped data. If 0077 has not shipped outside this chapter branch, generate the ids in Go after the migration, or accept and note the exception in the migration comment.

## Areas checked with nothing real found

- configfold (`configfold.go`, `move.go`): marker, backup-before-move, EXDEV copy and verify, and dir merge are all resumable and repeat safely.
- dbfold (`dbfold.go`): both databases are backed up before any write. Legacy migrations run on the copy in place. Import is one transaction with deferred keys, and the marker is written inside the same transaction. Removal happens only after import. Resume after every step was traced.
- homefold Files and Runs resume: every step checks its own end state. `buildMaps` runs once, before the `runs` state is persisted. `INSERT OR IGNORE` makes a rerun of `copyRows` after commit harmless. The old app's lock is held across the move.
- Migrations 0079–0081: the 0079 tabs rebuild is safe because no table references `tabs`. The 0080 op_log fold uses TEXT ids, so no collision. 0081's audit cascades are correct.
- DB MCP session pool (`dbmcp/sessions.go`, `http.go`): tokens are 32-byte random values, hashed in memory only, and compared in constant time. Only session tokens are accepted on the session server. It listens on loopback on an ephemeral port. Revoking a token cancels its in-flight calls. Abandoning approvals is scoped by session. Grant and release are serialized under `p.mu`. The pool is not bound to the renderer (package func, `bridge/dbmcp.go:155`). Read lanes are left to the external instance.
- Audit trail secret handling (`dbaudit/recorder.go`, `dbmcp/audit.go`): statements are masked through the adapter or withheld. Adapter error messages are never stored. Summaries keep only allow-listed keys, and URLs lose credentials and query. The exemption reason is redacted and truncated. Trimming per task is inside the insert transaction.
- Panel forward size limits: the message is capped at 32 KiB with a 1 KiB reserve, plus 200 lines per item and 32 pending items. Edits recorded while a forward runs are serialized by `taskMu`, so no edit is lost between compose and clear.
- `install.sh` integrity: release asset size, sha256 digest, codesign, bundle id and version are checked before the swap. The legacy app is trashed only after the installed bundle passes those checks again and its bundle id matches.
