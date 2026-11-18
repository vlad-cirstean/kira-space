# P672 Part 5 UI uniformity: menus, popovers, tooltips

Spec row: `../SPEC.md` P672 Part 5. Rules RM.1-RM.4, RT.1-RT.2, migration items M16-M19
(`P672-inventory.md` §5, §11, appendix A.2, A.11). Base: `chapter/v0.3.17` at `18cbdeb0b`.
Inventory line numbers are at `6527aec4`. Every fact below was re-checked at `18cbdeb0b` via
`codegraph_explore` and template scans. Line numbers are hints: implementer greps the named pattern
or `data-testid` before editing.

UI-only phase. Look and layout change. Behaviour, i18n keys and every `data-testid` stay. No new
i18n key.

## Rules applied (recap)

- RM.1 data menus. A menu is a `MenuItem[]` rendered by `MenuItems`
  (`packages/workbench/src/components/MenuItems.vue`); the host owns `DropdownMenu`,
  `DropdownMenuTrigger` and `DropdownMenuContent`. A hand entry stays only for a kind `MenuItem`
  cannot express: `DropdownMenuRadioGroup`/`DropdownMenuRadioItem`, `DropdownMenuCheckboxItem`, a
  rich `DropdownMenuLabel` block. Each hand block gets a one-line comment naming why.
- An option picker that shows the current value is a radio group (`DropdownMenuRadioGroup`), never
  plain items with a check icon. Twins today: `AdeTaskStatusMenu`, `AdeStepMenu`, `MethodSelect`,
  `EndpointChip`.
- RM.2 menu icons 13 and muted, never toned. A destructive item is `danger: true` (primitive
  variant `danger`; U26 forbids `destructive`), and its icon follows the item's error colour.
  Action menus use an icon; option lists (kinds, presets, mask kinds, steps) use none.
  `MenuItems` reserves the icon column either way.
- Separators are `{ kind: 'separator' }`; section titles `{ kind: 'label' }`. Shortcut hints only
  via `shortcut: ShortcutId` (rendered as `DropdownMenuShortcut`), never hand text. None of the
  hand menus below shows a shortcut today.
- A disabled item's reason is `hint` (tooltip plus sr-only text), never a visible second line.
  `sectionsToMenuItems` (`packages/git-ui/src/lib/menuModel.ts`) already maps `disabledReason` to
  `hint`.
- RM.3 two popover shapes. Form: default `PopoverContent` (`flex flex-col gap-2.5 p-2.5`), fields
  and an action row as direct children. List: `PopoverContent flush`, optional header
  (`SectionHeading` or `PopoverHeader`), a `p-1` row list (`rowVariants()` rows), optional
  `PopoverFooter`.
- RM.4 widths `w-56`, `w-80`, `w-96`, `w-120`; `w-auto` only for a popover that sizes to one
  control (colour grid, estimate stepper, autocomplete). No padding or gap class on
  `PopoverContent` (U111).
- RT.1 plain-text tooltip is `TextTooltip` (`text` or `parts`). A tooltip with markup, a class on
  `TooltipContent`, a controlled `open`, a `v-if` on `Tooltip` or attrs on the trigger stays the
  rich `Tooltip` triple.
- RT.2 delays are theme-owned (`TOOLTIP_DELAY_MS`, U29 forbids provider props). Scan at base:
  zero `delay-duration` in app code. Nothing to change. Wording stays (copy specs hold it).

## Baselines

`scripts/check-ui-primitives.sh` TEMPORARY block holds 32 lines naming `P672 Part 5`
(`grep -c 'P672 Part 5'` = 32): U111 lines 155-185 (31 files) and U116 line 244
(`packages/docker-ui/src/components/ContainerList.vue`). Each line is deleted in the same commit
that fixes its file; the stale check fails a commit that fixes a file and leaves its line, so the
two go together. Run `bash scripts/check-ui-primitives.sh` after every commit (part of
`bun run lint`). New guards U122-U123 (C8) land with zero offenders and no TEMPORARY line.

## Theme and workbench pieces

All in C1. Each piece has a concrete consumer in this part.

