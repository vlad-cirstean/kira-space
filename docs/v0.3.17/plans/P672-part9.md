# P672 Part 9 UI uniformity: states, icons, type, spacing

Spec row: `../SPEC.md` P672 Part 9. Rules RE.1-RE.3, RI.1, RTy.1-RTy.3, RSp.1-RSp.2, RBo.1-RBo.3;
migration items M30, M32-M36 (`P672-inventory.md` §10, §13-§16, appendix A.10). M31 (RE.4 toasts)
is dropped: decision D3 keeps toasts app-wide as they are. Base: `chapter/v0.3.17` at `670d1ad52`
(Parts 1-8 Done). Inventory line numbers are at `6527aec4`. Every fact below was re-checked at
`670d1ad52` via `codegraph_explore` and fresh class-token scans. Line numbers are hints: implementer
greps the named pattern or `data-testid` before editing.

UI-only phase. Look and layout change only. Behaviour, `v-model` wiring and every `data-testid`
stay, except `host-view-failure-reason` (C3). i18n: reuse existing keys only (`common.state.loadFailed.title` "Couldn't load",
`common.action.retry.label`, `common.state.noMatches.empty`, view-specific failure keys). No IPC,
Go or flow change.

Part 9 is the last first-pass Part. After C1 no guard keeps a TEMPORARY entry, and C1 removes the
TEMPORARY block and its whole mechanism from `scripts/check-ui-primitives.sh`.

## Rules applied (recap)

- RE.1 state primitive by scope: whole pane `EmptyState fill`, list or section `InlineState`,
  failed operation with content kept `Alert error`, invalid field `FieldError`. A pane-level state
  drawn by hand (`div` with centred muted text, `Alert` standing in for an empty or failed pane) moves
  to `EmptyState`.
- RE.2 `EmptyState` is icon (24, muted) plus title, optional description, one optional action
  (`toolbar` `kira`, or `link` `inline`). Every `EmptyState` has `icon` or `loading`. Loading is the
  `loading` prop (Spinner after `SPINNER_DELAY`), never a `#media` Spinner. No toned media
  (`text-error`, `text-warn` glyph); `#media` stays only for a content illustration (none today).
  Failure: `icon="warning"`, title `common.state.loadFailed.title` or the view's own failure key,
  the error text as `description`, Retry `toolbar@kira`. Side panels keep no button (U78).
- RE.3 Skeleton shapes: text `h-4` with `w-full`, `w-2/3` or `w-1/3`; title `h-control-lg w-1/3`;
  block `h-24 w-full`.
- RI.1 icon sizes by role: 13 in labelled and icon buttons of size `kira`, `kira-lg`, `kira-icon`,
  in menus, rows, bars; 12 in Badge, chip, inline text and `status`/`inline` size buttons (their
  text is `text-kira-sm`); 16 in DetailHeader, card tile and title bar (`size="title"`,
  `title-labelled`); 24 in empty states.
- RTy.1 `text-kira-xl` only for a landing hero (ModuleStart title, pairing codes). RTy.2
  `font-data`, never `font-mono`; code blobs are `CodeBlock`. RTy.3 `leading-normal` only on
  prose, `leading-none` only on icon-aligned lines; hand `tracking-*`, `uppercase`, `normal-case`
  only inside header primitives (SectionHeading, PanelHeader, DetailSection) and masked-secret or
  pairing-code data.
- RSp.1 content pane root `p-3`, `gap-3` between blocks, `gap-2` inside. RSp.2 scale 0.5, 1, 1.5,
  2, 3, 4.
- RBo.1 `rounded-kira-sm` for controls, rows, chips, inset blocks; `rounded-kira` for containers
  (pane frames, cards, dialogs); `rounded-full` only for dots, avatars, count badges. RBo.2 `border-2`
  and `border-l-3` go except plan board and glyph outlines; `border-l-2` only for quotes. RBo.3
  shadows only from elevation tiers, no arbitrary `shadow-[...]`.

## Current state (scans at `670d1ad52`)

Frontend paths relative to `apps/kira-space/frontend/`.

- `font-mono` (U115, TEMPORARY): 5 files, listed in C1.
- `EmptyState` with `#media`: 11 (5 Spinner, 6 toned icon).
- `EmptyState` without `icon` or `loading`: 16.
- Failure `EmptyState` with the error text as title: 4 repo views.
- Hand-drawn or `Alert` pane states: 11 hosts.
- Off-shape Skeletons: 4 hosts.
- Labelled or icon buttons (`kira`, `kira-lg`, `kira-icon`) with a 12 icon: 25 sites in 20 files;
  title-bar review buttons at 12: 5 sites in 2 files.
- `text-kira-xl` off hero: 5 sites (docker stats). Off-rule `leading-*`: 5. Hand case or tracking
  resets in header slots: 4, plus 1 redundant `tracking-normal`.
- Pane roots `p-4`: 15 files; off-scale `gap-5`/`gap-6`/`py-6`: 6 sites.
- `border-l-3` off plan board: 2 rows; arbitrary inset shadows: 2; non-quote `border-l-2` accent:
  1; `rounded-kira` on rows or inset blocks: 5.

