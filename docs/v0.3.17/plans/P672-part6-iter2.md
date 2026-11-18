# P672 Part 6 iter2: headings, headers, key-value, cards, panels, empty containers

Iteration of `P672 Part 10` (master `plans/P672-part10.md`). Planned against `b33dd339a` (plus
the Part 5 iter2 plan commit `e87689d19`, docs only). UI only: no Go, no bridge, no new unit tests.
First pass: `plans/P672-part6.md`.

## Rules applied

ARCHITECTURE "UI uniformity rules (P672)":

- Heading roles (RH): PanelHeader titles a panel bar. SectionHeading titles a section in a panel,
  list or popover. FieldSet + FieldLegend groups a form. CardTitle titles a card, through
  DetailSection only. DetailHeader titles a view or detail pane. "No other heading markup."
- Key-value: KeyValueList, default density in panes, compact in sidebars and tooltips.
- Cards (RC): inset is a small text block, section a titled group (DetailSection), list a scroll
  list of rows. Classes only place the card. Code text is CodeBlock. A row is never a Card.
- State primitives: list or section empty is InlineState; pane loading is EmptyState `fill loading`.
- Type roles: case and tracking come only from SectionHeading, PanelHeader and DetailSection,
  "whose action slots reset them". font-data marks data only.

Deliberate exceptions kept: git ViewToolbar detail head, Studio tab view head (ViewToolbar), and
AdeTaskCard per D1. None of them is re-opened. Two notes on the guard work:

- F21 turns a Studio kube tab head into ViewToolbar, so it now follows that exception.
- AdeTaskCard gets an explicit U124 exempt, because the extended regex now sees its bound class.

## Current state (fresh scan, rerunnable)

Scan scripts live in the scratchpad (`p6i2/p6lib.py`, `slotted.py`). Every count below covers the six
guard DIRS minus `components/ui/`, and every scan reads whole files with tags spanning lines.

- `bash scripts/check-ui-primitives.sh` is green. It has 48 PERMANENT lines, and its highest guard
  is U133.
- `<Card` appears 31 times in 29 files. The only CardHeader and CardTitle caller is DetailSection.
- Card class evasions, found with `grep -rlzP '<Card\b(?&t)\s:class='`, are in 5 files:
  - AdeTaskCard (D1), CodeBlock (exempt) and MemoryStart;
  - AdeStepBlock and ScriptProgress, both through a variable or condition the regex cannot read.
- Card class tokens border, opacity, bg, rounded and shadow appear in 3 files: AdeTaskCard,
  MemoryStart and ScriptProgress.
- A hand box without a border (rounded-kira(-sm) + bg-elevated|bg-field + padding, not inline-flex)
  appears in 5 files: CliToolsPane:103, AdeReposPage:431, AdeBranchPanel:510,550 and
  AdeRunOutcome:112. PickerTrigger, VarText and the AdeLinkStrip `more` pill are inline-flex chips
  or controls.
- A hand bar (h-bar|h-tabbar + items-center + px-) appears in 3 files outside primitives:
  ResourceTabPanel:46, ProofChangePanel:70 and AdeShell:111. AdeShell is the ADE primary tab strip.
- DetailHeader: 16 tags. KeyValueList: 22 tags in 21 files. SectionHeading: 67 tags, 10 of them
  with default-slot content. Guards U112, U113 and U124 have 0 offenders.
- SectionHeading slot content renders uppercase and tracking-wider today, because the slot has no
  reset. That hits these hosts:
  - FiltersDialog:223,256: link Buttons All and None;
  - WorktreeList:124: a toolbar Button;
  - StackList:93: the Restack Button;
  - ValidationSection:29: data chip Badges;
  - AdeMainBranches:70: ProjectPicker and a Button.
  - PreviewCommandPanel, GlobalStashList and PhoneApprovals hold an icon or count only, so they
    show no visible change.
