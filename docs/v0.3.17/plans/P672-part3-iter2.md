# P672 Part 3 iter2: forms and labels, second pass

Iteration 2 of `P672 Part 10` (master loop: `plans/P672-part10.md`). Re-opens `P672 Part 3: forms and
labels` against the current tree (base `148b60e1b`: Parts 1-9, P660 Part 2, Part 2 iter2 landed).
Fresh scan; first-pass summaries not trusted. One sequential Sonnet implementer.

## Rules applied

ARCHITECTURE "UI uniformity rules (P672)", forms bullet, plus:

- RF.1 every form control has an accessible name: FieldLabel `for` its `id`, or `aria-label` where
  no visible label fits (bar, row, table cell). Placeholder is not a name.
- RF.2 one field label: FieldLabel. Bare Label only `sr-only`, or placement-only for a checkbox or
  switch in a bar, menu or list row.
- RF.3 two layouts: stacked Field, or `Field orientation="responsive"` under an `@container`.
  `orientation="horizontal"` is for checkbox and switch rows (control first) and bars only.
- RF.4 order inside Field: FieldLabel, control, FieldDescription, FieldError.
- RF.5 a group is FieldSet with FieldLegend as its first child; a legend outside a fieldset is a
  heading with no group.
- RF.6 widths w-24, w-40, w-56 only; textarea steps T1-T3; `size="kira"` only in bars and rows.
- RTa: a `choice` tabs label is Field + FieldLabel with the same text as its aria-label.
- Library: shadcn-vue RadioGroup for a radio set (never a raw `<input type=radio>`), Tailwind
  utilities, `useId()` for ids. No Pinia or TanStack Query change: no state or fetch moves.

Deliberate first-pass exceptions re-checked; see "Exemptions re-checked".

## Current state (scans at `148b60e1b`)

709 non-`components/ui/` `.vue` files in the six guard DIRS. Scratchpad parser builds one tag stack
per template (quote-aware, multi-line), queried per rule.

- `bash scripts/check-ui-primitives.sh`: green, 0 offenders. PERMANENT: 48 lines. Highest guard
  U132.
- Bare `<Label>`: 24 tags. sr-only or bar/list-row checkbox: 19. Offenders: 5 (F10).
- Field orientation: vertical 143, horizontal 79, responsive 32. Label-first horizontal: 3
  (`AdeHistoryBar.vue:23` bar, ok; `AdeWorkflowGraph.vue:297`, `RepoConfigForm.vue:137`: F4).
- FieldDescription before its control inside a Field: 6 tags (3 in RepoConfigForm: F5; 3 in
  CatalogText intro hosts with no control after, ok). After: 49.
- FieldLegend: 50 tags. Outside any FieldSet: 6 (F6). Not first child of its FieldSet: 3 (F6).
- FieldLabel without `for` while the Field holds a control: 21 (F2).
- Unnamed text controls (draft U133 regex below): 85 tags in 45 files (F1).
- Unnamed Checkbox / RadioGroup by hand read: 5 (F1).
- Raw radio inputs: 1 file (`packages/theme/src/SwatchRadio.vue`, 4 hosts; U28 exempt). Restyled
  RadioGroupItem swatches: 1 (`AdeTaskStatuses.vue:213`). F3.
- Width outliers in forms: `w-36` (AdeHistoryBar), `w-42.5` (AdeNotesEditor). F13.
- FieldSet gap overrides: 0. Form controls with `size="kira"` outside bars or rows: 0.
- Textareas: 1 script textarea on T1 (F12), 1 data list without `font-data` (F12).

## Findings

### F1. Unnamed form controls (RF.1): 90 controls in 49 files

Draft guard regex (U133, C9) matched 85 tags in 45 files:

```
<(?:Input|Textarea|NativeSelect|InputGroupInput|InputGroupTextarea|NumberStepperInput|AutocompleteField)\b(?!(?&t)\s(?::?id|:?aria-label|:?aria-labelledby|v-bind)=)
```

`id` counts as named only because F2 pairs every `id` with a FieldLabel `for`; C9 adds the
companion check. AutocompleteField, NumberStepperInput and SecretInput forward `$attrs` to the
inner input (`inheritAttrs:false`), so `id` and `aria-label` work on them.

