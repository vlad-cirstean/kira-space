# P672 Part 2 iter2: theme pieces and guards, second pass

Spec row: `P672 Part 10` (Part 2 iteration). Master plan: `plans/P672-part10.md`. Base commit
`6c098b5f5`. UI only: no Go, no bridge, no new unit tests.

Subject: the six Part 2 pieces (Field responsive, PopoverContent flush, KeyValueList compact,
DetailHeader, DetailSection, TextTooltip) and guards U110-U132: regex misses, missing hosts,
hand-rolled equivalents, single-consumer pieces, exemption reasons.

## Rules applied

- RT.1 (ARCHITECTURE "Plain hint"): a plain-text hint is TextTooltip. Part 5 kept a Tooltip with a
  `v-if` on `<Tooltip>` as a rich triple. That was a scan artifact, not a requirement: a structural
  directive (`v-if`, `v-else-if`, `v-else`, `v-for`, `:key`) moves to `<TextTooltip>` unchanged.
  Re-opened here on that reading. A class on TooltipContent, controlled `:open`, markup in the body,
  or a `:disabled` condition that differs from the text stays rich.
- RH/RC (headings, cards): a titled section card is DetailSection; spacing inside a section is
  gap-2 (RSp "gap-2 inside").
- RH.2: a view's title row is DetailHeader, title lg.
- Key-value data is KeyValueList (default density in panes).
- Checkbox: the primitive owns size, radius, border and fill; classes place it only.
- Hand tooltip around a truncating span showing the same text is TruncatedText (U91 rule).
- Deliberate first-pass exceptions (master plan) stay. None re-opened besides RT.1 above.

## Current state (scans at `6c098b5f5`)

709 non-`components/ui/` `.vue` files in the six guard DIRS. Multi-line scans (`grep -Pzo`, and a
scratchpad script that pairs each `<TooltipContent>` with its `<Tooltip>` open tag).

- DetailHeader: 15 files. DetailSection: 22 tags in 8 files. TextTooltip: 114 tags in 73 files.
  KeyValueList: 20 tags in 19 files, compact in 8. Field responsive: 32 tags in 16 files.
  PopoverContent: 41 tags in 38 files, flush 21. SectionHeading: 67 tags. CardTitle: 3 files.
- Every current guard: 0 offenders (`bash scripts/check-ui-primitives.sh` green). PERMANENT: 49
  lines.

### F1. U123 misses plain-text tooltips (95 sites)

U123 matches only a bare `<Tooltip>` and a bare `<TooltipContent>`. 95 plain-text bodies (text or
one `{{ }}`) sit outside TextTooltip. Classified by attrs:

A. 76 sites: `<Tooltip>` carries only `v-if`/`v-else-if`/`v-else`/`v-for`/`:key`, TooltipContent
has no attrs. Convert. `A/` is `apps/kira-space/frontend/src/`.

- `A/workbench/OperationsPanel.vue` 378, 407
- `A/ade/v2/review/AdeReviewActions.vue` 93; `AdeReviewSync.vue` 93, 109, 132, 147, 161;
  `AdeReviewHeader.vue` 89
- `A/ade/v2/panel/AdeWorkflowBlock.vue` 66; `A/ade/v2/details/AdeTaskChain.vue` 104
- `A/repo/RepoWorktreeRow.vue` 64, 75, 85; `RepoSearchRow.vue` 97; `RepoIdentity.vue` 64;
  `RepoList.vue` 489, 505
- `A/hostview/IssueAttachments.vue` 57
- `A/studio/workbench/GenerateDataDialog.vue` 433
- `A/studio/views/shared/keyvalue/KeyValueRowsTable.vue` 242; `shared/document/DocumentRow.vue` 88;
  `shared/celleditor/CellGenerateMenu.vue` 49; `shared/celleditor/CellEditorView.vue` 417, 438, 484
- `A/studio/views/httprequest/FormDataTable.vue` 154; `HttpRequestView.vue` 335, 343, 351, 415;
  `ResponsePane.vue` 423; `TimelinePane.vue` 265, 297
- `A/studio/views/definition/ColumnsSection.vue` 102; `ConstraintsSection.vue` 96;
  `DefinitionView.vue` 337
