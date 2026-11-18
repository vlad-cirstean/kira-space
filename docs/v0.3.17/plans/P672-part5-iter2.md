# P672 Part 5 iter2: menus, popovers, tooltips, selects, second pass

Iteration 5 of `P672 Part 10` (master loop: `plans/P672-part10.md`). Re-opens `P672 Part 5` against
current tree (base `b33dd339a`: Parts 1-9, P660 Part 2, Part 2 iter2, Part 3 iter2 landed). Fresh
scan; first-pass summaries not trusted. Discovery through `codegraph_explore` plus scratchpad tag
parser. One sequential Sonnet implementer (see Stream decision). Accessibility-only items left out
by request.

## Rules applied

ARCHITECTURE "UI uniformity rules (P672)", menus, popovers and plain-hint bullets, plus:

- RM.1 data menu is a `MenuItem[]` rendered by `MenuItems.vue`. Hand entry only for content a
  MenuItem kind cannot carry, with a comment naming that content.
- RM.2 option picker showing current value is a radio group; on/off toggle is a checkbox item. Both
  draw one indicator, owned by the primitive.
- RM.3 disabled reason is `hint`, never visible inline text.
- RP.1 popover has two shapes: default (form) or flush (list). Flush pieces: SectionHeading or
  PopoverHeader, top bar, `p-1` row list, PopoverFooter. Insets come from the pieces, never a host
  class.
- RP.2 popover action row is PopoverFooter with `toolbar` `kira` Buttons, leading icon.
- RT.1 plain hint is TextTooltip; icon button is TooltipIconButton (shortcut through `kbd`); rich
  body only for markup TextTooltip cannot carry.
- Library: shadcn-vue DropdownMenu radio/checkbox primitives (reka-ui), Popover, Command; Tailwind
  utilities. No Pinia or TanStack Query change: no state or fetch moves.

### Exemptions re-checked

- `<!-- radio: MenuItem has no radio kind -->` (7 hosts) and `<!-- checkbox: MenuItem has no
  checkbox kind -->` (2 hosts): stale once F1 adds the kinds. Converted (F2), except three hosts
  whose option body MenuItem still cannot carry (F2 table); their comment names the real content.
- `ProofScreenshotMenu.vue:121` rich label block: still holds, kept.
- Rich `:disabled`-condition tooltips (BranchRequestBody, AdeReposPage, ApprovalItemCard,
  ReviewResultOption, AutomationsStart, PendingResponse): Part 2 iter2 rule, kept.
- RefreshButton `v-if`/`v-else` tooltip body: two bodies, stays rich.
- ColumnsMenu and ProjectionMenu hint footers (`Separator my-1` plus `px-1.5 pb-1.5 text-kira-sm
  text-subtle`): twin hint text, not actions. Kept.
- `Separator class="my-1"` between list sections: same in all 6 hosts. Not a finding.
- PopoverContent widths: every host on w-56/w-80/w-96/w-120/w-auto. Not a finding.

## Current state (scans at `b33dd339a`)

709 non-`components/ui/` `.vue` files in the guard DIRS.

- `bash scripts/check-ui-primitives.sh`: green. PERMANENT 48 lines. Highest guard U133.
- `MenuItem` union (`packages/workbench/src/state/contextMenu.ts`): kinds `item`, `submenu`,
  `separator`, `label`. No radio or checkbox. `item.checked` draws a trailing hand check
  (`MenuItems.vue:119-121`). `item.detail`: 0 producers.
- `checked:` producers: 13 (board `menu.ts:140,148,184,249,345`, `useTaskMenu.ts:176`,
  `grid/menu.ts:696,705`, `project/menu.ts:402`, `collectionMenu.ts:26,36`, `RepoList.vue:222`,
  `CellEditorView.vue:334,346`).
- Hand `DropdownMenuRadioItem` hosts: 7. Hand `DropdownMenuCheckboxItem` hosts: 2. Each of 7 radio
  hosts overrides `#indicator-icon` with `CodiconIcon check 13`; primitive default is lucide
  `CheckIcon size-3.25`.
- Raw `<Tooltip>`: 27. `<Tooltip :disabled="true">` with no content: 4. TextTooltip wrapping an
  icon-only Button: 2. Hand `<Kbd>` in TooltipIconButton slot: 3. Text-plus-Kbd plain triple: 2.
