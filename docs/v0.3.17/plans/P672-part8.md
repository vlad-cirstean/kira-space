# P672 Part 8 UI uniformity: tabs, segmented, choice

Spec row: `../SPEC.md` P672 Part 8. Rules RTa.1-RTa.2, migration items M28-M29
(`P672-inventory.md` §17, appendix A.8). Base: `chapter/v0.3.17` at `2527ba1d4` (Parts 1-7 Done).
Inventory line numbers are at `6527aec4`. Every fact below was re-checked at `2527ba1d4` via
`codegraph_explore` and a quote-aware `<SecondaryTabs>` tag scan. Line numbers are hints:
implementer greps the named pattern or `data-testid` before editing.

UI-only phase. Look and layout change only. Behaviour, `v-model` wiring and every `data-testid` stay.
No new i18n key (aria-labels reuse existing keys or data). No IPC, Go or flow change.

## Rules applied (recap)

- RTa.1 role picks the variant: picking a *value* (form field, setting, draft field, stored value
  edited in place) is `choice`; switching *content* sections is `tabs`; switching the *view of the
  same content* in a bar or composer header is `segmented`. A control hosted in a bar or toolbar
  (`ViewToolbar`, `PanelBar`, request bar) stays `segmented` even when it sets a value: bar host
  wins, as the inventory §17.1 table says.
- Size follows RB.1: `choice` `kira-lg` beside default-size form controls; `choice` default (`kira`)
  in a row, card row or a composer whose buttons are `kira`. `tabs` and `segmented` take no `size`
  (SecondaryTabs default is `kira`).
- A `choice` label is `Field` + `FieldLabel` (no `for`; the group is a toggle group, not an input)
  plus `:aria-label` with the same text, the shape `ClusterMcpDialog`, `RunMcpTab` and
  `AdeAddPopover` kind already use. Hosts already on `FieldSet` + `FieldLegend` (`StashDialog`,
  `WorktreeDialog`) stay.
- RTa.2 `ConnectionEnginePicker` onto `RadioGroupCard`: whole card is the radio, engine icon 16 and
  name in the title slot, selected `border-focus bg-select` from the primitive. No description: the
  engine name is the whole content, and the SPEC row asks only icon 16 plus the card. Its U36
  PERMANENT line goes.
- `SecondaryTabs` stays the only ToggleGroup host (U4); `choice` keeps its `bg-select` (U37
  PERMANENT for `SecondaryTabs.vue`, unchanged).

## Current state (scan at `2527ba1d4`)

79 `<SecondaryTabs>` tags (none under `mobile/`). Variants: `tabs` 35, `segmented` 34, `choice` 9,
`text` 1. Explicit `size`: 23 (`kira-lg` 22, `kira` 1).

Changes in this Part (19 `.vue` files):

| File (frontend `src/` or package) | Today | Target | Role |
|---|---|---|---|
| `studio/project/connection/ConnectionGeneralTab.vue:187` | segmented kira-lg | choice kira-lg | connection mode value (fields/URI) |
| `studio/project/connection/ConnectionMcpMasked.vue:53` | segmented kira-lg | choice kira-lg | form value |
| `studio/project/connection/ConnectionMcpReview.vue:78` | segmented kira-lg | choice kira-lg | form value per class |
| `studio/project/connection/ConnectionMcpTab.vue:281` | segmented kira-lg | choice kira-lg | form value per class |
| `workbench/src/automations/form/ParamsEditor.vue:173` | segmented kira-lg | choice kira-lg | param type value |
| `workbench/src/memory/import/ImportConfirmDialog.vue:158` | segmented kira-lg, bare `Label` | choice kira-lg, Field + FieldLabel | origin value |
| `workbench/src/memory/ScopeField.vue:30` | segmented kira-lg, bare `Label` | choice kira-lg, Field + FieldLabel | scope value |
| `workbench/src/settings/fields/RowDensityField.vue:49` | segmented default, label `<span>` | choice kira-lg, FieldLabel | setting value |
| `ade/v2/dialog/AdeAgentDialog.vue:235` | segmented kira-lg | choice kira-lg | per-target value |
| `ade/v2/panel/AdeEstimateField.vue:81` | segmented kira | choice kira-lg | unit value beside default Input |
| `ade/v2/links/AdeLinkCard.vue:126` | segmented default | choice default | stored link kind, card row |
| `ade/v2/stage/AdeStageDialog.vue:168` | choice kira-lg, hand `div` label | choice kira-lg, Field + FieldLabel | label shape only |
| `views/repo/ReviewThread.vue:254` | segmented default | choice default | draft comment kind, composer with kira buttons |
| `ade/v2/AdeAddPopover.vue:183` | tabs kira-lg | tabs default | content tabs |
| `ade/v2/workflows/AdeWorkflowsPage.vue:40` | tabs kira-lg | tabs default | page tabs |
| `ade/v2/workflows/AdeWorkflowEditor.vue:39` | segmented kira-lg | segmented default | graph/YAML view, row with kira toolbar buttons |
| `ade/v2/workflows/AdeLibraryStepEditor.vue:72` | segmented kira-lg | segmented default | same |
| `ade/v2/needs/AdeAllSessions.vue:106` | segmented kira-lg | segmented default | running/stopped filter bar |
| `studio/project/connection/ConnectionEnginePicker.vue` | hand reka `RadioGroupItem` card | `RadioGroupCard` | RTa.2 |