- SectionHeading labels that hold data values get uppercased: StackList:93 (branch name),
  SchemaBrowser:227 (gRPC service name) and AdeSettingsResult:94 (file name, hand label).
- Hand title-bar titles (`text-kira-md font-medium` in a file rendering TitleBarBase) appear in
  6 files: OpenedFilesWindow:12, ConfigSyncReviewWindow:82, HostViewFrame:29, ProofHeader:33,
  AdeReviewHeader:62 and WorkbenchTitleBar:46.
- Hand section captions (`text-kira-sm text-subtle|text-muted-foreground` div or span over a list)
  appear 14 times in 8 files. The findings below list them.
- codegraph_explore calls covered these areas:
  - DetailHeader, DetailSection, SectionHeading, KeyValueList, PanelHeader, Card and its parts;
  - EmptyState, CodeBlock, AdeStepBlock, ScriptProgress, MemoryStart;
  - OpLogDetail, TimelinePane, rowVariants, AdeRow;
  - AdeInspectorSection, AdeReviewHeader, ProofHeader, TitleBar consumers;
  - AdeDetailsTab, AdeTaskChain, AdeLinkStrip, the list-Card precedents and the SectionHeading
    slot consumers.

## Findings

Paths are relative to `apps/kira-space/frontend/src/` unless they start with `packages/`.

### Theme pieces (shared)

F1 SectionHeading default slot leaks case and tracking into actions.
`packages/theme/src/components/SectionHeading.vue`.

- Change: wrap the bare `<slot />` in both branches in
  `<span class="ml-auto flex items-center gap-1 normal-case tracking-normal"><slot /></span>`.
  Render the wrapper only when `$slots.default` exists. This mirrors PanelHeader `#actions`,
  FieldLegend `#actions` and DetailSection CardAction. ARCHITECTURE type roles already claims the
  reset.
- Host cleanup in the same commit:
  - FiltersDialog:224,257: drop the `span.flex.gap-1` wrapper.
  - ValidationSection:30: drop the `span.flex.items-center.gap-1.5` wrapper.
  - WorktreeList:125: drop `class="ml-auto"`.

F2 SectionHeading `data` prop. It is a boolean, like DetailHeader `data`. The label span gains
`font-data normal-case tracking-normal`. Hosts:

- `packages/git-ui/src/components/StackList.vue:93` gets `data`, because the base branch name is
  the label.
- `studio/views/grpcrequest/SchemaBrowser.vue:227` gets `data`, for the service name.
- `ade/v2/settings/AdeSettingsResult.vue:94`: the hand file-name div becomes
  `<SectionHeading flush data :label="f.name" />`.

F3 New `packages/workbench/src/components/TitleBarTitle.vue`:

- markup: `<span class="min-w-0 truncate text-kira-md font-medium"><slot /></span>`; attrs fall
  through, so each host keeps its testid;
- replaces the hand span in OpenedFilesWindow:12, ConfigSyncReviewWindow:82, HostViewFrame:29,
  ProofHeader:33 and AdeReviewHeader:62;
- `packages/workbench/src/components/WorkbenchTitleBar.vue:46` uses `<TitleBarTitle class="shrink-0">`.
  The app title never shrinks; with truncate plus shrink-0 it does not truncate either.

### Cards

F4 `ade/v2/panel/AdeStepBlock.vue:91-95,222`: `:class="boxClass"` on `Card variant="list"` evades
U124.

- `border-info` is dead, because the list variant draws no border.
- `text-muted-foreground` mutes the whole card for a skipped step. The STATE_TONE Badge already
  shows "Skipped" and "Now".
- Change: delete `boxClass` and the `:class` binding.

F5 `packages/theme/src/components/ScriptProgress.vue:45`: `:class="state === 'failed' &&
'border-error'"` is dead (no border) and a bound-class evasion. The failure already shows through
the icon and FieldError. Change: delete the binding.

F6 `packages/workbench/src/memory/MemoryStart.vue:130`: `:class="{ 'opacity-50': version.historical }"`
uses Card opacity as state, and it duplicates the "Historical" warn Badge at :138. Change: delete it.