Fix per control: inside a Field with a visible FieldLabel, `useId()` and `:for`/`:id` (F2 shape).
In a bar, table cell or list row, `:aria-label` from the visible column header or caption key;
when none exists, a new key `<module>.<surface>.<field>.label` in
`packages/theme/src/i18n/en.json` (pseudo locale regenerates). Placeholder text stays, never as the
name.

Files (count of controls):

- `apps/kira-space/frontend/src/studio/project/connection/ConnectionGeneralTab.vue` (8),
  `ConnectionAdvancedTab.vue` (2), `ConnectionMcpTab.vue` (2), `ConnectionPreconnectTab.vue` (2),
  `ConnectionMaskReview.vue` (1), `MaskRuleList.vue` (1): FieldLabel ones via F2; mask rows by
  `aria-label`.
- `apps/kira-space/frontend/src/studio/api/EnvironmentFields.vue` (2), `ImportCurlDialog.vue` (1),
  `CopyAsCurlDialog.vue` (1).
- `apps/kira-space/frontend/src/studio/kubernetes/components/ClusterMcpDialog.vue` (1).
- `apps/kira-space/frontend/src/studio/project/credentialPaste/CredentialPastePanel.vue` (2).
- `apps/kira-space/frontend/src/studio/views/console/kibana/FilterEditor.vue` (1).
- `apps/kira-space/frontend/src/studio/views/documents/DocumentFilterRow.vue` (2),
  `studio/views/grid/FilterToolbar.vue` (2), `studio/views/stream/StreamFilterRow.vue` (2),
  `studio/views/stream/StreamToolbar.vue` (1).
- `apps/kira-space/frontend/src/studio/views/grpcrequest/GrpcRequestView.vue` (2),
  `SchemaBrowser.vue` (3).
- `apps/kira-space/frontend/src/studio/views/httprequest/FormDataTable.vue` (4),
  `HttpRequestView.vue` (1), `RequestBodyPane.vue` (1).
- `apps/kira-space/frontend/src/studio/views/shared/DateTimePicker.vue` (4: date, hour, minute,
  second cells), `shared/celleditor/TimestampPane.vue` (1), `shared/fields/FieldRowsTable.vue`
  (5), `shared/keyvalue/KeyValueAddPopover.vue` (1), `KeyValueEditPopover.vue` (1),
  `shared/page/PagerControls.vue` (1).
- `apps/kira-space/frontend/src/studio/workbench/GenerateDataDialog.vue` (3).
- `apps/kira-space/frontend/src/ade/v2/review/AdeWorkflowReviewPanel.vue` (2).
- `apps/kira-space/frontend/src/configsync/ConfigSyncUnitRow.vue` (1, NativeSelect; F10 for the
  row's Label).
- `apps/kira-space/frontend/src/workbench/settings/ConfigSyncPane.vue` (1),
  `workbench/GitCredentialDialog.vue` (1).
- `apps/kira-space/frontend/src/views/repo/ReviewThread.vue` (1, Textarea: `common.comment.label`,
  as CommentComposer).
- `apps/kira-space/frontend/mobile/components/ReplySheet.vue` (1), `mobile/screens/LaterScreen.vue`
  (1), `mobile/screens/TerminalScreen.vue` (1).
- `packages/docker-ui/src/components/EditRecreateSection.vue` (7), `EditInPlaceSection.vue` (2),
  `EditSize.vue` (1, unit NativeSelect).
- `packages/workbench/src/automations/form/ParamsEditor.vue` (3), `ToolsField.vue` (1),
  `McpServerTools.vue` (1), `automations/run/RunScriptDialog.vue` (1).
- `packages/workbench/src/memory/distill/DistillView.vue` (1).
- `packages/workbench/src/components/InlineEditInput.vue` (1): single Input root, attrs fall
  through, no label prop. Add required prop `label: string` bound to `:aria-label`. Five callers
  pass it: `AdeTaskTitle.vue`, `RefsPopoverBranches.vue`, `ScriptGroupHeader.vue`, `RepoList.vue`,
  `CollectionRow.vue` (the thing being renamed: task title, branch name, collection name, repo
  nickname; reuse the existing rename or field label key).

Outside the regex (by hand):

- `apps/kira-space/frontend/src/studio/project/DataGripImportDialog.vue:229`,
  `studio/views/httprequest/ResponseHistoryList.vue:213`,
  `studio/views/shared/fields/FieldRowsTable.vue:287`: Checkbox with no label and no aria-label.
  Row-select checkboxes: `:aria-label` from the row's name (`common.action.select.label` with the
  row name if the key takes one; else a new `…select.label` key with `{name}`).
