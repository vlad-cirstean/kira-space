# P643 Details tab on disk: plan

Design-first. User approves this plan, and answers "Open questions", before any build Part starts.
Planner: Opus. Base `1e1300e16`. Discovery through `codegraph_explore`, then targeted reads.

## Decisions (user answers to section 18, recorded at Part 1)

- Q1 yes: no watcher; stat on open, focus, Refresh, submit.
- Q9 yes: merged `branches` and `lease` tools with an `action` argument; `run_outcome` folds into `state.yaml`.
- Q12 yes: spike runs haiku 3 per flow and arm plus one sonnet run each; cap 0.25 USD per run, 5 USD total.
- Q15 changed: `details.edit` approvals also reach the phone. Recorded for later Parts (supersedes
  "desktop only" in section 7.3 and Q15 recommendation).
- Silent accept: a submit change that needs no approval and has no notification today is accepted
  with no notification at all. Overrides the notify entries of section 7.3 for such rows. A new proof
  keeps today's behavior: accept plus the existing "proof ready" notification, not silent. Only
  changes that need the user's approval raise a prompt.
- Q6 changed: `submit` is not uniformly non-blocking. It behaves like the old tool each prompted item
  replaces: an item whose old tool blocked for the user's answer blocks the `submit` call the same way
  (same wait and timeout as today's equivalent, `request_branches` style up to 30 min); an item whose
  old tool returned at once returns at once. Section 7 must map each prompted item type to its old
  tool's behavior (`handover`, `report_artifacts`, `proof_save`, `proof_list`, ...). A submit with
  mixed items waits for the blocking items only and reports the rest as `pending` in the same reply.
  Verified against the code (table in section 7.3): of today's items none maps to a blocking old
  tool. Under D-Q2 variant F the child draft item maps to `create_task`, which blocks (30 min), so
  a submit holding a draft waits for that draft only; every other item returns at once. Spike
  stubs stay as built.
- Q3 yes: folders named by task id, `<home>/tasks/<taskId>/`.
- Q10 changed: no proof outside a task. Proofs live only in a task folder. Removed: standalone
  folder `<home>/proofs/`, kira-studio `submit`, `proof_save` and `proof_list` for every client,
  Window menu "Validation proof", standalone picker, proof history pane, unattached sessions, attach
  of an unattached proof to a task, detach. Exact list in section 10.
- Q11 changed: no backup after migration. No `VACUUM INTO`, no 30-day backup retention. Migration
  stays one-time and resumable; image base64 payloads are rewritten to files, one-way.
- Q17 new, decided: proof images take PNG, JPEG, WebP and SVG (today JPEG only). Raster: format by
  magic bytes, extension must agree, full decode check, caps per image 10 MiB and 16 MP, each side 1
  to 16384 pixels, no animation. SVG (agents draw schemas and diagrams): XML parse, 2 MiB cap,
  reject (never sanitize) script, foreignObject, event handlers, DOCTYPE, external references;
  shown only through `<img>` from a data URL. WebP is the least needed of the four (rarely made by
  agents, the only one needing a new Go dependency) and can drop alone. No GIF, HEIC or AVIF. Spec
  in section 10 "Images".
- D-Q2 (reverses Q2, conditional on spike flow F6): default = child task drafts. An orchestrator
  writes `children/<draft>/` (`task.yaml` plus `brief.md`) in its own task folder and calls
  `submit`; Kira validates as `create_task` does, raises the same `task.create` approval, and on
  approval creates the real task and its folder. `create_task` goes. Built only if F6 passes its
  gate (section 14: files arm cost at most 1.1 times tools, turns at most tools plus 1 per child,
  pass rate not lower). F6 fails = variant T: `create_task` stays a tool with its approval, as the
  plan said before. The switch, both tool tables and the verdict rule: section 7.5.
- Q16 new (raised by Q10 plus Q11, section 18): fate of today's unattached proof sessions at
  migration. Plan recommendation applies until the user answers.
- Efficiency measured through the whole migration (user requirement): spike arm T numbers are the
  baseline in `plans/P643-spike-results.md` before Part 2; Parts 3, 4, 5 each rerun their flows
  against the real build; final benchmark plus Kira-side cost at the end of Part 5; a miss sends
  the Part back, a final miss goes to the user. Section 14.1.
- All other questions (Q4 to Q8, Q13, Q14): plan's recommendation.

## 1. Decisions taken as input

From SPEC P643 Scope and the design talk. Binding.

- Content (notes, handovers, links with PR link, proofs with images) lives in files under the home
  folder; files are source of truth for content. State stays in SQLite; no file edit drives state.
- One owner per field. SQLite keeps id, path, hash, review status and rebuildable caches; never an
  independent write of content.
- One sync tool, explicit `submit`, is the consent point. No watcher for pickup.
- `handover`, `report_artifacts`, `proof_save`, `proof_list` go. `task_info`, `lease_status`,
  `list_tasks`, `task_chain` become file reads. Agent tools end near 6.
- Tasks stay Kira-created; `task.yaml` and `state.yaml` stay Kira-written read-only files. How
  an orchestrator asks for a child is D-Q2: draft folder plus `submit` (default) or `create_task`
  (if spike F6 fails). Either way the user's `task.create` approval is the consent point.
- Proof window shows one proof; no list, no rescan, no retention of 30.
- No FUSE, no chmod, no lock. Sandbox grants each task folder; read-only files denied in sandbox.
- Spike first, with go/no-go gate; benchmarks against its baseline after every agent-facing Part.

Two SPEC sentences conflict; plan resolves them (Q1, D-Q2):

- "a watcher covers hand edits only" / "watch open task folders for hand edits only" versus "No
  watcher anywhere" and the user's later rejection of a watcher. Plan: no watcher; stat on view.
- "Tasks are folders; an orchestrator creates a child by writing a folder" versus "Tasks stay
  Kira-written read-only files; `create_task` stays a tool". Plan (D-Q2): both, split by role. A
  task folder is agent-draftable but Kira-created: the orchestrator writes a draft child folder
  inside its own task folder, Kira creates the task and its real folder only on the user's
  approval, and the created folder's `task.yaml` and `state.yaml` stay Kira-written. Built only if
  spike F6 passes; otherwise the latter sentence holds unchanged.

## 2. Current state map

Content today, all in `kira.db`:

| Content | Table / column | Writers today | Code |
|---|---|---|---|
| User notes | `ade_task_handovers` state `note`, source `user` | user (`AddNote`, `UpdateHandover`), task create, capture | `ade/handover.go`, `storage/repos/adehandover.go` (`AddNote`, `Notes`) |
| Handover draft | `ade_task_handovers` state `draft`, one per (task, stage), partial unique index | `handover` tool (`RecordHandover`), user edit, run summary fold | `ade/handover.go:71`, `internal/agentmcp/handover.go` |
| Handed handover | state `handed` | stage move (`StepDone`, `SetTaskStep`), run handover, orchestrator brief/pass | `ade/handover.go`, `ade/orchestrator.go`, `ade/taskcreate.go` |
| Child mirror | state `note`, source `child`, `from_task_id` | repo layer `mirrorFinish` on child finish | migration `0093_p571_orchestrator.sql` |
| Stored links | `ade_task_artifacts` (kind, title, url, ref, source, session) | `report_artifacts`, Details paste (`AddLink`, `EditLink`, `RemoveArtifact`) | `ade/artifacts.go` (`storedSource`, `storeCleanLocked`), `ade/links.go`, `repos/adeartifact.go` |
| Jira and PR link | `ade_tasks.jira_key`, `jira_url`, `host_url` | Details paste into empty slot, task edit | `ade/links.go:90-165`, `ade/board.go:688` |
| Proofs | `proof_sessions`, `proof_revisions`, `proof_steps` (payload BLOB, image as base64 JSON), `proof_view_state`, `proof_task_links` | `proof_save` (kira-studio), user link/unlink | `internal/proof/*`, `storage/repos/proof.go`, `dbmcp/proof_tools.go`, `ade/proof.go` |

Detected links (branches, change requests, deployments, tracker issues) derive on read
(`ade/artifacts.go` sources); they stay derived, never stored, never in files.

Agent surface (`internal/agentmcp`, server name `kira-space`; `tools.go` lists ids):

- Run grant: `finish_step` (summary is the run's handover), `run_outcome`, `SpaceToolIDs` (10):
  `task_info`, `declare_repos`, `request_branches`, `rename_branch`, `change_branch`,
  `report_artifacts`, `operations`, `acquire_lease`, `release_lease`, `lease_status`. 12 tools.
- Session without run (`handoverGranted`, `handover.go:28`): `handover` replaces `finish_step`. 12.
- Orchestrator adds `create_task`, `list_tasks`, `task_chain` (`orchestrator.go`). 15.
- Conditional: `git`, `reply_review_comment`, `fetch_remote`, `push_branch`.
- kira-studio server (`dbmcp`): `proof_save`, `proof_list`; in a Space task a new proof attaches
  through `dbmcp.Config.ProofAttach` (`ade.TaskBoard.AttachProof`).
- `task_info` carries no notes, links or handovers today (`space.go:59` `TaskInfo`); content reaches
  agents through prompts (`ade/vars.go` `composeManualMessage`, `capNotes`, `handoverFor`) and the
  finish reply artifact list (`agentmcp/artifacts.go` `finishContent`).
- `task_info` `waitSeconds` waits on branch setup (`BranchStatus`); `taskinfowait_test.go` covers it.
- Tool texts: `internal/llmprompts/registry.go` (`HeadlessTool*`, `HeadlessSpaceInstructions`).

Folders and access:

- Home: `kirapaths.SpaceHome()` = `$KIRA_SPACE_HOME` or `~/.kira-space` (`internal/kirapaths/paths.go`).
- `TaskBoardDeps.TasksDir` = `<home>/ade/tasks` (`appwire/wire.go:271`): work cwd for runs on a task
  with no branch, created on first use, removed at archive (`ade/taskdir.go`).
- Sandbox: Seatbelt on macOS only (`internal/agentsandbox/profile.go`), off by default
  (`adewire.DefaultSandbox`). Supports literal write denies inside an allowed subpath
  (`profile.go:157-209`). Inputs in `ade/sandbox.go` `sandboxInput`, TUI in `ade/sandbox_tui.go`.
- Claude reaches dirs outside cwd through `--add-dir` (`harness.HeadlessSpec.AddDirs`,
  `InteractiveSpec.AddDirs`). Deny rules via `DisallowedTools` (`Edit(...)` patterns; OpenCode maps
  them in `internal/harness/opencode/tools.go` `translateRules`).
- Existing watcher: `ade/folderwatch.go` (repo import only). No content watcher exists.

Reusable pieces:

- `internal/configfs`: YAML node parse with line mapping, JSON Schema validate, `YAMLSchemaSkip`,
  atomic write with `baseHash`, generated `AGENTS.md` (`Folder.WriteDocs`).
- `renameio.WriteFile` for atomic writes (`llmprompts/store.go:338`).
- `internal/pathsafe.ValidateRelPath`: absolute, `..`, symlink escape.
- `internal/proof` validators: `checkTable`, `checkChart`, `checkDiagram`, `checkImage`,
  `validate` (`validate.go`, `chart.go`, `diagram.go`, `image.go`). Unexported today.
- Panel edit forward (`ade/panelforward.go`): pending user edits pasted into running sessions.
- Lease notice delivery: `SendIfIdle`, Stop/UserPromptSubmit hook reply, next tool result `notices`
  (ARCHITECTURE "Resource leases (P620)").
- Approvals queue with item lists (`branch.request` model; `proof.review` for proof edits).
- `go-udiff` text diff (`panelforward.go` `textDiff`).

UI today: `details/AdeDetailsTab.vue` (`AdeLinkStrip` plus `AdeTaskChain`), `notes/*`,
`studio/proof/ProofWindow.vue` (history pane `ProofHistory.vue`, tabs, change panel),
`panel/AdePanelForwardBar.vue`. Board and mobile read SQLite (`Task.notes`, `handoverIn`,
`handoverDrafts`, `Jira`, `HostLink`).

Tests touching this area: flows `adeflow` (`handover`, `notes`, `artifacts`, `detailslinks`,
`detailslist`, `detailsopen`, `orchestrator`, `panelforward`, `proofchain`, `stagemove`,
`taskinfowait`, `taskpanel`, `agent_tools`, `leases`, `archived`, `dbmcpstep`, `twowindows`),
`proofflow/*`, `captureflow`, `githostflow/githublinks`, `journeyflow/firstrun`, `termflow/smart`;
UI `ade-v2-{artifacts,details-links,details-list,details-open,handover,links-strip,notes,orchestrator,proof-chain,task-panel}.spec.ts`
plus proof window specs; real `claude` `ade_test.go`, `orchestrator_test.go`, `proof_test.go`,
`automation_test.go`, `hooks_test.go`.

## 3. Core idea: keep the tables, move the content

Existing tables already split state from content. Plan keeps every table as the state and index
layer and turns content columns into caches written only by sync:

| Table | State columns (SQLite owns) | Cache columns (file owns, sync writes) |
|---|---|---|
| `ade_task_handovers` | id, task, state, source, from/to step, finished, dismissed, edited, session, from_task, times | `body`; new `rel_path` |
| `ade_task_artifacts` | id, task, source, session, step, run, created | kind, title, url, ref |
| `ade_tasks` | everything else | `jira_key`, `jira_url`, `host_url` |
| `proof_sessions`, `proof_revisions`, `proof_task_links`, `proof_view_state` | all (review state, current/pending rev, seen, link, view state) | `name`, `summary` |
| `proof_steps` | rev, ordinal, diff, origin | description, query, language, source, view, payload (image payload: metadata plus file name and hash, no bytes) |

Result: board, mobile, prompts and Details keep reading SQLite. "UI reads SQLite, never files"
holds with no new list index. The cache holds the last accepted copy, which is the diff base for
submit and the restore source for reject. Rebuild = read files, treat them as accepted.

New tables (one migration, Part 2):

- `task_files(task_id, rel_path, kind, size, version, sha256, accepted_sha256, accepted_by,
  accepted_at, review, problem)`, PK (task_id, rel_path). Every file belongs to a task (Q10).
  `version` is the store's opaque token (local: size plus mtime ns). `review` in
  `accepted|pending|invalid`. `accepted_by`, `accepted_at` and `review` are state; the rest rebuilds.
- `task_file_intents(task_id, rel_path, sha256, writer, created_at)`: journal for file-then-DB writes.
- `task_files_migration(task_id, part, state, done_at, verify_json)`: resumable migration marker.
- D-Q2 variant F only, own migration in Part 5: `task_child_drafts` (section 7.5).

Justification per index: `task_files` is needed for change detection without reading every file
(size plus version before hash) and holds review state; intents make a retry safe; migration
markers make the one-time move resumable. No search index: board, list and archived finder keep
their SQLite fields (title, Jira key) as today.

Accepted image bytes: content-addressed, Kira-only store `<home>/tasks-blobs/<sha256>.<ext>` (ext of the sniffed format), outside
every agent grant. Needed only to restore an image a rejected edit deleted or changed. Garbage
collected after accept and at archive.

## 4. Layout

Root `<home>/tasks/` (new, beside `ade/`; listed in ARCHITECTURE "stays at home root"). Folder per
task: `<home>/tasks/<taskId>/`. Id, not title: stable relative paths, a rename moves nothing.
The work cwd `<home>/ade/tasks/<id>` stays where it is: Claude resume keys on cwd, so moving it
breaks resume of existing sessions.

```
<home>/tasks/
  AGENTS.md                    generated: formats, rules, limits (configfs WriteDocs pattern)
  <taskId>/
    README.md                  Kira, read-only: this task's files, what to edit, submit rules
    task.yaml                  Kira, read-only: definition mirror
    state.yaml                 Kira, read-only: state mirror (Part 5)
    links.yaml                 content: links, Jira slot, PR slot
    notes/<created>-<id>.md    content: one file per user note
    handovers/draft-<step>.md  content: open draft of a stage
    handovers/<NNNN>-<from>-to-<to>.md   content: one file per handed handover
    proofs/<slug>/proof.md     content: proof definition and explanation
    proofs/<slug>/<file>.json  content: step data (table, document, keyvalue, chart)
    proofs/<slug>/<file>.mmd   content: diagram source
    proofs/<slug>/<file>.png   content: image step (.png, .jpg, .jpeg, .webp or .svg)
    children/<draft>/task.yaml orchestrator draft of a child task (D-Q2 variant F only)
    children/<draft>/brief.md  draft brief, becomes the child's first handed handover
<home>/tasks-blobs/            Kira only: accepted image bytes
```

No proof folder exists outside a task folder (Q10). `children/` exists only in an orchestrator's
folder under D-Q2 variant F; in any other task it is "not Details" (`ignored`). Kira removes a
draft folder once its task exists (section 7.5).

`<created>` is `YYYYMMDDTHHMMSS` from the row, not a file time; names sort by time for agents.
Hidden files, unknown names and anything deeper than `proofs/<slug>/` are "not Details": left
alone, reported once as `ignored` by submit.

## 5. Formats

| File | Format | Why |
|---|---|---|
| notes, handovers | Markdown, optional YAML frontmatter | prose; agents edit Markdown natively; Claude `Edit` string replace works; readable diffs |
| `links.yaml`, `task.yaml`, `state.yaml` | YAML | small structured; comments carry the header; fewer tokens than JSON; configfs gives line numbers |
| `proof.md` | YAML frontmatter (structure) plus Markdown body (explanation) | one file per proof as the user asked; frontmatter validates with line numbers |
| proof data | JSON (`.json`), Mermaid (`.mmd`), PNG, JPEG, WebP or SVG image (`.png`, `.jpg`, `.jpeg`, `.webp`, `.svg`) | existing validators take raw JSON (precision and key order kept, P420); images as files, not base64 |
| child draft (D-Q2 variant F) | `task.yaml` YAML plus `brief.md` Markdown | same split as a task: small structured definition, prose brief; same YAML pipeline with line numbers |

Every content file is content only. Metadata (stage names, author, session, times, finished,
dismissed) lives in SQLite and shows in README and UI, never in the file. An agent cannot forge
authorship by editing a header. Kira never rewrites an agent-written file to normalize it, so an
agent's next `Edit` never fails on "modified since read".

Header in each file, one comment line (YAML `#` in YAML and frontmatter, `<!-- -->` in a bare
Markdown file): `Kira <kind>. Edit, then call submit. Rules: <relative README path>`. Optional on
agent-created files; validation does not need it.

Handover draft `handovers/draft-implement.md`: body only. Handed file: body only, name encodes
stages. Note: body only.

`links.yaml`:

```yaml
# Kira links. Edit, then call submit. Rules: README.md
jira: {key: ABC-12, url: https://acme.atlassian.net/browse/ABC-12}
pr: https://github.com/acme/app/pull/7
links:
  - {kind: doc, title: Design, url: https://...}
  - {kind: link, title: Fixture, ref: testdata/orders.csv}
```

Same rules as `agentmcp.CleanArtifact` (kinds, caps, absolute http(s) URL; `ref` relative, never
absolute). Entry key = normalized URL, else (kind, ref). Author per entry stays in
`ade_task_artifacts.source/session_id`.

`proof.md`:

```yaml
---
# Kira proof. Edit, then call submit. Rules: ../../README.md
kira: proof/1
name: Orders total matches ledger
summary: Totals agree after the backfill.
reason: ""            # required when changing an accepted proof
steps:
  - id: totals        # stable; keep it when you change the step
    description: Ledger and orders totals match
    view: table       # table | document | keyvalue | chart | diagram | image
    data: totals.json # file in this folder: .json, .mmd for diagram, .png/.jpg/.jpeg/.webp/.svg for image
    query: |
      select sum(total) from orders
    language: sql
    source: pg-prod
---
Optional longer explanation.
```

Step `id` replaces `keepOrdinal`/`replacesOrdinal`: same id and same content hash = unchanged, same
id changed = changed, new id = added, missing id = removed. Caps and checks are today's
(`proof/validate.go`, 100 steps, 32 MiB per proof, image caps); errors name file, line and step id.

## 6. Writer matrix

Cells: R read, A create new file, W rewrite, D delete, `-` none. Agent columns: run = headless step
run, manual = manual step TUI session, review = review agent, orch = orchestrator on its own task,
child = a child task's agent on its parent's folder. Hand = user's own editor while the app runs.

| File | run | manual | review | orch | child | user UI | hand | Kira |
|---|---|---|---|---|---|---|---|---|
| `AGENTS.md`, `README.md` | R | R | R | R | - | R | - | W |
| `task.yaml` | R | R | R | R (+children R) | - | R | - | W |
| `state.yaml` | R | R | R | R (+children R) | - | R | - | W (coalesced) |
| `notes/*.md` | R | R | R | R | - | A W D | A W D | A (create, capture, migration) |
| `handovers/draft-<cur>.md` | A W | A W | A W | A W | - | W D (stage dialog) | W | D, rename on move |
| `handovers/draft-<other>.md` | - | - | - | - | - | W D | W | D |
| `handovers/<NNNN>-*.md` | R | R | R | R | - | W D | W | A (move, run, brief, pass) |
| `links.yaml` | A W own entries | same | same | same | - | W | W | W (UI actions, migration) |
| `proofs/<slug>/*` | A, W own | A, W own | A, W own | A, W own | - | D (delete proof) | W | W (restore on reject), D |
| children's folders | - | - | - | R | - | - | - | W |
| `children/<draft>/*` (D-Q2 F) | - | - | - | A W D | - | R | W | D (after create) |

Collision rules for multi-writer files:

| File | Rule | Residual, why acceptable |
|---|---|---|
| notes | one file per note; agent change = prompt; user writes win silently | two windows editing one note: last UI save wins, as today |
| draft of current stage | hash check: user or another session changed it after this session's last sync and this session's version differs = warn-and-confirm prompt | today's `UpsertHandoverDraft` overwrites silently; this is strictly better |
| handed handovers | one file each; agent change = prompt | none |
| `links.yaml` | entry diff: new and own entries silent; others' entries = prompt; file changed by someone else after this session's sync = warn-and-confirm | small file, rare overlap; per-entry files rejected to keep one read for agents |
| proofs | single owner (creating session); another agent's change = prompt | user deletes a proof an agent still edits: next submit recreates it as new, prompt shows "new proof" |
| `task.yaml`, `state.yaml`, README | single owner Kira; any other write restored | an unsandboxed agent can scribble; Kira restores at the next submit or rewrite |
| `children/<draft>/*` (D-Q2 F) | single owner the orchestrator task; nothing is accepted until the user approves the task | user hand edit before submit goes into the request; the approval shows the payload, so the user sees it |

"Last sync" of a session = its launch or its last accepted submit; held in memory per session key
(restarts end interactive sessions; recovered headless runs start with no base, so a collision then
falls back to plain classification plus notify).

## 7. Submit

### 7.1 Tool

`submit` on `kira-space`: `{paths?: [relPath], finished?: bool}`. Empty `paths` = scan whole task
folder. `finished` replaces `handover`'s flag for sessions without a run: it raises the "move to
next stage" prompt (state signal, so it is an argument, never a file field). Granted to every
Space grant (runs, sessions, review agent, orchestrator).

`finish_step` keeps status, reason, conflictedFiles, lastGitError, tried; `summary` goes. It runs an
implicit submit first. Status of an ok result with a route that continues and no draft for the
step is refused: "write handovers/draft-<step>.md, then call finish_step" (Q7). Reply: ack, submit
results, files changed but not accepted, the quick artifact list as today. The implicit submit
skips child drafts (D-Q2 variant F): `finish_step` keeps returning at once, and the reply lists each
unsubmitted `children/<draft>/` with "call submit to request this task".

kira-studio gets no `submit` and keeps no proof tool (Q10): a proof is written only in a task
folder and submitted through kira-space `submit`. A DB MCP client outside a task (external client,
Studio session) can no longer make a proof.

### 7.2 Algorithm (under `taskMu`)

1. Walk the folder (depth cap 3, `Lstat`; symlink, hard link count above 1, FIFO, device = refused
   per file; at most 500 files and 64 MiB per task, 32 MiB per proof).
2. Classify path by pattern into kind; no match = `ignored`.
3. `Store.Stat`: size and version equal to `task_files` = unchanged, no read.
4. Read up to the kind cap, sha256. Equal to `accepted_sha256` = refresh version only.
5. Parse and validate: configfs YAML pipeline (line, column), kind schema, kind rules (handover
   16 KiB, note 1 MiB, link rules, proof validators exported from `internal/proof` as
   `ValidateStep(view, raw)`, image checks of section 10 on the relative file). Invalid = per-file
   `invalid` with message; the cache keeps last good; other files continue (P638 pattern).
6. Missing file with an index row = proposed delete.
7. Semantic diff against the cache: text = unified hunks (go-udiff); links = entry add, change,
   remove; proof = step diff by id plus summary and reason.
8. Decide per change from the prompt matrix (7.3) with writer role and entry owner.
9. Collision upgrade: `accepted_at` after this session's last sync and `accepted_by` not this
   session = warn-and-confirm prompt showing who changed it and when.
10. Apply silent changes: intent row, cache rows and `task_files` in one transaction, blob copy for
    images, intent removed. Prompted changes: `review = pending`, one approvals item per submit.
11. Rejected and read-only writes: restore file from cache (or regenerate a Kira file), intent
    first, then atomic write.
12. Reply per file: `accepted`, `pending`, `rejected` (reason), `invalid` (message, line),
    `restored` (read-only), `unchanged`, `ignored`. Update the session's last sync.
13. D-Q2 variant F: child drafts validated in step 5 open their `task.create` approvals here, then
    `taskMu` is released and submit waits for those answers only (section 7.5); steps 10 to 12
    already ran for every other file. The reply goes out after the wait, one entry per draft.

Kira's own writes (UI edits, stage move rename, restore): intent row, atomic file write
(`renameio`), then cache and index in one transaction, then intent removed. Start-up reconcile:
an intent whose file hash matches completes the DB side; otherwise drops. File is truth, retry safe.

### 7.3 Prompt and notify matrix

| Change by an agent | Result | Who hears |
|---|---|---|
| Draft of current stage, created or edited | silent accept | board push; panel shows "draft updated" |
| Draft of another stage | rejected, file removed: "The task moved on to <step>" (today's text) | agent |
| Handed handover edited or deleted | prompt (diff, Approve, Deny) | user via approvals |
| User note edited, deleted or created | prompt | user |
| New link, edit or remove of own link, empty Jira or PR slot filled | silent accept | panel forward chip shows to user |
| Edit or remove of another author's link, Jira or PR slot replaced | prompt | user |
| New proof | accept, existing "proof ready" notification (kept as today) | user |
| Edit of an accepted proof | pending review, existing `proof.review` approval with step diff | user |
| `task.yaml`, `state.yaml`, README written | restored | agent (`restored`) |
| Collision (7.2 step 9) | warn-and-confirm prompt | user |
| Invalid file | `invalid` with line | agent; UI problem strip |
| Child draft `children/<draft>/` (D-Q2 F) | today's `task.create` approval; blocks up to 30 min | user via approvals, desktop and phone |

Blocking per item follows the old tool (Decisions, Q6 changed): a blocking item holds the `submit`
call, a non-blocking one returns `pending` at once. Silent-accept rows raise no notification
(Decisions).

Old tool behaviour, read from handler code at base `c9072e38d`. a = returns at once; b = blocks for
the user's answer; c = waits on something else. `agentmcp/` is `internal/agentmcp/`; other paths
sit under `apps/kira-space/internal/`.

| Tool | Server | Behaviour | Code |
|---|---|---|---|
| `finish_step` | kira-space | a: stores the finish, applied at run exit; reply = ack plus artifact list | `agentmcp/mcp.go:377-404`, `ade/runs.go:51-59` |
| `handover` | kira-space | a: upserts the stage draft; `finished` opens the "move to next stage" prompt and does not wait; task moved on = refusal at once | `agentmcp/handover.go:33-66`, `ade/handover.go:71-100` |
| `task_info` | kira-space | a; with `waitSeconds` (0 to 600) c: waits for the branch's prepare script, with progress; ends at setup end, deadline or caller gone | `agentmcp/space.go:340-381`, `ade/agenttools.go:407-440` |
| `declare_repos` | kira-space | a | `agentmcp/space.go:383-406`, `ade/agenttools.go:223-272` |
| `request_branches` | kira-space | a for branches whose worktree exists; rest b: one task request, wait up to 30 min (`defaultBranchRequestWait`). Timeout: entry outcome `timeout`, "No answer from the user in 30 minutes; nothing was created. Call request_branches again later or ask the user." Deny: `denied` with `deniedText`. Caller gone: items withdrawn, tool error "The branch request was cancelled." | `agentmcp/space.go:408-432`, `ade/branchrequest.go:31,442-489,775-804,880` |
| `rename_branch` | kira-space | refusal a; else b, same request and 30 min wait; timeout `timeout`, deny `denied` | `agentmcp/space.go:434-456`, `ade/branchrename.go:48-71` |
| `change_branch` | kira-space | refusal a; else b, same request and wait; timeout `timeout`, deny `denied` | `agentmcp/space.go:458-480`, `ade/branchchange.go:37-60` |
| `report_artifacts` | kira-space | a: links stored at once; kind proof attaches a closed proof by name at once | `agentmcp/space.go:482-527`, `ade/artifacts.go:527-536`, `ade/proof.go:59` |
| `operations` | kira-space | a: list; cancel sends the signal and does not wait for the op to end | `agentmcp/space.go:310-338`, `ade/agenttools.go:568-598` |
| `acquire_lease` | kira-space | a: `held`, or `queued` with position; the grant comes later as a notice (session message, hook reply, next lease reply) | `agentmcp/leases.go:80-101`, `ade/leases.go:373-398` |
| `release_lease`, `lease_status` | kira-space | a | `agentmcp/leases.go:102-143`, `ade/leases.go:400-431` |
| `run_outcome` | kira-space | a: reads stored runs | `agentmcp/outcomes.go:53`, `ade/agenttools.go:476-547` |
| `create_task` | kira-space | b: `task.create` approval, wait up to 30 min (`defaultTaskRequestWait`); outcomes `timeout`, `denied`, `failed` (withdrawn), each with a `next` text | `agentmcp/orchestrator.go:179-204`, `ade/taskcreate.go:26,198-235` |
| `list_tasks`, `task_chain` | kira-space | a | `agentmcp/orchestrator.go:206-264`, `ade/orchestrator.go:142-260` |
| `proof_save` | kira-studio | a. New: `closed`, ready prompt, attach to the session's task. Edit: `pending_review`, `proof.review` approval raised, call does not wait | `dbmcp/proof_tools.go:73-109`, `proof/service.go:126-151,258-276` |
| `proof_list` | kira-studio | a; the only place an agent saw an edit's answer (`pendingEdit`) | `dbmcp/proof_tools.go:112-119`, `proof/service.go:287-297` |
| db tools (`run_query` and peers) in prompt mode | kira-studio | b through the approval broker: wait up to 10 min (`ApprovalTimeout`); outcomes denied, timedOut, abandoned, queueFull. Proof tools bypass the broker | `dbmcp/approval.go:22,368-440`, `dbmcp/proof_tools.go:16-17` |

Submit item to old tool:

| Submit item | Old tool | Old | Submit |
|---|---|---|---|
| Draft of current stage | `handover`, `finish_step` summary | a | accepted at once |
| `finished: true` | `handover {finished}` | a, prompt raised | accepted, prompt raised, no wait |
| Draft of another stage | `handover` refusal | a | rejected at once |
| New link, own link, empty Jira or PR slot | `report_artifacts` | a | accepted at once |
| Other author's link, Jira or PR slot replaced | none | - | `pending` at once |
| User note created, edited, deleted | none (user only) | - | `pending` at once |
| Handed handover edited, deleted | none (no agent write) | - | `pending` at once |
| New proof | `proof_save` new, `report_artifacts` kind proof | a | accepted at once, ready prompt |
| Proof edit | `proof_save` edit | a, `pending_review` | `pending` at once, `proof.review` approval |
| Proof deleted by an agent | none (user only, `bridge/proof.go:252`) | - | `pending` at once; deny restores |
| `task.yaml`, `state.yaml`, README written | none (Kira-owned) | - | `restored` at once |
| Collision (7.2 step 9) | none | - | `pending` at once |
| Invalid file | none | - | `invalid` at once |
| Child draft (D-Q2 F) | `create_task` | b, 30 min | b: same approval, same 30 min wait; reply = `create_task`'s |

Items with no old tool follow the closest analogue, `proof_save` edit: pending, not blocking.

Result: of today's items none maps to a blocking old tool. Tools that blocked for the user
(`request_branches`, `rename_branch`, `change_branch`, db tools) stay tools (`branches`,
kira-studio db tools), so their wait is unchanged. The one wait on something else (`task_info
waitSeconds`) moves to `branches {action: wait}`. `create_task` depends on D-Q2: variant T keeps
the tool and its wait; variant F turns it into the child draft item, the one blocking submit item.
So under variant T `submit` never blocks. Under variant F a submit holding a child draft applies
every other item first, then waits for the drafts only and reports all items in one reply (the
mixed rule). The implicit submit of `finish_step` skips drafts and never blocks (7.1).

A `pending` item's answer reaches the agent through the lease notice path (`SendIfIdle`, hook
reply, next tool result `notices`) and the `pending` list in `state.yaml`. Today a `proof_save`
edit's answer showed only in `proof_list`, so this is more, not less. `proof_save`'s
`pending_review` is non-blocking: `CloseSession` returns at once after raising the approval
(`proof/service.go:258-276`). Deny restores the accepted copy on disk. A new submit of the same
file replaces its open item. Approvals kind `details.edit` (new) reaches desktop and phone
(Decisions, Q15 changed).

User changes:

- UI edit: Kira writes the file, accepted at once; recorded in the panel forward set as today, the
  forwarded message names the relative path.
- Hand edit while the app runs: no watcher. Kira stats the task folder on task open, Details tab
  focus, Refresh, every submit and `finish_step`, and at start for live tasks. A change not made
  through submit or the UI shows the banner "N files changed on disk, not submitted" with a diff per
  file, Accept or Discard. Kira cannot tell a hand edit from an agent write that skipped submit, so
  both need this one click; Accept is the user's consent.

### 7.4 Edit protection

- Agents read freely. No lock: no true file lock binds a non-cooperating writer.
- Ask-to-edit through the matrix, enforced at submit; Kira restores anything rejected.
- Sandbox on: task folder in `Allow`, literal write denies on `README.md`, `task.yaml`,
  `state.yaml`; orchestrator gets children folders in `Read`. Sandbox off (default, and every
  Linux host): `DisallowedTools` `Edit(//<abs>/task.yaml)` and `Write(...)` for the same files;
  restore covers the rest.
- Every run, session and review agent gets `--add-dir <home>/tasks/<id>` (and children for an
  orchestrator) automatically. Nothing outside `<home>/tasks/<id>` is granted.

### 7.5 Child tasks (Decision D-Q2: depends on F6 verdict)

The one switch. Part 5 reads the `D-Q2` verdict line in `docs/v0.3.14/plans/P643-spike-results.md`
("Spike" section, written by Part 1) and builds exactly one variant; the other stays unbuilt,
never half-built.

- Verdict rule, F6 haiku medians against arm T: files arm cost at most 1.1 times, turns at most T
  plus 1 per child (plus 2 for F6's two children), oracle pass rate not lower. All three pass =
  `D-Q2: F` (more efficient or the same). Any fail = `D-Q2: T`.
- Who decides: the orchestrating session, from the results file's numbers. Borderline (any F6
  metric within 10 percent of its threshold) = an Opus planner reviews the raw runs and writes the
  verdict line with its reason. The verdict line is committed before Part 5's plan starts.

Variant T (F6 fails): `create_task` stays a tool with its approval, exactly as today
(`agentmcp/orchestrator.go:179-204`, `ade/taskcreate.go:198-235`); brief written as the child's
first handed file. Orchestrator run tools 15 to 6. Nothing below applies.

Variant F (F6 passes, default):

- Draft: `children/<draft>/` in the orchestrator's own folder. `<draft>` is a slug (proof slug
  rules), unique per orchestrator, case folded. `task.yaml`:

  ```yaml
  # Kira child task draft. Edit, then call submit. Rules: ../../README.md
  kira: child/1
  title: Backfill order totals
  workflow: implement        # workflow id or name
  repos:
    - {repo: app, base: main} # nickname, name or code repo id; base optional
  ```

  `brief.md`: body only, the child's first handover. No `kind` (a child is always kind `task`, as
  `buildNewTask(..., AdeTaskKindTask, ...)` today) and no `depends` (`create_task` has none; not
  added here).