- shadcn `<Select>`/Combobox: 0. Command: 5 files. NativeSelect: 47 files. PickerTrigger: 8.
  ContextMenu primitive: 1.
- PopoverContent flush hosts: 29. Top bar inset shapes: 5. Row list body recipes: 4. Hand rows off
  `rowVariants`: 2. Footer shapes: 3.

## Findings

### F1. MenuItem lacks radio and checkbox kinds; three check looks

Same "current value" mark renders three ways: `item.checked` trailing hand check in MenuItems, hand
`DropdownMenuRadioItem` with codicon override, primitive default lucide check (none use it, all
override). Producers set `checked` on plain items, so radio and toggle semantics are lost and
skipped/disabled options carry no hint.

Change:

- `contextMenu.ts`: add
  - `MenuRadio = { type: 'radio'; id: string; value: string | null; items: readonly (MenuRadioOption
    | MenuSeparator)[] }`;
  - `MenuRadioOption = { type: 'option'; id: string; value: string; testId?: string; label: string;
    parts?; detail?: string; swatch?; disabled?: boolean; hint?: string; run(): void }`;
  - `MenuCheckbox = { type: 'checkbox'; id: string; testId?: string; label: string; icon?: string;
    checked: boolean; disabled?: boolean; hint?: string; keepOpen?: boolean; run(): void }`.
  - Remove `checked` from `item`. Typecheck then holds every producer (no guard needed).
- `MenuItems.vue`: render `radio` as `DropdownMenuRadioGroup :model-value="value"` with one
  `DropdownMenuRadioItem` per option (reserved `size-4` icon column holding swatch when set,
  `data-value`, `data-testid` from `testId ?? menu-item-${id}`, detail line on `DETAIL_ROW`, hint via
  existing hint tooltip). `checkbox` renders `DropdownMenuCheckboxItem` with icon column,
  `@select` calling `preventDefault` when `keepOpen`. Drop trailing check span (119-121). If
  MenuItems also renders under the ContextMenu family, mirror with `ContextMenuRadio*` /
  `ContextMenuCheckboxItem`; implementer checks host family first.
- Swatch span gets `data-testid="menu-swatch"` (spec hook for F2 EnvironmentSelect).

### F2. Indicator and hand hosts

- `DropdownMenuRadioItem.vue`, `DropdownMenuCheckboxItem.vue`: default `#indicator-icon` becomes
  `<CodiconIcon name="check" :size="13" />` (lucide import dropped). Every host override removed.
  Mirror in ContextMenu radio/checkbox primitives if they exist.
- Producers move to the new kinds:

| Site | Now | Change |
| --- | --- | --- |
| `ade/v2/board/menu.ts:140,148` step submenu | items + `checked`, skipped steps disabled, no hint | `radio` (value current step id or `DONE`); skipped option `disabled` + `hint: t('common.skipped.status')` |
| `ade/v2/board/menu.ts:184` status | items + `checked` | `radio` |
| `ade/v2/board/menu.ts:249` plan entry | `checked: here, disabled: here` | `radio` over plan days; current one not disabled (radio no-op on same value) |
| `ade/v2/board/menu.ts:345` workflow | items + `checked` | `radio` |
| `ade/v2/board/menu.ts:51` `TaskMenuItem.checked?` | field | replace with radio/checkbox kinds in TaskMenuItem, mapped 1:1 in `useTaskMenu.ts:176` |
| `repo/RepoList.vue:211-223` Color submenu | swatch + `checked` | `radio` options with `swatch` |
| `CellEditorView.vue:327-350` format menu | auto + groups + separators + `checked` | one `radio` whose `items` keep the separators; hints unchanged |
| `grid/menu.ts:690-708` mask kinds | items + separator + `checked` | `radio` (value `null` for mask-none option id `none`) |
| `project/menu.ts:392-405` set-as-default | toggle item, star icon, `checked` | `checkbox` with `icon: 'star'`; testId kept `menu-item-set-as-default` |
| `lib/collectionMenu.ts:9-50` move to collection | items + `checked` | `radio` (none option value `null`) |
| `AdeStepMenu.vue:63-83` | MenuItems + separator + hand radio, inline skipped span | one `MenuItem[]`: back/next items, separator, `radio`; skipped option `disabled` + hint. Testids kept `ade-row-step-option-${id}` |
| `AdeStepMover.vue:46-64` | same shape | same; testids kept `ade-step-option-${id}` (`data-disabled` still set by reka) |
| `EnvironmentSelect.vue:77-` | hand radio with `conn-dot`, then MenuItems manage | one `MenuItem[]`: `radio` options with `swatch` (none option without), separator, manage items. Testids kept `api-environment-option`, `api-environment-option-none`; `data-value` kept |
| `EndpointChip.vue:67-94` | hand two-line radio (TruncatedText + rowMeta host) | `radio` options with `detail` = host. Testids kept `docker-context-automatic`, `docker-context-option`, `data-value` kept |
| `OpLogToolbar.vue:95-106` | MenuItems "all" (icon column) + hand checkboxes (no column: misaligned) | one `MenuItem[]`: "all" item, `checkbox` per facet, `keepOpen: true`; testids `ops-facet-${value}` |
| `MemoryFilters.vue:64-89` | labels + hand checkbox groups + separator | `label`, `checkbox` x n (`keepOpen`), separator, `label`, `checkbox` x n. Testids kept `memory-filter-scope-*`, `memory-filter-origin-*` |

