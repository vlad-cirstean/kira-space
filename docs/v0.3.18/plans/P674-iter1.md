# P674 Part 2: UI uniformity audit, iteration 1

Auditor ledger, wave plan, stream table and guard list for iteration 1. Master plan: `P674.md`.
Implementers tick each entry `done <sha>` as it lands.

## Base and tool report

- Base commit: `bc09408cb` (`chapter/v0.3.18`).
- `bun run audit:ui --out <scratchpad>/audit.md`, default thresholds, 5.9 s. Scan set 1587 files (713
  `.vue`, 874 `.ts`). Report is not committed; rerun to reproduce.
- Clusters per channel: C1 53, C2 201, C3 364, C4 20, C5 31, C6 119, C7 24 (812 total).
- Triaged: every C1, C4, C5, C6, C7 cluster; every C2 and C3 cluster with module spread >= 2, by
  family (C2, C3 are dominated by layout utility sets, import lists and test fixtures).
- Hand sweep (channel `S`): every area in master plan step 3, like against like. Sweep findings:
  F1.1 (Studio view heads vs git and ADE heads), F1.5 (secrets sections), F1.6 (log views),
  F1.9 (settings field reset), F1.10 (ADE YAML panes), F1.11 (toasts).
- Outcome: 17 findings (11 EXTRACT, 3 MERGE, 3 MIGRATE), 4 PARK, rest DISMISS (list at end).
  Guards U162-U170 (9 lines), 6 new PERMANENT lines (48 to 54).

## Leads from the orchestrator

1. Two `RelativeTime.vue`: confirmed. F1.2 MERGE.
2. Relative time in two places: confirmed, and a third copy (cell editor `timestamp.ts`
   `relativeTime`, `Intl.RelativeTimeFormat`). F1.3 MERGE.
3. `VirtualList` vs `useVirtualRows`: confirmed. `packages/docker-ui/src/components/VirtualList.vue`
   is a generic keyboard listbox used only by docker; three hand copies of the same shape sit in
   Studio and repo. F1.4 promotes it to workbench; six named holdouts stay (PERMANENT).

## Concurrent streams

- P675 is Go only: no overlap.
- P678 Part 1 (not in tree at base) adds `KeptVariablesPanel`, approval-card additions,
  `KeptVarsService`, a Settings view, and optional props on `ApprovalPairs` and `HighlightedText`.
  This iteration never edits `ApprovalPairs` or `HighlightedText`. Overlap: `ApprovalItemCard.vue`
  (F1.15, one import plus one call swap, wave 1) and `en.json` (key removals here, key additions
  there; additive hunks). Whichever lands second rebases. P678 code is new code for Part 3's scan.
- P676, P677, P678 Part 2, P679, P680 add API and DB MCP UI later; Part 3 and the final scan rescan
  them. Out of scope here.

## Ledger

Paths drop `apps/kira-space/frontend/` for `src/` and `mobile/`; `packages/` paths are repo
relative. Specs: grep the testids under `apps/kira-space/tests/ui/` and `tests/mobile/`; named
files are the known hits.

### F1.1 [DUP] EXTRACT: view head

- Sites (hand-built head: dot, kind icon, title with dim prefix, badges, actions, band):
  `src/studio/api/EnvironmentsView.vue:125`, `src/studio/api/VariableSetView.vue:105`,
  `src/studio/views/browse/BrowseView.vue:341`, `src/studio/views/grid/DataView.vue:244`,
  `src/studio/views/documents/DocumentView.vue:350`, `src/studio/views/console/ConsoleView.vue:233`,
  `src/studio/views/console/kibana/KibanaDiscoverView.vue:187`,
  `src/studio/views/httprequest/HttpRequestView.vue:303`,
  `src/studio/views/grpcrequest/GrpcRequestView.vue:272`, `src/studio/views/stream/StreamView.vue:78`,
  `src/studio/views/definition/DefinitionView.vue:258`,
  `src/studio/views/shared/keyvalue/KeyValuePane.vue:149`,
  `src/ade/v2/repos/AdeReposPage.vue:319`, `packages/git-ui/src/components/GitViewHead.vue:34`.
- Drift found: title `truncate` (Studio, git) vs `TruncatedText` (ADE); icon muted (git, ADE) vs
  not (Studio); stray space between prefix and name (`KeyValuePane.vue:157`); empty `ml-auto` span
  rendered with no actions (Studio). P104's "inlined" note predates three modules sharing the head.
- Shared home: `packages/workbench/src/components/ViewHead.vue` (consumers in Studio, ADE, git-ui;
  git-ui already imports `@workbench`).
- Change: props `color?: string` (dot when defined; band always via `colorMarkClass('band', color)`),
  `icon: string`, `iconClass?` (default `text-muted-foreground`), `prefix?: string`,
  `title?: string`, `testid?`, `titleTestid?`, `dotTestid?`, `bandTestid?`. Slots: `lead` (wrapped
  in the `size-4` box; Studio passes `EngineIcon`), `title` (replaces the title, Kibana pickers),
  default (badges), `actions` (wrapper `ml-auto flex shrink-0 items-center gap-1` only when given).
  Title renders `TruncatedText` with the prefix span inside. Every existing testid on root, dot,
  band and title stays, passed through props. Root is `ViewToolbar`. `GitViewHead` keeps its
  props and renders `ViewHead`. Update the P104 comment in each migrated view (delete it).
- Guard: U162 `v`, exempt `ViewHead.vue|ApprovalItemCard.vue` (approval card band is a card edge,
  not a view head), regex `colorMarkClass\('band'`.
- Visible change: yes (Studio kind icon muted when no `iconClass`; overflow tooltip on head titles;
  KeyValuePane prefix space gone).
- Specs: `api-ui-consistency.spec.ts`, `tabs.spec.ts`, `repo-commit-meta.spec.ts`,
  `ade-repos.spec.ts`, plus specs hitting each view's title testid (`environments-target`,
  `keyvalue-target`, `data-*`, `http-*`, `grpc-*`, `console-*`, `stream-*`, `definition-*`).
- Wave 1 creates; wave 2 A (12 Studio), B (`AdeReposPage`, `GitViewHead`).
- Wave 1: done 5de5f67c7 (ViewHead created; migrations in wave 2).

### F1.2 [DUP] MERGE: RelativeTime component

- Sites: `src/components/RelativeTime.vue` (reactive `useRelativeTime`, no tooltip; used by
  `src/workbench/settings/MobileAccessPane.vue:25,324`, `mobile/screens/LaterShelf.vue:10,87`) and
  `src/hostview/RelativeTime.vue` (static `formatRelative` in `TextTooltip` with `formatDateTime`;
  used by `src/hostview/IssueSidebar.vue:14,73,77`, `src/hostview/IssueComments.vue:11,61`,
  `src/ade/v2/review/AdeReviewRemoved.vue:16,100`). Standalone instants rendered as bare text in a
  template: `src/studio/api/VariableHistoryMenu.vue:66`,
  `src/studio/views/httprequest/ResponseHistoryList.vue:228`,
  `src/studio/views/grpcrequest/CallHistoryList.vue:180`,
  `src/studio/workbench/panels/StudioStart.vue:132`, `src/hostview/HostViewConversation.vue:109`,
  `src/hostview/IssueAttachments.vue:55`, `src/ade/v2/details/AdeProofEntry.vue:64`.
