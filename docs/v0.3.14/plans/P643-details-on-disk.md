# P643 Details tab on disk: plan

Design-first. User approves this plan, and answers "Open questions", before any build Part starts.
Planner: Opus. Base `1e1300e16`. Discovery through `codegraph_explore`, then targeted reads.

## 1. Decisions taken as input

From SPEC P643 Scope and the design talk. Binding.

- Content (notes, handovers, links with PR link, proofs with images) lives in files under the home
  folder; files are source of truth for content. State stays in SQLite; no file edit drives state.
- One owner per field. SQLite keeps id, path, hash, review status and rebuildable caches; never an
  independent write of content.
- One sync tool, explicit `submit`, is the consent point. No watcher for pickup.
- `handover`, `report_artifacts`, `proof_save`, `proof_list` go. `task_info`, `lease_status`,
  `list_tasks`, `task_chain` become file reads. Agent tools end near 6.
- Tasks stay Kira-written read-only files; `create_task` stays a tool with its approval.
- Proof window shows one proof; no list, no rescan, no retention of 30.
- No FUSE, no chmod, no lock. Sandbox grants each task folder; read-only files denied in sandbox.
- Spike first, with go/no-go gate.

Two SPEC sentences conflict; plan resolves them, user confirms (Q1, Q2):

- "a watcher covers hand edits only" / "watch open task folders for hand edits only" versus "No
  watcher anywhere" and the user's later rejection of a watcher. Plan: no watcher; stat on view.
- "Tasks are folders; an orchestrator creates a child by writing a folder" versus "Tasks stay
  Kira-written read-only files; `create_task` stays a tool". Plan: the latter; no task-folder Part.

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

- `task_files(scope, rel_path, kind, size, version, sha256, accepted_sha256, accepted_by,
  accepted_at, review, problem)`, PK (scope, rel_path). `scope` = task id or `proofs` (standalone).
  `version` is the store's opaque token (local: size plus mtime ns). `review` in
  `accepted|pending|invalid`. `accepted_by`, `accepted_at` and `review` are state; the rest rebuilds.
- `task_file_intents(scope, rel_path, sha256, writer, created_at)`: journal for file-then-DB writes.
- `task_files_migration(task_id, part, state, done_at, verify_json)`: resumable migration marker.

Justification per index: `task_files` is needed for change detection without reading every file
(size plus version before hash) and holds review state; intents make a retry safe; migration
markers make the one-time move resumable. No search index: board, list and archived finder keep
their SQLite fields (title, Jira key) as today.

Accepted image bytes: content-addressed, Kira-only store `<home>/tasks-blobs/<sha256>.jpg`, outside
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
    proofs/<slug>/<file>.jpg   content: image step
