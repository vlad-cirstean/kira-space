# P674 Part 4: UI uniformity audit, iteration 3

Auditor ledger, wave plan, stream table and guard list for iteration 3. Master plan: `P674.md`.
Earlier ledgers: `P674-iter1.md`, `P674-iter2.md`. Implementers tick each entry `done <sha>` as it
lands.

## Base and tool report

- Base commit: `8adcffe4d` (`chapter/v0.3.18`). In tree since iteration 2's scan (`cfcd9c5d6`): P677
  (`ResultReview`, `ApiResponseBody/View/Details`, `ReviewChoiceField`, `ReviewResultOption`,
  `pendingResponse.ts`, `prettyJson.ts`), P678 Part 2 (script kept marks, "Will keep"), P680 Part 1
  (`DbParamsList`, `KeptVariablesPanel` props, statement marks), iteration 2 waves 1-3.
- Not in tree, excluded: P679 (user-supplied values: `UserInputsPanel` and its edits to
  `ApiRequestBody`, `KeptValueView`, `KeptVariablesPanel`, `StudioMcpKeptPane`) and P680 Part 2 (DB
  script and review UI: `DbQueryBody`, `DbExemptionBody`, DB `PendingResponse` onto `ResultReview`).
  Final scan (Part 5) covers them. Wave 3 runs every new guard over the tree as it stands then and
  fixes leftovers in landed P679/P680 Part 2 files.
- `bun run audit:ui --out <scratchpad>/audit3.md`, default thresholds, 5.7 s. Scan set 1612 files
  (731 `.vue`, 881 `.ts`). Report not committed; rerun to reproduce.
- Clusters per channel: C1 42, C2 197, C3 344, C4 20, C5 29, C6 115, C7 24 (771 total).
- Key diff against iterations 1 and 2 reports: 6 new keys (C1 3, C3 2, C4 1). C2, C5, C6, C7 carry
  no new key. Every cluster (all channels) with a site in a file changed since `cfcd9c5d6` (79
  `.vue`/`.ts` files) was re-read; all sit in families dismissed before except the ones below.
- Hand sweep (channel `S`), like against like, on P677/P678 Part 2/P680 Part 1 surfaces against the
  same job elsewhere: HTTP status chips, response body notes (binary, truncated), JSON
  pretty-printing, kept name marks, docker measure sections, catalog key ownership.
- Discovery: this worktree has no codegraph index. `codegraph_explore` ran on the
  `/home/user/kira-space` index (branch `chapter/v0.3.17`, tip `3cb14644b`: stale, predates P676 and
  every later phase) for pre-existing symbols (`PendingResponse`, `TabbedContent`,
  `ConnectionMcpReview`, `ReviewResultOption`, `beautifyJson`/`beautifyWith`/`tryParseJson`,
  `statusClass`/`statusHint`, `copyAll` `prettyJson`, `useContainerSize`, `useEngineDiskUsage`).
  Current source of every candidate was re-read from this worktree.
- Outcome: 6 findings (3 EXTRACT, 1 MERGE, 1 MIGRATE, 1 GUARD-GAP MIGRATE), 0 PARK, rest DISMISS.
  Guards U177-U181 in `check-ui-primitives.sh` (5 lines), U182 in `scripts/copy/check-copy.ts`.
  PERMANENT count in `check-ui-primitives.sh` unchanged (53); U182 adds 4 named pair exemptions in
  `scripts/copy/exemptions.ts`.

## Concurrent streams

- P679 and P680 Part 2 may land while this iteration runs. Overlap: `ApiResponseBody.vue` (F3.4,
  F3.5), `KeptValueView.vue` (F3.5), `keptHighlight.ts` (F3.5), `KeptVariableRow.vue` (F3.4),
  `DbParamsList.vue` (F3.4, F3.6 key swap), `keptFormat.ts` (F3.6 key swap), `en.json` (key moves
  here, key additions there). Whichever lands second rebases; additive hunks except the named lines.
  P679/P680 Part 2 code built after wave 1 must use the moved keys and new pieces.

## Ledger

Paths drop `apps/kira-space/frontend/` for `src/` and `mobile/`; `packages/` paths are repo
relative. Specs: grep the testids under `apps/kira-space/tests/ui/` and `tests/mobile/`; named files
are known hits.

### F3.1 [DRIFT] EXTRACT: HTTP status badge

