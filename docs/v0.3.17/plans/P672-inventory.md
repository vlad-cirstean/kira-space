# P672 inventory: UI elements and uniformity rules

Spec row: `../SPEC.md` P672 Part 1. Plan: `P672.md`. Base: `chapter/v0.3.16` at `6527aec4` (moved to chapter v0.3.17).
Analysis only. Status: complete (§0-§20, appendix A.1-A.13).

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

- Two ways to build the same dropdown: `MenuItems` from a `MenuItem[]` (3 dropdown hosts:
  Git `AppToolbar`, `PullStrategyPicker`, Workbench `ContextMenu`) vs hand `DropdownMenuItem` lists
  (21 files, ADE 12: `AdeStatusPicker`, `AdeStepMenu`, `AdeTaskStatusMenu`, `AdeReviewActions`,
  `AdeTaskBranchMenu`, …; A.11). Context menus already use the data form; dropdowns
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
11. **Checkbox restyled in 34 places** (§4 RF.8) and dropdowns hand-built in 21 files (§5).
12. **Pane padding p-3 vs p-4, sm vs md body text** (§13, §14).

## 19. Decisions for the user

- **D1 ADE palette** (RS.3): (a) keep ADE pastels as board/card identity only, theme tones
  everywhere else (recommended: smallest change, keeps the documented board look); or (b) retire the
  ADE palette app-wide.
- **D2 New shared pieces**: `DetailHeader`, `Field orientation="responsive"`, `PopoverContent
  flush`, `KeyValueList density="compact"`, `DetailSection` and `TextTooltip` (from `AdeTip`)
  promoted to theme. All are thin
  compositions of existing primitives; no new dependency. Accept or name any to drop.
- **D3 Toasts** (RE.4): restrict to background outcomes (recommended) vs allow app-wide.

## 20. Migration items by Part

Each item names its rule and scope. Part numbers match `P672.md` "Parts 2..n" and `SPEC.md`.

Part 2 (theme pieces + lint), runs first:
- M1 `Field orientation="responsive"` (RF.2), `PopoverContent flush` (RM.3), `KeyValueList
  density="compact"` (RH.4), `DetailHeader` (RH.2), `DetailSection` to theme as the titled section
  card (RC.1), `TextTooltip` from `AdeTip` (RT.2). Theme only; no call-site migration beyond the
  move of `DetailSection` and `AdeTip` imports.
- M2 New guards in `scripts/check-ui-primitives.sh` for rules a regex can hold, added with the
  current offenders as a temporary, dated baseline that later Parts empty (never PERMANENT):
  Checkbox `size-*` class or child (RF.8); `PopoverContent` padding classes (RM.3); `h[1-6]` with
  `text-kira-lg` outside `DetailHeader`/`DialogTitle` (RH.3); `grid-cols-[` on `KeyValueList`
  and hand dt/dd grids (RH.4); `text-tone-*` outside the ADE board allowlist (RS.3, after D1);
  `font-mono` (RTy.2); `iconClass` tones in `MenuItem` (RM.2); `segmented` inside `Field`/
  `DialogBody` (RTa.1).
- M3 ARCHITECTURE "UI architecture": rules RD-RTa as short bullets; PanelHeader vs SectionHeading
  grey step (§7.1); spacing table (§14.2).

Part 3 (forms): M4 RF.1 labels (A.4); M5 RF.2 columns layout: `AdeInspectorRow`, `EditField`,
`AdeRow` editable label column onto `Field responsive`; M6 RF.3/RF.8 `CheckboxRow` and A.6; M7
RF.4-RF.6 FieldSet gaps, widths, textarea heights; M8 RF.7 hand error text (A.9 + bound classes);
M9 input `kira` in forms (§4.1 list) to default, drop explicit `size="kira-lg"`.

Part 4 (buttons + dialogs): M10 RB.1/RB.2 in-view outline buttons to `toolbar@kira` (candidates: ADE 86,
Runs 23, Studio, Git, Memory, API per §3.1 table), keep commit/decision uses; M11 RB.3
`ACTION_CLASS` off-board; M12 RD.1 sizes per A.1; M13 RD.2 footers (5 dialogs); M14 RD.3/RD.4
header extras and intro text; M15 RD.5 `DialogBody` overrides.

Part 5 (menus, popovers, tooltips): M16 RM.1 hand dropdowns (21 files, A.11) to `MenuItems`; M17 RM.2
toned menu icons; M18 RM.3 every `PopoverContent` in A.2 to default or `flush` + section rows;
M19 RT.1/RT.2 hand Tooltip triples with a text-only body to `TextTooltip`.

Part 6 (headings, detail headers, key-value, cards): M20 RH.2 the 11 detail headers onto
`DetailHeader`; M21 RH.3 the 13 `h3` section titles; M22 RH.4 `KeyValueList` variants and hand
grids; M23 RC.1-RC.3 cards per A.3.

Part 7 (status, tones, colours; needs D1): M24 RS.1 status map (`AdeOpStatus` running to info,
Git change-request badge classes to `Badge` tones); M25 RS.3 per D1 (A.7); M26 RL.4/RI.2 warning
and error icon colours; M27 RCo.2 redundant `text-fg`, row meta greys (RL.2).

Part 8 (tabs, segmented, choice): M28 RTa.1 the 9 form `segmented` to `choice`, ADE `tabs@kira-lg`
to default; M29 RTa.2 `ConnectionEnginePicker` onto `RadioGroupCard`, `EngineIcon` 16, delete its
U36 PERMANENT line.

Part 9 (states, icons, type, spacing residue): M30 RE.1-RE.3 EmptyState media (A.10), failure
shape, Skeleton shapes; M31 RE.4 toasts (needs D3); M32 RI.1 labelled-button icons 12 to 13; M33
RTy.1-RTy.3 sizes, `font-mono`, leading/tracking; M34 RSp.1/RSp.2 pane `p-4` to `p-3`, alignment;
M35 RBo.1-RBo.3 radii, borders, shadows; M36 remove every temporary baseline left in the M2 guards
(all must be empty here).

## Appendix: exhaustive per-file lists

Generated from the template scans in §0 at base `6527aec4`; line numbers are the opening tag.
Parts re-run the same scans against their own base before applying.

### A.1 Dialogs (62)

Flags: `F` fixed-height, `D` DialogDescription, `H` header extras, `S` footer `#start`, `B` DialogBody class, `T` tabs in body. Rule column names the rule the dialog breaks today (blank: none found by scan).

