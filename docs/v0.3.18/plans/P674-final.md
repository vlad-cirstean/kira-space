# P674 Part 5: UI uniformity audit, final whole-app scan

Auditor ledger, wave plan, stream table and guard list for the final scan. Master plan: `P674.md`
("Final scan Part"). Earlier ledgers: `P674-iter1.md`, `P674-iter2.md`, `P674-iter3.md`.
Implementers tick each entry `done <sha>` as it lands.

## Base and tool report

- Base commit: `9428ea80c` (`chapter/v0.3.18`). In tree since iteration 3's scan (`8adcffe4d`): P679
  (`UserInputsPanel`, `useKeptReveal`, `KeptValueView` Reveal and Copy, "Supplied by you", origin,
  Approve gating `blankInput`), P680 Part 2 (`DbScriptBlock`, `DbResponseBody` on `ResultReview`,
  `StagedKeepList`, `DbResponseDetails` byte rows, `PendingResponse` `field`), iteration 3 waves 1-3.
- `bun run audit:ui --out <scratchpad>/auditF.md`, default thresholds, 5.7 s. Scan set 1619 files
  (737 `.vue`, 882 `.ts`). Report not committed; rerun to reproduce.
- Clusters per channel: C1 42, C2 196, C3 346, C4 20, C5 29, C6 114, C7 24 (771 total).
- Key diff against the iteration 1, 2 and 3 reports (kept in the scratchpad as `audit*.md.keys`): 8
  new keys (C1 2, C3 6), all in P679/P680 Part 2 approval files. Every cluster with a site in a file
  P679 or P680 Part 2 changed was re-read; all other new-file sites sit in families dismissed before.
- Hand sweep (channel `S`), like against like, on P676-P680 surfaces against the same job elsewhere:
  kept "Will keep" lists, transform script blocks, held-response bodies, masked-value reveal
  toggles, approval footers (desktop and phone), protocol status chips.
- All-subject pass: every v0.3.17 family (forms and labels, buttons and dialogs, menus, popovers,
  tooltips, headings, key-value, cards, status and tones, tabs and choice, states, icons, type,
  spacing, radii, frames) against `docs/ARCHITECTURE.md` "UI uniformity rules (P672, P674)", plus the
  spec row's look-for list. `check-ui-primitives.sh`, `check-copy.ts`, `check-catalog.ts` pass on the
  base. Only the findings below survive.
- Iteration 3 family recheck, whole tree:
  - Protocol status chip: no HTTP status rendered outside `HttpStatusBadge`; gRPC status has the same
    hand pair in two places with different text (F4.4).
  - JSON display: no `JSON.stringify(JSON.parse(` left; the remaining `JSON.stringify(_, null, 2)`
    (`ConsoleResultGrid` key-value copy) serialises a built object of strings, no number loss.
    `PendingResponse` "As sent" shows the agent's exact bytes unindented by design (comment names
    it). Nothing to fix.
  - Catalog ownership: shell dirs outside U182's table borrow 13 keys from module namespaces (F4.3).
- Discovery: this worktree has no codegraph index. `codegraph_explore` ran on the
  `/home/user/kira-space` index (branch `chapter/v0.3.17`, stale: predates P676 and every later
  phase; its on-disk source is the main checkout, not this worktree) for pre-existing symbols
  (`SecretInput`, `TooltipDisabledTrigger`, `TooltipIconButton`, `safeKind`, `ApprovalItemActions`,
  `PhoneApprovals`). Current source of every candidate was re-read from this worktree.
- Outcome: 7 findings (2 EXTRACT, 4 MIGRATE, 1 GUARD-GAP MIGRATE), 1 PARK, rest DISMISS. Guards
  U183-U187 in `check-ui-primitives.sh` (5 lines); U182 extended (module table, 5 pairs). PERMANENT
  count stays 53.

## Ledger

Paths drop `apps/kira-space/frontend/` for `src/` and `mobile/`; `packages/` paths are repo
relative. Specs: grep the testids under `apps/kira-space/tests/ui/` and `tests/mobile/`; named files
are known hits.

### F4.1 [DUP] MIGRATE: "Will keep" list onto StagedKeepList

- Sites: `src/studio/approvals/ApiResponseBody.vue:80-88` (hand section) vs
  `src/studio/approvals/StagedKeepList.vue` (P680 Part 2, used by `DbResponseBody.vue:67`). Audit
  C1 `10a879c5d5`. P680 Part 2 Result names the swap as pending.
