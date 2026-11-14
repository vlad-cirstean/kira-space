# P642 Task list day styling

Spec row: "In the ADE task list: replace the today background (current colour is wrong; pick from
theme tokens, light and dark), add a vertical left bar spanning each day's rows to show where the day
starts and ends, today's bar differs slightly from other days'. Visual spec and screenshot refresh."

User's words: "The today background color in ade task list is not ok, also i want to have a vertical
left bar showing when a day starts and ends. And the today one to be slightly different than the
other days one."

Base commit: `ee9e0d597`. Frontend presentation only.

## Current state

Task list = ADE Plan view. `AdePlanView.vue` (line 422) renders one `AdeDayBand` per
`model.view.bands` entry inside `div.px-4.pb-4.pt-3`, on the `bg-chrome` plan ground (line 378).
Bands come from `buildTimeline` (`board/timeline.ts`, `buildBand` line 239). Each `TimelineBand`
already carries `isToday` (`dk === 0`), `isLater` (`key === LATER` = 9999), `isPast`, `weekend`,
`dayOff`. No model change needed.

`AdeDayBand.vue`:

- Root `div.flex.flex-col.rounded-kira` (line 94). Today adds `bg-today-tint` (line 97). Root carries
  `data-testid="ade-day-band"`, `data-today`, `data-ade-day="<key>"`.
- Head row `ade-band-head`: label, hours, then a 1px rule `span.h-px.flex-1` with `ruleClass`
  (line 86): today `bg-today`, Monday/Later `bg-border`, else `bg-border/50`.
- Body `ade-band-drop`: history rows, overdue strip, overflow strip, `AdeTaskCard`s, span
  (continuation) rows. Weekend gets a stripe gradient.

Tokens (`apps/kira-space/frontend/src/ade/v2/tones.css`, imported by `frontend/src/styles.css`):

```css
--color-today: color-mix(in srgb, var(--kira-fg) 35%, var(--kira-border));
--color-today-tint: color-mix(in srgb, var(--kira-fg) 5%, transparent);
```

Only `AdeDayBand.vue` uses either (grep: no other caller).

### Why today's background is wrong

`--color-today-tint` is a 5% white wash, not a palette surface. On the `bg-chrome` ground
(`#131315`) it resolves to about `#1d1d1f`. Cards are `Card variant="section"` = `bg-elevated`
(`#212125`). Card-to-ground step drops from `#13`→`#21` on other days to `#1d`→`#21` on today:
luminance ratio about 1.05, so today's cards melt into a flat grey slab. The surface ladder
(chrome < bg < elevated) has no grey between chrome and elevated that keeps card contrast, so any
neutral grey repeats the defect.

History: v0.3.12 ("Today is a neutral grey") replaced an amber today marker; spec test
`ade-v2-plan.spec.ts:630` "today is a subtle grey, not yellow, not like other days" pins the head rule
as grey (chroma <= 12) and lighter than other days' rule. That user decision stands for the rule and
the bar; the background moves off grey to a hue.

### Light and dark

