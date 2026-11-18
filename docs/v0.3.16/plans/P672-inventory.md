# P672 inventory: UI elements and uniformity rules

Spec row: `../SPEC.md` P672 Part 1. Plan: `P672.md`. Base: `chapter/v0.3.16` at `6527aec4`.
Analysis only. Status: in progress (sections land as written; see "Sections" for what is done).

## 0. Method and scope

- Sources: every `.vue` under `apps/` and `packages/` except `packages/theme/src/components/ui/`
  (the shadcn-vue primitives themselves), `tests/`, `testdata/`, `node_modules/`. 716 files.
  Per module: ADE 163, Studio (DB views, project tree, connections, proof, approvals) 161, Git
  (git-ui + `repo/` + `views/repo/`) 79, Workbench shared 75, Docker 40, Runs (automations) 38,
  Theme composites 28, Shell 27, API 27, Settings 22, Memory 21, Mobile 15, Kubernetes 14, Proto 6.
- Discovery: `codegraph_explore` on every primitive and composite named below (source, callers,
  blast radius), plus module walks (ADE, Runs, Studio, API, Docker, Kubernetes, Git, Memory, shell).
- Counts: template scans (tag + attribute parse, quote-aware) over the 716 files; class tokens
  from static `class="…"` and quoted literals inside `:class`. Class strings built in `.ts`
  (cva tables, ADE `tones.ts`) are read by hand. Counts are uses (tags), files in parentheses.
- Existing contract this inventory extends, never contradicts: `docs/ARCHITECTURE.md` "UI
  architecture" and "UI leftover conventions (P267)", `packages/theme/src/tokens.css`,
  `scripts/check-ui-primitives.sh` U1-U109 (dialog structure, button variants/sizes, weights 400/500,
  icon sizes 12/13/16/24, density scale 0.5/1/1.5/2/3/4, tone vocabulary ok/warn/error/info, no
  filled buttons, Tooltip not title, EmptyState/InlineState, rowVariants, z ladder, …).

Finding in one line: the primitive layer is already uniform (lint holds it). The divergence the
user sees sits one level up, in *composition*: which primitive a surface picks for the same job,
module-local wrappers that restyle it, free-form spacing/typography around it, and two parallel
colour vocabularies. Lint cannot see those; the rules below name them.

Rule ids: `R<family>.<n>`. Migration ids: `M<n>`, each tagged with the Part that applies it
(`P672.md` "Parts 2..n").

## 1. Baseline: what the primitive layer already fixes

| Concern | Fixed by | Holds? |
|---|---|---|
| Dialog shell | `DialogContent size` sm/md/lg/xl/2xl/full, Header/Body/Footer class strings (`ui/dialog/index.ts:18-20`), U1-U3, U16, U100, U107 | yes: 62/62 dialogs use all four parts |
| Buttons | `buttonVariants` 9 variants, 10 sizes (`ui/button/index.ts`), U7, U8, U83, U84, U92, U106 | yes at variant level |
| Weights | 400/500 only, U11 | yes (2 `font-bold` are ANSI log data) |
| Icon sizes | 12/13/16/24, U12 | yes |
| Tones | ok/warn/error/info (Badge, Alert, StatusDot), U26 | **no in ADE** (§9) |
| Density | 0.5/1/1.5/2/3/4, U88 | yes for step values; not for *which* step (§13) |
| Colours | tokens only, U34 | yes for literals; two token vocabularies (§12) |
| Empty/loading/error | EmptyState, InlineState, QueryState, Spinner, U14, U25, U54, U74 | yes for primitive choice; shape free (§10) |
| Rows | `rowVariants` menu/tree/double/nav, `useRowHeight`, U18, U19, U37, U98 | partly (§6) |
| Headings | PanelHeader, SectionHeading, FieldLegend, `text-kira-lg font-medium`, U21, U94 | partly (§7) |

## 2. Dialogs

### 2.1 Inventory

62 `DialogContent` in 62 files, plus `CommandDialog` (palette) and `TextPromptDialog` (own
scrim). Sizes: md 24, lg 14, sm 9, xl 7, 2xl 7, full 1; `fixed-height` 11. Per module:

| Module | sm | md | lg | xl | 2xl | full |
|---|---|---|---|---|---|---|
| ADE | 3 | 1 | 4 | 1 | 3 | |
| API | 1 | 2 | 1 | 2 | 1 | |
| Studio | | 4 | 2 | 3 | 3 | |
| Git | | 15 | | | | |
| Runs | | 1 | 3 | | | |
| Memory | | 1 | 1 | | | |
| Docker, Kubernetes | | | 2 | | | |
| Shell, Workbench, Mobile | 5 | | 1 | 1 | | 1 |

Footer button sequences (left to right, `Close>` = `DialogClose` wrapper):

| n | sequence | verdict |
|---|---|---|
| 31 | dialog, dialog-primary | canonical |
| 3 | Close>, dialog, dialog-primary | canonical (same look) |
| 3 | dialog, dialog-danger | canonical destructive |
| 2 | dialog, dialog-approve | canonical approval |
| 1 | dialog-deny, dialog-approve | canonical approval |
| 3 | dialog (single dismiss) | canonical info dialog |
| 2+1 | dialog-danger, dialog-primary (+Close>) | two emphasised buttons: violates "one emphasised button" (P267) |
| 1 | dialog, dialog-primary, dialog-danger | danger right of primary: order flip |
| 1 | dialog-primary, dialog, dialog, dialog-primary | primary first |
| 7 | multi-step footers (2-8 buttons, step-dependent `v-if`) | review per dialog (wizards) |

Header extras (content in `DialogHeader` beyond title): `AdeAgentDialog`, `NewConnectionDialog`,
`ResetDialog`, `ApprovalsDialog`, `ScheduleConfirmDialog`, `RestoreDialog`. `DialogDescription` in 14
of 62 (the rest put intro text as `FieldDescription` or a `<p>` in the body). Footer `#start` slot
in 6. Errors: `Alert` in body in 45, `FieldError` in 19 (mixed in some). `dirty` guard 39,
`destructive` 6. No footer: `AdeRunSandboxBadge` (info popout), `CredentialsUpdateDialog`,
`SettingsShell` (nav dialog).