| Module | File | Size | Flags | Footer | Breaks |
|---|---|---|---|---|---|
| ADE | `ade/v2/details/AdeDetailsDiskBanner.vue` | xl | D | dialog-danger, dialog-primary | RD.2 two emphasised |
| ADE | `ade/v2/dialog/AdeAgentDialog.vue` | lg | HT | dialog, dialog-danger, dialog-primary | RD.2 two emphasised; RD.3; RD.4? |
| ADE | `ade/v2/panel/AdeRunSandboxBadge.vue` | lg |  |  | RD.4? |
| ADE | `ade/v2/port/AdePortBackupsDialog.vue` | 2xl | F | dialog | RD.4? |
| ADE | `ade/v2/port/AdePortExportDialog.vue` | 2xl | F | dialog, dialog-primary | RD.4? |
| ADE | `ade/v2/port/AdePortImportDialog.vue` | 2xl | F | dialog, dialog-primary | RD.4? |
| ADE | `ade/v2/review/AdeReviewSubmitDialog.vue` | md |  | Close>, dialog, dialog-primary | RD.4? |
| ADE | `ade/v2/run/AdeRepoGateDialog.vue` | sm |  | dialog, dialog-primary | RD.4? |
| ADE | `ade/v2/run/AdeRunDialog.vue` | lg |  | dialog, dialog-primary | RD.4? |
| ADE | `ade/v2/settings/AdeSettingsSecrets.vue` | sm |  | dialog, dialog-primary | RD.4? |
| ADE | `ade/v2/stage/AdeStageDialog.vue` | lg | T | dialog, dialog, dialog-primary | RD.4? |
| ADE | `ade/v2/workflows/statuses/AdeTaskStatuses.vue` | sm | D | dialog, dialog-primary |  |
| API | `studio/api/ApiSecrets.vue` | sm |  | dialog, dialog-primary | RD.4? |
| API | `studio/api/ApiVariablesDialog.vue` | 2xl | FT | dialog-danger, dialog-primary | RD.2 two emphasised; RD.4? |
| API | `studio/api/CopyAsCurlDialog.vue` | xl |  | dialog, dialog-primary | RD.4? |
| API | `studio/api/DynamicValuesDialog.vue` | md |  | dialog | RD.4? |
| API | `studio/api/EditRawRequestDialog.vue` | xl |  | dialog, dialog-primary | RD.4? |
| API | `studio/api/ImportCurlDialog.vue` | lg |  | dialog, dialog-primary | RD.4? |
| API | `studio/api/SaveRequestDialog.vue` | md |  | dialog, dialog-primary | RD.4? |
| Docker | `docker-ui/src/components/RecreateConfirmDialog.vue` | lg | D | dialog, dialog-danger |  |
| Git | `git-ui/src/components/WorktreeList.vue` | md |  | dialog, dialog-danger | RD.4? |
| Git | `git-ui/src/components/dialogs/BranchDialog.vue` | md |  | dialog, dialog-primary | RD.4? |
| Git | `git-ui/src/components/dialogs/CheckoutDialog.vue` | md | D | dialog, dialog-primary, dialog-danger | RD.2 two emphasised |
| Git | `git-ui/src/components/dialogs/CherryPickDialog.vue` | md | D | dialog, dialog-primary |  |
| Git | `git-ui/src/components/dialogs/ForcePushDialog.vue` | md | D | dialog, dialog-danger |  |
| Git | `git-ui/src/components/dialogs/PostCheckoutPullDialog.vue` | md | D | dialog, dialog-primary |  |
| Git | `git-ui/src/components/dialogs/PullDialog.vue` | md | D | dialog, dialog-primary |  |
| Git | `git-ui/src/components/dialogs/RenameRefDialog.vue` | md |  | dialog, dialog-primary | RD.4? |
| Git | `git-ui/src/components/dialogs/RepoSettingsDialog.vue` | md |  | dialog, dialog-primary | RD.4? |
| Git | `git-ui/src/components/dialogs/ResetDialog.vue` | md | DH | dialog, dialog-primary, isHardDestructive ? 'dialog-danger' : 'dialog-primary' | RD.2 order; RD.3 |
| Git | `git-ui/src/components/dialogs/RevertDialog.vue` | md | D | dialog, dialog-primary |  |
| Git | `git-ui/src/components/dialogs/StackDialog.vue` | md |  | multi-step (5) | RD.4? |
| Git | `git-ui/src/components/dialogs/StashDialog.vue` | md | T | multi-step (8) | RD.4? |
| Git | `git-ui/src/components/dialogs/TagDialog.vue` | md |  | dialog, dialog-primary | RD.4? |
| Git | `git-ui/src/components/dialogs/WorktreeDialog.vue` | md | T | dialog, dialog-primary | RD.4? |
| Kubernetes | `studio/kubernetes/components/ClusterMcpDialog.vue` | lg | DT | dialog, dialog-primary |  |
| Memory | `workbench/src/memory/AddMemoryDialog.vue` | lg |  | dialog-primary, dialog, dialog, dialog-primary | RD.2 two emphasised; RD.4? |
| Memory | `workbench/src/memory/import/ImportConfirmDialog.vue` | md | T | dialog, dialog-primary | RD.4? |
| Mobile | `mobile/components/ReplySheet.vue` | sm | D | dialog, dialog-approve |  |
| Runs | `workbench/src/automations/RunFileDialog.vue` | lg | T | Close>, dialog-danger, dialog-primary | RD.2 two emphasised; RD.4? |
| Runs | `workbench/src/automations/run/RunScriptDialog.vue` | lg |  | Close>, dialog, dialog-primary | RD.4? |
| Runs | `workbench/src/automations/schedule/ScheduleConfirmDialog.vue` | lg | HS | dialog, dialog-approve | RD.3; RD.4? |
| Runs | `workbench/src/automations/schedule/ScheduleTargetDialog.vue` | md |  | Close>, dialog, dialog-primary | RD.4? |
| Shell | `workbench/GitCredentialDialog.vue` | sm | S | dialog, dialog-primary | RD.4? |
| Shell | `workbench/PairingRequestDialog.vue` | sm |  | dialog-deny, dialog-approve | RD.4? |
| Studio | `studio/project/ConnectionFileDialog.vue` | 2xl | FST | multi-step (5) | RD.4? |
| Studio | `studio/project/CredentialsUpdateDialog.vue` | md |  |  | RD.4? |
| Studio | `studio/project/DataGripImportDialog.vue` | xl | FSB | dialog, dialog, dialog-primary, dialog-primary | RD.2 two emphasised; RD.4? |
| Studio | `studio/project/FiltersDialog.vue` | lg | S | dialog, dialog-primary | RD.4? |
| Studio | `studio/project/MaskingReviewPrompt.vue` | md |  | dialog, dialog-primary | RD.4? |
| Studio | `studio/project/NewConnectionDialog.vue` | 2xl | FHB | dialog, dialog-primary | RD.3; RD.4? |
| Studio | `studio/project/SchemaDialog.vue` | xl | FS | dialog, dialog-primary | RD.4? |
| Studio | `studio/proof/ProofReadyPrompt.vue` | md | D | dialog, dialog-primary |  |
| Studio | `studio/views/httprequest/ResponseDiffDialog.vue` | 2xl | FB | dialog | RD.4? |
| Studio | `studio/workbench/GenerateDataDialog.vue` | xl |  | dialog, dialog, dialog-primary | RD.4? |
| Studio | `studio/workbench/UploadObjectDialog.vue` | md |  | dialog, dialog-primary | RD.4? |
| Studio | `studio/workbench/apiaccess/ApiAccessRuleDialog.vue` | lg |  | dialog, dialog-primary | RD.4? |
| Workbench | `workbench/src/approvals/ApprovalsDialog.vue` | full | FH | dialog-deny | RD.3; RD.4? |
| Workbench | `workbench/src/components/ConfirmDialogView.vue` | sm |  | dialog, dialog, tone === 'danger' ? 'dialog-danger' : 'dialog-primary' | RD.4? |
| Workbench | `workbench/src/components/SettingsShell.vue` | xl | FB |  | RD.4? |
| Workbench | `workbench/src/components/UpdateDialog.vue` | sm |  | multi-step (6) | RD.4? |
| Workbench | `workbench/src/restore/RestoreDialog.vue` | lg | DH | dialog, dialog-primary | RD.3 |

`RD.4?`: no `DialogDescription`; the Part checks whether the body opens with intro text that belongs there.

### A.2 PopoverContent (41)

