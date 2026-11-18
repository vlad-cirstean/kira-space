# P672 Part 7 UI uniformity: status, tones, colours

Spec row: `../SPEC.md` P672 Part 7. Rules RS.1-RS.3, RL.2, RL.4, RI.2, RCo.1-RCo.2, migration items
M24-M27 (`P672-inventory.md` §6, §9, §12, appendix A.7). D1 decided: option (a), ADE pastel tones on
the plan board and task cards only. Base: `chapter/v0.3.17` at `f2f09afe8`. Inventory line numbers
are at `6527aec4`. Every fact below was re-checked at `f2f09afe8` via `codegraph_explore` and
template scans. Line numbers are hints: implementer greps the named pattern or `data-testid` before
editing.

UI-only phase. Colour and tone change only. Behaviour, i18n keys and every `data-testid` stay. No
new i18n key. No IPC, Go or flow change.

## Rules applied (recap)

- RS.1 one status-to-tone map: running or in progress info (+ Spinner); succeeded ok; waiting,
  attention, partial, stale warn; failed, broken, conflict error; idle, stopped, draft, unknown
  neutral. Agent liveness `working` is ok (same as a running container and the status-bar agent
  dot); `unseen` and `waiting` info; `input` warn.
- RS.2 display by density: Badge in a row or header, StatusDot in a dense list, tone text in prose.
- RS.3 / D1: ADE pastel tones (`text|bg|border-tone-*`, `TONE_*_CLASS` maps, `SOLID_CLASS`,
  `ACTION_CLASS`, an `AdeChip` with `tone`) live only under `ade/v2/plan/` and the mobile board
  card `PlanTaskCard.vue` (plus `AdeAutomationChip.vue`, whose only caller is `AdeTaskCard`). Every
  other ADE surface (panel, sessions, needs, links, workflows, review, notes, settings, Runs) uses
  theme tones (`text-ok|warn|error|info`, `Badge` tone variants, `StatusDot` `tone`). A task status
  `color` chip (`AdeChip :color`, `STATUS_CHIP_CLASS`) is user data, not a tone; it stays
  everywhere.
- RL.2 trailing row meta uses `rowMeta` (`text-kira-sm text-muted-foreground tabular-nums`). The
  spec row says "row meta subtle"; ARCHITECTURE P267 (line 1402) puts row meta on muted and
  `rowMeta` is muted. Deliberate reading: hand meta spans move to `rowMeta`, colour stays muted.
- RL.4 / RI.2 warning icon `text-warn`, error icon `text-error` (name `error`); never muted, never
  pastel off-board.
- RCo.1 status colour only from RS.1 tones. RCo.2 drop redundant `text-fg`, only in files this
  Part touches; keep it where it resets colour inside a muted, toned or Badge parent.
- U25: no `<p>`/`<div>` with `text-error` (FieldError, InlineState or Alert). U26: no tone alpha
  (`ring-info/30` is banned). U35: no `outline-none`/`focus-visible:outline-*`.

## Baselines

`scripts/check-ui-primitives.sh` TEMPORARY block holds 31 lines; 19 name `P672 Part 7`
(`grep -c 'P672 Part 7'` = 19), all U114, lines 155-173. Files (under
`apps/kira-space/frontend/src/ade/v2/` unless marked):

`AdeActivityIcon.vue`, `context/AdeContextPane.vue`, `dialog/AdeAgentDialog.vue`,
`links/AdeLinkStrip.vue`, `needs/AdeAllSessionRow.vue`, `needs/AdeNeedsPage.vue`,
`panel/AdeBranchPanel.vue`, `panel/AdeRunOutcome.vue`, `panel/AdeStepBlock.vue`,
`panel/AdeTaskIssueLine.vue`, `review/AdeReviewSync.vue`, `sessions/AdeSessionsRow.vue`,
`settings/AdeSettingsPane.vue`, `workflows/AdeStepNode.vue`, `workflows/AdeStepsTab.vue`,
`workflows/AdeWorkflowsTab.vue`, and `packages/workbench/src/automations/`: `RunFileDialog.vue`,
`ScriptBrokenRow.vue`, `ScriptRow.vue`.

Delete each line in the same commit that fixes its file (stale check fails a fixed file whose line
stays; a dropped line before the fix fails U114). 12 TEMPORARY lines remain after this Part (U117
Part 8 ×7, U115 Part 9 ×5); never touch them. Run `bash scripts/check-ui-primitives.sh` after every
commit (part of `bun run lint`). U114 regex and exempt (`/ade/v2/plan/`) stay; after C4 it has zero
offenders and no TEMPORARY line. New guard U125 (C7) lands with zero offenders, no TEMPORARY line.

