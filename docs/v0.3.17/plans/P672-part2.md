# P672 Part 2 UI uniformity: theme pieces and guards

Spec row: `../SPEC.md` P672 Part 2. Batch 6, sequential (`plans/batches.md`, `P672.md` "Parts
2..9"). Base: `chapter/v0.3.16` at `23e52ed0` (moved to chapter v0.3.17). UI-only: no IPC, no backend, no flow half.

## Goal

Inventory §20 M1-M3 (`P672-inventory.md`). Decisions D1 (a), D2 all five, D3 toasts unchanged
(`P672.md` "Decisions taken").

1. Theme pieces: `Field orientation="responsive"`, `PopoverContent flush`, `KeyValueList
   density="compact"`, `DetailHeader` composite; Docker `DetailSection` and ADE `AdeTip` moved into
   `packages/theme` (`DetailSection`, `TextTooltip`), every import switched.
2. Lint guards U110-U117 in `scripts/check-ui-primitives.sh`, each with a dated temporary baseline
   of current offenders (new `TEMPORARY` block, never `PERMANENT`).
3. ARCHITECTURE: new subsection with canonical rules RD..RTa in short form.

Hosts: each new piece gets one real host in this Part (user ask: every piece proven in a real host).
Chosen so the visual change is nil or one small step that Parts 5/6 would make anyway:

| Piece | Host | Visual change |
|---|---|---|
| `Field orientation="responsive"` | `AdeInspectorRow` root (replaces its hand grid) | none (same classes) |
| `PopoverContent flush` | `AdeTaskAddMenu` (class `w-80 gap-0 overflow-hidden p-0` today) | none |
| `KeyValueList density="compact"` | `AdeBranchTip` (class `text-kira-sm` today) | row gap 4px to 2px |
| `DetailHeader` | Docker `ResourceDetail` header | min height 26px, title `h2`, registry button right-aligned |
| `DetailSection` | 7 Docker files (move) | none |
| `TextTooltip` | 28 ADE files (move) | none |

This narrows SPEC "no call-site restyle beyond the two moves" by three one-file hosts; Parts 5 and 6
skip them. No knip risk either way: `packages/theme` is all-entry in `knip.json` (`"entry":
["src/**/*.{ts,vue}"]`), so a theme file without a caller passes; the two moved originals are
deleted (`git mv`), so `apps/kira-space/frontend` and `packages/docker-ui` keep no orphan.

## Current state (read via codegraph at `23e52ed0`)

Re-verified against the inventory (base `6527aec4`); counts match unless noted.

- `packages/docker-ui/src/components/DetailSection.vue` (17 lines): `Card variant="section"` >
  `CardHeader` > `CardTitle as="h3"` + `CardAction` (`#actions`), `CardContent :flush`. Props `title:
  string; flush?: boolean`. 21 tags in 7 files, each `import DetailSection from './DetailSection.vue'`:
  `ContainerSizeSection.vue:12` (1 tag), `EngineDiskSection.vue:27` (1), `ContainerEditView.vue:23`
  (1), `StatsView.vue:8` (3), `ResourceDetail.vue:23` (3), `ContainerOverview.vue:11` (6),
  `EngineOverview.vue:24` (6). No test or doc names it.
- `apps/kira-space/frontend/src/ade/v2/AdeTip.vue` (20 lines): `Tooltip` > `TooltipTrigger as-child`
  > slot, `TooltipContent` default with `VarText :parts` or `text`; bare slot when both empty. Props
  `text?: string; parts?: readonly TextPart[]`. 50 tags in 28 files (inventory: 50 uses / 56
  callers, codegraph 56 callers). Import paths: 25 `'../AdeTip.vue'`, 3 `'./AdeTip.vue'`
  (`AdeActivityIcon`, `AdeOrchestratorBadge`, `AdeParentChip`). Files: `ade/v2/` `AdeActivityIcon`,
  `AdeOrchestratorBadge`, `AdeParentChip`, `automation/AdeAutomationChip`, `dialog/AdeAgentDialog`,
  `panel/` `AdeArchivedPanel`, `AdeBranchPanel`, `AdeMainBranches`, `AdeMainTab`,
  `AdePanelForwardBar`, `AdeRunOutcome`, `AdeStepBlock`, `AdeTaskPanel`; `plan/` `AdeAttention`,
  `AdeBranchPr`, `AdeBranchRow`, `AdeHistoryControl`, `AdeLeaseChip`, `AdeTaskCard`,
  `AdeTaskSessions`; `review/` `AdeReviewAgentPanel`, `AdeWorkflowReviewPanel`;
  `run/AdeTaskActionButton`; `sessions/AdeSessionActions`; `updates/AdeUpdatesDot`; `workflows/`
  `AdeStepNode`, `AdeStepsTab`, `AdeWorkflowsTab`. No test or doc names it.
- `ui/field/index.ts:6` `fieldVariants = cva('flex text-kira-md', { orientation: vertical 'flex-col
  gap-1', horizontal 'flex-row items-center gap-1.5' })`; `Field.vue` passes `orientation` and sets
  `data-orientation`. `FieldLabel` = `Label` + `text-muted-foreground`, `data-slot="field-label"`;
  bare `Label` has `data-slot="label"`.
- `ade/v2/workflows/inspector/AdeInspectorRow.vue` (36 lines, 24 tags in 12 files): root `div
  data-slot="inspector-row"`, `stack` ? `flex flex-col gap-1` : `grid grid-cols-1 gap-1
  @[24rem]:grid-cols-[7.5rem_minmax(0,1fr)] @[24rem]:gap-x-3`; `Label` `text-kira-sm
  text-muted-foreground` plus `@[24rem]:flex @[24rem]:min-h-control-lg @[24rem]:items-center`
  unless `stack`; then a `div flex min-w-0 flex-col gap-1` with slot, `#hint` FieldDescription,
  `#error` FieldError. The `@container` ancestor is `AdeStepInspector.vue:229`. Nothing reads
  `inspector-row`.
- `ui/popover/PopoverContent.vue`: props `PopoverContentProps & { class; destructive }`,
  `reactiveOmit(props, 'class', 'destructive')`, class `cn(floatingSurface, '… flex flex-col gap-2.5
  p-2.5 text-kira-md … w-72 …', props.class)`. 41 uses, A.2 unchanged.
- `components/KeyValueList.vue`: `dl data-slot="key-value-list"`, `m-0 grid
  grid-cols-[max-content_minmax(0,1fr)] gap-x-3 gap-y-1 text-kira-md [&>dd]:m-0 [&>dd]:min-w-0
  [&>dt]:text-muted-foreground`. 15 uses; class variants: `AdeBranchTip.vue:70` `text-kira-sm`,
  `AdeArchivedPanel.vue:199` and `AdeBranchPanel.vue:426` `items-center`, `IssueSidebar.vue:30`
  `items-baseline gap-y-2`, `TimelinePane.vue:330` `py-0.5 text-kira-sm font-data`,
  `ApprovalDetails.vue:7` `grid-cols-[72px_minmax(0,1fr)] items-baseline [&>dt]:text-kira-sm
  [&>dt]:text-subtle`, `CommitMeta.vue:390`/`:403` `text-graph-sm`.
- `components/TooltipDetails.vue:18` hand `dl grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5` with
  `div.contents` row wrappers (16 callers). Stays in Part 2; U113 baseline, Part 6 decides.
- `ui/card/CardContent.vue` (`flush` prop) and `ui/dialog/DialogBody.vue` (`flush`: `gap-0 p-0`) are
  the precedent for a boolean `flush` prop.
- Docker `ResourceDetail.vue:121-143` header: `div flex shrink-0 items-center gap-2 px-3 py-2`,
  `CodiconIcon :size="16" text-muted-foreground`, title `span min-w-0 truncate text-kira-lg
  font-medium data-testid="docker-resource-title"`, registry `TooltipIconButton
  data-testid="docker-resource-registry"`, `span text-kira-sm text-muted-foreground
  data-testid="docker-registry-missing"`, `Badge` kind label. No visual baseline shows it.
- `scripts/check-ui-primitives.sh` (369 lines): guards U1-U109 in the `GUARDS` heredoc (format
  `id@@kinds@@built-in exempt@@message@@regex`, unquoted heredoc so `${TOK}` expands); one
  `PERMANENT` heredoc (53 lines today; ARCHITECTURE:1437 still says 38); stale permanent and stale
  built-in exemptions fail. No temporary-baseline mechanism and no self-test file exists (only
  `check-conventions.ts` has a `.spec.ts`).
- `PanelHeader` (h-bar, sm, muted, uppercase, 500) vs `SectionHeading` (h-control-sm, sm, subtle,
  uppercase): the two-step grey hierarchy (inventory §7.1) for ARCHITECTURE.

## Design

All pieces `<script setup lang="ts">`, Tailwind utilities only, no `<style>`, `cn()` for class
merge, `data-slot` on the root (shadcn-vue convention), no new dependency, no new catalog key (no
piece carries text; hosts reuse their keys, so C3/C4 see nothing new).

### 1. `Field orientation="responsive"` (RF.2)

`packages/theme/src/components/ui/field/index.ts`: move `flex` out of the base so no variant fights
it (check-class-conflicts):

```ts
export const fieldVariants = cva('text-kira-md', {
  variants: {
    orientation: {
      vertical: 'flex flex-col gap-1',
      horizontal: 'flex flex-row items-center gap-1.5',
      // Label column beside the control once the nearest @container is 24rem wide, stacked below.
      responsive:
        'grid grid-cols-1 gap-1 @[24rem]:grid-cols-[7.5rem_minmax(0,1fr)] @[24rem]:gap-x-3 @[24rem]:[&>[data-slot$=label]]:flex @[24rem]:[&>[data-slot$=label]]:min-h-control-lg @[24rem]:[&>[data-slot$=label]]:items-center @[24rem]:[&>:not([data-slot$=label])]:col-start-2',
    },
  },
  defaultVariants: { orientation: 'vertical' },
});
```

- `[data-slot$=label]` matches both `FieldLabel` (`field-label`) and bare `Label` (`label`), so the
  host and Part 3 call sites work. Every other child (control, `FieldDescription`, `FieldError`, a
  wrapper div) lands in column 2 under the control. An `sr-only` label is out of flow; the control
  still sits in column 2, aligned with its neighbours.
- Needs an `@container` ancestor (unnamed, nearest), same as `AdeInspectorRow` today. Without one it
  stays stacked: safe fallback. ARCHITECTURE says so.
- `Field.vue` unchanged (its prop type comes from `FieldVariants`).

Host `AdeInspectorRow.vue`: root `div` becomes `<Field :orientation="stack ? 'vertical' :
'responsive'">` (import `Field` from `@theme/components/ui/field`), drop `data-slot="inspector-row"`
and the root class binding; the `Label` keeps `text-kira-sm text-muted-foreground` only (drop the
three `@[24rem]:` classes Field now owns). Rendered classes equal today's; Part 3 retires the
component.

### 2. `PopoverContent flush` (RM.3)

`packages/theme/src/components/ui/popover/PopoverContent.vue`: add `flush?: boolean` (default
false) to the props intersection, add `'flush'` to `reactiveOmit` so reka never sees it, and put
`props.flush && 'gap-0 overflow-hidden p-0'` between the base string and `props.class` in `cn()`.
Comment one line: list popover with its own header, scroll list and footer rows.

Host `AdeTaskAddMenu.vue:59-64`: `class="w-80 gap-0 overflow-hidden p-0"` becomes `flush
class="w-80"`.

### 3. `KeyValueList density` (RH.4)

`packages/theme/src/components/KeyValueList.vue`:

- props `{ class?: HTMLAttributes['class']; density?: 'default' | 'compact' }`, default `'default'`.
- base `m-0 grid grid-cols-[max-content_minmax(0,1fr)] gap-x-3 [&>dd]:m-0 [&>dd]:min-w-0
  [&>dt]:text-muted-foreground`; density `default` adds `gap-y-1 text-kira-md`, `compact` adds
  `gap-y-0.5 text-kira-sm`. Local `cva` in the script block (not exported: a `.ts` under
  `components/` fails N9, and no other file needs it).
- `:data-density="density"` on the `dl` (same idea as Field's `data-orientation`; spec hook).
- Comment: compact is for sidebars and tooltips.

Host `AdeBranchTip.vue:70`: `class="text-kira-sm"` becomes `density="compact"`.

### 4. `DetailHeader` (RH.2), new `packages/theme/src/components/DetailHeader.vue`

```vue
<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { FieldDescription } from '@theme/components/ui/field';

// Top strip of a view or detail pane: leading icon, title, meta, right-aligned actions.
defineProps<{ title?: string; icon?: string; titleTestid?: string }>();
</script>

<template>
  <div data-slot="detail-header" class="flex shrink-0 flex-col gap-0.5 px-3 py-2">
    <div class="flex min-h-control-lg min-w-0 items-center gap-2">
      <CodiconIcon v-if="icon" :name="icon" :size="16" class="shrink-0 text-muted-foreground" />
      <slot name="title">
        <h2 class="m-0 min-w-0 truncate text-kira-lg font-medium" :data-testid="titleTestid">
          {{ title }}
        </h2>
      </slot>
      <div v-if="$slots.meta" class="flex min-w-0 shrink-0 items-center gap-1.5 text-kira-sm text-subtle">
        <slot name="meta" />
      </div>
      <div v-if="$slots.actions" class="ml-auto flex shrink-0 items-center gap-1">
        <slot name="actions" />
      </div>
    </div>
    <FieldDescription v-if="$slots.description"><slot name="description" /></FieldDescription>
  </div>
</template>
```

- `#title` replaces the `h2` (inline-edit titles such as `AdeTaskPanel`'s Input, Part 6).
- `div`, not `header`: a `header` outside `main`/`article`/`section` is a banner landmark, and a
  window shows several detail panes.
- Lint fit: `min-h-control-lg` (no raw `h-6`, U38), gaps 0.5/1/1.5/2 and `px-3 py-2` (U88), icon 16
  (U12), `h2` with `text-kira-lg font-medium` (U94), plain `truncate` with no tooltip wrapper (U91),
  prop named `*Testid` (U97). U112 exempts this file by name.
- Actions slot takes `TooltipIconButton` / `toolbar@kira` and at most one `dialog-primary@kira`
  (RB.1/RB.2); meta takes badges and subtle sm facts. Rule text in ARCHITECTURE.

Host `ResourceDetail.vue:121-143` becomes:

```vue
<DetailHeader :title="title" :icon="ICONS[kind]" title-testid="docker-resource-title">
  <template #meta>
    <span v-if="registry.missing.value" data-testid="docker-registry-missing">{{
      t('docker.detail.registryMissing.status') }}</span>
    <Badge>{{ t(KIND_LABELS[kind]) }}</Badge>
  </template>
  <template #actions>
    <TooltipIconButton v-if="registry.url.value" icon="link-external" … same props and testid … />
  </template>
</DetailHeader>
```

Drop the `CodiconIcon` import if unused. `docker-resource-title` stays on the `h2`; specs reading
its text keep working.

### 5. `DetailSection` to theme (RC.1)

`git mv packages/docker-ui/src/components/DetailSection.vue
packages/theme/src/components/DetailSection.vue`. Content unchanged except one comment line: titled
section card (`Card variant="section"`). The 7 imports become `import DetailSection from
'@theme/components/DetailSection.vue';` (sorted by Biome into the `@theme` group). Tags unchanged.

### 6. `TextTooltip` from `AdeTip` (RT.2)

`git mv apps/kira-space/frontend/src/ade/v2/AdeTip.vue
packages/theme/src/components/TextTooltip.vue`. Same props and markup; add one comment line:
plain-text tooltip, renders the bare slot when `text` and `parts` are empty. In the 28 files:
`import AdeTip from '…AdeTip.vue'` becomes `import TextTooltip from
'@theme/components/TextTooltip.vue'`, `<AdeTip` becomes `<TextTooltip`, `</AdeTip>` becomes
`</TextTooltip>`. Then `bunx biome check --write` on the touched files (import order). Afterwards
`grep -rn AdeTip apps packages` must return nothing.

## Lint guards (M2)

### Temporary baseline mechanism

New heredoc after `PERMANENT` in `scripts/check-ui-primitives.sh`:

```sh
# "U<n> path # YYYY-MM-DD P672 Part <k>": a current offender of a P672 guard, kept until Part <k>
# fixes it. Never PERMANENT. An entry that stops matching fails the run, so the block only shrinks;
# P672 Part 9 deletes the block and this mechanism once it is empty.
cat >"$TMP/temp" <<'TEMPORARY'
U110 apps/kira-space/frontend/src/studio/api/ApiVariablesTable.vue # 2026-11-18 P672 Part 3
…
TEMPORARY
```

Script changes (POSIX sh, like the rest):

1. Validate every non-comment `TEMPORARY` line against `^U11[0-7] [^ ]+ # [0-9]{4}-[0-9]{2}-[0-9]{2}
   P672 Part [3-9]$` (`grep -Ev`); a malformed line fails with `check-ui-primitives: malformed
   temporary baseline: <line>`.
2. Strip the comment into `$TMP/temp_ids` (same `sed` as `perm`), `sort -u`.
3. A pair present in both `perm` and `temp_ids` fails (`in PERMANENT and TEMPORARY`).
4. In the hits loop: allowed when in `perm` (record `used_perm`) or in `temp_ids` (record
   `used_temp`); else the existing failure message.
5. Stale loop over `temp_ids`: `check-ui-primitives: stale temporary baseline: <id> <path> (fixed;
   delete the line)`.
6. On success, print `check-ui-primitives: <n> temporary P672 baseline entries left.` when n > 0,
   before the existing pass line.
7. Header comment: one line naming U110-U117 (P672 Part 2 plan) and the `TEMPORARY` block.

Date: the day the implementer writes the block (`date +%F`), same on every line. Granularity is per
file, like `PERMANENT`.

### Guards (append after U109; `${TOK}` as in the file)

```
U110@@v@@@@Checkbox restyled: size class or a check icon child (the primitive draws the 14px box and its CheckIcon)@@<Checkbox\b(?&t)\s:?class="[^"]*${TOK}size-|(?s)<Checkbox\b(?&t)(?<!/)>(?:(?!</Checkbox>).)*?<CodiconIcon\b
U111@@v@@@@PopoverContent padding or gap class (default form popover, or flush for a list popover)@@<PopoverContent\b(?&t)\s:?class="[^"]*${TOK}(?:p|px|py|pt|pb|gap)-
U112@@v@@DetailHeader.vue@@heading element outside DetailHeader (titles DetailHeader, sections SectionHeading or FieldLegend, cards CardTitle)@@<h[1-6]\b
U113@@v@@@@KeyValueList class sets grid, gap, type or alignment (use density)@@<KeyValueList\b(?&t)\s:?class="[^"]*${TOK}(?:grid-cols-|gap-|text-|font-|items-|py-|\[&)
U113@@v@@KeyValueList.vue@@hand key-value grid or raw <dl> (use KeyValueList)@@${TOK}grid-cols-\[(?:auto|max-content|72px|7\.5rem)_(?:1fr|minmax\(0,1fr\))\]|<dl\b
U114@@v@@/ade/v2/plan/@@ADE tone class off the plan board (theme tones and Badge elsewhere, P672 D1)@@${TOK}(?:text|bg|border)-tone-[a-z]
U115@@vt@@@@font-mono (use font-data)@@${TOK}font-mono(?![\w-])
U116@@vt@@@@toned menu icon (menu icons stay muted; a destructive item uses variant destructive)@@iconClass:\s*['"\x60][^'"\x60]*${TOK}text-(?:ok|warn|error|info|tone-)
U117@@v@@@@segmented SecondaryTabs inside a form (a form value is variant choice)@@(?s)<(?:Field|FieldSet|DialogBody)\b(?&t)(?<!/)>(?:(?!</(?:Field|FieldSet|DialogBody)>).)*?<SecondaryTabs\b(?&t)\svariant="segmented"
```

Notes:

- Rule map: U110 RF.8, U111 RM.3, U112 RH.1/RH.3, U113 RH.4, U114 RS.3 (D1), U115 RTy.2, U116 RM.2,
  U117 RTa.1.
- U112 with U94 means no raw `h1`-`h6` outside `DetailHeader`; `CardTitle as="h3"` is not matched.
- U114 built-in exempt `/ade/v2/plan/` is the D1 board allowance (a rule, not an offender list), so
  it lives in the exempt column, not in a baseline. `ade/v2/tones.ts` strings (consumed by
  `AdeChip`) are `.ts` and not scanned (kind `v`); Part 7 handles `AdeChip` hosts.
- No `$` in any regex (the heredoc is unquoted). `(?s)` as the last alternative follows U92.
- `<Checkbox\b` does not match `<CheckboxRow`; `<Field\b` does not match `<FieldSet`.

### Expected baseline after this Part's hosts (118 lines), scanned at `23e52ed0`

Implementer regenerates it: add the guards with an empty block, run the script, turn each `U11x
path` failure into a line, then diff against this list. A difference means a hit changed since
planning; record it in the Result.

| Guard | Part | Files |
|---|---|---|
| U110 (18) | 3 | Studio `api/ApiVariablesTable`, `approvals/ReviewResultOption`, `project/DataGripImportDialog`, `project/FiltersDialog`, `project/connection/CheckboxRow`, `project/connection/ConnectionMcpTables`, `views/documents/ProjectionMenu`, `views/grid/ColumnsMenu`, `views/httprequest/RequestSettingsPane`, `views/httprequest/ResponseHistoryList`, `views/shared/fields/FieldRowsTable`, `views/stream/StreamFilterRow`; git-ui `components/FileTree`; workbench `automations/form/ParamsEditor`, `automations/form/ToolsField`, `automations/run/ParamsForm`, `memory/distill/DistillResult`, `memory/import/ImportConfirmDialog` |
| U111 (31) | 5 | ADE `AdeAddPopover`, `links/AdeLinkAdd`, `panel/AdeTaskBranchForm`, `plan/AdeArchivedFinder`, `plan/AdeEstimateChip`, `plan/AdeTaskChildren`, `sessions/AdeSessionsMenu`, `status/AdeAgentsStatusItem`, `updates/AdeUpdatesPopover`, `workflows/statuses/AdeTaskStatuses`; Studio `api/VariableHistoryMenu`, `api/VariablesOverviewPanel`, `project/ErrorPopover`, `views/console/kibana/FilterBar`, `views/console/kibana/TimePicker`, `views/documents/DocumentToolbar`, `views/grid/ColumnsMenu`, `views/grid/PreviewCommandPanel`, `views/shared/SavedListMenu`, `views/shared/celleditor/CellGenerateMenu`, `views/shared/celleditor/TimestampPane`, `views/stream/StreamFilterRow`, `views/stream/StreamToolbar`; git-ui `App`, `components/RefsPopover`, `components/SearchBox`; workbench `automations/form/ScheduleFields`, `automations/run/AdeContextFields`, `components/AutocompleteField`, `components/RunningOpsStatusItem`, `components/pickers/PickerPopover` (`AdeTaskAddMenu` leaves via `flush`) |
| U112 (22) | 6 | mobile `AppShell`, `components/PhoneApprovals`, `screens/PairScreen`, `screens/TerminalScreen`; ADE `later/AdeShelfSection`, `needs/AdeAllSessions`, `panel/AdeArchivedPanel`, `panel/AdeBranchPanel`, `port/AdePortExportDialog`, `port/AdePortResult`, `settings/AdeSettingsResult`, `settings/AdeSettingsSecrets`, `workflows/statuses/AdeTaskStatuses`; Studio `api/ApiAgentAccess`, `api/ApiSecrets`; docker `EngineOverview`; git-ui `review/ReviewNoBranch`, `review/ReviewView`; workbench `automations/RunSecretsSection`, `llmPrompts/PromptDetail`, `memory/distill/DistillResult`, `memory/distill/DistillRewriteResult` |
| U113 (14) | 6 | class: ADE `panel/AdeArchivedPanel`, `panel/AdeBranchPanel`; `hostview/IssueSidebar`; Studio `views/httprequest/TimelinePane`; git-ui `components/CommitMeta`; workbench `approvals/ApprovalDetails`. Grid or `dl`: ADE `AdeAddPopover`, `panel/AdeMainTab`; `repo/RepoIdentity`; Studio `approvals/ProofReviewBody`; theme `TooltipDetails`; workbench `approvals/ApprovalPairs`, `components/OpLogDetail`, `memory/MemoryStart` (`AdeBranchTip` and `AdeInspectorRow` leave via this Part's hosts) |
| U114 (19) | 7 | ADE `AdeActivityIcon`, `context/AdeContextPane`, `dialog/AdeAgentDialog`, `links/AdeLinkStrip`, `needs/AdeAllSessionRow`, `needs/AdeNeedsPage`, `panel/AdeBranchPanel`, `panel/AdeRunOutcome`, `panel/AdeStepBlock`, `panel/AdeTaskIssueLine`, `review/AdeReviewSync`, `sessions/AdeSessionsRow`, `settings/AdeSettingsPane`, `workflows/AdeStepNode`, `workflows/AdeStepsTab`, `workflows/AdeWorkflowsTab`; workbench `automations/RunFileDialog`, `automations/ScriptBrokenRow`, `automations/ScriptRow` |
| U115 (5) | 9 | ADE `details/AdeDetailsDiskBanner`, `panel/AdePanelForwardBar`, `request/DetailsEditBody`; Studio `proof/ProofHeader`; `workbench/settings/FileTypesPane` |
| U116 (1) | 5 | docker `ContainerList` |
| U117 (8) | 8 | ADE `dialog/AdeAgentDialog`; Studio `project/connection/ConnectionGeneralTab`, `ConnectionMcpMasked`, `ConnectionMcpReview`, `ConnectionMcpTab`; workbench `automations/form/ParamsEditor`, `memory/import/ImportConfirmDialog`, `settings/fields/RowDensityField` |

Paths above are short; baseline lines carry the full repo path (`apps/kira-space/frontend/src/…`,
`apps/kira-space/frontend/mobile/…`, `packages/<pkg>/src/…`).

### How later Parts empty it

- Each Part fixes the files of its guard(s) and deletes those lines in the same commit; a fixed file
  left in the block fails as stale, a new offender fails outright.
- A genuine requirement a primitive cannot meet follows the existing named-exemption rule (a
  `PERMANENT` line with its reason, reviewed in that Part's plan); a `TEMPORARY` line is never
  renamed into one.
- Part 7 may move a file it classifies as board into U114's exempt column (D1 rule), else fixes it.
- Regex coverage gaps go to the owning Part's own re-scan: U117 misses `ScopeField` (segmented not
  inside a `Field`); U111 also lists bar popovers (`git-ui App`, `AutocompleteField`) that Part 5
  must route to `flush` plus inner padding or a named exemption.
- Part 9 (M36) finds the block empty, deletes the block and the mechanism (validation, merge, stale
  loop, count line), and the ARCHITECTURE sentence about it.

### Self-test

Repo convention for `check-ui-primitives.sh`: no spec file; its stale checks are the self-test. The
implementer proves each new guard by hand, nothing committed: (a) the script passes with the full
block; (b) delete one line per guard, the script names exactly that file, restore; (c) add a bogus
line (`U115 packages/theme/src/components/KeyValueList.vue # <date> P672 Part 9`), the script reports
it stale; (d) add a malformed line, the script reports it malformed. Record the four results in the
Result.

## ARCHITECTURE (M3)

New `### UI uniformity rules (P672)` after "Shared pickers (P398)", before `## Process model`.
Bullets, terse, one rule each:

- Dialog size by content: sm confirm or single field; md form up to about 8 fields; lg sections or
  list picker; xl two panes, diff, preview; 2xl fixed-height editor or multi-tab config; full review
  windows only.
- Dialog footer: `#start` secondary actions, then dismiss, alternate, one emphasised button
  (dialog-primary, dialog-danger or dialog-approve) rightmost. Header is the title plus an optional
  DialogDescription for intro text; mode tabs open DialogBody. Body keeps dialogBodyClass (only
  `flush`, `min-h-*`); sections are FieldSet + FieldLegend or SectionHeading. Failure is an error
  Alert as last body child, field validation FieldError.
- Button size by host: form commit kira-lg; bars, rows, popover action rows kira or
  TooltipIconButton; link is inline. Outline (dialog*) only commits a form or answers a question;
  every other in-view action is toolbar with a leading icon. ACTION_CLASS only on the plan board and
  task card step action.
- One field label: FieldLabel (bare Label only sr-only or for a checkbox in a bar or menu). Two
  layouts: stacked Field, or `Field orientation="responsive"` (label column 7.5rem once the nearest
  `@container` is 24rem wide, stacked below; the host pane carries `@container`). Checkbox and
  switch rows are `Field orientation="horizontal"`; Checkbox takes no size class and no icon child.
  FieldSet without gap override. Fixed widths w-24, w-40, w-56; textarea rows 3, code min-h-32,
  prompt min-h-24.
- Menus render MenuItems from a MenuItem[]; menu icons 13 and muted. Popovers have two shapes:
  default (form) and `flush` (list: header row px-1.5 h-control, p-1 row list, footer px-1.5 py-1);
  widths w-56, w-80, w-96, w-120, w-auto for bar popovers only.
- Rows come from rowVariants; trailing meta is rowMeta; entity lists are rows, tabular data SlickGrid
  or Table.
- Heading roles: PanelHeader (panel title bar, muted) is one grey step above SectionHeading (section
  in a panel, list or popover, subtle); FieldSet + FieldLegend groups a form; CardTitle on Card
  section titles a card (DetailSection); DetailHeader titles a view or detail pane (icon 16 muted,
  h2 lg 500, `#meta`, right-aligned `#actions`, `#description`). No other heading markup.
- Key-value data is KeyValueList: default density in panes, `density="compact"` (sm, row gap 0.5)
  in sidebars and tooltips; dt muted, dd fg, font-data per data value, no class for grid, gap or type.
- Card variants: inset (small block of text), section (titled group, DetailSection), list (scroll
  list of rows); classes only place the card in its parent. Code text is CodeBlock; a row is never a
  Card.
- Status tones: running or in progress info (+ Spinner), succeeded ok, waiting or
  partial warn, failed error, idle or unknown neutral. Row or header uses Badge, dense list
  StatusDot, prose tone text. ADE pastel tones stay on the plan board and task cards (D1).
- State primitive by scope: whole pane EmptyState fill, list or section InlineState, failed operation
  with content kept Alert error, invalid field FieldError. EmptyState media only Spinner or a content
  illustration. Skeleton shapes: text h-4, title h-control-lg w-1/3, block h-24 w-full.
- Plain hint: TextTooltip (`text` or `parts`; no tooltip when empty) or Tooltip; icon button
  TooltipIconButton; disabled reason TooltipDisabledTrigger; several facts TooltipDetails.
- Type roles: xl landing hero, lg detail and dialog titles, md body and controls (no class), sm meta,
  badges, section labels; font-data for data only, never font-mono.
- Spacing: bar px-1.5 gap-1; row from rowVariants gap-1.5; content pane p-3, gap-3 between blocks,
  gap-2 inside; dialog body p-3 gap-3; card, popover, radio card keep the primitive 2.5 inset with
  gap-1.5 inside; inline icon and text gap-1, chips gap-1.5. Icons 13 in buttons, menus, rows, bars;
  12 in Badge or inline text; 16 DetailHeader or card tile; 24 empty state.
- Radii: rounded-kira-sm controls, rows, chips, inset blocks; rounded-kira containers; rounded-full
  dots, avatars, count badges. Dividers 1px border-border; border-l-2 only for quotes; shadows only
  from elevation tiers.
- Tabs by role: a form value is SecondaryTabs `choice` (or RadioGroupCard for up to 6 options with
  descriptions); content sections `tabs`; view switch in a bar `segmented`.
- Guards U110-U117 in check-ui-primitives.sh hold these rules; current offenders sit in its dated
  TEMPORARY block, one line per file naming the P672 Part that fixes it, never PERMANENT. A fixed
  entry left behind fails as stale; Part 9 removes the block.

Edits to existing lines in the same commit:

- `:784` Tooltip bullet: append "A plain-text hint uses TextTooltip."
- `:787` Card bullet: append "Card section with a title is DetailSection."
- `:1405`: "View or detail titles use DetailHeader (text-kira-lg font-medium); sections use …"
  (rest unchanged).
- `:1437`: "keeps 53 named PERMANENT lines" (stale 38 corrected) and "plus a dated TEMPORARY block
  for the P672 guards".
- `:1435`: "(U83-U117)".

## Tests

No unit tests: no piece holds complex logic (WORKING_AGREEMENT "Testing"). No flow half: UI only, no
IPC change. One UI assertion per piece in an existing spec whose scenario already reaches the host:

| Piece | Spec | Assertion |
|---|---|---|
| Field responsive | `apps/kira-space/tests/ui/ade-v2-workflows.spec.ts`, a test that opens the Graph editor with a step selected (`[data-testid="ade-wf-step"]` visible) | first `[data-testid="ade-wf-step"] [data-slot="field"][data-orientation="responsive"]` visible; its label box and the next child box share a top within 4px and label x < control x (inspector at default width is past 24rem) |
| PopoverContent flush | `ade-v2-row-actions.spec.ts` "the card add button adds a repo…" | after clicking `ade-card-add`, `[data-testid="ade-card-add-menu"]` has CSS `padding-top` `0px` and `row-gap` `0px`, before picking the repo |
| KeyValueList compact | `ade-v2-plan.spec.ts` "branch tooltip shows plan colours" | `tip.locator('[data-slot="key-value-list"]')` has `data-density` `compact` and CSS `row-gap` `2px` |
| DetailHeader | `apps/kira-space/tests/ui/studio/docker-module.spec.ts` registry test (~line 367) | `[data-testid="docker-resource-detail"] [data-slot="detail-header"]` contains `getByRole('heading', { level: 2 })` with the image tag text and the `docker-resource-registry` button |
| DetailSection | same docker test | resource detail has `getByRole('heading', { level: 3, name: 'Details' })` |
| TextTooltip | `ade-v2-panel-forward.spec.ts:118` (hovers the `AdePanelForwardBar` host, already asserts the tooltip text) | unchanged; run it |

Implementer's targeted run (`playwright test --config=apps/kira-space/playwright.config.ts
--project=ui <files>` after `bun run build:test`): the five specs above plus `ade-v2-card-icons`,
`ade-v2-panel` (branch tip), `consistency` (docker registry error), `ade-v2-flat-steps`,
`ade-v2-stage-settings` (inspector rows), `ade-v2-plan`, `ade-v2-review-open` (tooltips). Plus
`bun run typecheck`, `bun run lint`, `bun run lint:dead`.

Visual baselines: `apps/kira-space/tests/visual/ade-plan-branches.spec.ts` `plan-branch-tip.png`
changes (compact row gap); re-record in the PR CI fix pass. Expected unchanged:
`ade-workflow-graph` shots (inspector classes identical), Docker and ADE tooltip shots (moves only).
No baseline shows `ResourceDetail` or the card add menu. Any other CI diff on these surfaces gets
re-recorded in the same pass.

## File ownership (one sequential implementer; no streams)

| File | Change |
|---|---|
| `packages/theme/src/components/ui/field/index.ts` | responsive variant, `flex` out of base |
| `packages/theme/src/components/ui/popover/PopoverContent.vue` | `flush` prop |
| `packages/theme/src/components/KeyValueList.vue` | `density` prop, `data-density` |
| `packages/theme/src/components/DetailHeader.vue` | new |
| `packages/theme/src/components/DetailSection.vue` | moved from docker-ui |
| `packages/theme/src/components/TextTooltip.vue` | moved from `ade/v2/AdeTip.vue` |
| `packages/docker-ui/src/components/{ContainerSizeSection,EngineDiskSection,ContainerEditView,StatsView,ContainerOverview,EngineOverview}.vue` | import path |
| `packages/docker-ui/src/components/ResourceDetail.vue` | import path, DetailHeader host |
| the 28 `ade/v2/**` TextTooltip files (Current state) | import, tag rename |
| `apps/kira-space/frontend/src/ade/v2/workflows/inspector/AdeInspectorRow.vue` | Field responsive host |
| `apps/kira-space/frontend/src/ade/v2/plan/AdeTaskAddMenu.vue` | flush host |
| `apps/kira-space/frontend/src/ade/v2/plan/AdeBranchTip.vue` | compact host |
| `scripts/check-ui-primitives.sh` | U110-U117, TEMPORARY block and mechanism |
| `docs/ARCHITECTURE.md` | new subsection, five line edits |
| the five specs in Tests | assertions |

## Commits (Conventional Commits, hook clean, no `--no-verify`)

1. `refactor(theme): move DetailSection from docker-ui into theme`
2. `refactor(theme): move AdeTip into theme as TextTooltip`
3. `feat(theme): Field responsive, PopoverContent flush, KeyValueList density` (with hosts)
4. `feat(theme): DetailHeader composite, first host Docker ResourceDetail`
5. `chore(lint): P672 guards U110-U117 with dated temporary baselines`
6. `docs(architecture): P672 UI uniformity rules`
7. `test(ui): assert P672 theme pieces in their hosts`

Fixes found by the targeted run land as follow-up commits. Then mark the SPEC row Done with its
Result (pieces, hosts, guard ids with baseline counts per Part, self-test results, specs run, the
expected `plan-branch-tip.png` re-record).

## Shared files with Parts 3-9 (sequential, no batch with Part 2)

- `scripts/check-ui-primitives.sh` `TEMPORARY` block: each Part deletes only its own lines (table
  above); Part 9 deletes the block and mechanism.
- `docs/ARCHITECTURE.md` "UI uniformity rules (P672)": a Part edits a bullet only if its plan
  changes the rule.
- Theme pieces: Part 3 uses `Field responsive` and retires `AdeInspectorRow` (its root is already
  Field); Part 5 uses `flush` (skip `AdeTaskAddMenu`) and `TextTooltip`; Part 6 uses `DetailHeader`
  (skip `ResourceDetail`), `KeyValueList density` (skip `AdeBranchTip`; `TooltipDetails` is its
  call) and `DetailSection` for section cards.
