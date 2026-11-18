# P672 Part 7 iter2: status and tones

Iteration of `P672 Part 10` (master `plans/P672-part10.md`). Planned against `42a1885ba`. UI only:
no Go, no bridge, no new unit tests. First pass: `plans/P672-part7.md`.

## Rules applied

ARCHITECTURE "UI uniformity rules (P672)":

- RS.1 status tone map:
  - running or in progress: info, plus Spinner;
  - succeeded: ok;
  - waiting, attention, partial or stale: warn;
  - failed, broken or conflict: error;
  - idle, stopped, draft or unknown: neutral.
- RS.2: a row or header uses Badge, a dense list uses StatusDot, prose uses tone text.
- RS.3 / D1: ADE pastels stay on the plan board and task cards.
- RCo.1: status colour comes only from RS.1 tones.
- RI.2: icons are muted by default. A status icon follows RS.1: `warning` is text-warn, `error` is text-error, `pass` is text-ok.
- RL.4: a row status icon follows RS.1.
- `@theme/lib/statusTone` holds the shared map (`StatusTone`, `TONE_BADGE`, `TONE_TEXT`, `TONE_DOT`, `CHANGE_REQUEST_TONE`).

Deliberate exceptions kept, none re-opened:
- git ViewToolbar detail head;
- Studio tab view head;
- AdeTaskCard per D1;
- bar toggles staying `segmented`;
- engine cards;
- toasts per D3.

Accessibility is not a goal, so no item is a11y-only.

## Current state (fresh scan, rerunnable)

Scan scripts live in the scratchpad dir `p7i2/` (`files.sh`, `maps.py`, `icons.py`). Every count covers the six guard DIRS minus `components/ui/`, specs excluded: 1559 `.vue`/`.ts` files.

codegraph_explore calls (8) covered:
1. statusTone, TONE_BADGE, TONE_TEXT, TONE_DOT, changeRequestTone, StatusDot, Badge variants;
2. the DataGripImportDialog engine colour map and kinds.ts;
3. AdeAuditEntry, ConfigSyncUnitRow, ApprovalItemCard, RunState, runOps, HostViewSidebar;
4. OpStatus, the operations panel status tone and the StatusDot op row;
5. opLog.ts STATUS_ICON and AdeLinkTarget repoTone;
6. ChangeRequestView, IssueSidebar and IssueView;
7. issueFormat, ContainerDetail, AdePortImportDialog, ConnectionMaskReview and ImportFileRow;
8. connTextClass callers.

Scan results:
- `bash scripts/check-ui-primitives.sh` is green: highest guard U146, 48 PERMANENT lines.
- Stock Tailwind palette colours: 0. `text|bg-destructive`: 0.
- Object-literal tone maps with 2 or more entries: 32 files (`maps.py`). Each was read and checked against RS.1 and its siblings.
- CodiconIcon tags: 431 (`icons.py`). One `spin` attribute exists, and it is a dead prop.
- Badge variant counts: none 68, warn 39, chip 23, info 17, default 16, count 8, error 7, ok 4, plus about 55 bound `:variant`.
- `text-(warn|note)-text` outside Alert: 6 hits in 4 files (F1).
- A tone class on a toolbar Button or TooltipIconButton: 7 files (F14).
- A toned action icon (play, debug-stop): 2 files (F15).
- `is-live` class: 3 hits. No CSS reads it anywhere.

## Findings

### F1. `text-warn-text` outside Alert

`warn-text` (status-yellow) is the Alert-internal readable warn. Plain warn text elsewhere uses `text-warn`, for example in AdeLinkStrip and AdeSessionsRow.

Change these to `text-warn`:
- `apps/kira-space/frontend/src/ade/v2/request/BranchRequestBody.vue:173`;
- `apps/kira-space/frontend/src/ade/v2/workflows/AdeStepNode.vue:203` and `:248`;
- `apps/kira-space/frontend/src/ade/v2/panel/AdeRunSandboxBadge.vue:48` (return value) and `:119`;
- `packages/workbench/src/approvals/ApprovalItemCard.vue:28`, removed by F2.

The `--kira-warn-text` and `--kira-note-text` tokens stay, because the Alert warn and info variants read them.

### F2. ApprovalItemCard local tone maps

