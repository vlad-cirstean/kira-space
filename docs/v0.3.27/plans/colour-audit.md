# Colour audit: input to P766 (unify) and P767 (rollout)

Owner request: (a) apply the P764 faint-band design to Studio, API, repos and every other coloured
surface; (b) unify colour components so a later colour change is one edit.

Base: `906bff1d0`. P764 (`P764.md`, Pure-B: 10 inks, header-only band, neutral 1.5px ring, ripple
removed) is designed, not implemented. This audit assumes P764 lands first. Discovery used
`codegraph_explore` plus grep for exact lines. Frontend paths are under
`apps/kira-space/frontend/src/` unless rooted. Go paths are under `apps/kira-space/internal/` unless
rooted.

## 1. Inventory

### 1.1 Colour sources (what is stored, who picks it)

| Source | Stored as | Chosen by | Validated | Rendered through |
|---|---|---|---|---|
| Task | int slot `ade_tasks.color`, 0-19 (P764: 0-9) | server, `pickColorSlot` (`storage/repos/adetask.go:388`), farthest-first after P764 | `patchLeaves` range (`adetask.go:594-599`), `maxColorSlots` (`:60`) | `ade/v2/palette.ts:2-27` `PALETTE` hex + `taskColor` |
| Repo | name `code_repos.color` (13-name palette) | auto on import `palette.AutoRepoColor(sortOrder)` (`storage/repos/coderepos.go:79-81`, rotation `internal/palette/palette.go:14`); user via `SetRepoColor` (`bridge/codeworkspace.go:341-356`), picker `repo/RepoIdentity.vue:40`, menu `repo/RepoList.vue:230` | `palette.Valid` (`codeworkspace.go:345`, `storage/model/coderepos.go:40`, `storage/repos/coderepos.go:122`) | `state/coderepos.ts:54` `colorOf`; worktree follows anchor `repo/state/repoLinks.ts:22-25` `repoColorOf` |
| Connection (DB, Studio) | name in connection config file | user, `ConnectionGeneralTab.vue:151` | `model.ValidConnectionColor` (`connfiles/validate.go:87`, `storage/repos/connections.go:69`); file schema enum `connfiles/schema.go:42` | `connectionRecord(id).color` |
| API environment | name in api env file | user, `studio/api/EnvironmentFields.vue:47`; menus `EnvironmentSelect.vue:50,59`, `RequestVariablesPanel.vue:108`, `VariablesView.vue:96` | `model.ValidPaletteColor` (`bridge/variables.go:37`, coerce `storage/repos/variables.go:34`); schema `apifiles/rules.go:256` | `studio/api/state/variables.ts:118-123` `environmentColorForTab` |
| Script (Automations) | name in script file | user, `packages/workbench/src/automations/form/RunScriptTab.vue:78` | `palette.Valid` (`internal/scripts/scripts.go:211`); schema `internal/scripts/schema.go:45` | `script.color`; copied to terminal tab (`packages/workbench/src/automations/runScript.ts:51`, `run/RunScriptDialog.vue:109`) |
| Terminal tab | `terminalTabStateSchema.color` (`packages/shared/domain/tabs.ts:65`), never persisted | from script | zod enum | `packages/workbench/src/tabs/terminalTabKind.ts:48` |
| Task status | name `ade_task_statuses.color`, own 16-name palette | user, `ade/v2/workflows/statuses/AdeTaskStatuses.vue:198-201` | DB CHECK, `storage/model/adetask.go:414` `AdeStatusColors` | `ade/v2/statusColors.ts`, tokens `--kira-status-*` (`packages/theme/src/tokens.css:177-193`) |

Named palette (repo, connection, environment, script, terminal): `packages/shared/domain/color.ts:8-21`
(13 names storable), `:34-43` (8 offered). Inks `--kira-conn-*` at `oklch(0.72 0.075 h)`
(`tokens.css:161-175`), Tailwind `--color-conn-*` (`packages/theme/src/tailwind-core.css:71-82`).

### 1.2 Render sites

Treatment today: rail (4px left bar, `colorMarkClass('rail')`), bar (2x14 tab mark), dot (5px),
swatch (10px menu dot), band (2px line under a view toolbar, `colorMarkClass('band')`), line (3px
`ColorLine`), tint (alpha fill), ink text.

