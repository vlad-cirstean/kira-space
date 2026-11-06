# v0.3.0

Branch `chapter/v0.3.0`, based on `chapter/v0.2.36` (v0.2.36 not yet merged to main). First `P` is 537 (highest in v0.2.36 is P536), last is P547.

Source: user request. Eleven phases (P537 split into two Parts): merge both apps into one Kira Space, then fold the two homes into one, move config YAML into one subfolder, merge the main SQLite DBs, then proof, notes and timeline rework in the task details panel, then Kira DB audit trail and the Studio MCP workflow option. Close duty after the last phase lands: one full suite run, 2 code review rounds (user ask), hand-written close. After the close and push, open the PR to main (CLAUDE.md close rules), subscribe to it, and fix failing checks until it merges. Each phase: Opus plan, Sonnet implementation.

Sequencing: P537 first. Every later phase builds on the merged app, so none starts before P537 (all its Parts) is committed. P538 to P540 run in order: home first, then config subfolder, then DB merge; each migration runs after the previous one landed. Table order is execution order. P542 to P545 share the task details panel and the notes editor; run one at a time. P546 also depends on v0.2.36 P528 (exemption approvals) having landed.

| Phase | Title | Status |
|---|---|---|
| P537 Part 1 | Merge Kira Space and Kira Studio: relocate Studio into `apps/kira-space` and freeze the merged bridge | Done |
| P537 Part 2 | Merge Kira Space and Kira Studio: finish in three parallel streams (Go, frontend, build/tests/docs) | Done |
| P538 | Fold the Studio home folder into the Kira Space home folder | Pending |
| P539 | Move every config YAML into one subfolder of the home | Pending |
| P540 | Merge the main SQLite databases into one | Pending |
| P541 | Mermaid visualiser in the proof tool and proof MCP tool | Pending |
| P542 | Proof as an artifact chain in the task details panel | Pending |
| P543 | Unified notes: user notes and agent handovers in one chain | Pending |
| P544 | Timeline becomes a toggle merged into the same list | Pending |
| P545 | Forward panel edits (diff only) to agents running in the step | Pending |
| P546 | Kira DB audit trail in the details panel | Pending |
| P547 | Studio MCP option in workflow steps | Pending |

## Rows

### P537 shared ask (both Parts): Merge Kira Space and Kira Studio into one app named Kira Space

- Ask: merge both apps (`apps/kira-space` and `apps/kira-studio`) into one app named Kira Space. Module order in the app: the existing Kira Space modules first, then the Studio modules.
- Decide in plan: split into Parts if needed (CLAUDE.md Part rule: `P537 Part 1: …`, each with own plan, pass and Result); Go module and `main.go` merge, shared packages already duplicated (`appcore`, `appshell`, `appwire`, `bridge`, `config`, `storage`, `oplog`, `flowharness`, `flows`, `buildinfo`, `layering_test.go`); one frontend, one bridge, binding generation; Taskfile, build and packaging (`docs/PACKAGING.md`), CI and test layout (`tests/`, playwright configs); DB, storage and config migration for existing installs of both apps; what happens to the Studio MCP server (`internal/dbmcp`) and the `kira-db` naming; docs (`ARCHITECTURE.md`, `DEV_ENVIRONMENT.md`, READMEs) updated.
- Plan: `plans/P537.md` (decisions shared by both Parts). Split into two Parts (couplings in `P537.md`).

### P537 Part 1: Merge Kira Space and Kira Studio: relocate Studio into `apps/kira-space` and freeze the merged bridge

- Enabling step, one sequential implementer. Every Studio file moves to its final path; every path reference is rewired; the Go bridge merges to its final bound surface (one binary, one bindings tree); Studio source typechecks in Space's frontend project. Writes the rename table Part 2 builds on.
- Plan: `plans/P537-part1.md`.

Result:

- Bound surface: 62 services, 178 files in `apps/kira-space/frontend/bindings` (519 methods). Count equals Space's plus Studio's minus 15 merged.
- Bindings file-list hash: sha256 of `find . -type f | LC_ALL=C sort` run in `apps/kira-space/frontend/bindings` = `625c313f28c4ab100dd545eac18ef7c76b60c676973f0a3aa252f94fc54b4472`.
- Service rename table (Studio name, merged name):
  - One service, Space's name: Agent, Approvals, Prompts, Files, KeepAwake, Layout, Lifecycle, Link, Windows, Update, Terminal, LlmPrompts, Tabs, CustomScripts, ScriptRuns, App, Docker, Kubernetes, Proof. Tabs, CustomScripts and ScriptRuns route over both stacks. Layout is Space's.
  - Twins: `OpsService` to `StudioOpsService`, `SettingsService` to `StudioSettingsService`, `ConfigFilesService` to `StudioConfigFilesService`.
  - Studio-only services keep their names.