- Detection: submit classifies a `children/<draft>/` with both files as one item, kind
  `child-draft`. A draft in a non-orchestrator task: `invalid` "Only an orchestrator task creates
  tasks." (today's text).
- Validation, same checks and texts as today (`cleanCreateTask`, `planChildTask`): title 1 to 200
  runes; workflow required, resolves by id then exact name and has a step; brief required, at most
  16384 bytes (`MaxHandoverBytes`); repos at most 10, each non-empty, registered, not listed twice;
  base resolves (`storedBase`). Errors name the file and line (YAML pipeline). Bad input fails
  before any prompt, as today.
- Approval: today's `openTaskRequest` unchanged: kind `task.create`, same `taskCreatePayload`
  (parent id and title, title, workflow, repos, brief), same Origin, Target, Operation, title text
  and `taskCreateOwner`. One item per draft, as today. The phone's `task.create` approval (P637)
  keeps working: same item, only the caller changes. Run note `noteTaskApproval` and agent op
  `opAgentApproval` kept.
- Wait: blocks like `create_task` (`SpawnTask`): up to `defaultTaskRequestWait` 30 min, without
  `taskMu`; drafts of one submit wait concurrently on one deadline. Approve: `createChild`
  unchanged (re-checks the parent under `taskMu`, plan day or backlog). Kira then writes the child's
  folder (README, `task.yaml`, `state.yaml`, brief as `handovers/0001-<orch>-to-<entry>.md`).
