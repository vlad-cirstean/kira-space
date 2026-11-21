# P709 iteration 3 plan: task colour line, issue key beside the title, status on line 2

Base: `22a7d1cab` (branch `stream/p709`, worktree `/home/user/kira-space-v0.3.22-p709`). Plans on
top of iteration 2 as landed (`5161630c6..22a7d1cab`). One sequential Sonnet implementer: every
item touches `AdeTaskCard.vue` / `taskGrid.ts` / `usePlanModel.ts`, order-dependent.

User feedback on iteration-2 shots:

1. Drop repo names from the card meta text (branch rows below already show repo badges).
2. Issue key (`task.jira?.key`) beside the title, not in meta: small muted, fixed style, never
   truncated, skipped when it equals the shown title; title keeps the rest and truncates.
3. Status (`AdeTaskStatusMenu`) moves from line 1 to the start of line 2, fixed-width column,
   aligned across cards. Line 1: title (full width), step action, hover overlay at its end.
   Line 2: status, step segments + menu + percent, meta, flags, estimate.
4. (Coordinator, later) The 10px colour square is barely visible. Replace it with a horizontal
   line in the task colour on the plan card, task panel header, review window header and Files
   window header; one language everywhere; parked = dashed.

## 1. Findings (verified in the p709 worktree)

### 1.1 Card head today (`plan/AdeTaskCard.vue:153-311`)

- Head `flex h-board-row items-center gap-1.5 px-2`: lead `TASK_LEAD` (w-6) | column.
- Line 1 `:175`: `AdeTaskSwatch` | title cell `relative flex min-w-0 flex-1` (`AdeTaskTitle` +
  `ade-card-actions` overlay `HOVER_OVERLAY`) | `TASK_STATUS_CELL` (w-20, status menu) |
  `TASK_ACTION_CELL` (w-control = 22px, step action).
- Line 2 `:233`: `TASK_SWATCH_GAP` (w-2.5) | `TASK_STEP_CELL` (w-32 `@lg:w-48`: segs `w-10`,
  `AdeStepMenu` `max-w-28`, percent `@max-lg:hidden`) | meta `flex-1` (`ade-card-meta`) | flags
  `shrink-0` | `TASK_STATUS_CELL` holding `AdeEstimateChip` | `w-control` spacer.
- `taskGrid.ts` comment and ARCHITECTURE :2912 state "status and estimate 80px".
- Status button (`AdeTaskStatusMenu.vue:51`) `h-4.5 min-w-8 max-w-20 truncate px-1 text-kira-sm`;
  shared with `later/AdeShelfRow.vue`. Statuses are user-named (fixture: To do, In progress, In
  review, Done); "In progress" at 11px ≈ 55px + 8px padding.

### 1.2 Meta (`plan/usePlanModel.ts:205-221` `cardMeta`)

`[day month · ][Nd → end · ][KEY · ]repo · repo`. Consumers: `AdeTaskCard.vue:265` and
`later/AdeShelfRow.vue:28` (`facts` = meta + notes; shelf rows have no branch rows, so repos are
their only repo hint). `CardModel.meta` doc comment `:125`. No Go parity, no other constructor of
`CardModel` (grep `defaultTitle:`: only `usePlanModel.ts`). `taskTitle` (`board/labels.ts:100`)
falls back to the Jira key when the task has no title: fixture `T_bill`, `T_auth`, `T_hooks`,
`T_push`, `T_csv` have `title: ''` and a key, so their shown title IS the key (skip rule matters).
Keys with a distinct title: `T_cart` WEB-311, `T_search` SRCH-41, `T_alerts` PAY-130. No key:
`T_notif`, `T_deps`, `T_spike`.

### 1.3 Branch row columns (`plan/AdeBranchRow.vue:428-592`, `BRANCH_GRID`)

`px-2`, depth span, grid `repo 80px | name 1fr | extras auto | PR 22px | tag 22px`, gap 6. Head
action cell and branch tag slot share the card's right edge: spec pins action centre = tag centre.
Head status column (x = card + 1 + 8 + 24 + 6) never lined up with the branch repo column (x =
card + 1 + 8 + 6); unchanged by this plan.

