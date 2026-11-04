# P505 plan: flat minimalist redesign

Row: `SPEC.md` P505. Base: `ec95692e` (chapter open). One sequential Sonnet implementer.

Discovery: `codegraph_explore` (token readers, Monaco/xterm/chart themes, colour tables), then
Grep/Read for CSS, lint scripts and specs (CSS is not in the symbol index).

## 1. What the design surface is

The app is already token-central. Nearly every colour reaches the screen through `--kira-*` in
`packages/theme/src/tokens.css`, then one of three paths:

- Tailwind utilities via `@theme` in `packages/theme/src/tailwind-core.css`. Usage sites:
  `border-border` 192, `bg-field` 63, `bg-elevated` 58, `bg-bg` 49, `bg-chrome` 41,
  `border-border-strong` 41, `rounded-kira-sm` 69, `rounded-kira` 40, `rounded-kira-xs` 25.
- `var(--kira-*)` in the CSS files: `base.css`, `review-decorations.css`, `blame-annotation.css`,
  `primitives.css`, `workbench/src/editor/edDecorations.css`, `git-ui/src/theme/git.css` (lanes are
  `--kira-conn-*`), `kira-studio/.../slick/slickTheme.css`.
- JS reads via `getComputedStyle`: Monaco `defineKiraTheme` (`workbench/src/editor/monacoTheme.ts`),
  xterm `terminalTheme` (`workbench/src/terminal/terminalRenderer.ts`), ECharts `chartTheme`
  (`kira-studio/frontend/src/charts/theme.ts`). Each re-reads tokens. Changing a token recolours
  all three with no code change.

Derived tables that follow tokens with no edit: `connColor.ts` (`bg-conn-*`), `methodColor.ts`
(`--kira-method-*` aliases conn hues), status `--kira-status-*`, graph lanes, chart series,
`--kira-search-match*`, shadcn bridge (`--primary`, `--border`, `--muted-foreground`).

Hard-coded colour literals outside `tokens.css` (`.vue/.ts/.css`, specs excluded):

- `monacoTheme.ts` 83: `cssVar(name, fallback)` fallbacks. Dead at runtime but must mirror tokens.
- `ade/v2/tones.css` 21 and `ade/v2/palette.ts` 20: deliberate literals, pinned by
  `scripts/check-ade-colours.sh` allowlist (fails on unknown literal and on stale entry).
- `EngineIcon.vue` 32, `setiFileIcon.ts` 15: brand and file-type identity colours.
- `docker-ui/.../OriginGlyph.vue:11`: `style="color: #00b4ff"`.
- `slickTheme.css` 4: comments only. `ansi.ts` 3: program output colours. git-ui `#123`: PR
  numbers in comments. Testing helpers 3: not app UI.

Outside the frontend: `internal/shell/window.go:77` and `internal/shell/popup.go:20`
`BackgroundColour: application.NewRGB(24, 24, 24)` (mirrors `--kira-bg-chrome`);
`apps/kira-space/frontend/mobile/index.html` `theme-color` and
`mobile/public/manifest.webmanifest` `background_color`/`theme_color` `#181818`. Go palettes
(`AdeStatusColors`) are names only; no Go hex reaches the UI.

Shadows, gradients, rings:

- `shadow-kira` 6 sites (popover, dropdown, sub-menu, tooltip, native-select picker,
  `ErrorPopover`), `shadow-kira-dialog` 3 (dialog, command dialog, git-ui `App.vue:1906`). All
  pair with `ring-1 ring-fg/10`, a hairline. Token-driven.
- Inset `shadow-[inset…]` 4 sites are hairline rails (resizable handle, `DocumentRow`,
  `OpLogPanel`). Not depth. Keep.
- Gradients 2: `swatch-none` (the "no colour" slash glyph) and `AdeDayBand` hatch pattern. Both
  carry meaning. Keep.

Light theme: none exists. `index.html` in both apps (and mobile, proto) hardcodes
`class="dark"`; `tokens.css` is a single `:root`; no setting switches theme; Monaco is `vs-dark`.
`proto/grid/theme-alt.css` is a prototype-only light variant, not shipped. Adding a light theme
needs a switch (setting, Appearance control, class swap): a feature. The row forbids feature
change, so this plan ships one retuned dark palette. **Open for the user:** if a light theme is
wanted, it is a separate phase. The orchestrator raises this; the implementer does not build it.