- `packages/git-ui/src/components/dialogs/CherryPickDialog.vue:139`, `RevertDialog.vue:93`:
  RadioGroup of mainline cards with a `<p>` caption above and no name. ResetDialog shape: wrap in
  `FieldSet`, caption becomes `FieldLegend`, any explanatory sentence becomes FieldDescription
  after it. Drop `class="gap-1.5"` on the RadioGroup only if FieldSet's
  `has-[>[data-slot=radio-group]]:gap-3` then doubles; RadioGroup's own item gap stays.

### F2. FieldLabel without `for` while the Field holds a control: 21 labels

Clicking the label does not focus the control and the control has no name.

- `ConnectionGeneralTab.vue` (10), `ConnectionAdvancedTab.vue` (2), `ConnectionMcpTab.vue` (2),
  `ConnectionPreconnectTab.vue` (2), `EnvironmentFields.vue` (2: name, description; colour: F3),
  `ClusterMcpDialog.vue:92`, `RunScriptDialog.vue:181`.
- Fix: `const idBase = useId()` per file, `:for="`${idBase}-<field>`"` and the same `:id` on the
  control (wrapper components forward it).
- `RunScriptDialog.vue:217`: a second FieldLabel ("Suffix", `class="mt-1 text-muted-foreground"`)
  sits in the same Field as the prompt. Split into its own Field (F8 shape); drop both classes
  (FieldLabel already muted; Field gap owns spacing).

### F3. Swatch radios: raw inputs and a restyled RadioGroupItem

`packages/theme/src/SwatchRadio.vue` is a raw `<input type="radio">` inside a Label (U28 exempt
by name). Its `aria-label` is the raw colour token (`red`), not the translated `colourLabel()`.
Four hosts hand-roll the same group: `fieldset m-0 flex h-6.5|h-control flex-wrap items-center
gap-1 border-0 p-0` plus a TextTooltip loop:

- `apps/kira-space/frontend/src/repo/RepoIdentity.vue:48`: caption is a hand `span
  text-muted-foreground`, not FieldLabel.
- `ConnectionGeneralTab.vue:146`, `EnvironmentFields.vue:41`: FieldLabel without `for`, dead
  `color-picker` class (no CSS defines it), `h-6.5`.
- `packages/workbench/src/automations/form/RunScriptTab.vue:76`: no visible label; tooltip shows
  the raw token (`swatch`), not `colourLabel`.

`apps/kira-space/frontend/src/ade/v2/workflows/statuses/AdeTaskStatuses.vue:205-226`: reka
RadioGroup with RadioGroupItem restyled `size-4 border-transparent data-[state=checked]:border-fg`
plus `STATUS_SWATCH_CLASS`, a native `:title`, and an empty `<span />` indicator.

Fix: one theme piece `SwatchPicker` (see "Theme pieces") on shadcn-vue RadioGroup. Hosts:

- RepoIdentity: `<Field><FieldLabel :id="labelId">{{ colour }}</FieldLabel><SwatchPicker
  :model-value="color" :colors="PALETTE_COLOR_CHOICES" :labelledby="labelId"
  data-testid="repo-color" @update:model-value="chooseColor" />` + FieldError. Drop the span.
- ConnectionGeneralTab, EnvironmentFields: same, FieldLabel `:id`, picker `labelledby`; keep
  `api-environment-color-picker` testid on the picker. Drop `color-picker`, `h-6.5`.
- RunScriptTab: same inside a `Field class="flex-none"` beside the collection Field, with a visible
  FieldLabel `common.field.colour.label` (the `items-end` row aligns both controls).
- AdeTaskStatuses: `<SwatchPicker :colors="STATUS_COLORS" :swatch-class="(c) =>
  STATUS_SWATCH_CLASS[c]" :color-label="statusColorLabel" testid-prefix="ade-status-color"
  :aria-label="t('common.field.colour.label')" class="grid grid-cols-8 gap-1.5" />` in the
  popover. Drops `:title` (TextTooltip per item now).
