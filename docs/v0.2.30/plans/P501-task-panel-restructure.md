# P501 plan: ADE task panel restructure

Row: `docs/v0.2.30/SPEC.md` P501. Base: `3d715ebd` (P496 and P500 landed). Backend changes (session origin, PR base/checks, artifact rename, empty-title paste), so the plan has a Go flow test, contract JSON and UI specs.

Paths: `web/` = `apps/kira-space/frontend/src/ade/v2/`, `int/` = `apps/kira-space/internal/`, `mcp/` = `internal/agentmcp/` (repo root package).

## Today (verified with codegraph_explore)

- `web/panel/AdeTaskPanel.vue`: `AdePanelFrame` tabs `task`, `notes`, `artifacts`, `sessions`. The Notes tab is an inline div (`ade-notes-tab`: `AdeNotesEditor`, `ade-notes-error`, `AdeHandoverList`). The Artifacts tab is an inline div (`ade-artifacts-tab`: `AdeArtifactList`). Tab state is `useAdePanelTabsStore().taskTab(id)`, default `'task'` (`web/state/adePanelTabs.ts`). `adeBoardUi.openSession`/`openSessions` call `setTaskTab(id, 'sessions')`.
- `web/panel/AdeTaskTab.vue` (post-P496, no handover row): `KeyValueList` with Status, Jira `AdeLinkRow` + `AdeTaskIssueLine`, host `AdeLinkRow` (view button), Estimate, branch request row, not-merging switch. Then error alert, `AdeWorkflowBlock`, then the Branches block (header with `ProjectPicker ade-add-repo` and `ade-add-branch`; rows `ade-task-branch` with tag chip, repo tag, integration chips, label, `AdeTaskBranchMenu`). The branch rows show no PR.
- `web/panel/AdeLinkRow.vue` is one input per link (dt/dd, copy, view). `web/panel/useLinkFields.ts` gives `saveJira`/`saveHost`/`clearJira`/`clearHost`; `saveJira` refuses non-Jira text (`space.ade.link.notJira.error`). `web/jira.ts` `parseJira`.
- `web/artifacts/AdeArtifactList.vue` (props `taskId`, `readonly`, `compact`) lists merged artifacts. It renders `AdeArtifactAdd.vue` (popover form, one input per field) and remove. Callers: `AdeTaskPanel`, `AdeArchivedPanel` (readonly), `stage/AdeStageDialog.vue` (compact readonly, `ade-stage-artifacts`). `AdeArtifactAdd` has no other caller.
- `web/panel/AdeArchivedPanel.vue` has its own local `tab` ref and the same four tabs, read-only.
- `web/panel/AdeBranchPanel.vue` has its own `details`/`sessions` tabs (`ade-panel-tab-details`, used by `ade-v2-needs.spec.ts:50`, `ade-v2-task-flow.spec.ts:104`). Only one panel renders at a time, so a task-panel `details` tab reuses the same testid without collision. It already reads `useChangeRequests().data.branches[id]` and `prState.ts` (`PR_TONE`, `prStateLabel`).
- `adewire.ChangeRequest` = `{number,title,state,url}`. `ade/board.go` `resolveRepoHost` builds it (line ~701) from `githost.ChangeRequest`, which already carries `BaseRef` and `ChecksState` (`""|SUCCESS|FAILURE|PENDING|ERROR|EXPECTED`). Other builders: `ade/archived.go:93`, `ade/hostlink.go:212`, `ade/artifacts.go:306`. Fakehost has `SetChecksState`.
- Backend artifact add: `bridge/adetask.go` `AddArtifact` validates kind and lengths; `ade/artifacts.go` `normalizeArtifact` already detects host kind and ref through `b.hostLink(url)`. There is no rename (`UpdateArtifact`) call.
- No agent writes task notes: `AdeTask.notes` is user-only (no MCP tool). "Agent notes" in the row means agent handovers.

### Session id finding: not stored

