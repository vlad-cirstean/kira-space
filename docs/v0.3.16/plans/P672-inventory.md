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
  500). Bare `Label` only wraps a checkbox/switch/radio text in a horizontal `Field`, or is
  `sr-only`. No class on either except `sr-only`, `font-data` (label is data, e.g. param name).
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

<!-- sections 5.. land in following commits -->