## 2. Lints and tests that constrain the work

- `scripts/check-tokens.sh`: every `var(--kira-*)` must resolve. No token is added or removed, so
  it stays green.
- `scripts/check-theme-classes.sh`: no stock `shadow-sm…2xl`, no `rounded-sm/md/lg/xl`, no shadcn
  colour aliases, focus ring only via `focus-ring`. This plan uses only existing kira names.
- `scripts/check-class-conflicts.ts`: every class must resolve through tw-merge config
  (`packages/theme/src/lib/utils.ts`). `rounded-kira-lg` is registered there (line 46).
- `scripts/check-ui-primitives.sh`: element rules (colours U-rules). Run it; fix what it flags.
- `scripts/check-ade-colours.sh`: allowlist must equal the literal set under `ade/` exactly.
- No contrast lint exists. Contrast floors live in `tokens.css` comments: `--kira-fg-subtle`
  ≥4.5:1 on `--kira-bg` and `--kira-bg-elevated`; status palette ≥6.4:1 on elevated. This plan
  keeps them and adds: text tokens ≥4.5:1 on every surface they sit on; focus ring and accent
  fill ≥3:1 (WCAG 1.4.11) on bg; white on accent ≥4.5:1.

Specs asserting a literal token value (found by grepping every old value's `rgb()` triple and
hex across `apps/*/tests`, `packages/*/src`, `site/tests`):

| Spec | Line | Old | New |
|---|---|---|---|
| `apps/kira-studio/tests/ui/control-sizing.spec.ts` | 120 | `rgb(0, 120, 212)` focus | `rgb(90, 138, 230)` |
| `apps/kira-studio/tests/ui/data-view.spec.ts` | 1566 (comment 1541) | `rgb(55, 148, 255)` info | `rgb(105, 161, 232)` |
| `apps/kira-studio/tests/ui/api-ui-consistency.spec.ts` | 165-167 | `rgb(110, 110, 110)` fg-disabled | `rgb(99, 99, 107)` |
| same | 415, 424 | `rgb(78, 201, 176)` var-resolved | `rgb(100, 190, 173)` |
| same | 415, 425 | `rgb(204, 167, 0)` warn | `rgb(213, 180, 73)` |
| same | 1002-1003 | `rgb(49, 49, 49)` bg-input | `rgb(39, 39, 44)` |
| `apps/kira-studio/tests/ui/row-coloring.spec.ts` | 24 | `rgb(181, 206, 168)` syntax-number | `rgb(180, 199, 158)` |
| same | 25 | `rgb(204, 204, 204)` fg | `rgb(212, 212, 216)` |
| same | 90 (comment) | `rgb(206, 145, 120)` syntax-string | `rgb(210, 154, 126)` |
| `apps/kira-studio/tests/ui/http-request-body.spec.ts` | 2 sites | `rgb(86, 156, 214)` syntax-keyword | `rgb(110, 159, 216)` |
| `apps/kira-space/tests/ui/ade-v2-plan.spec.ts` | 542 | `rgb(240, 184, 92)` tone-amber | `rgb(229, 180, 115)` |

Update the hex in each neighbouring comment too. Not touched: `ade-v2-board-parity.spec.ts`
(`#93b6ff` comes from its mockup fixture, not from `tones.css`), `setiFileIcon.spec.ts` (icons
unchanged), `workbench.spec.ts` (asserts "not white"). Rerun the grep after the token commits:
any remaining hit of an old triple is a missed spec.

Visual baselines: all 55 PNGs change (`apps/kira-studio/tests/visual/*-snapshots` 22,
`apps/kira-space/tests/visual/*-snapshots` 33). Not re-recorded in this phase. They re-record
once in the chapter's full suite run, after `fc-match sans-serif` prints Inter
(`docs/DEV_ENVIRONMENT.md` visual section). Review recorded diffs then: only colour may differ, no
geometry. Presentation `screenshots/` and `site/tests/visual.spec.ts-snapshots/` also change;
§7 commit 5 handles them.

## 3. Design direction

Neutral, slightly cool zinc greys. Fewer, smaller surface steps. Hairline borders do the
separating; shadows only lift floating surfaces a little. One muted blue accent. State and
identity colours lose chroma so nothing shouts. Same fonts, sizes, spacing, border widths, radii
scale, markup.