- `ade_task_handovers` (migration 0057) and `ade_task_artifacts` (0058) have no session column. `model.AdeHandover`, `model.AdeHandoff`, `model.AdeTaskArtifact`, `adewire.Handover`, `adewire.Artifact` carry none.
- Artifacts store `run_id`. `ade_runs.session_id` links a headless run to its session, but a TUI session (manual step, branch Start, review agent) has no run, so its handover drafts and reported artifacts cannot be traced.
- `mcp.Grant` has no `SessionID`; `mcp.SpaceCall` is `{TaskID, RunID, StepID}`; `HandoverRecorder.RecordHandover(ctx, taskID, stepID, notes, finished)`.
- Grant timing: `ade/launches.go` `prepareWithGrant` registers the grant, then `tr.Prepare(pa)`; `ade/tracker.go` `Prepare` mints `recordID = uuid.NewString()` when `args.Resume == ""`. So a fresh TUI session's id is unknown at grant time; a resume's is `pa.Resume`. Headless `ade/runs.go` creates `sessionID` before registering its grant (`superviseAgent`, ~line 852).

So P501 adds the backend record.

## Design

### Backend 1: origin session on handovers and artifacts

- Migration `int/storage/migrations/0066_p501_origin_sessions.sql`:
  - `ALTER TABLE ade_task_handovers ADD COLUMN session_id TEXT NOT NULL DEFAULT '';`
  - `ALTER TABLE ade_task_artifacts ADD COLUMN session_id TEXT NOT NULL DEFAULT '';`
  - Register `{Version: 66, Name: "p501_origin_sessions", File: "0066_p501_origin_sessions.sql"}` in `migrations/embed.go` `names`. `embed_test.go` counts match.
  - Old rows stay `''` (unknown). No backfill: `run_id` maps only headless artifacts, and guessing is worse than unknown.
- `mcp.Grant`: add `SessionID string` (the Kira session record id; "" outside ADE). `mcp.SpaceCall`: add `SessionID`; `spaceTask` copies `g.SessionID`.
- `mcp.HandoverRecorder.RecordHandover(ctx, taskID, stepID, sessionID, notes string, finished bool)`; tool handler passes `g.SessionID`. Update every implementer and fake (find with codegraph_explore `HandoverRecorder RecordHandover`).
- Session id at grant time:
  - `ade/tracker.go` `PrepareArgs`: add `RecordID string`. `Prepare` uses it when `Resume == ""` and `RecordID != ""`, else mints as today.
  - `ade/launches.go` `prepareWithGrant`: before building the grant, `if pa.Resume == "" && pa.RecordID == "" { pa.RecordID = uuid.NewString() }`; grant `SessionID: cmpNonEmpty(pa.Resume, pa.RecordID)`.
  - `ade/runs.go` headless grant: `SessionID: l.sessionID`.
  - `internal/scriptruns/smart_run.go:139` grant stays without one.
- Handovers (`int/storage/repos/adehandover.go`, `int/ade/handover.go`):
  - `model.AdeHandover.SessionID`, `model.AdeHandoff.SessionID`. Add `session_id` to `handoverCols`, `scanHandover`, inserts.
  - `UpsertHandoverDraft`: insert and `ON CONFLICT ... DO UPDATE` set `session_id = excluded.session_id` (latest writer owns the draft).
  - `applyHandoff`: a promoted draft keeps its `session_id`; an inserted `run` row takes `AdeHandoff.SessionID`; a `user` row stays `''`.
  - `recordRunHandoverLocked(run)` sets `SessionID: run.SessionID`. `RecordHandover` sets it from the param.
  - `toWireHandover` copies it.
- Artifacts (`int/storage/repos/adeartifact.go`, `int/ade/artifacts.go`):
  - `model.AdeTaskArtifact.SessionID`; `artifactCols`, `scanArtifact`, `UpsertArtifact` insert. On conflict: `session_id = CASE WHEN excluded.session_id <> '' THEN excluded.session_id ELSE ade_task_artifacts.session_id END` (a user re-add never wipes the agent origin).
  - `storeArtifacts(taskID, source, stepID, runID, sessionID, in)`: `ReportArtifacts` passes `call.SessionID`, `AddArtifact` passes `''`.
  - `wireStoredArtifact` copies it; detected rows leave it `''`.