<home>/proofs/<slug>/...       standalone proofs (no task), same proof format
<home>/tasks-blobs/            Kira only: accepted image bytes
```

`<created>` is `YYYYMMDDTHHMMSS` from the row, not a file time; names sort by time for agents.
Hidden files, unknown names and anything deeper than `proofs/<slug>/` are "not Details": left
alone, reported once as `ignored` by submit.

## 5. Formats

| File | Format | Why |
|---|---|---|
| notes, handovers | Markdown, optional YAML frontmatter | prose; agents edit Markdown natively; Claude `Edit` string replace works; readable diffs |
| `links.yaml`, `task.yaml`, `state.yaml` | YAML | small structured; comments carry the header; fewer tokens than JSON; configfs gives line numbers |
| `proof.md` | YAML frontmatter (structure) plus Markdown body (explanation) | one file per proof as the user asked; frontmatter validates with line numbers |
| proof data | JSON (`.json`), Mermaid (`.mmd`), JPEG (`.jpg`) | existing validators take raw JSON (precision and key order kept, P420); images as files, not base64 |

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
    data: totals.json # file in this folder: .json, .mmd for diagram, .jpg for image
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

Collision rules for multi-writer files:

| File | Rule | Residual, why acceptable |
|---|---|---|
| notes | one file per note; agent change = prompt; user writes win silently | two windows editing one note: last UI save wins, as today |
| draft of current stage | hash check: user or another session changed it after this session's last sync and this session's version differs = warn-and-confirm prompt | today's `UpsertHandoverDraft` overwrites silently; this is strictly better |
| handed handovers | one file each; agent change = prompt | none |
| `links.yaml` | entry diff: new and own entries silent; others' entries = prompt; file changed by someone else after this session's sync = warn-and-confirm | small file, rare overlap; per-entry files rejected to keep one read for agents |
| proofs | single owner (creating session); another agent's change = prompt | user deletes a proof an agent still edits: next submit recreates it as new, prompt shows "new proof" |
| `task.yaml`, `state.yaml`, README | single owner Kira; any other write restored | an unsandboxed agent can scribble; Kira restores at the next submit or rewrite |

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
results, files changed but not accepted, the quick artifact list as today.

kira-studio `submit` `{proof: "<slug>"}` for standalone proofs under `<home>/proofs/`. Omitted
from the kira-studio server under a Space grant, so a task agent sees one `submit`.

### 7.2 Algorithm (under `taskMu`)

1. Walk the folder (depth cap 3, `Lstat`; symlink, hard link count above 1, FIFO, device = refused
   per file; at most 500 files and 64 MiB per task, 32 MiB per proof).
2. Classify path by pattern into kind; no match = `ignored`.
3. `Store.Stat`: size and version equal to `task_files` = unchanged, no read.
4. Read up to the kind cap, sha256. Equal to `accepted_sha256` = refresh version only.
5. Parse and validate: configfs YAML pipeline (line, column), kind schema, kind rules (handover
   16 KiB, note 1 MiB, link rules, proof validators exported from `internal/proof` as
   `ValidateStep(view, raw)`, image JPEG checks on the relative file). Invalid = per-file
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
| New proof | accept, existing "proof ready" prompt (notify) | user |
| Edit of an accepted proof | pending review, existing `proof.review` approval with step diff | user |
| `task.yaml`, `state.yaml`, README written | restored | agent (`restored`) |
| Collision (7.2 step 9) | warn-and-confirm prompt | user |
| Invalid file | `invalid` with line | agent; UI problem strip |

Prompt items are non-blocking: submit returns `pending` at once. The answer reaches the agent
through the lease notice path (`SendIfIdle`, hook reply, next tool result `notices`) and the
`pending` list in `state.yaml`. Deny restores the accepted copy on disk. A new submit of the same
file replaces its open item. Approvals kind `details.edit` (new; desktop only like Studio kinds,
not on the phone allowlist unless the user asks).

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
  children: [{id, title, status, step, finished, archived, lastHandover: ../<id>/handovers/<file>, preview}]
pending: [{path, kind, since}]  # submit items waiting on the user
```

Worktree paths are absolute: the mirror is host-local state, regenerated per host, never content
(Q4). Writes: one writer goroutine, per-task dirty flag, coalesce 150 ms (inside 100 to 250),
skip when bytes equal, `renameio`. Triggers: board change (`notifyBoard` `taskChanged`), lease
book change, op start and end, setup change. Live tasks only; one final write at archive.

`task.yaml`: id, title, kind, workflow, parent, repos, created. Same header and writer.

Orchestrator children under an enforced sandbox: a child created after launch is outside the fixed
Seatbelt profile and `--add-dir` list. `chain.children[].preview` (first 500 bytes, as
`list_tasks` today) and the `create_task` reply cover it until the next session.

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
| `create_task` | kira-space | orchestrator | kept; brief written as the child's first handed file |
| `list_tasks` | kira-space | orchestrator | removed: `state.yaml` `chain` |
| `task_chain` | kira-space | orchestrator | removed: read own and children's `handovers/` |
| (new) `submit` | kira-space | | every Space grant |
| `git`, `reply_review_comment`, `fetch_remote`, `push_branch` | kira-space | conditional | unchanged |
| `proof_save`, `proof_list` | kira-studio | DB MCP sessions | removed: proof files plus `submit` |
| (new) `submit` | kira-studio | | standalone sessions only |

Counts (conditional tools excluded): run 12 to 5 (`submit`, `finish_step`, `branches`, `lease`,
`cancel_operation`); session 12 to 4; orchestrator run 15 to 6. kira-studio proof tools 2 to 0 in a
task, 1 standalone. Lease ranking, heartbeat and notices keep working: `lease` calls are the
heartbeat (`mcpRecorder`).

