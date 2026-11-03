# P500 plan: task card icon row

Row: `SPEC.md` P500. Frontend only, IPC unchanged, so no backend flow test. Paths are under
`apps/kira-space/frontend/src/ade/v2/` unless full. Base: tip `2b86256c` (P495, P498, P499 landed).
P496 and P497 run before P500 (table order); build on their commits.

## Today (verified with `codegraph_explore`, then grep for spec usage)

- `plan/AdeTaskCard.vue`: outer `div[data-testid=ade-task]` holds a left column
  `div.w-52.5[data-testid=ade-task-cells]`, then the `Card` (`ade-card`). The column renders one
  `AdeActionCell` for the task (`card.tag` + `<AdeTaskActionButton quiet>`,
  `@contextmenu=taskMenu.open`) and one per branch row (`row.tag`, `row.tag.actions`,
  `@act=act(row,a)`, `@contextmenu` = task menu only for a review card). `act()` (lines 63-81)
  dispatches `BranchAction` kinds: `act` -> `dialogs.act`, `forcePush` -> emit, `seeError` /
  `seeLog` -> select branch + details tab + focus flag, `takeOver` -> `takeOver.request`,
  `start` -> `dialogs.start`.
- Header line (`ade-card-head`, first flex row): colour dot, step segments, step label, bar,
  `AdeTaskSessions`, `AdeAttention`, `AdeAutomationChip`, meta (`flex-1`), change-base icon,
  review icon (`ade-card-review`). Review cards (`card.review`) have no head.
- `plan/AdeActionCell.vue`: tag as `AdeChip` (`data-testid=ade-tag`, `data-tone`) wrapped in
  `AdeBranchTip` (branch) or `AdeTip` (task); action `Button`s with testids from `TESTID`
  (`ade-force-push`, `ade-rebase`, `ade-queue-after`, `ade-change-base`, `ade-abort-rebase`,
  `ade-see-log`, `ade-take-over`, `ade-see-error`, `ade-start`). Sole caller: `AdeTaskCard`.
- `plan/AdeBranchRow.vue`: left part in `AdeBranchTip`, then `AdeAttention`, then review icon
  `ade-branch-review` with `rowActions` (hover-only: `opacity-0 group-hover:opacity-100`).
  `onMenu` builds the row menu (label, review code, automation submenu, `fixItems`) and calls
  `contextMenu.openContextMenu(ev, …)`; a review card row emits `taskMenu` instead.
- `board/actions.ts` (P495 added `resume`): `TaskCell.tag {label, tone?, color?, tip}`;
  `taskCell` yields four tag states (status, blocked, merged, not merging). `BranchTag {label,
  tone, tip, tipParts?, actions, since?}` from a first-match rule table (`BRANCH_RULES`, 21 rules)
  built by helper `tag(label, tone, tip, actions)`. `branchTag` appends `start`. No rule carries
  an identity; only the label distinguishes states.
- `menu.ts` `fixItems` already lists every rebase act (same `rebaseActs` source as `actsOf`) and
  force push. Not in it: `seeError`, `seeLog`, `takeOver`, `start`.
- `run/AdeTaskActionButton.vue`: text `Button`, testid `ade-task-action-<kind>`; callers
  `AdeTaskCard` (quiet), `panel/AdeTaskPanel.vue`, `panel/AdeWorkflowBlock.vue`.
- Other readers of tags (stay as text chips, untouched): `AdeBranchTip`, `panel/AdeTaskTab.vue`,
  `panel/AdeBranchPanel.vue`. Mobile `mobile/state/useAgentsModel.ts` calls `taskCell`; unit
  `tests/unit/ade-v2-board-parity.spec.ts` compares `label` / `actions[].label` only. Both stay
  green with additive fields.
- `useTaskMenu` (P495) stays the only task-menu source; P500 adds no task-menu entry.
- `common.moreActions.tooltip` ("More actions") exists in `en.json`. Every codicon named below
  exists in `node_modules/@vscode/codicons/dist/codicon.css` (checked), except where marked
  "verify".

## Design

### 1. State identity: `board/actions.ts`

- `TaskCell.tag` gains `kind: TaskTagKind`, `type TaskTagKind = 'status' | 'blocked' | 'merged'
  | 'notMerging'`. Set in `taskCell`'s three return sites.