- Delete `packages/theme/src/SwatchRadio.vue`. Delete `SwatchRadio.vue` from U28's exempt field in
  the same commit (it goes stale: the run fails otherwise).

### F4. Label-first horizontal Fields (RF.3)

- `apps/kira-space/frontend/src/repo/RepoConfigForm.vue:137` timeout: `Field
  orientation="horizontal"`, FieldLabel then Input `w-24`. Becomes stacked `<Field>`. First-pass
  plan listed it; the landing missed it.
- `apps/kira-space/frontend/src/ade/v2/workflows/AdeWorkflowGraph.vue:297` name: horizontal Field
  `w-auto`, label first. Becomes `Field orientation="responsive"`; add `@container` to the nearest
  wrapper that spans the form width; drop the single-child `flex flex-wrap` div around it.

### F5. FieldDescription before the control (RF.4)

`RepoConfigForm.vue` 114 (prepare), 156 (base path), 189 (targets): move each FieldDescription
after its control, before FieldError. Spec `repo-config-form.spec.ts` reads by testid; order only.

### F6. Legends outside a fieldset or not first child (RF.5)

FieldLegend outside any FieldSet (renders as a stray `<legend>`; no group name):

- `apps/kira-space/frontend/src/workbench/settings/AboutPane.vue:23`
- `apps/kira-space/frontend/src/workbench/settings/AppearancePane.vue:45` (also `pt-0`)
- `apps/kira-space/frontend/src/studio/workbench/settings/StudioMcpConnectionsPane.vue:46`
- `apps/kira-space/frontend/src/studio/workbench/settings/ApiAccessRulesSection.vue:69`
- `apps/kira-space/frontend/src/studio/workbench/settings/KubeMcpSection.vue:34`
- `apps/kira-space/frontend/src/studio/workbench/settings/ScriptsMcpSection.vue:27`

Fix: wrap the legend and its section content in `<FieldSet>`; keep the section's testid on the
FieldSet (or where it is). A section component whose root is now FieldSet keeps one root.

FieldLegend inside an action row div, so not the fieldset's first child (fieldset unnamed):

- `apps/kira-space/frontend/src/workbench/settings/UpdatesPane.vue:40-50` (reset
  TooltipIconButton)
- `packages/workbench/src/automations/form/ParamsEditor.vue:145` (`class="mb-0"`, text Button)
- `packages/workbench/src/automations/form/ToolsField.vue:64-75` (`class="mb-0"`, text Button
  `ml-auto`)

Fix: FieldLegend `#actions` slot (Theme pieces). Each host: `<FieldLegend>{{ title }}<template
#actions>…</template></FieldLegend>` as the first child of FieldSet; drop the wrapper div, `mb-0`
and the Button's `ml-auto`. Drop `pt-0` in AppearancePane and `GitPane.vue:99` (primitive `pt-1`
is the one legend offset; GitPane's legend is already first child).

### F7. Label-with-reset head row outliers

Convention (24 sites): `div flex items-center justify-between gap-1` + FieldLabel + `TooltipIconButton
icon="discard" :label="t('common.action.resetToDefault.label')"`. Outliers:

- `packages/workbench/src/llmPrompts/PromptArgumentField.vue:36`: `gap-2` and a text `Button
  variant="toolbar" size="status"`. Becomes the convention; keep `llm-prompt-reset`.
- `apps/kira-space/frontend/src/ade/v2/run/AdeRunDialog.vue:243-252`: hand `div flex flex-col
  gap-1` instead of Field, FieldLabel `flex-1`, text toolbar Button. Becomes `<Field>` with the
  convention head row; FieldLabel `for` the Textarea `id`; keep `ade-run-reset`.
- `apps/kira-space/frontend/src/ade/v2/dialog/AdeAgentDialog.vue:258-268`: same, Button also
  `class="text-muted-foreground"`. Same fix; keep `ade-dialog-reset`.

### F8. Prompt suffix shown three ways

Read-only suffix appended to an editable prompt:

- `AdeRunDialog.vue:241` and `:272`: FieldDescription (`ade-run-suffix`).
- `AdeAgentDialog.vue`: muted CodeBlock, no label.
- `RunScriptDialog.vue:217`: FieldLabel "Suffix" + muted CodeBlock.

Fix: one shape: own `<Field>` with FieldLabel (existing suffix label key from RunScriptDialog) and
the muted CodeBlock. Keep `ade-run-suffix` on the CodeBlock in AdeRunDialog.

