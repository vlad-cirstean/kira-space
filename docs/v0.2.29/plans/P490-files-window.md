# P490 Kira Space: file open window shows only ad hoc file tabs

Plan for SPEC row P490. Planned against `4f5a24ce` (P492 plan; P489 landed). One sequential
implementer, Stream B. Discovery used `codegraph_explore` over the opened-files path
(`openedfiles.Service`, `deliver`, `OpenedFilesService`, `OpenNewWindowKeyed`, `OpenWindow`), the
ephemeral-window precedents (`HostViewService.openTarget`, `ProofService.OpenWindow`,
`IsHostViewKey`, `IsCaptureKey`, `closeDecision`, `AdeReviewWindow.vue`, `HostViewFrame.vue`), the
boot path (`mountShell`, `App.vue`, `openPendingFiles`), and the tab/workspace model
(`createTabsStore`, `workspaceKeyOf`, `visibleWorkspace`, `createWorkbenchHost`, `TabStrip.vue`,
`MainView.vue`, `openRepoFileTab`, `openLocalFileTab`).

## 1. Goal and invariants

- Every file open (Finder Open With, default-app double-click, terminal Cmd-click) lands in one
  **files window**: title bar, one tab strip, the file view. No mode switcher, no repo sidebar, no
  Operations dock, no status bar, no graph tab, no "+" new-tab button.
- Its tabs are the opened files only: `repo-file` for a file inside an imported repo (blame, edit,
  repo rail colour kept), `local-file` otherwise. Tabs from several repos and loose files share one
  strip.
- A later open while the files window lives reuses it: the window comes to the front, new targets
  become tabs (an already-open file is reactivated, cursor moved). Only when no files window lives
  does an open mint one.
- The files window is ephemeral: never restored on relaunch, row (and its `tabs` rows, `ON DELETE
  CASCADE`) dropped on close, never counted as the last window, never the mobile launch target.
- Unchanged: normal workbench windows (boot, fall-forward, restored tabs), review / host view /
  capture windows, Kira Studio (byte-identical behaviour), `Target` wire shape,
  `OpenedFilesService.Pending/Read/TerminalResolve/TerminalOpen` signatures.

## 2. What exists (current source)

- `apps/kira-space/internal/openedfiles/openedfiles.go:140` `deliver(targets)`: mints a uuid key,
  stores `pending[key]`, calls `OpenWindow(key)` = `shell.OpenNewWindowKeyed` (`appwire.go:522`).
  Result: a full, persistent, restorable workbench window per open.
- `apps/kira-space/frontend/src/main.ts:164` `workbenchWindow` excludes review / host view / capture
  windows; `:183` drains `openPendingFiles()` in every workbench window.
- `state/openedFiles.ts:9` `openTarget`: repo target calls `openRepoWorkspace` (adds repo to the
  switcher, pinned graph tab, `setMode('git')`); local target goes to `GENERAL_WORKSPACE`.
- Tab strip shows `tabsForWorkspace(host.activeWorkspace)`; `activeWorkspace` is
  `visibleWorkspace()` (`workspace.ts:141`): the active repo or `GENERAL_WORKSPACE` in git mode. So
  a repo file and a loose file today live in two workspaces and never share a strip.