Body composition varies most: `Field` stacks (45), `FieldSet`+`FieldLegend` sections (5), `Card` boxes
(4), `SecondaryTabs` inside body (9), `h3 text-kira-lg` section titles inside body (ADE port export,
ADE settings secrets/result, API secrets, Runs secrets, Memory distill: 9 uses, §7). DialogBody
class overrides: `gap-1.5`, `flex flex-col gap-2`, `flex flex-col gap-3` (redundant), `gap-0 py-0`,
`min-h-80`, `flex-row` (7 of 62).

Dialog-like surfaces that are not dialogs: `AdeAddPopover` (form in a popover, footer-like row),
`AdeTaskBranchForm` (form in popover), `ClusterUnavailable`, inline edit panes. Covered in §4 and §5.

### 2.2 Canonical rules

- **RD.1 Size by content, one table.** sm (400px): confirm, single field, prompt. md (480px):
  form up to ~8 fields, one column. lg (600px): form with sections or side-by-side fields,
  list picker. xl (720px): two-pane content, diff, preview. 2xl (900px) + `fixed-height`: editor,
  multi-tab config. full: review windows only. A dialog not matching its row moves (list in
  appendix A.1 marks each).
- **RD.2 Footer order** `[#start: secondary/tertiary actions, status] … [dismiss] [alternate] [primary]`.
  Dismiss (`dialog`, `data-dialog-dismiss`) left of the emphasised one; exactly one emphasised button
  (`dialog-primary`, `dialog-danger` or `dialog-approve`), always rightmost. A second action that is
  not the primary is `dialog`. `dialog-deny`/`dialog-approve` pair only for approvals.
- **RD.3 Header** is title only, plus `DialogDescription` when the dialog needs one line of
  context. Mode switches (`SecondaryTabs`) go to the top of `DialogBody`, never into the header.
  Badges/status go into the description line or the body. Close X stays (U16, closable).
- **RD.4 Intro text** under the title is `DialogDescription`, never a body `<p>` or
  `FieldDescription` as the first body child.
- **RD.5 Body** keeps `dialogBodyClass` (gap-3, p-3). No `DialogBody` class except `flush` and
  `min-h-*` for fixed editors. Sections inside a body are `FieldSet`+`FieldLegend` (forms) or
  `SectionHeading` (lists), never `h3 text-kira-lg` (RH.3).
- **RD.6 Errors**: operation failure is `Alert variant="error"` as the last body child; field
  validation is `FieldError` under the field. Footer `#start` may hold a one-line `FieldError`
  status only when the body scrolls (existing P267 rule, restated).

## 3. Buttons and icon buttons

### 3.1 Inventory

`Button` 688 (311 files); `TooltipIconButton` 300 (160); `SplitButton`; `MenuIconButton`.
Variants: dialog 229, toolbar 214, dialog-primary 112, link 80, dialog-danger 23, dialog-approve
11, title 6, dialog-deny 5, dynamic 8. Sizes: kira-lg 286, kira 238, inline 92, kira-icon 21, touch
16, status 15, title-labelled 6, touch-icon 4, title 2. `TooltipIconButton` size: default (kira-icon)
283, title 9, inline 4; variant default (toolbar) 286.

ADE action buttons: `ACTION_CLASS` (31 uses, `ade/v2/tones.ts:35`) adds tone outlines (amber, red,
green, blue, purple, grey, agent) over `Button variant="dialog"`, so ADE task actions are coloured
outlines while every other module's row/toolbar actions are plain `toolbar` buttons.

Variant x size x host (outside dialog footers):

| Module | toolbar@kira | dialog@kira | dialog@kira-lg | toolbar@kira-lg |
|---|---|---|---|---|
| ADE | 19 | 41 | 45 | 4 |
| Studio | 46 | 14 | 28 | |
| Git | 34 | 9 | 9 | |
| Runs | | 1 | 22 | |
| Memory | 15 | 3 | 7 | |
| Settings | | | 10 | |
| API | 6 | 3 | 8 | |
| Docker | 20 | 1 | | |
| Kubernetes | 2 | | | |

So an in-view action has three looks: borderless `toolbar` (Docker, Git, Studio views), small
outline `dialog@kira` (ADE task panel, branches, notes, banners: `AdeMainTab`, `AdeBranchPanel`,
`AdeStepBlock`, `AdeWorkflowBlock`, `RestoreBanner`, `FailureBanner`, `CookiesPane`, …) and large
outline `dialog@kira-lg` (Runs editor tabs, Settings panes, ADE settings/workflows, Studio connection
tabs). ADE alone splits 41/45 between the two outline sizes. `dialog-primary@kira` 16 in Studio
query/console bars (`QueryBar`, `ConsoleView`, `FilterEditor`, `StreamToolbar`, `KeyValuePane`,
`DocumentEditor`, `GrpcRequestView`) as the run/apply button. `toolbar@kira-lg` 4 in ADE
(`AdePlanView`, `AdeSessionStrip`, `AdeSessionsMenu`). `link@kira` 3 (`AdePlanView`,
`AdeStepStatus`, `ShowMoreButton`) where `link` is otherwise `inline` (77).

### 3.2 Canonical rules

- **RB.1 Size by host, not by module.** Form commit (dialog footer, form pane save bar, a form
  field's own action such as Browse/Test): `kira-lg`, matching the 26px controls (ARCHITECTURE
  "Dialog and form controls are size kira-lg"). Any bar (view toolbar, panel header, query bar,
  strip, banner, `Alert` action row), row and popover action row: `kira` text or `kira-icon`
  (`TooltipIconButton`). Status bar `status`; mobile `touch`/`touch-icon`; `link` is always
  `inline`.
- **RB.2 Variant by role.** Outline (`dialog*`) only commits a form or answers a question (dialog
  footer, save bar, confirm/approve strip, banner decision such as Restore/Dismiss, query Run in a
  query bar: `dialog-primary@kira`). Every other in-view action, including labelled ones in a detail
  header or task panel section, is `toolbar` (`kira`) with a leading icon. Text action in prose,
  alert or empty state: `link@inline`.
- **RB.3 ADE `ACTION_CLASS`** stays only for the plan board and task card primary step action
  (the documented board visual, P263 §2.10). Elsewhere in ADE (task panel, settings, workflows,
  agents, repos pages) actions follow RB.1/RB.2.