N10 (600-line cap) budget, files at the edge: `BrowseView.vue` 599, `DocumentView.vue` 599,
`ConsoleResultGrid.vue` 599, git-ui `ReviewView.vue` 599, `CellEditorView.vue` 591,
`AdeBranchPanel.vue` 578, `OpLogPanel.vue` 576. Each edit below states its net line change for
these; anything that would cross 600 trims a comment or folds an attribute in the same file.

## Baselines

`scripts/check-ui-primitives.sh` at `670d1ad52`:

- PERMANENT: 50 lines. One reason text changes (C9, U26 `OpLogPanel.vue`); none added or removed.
- TEMPORARY (lines 152-161): 5 lines, all `U115 ... # 2026-11-18 P672 Part 9`:
  `AdeDetailsDiskBanner.vue`, `AdePanelForwardBar.vue`, `DetailsEditBody.vue`, `ProofHeader.vue`,
  `FileTypesPane.vue`. C1 deletes all five with their fix.
- TEMPORARY mechanism (removed in C1 once the block is empty):
  - header comment line 15 ("their current offenders sit in the dated TEMPORARY block");
  - comment and heredoc lines 152-161 (`cat >"$TMP/temp" <<'TEMPORARY'`);
  - malformed check lines 360-365 (regex `^U11[0-7] ... P672 Part [3-9]$`);
  - "in PERMANENT and TEMPORARY" check lines 366-371 (`temp_ids`, `both`);
  - `used_temp` branch in the hit loop lines 372-384 (keep the `used_perm` branch and the
    unexempted-hit `else`);
  - stale temporary check lines 394-400;
  - count line 423-424 ("temporary P672 baseline entries left").
  After removal `grep -n 'temp\|TEMPORARY' scripts/check-ui-primitives.sh` names nothing but
  unrelated words. Remaining self-checks: PERMANENT stale exemption, unexempted hit, stale built-in
  exemption. Verify each still fires (scratch edits, reverted, never committed): a bogus PERMANENT
  line (stale exemption), `font-mono` added to a scratch `.vue` (U115 hit), a dead built-in exempt
  name on any guard row (stale built-in exemption).

Recount PERMANENT after C9:
`awk '/<<.PERMANENT/{f=1;next} /^PERMANENT$/{f=0} f && /^U/' scripts/check-ui-primitives.sh | wc -l`
(expect 50).

## Theme pieces

- `EmptyState.vue`: no new prop. Comment line 14 changes from "`media` slot replaces the icon
  (tinted icon)" to "`media` only for a content illustration; loading is the `loading` prop". First
  consumer of the rule: C2.
- `PanelHeader.vue` (workbench): `actions` wrapper gets `normal-case tracking-normal`, same as the
  `start` wrapper. First consumer: `AdeNeedsPage.vue` drops its own reset (C7).
- `DetailSection.vue` (theme): `<CardAction class="normal-case tracking-normal">`. First consumers:
  docker `ContainerSizeSection.vue`, `EngineDiskSection.vue` drop their reset (C7). Check
  `CardAction` merges `class` via `cn` (shadcn-vue default) before relying on it.
- No new component, store or composable. No library added: every piece is shadcn-vue `Empty`,
  `Skeleton`, theme `CodeBlock`, `EmptyState`, `InlineState`; VueUse, Pinia and TanStack Query
  untouched (no state or fetching change; `QueryState` keeps its TanStack Query contract).

## Stream decision

One sequential implementer. Every group touches shared files (`ContainerDetail.vue` and
`EngineOverview.vue` in states, icons and type; `HostViewFrame.vue` in states and spacing;
`AdeStepsTab.vue` and `AdeWorkflowsTab.vue` in spacing and borders), and C10 plus C11 edit the guard
script and ARCHITECTURE after all fixes. No split.

## Commits

Order: C1-C9 fixes, C10 guards, C11 test (only if needed), C12 ARCHITECTURE, C13 SPEC. Spec edits
that a fix breaks ride in that fix commit. Run `bash scripts/check-ui-primitives.sh` and
`bun run lint` (biome catches unused imports such as `CodiconIcon`, `Spinner`, `Alert` left behind)
before every commit.

### C1 `refactor(ui): font-mono to font-data and CodeBlock (M33)`

U115 offenders (delete each TEMPORARY line in this commit):

- `src/workbench/settings/FileTypesPane.vue:153` `<span class="font-mono">` to `font-data`.
- `src/studio/proof/ProofHeader.vue:46` `font-mono` to `font-data` (rest of the class stays).
- `src/ade/v2/request/DetailsEditBody.vue:57-60` `<pre v-if="file.text" class="max-h-48
  overflow-auto rounded-kira-sm bg-field p-2 font-mono text-kira-sm">` to
  `<CodeBlock v-if="file.text" class="max-h-48 overflow-auto">`.
- `src/ade/v2/panel/AdePanelForwardBar.vue:112-115` preview `pre` inside `CollapsibleContent` to
  `CodeBlock class="max-h-60 overflow-auto"`, keep `data-testid="ade-panel-forward-preview"`.
- `src/ade/v2/details/AdeDetailsDiskBanner.vue:118-121` diff `pre` to `CodeBlock`, keep its
  testid and any `max-h`/`overflow` class.