1. `MenuItem` (`packages/workbench/src/state/contextMenu.ts`) item and submenu gain
   `testId?: string`. `MenuItems` uses `const tid = item.testId ?? \`menu-item-${item.id}\``
   for the item or sub trigger, `${tid}-shortcut` for the shortcut and `${tid}-reason` on the
   hint sr-only span (new testid). Why: every hand menu below carries its own testids
   (`ade-lease-force`, `kibana-revert`, …); P672 keeps every testid. `id` stays unique (it is the
   `v-for` key and the hint id); `testId` may repeat (AdeTaskCard).
2. Item gains `parts?: readonly TextPart[]`; when set, `MenuItems` renders `<VarText :parts>` in
   place of `label`. `label` stays required and carries `plainText(parts)` (typeahead, a11y).
   Why: AdeTaskCard change-base items render `VarText`.
3. Danger item icon: drop the `text-muted-foreground` fallback when `item.danger`, so the
   `<i>` inherits `text-error` from the item. Why: `CodiconIcon` renders `<i>`, and the primitive
   rule `data-[variant=danger]:*:[svg]:text-error` never reaches it; RM.2 says the icon follows.
4. Remove `iconClass` from the type and from `MenuItems`. Only producer is ContainerList
   (`'text-ok'` start, `'text-error'` stop); drop both. U116 line 244 goes. Why: RM.2 forbids
   toned icons; a field with no legal use goes, no dead prop.