- Change: `ApiResponseBody` `#after` slot renders
  `<StagedKeepList v-if="keep.length" :keep="keep" testid="api-review-keep" />`; drop the
  `SectionHeading`, `KeptNameMark`, `KeptValueView`, `keptTypeLabel` imports if unused. Testids
  unchanged (`api-review-keep`, `api-review-keep-<name>`).
- Guard: U183 `vt`, exempt `StagedKeepList.vue`, regex `studio\.kept\.willKeep\.label`.
- Visible change: no.
- Specs: `studio/approvals-api-kept-review.spec.ts`, `studio/approvals-db-script-review.spec.ts`.
- Wave 2, stream A.

### F4.2 [DUP] EXTRACT: transform script block with kept marks

- Sites: `src/studio/approvals/ApiRequestBody.vue:124-145` (transform `JsScriptBlock` with
  `keptScriptRanges(scriptSites, 'transform')`, "may keep" line of kept marks, reads note) and
  `src/studio/approvals/DbScriptBlock.vue` (P680 Part 2: same three parts, same keys, same classes,
  used by `DbQueryBody.vue:37`, `DbExemptionBody.vue:37`).
- Shared home: rename `DbScriptBlock.vue` to `src/studio/approvals/TransformScriptBlock.vue`
  (`git mv`), generalised.
- Change: props `label: string`, `script` (the shared `transformViewSchema` shape, `source` and
  `mayKeep`; both payloads already parse through it, `packages/shared/domain/dbmcp.ts:129`: export
  its type), `kept: readonly KeptUse[]`, `testid: string` (prefix: `api-approval` /
  `db-approval`), `scriptTestid: string`. Renders the current `DbScriptBlock` template with
  `scriptTestid` on `JsScriptBlock`, `${testid}-may-keep` and `${testid}-script-reads`. API site
  passes `script=payload.script.transform`, `scriptTestid="api-approval-script-transform"`,
  `kept=kept` (all uses, as today), the transform label key; DB sites pass
  `script=payload.script`, `scriptTestid="db-approval-script"`, `kept=payload.kept`, the query
  label key. `ApiRequestBody` drops `scriptSites`, `mayKeep`, `readsInScript` when
  unused (prepare block keeps its own `keptScriptRanges` call over the kept sites only). Testids
  unchanged. Highlight condition stays `kept.length || mayKeep.length`.
- Guard: U184 `vt`, exempt `TransformScriptBlock.vue`, regex
  `studio\.kept\.mayKeep\.label|studio\.kept\.scriptReads\.hint`.
- Visible change: no.
- Specs: `studio/approvals-api-kept-script.spec.ts`, `studio/approvals-api-script.spec.ts`,
  `studio/approvals-db-script.spec.ts`, `studio/dbmcp-approval-script.spec.ts`.
- Wave 2, stream A (component rename plus 3 sites in one commit; knip fails on an unused file).

### F4.3 [GUARD-GAP] MIGRATE: catalog ownership in the shell dirs

- Rule: ARCHITECTURE catalog ownership line (U182). Iteration 3 left `src/workbench/`, `src/state/`,
  `src/shortcuts/`, `src/bridge/`, `src/App.vue`, `src/main.ts` and `packages/workbench/src/` (except
  automations, memory) out of the table. Scan of those dirs: 13 borrowed keys of shared vocabulary
  plus shell hosting of module UI.
