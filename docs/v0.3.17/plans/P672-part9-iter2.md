# P672 Part 9 iter2: states, icons, type scale, spacing, radius, shadow, class soup

Iteration of `P672 Part 10` (master `plans/P672-part10.md`). Planned against `42a1885ba`. UI only:
no Go, no bridge, no new unit tests. First pass: `plans/P672-part9.md`. Sibling iterations run in
parallel: 7 (status, tones) and 8 (tabs, segmented, choice). Overlap is listed under Files touched.

## Rules applied

ARCHITECTURE "UI uniformity rules (P672)" and the icon table:

- State primitive by scope: InlineState for a list or section, EmptyState for a pane, Skeleton
  shapes for placeholders, Spinner only through EmptyState `loading` or RunState.
- Type roles: xl only on the hero, lg for detail and dialog titles, md body with no class.
- Spacing: content pane `p-3`, `gap-3` between blocks, `gap-2` inside. Steps 5 and up fail U88,
  except EmptyState inset, pairing hero and indent paddings.
- Icons: 13 in kira buttons, menus, rows and bars; 12 in Badge, chip, inline text and status or
  inline size buttons; 16 DetailHeader, card tile or title bar; 24 empty state.
- Icon table: delete (destroys stored data) `trash`; remove from a list, row or selection `close`;
  clear all `clear-all`; copy `copy`; duplicate `files`; edit `edit`; back to previous view
  `arrow-left`; step in a sequence `chevron-left`/`chevron-right`. Code reads names from `ICON`.
- Muted text is a tone (`text-muted-foreground`, `text-subtle`), never opacity. Disabled rows use
  `rowVariants({ disabled })`, muted rows `rowVariants({ muted })`.
- Shared class strings live in `packages/theme/src/lib/frames.ts` and `rowVariants.ts`; tree depth
  indent is `rowIndent(depth)`.

Deliberate exceptions kept, not re-opened: git ViewToolbar detail head, Studio tab view head,
AdeTaskCard and the plan board (D1, including its borders and pl-5/pl-6/pl-8 indents), bar value
toggles stay segmented, engine cards without descriptions, toasts.

## Current state (fresh scan, rerunnable)

Scan helper: scratchpad `p9i2/lib.py` (711 .vue files across the six guard DIRS minus
`components/ui/`, whole-file, tags spanning lines). Guard dry-runs: `rg -nUP` over the same DIRS.

- `bash scripts/check-ui-primitives.sh` green. 48 PERMANENT lines. Highest guard U146.
- Skeleton: 12 tags in 8 files, all canonical shapes. No finding.
- EmptyState: 124 tags, every one with `icon` or `loading`. No finding.
- InlineState: list and section empties all use it. No finding.
- Shadows: only tier classes; git-ui App drawer `shadow-kira` is a tier. No finding.
- `rounded-full`: dots, avatars, count badges, progress bars only. No finding.
- Spinner: only inside EmptyState `loading`, RunState and button busy slots. No finding.
- Text glyphs as icons (`>\s*[✕▶●•→↔]\s*<` plus `?\s*'●'`): 13 sites in 13 files (F2).
- Icon concept mismatches against the icon table: 11 sites (F1).
- Icon size off role: status-size buttons 8 icons, 6 at 12, 2 at 13 (F3). Inline-size buttons: 7 at
  13, 2 at 12 (F3 takes the inline text ones only).
- Opacity as muted or state, bare `opacity-50` outside `disabled:`: 9 hits in 7 files (F4).
- Hand depth indent `paddingLeft: \`...depth...\``: 5 hits in 5 files (F5).
- Margin stacking in a pane: `mt-6` one hit (U88 step), plus margin-stacked blocks in
  ChangeRequestView and DescribeView (F6).
- Disclosure: 11 hosts, 5 content offsets, 4 trigger looks (F7).
- `paneFrame` hand copies (`rounded-kira` + `bg-bg` in one class): 13 hits in 8 files; window body
  (`bg-chrome px-1.5 pb-0.5`): 5 hits in 5 files (F8).