## Theme pieces

All land in C1, each with its first consumer in C1.

1. `packages/theme/src/lib/statusTone.ts` (new; RS.1 as data):

   ```ts
   // RS.1 status tones. Badge, tone text and StatusDot read the same key.
   export type StatusTone = 'ok' | 'warn' | 'error' | 'info' | 'neutral';
   export type ToneBadgeVariant = 'ok' | 'warn' | 'error' | 'info' | 'default';
   export const TONE_BADGE: Record<StatusTone, ToneBadgeVariant> = {
     ok: 'ok', warn: 'warn', error: 'error', info: 'info', neutral: 'default',
   };
   export const TONE_TEXT: Record<StatusTone, string> = {
     ok: 'text-ok', warn: 'text-warn', error: 'text-error', info: 'text-info',
     neutral: 'text-muted-foreground',
   };
   export const TONE_DOT: Record<StatusTone, 'ok' | 'warn' | 'error' | 'info' | 'muted'> = {
     ok: 'ok', warn: 'warn', error: 'error', info: 'info', neutral: 'muted',
   };
   // Keyed by string: git-ui and ADE carry the state as a plain string.
   export const CHANGE_REQUEST_TONE: Readonly<Record<string, StatusTone>> = {
     open: 'info', draft: 'neutral', merged: 'ok', closed: 'neutral',
   };
   export const changeRequestTone = (state: string): StatusTone =>
     CHANGE_REQUEST_TONE[state] ?? 'neutral';
   ```

   `ToneBadgeVariant` is a subset of Badge `variant` and of git-ui `RefBadgeVariant`, so both
   accept it unchanged. First consumers (C1): `packages/git-ui/src/lib/badgeClass.ts`
   (`TONE_BADGE`, `changeRequestTone`), `packages/git-ui/src/components/CommitMeta.vue`
   (`TONE_TEXT`), `apps/kira-space/frontend/src/ade/v2/panel/prState.ts`. `TONE_DOT` first
   consumer: `AdeLinkPill.vue` (C3); C1 lands it with the module because C3 follows in the same
   Part.

2. `apps/kira-space/frontend/src/ade/v2/tones.ts` additions (board `Tone` to theme, and back):

   ```ts
   // Off-board render of a board tone (D1). Purple is merged on the board.
   export const THEME_TONE: Record<Tone, StatusTone> = {
     amber: 'warn', red: 'error', green: 'ok', blue: 'info', purple: 'ok', grey: 'neutral',
   };
   // Board pastel for an RS.1 tone.
   export const PASTEL: Record<StatusTone, Tone> = {
     ok: 'green', warn: 'amber', error: 'red', info: 'blue', neutral: 'grey',
   };
   ```

   First consumers: `THEME_TONE` in `AdeBranchPanel.vue` (C2, `row.tag.tone`); `PASTEL` in
   `prState.ts` (C1). Also in C1: `SOLID_CLASS` loses its `agent` key (type `Record<Tone,
   string>`); `ACTION_CLASS` is deleted (C1 moves its only consumer, see C1).

No new token. No new component. No light theme: off-board surfaces move from `--kira-status-*`
pastels to `--kira-*` theme tones, which shrinks the dark-only surface; board pastels stay, so the
ARCHITECTURE "Known open items" dark-only entry stays (wording updated in C8).

## Shapes (copy exactly)

- Toned AdeChip off-board to Badge, keeping own `data-testid` and `wide`:
  `<AdeChip :label="x" tone="amber" data-testid="t" />` becomes
  `<Badge variant="warn" data-testid="t">{{ x }}</Badge>`; a mapped tone becomes
  `<Badge :variant="TONE_BADGE[THEME_TONE[tone]]">`; `wide` becomes `class="min-w-21
  justify-center"`. Import `Badge` from `@theme/components/ui/badge`. Grey tone becomes
  `variant="default"`.
- Status-colour AdeChip (`:color=`) stays an AdeChip.
- Pastel solid fill to theme fill: `bg-tone-<x>-solid` to `bg-ok|warn|error|info`; ink
  `text-tone-ink` to `text-chrome`.
- Error sentence block: `<Alert variant="error" data-testid="t"><AlertDescription>…
  </AlertDescription></Alert>`; warning block the same with `variant="warn"`.
- Off-board `Tone` source (board tables: `actions.ts`, `needsYou.ts`, `artifacts/status.ts`,
  `stepBlocks.ts`, `useTaskLinks.ts`) is mapped at the render site via `THEME_TONE`; off-board-only
  local maps change their value type to `StatusTone` and use RS.1 values directly.

## Stream decision