Other hand code blobs (RC rule; not U115 hits): `src/ade/v2/settings/AdeSettingsResult.vue:95`
(`rounded-kira bg-field p-2` pre) to `CodeBlock`. Read `AdeReviewRemoved.vue:111` and git-ui
`ConflictBanner.vue:146`: convert if a hand `pre`/`code` block with its own bg, radius or padding;
leave plain inline text. `DescribeView.vue:227` (kube manifest) and docker `InspectView.vue:20` are
full-pane viewers, not inset blocks: keep `pre`, fix leading only in C7.

`CodeBlock` is `Card` with `select-text font-data text-kira-sm whitespace-pre-wrap break-all`; class
on it only places it (U124 allows `max-h-*`, `overflow-*`). Specs grepping `pre` inside those testids
(grep `ade-panel-forward-preview`, `ade-details-disk`, `details-edit`) switch to the testid.

Guard script in the same commit: delete the five lines, then the mechanism listed in Baselines;
header line 15 becomes `# Guards U110-U117: docs/v0.3.17/plans/P672-part2.md.` Run the three
scratch self-check probes.

### C2 `refactor(ui): EmptyState loading and muted icon instead of media (M30)`

Spinner media to `loading`:

- `src/studio/views/shared/keyvalue/KeyValueRowsTable.vue:188`
  `:icon="rt ? 'database' : undefined"` plus `#media` Spinner to `icon="database" :loading="!rt"`.
- `src/studio/views/stream/StreamRowsTable.vue:259`, `src/studio/views/documents/DocumentView.vue:455`
  (599 lines; net change must be zero or negative), git-ui `StashDetailPane.vue:103`, git-ui
  `DetailPane.vue:111`: same shape, read each condition and map it to `loading`.

Toned media to `icon`:

- kubernetes `ClusterUnavailable.vue:56` (`debug-disconnect text-error`) to
  `icon="debug-disconnect"`.
- docker `ContainerDetail.vue:127` (`warning text-warn`) to `icon="warning"`.
- docker `UnavailableState.vue:53`, kubernetes `ConnectionState.vue:22`: `icon="debug-disconnect"`.
- `GitBlockedPanel.vue:31` (`STATE_ICONS.warning text-error`) to `:icon="STATE_ICONS.warning"`.
- workbench `QueryState.vue:85` (`error text-error`) to `icon="warning"`; title stays
  `common.state.loadFailed.title`, description stays the error message, Retry `toolbar@kira` stays.

Remove now-unused `CodiconIcon`/`Spinner` imports (ConnectionState, GitBlockedPanel, ContainerDetail
keeps CodiconIcon for buttons). `EmptyState.vue` comment per Theme pieces.

Failure shape (RE.2), error text out of the title:

- `src/views/repo/RepoGraphView.vue:101` `:title="errorMessage"` to
  `:title="t('git.graph.loadFailed.error')" :description="errorMessage"` (keep `icon="warning"`);
  if `errorMessage` already equals that key's text, use `common.state.loadFailed.title`.
- `src/views/repo/RepoDiffView.vue:357-361` title `errorMessage || openDiffFailed` to title
  openDiffFailed key, `description` errorMessage.
- `src/views/repo/LocalFileView.vue:139-143`, `RepoFileView.vue:280-284`: title openFileFailed key,
  `description` errorMessage.

Specs: `consistency.spec.ts:287/296/307/516` (`query-error`), `repo-graph-failures.spec.ts:177`
(`git-blocked-panel`), `copy-wording.spec.ts:360/381`, `copy-coverage.spec.ts:754-755`,
`studio/kubernetes-module.spec.ts:62/67/207`, `studio/kubernetes-tabs.spec.ts:226`,
`studio/docker-module.spec.ts:35-36`, `repo-workspace.spec.ts:315/353`,
`copy-coverage-git.spec.ts:325`. A `toHaveText(errorMessage)` on an `EmptyState` title becomes a
check on `[data-slot="empty-description"]` inside the host testid (slot names in
`packages/theme/src/components/ui/empty/`: `empty-title`, `empty-description`, `empty-icon`).

### C3 `refactor(ui): hand-drawn and Alert pane states to EmptyState (M30)`

- `src/views/repo/RepoReviewView.vue:72` `<Alert v-else variant="error">` to
  `<EmptyState v-else fill icon="warning" :title="t('common.state.loadFailed.title')"
  :description="errorMessage" />`, keep its testid.
- `src/ade/v2/review/AdeReviewFiles.vue:223` Alert info with errorMessage: same shape, no button.
- `src/hostview/HostViewFrame.vue`:
  - loading (73-77): root `p-4` to `p-3`; Skeletons `h-4 w-1/2`, `h-4 w-full` x2 to
    `h-control-lg w-1/3`, `h-4 w-full`, `h-4 w-2/3`; keep `host-view-loading`.
  - failure (78-85): `<EmptyState v-else-if="failure" fill icon="warning"
    data-testid="host-view-failure" :title="t('common.state.loadFailed.title')"
    :description="failure">` with Retry `<Button size="kira" variant="toolbar"
    data-testid="host-view-retry">` in the default slot. `EmptyState` forwards attrs to its root
    only, so `host-view-failure-reason` goes (the one deliberate testid change of this Part): the
    four specs read `${t('host-view-failure')} [data-slot="empty-description"]` instead, edited in
    this commit (`host-view.spec.ts:134/143`, `issue-view.spec.ts:127/135`,
    `issue-view-fetch.spec.ts:66`; `github-pr-open.spec.ts:23` if it names the reason). Drop `Alert`
    import.
