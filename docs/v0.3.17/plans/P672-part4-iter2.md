# P672 Part 4 iter2: buttons and dialogs, second pass

Iteration 2 of `P672 Part 10` (master loop: `plans/P672-part10.md`). Re-opens `P672 Part 4: buttons
and dialogs` against current tree (base `b33dd339a`; later commits up to `428b6b6c6` touch docs only).
Fresh scan; first-pass summaries not trusted. One sequential Sonnet implementer. Accessibility is
not a goal: no a11y-only item planned (`:aria-describedby="undefined"` on git dialogs, redundant
`aria-label` equal to text, left as is).

## Rules applied

ARCHITECTURE "UI uniformity rules (P672)", dialog and button bullets (lines 1456-1457), plus the
first-pass plan `plans/P672-part4.md`:

- RD.1 size by content: sm confirm or one field; md form up to ~8 fields; lg sections or list picker;
  xl two panes, diff, preview; 2xl fixed-height editor or multi-tab config; full review windows only.
- RD.2 footer: `#start` secondary, then dismiss, alternate, one emphasised button (dialog-primary,
  dialog-danger, dialog-approve) rightmost. Dismiss (Cancel, Close, Done, Dismiss) is `dialog`.
- RD.3 header: DialogTitle plus optional DialogDescription for intro text.
- RD.5 body keeps dialogBodyClass; only `flush` and `min-h-*`. `flush` is for edge-to-edge content,
  never re-added padding.
- RD.6 failure is an error Alert as last body child; FieldError only for one field.
- RD.7 destructive confirm: `DialogContent destructive`, dismiss carries `data-dialog-dismiss`.
- RB.1 size by host: form commit kira-lg; bars, rows, popover rows kira or TooltipIconButton; link
  inline.
- RB.2 outline (dialog*) only commits a form or answers a question; every other in-view action is
  toolbar with a leading 13px CodiconIcon. Labelled destructive commit (footer, confirm strip) is
  dialog-danger; in-view one is toolbar + `data-tone="error"`.
- RB.3 text action in prose, Alert or EmptyState is `link@inline`; failure EmptyState Retry is
  toolbar + `refresh`; banner decision stays `dialog@kira`.
- Library: theme Button, Kbd, Alert, Field primitives; Tailwind utilities; icon names from
  `packages/theme/src/icons.ts` `ICON` where an entry exists. No Pinia or TanStack Query change.

## Current state (scans at `b33dd339a`)

709 non-`components/ui/` `.vue` files in the six guard DIRS, parsed into tag trees (scratchpad
parser, quote-aware, multi-line).

- 62 DialogContent: md 17, sm 15, lg 15, xl 8, 2xl 6, full 1. 60 DialogFooter.
- Button (variant@size) top counts: toolbar@kira 208, dialog@kira-lg 137, dialog-primary@kira-lg 82,
  link@inline 80, dialog@kira 35, dialog-primary@kira 23, toolbar@kira-icon 21, toolbar@inline 14,
  toolbar@status 12, dialog-danger@kira-lg 11.
- Labelled toolbar@kira Buttons whose first child is not CodiconIcon, Spinner or EngineIcon: 93
  (92 after dropping aria-pressed toggles and chevron-down triggers; 62 files).
- 20 `<Kbd>` inside a Button, in 8 files, each pairing a hand `:aria-keyshortcuts` with a hand Kbd.
- 4 save bars hand-roll discard (dialog-danger) beside save (dialog-primary): two emphasised buttons.
- 6 footers style `#start` text by hand (4 shapes); 5 prompt dialogs render "N more waiting" 3 ways.
- `bash scripts/check-ui-primitives.sh` green; highest guard U133; PERMANENT 48 lines.

Dry runs of every new guard regex below ran against the tree with `grep -Pzq`; hit lists match the
findings exactly.

## Findings

Line numbers are tag start lines at `b33dd339a`.

### F1 ConnectionFileDialog footer has three emphasised buttons (RD.2)

`apps/kira-space/frontend/src/studio/project/ConnectionFileDialog.vue`: `#start` delete
`dialog-danger` (~392), main discard `dialog-danger` (~430), save `dialog-primary`. Siblings
ApiVariablesDialog (316) and RunFileDialog (422) keep one emphasised button.

Change: delete and discard become `variant="dialog"`, each preceded by
`<!-- conventions-allow N17: second footer action; one emphasised button rightmost (RD.2) -->`.
Delete keeps its `trash` icon. Save stays `dialog-primary`.

### F2 Lone dismiss rendered as dialog-primary (RD.2)

Dismiss is not a commit. Siblings (AdePortBackupsDialog, ResponseDiffDialog, DynamicValuesDialog,
UpdateDialog checkFailed) use `dialog`.

- `apps/kira-space/frontend/src/studio/project/DataGripImportDialog.vue:369` report close.
- `packages/workbench/src/memory/AddMemoryDialog.vue:225` Done.
- `packages/workbench/src/components/UpdateDialog.vue:174` upToDate close.
- `apps/kira-space/frontend/src/capture/CaptureWindow.vue:95` Dismiss (window footer row).