- Module table additions in `scripts/copy/ownership.ts` (`moduleOf` takes the first match: put the
  specific prefix before the catalog-wide one):
  - `apps/kira-space/frontend/src/workbench/`, `src/state/`, `src/shortcuts/`, `src/bridge/`,
    `src/App.vue`, `src/main.ts` to module `shell`, `ALSO_OWNS.shell = ['space']` (the shell is the
    Space app's frame; 176 `space.*` reads, no `shell` catalog namespace exists).
  - `packages/workbench/src/llmPrompts/` to `llmPrompts`.
  - `packages/workbench/src/` to `workbench`.
- Shell pairs (`exemptions.ts` `OWNERSHIP_PAIRS`, module `shell`, reason each):
  `studio.settings.` (settings dialog nav and app panes host Studio fields: `SettingsDialog`,
  `AdvancedPane`, `AppearancePane`), `studio.mode.` (mode switcher names Studio's modes:
  `modes.ts`, `SettingsDialog`), `studio.operations.` (op log hosts Studio operation rows:
  `OperationsPanel`), `studio.tab.` (`host.ts` wires Studio tab kinds' incognito marker),
  `llmPrompts.nav.` (settings nav names the prompts pane).
- Key moves (English byte-identical; every caller of the old key swaps, owner included, one commit
  per group; final names may adjust to pass check-catalog C1/C2, record them in the tick line):
  - `studio.keyValue.rows.aria` to `common.field.rows.label` (op log column and detail row,
    `KeyValueRowsTable` aria).
  - `studio.common.nodeKind.connection.label` to `common.field.connection.label` (`nodeKindLabel.ts`
    map entry, `OperationsPanel`).
  - `git.search.field.subject.label` to `common.field.subject.label`;
    `git.search.field.author.label` to `common.field.author.label`; `git.toolbar.pull.label` to
    `common.action.pull.label`.
  - `docker.metric.cpu.label` to `common.metric.cpu.label`; `docker.exec.close.tooltip` to
    `common.tab.close.tooltip` (shared `TabStrip` close).
  - `automations.run.state.done.status` to `common.state.succeeded.status` (op log, script run and
    terminal tab kinds, `runText.ts`).
  - `automations.dialog.script.noCollection.option` to `common.collection.none.option`
    (`collectionMenu.ts`).
  - `memory.start.event.source.ui.label` to `common.module.app.label`;
    `mobile.shell.heading.title` to `common.module.agents.label` (ADE mode label, phone shell,
    metrics); `studio.settings.dbMcp.scripts.legend.label` to `common.module.scripts.label`;
    `studio.settings.nav.studioMcp.label` to `common.module.studioMcp.label` (settings nav, prompts
    pane area, metrics).
  - Before each move grep the old key and its parent prefix as a template literal across `apps`,
    `packages`, `internal` and specs (none found at planning; `nodeKindLabel.ts` and
    `memory/labels.ts` hold literal maps).
- Guard: U182 extended (table, `ALSO_OWNS`, pairs); no new id. Unused or stale pair fails the run.
- Visible change: no (English unchanged).
- Specs: none beyond `bun run typecheck`, `bun run lint` (check-catalog `--unused`, C3),
  `bun run lint:rules`.
- Wave 1 (moves); wave 3 (table and pairs).

### F4.4 [DRIFT] EXTRACT: gRPC status badge

- Sites: `src/studio/views/grpcrequest/ResponsePane.vue:300-310` (`TextTooltip v-if="codeHint"`
  around `Badge data :variant="grpcCodeClass(code)"` "NAME (code)", plus a duplicate bare `Badge`
  in `v-else`), `src/studio/views/grpcrequest/CallHistoryList.vue:176-178` (same pair, text "NAME"
  only). F3.1's gRPC sibling: one status, two texts.
- Shared home: `src/studio/components/GrpcStatusBadge.vue` beside `HttpStatusBadge.vue`.
- Change: `defineOptions({ inheritAttrs: false })`; props `code: number`, `codeName: string`.
  Renders `<TextTooltip :text="grpcCodeHint(code)"><Badge data :variant="grpcCodeClass(code)"
  v-bind="$attrs">{{ codeName }} ({{ code }})</Badge></TextTooltip>` (TextTooltip renders the bare
  badge when the hint is empty, so the pane's `v-else` copy goes). Pane keeps
  `data-testid="grpc-status-chip"` and drops `codeHint` and the imports if unused.
- Guard: U185 `v`, exempt `GrpcStatusBadge.vue`, regex `\bgrpcCodeClass\(|\bgrpcCodeHint\(`.
- Visible change: yes (gRPC history chip reads "OK (0)").
- Specs: `studio/grpc-request.spec.ts`, `studio/api-ui-consistency.spec.ts`,
  `list-keyboard.spec.ts` (grep `grpc-status-chip`, `grpc-history`).
- Wave 2, stream A.

### F4.5 [DRIFT] EXTRACT: masked value reveal toggle

- One action (show a hidden value) with three wordings, two icon rules and pressed state on some:
  - `packages/workbench/src/components/SecretInput.vue:111-127`: "Show value" / "Hide value"
    (`common.secret.show.label`, `common.hideValue.label`), eye flips, `pressed`.
  - `packages/docker-ui/src/components/ContainerOverview.vue:149-156`: "Reveal value" /
    "Hide value" (`docker.env.revealValue.tooltip`), eye flips, no `pressed`.
  - `src/studio/api/VariableRow.vue:42-46,82-90`: "Reveal" then "Show value" / "Hide value", static
    aria "Reveal", icon always `eye`, `pressed`.
  - `src/studio/api/VariableHistoryMenu.vue:77-84`: "Reveal" / "Hide" (`common.action.reveal.label`,
    `common.action.hide.label`), eye flips, `pressed`.
  - `src/studio/kept/KeptValueView.vue:88-98` (P679): "Reveal" / "Hide", eye flips, no `pressed`.
  - Not sites: `CredentialPastePanel` show text and show password (named objects), item visibility
    toggles (repo hide, filter disable, log open).
- Shared home: `packages/theme/src/components/ValueRevealButton.vue` (presentational).
- Change: `defineOptions({ inheritAttrs: false })`; props `shown: boolean`, `label?: string`
  (tooltip override, SecretInput's not-revealable text). Renders `TooltipIconButton`
  `:icon="shown ? 'eye-closed' : 'eye'"`, `:label="label ?? (shown ? hide : show)"`,
  `:aria-label="shown ? hide : show"`, `:pressed="shown"`, `v-bind="$attrs"` (testid, click,
  disabled, `disabled-trigger`, `type` fall through to `TooltipIconButton`). Show is
  `common.secret.show.label`, hide `common.hideValue.label`. Sites: SecretInput
  `shown=visible`, `label` only when not revealable; ContainerOverview `shown=revealed.has(i)`
  (delete `docker.env.revealValue.tooltip`); VariableRow `shown=!masked()` (drop
  `valueToggleLabel`; the first click still emits `reveal`); VariableHistoryMenu
  `shown=!isMasked(entry)`; KeptValueView `shown=secretReveal.value.value !== null`,
  `:disabled="secretReveal.busy.value"`. Testids unchanged. `common.action.reveal.label` and
  `common.action.hide.label` keep their other callers (confirm dialogs, proof, repo hide).
- Guard: U186 `vt`, exempt `ValueRevealButton.vue`, regex
  `common\.secret\.show\.label|common\.hideValue\.label|docker\.env\.revealValue|(?s:import MaskedValue\b.*?(?:icon="eye"|'eye'))`
  (whole-file record under `grep -Pz`; the last branch fails a file that masks a value and draws
  its own eye).
- Visible change: yes ("Show value" / "Hide value" on kept, variable row, variable history and
  docker env toggles; variable row eye flips when shown; pressed look on kept and docker env).
- Specs: `studio/kept-variables-user.spec.ts`, `studio/http-variables.spec.ts`,
  `studio/api-secret-reveal-isolation.spec.ts`, `studio/secret-reveal.spec.ts`,
  `studio/credential-reveal.spec.ts`, `git-credential-relay.spec.ts` (grep `-reveal` testids).
  No spec asserts the old label text.
- Wave 1 (component plus SecretInput, one commit); wave 2 stream A (3 Studio sites), stream B
  (ContainerOverview plus key deletion; the deletion is a distinct key, additive hunk).

### F4.6 [DUP] MIGRATE: approval footer requester and labels

- Sites: `packages/workbench/src/approvals/ApprovalItemActions.vue:23-33` (origin join
  `[o.client, o.cwd, o.task, o.step, o.source]` behind `safeKind`, labels behind `safeKind`,
  `unreadable` hides Approve), `mobile/components/PhoneApprovals.vue:31-36,73,84` (same join and
  `def.requester`, `labels` called bare: a throwing kind breaks the phone list while the desktop
  falls back).
- Shared home: `packages/workbench/src/approvals/approvalFooter.ts` beside `safeKind.ts`.
- Change: `approvalRequester(def, item): Safe<string>` (kind `requester` or the origin join) and
  `approvalLabels(def, item): Safe<{ approve: string; deny: string } | undefined>`. Desktop computes
  through them (no behaviour change). Phone reads `.value`, and hides Approve when either `failed`
  (desktop rule). Testids unchanged.
- Guard: U187 `vt`, exempt `approvalFooter.ts`, regex `\.client,\s*\w+\.cwd,\s*\w+\.task\b`.
- Visible change: no.
- Specs: `tests/mobile/approvals.spec.ts`, `approvals-alignment.spec.ts`, `studio/approvals-api.spec.ts`.
- Wave 1 (helper plus desktop), wave 2 stream B (phone).

### F4.7 [DRIFT] MIGRATE: composed kept copy in P679 code

- Rule: ARCHITECTURE copy line "Never build a string by concatenating translated parts". Sites:
  `src/studio/approvals/UserInputsPanel.vue:44` (`${t('studio.kept.input.replace.label')}: ${i.label}`),
  `src/studio/workbench/settings/KeptEntryRow.vue:47` (`${t('studio.kept.origin.label')} ${entry.origin}`).
- Change: `studio.kept.input.replace.label` becomes "Enter a new value: {label}" with
  `{ label: i.label }`; `studio.kept.origin.label` becomes "Goes only to {origin}" with
  `{ origin: entry.origin }`. Rendered text unchanged.
- Guard: none here. The tree holds 29 more composed sites `check-copy.ts` misses (template
  literal around `t(`); the composition policy and guard are P688 (PARK). A pattern at zero is not
  possible until then.
- Visible change: no.
- Specs: `studio/approvals-api-input.spec.ts`, `studio/kept-variables-user.spec.ts`.
- Wave 1.

## PARK (orchestrator adds the SPEC row; next free number at write time is P688)

- P688 Copy guard gaps: composed translations and catalog-free hint text. `check-copy.ts` claims
  "concatenated or composed translations" but misses a template literal around `t(` (29 sites
  after F4.7: `ade/v2/board/setupStatus.ts:14`, `reviewCode.ts:66`, `hostview/CommentComposer.vue:57`,
  `RawExchangePane.vue:121`, `ResponseHistoryList.vue:54`, `pendingChanges.ts:569`, `sqlHover.ts:45`,
  `ProjectTree.vue:319`, `confirmOverwrite.ts:10`, `CopyLogButton.vue:32`, `terminalRenderer.ts:90`,
  `ImportView.vue:32`, `DistillResult.vue:47`, `scriptVars.ts:192`, `RunPreview.vue:36`,
  `docker-ui/lib/editDiff.ts:180,354`, `git-ui/state/ops.ts` 7 sites, `liveAnnouncements.ts:270,283`).
  Needs a policy (sentence joins, label and data joins with " · ", unit suffixes) before a guard.
  Same phase: UI-visible English outside the catalog in `packages/shared/domain` (`statusHint`,
  `GRPC_CODE_HINTS`), shown as status chip tooltips; `check-copy.ts` does not scan
  `packages/shared`. Copy work across every module, not a UI uniformity fix.
- Still Planned: P682, P683, P684, P685, P687.

## Waves and streams

Wave-2 sites: Studio 9 (F4.1 1, F4.2 3, F4.4 2, F4.5 3), docker 1, mobile 1. Under 40: one
sequential wave-2 Sonnet by default; the two-stream split below is safe (no ordering dependency,
disjoint files).

Module-local pieces (`TransformScriptBlock`, `GrpcStatusBadge`) are created by the wave-2 stream
owning their module, in the same commit as their consumers (knip fails a new file with no
importer), as in iteration 3.

### File ownership

| Owner | Files |
|-------|-------|
| Wave 1 | `packages/theme/src/i18n/en.json`; every caller file of an F4.3 moved key (any module); `packages/theme/src/components/ValueRevealButton.vue` (new), `packages/workbench/src/components/SecretInput.vue`; `packages/workbench/src/approvals/approvalFooter.ts` (new), `ApprovalItemActions.vue`; `UserInputsPanel.vue` line 44, `KeptEntryRow.vue` line 47; `docs/ARCHITECTURE.md` rules |
| Wave 2 A Studio | `src/studio/approvals/ApiResponseBody.vue`, `ApiRequestBody.vue`, `DbScriptBlock.vue` to `TransformScriptBlock.vue`, `DbQueryBody.vue`, `DbExemptionBody.vue`, `packages/shared/domain/dbmcp.ts` (type export only), `src/studio/components/GrpcStatusBadge.vue` (new), `src/studio/views/grpcrequest/ResponsePane.vue`, `CallHistoryList.vue`, `src/studio/api/VariableRow.vue`, `VariableHistoryMenu.vue`, `src/studio/kept/KeptValueView.vue`, Studio UI specs |
| Wave 2 B docker and mobile | `packages/docker-ui/src/components/ContainerOverview.vue`, `mobile/components/PhoneApprovals.vue`, `tests/mobile/approvals.spec.ts`; `en.json` deletion of `docker.env.revealValue.tooltip` only |
| Wave 3 | `scripts/check-ui-primitives.sh`, `scripts/copy/ownership.ts`, `scripts/copy/exemptions.ts`, `docs/ARCHITECTURE.md` guard range and catalog line |

Shared file: `en.json` (wave 1 moves and rewords; wave 2 B one key deletion, distinct key). Wave 1
lands before wave 2 starts.

### Wave 1 (one Sonnet, sequential, commits per finding group)

1. F4.3: per namespace group (studio, git, docker, automations, memory and mobile), move keys in
   `en.json` (keep sorted: `biome check --write`), swap every caller, `bun run typecheck`,
   `bun scripts/copy/check-catalog.ts --unused`, commit.
2. F4.7: reword two keys, swap two sites; commit.
3. F4.5: `ValueRevealButton` plus SecretInput; commit.
4. F4.6: `approvalFooter.ts` plus `ApprovalItemActions`; commit.
5. ARCHITECTURE rule lines (below). No guard lines. `bun run typecheck`, `bun run lint`, specs of
   touched surfaces. Tick entries.

### Wave 2

| Stream | Sites |
|--------|-------|
| A Studio | F4.1, F4.2 (rename plus 3 sites), F4.4 (component plus 2 sites), F4.5 (VariableRow, VariableHistoryMenu, KeptValueView) |
| B docker and mobile | F4.5 (ContainerOverview, key deletion), F4.6 (PhoneApprovals) |

- One sequential implementer: A then B in one worktree, commit per finding. Two streams: worktree
  each off the wave-1 commit.
- Never touched in wave 2: `scripts/**`, `docs/ARCHITECTURE.md`, `packages/theme/src/**` (except B's
  key deletion), `packages/workbench/src/**`.
- Each stream runs its guards by hand (`grep -Pzl` with the script's `PRE` and `TOK`) over its dirs
  and reaches zero (A: U183, U184, U185, U186; B: U186, U187), then `bun run typecheck`,
  `bun run lint` and its specs. A conflict at rebase outside `en.json` means the table was wrong:
  stop and report.

### Wave 3 (one Sonnet, after rebase onto the chapter branch)

1. Header `# Guards U183-U187: docs/HISTORY.md (v0.3.18).` and the lines below in the `GUARDS`
   heredoc (unquoted: escape a literal `$`; no backtick).
2. F4.3 table, `ALSO_OWNS`, shell pairs in `ownership.ts` and `exemptions.ts`; `check-copy.ts`
   header unchanged (U182 already named).
3. Run every new guard and `bun scripts/copy/check-copy.ts` over the whole tree; fix leftovers in
   the same commit.
4. Test each regex: zero hits on the tree, one hit on a planted offender in a scratch copy (U186:
   a scratch `.vue` importing `MaskedValue` with `icon="eye"`; U182: `t('git.toolbar.pull.label')`
   in a scratch `src/workbench/` file and in a scratch `packages/workbench/src/components/` file).
5. `bun run lint:rules`; commit. Remove worktrees, push.

```
U183@@vt@@StagedKeepList.vue@@hand Will keep list (use StagedKeepList)@@studio\.kept\.willKeep\.label
U184@@vt@@TransformScriptBlock.vue@@hand transform script kept marks (use TransformScriptBlock)@@studio\.kept\.mayKeep\.label|studio\.kept\.scriptReads\.hint
U185@@v@@GrpcStatusBadge.vue@@hand gRPC status chip (use GrpcStatusBadge)@@\bgrpcCodeClass\(|\bgrpcCodeHint\(
U186@@vt@@ValueRevealButton.vue@@hand value reveal toggle (use ValueRevealButton)@@common\.secret\.show\.label|common\.hideValue\.label|docker\.env\.revealValue|(?s:import MaskedValue\b.*?(?:icon="eye"|'eye'))
U187@@vt@@approvalFooter.ts@@hand approval origin text (use approvalRequester)@@\.client,\s*\w+\.cwd,\s*\w+\.task\b
```

### ARCHITECTURE (wave 1 writes rules, wave 3 updates the guard range)

- Guard range line `U83-U187` (rules line says `U110-U187`); PERMANENT count stays 53.
- A held result's "Will keep" list is `StagedKeepList`. U183.
- A transform script in an approval card with its kept marks, "may keep" line and reads note is
  `TransformScriptBlock`. U184.
- A gRPC status is `GrpcStatusBadge` (Studio): `grpcCodeClass` tone, `grpcCodeHint` tooltip, data
  font, "NAME (code)". U185.
- A masked value's show and hide toggle is `ValueRevealButton`: "Show value" / "Hide value", eye
  flips, pressed when shown. U186.
- An approval footer's requester and button words come from `approvalRequester` and
  `approvalLabels` on desktop and phone. U187.
- Catalog ownership line gains the shell: shell dirs read `space` plus shared namespaces, pairs
  `studio.settings.`, `studio.mode.`, `studio.operations.`, `studio.tab.`, `llmPrompts.nav.`;
  `packages/workbench/src` reads `workbench`, `llmPrompts` its own. U182.

## Tests

- Pure refactors (F4.1, F4.2, F4.3, F4.6, F4.7): no new spec; ledger specs plus
  `bun run typecheck`, `bun run lint`.
- F4.4, F4.5: presentational, no new behaviour (F4.5 keeps every site's own reveal logic):
  ledger specs; locator drift fixed in the spec. Renderer-only, no flow half.
- No value crossing IPC changes.
- Visual specs fail on visible changes; chapter end re-records.

## Visible-change list (chapter-end visual re-record)

F4.4 gRPC history status chip reads "NAME (code)". F4.5 reveal toggles on kept values, variable
rows, variable history and docker env read "Show value" / "Hide value", variable row eye flips,
kept and docker env toggles show pressed.

## Dismissed clusters

Key plus reason. Iteration 1-3 dismissed families still hold (C2 generic layout utility sets; C3
per-kind view stores P683, git-core/git-ipc mirrors, Monaco and SlickGrid hosts, fixtures, import
lists; C6 literal-icon and generic-concept families, including `4abb3b6c4f` clear icons now with a
`KeptEntryRow` site: delete of an entry, trash).

### New keys

- `10a879c5d5` C1: F4.1.
- `297af13e6d`, `31c9d30266` C3: `ApiResponseBody` and `DbResponseBody` held-response scaffolding
  (payload parse, request id slice, `usePendingResponse`, result slot). 2 sites one module, no
  visible divergence (the DB result is always JSON, so its `FlushEditor` fallback equals the API
  `ApprovalCode` branch in practice).
- `bcae2fda0d`, `5fe53de175`, `aea46766f2` C3: import lists.
- `b056ef545a` C1, `d445875afc` C3: `DbQueryBody` and `DbExemptionBody`, 2 sites one module,
  identical (iteration 3 seen; still no divergence).

### Sweep

- `kinds.ts` "keeps" risk tag (`queryScriptRisks`, `apiRisks`): 2 sites one file, identical output.
- Approve gated by a typed input (`ApprovalItemActions`): follows the disabled-reason rule
  (`TooltipDisabledTrigger`). Phone never shows Studio kinds (mobile kinds are the three Space
  kinds, `mobileweb/approvals.go` allowlists the same), so no phone input path.
- `UserInputsPanel`: `SecretInput`, `Disclosure`, `Field`; no hand piece.
- `DbResponseDetails` byte rows: `formatBytes` (workbench re-export of theme).
- Section heading to content gap (`gap-0.5` results and op log detail, `gap-1` approval card
  sections, `gap-2` settings sections and forms): one gap per surface kind, no rule names it.
- Protocol hint text in `packages/shared` and composed translations: P688.

## Carry-forward

- P688 (new, PARK above). P682, P683, P684, P685, P687 (still Planned).
- Known open items: none added.

## For the chapter-end code-review rounds

- P688 holds the copy-guard gap; reviewers should not re-flag composed `t(` sites or the shared
  domain hint strings as new.
- Held-response bodies (`ApiResponseBody`, `DbResponseBody`) and the two DB card bodies stay
  duplicated by choice (2 sites each, no divergence); a third kind should extract them.
- `PendingResponse` "As sent" renders the agent's exact bytes unindented, while `ResultReview`
  "Agent gets" indents through `indentJson`: intended (exact-bytes tab), not drift.
- The codegraph index used here is stale (`chapter/v0.3.17`); a reviewer tracing P676-P680 symbols
  re-reads the worktree source.
