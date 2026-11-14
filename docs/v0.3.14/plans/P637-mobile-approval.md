# P637 Mobile approval of agent requests (J-ADE-072, F-B8)

Stream B of batch 2 (`batch-2.md`). Worktree `/home/user/kira-space-stream-b`, branch
`stream-b-v0.3.14-2`, base `bfc5895d7`.

## Ask

SPEC.md row: paired phone lists pending agent approvals and approves or denies; desktop state
follows. Reuse existing mobile pairing and approval model. Flow half: approval via mobile route
unblocks agent step, deny blocks it, expired device token refused. UI half: phone view of approval
list and desktop reflecting result. Covers mobile-approval part of J-ADE-072 (v0.3.13
`journeys-ade-3.md`: "approve 3 from desktop, 1 from mobile"; step 10 "answer branch request" from
mobile; oracle "approvals resolved per item"). F-B8 (v0.3.13 `stream-b.md`): "no mobile approval
route exists".

## What exists today

- Queue: `internal/approvals` (`Queue`, `Item`, `Owner`, `Service`). One queue per app
  (`appwire.go:397`, `w.Approvals`). Owners answer their own kinds: `ade.taskCreateOwner`
  (`task.create`, no options), `ade.branchOwner` (`branch.request`, options
  `{itemIds, remoteItemIds, shownItemIds}`), Studio owners for `db.*`, `kube.read`, `script.run`,
  `api.request`, `proof.review`. An answer that resolves calls `Queue.Remove`, which closes the
  routed prompt and pushes `kira:approvals:changed` (full list).
- Push path: `events.AttachApprovals` emits through the `appevent.Tap` (`appwire.go:364`), which
  already feeds `mobileweb.Hub.Publish`. Hub drops it: `eventChannels` (`mobileweb/events.go:20`)
  allowlists ADE read channels and agent session channels only.
- Phone server: `mobileweb.routes()` (`routes.go:72`) is the whole API. No approvals route.
  `requirePerm` gates `permWrite` and `permAgentInput` (global switch, then device flag).
  `withDevice` refuses expired (401 `E_EXPIRED`, cookie cleared), revoked, unknown tokens.
  `appMux` adds CSRF, idempotency, write rate, audit to every POST row.
- `mobileweb.Config` has no approvals dependency; `bridge.MobileAccessService` builds the Config
  (`bridge/mobile.go:154`).
- Phone app (`frontend/mobile/`): tabs Need You, Later, Plan. `NeedsScreen.vue` lists board-derived
  needs (questions, stuck runs) from `useAgentsModel`; no approvals. `api/events.ts` `CHANNELS` has no
  approvals channel. `main.ts` provides `adeReaderKey`, `agentControlKey`; not `approvalsControlKey`.
- Shared approval UI: `packages/workbench/src/approvals/` (`ApprovalItemCard`, `ApprovalItemActions`,
  `useApprovals`, `useApprovalActions`, `registry.ts`), Pinia `state/approvals.ts`. Both queries pass
  the module `queryClient` singleton from `@workbench/state/queryClient`, not the phone's own client.
- Space kind renderers: `frontend/src/ade/v2/request/kinds.ts` (`spaceApprovalKinds`). Not phone-safe
  as is: `TaskCreateBody.vue` loads `AdeNotesEditor`, whose chunk imports `bridge/control`
  (`@bindings`), and `BranchRequestBody.vue` imports `useBoard` from `../queries`, which imports
  `bridge/control`. The mobile Vite build has no `@bindings` alias, so either import fails
  `bun run build:mobile`.
- ARCHITECTURE.md "ADE" branch request section states "Desktop only ... Mobile has no approval
  surface."

## Design

### Which kinds a phone sees

Phone kinds: `task.create` and `branch.request` (the Space agent requests J-ADE-072 names). Studio
kinds stay desktop only: their payloads carry statements, result rows, request bodies, script
commands and folders, and a phone answer of a `db.response` sends data to an agent. Allowlist lives
in Go (`mobileweb/approvals.go`), checked on read, push and answer.

Projection (phone never sees absolute paths, ARCHITECTURE "drop cwd" rule):