- Type: one summary at `text-kira-lg` in a body role, two row names at `text-kira-lg` (F2, F9).

Codegraph areas covered (`codegraph_explore` calls): EmptyState, InlineState, Skeleton, Spinner;
ICON, STATE_ICONS, CodiconIcon; EditRows, MaskRuleRow, ApiAccessRulesSection, ParamsEditor;
StatusDot, WorktreeList, StackList, RefsPopoverBranches; rowVariants, MemoryPanel, MemoryContext,
VariablesOverviewPanel; rowIndent, DocumentTree, ExplainResultView, FiltersDialog, OpLogPanel;
Disclosure; frames.ts, paneFrame.

## Findings

Paths are relative to `apps/kira-space/frontend/src/` unless they start with `packages/` or
`apps/`.

### F1. Icon concept mismatches (icon table)

| Site | Now | Change |
|---|---|---|
| `views/repo/ReviewThread.vue:217` | `name="pencil"` (edit) | `:name="ICON.edit"` |
| `packages/git-ui/src/components/review/ReviewToolbars.vue:147` | back `icon="chevron-left"` | `:icon="ICON.back"` |
| `apps/kira-space/frontend/mobile/screens/TerminalScreen.vue:155` | back `chevron-left` 16 | `ICON.back`, size stays 16 |
| `ade/v2/details/AdeProofEntry.vue:70` | "Delete proof" `icon="close"` | `:icon="ICON.delete"` (stored proof is destroyed) |
| `studio/project/ConnectionFileDialog.vue:~389` | duplicate `name="copy"` | `:name="ICON.duplicate"` |
| `studio/views/httprequest/ResponseHistoryList.vue:~164` | clear history `ICON.delete` | `ICON.clearAll` (as CookiesPane, OpLogToolbar, ReviewCommentsPane) |
| `studio/views/grpcrequest/CallHistoryList.vue:~128` | clear history `ICON.delete` | `ICON.clearAll` |
| `packages/docker-ui/src/components/EditRows.vue:24` | remove draft row `trash` | `ICON.remove` |
| `packages/workbench/src/automations/form/ParamsEditor.vue:177` | remove draft param `trash` | `ICON.remove` |
| `studio/project/connection/MaskRuleRow.vue:69` | remove draft rule `trash` | `ICON.remove` |

Draft-row removes match FieldRowsTable, ApiVariablesTable, RepoEnvRow and the ParamsEditor option
row, which already use `close`. Kept `trash` (the remove destroys stored data at once):
ApiAccessRulesSection (instant settings write), AdeLeasesPane removeQueue, WorktreeList, AdeReposPage
repo remove. Each changed site imports `ICON` from `@theme/icons` if missing.

### F2. Text glyphs instead of icons or StatusDot

- `ade/v2/workflows/AdeWorkflowsTab.vue:174-179` and `ade/v2/workflows/AdeStepsTab.vue:~137-142`:
  the "✕" error span becomes
  `<span class="shrink-0 text-error" data-testid="..."><CodiconIcon :name="ICON.error" :size="13" /></span>`,
  keeping its testid and TextTooltip. Drop `text-kira-sm font-medium` from the span. Row name span
  (AdeWorkflowsTab:168, AdeStepsTab:~130) drops `text-kira-lg`: a row title is md, as siblings
  AdeStepBlock and AdeNoteEntry.
- `ade/v2/workflows/AdeStepNode.vue:179`: `▶` becomes
  `<span class="text-ok" data-testid="ade-wf-node-start"><CodiconIcon :name="ICON.run" :size="13" /></span>`.
- Dirty dot `•` (`text-warn text-kira-lg leading-none`, role img) becomes
  `<StatusDot tone="warn" :label="t('common.state.unsavedChanges.status')" data-testid="..." />`
  inside the existing TextTooltip, the shape `DiscoverSavedControls.vue:173` uses:
  - `studio/views/httprequest/HttpRequestView.vue:320-328` (`http-dirty`);
  - `studio/views/grpcrequest/GrpcRequestView.vue:285-293` (`grpc-dirty`).