- Shared home: `packages/workbench/src/components/RelativeTime.vue`.
- Change: prop `at: number | string | Date`; text from `useRelativeTime(at)` (ticks, handles future
  instants); `TextTooltip` shows `formatDateTime(at)`; `<time :datetime>` root. Delete both app
  copies; wave 1 swaps the five importers. Wave 2 replaces the seven mustache sites (keep each
  site's class and leading `· `). String interpolations (`formatRelative` inside a label, a menu
  name, a `matchesQuery` haystack, a `:label` prop) stay on `formatRelative`. Rows sharing one
  clock (`AdeSessionsRow`, `AgentUsageItem`, `branchOptions.ts`) keep `formatRelative(at, now)`.
- Guard: U163 `v`, exempt none, regex `\{\{\s*formatRelative\(`. The duplicate basename is caught by
  audit C4 on the next scan; TypeScript fails on the deleted files.
- Visible change: yes (absolute tooltip on mobile, settings and Studio row times; times tick).
- Specs: new `apps/kira-space/tests/ui/relative-time.spec.ts` (see Tests); existing
  `http-variables.spec.ts`, `api-secret-reveal-isolation.spec.ts`,
  `grpc-stream-terminal-race.spec.ts`, mobile Later specs, hostview issue specs.
- Wave 1 (component, five import swaps, deletions); wave 2 A (4), B (3).
- Wave 1: done d4f4d18dd (component, five importers, deletions); mustache sites in wave 2.

### F1.3 [DRIFT] MERGE: one relative-time vocabulary

- Sites: `packages/theme/src/format.ts:97` `formatRelative` (own floor math, keys
  `format.relative.minutes|hours|days`, "5m ago", no future); `packages/workbench/src/behaviour/format.ts:47-60`
  `relativeOptions`, `formatRelativeAt`, `useRelativeTime` (VueUse, keys `past`, `future`,
  `unitMinute|Hour|Day`, round); `src/studio/views/shared/celleditor/timestamp.ts:269-286`
  `relativeTime` (`Intl.RelativeTimeFormat`, "3 days ago", years and months).
- Shared home: `packages/theme/src/format.ts`.
- Change: export `relativeTimeOptions()` (VueUse `FormatTimeAgoOptions`: `rounding: 'floor'`,
  units minute, hour, day (< 30 d), month (< 365 d), year; messages `justNow`, `past`, `future`,
  `unitMinute|Hour|Day`, month `format.age.months`, year `format.age.years`). `formatRelative(at,
  now = Date.now())` becomes `formatTimeAgo(new Date(toMillis(at)), relativeTimeOptions(), now)`.
  `useRelativeTime` uses the same options. Delete `relativeOptions`, `formatRelativeAt` (caller
  `src/ade/v2/sessions/AdeSessionsRow.vue` moves to `formatRelative`, wave 1). Delete
  `timestamp.ts` `relativeTime` and `RELATIVE_STEPS`; its caller uses `formatRelative(date,
  now.getTime())` (wave 2 A). Remove catalog keys `format.relative.minutes`, `.hours`, `.days`.
  Update `packages/theme/src/format.spec.ts` cases (existing unit test).
- Guard: none. Deleted keys and functions fail typecheck (`MessageKey` is typed); ARCHITECTURE rule
  line states the one vocabulary.
- Visible change: yes (future instants read "in 5m"; ages over 30 days read "4mo ago", "1y ago";
  cell editor relative line becomes terse).
- Specs: `format.spec.ts`; cell editor timestamp specs (grep `celleditor`, `timestamp`).
- Wave 1; wave 2 A (`timestamp.ts`).
- Wave 1: done f11055965; `timestamp.ts` in wave 2.

### F1.4 [DUP] EXTRACT (promote): VirtualList

- Sites: `packages/docker-ui/src/components/VirtualList.vue` (generic: `useVirtualRows` +
  `useListNavigation` + per-row handler cache, `role="listbox"`, row slot); docker consumers
  `NetworkList.vue`, `ImageList.vue`, `VolumeList.vue`, `ContainerList.vue` (all under
  `packages/docker-ui/src/components/`). Hand copies of the same shape:
  `src/repo/RepoSearchView.vue:38-192`, `src/studio/kubernetes/components/ResourceTable.vue:123-213`,
  `src/studio/views/browse/BrowseView.vue:272-537`.
- Shared home: `packages/workbench/src/components/VirtualList.vue` (`git mv`).
- Change: `testid` becomes required (drop default `'docker-list'`; docker callers pass it);
  optional `v-model:activeKey` (BrowseView binds its store key, ResourceTable its ref); internal
  ref when unbound. Wave 1 moves the file, swaps the four docker imports, updates the
  `side_panels` entry in `scripts/check-ui-primitives.sh` to the new path (the script exits 2 on a
  missing file). Wave 2 migrates the three hand copies to the row slot, keeping their testids and
  row markup. Holdouts, each a named requirement (PERMANENT, below): `AdeReposPage.vue` grouped
  `windowRows` with group heads; `KeyValueRowsTable.vue` custom `visibleRows` windowing;
  `StreamRowsTable.vue` frozen index column; `DocumentView.vue` variable-height rows with tooltip
  delegation on the scroller; `ConsoleResultGrid.vue` two virtualizers over mapped indices;
  `ContainerTable.vue` header row inside the listbox sized by container queries.
- Guard: U164 `v`, exempt `VirtualList.vue`, regex
  `(?s)\A(?=.*useVirtualRows\()(?=.*useListNavigation\b[<(])(?=.*role="listbox")`; base offenders
  are exactly the 3 migrated files, the 6 holdouts and VirtualList itself.
- Visible change: no.
- Specs: `list-keyboard.spec.ts`, `repo-workspace.spec.ts`, `panel-find.spec.ts`,
  `browse-folder-nav.spec.ts`, `kubernetes-tabs.spec.ts`, `docker-module.spec.ts`,
  `copy-wording-docker-automations.spec.ts`, `copy-coverage-docker-automations.spec.ts`.
- Wave 1 (move, docker imports, side_panels); wave 2 A (BrowseView, ResourceTable), B
  (RepoSearchView).
- Wave 1: done 0b871ba51; hand copies in wave 2.

### F1.5 [DUP] EXTRACT: config secrets section

- Sites: `src/ade/v2/settings/AdeSettingsSecrets.vue` (175 lines) and `src/studio/api/ApiSecrets.vue`
  (177 lines): same list, set dialog, delete confirm, error alert. Drift: verb Delete with
  `dialog-danger` vs Clear with `dialog`; `p-2` only on ApiSecrets; catalog
  `space.ade.settings.secrets.*` vs `studio.api.config.secrets.*` with the same English.
