# P627 journeys: Kubernetes module

Dedicated Kubernetes file. Earlier entries stay valid: J-K8S-001..002 (`journeys-studio-mcp-approvals.md`),
J-K8S-010..012 (`journeys-infra-misc-2.md`). Ids here start at J-K8S-020.

Runs in seconds: `flowharness.KubeFake` (in-process HTTPS API server: discovery, Table lists, objects, events,
logs, exec-plugin cluster); seams `kubernetes.WithEnv`, `WithListLimits(page, max)` (e.g. page 5, cap 12 instead
of 500 and 5,000), `WithClock`. Timeouts (`callTimeout` 15 s, `listTimeout` 30 s, `statusTimeout` 5 s) hit by a
fake that never answers plus shortened context, never by waiting.

Facts read from code (`kubernetes/service.go`, `list.go`, `logs.go`, CodeGraph; `docs/ARCHITECTURE.md` 3248):

- Read-only module: GET only; no create, update, delete, exec, attach, port-forward. Journeys asking for exec or
  port-forward assert absence (OQ-K8S-1).
- Cluster row per kubeconfig context and server (`kube_clusters`); id stable; context renamed or server changed
  means unknown cluster. Kubeconfig re-parsed when file size or mtime changes. Order: user order, then current
  context, then name. Connect lazy: nothing probed until selected.
- `Status` returns trouble as value with reason (unreachable, unauthorized, forbidden, plugin, config, timeout);
  `refresh` drops client and ends that cluster's log streams.
- List: server Table format, `limit` page and `continue`, row cap; `truncated` true past cap; all-namespaces
  adds Namespace column; row key uid else `namespace/name`; bad label selector `E_BAD_REQUEST` before any call.
- Describe: overview, labels, annotations, owners, conditions, up to 200 events, YAML; Secret `data` and
  `stringData` become `<N bytes>`; last-applied annotation and managedFields dropped.
- Logs: tail 100, 1000, 5000 or -1 only; follow with all asks 20,000 lines; non-follow byte limit 8 MiB; line cap
  64 KiB with marker `…`; one trailing CR stripped; invalid UTF-8 replaced; coalesced 16 ms or 500 lines;
  duplicate stream id refused; previous-container logs option; UI keeps newest 20,000.
- Every cluster call records one op (kind `kubernetes`); MCP calls source `mcp`; MCP `ReadLogs` cap 256 KiB.

### J-K8S-020 On-call session: find failing pod across namespaces, read logs, copy evidence

- Status: partial (`kubeflow` discovery, list, logs, ops; chain new).
- Persona: on-call engineer.
- Pre: KubeFake cluster `prod`: 14 namespaces, 60 kinds (3 CRDs), 4,000 pods; one pod CrashLoopBackOff with 2
  containers and previous logs; kubeconfig with 3 contexts.
- Steps:
  1. Open module: cluster list shows 3 contexts in order current first; none probed.
  2. Select `prod`: status ok with server version; pods list opens for default namespace.
  3. Switch namespace to "all": Namespace column appears.
  4. Type filter `checkout`; sort by Restarts descending.
  5. Open kind picker; search `ingress`; open; go back to Pods via kind bar (recent kinds).
  6. Click failing pod (preview tab); double-click to keep tab.
  7. Describe: conditions, events (most recent first), owners; follow owner link to ReplicaSet, then Deployment.
  8. Copy YAML of Deployment; copy pod name; copy `namespace/name`.
  9. Open logs: container picker (2 containers), tail 1000, timestamps on, follow on.
  10. Toggle previous: crash output of last run.
  11. Search log for `panic`; select 40 lines around; copy.
  12. Open Secret referenced by pod; view YAML; copy.
  13. Open operations log.
  14. Close tab with stream running.
- Oracle: `KubeFake.Requests()`: no request to any cluster before step 2; only GET verbs overall; step 3 list path
  without namespace; lists carry `limit` and `continue`; clipboard: YAML parses and has no `managedFields` nor
  last-applied annotation, names exact; log clipboard = selected lines with timestamps as shown; Secret YAML shows
  `<N bytes>` for each key and clipboard never holds decoded or base64 secret data; op log has one row per call
  with kubectl-style command text; after 14 stream closed on server (fake open stream count 0).
- Risk: secret leak through copy; follow stream left open.

### J-K8S-021 Large cluster: paging, truncation, sort and filter honesty

- Status: partial (`kubeflow` list_test with `WithListLimits`).
- Pre: `WithListLimits(5, 12)`; namespace with 30 pods; other with exactly 12; other with 13; empty one.
- Steps:
  1. List 30-pod namespace: truncated banner with count shown and cap.
  2. Sort by name descending; filter text.
  3. Apply label selector `app=web` (8 pods).
  4. Type bad selector `app==`, `a in (`, 2,000-char selector.
  5. List namespace with 12 pods, then 13.
  6. List empty namespace; list cluster-scoped kind (nodes) with namespace selected.
  7. KubeFake returns 410 Gone for continue token mid-paging.
  8. KubeFake hangs on page 2.
  9. Refresh while a list call is in flight; switch namespace mid-flight.
- Oracle: 1 rows 12, `truncated` true, 3 requests (5+5+2 rows consumed); UI states that sort and filter cover
  loaded rows only (OQ-K8S-2: wording); 3 selector passed to server, not truncated; 4 `E_BAD_REQUEST`, zero
  requests; 5 exactly 12 gives `truncated` false, 13 gives true; 6 empty state, no namespace segment in node
  path; 7 readable error with retry, no partial list presented as complete; 8 timeout reason, list cancelled; 9
  stale response never overwrites newer namespace's rows.
- Risk: off-by-one at cap; user trusting sorted truncated list.