- `A/studio/views/grpcrequest/GrpcRequestView.vue` 300, 308, 316; `ResponsePane.vue` 305, 506
- `A/studio/views/grid/ColumnsMenu.vue` 173
- `A/studio/views/stream/StreamToolbar.vue` 242, 264; `StreamRowsTable.vue` 386, 403
- `A/studio/views/console/ExplainResultView.vue` 124; `ConsoleView.vue` 314, 331
- `A/studio/api/VariablesOverviewPanel.vue` 171; `EnvironmentFields.vue` 58;
  `EnvironmentsView.vue` 199; `DynamicValuesDialog.vue` 129
- `A/studio/kubernetes/components/KindBar.vue` 90
- `A/studio/project/DataGripImportDialog.vue` 262, 293; `TreeRow.vue` 223;
  `connection/MaskRuleRow.vue` 54; `connection/ConnectionGeneralTab.vue` 162
- `packages/workbench/src/components/OpLogPanel.vue` 449; `components/TabStrip.vue` 300, 314;
  `automations/ScriptRow.vue` 71, 82; `automations/form/RunScriptTab.vue` 90;
  `automations/runs/RunOutcomeBlock.vue` 107
- `packages/git-ui/src/components/AppToolbar.vue` 379
- `packages/docker-ui/src/components/EndpointChip.vue` 103; `ContainerSizeSection.vue` 32;
  `EngineDiskSection.vue` 64; `EditInPlaceSection.vue` 407; `EditField.vue` 46; `OriginIcon.vue` 76
- `packages/theme/src/components/SecondaryTabs.vue` 97; `SplitButton.vue` 47

B. 6 sites: attrs on TooltipContent. Convert after C1 adds the props.

- `data-testid`: `A/studio/views/grid/DataToolbar.vue` 334 (`toolbar-count-tooltip`),
  `A/studio/views/stream/StreamToolbar.vue` 205 (`stream-count-tooltip`),
  `A/studio/views/shared/document/DocumentToolbar.vue` 119 (`document-count-tooltip`).
- `side="bottom"` plus `v-for` on Tooltip: `ModeSwitcher.vue` 51 (find with
  `grep -rn ModeSwitcher.vue`).
- `v-if` on TooltipContent whose condition is the text being non-empty:
  `packages/workbench/src/memory/import/ImportJobDetail.vue` 100 (folded into F5),
  `packages/git-ui/src/components/RefreshButton.vue` 96. RefreshButton: its `v-else` content at 99
  holds a Kbd and stays rich; convert only if the two branches split cleanly, else leave and say so
  in the commit.

D. 11 sites with `:disabled` on Tooltip.

- Convert (condition equals "text is empty"; TextTooltip renders the bare slot then):
  `A/ade/v2/review/AdeReviewHeader.vue` 74 (`remoteBaseHint === ''`),
  `A/studio/views/stream/StreamFilterRow.vue` 285 (`!timestampError`),
  `DocumentEditor.vue` 58, `A/studio/project/TreeRow.vue` 196, `FiltersDialog.vue` 299 (each also
  `v-if` on TooltipContent with the same condition). Verify each condition before converting.
- Stay rich (condition differs from the text): `BranchRequestBody.vue` 160, `AdeReposPage.vue` 326,
  `ReviewResultOption.vue` 42, `ApprovalItemCard.vue` 158, `AutomationsStart.vue` 44, theme
  `TruncatedText.vue` 35.

X. 2 sites stay rich: `DocumentView.vue` 490 (controlled `:open`), `ScriptRow.vue` 108 (class
`whitespace-pre-wrap`).

Rich bodies (markup) checked separately: 17 sites (Kbd chords, TooltipDetails, VarText, lists,
`AdeBranchTip` surface variant). All need markup. Not findings.

### F2. U116 guards nothing

`MenuItem` (`packages/workbench/src/state/contextMenu.ts`) lost `iconClass` in Part 5; typecheck
already rejects it, and U116's `iconClass:` regex has 0 possible hits. The rule it held (menu icons
13 and muted) still has one open path: a toned CodiconIcon inside the hand entries U122 allows
(`DropdownMenuRadioItem`, `DropdownMenuCheckboxItem`, `DropdownMenuLabel`, `DropdownMenuSubTrigger`).
Current hits for that: 0. Retarget U116 to it.

### F3. U110 misses Checkbox colour restyles