- Sites: `src/studio/views/httprequest/ResponsePane.vue:296-300`,
  `src/studio/views/httprequest/ResponseHistoryList.vue:232-239`,
  `src/studio/views/httprequest/ResponseDiffDialog.vue:276-284`, `:295-303`,
  `src/studio/views/httprequest/TimelinePane.vue:260-263`,
  `src/studio/approvals/ApiResponseView.vue:17-24,42` (P677).
- Drift: the five HTTP-pane sites render `TextTooltip :text="statusHint(s)"` around
  `Badge data :variant="statusClass(s)"`. The approval review hand-builds a second map
  (`>= 500` error, `>= 400` warn, 2xx ok, else neutral) through `TONE_BADGE`: 4xx is warn not error,
  3xx neutral not warn, 1xx neutral not info; no `data` font; no hint tooltip.
- Shared home: `src/studio/components/HttpStatusBadge.vue` (beside `ReviewChoiceField`; views and
  approvals both import `studio/components`, no biome boundary hit).
- Change: `defineOptions({ inheritAttrs: false })`; props `status: number`, `statusText?: string`.
  Renders `<TextTooltip :text="statusHint(status)"><Badge data :variant="statusClass(status)"
  v-bind="$attrs">{{ status }} {{ statusText }}</Badge></TextTooltip>` (attrs, testid included,
  land on the Badge). Sites pass `status` and `statusText`, keep their testids
  (`http-status`, `http-history-status-chip`, `http-diff-status-a`/`-b`,
  `${testid}-status`). `ResponsePane` drops its `hint` computed if unused; TimelinePane keeps its
  `v-if="hop.status > 0"` and failed-chip branch. `ApiResponseView` drops `tone`, `statusText`,
  `StatusTone`/`TONE_BADGE` imports and the wrapping `div` if it only held the badge.
- Guard: U177 `v`, exempt `HttpStatusBadge.vue`, regex `\bstatusClass\(|>=\s*500\b`.
- Visible change: yes (approval response status: 4xx red, 3xx amber, 1xx info, data font, hint
  tooltip).
- Specs: `studio/http-request.spec.ts`, `studio/http-history.spec.ts`, `studio/http-pipes.spec.ts`,
  `studio/http-timeline.spec.ts`, diff dialog specs (grep `http-diff-status`),
  `studio/approvals-api-response.spec.ts`, `approvals-alignment.spec.ts`.
- Wave 2 (component plus all 6 sites in one commit; knip fails on a file with no importer).

### F3.2 [DUP] EXTRACT: docker measured section

- Sites: `packages/docker-ui/src/components/ContainerSizeSection.vue:24-57`,
  `packages/docker-ui/src/components/EngineDiskSection.vue:51-80` (audit C1 `f7c4d6d80f`, C6
  `43ecd8746e`, `ec53a334ac`). Same job: a `DetailSection` measured only on demand
  (`enabled: false` query, `refetch`), taken-at time and refresh in actions, loading, not measured
  with Measure button, error, data.
- Drift: loading is hand `div` plus `Spinner` plus text vs `InlineState kind="loading"`; not
  measured is a bare `span` vs `InlineState kind="empty"`; error is `InlineState` vs `Alert`;
  refresh glyph literal `icon="refresh"` (ICON has `refresh`).
- Shared home: `packages/docker-ui/src/components/MeasuredSection.vue` (both consumers in
  docker-ui).