- Renamed bridge types: `StudioOpsRecentArgs`, `StudioOpsCancelArgs`, `StudioCanceller`, `StudioSettingsSetArgs`, `StudioSources`.
- Renamed channels: `kira:settings:changed` to `studio:kira:settings:changed`, `kira:op:update` to `studio:kira:op:update`. TS keys `CHANNEL.studioSettingsChanged`, `CHANNEL.studioOpUpdate` in `packages/shared/protocol/events.ts`. `createCoreControl` takes a `channels` arg.
- Studio tab-kind set (`studiostorage/model.IsStudioTabKind`): data, definition, console, document, keyvalue, stream, browse, http-request, grpc-request, variable-set, environments. `terminal` and `script-run` go to Space DB.
- Contract-key renames for Stream A: `OpsService.Recent` to `StudioOpsService.Recent` (and likewise `Cancel`, `SettingsService.*`, `ConfigFilesService.*`) in `tests/contract/studio` JSON and Go keys. Contract dir is `apps/kira-space/tests/contract/studio`.
- Go layout: Studio packages without Space twin sit as `apps/kira-space/internal/X`; Studio storage is `studiostorage`; `studioflowharness`, `studioflows` stay until Stream A. `tests/claude-studio` is a separate nested module.
- Frontend layout: `apps/kira-space/frontend/src/studio`, tests in `tests/<suite>/studio`, `ipc`/`proto`/`support` in `tests/<suite>/`. Configs: `playwright.studio.config.ts`, `playwright.studio-perf.config.ts`, `playwright.proto.config.ts`; Space's own configs ignore `studio/**`. Scripts: `*:studio` kept, `dev:studio`, `build:studio`, `package:studio` and Studio-only typecheck scripts removed.
- Deviations from `P537.md`:
  - `wails/runtime.js` and `proto/` moved into Space's frontend (Space had no `wails/` dir).
  - `connectionId` is `omitempty` in Space's `TabRecord`.
  - Space `WindowModes.Valid` extended with `studio`, `api`, `docker`, `kubernetes`. Space Layout lacks Studio's operations and cellEditor panel fields.
  - Merged Terminal composes Studio launches; id-collision routing in Tabs, CustomScripts and ScriptRuns picks the Studio stack by id lookup.
  - `CustomScriptsService.Publish` got `//wails:ignore` (non-empty interface param).
  - `unit/studio/mock-runtime-bindings.spec.ts`: "every bound method mapped" check limited to services Space's table does not map.
  - Mutation area `kira-studio` merged into `kira-space` in `tools/mutation/areas.json`; `scripts/test-ui-changed.sh` cross-app import check removed.
  - Minimal path-only edits in `site/` (source refs in `docs/studio/*`, `reference/*`, icon imports now `apps/kira-space/build`) so `check-docs` and the site build pass. Prose untouched; Stream C owns the rest.
  - biome `noRestrictedImports` group `**/apps/kira-studio/**` now `**/frontend/src/studio/**`. `knip.json` workspaces merged.
  - Proto preview port 9246 unchanged.
  - Fixed pre-existing `tests/claude/review_test.go` vet error and two lint findings in merged code.

### P537 Part 2: Merge Kira Space and Kira Studio: finish in three parallel streams (Go, frontend, build/tests/docs)

