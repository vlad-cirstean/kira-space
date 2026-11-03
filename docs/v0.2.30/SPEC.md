# v0.2.30

Branch `chapter/v0.2.30`. First `P` is 495 (highest in v0.2.29 is P494), last is P504.

Source: user request. Phases run top to bottom, one at a time. Scope is closed: further phases go to v0.2.31. Ends with one code review round, then close, then PR.

All rows are Kira Space (`apps/kira-space`), ADE unless named.

Parallel streams: code files are disjoint between every pair of P495, P496, P497, P498 and P499, except one shared file, `packages/theme/src/i18n/en.json`. P495/P496 touch neighbouring panel files (`AdeTaskPanel.vue`, `AdeTaskTab.vue`) but no common file. P499 (Go review store, `packages/git-ipc`, `packages/git-ui`, review window) shares nothing with P495–P498 but `en.json`. So P499 can run as a stream beside one of P495–P498, if the plan confirms the split per CLAUDE.md: an ownership table, `en.json` key ownership, no ordering dependency. Without that confirmation, rows run in sequence. Cap 2 streams. P500–P502 run in sequence, never as streams: P500 shares `board/actions.ts` with P495, P501 and P502 share `AdeTaskPanel.vue` with each other and with P496.

| Phase | Title | Status |
|---|---|---|
| P495 | ADE: stage action resumes the existing agent session | Done |
| P496 | ADE: handover only in Notes | Todo |
| P497 | ADE: task "+" launch menu shares the Runs dropdown | Todo |
| P498 | ADE: one Agents tab for Context and Settings | Done |
| P499 | Review: show reviewed changes removed since review | Done |
| P500 | ADE: task card icon row | Todo |
| P501 | ADE: task panel restructure | Todo |
| P502 | ADE: multi-branch task panel simplification | Todo |
| P503 | Code review (1 round) | Todo |
| P504 | Close chapter v0.2.30 | Todo |

## Rows

### P495 ADE: stage action resumes the existing agent session

- Today: a manual stage with `session: true` and no running TUI session shows `▶ <Step> session` (`TaskAction` kind `session`, `manualAction` in `frontend/src/ade/v2/board/actions.ts`). It always starts a new session (`useTaskAction` → `dialogs.session` → `sessionSpec` → `adeTaskLaunchStep`), even when a stopped session of that stage exists.
- Want: when the stage has a stopped TUI session with a conversation (`agentSessionId` set; newest wins), the default action is Resume. Resume continues that session via the existing take over (`useAdeTakeOverStore.request` → `TaskBoard.TakeOver`, which resumes a stopped TUI record). Running session: unchanged.
- Start new stays available, but only in the task menu, not as the default button.
- Archive and Backlog leave the task panel header (`ade-panel-archive`, `ade-panel-shelve` in `panel/AdeTaskPanel.vue`) and live only in the task menu. The menu already holds `ade-task-archive`, `ade-task-backlog` and `ade-task-later` (`taskMenuModel`, `board/menu.ts`). A finished task's step action `Archive` (kind `archive`) is a step action, not the panel button, and stays.
- The task menu has one source: `taskMenuModel` mapped by `useTaskMenu` (`plan/useTaskMenu.ts`). Right-click (`AdeTaskCard`, `AdeShelfRow`, panel) and the 3-dots buttons (`ade-panel-more`, shelf row) all open it. Its `ade-task-action` item already mirrors the step action (so shows Resume). Start new goes into `taskMenuModel` only, never a second list. Plan confirms every entry point still goes through `useTaskMenu`.
- Phone: `mobile/state/useTaskActions.ts` maps `action.kind === 'session'` to its Start button. Plan keeps the phone's behaviour unchanged and says how.
- Test: backend flow — take over of a stopped manual-stage TUI session resumes its conversation, unless an existing flow already asserts exactly that (plan names it). UI spec on the same scenario: Resume default, Start new in the menu from right-click and 3-dots, Archive/Backlog absent from the header and present in the menu. Update `ade-v2-task-menu`, `ade-v2-archive`, `ade-v2-later`, `ade-v2-task-flow` specs as needed.
- Result: Done. New `resume` action: default stage action resumes the newest stopped TUI session with a conversation via take over; running session unchanged. Start new moved into `taskMenuModel` only (right-click and 3-dots share `useTaskMenu`). Archive and Backlog removed from panel header; menu keeps them. Phone behaviour unchanged. Specs: new `ade-v2-stage-resume`; `ade-v2-task-flow`, `ade-v2-archive`, `ade-v2-later` updated.

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
- Result: Done. Single Agent config tab with Context and Settings sub-tabs; Settings keeps secrets column. One leave-guard store and one editor composable replace the duplicated pair. Frontend only. Specs: new `ade-agents-tab` (leave guard across sub-tabs); `ade-agent-context`, `ade-agent-settings`, `ade-agent-port` updated.

### P499 Review: show reviewed changes removed since review

