# P493 code review (round 1)

Base: `697cd65e` (v0.2.28 squash merge; v0.2.28 left no separate review commit on `main`). Diff: `697cd65e..HEAD`, phases P484-P486, P488-P492. P487 cancelled, skipped. Docs-only files and `docs/v0.2.30/` out of scope.

Counts: 1 Medium, 4 Low.

## Findings

### 1. Native tab menu ignores Kubernetes tabs (Medium, correctness)

- Where: `apps/kira-studio/frontend/src/App.vue:103-105` and `:131-133`; `apps/kira-studio/frontend/src/shortcuts/studioCommands.ts:15,152-178`.
- What: P486 routes `tab-next`/`tab-prev`/`tab-close` to `useKubeTabsStore` only inside the palette commands. The Window menu items carry the real accelerators (`appshell/menu.go:63-65`, `tab.next`/`tab.prev`/`tab.close`). They emit `kira:menu:tab*`, and `App.vue` handles those with the shell tab store only. In Kubernetes mode `modeStore.activeTab` is null, so Cmd+W and next/previous-tab keys do nothing to resource tabs.
- Why: the SPEC result says tab commands act on Kubernetes tabs. That holds for the palette only, not for the keys users press. `kubernetes-tabs.spec.ts` only drives the palette (`runPaletteCommand`), so the gap is untested.
- Fix: move the mode branch into one helper set, for example `nextTab()`/`prevTab()`/`closeTab()` in `studioCommands.ts` or `state/tabs.ts`. Call it from the palette commands and from `control.onTabNext`/`onTabPrev`/`onTabClose` in `App.vue`. Extend `kubernetes-tabs.spec.ts` to emit the menu channels (the mock runtime's event push) and assert that the kube tab steps and closes.

### 2. `LogsOpen` blocks forever if the recorded call panics before the open result (Low, correctness)

- Where: `apps/kira-studio/internal/kubernetes/logs.go:243-256`, `:261-272`.
- What: `KubeRecorder` runs `fn` through `Host.RunOp`, and `safeRun` recovers panics. If `runLogStream` panics before it sends on `opened` (inside `openLogStream`/client-go), then `ran` is already true. Nothing ever sends on `opened`, so `LogsOpen` blocks its IPC call forever. The registry entry for `StreamID` also stays: `stopBase` runs, but `s.logs.remove` does not, so a retry with the same id gets "streamId is already open".
- Why: before P484 a panic simply crashed. The new recovery path turns it into a hung bound call plus a leaked registry entry.
- Fix: make the open result exactly-once and unconditional. Add a `sync.Once` send helper, defer it in the goroutine with the final `err`, and remove the stream when it has not reported success. Alternatively, have `runLogStream` defer `if !sent { s.logs.remove(...); opened <- errPanic }`.

### 3. Terminal hover resolution does full repo resolution per path (Low, performance)

- Where: `apps/kira-space/internal/openedfiles/terminal.go:39-50`; `openedfiles.go` `Resolve`.
- What: `Openable` runs on every hovered terminal row with candidates (`fileLinkProvider.ts` → `TerminalResolve`). Each call runs `Repos.List()` (a DB query) and then a full `Resolve` per path. For every candidate path, `Resolve` runs `EvalSymlinks` on every imported repo root. That is up to 20 paths × N repos symlink walks per hover. Only the boolean is used, and repo matching never changes the boolean: a file outside every repo still resolves.
- Why: hover is the hottest path in this feature. The work grows with the repo count, and the result does not depend on it.
- Fix: in `Openable`, call `Resolve(abs, nil)`, or split out the supported/regular-file check that `Resolve` does before repo matching and call only that. Drop the `s.repos()` call there.

### 4. Cluster-change event refetches several times per window, including the window that caused it (Low, performance)

- Where: `apps/kira-studio/frontend/src/kubernetes/queries.ts:32-38`; `apps/kira-studio/internal/kubernetes/service.go:261,346`.
- What: `useClusters()` subscribes to `kira:kubernetes:clusters` once per caller. There are 4 callers: `ClusterList`, `KubernetesView`, `ClusterMcpDialog` and `DatabaseMcpPane`. One `Reorder`/`SetMcp` event therefore fires up to 4 `invalidateQueries` in each window. TanStack cancels the superseded fetches, but each one has already sent a `Clusters` IPC call. `Clusters` calls `store.Ensure` per context, a DB write. The event is broadcast, so the window that reordered also refetches, right after `useReorderClusters.onSuccess` already set the fresh result.
- Why: a single reorder causes redundant IPC calls, kubeconfig reloads and DB writes in every window.
- Fix: subscribe once per window, for example in the module's setup or with a ref-counted `createSharedComposable` (VueUse) around the listener. Skip the window that caused the event: emit with the origin window key and ignore your own, or rely on the mutation result there.

### 5. `openTab` passes a fake record to `workspaceKeyOf` (Low, maintainability)

- Where: `packages/workbench/src/state/createTabsStore.ts:383`.
- What: `host.workspaceKeyOf({ kind, workspaceId } as unknown as R)` builds a two-field object and casts it to a full tab record. Both current hosts read only `kind`/`workspaceId`. Any host that later reads another field (`state`, `path`, `connectionId`) gets `undefined`, and the type system cannot warn about it.
- Why: the cast hides a narrower contract than the type claims.
- Fix: type the host hook as `workspaceKeyOf(tab: Pick<R, 'kind' | 'workspaceId'>): K`. `TabRecordLike` already carries both fields, so both implementations compile unchanged, and the cast goes away.

## Dimensions with nothing further

- Security: no credential reads. Kubernetes stays on client-go (P487 dropped). The Jira change is command text only. Terminal path calls validate length, NUL bytes, count and position, and require an absolute fallback cwd. They only answer a boolean or open supported regular files.
- Rules: every changed `.vue` file is `<script setup lang="ts">` with no `<style>` block. New stores (`kubeTabs`, `openedFilesWindow`) each own one concern. VueUse (`useElementSize`, `tryOnScopeDispose`), TanStack Query and shadcn/theme primitives are used. The new unit spec `terminal-file-paths.spec.ts` covers a parser with interacting rules, which the unit-test rule allows. Other new tests are flow and UI pairs. Comments are concise.
- Backend P485/P490 storage: the `sort_order` migration, dense `ReorderSortOrder` and the purge of `files-` window rows (tabs cascade on `windows` delete) are correct.