`packages/workbench/src/approvals/ApprovalItemCard.vue:22-37` defines four local maps: TONE_TEXT, RISK_TEXT, RISK_ICON and RISK_GLYPH.
- RISK_TEXT and RISK_ICON are TONE_TEXT keyed by the ApprovalRisk tone (`'warn'|'neutral'|'error'`, `registry.ts:6`). RISK_TEXT also differs, through the warn-text of F1.

Change:
1. Import `TONE_TEXT` and `StatusTone` from `@theme/lib/statusTone`.
2. Delete RISK_TEXT and RISK_ICON. Lines :143 and :153 read `TONE_TEXT[risk.tone]`.
3. Replace the local operation TONE_TEXT with `OP_TONE: Record<'neutral'|'caution'|'danger', StatusTone> = { neutral: 'neutral', caution: 'warn', danger: 'error' }`. Line :118 reads `TONE_TEXT[OP_TONE[item.operation.tone]]`.
4. RISK_GLYPH stays: it maps a glyph, not a colour.

### F3. ChangeRequestView state badge contradicts CHANGE_REQUEST_TONE

`apps/kira-space/frontend/src/hostview/ChangeRequestView.vue:50-61` has a `stateVariant` map:

| State | ChangeRequestView | git-ui and ADE (CHANGE_REQUEST_TONE) |
|---|---|---|
| open | ok | info |
| merged | info | ok |
| closed | error | neutral |
| draft | chip | neutral |

Change: bind `:variant="TONE_BADGE[changeRequestTone(detail.state)]"` and delete `stateVariant`.

### F4. Issue status tone duplicated

Two maps hold the same issue status tone:
- `apps/kira-space/frontend/src/hostview/issueFormat.ts:9-18` `statusVariant`: done ok, indeterminate info, everything else `chip`.
- `apps/kira-space/frontend/src/ade/v2/artifacts/status.ts:26-33` `ISSUE_CATEGORY_TONE`: board Tone.

Change:
1. Add to `packages/theme/src/lib/statusTone.ts`:
   - `ISSUE_CATEGORY_TONE: Readonly<Record<string, StatusTone>> = { new: 'neutral', indeterminate: 'info', done: 'ok' }`;
   - `issueTone(category?: string): StatusTone`, which returns `'neutral'` for a missing or unknown category.
2. In `artifacts/status.ts`, `issueCategoryTone` returns `PASTEL[issueTone(c)]`. The link pills keep the board Tone.
3. `hostview/IssueView.vue:45` and `hostview/IssueSidebar.vue:17` use `TONE_BADGE[issueTone(...)]`.
4. Delete `statusVariant` from issueFormat.ts, keeping the file's other exports.
5. `ade/v2/.../AdeTaskIssueLine.vue:72` simplifies to `TONE_BADGE[issueTone(issue.statusCategory)]`.

Visible change: the hostview `new` badge goes from chip to default.

### F5. HostViewSidebar status icons are uncoloured

`apps/kira-space/frontend/src/hostview/HostViewSidebar.vue` renders two maps with no tone: CHECK_ICON (:21-28, rendered at :127) and REVIEWER_ICON (:14-20, rendered at :108). Its sibling `ade/v2/plan/glyphs.ts:94-119` CHECKS_GLYPH tones the same states.

Change:
1. Add to statusTone.ts:
   - `CHECK_TONE`, keyed lowercase: success ok, failure error, error error, pending neutral, expected neutral, skipped neutral, neutral neutral;
   - `checkTone(state: string): StatusTone`, which lowercases the state and falls back to neutral.
2. `glyphs.ts` CHECKS_GLYPH classes come from `TONE_TEXT[checkTone(key)]`, with the same output as today.
3. HostViewSidebar: the check icon gets `:class="TONE_TEXT[checkTone(c.state)]"`.
4. HostViewSidebar: add a local `REVIEWER_TONE` (approved ok, changesRequested warn, rejected error, commented neutral, pending neutral). The reviewer icon reads `TONE_TEXT[...]`.
5. Mergeable line at :85-93: `<p ... data-testid="host-view-mergeable">` shows conflicting vs clean as plain text, and RS.1 says conflict is error.
   - Render a Badge: `error` when conflicting, `ok` when clean.
   - Keep the testid and the `:data-mergeable` attribute.
   - A tone on the `<p>` would trip U25, which is why this becomes a Badge.

### F6. AdeAuditEntry spinner and pending tone