- Reply per draft = today's `CreateTaskResult`: `outcome` (`created`, `denied`, `timeout`,
  `failed`), `taskId` when created, `title`, `next`. Today's `next` texts kept; "Call create_task
  again" becomes "Call submit again". `create_task` returns no preview; nor does this reply. The id
  also lands in the orchestrator's `state.yaml` `chain.children[]` with `draft: <draft>`.
- Idempotency: table `task_child_drafts(task_id, draft, sha256, state, child_id, reason,
  updated_at)`, PK (task_id, draft); `sha256` over `task.yaml` plus `brief.md`; `state` in
  `pending|created|denied|timeout|withdrawn`. The `created` row is written in the same transaction
  as `CreateChildTask` (repo call gains the marker), then the draft folder is removed; start-up
  reconcile removes the folder of any `created` row. Resubmit:
  - `created`, same hash (folder came back, or crash before removal): reply `created` with the
    stored `taskId`, folder removed, no prompt, no second task.
  - `created`, other hash: `invalid` "draft <draft> already created task <id>; use a new folder
    name".
  - `pending`, same hash: joins the open request, no second prompt. Other hash: the open item is
    withdrawn (its waiter gets `failed`, "replaced by a newer submit") and a new one is raised.
  - `denied`, same hash: refused at once, today's "Do not retry it unchanged" text. Other hash: new
    prompt.
  - `timeout` or `withdrawn`: new prompt (today: "Call create_task again").