### 1.4 Width budget, 400px list (default `LIST_DEFAULT`)

Pane 400 → `px-3` 376 → card border 374 → head `px-2` 358 → lead 24 + gap 6 → column 328px.
At list 560 (spec `@lg` case: 900 viewport, 340 panel) the column is 488px. 360px list (min):
288px. Implementer measures the real column width first (scrollbar may take width) and records it
in the commit body; numbers below assume 328.

### 1.5 Task colour marks today (grep `taskColor|AdeTaskSwatch|card.color`)

| Host | Today |
| --- | --- |
| Plan card head `AdeTaskCard.vue:176` | `AdeTaskSwatch` 10px before title (`ade-task-swatch`) |
| Span row `AdeDayBand.vue:231` | `AdeTaskSwatch` |
| History row `AdeDayBand.vue:169` | `TASK_SWATCH_GAP` spacer (no colour: archived) |
| Review card `AdeBranchRow.vue:447` | `AdeTaskSwatch` before owner pill (`taskRow`) |
| Shelf row `later/AdeShelfRow.vue:55` | `AdeTaskSwatch` (outline when parked) |
| Task panel `panel/AdeTaskPanel.vue:116-121` | 12px square `ade-panel-dot` (outline parked) |
| Branch panel `panel/AdeBranchPanel.vue:314-319` | 12px outline square `ade-panel-dot` (review: `border-info`) |
| Review window `review/AdeReviewHeader.vue:58-62` | 10px square `ade-review-task-colour`; falls back to slot 0 with no task |
| Files window `files/AdeTaskFilesWindow.vue:31-35` | 10px square `ade-files-window-colour` |
| All sessions `needs/AdeAllSessions.vue:130` | 10px square in `SectionHeading #leading` (archived: subtle) |
| Needs row `needs/AdeNeedsRow.vue:93-96` | 8px square before task title in meta text |
| Branch tooltip `plan/AdeBranchTip.vue:44-48` | 10px square before branch name |
| Branch tooltip base row `AdeBranchTip.vue:74-78` | 8px square, parent task colour (`baseMarker.ts:51`) |
| Mobile `PlanTaskCard.vue:72` | `StatusDot`; separate mobile surface |

Contrast of all 20 `palette.ts` colours (WCAG relative luminance): on `--kira-bg-chrome` #131315
min 5.02:1 (`#727ed7`) max 8.31:1; on `--kira-bg` #1b1b1e min 4.65:1; on `--kira-bg-elevated`
min 4.34:1. Every colour clears the 3:1 non-text bar at full strength: the square failed on size
(100px² of colour), not contrast. Use the full colour, no tint, no opacity.

Spec/test-id users: `ade-task-swatch` (`ade-v2-task-row.spec.ts:42,135,166-176,330-340`,
`ade-v2-plan.spec.ts:419`); `ade-panel-dot` (`ade-v2-task-panel.spec.ts:101`, right-click target
for the header menu); `ade-review-task-colour` / `ade-files-window-colour`: no spec.

## 2. Design

### 2.1 Colour line: one language

Form: a 3px horizontal line in the full task colour along the top edge of the thing that IS the
task (card, panel header, window title bar). Rows too tight or inline for an edge (shelf row,
section heading, tooltip, needs meta) get the same line in miniature: a 16×3px rounded bar
(`mark`) before the name. Parked: dashed. No square remains on desktop.

Why a top edge and not iteration 1's left bar: the left bar ran the card's full height, so its
mass grew with branch count (a 5-branch card shouted, a 1-branch card whispered) and it sat beside
the lead/agent column and branch-tree connectors, crowding the left edge where structure lives. A
top line has the same mass on every card (cards share one width), reads like a coloured tab
header right above the title, keeps the left edge for structure, and is the one form that also
fits a panel header and a window title bar. 3px × 374px of colour is ~11× the square's area.

Implementations (two, same look):