### J-K8S-022 Credentials: expired, RBAC-denied, exec plugin, unreachable

- Status: partial (`kubeflow` exec-plugin cluster; reasons).
- Pre: contexts: `ok`; `expired` (401); `limited` (403 on secrets and on cluster-scoped lists, allowed in one
  namespace); `plugin` (exec credential plugin script: first run fails, then succeeds); `gone` (connection
  refused); `slow` (never answers).
- Steps:
  1. Select `expired`: status unavailable, reason unauthorized, copy says re-login; press Retry after fake starts
     accepting token.
  2. Select `limited`: namespaces list forbidden: UI offers typing namespace by hand.
  3. Type allowed namespace; list pods works; list secrets forbidden; describe pod works; events forbidden.
  4. Select `plugin`: first failure shows plugin stderr trimmed; Retry succeeds.
  5. Plugin prints token on stdout; check logs and op rows.
  6. Select `gone`, then `slow`; cancel `slow` from operations panel.
  7. Switch quickly `slow` to `ok`.
  8. Agent (MCP) reads `limited` secrets.
- Oracle: each reason maps to distinct copy (unauthorized, forbidden, plugin, unreachable, timeout); 2 and 3
  partial-permission state usable, describe shows "events unavailable (forbidden)" not whole-page error; no token,
  cert or plugin stdout in app log, op log or error text (grep fixture token); plugin command resolved to
  absolute path from probed PATH; 6 cancel ends request (fake sees disconnect); 7 `ok` data shown, late `slow`
  error not shown on `ok`; 8 tool result forbidden text, op row source `mcp`.
- Risk: credential material in logs; whole view dead on one forbidden sub-call.

### J-K8S-023 Context switch and kubeconfig change mid-flow

- Status: new.
- Steps:
  1. On `prod` keep 3 tabs (pod describe, logs following, deployment YAML).
  2. Select cluster `dev` in sidebar.
  3. Return to `prod`.
  4. In terminal `kubectl config use-context dev`; return to app.
  5. Edit kubeconfig: rename context `prod` to `production`.
  6. Edit kubeconfig: change `dev` server URL.
  7. Remove kubeconfig file; restore it.
  8. Set `KUBECONFIG` to two files with duplicate context name.
  9. Reorder clusters by drag and by keyboard menu; restart app.
- Oracle: tabs are per cluster and per window, in memory: `prod` tabs reappear at 3 with logs stream still one
  stream (or ended marker), never showing `dev` data; 4 only "current" badge moves, selection stays; 5 old row
  becomes unknown cluster (tabs show cluster-gone state, no request to renamed target under old id), new row
  appears with fresh id and default MCP settings; 6 same; 7 config problem reported in list header, not error
  dialog; 8 first file wins; 9 `kube_clusters.sort_order` persisted, tabs not restored after restart.
- Risk: request sent to different server under same stored id (MCP approval "allow" carried to new server).

### J-K8S-024 Logs under stress and edge input

- Status: partial (`kubeflow` logs_test).
- Pre: KubeFake pod log script: 100,000 lines, one 200 KiB line, CRLF lines, invalid UTF-8, line without
  newline at EOF, timestamps; second container; init container.
- Steps:
  1. Open logs tail 100 no follow: 100 lines.
  2. Tail all no follow: stops at 8 MiB; marker shown.
  3. Follow with tail all: starts from last 20,000.
  4. Burst of 50,000 lines: UI stays responsive; buffer capped.
  5. Open same pod logs in second tab and in second window.
  6. Send duplicate stream id (replay of open).
  7. Pod deleted during follow.
  8. Open logs of container that does not exist; previous logs when none.
  9. Pause follow, scroll, select across 3,000 lines with Shift+click, copy.
  10. Status refresh on cluster while following.
  11. Close window.
- Oracle: request params `tailLines`, `limitBytes` (non-follow only), `follow`, `previous`, `timestamps`,
  `container` exact per step; long line cut at 64 KiB with `…`; CR stripped; trailing partial line delivered at
  EOF; events arrive in batches of at most 500; 6 Contract error "streamId is already open", first stream
  unaffected; 7 final event with error text, stream removed; 8 open error returned synchronously with reason;
  clipboard line count 3,000 exact; 10 stream ended once (client dropped); 11 server stream count 0.
- Risk: partial last line lost; copy of virtualized selection.

### J-K8S-025 Agent investigation through MCP with per-cluster approval

- Status: partial (`dbmcpflow` kube tools).
- Steps:
  1. Cluster `prod` MCP off: agent lists clusters (prod absent or marked off).
  2. Enable MCP for `prod`, approval `prompt`, description 1,001 chars (refused), then 200 chars.
  3. Agent lists pods: approval item `kube.read:<id>` shows command text; approve.
  4. Agent reads logs (256 KiB cap) of noisy pod.
  5. Agent describes Secret.
  6. Set approval `allow`; agent lists again.
  7. Deny an item after 3 are queued; minimise dialog with Escape.
  8. Disable MCP while a call waits.
  9. Restart app; agent retries.
- Oracle: off cluster yields no request; description limit error; approved call's op row source `mcp`; log result
  at most 256 KiB with truncation note; Secret values `<N bytes>`; `allow` needs no item; minimise never answers;
  8 waiting call ends refused; settings persist (`kube_clusters` MCP columns).
- Risk: approval bypass after settings change.

### J-K8S-026 Features outside the surface

- Status: new (defines absence).
- Steps: look for exec, attach, port-forward, edit, delete, scale, restart rollout in menus, shortcuts, MCP.
- Oracle: none offered; `KubeFake.Requests()` across whole suite contains GET only. If intended, OQ-K8S-1.
- Risk: write path added without approval surface.