- git-ui `App.vue:1721-1732` boot error: `div` (`p-4`, muted `p`, `dialog@kira` Retry) to
  `<EmptyState v-if="bootError && !repoState" fill icon="warning" data-testid="boot-error"
  :title="t('common.state.loadFailed.title')" :description="t('git.app.bootError.error', { error:
  bootError })">` with Retry `variant="toolbar" size="kira" data-testid="boot-retry"`. App.vue is a
  named N10 host; keep the G12 comment.
- `src/studio/views/console/ConsoleResultGrid.vue:447-461` (599 lines): the two `no-rows` divs and
  their 2-line comment become:
  `EmptyState v-if="page?.rowCount === 0" class="no-rows absolute inset-0" icon="table"
  :title="t('studio.common.noRows.empty')"`, then `<div v-else-if="!page" class="no-rows absolute
  inset-0" />` (blank while no page, as today; later branches read `page`), then
  `EmptyState v-else-if="rowIndices.length === 0" class="no-rows absolute inset-0"
  data-testid="console-no-matching-rows" :icon="ICON.search"
  :title="t('studio.common.noMatchingRows.empty')"`. Net: 16 lines out, 14 in, 2 imports
  (`EmptyState`, `ICON` from `@theme/icons`) = 599. U85 allows `absolute inset-0`.
- docker `ContainerDetail.vue:112-118` recreating `div` to `<EmptyState v-if="recreating" fill
  loading data-testid="docker-detail-recreating" :title="t('docker.detail.recreating.loading')" />`;
  drop `Spinner` import. Note `loading` hides the state for `SPINNER_DELAY`: specs asserting
  `docker-detail-recreating` visible right after Apply may need `toBeVisible({ timeout })` (they use
  default 5s; fine).
- `src/ade/v2/review/AdeReviewWindow.vue:140` `<p class="m-0 p-4 text-muted-foreground"
  data-testid="ade-review-empty">` to `EmptyState fill icon="diff"` with the same title text and
  testid.
- `src/ade/v2/needs/AdeNeedsPage.vue:50-56` `ade-needs-empty` div (`px-3 py-6 text-kira-lg text-ok`)
  to `<EmptyState v-if="needs.empty" icon="check" data-testid="ade-needs-empty"
  :title="t('space.ade.needsPage.empty.emptyHint')" />` (no fill: footer follows). Spec
  `ade-v2-needs.spec.ts` keeps reading the testid text.
- studio `FkPreviewPopover.vue:129-136` Alert info `fk-preview-empty` in a popover list to
  `<InlineState kind="empty" data-testid="fk-preview-empty" ...>` (check `InlineState` props).
  Spec `studio/interaction.spec.ts:1683`.
- `src/studio/views/shared/celleditor/CellEditorView.vue:547-559` Alert info faking an empty pane
  (`flex-1 basis-5/12 min-h-0 overflow-auto items-center text-subtle`) to a wrapper
  `<div class="min-h-0 flex-1 basis-5/12" data-testid="cell-editor-decoded-empty">` holding
  `<EmptyState fill icon="warning" :title="..." />` with the same text. Stays under 600 (591 + about
  4). Specs `studio/cell-editor.spec.ts:1082/1090/1249-1250` read testid text: keep text identical.
- `src/ade/v2/sessions/AdeTuiPane.vue:87` phone overlay (`absolute inset-0 ... bg-bg p-4`): `p-4` to
  `p-3` only; it is an overlay with live content, not an empty state.

### C4 `refactor(ui): icon on every EmptyState (M30)`

Add `icon` (24, muted) to each icon-less host; no-match empties use `:icon="ICON.search"` (from
`@theme/icons`, same as `SearchFilterNotice.vue`):

- `src/ade/v2/workflows/AdeWorkflowGraph.vue:432` `ade-wf-no-valid`: `icon="type-hierarchy"`.
- `src/ade/v2/workflows/AdeLibraryStepForm.vue:118` `ade-step-no-valid`: `icon="symbol-event"`.
- `src/ade/v2/panel/AdePanel.vue:49` `ade-panel-empty`: `icon="tasklist"`.
- `src/studio/views/browse/BrowseView.vue:495` `browse-empty` items: `icon="folder"`; `:501`
  no-matches: `:icon="ICON.search"`. File is 599: two added lines need one trimmed (fold a comment
  or put the icon on the `fill` line only if prettier keeps it; run `bun run format` and recount).
- kubernetes `ResourceBrowser.vue:75` `kube-kinds-empty`: `icon="symbol-misc"`; `ClusterList.vue:150`
  `kube-clusters-empty`: `icon="server"`; `DescribeView.vue:160` and `ResourceTable.vue:158`
  no-matches: `ICON.search`.
- mobile `screens/NeedsScreen.vue:82` `needs-empty`: `icon="check"`; `screens/PlanScreen.vue:24`
  `plan-empty`: `icon="tasklist"`.
- workbench `memory/MemoryPanel.vue:177` `memory-empty`: computed icon `ICON.search` when filtered,
  `book` otherwise; `memory/import/ImportView.vue:56` `import-empty`: `icon="cloud-download"`.