- Wire (Go `int/bridge/adewire/wire.go`, TS `web/wire.ts`): `Handover.sessionId`, `Artifact.sessionId` (string, `''` = user, detected or unknown). Keep both wire sides in sync; the adewire round-trip test catches drift.

### Backend 2: PR base and checks on the branch

- `adewire.ChangeRequest`: add `BaseRef string \`json:"baseRef"\`` and `ChecksState string \`json:"checksState"\``. TS `ChangeRequest` mirrors.
- `resolveRepoHost` fills both from `githost.ChangeRequest`. `archived.go`, `hostlink.go`, `artifacts.go` builders set them when the source has them, else `""` (find each with codegraph_explore `ChangeRequest` blast radius; no default left implicit).
- UI fixture `apps/kira-space/tests/fixtures/ade-v2/change-requests.json` (hand-written): add `baseRef` and `checksState` to every entry.

### Backend 3: rename and paste

- `AdeTaskService.UpdateArtifact(UpdateArtifactArgs{taskId,id,title})`: stored rows only (same check as `RemoveArtifact`), title trimmed, 1..200 chars, task writable. Repo `UpdateArtifactTitle(taskID, id, title, now)`. Board push invalidates the artifacts query, same as Add/Remove. Register in the service binding and TS bridge (follow `RemoveArtifact` end to end).
- `AddArtifact` allows an empty title when `url` is set: `normalizeArtifact` fills it with the host link ref (`api#7`, `api!7`) when `b.hostLink(url)` matches, else the URL host plus path (trimmed to 200). Kind then comes from detection (`changeRequest`/`issue`), else the passed kind (`link` from paste). Bridge validation drops "title required" only for this case.

### Frontend: tabs

- `web/panel/AdeTaskPanel.vue` tabs: `main` (`space.ade.taskPanel.main.label` "Main"), `details` (`common.details.title`, exists), `sessions` (unchanged). Header (title input) unchanged.
- `web/state/adePanelTabs.ts`: default task tab `'main'`. Type the tab value as `'main' | 'details' | 'sessions'` so a stale `'task'`/`'notes'`/`'artifacts'` cannot compile.
- `web/panel/AdeArchivedPanel.vue`: same three tabs; Details renders `AdeDetailsTab readonly`.

### Frontend: Main tab

- `git mv web/panel/AdeTaskTab.vue web/panel/AdeMainTab.vue`. Testid `ade-main-tab`. Keeps Status, Estimate, branch request row, not-merging switch, error alert, `AdeWorkflowBlock`. Drops both `AdeLinkRow`s and `AdeTaskIssueLine` (they move to Details). Never shows a handover (P496).
- New `web/panel/AdeMainBranches.vue` (prop `card`): the whole Branches block moved verbatim out of the old tab (header, add repo, add branch, rows, `AdeTaskBranchMenu`, its own error emit). This is the P502 seam: P502 rewrites only this file plus its spec. `AdeMainTab` holds no branch logic.
- PR line per branch, inside `AdeMainBranches` under the row button, only when `useChangeRequests().data.branches[row.id]` exists: `ade-task-branch-pr` with
  - state chip (`PR_TONE`, `prStateLabel` from `prState.ts`),
  - title as a link (opens host view through `useOpenHostView` for a change request, as `AdeBranchPanel` does),
  - target `→ {baseRef}` (`ade-task-branch-pr-base`), hidden when `''`,
  - checks glyph (`ade-task-branch-pr-checks`, `data-state` = raw value): `SUCCESS` pass glyph green, `FAILURE`/`ERROR` error glyph red, `PENDING`/`EXPECTED` static watch glyph (no raw `animate-spin`; check-ui-primitives bans it), `''` nothing. Tooltip via `AdeTip`. Glyph names from `ICON` (`@theme/icons`); a table for check states joins `plan/glyphs.ts`.

### Frontend: Details tab

New `web/details/` folder.

