# P487 Kubernetes: drive the kubectl CLI instead of the native client

Plan for SPEC row P487. Planned against `41713b47` (P484, P485, P486 landed). Discovery used
`codegraph_explore` over `internal/kubernetes` (`client.go`, `env.go`, `config.go`, `errors.go`,
`discovery.go`, `list.go`, `describe.go`), `flowharness/kubefake.go`, `flowharness` dispatch and the
CLI resolvers (`toolexec.Locate`, `harness.ResolveCLI`). Files changed since the last index sync
(`service.go`, `commands.go`, `record.go`, `logs.go`, `mcpreads.go`) were read from disk. kubectl
behaviour was checked against kubectl source (`pkg/cmd/get/get.go`, `pkg/cmd/apiresources`).

No split into Parts. One sequential implementer, Stream A. The backend switch is atomic: no
intermediate state runs half on client-go. The UI half (missing-kubectl state) needs the contract
fixture the backend flow writes, so it is order-dependent too.

## 1. Goal and invariants

- Every cluster operation runs `kubectl` as a child process. `internal/kubernetes` imports no
  `k8s.io/client-go` package. `k8s.io/api`, `k8s.io/apimachinery` and `sigs.k8s.io/yaml` stay, for
  decoding and label-selector validation only.
- Read-only stays (`doc.go`). Only these subcommands ever run: `config view`, `version`,
  `api-resources`, `get`, `logs`. Nothing else is added.
- Kira code never reads kubeconfig credentials. It never opens a kubeconfig file (stat only, for the
  change fingerprint and the missing state). Contexts come from `kubectl config view -o jsonpath`
  selecting contexts, cluster servers and current context only. Never `--raw`, never `--flatten`,
  never the `users` section.
- Missing `kubectl`: a clear state with install steps (`brew install kubectl`, homebrew/core, no tap
  trust).
- Wire shapes stay. UI components change only for the new missing-kubectl state. `control.ts`,
  `queries.ts`, `tabs.ts`, `tabHost.ts` and every P486 component stay unchanged.

## 2. Inventory: native client today, kubectl after

The module has no exec, attach, port-forward, apply, delete, scale or restart path (P439 `doc.go`).
The spec row's list names them; none exist, none are added.

| Today (client-go) | File | kubectl argv (after `kubectl`) |
|---|---|---|
| `clientcmd` load of merged kubeconfig | `config.go` | `config view -o jsonpath=<T>` (T in 4.2); per file `--kubeconfig <f>` only when KUBECONFIG lists several existing files |
| `GET /version` | `service.go` `probe` | `version -o json --request-timeout=5s --context <ctx>` |
| `core.Namespaces().List` paged | `discovery.go` | `get namespaces -o json --request-timeout=15s --context <ctx>` |
| `disc.ServerPreferredResources` | `discovery.go` | `api-resources --verbs=list,get -o wide --request-timeout=15s --context <ctx>` |
| `GET` Table list, paged, `labelSelector` | `list.go` | `get <res> (-A \| -n <ns>) [-l <sel>] --request-timeout=30s --context <ctx>` (human output, parsed, 4.4) |
| `GET` one object as JSON | `describe.go` | `get <res> <name> [-n <ns>] -o json --request-timeout=15s --context <ctx>` |
| `core.Events().List` by uid | `describe.go` | `get events [-n <ns> \| -A] --field-selector involvedObject.uid=<uid> -o json --request-timeout=15s --context <ctx>` |
| `Pods().GetLogs().Stream` (UI) | `logs.go` | `logs <pod> -n <ns> [-c <c>] [--previous] --tail=<n> [--timestamps] [-f] [--limit-bytes=8388608] --context <ctx>` |
| `Pods().GetLogs().Stream` (MCP) | `mcpreads.go` | same, no `-f`, no `--limit-bytes` (cap applied by `readTail`) |

`<res>`: `kubectlResource(key)`. Core `v1/pods` gives `pods`. Grouped `apps/v1/deployments` gives
`deployments.v1.apps` (fully specified, so the version is exact). `--chunk-size` is left at kubectl's
default 500, which equals `defaultListPage`. `-A` on a cluster-scoped kind is harmless: kubectl prints
no NAMESPACE column for it.

`--request-timeout` bounds the API call. The Go ctx deadline (same value plus 5 s grace) kills the
process group (`procgroup.GracefulCancel`), so an exec plugin that hangs cannot outlive the call.