F7 `ade/v2/panel/AdeRunOutcome.vue:112`: hand card `flex flex-col gap-1.5 rounded-kira-sm bg-elevated
p-2`.

- Change: `<Card class="..."` with the inset variant (default). The content stays as it is.
- Reason: inset is a text and status block with gap-1.5. Its only control is a Button, and the
  rule names only Input, Select and Checkbox as the triggers for a section card.
- Keep `data-testid` and `:data-run-id` on Card. No class beyond placement.

F8 `workbench/settings/CliToolsPane.vue:70-120`: an inset Card with a hand title (`span.font-medium`
tool.label plus Badge) and copy controls.

- Change: `DetailSection :title="tool.label"` with the Badge in `#actions`, moving the testid
  `cli-tool-${provider}` to it.
- Hand code box at :102-104 (`code ... rounded-kira-sm bg-field px-2 py-1 font-data`) becomes
  `CodeBlock class="min-w-0 flex-1"`. Rule: code text is CodeBlock.

F9 `studio/project/connection/PrivacyOverview.vue:18-21`: an inset Card with a hand title
(`div.text-fg.font-medium`).

- Change: `DetailSection :title="..."` and keep `data-testid="connection-privacy-overview"`.
- The sibling sections in ConnectionPrivacyTab already use DetailSection.

F10 `ade/v2/panel/AdeBranchPanel.vue:503-562`: two lists ("Merged into" and "Deployed to") draw each
row as its own hand box, `flex min-h-row items-center gap-2 rounded-kira-sm bg-elevated px-1.5`.

- Rule: a row is never a Card, and a list of rows is one list Card. Precedents: ScriptsMcpSection
  and ApiAccessRulesSection.
- Change, per list:
  - `<SectionHeading flush :label>` replaces the hand caption at :504 and :544. Its colour also
    changes from muted to subtle, the SectionHeading tone.
  - Wrap the rows in `<Card v-if="rows.length" variant="list">`. Each row keeps its markup minus
    `rounded-kira-sm bg-elevated`.
  - InlineState stays outside the Card.
- File is 578 lines and must stay at or under 600 (N10). Check `wc -l`. If it goes over, extract
  the two lists into `AdeBranchTargets.vue`.

F11 `ade/v2/panel/AdeArchivedPanel.vue:232-262`: the hand caption "Branches" (`span pb-0.5 text-kira-sm
text-subtle`) has rows that each carry `bg-elevated px-1.5 py-1`.

- Change: `SectionHeading flush`, then `Card variant="list"` around the rows.
- Rows drop `bg-elevated` and keep `flex flex-col gap-1 px-1.5 py-1` with their testid.

F12 `ade/v2/repos/AdeReposPage.vue:427-450`: folder rows each have `rounded-kira-sm bg-field px-2 py-1`,
and the empty `p.m-0.text-muted-foreground` at :427 is hand-made.

- Rows go in one `Card variant="list"`. Each row gets `:class="rowVariants()"` (menu layout:
  h-control, px-1.5, gap-1.5) plus `gap-2`, dropping its own box classes.
- The empty `p` becomes `InlineState kind="empty"`.
- Keep the testid `ade-repos-folder` and `:data-path`.

### Section headings

F13 Hand section captions become `SectionHeading flush :label`:

- `ade/v2/port/AdePortResult.vue:17,23,29,38` (four captions; the file already opens with a
  SectionHeading flush);
- `ade/v2/settings/AdeSettingsResult.vue:98,119` (env and kira captions);
- `ade/v2/panel/AdeMainTab.vue:171`, for the children caption. The empty span at :173-180 becomes
  `InlineState kind="empty"` and keeps `data-testid="ade-main-children-empty"`.
- `ade/v2/sessions/AdeSessionsList.vue:69`, for "Stopped". This becomes
  `<div v-if="inline" class="px-1.5 pt-2"><SectionHeading flush :label /></div>`, following the
  AdeShelfSection wrapper precedent.