- `createTabsStore.ts:380` `openTab` derives the new tab's key as `workspaceId ??
  fallbackWorkspaceKey(kind)`, not through `host.workspaceKeyOf`. Every other store action
  (`setActiveTabId`, `closeTabInternal`, `tabsForWorkspace`, `stepTab`, `closeAll`, `moveTab`,
  `hydrateTabs`) uses `workspaceKeyOf`. Both apps' `workspaceKeyOf` equal the inline formula today.
- Ephemeral precedent: `main.go:167` `Ephemeral` predicate (`IsHostViewKey`, `IsCaptureKey`,
  review rows), `purgeHostViewWindows` / `purgeCaptureWindows` (two identical loops),
  `OnWindowClosing` (`OpenedFiles.Forget` already wired). `HostViewService.openTarget`
  (`hostview.go:276`): focus the live window or create one; `openMu` serializes, `FocusWindow` false
  means stale.
- Chrome precedent: `HostViewFrame.vue` / `AdeReviewHeader.vue` use `@workbench/components/TitleBar.vue`
  (drag region, traffic-light inset). `AdeReviewWindow.vue:120` frames `TabStrip` + `MainView` in a
  bordered section on `bg-chrome`.
- `App.vue:148` picks the window root (`CaptureWindow`, `HostViewWindow`, `AdeReviewWindow`, else the
  workbench); menu handlers `onToggleProjectPanel` / `onToggleOperationsPanel` already skip the
  special windows (shared layout store).
- UI fixture `fixtures.ts`: default ready selector is the status bar; `reviewTest` uses
  `readySelector`. `ControlMockHandle.respond(channel, response)` changes an answer mid-test.

## 3. Backend design

### 3.1 Files window key and reuse (`openedfiles.go`)

- `const WindowKeyPrefix = "files-"`; `func IsWindowKey(key string) bool` (prefix test), same shape
  as `IsHostViewKey` / `proof.IsWindowKey`.
- `const ChannelArrived = "kira:openedFiles:arrived"`: a signal (no payload) that targets wait in
  `Pending` for this window.
- `Service` gains:
  - `FocusWindow func(key string) bool` (raise; false when no live window has the key),
  - `SetWindowTitle func(key, title string)`,
  - `Events interface{ EmitTo(windowKey, name string, data any) }`,
  - `openMu sync.Mutex` (serializes `deliver`), `filesKey string` (live files window, guarded by `mu`).
- `deliver(targets)` (still the one window choice; empty targets or nil `OpenWindow` returns):
  1. `openMu.Lock()` for the whole call.
  2. Under `mu`: if `filesKey != ""`, append targets to `pending[filesKey]`; read `key := filesKey`.
  3. If `key != ""` and `FocusWindow != nil` and `FocusWindow(key)`: `Events.EmitTo(key,
     ChannelArrived, nil)`; return.
  4. Else (no live window, or focus said stale): under `mu`, take back `pending[key]` (if any; it
     already holds this call's targets plus anything undrained) and delete it; mint `key =
     WindowKeyPrefix + uuid`; `pending[key] = taken-back ?? targets`; `filesKey = key`. Release `mu`.
     Then `OpenWindow(key)`, then `SetWindowTitle(key, i18n.T("backend.space.openedFiles.window.title",
     nil))` when set.
- Lock rule (deadlock guard): `FocusWindow`, `OpenWindow`, `SetWindowTitle`, `EmitTo` never run under
  `mu`. `WindowRegistry.Focus` does `InvokeSync` on the main thread, and `Forget` runs from
  `WindowClosing`; holding `mu` across Focus could deadlock. `Forget` takes only `mu`, never `openMu`.
  Comment this on `deliver` in one line.
- `Forget(key)`: as today, plus `if filesKey == key { filesKey = "" }`.
- Window closes between a true `FocusWindow` and the page's drain: those targets are dropped with the
  window (`Forget`). Accepted: the user closed the window at that instant. Stated in the comment.
- Page not yet subscribed when `EmitTo` fires: harmless. Targets wait in `Pending`; the page
  subscribes before its boot drain (4.3).
- `Pending`, `Read`, `Open`, `OpenAt`, `Openable`, `Add`, `Close` keep their signatures. Update the
  package and `Service` / `Open` doc comments ("one new window" becomes "the files window").

### 3.2 Bound surface (`bridge/openedfiles.go`)

```go
type OpenedFilesWindowArgs struct {
	WindowKey string `json:"windowKey"`
}