File: `apps/kira-space/frontend/src/ade/v2/details/AdeAuditEntry.vue`.
- :101 `<CodiconIcon ... name="loading" spin :size="12" />`. CodiconIcon has no `spin` prop, so the icon is static. Change it to `<Spinner :size="12" label="" />`, as AdeOpStatus does.
- :26-30 `OUTCOME_VARIANT`: pending is `info`. A pending approval waits on the user, which RS.1 calls warn; sibling AdeProofEntry pending is warn, and this map's own fallback is warn. Set pending to `warn`.
- Retype the map as `Record<..., StatusTone>` and read it through `TONE_BADGE`.

### F7. Op log success icon is muted

`packages/workbench/src/lib/opLog.ts:50-55` STATUS_ICON: ok `pass` uses `text-muted-foreground`. RI.2 says `pass` is text-ok.

Change the ok tone to `text-ok`, read as `TONE_TEXT.ok`. OpLogDetail:120-124 renders it per step. Running already renders a Spinner in OpLogPanel.

### F8. Docker container state tone duplicated

`packages/docker-ui/src/components/ContainerDetail.vue:80-92` stateVariant (running ok, paused and restarting warn, dead error, else default) duplicates `packages/docker-ui/src/lib/format.ts:15-20` `stateDotTone`.

Change:
1. Replace `stateDotTone` with `containerStateTone(state): StatusTone`.
2. The ContainerDetail Badge reads `TONE_BADGE[containerStateTone(c.state)]`. Delete stateVariant.
3. These StatusDot callers read `TONE_DOT[containerStateTone(...)]`:
   - ContainerDetail:140;
   - ContainerTable:152;
   - ContainerRow:57;
   - ResourceDetail:171.

### F9. Memory import state tones

- `packages/workbench/src/memory/import/ImportFileRow.vue:18-30`: the default branch, which includes `pending`, goes to info. Pending is waiting, not running.
  - pending: neutral (default variant);
  - extracting, verifying and finalizing: info;
  - extracted: info;
  - done: ok;
  - failed: error;
  - skipped and cancelled: neutral.
  Express it as a `Record<FileState, StatusTone>` read through TONE_BADGE.
- `ImportJobDetail.vue:92`: `<span data-testid="import-job-state">` shows the job state as plain text in the DetailHeader `#meta`. RS.2 says a header uses Badge.
  - Change it to a Badge with a local `JOB_TONE`: scanning info, awaiting warn, running info, paused warn, done ok, cancelled neutral, failed error.
  - Keep the testid and the text.

### F10. Change-kind tone maps disagree

Five maps tone the same added/changed/removed concept:

| Map | Disagreement |
|---|---|
| `studio/.../ResponseDiffDialog.vue:169-174` HEADER_STATUS_CLASS | none |
| `.../ProofChangePanel.vue:47-52` DIFF_VARIANT | none |
| `.../fileTreeModel.ts:30-38` STATUS_COLOR_CLASS | none |
| `apps/kira-space/frontend/src/configsync/ConfigSyncUnitRow.vue:33-41` KIND_TONE | modify info, delete warn, conflict warn |
| `apps/kira-space/frontend/src/repo/RepoTreeRow.vue:38-48` | `A` is text-warn, though its own comment says it should be green-ish |

Change:
1. Add `CHANGE_TONE: Readonly<Record<'added'|'changed'|'removed'|'unchanged', StatusTone>>` to statusTone.ts: added ok, changed warn, removed error, unchanged neutral.
2. Make all five read it, through TONE_TEXT or TONE_BADGE:
   - ConfigSync: add maps to added, modify to changed, delete to removed. Conflict is error (RS.1); blocked stays error; unchanged and unmatched are neutral.
   - fileTreeModel: added, renamed and copied map to added; modified and typeChanged to changed; deleted to removed. Unmerged stays error.
   - RepoTreeRow: A is added (ok), M is changed, D is removed and keeps `line-through`, `?` stays muted.

### F11. Connection icon fallback differs between sibling views

- `connTextClass(...) ?? 'text-info'` in `apps/kira-space/frontend/src/studio/views/shared/keyvalue/KeyValuePane.vue:65`.
- `?? 'text-muted-foreground'` in DataView:67, StreamView:33 and DocumentView:90.
- `apps/kira-space/frontend/src/studio/workbench/panels/StudioStart.vue:53-58` iconColorClass: document kinds get the connection colour or muted; every other kind is always `text-info`.