| Module | File:line | class |
|---|---|---|
| ADE | `ade/v2/AdeAddPopover.vue:182` | `w-120 gap-0 overflow-hidden p-0` |
| ADE | `ade/v2/links/AdeLinkAdd.vue:83` | `w-80 gap-1.5` |
| ADE | `ade/v2/links/AdeLinkStrip.vue:285` | `w-80` |
| ADE | `ade/v2/panel/AdeRunSandboxBadge.vue:79` | `w-96 text-kira-md` |
| ADE | `ade/v2/panel/AdeTaskBranchForm.vue:94` | `w-80 gap-0 overflow-hidden p-0` |
| ADE | `ade/v2/plan/AdeArchivedFinder.vue:37` | `w-96 p-0` |
| ADE | `ade/v2/plan/AdeEstimateChip.vue:60` | `w-auto p-2` |
| ADE | `ade/v2/plan/AdeTaskAddMenu.vue:59` | `w-80 gap-0 overflow-hidden p-0` |
| ADE | `ade/v2/plan/AdeTaskChildren.vue:74` | `w-80 gap-0 p-1` |
| ADE | `ade/v2/sessions/AdeSessionsMenu.vue:46` | `w-120 p-0` |
| ADE | `ade/v2/status/AdeAgentsStatusItem.vue:72` | `w-96 p-1` |
| ADE | `ade/v2/updates/AdeUpdatesPopover.vue:28` | `w-80 p-0` |
| ADE | `ade/v2/workflows/statuses/AdeTaskStatuses.vue:198` | `w-auto p-2` |
| API | `studio/api/VariableHistoryMenu.vue:47` | `w-80 gap-0 p-0` |
| API | `studio/api/VariablesOverviewPanel.vue:97` | `w-96 gap-0 p-0` |
| Git | `git-ui/src/App.vue:1949` | `w-auto flex-row items-center gap-1 px-2 py-1` |
| Git | `git-ui/src/components/RefsPopover.vue:434` | `w-96 p-0 gap-0` |
| Git | `git-ui/src/components/SearchBox.vue:288` | `p-0 gap-0 w-120` |
| Proto | `proto/grid/NavPreview.vue:32` | `` |
| Runs | `workbench/src/automations/form/ScheduleFields.vue:146` | `w-96 p-0` |
| Runs | `workbench/src/automations/run/AdeContextFields.vue:70` | `w-96 p-0` |
| Studio | `studio/project/ErrorPopover.vue:41` | `w-96 max-h-60 gap-0 overflow-hidden p-0` |
| Studio | `studio/views/console/kibana/FilterBar.vue:88` | `w-80 p-2` |
| Studio | `studio/views/console/kibana/FilterBar.vue:130` | `w-80 p-2` |
| Studio | `studio/views/console/kibana/TimePicker.vue:92` | `w-80 p-2` |
| Studio | `studio/views/documents/DocumentToolbar.vue:156` | `w-56 p-0` |
| Studio | `studio/views/grid/ColumnsMenu.vue:131` | `w-56 gap-0 p-0` |
| Studio | `studio/views/grid/PreviewCommandPanel.vue:103` | `w-120 gap-0 p-0` |
| Studio | `studio/views/shared/SavedListMenu.vue:87` | `w-80 gap-0 p-0` |
| Studio | `studio/views/shared/celleditor/CellGenerateMenu.vue:36` | `w-56 gap-0 p-0` |
| Studio | `studio/views/shared/celleditor/TimestampPane.vue:179` | `w-56 gap-0 p-0` |
| Studio | `studio/views/shared/keyvalue/KeyValueAddPopover.vue:76` | `w-80` |
| Studio | `studio/views/shared/keyvalue/KeyValueEditPopover.vue:73` | `w-80` |
| Studio | `studio/views/stream/StreamFilterRow.vue:238` | `w-56 p-0` |
| Studio | `studio/views/stream/StreamFilterRow.vue:305` | `w-56 p-0` |
| Studio | `studio/views/stream/StreamToolbar.vue:318` | `w-96 p-0` |
| Workbench | `workbench/src/components/AutocompleteField.vue:451` | `autocomplete-suggestions z-(--kira-z-autocomplete) w-auto min-w-50 max-w-completion-max-w max-h-60 gap-0 overflow-x-hidden overflow-y-auto p-0.5 font-data` |
| Workbench | `workbench/src/components/AutocompleteField.vue:499` | `var-hover-panel pointer-events-none z-(--kira-z-autocomplete) w-auto max-w-96 gap-0 overflow-hidden px-1.5 py-1 font-data` |
| Workbench | `workbench/src/components/PointPopover.vue:22` | `` |
| Workbench | `workbench/src/components/RunningOpsStatusItem.vue:45` | `w-96 p-1` |
| Workbench | `workbench/src/components/pickers/PickerPopover.vue:127` | `w-80 gap-0 p-0` |

### A.3 Card (41)

| Module | File:line | variant | class |
|---|---|---|---|
| ADE | `ade/v2/agents/AdeLeasesPane.vue:45` | section | `flex flex-col gap-2 p-3` |
| ADE | `ade/v2/details/AdeProofEntry.vue:46` | default | `flex flex-col gap-2 p-2 text-kira-md` |
| ADE | `ade/v2/details/AdeTaskChain.vue:119` | default | `flex flex-col gap-2 p-2` |
| ADE | `ade/v2/dialog/AdeAgentDialog.vue:284` | default | `whitespace-pre-wrap font-data text-kira-sm text-muted-foreground` |
| ADE | `ade/v2/notes/AdeNoteEntry.vue:68` | default | `flex flex-col gap-1 p-2 text-kira-md` |
| ADE | `ade/v2/panel/AdeStepBlock.vue:216` | section | `gap-0 py-1` |
| ADE | `ade/v2/plan/AdeTaskCard.vue:117` | section | `box-border flex min-w-0 flex-1 flex-col overflow-hidden in-[.sortable-fallback]:max-h-board-row in-[.sortable-fallback]:max-w-64 in-[.sortable-fallback]:shadow-kira` |
| ADE | `ade/v2/review/AdeReviewRemoved.vue:77` | default | `` |
| ADE | `ade/v2/review/AdeWorkflowReviewPanel.vue:205` | default | `flex flex-col gap-1 p-2` |
| ADE | `ade/v2/run/AdeRunDialog.vue:227` | default | `select-text whitespace-pre-wrap break-words font-data text-kira-sm` |
| ADE | `ade/v2/run/AdeRunDialog.vue:235` | default | `select-text whitespace-pre-wrap break-words font-data text-kira-sm` |
| ADE | `ade/v2/run/AdeRunDialog.vue:245` | default | `whitespace-pre-wrap font-data text-kira-sm` |
| ADE | `ade/v2/workflows/AdeStepInspector.vue:289` | default | `text-kira-sm text-subtle` |
| ADE | `ade/v2/workflows/inspector/AdeStepAgentFields.vue:94` | default | `font-data text-kira-sm whitespace-pre-wrap` |
| ADE | `ade/v2/workflows/inspector/AdeStepAgentFields.vue:121` | default | `text-kira-sm text-muted-foreground` |
| Docker | `docker-ui/src/components/DetailSection.vue:8` | section | `` |
| Memory | `workbench/src/memory/AddMemoryDialog.vue:136` | default | `gap-0.5` |
| Memory | `workbench/src/memory/MemoryStart.vue:126` | default | `gap-0.5` |
| Memory | `workbench/src/memory/import/ImportJobDetail.vue:290` | default | `gap-0` |
| Memory | `workbench/src/memory/settings/AgentMcpSection.vue:44` | default | `font-data whitespace-pre-wrap break-all select-all text-kira-sm leading-normal` |
| Memory | `workbench/src/memory/settings/CheckHookSection.vue:32` | default | `font-data whitespace-pre-wrap break-all select-all text-kira-sm leading-normal` |
| Memory | `workbench/src/memory/settings/CheckHookSection.vue:54` | default | `font-data whitespace-pre-wrap break-all select-all text-kira-sm leading-normal` |
| Mobile | `mobile/screens/PairScreen.vue:67` | default | `flex flex-col gap-1` |
| Runs | `workbench/src/automations/form/ParamsEditor.vue:158` | default | `` |
| Runs | `workbench/src/automations/form/ScheduleFields.vue:97` | default | `gap-3` |
| Runs | `workbench/src/automations/run/RunPreview.vue:40` | default | `text-kira-sm` |
| Settings | `workbench/settings/CliToolsPane.vue:70` | default | `flex flex-col gap-2 p-3` |
| Shell | `hostview/CommentComposer.vue:81` | section | `relative gap-2 p-2` |
| Shell | `hostview/CommentComposer.vue:105` | default | `min-h-16` |
| Studio | `studio/project/FiltersDialog.vue:234` | list | `flex max-h-56 flex-col gap-px overflow-y-auto` |
| Studio | `studio/project/FiltersDialog.vue:279` | list | `flex max-h-56 flex-col gap-px overflow-y-auto` |
| Studio | `studio/project/connection/ConnectionMaskReview.vue:141` | section | `` |
| Studio | `studio/project/connection/ConnectionPrivacyTab.vue:54` | section | `` |
| Studio | `studio/project/connection/PrivacyOverview.vue:18` | default | `gap-1` |
| Studio | `studio/views/console/ExplainResultView.vue:155` | list | `` |
| Studio | `studio/views/httprequest/TimelinePane.vue:250` | default | `gap-0.5` |
| Studio | `studio/workbench/settings/ApiAccessRulesSection.vue:83` | list | `flex-row items-center justify-between gap-1.5` |
| Studio | `studio/workbench/settings/McpExposureRow.vue:23` | list | `flex-row items-center justify-between gap-1.5` |
| Studio | `studio/workbench/settings/StudioMcpConnectionsPane.vue:53` | list | `flex-row items-center justify-between gap-1.5` |
| Theme | `theme/src/components/CodeBlock.vue:11` | default | `` |
| Theme | `theme/src/components/ScriptProgress.vue:44` | default | `` |

