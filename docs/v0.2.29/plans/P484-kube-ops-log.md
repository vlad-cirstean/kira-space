# P484 Kubernetes: ops panel tracks every action

Plan for SPEC row P484. Planned against `2418fa02` (chapter opened). One sequential implementer.
Discovery used `codegraph_explore` over the op log (`adapterhost.Host.RunOp`, `oplog.Wiring`,
`OpLogPanel`), the Kubernetes module (`kubernetes.Service`, `BoundService`, `logs.go`) and the P440
MCP tools (`dbmcp.kubeRun`).

## 1. Goal and invariants

- Every Kubernetes call that reaches a cluster records one row in Kira Studio's Operations panel.
  UI calls and MCP calls go through one seam inside `kubernetes.Service`.
- The seam survives P487 (kubectl swap): P487 rewrites what runs inside a recorded call, never the
  recording.
- Row shape is Studio's existing `OpRecord`, kind `kubernetes`. No migration, no new op kind, no new
  wire field on `OpRecord`.
- Module stays read-only (`kubernetes/doc.go`, ARCHITECTURE "Kubernetes module"). P484 records
  actions; it adds none.
- P440 MCP rows stay byte-identical (`tests/contract/dbmcp-kube.json` `OpsService.Recent`).

## 2. Action inventory

Spec names apply, delete, scale, restart, exec, port-forward, logs. Tree today (`grep` of exported
`Service`/`BoundService` methods, confirmed by `codegraph_explore`):

| Action | Exists | Kind of call | P484 |
|---|---|---|---|
| apply | no | — | nothing to record; not added |
| delete | no | — | same |
| scale | no | — | same |
| restart (rollout) | no | — | same |
| exec | no | — | same |
| port-forward | no | — | same |
| `LogsOpen` (UI, follow or not) | yes | long-running stream | record, row spans stream |
| `ReadLogs` (MCP, one-shot) | yes | read | record |
| `Status` (probe, refresh) | yes | read, may run exec SSO plugin | record |
| `Namespaces` | yes | read | record |
| `Kinds` (discovery) | yes | read | record |
| `List` / `ListCapped` | yes | read, polled every 10 s by UI | record unless background |
| `Describe` (object + events) | yes | read | record |
| `SetMcp` | yes | local DB write, no cluster | not recorded (rule below) |
| `Clusters` | yes | kubeconfig parse, no cluster | not recorded (rule below) |
| `LogsClose` | yes | ends a stream | no own row; finishes the stream's row |

The six missing actions are out of this phase: doc.go forbids them and no SPEC row in v0.2.29 adds
them. The seam (3) records any future action with one call. Orchestrator: P487's row also lists
apply/delete/scale/restart for its inventory; same finding applies there.

Rules:

- Recorded: every call that reaches the API server (or, after P487, spawns `kubectl` against a
  cluster).