- Denied: row `denied`, reason = today's deny text (the item carries no user options); folder
  stays; `state.yaml` `drafts: [{draft, state, reason, since}]` lists every not-created draft. Agent
  edits and resubmits.
- Caller gone: as `SpawnTask` `ctx.Done`: request withdrawn, row `withdrawn`, folder stays.
- Sandbox: the draft sits in the orchestrator's own granted folder, so writing it needs no new
  grant. The created child folder is outside the fixed Seatbelt profile and `--add-dir` list, as
  before: `chain.children[].preview` (first 500 bytes) and the reply cover it until the next
  session (section 8).
- Migration: none; no draft exists before Part 5.

Tool tables per variant (orchestrator run, conditional tools excluded):

| Tool | Variant T | Variant F |
|---|---|---|
| `submit`, `finish_step`, `branches`, `lease`, `cancel_operation` | kept | kept |
| `create_task` | kept, brief written as first handed file | removed: `children/<draft>/` plus `submit` |
| `list_tasks`, `task_chain` | removed: `state.yaml` `chain` | removed: `state.yaml` `chain` |
| Count | 15 to 6 | 15 to 5 |

## 8. State file (Part 5)

`state.yaml`, header: `# Read-only. Kira rewrites this file. Change state only with Kira tools:
branches, lease, cancel_operation, finish_step.` Coarse state only:

```yaml
rev: 41                       # increments per rewrite; no file times
task: {id, kind, status, step: {id, name, position, of}, workflow}
repos:                        # per repo: branch, base, worktree, setup {state, note}, mismatch
registered: [...]             # only while the task has no repo (task_info rule today)
leases: [{queue, state: held|queued, position, holder}]
operations: [{opId, kind, subject, startedAt}]
runs: [{runId, kind, step, state, outcome}]   # last 3; outcome capped at 2 KiB
chain:
  parent: {id, title, path: ../<id>/}
  children: [{id, title, status, step, finished, archived, lastHandover: ../<id>/handovers/<file>, preview, draft}]
pending: [{path, kind, since}]  # submit items waiting on the user
drafts: [{draft, state, reason, since}]  # D-Q2 variant F, orchestrator only: drafts not created
```

Worktree paths are absolute: the mirror is host-local state, regenerated per host, never content
(Q4). Writes: one writer goroutine, per-task dirty flag, coalesce 150 ms (inside 100 to 250),
skip when bytes equal, `renameio`. Triggers: board change (`notifyBoard` `taskChanged`), lease
book change, op start and end, setup change. Live tasks only; one final write at archive.

`task.yaml`: id, title, kind, workflow, parent, repos, created. Same header and writer.

Orchestrator children under an enforced sandbox: a child created after launch is outside the fixed
Seatbelt profile and `--add-dir` list. `chain.children[].preview` (first 500 bytes, as
`list_tasks` today) and the create reply (`create_task` under D-Q2 variant T, the child draft's
`submit` entry under variant F) cover it until the next session.

## 9. MCP tools before and after

| Tool | Server | Before | After |
|---|---|---|---|
| `finish_step` | kira-space | runs; summary = handover | kept; summary moves to draft file; implicit submit |
| `run_outcome` | kira-space | task grants | removed: `state.yaml` `runs` |
| `handover` | kira-space | sessions without run | removed: draft file plus `submit {finished}` |
| `task_info` | kira-space | Space | removed: `task.yaml`, `state.yaml`; wait moves to `branches wait` |
| `declare_repos` | kira-space | Space | merged: `branches {action: declare}` |
| `request_branches` | kira-space | Space | merged: `branches {action: request}` |
| `rename_branch` | kira-space | Space | merged: `branches {action: rename}` |
| `change_branch` | kira-space | Space | merged: `branches {action: change}` |
| (new) | kira-space | | `branches {action: wait}`: today's `BranchStatus` wait, no polling |
| `report_artifacts` | kira-space | Space | removed: `links.yaml`, proofs folder, `submit` |
| `operations` | kira-space | Space | list: `state.yaml`; cancel: `cancel_operation` |
| `acquire_lease` | kira-space | Space | merged: `lease {action: acquire}` |
| `release_lease` | kira-space | Space | merged: `lease {action: release}` |
| `lease_status` | kira-space | Space | removed: `state.yaml` `leases`; notices stay on `lease` replies |
| `create_task` | kira-space | orchestrator | D-Q2 (7.5): variant T kept, brief written as the child's first handed file; variant F removed, `children/<draft>/` plus `submit` |
| `list_tasks` | kira-space | orchestrator | removed: `state.yaml` `chain` |
| `task_chain` | kira-space | orchestrator | removed: read own and children's `handovers/` |
| (new) `submit` | kira-space | | every Space grant |
| `git`, `reply_review_comment`, `fetch_remote`, `push_branch` | kira-space | conditional | unchanged |
| `proof_save`, `proof_list` | kira-studio | every DB MCP client | removed for every client: proof files in a task folder plus kira-space `submit`; no proof outside a task (Q10) |

Counts (conditional tools excluded): run 12 to 5 (`submit`, `finish_step`, `branches`, `lease`,
`cancel_operation`); session 12 to 4; orchestrator run 15 to 6 (D-Q2 variant T) or 15 to 5
(variant F). kira-studio proof tools 2 to 0 for every
client. Lease ranking, heartbeat and notices keep working: `lease` calls are the
heartbeat (`mcpRecorder`).

## 10. Proofs

- A proof exists only in a task folder: `<home>/tasks/<id>/proofs/<slug>/` (Q10). Slug from name,
  unique per task (case folded, as `name_key`).
- New proof: files plus `submit`; Kira creates session, revision and steps rows (caches), links it
  to the task (state), raises the ready prompt. Edit: pending revision from disk version, current
  revision stays the accepted copy; approve copies view state of unchanged steps by step id;
  deny restores files from caches and blobs. Step diff and `origin_ordinal` derive from step ids.
- Delete: the user deletes a proof from its Details entry or its window; folder and rows go.
- Retention of 30 (`pruneClosed`) goes: files on disk stay until the user deletes them or the task
  folder goes.
- View state stays in `proof_view_state` (UI state, keyed by session, rev, ordinal).
- Window: one proof per window, opened by (task id, slug). Entry points: Details chain entry
  (`AdeProofEntry.vue`), ready prompt (`ProofReadyPrompt.vue`), `proof.review` approval
  (`approvals/ProofOpenWindow.vue`). Tabs per step, explanation, data views, compare side for a
  pending edit, screenshots unchanged. Header shows the relative path with Open and Reveal.

