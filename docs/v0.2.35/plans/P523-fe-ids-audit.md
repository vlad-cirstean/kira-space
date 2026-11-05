# P523 audit: frontend id generation

Input: P523 plan §3.4 at `ff6539c77` (33 frontend sites). Tree: `chapter/v0.2.35` (`79021633d`). Question: which ids
should the frontend not mint at all. Each id traced from creation through the bridge to its Go handler
(`codegraph_explore`, then targeted reads).

## Verdict key

- **OK**: DOM key, in-memory handle, correlation tag, idempotency key, or user-edited draft the server validates.
  Keep client-side; may use the shared helper.
- **MOVE**: Go should mint. The frontend never reuses the value, so a client id buys nothing.
- **HARDEN**: client mint is needed (the id must exist before the call returns), but Go trusts it today. Go must
  check format and ownership.

## Shared facts behind the verdicts

- **Op ids (Studio).** `adapterhost.Host.RunOp` (`host.go:226`) mints a v4 when `OpSpec.OpID` is empty. The
  bridge wires refuse an empty one: `ReadRequestWire`, `CountRequestWire`, `MutateRequestWire`,
  `ExecuteRequestWire`, `ObjectDownloadRequestWire` (`adapterhost/wire.go:62,94,161,189,218`), `bridge/http.go:69`,
  `bridge/grpc.go:223`. Kibana `DataViews`/`DataView`/`Suggest` (`bridge/kibana.go`) and gRPC describe
  (`bridge/grpc.go:168`) already accept an empty one.
- **Op id is trusted.** No format or length check. `RunOp` refuses only an id still in `h.running`.
  `oplog.Wiring.handleOpStart` (`oplog/wire.go:196`) does a plain `INSERT` into `op_log`; a reused id from a
  finished op fails that insert (logged only). `handleOpEnd` then runs `UPDATE op_log ... WHERE id = ?`
  (`repos/ops.go:42`), which **overwrites the earlier op's history row** (status, command, error, rows).
- **Op id reaches engines and logs.** ClickHouse `query_id` = `kira-<opId>-<n>` (`clickhouse/adapter.go:107`), Mongo
  `comment` = opId (`mongo/adapter.go:411`), and every adapter's cancel path writes `cancel(<opId>)` into the app
  log. Values are bound, not spliced into SQL, so no injection. A newline in a crafted id can forge a log line.
- **Early cancel.** `CancelOp` remembers an unknown id for 5 s (`host.go:372`) so Stop can beat Run. This is the one
  real reason for client-minted op ids. It needs the id before `RunOp` is called.
- **Tabs.** Both apps' `TabsRepo.Save` upsert `ON CONFLICT(id) DO UPDATE SET ... window_key = excluded.window_key`
  (`apps/*/internal/storage/repos/tabs.go`). A save that names another window's tab id **moves that row into the
  calling window**, and the owner's next save prunes nothing back. `appstorage.ValidateTab` checks only non-empty.
  The tab id is also the terminal id (`useTerminalMount.ts:92`): it reaches `terminal.ValidateOpen` (non-empty
  only), `script_runs.terminal_id`, and `KIRA_TERMINAL_ID` in the agent env. The per-terminal settings file name
  is a SHA-256 of it (`statusline.go:109`), so no path injection.
- **Mobile.** `idempotent` (`mobileweb/idempotency.go:119`) requires `uuid.Parse` + length 36 and keys entries by
  `device|key`, so one device cannot replay or collide with another device's key. The LRU (1024 entries) is shared
  across devices: a paired device can evict others' entries and weaken their retry safety. No body binding: same key
  on the same route replays the first result. Plain-HTTP origin has no `crypto.randomUUID`; the hand-rolled v4 uses
  `getRandomValues` (CSPRNG). No `Math.random` id exists anywhere in frontend code.
- **Every persisted domain entity except tabs is already minted in Go** (connections, saved queries, API items,
  ADE tasks/branches/runs/sessions, scripts runs, mobile devices). The user's concern is mostly met today.

## Table

Columns: Persisted = stored by Go. Trusts = Go accepts the client value as the id.

