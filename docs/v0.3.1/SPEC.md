# v0.3.1

Branch `chapter/v0.3.1`, based on `chapter/v0.3.0` (v0.3.0 must merge first). First `P` is 549 (highest in v0.3.0 is P548), last is P552.

Source: user request. Four phases: screenshots stored as WebP, site expansion with CI-generated images, SQL query builder for the app's own SQLite, tighter SQLite schema. Close duty after the last phase lands: one full suite run, 2 code review rounds, hand-written close. After the close and push, open the PR to main (CLAUDE.md close rules), subscribe to it, and fix failing checks until it merges. Each phase: Opus plan, Sonnet implementation.

Sequencing: P549 then P550 are a chain: P550 needs the WebP pipeline. P551 and P552 touch the DB layer and need v0.3.0 P544 (single merged DB) landed. Reading of the code: both touch `apps/kira-space/internal/storage/repos` and `storage/migrations` (modernc.org/sqlite v1.61.1; `foreign_keys` set in `storage/db.go`; no ORM, raw `sqlitex` helpers). P551 edits repo Go files and leaves migrations as SQL. P552 adds migration SQL files and tightens types the repos scan (bool, JSON, timestamps). Overlap is small but real, in repo scan code and in `db.go`. Default: P551 then P552, so P552 verifies against builder-based repos. Parallel allowed only if the P552 plan keeps every column name and Go type unchanged and lists per-file ownership with zero overlap. P550 and P549 share no files with P551 or P552: once P549 lands, P550 may run beside P551 or P552. Up to 3 phases at once per CLAUDE.md batch rule; a batch plan goes under `plans/PARALLEL.md`. Table order is execution order otherwise.

| Phase | Title | Status |
|---|---|---|
| P549 | Screenshots stored as lossless WebP | Pending |
| P550 | Site expansion with CI-generated images | Pending |
| P551 | Go SQL query builder for the app's SQLite | Pending |
| P552 | Tighten SQLite column types and constraints | Pending |

## Rows

### P549 Screenshots stored as lossless WebP

- Ask: product screenshots stored as lossless WebP only. Measured: a 150 KB PNG becomes 47 KB, identical pixels; quality 90 is no smaller. Playwright's WebKit `screenshots` project still captures PNG as an intermediate. The framed-variant post-processing step converts to lossless WebP and deletes the PNG, so only `.webp` is committed. Update `site/src/data/shots.ts` and every site reference, site visual baselines, docs (`docs/DEV_ENVIRONMENT.md`, screenshots README) and every check that globs PNG names.
- Ask: macOS window controls (traffic-light "semaphore" buttons) were redesigned in macOS 26. Framed screenshots must show the new look. Frame code: `frame()` in `packages/workbench/src/testing/screenshots/capture.ts` (HTML frame, 10px radius, 1px inset border) plus `screenshots/assets/traffic-lights.svg` (52x12 at 14,11). All framed shots under `screenshots/` and site assets regenerate with the new frame.
- Decide in plan (frame): size, spacing, colours, window corner radius, shadow or glass treatment to match macOS 26. State the reference used. No macOS in the sandbox: if no reference can be verified, the plan says so and the user supplies a reference screenshot.
- Not in scope: Astro image optimisation, responsive sizes, AVIF, fallbacks. User is not optimising Astro yet.
- Decide in plan: converter (library already in the toolchain, for example sharp; license check, fully open source); lossless settings; which existing PNGs migrate (`screenshots/`, site assets); how git history growth stays small (PNGs deleted, no new binary except WebP); guard against PNG left behind.
- Plan: `plans/P549.md`.

### P550 Site expansion with CI-generated images

- Ask: improve the Astro presentation site. More functionality and explanations: module pages, how it works, workflows, MCP and agents, Studio modules, security and privacy, install and migration guides, changelog or feature index, better search and navigation. Images, screenshots and diagrams for many more scenarios wherever useful.
- The site publishing GitHub Action gains a build step: run the `screenshots` Playwright project against the app build before the site build, so images follow the real UI and need not all be committed. Workflow changes cannot be pushed from this environment: patches go to `docs/pending-workflows/` per `docs/DEV_ENVIRONMENT.md`.
- Ask: site images are cropped to what matters. Full-window shots only where the whole window makes sense: overview, hero, layout, module switching. A dialog, popover, menu, panel or toolbar region is captured cropped to that element (element screenshot with small padding, not the whole window), with at most a light shadow instead of the full window frame.
- Depends on P549.
- Decide in plan (crops): capture mechanism (Playwright locator screenshots, per-shot crop regions, padding, scale 2); naming; which scenarios are full vs cropped; frames apply to full-window shots only; how cropped shots keep consistent width and alignment in the page layout.
- Decide in plan: scenario list; which shots generate in CI and which stay committed as WebP; CI cost and caching (hash of app source and shot specs); flake handling so one bad shot does not break the deploy; content structure and navigation; copy style; naming and referencing of generated shots; site visual tests.
- Plan: `plans/P550.md`.

