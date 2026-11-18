# P672 Part 10, iteration Part 8 iter2: tabs, segmented, choice

Master: `plans/P672-part10.md`. First pass: `plans/P672-part8.md`. Tree: `chapter/v0.3.17` at
`42a1885ba`. UI only: no Go, no bridge, no new unit tests. Paths below are relative to
`apps/kira-space/frontend/src/` unless they start with `packages/` or `tests/`.

## Rules applied

- ARCHITECTURE RTa (~line 1470): a form value is SecondaryTabs `choice` (RadioGroupCard for up to 6
  options with descriptions). Content sections are `tabs`. A view switch in a bar is `segmented`.
  `choice` is `kira-lg` beside form controls and default in a row or composer. `tabs` and `segmented`
  take no size (U126). A `choice` gets Field + FieldLabel with the same text as its aria-label.
- ARCHITECTURE ~772: runs params `select` renders `choice` up to 5 choices, NativeSelect past that.
  Applied here only where a direct sibling doing the same job already renders `choice` or
  `segmented`. A general threshold for every NativeSelect is a design call (see Decision list).
- ARCHITECTURE ~2366: ADE tab counts render `Badge variant="count"` through `countBadge`. Siblings
  outside ADE render the same count as a muted span. Two styles for one job: F13.
- Exceptions kept: "Bar value toggles stay `segmented`" (F6 and F19 apply it, never undo it). "Engine
  cards without descriptions" untouched. No exception re-opened.
- Accessibility is not a goal. Aria-labels below exist only because RTa ties the `choice` label to
  FieldLabel text and U133 needs a name on every control.

## Current state (rerunnable)

Scan scripts in the scratchpad (`p8i2/scan.py`, `hand.py`, `sel.py`): quote-aware tag scan over the
six guard DIRS, minus `components/ui/`. Equivalent grep: `grep -rlzP --include=*.vue` with the
`(?(DEFINE)(?<t>...))` quote-aware tag prefix used in `scripts/check-ui-primitives.sh`.

- `<SecondaryTabs`: 79 tags. None under `mobile/`.
- `<RadioGroup` 7, `<RadioGroupItem` 2 (EnvironmentsView, SwatchPicker), `<RadioGroupCard` 8 (Reset,
  Revert, CherryPick, review route, engine picker).
- `<Toggle` 10. `workbench/OperationsPanel.vue:351` has `variant="outline"`. The other 9 (kube
  LogsView ×4, docker LogsView ×3, AdeTaskChain ×2) use the default variant with `size="kira"`.
- `<Tabs>` / `<TabsContent>`: AdeShell and ContainerEditView only.
- `<Button ... :aria-pressed=` (any variant): AdeReposPage, AdeNotesEditor, PreviewCommandPanel,
  ApiVariablesDialog, mobile KeyBar, theme TooltipIconButton. The same with `variant="toolbar"`: the
  first four.
- U117 extended dry run (below): 2 files, AdeStepSandbox and AdeStepRunFields. DialogBody part: 0.
- G8a dry run (below): 5 files, AdeReposPage, AdeNotesEditor, PreviewCommandPanel,
  ApiVariablesDialog, OperationsPanel.
- `countBadge` attr hosts: AdeAllSessions:110, AdePanelFrame:19. SecondaryTabs hosts with `count`
  items and no `countBadge`: HttpRequestView, http ResponsePane cookies, DockerPanel, ReviewToolbars,
  RefsPopover, OpLogToolbar.
- codegraph_explore: 5 calls (SecondaryTabs hosts and hand tab strips; ADE inspector fields;
  RepoFilePane/ContainerEditView/KindBar; PanelBar/PanelHeader/AdeWorkflowsTab; RepoScriptField
  callers).

## Findings

### A. SecondaryTabs role per host

- F1 `ade/v2/workflows/inspector/AdeStepSandbox.vue:107` (`ade-wf-sandbox-access`). Default `tabs`
  inside `Field orientation="responsive"` with FieldLabel: a form value. Change: add
  `variant="choice" size="kira-lg" :aria-label="t('space.ade.workflows.sandbox.access.label')"`
  (the FieldLabel key).
- F2 `ade/v2/workflows/inspector/AdeStepRunFields.vue`. Both strips are form values inside a Field.
  - :100 scope (`ade-wf-step-scope`): add `variant="choice" size="kira-lg"`. Keep its aria-label.
  - :125 gate (`ade-wf-step-gate`): add `variant="choice" size="kira-lg"
    :aria-label="t('common.action.start.label')"` (its FieldLabel text).