Change: `variant="dialog-primary"` to `variant="dialog"`. Size unchanged.
Also DataGripImportDialog footer order `[chooseAnother 340][cancel 349][import 357]` becomes
`[cancel][chooseAnother][import]` (dismiss before alternate).

### F3 GenerateDataDialog running status in main footer slot (RD.2)

`apps/kira-space/frontend/src/studio/workbench/GenerateDataDialog.vue:434`: TextTooltip + Spinner
span sits between footer buttons. Change: wrap it in `<template v-if="running" #start>`; drop its
own `v-if`. Keep the span wrapper and `text-info` (Spinner tone).

### F4 Footer `#start` status text styled by hand, four shapes

- `span.text-kira-sm.text-muted-foreground`: GitCredentialDialog
  (`apps/kira-space/frontend/src/workbench/GitCredentialDialog.vue:107`), ScheduleConfirmDialog
  (`packages/workbench/src/automations/schedule/ScheduleConfirmDialog.vue:232`).
- `span.text-kira-sm.text-subtle`: ConnectionFileDialog `connection-save-first` (~401).
- `span.text-subtle.text-kira-sm.leading-normal`: SettingsDialog `#footerStart`
  (`apps/kira-space/frontend/src/workbench/SettingsDialog.vue:276`, rendered inside SettingsShell's
  DialogFooter `#start`).
- FieldDescription: SchemaDialog (`apps/kira-space/frontend/src/studio/project/SchemaDialog.vue:190`),
  FiltersDialog (`apps/kira-space/frontend/src/studio/project/FiltersDialog.vue:332`),
  DataGripImportDialog (333, with `font-data`).

Change:

- Theme: `packages/theme/src/components/ui/dialog/DialogFooter.vue` start wrapper class becomes
  `mr-auto flex min-w-0 items-center gap-1.5 text-kira-sm text-subtle`. Buttons, Badge and FieldError
  set their own type and colour (buttonVariants base `text-kira-md`, variants set colour), so only
  bare text inherits.
- Hosts: each listed element becomes a bare `<span>` keeping its testid and content. DataGrip keeps
  `class="font-data"`. FieldError branches stay. Drop now-unused FieldDescription imports.

### F5 "N more waiting" queue count rendered three ways

Footer `#start` muted span (GitCredentialDialog, ScheduleConfirmDialog) is the majority shape.
Outliers render it as a body paragraph:

- `apps/kira-space/frontend/src/studio/project/MaskingReviewPrompt.vue:54` `p.m-0.text-muted-foreground`
  (only child of DialogBody 53).
- `apps/kira-space/frontend/src/studio/proof/ProofReadyPrompt.vue:55` same (DialogBody 54 exists only
  for it).
- `apps/kira-space/frontend/src/workbench/PairingRequestDialog.vue:52` `p.m-0.text-subtle`
  (`${testidPrefix}-queue-count`).

Change: each moves to `<template v-if="more > 0" #start><span data-testid=…>` in its DialogFooter,
testids kept. MaskingReviewPrompt and ProofReadyPrompt drop their now-empty DialogBody (and import).
PairingRequestDialog keeps its body (slot + expires line).

### F6 Shortcut hint hand-rolled inside Buttons (20 Kbd, 8 files)

Each site repeats `:aria-keyshortcuts="formatShortcut(id)"` plus `<Kbd>{{ formatShortcut(id) }}</Kbd>`;
UpdateDialog omits the attribute, AdeStageDialog makes both conditional. TooltipIconButton already
centralises this as a `kbd` prop; Button has no equivalent.

Sites: GitCredentialDialog 111/121; PairingRequestDialog 57/69; MaskingReviewPrompt 58/69;
ProofReadyPrompt 59/70; UpdateDialog 162/173/184/193; ScheduleConfirmDialog 237/249;
`packages/workbench/src/approvals/ApprovalItemActions.vue` 53/65;
`apps/kira-space/frontend/src/ade/v2/stage/AdeStageDialog.vue:201`.

Change:

- Theme `packages/theme/src/components/ui/button/Button.vue`: new prop `kbd?: string` (doc comment
  as TooltipIconButton: "Shortcut text (callers format it), shown as a Kbd after the label").
  Renders `<Kbd v-if="kbd && !asChild">{{ kbd }}</Kbd>` after `<slot />` and binds
  `:aria-keyshortcuts="kbd || undefined"` on the Primitive.
- Hosts: replace attribute + Kbd pair with `:kbd="formatShortcut('<id>')"`. AdeStageDialog:
  `:kbd="routed ? formatShortcut('form.submit') : undefined"`. Drop unused Kbd imports.

### F7 WorktreeList remove confirm diverges from sibling confirm dialogs (RD.3, RD.6, RD.7, RF)

`packages/git-ui/src/components/WorktreeList.vue:190-232`. Siblings ForcePushDialog, ResetDialog,
RevertDialog, CherryPickDialog, RecreateConfirmDialog, ConfirmDialogView.