### F9. Hand Field rows in ADE dialogs

- `apps/kira-space/frontend/src/ade/v2/stage/AdeStageDialog.vue:99-117`: `div flex` with FieldLabel
  `ml-auto` and NativeSelect. Becomes `Field orientation="responsive"` (label then select), host
  wrapper gets `@container`; drop `ml-auto`.
- `AdeRunDialog.vue:186-199` per-repo branch name rows: AdeRepoTag + FieldLabel
  `whitespace-nowrap` + Input in a `flex items-center gap-3` div. Becomes `Field
  orientation="responsive"` per row: FieldLabel holds the AdeRepoTag and the label text, Input
  `:id`. Wrapper keeps `ade-run-branches`, gets `@container`.
- `AdeAgentDialog.vue:224-242` targets: span caption plus `choice` SecondaryTabs with
  `aria-label="tg.branch"`. RTa: Field + FieldLabel with the branch name (`font-data`), the tabs
  keep `:aria-label="tg.branch"`.

### F10. Bare Label offenders (RF.2)

- `apps/kira-space/frontend/src/configsync/ConfigSyncUnitRow.vue:97`: Label `text-kira-sm
  text-muted-foreground` before its control in a list row. Control first; Label placement-only.
- `apps/kira-space/frontend/src/workbench/settings/MobileAccessPane.vue:335`, `:345`: label-first
  Switch rows wrapped in `Label class="flex items-center gap-1"`. Become `Field
  orientation="horizontal"` + Switch `:id` + FieldContent/FieldLabel `:for` (settings pane, not a
  bar).
- `packages/workbench/src/memory/AddMemoryDialog.vue:162`: bare Label as a caption with no control.
  Becomes FieldLabel if it names the list below (give the list `aria-labelledby`), else
  SectionHeading. Implementer reads the block and picks; record the choice in the commit body.
- `packages/docker-ui/src/components/EditRecreateSection.vue:342`: span wraps Checkbox plus "ro"
  text. Becomes `<Label>` placement-only around Checkbox and text (list row).

### F11. Studio MCP labels with inline notes

FieldLabel holding `<span class="text-subtle">— note</span>`:
`ClusterMcpDialog.vue:94`, `ConnectionMcpTables.vue:95`, `ConnectionMcpTab.vue:201`, `:279`,
`ConnectionMcpReview.vue:72`. Fix: note leaves the label and becomes FieldDescription after the
control (drop the dash).

- `ConnectionMcpReview.vue`: per-mode FieldDescription captions above `choice` SecondaryTabs become
  Field + FieldLabel (RTa); the outer group becomes FieldSet + FieldLegend.
- `ConnectionMcpTables.vue`: outer Field that holds a nested Field becomes FieldSet + FieldLegend.

### F12. Textareas

- `ConnectionPreconnectTab.vue:55` preconnect command (a script): T1 to T3 `min-h-32 max-h-90
  resize-y font-data`.
- `packages/git-ui/src/components/dialogs/RepoSettingsDialog.vue:306` base-candidates list (data):
  add `font-data`, as the GitPane protected-branches list.

### F13. Widths

- `apps/kira-space/frontend/src/ade/v2/plan/AdeHistoryBar.vue:28` date Input `w-36` to `w-40`.
- `apps/kira-space/frontend/src/ade/v2/notes/AdeNotesEditor.vue:256` link Input `w-42.5` to
  `w-40`.

### F14. Kibana SortControl captions

`apps/kira-space/frontend/src/studio/views/console/kibana/SortControl.vue:43-84` (bar): span
captions plus `aria-label` duplicate. Becomes two `Field orientation="horizontal" class="w-auto"`
with `FieldLabel :for` and NativeSelect `:id`, as the AdeHistoryBar sibling. Drop the duplicate
`aria-label`. Sample size Field takes the `ml-auto`.

### Exemptions re-checked

- Bare Labels kept (19): sr-only, or placement-only Checkbox/Switch in bar, menu or list row
  (FiltersDialog, DataGripImportDialog rows, AdeReposPage, AdeStepResults, AdeNotesEditor sr-only,
  ScopeField and ImportConfirmDialog captions as sr-only). Requirement: no visible field context.
