# P643 Details tab on disk: plan

Build plan for the narrowed design the user chose after the spike (fallback (a), narrowed). Planner:
Opus. Base `abe943dbe`. Discovery through `codegraph_explore`, then targeted reads. Spike data:
`P643-spike-results.md` (Part 1, 1b, 1c).

## Decisions

Final user answers. Binding for every build Part.

- Design (after spike, user "Looks good"): files are the source of truth for content and the
  readable state. `create_task` keeps its tool entry and stays exactly as today (name, args, 30 min
  blocking wait, `task.create` approval, phone kind; user, final). Child draft folders (former D-Q2
  variant F) are dropped: no `children/`, no `task_child_drafts`, no draft item.
- Proofs without a tool (user, final, accepted although spike F2 and F3 favour a tool): the agent
  writes `proofs/<slug>/proof.md` (single file, step data inline; images and large data beside it)
  itself and calls `submit`; editing works the same way. No `proof` tool, no `create` tool. Expected
  cost gap on F2 and F3: 1.2 to 1.3 times T, reported, not gated (14.1). Flow: section 7.5.
- Create-first rejected (analysis `P643-create-submit-analysis.md`): no `create {kind}` tool that
  writes a skeleton for the agent to fill; it costs 3 to 4 calls per new resource against 1 or 2 and
  reopens child drafts, draft cleanup and a blocking `submit`.
- Leases stay in `state.yaml` (user): the per-task file lists the task's leases; a lease change
  rewrites only the affected tasks' `state.yaml`. Lease state stays in Kira's lease book, changed
  only through `lease`; never driven by a file edit.
- Waits (settled for Part 5, section 9.2): `task_info` goes; no wait tool. A `branches` request
  returns only once each requested worktree's setup is ready or failed, within the call deadline;
  the same request again is the ready check. Interactive sessions also get a lease-style push when
  a setup they asked about ends. `state.yaml` `repos[].setup` carries state and failure note. Until
  Part 5 lands, `task_info` stays unchanged. F5 re-benchmarked (14.1).
- Content is files (notes, handovers, links incl. Jira and PR slots, proofs with images as files).
  State stays in SQLite (approvals, stages, branches, leases, runs, status, workflow, plan
  placement) and is never driven by a file edit. `state.yaml` and `task.yaml` are read-only mirrors
  Kira rewrites atomically; state changes only over MCP.
- Q1 yes: no watcher. Explicit `submit` is the consent point. Kira stats the folder on
  task open, Details focus, Refresh, submit, `finish_step`. Banner with Accept or Discard.
- No chmod, no FUSE, no lock. No index unless it is state or needed (section 3).
- Proof window shows one proof, one file.
- Relative paths only in the file layer (future cloud).
- Q3 yes: folders by task id, `<home>/tasks/<taskId>/`.
- Q9 yes, adjusted by spike: merged `branches {action: declare|request|rename|change}` and
  `lease {action: acquire|release}`; `run_outcome`, `lease_status`, `operations` list fold into
  `state.yaml`; `cancel_operation` tool. Branch setup wait: blocking `branches` request (Waits bullet above).
- Q10: no proof outside a task. Removed: standalone proofs folder, kira-studio `proof_save` and
  `proof_list` for every client, Window menu "Validation proof", standalone picker, history pane,
  unattached sessions, attach and detach. List in section 10.
- Q11: no backup after migration. No `VACUUM INTO`, no 30-day retention. Migration one-time,
  resumable; image base64 payloads rewritten to files after each task's verify, one-way.
- Q15: `details.edit` approvals reach desktop and phone.
- Q16 final (user accepted the recommendation): taskless proof sessions are deleted at migration
  after one log line naming each.
- Q17: proof images PNG, JPEG, WebP, SVG (safe SVG rules, section 10). WebP is the one to drop.
- Silent accept: a change that needs no approval and has no notification today is accepted with no
  notification. A new proof keeps accept plus the existing "proof ready" notification.
- Q6: blocking mirrors the old tools (table 7.3). No `submit` item blocks: none maps to a blocking
  old tool now that child drafts are gone. `create_task` keeps its own 30 min wait.
- Benchmarks: per-Part and final (14.1). Sonnet, 3 reps per flow and arm, 0.5 USD per-run cap,
  real implementation, same harness. Gate: cost at most 1.2x, final context at most 1.0x, pass
  rate not lower, failed edit-path calls plus retries not higher; turns reported only. F2 and F3
  (proofs as files) are reported, not gated: the user chose the design regardless.
- Q4 to Q8, Q13, Q14: plan recommendation (section 18).

## 1. Inputs and SPEC conflicts

From SPEC P643 Scope, the design talk and the spike. SPEC conflicts and how the plan resolves them:

- "a watcher covers hand edits only" versus "No watcher anywhere": no watcher (Q1).
- "Tasks are folders; an orchestrator creates a child by writing a folder" versus "Tasks stay
  Kira-written read-only files; `create_task` stays a tool": the latter. Spike F6 made child drafts
  24 to 40 percent dearer; the user dropped them.
- "agent writes the proof file and calls `submit`" versus spike F2 and F3 (+18 to +40 percent cost
  as files): SPEC wins (user, final). No `proof` tool; the files arm gap is reported (14.1).
- "for standalone sessions `submit` is on kira-studio": dropped by Q10 (no proof outside a task).
- "one-time migration with backup": no backup (Q11).
- Tool list: SPEC "about 6"; plan 5 for a run, 6 for an orchestrator run (section 9.3; Part 5
  removes `task_info`, 9.2).

## 2. Current state map

Content today, all in `kira.db`:

| Content | Table / column | Writers today | Code |
|---|---|---|---|
| User notes | `ade_task_handovers` state `note`, source `user` | user (`AddNote`, `UpdateHandover`), task create, capture | `ade/handover.go`, `storage/repos/adehandover.go` |
| Handover draft | state `draft`, one per (task, stage) | `handover` tool (`RecordHandover`), user edit, run summary fold | `ade/handover.go`, `internal/agentmcp/handover.go` |
| Handed handover | state `handed` | stage move (`StepDone`, `SetTaskStep`), run handover, orchestrator brief (`createChild`) | `ade/handover.go`, `ade/orchestrator.go`, `ade/taskcreate.go` |
| Child mirror | state `note`, source `child`, `from_task_id` | repo `mirrorFinish` on child finish | migration `0093_p571_orchestrator.sql` |
| Stored links | `ade_task_artifacts` | `report_artifacts`, Details paste (`AddLink`, `EditLink`, `RemoveArtifact`) | `ade/artifacts.go`, `ade/links.go`, `repos/adeartifact.go` |
| Jira and PR link | `ade_tasks.jira_key`, `jira_url`, `host_url` | Details paste, task edit | `ade/links.go`, `ade/board.go` |
| Proofs | `proof_sessions`, `proof_revisions`, `proof_steps` (payload BLOB, image base64), `proof_view_state`, `proof_task_links` | `proof_save` (kira-studio), user link and unlink | `internal/proof/*`, `storage/repos/proof.go`, `dbmcp/proof_tools.go`, `ade/proof.go` |

Detected links (branches, change requests, deployments, tracker issues) derive on read; never
stored, never in files.

Agent surface (`internal/agentmcp`, server `kira-space`, ids in `tools.go`):

- Run grant: `finish_step`, `run_outcome`, `SpaceToolIDs` (10): `task_info`, `declare_repos`,
  `request_branches`, `rename_branch`, `change_branch`, `report_artifacts`, `operations`,
  `acquire_lease`, `release_lease`, `lease_status`. 12.
- Session without run (`handoverGranted`): `handover` replaces `finish_step`. 12.
- Orchestrator adds `OrchestratorToolIDs`: `create_task`, `list_tasks`, `task_chain`. 15.
- Conditional: `git`, `reply_review_comment`, `fetch_remote`, `push_branch`.
- kira-studio (`dbmcp`, step option `DbMcp`): `proof_save`, `proof_list` (`addProofTools`); a new
  proof in a task attaches through `dbmcp.Config.ProofAttach` (`ade.TaskBoard.AttachProof`).
- Prompts: `llmprompts.AdeStepPrompt` (ends with `headless.space.suffix` and the finish suffix),
  `AdeManualMessage`, `AdeReviewMessage`, `AdeStartMessage`; built in `ade/vars.go`
  (`stepVars`, `composeManualMessage`, `capNotes`), handover from `handoverFor`. Tool texts in
  `internal/llmprompts/registry.go`; `HeadlessSpaceInstructions` says "Call task_info first".
- `task_info` `waitSeconds` (0 to 600) waits on `BranchStatus` with progress.
- `create_task` reply `next` text names `task_chain` and `list_tasks` (`ade/taskcreate.go`
  `createChild`).

Folders and access:

- Home `kirapaths.SpaceHome()`. Work cwd for a task with no branch: `<home>/ade/tasks/<id>`
  (`ade/taskdir.go`); stays (Claude resume keys on cwd).
- Sandbox: Seatbelt on macOS, off by default; literal write denies inside an allowed subpath.
  `--add-dir` through `harness.HeadlessSpec.AddDirs`; deny rules through `DisallowedTools`.
- `configfs.Folder.WriteDocs` writes `AGENTS.md` for config folders (scripts, agent settings,
  connections, repo config, workflows, step library). Unchanged by P643.

Reusable: `internal/configfs` (YAML node parse with lines, schema validate, atomic write),
`renameio.WriteFile`, `internal/pathsafe.ValidateRelPath`, `internal/proof` validators,
`ade/panelforward.go` (`textDiff`, go-udiff), lease notice delivery (`SendIfIdle`, hook reply,
next tool result `notices`), approvals queue (`branch.request` item lists, `proof.review`),
`mobileweb/approvals.go` kind allowlist.

Tests touching this area: flows `adeflow` (`handover`, `notes`, `artifacts`, `detailslinks`,
`detailslist`, `detailsopen`, `orchestrator`, `panelforward`, `proofchain`, `stagemove`,
`taskinfowait`, `taskpanel`, `agent_tools`, `leases`, `archived`, `dbmcpstep`, `twowindows`,
`toolnames`), `proofflow/*`, `captureflow`, `githostflow/githublinks`, `journeyflow/firstrun`,
`termflow/smart`, `mobileflow`; UI `ade-v2-{artifacts,details-links,details-list,details-open,handover,links-strip,notes,orchestrator,proof-chain,task-panel}.spec.ts`,
proof window specs, `ade-mobile-approval.spec.ts`; real `claude` `ade_test.go`,
`orchestrator_test.go`, `proof_test.go`, `automation_test.go`, `hooks_test.go`.

## 3. Core idea: keep the tables, move the content

Tables already split state from content. Every table stays as state and index; content columns
become caches written only by sync:

| Table | State columns (SQLite owns) | Cache columns (file owns, sync writes) |
|---|---|---|
| `ade_task_handovers` | id, task, state, source, from/to step, finished, dismissed, edited, session, from_task, times | `body`; new `rel_path` |
| `ade_task_artifacts` | id, task, source, session, step, run, created | kind, title, url, ref |
| `ade_tasks` | everything else | `jira_key`, `jira_url`, `host_url` |
| `proof_sessions`, `proof_revisions`, `proof_task_links`, `proof_view_state` | review state, current and pending rev, seen, link, view state | `name`, `summary` |
| `proof_steps` | rev, ordinal, diff, origin; new `step_key` (step id) | description, query, language, source, view, payload (image: metadata plus file and hash, no bytes) |