- Stay hand (MenuItem cannot carry the body), comment rewritten:
  - `AdeTaskStatusMenu.vue:62-76`, `AdeStatusPicker.vue:61-75`: `<!-- radio: option body is an
    AdeChip -->`.
  - `MethodSelect.vue:40-55`: `<!-- radio: option label takes the per-method colour -->`.
  - All three drop their `#indicator-icon` override.

### F3. Dead tooltip wrappers around RunState

`<Tooltip :disabled="true"><TooltipTrigger as-child><RunState …/></TooltipTrigger></Tooltip>`, no
content: `BrowseView.vue:387`, `StreamToolbar.vue:343`, `DocumentToolbar.vue:195`,
`ConsoleView.vue:367`. Siblings (KeyValuePane, HttpRequestView, DefinitionView, GrpcRequestView)
render RunState bare. Change: render `<RunState :state="runState" />` bare; drop unused Tooltip
imports.

### F4. TextTooltip around icon-only Button

- `ade/v2/review/AdeReviewRemoved.vue:89` (size `kira-icon`, separate aria-label, `:pending`).
- `packages/workbench/src/components/TabStripNewButton.vue:23` (size `title`, separate aria-label,
  `aria-haspopup`, testid `tab-strip-new`).

Change: TooltipIconButton with `icon`, `label`, `aria-label` prop, `size`, and passthrough attrs
(`aria-haspopup`, testid). If TooltipIconButton lacks `pending`, AdeReviewRemoved passes it through
`$attrs` to Button (check: TooltipIconButton forwards attrs to Button). No new prop unless forwarding
fails; then add `pending?: boolean` to TooltipIconButton.

### F5. Shortcut chips by hand

- Hand `<Kbd>` in TooltipIconButton slot: `AdeTaskCard.vue:213`, `AdeBranchRow.vue:422`, `:556`.
  Change: `:kbd="formatShortcut('ade.reviewCode')"`; slot keeps the VarText body only.
- Text plus Kbd raw triples: `RepoFileEditBar.vue:29-44`, `DiscoverSavedControls.vue:180-196`.
  Change: add `kbd?: string` to `TextTooltip.vue` (renders `<Kbd>` after text, same as
  TooltipIconButton); convert both to `<TextTooltip :text :kbd>`.

### F6. Flush popover top bars: five insets

| Host | Now |
| --- | --- |
| `AdeAddPopover.vue:182` | `div.p-1 > SecondaryTabs w-full` |
| `RefsPopover.vue:442,457` | `SecondaryTabs class="mx-1 mt-1"`, `div.m-1 > SearchField` |
| `PreviewCommandPanel.vue:112` | `SecondaryTabs class="m-2 self-start" variant="segmented"` |
| `ColumnsMenu.vue:133` | `div.flex.gap-1.p-1` All/None |
| `ProjectionMenu.vue` (same bar) | `div.flex.gap-1.p-1` |
| `FkPreviewPopover.vue:103,114` | `div.flex.items-center.justify-between.flex-none.gap-1.p-1.5` title, `div.flex.flex-none.gap-1.p-1.5` actions |

