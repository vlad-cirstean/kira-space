# v0.3.21

Chapter v0.3.21, branch `chapter/v0.3.21`, tag `v0.3.21`. First phase `P699` (highest history phase: `P698`).

| # | Phase | Status |
|---|-------|--------|
| P699 | File watching: one shared seam, FSEvents on macOS | Done |
| P700 | File interactions: one shared file service | Done |
| P701 | Restore after restart: dialog to resume scripts, Docker and agents | Done |
| P702 | Agents remove worktrees; no prompt for worktree create or delete | Done |
| P703 | Edit prompts only for another agent's notes; the watcher raises them, no `submit` | Done |
| P704 | Task Files: open in its own window, off the main chrome | Done |
| P705 | Mobile app: show the status info it is missing | Done |
| P706 | Pull request icon: one glyph for every PR status | Done |

## P699

All file watching goes through one seam, and macOS uses FSEvents for every watcher. Today
`gitclient` has the seam (`watcher.go`, `watcher_fsevents_darwin.go`, `watcher_fsnotify.go`) while
`ade/folderwatch.go` and `ade/taskwatch.go` call `fsnotify` directly, so on macOS they pay one
descriptor per file (kqueue) and `taskwatch` caps at 8 live task watchers (ARCHITECTURE "Known open
items"). Deliverable: a shared watch package with one backend per platform (FSEvents on darwin with cgo,
`fsnotify` elsewhere and with cgo off), the events plus rescan contract the git seam already has,
realpath normalisation, dropped-event handling, and migration of `gitclient`, `folderwatch` and
`taskwatch` onto it. Remove the 8-watcher cap and its Known open items entry. A stream per watched
folder is enough for tasks (few files change at once); the plan checks the per-process stream limit and
decides stream sharing. Keep `fsnotify` for Linux. Tests: the backend contract suite runs against both
backends where the platform allows; plan states what only a Mac run can prove.

Result: `internal/fswatch` (module root, leaf) is the seam: `Watch(Options{Roots, MaxDepth, SkipDir, Latency})`,
caller-spelled events, scope verdicts, bounded output that overflows to one `Rescan`, and a shared `Debouncer`.
FSEvents backend on darwin with cgo, fsnotify elsewhere. gitclient, ade folder watch, ade task watch and
configfs migrated; direct library imports and `maxTaskWatches` deleted; guard G7 keeps it so. Decision: one
stream per watcher, no sharing (about 50 streams expected, far below reported limits). Darwin delta: events
under another worktree's own `worktrees/<name>/` dir no longer raise `refsChanged` (fsnotify never saw
them); worktree add and remove still do. Task watch Rescan pushes the board with no pass. macOS CI job now
runs the fswatch contract suite on both backends, configfs, `TestFolderWatch`, `TestDetailsWatch` (incl.
`TestDetailsWatchBeyondEight`) and a no-cgo vet of fswatch; timeout 25 min. The FSEvents backend could not
be built or run off a Mac: only PR CI proves it.

## P700

All file interactions go through one shared file service (a single type, not scattered `os` calls):
reads, atomic writes, moves, deletes, directory listing, hashing, path safety (symlink and traversal
checks), size caps and the watch seam from P699. Callers that touch files today (task files and Details,
storage repos, config sync, repo files, scriptruns, worktree and port import/export, proofs and charts)
move onto it, so there is one place for atomic write, permission and error semantics. Deliverable: audit
list in the plan (every direct `os`/`ioutil`/`filepath.Walk` use outside tests and the new service, with
keep or move and why: build tooling, harness and tests may stay), the service, migration in commit
groups, and a guard in `scripts/` or golangci config that rejects new direct file calls outside the
service. Order dependency: after P699 (the service owns the watcher API).

Result: `internal/fileservice` is the one file seam: rooted (`Contain` or `Refuse`, `PlainOnly`) and host-wide
Services; atomic write with file and dir fsync, perms, read caps, `*fs.PathError` errors, verified copy,
capped walk, hashing, watch over fswatch. pathsafe, fsmove and the configfs atomic helpers are folded and
deleted. 289 call sites in 87 files moved (repo files, config and config sync, task files and ADE,
import/export, proofs, transfers); 246 sites in 70 files stay allowlisted (the service, fswatch, harness,
bootstrap, build tooling, test tooling, host probes) by guard G8, which also folds G4. Fixed: blob and usage
writes had no fsync; no write fsynced its dir. Deviations: `configsync` uses `Host()` (its names are
regex-validated); proof shot files use `Host()` with sanitized names, not a rooted Service; the service
gained `Lstat`, `Resolve`, `ReadFile`, `WriteFile`, `HostPrivate`, `ValidName`, `Policy.Existing`. Behaviour:
host-wide Services refuse relative names; modes and dir perms match the old code (audited site by site
against the pre-P700 calls). 0700 dirs map to `HostPrivate` (ade task dir and task folder, mcpauth, agentmcp,
agentcfg, agentport backup writes, llmprompts, scripts workdir, adeflow backup copies, task blobs); 0600 temp files
to `TempFile`/`TempDir`; former `configfs.WriteAtomic` sites (adeflow journal, agentctx snapshot, agentusage)
use `Host()` so created parents stay 0755; `KeepPerm` only for codeworkspace edit and collection export.

