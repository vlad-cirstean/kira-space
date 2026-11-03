# P495 ADE: stage action resumes the existing agent session

Plan for `SPEC.md` row P495. All paths under `apps/kira-space/` unless rooted otherwise.

## Findings (current tree)

- `manualAction` (`frontend/src/ade/v2/board/actions.ts:77`) returns kind `session` when the step has
  `session: true` and no running TUI session matches (`talking`). It never looks at stopped sessions.
- `useTaskAction.run` (`frontend/src/ade/v2/run/useTaskAction.ts:24`) maps `session` to
  `dialogs.session(taskId)` and `takeOver` to `takeOver.request(sessionId)`.
- `useAdeTakeOverStore.request` (`frontend/src/ade/v2/state/adeTakeOver.ts:80`) calls
  `TaskBoard.TakeOver` with `stopIfRunning: true`. `takeOverLocked` (`internal/ade/launches.go:150`)
  resumes a stopped TUI row in place (`pa.Resume = rec.ID`, same row id; `TestCloseSession` already
  shows the same id going back to `running`). No backend change needed.
- `Session` wire type has `agentSessionId`, `mode`, `state`, `stepId`, `purpose`, `lastActiveAt`.
  `copyItems` (`board/menu.ts:108`) already ranks by `lastActiveAt`; reuse that ordering.
- Task menu: `taskMenuModel` (`board/menu.ts:256`) is pure; `useTaskMenu` (`plan/useTaskMenu.ts:35`)
  maps `TaskMenuCmd` to calls. Callers: `AdeTaskCard.vue` (right-click on action cell, head, branch
  rows; `openAt` from keyboard), `AdeShelfRow.vue` (right-click `menu.open`, `ade-shelf-more`
  `openAt`), `AdeTaskPanel.vue` (`ade-panel-more` `openAt`). The panel has no right-click today.
- `ade-task-action` item (`menu.ts:279`) copies `c.action.label` and runs `perform()`; it shows
  Resume once the action is Resume, no change needed there.
- Panel header (`panel/AdeTaskPanel.vue:145`): `ade-panel-archive` (`quietArchive`), `ade-panel-shelve`
  (`shelfTarget`, `moveShelf`, `useShelveTask`). Menu already has `ade-task-archive`,
  `ade-task-backlog`, `ade-task-later`.
- Phone: `mobile/state/useTaskActions.ts:24` `startKind` maps `session` to `launch`
  (`writes.launchStep`). The phone's card action comes from the same `taskCell`
  (`mobile/state/useAgentsModel.ts`), so a new kind reaches it.
- Fixture safety: in `tests/fixtures/ade-v2/sessions.json` every stopped TUI row has `stepId ''`
  (`3c3c`, `sp11`) or `purpose 'review'` (`rv01`). A strict `stepId === step.id` match leaves every
  existing fixture action unchanged (`ade-v2-launch`, `ade-v2-board-parity` keep passing).
- Backend coverage: `harnessflow/provider_test.go` `TestLeanProviderSeams` (lines 424-483) takes
  over a stopped manual-stage TUI session, but under the lean fake provider, to test the provider's
  late-id seam, and records no contract. `adeflow/sessions_test.go` `TestCloseSession` resumes a
  stopped ad hoc TUI (no stage). `TestInteractiveSessions` takes over a headless run. None asserts
  the default-provider stage scenario with a contract for the UI half. So add one flow test.

## Design

### 1. New task action kind `resume` (`board/actions.ts`)

- Add `'resume'` to `TaskActionKind`. `sessionId` doc comment: "Session to open for `takeOver` and
  `resume`."
- New helper `resumableSession(sessions, step): Session | undefined`, local to `actions.ts`:
  filter `mode === 'tui' && state === 'stopped' && agentSessionId !== '' && stepId === step.id &&
  (purpose !== 'review' || step.id === 'review')`; newest by `lastActiveAt`, tie on `startedAt`.
  Strict `stepId` match (no `''` task-level rows): a task-level ad hoc session is not the stage's.
  Purpose filter mirrors `talking`.
- `manualAction`: keep the `talking` check first (running: unchanged). When `step.session && !talking`:
  - resumable session found: return `{ kind: 'resume', label: t('space.ade.board.taskAction.resume.label', { step }),
    tip: t('space.ade.board.taskAction.resume.tooltip', { step }), tone: 'agent', stepId: step.id,
    sessionId: s.id }`.
  - else: today's `session` action.
- Export nothing new beyond the kind; `menu.ts` reads `c.action.kind`.

