# P648 iteration 2: task-list details back, branch-row refinement, defect fixes

Plan by Opus. One Sonnet implementer, sequential. Base: `8b20f281` (P648 iter 1, commits
`16cc45c5..8b20f281`). Plans against current tree.

User asks:

- Task list shows less at a glance than before P647. Restore most lost details, keep P648 actions,
  keep the list working at 400px default (360px floor) on a MacBook Air.
- Refine the branch-row design a bit (planner chooses); not a revert to the old row.
- Fix three defects seen in real-app screenshots (form popover position, focus-pinned cluster,
  drag ghost covering the target).

Constraints: UI only, no backend/Go, no new tests, no test run. Done = `bun run typecheck`,
`bun run lint` (includes `lint:dead`/knip and catalog check), `bun run build` green in normal hooked
commits. CLAUDE.md frontend rules apply (Tailwind utilities, no `<style>`, shadcn-vue primitives,
`<script setup lang="ts">`). Lint rules hit in P648: U36 (no hand-rolled `rounded-kira + border +
p-*` box), U38 (no `text-kira-xs`, stock text sizes, raw `h-5..h-12`; heights from tokens), C3 and
U37 exemptions (`bg-select` on card head and branch row must stay). No new i18n keys needed.

All paths under `apps/kira-space/frontend/src/ade/v2/` unless rooted otherwise.

## 1. Measured px budget

Tokens: `text-kira-sm` 11px, `text-kira-md` 12px, `text-kira-lg` 13px, `h-4.5`/`size-4.5` 18px,
`size-control`/`h-control` (`kira-icon` button) 22px, `h-row` 28px (22 compact), `h-board-row` 44px,
`gap-0.5` 2px, `gap-1` 4px, `gap-1.5` 6px, `px-2` 8px. Mono name glyph at 12px ≈ 7.2px/char,
11px UI text ≈ 6px/char.

Measured from `plan-01-full-1280x800.png` and `plan-02-hover-head-1440x900.png` (2x / 1.44x device
scale): card outer width 356px at list 400 (both window sizes). Plan scroll container (the
`@container`) ≈ 380px.

| Box | list 400 | list 360 (floor) |
|---|---|---|
| card outer | 356 | 316 |
| card content (minus `border-l-3`, `border` 1, `px-2` 16) | 336 | 296 |
| head line 2 (minus `pl-6` 24) | 312 | 272 |
| branch row grid, depth 1 (minus 16 indent, 6 gap) | 314 | 274 |
| `@container` width | ≈380 | ≈340 |

Consequence: P648's `@max-[400px]:hidden` on card meta hides meta at the default width, always.
`@max-[380px]:hidden` on lease and automation chips sits right at the default width, so they vanish
on any narrowing. Both are defects; section 3 removes them.

## 2. Inventory: pre-P647 (`0899aacd`) vs HEAD

Card head (`plan/AdeTaskCard.vue`):

| # | Detail | 0899aacd | HEAD | Iter 2 |
|---|---|---|---|---|
| H1 | Title | own line, `line-clamp-2`, full width | line 1, one line, truncates (198px idle, ~114 hovered) | keep one line (3.1) |
| H2 | Step segments | `w-20` | `w-12` | keep `w-12` |
| H3 | Step label | `max-w-36` | menu trigger `max-w-28` | keep |
| H4 | Run progress bar + percent | bar `w-11.5` + `40%` | tooltip only | percent text back (3.2) |
| H5 | Automation chip | always, `max-w-40` | hidden `@max-[380px]` | always, `max-w-24` (3.3) |
| H6 | Lease chip | always, `max-w-40` | hidden `@max-[380px]` | always, `max-w-24` (3.3) |
| H7 | Parent chip | icon + title `max-w-40` | icon only | icon + title `max-w-24` (3.3) |
| H8 | Meta (date, span, Jira, repos) | flex-shrink text | hidden at 400 (defect) | always, truncates last (3.4) |
| H9 | Updates | dot + tooltip | popover button | keep |
| H10 | Change base, Review code | always on line 1 | hover cluster | keep (actions) |
| H11 | Estimate | none | chip | keep |

Branch row (`plan/AdeBranchRow.vue`, board layout):