## P701

After Kira restarts, show a dialog that offers to bring the previous session back at once. It lists what
was running when Kira stopped and each row has a checkbox the user can select or deselect:

- **Scripts:** Kira-started runs that were running (the P697 run records).
- **Docker:** the containers and compose projects Kira had running, shown only when Docker itself was
  restarted too (the daemon no longer holds them).
- **Agents:** sessions that were running. Each agent has two separate checkboxes: *Resume* (reopen the
  session) and *Continue* (after the session opens, pass it at the input a short continue message so it
  carries on from where it was). *Continue* is enabled only with *Resume* on.

One primary button restores the selection. Follow P696 (no footer Cancel beside the X; the X dismisses
and restores nothing). Deliverable: what counts as "was running" persisted at quit and crash, how the
dialog learns it (TanStack Query over a bridge call), restore actions for each kind reusing existing
run, Docker and agent-resume paths, the continue message in the prompt registry (one very short
sentence), and UI plus flow tests split at the IPC boundary. Interaction with P697 required services:
restoring scripts satisfies the required-services gate.

**Result.** Launch dialog lists scripts, Docker (only when pending) and agents; Resume per row, Continue
needs Resume (required for review runs); X restores nothing. Migration 0103 marks TUI rows at quit
(`Tracker.MarkQuit`, working flag) and crash. `sessionrestore.Service` joins the three offers (`Claim`
once per process inside a 2 min window, scripts first, Docker and agents after, each item alone).
Prompt `restore.continue.line`. Shared `RestoreDialog` gained groups and an option; row builders moved
to `automations/restore/rows.ts` and `docker-ui/src/restoreRows.ts`. Flows: adeflow
`TestSessionRestore*`, dockerflow `TestSessionRestoreDocker`; UI `session-restore.spec.ts`; real
`claude` `TestAdeRestoreRunResumeOnly` passes, `TestAdeRestoreSessionContinue` blocked by the baseline
TUI `no SessionStart hook event` failure. Deviations: no `unsupported` Continue state, `Claim` takes no
window key, takeover reuses `opTakeOver`. Visual baselines unchanged.

## P702

Agents can remove worktrees (a Kira MCP tool, through the same code path as the UI), and creating or
deleting a worktree no longer shows an approval prompt, whether the user or an agent starts it. Remove
the prompts and their copy and specs. Keep what the prompt protected: the plan lists any real safety
(dirty tree, unpushed commits, the worktree in use by a live session) and decides which checks stay as
errors or inline refusals, without a dialog. MCP surface, approval rules (`internal/agentmcp`), the
ops panel entry, docs and tests (flow plus UI) are in scope. Never delete a worktree that holds
uncommitted work without an explicit user path; the plan names that path.

Result: agent `branch.request` creates new branches, named-row worktrees and free existing branches with no
approval (`createAutoSlots`; only adopt still asks, as do rename and change). New `state` action
`worktree.remove {repo, branch}` runs the UI's `removeRowBranch` path; refuses dirty, busy, locked,
open-elsewhere, main-checkout and the caller's own cwd; the git branch stays. A headless run holds its own
branch, so it cannot remove that worktree (real `claude` confirms the refusal). User removal has no dialog:
clean rows remove at once (unmerged commits no longer confirm); a dirty row offers "Discard N changes and
remove", the Change branch form an inline discard button, the Git module worktree row an inline "Discard
changes and remove". Archive keeps its confirm. `useDiscardGuard`, `TaskBranchRisk`, the git-ui remove
dialog and their keys deleted. Op `agentWorktreeRemove`. Flows `TestAgentBranchCreateNoPrompt`,
`TestAgentWorktreeRemove`, `TestUserWorktreeRemoveNoConfirm` (contract `ade-worktree-remove`) with
`ade-v2-worktree-remove.spec.ts`; `TestWorktreeRemoveInline` (contract `git-worktree-remove`) with
`git-worktree-remove.spec.ts`; `TestBranchRequest` and neighbours migrated; real `claude`
`TestAdeBranchRequest` reworked (create, held rename, refused own-worktree remove). No flow covers a session
of another task holding the worktree: a same-task open-session refusal stands in. Baselines to re-record at
chapter end: contracts `ade-worktree-remove`, `git-worktree-remove`, `approvals-space`, `ade-branch-request`
and `ade-branch-adopt` (stale keys pruned), llmprompts goldens and `budget.tsv`, UI visual baselines.