- git-ui current marker `●`, three tones today. All become `<StatusDot tone="muted" ... />`,
  keeping `data-kira-tip` (or role/aria via StatusDot `label`):
  - `packages/git-ui/src/components/WorktreeList.vue:143-148` (`text-subtle`);
  - `packages/git-ui/src/components/StackList.vue:125-130` (`text-muted-foreground`);
  - `packages/git-ui/src/components/RefsPopoverBranches.vue:218-226` (`text-focus`): keep the
    `w-2.5` slot span for column alignment, render `<StatusDot v-if="row.isHead" tone="muted"
    :label="t('git.common.currentBranch.tooltip')" />` inside it, drop the ternary text and its
    role/aria attributes from the slot.
- Text arrows become `<CodiconIcon name="arrow-right" :size="12" />` (precedent AdeReviewHeader:107,
  AdeReviewSubmitDialog:129, MaskRuleRow:111, MaskRuleList:110), keeping any tone class on a wrapper
  or the icon:
  - `ade/v2/panel/AdeRunOutcome.vue:177`;
  - `studio/views/httprequest/TimelinePane.vue:258`;
  - `studio/views/httprequest/ResponseDiffDialog.vue:297` (muted);
  - `studio/views/grid/PreviewCommandPanel.vue:140` (muted).
- `packages/git-ui/src/components/review/ReviewToolbars.vue:160`: `↔` becomes
  `<CodiconIcon name="arrow-swap" :size="13" class="shrink-0 text-muted-foreground" />`.

Kept: `·` separators, `••••` masked secrets (data), DescribeView `×count`, AdeStepBlock and run-row
glyph boxes (`ADE_ROW_GLYPH`, plan-board family, D1), arrows inside i18n or computed strings
(AdeRunOutcome:91, StackDialog:114, tabKinds titles), AdeNotesEditor list glyph.

### F3. Icon size off role

- `packages/workbench/src/components/OpLogToolbar.vue:96` and
  `packages/workbench/src/memory/MemoryFilters.vue:90`: status-size filter trigger icon 13 becomes
  12 (the other 6 status-size buttons use 12).
- Inline text links and inline icons, 13 becomes 12:
  - `studio/project/ProjectTree.vue:310` (`tree-connection-broken-*` link);
  - `packages/git-ui/src/components/CommitMeta.vue:326` (change request icon beside subject).

Kept: OpLogPanel:521,534 row actions (row role, 13, as every other row action), IssueAttachments:46
(tile body), TabStrip close (row of tabs, `size-4` override), plan-board inline buttons (D1),
AdeAgentsStatusRow AdeActivityIcon (own component, not CodiconIcon), TerminalScreen touch-icon 16.

### F4. Opacity as muted or state

- `studio/views/shared/celleditor/TimestampPane.vue:119,123`: separator `opacity-50` becomes
  `text-subtle` (as AppMetricsItem).
- `studio/api/VariablesOverviewPanel.vue:138`: drop `{ 'opacity-50': row.shadowed }`. Scope Badge
  and tooltip already show the state.
- `packages/workbench/src/memory/MemoryPanel.vue:207`: drop `memory.historical && 'opacity-50'`.
  Historical Badge at :240 shows it (as Part 6 iter2 F6).
- `packages/workbench/src/memory/MemoryContext.vue:~36-41`: the row class becomes
  `rowVariants({ layout: 'menu', muted: fact.historical })`, dropping the hand opacity.
- `studio/kubernetes/components/ClusterList.vue:170`: drag `'opacity-50'` becomes
  `SORTABLE_GHOST_CLASS` from `packages/workbench/src/behaviour/sortableReorder.ts:7`.

Kept: `studio/views/console/kibana/FilterBar.vue:75`, disabled kibana filter pill dimmed like a
disabled control (built-in exempt in the U90 extension, requirement named there).