- Change: `<script setup lang="ts" generic="T">`. Props `title: string`, `testid: string` (prefix),
  `query: UseQueryReturnType<T | undefined, Error>`, `takenAt: (data: T) => string`,
  `measureTooltip: string`, `notMeasured: string`. Default slot `{ data: T }`. Attrs fall through to
  `DetailSection` (EngineDisk's `class="shrink-0"`). Actions: `RelativeTime` of `takenAt(data)`
  (`${testid}-taken`) and `TooltipIconButton :icon="ICON.refresh"` (`${testid}-refresh`, disabled
  while `isFetching`, click `query.refetch()`). Body, in order: error with no data
  `InlineState kind="error"` (`${testid}-error`); error with data kept (re-measure failed)
  `Alert variant="error"` with the message (`${testid}-error`), per the ARCHITECTURE rule "failed
  operation with content kept is Alert error"; `isFetching && !data` `InlineState kind="loading"`
  `docker.measuring.loading` (`${testid}-measuring`); `!data` row `flex flex-wrap items-center gap-3`
  with `InlineState kind="empty"` and the Measure `Button size="kira" variant="toolbar"`
  (`dashboard` glyph, `docker.measure.label`, `${testid}-measure`); else the slot. Hosts pass
  `testid="docker-size"` / `"docker-disk"`, their tooltip and not-measured keys, and keep only the
  data markup (`KeyValueList`, table and volumes). `useContainerSize` and `useEngineDiskUsage` drop
  `refresh` from their return when no caller is left (knip).
- Guard: U178 `vt`, exempt `MeasuredSection.vue`, regex
  `docker\.measuring\.loading|docker\.measure\.label`.
- Visible change: yes (container size loading and not-measured lines take InlineState look; disk
  first-measure failure shows InlineState instead of Alert).
- Specs: `studio/docker-disk.spec.ts`; extend it with one case: a re-measure that fails keeps the
  table and shows `docker-disk-error` (renderer-only, no flow half).
- Wave 2 (component plus both sites in one commit).

### F3.3 [DRIFT] MIGRATE: byte sizes and response body notes

- Rule: ARCHITECTURE "packages/theme/src/format.ts owns formatBytes". One fact, one wording.
- Sites, raw byte count instead of `formatBytes`:
  `src/studio/views/httprequest/ResponsePane.vue:487` (`studio.http.response.binary.status`,
  `{ count: response.bodyBytes }`, "{count} bytes of binary data"),
  `src/studio/views/shared/celleditor/detect.ts:376` (`base64Bytes.status`), `:381`
  (`hexBytes.status`).
- Sites, same fact under a second key: `src/studio/approvals/ApiResponseView.vue:56-66` (P677):
  binary body "Binary body, {bytes} not shown" (`studio.dbMcp.approval.api.responseBinary.status`)
  vs HTTP pane's binary text; truncated body "Cut at {bytes}"
  (`studio.dbMcp.approval.api.responseCut.status`) vs HTTP pane "Response truncated at {size}. The
  server sent more than that." (`studio.http.response.truncated.description`). Both are the masked
  response cut at Kira's response cap, `bodyBytes` the kept size.
- Change: `studio.http.response.binary.status` becomes "Binary body, {size} not shown" (no plural
  form) taking `{ size: formatBytes(bytes) }`; `ResponsePane` and `ApiResponseView` render it.
  `ApiResponseView` truncation renders `studio.http.response.truncated.description` with
  `{ size: formatBytes(bodyBytes) }` (keeps its subtle line: card surface, not a pane strip).
  Delete `studio.dbMcp.approval.api.responseBinary.status` and `responseCut.status`.
  `studio.cellEditor.detect.base64Bytes.status` becomes "{size} decoded",
  `hexBytes.status` becomes "{size} of data" (bare "{size}" is a C4 near-duplicate of `studio.console.format.engine.error`), both with `{ size: formatBytes(bytes) }`.
- Guard: U179 `vt`, exempt none, regex `\bcount:\s*(?:[\w.]+\.)?(?:bytes|\w*Bytes)\b`.
- Visible change: yes (HTTP binary note, approval binary and truncation wording, cell editor
  decoded size).
- Specs: `studio/http-request.spec.ts` (grep `http-response-binary`),
  `studio/approvals-api-response.spec.ts`, cell editor detect specs (grep `decoded` and the detect
  testids under `tests/ui/studio/`).
- Wave 1 (3 sites plus keys in one commit: params change fails typecheck otherwise).

### F3.4 [DUP] EXTRACT: kept name mark

- Sites (kept variable name in the kept mark, then " · type"):
  `src/studio/approvals/KeptVariableRow.vue:23-26` (`dd class="font-data"`, type inherits size),
  `src/studio/approvals/DbParamsList.vue:38-45` (P680 Part 1, `{{kept.name}}` text, type inherits),
  `src/studio/approvals/ApiResponseBody.vue:84-89` (P678 Part 2 "Will keep": `font-data` inside
  `mark-class`, type `text-kira-sm`). Settings `KeptEntryRow` is a row surface (no mark), not a
  site.
- Shared home: `src/studio/kept/KeptNameMark.vue`.
- Change: props `name: string` (text inside the mark), `type: string`. Renders
  `<span class="font-data"><HighlightedText :parts="[{ text: name, match: true }]"
  :mark-class="KEPT_MARK_CLASS" /><span class="text-muted-foreground"> · {{ type }}</span></span>`.
  Sites: KeptVariableRow `name=use.name`, `type=keptTypeLabel(use)` (dd drops `font-data`);
  DbParamsList `name=row.ref`, `type=row.type` (value dd drops `font-data`; placeholder dt keeps it);
  ApiResponseBody `name=k.name`, `type=keptTypeLabel(k)`. Testids stay on the hosts.
- Guard: U180 `v`, exempt `KeptNameMark.vue`, regex
  `:mark-class="[^"]*KEPT_MARK_CLASS[^"]*"\s*/>\s*<span` (no backtick or `$`: the heredoc is
  unquoted).
- Visible change: yes ("Will keep" type text takes the card body size).
- Specs: `studio/approvals-api-kept.spec.ts`, `studio/approvals-api-kept-review.spec.ts`,
  `studio/approvals-api-kept-script.spec.ts`, `studio/dbmcp-kept-params.spec.ts`.
- Wave 2 (component plus 3 sites in one commit).

### F3.5 [DUP] MERGE: JSON pretty-print onto the lossless beautifier

- Shared piece: `src/studio/beautify.ts` `beautifyJson(text, 'indented')`, the Studio JSON scanner
  that never round-trips through `JSON.parse` (numbers kept from their raw slice; the file header
  names it the one definition).
- Sites:
  - two identical wrappers: `src/studio/views/console/copyAll.ts:14` `prettyJson` (exported, used by
    `views/console/menu.ts:199,209,269`), `src/studio/views/documents/menu.ts:15` `prettyJson`
    (local, `:132`);
  - lossy `JSON.stringify(JSON.parse())`: `src/studio/approvals/prettyJson.ts` (P677; returns null
    for a big integer or escape, so such a body shows unindented) used by `ApiResponseBody.vue:36`,
    `ApiResponseView.vue:35`, `src/studio/kept/KeptValueView.vue:36,55`;
    `src/studio/kept/keptHighlight.ts:133` `prettyPlain` (behind `prettyKeptJson`, used by
    `ApiRequestBody.vue:59`: a request body with an integer above 2^53 shows altered digits in the
    approval card); `src/studio/editor/hoverInfo.ts:31` `formatHoverValue` (variable hover, same
    digit loss; used by `src/studio/api/state/variableCompletion.ts:159`).
- Name collision: three functions named `prettyJson` (approvals, console, documents).
- Change: `beautify.ts` exports `indentJson(text: string): string | null` (indented `beautifyJson`,
  null when not JSON) and `prettyJson(text: string): string` (`indentJson(text) ?? text`). Delete
  `approvals/prettyJson.ts`, the `copyAll.ts` and `documents/menu.ts` copies, `prettyPlain` and
  `formatHoverValue`. Callers: approvals and `KeptValueView` use `indentJson`; console menu,
  documents menu and `copyAll` use `prettyJson`; `prettyKeptJson` keeps its sentinel swap and calls
  `indentJson`; `variableCompletion` uses `prettyJson`. `KeptValueView` keeps `JSON.parse` only for
  its string-or-not test. Retarget `apps/kira-space/tests/unit/studio/hover-value-caption.spec.ts`
  to `prettyJson` from `beautify.ts` (same expectations hold: two-space indent, `": "`).
- Guard: U181 `vt`, exempt none, regex `JSON\.stringify\(\s*JSON\.parse\(`.
- Visible change: yes (big-integer and escaped JSON now indented in the review, kept values and
  hover; request body digits exact).
- Specs: `studio/approvals-api-response.spec.ts` (extend: a response body with an integer above
  2^53 renders indented with exact digits), `studio/approvals-api-kept*.spec.ts`,
  `studio/kept-variables.spec.ts`, `studio/http-variables.spec.ts` (hover), console and documents
  copy specs (grep `canonicalJson`, `copy-all`), unit `beautify-depth.spec.ts`,
  `hover-value-caption.spec.ts`.
- Wave 2 (one commit: helpers, deletions, all callers).

### F3.6 [GUARD-GAP] MIGRATE: catalog key used outside its namespace

- Rule: ARCHITECTURE catalog lines "Shared vocabulary goes in common.*, or <namespace>.common.*
  inside a module" and "Add new strings under the owning module's namespace (common.* if shared)";
  check-catalog C3 forbids a second key for the same English, so a borrowed key is shared
  vocabulary and belongs in `common.*`. No guard checks it; iterations 1 and 2 moved three borrowed
  keys by hand (`docker.logs.tail.option`, `docker.edit.unset.status`,
  `workbench.approvals.expired.status`). 62 borrowed keys remain, about 80 call sites.
- Module table (file path prefix to own namespace): `src/studio/` studio; `src/ade/`, `src/repo/`,
  `src/views/`, `src/hostview/`, `src/capture/`, `src/configsync/`, `src/openedfiles/` space;
  `mobile/` mobile; `packages/docker-ui/src/` docker; `packages/git-ui/src/` git;
  `packages/workbench/src/automations/` automations; `packages/workbench/src/memory/` memory.
  Always allowed: own, `common`, `format`, `shared`, `theme`, `backend`. Out of the table this
  iteration: `src/workbench/`, `src/components/`, `src/state/`, rest of `packages/workbench` (the
  shell hosts every module; final scan decides).
- Allowed pairs (named exemptions, reason each): mobile to `space.` (the phone app renders the Space
  screens with Space wording); space to `git.` (ADE hosts git-ui review comments and git operations);
  space to `studio.proof.` (ADE renders Studio proofs); space to `memory.distill.` (repo hosts memory
  distill of a selection).
- Key moves (old to new; every caller of the old key swaps, owner included, in one commit):
  - `automations.dialog.run.suffix.label` to `common.run.addedByKira.label`;
    `automations.dialog.run.untitled.title` to `common.run.untitled.title`;
    `automations.form.mcp.prompt.label` to `common.mcp.askFirst.label`;
    `automations.panel.none.empty` to `common.scripts.none.empty`;
    `automations.params.title.label` to `common.field.parameters.label`;
    `automations.run.state.waiting.status` to `common.state.waitingForYou.status`;
    `automations.script.paramNamed.label` to `common.field.parameterNamed.label`.
  - `docker.edit.addVariable.label` to `common.action.addVariable.label`;
    `docker.edit.attach.label` to `common.action.attach.label`;
    `docker.edit.pending.title` to `common.changes.pending.title`;
    `docker.field.env.label` to `common.field.environment.label`;
    `docker.field.labels.label` to `common.field.labels.label`;
    `docker.logs.follow.label`, `docker.logs.timestamps.label`, `docker.logs.wrap.label`,
    `docker.logs.tail.aria` to `common.logs.follow.label`, `common.logs.timestamps.label`,
    `common.logs.wrap.label`, `common.logs.tail.aria`;
    `docker.resource.id.label` to `common.field.id.label`;
    `docker.resource.network.label` to `common.field.network.label`;
    `docker.tab.logs.label` to `common.tab.logs.label`;
    `docker.tab.overview.label` to `common.tab.overview.label`.
  - `git.dialog.postCheckoutPullConfirm.notNow.label` to `common.action.notNow.label`;
    `git.menu.stashShow.label` to `common.action.showChanges.label`.
  - `memory.common.origin.agent.label` to `common.origin.agent.label`;
    `memory.import.confirm.skipped.label` to `common.files.skipped.label`.
  - `mobile.backlog.handle.aria` to `space.common.dragToReorder.aria`;
    `mobile.needs.takeOverConfirm.title` to `space.common.takeOverConfirm.title` (owner space, mobile
    reads it through its pair).
  - `space.ade.branchPanel.changes.label` to `common.changes.label`;
    `space.ade.changes.uncommitted.label` to `common.changes.uncommitted.label`;
    `space.ade.context.readOnly.hint` to `common.content.readOnly.hint`;
    `space.ade.needs.kind.question.label` to `common.kind.question.label`;
    `space.ade.proof.entry.delete.tooltip` to `studio.proof.delete.tooltip`;
    `space.common.pending.status` to `common.state.pending.status`;
    `space.settings.agentUsage.session.label` to `common.field.session.label`.
  - `studio.copy.format.json.label` to `common.copyFormat.json.label`;
    `studio.http.raw.requestPending.label` to `common.field.request.label`;
    `studio.http.diff.unchangedRow.status` to `common.diff.unchangedRow.status`.
  - `workbench.tabStrip.newTab.label` to `common.tab.new.label`;
    `workbench.tabStrip.discard.title`, `.body` to `common.tab.discard.title`, `.body`;
    `workbench.statusBar.runs.open.hint` to `automations.statusBar.runs.open.hint` when automations
    is its only caller, else `common.statusBar.runs.open.hint`.
  - Stay (covered by a pair): `git.op.inProgress.rebase.label`, `git.review.comments.*` (6 keys),
    `git.toolbar.forcePushing.loading`, `space.ade.later.add.label`,
    `space.ade.request.taskCreate.brief.label`, `space.common.{backlog,needYou,plan,reply}.label`,
    `space.common.day.{earlier,today,tomorrow}.label`, `space.common.status.blocked.status`,
    `studio.proof.{delete.title,delete.body,tabs.aria,diagram.invalid.title}`,
    `memory.distill.selection.label`.
- Before each move: grep the old key and its parent prefix as a template literal
  (`` `docker.tab.${ `` and the like) across `apps`, `packages`, `internal` and specs. A key reached
  through a dynamic prefix stays and its borrower moves to a pair or is reported back; English stays
  byte-identical (no copy change). Key names may adjust to pass check-catalog C1/C2 (depth, role);
  record the final name in the tick line.
- Guard: U182 in `scripts/copy/check-copy.ts` (it already scans the source set; rule in
  `sourceRules.ts` or a scripted pass in `check-copy.ts`). For each file in the module table, every
  string literal that is a catalog key (`KEY_LITERAL` shape, present in `en.json`) must start with an
  allowed namespace or a pair prefix. The 4 pairs go in `scripts/copy/exemptions.ts` with their
  reasons. Unused or stale pair fails the run (same as the existing exemptions).
- Visible change: no (English unchanged).
- Specs: none beyond `bun run typecheck` (MessageKey literals), `bun run lint` (check-catalog
  `--unused`, C3), `bun run lint:rules`; text-asserting specs keep passing since English is
  unchanged.
- Wave 1 (all moves and callers across modules; one commit per namespace group).

## PARK

None new. Still Planned: P682, P683, P684, P685, P687.

## Waves and streams

Wave-2 sites: Studio 18 (F3.1 6, F3.4 3, F3.5 9 files), docker 2. Under 40: one sequential wave-2
Sonnet by default; the two-stream split below is safe if the orchestrator wants it (no ordering
dependency, disjoint files).

Deviation from the master plan: module-local pieces (`HttpStatusBadge`, `KeptNameMark`,
`MeasuredSection`, `beautify.ts` helpers) are created by the wave-2 stream owning their module, in
the same commit as their consumers, because knip fails on a new file or export with no importer.
Wave 1 holds the cross-module catalog work.

### File ownership

| Owner | Files |
|-------|-------|
| Wave 1 | `packages/theme/src/i18n/en.json`; every caller file of an F3.6 moved key (any module); F3.3 `ResponsePane.vue`, `ApiResponseView.vue` (lines 56-66 only), `detect.ts`; `scripts/copy/exemptions.ts` stays untouched until wave 3; `docs/ARCHITECTURE.md` rules |
| Wave 2 A Studio | `src/studio/components/HttpStatusBadge.vue` (new), `src/studio/kept/KeptNameMark.vue` (new), `src/studio/beautify.ts`, `ResponsePane.vue`, `ResponseHistoryList.vue`, `ResponseDiffDialog.vue`, `TimelinePane.vue`, `ApiResponseView.vue`, `ApiResponseBody.vue`, `KeptVariableRow.vue`, `DbParamsList.vue`, `KeptValueView.vue`, `keptHighlight.ts`, `editor/hoverInfo.ts`, `api/state/variableCompletion.ts`, `views/console/copyAll.ts`, `views/console/menu.ts`, `views/documents/menu.ts`, `approvals/prettyJson.ts` (deleted), `tests/unit/studio/hover-value-caption.spec.ts`, Studio UI specs |
| Wave 2 B docker | `packages/docker-ui/src/components/MeasuredSection.vue` (new), `ContainerSizeSection.vue`, `EngineDiskSection.vue`, `packages/docker-ui/src/queries.ts`, `studio/docker-disk.spec.ts` |
| Wave 3 | `scripts/check-ui-primitives.sh`, `scripts/copy/check-copy.ts`, `scripts/copy/sourceRules.ts`, `scripts/copy/exemptions.ts`, `docs/ARCHITECTURE.md` guard range |

Shared files: none in wave 2 (no catalog edits). Wave 1 lands before wave 2 starts, so wave-2 edits
of `ResponsePane.vue`, `ApiResponseView.vue`, `DbParamsList.vue` sit on wave 1's lines.

### Wave 1 (one Sonnet, sequential, commits per finding group)

1. F3.6: per namespace group, move keys in `en.json` (keep sorted: `biome check --write`), swap
   every caller, `bun run typecheck`, `bun scripts/copy/check-catalog.ts --unused`, commit. Order:
   automations, docker, git, memory, mobile, space, studio, workbench.
2. F3.3: reword three keys, delete two, swap the 3 sites; commit.
3. ARCHITECTURE rule lines (section below). No guard lines.
4. `bun run typecheck`, `bun run lint`, specs of every touched surface. Tick entries.

### Wave 2

| Stream | Sites |
|--------|-------|
| A Studio | F3.1 (component, 6 sites), F3.4 (component, 3 sites), F3.5 (helpers, deletions, callers, unit spec retarget, approvals-api-response extension) |
| B docker | F3.2 (component, 2 sites, queries.ts, docker-disk spec case) |

- One sequential implementer: A then B in one worktree, commit per finding.
- Two streams: worktree each off the wave-1 commit.
- Never touched in wave 2: `scripts/**`, `docs/ARCHITECTURE.md`, `packages/theme/src/**`,
  `packages/workbench/src/**`, `en.json`.
- Each stream runs its guards by hand (`grep -Pzl` with the script's `PRE` and `TOK`) over its dirs
  and reaches zero (A: U177, U179, U180, U181; B: U178), then `bun run typecheck`, `bun run lint`
  and its specs.
- A conflict at rebase means the ownership table was wrong: stop and report.

### Wave 3 (one Sonnet, after rebase onto the chapter branch)

1. Add header `# Guards U177-U181: docs/HISTORY.md (v0.3.18).` and the five lines below to the
   `GUARDS` heredoc (unquoted: `${TOK}` expands; escape a literal `$`; no backtick).
2. U182 in `scripts/copy/check-copy.ts` per F3.6; pairs in `exemptions.ts`; update the file header
   (`U64-U71` plus U182).
3. Run every new guard over the whole tree as it stands, P679 and P680 Part 2 files included if
   landed. Fix each leftover offender in the same commit (expected candidates: a P680 Part 2 DB
   review status badge or JSON pretty-print, a P679 key borrowed from another namespace, a kept name
   mark in `UserInputsPanel`).
4. Test each regex: zero hits on the tree, one hit on a planted offender in a scratch copy. U182:
   plant `t('docker.tab.logs.label')`-style borrowed key in a scratch Studio file.
5. `bun run lint:rules`; commit. Remove worktrees, push.

```
U177@@v@@HttpStatusBadge.vue@@hand HTTP status chip (use HttpStatusBadge)@@\bstatusClass\(|>=\s*500\b
U178@@vt@@MeasuredSection.vue@@hand docker measure section (use MeasuredSection)@@docker\.measuring\.loading|docker\.measure\.label
U179@@vt@@@@byte size passed as a count (use formatBytes, {size})@@\bcount:\s*(?:[\w.]+\.)?(?:bytes|\w*Bytes)\b
U180@@v@@KeptNameMark.vue@@hand kept name and type (use KeptNameMark)@@:mark-class="[^"]*KEPT_MARK_CLASS[^"]*"\s*/>\s*<span
U181@@vt@@@@JSON.parse/stringify pretty-print (use beautify.ts indentJson, prettyJson)@@JSON\.stringify\(\s*JSON\.parse\(
```

U180 needs the multi-line match `grep -Pz` gives the loop. Confirm U181 does not fire on `ejson.ts`
(`JSON.stringify(canonicalToRelaxed(`) and U179 does not fire on `formatBytes` callers.

### ARCHITECTURE (wave 1 writes rules, wave 3 updates the guard range)

- Guard range line `U83-U181` for `check-ui-primitives.sh`; copy guard line names U182. PERMANENT
  count stays 53.
- An HTTP status is `HttpStatusBadge` (Studio): `statusClass` tone, `statusHint` tooltip, data font.
  U177.
- A docker section measured on demand is `MeasuredSection`: loading and not-measured as
  InlineState, first failure InlineState error, re-measure failure Alert error with data kept. U178.
- A byte size is `formatBytes` passed as `{size}`, never a raw count with "bytes". A binary body and
  a cut response body read the same in the HTTP pane and the approval card. U179.
- An approval-card kept name with its type is `KeptNameMark`. U180.
- Studio JSON pretty-printing goes through `beautify.ts` (`indentJson`, `prettyJson`): lossless,
  never `JSON.stringify(JSON.parse())`. U181.
- Catalog ownership: a module's sources use its own namespace plus `common`, `format`, `shared`,
  `theme`, `backend`; pairs mobile to `space.`, space to `git.`, `studio.proof.`,
  `memory.distill.`. U182.

## Tests

- Pure refactors (F3.1, F3.4, F3.6): no new spec; ledger specs plus `bun run typecheck`,
  `bun run lint`.
- F3.2 `MeasuredSection` has state behaviour: one added case in `studio/docker-disk.spec.ts`
  (re-measure failure keeps data, shows Alert). Renderer-only.
- F3.5 changes displayed text for big integers: extend `studio/approvals-api-response.spec.ts`
  with a body holding an integer above 2^53 (exact digits, indented). The held text crosses IPC
  unchanged; only rendering changes, so no flow half. Retarget the unit hover spec.
- F3.3: values crossing IPC (`bodyBytes`) are unchanged; only formatting changes, no flow test.
- Visual specs are expected to fail on visible changes; chapter end re-records.

## Visible-change list (chapter-end visual re-record)

F3.1 approval response status badge tone, data font and hint tooltip. F3.2 container size loading
and not-measured lines; disk first-measure failure as InlineState. F3.3 HTTP binary note ("Binary
body, 12 KB not shown"), approval binary and truncation wording, cell editor decoded size. F3.4
"Will keep" type size. F3.5 big-integer and escaped JSON indented in review, kept values and hover.

## Dismissed clusters

Key plus reason. Iteration 1 and 2 dismissed families still hold (C2 generic layout utility sets;
C3 per-kind view stores P683, git-core/git-ipc mirrors, Monaco and SlickGrid hosts, fixtures, import
lists; C6 literal-icon and generic-concept families).

### New keys

- `584401ee07` C1: `RunScriptDialog` and `ScheduleConfirmDialog` preview `QueryState`, 2 sites one
  module, identical composition of shared pieces.
- `da9453227b` C1, `35db3c9d95` C3, `93feb5fecf` C3: `DbQueryBody` and `DbExemptionBody` bodies, 2
  sites one module, identical; P680 Part 2 rewrites both (final scan rechecks).
- `f7c4d6d80f` C1: F3.2.
- `ea83f90207` C4: `RowActionButton` declines `CopyIconButton`'s own Tooltip for a named
  requirement (P161: per-row reka Tooltip cost; `DocumentView` owns one delegated tooltip).

### Clusters touching files changed since iteration 2

- C1 `3047589af3`: Studio MCP API pane switch fields, settings `Field` family (iteration 2).
- C2 `ff40650184`, `511d91c4e8`, `c8df8ffe7d`, `800c51eaa5`, `ce3883fa28`, `f2402a4405`,
  `fbd60e6b38`, `9b64b1ff02`, `09949c89fc`, `d2539cee10`, `527cd3acb6`, `baca5a45b7`,
  `af79d2bc3f`, `3f8de2a7e5`, `a0dc4ca87d`, `78509420ea`, `9549b166c1`, `1f85554ed7`,
  `70ce875566`, `2f1baaf684`: generic list, stack and label utility sets.
- C5 `ddb14076c4`: byte limits passed into `formatBytes` (`256 * 1024`), not display math.
  `82c4c8be05`: settings default chord data.
- C6 `ec53a334ac` refresh literal in docker sections: F3.2 (others in the literal-icon family).
  `43ecd8746e`: F3.2 hosts both glyphs in one place (refresh to re-measure, dashboard on the first
  Measure). Rest: families above.
- C7 `e12948b16c`: P682.

### Sweep

- `ReviewChoiceField`: P677 already unified the connection and API environment review choices.
- DB `PendingResponse` vs `ResultReview`: P680 Part 2 moves DB review onto `ResultReview`.
- Item id slice (`item.id.slice(item.kind.length + 1)`) in `DbResponseBody`, `ApiResponseBody`: 2
  sites one module; P680 Part 2 adds a third, final scan rechecks.
- Section heading plus content gap (`gap-0.5`, `gap-1`, `gap-2` under `SectionHeading flush`):
  density by content; final scan's spacing pass.
- Git toolbar progress chips vs ADE `AdeOpStatus` badge: toolbar status vs row badge, different
  surfaces. `UpdateDialog` spinner lines: operation progress in a dialog; InlineState's spinner
  delay would hide it.
- Per-domain tone maps (`AdeStepBlock`, `AdeRunOutcome`, `AdeBranchPanel`, `HostViewSidebar`,
  `ConfigSyncUnitRow`): different domains, each one map.
- `KeptVariablesPanel` Manage link: navigation link (iteration 2).

## Iteration gate

- Iteration 3 findings: 6 (3 EXTRACT, 1 MERGE, 1 MIGRATE, 1 GUARD-GAP MIGRATE). Kinds iterations 1
  and 2 never had: DRIFT of a status tone (HTTP status), DUP of a data formatter against a lossless
  parser (JSON pretty-print), GUARD-GAP for catalog namespace ownership.
- The orchestrator prompt's test (5 or more, or a new kind) is met. `P674.md` sets the cap:
  "Iteration 3 never triggers an iteration 4" and "Max 3 iterations before the final scan".
- Verdict: no iteration 4. Part 5 (final scan) runs next. It covers P679 and P680 Part 2 files,
  rechecks the three new families tree-wide (status chips, JSON display, catalog ownership including
  `src/workbench/` and the shell dirs left out of U182's table), and keeps the carry-forward list
  (P682-P685, P687). If the user wants an iteration 4 instead, that overrides `P674.md` and is
  theirs to call.