One sequential implementer. C1 pieces (`statusTone.ts`, `tones.ts`, `prState.ts`) feed every later
commit; `tones.ts`, the guard TEMPORARY block, `ARCHITECTURE.md` and `SPEC.md` are shared by all; C2
and C3 both edit `AdeStepBlock`-adjacent panel files and `AdeTaskPanel`; C6 sweeps files earlier
commits touch. No split.

## Commits

Each commit: `bun run typecheck`, `bun run lint` (includes the guard and `check-conventions.ts`)
green. Delete the named TEMPORARY lines in the same commit. Paths under
`apps/kira-space/frontend/src/ade/v2/` unless marked.

### C1 `feat(ui): status tone map, change-request tones, op status info`

- `packages/theme/src/lib/statusTone.ts`: new, shape above.
- `tones.ts`: add `THEME_TONE`, `PASTEL`; drop `SOLID_CLASS.agent`; delete `ACTION_CLASS`.
- `run/AdeTaskActionButton.vue`: label branch (panel only: `AdeWorkflowBlock`, `AdeTaskPanel`;
  board uses `icon`) drops `:class="ACTION_CLASS[action.tone]"` and takes
  `:variant="action.tone === 'red' ? 'dialog-danger' : 'dialog'"` like the icon branch. Agent
  `▶ Run` loses its agent outline (neutral dialog button, RCo.1).
- `sessions/AdeSessionBadge.vue`: `SOLID_CLASS.agent` becomes literal
  `bg-agent text-chrome border-transparent`; drop the import.
- `panel/prState.ts`: `PR_TONE` becomes
  `Object.fromEntries(Object.entries(CHANGE_REQUEST_TONE).map(([k, v]) => [k, PASTEL[v]]))` typed
  `Record<string, Tone>` (board `AdeBranchPr` only: open blue, merged green, closed and draft
  grey). Off-board callers stop using `PR_TONE` in C2.
- `plan/AdeOpStatus.vue`: `variant="warn"` to `variant="info"` (M24).
- `packages/git-ui/src/lib/badgeClass.ts`: drop `PR_BADGE_VARIANT` and the merged
  `bg-conn-violet/15 text-conn-violet` branch; `changeRequestBadgeClass(state)` returns
  `refBadgeClass(TONE_BADGE[changeRequestTone(state)], 'no-underline cursor-pointer')` (M24).
  The violet alpha also left U26's blind spot (`conn-violet` is no tone name).
- `packages/git-ui/src/components/CommitMeta.vue`: delete `CHANGE_REQUEST_ICON_CLASS`; line 339
  uses `TONE_TEXT[changeRequestTone(changeRequestIcon!.state)]`. File is 474 lines.
- Callers `refBadges.ts`, `RefsPopoverBranches.vue:240`, `StackList.vue:137` need no edit (same
  function); confirm by typecheck.

No TEMPORARY line changes.

### C2 `refactor(ade): panel, branch, step and run tones off the board (M24, M25)`

TEMPORARY lines removed: `panel/AdeBranchPanel.vue`, `panel/AdeStepBlock.vue`,
`panel/AdeRunOutcome.vue`, `panel/AdeTaskIssueLine.vue`.

- `board/stepBlocks.ts`: `RUN_GLYPH` running `tone: 'amber'` and `STEP_TONE` running `'amber'` to
  `'blue'` (RS.1 running info; also the board and mobile card dot, pastel blue there).
- `artifacts/status.ts`: add `export const ISSUE_CATEGORY_TONE: Record<string, Tone> = { new:
  'grey', indeterminate: 'blue', done: 'green' }` and `issueCategoryTone(c) => ... ?? 'grey'`.
  `links/useTaskLinks.ts:28` and `panel/AdeTaskIssueLine.vue:32` delete their local
  `CATEGORY_TONE` and import it.
- `panel/AdeTaskIssueLine.vue`: AdeChip (testid `ade-task-issue-status`) to Badge
  `TONE_BADGE[THEME_TONE[issueCategoryTone(...)]]`, same testid; :98 gap `text-tone-amber` to
  `text-warn`.
- `panel/AdeBranchPanel.vue` (571 lines; N10 cap 600, net change must not grow it past 600):
  - :324 `border-tone-blue-solid` to `border-info`; :341 `text-tone-amber` to `text-warn`.
  - :329 and :435 `AdeChip :tone="row.tag.tone"` to Badge
    `TONE_BADGE[THEME_TONE[row.tag.tone ?? 'grey']]` (:435 keeps `min-w-21 justify-center`). If
    `row.tag` carries `color`, keep an AdeChip `:color` branch (check `TaskCell`/branch tag type).
  - :458-462 PR chip to Badge `TONE_BADGE[changeRequestTone(changeRequest?.state ?? '')]`.
  - `INTO_TONE` and `DEPLOY_TONE` value type to `StatusTone`: merged ok, stale warn, not merged
    neutral; deployed ok, stale warn, not deployed and unknown neutral. Chips at :511, :549 to
    Badge `TONE_BADGE[...]`.
  - Drop `PR_TONE`, `AdeChip` (if unused), `Tone` imports.