- `studio/views/shared/celleditor/CellEditorView.vue:513-519,548-554`, for the "Decoded" and
  "Timestamp" strips.
  - Each becomes `<SectionHeading :label class="shrink-0 bg-elevated">` with the icon in `#leading`.
    The `class` is placement and fill only.
  - Part 5 iter2 also edits this file at :327-350, in another hunk.
- `studio/views/grpcrequest/ResponsePane.vue:438,450`, for the header and trailer captions
  (see F17).

F14 Hand section heads with actions become a SectionHeading flush with its actions in the default
slot. This needs F1.

- `ade/v2/details/AdeTaskChain.vue:78-112`: the chain title plus the Timeline and DB audit Toggles
  and the add TooltipIconButton.
- `ade/v2/links/AdeLinkStrip.vue:167-179`: the links title plus AdeLinkAdd. Keep the testid
  `ade-links-title` on the SectionHeading; no spec reads it.
- `packages/workbench/src/components/OpLogDetail.vue:81-90,100-108`: the "Command" and "Error"
  captions, each with an inline copy TooltipIconButton.

F15 `ade/v2/details/AdeTaskChain.vue:150-165`: the hand empty row (`text-kira-sm text-subtle` plus an
add Button) becomes `InlineState kind="empty" data-testid="ade-notes-empty"` holding the text span
and the Button.

F16 `studio/api/ApiAgentAccess.vue:41-55`: a per-environment group head (dot, `span text-kira-md
font-medium` name, Badge, Edit Button) is a hand section head inside a list.

- Change: `<SectionHeading flush :label="env.name">`.
  - The `colorMarkClass('dot')` span goes in `#leading`.
  - The Badge and the Button go in the default slot.
- Button variant and size stay as they are. Iteration 4 owns them, so this is an overlap.
- Precedent: AdeAllSessions groups use a SectionHeading with a leading dot.

### Key-value

F17 `studio/views/grpcrequest/ResponsePane.vue:437-460` (`grpc-response-metadata`): hand key-value
rows (`flex gap-1.5`, key `min-w-40 font-data`) under hand captions, a hand empty line, and a dead
`metadata-group` class (no style block).

- Per group, replace the wrapper with `<section class="flex flex-col gap-1">` (drop
  `metadata-group`), then:
  - `SectionHeading flush :label`;
  - `<KeyValueList v-if="list.length">` with `dt.font-data` and `dd.font-data.wrap-anywhere` (the
    font-data and wrap classes go on the cells, not on the KeyValueList);
  - `InlineState kind="empty"` for the "no header" and "no trailer" text.
- Default density (pane).

F18 `packages/workbench/src/memory/MemoryStart.vue`:

- :102: `KeyValueList density="compact"` sits in the main detail pane (`memory-detail`, p-3). Drop
  `density`; the default applies.
- :92-95: the "Reason" label plus value (`span.font-medium` inside a muted `p`) is a hand key-value
  pair. Move it into that KeyValueList as its first row:
  `<dt>Reason</dt><dd class="whitespace-pre-wrap">`.

F19 `ade/v2/request/DetailsEditBody.vue:33-36`: same shape as F18. The "Reason" label plus value
becomes a one-row `KeyValueList` and keeps `data-testid="ade-details-edit-reason"` on the `dd`.

F20 `packages/workbench/src/memory/import/ImportJobDetail.vue:256-275`: group and doc-path inline
pairs (`flex items-center gap-1.5 text-kira-sm text-muted-foreground` + label span + Identifier)
become one `KeyValueList` with two `v-if` rows.

- Keep the testids `import-file-group` and `import-file-doc` on the `dd`.
- The `import-file-orphaned` FieldDescription between them moves after the list.
- The ImportJobDetail pane uses DetailHeader, so default density.

### Panel and view heads

