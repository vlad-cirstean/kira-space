# v0.2.30

Branch `chapter/v0.2.30`. First `P` is 495 (highest in v0.2.29 is P494).

Source: user request. Phases run top to bottom, one at a time. Scope is closed: further phases go to v0.2.31. Ends with one code review round, then close, then PR.

All rows are Kira Space (`apps/kira-space`), ADE unless named.

Parallel streams: code files are disjoint between every pair of P495, P496, P497, P498 and P499, except one shared file, `packages/theme/src/i18n/en.json`. P495/P496 touch neighbouring panel files (`AdeTaskPanel.vue`, `AdeTaskTab.vue`) but no common file. P499 (Go review store, `packages/git-ipc`, `packages/git-ui`, review window) shares nothing with P495–P498 but `en.json`. So P499 can run as a stream beside one of P495–P498, if the plan confirms the split per CLAUDE.md: an ownership table, `en.json` key ownership, no ordering dependency. Without that confirmation, rows run in sequence. Cap 2 streams.

| Phase | Title | Status |
|---|---|---|
| P495 | ADE: stage action resumes the existing agent session | Todo |
| P496 | ADE: handover only in Notes | Todo |
| P497 | ADE: task "+" launch menu shares the Runs dropdown | Todo |
| P498 | ADE: one Agents tab for Context and Settings | Todo |
| P499 | Review: show reviewed changes removed since review | Todo |
| P500 | Code review (1 round) | Todo |
| P501 | Close chapter v0.2.30 | Todo |

## Rows

### P495 ADE: stage action resumes the existing agent session

- Today: a manual stage with `session: true` and no running TUI session shows `▶ <Step> session` (`TaskAction` kind `session`, `manualAction` in `frontend/src/ade/v2/board/actions.ts`). It always starts a new session (`useTaskAction` → `dialogs.session` → `sessionSpec` → `adeTaskLaunchStep`), even when a stopped session of that stage exists.
- Want: when the stage has a stopped TUI session with a conversation (`agentSessionId` set; newest wins), the default action is Resume. Resume continues that session via the existing take over (`useAdeTakeOverStore.request` → `TaskBoard.TakeOver`, which resumes a stopped TUI record). Running session: unchanged.
- Start new stays available, but only in the task menu, not as the default button.
- Archive and Backlog leave the task panel header (`ade-panel-archive`, `ade-panel-shelve` in `panel/AdeTaskPanel.vue`) and live only in the task menu. The menu already holds `ade-task-archive`, `ade-task-backlog` and `ade-task-later` (`taskMenuModel`, `board/menu.ts`). A finished task's step action `Archive` (kind `archive`) is a step action, not the panel button, and stays.
- The task menu has one source: `taskMenuModel` mapped by `useTaskMenu` (`plan/useTaskMenu.ts`). Right-click (`AdeTaskCard`, `AdeShelfRow`, panel) and the 3-dots buttons (`ade-panel-more`, shelf row) all open it. Its `ade-task-action` item already mirrors the step action (so shows Resume). Start new goes into `taskMenuModel` only, never a second list. Plan confirms every entry point still goes through `useTaskMenu`.
- Phone: `mobile/state/useTaskActions.ts` maps `action.kind === 'session'` to its Start button. Plan keeps the phone's behaviour unchanged and says how.
- Test: backend flow — take over of a stopped manual-stage TUI session resumes its conversation, unless an existing flow already asserts exactly that (plan names it). UI spec on the same scenario: Resume default, Start new in the menu from right-click and 3-dots, Archive/Backlog absent from the header and present in the menu. Update `ade-v2-task-menu`, `ade-v2-archive`, `ade-v2-later`, `ade-v2-task-flow` specs as needed.
- Result: pending.

### P496 ADE: handover only in Notes

- Today: the Task tab (`panel/AdeTaskTab.vue`) shows a Handover row (`handoverSnippet`, testid `ade-task-handover-row`) that jumps to Notes. The Notes tab already lists handovers (`AdeHandoverList` in `AdeTaskPanel.vue`).
- Want: no handover in the Task tab. Handover shows only in Notes. Remove the row, its computed and any i18n key left unused.
- Frontend only; IPC unchanged, so no backend test.
- Test: UI spec `ade-v2-handover` updated: no handover on the Task tab, still in Notes.
- Result: pending.

### P497 ADE: task "+" launch menu shares the Runs dropdown