- Keep only allowlisted kinds.
- `Origin.Cwd` and `Target.Location` set to `""`.
- Payload: decode to `any`, drop every `cwd` key (reuse `dropCwd`), replace every `worktree` string
  value with `filepath.Base` of it (branch.request `holder.worktree` is an absolute worktree path;
  `BranchRequestBody` shows it as the holder tag for kind `other`). Re-marshal.

### Go: phone server

New file `apps/kira-space/internal/mobileweb/approvals.go`:

```go
// Approvals is the app's approvals queue (internal/approvals.Queue).
type Approvals interface {
	List() []approvals.Item
	Answer(ids []string, approve bool, options map[string]json.RawMessage) []approvals.Result
}
```

- `phoneApprovalKinds` set; `phoneApprovals(items []approvals.Item) ([]approvals.Item, error)`
  projection above. Never returns nil (empty slice), so JSON is `[]`.
- `handleApprovals` (GET): 503 `codeUnavailable` with a new catalog key
  `backend.space.mobileweb.approvalsUnavailable.body` when `cfg.Approvals` is nil; else
  `writeJSON(200, phoneApprovals(cfg.Approvals.List()))`.
- `handleApprovalAnswer` (POST): body `{ids []string, approve bool, options map[string]json.RawMessage}`
  via `decodeBody`. `len(ids) == 0` gives `ipcerr.Required("ids")` through `writeServiceError`. Cap
  ids at 50 (`ipcerr.Contract`). For each id: answerable only when its kind prefix is allowlisted and
  the id is in `cfg.Approvals.List()` now; otherwise result `{id, code, error}` from
  `ipcerr.NotFoundNamed("approval")`, the same shape `Queue.answerOne` returns for a gone item.
  Answer the answerable ids in one `cfg.Approvals.Answer` call, options filtered to those ids; merge
  results back in request order. `auditAdd(r, "approval", id)` per id. Response 200 `[]Result`.
- Routes rows in `routes()`:
  - `read("/api/approvals", s.handleApprovals)`
  - `write("/api/approvals/answer", "approval.answer", permAgentInput, s.handleApprovalAnswer)`

  Permission is `permAgentInput`: answering an approval is a reply to an agent, the same class as
  `session.send`; a phone with "Changes" only cannot steer agents. Reads need a paired device only,
  like `/api/agent/sessions`.
- `Config` gains `Approvals Approvals` (optional; doc line in the struct comment block).
- `eventChannels` gains `appevent.ChannelApprovalsChanged: approvalsEvent`, where `approvalsEvent`
  type-asserts `[]approvals.Item` (error otherwise, which Publish logs and drops) and returns
  `phoneApprovals`.

### Go: wiring

- `bridge.MobileAccessService` gains `Approvals mobileweb.Approvals`; `NewMobileAccessService`
  passes it into `mobileweb.Config`.
- `appwire.go:600`: `Approvals: w.Approvals` (queue is built at line 397, before).

Desktop state follows with no new Go: an owner answer calls `Queue.Remove`, which closes the routed
prompt and pushes the list; the agent's call unblocks; task.create approval creates the child and
pushes the board.

### Frontend: shared kind defs, phone-safe

- `frontend/src/ade/v2/request/BranchRequestBody.vue`: import `useBoard` from `../readQueries`
  (`queries.ts` re-exports the same function; desktop behaviour unchanged).
- New `frontend/src/ade/v2/request/kindDefs.ts`: `branchRequestKind(): ApprovalKindDef` (current
  `branch.request` def, unchanged) and `taskCreateKind(content: Component): ApprovalKindDef` (current
  `task.create` def with `content` injected). No import of `TaskCreateBody`.
- `kinds.ts` (`spaceApprovalKinds`) builds from `kindDefs.ts` with `markRaw(TaskCreateBody)`. Output
  identical to today.

### Frontend: phone