Change: new `PopoverBar.vue` (theme popover dir, `flex flex-none items-center gap-1 p-1`, `class`
merge via `cn` for `justify-between` only). Every host above wraps its bar in PopoverBar; margin
classes on SecondaryTabs/SearchField dropped (`w-full` stays where tabs fill). FkPreviewPopover title
row: PopoverHeader-style title inside PopoverBar `justify-between`.

### F7. Flush row list: four body recipes

- `flex flex-col gap-0.5 p-1`: AdeTaskChildren:76, AdeAgentsStatusList:56, RunningOpsStatusItem:53,
  StreamFilterRow:237, VariablesOverviewPanel:109.
- `flex flex-col gap-px p-1`: CellGenerateMenu:37.
- `p-1`, no gap: SavedListMenu:91,130, ColumnsMenu:150, ProjectionMenu.
- `flex max-h-80 flex-col overflow-auto p-1`: VariableHistoryMenu:48. FkPreviewPopover:123 `p-1.5`.

Change: new `PopoverList.vue` (`flex flex-col gap-0.5 p-1`; `class` merge for `max-h-*
overflow-auto` only). All hosts above use it.

### F8. Hand rows in flush lists

- `RunningOpsStatusItem.vue:54` `flex items-center gap-2 rounded-kira-sm px-2 py-1`, two-line body.
  Change: `rowVariants({ layout: 'double' })`, detail in `rowMeta`.
- `VariableHistoryMenu.vue:57` `flex items-center gap-1 px-1.5 py-1`. Change: `rowVariants()`.
- `AdeUpdatesPopover.vue:28` hosts `AdeTaskUpdates framed=false`: padded `px-2 py-1.5 gap-1.5`
  block, Badge title, hand rows `flex items-center gap-2`. Change: popover takes list shape:
  SectionHeading (title, count) + PopoverList of `rowVariants()` rows. `framed=false` branch in
  AdeTaskUpdates renders that shape; framed card branch unchanged.

### F9. Footers: three shapes

- `RunningOpsStatusItem.vue:84`, `AdeAgentsStatusList.vue:65`: `Button variant="link"
  size="inline"`. Change: PopoverFooter + `toolbar` `kira` Button with leading icon (`link-external`
  or existing icon of the target view).
- SavedListMenu `#footer` hosts (`FilterHistoryMenu`, `ConsoleSavedMenu`,
  `console/kibana/DiscoverSavedMenu.vue`): hand `Separator my-1` + `button :class="cn(rowVariants(),
  'w-full text-primary')"` with hand icon column. Change: `SavedListMenu.vue` wraps its `#footer`
  slot in PopoverFooter; hosts pass a `toolbar` `kira` Button with leading `add` icon. DiscoverSaved
  Alert moves above PopoverFooter (stays in list area).

### F10. Missing header

`AdeAgentsStatusList` has no SectionHeading; status-bar sibling RunningOpsStatusItem has one. Change:
SectionHeading with `ade.agents.status.title` key (add to catalog if absent, en only plus existing
locales per i18n rule).

### F11. Off-inset blocks inside popovers

- `AdeAddPopover.vue` new-task form: `p-3 gap-2 text-kira-md` inside flush; FieldDescription `px-3
  py-1.5`, FieldError `px-3 pb-2`. Change: form block `flex flex-col gap-2.5 p-2.5` (default
  popover inset), Description/Error lose padding classes and sit inside it.
- `ErrorPopover.vue:45` FieldError `p-2` / `px-2 pb-1` inside flush. Change: wrap in `div.p-2.5`;
  no padding on FieldError.
- `console/kibana/TimePicker.vue:92-135` (default popover): hand span labels `w-10` in
  `div.flex.items-center.gap-2.text-kira-sm.text-muted-foreground`; apply button `self-end` in a
  wrapper. Change: from/to become `Field orientation="horizontal"` + FieldLabel `for` (U133 holds
  ids); apply row `div.flex.justify-end.gap-1` same as other default popovers.

### F12. Selects and comboboxes: nothing real

No shadcn Select/Combobox. Searchable pickers go through `PickerPopover`/`PickerCombobox` (12
callers). Hand Command lists: `AdeContextFields.vue:70-89` (task list), `ScheduleFields.vue`
(timezone), `AdeArchivedFinder`, `AdeSessionsList`. Each has content PickerCombobox would not add
(task title + state, archived two-line rows, session rows, flat zone list with `pickZone`), and all
use theme `Command*` primitives. NativeSelect (47) is the form select per Part 3. No change.

