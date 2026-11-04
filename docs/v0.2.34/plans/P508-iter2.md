# P508 iter2 plan: flatter, consistent visual pass (amended)

Base: `4861680cd`. Supersedes `P508.md` where they differ. `P508.md` §2 separator rules still stand, amended below.
One sequential Sonnet implementer. Styling only.

## Why amended

User feedback after the first pass:

1. Flat buttons are bordered. Outline/secondary buttons keep a visible 1px border.
2. Too many textures: shadows, ring+shadow stacks, inset rails, hatch, dashed edges, mixed alpha tints.
3. Too many greys: 4 surface steps plus hover, 2 borders, 4 text greys, badge, tone-grey-tint, scrollbar alphas, syntax greys.
4. Radius: 5 tokens plus arbitrary values and one-offs. Unify on one small scale.

## Standing constraints (from P508.md, unchanged)

- Class strings, token values, `@theme` maps, existing CSS files only. No component swaps, no new props, no markup moves, no new scoped `<style>`. Hex only in `packages/theme/src/tokens.css` (plus Monaco fallbacks and the ADE 20-slot `palette.ts`).
- Keep heights, widths, padding, gaps. A border carrying state flips to `border-transparent`, never drops its width.
- Separators only where `P508.md` §2 rules 1-7 allow.
- P509 files untouched: `ade/v2/panel/AdeTaskPanel.vue`, `AdeMainTab.vue`, `AdeMainBranches.vue`, `AdeTaskIssueLine.vue`, `useLinkFields.ts`, `AdeLinkGroups.vue`, `details/*` (last two do not exist yet). Token value changes reach them; no edit to their source.
- No new tests. Baselines re-record once after P509.

## Current tree (re-read 2026-11-04)

### Landed commits

- `5ceba8815` tokens and tones: chrome `#131315`, hover `#242427`, border `#2b2b30`, syntax aliases, warn/note-text folds, `--kira-agent`, ADE tones on theme tokens (tint `color-mix(... 14%)`), purple ink, `check-ade-colours.sh` palette-only, Monaco fallbacks, `rounded-kira-md` fix. **Keep.** W1 changes values on top.
- `42bf79702` primitives: Card/CardHeader/CardFooter/Table/Dialog header-footer/Alert/Kbd/paneFrame borders gone, active tab `border-transparent`, `title` button `aria-pressed` border gone, read-only Input `border-transparent`, floating surfaces `ring-1 ring-border-strong`. **Keep.** W3 swaps the ring for a border, W4/W5 retune Alert tint and radii.
- No button border was removed. `dialog`, `dialog-primary`, `dialog-danger` still carry `border`; toolbar/ghost/title never had one. Nothing to restore.

### Uncommitted (33 files, stopped implementer)

All are separator removals from `P508.md` W3, plus the `ViewToolbar` `border` prop deletion and its 16 callers. Disposition:

- Keep 31 files as they are.
- Revert `packages/workbench/src/llmPrompts/PromptEditor.vue`: an editable prompt field, so rule 1 (input edge) applies. Run `git checkout -- packages/workbench/src/llmPrompts/PromptEditor.vue`.
- Fix `packages/workbench/src/components/ToolbarSeparator.vue`: keep `bg-transparent`, restore `mx-0.5` (was changed to `mx-1`, which widens every toolbar gap by 2px).

## Target vocabulary

### Neutral scale (final)

Four surfaces, one border, three text levels.

```
--kira-bg-chrome:   #131315   frame: title/status/tab bars, ADE plan ground
--kira-bg:          #1b1b1e   base: panes, editors
--kira-bg-elevated: #212125   raised + overlay: cards, table head, active tab, popover, menu, dialog, tooltip   (was #1f1f23)
--kira-bg-input:    #27272c   field + hover + pressed: inputs, inset card, kbd, row/button hover
--kira-border:      #323237   the one line: input edge, secondary button, floating edge, grid cell, resize, menu separator
--kira-fg:          #d4d4d8
--kira-fg-muted:    #a1a1aa
--kira-fg-subtle:   #8e8e97
```