- F3 `ade/v2/workflows/inspector/AdeStepAgentFields.vue:58` (`ade-wf-step-mode`, prompt or smart).
  Bare `tabs` strip, a step value, no label. Change: wrap in `<Field orientation="responsive">
  <FieldLabel>{{ t('common.mode.label') }}</FieldLabel>`, strip gets `variant="choice"
  size="kira-lg" :aria-label="t('common.mode.label')"`. Precedent: ConnectionGeneralTab mode choice.
- F4 `ade/v2/workflows/AdeStepInspector.vue:218` (`ade-wf-step-kind`). Step kind value in the header
  row beside the inline name Input. Change: `variant="choice"`, no size (row host),
  `:aria-label="t('common.field.kind.label')"`.
- F5 `repo/RepoScriptField.vue:57` (command or run mode). Value inside a Field owned by the hosts
  RepoConfigForm and RepoEnvRow; U117 misses it because the Field sits in the parent. Change:
  `variant="choice" size="kira-lg"`. Keep its aria-label.
- F6 view switch of the same content inside a ViewToolbar renders `tabs`; every sibling view switch
  (CommentComposer, ProofChangePanel, PreviewCommandPanel, RepoDiffView, AdeWorkflowEditor) renders
  `segmented`. Change each to `variant="segmented"`, no size:
  - `views/repo/LocalFileView.vue:95` (source or reading);
  - `views/repo/RepoFileView.vue:201` (source or reading);
  - `studio/views/httprequest/ResponsePane.vue:311` (pretty or raw; the pane-section strip beside it
    stays `tabs`);
  - `studio/views/definition/DefinitionView.vue:289` (structure or source).

### B. Hand option groups and toggles

- F7 `ade/v2/repos/AdeReposPage.vue:382-400`. "Other files" is a wrap of toolbar Buttons with
  `:aria-pressed="pickedOther?.file === f.file"`: a single content pick, hand-rolled tabs. Change:
  `<SecondaryTabs wrap :model-value="pickedOther?.file ?? ''" :items="otherFileItems"
  @update:model-value="pickOtherFile">` (tabs variant). Items: `value: f.file`, `label: f.file`,
  `testid: \`ade-repos-file-${f.file}\``. `#item` slot keeps StatusDot `error` for a problem file,
  else `CodiconIcon name="file" :size="13"`, then the label. File is 491 lines; stays under N10.
- F8 `studio/views/grid/PreviewCommandPanel.vue:120-140`. Pending-change pick list made of toolbar
  Buttons with `:aria-pressed`. Rows with a selected state are `rowVariants`. Change: `<button
  type="button" :class="cn(rowVariants({ layout: 'menu', selected: c.id === selected?.id }),
  'w-full')" :aria-pressed="c.id === selected?.id">`. Keep `data-testid="pending-diff-item"` and
  `data-column`. Precedent: TimePicker presets.
- F9 `ade/v2/notes/AdeNotesEditor.vue:233-247`. Format toggles: hand toolbar Buttons with
  `:aria-pressed`, native `:title` and a text glyph fallback. Icon toggles elsewhere are
  TooltipIconButton `:pressed`. Change: `<TooltipIconButton v-for="btn in toolbarButtons"
  :key="btn.id" :icon="btn.icon" :label="btn.label" :pressed="btn.active"
  data-testid="ade-notes-toolbar-btn" @mousedown.prevent @click="btn.run" />` (keep the existing
  mousedown focus keeper). Heading button icon: `text-size` (in codicon.css). Drop the `glyph` field
  and the `text-kira-sm` class.
- F10 `studio/api/ApiVariablesDialog.vue:244-252`. Bulk-edit toggle is a toolbar Button with
  `:aria-pressed`. Text toggles elsewhere are `Toggle`. Change: `<Toggle v-model="bulk" size="kira"
  data-testid="api-variables-bulk-toggle">` with the same label.
- F11 `workbench/OperationsPanel.vue:351`. Only Toggle with `variant="outline"`; all 9 siblings use
  the default. Change: drop `variant="outline"`.
