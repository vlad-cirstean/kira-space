# P672 Part 3 UI uniformity: forms and labels

Spec row: `../SPEC.md` P672 Part 3. Rules RF.1-RF.8, migration items M4-M9 (`P672-inventory.md`
§4.2, §20, appendix A.4, A.6, A.9, A.13). Base: `chapter/v0.3.17` at `077ca783e`. Inventory line
numbers are at `6527aec4`; every fact below was re-checked at `077ca783e` via `codegraph_explore`
and template scans. Line numbers are hints: implementer greps the named pattern before editing.

UI-only phase. Look and layout change; behaviour, i18n keys and every `data-testid` stay.

## Rules applied (recap)

- RF.1 one visible label: `FieldLabel` (with `for` when a control exists). Bare `Label` only
  `sr-only`, or for a checkbox/switch inside a bar, menu or list row.
- RF.2 two layouts: stacked (`Field`, default vertical) or columns (`Field orientation="responsive"`,
  needs an `@container` ancestor). No hand grids, no `<dl>` form rows.
- RF.3 checkbox and switch rows: `Field orientation="horizontal"`, control first, then
  `FieldContent(FieldLabel, FieldDescription)`.
- RF.4 `FieldSet` keeps its own gap; no `gap-*` override.
- RF.5 fixed widths only `w-24`, `w-40`, `w-56`; else `flex-1`/full. No `flex-2`, no `basis-*`.
- RF.6 textarea steps T1/T2/T3 (below).
- RF.7 field error is `FieldError`; panel-level failure is `Alert variant="error"`.
- RF.8 `Checkbox` takes no `size-*` class and no icon child; primitive draws the indicator.
- Control size: form controls take the primitive default (`kira-lg`); bars and rows keep `kira`.

## Shapes (copy exactly)

Columns row:

```vue
<Field orientation="responsive">
  <FieldLabel :for="id">{{ label }}</FieldLabel>
  <Input :id="id" ... />
  <FieldDescription v-if="hint">{{ hint }}</FieldDescription>
  <FieldError v-if="error">{{ error }}</FieldError>
</Field>
```

Responsive variant puts every non-label child at `col-start-2`; no wrapper div.

Checkbox/switch row (WordWrapField shape):

```vue
<Field orientation="horizontal">
  <Switch :id="id" ... />
  <FieldContent>
    <FieldLabel :for="id">{{ label }}</FieldLabel>
    <FieldDescription v-if="hint">{{ hint }}</FieldDescription>
  </FieldContent>
  <TooltipIconButton v-if="resettable" class="ml-auto" ... />
</Field>
```

Ids: `const id = useId()` (Vue 3.5) per row, or a stable string already present. Never two
controls with one id.

Textarea steps:

- T1 short text: primitive default (`field-sizing-content min-h-16`). Add only `font-data` for data.
  Drop `rows`, `resize-*`, `field-sizing-*`, `w-full`, `leading-normal`, `min-h-*`.
- T2 prompt/body: `class="min-h-24 max-h-90 resize-y"`.
- T3 code/script: `class="min-h-32 max-h-90 resize-y font-data"`.

`rows` is inert under `field-sizing-content`; drop it everywhere except exceptions below.

## Stream decision

One sequential Sonnet implementer. Reasons: commit 1 changes theme `Checkbox` and `Field`
horizontal variant, which every later commit depends on; `scripts/check-ui-primitives.sh`
TEMPORARY block and `docs/ARCHITECTURE.md` are shared by every group; label and width edits hit
the same templates (dialogs, settings panes) as the wrapper retirements. No split.

## Commits

One commit per group, in order. Each commit: `bun run typecheck` (or package typecheck), lint,
`bash scripts/check-ui-primitives.sh` clean, pre-commit hook clean without `--no-verify`.

### C1 `feat(theme): checkbox indeterminate glyph, horizontal field top-align`

- `packages/theme/src/components/ui/checkbox/Checkbox.vue`: default slot content becomes
  `<MinusIcon v-if="slotProps.state === 'indeterminate'" /><CheckIcon v-else />` (`MinusIcon` from
  `@lucide/vue`). Slot still `v-bind="slotProps"`; FileTree and FiltersDialog read `state`.