The SPEC row's "9 form `segmented` uses" are the inventory's 8 `kira-lg` form uses plus
`AdeEstimateField`. Fresh scan adds three value pickers the inventory filed under view switches:
`RowDensityField` (already a U117 Part 8 baseline line), `AdeLinkCard` kind and `ReviewThread` kind.
`AdeStageDialog` gets the choice label shape only.

Kept as is (checked, role already right):

- `tabs` default (33): AdeAgentsPage, AdeReviewWindow, AdeStepInspector, AdeStepSandbox,
  AdeStepAgentFields, AdeStepRunFields ×2, AdeSettingsResult, AdePanelFrame, RepoScriptField,
  RepoFilePane, GitPanel ×2, LocalFileView, RepoFileView, HttpRequestView, http ResponsePane ×2,
  DefinitionView, GrpcRequestView:430, grpc ResponsePane, ApiVariablesDialog, KindBar,
  ResourceTabPanel, ConnectionFileDialog, PromptDetail, ImportView, RunFileDialog, RefsPopover,
  ReviewToolbars:197, ContainerDetail, ContainerEditView, DockerPanel. Their classes
  (`w-full`, `shrink-0`, `mx-1 mt-1`, `min-w-0 overflow-x-auto`) only place the strip.
- `segmented` in a bar or composer header (19): RepoDiffView, CommentComposer (write/preview),
  KeyValueToolbar, TimestampPane (zone view), RequestBodyPane (ViewToolbar), GrpcRequestView:368
  (request bar), SchemaBrowser (ViewToolbar), DataToolbar, PreviewCommandPanel, StreamToolbar,
  DocumentToolbar, QueryBar, ProofChangePanel, OpLogToolbar, DistillView ×2 (PanelBar),
  FileTreeToolbar, ReviewFilesPane, ReviewToolbars:226; plus the three ADE bar rows above once
  their `size` drops.
- `choice` (8 besides AdeStageDialog): AdeAddPopover kind, StashDialog, WorktreeDialog,
  ClusterMcpDialog, RunFolderTab, RunMcpTab ×2, ParamsForm. `text`: TabbedContent.

## Baselines

`scripts/check-ui-primitives.sh` TEMPORARY block holds 12 lines; 7 name `P672 Part 8`
(`grep -c 'P672 Part 8'` = 7), all U117, lines 161-167:

```
U117 apps/kira-space/frontend/src/studio/project/connection/ConnectionGeneralTab.vue # 2026-11-18 P672 Part 8
U117 apps/kira-space/frontend/src/studio/project/connection/ConnectionMcpMasked.vue # 2026-11-18 P672 Part 8
U117 apps/kira-space/frontend/src/studio/project/connection/ConnectionMcpReview.vue # 2026-11-18 P672 Part 8
U117 apps/kira-space/frontend/src/studio/project/connection/ConnectionMcpTab.vue # 2026-11-18 P672 Part 8
U117 packages/workbench/src/automations/form/ParamsEditor.vue # 2026-11-18 P672 Part 8
U117 packages/workbench/src/memory/import/ImportConfirmDialog.vue # 2026-11-18 P672 Part 8
U117 packages/workbench/src/settings/fields/RowDensityField.vue # 2026-11-18 P672 Part 8
```

Delete each line in the same commit that fixes its file (a fixed file whose line stays fails as
stale; a line dropped before the fix fails U117). `ScopeField`, `AdeAgentDialog` and
`AdeEstimateField` have no line (their segmented sits outside `Field`/`DialogBody`, so U117 never
matched). After this Part 5 TEMPORARY lines remain (U115 Part 9); never touch them. U117 regex
stays and has zero offenders after C2.

