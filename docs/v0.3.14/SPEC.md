# v0.3.14: follow-ups from v0.3.13 and UI fixes

Goal: close gaps v0.3.13 found (agent config port journey, MySQL `DELIMITER`, mobile approval of
agent requests, collection file with one bad item) and ship five UI fixes; shard the long CI jobs. Each new feature follows
the IPC-boundary test split (flow test plus UI spec). First `P` is P634.

| P | Phase | Status |
|---|---|---|
| P634 | CI job sharding: split `go-containers` (about 24 min) and `go` (about 8 min) into shards behind stable required-check names | Done |
| P635 | Agent config import/export journey (J-HAR-002): extend `harnessflow` AgentPort tests and `ade-agent-port` UI spec | Done |
| P636 | MySQL console `DELIMITER` directive (F-A12) | Done |
| P637 | Mobile approval of agent requests (J-ADE-072, F-B8) | Done |
| P638 | Collection file with a bad item: valid items load, bad ones reported (F-A15, J-COLL-003) | Done |
| P639 | Toasts at top right, placed like prompts | Done |
| P640 | API module: dialog when closing an unsaved tab, new or modified | Done |
| P641 | ADE task: own Jira and PR links first in their group | Done |
| P642 | ADE task list: today background, day start/end left bar, today bar differs | Done |
| P643 | Details tab on disk: notes, handovers, links, PR link and proofs (with images) as files, source of truth; design plan first, user approves it before the build | Todo |
| P644 | Chapter non-UI full run, 2 code review rounds | Todo |
| P645 | Close chapter | Todo |

## Scope

### P634 CI job sharding

`go-containers` runs 24 min, longest job; `go` 8 min. New v0.3.13 tests lengthened both. Shard
each into matrix jobs over one split script (extend `scripts/go-test-split.sh`; balance by measured
package time, never drop a package: unmatched packages land in a remainder shard). Keep required
check names `go (Linux)` and `go-containers (Linux)` via aggregator jobs, as `ui` and
`test-matrix` do. Keep `pr-rerun-flaky.yml` working with shard job names. Reuse the Testcontainers
image cache per shard, pulling only images the shard needs. Record before/after wall time in
Result. Check `ui` shards (8, about 9 min), `db-compat` and `checks` for further cheap wins.

### P635 Agent config import/export journey

`internal/agentport` exists (Claude and OpenCode providers, preview with diff, backup on export, MCP
registration) with `TestAgentPortClaude`, `TestAgentPortOpenCode` and `ade-agent-port.spec.ts`.
v0.3.13 stream B wrongly dropped J-HAR-002 as "feature absent". Cover full journey from
`docs/v0.3.13/plans/journeys-ade.md` (read from git history): import from user Claude config with
hooks, MCP servers with secrets and statusline; masked preview diff; apply; export back after edits;
backups listed; undo via backup. Oracle: backup files hold original bytes; exported config parses
and equals settings; secrets masked in preview and never written to `settings.yaml` in clear. Fix
every bug found.

### P636 MySQL `DELIMITER`

Console statement splitter honours `DELIMITER <token>` so trigger, procedure and function scripts
and mysqldump output run. Splitter has several interacting rules (quotes, comments, delimiter
change mid-script): unit tests allowed. Flow plus UI halves: script with trigger body runs and the
trigger exists afterwards.

### P637 Mobile approval of agent requests

Paired phone lists pending agent approvals and approves or denies; desktop state follows. Reuse the
existing mobile pairing and approval model; design in the phase plan. Flow half: approval via
mobile route unblocks the agent step, deny blocks it, expired device token refused. UI half: phone
view of the approval list and the desktop reflecting the result. Covers J-ADE-072 (parent with
children, leases, notifications, mobile approval).

### P638 Bad item in a collection file

Validation of a hand-edited collection file is whole-file today: one bad item serves no rows on
first load. Load valid items, skip bad ones, show each skipped item with reason. Remove the entry
from ARCHITECTURE.md Known open items. Flow plus UI halves.

### P639 Toasts top right

Toasts anchor top right, same placement as prompts. No overlap between a prompt and a toast: stack
them. One shadcn-vue toast primitive, no new hand-rolled one. Update affected UI specs and
screenshots.

