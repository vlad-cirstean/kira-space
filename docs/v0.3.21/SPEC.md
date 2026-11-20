# v0.3.21

Chapter v0.3.21, branch `chapter/v0.3.21`, tag `v0.3.21`. First phase `P699` (highest history phase: `P698`).

| # | Phase | Status |
|---|-------|--------|
| P699 | File watching: one shared seam, FSEvents on macOS | Planned |
| P700 | File interactions: one shared file service | Planned |
| P701 | Restore after restart: dialog to resume scripts, Docker and agents | Planned |
| P702 | Agents remove worktrees; no prompt for worktree create or delete | Planned |
| P703 | Edit prompts only when an edit touches another agent's notes | Planned |
| P704 | Task Files: open in its own window, off the main chrome | Planned |

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

## P704

The task Files tab (P691) moves out of the main task chrome. It opens its own window with the project's
files, the way the review window opens, because it is a place to browse, not a primary step. It is
less important than Review: no big button, and no entry in the right-click menu. The plan picks a quiet
entry point (for example a small icon in the task header or a command palette entry) and the window
reuses the review window's wiring. Remove the old tab, its layout space, specs and baselines; add the
window spec. Keep the existing file views and diff behaviour inside the new window.

## Streams

P699 then P700 run in order (the file service owns the watcher API). P701, P702, P703 and P704 are
independent of each other and of the file work, except P703 and P702 both touch approval wiring
(`internal/agentmcp`, approvals copy): the planner of the second lists shared hunks, otherwise run them
together. Batch up to three streams as in the working agreement, plans and ownership tables decide.