PERMANENT block: delete line 106 (`U36 .../ConnectionEnginePicker.vue # engine tile is a selectable
radio card, not a content box`) in C5; it goes stale once the hand class is gone. The block holds
51 `U` lines today (`awk '/<<.PERMANENT./{f=1;next} /^PERMANENT$/{f=0} f' scripts/check-ui-primitives.sh | grep -c '^U'`);
ARCHITECTURE line 1437 still says 53 (stale since Part 5 removed two U91 lines). After C5: 50.

Run `bash scripts/check-ui-primitives.sh` after every commit (also part of `bun run lint` and the
pre-commit hook). New guard U126 (C6) lands with zero offenders, no TEMPORARY line.

## Theme pieces

None. `SecondaryTabs` (`choice`, `wrap`), `RadioGroupCard`, `Field`/`FieldLabel` and `TextTooltip`
already exist and are used as is. No theme file changes.

## Stream decision

One sequential implementer. 19 `.vue` files, but every commit edits the same guard script blocks
(TEMPORARY, PERMANENT, guard rows) and C6 depends on C1-C5 having emptied its offenders. Too small
and too order-bound to split.

## Commits

One commit per logical group. `bun run lint` (guard, conventions, copy) and typecheck run in the
pre-commit hook on each; never `--no-verify` on a commit that ships. Any `test:` or `fix:` commit
lands before C7.

### C1 `refactor(studio): connection form values to SecondaryTabs choice (M28)`

Each: `variant="segmented"` to `variant="choice"`, keep `size="kira-lg"`, keep the existing
`Field` + `FieldLabel`, add `:aria-label` with the label's text (the visible `FieldLabel` is not
tied to the group):

- `ConnectionGeneralTab.vue:187`: `:aria-label="t('common.mode.label')"`.
- `ConnectionMcpMasked.vue:53`: `:aria-label="t('studio.project.connectionDialog.mcp.masked.label')"`.
- `ConnectionMcpReview.vue:78`: `:aria-label="t(m.label)"` (the per-mode `FieldDescription`
  caption is the group name).
- `ConnectionMcpTab.vue:281`: `:aria-label="t(c.label)"`.

Delete the four Studio U117 TEMPORARY lines.

### C2 `refactor(workbench): params, memory and density values to choice (M28)`

- `automations/form/ParamsEditor.vue:173`: `variant="choice"`, keep `size="kira-lg"`, add
  `:aria-label="t('common.field.type.label')"`. Row stays `flex items-center gap-1.5` with the
  name `Input` and the remove button.
- `memory/import/ImportConfirmDialog.vue:156-166`: replace

  ```vue
  <div class="flex flex-col gap-1">
    <Label>{{ t('memory.import.confirm.origin.label') }}</Label>
    <SecondaryTabs ... variant="segmented" size="kira-lg" ... />
  </div>
  ```

  with

  ```vue
  <Field>
    <FieldLabel>{{ t('memory.import.confirm.origin.label') }}</FieldLabel>
    <SecondaryTabs
      v-model="origin"
      :items="originItems"
      variant="choice"
      size="kira-lg"
      :aria-label="t('memory.import.confirm.origin.label')"
      data-testid="import-origin"
    />
  </Field>
  ```

  Drop the `Label` import if no other `<Label>` remains in the file (grep first).
- `memory/ScopeField.vue:28-39`: same swap (`div` + `Label` to `Field` + `FieldLabel`, `choice`,
  keep `kira-lg`, aria-label and testid). Drop the `Label` import. Component stays two roots
  (scope Field, project Field).