Change:

- `DialogContent` (191) gains `destructive`; Cancel (217) gains `data-dialog-dismiss`.
- Intro `<p><CatalogText k="git.worktree.removeConfirm.body">` (204) moves to a
  `<DialogDescription v-if="pendingRemove?.preflight.verdict === 'dirty'">` in DialogHeader.
- Blocked `<FieldError>` (201) becomes `<Alert variant="error"><AlertDescription>`.
- Token Input (210) wrapped in `Field` + `FieldLabel :for` with a `useId()` id (ForcePushDialog
  shape). Rename catalog key `git.common.confirmToken.aria` to `git.common.confirmToken.label` in
  `packages/theme/src/i18n/en.json` (sole user is this file).
- Leave `:aria-describedby="undefined"` (a11y-only).

### F8 Dialog failure rendered as FieldError, not an error Alert (RD.6)

- `apps/kira-space/frontend/src/studio/workbench/apiaccess/ApiAccessRuleDialog.vue:160` save error.
- `apps/kira-space/frontend/src/studio/kubernetes/components/ClusterMcpDialog.vue:119` save error.
- `packages/workbench/src/components/UpdateDialog.vue:96` checkFailed, `:134` install error.

Change: `<Alert variant="error" data-testid=…><AlertDescription>…</AlertDescription></Alert>`, testid
on Alert. UpdateDialog 96 stays in its v-else-if chain. Not changed (field or row level, correct):
ScheduleConfirmDialog 218/221 and RunScriptDialog 248/251 (preview fill-in), AdeRunDialog 243,
AdePortImportDialog 170 (row), RunFileDialog 325/363 (field).

### F9 `flush` body plus hand `p-3`, and doubled padding (RD.5)

- `apps/kira-space/frontend/src/studio/project/DataGripImportDialog.vue:194` `DialogBody flush`;
  both branches (195, 301) `div.flex.flex-col.gap-2.p-3`. Change: plain `<DialogBody>`, branch divs
  drop `p-3`.
- `apps/kira-space/frontend/src/studio/project/NewConnectionDialog.vue:126` flush; details div 128
  `gap-3 p-3`; `connection/ConnectionEnginePicker.vue:27` root `p-3`. Change: plain DialogBody; drop
  `p-3` from the div and from the picker root (picker's other host, ConnectionGeneralTab, loses its
  own `p-3` below, so padding stays single there too).
- ConnectionFileDialog body (268, plain, p-3) wraps five tab roots that each add `p-3`:
  `connection/ConnectionGeneralTab.vue:128`, `ConnectionAdvancedTab.vue:81`,
  `ConnectionPreconnectTab.vue:53`, `ConnectionMcpTab.vue:181`, `ConnectionPrivacyTab.vue:46`
  (all under `apps/kira-space/frontend/src/studio/project/`). Sibling RunFileDialog tabs carry none.
  Change: drop `p-3` from each root (only host is ConnectionFileDialog; codegraph-checked).

### F10 Save bars hand-rolled four times, each with two emphasised buttons (RD.2, RB.2)

- `apps/kira-space/frontend/src/ade/v2/workflows/AdeWorkflowSaveBar.vue` (callers AdeWorkflowGraph
  290, AdeWorkflowYaml 136, AdeLibraryStepYaml 136, AdeLibraryStepForm 81).
- `apps/kira-space/frontend/src/ade/v2/settings/AdeSettingsPane.vue:74-105`.
- `apps/kira-space/frontend/src/ade/v2/context/AdeContextPane.vue:74-105`.
- `apps/kira-space/frontend/src/repo/RepoFilePane.vue:144-172` (copy-shared `mr-auto`, create label).

Change: new `packages/workbench/src/components/SaveBar.vue` (`<script setup lang="ts">`, Tailwind
only):

- Props: `dirty: boolean`, `saving: boolean`, `canSave?: boolean` (default `dirty`),
  `testidPrefix: string`, `saveLabel?: string` (default `common.action.save.label`).
- Emits `save`, `discard`.
- Template: root `div.flex.flex-wrap.items-center.gap-2` with `data-testid="${prefix}-savebar"`;
  `div.mr-auto.flex.min-w-0.flex-1.items-center.gap-2.text-kira-sm.text-subtle` holding
  `<slot name="start" />`; discard `dialog@kira-lg` (one N17 marker, reason as F1), disabled
  `!dirty || saving`, testid `${prefix}-discard`; save `dialog-primary@kira-lg`, disabled `!canSave`,
  `:pending="saving"`, testid `${prefix}-save`.
- Delete AdeWorkflowSaveBar.vue; its 4 callers use SaveBar `testid-prefix="ade-wf"` with the applies
  hint in `#start`.
- AdeSettingsPane / AdeContextPane: `#start` holds the size span (keeps `data-testid` and
  `:class="over && 'text-error'"`); prefixes `ade-settings`, `ade-context`; `:can-save="dirty"`.
- RepoFilePane: `#start` holds copy-shared Button (drop `mr-auto`); prefix `repo-file`;
  `:can-save="draft.canSave.value"`; `:save-label` create or save.