### A.4 Bare `Label` as a field label (beside an input, not wrapping a checkbox/switch)

Files with bare `Label` (any use; Part 3 classifies each use against RF.1): `ade/v2/AdeAddPopover.vue` (3), `ade/v2/dialog/AdeAgentDialog.vue` (2), `ade/v2/panel/AdeMainTab.vue` (2), `ade/v2/panel/AdeRow.vue` (1), `ade/v2/plan/AdeHistoryBar.vue` (1), `ade/v2/port/AdePortExportDialog.vue` (1), `ade/v2/repos/AdeReposPage.vue` (1), `ade/v2/request/BranchRequestBody.vue` (2), `ade/v2/request/DetailsEditBody.vue` (1), `ade/v2/review/AdeWorkflowReviewPanel.vue` (1), `ade/v2/run/AdeRunDialog.vue` (2), `ade/v2/stage/AdeStageDialog.vue` (2), `ade/v2/workflows/AdeWorkflowGraph.vue` (1), `ade/v2/workflows/inspector/AdeInspectorRow.vue` (1), `ade/v2/workflows/inspector/AdeStepResults.vue` (1), `ade/v2/workflows/statuses/AdeTaskStatuses.vue` (2), `studio/api/ApiAgentAccessForm.vue` (1), `studio/api/SaveRequestDialog.vue` (2), `docker-ui/src/components/DockerPanel.vue` (1), `git-ui/src/components/FileTreeToolbar.vue` (1), `studio/kubernetes/components/ClusterMcpDialog.vue` (1), `workbench/src/memory/AddMemoryDialog.vue` (3), `workbench/src/memory/MemoryPanel.vue` (1), `workbench/src/memory/ScopeField.vue` (2), `workbench/src/memory/distill/DistillResult.vue` (2), `workbench/src/memory/import/ImportConfirmDialog.vue` (3), `mobile/screens/PairScreen.vue` (1), `workbench/src/automations/RunFileDialog.vue` (1), `workbench/src/automations/form/McpServerTools.vue` (1), `workbench/src/automations/form/McpToolsField.vue` (1), `workbench/src/automations/form/ParamsEditor.vue` (2), `workbench/src/automations/form/RunFolderTab.vue` (1), `workbench/src/automations/form/RunMcpTab.vue` (1), `workbench/src/automations/form/ScheduleFields.vue` (2), `workbench/src/automations/form/SmartSettingsFields.vue` (1), `workbench/src/automations/form/ToolsField.vue` (1), `workbench/src/automations/run/ParamsForm.vue` (1), `workbench/settings/AdvancedPane.vue` (3), `workbench/settings/AgentPane.vue` (5), `workbench/settings/AppearancePane.vue` (2), `workbench/settings/CapturePane.vue` (2), `workbench/settings/ConfigSyncPane.vue` (3), `workbench/settings/DateFormatField.vue` (1), `workbench/settings/ExperimentalPane.vue` (1), `workbench/settings/FileTypesPane.vue` (1), `workbench/settings/GitPane.vue` (4), `workbench/settings/MemoryPane.vue` (1), `workbench/settings/MobileAccessPane.vue` (5), `workbench/settings/UpdatesPane.vue` (1), `workbench/src/settings/fields/FontFamilyField.vue` (1), `workbench/src/settings/fields/FontSizeField.vue` (1), `workbench/src/settings/fields/LogLevelField.vue` (1), `workbench/src/settings/fields/NotifyPromptsField.vue` (1), `workbench/src/settings/fields/WordWrapField.vue` (1), `configsync/ConfigSyncUnitRow.vue` (1), `studio/approvals/ReviewResultOption.vue` (1), `studio/project/FiltersDialog.vue` (2), `studio/project/connection/CheckboxRow.vue` (1), `studio/project/connection/ConnectionMcpTables.vue` (1), `studio/project/credentialPaste/CredentialPastePanel.vue` (1), `studio/views/documents/ProjectionMenu.vue` (1), `studio/views/grid/ColumnsMenu.vue` (1), `studio/views/httprequest/RequestSettingsPane.vue` (14), `studio/views/stream/StreamComposeMessage.vue` (3), `studio/views/stream/StreamFilterRow.vue` (1), `studio/workbench/GenerateDataDialog.vue` (2), `studio/workbench/UploadObjectDialog.vue` (2), `studio/workbench/apiaccess/ApiAccessRuleDialog.vue` (3), `studio/workbench/settings/ApiAccessRulesSection.vue` (1), `studio/workbench/settings/ApiPane.vue` (7), `studio/workbench/settings/CachePane.vue` (3), `studio/workbench/settings/DataPane.vue` (1), `studio/workbench/settings/StudioMcpApiPane.vue` (2), `studio/workbench/settings/StudioMcpServerPane.vue` (3), `theme/src/SwatchRadio.vue` (1)

### A.5 Headings h1-h6 (24)