### 2. Action wiring (`run/useTaskAction.ts`)

- `if (a.kind === 'resume') return takeOver.request(a.sessionId ?? '');` beside the `takeOver` line.
  `request` already sets `ui.actionError` on failure and guards double launch via `pending`.
- Add `startNew()` to the returned object: `dialogs.session(toValue(card).task.id)` after clearing
  `ui.actionError[taskId]`. One owner for "start a fresh stage session"; `useTaskMenu` calls it.
- `AdeTaskActionButton.vue`: comment line listing actions gains `Resume <Step>`. Testid becomes
  `ade-task-action-resume` automatically.

### 3. Start new in the task menu (`board/menu.ts`, `plan/useTaskMenu.ts`)

- `TaskMenuCmd` gains `{ kind: 'startNew' }`.
- `taskMenuModel`, inside `if (!review)` right after the `ade-task-action` push: when
  `c.action?.kind === 'resume'` push `{ type: 'item', id: 'ade-task-start-new',
  label: t('space.ade.board.taskMenu.startNew.label', { step: c.progress.step?.name ?? '' }),
  hint: t('space.ade.board.taskMenu.startNew.tooltip', { step }), cmd: { kind: 'startNew' } }`.
  Only there: Start new never shows as a default button and never in a second list.
- `useTaskMenu.run`: `case 'startNew': return startNew();` (from `useTaskAction(card)`, already
  instantiated there).
- Entry points (all already through `useTaskMenu`, confirmed by codegraph callers: `AdeTaskCard`,
  `AdeShelfRow`, `AdeTaskPanel`): card right-click and keyboard, shelf row right-click and
  `ade-shelf-more`, panel `ade-panel-more`. Add panel right-click: `@contextmenu="taskMenu.open"` on
  the `#header` slot's root `div` in `AdeTaskPanel.vue`, so the spec's "panel right-click" is real.
  `useTaskMenu.open` already calls `preventDefault`/`stopPropagation`. Nothing else builds a task
  menu list.

### 4. Panel header loses Archive and Backlog (`panel/AdeTaskPanel.vue`)

- Delete the `ade-panel-archive` `AdeTip`/`Button` and the `ade-panel-shelve` `Button`.
- Delete `quietArchive`, `shelfTarget`, `moveShelf`, `const shelve = useShelveTask()`, and imports
  left unused: `archiveTip`, `useShelveTask`, `errorText` only if no other use (it is used by
  `saveNotes`: keep). `useAdeDialogsStore` stays only if still used; else drop. `Button` stays (review
  button).
- Header comment unchanged in meaning. Finished task's step action `Archive` (kind `archive`,
  `AdeTaskActionButton`) stays.
- Behaviour note: menu `shelve` sends `beforeId = shelfIds(board, shelf)[0]` (top of shelf); the
  panel button sent `''`. Menu is the one source now; specs follow its args.

### 5. Phone (`mobile/state/useTaskActions.ts`)

- `startKind`: `return kind === 'session' || kind === 'resume' ? 'launch' : null;` with the comment
  "Resume is desktop only; the phone's Start still opens a new session." Phone keeps today's Start
  button and `launchStep` call for a manual session stage, whether or not a stopped session exists.
  No mobile UI change, no mobile spec change.

### 6. Copy (`packages/theme/src/i18n/en.json`)

Add only:
- `space.ade.board.taskAction.resume.label`: `"Resume {step}"`
- `space.ade.board.taskAction.resume.tooltip`: `"Continue your last {step} session in {product}"`
- `space.ade.board.taskMenu.startNew.label`: `"Start new {step} session"`
- `space.ade.board.taskMenu.startNew.tooltip`: `"Open a fresh {product} session for {step}; the last one stays in Sessions"`

Remove none: `space.common.archive.label`, `space.common.backlog.label`, `common.later.label` and
`archiveTip`'s key all stay in use elsewhere. `bun scripts/copy/check-catalog.ts --unused` (in
`bun run lint`) confirms. Run `bun scripts/copy/check-copy.ts` for the wording rules.

## Tests (split at IPC)

### Backend flow: `internal/flows/adeflow/sessions_test.go` `TestStageSessionResume` (new)

- `flowharness.New(t, flowharness.WithTrackerGrace(time.Second))`; fake claude `"*"`:
  `{WaitFile: flag("first")}`, `{WaitFile: flag("again")}`.
