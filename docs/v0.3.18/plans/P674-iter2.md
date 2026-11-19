# P674 Part 3: UI uniformity audit, iteration 2

Auditor ledger, wave plan, stream table and guard list for iteration 2. Master plan: `P674.md`.
Iteration 1 ledger: `P674-iter1.md`. Implementers tick each entry `done <sha>` as it lands.

## Base and tool report

- Base commit: `cfcd9c5d6` (`chapter/v0.3.18`). In tree since iteration 1's scan: P676 (scripts in
  approval card, `JsScriptBlock`, settings switch), P678 Part 1 (kept variables: card panel, settings
  view, `studio/kept/*`), iteration 1 waves 1-3.
- Not in tree: P677 (`ResultReview.vue`, `ApiResponseBody/View/Details.vue`, `ReviewChoiceField.vue`,
  `ReviewResultOption.vue`, edits to `PendingResponse.vue`, `StudioMcpApiPane.vue`,
  `settingsDomain.ts`) lands concurrently in another worktree. Excluded here; final scan (Part 4)
  covers them. Wave 3 runs every new guard over the full tree as it stands then, P677 files included,
  and fixes leftover offenders (see Wave 3).
- `bun run audit:ui --out <scratchpad>/audit2.md`, default thresholds, 6.1 s. Scan set 1601 files
  (723 `.vue`, 878 `.ts`). Report not committed; rerun to reproduce.
- Clusters per channel: C1 42, C2 198, C3 345, C4 19, C5 29, C6 115, C7 24 (772 total).
- Key diff against iteration 1's ledger: 243 keys seen before; new keys C1 9, C2 195, C3 322, C6 3;
  C4, C5, C7 carry no new key. Every C4, C5, C7 cluster was re-checked for sites in files changed since
  `bc09408cb`: none new outside iteration 1's own pieces.
- Triaged: every new C1 and C6 key; every C2 and C3 cluster with module spread >= 2 touching a file
  changed since iteration 1, plus a family pass over all new C3 keys with spread >= 2.
- Hand sweep (channel `S`), like against like, focused on P676/P678 surfaces and on what iteration 1's
  pieces now make visible: copy buttons, masked values, secrets sections, standalone instants,
  countdowns, load-failure states, approval meta links, settings numeric fields, catalog borrowing.
- Discovery: this worktree has no codegraph index. `codegraph_explore` ran on the
  `/home/user/kira-space` index for callers and blast radius of pre-existing symbols (`useCountdown`,
  `RunSecretsSection`, `useAgentFileEditor`, `QueryState` 64 callers, `useCopy` 25, `SecretInput` 16,
  `HighlightedText` 20, `Identifier`); current source of every candidate was re-read from this
  worktree, since that index predates P676, P678 and iteration 1.
- Outcome: 11 findings (5 EXTRACT, 5 MIGRATE, 1 GUARD-GAP MIGRATE), 1 PARK (P687), rest DISMISS.
  Guards U171-U176 (6 lines), U130 exempt column edited, PERMANENT count unchanged (53).

## Concurrent streams

- P677 overlap: `PendingResponse.vue` (F2.9 site), `settingsDomain.ts` (F2.10 adds a constant,
  additive), `en.json` (key moves here, key additions there). F2.9's `PendingResponse` site moves only
  in a wave that runs on a tree where P677 has landed; otherwise wave 3 catches it through U175.
- P678 Part 2 and P679 later edit `KeptVariablesPanel`, `ApiRequestBody`, `KeptValueView`
  (secret values) and `StudioMcpKeptPane`. They build on this iteration's pieces (`MaskedValue`,
  `useKeptExpiry`, `CopyIconButton`); final scan rescans them.

## Ledger

Paths drop `apps/kira-space/frontend/` for `src/` and `mobile/`; `packages/` paths are repo
relative. Specs: grep the testids under `apps/kira-space/tests/ui/` and `tests/mobile/`; named files
are known hits.

### F2.1 [DUP] EXTRACT: copy icon button

- Sites (icon-only copy button; 6 swap to `check` for feedback, 9 do not; 3 call `copyText` with no
  announce or error surface):
  `src/workbench/settings/CliToolsPane.vue:107`, `src/ade/v2/notes/AdeNoteEntry.vue:130`,
  `src/ade/v2/links/AdeLinkCard.vue:143`, `src/ade/v2/panel/AdeBranchPanel.vue:454`, `:486`,
  `src/studio/views/httprequest/RawExchangePane.vue:262`,
  `src/studio/kubernetes/components/DescribeView.vue:154`, `src/studio/kept/KeptValueView.vue:77`,
  `packages/workbench/src/components/Identifier.vue:29`,
  `packages/workbench/src/components/OpLogPanel.vue:536`,
  `packages/workbench/src/components/OpLogDetail.vue:85`, `:103`,
  `packages/docker-ui/src/components/InspectView.vue:13`,
  `packages/docker-ui/src/components/ContainerOverview.vue:151`,
  `packages/git-ui/src/components/review/ReviewCommentsPane.vue:107`.
