# P485 Kubernetes: reorder clusters

Plan for SPEC row P485. Planned against `49c61ed1`. One sequential implementer, Stream A (P485, P486,
P487 run in order). Discovery used `codegraph_explore` over the cluster store (`ClusterStore`,
`KubeClustersRepo`, `Service.Clusters`), the cluster tree (`ClusterTree.vue`, `queries.ts`,
`control.ts`) and the existing reorder precedents (`connectionReorder.ts`, `useTreePointerDrag`,
`useSortableReorder`, `ConnectionsRepo.Reorder`, `sqlitex.ReorderSortOrder`, dbflow
`TestConnectionOrder`).

## 1. Goal and invariants

- User orders the cluster list. Order persists across restarts and shows in every window and every
  consumer of `Service.Clusters` (tree, Settings MCP pane, MCP `list_clusters`).
- Default order unchanged until the user reorders: current context first, then name. Existing
  contracts (`kubernetes.json` `KubernetesService.Clusters*`, `dbmcp-kube.json`) stay byte-identical.
- Reorder is a setting write: no op row (P484 rule). No cluster call, so P487 never touches it.
- No new wire field on `Cluster`. Order is list order.

## 2. Interaction: drag, with its keyboard path

Pick: pointer drag on the cluster row. Reason: Studio's one ordered sidebar list, connections
(P308), reorders by drag; the Kubernetes tree sits in the same sidebar and gets the same gesture.
Drag alone is pointer-only, so the existing keyboard chord (`list.moveUp`/`list.moveDown`, Alt+Up/
Down) and context-menu Move up / Move down ship as its accessible path, exactly as P308 does. One
design, not two.

Drag engine: `useTreePointerDrag` (`packages/workbench/src/behaviour/treePointerDrag.ts`, on VueUse
`useMouse`, `useElementByPoint`, `useRafFn`; MIT). It never moves DOM, draws a drop line, auto-scrolls
at edges, cancels on Escape and swallows the ending click.

Declined: `useSortableReorder` (vue-draggable-plus over SortableJS, both MIT, already a dependency).
Requirement it fails: the cluster tree is a flat list where an expanded cluster's category and kind
rows are siblings of the cluster row. Sortable moves the dragged row's DOM node alone, so an expanded
cluster drags without its subtree, and a drop can land between another cluster's children. The tree
drag resolves only cluster rows and only draws a line. No new dependency.

