# P672 Part 6 UI uniformity: headings, detail headers, key-value, cards

Spec row: `../SPEC.md` P672 Part 6. Rules RH.1-RH.4, RC.1-RC.3, migration items M20-M23
(`P672-inventory.md` §7, §8, appendix A.3, A.5). Base: `chapter/v0.3.17` at `6e78c7e42`.
Inventory line numbers are at `6527aec4`. Every fact below was re-checked at `6e78c7e42` via
`codegraph_explore` and template scans. Line numbers are hints: implementer greps the named pattern
or `data-testid` before editing.

UI-only phase. Look and layout change. Behaviour, i18n keys and every `data-testid` stay. No new
i18n key (EmptyState in ReviewView reuses `git.review.start.title` and `git.review.noRepo.emptyHint`).

## Rules applied (recap)

- RH.1 heading roles fixed: `PanelHeader` titles a panel bar; `SectionHeading` a section in a
  panel, list, pane or popover; `FieldSet` + `FieldLegend` a form group; `CardTitle` a
  `Card variant="section"` (`DetailSection`); `DetailHeader` a view or detail pane. No other
  heading markup (U112).
- RH.2 one `DetailHeader` shape: `px-3 py-2`, title row `min-h-control-lg gap-2`, leading icon 16
  muted (or `#leading`), title lg 500, `#meta` sm subtle, `#actions` right-aligned, optional
  `#description` below. Extended in C1 (props and slots below) so every host fits without classes.
- RH.3 section titles in dialogs and panes: `SectionHeading` for read-only or list content,
  `FieldLegend` for a group of inputs. `h3 text-kira-lg` goes.
- RH.4 key-value data is `KeyValueList` only: two densities, dt muted, dd fg, `font-data` per data
  value. No class sets grid, gap, type or alignment (U113 row 1); no hand KV grid or raw `<dl>`
  (U113 row 2).
- RC.1 card jobs: inset (small block of text; never holds Input, Select, Checkbox, ARCHITECTURE
  line 787), section (titled group or controls; `CardHeader`/`CardContent`, `DetailSection`), list
  (scroll list of rows, p-1). Card classes only place the card in its parent (`shrink-0`,
  `max-h-*`, `min-w-0`, `overflow-y-auto`, `relative`, `min-h-*`).
- RC.2 code text is `CodeBlock` (class prop for `break-words`, `select-all`, tone only).
- RC.3 a row is never a `Card`: rows are `rowVariants` rows inside one list card.
- D1: ADE pastel tones stay on the plan board and task cards. `AdeTaskCard` is out of scope. No
  `text-tone-*` change here (Part 7 owns tones; `AdeBranchPanel` checkout amber stays as is).

## Baselines

`scripts/check-ui-primitives.sh` TEMPORARY block holds 34 lines naming `P672 Part 6`
(`grep -c 'P672 Part 6'` = 34): U112 lines 154-175 (22 files), U113 lines 176-187 (12 files).
Total TEMPORARY lines 65; 31 remain after this part (Part 7 19, Part 8 7, Part 9 6). Delete each
line in the same commit that fixes its file: the stale check fails a commit that fixes a file and
leaves its line, and a commit that drops a line before fixing its file fails the guard. Two files
carry both a U112 and a U113 line (`AdeArchivedPanel`, `AdeBranchPanel`): C2 fixes both
violations and deletes both lines. Run `bash scripts/check-ui-primitives.sh` after every commit
(part of `bun run lint`). New guard U124 (C7) lands with zero offenders and no TEMPORARY line.

Exempt columns stay live: U112 exempts `DetailHeader.vue`, so DetailHeader keeps literal `<h1`
and `<h2` tags (no `<component :is>`); U113 row 2 exempts `KeyValueList.vue`, which keeps `<dl`.

## Theme pieces

All in C1. Each has a first consumer in this part.

1. `packages/theme/src/components/DetailHeader.vue`, extended (only consumer today:
   `packages/docker-ui/src/components/ResourceDetail.vue`, which uses `title`, `icon`,
   `titleTestid`, `#meta`, `#actions` and keeps working unchanged):
   - `as?: 'h1' | 'h2'` (default h2). First consumer: mobile `PairScreen` (spec asserts level 1).
   - `wrap?: boolean`: title wraps (`whitespace-normal break-words leading-snug`, no truncate),
     title row `items-start`. First consumer: `ProofStepView` (spec asserts no ellipsis).
   - `data?: boolean`: title `font-data break-all`. First consumer: `AdeBranchPanel` (branch name).
   - `flush?: boolean`: no `px-3 py-2` (host already pads). First consumer: `PromptDetail`.
   - `#leading` slot replaces the icon (StatusDot, colour dot, chips, back button). First
     consumer: `ContainerDetail` StatusDot.
   - `#eyebrow` slot: one line above the title row. First consumer: `AdeBranchPanel` back link.
   - `#title` slot now renders *inside* the heading (rich title content keeps the heading role).
     First consumer: `PromptDetail` (`PromptTitle`).
   - `#control` slot replaces the heading (inline-edit title). First consumer: `AdeTaskPanel`.
   - `#description` becomes a `div` (rich fact lines fit), look unchanged: sm subtle
     leading-normal.
   - Default slot: blocks below the header (Alerts, forward bar, notices). First consumer:
     `AdeTaskPanel`.