- Today: the task Sessions tab "+" (`sessions/AdeTaskLaunchMenu.vue`, testid `ade-task-launch`) is a `TooltipIconButton` with a shadcn `DropdownMenu`: terminals and agent sessions per worktree, provider submenus, disabled reasons. The Runs module (Automations, `packages/workbench/src/automations`) "+" buttons open a context-menu-store dropdown under the button (`openContextMenuAt`, `MenuItem[]` with icons). These are `automations-add` in `AutomationsPanel.vue` (items from `panelMenus.ts` `newScriptItems`) and the tab strip "+" (`AutomationsNewTab.vue` over `TabStripNewButton`).
- Want: the task "+" looks and behaves like the Runs dropdown, through one shared component, not a copy. Options stay the task's own (terminal / agent session per worktree, provider choice, disabled reasons).
- Plan names the shared component and its home (`packages/workbench`). Plan says which Runs "+" adopts it: at least the one the task "+" matches. Plan maps today's labels, submenus and disabled reasons onto `MenuItem` (`hint`, `submenu`). Keep testids stable or update specs in the same pass.
- Frontend only; IPC unchanged, so no backend test.
- Test: UI spec `ade-v2-task-launch` (and `ade-v2-launch` if touched) on the new dropdown; the Runs spec covering the adopted "+" still passes.
- Result: pending.

### P498 ADE: one Agents tab for Context and Settings

- Today: ADE top bar has two tabs, Context (`context/AdeContextPage.vue`, `ade-tab-context`) and Settings (`settings/AdeSettingsPage.vue`, `ade-tab-settings`), in `shell/AdeShell.vue`. Both pages repeat the same draft/dirty/hash/conflict editor logic. Each has its own leave-guard store (`state/adeContextUi.ts`, `state/adeSettingsUi.ts`). `adeBoardUi.view` carries both values.
- Want: one top tab for agent configuration, with sub-tabs (`SecondaryTabs`) for Context and Settings. Settings keeps its secrets column (`AdeSettingsSecrets`). Plan picks the tab label.
- One leave guard covers both sub-tabs and the top tab: unsaved edits ask before switching sub-tab or leaving. One store for that concern, not two. The shared editor logic moves to one composable.
- Frontend only; IPC unchanged, so no backend test.
- Test: UI specs `ade-agent-context`, `ade-agent-settings`, `ade-agent-port`, `ade-agent-missing`, `agent-provider` updated to the new tab; one new test for the leave guard across sub-tabs.
- Result: pending.

### P499 Review: show reviewed changes removed since review

- Today: review marks are stored per (repo, branch, path) in the review DB (`internal/gitreview/store.go`, `FileRecord`). Each record holds state, reviewed ranges in snapshot coordinates, `reviewed_at_sha` and the snapshot content. `RangeFiles`/`rangeFilesRecords` (`internal/gitsession/incremental.go`) lists only paths in the current range diff. A reviewed file gone from the diff silently drops out. Within a listed file, `ProjectRanges` (`gitreview/project.go`) drops reviewed ranges whose lines were deleted. `markFileAt` then overwrites the snapshot, so that content is lost.
- Correct today: unreviewed content that leaves the diff disappears. Keep that.
- Want: content the user marked reviewed (whole file or hunk/range), now gone from the diff, shows in the review window as "removed since review". Each entry shows path, when it was reviewed, and the stored reviewed content now gone. The user acknowledges (dismisses) each entry; a dismissed entry never comes back.
- Plan designs storage so removed reviewed content survives a later mark of the same file. Today the next `Put` replaces the snapshot. Likely shape: a migration under `gitreview/migrations` (next after `0005`) recording removed reviewed content, written before the overwrite. Plan names the wire (`gitrpc` method, `packages/git-ipc` types) and where the section renders (`packages/git-ui` `ReviewFilesPane` vs the ADE review window `frontend/src/ade/v2/review/`).
- Scope: committed review marks. Plan states whether uncommitted marks (`review_working`, `working_review.go`) can lose reviewed content the same way; if yes, include it, else say why not.
- Plan may split into Parts (storage + wire, then UI).
- Test: backend flow in `internal/flows/reviewflow`: mark a file and a hunk reviewed; remove both from the branch; the list returns both entries with their content; dismiss clears them. UI spec on the same scenario (section shows, content visible, dismiss hides). `gitreview` unit tests only for the range arithmetic, if any is new.
- Result: pending.

### P500 Code review (1 round)

One Opus review over the full diff since `main`, findings file under `plans/`, then one Sonnet fixer. Then close, then PR.

### P501 Close chapter v0.2.30

Hand-written close per CLAUDE.md "Closing a chapter". Push, open PR to `main`.