## 4. Forms: labels, fields, help, validation

### 4.1 Inventory

Label primitives: `Label` 148 (80 files) vs `FieldLabel` 92 (37). `FieldLabel` =
`Label` + `text-muted-foreground` (`ui/field/FieldLabel.vue:12`); bare `Label` renders `text-fg`
`font-medium`. Who uses which:

| Module | FieldLabel | Label |
|---|---|---|
| Git | 37 | 2 |
| Studio | 26 | 50 |
| Runs | 20 | 12 |
| API | 4 | 3 |
| ADE | 2 | 31 |
| Settings | 0 | 34 |
| Memory | 0 | 11 |
| Kubernetes, Docker | 2 / 0 | 1 / 1 |

Label beside an input (not a checkbox/switch): ADE 16, Studio 12, Settings 4, Memory 3, Git 2 use
bare `Label` (white) where Git/Runs/Studio forms use `FieldLabel` (muted): **the same "field
label" renders in two colours.** `Label` class overrides add a third/fourth look: `text-muted-foreground`
8, `text-kira-sm text-muted-foreground` 4, `text-kira-sm` 2, `text-kira-sm font-normal
text-muted-foreground` 2 (27 class variants in 148 uses).

Module-local label layouts (all hand-rolled, each different):

| Wrapper | Where | Label look | Layout |
|---|---|---|---|
| `Field` (vertical) | 131 uses | FieldLabel muted md | label above, gap-1.5 |
| `Field orientation="horizontal"` | 32 | FieldLabel after checkbox | inline |
| `AdeInspectorRow` | ADE workflow inspector, 24 | `Label text-kira-sm muted` | 7.5rem label column at `@24rem`, else above |
| `AdeRow` label slot | ADE task panel, 15 | `ADE_ROW_LABEL` w-24 | label column 96px |
| `EditField` | Docker edit, 25 | `<span text-muted-foreground>` (not a label) | above, gap-1 |
| `CheckboxRow` | Studio connection, 9 | `Label` wrapping Checkbox `size-3.5` + inner CodiconIcon | inline, description wraps |
| Settings fields (`settings/fields/*`) | 22 panes | bare `Label`, md fg | above |
| Repo config (`RepoConfigForm`) | Git | FieldSet gap-3 + FieldLabel | above |

Help text: `FieldDescription` 310 (`w-full` override 41: used inside horizontal flex rows to force a
wrap). Validation: `FieldError` 133 with `useFormErrors`. Hand error text: 42 `text-error`
spans/divs (Studio 15, Runs 7, ADE 5, Docker 4, Workbench 3, …).

Controls: `Input` 134 (size default 93, explicit `kira-lg` 20, `kira` 18; the default is already
`kira-lg`, `ui/input/index.ts:25`, so explicit `kira-lg` is noise; `kira` is right in bars
(`SchemaBrowser`, `DateTimePicker`, `PagerControls`, `StreamComposeMessage`, `InlineEditInput`) but
sits in forms in `RequestSettingsPane` x3 and the ADE task panel `AdeEstimateField`,
`AdeWorkflowBlock`, `AdeLinkAdd`, `AdeLinkEdit`, `AdeStatusName`); `NativeSelect` 55 (`variant
bordered` 46, none 9); `Textarea` 33 (16 class variants: `min-h-12`, `min-h-24`, `min-h-32`,
`max-h-90`, `field-sizing-fixed`, `resize-y/none`); `Checkbox` 78 (`size-3.5` override 17 although
the primitive box is 14px); `Switch` 30; `SecretInput` 9; `NumberStepperInput` 19; `AutocompleteField` 10.
Field width: free (`w-24`, `w-40`, `w-48`, `w-20`, `w-32`, `w-36`, `flex-1`, `basis-24`, `flex-2`).

### 4.2 Canonical rules

- **RF.1 One field label.** Every visible label of a form control is `FieldLabel` (muted, md,
  500), including the text after a checkbox/switch in a horizontal `Field` (RF.3). Bare `Label`
  only for `sr-only` labels and for a checkbox in a bar or menu row outside any form (toolbar
  toggle, list "select all"). No class on either except `sr-only`, `font-data` (label is data,
  e.g. param name).
- **RF.2 Two layouts only.** (a) *Stacked*: `Field` (label above control, `FieldDescription` and
  `FieldError` below). Dialogs, popover forms, narrow panes. (b) *Columns*: label column beside
  control for property panes wider than 24rem (ADE inspector, Docker edit, ADE task panel
  properties, connection general tab). Both come from the theme: add `orientation="responsive"` to
  `Field` (label column 7.5rem at `@24rem`, stacked below), retire `AdeInspectorRow`,
  `EditField`'s label part, and `AdeRow`'s label column for editable properties. `AdeRow` keeps
  read-only lead/body/actions rows (§6).
- **RF.3 Checkbox/switch row**: `Field orientation="horizontal"` + control + `FieldLabel`, help in
  `FieldContent` > `FieldDescription`. `CheckboxRow` goes; no `size-*` class on Checkbox.
- **RF.4 Groups**: `FieldSet` + `FieldLegend` (+ `FieldDescription`). No gap override on `FieldSet`
  (its gap-4 between fields, gap-3 for checkbox/radio groups, is the rhythm).
- **RF.5 Control widths**: full width in stacked layout (`w-full` implicit). Fixed widths only for
  short typed values, three steps: `w-24` (port, count, number), `w-40` (short id, enum), `w-56`
  (name). Side-by-side fields share a row with `flex-1` each; never `flex-2`/`basis-*`.
- **RF.6 Textarea heights**: three rows (`rows="3"`), code/script `min-h-32 resize-y font-data`,
  prompt/body `min-h-24 resize-y`. Nothing else.
- **RF.7 Errors**: hand `text-error` text near a control becomes `FieldError` (with
  `useFormErrors`), a list line becomes `InlineState kind="error"`, a pane failure `Alert`.
- **RF.8 Checkbox indicator**: the primitive's own `CheckIcon` (`ui/checkbox/Checkbox.vue:34`).
  34 uses pass `class="size-3.5"` and/or a `<CodiconIcon name="check" :size="12" />` child (list
  A.6: Studio connection/MCP/filters/grid menus, API variables, Memory distill, Git file tree,
  Runs params, approvals). Both go; the box stays 14px everywhere.

