# P491 ADE: hide archived tasks unless History is open

Plan for SPEC row P491. Planned against `5e2d6cee` (P488 result). Stream C, second of P488, P491,
P492. One sequential implementer. Discovery used `codegraph_explore` over the archive flow
(`adeDialogs.archive`, `TaskBoard.archived`, `ArchivedTask`, `AdeArchivedPanel`), the Plan history
path (`buildTimeline`, `buildBand`, `collectDayKeys`, `historyItems`, `TimelineBand`,
`AdeHistoryControl`, `AdeHistoryBar`, `AdeArchivedFinder`, `useAdeBoardUiStore`) and the P492
agents widget (`AdeAgentsStatusItem`, `AdeAgentsStatusList`, `agentsSummary`). Then grep for every
`board.history`, `band.history` and `showHistory` reader, specs included.

## 1. Requirement and current behaviour

- Archived tasks stay hidden on the Plan until the user opens History. That includes tasks archived
  today.
- Today: `board.tasks` comes from `ListLive` (backend), so archived tasks never reach the cards.
  They reach the Plan only as `board.history` rows (`ade-history-row`, `AdeDayBand.vue:146`).
- The leak is one line, `apps/kira-space/frontend/src/ade/v2/board/timeline.ts:364` `buildBand`:

  ```ts
  history: ctx.showHistory || dk >= 0 ? ctx.history.filter((h) => h.day === dk) : [],
  ```

  `dk >= 0` puts every task archived today (and any clock-skewed future `archivedAt`) in the Today
  band with History closed. Past days already need `showHistory` (`collectDayKeys:403` adds negative
  bands only then).
- `docs/ARCHITECTURE.md:2323` documents the old rule ("today's archive rows always show"). UI spec
  `ade-v2-plan.spec.ts:47` asserts it.
- Second leak, by consequence: `AdePlanView.vue:294` `onFindArchived` opens History only when
  `item.day < 0`. After the fix, picking a task archived today in the finder would select it with
  its row hidden. Fix it in the same pass.

## 2. Backend or frontend filtering: frontend only

Decision: no backend change, no backend flow test.