`packages/git-ui/src/components/FileTree.vue:532`:
`:class="{ 'border-warn': reviewAttentionFor(row.node.change.path) === 'changed' }"` restyles the
primitive's border. The file name already carries `nameTone` (`text-warn` for changed,
`packages/git-ui/src/lib/reviewAttention.ts:29`), and the toggle title already says "mark again".
Drop the class. Other Checkbox classes are placement only and stay (`shrink-0`, `mt-0.5`,
`after:inset-0`).

### F4. DetailSection spacing and hand section cards

DetailSection's CardContent has no inner gap, so hosts space children by hand:

- `packages/docker-ui/src/components/EngineOverview.vue` 248, 249, 266, 267 (`mt-2` wrappers);
  check the status section 215-237 for the same.
- `packages/docker-ui/src/components/StatsView.vue` 31, 42 (`mb-2` header div).
- `packages/docker-ui/src/components/ContainerSizeSection.vue` (FieldError `mb-2`).
- `packages/docker-ui/src/components/EngineDiskSection.vue` (Alert, InlineState, div stack).
- `A/ade/v2/agents/AdeLeasesPane.vue` 89 (inner `flex flex-col gap-2` wrapper).

Two titled section cards are hand-built instead of DetailSection:

- `A/studio/project/connection/ConnectionMaskReview.vue` 143-147: `Card variant="section"`,
  CardHeader, CardTitle, `CardContent class="flex flex-col gap-2"`, testid
  `connection-privacy-review` on the Card.
- `A/studio/project/connection/ConnectionPrivacyTab.vue` 54-58: same shape, rules section.

After these, CardHeader and CardTitle have one consumer outside the primitive: DetailSection.

### F5. Hand detail header in ImportJobDetail

`packages/workbench/src/memory/import/ImportJobDetail.vue` 82-165: a `div p-3` row with a
`text-kira-md font-medium` title in TextTooltip (roots), state span, channel/origin/scope Badges,
toolbar Buttons (start, pause, resume, cancel, retry failed, dismiss). It is a view title row: use
DetailHeader. Title goes md to lg (RH.2), matching the other 15 DetailHeader hosts.

Window title bars on TitleBarBase (OpenedFilesWindow, AdeReviewHeader, HostViewFrame,
ConfigSyncReviewWindow, ProofHeader) are uniform with each other. Not a finding.

### F6. Hand key-value columns

- `A/ade/v2/agents/AdeLeasesPane.vue` 90-135: holder and waiters rows, `w-16 shrink-0
  text-muted-foreground` label spans, `text-kira-sm`.
- `A/studio/project/credentialPaste/CredentialPastePanel.vue` 162-195: `w-20` label beside a
  NativeSelect or value, Badge, TooltipIconButton.

Both are label/value pairs: KeyValueList.

### F7. TextTooltip evades U91

U91 bans a Tooltip wrapping a truncating span (use TruncatedText). TextTooltip hides that triple, so
U91 cannot see it. Sites where the tooltip repeats the span's own text (TruncatedText shows it only
when cut off; these show it always):

- `A/ade/v2/plan/AdeTaskCard.vue` 277 (`card.meta`, testid `ade-card-meta`).
- `A/ade/v2/panel/AdeStepBlock.vue` 270 (`block.scope`, `ade-step-scope`), 330 (`rl.note`,
  `ade-run-note`).
- `A/studio/views/shared/celleditor/CellEditorView.vue` 429 (Badge `max-w-56 truncate`, text
  `statusLine`, `cell-editor-status`).

Legit (tooltip text differs from the span): `ImportJobDetail.vue` 84 (roots vs title),
`packages/git-ui/src/components/UncommittedChangesStrip.vue` 170 (`tooltipText` vs count label).

### Clean or justified (no action)

- KeyValueList grid, gap or type overrides: none. `dt`/`dd` outside `dl`: only ApprovalDetails
  children, inside a KeyValueList. Other `grid-cols-[...]` are tables.
- U111 (PopoverContent spacing): no misses. `role="heading"`: none. Bound `:variant` link Buttons:
  none. `ContextMenu*Item`: none.
- Field responsive hosts: ADE and docker EditField; label-first horizontal Fields are Part 3 work
  (Handoff).