- F12 theme, one idle/on text rule for every toggle-shaped control.
  - Today: toolbar Button, `tabs` and `choice` idle text is muted; `segmented` (34 hosts) and Toggle
    (10 hosts) idle text is `fg`, because `toggleVariants` sets no idle colour and SecondaryTabs adds
    it per variant.
  - Change `packages/theme/src/components/ui/toggle/index.ts` base: add `text-muted-foreground
    aria-pressed:text-fg data-[state=on]:text-fg` (hover:text-fg already there).
  - `packages/theme/src/components/SecondaryTabs.vue`: drop the `tabs` per-variant text class; drop
    `text-muted-foreground hover:text-fg data-[state=on]:text-fg` from `CHOICE_ITEM`. Keep
    `TEXT_ITEM`'s `text-subtle`; confirm `cn` (twMerge) resolves it over the base muted class.
- F13 count style. Remove the `countBadge` prop from SecondaryTabs. Every item `count` renders
  `<Badge variant="count" class="px-1">`, in both the plain and tooltip branches. Drop the
  `count-badge` attr at `ade/v2/AdeAllSessions.vue:110` and `ade/v2/panel/AdePanelFrame.vue:19`.
  Hosts that change from span to Badge with no edit: HttpRequestView, http ResponsePane cookies,
  DockerPanel, ReviewToolbars, RefsPopover, OpLogToolbar. Typecheck catches any missed
  `countBadge` attr after the prop goes.

### C. NativeSelect value pickers whose direct siblings render choice or segmented

Each set is fixed, 2-4 options. Item testids follow `${testid}-${value}`; group keeps the old
select testid.

- F14 `workbench/settings/DateFormatField.vue:45`. Sibling RowDensityField renders `choice`. Change:
  mirror RowDensityField: FieldLabel without `for` (drop `useId` and its comment), `<SecondaryTabs
  variant="choice" size="kira-lg" :aria-label="t('space.settings.appearance.commitDate.label')"
  data-testid="settings-date-format">` with items `relative` and `absolute`.
- F15 `studio/workbench/settings/DataPane.vue:43` page size (PAGE_SIZES 10/100/1000/10000). The grid
  toolbar page-size control is `segmented`. Change: `choice kira-lg`, values as strings, cast back
  to number in the handler.
- F16 MCP approval policy. Siblings ClusterMcpDialog approval, RunMcpTab run, ConnectionMcpTab render
  `choice`. Change each to `choice kira-lg`:
  - `studio/workbench/settings/StudioMcpApiPane.vue:73` (always or never). Disabled while pending:
    set `disabled` on each item.
  - `studio/workbench/settings/ApiAccessRulesSection.vue:150` (ask or deny). Same disabled rule.
  - `studio/workbench/apiaccess/ApiAccessRuleDialog.vue:94` (allow or deny).
- F17 `ade/v2/workflows/inspector/AdeStepReviewFields.vue:31` send-to (implementer or new). Inspector
  siblings (F2 gate and scope) and AdeWorkflowReviewPanel route render `choice`. Change: `choice
  kira-lg`.
- F18 `packages/docker-ui/src/components/EditSize.vue:44` unit MB/GB beside a number Input. Sibling
  AdeEstimateField unit renders `choice`. Change: `choice kira-lg`, items disabled when `disabled ||
  unlimited`, group testid `${testid}-unit`, items `${testid}-unit-mb` and `${testid}-unit-gb`.
- F19 `studio/project/connection/ConnectionMaskReview.vue:224` filter (pending or all) beside a
  SearchField in the bar. Bar filters AdeAllSessions and OpLogToolbar render `segmented`. Change:
  `segmented`, no size.

## Decision list (not findings)

No direct sibling renders these as `choice`. Converting them means making the ~772 "up to 5 choices"
threshold app-wide. Orchestrator or user decides; not in this plan.

- `LogLevelField` (5 options, settings).
- HTTP version: `ApiPane:117`, `RequestSettingsPane:147` (`HTTP_VERSIONS` 1.1, 2).
- `ConfigSyncUnitRow:86` mine or shared.
- `FormDataTable:129` text or file (table cell).
- git `SearchBox:258` scope (3).
- Docker RESTARTS, PROTOCOLS, MOUNT_TYPES.
- Kibana sample sizes and interval.
- MaskRule kind.
- RepoSettingsDialog options.

## Exemptions re-checked (kept)

- `tabs` hosts that switch content sections: HttpRequestView panes, GitPanel, KindBar chips,
  ResourceTabPanel, ContainerEditView inPlace/recreate, PromptDetail, dialog section tabs.