- `panel/AdeStepBlock.vue`:
  - `STATE_TONE` (:49) to `Record<..., StatusTone>`: past ok, now info, next neutral, skipped
    neutral; AdeChip :241 to Badge.
  - :93 `current && 'border-tone-amber'` to `border-info`.
  - `GLYPH_BOX` (:146): done `bg-ok text-chrome`, stuck and failed `bg-error text-chrome`, running
    `border border-info text-info`. `TEXT_TONE` (:160): stuck, failed `text-error`, running
    `text-info`, done `text-ok`.
  - :262 route `text-tone-blue` to `text-info`; :267 gated `text-tone-amber` to `text-warn`.
  - :295, :309 `TONE_TEXT_CLASS[rl.tone]` to `TONE_TEXT[THEME_TONE[rl.tone]]`.
  - :319 AdeChip (red/amber) to Badge `error`/`warn`.
- `panel/AdeRunOutcome.vue`: `STATE_TONE` (:59) to `StatusTone`: done ok, failed error, stuck
  error, back warn, running info, pending neutral, review info. AdeChips :118, :120
  (`resultTone` returns `StatusTone`), :130 (verified ok / warn) to Badge. :160 `li text-tone-red`
  to `text-error`; :185 `text-tone-amber` to `text-warn`.
- `panel/AdeMainBranches.vue:134`: `TONE_TEXT_CLASS[PR_TONE[...]]` to
  `TONE_TEXT[changeRequestTone(pr.state)]`.
- `plan/glyphs.ts:97,102,107` `CHECKS_GLYPH` classes `text-tone-green` / `text-tone-red` to
  `text-ok` / `text-error` (shared with board `AdeBranchPr`; theme tones on the board are allowed,
  D1 limits pastels, not theme tones).
- `panel/AdeTaskPanel.vue`: `chip` computed (:74) and `v-bind="chip"` (:124) go: render
  `<Badge v-if="card.parked" variant="default">{{ label }}</Badge>` and
  `<AdeChip v-else :label="label" :color="card.status.color" />`; :126 blocked AdeChip to
  `<Badge variant="error" data-testid="ade-panel-blocked">`.
- `panel/AdeMainTab.vue:78` blocked AdeChip to `<Badge variant="error" data-testid="ade-task-blocked">`.
- `panel/AdeArchivedPanel.vue`: :140 archived grey, :206 host link grey, :265 removed grey to
  Badge `default`; :264 merged purple to Badge `ok`; :275 PR chip to Badge
  `TONE_BADGE[changeRequestTone(b.changeRequest.state)]`; status `:color` chip (:144) stays.
- `AdeParentChip.vue`: `TONE_TAG_CLASS.grey|.blue` on Badge chip to Badge `default` / `info`
  (shared with `AdeMainTab`, `AdeArchivedPanel`, `AdeTaskCard`; theme Badge is fine on the board).
- `AdeOrchestratorBadge.vue`: `TONE_TAG_CLASS.blue` to Badge `info`.
- `details/AdeProofEntry.vue:60` pending amber to Badge `warn`, testid `ade-proof-pending`.

### C3 `refactor(ade): sessions, needs, links, workflows and review tones (M25)`

TEMPORARY lines removed: `AdeActivityIcon.vue`, `sessions/AdeSessionsRow.vue`,
`needs/AdeAllSessionRow.vue`, `needs/AdeNeedsPage.vue`, `links/AdeLinkStrip.vue`,
`workflows/AdeStepNode.vue`, `workflows/AdeStepsTab.vue`, `workflows/AdeWorkflowsTab.vue`,
`review/AdeReviewSync.vue`, `context/AdeContextPane.vue`, `settings/AdeSettingsPane.vue`,
`dialog/AdeAgentDialog.vue`.

- `AdeActivityIcon.vue` (18 callers incl. board and mobile card; theme tones allowed there):
  :22 input `bg-tone-amber-solid … text-tone-ink` to `bg-warn … text-chrome`; :28 working
  `bg-tone-green-solid` to `bg-ok`; :33 unseen `bg-info outline outline-1 outline-offset-1
  outline-info` (replaces `ring-2 ring-tone-blue-solid/30`; no alpha, U26; plain `outline-*`, not
  `focus-visible:outline-*`, U35); :38 waiting `border-info … text-info`.