- git-ui `components/review/ReviewView.vue:486` no-repo: `icon="repo"` (599 + 1 = 600, at the cap).
- Any further hit the C10 dry run of U131 reports: fix here before C10.

Icons are codicon names; implementer checks each exists in the codicon set the theme ships
(`CodiconIcon` renders nothing for an unknown name).

### C5 `refactor(ui): Skeleton to the three shapes (M30)`

- `src/ade/v2/links/AdeLinkStrip.vue:183` three `h-4.5 w-16 rounded-kira-sm` to one
  `Skeleton class="h-4 w-1/3"`; `ade-v2-links-strip.spec.ts:365` `toHaveCount(3)` to
  `toHaveCount(1)` in this commit.
- `src/ade/v2/panel/AdeTaskIssueLine.vue:62` `h-3 w-32` to `h-4 w-1/3`
  (`ade-v2-issue-tracker.spec.ts:69/75` only checks visibility).
- workbench `OpLogPanel.vue:333` `h-4.5` to `h-4 w-full`.
- kibana `HitsHistogram.vue:187` `h-full w-full` inside the fixed `h-32` chart box: keep; documented
  in C12 as the block shape filling a fixed-height chart host (`h-24` would leave a gap under it).
- Already canonical (no change): docker `ContainerDetail`/`ContainerEditView` (`h-control-lg w-1/3`,
  `h-4 w-2/3`), `AdeRunDialog` (`h-24 w-full`, spec `ade-v2-run.spec.ts:99/103`).

### C6 `refactor(ui): button icons 13 and title bar 16 (M32)`

12 to 13 in `kira`/`kira-lg`/`kira-icon` buttons:

- docker: `ContainerDetail.vue:154/165/176`, `EngineOverview.vue:187/198/207`,
  `ExecView.vue:52/76`, `UnavailableState.vue:66`.
- ADE: `AdeRunSandboxBadge.vue:78`, `plan/AdeDayBand.vue:162/227`, `plan/AdeHistoryControl.vue:24`,
  `sessions/AdeSessionStrip.vue:59`, `sessions/AdeSessionsMenu.vue:43`,
  `workflows/inspector/AdeStepResults.vue:126`, `workflows/statuses/AdeTaskStatuses.vue:257`,
  `notes/AdeNotesEditor.vue:246`, `review/AdeReviewRemoved.vue:98`, `run/AdeRepoGateDialog.vue:99`.
- kubernetes `ClusterUnavailable.vue:74`; studio `MaskKindMenu.vue:36`; kibana
  `DiscoverSavedControls.vue:206`.
- workbench `automations/form/ScheduleFields.vue:150`, `automations/run/AdeContextFields.vue:67`,
  `memory/settings/AgentMcpSection.vue:74`.
- workbench `OpLogDetail.vue:88/108` `TooltipIconButton :icon-size="12"`: drop the prop (default
  13).

12 to 16 in title-bar buttons (`size="title-labelled"`, matching WorkbenchTitleBar, HostViewFrame,
TitleBarWindowActions): `AdeReviewSync.vue:67` (`:icon-size="12"`), `:87`, `:105`;
`AdeReviewActions.vue:126`, `:139`. Check the `title-labelled` size gap fits 16 (it already hosts
16 in HostViewFrame).

Keep 12: `ExecView.vue:69` session-chip close; `status`/`inline` size hosts (StatusBarItem,
AdeTaskChildren, `OpLogDetail.vue:167`, AdeProofEntry, AdeStepMenu, AboutPane).

Implementer reruns the scan before editing: a `<Button` whose `size` is `kira`, `kira-lg` or
`kira-icon` (or no size inside a toolbar) containing `<CodiconIcon ... :size="12"`. Not guarded
(size follows host role; a regex cannot tell a chip button from a toolbar button).

### C7 `refactor(ui): type sizes, leading and header tracking (M33)`

- docker `StatsView.vue:33/44`, `EngineOverview.vue:217/241/259` `text-kira-xl font-medium` to
  `text-kira-lg font-medium`.
- `ModuleStart.vue:17` drop redundant `tracking-normal`.
- Leading: `ModeSwitcher.vue:45` `leading-3.5` to `leading-none`; kube `DescribeView.vue:227`,
  `LocalFileView.vue:114`, `RepoFileView.vue:257` `leading-relaxed` to `leading-normal` (the two
  file views also `p-4` to `p-3`); `ProofWindow.vue:105` `leading-snug` to `leading-normal`.
- Header resets: `PanelHeader.vue` actions wrapper and `DetailSection.vue` `CardAction` get
  `normal-case tracking-normal` (Theme pieces); drop the reset from `AdeNeedsPage.vue:36`,
  `ContainerSizeSection.vue:27`, `EngineDiskSection.vue:59`. `ProofScreenshotMenu.vue:124`: find the
  ancestor setting `uppercase`; if a header primitive, the reset moves there; if none, drop it as a
  no-op.
- Keep (exceptions, named in C12): `LogLines.vue:114` `leading-relaxed` (virtual row height 18),
  AutocompleteField mirror overlay, `AdeTaskCard.vue:279` `leading-3.5` (plan board),
  DetailHeader `leading-snug` (header primitive), `CellEditorView.vue:529/567` `leading-4` (bar
  strip), `VariableRow`/`VariableHistoryMenu` `tracking-widest` (masked secret dots),
  `MobilePairingDialog`/`PairScreen` `text-kira-xl tracking-widest` (pairing code hero).

