# P486 Kubernetes: resource browser redesign, tabbed pods

Plan for SPEC row P486. Planned against `1f510d3a`. One sequential implementer, Stream A (P487 runs
after it). Discovery used `codegraph_explore` over the module frontend (`KubernetesView`,
`ResourceBrowser`, `ResourceDetail`, `LogsView`, `ClusterTree`, `useKubernetesUiStore`, `queries.ts`),
the tab seams (`TabStrip`, `TabStripHost`, `createTabsStore`, `useTabsStore`, `STUDIO_TAB_KIND_MODE`,
`createProofTabHost`, `AdeReviewWindow`) and the primitives (`PickerPopover`, `pickerOptions`,
`ToggleGroup`, `usePersistedState`, `useLiveSize`, `useShortcut`, `useTreeShortcuts`). Several module
files had changed since the last index sync; those were read from disk.

No split into Parts. Both halves (navigation, tabs) rewrite the same three files (`state.ts`,
`ResourceBrowser.vue`, `KubernetesView.vue`); two Parts would rework them twice. Frontend only, one
implementer.

## 1. Goal and invariants

- Pods first: picking a cluster lists its pods at once. No kind tree.
- Other kinds on demand: a compact kind bar (Pods plus up to 3 recent kinds) and a searchable kinds
  picker grouped by category.
- Compact navigation: the sidebar is a flat cluster list (no child rows).
- Open a resource in a tab; several tabs at once; each pod tab keeps its own log stream.
- Frontend only. No Go change, no new or changed IPC method, no wire field, no contract change. The UI
  reads data only through `control.ts`, `queries.ts` and `wire.ts` (P487 constraint, 9).
- P484 holds: each cluster call still records through the backend seam; `queries.ts` background helper
  (`isRefetch`) stays on every query; each pod tab's log stream is its own op row.
- P485 holds: drag, Alt+Up/Down and menu reorder work on the new cluster list.
- Module stays read-only. No new action.

## 2. Layout

```
sidebar (KubernetesPanel)   main (KubernetesView)
+----------------------+   +--------------------------------+---------------------------------+
| KUBERNETES   [s][r]  |   | [Pod][Secret][Service] [More v]| [<] web-1 | web-2 | db-0 (prev)  |
| o kind-dev   Current |   | [ns: default v] [filter] [sel] | kind-dev > default > Pod web-1  |
| o bad-token          |   |  Name    Ready  Status         |       [Describe] [Logs]         |
| o down               |   |  web-1   1/1    Running   <-sel| ... logs / describe ...          |
| o plugin-ctx         |   |  web-2   1/1    Running        |                                 |
+----------------------+   +------------ resize -----------+---------------------------------+
```

- Sidebar: one row per context (icon, name, Current mark, status dot). Click selects the cluster.
- Main, left pane "resources": kind bar row, filter row, table. Full width while no tab is open or the
  tab pane is the only thing hidden; fixed px width (resizable, persisted) while tabs are open.
- Main, right pane "tabs": exists while at least one tab is open. A tab bar (`h-tabbar`, `bg-chrome`,
  `border-b`) holds a list toggle button and the module's own `TabStrip`; below it the active tab's
  panel.
- List toggle hides the resources pane so a log reads full width; persisted.
- Tabs are global to the window, not per cluster: switching cluster changes the list, never the open
  tabs.

### Why a module-local tab strip, not Studio's tab store

`createTabsStore`/`useTabsStore` is declined. Requirements it fails:

- It persists every non-terminal tab through `TabsControl` and Go's `RenderableTabKinds` (a parity
  test guards the vocabulary). Pod names are ephemeral (a rollout renames them), so a restored pod tab
  mostly points at a deleted pod. Pod tabs are deliberately in-memory, per window.
- Kubernetes mode is `tabStrip: false`; turning the shell strip on puts the list behind a tab, so
  opening a second pod means leaving the pod. The split keeps the list beside the tabs.
- `createTabsStore.ts` is Stream B's file this chapter (P490 plan 9).

