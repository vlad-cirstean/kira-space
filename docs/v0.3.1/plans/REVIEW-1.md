# v0.3.1 code review, round 1

Base: `56a24c16e` (v0.3.0 squash merge on `main`; holds v0.3.0's review round 2 fixes, the last review session). Head: `187e698b0`. Scope: `git diff 56a24c16e 187e698b0` (291 files): P551 site, P552 STRICT schema and migrations 0083-0090, P553 sqlb conversion and N16, P554 configsync, CI and pending patches.

Findings: 0 High, 4 Medium, 4 Low.

## Medium

### M1. Config sync credential gate misses name-keyed literals

- `apps/kira-space/internal/configsync/gate.go:31-40`: `entryCredentialProblem` encodes only the entry value, so the key name is lost. In combined mode (agent default), entry `env/GITHUB_TOKEN: hunter2` reaches `scanNode` as a bare scalar; `credshape.Value("hunter2")` is false, so the entry is offered and written. The same text in shared mode is blocked by the `env` parent rule at `gate.go:65`. One unit, two verdicts.
- `gate.go:65`: the name rule fires only under `env` or `headers`. A shared `connections/*.yaml` with `password: hunter2` or `uri: postgres://u:p@h/db` passes: top-level parent is `""`, value is not credential-shaped. Connections and api have no module validator in configsync (`apps/kira-space/internal/appwire/configsync.go:39-59`, SPEC deviation), so nothing else stops it before `Apply` writes it and moves the base.
- Scenario: shared repo carries a literal secret in an agent env entry or a connection `password`; preview shows a clean add, Pull writes it to `<home>/config`. Breaks the P554 ask "no secrets in the shared repo".
- Fix direction: pass the entry id (`env/<name>`) into the entry check; apply `credshape.Name` to any key holding a literal (or to `x-kira-secret` schema fields) in the whole-file scan.

### M2. Studio fold into STRICT tables can fail every launch

- `apps/kira-space/internal/dbfold/dbfold.go:513`: module tables copy with `INSERT OR IGNORE` into the P552 STRICT tables. The fold applies none of the repairs migrations 0084-0090 apply in place (booleans, negative `sort_order`, port range, enum modes, invalid JSON). `OR IGNORE` silently drops a Studio row that fails a new CHECK.
- A dropped parent (e.g. a `connections` row with `port = 0` or `mcp_read_mode` outside the enum) leaves its children (`connection_mask_rules`, saved queries, filter history) inserted; `PRAGMA defer_foreign_keys = ON` (`dbfold.go:304`) defers the check to `COMMIT`, which then fails. A STRICT datatype mismatch aborts the statement outright (`OR IGNORE` does not cover it).
- `main.go:314-321` turns any fold error into `reporter.Fatal`, and the marker resumes the same import next launch: the app never starts. Contradicts P552 "never fail startup on row data". `dbfold` has no test.
- Fix direction: copy through the same repair expressions the migrations use (or stage into loose tables, then reuse the migration SELECTs), and quarantine dropped rows with their children.

### M3. Preview session dies on background fetch and on any second Preview

- `apps/kira-space/internal/configsync/fetch.go:175`: every successful fetch sets `s.sess = nil`, even when the fetched commit is unchanged. Background interval minimum is 5 min (`files.go:36`), preview TTL 10 min (`service.go:110`). A user reviewing for over one interval clicks Pull and gets `previewExpired` though nothing moved.
- `apply.go:66`: one global session; each `Preview` replaces it. With the review window open, Settings Pull (`ConfigSyncPane.vue:116`) mints a new token; the window's Pull then fails. A window refocus refetch (TanStack default) does the same to the pane.
- Fix direction: keep the session when the fetched sha is unchanged (takeSession already rechecks commit and file hashes); key sessions by token so each caller holds its own.

### M4. Build carousel auto-rotates with no pause control

- `site/src/components/BuildCarousel.astro:52-73,105-109`: slides advance every 6 s. The only pauses are hover, focus, touch, offscreen and hidden tab. No visible pause/stop button, which WCAG 2.2.2 (Pause, Stop, Hide) and the APG carousel pattern require for auto-rotation. A keyboard or magnifier user reading a slide without focus inside it loses it.
- Fix direction: add a rotation toggle button (first in tab order, `aria-label` switching play/pause), as APG shows.

## Low

### L1. Quarantine rows record the wrong migration

- `apps/kira-space/internal/storage/migrations/0085_p552_connections.sql` through `0090_p552_scripts_misc.sql` and `p552_sql.go`: every `schema_quarantine` insert writes `migration = 84`. `warnQuarantine` still works (`>= 84`), but the column no longer tells which step repaired or removed a row.

### L2. A failed boot recovery is overwritten by the next apply

- `apps/kira-space/main.go:98-100` only logs a `Recover` error. `rollbackFiles` keeps `apply.json` on failure (`apply.go:344-346`), but `Service.Apply` never checks for it: `writeJournal` (`apply.go:194`) replaces it. Files stay half-applied and the record of the interrupted apply is gone.
- Fix direction: refuse Preview/Apply while a journal exists, surfacing it as a status problem.

### L3. Apply and rollback reset config file modes to 0644

- `apps/kira-space/internal/configsync/service.go:160` and `apply.go:333` write with fixed `0o644` through `renameio.WriteFile`. A local file the user restricted (e.g. 0600) becomes world-readable after a pull or a rollback.

### L4. Repo change or removal races an in-flight fetch

- `SetRepo`/`RemoveRepo` (`service.go:295-350`) take `opMu` but not `fetchMu`. A fetch whose network phase finished before `RemoveRepo` then takes `opMu`, fails `readTree` on the deleted mirror and `setProblem` saves `state.json` (`fetch.go:181-182`), recreating `.sync` with a stale problem for an unconfigured repo. Same path after `SetRepo` to a new URL stores the old repo's failure.

## Checked, nothing real

- configsync paths: module names allowlisted, file names `^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`, symlink and submodule tree entries skipped; no traversal. Repo URL refuses credentials and leading `-`; branch regex blocks options; `GIT_ALLOW_PROTOCOL` set; `GIT_TERMINAL_PROMPT=0` from gitclient; askpass only via broker. No credential read, G5 holds.
- Journal: committed and applying crash points in `commit`/`finish`/`Recover` all settle.
- sqlb: no user input concatenated into SQL; fragments bind through `Var`. `$` sequences in fragments checked against go-sqlbuilder compile rules. `OffsetOnly` follows `OrderBy` at both callers. `Cached` arg order matches `opColumns` (25). `IsNotDistinctFrom`, `NotIn` on subqueries, `NULLIF` replacements (`nullableString`) and partial-index upserts verified.
- Migrations: FK-off marker on 0084-0090, rebuilt indexes present under new names, no AUTOINCREMENT, no triggers or views, connections and repo-config carry logic consistent with markers.
- Performance: op_log upsert and ADE live list built once; background fetch at 30 min default with capped backoff; migration timing measured in P552 (0.62 s at 50k op_log rows). Nothing real.

## Already tracked, not counted

- P551 install WebP images still missing (`site/src/assets/install/` absent): `installShots.ts` throws under `CI=true`, so Pages fails until the user supplies them. SPEC P551 row stays Pending for this.