| # | Detail | 0899aacd | HEAD | Iter 2 |
|---|---|---|---|---|
| B1 | Base marker | `BranchChip max-w-28` | tooltip only | muted inline text (4.3) |
| B2 | Context (draft `No branch yet · from main`, review PR title) | line 2 | tooltip only | inline muted tail on line 1 (4.3) |
| B3 | Branch progress (segs + `Implement · stuck`) | line 2 | tooltip only | line 2 (4.4) |
| B4 | Merged / deployed (`dev ✓ │ ▲staging ✓`) | line 2 | tooltip only | line 2, right-aligned (4.4) |
| B5 | Repo badge | unbounded | `max-w-20` | keep |
| B6 | Checked-out chip | `max-w-32` | `max-w-24` | keep |
| B7 | Fix / More / Review actions | always visible | hover cluster | keep (actions) |
| B8 | PR + checks glyph | none | always | keep |
| B9 | Row height | 40 (two lines always) | `h-row` 28 | 28, or 44 only with B3/B4 |

## 3. Card head

### 3.1 Line 1 stays; status never shrinks

Line 1 idle at 400: lead 18 + 6 + title + 6 + status ≤80 + 6 + action 22 → title ≈198px (158 at
floor). Title keeps one line with `TruncatedText` tooltip. Not restoring `line-clamp-2`: the hover
cluster sits in flow after the title, so a wrapping title would re-wrap on hover and change the
card height under the pointer (and under drag geometry). Panel shows the full title on select.

- `plan/AdeTaskStatusMenu.vue`: trigger class `shrink` to `shrink-0` (keeps `max-w-20 truncate`).
  Screenshot 02 shows `In reviev` clipped because flex shrank status alongside the title; now only
  the title (`min-w-0`) yields.

### 3.2 Percent back on line 2

After `AdeStepMenu`, when `p.showBar`: `<AdeTip :text="p.barTip"><span class="shrink-0 text-kira-sm
text-muted-foreground" data-testid="ade-step-percent">{{ formatPercent(p.percent) }}</span></AdeTip>`.
26px incl. gap. Bar itself stays out (46px for what segments + percent already say). `stepTip`
reverts to `p.workflowTip` only (bar tip now on the percent).

### 3.3 Chips back, compact

- `plan/AdeLeaseChip.vue` (sole caller is the card): wrapper `inline-flex shrink-0` to `inline-flex
  min-w-0 shrink`; `AdeChip` class `max-w-40 truncate` to `max-w-24 truncate`.
- `automation/AdeAutomationChip.vue` (sole caller is the card): button `shrink-0` to `min-w-0
  shrink`; `AdeChip` `max-w-40` to `max-w-24`. `RunElapsed` keeps `shrink-0` (add it).
- `AdeTaskCard.vue`: drop both `<span class="inline-flex @max-[380px]:hidden">` wrappers; render
  the chips directly.
- Parent chip: drop `icon-only` on the card; wrap in `<span class="inline-flex min-w-0 max-w-24">`
  (Badge already `min-w-0 shrink`, title span truncates). Keep the `iconOnly` prop (AdeMainTab and
  AdeArchivedPanel do not use it; if knip flags it unused, delete the prop and its branches in the
  same commit).
- Branch-request chip: add `min-w-0 max-w-24 truncate`.

Each is 96px max, present only while that state exists (rare together). Tooltips already carry
full text.

### 3.4 Meta always present, truncates first

`ade-card-meta` span: drop `@max-[400px]:hidden`; keep `min-w-0 flex-1 truncate` (basis 0: it takes
only what is left and never pushes). Line 2 at 400, typical in-progress card: segs 48 + 6 + step
≈98 + 6 + percent 26 + est 24 + 6 + 2 glyphs 40 → 254, meta gets ≈58px (`3d → Mon 28…`), full text in
its AdeTip. Line 2 order unchanged otherwise. Overflow rule: icons `shrink-0`; text chips (step,
automation, lease, parent, request) `min-w-0 shrink`; meta shrinks to 0 first because of basis 0.

### 3.5 Hover-only by design

Stay hover/tooltip only: full long title (H1), run bar detail (`barTip` on percent), workflow step
list (segments tooltip), head action cluster (add, change base, review, more), empty-estimate clock,
updates list (popover). Reason: actions are verbs, not state; long text has a one-line visible form.

## 4. Branch row refinement (board layout only; panel template untouched)

### 4.1 Before / after at 400px (depth 1, grid 314px)

Before (0899aacd, always 40px, two lines, actions always on):

```
[repo 51][op][⑂ base chip ≤112][name ……][attn 18][tag 18][fix 22][more 22][review 20]
          [segs][Implement · stuck][context …][dev ✓ │ ▲staging ✓]
```

Name budget: 314 − 51 − 112 − 6·5 − 18 − 18 − 64 ≈ 21px when a base chip shows; ≈133 without.

After (iter 2):

