# v0.3.21

Chapter v0.3.21, branch `chapter/v0.3.21`, tag `v0.3.21`. First phase `P699` (highest history phase: `P698`).

| # | Phase | Status |
|---|-------|--------|
| P699 | File watching: one shared seam, FSEvents on macOS | Done |
| P700 | File interactions: one shared file service | Planned |
| P701 | Restore after restart: dialog to resume scripts, Docker and agents | Done |
| P702 | Agents remove worktrees; no prompt for worktree create or delete | Planned |
| P703 | Edit prompts only for another agent's notes; the watcher raises them, no `submit` | Planned |
| P704 | Task Files: open in its own window, off the main chrome | Done |
| P705 | Mobile app: show the status info it is missing | Planned |
| P706 | Pull request icon: one glyph for every PR status | Planned |

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

## Streams

P705 is independent of the file work and can run in any free slot. P699 then P700 run in order (the file service owns the watcher API). P701, P702, P703 and P704 are
independent of each other and of the file work, except P703 and P702 both touch approval wiring
(`internal/agentmcp`, approvals copy): the planner of the second lists shared hunks, otherwise run them
together. Batch up to three streams as in the working agreement, plans and ownership tables decide.