### P640 API tab close dialog

Closing an API-module tab asks to confirm when the tab is brand new and unsaved, and when it holds
unsaved changes to a saved request. Dialog offers Save, Discard, Cancel. Tab closes only after Save
or Discard. Dialog is shadcn-vue. Flow half: save then close persists, discard leaves the saved file
unchanged. UI spec for both cases.

### P641 Own links first

On an ADE task, the task's own Jira and PR links sort first inside their group, before links
collected from children or other sources. Update link strip and any sorted list sharing the order.

### P642 Task list day styling

In the ADE task list: replace the today background (current colour is wrong; pick from theme
tokens, light and dark), add a vertical left bar spanning each day's rows to show where the day
starts and ends, today's bar differs slightly from other days'. Visual spec and screenshot refresh.

### P643 Details tab on disk

Whole Details tab content moves out of SQLite into files under the home folder: initial notes,
handovers, links including the PR link, and proofs with their images (image files beside the
markdown, not base64 rows). Files are the source of truth for all content. State stays in SQLite and is never driven by a file edit: approvals, stages, branches (git and worktree state), leases, runs, status, workflow, plan placement.
Planner chooses the file split (several files per task; one read-only state file, header says changes go only over MCP, Kira rewrites it atomically: branches, worktree and setup state, leases, operations, status, step, task chain). Tasks are folders; an orchestrator creates a child by writing a folder. One sync tool (agent says files are edited): Kira reads, validates, diffs against its SQLite copy, raises the right prompt (planner decides which edits prompt and which only notify) and returns accepted or rejected per file; `finish_step` may submit implicitly; a watcher covers hand edits only. Tools left: sync, finish_step, run_outcome, declare_repos, request_branches, rename_branch, change_branch, acquire_lease, release_lease, operations. `task_info`, `lease_status`, `list_tasks`, `task_chain` become file reads. Rule: each field has one owner. Content owned by files; SQLite keeps id, path, content hash, review status and a rebuildable cache for lists, never an independent write. State owned by SQLite; the state file is a one-way generated mirror. Build order inside the phase: content files and sync tool, then state file, then tasks as folders only if the first two work. Tasks stay Kira-written read-only files (no agent-written task folders); `create_task` stays a tool with its approval. Agent tools end at about 6: submit, finish_step (with run outcome), branches (request, rename, change, declare), lease, cancel_operation, create_task. Kira writes a README per task folder (file meanings, structure, read-only files, limits, submit behaviour) and a header in each file; step and session prompts point to the folder. Performance: state file holds coarse state only, writes atomic and coalesced (100 to 250 ms), size and mtime before hash, no global watcher (submit is the trigger; watch open task folders for hand edits only), UI reads SQLite never files, size caps enforced on submit. Spike first: real-claude comparison of tokens and turns, tools versus files, on one handover and proof task. No FUSE (macOS only: macFUSE not fully open source, FUSE-T license unverified); loopback NFS mount noted as a later option. Proofs: `proof_save` and `proof_list` removed; the agent writes the proof file and calls `submit`, which validates (what `proof_save` checks today, errors name the step) and raises the proof prompt. In a Space task it is the one `submit`; for standalone sessions `submit` is on kira-studio. No watcher anywhere: an explicit agent call is the consent point. Proof window shows one proof like today: a tab per step with its explanation, data (table, chart, key-value, document, diagram) or image, and view state. Standalone proofs live in a shared proofs folder under Kira's home. SQLite holds an index row per content file (path, size, mtime, hash, listed fields, review status); every index is a rebuildable cache except review status. No index: board, list and the archived finder (searches board history by title and Jira key, in memory) keep reading SQLite list fields as today; planner names the few list fields kept in SQLite beside the file header, limited to what the board shows or searches; a task's files are read only when it opens. SQLite keeps state and review state (the last accepted copy of a proof, for the accept or reject diff). Proof window shows exactly one proof, one file: no list, no rescan, no retention of 30. Sandbox: Kira grants each task folder to every run, session and review agent automatically; state and task files are read-only there (sandbox deny); no chmod or immutable flag, an agent edit of a Kira-written file is harmless because Kira rewrites it; no lock on content files (no true file lock exists against a non-cooperating writer). Collisions: one file per entry (one per handover, one per note) so writers rarely overlap; Kira stores the hash of each file's last accepted version and a submit that finds both sides changed raises the warn-and-confirm prompt (decision 3 policy). `finish_step` reports content files changed but never submitted. Risks the plan covers: one-time migration with backup and verify, size of the change (planner may split into several phases), file-then-database write order so a retry is safe, path traversal, symlinks and size caps checked on submit. Layout is the planner's call, derived not assumed: a writer matrix of every file against every writer (step run agent, manual session, review agent, orchestrator, child task, user in the UI, Kira) with read, append or rewrite per cell; number and content of files follow from it; every file with more than one writer states its collision rule (append-only, hash check with warn prompt, or single owner); residual collisions listed with why each is acceptable. Future cloud hosting: file layer uses relative paths only and nothing outside the local adapter depends on absolute paths, symlinks or file times. Opus plan decides, then user approves before any build: format and location, one-time migration of
existing rows, edit protection (agents read freely; Kira lock versus ask-to-edit), proof revisions,
pending edit accept/reject, view state and retention of 30 (on disk, or SQLite keeps review state
only), MCP tool changes (`handover` removed; `finish_step` keeps its end-of-step signal, summary text moves to the file; `report_artifacts`, `proof_save`, `proof_list` removed, agent writes the files and Kira validates on read; `task_info` drops notes, links and handovers; branch, lease, operations and orchestrator tools stay), behaviour when a file is edited by hand while the app runs. Flow plus UI
halves per the split.