## 5. Menus, context menus, popovers

### 5.1 Inventory

Menus are uniform at primitive level: `DropdownMenuContent` 24 uses, 0 class overrides (min-w-45
max-w-80 from the primitive); `DropdownMenuItem` 0 overrides; context menus all go through
`openContextMenuAt` (25 call sites, no `ContextMenuContent` tag in app code); `MenuItems` renders a
`MenuItem[]` in either. Divergence:

- Two ways to build the same dropdown: `MenuItems` from a `MenuItem[]` (4 hosts: shell, Git, Studio)
  vs hand `DropdownMenuItem` lists (22 files, ADE 12: `AdeSessionsMenu`, `AdeReviewActions`,
  `AdeTaskAddMenu` items, `AdeBoardMenu`, …). Context menus already use the data form; dropdowns
  mostly do not, so separators, icons (13 muted), shortcuts and disabled reasons differ by author.
- Coloured menu icons: Docker `ContainerList` passes `iconClass` `text-ok`/`text-error` for
  Start/Stop (2). Every other menu icon is muted.

`PopoverContent` (default `w-72 p-2.5 gap-2.5`): 41 uses, **24 class variants** (A.2). Two real
shapes hide behind them:

| Shape | Uses | Classes seen |
|---|---|---|
| list/sections popover (own header, scroll list, footer) | 27 | `p-0`, `gap-0 p-0`, `gap-0 overflow-hidden p-0`, `p-0 gap-0`, `p-1`, `gap-0 p-1`, `max-h-60 … p-0` |
| form popover (fields + action row) | 10 | default, `p-2`, `gap-1.5`, `w-auto p-2`, `text-kira-md` |
| bar/inline (Git title bar, autocomplete, hover panel) | 4 | own `px-* py-*`, z-index |

Widths: w-56 9, w-80 15, w-96 10, w-120 5, w-auto 4 (fits the w-56/80/96/120 ladder in
ARCHITECTURE). Padding inside list popovers then varies again per author (`p-1` list vs `px-1.5`
header vs `p-2` footer). `PickerPopover` (24 hosts) is the one shared list popover and already
shapes header/search/list/footer.

### 5.2 Canonical rules

- **RM.1 Data menus.** Every dropdown and context menu renders `MenuItems` from a `MenuItem[]`.
  Hand `DropdownMenuItem` only for an item kind `MenuItem` cannot express (checkbox/radio item,
  submenu with live content); name it in a comment.
- **RM.2 Menu icons** are 13, muted, never toned. Destructive item: `variant="destructive"` text,
  icon follows. Drop `iconClass` tones.
- **RM.3 Two popover shapes.** (a) *Form popover*: `PopoverContent` default padding/gap, fields
  per §4 stacked, action row is a right-aligned `toolbar`/`dialog-primary@kira` row (RB.1). (b)
  *List popover*: `PopoverContent flush` (new prop: `p-0 gap-0 overflow-hidden`, same idea as
  `DialogBody flush`/`CardContent flush`) with a header row `px-1.5 h-control` (SectionHeading
  look), a `p-1` scroll list of `rowVariants` rows, an optional footer `px-1.5 py-1` with
  `toolbar@kira`. A picker uses `PickerPopover`. No other padding classes on `PopoverContent`.
- **RM.4 Widths** stay the four steps `w-56/80/96/120` (plus `w-auto` only for bar popovers that
  size to a single control).

## 6. Lists, rows, trees, tables

### 6.1 Inventory

- `rowVariants` (menu/tree/double/nav + `rowMeta`, `rowIndent` 6+depth*14): 68 uses. Heights
  from `useRowHeight`. Holds well in trees (Studio project tree, Git file tree, Docker groups).
- Grid data: SlickGrid in Studio views and Git log; `Table` primitive 12 (Studio 10, Settings 1,
  Docker 1; 1 class override). `VirtualList` 4 (Docker); `useVirtualRows` for Memory/Runs lists.
- ADE row system is parallel: `AdeRow` (lead/body/actions, label column `ADE_ROW_LABEL` w-24) in
  the task panel and details tab; `AdeAllSessionRow`, `AdeBranchRow`, `AdeSessionsRow` hand rows with
  ADE tone text. Memory result rows: `rowHeight.double + 8` padding (own rhythm).
- Row meta: `rowMeta` (subtle, sm) vs hand `text-kira-sm text-muted-foreground` spans for the same
  trailing info (age, count, size) in Docker list, Git refs, Runs list, ADE rows. Muted vs subtle
  for the same role (P267 grey rule says *subtle* is for de-emphasised meta).
- Warning/error icon colour in rows and banners: `text-warn` 2, `text-tone-red` 2 (Runs
  `ScriptRow`, `ScriptBrokenRow`), `text-error` 3, muted 1 (Kubernetes `KindBar` partial warning),
  inherit 8 (§15).
- Selection: `bg-select` row state from `rowVariants`; ADE cards use own selected ring.

### 6.2 Canonical rules

- **RL.1 Rows come from `rowVariants`** (menu, tree, double, nav). A list row with lead icon,
  title, meta and hover actions is `row({ kind: 'double' | 'menu' })` + `rowMeta` for trailing
  meta. `AdeRow` stays only as the read-only property row of the ADE task panel (lead/body/actions);
  its label column folds into RF.2 for editable properties.
- **RL.2 Trailing meta** (age, count, size, branch) is `rowMeta` (subtle, sm, `font-data` for
  numbers/ids). Never hand muted spans.
- **RL.3 Tabular data** with sortable columns is SlickGrid (data views) or `Table` (short static
  lists ≤ ~50 rows). Lists of entities are rows, not tables.
- **RL.4 Row status icon** follows RS.1 tones (`text-warn` warning, `text-error` failure), never
  ADE tones outside the board (§9), never muted for a warning.

## 7. Headings, section headers, detail headers, key-value

### 7.1 Inventory

Header primitives and their look:

| Role | Component | Look | Uses |
|---|---|---|---|
| panel title bar | `PanelHeader` | h-bar, px-1.5, sm, **muted**, uppercase, tracking-wider, 500 | 19 (ADE 10) |
| sub-section label in a panel/list | `SectionHeading` | h-control-sm, px-1.5, sm, **subtle**, uppercase | 54 |
| form group label | `FieldLegend` | sm, subtle, uppercase | via FieldSet |
| card header | `CardTitle` in `Card section` | h-control px-3 | 3 (`DetailSection` Docker, Studio 2) |
| page/detail title | hand `h1`-`h4` `text-kira-lg font-medium` | lg 500 fg | 24 (A.5) |

PanelHeader muted vs SectionHeading subtle is a deliberate two-step hierarchy (title of a panel
one step stronger than a section inside it). It stays; ARCHITECTURE gets the sentence.

Divergence:
- `h3 text-kira-lg font-medium` used as a *section* title inside dialogs/panes, competing with
  `FieldLegend`/`SectionHeading`: `AdePortExportDialog` x3, `AdePortResult`, `AdeSettingsResult`,
  `AdeSettingsSecrets`, `ApiAgentAccess`, `ApiSecrets`, `RunSecretsSection`, `DistillResult`,
  `DistillRewriteResult`, `AdeShelfSection`, `AdeAllSessions` (13). Same role, two looks:
  uppercase 11px subtle vs sentence-case 13px fg.
- Detail headers (top strip of a detail view: icon, title, status, actions) are hand-rolled in at
  least 11 places, each different: Docker `ContainerDetail:143`, `ResourceDetail:121`
  (`flex shrink-0 items-center gap-2 px-3 py-2`, icon 16); `ProofStepView:146` (`min-h-control-lg
  items-start gap-2 px-3 py-2`, icon 16 muted, title `text-kira-lg leading-snug`); `PromptDetail:79`
  (`header flex-col gap-1`, h3 + `dialog@kira` Reset + three FieldDescriptions); `ScriptRunView:83`
  (`flex items-center gap-2`, title `font-medium` at md, `dialog-primary@kira-lg` inline);
  `AdeTaskPanel` (title is an inline `Input text-kira-lg`), `AdeArchivedPanel:152`,
  `AdeBranchPanel:319` (h3 line-clamp-2), `AdeWorkflowsTab:169`, `AdeStepsTab:125`, Git `CommitMeta`
  (subject + facts + `px-3 py-2` action row), API `EnvironmentsView` (title inside a
  `ViewToolbar`, `text-kira-md`), Git `ReviewView:485` h2. Title size: lg in 8, md in 3; padding
  px-3 py-2 in 6, none in 3, pt-3 in 2; icon 16 in 3, none in 8.
- Key-value: `KeyValueList` (grid max-content/1fr, gap-x-3 gap-y-1, dt muted) 15 uses, 7 class
  variants (`text-graph-sm` 2, `text-kira-sm` 1, `py-0.5 text-kira-sm font-data`, `items-baseline
  gap-y-2`, a `72px` column with subtle dt). Hand KV grids beside it: `grid-cols-[auto_minmax(0,1fr)]`
  3, `[auto_1fr]` 2, `[max-content_1fr]` 1, `[7.5rem_…]` (AdeInspectorRow), `[72px_…]`; `AdeRow`
  label w-24 rows; Docker overview `UsageBar` rows. dt colour muted vs subtle across them.

### 7.2 Canonical rules

- **RH.1 Header roles fixed**: panel title `PanelHeader`; section inside a panel/list/popover
  `SectionHeading`; group inside a form `FieldSet`+`FieldLegend`; card title `CardHeader`/`CardTitle`
  on `Card variant="section"`; view/detail title `DetailHeader` (RH.2). No other heading markup.
- **RH.2 One `DetailHeader`** (new theme composite; the only new component this inventory asks
  for besides `Field orientation="responsive"` and `PopoverContent flush`): `flex min-h-control-lg
  shrink-0 items-center gap-2 px-3 py-2`, optional leading icon 16 muted, title `h2 m-0 min-w-0
  truncate text-kira-lg font-medium` (slot for an inline-edit title), `#meta` slot (badges, status,
  subtle sm facts), `#actions` slot right-aligned (`TooltipIconButton`/`toolbar@kira`, one optional
  `dialog-primary@kira` decision). Optional `#description` line below (FieldDescription look).
  Rationale: 11 hand copies, five paddings, two title sizes.
- **RH.3 Section titles** inside dialogs and panes are `SectionHeading` (list/read-only content) or
  `FieldLegend` (form groups). `h3 text-kira-lg` goes (13 uses).
- **RH.4 Key-value** is `KeyValueList` only, two densities: default (md, gap-y-1) and
  `density="compact"` (new prop: sm, gap-y-0.5) for sidebars/tooltips. dt always muted, dd fg,
  `font-data` per value when the value is data. No `grid-cols-[…]` override, no hand KV grids.

## 8. Cards

### 8.1 Inventory

`Card` 41 uses (ADE 15, Studio 10, Memory 6, Runs 3, Shell 2, Theme 2, Settings/Docker/Mobile 1),
variants default(inset) 28, section 7, list 6; **25 class variants** (A.3). Recurrent overrides:
`gap-0.5` 3, `flex-row items-center justify-between gap-1.5` 3 (card used as a row), code/blob
display `font-data whitespace-pre-wrap break-all select-all text-kira-sm leading-normal` 3 + 2
(card used as a code box: `CodeBlock` exists), `flex flex-col gap-2 p-3` 2 (re-pads inset), `max-h-56
flex-col gap-px overflow-y-auto` 2 (list card as scroll list). `CardHeader` 3, `CardContent` 1: almost
every card skips the parts and lays out by hand. Hand boxes (`rounded border p-*`) are banned by U36;
one exemption `ConnectionEnginePicker` (§17). ADE task cards (`AdeTaskCard`) are their own board
visual (P263), out of scope.

### 8.2 Canonical rules

- **RC.1 Three card jobs = three variants**: `inset` (one small block of facts/text inside a form
  or pane, own padding px-2.5 py-2), `section` (titled group with `CardHeader` + `CardContent`, the
  Docker `DetailSection` pattern, which moves to `packages/theme` as the shared titled section),
  `list` (scroll list of rows, p-1). Class overrides limited to layout of the card in its parent
  (`shrink-0`, `max-h-*`, `min-w-0`).
- **RC.2 Code/blob text** is `CodeBlock`, never a `Card` with `font-data` classes.
- **RC.3 A row is not a card**: `Card` with `flex-row justify-between` becomes a `rowVariants` row
  or an `Alert` (when it is a notice with an action).