Contrast checked (WCAG): fg-subtle 5.71 chrome, 5.29 bg, 4.94 elevated, 4.58 field. fg-muted 5.8 or higher on all four. White on new `--kira-accent-hover` 6.32.

`--kira-select`, `--kira-accent*`, `--kira-focus`, `--kira-scrim`, status/conn/method/syntax hues stay (not greys).

### Grey mapping, old to new

| Old token / utility | New | Consumers |
|---|---|---|
| `--kira-hover` `#242427`, `bg-hover` (+ `hover:`/`focus:`/`aria-*:` variants) | `--kira-bg-input`, `bg-field` | 22 hits / 13 files; `slickTheme.css:250`; Monaco 3 keys |
| `--kira-border-strong` `#37373d`, `*-border-strong` (border, ring, bg, outline, sides) | `--kira-border`, `*-border` | 69 hits / 42 files; `slickTheme.css:53,347`; `charts/theme.ts:30,35`; Monaco |
| `--kira-border` `#2b2b30` | `#323237` | value only |
| `--kira-bg-elevated` `#1f1f23` | `#212125` | value only |
| `--kira-fg-disabled` `#63636b`, `text-disabled` | `--kira-fg-subtle`, `text-subtle` | `AdeDayBand.vue:81,86`; `review-decorations.css:25` |
| `bg-disabled` (dots) | `bg-subtle` | `StatusDot.vue:21`, `AdeActivityIcon.vue:40`, `AdeArchivedPanel.vue:125` |
| `border-disabled` (empty swatch) | `border-border` | `connColor.ts:73` |
| `--kira-badge` `#4a4a52`, `bg-badge` | `bg-field` | `badge/variants.ts:18` |
| `--color-tone-grey-tint` | `var(--kira-bg-input)` | `tones.css` |
| `--kira-status-grey` `#b0b8c1` | `var(--kira-fg-muted)` | token alias |
| `--kira-conn-grey` `#9fa5ac` | `var(--kira-fg-subtle)` | token alias |
| `--kira-syntax-operator` `#c8c8cc`, `--kira-syntax-punctuation` `#bdbdc4` | `var(--kira-fg)` | token alias; Monaco fallbacks |
| `--kira-scrollbar` `#71717a59` | `var(--kira-border)` | token alias |
| `--kira-scrollbar-hover` `#8e8e9780`, `--kira-scrollbar-active` `#a1a1aa80` | `var(--kira-fg-subtle)` | token alias |
| `bg-bg/90` | `bg-bg` | `AdeTuiPane.vue:59` |

Deleted tokens: `--kira-hover`, `--kira-border-strong`, `--kira-fg-disabled`, `--kira-badge`, `--kira-shadow-dialog`, `--kira-radius-xs`, `--kira-radius-lg`, `--kira-radius-pill`. Deleted `@theme` names: `--color-hover`, `--color-border-strong`, `--color-disabled`, `--color-badge`, `--shadow-kira-dialog`, `--radius-kira-lg`, `--radius-kira-pill`. `--radius-kira-xs` stays one phase as `var(--kira-radius-sm)` (see Radius). Scrollbar, status-grey, conn-grey, syntax-operator/punctuation names stay as aliases (Monaco, xterm and palettes read them by name).

State distinctions that collapse would erase, kept with in-set tokens:

- `AdeDayBand.vue` `dotClass`: future dot `border-border-strong` becomes `border-subtle`; past stays `border-border`.
- `AdeTaskCard.vue` `segClass`: `todo` `bg-border-strong` becomes `bg-border`; `skipped` `bg-border` becomes `bg-field`.
- `AdeTaskCard.vue` `boxClass`: unselected `border-border-strong` becomes `border-transparent` (P508.md W6).
- `AdeDayBand.vue` greyed label/sub `text-disabled` becomes `text-subtle` (greyed and past now read alike; accepted).

### Texture vocabulary

Audit (`apps/*/frontend/src`, `packages/*/src`, `.vue/.ts/.css`):