- Not recorded: app-setting writes (`SetMcp`; P485's cluster order). Same as DB connection edits,
  which no module records. Kubeconfig-only reads (`Clusters`).
- Not recorded: background refetches (`background: true`, 4.4). Studio's op log has no background
  lane (Space's has); a 10 s poll per open list would write 360 rows an hour. The first load of a
  key and every user-driven call still record.
- Not recorded: failures before the op starts (missing field, unknown cluster id, duplicate stream
  id). They return as errors, as `adapterhost` pre-op failures do.

## 3. Op log shape

`model.OpRecord` / `packages/shared/domain/ops.ts` `opRecordSchema`, unchanged:

| Field | Kubernetes row |
|---|---|
| `id` | minted by `Host.RunOp` |
| `connectionId` | `null` (connectionless, like `http`, `grpc`) |
| `tabId` | `null` (module has no tabs) |
| `kind` | `kubernetes` |
| `status` | `running`, then `ok`, `error` or `cancelled` |
| `startedAt`, `durationMs` | `Host.RunOp` |
| `rows` | count below, else `null` |
| `command` | kubectl command line (4.1) |
| `error` | error text; `Status` unavailable: its `message` |
| `path`, `commandTruncated` | `null` / `false` |
| `source` | `user`, or `mcp` from `adapterhost.WithOpSource` (dbmcp `withPanicRecovery`) |

Per action:

| Action | `command` | `rows` |
|---|---|---|
| `Status` | `kubectl version --context <ctx>` | `null` |
| `Namespaces` | `kubectl get namespaces --context <ctx>` | namespaces |
| `Kinds` | `kubectl api-resources --context <ctx>` | kinds |
| `List` | `kubectl get <res> (-A \| -n <ns>) [-l <sel>] --context <ctx>` | rows returned |
| `Describe` | `kubectl describe <res> <name> [-n <ns>] --context <ctx>` | `1` |
| `LogsOpen` | `kubectl logs <pod> -n <ns> [-c <c>] [--previous] --tail <n> [--timestamps] [-f] --context <ctx>` | lines delivered |
| `ReadLogs` | same, never `-f` | lines returned |

`Namespaces`, `Kinds`, `List`, `Describe`, `ReadLogs` strings equal today's MCP statements
(`dbmcp/kube_tools.go` `kubeListStatement`, `kubeDescribeStatement`, `kubeLogsStatement`, inline
namespaces/kinds strings). `--tail -1` for Tail all. `-f` sits right before `--context`, only when
`Follow`.

Status mapping: `Host.RunOp` rules (`err == nil` ok; derived ctx cancelled gives cancelled; else error).
`Status` returns trouble as a value (`unavailable`), so its recorded closure returns an error built
from `st.Message` while the bound method still returns `(st, nil)`.

## 4. Design

### 4.1 `internal/kubernetes/commands.go` (new): one source of command text

- Move from `dbmcp/kube_tools.go`: `shellWord`, `plainWord`, `kubectlResource`, and the three
  statement builders. Export:
  - `StatusCommand(ctx string) string`
  - `NamespacesCommand(ctx string) string`
  - `KindsCommand(ctx string) string`
  - `ListCommand(ctx string, a ListArgs) string`
  - `DescribeCommand(ctx string, a DescribeArgs) string`
  - `LogsCommand(ctx string, a LogsReadArgs, follow bool) string`
- `dbmcp` calls these for the approval statement (`kubeRun`'s `statement` func). Dialog text and op
  command then come from one function. dbmcp keeps its own arg types and maps them to
  `kubernetes.ListArgs` etc., as it already does for the read.
- Comment above the builders today says "kubectl never runs them". Keep that until P487.

### 4.2 `internal/kubernetes/record.go` (new): the seam

```go
// Recorder runs one cluster call under an Operations panel row.
type Recorder interface {
	Record(ctx context.Context, command string, fn func(context.Context, *OpNote) error) error
}

// OpNote carries what the call learned while running.
type OpNote struct{ /* mu, rows *int, command string */ }
func (n *OpNote) SetRows(k int)
func (n *OpNote) SetCommand(c string) // P487: the argv actually run
func (n *OpNote) Rows() *int
func (n *OpNote) Command() string

func WithRecorder(r Recorder) Option
```

- Default recorder (nil option): runs `fn` with a throwaway note. Unit tests and `logs_test.go`
  stay as they are.
- `Service.record(ctx, clusterID string, background bool, command func(ctxName string) string,
  fn func(context.Context, *OpNote) error) error`:
  1. `s.target(clusterID)` resolves the context name; its error returns unrecorded (pre-op).
  2. `background`: run `fn` with a throwaway note, no row.
  3. Else `s.rec.Record(ctx, command(name), fn)`.
- Every recorded closure uses the ctx it is handed. Panel Cancel (`OpsService.Cancel`, then
  `Host.CancelOp`) cancels that ctx, so a slow list or describe stops.
- Wrapped methods: `Status`, `Namespaces`, `Kinds`, internal `list` (covers `List` and
  `ListCapped`), `Describe`, `ReadLogs`, `LogsOpen` (4.3). Validation stays before `record`.
- `Status{Refresh: true}`: `drop` runs before `record`, so the probe row covers the reconnect.

### 4.3 Logs: row spans the stream (`logs.go`)

Today `LogsOpen` opens synchronously, then `go s.pumpLogs(...)`. New flow:

1. Validate; `s.logs.add` duplicate check (pre-op, unrecorded); resolve context name.
2. `opened := make(chan error, 1)`; `go s.rec.Record(context.Background(), LogsCommand(...),
   func(runCtx, note) error { ... })`.
3. Inside: `ctx, cancel := context.WithCancel(runCtx)`; store `cancel` in the registry entry;
   `openLogStream`; send its error on `opened`; on failure return it (row error).
4. On success pump (existing `pumpLogs`, now counting lines into `note.SetRows`). Return:
   - `runCtx.Err() != nil` (panel Cancel) returns `context.Canceled`; row cancelled.
   - mid-stream read error (today `final.err`) returns that error; row error.
   - else (EOF, `LogsClose`, `CloseWindow`, `closeCluster`, `Shutdown`) returns `nil`; row ok.
5. `LogsOpen` waits on `opened` and returns its error. Caller contract unchanged: open errors return
   synchronously; later failures arrive as the stream's final event.

- Panel Cancel ends the stream the same way `LogsClose` does: `pumpLogs` emits `Ended` (no error),
  `LogsView.vue` shows its ended state and Reconnect. No frontend change for it.
- Registry remove/cancel order stays as `pumpLogs`' defers have it.
- Quit: `Shutdown` cancels streams; `oplog.Wiring.finishInFlight` covers any row still running.

### 4.4 Background flag

- Add `Background bool \`json:"background,omitempty"\`` to `StatusArgs`, `ClusterArgs`, `ListArgs`,
  `DescribeArgs`. MCP never sets it. `LogsOpenArgs` gets none: each open is a user act.
- Frontend `kubernetes/queries.ts`: a query fn passes `background = qc.getQueryData(queryKey) !==
  undefined`. First load of a key records; interval refetch, focus refetch and invalidation
  refetch of loaded data do not. One helper in `queries.ts`, used by `statusOptions`,
  `kindsOptions`, `useClusterStatus`, `useKinds`, `useNamespaces`, `useResourceList`, `useDescribe`.
  `qc` from `useQueryClient()` in each hook (`useClusterProbes` included).
- `useRefreshCluster`'s `status(id, true)` passes `background: false`: Refresh is a user act. Its
  follow-up invalidation refetches are background.
- `kubernetes/control.ts`: `status`, `namespaces`, `kinds`, `list`, `describe` take a trailing
  `background: boolean`. `wire.ts` types follow. Regenerate bindings (`wails3 task
  common:generate:bindings`; gitignored).

### 4.5 Host adapter and wiring

- `internal/bridge/kubernetes.go`: `KubeRecorder{Host *adapterhost.Host}` implements
  `kubernetes.Recorder`:
  `Host.RunOp(ctx, OpSpec{Kind: "kubernetes"}, func(ctx, op) { op.SetCommand(command); note :=
  &kubernetes.OpNote{}; err := fn(ctx, note); if c := note.Command(); c != "" { op.SetCommand(c) };
  if r := note.Rows(); r != nil { op.SetRows(*r) }; return nil, err })`.
- `kubernetes` package never imports `adapterhost`/`adapters`. The seam is its own interface, so
  P487 and tests swap freely.
- `appwire/wire.go:164`: `kubernetes.NewBoundService(store, events, append([]kubernetes.Option{
  kubernetes.WithRecorder(bridge.KubeRecorder{Host: deps.Router.Host()})}, opts.KubernetesExtra...)...)`.
  Extras last so a flow seam can override.

### 4.6 dbmcp: drop its own row

- `kubeRun`: drop `s.cfg.Ops.RunOp`; call `read(ctx)` directly. `read` no longer returns a row
  count (Service sets rows). Approval statements come from 4.1.
- Remove `Config.Ops` and `OpRunner` (`server.go:118`, `kube_tools.go:30`); registration gate
  `server.go:268` becomes `s.cfg.Kube != nil`. `bridge/dbmcp.go` stops passing `Ops`.
- Row counts stay equal: kinds = `len(r.Kinds)` (MCP view maps 1:1), list = rows returned, describe
  = 1, logs = lines.

### 4.7 Operations panel (`frontend/src/workbench/panels/OperationsPanel.vue`)

- Already handles kind `kubernetes`: `codeLanguage` null, Re-run disabled (no connection), Cancel via
  `opLogMenuItems`, source cell, live elapsed clock for running rows (`OpLogPanel`). No change.
- No cluster facet: it needs a new `OpRecord` field and an `op_log` migration. The command already
  names `--context`, and the text filter matches it. Declined, not half-built.

## 5. Library rules

- No new UI surface, component or composable. Tailwind and shadcn-vue: nothing to style. VueUse:
  existing `useDocumentVisibility` stays.
- Pinia: the existing ops store (`createOpLogStore`) shows the rows; no new store.
- TanStack Query: the background decision lives in the query fns (4.4), reading the query cache;
  no hand-rolled refetch tracking.
- No new dependency.

## 6. Tests

One scenario, run on both sides of the IPC boundary.

### 6.1 Backend flow: `internal/flows/kubeflow/ops_test.go` (new) `TestOpsRecordEveryAction`

Boot as `helpers_test.go` `boot`. Read rows with `app.W.Ops.Recent`. Steps:

1. `Status kind-dev`: row ok, `kubectl version --context kind-dev`, rows null, source user,
   connectionId null.
2. `Kinds`, `Namespaces`: rows ok, commands as 3, rows = result lengths.
3. `List v1/pods default`: `kubectl get pods -n default --context kind-dev`, rows 3.
4. Same `List` with `Background: true`: fake request count grows, row count does not.
5. `Describe v1/pods default web-1`: `kubectl describe pods web-1 -n default --context kind-dev`,
   rows 1.
6. `LogsOpen` follow, stream `f1`, container `app`, tail 1000: a `running` row
   (`... --tail 1000 -f --context kind-dev`) while lines arrive. `Ops.Cancel(row.id)`: stream ends
   with no error (`waitEnded`), `fake.FollowEnded() == 1`, row `cancelled`, rows = lines delivered.
7. `LogsOpen` follow `f2`, then `LogsClose f2`: row ok.
8. `LogsOpen` with an open-time failure (unknown container, or `Previous` with no previous log;
   use whichever `KubeFake.log` already errors on): `LogsOpen` returns the error, row error.
9. `Status down`: returns `unavailable`, row error with the message.
10. Pre-op: `Status` with id `nope`: `E_NOT_FOUND`, no row. `SetMcp`: no row.
11. Contract `OpsService.Recent` under scenario `kubernetes` (`tests/contract/kubernetes.json`):
    the rows from steps 1 to 9, newest first, `Mask("id", "startedAt", "durationMs")`, URL
    replacements as `env.contract`. Also contract the running logs row from step 6 as
    `OpsService.Recent:running` (feeds the UI cancel test).

`dbmcpflow/kube_test.go` "ask" subtest: add one assertion that exactly one `kubernetes` row exists
after the approved list. Guards against a double row (Service plus MCP). Its contract stays as is.

### 6.2 UI spec: `tests/ui/kubernetes-ops.spec.ts` (new), same scenario

1. `contract: the Operations panel lists every Kubernetes action`: `opsRecent` answers the
   `kubernetes` `OpsService.Recent` contract. Open the panel (`toggle-operations-panel`). Row count
   equals the contract's; each `command` shows; status cells show ok / error / cancelled; source
   cell `User`; Re-run disabled on a kube row.
2. `contract: Cancel in the panel ends a running log stream`: open kind-dev pods, web-1, Logs tab (as
   `kubernetes-module.spec.ts` logs test). Emit `kira:op:update` with `OpsService.Recent:running`.
   Right-click the row, click Cancel: `OpsService.Cancel` called with that id. Emit the cancelled update
   plus `kira:kubernetes:logs` `{ended: true}`: `kube-logs-ended` and `kube-logs-reconnect` show.
3. `contract: a refetch is background; first load and a namespace switch are not`: open pods:
   first `List` call `background` false. Advance the poll with `page.clock` (precedent:
   `consistency.spec.ts`): next `List` call `background` true. Switch namespace: `background`
   false. First `Status` call `background` false.

Existing `kubernetes-module.spec.ts` and `kubernetes-mcp.spec.ts` must pass unchanged (args matched
with `toMatchObject`).

Fast checks per commit: Go build, vet, lint, typecheck. Run only `kubernetes`, `dbmcp`,
`flows/kubeflow`, `flows/dbmcpflow` Go tests and the three `kubernetes-*.spec.ts` UI specs. Full
suite runs once at chapter end (CLAUDE.md).

## 7. Docs

- `docs/ARCHITECTURE.md` "Kubernetes module": one bullet. Every cluster call records one op, kind
  `kubernetes`, through `kubernetes.Recorder` (`bridge.KubeRecorder` over `Host.RunOp`); command
  from `commands.go`; background refetches, setting writes, kubeconfig reads and pre-op failures
  are not recorded; a log stream's row runs until the stream ends; panel Cancel ends it.
- "Kubernetes tools (P440)": "Calls log as op kind `kubernetes`" now names the same seam; drop the
  `Config.Ops` mention ("register when `Config.Kube` and `Config.Ops` are set" becomes `Config.Kube`).
- Comments: `ops.ts` `'kubernetes'` and Go `opKinds` say "a Kubernetes read made through the DB MCP
  server" becomes "a Kubernetes cluster call, from the module or MCP".

## 8. Commits

1. `refactor: move Kubernetes command text into kubernetes package` (4.1; dbmcp uses it; no
   behaviour change; dbmcp flow passes).
2. `feat: record Kubernetes cluster calls in the op log` (4.2, 4.4 Go args, 4.5, 4.6).
3. `feat: Kubernetes log stream op spans the stream` (4.3).
4. `feat: Kubernetes refetches skip the op log` (4.4 frontend, bindings).
5. `test: Kubernetes ops flow and contract` (6.1).
6. `test: Kubernetes ops UI spec` (6.2).
7. `docs: Kubernetes op recording` (7).

## 9. Ordering consequences

- P484 runs before P487, so P484 records around client-go calls. P487 keeps every `s.record` call
  and `Recorder` untouched; it replaces only the closure bodies.
- After P487 `commands.go` is the argv source for both the spawn and the op command, so the panel
  shows what actually ran. If P487's argv differs (`-o json`, `--request-timeout`), P487 updates the
  builders and regenerates `kubernetes.json` `OpsService.Recent`. The UI spec reads the contract, so
  it follows with no edit. `dbmcp-kube.json` and the approval dialog change with it; P487's plan
  states the new strings.
- An action P487 runs as several spawns (describe: object plus events) stays one row. Its closure
  calls `note.SetCommand` with the argvs joined by newline.
- P487 logs: `kubectl logs -f` is a child process killed on ctx cancel. Row lifecycle (4.3) is
  unchanged: open result on a channel, row spans the process, panel Cancel kills it.
- P487 `Clusters` (`kubectl config view`) is a kubeconfig read: unrecorded under P484's rule unless
  P487's plan changes the rule and says why.
- Missing `kubectl` (P487): a failure to spawn inside a recorded closure is an error row. The plan
  for P487 decides whether that is pre-op instead.
- P485 cluster order is a setting write: not recorded. P486's redesign consumes `queries.ts`; it
  keeps the background helper, and each pod tab's log stream gets its own row.