Flat rules:

1. Depth comes from 1px hairlines and surface steps, not shadows. Floating surfaces keep their
   existing `ring-1 ring-fg/10` hairline plus a much softer shadow.
2. No new gradients, glows or halos. Existing meaningful ones (slash glyph, day hatch) stay.
3. Border *widths* never change (a width change moves boxes). Flattening a heavy border means a
   quieter border *colour*.
4. Surface ladder, darkest to lightest: chrome < bg < elevated < hover < field (input). Steps
   1.05-1.16:1, enough to read, never boxy.
5. Accent marks primary action, selection and focus only. Status and identity hues stay muted.
6. Hover is one surface step up (`bg-hover`); pressed/selected is `bg-select` (blue-tinted) or
   `bg-field`. Disabled stays `opacity-50`. Existing state classes stay; tokens change their look.
7. Focus: the one `focus-ring` utility, 1px inset, unchanged mechanism. Colour `--kira-focus`
   moves to a lighter blue than the accent fill so the ring reads on the accent button too.

### 3.1 Dark palette (`tokens.css`)

Ratios computed with WCAG relative luminance (script kept out of the repo).

| Token | Old | New | Check |
|---|---|---|---|
| `--kira-bg-chrome` | `#181818` | `#161618` | bg/chrome 1.05 |
| `--kira-bg` | `#1f1f1f` | `#1b1b1e` | |
| `--kira-bg-elevated` | `#202020` | `#1f1f23` | elevated/bg 1.05 |
| `--kira-bg-input` | `#313131` | `#27272c` | input/bg 1.16 |
| `--kira-hover` | `#2a2d2e` | `#26262a` | hover/bg 1.14, hover/chrome 1.20 |
| `--kira-select` | `#04395e` | `#1f3354` | select/bg 1.36; fg on it 8.56 |
| `--kira-fg` | `#cccccc` | `#d4d4d8` | 11.63 bg, 10.06 input |
| `--kira-fg-muted` | `#9d9d9d` | `#a1a1aa` | 6.70 bg, 5.80 input, 4.94 select |
| `--kira-fg-subtle` | `#8a8a8a` | `#8e8e97` | 5.29 bg, 5.06 elevated, 4.58 input |
| `--kira-fg-disabled` | `#6e6e6e` | `#63636b` | exempt (WCAG 1.4.3), 2.89 bg |
| `--kira-border` | `#2b2b2b` | `#28282d` | hairline, 1.17 bg |
| `--kira-border-strong` | `#313131` | `#37373d` | 1.45 bg, 1.26 input |
| `--kira-accent` | `#0078d4` | `#3b6ed1` | 3.55 bg; white text 4.84 |
| `--kira-accent-fg` | `#ffffff` | unchanged | |
| `--kira-focus` | `#0078d4` | `#5a8ae6` | 5.08 bg, 4.40 input, 3.74 select |
| `--kira-badge` | `#616161` | `#4a4a52` | fg on it 5.94 |
| `--kira-scrollbar` | `#79797966` | `#71717a59` | |
| `--kira-scrollbar-hover` | `#646464b3` | `#8e8e9780` | |
| `--kira-scrollbar-active` | `#bfbfbf66` | `#a1a1aa80` | |
| `--kira-scrim` | `rgb(0 0 0 / 0.3)` | `rgb(0 0 0 / 0.4)` | dialog separation, shadow is lighter |
| `--kira-error` | `#f14c4c` | `#e66e68` | 5.56 bg, 4.81 input |
| `--kira-warn` | `#cca700` | `#d5b449` | 8.55 bg; bg text on it 8.55 |
| `--kira-ok` | `#23d18b` | `#57bc8a` | 7.34 bg |
| `--kira-info` | `#3794ff` | `#69a1e8` | 6.43 bg |
| `--kira-state-on` | `#e0a33c` | `#e79f55` | 7.76 bg; oklch hue 65 vs warn 92 (was 77 vs 93) |
| `--kira-var-resolved` | `#4ec9b0` | `#64bead` | 7.78 bg; distinct from syntax-property |
| `--kira-warn-text` | `#d9c47a` | `#d5c488` | 9.89 bg |
| `--kira-note-text` | `#a8c8ee` | `#acc7e8` | 9.89 bg |
| `--kira-shadow` | `0 2px 8px rgb(0 0 0 / 0.32)` | `0 2px 6px rgb(0 0 0 / 0.18)` | |
| `--kira-shadow-dialog` | `0 8px 28px rgb(0 0 0 / 0.45)` | `0 6px 20px rgb(0 0 0 / 0.28)` | |