- `packages/theme/src/components/ui/field/index.ts`: `horizontal` variant appends
  `has-[>[data-slot=field-content]]:items-start`.
- `packages/workbench/src/settings/fields/WordWrapField.vue` (path: grep `WordWrapField.vue`):
  drop `class="items-start"` on its `Field`.

### C2 `refactor(ui): checkbox primitive indicator (U110)`

Per file: remove `size-*` from `<Checkbox class>` and any `CodiconIcon` child inside `<Checkbox>`.
Keep `after:inset-0`, conditional `border-warn` and any layout class (`flex-none`, `mt-*`).
Delete the file's U110 line in the TEMPORARY block in the same commit.

18 files (U110 lines 153-170 of `scripts/check-ui-primitives.sh`), Studio CheckboxRow excluded
(C3):

- `apps/kira-space/frontend/src/studio/api/ApiVariablesTable.vue`
- `.../studio/**/ReviewResultOption.vue`
- `.../studio/**/DataGripImportDialog.vue`
- `.../studio/**/FiltersDialog.vue` (keep its custom slot reading `state`; drop size/icon only)
- `.../studio/project/connection/ConnectionMcpTables.vue`
- `.../studio/**/ProjectionMenu.vue`, `ColumnsMenu.vue`
- `.../studio/api/RequestSettingsPane.vue`, `ResponseHistoryList.vue`
- `.../studio/**/FieldRowsTable.vue`, `StreamFilterRow.vue`
- `packages/git-ui/src/**/FileTree.vue` (keeps custom slot; drop size/icon only)
- `packages/workbench/src/**/ParamsEditor.vue`, `ToolsField.vue`, `ParamsForm.vue`,
  `DistillResult.vue`, `ImportConfirmDialog.vue`

Exact paths: the U110 baseline lines name them. After C3 the block holds no U110 line.

### C3 `refactor(studio): retire CheckboxRow`

- 9 call sites become checkbox-row shape: `ConnectionAdvancedTab.vue` (2), `ConnectionMaskReview.vue`
  (1), `ConnectionMcpTab.vue` (2), `ConnectionMcpTables.vue` (1), `ConnectionPreconnectTab.vue` (1),
  `MaskRuleRow.vue` (2, keep `flex-none` on the `Field`). Props map: `label` to `FieldLabel`,
  `description` to `FieldDescription`, `v-model`/`testid` onto `Checkbox`.
- Delete `apps/kira-space/frontend/src/studio/project/connection/CheckboxRow.vue` and its U110 line.
- Grep `CheckboxRow` afterwards: zero hits outside git history.

### C4 `refactor(ade): retire AdeInspectorRow`

- 24 tags in 12 files under `apps/kira-space/frontend/src/ade/v2/workflows/inspector/` and
  `workflows/` (grep `<AdeInspectorRow`): AdeStepSettings, AdeStepStatus, AdeStepReviewFields (2),
  AdeStepScriptFields (stack), AdeStepAgentPick, AdeStepManualFields (3, one stack),
  AdeStepEndFields, AdeStepRunFields, AdeStepAgentFields, AdeStepSandbox, AdeStepResults,
  AdeStepAdvanced, AdeStepInspector (extra prompt, stack).
- Each becomes columns-row shape (`stack` becomes plain `<Field>`): `:label`/`:for` to
  `<FieldLabel :for>`, default slot inline, `#hint` to `FieldDescription`, `#error` to `FieldError`.
  Drop the old `text-kira-sm text-muted-foreground` label classes (FieldLabel owns colour).
- `@container` ancestor already at `AdeStepInspector.vue` (~229); confirm, add none.
- Delete `AdeInspectorRow.vue`. Grep: zero hits.

### C5 `refactor(ade): task panel fields onto Field columns`