### F11 Inline commit-row and confirm-strip buttons off pattern (RB.2)

Commit-row Cancel is `dialog` at the row's commit size in 9 sites (AdeNoteEntry, AdeTaskChain,
CommentComposer, KeyValue popovers, StreamCompose, CredentialPaste, git-ui App.vue 1954,
ConfigSyncReviewWindow). Outliers use toolbar without icon:

- `apps/kira-space/frontend/src/views/repo/ReviewThread.vue:282`.
- `apps/kira-space/frontend/src/studio/views/console/kibana/FilterEditor.vue:153`.
- `apps/kira-space/frontend/src/studio/views/documents/DocumentEditor.vue:53` (save `dialog-primary`
  ~40).
- `packages/git-ui/src/components/RefsPopoverBranches.vue:259` (popover confirm beside
  dialog-danger).

Change: `variant="dialog"`, size matching the row's commit button.

`packages/git-ui/src/components/review/ReviewCommentsPane.vue:120-131` confirm-clear strip is two
toolbar buttons, confirm first. Change: order Cancel then Confirm; Cancel `dialog@kira`, Confirm
`dialog-danger@kira`; drop `text-graph-md` from the strip div.

### F12 Alert and EmptyState text actions styled as toolbar (RB.3)

33 siblings use `link@inline`. Change to `variant="link" size="inline"`:

- `apps/kira-space/frontend/src/views/repo/RepoFileView.vue:228`, `:231` (reload, overwrite).
- `apps/kira-space/frontend/src/studio/views/httprequest/ResponsePane.vue:369`.
- `apps/kira-space/frontend/src/studio/views/grpcrequest/ResponsePane.vue:359`.
- `packages/docker-ui/src/components/ContainerEditView.vue:216` (refresh in Alert).
- `apps/kira-space/frontend/src/studio/views/shared/page/SearchFilterNotice.vue:15` (EmptyState).
- `packages/docker-ui/src/components/ContainerDetail.vue:127` (EmptyState, back to overview).

`packages/git-ui/src/components/ConflictBanner.vue:94/107/117` continue/skip/abort: git operation
actions, sibling AdeRunOutcome rebase acts (toolbar + icon, abort `data-tone="error"`). Change: keep
toolbar@kira, add leading icons `debug-continue`, `debug-step-over`, `debug-stop`.

### F13 Labelled toolbar buttons without leading icon (RB.2)

Add `<CodiconIcon name="…" :size="13" />` as first child (import CodiconIcon where missing):

- Failure EmptyState Retry, `refresh`: `apps/kira-space/frontend/src/hostview/HostViewFrame.vue:86`,
  `packages/workbench/src/components/QueryState.vue:85`, `packages/git-ui/src/App.vue:1729`,
  `packages/git-ui/src/components/NoRepositoryPanel.vue:96`,
  `packages/docker-ui/src/components/ConnectionState.vue:22`.
- Retry `refresh`: `ade/v2/links/AdeLinkStrip.vue:189`, `packages/workbench/src/memory/import/ImportFileRow.vue:65`.
- Clear `clear-all`: `studio/views/grid/FilterToolbar.vue:249`, `studio/views/stream/StreamFilterRow.vue:323`,
  `studio/views/documents/DocumentFilterRow.vue:188`, `packages/docker-ui/src/components/LogsView.vue:231`,
  `packages/workbench/src/memory/MemoryFilters.vue:106`.
- All / None `check-all` / `clear-all`: `studio/views/grid/ColumnsMenu.vue:134/137`,
  `studio/views/documents/ProjectionMenu.vue:83/92`, `packages/workbench/src/restore/RestoreDialog.vue:71/81`.
- Copy `copy`: `ade/v2/panel/AdeRunOutcome.vue:141`, `packages/workbench/src/components/CopyLogButton.vue:37`,
  `packages/workbench/src/memory/distill/DistillRewriteResult.vue:91`, `DistillResult.vue:137`,
  `packages/git-ui/src/components/UndoButton.vue:46` (recovery sha).
- Show/hide log `output`: `ade/v2/panel/AdeRunOutcome.vue:201`, `ade/v2/review/AdeWorkflowReviewPanel.vue:348`.
- Reopen / mark addressed `issue-reopened` / `check` (by state): `ade/v2/review/AdeWorkflowReviewPanel.vue:230`,
  `views/repo/ReviewThread.vue:199`.
- Edit `edit`: `views/repo/RepoFileEditBar.vue:15`, `studio/api/VariableSetView.vue:135`.
- RepoFileEditBar save `save` (31), done `check` (46).
- Reply `reply`: `hostview/HostViewConversation.vue:118`.
- Choose file `folder-opened`: `studio/views/httprequest/FormDataTable.vue:139`,
  `studio/views/httprequest/BinaryBodyPicker.vue:40`, `studio/views/grpcrequest/SchemaBrowser.vue:151`.