- Drift: feedback glyph on 6 of 15; literal `'check'`/`'copy'` bypass ICON; error tone only in ADE;
  `useCopy` doc already says "icon buttons swap to `check`".
- Shared home: `packages/workbench/src/components/CopyIconButton.vue` (uses `useCopy`, so workbench,
  not theme).
- Change: props `text: string | (() => string | Promise<string>)`, `label: string`,
  `message?: string` (announce text, passed to `copy`), `disabled?: boolean`. Renders
  `TooltipIconButton` with `copied ? ICON.copied : ICON.copy`, `tone="error"` while `error` is set.
  Attrs fall through (testid, `size`, `icon-size`, `tabindex`, class). AdeLinkCard is the one
  `@click.stop` site: the component stops click propagation always (a copy click never means the
  surrounding row's action), so no prop. New `ICON.copied = 'check'` in
  `packages/theme/src/icons.ts`. `Identifier` renders `CopyIconButton`. Hosts with a lazy or
  domain-built text pass the getter: `KeptValueView` (`copyValue` body becomes the getter),
  `RawExchangePane` (`section.onCopy` becomes a `section.copyText` getter plus `section.copyMessage`;
  the section builder lives in the same file), `ReviewCommentsPane` (pass the formatted text the
  composable's `copyForAi` copies; expose it from `reviewComments` as a getter if it is not already).
  `OpLogPanel` and `OpLogDetail` keep their announce messages through `message`.
- Guard: U171 `v`, exempt `CopyIconButton.vue`, regex
  `(?<![\w:-])icon="copy"|:icon="(?:ICON\.copy|copied\b[^"]*)"`. Labelled text buttons
  (`<CodiconIcon name="copy">` plus label) are a different job and stay.
- Visible change: yes (check feedback on 9 more buttons; error tone everywhere).
- Specs: `settings-cli-tools.spec.ts`, `ade-v2-notes.spec.ts`, `ade-v2-handover.spec.ts`,
  `ade-v2-details-links.spec.ts`, `operations.spec.ts`, `studio/kept-variables.spec.ts`,
  `studio/approvals-api-script.spec.ts`, kube describe and docker inspect specs (grep
  `kube-describe-copy`, `docker-inspect-copy`), HTTP raw pane specs (grep the section copy testids),
  git review comments specs.
- Wave 1 (component, ICON, Identifier, OpLogPanel, OpLogDetail); wave 2 (12 sites).

### F2.2 [DRIFT] EXTRACT: masked value

- Sites (a hidden secret shown as bullets: 8 or 4 bullets, with or without `text-subtle
  tracking-widest`): `src/studio/api/VariableRow.vue:82`, `src/studio/api/VariableHistoryMenu.vue:74`,
  `src/studio/kept/KeptValueView.vue:67`, `src/studio/project/credentialPaste/CredentialPastePanel.vue:111`
  (`'•'.repeat(8)`), `src/ade/v2/port/AdePortImportDialog.vue:252` (and comment `:29`),
  `packages/docker-ui/src/components/ContainerOverview.vue:140`,
  `packages/workbench/src/automations/run/RunPreview.vue:33`, scenario
  `packages/workbench/src/testing/ui/scenarios/automationsSecrets.ts:183` (asserts `'••••'`).
- Shared home: `packages/theme/src/format.ts` `MASKED_TEXT = '••••••••'` (8, the majority; matches
  U130's masked-data tracking); `packages/theme/src/components/MaskedValue.vue` (presentational).
- Change: `MaskedValue` renders `<span data-slot="masked-value" class="text-subtle
  tracking-widest">{{ MASKED_TEXT }}</span>`, attrs fall through (testid, `flex-1`, `truncate`).
  Template sites render `MaskedValue` keeping their testids (`variable-value-masked`,
  `variable-history-masked`, `kept-value-secret`, `ade-port-secret` stays on the `li`).
  ContainerOverview renders `MaskedValue` in a `v-else` beside the revealed value span. Composed
  strings use `MASKED_TEXT` (`RunPreview` env parts, `CredentialPastePanel.shown`). The scenario
  imports `MASKED_TEXT`. Reword the AdePortImportDialog comment to "the mask". `mask.ts`/`mask.go`
  per-character data masking is a different job (length hints) and stays.
  U130 exempt column: replace `VariableRow.vue|VariableHistoryMenu.vue` with `MaskedValue.vue`.
- Guard: U172 `vt`, exempt `format.ts`, regex `•{4}|'•'\.repeat\(`.
- Visible change: yes (kept, ADE port and run preview masks go from 4 to 8 bullets; kept, ADE port and
  docker masks take subtle colour and wide tracking).
- Specs: `studio/http-variables.spec.ts`, `studio/api-secret-reveal-isolation.spec.ts`,
  `studio/kept-variables.spec.ts`, `ade-agent-port.spec.ts`, `studio/consistency-docker-automations.spec.ts`,
  credential paste specs (grep `credential-paste`), automations secrets scenario hosts (grep
  `automationsSecrets`).
- Wave 1 (constant, component, U130 exempt edit deferred to wave 3, RunPreview, scenario); wave 2 (6
  sites).

### F2.3 [DUP] MIGRATE: run secrets to ConfigSecretsSection

- Site: `packages/workbench/src/automations/RunSecretsSection.vue` (rendered by
  `packages/workbench/src/automations/RunFileDialog.vue:383`). Same job as F1.5's
  `ConfigSecretsSection`: values a config file references by name, set once, never shown. Iteration 1
  missed it. Drift: verb Clear without confirm vs the ARCHITECTURE rule "removing a stored secret is
  Delete"; inline input per row vs set dialog; Saved badge vs Set/Not set badge.
- Shared home: `packages/workbench/src/config/ConfigSecretsSection.vue` (no prop change).
- Change: `RunSecretsSection` keeps its `v-if="secretParams.length > 0"` and maps
  `secrets = secretParams.map(p => ({ name: p.name, set: isSaved(p.name), referenced: true }))`,
  `storage = { available: !storageOff, reason: t('automations.params.secretValue.storageOff.description') }`,
  `setSecret(name, v) => scripts.setSecrets(id, { [name]: v })`,
  `deleteSecret(name) => scripts.setSecrets(id, { [name]: null })`, `testidPrefix="run-secret"`;
  slots `title` (keep `automations.file.secrets.title`) and `description`
  (`automations.file.secrets.hint`). It becomes a thin host; drop its `useAsyncAction`, drafts and
  markup. Remove catalog keys left unused (`automations.params.secretValue.saved.status` only if no
  other caller: `RunPreview` uses it). Testids change: `run-secret-value-<n>` to `run-secret-value`
  (dialog), `run-secret-save-<n>` to `run-secret-save`, `run-secret-clear-<n>` to
  `run-secret-delete`, `run-secret-saved-<n>` to `run-secret-state`, `run-secrets-off` to
  `run-secrets-storage-off`; update the scenario.
- Guard: none (semantic: one component per job; no stable pattern). ARCHITECTURE rule line names run
  secrets beside API and ADE.
- Visible change: yes (run file dialog secrets become list plus set dialog; Clear becomes Delete with
  danger confirm).
- Specs: scenario `packages/workbench/src/testing/ui/scenarios/automationsSecrets.ts` and its UI spec
  hosts (grep `automationsSecrets`), `repo-script-picker.spec.ts` (unaffected testid, run it).
- Wave 1 (all workbench).

### F2.4 [GUARD-GAP] MIGRATE: standalone instants through useRelativeTime

- Rule: "A standalone instant in a template is `RelativeTime`" (ARCHITECTURE, U163). U163 catches
  only `{{ formatRelative(`; these render `useRelativeTime` text bare.
- Sites: `src/ade/v2/later/AdeShelfRow.vue:23,71`, `src/ade/v2/needs/AdeAllSessionRow.vue:19,65`,
  `src/studio/approvals/KeptVariableRow.vue:19,45`,
  `src/studio/workbench/settings/KeptEntryRow.vue:20,48`,
  `packages/docker-ui/src/components/ContainerSizeSection.vue:22,30` and
  `packages/docker-ui/src/components/EngineDiskSection.vue:37,62` (both hand-build `RelativeTime`:
  `TextTooltip` with `formatDateTime` around the text).
- Not offenders (composed text inside `t()`): `src/ade/v2/review/AdeReviewSync.vue:20`,
  `src/ade/v2/sessions/AdeSessionTabTip.vue:12`, `src/ade/v2/status/AdeAgentsStatusRow.vue:13`.
- Change: each site renders `<RelativeTime :at="..." />` keeping its class and testid (on the
  component root via attrs); delete the `useRelativeTime` call. Docker sections pass
  `size.takenAt` / `usage.takenAt` and drop their `TextTooltip`.
- Guard: U173 `v`, exempt `RelativeTime.vue|AdeReviewSync.vue|AdeSessionTabTip.vue|AdeAgentsStatusRow.vue`
  (the three compose the text into a label), regex `\buseRelativeTime\(`.
- Visible change: yes (absolute tooltip on ADE shelf and all-sessions rows and on kept ages).
- Specs: `module-state.spec.ts`, `ade-v2-needs.spec.ts`, `status-agents.spec.ts`,
  `studio/docker-disk.spec.ts`, `studio/kept-variables.spec.ts`, `studio/approvals-api-kept.spec.ts`,
  `relative-time.spec.ts`.
- Wave 2 (6 sites).

### F2.5 [DUP] EXTRACT: kept variable expiry

- Sites: `src/studio/approvals/KeptVariableRow.vue:20-27` and
  `src/studio/workbench/settings/KeptEntryRow.vue:21-28` (identical `useCountdown` plus
  Expired / "Expires in" computed; audit C3 `29a53bdcc5`). Both borrow
  `workbench.approvals.expired.status` from the approvals namespace.
- Shared home: `src/studio/kept/keptExpiry.ts` (module-local, both consumers in Studio).
- Change: `useKeptExpiry(expiresAt: () => string): ComputedRef<string>` (the current computed body).
  New key `common.expired.status` ("Expired"); `workbench.approvals.expired.status` removed and its
  third caller `packages/workbench/src/approvals/ApprovalItemCard.vue:54` swaps to the new key.
  Kept TTLs keep `formatCompactDuration` (long lifetimes, as `KeyValueToolbar` TTLs); the approval
  clock keeps `formatElapsed`.
- Guard: none (module-local; the duplicate is deleted; audit C3 shows a recurrence).
- Visible change: no.
- Specs: `studio/kept-variables.spec.ts`, `studio/approvals-api-kept.spec.ts`, `approvals.spec.ts`.
- Wave 1 (`keptExpiry.ts`, key move with all three key swaps); wave 2 (two rows adopt it, with F2.4
  and F2.8 in the same files).

### F2.6 [DRIFT] MIGRATE: pairing countdown

- Site: `src/workbench/PairingRequestDialog.vue:49` "Expires in {seconds}s" (`common.expiresIn.status`,
  raw seconds). ARCHITECTURE: "A countdown or running clock is `formatElapsed`"; approvals show
  "1:27 left".
- Change: `t('common.expires.status', { time: formatElapsed(remainingSeconds * 1000) })`; delete
  `common.expiresIn.status`. `usePendingDecision` keeps its own paused tick (requirement: the dialog
  is always mounted and must not tick while hidden; `useCountdown` cannot pause).
- Guard: none (deleted key fails typecheck).
- Visible change: yes ("Expires in 1:27" instead of "Expires in 87s").
- Specs: `mobile-access.spec.ts`, pairing prompt specs (grep `-expires"`).
- Wave 2 (1 site).

### F2.7 [DRIFT] EXTRACT (extend QueryState): load failure with Retry

- Rule: ARCHITECTURE "a QueryState load failure is 'Couldn't load' plus a Retry action"; "list or
  section InlineState, failed operation with content kept Alert error; Retry is a toolbar Button
  with `refresh`". The inline variant has no Retry, so callers override the `error` slot four ways.
- Sites, hand Retry: `src/studio/workbench/settings/StudioMcpKeptPane.vue:116` (Alert plus `dialog`
  Retry), `src/ade/v2/links/AdeLinkStrip.vue:186` (FieldError plus Retry),
  `src/studio/project/connection/ConnectionMcpTables.vue:142`,
  `src/studio/project/connection/ConnectionMaskReview.vue:212` (InlineState with own text plus
  Retry). Alert for a load failure, no Retry: `packages/workbench/src/memory/MemoryPanel.vue:172`
  (pane, strip), `packages/workbench/src/memory/MemoryStart.vue:56` (pane, strip),
  `packages/workbench/src/memory/settings/AgentMcpSection.vue:39`,
  `packages/workbench/src/memory/settings/CheckHookSection.vue:25`,
  `packages/workbench/src/automations/form/McpToolsField.vue:98`,
  `packages/workbench/src/automations/schedule/ScheduleConfirmDialog.vue:208`,
  `packages/workbench/src/automations/run/RunScriptDialog.vue:239`.
- Shared home: `packages/workbench/src/components/QueryState.vue`.
- Change: props `errorLabel?: string` (replaces the error text; ConnectionMcpTables and
  ConnectionMaskReview pass their offline wording) and `errorTestid?: string` (default
  `query-error`; keeps `settings-kept-error`, `memory-error`, `schedule-confirm-preview-error`,
  `run-preview-error`, `connection-mcp-tables-offline`, `connection-privacy-review-offline`). Inline
  error renders `InlineState kind="error"` and, when `canRetry`, a sibling toolbar Retry
  (`data-testid="query-retry"`, `ICON.refresh`) in a `flex min-w-0 items-center gap-2` wrapper;
  without Retry it stays the bare InlineState. Pane error uses `errorLabel ?? message` and
  `errorTestid`. Every listed site drops its `#error` slot. `settings-kept-retry` becomes
  `query-retry` in specs. `RunLog.vue:17` and `AdeRunLog.vue:37` keep their slot (the log frame shows
  its own error through `LogLines`).
- Guard: U176 `v`, exempt none, regex `#error="\{[^"]*\bretry\b|#error="[^"]*"\s*>\s*<Alert\b`.
- Visible change: yes (Retry beside every retryable inline load error, all 64 QueryState callers;
  memory panel and start show the pane failure block; settings and dialog load errors read as
  InlineState instead of Alert).
- Specs: `studio/kept-variables.spec.ts`, `studio/connection-mask-review.spec.ts`, connection MCP
  tables specs (grep `connection-mcp-tables`), `ade-v2-details-links.spec.ts`,
  `settings-memory.spec.ts`, `operations-memory.spec.ts`, `automations-mcp.spec.ts`,
  `studio/consistency-docker-automations.spec.ts`, `studio/ui-leftovers-docker-automations.spec.ts`,
  schedule and run dialog specs (grep `run-preview`, `schedule-confirm`), plus any spec hitting
  `query-error` (`consistency.spec.ts`, `studio/kubernetes-module.spec.ts`).
- Wave 1 (QueryState, 7 workbench sites); wave 2 (4 sites).

### F2.8 [NEAR] MIGRATE: hand mark

- Site: `src/studio/approvals/KeptVariableRow.vue:34` `<mark :class="['rounded-kira-sm text-inherit',
  KEPT_MARK_CLASS]">` copies `HighlightedText`'s mark markup.
- Change: `<HighlightedText :parts="[{ text: use.name, match: true }]" :mark-class="KEPT_MARK_CLASS" />`.
- Guard: U174 `v`, exempt `HighlightedText.vue`, regex `<mark[\s>]`.
- Visible change: no.
- Specs: `studio/approvals-api-kept.spec.ts`.
- Wave 2.

### F2.9 [NEAR] EXTRACT: approval meta link

- Sites (Button `variant="link" size="inline"` restyled with the same 7 classes `text-kira-sm
  font-normal text-muted-foreground underline decoration-border underline-offset-3`):
  `packages/workbench/src/approvals/ApprovalPairs.vue:60`,
  `src/studio/approvals/ProofOpenWindow.vue:18`, `src/studio/approvals/PendingResponse.vue:269`
  (audit C2 `31f45724c4`).
- Shared home: `packages/theme/src/components/ui/button/index.ts` new variant `link-meta` (those
  classes plus `cursor-pointer`).
- Change: sites use `variant="link-meta" size="inline"`; `PendingResponse` keeps `self-start`.
  `KeptVariablesPanel` Manage stays `variant="link"` (navigation link, as the HTTP panes' "Edit global
  defaults").
- Guard: U175 `vt`, exempt none (`ui/` is never scanned), regex `${TOK}decoration-border(?![\w-])`.
- Visible change: no.
- Specs: `approvals-alignment.spec.ts`, `approvals.spec.ts`, proof window specs (grep
  `proof-open-window`).
- Wave 1 (variant, ApprovalPairs); wave 2 (ProofOpenWindow; PendingResponse only when P677 has
  landed, else wave 3).

### F2.10 [DRIFT] MIGRATE: kept retention field

- Site: `src/studio/workbench/settings/StudioMcpKeptPane.vue:29-63,79-100`. Every other numeric
  settings leaf (`ApiPane`, `AdvancedPane`, `GitPane`, `CachePane`, `FontSizeField`; audit C1
  `6412b92d02`, `8dc3af3728`) has a head row with `ResetToDefaultButton`, a range constant from
  `src/state/settingsDomain.ts`, and `FieldError v-if ... FieldDescription v-else`. This field has
  pane-local `RETENTION_MIN = 5`, `RETENTION_MAX = 240` duplicating the schema
  (`settingsDomain.ts:145`), no reset, and shows description and error together.
- Change: export `KEPT_RETENTION_MINUTES_RANGE = { min: 5, max: 240 }` and
  `KEPT_RETENTION_MINUTES_DEFAULT = 60` from `settingsDomain.ts`, used by the schema, both defaults
  objects (`:353`, `:462`) and the pane. Head row `flex items-center justify-between gap-1` with
  `FieldLabel` and `ResetToDefaultButton` (`data-testid="settings-reset-dbMcp-keptRetentionMinutes"`,
  `:at-default="minutes === KEPT_RETENTION_MINUTES_DEFAULT"`, click runs `retention.run(default)`).
  `FieldError` then `FieldDescription v-else`. The pane stays instant like the other Studio MCP panes
  (no draft); it does not take `SettingsPaneProps`.
- Guard: none (semantic; C1 shows the near cluster).
- Visible change: yes (reset button; description hidden while the error shows).
- Specs: `studio/kept-variables.spec.ts`, `studio/studio-mcp-settings.spec.ts`.
- Wave 1 (`settingsDomain.ts` constants); wave 2 (pane, with F2.7 in the same file).

### F2.11 [DRIFT] MIGRATE: "Not set" key owned by docker

- Sites: key `docker.edit.unset.status` rendered by `packages/workbench/src/components/SecretInput.vue:55`,
  `packages/workbench/src/config/ConfigSecretsSection.vue:119`,
  `packages/docker-ui/src/lib/editDiff.ts:158`. Workbench pieces borrow a docker key (iteration 1
  moved `docker.logs.tail.option` for the same reason).
- Change: move to `common.unset.status`; swap all three call sites in one commit.
- Guard: none (removed key fails typecheck).
- Visible change: no.
- Specs: `api-agent-access.spec.ts`, `ade-agent-settings.spec.ts`, docker edit specs (grep
  `docker-edit`).
- Wave 1.

## PARK (orchestrator adds SPEC row; next free number at write time)

- P687 Hash-guarded draft editor model. Draft, base hash, Reload and Overwrite on `E_CONFLICT` are
  implemented about ten times: `src/ade/v2/agents/useAgentFileEditor.ts`,
  `src/ade/v2/workflows/AdeYamlPane.vue:42-136` (near line-for-line copy; C3 `0c13f97584`,
  `fa812f52d4`, `7f44f4d16e`), `AdeWorkflowGraph.vue`, `AdeLibraryStepForm.vue`,
  `src/ade/v2/state/adeWorkflowDraft.ts`, `src/repo/RepoFilePane.vue`, `src/views/repo/useFileEdit.ts`
  with `src/state/fileEdit.ts`, `packages/workbench/src/config/useConfigFormDraft.ts`,
  `packages/workbench/src/state/llmPromptDrafts.ts`, `packages/docker-ui/src/state/dockerEdit.ts`.
  Which composable owns a hash-guarded draft across modules (and how it relates to P685's dialog
  shell over `useConfigFormDraft`) is a design decision, not a UI move. Merging only the ADE pair now
  would be churn for that decision.

## Waves and streams

Wave-2 site count: Studio 18, Space/ADE/git/shell/docker 16 (34 < 40). Default per master plan: one
sequential wave-2 Sonnet. The user cap allows two; the split below is safe if the orchestrator wants
it (no ordering dependency, disjoint files).

### Wave 1 (one Sonnet, sequential, commits per finding group)

Wave 1: done cfd18dda3

1. `packages/theme`: `icons.ts` (`ICON.copied`, F2.1), `format.ts` (`MASKED_TEXT`, F2.2),
   `components/MaskedValue.vue` (F2.2), `components/ui/button/index.ts` (`link-meta`, F2.9),
   `i18n/en.json` (F2.5 `common.expired.status` add plus `workbench.approvals.expired.status` remove,
   F2.6 `common.expiresIn.status` remove, F2.11 key move).
2. `packages/workbench`: `components/CopyIconButton.vue` (F2.1), `components/QueryState.vue`
   (F2.7).
3. Workbench sites: F2.1 `Identifier.vue`, `OpLogPanel.vue`, `OpLogDetail.vue` x2; F2.2
   `RunPreview.vue`, scenario `automationsSecrets.ts`; F2.3 `RunSecretsSection.vue` and the scenario;
   F2.5 `ApprovalItemCard.vue` key swap; F2.7 `MemoryPanel.vue`, `MemoryStart.vue`,
   `AgentMcpSection.vue`, `CheckHookSection.vue`, `McpToolsField.vue`, `ScheduleConfirmDialog.vue`,
   `RunScriptDialog.vue`; F2.9 `ApprovalPairs.vue`; F2.11 `SecretInput.vue`,
   `ConfigSecretsSection.vue`.
4. Moves that keep the tree compiling: F2.5 `src/studio/kept/keptExpiry.ts` (created) and the key
   swap in both kept rows; F2.6 PairingRequestDialog key swap (deleted key) done here with the
   `formatElapsed` change (one line, `src/workbench/`); F2.11 `packages/docker-ui/src/lib/editDiff.ts`
   key swap.
5. `src/state/settingsDomain.ts` constants (F2.10), schema and both defaults use them.
6. ARCHITECTURE rule lines (section below). No guard lines.
7. `bun run typecheck`, `bun run lint`, specs of every touched surface. Commit.

### Wave 2

Wave 2: done 2efc3aa51

| Stream | Owns this iteration | Sites |
|--------|---------------------|-------|
| A Studio | `src/studio/**` | F2.1 RawExchangePane, DescribeView, KeptValueView; F2.2 VariableRow, VariableHistoryMenu, KeptValueView, CredentialPastePanel; F2.4 KeptVariableRow, KeptEntryRow; F2.5 both kept rows adopt `useKeptExpiry`; F2.7 StudioMcpKeptPane, ConnectionMcpTables, ConnectionMaskReview; F2.8 KeptVariableRow; F2.9 ProofOpenWindow, PendingResponse (only if P677 landed); F2.10 StudioMcpKeptPane |
| B Space, ADE, git, shell, docker | `src/{ade,repo,hostview,views,openedfiles,components,workbench}/**`, `packages/git-ui/src/**`, `packages/docker-ui/src/**` | F2.1 CliToolsPane, AdeNoteEntry, AdeLinkCard, AdeBranchPanel x2, InspectView, ContainerOverview, ReviewCommentsPane; F2.2 AdePortImportDialog, ContainerOverview; F2.4 AdeShelfRow, AdeAllSessionRow, ContainerSizeSection, EngineDiskSection; F2.7 AdeLinkStrip |

- One sequential implementer: run A then B in one worktree, commit per finding group.
- Two streams: worktree each off the wave-1 commit. Shared files: none in wave 2 (no catalog edits:
  F2.3's key removals are wave 1). Specs under `apps/kira-space/tests/ui/` belong to the stream owning
  the surface; `studio/kept-variables.spec.ts` is A's.
- Never touched in wave 2: `scripts/check-ui-primitives.sh`, `docs/ARCHITECTURE.md`,
  `packages/theme/src/**`, `packages/workbench/src/**`, `src/state/settingsDomain.ts`.
- Each stream runs U171-U176 by hand (`grep -Pzl` with the script's `PRE`) over its dirs and reaches
  zero, then `bun run typecheck`, `bun run lint` and its specs.
- A conflict at rebase means the ownership table was wrong: stop and report.

### Wave 3 (one Sonnet, after rebase onto the chapter branch)

1. Add header `# Guards U171-U176: docs/HISTORY.md (v0.3.18).` and the six lines below to the
   `GUARDS` heredoc (unquoted: `${TOK}` expands; escape a literal `$`; named groups only).
2. Edit U130's exempt column: `VariableRow.vue|VariableHistoryMenu.vue` becomes `MaskedValue.vue`.
3. Run every new guard over the whole tree as it stands, P677 files included if landed. Fix each
   leftover offender in the same commit (expected: `PendingResponse.vue` for U175 if wave 2 skipped
   it; any copy button, mask, `useRelativeTime`, `<mark>` or QueryState error slot P677 added).
4. Test each regex: zero hits on the tree, one hit on a planted offender in a scratch copy.
5. `bun run lint:rules`; commit. Remove worktrees, push.

```
U171@@v@@CopyIconButton.vue@@hand copy icon button (use CopyIconButton)@@(?<![\w:-])icon="copy"|:icon="(?:ICON\.copy|copied\b[^"]*)"
U172@@vt@@format.ts@@hand masked value (use MaskedValue, MASKED_TEXT)@@•{4}|'•'\.repeat\(
U173@@v@@RelativeTime.vue|AdeReviewSync.vue|AdeSessionTabTip.vue|AdeAgentsStatusRow.vue@@standalone instant via useRelativeTime (use RelativeTime)@@\buseRelativeTime\(
U174@@v@@HighlightedText.vue@@hand mark element (use HighlightedText)@@<mark[\s>]
U175@@vt@@@@hand meta link classes (use Button variant link-meta)@@${TOK}decoration-border(?![\w-])
U176@@v@@@@QueryState error slot with hand Retry or Alert (use QueryState errorLabel, errorTestid)@@#error="\{[^"]*\bretry\b|#error="[^"]*"\s*>\s*<Alert\b
```

U176 needs the multi-line match `grep -Pz` gives the loop; confirm `RunLog.vue` and `AdeRunLog.vue`
stay at zero.

### ARCHITECTURE (wave 1 writes rules, wave 3 updates the guard range)

- Guard range line `U83-U176`; PERMANENT count stays 53.
- An icon-only copy button is `CopyIconButton` (workbench): `ICON.copy`, `ICON.copied` for
  `TIMING.copiedFeedback`, error tone on failure. U171.
- A hidden secret value is `MaskedValue` (`MASKED_TEXT`, 8 bullets, subtle, wide tracking); composed
  text uses `MASKED_TEXT`. U172. U130's masked-data tracking exemption names `MaskedValue`.
- Config secrets rule names run secrets: API, ADE and run file secrets render `ConfigSecretsSection`.
- Extend the RelativeTime line: `useRelativeTime` only inside composed text (U173).
- Marks render through `HighlightedText` (`markClass` for non-search meaning). U174.
- An approval-card meta link is Button `link-meta`. U175.
- QueryState inline load failure shows Retry when retryable; `errorLabel` and `errorTestid` replace
  the text and testid; an `error` slot only for a host that renders the failure inside its own frame
  (log views). U176.
- Countdown line: an expiry clock reads `common.expires.status` with `formatElapsed` under an hour of
  life; a TTL counted in minutes or hours (key TTL, kept variable) reads `formatCompactDuration` with
  `ceil`.
- Numeric settings leaf: range and default constants live in `settingsDomain.ts`; head row with
  `ResetToDefaultButton`; `FieldError` replaces `FieldDescription`.

## Tests

- Pure refactors (F2.3 markup aside, F2.5, F2.8, F2.9, F2.11): no new spec; ledger specs plus
  `bun run typecheck`, `bun run lint`.
- `CopyIconButton` has behaviour (feedback timing, error tone): one targeted UI spec
  `apps/kira-space/tests/ui/copy-icon-button.spec.ts` on a real host (op log detail command copy or
  CLI tools pane): click shows the check glyph, it reverts after `TIMING.copiedFeedback` under a fake
  clock, a rejected clipboard write shows error tone. Renderer-only: no flow half.
- `QueryState` inline Retry: extend an existing spec that already forces a load failure
  (`studio/kept-variables.spec.ts` error case) to click `query-retry` and assert the list renders.
  Renderer-only.
- F2.3 changes behaviour (Delete confirm, dialog entry): update the automations secrets scenario; the
  backend call (`setSecrets` with `null`) is unchanged, so no flow test.
- F2.6: the expiry value crosses IPC as `expiresAtMs`; only display changes, no flow test.
- Visual specs are expected to fail on visible changes; chapter end re-records.

## Visible-change list (chapter-end visual re-record)

F2.1 check feedback and error tone on 9 more copy buttons. F2.2 masks: kept, ADE port, run preview 4
to 8 bullets; subtle wide tracking on kept, ADE port, docker env. F2.3 run file dialog secrets
section (list, set dialog, Delete). F2.4 tooltips on ADE shelf and all-sessions ages and kept ages.
F2.6 pairing expiry clock. F2.7 Retry beside retryable inline load errors; memory panel and start
failure block; settings and dialog load errors as InlineState. F2.10 kept retention reset button and
description swap.

## Dismissed clusters

Key plus reason. Iteration 1's dismissed families still hold; a new key not listed here sits in one
of them (C2 generic layout utility sets; C3 per-kind view stores P683, git-core/git-ipc mirrors,
Monaco and SlickGrid hosts, connection defaults fixtures, import lists).

### C1

- `6412b92d02`, `8dc3af3728`, `74c3222998`, `bb9700f51f`, `1d3dae0a42`, `6fc1e4f2d0`, `3047589af3`:
  settings `Field` blocks, now with `ResetToDefaultButton`; identical composition, uniform result
  (F2.10 brings the one divergent field in line).
- `c4864e667f`: both toasts already render `PendingToast`; slot text differs by job.
- `7f769abdbe`: HTTP and gRPC save action, 2 sites, identical (P683 family).
- `6b8301dcc1`: ADE "Switch to YAML" alert, 2 sites one module, identical (iteration 1 kept it).

### C2

- `31f45724c4`: F2.9. `9b64b1ff02`, `ce3883fa28`, `ff40650184`, `70ce875566`, `78509420ea`,
  `2f1baaf684`, `7ac1fa0e11`, `09949c89fc`, `1f85554ed7`, `511d91c4e8`, `a0dc4ca87d`, `c8df8ffe7d`,
  `fbd60e6b38` and the rest touching P676/P678 files: generic list and stack utility sets.
- `44cc13a165`, `63846e8141`, `b22140e403`, `afc4116e57`, `c1dc8df7f3`, `5822424c79`, `7e71334d77`,
  `23f103fb56`, `5dd882dd4a`, `9c397683aa`: inside iteration 1's shared pieces.

### C3

- `29a53bdcc5`: F2.5. `0c13f97584`, `fa812f52d4`, `7f44f4d16e`, `2cd4f1017a`, `18ddd42f95`,
  `8d5b79b03e`, `0cb1b1ccc6`: P687 (and P685 for config dialogs).
- `8e5b41322e`, `52b9a69264`, `ce64cfd355`: dialog footer markup and import lists of shared pieces.
- `b14b3cbeda`, `e11c8a0135`: docker callers of `VirtualList`.
- `518bdf9800`, `3a786b628f`: `MemoryPanel` and `OpLogPanel` use `useVirtualRows` plus
  `useListNavigation` with `measureElement`: variable-height rows, the U164 DocumentView reason;
  `VirtualList` has fixed rows. U164 not widened.
- `5d7fe16716`, `678198b3b0`: cluster list vs connection tree drag handlers; different drop rules
  (tree nesting), drop indicator already shared (F1.12).
- `84a37dac9d`: kube vs docker log stream open, P684.
- `77f36a1258`: both restore dialogs render workbench `RestoreDialog`.
- `40ccef850c`: both ADE confirms render one shared confirm component.

### C6

- `a0dbdf0479` `terminal`: open-a-terminal concept, not take over; glyph shared with `ICON.takeOver`.
- `be366ccd96` `empty-window`: title bar new window, not open in new tab.
- `efa498cb57` `debug-continue`: git conflict banner Continue (`git rebase --continue`), git's verb.
- `4abb3b6c4f` rest: `KeptEntryRow` trash for Clear destroys the value (`ICON.delete` rule); verb
  Clear matches Clear all for the same IPC call.

### Sweep

- Kept rows in card (`KeyValueList`) vs settings (row): two surfaces, same fields, different layout
  by surface kind.
- Truncation notes (`studio.common.truncated.status`, `common.truncated.status`,
  `studio.dbMcp.preview.truncated.status`): different facts (value cut, log head dropped, row limit).
- Reveal toggles (`eye` / `eye-closed`): consistent semantics at every site.
- `KeptVariablesPanel` Manage link: navigation link, same as the HTTP panes' settings links.
- `JsScriptBlock`: `SectionHeading` plus `FlushEditor`, already shared pieces; one component, two
  callers.