2. `packages/theme/src/components/KeyValueList.vue`:
   - Base adds `items-baseline` (one alignment for every list; replaces the `items-center` and
     `items-baseline` overrides). Child selectors `[&>dt]`/`[&>dd]` become `[&_dt]`/`[&_dd]` so
     rows wrapped in `div.contents` (TooltipDetails, ApprovalPairs) get dt muted and dd
     `m-0 min-w-0` without per-element classes.
   - `columns?: 'auto' | 'fixed'` (default auto = `grid-cols-[max-content_minmax(0,1fr)]`; fixed =
     `grid-cols-[4.5rem_minmax(0,1fr)]`, 72px key column). First consumer: `ApprovalDetails`
     (`approvals-alignment.spec.ts:346-354` holds dd at title x + 84 = 72 + gap-x-3).
3. `packages/theme/src/components/SectionHeading.vue`:
   - `flush?: boolean`: `px-0`. First consumer: `StudioStart.vue:116` (`class="mb-1.5 px-0"`
     becomes `flush class="mb-1.5"`).
   - `#leading` slot before the label (non-collapsible branch); label span gains
     `min-w-0 truncate`. First consumer: `AdeAllSessions` group colour square.

No new composable, store or query: everything is shadcn-vue primitives (Card parts, Field parts,
Badge, Button, EmptyState), theme composites and Tailwind utilities. Every component stays
`<script setup lang="ts">`.

## Shapes (copy exactly)

DetailHeader template (classes via `cn`):

```vue
<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { cn } from '@theme/lib/utils';
import { computed } from 'vue';

// View or detail pane title strip (RH.2). `wrap` lets a long title wrap instead of truncating;
// `#control` replaces the heading with an inline-edit input; default slot holds blocks below.
const props = defineProps<{
  title?: string;
  icon?: string;
  titleTestid?: string;
  as?: 'h1' | 'h2';
  wrap?: boolean;
  data?: boolean;
  flush?: boolean;
}>();
const titleClass = computed(() =>
  cn(
    'm-0 min-w-0 flex-1 text-kira-lg font-medium text-fg',
    props.wrap
      ? 'flex min-h-control-lg items-center whitespace-normal break-words leading-snug'
      : 'truncate',
    props.data && 'font-data break-all',
  ),
);
</script>

<template>
  <div
    data-slot="detail-header"
    :class="cn('flex shrink-0 flex-col gap-0.5', !flush && 'px-3 py-2')"
  >
    <div v-if="$slots.eyebrow" class="flex min-w-0 items-center"><slot name="eyebrow" /></div>
    <div
      :class="cn('flex min-h-control-lg min-w-0 gap-2', wrap ? 'items-start' : 'items-center')"
    >
      <div v-if="$slots.leading" class="flex h-control-lg shrink-0 items-center gap-1.5">
        <slot name="leading" />
      </div>
      <div v-else-if="icon" class="flex h-control-lg shrink-0 items-center">
        <CodiconIcon :name="icon" :size="16" class="text-muted-foreground" />
      </div>
      <slot v-if="$slots.control" name="control" />
      <h1 v-else-if="as === 'h1'" :class="titleClass" :data-testid="titleTestid">
        <slot name="title">{{ title }}</slot>
      </h1>
      <h2 v-else :class="titleClass" :data-testid="titleTestid">
        <slot name="title">{{ title }}</slot>
      </h2>
      <div
        v-if="$slots.meta"
        class="flex h-control-lg min-w-0 shrink-0 items-center gap-1.5 text-kira-sm text-subtle"
      >
        <slot name="meta" />
      </div>
      <div v-if="$slots.actions" class="ml-auto flex h-control-lg shrink-0 items-center gap-1">
        <slot name="actions" />
      </div>
    </div>
    <div
      v-if="$slots.description"
      class="flex min-w-0 flex-col gap-0.5 text-kira-sm leading-normal text-subtle"
    >
      <slot name="description" />
    </div>
    <div v-if="$slots.default" class="mt-1 flex flex-col gap-1.5"><slot /></div>
  </div>
</template>
```

Title takes `flex-1` so `#meta` sits right of the title text only when the title is short; the
title truncates (or wraps) before meta and actions shrink. ResourceDetail look: unchanged except
title `flex-1` (meta moves to the right edge before actions). Accept; Docker visual baselines
re-record.