- Shared home: `packages/workbench/src/config/ConfigSecretsSection.vue`.
- Change: props `secrets: readonly ConfigSecretRow[]` (`{ name: string; isSet: boolean }` plus any
  field either host renders; implementer maps `ApiSecret` and `AgentSecretView` in the host),
  `storage` (the shared storage-availability shape both read), `setSecret(name, value):
  Promise<void>`, `deleteSecret(name): Promise<void>`, `testidPrefix` (keeps `api-secrets*` and
  `ade-settings-secret*`). Slots `title`, `description` for host copy (reference syntax example).
  Verb Delete, `ICON.delete`, confirm tone danger. Row actions follow the toolbar rule. Local error
  ref stays (P682 decides the wider pattern). New keys `common.configSecrets.*` (union of both
  sets, wave 1); hosts' duplicate keys removed in wave 2.
- Guard: none. Removed keys fail typecheck; both hosts render the one component (rule line).
- Visible change: yes (ApiSecrets: Clear becomes Delete; padding aligns).
- Specs: `api-agent-access.spec.ts`, `ade-agent-settings.spec.ts`.
- Wave 1 creates; wave 2 A (ApiSecrets), B (AdeSettingsSecrets).
- Wave 1: done 519a7699b (component, `common.configSecrets.*`; host key swaps for duplicate English); host swaps in wave 2.

### F1.6 [DUP] EXTRACT: log tail model and line rendering

- Sites: `src/studio/kubernetes/components/LogsView.vue:29-30` and
  `packages/docker-ui/src/components/LogsView.vue:26-27` (`MAX_LINES`, `TAILS`); both `tailLabel`
  (kube borrows `docker.logs.tail.option`), append-with-trim, plain text from `parseAnsi`,
  `segmentStyle` (third copy in `packages/workbench/src/components/LogLines.vue:93`), and the same
  `#line` slot (timestamp, segments, `HighlightedText`). Drift: bold `font-bold` vs
  `font-medium`; toolbar order differs.
- Shared home: `packages/workbench/src/lib/logTail.ts`; `LogLines.vue` props.
- Change: `logTail.ts` exports `LOG_MAX_LINES`, `LOG_TAILS`, `tailLabel(n)`, `tailItems()`,
  `appendCapped(lines, next, max = LOG_MAX_LINES)`, `plainText(segments)`. Move key
  `docker.logs.tail.option` to `common.logs.tail.option`. `LogLines` gains optional
  `showTimestamps`, `needle`, `currentLine` and renders the line itself (its own `segmentStyle`
  wins; host copies deleted). Both views drop the `#line` slot. Toolbar order in both: follow,
  timestamps, wrap, tail, then find and copy or clear. Find model stays per host (P684).
- Guard: U165 `vt`, exempt `logTail.ts`, regex `\b(?:MAX_LINES|TAILS)\s*=\s*[\[\d]`.
- Visible change: yes (kube bold weight; toolbar order).
- Specs: `kubernetes-tabs.spec.ts`, `kubernetes-hardening.spec.ts`, `copy-coverage.spec.ts`,
  `docker-module.spec.ts`, `module-state.spec.ts`, `ui-leftovers-docker-automations.spec.ts`.