- New `frontend/mobile/api/approvals.ts`: `httpApprovalsControl: ApprovalsControl`:
  - `approvalsList` → `getJson<ApprovalItem[]>('/api/approvals')`, parsed with the shared approvals
    schema if `@shared/domain/approvals` exports one (else typed cast, as `adeReader.ts` does).
  - `approvalsAnswer(args)` → `postJson<ApprovalResult[]>('/api/approvals/answer', args,
    { idempotencyKey: newUuid() })`: one key per click (one intent; `useApprovalActions` never
    retries).
  - `onApprovalsChanged(cb)` → `onChannel(CHANNEL.approvalsChanged, (d) => cb(d as ApprovalItem[]))`.
- `api/events.ts`: add `CHANNEL.approvalsChanged` to `CHANNELS`.
- New `frontend/mobile/approvals/TaskCreateBrief.vue`: brief as plain text
  (`whitespace-pre-wrap text-kira-md`, testid `ade-task-create-brief`, label from
  `space.ade.request.taskCreate.brief.label`). Plain text because the notes editor chunk is desktop
  only (bridge import); a phone needs to read the brief, not edit it.
- New `frontend/mobile/approvals/kinds.ts`: `mobileApprovalKinds()` →
  `{ 'branch.request': branchRequestKind(), 'task.create': taskCreateKind(markRaw(TaskCreateBrief)) }`.
- `main.ts`: `app.provide(approvalsControlKey, httpApprovalsControl)`.
- `App.vue`: `provideApprovalKinds(mobileApprovalKinds())` in setup (`provide` needs a component
  setup; `approvalKindsKey` is not exported, keep it that way).
- New `frontend/mobile/components/PhoneApprovals.vue` (`<script setup lang="ts">`, Tailwind only):
  - `useApprovals()` data; one `<li>` per item: `ApprovalItemCard` (shared) plus a touch footer:
    requester text (same rule as `ApprovalItemActions`), Deny and Approve `Button size="touch"`
    (`variant="dialog-deny"` / `"dialog-approve"`, `:pending` from `useApprovalsStore().answering`,
    labels from `useApprovalKind(item.kind)?.labels`), testids `approval-deny-<id>`,
    `approval-approve-<id>`, click `useApprovalActions().answer([id], bool)`.
  - Footer buttons only when `useAuthStore().permissions.agentInput`; else one `PermissionHint`
    with new key `mobile.approvals.answerOff.hint`.
  - Section heading `mobile.approvals.heading.title` with count; testid `phone-approvals`.
  - Watch item ids and `store.prune(ids)`, as `ApprovalsSurface` does.
  - Not reusing `ApprovalItemActions`: its buttons are `kira-lg`, below the phone's touch-target
    size; the touch footer reuses its store and actions, so no logic is copied.
- `NeedsScreen.vue`: render `PhoneApprovals` above the needs list. Empty state shows only when both
  `model.needs.empty` and approvals list is empty. Approvals render even while `boardQuery` is
  loading or failed (own query): place `PhoneApprovals` outside `QueryState`.
- `TabBar.vue`: needs badge counts `model.needs.badge + approvals.length`; badge shows when the sum
  is above 0.
- `AppShell.vue` resync: also invalidate `['approvals']` on the workbench `queryClient` singleton
  (`@workbench/state/queryClient`), since `useApprovals` lives there, not in the phone's client.
- Tooltip: `ApprovalItemCard` uses shadcn `Tooltip` with `:disabled` when no tooltip. Confirm it
  renders in the phone app (BranchRequestBody uses it too); if reka needs a `TooltipProvider`
  ancestor, wrap `PhoneApprovals` in the theme `TooltipProvider`.

Library rule check: shadcn-vue `Button`, `Badge`, `Tooltip`; TanStack Query through the shared
`useApprovals`/`useAnswerApprovals`; Pinia `useApprovalsStore` and `useAuthStore`; VueUse
`useEventSource` already behind `onChannel`. No hand-rolled primitive, no scoped style.

### Not in scope

- Studio kinds on phone (decision above).
- Showing on desktop which device answered. Audit line records device and ids.
- Push notifications to the phone while the app is closed (no service worker by design).

## Files

Go:

- `apps/kira-space/internal/mobileweb/approvals.go` (new)
- `apps/kira-space/internal/mobileweb/routes.go` (two rows)
- `apps/kira-space/internal/mobileweb/server.go` (`Config.Approvals`)
- `apps/kira-space/internal/mobileweb/events.go` (`eventChannels` entry)
- `apps/kira-space/internal/bridge/mobile.go` (field, Config pass-through)
- `apps/kira-space/internal/appwire/appwire.go` (one field in the `MobileAccessService` literal)
- `apps/kira-space/internal/flows/mobileflow/approvals_test.go` (new)
- `apps/kira-space/internal/flows/mobileflow/writes_test.go` (`TestPhonePermissionMatrix` agent row)
- `apps/kira-space/tests/contract/mobile-approvals.json` (new, written by the flow test)

Frontend:

- `apps/kira-space/frontend/src/ade/v2/request/kindDefs.ts` (new), `kinds.ts`,
  `BranchRequestBody.vue`
- `apps/kira-space/frontend/mobile/api/approvals.ts` (new), `api/events.ts`
- `apps/kira-space/frontend/mobile/approvals/kinds.ts`, `approvals/TaskCreateBrief.vue` (new)
- `apps/kira-space/frontend/mobile/components/PhoneApprovals.vue` (new)
- `apps/kira-space/frontend/mobile/main.ts`, `App.vue`, `AppShell.vue`, `TabBar.vue`,
  `screens/NeedsScreen.vue`

Tests (UI):

- `apps/kira-space/tests/mobile/approvals.spec.ts` (new)
- `apps/kira-space/tests/mobile/support/mockServer.ts` (approvals routes and state)
- `apps/kira-space/tests/ui/ade-mobile-approval.spec.ts` (new, desktop half)

Shared (see `batch-2.md`):

- `packages/theme/src/i18n/en.json`: `mobile.approvals.heading.title`,
  `mobile.approvals.answerOff.hint`, `backend.space.mobileweb.approvalsUnavailable.body` (plus any
  other catalog the repo keeps in sync, e.g. other locale files if present; `check-catalog.ts` says)
- `docs/ARCHITECTURE.md`
- `docs/v0.3.14/SPEC.md` (landing only)

## Implementer steps

1. Go phone server: `approvals.go`, `Config.Approvals`, routes rows, `eventChannels` entry, catalog
   key. `go build ./...` and `go test ./apps/kira-space/internal/mobileweb/...`
   (`TestRoutes_*` walk the table; a nil `Approvals` answers 503, which counts as reached). Commit
   `feat: phone approvals routes and push`.
2. Wiring: `bridge/mobile.go`, `appwire.go`. Commit with step 1 or as `feat: wire approvals queue to
   phone server`.
3. Flow test `mobileflow/approvals_test.go` (below) plus the matrix row; write contracts with
   `KIRA_CONTRACT=write`, rerun without. Commit `test: phone approvals flow half`.
4. Frontend kind split (`kindDefs.ts`, `kinds.ts`, `BranchRequestBody.vue` import). `bun run
   typecheck:web`. Commit `refactor: phone-safe approval kind defs`.
5. Phone app files. `bun run build:mobile`, `bun run typecheck:mobile`. Commit `feat: approve agent
   requests from the phone`.
6. Mock server, `tests/mobile/approvals.spec.ts`, `tests/ui/ade-mobile-approval.spec.ts`. Run the
   targeted specs only (commands below). Commit `test: phone approvals UI half`.
7. ARCHITECTURE.md (below). Commit `docs: phone approvals facts`.
8. `bun run lint`, `bun run lint:go`, `bun run typecheck`. Fix every finding, pre-existing or not
   (working agreement). No `--no-verify` on a shipping commit.

### Flow test: `TestPhoneApprovals` (mobileflow)

Copy the orchestrator setup pattern from `adeflow/orchestrator_test.go` (`newORFixture`,
`createTaskCall`, `mcpResult`, `taskItems`) as small local helpers; packages differ, so they cannot
be shared. Repoless: orchestrator workflow `orch` (one agent step `plan`, `runs_on: once`), child
workflow `manual` (one manual step). Fake agent scenario key `tasks` (the repoless key adeflow uses)
with three `create_task` calls: "API login", "Denied one", "Expired one", then `done`.
`flowharness.WithBranchRequestWait` stays default (30 min task wait), so items stay open until
answered.