Board, mobile, prompts and Details keep reading SQLite. The cache is the last accepted copy: diff
base for submit, restore source for deny. Rebuild = read files, treat as accepted.

New tables (one migration, Part 2):

- `task_files(task_id, rel_path, kind, size, version, sha256, accepted_sha256, accepted_by,
  accepted_at, review, problem)`, PK (task_id, rel_path). `version` is the store token (local: size
  plus mtime ns). `review` in `accepted|pending|invalid`. `accepted_by`, `accepted_at`, `review`
  are state; the rest rebuilds. Needed for change detection without reading every file and for
  review state.
- `task_file_intents(task_id, rel_path, sha256, writer, created_at)`: file-then-DB journal; makes a
  retry safe.
- `task_files_migration(task_id, part, state, done_at, verify_json)`: resumable migration marker.

No search index: board, list and archived finder keep their SQLite fields.

Accepted image bytes: Kira-only content-addressed store `<home>/tasks-blobs/<sha256>.<ext>`,
outside every agent grant. Needed only to restore an image a denied edit changed. Collected after
accept and at archive.

## 4. Layout

Root `<home>/tasks/` (beside `ade/`; ARCHITECTURE "stays at home root"). One folder per task id.
No `AGENTS.md` at the root or in a task folder (section 7.6).

```
<home>/tasks/
  <taskId>/
    README.md                  Kira, read-only: <=35 lines, paths, proof example, index of proofs and children
    task.yaml                  Kira, read-only: definition mirror (Part 5)
    state.yaml                 Kira, read-only: state mirror (Part 5)
    links.yaml                 content: links, Jira slot, PR slot
    notes/<created>-<id>.md    content: one file per user note
    handovers/draft-<step>.md  content: open draft of a stage
    handovers/<NNNN>-<from>-to-<to>.md   content: one file per handed handover
    proofs/<slug>/proof.md     content: proof, step data inline
    proofs/<slug>/<id>.json    content: step data over the inline cap (64 KiB), agent-written
    proofs/<slug>/<file>.png   content: image step (.png, .jpg, .jpeg, .webp, .svg)
<home>/tasks-blobs/            Kira only: accepted image bytes
```

Per-file layout stays (spike 1b: one consolidated `handovers.md` made F4 44 percent dearer).
`<created>` is `YYYYMMDDTHHMMSS` from the row. Hidden files, unknown names and anything deeper
than `proofs/<slug>/` are "not Details": left alone, reported once as `ignored` by submit.

## 5. Formats

| File | Format | Why |
|---|---|---|
| notes, handovers | Markdown, body only | prose; `Edit` string replace works; readable diffs |
| `links.yaml`, `task.yaml`, `state.yaml` | YAML | small structured; comment header; configfs line numbers |
| `proof.md` | YAML frontmatter (structure plus inline step data) and Markdown body (explanation) | one proof one file; one Read gives the whole proof (spike 1c FP: inline edits applied 3 of 3) |
| proof side files | JSON (`.json`) over the inline cap; PNG, JPEG, WebP, SVG images | images never base64 in text; validators take raw JSON (precision, key order, P420) |

Content files hold content only. Metadata (stage names, author, session, times) lives in SQLite
and shows in README and UI. Kira never rewrites an agent-written file to normalize it.

`reason` is never a file field. It is a request property, like `finished`: an argument of `submit`
(section 7.1). Spike 1c: two of three FP runs missed a `reason:` frontmatter key that
no read doc mentioned.

Header comment, first line of every Kira-created content file (YAML `#`, bare Markdown
`<!-- -->`): `Kira <kind>. Edit, then call submit (reason when the change needs approval).`
Notes and handovers carry no header (body only; prompts and README say it). Optional on
agent-created files; validation never needs it.

`links.yaml`:

```yaml
# Kira links. Edit, then call submit (reason when you change another author's link).
jira: {key: ABC-12, url: https://acme.atlassian.net/browse/ABC-12}
pr: https://github.com/acme/app/pull/7
links:
  - {kind: doc, title: Design, url: https://...}
  - {kind: link, title: Fixture, ref: testdata/orders.csv}
```

Rules are `agentmcp.CleanArtifact`'s (kinds, caps, absolute http(s) URL, `ref` relative). Entry key
= normalized URL, else (kind, ref). Author per entry stays in `ade_task_artifacts`.

`proof.md` (spike 1c FP shape; `reason` removed):

```yaml
---
# Kira proof. Edit, then call submit (pass reason to submit when the proof is accepted).
kira: proof/1
name: Orders match ledger
summary: All 20 orders of the day match their ledger lines on id and total.
steps:
  - id: totals                 # stable; keep it when you change the step
    description: Orders and ledger agree on all 20 rows
    view: table                # table | document | keyvalue | chart | diagram | image
    inline:
      columns: [{"name": "order_id"}, {"name": "order_total"}, {"name": "ledger_total"}]
      rows:
        - [1, 1900, 1900]
    query: |
      select id, total from orders
    language: sql
    source: pg-prod
  - id: flow
    description: The matching path
    view: diagram
    inline: |
      flowchart LR
        A --> B
  - id: shot
    description: Report page
    view: image
    data: shot.png             # file in this folder; images always a file
---
Optional longer explanation.
```

Per step exactly one of `inline` (table, document, keyvalue, chart as YAML or JSON flow; diagram as
block scalar) or `data` (file in the proof folder: `.json` for structured views, image file for
`image`). Inline cap 64 KiB per step; over it submit answers `invalid` with the fix ("move the data
to proofs/<slug>/<id>.json and set data: <id>.json"). Inline YAML is
converted to JSON preserving integer precision and key order (yaml.v3 node walk, not `any`
decode). Step `id` replaces `keepOrdinal`/`replacesOrdinal`: same id same hash = unchanged, same id
changed = changed, new id = added, missing id = removed. Caps and checks are today's
(`proof/validate.go`, 100 steps, 32 MiB per proof, image caps); errors name file, line and step id.

## 6. Writer matrix

Cells: R read, A create, W rewrite, D delete, `-` none. run = headless step run, manual = manual
step session, review = review agent, orch = orchestrator on its own task, child = a child task's
agent on the parent's folder. Hand = user's editor while the app runs.

| File | run | manual | review | orch | child | user UI | hand | Kira |
|---|---|---|---|---|---|---|---|---|
| `README.md` | R | R | R | R (+children R) | - | R | - | W |
| `task.yaml`, `state.yaml` | R | R | R | R (+children R) | - | R | - | W (coalesced) |
| `notes/*.md` | R | R | R | R | - | A W D | A W D | A (create, capture, migration) |
| `handovers/draft-<cur>.md` | A W | A W | A W | A W | - | W D | W | D, rename on move |
| `handovers/draft-<other>.md` | - | - | - | - | - | W D | W | D |
| `handovers/<NNNN>-*.md` | R | R | R | R (+children R) | - | W D | W | A (move, run, brief, pass) |
| `links.yaml` | A W own entries | same | same | same | - | W | W | W (UI, migration) |
| `proofs/<slug>/*` | A W | same | same | same | - | D (delete proof) | W | W (restore, migration), D |
| children's folders | - | - | - | R | - | - | - | W |

Collision rules:

| File | Rule | Residual, why acceptable |
|---|---|---|
| notes | one file per note; agent change = prompt; user writes win | two windows on one note: last UI save wins, as today |
| draft of current stage | changed by someone else after this session's last sync and this version differs = warn-and-confirm | today's upsert overwrites silently; strictly better |
| handed handovers | one file each; agent change = prompt | none |
| `links.yaml` | entry diff: new and own entries silent; others' entries = prompt; file changed by another after this session's sync = warn-and-confirm | small file, rare overlap; one file keeps one read |
| proofs | single owner (creating session); another agent's change = prompt; agent and user editor share the rule | user deletes a proof an agent still edits: next write recreates it as new |
| `task.yaml`, `state.yaml`, README | Kira only; any other write restored | an unsandboxed agent can scribble; restored at next submit or rewrite |

"Last sync" = launch or last accepted submit of the session, held in memory per
session key. A recovered headless run has no base: collisions fall back to plain classification.

## 7. Submit, proofs, agent guidance

### 7.1 `submit`

`submit` on `kira-space`: `{paths?: [relPath], finished?: bool, reason?: string}`.

- Empty `paths` = scan the whole task folder.
- `finished` replaces `handover {finished}` for sessions without a run: raises the "move to next
  stage" prompt (state signal, so an argument, never a file field).
- `reason` (1 to 500 runes): required only when the submit holds a change that needs the user's
  approval (other author's link, Jira or PR slot replaced, note or handed handover changed, accepted
  proof changed, collision). Optional and ignored otherwise. Shown in the approval. One reason per
  submit; split submits for different reasons.
- Missing `reason` on an approval item: that item is `invalid`, nothing of it applied, other items
  continue. Message names the field and gives an example, one line:
  `proofs/orders/proof.md: needs reason (the user approves changes to an accepted proof). Call submit again with {"reason": "order 12 corrected"}.`
- Description (registry, short): `Call after editing files in your task folder: Kira validates and
  answers per file (accepted, pending user, invalid with line). Changes not submitted do not count.
  Pass reason when you change something the user approves: an accepted proof, a note, a handed
  handover, another author's link.`
- Reply wording (fixes spike 1 F3 forgotten submit and 1c retry): first line a count summary
  `accepted 2, pending user 1, invalid 0`; then one line per file; invalid lines end with what to
  change. Silent-accept items get no notification.

Granted to every Space grant (runs, sessions, review agent, orchestrator).

`finish_step` keeps status, reason, conflictedFiles, lastGitError, tried; `summary` goes. It runs
an implicit submit first (no reason: approval items in it come back `invalid` in the reply and
the finish still records). Ok status, route continues, no draft for the step = refused: "write
handovers/draft-<step>.md, then call finish_step" (Q7). Reply: ack, submit results, artifact list
as today (`finishContent`).

### 7.2 Algorithm (under `taskMu`)

1. Walk the folder (depth cap 3, `Lstat`; symlink, hard link count above 1, FIFO, device refused
   per file; at most 500 files, 64 MiB per task, 32 MiB per proof).
2. Classify path by pattern into kind; no match = `ignored`. Paths under `proofs/<slug>/` group
   into one proof unit per slug, decided all or nothing (7.5).
3. `Store.Stat`: size and version equal to `task_files` = unchanged, no read.
4. Read up to the kind cap, sha256. Equal to `accepted_sha256` = refresh version only.
5. Parse and validate: configfs YAML pipeline (line, column), kind schema, kind rules (handover
   16 KiB, note 1 MiB, link rules, proof validators exported from `internal/proof` as
   `ValidateStep(view, raw)`, image checks of section 10). Invalid = per-file `invalid` with
   message; the cache keeps last good; other files continue (P638 pattern).
6. Missing file with an index row = proposed delete.
7. Semantic diff against the cache: text = unified hunks (go-udiff); links = entry add, change,
   remove; proof = step diff by id plus name and summary.
8. Decide per change from the matrix (7.3) with writer role and entry owner; approval items without
   `reason` = `invalid` (7.1).
9. Collision upgrade: `accepted_at` after this session's last sync and `accepted_by` not this
   session = warn-and-confirm prompt naming who changed it and when.
10. Apply silent changes: intent row, cache rows and `task_files` in one transaction, blob copy for
    images, intent removed. Prompted changes: `review = pending`, one approvals item per submit.
11. Denied and read-only writes: restore the file from cache (or regenerate a Kira file), intent
    first, then atomic write.
12. Reply per file: `accepted`, `pending`, `rejected` (reason), `invalid` (message, line),
    `restored` (read-only), `unchanged`, `ignored`. Update the session's last sync.

Kira's own writes (UI edits, stage move rename, restore, migration): intent row, atomic write
(`renameio`), cache and index in one transaction, intent removed. Start-up reconcile: an intent
whose file hash matches completes the DB side; otherwise dropped. File is truth, retry safe.

### 7.3 Prompt, notify and blocking matrix

| Change by an agent | Result | Who hears |
|---|---|---|
| Draft of current stage, created or edited | silent accept | board push; panel "draft updated" |
| Draft of another stage | rejected, file removed: "The task moved on to <step>" | agent |
| Handed handover edited or deleted | prompt (diff, Approve, Deny) | user via approvals, desktop and phone |
| User note edited, deleted or created | prompt | user, desktop and phone |
| New link, own link edited or removed, empty Jira or PR slot filled | silent accept | panel forward chip |
| Another author's link edited or removed, Jira or PR slot replaced | prompt | user, desktop and phone |
| New proof | accept, existing "proof ready" notification | user |
| Accepted proof changed | pending, existing `proof.review` approval with step diff | user |
| Proof deleted by an agent | pending; deny restores | user |
| `task.yaml`, `state.yaml`, README written | restored | agent (`restored`) |
| Collision (7.2 step 9) | warn-and-confirm prompt | user |
| Invalid file, or approval item without reason | `invalid` with line or field | agent; UI problem strip |

Old tool behaviour (code at base `c9072e38d`). a = returns at once; b = blocks for the user; c =
waits on something else. Paths under `apps/kira-space/internal/` unless `agentmcp/`
(`internal/agentmcp/`).

| Tool | Server | Behaviour | Code |
|---|---|---|---|
| `finish_step` | kira-space | a: stores the finish, applied at run exit | `agentmcp/mcp.go`, `ade/runs.go` |
| `handover` | kira-space | a: upserts the stage draft; `finished` opens the move prompt, no wait | `agentmcp/handover.go`, `ade/handover.go` |
| `task_info` | kira-space | a; `waitSeconds` c: waits for the prepare script, with progress | `agentmcp/space.go`, `ade/agenttools.go` |
| `declare_repos` | kira-space | a | `agentmcp/space.go`, `ade/agenttools.go` |
| `request_branches` | kira-space | a for existing worktrees; rest b: one request, up to 30 min | `agentmcp/space.go`, `ade/branchrequest.go` |
| `rename_branch`, `change_branch` | kira-space | refusal a; else b, same 30 min wait | `ade/branchrename.go`, `ade/branchchange.go` |
| `report_artifacts` | kira-space | a: links stored; kind proof attaches a closed proof | `agentmcp/space.go`, `ade/artifacts.go`, `ade/proof.go` |
| `operations` | kira-space | a | `ade/agenttools.go` |
| `acquire_lease` | kira-space | a: `held` or `queued`; grant later as a notice | `agentmcp/leases.go`, `ade/leases.go` |
| `release_lease`, `lease_status` | kira-space | a | `agentmcp/leases.go` |
| `run_outcome` | kira-space | a | `agentmcp/outcomes.go` |
| `create_task` | kira-space | b: `task.create` approval, up to 30 min | `agentmcp/orchestrator.go`, `ade/taskcreate.go` |
| `list_tasks`, `task_chain` | kira-space | a | `agentmcp/orchestrator.go`, `ade/orchestrator.go` |
| `proof_save` | kira-studio | a. New: `closed`, ready prompt, attach. Edit: `pending_review`, `proof.review`, no wait | `dbmcp/proof_tools.go`, `proof/service.go` |
| `proof_list` | kira-studio | a | `dbmcp/proof_tools.go` |

New item to old tool:

| Item | Old tool | Old | New |
|---|---|---|---|
| Draft of current stage | `handover`, `finish_step` summary | a | accepted at once |
| `submit {finished}` | `handover {finished}` | a, prompt raised | accepted, prompt raised, no wait |
| Draft of another stage | `handover` refusal | a | rejected at once |
| New or own link, empty slot | `report_artifacts` | a | accepted at once |
| Other author's link, slot replaced | none | - | `pending` at once |
| Note, handed handover changed | none | - | `pending` at once |
| New proof (files plus `submit`) | `proof_save` new | a | accepted at once, ready prompt |
| Proof change (files plus `submit`) | `proof_save` edit | a | `pending` at once, `proof.review` |
| Proof deleted by agent | none | - | `pending` at once |
| Kira-owned file written | none | - | `restored` at once |
| Collision, invalid | none | - | `pending` or `invalid` at once |
| Child task | `create_task` | b | unchanged: `create_task`, b, 30 min |

So `submit` never blocks. Tools that blocked keep their waits: `branches` (request, rename,
change), `create_task`, kira-studio db tools. From Part 5 a `branches` request also waits for the
worktree setup, replacing `task_info waitSeconds` (9.2).

A `pending` item's answer reaches the agent through the lease notice path (`SendIfIdle`, hook
reply, next tool result `notices`) and the `pending` list in `state.yaml`. Deny restores the
accepted copy on disk. A new submit of the same file replaces its open item. Approvals kind
`details.edit` (new) reaches desktop and phone (`mobileweb/approvals.go` allowlist plus phone
view).