- Wave 1 (`logTail.ts`, `LogLines`, key move with both call sites' key swap); wave 2 A (kube), C
  (docker).
- Wave 1: done 6b286fb2e (logTail, LogLines, key move, `#line` slot dropped in both views); toolbar order and logTail adoption in wave 2.

### F1.7 [DUP] EXTRACT: save-conflict alert

- Sites (error Alert with Refresh plus Overwrite): `src/views/repo/RepoFileView.vue:238`,
  `src/ade/v2/workflows/AdeWorkflowGraph.vue:421`, `src/ade/v2/workflows/AdeWorkflowYaml.vue:169`,
  `src/ade/v2/workflows/AdeLibraryStepYaml.vue:175`, `src/ade/v2/workflows/AdeLibraryStepForm.vue:108`,
  `src/ade/v2/settings/AdeSettingsPane.vue:105`, `src/ade/v2/context/AdeContextPane.vue:105`,
  `src/repo/RepoFilePane.vue:182`, `src/studio/api/ApiVariablesDialog.vue:303`,
  `src/studio/project/ConnectionFileDialog.vue:374`, `packages/workbench/src/automations/RunFileDialog.vue:411`.
- Shared home: `packages/workbench/src/config/SaveConflictAlert.vue`.
- Change: props `error: string`, `conflict: boolean`, `testidPrefix`; default slot for the
  description; emits `reload`, `overwrite`. Key `space.ade.workflows.conflict.overwrite.label` moves
  to new `common.action.overwrite.label`; `src/studio/state/confirmOverwrite.ts:11` swaps key.
- Guard: U166 `vt`, exempt `SaveConflictAlert.vue|confirmOverwrite.ts` (confirm dialog button, not
  an alert), regex `common\.action\.overwrite\.label`.
- Visible change: no.
- Specs: `ade-v2-workflows.spec.ts`, `ade-v2-workflow-steps.spec.ts`, `repo-config-form.spec.ts`,
  conflict specs (grep `conflict`, `overwrite`).
- Wave 1 creates (and migrates `RunFileDialog`); wave 2 A (2 plus key swap), B (7; the two YAML
  sites go with F1.10).
- Wave 1: done 6cc6dba71 (component, RunFileDialog, key move and all 11 key swaps); component adoption in wave 2.

### F1.8 [DUP] EXTRACT: config form problem alert

- Sites (`common.configForm.problem.status` plus optional Show YAML): `src/repo/RepoFilePane.vue:109`,
  `src/studio/api/BulkVariablesEditor.vue:44`, `src/studio/api/ApiVariablesDialog.vue:200`, `:238`,
  `src/studio/project/ConnectionFileDialog.vue:286`, `packages/workbench/src/automations/RunFileDialog.vue:296`.
- Shared home: `packages/workbench/src/config/ConfigFormProblemAlert.vue` (beside
  `ConfigFileProblem.vue`).
- Change: props `problem: { line?: number; message: string }`, `testid`, `showYaml?: boolean`; emits
  `show-yaml`.
- Guard: U167 `vt`, exempt `ConfigFormProblemAlert.vue`, regex `common\.configForm\.problem\.status`.
- Visible change: no.
- Specs: `repo-config-form.spec.ts`, `http-variables.spec.ts`, connection dialog specs.
- Wave 1 creates (and migrates `RunFileDialog`); wave 2 A (4), B (1).
- Wave 1: done 6cc6dba71 (component, RunFileDialog); adoption in wave 2.

### F1.9 [DUP] EXTRACT: reset-to-default button

- Sites (hand `TooltipIconButton icon="discard"` with `common.action.resetToDefault.label`):
  `src/workbench/settings/ExperimentalPane.vue:42`, `MemoryPane.vue:35`, `CapturePane.vue:69,81`,
  `DateFormatField.vue:45`, `UpdatesPane.vue:45`, `AgentPane.vue:96,146,170,200,239`,
  `GitPane.vue:110,133,164,190`, `AppearancePane.vue:101,129`, `AdvancedPane.vue:105,119,147`
  (all `src/workbench/settings/`); `src/studio/workbench/settings/ApiPane.vue:116,141,172,208,234,250,292`,
  `CachePane.vue:72`, `DataPane.vue:38`; `src/studio/project/connection/ConnectionAdvancedTab.vue:135`;
  `src/ade/v2/dialog/AdeAgentDialog.vue:262`; `src/ade/v2/run/AdeRunDialog.vue:253`;
  `packages/workbench/src/settings/fields/WordWrapField.vue:43`, `NotifyPromptsField.vue:54`,
  `LogLevelField.vue:47`, `FontFamilyField.vue:48`, `RowDensityField.vue:42`, `FontSizeField.vue:50`;
  `packages/workbench/src/llmPrompts/PromptArgumentField.vue:40`. Literal `discard` bypasses
  `ICON.reset` at every site.
- Shared home: `packages/theme/src/components/ResetToDefaultButton.vue` (presentational).
- Change: prop `atDefault?: boolean` (drives `disabled` and `disabled-trigger`); renders
  `TooltipIconButton` with `ICON.reset` and the reset label; attrs fall through (testid,
  `@click`, class). Sites that hide instead (`v-if="edited"`) keep their `v-if`.
- Guard: U168 `vt`, exempt `ResetToDefaultButton.vue|PromptDetail.vue` (labelled text Button in a
  detail head, not a field reset), regex `common\.action\.resetToDefault\.label`.
- Visible change: no.
- Specs: `settings-memory.spec.ts`, `settings-updates.spec.ts`, `agent-provider.spec.ts`, other
  `settings-reset-*` hits, `settings-llm-prompts.spec.ts`, `ade-v2-dialogs.spec.ts`.
- Wave 1 creates and migrates the 7 workbench sites; wave 2 A (10), B (2), C (20).
- Wave 1: done 619c786b2 (component, 7 workbench sites).

### F1.10 [DUP] EXTRACT: ADE YAML pane

- Sites: `src/ade/v2/workflows/AdeLibraryStepYaml.vue` and `AdeWorkflowYaml.vue`: byte-identical
  but for query hooks, entry type, testid prefix and one comment.
- Shared home: `src/ade/v2/workflows/AdeYamlPane.vue` (module-local, both consumers in ADE).
- Change: props `fileName: string`, `kind: 'step' | 'workflow'` (picks `useStepYaml`/`useWorkflowYaml`
  and validate, save), `testidPrefix` (`ade-step-yaml`, `ade-wf-yaml`). Uses `SaveConflictAlert`.
  Wave 2 B deletes both files and points their parents at `AdeYamlPane`.
- Guard: none (module-local; deleted files fail the build).
- Visible change: no.
- Specs: `ade-v2-workflows.spec.ts`, `ade-v2-workflow-steps.spec.ts`.
- Wave 1 creates; wave 2 B.
- Wave 1: done 17b2d4937, 969d2e7d1 (pane, both parents repointed, both old files deleted).

### F1.11 [DUP] EXTRACT: pending toast

- Sites: `packages/workbench/src/approvals/ApprovalsToast.vue:20`,
  `packages/workbench/src/prompts/PromptsToast.vue:12`: same floating surface, text column and
  Show button. Drift: `min-w-0` and `truncate` only in PromptsToast.
- Shared home: `packages/workbench/src/components/PendingToast.vue`.
- Change: props `testid`, `showTestid`; default slot for the text lines; emits `show`. Column
  always `min-w-0`.
- Guard: none (two consumers, both migrated; a third toast shows in audit C1).
- Visible change: no.
- Specs: `approvals.spec.ts`, `toast-placement.spec.ts`, `prompts.spec.ts`, `prompts-typing.spec.ts`.
- Wave 1 (both sites).
- Wave 1: done 8f03a9e69.

### F1.12 [DUP] EXTRACT: drop indicator

- Sites: `src/studio/api/CollectionRow.vue:144`, `src/studio/kubernetes/components/ClusterList.vue:196`,
  `src/studio/project/TreeRow.vue:170`.
- Shared home: `packages/theme/src/lib/frames.ts` `dropIndicator`
  (`'pointer-events-none absolute inset-x-0 h-0.5 bg-focus'`).
- Guard: U169 `vt`, exempt `frames.ts`, regex
  `['"](?=[^'"]*${TOK}h-0\.5(?![\w-]))(?=[^'"]*${TOK}bg-focus(?![\w-]))`.
- Visible change: no.
- Specs: `connection-order.spec.ts`, `api-order.spec.ts`, `kubernetes-order.spec.ts`.
- Wave 1 adds the constant; wave 2 A (3).
- Wave 1: done e6cf0e865 (constant); sites in wave 2.

### F1.13 [DUP] EXTRACT: window tab strip

- Sites: `src/ade/v2/review/AdeReviewWindow.vue:135`, `src/openedfiles/OpenedFilesWindow.vue:25`,
  `src/studio/proof/ProofWindow.vue:115`, `packages/workbench/src/components/WorkbenchShell.vue:97`.
- Shared home: `packages/theme/src/lib/frames.ts` `windowTabStrip`
  (`'h-tabbar min-h-0 shrink-0 overflow-hidden bg-chrome'`).
- Guard: U170 `vt`, exempt `frames.ts`, regex
  `['"](?=[^'"]*${TOK}h-tabbar(?![\w-]))(?=[^'"]*${TOK}min-h-0(?![\w-]))(?=[^'"]*${TOK}bg-chrome(?![\w-]))`.
  `AdeShell.vue` (`min-h-tabbar`, no `min-h-0`) and `ResourceTabs.vue` stay out by design.
- Visible change: no.
- Specs: window specs (grep `opened-files`, `ade-review-window`, `proof-window`), `tabs.spec.ts`.
- Wave 1 (constant, WorkbenchShell); wave 2 A (1), B (2).
- Wave 1: done e6cf0e865 (constant, WorkbenchShell); sites in wave 2.

### F1.14 [DRIFT] MIGRATE: icon per concept

- Sites and change:
  - remove (takes out of a list; `space.repo.removeConfirm`, automations `removeConfirm` "will no
    longer appear"): `src/ade/v2/repos/AdeReposPage.vue:363` and
    `packages/workbench/src/automations/panelMenus.ts:121` `trash` to `ICON.remove`.
  - cancel a running op: `packages/git-ui/src/components/AppToolbar.vue:425` `close` to `ICON.stop`
    (as `OpLogPanel`, `LoadMoreButton`, `DistillProgress`, `ImportJobDetail`).
  - resume: new `ICON.resume = 'debug-continue'`; `src/ade/v2/panel/AdeStepBlock.vue:346`,
    `src/ade/v2/review/AdeWorkflowReviewPanel.vue:348`,
    `packages/workbench/src/automations/runs/RunOutcomeBlock.vue:103` (`play`), and
    `src/ade/v2/sessions/useSessionMenu.ts:86`, `packages/workbench/src/memory/import/ImportJobDetail.vue:150`
    (literal) to `ICON.resume`.
  - take over: new `ICON.takeOver = 'terminal'`; `src/ade/v2/sessions/useSessionMenu.ts:96`
    (`debug-continue`), `src/ade/v2/panel/AdeStepBlock.vue:376`,
    `src/ade/v2/sessions/AdeSessionActions.vue:47` to `ICON.takeOver`.
  - open in new tab: new `ICON.openInNewTab = 'empty-window'`;
    `src/studio/views/grid/FkPreviewPopover.vue:119` (`arrow-right`),
    `src/studio/views/console/kibana/DiscoverSavedMenu.vue:173` to `ICON.openInNewTab`.
  - push (same `git.toolbar.push.tooltip` label): `packages/git-ui/src/lib/rowMenuModel.ts:218`
    `cloud` to `repo-push`.
- Shared home: `packages/theme/src/icons.ts`, ARCHITECTURE ICON table.
- Guard: none (semantic: the same glyph is right for other concepts). ICON table rows added.
- Visible change: yes (every changed glyph above).
- Specs: `ade-repos.spec.ts`, automations panel specs, git toolbar specs, `ade-v2-*` session and
  step specs, memory import specs, `data-view` FK preview specs, Kibana saved specs.
- Wave 1 (`icons.ts`, workbench sites `panelMenus.ts`, `RunOutcomeBlock.vue`,
  `ImportJobDetail.vue`); wave 2 A (2), B (7).
- Wave 1: done 336ba652a (icons, workbench sites); rest in wave 2.

### F1.15 [DUP] MERGE: countdown clock into formatElapsed

- Sites: `packages/workbench/src/approvals/useCountdown.ts:6` `formatClock(seconds)` (m:ss) duplicates
  `packages/theme/src/format.ts:82` `formatElapsed(ms)`; callers
  `packages/workbench/src/approvals/ApprovalsToast.vue:42`, `ApprovalItemCard.vue:54`.
- Change: callers use `formatElapsed(seconds * 1000)`; delete `formatClock`.
- Guard: none (deleted function).
- Visible change: no under one hour; h:mm:ss above (expiry beyond an hour was "75:00").
- Specs: `approvals.spec.ts`, `toast-placement.spec.ts`.
- Wave 1.
- Wave 1: done 8f03a9e69.

### F1.16 [DRIFT] MIGRATE: displayed percentages

- Sites: `src/studio/workbench/settings/CachePane.vue:43` (`Math.round(...)%`),
  `packages/docker-ui/src/components/EngineOverview.vue:244` (`${engine.cpus * 100}%`).
- Change: `formatPercent(...)`. Style widths (`TimelinePane`, `UsageBar`, `EngineOverview:225-227`,
  `AdeWorkflowGraph:313`, `Progress`) are layout, not display text.
- Guard: none (layout percentages share the shape); `format.ts` rule already covers it.
- Visible change: yes (one decimal under 100%).
- Specs: cache settings specs, `docker-module.spec.ts`.
- Wave 2 A (1), C (1).

### F1.17 [DRIFT] MIGRATE: chart missing value

- Site: `src/studio/proof/chartOption.ts:214` en dash `'–'` for a null value.
- Change: `displayValue(null)` (em dash, the one placeholder).
- Guard: none (single site).
- Visible change: yes (chart tooltip null reads `—`).
- Specs: proof chart specs (grep `proof-chart`).
- Wave 2 A.

## PARK (orchestrator adds SPEC rows, numbers fixed here)

- P682 Async action error state. ~70 files keep `pending`/`error` refs around an awaited call
  (`.value = errorText(` in components, stores, mobile; C7 `e12948b16c`), beside `useAsyncAction`
  (61 callers) and TanStack `useMutation().error`. Which owns a write's error (local ref,
  `useAsyncAction`, mutation state) is a design decision across every module and store.
- P683 Studio view-store script dedup. `search.ts`, `page.ts`, `state.ts` repeat across
  grid, documents, keyvalue, stream, and grpc/http `state.ts` (C3 families). Per-kind store design,
  not UI.
- P684 One log find model. Kube logs filter lines (`SearchField`) plus `CopyLogButton`; docker logs
  find in place (`FindBar`) plus Clear. Picking one model changes behaviour in one module.
- P685 Config-file dialog shell. `ApiVariablesDialog`, `ConnectionFileDialog`, `RunFileDialog`
  repeat draft wiring (`useConfigFormDraft`, save glue, secret error merge), head toolbar, section
  tabs and footer (C1 `2f1a55c903`, C3 `8c58cf4a56`, `df06574d11`, `f20c31e2ec`, `008bc386e0`,
  `c27e000bf7`, `34281be21f`). F1.7 and F1.8 take the alerts now; the shell API (sections, secret
  errors, per-kind forms) is a design decision over three save flows.

## Waves and streams

Wave-2 site count: A 45, B 28, C 22 (95 > 40): three streams.

### Wave 1 (one Sonnet, sequential, commits per finding group)

1. `packages/theme`: `format.ts` (F1.3 options, `formatRelative`), `format.spec.ts`, `icons.ts`
   (F1.14), `lib/frames.ts` (F1.12, F1.13), `components/ResetToDefaultButton.vue` (F1.9),
   `i18n/en.json` (F1.3 removals, F1.5 `common.configSecrets.*`, F1.6 key move, F1.7 key move).
2. `packages/workbench`: `components/ViewHead.vue` (F1.1), `components/RelativeTime.vue` (F1.2),
   `behaviour/format.ts` (F1.3), `components/VirtualList.vue` via `git mv` (F1.4),
   `config/ConfigSecretsSection.vue` (F1.5), `lib/logTail.ts` and `components/LogLines.vue` (F1.6),
   `config/SaveConflictAlert.vue` (F1.7), `config/ConfigFormProblemAlert.vue` (F1.8),
   `components/PendingToast.vue` (F1.11).
3. Workbench sites: F1.7/F1.8 `automations/RunFileDialog.vue`; F1.9 six `settings/fields/*` and
   `llmPrompts/PromptArgumentField.vue`; F1.11 both toasts; F1.13 `WorkbenchShell.vue`; F1.14
   `panelMenus.ts`, `RunOutcomeBlock.vue`, `ImportJobDetail.vue`; F1.15 `useCountdown.ts`,
   `ApprovalsToast.vue`, `ApprovalItemCard.vue`.
4. Moves that keep the tree compiling: F1.2 five importers and both deletions; F1.3
   `AdeSessionsRow.vue`; F1.4 four docker imports plus `side_panels` path in
   `scripts/check-ui-primitives.sh` (no guard lines); F1.6 both LogsViews' tail key swap.
5. `src/ade/v2/workflows/AdeYamlPane.vue` (F1.10, created only).
6. ARCHITECTURE (section below).
7. `bun run typecheck`, `bun run lint`, specs of every touched surface. Commit.

### Wave 2 streams (worktree each, off the wave-1 commit)

| Stream | Owns this iteration | Sites |
|--------|---------------------|-------|
| A Studio | `src/studio/**` | F1.1 x12, F1.2 x4, F1.3 `timestamp.ts`, F1.4 BrowseView and ResourceTable, F1.5 ApiSecrets, F1.6 kube LogsView, F1.7 ApiVariablesDialog, ConnectionFileDialog, `confirmOverwrite.ts`, F1.8 x4, F1.9 x10, F1.12 x3, F1.13 ProofWindow, F1.14 FkPreviewPopover and DiscoverSavedMenu, F1.16 CachePane, F1.17 chartOption |
| B Space and ADE | `src/{ade,repo,hostview,views,openedfiles,components}/**`, `packages/git-ui/src/**` | F1.1 AdeReposPage and GitViewHead, F1.2 x3, F1.4 RepoSearchView, F1.5 AdeSettingsSecrets, F1.7 x6 (RepoFileView, AdeWorkflowGraph, AdeLibraryStepForm, AdeSettingsPane, AdeContextPane, RepoFilePane), F1.8 RepoFilePane, F1.9 AdeAgentDialog and AdeRunDialog, F1.10 delete two YAML files and repoint parents, F1.13 AdeReviewWindow and OpenedFilesWindow, F1.14 AdeReposPage, AppToolbar, AdeStepBlock x2, AdeWorkflowReviewPanel, useSessionMenu x2, AdeSessionActions, rowMenuModel |
| C Shell and docker | `src/workbench/**`, `mobile/**`, `packages/docker-ui/src/**` | F1.6 docker LogsView, F1.9 x20 (`src/workbench/settings/*`), F1.16 EngineOverview |

- Shared files: `packages/theme/src/i18n/en.json`, removals only, distinct keys: A removes
  `studio.api.config.secrets.*`; B removes `space.ade.settings.secrets.*`; C none. No new keys in
  wave 2. Specs under `apps/kira-space/tests/ui/` belong to the stream owning the surface; a spec
  touching two streams' surfaces goes to wave 3.
- Never touched in wave 2: `scripts/check-ui-primitives.sh`, `docs/ARCHITECTURE.md`,
  `packages/theme/src/**`, `packages/workbench/src/**` (all workbench sites are wave 1 this
  iteration).
- No ordering dependency between streams: each consumes only wave-1 pieces; no stream edits a file
  another stream or wave 1 edits, except the listed `en.json` removals.
- Each stream runs the guard regexes for its findings by hand (`grep -Pzl` with the script's
  `PRE`) over its dirs and reaches zero, then `bun run typecheck`, `bun run lint` and its specs.
- A conflict outside `en.json` at rebase means the ownership table was wrong: stop and report.

### Wave 3 (one Sonnet, after rebasing A, B, C onto the chapter branch)

1. Add header `# Guards U162-U170: docs/HISTORY.md (v0.3.18).` and the nine lines below to the
   `GUARDS` heredoc. It is unquoted: `${TOK}` expands; escape any literal `$`. `PRE` defines group
   1, so a backreference uses a named group with `\k<name>`, never `\1`.
2. Add the six U164 PERMANENT lines below.
3. Test each regex: zero hits on the tree, one hit on a planted offender in a scratch copy.
4. `bun run lint:rules`; fix any leftover offender; commit. Remove worktrees, push.

```
U162@@v@@ViewHead.vue|ApprovalItemCard.vue@@hand view head band (use ViewHead)@@colorMarkClass\('band'
U163@@v@@@@relative time as bare template text (use RelativeTime)@@\{\{\s*formatRelative\(
U164@@v@@VirtualList.vue@@hand virtual listbox (use VirtualList)@@(?s)\A(?=.*useVirtualRows\()(?=.*useListNavigation\b[<(])(?=.*role="listbox")
U165@@vt@@logTail.ts@@log tail constants outside logTail.ts (use LOG_TAILS, LOG_MAX_LINES)@@\b(?:MAX_LINES|TAILS)\s*=\s*[\[\d]
U166@@vt@@SaveConflictAlert.vue|confirmOverwrite.ts@@hand save-conflict alert (use SaveConflictAlert)@@common\.action\.overwrite\.label
U167@@vt@@ConfigFormProblemAlert.vue@@hand config form problem alert (use ConfigFormProblemAlert)@@common\.configForm\.problem\.status
U168@@vt@@ResetToDefaultButton.vue|PromptDetail.vue@@hand reset-to-default button (use ResetToDefaultButton)@@common\.action\.resetToDefault\.label
U169@@vt@@frames.ts@@hand drop indicator (use dropIndicator)@@['"](?=[^'"]*${TOK}h-0\.5(?![\w-]))(?=[^'"]*${TOK}bg-focus(?![\w-]))
U170@@vt@@frames.ts@@hand window tab strip (use windowTabStrip)@@['"](?=[^'"]*${TOK}h-tabbar(?![\w-]))(?=[^'"]*${TOK}min-h-0(?![\w-]))(?=[^'"]*${TOK}bg-chrome(?![\w-]))
```

```
# P674 iteration 1: virtual lists VirtualList cannot host
U164 apps/kira-space/frontend/src/ade/v2/repos/AdeReposPage.vue # grouped windowRows with group heads, not a flat row list
U164 apps/kira-space/frontend/src/studio/views/shared/keyvalue/KeyValueRowsTable.vue # custom visibleRows windowing over key and value columns
U164 apps/kira-space/frontend/src/studio/views/stream/StreamRowsTable.vue # frozen index column beside the virtual body
U164 apps/kira-space/frontend/src/studio/views/documents/DocumentView.vue # variable-height rows, tooltip delegation on the scroller
U164 apps/kira-space/frontend/src/studio/views/console/ConsoleResultGrid.vue # two virtualizers over mapped indices
U164 packages/docker-ui/src/components/ContainerTable.vue # header row inside the listbox, sized by container queries
```

### ARCHITECTURE (wave 1 writes rules, wave 3 updates the guard range)

- Rename "UI uniformity rules (P672)" to "UI uniformity rules (P672, P674)". Guard range line
  `U83-U170`; PERMANENT count 54.
- Replace "A detail head inside a toolbar row ... stays ViewToolbar (git detail-head, Studio tab
  view-head)" with: a view head (dot, kind icon, title with dim prefix, badges, actions, band) is
  `ViewHead`. U162.
