# P509 Part 2 plan: one row grid for the task panel's Main tab

Base: `6aa3c7537` (docs: P508 plan). Implementation starts on top of P508 and P509 Part 1 as landed. One sequential Sonnet implementer.

Ask (user): Main tab rows look different, are unaligned and tire the eye. Give every row one shared layout: same columns, alignment, indentation grid, spacing, type scale, icon size, status placement. The tab must scan top to bottom.

## Scope boundary

- Layout and markup only. Keep every testid, `data-*` attribute, handler, emitted event, query and string. No new copy keys. No bridge or Go change.
- Tailwind utilities and theme tokens only. No scoped `<style>`, no hex, no new `--kira-*` token. Reuse `rowIndent` and `rowMeta` from `packages/theme/src/lib/rowVariants.ts` (the repo's row recipe; `AdeShelfRow.vue` already uses it).
- Board look stays. `plan/AdeBranchRow.vue` is shared with `plan/AdeTaskCard.vue`. It gains a `layout` prop, and board mode renders exactly as before.
- The Main tab is `panel/AdeMainTab.vue` (tabs Main, Details, Sessions in `AdeTaskPanel.vue`). The panel header (`AdeTaskPanel.vue` `#header`, `AdePanelFrame.vue`) is out of scope.
- Details-tab rows (`AdeLinkGroups.vue`, `AdeDetailsLink.vue`, `AdeTaskIssueLine.vue`, `AdeTaskTimeline.vue`) adopt the same row component in W5. This changes presentation only, so the panel reads as one system. Add and edit behaviour stays with Part 1.

## Discovery

Done via `codegraph_explore` (AdeMainTab, AdeMainBranches, AdeStepBlock, AdeBranchRow, AdeTaskIssueLine, AdeLinkGroups, AdeOriginSession, AdeTaskTimeline, AdeTagIcon, statusColors, KeyValueList). Then templates were read in full.

- `AdeLinkGroups.vue` and `AdeTaskTimeline.vue` render in the **Details** tab (`details/AdeDetailsTab.vue`), not Main. `AdeTaskIssueLine.vue` has one caller: `AdeLinkGroups.vue:222`.
- `AdeEstimateField`, `AdeStatusPicker`, `AdeStepMover`, `AdeWorkflowBlock`, `AdeStepBlock`, `AdeTaskBranchMenu` and `AdeMainBranches` each have one caller, all inside the Main tab. `AdeBranchRow` has callers in `AdeTaskCard.vue` (board) and `AdeMainBranches.vue`.
- Bug: `AdeTaskIssueLine.vue` renders `<dt/><dd>` inside an `<li>` (`AdeLinkGroups.vue:221`). That is invalid markup and ignores the `dl` grid it was written for.
- `rowIndent(depth)` = `6 + 14·depth` px left padding. `rowMeta` = `text-kira-sm text-muted-foreground tabular-nums`. Guard U19 bans hand-rolled `depth * N` indents in side panels. Guard U88 bans off-scale gaps (scale 0.5, 1, 1.5, 2, 3, 4). Guard U12 allows CodiconIcon sizes 12/13/16/24. Guard U10 bans raw `<label>`.
- `--kira-row-height` is 28px (compact density 22px), `h-control` 22px, `h-control-lg` 26px, `h-board-row` 40px.

### Current rows on the Main tab (container `px-3 py-3 gap-3`, so x below is relative to the 12px inset)

| # | Row (file) | Columns, left to right | Height | Left edge | Type | Icons / controls | Status placement |
|---|---|---|---|---|---|---|---|
| 1 | Status (`AdeMainTab` `KeyValueList`) | dt label, dd: picker, blocked chip, why | min 28 | 0, value at label max-content+12 | label kira-md muted; why kira-sm subtle | icon btn 22 (icon 13), PickerTrigger kira 22 | chip after control |
| 2 | Estimate (`AdeEstimateField`) | dt, dd: Input, unit tabs, clear, hint | 28 inner | same | hint kira-sm muted | Input kira-lg 26, SecondaryTabs kira-lg 26 | none |
| 3 | Branch request | dt, dd: text, Review | min 28 | same | kira-md | Button dialog kira-lg 26 | text before button |
| 4 | Not merging | empty dt, dd: Switch, Label, hint | min 28 | value column | Label kira-md medium; hint kira-sm subtle | Switch | none |
| 5 | Branches heading (`AdeMainBranches`) | label, spacer, ProjectPicker, Add branch | ~22, `pb-0.5` | 0 | kira-sm subtle | picker kira, Button dialog kira | none |
| 6 | Branch row (`AdeBranchRow`, board recipe) | `px-2`, spacer 16·d plus elbow, repo chip, base chip, owner, agent icon, name over meta line, attention, tag, action icons, more, review, edit menu | 40 | 8+16·d | name kira-md medium `font-data`; meta kira-sm subtle | tag icon 13, urgent disc 20, icon btn 22, review `size-5` 20, edit menu `size-5` 20 | tag right of body |
| 7 | PR line (`AdeMainBranches`) | inline `paddingLeft 16·d+8`, state icon, title link, base, checks | ~20, `pb-1` | 8+16·d | kira-sm | icons **12** | state glyph left, checks right |
| 8 | Workflow header (`AdeWorkflowBlock`) | label, sr-only Label, select, outdated badge, Edit link | ~26 | 0 | kira-sm muted | NativeSelect default kira-lg 26, Button link inline | badge after select |
| 9 | Step mover (`AdeStepMover`) | inline "Step" label, back, picker, next | 22 | 0 | kira-sm muted | icon btn 22, PickerTrigger kira 22 | none |
| 10 | Step head (`AdeStepBlock`, Card `px-2 py-2`) | state chip `min-w-10`, actions slot, `n.`, name, SmartBadge, mode | min 24 (`min-h-6`) | 8 | name **kira-lg** medium; `n.`/mode kira-sm subtle | chip | chip first |
| 11 | Step row (nested `bg-bg` box `px-1.5 py-1`) | glyph box, status, approve, route, gated, scope | min 22 | 14 | kira-sm toned/subtle | glyph `size-4` filled, Button kira | glyph left, status second |
| 12 | Run line | `pl-6`, glyph `w-3.5`, repo tag, branch, status `min-w-16`, sandbox, buttons, returns chip, note | min 22 | 38 | kira-sm | text glyph, Button kira 22 | glyph left, status mid-row, chip and note after buttons |
| 13 | Run details (setup, outcome, log) | `ml-6 my-0.5` | block | 38 | n/a | n/a | n/a |
| 14 | Alerts (field, workflow, step) | full width | block | 0 or 8 | n/a | n/a | n/a |

Result: 7 distinct heights (20, 22, 24, 26, 28, 40, header), 8 left edges, 3 label styles (kira-md muted dt, kira-sm subtle, kira-sm muted inline), 4 grey text tones, icons at 12/13/16, buttons at 20/22/26, and status at left, middle or right depending on the row.

### Details-tab rows (W5)

- "Links" title (kira-sm muted) and group titles (kira-sm subtle).
- Link `<li>`: min 28, `gap-2`. Kind chip `wide` (`min-w-21`), title link, status chip, spacer, origin session, edit and remove (22).
- Issue line: `<li pl-1>` holding `<dt/><dd>`. Eye icon **16**. PR count and gap lines in kira-sm.
- Timeline: toggle with chevron **12**, kira-sm muted. Entries are min 28 with `gap-2`: time `w-20` kira-sm subtle, then label or link, then origin session.

## The row grid

Every row is one line of four cells: **lead | label (optional) | body | actions**. Cells sit in a flex row and align to the first line (`items-start`, each fixed cell `h-row`).

```
x: rowIndent(depth) | lead w-5 | gap-1 | label w-24 (labelled rows) | gap-1 | body flex-1 | gap-1 | actions | pr-1.5
```

- **Indent**: `rowIndent(depth)` (6px plus 14px per level). Children sit one level below their parent: PR lines under their branch, run lines under the step. Branch stacks use `row.depth`.
- **Lead (`w-5`, 20px)**: the row's state glyph only, centred. Codicon 13. Text glyphs (✓ ● ! ✕) sit in a `size-4` box. The urgent disc stays `size-5`; it fills the column exactly. The lead cell is always rendered, even when empty, so the label and body rails never move.
- **Label (`w-24`, 96px)**: field name, in `text-kira-sm text-muted-foreground`. It is present on field rows (Status, Estimate, Branch request, Not merging, Branches, Workflow, Step) and on Details link rows (kind chip). Unlabelled rows start their body where labels start, which gives two rails at depth 0: label and item text at x=30, values at x=130, measured from the tab's content box.
- **Body (`flex-1 min-w-0`, `gap-1.5`)**: the primary text first (`text-kira-md text-fg`, truncating). Then the status word or chip, coloured by tone. Then secondary facts in `rowMeta`.
- **Actions (`shrink-0`, `gap-1`)**: buttons only, right-aligned at the row end. Text buttons use `variant="dialog" size="kira"` (22px). Icon buttons use the `TooltipIconButton` default (`kira-icon`, 22px, icon 13). No `size-5` overrides.
- **Height**: `min-h-row` (28px; 22px in compact density). One row is one line. Anything longer (alerts, logs, outcome reports, setup progress) goes below the row in `AdeRowDetail`. That block aligns to the body rail of its row.
- **Type scale**: two sizes only. `text-kira-md` is for primary text and controls. `text-kira-sm` is for labels, status words and meta. `font-medium` marks only a group head's primary text (step name, branch name). `text-kira-lg` and `text-subtle` leave the tab; meta is `rowMeta` (`text-muted-foreground`).
- **Controls in a body**: size `kira` (`h-control`, 22px) so they fit a compact 22px row. That covers Input, SecondaryTabs, PickerTrigger and NativeSelect.
- **Spacing between rows**: none. Row height carries the rhythm. Between groups (fields, branches, workflow) use `gap-3`. Step cards stack with `gap-1`.

### Shared constants: `apps/kira-space/frontend/src/ade/v2/panel/adeRow.ts` (new)

```ts
import { rowIndent, rowMeta } from '@theme/lib/rowVariants';

export { rowIndent, rowMeta };
// One task-panel row: lead | label? | body | actions, each cell aligned to the first line.
export const ADE_ROW = 'flex min-h-row min-w-0 items-start gap-1 pr-1.5 text-kira-md text-fg';
export const ADE_ROW_LEAD = 'relative flex h-row w-5 shrink-0 items-center justify-center';
export const ADE_ROW_LABEL = 'flex h-row w-24 shrink-0 items-center text-kira-sm font-normal text-muted-foreground';
export const ADE_ROW_BODY = 'flex min-h-row min-w-0 flex-1 items-center gap-1.5';
export const ADE_ROW_ACTIONS = 'flex h-row shrink-0 items-center gap-1';
/** A text state glyph (✓ ● ! ✕) in the lead cell; callers add the tone or fill. */
export const ADE_ROW_GLYPH = 'box-border flex size-4 items-center justify-center rounded-kira-sm text-kira-sm font-medium';
/** A status word right after the primary text; callers add the tone text class. */
export const ADE_ROW_STATUS = 'shrink-0 whitespace-nowrap text-kira-sm';
```

### `panel/AdeRow.vue` (new)

- Props: `depth?: number` (0), `label?: string`, `labelFor?: string`, `as?: 'div' | 'li'` (`'div'`), `term?: boolean`.
- Slots: `lead`, `label`, default (body), `actions`.
- Root: `<component :is="as" data-slot="ade-row" :data-depth="depth" :class="ADE_ROW" :style="rowIndent(depth)">`. Attributes fall through, so callers' `data-testid`, `data-*` and `@contextmenu`/`@dblclick` land on the root.
- Lead: `<span data-slot="ade-row-lead" :class="ADE_ROW_LEAD"><slot name="lead" /></span>`, always rendered.
- Label: rendered when `label !== undefined` or the `label` slot is used. It is `data-slot="ade-row-label"`. With `labelFor` it renders the shadcn `Label` (`for=labelFor`, `cn(ADE_ROW_LABEL)` overrides its kira-md/medium). Otherwise it renders a span (`dt` when `term`). Text sits in an inner `<span class="truncate">`. `label=""` reserves the column with no text.
- Body: `<div>` (`dd` when `term`) with `data-slot="ade-row-body"` and `:class="ADE_ROW_BODY"`.
- Actions: `<div data-slot="ade-row-actions" :class="ADE_ROW_ACTIONS">`, rendered when the slot is used.
- With `term`, the root renders as `div`, so a `<dl>` holds `div > dt + dd` groups, which is valid HTML.

### `panel/AdeRowDetail.vue` (new)

- Props: `depth?: number` (0), `labelled?: boolean`.
- Template: `<div data-slot="ade-row-detail" class="flex min-w-0 gap-1 pr-1.5 pb-1" :style="rowIndent(depth)"><span class="w-5 shrink-0" aria-hidden="true" /><span v-if="labelled" class="w-24 shrink-0" aria-hidden="true" /><div class="min-w-0 flex-1"><slot /></div></div>`. It shares geometry with `AdeRow` by construction.

## Mapping every row onto the grid

Main tab root (`AdeMainTab.vue`): `flex min-h-0 flex-1 flex-col gap-3 overflow-auto px-1.5 py-3`. The 6px gutter plus `rowIndent(0)` keeps text at today's 12px inset.

| Row | lead | label | body (in order) | actions | depth |
|---|---|---|---|---|---|
| Status | – | Status | `AdeStatusPicker`, blocked `AdeChip` (`ade-task-blocked`), why (`rowMeta truncate`, `ade-task-status-why`) | – | 0 |
| Estimate | – | Estimate (`labelFor="ade-est-num"`) | Input, unit tabs, clear, span hint (`rowMeta`) | – | 0 |
| Branch request (`ade-task-branch-request-row`) | – | Branch request | status text (kira-md) | Review (`ade-task-branch-request-open`, dialog kira) | 0 |
| Not merging | – | label slot: `Label for="ade-task-parked"` | Switch (`ade-task-parked`), hint (`rowMeta`) | – | 0 |
| Field error | `AdeRowDetail` holding the `ade-task-error` Alert | | | | 0 |
| Branches heading | – | Branches | – | ProjectPicker (`ade-add-repo`, kira), Add branch (`ade-add-branch`, dialog kira) | 0 |
| Branch row (`AdeBranchRow layout="panel"`) | `AdeTagIcon`, plus the elbow when `depth > 0` | – | repo chip, base chip, owner, agent icon, name (`font-data font-medium`), prog, context, merged and deployed (`rowMeta`), attention | shown actions, more, review, edit menu | `row.depth` |
| PR line (`ade-task-branch-pr`) | PR state icon 13 (`ade-task-branch-pr-state`) | – | title link (`text-info truncate`), base (`font-data`, `rowMeta`), checks icon 13 | – | `row.depth + 1` |
| Workflow | – | Workflow (`labelFor="ade-workflow-select"`) | NativeSelect `size="kira"`, outdated Badge | Edit workflows (`ade-edit-workflows`, dialog kira) | 0 |
| Workflow error | `AdeRowDetail` | | | | 0 |
| Step (`ade-step-mover`) | – | Step | back, picker, next | – | 0 |
| Step head (`ade-step-block-head`, right-click menu) | step glyph box (`ADE_ROW_GLYPH` plus `glyphClass`, moved from the step row) | – | `n.` (`rowMeta`), name (`ade-step-name`, `font-medium`, `text-muted-foreground` when done), state `AdeChip`, SmartBadge, mode (`ade-step-block-mode`, `rowMeta truncate`) | `#actions` slot (task action button) | 0 |
| Step status (`ade-step` wrapper keeps `data-step-id`) | – | – | status (`ade-step-status`, `ADE_ROW_STATUS` plus `statusClass`), route (`ade-step-route`, tone blue), gated (tone amber), scope (`ade-step-scope`, `rowMeta`, `font-data` for scripts) | Approve (`ade-step-approve`) | 0 |
| Run line (`ade-run-line`) | run glyph (`ade-run-glyph`, `ADE_ROW_GLYPH` plus the tone text class) | – | repo tag, branch (`ade-run-branch`, `font-data rowMeta`), status (`ade-run-status`, `ADE_ROW_STATUS` plus tone; drop `min-w-16`), sandbox badge, returns chip (`ade-run-returns`), note (`ade-run-note`, `rowMeta truncate`) | log toggle, resume, review, take over, retry | 1 |
| Run details (setup, outcome, log) | `AdeRowDetail` | | | | 1 |
| Step error | `AdeRowDetail` | | | | 0 |

The step card stays `Card variant="section"` with `class="gap-0 py-1"` (no `px`; rows pad themselves). It keeps the `skipped` text tone and the current-step `boxClass` as P508 left them. The nested `bg-bg` box around the step row and run lines goes. `ade-step` becomes a plain `flex flex-col` wrapper.

### Details tab (W5, presentation only)

| Row | lead | label | body | actions | depth |
|---|---|---|---|---|---|
| Links title, group titles | – | title text | – | – | 0 |
| Empty group (`ade-details-empty`) | – | `""` | text (`rowMeta`) | – | 0 |
| Link (`ade-details-link`, `as="li"`, read mode) | – | label slot: kind `AdeChip wide` | title (`ade-details-link-title`), status chip | `#trailing` (origin session), edit, remove | 0 |
| Link, edit mode | – | label slot: as Part 1 decides | Part 1's edit control | – | 0 |
| Issue title, loading, PR count, gap (`AdeTaskIssueLine`) | – | `""` | title plus status chip; Skeleton; count (`rowMeta`); gap (`text-kira-sm text-tone-amber`) | view (`ade-task-issue-view`, icon 13, default size) | 0 |
| Timeline toggle (`ade-details-timeline-toggle`) | chevron 13 | – | header text (`text-kira-sm text-muted-foreground`) | – | 0 |
| Timeline entry (`ade-details-timeline-entry`, `as="li"`) | – | relative time | label or link | origin session | 0 |

The timeline toggle is a `CollapsibleTrigger` button, so it takes the constants directly (`ADE_ROW`, `ADE_ROW_LEAD`, `ADE_ROW_BODY`, `:style="rowIndent(0)"`), not `AdeRow`. `AdeTaskIssueLine` root becomes `<div class="flex flex-col" data-testid="ade-task-issue">`, which drops the invalid `dt`/`dd`. The `<li>` in `AdeLinkGroups` loses `pl-1` and becomes `flex flex-col`.

## Ownership (P509 Part 1 vs Part 2, and P508)

Order: P508, then P509 Part 1, then P509 Part 2, per the table. No two of them run at once. Part 2 plans against `6aa3c7537` but implements on Part 1's last commit. Its implementer re-reads every shared file from disk first (the W5 re-read is discovery: use `codegraph_explore` on them).

| File or region | Part 1 (inline add/edit) | Part 2 (row layout) |
|---|---|---|
| `panel/adeRow.ts`, `panel/AdeRow.vue`, `panel/AdeRowDetail.vue` (new) | consume only, after Part 2 lands | owns |
| `panel/AdeMainTab.vue`, `AdeMainBranches.vue`, `AdeWorkflowBlock.vue`, `AdeStepBlock.vue`, `AdeStepMover.vue`, `AdeStatusPicker.vue`, `AdeEstimateField.vue`, `AdeTaskBranchMenu.vue` | never | owns (template and class strings; script only for imports) |
| `plan/AdeBranchRow.vue` | never | owns the `layout` prop and panel-mode template (P508 class edits there land first; keep them) |
| `panel/AdeTaskIssueLine.vue` | never | owns |
| `details/AdeTaskTimeline.vue` | never | owns (template only) |
| `details/AdeDetailsLink.vue` | owns `<script>`, events, edit mode, and every control inside the row | only the read-mode root `<li>` becomes `<AdeRow as="li">`, with existing children moved into lead/label/body/actions slots unchanged |
| `details/AdeLinkGroups.vue` | owns `<script>`, paste/add UI, group data, alerts, editing state | only the title divs, the empty-state div and the `<li>` wrapping `AdeTaskIssueLine` |
| `details/AdeDetailsLinkEdit.vue`, `panel/useLinkFields.ts`, `jira.ts`, `details/linkGroups.ts`, `queries.ts`, bridge, Go, flow tests | owns | never |
| `tests/ui/ade-v2-task-panel.spec.ts` | owns its Details tests | appends one new `test(...)` at the file end; never edits existing tests |
| Any new row Part 1 adds (inline add line, etc.) | builds with plain classes | W5 moves it onto `AdeRow` if it is a row (one line, or label plus value) |

If Part 1 renames or replaces `AdeDetailsLink.vue`, apply the same wrapper rule to whichever component renders a read-mode link row. If Part 1 left a file Part 2 owns changed, stop and report; do not merge by guess.

P508 (styling, lands first) touches `plan/AdeBranchRow.vue`, `panel/AdeStepBlock.vue` and `panel/AdePanelFrame.vue` class strings only. Part 2 keeps P508's colour and border choices (elbow colour, pending-glyph border, step card tint) and changes only geometry and markup.

## Work items (one sequential Sonnet implementer; one commit each, Conventional Commits; end each with the session trailers)

Run `bun run typecheck:space-web` and `bun run lint:rules` after each item. Commit each item as it lands.

### W1 `feat(ade): shared task-panel row`

- Add `panel/adeRow.ts`, `panel/AdeRow.vue` and `panel/AdeRowDetail.vue` exactly as specified above. `<script setup lang="ts">`, one terse header comment each.

### W2 `refactor(ade): Main tab fields on the row grid`

- `AdeMainTab.vue`: root classes per the mapping. Replace `KeyValueList` with `<dl class="m-0 flex flex-col">` and four `<AdeRow term …>` rows (table above). Move the field error Alert into an `AdeRowDetail` after the `dl`. Drop the `KeyValueList` import.
- `AdeStatusPicker.vue`: no change. Its Alert wraps, because `ADE_ROW_BODY` sets no `whitespace`.
- `AdeEstimateField.vue`: Input `size="kira"`, SecondaryTabs `size="kira"`. Delete the sr-only `Label` (the row's `labelFor` labels `ade-est-num`). Hint span uses `rowMeta`. Inner wrapper becomes `flex items-center gap-1.5` (no `min-h-row`; the row owns height). `FieldError` stays below.

### W3 `refactor(ade): Main tab branches on the row grid`

- `plan/AdeBranchRow.vue`: add `layout?: 'board' | 'panel'` (default `'board'`). In board mode the output is byte-for-byte today's DOM except actions wrapped in `<span class="contents">`. Panel mode:
  - Root: `ADE_ROW` plus `selected ? 'bg-select' : ''` (no `h-board-row`, no `px-2`, no `gap-1.5`), `:style="rowIndent(row.depth)"`, `data-slot="ade-row"`. Keep `ref`, role, tabindex, every `data-*` and every handler.
  - The depth spacer is removed. The lead `<span data-slot="ade-row-lead" :class="ADE_ROW_LEAD">` holds the elbow (when `depth`), drawn `absolute -left-1 -top-1 box-border h-4.5 w-1.5 rounded-bl-kira-sm border-b-2 border-l-2` with P508's border colour. The lead also holds `AdeTagIcon`. The elbow then drops from the parent's lead centre (parent lead centre minus child lead start = 10 − 14 = −4px).
  - `AdeBranchTip` content: one line, `ADE_ROW_BODY`, `data-slot="ade-row-body"`. The name/meta `flex-col` becomes inline: name span unchanged (drop `leading-4`), meta span `flex min-w-0 items-center gap-1.5 overflow-hidden whitespace-nowrap` (drop `leading-3.5`). `AdeAttention` moves to the body end.
  - Actions wrapper: `ADE_ROW_ACTIONS`, `data-slot="ade-row-actions"`. The review button drops `size-5` in panel mode only (`:class="layout === 'panel' ? 'shrink-0' : 'size-5 shrink-0'"`).
- `AdeMainBranches.vue`: root `flex flex-col`. The heading becomes `<AdeRow :label="t('common.module.branches.label')">` with the adders in `#actions`. Branch items pass `layout="panel"`. The PR line becomes `<AdeRow :depth="row.depth + 1" data-testid="ade-task-branch-pr" :data-branch-id="row.id">` per the mapping; delete the inline `paddingLeft` style; icons go 12 to 13. The `ade-task-branch-item` wrapper stays `flex flex-col`.
- `AdeTaskBranchMenu.vue`: trigger drops `size-5` (default `kira-icon`).

### W4 `refactor(ade): Main tab workflow on the row grid`

- `AdeWorkflowBlock.vue`: root `flex flex-col gap-1`. The header becomes `<AdeRow :label="t('space.common.workflow.label')" label-for="ade-workflow-select">`. Delete the visible span and the sr-only `Label`. NativeSelect `size="kira"`. Edit workflows becomes `variant="dialog" size="kira"` in `#actions`. The error Alert goes in `AdeRowDetail`.
- `AdeStepMover.vue`: root becomes `<AdeRow v-if="card.blocks.length" :label="t('space.common.step.label')" data-testid="ade-step-mover">` with the three controls in the body. Delete the inline label span and the extra wrappers.
- `AdeStepBlock.vue`: Card `class="gap-0 py-1"`. Head, step status and run lines become `AdeRow` per the mapping. Setup, outcome and log become `<AdeRowDetail :depth="1">` (drop `my-0.5 ml-6`). The error Alert goes in `<AdeRowDetail>`. Replace `GLYPH_BOX`'s `size-4`/box classes with `ADE_ROW_GLYPH` (keep the fill/tone maps). Run glyph: `ADE_ROW_GLYPH` plus `TONE_TEXT_CLASS[rl.tone]` (drop `w-3.5 text-center`). Head name loses `text-kira-lg`. Done name becomes `text-muted-foreground`. `n.`, mode and scope switch from `text-kira-sm text-subtle` to `rowMeta`. The state chip drops `min-w-10 text-center`.

### W5 `refactor(ade): Details rows on the row grid`

First re-read the landed `AdeLinkGroups.vue`, `AdeDetailsLink.vue` and any new Part 1 row component with `codegraph_explore`. Then apply the Details mapping within the ownership table only.

- `AdeDetailsLink.vue`: read-mode root becomes `AdeRow as="li"`. Drop the `flex-1` spacer (the actions cell is right-aligned by `flex-1` on the body).
- `AdeLinkGroups.vue`: titles and the empty state become `AdeRow`. The issue `<li>` becomes `flex flex-col` without `pl-1`.
- `AdeTaskIssueLine.vue`: rows per the mapping. Eye icon goes 16 to 13 (drop `:icon-size`).
- `AdeTaskTimeline.vue`: toggle and entries per the mapping. Chevron goes 12 to 13. `ol` and `li` stay.

### W6 `test(ade): Main tab rows share one grid`

Layout only, no IPC, so no backend half (as `ade-v2-main-branches.spec.ts` states for its own frontend-only change).

Append to `apps/kira-space/tests/ui/ade-v2-task-panel.spec.ts`: `test('Main tab rows share one grid', …)` on `T_bill` (branches, PR lines, stacked branch, run lines). `evaluateAll` over `[data-testid="ade-main-tab"] [data-slot="ade-row"]` and read per row: `data-depth`, lead `x`, label `x`/`width` if any, body `x`, row `height`, and `getComputedStyle(row).minHeight`. Assert:

1. Every row at one depth shares one lead `x`. Depth d+1 lead `x` minus depth d lead `x` = 14.
2. Every label has the same `x` and `width`. Every labelled body shares one `x`.
3. A depth-0 unlabelled body `x` equals the label `x`.
4. Every row's `height` equals its `minHeight` (one line each in this fixture).
5. At least 12 rows matched, so the test cannot pass on an empty selector.

Then re-grep the Main-tab and Details testids touched above across `apps/kira-space/tests/ui` and fix any expectation the markup change broke. Expected: none. Testids, text and order of interactive elements are kept. The flat-steps right-click at `{ x: 200, y: 8 }` still lands inside the 28px head.

## Verification (fast checks plus one UI spec run; the chapter's full run comes after P509)

- `bun run typecheck`
- `bun run lint` (biome plus `lint:rules`: U10, U12, U18, U19, U88, class conflicts, copy catalog)
- `bun run build:test:space`
- One playwright invocation, scoped: `bunx playwright test --config=apps/kira-space/playwright.config.ts --project=ui ade-v2-task-panel ade-v2-main-branches ade-v2-flat-steps ade-v2-run ade-v2-task-flow ade-v2-panel ade-v2-task-status ade-v2-branch-request ade-v2-return-limit ade-v2-norepo ade-v2-sandbox ade-v2-resume ade-v2-review-step ade-v2-task-branches ade-v2-plan ade-v2-card-icons ade-v2-issue-tracker ade-v2-artifacts ade-v2-add-link consistency`. Fix failures in the same pass, then rerun only the failing specs.
- Board check: `ade-v2-plan` and `ade-v2-card-icons` pass unchanged, which proves board-mode `AdeBranchRow` held.
- Do not run or re-record `test:visual:*`. `visual/ade-sandbox` (run line) is expected to differ; it re-records in the chapter's full run.
- The orchestrator verifies with greps: `KeyValueList` gone from `AdeMainTab.vue`; `text-kira-lg|text-subtle|size-5|:size="12"|:icon-size` gone from the W2–W5 files (except the board-mode `size-5`); `AdeRow` used in every Main-tab file listed; `paddingLeft` gone from `AdeMainBranches.vue`.

## Risks

- **Shared `AdeBranchRow`**: panel mode diverges in markup. Board mode must stay identical; the `contents` wrapper is the only board DOM change. Covered by `ade-v2-plan`/`ade-v2-card-icons`.
- **Single-line branch rows**: context, prog and chips now share the name's line and truncate earlier in a narrow panel. `AdeBranchTip` still shows everything on hover. This is the deliberate trade for one row height.
- **Compact density**: `h-row` drops to 22px. The elbow (`h-4.5`) overshoots the line centre by 3px. Controls are 22px and still fit.
- **Label width `w-24`**: fits the longest English label ("Branch request", about 80px at 11px). Longer text truncates.
- **Taller workflow**: step and run rows go from 22 to 28px at comfortable density. The card's nested box goes, so net height grows about 6px per run line. This is accepted for one rhythm.
- **Order with Part 1**: Part 1 lands first and may restructure the Details files. The W5 re-read and the "stop and report" rule cover it.
- **P508 class edits** in `AdeBranchRow.vue`/`AdeStepBlock.vue` land first. Rewrite those files' templates from the landed version, never from this plan's base.
- **Visual baselines** shift (Main tab in `space.shot.ts` and `visual/ade-sandbox`). They re-record once in the chapter's full run.