User changes:

- UI edit: Kira writes the file, accepted at once; recorded in the panel forward set as today, the
  forwarded message names the relative path.
- Hand edit while the app runs: no watcher. Stat on task open, Details focus, Refresh, every submit
  and `finish_step`, and at start for live tasks. A change not made through submit or UI shows
  the banner "N files changed on disk, not submitted" with a diff per file, Accept or Discard.
  Accept is the user's consent.

### 7.4 Edit protection

- Agents read freely. No lock.
- Ask-to-edit through the matrix, enforced at submit; Kira restores anything rejected.
- Sandbox on: task folder in `Allow`, literal write denies on `README.md`, `task.yaml`,
  `state.yaml`; orchestrator gets children folders in `Read`. Sandbox off (default, every Linux
  host): `DisallowedTools` `Edit(//<abs>/task.yaml)` and `Write(...)` for the same files; restore
  covers the rest.
- Every run, session and review agent gets `--add-dir <home>/tasks/<id>` (and existing children for
  an orchestrator). Nothing else under `<home>/tasks/` is granted.

### 7.5 Proofs through files and `submit`

User decision: no `proof` tool and no `create` tool. The agent writes the files itself and calls
`submit`, for a new proof and for an edit. Cost: spike F2 FP 1.31, F3 FA 1.18 and FP 1.24 times T
(Part 1b, 1c); reported, not gated (14.1).

New proof, agent side:

1. Picks a slug: the folder name, `[a-z0-9][a-z0-9-]*`, 1 to 40 chars, unique per task.
2. Writes image and large data side files first (`Write` or a Bash copy from its worktree into
   `proofs/<slug>/`): PNG, JPEG, WebP, SVG under the section 10 rules; `<id>.json` over 64 KiB.
3. Writes `proofs/<slug>/proof.md` with `Write` (new file: Claude Code needs no prior Read; 11 of
   11 new-file Writes in the spike transcripts passed with none, analysis section 2).
4. Calls `submit` once (or `finish_step`, which submits first). No `reason`.

Format source, read once, no extra file: the README carries one short example block (7.6) beside
the `Proofs:` index. Why the README and not `proofs/README-example` or a Kira template path: the
spike files arms read README in 3 of 3 runs before acting, so the example rides a read the agent
already makes; a separate file costs one more turn (0.007 to 0.009 USD, about 10 percent of an F2
T run, analysis section 2) and a second path to name. The block is 8 lines, paid only when the
README is read. Fallback if Part 4's F1 context ratio rises above 1.0 because of it: the block moves
to a Kira-owned read-only `proofs/EXAMPLE.md` and the README keeps one line pointing at it.

Submit, Kira side (under `taskMu`, the 7.2 pipeline):

- Walk and classify `proofs/<slug>/**`. A folder name that fails the slug rule, or a file deeper
  than `proofs/<slug>/`, comes back `invalid` with the fix: `proofs/Orders Match/: folder name must
  be a-z, 0-9 and -; rename it to proofs/orders-match/.` (Classify today reports these `ignored`;
  Part 4 adds the proof-folder hint.)
