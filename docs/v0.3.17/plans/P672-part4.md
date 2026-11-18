# P672 Part 4 UI uniformity: buttons and dialogs

Spec row: `../SPEC.md` P672 Part 4. Rules RB.1-RB.3, RD.1-RD.6, migration items M10-M15
(`P672-inventory.md` §2, §3, §3.1 hosts, appendix A.1). Base: `chapter/v0.3.17` at `6026e6cb6`.
Inventory line numbers are at `6527aec4`. Every fact below was re-checked at `6026e6cb6` via
`codegraph_explore` and template scans. Line numbers are hints: implementer greps the named pattern
or `data-testid` before editing.

UI-only phase. Look, layout and button placement change. Behaviour, i18n keys and every
`data-testid` stay. No new i18n key unless named below.

## Rules applied (recap)

- RB.1 size by host. Form commit (dialog footer, save bar, a field's own action) `kira-lg`. Bar,
  row, popover action row `kira` or `kira-icon`/TooltipIconButton. Status bar `status`, mobile
  `touch`. `variant="link"` always `size="inline"`.
- RB.2 outline (`dialog*`) only commits a form or answers a question: footer, save bar,
  confirm/approve strip, banner decision, query Run (`dialog-primary@kira`). Every other in-view
  action is `toolbar@kira` with a leading `CodiconIcon :size="13"`. Text action in prose, Alert or
  EmptyState is `link@inline`.
- RB.3 `ACTION_CLASS` (`apps/kira-space/frontend/src/ade/v2/tones.ts:35`) only on the plan board and
  the task step action (`AdeTaskActionButton`).
- In-view labelled destructive: `toolbar` + `data-tone="error"` (ARCHITECTURE line 1421 keeps
  `dialog-danger` for a labelled destructive commit in a footer or confirm strip).
- RD.1 size: `sm` 400 confirm, single field, prompt; `md` 480 form up to 8 fields; `lg` 600
  sections, side by side, list picker; `xl` 720 two panes, diff, preview; `2xl` 900 + `fixedHeight`
  editor or multi-tab config; `full` review windows only.
- RD.2 footer `[#start] … [dismiss][alternate][primary]`. Exactly one emphasised button
  (`dialog-primary`, `dialog-danger`, `dialog-approve`), rightmost. A second action is `dialog`.
  `dialog-deny`/`dialog-approve` pair only for approvals.
- RD.3 header is `DialogTitle` plus optional `DialogDescription`. Badges and subject text go into
  the description; mode tabs open the body; extra buttons go to footer `#start`.
- RD.4 intro text is the header `DialogDescription`, not a body paragraph.
- RD.5 `DialogBody` takes no class except `flush` and `min-h-*`.
- RD.6 failure: error `Alert` as last body child; field validation `FieldError` under its field.

## Baselines

`scripts/check-ui-primitives.sh` TEMPORARY block (lines 152-250) holds zero lines naming
`P672 Part 4` (`grep -c 'P672 Part 4'` = 0). Part 2's guards U110-U117 own no Part 4 offender. So
this part removes no baseline line. It adds four guards (C9) whose offenders this part fixes first,
so they land with zero offenders and no TEMPORARY entry. Every commit before C9 leaves the script
as is and it passes.

## New theme piece

`packages/theme/src/components/ui/dialog/DialogHeader.vue` stacks title and description.

Why: `dialogHeaderClass` is `flex flex-row items-center gap-1.5 px-3 py-2`. A `DialogDescription`
in the header sits beside the title. That is why seven git dialogs put the description in
`DialogBody`, and why `packages/workbench/src/restore/RestoreDialog.vue` wraps both in a hand
`flex-col` div. RD.3/RD.4 need the description in the header, so the primitive must stack it.

Change: wrap `<slot />` in `<div class="flex min-w-0 flex-1 flex-col gap-0.5"><slot /></div>`; keep
the X button after it (`ml-auto` stays harmless). `dialogHeaderClass` unchanged, so
`packages/workbench/src/prompt/TextPromptDialog.vue` (uses the class directly) is unaffected.
`packages/theme/src/components/ui/command/CommandDialog.vue` renders its header `sr-only`
(verified), so stacking is invisible there.

Second piece: `packages/workbench/src/components/SettingsShell.vue` gains a `footerStart` slot
rendered into `UiDialogFooter #start`. Why: `apps/kira-space/frontend/src/workbench/SettingsDialog.vue`
puts its save status in a hand `div.mr-auto` inside the footer slot; `#start` is the footer's own
place for it (RD.2).

No new button variant or size. No new composable or store: every change is template markup on
existing shadcn-vue primitives (`Button`, `Dialog*`, `Alert`, `CodiconIcon`) and Tailwind classes.
Frontend rules check: no scoped `<style>` added; no hand-rolled primitive; Pinia/TanStack/VueUse
untouched (no state or fetch logic moves).

## Shapes (copy exactly)

In-view action (RB.2):

```vue
<Button variant="toolbar" size="kira" data-testid="…" @click="…">
  <CodiconIcon name="refresh" :size="13" />{{ label }}
</Button>
```

In-view labelled destructive: same plus `data-tone="error"`, icon `trash` or the action's own.

Footer (RD.2):

```vue
<DialogFooter>
  <template #start><Button variant="dialog" size="kira-lg">…</Button></template>
  <Button variant="dialog" size="kira-lg" data-dialog-dismiss @click="cancel">{{ cancel }}</Button>
  <Button variant="dialog" size="kira-lg">{{ alternate }}</Button>
  <Button variant="dialog-primary" size="kira-lg">{{ commit }}</Button>
</DialogFooter>
```

Header (RD.3/RD.4):

```vue
<DialogHeader>
  <DialogTitle>{{ title }}</DialogTitle>
  <DialogDescription>{{ intro }} <Badge …/></DialogDescription>
</DialogHeader>
```

Icon choice: reuse the icon the same action already has elsewhere (grep the testid's sibling
TooltipIconButton); else `play` start/resume/run, `debug-stop` stop, `refresh` retry/again,
`copy` copy, `add` new/add, `eye` review/view, `eye-closed` hide, `go-to-file` open/focus, `history`
history, `discard` reset/restore, `clear-all` clear, `arrow-right` forward/move on, `fold` collapse,
`unfold` load all, `check` seen, `git-merge` merge, `repo-push` push, `cloud-download` import.

## Stream decision

One sequential Sonnet implementer. Not split:

- C1 (theme `DialogHeader`) changes every dialog's header layout; C2-C5 depend on it.
- Button and dialog edits hit the same files (`AdeAgentDialog`, `AdeRunDialog`,
  `AdeWorkflowReviewPanel`, `ApprovalsDialog`, git dialogs, `AdeRunSandboxBadge`).
- C9 guard script and C10 ARCHITECTURE are shared by every group.

## Commits

Order is binding. Run `bun run lint` (includes `check-ui-primitives.sh`) and `bun run typecheck`
before each commit. Every commit passes hooks without `--no-verify`. Any test or fix commit found
later goes before C10 and C11 (rebase it in place; the docs commits stay last).

### C1 `feat(theme): stack dialog header title and description`

- `packages/theme/src/components/ui/dialog/DialogHeader.vue`: wrap default slot as above.
- `packages/workbench/src/components/SettingsShell.vue`: add `<template #start><slot
  name="footerStart" /></template>` inside `UiDialogFooter`; type the slot in `defineSlots`.
- `apps/kira-space/frontend/src/workbench/SettingsDialog.vue`: move the `div.mr-auto.min-w-0`
  content (`FieldError` `settings-save-error`, span `settings-footer-status`) into
  `<template #footerStart>`; drop the wrapper div.

### C2 `refactor(ui): dialog header extras and intro text (M14)`

Header extras out of `DialogHeader` (RD.3):

- `apps/kira-space/frontend/src/studio/project/NewConnectionDialog.vue`: move "Change engine"
  toolbar button (`chevron-left`, shown when `step === 'details'`) to `DialogFooter #start` as
  `variant="dialog" size="kira-lg"` with the same icon and testid; delete the `span.grow` spacer.
- `packages/git-ui/src/components/dialogs/ResetDialog.vue`: subject span leaves `DialogTitle`;
  becomes `<DialogDescription>` (same testid on the span inside).
- `packages/workbench/src/approvals/ApprovalsDialog.vue`: Badges `approvals-count`,
  `approvals-position` into a `DialogDescription` (`class="flex items-center gap-1.5"`).
- `packages/workbench/src/automations/RunFileDialog.vue`,
  `packages/workbench/src/automations/run/RunScriptDialog.vue`: `<SmartBadge v-if="isSmart" />`
  into `<DialogDescription v-if="isSmart">`.
- `packages/workbench/src/automations/schedule/ScheduleConfirmDialog.vue`: SmartBadge and span
  `schedule-confirm-due` into one `DialogDescription` (`flex items-center gap-1.5`).
- `packages/workbench/src/restore/RestoreDialog.vue`: drop hand `div.flex.min-w-0.flex-col.gap-0.5`;
  title and description become direct header children.
- `apps/kira-space/frontend/src/ade/v2/dialog/AdeAgentDialog.vue`: drop the
  `span.text-kira-lg.font-medium` inside `DialogTitle`; move `data-testid="ade-dialog-title"` onto
  `DialogTitle` (title primitive already sets lg/500; check `ade-v2-dialogs.spec.ts` text reads).

`DialogDescription` from body to header (U121):

- `packages/git-ui/src/components/dialogs/CherryPickDialog.vue` (~94),
  `ForcePushDialog.vue` (~103), `PostCheckoutPullDialog.vue`, `PullDialog.vue`,
  `RevertDialog.vue` (~76): move the element unchanged.
- `CheckoutDialog.vue`: the four headline descriptions (inProgressOperation, worktreeConflict,
  blockedByUntracked, trackedBlocker, one `v-if` chain) move to the header as the same chain; the
  file lists stay in the body.
- `ResetDialog.vue`: the three-way chain (~120/124/138) moves to the header after the subject
  description, merged into one `DialogDescription` (subject, then chain text). The `noBranch`
  `FieldDescription` (~116) stays under its field.

Intro text from first body child to header `DialogDescription` (RD.4; keep testids):

- `apps/kira-space/frontend/src/ade/v2/port/AdePortBackupsDialog.vue` (FieldDescription
  backupList.hint)
- `apps/kira-space/frontend/src/ade/v2/run/AdeRepoGateDialog.vue` (repoGate.body)
- `apps/kira-space/frontend/src/studio/api/EditRawRequestDialog.vue` (`edit-raw-hint`)
- `apps/kira-space/frontend/src/studio/project/FiltersDialog.vue` (intro.body)
- `apps/kira-space/frontend/src/studio/project/MaskingReviewPrompt.vue` (p `masking-review-body`)
- `apps/kira-space/frontend/src/studio/project/SchemaDialog.vue` (intro.body)
- `packages/git-ui/src/components/dialogs/BranchDialog.vue`, `RenameRefDialog.vue`,
  `TagDialog.vue` (CatalogText description)
- `packages/workbench/src/automations/schedule/ScheduleTargetDialog.vue`
  (automations.target.hint)
- `apps/kira-space/frontend/src/workbench/GitCredentialDialog.vue` (p `git-credential-repo`)

Stay in body (content, not intro): `ProofReadyPrompt` "more" paragraph (status),
`ConfirmDialogView` message, `PairingRequestDialog` slot.

### C3 `refactor(ui): one emphasised footer button, rightmost (M13)`

- `apps/kira-space/frontend/src/ade/v2/details/AdeDetailsDiskBanner.vue` (~126):
  `ade-details-disk-discard-all` `dialog-danger` to `dialog`; stays left of accept-all.
- `apps/kira-space/frontend/src/studio/api/ApiVariablesDialog.vue` (~317): discard
  (`draft.discard`) `dialog-danger` to `dialog`.
- `packages/git-ui/src/components/dialogs/CheckoutDialog.vue` (~135/141): move the `canDiscard`
  button before the stash button;
  `:variant="canStashAndCarry ? 'dialog' : 'dialog-danger'"`. Rightmost is stash (primary) when
  shown, else discard (danger).
- `packages/workbench/src/automations/RunFileDialog.vue` (~425): `run-file-cancel` `dialog-danger`
  to `dialog`; add `data-dialog-dismiss`.
- `packages/git-ui/src/components/dialogs/ResetDialog.vue` (~217): stash-first branch
  `:variant="stashFirst ? 'dialog-primary' : 'dialog-danger'"` (label already switches to
  "Discard and reset").
- `packages/workbench/src/approvals/ApprovalsDialog.vue` (~90): deny `size="kira"` to `kira-lg`.
- `apps/kira-space/frontend/src/ade/v2/dialog/AdeAgentDialog.vue` (~318/328): delete-anyway
  `dialog-danger` to `dialog`; send drops `ACTION_CLASS`, becomes
  `:variant="view.overridden ? 'dialog-danger' : 'dialog-primary'"`. Body override button (~164,
  inside the Alert) `dialog-danger@kira-lg` to `dialog@kira` (Alert action). Remove the
  `ACTION_CLASS` import.
- `packages/git-ui/src/components/dialogs/WorktreeDialog.vue` (~341): body `detach-here`
  `dialog-primary@kira-lg` inside an Alert to `dialog@kira`.
- `apps/kira-space/frontend/src/ade/v2/run/AdeRunDialog.vue` (~294): primary drops
  `ACTION_CLASS.agent`; plain `dialog-primary`. Remove import.
- `apps/kira-space/frontend/src/ade/v2/panel/AdeRunSandboxBadge.vue`: profile dialog gains a
  footer: `#start` copy button (moved from body, `dialog@kira-lg`, same testid), then
  `<DialogClose as-child><Button variant="dialog" size="kira-lg" data-dialog-dismiss>` Close
  (`common.action.close.label`). Popover's view-profile button: `dialog@kira` to `toolbar@kira` +
  `eye` icon. Size change in C5.

Justified exception (no change): `ForcePushDialog` plain confirm `dialog-danger` inside a
Disclosure behind an ack checkbox; it is the dialog's confirm, footer holds only dismiss.

### C4 `refactor(ui): drop DialogBody overrides (M15)`

- `apps/kira-space/frontend/src/studio/workbench/apiaccess/ApiAccessRuleDialog.vue` (~88),
  `apps/kira-space/frontend/src/studio/kubernetes/components/ClusterMcpDialog.vue` (~79): drop
  `class="flex flex-col gap-3"` (default already).
- `apps/kira-space/frontend/src/studio/api/DynamicValuesDialog.vue` (~89): drop `gap-1.5`; if the
  list needs a tight gap, wrap the list in `div.flex.flex-col.gap-1.5`.
- `apps/kira-space/frontend/src/studio/project/MaskingReviewPrompt.vue` (~49): drop
  `flex flex-col gap-2`.
- `packages/workbench/src/approvals/ApprovalsDialog.vue` (~80): `class="gap-0 py-0"` to `flush`;
  wrap children in `div.flex.flex-col.px-3` (keep scroll on `DialogBody`; spec
  `approvals-one-at-a-time` asserts dialog-body scroll).
- `packages/workbench/src/components/SettingsShell.vue` (~215): `flush class="flex-row"` to
  `flush`; wrap nav + section in `div.flex.min-h-0.flex-1.flex-row`.

### C5 `refactor(ui): dialog sizes by content (M12)`

- `apps/kira-space/frontend/src/studio/api/DynamicValuesDialog.vue` md to lg (list picker).
- `apps/kira-space/frontend/src/studio/project/MaskingReviewPrompt.vue`,
  `apps/kira-space/frontend/src/studio/proof/ProofReadyPrompt.vue`,
  `packages/git-ui/src/components/dialogs/PullDialog.vue`, `PostCheckoutPullDialog.vue`,
  `RenameRefDialog.vue`, `packages/git-ui/src/components/WorktreeList.vue` (confirm dialog): md
  to sm (confirm or single field).
- `apps/kira-space/frontend/src/ade/v2/port/AdePortBackupsDialog.vue`: `2xl` + `fixedHeight` to
  `lg`, drop `fixedHeight` (list picker).
- `apps/kira-space/frontend/src/ade/v2/panel/AdeRunSandboxBadge.vue` profile: lg to xl (code
  preview).

Keep (justified, verified content): `ImportCurlDialog` lg (code input), `SettingsShell` xl (two
panes), `RunScriptDialog`/`ScheduleConfirmDialog` prompt previews, `CheckoutDialog` md.

### C6 `refactor(ade): ACTION_CLASS only on the task step action (M11)`

After C3 `AdeAgentDialog` and `AdeRunDialog` are clean. Remaining:

- `apps/kira-space/frontend/src/ade/v2/review/AdeWorkflowReviewPanel.vue`: send-agent (~312)
  `dialog-approve` + ACTION_CLASS to `dialog-primary@kira`; finish results (~383) to `dialog@kira`.
- `apps/kira-space/frontend/src/ade/v2/sessions/AdeSessionActions.vue` (~43): takeover and stop
  to `toolbar@kira` with icons (`terminal`, `debug-stop`; stop `data-tone="error"`); drop
  ACTION_CLASS and `class="shrink-0 rounded-kira-sm px-2"` (keep `shrink-0`).
- `apps/kira-space/frontend/src/ade/v2/needs/AdeNeedsRow.vue` (~24): delete `buttonClass` computed;
  button `dialog@kira`, keep `w-23`.
- `apps/kira-space/frontend/src/ade/v2/needs/AdeAllSessionRow.vue` (~72): same.
- `apps/kira-space/frontend/src/ade/v2/panel/AdeSetupProgress.vue` (~55): retry `toolbar@kira`
  + `refresh`.
- `apps/kira-space/frontend/src/ade/v2/panel/AdeBranchPanel.vue`: delete `actionClass` helper
  (~166); header actions and merge (~357/368) `toolbar@kira` + icons; force-push (~515)
  `toolbar@kira` + `data-tone="error"` + `repo-force-push`.
- `apps/kira-space/frontend/src/ade/v2/panel/AdeStepBlock.vue` (~283/342/372/385): approve stays
  outline (`dialog-approve@kira`, answers a question) without ACTION_CLASS; log toggle, resume,
  review, takeover, retry to `toolbar@kira` + icons.
- `apps/kira-space/frontend/src/ade/v2/panel/AdeRunOutcome.vue` (~197): acts to `toolbar@kira` +
  icons; abortRebase gets `data-tone="error"`.
- `apps/kira-space/frontend/src/ade/v2/run/AdeTaskActionButton.vue` (~46): keeps ACTION_CLASS
  (sole keeper); labelled size `kira-lg` to `kira` (host is a panel header and step action row).

Check: `grep -rn ACTION_CLASS apps/kira-space/frontend/src` hits only `tones.ts` and
`AdeTaskActionButton.vue`.

### C7 `refactor(ade): in-view actions are toolbar buttons (M10)`

All `apps/kira-space/frontend/src/ade/v2/`. Outline to `toolbar@kira` + leading 13px icon unless
noted:

- `AdeAddPopover.vue`: trigger icon 12 to 13; nw-add `kira-lg` to `kira` (popover row).
- `repos/AdeReposPage.vue`: import, file buttons (aria-pressed keeps working on toolbar),
  add-folder.
- `automation/AdeAutomationsBlock.vue`: open, stop.
- `review/AdeReviewAgentPanel.vue`: forward, focus, start.
- `review/AdeReviewSubmitDialog.vue`: in-body open button.
- `review/AdeWorkflowReviewPanel.vue`: stop, resume.
- `notes/AdeNoteEntry.vue`: move-on (cancel/save stay outline: inline edit commit).
- `plan/AdePlanView.vue`: load-all, collapse (~448/460 `toolbar@kira-lg` to `kira`); AlertAction
  (~416) `link@kira` to `link@inline`.
- `plan/AdeHistoryBar.vue`: hide, current.
- `updates/AdeTaskUpdates.vue`: seen.
- `workflows/AdeWorkflowsTab.vue`: import, new (import-go stays outline).
- `workflows/AdeWorkflowEditor.vue`: copy YAML.
- `workflows/AdeLibraryStepEditor.vue`: copy (`ade-step-copy`) `toolbar@kira` + `copy`; delete
  (`ade-step-delete`) `toolbar@kira` + `trash` + `data-tone="error"` (in-view destructive; confirm
  dialog already guards it).
- `workflows/AdeStepsTab.vue`: new. `workflows/AdeWorkflowGraph.vue`: add step.
- `workflows/inspector/AdeStepStatus.vue` (~61): `link@kira` to `link@inline`.
- `workflows/inspector/AdeStepSandbox.vue`: preset `kira` to `kira-lg` (field action).
- `workflows/statuses/AdeTaskStatuses.vue`: add `kira` to `kira-lg` (form list action).
- `sessions/AdeSessionStrip.vue` (~41), `sessions/AdeSessionsMenu.vue` (~32): `toolbar@kira-lg` to
  `toolbar@kira`; strip is `py-1` content-sized, so check its height in `ade-v2-sessions` and
  the `ade-sessions-menu` baseline; strip height may drop by 4px (intended, matches bar rule).
- `needs/AdeNeedsPage.vue`: show-all.
- `panel/AdeTaskPanel.vue`: review.
- `panel/AdeMainBranches.vue`: add. `panel/AdeMainTab.vue`: review.
- `panel/AdeWorkflowBlock.vue`: edit.
- `panel/AdeArchivedPanel.vue`: restore.
- `panel/AdePanelForwardBar.vue`: send `dialog-approve` to `dialog-primary` (commits the bar).
- `details/AdeTaskChain.vue`: add-empty (cancel/save stay outline).
- `AdeLinkTarget.vue`: add to `dialog@kira`.

Keep outline in ADE: `AdeSettingsPane`, `AdeContextPane`, `AdeWorkflowSaveBar` save bars;
`AdeNotesEditor` add, `AdeLeasesPane` add (form commit); `AdeDetailsDiskBanner` row accept/discard
and review (banner decision); `AdeView` front door; `AdeTuiPane` banner; form field actions
(`AdeContextFields`, `AdeStepResults`, `AdeStepInspector` save-library, `AdeSettingsSecrets`,
`AdePort*` retry/env copy/restore, `AdeRepoGateDialog` open repos) keep `dialog@kira-lg`.

### C8 `refactor(ui): in-view actions are toolbar buttons in Studio and workbench (M10)`

- `apps/kira-space/frontend/src/studio/api/EnvironmentsView.vue`: new (`new-environment`)
  `toolbar@kira` + `add` (spec `studio/control-sizing` checks only right edge inside dialog; still
  holds).
- `apps/kira-space/frontend/src/studio/views/httprequest/CookiesPane.vue`: clear all, `clear-all`.
- `apps/kira-space/frontend/src/studio/views/httprequest/HttpRequestView.vue`: send
  (`http-send`) `dialog-approve` to `dialog-primary@kira` (query Run shape).
- `apps/kira-space/frontend/src/studio/views/shared/keyvalue/KeyValueAddPopover.vue`,
  `KeyValueEditPopover.vue`: action row `kira-lg` to `kira`.
- `apps/kira-space/frontend/src/studio/project/connection/ConnectionMaskReview.vue`: allow-all
  `dialog-primary` to `dialog@kira` (one emphasised per strip; row allow/undo stay outline).
- `packages/workbench/src/llmPrompts/PromptDetail.vue`: reset, `discard`.
- `packages/workbench/src/components/OpLogToolbar.vue`: clear, `clear-all`.
- `packages/workbench/src/components/OpLogPanel.vue`: clear-filters inside EmptyState to
  `link@inline`.
- `packages/workbench/src/components/ShowMoreButton.vue`: `link@kira` to `link@inline`.
- `packages/workbench/src/automations/runs/RunsSection.vue`: show history, stop.
- `packages/workbench/src/automations/runs/RunOutcomeBlock.vue`: copy, resume, again.
- `packages/workbench/src/automations/runs/ScriptRunView.vue`: stop to toolbar; review, decline,
  continue stay outline at `kira` (they answer the run's question).

Keep outline (verified hosts): footers; save bars (`RepoFilePane`, `SettingsDialog`); inline commit
rows (`CommentComposer`, `ReviewThread`, `CredentialPastePanel`, `StreamComposeMessage`,
`FilterEditor`, `TimePicker`, `DocumentEditor`, `KeyValuePane` save); approvals
(`ApprovalItemActions`, `ProofChangePanel`); query Run (`ConsoleView`, `QueryBar`,
`GrpcRequestView`, `StreamToolbar`, `StaleDataStrip`); Alert/banner actions `dialog@kira`
(`FailureBanner`, `RestoreBanner`, `RunsRestoreBanner`, `CopyAsCurlDialog` hide/reveal, `App.vue`
boot retries, `ReviewView` retries, `ConnectionMcpTables` retry); toasts; capture and config-sync
windows; `TextPromptDialog`; front doors (`GitStart`, `StudioStart`, `ApiStart`,
`AutomationsStart`, `BootFailure`, reconnect EmptyStates); popover confirms (`App.vue` ~1958,
`RefsPopoverBranches` ~256). Form field and list actions keep `dialog@kira-lg` (`RepoConfigForm`,
`ConnectionGeneralTab` browse, `UploadObjectDialog`, `RunFolderTab`, `McpServerTools`,
`ParamsEditor`, `ToolsField`, `ScheduleFields`, `MaskRuleList`, `MaskKeySection`, `AgentPane` and
`NotifyPromptsField` test, `CachePane`, `CliToolsPane`, `ConfigSyncPane`, `MobileAccessPane`,
`StudioMcp*`, `KubeMcp*`, `ApiAccessRules`, `ApiAgentAccess`, `ApiConfigProblems`, `ApiSecrets`,
`RunSecretsSection`, `SemanticModel*`, `AgentMcp*`, `CheckHook*`, `RunScriptDialog` edit/reset,
`StackDialog` clear, `PairScreen`).

Exceptions (no change, named): `IssueAttachments` `dialog@inline` attachment tiles; `MaskKindMenu`
select-like trigger; `DataView.vue` ~345 TooltipIconButton `dialog-primary` grid commit;
`CredentialsUpdateDialog` has no footer (panel owns its commit row).

Before C9, scan for remaining in-view outline: implementer re-runs the outline scan (every
`variant="dialog*"` outside `DialogFooter`, grep `-B` for host) and classifies any site missed
here by RB.1/RB.2. Any new site follows the same rule, same commit.

### C9 `chore(lint): guard dialog body, header, ACTION_CLASS and link size (U118-U121)`

`scripts/check-ui-primitives.sh`, after the U117 row:

```
U118@@v@@@@DialogBody class other than min-h-* (only flush and min-h-*, RD.5)@@<DialogBody\b(?&t)\s:?class="(?!min-h-[\w.\[\]-]+")
U119@@vt@@ade/v2/tones.ts|AdeTaskActionButton.vue@@ACTION_CLASS off the task step action (RB.3)@@\bACTION_CLASS\b
U120@@v@@@@link Button not size inline (RB.1)@@<Button\b(?=(?&t)\svariant="link")(?!(?&t)\ssize="inline")
U121@@v@@@@DialogDescription inside DialogBody (intro text is the header description, RD.4)@@(?s)<DialogBody\b(?&t)(?<!/)>(?:(?!</DialogBody>).)*?<DialogDescription\b
```

Header comment, after line 15: `# Guards U118-U121: docs/v0.3.17/plans/P672-part4.md.`

Dry run at `6026e6cb6` (same `-Pzl`, same DIRS): U118 hits exactly ApiAccessRuleDialog,
DynamicValuesDialog, ClusterMcpDialog, MaskingReviewPrompt, SettingsShell, ApprovalsDialog
(RunFileDialog `min-h-80` passes); U119 hits the 11 files in C3/C6 plus `tones.ts`; U120 hits
AdePlanView, AdeStepStatus, ShowMoreButton; U121 hits the seven git dialogs in C2. After C2-C8 all
four are zero; both U119 exempt names are used, so the stale built-in check passes. A header-only
guard (non-title child in `DialogHeader`) was dry-run and rejected: `(?&t)`-based nesting cannot
tell a span inside `DialogTitle` from a sibling without false hits; C2 review covers it.

### C10 `docs: architecture for P672 Part 4`

`docs/ARCHITECTURE.md`:

- line 1420: replace "ADE task actions use ACTION_CLASS tone outlines" with "the ADE task step
  action (AdeTaskActionButton) alone uses ACTION_CLASS tone outlines".
- line 1421: "A labelled destructive commit (footer, confirm strip) is dialog-danger; an in-view,
  icon or toolbar one is toolbar plus data-tone="error"."
- line 1423: append "DialogHeader stacks title and DialogDescription; extras (badges, subject)
  live in the description, buttons in footer #start."
- line 1435: `U83-U117` to `U83-U121`.
- line 1470: "Guards U110-U121 … U118-U121 landed with zero offenders and have no TEMPORARY line."

### C11 `docs: P672 Part 4 done`

`docs/v0.3.17/SPEC.md` Part 4 row: Status Done; Result names commits C1-C10 (and any test/fix
commits), guards U118-U121, files touched, specs run, visual baselines for CI re-record. Last
commit of the phase.

## Test split

UI-only phase (no IPC, no Go, no flow test change). Frontend half: existing UI specs, updated only
where a selector or asserted look changed. No new unit test (none of this is complex logic). No new
spec: guards U118-U121 are the regression check.

Spec updates expected:

- `ade-v2-dialogs.spec.ts`: `ade-dialog-title` now on `DialogTitle`; send-button style reads.
- `approvals.spec.ts`, `approvals-one-at-a-time.spec.ts`: badges inside description; body scroll
  still on `[data-slot="dialog-body"]`.
- `git-choice-cards.spec.ts`, any git dialog spec reading description text inside
  `[data-slot="dialog-body"]`: retarget to `[data-slot="dialog-header"]` or role.
- `studio/tree.spec.ts`, `studio/sql-schema.spec.ts` use `[data-slot="dialog-footer"] button` with
  text Save/Cancel: still valid; re-check after NewConnection `#start` button (filter by text, not
  index).
- Any spec asserting `rgb` border on a now-toolbar button (`consistency.spec.ts`,
  `ade-v2-row-actions.spec.ts`, `studio/control-sizing.spec.ts`, `studio/api-ui-consistency.spec.ts`):
  update expected look, never drop the assertion.

Targeted run near phase end (one pass, from `apps/kira-space`), specs touching changed testids:

- ADE: ade-v2-task-branches, ade-v2-shell-bar, ade-v2-orchestrator, ade-v2-notes, ade-v2-shell,
  ade-v2-panel, ade-v2-plan, ade-v2-row-actions, ade-v2-multi-worktree, ade-v2-add,
  ade-v2-add-link, ade-v2-op-kinds, ade-v2-automations, ade-v2-needs, ade-v2-panel-tabs,
  ade-v2-base-rebase, ade-v2-resume, ade-v2-review-step, ade-v2-handover, ade-v2-archived,
  ade-v2-launch, ade-v2-review-open, ade-v2-dialogs, ade-v2-task-row, ade-v2-sessions,
  ade-v2-task-launch, ade-v2-run, ade-v2-archive, ade-v2-branch-request, ade-v2-panel-forward,
  ade-v2-sandbox, ade-v2-return-limit, ade-v2-task-menu, ade-v2-review-uncommitted,
  ade-v2-card-icons, ade-v2-task-status, ade-v2-review, ade-v2-flat-steps, ade-v2-repo-required,
  ade-v2-stage-move, ade-v2-norepo, ade-v2-stage-resume, ade-v2-links-strip, ade-v2-sessions-menu,
  ade-v2-task-panel, ade-v2-details-list, ade-v2-db-audit, ade-v2-session-status,
  ade-v2-workflows, ade-v2-workflow-steps, ade-v2-workflow-layout, ade-v2-background-ops,
  ade-v2-details-files, ade-repos, ade-agent-missing, ade-ids, ade-tracked-updates,
  ade-readable-names, repo-config-file, status-agents.
- Shared: module-state, ui-leftovers, copy-wording, copy-coverage, consistency, operations,
  settings-llm-prompts, dialog-surface, update-dialog, approvals, approvals-one-at-a-time,
  git-choice-cards, git-credential-relay.
- Studio: studio/control-sizing, studio/api-ui-consistency, studio/http-history,
  studio/http-pipes, studio/http-request-body, studio/http-raw, studio/http-curl,
  studio/http-dynamic-values, studio/http-variables, studio/http-request, studio/http-timeline,
  studio/redis-edit, studio/dbmcp-script-long, studio/terminal-claude-missing, studio/tree,
  studio/sql-schema.

Sandbox lessons for spec edits: Playwright click on a menu item inside Monaco needs
`locator.hover()` first (commit `6026e6cb6`); reka radio arrow presses need
`keyboard.press('ArrowDown', { delay: 40 })` (ResetDialog, CherryPick, Revert radio specs). A
timeout under load gets one lone rerun before any fix.

Visual baselines changed, re-recorded in PR CI (not locally): approvals-dialog, ade-sessions-menu,
sandbox-popover, branch-panel-details, plan-branch-*, plan-day-bars, review-step-panel,
workflow-graph*, review-uncommitted-only, ade-repos-page, repo-gate, git-reset-dialog,
git-stash-dialog, git-repo-settings-dialog, docker-restore-dialog, runs-restore-dialog,
new-connection-dialog, schema-dialog, settings-*, operations-detail, operations-list, console,
proof-*. Result lists the ones CI actually re-recorded.

## Risks

- Header stacking (C1) leaves a title-only header unchanged. A header with a description grows by
  one line. Visual baselines catch it; intended.
- Moving long git descriptions (CheckoutDialog chain, ResetDialog chain) into the header makes the
  header taller in `md`/`sm`; body scroll stays below it. Check ResetDialog at `sm` breakpoints in
  the targeted spec screenshots.
- Toolbar buttons lose the outline border; in dense rows (AdeNeedsRow keeps outline) the hit
  target is unchanged (`h-control`). AdeSessionStrip `kira-lg` to `kira` lowers the strip by 4px
  (content-sized).
- `dialog-approve` to `dialog-primary` on `http-send` and forward-bar send changes colour only;
  specs reading `--kira-ok` border must update.
- U120 regex reads a `Button` tag only; a `link` rendered via `buttonVariants({ variant: 'link' })`
  in script is not scanned (none exist today).
- `AdeTaskActionButton` labelled size `kira-lg` to `kira` shortens panel header actions; panel
  header is a bar (RB.1), so intended.

## Out of scope

- Tab sizes (SecondaryTabs `kira-lg` in AdeLibraryStepEditor bar), popover padding (Part 5 U111
  lines), rows/cards/status tones (Parts 5-8), TEMPORARY block removal (Part 9).
- Copy wording of button labels beyond variant choice; i18n keys unchanged.
- Mobile `touch` buttons (verified unchanged); status bar `status` buttons.
- Plan board tones and `AdeTaskCard` (D1, board visual exempt).