```
row without detail (28px):
[repo 51][name 137][⑂ base ≤80 · context tail]   [PR 18][checks 13][attn 18][tag 18]
                   ^ hover cluster appears here (pencil 22, fix 22, more 22, review 18)
row with progress or merged/deployed (44px):
[repo 51][name 137][⑂ base …]                    [PR][checks][attn][tag]
         [▪▪▫ Implement · stuck][context]          [dev ✓ │ ▲staging ✓]
```

Design changes vs both versions:

1. Grid, not flex: `grid grid-cols-[auto_minmax(0,1fr)_auto]`. Column 1 = repo badge, so line 2
   starts exactly under the branch name (detail hangs off the name, repo stays a quiet left key).
2. State glyphs form one right group with tighter `gap-1` (PR, checks, op, attention, tag), apart
   from identity on the left (`gap-1.5`). Danger actions stay at the group end.
3. Base marker loses its pill: muted `git-branch` icon 12px + short base name, `text-kira-sm
   font-data`, `max-w-20 truncate`, amber when pending, blue tone when someone else's. A same-task
   base (label `⑂` only) shows nothing: the depth connector already draws it. Pending same-task base
   shows the amber icon only.
4. Context is a muted tail on line 1, not its own line: draft (`No branch yet · from main`) and
   review PR title take only leftover width (flex basis 0), truncate, full text in the branch tip.
5. Line 2 exists only for B3/B4. Progress at left, merged/deployed right-aligned under the state
   group, so both "where it is" facts line up in a right column.

Budget line 1 at 400: 51 + 6 + 137 + 6 + right group (18+13+18+18 + 3·4 = 79) = 279 → 35px left for
base/context idle. Hover cluster (≈90) squeezes base/context to 0 first, then the name; no element
moves right or is covered. At floor (274): name truncates ≈5px idle. Line 2 at 400: segs 3–6 × 9 =
≤54 + 4 + label ≤`max-w-28` 112 + chips ≈110 → 280 of 314; context gets the rest.

### 4.2 Structure

Root (board branch of the existing `:class`): `box-border flex w-full gap-1.5 px-2` plus
`twoLine ? 'h-board-row items-center' : 'h-row items-center'`; keep `group group/row`,
`hover:bg-field` / `bg-select`, all attributes, handlers, testids.

`const twoLine = computed(() => !panel.value && (props.row.prog !== null || hasChips.value));`

Children:

1. Depth indent span (unchanged, `self-stretch`).
2. `<div class="grid min-w-0 flex-1 grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-1.5
   gap-y-0.5">`:
   - Line 1 col 1: repo `Badge` (unchanged classes, `max-w-20 truncate`).
   - Line 1 col 2: `<div class="flex h-control min-w-0 items-center gap-1.5">` holding
     - `AdeBranchTip v-model:open="tipOpen"` around `<span class="flex min-w-0 shrink items-center
       gap-1.5">`: owner pill (`isReview`), sparkle, name (`truncate font-data text-kira-md
       font-medium`, colours as today), attach glyph, checked-out `BranchChip max-w-24`, base text
       (4.3).
     - hover cluster (unchanged markup and classes except section 5.2 focus change).
     - context tail (4.3).
   - Line 1 col 3: `<div class="flex items-center gap-1">`: `AdeOpStatus` (moves here from the
     name group: it is a transient state badge), `AdeBranchPr`, `AdeAttention`, `AdeTagIcon`, danger
     actions, `<slot />`.
   - Line 2 (`v-if="twoLine"`): `AdeBranchTip :row :color` (no v-model) around `<div
     class="col-span-2 col-start-2 flex h-4.5 min-w-0 items-center gap-1.5 overflow-hidden
     whitespace-nowrap" data-testid="ade-branch-line2">` with progress (4.4), `<span
     class="flex-1" />`, chips (4.4).