- Add `add`: `studio/views/grpcrequest/SchemaBrowser.vue:203`, `studio/api/ApiVariablesTable.vue:133`,
  `packages/docker-ui/src/components/EditRows.vue:29`, `packages/git-ui/src/components/WorktreeList.vue:125`.
- Compare `diff`: `studio/views/httprequest/ResponseHistoryList.vue:148`.
- Environments `server-environment` (ICON.environments) 240, variables `variable-group` (ICON.variables)
  232: `studio/api/EnvironmentsView.vue`.
- Reconnect `plug`: `studio/kubernetes/components/LogsView.vue:239`.
- Change engine `arrow-swap`: `studio/project/connection/ConnectionGeneralTab.vue:160`.
- Jump to latest `arrow-down`: `packages/docker-ui/src/components/LogsView.vue:276`.
- Measure `dashboard`: `packages/docker-ui/src/components/ContainerSizeSection.vue:50`,
  `EngineDiskSection.vue:82`.
- Attach `plug`: `packages/docker-ui/src/components/EditInPlaceSection.vue:429`.
- Body search `search`: `packages/git-ui/src/components/SearchResults.vue:134`.
- Restack `layers`: `packages/git-ui/src/components/StackList.vue:97`.
- Load more `fold-down`: `packages/git-ui/src/components/LoadMoreButton.vue:63`,
  `review/ReviewCommitList.vue:186`. LoadMoreButton Cancel (69) `debug-stop`; drop `class="underline"`
  and the G34 comment.
- Memory import job (`packages/workbench/src/memory/import/ImportJobDetail.vue`): start 113 `play`,
  pause 123 `debug-pause`, resume 133 `debug-continue`, cancel 143 `debug-stop`, retry failed 153
  `refresh`, dismiss 162 `close`, discard 172 `trash` + `data-tone="error"`.
- Back `arrow-left` (ICON.back): `packages/workbench/src/memory/import/ImportView.vue:40`,
  `distill/DistillView.vue:118`.
- View `eye`: `packages/workbench/src/memory/import/ImportStatus.vue:62`.
- Distill store `save` (147), import `cloud-download` (157, memory module's import icon):
  `packages/workbench/src/memory/distill/DistillResult.vue`.
- Remove queue `trash` + `data-tone="error"`: `ade/v2/agents/AdeLeasesPane.vue:79`.

Paths without prefix are under `apps/kira-space/frontend/src/`.

Not icons, other shape:

- `ade/v2/agents/AdeLeasesPane.vue:95` holder DropdownMenuTrigger shows the current value: add
  trailing `<CodiconIcon name="chevron-down" :size="13" data-icon="inline-end" />` (AdeSessionsMenu,
  MaskKindMenu precedent).
- `packages/docker-ui/src/components/ContainerEditView.vue:284` Apply and `:317` Recreate commit the
  edit form: `dialog-primary@kira-lg` (RB.1 form commit).

### Checked, no change

- Sizes RD.1: all 62 fit. ResponseDiffDialog 2xl fixed-height holds a fixed-height Monaco diff
  editor (2xl "fixed-height editor"); AdePortExport/Import 2xl multi-section config; NewConnection
  2xl matches ConnectionFileDialog.
- ViewToolbar tab strip between header and body in ApiVariablesDialog, ConnectionFileDialog,
  RunFileDialog: same in all three.
- `:dirty="false"`: required by U100.
- Commit-row gaps (gap-1, 1.5, 2): lead only, not a finding.

## Exemptions re-checked

First-pass exceptions stand: IssueAttachments `dialog@inline`; MaskKindMenu select-like trigger;
DataView TooltipIconButton dialog-primary; CredentialsUpdateDialog with no footer; ForcePushDialog
plain confirm; outline banner decisions (FailureBanner, RestoreBanner, RunsRestoreBanner,
CopyAsCurlDialog, App.vue boot retries, ReviewView retries, ConnectionMcpTables retry); popover
confirms; front-door EmptyStates. Master deliberate exceptions (git ViewToolbar detail head, Studio
tab head, AdeTaskCard D1, bar segmented toggles, engine cards, toasts D3) untouched.

New built-in exempt names for U134, each with a requirement:

- `DateTimePicker.vue`: month label and calendar cells are picker cells, not actions.
- `AdeSessionStrip.vue`: session tab chips select a session.
- `DynamicValuesDialog.vue`: copy rows render as Buttons (row shape, rows scope).
- `AdeTaskStatuses.vue`: colour swatch trigger shows the value as a dot.

## Theme pieces

- `packages/theme/src/components/ui/button/Button.vue`: `kbd` prop (F6).
- `packages/theme/src/components/ui/dialog/DialogFooter.vue`: `#start` text style (F4).
- No index export change (props only; SaveBar lives in workbench).

## Guards (`scripts/check-ui-primitives.sh`)

All land after the fixes with zero offenders. `(?&t)` and TOK as defined in the script. Closing tags
use `</X\s*>` (prettier wraps `</Button\n>`).

- U118 extend in place, add alternative: `|<DialogBody\b(?=(?&t)\sflush\b)(?&t)>\s*(?:<template\b(?&t)>\s*)?<div\b(?=(?&t)\sclass="[^"]*${TOK}p-3\b)`;
  message gains "; flush body never re-adds p-3". Hits now: DataGripImportDialog.
- U134 v, exempt `DateTimePicker.vue|AdeSessionStrip.vue|DynamicValuesDialog.vue|AdeTaskStatuses.vue`,
  "labelled toolbar Button without leading icon (RB.2; CodiconIcon 13 first, select-like trigger
  ends chevron-down)":
  `<Button\b(?=(?&t)\svariant="toolbar")(?=(?&t)\ssize="kira")(?!(?&t)\s:?aria-pressed=)(?&t)>(?!(?:(?!</Button\s*>)[\s\S])*?name="chevron-down")\s*(?!<(?:CodiconIcon|Spinner|EngineIcon)\b)(?!</Button)\S`.
  Hits now: 62 files (F11-F13).