### F5. Hand depth indent (use rowIndent)

`:style="rowIndent(depth)"` from `@theme/lib/rowVariants` replaces:

- `studio/views/console/ExplainResultView.vue:165` (`depth*18+4`);
- `studio/project/FiltersDialog.vue:276` (`8+depth*14`);
- `packages/git-ui/src/components/StackList.vue:117` (`4+depth*8`; row keeps `px-1` minus the left
  padding: change class to `pr-1` so the style owns the left);
- `studio/views/shared/document/DocumentTree.vue:57` (`depth*16+4`).

Check each row's own `px-*` and replace with `pr-*` where the style now owns the left. Kept:
`packages/workbench/src/components/OpLogPanel.vue:476` (column-cell indent after the tree glyph,
not a row indent) and AdeBranchRow:427 (plan board, a width).

### F6. Pane spacing by margins

- `hostview/ChangeRequestView.vue:93-128`: main content stacks with `mb-3` (meta), the
  conversation's `mt-6` and `mt-4` (compose). Wrap main content in `flex flex-col gap-3`, drop the
  three margins. `hostview/HostViewConversation.vue:48` drops `mt-6`.
- `hostview/IssueView.vue:85`: `gap-4` becomes `gap-3`.
- `studio/kubernetes/components/DescribeView.vue:160,168`: pane `p-2` plus per-section `mb-3`
  becomes pane `flex flex-col gap-3 p-3`, sections drop `mb-3`.

### F7. Disclosure class soup

Hosts pass five content offsets (`pt-1`, `pt-1.5`, child `mt-1`, none, `pt-2` + child `mt-3`) and
four trigger looks. Centralise in `packages/theme/src/components/Disclosure.vue`:

- CollapsibleContent default `flex flex-col gap-2 pt-1`.
- New boolean prop `meta`: trigger adds `text-kira-sm text-muted-foreground` (secondary
  disclosure under a block). Default trigger stays `text-kira-md`, `font-medium`-free.
- Remove `triggerClass` and `contentClass` props.

Hosts drop their own offsets, child margins and trigger classes:

| Host | Change |
|---|---|
| `workbench/settings/FileTypesPane.vue:131` | drop content `flex flex-col gap-1 pt-1` |
| `ade/v2/port/AdePortExportDialog.vue:168` | drop `content-class="pt-1"` |
| `ade/v2/port/AdePortImportDialog.vue:254` | drop content `pt-1` |
| `ade/v2/panel/AdeArchivedPanel.vue:297` | drop content `flex flex-col gap-1 pt-1` |
| `studio/project/connection/PrivacyLimits.vue:15` | drop `trigger-class="text-fg font-medium"` and content `pt-1.5` |
| `studio/project/connection/MaskKeySection.vue:44` | drop `trigger-class="text-fg font-medium"` and content `flex flex-col gap-2 pt-1.5` |
| `packages/workbench/src/llmPrompts/PromptDetail.vue:158-160` | drop child `mt-1` |
| `packages/workbench/src/automations/runs/ScriptRunView.vue:143-149` | trigger classes become `meta`, drop child `mt-1` |
| `studio/views/httprequest/ResponseDiffDialog.vue:345` | trigger classes become `meta` |
| `studio/views/httprequest/TimelinePane.vue:313` | trigger classes become `meta`, drop `class="mt-0.5"` |
| `packages/git-ui/src/components/dialogs/ForcePushDialog.vue:140-174` | `trigger-class="text-muted-foreground"` becomes `meta`, drop `class="pt-2"` and children `mt-3` |

ForcePushDialog sits in a DialogBody (`gap-3`), so the outer `pt-2` goes too.

### F8. Hand pane frame and window body

- Add to `packages/theme/src/lib/frames.ts`:
  `export const windowBody = 'flex min-h-0 flex-1 gap-0.5 bg-chrome px-1.5 pb-0.5';` with doc
  line "Standalone window body: chrome gutter around framed panes."