### C8 `refactor(ui): pane roots p-3 and on-scale gaps (M34)`

`p-4` to `p-3` on pane roots: ADE `AdeLeasesPane.vue:49`, `workflows/AdeWorkflowsTab.vue:208`,
`workflows/AdeWorkflowsPage.vue:52`, `workflows/AdeStepsTab.vue:162`,
`settings/AdeSettingsPane.vue:49`, `context/AdeContextPane.vue:37`; hostview
`ChangeRequestView.vue:93`, `IssueView.vue:84`, `IssueSidebar.vue:26`, `HostViewSidebar.vue:67`;
workbench `DistillView.vue:130`, `MemoryStart.vue:61`, `BootFailure.vue:18`, `ModuleStart.vue:13`.
Rescan `class="[^"]*\bp-4\b` on root elements first; dialogs, popovers and cards keep their
primitive inset.

Off-scale gaps (scale tops at 4): `IssueSidebar.vue:26`, `HostViewSidebar.vue:67` `gap-5` to
`gap-3`; `IssueView.vue:85`, `AdeStepInspector.vue:227`, `AdeSettingsPane.vue:40` `gap-6` to
`gap-4`. (`AdeNeedsPage` `py-6` went in C3.) Indent paddings (`pl-5`, `pl-6`, `pl-8`) align nested
content to a parent glyph; they stay and are outside the extended U88.

### C9 `refactor(ui): radii, borders and shadows by role (M35)`

- `AdeStepsTab.vue:60-66` and `AdeWorkflowsTab.vue:32-37` `rowClass`: drop
  `rounded-kira border-0 border-l-3 border-solid` and the
  `selected ? 'border-l-tone-amber-solid' : 'border-l-transparent'` term; selection comes from
  `rowVariants({ selected })`. Keep `box-border w-full flex-col items-stretch gap-0.5
  whitespace-normal px-2 py-2 text-left`. Specs reading the amber class (grep
  `border-l-tone-amber`) switch to `data-selected` or `aria-selected`, whichever `rowVariants`
  hosts expose.
- studio `DocumentRow.vue:55` `selected && 'shadow-[inset_2px_0_0_var(--primary)]'`: drop; pass
  `selected` to the row's `rowVariants` call (check it already does; then the shadow was a second
  marker).
- workbench `OpLogPanel.vue:399` `'bg-error/15 shadow-[inset_2px_0_0_var(--color-error)]'` to
  `'bg-error/15'`; PERMANENT U26 reason "error log row wash at the shared /15 tint, with the inset
  rail" to "error log row wash at the shared /15 tint".
- git-ui `ReviewFilesPane.vue:285` `border-l-2 border-l-warn` accent: drop (the pane divider is the
  parent's `border-border`).
- `rounded-kira` to `rounded-kira-sm` on rows and inset blocks: `AdeReposPage.vue:431`,
  `AdeAllSessionRow.vue:49`, `AdeBranchPanel.vue:510/550`, `AdeRunOutcome.vue:112`. Pane frames
  (`AdeNeedsPage` root and siblings) stay `rounded-kira`.
- Keep (exceptions, named in C12 and in U132 exempt column): plan board `border-l-3`
  (`/ade/v2/plan/`, `AdeShelfRow.vue` task colour rail); glyph outlines `border-2`
  (`AdeActivityIcon.vue` ring, `AdeTaskPanel.vue` and `AdeBranchPanel.vue` panel-dot swatches,
  `AdeStepNode.vue` Vue Flow handle). Quote `border-l-2` (`ReviewThread.vue`,
  `AdeWorkflowReviewPanel.vue`, `AdeArchivedPanel.vue`) and plan-board connector strokes
  (`AdeBranchRow.vue`) stay.

### C10 `chore(lint): guards U127-U132 and U88 off-scale steps`

`scripts/check-ui-primitives.sh`, header comment after the Part 8 line:
`# Guards U127-U132 and U88 steps 5-12: docs/v0.3.17/plans/P672-part9.md.`

Rows after U126 (all land with zero unexempted offenders; no TEMPORARY entry exists or is added):

```
U127@@v@@@@EmptyState media holding a CodiconIcon or Spinner (icon prop is muted; loading prop for a spinner, RE.2)@@(?s)<EmptyState\b(?&t)(?<!/)>(?:(?!</EmptyState>).)*?<template\s+#media>\s*<(?:CodiconIcon|Spinner)\b
U128@@vt@@ModuleStart.vue|MobilePairingDialog.vue|PairScreen.vue@@text-kira-xl off the landing hero (detail values are text-kira-lg, RTy.1)@@${TOK}text-kira-xl(?![\w-])
U129@@vt@@@@arbitrary shadow (shadows come from the elevation tiers, RBo.3)@@${TOK}!?shadow-\[
U130@@v@@SectionHeading.vue|PanelHeader.vue|DetailSection.vue|VariableRow.vue|VariableHistoryMenu.vue|MobilePairingDialog.vue|PairScreen.vue@@hand case or tracking outside header primitives (RTy.3)@@${TOK}(?:uppercase|normal-case|tracking-(?:tighter|tight|normal|wide|wider|widest))(?![\w-])
U131@@v@@@@EmptyState without icon or loading (RE.2)@@<EmptyState\b(?!(?&t)\s(?::?icon|loading)(?=[\s=/>]))
U132@@vt@@/ade/v2/plan/|AdeShelfRow.vue|AdeActivityIcon.vue|AdeTaskPanel.vue|AdeBranchPanel.vue|AdeStepNode.vue@@border-l-3 or border-2 off the plan board and glyph outlines (RBo.2)@@${TOK}!?(?:border-l-3|border-2)(?![\w-])
```

U88 regex: `(?:0\.25|1\.25|1\.75|2\.5|3\.5)` to `(?:0\.25|1\.25|1\.75|2\.5|3\.5|5|6|7|8|10|12)`,
message to "off-scale density (scale is 0.5, 1, 1.5, 2, 3, 4)" unchanged; exempt column
`EmptyState.vue|PairScreen.vue` (theme `p-6` inset, pairing hero `py-8`).

Dry run after C9, then adjust before committing:

- Each built-in exempt name must exempt a real hit, else the script fails "stale built-in
  exemption": drop names that match nothing (e.g. if `components/ui/` files are outside the scan or
  a listed file has no hit).