The seam built for this exists: `TabStrip.vue` takes a `host` prop (`TabStripHost`), used by
`proof/tabHost.ts` and Kira Space's review window. Strip, chips, close, close others/right/all,
preview italics, dblclick promote, middle-click close and Sortable drag reorder come with it. No
change to `packages/workbench`.

## 3. Files

All under `apps/kira-studio/frontend/src/kubernetes/` unless noted.

| File | Change |
|---|---|
| `components/ClusterTree.vue` | `git mv` to `components/ClusterList.vue`; flat list (4.1) |
| `components/clusterReorder.ts` | row selector role-agnostic (4.1) |
| `components/KubernetesPanel.vue` | imports `ClusterList`; search binds `ui.clusterSearch` |
| `components/KubernetesView.vue` | rewritten: split layout (4.5) |
| `components/ResourceBrowser.vue` | kind bar, filter row, table; bottom detail removed (4.3) |
| `components/KindBar.vue` | new: chips plus kinds picker (4.2) |
| `components/ResourceTable.vue` | row click opens tabs, row menu, compact columns (4.4) |
| `components/ResourceTabs.vue` | new: tab bar, strip, lazy-mounted panels (4.6) |
| `components/ResourceDetail.vue` | `git mv` to `components/ResourceTabPanel.vue` (4.7) |
| `components/LogsView.vue`, `DescribeView.vue` | no change expected |
| `state.ts` | ui store reshaped (4.8) |
| `tabs.ts` | new: `useKubeTabsStore` (4.9) |
| `tabHost.ts` | new: `createKubeTabHost` (4.10) |
| `queries.ts` | `useClusterProbes` status only (4.1) |
| `kinds.ts` | new: category order, labels, icons, default kind (4.2) |
| `apps/kira-studio/frontend/src/shortcuts/studioCommands.ts` | kube branch in tab commands (4.11) |

## 4. Design

### 4.1 Flat cluster list (`ClusterList.vue`)

- Rows: clusters only. Drop `CategoryRow`, `KindRow`, `closedCategories`, `toggleRow`, `depthOf`,
  `CATEGORY_LABEL` (moves to `kinds.ts`), `TreeTwisty`, the `tree` option of `useListNavigation`.
- Container `role="listbox"`, `aria-label` the product label, `data-testid="kube-cluster-list"`. Rows
  `role="option"`, `aria-selected` when `row.cluster.id === ui.clusterId`. Keep on each row:
  `data-testid="kube-cluster-row"`, `data-kind="cluster"`, `data-cluster-id`, `data-context`,
  `data-state`, the drop line `kube-drop-indicator`, menu ids `copy-context`, `refresh`,
  `kube-cluster-mcp`, `move-up`, `move-down`. Row class stays `rowVariants({ layout: 'tree', selected
  })` with `rowIndent(0)` (`rowVariants` has no list layout; `tree` is the sidebar row look).
- Empty state testid `kube-tree-empty` becomes `kube-clusters-empty` (consistency spec follows).
- Search (`ui.clusterSearch`, renamed from `treeSearch`) filters by context name only.
- `onSelect` and `onOpen` both call `ui.selectCluster(id)`.
- Probes: `useClusterProbes(ids, active)` keeps statuses only; `active` = `[ui.clusterId]` plus ids
  already probed this session (so dots stay once known). Drop the `kinds` half and `kindsOptions`
  from `useClusterProbes`; `useKinds` (selected cluster) keeps its background helper.
- Reorder: keep `useClusterReorder` (`useTreePointerDrag`). Selector `CLUSTER_ROW` becomes
  `[data-kind="cluster"]`. Sortable (`useSortableReorder`) is now viable (no child rows) but declined:
  no requirement the current engine fails, it draws the drop line `kubernetes-order.spec.ts` asserts,
  and the keyboard and menu path already share `moveCluster`. Update the `clusterReorder.ts` doc
  comment (no category/kind rows any more).
- `moveViaMenu`, `useTreeShortcuts` (`list.moveUp`/`list.moveDown`), `menuFor`, `ClusterMcpDialog`:
  unchanged.

### 4.2 Kind bar (`KindBar.vue`, `kinds.ts`)

`kinds.ts`:

- `CATEGORY_ORDER: KindCategory[]` = workloads, network, config, storage, access, cluster, custom.
- `categoryLabel(c)` (moved from ClusterTree; same keys).
- `kindIcon(k: KubeKind)`: `v1/pods` `package`; else by category (workloads `server-process`,
  network `globe`, config `settings`, storage `database`, access `shield`, cluster `type-hierarchy`,
  custom `extensions`). All names confirmed in the codicon set.
- `POD_KIND = 'v1/pods'`.
- `defaultKind(kinds)`: pods if listed, else the first by `CATEGORY_ORDER` then `compareNatural` on
  `kind`, else undefined.

`KindBar.vue` props `kinds: KubeKind[]`, `value: string` (effective key), `recent: string[]`; emits
`select(key)`:

- shadcn-vue `ToggleGroup` `type="single"`, `size="kira"`, `:model-value="value"`; an
  `update:model-value` to empty (reka deselect) is ignored. Items: Pods (when listed), then `recent`
  in stored order, each listed in `kinds`. Item `data-testid="kube-kind-chip"`, `:data-kind="key"`,
  icon `kindIcon` plus `kind.kind`.
- `PickerPopover` after the chips: `trigger="toolbar"`, `size="kira"`, `icon="list-flat"`,
  `triggerLabel` `kubernetes.kinds.more.label`, `testid="kube-kind-more"`, `:model-value="value"`.
  Options: every kind, `value` key, `label` `kind.kind`, `detail` group/version (`apps/v1`, core
  `v1`), `group` its category, `keywords` `[resource]`, `testid` key with `/` replaced by `_`
  (`apps_v1_deployments`). `groups` = `CATEGORY_ORDER` mapped to `{ id, label: categoryLabel }`.
  Search placeholder `kubernetes.kinds.picker.search.placeholder`; empty text
  `common.state.noMatches.empty`.
- `kinds.partial` true: a muted warning glyph (`warning`, tooltip `kubernetes.kinds.partial.hint`)
  after the picker, `data-testid="kube-kinds-partial"`. Today nothing shows it.

### 4.3 Resources pane (`ResourceBrowser.vue`)

- Props `cluster: KubeCluster`. `kinds = useKinds(cluster.id)`.
- `kind` = `kinds` entry for `ui.kind` if listed, else `defaultKind(kinds)`. Nothing is written to the
  store until the user picks; so the stored choice survives a discovery hiccup.
- Wrapped in `QueryState :query="kinds"`. No kinds at all: `EmptyState` `kube-kinds-empty`, title
  `kubernetes.kinds.none.empty`.
- Row 1 (`ViewToolbar`): `KindBar` (`@select` calls `ui.selectKind(key)`), then `ml-auto` count
  (`kube-count`) and refresh (`kube-list-refresh`).
- Row 2 (`ViewToolbar`, `flex-wrap`): `NamespacePicker` (namespaced kinds), list filter
  `SearchField` `flex-1 min-w-32` (`kube-list-filter`), label selector `Input` `w-40`
  (`kube-label-selector`). `kube-kind-title` is dropped (the active chip names the kind).
- Selector error (`kube-selector-error`), list `QueryState`, `ResourceTable`: as today.
- Remove the bottom detail: `ResizeHandle`, `hasDetail`, `liveHeight`, `detailHeight`, `ResourceDetail`.
- Namespace fallback and `useResourceList` args: unchanged (P484 `background` flag stays in
  `queries.ts`).

### 4.4 Table (`ResourceTable.vue`)

- New props: `clusterId`, `context`, `kind: KubeKind`. `data` as today.
- Click a row: `tabs.open(ref, { preview: true })`. Double-click: `tabs.open(ref, { preview: false })`.
  `useListNavigation` `onOpen` (Enter): permanent open. `onSelect` (arrow keys) moves the roving row
  only; it never opens (arrowing must not fire a describe per row).
- `ref` is `KubeResourceRef` (4.9) built from the props and the row.
- Row selected state: the row equals the active tab's ref (same cluster, kind, namespace, name).
- Right-click: `contextMenu.openContextMenu(e, items)` (keyboard: `nav.contextMenu`, as ClusterList).
  Items: `kube-open` (`common.action.open.label`, permanent tab); for a kind with `logs`,
  `kube-open-logs` (`kubernetes.row.openLogs.label`, icon `output`, permanent tab on the Logs view);
  `copy-name` (`common.action.copyName.label`).