- `size="kira"` controls (SchemaBrowser, DateTimePicker, TimestampPane, PagerControls,
  FileTreeToolbar, SearchBox, InlineEditInput, AdeNotesEditor): bars and rows. Keep.
- Widths kept: DateTimePicker `w-14` (two-digit time cells; w-24 doubles the picker),
  AdeCaptureBox `w-full min-w-32 max-w-72 @5xl:w-72` (shrinks in the title bar), LogsView
  SearchField `w-48` and ResourceBrowser `min-w-32` (bar search, not a form). Requirement named.
- U28 `SwatchRadio.vue`: removed with F3.
- FieldDescription `class="w-full"` (about 40, Studio connection tabs): parents are template (17),
  Field (10), div (7) and others; inert in flex-col. Cleanup lead, not a rule break (Handoff).

## Theme pieces

- `packages/theme/src/SwatchPicker.vue` (`<script setup lang="ts">`): shadcn-vue `RadioGroup` +
  `RadioGroupItem`, one TextTooltip per item.
  - `defineModel<string>({ required: true })`.
  - Props: `colors: readonly string[]`; `swatchClass?: (c: string) => string` (default
    `connSwatchClass`); `colorLabel?: (c: string) => string` (default `colourLabel`, `none` to
    `common.field.noColour.label`); `testidPrefix?: string` (default `color`); `labelledby?:
    string`.
  - Root: `RadioGroup` `class="flex flex-wrap items-center gap-1"` merged with host class (cn),
    `orientation="horizontal"`, `:aria-labelledby="labelledby"`; host `aria-label` falls through.
  - Item: `RadioGroupItem :value="c" :aria-label="colorLabel(c)" :data-testid="`${testidPrefix}-${c}`"`
    with `size-4 border-transparent data-[state=checked]:border-transparent
    data-[state=checked]:outline-2 outline-offset-2 outline-fg` plus `swatchClass(c)`; slot an
    empty `<span />` so no dot draws. Tooltip text `colorLabel(c)`.
  - `connColor.ts`: add `CONN_SWATCH` map spelled in full (`bg-conn-red
    data-[state=checked]:bg-conn-red`, …; `none` to `swatch-none data-[state=checked]:bg-transparent`)
    and `export function connSwatchClass(c)`. Tailwind only emits scanned literals.
- `FieldLegend.vue` `#actions` slot: when `$slots.actions`, legend gets `flex w-full items-center
  justify-between gap-1` and renders `<span>default</span><span class="flex items-center gap-1
  normal-case tracking-normal">actions</span>`. Mirrors SectionHeading's action slot. No change
  without the slot.

## Stream decision

One sequential implementer. Theme pieces (SwatchPicker, FieldLegend, connColor), `en.json`, the
guard script and ARCHITECTURE are shared by most findings, and F1/F2 touch the same Studio
connection files as F3/F11. Overlap is semantic, not additive hunks.

## Commits

Each commit passes the pre-commit hook (lint incl. guard script and N8-N12, typecheck, build).

C1 `feat(theme): SwatchPicker on RadioGroup, FieldLegend actions slot`

- SwatchPicker, `connSwatchClass`, FieldLegend `#actions` as in "Theme pieces". No host yet.

C2 `refactor(ui): colour pickers on SwatchPicker` (F3)

- Four SwatchRadio hosts and AdeTaskStatuses. Delete `SwatchRadio.vue`; delete `SwatchRadio.vue`
  from U28's exempt field (otherwise the stale-exempt check fails). U28 exempt becomes empty.

C3 `refactor(ui): legends first in their fieldset` (F6)

- Six FieldSet wraps, three `#actions` conversions, `pt-0`/`mb-0` drops.

C4 `refactor(ui): field order and layout fixes in repo and ADE forms` (F4, F5, F7, F8, F9, F13)

- RepoConfigForm, AdeWorkflowGraph, PromptArgumentField, AdeRunDialog, AdeAgentDialog,
  AdeStageDialog, RunScriptDialog (suffix half of F2 and F8), AdeHistoryBar, AdeNotesEditor.
- Watch N10: AdeRunDialog and AdeAgentDialog near the 600-line cap; check `wc -l` after; split a
  child component if over.

C5 `refactor(studio): MCP labels, notes and groups` (F11, F14)

C6 `fix(a11y): name every form control` (F1, F2; may split per package, each split passes the hook)