- Repo via `cloneWithMain`/`importRepo`; workflow: `flowYAML("flow", ...)` with the same manual
  `work` step string `TestInteractiveSessions` uses (lines 80-81), as the only step.
  `createTask`; open `sessionWindow`. No branch setup needed (`TestLeanProviderSeams` launches a
  manual step on a branchless task).
- `LaunchStep` → `openLaunch` → `waitSession(..., "running")`. Release `first` →
  `waitSession(..., "stopped")`. `WaitUntil` the row's `AgentSessionID != ""` (the UI rule keys on
  it). If the fake TUI never reports one, fix the fake's SessionStart hook, not the assertion.
- Assert stopped row: `Mode == "tui"`, `StepID == "work"`, `Purpose == ""`.
  `app.Contract(t, "ade-stage-resume", "AdeTaskService.Sessions#stopped-step", row,
  flowharness.Mask("id", "agentSessionId", "terminalId", "taskId", "branchId", "runId", "resumes",
  "cwd", "startedAt", "lastActiveAt"))`.
- `args := adewire.TakeOverArgs{SessionID: row.ID, StopIfRunning: true}`;
  `app.Contract(t, "ade-stage-resume", "args:AdeTaskService.TakeOver", args)`.
- `TakeOver(args)`: `launch.SessionID == row.ID`, `launch.Cwd == row.Cwd`,
  `strings.Contains(launch.Command, "--resume "+row.AgentSessionID)`. `openLaunch`;
  `waitSession(row.ID, "running")` with `StepID` still `"work"`. Release `again`.
- Regenerate: `KIRA_CONTRACT=write CGO_ENABLED=1 go test ./internal/flows/adeflow -run TestStageSessionResume`
  (from `apps/kira-space`); commit `tests/contract/ade-stage-resume.json`.

### UI spec: `tests/ui/ade-v2-stage-resume.spec.ts` (new)

Header comment names the backend half. Sessions snapshot: `adeFixture('sessions')` minus `9ab0`
(running TUI on `T_auth`), plus two rows built from the contract row (`mode`, `state`, `purpose`,
`stepId` shape from the contract; identity fields set): `rs01` older and `rs02` newer
(`lastActiveAt`), both `taskId 'T_auth'`, `branchId 'b_auth'`, `stepId 'review'` (T_auth's current
manual session step), `agentSessionId 'claude-rs0N'`. One row with `agentSessionId ''` (`rs00`,
newest of all) proves an id-less row is skipped.

Tests:
1. `contract: Resume is the default and takes over the newest session` — `ade-task-action-resume`
   visible on `T_auth` card, `ade-task-action-session` absent; click; `adeTaskTakeOver` called once,
   args keys equal the contract args keys, `sessionId 'rs02'`, `stopIfRunning true`.
2. `Start new sits in the task menu from right-click and 3-dots` — right-click card head:
   `menu-item-ade-task-action` reads `Resume Review`, `menu-item-ade-task-start-new` present; click it:
   `ade-dialog` opens; send; `adeTaskLaunchStep` called with `{ taskId: 'T_auth', message: '' }`.
   Then open panel, `ade-panel-more`: `menu-item-ade-task-start-new` present. Panel header
   right-click: same menu.
3. `Archive and Backlog live in the menu, not the header` — open `T_auth` panel:
   `ade-panel-archive` and `ade-panel-shelve` count 0; `ade-panel-more` shows
   `menu-item-ade-task-archive` and `menu-item-ade-task-backlog`.
4. `a running stage session keeps today's action` — fixture unchanged (with `9ab0`), no
   `ade-task-action-resume`, no `ade-task-start-new` in the menu.

### Existing specs to update

- `tests/ui/ade-v2-task-flow.spec.ts` (Archive confirm): replace `ade-panel-archive` clicks with
  `ade-panel-more` → `menu-item-ade-task-archive`.
- `tests/ui/ade-v2-archive.spec.ts` `openArchive`: same replacement.
- `tests/ui/ade-v2-later.spec.ts`: "selecting a row opens its panel" asserts the menu offers
  `ade-task-backlog` for `one` and `ade-task-later` for `two` via `ade-panel-more` (not the button
  text). "a Backlog task moves to Later from the panel button" becomes "from the panel menu":
  `ade-panel-more` → `menu-item-ade-task-later`; expect `beforeId` the first Later id (menu's
  `shelfIds` rule), not `''`.
- `tests/ui/ade-v2-task-menu.spec.ts`: no testid it uses goes away; rerun only.
- `ade-v2-launch.spec.ts`, `ade-agent-missing.spec.ts` (`ade-task-action-session`): unaffected by
  the strict match; rerun only. Not edited (P497 may own `ade-v2-launch`).