F21 `studio/kubernetes/components/ResourceTabPanel.vue:46`: the hand bar `flex h-bar shrink-0
items-center gap-2 px-2` becomes `<ViewToolbar>`. That gives gap-1.5; the siblings ResourceBrowser
and LogsView use ViewToolbar. The children stay.

F22 `studio/proof/ProofChangePanel.vue:69-73`: the hand bar `flex h-tabbar shrink-0 items-center
bg-chrome px-3 text-kira-sm font-medium text-muted-foreground` becomes
`<PanelHeader class="bg-chrome">{{ title }}</PanelHeader>`. `--kira-tabbar-h` equals
`--kira-bar-h`, so the height is unchanged.

### Empty and loading containers

F23 `configsync/ConfigSyncReviewWindow.vue:91-93`: the hand pane loading div (`m-auto
text-muted-foreground` plus text) becomes `<EmptyState fill loading :title="...loading key">`, per
the rule that pane loading is EmptyState.

### Labels over code blocks and controls (dialog hosts; overlap with iteration 4)

F24 `ade/v2/run/AdeRunDialog.vue:222-233`: the handover and task-notes captions
(`span.text-muted-foreground` over a CodeBlock) become `Field` + `FieldLabel` + CodeBlock. This is
the same file's suffix convention at :274-277. Keep the testids on Field.

F25 `ade/v2/dialog/AdeAgentDialog.vue:182-196` (`ade-dialog-base`): `span.text-muted-foreground` +
BranchPicker becomes `<Field orientation="responsive">`, with FieldLabel `for` the picker trigger id
and the picker. BranchPicker drops `size="kira"`, because a form control takes the primitive default
size. Keep the testid on Field. The Part 3 iter2 handoff named this lead.

Not changed: the AdeRunDialog `ade-run-smart` row :236-239 (SmartBadge + script name over the
CodeBlock). It is a value line, not a caption.

## Exemptions re-checked

- AdeTaskCard: D1 board visuals. Its bound `:class` plus pastel and rail classes get the U124 exempt
  name, with requirement "D1 board card tone, rail and selected fill". The existing U37 PERMANENT
  line stays.
- CodeBlock U124 exempt: still needed. The inset card carries code type by design.
- U36: `PickerTrigger.vue` exempt and the VarText PERMANENT line still hold, both for the bordered
  alternative.
- The U13 extension needs exempts `ViewToolbar.vue|PanelHeader.vue|AdeShell.vue`. The first two
  are the primitives. AdeShell is the primary ADE tab strip: it wraps below `@5xl` and holds
  TabsList, which is the U5 exempt.
- Checked and not findings:
  - AdeStepNode: a fixed canvas graph node.
  - Compact KeyValueList in AdeBranchTip (tooltip), IssueSidebar (sidebar), OpLogDetail (op log
    row body), ApprovalDetails and TooltipDetails.
  - CommitMeta compact: inside the git detail-head exception.
  - Inset text Cards: AdeReviewRemoved, AdeWorkflowReviewPanel comments, AdeNoteEntry, AdeProofEntry,
    ImportJobDetail dropped, TimelinePane hops, RunPreview, AddMemoryDialog, PairScreen and the
    finish-note and preview cards.
  - ParamsEditor and ScheduleFields: section Cards with CardContent.
  - EnvironmentsView and WorkingDetailPane heads: exceptions.
  - The AdeLinkStrip `more` button: an inline pill matching AdeLinkPill.
  - AdeHistoryBar: a bar.

## Guards

Extend three regexes in place, under the same ids, and add one new guard. Iterations 4 and 5 add
guards in parallel, so the new one carries a provisional id, `G6a`. The orchestrator assigns the
final U id at landing. Every guard lands with zero offenders, after the fixes it holds.