### P551 Go SQL query builder for the app's SQLite

- Ask: use an existing, well-maintained, fully open-source Go SQL query builder for the app's own SQLite (repos under `apps/kira-space/internal/storage` and `studiostorage/repos`, or their merged successor). Decline hand-rolling. Migrations stay SQL files.
- Design so a PostgreSQL backend can be added later. Keep in mind: dialect abstraction, placeholders, upsert and `RETURNING`, identifier quoting, type mapping (booleans, timestamps, JSON, blobs), transactions, migrations runner per dialect, concurrency and locking differences, pagination, full text, SQLite-only code (pragmas, rowid, `INSERT OR REPLACE`, `strftime`/`datetime`).
- Not in scope: Postgres support itself.
- Needs v0.3.0 P544 landed.
- Decide in plan: library and reasoning (license at package level; name any requirement that rules a candidate out); repo interface boundary; migration order, repo by repo, behaviour unchanged each step; where raw SQL that cannot be expressed stays isolated; test strategy (existing flow tests and adapter-free repo tests; no new unit tests unless complex); perf check on hot queries.
- Plan: `plans/P551.md`.

### P552 Tighten SQLite column types and constraints

- Ask: columns are loose today. Tighten the app DB schema: `STRICT` tables (SQLite 3.37+; check the version bundled by the driver), proper types, `NOT NULL`, `CHECK` (enums, ranges, booleans 0/1, `json_valid`), `UNIQUE`, foreign keys with explicit `ON DELETE`, defaults, indexes on foreign keys. Table-rebuild migrations copy data safely, back up first, handle rows that violate new constraints (repair, quarantine, or fail with a clear message). `PRAGMA foreign_keys` on.
- Depends on v0.3.0 P544.
- Decide in plan: audit of every table and column; constraint catalogue per table; migration strategy (per table or grouped; order by foreign key dependencies); verification (schema snapshot test is acceptable: it guards complex migration logic; migration flow test on a seeded old-shape DB); interplay with P551 (builder types) and a possible later Postgres schema; order relative to P551.
- Plan: `plans/P552.md`.

## Close

2 code review rounds unless the user says otherwise. After P552: one full suite run with all failures fixed, review rounds, hand-written close per CLAUDE.md, PR.

### Site and images check (mandatory)

Runs after P552 and the full suite run, before the code review. Close does not start until every item passes.

- [ ] Screenshots are WebP. `git ls-files '*.png'` lists no product screenshot under `screenshots/` or `site/`. No PNG intermediate committed. `bun run screenshots` regenerates every shot as `.webp`; `git status --porcelain screenshots/` is empty on rerun. The orchestrator views each image.
- [ ] Frame matches macOS 26 on every framed shot (traffic lights, corner radius, shadow). Orchestrator views each framed image against the plan's stated reference.
- [ ] Every site image is either an intended full-window shot or cropped to its subject. No full-window shot of a dialog, popover, menu or panel. Orchestrator views each image.
- [ ] Site build with CI generation. The pending workflow patch in `docs/pending-workflows/` runs the screenshots project before `site:build`. Run the same steps locally from a clean checkout: generated shots appear, `site:build` and `site:check` pass. A failing shot does not break the build, per the P550 flake rule. Cache key covers app source and shot specs.
- [ ] Site content. New pages, navigation and search work. Visual baselines re-recorded for the final UI. Install command, repo URLs, download links and migration notes correct. Redirects work.
- [ ] One app presented everywhere: Kira Space, Space modules first, then Studio modules. No page, shot or alt text shows two products or stale kira-studio wording.
- [ ] Docs match final behaviour: README, ARCHITECTURE (builder, schema constraints, WebP), PACKAGING, DEV_ENVIRONMENT (screenshots, site CI), site docs.
- [ ] `docs/pending-workflows/*.patch` match the final state. Applied from a session with workflow scope.