No unit test: the resumable-session filter is one filter plus a sort, below the CLAUDE.md bar.

## Fast checks (implementer)

From repo root: `bun run typecheck`, `bun run lint`, `bun run build:test:space`. Go:
`CGO_ENABLED=1 go test ./apps/kira-space/internal/flows/adeflow -run 'TestStageSessionResume|TestInteractiveSessions|TestCloseSession'`.
UI, own and changed specs only:
`npx playwright test --config=apps/kira-space/playwright.config.ts --project=ui ade-v2-stage-resume ade-v2-task-flow ade-v2-archive ade-v2-later ade-v2-task-menu ade-v2-launch`.
Full suites wait for the chapter's single run.

## Commits

1. `feat(ade): resume a stopped stage session as the default step action` — actions.ts,
   useTaskAction.ts, menu.ts, useTaskMenu.ts, mobile useTaskActions.ts, en.json keys.
2. `refactor(ade): move Archive and Backlog from the task panel header to the menu` — AdeTaskPanel.vue.
3. `test(ade): stage session resume flow and UI spec` — sessions_test.go, contract JSON, new spec,
   updated specs.
4. `SPEC.md` row P495 Result + Status Done (orchestrator or implementer per chapter practice).

## File ownership

P495 owns (edits):
- `apps/kira-space/frontend/src/ade/v2/board/actions.ts`
- `apps/kira-space/frontend/src/ade/v2/board/menu.ts`
- `apps/kira-space/frontend/src/ade/v2/run/useTaskAction.ts`
- `apps/kira-space/frontend/src/ade/v2/run/AdeTaskActionButton.vue` (comment only)
- `apps/kira-space/frontend/src/ade/v2/plan/useTaskMenu.ts`
- `apps/kira-space/frontend/src/ade/v2/panel/AdeTaskPanel.vue`
- `apps/kira-space/frontend/mobile/state/useTaskActions.ts`
- `packages/theme/src/i18n/en.json` — only keys `space.ade.board.taskAction.resume.*` and
  `space.ade.board.taskMenu.startNew.*` (added; none removed)
- `apps/kira-space/internal/flows/adeflow/sessions_test.go`
- `apps/kira-space/tests/contract/ade-stage-resume.json` (new, generated)
- `apps/kira-space/tests/ui/ade-v2-stage-resume.spec.ts` (new)
- `apps/kira-space/tests/ui/ade-v2-task-flow.spec.ts`
- `apps/kira-space/tests/ui/ade-v2-archive.spec.ts`
- `apps/kira-space/tests/ui/ade-v2-later.spec.ts`

Reads only, no edit: `state/adeTakeOver.ts`, `state/adeDialogs.ts`, `plan/AdeTaskCard.vue`,
`later/AdeShelfRow.vue`, `internal/ade/launches.go`, `tests/fixtures/ade-v2/sessions.json`,
`tests/ui/ade-v2-task-menu.spec.ts`, `tests/ui/ade-v2-launch.spec.ts`.

Overlap with other rows:
- P496: `panel/AdeTaskTab.vue` and handover keys in `en.json`. No common file except `en.json`
  (disjoint keys). P496 does not touch `AdeTaskPanel.vue` if it stays to the Task tab row.
- P497: `sessions/AdeTaskLaunchMenu.vue`, `packages/workbench/src/automations/*`,
  `ade-v2-task-launch.spec.ts`, maybe `ade-v2-launch.spec.ts`. No common file with P495 except
  `en.json`; P495 leaves `ade-v2-launch.spec.ts` untouched for this reason.
- P498: `context/`, `settings/`, `shell/AdeShell.vue`, `state/adeContextUi.ts`,
  `state/adeSettingsUi.ts`, `state/adeBoardUi.ts`. No common file except `en.json`.
- P499: Go `internal/gitreview`, `internal/gitsession`, gitrpc, `packages/git-ipc`,
  `packages/git-ui`, `frontend/src/ade/v2/review/`. No common file except `en.json`.
- P500: shares `board/actions.ts` (and `AdeTaskCard.vue` menu handlers it moves). Runs after P495.
- P501, P502: share `panel/AdeTaskPanel.vue`. Run after P495.

No ordering dependency between P495 and P496-P499. A P495 stream beside P499 holds if the plan of
that split pairs this table with P499's and both keep to their own `en.json` keys.