- `useId()` pairs, `aria-label`s, InlineEditInput `label` prop with its five callers, F1 by-hand
  Checkbox and RadioGroup sites (CherryPick, Revert on FieldSet + FieldLegend).
- New `en.json` keys only where no existing key fits; list them in the commit body.

C7 `refactor(ui): bare Label offenders and textarea steps` (F10, F12)

C8 Any PERMANENT line or built-in exempt the script reports stale after C2-C7: delete in the
commit that made it stale, note it in the body. Expected: only U28 `SwatchRadio.vue` (C2).

C9 `chore(lint): U133 unnamed form control guard`

- Header line: `# Guard U133: docs/v0.3.17/plans/P672-part3-iter2.md.`
- `U133@@v@@@@form control without id, aria-label or aria-labelledby (RF.1; FieldLabel for/id or
  aria-label)@@<regex above>`.
- Dry run on base `148b60e1b`: 85 tags in 45 files. After C6: 0. Record both in the commit body.
- Same commit: second U133 line for FieldLabel without `for` in a Field that holds a control only
  if a single-tag regex can express it; the tag-stack check cannot run in `grep -P`, so if not,
  skip it and say so in the commit body (F2 rests on review).

C10 Specs: selector updates only where a change broke a spec (see Test split). One commit
`test(ui): follow form label changes`.

C11 `docs: forms rules for swatches, legends, named controls`

- ARCHITECTURE forms bullet (line 1458): add "Every control has a name: FieldLabel `for` its id, or
  aria-label in bars, rows and cells (U133). A colour choice is SwatchPicker. FieldLegend is the
  first child of its FieldSet; legend actions go in its `#actions` slot. FieldDescription follows
  the control."
- Guard ranges: line 1436 `U83-U132` to `U83-U133`, line 1471 `U110-U132` to `U110-U133`.

C12 `docs: P672 Part 10 progress, Part 3 iter2` (last)

- SPEC Part 10 section, after the Part 2 iter2 line: `Part 3 iter2: <result>, plan
  plans/P672-part3-iter2.md.` Result names counts: controls named, FieldLabel pairs, swatch hosts,
  legends fixed, guard U133 offenders before/after.

## Test split

UI only. No backend change, so no flow test. No new unit test (no complex logic).

- Run specs touching changed surfaces or testids (grep `apps/kira-space/tests/ui` and
  `tests/mobile` for each changed testid first):
  - Swatches: `consistency.spec.ts:216` (hover `repo-color color-blue`), `ade-repos.spec.ts:260`
    (`.check({ force: true })` on `color-red`; RadioGroupItem is `role=radio`, check works),
    `ade-v2-task-status.spec.ts:150-152` (16 `[role=radio]` items, click
    `ade-status-color-purple`), `studio/api-variables-form.spec.ts`,
    `studio/connection-form.spec.ts`, `automations-form.spec.ts`, `automations-scripts.spec.ts`.
  - Legends: `settings-about.spec.ts`, `settings-updates.spec.ts`, `studio/studio-mcp-settings.spec.ts`,
    `studio/dbmcp-api-rules.spec.ts`, `studio/kubernetes-mcp.spec.ts`, `automations-smart.spec.ts`,
    `automations-mcp.spec.ts`.
  - Repo and ADE: `repo-config-form.spec.ts`, `repo-config-file.spec.ts`, `ade-v2-run.spec.ts`,
    `ade-v2-dialogs.spec.ts`, `ade-v2-stage-move.spec.ts`, `ade-v2-stage-settings.spec.ts`,
    `ade-v2-workflows.spec.ts`, `ade-v2-workflow-steps.spec.ts`, `ade-v2-notes.spec.ts`,
    `settings-llm-prompts.spec.ts`, `prompts.spec.ts`.
  - Studio: `studio/connection-mcp-*.spec.ts`, `studio/preconnect.spec.ts`,
    `studio/connection-mask-review.spec.ts`, `studio/kibana-discover.spec.ts`,
    `studio/docker-edit.spec.ts`, `studio/grpc-request.spec.ts`, `studio/http-curl.spec.ts`.
  - Other: `config-sync.spec.ts`, `config-sync-repos.spec.ts`, `mobile-access.spec.ts`,
    `git-credential-relay.spec.ts`, `memory-distill.spec.ts`, `ade-v2-branch-rename.spec.ts`,
    `git-choice-cards.spec.ts` and `copy-coverage-git.spec.ts` (cherry-pick, revert).
  - Mobile: `bun run build:mobile` first, then `tests/mobile/writes.spec.ts`,
    `tests/mobile/terminal.spec.ts`, `tests/mobile/consistency.spec.ts`.