Rules (P308's):

- Drop lands before or after the cluster row under the pointer (top or bottom half). A kind or
  category row is no target.
- Disabled while `ui.treeSearch` is non-empty (visible order is partial) or fewer than 2 clusters.
- Menu Move up disabled on the first cluster, Move down on the last; both disabled while searching.
- Optimistic: the clusters cache reorders at once; the backend result replaces it; a failure
  restores the stored order.

## 3. Backend

### 3.1 Migration `0057_p485_kube_cluster_order.sql` (new)

```sql
-- P485: user order of the cluster list. NULL until the user first reorders; NULL rows sort last.
ALTER TABLE kube_clusters ADD COLUMN sort_order INTEGER;
```

Register `{Version: 57, Name: "p485_kube_cluster_order", File: "0057_p485_kube_cluster_order.sql"}`
in `internal/storage/migrations/embed.go`. Nullable on purpose: NULL means never placed, which keeps
today's default order (1) and puts a context first seen after a reorder at the end. No data migration,
so no migration test.

### 3.2 Store (`internal/kubernetes/store.go`, `internal/storage/repos/kube_clusters.go`)

- `ClusterRecord` gains `SortOrder *int` (nil when NULL).
- `ClusterStore` gains `Reorder(ids []string) error`: applies the order and rewrites `sort_order`
  dense.
- Repo: `kubeClusterCols` adds `sort_order`; `scanKubeCluster` scans it via `sql.NullInt64`.
  `Reorder` runs `sqlitex.ReorderSortOrder` in a transaction, as `ConnectionsRepo.Reorder` does:
  - select: `SELECT id FROM kube_clusters ORDER BY sort_order IS NULL, sort_order, context`
  - update: `UPDATE kube_clusters SET sort_order = ? WHERE id = ?`
  Rows `ids` leaves out (stale contexts no longer in the kubeconfig) follow in their existing order,
  so every row is placed after the first reorder.

### 3.3 Service (`internal/kubernetes/service.go`, `types.go`, `bound.go`)

- `types.go`: `ReorderArgs{IDs []string \`json:"ids"\`}`.
- `Service.Clusters` sort replaces the current comparator:
  1. both placed: `SortOrder` ascending;
  2. one placed: placed first;
  3. neither: current first, then context name (today's rule).
- `Service.Reorder(ctx, ReorderArgs) (ClustersResult, error)`:
  1. empty `ids`: `ipcerr.Required("ids")`; a duplicate: `ipcerr.Contract("ids must be unique")`
     (connections' wording).
  2. each id through `s.store.Get`; a store error `ipcerr.Internal`; a missing row
     `unknownCluster()` (E_NOT_FOUND). Checked before the write, so a bad id never reaches
     `ReorderSortOrder`'s plain error.
  3. `s.store.Reorder(ids)`; error `ipcerr.Internal`.
  4. `s.clustersChanged()` (3.4), then return `s.Clusters(ctx)`.
  Not wrapped in `s.record`: a setting write (P484 rule).
- `BoundService.Reorder(args ReorderArgs) (ClustersResult, error)`. `bridge.KubernetesService`
  embeds `BoundService`, so the binding appears with no bridge edit. Regenerate bindings
  (`wails3 task common:generate:bindings`; gitignored).

### 3.4 Other windows see the change

Studio runs several windows (ARCHITECTURE "Multi-window (P8)"). The desktop query client has
`staleTime: Infinity` and no focus refetch, so another window keeps its old order until told.

- `channels.go`: `ChannelClusters = "kira:kubernetes:clusters"`, payload `struct{}`. Comment: a
  cluster setting changed; listeners refetch `Clusters`.
- `s.clustersChanged()` emits it through `s.emit`. Called by `Reorder` and by `SetMcp` after a
  successful write: `SetMcp` is the same concern (a stored cluster setting other windows show) and
  has the same stale-window gap today. One line, same channel; no second mechanism.

## 4. Frontend (`apps/kira-studio/frontend/src/kubernetes/`)

### 4.1 Wire and control

- `wire.ts`: `KUBE_CHANNEL.clusters = 'kira:kubernetes:clusters'`.
- `control.ts`: `reorder: (ids: string[]) => unwrap(KubernetesService.Reorder({ ids })).then((r) =>
  trust<KubeClusters>(r))`; `onClusters: (cb: () => void) => on(KUBE_CHANNEL.clusters, cb)`.

### 4.2 Queries (`queries.ts`), TanStack Query

- `useClusters()`: subscribe to `kubeControl.onClusters` and invalidate `kubeKeys.clusters`;
  unsubscribe with VueUse `tryOnScopeDispose`. Every consumer (tree, MCP dialog, Settings pane)
  stays fresh; TanStack dedupes the concurrent invalidations.
- `useReorderClusters()`: `useMutation`, key `['kubernetes', 'reorder']`.
  - `onMutate(ids)`: `cancelQueries(clusters)`, snapshot, `setQueryData` with the clusters in `ids`
    order (ids not listed keep their place after them, the backend's rule); return the snapshot.
  - `onSuccess(result)`: `setQueryData(clusters, result)`.
  - `onError(_, _, snap)`: restore the snapshot, invalidate clusters.
- `moveCluster(id, step)` helper on top of it (swap with neighbour, as `moveConnection`).

### 4.3 Drop math, shared with connections

`orderAfterDrop` and its drop type move from `project/connectionReorder.ts` into
`treePointerDrag.ts` as exports (`orderAfterDrop`, `EdgeDrop { id; edge: 'top' | 'bottom' }`).
`connectionReorder.ts` imports them; `ConnectionDrop` becomes an alias of `EdgeDrop`. No behaviour
change for connections. Avoids a second copy of the same splice.

### 4.4 `components/clusterReorder.ts` (new): `useClusterReorder`

Same shape as `useConnectionReorder`, over `useTreePointerDrag`:

- row selector `[role="treeitem"][data-kind="cluster"]`, id from `data-cluster-id`;
- `resolve` returns `EdgeDrop` for a cluster row other than the dragged one, else null;
- `onDrop` calls `orderAfterDrop`, then `onReorder(next)` only when the order changed;
- returns `draggingId`, `drop`.

### 4.5 `ClusterTree.vue`

- Cluster row: add `data-kind="cluster"`, `:data-cluster-id="row.cluster.id"`, `relative` in its
  class, and the drop line as a child when `drop?.id === row.cluster.id`: Tailwind
  `pointer-events-none absolute inset-x-0 h-0.5 bg-focus` plus `top-0` / `bottom-0`,
  `data-testid="kube-drop-indicator"` (TreeRow.vue's markup). Dragged row gets `opacity-50`
  (`SORTABLE_GHOST_CLASS`). Tree gets `select-none` while dragging.
- `useClusterReorder({ container: treeEl, ids: () => list.value.map((c) => c.id), disabled: () =>
  searching.value || list.value.length < 2, rowHeight: () => rowHeight.value,
  onReorder: (ids) => reorder.mutate(ids) })`, with `const { single: rowHeight } = useRowHeight()`
  at setup (`@workbench/lib/rowHeight`, as ProjectTree). `onError` logs through `logError('kubernetes',
  'reorder failed', err)`, as ProjectTree does.
- `menuFor` cluster items gain `move-up` / `move-down` (labels `common.moveUp.tooltip`,
  `common.moveDown.tooltip`, icons `arrow-up` / `arrow-down`, shortcuts `list.moveUp` /
  `list.moveDown`, disabled per 2).
- `useTreeShortcuts(nav, { 'list.moveUp': …, 'list.moveDown': … })`: acts only on a cluster row key
  (`c:<id>`), runs the menu item (one gating path, as ProjectTree's `moveViaMenu`), then
  `nav.focusKey(key)` after the move lands so focus stays on the moved row.
- No scoped `<style>`; Tailwind only. No new i18n key: `common.moveUp.tooltip` and
  `common.moveDown.tooltip` exist.

### 4.6 Pinia

No store change. Order is server state (TanStack). `useKubernetesUiStore` keeps selection and
search only.

## 5. Tests

One scenario, both sides of the IPC boundary. Kubeconfig contexts from `boot`: `kind-dev`
(current), `bad-token`, `down`, `plugin-ctx`; default order `kind-dev, bad-token, down, plugin-ctx`.
Scenario move: drag `down` above `kind-dev`, giving `down, kind-dev, bad-token, plugin-ctx`.

### 5.1 Backend flow: `internal/flows/kubeflow/order_test.go` (new) `TestClusterOrder`

1. `Clusters`: default order as above.
2. Mark events and ops. `Reorder{IDs: [down, kind-dev, bad-token, plugin-ctx]}`: result in that
   order; `Clusters` returns it too; one `ChannelClusters` event after the mark; no op row
   (`app.W.Ops.Recent` has no `kubernetes` row).
3. Contracts under scenario `kubernetes` (via `e.contract`): `args:KubernetesService.Reorder` (the
   args) and `KubernetesService.Reorder` (the result).
4. Errors: no ids and a duplicate id give `E_CONTRACT`; id `nope` gives `E_NOT_FOUND`; order
   unchanged after each.
5. New context last: write a second kubeconfig file with one context `aa-new` (sorts first by name,
   so the test proves NULL-last, not alphabetic luck) on `e.fake.URL`; set `KUBECONFIG` to both
   paths. `Clusters`: `down, kind-dev, bad-token, plugin-ctx, aa-new`.
6. `app.Restart`: same order.
7. `SetMcp` on `kind-dev` emits one `ChannelClusters` event.

No unit test: `ReorderSortOrder` is shared and exercised here; the comparator has three branches.

### 5.2 UI spec: `tests/ui/kubernetes-order.spec.ts` (new), same scenario

Fixture: `kubernetesFixture.ts` `defaults()` gains `Reorder: () => k(KEY('Reorder'))`; a helper
`clustersIn(order)` builds a `Clusters` result from the contract in a given order (for keyboard and
failure tests). Drag helper as `connection-order.spec.ts` `dragTo`.

1. `contract: drag a cluster above another: drop line, one Reorder call, new order`: drag `down` to
   top of `kind-dev`; `kube-drop-indicator` shows on `kind-dev`; release; row order equals the
   contract result; indicator gone; one `Reorder` call whose args equal
   `args:KubernetesService.Reorder`.
2. `dropping on a kind row, or pressing Escape, reorders nothing`: expand `kind-dev` (kinds show);
   drag `down` onto a kind row: no indicator; release. Drag onto `kind-dev`, Escape: indicator gone.
   No `Reorder` call; order unchanged.
3. `menu Move up is disabled on the first cluster; Alt+ArrowDown moves and keeps focus`: right-click
   first row: `menu-item-move-up` disabled. Focus `kind-dev`, Alt+ArrowDown: `Reorder` args
   `[bad-token, kind-dev, down, plugin-ctx]`; `kind-dev` row focused after.
4. `a search disables reorder`: type in tree search; menu Move up/down disabled; drag shows no
   indicator.
5. `a failed Reorder restores the stored order`: `Reorder` answers `kubeError('E_NOT_FOUND', …)`;
   after the drag the order returns to the default.
6. `contract: a clusters event refetches the list`: emit `kira:kubernetes:clusters` with `Clusters`
   now answering the reordered list; rows follow.

Existing `kubernetes-module.spec.ts`, `kubernetes-mcp.spec.ts`, `kubernetes-ops.spec.ts` and
`connection-order.spec.ts` (4.3 touches its code) must pass unchanged.

Fast checks per commit: Go build, vet, lint, typecheck. Run only `kubernetes`, `storage/repos`,
`storage/migrations`, `flows/kubeflow`, `flows/dbflow` (`TestConnectionOrder`), `flows/dbmcpflow`
Go tests and the five specs above. Full suite runs once at chapter end.

## 6. Docs

`docs/ARCHITECTURE.md` "Kubernetes module", one bullet: P485 user order in `kube_clusters.sort_order`
(migration 0057, NULL until first reorder, NULL rows last by current-then-name);
`Service.Reorder` through `sqlitex.ReorderSortOrder`; drag via `useTreePointerDrag` plus
`list.moveUp`/`list.moveDown` and menu; Sortable declined for the flat tree; not an op;
`kira:kubernetes:clusters` tells other windows to refetch after `Reorder` and `SetMcp`. Edit the
"Cluster rows" bullet's ordering words only if they name an order (today they do not).

## 7. Commits

1. `feat: store Kubernetes cluster order` (3.1, 3.2, 3.3, 3.4; bindings).
2. `refactor: share tree drop order between connections and clusters` (4.3).
3. `feat: reorder Kubernetes clusters by drag, keyboard and menu` (4.1, 4.2, 4.4, 4.5).
4. `test: Kubernetes cluster order flow and contract` (5.1).
5. `test: Kubernetes cluster order UI spec` (5.2).
6. `docs: Kubernetes cluster order` (6; SPEC P485 row Done with Result).

## 8. Ordering consequences

- P484: `Reorder` and `SetMcp` stay unrecorded. Flow step 2 asserts it. The ops contract does not
  change.
- P486 (redesign) runs next and may replace `ClusterTree.vue`'s layout. It keeps reorder on whatever
  cluster list it renders: `useClusterReorder` is separate from the tree for that reason. Keep
  `data-kind="cluster"`, `data-cluster-id`, `kube-cluster-row`, `kube-drop-indicator` and menu ids
  `move-up` / `move-down`, or update `kubernetes-order.spec.ts` in the same commit. If P486 moves
  clusters out of a tree into a non-tree list with no child rows, `useSortableReorder` becomes
  viable; P486's plan decides and says why.
- P487 (kubectl) changes how contexts are gathered (`kubectl config view -o json` or
  `get-contexts`), not where they are stored or sorted. The sort stays in `Service.Clusters` after
  gathering, over `ClusterRecord.SortOrder`. `Reorder` spawns nothing. P487's regenerated
  `Clusters` contract keeps the default order unless its argv changes what contexts exist.
- MCP `list_clusters` (dbmcp `kubeListClusters`) reads `Service.Clusters`, so it follows user order.
  `dbmcp-kube.json` does not change: its scenario never reorders.

## 9. Stream file ownership (Streams A, B, C in parallel)

Stream A (this phase) owns, with zero overlap with Streams B and C (`apps/kira-space`):

- `apps/kira-studio/internal/kubernetes/` (`store.go`, `service.go`, `types.go`, `bound.go`,
  `channels.go`)
- `apps/kira-studio/internal/storage/repos/kube_clusters.go`
- `apps/kira-studio/internal/storage/migrations/0057_p485_kube_cluster_order.sql`, `embed.go` (one
  appended line)
- `apps/kira-studio/internal/flows/kubeflow/order_test.go`
- `apps/kira-studio/frontend/src/kubernetes/` (`wire.ts`, `control.ts`, `queries.ts`,
  `components/ClusterTree.vue`, `components/clusterReorder.ts`)
- `apps/kira-studio/frontend/src/project/connectionReorder.ts` (import swap only)
- `apps/kira-studio/tests/ui/kubernetes-order.spec.ts`, `tests/ui/support/kubernetesFixture.ts`
- `apps/kira-studio/tests/contract/kubernetes.json` (generated by the flow test)

Shared files and conflict rule:

- `packages/workbench/src/behaviour/treePointerDrag.ts`: Stream A adds two exports only. Neither B
  (terminal file links, file-open window) nor C (ADE) is expected to touch it. If one does, keep both
  sides: the change is additive.
- `docs/v0.2.29/SPEC.md`: each stream edits only its own rows (Status and Result). A conflict is
  two hunks in different rows; keep both.
- `docs/ARCHITECTURE.md`: Stream A edits only the "Kubernetes module" section. Keep both sides of any
  conflict; never reorder another stream's bullets.
- `packages/theme/src/i18n/en.json`: Stream A adds no key. If a later Stream A phase must add one,
  insert under the `kubernetes.*` keys only; a conflict with another stream's keys merges as union,
  then run the i18n check.
- `apps/kira-studio/internal/storage/migrations/embed.go`: Studio-only; B and C migrate Kira Space's
  DB (`apps/kira-space`). If another Studio change lands 0057 first, take the next free number and
  rename the file in the same commit.
- Contracts: Stream A writes only `tests/contract/kubernetes.json`. B and C write their own scenario
  files. A regenerated file is never hand-merged: rerun the owning flow test.

Landing: rebase onto `chapter/v0.2.29`; a real conflict outside the shared files above means the
ownership boundary was wrong. Stop and report it rather than resolving by guess.