- Window body hosts take `cn(windowBody, ...)`:
  `openedfiles/OpenedFilesWindow.vue:17`, `ade/v2/review/AdeReviewWindow.vue:106`,
  `configsync/ConfigSyncReviewWindow.vue:90`, `studio/proof/ProofWindow.vue:94`,
  `packages/workbench/src/components/WorkbenchShell.vue:63` (`cn(windowBody, 'workbench-shell
  flex-col')`). OpenedFilesWindow and ConfigSyncReviewWindow gain `gap-0.5` (single child each, no
  visual change).
- Pane frame hosts take `cn(paneFrame, '<placement>')`, placement being the classes not in
  `paneFrame`:
  - `openedfiles/OpenedFilesWindow.vue:19`;
  - `ade/v2/review/AdeReviewWindow.vue:108,128,162`;
  - `ade/v2/workflows/AdeWorkflowGraph.vue:309` (`flex min-h-96 flex-1`);
  - `ade/v2/workflows/AdeLibraryStepForm.vue:90` (`min-h-96 flex-1`);
  - `ade/v2/needs/AdeNeedsPage.vue:31`;
  - `configsync/ConfigSyncReviewWindow.vue:91`;
  - `studio/proof/ProofWindow.vue:97,154`;
  - `packages/workbench/src/components/WorkbenchShell.vue:67,120`, and `:88` becomes
    `:class="mainFramed ? paneFrame : undefined"` next to its static
    `h-full flex flex-col min-w-0 min-h-0 overflow-hidden` (twMerge keeps one copy).
- `paneFrame` adds `overflow-hidden min-h-0 min-w-0` on AdeWorkflowGraph and AdeLibraryStepForm.
  `min-h-96` wins over `min-h-0` through `cn` order. Graph scroller is absolute inside, popovers
  teleport, so clipping is safe. Implementer confirms both views still scroll.

### F9. Type role

- `studio/proof/ProofWindow.vue:105`: summary `text-kira-lg` dropped (body md), as sibling
  AdeProofEntry summary `text-kira-md`.
- Row names in F2 (AdeWorkflowsTab, AdeStepsTab).

## Exemptions re-checked

- UsageBar and EngineOverview stacked bar stay hand progress bars: Progress has no tone or segments.
- Docker row button `rounded-none` overrides (ResourceDetail:165, EngineOverview:272): checked,
  geometry of a flush table action, kept.
- ImportJobDetail:138 resume `debug-continue`: no icon table row covers resume; kept.
- TextPromptDialog title copies DialogTitle look (lg, normal weight): consistent, kept.
- OpLogPanel:476 indent and FilterBar:75 opacity: named requirements above.
- PERMANENT list unchanged (48).

## Guards

Rows in `scripts/check-ui-primitives.sh`. Write a backtick as `\x60`, non-ASCII glyphs as `\x{....}`,
no literal `$`. Each new or extended row lands with zero offenders after its findings.

- U73 extend in place: retired list `gear|settings|kebab-vertical` adds `pencil`. Base hit:
  ReviewThread:217 only.
- U88 extend in place: utility group adds `mt|mb|my|pt|pb`:
  `${TOK}(?:gap|gap-x|gap-y|p|px|py|mt|mb|my|pt|pb)-(?:0\.25|...|12)(?![\w.-])`. Base hit:
  HostViewConversation:48 `mt-6` only. Message unchanged.
- U90 extend in place: add `|(?<![\w:-])opacity-50(?![\w.-])` (never catches `disabled:opacity-50`
  or `group-hover:`). Built-in exempt `rowVariants.ts|sortableReorder.ts|FilterBar.vue` (FilterBar:
  disabled kibana filter pill dimmed like a disabled control). Message: "hover tone override, or
  opacity as muted or state (use tone, rowVariants muted/disabled, SORTABLE_GHOST_CLASS)". Base
  hits: the 9 in F4 plus the three exempts.
