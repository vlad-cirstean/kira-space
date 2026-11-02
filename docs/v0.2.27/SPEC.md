# v0.2.27

Branch `chapter/v0.2.27`. First `P` is 459 (highest in v0.2.26 is P458).

Source: user request. Phases run top to bottom, one at a time. Scope is closed: further phases go to v0.2.29. Ends with one code review round, then close, then PR. The earlier v0.2.27 draft is now `docs/v0.2.28/`.

| Phase | Title | Status |
|---|---|---|
| P459 | Task details: drop Notes, generic labels, remove Jira PR search | Done |
| P460 | Jira issue view: fast open, readable render, images | Done |
| P461 | Task "+" new terminal or agent works; remove list-row terminal button | Done |
| P462 | Global capture dialog: keyboard only, PR-derived branches | Done |
| P463 | Persist task tabs across task switches | Done |
| P464 | Review window: default tab, PR line comment, send clears, resizable panels | Done |
| P465 | Review window: re-review on edits, visual indicators, next/prev unreviewed | Todo |
| P466 | ADE panel tabs: grey circle count badges | Done |
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

**Result.** Open cost was `Issue` plus `workitem view` plus a comment-list call, all fresh, in two serial stages. Now one `workitem view` per read (`Issue` only on a site's first read), comment overflow runs in parallel, and a saved copy serves under 30 s as is, under 10 min with `Stale` (window revalidates once), else one coalesced read. Window open starts a background prefetch; posting a comment drops the copy. Render: callouts (`markdown-it-github-alerts`, MIT), mention and status chips, scrolling tables, 760 px column, relative times with tooltip. Images show only as links and tiles that open in the browser: acli has no download command, the attachment content URL needs credentials Kira never touches, and the CSP allows only `img-src 'self' data:`. Open item added to ARCHITECTURE.md. Tests: Go flows `TestIssueView*`, new `TestIssueViewFetch` (spawn counts), `jira` unit tests for cache rules and parallel overflow; UI specs `issue-view`, `host-view`, new `issue-view-fetch`; contracts regenerated. Ran only P460's specs plus targeted reruns (32 UI passed; flows and jira tests pass); full suites deferred to chapter end per coordinator. A partial full UI run showed one failure, `ade-v2-branch-request` "Deny", which passed on rerun (load). No real-site check was possible.

### P461 Task "+" new terminal or agent works; remove list-row terminal button

- "+" in a task (new terminal or agent) does nothing. Root-cause and fix; both options work.
- Remove the new-terminal button from the task row in the task list.
- Test: UI spec (press "+", terminal and agent appear); backend flow if the cause is backend.

Result: Cause frontend. A Tooltip root between DropdownMenu and its trigger stole the menu's Popper anchor (reka shares one PopperRoot context), so the menu stayed at translate(0,-200%), off-screen. Plus now `TooltipIconButton` inside `DropdownMenuTrigger as-child` (same fix on card change-base menu). Card-head plus removed; plus stays on Sessions tab bar. Specs `ade-v2-task-launch`, `agent-provider`, `ade-v2-base-rebase` click by mouse and assert in-viewport. Full `test:ui:space`: 988 passed, 1 failed (spec helper, fixed, targeted rerun of 4 related specs 58 passed). Rule added to `docs/ARCHITECTURE.md`.

### P462 Global capture dialog: keyboard only, PR-derived branches

- Remove the checkboxes. Keyboard only: Enter adds to Later, a modifier plus Enter adds to Today (plan picks the modifier, shows it in the dialog). Remove the open option.
- When the input is a PR, resolve repo and head branch from the PR, fetch them, and add them as branches on the new task for review.
- Test: backend flow (PR input yields repo and branch on the task) and UI spec (shortcuts, no checkboxes) on the same scenario.

Result: radios and the open actions are gone. Enter adds to Later, CmdOrCtrl+Enter (`form.submit`) to Today; hints show both. `CaptureTask` takes an optional plan day. A pasted PR is fetched and its head branch attached to the new task as a review branch (`TaskBoard.AddReviewLink`). Partial outcomes (repo not imported, PR not found) return a `notice` and keep the task. Tests: captureflow, `adeflow` `TestCaptureChangeRequestForReview`, `tests/ui/capture.spec.ts`. The add popover Jira label is now the generic Issue key (P459 handoff).

### P463 Persist task tabs across task switches

- Switching tasks loses the tab: leave a task on Session, open another on Notes, return, first shows Notes. Keep the last active tab per task for the app session. Plan decides whether it survives restart.
- Pinia store, one concern. No refetch from the switch alone.
- Test: UI spec.

Result: Cause frontend. Task tab, branch tab and picked session were single global refs in `adeBoardUi`. New store `adePanelTabs` keeps them per task (tab, picked session) and per branch (tab); in memory for the app session, no restart persistence. `AdePanel` prunes entries of tasks and branches that leave the board. Readers and writers moved; `showSessions` goes through `openSessions`; See error/See log/Needs set the branch Details tab explicitly. No refetch added. New spec `ade-v2-panel-tabs` (task and branch switches, flat board and sessions call counts). Per coordinator rule no full suite: targeted `ade-v2-panel-tabs`, `ade-v2-panel`, `ade-v2-sessions`, `ade-v2-handover` 64 passed. Partial full run (about 580 of 991) showed no failures before stop.

### P464 Review window: default tab, PR line comment, send clears, resizable panels

- Comments panel opens on the first tab (the agent-question tab, "q") by default.
- Add a dedicated option in that panel to write a comment on the PR at that line (host review comment). Plan finds the existing change-request comment path and confirms it per host.
- "Send review comments" clears the sent comments.
- Code diff and agent panels resize freely. Agent panel today stops at a max width. Remove or raise the cap; keep sane minimums.
- Test: backend flow (PR line comment, send clears) and UI spec on the same scenario.

Result: Compose thread opens on Question, Change second (`ReviewThread`). Send clears: `gitreview.RemoveComments` replaces `MarkSent` in `ForwardReviewComments`; panel fires a comments repaint so open diff tabs drop glyphs; workflow review Send unchanged. PR line comment: contract 57 to 58, new `changeRequest.lineComment` (`githost.ChangeRequestLineComments`, GitHub `-F` only for `line`/`start_line`, Azure `threadContext`, `fakehost`/`fakeaz` record it, `gitsession.CommentOnChangeRequestLines` with `headMoved` check against the fresh host head). Frontend: third compose tab "{short} comment" on committed window-branch tabs (`useHostLineComment`, `reviewSource.postToChangeRequest`), blocked hint, posted state; i18n `git.review.comments.host.*`. Panes: `AdeReviewWindow` bounds from row width (right first, centre min 320), no fixed max. Also fixed pre-existing `typecheck:space-unit` error in `bridge/index.ts` (`hostViewIssue` `fresh` default). Ran: `go test` githost, gitrpc, gitsession, gitreview, ade, githostflow (`-count=2`), adeflow `TestReviewForwardComments|TestReviewStep`; contracts `ade-review-actions` and `ade-review-forward` regenerated; UI `ade-v2-review`, `ade-v2-review-working`, `repo-review-interaction`, `git-host-azure`, `copy-coverage`; visual `ade-review-diff` unchanged. Hooks green. No full suite.

### P465 Review window: re-review on edits, visual indicators, next/prev unreviewed

- Agent edits after a file was reviewed: new files show, edits to already reviewed files do not, or the left-panel checkbox stays stale. Fix: a reviewed file whose content changed since review returns to unreviewed. Plan picks the change key (blob hash or diff hash).
- Clear visual indicators of which file needs attention: unreviewed, changed since review, reviewed.
- Left panel gets previous and next unreviewed file buttons.
- Test: backend flow (review, edit, file unreviewed again) and UI spec on the same scenario.

### P466 ADE panel tabs: grey circle count badges

- ADE panel tabs showing a number show it in a grey circle. Distinctive, not distracting. Tailwind; one shared badge component.
- Test: UI spec.

Result: `SecondaryTabs` gets opt-in `countBadge` (`Badge variant="count"`, `px-1` so one digit is a circle). Task, branch, archived Sessions tabs and the All sessions Running/Stopped filter pass `count`; labels lose `{count}` (`space.common.sessions.label` added, `space.ade.panel.sessionsTab` and `space.ade.sessionList.trigger` removed). Sessions menu trigger uses the same Badge. U101 exemptions added for `SecondaryTabs.vue` and `AdeSessionsMenu.vue`. Also fixed a pre-existing `hostViewIssue` typecheck error (`fresh` default). Ran: typecheck space-web, space-tests, web:studio, lint; UI specs `ade-v2-panel-tabs` (new test), `ade-v2-needs`, `ade-v2-sessions-menu`, `ade-v2-archived`, `ui-leftovers`, `copy-coverage` pass. Visual `ade-sessions-menu`, `ade-sandbox` pass; `ade-plan-branches` diffs on an icon glyph only (sandbox font, no tab row), baseline untouched.

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