- `AdeDetailsTab.vue` (props `card`, `readonly?`), testid `ade-details-tab`, sections in order:
  1. Notes: `AdeNotesEditor` + `ade-notes-error` (moved from the panel; save logic moves with it).
  2. Links: `AdeLinkGroups.vue`.
  3. Handovers: `AdeHandoverList` (unchanged component; P496 rule: handovers live here, never in Main).
  4. Timeline: `AdeTaskTimeline.vue`.
- `linkGroups.ts` (pure): `linkGroups(task, artifacts) => { main: LinkItem[]; related: LinkItem[] }`.
  - Main: task Jira (`task.jira`), task host link (`task.hostUrl`), detected change requests (artifact kind `changeRequest` with source `host`).
  - Related: every other artifact except kind `branch` (branches show in Main), minus URLs already in Main.
  - `LinkItem = { key, label, url, kind, origin: 'jira' | 'host' | 'stored' | 'detected', artifactId?, sessionId, status? }`. Label = artifact title, else ref; never a bare number (`#7` becomes `api#7` via ref, or the title).
- `AdeLinkGroups.vue` (props `card`, `readonly`): two groups "Main" and "Related" (`ade-details-main`, `ade-details-related`), each item `ade-details-link` (`data-origin`, `data-url`). An item is a link (`<a>` opens externally via the existing open-external helper; change requests open host view). Badge for status when set. Session icon (see below).
  - Inline edit, Notion-style: one item at a time turns into an `Input` (`ade-details-link-input`) on the edit icon or double-click; driven by `useInlineEdit` (`@workbench/behaviour/inlineEdit`). Enter or blur commits, Escape cancels. Empty commit removes.
    - `jira`: commit via `useLinkFields.saveJira` (text is key or URL); empty via `clearJira`.
    - `host`: `saveHost` / `clearHost`.
    - `stored`: edits the title via `UpdateArtifact`; empty calls `RemoveArtifact`.
    - `detected`: read-only (no edit icon).
    - Row error shows inline (`ade-details-link-error`).
  - Paste line (`ade-details-paste`): one `Input` with placeholder "Paste a link". Enter or a paste event submits; trims; one item per line (so multi-line paste adds several). Rule per value, in order:
    1. `parseJira` matches and `task.jira` is empty: `saveJira`.
    2. Host link (frontend has no parser; ask the backend): if `task.hostUrl` is empty, try `saveHost`; on the `backend.space.gitHost.notLink.error` refusal fall through.
    3. Else `AddArtifact {kind:'link', title:'', url}` (backend detects kind and ref). A Jira key already on the task with a different key adds an `issue` artifact with `title = key`, `url` from `parseJira`.
    - Non-URL, non-Jira text: inline error `space.ade.details.paste.notLink.error`.
  - `readonly`: no paste line, no edit icon, no remove.
- `AdeTaskTimeline.vue` (props `card`, `readonly`): shadcn `Collapsible`, closed by default, trigger `ade-details-timeline-toggle` with a count. Content `ade-details-timeline`, entries `ade-details-timeline-entry` (`data-kind`), oldest first. Open state per task in `useAdePanelTabsStore` (new `timelineOpen(id)`/`setTimelineOpen`); in-memory, same as tab state.
  - Pure `taskTimeline.ts` (not `board/timeline`, which exists): `taskTimeline(task, branches, artifacts, handovers) => Entry[]`. Kinds: `created` (task `createdAt`), `branch` (each `Branch.addedAt`), `artifact` (stored rows, `createdAt`), `handover` (each row, `createdAt`; draft labelled draft). Detected artifacts are excluded (no real time). Each entry: time, label, link if any, `sessionId`.