Steps and oracles:

1. `serve`, pair phone, `SetDevicePermissions(agentInput true)`, `SetAgentInputEnabled(true)`.
   Start orchestrator run. `waitTaskItem` for "API login".
2. Put two synthetic items on `app.W.Approvals`: `db.query:p637` (kind `db.query`, `Origin.Cwd`
   set to `app.Work`), and `branch.request:p637` (kind `branch.request`, payload with one `adopt`
   item whose `holder.worktree` is `filepath.Join(app.Work, "wt", "feat-x")`, `Origin.Cwd` set).
3. `GET /api/approvals`: 200; ids exactly {task.create item, `branch.request:p637`}; no `db.query`;
   body has no `cwd` key (`hasKey` from `reads_test.go`) and does not contain `app.Work`; branch item
   `holder.worktree == "feat-x"`. Contract `http:GET /api/approvals#pending` (mask `createdAt`,
   `expiresAt`, `parentTaskId`, as `orItemMask` does).
4. Open `p.events()` SSE. `POST /api/approvals/answer {ids:[taskItem], approve:true}`: 200, one
   result, empty error. Contract `http:POST /api/approvals/answer#approved`.
   - Agent unblocked: `mcpResult("create_task", 1)` outcome `created`, `taskId` set; board has the
     child with `ParentID == orch.ID`, title "API login".
   - Desktop follows: `ApprovalsSvc.List()` lacks the id; `PromptsSvc` list has no `approval` entry
     with that ref; an `app.Events` push `kira:approvals:changed` after the answer without it:
     `ContractEvent(... "mobile-approvals" ...)` for the desktop UI spec; contract
     `AdeTaskService.Board#afterPhoneApprove` (mask as `orBoardMask`).
   - Phone follows: SSE frame `kira:approvals:changed` whose list lacks the id and still holds no
     `db.query` item. Contract `sse:kira:approvals:changed#afterApprove`.
5. Wait for "Denied one". Turn the device agent flag off: answer gives 403
   `E_AGENT_INPUT_OFF/device`; item still listed on desktop; no `mcp-create_task-2.json`. Flag on.
6. `POST answer {ids:[deniedItem], approve:false}`: result ok. `mcpResult("create_task", 2)` outcome
   `denied`, no `taskId`; children count stays 1. Contract `http:POST /api/approvals/answer#denied`.
7. `POST answer {ids:["db.query:p637"], approve:true}`: 200, result code `E_NOT_FOUND` (whatever
   `ipcerr.NotFoundNamed` maps to); `db.query:p637` still in `ApprovalsSvc.List()`. Contract
   `http:POST /api/approvals/answer#notPhoneKind`.
8. Wait for "Expired one". `UPDATE mobile_devices SET expires_at = <past>` (as
   `TestPhoneExpiredDevice`). `GET /api/approvals` and `POST answer` both 401 `E_EXPIRED`; item still
   listed; no `mcp-create_task-3.json`. Contract `http:POST /api/approvals/answer#expired`.
9. Remove the synthetic items (`app.W.Approvals.Remove`) before the test ends.

`TestPhonePermissionMatrix` (`writes_test.go`): add
`{"/api/approvals/answer", map[string]any{"ids": []string{"task.create:none"}, "approve": true}}` to
the `agent` slice. It must refuse with the same codes as `send` and `take-over`.

### Phone UI spec: `tests/mobile/approvals.spec.ts`

Mock server gains `state.replies.approvals` (default `[]`), `GET /api/approvals`, and
`POST /api/approvals/answer` (records the request like other writes; reply from
`state.replies.approvalAnswer` or `[{id, code:'', error:''}]` per id; honours `auth` like other device
routes). Contracts read from `mobile-approvals`.

1. `contract:` pending list renders on Need You: card `approval-item-<id>` with operation, target
   "API login", brief text, workflow row; tab badge counts it; heading count.
2. `contract:` Approve sends `POST /api/approvals/answer` with `{ids:[id], approve:true}` and an
   `Idempotency-Key`; then `server.emit('kira:approvals:changed', <afterApprove frame>)` removes the
   card and shows the empty state.