| Module | File:line | tag | class |
|---|---|---|---|
| ADE | `ade/v2/later/AdeShelfSection.vue:51` | h3 | `shrink-0 px-3 pb-1 pt-3 text-kira-lg font-medium` |
| ADE | `ade/v2/needs/AdeAllSessions.vue:131` | h3 | `m-0 truncate text-kira-lg font-medium` |
| ADE | `ade/v2/panel/AdeArchivedPanel.vue:152` | h3 | `m-0 line-clamp-2 min-w-0 flex-1 break-words text-kira-lg font-medium text-fg` |
| ADE | `ade/v2/panel/AdeBranchPanel.vue:319` | h3 | `m-0 line-clamp-2 min-w-0 flex-1 break-all font-data text-kira-lg font-medium text-fg` |
| ADE | `ade/v2/port/AdePortExportDialog.vue:137` | h3 | `m-0 text-kira-lg font-medium` |
| ADE | `ade/v2/port/AdePortExportDialog.vue:209` | h3 | `m-0 text-kira-lg font-medium` |
| ADE | `ade/v2/port/AdePortExportDialog.vue:231` | h3 | `m-0 text-kira-lg font-medium` |
| ADE | `ade/v2/port/AdePortResult.vue:14` | h3 | `m-0 text-kira-lg font-medium` |
| ADE | `ade/v2/settings/AdeSettingsResult.vue:51` | h3 | `m-0 text-kira-lg font-medium` |
| ADE | `ade/v2/settings/AdeSettingsSecrets.vue:75` | h3 | `m-0 text-kira-lg font-medium` |
| ADE | `ade/v2/workflows/statuses/AdeTaskStatuses.vue:258` | h4 | `m-0 text-kira-lg font-medium text-fg` |
| API | `studio/api/ApiAgentAccess.vue:28` | h3 | `m-0 text-kira-lg font-medium` |
| API | `studio/api/ApiSecrets.vue:77` | h3 | `m-0 text-kira-lg font-medium` |
| Docker | `docker-ui/src/components/EngineOverview.vue:173` | h2 | `text-kira-lg font-medium` |
| Git | `git-ui/src/components/review/ReviewNoBranch.vue:16` | h2 | `text-kira-lg font-medium text-fg` |
| Git | `git-ui/src/components/review/ReviewView.vue:485` | h2 | `text-kira-lg font-medium text-fg` |
| Memory | `workbench/src/memory/distill/DistillResult.vue:180` | h3 | `m-0 text-kira-lg font-medium` |
| Memory | `workbench/src/memory/distill/DistillRewriteResult.vue:76` | h3 | `m-0 text-kira-lg font-medium` |
| Mobile | `mobile/AppShell.vue:49` | h1 | `m-0 flex-1 text-kira-lg font-medium` |
| Mobile | `mobile/components/PhoneApprovals.vue:45` | h2 | `m-0 flex items-center gap-2 px-3 pt-3 text-kira-lg font-medium text-fg` |
| Mobile | `mobile/screens/PairScreen.vue:58` | h1 | `m-0 text-kira-lg font-medium` |
| Mobile | `mobile/screens/TerminalScreen.vue:151` | h1 | `m-0 min-w-0 flex-1 truncate text-kira-lg font-medium` |
| Runs | `workbench/src/automations/RunSecretsSection.vue:39` | h3 | `m-0 text-kira-lg font-medium` |
| Workbench | `workbench/src/llmPrompts/PromptDetail.vue:81` | h3 | `m-0 min-w-0 truncate text-kira-lg font-medium` |

### A.6 Checkbox with custom indicator or size class

34 uses: `studio/api/ApiVariablesTable.vue:114`, `studio/approvals/ReviewResultOption.vue:34`, `studio/project/DataGripImportDialog.vue:229`, `studio/project/FiltersDialog.vue:246`, `studio/project/FiltersDialog.vue:301`, `studio/project/connection/CheckboxRow.vue:12`, `studio/project/connection/ConnectionMcpTables.vue:138`, `studio/project/connection/ConnectionMcpTables.vue:177`, `studio/views/documents/ProjectionMenu.vue:108`, `studio/views/grid/ColumnsMenu.vue:166`, `studio/views/grid/ColumnsMenu.vue:177`, `studio/views/httprequest/RequestSettingsPane.vue:135`, `studio/views/httprequest/RequestSettingsPane.vue:170`, `studio/views/httprequest/RequestSettingsPane.vue:206`, `studio/views/httprequest/RequestSettingsPane.vue:240`, `studio/views/httprequest/RequestSettingsPane.vue:252`, `studio/views/httprequest/RequestSettingsPane.vue:272`, `studio/views/httprequest/RequestSettingsPane.vue:286`, `studio/views/httprequest/RequestSettingsPane.vue:308`, `studio/views/httprequest/RequestSettingsPane.vue:339`, `studio/views/httprequest/RequestSettingsPane.vue:351`, `studio/views/httprequest/ResponseHistoryList.vue:213`, `studio/views/shared/fields/FieldRowsTable.vue:288`, `studio/views/stream/StreamFilterRow.vue:252`, `git-ui/src/components/FileTree.vue:493`, `git-ui/src/components/FileTree.vue:530`, `workbench/src/automations/form/ParamsEditor.vue:232`, `workbench/src/automations/form/ParamsEditor.vue:275`, `workbench/src/automations/form/ParamsEditor.vue:286`, `workbench/src/automations/form/ToolsField.vue:73`, `workbench/src/automations/run/ParamsForm.vue:110`, `workbench/src/memory/distill/DistillResult.vue:99`, `workbench/src/memory/distill/DistillResult.vue:112`, `workbench/src/memory/import/ImportConfirmDialog.vue:188`

### A.7 ADE tone classes in `.vue` files (all lines; Part 7 sorts board from non-board)

56 lines: `ade/v2/AdeActivityIcon.vue:22`, `ade/v2/AdeActivityIcon.vue:28`, `ade/v2/AdeActivityIcon.vue:33`, `ade/v2/AdeActivityIcon.vue:38`, `ade/v2/context/AdeContextPane.vue:77`, `ade/v2/dialog/AdeAgentDialog.vue:225`, `ade/v2/links/AdeLinkStrip.vue:201`, `ade/v2/needs/AdeAllSessionRow.vue:30`, `ade/v2/needs/AdeAllSessionRow.vue:32`, `ade/v2/needs/AdeAllSessionRow.vue:33`, `ade/v2/needs/AdeAllSessionRow.vue:34`, `ade/v2/needs/AdeNeedsPage.vue:51`, `ade/v2/panel/AdeBranchPanel.vue:307`, `ade/v2/panel/AdeBranchPanel.vue:333`, `ade/v2/panel/AdeRunOutcome.vue:160`, `ade/v2/panel/AdeRunOutcome.vue:185`, `ade/v2/panel/AdeStepBlock.vue:90`, `ade/v2/panel/AdeStepBlock.vue:145`, `ade/v2/panel/AdeStepBlock.vue:146`, `ade/v2/panel/AdeStepBlock.vue:147`, `ade/v2/panel/AdeStepBlock.vue:148`, `ade/v2/panel/AdeStepBlock.vue:160`, `ade/v2/panel/AdeStepBlock.vue:161`, `ade/v2/panel/AdeStepBlock.vue:162`, `ade/v2/panel/AdeStepBlock.vue:163`, `ade/v2/panel/AdeStepBlock.vue:260`, `ade/v2/panel/AdeStepBlock.vue:265`, `ade/v2/panel/AdeTaskIssueLine.vue:98`, `ade/v2/plan/AdeBranchChips.vue:14`, `ade/v2/plan/AdeBranchChips.vue:25`, `ade/v2/plan/AdeBranchProg.vue:11`, `ade/v2/plan/AdeBranchProg.vue:15`, `ade/v2/plan/AdeBranchTip.vue:29`, `ade/v2/plan/AdeBranchTip.vue:30`, `ade/v2/plan/AdeBranchTip.vue:31`, `ade/v2/plan/AdeBranchTip.vue:34`, `ade/v2/plan/AdeDayBand.vue:91`, `ade/v2/plan/AdeDayBand.vue:97`, `ade/v2/plan/AdeDayBand.vue:172`, `ade/v2/plan/AdeDayBand.vue:183`, `ade/v2/plan/AdeStepMenu.vue:33`, `ade/v2/plan/AdeTaskCard.vue:96`, `ade/v2/plan/AdeTaskCard.vue:265`, `ade/v2/plan/AdeTaskChildren.vue:49`, `ade/v2/plan/AdeTaskSessions.vue:57`, `ade/v2/review/AdeReviewSync.vue:165`, `ade/v2/sessions/AdeSessionsRow.vue:133`, `ade/v2/settings/AdeSettingsPane.vue:77`, `ade/v2/workflows/AdeStepNode.vue:116`, `ade/v2/workflows/AdeStepNode.vue:283`, `ade/v2/workflows/AdeStepsTab.vue:134`, `ade/v2/workflows/AdeWorkflowsTab.vue:177`, `workbench/src/automations/RunFileDialog.vue:418`, `workbench/src/automations/ScriptBrokenRow.vue:30`, `workbench/src/automations/ScriptBrokenRow.vue:33`, `workbench/src/automations/ScriptRow.vue:78`