- `sessions/AdeSessionsRow.vue:133` `text-tone-amber` to `text-warn`.
- `needs/AdeAllSessionRow.vue:29-33` stateClass: unseen `text-info`, input `text-warn`, working
  `text-ok`, waiting `text-info`.
- `needs/AdeNeedsPage.vue:52` `text-tone-green` to `text-ok` (EmptyState refactor is Part 9).
- `needs/AdeNeedsRow.vue:42` hand span `TONE_TAG_CLASS[item.tone]` (testid `ade-needs-kind`) to
  `<Badge :variant="TONE_BADGE[THEME_TONE[item.tone]]" class="w-23 justify-center"
  data-testid="ade-needs-kind">`. `ROW_BG_CLASS` stays if it uses no tone token (grey
  `bg-elevated`); a toned row background goes neutral.
- `apps/kira-space/frontend/mobile/screens/NeedsScreen.vue:98` AdeChip `item.tone` to Badge
  `TONE_BADGE[THEME_TONE[item.tone]]` (import `THEME_TONE` from `@ade/tones`).
- `links/AdeLinkStrip.vue:201` `<p … text-tone-amber>` (testid `ade-links-gaps`) to `text-warn`
  (warn, not error: U25 untouched).
- `links/AdeLinkPill.vue:41` `StatusDot :class="TONE_TEXT_CLASS[item.tone]" :data-tone="item.tone"`
  to `StatusDot :tone="TONE_DOT[THEME_TONE[item.tone]]" :data-tone="THEME_TONE[item.tone]"`;
  drop `color="currentColor"` if present.
- `links/AdeLinkCard.vue:86` AdeChip `artifactStatusTone` to Badge
  `TONE_BADGE[THEME_TONE[artifactStatusTone(item.status)]]`.
- `workflows/AdeStepNode.vue`: :179 `text-tone-green` to `text-ok`; :192 kind AdeChip (testid
  `ade-wf-node-kind`) to `<Badge variant="default" data-testid="ade-wf-node-kind">`; :282
  `StatusDot :color="r.ok ? 'var(--color-tone-green-solid)' : …"` to `:tone="r.ok ? 'ok' :
  'error'"`; :290 Handle `!bg-tone-green-solid` / `!bg-tone-red-solid` to `!bg-ok` / `!bg-error`.
- `workflows/AdeStepsTab.vue`: :137 kind AdeChip to `<Badge variant="default">`; :140 ✕
  `text-tone-red` to `text-error`.
- `workflows/AdeWorkflowsTab.vue:177` ✕ `text-tone-red` to `text-error`.
- `workflows/AdeStepInspector.vue:137` `StatusDot :class="TONE_TEXT_CLASS[KIND_TONE[…]]"
  color="currentColor"` to `<StatusDot tone="muted" />` (step kind is a category, not a status).
- `board/workflowForm.ts`: delete `KIND_TONE` (no consumer left after the three edits above).
- `workflows/AdeResultEdge.vue` `TONE_STROKE`: `stroke-tone-green-solid` / `stroke-tone-red-solid`
  to `stroke-ok` / `stroke-error`; `workflows/AdeWorkflowGraph.vue:320-321` marker fills to
  `fill-ok` / `fill-error`. `data-tone` ok/fail stays (spec `ade-v2-workflows:232-248`).
- `review/AdeWorkflowReviewPanel.vue`: `PHASE_TONE` (:145) to `StatusTone`: waiting info, sent
  info, working info, stuck error, done ok; :176 phase AdeChip (testid `ade-review-phase`) to
  Badge; :222 uncommitted amber to Badge `warn` (testid kept); :228 kind chip to Badge `default`;
  :229 state chip to Badge `addressed ? 'ok' : 'warn'`.
- `review/AdeReviewSync.vue:165` span `text-tone-red` to `text-error`.
- `notes/AdeNoteEntry.vue`: :88, :109, :116 grey to Badge `default`; :110 draft amber to Badge
  `default` (RS.1 draft neutral); :111 finished green to Badge `ok`.
- `updates/AdeTaskUpdates.vue:30` strip title AdeChip blue to Badge `info`.
- `agents/AdeLeasesPane.vue:113` release asked amber to Badge `warn`.
- `context/AdeContextPane.vue:77`, `settings/AdeSettingsPane.vue:77` `text-tone-red` to
  `text-error`.