- A standalone instant in a template is `RelativeTime` (ticks, absolute tooltip); `formatRelative`
  only inside composed text; one vocabulary from `relativeTimeOptions()` (floor; m, h, d, mo, y;
  past "ago", future "in"). U163.
- The 200-row list rule names `packages/workbench/src/components/VirtualList.vue`; a flat keyboard
  listbox uses it; direct `useVirtualRows` plus `useListNavigation` needs a PERMANENT reason. U164.
- Config secrets render `ConfigSecretsSection`; removing a stored secret is Delete.
- Log views take tail constants from `logTail.ts` and render lines through `LogLines`; toolbar
  order follow, timestamps, wrap, tail. U165.
- Config files: save conflict is `SaveConflictAlert`, form problem is `ConfigFormProblemAlert`.
  U166, U167.
- Reset to default is `ResetToDefaultButton`. U168.
- `frames.ts` owns `dropIndicator` and `windowTabStrip`. U169, U170.
- A pending-work toast is `PendingToast`.
- ICON table rows: `resume` debug-continue, `takeOver` terminal, `openInNewTab` empty-window;
  cancel a running op is `ICON.stop`; taking an item out of a list is `ICON.remove` even when
  confirmed; `ICON.delete` only when data is destroyed.
- A countdown or running clock is `formatElapsed`; a displayed percentage is `formatPercent`.