- Card edge = the card's own top border (`border-t-3` + inline `borderTopColor`). Why not an inner
  element: the card is `overflow-hidden`, which clips at the padding box, so an inner line would
  sit under the 1px grey top border (grey over colour, the clash). Using the border segment
  itself: no second line, rounded corners stay one shape (the 6px corner blends colour into the
  1px side, standard border join), parked `border-dashed` already dashes the top too.
- Header edge = `AdeTaskLine variant="edge"`: `pointer-events-none absolute inset-x-0 top-0 h-0.75`
  overlaid on a `relative` header. Overlay, never in flow: no height change; in the window title
  bars it keeps macOS traffic lights and drag geometry exactly where they are.
- Mark = `AdeTaskLine variant="mark"`: `inline-block h-0.75 w-4 shrink-0 rounded-full`.

Dashes for `AdeTaskLine` (parked): inline `background: repeating-linear-gradient(90deg, <c> 0 6px,
transparent 6px 10px)` (deterministic 6/4, a 16px mark shows two dashes; a native 3px dashed
border renders ~1 dash in 16px). Solid: `background: <c>`. Card edge uses the native dashed
border, matching its dashed sides.

States (card):

- Selected: `border-focus` stays on left/right/bottom; inline `borderTopColor` wins on top, so the
  task colour never disappears. Selection still reads via the focus sides plus `bg-select` head.
- Ripple: `border-subtle/50` sides; top stays colour. Unselected: `border-border` sides.
- Hover: head ground only (`GROUND_ROW`); border widths fixed in every state, no shift.
- Card grows 2px (3px top vs 1px); identical for every card; head stays 44px.

New component `plan/AdeTaskLine.vue` (`<script setup lang="ts">`, Tailwind only):

```vue
<script setup lang="ts">
import { computed } from 'vue';
import { TASK_LINE_EDGE, TASK_LINE_MARK } from './taskGrid';

// Task colour as a line: `edge` overlays the top of a relative header, `mark` sits inline before a
// name. Parked dashes it (shape cue, not hue alone).
const props = defineProps<{ color: string; parked?: boolean; variant: 'edge' | 'mark' }>();
const paint = computed(() => ({
  background: props.parked
    ? `repeating-linear-gradient(90deg, ${props.color} 0 6px, transparent 6px 10px)`
    : props.color,
}));
</script>

<template>
  <span
    :class="variant === 'edge' ? TASK_LINE_EDGE : TASK_LINE_MARK"
    :style="paint"
    :data-parked="parked || undefined"
    data-testid="ade-task-line"
    aria-hidden="true"
  />
</template>
```

Host passes `data-testid` to override where an id already exists (fallthrough attrs win).

Per host:

| Host | Change |
| --- | --- |
| Plan card (all cards, incl. review cards and parked) | `boxClass` adds `border-t-3`; Card `:style="{ borderTopColor: card.color }"`. Swatch removed from line 1 |
| Span row (`AdeDayBand.vue:224`) | Same edge: class `border-t-3`, `:style="{ borderTopColor: s.color }"`; swatch removed. Reads "same task, continued" |
| History row | Drop the `TASK_SWATCH_GAP` spacer; no colour (archived, none in `h`) |
| Review card swatch (`AdeBranchRow.vue:447`) | Removed: the review card's own top edge carries the colour. Drop `AdeTaskSwatch` import |
| Shelf row | `AdeTaskLine variant="mark"` (parked dashed) replaces the swatch, same slot |
| Task panel header | `ade-panel-dot` element removed; `<AdeTaskLine variant="edge" :color="card.color" :parked="card.parked" />` first in the `#header` slot; `AdePanelFrame.vue:15` head div adds `relative` |
| Branch panel header | Same edge (task colour, dashed when parked; review task: keep today's info hue, pass `color="var(--kira-info)"` (`tokens.css:32`, #69a1e8, behind `border-info`), so "review" stays distinct); outline dot removed |
| Review window | `AdeTaskLine variant="edge" v-if="task"` as last child inside `TitleBarBase` (root already `relative`), `data-testid="ade-review-task-colour"`, `:parked="task.kind === 'parked'"`. No fake slot-0 colour without a task |
| Files window | Same inside its `TitleBarBase`, `data-testid="ade-files-window-colour"` |
| All sessions group | `mark` in `#leading` (archived keeps `var(--kira-fg-subtle)`) |
| Needs row | `mark` before the task title (`taskColor` prop) |
| Branch tooltip name row + base row | `mark` (base row: parent task colour) |
| Mobile `PlanTaskCard` | Keeps its dot: separate mobile surface, not in the ask |

Delete `plan/AdeTaskSwatch.vue`, `TASK_SWATCH`, `TASK_SWATCH_GAP` (no user left; grep must be
empty). Status dots (`STATUS_DOT_CLASS`, statuses editor) are status colour, untouched.

Title x after the swatch leaves: card `1 + 8 + 24 + 6 = 39px`; span `1 + 8 + 24 (13px icon +
mx-[5.5px]) + 6 = 39`; history same without the spacer. All three move 16px left together.

### 2.2 Issue key beside the title

Placement: directly AFTER the title text, inside the title cell. Rejected: before the title (after
the old swatch slot). Keys vary (SRCH-41 to AUTH-212, any project prefix); a fixed slot would
either truncate long keys (forbidden) or waste ~60px on every keyless card at 400px, and a
variable slot moves the title x per card, breaking the one alignment users scan. After the title,
the title x stays 39px on every card and the key reads as the title's suffix.

Behaviour: title span shrinks first (it is a flex child with `min-w-0`; `TruncatedText` ellipsis
plus tooltip), key is `shrink-0 whitespace-nowrap`, so a short title is followed by its key and a
long title truncates with the key pinned at the cell end. On hover the overlay may cover the key of
a long-title card, as it covers the title end today: accepted (key is also in the panel header).

New component `plan/AdeTaskKey.vue` (shared by card and shelf row; hides while the title is being
renamed so the input gets the room):

```vue
<template>
  <span
    v-if="card.issueKey && !editing"
    class="shrink-0 whitespace-nowrap font-data text-kira-sm text-subtle"
    data-testid="ade-task-key"
    >{{ card.issueKey }}</span
  >
</template>
```

`editing` = same computed as `AdeTaskTitle.vue:18-20`. Style: plain muted mono text, not a Badge:
repo badges are chips, so a chip key would read as a repo; mono matches branch names (identifiers).

Model (`usePlanModel.ts`):

- `CardModel` add `issueKey: string` (doc: "Jira key shown after the title; empty when none or when
  it is the title") and `repos: string[]` (doc: "Repo labels of the task's branches, in branch
  order; the shelf row shows them").
- `buildCard`: `const title = taskTitle(...)`; `issueKey: key && key !== title ? key : ''` with
  `key = task.jira?.key ?? ''`; `repos: repoNamesOf(c, branches)` (the loop moved out of
  `cardMeta`).
- `cardMeta(c, entry)` returns `[dated ? \`${day} ${month}\` : '', span > 1 ? \`${n}d → ${end}\` :
  ''].filter(Boolean).join(' · ')` (no trailing separator, no key, no repos). Doc `:125`
  becomes `` `3d → Mon 28`: schedule only. ``
- `AdeShelfRow.vue` `facts`: `[card.meta, card.repos.join(' · '), notes]` filtered, joined. Shelf
  rows have no branch rows, so they keep repo names; `AdeTaskKey` goes inside `ade-shelf-title`
  after `AdeTaskTitle` (`ade-shelf-title` gets `gap-1.5`).

### 2.3 Card head grid (status to line 2)

```
line 1 | lead w-6 | title cell flex-1: title · key · (overlay) | action w-control |
line 2 | (lead)   | status w-20 | step w-28 @lg:w-48 | meta flex-1 | flags auto | time 2×control+6 (right-aligned) |
```

Budget at 328px, line 2, 4 gaps of 6: status 80 + step 112 + time 50 + gaps 24 = 266, meta + flags
62px (fits `3d → Mon 28` ≈ 58px; meta now short since key and repos left). At 488px meta gets
222px with step 192. At 288px meta 22px: truncates (meta is also in the panel).

Choices:

- Line 2 starts at the title x (no spacer): status x = title x on every card, the cleanest vertical
  in the list (status text at title x + 4px button padding).
- Status keeps 80px (`w-20`): button `max-w-20` unchanged (shared with the shelf row), long custom
  names truncate inside it.
- Step cell narrows to `w-28` below `@lg` (was `w-32`), segments `w-8 @lg:w-10`: step menu gets
  76px; long step names ellipsize (button already `truncate`). Percent stays `@max-lg:hidden`.
- Time column (estimate) leaves the status width: `TASK_TIME_CELL`, width = PR slot + gap + tag
  slot (50px), `justify-end`, no trailing spacer. Its right edge = card content right = action
  right = branch tag right; its left = branch PR slot left. So the estimate sits over the branch
  rows' PR/tag columns and lines up down the whole card.
- Line 1 right column: action cell stays last child (specs `line1.lastElementChild`, card-icons
  "last button of line 1"); hover overlay now ends at the action cell's left (title cell end).

`taskGrid.ts` (final):

```ts
// Plan task list column tracks: every card head, branch row, span and history row shares them, so
// columns line up whatever the content. Widths go on wrapper cells, never on shared components.
export const TASK_LEAD = 'inline-flex w-6 shrink-0 justify-center';
/** Line 2 lead: the task status, at the title x. */
export const TASK_STATUS_CELL = 'flex w-20 shrink-0 min-w-0 items-center overflow-hidden';
export const TASK_ACTION_CELL = 'flex w-control shrink-0 items-center justify-center';
export const TASK_STEP_CELL = 'flex w-28 shrink-0 min-w-0 items-center gap-1 @lg:w-48';
/** Estimate: spans the branch rows' PR and tag slots, right-aligned to the card edge. */
export const TASK_TIME_CELL =
  'flex w-[calc(var(--spacing-control)*2+--spacing(1.5))] shrink-0 min-w-0 items-center justify-end overflow-hidden';
export const BRANCH_GRID = …unchanged…;
export const SLOT = …unchanged…;
export const GROUND_CARD = …unchanged…;
/** Task colour on a card or span row: its top border, coloured inline. */
export const TASK_EDGE = 'border-t-3';
export const TASK_LINE_EDGE = 'pointer-events-none absolute inset-x-0 top-0 h-0.75';
export const TASK_LINE_MARK = 'inline-block h-0.75 w-4 shrink-0 rounded-full';
export const GROUND_ROW / GROUND_SELECTED / HOVER_OVERLAY = …unchanged…;
```

If Tailwind rejects `--spacing(1.5)` inside `calc` in the arbitrary value, use
`calc(var(--spacing-control)*2+var(--spacing)*1.5)`. Confirm computed width 50px.

`AdeTaskCard.vue` markup:

- `boxClass`: add `TASK_EDGE`; Card `:style="{ borderTopColor: card.color }"`.
- Line 1 `:175-232`: children = title cell `relative flex min-w-0 flex-1 items-center gap-1.5`
  [`AdeTaskTitle`, `AdeTaskKey`, overlay span unchanged]; `TASK_ACTION_CELL`. Remove swatch and
  status cell.
- Line 2 `:233-309`: children = `<span :class="TASK_STATUS_CELL"><AdeTaskStatusMenu :card="card"
  /></span>`; step cell (segments `w-8 @lg:w-10`); meta (unchanged element/test id); flags
  (unchanged); `<span :class="TASK_TIME_CELL"><AdeEstimateChip v-if="!card.review" :card="card"
  /></span>`. Remove `TASK_SWATCH_GAP` spacer and trailing `w-control` spacer.
- Imports: drop `AdeTaskSwatch`, `TASK_SWATCH_GAP`; add `AdeTaskKey`, `TASK_EDGE`,
  `TASK_TIME_CELL`.

Branch rows, span and history rows: tracks unchanged (span/history titles still land at the card
title x per §2.1).

## 3. Work items (in order)

0. Before shots (§6) on untouched `22a7d1cab`, fresh `bun run build:test`. Measure the real head
   column width at the default layout (§1.4).
1. `usePlanModel.ts` (§2.2 model), `AdeTaskKey.vue`, `AdeShelfRow.vue` facts + key.
   `bun run typecheck:web && bun run lint`. Commit `feat(ade): issue key beside the task title,
   schedule-only card meta`.
2. `taskGrid.ts`, `AdeTaskCard.vue` grid (§2.3). Same checks. Commit `feat(ade): task status leads
   card line 2 with an aligned estimate column`.
3. `AdeTaskLine.vue`, card/span edge, every host in the §2.1 table, delete `AdeTaskSwatch.vue` and
   the swatch constants. Same checks plus `bash scripts/check-ui-primitives.sh`. Commit
   `feat(ade): task colour as a line on cards, panels and windows`.
4. Specs (§4). Run the UI set (§5); fix; commit `test(ade): ...` per group.
5. Re-record visual baselines (§5); inspect each PNG by eye. Commit `test(ade): re-record plan,
   panel and files baselines`.
6. After shots (§6); compare; fix and loop 4-6 on any misalignment, clipping or hidden line.
7. `docs/ARCHITECTURE.md`: :2910 colour sentence (task colour is a 3px line in the full colour:
   the card and span row top border, an overlaid top edge on the task/branch panel head and the
   review and Files window title bars, a 16×3px mark in shelf rows, All sessions, needs rows and
   the branch tooltip; dashed when parked; history rows carry none; mobile keeps its dot); :2911
   ("The task colour swatch (P709) sits after the lead" becomes "status leads card line 2; the
   issue key follows the title"); :2912 tracks (lead 24px, status 80px leading line 2 at the
   title x, step 112/192px, time column = PR + tag slots right-aligned, action/PR/tag 22px; meta is
   schedule only). Commit `docs: P709 iteration 3`.
8. `docs/v0.3.22/SPEC.md` `## P709` Result: append "Iteration 3:" paragraph (key, meta, status
   line 2, colour line per surface with the left-bar reasoning, specs and baselines changed, shot
   dirs, before = `22a7d1cab`). Row stays Done. Commit.

## 4. Tests

No unit test (one-rule string join, layout). No flow half: no IPC or backend change.

Update (record each in the commit body; keep every other test id):

- `ade-v2-task-row.spec.ts` grid test (`:300-422`, both cases):
  - drop `swatch`, `swatchGap`; keep `title` same-x; `spanTitle[0]` = `title[0]` (±1).
  - status: `same(geo.status)` plus status x = title x (±1) and status centre y > title centre y
    + 8 (line 2).
  - time cell = line 2 `lastElementChild`: `same` x; its left = branch PR slot (`cell(el, 3)`) x
    (±1), its right = tag slot right (±1).
  - key: `gridBoard` sets `T_cart` long title (has WEB-311). Expect `ade-task-key` on `T_cart`
    (`WEB-311`) and `T_search` (`SRCH-41`); for `T_search` key x = title right + 6 (±1); for
    `T_cart` key right ≤ action cell x − 6 + 1; every key `scrollWidth <= clientWidth`; count 0 on
    `T_notif` (no key) and `T_bill` (title is its key).
  - add `ade-task-key` to the hover `boxes()` list.
- `ade-v2-task-row.spec.ts:28-42`: `ade-row-lead ade-task-swatch` → `ade-row-lead ade-task-line`
  count 0 (keep `ade-card-dot` 0). Name: "... and the task colour sits on the card's top edge".
- `:119-150` shelf: one `ade-task-line` in the row (mark); `ade-shelf-facts` of a shelf task with a
  branch still contains its repo label; if the shelf fixture task has a distinct key, one
  `ade-task-key`.
- `:151-185` card test → `a task card wears its colour as a 3px top edge, on the plan ground`:
  every card `borderTopWidth` 3px, left 1px, `borderTopColor` = its inline `style.borderTopColor`
  (non-empty), no `ade-task-swatch` / `ade-task-line` in the head, card bg = plan bg; parked
  `T_spike` (if on the Plan; else via its shelf) `borderTopStyle` dashed; after selecting `T_bill`
  its top colour unchanged and `borderLeftColor` = focus.
- `ade-v2-plan.spec.ts:386-420` card box: top `3px`; replace the swatch background check with
  `borderTopColor` not transparent and not the `--kira-border` colour.
- `ade-v2-plan.spec.ts:436` marked test and `:544` focus test: read `borderLeftColor` instead of
  `borderTopColor` (top edge now carries the task colour).
- `ade-v2-plan.spec.ts:556` name: "task colour line" instead of "task swatch".
- `ade-v2-plan.spec.ts:160`: meta `'3d → Mon 28'`; `ade-task-key` `'SRCH-41'`; meta has no
  `web-app`. `:196/:199`: keep regexes as `/^Tue 29 Sep/` and `/^Wed 30 Sep/` (no trailing ` · `
  when nothing follows; check exact text and pin it with `toHaveText` when stable).
- `ade-v2-card-icons.spec.ts:186-196` "hover actions sit right": compare the overlay right edge
  with the action cell x (line 1 `lastElementChild`), comment "left of the action column".
  `:64-80` keeps status-before-meta order (still true).
- `ade-v2-task-panel.spec.ts:101`: right-click `ade-panel-title` instead of `ade-panel-dot`. Add
  one assertion in an existing task-panel test: `ade-panel-head ade-task-line` count 1, height
  3px, y = head y (±1); parked task (`T_spike` panel) has `data-parked`.
- Review/Files headers: add to `ade-v2-review.spec.ts:40` boot test `ade-review-task-colour`
  height 3px, top = title bar top; same in one `ade-v2-task-files-window.spec.ts` test for
  `ade-files-window-colour`.
- Any other failure: root-cause; fix product when it is a regression; change an assertion only
  when it pins swatch, status-on-line-1, meta text with key/repos, or top-border-as-focus. Never
  loosen focus, selection, height or ring assertions.

## 5. Verification

From `/home/user/kira-space-v0.3.22-p709`:

```sh
bun run typecheck:web && bun run lint && bash scripts/check-ui-primitives.sh
bun run build:test
bunx playwright test --config=apps/kira-space/playwright.config.ts --project=ui \
  ade-v2 consistency panel-resize
fc-match sans-serif   # must print Inter
bunx playwright test --config=apps/kira-space/playwright.config.ts --project=visual \
  ade-plan-branches ade-plan-days ade-task-files --update-snapshots
bunx playwright test --config=apps/kira-space/playwright.config.ts --project=visual
```

Baselines expected to change: `plan-branches-stacked`, `plan-branches-review`,
`plan-branches-pending` (if the crop includes the card edge; else unchanged), `plan-day-bars`,
`plan-branch-tip`, `plan-branch-plain-tip` (tooltip mark), `branch-panel-details` (only if the crop
includes the panel head), `files-open`, `files-reading`, `files-search`, `files-launch-error`
(title bar line). Possibly `ade-links-card` (check its crop). Expected unchanged: `files-empty`,
`review-step-panel`, `review-diff-gutter`, `review-uncommitted-only`, all non-ADE baselines. Any
other change: find why before re-recording (`--update-snapshots <spec>` for that spec only).

Real checks before done: grep empty for `AdeTaskSwatch|TASK_SWATCH|ade-task-swatch` under
`apps/kira-space/frontend` and `tests`; grep empty for `size-2.5 shrink-0 rounded-kira-sm"` /
`size-2 shrink-0 rounded-kira-sm"` task-colour squares in the §1.5 hosts; `cardMeta` has no
`repoLabel`/`jira`; computed card `border-top-width` 3px with the task colour; time cell computed
width 50px; status x = title x in the 400px shot (measured, not eyeballed).

## 6. Screenshot procedure (implementer; not committed)

Output `screenshots/p709-iter3/before/` and `.../after/` (gitignored, never `git add`; never beside
a UI suite in the same checkout). Temporary `apps/kira-space/tests/screenshots/p709-iter3.shot.ts`,
deleted after step 6. Base it on the iteration-2 script (`P709-iter2.md` §6) with:

- Board: `T_cart` long title (keeps WEB-311), `T_notif` title `Fix` and `est: ''`, `T_deps`
  without workflow, `T_search` unchanged (short title + SRCH-41), `T_bill` (title = key).
  Make one Plan task parked if none shows on the Plan (`kind: 'parked'` on `T_deps`) so the
  dashed edge is in frame.
- Layouts: `list400` (1400×900, default layout: 400px list beside the panel) and `wide`
  (2000×1250, `panel.ade.width: 520`, list ≈ 1480px).
- Per layout, dark: `${name}-rest.png` (`ade-shell`), `${name}-hover.png` (hover `T_auth` head,
  `ade-plan`), `${name}-selected.png` (click `T_search` head: selected card + task panel header
  with its line), `${name}-parked.png` (select the parked task: dashed card edge + dashed panel
  edge), `${name}-branch-panel.png` (click a `b_auth` row: branch panel head),
  `${name}-end.png` (plan scrolled to bottom: span and history rows).
- Review window: a `reviewTest` case using the same `reviewControl()` / `GIT` setup as
  `ade-v2-review.spec.ts` `openReview` (`storage: AGENT_PANEL_ON`), viewport 1400×900 and
  2000×1250; shot of `ade-review-window` (`review-1400.png`, `review-2000.png`).
- Files window: `openFilesWindow(relaunch)` from `support/adeV2`, viewport 1400×900 and 2000×1250;
  shot of `ade-files-window` (`files-1400.png`, `files-2000.png`).
- All sessions (`AdeAllSessions`) if reachable from the Needs page with the fixture sessions: one
  shot (`all-sessions.png`).

```sh
bun run build:test
P709_OUT=screenshots/p709-iter3/before bunx playwright test \
  --config=apps/kira-space/playwright.config.ts --project=screenshots p709-iter3   # step 0
P709_OUT=screenshots/p709-iter3/after bunx playwright test \
  --config=apps/kira-space/playwright.config.ts --project=screenshots p709-iter3   # step 6
```

If the `screenshots` project does not register `reviewTest` windows, import `reviewTest` from
`../ui/fixtures` in the same file (it boots its own window). Dark only (no light theme,
ARCHITECTURE Known open item). Open each pair with Read and check: title x one column with no
swatch; key right after short titles, at the cell end on `T_cart`, absent on `T_bill`/`T_notif`;
status at the title x on line 2 on every card; estimate right edge on the action/tag column; meta
holds no repo names; colour line visible on every card, the panel head, branch panel, review and
Files title bars; dashed when parked; selected card keeps its colour top with focus sides; no
clipping at 400px. Report both dirs in Result and the final message.

## 7. Risks

- Card top border join: at the 6px corner the 3px colour blends into the 1px side diagonally. If
  the shot shows a visible notch, keep the border approach and say so in the Result; never add an
  inner element (clash with the grey top border, §2.1).
- Selected card loses its focus-coloured top edge (task colour wins). Selection still shows on
  three sides plus the head `bg-select`; specs reading `borderTopColor` for focus move to
  `borderLeftColor`.
- Spec `16px` gap between same-day cards (`ade-v2-plan.spec.ts:446`) measures card boxes: unchanged
  (border is inside the box), but card height +2px may shift visual crops.
- Narrow budget: meta gets ~62px at 400px and ~22px at 360px; step labels truncate earlier
  (`w-28`). Acceptable; both are in the panel. If `T_auth` step label becomes unreadable, drop
  segments to `w-6` below `@lg` before narrowing status.
- Long-title card on hover: overlay may cover the key (as it covers the title end). Accepted.
- `AdeTaskKey` while renaming: hidden, so `InlineEditInput` takes the cell.
- Shelf rows keep repo names in facts (no branch rows there). If the user meant "no repo names
  anywhere", it is one line in `AdeShelfRow.vue`; name this reading in the Result.
- Review window without a task now shows no line (was slot-0 colour): intended (no fake colour).
- `TitleBarBase` drag region: line is `pointer-events-none`, so dragging and traffic lights are
  untouched; check `wails-drag` still covers the top 3px in the review/Files shots by reading the
  computed style, not by guessing.
- `perf/heap-ram.spec.ts` `ade-panel` refs: unaffected (no panel structure change beyond one span);
  check by reading.