Kira Space has one dark `:root`; no light theme exists (`docs/ARCHITECTURE.md` Known open items, "No
light theme; ade's kept tone and palette values are dark-only (P138)"). Every new value resolves
through `--kira-*` tokens, so a future light `:root` flips them with no `ade` edit. No literal colour:
`scripts/check-ade-colours.sh` fails on any literal under `ade/`. Nothing more to do for light here.

## Design

### Today background

```css
--color-today-tint: color-mix(in srgb, var(--kira-info) 7%, transparent);
```

Faint blue from the status `info` token (`#69a1e8`). On chrome it resolves near `#181c22`. Cards
stay distinct by hue, not by a 4-step luminance gap. Not amber (rejected before), not grey (this
complaint). `info` over `accent` because `accent`/`select` mean selection; a selected card head is
solid `bg-select` (`#1f3354`), far stronger than a 7% wash, so the two never read alike.
Translucent, so it composes over any ground and under the weekend stripe. Keep the existing rounded
band fill (`rounded-kira`) on the root; only the token value changes.

Fallback if the user still dislikes it: drop the fill and let the today bar plus head rule carry
today. Not built; orchestrator raises it only if asked.

### Day bar

One vertical bar per day band, on its left edge, from the top of the date heading to the bottom of
the day's last row. Rounded caps mark start and end. 4px inset at top and bottom leaves an 8px gap
between consecutive days, so each start and end reads on its own.

- Placement: in the plan's existing 16px `px-4` gutter, outside the band box. Band root gets
  `relative`; bar is `absolute -left-2.5 top-1 bottom-1` (band x minus 10px). No band padding
  change, so card width, card inset (spec "the timeline has no date gutter" asserts inset <= 8px) and
  every existing card baseline stay unchanged.
- Shape: `rounded-full`, `pointer-events-none`, `aria-hidden="true"`,
  `data-testid="ade-day-bar"`.
- Other days: `w-0.5` (2px), `bg-day-bar`. New token
  `--color-day-bar: color-mix(in srgb, var(--kira-fg) 18%, var(--kira-border));` (about `#4e4e52`),
  between the dim `bg-border/50` rule and today's rule.
- Today: `w-0.75` (3px), `bg-today` (same token as today's head rule, about `#6b6b6f`): slightly
  wider, slightly lighter, same grey family. Bar and rule tie visually.
- Past (history), weekend and day-off bands: same as other days. Later band: no bar
  (`v-if="!band.isLater"`); it is a tail bucket, not a day.
- Empty day: bar spans head plus the `min-h-1` drop area, still capped and gapped.

Tailwind v4 default spacing (`0.25rem`, no `--spacing` override in `packages/theme`) makes `w-0.75`
3px and `-left-2.5` -10px. Utility classes only, no scoped `<style>`. No shadcn-vue primitive fits a
decorative bar (plain `span`). No VueUse, Pinia or TanStack Query concern: static presentation off
existing props.

## Files

| File | Change |
|---|---|
| `apps/kira-space/frontend/src/ade/v2/tones.css` | `--color-today-tint` value; add `--color-day-bar` |
| `apps/kira-space/frontend/src/ade/v2/plan/AdeDayBand.vue` | root `relative`; bar `span`; `barClass` computed |
| `apps/kira-space/tests/ui/ade-v2-plan.spec.ts` | new test after line 667 (bar geometry, today bar, today bg token) |
| `apps/kira-space/tests/visual/ade-plan-days.spec.ts` | new visual spec |
| `apps/kira-space/tests/visual/ade-plan-days.spec.ts-snapshots/plan-day-bars-visual-linux.png` | new baseline |
| `docs/v0.3.14/SPEC.md` | P642 row Done plus Result (implementer's closing commit) |

No change to `AdePlanView.vue`, `timeline.ts`, Go code, i18n (bar is `aria-hidden`, no text) or
`packages/theme` (ADE tones live in `tones.css`, built on `--kira-*`).

Overlap: none with P641 (link order: `ade/v2/links/*`, link strip, `tests/ui` and `tests/visual`
link specs) or P640 (API module tab close). `ade-v2-plan.spec.ts` is P642 only unless P641's plan
names it; if it does, both edits are additive new `test(...)` blocks and rebase cleanly.

## Implementer steps

1. `tones.css`: replace the `--color-today-tint` line, add `--color-day-bar` under `--color-today`:

   ```css
   --color-today: color-mix(in srgb, var(--kira-fg) 35%, var(--kira-border));
   --color-today-tint: color-mix(in srgb, var(--kira-info) 7%, transparent);
   --color-day-bar: color-mix(in srgb, var(--kira-fg) 18%, var(--kira-border));
   ```

2. `AdeDayBand.vue` script: add

   ```ts
   const barClass = computed(() => (props.band.isToday ? 'w-0.75 bg-today' : 'w-0.5 bg-day-bar'));
   ```

3. `AdeDayBand.vue` template: root class `flex flex-col rounded-kira` becomes
   `relative flex flex-col rounded-kira`. First child of the root, before `ade-band-head`:

   ```vue
   <span
     v-if="!band.isLater"
     class="pointer-events-none absolute -left-2.5 top-1 bottom-1 rounded-full"
     :class="barClass"
     aria-hidden="true"
     data-testid="ade-day-bar"
   />
   ```

   Keep `bg-today-tint` on the root as is. Update the file's header comment only if it lists band
   parts (it does: add "left day bar").

4. UI spec, `ade-v2-plan.spec.ts`, new test `each day has a left bar from its heading to its last
   row; today's is wider and lighter` (see Oracles). Reuse `openPlan`, `bandOf`; set viewport
   `1440x2400` so all bands render.

5. Visual spec `tests/visual/ade-plan-days.spec.ts` (pattern: `ade-plan-branches.spec.ts`): open
   plan, viewport `1400x1800`, compute clip from `bandOf(page, 0)` and `bandOf(page, 1)` boxes:
   `x = band0.x - 16`, `y = band0.y`, `width = band0.width + 16`,
   `height = band1.y + band1.height - band0.y`; `await expect(page).toHaveScreenshot('plan-day-bars.png', { clip })`.
   Covers today fill, today bar, a normal day bar and the gap between them.

6. Record only the new baseline (Inter present: `fc-match sans-serif` prints `Inter-Regular.otf:
   "Inter" "Regular"` on this box, checked during planning; recheck first, never record otherwise per
   `docs/DEV_ENVIRONMENT.md` "tests/visual/* pixel diffs"):
   `bun run build:test && bunx playwright test --config=apps/kira-space/playwright.config.ts --project=visual ade-plan-days --update-snapshots`.
   Run it twice without `--update-snapshots` to confirm it is stable. Disk is about 5 GB free: no
   other build, clean `test-results/` after.

7. Fast checks per commit: `bun run typecheck:web`, `bun run lint` (runs `check-ade-colours.sh`,
   `check-tokens.sh`, class-conflict check). Run own specs once at the end:
   `bunx playwright test --config=apps/kira-space/playwright.config.ts --project=ui ade-v2-plan`.

8. Commits: `fix: today background and day bars in ADE task list` (steps 1 to 3),
   `test: day bar geometry and today background` (steps 4 to 6), then SPEC Result.

## Oracles

UI spec (`ui` project), all from computed style and bounding boxes:

- Every band with `data-ade-day != "9999"` has exactly one `[data-testid="ade-day-bar"]`; the Later
  band (if present) has none.
- Left edge: `bar.x + bar.width <= band.x` and `bar.x >= band.x - 16` (inside the gutter, outside the
  band).
- Span: `|bar.y - (band.y + 4)| <= 1` and `|(bar.y + bar.height) - (band.y + band.height - 4)| <= 1`.
  For a band with cards or span rows: head top >= bar top, last `ade-card` / `ade-span-row` bottom
  <= bar bottom. So the bar covers exactly that day's heading and rows.
- Day separation: for consecutive bands, `bar[i].bottom < bar[i+1].top` (gap visible).
- Rounded caps: computed `border-radius` (parsed px) >= `width / 2`.
- Today differs: today bar computed `width` is `3px`, other bars `2px`; today bar `backgroundColor`
  differs from another day's and its luminance is higher (reuse the `rgb`/`lum` helpers of the test
  at line 630; lift them to file scope if both tests use them).
- Today bar colour equals today head rule colour (same `--color-today` token).
- Today background token: append a probe `div` with inline `background: var(--color-today-tint)` to
  `document.body`, read its computed `backgroundColor`, remove it; today band computed
  `backgroundColor` equals it. Hue check: blue channel > red channel. Other bands stay
  `rgba(0, 0, 0, 0)`.
- Existing test "today is a subtle grey, not yellow, not like other days" must still pass unchanged
  (it asserts the head rule and non-transparent today fill only).
- Existing test "the timeline has no date gutter" must still pass (card inset <= 8px unchanged).

Visual (`visual` project, PR CI): new `plan-day-bars-visual-linux.png`.

Existing baselines: none should change. No visual spec captures a band. `ade-plan-branches` shoots
cards `T_search`, `R_li`, `T_deps` and row `b_auth`, none in today's band (fixture today
2026-09-22 holds `T_bill`); the bar sits outside card boxes. `ade-sandbox` and `ade-sessions-menu`
open from `T_bill` but shoot opaque popovers. If CI flags one of these anyway, re-record that file only
from this box after the `fc-match` check, with `--update-snapshots` scoped to the spec.

Presentation screenshots (`bun run screenshots`, `tests/screenshots/build/*.shot.ts`: `startTask`,
`branchesHandover`, `proof`, `dbPrompt` open `ade-plan`) are gitignored; CI builds the published set,
so the "screenshot refresh" there is automatic. Optional eyeball:
`bunx playwright test --config=apps/kira-space/playwright.config.ts --project=screenshots startTask`;
never beside a UI suite in the same checkout.

## Tests: no flow half

Presentation only: no IPC call, payload, Go type or backend state changes. The CLAUDE.md
backend-plus-frontend split has nothing to assert on the Go side, so this phase ships no flow test.
No unit test either (no logic beyond one ternary).

## Verification before accepting

- `git diff ee9e0d597 --stat` touches only the files in the table.
- grep: `bg-today-tint`, `bg-today`, `bg-day-bar` used only in `AdeDayBand.vue`; `--color-day-bar`
  defined once in `tones.css`; no hex or `rgb(` literal added under `frontend/src/ade/`.
- No `<style>` block in `AdeDayBand.vue`.
- New UI test and the two existing band tests pass; new visual spec passes twice without update.
- `typecheck:web` and `lint` clean; pre-commit hook passes without `--no-verify`.