## Tests

- Pure refactors (F1.4, F1.7, F1.8, F1.9, F1.10, F1.11, F1.12, F1.13, F1.15): no new spec; run the
  ledger's specs plus `bun run typecheck`, `bun run lint`.
- One new UI spec: `apps/kira-space/tests/ui/relative-time.spec.ts` on a real host (hostview issue
  sidebar or Studio response history). Asserts the text ticks under a fake clock from "just now" to
  "1m ago", the tooltip shows the absolute date-time, and a future instant reads "in 5m".
  Renderer-only: no flow half (no value crosses IPC).
- `ViewHead`, `ConfigSecretsSection`, `LogLines` props, `VirtualList` `activeKey`: behaviour
  already covered by the host specs listed; no new spec.
- F1.3: update `packages/theme/src/format.spec.ts` (existing unit test) for floor rounding, future
  instants, month and year units.
- No DRIFT here changes a value crossing IPC: no flow test.
- Visual specs are expected to fail on visible changes; chapter end re-records.

## Visible-change list (chapter-end visual re-record)

F1.1 Studio head icon colour, head title tooltip, KeyValuePane prefix space. F1.2 tooltips on
relative times (mobile Later, mobile access, Studio history rows, Studio start, ADE proof, hostview
comments and attachments). F1.3 future and month/year wording, cell editor relative line. F1.5
ApiSecrets Delete verb and padding. F1.6 kube log bold weight, both log toolbars' order. F1.14
glyphs at the listed sites. F1.15 countdown over one hour. F1.16 cache hit rate and engine CPU
total. F1.17 chart tooltip placeholder.