- G9a (new, kind `v`, exempt `OpLogPanel.vue`): "hand depth indent (use rowIndent)",
  regex `paddingLeft:\s*\x60[^\x60]*\bdepth\b`. Base hits: 5 (F5 plus OpLogPanel).
- G9b (new, kind `v`): "Disclosure trigger or content class (use the meta prop; content spacing is
  built in)", regex `<Disclosure\b(?&t)\s:?(?:trigger-class|content-class)=`. Base: every host
  with those attrs (ForcePushDialog, AdePortExportDialog, PrivacyLimits, MaskKeySection,
  ScriptRunView, ResponseDiffDialog, TimelinePane). Props are removed, so vue-tsc also fails a
  bound use.
- G9c (new, kind `v`, no exempt): "hand pane frame or window body
  (use paneFrame, windowBody)", regex
  `\bclass="(?=[^"]*${TOK}rounded-kira(?![\w-]))(?=[^"]*${TOK}bg-bg(?![\w-]))|\bclass="(?=[^"]*${TOK}bg-chrome(?![\w-]))(?=[^"]*${TOK}px-1\.5(?![\w.-]))(?=[^"]*${TOK}pb-0\.5(?![\w.-]))`.
  Base hits: 13 pane frame plus 5 window body, exactly F8.
- G9d (new, kind `v`): "text glyph as icon or status (use CodiconIcon or StatusDot)", regex
  `>\s*(?:\x{2715}|\x{2713}|\x{25B6}|\x{25CF}|\x{2022}|\x{2192}|\x{2194}|\x{21A9})\s*<|\?\s*'(?:\x{25CF}|\x{2022})'`
  (multiline as the script's other `(?s)` rows run). Base hits: the 13 sites of F2 (12 template
  text, RefsPopoverBranches ternary). Script-side maps (AdeStepBlock STEP_GLYPH, AdeNotesEditor,
  CredentialPastePanel) are in `<script>` or not ternary text, so they stay clear; confirm on the
  dry-run and narrow the regex rather than add exempts if one hits.

Header comment: add line `# Guards G9a-G9d (as assigned), U73/U88/U90 extensions:
docs/v0.3.17/plans/P672-part9-iter2.md.` PERMANENT stays 48. Orchestrator assigns final U ids;
implementer uses them everywhere G9a-G9d appears (script, ARCHITECTURE, SPEC).

## Docs updates (`docs/ARCHITECTURE.md`)

- Icon table: add "Remove a draft row from a form list before save: `close` (`ICON.remove`); trash
  only when the click destroys stored data."
- UI uniformity rules: add "Pane frame is `paneFrame`, standalone window body is `windowBody`
  (frames.ts); no hand copy." Add "Status glyphs are StatusDot or CodiconIcon, never text glyphs;
  arrows between values are `arrow-right` 12." Add "Disclosure owns its content spacing; secondary
  disclosures pass `meta`; no trigger or content classes." Add "Opacity never marks muted or
  state: tone, `rowVariants` muted/disabled, or `SORTABLE_GHOST_CLASS`." Spacing line: "blocks in a
  pane stack with `gap-3`, never margins".
- Guard range line: extend to the new highest id.

`docs/v0.3.17/SPEC.md` P672 Part 10 row, progress line last:
`Part 9 iter2: <result>, plan plans/P672-part9-iter2.md.`

## Stream decision

One sequential Sonnet implementer. Findings share `scripts/check-ui-primitives.sh`, theme pieces
used by later commits, and several files carry two findings (StackList F2+F5, ReviewToolbars F1+F2,
TimelinePane F2+F7, ResponseDiffDialog F2+F7, ProofWindow F8+F9). No split.

## Commits

1. `refactor(theme): Disclosure owns content spacing, add meta trigger; add windowBody frame`:
   Disclosure.vue, frames.ts. Hosts with removed props must land in the same commit to keep
   typecheck green: fold F7 host edits here, or keep the props until C6 and remove them there.
   Prefer: C1 adds `meta` and the default content class only; C6 removes the props with the hosts.