- U129 with `check-theme-classes.sh` overlap: that script bans stock `shadow-(sm|...)`; U129 bans
  only bracket values. No overlap.
- U130 may hit `components/ui/` primitives (badge, card title): if the scan covers them, add the
  hitting primitive file names to the exempt column (header primitives by definition), never a
  PERMANENT line.
- U131: `QueryState.vue` and other generic hosts pass `:icon` through; if one renders `EmptyState`
  with a bound `v-bind` object, give it an explicit `:icon`.
- If a dry run shows an unexpected hit, fix it in a follow-up `refactor` commit before C10, never
  a TEMPORARY or PERMANENT line.

Probe each new row by hand (scratch edit, run, see the guard name the file, revert).

Rejected guards: icon size 12 vs 13 (host role, not regex-visible); pane root `p-4` (regex cannot
tell a pane root from a dialog body); failure EmptyState title from an error variable (dynamic
text). C12 records the rules in prose.

### C11 `test(ui): P672 Part 9 state assertions`

Only if the targeted run needs edits not already made in C1-C9 (spec failures from a fix land in
the fix commit when found then). Plus one assertion per new shape, in existing specs:

- `consistency.spec.ts` (the `query-error` test ~line 287): `query-error` holds
  `[data-slot="empty-icon"] svg.icon[width="24"]` and no `.text-error` descendant.
- `host-view.spec.ts` failure test (~line 134): `host-view-failure` holds
  `[data-slot="empty-title"]` with "Couldn't load"; `host-view-retry` visible.
- `studio/docker-module.spec.ts`: `docker-stats-cpu` class does not match `/text-kira-xl/`
  (stats tab, running container) and a detail action button `svg.icon` has `width="13"`.

### C12 `docs: architecture for P672 Part 9`

`docs/ARCHITECTURE.md`:

- line 785 "Loading uses Spinner; Skeleton only where the layout is known." append "A pane loading
  state is `EmptyState loading`."
- line 1379 EmptyState: "and media slot for a custom glyph" to "and media slot only for a content
  illustration; `loading` swaps the icon for a Spinner. Every EmptyState has icon or loading
  (U131); media never holds a CodiconIcon or Spinner (U127)."
- line 1464 state line: append "Failure: icon warning, title `common.state.loadFailed.title` or the
  view's failure key, error text as description, Retry toolbar. A block Skeleton may fill a
  fixed-height chart host."
- line 1466 type line: append "xl only ModuleStart and pairing codes (U128); `leading-normal` only
  on prose, `leading-none` on icon-aligned lines; case and tracking only in SectionHeading,
  PanelHeader and DetailSection (their action slots reset them) or masked data (U130)."
- line 1467 spacing and icon line: "Icons 13 in buttons, menus, rows, bars; 12 in Badge or inline
  text" to "Icons 13 in kira buttons, menus, rows, bars; 12 in Badge, chip, inline text and status
  or inline size buttons; 16 DetailHeader, card tile or title bar; 24 empty state." Append "Steps 5
  and up fail U88 except EmptyState inset and pairing hero; indent paddings stay."
- line 1468 radii line: append "border-l-3 and border-2 only on the plan board and glyph outlines
  (U132); no arbitrary shadow (U129)."
- line 1435 `U83-U126` to `U83-U132`.
- line 1437 "plus a dated TEMPORARY block for the P672 guards" removed; PERMANENT count from the
  awk recount (expect 50).
- line 1470 to "Guards U110-U132 in check-ui-primitives.sh hold these rules with zero offenders; a
  real exception is a built-in exempt name or a reasoned PERMANENT line."

### C13 `docs: P672 Part 9 done`

`docs/v0.3.17/SPEC.md` Part 9 row: Status Done; Result in the Part 8 format: commits C1-C12 plus
any `test:`/`fix:` commits; the 5 U115 lines and the TEMPORARY mechanism removed (no guard keeps a
TEMPORARY entry); M31 dropped per D3; counts per group (media 11, icons added 16, hand states 11,
skeletons 3, button icons 25 plus 5 title bar, xl 5, leading 5, pane roots 15, borders, shadows,
radii); guards U127-U132 and U88 steps; deliberate visual changes (muted failure icons, docker stat
size, no amber step rail, review title icons 16); files touched; specs run; visual baselines for CI.
Last commit of the phase.