### A.8 SecondaryTabs (79)

| Module | File:line | variant | size |
|---|---|---|---|
| ADE | `ade/v2/AdeAddPopover.vue:188` | tabs | kira-lg |
| ADE | `ade/v2/AdeAddPopover.vue:224` | choice | kira-lg |
| ADE | `ade/v2/agents/AdeAgentsPage.vue:40` | tabs | default |
| ADE | `ade/v2/dialog/AdeAgentDialog.vue:241` | segmented | kira-lg |
| ADE | `ade/v2/links/AdeLinkCard.vue:122` | segmented | default |
| ADE | `ade/v2/needs/AdeAllSessions.vue:105` | segmented | kira-lg |
| ADE | `ade/v2/panel/AdeEstimateField.vue:82` | segmented | kira |
| ADE | `ade/v2/panel/AdePanelFrame.vue:19` | tabs | default |
| ADE | `ade/v2/review/AdeReviewWindow.vue:163` | tabs | default |
| ADE | `ade/v2/settings/AdeSettingsResult.vue:53` | tabs | default |
| ADE | `ade/v2/stage/AdeStageDialog.vue:170` | choice | kira-lg |
| ADE | `ade/v2/workflows/AdeLibraryStepEditor.vue:71` | segmented | kira-lg |
| ADE | `ade/v2/workflows/AdeStepInspector.vue:221` | tabs | default |
| ADE | `ade/v2/workflows/AdeWorkflowEditor.vue:38` | segmented | kira-lg |
| ADE | `ade/v2/workflows/AdeWorkflowsPage.vue:40` | tabs | kira-lg |
| ADE | `ade/v2/workflows/inspector/AdeStepAgentFields.vue:58` | tabs | default |
| ADE | `ade/v2/workflows/inspector/AdeStepRunFields.vue:100` | tabs | default |
| ADE | `ade/v2/workflows/inspector/AdeStepRunFields.vue:124` | tabs | default |
| ADE | `ade/v2/workflows/inspector/AdeStepSandbox.vue:97` | tabs | default |
| API | `studio/api/ApiVariablesDialog.vue:174` | tabs | default |
| Docker | `docker-ui/src/components/ContainerDetail.vue:232` | tabs | default |
| Docker | `docker-ui/src/components/ContainerEditView.vue:247` | tabs | default |
| Docker | `docker-ui/src/components/DockerPanel.vue:134` | tabs | default |
| Git | `repo/GitPanel.vue:159` | tabs | default |
| Git | `repo/GitPanel.vue:218` | tabs | default |
| Git | `repo/RepoFilePane.vue:94` | tabs | default |
| Git | `repo/RepoScriptField.vue:57` | tabs | default |
| Git | `views/repo/LocalFileView.vue:95` | tabs | default |
| Git | `views/repo/RepoDiffView.vue:314` | segmented | default |
| Git | `views/repo/RepoFileView.vue:201` | tabs | default |
| Git | `views/repo/ReviewThread.vue:254` | segmented | default |
| Git | `git-ui/src/components/FileTreeToolbar.vue:75` | segmented | default |
| Git | `git-ui/src/components/RefsPopover.vue:444` | tabs | default |
| Git | `git-ui/src/components/dialogs/StashDialog.vue:417` | choice | kira-lg |
| Git | `git-ui/src/components/dialogs/WorktreeDialog.vue:239` | choice | kira-lg |
| Git | `git-ui/src/components/review/ReviewFilesPane.vue:188` | segmented | default |
| Git | `git-ui/src/components/review/ReviewToolbars.vue:197` | tabs | default |
| Git | `git-ui/src/components/review/ReviewToolbars.vue:226` | segmented | default |
| Kubernetes | `studio/kubernetes/components/ClusterMcpDialog.vue:110` | choice | kira-lg |
| Kubernetes | `studio/kubernetes/components/KindBar.vue:61` | tabs | default |
| Kubernetes | `studio/kubernetes/components/ResourceTabPanel.vue:54` | tabs | default |
| Memory | `workbench/src/memory/ScopeField.vue:29` | segmented | kira-lg |
| Memory | `workbench/src/memory/distill/DistillView.vue:127` | segmented | default |
| Memory | `workbench/src/memory/distill/DistillView.vue:128` | segmented | default |
| Memory | `workbench/src/memory/import/ImportConfirmDialog.vue:159` | segmented | kira-lg |
| Memory | `workbench/src/memory/import/ImportView.vue:49` | tabs | default |
| Runs | `workbench/src/automations/RunFileDialog.vue:251` | tabs | default |
| Runs | `workbench/src/automations/form/ParamsEditor.vue:167` | segmented | kira-lg |
| Runs | `workbench/src/automations/form/RunFolderTab.vue:71` | choice | kira-lg |
| Runs | `workbench/src/automations/form/RunMcpTab.vue:77` | choice | kira-lg |
| Runs | `workbench/src/automations/form/RunMcpTab.vue:93` | choice | kira-lg |
| Runs | `workbench/src/automations/run/ParamsForm.vue:81` | choice | kira-lg |
| Settings | `workbench/src/settings/fields/RowDensityField.vue:49` | segmented | default |
| Shell | `hostview/CommentComposer.vue:88` | segmented | default |
| Studio | `studio/approvals/TabbedContent.vue:24` | text | default |
| Studio | `studio/project/ConnectionFileDialog.vue:248` | tabs | default |
| Studio | `studio/project/connection/ConnectionGeneralTab.vue:187` | segmented | kira-lg |
| Studio | `studio/project/connection/ConnectionMcpMasked.vue:53` | segmented | kira-lg |
| Studio | `studio/project/connection/ConnectionMcpReview.vue:78` | segmented | kira-lg |
| Studio | `studio/project/connection/ConnectionMcpTab.vue:259` | segmented | kira-lg |
| Studio | `studio/proof/ProofChangePanel.vue:82` | segmented | default |
| Studio | `studio/views/console/kibana/QueryBar.vue:67` | segmented | default |
| Studio | `studio/views/definition/DefinitionView.vue:289` | tabs | default |
| Studio | `studio/views/documents/DocumentToolbar.vue:94` | segmented | default |
| Studio | `studio/views/grid/DataToolbar.vue:309` | segmented | default |
| Studio | `studio/views/grid/PreviewCommandPanel.vue:117` | segmented | default |
| Studio | `studio/views/grpcrequest/GrpcRequestView.vue:370` | segmented | default |
| Studio | `studio/views/grpcrequest/GrpcRequestView.vue:433` | tabs | default |
| Studio | `studio/views/grpcrequest/ResponsePane.vue:335` | tabs | default |
| Studio | `studio/views/grpcrequest/SchemaBrowser.vue:123` | segmented | default |
| Studio | `studio/views/httprequest/HttpRequestView.vue:471` | tabs | default |
| Studio | `studio/views/httprequest/RequestBodyPane.vue:203` | segmented | default |
| Studio | `studio/views/httprequest/ResponsePane.vue:322` | tabs | default |
| Studio | `studio/views/httprequest/ResponsePane.vue:341` | tabs | default |
| Studio | `studio/views/shared/celleditor/TimestampPane.vue:140` | segmented | default |
| Studio | `studio/views/shared/keyvalue/KeyValueToolbar.vue:128` | segmented | default |
| Studio | `studio/views/stream/StreamToolbar.vue:283` | segmented | default |
| Workbench | `workbench/src/components/OpLogToolbar.vue:67` | segmented | default |
| Workbench | `workbench/src/llmPrompts/PromptDetail.vue:110` | tabs | default |