- `dialog/AdeAgentDialog.vue:219-225` div `text-kira-md text-tone-amber` (testid
  `ade-dialog-risk`) to `<Alert variant="warn" data-testid="ade-dialog-risk"><AlertDescription>
  …</AlertDescription></Alert>` (Alert imported, used at :140; add `AlertDescription` import if
  missing).

### C4 `refactor(ui): Runs script and run file tones on theme (M25)`

TEMPORARY lines removed: `packages/workbench/src/automations/ScriptRow.vue`, `ScriptBrokenRow.vue`,
`RunFileDialog.vue`.

- `ScriptRow.vue:75-80` problem icon (testid `script-row-problem`): broken config is an error:
  `name="error"` `class="text-error"` (RI.2).
- `ScriptBrokenRow.vue:29` icon to `name="error"` `text-error`; :32 status span to `text-error`.
- `RunFileDialog.vue:416-422` `<div class="text-kira-sm text-tone-red" data-testid="run-file-error">`
  to `<Alert variant="error" data-testid="run-file-error"><AlertDescription>{{
  errorText(query.error.value) }}</AlertDescription></Alert>`, last body child (RD). A div with
  `text-error` would fail U25.

After C4: `grep -c 'P672 Part 7' scripts/check-ui-primitives.sh` = 0; U114 zero offenders.

### C5 `fix(ui): warning and error icon colours (M26)`

- `studio/kubernetes/components/KindBar.vue:86` `text-muted-foreground` to `text-warn` (testid
  `kube-kinds-partial`).
- `studio/project/DataGripImportDialog.vue:289` wrapper span `text-subtle` to `text-warn`.
- `studio/views/grpcrequest/ResponsePane.vue:499` truncated warning icon (testid
  `grpc-message-truncated`) gains `class="text-warn"`.
- `packages/git-ui/src/components/AppToolbar.vue:375` autofetch-stopped icon gains
  `class="text-warn"` (button text stays toolbar colour).
- `studio/project/ProjectTree.vue:317`, `studio/project/TreeRow.vue:219` warning icon inside a
  `text-error` parent to `name="error"` (colour inherits error).

Already compliant, no edit: `ExplainResultView`, `EndpointChip`, `ErrorPopover`,
`BranchRequestBody`. EmptyState media icons (`ContainerDetail`, `QueryState`, `GitBlockedPanel`)
are Part 9.

### C6 `refactor(ui): row meta via rowMeta, redundant text-fg (M27)`

Row meta: hand `text-kira-sm text-muted-foreground|text-subtle` trailing meta spans in a row become
`:class="[rowMeta, <placement classes>]"` (import `rowMeta` from `@theme/lib/rowVariants`). Verify
each is trailing meta (timestamp, count, size, usage, duration, offset) before editing:

- `ade/v2/workflows/AdeWorkflowsTab.vue:190` (`ade-wf-row-used`),
  `ade/v2/workflows/AdeStepsTab.vue:146` (`ade-step-row-used`), `ade/v2/later/AdeShelfRow.vue:71`
  (`ade-shelf-ago`).
- `repo/RepoSearchRow.vue:90,114`.
- `studio/views/httprequest/ResponseHistoryList.vue:223,238,239,240`.
- `studio/views/grpcrequest/ResponsePane.vue:491,496,510`,
  `studio/views/grpcrequest/CallHistoryList.vue:180`.
- `studio/workbench/panels/StudioStart.vue:133`.
- `packages/workbench/src/memory/import/ImportFileRow.vue:43,54`.
- `packages/docker-ui/src/components/ContainerTable.vue:162`.
- `studio/views/stream/StreamRowsTable.vue:403`, `studio/views/console/ExplainResultView.vue:180`.

Not meta, leave: `AdeMainTab:174` label, `DiscoverSavedMenu:168`, `ContainerRow:108`,
`RepoWorktreeRow:60`.

`text-fg`: in every file C1-C6 touch, drop `text-fg` where the element inherits fg already (no
muted, toned, subtle or Badge ancestor). Keep it where it resets colour (e.g. `AdeLinkPill` label
on a default Badge). `grep -n 'text-fg'` per touched file; no sweep beyond touched files (132 uses
in 101 files repo-wide; RCo.2 scope).

### C7 `chore(lint): guard U125 for ADE pastels off the board`

`scripts/check-ui-primitives.sh`: header comment after the Part 6 line:

```
# Guard U125: docs/v0.3.17/plans/P672-part7.md.
```

Row after U124:

```
U125@@vt@@/ade/v2/plan/|/ade/v2/tones.ts|AdeChip.vue|PlanTaskCard.vue|AdeAutomationChip.vue@@ADE pastel map, tone token or toned AdeChip off the plan board (theme tones, Badge, statusTone.ts; P672 D1)@@\b(?:TONE_TEXT_CLASS|TONE_TAG_CLASS|SOLID_CLASS|ACTION_CLASS)\b|${TOK}(?:ring|outline|fill|stroke)-tone-[a-z]|--color-tone-|<AdeChip\b(?![^>]*\s:color=)
```