KeyValueList base: `m-0 grid items-baseline gap-x-3 [&_dd]:m-0 [&_dd]:min-w-0
[&_dt]:text-muted-foreground`, plus `columns` variant (`auto`, `fixed`) and the density variant
as today.

SectionHeading non-collapsible branch:

```vue
<div v-else data-slot="section-heading" :class="cn('flex items-center justify-between gap-1.5
  h-control-sm text-kira-sm text-subtle uppercase tracking-wider', flush ? 'px-0' : 'px-1.5')">
  <span class="flex min-w-0 items-center gap-1.5">
    <slot name="leading" /><span class="min-w-0 truncate">{{ label }}</span>
  </span>
  …count, default slot unchanged
</div>
```

Collapsible branch takes `flush` the same way. Root keeps the attribute fallthrough (single root
per branch).

Read-only section: `<SectionHeading flush :label="…" />` directly above its content in a host
that already pads (`p-3` pane or dialog body). Form group: `<FieldSet><FieldLegend>…</FieldLegend>
…fields</FieldSet>` (Part 3 shape, no gap override).

Studio list card (RC.3):

```vue
<Card variant="list">
  <ul class="m-0 flex list-none flex-col p-0" data-testid="(host list testid)">
    <li v-for="…" :class="cn(rowVariants({ layout: 'double' }),
      'items-center justify-between gap-1.5 whitespace-normal')" data-testid="(row testid)">
```

## Stream decision

One sequential implementer. C1 theme pieces are consumed by every later commit; the guard
TEMPORARY block, `ARCHITECTURE.md` and `SPEC.md` are shared by all; C5 and C6 both touch
`TimelinePane` and `MemoryStart`; C2 and C5 both reason about KeyValueList alignment. No split.

## Commits

Each commit: typecheck (`bun run typecheck`), lint (`bun run lint`, includes the guard) green.
Delete the named TEMPORARY lines in the same commit.

### C1 `feat(ui): detail header slots, key-value columns, section heading flush`

Files: `packages/theme/src/components/DetailHeader.vue`, `KeyValueList.vue`,
`SectionHeading.vue`, `apps/kira-space/frontend/src/studio/workbench/panels/StudioStart.vue`
(`flush` replaces `px-0`). Shapes above. No TEMPORARY line changes. Check ResourceDetail still
renders (`studio/docker-module.spec.ts:373` heading level 2, `:378` level 3 "Details").

### C2 `refactor(ade): panel and workflow detail headers (M20)`

`AdePanelFrame.vue` (`apps/kira-space/frontend/src/ade/v2/panel/`): head becomes
`<div class="shrink-0" data-testid="ade-panel-head"><slot name="header" /></div>`; comment updated
("summary slot holds a DetailHeader").

`AdeTaskPanel.vue`: `#header` holds one `DetailHeader`:
- `#leading`: dot (`ade-panel-dot`), status chip, orchestrator badge, blocked chip.
- `#control`: the inline `Input` (`ade-panel-title`, keeps `appearance="inline"` and
  `class="min-w-0 flex-1 text-kira-lg font-medium"`, placeholder, aria-label, handlers).
- default slot: title error Alert, action error Alert, forward bar, automations block, review note
  (same order, same `v-if`s).
- `#actions` on AdePanelFrame stays.

`AdeBranchPanel.vue` (569 lines; N10 cap 600, net change must not grow it past 600):
- `#header` holds one `DetailHeader data wrap`: `#eyebrow` back link (`ade-panel-back`);
  `#leading` dot, chips, agent; title = branch name (move the h3's `data-testid`, if any, to
  `title-testid`); `#description` checkout line (`ade-panel-checkout`, tone classes unchanged) and
  facts (`ade-panel-facts`); default slot the Alerts.
- Action row (`flex flex-wrap gap-1.5 pt-1`: retry-setup, force-push, review, rebase, other)
  moves into AdePanelFrame `#actions` (PanelBar, like AdeTaskPanel). Buttons keep variant, size,
  testids; if a button is `dialog`/`dialog-primary` today it becomes `toolbar@kira` to match the
  bar (AdeTaskPanel review uses `toolbar@kira`). N17: no delete/discard label among them.