### A.9 Hand `text-error` text with a static class (RF.7; bound `:class` uses add 21 more, found by Part 3 grep)

21 uses: `ade/v2/port/AdePortImportDialog.vue:170`, `ade/v2/review/AdeReviewRemoved.vue:116`, `configsync/ConfigSyncUnitRow.vue:77`, `hostview/CommentComposer.vue:113`, `studio/project/DataGripImportDialog.vue:323`, `studio/project/TreeRow.vue:218`, `studio/project/TreeRow.vue:233`, `studio/proof/ProofScreenshotMenu.vue:100`, `studio/views/shared/page/SearchToolbar.vue:308`, `git-ui/src/components/StackList.vue:180`, `theme/src/components/InlineState.vue:13`, `workbench/src/automations/run/AdeContextFields.vue:53`, `workbench/src/automations/run/AdeContextFields.vue:94`, `workbench/src/automations/run/ParamsForm.vue:61`, `workbench/src/automations/runs/RunOutcomeBlock.vue:68`, `workbench/src/automations/runs/RunOutcomeBlock.vue:75`, `workbench/src/automations/runs/RunOutcomeBlock.vue:83`, `workbench/src/automations/runs/RunOutcomeBlock.vue:84`, `workbench/src/components/OpLogDetail.vue:143`, `workbench/src/components/OpLogPanel.vue:499`, `workbench/src/memory/distill/DistillView.vue:146`

### A.10 EmptyState with `#media` slot

12 uses: `studio/kubernetes/components/ClusterUnavailable.vue:49`, `studio/views/documents/DocumentView.vue:448`, `studio/views/shared/keyvalue/KeyValueRowsTable.vue:183`, `studio/views/stream/StreamRowsTable.vue:247`, `studio/views/stream/StreamRowsTable.vue:253`, `docker-ui/src/components/ConnectionState.vue:13`, `docker-ui/src/components/ContainerDetail.vue:120`, `docker-ui/src/components/UnavailableState.vue:52`, `git-ui/src/components/DetailPane.vue:106`, `git-ui/src/components/GitBlockedPanel.vue:23`, `git-ui/src/components/StashDetailPane.vue:105`, `workbench/src/components/QueryState.vue:78`

### A.11 Menus (every dropdown and context menu host)

Dropdowns (`DropdownMenuContent`, item build in parentheses):

- ADE: `AdeLeasesPane.vue` (hand items), `AdeStatusPicker.vue` (hand items), `AdeStepMover.vue` (hand items), `AdeTaskBranchMenu.vue` (hand items), `AdeStepMenu.vue` (hand items), `AdeTaskCard.vue` (hand items), `AdeTaskStatusMenu.vue` (hand items), `AdeReviewActions.vue` (hand items), `AdeStepNode.vue` (hand items), `AdeStepsTab.vue` (hand items), `AdeWorkflowGraph.vue` (hand items), `AdeStepSandbox.vue` (hand items)
- API: `EnvironmentSelect.vue` (hand items), `MethodSelect.vue` (hand items)
- Docker: `EndpointChip.vue` (hand items)
- Git: `AppToolbar.vue` (MenuItems), `PullStrategyPicker.vue` (MenuItems)
- Memory: `MemoryFilters.vue` (hand items), `ImportMenu.vue` (hand items)
- Studio: `MaskKindMenu.vue` (hand items), `ProofScreenshotMenu.vue` (hand items), `DiscoverSavedControls.vue` (hand items)
- Workbench: `ContextMenu.vue` (MenuItems), `OpLogToolbar.vue` (hand items)

Context menus (`openContextMenuAt` call sites, all data-driven `MenuItem[]`):

- ADE: `AdeBranchPanel.vue` (1), `useTaskMenu.ts` (1), `useReviewCode.ts` (1)
- API: `ApiNewTab.vue` (1), `CollectionsTree.vue` (1)
- Docker: `ContainerList.vue` (1)
- Git: `RepoFileTree.vue` (1), `RepoList.vue` (1), `FileTree.vue` (1), `useSectionMenu.ts` (1)
- Kubernetes: `ClusterList.vue` (1), `ResourceTable.vue` (1)
- Runs: `AutomationsNewTab.vue` (1)
- Studio: `ProjectTree.vue` (1), `BrowseView.vue` (1), `ConsoleResultGrid.vue` (2), `DocumentView.vue` (1), `KeyValueRowsTable.vue` (1), `StreamRowsTable.vue` (1)
- Workbench: `MenuIconButton.vue` (1), `OpLogPanel.vue` (1), `contextMenu.ts` (3)

### A.12 Lists, rows, tables