- U124 (Card) changes:
  - Exempt field: `CodeBlock.vue|AdeTaskCard.vue`.
  - Token list: append `|border|opacity-|bg-|rounded|shadow`.
  - New second alternative: `|<Card\b(?&t)\s:class=`, which fails any bound class.
  - Message: "Card class sets padding, gap, type, fill, border, opacity or row layout, or binds
    :class (variant inset, section or list; state shows through a Badge; code is CodeBlock; a row
    is never a Card)".
  - Dry run on base: 5 files. After F4-F6: AdeTaskCard (exempt) and CodeBlock (exempt), so 0.
- U36 (hand box): append a second alternative for a borderless box:
  `|\bclass="(?![^"]*${TOK}inline-flex(?![\w-]))(?=[^"]*${TOK}rounded-kira(?:-sm)?(?![\w-]))(?=[^"]*${TOK}bg-(?:elevated|field)(?![\w-]))(?=[^"]*${TOK}(?:p|px|py)-)`.
  - Dry run on base: 4 files (CliToolsPane, AdeReposPage, AdeBranchPanel, AdeRunOutcome). The
    inline-flex exclusion leaves out the AdeLinkStrip `more` pill, VarText and PickerTrigger. After
    F7, F8, F10 and F12: 0.
  - The bordered alternative keeps its PickerTrigger exempt and VarText PERMANENT line.
- U13 (hand bar): append the alternative
  `|\bclass="(?=[^"]*${TOK}h-(?:bar|tabbar)(?![\w-]))(?=[^"]*${TOK}items-center(?![\w-]))(?=[^"]*${TOK}px-)`
  and set the exempt field to `ViewToolbar.vue|PanelHeader.vue|AdeShell.vue`.
  - Dry run on base: ResourceTabPanel, ProofChangePanel, AdeShell, ViewToolbar and PanelHeader.
    After F21 and F22: exempt only, so 0.
  - Check that the existing border-b alternative still reports none.
- G6a, new: a hand title-bar title in a TitleBarBase host.
  - Line: `G6a@@v@@@@hand title bar title (use TitleBarTitle)@@(?s)\A(?=.*<TitleBarBase\b).*\bclass="(?=[^"]*${TOK}text-kira-md(?![\w-]))(?=[^"]*${TOK}font-medium(?![\w-]))`.
  - Dry run on base: 6 files. After F3: 0.
- PERMANENT stays 48.
- Header line: `# Guards G6a (final id at landing), U13, U36, U124 (extended): docs/v0.3.17/plans/P672-part6-iter2.md.`
- Hand section captions get no guard. Their class signature (`text-kira-sm text-subtle`) equals
  rowMeta and other meta text, so a regex would need exemptions with no requirement behind them.

## Docs updates

ARCHITECTURE "UI uniformity rules (P672)":

- Heading roles: after "SectionHeading (section in a panel, list or popover, subtle)", add:
  "actions in its default slot, right-aligned with case reset; `data` for a data-valued label;
  `#leading` icon or dot". Also add: "A window title bar title is TitleBarTitle."
- Card variants: after "classes only place the card in its parent", add: "no bound :class; state
  shows through a Badge, never Card border, fill or opacity. Rows in a list sit in one list Card,
  never a box per row." Change the U124 sentence to name fill, border, opacity and bound class.
- Guard range lines: ~1436 "U83-U133" and ~1471 "Guards U110-U133" extend to the final id. Update
  the PERMANENT count line ~1438 only if it changes (expected: unchanged at 48).

SPEC `P672 Part 10` section, last commit: `Part 6 iter2: <result>, plan plans/P672-part6-iter2.md.`

## Stream decision

One sequential implementer. F1-F3 are shared theme and workbench pieces, and most host commits
depend on F1. The guard script is shared. No streams.

## Commits

Each commit passes the pre-commit hook (lint with the guard script and N8-N12, typecheck, build).

- C1 `feat(theme): SectionHeading action slot reset and data label, TitleBarTitle` (F1 theme half,
  F2 prop, F3 component, no hosts).