- `line-clamp-2` drops: `wrap` shows the whole name (`break-all` from `data`).
- Details tab `KeyValueList class="items-center"`: drop the class; dd `min-h-row flex …` drops
  `min-h-row` (baseline base aligns dt with the dd's control text). Delete U112 line 161 and U113
  line 177.

`AdeArchivedPanel.vue`:
- `#header` holds `DetailHeader wrap`: `#leading` chips; title = archived task title (the
  TextTooltip around the h3 goes: the wrapped title is never clipped); `#description` the facts
  row text and restore hint; default slot the Alerts.
- Restore button (`ade-archived-restore`) moves into AdePanelFrame `#actions`.
- `KeyValueList class="items-center"`: drop class and any dd `min-h-row`. Drop `TextTooltip`
  import if unused. Delete U112 line 160 and U113 line 176.

`ade/v2/workflows/AdeWorkflowsTab.vue` and `AdeStepsTab.vue`: the detail pane's
`<PanelHeader>{{ name }}</PanelHeader>` becomes `<DetailHeader :title="…same expression…" />`
(item name is a detail title, sentence case lg, not an uppercase panel label). List pane
`PanelHeader` stays (panel bar). Drop the `PanelHeader` import only if unused (list pane keeps
it). Row-name size and the editor's `p-4` stay (Part 9 type and spacing).

### C3 `refactor(ui): docker, proof, runs, prompt, review and mobile detail headers (M20)`

- `packages/docker-ui/src/components/ContainerDetail.vue` header (lines 139-210): one
  `DetailHeader`; `#leading` StatusDot; `#title` name span content (`title-testid` =
  `docker-detail-name`, drop the span); `#meta` state Badge (`docker-detail-state`); `#actions`
  start/stop/restart buttons unchanged (icon tone and size are Part 7/9); `#description` second
  line (image, registry TooltipIconButton, Identifier, status, ports, live stats
  `docker-detail-live`), flex-wrap row inside, `text-muted-foreground` class drops (description
  colour is subtle).
- `packages/docker-ui/src/components/EngineOverview.vue`: root becomes `flex h-full min-h-0
  flex-col` (keeps `docker-engine-overview` testid); `DetailHeader :title` with `#meta` version
  Badge and EndpointChip, `#actions` start-stopped and stop-all buttons; body
  `<div ref="scrollEl" class="min-h-0 flex-1 overflow-auto p-3">` (ContainerTable reads
  `scrollEl`; keep the ref name). Delete U112 line 169.
- `apps/kira-space/frontend/src/studio/proof/ProofStepView.vue:146` (`proof-step-header`):
  `DetailHeader wrap data-testid="proof-step-header" :icon :title title-testid=
  "proof-step-description"`; `#meta` source Badge (`proof-step-source`). Drop the hand
  `mt-0.5`/`leading-snug` classes. `proof-window.spec.ts` (not clipped, no ellipsis, larger than
  `proof-title`) holds.
- `packages/workbench/src/automations/runs/ScriptRunView.vue` (`script-run-view`): root `flex
  h-full min-h-0 flex-col`; `DetailHeader data-testid="script-run-header"`: `#leading`
  SmartBadge; title scriptName; `#meta` RunStatusBadge, RunElapsed, model, resumedFrom;
  `#actions` review, decline, stop (order and variants unchanged, drop `ml-auto` classes). Body
  `div.flex.min-h-0.flex-1.flex-col.gap-2.overflow-y-auto.px-3.pb-3` holds the rest.
  `ade-v2-automations.spec.ts:406` (run-status inside header) holds.
- `packages/workbench/src/llmPrompts/PromptDetail.vue:82`: `DetailHeader flush` (PromptsPane
  `main` pads `pl-3`); `#title` PromptTitle; `#actions` Reset button; `#description` the three
  FieldDescriptions become plain `<p class="m-0">` lines (description wrapper owns the look).
  Delete U112 line 173.
- `packages/git-ui/src/components/review/ReviewNoBranch.vue:16` (`review-no-branch`):
  `DetailHeader :title="t('git.review.start.title')"` with `#description` hint; BranchPicker in
  `div.px-3.pb-3` below. Delete U112 line 170.
- `packages/git-ui/src/components/review/ReviewView.vue` (598 lines, N10): noActiveRepo block
  becomes `<EmptyState fill :title="t('git.review.start.title')"
  :description="t('git.review.noRepo.emptyHint')" />` (EmptyState already imported; net lines
  drop). Delete U112 line 171.
- `apps/kira-space/frontend/mobile/AppShell.vue:49`: header becomes `DetailHeader as="h1"
  class="border-b border-border bg-chrome"` (class falls through to the root), `#meta` StatusDot,
  `#actions` refresh touch-icon button. Delete U112 line 154.
- `apps/kira-space/frontend/mobile/screens/PairScreen.vue:57`: `DetailHeader as="h1" flush`
  (copy-wording spec: level 1 heading in `pair-screen`). Delete U112 line 156. (Its Card is C6.)
- `apps/kira-space/frontend/mobile/screens/TerminalScreen.vue:151`: `DetailHeader as="h1"`;
  `#leading` back button; `#meta` status; `#actions` font buttons. The hand `px-2 py-1` goes.
  Delete U112 line 157.

Exceptions (no change, rationale goes into ARCHITECTURE in C8):
- `packages/git-ui/src/components/CommitMeta.vue`, `StashDetailPane.vue`, `WorkingDetailPane.vue`
  keep `ViewToolbar data-testid="detail-head"`: the head sits in a toolbar row that must align
  with the sibling graph toolbar (`repo-commit-meta.spec.ts:87` asserts `detail-head` and
  `git-view-head` both 34px). DetailHeader (control-lg plus py-2) cannot meet it.
- `apps/kira-space/frontend/src/studio/api/EnvironmentsView.vue` keeps its `ViewToolbar`
  view-head: the same toolbar-head pattern is shared by 13 Studio tab views (`colorMarkClass
  ('band')` strip); converting one breaks that set.

### C4 `refactor(ui): section titles to SectionHeading and FieldLegend (M21)`

Read-only sections, `<SectionHeading flush :label="…" />` (drop the h3, keep surrounding testids):
- `apps/kira-space/frontend/src/ade/v2/port/AdePortResult.vue:14` (line 163).
- `apps/kira-space/frontend/src/ade/v2/settings/AdeSettingsResult.vue:51` (line 164).
- `apps/kira-space/frontend/src/ade/v2/settings/AdeSettingsSecrets.vue:75` (line 165).
- `apps/kira-space/frontend/src/studio/api/ApiAgentAccess.vue:28` (line 167).
- `apps/kira-space/frontend/src/studio/api/ApiSecrets.vue:77` (line 168).
- `packages/workbench/src/memory/distill/DistillResult.vue:180` (line 174).
- `packages/workbench/src/memory/distill/DistillRewriteResult.vue:76` (line 175).

Form groups, `FieldSet` + `FieldLegend` (Part 3 shape):
- `apps/kira-space/frontend/src/ade/v2/workflows/statuses/AdeTaskStatuses.vue:257`: h4 inside a
  `Field` becomes the legend of a `FieldSet` wrapping the defaults' fields (line 166).
- `packages/workbench/src/automations/RunSecretsSection.vue:39` (line 172).
- `apps/kira-space/frontend/src/ade/v2/port/AdePortExportDialog.vue`: files section (136,
  checkboxes) FieldSet + FieldLegend; env (211) and skipped (233) sections `SectionHeading flush`.
  Env lines `<pre … data-testid="ade-port-env-lines">` become `<CodeBlock
  data-testid="ade-port-env-lines">` (RC.2; overflow handled by CodeBlock wrap). Line 162.

Lists:
- `apps/kira-space/frontend/src/ade/v2/later/AdeShelfSection.vue:51`: `<div class="shrink-0
  px-1.5 pt-3"><SectionHeading :label="…" /></div>` (heading text lines up with px-3 rows).
  Line 158.
- `apps/kira-space/frontend/src/ade/v2/needs/AdeAllSessions.vue:131`: group header div becomes a
  wrapper `div.px-1.5.pb-1` holding `<SectionHeading :label="g.title"><template #leading><span class="size-2.5 shrink-0
  rounded-kira-sm" :style="{ background: g.color }" /></template></SectionHeading>`. Line 159.
- `apps/kira-space/frontend/mobile/components/PhoneApprovals.vue:45`: `<div class="px-1.5
  pt-3"><SectionHeading :label="t('mobile.approvals.heading.title')"><Badge variant="count"
  data-testid="phone-approvals-count">{{ items.length }}</Badge></SectionHeading></div>`. The
  `Badge variant="count"` stays, so the PERMANENT U101 entry for this file stays live. Line 155.

After C4: `grep -c '^U112 .*P672 Part 6' scripts/check-ui-primitives.sh` = 0 and U112 reports no
offender.

### C5 `refactor(ui): key-value through KeyValueList (M22)`

- `apps/kira-space/frontend/src/hostview/IssueSidebar.vue:30`: drop `items-baseline gap-y-2`;
  `density="compact"` (sidebar). Line 178.
- `apps/kira-space/frontend/src/repo/RepoIdentity.vue:68`: hand `<dl>` becomes `KeyValueList`;
  drop dt classes and dd `m-0 min-w-0`; keep the `font-data` spans and testids
  (`repo-full`, `repo-path`, `repo-source`). Line 179.
- `apps/kira-space/frontend/src/studio/approvals/ProofReviewBody.vue:27`: ordinal list, not
  key-value: `li` becomes `flex gap-2` (ordinal `shrink-0`, text `min-w-0`). Line 180.
- `apps/kira-space/frontend/src/studio/views/httprequest/TimelinePane.vue:330`:
  `density="compact"`, drop the class; `font-data` on each data dd. Line 181. (Its Card is C6.)
- `packages/git-ui/src/components/CommitMeta.vue`: both `KeyValueList class="text-graph-sm"`
  (391, 404) become `density="compact"`; identity `<p>` lines `text-graph-sm` to `text-kira-sm`;
  drop `text-graph-sm` on the dd Button. Line 182.
- `packages/theme/src/components/TooltipDetails.vue`: hand `<dl>` becomes `KeyValueList
  density="compact"`; keep `div.contents` rows with `tooltip-details-row` and `data-label`; drop
  per-dt/dd classes (descendant selectors from C1 cover them). 16 callers unchanged. Line 183.
- `packages/workbench/src/approvals/ApprovalDetails.vue`: `KeyValueList columns="fixed"
  density="compact"`, drop the class (`items-baseline` now in base; dt muted replaces subtle).
  Line 184.
- `packages/workbench/src/approvals/ApprovalPairs.vue`: Collapsible stops being the grid; inside
  it `KeyValueList` (default density, `font-data` on each dd) with the `div.contents` rows; the
  `approval-more` trigger moves after the list (no `col-span-full`). Line 185.
- `packages/workbench/src/components/OpLogDetail.vue:60`: `KeyValueList density="compact"`.
  Line 186.
- `packages/workbench/src/memory/MemoryStart.vue:101`: `KeyValueList density="compact"` (drop
  `items-center`). Line 187.
- `apps/kira-space/frontend/src/studio/kubernetes/components/DescribeView.vue:173`: drop dd
  `text-kira-sm` (RH.4 type from density).

After C5: no `P672 Part 6` line left.

### C6 `refactor(ui): cards by job, code as CodeBlock, rows out of cards (M23)`

ADE (`apps/kira-space/frontend/src/ade/v2/`), AdeTaskCard untouched (D1):
- `agents/AdeLeasesPane.vue:71`: section Card with `flex flex-col gap-2 p-3`, title q.name and
  Remove button becomes `DetailSection :title="q.name"` with `#actions` Remove (same testids),
  body in its CardContent.
- `details/AdeProofEntry.vue:46`, `details/AdeTaskChain.vue:117`, `notes/AdeNoteEntry.vue:68`,
  `review/AdeWorkflowReviewPanel.vue:204`: drop `flex flex-col gap-* p-2 text-kira-md` (inset
  gives padding and gap-1.5; md is the body default).
- `dialog/AdeAgentDialog.vue:282`: `CodeBlock class="text-muted-foreground"`.
- `panel/AdeStepBlock.vue:217`: `variant="list"`, drop `gap-0 py-1`.
- `run/AdeRunDialog.vue:221,229`: `CodeBlock class="break-words"`; `:239` `CodeBlock`.
- `workflows/AdeStepInspector.vue:287`, `workflows/inspector/AdeStepAgentFields.vue:122`: plain
  `Card` holding the VarText preview in `FieldDescription` (sm subtle look moves off the Card).
- `workflows/inspector/AdeStepAgentFields.vue:94`: `CodeBlock`.

Shell, Studio, Settings, Mobile:
- `apps/kira-space/frontend/mobile/screens/PairScreen.vue:66`: drop `flex flex-col gap-1`.
- `apps/kira-space/frontend/src/hostview/CommentComposer.vue:81`: section keeps `relative`;
  content into `<CardContent class="flex flex-col gap-2">`. `:103` keeps `min-h-16`.
- `apps/kira-space/frontend/src/studio/project/FiltersDialog.vue:233,276`: list cards keep
  `max-h-56 overflow-y-auto` only (drop `flex flex-col gap-px`; variant gives flex-col p-1).
- `apps/kira-space/frontend/src/studio/project/connection/ConnectionMaskReview.vue`,
  `ConnectionPrivacyTab.vue`: inner `div.flex.flex-col.gap-2.px-3.pb-3` becomes `<CardContent
  class="flex flex-col gap-2">`.
- `apps/kira-space/frontend/src/studio/project/connection/PrivacyOverview.vue` (`gap-1`),
  `studio/views/httprequest/TimelinePane.vue:250` (`gap-0.5`): drop the class.
- `apps/kira-space/frontend/src/studio/workbench/settings/ApiAccessRulesSection.vue:82`,
  `McpExposureRow.vue:23`, `StudioMcpConnectionsPane.vue:53`: Studio list card shape. Each `ul`
  (ApiAccessRulesSection, StudioMcpConnectionsPane, and the `ul`s in `KubeMcpSection.vue`,
  `ScriptsMcpSection.vue` that hold `McpExposureRow`) wraps in one `Card variant="list"`; the
  row Card becomes the `rowVariants({ layout: 'double' })` element (in McpExposureRow its root
  becomes a `div` with the row classes; the `li` wrapper in callers stays). Keep testids. Drop
  `Card` imports left unused.
- `apps/kira-space/frontend/src/workbench/settings/CliToolsPane.vue:70`: drop `flex flex-col
  gap-2 p-3` (inset, text and Badge only).

Workbench:
- `packages/workbench/src/automations/form/ScheduleFields.vue:102`: holds Input, NativeSelect,
  Checkbox, so `variant="section"` (ARCHITECTURE line 787); children in `<CardContent
  class="flex flex-col gap-3">`; drop `gap-3` from Card; keep `schedule-fields` testid on Card.
- `packages/workbench/src/automations/form/ParamsEditor.vue:163` (`param-row`): holds Input,
  Checkbox, SecretInput, so `variant="section"`, children in `<CardContent class="flex flex-col
  gap-1.5">`.
- `packages/workbench/src/automations/run/RunPreview.vue:40`: drop `text-kira-sm` (put
  `text-kira-sm` on inner meta spans only if they are meta).
- `packages/workbench/src/memory/AddMemoryDialog.vue`, `MemoryStart.vue` (`gap-0.5`),
  `import/ImportJobDetail.vue` (`gap-0`): drop the class.
- `packages/workbench/src/memory/settings/AgentMcpSection.vue:44`, `CheckHookSection.vue:32,54`:
  `CodeBlock class="select-all"`.

Unchanged: `AdeReviewRemoved`, `ExplainResultView`, theme `CodeBlock`, `DetailSection`,
`ScriptProgress`, `AdeTaskCard`.

### C7 `chore(lint): guard U124 for card classes`

`scripts/check-ui-primitives.sh`: header comment after the Part 5 line:

```
# Guard U124: docs/v0.3.17/plans/P672-part6.md.
```

Row after U123:

```
U124@@v@@CodeBlock.vue@@Card class sets padding, gap, type or row layout (variant inset, section or list; code is CodeBlock; a row is never a Card, RC.1-RC.3)@@<Card\b(?&t)\s:?class="[^"]*${TOK}(?:font-data|whitespace-pre|flex-row|justify-between|p-|px-|py-|pt-|pb-|gap-|text-kira|text-muted|text-subtle)
```

Dry run at `6e78c7e42` (`grep -rlPzo` equivalent of the pattern): 27 files, the 26 C6 files with
a class plus `CodeBlock.vue` (exempt, keeps the exempt live). `<CardContent`, `<CardHeader` do
not match (`\b` after `Card`). AdeTaskCard's class has none of the tokens. After C6: zero
offenders. If the dry run after C6 shows any hit, fix the file in C6 (amend nothing; follow-up
commit before C7), never a TEMPORARY line.

Rejected guard: DetailHeader-shaped hand header (flex row with icon 16 and lg title); markup
shape varies too much for a regex without false hits. U112 plus RH.3 hold it.

### C8 `docs: architecture for P672 Part 6`

`docs/ARCHITECTURE.md`:
- line 1460 (heading roles): DetailHeader sentence becomes "DetailHeader titles a view or detail
  pane (icon 16 muted or `#leading`, h2 lg 500 or `as="h1"` on a mobile screen, `#meta`,
  right-aligned `#actions`, `#description`, `#eyebrow`; `wrap` for long titles, `data` for a
  data name, `#control` for an inline-edit title). A detail head inside a toolbar row aligned with
  a sibling toolbar stays ViewToolbar (git detail-head, Studio tab view-head)."
- line 1461 (key-value): append "Rows align on baseline; `columns="fixed"` gives a 72px key
  column for aligned approval details."
- line 1462 (cards): append "Card inset with an Input, Select or Checkbox is a Card section.
  U124 fails a Card class that sets padding, gap, type or row layout."
- line 1435: `U83-U123` to `U83-U124`.
- line 1470: `U110-U123` to `U110-U124`, `U118-U123` to `U118-U124`.

### C9 `docs: P672 Part 6 done`

`docs/v0.3.17/SPEC.md` Part 6 row: Status Done; Result names commits C1-C8 (and any spec/fix
commits), guard U124, the 34 baseline lines removed (31 left), files touched, specs run, visual
baselines for CI to re-record. Last commit of the phase.

Any spec update or fix commit lands before C8.

## Test split

UI-only phase (no IPC, no Go, no flow test change). Frontend half: existing UI specs, updated only
where a selector or asserted look changed. No new unit test (no complex logic). No new spec:
guards U112, U113, U124 are the regression check.

Spec updates expected:

- `studio/docker-module.spec.ts:373,378`: heading level 2 in the resource header and level 3
  "Details" hold; recheck after the title `flex-1` change. Container detail specs on
  `docker-detail-name` now read the h2 (same testid, same text).
- `mobile/copy-wording.spec.ts:25`: level 1 in `pair-screen` holds via `as="h1"`.
- `proof-window.spec.ts`: description not clipped, no ellipsis, larger than `proof-title`.
- `approvals-alignment.spec.ts:346-354`: dt x = title x + 12, dd x = title x + 84, baselines
  equal; holds via `columns="fixed"` and base `items-baseline`.
- `repo-commit-meta.spec.ts:87`: unchanged head; KV text size changes, any font-size read on
  identity lines updates to kira-sm (never drop the assertion).
- ADE panel specs: `ade-panel-actions` now also holds branch and archived actions. Specs that
  locate `ade-panel-back`, `ade-archived-restore` or branch action testids by page or panel scope
  hold; any locator scoped under `ade-panel-head` retargets to the panel.
- `ade-v2-archived.spec.ts`: a hover on the title expecting a tooltip goes (title wraps instead);
  retarget to the visible full title.
- `window-chrome` / `status-agents` / `agent-usage` / `update-dialog`: `tooltip-details-row`
  rows and `data-label` unchanged.

Targeted run near phase end (one pass, from `apps/kira-space`), specs touching changed testids or
layouts:

- ADE: ade-v2-panel, ade-v2-task-panel, ade-v2-archived, ade-v2-archive, ade-v2-task-flow,
  ade-v2-stage-resume, ade-v2-panel-forward, ade-v2-later, ade-v2-leases, ade-v2-automations,
  ade-v2-workflows, ade-v2-workflow-steps, ade-v2-port, ade-v2-settings, ade-v2-force-push,
  ade-v2-base-rebase, ade-v2-review, ade-v2-notes, ade-v2-proof-chain, status-agents.
- Shared: approvals-alignment, repo-commit-meta, repo-review-interaction, repo-workspace,
  settings-llm-prompts, automations-recurring, automations-scripts, memory-distill,
  memory-import, operations, issue-view, host-view, window-chrome, agent-usage, update-dialog,
  ui-leftovers, consistency.
- Studio (under `studio/`): docker-module, proof-window, api-agent-access, api-secret-reveal-
  isolation, connection-privacy, http-request, http-variables, kubernetes, db-mcp settings specs
  (`settings-db-mcp*`), api-access-rules.
- Mobile (`tests/mobile/`): copy-wording and the approvals and terminal specs.

Implementer greps `tests/` for every changed testid and file name first and adds any spec found
to this run.

Visual baselines for CI to re-record (PR CI owns `visual`): `branch-panel-details`,
`plan-branches-review`, `docker-*`, `proof-*`, `approvals-dialog`, `approvals-toast`,
`connection-privacy-review`, `memory-distill-result`, `review-step-panel`,
`review-uncommitted-only`, `runs-restore-dialog`, `settings-memory`,
`status-bar-agents-tooltip`, `workflow-graph-review`, plus any ADE panel, mobile or settings shot
the run flags.

Sandbox lessons for spec edits:

- Playwright click on a menu item inside Monaco needs `locator.hover()` first.
- reka radio arrow presses need `keyboard.press('ArrowDown', { delay: 40 })`.
- N17 (`scripts/check-conventions.ts`) forbids the `dialog` variant on a delete/discard label
  without an allow marker; buttons moved into `#actions` keep their variants unless C2 says
  otherwise, and AdeLeasesPane Remove keeps its current variant.
- N10 caps `.vue` files at 600 lines: watch `ReviewView.vue` (598), `AdeBranchPanel.vue` (569),
  `CommitMeta.vue` (481).
- U91 exemptions go stale when their pattern vanishes: removing the AdeArchivedPanel title
  tooltip removes no U91 pattern there (none listed); recheck the guard output for any stale
  exempt after each commit.

## Risks

- `items-baseline` in the KeyValueList base changes every list: a dd holding only an icon or a
  block aligns on its bottom edge. Scan the 15 hosts (AdeBranchTip, DescribeView, AboutPane,
  ContainerOverview, ContainerSizeSection, EngineOverview, ResourceDetail, StatsView plus the C5
  hosts) in the targeted run; a block-only dd gets `self-start` on that dd (placement, not a list
  class).
- `[&_dt]`/`[&_dd]` descendant selectors reach nested `dl`s inside a dd. None today; a nested
  KeyValueList gets the same styles anyway.
- Moving branch and archived actions into PanelBar `#actions` with flex-wrap: many buttons wrap
  to a second bar line in a narrow panel. PanelBar already wraps (`flex-row flex-wrap`); verify
  in `ade-v2-panel` and visual baselines.
- AdeAllSessions uppercases the task title via SectionHeading. RH.1 places a list section title
  there; accepted, noted in Result.
- ScriptRunView restructure moves scrolling from root to body; a spec reading root scroll would
  break. None found; recheck in targeted run.
- ContainerDetail second line colour moves from muted to subtle (description look).

## Out of scope

- `AdeTaskCard` and every plan board visual (D1).
- Tones (`text-tone-*`, toned icons, button tones): Part 7. Icon sizes and type roles beyond
  the headers here (ADE workflow row names lg, editor `p-4`): Part 9.
- Converting the 13 Studio tab view-heads or the git ViewToolbar detail-heads (exceptions above).
- Any i18n, behaviour or IPC change.