Tasks (all covered by P764 unless marked "missed"):

| Surface | File:line | Today |
|---|---|---|
| Plan card, day band, branch rows, details head, branch/archived panel head, review bar, files bar, list marks, base marker, phone plan dot | see `P764.md` §1 table | edge, outline, 3px line, mark, dot |
| Branch tooltip marks (missed by P764 list; follows `color` prop) | `ade/v2/plan/AdeBranchTip.vue:45,71-76` | 16x3 `ColorLine` mark |
| Phone needs dot (missed by P764 list; follows `taskColor`) | `frontend/mobile/screens/NeedsScreen.vue:106` | `StatusDot` |
| All-sessions group mark | `ade/v2/needs/AdeAllSessions.vue:131` | 16x3 mark |

Repos:

| Surface | File:line | Today |
|---|---|---|
| Git panel repo row | `repo/RepoList.vue:456-460` | rail |
| Git panel worktree row | `repo/RepoWorktreeRow.vue:49-53` | rail |
| ADE repos page list | `ade/v2/repos/AdeReposPage.vue:248-252` | rail |
| ADE repos page head | `ade/v2/repos/AdeReposPage.vue:316-319` (`ViewHead :color`) | dot + 2px band |
| Git module view head | `packages/git-ui/src/components/GitViewHead.vue:21,31-33` | dot + 2px band |
| Repo tabs (graph, file, diff, multi-diff) | `state/tabKinds.ts:101-104,112,129,163,178`, drawn `packages/workbench/src/components/TabStrip.vue:452` | bar |
| Repo chip | `ade/v2/AdeRepoTag.vue:10,14` (8 callers: review header, branch tip, run, gate, agent, services, archived, step block) | 12% tint + ink text (`state/coderepos.ts:16-26` `repoTint`) |
| Branch row repo chip | `ade/v2/plan/AdeBranchRow.vue:75,306-307,437-438` | 12% tint + ink text |
| Link target repo name | `ade/v2/AdeLinkTarget.vue:27,68-69` | ink text (`repoText`, `coderepos.ts:29-34`) |
| Project picker option label | `repo/state/projectOptions.ts:17` | ink text |
| Repo colour picker / menu | `repo/RepoIdentity.vue:40`, `repo/RepoList.vue:230` | swatch |

Studio, database:

| Surface | File:line | Today |
|---|---|---|
| Project tree row | `studio/project/TreeRow.vue:86,164` | rail |
| DB tabs (data, definition, console, document, keyvalue, stream, browse) | `studio/state/tabKinds.ts:92-94` + `TabStrip.vue:452` | bar |
| View heads: data, definition, console, document, keyvalue, stream, browse | `studio/views/grid/DataView.vue:71,250-254`, `definition/DefinitionView.vue:230,254-255`, `console/ConsoleView.vue:260-261` (colour `console/useConsoleLanguage.ts:21-23`), `documents/DocumentView.vue:91,344-345`, `shared/keyvalue/KeyValuePane.vue:149-150`, `stream/StreamView.vue:34,76-77`, `browse/BrowseView.vue:91,316-317` | dot + 2px band |
| Engine/kind icon in head | `DataView.vue:68`, `DocumentView.vue:89`, `KeyValuePane.vue:65`, `StreamView.vue:32` (`connIconClass`) | ink icon |
| Start panel recent tables | `studio/workbench/panels/StudioStart.vue:54,132` | bar + ink icon |
| Operations log row | `workbench/OperationsPanel.vue:374` | dot |
| Approvals dialog header | `packages/workbench/src/approvals/ApprovalsDialog.vue:72,141-144` | 3px `ColorLine under` |
| Approval item part | `packages/workbench/src/approvals/ApprovalItemCard.vue:183` | dot |
| Connection colour picker | `studio/project/connection/ConnectionGeneralTab.vue:151` | swatch |

Studio, API / HTTP client:

| Surface | File:line | Today |
|---|---|---|
| HTTP and gRPC request heads | `studio/views/httprequest/HttpRequestView.vue:82,307-311`, `grpcrequest/GrpcRequestView.vue:75,271-272`; colour env first, else connection (`studio/views/shared/request/useRequestChrome.ts:16-24`) | dot + 2px band |
| Request tabs | `studio/state/tabKinds.ts:92` (connection colour, not env) | bar |
| Environment select trigger | `studio/api/EnvironmentSelect.vue:88` | dot |
| Request variables panel | `studio/api/RequestVariablesPanel.vue:176,259` | dot |
| Variables matrix column head | `studio/api/VariablesMatrix.vue:347` | dot |
| Variables view owner | `studio/api/VariablesView.vue:159`; its `ViewHead` (`:129-133`) passes no colour | dot; empty 2px band slot |
| HTTP method colours | `tokens.css:204-211` aliases of `--kira-conn-*` | ink text (categorical) |

Other:

| Surface | File:line | Today |
|---|---|---|
| Automations script row | `packages/workbench/src/automations/ScriptRow.vue:54-58` | rail |
| Terminal / script tab | `packages/workbench/src/tabs/terminalTabKind.ts:48` + `TabStrip.vue:452` | bar |
| Context menu colour items | `packages/workbench/src/components/MenuItems.vue:49-50` | swatch |
| Swatch pickers (5) | `packages/theme/src/SwatchPicker.vue:40-54` (ring `outline-2 outline-offset-2 outline-fg`, `:48`) | swatch |
| Task status chip, dot, text, swatch | `ade/v2/AdeChip.vue:17`, `plan/AdeTaskStatusMenu.vue:52`, `plan/AdeTagIcon.vue:34`, `workflows/statuses/AdeTaskStatuses.vue:194-201`, phone `mobile/screens/PlanTaskCard.vue:76` | 15% tint + ink text, dot |
| Git graph lanes | `packages/git-ui/src/theme/git.css:20-27` aliases of `--kira-conn-*`; allocation `packages/git-core/src/graph/lanes.ts:133` | lane strokes (categorical) |
| ADE tones, status tones | `ade/v2/tones.css:5-17`, `packages/theme/src/lib/statusTone.ts` | semantic, not user colour |
| Desktop window ground | `internal/shell/window.go:77`, `internal/shell/popup.go:20` | fixed RGB, not user colour |
| Docker UI, api-core, git-core | `StatusDot` with tone only | no user colour |
| Phone (mobile) | only task colour (`mobile/state/useAgentsModel.ts:206`, `PlanTaskCard.vue:72`, `NeedsScreen.vue:106`) | dot |

## 2. Treatment under the faint-band design

Rule carried from P764: hue lives in a header band only; body, text, borders and controls stay
neutral; list rows keep a small mark; `none` shows no band (neutral chrome) and no mark.

| Surface | Treatment | Reason |
|---|---|---|
| `ViewHead` (all Studio view heads, request heads, ADE repos head, git module head) | band | It is a header. Toolbar ground takes the band; the 2px band line under it goes; the dot goes (Q3). One component change reaches 12 heads. |
| Approvals dialog header | band | Header; same as P764's review bar. Drop the 3px line. |
| Engine/kind icon inside a banded head | neutral (Q6) | Principle: controls neutral; hue already in the band. |
| Repo chip (`AdeRepoTag`, branch row chip) | band (Q4) | Chip ground becomes the repo's rest band, text fg. Today's 12% tint is a third tint recipe; the band reuses the proven contrast floor (4.86 subtle). |
| Tree rails, repo/worktree/script rows, ADE repos list | mark only | List rows, matching P764's list-row default. Selected row stays neutral. |
| Tab bars | mark only (Q5) | Tab chips already use active/selected grounds; a band would collide. |
| Dots (env select, variables, operations, approval parts, menus) | mark only | Inline marks too small for a band. |
| Variables matrix column head | mark only | Column head inside a table; a band per column would stripe the grid. |
| Link target and picker repo text | mark only (ink text) | Inline text; ink from the shared module. |
| Swatch pickers | leave | They show inks by definition. Source inks from the module. |
| Task status palette | leave (Q7) | Status meaning, not identity; own 16-hue set by design (P411). |
| HTTP method colours, git graph lanes | leave, decouple | Categorical. They alias `--kira-conn-*` today, so a palette change would move them; give them own values first (P766). |
| ADE tones, status tones, desktop window ground | leave | Not user colour. |