| Site | Purpose | Persisted? | Trusts? | Security note | Verdict |
|---|---|---|---|---|---|
| Studio `views/shared/viewOp.ts:110` | op id, paged read; Stop cancels it | `op_log.id` | yes, no format check | reuse overwrites old `op_log` row; log-line forgery | HARDEN |
| Studio `views/shared/viewOp.ts:243` | op id, count; also client supersede guard `countOpId` | `op_log.id` | yes | never cancelled; guard can be a local counter | MOVE |
| Studio `views/shared/immediateMutation.ts:30` | op id, single mutation | `op_log.id` | yes | never retained | MOVE |
| Studio `views/shared/keyvalue/mutations.ts:67` | op id, key-value mutation | `op_log.id` | yes | never retained | MOVE |
| Studio `views/httprequest/state.ts:169` | op id, HTTP send; Stop | `op_log.id` unless incognito | yes | as viewOp:110 | HARDEN |
| Studio `views/grpcrequest/state.ts:204` | op id, gRPC call; Stop | `op_log.id` unless incognito | yes | as viewOp:110 | HARDEN |
| Studio `views/grpcrequest/schemaQuery.ts:107` | cancel handle, describe (`Host.Cancellable`, no op row) | no | yes | in-memory only | HARDEN (format only) |
| Studio `views/grid/fkPreview.ts:50` | op id, FK popover read; cancelled on close | `op_log.id` | yes | as viewOp:110 | HARDEN |
| Studio `views/grid/pendingChanges.ts:461` | op id, commit | `op_log.id` | yes | commit has no Stop (`generate.ts:315`) | MOVE |
| Studio `views/grid/fakeData/generate.ts:335` | op id per batch; Stop | `op_log.id` | yes | as viewOp:110 | HARDEN |
| Studio `views/console/state.ts:263` | op id, auto-explain; Stop | `op_log.id` | yes | as viewOp:110 | HARDEN |
| Studio `views/console/state.ts:473` | op id, run statement; Stop | `op_log.id` | yes | as viewOp:110 | HARDEN |
| Studio `views/console/state.ts:602` | op id, run all; Stop | `op_log.id` | yes | as viewOp:110 | HARDEN |
| Studio `console/kibana/useDiscover.ts:59` | op id, data view fetch | `op_log.id` | yes; empty already OK | never retained | MOVE |
| Studio `console/kibana/useDiscover.ts:116` | op id, histogram; abort cancels | `op_log.id` | yes | as viewOp:110 | HARDEN |
| Studio `console/kibana/useDiscover.ts:244` | op id, value suggest | `op_log.id` | yes; empty already OK | never retained | MOVE |
| Studio `console/kibana/kqlCompletion.ts:207` | op id, KQL suggest; cancelled | `op_log.id` | yes | as viewOp:110 | HARDEN |
| Studio `console/kibana/DataViewPicker.vue:23` | op id, data view list | `op_log.id` | yes; empty already OK | never retained | MOVE |
| Studio `state/objectStore.ts:71,89,108` (3) | op id, object download/upload/delete | `op_log.id` | yes | never retained | MOVE (3) |
| Studio `views/grid/pendingChanges.ts:402` | pending insert row key | no | not sent (commit sends `values`) | none | OK |
| Studio `views/stream/streamFilterHistory.ts:76` | in-memory history entry | no | not sent | none | OK |
| Studio `kubernetes/tabs.ts:53` | kube tab, unpersisted store | no | not sent | none | OK |
| Studio `kubernetes/components/LogsView.vue:95` | kube log stream handle | no | yes | Go refuses a live dup (`kubernetes/logs.go:237`); events go to the opening window | OK |
| Space `state/tabs.ts:70` | pinned repo-graph tab (plan says "terminal": wrong) | `tabs.id` | yes | cross-window row move | HARDEN |
| Space mobile `state/useAdeWrites.ts:12` | `Idempotency-Key` | no (10 min LRU) | yes, validated, device-scoped | shared LRU eviction across devices | OK |
| Space `ade/v2/board/workflowForm.ts:48` | step id draft in workflow YAML | YAML, `*.step_id` columns | user-edited; Go validates charset and per-workflow uniqueness (`adeflow/parse.go:317,320`) | none | OK |
| `docker-ui/components/LogsView.vue:132` | docker log stream handle | no | yes | Go refuses a live dup (`docker/logs.go:213`) | OK |
| `docker-ui/state/dockerExec.ts:24` | docker exec terminal handle | no | yes | dup refused (`docker/exec.go:158`); `execWrite`/`execClose` not window-scoped, id is the capability | OK |
| `workbench/state/createTabsStore.ts:388` | tab id, also terminal id | `tabs.id`, `script_runs.terminal_id` | yes | cross-window row move; unbounded terminal id | HARDEN |
| `workbench/state/createTabsStore.ts:419` | duplicated tab id | `tabs.id` | yes | as :388 | HARDEN |
| `workbench/memory/distill/distillStore.ts:38` | progress correlation tag | no | yes, 1..64 chars (`memorydistill.go:136`) | broadcast to all windows; harmless | OK |

Outside §3.4, checked for completeness:

| Site | Purpose | Verdict |
|---|---|---|
| Space mobile `state/auth.ts:26` | 4-digit pairing match code, phone-chosen, sent in the long-poll `POST /api/pair` | MOVE, own follow-up phase (protocol change) |
| Studio `celleditor/generate.ts:41,46,69` | uuid/ulid/token the user inserts into their own column | OK (user data) |
| Studio `workbench/GenerateDataDialog.vue:155` | `Math.random` PRNG seed for fake data, user-editable | OK (not an id) |
| `workbench/src/testing/e2eReal.ts:146` | test tooling | OK |

Pairing code: a phone-chosen code lets anyone who learns the victim's code (shoulder, or reading the plain-HTTP
body) submit a second request with the same code and a forged label. A server-minted code per request gives a
concurrent attacker a different code. Fix needs a two-step pair flow (`POST /api/pair` returns `{requestId, code}`,
then wait). Not P523 scope; name it as a follow-up phase.

## Summary

33 §3.4 sites: **9 OK, 10 MOVE, 14 HARDEN**. Extra: pairing code MOVE (follow-up phase).

Top risks:

1. Op id reuse overwrites a finished op's `op_log` row (Append fails silently, Finish updates by id).
2. Tab save moves another window's tab row (`ON CONFLICT(id) ... window_key = excluded.window_key`).
3. Op ids and terminal ids have no format or length check; they reach engine metadata (ClickHouse `query_id`, Mongo
   `comment`), the app log, `script_runs.terminal_id` and the agent env.
4. Mobile idempotency LRU is shared across devices (eviction only; replay stays device-scoped).
5. Pairing code is client-chosen.

No site has a predictable id (no `Math.random` id; mobile uses `getRandomValues`). No id reaches a file path
unhashed, YAML (except the validated step id) or SQL text.

## Recommended changes to the P523 plan

1. **MOVE sites drop the frontend id entirely, no helper.** Ten sites: `viewOp.ts:243`, `immediateMutation.ts:30`,
   `keyvalue/mutations.ts:67`, `pendingChanges.ts:461`, `useDiscover.ts:59,244`, `DataViewPicker.vue:23`,
   `objectStore.ts:71,89,108`. Go change: make `opId` optional in `ReadRequestWire`/`CountRequestWire`/
   `MutateRequestWire`/`ObjectDownloadRequestWire` `Validate` (`RunOp` already mints; it becomes `ids.NewID()`
   under P523). Kibana calls pass `''` today with no Go change. `runPagedCount` keeps its supersede guard with a
   local counter. Drop these from the §3.4 inventory; they become Go `ids.NewID()` sites (one, in `RunOp`).
2. **HARDEN op ids in Go, same phase.** In `RunOp` (and `Host.Cancellable`, `CancelOp`): reject a non-empty `OpID`
   that is not `ids.IsUUID`. In `oplog.Wiring.handleOpStart`: on `Append` failure, drop the op from `inFlight` and
   mark it so `handleOpEnd` skips `Finish`; or make `Append` `INSERT ... ON CONFLICT DO NOTHING` and check rows
   affected. This closes the overwrite. Keep client mint for the 11 cancellable sites (early-cancel needs it).
3. **HARDEN tabs in Go.** Change both `TabsRepo.Save` upserts to `ON CONFLICT(id) DO UPDATE ... WHERE
   tabs.window_key = excluded.window_key` and fail the save (or skip the row) when the id belongs to another window.
   Add a length bound (64) and charset check to `appstorage.ValidateTab` and `terminal.ValidateOpen` (accept legacy
   ids: v4, v7, and the `docker-exec:`/`kube-*:` prefixed forms). Require UUID shape for tab ids only after the
   implementer confirms no stored or older-client tab id is non-UUID; otherwise charset plus length.
4. **Shared helper placement.** `packages/shared/ids.ts` (`newUuid`, `isUuid`) as planned is right: both apps and
   `packages/workbench`, `packages/docker-ui` import it, and mobile needs the `getRandomValues` path. It ends up with
   23 callers (11 HARDEN op sites, 3 tab sites, 9 OK sites), not 33. Add a lint rule that bans bare
   `crypto.randomUUID` in frontend code, so new sites go through the helper and a reviewer sees each one.
5. **Fix §3.4 errors.** Space `state/tabs.ts:70` is the pinned repo-graph tab, not a terminal. The terminal id is
   the tab id from `createTabsStore.ts:388`. The 21-site "op" row mixes cancellable and fire-and-forget ids; split it
   per item 1.
6. **Workflow step id.** Keep client mint (user-edited YAML, Go validates). `stp_` nanoid passes the charset rule.
7. **Pairing code** and the shared idempotency LRU: add as follow-up `SPEC.md` phases, not P523 scope.