- Shadows: `shadow-kira` 5 (popover, menu, sub-menu, tooltip, ErrorPopover) plus `native-select` picker; `shadow-kira-dialog` 4 (dialog, command dialog, git-ui drawer, `check-theme-classes.sh` text); arbitrary 5: `DocumentRow.vue:55` and `OpLogPanel.vue:398` inset 2px rails, `AdeActivityIcon.vue:27` halo, `resizable/index.ts:26,32` inset hairline. `shadow-none` 10 (resets).
- Rings: `ring-1 ring-border-strong` 7 floating surfaces (landed); `AdeTaskCard.vue:71` `border-focus ring-1 ring-focus`; `refBadges.ts:167` `ring-1 ring-inset ring-current`; `focus-ring` utility 14 in 8 files; `ring-0` 2 resets.
- Alpha fills, 41 hits at 10 strengths: `/5` 6, `/6` 1, `/7` 1, `/10` 4, `/14` 6 (tone tints), `/15` 17, `/16` 6, `/18` 2, `/20` 1, `/40` 1, `/80` 2, `/90` 1. Plus `color-mix` 8%/10%/14% in `slickTheme.css:384`, `review-decorations.css:35,42`, `ApprovalTarget.vue:35`, `OpLogPanel.vue:398`.
- Gradients: `AdeDayBand.vue:132` weekend hatch; `swatch-none` utility (no-colour glyph).
- Dashed edges: 6 `border-dashed` / 1 `outline-dashed` (AdeTaskCard ripple, AdeEndNode, AdeStepResults, AdeWorkflowGraph, AdeStepNode, AdeSessionBadge, VarText).
- `opacity-50` 34 (disabled), `opacity-0/100` 24 (show/hide). No backdrop-blur. `cn-menu-translucent` marker class: no CSS defines it.

Allowed set after this pass:

1. **Fill**: opaque surface tokens only. Status hues tint at exactly `/15` (or `color-mix(... 15%, transparent)`), only on chips, badges, alerts, tone tints, the danger hover, diff/review line washes, the error log row, the graph halo, the sparkline area. Exempt: `--kira-search-match` 25% (functional highlight).
2. **Edge**: `border border-border`, 1px. No ring for edges.
3. **Shadow**: one, `shadow-kira`, on floating surfaces and dialogs only. New value `0 4px 16px rgb(0 0 0 / 0.24)`.
4. **State marks**: `focus-ring`, `outline-1 outline-focus` (drop target, day highlight), tone borders, task-colour left edge, 2px inset rail (`DocumentRow`, `OpLogPanel`), resize-handle inset hairline.
5. **Opacity**: `opacity-50` disabled, `opacity-0/100` reveal. Nothing else.
6. Everything else goes: gradients (except `swatch-none` glyph), dashed edges, ring+border stacks, halo shadows, row washes.

Alpha mapping:

- `/10`, `/14`, `/16`, `/18`, `/20` on status hues: `/15`. Files: `alert/index.ts` (3), `button/index.ts` (`dialog-danger` hover only, see Buttons), `DropdownMenuItem.vue:34`, `badge/variants.ts` 14-17 (and its `/16` comment), `git-ui/lib/badgeClass.ts:21`, `git-ui/graph/rowSvg.ts:300` (and comment 298), `tones.css` six tints, `review-decorations.css:35,42`, `ApprovalTarget.vue:35` (14% to 15%), `OpLogPanel.vue:398` arbitrary to `bg-error/15`, `slickTheme.css:384` 8% to 15%.
- Row washes `/5`, `/6`, `/7`: delete. `AdeNeedsRow.vue:30-34` map values become `''` (keep the map; rail/icon/text carry tone), `AdeAllSessionRow.vue:50` binding drops the class (`:class` stays, value `''`), `AdeStepBlock.vue:86` current becomes `border-tone-amber` (no fill, full-strength tone edge).
- `bg-primary/80` (2): `bg-primary-hover`, new token `--kira-accent-hover: #355dad` (the old 80% blend over `--kira-bg`), mapped `--color-primary-hover` in `shadcn-bridge.css` next to `--color-primary`.
- `bg-bg/90`: `bg-bg`.