- `BranchTag` gains `kind: BranchTagKind`. `tag()` helper takes `kind` first:
  `tag(kind, label, tone, tip, actions)`. Kinds, one per rule: `notCreated`, `preparing`,
  `setupFailed`, `merged`, `parked`, `review`, `reviewConflict` (the `review` rule's conflict
  branch), `rebasing`, `rebasePending`, `rebaseFailed` (`rebaseRunFailed`, both stuck and failed),
  `rebaseInProgress`, `baseMissing`, `conflict`, `rebaseConflicts`, `notPushed`, `behind`,
  `sharesFiles`, `waiting`, `baseAttention`, `checking`, `checkFailed`, `clean`. `branchTag`'s
  fallback `tag(...)` uses `clean`.
- No glyph or class in `actions.ts`: it is shared with mobile, glyphs are a desktop view concern.
- Update the header comment ("Left action column: …") to "Card tag and actions: …".

### 2. Glyph tables: `plan/glyphs.ts` (new)

```ts
export interface Glyph { icon: string; urgent?: boolean; spin?: boolean }
export const TASK_TAG_GLYPH: Record<TaskTagKind, Glyph>
export const BRANCH_TAG_GLYPH: Record<BranchTagKind, Glyph>
export const TASK_ACTION_ICON: Record<TaskAction['kind'], string>
export const BRANCH_ACTION_ICON: Record<BranchAction['kind'], string>
export function rowActionSplit(actions: readonly BranchAction[]): { shown: BranchAction[]; overflow: BranchAction[] }
```

Export `TaskTagKind`, `BranchTagKind`, `TaskActionKind`, `BranchActionKind` from `actions.ts` as
needed (types only). `Record<…>` keeps the tables exhaustive at typecheck.

Task tag: `status` circle-filled (coloured by status colour), `blocked` error (urgent),
`merged` git-merge, `notMerging` circle-slash.

Branch tag (each state its own glyph; same-meaning conflict kinds share one):

| kind | icon | urgent |
|---|---|---|
| notCreated | circle-large-outline | |
| preparing | gear, spin | |
| setupFailed | error | yes |
| merged | git-merge | |
| parked | circle-slash | |
| review | eye | |
| reviewConflict, conflict, rebaseConflicts | flame | yes |
| rebasing | sync, spin | yes |
| rebasePending | history | yes |
| rebaseFailed | sync-ignored | yes |
| rebaseInProgress | debug-pause | yes |
| baseMissing | debug-disconnect (verify) | yes |
| notPushed | cloud-upload | |
| behind | arrow-down | |
| sharesFiles | files | |
| waiting | watch | |
| baseAttention | warning | |
| checking | loading, spin | |
| checkFailed | question | |
| clean | check | |

Task action: `run` play, `session` debug-start, `resume` debug-continue, `advance` arrow-right,
`finish` pass, `takeOver` person, `approve` check, `retry` debug-restart, `archive` archive.
Branch action: `forcePush` repo-force-push, `rebase` repo-sync, `queue` list-ordered,
`changeBase` git-branch, `abortRebase` debug-stop, `seeLog` output, `takeOver` person,
`seeError` bug, `start` debug-start.

"verify": grep `codicon-<name>:before` in the codicon CSS; pick another unused glyph if missing,
keeping one glyph per state.

`rowActionSplit`: `actions.length <= 2` -> all shown, no overflow. Else first action shown, rest
overflow. So a row's status cluster is at most 3 icons: status + 2 actions, or status + 1 action
+ More. Trivial slice, no unit test.

### 3. Status glyph component: `plan/AdeTagIcon.vue` (new)

Props: `glyph: Glyph`, `label: string` (already `tagLabel`-resolved), `tip: string`,
`tipParts?`, `tone?: Tone`, `color?: StatusColor`, `kind: string`.

- Renders `AdeTip` (text = `label`, then tip; use `parts` as `[label, '\n', …tipParts]` or a
  `<template>` with VarText, whichever `AdeTip` supports cleanly) around a
  `span role="img" :aria-label="label"` holding `CodiconIcon :name="glyph.icon" :size="13"`.
- Attributes: `data-testid="ade-tag"` (kept, specs already scope by it), `:data-kind`,
  `:data-tone` (tone, or omitted when `color`), `:data-color`, `:data-icon`,
  `:data-urgent="glyph.urgent || undefined"`.
- Classes (Tailwind only): non-urgent -> `TONE_TEXT_CLASS[tone]` or status text colour;
  urgent -> `size-5 rounded-full inline-flex items-center justify-center` + `TONE_SOLID_CLASS[tone]`
  (solid disc: louder than the plain glyphs). Spin -> `motion-safe:animate-spin` on the icon.