// IsFilesWindow reports whether windowKey names the files window; read once at boot.
func (s *OpenedFilesService) IsFilesWindow(args OpenedFilesWindowArgs) (bool, error)
```

- Empty `windowKey`: `ipcerr.Required("windowKey")`. Answer: `openedfiles.IsWindowKey(args.WindowKey)`.
- Why a bound call and not a frontend prefix test: every special window learns its kind from Go at
  boot (`captureWindowItem`, `hostViewWindowTarget`, `adeTaskReviewWindowTarget`); the prefix stays a
  backend fact. Why not `Pending`: it drains once, and a webview reload must still boot as a files
  window (its tabs restore from `tabs` rows).
- Bound service count stays 37 (a method on an existing service). Regenerate bindings
  (`wails3 task common:generate:bindings`; gitignored).

### 3.3 Wiring

- `appwire.go`: `w.OpenedFiles.Events = emitter` in `Build` (next to `:300`). `BindShell`:
  `w.OpenedFiles.FocusWindow = focusWindow`, `w.OpenedFiles.SetWindowTitle = setTitle` beside the
  existing `OpenWindow = h.OpenNewWindowKeyed`. The flow harness binds through the same `BindShell`,
  so flows get it.
- `main.go`:
  - `Ephemeral`: add `openedfiles.IsWindowKey(key)`. `OpenNewWindowKeyed` then registers the window
    through `AddEphemeral`; `closeDecision` drops its row on close; `MainKey` and the mobile launch
    target skip it.
  - Startup purge: replace `purgeHostViewWindows` and `purgeCaptureWindows` (identical loops) with one
    `purgeKeyedWindows(repositories, func(key string) bool { return bridge.IsHostViewKey(key) ||
    bridge.IsCaptureKey(key) || openedfiles.IsWindowKey(key) })`, before `Windows.List()` as today.
    One warn line per failure, scope `window`. A third copy of the loop would be duplication.
- `internal/shell` unchanged: `OpenNewWindowKeyed` already creates the row, cascades bounds and
  honours `Ephemeral`.
- i18n: `backend.space.openedFiles.window.title` = "Opened files".

## 4. Frontend design

### 4.1 Window kind store (`state/openedFilesWindow.ts`, new)

Pinia, one concern: whether this webview is the files window.

```ts
export const useOpenedFilesWindowStore = defineStore('openedFilesWindow', () => {
  const active = ref(false);
  return { active };
});
```

Set once in `mountShell` before `hydrateTabs` (4.3); never changes after.

### 4.2 One strip for every opened file (workspace key)

- `state/workspace.ts`: `export const FILES_WORKSPACE = '__files__'` beside `GENERAL_WORKSPACE`
  (never a uuid, same reasoning). `visibleWorkspace()` returns `FILES_WORKSPACE` first when
  `useOpenedFilesWindowStore().active`.
- `state/tabs.ts`:
  - `workspaceKeyOf(tab)`: `FILES_WORKSPACE` in the files window, else `tab.workspaceId ??
    GENERAL_WORKSPACE`. A tab's `workspaceId` keeps its repo id, so `repoIdOfTab`, `RepoFileView`,
    blame, edit and `repoRailColor` keep working; only the store's partition key changes.
  - `fallbackWorkspaceKey`: `FILES_WORKSPACE` in the files window, else `GENERAL_WORKSPACE`.
  - `onHydrated`: returns early in the files window (no `openRepos`: no switcher, no graph shell, no
    `OpenWorkspace` bookkeeping; `CodeWorkspaceService` opens a repo session lazily on read/write).
  - Stores read without an explicit pinia: `main.ts` calls `useX(pinia)` first, which sets the active
    pinia, as `openPendingFiles` already relies on.
- `packages/workbench/src/state/createTabsStore.ts` `openTab`: derive the key the way every other
  action does:

  ```ts
  const workspaceKey =
    workspaceId === null
      ? host.fallbackWorkspaceKey(kind)
      : host.workspaceKeyOf({ kind, workspaceId } as unknown as R);
  ```

  Update the comment above it. Kira Studio's `workspaceKeyOf` returns `workspaceId` whenever one is
  set (`mode.ts:27`), so Studio is unchanged and no Studio file is edited. Kira Space normal windows
  are unchanged for the same reason. `reuseExistingTab` keeps deduping on `workspaceId` (repo
  identity), which is right in both window kinds.
- Result in the files window: one key, so `activeIdByWorkspace`, close others / to the right / all,
  Next/Previous/Close Tab, drag reorder and duplicate work unchanged across repo and loose files.
- `openRepoFileTab` passes `workspaceId` to `removeFromPreviewCohort`; the files window never opens a
  preview tab (`preview: false`), so that path is a no-op there. No change.

### 4.3 Boot (`main.ts`)

- Replace `modeStore.hydrateMode(await control.windowsEnsure())` with one `Promise.all` of
  `control.windowsEnsure()` and `queryClient.query({ queryKey: openedFilesWindowKey(windowKey),
  queryFn: () => control.openedFilesIsFilesWindow({ windowKey }) })` (TanStack, same as the capture /
  host view / review target reads). Set `openedFilesWindowStore.active` before the hydrate
  `Promise.all`, so `hydrateTabs` partitions under `FILES_WORKSPACE`.
- `workbenchWindow = !reviewTarget && !hostViewTarget && !captureItem && !filesWindow`. The
  `ensureWorkspaceShell` loop, the repo fall-forward and `installAgentNotifyFocus` stay gated on it.
- Drain only in the files window, subscribe first:

  ```ts
  if (filesWindow) {
    control.onOpenedFilesArrived(() => void openPendingFiles());
    await openPendingFiles();
  }
  ```

  No unsubscribe: the window is the lifetime (`installAdeSignals` precedent). Two overlapping drains
  are safe: `Pending` drains atomically in Go.
- `openedfiles/queries.ts` (new): `openedFilesWindowKey(windowKey)` (`['openedFiles', 'window',
  windowKey]`). `openedfiles/wire.ts` (new): `CHANNEL_OPENED_FILES_ARRIVED =
  'kira:openedFiles:arrived'`.
- `bridge/index.ts`: `openedFilesIsFilesWindow(args: { windowKey: string }): Promise<boolean>` and
  `onOpenedFilesArrived(cb: () => void): () => void` (`on(CHANNEL_OPENED_FILES_ARRIVED, cb)`), next to
  `openedFilesPending`.

### 4.4 Opening targets (`state/openedFiles.ts`)

`openTarget` runs only in the files window now, so the workbench branches go:

- repo target: `openRepoFileTab(target.repoId, target.relPath, { preview: false, reveal })`. No
  `openRepoWorkspace`, no mode change.
- local target: `openLocalFileTab(target.path, reveal)`. No `activateWorkspace`, no `setMode`.
- Stays private (P489's fixer un-exported it; nothing outside the file needs it). Drop the now-unused
  `useModeStore` / `useWorkspaceStore` imports. Update the header comment: boot drain plus every
  `kira:openedFiles:arrived`; last target ends active.
- Reveal on a reused tab already works (P489 4.5).

### 4.5 Window root (`openedfiles/OpenedFilesWindow.vue`, new)

`<script setup lang="ts">`, Tailwind only, no scoped style. Template:

- `TitleBarBase` (`@workbench/components/TitleBar.vue`): one label `t('space.openedFiles.window.title')`
  (`min-w-0 truncate text-kira-md font-medium`, testid `opened-files-title`). No buttons: settings,
  modes, panels, keep-awake and New Window belong to workbench windows.
- Body as `AdeReviewWindow.vue`'s centre pane: `flex min-h-0 flex-1 bg-chrome px-1.5 pb-0.5`, one
  `section` `flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-kira border border-border
  bg-bg`, `aria-label` = the title.
  - Tab row `h-tabbar min-h-0 shrink-0 overflow-hidden border-b border-border bg-chrome` (testid
    `opened-files-tabs`) with `<TabStrip />` and no `#new-tab` slot, so no "+".
  - `<MainView>` in `min-h-0 flex-1`, `#empty`: `EmptyState` (`@theme/components/EmptyState.vue`)
    with `space.openedFiles.window.empty.title` / `.body` (testid `opened-files-empty`).