Other textures:

- Floating edge: the 7 landed `ring-1 ring-border-strong` become `border border-border`: `DialogContent.vue`, `CommandDialog.vue`, `PopoverContent.vue`, `DropdownMenuContent.vue`, `DropdownMenuSubContent.vue`, `tooltip/index.ts`, `TextPromptDialog.vue`. `native-select` picker `[&::picker(select)]:border-border-strong` to `border-border`. Border-box keeps outer size.
- `shadow-kira-dialog` to `shadow-kira` (3 sites); delete token, `@theme` entry, twMerge entry.
- `AdeActivityIcon.vue:27`: drop the arbitrary halo shadow.
- `AdeTaskCard.vue:71`: selected `border-focus ring-1 ring-focus` to `border-focus`.
- `refBadges.ts:167`: `ring-1 ring-inset ring-current` to `outline outline-1 -outline-offset-1 outline-current` (state mark, same pixels). Update its comment.
- `AdeDayBand.vue:132`: weekend hatch to `bg-bg` (one tone step above the chrome plan ground).
- Dashed to solid: every `border-dashed` and `outline-dashed` listed above becomes solid (delete the `-dashed` class, keep width and colour).
- Drop `cn-menu-translucent` from `DropdownMenuContent.vue`, `DropdownMenuSubContent.vue`. Keep `cn-menu-target`.

### Buttons

`packages/theme/src/components/ui/button/index.ts`. Base keeps `border border-transparent bg-clip-padding`; base radius becomes `rounded-kira-sm`; drop now-redundant per-variant `rounded-kira-sm`.

- Primary (solid): `toolbar-primary`, `dialog-primary`: `border-primary bg-primary text-primary-foreground hover:border-primary-hover hover:bg-primary-hover`.
- Secondary (outline, flat, bordered): `dialog`: `border-border bg-transparent text-fg hover:bg-field`. No fill at rest, no shadow.
- Danger outline: `dialog-danger`: `border-error bg-transparent text-error hover:bg-error/15`.
- Ghost (no border, hover tone): `toolbar`, `ghost`, `danger`, `title`: transparent border, `hover:bg-field`. `toolbar` pressed `aria-pressed:bg-field aria-pressed:text-fg` stays. `title` drops its duplicate `border border-transparent`.
- `link`: unchanged.
- `icon-sm` size: `rounded-[min(var(--kira-radius-sm),12px)] in-data-[slot=button-group]:rounded-kira` to nothing (base radius applies).
- Hand-rolled bordered buttons keep their border: `AdeDayBand.vue` history chip (`border border-border`, P508.md W6 said drop it: do not), `AdeAgentDialog.vue` danger outline, `DateTimePicker.vue` today cell. Dashed add-buttons (`AdeStepResults.vue`, `AdeWorkflowGraph.vue`) become solid secondary-looking: `border-dashed` goes.
- `toggle`/`toggle-group` `outline` segmented edge stays (rule 1).

### Radius

Audit: `rounded-kira-sm` 66 in 42 files, `rounded-kira` 38 in 28, `rounded-full` 24 in 16, `rounded-kira-xs` 25 in 21, `rounded-none` 14 in 13 (resets), `rounded-kira-lg` 3 plus `rounded-kira-lg!` 1, `rounded-kira-pill` 2, `rounded-kira!` 2, `rounded-xs` 1 (`connColor.ts:62`), arbitrary 5 (`button/index.ts:34`, `toggle/index.ts:17`, `input-group/index.ts:14,64,66`), sided `rounded-{l,r,t,b}-kira-sm` 4 (ToggleGroupItem), `rounded-bl-kira-sm` 1, `rounded-{l,r}-none` 3 (SplitButton). CSS: `slickTheme.css:641` literal `6px`, `base.css:68` scrollbar thumb `var(--kira-radius)`. Tokens: xs 2, sm 4, base 6, lg 8, pill 10 px.

