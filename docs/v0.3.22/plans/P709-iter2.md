# P709 iteration 2 plan: one ground, task swatch, borderless step action, panel always open

Base: `beeeebe48` (branch `stream/p709`, worktree `/home/user/kira-space-v0.3.22-p709`). Plans
against iteration 1 as landed (`605a98ec3..beeeebe48`). One sequential Sonnet implementer; work is
order-dependent (card ground, swatch and grid share `AdeTaskCard.vue` / `taskGrid.ts`).

User feedback on iteration-1 shots ("looks much better, but"): (1) list ground black like the
board, (2) step button loses its border, (3) better task colour than the left bar; reconsider the
`max-w-4xl` cap, (4) right panel always open.

## 1. Findings (verified in the p709 worktree)

### 1.1 Ground

- Plan pane `AdePlanView.vue:375` is already `bg-chrome` (`--kira-bg-chrome` #131315, ARCHITECTURE
  :2910 "Plan pane ground is bg-chrome (terminal ground)"). Pixel sample of
  `screenshots/p709/after/{wide,narrow}-dark.png`: ground rgb(19,19,21) = chrome; card fill
  rgb(33,33,37) = `--kira-bg-elevated`.
- So the only non-black surface in the list is the card fill: `Card variant="section"` adds
  `bg-elevated` (`packages/theme/src/components/ui/card/index.ts`), `taskGrid.ts` `GROUND_CARD`
  sets `--row-ground` to elevated. Parked cards already sit on `bg-chrome` (`GROUND_PARKED`).