- Root testid `opened-files-window`.
- Last tab closed: the window stays open on the empty state. Auto-closing would race a `deliver` that
  already chose this key (focus true, then the page closes itself and drops the new targets).
- Status bar: none. Every Kira Space status item is app-wide (agents, runs, update, metrics) or tied
  to the workbench (blame reveal jumps to the graph tab, absent here); inline blame stays in the
  editor. This also keeps P490 off `StatusBar.vue` and `StatusBarItem.vue` (Stream C).

### 4.6 App root and commands

- `App.vue`: `<OpenedFilesWindow v-else-if="openedFilesWindow.active" />` after `AdeReviewWindow`.
  `onToggleProjectPanel` / `onToggleOperationsPanel` also skip when `openedFilesWindow.active` (the
  layout store is shared across windows). Tab Next/Prev/Close, Find, palette, update check stay.
- `shortcuts/spaceCommands.ts`: in the files window, leave out `open-settings`,
  `toggle-project-panel`, `toggle-operations-panel` (no settings mount, no panels; the toggles would
  move the main window's panels through the shared layout). Read the store inside
  `spacePaletteCommands()`; it is set before mount.
- i18n (`en.json`, `space.openedFiles.*`, new subtree): `window.title` "Opened files",
  `window.empty.title` "No files open", `window.empty.body` "Cmd-click a file path in a terminal, or
  open a file with Kira Space in Finder."

## 5. Library rules

- Pinia: the window-kind flag is a one-concern store (4.1). TanStack Query: the boot read (4.3), as
  the other window-target reads. shadcn-vue / theme: `EmptyState`, `TitleBar`, existing `TabStrip`,
  `MainView`. Tailwind utilities only. VueUse: nothing applies (no DOM listener). No new dependency.
- Window reuse is hand-written against `WindowRegistry.Focus`, like `HostViewService.openTarget` and
  `ProofService.OpenWindow`; no library owns Wails window identity.

## 6. Tests

One scenario on both sides of the IPC boundary; same step names.

### 6.1 Backend flow: `flows/windowflow/openedfiles_test.go` `TestOpenedFilesWindow` (renames `TestOpenedFilesNewWindow`)

Window creation and a bound call change, so a flow is required. Setup as today (repo `proj`,
`src/a.go`, `notes/b.json`, `notes/x.txt`, `notes/dir.json`).

1. First open: `Open([a.go, b.json, x.txt, dir.json])`: one keyed window, key has prefix `files-`,
   `WindowMgr.Titles[key]` = "Opened files". `Pending(key)` contract `OpenedFilesService.Pending`
   unchanged (byte-identical file entry).
2. Window kind: `IsFilesWindow(key)` true (contract `OpenedFilesService.IsFilesWindow`, args
   contract masks `windowKey`); `IsFilesWindow("main")` false (contract
   `OpenedFilesService.IsFilesWindow:other`); empty key `E_BAD_REQUEST`.
3. Later open reuses (the old symlink subtest): `Open([link])`: still one keyed window,
   `WindowMgr.Focused` ends with `key`, one `kira:openedFiles:arrived` event to `key`
   (`Events.Since(mark, ...)`, `WindowKey == key`), `Pending(key)` = `[{repoId, relPath: src/a.go}]`
   (contract `OpenedFilesService.Pending:reuse`).
4. Nothing qualifying: `Open([x.txt])`: no window, no focus, no event.
5. Window closed: `WindowMgr.CloseWindow(key)` then `OpenedFiles.Forget(key)` (what `WindowClosing`
   runs): next `Open([b.json])` mints a second `files-` key; `Pending(old)` empty, `Pending(new)` has
   b.json.
6. Stale window (focus false, no `Forget`, e.g. a failed open): drop the live key from
   `WindowMgr.Known` only; `Open([b.json])` mints a new key, and its `Pending` holds the targets
   (none lost); old key's `Pending` empty.
7. Forget before pull (existing subtest): still clears the entry; a following open mints a new key.

`terminallinks_test.go` `TestTerminalFileLinks`: step 3 stays "one new keyed window" only when it is
the first open in that test; step 4 now asserts the same key reused (focus plus arrived event) and
`Pending` returns only the b.json target. Contract `OpenedFilesService.Pending:terminal` unchanged.

Ephemeral and purge live in `main.go` (not reachable from the harness, as for host view); covered by
review, stated here.

### 6.2 UI spec: `tests/ui/opened-files.spec.ts` (rewritten), same scenario

- `fixtures.ts`: export `filesTest` with `readySelector: '[data-testid="opened-files-window"]'` (a
  files window has no status bar), like `reviewTest`.
- `ipcChannels.ts`: `openedFilesIsFilesWindow: 'kira:openedFiles:isFilesWindow'`,
  `openedFilesArrived: 'kira:openedFiles:arrived'`. `mockRuntime.ts`: FQN
  `OpenedFilesService.IsFilesWindow`, default `'false'` (every other spec boots a workbench window).
- Boot mocks: `openedFilesIsFilesWindow` answers the contract; `openedFilesPending` answers the
  contract `Pending` targets; repo read stubs and `openedFilesRead` as today's spec and
  `terminal-file-links.spec.ts` test 5 do.

Tests:

1. `contract: a files window shows only its file tabs`: `opened-files-window` visible; tab strip has
   exactly the contract's targets (one `repo-file` a.go, one `local-file` b.json), the last active; no
   `repo-graph` tab; zero `mode-tab`, `project-panel`, `status-bar`, `tab-strip-new`; title shows
   "Opened files".
2. `contract: a later open adds a tab to the same window`: `control.respond(openedFilesPending,
   contract Pending:reuse)` then `emitWailsEvent(IPC.openedFilesArrived)`: the a.go tab is
   reactivated (no third tab, count stays 2). Then respond with `Pending:terminal` (line 2, col 5) and
   emit again: same tab active, cursor at 2:5 (P489 test 5's selection check).
3. `contract: tabs from two repos and a loose file step and close as one strip`: Close Others on
   b.json closes a.go; Close Tab on the last shows `opened-files-empty`; the window stays.
4. `contract: a workbench window never drains opened files`: default mocks (`IsFilesWindow` false):
   no `OpenedFilesService.Pending` call in the log; `status-bar` and `mode-tab` render.

`terminal-file-links.spec.ts` tests 5 and 6 (window places the cursor; local file at its line) move
to `filesTest` with `openedFilesIsFilesWindow` true; assertions unchanged.

No unit test: no logic meets CLAUDE.md's bar (a flag, a key choice, a lock-ordered branch covered by
the flow).

### 6.3 Verification per commit

Fast checks: Go build, vet, lint, typecheck, biome, i18n check. Run only `windowflow` Go tests,
`opened-files`, `terminal-file-links`, `terminal-links`, `repo-file-edit`, `repo-workspace` UI specs
(Kira Space) and Kira Studio's `view-state` and `tabs-save-retries-after-failure` unit specs (shared
`createTabsStore`). Full suite runs once at chapter end.