- Compact columns: VueUse `useElementSize` on the table root. Width under 512 px (32 rem): show column
  0 plus the first two of `Ready`, `Status`, `Age` the list has; none of those: column 1. Header and
  cells read one `visibleColumns` index list; sort indexes stay real column indexes. Root gets
  `:data-compact`.
- Testids `kube-table`, `kube-table-head`, `kube-rows`, `kube-row` (`data-name`, `data-namespace`),
  `kube-sort-*`, `kube-list-empty`, `kube-list-no-match`, `kube-truncated`: unchanged.

### 4.5 View (`KubernetesView.vue`)

- Config `missing`/`invalid`: `ModuleStart` as today, whole view.
- Else a row (`flex h-full min-h-0`, `ref="row"`):
  - Resources section `data-testid="kube-resources"`, `v-show="!listHidden || tabs.count === 0"`.
    Width: `flex-1` while no tab; else `flex-none` at `pane.size` px.
    - No cluster selected: `ModuleStart` `kube-start` (title `kubernetes.start.title`, body
      `kubernetes.start.body`, new text 7).
    - Else `QueryState` on `useClusterStatus`: `ClusterUnavailable`, else `ResourceBrowser`.
  - `ResizeHandle` (`axis="x"`, `line`, label `kubernetes.tabs.resize.label`, `min` 280, `max`
    row width minus 360 via VueUse `useElementSize`, `v-bind="pane.bind"`) while tabs exist and the
    list shows. `pane = useLiveSize(() => ui.listWidth, (w) => (ui.listWidth = w))` (AdeReviewWindow
    precedent).
  - `ResourceTabs` (`flex-1 min-w-80`) while `tabs.count > 0`.
- `kube-browser` testid moves to the resources section's inner `ResourceBrowser` root, unchanged name.
- Cluster pruning: `watch` the clusters data; `tabs.dropClusters(ids not in the list)` once data is
  loaded (a context removed from the kubeconfig takes its tabs).

### 4.6 Tab pane (`ResourceTabs.vue`)

- Bar (`h-tabbar shrink-0 border-b border-border bg-chrome flex items-center`):
  `TooltipIconButton` `kube-list-toggle` (icon `layout-sidebar-left` / `layout-sidebar-left-off`,
  label `kubernetes.tabs.hideList.label` / `kubernetes.tabs.showList.label`, `pressed` when hidden),
  then `<TabStrip :host="host" class="min-w-0 flex-1" />` with `host = createKubeTabHost(tabs)` built
  once in setup.
- Panels: one `ResourceTabPanel` per tab in `mounted` (ids of tabs activated at least once, in tab
  order), `v-show` on the active one, `:key="tab.id"`. A tab mounts on first activation and stays
  mounted until closed, so a pod's log buffer and stream survive tab switches. Closing removes it from
  `tabs`, so its panel unmounts and `LogsView` closes its stream (`LogsClose`, op row ends).
- `section` `aria-label` `kubernetes.tabs.aria`, `data-testid="kube-tabs"`.

### 4.7 Tab panel (`ResourceTabPanel.vue`, from `ResourceDetail.vue`)

- Props `tab: KubeTab`. Reads cluster, kind, namespace, name from `tab.state.ref`.
- Root `data-testid="kube-detail"`, `:data-tab-id`, `:data-active` (tests scope to the active panel).
- Header (`h-bar`, `border-b`): muted breadcrumb `context › namespace ›` (namespace part only when
  set), then `kindIcon` plus name (`kube-detail-title`, `font-medium`, `truncate`), then
  `SecondaryTabs` Describe / Logs (`kube-detail-tab-describe`, `kube-detail-tab-logs`; Logs only when
  `kind.logs`). `kube-detail-close` and Escape-to-close are removed (the tab closes it).
- View: `tab.state.view`; `update` calls `tabs.setView(tab.id, v)`, which also promotes a preview tab.
- `useDescribe` and `LogsView` wiring as today (`LogsView` needs `describe.data.containers`).