- RepoFilePane form/yaml: content sections, not in a bar, matches dialog config tabs.
- AdeWorkflowsPage strip `px-1 py-0.5`: placement, outside this subject (spacing, iteration 9).
- EditBufferActions pressed beautify: an action that shows state, already TooltipIconButton.
- Toggles wrapped in TextTooltip: same Toggle, look unchanged.
- EnvironmentsView RadioGroup and SwatchPicker: first-pass exception with named reason (swatch).
- mobile KeyBar ctrl swaps to `dialog-primary`: no touch toggle primitive; the armed modifier must
  read as armed. G8a scopes to `variant="toolbar"` so KeyBar is not hit.
- DateTimePicker cells: date-cell selection, own primitive.
- AdeDayBand: D1 board.

## Guards

Header comment: `# Guards U117 (extended), U<G8a>: docs/v0.3.17/plans/P672-part8-iter2.md.`

- U117 extended in place. New message: `tabs or segmented SecondaryTabs inside a form field (a form
  value is variant choice)`. New regex:

  ```
  (?s)<(?:Field|FieldSet)\b(?&t)(?<!/)>(?:(?!</(?:Field|FieldSet)>).)*?<SecondaryTabs\b(?!(?&t)\svariant="choice")|(?s)<DialogBody\b(?&t)(?<!/)>(?:(?!</DialogBody>).)*?<SecondaryTabs\b(?&t)\svariant="segmented"
  ```

  Today: 2 files (F1, F2). After F1-F3: 0. Implementer re-runs the dry run before committing.
- G8a (new): hand toggle. Message: `hand toggle (icon toggle is TooltipIconButton :pressed, text
  toggle is Toggle default variant, a single pick is SecondaryTabs or a rowVariants list)`. Regex:

  ```
  <Button\b(?=(?&t)\svariant="toolbar")(?&t)\s:?aria-pressed=|<Toggle\b(?&t)\s:?variant=
  ```

  Today: 5 files. After F7-F11: 0. Implementer confirms with a dry run.
- No `countBadge` guard: F13 deletes the prop; typecheck rejects any use.
- PERMANENT count stays 48. No exempt names added.

## Docs updates

`docs/ARCHITECTURE.md`:

- RTa (~1470): add "A view switch of the same content in a ViewToolbar is `segmented`." Add "A text
  toggle is `Toggle` (default variant); an icon toggle is TooltipIconButton `:pressed`; a single
  pick from a list is SecondaryTabs or a `rowVariants` list, never pressed toolbar Buttons." Add
  "Toggle-shaped controls share idle muted text and `fg` when on (toggleVariants)."
- ~2366: count style app-wide: every SecondaryTabs `count` renders `Badge variant="count"`; drop the
  `countBadge` mention.
- Guard ranges: `U83-U146` (~1435) and `Guards U110-U146` (~1471) move to the new top id.

`docs/v0.3.17/SPEC.md`: one progress line under P672 Part 10: `Part 8 iter2: <result>, plan
plans/P672-part8-iter2.md.`

## Stream decision

One sequential implementer. F12/F13 change the theme every other group renders through; guards and
ARCHITECTURE are shared.

## Commits

1. `refactor(theme): one idle text and count style for toggle controls` (F12, F13, two count-badge
   hosts).
2. `refactor(ui): form values in ADE and repo editors render choice` (F1-F5, F17).
3. `refactor(ui): bar view switches and filters render segmented` (F6, F19).
4. `refactor(ui): hand option groups and toggles use shared pieces` (F7-F11).
5. `refactor(ui): settings and MCP value selects render choice` (F14-F16, F18).
6. `test(ui): follow choice, segmented and toggle changes` (spec edits below).
7. `chore(guards): extend U117, add hand toggle guard` (U117, G8a).
8. `docs: tabs and toggle rules for P672 Part 8 iter2` (ARCHITECTURE).
9. `docs: P672 Part 8 iter2 progress` (SPEC).

## Test split

Targeted UI specs only, once near the end. Visual baselines re-record in PR CI. Reka arrow presses
use `tests/ui/support/radioKeys.ts`; on-state asserts use `tests/ui/support/toggleGroup.ts`
(`expectOn`). New helpers go in shared support files (N8).

Spec edits known now:

- `tests/ui/module-state.spec.ts:203-213`: `[data-file="second.yaml"]` and `aria-pressed` become
  testid `ade-repos-file-second.yaml` and `data-state="on"` (F7).
- `tests/ui/ade-v2-workflows.spec.ts:163`: `selectOption('new')` becomes a click on
  `<testid>-new` (F17).