## 9. Badges, chips, status

### 9.1 Inventory

- `Badge` 203 uses (Studio 81, ADE 37, Memory 19, Shell 16, Git 14, API 13), 22 class variants, all
  layout (`shrink-0`, `ml-auto`, truncation). Variants default/chip/ok/warn/error/info (tint /15),
  count (rounded-full). Holds.
- `AdeChip` 46 (ADE 42, Mobile 4): ADE tone chip (`TONE_TAG_CLASS`/`STATUS_CHIP_CLASS`, min-w-21),
  pastel tones amber/red/green/blue/purple/grey from `ade/v2/tones.ts` (dark-only, documented
  ARCHITECTURE:52, :2827). Same job as `Badge` tones.
- `StatusDot` 28. `SmartBadge` (Runs). `RunStatusBadge`: running = **info**. `RunState`: text tone +
  Spinner/StatusDot. `AdeOpStatus` (5 hosts: review panel, task card, branch row, session strip,
  sessions row): `Badge warn` + Spinner, so a running operation is **warn** in ADE and **info** in
  Runs. Docker `stateVariant`: running ok, paused/restarting warn, dead error, else default. Git
  `changeRequestBadgeClass` hand class per PR state.
- Two colour vocabularies: theme ok/warn/error/info (`--kira-ok` …) vs ADE `--kira-status-*`
  pastels (`text-tone-*` 39 in ADE, 4 in Runs; `bg-tone-*` 13; `border-tone-*` 4; A.7). ADE red
  (pastel) ≠ theme error. Inside ADE: `text-tone-red` 16 vs `text-error` 5 for the same "failed"
  meaning; ADE `Alert`s use theme tones (46 `variant="error"`).

### 9.2 Canonical rules

- **RS.1 One status-to-tone map** for every module: running / in progress = `info` (+ Spinner);
  succeeded / up / clean = `ok`; waiting / needs attention / partial / stale = `warn`; failed /
  broken / conflict = `error`; idle / stopped / draft / unknown = default (neutral). `AdeOpStatus`
  moves running from warn to info; Docker running stays ok (a *state*, not an operation: "up").
- **RS.2 Status display by density**: in a row or header, `Badge` with tone; in a dense list or
  tree, `StatusDot`; inline in prose, tone text. `RunState` stays as the run-specific composite of
  these.
- **RS.3 ADE tones** (decision D1 below): recommended (a) `AdeChip`/`text|bg|border-tone-*` stay
  only on the plan board and task cards (columns, task status chips, card accents: the documented
  board identity); every other ADE surface (task panel, sessions, settings, workflows, dialogs) and
  every non-ADE module uses theme tones and `Badge`. Runs `ScriptRow`/`ScriptBrokenRow`/
  `RunFileDialog` move to `text-error`/`text-warn`. Alternative (b): retire the ADE palette, map
  amber→warn, red→error, green→ok, blue→info, grey→default, purple→`chip` (no fifth theme tone is
  invented).

## 10. Empty, loading, error, toasts

### 10.1 Inventory

- `EmptyState` 112 uses (Studio 42, Git 30, Docker 9, API 7, Kubernetes 6, …), 14 prop-shape
  combinations: icon+fill 27, icon no fill 26, with action 13, title only, description only, …
  `#media` slot 12 (A.10): coloured media icons `text-warn` (Docker `ContainerDetail`), error tone
  (Kubernetes `ClusterUnavailable`), Spinner (loading states), grid-overlay icons. The same "load
  failed" state renders with warning icon muted (`ProofStepView`), warning `text-warn` (Docker),
  error icon `text-error` (Kubernetes), no icon (API).
- `InlineState` (px-1.5 py-1 sm subtle/error) for list-level empty/loading/error.
- `QueryState` wraps TanStack queries with `#loading`/`#error`/`#empty` slots; overridden ad hoc.
- `Skeleton` 12 uses, 9 class variants (`h-4 w-full`, `h-4 w-2/3`, `h-control-lg w-1/3`, `h-3 w-32`,
  `h-4.5 w-16 rounded-kira-sm`, `h-24 w-full`, …). Spinner 27 (text-info override 1).
- Loading text: `common.state.loading.loading` in InlineState vs `Spinner` + text in a `p-4 flex`
  div (Docker recreating).
- Toasts: 14 calls, ADE 13 (error 7, success 3, dismiss 2, loading 1), Studio 1 (error). Other
  modules report the same kind of outcome inline (`Alert`, FailureBanner, InlineState).

### 10.2 Canonical rules

- **RE.1 Which state primitive**: whole view/pane empty or failed: `EmptyState fill`; a list or
  section inside a view: `InlineState`; an operation failed while content stays: `Alert error`;
  a field invalid: `FieldError`.
- **RE.2 EmptyState shape**: `icon` prop (24, muted from the primitive) + `title` + optional
  `description` + optional one action (`toolbar@kira` or `link@inline`). `#media` only for a
  `Spinner` (loading) or an illustration that is the content (grid overlay). No toned media icons:
  failure uses `icon="warning"`, title `common.state.loadFailed.title`, the error text as
  description, Retry as the action.
- **RE.3 Loading**: `QueryState` default loading where the shape is unknown; `Skeleton` only where
  the final layout is known, three shapes: text line `h-4` (widths `w-full`/`w-2/3`/`w-1/3`), title
  `h-control-lg w-1/3`, block `h-24 w-full`.
- **RE.4 Toasts** only for the outcome of an action whose surface is gone or out of view (background
  job, closed dialog, board drag). An action whose surface is still visible reports inline. ADE's 13
  calls are checked against this; Studio's 1 likewise. No new toast sites elsewhere.

## 11. Tooltips

### 11.1 Inventory

`TooltipContent` 175 (default 174, surface 1). Wrappers: `TooltipIconButton` 300 (iconSize default
13), `TooltipDisabledTrigger` 23, `TooltipDetails` 8 and `AttributeTooltip` 8 (structured tooltip
body), `AdeTip` 50 uses / 56 callers (`ade/v2/AdeTip.vue`: Tooltip + Trigger + default
TooltipContent from a `text` or `VarText` `parts` prop, renders the bare slot when the text is
empty). Visually uniform: every variant lands on the same `TooltipContent default`. The only
divergence is API: ADE writes `<AdeTip :text>`, everyone else writes the three-tag triple (174),
and the "no tooltip when text is empty" guard is re-coded by hand outside ADE.