- C2 `refactor(ui): section heading hosts` (F1 host cleanup, F2 hosts, F13, F14, F15, F16).
- C3 `refactor(ui): cards place only, list rows in one list card` (F4-F12).
- C4 `refactor(ui): key-value lists for reason, import and gRPC metadata` (F17-F20).
- C5 `refactor(ui): title bar titles, kube tab head, proof change bar, config sync loading`
  (F3 hosts, F21, F22, F23).
- C6 `refactor(ade): run and agent dialog captions as fields` (F24, F25).
- C7 `chore(lint): extend U13, U36, U124; add title bar title guard` (header line). Delete any
  PERMANENT line or exempt that went stale in the commit that made it stale.
- C8 `test(ui): ...` only if a spec breaks (see Test split).
- C9 `docs: P672 Part 6 iter2 rules in ARCHITECTURE`.
- C10 `docs: P672 Part 10 progress, Part 6 iter2` (SPEC, last).

## Test split

No new unit tests. Run targeted UI specs once, near the end. The testid greps against
`apps/kira-space/tests` found:

- ui: ade-v2-panel, ade-v2-run, ade-v2-sandbox, ade-v2-base-rebase, ade-v2-repo-script,
  ade-v2-launch, ade-v2-flat-steps, ade-v2-archived, ade-v2-handover, ade-v2-dialogs,
  ade-v2-details-list, ade-v2-notes, ade-v2-review, ade-agent-port, ade-agent-settings, ade-repos,
  host-view, issue-view, opened-files, memory-module, memory-import, settings-cli-tools,
  consistency;
- ui/studio: proof-window, kubernetes-tabs, connection-privacy, grpc-request, cell-editor,
  api-agent-access, data-view (op log).

Notes:

- No mobile host is touched. PhoneApprovals gets only the SectionHeading slot wrapper, so run
  `bun run build:mobile` and `tests/mobile/approvals.spec.ts` once.
- Specs reading case: none assert uppercase or `toHaveClass` on the touched elements (grep done).
- Visual baselines re-record in PR CI for: ADE branch panel, archived panel, run outcome, step
  block, CLI tools pane, privacy overview, gRPC metadata, cell editor strips, proof change bar,
  title bars and memory detail.
- A timeout under load gets one lone rerun first.

## Risks

- F1 changes every SectionHeading that has slot content.
  - WorktreeList relied on `ml-auto`, and the wrapper now carries it.
  - AdeMainBranches places two controls in the slot. The wrapper's gap-1 replaces the outer gap-1.5
    between them. Check visually in a spec screenshot.
- F10: AdeBranchPanel at 578 lines (N10). F17: gRPC ResponsePane at 554 lines.
- F13 CellEditorView: SectionHeading `h-control-sm` is taller than the current 20px strip, so the
  editor body loses a few px. That is accepted.
- F24 and F25 overlap iteration 4 (dialogs and buttons). F16 overlaps the iteration 4 Button
  variant. If iteration 4 lands first, rebase hunk by hunk.

## Out of scope

- Row title sizes: `text-kira-md font-medium` in AdeStepNode:185, AdeBranchRow:334,459 and
  AdeDayBand:131 (Part 9 type roles, D1 board).
- Popover internals: AdeRunSandboxBadge popover captions and the network pair (:86-128) belong to
  Part 5 (RP flush pieces). Handed off.
- Button variants and sizes in touched hosts.

## Handoff (leads, re-verify)

- Part 5 or final scan: AdeRunSandboxBadge popover.
  - List captions and the denials caption are `span text-kira-sm text-muted-foreground`; they
    should be SectionHeading flush.
  - The network label and value pair should be KeyValueList compact.
  - The "none" span should be InlineState empty.
- Part 7: the AdeRunOutcome state Badge and the AdeStepBlock state Badge carry the only state signal
  once F4 lands; check that STATE_TONE covers `now` as info.
- Part 9: row title type in AdeStepNode, AdeBranchRow and AdeDayBand. Title-bar meta spans
  (`host-view-ref`, `proof-steps`) use differing sizes and tones.