## Results

### P634 Result

Scope change (user): `go-containers` stays one job, no shards, no aggregate. `go (Linux)` is sharded.

- `scripts/go-test-split.sh`: shard table (`rest`: `adeflow1`, `adeflow2`, `other`; `containers`: one
  remainder shard), `--matrix`, `--list`, `--bundles`, `--kinds`, `--pins-hash`, `--check`. `dbmcpflow` moved into
  the containers group. `pull-test-images.sh [--print] [kind...]` with `flow` pseudo-kind.
- `pr.yml`: `go-split`, `go (Linux, <shard>)`, aggregate `go (Linux)`; `go-containers (Linux)` one job, restore-only
  per-bundle caches `tc-images-<os>-<bundle>-<pins-hash>`, `-timeout 7m`, 15 min job cap, no save steps.
  `cache-warm.yml`: `tc-images` matrix saves bundles (miss: elastic 113 s, sql 94 s, kv 75 s, stream 124 s, flow 5 s).
- Local: `--check` ok; 21 containers packages, rest union equals whole (no duplicate); `--matrix rest` is
  `["adeflow1","adeflow2","other"]`; adeflow slices 95 and 95 tests (190 top-level, plan said 191); slice
  test time 157 s and 140 s on a loaded 4-CPU box. Docker daemon unavailable here, so `containers/sql` did not
  run locally; CI ran it. actionlint v1.7.7: only the old-label `macos-26` findings, present before.
- Two pre-existing flakes fixed on the spot: `adeflow` `TestOrchestratorPass` (handover row lands after run
  `done`), `termflow` `TestScheduleNoDuplicateFireAfterRestart` (fake clock keeps the quit loop timer; 4 of 40 failed before, 0 of 60 after).
- Dispatched `pr.yml` on `chapter/v0.3.14`, seconds from run start (queue = `started_at` minus `created_at`):

| job | before (43628609545) | run A 43638705328 (cache miss) | run B 43640692677 (hit, `go other` flaked) | run C 43642452955 (hit) |
|---|---|---|---|---|
| `go (Linux)` aggregate end | 472 | 416 | 359 (failed) | 360 |
| `go adeflow1` wall, test step | | 218, 101 (queue 133) | 209, 114 | 190, 96 (queue 133) |
| `go adeflow2` wall, test step | | 245, 133 | 237, 117 (queue 103) | 233, 123 (queue 112) |
| `go other` wall, test step | | 304, 194 (queue 94) | 228, 138 (queue 102), failed | 275, 117 |
| `go-containers (Linux)` wall, tests | 477 hit, 1424 miss | 326, 169 | 335, 140 | 406, 132 |
| `go-containers` image wait | | 70 (miss: pulled) | 70 | 64 |
| `ui (Linux)` end | | 704 | 747 | 708 |

  Run A missed every bundle (mirror pulls, no save, no rerun, 326 s against 1424 s before). Runs B and C
  restored every cache bundle step (elastic, sql, kv, stream; 6 to 22 s each); `flow` has no tar and pulls through the mirror.