- `settings/fields/RowDensityField.vue`: label `<span>` (line 39) to `<FieldLabel>` (add to the
  `@theme/components/ui/field` import); SecondaryTabs `variant="choice"`, add `size="kira-lg"`
  (settings controls are default size, 26px like `FontSizeField`'s stepper) and
  `:aria-label="t('workbench.settings.rowHeight.label')"`.

Delete the three workbench U117 TEMPORARY lines. U117 now has zero offenders.

### C3 `refactor(ui): ADE and review value pickers to choice (M28)`

- `ade/v2/dialog/AdeAgentDialog.vue:235`: `variant="choice"`, keep `size="kira-lg"`, add
  `:aria-label="tg.branch"` (the target's branch name is the visible caption above the group).
- `ade/v2/panel/AdeEstimateField.vue:81`: `variant="choice"`, `size="kira"` to `size="kira-lg"`
  (the number `Input` beside it is default size since Part 3). Keep `@mousedown.prevent` and its
  comment, keep the aria-label.
- `ade/v2/links/AdeLinkCard.vue:126`: `variant="segmented"` to `variant="choice"`; no size (card
  row host). Keep the `#item` icon slot, tooltips and testids.
- `ade/v2/stage/AdeStageDialog.vue:158-180`: wrapper `div class="flex flex-col gap-1.5"
  data-testid="ade-stage-sessions"` to `<Field data-testid="ade-stage-sessions" v-if=...>`; the
  hand `div class="text-muted-foreground"` label to `<FieldLabel>` holding the same text and the
  `({{ sessionStages }})` span. SecondaryTabs and `FieldDescription` unchanged. Add `Field`,
  `FieldLabel` to the field import. `ade-v2-stage-move` asserts `toContainText` on the testid;
  text is unchanged.
- `views/repo/ReviewThread.vue:254`: `variant="choice"`, no size (composer buttons are `kira`),
  add `:aria-label="t('common.field.kind.label')"`.

### C4 `refactor(ade): tabs and bar view switches at default size (M28)`

- `ade/v2/AdeAddPopover.vue:183`: drop `size="kira-lg"` (content tabs). Keep `class="w-full"`.
- `ade/v2/workflows/AdeWorkflowsPage.vue:40`: drop `size="kira-lg"`. Keep `class="px-1 py-0.5"`
  (places the strip in the page; AdeAgentsPage tabs sit in a PanelHeader, out of scope).
- `ade/v2/workflows/AdeWorkflowEditor.vue:39` and `AdeLibraryStepEditor.vue:72`: drop
  `size="kira-lg"`; the strip then matches the `toolbar@kira` buttons in the same row.
- `ade/v2/needs/AdeAllSessions.vue:106`: drop `size="kira-lg"`.

### C5 `refactor(studio): engine picker on RadioGroupCard (M29)`

`studio/project/connection/ConnectionEnginePicker.vue`:

- Imports: drop `Tooltip, TooltipContent, TooltipTrigger` and reka `RadioGroupItem`; import
  `RadioGroup, RadioGroupCard` from `@theme/components/ui/radio-group` and `TextTooltip` from
  `@theme/components/TextTooltip.vue`.
- Template body of the `RadioGroup` (keep the group's props, class `grid-cols-3 gap-1.5
  content-start`, aria-label, model, testid and `@update:model-value`):

  ```vue
  <TextTooltip
    v-for="option in filteredKinds"
    :key="option"
    :text="
      SUPPORTED_KINDS.has(option)
        ? ''
        : t('studio.project.connectionDialog.kind.unsupported.tooltip', {
            kind: KIND_LABEL[option],
          })
    "
  >
    <RadioGroupCard
      :value="option"
      :disabled="!SUPPORTED_KINDS.has(option)"
      :data-testid="`connection-kind-${option}`"
      @click="pick(option)"
    >
      <template #title>
        <span class="flex items-center gap-1.5">
          <span
            class="flex"
            :class="
              SUPPORTED_KINDS.has(option) ? KIND_ACCENT_CLASS[option] : 'text-muted-foreground'
            "
          >
            <EngineIcon :kind="option" :size="16" />
          </span>
          {{ KIND_LABEL[option] }}
        </span>
      </template>
    </RadioGroupCard>
  </TextTooltip>
  ```

- Why `TextTooltip` with empty text: a supported card gets no tooltip (the old one repeated the
  visible name), so no `TooltipTrigger` stamps its own `data-state` over the card's
  `data-state=checked` (the reason `SecondaryTabs` wraps tooltip items in a span). Only a disabled
  card, never checked, gets the trigger. Keep the `pick` comment (change plus click, re-click of the
  checked kind). The `aria-label` per item goes: `RadioGroupCard` labels itself from the title.
- `scripts/check-ui-primitives.sh`: delete PERMANENT line 106 (U36 ConnectionEnginePicker).

Look changes (accepted, visual baseline): icon 22 above the name becomes icon 16 left of the name;
name weight 400 to 500 (card title); padding py-3 px-2 to the primitive py-2 px-2.5; selected card
gains the `bg-select` fill.

### C6 `chore(lint): guard U126 for SecondaryTabs size outside choice`

`scripts/check-ui-primitives.sh`: header comment after the Part 7 line:

```
# Guard U126: docs/v0.3.17/plans/P672-part8.md.
```

Row after U125:

```
U126@@v@@@@SecondaryTabs tabs, segmented or text with a size (only choice sizes to its host; tabs and segmented keep the default, RTa.1)@@<SecondaryTabs\b(?!(?&t)\svariant="choice")(?&t)\s:?size=
```

Dry run at `2527ba1d4`: 14 files (AdeAddPopover tabs, AdeAgentDialog, AdeAllSessions,
AdeEstimateField, AdeLibraryStepEditor, AdeWorkflowEditor, AdeWorkflowsPage, ConnectionGeneralTab,
ConnectionMcpMasked, ConnectionMcpReview, ConnectionMcpTab, ParamsEditor, ScopeField,
ImportConfirmDialog). After C1-C4: zero. Check by hand before committing: add `size="kira"` to a
`tabs` strip in a scratch copy, run the script, see U126 name the file, revert. If the dry run after
C5 shows any hit, fix it in a follow-up commit before C6, never a TEMPORARY line.

Rejected guards: "form value must be `choice`" (no regex tells a value from a view; U117 already
fails `segmented` inside `Field`/`FieldSet`/`DialogBody`); `choice` must be `kira-lg` (row hosts
take `kira`).

### C7 `docs: architecture for P672 Part 8`

`docs/ARCHITECTURE.md`:

- line 772: "It replaces round RadioGroup except ConnectionEnginePicker, AdeTaskStatuses colours,
  EnvironmentsView active env and card radios." to "It replaces round RadioGroup except
  AdeTaskStatuses colours, EnvironmentsView active env and card radios."
- line 783 (RadioGroupCard): extend the parenthesis to "(Reset mode, review route, mainline
  pickers, P667; Studio engine picker with icon 16 and name as the title, no description)".
- line 1469 (tabs by role): append "Bar-hosted controls stay `segmented`. Size: `choice` kira-lg
  beside form controls, default in a row or composer; `tabs` and `segmented` take no size (U126).
  A `choice` label is Field + FieldLabel with the same text as its aria-label."
- line 1435: `U83-U125` to `U83-U126`.
- line 1437: `53 named PERMANENT lines` to `50 named PERMANENT lines` (recount with the awk line in
  Baselines; write the number it prints).
- line 1470: `U110-U125` to `U110-U126`, `U118-U125` to `U118-U126`.

### C8 `docs: P672 Part 8 done`

`docs/v0.3.17/SPEC.md` Part 8 row: Status Done; Result names commits C1-C7 plus any `test:` or
`fix:` commits, the 12 value pickers moved to `choice` and the label shape, the 5 size drops, the
engine picker on `RadioGroupCard` and the U36 line removed, guard U126, the 7 U117 baseline lines
removed (5 left, Part 9), files touched, specs run, visual baselines for CI to re-record. Last
commit of the phase.

## Test split

UI-only phase: no IPC, Go or flow change, so no flow-test half (CLAUDE.md Tests split applies to
features; this is a restyle with unchanged behaviour). No new unit test (no complex logic). Guard
U126 plus U117 hold the rules. Specs get one assertion per changed piece, in a `test:` commit
before C7:

- `tests/ui/studio/connections.spec.ts` (~line 105, before the postgres click): the postgres card
  has `data-slot="radio-group-card"`, `role="radio"`, `data-state="checked"` (dialog starts on
  postgres, `NewConnectionDialog.vue:31`); `connection-kind-mysql` has `data-state="unchecked"`;
  postgres card `svg.icon` has `width="16"`.
- `tests/ui/memory-import.spec.ts` (a test that opens the confirm dialog): `import-origin-external`
  has `data-state="on"` and class matching `/data-\[state=on\]:bg-select/` (the `choice` marker).
- `tests/ui/studio/control-sizing.spec.ts` row-density test (line 438): add the same class
  assertion on the selected item and a 26px height check (`boundingBox().height` close to 26). The
  connected-outline radii assertions stay valid (`choice` is outline, spacing 0 without `wrap`).
- `tests/ui/ade-v2-dialogs.spec.ts` (~line 117): the `ade-dialog-target-0` group root has
  `data-slot="secondary-tabs"` and its "new session" button matches the `choice` class marker after
  the click.

Existing specs that touch changed hosts (selectors stay; must pass): ade-v2-add, ade-v2-add-link,
ade-v2-base-rebase, ade-v2-details-links, ade-v2-dialogs, ade-v2-needs, ade-v2-notes,
ade-v2-op-kinds, ade-v2-orchestrator, ade-v2-panel-tabs, ade-v2-review, ade-v2-review-working,
ade-v2-stage-move, ade-v2-task-branches, ade-v2-task-flow, ade-v2-task-panel, ade-v2-workflows,
ade-v2-workflow-steps, consistency, copy-coverage, memory-import, memory-provenance,
memory-ranking, module-state, ui-leftovers, automations-mcp, automations-scripts (no spec
names a `params-editor`/`param-*` testid today; ParamsEditor is covered by the RunFileDialog
specs only through render); `studio/`:
connections, connection-form, connection-explain-threshold, connection-mcp-access,
connection-mcp-expensive, connection-mcp-masked, connection-mcp-tables, connection-mcp-timeout,
connection-throttle, dbmcp-review-mode, control-sizing, settings-apply-on-save, tree.

Targeted run near phase end (one pass, from `apps/kira-space`, after `bun run build:test`): the list
above. Implementer greps `tests/` for every changed file name and testid (`ade-wf-tabs`,
`ade-wf-mode`, `ade-step-mode`, `ade-all-`, `ade-links-card-kind`, `ade-estimate-unit`,
`ade-dialog-target`, `ade-stage-`, `review-thread-kind`, `param-type-`, `import-origin`, `-scope-`,
`connection-mcp-`, `connection-kind-`, `rowDensity`, `secondary-tabs`) first and adds any spec found.
No mobile spec is in scope (no `SecondaryTabs` under `mobile/`); if one is added, run
`bun run build:mobile` first.

Visual baselines for CI to re-record (PR CI owns `visual`): `new-connection-dialog` (engine cards),
`settings-appearance` (row density choice), `ade-links-card` (kind choice), `workflow-graph*`
(editor mode strip at kira, if in frame), `connection-privacy-*` if the shot shows MCP choice rows,
plus any shot CI flags. Docs screenshots under `tests/screenshots/` are not CI baselines.

Sandbox lessons for spec edits:

- Playwright click on a menu item inside Monaco needs `locator.hover()` first.
- reka radio and toggle arrow presses need `keyboard.press(key, { delay: 40 })`
  (`tests/ui/support/radioKeys.ts` `pressArrow`); `choice` toggles use `tests/ui/support/toggleGroup.ts`
  (`expectOn`, `expectStaysOnAfterClick`).
- N17 (`scripts/check-conventions.ts`) forbids the `dialog` variant on a delete/discard label
  without a `conventions-allow N17` marker; this Part adds no button.
- N10 caps `.vue` files at 600 lines: largest touched is `ConnectionMcpTab.vue` (400); edits add
  at most a line or two per file.
- U91 exemptions go stale when their pattern vanishes: read the guard output for any stale exempt
  or stale PERMANENT/TEMPORARY entry after each commit (C5 removes the engine picker's hand
  Tooltip and reka item; check no other exempt named that file).
- A timeout under load gets a lone rerun first.

## Risks

- `choice` reads heavier than `segmented` (solid `bg-select` on the selected item). Accepted by
  RTa.1; visual baselines change in settings, connection MCP tab, ADE dialogs and links card.
- `RowDensityField` and `AdeEstimateField` grow from 22px to 26px. Main tab estimate row then
  matches its Input; `ade-v2-task-panel` "Main tab rows share one grid" measures x positions, not
  height, but run it.
- `ConnectionEnginePicker` unsupported branch is unreachable today (all 11 kinds are in
  `SUPPORTED_KINDS`); its tooltip on a disabled card has the same pointer-event limit as before.
  Parity, not a regression.
- `FieldLabel` without `for` renders a `<label>` tied to nothing; screen readers get the group name
  from `aria-label`. Same as existing `choice` hosts.
- `AdeLinkCard` kind and `ReviewThread` kind were not in the inventory's form list; RTa.1 classes
  them as values. If the user disagrees, revert C3's two lines; the guard does not depend on them.

## Out of scope

- `segmented` in bars that set a value (`RequestBodyPane` body mode, gRPC TLS and source toggles,
  `TimestampPane` zone): bar host keeps `segmented` per §17.1.
- `RadioGroupCard` for other single choices with descriptions (RTa.1 optional branch); no host
  qualifies in this scan.
- `AdeWorkflowsPage` strip moving into a `PanelHeader` like `AdeAgentsPage`; layout change beyond
  RTa.
- Primary tab bars (`tabChipVariants`, U6) and `TabbedContent` `text` variant.
- States, icons, type, spacing and U115 baselines: Part 9.
- Any i18n, behaviour or IPC change.