- Session link (`AdeOriginSession.vue`, prop `sessionId`): icon button `ade-origin-session` shown only when `sessionId` is non-empty and `useSessions()` knows it. Click: `ui.openSession({ taskId, branchId: '', sessionId })`, which switches to Sessions and reveals it (stopped ones included). Archived panel: emit up and switch its local tab to sessions. Used in `AdeLinkGroups`, `AdeHandoverList` (row header) and `AdeTaskTimeline`.
- `AdeArtifactList.vue`: keep for the stage dialog only. Drop the add popover and remove button (Details owns those), so props become `taskId`, `compact`. Delete `AdeArtifactAdd.vue` (knip flags it otherwise). If after the change only the stage dialog calls it, keep the name.

### Data

- `useTaskArtifacts(taskId)` and `useHandovers(taskId)` (`web/queries.ts`) already exist; Details and Timeline reuse them (TanStack Query, no new fetch). New mutation `useUpdateArtifact` beside `useRemoveArtifact`, same invalidation.
- No new Pinia store. Timeline open state joins `adePanelTabs` (same concern: panel view state).

## Library fit

- Tailwind utilities only, no scoped `<style>`.
- shadcn-vue: `Collapsible` (timeline), `Input` (paste, inline edit), `Badge` (status), `Button`, `Tooltip` via `AdeTip`. Groups are plain lists; `KeyValueList` stays for Main fields.
- VueUse: `onKeyStroke` not needed (`useInlineEdit` covers keys); paste uses the native `@paste` on `Input`.
- Pinia: `adePanelTabs` only. TanStack Query: existing queries plus `useUpdateArtifact`.
- Every new component `<script setup lang="ts">`.

## i18n

`packages/theme/src/i18n/en.json`. Implementer greps each new key path before adding (no collision; `space.ade.details` and `space.ade.taskPanel.main` absent at base).

- Add: `space.ade.taskPanel.main.label` "Main"; `space.ade.details.main.title` "Main"; `.related.title` "Related"; `.empty.body`; `.paste.placeholder` "Paste a link or Jira key"; `.paste.notLink.error`; `.edit.tooltip`; `.remove.tooltip`; `.timeline.title` "Timeline"; `.timeline.created.label`, `.branch.label`, `.artifact.label`, `.handover.label`, `.draft.label`; `.session.tooltip` "Open the session that added this"; `space.ade.mainTab.pr.base.label` "→ {base}"; `.pr.checks.success/failure/pending.tooltip`.
- Remove when unused (catalog `--unused` lint flags them): `space.ade.taskTab.issue.label`, `.pr.label`, `.jiraPlaceholder.placeholder`, `.linkPlaceholder.placeholder` only if no caller remains; `space.ade.artifacts.add.*` and `AdeArtifactAdd` form keys; the panel `artifacts` tab label only if unused elsewhere.
- Backend error keys: `backend.space.ade.artifactTitle.error` (reuse the existing title-length key if one exists).

## Tests

### New: Go flow test (backend half)

`int/flows/adeflow/taskpanel_test.go` `TestTaskPanelOrigins`, contract `ade-task-panel`:

1. Launch a manual-step TUI session (helpers as in `TestStageHandover`); call `handover` through `handoverCall`; assert the draft's `sessionId == launch.SessionID`.
2. Same session calls `report_artifacts` (`reportCall`); stored artifact `sessionId == launch.SessionID`.
3. Take over (resume) the session, call `handover` again; draft keeps the same id.
4. Headless run finishes a step: run handover `sessionId == run's session id`.
5. User `AddArtifact` with the same URL: `sessionId` stays the agent's. A different user URL: `''`.
6. `AddArtifact {kind:'link', title:'', url: forge MR link}`: title = ref, kind `changeRequest`.
7. `UpdateArtifact` renames; detected row id refused; empty title refused.
8. Fakehost `SetChecksState(n, "FAILURE")` and a base ref: `ChangeRequests` returns `baseRef`, `checksState`.

Contract keys: `handovers`, `artifacts`, `changeRequests` with `Mask("id","createdAt","updatedAt","taskId")`; `sessionId` stays visible but compared against the launch id inside Go (contract masks it, since ids are random).

### Updated Go tests and contracts