5. New `packages/theme/src/components/ui/popover/PopoverFooter.vue`, exported from the popover
   `index.ts`: `<div data-slot="popover-footer" :class="cn('flex items-center justify-end gap-1
   border-t border-border px-1.5 py-1', props.class)"><slot /></div>` (`class?:
   HTMLAttributes['class']`). Why: the list footer recipe (ARCHITECTURE line 1458 "footer px-1.5
   py-1") is hand-copied in ErrorPopover, AdeTaskAddMenu, RunningOpsStatusItem,
   AdeAgentsStatusList and the MemoryFilters picker footer; shadcn-vue ships `DialogFooter` but
   no popover footer.
6. `PointPopover` (workbench) gains `flush?: boolean`, forwarded to its `PopoverContent`. Why:
   FkPreviewPopover passes `gap-0 overflow-hidden p-0` by class today, the hand copy of `flush`.
7. First consumers of existing `PopoverHeader`/`PopoverTitle` (zero callers at base): KeyValue
   popovers and StreamComposeMessage (C5).

Library fit (CLAUDE.md Frontend): every primitive is shadcn-vue (DropdownMenu*, Popover*,
Tooltip*), styling is Tailwind utilities, no scoped `<style>` added. No new composable, Pinia store
or TanStack query: all work is template and type level; existing stores and queries feeding these
menus stay as they are.

## Shapes (copy exactly)

Data menu in a host:

```vue
<DropdownMenuContent align="end">
  <MenuItems :items="menuItems" />
</DropdownMenuContent>
```

```ts
const menuItems = computed<MenuItem[]>(() => [
  { kind: 'item', id: 'force', testId: 'ade-lease-force', label: t('…'), icon: 'debug-disconnect',
    danger: true, run: () => forceRelease() },
]);
```

List popover:

```vue
<PopoverContent flush class="w-96">
  <SectionHeading>{{ title }}</SectionHeading>
  <div class="flex flex-col gap-0.5 p-1"><!-- rows: rowVariants() --></div>
  <PopoverFooter><Button variant="toolbar" size="kira">…</Button></PopoverFooter>
</PopoverContent>
```

Form popover: `<PopoverContent class="w-80">` with `PopoverHeader`/`PopoverTitle` (optional),
`Field` children, action row `<div class="flex justify-end gap-1">` (dismiss `dialog@kira
data-dialog-dismiss`, then the emphasised one rightmost).

Tooltip: `<TextTooltip :text="expr"><Button …/></TextTooltip>` (string literal: `text="…"`; a
`{{ t('k') }}` body becomes `:text="t('k')"`).

## Stream decision

One sequential implementer. C1 changes `MenuItem` and `MenuItems`, which C2-C3 consume; the guard
script TEMPORARY block, ARCHITECTURE and several hosts (DiscoverSavedControls, OpLogToolbar,
DocumentToolbar, SavedListMenu, TimestampPane, StreamFilterRow, VariablesOverviewPanel,
RefsPopover) are touched by more than one of menus, popovers and tooltips. No split.

## Commits

Order is binding. Run `bun run lint` (includes `check-ui-primitives.sh`) and `bun run typecheck`
before each commit. Every commit passes hooks without `--no-verify`. Any spec or fix commit found
later goes before C9 and C10 (rebase it in place; the docs commits stay last).

### C1 `feat(ui): menu item testId and parts, popover footer, drop iconClass`

- `packages/workbench/src/state/contextMenu.ts`: pieces 1, 2, 4 on the type.
- `packages/workbench/src/components/MenuItems.vue`: pieces 1-4 (render `tid`, `parts`, danger
  icon colour, drop `iconClass`).
- `packages/docker-ui/src/components/ContainerList.vue`: drop both `iconClass`; stop gets
  `danger: true` only if it is not already (verify); delete U116 line 244.
- `packages/theme/src/components/ui/popover/PopoverFooter.vue` new, export in `index.ts`.
- `packages/workbench/src/components/PointPopover.vue`: piece 6.
- Grep `iconClass` across `apps packages` after: zero hits.

### C2 `refactor(ade): data menus (M16, M17)`

- `ade/v2/agents/AdeLeasesPane.vue`: holder menu to `MenuItems`. Items `request`
  (`ade-lease-request`, icon `bell`) and `force` (`ade-lease-force`, icon `debug-disconnect`,
  `danger`).
- `ade/v2/review/AdeReviewActions.vue`: items `testId: \`ade-review-action-${event}\``, disabled
  when gated, `hint: gate.reason`. The visible reason div goes; its testid
  `ade-review-action-${event}-reason` now sits on the `MenuItems` sr-only span (piece 1), so
  `ade-v2-review.spec.ts` (around lines 958, 1010) keeps its selector. A `toBeVisible` on it still
  holds (sr-only is 1px, not hidden); if a spec reads it as visible text under the item, assert
  `toHaveText` instead.
- `ade/v2/plan/AdeTaskCard.vue`: change-base menu to `MenuItems`; per choice `{ id: c.id-or-index,
  testId: 'ade-card-change-base-item', label: plainText(c.parts), parts: c.parts, run: () =>
  dialogs.act(c.act) }`. Board look is D1-exempt; only the menu changes.
- `ade/v2/plan/AdeStepMenu.vue`: live label, back and next go to `MenuItems` (`label` kind, items
  with icons `arrow-left`/`arrow-right`, keep testids), then `DropdownMenuSeparator`, then the
  step `DropdownMenuRadioGroup` stays hand with comment `<!-- radio: MenuItem has no radio kind
  -->`. `ade-v2-row-actions` "Next step" by role `menuitem` stays valid.
- `ade/v2/plan/AdeTaskStatusMenu.vue`: radio-only; add the comment only.
- `ade/v2/panel/AdeStatusPicker.vue`: plain items to `DropdownMenuRadioGroup` +
  `DropdownMenuRadioItem` (copy AdeTaskStatusMenu: `AdeChip` in the item, primitive indicator),
  testids `ade-status-option-${id}` on the radio items; comment.
- `ade/v2/panel/AdeStepMover.vue`: plain items to a radio group like AdeStepMenu; skipped steps
  `disabled` (`ade-v2-panel` reads `data-disabled` on `ade-step-option-plan`: radio items carry
  it); comment.
- `ade/v2/workflows/AdeStepsTab.vue`: new-step kinds to `MenuItems` (`ade-step-new-${k}`, no
  icon).
- `ade/v2/workflows/inspector/AdeStepSandbox.vue`: presets to `MenuItems`
  (`ade-wf-sandbox-preset-${id}`, no icon).
- `ade/v2/workflows/AdeWorkflowGraph.vue`: kinds (`ade-wf-add-step-${k}`), separator, `label`
  item, library steps (`ade-wf-add-library-${id}`).
- `ade/v2/workflows/AdeStepNode.vue`: keep `v-model:open` on the host root. Submenu `add-after`
  (`testId: 'ade-wf-step-add-after'`, icon `add`, children `ade-wf-step-add-after-${k}`),
  separator, up/down/start/skip (icons `arrow-up`, `arrow-down`, `debug-start`, `debug-step-over`;
  keep testids), separator, remove (`ade-wf-step-remove`, icon `trash`, `danger`; the hand
  `class="text-error"` goes).
- `ade/v2/panel/AdeTaskBranchMenu.vue`: add/change branch (`git-branch`), change repo (`repo`),
  remove branch (`trash`, `danger`), separator, remove repo (`trash`, `danger`); keep testids
  (`support/branchMenu.ts` drives them).

All ADE paths are under `apps/kira-space/frontend/src/`.

### C3 `refactor(ui): studio, workbench and docker data menus (M16)`

- `studio/views/console/kibana/DiscoverSavedControls.vue`: `kibana-save-as` (`save-as`),
  `kibana-revert` (`discard`), `kibana-detach` (`debug-disconnect`) to `MenuItems`.
- `studio/api/MethodSelect.vue`: radio-only; comment only.
- `studio/api/EnvironmentSelect.vue`: radio group stays hand (comment); `api-environment-manage`
  (`settings-gear`) after a separator to `MenuItems`.
- `studio/project/connection/MaskKindMenu.vue`: kinds to `MenuItems` (`mask-kind-menu-${v}`, no
  icon).
- `studio/proof/ProofScreenshotMenu.vue` (in `SplitButton #menu`): copy (`copy`), save-all
  (`save-all`), choose (`folder`), reset (`discard`) to `MenuItems`, keep testids; the rich
  location `DropdownMenuLabel` block stays hand with comment.
- `packages/workbench/src/components/OpLogToolbar.vue`: `ops-facet-all` to `MenuItems`; checkbox
  items stay (comment).
- `packages/workbench/src/memory/MemoryFilters.vue`: checkbox-only; comment only.
- `packages/workbench/src/memory/import/ImportMenu.vue`: `memory-import-files` (`files`),
  `memory-import-folder` (`folder`) to `MenuItems`.
- `packages/docker-ui/src/components/EndpointChip.vue`: radio-only; comment only.

After C3: `grep -rlP '<DropdownMenuItem\b' --include=*.vue apps packages` returns only
`DropdownMenuItem.vue` and `MenuItems.vue`.

### C4 `refactor(ade): popover shapes and widths (M18)`

Delete each file's U111 line in this commit. All under `apps/kira-space/frontend/src/ade/v2/`.

- `AdeAddPopover.vue`: `flush class="w-120"`.
- `links/AdeLinkAdd.vue`: `class="w-80"` (drop `gap-1.5`); wrap Input, FieldDescription and
  FieldError in one `<Field>`.
- `links/AdeLinkStrip.vue` (`w-80`): no popover change (tooltip in C7).
- `panel/AdeRunSandboxBadge.vue`: drop the redundant `text-kira-md`.
- `panel/AdeTaskBranchForm.vue`: `flush class="w-80"`; error wrapper `m-2` to `p-1`.
- `plan/AdeArchivedFinder.vue`: `flush class="w-96"`.
- `plan/AdeEstimateChip.vue`: `class="w-auto"` (drop `p-2`).
- `plan/AdeTaskAddMenu.vue` (already flush): add-branch button with `border-t` moves into
  `<PopoverFooter>` as `variant="toolbar" size="kira"` with icon `add`; error wrapper to `p-1`.
- `plan/AdeTaskChildren.vue`: `flush class="w-80"`; inner `div.flex.flex-col.gap-0.5.p-1`; each
  row a `<button>` with `cn(rowVariants(), 'w-full border-0 bg-transparent text-left')`, keep
  testid, `data-task-id`, `@click.stop`.
- `sessions/AdeSessionsMenu.vue`: `flush class="w-120"`.
- `status/AdeAgentsStatusItem.vue`: `flush class="w-96"`. `AdeAgentsStatusList.vue`: rows in
  `div.flex.flex-col.gap-0.5.p-1`; the `status-agents-all` link moves into `PopoverFooter`.
- `updates/AdeUpdatesPopover.vue`: `flush class="w-80"`.
- `workflows/statuses/AdeTaskStatuses.vue` colour popover: `class="w-auto"` (drop `p-2`).

### C5 `refactor(ui): studio and api popover shapes (M18)`

Delete each offender's U111 line in this commit. Studio paths under
`apps/kira-space/frontend/src/studio/`.

- `api/VariableHistoryMenu.vue`: `flush class="w-80"`.
- `api/VariablesOverviewPanel.vue`: `flush class="w-96"`.
- `project/ErrorPopover.vue`: `flush class="w-96 max-h-60"`; `ViewToolbar` footer becomes
  `PopoverFooter` with Copy (`copy`) and Close (`close`) `toolbar@kira`, keep testids.
- `views/console/kibana/FilterBar.vue` (two tags): `class="w-80"`.
- `views/console/kibana/TimePicker.vue`: `class="w-80"`; drop `mt-2 pt-2` on the inner wrapper.
- `views/documents/DocumentToolbar.vue`: `flush class="w-56"`; `ProjectionMenu.vue` list `p-0.5`
  to `p-1`.
- `views/grid/ColumnsMenu.vue`: `flush class="w-56"`; list `p-0.5` to `p-1`.
- `views/grid/PreviewCommandPanel.vue`: `flush class="w-120"`; `PanelHeader` becomes
  `SectionHeading` with the close `TooltipIconButton` (`preview-command-close`) in its slot.
- `views/shared/SavedListMenu.vue`: `flush class="w-80"`; add `p-1` to the listbox div.
- `views/shared/celleditor/CellGenerateMenu.vue`: `flush class="w-56"`.
- `views/shared/celleditor/TimestampPane.vue` calendar: `flush class="w-56"`.
- `views/shared/keyvalue/KeyValueAddPopover.vue`, `KeyValueEditPopover.vue` (`w-80`, not
  offenders): drop the inner `div.flex.flex-col.gap-1.5.p-1.5`; `SectionHeading` title becomes
  `<PopoverHeader><PopoverTitle>`.
- `views/stream/StreamToolbar.vue`: `w-96 p-0` to default form `class="w-96"`.
  `StreamComposeMessage.vue`: `PanelHeader` goes; `PopoverHeader`/`PopoverTitle` instead, fields
  as direct children (multi-root), action row right-aligned with `kira` buttons. Grep specs
  (`stream-produce.spec.ts`) for the header close button before removing it; if a spec closes via
  it, close with Escape instead (popover dismiss) and update the spec in this commit.
- `views/stream/StreamFilterRow.vue` (two tags): `flush class="w-56"`.

### C6 `refactor(ui): git and workbench popover shapes (M18)`

Delete each offender's U111 line in this commit.

- `packages/git-ui/src/App.vue` force-delete popover: form `class="w-80" destructive`, keep
  `:side-offset="0"`; text as `<p>`; action row `div.flex.justify-end.gap-1`, Cancel
  `dialog@kira data-dialog-dismiss` first, then `dialog-danger@kira` rightmost; keep testids.
- `packages/git-ui/src/components/RefsPopover.vue`: `flush class="w-96"`.
- `packages/git-ui/src/components/SearchBox.vue`: `flush class="w-120"`.
- `packages/workbench/src/automations/form/ScheduleFields.vue`,
  `packages/workbench/src/automations/run/AdeContextFields.vue`: `flush class="w-96"`.
- `packages/workbench/src/components/AutocompleteField.vue`:
  - suggestions: `flush`, keep `autocomplete-suggestions z-(--kira-z-autocomplete) w-auto
    min-w-50 max-w-completion-max-w max-h-60 overflow-x-hidden overflow-y-auto font-data`; `p-0.5`
    moves to the `ul`. twMerge keeps the later `overflow-y-auto` over flush `overflow-hidden`
    (class prop merges after the variant); verify scroll in `studio/autocomplete.spec.ts`.
  - hover panel: `flush`, keep `var-hover-panel pointer-events-none z-(…) w-auto max-w-96
    font-data`; `px-1.5 py-1` moves to an inner `div`.
- `packages/workbench/src/components/pickers/PickerPopover.vue`: `flush class="w-80"`.
  `MemoryFilters` `#footer` becomes `<PopoverFooter v-if="project">` with a `toolbar@kira`
  button.
- `apps/kira-space/frontend/src/studio/views/grid/FkPreviewPopover.vue`: `<PointPopover flush class="w-80
  max-h-(--reka-popover-content-available-height)">` (drop `gap-0 overflow-hidden p-0`).
- `packages/workbench/src/components/RunningOpsStatusItem.vue`: `flush class="w-96"`; title div
  becomes `SectionHeading`; rows in a `p-1` list; footer link into `PopoverFooter`.

After C6: `grep -c 'U111 ' scripts/check-ui-primitives.sh` returns 0 TEMPORARY lines, and U111
reports no offender.

### C7 `refactor(ui): plain tooltips through TextTooltip (M19)`

Every strict triple (pattern of U123, below) becomes `TextTooltip`; drop now-unused `Tooltip*`
imports, add `TextTooltip` from `@kira/theme` (match the file's existing theme import). 64 triples
in 45 files at base:

- ADE (`apps/kira-space/frontend/src/ade/v2/`): `details/AdeTaskChain` 1, `links/AdeLinkStrip` 1,
  `repos/AdeReposPage` 2, `review/AdeReviewRemoved` 1.
- App (`apps/kira-space/frontend/src/`): `hostview/RelativeTime` 1, `repo/RepoTreeRow` 1,
  `views/repo/RepoMultiDiffView` 1.
- Studio (`apps/kira-space/frontend/src/studio/`): `api/EnvironmentsView` 1,
  `api/VariablesOverviewPanel` 1, `project/connection/MaskRuleRow` 1, `views/console/ConsoleView`
  3, `views/console/kibana/DiscoverSavedControls` 1, `views/console/kibana/FieldSidebar` 1,
  `views/definition/DefinitionView` 1, `views/documents/DocumentFilterRow` 3,
  `views/documents/DocumentToolbar` 1, `views/grid/DataToolbar` 1, `views/grid/FilterToolbar` 1,
  `views/grpcrequest/CallHistoryList` 2, `views/grpcrequest/GrpcRequestView` 2,
  `views/httprequest/BinaryBodyPicker` 1, `views/httprequest/HttpRequestView` 2,
  `views/httprequest/ResponseDiffDialog` 4, `views/httprequest/ResponseHistoryList` 2,
  `views/httprequest/ResponsePane` 2, `views/shared/DateTimePicker` 1,
  `views/shared/SavedListMenu` 1, `views/shared/celleditor/CellEditorView` 2,
  `views/shared/celleditor/TimestampPane` 1, `views/stream/StreamFilterRow` 2.
- Git (`packages/git-ui/src/components/`): AppToolbar 2, CommitMeta 2, LoadMoreButton 1,
  RefsPopover 1, StashDetailPane 1, UncommittedChangesStrip 1, UndoButton 2.
- Theme: `packages/theme/src/NumberStepperInput.vue` 2 (import `TextTooltip` relatively, as the
  file imports other theme pieces).
- Workbench (`packages/workbench/src/`): `automations/ScriptBrokenRow` 1,
  `automations/smart/SmartBadge` 1, `components/OpLogPanel` 1, `components/OpLogToolbar` 1,
  `components/TabStrip` 1, `components/TabStripNewButton` 1, `memory/import/ImportJobDetail` 1.

Never place `TextTooltip` between a `DropdownMenuTrigger`/`PopoverTrigger` and its root
(ARCHITECTURE line 95); a triple wrapping such a trigger today keeps its order as is (the trigger
stays inside the tooltip slot). If a converted trigger relied on an empty-text case showing an
empty tooltip, `TextTooltip` now shows none: intended. Loose triples stay (RT.1 rich form).

### C8 `chore(lint): guards U122-U123 for menus and tooltips`

`scripts/check-ui-primitives.sh`: header comment line after the U118 line:

```
# Guards U122-U123: docs/v0.3.17/plans/P672-part5.md.
```

Guard rows after U121:

```
U122@@v@@DropdownMenuItem.vue|MenuItems.vue@@hand DropdownMenuItem (menus render MenuItems from a MenuItem[]; radio, checkbox and label blocks are other tags, RM.1)@@<DropdownMenuItem\b
U123@@v@@@@plain-text Tooltip triple (use TextTooltip, RT.1)@@(?s)<Tooltip>\s*<TooltipTrigger as-child>(?:(?!</TooltipTrigger>|<Tooltip\b).)*</TooltipTrigger>\s*<TooltipContent>\s*(?:\{\{(?:(?!\}\}).)*\}\}|[^<{]+)\s*</TooltipContent>\s*</Tooltip>
```

Dry run at `18cbdeb0b` (same `-Pzl`, same DIRS): U122 hits the 16 C2/C3 files plus both exempt
names; U123 hits exactly the 45 C7 files (`TextTooltip.vue` itself does not match: its `Tooltip`
has `v-if` and its content has markup). After C2-C7 both are zero. Both U122 exempt names hit,
so the stale-exempt check passes. Rejected guard: PopoverContent without `flush` holding a row
list; markup nesting cannot be told apart by regex without false hits; U111 plus U15 hold the
shape.

### C9 `docs: architecture for P672 Part 5`

`docs/ARCHITECTURE.md`:

- line 1458 becomes: "Menus render MenuItems from a MenuItem[] (`testId` keeps a host testid;
  disabled reason is `hint`); a hand entry only for radio, checkbox or a rich label block, with a
  comment. An option picker showing the current value is a radio group. Menu icons 13 and muted;
  a danger item's icon follows its error colour. Popovers have two shapes: default (form: fields
  and a right-aligned action row, optional PopoverHeader) and `flush` (list: SectionHeading or
  PopoverHeader, p-1 row list, PopoverFooter px-1.5 py-1); widths w-56, w-80, w-96, w-120, w-auto
  only for a popover sized to one control."