### 11.2 Canonical rules

- **RT.1** Plain hint: `Tooltip` + `TooltipContent` default. Icon button: `TooltipIconButton`.
  Disabled control with a reason: `TooltipDisabledTrigger`. Facts list: `TooltipDetails`.
- **RT.2** `AdeTip` moves unchanged to `packages/theme/src/components/TextTooltip.vue` (same
  props) and becomes the plain-text form for every module; ADE imports switch. Low priority: no
  visual change, folded into the Part that touches tooltips.

## 12. Colours

### 12.1 Inventory

- Text colour tokens: muted 511, subtle 225, fg 98, error 42; theme tones elsewhere via Badge/Alert.
  Muted vs subtle is the documented grey rule (P267: muted secondary text, subtle de-emphasised
  meta); breaches are role mixes (row meta muted, §6; KV dt subtle in one variant, §7).
- Backgrounds: bg-bg 43, bg-chrome 34, bg-elevated 23, bg-field 20 on the documented ladder.
- Raw colour literals: only `studio/theme/EngineIcon.vue` (31, vendor logo fills; legitimate).
- Second vocabulary: ADE tones (§9), leaking into Runs (4 lines).

### 12.2 Canonical rules

- **RCo.1** Status colour only from RS.1 tones; ADE pastels per RS.3.
- **RCo.2** Grey by role: primary content `fg` (default, no class); secondary text, labels, dt,
  icons `text-muted-foreground`; meta/de-emphasised, section labels `text-subtle`. No `text-fg`
  class on text that already inherits fg (98 uses mostly redundant; Part sweeps where touched).

## 13. Typography

### 13.1 Inventory