## Theme and workbench pieces

- `packages/theme/src/components/ui/popover/PopoverBar.vue`, `PopoverList.vue`: new, exported from
  popover `index.ts` (shared file).
- `DropdownMenuRadioItem.vue`, `DropdownMenuCheckboxItem.vue`: codicon default indicator.
- `TextTooltip.vue`: `kbd?: string`.
- `contextMenu.ts`, `MenuItems.vue`: radio, checkbox kinds.

## Stream decision

One sequential implementer. F1 changes the shared MenuItem union and MenuItems; F2 producers depend
on it. Popover and tooltip findings are independent of F1 but small; splitting adds a worktree for
little gain. Against iterations 4 and 6: overlap only in shared files below.

## Commits

1. `feat(theme): codicon indicator in menu radio and checkbox items, PopoverBar, PopoverList,
   TextTooltip kbd` (F2 primitives, F6/F7 pieces, F5 prop).
2. `feat(workbench): radio and checkbox MenuItem kinds` (F1: contextMenu.ts, MenuItems.vue).
3. `refactor(ui): menu producers use radio and checkbox kinds` (F2 data producers: board menu.ts,
   useTaskMenu.ts, RepoList, CellEditorView, grid/menu.ts, project/menu.ts, collectionMenu.ts).
4. `refactor(ui): hand menu radios and checkboxes become MenuItem data` (F2 hosts: AdeStepMenu,
   AdeStepMover, EnvironmentSelect, EndpointChip, OpLogToolbar, MemoryFilters; comments on the three
   hand hosts) plus spec selector updates.
5. `refactor(ui): popover bars, row lists, rows and footers` (F6-F11).
6. `refactor(ui): tooltip wrappers and shortcut chips` (F3-F5).
7. `test(ui): guards for menu indicator, dead tooltips, icon tooltips, popover insets` (guards after
   the fixes they hold).
8. `docs: P672 Part 5 iter2 rules in ARCHITECTURE`.
9. `docs: P672 Part 10 progress, Part 5 iter2` (SPEC, last).

## Guards

Provisional ids `G5a`-`G5e`. Iterations 4 and 6 add guards in parallel: orchestrator assigns final
U ids (next after U133) at landing, in landing order. Each lands with zero offenders. Header line:
`# Guards <ids>, U123 extended: docs/v0.3.17/plans/P672-part5-iter2.md.`

```
G5a@@v@@@@menu indicator override (the primitive draws the codicon check)@@<template #indicator-icon\b
G5b@@v@@@@disabled Tooltip wrapper (render the trigger bare)@@<Tooltip\b(?&t)\s:disabled="true"
G5c@@v@@@@TextTooltip around an icon-only Button (use TooltipIconButton)@@(?s)<TextTooltip\b(?&t)(?<!/)>\s*<Button\b(?&t)\ssize="(?:kira-icon|icon|title)"
G5d@@v@@@@Kbd inside a TooltipIconButton body (use :kbd)@@(?s)<TooltipIconButton\b(?&t)(?<!/)>(?:(?!</TooltipIconButton>).)*?<Kbd\b
G5e@@v@@PopoverBar.vue|PopoverList.vue@@margin or padding class on a popover bar control (use PopoverBar)@@<(?:SecondaryTabs|SearchField)\b(?&t)\s:?class="[^"]*${TOK}(?:m|mx|my|mt|mb|ml|mr|p|px|py)-
```

G5c: implementer confirms the icon sizes list against Button `size` variants before landing (all
icon-only sizes). G5e excludes `components/ui/` already via DIRS.

U123 extended in place (same id): TooltipContent body allows trailing `<Kbd>{{…}}</Kbd>`, so text
plus Kbd triple counts as plain:

```
…(?:\{\{(?:(?!\}\}).)*\}\}|[^<{]+)\s*(?:<Kbd>\{\{(?:(?!\}\}).)*\}\}</Kbd>\s*)?</TooltipContent>\s*</Tooltip>
```

U123 message gains `(text and kbd: TextTooltip :kbd)`. No checked-field guard: removed field fails
typecheck. PERMANENT goes 48 to 53.