- `handover_test.go` (`hoMask` already masks `sessionId`), `artifacts_test.go` (add `"sessionId"` to `artMask`), ChangeRequest tests in `lifecycle_test.go` and `githostflow/host_test.go` if they compare whole structs.
- Fakes of `HandoverRecorder` (codegraph_explore `HandoverRecorder` for all) and `mcp_test.go` grants.
- Regenerate: `KIRA_CONTRACT=write go test -p 2 ./internal/flows/adeflow -run 'TestStageHandover|TestTaskArtifacts|TestTaskPanelOrigins'`, then rerun without write and `-count=2`. Also run `go test ./internal/agentmcp/... ./apps/kira-space/internal/ade/... ./apps/kira-space/internal/storage/...` (embed count, repos).

### New: UI spec (frontend half)

`apps/kira-space/tests/ui/ade-v2-task-panel.spec.ts`, reads `contract('ade-task-panel', ...)` for handovers and artifacts with session ids:

- Tabs are Main, Details, Sessions; Main is default; title input still in the header.
- Main shows workflow and branches; branch with a PR shows `ade-task-branch-pr` with base and a failure glyph (`data-state="FAILURE"`); a branch without PR shows none. Main never shows handover text.
- Details: Main group holds the Jira and host link and the detected PR; Related holds a stored link; every item is an `<a>` with href; no item text is only digits.
- Paste: Jira key on a task without Jira lands in Main (sends `UpdateTask`); a plain URL sends `AddArtifact` with empty title and shows in Related; garbage text shows the inline error.
- Inline edit: edit a stored title, Enter sends `UpdateArtifact`; Escape cancels with no call; empty removes (`RemoveArtifact`); detected item has no edit icon.
- Timeline: collapsed by default; toggle opens; entries oldest first with `created`, `branch`, `artifact`, `handover`.
- Session link: clicking `ade-origin-session` on a handover and on an artifact opens the Sessions tab with that session selected; an item with `sessionId ''` has no icon.
- Archived panel: Details read-only (no paste, no edit).

### Existing specs to update (testids moved or renamed)

- `support/adeV2.ts:374` `openTask` waits for `ade-task-tab`: wait for `ade-main-tab`. Line 136 change-requests fixture gains the new fields.
- `ade-v2-panel-tabs.spec.ts` 23-25, 32, 52: tab values and `ade-notes-tab` to `ade-details-tab`.
- `copy-coverage.spec.ts:92`: tab loop `['main','details','sessions']`.
- `ade-v2-panel.spec.ts`: 72, 98-99 (`ade-task-jira-*`), 106-108, 127-129 (`ade-task-host-*`), 579 (`ade-task-host-view`) move to Details inline edit/paste; 191 `ade-task-tab dt`; 198-208 notes via Details; 242, 255 `ade-panel-tab-task`; `ade-task-branch` 40, 232, 284, 298 unchanged testid (now inside `AdeMainBranches`).
- `ade-v2-handover.spec.ts` 153-156, 175: Notes tab becomes Details; P496 absence check now on `ade-main-tab`.
- `ade-v2-artifacts.spec.ts`: tab `artifacts` to `details`; add form (`ade-artifact-add*`) replaced by paste; remove via inline edit; stage dialog and archived tests keep `ade-artifacts`/`ade-artifact-row` where `AdeArtifactList` still renders.
- `ade-v2-issue-tracker.spec.ts:62`, `ade-v2-archived.spec.ts:104`, `ui-leftovers.spec.ts:32-33`, `git-host-provider.spec.ts:133,137` (`ade-task-host-link`), `ade-v2-task-branches.spec.ts:200`, `ade-v2-branch-request.spec.ts:177`, `ade-v2-task-flow.spec.ts:103`.
- Grep before finishing, must return nothing: `grep -rn "ade-task-tab\|ade-notes-tab\|ade-artifacts-tab\|ade-panel-tab-notes\|ade-panel-tab-artifacts\|ade-panel-tab-task\|ade-task-jira\|ade-task-host\|ade-artifact-add" apps/kira-space`.
- Also check `perf/heap-ram.spec.ts`, `ade-v2-add-link`, `ade-v2-add`, `ade-v2-shell`, `ade-v2-automations` for tab clicks.