## Dismissed clusters

Key plus reason. A key not listed here and not named in a finding is in a dismissed family below.

### C1 Template subtrees

- `7d21298f1f`, `9aadc7c574`, `19ca60109c`: see F1.7, F1.8.
- `e3ea058c23`, `44eda84344`, `f0e40ccef1`, `595f6f64b5`, `c1cde077a2`: see F1.9.
- `c49c2d0bbc`, `735603159a`, `eecdc389e3`: see F1.1.
- `3c250fdcb3`: see F1.6. `99f18f00df`, `bddf1011a9`, `db359d73b6`: see F1.5. `8e6e85e50e`: see
  F1.10. `8d477eca5b`: see F1.11. `5c17282481`: see F1.13. `2f1a55c903`: see P685.
- `512242a115`, `9d3ca216a8`, `3047589af3`, `d272af9382`, `e6d4fffcfc`, `fe0d95f97c`, `d4465a08a4`
  (settings and dialog `Field` blocks), `ac356cd767`, `1e6d48e961`, `d60dd379f1`, `b78ffa48a4`,
  `0bc265a7d5`, `1384144a9c`, `340e90de52`, `930616789e`, `98d0ed35cf`, `c01b25ab5f`,
  `e2fdd62db6`, `ec76bf3b79`, `513b64457a`, `212b2e8abd`, `339c7a2b92`, `3ca1111162`,
  `560587aa6d`, `70dc91b004`, `95506c3868`, `a60f136949`, `b28bb73075`, `b646ee0770`,
  `c672f0f2f8`, `d3edf6cc77`, `e8dbe1134e`, `f1ad1ed824`, `f5f3475976`: identical composition of
  shared primitives with the same look; the sameness is the uniform result, not a reimplementation
  (each 2-3 sites, no visible divergence).

### C2 Class strings

- `0df039662c`: F1.12. `3cb12e9f47`: F1.13. `63846e8141`, `797f07aba9`: F1.1 (head icon box).
- `7fa2d94cdd` (`absolute inset-0 no-rows`): grid overlay class owned by `slickTheme.css`, wraps
  `InlineState` already.
- `74adef8b7c` (`font-medium text-kira-lg`): docker metric figure, one module, consistent.
- Every other C2 cluster (`near class sets`, `exact class set flex ...`, `shrink-0 w-*`, `gap-*`
  stacks): generic layout utility sets, not a component job.

### C3 Script duplicates

- `5023aada8d`, `1276fe06fb`: F1.6. `ad40e06877`, `bdc7c4071a`, `f401e2d3f0`: F1.5. `897baa249e`:
  F1.10. `07c53139b5`: F1.2. `af7ebb83d0`, `b14b3cbeda`, `e11c8a0135`: F1.4. `8c58cf4a56`,
  `df06574d11`, `f20c31e2ec`, `008bc386e0`, `c27e000bf7`, `34281be21f`: P685. `53b6d123f9` and the
  per-kind `search.ts`/`page.ts`/`state.ts` families: P683.
- Import-list fragments (`import { Alert ... }`, `import CodiconIcon ...`, `Button`,
  `TooltipIconButton`, `SectionHeading`, `SecondaryTabs`, `CodeBlock`): imports of shared pieces.
- Fixture and test-data fragments (`detail: '~N'`, `kind: 'table'`, `dataType: 'UIntN'`,
  `path: 'database:kira_test/...'`, `generatedAt`, `scripts: RUN_SCRIPTS`, `1bc36c9734`,
  `bccab77529`, `bcddbe8fb1`, `c06d833d63`, `d5cc869c0d`, `e7cb0496ec`, `f72bcfe092`,
  `fdca10a1db`, `735846aa78`): fake data and dev fixtures.
- `git-core` vs `git-ipc` contract mirrors (`68c15e8c65`, `728bc6bcaf`, `7f68febdc0`, `d988ced533`,
  `5497306cdc`, `3638a61cc0` and kin): by design, one side per process.

### C4 Shared-piece near-miss

- `71c8b10f67` RelativeTime: F1.2. `d4a1e161bf` LogsView: F1.6 (two hosts stay, one per module).
- `abbee1bef8` App, `3ab0cfef6b` DockerPanel, `953799b2d7` DockerView, `8139fe6cb9` StatusBar,
  `d0446f6267` WorkbenchShell: app host glue wrapping the package piece of the same name.
- `7a72e4a3d1` ResponsePane: gRPC vs HTTP response, different jobs. `e2697b4770` RestoreDialog:
  docker volume restore vs workbench session restore.
- `61f8f44ccd` SearchField: picker filters inside popovers own focus and size, `RepoScriptField` is
  an autocomplete input. `2380f506c3`, `5afa682e39`, `035756f333`, `6fa738cf97`: `RowActionButton`
  is a hover row action with spinner, `TabbedContent` a tab menu. `5d79c5d953` StatusDot: D1 board
  chips. `173afea9ed`, `2ae78e3e33`: RestoreDialog list is not a start page or tooltip.
  `4469c3a960`: `AdeStatusName` is a rename dialog field, not inline edit. `a31d45c841`:
  `ApiAccessRuleDialog` is a form dialog, not a confirm. `a6b96fb1e6`: KeyValueView op list shares
  markup score only.