- Exemptions re-checked, reasons hold: U125 AdeAutomationChip (only AdeTaskCard, board, D1); U130
  VariableRow and VariableHistoryMenu (masked dots need `tracking-widest`); U132 AdeShelfRow (task
  colour rail, D1), AdeTaskPanel and AdeBranchPanel (swatch glyph `border-2`), AdeActivityIcon and
  AdeStepNode (glyph outlines); U26 OpLogPanel wash, U37 board, U101 counts. Other PERMANENT lines
  carry named reasons.
- `font-mono`: only settings font stacks and terminal. No offenders.
- Single-consumer pieces: none of the six; DetailSection has 8 hosts, DetailHeader 15.

## Theme pieces

- `packages/theme/src/components/TextTooltip.vue`: add `side?: TooltipContentProps['side']` (from
  reka-ui, as the TooltipContent primitive types it) and `contentTestid?: string`, bound to
  TooltipContent as `:side` and `:data-testid`. Default side stays the primitive's.
- `packages/theme/src/components/DetailSection.vue`: CardContent gets
  `:class="flush ? undefined : 'flex flex-col gap-2'"`. A flush section (menu rows, tables) keeps no
  gap. Add a `data-testid` passthrough only if a host needs it on the Card (attrs already fall
  through to the root Card; keep that).

## Stream decision

One sequential implementer. TextTooltip, DetailSection and `scripts/check-ui-primitives.sh` are
shared by every group, and the guard commit must follow every fix.

## Commits

C1 `feat(theme): TextTooltip side and contentTestid, DetailSection content gap`

- TextTooltip and DetailSection as in "Theme pieces".
- Same commit: drop the hand spacing F4 lists in EngineOverview, StatsView, ContainerSizeSection,
  EngineDiskSection and AdeLeasesPane (remove `mt-2`/`mb-2` wrappers between direct children; keep a
  wrapper only when it does more than spacing). Without this the gap doubles.

C2 `refactor(ui): plain tooltips through TextTooltip` (may split per package for size; each split
commit must pass the hook)

- Every A site, every B site except ImportJobDetail (C4), the five D sites.
- Shape: `<Tooltip v-if="c"><TooltipTrigger as-child><X/></TooltipTrigger><TooltipContent>{{ s
  }}</TooltipContent></Tooltip>` becomes `<TextTooltip v-if="c" :text="s"><X/></TextTooltip>`.
  Literal text becomes `:text="t(...)"` exactly as the body had it. Directives on `<Tooltip>` move
  to `<TextTooltip>`. B testids become `content-testid`, `side` stays `side`.
- Drop now-unused `Tooltip`/`TooltipContent`/`TooltipTrigger` imports; add the TextTooltip import.
- `A/workbench/OperationsPanel.vue`: converting 378 and 407 makes the PERMANENT line
  `U91 apps/kira-space/frontend/src/workbench/OperationsPanel.vue` stale (U91 no longer sees a
  TooltipTrigger). Delete that line in this commit. PERMANENT goes 49 to 48. The extended U91 in C6
  must not hit OperationsPanel, since its tooltips show `cwd`/`repoRoot`, not the cell text.
- Any other PERMANENT line or built-in exempt name the script reports stale: delete it in this
  commit, note it in the commit body.
- Watch N10: a file over 600 lines after the change splits; conversions shrink files, so not
  expected.

C3 `refactor(studio): privacy section cards on DetailSection`

- ConnectionMaskReview 143-147: `<DetailSection :title="t('...privacy.review.title')"
  data-testid="connection-privacy-review">`, children unchanged, closing tags to match. Drop
  Card/CardHeader/CardTitle/CardContent imports.