- Targets: `go` about 360 s in runs B and C (target 390): met on hit, 416 s on run A. `go-containers` miss 326 s
  (target 450): met. Hit 335 and 406 s (target 390): run C missed it; setup took 117 s and cache restores added 58 s.
- Queueing: 26 jobs against the concurrency limit delayed `go adeflow1`, `adeflow2`, `other` by 94 to 133 s in
  several runs; this is the main overshoot on `go`.
- ui setup (GTK) step takes 94 to 132 s per shard; its apt sub-step time was not measured separately. Candidate: skip the
  GTK install when `bindings: "true"` only needs the `wails3` version check. Not added as a row; orchestrator asks the user.

### P635 Result

- Flow: `TestAgentPortClaudeJourney` (J-HAR-002) plus OpenCode extension in `harnessflow`. Covers import with hooks,
  MCP servers with secrets and statusline; masked preview diff; apply; export after edits; backups listed; undo via backup.
- New `agentport` `Backups`/`Restore`. `Restore` refuses `~/.claude.json`. Bridge binding added.
- UI: `AdePortBackupsDialog`; `ade-agent-port.spec.ts` extended, 8 UI specs pass.
- Real `claude` row (DEV_ENVIRONMENT "Agent config import and export") passes.
- No bugs found in existing code.
- Deviations: SessionStart hook keeps the user's form, no timeout added. `.claude.json` literal count can drop after
  export. `envNames` omits `env.JIRA_TOKEN`.

### P636 Result

- Frontend splitter handles `DELIMITER` (`sqlSplit.ts`, option `delimiterDirective`, rules D1 to D10). No Go production change;
  the server never sees the directive.
- Comment-only fragments no longer sent on MySQL/MariaDB. Format keeps directives.
- Tests: 13 splitter unit cases plus 4 Format cases. Flow tests pass on MySQL and MariaDB. UI spec `console-script.spec.ts`
  passes. Test ID J-CONS-028.
- Known open items (`docs/ARCHITECTURE.md`): new, op-log Re-run mis-splits a trigger body with inner `;` and no `BEGIN`.
  Splitter item narrowed to Postgres `BEGIN ATOMIC`.

### P639 Result

- New `CornerStack.vue` (`packages/workbench`): one top-right column. Chips (`ApprovalsToast`, `ConfigSyncToast`) first, Sonner toasts below. 8px under the title bar, 16px from the right edge. Toast offset follows the chips' measured height.
- `Sonner.vue` takes `offset` and `mobileOffset`. `App.vue` mounts `CornerStack` in place of the bottom-right `Toaster`. `ConfigSyncToast` moved from bottom-right to top-right. Still one shadcn-vue toast primitive.
- UI: new `toast-placement.spec.ts`, 4 cases. 187 of 187 targeted specs pass. Visual baseline `approvals-toast-visual-linux.png` unchanged.
- No bugs found in existing code.

### P638 Result

- `configfs.YAMLSchemaSkip` drops bad array elements instead of failing the file (`skip.go`, at most 100 per file). Whole-file problems stay whole-file: YAML syntax, anchors, size cap, `/name`, `/variables/...`.
- apifiles collection load skips per item. Valid items load. A skipped item keeps its last good row. A duplicate id skips the later item. `FileView.skipped` and `CollectionFile.Skipped` carry line, index, id, name, message.
- Edits to a file with skipped items are refused (`BrokenError`, line of the first skipped item). The file is never rewritten.
- UI: `ApiConfigProblems.vue` shows a warn Alert per partly loaded file and a Skipped report in FileView, each item with line and reason. i18n keys `studio.api.config.skipped.title`, `label`, `hint`.
- Flow: `apiflow` `configfiles_test.go` extended, contract `api-collection-skipped.json`; `configfs` `skip_test.go`. UI: `api-collection-skipped.spec.ts`.
- Known open item "Collection file with one bad item serves no rows on first load" deleted from `docs/ARCHITECTURE.md`.