## 7. Docs

`docs/ARCHITECTURE.md` "External file opens and default app (P332)":

- Coalescer bullet: "into one new window" becomes "into the files window". Add one bullet: the files
  window (P490), key `files-<uuid>` (`openedfiles.IsWindowKey`), ephemeral (never restored, row and
  tabs dropped on close, purged at boot by `purgeKeyedWindows`). `deliver` raises the live one and
  signals `kira:openedFiles:arrived`, else opens one; the page drains `Pending` at boot and on each
  signal. The page learns its kind from `OpenedFilesService.IsFilesWindow`. It shows `TitleBar`, one
  `TabStrip` and `MainView`: no modes, sidebar, dock or status bar. Every tab partitions under
  `FILES_WORKSPACE`; `workspaceId` keeps the repo id.
- Local-file bullet: "in the general workspace" becomes "in the files window".
- P489 bullet: unchanged. Bound service count unchanged.
- Terminal links bullet (`:854`): "into the opened-files window" stays true.

## 8. Commits

1. `feat: opened files reuse one ephemeral files window` (3.1 to 3.3, backend i18n key, bindings).
2. `test: opened files window flow and contract` (6.1).
3. `feat: opened files window shows only file tabs` (4.1 to 4.6).
4. `test: opened files window UI spec` (6.2).
5. `docs: opened files window` (7).
6. `docs: P490 result` (SPEC row Done and Result).