## 10. Proofs

- Folder per proof, slug from name, unique per scope (case folded, as `name_key`).
- New proof: files plus `submit`; Kira creates session, revision and steps rows (caches), links it
  to the task (state), raises the ready prompt. Edit: pending revision from disk version, current
  revision stays the accepted copy; approve copies view state of unchanged steps by step id;
  deny restores files from caches and blobs. Step diff and `origin_ordinal` derive from step ids.
- Images: relative file in the proof folder only. The absolute-path source of `proof_save` goes
  (the app no longer reads arbitrary paths). An accepted image changed in place counts as a change;
  its accepted bytes sit in `tasks-blobs` for restore.
- User attach of a standalone proof to a task moves its folder into `proofs/` of the task (Kira
  write); detach moves it back to `<home>/proofs/`. Delete removes folder and rows.
- Retention of 30 (`pruneClosed`) goes: files on disk stay until the user deletes them.
- View state stays in `proof_view_state` (UI state, keyed by session, rev, ordinal).
- Window: one proof per window, opened by (scope, slug). `ProofHistory.vue` pane goes. Tabs per
  step, explanation, data views, compare side for a pending edit, screenshots unchanged. Header
  shows the relative path with Open and Reveal. Window menu "Validation proof" opens a picker over
  the standalone folder listing (Q10).

## 11. Migration (one-time, resumable)

Framework in Part 2, content exporters in Parts 3 and 4.

1. Backup: `VACUUM INTO <home>/backups/kira-pre-p643-<ts>.db` once (skip when the file exists).
2. Per task, live and archived, marker in `task_files_migration`: export to
   `<home>/tasks/<id>.migrating/`, verify (re-read, parse, per-row body hash equal, counts equal),
   rename to `<home>/tasks/<id>/` (merging into an existing folder only for files absent there),
   insert `task_files` and `rel_path`, mark done. A crash leaves `.migrating`: deleted, task redone.
3. Notes to `notes/`, drafts to `handovers/draft-<step>.md`, handed rows to numbered files in
   `created_at` order. Child mirror rows: dropped when the child's own handed-into-done file
   exists (parent chain derives it from the child); else written as a read-only legacy handed file.
4. Stored artifacts plus Jira and host slots to `links.yaml`.
5. Proofs: linked sessions to the task's `proofs/<slug>/`, unattached to `<home>/proofs/<slug>/`;
   image payloads decoded to `.jpg`, payload column rewritten to metadata. A pending revision
   becomes the disk version with review pending; its approval re-raises (`RaiseUnreviewed`).
6. Columns stay as caches; nothing dropped (core idea, section 3). Image base64 payloads are
   rewritten, so the backup is the only way back for them (kept 30 days, then the user deletes).
7. Migration runs at start before the board starts, with a progress line in the log; a failure
   leaves the task on SQLite reads (flag per task) and an error banner; no partial task.

## 12. UI changes

- Details tab: same layout, same SQLite queries. New folder bar (relative path, Open, Reveal, Copy
  path) through a bound open/reveal service scoped to `tasks/<id>` (configsvc pattern, no renderer
  path reaches the OS). New `AdeDetailsDiskBanner.vue` (changed on disk, not submitted: per-file
  diff dialog, Accept, Discard; shadcn-vue Dialog, Alert). File problem strip (invalid file, line,
  Open) reusing `ConfigFileProblem` styling.
- Pending edits: approvals dialog body for `details.edit`: one row per file change, diff, per-item
  approve like `branch.request` items. Proof edits keep `proof.review`.
- Proof window: no history pane, opened per proof, path header, picker for standalone proofs.
- Panel forward bar: item labels carry the relative path.
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
  are today's exact ones; kira-studio `proof_save` stub with today's schema.
- Arm F (files): stub server with `submit`, `finish_step` (no summary), `branches`, `lease`,
  `cancel_operation`, plus a fixture task folder (README, `task.yaml`, `state.yaml`, notes,
  handovers, `links.yaml`). Stub `submit` runs real parse and validation code where it exists
  (YAML, proof validators), else shape checks.