- Status colour text: add `STATUS_TEXT_CLASS: Record<StatusColor, string>` (`text-status-<c>`
  literals) to `statusColors.ts`; no such map exists today.
- Never hover-only: no `rowActions`, no opacity tricks.

### 4. Task action as icon: `run/AdeTaskActionButton.vue`

Add prop `icon?: boolean`. When set, render `TooltipIconButton`:
`:icon="TASK_ACTION_ICON[action.kind]"`, `:label="action.label"`, `variant` = `dialog-danger`
for red tone else `dialog`, `size="kira-icon"`, `:pending="busy"`, same testid
`ade-task-action-${kind}`, `@click.stop="perform"`. Tooltip slot: label, then `action.tip` on a
second line. Text mode (panel, workflow block) unchanged. Drop the `quiet` prop if no caller
needs it after the card switches to `icon` (grep; panel callers pass none today).

### 5. Card: `plan/AdeTaskCard.vue`

- Delete the `ade-task-cells` column and the `AdeActionCell` import. Keep the outer
  `div[data-testid=ade-task]` (many specs scope by it) holding only the `Card`.
- Header line: insert `<AdeTagIcon v-if="card.tag" …>` right after the colour dot
  (`liveNow` not needed: task tags have no `since`). Append
  `<AdeTaskActionButton :card="card" icon />` as the last item of the line, after the review icon.
- The `Card` already has `@contextmenu="taskMenu.open"`; the header icons sit inside it, so a
  right-click on them opens the task menu. `AdeTaskActionButton` keeps `.stop` on click only, not
  on contextmenu.
- Move `act()` out (section 6). Remove now-unused `tabs`, `takeOver`, `errorText` imports.
- Widths: the card gains the column's 210 px. Header row may now overflow less; keep meta
  `flex-1 truncate`. Height unchanged (`h-17` head).

### 6. Branch row: `plan/AdeBranchRow.vue`

- Right edge, after `AdeAttention`: `AdeTagIcon` for `row.tag` (label via `tagLabel(row.tag,
  liveNow)`), then the `shown` actions as `TooltipIconButton`s
  (`BRANCH_ACTION_ICON[a.kind]`, `label=a.label`, tooltip slot `VarText :parts="a.tipParts ??
  [a.tip]"`, `disabled-trigger`, `:disabled`, `variant` `dialog-danger` for `queue`,
  `abortRebase`, `takeOver`, `seeError` (same split as today's `ACTION_VARIANT`) else `dialog`,
  testid kept from today's `TESTID` map, `@click.stop="act(a)"`).
- If `overflow.length`, a More button: P497's `packages/workbench/src/components/MenuIconButton.vue`
  with `icon="kebab-vertical"`, `:label="t('common.moreActions.tooltip')"`,
  `:items="menuItems"`, `data-testid="ade-branch-more"`, `@click.stop` (the row's own click
  selects; More must not). Confirm P497 landed it first (`git log -- packages/workbench/src/components/MenuIconButton.vue`).
- Move `act(a)` here from the card (same body, using `props.row`; `forcePush` -> `emit('forcePush')`).
- Split `onMenu`: `menuItems(): MenuItem[]` builds today's list; `onMenu(ev)` keeps the review-row
  branch, else `preventDefault`/`stopPropagation` + `openContextMenu(ev, menuItems())`.
  `menuItems` appends overflow actions whose kind is `seeError`, `seeLog`, `takeOver`, `start`
  (ids `ade-row-action-<kind>`, `label`, `hint: a.tipParts`, `disabled`, `run: () => act(a)`)
  after the label row. Rebase acts and force push are already there via `fixItems`; do not add
  them twice. Verify by reading `rebaseActs` once: every `actsOf` id comes from it.
- Review icon `ade-branch-review`: drop `rowActions` (`:class="selected ? … : rowActions"`), so it
  is always visible. Remove the `rowActions` import if unused.
- Right-click on any right-edge icon bubbles to the row's `@contextmenu="onMenu"` (row menu; for a
  review card row, the task menu via `taskMenu` emit). That is the "context-menu handler moves onto
  the icons" ask; no per-icon handler needed.
- `AdeTaskCard` no longer passes `@act`; it still listens to `forcePush` and `taskMenu`.

### 7. Delete `plan/AdeActionCell.vue`

Sole caller gone. `knip` (`bun run lint:dead`) must stay clean.

### Library fit

Tailwind classes only, no scoped style. Primitives: `TooltipIconButton` (shadcn `Button` +
`Tooltip`), `AdeTip`, `CodiconIcon`, P497's `MenuIconButton` over the `contextMenu` Pinia store.
No new store, no query, no VueUse need. All components `<script setup lang="ts">`.

### i18n

No new key: labels reuse each tag's and action's existing label; More reuses
`common.moreActions.tooltip`. No key becomes unused (labels still render as aria-label/tooltip and
in panel chips). So P500 does not touch `en.json`.

## Tests

Frontend only; IPC unchanged, so no backend flow. One new UI spec plus updates.

### New: `apps/kira-space/tests/ui/ade-v2-card-icons.spec.ts`

On the default ADE board fixture (`openPlan`), plus targeted board edits like
`ade-v2-base-rebase.spec.ts` does:
1. No `[data-testid="ade-task-cells"]` and no `ade-action-cell` anywhere.
2. Task card head holds `ade-tag` (`data-kind=status`, `aria-label` = status name) before
   `ade-card-meta`, and `ade-task-action-<kind>` is the head line's last button; it is icon-only
   (no text content) and hovering shows the label in the tooltip. Clicking `ade-task-action-session`
   on `T_auth` opens the session dialog (same assertion as `ade-v2-launch`).
3. Branch rows: each row has exactly one `ade-tag`; across the fixture, distinct `data-kind` map to
   distinct `data-icon` (except the shared conflict family). A setup-failed row (`T_search`) and a
   conflict row carry `data-urgent` and a `-solid` class; a clean row has neither.
4. A branch whose last rebase run failed (inject a failed `rebase` run as in
   `ade-v2-base-rebase.spec.ts`) shows at most 3 status-cluster icons: `ade-tag`, one action,
   `ade-branch-more`. Clicking More opens the context menu with the overflow items
   (`ade-row-action-seeLog`, `ade-row-action-takeOver`, and the rebase acts via `ade-fix-*`).
   Clicking `ade-row-action-seeLog` opens branch details with the rebase log focused.
5. Always visible: with the mouse parked off the card, the row's `ade-tag`, action icons and
   `ade-branch-review` have computed `opacity` `1` and `toBeVisible()`.
6. Right-click on the task `ade-tag` opens the task menu (`menu-item-ade-task-action` present);
   right-click on a branch row's `ade-tag` opens the row menu (`ade-fix-…` or `ade-review-code`
   present, no task items).