- line 1465 (plain hint): append "A plain-text Tooltip triple fails U123."
- line 1435: `U83-U121` to `U83-U123`.
- line 1470: "Guards U110-U123 … U118-U123 landed with zero offenders and have no TEMPORARY line."

### C10 `docs: P672 Part 5 done`

`docs/v0.3.17/SPEC.md` Part 5 row: Status Done; Result names commits C1-C9 (and any spec/fix
commits), guards U122-U123, the 32 baseline lines removed (65 left), files touched, specs run,
visual baselines for CI re-record. Last commit of the phase.

## Test split

UI-only phase (no IPC, no Go, no flow test change). Frontend half: existing UI specs, updated only
where a selector or asserted look changed. No new unit test (no complex logic). No new spec: guards
U122-U123 and U111/U116 are the regression check.

Spec updates expected:

- Radio conversions (AdeStatusPicker, AdeStepMover): role becomes `menuitemradio`. Specs select
  by testid; any `getByRole('menuitem', …)` on those options retargets to `menuitemradio`.
- `ade-v2-review.spec.ts`: reason testid kept on the sr-only span; a check that the text renders
  under the item becomes `toHaveText` on the testid.
- `stream-produce.spec.ts`: compose close button, if used (C5).
- `consistency.spec.ts`, `studio/api-ui-consistency.spec.ts`, `ui-leftovers.spec.ts`: any
  popover padding or width reads update to the new shape, never drop the assertion.