## 9. Stream file ownership

Streams run in parallel worktrees off `chapter/v0.2.29`. Stream A: Kubernetes in `apps/kira-studio`
(P485 to P487). Stream C: ADE, `apps/kira-space/frontend/src/ade/**` (P492 next; reserves
`ade/v2/status/**` and `packages/workbench/src/components/StatusBarItem.vue`).

Owned by Stream B for P490 (no other stream edits these):

- `apps/kira-space/internal/openedfiles/openedfiles.go`
- `apps/kira-space/internal/bridge/openedfiles.go`
- `apps/kira-space/internal/appwire/appwire.go`
- `apps/kira-space/main.go`
- `apps/kira-space/internal/flows/windowflow/openedfiles_test.go`, `terminallinks_test.go`
- `apps/kira-space/tests/contract/opened-files.json`, `terminal-file-links.json`
- `packages/workbench/src/state/createTabsStore.ts` (one expression in `openTab`)
- `apps/kira-space/frontend/src/main.ts`, `App.vue`
- `apps/kira-space/frontend/src/state/tabs.ts`, `workspace.ts`, `openedFiles.ts`,
  `openedFilesWindow.ts` (new)
- `apps/kira-space/frontend/src/openedfiles/**` (new: `OpenedFilesWindow.vue`, `queries.ts`,
  `wire.ts`)