### P637 Result

- Phone routes `GET /api/approvals` and `POST /api/approvals/answer` (`mobileweb/approvals.go`). Answer needs Agent input permission and writes audit `approval.answer`. Only `task.create` and `branch.request` reach the phone. Other kinds stay desktop only, checked on read, push and answer.
- Phone payload strips paths. The `cwd` key stays an empty string because the shared schema requires it.
- Phone UI: `PhoneApprovals.vue` on the Need You tab, shared `ApprovalItemCard`, kind defs split into `kindDefs.ts`, plain-text `TaskCreateBrief.vue`. Desktop follows through the queue push.
- Flow: `TestPhoneApprovals`, `TestPhonePermissionMatrix`, contract `mobile-approvals.json`. UI: 9 mobile specs in `approvals.spec.ts` plus desktop `ade-mobile-approval.spec.ts` pass.
- `scripts/check-ui-primitives.sh` allowlists U101 for `PhoneApprovals.vue`.
- Fixed flake: `ade-v2-orchestrator.spec.ts` "task.create brief is in the dialog first frame" (framesLate Infinity, 4 of 6 runs). Cause: `usePrompts` push did not cancel the in-flight initial list fetch, which overwrote the routed entry so the dialog never opened (now cancels, like `useApprovals`). Also `TaskCreateBody` renders the readonly brief with sync `AdeNoteBody`, not the lazy editor chunk. 8/8 pass, file 48/48.

### P641 Result

- `linkGroups.ts` sorts in one pass with `isOwn`: Jira and host slots, detected change requests and issues, and stored rows whose URL is in `ownUrls`. Order inside a group: own, others, detected branches.
- `useTaskLinks` builds `ownUrls` from board change requests of the task's branches (`useChangeRequests`). One strip component, so Details, archived Details, run and stage dialogs share the order.
- Unit: `ade-v2-link-groups.spec.ts`, 4 cases. UI: 2 new tests in `ade-v2-links-strip.spec.ts`. 46 of 46 targeted specs pass.
- Visual baselines judged unchanged from fixtures. Not run locally, they run in PR CI.
- No backend change. `docs/ARCHITECTURE.md` ADE links line and Tests line updated.

### P642 Result

- Today tint is now a faint blue, 7% `--kira-info` mix (`--color-today-tint`, `tones.css`). Was a 5% grey mix. Fallback if the blue is disliked: drop the fill.
- New token `--color-day-bar`. `AdeDayBand.vue` renders one bar span per day (`ade-day-bar`): 2px, today 3px and lighter (`bg-today`), none for Later. The bar spans the heading to the last row.
- UI: new test in `ade-v2-plan.spec.ts`. Visual: new spec in `ade-plan-days.spec.ts`, new baseline `plan-day-bars-visual-linux.png`.
- No Go half: presentation only.

### P640 Result

- Shared confirm dialog: store `choose()` returns the picked action, with an optional third button (testid `confirm-dialog-secondary`). `confirm()` signature and testids unchanged.
- Tab kinds take an optional `confirmClose` hook (`TabKindDef`, `createTabsStore`). `http-request` and `grpc-request` use it through new Pinia store `studio/api/state/tabClose.ts`: Save, Discard, Cancel. Tab closes only after Save or Discard.
- `saveRequest` and `saveGrpcRequest` return boolean. Save as (new tab) uses `waitForClose` on the save dialog store.
- Decisions: an untouched blank new tab closes without asking. Incognito tabs never ask. Title copy has no question mark (catalog rule C7). Keys `studio.api.tabClose.*`.
- Bug found and fixed: Save as cancel passed its click event as `saved`, so cancelling closed the tab. `closeSaveDialog(saved = false)`, template calls it with no argument.
- Flow: `TestTabCloseSaveOrDiscard` (save then close persists, discard leaves the file unchanged), contract `api-tab-close.json`. UI: `api-tab-close.spec.ts`, 7 cases. 149 of 149 neighbour specs pass.