### 4.8 UI store (`state.ts`, `useKubernetesUiStore`)

Keeps one concern: the browser's own selection and layout.

- Persisted `kira.kubernetes.selection`: `{ clusterId, perCluster: { [id]: { kind, namespace,
  recent } } }`. Schema adds `recent: z.array(z.string()).max(3).default([])`, so values stored before
  P486 still parse (without `.default` they fail and reset the selection).
- `selectCluster(id)`: sets `clusterId`; clears `labelSelector`, `listFilter` when it changed. No
  expansion.
- `selectKind(key)`: remembers `kind`; when `key !== POD_KIND` and not in `recent`, appends it,
  dropping the oldest past 3; clears `labelSelector`, `listFilter` when it changed.
- `setNamespace(ns)`: as today, minus `selected`.
- Persisted `kira.kubernetes.listWidth` (default 440) and `kira.kubernetes.listHidden` (default false).
- Remove: `selected`, `detailTab`, `KubeDetailTab`, `KubeRowRef`, `expanded`, `toggleExpanded`,
  `selectRow`, `detailSize` (`kira.kubernetes.detailHeight` is no longer read). Rename `treeSearch` to
  `clusterSearch`.

### 4.9 Tabs store (`tabs.ts`, `useKubeTabsStore`)

Pinia setup store, one concern: open resource tabs of this window. In memory only (2).

```ts
export interface KubeResourceRef {
  clusterId: string;
  context: string;   // shown in the breadcrumb and the collision title
  kind: KubeKind;    // snapshot at open: the tab never needs the Kinds query
  namespace: string; // '' for a cluster-scoped kind
  name: string;
}
export type KubeTabView = 'describe' | 'logs';
export interface KubeTab extends TabLike {
  kind: 'resource';
  state: { ref: KubeResourceRef; view: KubeTabView };
}
```

- State: `list: ref<Omit<KubeTab, 'active'>[]>`, `activeId: ref<string | null>`,
  `previewId: ref<string | null>`. `tabs` computed adds `active`.
- Identity: `refKey(r)` = `clusterId|kind.key|namespace|name`; tab id `kube-tab:${crypto.randomUUID()}`.
- `open(r, { preview, view? })`:
  1. A tab with the same `refKey` exists: activate it; `preview: false` promotes it; `view` sets it.
  2. Else `preview: true` and a preview tab exists: replace it in place (same index), keep it preview.
  3. Else append after the active tab (end when none); preview per arg.
  Default view `describe`; `open-logs` passes `'logs'`.
- `activate(id)`, `promote(id)`, `setView(id, view)` (promotes), `close(id)` (activates the right
  neighbour, else the left, else none; clears `previewId` when it was the preview), `closeOthers(id)`,
  `closeToTheRight(id)`, `closeAll()`, `move(fromId, toId)`, `dropClusters(ids)`,
  `activateNext()`, `activatePrev()` (wrap), `closeActive()`.
- `titleOf(tab)`: `name`; when another open tab has the same name, `name · context/namespace`
  (`· context` for cluster-scoped).
- `count` computed.
- No unit test (CLAUDE.md bar): each action is a short splice; the UI spec covers open, replace,
  promote, close and neighbour activation.

### 4.10 Tab host (`tabHost.ts`)

`createKubeTabHost(store): TabStripHost<'kubernetes', KubeTab>`, `proof/tabHost.ts` shape:

- `activeWorkspace: computed(() => 'kubernetes')`.
- `tabs`: getter `activeIdByWorkspace` `{ kubernetes: store.activeId }`; `tabsForWorkspace` returns
  `store.tabs`; `activateTab`, `isPreview`, `promoteTab`, `closeTab`, `closeOthers`,
  `closeToTheRight`, `closeAll`, `moveTab` map to the store. No `duplicateTab`: identity is one tab per
  resource, so a duplicate would be a second stream of the same pod.
- `kinds.resource`: `title: store.titleOf`, `menuExtras: () => []`.
- `iconFor`: `{ codicon: kindIcon(tab.state.ref.kind) }`. `railColorFor`: `undefined`.