Removed by Q10 (today's behaviour outside a task, verified in code):

- `proof_save`, `proof_list` are on the shared kira-studio server for every client
  (`dbmcp/server.go:395-397`); outside a task `proof_save` makes an unattached session. Removed:
  `addProofTools`, `ProofStore`, `ProofAttacher`, `dbmcp.Config.Proof` and `ProofAttach`
  (`dbmcp/proof_tools.go`, `bridge/dbmcp.go:63-64,136-137`), `ade.TaskBoard.AttachProof`
  (`ade/proof.go:59`) and `report_artifacts` kind proof. External DB MCP clients and Studio sessions
  lose proofs.
- Window menu "Validation proof" opens the window with no task (`appshell/menu.go:16-17,67`,
  `appwire/menuactions.go:14-17`). Removed with its tests (`appshell/menu_test.go:33`,
  `proofflow/image_test.go:267-281`). `ProofService.OpenWindow` with no session
  (`bridge/proof.go:279-320`) goes: an open names (task id, slug).
- History pane lists every closed session, newest first, with delete (`ProofHistory.vue`,
  `ProofService.Sessions`, `bridge/proof.go:201`). Removed.
- Unattached sessions and attach: `ProofChoices` (`ade/proof.go:148`, `bridge/adetask.go:1020`),
  `AdeProofAttachMenu.vue`, `useProofChoices`, `useLinkProof`, `LinkProof`,
  `proof.Service.Unattached`, `ProofRepo.UnattachedSessions`. Removed.
- Detach returns a proof to the unattached list (`proof/tasklink.go:74-81`, `UnlinkProof`,
  `useUnlinkProof` in `AdeProofEntry.vue`). Removed: delete replaces it.
- Retention prunes unattached sessions beyond 30 (`repos/proof.go:512-544`, `ProofKeepClosed`).
  Removed; nothing is unattached any more.
- Agent draft path (`proof.Service.Save`, `StartSession`, `EditSession`, `AddStep`,
  `CloseSession`, `PurgeStaleDrafts`): replaced by submit; Part 4 removes each one with no caller
  left.

### Images (Q17)

Why JPEG only today (`proof/image.go:18-27,91-112`, `ProofImage.vue:8,64`): bytes are stored as
base64 inside the step payload, capped at 4 MiB (`maxPayloadBytes`), so the file cap is 3 MiB less
1 KiB; the integrity decode uses stdlib `image/jpeg`, its memory bounded by the 16 MP area cap; the
window hard-codes `data:image/jpeg` (window CSP allows `data:`, not `blob:`). No rule needs JPEG
itself. Files remove the base64-in-payload reason.

- Allowlist: PNG, JPEG, WebP, SVG. PNG and JPEG decode with stdlib `image/png`, `image/jpeg`. WebP
  needs `golang.org/x/image/webp` (BSD-3, Go project, new dependency; Part 4 confirms the license
  per feature): lossy, lossless and alpha, no animation. WebKit on macOS 11+, WebView2 and
  WebKitGTK render all four. WebP is the least needed; dropping it removes the dependency and
  nothing else.
- Refused: GIF, BMP, TIFF, HEIC, AVIF, gzip SVG (`.svgz`), any animated raster (APNG `acTL` chunk
  before `IDAT`, which `image/png` ignores while the webview would animate; WebP VP8X animation flag
  or `ANIM` chunk).
- Sniff by content, never by extension: PNG `89 50 4E 47 0D 0A 1A 0A`, JPEG `FF D8 FF`, WebP
  `RIFF` then 4 bytes then `WEBP`; SVG = after optional BOM, whitespace, `<?xml?>` and comments,
  root element `svg` in namespace `http://www.w3.org/2000/svg`. Extension must agree: `.png`;
  `.jpg` or `.jpeg`; `.webp`; `.svg`.
- Raster checks in order: size at most 10 MiB (from `Lstat`, before read; fits a 4K PNG screenshot, under
  the 32 MiB proof cap); `DecodeConfig` of the sniffed format; each side 1 to 16384 pixels (WebKit
  decode limit) and area at most 16 MP; animation check; full decode as integrity check (area cap
  bounds memory: 16-bit RGBA PNG 8 bytes per pixel = 128 MB, near JPEG's 150 MB today).
- SVG checks, on submit, with `encoding/xml` (no raster decode). Reject, never sanitize: the error
  names element or attribute and line, the agent fixes its file, Kira never rewrites it (section 5).
  - Size at most 2 MiB (as `MAX_SVG_BYTES` of Mermaid diagrams, `diagramRender.ts:4`), at most
    50,000 elements, depth at most 64. Not well-formed XML refused.
  - Any `xml.Directive` token (DOCTYPE, ENTITY) refused: no entity expansion, no external DTD.
  - Elements refused: `script`, `foreignObject`, `iframe`, `object`, `embed`, `audio`, `video`,
    `animate`, `animateMotion`, `animateTransform`, `set` (SMIL runs inside `<img>`; a proof is a
    still, as for rasters).
  - Attributes refused: every `on*` event handler; any value holding `javascript:`.
  - `href` and `xlink:href`: only `#fragment` or `data:image/png|jpeg|webp;base64,...`; the embedded
    raster passes the raster checks and counts toward the 2 MiB. No nested SVG data URL, no `http:`,
    `https:`, `file:` or relative path.
  - CSS (`<style>` text and `style` attributes) and presentation attributes: `url()` only to
    `#fragment` or an allowed `data:image`; `@import` and `@keyframes` refused.
  - Intrinsic size without decode: `width` and `height` in px or unitless, else `viewBox`; one of
    them required; percentages without `viewBox` refused; each side 1 to 16384, area at most 16 MP.
- Step payload: `{format, width, height, bytes, file, sha256}`, no bytes. `ProofImagePayload`
  (`packages/shared/domain/proof.ts:96-103`) gains `format` (`png | jpeg | webp | svg`) and drops
  `base64`. The bridge reads the accepted file (or its blob) and returns base64 plus format.
- Rendering: `ProofImage.vue` builds `data:image/<png|jpeg|webp|svg+xml>;base64,...` for an `<img>`
  with the payload's width and height. Window CSP is `img-src 'self' data:` (`frontend/index.html:7`)
  and stays. SVG goes only through `<img>`: never `v-html` or inline DOM (unlike Mermaid diagrams,
  inlined after DOMPurify), never `iframe`, `object` or `embed`. In `<img>` the webview runs no
  script and loads nothing external, a second guard after the validator.
- Screenshots: `modern-screenshot` `domToCanvas` (`screenshot.ts`) draws the `<img>`; an SVG
  without `foreignObject` does not taint the canvas, so copy and save keep working. Proofs do not
  reach the phone (no proof code in `mobileweb` or `frontend/mobile`); nothing to change there.
- Migration: today's base64 rows are JPEG; written as `.jpg`, format `jpeg`, no re-encode.
- Errors name file and step id, as other step errors:
  - `proofs/<slug>/<file>: step <id>: not a PNG, JPEG, WebP or SVG image. Allowed: .png, .jpg,
    .jpeg, .webp, .svg; no GIF or animation.`
  - `...: SVG element <script> at line 12 is not allowed (no script, foreignObject, iframe, object,
    embed, audio, video or animation).`
  - `...: attribute onload on <rect> at line 4 is not allowed (no event handlers or javascript:).`
  - `...: href "https://x.test/a.png" on <image> at line 9 points outside the file; use #id or an
    embedded data:image PNG, JPEG or WebP.`
  - `...: CSS at line 3 loads from outside the file (@import or url() to a non-fragment); inline
    it.`
  - `...: SVG has a DOCTYPE or entity declaration at line 2; remove it.`
  - `...: SVG needs a viewBox, or width and height in px; each side 1-16384, at most 16 megapixels.`
  - `...: SVG is <n> KiB; the limit is 2048 KiB.`
  - `...: SVG is not well-formed XML at line <l>: <parser message>.`
  - `...: the file holds a PNG image but ends in .jpg; rename it to .png.`
  - `...: animated images are not supported; save a single frame.`
  - `...: image is <w>x<h>; each side must be 1-16384 pixels and the area at most 16 megapixels.`
  - `...: image is <n> KiB; the limit is 10240 KiB per image.`
  - `...: <format> data is damaged and does not decode.`
- An accepted image changed in place counts as a change; its accepted bytes sit in `tasks-blobs`
  for restore. Relative file in the proof folder only; the absolute-path source of `proof_save`
  goes (the app no longer reads arbitrary paths).

## 11. Migration (one-time, resumable)

Framework in Part 2, content exporters in Parts 3 and 4.

1. No backup (Q11 changed): no `VACUUM INTO`, no backup retention. Safety comes from per-task
   verify before rename and from rewriting a task's image payloads only after its verify passes.
2. Per task, live and archived, marker in `task_files_migration`: export to
   `<home>/tasks/<id>.migrating/`, verify (re-read, parse, per-row body hash equal, counts equal),
   rename to `<home>/tasks/<id>/` (merging into an existing folder only for files absent there),
   insert `task_files` and `rel_path`, mark done. A crash leaves `.migrating`: deleted, task redone.
3. Notes to `notes/`, drafts to `handovers/draft-<step>.md`, handed rows to numbered files in
   `created_at` order. Child mirror rows: dropped when the child's own handed-into-done file
   exists (parent chain derives it from the child); else written as a read-only legacy handed file.
4. Stored artifacts plus Jira and host slots to `links.yaml`.
5. Proofs: linked sessions to the task's `proofs/<slug>/`; image payloads decoded to `.jpg`
   (format `jpeg`), payload column rewritten to metadata after the task's verify. A pending
   revision becomes the disk version with review pending; its approval re-raises
   (`RaiseUnreviewed`). Unattached sessions (no task, so no folder under Q10): Q16; recommended,
   rows deleted after a log line naming each, since retention would have pruned them past 30 anyway.
6. Columns stay as caches; nothing else dropped (core idea, section 3). Image base64 rewrite is
   one-way: no backup (Q11 changed); the `.jpg` file is the only copy once verify passed.
7. Child drafts (D-Q2 variant F): nothing to migrate; no draft exists before Part 5.
8. Migration runs at start before the board starts, with a progress line in the log; a failure
   leaves the task on SQLite reads (flag per task) and an error banner; no partial task.

## 12. UI changes

- Details tab: same layout, same SQLite queries. New folder bar (relative path, Open, Reveal, Copy
  path) through a bound open/reveal service scoped to `tasks/<id>` (configsvc pattern, no renderer
  path reaches the OS). New `AdeDetailsDiskBanner.vue` (changed on disk, not submitted: per-file
  diff dialog, Accept, Discard; shadcn-vue Dialog, Alert). File problem strip (invalid file, line,
  Open) reusing `ConfigFileProblem` styling.
- Pending edits: approvals dialog body for `details.edit`: one row per file change, diff, per-item
  approve like `branch.request` items. Proof edits keep `proof.review`.
- Proof window: no history pane, opened per proof from a task only, path header, image by format.
  Window menu item, attach menu and detach action removed (section 10).
- Panel forward bar: item labels carry the relative path.
- Child task approval (D-Q2 variant F): today's `task.create` approval view on desktop and phone,
  unchanged. Details of an orchestrator lists `children/<draft>/` read-only in the folder bar with
  its `state.yaml` `drafts` state.
- Stage dialog and handover prompt: unchanged (read SQLite caches).
- Mobile: unchanged (reads caches).
- Server state through TanStack Query; banner state through a query on a new `DiskChanges` call;
  no new Pinia store unless a component needs shared client state.

## 13. Storage seam (future cloud)

`apps/kira-space/internal/taskfiles/store`: interface `Store { Stat, Read, List, WriteAtomic(rel,
data, ifVersion), Remove, Rename }`, relative paths only, `Version` opaque (local: size plus mtime
ns; object storage: ETag). Only the local adapter joins a root, follows the filesystem, checks
symlinks and reads file times. Everything else (submit, validators, migration, UI wire) carries
relative paths. Agents still need a local directory: a cloud build would sync or mount it; out of
scope, the seam keeps it open. Loopback NFS noted as a later option, not built.

## 14. Spike (Part 1) and gate

Goal: measure tokens and turns for typical flows, tools versus files, before building.

- Real `claude` test in the nested module: `apps/kira-space/tests/claude/details_spike_test.go`,
  fixtures in `apps/kira-space/tests/claude/testdata/details-spike/`.
- Arm T (tools): real `agentmcp.Server` with a stub `SpaceTools` backend, so tool list and texts
  are today's exact ones; kira-studio `proof_save` stub with today's schema. Part 1 also freezes
  that tool list (names, descriptions, schemas) to `testdata/details-spike/tools-baseline.json`;
  arm T serves the frozen list, so it stays runnable after Parts 3 to 5 remove the tools.
- Arm F (files): stub server with `submit`, `finish_step` (no summary), `branches`, `lease`,
  `cancel_operation`, plus a fixture task folder (README, `task.yaml`, `state.yaml`, notes,
  handovers, `links.yaml`). Stub `submit` runs real parse and validation code where it exists
  (YAML, proof validators), else shape checks.
- Flows: F1 handover (do a one-line change, hand over, finish); F2 new proof (table plus image);
  F3 edit that proof (change one row, reason); F4 orchestrator summary of two children's last
  handovers; F5 wait for a branch setup to be ready; F6 orchestrator creates 2 child tasks (arm T:
  `create_task` twice with today's schema, stub approves; arm F: two `children/<draft>/` folders
  plus `submit`, stub runs the real draft validation where it exists and approves).
- Runs: haiku, 3 per flow and arm (36 runs), plus 1 sonnet run per flow and arm (12). Budget cap
  0.25 USD per run, 5 USD total (Q12; F6 adds 8 runs inside the same cap).
- Measure from the stream-json result: `num_turns`, input, cache read, cache write and output
  tokens, `total_cost_usd`, tool calls by name, wall time, oracle pass (draft text present, proof
  valid, summary names both children, F6: both children created with the asked title, workflow,
  repos and brief). Context per run from per-message `usage`: turn-1 context (input plus cache
  read plus cache write of the first assistant message), final context (same, last message),
  growth (final minus turn-1).
- Results: committed file `docs/v0.3.14/plans/P643-spike-results.md`, written by Part 1 and
  extended by every later benchmark (14.1). Part 1 writes two sections: "Baseline" (arm T medians
  per flow: cost, turns, pass rate, turn-1 context, final context, growth), recorded before Part 2
  starts and never rewritten; "Spike" (arm F, gate result per flow, the `D-Q2` line). No test reads
  the file. Script `bun run test:claude:details` (new, `-run 'TestDetailsSpike|TestDetailsBench'`);
  real `claude`, opt-in, never in hooks or CI.
- Gate, per flow F1 to F5, haiku medians: cost of F at most 1.2 times T, turns of F at most T plus
  2, oracle pass rate of F at least T's. All five pass = go. Any fail = no-go report with the
  numbers and a fallback for the user to pick: (a) files stay source of truth but writes go through
  tools that write the files (keep `handover`, `proof_save` as file writers), or (b) stop P643
  after the spike.
- F6 is its own switch, not part of go/no-go: cost of F at most 1.1 times T, turns of F at most T
  plus 1 per child (plus 2), pass rate not lower. The "Spike" section ends with one line `D-Q2: F`
  or `D-Q2: T` and the three numbers; borderline rule and who decides in section 7.5.

### 14.1 Benchmarks through the build

Efficiency is measured for the whole migration, not only the spike.

- Harness: `apps/kira-space/tests/claude/details_bench_test.go` (`TestDetailsBench`), same nested
  module, models, run counts (haiku 3 per flow, sonnet 1), 0.25 USD per-run cap and metrics as the
  spike. Arm R runs against the real implementation: real `ade.TaskBoard`, `agentmcp.Server` and
  task folder, as `orchestrator_test.go` does today; no stub. Arm T is the spike's frozen-tool arm,
  rerun beside R so both share model and day. `bun run test:claude:details`; opt-in, never in
  hooks or CI.
- Per-Part benchmark, after each Part that changes what agents see, before that Part counts done:

  | After | Flows | Compared against |
  |---|---|---|
  | Part 3 notes, handovers, links | F1 | Baseline F1 |
  | Part 4 proofs | F2, F3 | Baseline F2, F3 |
  | Part 5 state file, tool removals | F4, F5, F6 (variant F: drafts; variant T: `create_task`, regression check) | Baseline F4, F5, F6 |

  Part 2 changes nothing agents see; no benchmark.
- Final benchmark, once Streams A and B have landed (end of Part 5): F1 to F6, arm T (old tools)
  against arm R (shipped files).
- Gate, per flow, haiku medians, same for per-Part and final: cost at most 1.2 times baseline;
  turns at most baseline plus 2; pass rate not lower; final context at most 1.0 times; turn-1
  context at most 1.0 times (plan reading: the requirement names turn-1 context without a bound).
  F6 keeps its D-Q2 bound (1.1 times, plus 1 per child) when variant F shipped.
- Kira-side cost, final benchmark, Go tests outside the `claude` module (deterministic, run in the
  normal loop; numbers written to the results file once at Part 5 end):
  - submit latency, one changed file in a 500-file folder, p50 and p95: p95 at most 200 ms;
  - stat on task open, 500-file folder: at most 5 ms (Part 2 bench);
  - `state.yaml` rewrite: time per rewrite at most 5 ms; writes per task under a 100-change lease
    storm at most one per 150 ms window (coalescing);
  - task-folder disk size: per-task median and total, against `kira.db` size before migration;
    reported, no bound;
  - migration time on a large DB (generated fixture: 2,000 tasks, 50,000 handovers, 5,000 notes,
    500 proofs with images, in the migration flow test's `testdata/`): reported with per-task
    median; bound 2 minutes total on the dev machine.
  Bounds above are plan estimates; Part 2's plan may tighten them with a measured reason, never
  loosen them silently.
- Failure: a Part that misses any metric goes back to its implementer, same phase, same number,
  and reruns its benchmark. A final benchmark miss is raised to the user with the numbers, never
  accepted. A borderline result (within 10 percent of a bound) gets an Opus review of the raw runs
  before the verdict.
- Results: each benchmark appends its own section ("Part 3", "Part 4", "Part 5", "Final",
  "Kira-side") to `docs/v0.3.14/plans/P643-spike-results.md`, committed. The P643 `SPEC.md`
  Result carries a short summary: per-flow final ratios, gate verdict, D-Q2 verdict, Kira-side
  numbers.

### Spike result (Part 1)

Full tables, per-metric verdicts and every run: `P643-spike-results.md`. Test:
`apps/kira-space/tests/claude/details_spike_test.go` (opt-in, `KIRA_SPIKE=run`; flows F1 to F6, F6
added by the user: orchestrator creates 2 child tasks). 48 runs, 0.97 USD. Gate (user re-weighted:
cost, final context, pass rate, edit ease; turns reported only), haiku medians F over T:

- Cost: F1 1.06, F2 1.38, F3 1.29, F4 1.15, F5 1.30, F6 1.24. Limit 1.2 (F6 1.1): passes F1 and F4 only.
- Final context: 1.02 to 1.08 in every flow. Limit 1.0: fails all six, by 2 to 8 percent.
- Pass rate: F3 files 2/3 (one run edited the proof and never submitted) versus 3/3; all else equal.
- Edit ease: F2 files worse (1.00 versus 0.67 failed or retried per run); others equal or better.
- Verdict: NO-GO on all six flows. The user picks: (a) files stay source of truth, writes go through
  tools that write the files, or (b) stop P643 after the spike. Turn-1 context is equal because
  Claude Code defers MCP tool schemas behind ToolSearch, so fewer tools did not shrink context.

## 15. Tests per Part (IPC split)

Each feature Part ships one Go flow test and one UI spec on a shared contract.

- Part 2: no user feature. Unit tests only where rules interact: classify matrix (writer, kind,
  owner, collision) and submit diff (links entries, proof step ids). Store adapter conformance:
  traversal, symlink, hard link, FIFO, caps, atomic replace. Bench: stat of a 500-file folder under
  5 ms. Migration framework flow test with a fake exporter (crash, resume, verify fail).
- Part 3: flow `adeflow` `TestDetailsFiles` (contract `ade-details-files.json`): user note writes
  a file; agent draft plus submit accepted and on the board; agent edits a user note: pending,
  approve updates cache, deny restores file; collision prompt; hand edit seen on open, Accept and
  Discard; invalid file with line; traversal, symlink and cap refused; `finish_step` implicit submit
  and refusal without draft; read-only file restored; migration of handovers, notes and links with
  verify. UI `ade-v2-details-files.spec.ts` on the same contract: banner, diff dialog, approvals
  item, problem strip, folder bar. Update listed adeflow tests and UI specs; real `claude`
  `TestAdeReportArtifacts` becomes `TestAdeDetailsSubmit`; `TestAdeManualSession` writes a draft.
- Part 4: flow `proofflow` `TestProofFiles` (contract `proof-files.json`): new proof raises ready
  prompt; step error names file, line and step id; relative image only; edit pending, approve
  keeps view state by step id, deny restores files and image; user delete removes folder and rows;
  migration of linked sessions and images (`.jpg`), unattached sessions per Q16. Removal
  assertions: kira-studio lists no proof tool for any client, Window menu has no "Validation
  proof", `OpenWindow` without (task id, slug) refused, no unattached or attach call left.
  Image unit table (`proof/image_test.go`, decision structure too large to hold in the head):
  one valid file per format (PNG, JPEG, WebP lossy and lossless, SVG); extension mismatch; GIF and
  `.svgz` refused; APNG and animated WebP refused; size, side and area caps per format; damaged
  data. Hostile SVG fixtures in `internal/proof/testdata/svg/`, one per rule: `<script>`,
  `<foreignObject>`, `iframe`/`object`/`embed`, SMIL `<animate>` and `<set>`, `onload` attribute,
  `javascript:` href, external `href` (https), external `xlink:href` on `<use>`, nested SVG data
  URL, `@import` in `<style>`, external `url()` in a `style` attribute, DOCTYPE with entity
  (billion laughs), not well-formed, no size (no viewBox, no width and height), side over 16384,
  over 2 MiB, depth over 64; each asserts the error names element or attribute and line. Plus one
  allowed SVG using `#fragment` refs, a gradient `url(#g)` and an embedded `data:image/png`. UI
  proof window spec (one proof, no history, path header, image per format incl. SVG through
  `<img>` data URL, no inline `<svg>` in the DOM) and `ade-v2-proof-chain.spec.ts`. Real `claude`
  `TestProofMcpThroughClaude`, `TestProofChartThroughClaude` rewritten for files.
- Part 5: flow `adeflow` `TestDetailsStateFile` (contract `ade-state-file.json`): lease, op, setup
  and stage changes land in `state.yaml` within 250 ms, coalesced (count writes), byte-equal skip;
  merged tools (`branches` all actions incl. wait, `lease`, `cancel_operation`); removed tools
  absent from every grant (`TestOrchestratorToolGrants`, agentmcp `TestGrantScopes`); orchestrator
  reads children through `chain`. UI spec: folder bar lists `state.yaml` read-only with its `rev`,
  which rises after a lease change; leases and branch request specs still pass. Real `claude`
  `TestAdeOrchestrator`, `TestAdeHeadlessRun`, branch request rows rerun.
- Part 5, D-Q2 variant F only: flow `adeflow` `TestOrchestratorChildDrafts` (contract
  `ade-child-drafts.json`, shared with the existing task create approval specs: same
  `task.create` payload): draft plus submit raises the approval and submit blocks; approve creates
  the child and its folder, reply carries `taskId`, draft folder gone, `chain.children[].draft`
  set; deny marks `denied` in `state.yaml`, unchanged resubmit refused, changed resubmit prompts
  again; timeout; caller gone withdraws; resubmit after create returns the same id and creates no
  second task (crash between create and folder removal included); invalid title, workflow, repo,
  base and brief size each name file and line; draft in a non-orchestrator task refused; mixed
  submit (draft plus link) accepts the link before the wait; `finish_step` skips drafts.
  `TestOrchestratorSpawn` and `TestOrchestratorSecondPassSeesFirstPassChild` move from
  `create_task` to drafts; `TestOrchestratorToolGrants` asserts `create_task` absent; mobileflow
  `TestPhoneApprovals` answers a draft's `task.create` item. UI: `ade-v2-orchestrator.spec.ts`
  and `ade-mobile-approval.spec.ts` on the same contract (approval view unchanged, origin a
  submit). Real `claude` `orchestrator_test.go` rewritten for drafts. Variant T: today's
  orchestrator tests stay, only `list_tasks`, `task_chain` removals apply.
- Every Part: matching real `claude` rows of DEV_ENVIRONMENT ("ADE runs and sessions", "Agent
  sandbox") run once near Part end.
- Parts 3, 4, 5: benchmark of section 14.1 against the baseline, its gate binding for the Part.
  End of Part 5: final benchmark plus Kira-side cost tests (14.1).

## 16. Parts

Numbers stay P643 (`P643 Part 1` to `Part 5`). Order fixed: Part 1, gate (baseline recorded,
D-Q2 verdict committed), Part 2, Part 3, then Parts 4 and 5 as two streams, then the final
benchmark. Parts 3, 4 and 5 each end with their benchmark (14.1) and count done only when it
passes; each adds its flows to `tests/claude/details_bench_test.go`.

| Part | Content | Main files |
|---|---|---|
| 1 Spike | real `claude` comparison, gate, baseline, D-Q2 verdict | `tests/claude/details_spike_test.go`, `tests/claude/testdata/details-spike/**` (incl. `tools-baseline.json`), `package.json` (`test:claude:details`), `docs/DEV_ENVIRONMENT.md` (real `claude` row), new `plans/P643-spike-results.md` |
| 2 Foundation | store seam, formats, validators export, submit core (walk, classify, diff, apply, restore), intents, `task_files`, migration framework, README and `AGENTS.md` generator, task folder grant (add-dir, sandbox allow and denies, deny rules) | new `internal/taskfiles/**`, `internal/taskfiles/store/**`, `storage/migrations/0101_p643_task_files.sql`, `storage/repos/taskfiles.go`, `internal/proof/validate.go` (export), `ade/sandbox.go`, `ade/sandbox_tui.go`, `ade/launches.go`, `appwire/wire.go` |
| 3 Notes, handovers, links | file writes for every UI action, caches, stage move rename, `submit` tool, `finish_step` change, `handover` and `report_artifacts` removed, prompts and approvals kind `details.edit`, disk banner, migration exporters, panel forward paths | `ade/handover.go`, `ade/links.go`, `ade/artifacts.go`, `ade/orchestrator.go`, `ade/taskcreate.go`, `ade/panelforward.go`, new `ade/details.go`, `agentmcp/{handover,artifacts,mcp,space,tools}.go`, new `agentmcp/submit.go`, `llmprompts/registry.go`, `bridge/adetask.go`, `adewire/wire.go`, `frontend/src/ade/v2/details/**`, `request/kindDefs.ts`, tests |
| 4 Proofs | proof format, submit kind proof, image formats (Q17) incl. SVG checks, proof window single and task-only, retention removed, `proof_save`/`proof_list` removed for every client, standalone entry points removed (Q10 list, section 10), proof migration | `internal/proof/**`, `storage/repos/proof.go`, `dbmcp/proof_tools.go`, `dbmcp/server.go`, `ade/proof.go`, `bridge/proof.go`, `bridge/dbmcp.go`, `bridge/adetask.go`, `appshell/menu.go`, `appwire/menuactions.go`, `frontend/src/studio/proof/**` (`ProofImage.vue` changed, `ProofHistory.vue` removed), `details/AdeProofEntry.vue`, `AdeProofAttachMenu.vue` (removed), `packages/shared/domain/proof.ts`, `go.mod` (`golang.org/x/image`), tests |
| 5 State file and tools | `state.yaml`, `task.yaml` writer, merged `branches`, `lease`, `cancel_operation`, removals (`task_info`, `lease_status`, `list_tasks`, `task_chain`, `run_outcome`, `operations`), orchestrator chain reads; D-Q2 per spike verdict (7.5): variant F adds child drafts (submit kind `child-draft`, blocking wait, `task_child_drafts`, `create_task` removed), variant T keeps `create_task` | new `ade/statefile.go`, `agentmcp/{space,leases,orchestrator,outcomes,tools}.go`, new `agentmcp/branches.go`, `llmprompts/registry.go`, `ade/leases.go`, `ade/orchestrator.go`, `details/` folder bar, tests; variant F also `ade/taskcreate.go`, new `ade/childdrafts.go`, `agentmcp/submit.go`, `storage/repos/adetask.go` (`CreateChildTask` marker), `storage/migrations/0102_p643_child_drafts.sql`, `taskfiles` classify |

Streams after Part 3: Stream A = Part 4, Stream B = Part 5. No ordering dependency: Part 4 touches
proof storage and window, Part 5 touches state mirror, tool shapes and (variant F) child drafts.
The D-Q2 verdict is fixed at the gate, before Part 2, so Stream B's plan knows its variant. Shared
files, additive hunks:

| Shared file | Stream A edit | Stream B edit |
|---|---|---|
| `internal/agentmcp/tools.go` | none | tool id lists |
| `internal/llmprompts/registry.go` | proof file texts, `proof_save`/`proof_list` entries removed | `branches`, `lease`, `cancel_operation` texts, removed entries; variant F: `create_task` removed, child draft texts |
| `internal/agentmcp/submit.go` | proof item results | variant F only: blocking wait for child drafts |
| `internal/taskfiles/classify.go` | `proofs/<slug>/` patterns | variant F only: `children/<draft>/` pattern |
| `internal/taskfiles/readme.go` | proof section of README and `AGENTS.md` | `state.yaml`, `task.yaml` section |
| `docs/ARCHITECTURE.md` | "Validation proof" section | ADE agent tools, leases, orchestrator sections |
| `frontend/src/ade/v2/details/AdeDetailsFolderBar.vue` | none | file list with `state.yaml` |

Every Part updates ARCHITECTURE.md (new "Task files (P643)" section in Part 2, extended after) and
removes stale text (handover tool, `report_artifacts`, proof retention, `task_chain`).

## 17. Risks

- Agent writes files and never submits: `finish_step` implicit submit, session end leaves the
  banner, `state.yaml` `pending` shows nothing unexpected. No silent loss, no silent accept.
- Unsandboxed agent writes another task's folder: never auto-accepted; that task's banner shows it.
- Token cost: spike gate. Prompts keep inlining incoming handover and notes (capped), so a step
  needs no read for its common input (Q13).
- Change size: five Parts, two streams; columns kept as caches limit blast radius (32 callers of
  `model.AdeHandover` keep working).
- File then DB order: intents journal, start-up reconcile, atomic rename.
- Path traversal, symlinks, special files, size caps: checked per file at submit and in the store.
- Collision false positives: warn prompt only when another writer changed the file after this
  session's last sync.
- Seatbelt profile and `--add-dir` fixed at launch: children created later unreadable under
  enforced sandbox; previews cover it.
- Migration on large homes: per task, resumable, verify before rename, no backup (Q11 changed);
  time measured on a large fixture (14.1).
- Image base64 rewrite is one-way and has no backup (Q11 changed): rewrite only after the task's
  verify passed; a bug in the exporter loses that image. Part 4's migration test checks decoded
  bytes against the payload hash before the rewrite.
- Agent-written Markdown rendered in UI: same renderers and sanitizers as today.
- Proofs gone for DB MCP clients outside a task (Q10): external clients and Studio sessions lose
  `proof_save`; nothing replaces it. User decision, recorded so ARCHITECTURE says it.
- Hostile SVG: validator rejects script, foreignObject, handlers, entities and external
  references; rendering only through `<img>` from a data URL is a second guard (no script, no
  fetch in image context); never inlined. One fixture per rule (section 15).
- Large image data URLs over IPC: up to 10 MiB raster as base64 (about 13.4 MiB) per step read;
  the bridge serves one step on demand, as today's payload read does.
- New dependency `golang.org/x/image/webp` (BSD-3): only for WebP; dropping WebP removes it.
- D-Q2 variant F: a submit holding a child draft blocks up to 30 min, as `create_task` does. Other
  items apply before the wait; `finish_step` skips drafts so a run's finish never blocks. Duplicate
  children prevented by `task_child_drafts` and the created marker in the create transaction.
- Benchmarks cost tokens: per-Part runs are a flow subset, capped at 0.25 USD per run; real `claude`
  only, opt-in, never in CI.

## 18. Open questions for the user

Each with the plan's recommendation.

- Q1 Hand edits while the app runs: no watcher; stat on open, focus, Refresh, submit; banner with
  Accept or Discard. Alternative: watcher on the open task folder that only shows the banner sooner,
  never ingests. Recommend: no watcher.
- Q2 Answered, reversed and conditional (Decision D-Q2, section 7.5): an orchestrator drafts a
  child as `children/<draft>/` (`task.yaml`, `brief.md`) in its own folder and calls `submit`; Kira
  validates as `create_task`, raises the same `task.create` approval (desktop and phone), blocks
  up to 30 min as `create_task` does, creates the task and its folder on approval; `create_task`
  goes. Built only if spike F6 passes (cost at most 1.1 times, turns at most plus 1 per child, pass
  rate not lower); else `create_task` stays as today.
- Q3 Answered: `<home>/tasks/<taskId>/`, named by task id.
- Q4 `state.yaml` holds absolute worktree paths (host-local mirror, not content); content files
  stay relative only. Recommend: yes.
- Q5 Agent edits a user note or a handed handover: prompt with diff. Alternative: always reject.
  Recommend: prompt.
- Q6 Prompted submit items. Answered: follow the old tool per item (see Decisions). Verified
  (section 7.3): every item of today's set maps to a tool that returned at once, so it returns at
  once, answer by notice and `state.yaml`. The one blocking item is the D-Q2 variant F child
  draft (`create_task`, 30 min); a mixed submit waits for drafts only.
- Q7 `finish_step` ok with no draft while the route continues: refuse with the path. Alternative:
  accept and note it. Recommend: refuse.
- Q8 Child finish mirror rows: derive from the child's file, no copy in the parent. Recommend: yes.
- Q9 Merged tools with an `action` argument (`branches`, `lease`) and `run_outcome` folded into
  `state.yaml`. Spike F5 measures the merged schema. Recommend: yes.
- Q10 Answered, changed: no proof outside a task. Standalone folder, picker, menu item, history
  pane, unattached sessions, attach, detach and kira-studio proof tools all go (section 10 list).
- Q11 Answered, changed: no backup. Migration one-time and resumable; image base64 rewritten to
  files after each task's verify, one-way.
- Q12 Spike models: haiku 3 runs per flow and arm plus one sonnet run each, 5 USD cap. Recommend:
  yes.
- Q13 Step prompts keep inlining incoming handover and notes (capped) beside the folder pointer.
  Recommend: yes.
- Q14 Gate thresholds: cost at most 1.2 times, turns at most plus 2, pass rate not lower.
  Recommend: as stated.
- Q15 Answered, changed: `details.edit` approvals reach desktop and phone.
- Q16 New (Q10 plus Q11 leave no place and no backup for them): today's unattached proof
  sessions at migration. Recommend: delete their rows after one log line naming each (retention
  pruned them past 30 anyway). Alternative: keep dormant rows, shown nowhere, until a later
  chapter drops them.
- Q17 Answered: image formats PNG, JPEG, WebP, SVG; raster 10 MiB, 16 MP, sides 1 to 16384, no
  animation; SVG 2 MiB, rejected on unsafe content, shown only through `<img>` (section 10). WebP
  is the one to drop if any.