Change:
1. Add `connIconClass(color?: string): string` to `packages/theme/src/connColor.ts`, returning `connTextClass(color) ?? 'text-muted-foreground'`.
2. Use it in all five call sites. StudioStart applies it to every kind and drops the info branch.

### F12. RefreshButton pending dot uses a data colour for a status

`packages/git-ui/src/components/RefreshButton.vue:86-90`: `<StatusDot ... color="var(--kira-focus)">`. StatusDot `color` is for data colours, and the sibling "new content" dot AdeUpdatesDot uses `tone="info"`.

Change: replace the colour with `tone="info"`.

### F13. Unsaved markers

- `apps/kira-space/frontend/src/studio/views/httprequest/HttpRequestView.vue:320-327` and `apps/kira-space/frontend/src/studio/views/grpcrequest/GrpcRequestView.vue:285-292` draw a `•` text glyph: `<span class="text-warn text-kira-lg leading-none" role="img" data-testid="http-dirty|grpc-dirty">`.
  - Change both to `<StatusDot tone="warn" :label="t('common.state.unsavedChanges.status')" data-testid="...">`, as in sibling `kibana/DiscoverSavedControls.vue:170`.
- Docker marks the same state muted:
  - `packages/docker-ui/src/components/EditField.vue:43`, changed dot `tone="muted"`;
  - `ContainerEditView.vue:249`, tab dirty dot `tone="muted"`.
  - Change both to `tone="warn"`, so every unsaved marker is a warn StatusDot.

### F14. One pattern for toned toolbar buttons

The toolbar Button variant colours through `data-[tone=error|ok]`, and that colour holds on hover. A `text-error` class loses to `hover:text-fg`. Today there are three patterns:
- a class `{ 'text-error': running }` on the stop button: ConsoleView:260, DataView:291, StreamToolbar:183, DocumentToolbar:71;
- a dead `{ 'is-live': running }` class on the stop button: KeyValuePane:176, HttpRequestView:369, GrpcRequestView:334;
- the prop pattern siblings already use: `tone="error"` (ContainerTable:208, MaskRuleRow:72) or `data-tone="error"` (ContainerRow:99 and others).

Change:
1. Extend the tone to warn:
   - `packages/theme/src/components/ui/button/index.ts:18` toolbar variant: add `data-[tone=warn]:text-warn data-[tone=warn]:hover:text-warn`;
   - `packages/theme/src/components/TooltipIconButton.vue:38`: `tone?: 'ok' | 'warn' | 'error'`.
2. The seven live stop buttons use `:tone="running ? 'error' : undefined"`. Drop the `text-error` and `is-live` bindings.
3. `ade/v2/workflows/inspector/AdeStepResults.vue:95`: `class="text-error"` becomes `tone="error"`.
4. `studio/views/console/kibana/QueryBar.vue:98`: the toolbar Button `class="text-error"` becomes `data-tone="error"`.
5. `studio/views/grid/DataToolbar.vue:327`: `:class="... 'text-error' : ... 'text-warn' : ''"` becomes `:tone="rt?.countError ? 'error' : rt?.count?.stale ? 'warn' : undefined"`.

### F15. Toned action icons in docker buttons

Action icons carry tone classes:
- `packages/docker-ui/src/components/ContainerDetail.vue:153` play `text-ok` and :164 debug-stop `text-error`;
- `packages/docker-ui/src/components/EngineOverview.vue:187` and :198, the same pair.

Siblings ContainerRow:103 and ContainerGroupRow:56 render untoned icons and tone the stop button with `data-tone="error"`.

Change: drop the icon classes. The stop Button gets `data-tone="error"`, matching ContainerRow. Start stays untoned, because RI.2 makes an action icon muted, and ContainerRow does not tone start.

### F16. gRPC chip uses the info status tone as a category colour

- `apps/kira-space/frontend/src/studio/api/CollectionRow.vue:158-165`: `:variant="grpcMethodClass(row.method)"`. `packages/shared/domain/grpc.ts:124` always returns `'info'`.
- `GrpcRequestView.vue:277-283`: `Badge variant="info"` (`grpc-method-chip`).

The HTTP sibling chip uses `Badge variant="chip" data :class="methodTextClass(token)"` (`methodColor.ts`, where `other` is `text-conn-grey`).