One scale:

- `rounded-kira-sm` 4px, **control**: anything at control/row height or smaller, and fields. Buttons, inputs, textarea, select, input-group, toggles, tabs, chips, badges, tags, kbd, checkbox, menu/list/command rows, inline marks (`VarText`, `HighlightedText`, `OpLogCodeBlock`), editors used as fields (`PromptEditor`, `AdeNotesEditor`), skeleton, square swatches, colour squares.
- `rounded-kira` 6px, **container**: panes, cards (all variants), alerts, floating surfaces (popover, menu, sub-menu, tooltip, dialog, command dialog, native-select picker), list boxes, code/`pre` boxes, ADE cards and graph nodes, tiles, empty-state media.
- `rounded-full`: circles and pills only. Dots, avatars, switch, radio, progress, count badge, capsule marks.
- `rounded-none` and sided `-none`: resets for joined groups only.

Mapping:

- `rounded-kira-xs` to `rounded-kira-sm` (all, except `AdeTaskPanel.vue:134`, a P509 file).
- `rounded-kira-lg`, `rounded-kira-lg!` to `rounded-kira`, `rounded-kira!` (dialogs, command, TextPromptDialog).
- `rounded-kira-pill` to `rounded-full` (`badge/variants.ts:18`, `AdeEndNode.vue:11`).
- `rounded-xs` (`connColor.ts:62` bar) to `rounded-full`.
- Arbitrary: `button/index.ts:34` and `toggle/index.ts:17` to `rounded-kira-sm`; `input-group/index.ts:64,66` `rounded-[calc(var(--kira-radius)-3px)]` to `rounded-kira-sm`; `input-group/index.ts:14` `[&>kbd]:rounded-[calc(var(--kira-radius)-5px)]` to `[&>kbd]:rounded-kira-sm`.
- Controls on the container tier move to `rounded-kira-sm`: `input-group/index.ts:48`, `CommandInput.vue:32` `rounded-kira!`, `CommandItem.vue:69` `in-data-[slot=dialog-content]:rounded-kira!`, button base.
- Containers on the control tier move to `rounded-kira`: Card `inset` and `list` (`card/index.ts`), `tooltip/index.ts`, `native-select` `[&::picker(select)]:rounded-kira-sm`, `RestoreDialog.vue:105`, `AdePortImportDialog.vue:180,233,255`, `MonacoDiff.vue:83`. Implementer re-checks every remaining `rounded-kira-sm` against the control rule and every `rounded-kira` against the container rule; report moved sites in the Result.
- `slickTheme.css:641` sort-number badge `border-radius: 6px` to `@apply rounded-full`.
- `base.css:68` scrollbar thumb: keep `var(--kira-radius)`.

Tokens (`tokens.css`): keep `--kira-radius-sm: 4px`, `--kira-radius: 6px`; delete xs, lg, pill. `tailwind-core.css`: keep `--radius-kira-sm`, `--radius-kira`; delete lg, pill; set `--radius-kira-xs: var(--kira-radius-sm)` with comment "drop with the last use, `AdeTaskPanel.vue`, in P509". `lib/utils.ts` twMerge radius: `['kira-xs', 'kira-sm', 'kira']`, shadow `['kira']`. Rewrite both radius comments (`tokens.css`, `tailwind-core.css`, `shadcn-bridge.css:14`) tersely: control, container, full.

## Work items (in order; Conventional Commits; fast checks after each)

### W0 Settle the stopped run

- `git checkout -- packages/workbench/src/llmPrompts/PromptEditor.vue`.
- `ToolbarSeparator.vue`: `mx-1` back to `mx-0.5`.
- Commit the other 31 uncommitted files plus `ToolbarSeparator.vue` as one commit (the `ViewToolbar` prop removal and its callers must land together): `style(workbench): drop chrome dividers and ViewToolbar border prop`.
- `OpLogPanel.vue:398` arbitrary class is still open from P508.md W1; it lands in W3.

### W1 Tokens