- `studio/tooltips.spec.ts` (`studio/support/tooltip.ts`): TextTooltip renders the same content
  slot; recheck.

Targeted run near phase end (one pass, from `apps/kira-space`), specs touching changed testids:

- ADE: ade-ids, ade-readable-names, ade-v2-add, ade-v2-add-link, ade-v2-archive, ade-v2-archived,
  ade-v2-artifacts, ade-v2-automations, ade-v2-background-ops, ade-v2-base-rebase,
  ade-v2-branch-request, ade-v2-card-icons, ade-v2-db-audit, ade-v2-details-links,
  ade-v2-details-list, ade-v2-flat-steps, ade-v2-force-push, ade-v2-leases, ade-v2-links-strip,
  ade-v2-main-branches, ade-v2-notes, ade-v2-op-kinds, ade-v2-panel, ade-v2-proof-chain,
  ade-v2-review, ade-v2-row-actions, ade-v2-sandbox, ade-v2-session-status,
  ade-v2-sessions-menu, ade-v2-task-branches, ade-v2-task-flow, ade-v2-task-launch,
  ade-v2-task-panel, ade-v2-task-row, ade-v2-task-status, ade-v2-workflow-layout,
  ade-v2-workflow-steps, ade-v2-workflows, status-agents.
- Shared: automations-recurring, consistency, copy-coverage, copy-coverage-git, copy-wording,
  git-host-azure, list-keyboard, memory-import, memory-ingest, memory-ranking, operations,
  operations-db, repo-refs-popover, running-ops-db, running-ops-status, toast-placement,
  ui-leftovers.