- Visual baselines re-record in PR CI (swatch outline ring, legend action rows, MCP notes as
  descriptions, RepoConfigForm timeout stacked).

Test lessons (carry into every spec run and edit):

- Monaco: hover the menu trigger first before clicking a context menu item.
- Reka radio and toggle groups: arrow keys need `delay: 40`, through `tests/ui/support/radioKeys.ts`
  (SwatchPicker is now a reka radio group; any spec walking it by keyboard uses the helper).
- N17: keep the conventions marker on files that carry one.
- N10: `.vue` cap 600 lines; split a file a change pushes over.
- N8: new spec helpers go in shared support files; no duplicate helper.
- Stale U91 exemptions: a PERMANENT line a change makes stale goes in that change's commit.
- A timeout under load gets one lone rerun before any fix.
- `bun run build:mobile` before mobile specs.

## Risks

- `getByLabel` strict mode: a new aria-label equal to an existing visible label on the same page
  makes `getByLabel(text)` match twice. Grep specs for `getByLabel(` on touched surfaces; prefer
  the FieldLabel pair over a duplicate aria-label.
- FieldLegend `#actions`: action names join the fieldset's accessible name (button content is
  valid legend content; legend must stay the first child). A spec using `getByRole('group', {
  name, exact: true })` on UpdatesPane, ParamsEditor or ToolsField breaks; grep first.
- SwatchPicker checked colour: RadioGroupItem sets `data-[state=checked]:bg-primary`; every swatch
  class must carry its own checked variant or the checked swatch turns primary. `connSwatchClass`
  and `STATUS_SWATCH_CLASS` both do.
- Radio semantics change from native inputs to `role=radio` buttons: `ade-repos` `.check()` must
  pass; if it does not, use `.click()` and say so in C10.
- FieldSet wraps in settings sections: FieldSet `gap-4` may change spacing inside a section that
  used a different gap; check each of the six in the dev build.
- `@container` added for responsive Fields: the host width decides the column; check
  AdeStageDialog and AdeRunDialog at `size="lg"`.

## Out of scope

- FieldError used as a pane load failure (Part 9 lead below).
- New Field variants or primitives beyond SwatchPicker and FieldLegend `#actions`.
- Row heights through rowVariants (`h-6.5` rows in FiltersDialog, DataGripImportDialog).
- FieldDescription `w-full` cleanup (inert).
- Items in Handoff.

## Handoff (leads for iterations 3..8; each planner re-verifies)

- Iteration 3, Part 4 iter2 (menus, popovers): AdeTaskStatuses colour popover now hosts
  SwatchPicker; check its PopoverContent width (`w-auto` for one control is allowed).
- Iteration 4, Part 5 iter2 (spacing, popovers): ForcePushDialog Disclosure `mt-3` wrappers.
  FieldDescription `class="w-full"` (about 40 in Studio connection tabs) inert.
- Iteration 5, Part 6 iter2 (cards, sections): AdeRunDialog hand captions "handover" and "task
  notes" as `span text-muted-foreground` over a CodeBlock; same pattern in AdeAgentDialog
  `ade-dialog-base` row (`span text-muted-foreground` + BranchPicker).
- Iteration 6, Part 7 iter2 (text roles): FieldDescription as generic padded status text in
  AdeAddPopover, HostViewConversation, CaptureWindow footer.
- Iteration 7, Part 8 iter2 (tabs, choices): ConnectionMcpReview `choice` tabs after F11; recheck
  RTa across Studio.
- Iteration 8, Part 9 iter2 (states): FieldError as pane load failure with `p-3` in git-ui
  DetailPane:72, StashDetailPane:69, WorkingDetailPane:60, ReviewCommentsPane:134,
  ReviewFilesPane:182/227/233, ReviewCommitRow:249, ErrorPopover, TerminalHostView footer,
  ResourceBrowser, PickerCombobox: should be Alert error or InlineState. FiltersDialog and
  DataGripImportDialog rows `h-6.5` (row height through rowVariants).