### 4.11 Global tab commands (`shortcuts/studioCommands.ts`)

`tab-next`, `tab-prev`, `tab-close` run on the shell's tab store, which holds no tab in Kubernetes
mode. Each gets one branch first: `if (useModeStore().active === 'kubernetes')` call
`useKubeTabsStore().activateNext()` / `activatePrev()` / `closeActive()` and return. `tabs.ts` imports
only Pinia, Vue and `wire.ts` types, so the import does not pull the lazy module chunk into the main
bundle (implementer checks the build's chunk list once).

## 5. Library rules

- Tailwind utilities only; no scoped `<style>` in any touched component.
- shadcn-vue: `ToggleGroup`/`ToggleGroupItem` (kind chips), `Popover` via `PickerPopover` (kinds
  picker), `Button` via `TooltipIconButton`, `ResizeHandle` (resizable).
- VueUse: `useElementSize` (compact table, resize max), `tryOnScopeDispose` (existing); `useLiveSize`
  is theme's thin wrapper.
- Pinia: `useKubernetesUiStore` (selection, layout) and new `useKubeTabsStore` (tabs). Two concerns,
  two stores.
- TanStack Query: all server state stays in `queries.ts`; no new query, no hand-rolled cache.
- Workbench `TabStrip` (Sortable drag inside) via its `host` seam. No new dependency.

## 6. Tests

No IPC change, so no backend flow: the backend halves of these scenarios are the existing kubeflow
contracts (`kubernetes.json`), which the UI specs replay. Fast checks per commit: typecheck, lint,
build. Run only the specs below; the full suite runs once at chapter end.

### 6.1 Fixture (`tests/ui/support/kubernetesFixture.ts`)

- `openCluster(page, context)`: click the cluster row, wait `data-state="ok"`.
- `openKind(page, context, key)`: `openCluster`; `v1/pods` waits for chip `[data-kind="v1/pods"]
  [data-state="on"]`; another key opens `kube-kind-more` and clicks `kube-kind-more-option-<key with
  _>`.
- `activePanel(page)`: `[data-testid="kube-detail"][data-active="true"]`.
- `tabChip(page, name)`: `[data-testid="tab"]` filtered by title text, scoped to `kube-tabs`.

### 6.2 New spec `tests/ui/kubernetes-tabs.spec.ts` (contract `kubernetes`)

1. `contract: a cluster opens on its pods; the sidebar has no kind rows`: click `kind-dev`: first
   `List` call `kind: 'v1/pods'`, namespace `default`; Pods chip on; 3 rows; sidebar rows equal 4,
   `kube-kind-row` count 0; no tab pane (`kube-tabs` count 0); resources pane full width.
2. `contract: other kinds open from the picker and stay as chips`: open `kube-kind-more`: 8 options
   under category headings; pick Secret: `List` `kind: 'v1/secrets'`, chip added after Pods. Pick
   Deployment, Service, Event: chips Pods plus the last 3 (Secret dropped). `page.reload()`, reopen
   Kubernetes: same chips, Event selected.
3. `contract: click previews, double-click keeps, so several pods stay open`: click `web-1`: one tab,
   `data-preview="true"`, one `Describe` call; click `web-2`: still one tab, now `web-2`; double-click
   `web-2`: `data-preview="false"`; click `db-0`: two tabs (`web-2` kept, `db-0` preview); click the
   `web-2` row again: no third tab, `web-2` active, row selected.
4. `contract: each pod tab keeps its own log stream`: open `web-1` and `web-2` permanent, Logs on
   each: two `LogsOpen` calls, distinct `streamId`s. Emit lines to `web-1`'s stream while `web-2` is
   active; activate `web-1`: lines show; `LogsOpen` still 2. Close `web-1` (`tab-close`): one
   `LogsClose` with its `streamId`; `web-2` active.
5. `contract: Open logs from the row menu`: right-click `db-0`, `menu-item-kube-open-logs`: permanent
   tab, Logs view, one `LogsOpen` with `pod: 'db-0'`. Secrets list: row menu has no Open logs.
6. `the list hides, resizes and compacts beside tabs`: open a tab; `kube-list-toggle` hides
   `kube-resources`, again shows it; reload keeps the state. Init script sets
   `kira.kubernetes.listWidth` to 300: `kube-table` `data-compact="true"`, head shows Name, Ready,
   Status only.
7. `tabs outlive a cluster switch; a removed context drops its tabs`: open `web-1`; select
   `plugin-ctx`: unavailable state on the left, `web-1` tab still on the right. Emit
   `kira:kubernetes:clusters` with `Clusters` now answering without `kind-dev` (`clustersIn`): tab
   pane gone.
8. `same name in two namespaces gets a qualified title`: `List` override (all namespaces) returns
   `web-1` in `default` and `staging`; open both: titles `web-1 · kind-dev/default` and
   `web-1 · kind-dev/staging`.
9. `the close tab command closes the active Kubernetes tab`: open two tabs; run `tab-close` the way
   `tabs.spec.ts` triggers it (implementer checks; the command palette entry if the chord cannot reach
   the page): one tab left.

### 6.3 Existing specs, same commit as the change that breaks them

- `kubernetes-module.spec.ts`: test 1 (kind rows become the Pods chip plus 8 picker options);
  describe test (`kube-detail-close` becomes closing the tab via `tab-close`; Secret via `openKind`
  picker; panel locators via `activePanel`); logs test (`activePanel`). Names keep `contract:`.
- `kubernetes-ops.spec.ts`: test 2 scopes `kube-detail-tab-logs` to `activePanel`; otherwise
  unchanged. Rows still record (backend unchanged).
- `kubernetes-order.spec.ts`: test 2 becomes `dropping outside a cluster row, or pressing Escape,
  reorders nothing`: drag `down` below the last row into the list's empty area (no indicator), then
  the Escape half as today. Others unchanged (`tree-search` testid stays in `KubernetesPanel`).
- `kubernetes-mcp.spec.ts`: unchanged.
- `consistency.spec.ts`: `kube-tree-empty` becomes `kube-clusters-empty` (Kubernetes test only).
- `copy-coverage.spec.ts` Kubernetes test: `KUBE_DATA` adds the Kinds contract kind names (`Pod`,
  `Deployment`, `Service`, `Secret`, `Event`, `Namespace`, and the rest the contract lists) and
  group/versions (`apps/v1`, `v1`); `kube-detail` scoped via `activePanel`; add a pass over
  `kube-tabs` and an open `kube-kind-more-content`.

## 7. i18n (`packages/theme/src/i18n/en.json`, `kubernetes.*` only)

Add:

- `kubernetes.kinds.more.label` "More kinds"
- `kubernetes.kinds.picker.search.placeholder` "Find a resource kind"
- `kubernetes.kinds.partial.hint` "Some API groups did not answer; their kinds are missing."
- `kubernetes.kinds.none.empty` "This cluster lists no resource kinds"
- `kubernetes.row.openLogs.label` "Open logs"
- `kubernetes.tabs.aria` "Open resources"
- `kubernetes.tabs.hideList.label` "Hide resource list"
- `kubernetes.tabs.showList.label` "Show resource list"
- `kubernetes.tabs.resize.label` "Resize resource list"

Change `kubernetes.start.body` to "Choose a context in the sidebar." Remove keys left unused:
`kubernetes.detail.close.label`, `kubernetes.detail.resize.label` (implementer greps every removed key
first). Key suffixes follow the catalog's existing convention; run the i18n check.

## 8. Docs

`docs/ARCHITECTURE.md` "Kubernetes module" only:

- "Frontend" bullet: replace "Mode `kubernetes` has no tab strip" and the persisted-state sentence:
  shell tab strip stays off; the module has its own `TabStrip` through `TabStripHost`
  (`kubernetes/tabHost.ts`), in-memory per window (pod names are ephemeral, so no restore);
  `kira.kubernetes.selection` adds recent kinds; `listWidth`, `listHidden` persist.
- New P486 bullet: flat cluster list; pods by default, kind bar (Pods plus 3 recent) and a categorized
  kinds picker; resources pane beside a tab pane; click previews, double-click/Enter/menu keeps; a tab
  mounts on first activation and stays mounted until closed, so each pod's log stream runs per tab;
  Studio's `createTabsStore` declined (persistence, Go kind vocabulary); `tab-next`/`tab-prev`/
  `tab-close` route to the module in Kubernetes mode.
- P485 bullet: the Sortable reason becomes "kept `useTreePointerDrag` after P486 flattened the list:
  same drop line, keyboard and menu path".

## 9. Ordering consequences

- P487 (kubectl) swaps the data source under the same `KubernetesService` methods and wire types. P486
  adds no method, field or contract key and reads data only through `control.ts`/`queries.ts`/`wire.ts`,
  so P487 needs no UI change unless its own plan changes a wire shape; the UI specs replay the
  contract, so P487's regenerated `kubernetes.json` flows through them. If P487 changes `Kinds`
  output (e.g. `kubectl api-resources` order or categories), `defaultKind`/`CATEGORY_ORDER` still
  hold; spec 6.2-2's option count follows the contract.
- P484: no recording change. Opening N pod tabs on Logs makes N `LogsOpen` op rows, each spanning its
  stream; closing a tab ends its row ok. The `isRefetch` helper stays on every query.
- P485: reorder kept on the flat list; selector and test 2 updated in the same commit.
- `kira.kubernetes.detailHeight` stays orphaned in localStorage; harmless, not migrated.

## 10. Commits

1. `feat: Kubernetes flat cluster list and kind bar, pods first` (4.1, 4.2, 4.3, 4.8, `queries.ts`,
   i18n kinds keys; fixture `openCluster`/`openKind`; `kubernetes-order.spec.ts` test 2,
   `kubernetes-module.spec.ts` test 1, `consistency.spec.ts`).
2. `feat: Kubernetes resources open in tabs` (4.4 to 4.7, 4.9, 4.10, rest of i18n; module, ops and
   copy-coverage spec updates).
3. `feat: tab commands act on Kubernetes tabs` (4.11).
4. `test: Kubernetes tabs UI spec` (6.2).
5. `docs: Kubernetes resource tabs` (8; SPEC P486 row Done with Result).

## 11. Stream file ownership

Stream A owns, with no overlap with Stream B (P490, `apps/kira-space`, `createTabsStore.ts`) or
Stream C (ADE, idle):

- `apps/kira-studio/frontend/src/kubernetes/**` (all files in 3, incl. renames and new files)
- `apps/kira-studio/frontend/src/shortcuts/studioCommands.ts` (three branches)
- `apps/kira-studio/tests/ui/kubernetes-tabs.spec.ts` (new), `kubernetes-module.spec.ts`,
  `kubernetes-ops.spec.ts`, `kubernetes-order.spec.ts`, `support/kubernetesFixture.ts`
- `apps/kira-studio/tests/ui/consistency.spec.ts`, `copy-coverage.spec.ts`: Kubernetes tests only

Not touched: any Go file, `tests/contract/*`, `packages/workbench/**` (`TabStrip.vue`, `host.ts`,
`createTabsStore.ts` used as they are), `apps/kira-space/**`.

Shared files and conflict rule:

- `docs/v0.2.29/SPEC.md`: Stream A edits only the P486 row (Status, Result). A conflict is hunks in
  different rows; keep both, never reorder or renumber.
- `docs/ARCHITECTURE.md`: Stream A edits only "Kubernetes module" bullets. Stream B edits the P332
  section, Stream C ADE bullets. Keep both sides.
- `packages/theme/src/i18n/en.json`: Stream A adds, changes and removes only `kubernetes.*` keys
  (7). Stream B adds `backend.space.openedFiles.window.title` and `space.openedFiles.*`. A conflict
  merges as union in the file's nesting; rerun the i18n check.
- Contracts: none written by P486. A regenerated contract is never hand-merged: rerun its flow.
- If P486 finds it must edit a `packages/workbench` file after all, stop and report: Stream B owns
  `createTabsStore.ts` and the strip is shared by both apps.

Landing: rebase onto `chapter/v0.2.29`; a real conflict outside the shared files above means the
boundary was wrong. Stop and report rather than resolving by guess.