- Studio: api-agent-access, api-config-files, api-cross-window-sync, api-lifecycle, api-order,
  api-secret-reveal-isolation, api-ui-consistency, api-variables-form, autocomplete, budgets,
  cell-editor, collections, connection-privacy, console, console-explain, control-sizing,
  data-view, docker-module, grid-connection-drop, http-curl, http-dynamic-values, http-request,
  http-variables, immediate-write-connection-drop, interaction, kibana-saved, mutations,
  op-id-guard, op-ids, proof-chart, proof-copy-all, proof-diagram, proof-image, proof-window,
  redis-edit, s3-object-edit, saved-queries-conflict, saved-queries-sync, secret-reveal,
  sql-schema, sqs-poll-visibility, stream-produce, tooltips, tree, view-connection-drop (all
  under `studio/`).

Sandbox lessons for spec edits:

- Playwright click on a menu item inside Monaco needs `locator.hover()` first.
- reka radio arrow presses need `keyboard.press('ArrowDown', { delay: 40 })`; applies to the new
  AdeStatusPicker/AdeStepMover radio groups if a spec drives them by keyboard.
- N17 (`scripts/check-conventions.ts`) forbids the `dialog` variant on a delete/discard label
  without an allow marker. The git force-delete Cancel is `dialog` on "Cancel" (passes); the
  delete button is `dialog-danger`. Any other `dialog` button labelled delete/discard needs the
  marker or `dialog-danger`.