## P703

Do not show an edit prompt unless the edit touches another agent's notes. Task-file edits by an agent
to its own notes, handovers and proofs, and to files nobody else authored, apply without a prompt;
only an edit that changes a note, handover, proof or link written by another agent still asks. Builds
on the P695 rules (latest edit by any author needs no approval; the watcher defers ask-first edits
to the disk banner): the plan reconciles the three rule sets into one table (author, age, kind, who
edited) with a flow test per row. Update the submit reason prompt and the README line if the rule
shifts, and the approvals copy.

Remove the `submit` tool: the watcher already sees every task-file change, so it raises the approval
itself when an edit touches another agent's notes, with nothing for the agent to call. Drop the tool,
its registry prompts and reason argument, the README and prompt lines that mention it (including the
P694 files prompt's "Call submit" wording), the proof and link file header text, the `fileErrors`
guidance that points at it, and the flow, UI and real `claude` tests that call it. The step finish flush
stays. The plan decides where the approval row gets its explanation (a short `reason:` line in the file,
or none, since the diff shows the change) and states how a watcher pass without a live agent (an outside
edit by the user) never raises an approval.

Result: An agent edit asks only for another agent's content. Author comes from handover and artifact
`source` plus `session_id`, and from the proof link `source` plus `kira_session_id` (`ProofCache.Create`
now writes both; legacy rows with no session id count as another agent). Asks: older note or handover
edit, any note, handover, link or accepted-proof delete, link edit, older accepted-proof edit, collision
with another agent's acceptance after the caller's last sync. User, pass, Kira, slot, own, create and
latest edits apply. One live grant: the watcher pass runs as that agent and raises `details.edit` itself
(`state.yaml` `pending` kind `details.edit`). No grant: banner only. Several: board push only, the finish
flush decides. Removed: `submit` tool, `reason` (args, payload, UI row, `ProofEdit.Reason`), `CleanReason`,
`ReasonError`, `StatusDeferred`, `Request.Watch`, `HeadlessToolSubmit`. README, headers, suffix,
instructions and the files load hint reworded; P702's `worktree.remove` text kept. Flow
`TestDetailsEditRules` plus UI `ade-v2-details-rules.spec.ts` share contract `ade-details-rules`. The flow
runs the rows that reach the real stores (user and own note apply, another agent's older note and link
wait, outside save is banner only); the full rule table (R1 to R16, collision, delete, legacy author) is
`taskfiles` `TestDecide`, the watcher raise is `TestDetailsWatch` and `TestDetailsWatchLatestProof`, the
finish flush reply is `TestDetailsFiles`. Two grants on one task (W3) has no flow: no harness drives two
live grants on a task. Real `claude`: `TestAdeDetailsSubmit` is `TestAdeDetailsOnSave`; bench arm R lost
`submit` (arm T and the spikes keep the frozen tool), its F3 oracles read the finish flush reply, and a
task's only proof is its latest, so the late correction applies at once. New Known open item: an
outside-editor save under one live grant is credited to the agent. Baselines to re-record at chapter end:
contracts `ade-automation`, `ade-details-files`, `ade-details-watch`, `ade-details-rules`, `mobile-approvals`, `proof`,
`proof-chart`, `proof-diagram`, `proof-explained`, `proof-files`, `proof-image` and `proof-pending` (proof
`sessionId` is now the writing session; already re-recorded here), llmprompts goldens and `budget.tsv`,
tool schema budget (unchanged numbers).

## P704

The task Files tab (P691) moves out of the main task chrome. It opens its own window with the project's
files, the way the review window opens, because it is a place to browse, not a primary step. It is
less important than Review: no big button, and no entry in the right-click menu. The plan picks a quiet
entry point (for example a small icon in the task header or a command palette entry) and the window
reuses the review window's wiring. Remove the old tab, its layout space, specs and baselines; add the
window spec. Keep the existing file views and diff behaviour inside the new window.

Result: Files tab removed; task panel tabs are Main, Details, Sessions. Header icon `ade-panel-files`
opens one Files window per task (`OpenFilesWindow` focuses an open one). Keys `adefiles-`, in-memory
registry (`internal/ade/fileswindow.go`), never restored; Archive closes it, a native close releases
file sessions. List and open file refetch on window focus; pane width persists per device. Guard U214.
Flow `TestTaskFilesWindow` and UI specs share contract `ade-task-files`. Baselines `files-open`,
`files-reading`, `files-search`, `files-launch-error`, `files-empty` and the doc screenshots showing a
task panel header are re-recorded at chapter end. `files1` heap checkpoint dropped.

## P705

The mobile app is nearly useless today because much of the status info the desktop shows is missing.
Deliverable: an audit in the plan comparing what the desktop board, task panel, sessions and runs show
with what mobile shows (`apps/kira-space/frontend/mobile/`, `internal/mobileweb`, `internal/bridge/mobile.go`,
`MobileStatus`, mobile wire schemas), a ranked list of the missing status (task and step state, running
agents and their last activity, what needs the user, run and script state, branch and git state, Docker,
connectivity and last-update age), and the additions that make it useful at a glance on a phone: data the
mobile bridge must carry, the screens that show it, push updates instead of manual refresh, and empty,
loading and offline states. Follow the frontend rules (shadcn-vue, Tailwind, Pinia, TanStack Query) and
keep the mobile bundle small. Tests split at the IPC boundary (flow test plus `mobile-ios` UI spec, same
scenario). Independent of P699 to P704 except P701 (restore state may also belong on mobile): the
planner states shared files.

Result: Phone gains Activity tab (agents, script runs, computer: connection, repos, Docker), richer Plan
cards (sessions, run line, Jira, branch rows with tag, ahead/behind, PR state and checks). New
`/api/runs` and `/api/computer`, script runs push with keep-list projection. Refresh button removed;
header shows update age, stream reopens on visible/online. Flow `TestPhoneStatus` plus
`tests/mobile/status.spec.ts` share contract `mobile-status.json`. Bundle `dist-mobile/assets` 1,585,470
B to 1,637,788 B; index js gzip 298.54 KB to 312.96 KB (includes chapter siblings).

## P706

Keep the generic pull-request icon (no GitHub brand mark), but make that one icon reflect all the PR
status and info the user expects. Today the PR glyph (`PR_GLYPH` in `ade/v2/plan/glyphs.ts`) shows only
the change-request state (open, draft, merged, closed), checks sit in a separate icon
(`CHECKS_GLYPH`, `AdeBranchPr.vue`), and the host sidebar (`HostViewSidebar.vue`) keeps a third table
(`CHECK_ICON`, `REVIEWER_ICON`). Deliverable: one shared glyph model and one component used by the branch
row, the host sidebar and mobile, where the icon's shape carries the PR state and a badge or tone carries
the checks state (pending, passed, failed, expected), with review state (approved, changes requested),
merge conflicts and auto-merge when the host reports them, plus one tooltip that spells out every part. The
plan lists every status the host data already carries (`checksState`, reviewers, mergeable, draft) and
ranks which ones the icon shows. Pending checks must be visible (animated, respecting reduced motion);
passed checks stop using a second icon. Icons come from Codicons (already shipped); no new pack. Remove the
duplicated tables, update specs and visual baselines (re-recorded at chapter end), copy keys and the
ARCHITECTURE entry. Frontend only unless the host data lacks a status the plan needs.

Result: `ade/v2/pr/prGlyph.ts` is the one PR status table and `PrGlyph.vue` the one component (button on
the branch row, tooltip on the task panel PR line, inline in the host view sidebar and on mobile plan
cards through `PlanTaskPrs.vue`). Shape is the state; a StatusDot badge is the checks (pulse while
pending, static under reduced motion, passed keeps the dot); tone ranks conflicts, changes requested,
auto-merge, approved for open and draft. One tooltip lists every part, one line each. `CHECKS_GLYPH`,
`PR_GLYPH`, `CHECK_ICON`, `REVIEWER_ICON`, `CHECK_TONE` and `panel/prState.ts` deleted; the host view
conversation reads `REVIEW_GLYPH` too; guard U215, and U125 lists `PrGlyph.vue` as a board tone owner.
Backend: `githost.ChangeRequest` carries `Mergeable` and `AutoMerge` (GitHub sync fields, Azure list
projection), index migration 0104 and schema version 3; the ADE wire carries review, mergeable and
auto-merge through `wireChangeRequest`. Fixture `change-requests.json` moved to wire vocabulary. Flow
`TestChangeRequestGlyphStatus` and UI spec `ade-v2-pr-glyph.spec.ts` share contract `ade-pr-glyph`;
mobile spec `plan-prs.spec.ts`. Baselines with a PR row on the Plan board are re-recorded at chapter
end (candidates: `ade-plan-branches` and the other Plan specs using the default change-requests mock).

## Streams

P705 is independent of the file work and can run in any free slot. P699 then P700 run in order (the file service owns the watcher API). P701, P702, P703 and P704 are
independent of each other and of the file work, except P703 and P702 both touch approval wiring
(`internal/agentmcp`, approvals copy): the planner of the second lists shared hunks, otherwise run them
together. Batch up to three streams as in the working agreement, plans and ownership tables decide.
