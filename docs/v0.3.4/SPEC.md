# v0.3.4

Proof images and agent branch bases. Branch `chapter/v0.3.4`, tag `v0.3.4`. First `P` is P569 (highest prior: P568).

| Phase | Title | Status |
|---|---|---|
| P569 | Proof: JPEG screenshots, agent image steps, menu bar entry | Implemented |
| P570 | Agents choose the base of a requested branch | Implemented |
| P571 Part 1 | Orchestrator tasks, backend: passes, spawn, chain reads | Implemented |
| P571 Part 2 | Orchestrator tasks, frontend: kind choice, card, Run pass | Implemented |
| P572 | About: GitHub link, add an About settings section if none | Implemented |
| P573 | Split docs/HISTORY.md into files | Implemented |
| P574 | Close chapter | Planned |

P569, P570 and P571 run as parallel streams (own worktree each): P569 owns proof, `dbmcp` proof tools and the proof window menu entry; P570 owns `request_branches` and the ADE base path; P571 owns the orchestrator task kind. Grouping in `plans/batch-P569-P570.md`.

Chapter ends the standard way after the last implementation phase: landing, one non-UI full run, 1 code review round, close phase P574, PR.

## P569 Proof: JPEG screenshots, agent image steps, menu bar entry

- Ask:
  - Proof window screenshot (copy, save active tab, save all tabs) encodes JPEG, not PNG, from now on.
  - `proof_save` gains an image step: the agent supplies a JPEG, as inline data or as a file path on disk. JPEG only.
  - Proof window opens from the menu bar, outside any task.
- Planner decides: step shape (`view` value, `data` fields), path validation (sandbox, size cap, JPEG signature check), how the window renders an image step, how the existing 32 MiB session cap and 8 MiB HTTP body cap apply, quality value for screenshots, clipboard handling (clipboard holds one image; PNG may stay there if JPEG copy is unsupported, planner verifies).
- Pointers: `apps/kira-space/internal/dbmcp/proof_tools.go`, `apps/kira-space/internal/proof/`, `apps/kira-space/internal/bridge/proof.go`, `proofshots.go`, `apps/kira-space/frontend/src/studio/proof/` (`screenshot.ts`, `useProofScreenshot.ts`), `docs/ARCHITECTURE.md` "Validation proof (P420)".
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary (CLAUDE.md Tests).
- Result: Implemented (stream A, branch `stream/p569`).
  - Image step: `proof_save` view `image`, `data` `{path}` or `{base64}`, JPEG only. Caps 3071 KiB, 16384 a side, 25 MP; each step validated once and its bytes copied at save. Migration 0092 widens the view CHECK.
  - Window: `ProofImage.vue` draws a data URL image, fit to width, result scroll as view state.
  - Screenshots: save and save all encode JPEG (0.92, opaque background). Copy stays PNG: `ClipboardItem.supports('image/jpeg')` is false in Playwright webkit and chromium. Bridge `SavePng`/`SavePngTo` renamed `SaveJpeg`/`SaveJpegTo`.
  - Menu: Window > Validation proof through new `shell.ItemAction`, opens the window with no task.
  - Tests: flow `TestProofImage`, `TestProofImageRefusals`, `TestProofImageSessionCap`, `TestProofMenuOpensWindow`; `TestMigration0092KeepsProofRows`; UI `proof-image.spec.ts` plus updated proof-window and proof-chart specs.
  - Deviations from the plan: session cap test uses nine max-size images (eight stay under 32 MiB); menu label reuses `backend.studio.proof.window.title` ("Validation proof", sentence case) since the catalog lint rejects a Title Case `backend.space.menu` key and a duplicate text; `JPEG` added to catalog proper nouns; consts live in `proof/image.go`.

## P570 Agents choose the base of a requested branch

- Ask: `request_branches` lets the agent give each branch a base. Today an entry is `{repo, branch}` and the base comes only from the user-set row (`SetBranchBase`, Change base).
- Planner decides: field name and shape (ref name vs task branch id, same options as `adewire.BaseChoice`), validation through the existing `resolveChoice` path (same repo, no cycles, `check-ref-format`, base not created yet), how the approval dialog shows each branch's base, behavior for an `existing` branch with a different base, prompt text and tool schema.
- Pointers: `internal/agentmcp/space.go`, `apps/kira-space/internal/ade/agenttools.go`, `branchrequest.go`, `base.go`, `setup.go`, `docs/ARCHITECTURE.md` "Task base and rebase runs (P241)" and "Branch requests".
- Tests: flow test plus UI spec on the same scenario (branch request dialog showing the base).