Exempts all stay live: `/ade/v2/plan/` (AdeBranchPr, AdeAttention, AdeBranchRow, AdeTagIcon,
AdeBranchTip, AdeLeaseChip), `/ade/v2/tones.ts` (map definitions), `AdeChip.vue`
(`TONE_TAG_CLASS`), `PlanTaskCard.vue` (toned AdeChip, `--color-tone-`), `AdeAutomationChip.vue`
(toned AdeChip, board only). `ACTION_CLASS` is gone after C1; the token stays in the regex so it
cannot return.

Dry run at `f2f09afe8`: 15 map files, 20 AdeChip-without-`:color` files, 6 tone-token lines (5
files). After C3: only the five exempt groups match. If the dry run after C6 shows any other hit,
fix it in a follow-up commit before C7, never a TEMPORARY line. U114 already covers
`text|bg|border-tone-*` in `.vue`; U125 covers maps (`.ts` too), other utility prefixes, CSS-var
use and the toned chip.

Rejected guard: toned Badge or theme tone on a non-status element (category chips coloured by
kind). No regex tells status from category; RS.1 review holds it.

### C8 `docs: architecture for P672 Part 7`

`docs/ARCHITECTURE.md`:
- line 1463 (status tones): append "Agent liveness `working` is ok; `unseen` and `waiting` info;
  `input` warn. `@theme/lib/statusTone` holds the map (`StatusTone`, `TONE_BADGE`, `TONE_TEXT`,
  `TONE_DOT`, `CHANGE_REQUEST_TONE`: open info, merged ok, draft and closed neutral; git-ui badges
  and ADE read it). Off-board ADE renders a board `Tone` through `THEME_TONE`; an AdeChip with a
  tone is board-only (`:color` status chips go anywhere). U125 fails an ADE pastel map, tone
  token or toned AdeChip off the board."
- line 780: unchanged (tone vocabulary already right).
- line 2847: "ade/v2/tones.ts and palette.ts are dark-only and serve the plan board and task
  cards only (D1); …" (rest unchanged).
- line 4456 known item: after "dark-only (P138)" sentence add "Since P672 Part 7 only the plan
  board and task cards use the kept tones; every other `ade` surface uses theme tones." Item stays
  open (board pastels remain).
- line 1435: `U83-U124` to `U83-U125`.
- line 1470: `U110-U124` to `U110-U125`, `U118-U124` to `U118-U125`.

### C9 `docs: P672 Part 7 done`

`docs/v0.3.17/SPEC.md` Part 7 row: Status Done; Result names commits C1-C8 (and any spec or fix
commits), theme piece `statusTone.ts`, guard U125, the 19 baseline lines removed (12 left: Part 8
7, Part 9 5), files touched, the RL.2 muted reading, specs run, visual baselines for CI to
re-record. Last commit of the phase.

Any spec update or fix commit lands before C8.

## Test split

UI-only phase (no IPC, no Go, no flow test change). Frontend half: existing UI specs, updated only
where a selector or asserted tone changed. No new unit test (maps are lookups, no complex logic).
No new spec: guards U114, U125 are the regression check.

Spec updates expected:

- `tests/ui/ade-v2-issue-tracker.spec.ts:35`: link pill `data-tone` `'blue'` to `'info'`.
- `tests/ui/ade-v2-workflow-steps.spec.ts:75`: `row.locator(t('ade-chip'))` "Agent step" to
  `row.locator('[data-slot="badge"]')` (or the row's Badge by text).
- `tests/mobile/ui-leftovers.spec.ts:52`: `needs-item` `ade-chip` to `[data-slot="badge"]`; :54
  (`plan-task`, board) stays.
- `tests/unit/ade-v2-link-groups.spec.ts`: `LinkItem.tone` stays `Tone`; must pass unchanged
  (issue tones now come from `artifacts/status.ts`, same values).
- `tests/unit/ade-v2-board-parity.spec.ts`: board tags and base marker unchanged; must pass.
- Stays: `ade-v2-plan.spec.ts:84-100` (board branch tags from `actions.ts`, untouched), :543,
  `mobile/tabs.spec.ts:66`, `ade-v2-plan.spec.ts:623` (AdeBranchTip), `ade-v2-task-status.spec.ts:69`
  (`data-color`), `ade-v2-workflows.spec.ts:232-248` (edge `data-tone`), `status-agents:89`,
  `agent-usage:130,133`.
- Op status specs check testid `ade-op-status` and `data-op` only (agent-ops, background-ops,
  repo-required, force-push, sessions-menu, review-step, op-kinds); a class or variant assertion,
  if found, updates to `info`.

Targeted run near phase end (one pass, from `apps/kira-space`):

- ADE: ade-v2-plan, ade-v2-panel, ade-v2-task-panel, ade-v2-archived, ade-v2-agent-ops,
  ade-v2-background-ops, ade-v2-repo-required, ade-v2-force-push, ade-v2-sessions-menu,
  ade-v2-review-step, ade-v2-op-kinds, ade-v2-issue-tracker, ade-v2-links-strip, ade-v2-workflows,
  ade-v2-workflow-steps, ade-v2-task-status, ade-v2-notes, ade-v2-proof-chain, ade-v2-leases,
  ade-v2-settings, status-agents, agent-usage.
- Git: repo-graph-change-request-badge, repo-graph-change-request-index, repo-commit-meta,
  git-host-azure, git-host-provider, github-links-in-app, git-bound-review.
- Shared: automations-scripts, automations-recurring, memory-import, ui-leftovers.
- Studio (under `studio/`): kubernetes, grpc and http request specs, datagrip import, project tree
  connection specs, stream, console explain, docker-module.
- Mobile (`tests/mobile/`): ui-leftovers, tabs, needs specs.
- Unit: ade-v2-link-groups, ade-v2-board-parity.

Implementer greps `tests/` for every changed testid, file name, `ade-chip`, `data-tone` and
`tone-` first and adds any spec found to this run. A timeout under load gets a lone rerun first.

Visual baselines for CI to re-record (PR CI owns `visual`): `plan-*` (PR state colour, running
step dot, activity icons), `branch-panel-details`, `plan-branches-review`, `workflow-graph*`,
`review-step-panel`, `ade-sessions-menu`, `status-bar-agents-*`, `runs-*`, git graph and
`commit-meta` shots with a PR badge, kubernetes kind bar, plus any ADE panel, needs, links or
mobile shot the run flags.

Sandbox lessons for spec edits:

- Playwright click on a menu item inside Monaco needs `locator.hover()` first.
- reka radio arrow presses need `keyboard.press('ArrowDown', { delay: 40 })`.
- N17 (`scripts/check-conventions.ts`) forbids the `dialog` variant on a delete/discard label
  without an allow marker; C1 `AdeTaskActionButton` uses `dialog-danger` for red actions, so no
  delete label lands on `dialog`.
- N10 caps `.vue` files at 600 lines: watch `AdeBranchPanel.vue` (571), `ResponsePane.vue` grpc
  (557), `CommitMeta.vue` (474). AdeChip-to-Badge swaps are line-neutral; Alert swaps add 1-2.
- U91 exemptions go stale when their pattern vanishes: recheck guard output for any stale exempt
  after each commit (U114, U23, U26 included).

## Risks

- Board visuals change where shared pieces moved to RS.1: running step dot amber to blue
  (`stepBlocks.ts`), PR state colour on `AdeBranchPr` (open green to blue, merged purple to green),
  checks glyphs and activity icons on theme tones, `AdeParentChip`/`AdeOrchestratorBadge` on theme
  Badge. D1 limits pastels, not theme tones; board branch tag "Merged" stays purple
  (`actions.ts`, mockup parity), so board PR text and tag can differ in hue. Accepted, noted in
  Result.
- `CHANGE_REQUEST_TONE` closed is neutral (stopped, not failed); git graph users lose the red
  closed and violet merged cue. RS.1 decides; noted in Result.
- `AdeActivityIcon` unseen ring becomes a 1px outline at full info; visually stronger than the
  30% ring. Visual baseline check.
- `AdeTaskActionButton` agent `▶ Run` loses its agent-coloured outline in the panel. RCo.1.
- `row.tag` in `AdeBranchPanel` may carry `color` (status chip): keep the AdeChip `:color` branch
  so U125 stays clean and the status colour survives.
- `THEME_TONE.purple = 'ok'` assumes purple means merged in every board source mapped off-board;
  review mapped sites (`actions.ts` tags, PR) and use a direct `StatusTone` where purple means
  something else (review `question` kind already goes `default`).

## Out of scope

- Plan board pastel tables and `AdeTaskCard` look beyond the shared pieces above (D1).
- A light theme or light tone variants (Known open item stays).
- EmptyState media icons, icon sizes, type roles: Part 9. U115/U117 baseline lines: Parts 8 and 9.
- `text-fg` beyond files this Part touches.
- Any i18n, behaviour or IPC change.