- `packages/theme/src/tokens.css`: neutral scale and aliases above; delete listed tokens; add `--kira-accent-hover: #355dad`; `--kira-shadow: 0 4px 16px rgb(0 0 0 / 0.24)`; delete `--kira-shadow-dialog`; radius tokens. Update the surface-ladder comment to "chrome < bg < elevated < field", keep the WCAG note on `--kira-fg-subtle` with the new elevated figure 4.94:1, drop the `--kira-fg-disabled` sentence.
- `packages/theme/src/tailwind-core.css`: delete `--color-hover`, `--color-border-strong`, `--color-disabled`, `--color-badge`, `--shadow-kira-dialog`, `--radius-kira-lg`, `--radius-kira-pill`; `--radius-kira-xs` alias.
- `packages/theme/src/shadcn-bridge.css`: `--primary-hover: var(--kira-accent-hover)` in `:root`, `--color-primary-hover: var(--primary-hover)` in `@theme inline`.
- `packages/theme/src/lib/utils.ts`: twMerge lists.
- `apps/kira-space/frontend/src/ade/v2/tones.css`: tints 14% to 15%; `--color-tone-grey-tint: var(--kira-bg-input)`.
- `packages/workbench/src/editor/monacoTheme.ts`: `--kira-hover` keys to `--kira-bg-input` `#27272c`; `--kira-border-strong` keys to `--kira-border` `#323237`; `--kira-bg-elevated` fallbacks `#212125`; `--kira-border` fallbacks `#323237`; operator/punctuation fallbacks `#d4d4d8`; scrollbar fallbacks to `#323237`/`#8e8e97`/`#8e8e97`; rewrite the `#71717a59` alpha-trap comment example (keep the rule, cite `--kira-search-match`).
- `packages/workbench/src/terminal/terminalRenderer.ts`: reads scrollbar names only; no edit.
- `apps/kira-studio/frontend/src/charts/theme.ts:30,35`: `--kira-border-strong` to `--kira-border`.
- `apps/kira-studio/frontend/src/views/shared/slick/slickTheme.css`: 53, 347 to `--kira-border`; 250 to `--kira-bg-input`; 384 to 15%; 641 radius.
- `packages/theme/src/review-decorations.css`: 25 to `--kira-fg-subtle`; 35, 42 to 15%.
- Typecheck will not catch a deleted utility. Tailwind silently emits nothing for an unknown class, so W2 must remove every consumer in the same commit or the next one before running lint.
- Commit: `refactor(theme): one neutral scale, one shadow, one radius scale`.

### W2 Grey consumers (mechanical)

Scope `apps/kira-studio/frontend/src apps/kira-space/frontend/src packages/*/src`, `.vue/.ts/.css`, excluding the P509 files (none of them use these utilities today; recheck with grep).

- `bg-hover` to `bg-field` (keeps every variant prefix): `perl -pi -e 's/(?<![\w-])bg-hover(?![\w-])/bg-field/g'`.
- `border-strong` utilities: `perl -pi -e 's/(?<![\w-])((?:[a-z-]+:)*(?:border(?:-[trblxyse])?|ring|bg|outline|divide|fill|stroke|text))-border-strong(?![\w-])/$1-border/g'`. Apply the AdeDayBand/AdeTaskCard exceptions above by hand first.
- `text-disabled`, `bg-disabled`, `border-disabled`, `bg-badge`, `bg-bg/90` per the table.
- After: an element whose rest fill is `bg-field` and hover is `hover:bg-field` loses its hover cue. List them (`grep -rnP 'bg-field(?=[^"]*hover:bg-field)'`); leave ones that are pressed/filled controls, report the list in the Result.
- Commit: `style(ui): collapse greys onto the neutral scale`.

### W3 Buttons and textures