Derived tokens (`--kira-search-match*`, `--kira-method-*`) keep their expressions.

Connection palette: `oklch(0.72 0.075 h)` (was 0.09 chroma), grey `oklch(0.72 0.012 250)`. All
≥6.36:1 on elevated.

| red | orange | amber | olive | green | teal | cyan | blue | indigo | violet | magenta | grey |
|---|---|---|---|---|---|---|---|---|---|---|---|
| `#d0938d` | `#ca9877` | `#baa16e` | `#a3ab74` | `#83b28c` | `#6db4a6` | `#68b3bb` | `#81a9d2` | `#96a1d4` | `#ac9acd` | `#c592b3` | `#9fa5ac` |

Status palette: `oklch(0.78 0.095 h)` (was 0.11), grey 0.015, brown 0.05. All ≥7.8:1 on
elevated (floor 6.4).

| grey | brown | red | orange | amber | yellow | lime | green |
|---|---|---|---|---|---|---|---|
| `#b0b8c1` | `#d1b099` | `#eea099` | `#e9a680` | `#dbaf71` | `#cab76f` | `#a9c27f` | `#8bc997` |

| teal | cyan | sky | blue | indigo | violet | purple | pink |
|---|---|---|---|---|---|---|---|
| `#6cccba` | `#65c9d4` | `#6fc4e5` | `#8ebaf4` | `#a5b3f4` | `#bbacee` | `#cfa5e1` | `#e69ec0` |

Syntax (Monaco via tokens): same hue roles as Dark Modern, lower chroma. All ≥4.5:1 on bg.

| comment | string | number | keyword, tag | control | name, property, attribute | function | operator | punctuation | meta | invalid |
|---|---|---|---|---|---|---|---|---|---|---|
| `#7f9a6f` 5.53 | `#d29a7e` 7.09 | `#b4c79e` 9.49 | `#6e9fd8` 6.23 | `#c296c4` 6.92 | `#93c6e8` 9.40 | `#d8d2a0` 11.17 | `#c8c8cc` 10.30 | `#bdbdc4` 9.20 | `#8e8e97` 5.29 | `#e66e68` 5.56 |

Radius scale: values unchanged (2/4/6/8/10px). They already read modern and are layout-neutral;
the change is tier use, §5.

### 3.2 ADE tones and work palette (Space)

Literal by design (queue state, not theme). Retune to the same muted family: text
`oklch(0.80 0.10 h)`, solid `oklch(0.70 0.12 h)`, tint = solid at the old alpha. Purple and grey
solids keep their ink, so they are re-picked for ink contrast.

| Tone | tint | text | solid | ink on solid |
|---|---|---|---|---|
| amber | `rgba(203, 146, 63, 0.14)` | `#e5b473` | `#cb923f` | `#15161a` 6.65 |
| red | `rgba(223, 128, 114, 0.14)` | `#f7a599` | `#df8072` | 6.43 |
| green | `rgba(92, 180, 123, 0.14)` | `#8ad1a0` | `#5cb47b` | 7.12 |
| blue | `rgba(118, 157, 233, 0.14)` | `#9cbeff` | `#769de9` | 6.69 |
| purple | `rgba(128, 93, 192, 0.16)` | `#c5b0f6` | `#805dc0` | `#ffffff` 4.95 (was 3.35) |
| grey | `#232327` | `#babec4` | `#7d8088` | `#15161a` 4.58 (was 3.60) |

`--color-tone-ink` `#15161a`, `--color-tone-ink-light` `#ffffff`, `--color-agent` `#d97757`
(Claude brand) unchanged.

Work palette (`palette.ts`, index stored server-side, so values are free to change): same hue,
chroma ×0.8, lightness capped at 0.74. All ≥4.65:1 on bg; 20 distinct.