Sizes: `text-kira-sm` 522, `md` 184, `lg` 40, `xl` 8, `text-graph-sm/md` 26 (Git graph and
`KeyValueList` variants). Body default is md (12px) from the root; `text-kira-sm` is set by hand on
meta, labels, badges, help in dense bars. Weights: `font-medium` 127, `font-normal` 29 (seen
undoing `Label`'s 500, e.g. `ReviewResultOption:33`; a symptom of RF.1). `font-data` 364, `font-mono`
6 (should be `font-data`). `leading-*`: normal 24, none 8, relaxed 4, snug 2, `3.5`/`4` 4.
`tracking-*`: normal 6, widest 4, wider 3. `italic` 7. `uppercase` 3 by hand (beyond PanelHeader /
SectionHeading / FieldLegend).

### 13.2 Canonical rules

- **RTy.1 Size roles**: xl 20 = empty/landing hero only; lg 13 = detail/dialog title (DetailHeader,
  DialogTitle); md 12 = body, controls, labels (default, no class); sm 11 = meta, badges, section
  labels, row meta, help text inside dense bars. A whole pane at sm is not allowed.
- **RTy.2 Families**: `font-data` for data (ids, paths, values, code, numbers in tables);
  `font-mono` goes (6). Text in UI chrome is never `font-data`.
- **RTy.3 Leading/tracking** come from the primitive; `leading-normal` only on multi-line prose
  blocks, `leading-none` only on icon-aligned single lines. Hand `tracking-*` and `uppercase` only
  inside the three header primitives.

## 14. Spacing and alignment

### 14.1 Inventory

Gaps: gap-1 355, gap-2 255, gap-1.5 219, gap-0.5 162, gap-3 114. Padding: p-0 121, px-2 99, px-3
64, py-1 62, px-1.5 62, p-3 59, p-2 39, p-4 22. Pane roots (scrolling content views): p-3 9
(`AdeDetailsTab`, `ContainerOverview`, `EngineOverview`, `StatsView`, `ScriptRunView`,
`SettingsShell`, …), p-4 7 (`AdeLeasesPane`, `AdeContextPane`, `AdeSettingsPane`, `HostViewSidebar`,
`IssueSidebar`, `BootFailure`, `ModuleStart`), p-1.5 4, p-2 2, p-1 2. Primitives use the off-scale
2.5 step themselves (Card inset `px-2.5`, `PopoverContent p-2.5 gap-2.5`, `RadioGroupCard px-2.5`),
which U88 allows only because they live in `ui/`. Arbitrary values: 82 (`grid-cols-[…]` 20, the
rest data-/has- selectors).

### 14.2 Canonical rules (container to spacing)

| Container | Padding | Inner gap |
|---|---|---|
| bar (toolbar, panel header, strip) | `px-1.5` (primitive) | `gap-1` |
| list row | from `rowVariants` | `gap-1.5` |
| content pane / detail body | `p-3` | `gap-3` between blocks, `gap-2` inside a block |
| dialog body | primitive `p-3 gap-3` | same |
| card / popover / radio card | primitive (2.5 kept, documented as the inset step) | `gap-1.5` inside |
| form stack | `FieldGroup`/`FieldSet` gaps | primitive |
| inline cluster (icon + text, badge row) | none | `gap-1` (icon-text), `gap-1.5` (chips) |

- **RSp.1** Pane roots use `p-3`; `p-4` goes (7). Hero/empty landing pages use `EmptyState`.
- **RSp.2** Alignment: rows and bars `items-center`; multi-line rows `items-start` with the icon
  `mt-0.5`; labels left-aligned; numbers right-aligned in tables; actions right-aligned (`ml-auto`
  or `justify-end`).
- **RSp.3** The 2.5 step stays inside primitives only (documented in ARCHITECTURE, no change).

## 15. Icons

### 15.1 Inventory

`CodiconIcon` sizes: 13 220, 12 122, 24 8, 16 6 (U12 holds). In labelled buttons 13 (93) vs 12
(42) for the same job. `EngineIcon` 22 in `ConnectionEnginePicker` (off scale; 16 elsewhere).
Warning/error icon colour: none (inherit) 8, `text-warn` 2, `text-error` 3, `text-tone-red` 2,
muted 1.

### 15.2 Canonical rules

- **RI.1 Size by host**: 13 in buttons, menus, rows, bars (with or without label); 12 inside a
  `Badge`, chip or `size="inline"` text; 16 leading icon of a `DetailHeader`/card tile; 24 empty
  state. `EngineIcon` follows (16 in the picker tile).
- **RI.2 Colour**: icons muted by default; status icons per RS.1 (`warning` = `text-warn`, `error` =
  `text-error`, `pass` = `text-ok`); never ADE tones off-board.

## 16. Borders, radii, shadows

### 16.1 Inventory

Radii: `rounded-kira-sm` 47, `rounded-kira` 31, `rounded-full` 19 (dots, count badges, avatars),
`rounded-none` 13 (joined controls). Borders: `border-border` 65, `border` 34, `border-r` 21,
`border-b` 19, `border-l-2` 6 (quote/accents), `border-2` 5, `border-l-3` 2, `border-info` 2.
Shadows: `shadow-kira` 2, arbitrary `shadow-[…]` 2, `shadow-md` 1.

### 16.2 Canonical rules

- **RBo.1** `rounded-kira-sm` (4px) for controls, rows, chips, inset blocks; `rounded-kira` (6px)
  for containers (card, popover, dialog: primitives); `rounded-full` only for dots, avatars, count
  badges.
- **RBo.2** Dividers are 1px `border-border` (`border-b` between bar and content, `border-r`
  between panes); accent bars `border-l-2` only for quotes/notes; `border-2`/`border-l-3` go.
- **RBo.3** Shadows only from the elevation tiers (primitives); `shadow-md` and arbitrary shadows go.

## 17. Tabs, segmented controls, choice

### 17.1 Inventory

`SecondaryTabs` 79 uses (Studio 23, ADE 19, Git 15, Runs 6, Memory 5, Kubernetes/Docker 3, A.8),
variants `tabs` (content sections), `segmented` (view mode), `choice` (a form value). By host:

| Host | Today | Right per role |
|---|---|---|
| form/dialog value (scope, transport, mode) | `choice` 9; `segmented@kira-lg` 8 (`ScopeField`, `ConnectionGeneralTab`, `ConnectionMcpMasked`, `ConnectionMcpReview`, `ConnectionMcpTab`, `AdeAgentDialog`, `ImportConfirmDialog`, `ParamsEditor`); `segmented@kira` 1 (`AdeEstimateField`) | `choice` |
| content sections of a view/panel/dialog | `tabs` 35 | `tabs` |
| toolbar view switch (grid/json, list/tree) | `segmented@kira` | `segmented@kira` |
| page-level tabs in ADE | `tabs@kira-lg` (`AdeWorkflowsPage`, `AdeAddPopover`) | `tabs` default size |

Radio cards: `RadioGroupCard` (px-2.5 py-2, `border-focus bg-select` when checked, title/description
slots). `ConnectionEnginePicker` hand-rolls the same on reka `RadioGroupItem` (px-2 py-3, focus
border, no `bg-select`, `EngineIcon` 22) under a U36 PERMANENT exemption
(`scripts/check-ui-primitives.sh:99`).

### 17.2 Canonical rules

- **RTa.1 Role picks the variant**: picking a *value* in a form = `choice` (or `RadioGroupCard` for
  ≤ 6 options with a description each); switching *content* = `tabs`; switching the *view of the
  same content* in a bar = `segmented`. Size follows RB.1 (form `kira-lg`, bar `kira`); `tabs` keep
  the default size.
- **RTa.2** `ConnectionEnginePicker` moves onto `RadioGroupCard` (engine icon 16 in the media/title
  slot, title, short description), and its U36 PERMANENT line is deleted.

## 18. Highest-impact inconsistencies, ranked

Rank = visibility (how often a user sees it) × spread (modules) × count.

1. **Field labels in two colours and five layouts** (§4): `Label` fg 148 vs `FieldLabel` muted
   92; `AdeInspectorRow`, `AdeRow`, `EditField`, `CheckboxRow`, settings fields each lay out labels
   differently. Every form in the app shows it.
2. **Status colours mean different things** (§9): running = info in Runs, warn in ADE; ADE pastel
   red vs theme red for "failed", ADE tones leaking into Runs; 46 `AdeChip` beside 203 `Badge`.
3. **Detail headers hand-rolled 11 times** (§7): five paddings, two title sizes, icon or none,
   actions as outline or toolbar buttons.
4. **In-view actions in three button looks** (§3): toolbar, outline `kira`, outline `kira-lg`
   for the same kind of action (ADE 41/45 split, Runs 22 `dialog@kira-lg`, Studio 16
   `dialog-primary@kira`).
5. **Section titles in two styles** (§7): `h3 text-kira-lg` (13) vs uppercase `SectionHeading` /
   `FieldLegend` inside the same kind of dialog.
6. **Popover padding: 24 class variants for two shapes** (§5).
7. **Segmented control used as a form value** (§17): 9 forms use `segmented` where `choice` is the
   form control; plus `ConnectionEnginePicker` hand radio card.
8. **Dialog footers breaking one-emphasis / order** (§2): 5 dialogs; header extras in 6.
9. **Empty/failed states drawn three ways** (§10): toned vs muted media, missing icon, ad hoc
   Skeleton shapes.
10. **Key-value grids** (§7): `KeyValueList` with 7 variants plus 8 hand grids, dt muted vs subtle.
11. **Checkbox restyled in 34 places** (§4 RF.8) and dropdowns hand-built in 22 files (§5).
12. **Pane padding p-3 vs p-4, sm vs md body text** (§13, §14).

## 19. Decisions for the user

- **D1 ADE palette** (RS.3): (a) keep ADE pastels as board/card identity only, theme tones
  everywhere else (recommended: smallest change, keeps the documented board look); or (b) retire the
  ADE palette app-wide.
- **D2 New shared pieces**: `DetailHeader`, `Field orientation="responsive"`, `PopoverContent
  flush`, `KeyValueList density="compact"`, `DetailSection` promoted to theme. All are thin
  compositions of existing primitives; no new dependency. Accept or name any to drop.
- **D3 Toasts** (RE.4): restrict to background outcomes (recommended) vs allow app-wide.

## Appendix: exhaustive per-file lists

Generated from the template scans in §0 at base `6527aec4`; line numbers are the opening tag.
Parts re-run the same scans against their own base before applying.

<!-- appendix lands in the next commit -->