- Three named streams off Part 1's last commit (user raised the stream cap to 3): A Go (flow harness and flows merge, claude tests, first-launch migration, old-Studio guard, copy); B frontend (one entry, module order, stores, settings dialog); C build, packaging, CI, playwright layout, docs. Zero file overlap, no ordering dependency; landing order and checks in the plan.
- Depends on P537 Part 1.
- Plan: `plans/P537-part2.md`.
- Notes: `plans/P537-part2-{a,b,c}.md`.
- Result: Done. Streams landed fast-forward in order A, B, C, no conflicts (rebased onto chapter branch each time). Stream tips after rebase: A `42568c8b`, B `832a8fab`, C `9020562e`. Worktrees and stream branches removed.
- Landing check: `bun run setup`, `go build ./...`, `go build -tags server ./apps/kira-space/...`, `go vet ./...`, `lint:go` (0 issues), `lint`, `typecheck`, `lint:dead` all pass on merged tree. `playwright --list` works for every config (main 2083 tests, perf 103, proto 6). `test:claude:db` test names all exist in merged `tests/claude`. Bindings: 62 services, 519 methods, identical to Part 1 snapshot. No landing-check fixes needed: knip had no stale `src/studio/main.ts` entry (Stream C removed it); `tests/unit/studio/go-ts-vocabulary-parity.spec.ts` still passes (Go `studiostorage` list not moved). Full suites not run (chapter end). Pre-push hook passed.
- Deviations: `appmerge` skips backup and marker work when Studio side holds no rows; Studio-only stack tests deleted (`TestPreviewRefusesTaskInStudio`, `TestStudioRefusesTaskContext`); Studio panes for app-wide keys dropped, two settings stores stay until P540; site is one landing page, `/space` redirects to `/`. Details in the notes files.
- Known open items: apply `docs/pending-changes/*.patch` to `.github/` from a session with workflow scope. Studio visual baselines change; re-record at chapter-end suite run. `apps/kira-studio` remains only in `.github/` (pending patches), `docs/HISTORY.md`, `CLAUDE.md`, and frozen eval data under `apps/kira-space/tests/claude/testdata/memory-eval/`. `~/.kira-studio` merge is P538 to P540.

### P538 Fold the Studio home folder into the Kira Space home folder

- Ask: the two apps are merged (P537), so their home folders merge too. Studio home `~/.kira-studio` (`KIRA_HOME`) folds into Space home `~/.kira-space` (`KIRA_SPACE_HOME`). All SQLite DBs and config YAML live under the one Kira Space home.
- Scope: one path resolver (`kirapaths`, both `config/paths.go` wrappers collapse); logs, tokens (for example `mcp-db-token.json`), `llm-prompts.json`, `kira.db`, `connections/`, `api/`, `runs/` and the rest of the Studio home move. One-time, idempotent migration of existing user data with a backup first and no loss; a half-finished run resumes cleanly.
- Decide in plan: audit the real file list of both homes (grep `kirapaths`, `config.KiraHome`, `KiraSpaceHome`); env var handling (`KIRA_HOME` retired, deprecated alias, or honoured once for migration; dev and test overrides); name collisions between the two homes (`logs/`, `llm-prompts.json`, `app.lock`); backup location and retention; what happens to the old folder after a verified move; shared dirs outside both homes (`kirapaths.SharedAgentDir`, `~/.kira-memory`) stay out unless the plan names a reason; `dbmcp` home seam; update `docs/PACKAGING.md`, `docs/DEV_ENVIRONMENT.md`, `docs/ARCHITECTURE.md` and test fixtures (Go, Playwright, `tests/`).
- Plan: `plans/P538.md`.

### P539 Move every config YAML into one subfolder of the home

- Ask: all config YAML folders move under one subfolder of the Kira Space home (for example `<home>/config/<module>/`; plan decides the name), so the folder can later be a git repo. Git ops are out of scope: no `git init`, no commit, no sync, no UI for it.
- Scope: connections, api (collections, environments), runs, workflows, steps, repos, `agent/settings.yaml`, `agent/context.md` and every other config file. Generated `AGENTS.md` files stay inside the subfolder. DB files, logs, tokens and secrets never go in it (nothing in it may need `.gitignore` to stay safe). Migration of existing layouts, idempotent, backup first; watchers and write paths updated.
- Decide in plan: audit the real list (grep `configfs.Folder`, `connfiles`, `apifiles`, `adeflow.Dir`/`StepsDir`, `repocfg`, `agentcfg`, `kirapaths`); subfolder name and per-module layout; files that look like config but hold secrets or runtime state and stay out (for example `agent-secrets.db`, the sealed key material, `app.lock`); `configfs` watcher root and the write-suppression of own writes; docs-in-folder files (`connfiles/docs.go`); `docs/ARCHITECTURE.md` and `docs/DEV_ENVIRONMENT.md` updates; test fixtures.
- Depends on P538.
- Plan: `plans/P539.md`.

### P540 Merge the main SQLite databases into one