## 3. Duplication map and target layout

### 3.1 Every separate palette, helper and constant

Frontend:

| # | What | Where | Note |
|---|---|---|---|
| F1 | Named palette enum + offered subset | `packages/shared/domain/color.ts:8-46`; aliases `packages/shared/domain/connection.ts:81-84` | storage contract; keep whole |
| F2 | Named inks | `packages/theme/src/tokens.css:161-175`, `tailwind-core.css:71-82` | `oklch(0.72 0.075 h)` |
| F3 | Named class maps and marks | `packages/theme/src/connColor.ts:14-106` (`connColorVar`, `CONN_BG`, `CONN_TEXT`, `CONN_SWATCH`, `connIconClass`, `MARK_SHAPE`, `colorMarkClass`, `colourLabel`) | "conn" name is historic; serves all named sources |
| F4 | Task inks (P764 adds bands) | `ade/v2/palette.ts:2-27` | hex in app code; exempt in `scripts/check-ui-primitives.sh:228` (U34) and allowlisted in `scripts/check-ade-colours.sh:18-38` |
| F5 | Repo tint and text | `state/coderepos.ts:16-34` | 12% alpha recipe, duplicate of F3 text map |
| F6 | 3px line / mark | `packages/workbench/src/components/ColorLine.vue` | second mark system beside `colorMarkClass` |
| F7 | Raw colour dot | `packages/theme/src/components/StatusDot.vue:9,33` `color` prop | inline style, bypasses F3 |
| F8 | Task band component (P764) | `ade/v2/AdeTaskBand.vue` (new) | task-only by name and prop |
| F9 | Selection ring (P764) | `ade/v2/plan/taskGrid.ts` `TASK_EDGE_SELECTED`, `GROUND_SELECTED` | ade-local |
| F10 | Status palette | `ade/v2/statusColors.ts`, `tokens.css:177-193` | separate by design |
| F11 | Method and lane aliases | `tokens.css:204-211`, `packages/git-ui/src/theme/git.css:20-27` | couple to F2 |
| F12 | Guards and docs | `scripts/check-ade-colours.sh` (20 task hex), `scripts/check-ui-primitives.sh:228`, `scripts/check-theme-classes.sh:474,484-487`, `docs/ARCHITECTURE.md:53` | must follow any move |

Backend:

| # | What | Where | Note |
|---|---|---|---|
| B1 | Named palette set + `Names` + `AutoRepoColor` | `internal/palette/palette.go:7-35` | the intended single home |
| B2 | Duplicate named set | `storage/model/connection.go:106-132` (`paletteColors`, `ValidPaletteColor`, `ValidConnectionColor`) | same 13 names, second map |
| B3 | Task slot count + picker | `storage/repos/adetask.go:60,303-333,388`, range check `:594-599` | P764 sets 10 |
| B4 | Status colours | `storage/model/adetask.go:414` | DB CHECK mirror |
| B5 | File schema enums | `connfiles/schema.go:42`, `apifiles/rules.go:256`, `internal/scripts/schema.go:45` | already derive from `palette.Names()` |

### 3.2 What P764 already centralises

- Task inks and both band tones in one table (`palette.ts`), with `taskColor` and `taskBand`.
- One band component (`AdeTaskBand`) for details head, review bar and files bar.
- Card ring and selected-row constants in `taskGrid.ts`.
- `TitleBar` takes `class` (`packages/workbench/src/components/TitleBar.vue`), so a band can wrap it.
- All `ColorLine variant="under"` uses go (`AdePanelFrame`, `AdeReviewHeader`, `AdeTaskFilesWindow`).

Gap in P764 to fix when it is implemented: `scripts/check-ade-colours.sh` allowlists the 20 old hex
values and fails on unused entries, so it must list the 30 new values (10 inks, 10 rest, 10 hover)
in the same commit. `P764.md` does not mention it.

### 3.3 Target layout (what P766 builds)

Frontend, one module in `packages/theme`:

- `packages/theme/src/tokens.css`: one hue block, the only colour source (matches
  `ARCHITECTURE.md:50`). Per hue: `--kira-hue-<name>` (ink), `--kira-hue-<name>-band`,
  `--kira-hue-<name>-band-hover`, OKLCH recipe in a comment. Keep `--kira-conn-*` as aliases of the
  inks so `bg-conn-*` classes and specs (`tests/ui/color-rails.spec.ts:13`) keep working. Method and
  lane tokens get their own hex (today's values) so they no longer follow a palette change.
- `packages/theme/src/colour.ts` (replaces `connColor.ts` and `ade/v2/palette.ts`):
  - `type Hue` and `HUES` in farthest-first order (P764 slot order).
  - `hueOfSlot(n)` for tasks; `hueOfName(PaletteColor)` for named sources (`none` and `grey` return
    null or neutral; legacy names map per Q2).
  - `inkVar`, `bandVars(hue)` returning `{ '--band', '--band-hover' }` style, literal class maps for
    bg, text, swatch (Tailwind literal rule, `ARCHITECTURE.md:51`), `colorMarkClass` kept under its
    name (guards and ARCHITECTURE refer to it).
  - `SELECTION_RING` and `SELECTION_ROW` class constants (moved from `taskGrid.ts`).
  - `colourLabel`.
- `packages/workbench/src/components/ColourBand.vue`: P764's `AdeTaskBand` generalised; prop
  `hue: Hue | null`; `null` renders neutral chrome. `AdeTaskBand` is deleted, its users pass
  `hueOfSlot(task.color)`.
- Delete `state/coderepos.ts` `repoTint`/`repoText` (callers use the module), and `ColorLine.vue`
  once its marks move to `colorMarkClass` (Q5 on mark shape).
- `StatusDot` `color` prop takes a `Hue` (class map) instead of a raw CSS string.

Backend, `internal/palette` owns every colour fact:

- Delete B2; callers use `palette.Valid` (`connfiles/validate.go:87`, `storage/repos/connections.go:69`,
  `storage/repos/variables.go:34`, `bridge/variables.go:37`).
- `palette.TaskSlots = 10`; `storage/repos/adetask.go` reads it instead of `maxColorSlots`.
- Optional: `palette.StatusColors` replaces `model.AdeStatusColors` (no behaviour change).
- `AutoRepoColor` uses the same farthest-first order as task slots over the offered names.

After P766, a colour change is: edit the hue block in `tokens.css` (and the recipe test). A new hue
is: one token block, one `Hue` entry, one class-map line per map, one Go name if storable.

## 4. Work split

### P766: unify (no visual change except where Q1/Q2 change inks)

Files:

- `packages/theme/src/tokens.css`, `packages/theme/src/tailwind-core.css`: hue block, conn aliases,
  method/lane own values; `packages/git-ui/src/theme/git.css:20-27`.
- New `packages/theme/src/colour.ts`; delete `packages/theme/src/connColor.ts` (update ~25 importers:
  every file in §1.2 that calls `colorMarkClass`, `connIconClass`, `connColorVar`, `connSwatchClass`).
- Delete `ade/v2/palette.ts` (importers: `plan/usePlanModel.ts:280`, `board/baseMarker.ts:51`,
  `review/AdeReviewHeader.vue`, `files/AdeTaskFilesWindow.vue`, `frontend/mobile/state/useAgentsModel.ts:206`).
- New `packages/workbench/src/components/ColourBand.vue`; delete `ade/v2/AdeTaskBand.vue`.
- `ade/v2/plan/taskGrid.ts`: ring constants imported from the module.
- `state/coderepos.ts`: drop `repoTint`/`repoText`; `AdeRepoTag.vue`, `AdeBranchRow.vue`,
  `AdeLinkTarget.vue`, `repo/state/projectOptions.ts` switch to the module (same look).
- `packages/theme/src/components/StatusDot.vue`, `packages/workbench/src/components/ColorLine.vue`.
- `packages/shared/domain/color.ts`: offered set per Q2.
- Go: `internal/palette/palette.go`, `storage/model/connection.go`, `storage/repos/adetask.go`,
  callers listed in §3.3.
- Guards: `scripts/check-ade-colours.sh` (allowlist becomes empty, keep the script),
  `scripts/check-ui-primitives.sh:228` (drop the `palette.ts` exemption),
  `scripts/check-theme-classes.sh:474,484-487` (point messages at `colour.ts`).
- `docs/ARCHITECTURE.md:50-53` and the ade colour section.

Backend and migration impact:

- Refactor only: none.
- If Q2 adds stored names (for example `lime`, `sky`): `internal/palette` set, `color.ts` enum,
  i18n `common.colour.<name>.label`. File schema enums follow `palette.Names()` already. No SQL
  migration: named colour columns are plain `TEXT NOT NULL DEFAULT 'none'` with no CHECK
  (`storage/migrations/0090_p557_baseline.sql:314` for `code_repos`), validated in Go. Existing
  names stay storable (`color.ts:27-33` rule).
- Task slots: `ade_tasks.color` and the orchestrator table keep `CHECK (color BETWEEN 0 AND 19)`
  (`0090_p557_baseline.sql:354`, `0093_p571_orchestrator.sql:11`). 0-9 satisfies it; tightening
  needs a SQLite table rebuild and buys nothing while Go rejects >= 10. Leave it.
- If Q1 keeps stored names but maps them onto Pure-B hues at display: no backend change.

Risks:

- Method colours and graph lanes move if their aliases are not cut first.
- ~25 importers of `connColor.ts`; a missed one fails typecheck, not at runtime. Run knip
  (`bun run lint:dead`) after.
- Specs pin computed colours and class literals (`tests/ui/color-rails.spec.ts`,
  `tests/ui/studio/tree.spec.ts`, `tabs.spec.ts`, `connections.spec.ts`, `api-ui-consistency.spec.ts`,
  `ade-repos.spec.ts`, P764's ade specs). Keeping `bg-conn-*` names limits churn.
- Contrast for named hues if Q1 keeps muted inks: bands need their own recipe check.

Tests (split at IPC boundary):

- Backend: existing flows prove the stored value round trip: `flows/repoflow/import_test.go`
  (`SetRepoColor`), `flows/apiflow/variables_matrix_test.go` (environment colour),
  `flows/adeflow/twowindows_test.go:14` plus P764's `TestTaskColorSlotsTen`. Only with new names
  (Q2): add `TestPaletteNewNames` in `repoflow`: `SetRepoColor` accepts each new name, rejects an
  unknown one; a connection file with a new name validates.
- Frontend: UI half of the same scenario: picking the new name in `RepoIdentity` paints the repo
  rail with the new ink (extend `tests/ui/color-rails.spec.ts`). Existing ade/studio colour specs
  must pass unchanged when Q1/Q2 change nothing visible.
- Unit: extend P764's `tests/unit/ade-v2-palette-contrast.spec.ts` into
  `tests/unit/colour-palette.spec.ts`: parse the hue block from `tokens.css`, check each band against
  the recipe, fg/muted/subtle >= 4.5:1 on every band, pairwise band distance floors. Advanced logic
  (gamut clamp, OKLab distances), so it meets the unit-test bar.

### P767: rollout

Files:

- `packages/workbench/src/components/ViewHead.vue`: toolbar wrapped in `ColourBand` (or ground from
  `bandVars`); delete the band line (`:67`) and, per Q3, the dot (`:44-48`). Reaches every view head
  in §1.2 with no per-view edit; `GitViewHead.vue` and `AdeReposPage.vue:316` need none either.
- `packages/workbench/src/components/ViewToolbar.vue`: none (`class` already merges).
- `packages/workbench/src/approvals/ApprovalsDialog.vue:72,141-144`: band on the header, drop the
  line and the resize-measured inset (`:38-45`) if nothing else uses it.
- Studio views with `connIconClass` (`DataView.vue:68`, `DocumentView.vue:89`, `KeyValuePane.vue:65`,
  `StreamView.vue:32`): neutral icon class per Q6. `StudioStart.vue:54` keeps ink.
- `ade/v2/AdeRepoTag.vue`, `ade/v2/plan/AdeBranchRow.vue:306-307,437-438`: band chip per Q4.
- `scripts/check-theme-classes.sh:474`: `p-toolbar-rail` message (band line gone).
- `docs/ARCHITECTURE.md:53`: "band under view toolbars" becomes "band as view head ground".

Backend and migration impact: none.

Risks:

- `ViewHead` `bandTestid` and `dotTestid` are pinned by Studio specs (`tests/ui/studio/*`, 12 files
  match `conn-dot` or band ids); rewrite them to assert the head background equals the band token.
- Request heads switch colour between environment and connection (`useRequestChrome.ts:18-24`);
  the band must follow the same rule. Incognito tabs read a per-tab environment.
- A head with `color === undefined` (Variables view) must keep plain chrome, not an empty band.
- Engine icons lose their tint inside heads; StudioStart keeps it, so the same connection reads
  two ways (Q6).

Tests (split at IPC boundary):

- Backend half (no new behaviour, existing flows): repo colour (`repoflow/import_test.go`),
  environment colour (`apiflow/variables_matrix_test.go`), connection file round trip
  (`flows/dbflow/connfiles_test.go`). Name in the P767 plan which flow backs each UI spec below.
- Frontend half, same scenarios: Studio data view head of a cyan connection computes the cyan rest
  band and has no band line (`tests/ui/studio/tree.spec.ts` or a new `studio/view-head-band.spec.ts`);
  HTTP request head takes the environment band, falls back to the connection band
  (`http-raw.spec.ts`, `grpc-request.spec.ts`); ADE repos head and git module head take the repo band
  (`ade-repos.spec.ts`); approvals header band (`dbmcp-approval.spec.ts`); repo chip ground equals
  rest band with fg text (`ade-v2-plan.spec.ts`).
- Rails, bars, dots: `tests/ui/color-rails.spec.ts` keeps passing unchanged (mark-only surfaces).

Screenshots (owner delivery, deviceScaleFactor 2):

- New `apps/kira-space/tests/visual/colour-band.spec.ts` in the `visual` project (WebKit,
  `apps/kira-space/playwright.config.ts:132`), `test.use({ deviceScaleFactor: 2 })` as
  `tests/screenshots/phone.shot.ts:13` does. One `toHaveScreenshot` per changed surface, each hue
  where it matters:
  - Studio heads: data, definition, console, document, keyvalue, stream, browse, HTTP (env colour and
    connection fallback), gRPC; Variables view (no colour);
  - approvals dialog;
  - ADE repos page (list rails + head), git module head;
  - repo chips (branch row, branch tooltip, review header);
  - tab strip and tree rails (unchanged marks, for context);
  - P764's `ade-task-tint` set (board column of all 10 slots, details head, review bar).
- Before/after: run the same spec on the pre-P767 commit in a worktree with `--update-snapshots`,
  then on P767; attach both sets. Local run: `bun run build:test && bunx playwright test
  --config=apps/kira-space/playwright.config.ts --project=visual
  apps/kira-space/tests/visual/colour-band.spec.ts --update-snapshots`. Never beside a UI suite in
  the same checkout.
- Committed baselines come from PR CI, the sole baseline authority; local images are for review
  only. Existing baselines that show a view head need re-recording in CI: `tests/visual/studio/*`
  (data-view, console, http-request-view, variables, approvals*, workbench), `ade-repos`,
  `ade-repo-gate`, `git-module`, plus P764's list.
- Not the `screenshots` project: it serves site manifest ids (`site/src/data/shotCapture.ts`) and
  writes WebP for the published site.

## 5. Questions for the owner

1. One palette app-wide: do connections, environments, repos, scripts and terminals take the
   Pure-B inks and bands too (recommended), or keep today's muted inks with their own bands?
2. If one palette: offer the 10 Pure-B hues (+ no colour, + grey?) in every picker. Stored legacy
   names display as the nearest hue: olive as lime, cyan as sky, indigo as blue. Add `lime` and
   `sky` as stored names (recommended) or only relabel?
3. View heads: drop the colour dot once the band shows (recommended)?
4. Repo chips: band ground with neutral text (recommended), or keep coloured text?
5. Tabs and list rails stay small marks (recommended)? And one mark shape: today's 16x3 task mark
   and the 2x14 tab bar both exist.
6. Engine icon inside a banded head: neutral (recommended) or keep the connection ink?
7. Task status colours stay a separate palette (recommended)?