- Today: review marks are stored per (repo, branch, path) in the review DB (`internal/gitreview/store.go`, `FileRecord`). Each record holds state, reviewed ranges in snapshot coordinates, `reviewed_at_sha` and the snapshot content. `RangeFiles`/`rangeFilesRecords` (`internal/gitsession/incremental.go`) lists only paths in the current range diff. A reviewed file gone from the diff silently drops out. Within a listed file, `ProjectRanges` (`gitreview/project.go`) drops reviewed ranges whose lines were deleted. `markFileAt` then overwrites the snapshot, so that content is lost.
- Correct today: unreviewed content that leaves the diff disappears. Keep that.
- Want: content the user marked reviewed (whole file or hunk/range), now gone from the diff, shows in the review window as "removed since review". Each entry shows path, when it was reviewed, and the stored reviewed content now gone. The user acknowledges (dismisses) each entry; a dismissed entry never comes back.
- Plan designs storage so removed reviewed content survives a later mark of the same file. Today the next `Put` replaces the snapshot. Likely shape: a migration under `gitreview/migrations` (next after `0005`) recording removed reviewed content, written before the overwrite. Plan names the wire (`gitrpc` method, `packages/git-ipc` types) and where the section renders (`packages/git-ui` `ReviewFilesPane` vs the ADE review window `frontend/src/ade/v2/review/`).
- Scope: committed review marks. Plan states whether uncommitted marks (`review_working`, `working_review.go`) can lose reviewed content the same way; if yes, include it, else say why not.
- Plan may split into Parts (storage + wire, then UI).
- Test: backend flow in `internal/flows/reviewflow`: mark a file and a hunk reviewed; remove both from the branch; the list returns both entries with their content; dismiss clears them. UI spec on the same scenario (section shows, content visible, dismiss hides). `gitreview` unit tests only for the range arithmetic, if any is new.
- Result: Done. New `review_removed` table (migration `0006`) written before a mark overwrites the snapshot; wire `review.removed.list` and `review.removed.dismiss`; ADE review window shows a removed-since-review section with path, review time and content; dismissed entries stay gone. Covers uncommitted (working) marks too. Specs: reviewflow flow and new `ade-v2-review-removed`. Caveat: `dismissing an entry` failed once, then passed 18/18; possible flake. Bindings are gitignored; regenerate via `scripts/setup.sh`. Open item: `MaxPatchBytes` limit.

### P500 ADE: task card icon row

- Today: `plan/AdeTaskCard.vue` has a left `AdeActionCell` column holding the primary action.
- Want: remove the left `AdeActionCell` column. The task tag becomes a toned icon with a tooltip in the card header line. The primary action (`AdeTaskActionButton`) is an icon at the right end. Each branch row shows status icon(s) at its right edge: max 2-3, the rest go in the row menu.
- Each state has a distinct glyph, not only a colour. Urgent states (conflict, setup failed, rebasing) are styled more prominently. The context-menu handler moves onto the icons. Icons are always visible, never hover-only. The card may grow a bit in height and width.
- Files: `AdeTaskCard.vue`, `AdeActionCell.vue`, `AdeBranchRow.vue`, `board/actions.ts`.
- Frontend only; IPC unchanged, so no backend test.
- Test: UI specs covering the card, action button and branch row updated to the new layout.
- Result: pending.

### P501 ADE: task panel restructure

- Want: tabs Main, Details, Sessions. Title stays where it is.
- Main: workflow steps and branches. A PR shows on its branch, with target branch and check result.
- Details: merges the task Notes and Artifacts tabs. Linked items sit in categories Main and Related, shown as links only (no bare numbers). Paste adds an item, with provider detection. Edit is inline, Notion-style, not one input per item. A collapsible chronological timeline lists everything added during the task's life.
- Handovers, agent notes and artifacts link to the agent session that created them. Plan checks whether entries already store the originating session id. If not, the backend records it, with a flow test.
- Plan accounts for P496 (handover only in Notes). Not parallel with P496.
- Files: `panel/AdeTaskPanel.vue`, `panel/AdeTaskTab.vue` and sibling tab components, adewire types, backend storage if the session id is missing.
- Test: backend flow if storage changes; UI specs for the tabs, Details editing, timeline and session links.
- Result: pending.

### P502 ADE: multi-branch task panel simplification

- Today: multi-branch tasks are confusing in the details panel. The task list is fine.
- Want: simplify how several branches show in the Main tab. Free hand for the planner.
- Runs after P501: both touch `AdeTaskPanel.vue`.
- Frontend only; IPC unchanged, so no backend test.
- Test: UI spec for a multi-branch task in the Main tab.
- Result: pending.

### P503 Code review (1 round)

One Opus review over the full diff since `main`, findings file under `plans/`, then one Sonnet fixer. Then close, then PR.

### P504 Close chapter v0.2.30

Hand-written close per CLAUDE.md "Closing a chapter". Push, open PR to `main`.