Result: `request_branches` entries take optional `base`, a short branch name. `agentBase` validates it through `resolveChoice` per branch before any prompt (syntax, not found, cycle, not created, user-set draft base). Stored on the row, no pending mark, worktree starts there. Existing branch keeps its base and `next` reports the mismatch. Two bases for one branch fail the call. Dialog shows the base, results and `task_info` carry `base`. Tests: adeflow `TestBranchRequest` (6 subtests), UI spec, real `TestAdeBranchRequest`.

## P571 Orchestrator tasks

Split by the planner (plan `plans/P571.md`): Part 1 backend (migration, engine, tools, flow tests), Part 2 frontend (starts after Part 1 is committed; its UI spec reads Part 1's contract).

- Ask: new task kind `orchestrator`. It runs a normal workflow (script, agent and interactive steps), may edit code, and coordinates work across other tasks. Manual trigger only, no cron.
- Decided (user):
  - Only orchestrator tasks can spawn tasks, so depth is always 1; children get no spawn tool.
  - No parallel limit: every spawn waits for a user prompt.
  - One orchestrator, many passes. A Run action starts the next pass, with an optional message through the stage dialog.
  - Its own stage handovers are its reference to previous work (newest through the existing handover path, older ones through a read tool). No Memory module.
  - No wake of the orchestrator when a child finishes; a child's final handover lands in the parent's chain.
- Planner decides, from code: pass mechanics (a finished task moves back to the first step via `SetTaskStep`; `ResetWorkflow` deletes runs, so not it; verify `SinceJSON`, `syncStatus` and status slots handle it), migration for the parent link (next free number, P569 takes 0092: whichever lands second renumbers), `create_task` tool shape (title, workflow, repos, brief as child's first handover with source `orchestrator`, optional base through the existing `BaseChoice` and `resolveChoice`, not P570's new field), approval prompt kind, read tools (`list_tasks`, `task_chain`), board presentation (card, children grouped, parent links both ways), grant rules (which task kinds carry which tools), wire types and bindings.
- Streams: confirm no ordering dependency on P570 (P571 must not need P570's `request_branches` field). If one exists, stop and report. Size: if the phase is too large for one implementer, propose `Part 1`/`Part 2` per the numbering rule instead of cutting scope.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.

### P571 Result

Part 1


Backend done. Migration `0093_p571_orchestrator` (marker `-- kira:foreign-keys off`; P569 owns 0092, whichever lands second renumbers). Kind `orchestrator` with `parent_id`; handover sources `orchestrator`, `pass`, `child` with `from_task_id`. Engine: `NextPass`, `create_task` approval (`task.create`), `list_tasks`, `task_chain`, child mirror, read-only child rows. Contract for Part 2: `apps/kira-space/tests/contract/ade-orchestrator.json`. Tests: flows `TestOrchestrator*`, `TestMigrateP571`, real claude `TestAdeOrchestrator` plus the ADE row, all passing.

- `create_task` waits on the `BranchRequestWait` app knob (one knob, `TaskBoardDeps.TaskRequestWait` carries it).
- Handover paging fix: the order key embeds the `draft` literal; condition builders escape placeholders in a field.
- Real claude run: the first `next` text made the agent pause; it now says not to wait.

Part 2 (frontend)

Kind choice in the Add popover (`ade-nw-kind`), task panel switch and card menu; refused once the orchestrator has children, a parent or is not a plain task (`board/kind.ts`). Orchestrator badge, children group on the card, parent chip and parent row (archived parent muted, no select), archived children from a popover. Run next pass reuses the stage dialog in `pass` mode (optional message, `NextPass` via `useNextPass`). Chain gains child and brief rows (child rows read-only, link selects the child). `task.create` approval body `TaskCreateBody` (title, workflow, repos with base, brief). i18n under `space.ade.orchestrator.*`. The orchestrator is never woken: manual trigger only.

- Deviation: `HistoryEntry.parentId` added to the wire (`adewire`, `toWireHistory`) because archived children are known only through history. Small Part 1 file edit.
- UI spec `tests/ui/ade-v2-orchestrator.spec.ts` (5 tests, same contract as the flow tests). Board ids are patched by the spec: the contract masks them.
- Tests: new spec plus `ade-v2-stage-move`, `ade-v2-notes`, `ade-v2-branch-request`, `ade-v2-add`, `ade-v2-handover`, 40 passed after `build:test`; `typecheck:tests` clean. No Go test rerun beyond build and vet.

## P572 About: GitHub link

- Ask: add a link to the GitHub repo in the app's About section. If the app has no About section, add one to Settings.
- Planner decides: find an existing About surface (settings pane, app menu About, window) from code; where the repo URL comes from (one source, no new hard-coded copy); how external links open (`LinkService.OpenExternal`, never `window.open`); Settings pane wiring if new (shadcn-vue, Tailwind, `<script setup lang="ts">`); i18n; ARCHITECTURE update.
- Tests: flow test plus UI spec on the same scenario where it applies at the IPC boundary (opening the link goes through the bridge).
- Result: native About (Wails `NSAlert`) cannot hold a link, so new Settings section About (last) shows name, version and the repo link. URL from `appupdate.RepoURL`, returned as `AppService.Info` `repoUrl`, opened through `LinkService.OpenExternal`. Info fetched by TanStack Query only while About is active (a mount-time fetch broke every spec that opens Settings without an `appInfo` fixture). `docker.engine.version.label` moved to `common.version.label`. Flow `TestAboutGitHubLink` and UI `settings-about.spec.ts` share contract `settings-about`; `copy-coverage.spec.ts` gained the About pane. Deviation: no `useQuery` `staleTime` (lint U53, client default already Infinity). Verified: flow, bridge and appupdate tests, typecheck, lint, UI `settings-about`, `settings-*`, `copy-coverage`, `consistency`, `docker-module`.

## P573 Split docs/HISTORY.md into files

- Ask (user): `docs/HISTORY.md` is 7,827 lines (45 chapter blocks); split it into several files. No size cap rule, just split.
- Decided: `docs/HISTORY.md` stays as a short index linking each file with its chapter range, so existing references to it resolve. Blocks move unchanged into `docs/history/`, cut at chapter boundaries only. A close appends its block to the newest history file. One-time split verified by concatenating the new files in order and comparing with the old file (minus the index).
- Planner decides: file naming and grouping, index format, `scripts/docshistory` changes (it reads and repoints links in HISTORY.md), `check-docs` and any other tool that reads HISTORY.md, link and code-comment references to `docs/HISTORY.md` (some cite chapter anchors), the CLAUDE.md "Closing a chapter" step 2 wording (hand-written close appends to the newest history file) and WORKING_AGREEMENT mentions, README mentions.
- Stream: own worktree, touches only docs, scripts and `CLAUDE.md`. Overlap with other streams: `docs/v0.3.4/SPEC.md` only.
- Result: `docs/HISTORY.md` is now a 6-entry index plus "Rules for a close" (imperative list, worked example). Limit (user decision): 1,500 lines per file in `docs/history/`; blocks stay whole, a block over the limit sits alone; a close appends to the newest file if it stays at or under 1,500 lines, else opens the next file (`NN-<first chapter>.md`, index entry added); existing blocks never move. Earlier size-balanced and version-series groupings were replaced. The 45 blocks were packed greedily in existing order, unchanged: `01-v1.md` (v1 to v1.2, 3 blocks, 1,331 lines), `02-v1.3.md` (v1.3 to v1.8, 6, 1,236), `03-v1.9.md` (v1.9, 1, 1,335), `04-v2.0.md` (v2.0, 1, 1,293), `05-v2.1.md` (v2.1 to v2.6, 4, 1,441), `06-v2.8.md` (v2.8 to v0.3.3 plus "Fixes outside chapters" after v0.2.12, 30, 1,185). v2.8 (99 lines) would push `05-v2.1.md` to 1,540, so it opens file 6. Newest file is `06-v2.8.md`. Verified: `concat-ok` (files joined equal `git show b16e4a84c:docs/HISTORY.md` lines 7-end byte for byte), 45 blocks, `order-ok`, no duplicate chapter, `balanced` for each file, no file over 1,500 lines, no missing index link, only the six planned files in `docs/history/`. `scripts/docshistory` is name-agnostic (globs `*.md`): `readBlocks(dir)`, `spliceBlock`, `requireBlocks`, `--history` default `docs/history`, `skipRepoint` skips `docs/history/` (test fixture `06-v2.8.md`); `render` only re-renders existing blocks and refuses a chapter with none (checked: `render v1.1` leaves tree clean, `render v9.9` exits 1). `TestBlocksSplice` replaces `TestBlocksIdempotent`; go vet, `go test ./scripts/docshistory/`, golangci-lint, `check-docs` pass. CLAUDE.md (close step 2 imperative placement list, highest-`P` line, forbidden-edit line), README.md and DEV_ENVIRONMENT.md reworded. No `docs/HISTORY.md (<ch>)` citation rewritten (index resolves them). No deviations.

## P574 Close chapter

- Chapter close by hand per CLAUDE.md "Closing a chapter": one Sonnet implementer, no planner, no plan file. History placement (limit 1,500 lines per file): append the v0.3.4 block (about 100 lines) to the newest file `docs/history/06-v2.8.md` (1,185 lines) if `wc -l` afterward is at most 1,500, and set its index range to "v2.8 to v0.3.4"; else open `07-v0.3.4.md` and add an index entry. Follow `docs/HISTORY.md` "Rules for a close".