Run only own and changed specs: `bun run build:test:space && npx playwright test --config=apps/kira-space/playwright.config.ts --project=ui <names>`. Full suite runs once after P502.

### Fast checks

`bun run typecheck:space-web`, `bun run typecheck:space-tests`, `bun run lint` (catalog `--unused`, knip, check-ui-primitives), `go vet` and `go build ./...`. Pre-commit runs them.

No unit test: `linkGroups` and `taskTimeline` are filters and a sort.

## Docs

`docs/ARCHITECTURE.md`:
- "Stage handover (P407)": `session_id` (migration 0066), who sets it (agent tool from `Grant.SessionID`, run row from `run.SessionID`, user `''`); frontend line: `AdeHandoverList` sits in the Details tab.
- "Task artifacts (P409)": `session_id`, upsert keeps a non-empty origin; `UpdateArtifact`; empty-title add; frontend now Details tab (Main/Related groups, paste, inline edit, timeline); `AdeArtifactList` stage dialog only.
- "Interactive sessions": `Grant.SessionID` and `PrepareArgs.RecordID` preset.
- ADE panel paragraph (wherever tabs are listed): Main/Details/Sessions; Main branches in `AdeMainBranches.vue`; PR line base and checks.
- "Tests": new flow and spec.

## Commits

1. `feat: P501 record origin session on handovers and artifacts` — migration 0066, embed, models, repos, `Grant.SessionID`, `SpaceCall.SessionID`, `PrepareArgs.RecordID`, recorder signature, fakes, wire Go and TS.
2. `feat: P501 PR base and checks on change requests` — adewire + builders + TS + fixture.
3. `feat: P501 artifact rename and empty-title paste` — `UpdateArtifact`, bridge, query, normalize.
4. `test: P501 task panel flow test and contracts` — `taskpanel_test.go`, masks, regenerated `ade-handover`, `ade-artifacts`, `ade-task-panel` JSON.
5. `refactor: P501 split Main tab and branches` — `AdeTaskTab` to `AdeMainTab`, `AdeMainBranches`, tab values, store default.
6. `feat: P501 Details tab` — `details/*`, panel and archived wiring, `AdeArtifactAdd` deleted, `AdeArtifactList` trimmed, i18n.
7. `feat: P501 PR line on Main branches`.
8. `feat: P501 session links` — `AdeOriginSession`, handover list hook-up.
9. `test: P501 task panel UI spec and moved testids`.
10. `docs: P501 architecture` and SPEC Result (row Done).

Each commit passes the hook; contracts regenerate in the commit that changes their shape (1 and 4 may fold if a contract breaks in 1).

## SPEC result

Mark P501 Done with: tabs, session-origin migration 0066 and wire field, PR base/checks, `UpdateArtifact`, paste rule, timeline, new flow test and spec, list of updated specs, any deviation.

## Split and order

- One sequential Sonnet implementer. Backend and frontend share wire types and the contract JSON the UI spec reads, so commit 1-4 must land before 6-9. No stream split.
- P502 runs strictly after P501 is committed. P502 owns `web/panel/AdeMainBranches.vue` and its spec; it should not need `AdeMainTab.vue` or `AdeTaskPanel.vue` beyond import lines.

## File ownership and overlap

P501 owns: `web/panel/AdeTaskPanel.vue`, `AdeTaskTab.vue` (renamed `AdeMainTab.vue`), new `AdeMainBranches.vue`, `AdeArchivedPanel.vue`, `web/details/*`, `web/artifacts/AdeArtifactList.vue`, `AdeArtifactAdd.vue` (deleted), `web/notes/AdeHandoverList.vue`, `web/state/adePanelTabs.ts`, `web/wire.ts`, `web/queries.ts` (artifact mutation), backend files listed above, `en.json` (keys listed), specs listed, `docs/ARCHITECTURE.md` sections listed, `SPEC.md` row P501.

Overlap: P502 edits `AdeMainBranches.vue` after this; no other phase in the chapter touches these files before P503 review.