## Docs updates

- ARCHITECTURE "UI uniformity rules (P672)":
  - menus bullet: "hand entry only for radio, checkbox or rich label" becomes "MenuItem kinds item,
    submenu, radio, checkbox, separator, label; hand entry only for option content a kind cannot
    carry (AdeChip, per-method colour, rich label block), with a comment. Indicator is the
    primitive's codicon check, never overridden. Skipped or blocked option: disabled plus hint."
  - popovers bullet: flush pieces list adds PopoverBar (top bar) and PopoverList (`gap-0.5 p-1`
    row list); action row is PopoverFooter with toolbar kira Buttons, leading icon; form block in a
    flush popover uses `p-2.5`.
  - plain-hint bullet: TextTooltip `kbd`; TooltipIconButton shortcut only via `kbd`.
  - guard range lines (~1436 "U83-U133", ~1471 "Guards U110-U133"): extend to final ids.
- `scripts/check-ui-primitives.sh` header line above.
- SPEC `P672 Part 10` section: `Part 5 iter2: menu radio/checkbox kinds, popover bar/list pieces,
  tooltip wrappers, <n> guards, plan plans/P672-part5-iter2.md.`

## Test split

No new unit tests. Implementer runs targeted UI specs once near phase end (`bun run build:mobile`
first if a mobile spec is in set). Timeout under load: one lone rerun first.

- Menus (F1, F2): `ade-v2-task-menu`, `ade-v2-row-actions`, `ade-v2-panel` (`data-disabled` on
  `ade-step-option-plan`), `ade-v2-task-status`, `ade-v2-artifacts`, `ade-v2-links-strip`,
  `operations`, `operations-db`, `memory-ranking`, `studio/tree` (`menu-item-set-as-default`
  `.codicon-check`, now in CheckboxItem indicator), `studio/api-ui-consistency` (967 MethodSelect
  `.codicon-check`; 1235-1307 environment options: `conn-dot` becomes `menu-swatch`),
  `studio/api-cross-window-sync`, `studio/http-curl:426`, `studio/http-variables:376`,
  `studio/docker-module:243-246` (`[role=menuitemradio]` count unchanged), `studio/cell-editor`,
  `studio/mask-preview`, `studio/api-lifecycle`, repo color menu spec.
- Popovers (F6-F11): kibana time picker and saved specs, `console`, `preview-command`, `fk-preview`,
  `saved-queries`, `running-ops-status`, `status-agents`, `ade-v2-add`, `ade-v2-updates`,
  `repo-refs-popover`.
- Tooltips (F3-F5): `tooltips`, repo file edit spec, tab strip spec, ADE review removed spec, ADE
  card/branch row specs touching `ade.reviewCode`.
- Screenshot `branchesHandover` (popover footer). Visual baselines re-record in PR CI.
- Lessons: Monaco menu items need hover before click; reka radio arrows use
  `tests/ui/support/radioKeys.ts` (`delay:40`); shared helpers (N8); 600-line `.vue` cap (N10); N17
  marker stays.

## Risks

- `DropdownMenuRadioGroup` model is a string: `null` values (mask none, collection none, env none)
  map to a sentinel option `value` (`__none`) inside MenuItems; producers keep `null` in data.
- Reka radio item closes menu on select; current behaviour same. Checkbox `keepOpen` must keep
  OpLogToolbar and MemoryFilters open as today (`@select.prevent`).
- Plan entry (`menu.ts:249`) loses `disabled: here`; radio on same value must be a no-op in `run`.
- AdeStepMenu/AdeStepMover lose visible "skipped" suffix; specs asserting that text switch to hint.
- EnvironmentSelect dot becomes swatch (size-2.5 vs 1.25): visual baseline change, intended.
- AdeTaskUpdates `framed=false` change must not alter framed card use.

## Out of scope

- Select/combobox restructuring (F12: none needed).
- ProofScreenshotMenu rich block, rich disabled-condition tooltips, RefreshButton.
- Accessibility-only items.

## File list

Owned by this iteration:

- `packages/workbench/src/state/contextMenu.ts`
- `packages/workbench/src/components/MenuItems.vue`
- `packages/theme/src/components/ui/dropdown-menu/DropdownMenuRadioItem.vue`
- `packages/theme/src/components/ui/dropdown-menu/DropdownMenuCheckboxItem.vue`
- (if present) ContextMenu radio/checkbox primitives in `packages/theme/src/components/ui/context-menu/`
- `packages/theme/src/components/ui/popover/PopoverBar.vue` (new)
- `packages/theme/src/components/ui/popover/PopoverList.vue` (new)
- `packages/theme/src/components/TextTooltip.vue`
- `packages/theme/src/components/TooltipIconButton.vue` (only if `pending` forwarding fails)
- `apps/kira-space/frontend/src/ade/v2/board/menu.ts`
- `apps/kira-space/frontend/src/ade/v2/plan/useTaskMenu.ts`
- `apps/kira-space/frontend/src/ade/v2/plan/AdeStepMenu.vue`
- `apps/kira-space/frontend/src/ade/v2/plan/AdeTaskStatusMenu.vue`
- `apps/kira-space/frontend/src/ade/v2/panel/AdeStepMover.vue`
- `apps/kira-space/frontend/src/ade/v2/panel/AdeStatusPicker.vue`
- `apps/kira-space/frontend/src/repo/RepoList.vue`
- `apps/kira-space/frontend/src/studio/views/shared/celleditor/CellEditorView.vue`
- `apps/kira-space/frontend/src/studio/views/shared/celleditor/CellGenerateMenu.vue`
- `apps/kira-space/frontend/src/studio/views/grid/menu.ts`
- `apps/kira-space/frontend/src/studio/project/menu.ts`
- `packages/workbench/src/lib/collectionMenu.ts`
- `apps/kira-space/frontend/src/studio/api/MethodSelect.vue`
- `apps/kira-space/frontend/src/studio/api/EnvironmentSelect.vue`
- `packages/docker-ui/src/components/EndpointChip.vue`
- `packages/workbench/src/components/OpLogToolbar.vue`
- `packages/workbench/src/memory/MemoryFilters.vue`
- `apps/kira-space/frontend/src/studio/views/browse/BrowseView.vue`
- `apps/kira-space/frontend/src/studio/views/stream/StreamToolbar.vue`
- `apps/kira-space/frontend/src/studio/views/documents/DocumentToolbar.vue`
- `apps/kira-space/frontend/src/studio/views/console/ConsoleView.vue`
- `apps/kira-space/frontend/src/ade/v2/review/AdeReviewRemoved.vue`
- `packages/workbench/src/components/TabStripNewButton.vue`
- `AdeTaskCard.vue`, `AdeBranchRow.vue` (ADE v2 dirs)
- `RepoFileEditBar.vue`, `studio/views/console/kibana/DiscoverSavedControls.vue`
- `AdeAddPopover.vue`, `RefsPopover.vue`, `PreviewCommandPanel.vue`, `ColumnsMenu.vue`,
  `studio/views/documents/ProjectionMenu.vue`, `FkPreviewPopover.vue`
- `AdeTaskChildren.vue`, `AdeAgentsStatusList.vue`, `RunningOpsStatusItem.vue`,
  `StreamFilterRow.vue`, `VariablesOverviewPanel.vue`, `VariableHistoryMenu.vue`
- `SavedListMenu.vue`, `FilterHistoryMenu.vue`, `ConsoleSavedMenu.vue`,
  `studio/views/console/kibana/DiscoverSavedMenu.vue`
- `AdeUpdatesPopover.vue`, `AdeTaskUpdates.vue`, `ErrorPopover.vue`,
  `studio/views/console/kibana/TimePicker.vue`
- i18n catalog entry for `ade.agents.status.title` (only if absent)
- Specs under `tests/ui/` named in Test split whose selectors change (api-ui-consistency,
  http-curl, http-variables, ADE step specs asserting skipped text)

Shared (merge with iterations 4 and 6):

- `scripts/check-ui-primitives.sh` (5 PERMANENT lines, U123 edit, header line)
- `docs/ARCHITECTURE.md` (menus, popovers, plain-hint bullets; guard range lines)
- `docs/v0.3.17/SPEC.md` (one progress line)
- `packages/theme/src/components/ui/popover/index.ts` (PopoverBar, PopoverList exports)
- possible overlap: Buttons converted in F4/F9 (iteration 4 buttons) and SectionHeading in F10/F8
  (iteration 6 headings); orchestrator checks these hosts against their file lists.