- ADE: `AdeAuditEntry.vue` (AdeRow 1), `AdeTimelineEntry.vue` (AdeRow 1), `AdeShelfRow.vue` (row 1), `AdeMainBranches.vue` (AdeRow 2), `AdeMainTab.vue` (AdeRow 6), `AdeStepBlock.vue` (AdeRow 3), `AdeStepMover.vue` (AdeRow 1), `AdeWorkflowBlock.vue` (AdeRow 1), `AdePortImportDialog.vue` (virtualRows 1), `AdeReposPage.vue` (row 3, virtualRows 1), `AdeStepsTab.vue` (row 1), `AdeWorkflowsTab.vue` (row 1)
- API: `CollectionRow.vue` (row 1)
- Docker: `ContainerGroupRow.vue` (row 1), `ContainerList.vue` (VirtualList 1), `ContainerOverview.vue` (row 1), `ContainerRow.vue` (row 1), `ContainerTable.vue` (virtualRows 1), `EngineDiskSection.vue` (Table 1), `EngineOverview.vue` (row 1), `ImageList.vue` (row 1, VirtualList 1), `NetworkList.vue` (row 1, VirtualList 1), `ResourceDetail.vue` (row 1), `VirtualList.vue` (virtualRows 1), `VolumeList.vue` (row 1, VirtualList 1)
- Git: `RepoList.vue` (row 1), `RepoSearchRow.vue` (row 2), `RepoSearchView.vue` (virtualRows 1), `RepoTreeRow.vue` (row 1), `RepoWorktreeRow.vue` (row 1), `CommitGrid.vue` (SlickGrid 25), `FileTree.vue` (row 1), `RefsPopoverBranches.vue` (row 2), `SearchResults.vue` (row 2), `StashRows.vue` (row 1), `TagList.vue` (row 1), `UncommittedChangesStrip.vue` (row 1, SlickGrid 1), `ReviewCommentsPane.vue` (row 1), `ReviewCommitList.vue` (virtualRows 1), `ReviewCommitRow.vue` (row 1)
- Kubernetes: `ClusterList.vue` (row 1), `ResourceTable.vue` (row 1, virtualRows 1)
- Memory: `MemoryContext.vue` (row 1), `MemoryPanel.vue` (row 1, virtualRows 1), `ImportFileRow.vue` (row 1), `ImportJobDetail.vue` (virtualRows 1)
- Proto: `SlickProto.vue` (SlickGrid 8), `TooltipProxy.vue` (SlickGrid 1)
- Runs: `ScriptBrokenRow.vue` (row 1), `ScriptGroupHeader.vue` (row 1), `ScriptRow.vue` (row 1), `RunsSection.vue` (row 1)
- Settings: `ConfigSyncPane.vue` (Table 1)
- Studio: `PendingResponse.vue` (Table 1, virtualRows 1), `TreeRow.vue` (row 1), `ConnectionMaskReview.vue` (virtualRows 1), `ConnectionMcpTables.vue` (virtualRows 1), `ProofChangePanel.vue` (row 1), `BrowseView.vue` (row 1, virtualRows 1), `ConsoleResultGrid.vue` (row 2, virtualRows 2, SlickGrid 9), `ConsoleSavedMenu.vue` (row 1), `ConsoleSlickGrid.vue` (SlickGrid 55), `ExplainResultView.vue` (row 1), `DiscoverSavedMenu.vue` (row 1), `FieldSidebar.vue` (row 1), `TimePicker.vue` (row 1), `ColumnsSection.vue` (Table 1), `ConstraintsSection.vue` (Table 1, SlickGrid 1), `DefinitionView.vue` (SlickGrid 1), `IndexesSection.vue` (Table 1), `PropertiesSection.vue` (Table 1), `ValidationSection.vue` (Table 1), `DocumentView.vue` (virtualRows 1), `ProjectionMenu.vue` (row 1), `ColumnsMenu.vue` (row 1), `DataToolbar.vue` (SlickGrid 4), `DataView.vue` (SlickGrid 4), `FilterToolbar.vue` (SlickGrid 3), `FkPreviewPopover.vue` (Table 1, SlickGrid 4), `SlickGridHost.vue` (SlickGrid 87), `CallHistoryList.vue` (row 1), `ResponsePane.vue` (row 1, virtualRows 1), `SchemaBrowser.vue` (row 2), `ResponseDiffDialog.vue` (Table 2), `ResponseHistoryList.vue` (row 1), `FilterHistoryMenu.vue` (row 1), `SavedListMenu.vue` (row 2), `CellEditorView.vue` (SlickGrid 2), `CellGenerateMenu.vue` (row 1), `DocumentRow.vue` (row 1), `KeyValueRowsTable.vue` (row 1, virtualRows 1), `StreamFilterRow.vue` (row 1), `StreamRowsTable.vue` (row 1, virtualRows 1), `GenerateDataDialog.vue` (Table 1), `StudioStart.vue` (row 1)
- Theme: `AttributeTooltip.vue` (SlickGrid 9), `TooltipAnchorBridge.vue` (SlickGrid 1)
- Workbench: `AutocompleteField.vue` (row 1), `LogLines.vue` (virtualRows 1), `OpLogPanel.vue` (virtualRows 1), `SettingsShell.vue` (row 1), `WorkbenchShell.vue` (SlickGrid 1), `PromptsPane.vue` (row 1)

### A.13 Forms (files with `Field`/`FieldSet`; label mix `FieldLabel`/`Label`)

- ADE: `AdeSettingsSecrets.vue` (F1 1/0), `AdeTaskStatuses.vue` (F2 0/2)
- API: `ApiSecrets.vue` (F1 1/0), `EnvironmentFields.vue` (F3 3/0), `SaveRequestDialog.vue` (F2 0/2)
- Docker: `EditPendingSummary.vue` (F0 S1 0/0), `RecreateConfirmDialog.vue` (F0 S2 0/0)
- Git: `RepoConfigForm.vue` (F5 S4 5/0), `RepoIdentity.vue` (F1 S1 0/0), `RunScriptRefPicker.vue` (F1 1/0), `BranchDialog.vue` (F2 2/0), `ForcePushDialog.vue` (F2 2/0), `PreflightPrediction.vue` (F1 1/0), `RenameRefDialog.vue` (F1 1/0), `RepoSettingsDialog.vue` (F8 S6 8/0), `ResetDialog.vue` (F2 S1 2/0), `StackDialog.vue` (F1 1/0), `StashDialog.vue` (F5 S1 5/0), `TagDialog.vue` (F4 4/0), `WorktreeDialog.vue` (F5 S1 5/0)
- Kubernetes: `ClusterMcpDialog.vue` (F3 2/1)
- Memory: `AgentMcpSection.vue` (F0 S1 0/0), `CheckHookSection.vue` (F0 S1 0/0), `SemanticModelSection.vue` (F0 S1 0/0)
- Runs: `RunFileDialog.vue` (F1 0/1), `McpToolsField.vue` (F0 S1 0/1), `ParamsEditor.vue` (F0 S1 0/2), `RunFolderTab.vue` (F2 1/1), `RunMcpTab.vue` (F5 4/1), `RunScriptTab.vue` (F2 2/0), `ScheduleFields.vue` (F5 3/2), `ScriptBodyField.vue` (F1 1/0), `SmartSettingsFields.vue` (F4 3/1), `ToolsField.vue` (F0 S1 0/1), `AdeContextFields.vue` (F2 2/0), `ParamsForm.vue` (F1 1/1), `RunScriptDialog.vue` (F1 2/0), `ScheduleConfirmDialog.vue` (F1 1/0)
- Settings: `AdvancedPane.vue` (F3 0/3), `AgentPane.vue` (F6 0/5), `AppearancePane.vue` (F2 0/2), `CapturePane.vue` (F2 0/2), `ConfigSyncPane.vue` (F3 0/3), `DateFormatField.vue` (F1 0/1), `ExperimentalPane.vue` (F1 0/1), `FileTypesPane.vue` (F1 0/1), `GitPane.vue` (F4 S2 0/4), `MemoryPane.vue` (F1 0/1), `MobileAccessPane.vue` (F3 S3 0/5), `UpdatesPane.vue` (F1 S1 0/1), `FontFamilyField.vue` (F1 0/1), `FontSizeField.vue` (F1 0/1), `LogLevelField.vue` (F1 0/1), `NotifyPromptsField.vue` (F1 0/1), `RowDensityField.vue` (F1 0/0), `WordWrapField.vue` (F1 0/1)
- Studio: `NewConnectionDialog.vue` (F2 2/0), `ConnectionAdvancedTab.vue` (F2 2/0), `ConnectionGeneralTab.vue` (F12 12/0), `ConnectionMcpMasked.vue` (F1 1/0), `ConnectionMcpReview.vue` (F1 1/0), `ConnectionMcpTab.vue` (F3 3/0), `ConnectionMcpTables.vue` (F1 1/1), `ConnectionPreconnectTab.vue` (F2 2/0), `RequestSettingsPane.vue` (F7 0/14), `UploadObjectDialog.vue` (F2 0/2), `ApiAccessRuleDialog.vue` (F3 2/3), `ApiAccessRulesSection.vue` (F1 0/1), `ApiPane.vue` (F7 0/7), `CachePane.vue` (F3 0/3), `DataPane.vue` (F1 0/1), `StudioMcpApiPane.vue` (F2 0/2), `StudioMcpServerPane.vue` (F3 0/3)
- Workbench: `PromptArgumentField.vue` (F1 1/0)

Module-local form layouts outside `Field`: `AdeInspectorRow` hosts, `EditField` hosts, settings `fields/*` panes (see §4.1); Part 3 lists their call sites by grep.