- A timeout under load gets one lone rerun before any fix.

Visual baselines changed, re-recorded in PR CI (not locally): git-row-menu (danger icon colour),
status-bar-agents-popover, ade-sessions-menu, git-refs-popover, sandbox-popover, ade-links-card,
workflow-graph*, operations-list, console, data-grid*, docker-* container menu. Result lists the
ones CI actually re-recorded.

## Risks

- `flush` adds `overflow-hidden`; AutocompleteField and ErrorPopover need scroll. Class prop merges
  last, so `overflow-y-auto` wins; the autocomplete spec confirms.
- RunningOpsStatusItem and PreviewCommandPanel titles change from bold/PanelHeader to
  `SectionHeading` (uppercase subtle): intended by RM.3, visible in baselines.
- `TextTooltip` sets no attrs on the `Tooltip`; only attr-less triples move (the U123 pattern
  guarantees it).
- Radio conversions add a primitive check indicator and change role; menu width may grow by the
  indicator column.
- AdeReviewActions reason moves from a visible line to a hint tooltip: less text in the menu,
  same information on hover and for screen readers.
- Width snaps (e.g. AdeSessionsMenu, SearchBox to `w-120`, FilterBar to `w-80`) change popover
  size; content wraps where it overflowed before.
- StreamComposeMessage loses its close button; Escape and outside click close the popover.

## Out of scope

- TimePicker span labels (forms rule; Part 10 second pass), AdeEstimateField segmented control (Part 8), row and
  key-value internals in popovers (Part 6), CellGenerateMenu as a DropdownMenu (stays a popover
  list), toasts, TEMPORARY block removal (Part 9).
- Context menus built by `useContextMenuStore` (already `MenuItems`) and git `sectionsToMenuItems`
  (already data).
- Tooltip copy wording and delays (theme-owned, unchanged).