### Update (same pass)

- `ade-v2-plan.spec.ts`: line 89 selector -> `[data-testid="ade-branch-row"] [data-testid="ade-tag"]`,
  text via `aria-label`; line 174 `allInnerTexts` -> `aria-label` list; line 483 rewrite: inside
  `ade-card`, `-solid` classes appear only on `[data-urgent]` tags and `ade-attention`.
- `ade-v2-panel.spec.ts` line 419: preparing tag ticks -> poll `aria-label`. Line 431 `ade-see-error`
  unchanged selector (setup failed has one action, shown).
- `ade-v2-task-menu.spec.ts` line 73, `ade-v2-review-open.spec.ts` line 118: right-click the
  review row's `ade-tag` instead of `ade-task-cells > *`.
- `ade-v2-flat-steps.spec.ts:15`, `ade-v2-stage-move.spec.ts:14`, `ade-v2-norepo.spec.ts:14`,
  `ade-v2-run.spec.ts:22`, `ade-v2-automations.spec.ts:300`, `copy-coverage.spec.ts:132`: scope
  `ade-task-cells` -> `ade-card-head`.
- `ade-v2-base-rebase.spec.ts:145`: `toHaveText('Rebase')` -> `toHaveAttribute('aria-label', 'Rebase')`.
- Any other spec reading action-button text on the card (grep `ade-task-action-` with
  `Text`; today only the menu item in `ade-v2-stage-resume` reads text, untouched).
- `tests/unit/ade-v2-board-parity.spec.ts`: no change expected; must pass.

### Visual baselines

`tests/visual/ade-plan-branches.spec.ts` screenshots `ade-card` `T_search`, `R_li` and row
`b_auth`: re-record `plan-branches-stacked`, `plan-branches-review`, `plan-branches-pending`.
`plan-branch-tip`, `plan-branch-plain-tip`, `branch-panel-details` should not change; if they
diff, find out why before re-recording. First confirm `fc-match sans-serif` prints `Inter`
(`docs/DEV_ENVIRONMENT.md` "tests/visual/* pixel diffs"); never update from a mismatched font
setup. Command: `bun run build:test:space && npx playwright test --config=apps/kira-space/playwright.config.ts --project=visual ade-plan-branches --update-snapshots`,
then rerun without `--update-snapshots`. Look at the new PNGs before committing.
`ade-sandbox` and `ade-sessions-menu` screenshot popovers only; unaffected.