## 3. Decisions

### 3.1 Lists parse kubectl's table output

kubectl cannot print the server Table as JSON: `get.go` `Complete` forces `ServerPrint=false` for any
json output, and `--raw` sets no `as=Table` Accept header. Columns and cells must match kubectl, so
List runs the human output and parses it. kubectl collects all chunks into one tabwriter and flushes
once per resource mapping, so the header offsets hold for every row. Declined:

- `kubectl proxy --unix-socket`: per-op command text would be fake again (P484), plus a long-lived
  process per cluster.
- `-o json` plus client-side columns: re-implements every kind's printer and CRD printer columns.

Parser (`table.go`): split the header on runs of 2+ spaces to get column start offsets in runes
(tabwriter pads 3, header names hold single spaces: `NOMINATED NODE`). Slice each row by those
offsets; the last column takes the rest of the line; trim each cell. A leading `NAMESPACE` column
maps to `Row.Namespace` and stays a cell (today's shape). `NAME` maps to `Row.Name`. `Row.Key` is
`namespace/name` (the UI keys rows that way already). A line whose NAME slice is blank is a
continuation of a multi-line cell: drop it. `No resources found` goes to stderr with exit 0: zero
rows, columns empty.

Header names: kubectl prints `strings.ToUpper(column.Name)`. The UI matches `Ready`, `Status`,
`Age` and test ids use the names, so convert back: lower each word and capitalise its first letter,
splitting on space and `-`. Words in a fixed acronym set stay upper: `IP`, `IPS`, `CIDR`, `URL`,
`ID`, `UID`, `TLS`, `CPU`, `API`, `DNS`, `TCP`, `UDP`, `OS`. So `CLUSTER-IP` gives `Cluster-IP`,
`PORT(S)` gives `Port(s)`. `Column.Type` is `"string"` for every column (the UI never reads it).

Row cap: parse all rows, keep `max`, set `Truncated` when more came. Stdout cap 64 MiB; past it the
call fails with `ipcerr.Internal("kubectl output too large")`.

### 3.2 Kinds parse `api-resources -o wide`

`api-resources -o json` exists only in kubectl 1.34 and later (1.30, 1.32 and 1.33 accept only wide
and name). `-o wide` works in every supported kubectl. Same `table.go` parser. Columns used: `NAME`,
`APIVERSION` (split into group and version), `NAMESPACED`, `KIND`. `--verbs=list,get` replaces the
verbs filter. Subresources never appear. Partial discovery: kubectl prints the rows, then exits 1 with
`unable to retrieve the complete list of server APIs` on stderr; at least one row plus that text sets
`Partial`. No rows plus an error maps as any other error. Category, sort and `Logs` logic stay.

`findKind` goes away: List and Describe pass the resource to kubectl, which resolves it through its
own discovery cache. An unknown kind fails in kubectl (`the server doesn't have a resource type`),
which maps to NotFound. Kind keys and names are validated before any spawn (4.6).

### 3.3 Describe keeps building its own Description

`get -o json` feeds the existing unstructured pipeline: managedFields drop, Secret redaction,
owners, conditions, containers, YAML manifest. Events: the second `get events` call. Its namespace is
the object's own `metadata.namespace` (empty means cluster-scoped, so `-A`), which replaces
`kind.Namespaced`. Decode into `corev1.EventList`; sorting, the 200 cap and the event fields stay. An
events failure still leaves the list empty (`slog.Debug`). `kubectl describe` text stays declined:
the UI needs the structured shape.

### 3.4 Logs stream from kubectl's stdout

`kubectl logs` stdout is the stream; `lineFramer`, coalescer, registry and cancel paths stay. The
open result can no longer come from one HTTP response. `LogsOpen` returns:

- nil at the first stdout byte;
- the mapped error when kubectl exits before any stdout (unknown container, no previous log);
- nil after `logsOpenGrace` (3 s) with the process still running and silent (an empty follow
  stream). A later failure arrives as the stream's final event (`Ended` with `Error`), which the UI
  already shows.

Tail: `100|1000|5000` pass through; `-1` is `--tail=-1`, or `--tail=20000` with follow. Non-follow
UI reads add `--limit-bytes=8388608`. `ReadLogs` (MCP) runs without `-f`, pipes stdout through
`readTail(mcpLogBytes)`, then frames. Cancel (LogsClose, window close, panel Cancel, Shutdown) kills
the process group. `runLogStream` keeps its op row semantics.

### 3.5 Status

`version -o json`: `serverVersion.gitVersion` set means `StatusOK`. kubectl exits 1 with only
`clientVersion` when the server fails; stderr is classified (4.5). A version-skew warning with exit 0
is ignored. `Refresh` keeps dropping the cluster's log streams (`drop`); no client cache remains to
drop.

## 4. Backend design (`apps/kira-studio/internal/kubernetes`)

### 4.1 Runner (`kubectl.go`, new)

- `locate(env Env) (path string, probed []string, err error)`: `toolexec.Locate` with a lookPath over
  `env.Path` (today's `resolveCommand`, moved here), then candidates `/opt/homebrew/bin/kubectl`,
  `/usr/local/bin/kubectl` (Docker Desktop installs there). Resolved per call (stat only), so an
  install after launch works without restart. A miss returns `cliMissing(probed)`.
- `WithKubectlCandidates([]string)` option: flow-test seam; the harness sets none so a host kubectl
  is never reached.
- `run(ctx, args []string, opts) ([]byte, error)`: `exec.CommandContext`, `Setpgid`,
  `procgroup.GracefulCancel`, stdin `/dev/null` (no prompt; interactive exec plugins fail with
  `standard input is ...`, today's plugin reason). Env: `os.Environ()` with `PATH=env.Path`, and
  `KUBECONFIG=env.Kubeconfig` when set. Stdout bounded by `opts.max`; stderr bounded 16 KiB, kept
  whole (klog lines precede the real error, so `toolexec.ExecError`'s first line is not enough).
  Non-zero exit returns `*kubectlError{Exit, Stderr, Stdout}`. Stdout is returned alongside, for
  partial discovery and Status.
- `start(ctx, args) (stdout io.ReadCloser, wait func() error, err error)`: the streaming variant for
  logs.
- `toolexec.RunIO` is not reused: its 1 MiB stdout cap and first-line stderr do not fit lists,
  manifests or logs. Shared code stays unchanged.

### 4.2 Kubeconfig (`config.go`, rewritten)

- `kubeconfigPaths`, the stat-based `ConfigMissing` and `configFingerprint` stay.
- Load runs `kubectl config view -o jsonpath=<T>` with T:
  `{.current-context}{"\n"}{range .contexts[*]}C{"\t"}{.name}{"\t"}{.context.cluster}{"\t"}{.context.user}{"\t"}{.context.namespace}{"\n"}{end}{range .clusters[*]}S{"\t"}{.name}{"\t"}{.cluster.server}{"\n"}{end}`.
  `.context.user` is the auth-info name the wire already exposes as `Cluster.User`, not a
  credential. Missing keys print empty (kubectl default `--allow-missing-template-keys=true`). Lines
  with the wrong field count are skipped.
- `loadedConfig` drops `clientcmdapi.Config` and holds plain maps: `contexts map[string]kctx{cluster,
  user, namespace, source}`, `servers map[string]string`, `current string`.
- `Source`: one existing file means that path. Several mean one more call per file with
  `--kubeconfig <f>` and template `{range .contexts[*]}{.name}{"\n"}{end}`; the first file naming a
  context is its source (kubectl's first-wins merge).
- kubectl exits non-zero: `ConfigInvalid` with the last error line as `Message`.
- kubectl missing: new `ConfigCLIMissing = "cliMissing"`. `ConfigInfo` gains
  `Setup *CLISetup \`json:"setup,omitempty"\``, filled only then:
  `{DocsURL: "https://kubernetes.io/docs/tasks/tools/", Steps: [{install, "brew install kubectl"},
  {check, "kubectl version --client"}]}` plus `Probed []string`. Same shape as Space's
  `clisetup.Setup` (Studio cannot import Space internals; a local type, 2 fields).
- `target` returns the `cliMissing` error when the loaded state is `cliMissing`, so no op row starts
  (P484: failures before the call are not recorded).
- `Clusters` stays unrecorded: `config view` contacts no cluster.

### 4.3 Service (`service.go`)

Drop `clients`, `cache`, `client()`, `signature`, `quietKlog` and the klog import. `drop` keeps
`s.logs.closeCluster(id)`. `serverOf`, `clusterOf`, `target`, `Cluster` use the new maps.
`WithListLimits(page, max)` becomes `WithListMax(max)`: paging is kubectl's job now. Delete
`client.go`.

### 4.4 Commands (`commands.go`)

One source for what runs and what the op log and approval dialog show. Each builder returns argv:
`statusArgs(ctx)`, `namespacesArgs`, `kindsArgs`, `listArgs(ctx, ListArgs)`,
`describeArgs(ctx, DescribeArgs)`, `eventsArgs(ctx, ns, uid)`, `logsArgs(ctx, LogsReadArgs, follow)`.
The exported `StatusCommand`, `NamespacesCommand`, `KindsCommand`, `ListCommand`, `DescribeCommand`
and `LogsCommand` keep their signatures (dbmcp `kube_tools.go` calls them) and return
`"kubectl " + shellWords(argv)`. The binary path is shown as `kubectl`. Remove the "kubectl never
runs it" comment. Describe's op row shows the object `get`; its events `get` runs inside the same op
and is not shown (one row per user action, P484).

### 4.5 Errors (`errors.go`, rewritten)

`classify(err)` reads `*kubectlError`. Message: stderr with klog lines (`^[IWEF]\d{4} `) removed,
then the last line starting `error:`, `Error from server` or `Unable to connect`, else the trimmed
tail (2 KiB). Rules, first match wins:

- Go ctx deadline, `Client.Timeout`, `i/o timeout`, `context deadline exceeded`,
  `request did not complete within`: timeout.
- `getting credentials`, `exec plugin`, `credential plugin`, `standard input is`: plugin
  (interactive when `standard input is`).
- `You must be logged in to the server`, `(Unauthorized)`: unauthorized.
- `(Forbidden)`: forbidden.
- `context "` + `does not exist`, `invalid configuration`, `error loading config`, `auth provider`,
  `auth-provider`, `no configuration has been provided`: config.
- `Unable to connect to the server`, `was refused`, `no such host`, `connection refused`,
  `dial tcp`, `network is unreachable`: unreachable.
- `(NotFound)`, `the server doesn't have a resource type`, `not found`: NotFound.
- `(BadRequest)`, `previous terminated container`, `is not valid for pod`: BadRequest.

`mapErr` keeps its ipcerr keys and the `{"reason": ...}` details. New reason `ReasonCLIMissing =
"cliMissing"`: `ipcerr.T(CodeNotFound, "backend.studio.kubernetes.kubectlMissing.error", {probed})`
with details reason. `unavailable()` uses the same classifier. Namespaces' forbidden check uses
`classify == ReasonForbidden` in place of `apierrors.IsForbidden`.

### 4.6 Argument validation

Argv never passes through a shell, but a value starting with `-` would read as a flag. Before any
spawn: kind key segments match `^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$` (2 or 3 segments); namespace,
name, pod and container must not start with `-` and must be non-empty where required. Fail with
`ipcerr.BadRequest`. Label selectors keep `labels.Parse`.

### 4.7 env.go

Keep the login-shell probe: kubectl and its exec plugins (aws, gke-gcloud-auth-plugin, kubelogin)
must see the terminal's PATH and KUBECONFIG. `pinExecPlugins` goes: kubectl resolves the plugin
command with its own process PATH, which is the resolved one. `resolveCommand` moves into `locate`.
`WithEnv` stays.

### 4.8 Untouched

`bound.go`, `channels.go`, `store.go`, `record.go` (Recorder and OpNote unchanged), `types.go` except
`ConfigInfo` (4.2), `lineFramer`, `logRegistry`, `readTail`, `SetMcp`, `Reorder`. `doc.go` text:
"runs only read kubectl subcommands (config view, version, api-resources, get, logs)".

## 5. Credentials and guard G5

G5 gains `apps/kira-studio/internal/kubernetes` (`internal/conventiontest/conventions_test.go`, new
`inKube`). It fails on:

- an import of `k8s.io/client-go/...` or `net/http`;
- string literals containing `--raw`, `--flatten`, `.users`, `client-key`, `client-certificate`,
  `token`, `auth-provider`, `Authorization` or `Bearer `;
- calls `os.ReadFile`, `os.Open`, `os.OpenFile` (stat stays allowed).

`credentialCall` (os.Getenv) is not applied: `env.go` reads PATH and KUBECONFIG, a path list. The
real-claude test module is outside the guard. ARCHITECTURE G5 bullet (line about guard scope) and
"CLI credentials: never touched" get the kubectl line.

## 6. P484 and P486 consequences

- P484 (op log): `Recorder`, `OpNote`, `Service.record`, `bridge.KubeRecorder` stay as they are. The
  command text becomes the argv that really runs. The approval dialog (dbmcp prompt mode) shows the
  same text. `OpsService.Recent` fixtures in `kubernetes.json` and `dbmcp-kube.json` are rewritten.
  Background refetches stay unrecorded.
- P486 (UI): same wire types and IPC methods, so `control.ts`, `queries.ts`, `wire.ts` call shapes
  hold. Data differences: `Column.Type` always `string`, `Row.Key` is `namespace/name`, column names
  title-cased from kubectl headers (3.1). `ResourceTable` compact columns (`Ready`, `Status`, `Age`)
  still match. P486 landed first; nothing in it changes.
- Order: P487 runs after P486 (table order). No later phase in this chapter touches the module.

## 7. Frontend (missing kubectl only)

- `wire.ts`: `ConfigState` adds `'cliMissing'`; `KubeConfigInfo` adds `setup?: KubeCliSetup`,
  `probed?: string[]`; `UnavailableReason` adds `'cliMissing'`.
- `KubernetesView.vue`: a third `ModuleStart` branch, `data-testid="kube-cli-missing"`: title, body
  naming kubectl, and the steps as copyable commands (`kube-cli-step-install`,
  `kube-cli-step-check`) plus a docs link button. Check `packages/` for a shared copyable command row
  first; Space's `CliToolsPane.vue` is app-local, so reuse is not possible across apps. If none exists,
  render with `Identifier` (copyable) and the existing copy helper. Tailwind only, no scoped style.
  A Retry button refetches the clusters query.
- `ClusterUnavailable.vue`: `COPY.cliMissing` entry (kubectl was removed after the list loaded).
- `packages/theme/src/i18n/en.json`: keys `kubernetes.config.cliMissing.{title,body}`,
  `kubernetes.config.cliMissing.docs.label`, `kubernetes.unavailable.cliMissing.{title,hint}`,
  `backend.studio.kubernetes.kubectlMissing.error`. Copy terse, names kubectl and Homebrew.

## 8. Tests

### 8.1 Fake kubectl on PATH (flowharness)

- New package `apps/kira-studio/internal/flowharness/fakekubectl`. It runs kubectl's own commands
  in-process: a cobra root over `k8s.io/kubectl/pkg/cmd/{get,logs,apiresources,config,version}` with
  `genericclioptions.ConfigFlags` (`k8s.io/kubectl` and `k8s.io/cli-runtime` v0.37.2, Apache-2.0,
  matching client-go v0.37.2; verified on the module proxy). Not `cmd.NewDefaultKubectlCommand`: it
  pulls kustomize. Output, exit codes and error text are kubectl's own, so the classifier is tested
  against real strings, not guessed ones. Library over hand-rolled fake (CLAUDE.md library rule).
- Before running, it appends one JSON line `{argv, kubeconfig, path}` to `$KIRA_FAKE_KUBECTL_LOG`.
  A marker file `kubectl.fail` next to the binary makes it exit 1 before running (a broken-binary
  case, if a test needs it).
- Registration: `flowharness.RegisterTool(name, run)`; `Main` dispatches registered names on
  `argv[0]`; `writeBin` symlinks every registered tool into `BinDir`. `fakekubectl` registers
  `kubectl` in `init`. Only kube test binaries blank-import it (`kubeflow`, `dbmcpflow`,
  `tests/claude`), so other flow binaries do not link kubectl.
- `harness.go`: `kubeEnv` unchanged; build adds `kubernetes.WithKubectlCandidates(nil)`;
  `WithListLimits` callers move to `WithListMax`. `KIRA_FAKE_KUBECTL_LOG` points into the test temp
  dir.
- `KubeFake`: keep. Adjust only what real kubectl needs: legacy discovery fallback (aggregated
  discovery Accept answered with plain `APIGroupList`), `includeObject` on Table lists, namespaces
  Table when not forbidden. kubectl's discovery cache lands in the test `HOME`.
- `kubeflow/helpers_test.go` cleanup: besides the non-GET check, read the kubectl log and fail on any
  subcommand outside `config view`, `version`, `api-resources`, `get`, `logs`, any `--raw` or
  `--flatten`, and any cluster call without `--context`. Every kube flow test then guards read-only
  and the credential rule at runtime.

### 8.2 Flow tests (`apps/kira-studio/internal/flows/kubeflow`)

Update every existing file for the new behaviour; scenarios stay:

- `clusters_test.go`: contexts, servers, current, source, missing and invalid kubeconfig.
- `status_test.go`: ok (`v1.31.0-fake`), unauthorized (`bad-token`), plugin (`plugin-ctx` with
  `kira-fake-cred.fail`), unreachable (`down`), refresh.
- `discovery_test.go`: kinds, categories, `Partial` from `broken.example.com`, namespaces,
  restricted.
- `list_test.go`: columns (`Name`, `Ready`, `Status`, `Restarts`, `Age`, `IP`), all namespaces,
  label selector, bad selector, truncation through `WithListMax`. Drop the request-path paging
  assertions (kubectl owns paging); assert kubectl's `limit=500` query once.
- `describe_test.go`, `logs_test.go`, `ops_test.go` (command text is the real argv), `order_test.go`.
- New `kubectl_test.go`: `TestKubectlMissing`. Env PATH without kubectl, no candidates. Assert
  `Clusters().Config.State == "cliMissing"`, `Setup.Steps[0].Command == "brew install kubectl"`,
  probed list, and no op row. Then a cluster call on a stored id returns reason `cliMissing`.
  Contract key `KubernetesService.Clusters:cliMissing`.
- `TestKubectlArgv`: one call of each kind; assert from the kubectl log that `KUBECONFIG` and `PATH`
  reached the child and `config view` used `-o jsonpath` with no `users` selector.
- `dbmcpflow/kube_test.go`: rerun; approval statement text and contract follow.
- Contracts: rewrite `tests/contract/kubernetes.json` and `dbmcp-kube.json` with
  `KIRA_CONTRACT=write`, then rerun without it.

### 8.3 Unit tests (earn the bar)

- `table_test.go`: header offsets, multi-word headers, empty middle cells, `<none>`, last column
  with spaces, wide runes, continuation lines, NAMESPACE detection, title-casing with acronyms. A
  parser with interacting rules.
- `errors_test.go`: table-driven classifier over cases the fake cannot produce (timeout text,
  interactive plugin, config errors, klog-prefixed stderr). A decision structure of 8 ordered rules.
- `logs_test.go` stays. No other unit test.

### 8.4 UI spec

- `tests/ui/kubernetes-module.spec.ts`: new test, same scenario as `TestKubectlMissing`. The mock
  serves `KubernetesService.Clusters:cliMissing`. Assert `kube-cli-missing` visible,
  `kube-cli-step-install` shows `brew install kubectl`, copy works, Retry refetches.
- `ClusterUnavailable` `cliMissing` reason: one assertion through a `Status` mock answering reason
  `cliMissing`.
- Run the `kubernetes-*` specs, `copy-coverage`, `copy-wording`, `consistency` (new keys, changed
  fixtures). The chapter's one full suite run comes later, before review.

### 8.5 Real claude

`apps/kira-studio/tests/claude/kube_test.go` (`TestKubeMcpThroughClaude`): blank-import
`fakekubectl`; tidy the nested module. Run once at phase end: `bun run test:claude:studio -run
TestKubeMcpThroughClaude -count=1` (DEV_ENVIRONMENT table row "DB MCP" covers
`internal/kubernetes/**`).

## 9. Docs

- `docs/ARCHITECTURE.md` "Kubernetes module": rewrite the Client, Environment, Connect, Lists,
  Describe, Logs and Flow-tests bullets for kubectl. State the P439 decision reversal (user request,
  P487), the table-parse reason (3.1), `api-resources -o wide` (3.2), the logs open grace (3.4), the
  missing state, and the cost below. P484 bullet: command text is the argv that runs.
- "CLI credentials: never touched": kubectl line (env passthrough with resolved PATH and KUBECONFIG;
  jsonpath view of contexts and servers only; never `--raw`, `--flatten` or users). G5 bullets
  (rules list near line 1331, and the guard paragraph) add `internal/kubernetes`.
- "Known open items": each kubectl call runs the context's exec plugin again (aws `eks get-token`
  does not cache; about 1 s per call). Kira cannot cache tokens without touching credentials.
- `docs/DEV_ENVIRONMENT.md` "Kubernetes flow tests": fake kubectl on PATH (`fakekubectl`, real
  kubectl commands in-process), the kubectl log, `WithListMax`, no host kubectl reached.
- `go.mod`: add `k8s.io/kubectl`, `k8s.io/cli-runtime` (test-only importers). `client-go` stays as an
  indirect or test dependency; `go mod tidy`.

## 10. Stream ownership

One stream. No split: the backend change is atomic, and the UI state needs the contract fixture the
backend flow writes.

| Stream | Owns |
|---|---|
| A | `apps/kira-studio/internal/kubernetes/**`, `apps/kira-studio/internal/flowharness/{harness.go,main.go,kubefake.go}`, `apps/kira-studio/internal/flowharness/fakekubectl/**`, `apps/kira-studio/internal/flows/kubeflow/**`, `apps/kira-studio/internal/flows/dbmcpflow/kube_test.go`, `apps/kira-studio/internal/dbmcp/kube_tools.go` (only if a signature moves), `apps/kira-studio/tests/claude/{kube_test.go,go.mod,go.sum}`, `apps/kira-studio/tests/contract/{kubernetes.json,dbmcp-kube.json}`, `internal/conventiontest/conventions_test.go`, `apps/kira-studio/frontend/src/kubernetes/{wire.ts,components/KubernetesView.vue,components/ClusterUnavailable.vue}`, `packages/theme/src/i18n/en.json`, `apps/kira-studio/tests/ui/{kubernetes-module.spec.ts,support/kubernetesFixture.ts,support/mockRuntime.ts}`, `go.mod`, `go.sum`, `docs/ARCHITECTURE.md`, `docs/DEV_ENVIRONMENT.md`, `docs/v0.2.29/SPEC.md` (P487 row Result) |

## 11. Commit sequence

1. `test: fake kubectl on PATH runs kubectl commands in-process` (fakekubectl, RegisterTool, writeBin,
   go.mod).
2. `feat: Kubernetes runs kubectl for kubeconfig, status and discovery` (kubectl.go, config.go,
   errors.go, table.go, discovery.go, service.go, commands.go; delete client.go).
3. `feat: Kubernetes lists, describe and logs run kubectl` (list.go, describe.go, logs.go,
   mcpreads.go, doc.go; client-go gone from the package).
4. `test: Kubernetes flows over kubectl` (kubeflow, dbmcpflow, unit tests, contracts).
5. `feat: Kubernetes shows install steps when kubectl is missing` (frontend, i18n, UI spec).
6. `test: guard G5 covers the Kubernetes module` (conventiontest).
7. `docs: Kubernetes module drives kubectl` (ARCHITECTURE, DEV_ENVIRONMENT, SPEC Result).

Fast checks (Go build, vet, lint, typecheck) per commit; hooks pass without `--no-verify`.

## 12. Done when

- `go list -deps ./apps/kira-studio/internal/kubernetes` shows no `k8s.io/client-go`.
- `grep -rn "client-go\|clientcmd" apps/kira-studio/internal/kubernetes` is empty.
- `go test ./apps/kira-studio/internal/flows/kubeflow ./apps/kira-studio/internal/flows/dbmcpflow
  ./apps/kira-studio/internal/kubernetes ./internal/conventiontest` passes, contracts unchanged on a
  rerun without `KIRA_CONTRACT`.
- The kubectl-log check in `boot` ran in every kube flow and found only the 5 allowed subcommands.
- `TestKubectlMissing` and its UI spec pass on the same fixture.
- `kubernetes-*`, `copy-coverage`, `copy-wording`, `consistency` UI specs pass.
- `TestKubeMcpThroughClaude` passes once.
- No UI file outside section 7 changed (`git diff --stat 41713b47 -- apps/kira-studio/frontend`).

## 13. Risks

- kubectl table edge cases (a cell with a newline or tab). Continuation lines drop; the unit test
  pins the behaviour.
- Logs open grace: a slow exec plugin (over 3 s) reports an open error as the stream's end event,
  not from `LogsOpen`. Acceptable; documented.
- Per-call process start (about 50 ms) plus exec plugin cost on the 10 s list refetch. Known open
  item (9).
- Header title-casing can differ from the server's own name for an acronym outside the set. The UI
  only depends on `Name`, `Ready`, `Status`, `Age`.