- Group per slug into one proof unit: `proof.md` plus the side files its steps name in `data:`.
  The unit is decided all or nothing: one invalid part (proof.md, a named side file missing or
  invalid) leaves every file of the unit on disk untouched, accepts nothing of it and lists each
  problem. A side file no step names is `invalid` ("not used by proof.md; name it in a step's data
  or remove it"), also untouched.
- Validate `proof.md`: frontmatter between `---` lines, YAML through the configfs node pipeline
  (line and column), `kira: proof/1`, `name` 1 to 80 chars, `summary` (new proof), 1 to 100 steps,
  step `id` `[a-z0-9-]{1,40}` unique, `view` in the enum, exactly one of `inline` or `data`, data
  through `proof.ValidateStep(view, raw)`, caps of section 5 and 10.
- Error lines name file, line, step id and field, and end with a one-line fix. Examples:
  - `proofs/orders/proof.md:7: steps[totals].inline: rows[3] has 2 cells; columns has 3. Add the missing cell.`
  - `proofs/orders/proof.md:12: YAML: mapping values are not allowed here. Quote the value: description: "Totals: all rows".`
  - `proofs/orders/proof.md:9: YAML: found a tab. Indent with spaces.`
  - `proofs/orders/proof.md:1: no frontmatter. Start the file with --- and end the YAML with ---.`
  - `proofs/orders/proof.md:15: steps[shot].data: shot.png not found in proofs/orders/. Write the image there first.`
  - `proofs/orders/proof.md:4: name "Orders match ledger" is used by proofs/orders-ledger/. Pick another name or edit that proof.`
  - `proofs/orders/proof.md:20: steps[big].inline is 80 KiB; the limit is 64 KiB. Move it to proofs/orders/big.json and set data: big.json.`
  A YAML parse error maps the yaml.v3 message to one fix hint per known class (tab, unquoted colon,
  bad indent, unclosed flow, duplicate key); unknown classes keep the line and the raw message.
- New proof (no accepted `proof.md` in the folder): accepted silently except the existing "proof
  ready" notification; session, revision and step rows (caches, `step_key` = step id), task link
  and `task_files` rows commit in one transaction; README `Proofs:` line regenerated.
- Edit of an accepted proof: `reason` required (7.1 error text, `Call submit again with {"reason":
  ...}`); with it, the pending revision from disk and the existing `proof.review` approval with the
  step diff (one item per proof unit). Never blocks.
- Deleted folder of an accepted proof: pending delete (7.3); deny restores files from caches and
  blobs.

Slug collision:

- Folder exists with an accepted `proof.md`: writing it again is an edit, not a new proof. Claude
  Code refuses an overwriting `Write` without a prior Read, so the agent learns the file exists;
  after a Read the overwrite goes through the edit path (reason, approval). No `-2` suffixing:
  Kira never renames an agent folder.
- Same `name` (case folded, `NormalizeName`) in another folder: `invalid` naming the other folder.
- Folder exists without an accepted `proof.md` (orphan or partial from an earlier invalid submit,
  or a crash before submit): treated as a new proof; nothing is cleaned up by Kira. Leftovers show
  in the disk banner like any unsubmitted change; the user may Discard them.

Submit reply for proofs (fixes spike gap 6, `proof_list` without step data). Per unit, one line,
after the count line of 7.1:

```
proofs/orders-match-ledger/: accepted, new proof "Orders match ledger": totals (table), flow (diagram), shot (image); 3 steps, 1 image
proofs/orders-match-ledger/: pending user, change to "Orders match ledger": totals changed, why added, flow removed (order 12 corrected)
proofs/orders/: invalid, 2 problems (lines follow)
```

Structured result: `FileResult` gains `proof: {slug, name, status, steps: [{id, view}], counts:
{steps, images, dataFiles}, changed?: {added, changed, removed}}` once per unit (on the `proof.md`
entry; side files of an accepted unit are folded into it, not listed again). So the agent learns
the step ids and views that `proof` used to return, without a Read. `proof_list` stays removed;
the README `Proofs:` index and the prompt `Proofs:` line name every proof with its steps.

Edit, agent side: Read `proofs/<slug>/proof.md` (one Read, data inline), Edit the step by id (keep
the `id` to keep view state), write new side files, `submit {reason}`.

What replaces the tool's idempotency and atomicity:

- Per-file hash: unchanged files (size and version equal, or hash equal to `accepted_sha256`) are
  `unchanged`, no work; a second `submit` with nothing changed answers `unchanged` per file. A
  second submit of a still-pending edit replaces its open approval item (7.3), never a second item.
- Proof unit commit: everything Kira writes to SQLite for one unit (proof rows, caches,
  `task_files` rows) commits in one transaction; image bytes go to the content-addressed blob store
  first (same hash = same file, idempotent), then the transaction. A crash before the commit
  leaves the files unaccepted on disk; the next submit or the start-up stat sees them again and
  reaches the same result. Kira-side file writes (restore on deny, migration) use the Part 2 intent
  journal (`task_file_intents`, `Recover` at start).
- Proof.md first, image later, same submit: the unit reads every file once and validates them
  together, so path order inside the folder does not matter. Image written later, separate submit:
  if the first submit had `data: shot.png` and no `shot.png`, the unit was invalid and nothing was
  accepted; the second submit accepts the whole proof as new. If the first submit accepted a proof
  without the image step, adding it later is an edit of an accepted proof: reason and approval, as
  `proof_save` edits were. The README example line says "write images first, then proof.md, then
  submit once" to steer agents away from that.
- Agent writes during a submit: the unit uses the bytes it read and hashed; a later write shows as
  a new version on the next submit or stat.
- `taskfiles.Request.Tool` and `Decide`'s `tool` argument (Part 2, "submit" or "proof") lose the
  "proof" value; Part 4 removes the field and the argument (error texts always say `submit`).

### 7.6 Agent guidance: README, headers, prompts, ToolSearch

Spike gaps addressed here: forgotten submit (spike 1 F3), missing `reason` (1c F3), discovery Bash
calls (1b, 1c), second `AGENTS.md` read (1b), failed calls before ToolSearch (1b F1), proof format
discovery without a tool (1c FP read README plus `AGENTS.md`).

README, one per task folder, Kira-written, at most 35 lines, no `AGENTS.md`:

```
# Task <id>: <title>
Edit, then call submit: a change counts only after submit answers per file. Pass reason when the change needs the user's approval.
Read-only, edits undone: README.md, task.yaml, state.yaml (step, branches, setup, leases, runs, children).
Write: handovers/draft-<step>.md (your stage's handover, Markdown, <=16 KiB). links.yaml. proofs/<slug>/proof.md (format below).
Ask first (submit with reason): notes/*.md (user notes), handovers/NNNN-*.md (handed), others' links, accepted proofs.
New proof: slug a-z0-9-; write images and big data into proofs/<slug>/ first, then proof.md, then submit once. Example:
---
name: Orders match ledger
summary: All 20 orders match their ledger lines.
steps:  # id a-z0-9- (keep it on edits); view table|document|keyvalue|chart|diagram|image; inline: data (<=64 KiB) or data: file here (.json .png .jpg .webp .svg)
  - {id: totals, description: Totals agree, view: table, inline: {columns: [{name: id}, {name: total}], rows: [[1, 1900]]}, query: "select id, total from orders", language: sql}
  - {id: shot, description: Report page, view: image, data: shot.png}
---
Proofs: orders-match-ledger "Orders match ledger": totals (table), flow (diagram), why (document)
Children: c-billing "Billing refunds" ../c-billing/ (latest handover: handovers/0003-review-to-implement.md)
```

- No tool list, no rule restated that a tool description carries, formats only for kinds the task
  uses. The proof example is the one format block (7.5 says why not a separate file): 7 lines plus
  its intro line, each at most 300 bytes. `Proofs:` line `none yet.` when empty; `Children:` lines
  only for an orchestrator, one per child, cap 20.
- Cap 35 (was 25): 5 fixed lines, 8 example lines, up to 6 `Proofs:` lines leave 16 for
  `Children:`; with at most one proof all 20 children fit. Overflow keeps the existing "and N more,
  see state.yaml chain" line. The spike's lean README was 17 lines; this one stays under that plus
  the example.
- README is regenerated on accept of a proof or link change, on child create, on stage move.
  Byte-equal skip.

`AGENTS.md`: none in `<home>/tasks/` or a task folder. Claude Code reads `CLAUDE.md`, not
`AGENTS.md`, and an `--add-dir` folder's memory files are not loaded by default, so no client needs
it. The configfs `WriteDocs` `AGENTS.md` of config folders stays as is.

Step, manual, review and start prompts (`AdeStepPrompt`, `AdeManualMessage`, `AdeReviewMessage`,
`AdeStartMessage`) gain one fragment `headless.space.files` after the handover block:

```
Task folder: <abs path>. Write your handover to handovers/draft-<step>.md there.
Proofs: orders-match-ledger "Orders match ledger": totals (table), flow (diagram), why (document)
Kira tools for this step: finish_step, submit. Load them first with ToolSearch select:mcp__kira-space__finish_step,mcp__kira-space__submit
Edit task files, then call submit (finish_step submits for you).
```

- Latest handover and notes stay inlined (Q13), so the common input needs no read.
- `Proofs:` line only when the task has proofs (cap 5 lines, 300 bytes each).
- Tool line: names the always-needed tools of the grant (run: `finish_step`, `submit`; session:
  `submit`; orchestrator adds `create_task`). The load sentence comes from a new
  optional harness interface `harness.ToolLoader{ LoadHint(names []string) string }`; Claude
  returns the `ToolSearch select:` sentence with `harness.ToolNames` names, other providers ""
  (no deferral). One select call loads all named schemas in one turn.
- Last line: the submit reminder. `finish_step` suffix stays last in step prompts.
- `HeadlessSpaceInstructions` rewritten: `Kira Space tools for this task. Task state is in
  state.yaml in the task folder. Declare repos and create or change branches only with branches;
  never git branch or git worktree. A branches request returns when the worktree is ready. Edit
  task files, then call submit.` (Part 3 writes it without the `state.yaml` and setup sentences;
  Part 5 adds both, 9.2.)

ToolSearch limits (spike 1b, measured): built-in tool schemas 33.1k tokens and the 2.4k system
prompt are fixed by Claude Code (87 percent of turn 1); MCP schemas cost 0 at turn 1 and are paid
on load; a listed tool name costs about 10 tokens. Kira controls: tool count, short stable names,
short descriptions and schemas (paid on load), the load hint, the README and the inlined prompt.
Calling a deferred tool before loading it fails with "input schema was not in your prompt"; the
hint removes that failure, it does not remove the ToolSearch turn.

## 8. State file (Part 5)

`state.yaml` header: `# Read-only. Kira rewrites this file. Change state only with Kira tools:
branches, lease, cancel_operation, finish_step. A branches request waits until the worktree is
ready.` (Part 2's `headers.go` text names `task_info`; Part 5 replaces it.) Coarse state only:

```yaml
rev: 41                       # increments per rewrite; no file times
task: {id, kind, status, step: {id, name, position, of}, workflow}
repos:                        # per repo: branch, base, worktree, setup {state, note}, mismatch
registered: [...]             # names, always (task_info includeRegistered goes)
leases: [{queue, state: held|queued, position, holder}]
operations: [{opId, kind, subject, startedAt}]
runs: [{runId, kind, step, state, outcome}]   # last 3; outcome capped at 2 KiB
chain:
  parent: {id, title, path: ../<id>/}
  children: [{id, title, status, step, finished, archived, lastHandover: ../<id>/handovers/<file>, preview}]
pending: [{path, kind, since}]  # submit items waiting on the user
```

Worktree paths are absolute: host-local mirror, regenerated per host, never content (Q4). One
writer goroutine, per-task dirty flag, coalesce 150 ms, byte-equal skip, `renameio`. Triggers:
board change (`notifyBoard`), lease book change, op start and end, setup change, `task_files`
review change. Live tasks only; one final write at archive.

`task.yaml`: id, title, kind, workflow, parent, repos, created. Same header and writer.

Orchestrator children created after launch are outside the fixed Seatbelt profile and `--add-dir`
list. `chain.children[].preview` (first 500 bytes) and the `create_task` reply cover them until the
next session. `create_task` reply `next` text changes from "read its final handover with
task_chain or list_tasks" to "read its handovers in ../<id>/handovers/ (state.yaml chain lists
it)".

## 9. MCP tools before and after

### 9.1 File plus submit versus tool, per flow

| Flow | Path | Spike numbers (sonnet, median cost F/T; final context) | Why |
|---|---|---|---|
| Handover (run) | `handovers/draft-<step>.md` plus `finish_step` (implicit submit) | F1 FL 1.08, FF 1.12 (inconclusive); context 1.00 to 1.01 (parity) | parity; prose edits are natural |
| Handover (session) | draft file plus `submit {finished}` | same shape as F1 | `handover` tool goes |
| Notes read | prompt inline plus `notes/*.md` | not a flow; prompt inlines notes in both arms | no tool existed |
| Notes, handed handovers change | file plus `submit` with reason, prompt | no data; rare agent action | no tool existed |
| Links | `links.yaml` plus `submit` | not measured; same edit-submit shape as F1 | `report_artifacts` goes |
| Task state read | `state.yaml` | F5 reads in both arms were 0 to 2 | replaces `lease_status`, `operations` list, `run_outcome` |
| Orchestrator chain read | `state.yaml` chain plus children's `handovers/` | F4 FL 0.72, FF 0.79; context 0.87, 0.88 | files win; `task_chain` returned whole chains |
| New proof | agent Writes `proofs/<slug>/proof.md` (and side files) plus `submit` | F2 files FP 1.31 (1c), haiku 1.38 (1) | user chose files; gap reported (14.1) |
| Proof edit | Read, Edit `proof.md` by step id, `submit {reason}` | F3 files FL 1.28, FA 1.18 (inconclusive), FP 1.24 | user chose files; gap reported (14.1) |
| Child task | `create_task` tool, unchanged | F6 files FL 1.36, FF 1.40 vs gate 1.1; context 1.06 to 1.09 | user dropped drafts |
| Branch setup wait | `task_info {waitSeconds}` until Part 5; then blocking `branches` request plus session push (9.2) | F5 FL 1.26, FF 1.17 (1 run each, inconclusive); haiku 1.30 | no wait tool (user); F5 re-benchmarked |

### 9.2 Setup wait (settled, Part 5)

Until Part 5 lands, `task_info` stays, schema and handler unchanged. User direction: `task_info`
goes; at most a worktree-ready check survives; the model is leases (state in `state.yaml`, Kira
pushes on grant).

Facts (code, `codegraph_explore`):

- A setup starts only in `startSetup` (`ade/setup.go`). Seven callers: `createBranch` (agent
  request, after approval, `ade/agenttools.go`), `applyExisting` (request for an existing branch,
  `ade/branchadopt.go`), `prepareAttached` (UI attach, review to task, `setTaskBranch`,
  `addHostLink`, `ade/board_writes.go`), `launchGate` (session launch, takeover, rebase; fresh
  worktree, `ade/launches.go`), `prepareWorktree` (step start, `ade/runs.go`), `RetrySetup` (user).
  Rename and change start none (change switches in the same folder). `create_task` creates branch
  rows only; the child's setup runs at its step start.
- No agent launches before its launch worktree is ready: `queueRun` holds a step run `pending`
  (`gateNote`: waiting, failed, worktree gone) until `onSetupReady`; `launchGate` refuses session,
  takeover and rebase with `E_PREPARING` (UI waits, `useSetupWait`) or the failure text.
- Mid-run waits come from: the agent's own branch request (new branch, or existing branch; an agent
  step with an unnamed branch starts in the repo root and requests mid-run), and a setup the agent
  did not start on a branch it uses (user Retry, UI attach, another session's request).
- Roles with Space tools: headless step run (`superviseAgent`, `OneShot`, one turn) and interactive
  sessions (manual, run-step TUI, review agent, orchestrator session). Rebase runs and review rounds
  get no Space grant (`space := !rebase && !round`).
- A request already blocks up to 30 min for approval (`defaultBranchRequestWait`). Harness MCP call
  timeout 35 min (`harness.MCPCallTimeout`). Prepare timeout 15 min default, 2 h max.
- Lease channel (`ade/leases.go`): `deliverLease`, `flushLease` (`SendIfIdle`), Stop and
  UserPromptSubmit hook replies, lease tool reply `notices`. Reaches sessions mid-turn and idle.
  Misses headless runs: `flushLease` returns for `run:` keys, runs emit no hooks, and `endLeases`
  drops pending messages when the one turn ends.
- Boot `Recover` (`ade/recover.go`) marks running setups failed with the restart reason, sets
  pending runs' note to failed and stops all sessions; nothing resumes by itself.

Design:

- Blocking request, every role. `branches {action: request}` replies per slot only once its setup
  is `ready` or `failed`, not at `running`. Covers new branches (after approval and create) and
  existing branches whose setup is running, whoever started it. One deadline per call for approval
  plus setup: call start plus 33 min (`harness.MCPCallTimeout` minus 2 min). Progress
  `preparing <branch>: <note>` through `ReportProgress`, as `BranchStatus` does now. The loop
  (`setupDone` channel, 5 s row re-read fallback) moves from `BranchStatus` to new
  `ade/setupwait.go`, called by `RequestBranches` (`ade/branchrequest.go`) and `applyExisting`
  before the reply is built. The same request again is the wait and the ready check: no other tool,
  no polling.
- Waiter leaving (Esc, run end, context cancel) never cancels the setup. `cancel_operation` on the
  setup op fails it; a blocked waiter then gets `failed`.
- Push, interactive sessions only, supplement. In-memory watcher per (repo, branch, session owner
  key), added when a request to that session ends before the setup does (deadline, or wait
  cancelled by Esc) or replies `failed`. At setup end (`runSetup` end, `recordSetupFailure`) Kira
  delivers to each watcher not blocked in a request on that branch, through the lease delivery path
  generalized to `deliverNotice` and `takeNotices` (`ade/leases.go`); notices also ride on
  `branches` replies. Ready removes the watcher; failed keeps it, so a later Retry reaches it.
  `run:` owners get no watcher (unreachable; their request blocks). Watchers drop on owner end
  (`endLeases`).
- Reply `Next` texts (`branchInfo`):
  - ready: `Ready. Work in <path>.` (unchanged)
  - failed: `The worktree setup failed. Tell the user; do not edit until it is fixed. Reason:
    <note>` (unchanged)
  - deadline, run: `Still preparing. Call branches with action request and the same branch again
    to keep waiting; do not edit in <path> yet.`
  - deadline, session: `Still preparing. Kira tells you when it is ready; do other work or end
    your turn. Do not edit in <path> yet.`
  - none: unchanged except `request_branches` reads `branches` with action request.
- Push texts:
  - `Kira: the worktree of <branch> (<repo>) is ready at <path>. Continue the work that waited for
    it.`
  - `Kira: the worktree setup of <branch> (<repo>) failed: <note>. Tell the user; do not edit
    there until it is fixed.`
- `state.yaml` `repos[].setup: {state: none|running|ready|failed, note}` from `setupInfoFrom` (no
  row with a worktree = `ready`, no worktree = `none`). `note` = failure reason when `failed`, else
  omitted. Rewritten on `startSetup`, setup end and `recordSetupFailure` (through `notifyBoard`),
  and once for live tasks at board start after `Recover`.
- Restart: watchers and queued messages are memory only. `Recover` marks running setups failed
  with the restart reason and stops sessions; `state.yaml` shows `failed` with that reason. A
  resumed agent tells the user; after Retry a `branches` request waits again.
- Timeouts: the wait is bound by the call deadline; the setup by the prepare timeout. A setup
  outlasting the call gets the deadline reply: a run calls again, a session gets the push.
- Launch gating (`queueRun`, `launchGate`) stays as is.

Rejected: (a) wait-only `task_info`, one more tool (kept only as the F5 fallback, 14.1); (c) launch
gating alone, exists but misses mid-run requests; (d) push alone, cannot reach a headless run
mid-turn or after its one turn; `branches {action: wait}`, extra schema text (spike F5 search
context 1.7k to 1.8k against T 0).

### 9.3 Tool table

| Tool | Server | Before | After |
|---|---|---|---|
| `finish_step` | kira-space | runs; summary = handover | kept; summary moves to draft file; implicit submit |
| `submit` | kira-space | | new, every Space grant |
| `task_info` | kira-space | Space | until Part 5 unchanged; then removed: task facts in `state.yaml`, setup wait in `branches` request (9.2) |
| `declare_repos`, `request_branches`, `rename_branch`, `change_branch` | kira-space | Space | merged: `branches {action: declare|request|rename|change}` |
| `acquire_lease`, `release_lease` | kira-space | Space | merged: `lease {action: acquire|release}`; notices stay on replies |
| `operations` | kira-space | Space | list in `state.yaml`; cancel: `cancel_operation` |
| `handover` | kira-space | sessions without run | removed: draft file plus `submit {finished}` |
| `run_outcome` | kira-space | task grants | removed: `state.yaml` `runs` |
| `report_artifacts` | kira-space | Space | removed: `links.yaml` plus `submit`; kind proof gone with attach |
| `lease_status` | kira-space | Space | removed: `state.yaml` `leases` |
| `create_task` | kira-space | orchestrator | kept unchanged; reply `next` text updated (8); brief written as the child's first handed file |
| `list_tasks`, `task_chain` | kira-space | orchestrator | removed: `state.yaml` `chain` plus children's `handovers/` |
| `git`, `reply_review_comment`, `fetch_remote`, `push_branch` | kira-space | conditional | unchanged |
| `proof_save`, `proof_list` | kira-studio | every DB MCP client | removed for every client (Q10) |

Counts (conditional tools excluded):

| Role | Before | After | Tools after |
|---|---|---|---|
| Run | 12 | 5 | `finish_step`, `submit`, `branches`, `lease`, `cancel_operation` |
| Session without run (manual, review agent) | 12 | 4 | run set minus `finish_step` |
| Orchestrator run | 15 | 6 | run set plus `create_task` |
| Orchestrator session | 15 | 5 | session set plus `create_task` |
| kira-studio proof tools, any client | 2 | 0 | |

Removed names: `handover`, `run_outcome`, `report_artifacts`, `operations`, `lease_status`,
`list_tasks`, `task_chain`, `task_info`, `proof_save`, `proof_list`. Merged away:
`declare_repos`, `request_branches`, `rename_branch`, `change_branch`, `acquire_lease`,
`release_lease`. New: `submit`, `branches`, `lease`, `cancel_operation`. `lease` calls stay the
heartbeat (`mcpRecorder`).

`task_info` removal, Part 5 (paths as in 7.3):

- `agentmcp/space.go`: `addTaskInfoTool`, `taskInfoArgs`, `maxWaitSeconds`,
  `SpaceTools.TaskInfo`, `SpaceTools.BranchStatus`, `TaskInfo.Branch` go. `TaskInfo` stays as the
  declare reply shape. The request handler passes the owner key and the progress reporter.
- `agentmcp/tools.go`: `task_info` out of `SpaceToolIDs`.
- `ade/agenttools.go`: `TaskBoard.TaskInfo` and `BranchStatus` go; `branchInfo` `Next` texts per
  role (9.2). New `ade/setupwait.go`: wait loop, watchers, push. `ade/branchrequest.go`,
  `ade/branchadopt.go`: wait before the reply. `ade/setup.go`: setup end and `recordSetupFailure`
  notify watchers and the state writer. `ade/leases.go`: `deliverNotice`, `takeNotices`.
- `llmprompts/registry.go`: `HeadlessToolTaskInfo` and its args go; arg texts "from task_info"
  (declare repo, request repo, `create_task` `repos.repo`) read "from state.yaml";
  `headless.space.suffix` names `branches`, `lease`, `cancel_operation`, `submit` and says a
  `branches` request returns when each worktree is ready; its `FilesVars` example names
  `mcp__kira-space__branches`; `branches` description adds "request returns when the worktree is
  ready or its setup failed". `HeadlessSpaceInstructions` per 7.6.
- `internal/taskfiles/headers.go`: `headerState` per section 8.
- Texts and fixtures: `internal/llmprompts/testdata/golden/ade.*.txt`, `budget.tsv`, contracts
  `ade-automation.json` and `ade-run.json`, `tests/unit/ade-v2-dialog.spec.ts` `SPACE_NOTE`,
  `termflow/smart_test.go`, site `reference/mcp-tools.md`. `tests/claude/details_spike*` and the
  bench T stubs keep `task_info` (frozen baseline).

## 10. Proofs

- A proof exists only in a task folder: `<home>/tasks/<id>/proofs/<slug>/` (Q10). Slug from name,
  unique per task (case folded, as `name_key`).
- New proof through files plus `submit` (7.5): Kira creates session, revision and step rows
  (caches, `step_key` = step id), links it to the task (state), raises the ready prompt. Edit:
  pending revision from the disk version, current revision stays the accepted copy; approve copies
  view state of unchanged steps by step id; deny restores files from caches and blobs. Step diff
  and `origin_ordinal` derive from step ids.
- Delete: the user deletes a proof from its Details entry or its window; folder and rows go.
- Retention of 30 (`pruneClosed`) goes.
- View state stays in `proof_view_state` (session, rev, ordinal).
- Window: one proof per window, opened by (task id, slug). Entry points: Details chain entry
  (`AdeProofEntry.vue`), ready prompt (`ProofReadyPrompt.vue`), `proof.review` approval
  (`approvals/ProofOpenWindow.vue`). Tabs per step, explanation, data views, compare side for a
  pending edit, screenshots unchanged. Header shows the relative path with Open and Reveal.

Removed by Q10 (verified in code):

- `addProofTools`, `ProofStore`, `ProofAttacher`, `dbmcp.Config.Proof` and `ProofAttach`
  (`dbmcp/proof_tools.go`, `bridge/dbmcp.go`), `ade.TaskBoard.AttachProof` (`ade/proof.go`), the
  `SpaceTools.AttachProof` method (`agentmcp/space.go`), `report_artifacts` kind proof, registry
  entries `DbmcpToolProofSave`, `DbmcpToolProofList`. External DB MCP clients and Studio sessions
  lose proofs.
- Window menu "Validation proof" (`appshell/menu.go`, `appwire/menuactions.go`) with its tests
  (`appshell/menu_test.go`, `proofflow/image_test.go` menu case). `ProofService.OpenWindow` with no
  session (`bridge/proof.go`): an open names (task id, slug).
- History pane (`ProofHistory.vue`, `ProofService.Sessions`).
- Unattached sessions and attach: `ProofChoices` (`ade/proof.go`, `bridge/adetask.go`),
  `AdeProofAttachMenu.vue`, `useProofChoices`, `useLinkProof`, `LinkProof`,
  `proof.Service.Unattached`, `ProofRepo.UnattachedSessions`.
- Detach (`proof/tasklink.go`, `UnlinkProof`, `useUnlinkProof`): delete replaces it.
- Retention (`repos/proof.go` prune, `ProofKeepClosed`).
- Agent draft path (`proof.Service.Save`, `StartSession`, `EditSession`, `AddStep`,
  `CloseSession`, `PurgeStaleDrafts`, `StepsForAgent`, `ListForAgent`): replaced by the file
  pipeline; Part 4 removes each one with no caller left.

### Images (Q17)

Why JPEG only today (`proof/image.go`, `ProofImage.vue`): base64 inside the step payload (4 MiB
cap), stdlib `image/jpeg` decode, window hard-codes `data:image/jpeg`. Files remove the reason.

- Allowlist: PNG, JPEG, WebP, SVG. PNG and JPEG via stdlib. WebP via `golang.org/x/image/webp`
  (BSD-3; Part 4 confirms the license for the decoder used): lossy, lossless, alpha, no animation.
- Refused: GIF, BMP, TIFF, HEIC, AVIF, `.svgz`, any animated raster (APNG `acTL` before `IDAT`;
  WebP VP8X animation flag or `ANIM`).
- Sniff by content: PNG `89 50 4E 47 0D 0A 1A 0A`, JPEG `FF D8 FF`, WebP `RIFF....WEBP`; SVG =
  after optional BOM, whitespace, `<?xml?>` and comments, root `svg` in the SVG namespace.
  Extension must agree.
- Raster: size at most 10 MiB (from `Lstat`); `DecodeConfig`; each side 1 to 16384, area at most
  16 MP; animation check; full decode as integrity check.
- SVG, `encoding/xml`, reject never sanitize; error names element or attribute and line:
  - at most 2 MiB, 50,000 elements, depth 64; not well-formed refused;
  - any `xml.Directive` (DOCTYPE, ENTITY) refused;
  - elements refused: `script`, `foreignObject`, `iframe`, `object`, `embed`, `audio`, `video`,
    `animate`, `animateMotion`, `animateTransform`, `set`;
  - attributes refused: every `on*`; any value holding `javascript:`;
  - `href`, `xlink:href`: only `#fragment` or `data:image/png|jpeg|webp;base64,...` (embedded raster
    passes raster checks, counts toward 2 MiB);
  - CSS (`<style>`, `style`) and presentation attributes: `url()` only to `#fragment` or allowed
    `data:image`; `@import` and `@keyframes` refused;
  - intrinsic size from `width`/`height` (px or unitless) or `viewBox`; one required; each side 1
    to 16384, area at most 16 MP.
- Step payload `{format, width, height, bytes, file, sha256}`, no bytes. `ProofImagePayload`
  (`packages/shared/domain/proof.ts`) gains `format`, drops `base64`. The bridge reads the accepted
  file (or blob) and returns base64 plus format.
- Rendering: `ProofImage.vue` builds `data:image/<png|jpeg|webp|svg+xml>;base64,...` for an
  `<img>`. CSP `img-src 'self' data:` stays. SVG only through `<img>`: never `v-html`, inline DOM,
  `iframe`, `object` or `embed`.
- Screenshots: `modern-screenshot` draws the `<img>`; SVG without `foreignObject` does not taint.
  Proofs do not reach the phone.
- Error texts (each `proofs/<slug>/<file>: step <id>: ...`): not an allowed image; SVG element at
  line not allowed; attribute not allowed; href points outside the file; CSS loads from outside;
  DOCTYPE or entity; size missing; too large; not well-formed; extension mismatch; animated;
  side or area out of range; damaged data. Exact wording in Part 4 plan, one per check.

## 11. Migration (one-time, resumable)

Framework in Part 2, exporters in Parts 3 and 4.

1. No backup (Q11). Safety: per-task verify before rename; image payloads rewritten only after the
   task's verify passed.
2. Per task, live and archived, marker in `task_files_migration`: export to
   `<home>/tasks/<id>.migrating/`, verify (re-read, parse, per-row body hash equal, counts equal),
   rename to `<home>/tasks/<id>/` (merge only files absent there), insert `task_files` and
   `rel_path`, mark done. A crash leaves `.migrating`: deleted, task redone.
3. Notes to `notes/`, drafts to `handovers/draft-<step>.md`, handed rows to numbered files in
   `created_at` order. Child mirror rows: dropped when the child's own handed-into-done file
   exists; else written as a read-only legacy handed file.
4. Stored artifacts plus Jira and host slots to `links.yaml`.
5. Proofs (Part 4): linked sessions to `proofs/<slug>/proof.md`, step ids `s<ordinal>`, data inline
   up to 64 KiB, else `s<ordinal>.json`; image payloads decoded to `s<ordinal>.jpg` (format
   `jpeg`), payload column rewritten to metadata after verify. A pending revision becomes the disk
   version with review pending; its approval re-raises (`RaiseUnreviewed`). Taskless sessions
   (Q16): rows deleted after one log line naming each (id, name, closed time).
6. Columns stay as caches; nothing else dropped.
7. Runs at start before the board starts, with a progress line in the log; a failure leaves the
   task on SQLite reads (flag per task) and an error banner; no partial task.
8. README written per task at the end of its export.

## 12. UI changes

- Details tab: same layout, same SQLite queries. Folder bar (relative path, Open, Reveal, Copy
  path) through a bound open/reveal service scoped to `tasks/<id>` (no renderer path reaches the
  OS). `AdeDetailsDiskBanner.vue` (changed on disk, not submitted: per-file diff dialog, Accept,
  Discard; shadcn-vue Dialog and Alert, Tailwind). File problem strip (invalid file, line, Open)
  reusing `ConfigFileProblem` styling.
- Approvals: `details.edit` body, one row per file change with diff and reason, per-item approve
  like `branch.request`. Phone view for `details.edit` (P637 pattern). Proof edits keep
  `proof.review`.
- Proof window: no history pane, opened per proof from a task only, path header, image by format.
  Window menu item, attach menu and detach removed.
- Panel forward bar: item labels carry the relative path.
- Stage dialog, handover prompt, mobile board: unchanged (read caches).
- Server state through TanStack Query; banner state through a query on a new `DiskChanges` call;
  no new Pinia store unless a component needs shared client state.

## 13. Storage seam (future cloud)

`apps/kira-space/internal/taskfiles/store`: `Store { Stat, Read, List, WriteAtomic(rel, data,
ifVersion), Remove, Rename }`, relative paths only, opaque `Version` (local: size plus mtime ns;
object storage: ETag). Only the local adapter joins a root, follows the filesystem, checks symlinks
and reads file times. Submit, validators, migration and UI wire carry relative paths.
Agents still need a local directory; a cloud build syncs or mounts it (out of scope). Loopback NFS
noted as a later option, not built.

## 14. Spike (Part 1, done) and benchmarks

Part 1 (haiku, 48 runs, 0.97 USD), Part 1b (sonnet, realistic fixtures, README and layout
ablations, 45 runs, 3.92 USD), Part 1c (sonnet proof flows, 12 runs, 0.94 USD). Spend 5.83 of the
6 USD spike cap; spike cap closed. Tables and every run: `P643-spike-results.md`.

Findings that shape this plan:

- Files-only: NO-GO on every flow. Remainder after README and layout fixes is call structure (2 to
  3 extra turns: read, write, submit), not text size.
- Files win: F4 orchestrator chain reads (cost 0.72 to 0.79, context 0.87 to 0.88).
- Files tie: F1 handover (cost 1.08 to 1.12 inconclusive, context parity).
- Tools win: F2 and F3 proofs (1.18 to 1.40), F6 child creation (1.36 to 1.40), F5 wait (1.17 to
  1.30, one sonnet run). The user kept proofs as files anyway (Decisions); F2 and F3 are reported.
- Lean README (17 lines) beat full README plus `AGENTS.md` by 0.003 to 0.008 USD per run and lost
  no pass rate; per-file history beat one `handovers.md` by 44 percent on F4.
- Gaps: forgotten submit (1 of 3 haiku F3), missing `reason` retry (2 of 3 FP F3), Bash discovery
  (2 to 5 calls), `AGENTS.md` second read, failed calls before ToolSearch (T F1 3 of 3, FF and FC
  2 of 3), `proof_list` without step data, proof format discovery (FP read README plus
  `AGENTS.md`). Each has a task in section 16.

### 14.1 Benchmarks through the build

- Harness: `apps/kira-space/tests/claude/details_bench_test.go` (`TestDetailsBench`), nested
  module, opt-in (`KIRA_BENCH=run`), `bun run test:claude:details`; never in hooks or CI. Fixtures
  in `apps/kira-space/tests/claude/testdata/details-bench/` (Part 1b fixtures copied: realistic
  handovers, notes, F3 proof).
- Arm T: frozen old tools. Part 2 freezes today's tool list (names, descriptions, schemas) to
  `testdata/details-bench/tools-baseline.json` before any tool changes, and arm T serves it through
  the spike's stub backends, so it stays runnable after removals.
- Arm R: real implementation: real `ade.TaskBoard`, `agentmcp.Server`, task folder, prompts
  composed by Kira's real `composePrompt` (with `headless.space.files`), as `orchestrator_test.go`
  does. No stub. Same flow prompt core and oracles as Part 1b. For F2 and F3 arm R is the files
  arm in the spike's FA/FP shape: proof path in the prompt, README `Proofs:` index and example
  block, single-file `proof.md` with inline data, `submit` (reason on F3).
- Model sonnet, 3 reps per flow and arm, 0.5 USD per-run cap, runs stop when a round's budget
  (below) is spent. Results written to `KIRA_BENCH_OUT` (required; the test skips without it), never
  to a default path under `docs/`.
- Metrics: as Part 1b (cost, final context, turns, pass, failed edit-path calls plus retries, tool
  calls, reads, ToolSearch).
- Gate per flow, sonnet medians, R over T: cost at most 1.2; final context at most 1.0; pass count
  not lower; failed edit-path calls plus retries (mean) not higher; turns reported only. A metric
  whose median gap is below the larger run-to-run range is marked inconclusive; an inconclusive
  fail gets an Opus review of the raw runs before the verdict. Binding for F1, F4, F5, F6.
- F2 and F3: same metrics, reported, not blocking (user chose files regardless). Expected cost 1.2
  to 1.3 times T (spike FP 1.31 on F2; FA 1.18, FP 1.24 on F3), context near 1.0. Pass count lower
  than T still goes back to the Part 4 implementer (correctness, not cost). Diagnostics reported:
  README reads, YAML or validation retries, forgotten submits, "has not been read" errors.
- Rounds (minimal):

  | After | Flows | Runs | Estimate USD |
  |---|---|---|---|
  | Part 3 | F1 | 6 | 0.47 (T 0.074, R about 0.08 per run) |
  | Part 4 | F2, F3 | 12 | 0.89 (T 0.064 to 0.067; R files arm 0.084 F2, 0.083 F3) |
  | Part 5 | F4, F5, F6 | 18 | 1.26 (F4 0.098 and 0.071; F5 0.057 and about 0.06; F6 0.067 and about 0.067) |
  | Final (end of Part 5, after both streams land) | F1 to F6 | 36 | 2.62 |

  Total Parts 3 to 5 rounds 2.62 USD; with the final 5.24 USD. Reserve for one Part rerun after a
  miss: 1.26 USD. Asked cap: 6.5 USD (question QN1), unchanged. Estimates are Part 1b and 1c
  medians; R is assumed near T where R uses the same tool shape (F5 wait, F6 `create_task`), near FL
  where it reads files (F1, F4) and at the measured files arms for F2 (FP 0.0837) and F3 (FA and FP
  0.079 to 0.085). F5 under option (d) has no measured arm; estimate kept at about 0.06.
- Kira-side cost, final, deterministic Go tests outside the `claude` module: submit latency (one
  changed file in a 500-file folder, p95 at most 200 ms); stat on task open, 500 files, at most 5
  ms; `state.yaml` rewrite at most 5 ms, at most one write per 150 ms window under a 100-change
  lease storm; task-folder disk size reported against `kira.db` size before migration; migration
  time on a generated large fixture (2,000 tasks, 50,000 handovers, 5,000 notes, 500 proofs with
  images, in the migration flow test's `testdata/`), bound 2 minutes. Part 2 may tighten a bound
  with a measured reason, never loosen it silently.
- Failure: a Part missing a metric goes back to its implementer (same phase, same number) and
  reruns its round. A final miss goes to the user with the numbers.
- Results: each round appends a section ("Part 3", "Part 4", "Part 5", "Final", "Kira-side") to
  `P643-spike-results.md`. The P643 `SPEC.md` Result carries per-flow final ratios and verdicts.

## 15. Tests per Part (IPC split)

Each feature Part ships one Go flow test and one UI spec on a shared contract. Unit tests only for
interacting rules.

- Part 2: no user feature. Unit tests: classify matrix (writer, kind, owner, collision, reason
  needed) and submit diff (link entries, proof step ids, inline YAML to JSON with integer precision
  and key order). Store adapter conformance: traversal, symlink, hard link, FIFO, caps, atomic
  replace. Bench: stat of a 500-file folder under 5 ms. Migration framework flow test with a fake
  exporter (crash, resume, verify fail). README generator: one golden per role (run task,
  orchestrator) asserting at most 25 lines (Part 4 raises the cap to 35 with the proof example).
- Part 3: flow `adeflow` `TestDetailsFiles` (contract `ade-details-files.json`): user note writes a
  file; agent draft plus submit accepted and on the board; agent edits a user note without reason:
  `invalid` naming `reason` with example; with reason: pending, approve updates cache, deny restores
  file; collision prompt; hand edit seen on open, Accept and Discard; invalid file with line;
  traversal, symlink and cap refused; `finish_step` implicit submit and refusal without draft;
  read-only file restored; `submit {finished}` raises the move prompt; links own entry silent,
  other author's entry pending; `details.edit` item listed for the phone; migration of handovers,
  notes and links with verify; composed step prompt names the folder, draft path, tool load line
  and ends with the submit reminder. UI `ade-v2-details-files.spec.ts` on the same contract:
  banner, diff dialog, approvals item with reason, problem strip, folder bar; phone approval spec
  case for `details.edit`. Update listed adeflow tests and UI specs; real `claude`
  `TestAdeReportArtifacts` becomes `TestAdeDetailsSubmit`; `TestAdeManualSession` writes a draft.
- Part 4: flow `proofflow` `TestProofFiles` (contract `proof-files.json`): agent-written new
  `proof.md` (inline data, image and `<id>.json` side files) plus submit is accepted, raises the
  ready prompt, links the proof and the reply carries name, step ids, views and counts; over-cap
  inline data `invalid` with the move-to-`<id>.json` fix; `proof.md` naming a missing image:
  whole unit `invalid`, files untouched, image written later plus submit accepts the proof as new;
  unreferenced side file `invalid`; bad folder name `invalid` with the suggested slug; duplicate
  name `invalid` naming the other folder; YAML error answers file:line and the fix hint; second
  submit with nothing changed `unchanged`; edit by step id without reason `invalid`, with reason
  pending, approve keeps view state by step id, deny restores files and image; resubmit of a
  pending edit replaces its approval item; user delete removes folder and rows; README and prompt
  `Proofs:` lines and the README example block present; migration of linked
  sessions and images (`.jpg`), taskless sessions deleted with log lines. Removal assertions:
  kira-studio lists no proof tool for any client, no `kira-space` grant lists a `proof` tool,
  Window menu has no "Validation proof",
  `OpenWindow` without (task id, slug) refused, no unattached or attach call left. Image unit table
  (`proof/image_test.go`): one valid file per format (PNG, JPEG, WebP lossy and lossless, SVG);
  extension mismatch; GIF and `.svgz` refused; APNG and animated WebP refused; size, side and area
  caps; damaged data. Unit table for the proof unit decision and the YAML fix-hint mapping (one
  row per hint class, unit all-or-nothing with missing, extra and invalid side files): interacting
  rules, so they earn a unit test. Hostile SVG fixtures in `internal/proof/testdata/svg/`, one per rule
  (`<script>`, `<foreignObject>`, `iframe`/`object`/`embed`, `<animate>`, `<set>`, `onload`,
  `javascript:` href, external `href`, external `xlink:href` on `<use>`, nested SVG data URL,
  `@import`, external `url()` in `style`, DOCTYPE with entity, not well-formed, no size, side over
  16384, over 2 MiB, depth over 64), each asserting element or attribute and line; one allowed SVG
  with `#fragment` refs, `url(#g)` gradient and embedded `data:image/png`. UI proof window spec (one
  proof, no history, path header, image per format incl. SVG through `<img>` data URL, no inline
  `<svg>`) and `ade-v2-proof-chain.spec.ts` on the contract. Real `claude`
  `TestProofMcpThroughClaude`, `TestProofChartThroughClaude` rewritten: the agent writes
  `proof.md` (and a chart step) and calls `submit`.
- Part 5: flow `adeflow` `TestDetailsStateFile` (contract `ade-state-file.json`): lease, op, setup,
  stage and pending changes land in `state.yaml` within 250 ms, coalesced (count writes), byte-equal
  skip; `branches` all four actions, `lease` both actions, `cancel_operation`; setup wait per the
  9.2 choice (`taskinfowait_test.go` rewritten for it: push delivered to a session and the failure
  text on a failed setup for (d), wait-only reply for (a), request reply after setup for (b)); removed tools absent from every grant
  (`TestOrchestratorToolGrants`, agentmcp `TestGrantScopes`, `toolnames_test.go`); orchestrator
  reads children through `chain` and README `Children:` lines; `create_task` reply `next` names the
  child's `handovers/` path. UI spec: folder bar lists `state.yaml` read-only with its `rev`, which
  rises after a lease change; leases and branch request specs still pass. Real `claude`
  `TestAdeOrchestrator`, `TestAdeHeadlessRun`, branch request rows rerun.
- Every Part: matching real `claude` rows of DEV_ENVIRONMENT ("ADE runs and sessions", "Agent
  sandbox") once near Part end. Parts 3, 4, 5: benchmark round (14.1), gate binding except F2 and F3
(reported).

## 16. Parts

Numbers stay P643 (`P643 Part 1` to `Part 5`). Order: Part 1 (done), Part 2, Part 3, then Parts 4
and 5 as two streams, then the final benchmark. Parts 3, 4, 5 count done only when their round
passes (F2 and F3 reported, not gated).

| Part | Content | Main files |
|---|---|---|
| 1 Spike | Done: Part 1, 1b, 1c; NO-GO for files-only; user chose narrowed fallback (a) | `tests/claude/details_spike*_test.go`, `testdata/details-spike/**`, `details-spike-1b/**`, `plans/P643-spike-results.md` |
| 2 Foundation | store seam, `task_files`, intents, migration framework, submit core (walk, classify, diff, apply, restore, reason rule), proof validators export plus inline YAML to JSON, README generator, header texts, task folder grant (add-dir, sandbox allow and denies, deny rules), harness hygiene, bench harness and frozen baseline | new `internal/taskfiles/**`, `internal/taskfiles/store/**`, `storage/migrations/0101_p643_task_files.sql`, `storage/repos/taskfiles.go`, `internal/proof/validate.go`, `ade/sandbox.go`, `ade/sandbox_tui.go`, `ade/launches.go`, `appwire/wire.go`, `tests/claude/details_spike1b_test.go`, new `tests/claude/details_bench_test.go`, `tests/claude/testdata/details-bench/**`, `package.json` (`test:claude:details`), `docs/DEV_ENVIRONMENT.md` row, `docs/ARCHITECTURE.md` "Task files (P643)" |
| 3 Notes, handovers, links, submit, prompts | file writes for every UI action, caches, stage move rename, `submit` tool (reason, reply wording), `finish_step` change, `handover` and `report_artifacts` removed (kind proof attach stays until Part 4 removes it), `details.edit` approvals desktop and phone, disk banner, migration exporters, panel forward paths, `headless.space.files` fragment and `harness.ToolLoader`, `HeadlessSpaceInstructions`, child brief written as a handed file, README content lines | `ade/handover.go`, `ade/links.go`, `ade/artifacts.go`, `ade/orchestrator.go`, `ade/taskcreate.go` (brief file), `ade/panelforward.go`, `ade/vars.go`, new `ade/details.go`, `agentmcp/{handover,artifacts,mcp,space,tools}.go`, new `agentmcp/submit.go`, `internal/harness/mcp.go`, `internal/harness/claude/mcp.go`, `llmprompts/registry.go`, `bridge/adetask.go`, `adewire/wire.go`, `mobileweb/approvals.go`, `frontend/src/ade/v2/details/**`, `request/kindDefs.ts`, mobile approval view, tests, bench F1 |
| 4 Proofs (Stream A) | proofs as agent-written files plus `submit` (7.5): `proof.md` format, submit proof unit (all or nothing, side files, slug and name rules, YAML fix hints, proof reply summary), `Request.Tool` and `Decide` `tool` argument removed, image formats incl. SVG, window single and task-only, retention removed, kira-studio proof tools removed, Q10 entry points removed, proof migration and image rewrite, Q16 deletion, README example block, line 4 and `Proofs:` line, prompt `Proofs:` line, proof header text | `internal/proof/**`, `storage/repos/proof.go`, `storage/migrations/0102_p643_proof_step_key.sql`, `dbmcp/proof_tools.go` (removed), `dbmcp/server.go`, `ade/proof.go`, new `internal/taskfiles/proofunit.go`, `internal/taskfiles/{submit,decide,classify,readme,headers}.go`, `bridge/proof.go`, `bridge/dbmcp.go`, `bridge/adetask.go`, `appshell/menu.go`, `appwire/menuactions.go`, `frontend/src/studio/proof/**`, `details/AdeProofEntry.vue`, `AdeProofAttachMenu.vue` (removed), `packages/shared/domain/proof.ts`, `go.mod` (`golang.org/x/image`), tests, bench F2 F3 |
| 5 State file and tools (Stream B) | `state.yaml`, `task.yaml` writer, `branches`, `lease`, `cancel_operation`, removals (`run_outcome`, `lease_status`, `operations`, `list_tasks`, `task_chain`, old branch and lease tools), setup wait open question (9.2: first a `codegraph_explore` check whether an agent ever waits for a setup it did not trigger and whether the lease channel reaches headless runs; default (d) push "worktree ready" and remove `task_info` if the check passes, else (b) where safe, fallback (a) wait-only `task_info`; F5 re-benchmarked either way), `state.yaml` header setup sentence, orchestrator chain reads, children add-dir, `create_task` `next` text, README `Children:` line, `HeadlessSpaceInstructions` `state.yaml` sentence | new `ade/statefile.go`, `agentmcp/{space,leases,orchestrator,outcomes,tools}.go`, new `agentmcp/branches.go`, `llmprompts/registry.go`, `ade/leases.go`, `ade/orchestrator.go`, `ade/taskcreate.go` (`next` text), `ade/launches.go` (children add-dir), `ade/agenttools.go` and `ade/setup.go` (setup wait per 9.2), `details/AdeDetailsFolderBar.vue`, tests, bench F4 F5 F6 |

Part 2 tasks for the spike gaps and hygiene:

- Gap 8: `TestDetailsSpike1bReport` writes only to `KIRA_SPIKE1B_OUT` and skips without it; no
  default path. `TestDetailsSpikeToolBytes` gets the same rule (same defect). File
  `tests/claude/details_spike1b_test.go`.
- Gap 7: bench harness (14.1), frozen `tools-baseline.json`, fixtures under
  `testdata/details-bench/`, `KIRA_BENCH_OUT` required.
- Gap 4: README generator `internal/taskfiles/readme.go` (lines of 7.6 the Part owns; Parts 3, 4,
  5 add their lines), golden tests with the 25-line cap (35 from Part 4); no `AGENTS.md` writer for
  tasks.
- Gaps 1 and 2 (core): header texts in `internal/taskfiles/headers.go`; reason rule in
  `internal/taskfiles/decide.go` with the error text builder.

Part 3 tasks for the spike gaps:

- Gap 1: `submit` `reason` arg, description and error text (`agentmcp/submit.go`,
  `llmprompts/registry.go`); flow test case without and with reason.
- Gap 2: README first line, `links.yaml` header, prompt last line, reply count line; flow test
  asserts prompt ending and reply first line.
- Gap 3: `headless.space.files` names folder and draft path; latest handover inlined (exists);
  flow test asserts the prompt text.
- Gap 5: `harness.ToolLoader` plus Claude `LoadHint`; prompt tool line from the grant's tool set;
  `toolnames_test.go` case; registry descriptions kept short (checkprompts length check per new
  entry, limit 400 chars).

Part 4 tasks for the spike gaps:

- Gap 6: `proof_list` removed; the submit reply's proof summary carries name, step ids, views and
  counts; README and prompt `Proofs:` lines; flow test asserts all three.
- Gap 1: `reason` required on an accepted proof's edit, with the 7.1 error text naming `submit`;
  `proof.md` header and README line 5 name it.
- Format gap (no tool): README example block and line 4 (`taskfiles/readme.go`; Part 2's line 4
  still names the proof tool), proof header without "Or call the proof tool." (`headers.go`),
  YAML fix hints, slug and name errors; golden README test keeps the 35-line cap.

Streams after Part 3 (cap 3; two used): Stream A = Part 4, Stream B = Part 5, each its own
worktree off the Part 3 end commit.

No ordering dependency: Part 4 touches proof storage, the submit proof unit, the proof window and
proof migration; Part 5 touches the state mirror, branch and lease tool shapes, orchestrator reads.
`state.yaml` `pending` reads `task_files.review` (Part 2, generic), not proof tables. Neither
stream reads a file the other creates.

File ownership:

| File or area | Owner |
|---|---|
| `internal/proof/**`, `storage/repos/proof.go`, `dbmcp/**` proof parts, `bridge/proof.go`, `bridge/dbmcp.go`, `appshell/menu.go`, `appwire/menuactions.go`, `frontend/src/studio/proof/**`, `details/AdeProofEntry.vue`, `AdeProofAttachMenu.vue`, `packages/shared/domain/proof.ts`, new `internal/taskfiles/proofunit.go`, `internal/taskfiles/{submit,decide}.go`, `ade/proof.go`, `go.mod`, migration `0102` | A |
| new `ade/statefile.go`, new `agentmcp/branches.go`, `agentmcp/{space,leases,orchestrator,outcomes,tools,mcp}.go`, `ade/leases.go`, `ade/agenttools.go`, `ade/setup.go`, `ade/orchestrator.go`, `ade/taskcreate.go`, `details/AdeDetailsFolderBar.vue` | B |
| shared, listed below | A and B |

Shared files, additive hunks:

| Shared file | Stream A edit | Stream B edit |
|---|---|---|
| `internal/llmprompts/registry.go` | `DbmcpToolProofSave`, `DbmcpToolProofList` removed | `branches`, `lease`, `cancel_operation` texts; removed entries; setup-wait text per 9.2 (`task_info` description or its removal); `HeadlessSpaceInstructions` sentence |
| `internal/taskfiles/classify.go` | proof-folder hint for a bad slug or a too-deep path | none (Kira-owned patterns exist from Part 2) |
| `internal/taskfiles/readme.go` | line 4, example block, `Proofs:` line, cap 35 | `Children:` line |
| `internal/taskfiles/headers.go` | `headerProof` without the proof tool | `headerState` setup-wait sentence |
| `ade/vars.go` | prompt `Proofs:` line input | none |
| `ade/launches.go` | none | children add-dir |
| `bridge/adetask.go` | proof choices and link calls removed | none |
| `tests/claude/details_bench_test.go` | F2, F3 flows | F4, F5, F6 flows |
| `docs/ARCHITECTURE.md` | "Validation proof" section | ADE agent tools, leases, orchestrator sections |

Landing: rebase each stream onto the chapter branch; resolve conflicts only in the files above;
stop on any other. Then the final benchmark (14.1).

Every Part updates ARCHITECTURE.md (Part 2 adds "Task files (P643)"; later Parts extend it) and
removes stale text (handover tool, `report_artifacts`, proof retention, `task_chain`, standalone
proofs).

## 17. Risks

- Agent writes files and never submits: `finish_step` implicit submit; prompt, README and header
  say submit; session end leaves the banner. No silent loss, no silent accept.
- Approval item without `reason`: one retry turn; mitigated by description, header, README and the
  one-line error with an example.
- Unsandboxed agent writes another task's folder: never auto-accepted; that task's banner shows it.
- ToolSearch hint wording depends on Claude Code's `select:` syntax: lives in a user-editable
  registry fragment and the Claude provider only; a wrong hint costs one failed call, as today.
- Change size: four build Parts, two streams; columns kept as caches limit blast radius.
- File then DB order: intents journal, start-up reconcile, atomic rename.
- Path traversal, symlinks, special files, size caps: checked per file at submit and store.
- Seatbelt profile and `--add-dir` fixed at launch: children created later unreadable under an
  enforced sandbox; previews and `create_task` reply cover it.
- Migration: per task, resumable, verify before rename, no backup (Q11); image rewrite only after
  verify; Part 4 test checks decoded bytes against the payload hash before rewrite.
- Proofs gone for DB MCP clients outside a task (Q10): recorded in ARCHITECTURE.
- Hostile SVG: validator rejects; `<img>` data URL rendering is the second guard.
- Large image data URLs over IPC: one step on demand, as today.
- New dependency `golang.org/x/image/webp` (BSD-3), only for WebP.
- Benchmarks cost tokens: rounds are flow subsets, per-run cap 0.5 USD, opt-in, never in CI.
- Proofs without a tool cost more turns (F2 and F3 about 1.2 to 1.3 times T): accepted by the user;
  reported each round so a regression past the spike numbers is visible.
- Proof built across two submits (accepted without an image step, image added later) becomes an
  edit needing reason and approval: README example line says images first, then `proof.md`, then
  one submit; the unit's all-or-nothing rule keeps an invalid first attempt from being accepted.
- YAML mistakes in hand-written `proof.md` (tabs, unquoted colons, indentation): one retry turn;
  file:line plus a one-line fix hint per error class; flow-style example in the README avoids deep
  indentation.
- README example block adds about 200 tokens to every README read (F1 context gate 1.0 is at
  parity): fallback moves it to `proofs/EXAMPLE.md` (7.5).
- Setup wait push (option d) cannot reach a headless run that calls no Kira tool or whose turn
  ended (9.2 findings): Part 5's check decides; fallback (a) keeps a wait-only tool.

## 18. Questions

Answered (kept for the record): Q1 no watcher; Q2 superseded by the narrowed design
(`create_task` stays, no drafts); Q3 folders by id; Q6 blocking mirrors old tools; Q9 merged
`branches` and `lease` (setup wait reopened for Part 5, 9.2); Q10 no proof outside a task; Q11 no backup; Q12 spike
models (done); Q15 phone; Q16 delete taskless proof rows after a log line; Q17 image formats.

Plan recommendations in force (user accepted "all other questions: plan's recommendation"):

- Q4 `state.yaml` holds absolute worktree paths; content files stay relative. Yes.
- Q5 Agent edits a user note or handed handover: prompt with diff (now with reason). Yes.
- Q7 `finish_step` ok with no draft while the route continues: refuse with the path. Yes.
- Q8 Child finish mirror rows derive from the child's file. Yes.
- Q13 Step prompts keep inlining incoming handover and notes beside the folder line. Yes.
- Q14 Gate: superseded by the benchmark gate in Decisions.

New, for the user:

- QN1 Benchmark spend for Parts 3 to 5 and the final: estimate 2.62 USD for the three Part rounds
  plus 2.62 USD for the final, 5.24 USD total (section 14.1; F2 and F3 now run the files arm).
  Recommend: approve a 6.5 USD cap (1.26 USD reserve, one Part rerun), per-run cap 0.5 USD.
  Alternative: final reuses each Part's T runs (saves about 1.2 USD) at the cost of comparing
  across days.
- QN2 superseded: no `proof` tool (user). Proofs work in every task folder through `submit`, which
  every Space grant has.

Open for a Part planner, not the user:

- Setup wait (Part 5): options (a) to (d), findings and default in 9.2.
