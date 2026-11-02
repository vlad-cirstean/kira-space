# v0.2.27

Branch `chapter/v0.2.27`. First `P` is 459 (highest in v0.2.26 is P458).

Source: user request. Phases run top to bottom, one at a time. Scope is closed: further phases go to v0.2.29. Ends with one code review round, then close, then PR. The earlier v0.2.27 draft is now `docs/v0.2.28/`.

| Phase | Title | Status |
|---|---|---|
| P459 | Task details: drop Notes, generic labels, remove Jira PR search | Done |
| P460 | Jira issue view: fast open, readable render, images | Todo |
| P461 | Task "+" new terminal or agent works; remove list-row terminal button | Todo |
| P462 | Global capture dialog: keyboard only, PR-derived branches | Todo |
| P463 | Persist task tabs across task switches | Todo |
| P464 | Review window: default tab, PR line comment, send clears, resizable panels | Todo |
| P465 | Review window: re-review on edits, visual indicators, next/prev unreviewed | Todo |
| P466 | ADE panel tabs: grey circle count badges | Todo |
| P467 | Start a workflow without a repo | Todo |
| P468 | Agent branch and worktree requests: nanoid worktree name, branch in task | Todo |
| P469 | Stage move: advance button with close-session prompt | Todo |
| P470 | Code review (1 round) | Todo |
| P471 | Close chapter v0.2.27 | Todo |

## Rows

### P459 Task details: drop Notes, generic labels, remove Jira PR search

- Task details (main view): drop the Notes section; it takes too much space. Plan decides whether notes stay reachable elsewhere (tab) or data stays untouched. No data loss.
- Replace the `gh` and Azure labels with a generic word (PR). Replace the Jira label with a generic word (Issue). Plan lists every UI string, i18n key and tooltip carrying a vendor name where the concept is generic.
- Jira-to-source-control PR search shows unrelated public repos. Check whether the Jira ticket itself exposes PR links (dev-status or remote links). If yes, use only those. If no, remove the search for now, UI and backend wiring that nothing else uses.
- Test: UI spec (labels, Notes gone). Backend flow only if backend changes.

**Result.** Notes row gone from Task tab; Notes tab unchanged, no data change. Task tab and archived panel label rows "Issue" and "PR" (`space.ade.taskTab.pr.label`); placeholders and destination names stay vendor-specific. Ticket exposes no PR links without credentials (acli gives the Development count only), so key search removed: `ade.KeyChangeRequests`, `githost.ChangeRequestSearch` (GitHub, Azure, fake host), `ByHeadContaining`, `changeRequests` on `TaskIssue`, `IssueDetail`, `ArtifactGap.count`. Task tab and issue view show Jira's PR count. Tests: flow `TestTaskIssueTracker` asserts a title-matching foreign PR no longer lists; `TestIssueView` count only; contracts regenerated; new UI spec for labels and Notes; full `test:flows:space` and `test:ui:space` pass (987 passed; 2 load timeouts passed on lone rerun); visual specs `ade-sandbox`, `ade-sessions-menu` unchanged. Open item added to ARCHITECTURE.md: Jira PR rows not listed.

### P460 Jira issue view: fast open, readable render, images

- Opening a Jira ticket is slower than the browser. Measure where the time goes (fetch fan-out, sequential calls, CLI spawn, render), fix the cause. Show cached or partial data first when the plan finds it safe.
- Rendering is ugly and hard to read. Restyle: typography, spacing, headings, lists, code, tables, panels, links, mentions, status chips.
- Render images and attachments inline. No credential access: the existing CLI path stays the only fetch route (CLAUDE.md credential rule). Plan finds how to fetch image bytes without touching tokens, or states the requirement no route meets.
- Test: Go flow for the fetch path; UI spec for render incl. an image.

### P461 Task "+" new terminal or agent works; remove list-row terminal button

- "+" in a task (new terminal or agent) does nothing. Root-cause and fix; both options work.
- Remove the new-terminal button from the task row in the task list.
- Test: UI spec (press "+", terminal and agent appear); backend flow if the cause is backend.

### P462 Global capture dialog: keyboard only, PR-derived branches

- Remove the checkboxes. Keyboard only: Enter adds to Later, a modifier plus Enter adds to Today (plan picks the modifier, shows it in the dialog). Remove the open option.
- When the input is a PR, resolve repo and head branch from the PR, fetch them, and add them as branches on the new task for review.
- Test: backend flow (PR input yields repo and branch on the task) and UI spec (shortcuts, no checkboxes) on the same scenario.

### P463 Persist task tabs across task switches

- Switching tasks loses the tab: leave a task on Session, open another on Notes, return, first shows Notes. Keep the last active tab per task for the app session. Plan decides whether it survives restart.
- Pinia store, one concern. No refetch from the switch alone.
- Test: UI spec.

### P464 Review window: default tab, PR line comment, send clears, resizable panels

- Comments panel opens on the first tab (the agent-question tab, "q") by default.
- Add a dedicated option in that panel to write a comment on the PR at that line (host review comment). Plan finds the existing change-request comment path and confirms it per host.
- "Send review comments" clears the sent comments.
- Code diff and agent panels resize freely. Agent panel today stops at a max width. Remove or raise the cap; keep sane minimums.
- Test: backend flow (PR line comment, send clears) and UI spec on the same scenario.

### P465 Review window: re-review on edits, visual indicators, next/prev unreviewed

- Agent edits after a file was reviewed: new files show, edits to already reviewed files do not, or the left-panel checkbox stays stale. Fix: a reviewed file whose content changed since review returns to unreviewed. Plan picks the change key (blob hash or diff hash).
- Clear visual indicators of which file needs attention: unreviewed, changed since review, reviewed.
- Left panel gets previous and next unreviewed file buttons.
- Test: backend flow (review, edit, file unreviewed again) and UI spec on the same scenario.

### P466 ADE panel tabs: grey circle count badges

- ADE panel tabs showing a number show it in a grey circle. Distinctive, not distracting. Tailwind; one shared badge component.
- Test: UI spec.

### P467 Start a workflow without a repo

- Allow starting a workflow with no repo selected. The first stage decides repos and branches. Remove the repo-required guard on the start path; downstream stages that need a repo handle its absence.
- Test: backend flow (start without repo) and UI spec.

### P468 Agent branch and worktree requests: nanoid worktree name, branch in task

- An agent requesting a branch and worktree: name the worktree folder with a nanoid, keep the requested branch name as is.
- Add the branch to the task's Branches section.
- Test: backend flow (names, task branches) and UI spec.

### P469 Stage move: advance button with close-session prompt

- After a stage change the Finish button shows while the open session belongs to the previous stage. Replace with a clear move-on button. If a previous-stage session is open, ask whether to close it, then advance.
- Test: UI spec; backend flow if the stage logic changes.

### P470 Code review (1 round)

One Opus review over the full diff since `main`, findings file under `plans/`, then one Sonnet fixer. Then close, then PR.

### P471 Close chapter v0.2.27

Hand-written close per CLAUDE.md "Closing a chapter". Push, open PR to `main`.