- Flows: F1 handover (do a one-line change, hand over, finish); F2 new proof (table plus image);
  F3 edit that proof (change one row, reason); F4 orchestrator summary of two children's last
  handovers; F5 wait for a branch setup to be ready.
- Runs: haiku, 3 per flow and arm (30 runs), plus 1 sonnet run per flow and arm (10). Budget cap
  0.25 USD per run, 5 USD total.
- Measure from the stream-json result: `num_turns`, input, cache read, cache write and output
  tokens, `total_cost_usd`, tool calls by name, wall time, oracle pass (draft text present, proof
  valid, summary names both children).
- Results: table in this plan's "Spike result" section (appended by Part 1), raw numbers in
  `testdata/details-spike/results.json` only if a later test reads them, else in the section.
- Gate, per flow, haiku medians: cost of F at most 1.2 times T, turns of F at most T plus 2, oracle
  pass rate of F at least T's. All flows pass = go. Any fail = no-go report with the numbers and a
  fallback for the user to pick: (a) files stay source of truth but writes go through tools that
  write the files (keep `handover`, `proof_save` as file writers), or (b) stop P643 after the spike.

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
  prompt; step error names file, line and step id; relative image only, JPEG caps; edit pending,
  approve keeps view state by step id, deny restores files and image; standalone `submit` on
  kira-studio; attach moves folder; migration of sessions and images. UI proof window spec (one
  proof, no history, path header, picker) and `ade-v2-proof-chain.spec.ts`. Real `claude`
  `TestProofMcpThroughClaude`, `TestProofChartThroughClaude` rewritten for files.
- Part 5: flow `adeflow` `TestDetailsStateFile` (contract `ade-state-file.json`): lease, op, setup
  and stage changes land in `state.yaml` within 250 ms, coalesced (count writes), byte-equal skip;
  merged tools (`branches` all actions incl. wait, `lease`, `cancel_operation`); removed tools
  absent from every grant (`TestOrchestratorToolGrants`, agentmcp `TestGrantScopes`); orchestrator
  reads children through `chain`. UI spec: folder bar lists `state.yaml` read-only with its `rev`,
  which rises after a lease change; leases and branch request specs still pass. Real `claude`
  `TestAdeOrchestrator`, `TestAdeHeadlessRun`, branch request rows rerun.