Change: both gRPC chips become `variant="chip" data :class="methodTextClass('other')"`. Delete `grpcMethodClass` and its export.

### F17. Local maps that duplicate TONE_TEXT

- `packages/workbench/src/components/StatusBarItem.vue:27` TONE_CLASS (info, warn, error);
- `apps/kira-space/frontend/src/workbench/AgentUsageItem.vue:60` TONE_CLASS (warn, error).

Change: both read `TONE_TEXT`, and the local maps are deleted.

### F18. Duplicate engine accent map

`apps/kira-space/frontend/src/studio/project/DataGripImportDialog.vue:33-45` `KIND_ACCENT_CLASS` is an exact copy of the exported map in `studio/project/connection/kinds.ts:19-29`.

Change: import it from kinds.ts. The use at :238 stays.

## Exemptions re-checked (no change)

- Board pastels (D1):
  - mobile PlanTaskCard `:color="var(--color-tone-*-solid)"`, a board card;
  - AdeTaskChildren and AdeTaskCard.
- Data colours through StatusDot `color`:
  - HostViewSidebar label `#color`;
  - mobile NeedsScreen and PlanTaskCard card colour.
- `OpLogPanel bg-error/15`: U26 PERMANENT, error log row wash.
- AppToolbar:373 auto-fetch-stopped `warning` icon text-warn: a status icon, as RI.2 allows.
- ProjectTree:313 broken-file link Button text-error: an error list in a link variant, not a toolbar tone.
- These maps already follow RS.1, so they are left alone:
  - AdeStepBlock and AdeRunOutcome STATE_TONE (now is info: the Part 6 iter2 handoff check passes);
  - ConnectionMaskReview, AdePortImportDialog, RunStatusBadge, AdeBranchPanel and AdeWorkflowReviewPanel;
  - runOps `blocked` to error (OpStatus has no warn).
- Domain thresholds stay separate: docker usage 70/90 and agent usage 80/95.
- Connection-state siblings agree: TreeRow `connecting` warn pulse and AppShell `reconnecting` warn.
- Shared domain colour helpers: HTTP statusClass and grpcCodeClass.
- PK/FK key marks (warn/info) are one DB-key convention across ColumnsSection, ConstraintsSection, FkPreviewPopover and SlickGridHost. They are consistent, so there is no finding.
- ExplainResultView verdict and issue tones: the parent is toned and the icon matches RS.1.
- Monaco decoration `var()` colours (reviewDecorations.ts) are outside the DOM class system.
- Not a tone matter, handed to the final scan: MemoryPanel:217 and MemoryStart:64 share an inline origin ternary (user info, external warn).

## Guards (provisional ids, dry-run counts before and after fixes)

| Id | Files | Message | Regex | Now | After |
|---|---|---|---|---|---|
| G7a | `vt` | readable warn or note token outside Alert (use text-warn or text-info, RCo.1) | `${TOK}text-(?:warn\|note)-text(?![\w-])` | 4 files | 0 |
| G7b | `v` | spin attribute on CodiconIcon (use Spinner, RS.1) | `<CodiconIcon\b(?&t)\s:?spin\b` | 1 | 0 |
| G7c | `v` | tone class on a toolbar button (use tone or data-tone) | `(?:<TooltipIconButton\b\|<Button\b(?=(?&t)\svariant="toolbar"))(?&t)\s:?class="[^"]*${TOK}text-(?:ok\|warn\|error\|info)(?![\w-])` | 7 | 0 |
| G7d | `v` | toned action icon (action icons stay muted, RI.2) | `<CodiconIcon\b(?=(?&t)\sname="(?:play\|debug-stop\|debug-start\|run-all\|debug-restart\|debug-pause)")(?&t)\s:?class="[^"]*${TOK}text-(?:ok\|warn\|error\|info)(?![\w-])` | 2 | 0 |
| G7e | `v` | StatusDot status through a theme var colour (use tone) | `<StatusDot\b(?&t)\scolor="var\(--kira-` | 1 | 0 |

Notes:
- The script never scans `components/ui`, so G7a needs no exempt for Alert.
- G7e matches only a literal `--kira-` var. The bound board and label `:color` uses stay legal.
- The implementer re-runs each regex on the post-fix tree, and every guard lands with 0 offenders.
- PERMANENT stays at 48.

## Docs updates

All in shared files.