- U135 v, "Kbd inside Button (Button kbd prop)": `<Button\b(?&t)>(?:(?!</Button\s*>)[\s\S])*?<Kbd\b`.
  Hits now: 8 files (F6).
- U136 v, "styled text in DialogFooter (#start sets text-kira-sm text-subtle; bare span)":
  `<DialogFooter\b(?&t)>(?:(?!</DialogFooter\s*>)[\s\S])*?(?:<FieldDescription\b|<span\b(?=(?&t)\sclass="[^"]*${TOK}text-(?:kira-sm|muted-foreground|subtle)\b))`.
  Hits now: 6 files (F4).
- U137 v, "dialog-danger footer without DialogContent destructive (RD.7)":
  `<DialogContent\b(?!(?&t)\s:?destructive\b)(?&t)>(?:(?!</DialogContent\s*>)[\s\S])*?<DialogFooter\b(?&t)>(?:(?!</DialogFooter\s*>)[\s\S])*?variant="dialog-danger"`.
  Hits now: ConnectionFileDialog (F1), WorktreeList (F7).
- U138 v, "FieldError as last DialogBody child (failure is an error Alert, RD.6)":
  `</FieldError\s*>\s*(?:</template\s*>\s*)*</DialogBody\s*>`. Hits now: ApiAccessRuleDialog,
  ClusterMcpDialog, UpdateDialog (F8).
- U139 v, "two adjacent emphasised buttons (one emphasised button rightmost, RD.2)":
  `<Button\b(?=(?&t)\svariant="dialog-(?:danger|primary|approve)")(?&t)>(?:(?!</Button\s*>)[\s\S])*</Button\s*>\s*(?:<!--(?:(?!-->)[\s\S])*-->\s*)?<Button\b(?=(?&t)\svariant="dialog-(?:danger|primary|approve)")`.
  Hits now: AdeContextPane, AdeSettingsPane, AdeWorkflowSaveBar, RepoFilePane, ConnectionFileDialog
  (F1, F10).
- U140 v, "dismiss label on an emphasised button (Close, Done, Dismiss are dialog)":
  `<Button\b(?=(?&t)\svariant="dialog-(?:primary|approve|danger)")(?&t)>(?:(?!</Button\s*>)[\s\S])*?t\('common\.(?:done\.status|action\.close\.label|action\.dismiss\.label)'\)`.
  Hits now: CaptureWindow, DataGripImportDialog, UpdateDialog, AddMemoryDialog (F2).

Header comment: add `# Guards U134-U140: docs/v0.3.17/plans/P672-part4-iter2.md.` after the U133
line; extend the U118-U121 comment with `; U118 flush rule: P672-part4-iter2.md`. No PERMANENT line
added (stays 48). Run the script after each guard; a stale exemption fails the run.

## Docs

`docs/ARCHITECTURE.md`:

- 1436: `(U83-U133)` to `(U83-U140)`. 1471: `Guards U110-U133` to `U110-U140`.
- 1456 dialog footer bullet append: "Dismiss (Cancel, Close, Done, Dismiss) is `dialog`. `#start`
  text inherits `text-kira-sm text-subtle` (bare span); queue counts go there. Destructive confirm:
  `DialogContent destructive`, dismiss `data-dialog-dismiss`. `flush` body never re-adds padding;
  tab panes inside a body carry none."
- 1457 button bullet append: "A shortcut hint on a Button is its `kbd` prop. Form save bars are
  SaveBar (packages/workbench): discard outline, save primary. A select-like trigger ends with
  chevron-down."

`docs/v0.3.17/SPEC.md`: progress line `Part 4 iter2: <result>, plan plans/P672-part4-iter2.md.` under
P672 Part 10 (master plan format), last commit.

## Stream decision

One sequential implementer. Theme pieces (C1) precede host adoption; guards need every fix; F-groups
share files (ConnectionFileDialog in F1/F4/F9, UpdateDialog in F2/F6/F8, DataGripImportDialog in
F2/F4/F9).

## Commits