- Every Part: matching real `claude` rows of DEV_ENVIRONMENT ("ADE runs and sessions", "Agent
  sandbox") run once near Part end.

## 16. Parts

Numbers stay P643 (`P643 Part 1` to `Part 5`). Order fixed: Part 1, gate, Part 2, Part 3, then
Parts 4 and 5 as two streams.

| Part | Content | Main files |
|---|---|---|
| 1 Spike | real `claude` comparison, gate | `tests/claude/details_spike_test.go`, `tests/claude/testdata/details-spike/**`, this plan (result section) |
| 2 Foundation | store seam, formats, validators export, submit core (walk, classify, diff, apply, restore), intents, `task_files`, migration framework, README and `AGENTS.md` generator, task folder grant (add-dir, sandbox allow and denies, deny rules) | new `internal/taskfiles/**`, `internal/taskfiles/store/**`, `storage/migrations/0101_p643_task_files.sql`, `storage/repos/taskfiles.go`, `internal/proof/validate.go` (export), `ade/sandbox.go`, `ade/sandbox_tui.go`, `ade/launches.go`, `appwire/wire.go` |
| 3 Notes, handovers, links | file writes for every UI action, caches, stage move rename, `submit` tool, `finish_step` change, `handover` and `report_artifacts` removed, prompts and approvals kind `details.edit`, disk banner, migration exporters, panel forward paths | `ade/handover.go`, `ade/links.go`, `ade/artifacts.go`, `ade/orchestrator.go`, `ade/taskcreate.go`, `ade/panelforward.go`, new `ade/details.go`, `agentmcp/{handover,artifacts,mcp,space,tools}.go`, new `agentmcp/submit.go`, `llmprompts/registry.go`, `bridge/adetask.go`, `adewire/wire.go`, `frontend/src/ade/v2/details/**`, `request/kindDefs.ts`, tests |
| 4 Proofs | proof format, submit kind proof, kira-studio `submit`, proof window single, retention removed, `proof_save`/`proof_list` removed, proof migration | `internal/proof/**`, `storage/repos/proof.go`, `dbmcp/proof_tools.go`, `dbmcp/server.go`, `ade/proof.go`, `bridge/proof.go`, `frontend/src/studio/proof/**`, `details/AdeProofEntry.vue`, `AdeProofAttachMenu.vue`, tests |
| 5 State file and tools | `state.yaml`, `task.yaml` writer, merged `branches`, `lease`, `cancel_operation`, removals (`task_info`, `lease_status`, `list_tasks`, `task_chain`, `run_outcome`, `operations`), orchestrator chain reads | new `ade/statefile.go`, `agentmcp/{space,leases,orchestrator,outcomes,tools}.go`, new `agentmcp/branches.go`, `llmprompts/registry.go`, `ade/leases.go`, `ade/orchestrator.go`, `details/` folder bar, tests |

Streams after Part 3: Stream A = Part 4, Stream B = Part 5. No ordering dependency: Part 4 touches
proof storage and window, Part 5 touches state mirror and tool shapes. Shared files, additive hunks:

| Shared file | Stream A edit | Stream B edit |
|---|---|---|
| `internal/agentmcp/tools.go` | none | tool id lists |
| `internal/llmprompts/registry.go` | proof file texts, kira-studio `submit` | `branches`, `lease`, `cancel_operation` texts, removed entries |
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
- Migration on large homes: per task, resumable, backup first, verify before rename.
- Image base64 rewrite is one-way: backup kept.
- Two `submit` tools in a task: kira-studio omits its own under a Space grant.
- Agent-written Markdown rendered in UI: same renderers and sanitizers as today.

## 18. Open questions for the user

Each with the plan's recommendation.

- Q1 Hand edits while the app runs: no watcher; stat on open, focus, Refresh, submit; banner with
  Accept or Discard. Alternative: watcher on the open task folder that only shows the banner sooner,
  never ingests. Recommend: no watcher.
- Q2 Task folders written by an orchestrator: no. Tasks stay Kira-written, `create_task` keeps its
  approval; a writable task definition costs the agent two calls (write plus submit) for the same
  approval and adds a validation surface. Recommend: drop the conditional "tasks as folders" Part.
- Q3 Folder root and names: `<home>/tasks/<taskId>/`. Alternative: slug plus id (readable, but a
  title change renames a folder agents hold open). Recommend: id.
- Q4 `state.yaml` holds absolute worktree paths (host-local mirror, not content); content files
  stay relative only. Recommend: yes.
- Q5 Agent edits a user note or a handed handover: prompt with diff. Alternative: always reject.
  Recommend: prompt.
- Q6 Prompted submit items: non-blocking, answer by notice and `state.yaml`. Alternative: block
  like `request_branches` (up to 30 min). Recommend: non-blocking.
- Q7 `finish_step` ok with no draft while the route continues: refuse with the path. Alternative:
  accept and note it. Recommend: refuse.
- Q8 Child finish mirror rows: derive from the child's file, no copy in the parent. Recommend: yes.
- Q9 Merged tools with an `action` argument (`branches`, `lease`) and `run_outcome` folded into
  `state.yaml`. Spike F5 measures the merged schema. Recommend: yes.
- Q10 Proof window entry outside a task: picker over the standalone folder listing (no index).
  Alternatives: open newest only, or remove the menu item. Recommend: picker.
- Q11 Image base64 payloads rewritten after migration, backup kept 30 days. Alternative: keep the
  old payloads one chapter. Recommend: rewrite.
- Q12 Spike models: haiku 3 runs per flow and arm plus one sonnet run each, 5 USD cap. Recommend:
  yes.
- Q13 Step prompts keep inlining incoming handover and notes (capped) beside the folder pointer.
  Recommend: yes.
- Q14 Gate thresholds: cost at most 1.2 times, turns at most plus 2, pass rate not lower.
  Recommend: as stated.
- Q15 `details.edit` approvals on the phone: desktop only for now. Recommend: desktop only.
