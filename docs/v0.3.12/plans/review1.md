# v0.3.12 code review, round 1

Base: `6a10591aa` (merged v0.3.11). Scope: `git diff 6a10591aa..HEAD` (P625: card look, branch row
lead, date heading rows, shell bar wrap, specs). Reviewer: Opus. Findings only, no fixes.

Checked and clean: card markup matches `v0.3.8` `AdeTaskCard.vue` except kept v0.3.10/11 additions
(`AdeOpStatus`, `AdeLeaseChip`, `pendingFor`, `unseen` sessions state); shelf row and branch row match
`v0.3.8` plus v0.3.11 `selected`/`rippleIds`; no references left to `taskTint.ts`, `cardColumns.ts`,
`CARD_*`, `ml-15`/`w-15`, `ade-row-attention`/`-agents`/`-flags`, `ade-branch-lead` (spec asserts its
absence); drop resolution (`usePlanDrag.resolveTarget` via `[data-ade-band]`) covers the new heading
row, so empty-day heading drop works; `scrollToDay`/`todayTop`/`syncTodayBelowLine` key off
`[data-ade-day]` and the `band.key === 0` marker, both unchanged; `--kira-fg`/`--kira-border` exist,
app is dark-only, so `--color-today` is lighter than `--color-border`; `@container`/`@5xl` and
`min-h-tabbar` resolve (Tailwind v4, `--spacing-tabbar`); `usePlanModel.ts`/`planModelMemo.ts` and the
query layer are untouched, card style is a plain `{ borderLeftColor }`, so v0.3.11 memoization and
single-flight refetch are not regressed. All touched components are `<script setup lang="ts">`,
Tailwind utilities, no scoped styles.

## F1 High: card head line still overflows at a narrow plan pane

- File: `apps/kira-space/frontend/src/ade/v2/plan/AdeTaskCard.vue:161` (step label), `:198` (meta);
  `apps/kira-space/frontend/src/ade/v2/plan/AdeTaskStatusMenu.vue:52` (status button).
- Defect: every head-line item but the meta is `shrink-0` (lead 18px, status up to 96px, segs 80px,
  step label up to 144px, bar about 80px, attention, chips, Change base, Review code, action button,
  six-plus 6px gaps). Sum exceeds about 420px of content width. Meta is `min-w-0 flex-1` (basis 0), so
  it collapses to 0 first and nothing else can give. `Card` is `overflow-hidden`, so the excess is
  clipped at the right edge. This is SPEC complaint 2 ("card top line cut off; user must widen the
  task list"), which P625 claims fixed.
- Failure scenario: panel open, plan pane about 480px (panel defaults to half the row, min 340px).
  Workflow task with long step label: `ade-card-head` scrollWidth 457 vs clientWidth 435. Meta
  `WEB-311 · web-app` is gone, the selected card's red Retry button sits on or past the card edge
  (right padding consumed, partly clipped when Review code or Change base also show).
- Fix (exact edits):
  1. `AdeTaskCard.vue:161`: `class="max-w-36 shrink-0 truncate text-kira-sm"` becomes
     `class="min-w-0 max-w-36 shrink truncate text-kira-sm"`.
  2. `AdeTaskCard.vue:198`: `class="min-w-0 flex-1 truncate text-kira-sm leading-3.5 text-subtle"`
     becomes `class="min-w-0 flex-[1_1_4rem] truncate text-kira-sm leading-3.5 text-subtle"`, so
     negative space is shared between step label and meta (by basis) instead of meta vanishing.
  3. `AdeTaskStatusMenu.vue:52`: replace `max-w-24 shrink-0` with `min-w-8 max-w-24 shrink` (Button
     base has `shrink-0`; `cn`/tailwind-merge lets `shrink` win). Shelf row uses the same component and
     only gains shrinkability.
  Segs (`w-20`), bar and all icon buttons stay `shrink-0`: they carry no text to truncate and the
  buttons must stay whole.
- Spec gap: `tests/ui/ade-v2-task-row.spec.ts:176` ("card head buttons stay inside the card with the
  panel open") runs at 1280px, where the pane is wide enough, and compares only against
  `card.right + 0.5`, so padding consumption passes. Add a narrow case: viewport 960x900 (or drag the
  panel splitter so the plan pane is about 480px), select `T_search`, then assert for every
  `ade-card-head` that `el.scrollWidth <= el.clientWidth` and that the last action/review button's
  right edge is at or left of `head.getBoundingClientRect().right - 8` (inside the `px-2` padding).

## F2 Low: drag highlight on an empty day is a 4px strip

- File: `apps/kira-space/frontend/src/ade/v2/plan/AdeDayBand.vue:119-120`.
- Defect: `highlight` outlines only the drop area. An empty band's drop area is now `min-h-1`
  (4px; was `min-h-control`). Dropping on an empty day's heading works (`[data-ade-band]` resolves),
  but the only feedback is a 4px outline under the heading row.
- Failure scenario: drag a card over `Sat 26` with no cards. Heading row is the obvious target; the
  user sees almost no outline and cannot tell the drop is accepted.
- Fix: move `highlight ? 'outline outline-1 outline-focus' : ''` from the drop div's `:class`
  (line 120) to the band root (`:class` at line 96), for example
  `:class="[band.isToday ? 'bg-today-tint' : '', highlight ? 'outline outline-1 outline-focus' : '']"`.
  Root already has `rounded-kira`, so the outline wraps heading and cards together.

## F3 Low: duplicate `data-today` on the band label

- File: `apps/kira-space/frontend/src/ade/v2/plan/AdeDayBand.vue:108`.
- Defect: P625 moved `data-today` to the band root (line 100); the label keeps its own copy. No
  frontend code or spec reads the label's attribute any more (`ade-v2-plan.spec.ts:632` and `:638`
  select `ade-day-band[data-today]`). Dead attribute.
- Failure scenario: none at runtime; a future selector can match two elements per Today band.
- Fix: delete line 108 (`:data-today="band.isToday || undefined"` inside the `ade-band-label` div).

No other real findings.
