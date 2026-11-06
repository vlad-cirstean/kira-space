# v0.3.1

Branch `chapter/v0.3.1`, based on `chapter/v0.3.0` (v0.3.0 must merge first). First `P` is 551 (highest in v0.3.0 is P550), last is P554.

Source: user request. Four phases: site expansion on the v0.3.0 image pipeline, SQL query builder for the app's own SQLite, tighter SQLite schema, GitOps for config files. Close duty after the last phase lands: one full suite run, 2 code review rounds, hand-written close. After the close and push, open the PR to main (CLAUDE.md close rules), subscribe to it, and fix failing checks until it merges. Each phase: Opus plan, Sonnet implementation.

Sequencing: P551 needs v0.3.0 P549 (WebP, frame) and P550 (image pipeline), both landed with the chapter base. P552 and P553 touch the DB layer and need v0.3.0 P544 (single merged DB) landed. Reading of the code: both touch `apps/kira-space/internal/storage/repos` and `storage/migrations` (modernc.org/sqlite v1.61.1; `foreign_keys` set in `storage/db.go`; no ORM, raw `sqlitex` helpers). P552 edits repo Go files and leaves migrations as SQL. P553 adds migration SQL files and tightens types the repos scan (bool, JSON, timestamps). Overlap is small but real, in repo scan code and in `db.go`. Default: P552 then P553, so P553 verifies against builder-based repos. Parallel allowed only if the P553 plan keeps every column name and Go type unchanged and lists per-file ownership with zero overlap. P551 shares no files with P552 or P553 and may run beside either. Up to 3 phases at once per CLAUDE.md batch rule; a batch plan goes under `plans/PARALLEL.md`. P554 needs v0.3.0 P540 (unified `<home>/config`, landed). Config YAML is file-based, so P554 has no DB dependency and is independent of P552 and P553. Table order is execution order otherwise.

| Phase | Title | Status |
|---|---|---|
| P551 | Site expansion: content, pages and explanations with images | Pending |
| P552 | Go SQL query builder for the app's SQLite | Pending |
| P553 | Tighten SQLite column types and constraints | Pending |
| P554 | GitOps for config files: shared config repos | Pending |

## Rows

### P551 Site expansion: content, pages and explanations with images