## Files touched

Shared (mark in the ownership table):

- `scripts/check-ui-primitives.sh`
- `docs/ARCHITECTURE.md`
- `docs/v0.3.17/SPEC.md`
- `packages/theme/src/components/SectionHeading.vue` (theme export, F1 and F2)
- `packages/theme/src/components/ScriptProgress.vue` (theme, F5)
- `packages/workbench/src/components/TitleBarTitle.vue` (new, F3)

Overlap with parallel iterations:

- Iteration 4 (buttons and dialogs):
  - `apps/kira-space/frontend/src/ade/v2/run/AdeRunDialog.vue`;
  - `apps/kira-space/frontend/src/ade/v2/dialog/AdeAgentDialog.vue`;
  - `apps/kira-space/frontend/src/studio/api/ApiAgentAccess.vue`;
  - `apps/kira-space/frontend/src/studio/project/FiltersDialog.vue`.
- Iteration 5 (menus and popovers): `apps/kira-space/frontend/src/studio/views/shared/celleditor/CellEditorView.vue`
  (Part 5 edits :327-350, this plan :513-554), plus the links popovers inside
  `apps/kira-space/frontend/src/ade/v2/links/AdeLinkStrip.vue`. This plan touches only :167-179.

Own:

- `apps/kira-space/frontend/src/ade/v2/panel/AdeStepBlock.vue`
- `apps/kira-space/frontend/src/ade/v2/panel/AdeRunOutcome.vue`
- `apps/kira-space/frontend/src/ade/v2/panel/AdeBranchPanel.vue` (maybe new `AdeBranchTargets.vue`)
- `apps/kira-space/frontend/src/ade/v2/panel/AdeArchivedPanel.vue`
- `apps/kira-space/frontend/src/ade/v2/panel/AdeMainTab.vue`
- `apps/kira-space/frontend/src/ade/v2/repos/AdeReposPage.vue`
- `apps/kira-space/frontend/src/ade/v2/port/AdePortResult.vue`
- `apps/kira-space/frontend/src/ade/v2/settings/AdeSettingsResult.vue`
- `apps/kira-space/frontend/src/ade/v2/sessions/AdeSessionsList.vue`
- `apps/kira-space/frontend/src/ade/v2/details/AdeTaskChain.vue`
- `apps/kira-space/frontend/src/ade/v2/request/DetailsEditBody.vue`
- `apps/kira-space/frontend/src/ade/v2/review/AdeReviewHeader.vue`
- `apps/kira-space/frontend/src/workbench/settings/CliToolsPane.vue`
- `apps/kira-space/frontend/src/studio/project/connection/PrivacyOverview.vue`
- `apps/kira-space/frontend/src/studio/views/grpcrequest/ResponsePane.vue`
- `apps/kira-space/frontend/src/studio/views/grpcrequest/SchemaBrowser.vue`
- `apps/kira-space/frontend/src/studio/views/definition/ValidationSection.vue`
- `apps/kira-space/frontend/src/studio/kubernetes/components/ResourceTabPanel.vue`
- `apps/kira-space/frontend/src/studio/proof/ProofChangePanel.vue`
- `apps/kira-space/frontend/src/studio/proof/ProofHeader.vue`
- `apps/kira-space/frontend/src/openedfiles/OpenedFilesWindow.vue`
- `apps/kira-space/frontend/src/configsync/ConfigSyncReviewWindow.vue`
- `apps/kira-space/frontend/src/hostview/HostViewFrame.vue`
- `packages/workbench/src/components/WorkbenchTitleBar.vue`
- `packages/workbench/src/components/OpLogDetail.vue`
- `packages/workbench/src/memory/MemoryStart.vue`
- `packages/workbench/src/memory/import/ImportJobDetail.vue`
- `packages/git-ui/src/components/WorktreeList.vue`
- `packages/git-ui/src/components/StackList.vue`
- spec files only if C8 needs them.