### Fast checks per commit

`bun run typecheck:space-web`, `typecheck:space-mobile`, `typecheck:space-tests`,
`typecheck:space-unit`, `bun run lint` (biome, check-ade-colours, catalog unused), `bun run
lint:dead`. Own specs only: `bun run build:test:space && npx playwright test
--config=apps/kira-space/playwright.config.ts --project=ui ade-v2-card-icons ade-v2-plan
ade-v2-panel ade-v2-task-menu ade-v2-review-open ade-v2-flat-steps ade-v2-stage-move ade-v2-norepo
ade-v2-run ade-v2-automations copy-coverage ade-v2-base-rebase ade-v2-force-push ade-v2-dialogs
copy-wording ade-v2-launch ade-v2-stage-resume ade-agent-missing ade-v2-handover`, plus the
unit parity spec. Full suite stays at chapter end.

## Commits

1. `refactor: tag kinds in board actions` (actions.ts, `glyphs.ts`, `STATUS_TEXT_CLASS`).
2. `feat: card tag and action as icons` (`AdeTagIcon`, `AdeTaskActionButton` icon mode,
   `AdeTaskCard`, `AdeBranchRow`, delete `AdeActionCell`).
3. `test: card icon row specs` (new spec + updates).
4. `test: re-record plan branch baselines`.
5. `docs: P500 result` (SPEC row Done + Result).

## File ownership and overlap

P500 owns:
- `plan/AdeTaskCard.vue`, `plan/AdeBranchRow.vue`, `plan/AdeActionCell.vue` (deleted),
  `plan/AdeTagIcon.vue` (new), `plan/glyphs.ts` (new)
- `board/actions.ts` (additive `kind` fields only)
- `run/AdeTaskActionButton.vue` (additive `icon` prop)
- `statusColors.ts` (additive `STATUS_TEXT_CLASS`)
- `tests/ui/ade-v2-card-icons.spec.ts` (new) and the spec lines listed above
- `tests/visual/ade-plan-branches.spec.ts-snapshots/plan-branches-{stacked,review,pending}-visual-linux.png`
- `docs/v0.2.30/SPEC.md` P500 row only

Read, not edited: `board/menu.ts`, `plan/useTaskMenu.ts`, `run/useTaskAction.ts`,
`board/rebaseActions.ts`, `AdeTip.vue`, `AdeBranchTip.vue`, `tones.ts`,
`packages/workbench/src/components/MenuIconButton.vue`, `packages/workbench/src/state/contextMenu.ts`.

Overlap:
- P495 (done): touched `board/actions.ts`, `board/menu.ts`, `plan/useTaskMenu.ts`,
  `run/useTaskAction.ts`. P500 builds on its tree: keeps `resume` and `startNew`, adds no task-menu
  entry, leaves `menu.ts` and `useTaskMenu.ts` untouched.
- P496 (`panel/AdeTaskTab.vue`, `en.json` key `space.ade.taskTab.handover`, `ade-v2-handover`
  spec): no common code file. P500 does not edit `ade-v2-handover.spec.ts` unless its line 194
  scope needs it (it scopes by `ade-task`, not `ade-task-cells`, so no). No `en.json` edit.
- P497 (`MenuIconButton.vue` new, `AutomationsPanel.vue`, `sessions/*`, launch specs): P500 reads
  and uses `MenuIconButton`, never edits it. Ordering dependency: P497 must land first (table order
  already guarantees it). Not a stream pair.
- P501 (`panel/AdeTaskPanel.vue`, `panel/AdeTaskTab.vue`, sibling tabs, adewire, maybe backend):
  no common file. `AdeTaskActionButton` gains an optional prop; panel callers keep text mode.
  `BranchTag.kind` / `TaskCell.tag.kind` are available to P501/P502 if their Main tab wants glyphs
  (`plan/glyphs.ts`, `AdeTagIcon`), reuse not copy.
- P502 (`AdeTaskPanel.vue` Main tab): no common file.
- `SPEC.md`: row-local edits only.

Runs in sequence after P497 (SPEC: P500-P502 never as streams).