1. C1 `feat(theme): Button kbd prop and DialogFooter start text style` (Button.vue, DialogFooter.vue).
2. C2 `refactor: Button kbd prop replaces hand Kbd in buttons` (F6, 8 files).
3. C3 `refactor: dialog footer start text and queue counts` (F3, F4, F5).
4. C4 `feat(workbench): SaveBar for form save bars` (F10; new SaveBar, delete AdeWorkflowSaveBar,
   7 hosts).
5. C5 `fix: one emphasised footer button, dismiss as outline` (F1, F2).
6. C6 `fix: dialog bodies: destructive confirm, error Alerts, single padding` (F7, F8, F9; en.json key).
7. C7 `fix: commit-row and confirm-strip button variants` (F11).
8. C8 `fix: Alert and EmptyState text actions as links, toolbar icons` (F12, F13).
9. C9 `chore(lint): guards U134-U140, U118 flush padding` (script only).
10. C10 `docs: ARCHITECTURE dialog and button rules for P672 Part 4 iter2`.
11. C11 `docs: P672 Part 4 iter2 progress` (SPEC.md).

Per commit: `bun run typecheck`, `bun run lint` (includes check-ui-primitives.sh, check-conventions.ts
N17 and N10 600-line cap).

## Test split

No flow test (no IPC change). No unit test (no complex logic). Targeted UI specs once near the end
(`apps/kira-space/tests/ui/`):

- `studio/connection-lifecycle.spec.ts`, `consistency.spec.ts` (ConnectionFileDialog, NewConnection,
  tab padding).
- `ade-agent-settings.spec.ts`, `ade-agent-context.spec.ts`, `ade-v2-automations.spec.ts`,
  `ade-v2-flat-steps.spec.ts`, `repo-file-edit.spec.ts`, `repo-config-file.spec.ts` (SaveBar testids).
- `prompts.spec.ts`, `update-dialog.spec.ts`, `studio/connection-mask-review.spec.ts`,
  `studio/proof-ready.spec.ts`, `mobile-access.spec.ts` (Kbd, queue counts, Alerts).
- `ade-v2-panel.spec.ts` (stage move kbd), `studio/datagrip-import.spec.ts`, `memory-module.spec.ts`,
  `studio/kubernetes-mcp.spec.ts` (`kube-mcp-error` text now inside Alert), `studio/docker-edit.spec.ts`,
  `ade-v2-leases.spec.ts`, `repo-workspace.spec.ts` (worktree remove).

Lessons: Monaco menu items need hover before click; Reka radio arrow presses need delay 40; shared
spec helpers (N8); one lone rerun on a timeout; `bun run build:mobile` before mobile specs. Visual
baselines re-record in PR CI only.

## Risks

- DialogFooter start style inherits into every `#start`: verify buttons and Badges in
  ConnectionFileDialog, RunFileDialog, ApiVariablesDialog `#start` keep `text-kira-md` / own colour.
- Specs asserting `toHaveText` on testids moved into an Alert (`kube-mcp-error`,
  `update-dialog-*-error`): Alert adds an icon, not text; adjust only if a spec fails.
- ContainerEditView Apply at kira-lg sits beside a kira-icon reset TooltipIconButton: height step in
  that row is expected (form commit size).
- Files near the N10 600-line cap (git-ui App.vue, ConnectionFileDialog): icon lines add little; split
  nothing unless N10 fails.

## Out of scope

- Raw kira-icon Buttons that should be TooltipIconButton (AdeReviewRemoved 90, AdeRepoGateDialog 93,
  ApiAccessRulesSection 114/123, ConsoleResultStrip 110, EnvironmentsView 160, FailureBanner 64): iteration 5.
- Hand Kbd in TooltipIconButton default slot (AdeTaskCard 213, AdeBranchRow 422/556; `kbd` prop
  exists) and hand Tooltip + Kbd (RefreshButton 100, RepoFileEditBar 43): iteration 5.
- AdeDetailsDiskBanner empty FieldDescription, DataGrip `h-6.5` hand rows, DynamicValuesDialog fake
  rows: states and rows scope.
- a11y-only items (aria-describedby, redundant aria-label).

## Files touched

Shared (cross-iteration):

- `scripts/check-ui-primitives.sh`
- `docs/ARCHITECTURE.md`
- `docs/v0.3.17/SPEC.md`
- `packages/theme/src/components/ui/button/Button.vue`
- `packages/theme/src/components/ui/dialog/DialogFooter.vue`
- `packages/theme/src/i18n/en.json` (one key rename)

Overlap risk with iteration 5 (menus, popovers, tooltips), additive hunks only:

- `apps/kira-space/frontend/src/studio/views/grid/ColumnsMenu.vue`
- `apps/kira-space/frontend/src/studio/views/documents/ProjectionMenu.vue`
- `packages/workbench/src/memory/MemoryFilters.vue` (PopoverFooter button)
- `packages/git-ui/src/components/RefsPopoverBranches.vue`
- `apps/kira-space/frontend/src/ade/v2/agents/AdeLeasesPane.vue` (DropdownMenuTrigger)
- `apps/kira-space/frontend/src/views/repo/RepoFileEditBar.vue` (Tooltip + Kbd stays theirs)
- `packages/git-ui/src/App.vue`
- `apps/kira-space/frontend/src/studio/api/EnvironmentsView.vue`