- Ask: improve the Astro presentation site. More functionality and explanations: module pages, how it works, workflows, MCP and agents, Studio modules, security and privacy, install and migration guides, changelog or feature index, better search and navigation. Images, screenshots and diagrams for many more scenarios wherever useful.
- Uses the image pipeline from v0.3.0 P550 (CI-generated shots, crop specs, `site/src/data/shots.ts` manifest) and the WebP and frame from v0.3.0 P549. New scenarios are added as shot specs and crop specs; this phase builds no pipeline.
- Decide in plan: scenario list; which shots generate in CI and which stay committed as WebP, within the P550 rules; content structure and navigation; copy style; naming and referencing of generated shots; site visual tests.
- Ask, Build with Kira story: site section explaining how building an app works in Kira Space. Each of five steps is a SECTION of the page; its panel has many interesting options, so each section shows MANY images: target 20 to 30 shots total across the five sections, hard cap 30, each section showing the panel's real options. Steps: (1) Start a planned task over a workflow. (2) A DB prompt. (3) Branches, handover and artifacts. (4) The review window. (5) The proof window.
- (a) Top overview carousel: one image per step, each slide names the step and links to its section. Arrows (mouse, keyboard, touch) and dots; no visible play/pause control. Autoplays on a timer; pauses on hover, focus and touch, and when tab or carousel is off-screen; resumes after; no autoplay under `prefers-reduced-motion`; arrows and dots still work.
- (b) Below it, five sections in order. Each has a clear name, short explanation, anchor id (`#start-task`, `#db-prompt`, `#branches-handover`, `#review`, `#proof`; plan may rename). Each opens with engaging copy: the problem it solves for the user, as a short pitch. Then the images (about 4 to 6 per section, 20 to 30 total, hard cap 30) showing the panel's real options, as a gallery, each with a one-line caption.
- Shots may be FAKED: CI cannot run real `claude`. Use staged fixtures or stubs so all shots read as one story (same task, branch, workflow, DB across sections). Need not be a real flow. Each image picks full-window (framed) or cropped, whichever reads best; at least some are full-window (e.g. review window, proof window, planned-task/workflow start). Shots come from the v0.3.0 P550 pipeline (`shotCapture.ts` manifest, crop kinds window/floating/region, or full window), generated in CI, none committed.
- Decide in plan, Build with Kira: section page layout (sticky step nav vs plain anchors; phone layout); overview carousel implementation (Astro island with Vue or small lib, vs pure CSS scroll-snap plus minimal JS; prefer a well-maintained open-source lib or CSS scroll-snap plus arrows); autoplay interval and pause rules; reduced-motion; gallery component per section (small in-section carousel vs grid with captions; one reusable component); images per section and total (about 4 to 6 each, 20 to 30 total, hard cap 30; note the P550 cache key and fallback); per-image full-window vs cropped, with why; one frame aspect ratio or mixed ratios without layout jump; shared scenario fixture for all shots; step copy, captions and i18n/content location (one typed data file keyed by section, beside `shots.ts`); **Shot generation speed**: evaluate and pick from: one build and one browser per CI shard; several shots per scenario session (drive the UI through states and capture repeatedly, no reboot per shot); mock-bridge/fixture-runtime FE-only mode (no Go backend, no real `claude`; mocked data that looks as the real app would show it, nothing needs to be real); shared storage state and seed; parallel Playwright workers and a CI shard matrix; per-shot cache keyed on its import closure (extends P550's key, so only changed shots regenerate); WebKit only; skip animation and font warmup via `prepareCapture`; time budget target under 5 minutes per shard. The plan states measured timing expectations (per-shot, per-session, per-shard, total on a cold and a warm cache) and how they were measured; tests (site spec asserts 5 named sections, anchors resolve, overview arrows move slides, slide links reach sections, images load).
- Ask, Install notice: `site/src/components/Install.astro` must state clearly: (1) app is beta, not production ready; (2) app is not notarized (macOS). Explain what no notarization means: user must (a) grant app access to its OWN key in Keychain (not other apps' keys), (b) grant access to working folders.
- Install images, USER-SUPPLIED (real macOS system dialogs, CI cannot generate): (1) Keychain access prompt, (2) folder-access prompt. Spec: WebP, hand-added under `site/src/assets/install/`, cropped to the dialog, alt text each. Exempt from lint N15: N15 bans tracked PNG under `screenshots/` and `site/`, and WebP under `screenshots/` only, so WebP under `site/src/assets/install/` already passes. Widen `scripts/check-conventions.ts` only if the plan picks another path or format. N15 message says "generated, not committed"; reword it to name the install exemption.
- Install images, missing-file rule: plan lists both as an input the user provides. Dev build shows a visible "screenshot pending" placeholder. CI build (`CI=true`) fails while either file is missing. Plan documents the check and where it lives.
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

### P554 GitOps for config files: shared config repos

- Ask: config files are unified under `<home>/config` (v0.3.0 P540). Add gitops support: every such resource can be local or shared. Some, like agent configs, can be combined (local + shared merged), or each entry can be deactivated individually. Settings gets an option to add a config repo. The app periodically fetches the repo for changes and shows a toast that new configs are available. Pressing the toast opens a review window comparing current config to what a pull would apply (diff per resource). If the user likes the changes they ask Kira to pull, which applies them. Settings also shows that changes exist and has the pull button, next to where the repo is configured.
- Needs v0.3.0 P540 (landed). Independent of P552 and P553: config YAML is file-based, no DB dependency.
- Decide in plan: repo layout and manifest of resources per module; local-vs-shared precedence and combine rules per module (agent configs combine; others override or exclusive); per-entry deactivate storage; fetch interval and auth (reuse existing git machinery, never touch CLI credentials; no secrets in the shared repo: connections and secrets stay local or are referenced); conflict handling when local edits diverge from shared; apply atomicity and rollback or backup (reuse the P540 backup approach); where the review window diff reuses existing diff components; multiple repos allowed or one; tests per the flow-test + UI-spec split rule (same scenario once on backend, once on frontend).
- Plan: `plans/P554.md`.

## Close

2 code review rounds unless the user says otherwise. After P554: one full suite run with all failures fixed, review rounds, hand-written close per CLAUDE.md, PR.

### Site and images check (mandatory)

Runs after P554 and the full suite run, before the code review. Close does not start until every item passes.

- [ ] Images use the v0.3.0 pipeline (P549, P550): WebP only, macOS 26 frame on full-window shots, every other site image cropped to its subject. New P551 scenarios generate in CI. Orchestrator views each new image.
- [ ] Site build with CI generation, from a clean checkout: generated shots appear, `site:build` and `site:check` pass.
- [ ] Site content. New pages, navigation and search work. Visual baselines re-recorded for the final UI. Install command, repo URLs, download links and migration notes correct. Redirects work.
- [ ] Build with Kira (P551): overview carousel shows 5 named steps, each slide links to its section; arrows work (mouse, keyboard, touch); autoplays, pauses on hover/focus/touch and when tab or carousel is off-screen, resumes after, off under reduced-motion; no visible play/pause control. Five sections in order, each with name, problem-pitch copy, anchor and captioned images; 20 to 30 images in total (cap 30), generated within the plan's time budget (target under 5 minutes per CI shard); mixed full-window and cropped shots, no layout jump; images load and read as one story (same task, branch, workflow, DB). Orchestrator views every image (all 20 to 30).
- [ ] Install notice (P551): `Install.astro` states beta and not notarized, with Keychain own-key and working-folder access explained. Both user-supplied WebP images present under `site/src/assets/install/`, cropped to the dialog, with alt text; CI build passes without placeholder. Orchestrator views both.
- [ ] One app presented everywhere: Kira Space, Space modules first, then Studio modules. No page, shot or alt text shows two products or stale kira-studio wording.
- [ ] Docs match final behaviour: README, ARCHITECTURE (builder, schema constraints), PACKAGING, DEV_ENVIRONMENT (screenshots, site CI), site docs.
- [ ] `docs/pending-changes/*.patch` match the final state. Applied from a session with workflow scope.
- [ ] GitOps config (P554): add repo in Settings, periodic fetch toast, review window diff per resource, pull applies atomically with backup. Combine and per-entry deactivate work for agent configs. No secrets in the shared repo. Flow test and UI spec both pass.