- Reading of ask: "board" = the Plan ground (code and ARCHITECTURE call it the plan board); cards
  drop their elevated fill and sit on that same chrome ground, drawn by a 1px edge. The right panel
  (`paneFrame`, `bg-bg` #1b1b1e) is not black and stays as is.

### 1.2 Step button border

- `AdeStepMenu.vue:62-74` trigger ("Implement 1/6 ⌄") is `variant="toolbar"`: base class
  `border border-transparent` (`ui/button/index.ts:7`). No visible border in before or after shots
  (crop checked). It has no border to lose.
- The only bordered control on the card head is the step action, `AdeTaskActionButton` icon mode
  (`run/AdeTaskActionButton.vue:25-39`, card cell `TASK_ACTION_CELL`): `variant="dialog"
  :outline="action.tone"` = 1px tone border (the red ↻ / green → squares at the head's right).
  That is "the step button" (it runs the step). Card instance drops the border; panel header and
  workflow block keep the labelled outline button.
- Guard U210 (`scripts/check-ui-primitives.sh:392`) requires `variant="dialog"` on every
  `ade-task-action-` button and bans class tone/border overrides; U211 limits `outline` to
  `AdeTaskActionButton.vue`/`TooltipIconButton.vue`. `outline` compound variants exist only for
  `variant: 'dialog'` (`ui/button/index.ts:58-83`). ARCHITECTURE :1426 and :1464 state the rule.
- Spec pin: `tests/ui/ade-v2-action-tone.spec.ts:62-72` expects `border: want` in all three hosts.

### 1.3 Task colour

Where task colour shows today (`taskColor()` / `card.color`):

- Square swatch, `size-2.5 rounded-kira-sm`: review window header (`review/AdeReviewHeader.vue:58`),
  files window header (`files/AdeTaskFilesWindow.vue:31`), All sessions groups
  (`needs/AdeAllSessions.vue:130`), branch tooltip "work-colour dot" (`plan/AdeBranchTip.vue:44`,
  before the branch name, `gap-1.5`).
- Task panel header `ade-panel-dot` (`panel/AdeTaskPanel.vue:157-161`): `size-3 rounded-kira-sm`,
  filled; parked = `border-2` outline in the colour. Branch panel: outline dot
  (`AdeBranchPanel.vue:315-320`).
- Mobile `PlanTaskCard.vue:72` `StatusDot` (dot). Out of scope.
- 3px left edge: plan card (`AdeTaskCard.vue:127,147`), span row (`AdeDayBand.vue:217-218`), Later
  shelf row (`later/AdeShelfRow.vue:40-42`).

History: P601 (docs/history/06-v2.8.md:1308) removed a colour square that sat in the lead slot, to
give the lead to the agent indicator. Spec pins `ade-card-dot` / `ade-shelf-dot` count 0. The new
swatch does not go back into the lead.

### 1.4 Panel open/close

- Only open/close switch: `shell/AdeShell.vue:184` `<AdePanel v-if="ui.selectedTaskId" />`. Later
  tab: `:180` `<AdePanel v-if="shelfSelected" />` (Later page, unchanged).
- Selection: Pinia `state/adeBoardUi.ts` `selectedTaskId` / `selectedBranchId`, in memory only,
  starts `null`. No persisted open state, no close button, no shortcut, no Escape deselect
  (grep `select(null)`: only `workflows/AdeWorkflowGraph.vue:231`, unrelated draft store). Later
  page auto-selects its first shelf task (`later/AdeLaterPage.vue:31-40`): the precedent.
- `panel/AdePanel.vue` already renders `ade-panel-empty` (`EmptyState`, "Select a task to see its
  details.") when the selection has no card and is not archived; renders `AdeArchivedPanel` for an
  archived selection (History).
- Width: `panel/AdePanelPane.vue` `LIST_DEFAULT = 400` (stored width 0 = list 400px, panel takes
  the rest), `MIN_WIDTH = 340` (panel), `MIN_PAGE = 360` (list), `panel.ade.width` persisted in
  `state/adeLayout.ts`. So with the panel always present the list is 400px by default, wider only
  when the user drags the splitter.
- Deselect paths: none in Plan UI. Archive keeps the id (History panel). Delete leaves an id with
  no card and no history entry: today an empty panel; after this change, auto-select.

### 1.5 `max-w-4xl`

Iteration 1 capped plan content (`AdePlanView.vue:415`) for the panel-less wide pane. With the
panel always beside the list, list width is the user's splitter choice; a cap leaves a dead strip
between list and panel (the 2000px shot's empty right strip). Drop it.

## 2. Design

### 2.1 One ground

Card root (`AdeTaskCard.vue` `boxClass`), parked or not:

- `bg-chrome border cursor-grab` plus `GROUND_CARD` (now chrome, below).
- Edge colour: selected `border-focus`; else in selection ripple `border-subtle/50`; else
  `border-border`. Replaces the ripple `outline-1 outline-border` (an outline on top of a border
  edge reads as a 2px line; with the panel always open a ripple is always on screen).
- Parked: `border-dashed` (shape cue now that the fill no longer differs); title stays
  `text-muted-foreground` (`AdeTaskTitle.vue:53`).
- No `border-l-3`, no inline `borderLeftColor`.

`taskGrid.ts`:

- `GROUND_CARD = '[--row-ground:var(--color-chrome)]'`; delete `GROUND_PARKED` (same ground now).
- Add `TASK_SWATCH = 'box-border size-2.5 shrink-0 rounded-kira-sm'` and
  `TASK_SWATCH_GAP = 'w-2.5 shrink-0'` (spacer under or in place of a swatch, keeps the title x).

Surfaces on chrome (unchanged tokens): head hover `bg-field` (#27272c), selected `bg-select`
(#1f3354), card edge `border` (#323237, "the one line"), repo badge `/12` tint, weekend stripes
(`var(--color-elevated)` behind cards, cards opaque). Check each in shots.

### 2.2 Task swatch beside the title

Pick: a 10px rounded square in the task colour directly before the title, same element and size as
the review/files headers, All sessions and the branch tooltip; panel header keeps its 12px
variant. Why: one task-colour shape everywhere; small and calm (10px of colour per task instead of
a full-height bar); next to the name, so it reads as the task's identity; lead slot stays the agent
indicator (P601). Colour-blind safety: the swatch is redundant identity (title and ID carry the
task); parked uses a shape cue (outline swatch, dashed card edge), never hue alone.

Markup (`:style` binding, as every other host does):

- Filled: `<span :class="TASK_SWATCH" :style="{ background: card.color }" data-testid="ade-task-swatch" aria-hidden="true" />`
- Parked: `:class="[TASK_SWATCH, 'border-2']" :style="{ borderColor: card.color }"` (as panel).

Hosts:

1. Card head line 1 (`AdeTaskCard.vue:175`): swatch first child, before the title cell. Line 2
   (`:232`): `<span :class="TASK_SWATCH_GAP" aria-hidden="true" />` first child, so step cell
   stays under the title. Line 1 last child stays the action cell (card-icons spec, iteration-1
   spec `line1.lastElementChild`).
2. Span row (`AdeDayBand.vue:212-229`): class `h-row w-auto justify-start gap-1.5 border
   border-border px-2 font-normal data-[selected]:bg-select` (drop `border-l-3`, drop `:style`);
   swatch (filled, `s.color`) between icon and title.
3. History row (`AdeDayBand.vue:151-171`): `border border-transparent` (drop `border-l-3`);
   `TASK_SWATCH_GAP` spacer between icon and title (archived: no colour in `h`; no fake swatch).
4. Review card (head-less, `card.review`): `AdeBranchRow.vue` board layout col 2, first child
   inside the `AdeBranchTip` span, before the owner pill: `<span v-if="taskRow && color"
   :class="TASK_SWATCH" :style="{ background: color }" data-testid="ade-task-swatch"
   aria-hidden="true" />`. Panel layout untouched.
5. Later shelf row (`later/AdeShelfRow.vue:36-56`): drop `border-l-3` and `borderLeftColor` from
   class/style (keep `rowIndent(0)`); swatch (filled, or parked outline when `card.parked`)
   between the lead and `ade-shelf-title`. Keeps the lead first child (spec).

Title x check: card 1 (edge) + 8 (`px-2`) + 24 (lead) + 6 + 10 + 6 = 55px; span 1 + 8 + 24 (13px
icon `mx-[5.5px]`) + 6 + 10 + 6 = 55; history same with spacer. Iteration 1 used a 3px edge: all
three rows move 2px left together.

### 2.3 Borderless step action on the card

- `ui/button/index.ts` compoundVariants: add five `variant: 'toolbar'` entries, one per outline
  tone: `agent` `text-agent hover:text-agent focus-visible:outline-agent`, `green`
  `text-status-green hover:text-status-green focus-visible:outline-status-green`, `amber`
  (`status-amber`), `red` `text-error hover:text-error focus-visible:outline-error`, `purple`
  (`status-violet`). No border class: toolbar keeps `border-transparent`, `hover:bg-field`. Update
  the `outline` comment: dialog draws border + text, toolbar text only.
- `AdeTaskActionButton.vue` icon branch: `variant="toolbar"` (keep `:outline="action.tone"`,
  `size="kira-icon"`, `class="shrink-0"`). Labelled branch unchanged (`dialog`).
- U210 regex: `(?!(?&t)\svariant="dialog")` becomes `(?!(?&t)\svariant="(?:dialog|toolbar)")`;
  message `task step action without its tone (variant="dialog" or icon variant="toolbar" with
  :outline="action.tone", no class tone or bound class, P692, P709)`.
- ARCHITECTURE :1426 and :1464: the plan card icon is toolbar with the tone on the glyph (no
  border); panel header and workflow block keep the tone outline.

### 2.4 Panel always open

- `AdeShell.vue:184`: `<AdePanel />` (no `v-if`). Template comment `:32` "the Plan keeps the panel
  open beside it".
- Auto-select, in `AdePlanView.vue` script (Plan-only, mirrors `AdeLaterPage`), `history` from
  `usePlanModel()` (same source `AdePanel` uses for the archived case):

  ```ts
  // The panel never closes: with no selection, or one gone from both board and History, the top
  // Plan task takes it.
  const fallbackId = computed(() => {
    const m = model.value;
    if (!m) return null;
    const id = ui.selectedTaskId;
    if (id && (m.cardFor(id) || history.value.some((h) => h.taskId === id))) return null;
    return m.view.bands.flatMap((b) => b.taskIds)[0] ?? null;
  });
  watch(fallbackId, (id) => { if (id) ui.select(id); }, { immediate: true });
  ```

  Keeps every explicit selection (shelf task, archived task, branch). Never calls `select(null)`.
- `AdePanel.vue` template order: `model && card` (task/branch) > `archivedId` > `!model` loading
  (`EmptyState fill loading :title="t('space.ade.plan.view.loading.loading')"`) > empty
  (`EmptyState fill icon="tasklist" :title="t('space.ade.panel.empty.emptyHint')"
  :description="t('space.ade.panel.empty.hint')" data-testid="ade-panel-empty"`). Empty only
  shows when the Plan has no task.
- `packages/theme/src/i18n/en.json` `space.ade.panel.empty`: `emptyHint` "No tasks on the Plan
  yet."; add `hint` "Type one in the capture box above, or use Add." Keep the catalog checks green
  (`catalog_test.go`, lint i18n check).
- No close control exists, none added. No shortcut changes. Later page keeps its own
  `shelfSelected` panel rule (not the Plan list). Mobile untouched.
- Ripple (`usePlanModel.ts:394`) now always has a selection; calmer ripple edge per §2.1.

### 2.5 Width

- `AdePlanView.vue:415`: `px-3 pb-4 pt-3` (drop `max-w-4xl`).
- Keep `LIST_DEFAULT = 400` (spec-pinned 360-400px list, user-resizable, persisted). The list runs
  the existing narrow grid (`@max-lg`, step percent hidden) by default; the wide grid appears when
  the user drags the list past `@lg` (512px).

## 3. Work items (in order)

0. Before shots (§6) on untouched `beeeebe48`, fresh `bun run build:test`.
1. `taskGrid.ts` (§2.1 constants), `AdeTaskCard.vue` ground, edge, ripple, parked, swatch +
   line-2 spacer (§2.1, §2.2.1). `bun run typecheck:web && bun run lint`. Commit
   `feat(ade): plan cards on the plan ground with a task swatch`.
2. `AdeDayBand.vue` span/history rows, `AdeBranchRow.vue` review swatch, `AdeShelfRow.vue`
   (§2.2.2-5). Same checks. Commit `feat(ade): task swatch on span, history, review and shelf rows`.
3. Button toolbar tone compounds, `AdeTaskActionButton.vue` icon variant, U210 (§2.3).
   `bash scripts/check-ui-primitives.sh` plus the checks above. Commit
   `feat(ade): borderless step action on plan cards`.
4. `AdeShell.vue`, `AdePlanView.vue` auto-select and cap removal, `AdePanel.vue`, `en.json`
   (§2.4, §2.5). Commit `feat(ade): task panel always open beside the plan`.
5. Test helpers and specs (§4). Run the UI set (§5). Fix, commit `test(ade): ...` per group.
6. Re-record visual baselines (§5). Inspect each PNG by eye. Commit
   `test(ade): re-record plan visual baselines`.
7. After shots (§6), compare with before; fix and loop 5-7 on any misalignment or clipping.
8. `docs/ARCHITECTURE.md`: :2910 colour line (task colour is a 10px swatch before the title on
   cards, span, review and shelf rows; history rows keep the slot; cards sit on the plan ground
   with a 1px `border` edge, dashed when parked, `border-subtle/50` in the ripple; the panel
   `ade-panel-dot` stays 12px); :2911 P601 note the swatch is outside the lead; :2912 P709 bullet
   drop "Plan content caps at max-w-4xl", add "The task panel is always open beside the Plan: no
   selection or a vanished one falls back to the top Plan task; no task shows `ade-panel-empty`";
   :1426/:1464 per §2.3. Commit `docs: P709 iteration 2`.
9. `docs/v0.3.22/SPEC.md` `## P709` Result: append an "Iteration 2:" paragraph (ground, swatch,
   borderless card step action, panel always open with fallback and empty state, cap removed,
   baselines and specs changed, shot dirs). Row stays Done. Commit.

## 4. Tests

No unit test (one-rule fallback, layout). No flow half: no IPC or backend change (same reason as
`ade-v2-action-tone.spec.ts` header).

Helpers (`tests/ui/support/adeV2.ts`): add `selectTask(page, id)`: click the card head, then
`await expect(page.locator(\`[data-testid="ade-card"][data-task-id="${id}"]\`)).toHaveAttribute('aria-current', 'true')`.
Use it in `openTask` (:487), `openBillTask` (:394), `openDetails` (:495) and `openSessions` (:321)
where they click a head, and in the local `openTask` of `ade-v2-panel.spec.ts:23`. Reason: the panel
now already shows the fallback task, so "panel visible" / "main tab visible" no longer proves the
switch.

New spec `tests/ui/ade-v2-panel-open.spec.ts` (frontend half only):

- `the plan opens with the top task in the panel`: `openPlan`; first `ade-card` in DOM has
  `aria-current="true"`; `ade-panel` visible; `ade-panel-title` placeholder matches that task's
  title/key; `ade-panel-empty` count 0.
- `the panel has no close control and stays after every switch`: no element in `ade-panel` with
  `aria-label` / testid matching `/close/i`; select `T_bill`, a branch row (`b_cart`), `ade-panel-back`,
  Later tab and back to Plan: `ade-panel` visible throughout on the Plan tab.
- `a selected task that leaves the board falls back to the top task`: select `T_bill`;
  `control.respond(IPC.adeTaskBoard, boardWithout('T_bill'))` (drop task, its branches, plan
  entries), `emitWailsEvent(page, IPC.adeTaskBoardChanged, null)`; top card gets `aria-current`,
  panel shows it.
- `an empty plan shows the empty panel`: board with `tasks`, `branches`, `pairs`, `history`
  emptied and `plan.day`/`plan.order` (and other per-task maps in `plan`) emptied (verify against
  `tests/fixtures/ade-v2/board.json`); `ade-panel` visible, `ade-panel-empty` has the new title and
  hint text.

Extend iteration-1 spec (`ade-v2-task-row.spec.ts:336`): case `the full-width list` becomes `the
widened list`: `openPlan` with `{ channel: IPC.layoutGetAll, response: { ...defaultLayout, panel:
{ ...defaultLayout.panel, ade: { width: 340 } } } }` at 1440x1000 (list past `@lg`); both cases
add: every head's `ade-task-swatch` has the same x (±1) and its right + 6 = `ade-card-title` x
(±1); every `ade-span-row` title starts at the card title x (±1).

Update (placement or look the feedback replaces; keep test ids; record each in the commit body):

- `ade-v2-plan.spec.ts:386` card box test: name `a card is a 6px rounded box with a 1px edge on the
  plan ground and a task swatch, 16px from its same-day neighbour`; expect radius 6px, top 1px,
  left 1px; `bg` equals plan bg (one ground); `ade-task-swatch` background non-transparent.
- `ade-v2-plan.spec.ts:436` marked test: unselected `T_search` `borderTopColor` = `--kira-border`
  token (was transparent).
- `ade-v2-plan.spec.ts:534` colour test: name says swatch instead of edge.
- `ade-v2-task-row.spec.ts:151` `a task card keeps a 3px task-colour left edge and no fill`
  becomes `a task card wears its colour as a swatch beside the title, on the plan ground`: every
  card border widths 1px, no inline `borderLeftColor`, card bg = plan bg, every head card has one
  `ade-task-swatch` with inline `background` or (parked) `borderColor`, swatch not inside
  `ade-row-lead`.
- `ade-v2-task-row.spec.ts:28` name `... and the task colour sits beside the title`; keep
  `ade-card-dot` 0; add `ade-row-lead ade-task-swatch` count 0. Shelf test `:119`: keep
  `ade-shelf-dot` 0 and lead first; add one `ade-task-swatch` in the row.
- `ade-v2-action-tone.spec.ts:62-72`: card host `{ border: 'rgba(0, 0, 0, 0)', text: want }`;
  panel and block hosts unchanged.
- Any other failure: root-cause first; fix product or helper when it is a real regression; change
  an assertion only when it pins no-panel, colour-edge, card-fill or card-step-border behaviour.
  Never loosen focus, selection, height or ring assertions.

## 5. Verification

From `/home/user/kira-space-v0.3.22-p709`:

```sh
bun run typecheck:web && bun run lint && bash scripts/check-ui-primitives.sh
bun run build:test
# The panel change touches every Plan spec: run all ADE ui specs once near the end.
bunx playwright test --config=apps/kira-space/playwright.config.ts --project=ui \
  ade-v2 consistency panel-resize
fc-match sans-serif   # must print Inter (DEV_ENVIRONMENT "tests/visual/* pixel diffs")
bunx playwright test --config=apps/kira-space/playwright.config.ts --project=visual \
  ade-plan-branches ade-plan-days --update-snapshots
bunx playwright test --config=apps/kira-space/playwright.config.ts --project=visual
```

Baselines expected to change (list now 400px beside the auto-opened panel at 1400px, plus ground,
swatch, edge): `plan-branches-stacked`, `plan-branches-review`, `plan-branches-pending`,
`plan-day-bars`. Expected unchanged: `plan-branch-tip`, `plan-branch-plain-tip`,
`branch-panel-details`, `ade-links`, `ade-links-card`, all other visual baselines; if one changes,
find why before re-recording it (`--update-snapshots <spec>` only for that spec). Full `visual`
run passes after.

Real checks before done: grep `TASK_SWATCH` in `AdeTaskCard.vue`, `AdeDayBand.vue`,
`AdeBranchRow.vue`, `AdeShelfRow.vue`; no `border-l-3` / `borderLeftColor` left in those four;
no `max-w-4xl` in `AdePlanView.vue`; no `v-if` on the Plan `<AdePanel />` in `AdeShell.vue`;
computed `border-top-color` of the card step action is transparent and its `color` is the tone;
`perf/heap-ram.spec.ts` (`ade-panel` refs :978, :1081) still reads right (not run locally; check
by reading).

## 6. Screenshot procedure (implementer; not committed)

Output `screenshots/p709-iter2/before/` and `.../after/` (gitignored `screenshots/*`, never
`git add`; never beside a UI suite in the same checkout). Temporary
`apps/kira-space/tests/screenshots/p709-iter2.shot.ts`, deleted after step 7 (`git status`
clean of it):

```ts
import { defaultLayout } from '@shared/domain/layout';
import { test } from '../ui/fixtures';
import { adeBoard, openPlan } from '../ui/support/adeV2';
import { IPC } from '../ui/support/ipcChannels';

const OUT = process.env.P709_OUT ?? 'screenshots/p709-iter2/after';
const board = adeBoard((b) => {
  const set = (id: string, f: Record<string, unknown>) =>
    Object.assign(b.tasks.find((t) => t.id === id) ?? {}, f);
  set('T_cart', { title: 'Cart total test fails intermittently when the promo code and the gift card are applied in the same checkout session on Safari' });
  set('T_notif', { title: 'Fix', est: '' });
  set('T_deps', { workflowId: '', entryStepId: '' });
});
const split = { ...defaultLayout, panel: { ...defaultLayout.panel, ade: { width: 520 } } };
for (const [name, w, h, layout] of [
  ['wide', 2000, 1250, null],
  ['wide-split', 2000, 1250, split],
  ['narrow', 1280, 900, null],
] as const) {
  test(`p709 iter2 ${name}`, async ({ relaunch }) => {
    const extra = [{ channel: IPC.adeTaskBoard, response: board }];
    if (layout) extra.push({ channel: IPC.layoutGetAll, response: layout });
    const { window: page } = await openPlan(relaunch, extra);
    await page.setViewportSize({ width: w, height: h });
    await page.locator('[data-testid="ade-load-all"]').click();
    const shell = page.locator('[data-testid="ade-shell"]');
    const plan = page.locator('[data-testid="ade-plan"]');
    await shell.screenshot({ path: `${OUT}/${name}-dark.png` });
    await page.locator('[data-testid="ade-card"][data-task-id="T_auth"] [data-testid="ade-card-head"]').hover();
    await plan.screenshot({ path: `${OUT}/${name}-hover-dark.png` });
    await page.locator('[data-testid="ade-branch-row"][data-branch-id="b_auth"]').hover();
    await plan.screenshot({ path: `${OUT}/${name}-rowhover-dark.png` });
    await plan.evaluate((el) => el.scrollTo(0, el.scrollHeight));
    await plan.screenshot({ path: `${OUT}/${name}-end-dark.png` });
  });
}
test('p709 iter2 empty', async ({ relaunch }) => {
  // Same empty board as the ade-v2-panel-open empty spec.
});
```

Fill the `empty` test with the empty board from §4 (after side only: before has no panel). Type
`extra` as the helper's `ControlSnapshot[]` if TS complains. `wide-split` on the before tree shows
the same as `wide` (no panel until a click): expected, it documents the change.

```sh
bun run build:test
P709_OUT=screenshots/p709-iter2/before bunx playwright test \
  --config=apps/kira-space/playwright.config.ts --project=screenshots p709-iter2   # step 0
P709_OUT=screenshots/p709-iter2/after bunx playwright test \
  --config=apps/kira-space/playwright.config.ts --project=screenshots p709-iter2   # step 7
```

Dark only (no light theme, ARCHITECTURE Known open item). Open each pair with Read: ground one
colour across list and cards; swatch at one x; card step action borderless in its tone; panel
present in every after shot; no dead strip at 2000px. Report both dirs in Result and final message.

## 7. Risks

- Reading of (1): if the user meant something other than the card fill (plan ground is already
  chrome), the change is local to `boxClass` / `GROUND_CARD`; say the evidence (pixel samples) in
  the Result so the user can redirect.
- Reading of (2): the step menu trigger has no border; the plan removes the border of the step
  action. Name this in the Result.
- Card edge `#323237` on `#131315` is low contrast; cards still separate by 16px gaps and the edge.
  If shots show cards dissolving, say so in the Result; never add a new token or literal here.
- tailwind-merge must drop `hover:text-fg` for `hover:text-<tone>` and `bg-elevated` for
  `bg-chrome` (custom names, default `isAny` colour matcher; parked cards already rely on the
  latter). Confirm with computed styles (§5).
- Auto-select fires panel queries (artifacts, issue, sessions) for the top task on every Plan load;
  a spec counting such IPC calls may see one more. Fix the count expectation only after confirming
  the extra call is the fallback task's.
- Panel always open: windows near 700px wide squeeze the list below 360px (`MIN_WIDTH` 340 +
  `MIN_PAGE` 360). Previously only with a selection; now always. Accepted; the narrow grid
  truncates.
- Helper hardening may miss local card-click helpers in other specs; the full `ade-v2` run finds
  them.
- `en.json` key change: run the i18n catalog check; Go `catalog_test.go` must stay green.
- Later page shares `AdeEstimateChip`, `AdeTaskTitle`, `AdeTaskStatusMenu` (unchanged); only
  `AdeShelfRow.vue` changes; shelf tests in `ade-v2-task-row.spec.ts:119` cover it.