- `button/index.ts` per Buttons.
- Floating edge ring to border (7 files plus `native-select`).
- `shadow-kira-dialog` to `shadow-kira`: `DialogContent.vue`, `CommandDialog.vue`, `git-ui/App.vue:1906`.
- Alpha fills per mapping. Row washes. `AdeStepBlock.vue:86`.
- `AdeActivityIcon.vue` halo, `AdeTaskCard.vue` selected ring, `refBadges.ts` ring, `AdeDayBand.vue` hatch, dashed to solid, `cn-menu-translucent`.
- Commits: `style(ui): flat buttons with 1px secondary border`, `style(ui): one shadow, one tint strength, no ring stacks`.

### W4 Radius

- Mapping above, then the control/container recheck.
- Commit: `style(ui): one radius scale`.

### W5 Remaining separator work from P508.md

Run `P508.md` W4 (Studio), W5 (Space outside ADE), W6 (ADE), W7 (git-ui, docker-ui), W8 (markdown h1) as written, with these amendments:

- One border token now: "header strong to hairline" items reduce to "header keeps `bg-elevated`, hairline `border-b border-border` only where rule 4 says".
- `git-ui/App.vue:1906` drawer: `border-l border-border` stays (floating edge), `shadow-kira`.
- `AdeDayBand.vue:195` chip keeps its border (bordered button).
- `AdeTaskCard.vue`: selected `border-focus` only (W3); add `border` to `boxClass` per P508.md.
- `AdeStepBlock.vue` pending pill `border-border` (was strong) stays.
- `AdeStepNode.vue`, `AdeEndNode.vue`: dashed already solid from W3; graph nodes keep `rounded-kira`.
- `HostViewConversation.vue:58` already `rounded-kira`, border gone (landed).
- Never edit the P509 files.
- Commits: one per P508.md work item (W4, W5, W6, W7, W8).

### W6 Guards and comments

- `scripts/check-theme-classes.sh`: alias messages `rounded-sm` and `rounded-md` to `rounded-kira-sm`, `rounded-lg` to `rounded-kira`, `rounded-xl` to `rounded-full`; `-input` alias to `-border`; `-accent` alias to `-field`; `p-float`, `p-panel`, `p-row`, `p-status` recipe text to the new classes (`border border-border`, `shadow-kira`, `hover:bg-field`); shadow alias message to `shadow-kira` only. Add one `check_alias` for `rounded-kira-(?:lg|pill)|rounded-xs|rounded-\[` and one for `shadow-kira-dialog|(?<![\w-])(?:bg|border|ring|text|outline)-(?:hover|border-strong|disabled|badge)(?![\w-])` so retired names stay retired (lint guards, not tests).
- `grep -rn "border-strong\|kira-hover\|fg-disabled\|kira-badge\|shadow-kira-dialog\|radius-lg\|radius-pill\|radius-xs" scripts docs/ARCHITECTURE.md packages apps --include=*.sh --include=*.ts --include=*.md --include=*.css --include=*.vue` must show only the `--radius-kira-xs` alias, its twMerge entry and `AdeTaskPanel.vue:134`. Fix stale prose in `docs/ARCHITECTURE.md` if any.
- Commit: `chore(lint): retire old grey, shadow and radius names`.

### W7 Spec expectations (edit only)

- `apps/kira-space/tests/ui/ade-v2-plan.spec.ts`: 72-81 weekend test asserts the drop zone `backgroundImage` is `none` and `backgroundColor` equals a `var(--kira-bg)` probe; rename it "weekend bands sit one tone step above the plan ground". 384-405: drop the `--kira-border-strong` probe; unselected card `borderTopColor` is `rgba(0, 0, 0, 0)`. 542: `rgb(219, 175, 113)` (`--kira-status-amber`). 327-345 stay (6px, 1px, 3px).
- `apps/kira-studio/tests/ui/control-sizing.spec.ts:516-530` and `apps/kira-space/tests/ui/repo-workspace.spec.ts:1063`: drop the `- 1` border-b arithmetic and its comment. `repo-workspace.spec.ts:1087` `4px` stays.
- Not run this phase, edited so the chapter full run starts green: `apps/kira-space/tests/ui/repo-graph-toolbar.spec.ts:9-24` divider probe `bg-transparent`, title and comment "transparent spacer"; `apps/kira-studio/tests/ui/data-view.spec.ts:1089` comment.
- Re-grep `apps/*/tests` for `border-strong`, `kira-hover`, `fg-disabled`, `rgb(31, 31, 35)`, `rgb(43, 43, 48)`, `rgb(55, 55, 61)`, `rgb(36, 36, 39)`, `rgb(99, 99, 107)` and `8px`/`10px`/`2px` radius expectations. `api-ui-consistency.spec.ts:165-167` asserts *not* fg-disabled; keep it. List any other hit in the Result; edit only clear token-value expectations.
- Commit: `test(ui): expectations for the flat palette`.

