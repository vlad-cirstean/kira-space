# v0.3.1

Branch `chapter/v0.3.1`, based on `chapter/v0.3.0` (v0.3.0 must merge first). First `P` is 551 (highest in v0.3.0 is P550), last is P553.

Source: user request. Three phases: site expansion on the v0.3.0 image pipeline, SQL query builder for the app's own SQLite, tighter SQLite schema. Close duty after the last phase lands: one full suite run, 2 code review rounds, hand-written close. After the close and push, open the PR to main (CLAUDE.md close rules), subscribe to it, and fix failing checks until it merges. Each phase: Opus plan, Sonnet implementation.

Sequencing: P551 needs v0.3.0 P549 (WebP, frame) and P550 (image pipeline), both landed with the chapter base. P552 and P553 touch the DB layer and need v0.3.0 P544 (single merged DB) landed. Reading of the code: both touch `apps/kira-space/internal/storage/repos` and `storage/migrations` (modernc.org/sqlite v1.61.1; `foreign_keys` set in `storage/db.go`; no ORM, raw `sqlitex` helpers). P552 edits repo Go files and leaves migrations as SQL. P553 adds migration SQL files and tightens types the repos scan (bool, JSON, timestamps). Overlap is small but real, in repo scan code and in `db.go`. Default: P552 then P553, so P553 verifies against builder-based repos. Parallel allowed only if the P553 plan keeps every column name and Go type unchanged and lists per-file ownership with zero overlap. P551 shares no files with P552 or P553 and may run beside either. Up to 3 phases at once per CLAUDE.md batch rule; a batch plan goes under `plans/PARALLEL.md`. Table order is execution order otherwise.

| Phase | Title | Status |
|---|---|---|
| P551 | Site expansion: content, pages and explanations with images | Pending |
| P552 | Go SQL query builder for the app's SQLite | Pending |
| P553 | Tighten SQLite column types and constraints | Pending |

## Rows

### P551 Site expansion: content, pages and explanations with images

- Ask: improve the Astro presentation site. More functionality and explanations: module pages, how it works, workflows, MCP and agents, Studio modules, security and privacy, install and migration guides, changelog or feature index, better search and navigation. Images, screenshots and diagrams for many more scenarios wherever useful.
- Uses the image pipeline from v0.3.0 P550 (CI-generated shots, crop specs, `site/src/data/shots.ts` manifest) and the WebP and frame from v0.3.0 P549. New scenarios are added as shot specs and crop specs; this phase builds no pipeline.
- Decide in plan: scenario list; which shots generate in CI and which stay committed as WebP, within the P550 rules; content structure and navigation; copy style; naming and referencing of generated shots; site visual tests.
- Plan: `plans/P551.md`.

### P552 Go SQL query builder for the app's SQLite

- Ask: use an existing, well-maintained, fully open-source Go SQL query builder for the app's own SQLite (repos under `apps/kira-space/internal/storage` and `studiostorage/repos`, or their merged successor). Decline hand-rolling. Migrations stay SQL files.
- Design so a PostgreSQL backend can be added later. Keep in mind: dialect abstraction, placeholders, upsert and `RETURNING`, identifier quoting, type mapping (booleans, timestamps, JSON, blobs), transactions, migrations runner per dialect, concurrency and locking differences, pagination, full text, SQLite-only code (pragmas, rowid, `INSERT OR REPLACE`, `strftime`/`datetime`).
- Not in scope: Postgres support itself.
- Needs v0.3.0 P544 landed.
- Decide in plan: library and reasoning (license at package level; name any requirement that rules a candidate out); repo interface boundary; migration order, repo by repo, behaviour unchanged each step; where raw SQL that cannot be expressed stays isolated; test strategy (existing flow tests and adapter-free repo tests; no new unit tests unless complex); perf check on hot queries.
- Plan: `plans/P552.md`.

### P553 Tighten SQLite column types and constraints

- Ask: columns are loose today. Tighten the app DB schema: `STRICT` tables (SQLite 3.37+; check the version bundled by the driver), proper types, `NOT NULL`, `CHECK` (enums, ranges, booleans 0/1, `json_valid`), `UNIQUE`, foreign keys with explicit `ON DELETE`, defaults, indexes on foreign keys. Table-rebuild migrations copy data safely, back up first, handle rows that violate new constraints (repair, quarantine, or fail with a clear message). `PRAGMA foreign_keys` on.
- Depends on v0.3.0 P544.
- Decide in plan: audit of every table and column; constraint catalogue per table; migration strategy (per table or grouped; order by foreign key dependencies); verification (schema snapshot test is acceptable: it guards complex migration logic; migration flow test on a seeded old-shape DB); interplay with P552 (builder types) and a possible later Postgres schema; order relative to P552.
- Plan: `plans/P553.md`.

## Close

2 code review rounds unless the user says otherwise. After P553: one full suite run with all failures fixed, review rounds, hand-written close per CLAUDE.md, PR.

### Site and images check (mandatory)

Runs after P553 and the full suite run, before the code review. Close does not start until every item passes.

- [ ] Images use the v0.3.0 pipeline (P549, P550): WebP only, macOS 26 frame on full-window shots, every other site image cropped to its subject. New P551 scenarios generate in CI. Orchestrator views each new image.
- [ ] Site build with CI generation, from a clean checkout: generated shots appear, `site:build` and `site:check` pass.
- [ ] Site content. New pages, navigation and search work. Visual baselines re-recorded for the final UI. Install command, repo URLs, download links and migration notes correct. Redirects work.
- [ ] One app presented everywhere: Kira Space, Space modules first, then Studio modules. No page, shot or alt text shows two products or stale kira-studio wording.
- [ ] Docs match final behaviour: README, ARCHITECTURE (builder, schema constraints), PACKAGING, DEV_ENVIRONMENT (screenshots, site CI), site docs.
- [ ] `docs/pending-changes/*.patch` match the final state. Applied from a session with workflow scope.