Overlap risk with iteration 6 (headings, detail headers, KV, cards):

- `packages/workbench/src/memory/import/ImportJobDetail.vue` (DetailHeader actions)
- `packages/git-ui/src/components/WorktreeList.vue`, `StackList.vue` (SectionHeading actions)
- `apps/kira-space/frontend/src/ade/v2/agents/AdeLeasesPane.vue` (DetailSection, KeyValueList)
- `packages/docker-ui/src/components/ContainerSizeSection.vue`, `EngineDiskSection.vue` (DetailSection)
- `apps/kira-space/frontend/src/ade/v2/review/AdeWorkflowReviewPanel.vue` (Card)

Owned (this iteration only), `apps/kira-space/frontend/src/`:

- `studio/project/ConnectionFileDialog.vue`, `DataGripImportDialog.vue`, `NewConnectionDialog.vue`,
  `SchemaDialog.vue`, `FiltersDialog.vue`, `MaskingReviewPrompt.vue`
- `studio/project/connection/ConnectionEnginePicker.vue`, `ConnectionGeneralTab.vue`,
  `ConnectionAdvancedTab.vue`, `ConnectionPreconnectTab.vue`, `ConnectionMcpTab.vue`,
  `ConnectionPrivacyTab.vue`
- `studio/proof/ProofReadyPrompt.vue`
- `studio/workbench/GenerateDataDialog.vue`, `studio/workbench/apiaccess/ApiAccessRuleDialog.vue`
- `studio/kubernetes/components/ClusterMcpDialog.vue`, `studio/kubernetes/components/LogsView.vue`
- `studio/api/VariableSetView.vue`, `studio/api/ApiVariablesTable.vue`
- `studio/views/httprequest/ResponsePane.vue`, `FormDataTable.vue`, `BinaryBodyPicker.vue`,
  `ResponseHistoryList.vue`
- `studio/views/grpcrequest/ResponsePane.vue`, `SchemaBrowser.vue`
- `studio/views/grid/FilterToolbar.vue`, `studio/views/stream/StreamFilterRow.vue`,
  `studio/views/documents/DocumentFilterRow.vue`, `studio/views/documents/DocumentEditor.vue`,
  `studio/views/console/kibana/FilterEditor.vue`, `studio/views/shared/page/SearchFilterNotice.vue`
- `workbench/GitCredentialDialog.vue`, `workbench/PairingRequestDialog.vue`, `workbench/SettingsDialog.vue`
- `capture/CaptureWindow.vue`
- `ade/v2/stage/AdeStageDialog.vue`, `ade/v2/settings/AdeSettingsPane.vue`,
  `ade/v2/context/AdeContextPane.vue`, `ade/v2/links/AdeLinkStrip.vue`, `ade/v2/panel/AdeRunOutcome.vue`
- `ade/v2/workflows/AdeWorkflowSaveBar.vue` (deleted), `AdeWorkflowGraph.vue`, `AdeWorkflowYaml.vue`,
  `AdeLibraryStepYaml.vue`, `AdeLibraryStepForm.vue`
- `repo/RepoFilePane.vue`, `views/repo/RepoFileView.vue`, `views/repo/ReviewThread.vue`
- `hostview/HostViewFrame.vue`, `hostview/HostViewConversation.vue`

Owned, packages:

- `packages/workbench/src/components/SaveBar.vue` (new), `UpdateDialog.vue`, `QueryState.vue`,
  `CopyLogButton.vue`
- `packages/workbench/src/approvals/ApprovalItemActions.vue`
- `packages/workbench/src/automations/schedule/ScheduleConfirmDialog.vue`
- `packages/workbench/src/memory/AddMemoryDialog.vue`, `import/ImportView.vue`, `import/ImportStatus.vue`,
  `import/ImportFileRow.vue`, `distill/DistillView.vue`, `distill/DistillResult.vue`,
  `distill/DistillRewriteResult.vue`
- `packages/workbench/src/restore/RestoreDialog.vue`
- `packages/git-ui/src/components/ConflictBanner.vue`, `NoRepositoryPanel.vue`, `SearchResults.vue`,
  `UndoButton.vue`, `LoadMoreButton.vue`, `review/ReviewCommitList.vue`, `review/ReviewCommentsPane.vue`
- `packages/docker-ui/src/components/ConnectionState.vue`, `ContainerDetail.vue`, `ContainerEditView.vue`,
  `EditInPlaceSection.vue`, `EditRows.vue`, `LogsView.vue`

## Handoff

Implementer reads this plan, `plans/P672-part10.md` (commit rules) and ARCHITECTURE 1423-1471.
Re-verify each file:line against the tree before editing (iterations 5 and 6 may land first; keep
their hunks). Done when every guard above is in the script with zero offenders, lint and typecheck
are green, targeted specs pass, and the SPEC progress line is committed.