3. Old line-1 `<span class="flex-1" />` goes (the grid's `1fr` column does the job).

`h-control` (22px) on line 1 fits the 22px `kira-icon` cluster buttons; 22 + 2 + 18 = 42 inside
`h-board-row` 44.

### 4.3 Base and context markup

```vue
<AdeTip v-if="row.base && (baseName || row.base.pending)" :parts="row.base.tip">
  <span class="inline-flex min-w-0 max-w-20 items-center gap-0.5 font-data text-kira-sm"
    :class="row.base.pending ? 'text-tone-amber' : row.base.tone === 'blue' ? 'text-tone-blue' : 'text-subtle'"
    data-testid="ade-base-marker" :data-pending="row.base.pending || undefined">
    <CodiconIcon name="git-branch" :size="12" class="shrink-0" />
    <span v-if="baseName" class="truncate">{{ baseName }}</span>
  </span>
</AdeTip>
```

`AdeTip` takes `parts` (checked). Tone classes may come from `TONE_TEXT_CLASS` (`../tones`: amber,
blue exist) instead of literals. Nested tooltip inside the AdeBranchTip trigger follows the
`AdeTagIcon` pattern already on rows. No `shadow-*` is used under `ade/v2`; drop the shadow class in
5.3 if any lint flags it.

Context: `<span v-if="row.context" class="min-w-0 flex-1 basis-0 truncate" :class="rowMeta"
data-testid="ade-branch-context">{{ row.context }}</span>` after the cluster. When there is no
context, add `<span v-else class="flex-1" />` so the col-2 inner flex still left-packs.

`baseVariant` becomes unused in the board template but stays used by the panel template; keep.

### 4.4 Line 2 content

Progress (`v-if="row.prog"`): reuse the panel markup (`ade-branch-prog`: segs `h-1.25 w-1.75`,
label `text-kira-sm` with `progClass`), label gets `max-w-28 truncate`, wrapper `shrink-0`. Chips:
same `ade-branch-merged` / divider / `ade-branch-deployed` spans as the panel, `shrink-0`, class
`c.tone === 'stale' ? 'text-tone-amber' : rowMeta`. Testids match the panel so the old specs'
selectors exist again on the board.

### 4.5 Hover-only on branch rows

Pencil, fix actions, More, Review (cluster); PR title/state/base (PR glyph tooltip); full branch tip
(tag text, repo, run tips). Reason: actions and long prose; every state now has a visible glyph or
text.

## 5. Defect fixes

### 5.1 Branch form popover at left window edge

Root cause (geometry, `plan-03b-branch-edit-form-1280x800.png`): the popover top sits right under
the row, so the anchor is correct (the pencil, via `PopoverAnchor as-child` > DropdownMenu >
trigger Button). Horizontal is wrong because `AdeTaskBranchForm` uses `align="end"`: the content
right edge aligns to the pencil right edge. P648 moved the pencil from the panel row's right end to
just after the branch name: pencil right ≈ 264px from the window edge, content `w-80` = 320px,
so left = −56 and Reka's collision shift clamps it to the window edge, covering the list. The
dropdown menu (`align="end"`, ≈180px) also hangs left over the row's own name (screenshot 03).

Fix:

- `panel/AdeTaskBranchForm.vue`: prop `align?: 'start' | 'end'` default `'end'`; `PopoverContent
  :align="align"`.
- `panel/AdeTaskBranchMenu.vue`: same prop, default `'end'`; pass to `AdeTaskBranchForm` and to
  `DropdownMenuContent :align="align"`.
- `plan/AdeBranchRow.vue` board cluster: `<AdeTaskBranchMenu align="start" …>`. Panel caller
  (`AdeMainBranches`) unchanged: its pencil sits at the right end, where `end` is right.

Result: menu and form open under the pencil, extending right (into the panel area at 400); nothing
lands left of the pencil.

### 5.2 Selected card keeps the cluster (focus-within)

Click on the head focuses it (`tabindex="0"`), so `group-focus-within/head:flex` keeps the cluster
open after the pointer leaves; the title stays truncated and status crowds. Replace with keyboard
focus only:

- `AdeTaskCard.vue` cluster: `group-focus-within/head:flex` to `group-focus-visible/head:flex
  group-has-[:focus-visible]/head:flex`.
- `AdeBranchRow.vue` cluster: same with `/row`.
- `AdeEstimateChip.vue` empty clock: `group-focus-within/head:visible` to
  `group-focus-visible/head:visible group-has-[:focus-visible]/head:visible` (also used in
  `AdeShelfRow`, root already `group/head`).

Menus and popovers keep the cluster pinned through the existing `has-[[data-state=open]]` /
`has-[[data-pinned]]`. Mouse focus left behind after a menu closes is not `:focus-visible`, so the
cluster hides on pointer leave. Combined with 3.1, the selected card shows the full idle layout.

### 5.3 Drag ghost covers the snap slot

Sortable fallback (`forceFallback`, `fallbackOnBody`) clones the whole card (all branch rows,
≈180px for PAY-102) at inline `opacity: 0.8`, positioned at the grab offset, so it sits over the
target card and the dashed slot (`plan-06b`).

Fix in two parts, no `!important`, no style block:

1. Smaller, lighter ghost. On the `Card` in `AdeTaskCard.vue` add
   `in-[.sortable-fallback]:max-h-board-row in-[.sortable-fallback]:max-w-64
   in-[.sortable-fallback]:opacity-75 in-[.sortable-fallback]:shadow-lg` (`in-*` variant already
   used in `DocumentView.vue`; `.sortable-fallback` is Sortable's default `fallbackClass`, set only
   on the clone). Ghost = card head only (44px), 256px wide, effective opacity 0.8 × 0.75 = 0.6.
   Use whatever shadow token the theme has for popovers if `shadow-lg` is flagged.
2. Offset from the pointer. `plan/AdeDayBand.vue`: keep the `useDraggable` return (`const sortable =
   useDraggable(…)`). Sortable appends the ghost in `_dragStarted` before `onStart`, then reads
   `options.fallbackOffset` on every move (`dx = pointer − tap + offset`). In `onStart`:

   ```ts
   const ev = evt.originalEvent;
   if (ev instanceof MouseEvent) {
     const r = evt.item.getBoundingClientRect();
     // Ghost's left edge 16px right of the pointer, vertically centred on it: the pointer's target stays visible.
     sortable.option('fallbackOffset', { x: ev.clientX - r.left + 16, y: ev.clientY - r.top - 22 });
   }
   ```

   Then the existing `emit('dragStart', …)`. `22` = half of `--kira-board-row-h`; name constants
   `GHOST_GAP = 16`, `GHOST_HALF_H = 22` at module top. Touch (`TouchEvent`) keeps the zero offset.
   Slot resolution reads the pointer (`usePlanDrag`), so it is unaffected.

Result: everything left of and under the pointer stays visible; the slot spans the full card width,
so its left part is never covered and the rest shows through at 0.6.

## 6. Commit groups (ordered, one hooked commit each)

1. `fix: branch edit menu and form open under the pencil` — `panel/AdeTaskBranchForm.vue`,
   `panel/AdeTaskBranchMenu.vue`, `plan/AdeBranchRow.vue` (only `align="start"`).
2. `fix: hover cluster shows on keyboard focus only` — `plan/AdeTaskCard.vue`,
   `plan/AdeBranchRow.vue`, `plan/AdeEstimateChip.vue`, `plan/AdeTaskStatusMenu.vue` (`shrink-0`).
3. `feat: restore card meta, percent, lease, automation and parent chips` — `plan/AdeTaskCard.vue`,
   `plan/AdeLeaseChip.vue`, `automation/AdeAutomationChip.vue`, `AdeParentChip.vue` (only if knip
   flags `iconOnly`).
4. `feat: branch rows show base, context, progress and targets` — `plan/AdeBranchRow.vue` (4.2-4.4).
5. `fix: compact drag ghost offset from the pointer` — `plan/AdeTaskCard.vue` (Card classes),
   `plan/AdeDayBand.vue`.

`bun run typecheck` and `bun run lint` before each commit; `bun run build` once after group 5, fix
any finding in a follow-up commit. No `--no-verify`.

## 7. Verification (implementer, then orchestrator)

- Fast checks green: typecheck, lint (incl. knip, catalog), build.
- Greps: no `@max-[400px]` / `@max-[380px]` in `AdeTaskCard.vue`; no `group-focus-within/` under
  `ade/v2/plan`; `align="start"` on `AdeTaskBranchMenu` in `AdeBranchRow.vue`; `align` prop used in
  `AdeTaskBranchForm.vue` `PopoverContent`; `ade-base-marker`, `ade-branch-context`,
  `ade-branch-prog`, `ade-branch-merged`, `ade-branch-deployed` in the board template of
  `AdeBranchRow.vue`; `grid-cols-[auto_minmax(0,1fr)_auto]` there; `ade-step-percent` in
  `AdeTaskCard.vue`; `fallbackOffset` and `sortable.option` in `AdeDayBand.vue`;
  `in-[.sortable-fallback]` in `AdeTaskCard.vue`; `shrink-0` in `AdeTaskStatusMenu.vue` trigger.
- No `<style` in touched files; no Go file and no test file in `git diff --stat 8b20f281`.
- Orchestrator screenshots (1280x800, 1440x900): idle list, hover head, selected card after mouse
  click (no cluster), branch edit menu and form (under pencil), mid-drag lower half. Before shots:
  `0899aacd` and current scratchpad PNGs.

## 8. Known fallout

UI specs touching card/branch shape already stale after P647/P648 stay unedited (user: no tests).
Board specs reading `ade-base-marker` now find a text span, not a `BranchChip`.

## 9. Out of scope

Backend, tests, panel branch-row layout, mobile `PlanTaskCard`, `AdeTaskChildren`, shelf rows
(beyond the estimate focus class), title wrapping.