### C5 Display drift

- `d42f874554`, `2d756aa337`, `ae37600549`, `7030906d9e`, `64083d2b39`, `d751ea7ce9`,
  `cd9fe18c19`, `80aa4d88b5`, `751e7f92dc`, `cda1944941`, `0008013a7c`, `63102032fc`,
  `c0ad7a10e3`, `8cca673797`, `ae31c8b114`, `b7e1ffb06a`, `be8cf61338`, `7b836a2622`,
  `975f566a2a`, `e3c6f34fc3`: input and data encoding (`datetime-local` values, local-day keys,
  screenshot file names, cell decoders, Monaco theme), not display formatting. `useCountdown.ts:9`
  is F1.15. `opLog.ts:106` `formatOpTime` is the op log's fixed-width column format.
- `ea3a9de947`: F1.3. `629ff26ff9`: F1.16 (two sites), rest are style widths. `5c67f46c8b`: F1.17.
- `ddb14076c4`: constants passed into `formatBytes` or byte limits, not display.
- `b9823b0a89`: same em dash as `displayValue`, inline fallbacks of a ripple text and a tab title.
- `379caff6ea`: path parsing. `1223cf7077`: docker `shortId` (12 chars) is not a git sha.
  `3e2f008844`: cURL parser. `7273d5a540`, `9cf2d782ce`: locale lookups. `82c4c8be05`: settings
  default chord data.

### C6 Vocabulary

- One concept, several icons, acted: `ccaca5d54e` (2 sites), `f459a73ad1`, `c2ca74103a`,
  `d45bd5db29`, `68740230e0`, `2a85792395`: F1.14.
- `ccaca5d54e` rest: `close` for remove is `ICON.remove`; `WorktreeList.vue:184` trash is right
  (worktree removal deletes the directory). `0d6feebde3`: MaskRuleRow removes an unsaved row,
  ApiAccessRules deletes a stored rule. `9359744c7b`: forward dismiss vs import data delete.
  `4abb3b6c4f`: `close` clears a field (`ICON.clearField`), `clear-all` clears a list.
  `38f0b307ca`: text filter toggle is search, memory facet menu is filter. `1253c72561`: key-value
  count, not run. `7f7a248f6f`: import a repo vs import files. `6929f49396`: move to next status vs
  calendar step. `b69e3e0c03`: back to engine list vs change engine. `d10c1bce2a`: menu verb vs
  toolbar object. `acfc79376d`, `9d7500a42f`, `acab56b702`, `10e016cfbe`, `fbae4b03e6`,
  `c8146562fa`, `d43c552d62`, `8a0cb7f1fa`, `da897df03a`, `13e52f61f8`, `8f6edff1b2`,
  `434219ccfc`, `ea1bd625c6`, `89af330ad6`, `515c5d20ef`, `5c2c0626fb`, `6371ad3d7e`,
  `980bb01248`, `a9f6e7a958`, `b251cf5dae`, `c35557a7b7`, `de8f136240`, `e2447b68b3`,
  `c67290b1bc`, `43ecd8746e`, `2ae3479c03`: the concept word is generic (open, status, start, empty
  hint, none, list, view, missing, add); each icon names its object.
- One icon, several concepts (`e510fdf47b`, `c406cc27e7`, `645ffef1b6`, `106c28604d`,
  `c035a2b621`, `8f3932d73b`, `107d652b5d`, `51f1f07a61`, `b32b867c8f`, `fb9f0a90ff`, `9f6cb66bd2`,
  `c0721757b8`, `9ea0546a69`, `668a45db3f`, `956ebd4a1f`, `e1315c0e04`, `14ce2e0ea1`,
  `20ac0b0777`, `007109c8ef`, `3c0b0d1313`, `a4b869c11e`, `c5ec5e23df`, `530bf683aa`,
  `7884d3b665`, `ab8b8c0266`, `1e3bb7f004`, `1e5d05e84f`, `df54a7bf20`): a glyph serving several
  objects is normal codicon use. `a4b869c11e` debug-continue loses take-over via F1.14.
- Literal icon bypassing ICON (`2bf11e5298`, `6236cb8500`, `f820d917ba`, `d187543f1e`,
  `ec53a334ac`, `cc2d6af0ec`, `3873164d34`, `45bdefb7b9`, `eff90b2f38`, `cbd0708dd4`, `a6b728473d`,
  `76009a4398`, `d2bde498c6`, `c0fdf95f2e`, `f8261fd023`, `7d72ab3743`, `c7da84b45a`, `e12fac66bd`,
  `fe02406b3f`, `22a5f547ac`, `98378f7c99`, `d578caa236`, `4bb817d3a2`, `fb980e59ee`): same glyph,
  no visible drift (~400 sites). `cc2d6af0ec` reset sites move with F1.9.
- English under 2 keys (`13446ea3f6`, `36306334e0`, `395495b557`, `4315b722ec`, `4479d01229`,
  `462ed59889`, `52c6f498d3`, `540ffc10ae`, `55af5935c3`, `670d1f52a4`, `718d061551`,
  `7d2572155a`, `7d4186382a`, `9f547f4546`, `a1a84ef8b4`, `a50eda8831`, `a845be1c2d`,
  `ac757d2597`, `bcd339ae77`, `c473203709`, `da8112bc6d`, `dbdfcd8c83`, `dbeec1e977`,
  `ed6d74106f`, `f0e4bdb402`): `backend.*` vs frontend namespaces or per-module labels, separate by
  design. Secrets duplicates go with F1.5.
- `42d9983942` Discard / Revert: two actions (replace unsaved changes on open vs revert to saved).

### C7 Library near-miss

- `e12948b16c`: P682.
- `6dbc1da8c1` role=listbox: F1.4 for the virtual lists; the rest are listbox containers driven
  by `useListNavigation`, not a hand widget.
- `8a94e3c6ef`, `cd04bdb11b`: `AbortSignal` plumbing. `71e09b471e`, `77682403ae`, `79d1ce5cc6`,
  `9314f0fc60`, `272ea23e5c`, `eb063ffad0`, `a0e4685dc0`, `c9bd6acc33`: SlickGrid and xterm engine
  hosts, `linkify.ts` delegation, terminal resize throttle. `c5b22511a1`, `890c1275df`: worker
  messaging. `9abd6e0f6f`: mobile SSE event source.
- `6792df35ac`, `9571dc3527`: `persisted.ts` is the storage owner. `9d11206ab9`: `clipboard.ts` is
  the clipboard owner.
- `8f65d28ed6`, `b5ec0b54f2`, `4cdee45a30`: documented module-level state for non-component
  readers.
- `55b1fda51d`: `TextPromptDialog` declines shadcn Dialog for a named requirement (must stay a DOM
  descendant of a Popover).
- `8ee260757d`: request timeout, not a debounce.
- `d4ad5858d6`: `readTokens.ts` observer lives in a class outside any effect scope; VueUse
  `useMutationObserver` needs one.