## Test split

UI-only phase: no IPC, Go or flow change, so no flow-test half (the CLAUDE.md split applies to
features; this is a restyle with unchanged behaviour). No new unit test (no complex logic). Guards
U127-U132, U88 and U115 hold the rules.

Specs touching changed hosts (selectors stay; must pass), under `apps/kira-space/tests/ui/`:
consistency, copy-wording, copy-coverage, copy-coverage-git, repo-graph-failures, repo-workspace,
host-view, issue-view, issue-view-fetch, github-pr-open, ade-v2-issue-tracker, ade-v2-run,
ade-v2-links-strip, ade-v2-needs, ade-v2-review*, ade-v2-workflows, ade-v2-workflow-steps,
ade-v2-panel-tabs, ade-v2-task-panel, ade-v2-plan*, ade-v2-sessions*, ade-v2-notes,
ade-v2-settings*, ade-v2-forward*, memory-*, automations-*, module-state, ui-leftovers; `studio/`:
kubernetes-module, kubernetes-tabs, docker-module, consistency-docker-automations, interaction,
cell-editor, console*, browse*, keyvalue*, stream*, documents*, kibana*, proof*, api-variables*;
mobile specs for NeedsScreen and PlanScreen.

Targeted run near phase end (one pass, from `apps/kira-space`, after `bun run build:test`; run
`bun run build:mobile` before the mobile specs): the list above. Implementer first greps `tests/`
for every changed file name and testid (`query-error`, `git-blocked-panel`, `kube-unavailable`,
`docker-detail-`, `docker-stats-`, `docker-engine-`, `host-view-`, `boot-error`, `boot-retry`,
`console-no-matching-rows`, `no-rows`, `fk-preview-empty`, `cell-editor-decoded-empty`,
`ade-review-empty`, `ade-needs-empty`, `ade-links-skeleton`, `ade-task-issue-loading`,
`ade-panel-forward-preview`, `border-l-tone-amber`, `empty-state`, `text-kira-xl`) and adds any spec
found.

Visual baselines for CI to re-record (PR CI owns `visual`): docker detail, engine overview, stats,
unavailable; kubernetes unavailable and lists; git blocked panel; query error states; host view
loading and failure; git-ui boot error and review; ADE workflows and steps lists, settings, leases,
context, needs, review window, links strip, plan (day band, history icons); console no-rows;
browse empty; memory panel; repo file and diff views; studio documents and op log; plus any shot CI
flags. Docs screenshots under `tests/screenshots/` are not CI baselines.

Sandbox lessons for spec edits:

- Playwright click on a menu item inside Monaco needs `locator.hover()` first.
- reka radio and toggle arrow presses need `keyboard.press(key, { delay: 40 })`
  (`tests/ui/support/radioKeys.ts`).
- N17 (`scripts/check-conventions.ts`) forbids the `dialog` variant on a delete/discard label
  without a `conventions-allow N17` marker. git-ui boot Retry moves from `dialog` to `toolbar`; no
  delete label is added.
- N10 caps `.vue` files at 600 lines: see the budget list in Current state; recount after
  `bun run format` on every edge file.
- U91 exemptions go stale when their pattern vanishes: after each commit read the guard output for
  stale exemption or stale built-in exemption (C2 and C3 remove `Alert`, `Spinner` and hand
  patterns that some existing exempt column may name; C9 changes the U26 file's pattern but the
  wash still matches).
- A timeout under load gets a lone rerun first.
- `EmptyState loading` stays invisible for `SPINNER_DELAY`: a spec asserting visibility of a loading
  state immediately needs the default expect timeout, not `timeout: 0`.

## Risks

- Muted failure icons read calmer than the red/amber glyphs; RE.2 asks for it. Visual baselines
  change in docker, kubernetes, git blocked and query error.
- HostViewFrame failure moves from an inline Alert to a centred pane state; specs reading
  `host-view-failure-reason` must still find the text (keep the testid on a child or update the
  four specs in the same commit).
- Dropping the amber step rail leaves selection to `rowVariants` `bg-select`; Part 7 kept the rail
  on purpose for contrast with the graph. Accepted by RBo.2; if the user objects, revert the two
  `rowClass` lines and add a U132 exempt name.
- Edge files at 599/600 lines (BrowseView, DocumentView, ConsoleResultGrid, git-ui ReviewView)
  can fail N10 after formatting; recount before commit.
- U130 and U132 regexes may hit primitives under `components/ui/` if the scan covers them; dry run
  decides the exempt column. A wrong exempt name fails as stale, so the column self-corrects.
- Codicon names picked for new icons may not exist; an unknown name renders an empty box silently.
  Check each against the shipped codicon list.

## Out of scope

- M31 RE.4 toasts: D3 keeps toasts app-wide as they are.
- Hand progress bars (`UsageBar`, `EngineOverview.vue:222`) onto theme `progress`: not in the SPEC
  row.
- Plan board borders, colour rails and connector strokes; glyph outlines; indent paddings
  (`pl-5`/`pl-6`/`pl-8`).
- `LogLines` row leading (virtual row height), AutocompleteField mirror, CellEditor bar leading.
- Any i18n key, behaviour, store, query or IPC change.