- `board.history` must still carry today's entries with History closed. Three readers need them:
  `AdeArchivedFinder` (searches all of `board.history`), `AdePanel.vue:30` (an archived selected
  task opens the read-only panel, e.g. right after Archive) and `AdeAllSessions.vue:54` (title of an
  archived task's session). The History button count (`historyCount`, `timeline.ts:476`) also
  counts day 0, so a closed History still says how many archived tasks it holds.
- History open/closed is per-window UI state (`kira.ade.showHistory`, `usePersistedState` in
  `adeBoardUi.ts:33`). The backend never sees it; filtering there would need a new IPC argument and
  a refetch per toggle for no gain.
- CLAUDE.md "split at the IPC boundary" asks for a backend half when the feature crosses IPC. This
  one does not: the wire payload and every Go path stay byte-identical, so a flow test would assert
  nothing P491 changed. The existing `adeflow` archive tests still cover the backend side of
  archiving.

## 3. Change

1. `timeline.ts` `buildBand`: `history: ctx.showHistory ? ctx.history.filter((h) => h.day === dk) : []`.
   Nothing else in `timeline.ts` changes. `historyCount` keeps day 0 (count stays "archived tasks in
   the last N weeks", today included), so the closed History button shows the hidden rows' count.
2. `AdePlanView.vue` `onFindArchived`:

   ```ts
   async function onFindArchived(taskId: string): Promise<void> {
     ui.select(taskId);
     const item = model.value?.view.history.find((h) => h.taskId === taskId);
     if (!item) return;
     ui.showHistory = true;
     await revealDay(item.day);
   }
   ```

   `revealDay(k >= 0)` already scrolls to day `k` (or Later). Day 0 scrolls to Today, where the row
   now shows.
3. No other component changes. `AdeDayBand` empty-band check (`:63`) already counts history rows;
   with History closed a band holds none, as past bands do today. `AdeHistoryControl` toggle,
   `AdeHistoryBar`, `hideHistory` and persisted `showHistory` stay as they are.
4. Archiving the selected task: the board refetch moves it to `board.history` with `day` 0. The card
   leaves; the read-only archived panel stays open through `AdePanel.vue:30` (unchanged). The Today
   band shows no row until History opens. This is the requested behaviour.

Not touched: `apps/kira-space/frontend/mobile/**` (no history list there), Go `internal/ade/**`,
wire types, i18n (no new or changed string), contracts.

## 4. Library rules

No new UI surface, component, store, composable, query or dependency. The fix edits a pure function
and one existing handler. Existing shadcn-vue `Button`/`Popover`/`Command`, the Pinia store
`adeBoardUi` (single concern: Plan UI state) and VueUse-backed `usePersistedState` carry the toggle
unchanged. TanStack Query `useBoard` data is not filtered in a query `select`: the finder, panel
and sessions readers need the unfiltered list (2).

## 5. Tests

UI spec only, as the SPEC row asks; backend half declined (2).

`apps/kira-space/tests/ui/ade-v2-plan.spec.ts`:

- Replace test `a task archived today lists in the Today band without Load history, name first`
  (`:47`) with `a task archived today stays hidden until History opens, name first`. Same board
  edit (`H_1.archivedAt = FIXED_NOW - 36e5`). Steps:
  1. Plan loaded: `ade-history-row` count 0; `bandOf(page, 0)` holds no `ade-history-row`.
  2. `ade-load-history` shows a count (text contains `5`; the five fixture entries all fall in the
     2-week window).
  3. Click `ade-load-history`: row `H_1` visible inside `bandOf(page, 0)`, contains
     `Audit log for admin actions`, name width > 80 (keeps the old "name first" check).
  4. Click `ade-load-history` again (Hide history): `ade-history-row` count 0.
- Add test `finding a task archived today opens History on Today`: same board edit; click
  `ade-archived-find`, pick `ade-archived-find-item[data-task-id="H_1"]`; expect `ade-load-history`
  text `Hide history`, row `H_1` visible in `bandOf(page, 0)`, `ade-panel-title` visible. Put it in
  `ade-v2-archived.spec.ts` next to `picking an old archived task reveals its day` (`:42`), reusing
  its `pickArchived` helper and the `adeTaskArchivedTask` override with `archivedTaskFx('H_1')`.
- Existing `loads the archived history into the day bands` (`:32`) already asserts 0 rows before and
  after the toggle with the fixture's past dates; leave it.
- Unit: no new test (CLAUDE.md bar: one condition). `tests/unit/ade-v2-timeline.spec.ts` `history`
  block uses `showHistory: true` and stays green; `ade-v2-board-parity.spec.ts` mirrors the mockup,
  whose `hItems` already filter on `showHistory` alone (`ade-v2-mockup-logic.js:832`), so the fix
  brings code and mockup into line.
- Run: `bun run build:test:space`, then `npx playwright test
  --config=apps/kira-space/playwright.config.ts --project=ui ade-v2-plan.spec.ts
  ade-v2-archived.spec.ts ade-v2-archive.spec.ts`, and `bun test
  apps/kira-space/tests/unit/ade-v2-timeline.spec.ts
  apps/kira-space/tests/unit/ade-v2-board-parity.spec.ts`. Fast checks per commit through the
  pre-commit hook.

## 6. Docs

- `docs/ARCHITECTURE.md:2323` (ADE, archived tasks bullet): replace "today's archive rows always
  show, past days load with the History toggle" with "archive rows, today's included, show only
  while History (`AdeHistoryControl`) is open". Keep the finder and Restore sentences. Add: picking
  any archived task in the finder opens History on its day.
- No `docs/DEV_ENVIRONMENT.md`, site or README change (site settings page has no archive text).

## 7. Commits

1. `fix: ADE hides tasks archived today until History opens` (`timeline.ts`, `AdePlanView.vue`,
   `ade-v2-plan.spec.ts`, `ade-v2-archived.spec.ts`).
2. `docs: archived tasks show only with History open` (ARCHITECTURE).
3. `docs: P491 result` (SPEC row Done plus Result).

## 8. File ownership (Stream C, P491)

Owned by P491 only. Stream A (P484-P487, `apps/kira-studio` Kubernetes) and Stream B (P489
terminal file links, ownership list in `P489-terminal-file-links.md` section 11; P490 Kira Space
file-open window shell, sidebar, workspace) touch none of them:

- `apps/kira-space/frontend/src/ade/v2/board/timeline.ts`
- `apps/kira-space/frontend/src/ade/v2/plan/AdePlanView.vue`
- `apps/kira-space/tests/ui/ade-v2-plan.spec.ts`
- `apps/kira-space/tests/ui/ade-v2-archived.spec.ts`
- `docs/v0.2.29/plans/P491-hide-archived-until-history.md`

Read, not edited: `AdeHistoryControl.vue`, `AdeArchivedFinder.vue`, `AdeDayBand.vue`,
`AdePanel.vue`, `adeBoardUi.ts`, `tests/ui/support/adeV2.ts`, `tests/unit/ade-v2-timeline.spec.ts`.

Reserved for P492 (Stream C, next; its own plan confirms). P491 must not edit them, so P492 starts
clean on top of P491:

- `apps/kira-space/frontend/src/ade/v2/status/AdeAgentsStatusItem.vue`, `AdeAgentsStatusList.vue`,
  `AdeAgentsStatusRow.vue`, `agentsSummary.ts`
- `packages/workbench/src/components/StatusBarItem.vue` (if the indicator needs a slot or prop
  there; Stream B's P490 window shell must not change this component, flag at P490 planning)
- `apps/kira-space/frontend/src/ade/v2/activity.ts`, `AdeActivityIcon.vue` (read, likely)
- a new or existing status-agents UI spec under `apps/kira-space/tests/ui/`
- `packages/theme/src/i18n/en.json`: P492 may add `space.statusBar.agents.*` keys

Shared, with resolution:

- `docs/v0.2.29/SPEC.md`: each stream edits only its own rows (status cell, Result under its own
  `### P<n>`). On rebase keep both sides' hunks; never rewrite another stream's row.
- `docs/ARCHITECTURE.md`: P491 edits only the ADE archived-tasks bullet (`:2323`). Stream A edits
  Kubernetes sections, Stream B the terminal links bullet and Kira Space sections. A textual
  conflict means adjacent hunks: keep both.
- `packages/theme/src/i18n/en.json`: P491 makes no change. Stream B adds only its own
  `space.terminal`/file-link keys; P492 only `space.statusBar.agents.*`. Conflicts are adjacent
  key insertions: keep both, keep JSON valid, run the i18n check.
- Contracts (`apps/*/tests/contract/*.json`), Wails bindings, `bridge/index.ts`, `ipcChannels.ts`,
  `mockRuntime.ts`, `go.mod`, `package.json`, lockfiles: P491 touches none.
- Stream C runs in the main checkout on `chapter/v0.2.29`. Other streams land by rebase; commit
  only P491's own files and retry on `.git/index.lock`.

## 9. Ordering consequences

None for Streams A and B. P492 follows P491 in table order and shares no file with it.