- `apps/kira-space/frontend/src/shortcuts/spaceCommands.ts`
- `apps/kira-space/tests/ui/opened-files.spec.ts`, `terminal-file-links.spec.ts`, `fixtures.ts`

Not touched by P490: `ade/**`, `workbench/StatusBar.vue`, `StatusBarItem.vue`, every Kira Studio
file, `internal/shell/**`, `go.mod`, `package.json`, lockfiles.

Shared files and conflict resolution:

- `docs/v0.2.29/SPEC.md`: Stream B edits only the P490 row (Status, Result). Keep both sides line by
  line; never reorder or renumber.
- `docs/ARCHITECTURE.md`: Stream B edits only the P332 section bullets in 7. Stream A edits
  "Kubernetes module", Stream C its ADE bullets. Different hunks; a conflict keeps both.
- `packages/theme/src/i18n/en.json`: Stream B adds `backend.space.openedFiles.window.title` and the
  new `space.openedFiles.*` subtree only. Stream A adds under `kubernetes.*` if at all; P492 adds none.
  A conflict merges as union in the file's sorted nesting; rerun the i18n check.
- `apps/kira-space/frontend/src/bridge/index.ts`, `tests/ui/support/ipcChannels.ts`,
  `tests/ui/support/mockRuntime.ts`: Stream B adds entries next to `openedFiles*` only. Keep both
  sides' entries.
- Contracts: Stream B writes only `opened-files.json` and `terminal-file-links.json`. A regenerated
  contract is never hand-merged: rerun the owning flow.
- Generated Wails bindings are gitignored: regenerate after rebase.
- `createTabsStore.ts`: shared package, Stream B only per this list. If Stream A's P486 (pod tabs)
  needs it, it rebases onto this one-expression change and keeps both.

No ordering dependency on Stream A or C: no shared symbol, no shared spec, no shared contract.
Landing per CLAUDE.md: rebase onto `chapter/v0.2.29` in any order; a real conflict outside the shared
list means the boundary was wrong: stop and report.