`docs/ARCHITECTURE.md`:
- Status tones line (1464). Extend the statusTone map list with:
  - `ISSUE_CATEGORY_TONE`/`issueTone`;
  - `CHECK_TONE`/`checkTone`;
  - `CHANGE_TONE` (added ok, changed warn, removed error).
  Add these facts:
  - "Unsaved marker is StatusDot warn."
  - "StatusDot `color` only for data colours."
  - "warn-text and note-text are Alert-only."
  - "Toolbar button tone through `tone` / `data-tone` (ok, warn, error), never a class."
  - "Connection icon colour `connIconClass` (fallback muted)."
- Guard ranges: lines 1436 and 1471 become `U83-U<last>` and `U110-U<last>`, with the ids the orchestrator assigns.

`scripts/check-ui-primitives.sh`:
- Add the header line `# Guards <ids>: docs/v0.3.17/plans/P672-part7-iter2.md.`

`docs/v0.3.17/SPEC.md`:
- Add the progress line `Part 7 iter2: <result>, plan plans/P672-part7-iter2.md.`

## Stream decision

Use one sequential implementer. F4, F5 and F10 add shared statusTone.ts exports that later findings read, and F14 edits the Button primitive.

## Commits

| Commit | Content |
|---|---|
| C1 `refactor: shared issue, check and change tones in statusTone` | statusTone.ts additions (F4, F5, F10 maps) and connIconClass (F11). No callers yet; typecheck green. |
| C2 `fix: change request, issue and check status tones follow statusTone` | F3, F4, F5 (hostview, artifacts/status.ts, AdeTaskIssueLine, glyphs.ts) |
| C3 `fix: change-kind tones from CHANGE_TONE` | F10 |
| C4 `fix: warn text, approval and audit tones` | F1, F2, F6, F7, F9, F17 |
| C5 `fix: toolbar button tone through prop, docker action icons muted` | F14 (including the Button/TooltipIconButton warn tone), F15 |
| C6 `fix: unsaved markers, refresh dot, connection and gRPC chip colours` | F8, F11, F12, F13, F16, F18 |
| C7 `chore: guards for tone classes, spin, warn-text and status dot colour` | G7a-G7e, script header, ARCHITECTURE |

The SPEC progress line is the orchestrator's.

## Test split

Targeted UI specs only, run once near the end. Before running, grep the testids touched to pick the specs:
- `host-view-mergeable`, `import-job-state`, `http-dirty`, `grpc-dirty`, `grpc-method-chip`, `toolbar-count`, `kibana-stop`, `docker-action-*`.

Expected areas:
- host-view and issue-view;
- config-sync;
- Studio http, grpc, data-view, documents, stream, console and kibana, keyvalue, start, proof window, datagrip import, collections;
- docker module;
- memory import;
- approvals, plus mobile approvals (run `bun run build:mobile` first);
- automations and op log;
- repo tree;
- ADE details audit, workflow inspector and run sandbox;
- git refresh button;
- status bar and agent usage.

Any spec asserting an old class (`text-warn-text`, `is-live`, a `text-error` class on a stop button, badge variant info on gRPC) or old text is updated to the new behaviour.

Other checks:
- Visual baselines re-record in CI.
- `bun run typecheck` and `bun run lint` (guards plus N10) after each commit.
- No new unit tests.

## Risks

- F3 and F4 change visible badge colours in hostview. The new colours are intentional (they align with git-ui and ADE).
- F10 changes ConfigSync modify from info to warn, and conflict and delete tones. The config-sync specs may assert variants.
- N10: HttpRequestView is 578 lines. F13 and F14 net-shrink it, so check it stays at or under 600 lines.
- F14 changes the Button primitive variant string. The U-guards on the primitive do not scan `components/ui`.

## Out of scope

- The required-field asterisk (`ParamsForm:59`, `AdeContextFields:53,94`) is a forms matter, handed to the final scan.
- The MemoryPanel and MemoryStart origin ternary.
- PK/FK key-mark centralisation.
- No a11y-only changes.

## Handoff

- Iteration 8: the ContainerEditView tab dirty dot (F13) sits in SecondaryTabs `#item`. Only its `tone` changes.
- Iteration 9:
  - F13 replaces a text glyph `•` with StatusDot;
  - F15 removes icon classes in docker buttons;
  - F6 swaps CodiconIcon for Spinner.
  Iteration 9 must not re-edit these lines in other ways.