- Ask: practically merge the SQLite DBs into one main DB with one migration sequence. Some stay isolated for good reasons; the main ones become one.
- Scope: Studio `kira.db` and Space `kira.db` become one. Audit every other DB (`review.db`, `agent-secrets.db`, `memory.db` under `~/.kira-memory`, op log, any other `*.db` found by grep on `storage`, `oplog`, `OpenAt`) and decide per DB: merge or stay isolated, with the reason written in the plan (candidates: secrets sealed separately; memory shared with the stdio server; `review.db` size or lifecycle). Data migration of existing user DBs, idempotent, backup first.
- Decide in plan: per-DB verdict table; merged schema and migration numbering (one sequence, how two existing histories fold in); table-name collisions and renames; copy path for existing Studio and Space DBs, including a user holding only one of them; single-writer and `SetMaxOpenConns` effects of one DB serving both subsystems (lock contention, long transactions, WAL); repository and flow-test fixtures; split into Parts per CLAUDE.md if needed (for example Part 1 schema and migrations, Part 2 data migration and cut-over).
- Depends on P538.
- Plan: `plans/P540.md`.

### P541 Mermaid visualiser in the proof tool and proof MCP tool

- Ask: Mermaid JS visualiser in the proof tool (`internal/proof`, `frontend/src/proof`). The proof MCP tool (`dbmcp/proof_tools.go`) offers that option: the agent can submit a Mermaid diagram as proof.
- Decide in plan: Mermaid library adoption and license check; render safety (sandbox, strict security level, size cap); invalid diagram handling and what the agent is told; storage shape for a diagram proof; tool schema and description text in the llmprompts registry.
- Plan: `plans/P541.md`.

### P542 Proof as an artifact chain in the task details panel

- Ask: proof becomes an artifact in the task details panel. Result is a chain of proof. Links of interest stay at the top, grouped. Everything else is in order (for example diagram, then handover, then other proof) and shown in that order.
- Handover text is terse. Add a short rule for this in the MCP tool description (llmprompts).
- Decide in plan: chain ordering key and model; where the artifact lives relative to `AdeDetailsTab.vue` and `AdeLinkGroups.vue`; how existing proof records migrate into the chain; exact wording and budget of the terse-handover rule.
- Plan: `plans/P542.md`.

### P543 Unified notes: user notes and agent handovers in one chain

- Ask: manual notes unified. The notes panel is hidden by default; a button reveals it so the user can write. User notes appear in the same ordered chain as agent handovers (a handover from the user): one unified model. Adding and editing any note uses the same rich text editor everywhere. Sorted by time, newest first.
- Touches `notes/AdeHandoverList.vue`, `notes/AdeNotesEditor.vue`, `notesExtensions.ts` and the stage handover prompt.
- Decide in plan: data model and migration of existing manual notes and handovers; author marker (user or agent); which editor instances get replaced; hidden-state persistence.
- Plan: `plans/P543.md`.

### P544 Timeline becomes a toggle merged into the same list

- Ask: timeline is no longer a collapsible; it is a toggle. Its items show in the same time order in the same list.
- Touches `details/AdeTaskTimeline.vue` and `taskTimeline.ts`.
- Decide in plan: merged-list item model with P542 and P543 entries; default toggle state and persistence; sort direction consistent with the notes (newest first).
- Plan: `plans/P544.md`.

### P545 Forward panel edits (diff only) to agents running in the step

- Ask: every edit in the panel offers forwarding the change, only the diff, to the agents running in that step.
- Decide in plan: which edits count (notes, links, proof, others in the panel); diff format; how running agents in the step are found and addressed; offer UI (per-edit prompt versus per-save control); no forward when no agent is running.
- Plan: `plans/P545.md`.

### P546 Kira DB audit trail in the details panel

- Ask: when Kira DB is used from a session in Kira Space, every action is recorded in the details panel in chronological order. Record the queries and ops themselves (not the data), time taken, success or failure. Recorded by code, not by the agent.
- Prompts (approvals) are also shown, with accepted or denied, especially exemption ones (v0.2.36 P528).
- Depends on the P537 merge and v0.2.36 P528.
- Decide in plan: capture point in `dbmcp` and link to the session and task; storage and retention; redaction so no row data or bound values land in the trail; how it joins the merged list from P544.
- Plan: `plans/P546.md`.

### P547 Studio MCP option in workflow steps

- Ask: a Studio MCP option in workflow steps. No token; the app manages it automatically. If the Studio MCP server is globally disabled for external use, an agent session started with the DB MCP option still gets it working for that session.
- Touches workflow step editing (`workflows/AdeStepInspector.vue`, `inspector/`) and the MCP server auth (`mcpauth`, `dbmcp`).
- Decide in plan: step schema field and validation; per-session scoped access when the global external switch is off, and that it never opens the server to external clients; session-scoped credential lifecycle handled by the app, never shown to the user; interaction with the P537 naming outcome.
- Plan: `plans/P547.md`.

## Close

2 code review rounds (user ask). After P547: one full suite run with all failures fixed, review rounds, hand-written close per CLAUDE.md, PR.