2. `refactor(ui): icon table concepts and sizes` (F1, F3).
3. `refactor(ui): icons and StatusDot instead of text glyphs; row title md` (F2, F9).
4. `refactor(ui): tone instead of opacity for muted and state` (F4).
5. `refactor(ui): rowIndent for tree depth indent` (F5).
6. `refactor(ui): Disclosure hosts drop hand spacing; remove trigger and content class props` (F7).
7. `refactor(ui): pane blocks stack with gap-3` (F6).
8. `refactor(ui): paneFrame and windowBody in window hosts` (F8).
9. `chore(ui): guards for indent, Disclosure classes, frames, glyphs; extend U73, U88, U90`.
10. `test(ui): ...` only if a spec reads a removed class or glyph text.
11. `docs: ARCHITECTURE UI rules for P672 Part 9 iter2`.
12. `docs: P672 Part 9 iter2 progress` (SPEC, last).

## Test split

Targeted UI specs only, after the whole plan lands. No new unit tests.

- `ui/ade-v2-workflows.spec.ts` (`ade-wf-row-error`, node start; YAML `✕ Line ...` text comes from
  `ade-wf-yaml-msg`, not touched).
- `ui/ui-leftovers.spec.ts`, `ui/studio/api-tab-close.spec.ts`,
  `ui/studio/api-request-conflict.spec.ts`, `ui/studio/grpc-request-conflict.spec.ts`,
  `ui/studio/grpc-request.spec.ts`, `ui/studio/collections.spec.ts`,
  `ui/studio/api-cross-window-sync.spec.ts` (`http-dirty`, `grpc-dirty`, history clear).
- `ui/studio/docker-edit.spec.ts`, `ui/ade-v2-proof-chain.spec.ts`,
  `ui/studio/connection-lifecycle.spec.ts`, `ui/studio/connection-files.spec.ts`,
  `ui/studio/connection-privacy.spec.ts`, `ui/studio/connection-mask-review.spec.ts`,
  `ui/copy-wording.spec.ts`, `ui/studio/api-ui-consistency.spec.ts`.
- `ui/memory-provenance.spec.ts`, `ui/memory-ranking.spec.ts`, `ui/operations.spec.ts`,
  `ui/studio/cell-editor.spec.ts`, `ui/studio/tree.spec.ts`, `ui/studio/kubernetes-module.spec.ts`.
- `ui/ade-v2-review.spec.ts`, `ui/repo-review-interaction.spec.ts`, `ui/ade-v2-force-push.spec.ts`,
  `ui/settings-file-types.spec.ts`, `ui/ade-v2-automations.spec.ts`, `ui/opened-files.spec.ts`,
  `ui/config-sync.spec.ts`, `ui/studio/proof-ready.spec.ts`, `ui/consistency-git.spec.ts`,
  `ui/git-host-provider.spec.ts`.
- Mobile terminal spec after `bun run build:mobile` (TerminalScreen back icon).
- Visual baselines (`visual/studio/connection-privacy`, `visual/studio/proof-window`, review,
  plan) re-record in PR CI only.
- Typecheck, lint, `bash scripts/check-ui-primitives.sh` per commit.

## Risks

- N10 600-line cap: HttpRequestView 578, GrpcRequestView 539; StatusDot swap is net zero to minus
  a few lines. Recount every touched file.
- Removing Disclosure props: any missed host falls through as attrs on Collapsible; G9b and
  vue-tsc catch it.
- `paneFrame` on AdeWorkflowGraph and AdeLibraryStepForm adds clipping; check graph scroll and
  inspector scroll.
- StatusDot `muted` on git-ui markers overlaps iteration 7 tone work: if iteration 7 lands a
  different tone for "current", follow its rule; marker becomes StatusDot either way.
- reka toggles need `delay: 40` in any spec touched.

## Out of scope