### W8 Sweep

Re-run the audit (script below) and record before/after in the Result:

- Greys: count of distinct neutral utilities (`bg|text|border|ring` over chrome, bg, elevated, field, border, fg, muted-foreground, subtle). Target: no `hover`, `border-strong`, `disabled`, `badge`.
- Textures: shadow classes (target `shadow-kira` only, plus the allowed inset rails and resize hairline), ring classes (target `focus-ring`, `ring-0` resets), alpha strengths (target `/15` only, plus search-match 25%), gradients (target `swatch-none` only), dashed (target 0).
- Radius: classes (target `rounded-kira-sm`, `rounded-kira`, `rounded-full`, `rounded-none`/sided resets, the one `rounded-kira-xs` in `AdeTaskPanel.vue`).
- Borders: `border-border` remaining hits each map to a P508.md rule 1-7 or a bordered button.

Audit one-liners: `grep -rhoP '(?<![\w-])(?:[a-z-]+:)*(shadow|ring|rounded)(-[\w\[\]\(\),%.!/-]+)?' <scope> | sed 's/.*://' | sort | uniq -c` and `grep -rhoP '(?<![\w-])(?:bg|text|border|ring|fill)-[a-z-]+/\d+' <scope> | sort | uniq -c`.

## Verification

- After each work item: `bun run typecheck`, `bun run lint` (biome plus `lint:rules`: `check-tokens.sh`, `check-theme-classes.sh`, `check-ui-primitives.sh`, `check-ade-colours.sh`, `check-class-conflicts.ts`).
- Before W7: `bun run build:test:studio`, `bun run build:test:space`.
- After W7, once: the 3 edited specs only, `ade-v2-plan`, `control-sizing`, `repo-workspace`, through the existing playwright configs scoped to those files. Fix failures in the same pass.
- No `test:visual:*`; all PNG baselines differ by design and re-record in the chapter's single full run after P509.
- The hook must pass on every commit. No `--no-verify`.

## Risks

- **Deleted utilities fail silently.** Tailwind emits nothing for `bg-hover` once `--color-hover` is gone; lint does not catch it until W6 adds the alias guard. Mitigation: W1 and W2 land back to back, W2's greps must hit zero before its commit.
- **Hover equals field.** Elements filled `bg-field` lose a hover change (pressed toggles, Kbd, inset card rows). Accepted; W2 lists them. Secondary buttons moved to transparent fill, so they keep a hover cue.
- **Single border is lighter than the old field edge** (`#323237` vs `#37373d`; 1.13:1 on field). Inputs still read by fill step (field vs bg). Grid lines get slightly heavier (`#2b2b30` to `#323237`).
- **Hatch removal is a design change the user asked for** ("no gradients") but a spec pins it. W7 edits that test; flag it in the Result so the orchestrator can confirm with the user.
- **AdeDayBand greyed vs past** labels now share `text-subtle`.
- **`--radius-kira-xs` alias survives P508.** P509 must swap `AdeTaskPanel.vue:134` to `rounded-kira-sm` and delete the alias and its twMerge entry. The orchestrator adds that to P509's plan input.
- **Floating ring to border**: popover/menu/dialog content box shrinks 1px each side inside the same outer size. No spec measures their inner box.
- Specs outside the 3 run here that read changed computed styles get edited in W7 but are first run in the chapter's full suite.
- Monaco and xterm read tokens once at load; token edits need an app reload.