```
#d38261 #d2a155 #b4b04d #90ba66 #64b477 #57b1a0 #56a6c2 #5a8fd5 #727ed7 #9676d1
#bc72cb #d36fa4 #d56c7e #b18765 #95a269 #669d8e #8092b0 #a182b7 #cc9191 #a4aab2
```

Slot order matches the old array one to one.

Literal formatting: write each value exactly as the check normalises it. The script lowercases
and strips spaces, so `rgba(203, 146, 63, 0.14)` in CSS becomes `rgba(203,146,63,0.14)` in the
allowlist. Replace every old allowlist line with its new value; leave unchanged ones.

## 4. Token-first strategy

Order: tokens first, then the few class edits, then literals outside tokens. Each commit leaves
lint, typecheck and its touched specs green.

Files that change, exhaustive:

- `packages/theme/src/tokens.css`: §3.1 values. Rewrite the header (no longer "VS Code Dark
  Modern derived") and every comment that cites an old hex or ratio. Keep comments short; drop
  history that only explained an old value.
- `packages/workbench/src/editor/monacoTheme.ts`: every `cssVar` fallback literal and the
  `tokenRules` fallbacks mirror the new token values. Fix the two comments citing old literals
  (`#202020`, `#79797966`, `0 2px 8px …`). Mapping unchanged.
- `internal/shell/window.go:77` and `internal/shell/popup.go:20`: `NewRGB(22, 22, 24)`; update the
  `#181818` comment.
- `apps/kira-space/frontend/mobile/index.html` `theme-color`, and
  `mobile/public/manifest.webmanifest` `background_color`, `theme_color`: `#161618`.
- `packages/theme/src/review-decorations.css`: comment only if it cites a value; rules use tokens.
- `apps/kira-space/frontend/src/ade/v2/tones.css`, `ade/v2/palette.ts`,
  `scripts/check-ade-colours.sh` allowlist: §3.2.
- Component classes, §5.
- Specs, §2 table.
- `docs/ARCHITECTURE.md` line 49 ("tokens mirror VS Code Dark Modern"): replace with one line
  stating the flat zinc palette and that `tokens.css` is its only source.

Not edited, follow tokens automatically: `tailwind-core.css`, `shadcn-bridge.css`, `base.css`,
`primitives.css`, `git.css`, `slickTheme.css`, `edDecorations.css`, `blame-annotation.css`,
`terminalRenderer.ts`, `charts/theme.ts`, `connColor.ts`, `methodColor.ts`, every `.vue` using
utilities.

## 5. Component class edits (styling only, same box metrics)

1. Dialog radius to the tier `tokens.css` already names for modal dialogs (`lg`), off the status
   chip tier (`pill`): `rounded-kira-pill` to `rounded-kira-lg` in
   `packages/theme/src/components/ui/dialog/DialogContent.vue:121`,
   `components/ui/command/CommandDialog.vue:38`, `components/ui/command/Command.vue:99` (keep its
   `!`), `packages/workbench/src/prompt/TextPromptDialog.vue:56`. Badge and `AdeEndNode` keep pill.
2. Flat command input: `components/ui/command/CommandInput.vue:32`
   `bg-border-strong/30 border-border-strong/30` to `bg-field border-transparent`.
3. Hairline instead of strong border on passive containers:
   `components/ui/card/index.ts:18` and `components/ui/kbd/Kbd.vue:13` `border-border-strong` to
   `border-border`. Inputs, checkbox, radio, switch, select, textarea keep `border-border-strong`:
   a field edge is functional.
4. ADE task card heavy border: `apps/kira-space/frontend/src/ade/v2/plan/AdeTaskCard.vue:71`
   `border-badge` to `border-border-strong` (selected state `border-focus ring-1 ring-focus`
   unchanged).
5. Hard-coded literal to token: `packages/docker-ui/src/components/OriginGlyph.vue:11`
   `style="color: #00b4ff"` to `class="text-info"`.

Nothing else in markup moves. No new token, no new `@theme` name, no scoped `<style>`.

## 6. Not touched

- Layout, spacing, sizes, control heights, type scale, fonts, border widths, z-ladder, motion
  tokens, markup, component choice, behaviour.
- Brand and identity literals: `EngineIcon.vue` (simple-icons brand hex and its `#ececec` halo),
  `setiFileIcon.ts`, `--color-agent`.
- `ansi.ts` and xterm's 16 ANSI colours (program output; terminal chrome follows tokens).
- `proto/grid/theme-alt.css` and `proto/**` (prototype, not shipped).
- Semantic gradients (`swatch-none`, `AdeDayBand` hatch), inset hairline shadows, the
  `AdeActivityIcon` working halo (a status signal), button `active:translate-y-px`.
- Light theme (§1).

## 7. Commits

Conventional Commits; each ends with the session trailers.

1. `feat(theme): flat zinc surfaces, text, accent and shadows`: `tokens.css` §3.1 surface/text/
   border/accent/focus/select/hover/badge/scrollbar/scrim/state/alert-text/shadow rows;
   `monacoTheme.ts` fallbacks; Go window and popup background; mobile `theme-color` and manifest;
   `ARCHITECTURE.md` line; the §2 specs for focus, info, fg-disabled, var-resolved, warn,
   bg-input, fg.
2. `feat(theme): muted connection, status and syntax palettes`: conn, status, syntax rows; Monaco
   fallbacks for syntax; `row-coloring.spec.ts`, `http-request-body.spec.ts`.
3. `feat(space): muted ADE tones and work palette`: `tones.css`, `palette.ts`,
   `check-ade-colours.sh`, `AdeTaskCard.vue`, `ade-v2-plan.spec.ts`.
4. `feat(theme): flatten dialog, command, card and kbd chrome`: §5 items 1, 2, 3, 5.
5. `chore: re-capture presentation screenshots`: `screenshots/**` from `bun run screenshots`,
   then site visual baselines
   (`bun run site:build && bunx playwright test --config=site/playwright.config.ts visual --update-snapshots`).
   Rerun `bun run screenshots`; `git status --porcelain screenshots/` must be empty.

## 8. Verification per commit

- `bun run typecheck`, `bun run lint` (biome plus every `lint:rules` script), `bun run build:studio`
  and `bun run build:space` (Space includes mobile).
- Commit 1: `go build ./internal/shell/... && go test ./internal/shell/...`; the pre-commit hook
  also runs `bun run lint:go` scope.
- Targeted specs, built bundles first (`bun run build:test:studio`, `bun run build:test:space`):
  - Studio `--project=ui`: `control-sizing`, `data-view`, `api-ui-consistency`, `row-coloring`,
    `http-request-body`, `focus-ring`, `color-rails`, `workbench`, `consistency`, `font-roles`.
  - Space `--project=ui`: `ade-v2-plan`, `focus-ring`, `color-rails`, `window-chrome`,
    `ade-v2-card-icons`.
  - Space unit: `bun test apps/kira-space/tests/unit/ade-v2-board-parity.spec.ts`.
- After commit 2, grep `apps/*/tests packages/*/src site/tests` for every old `rgb()` triple and
  hex from §3.1. Zero hits outside comments you rewrote.
- Not here: the full UI/e2e/Go suites and the 55 visual baselines. They run once in the chapter's
  single full run.

## 9. Visual self-check (implementer, before commit 5)

Readability is the risk, so look at real pixels:

1. `bun run screenshots`. Open all six `screenshots/*-detail.png` and the framed ones with Read.
2. Run both visual projects once without update:
   `bunx playwright test --config=apps/kira-studio/playwright.config.ts --project=visual` and the
   Space equivalent. They fail by design. Open every `test-results/**/*-actual.png` with Read, with
   its `*-expected.png` beside it. Never run this beside another UI suite in the same checkout.
3. Check each image for: text legible on every surface (muted and subtle text in grids, status
   bar, tree rows); selected row distinct from hover; inputs findable against panels; dialogs and
   popovers separated from what is under them; focus ring visible on a focused input and on the
   primary button; graph lanes, connection rails, status chips and method labels still
   distinguishable; Monaco syntax readable; geometry identical to the expected image (any moved
   edge is a regression: fix it).
4. If a pair is too close in practice, adjust that token within §3 rules, recompute its ratio, and
   note the final value in the commit body. Do not re-record any baseline. Delete `test-results/`.

## 10. Streams

One sequential implementer. Commits 1, 2 and 4 all edit `tokens.css` or its specs and share the
§2 spec files; commit 5 depends on every earlier one. Commit 3 (ADE) is file-disjoint, but too
small to pay for a worktree, and the §9 self-check covers both apps at once. No split.

## 11. Result

Pending.