## Files touched

Shared:
- `scripts/check-ui-primitives.sh`
- `docs/ARCHITECTURE.md`
- `docs/v0.3.17/SPEC.md` (orchestrator)
- `packages/theme/src/lib/statusTone.ts`
- `packages/theme/src/connColor.ts`
- `packages/theme/src/components/ui/button/index.ts`
- `packages/theme/src/components/TooltipIconButton.vue`

Overlap with iteration 8:
- `packages/docker-ui/src/components/ContainerEditView.vue` (tab dirty dot tone)

Overlap with iteration 9:
- `apps/kira-space/frontend/src/studio/views/httprequest/HttpRequestView.vue` (dirty glyph, stop tone)
- `apps/kira-space/frontend/src/studio/views/grpcrequest/GrpcRequestView.vue` (dirty glyph, stop tone, method chip)
- `packages/docker-ui/src/components/ContainerDetail.vue` (action icons, state tone)
- `packages/docker-ui/src/components/EngineOverview.vue` (action icons)
- `apps/kira-space/frontend/src/ade/v2/details/AdeAuditEntry.vue` (Spinner)

Own:
- `apps/kira-space/frontend/src/ade/v2/request/BranchRequestBody.vue`
- `apps/kira-space/frontend/src/ade/v2/workflows/AdeStepNode.vue`
- `apps/kira-space/frontend/src/ade/v2/panel/AdeRunSandboxBadge.vue`
- `apps/kira-space/frontend/src/ade/v2/artifacts/status.ts`
- `apps/kira-space/frontend/src/ade/v2/plan/glyphs.ts`
- AdeTaskIssueLine.vue (ADE task issue line)
- `apps/kira-space/frontend/src/ade/v2/workflows/inspector/AdeStepResults.vue`
- ProofChangePanel.vue
- `apps/kira-space/frontend/src/hostview/ChangeRequestView.vue`
- `apps/kira-space/frontend/src/hostview/issueFormat.ts`
- `apps/kira-space/frontend/src/hostview/IssueView.vue`
- `apps/kira-space/frontend/src/hostview/IssueSidebar.vue`
- `apps/kira-space/frontend/src/hostview/HostViewSidebar.vue`
- `apps/kira-space/frontend/src/configsync/ConfigSyncUnitRow.vue`
- `apps/kira-space/frontend/src/repo/RepoTreeRow.vue`
- fileTreeModel.ts
- ResponseDiffDialog.vue
- `apps/kira-space/frontend/src/studio/views/shared/keyvalue/KeyValuePane.vue`
- `apps/kira-space/frontend/src/studio/views/grid/DataView.vue`
- `apps/kira-space/frontend/src/studio/views/grid/DataToolbar.vue`
- StreamView.vue
- `apps/kira-space/frontend/src/studio/views/stream/StreamToolbar.vue`
- DocumentView.vue
- `apps/kira-space/frontend/src/studio/views/documents/DocumentToolbar.vue`
- `apps/kira-space/frontend/src/studio/views/console/ConsoleView.vue`
- `apps/kira-space/frontend/src/studio/views/console/kibana/QueryBar.vue`
- `apps/kira-space/frontend/src/studio/workbench/panels/StudioStart.vue`
- `apps/kira-space/frontend/src/studio/api/CollectionRow.vue`
- `apps/kira-space/frontend/src/studio/project/DataGripImportDialog.vue`
- `apps/kira-space/frontend/src/workbench/AgentUsageItem.vue`
- `packages/shared/domain/grpc.ts`
- `packages/workbench/src/approvals/ApprovalItemCard.vue`
- `packages/workbench/src/lib/opLog.ts`
- `packages/workbench/src/components/StatusBarItem.vue`
- `packages/workbench/src/memory/import/ImportFileRow.vue`
- `packages/workbench/src/memory/import/ImportJobDetail.vue`
- `packages/docker-ui/src/lib/format.ts`
- `packages/docker-ui/src/components/ContainerTable.vue`
- `packages/docker-ui/src/components/ContainerRow.vue`
- `packages/docker-ui/src/components/ResourceDetail.vue`
- `packages/docker-ui/src/components/EditField.vue`
- `packages/git-ui/src/components/RefreshButton.vue`
- Specs asserting changed classes or variants (implementer greps them).

The implementer resolves the full paths of the bare-named files with Glob before editing.