- `repo-graph-keep-state:199`, `window-chrome:263`: date format select becomes item click (F14).
- `dbmcp-api-settings:30-36`, `dbmcp-api-default:49-58`, `dbmcp-api-rules:21`: selectOption becomes
  item click; disabled assert moves to items (F16).
- `connection-mask-review:80,108,121`, `connection-privacy:14`: filter select becomes item click
  (F19).
- Studio data settings specs reading the page-size select (F15).

Run after edits: module-state, studio/mutations (F8, `pending-diff-item` stays), ui-leftovers:55-66
(F9, aria-pressed via `:pressed`), ade-v2-notes, ade-v2-archived, ade-v2-workflows, ade-v2-needs,
ade-v2-panel-tabs (F13), repo-graph-keep-state, window-chrome, dbmcp-api-settings,
dbmcp-api-default, dbmcp-api-rules, connection-mask-review, connection-privacy, http-request*,
data-view, docker specs (F13, F18), operations (F11), api-variables specs (F10), studio settings,
control-sizing.

## Risks

- F12 changes idle text of 34 `segmented` and 10 Toggle hosts: wide visual baseline churn, expected,
  recorded in PR CI. Specs asserting text colour (control-sizing) may need updates.
- F13 Badge is wider than the span; check strips that `wrap` or sit in narrow bars (RefsPopover,
  OpLogToolbar) do not overflow.
- F7 item value is a file name; SecondaryTabs empty model value (`''`) must not select the first item.
- F15 numeric values pass through ToggleGroup as strings.

## Out of scope

- Decision list items.
- Spacing of AdeWorkflowsPage strip (iteration 9).
- Status tones on StatusDot in F7 (iteration 7 owns tone choice; F7 keeps today's tone).

## Handoff

- `ConfigSyncUnitRow` li hand `bg-field` selected state: a row lead, not this subject.
- `DocumentView` row edit `aria-pressed`: a legit toggle; recheck in final scan.

## Files touched

Shared (other iterations may edit):

- `scripts/check-ui-primitives.sh` (U117, G8a, header comment).
- `docs/ARCHITECTURE.md` (RTa, ~2366, guard ranges).
- `docs/v0.3.17/SPEC.md` (progress line).
- `packages/theme/src/components/ui/toggle/index.ts` (F12; theme export surface unchanged).
- `packages/theme/src/components/SecondaryTabs.vue` (F12, F13; `countBadge` prop removed from its
  public props).

Overlap with iteration 9 (states, icons, type, spacing):

- `ade/v2/notes/AdeNotesEditor.vue`: F9 removes the text glyphs and `text-kira-sm`. Iteration 9's
  text-glyph scan would hit the same lines; this plan owns them.

Own:

- `ade/v2/workflows/inspector/AdeStepSandbox.vue`
- `ade/v2/workflows/inspector/AdeStepRunFields.vue`
- `ade/v2/workflows/inspector/AdeStepAgentFields.vue`
- `ade/v2/workflows/inspector/AdeStepReviewFields.vue`
- `ade/v2/workflows/AdeStepInspector.vue`
- `repo/RepoScriptField.vue`
- `views/repo/LocalFileView.vue`
- `views/repo/RepoFileView.vue`
- `studio/views/httprequest/ResponsePane.vue`
- `studio/views/definition/DefinitionView.vue`
- `ade/v2/repos/AdeReposPage.vue`
- `studio/views/grid/PreviewCommandPanel.vue`
- `studio/api/ApiVariablesDialog.vue`
- `workbench/OperationsPanel.vue`
- `ade/v2/AdeAllSessions.vue`
- `ade/v2/panel/AdePanelFrame.vue`
- `workbench/settings/DateFormatField.vue`
- `studio/workbench/settings/DataPane.vue`
- `studio/workbench/settings/StudioMcpApiPane.vue`
- `studio/workbench/settings/ApiAccessRulesSection.vue`
- `studio/workbench/apiaccess/ApiAccessRuleDialog.vue`
- `packages/docker-ui/src/components/EditSize.vue`
- `studio/project/connection/ConnectionMaskReview.vue`
- i18n catalogs only if a needed key is missing (`common.mode.label`, `common.field.kind.label`
  exist; implementer confirms).
- Specs: `tests/ui/module-state.spec.ts`, `tests/ui/ade-v2-workflows.spec.ts`,
  `repo-graph-keep-state`, `window-chrome`, `dbmcp-api-settings`, `dbmcp-api-default`,
  `dbmcp-api-rules`, `connection-mask-review`, `connection-privacy`, studio data settings spec.