UsageBar and EngineOverview progress bars, docker row button overrides, resume icon, plan board
(D1), AdeStepBlock glyph boxes, masked-secret bullets, arrows inside strings.

## Handoff

Implementer starts at `42a1885ba` plus sibling plan commits. Read this file and
`plans/P672-part10.md`. Implement C1-C12 in order, rerun guard dry-runs after C8, then targeted
specs. Report final U ids used, per-guard base hit counts, line counts of touched files near 600.

## Files touched

Shared (other iterations may edit):

- `scripts/check-ui-primitives.sh`
- `docs/ARCHITECTURE.md`
- `docs/v0.3.17/SPEC.md`
- `packages/theme/src/components/Disclosure.vue`
- `packages/theme/src/lib/frames.ts` (new export `windowBody`)

Overlap with iteration 7 (status, tones):

- `packages/git-ui/src/components/WorktreeList.vue`, `StackList.vue`, `RefsPopoverBranches.vue`
  (StatusDot tone on current marker)
- `studio/views/httprequest/HttpRequestView.vue`, `studio/views/grpcrequest/GrpcRequestView.vue`
  (dirty StatusDot)
- `packages/workbench/src/memory/MemoryPanel.vue`, `MemoryContext.vue`,
  `studio/api/VariablesOverviewPanel.vue` (opacity as state)

Own:

- `views/repo/ReviewThread.vue`
- `packages/git-ui/src/components/review/ReviewToolbars.vue`
- `apps/kira-space/frontend/mobile/screens/TerminalScreen.vue`
- `ade/v2/details/AdeProofEntry.vue`
- `studio/project/ConnectionFileDialog.vue`
- `studio/views/httprequest/ResponseHistoryList.vue`
- `studio/views/grpcrequest/CallHistoryList.vue`
- `packages/docker-ui/src/components/EditRows.vue`
- `packages/workbench/src/automations/form/ParamsEditor.vue`
- `studio/project/connection/MaskRuleRow.vue`
- `ade/v2/workflows/AdeWorkflowsTab.vue`, `AdeStepsTab.vue`, `AdeStepNode.vue`,
  `AdeWorkflowGraph.vue`, `AdeLibraryStepForm.vue`
- `ade/v2/panel/AdeRunOutcome.vue`, `ade/v2/panel/AdeArchivedPanel.vue`
- `studio/views/httprequest/TimelinePane.vue`, `ResponseDiffDialog.vue`
- `studio/views/grid/PreviewCommandPanel.vue`
- `packages/workbench/src/components/OpLogToolbar.vue`, `packages/workbench/src/memory/MemoryFilters.vue`
- `studio/project/ProjectTree.vue`, `packages/git-ui/src/components/CommitMeta.vue`
- `studio/views/shared/celleditor/TimestampPane.vue`
- `studio/kubernetes/components/ClusterList.vue`, `DescribeView.vue`
- `studio/views/console/ExplainResultView.vue`, `studio/project/FiltersDialog.vue`,
  `studio/views/shared/document/DocumentTree.vue`
- `hostview/ChangeRequestView.vue`, `HostViewConversation.vue`, `IssueView.vue`
- `workbench/settings/FileTypesPane.vue`, `ade/v2/port/AdePortExportDialog.vue`,
  `AdePortImportDialog.vue`, `studio/project/connection/PrivacyLimits.vue`, `MaskKeySection.vue`,
  `packages/workbench/src/llmPrompts/PromptDetail.vue`,
  `packages/workbench/src/automations/runs/ScriptRunView.vue`,
  `packages/git-ui/src/components/dialogs/ForcePushDialog.vue`
- `openedfiles/OpenedFilesWindow.vue`, `ade/v2/review/AdeReviewWindow.vue`,
  `ade/v2/needs/AdeNeedsPage.vue`, `configsync/ConfigSyncReviewWindow.vue`,
  `studio/proof/ProofWindow.vue`, `packages/workbench/src/components/WorkbenchShell.vue`
- Specs only if C10 is needed.
