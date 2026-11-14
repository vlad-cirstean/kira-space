# v0.3.14: follow-ups from v0.3.13 and UI fixes

Goal: close gaps v0.3.13 found (agent config port journey, MySQL `DELIMITER`, mobile approval of
agent requests, collection file with one bad item) and ship five UI fixes. Each new feature follows
the IPC-boundary test split (flow test plus UI spec). First `P` is P634.

| P | Phase | Status |
|---|---|---|
| P634 | Agent config import/export journey (J-HAR-002): extend `harnessflow` AgentPort tests and `ade-agent-port` UI spec | Todo |
| P635 | MySQL console `DELIMITER` directive (F-A12) | Todo |
| P636 | Mobile approval of agent requests (J-ADE-072, F-B8) | Todo |
| P637 | Collection file with a bad item: valid items load, bad ones reported (F-A15, J-COLL-003) | Todo |
| P638 | Toasts at top right, placed like prompts | Todo |
| P639 | API module: dialog when closing an unsaved tab, new or modified | Todo |
| P640 | ADE task: own Jira and PR links first in their group | Todo |
| P641 | ADE task list: today background, day start/end left bar, today bar differs | Todo |
| P642 | Details tab on disk: notes, handovers, links, PR link and proofs (with images) as files, source of truth; design plan first, user approves it before the build | Todo |
| P643 | Chapter non-UI full run, 1 code review round | Todo |
| P644 | Close chapter | Todo |

## Scope

### P634 Agent config import/export journey

`internal/agentport` exists (Claude and OpenCode providers, preview with diff, backup on export, MCP
registration) with `TestAgentPortClaude`, `TestAgentPortOpenCode` and `ade-agent-port.spec.ts`.
v0.3.13 stream B wrongly dropped J-HAR-002 as "feature absent". Cover full journey from
`docs/v0.3.13/plans/journeys-ade.md` (read from git history): import from user Claude config with
hooks, MCP servers with secrets and statusline; masked preview diff; apply; export back after edits;
backups listed; undo via backup. Oracle: backup files hold original bytes; exported config parses
and equals settings; secrets masked in preview and never written to `settings.yaml` in clear. Fix
every bug found.

### P635 MySQL `DELIMITER`

Console statement splitter honours `DELIMITER <token>` so trigger, procedure and function scripts
and mysqldump output run. Splitter has several interacting rules (quotes, comments, delimiter
change mid-script): unit tests allowed. Flow plus UI halves: script with trigger body runs and the
trigger exists afterwards.

### P636 Mobile approval of agent requests

Paired phone lists pending agent approvals and approves or denies; desktop state follows. Reuse the
existing mobile pairing and approval model; design in the phase plan. Flow half: approval via
mobile route unblocks the agent step, deny blocks it, expired device token refused. UI half: phone
view of the approval list and the desktop reflecting the result. Covers J-ADE-072 (parent with
children, leases, notifications, mobile approval).

### P637 Bad item in a collection file

Validation of a hand-edited collection file is whole-file today: one bad item serves no rows on
first load. Load valid items, skip bad ones, show each skipped item with reason. Remove the entry
from ARCHITECTURE.md Known open items. Flow plus UI halves.

### P638 Toasts top right

Toasts anchor top right, same placement as prompts. No overlap between a prompt and a toast: stack
them. One shadcn-vue toast primitive, no new hand-rolled one. Update affected UI specs and
screenshots.

### P639 API tab close dialog

Closing an API-module tab asks to confirm when the tab is brand new and unsaved, and when it holds
unsaved changes to a saved request. Dialog offers Save, Discard, Cancel. Tab closes only after Save
or Discard. Dialog is shadcn-vue. Flow half: save then close persists, discard leaves the saved file
unchanged. UI spec for both cases.

### P640 Own links first

On an ADE task, the task's own Jira and PR links sort first inside their group, before links
collected from children or other sources. Update link strip and any sorted list sharing the order.

### P641 Task list day styling

In the ADE task list: replace the today background (current colour is wrong; pick from theme
tokens, light and dark), add a vertical left bar spanning each day's rows to show where the day
starts and ends, today's bar differs slightly from other days'. Visual spec and screenshot refresh.

### P642 Details tab on disk

Whole Details tab content moves out of SQLite into files under the home folder: initial notes,
handovers, links including the PR link, and proofs with their images (image files beside the
markdown, not base64 rows). Files are the source of truth for all content. State stays in SQLite and is never driven by a file edit: approvals, stages, branches (git and worktree state), leases, runs, status, workflow, plan placement.
Planner chooses the file split (several files per task; one read-only state file, header says changes go only over MCP, Kira rewrites it atomically: branches, worktree and setup state, leases, operations, status, step, task chain). Tasks are folders; an orchestrator creates a child by writing a folder. One sync tool (agent says files are edited): Kira reads, validates, diffs against its SQLite copy, raises the right prompt (planner decides which edits prompt and which only notify) and returns accepted or rejected per file; `finish_step` may submit implicitly; a watcher covers hand edits only. Tools left: sync, finish_step, run_outcome, declare_repos, request_branches, rename_branch, change_branch, acquire_lease, release_lease, operations. `task_info`, `lease_status`, `list_tasks`, `task_chain` become file reads. Rule: each field has one owner. Content owned by files; SQLite keeps id, path, content hash, review status and a rebuildable cache for lists, never an independent write. State owned by SQLite; the state file is a one-way generated mirror. Build order inside the phase: content files and sync tool, then state file, then tasks as folders only if the first two work. Tasks stay Kira-written read-only files (no agent-written task folders); `create_task` stays a tool with its approval. Agent tools end at about 6: submit, finish_step (with run outcome), branches (request, rename, change, declare), lease, cancel_operation, create_task. Kira writes a README per task folder (file meanings, structure, read-only files, limits, submit behaviour) and a header in each file; step and session prompts point to the folder. Performance: state file holds coarse state only, writes atomic and coalesced (100 to 250 ms), size and mtime before hash, no global watcher (submit is the trigger; watch open task folders for hand edits only), UI reads SQLite never files, size caps enforced on submit. Spike first: real-claude comparison of tokens and turns, tools versus files, on one handover and proof task. No FUSE (macOS only: macFUSE not fully open source, FUSE-T license unverified); loopback NFS mount noted as a later option. Opus plan decides, then user approves before any build: format and location, one-time migration of
existing rows, edit protection (agents read freely; Kira lock versus ask-to-edit), proof revisions,
pending edit accept/reject, view state and retention of 30 (on disk, or SQLite keeps review state
only), MCP tool changes (`handover` removed; `finish_step` keeps its end-of-step signal, summary text moves to the file; `report_artifacts`, `proof_save`, `proof_list` removed, agent writes the files and Kira validates on read; `task_info` drops notes, links and handovers; branch, lease, operations and orchestrator tools stay), behaviour when a file is edited by hand while the app runs. Flow plus UI
halves per the split.

## Results