- ConnectionPrivacyTab 54-71: same, title `...privacy.rules.title`.
- Heading level changes from div to `h3` (DetailSection's CardTitle `as="h3"`): matches every other
  section.

C4 `refactor(ui): ImportJobDetail on DetailHeader, hand key-value columns to KeyValueList`

- ImportJobDetail 82-165: `<DetailHeader>` with `#title` the TextTooltip(roots) around the title
  span (drop `text-kira-md font-medium`; DetailHeader styles the title), `#meta` the state span and
  three Badges (channel Badge in `TextTooltip :text="job.sessionId ? t(...) : ''"`, which replaces
  the TooltipContent `v-if`), `#actions` the six Buttons unchanged. Keep every testid. Check
  DetailHeader's own padding against the `p-3` it replaces; the progress block below keeps its own.
- AdeLeasesPane 90-135: one `<KeyValueList>` (default density, pane) with `dt` holder / `dd`
  dropdown or free span plus asked Badge, `dt` waiters / `dd` the `ol` or empty hint. Drop the
  `w-16` spans and `text-kira-sm` (KeyValueList sets type). Keep every testid.
- CredentialPastePanel 162-195: one `<KeyValueList>`; each row's label `dt`, its select or value,
  label, Badge and TooltipIconButton in a `dd class="flex min-w-0 items-center gap-1.5"`. Keep
  testids.

C5 `refactor(ui): truncating spans on TruncatedText, review checkbox without tone`

- F7 sites: outer span keeps layout and type classes and the testid, drops `truncate`, holds
  `<TruncatedText :text="..." />`. CellEditorView: `<Badge class="max-w-56"
  data-testid="cell-editor-status"><TruncatedText :text="statusLine" /></Badge>`. AdeTaskCard is a
  board card: TruncatedText adds one ResizeObserver per card; acceptable at board sizes (render cap
  500), note it in the commit body.
- FileTree 532: delete the `:class` border-warn binding.

C6 `chore(lint): extend U91, U110, U123, retarget U116`

- U123: allow on `<Tooltip>` only `v-if`/`v-else-if`/`v-else`/`v-for`/`:key` attrs; on
  `<TooltipContent>` only `v-if`, `data-testid`, `side`. Sketch:
  `(?s)<Tooltip(?:\s+(?:v-if|v-else-if|v-for|:key)="[^"]*"|\s+v-else)*\s*>\s*<TooltipTrigger
  as-child>(?:(?!</TooltipTrigger>|<Tooltip\b).)*</TooltipTrigger>\s*<TooltipContent(?:\s+(?:v-if|data-testid|side)="[^"]*")*\s*>\s*(?:\{\{(?:(?!\}\}).)*\}\}|[^<{]+)\s*</TooltipContent>\s*</Tooltip>`.
  A rich site (`:disabled`, `:open`, class, markup) still passes.
- U110: add a third branch, a Checkbox class (static or bound) with a colour, fill or shape token:
  `<Checkbox\b(?&t)\s:?class="[^"]*${TOK}(?:bg-|border-|rounded|text-)`. Placement classes still
  pass.
- U116: new message "toned icon in a hand menu entry (menu icons stay muted)", regex: a CodiconIcon
  whose class holds `text-(?:ok|warn|error|info|tone-)` inside a `DropdownMenuRadioItem`,
  `DropdownMenuCheckboxItem`, `DropdownMenuLabel` or `DropdownMenuSubTrigger` block (tempered
  `(?:(?!</DropdownMenu(?:RadioItem|CheckboxItem|Label|SubTrigger)>).)*?`). Kinds `v`.
- U91: add a branch for TextTooltip whose `:text` equals the truncating child's sole `{{ }}`
  content, with a named capture and backreference, for example
  `<TextTooltip\b(?&t)\s:text="(?<u91>[^"]+)"(?&t)>\s*<(?:span|div|button|Badge)\b(?&t)\s:?class="[^"]*${TOK}truncate(?![\w-])(?&t)>\s*\{\{\s*\k<u91>\s*\}\}`.
  Check that a named group inside the GUARDS line does not collide with `PRE`'s DEFINE groups.
- Every guard lands with 0 offenders. Dry-run each regex against the base commit before the fixes
  (`git stash` or `git worktree`): it must hit the sites this plan lists and nothing else. Record
  the dry-run counts in the commit body.
- Header comment: `# Guards U91, U110, U116, U123 (extended): docs/v0.3.17/plans/P672-part2-iter2.md.`

C7 `test(ui): ...` only if a spec needs it (see Test split).

C8 `docs: architecture for P672 Part 2 iter2`

- "UI uniformity rules (P672)": plain hint line: TextTooltip takes `side` and `contentTestid`; a
  plain-text Tooltip triple with only structural directives fails U123. Card line: CardHeader and
  CardTitle only through DetailSection; DetailSection content stacks with gap-2 (flush: none).
  Checkbox: classes place only (U110).
- PERMANENT count line: 48 (or the true count after C2).
- Guard range line stays U83-U132 (no new id).

C9 `docs: P672 Part 10 progress, Part 2 iter2`

- Append to SPEC section "P672 Part 10": `Part 2 iter2: 95 plain tooltips to TextTooltip (87
  converted, rest stay rich by rule), DetailSection gap and two section cards, ImportJobDetail on
  DetailHeader, two KeyValueLists, four TruncatedText sites, U91/U110/U116/U123 extended, PERMANENT
  48. Plan plans/P672-part2-iter2.md.` (Adjust numbers to what landed.) Part 2 row and Result stay
  unchanged.

## Test split

UI only. No flow tests (no backend change). No new unit tests.

- Run specs touching changed testids or surfaces, found by grep in `apps/kira-space/tests/ui`:
  `count-tooltip`, `import-job`, `import-start`, `connection-privacy`, `docker-engine-`,
  `docker-stats`, `docker-size`, `ade-lease-`, `paste-credentials-`, `review-folder-check`,
  `ade-card-meta`, `ade-step-scope`, `ade-run-note`, `cell-editor-status`, `op-`, and every spec
  that hovers a tooltip on a converted surface (`grep -rln "tooltip" apps/kira-space/tests/ui`).
  Known hits: `ade-v2-leases.spec.ts`, `memory-import.spec.ts`, `studio/connection-mask-review`,
  `studio/connection-privacy`, `studio/connection-paste-credentials`, `studio/docker-module`,
  `studio/cell-editor`, `studio/data-view`, `operations.spec.ts`, `ade-v2-plan.spec.ts`,
  `ade-v2-run.spec.ts`, `ade-v2-review.spec.ts`.
- A spec reading a TooltipContent testid keeps working through `contentTestid`.
- Lint (`bun run lint`, includes the guard script and N8-N12), typecheck, build per commit.
- Visual baselines re-record in PR CI (header title size in ImportJobDetail, gap in docker cards,
  KeyValueList rows).

## Risks

- TextTooltip renders the bare slot when text is empty. A converted site whose text can be empty
  while the original still showed an empty tooltip changes behaviour; that is the intended fix.
- `v-else` chains: moving `v-else-if`/`v-else` from `<Tooltip>` to `<TextTooltip>` keeps the chain
  only if siblings stay adjacent. Check each chain in OperationsPanel, RepoWorktreeRow,
  CellEditorView, HttpRequestView, GrpcRequestView.
- DetailSection gap applies to every non-flush host: check the 8 hosts render no double spacing
  after C1 (EngineOverview, StatsView, ContainerSizeSection, EngineDiskSection, AdeLeasesPane and
  the rest found by `grep -rln DetailSection`).
- U123 broadened regex may hit a rich site with an attr order this plan missed; the dry run decides.
- U91 backreference branch: PCRE named group numbering inside `grep -Pz`; the dry run decides.

## Out of scope

- New guard ids, new theme pieces.
- Rich tooltips (markup, controlled, styled, differing `:disabled`).
- Card section uses without a title (ParamsEditor, ScheduleFields, CommentComposer, AdeTaskCard).
- Items in Handoff.

## Handoff (leads for later iterations; each planner re-verifies)

- Part 3: `AdeTaskStatuses` RadioGroupItem restyled `size-4` swatch instead of SwatchRadio.
  Label-first horizontal Fields in `AdeWorkflowGraph.vue:297` and `RepoConfigForm.vue:137`
  (`AdeHistoryBar.vue:23` is a bar).
- Part 5: `AdeAddPopover` is flush but hosts a form with inner `p-3`. `PreviewCommandPanel`
  SecondaryTabs `m-2`. `RunningOpsStatusItem` hand rows `rounded-kira-sm px-2 py-1`. `TimePicker`
  default popover holds a row list.
- Part 6: `AdeStepBlock` Card `:class="boxClass"` holds `border-info` (U124 evasion through a
  variable). `MemoryStart` Card `opacity-50` for historical versions. `OpLogDetail` hand section
  labels.
- Part 7: `KeyValuePane` `iconClass` fallback `text-info` vs `text-muted-foreground` in DataView,
  StreamView, DocumentView.
- Part 8: `TimestampPane` zone `segmented` beside an Input (value choice).
- Part 9: row titles at `text-kira-lg font-medium` in `AdeWorkflowsTab.vue:167` and
  `AdeStepsTab.vue:129`; "✕" text glyphs; hand indent steps via `style` (DocumentTree 16,
  ExplainResultView 18, FiltersDialog 14, OpLogPanel 12, StackList 8); `ApprovalItemCard` lg spans;
  `TextPromptDialog` `text-kira-lg` div.