- `apps/kira-space/frontend/src/ade/v2/panel/AdeMainTab.vue`:
  - replace `<dl class="m-0 flex flex-col">` with
    `<div class="@container flex flex-col gap-2 px-1.5" data-testid="ade-main-fields">`;
  - each `AdeRow term :label` becomes `Field orientation="responsive"` + `FieldLabel` (`for` where a
    control exists: estimate `ade-est-num`, parked `ade-task-parked`, orchestrator
    `ade-task-orchestrator`) + a `div class="flex min-w-0 items-center gap-1.5"` holding the
    existing control, chip and meta span (one grid cell);
  - branch-request row: `Button` stays in that cell, `size="kira"` stays (row action);
  - parked and orchestrator: `Label` in `#label` becomes `FieldLabel`; drop `Label` import;
  - keep `data-testid` on the `Field` (`ade-task-branch-request-row`, `ade-task-parent-row`,
    `ade-task-orchestrator-row`); `AdeRow`/`AdeRowDetail` imports drop if unused (error `Alert`
    sits as a plain child, no `AdeRowDetail`).
- `.../panel/AdeWorkflowBlock.vue` (~43): Workflow `AdeRow` becomes `Field orientation="responsive"`
  + `FieldLabel for` + `NativeSelect` (drop `size="kira"`, M9). Root gets `@container` if no
  ancestor has it (`AdeTaskPanel` has none; put `@container` on the block's own root).
- `.../panel/AdeMainBranches.vue` (~69): "Branches" label row becomes
  `<SectionHeading :label="...">` with the actions in its default slot.
- `.../panel/AdeRow.vue`: remove props `label`, `labelFor`, `term`, `#label` slot and dt/dd
  rendering; it stays a plain row for the remaining callers. `.../panel/adeRow.ts`: delete
  `ADE_ROW_LABEL`. Grep `ADE_ROW_LABEL|label-for=|<AdeRow[^>]*\bterm\b`: zero hits.
- `.../panel/AdeRowDetail.vue`: drop unused `labelled` prop (grep callers first).
- `.../panel/AdeEstimateField.vue`: `Input size="kira"` to default, `w-20` to `w-24`.
- `.../panel/AdeStatusName.vue`: `Input size="kira"` to default.
- `scripts/check-ui-primitives.sh`: delete U113 line for AdeMainTab (~227).

### C6 `refactor(docker-ui): EditField onto Field columns`

- `packages/docker-ui/src/components/EditField.vue`:
  - root `<fieldset ...>` becomes `<Field orientation="responsive" role="group" :aria-label>`
    keeping `:data-testid="docker-edit-field-${field}"` and `:data-changed`;
  - label `<span>` becomes `<FieldLabel :for="id">`, StatusDot+Tooltip stay inside it;
  - `const id = useId()`; slot props become `{ invalid, id }`;
  - FieldError (keep `docker-edit-field-error`), recreate hint and description stay as direct
    children (col 2).
- `EditInPlaceSection.vue`, `EditRecreateSection.vue`: single-control slots bind `:id="id"`;
  section roots `grid grid-cols-1 gap-3 @2xl:grid-cols-2` become `flex flex-col gap-3`.
  Widths (RF.5): EditRecreateSection Input `w-36` to `w-40`, NativeSelect `w-20` to `w-24`;
  EditInPlaceSection NativeSelect `w-48` to `w-40`; `EditSize.vue` Input `w-28` to `w-24`,
  NativeSelect `w-20` to `w-24`.
- `@container` already on `ContainerEditView` root; confirm.

### C7 labels per module (M4), one commit per module

Scan: every `<Label` in non-primitive `.vue` (148 uses, 80 files at base). Categories:

- (a) visible label beside a form control: `FieldLabel` with `for`; drop `text-kira-*`,
  `text-muted-foreground`, `font-*` classes; wrap in `Field` if not already.
- (b) `sr-only` labels: stay `Label`. AdeNotesEditor (~250), AdeCaptureBox (~39), AdeStepInspector
  (~209), AdeWorkflowsTab (~124), AdeStepResults (~53, ~75, ~104), RepoEnvRow (~35).
- (c) checkbox/switch in a bar, menu or list row: stays `Label`, placement classes only (`flex`,
  `gap-*`, `items-center`, `min-w-0`, `truncate`, `cursor-*`); drop type/colour classes except
  row-content `font-normal`/`font-data` (DistillResult ~122, DetailsEditBody ~47,
  BranchRequestBody ~142). Files: ProjectionMenu, ColumnsMenu, StreamFilterRow,
  ConnectionMcpTables (~169), FiltersDialog rows, DockerPanel (~157), MemoryPanel (~155),
  ConfigSyncUnitRow (~100), AdeReposPage (~462), MobileAccessPane (~329, ~339),
  ReviewResultOption, DistillResult (~108).
- (d) `Label` wrapping a checkbox/switch in a form: checkbox-row shape. AdeAgentDialog (~211,
  ~251), AdePortExportDialog (~177), RequestSettingsPane inherit toggles (7; delete
  `INHERIT_LABEL` constant), BranchRequestBody (~194), ApiAccessRuleDialog, McpServerTools.
- (e) `Label` wrapping a text control: `Field` + `FieldLabel :for` + control `:id`.
  StreamComposeMessage (3; `Input size="kira"` to default).
- (f) caption with no control: `FieldLabel` without `for`. ScopeField, ImportConfirmDialog origin,
  AddMemoryDialog (~162), AdeWorkflowReviewPanel (~266). Mobile PairScreen label to `FieldLabel`.

Stacked settings switches (label above switch) become checkbox-row shape: AdvancedPane (~82),
AgentPane (~130, ~153, ~176, ~215), CapturePane (~56), ExperimentalPane (~29), FileTypesPane
(~108), MobileAccessPane (~165, ~249), NotifyPromptsField (~29), ScheduleFields (~181),
UpdatesPane (~49, label-first horizontal: swap to control-first).

Other (a) sites: AdeAddPopover (hand grid to `Field` columns, root `@container`; delete its U113
line ~224), AdeHistoryBar form fields, AdeStageDialog, CachePane, SaveRequestDialog,
UploadObjectDialog, GenerateDataDialog (two stacked `Field` in `FieldGroup`; drop
`group-class="h-control-lg"`, default), FileTreeToolbar form field, CredentialPastePanel,
AdeTaskStatuses (~265 `Field` responsive, hint to `FieldDescription`; ~309), RepoConfigForm
(~137 horizontal label+Input to stacked `Field`).

Commit order inside C7 (each `refactor(<module>): FieldLabel ...`): ADE, Studio, Settings,
Runs/automations (workbench), Memory, Git/Docker/Kube/mobile. Run the guard script per commit.

U117 trap: wrapping a segmented `SecondaryTabs` in `Field`/`FieldSet`/`DialogBody` in a file not in
the U117 baseline makes a new offender. ScopeField: switch its `SecondaryTabs` to
`variant="choice"`. Any other hit: keep the tabs outside the `Field` and report it in Result.

After C7: grep `<Label` hits only (b) and (c) sites plus theme primitives and SwatchRadio.

### C8 `refactor(ui): fieldset gaps, fixed widths, textarea steps (M7)`

FieldSet gap overrides removed: RepoConfigForm (~92, ~110, ~185, ~208), RepoIdentity (~45),
GitPane (~98, ~180), MobileAccessPane (~188, ~239, ~291), UpdatesPane (~29).

Widths not already done in C5/C6:

- AdeStepResults NativeSelect `w-36` to `w-40`.
- RepoConfigForm `max-w-60` to `w-56`, `w-20` to `w-24`.
- RepoEnvRow `w-48` to `w-40`.
- RunMcpTab, ScheduleFields timeout `w-32` to `w-24`.
- SmartSettingsFields `Field` `w-32`/`w-28` to `w-24`.
- ConnectionGeneralTab `Field` `flex-2` to `flex-1`; `flex-none basis-24` to `flex-none w-24`.
- RepoSettingsDialog `group-class="h-control w-32"` to `group-class="w-24"`.
- AdeWorkflowGraph name input `w-72`: header form field, not a bar; becomes `w-56`.
- Excluded (bars, not forms): DateTimePicker, AdeCaptureBox, AdeHistoryBar search, ResourceBrowser,
  AdeNotesEditor, CheetahProto (prototype).

Textareas (37 sites; grep `<Textarea`):

- T2: AdeAgentDialog, AdeRunDialog, AdeStepInspector extra prompt, AdeStepAgentFields,
  AdeStepManualFields, AdeWorkflowReviewPanel finish note, RunMcpTab description.
- T3: AdeStepSandbox (2), RepoScriptField (drop `rows` prop and its pass-through; callers
  RepoConfigForm, RepoEnvRow stop passing it), RunScriptDialog, ImportCurlDialog (drop
  `curl-textarea` class: no CSS references it), CopyAsCurlDialog, StreamComposeMessage body,
  DistillView.
- T1: every other site.
- Exceptions kept as is: AdeReviewSubmitDialog (fills fixed-height dialog), mobile ReplySheet and
  PlanTaskCard (touch scale).

### C9 `refactor(ui): field errors via FieldError and Alert (M8)`

- AdePortImportDialog (~170) `s.problem` span to `FieldError`.
- ConfigSyncUnitRow (~79) problem to `FieldError`, keep testid.
- RunOutcomeBlock (~83, ~84) copyError/resumeError to `Alert variant="error"`
  (`AlertDescription`), keep testids.

Not converted (other Parts or not error text): destructive buttons and menu items (AdeStepResults
~97, AdeStepNode ~228, QueryBar ~98: Parts 4/5), code blobs (AdeRunOutcome CodeBlock,
RunOutcomeBlock ~70/~77: Part 6), row/status tones (TreeRow, StackList ~180, DataGripImportDialog
~323: Part 7), data tones (AdeReviewRemoved diff, OpLog, LogLines), SearchToolbar (~308) bar
text, ProofScreenshotMenu (~100) menu label (Part 5), live counters (DistillView ~148,
CommentComposer ~116), required asterisks (ParamsForm ~61, AdeContextFields).

### C10 `refactor(ui): form control sizes (M9)`

- `size="kira"` to default in forms: RequestSettingsPane (~180, ~216, ~318), AdeLinkAdd (~84),
  AdeLinkEdit (~40) (plus C5/C7 sites already done).
- Bars keep `kira`: SchemaBrowser, DateTimePicker, TimestampPane, PagerControls, InlineEditInput,
  FileTreeToolbar search, SearchBox, ProjectPickers.
- Drop explicit `size="kira-lg"` on `Input` (20 sites) and `NativeSelect` (19 sites): grep
  `<(Input|NativeSelect)\b[^>]*size="kira-lg"`. Keep it on `SearchField` (default `kira`) and on
  `Button`.

### C11 `chore(scripts): fix ui-primitives plan path`

- `scripts/check-ui-primitives.sh` line ~15: `docs/v0.3.16/plans/P672-part2.md` to
  `docs/v0.3.17/plans/P672-part2.md`.
- Confirm TEMPORARY block holds no U110 line and no AdeMainTab/AdeAddPopover U113 line.

### C12 `docs(architecture): forms rules after P672 Part 3`

`docs/ARCHITECTURE.md` "UI uniformity rules (P672)" (~1452) forms bullet:

- bare `Label` allowed with placement-only classes for a checkbox/switch in a bar, menu or list row,
  or `sr-only`;
- checkbox indeterminate shows the primitive minus glyph;
- settings switch rows are `Field horizontal`, control first, reset button `ml-auto`;
- widths `w-24`/`w-40`/`w-56` only; replace "textarea rows 3, code min-h-32, prompt min-h-24" with
  the T1/T2/T3 steps;
- form controls take primitive default size; `size="kira"` only in bars and rows.

### C13 `test(ui): forms and labels specs`

- `apps/kira-space/tests/ui/ade-v2-task-panel.spec.ts` (~162-201) "Main tab rows share one grid":
  rewrite to assert `[data-testid=ade-main-fields] [data-slot=field][data-orientation=responsive]`
  rows; label left x shared; control left x shared at panel width >= 24rem.
- `apps/kira-space/tests/ui/ade-v2-workflows.spec.ts` (~199-200): `[data-slot="label"]` to
  `[data-slot="field-label"]`.
- New assertions in existing specs:
  - `ade-v2-review.spec.ts`: review folder checkbox indeterminate shows `svg.lucide-minus`; no
    `.codicon-check` under any `[data-slot=checkbox]`;
  - `agent-provider.spec.ts`: agent settings switch row is `[data-slot=field][data-orientation=horizontal]`
    with switch before label;
  - `studio/docker-edit.spec.ts`: `field()` root has `data-orientation="responsive"` and a
    `[data-slot=field-label]`.
- Targeted run near phase end (implementer, once): the specs above plus ade-v2-stage-settings,
  ade-v2-sandbox, ade-v2-workflow-db-mcp, ade-v2-panel, ade-v2-row-actions, ade-v2-task-flow,
  consistency, ade-v2-workflow-switch, ade-v2-run, ade-v2-task-status, ade-v2-add, ade-v2-notes,
  ade-v2-orchestrator, studio/connection-mcp-tables, studio/data-view, studio/tooltips,
  studio/tree, studio/stream-produce, studio/sqs-poll-visibility, studio/http-curl,
  memory-provenance, memory-ranking, memory-import, single-select-toggles, git-host-azure.
  Fix every failure in this commit or a follow-up `fix:` commit.

### C14 `docs: P672 Part 3 done`

`docs/v0.3.17/SPEC.md` row P672 Part 3: Done, Result (files touched, wrappers retired, baseline
lines removed, visual baselines expected to change, any U117 or category exceptions found).
Last commit.

## Test split

- UI-only: no IPC, bridge or Go change, so no flow half; no unit tests (no complex logic).
- UI specs: C13 list, implementer's targeted run.
- Visual baselines expected to change, re-recorded in PR CI (not locally): `workflow-graph*.png`,
  `sandbox-popover`, `settings-advanced`, `settings-appearance`, `settings-experimental`,
  `settings-git`, `settings-memory`, `memory-distill-result`, `new-connection-dialog`,
  `connection-privacy-*`, `script-dialog`, `runs-restore-dialog`, `ade-repos-page`,
  `approvals-dialog`, `review-step-panel`, `git-repo-settings-dialog`. Likely unchanged:
  `git-reset-dialog`, `git-stash-dialog`, `docker-restore-dialog`, `http-request-view-at-rest`.
- Screenshots (`screenshots/build/*.shot.ts`) are UI suite: not run locally.

## Risks

- `Field responsive` needs an `@container` ancestor; without it rows stay stacked silently. C4/C5/C6
  name each ancestor; implementer confirms in DOM via the C13 assertions.
- Task panel narrow widths (< 24rem): rows stack; spec asserts column alignment only when wide.
- `useId` ids must not collide inside `v-for`; use one `useId()` per component instance and suffix
  per row key.
- Removing `size-*` from checkboxes inside dense rows (FieldRowsTable, ResponseHistoryList) may
  shift row height by 0-2px; acceptable, baselines re-record.
- U117 trap (see C7). Guard script per commit catches it.
- Category (c) vs (a) judgement: a checkbox in a dialog body is a form (d), in a toolbar or table
  row is (c). When unsure, form.
- EditField slot props change (`id` added) is additive; callers with multi-control slots (size
  value+unit) bind `id` to the first control only.

## Out of scope

- Buttons and dialog footers, dialog sizes (Part 4). Menus, popovers, tooltips (Part 5). Headings,
  detail headers, key-value, cards (Part 6), except `SectionHeading` for the Branches row. Status
  tones (Part 7). `SecondaryTabs` variants (Part 8), except ScopeField `choice` forced by U117.
  States, icons, type residue (Part 9).
- `SwatchRadio` (theme radio hit target), `CheetahProto`, mobile touch-scale textareas.
- Other guard baselines (U111-U117 lines not named above) stay for their Parts.