3. Deny sends `approve:false`.
4. Agent input off (`permissions.agentInput false`): card shows, no Approve/Deny, permission hint
   shows.
5. Refused answer (reply with the `#notPhoneKind` result shape for the id): card stays, error tag
   `approval-error` shows the message.
6. `contract:` expired: `auth: 'expired'` and the `#expired` body: tapping Approve returns to the
   pairing screen with the expired notice (existing shared 401 handler).
7. Branch request card from the `#pending` contract: holder tag shows `feat-x`, no `/` path; Approve
   sends options with `itemIds` and `shownItemIds`.
8. Reconnect resync refetches `/api/approvals` (count GETs before and after a dropped stream, as the
   existing reconnect specs do).

Check `tests/mobile/copy-wording.spec.ts` and `consistency.spec.ts` still pass with the new screen
content (run the whole `mobile-ios` project once; it is small).

### Desktop UI spec: `tests/ui/ade-mobile-approval.spec.ts`

`contract:` desktop follows a phone answer. Seed approvals with the `#pending` task.create item
(`seedApprovals`, desktop shows `approvals-dialog`), seed board. Emit `event:kira:approvals:changed`
from the contract (`pushApprovals`) and the `#afterPhoneApprove` board push: dialog and toast both
gone, child task "API login" on the board under the orchestrator. Reuse helpers from
`tests/ui/support/approvals.ts` and `@workbench/testing/ui/approvals`; no new IPC channel.

### ARCHITECTURE.md edits

- "Mobile agents web": routes bullet lists `GET /api/approvals` and POST `/api/approvals/answer`
  (`permAgentInput`, audit `approval.answer`); SSE allowlist bullet adds `kira:approvals:changed`
  (projected); new bullet: phone approval kinds `task.create`, `branch.request` only, projection
  (no cwd, no location, worktree base name), non-phone id answers `NotFound`, phone UI on Need You
  (`PhoneApprovals`, shared `ApprovalItemCard`, plain-text brief).
- ADE branch request section: replace "Desktop only ... Mobile has no approval surface." with the
  phone path.
- "Approvals surface (P520)": one line that the phone binds `ApprovalsControl` over HTTP and SSE.
- Tests line: `TestPhoneApprovals` (contract `mobile-approvals.json`), `approvals.spec.ts` (mobile),
  `ade-mobile-approval.spec.ts`.

## Verification

From repo root unless noted:

- `go build ./...`
- `go test ./apps/kira-space/internal/mobileweb/... ./apps/kira-space/internal/bridge/...`
- `cd apps/kira-space && go test -p 1 ./internal/flows/mobileflow -run 'TestPhoneApprovals|TestPhonePermissionMatrix' -count=1`
- `cd apps/kira-space && KIRA_CONTRACT=write go test ./internal/flows/mobileflow -run TestPhoneApprovals`
  once, then rerun without it.
- `bun run build:mobile && bun run typecheck && bun run lint && bun run lint:go`
- `bun run test:ui:mobile` (whole phone project: small, and wording checks walk every screen)
- `bun run build:test && npx playwright test --config=apps/kira-space/playwright.config.ts --project=ui apps/kira-space/tests/ui/ade-mobile-approval.spec.ts apps/kira-space/tests/ui/ade-v2-orchestrator.spec.ts`
  (orchestrator spec covers the unchanged desktop task.create body after the kind split)
- Real check before accepting: `grep -n "approvals" apps/kira-space/internal/mobileweb/routes.go`
  shows both rows; `grep -n "httpApprovalsControl" apps/kira-space/frontend/mobile/main.ts`;
  `grep -n "PhoneApprovals" apps/kira-space/frontend/mobile/screens/NeedsScreen.vue`;
  `grep -n "ChannelApprovalsChanged" apps/kira-space/internal/mobileweb/events.go`.

## Done when

Every oracle above passes in a normal run, contracts written and re-verified, hooks green, ARCHITECTURE
updated, SPEC.md row and Result filled at landing (per `batch-2.md`).
