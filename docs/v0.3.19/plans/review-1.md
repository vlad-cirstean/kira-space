# v0.3.19 code review, round 1

Base: `5022eddb3` (v0.3.18 merge on `origin/main`; prior chapter's review finished before its merge). Head: `1126c7665`. Scope: `git diff 5022eddb3..HEAD` (P690, P691, P692, P693). Reviewer: Opus, one pass over three dimensions.

Counts: 4 important, 0 pre_existing, 5 nit.

## Important

### R1. Removed argv subcommands fall through to a full GUI launch

`apps/kira-space/internal/appwire/wire.go:50` (`RunArgvShim`), `apps/kira-space/main.go:53`.

v0.3.18 wrote two external callers of the binary into user config: the Memory settings pane installed `<exe> memory-mcp` as a stdio MCP server in the agent CLI config (base `bridge/memory.go:405`, `InstallStdio`), and its pre-commit hook line ran `'<bin>/kira-space' memory-check --staged` in user repos (base `tests/contract/memory-settings.json:4`). After upgrade `RunArgvShim` returns `ok=false` for those words, so `main` continues into the full app start.

Failure: app not running, user runs `git commit` in a repo with the hook. The hook starts Kira Space with a window; the commit blocks until the user quits the app. Same for every `claude` session started outside the app: Claude Code spawns `kira-space memory-mcp`, which opens the GUI (stdio pipes attached). With the app already running, the spawn exits 0 at the instance lock, so Claude reports a broken MCP server on every session.

Fix: in `RunArgvShim`, handle the retired words (`memory-mcp`, `memory-ingest`, `memory-embed`, `memory-check`) explicitly: print one stderr line naming the removal and how to drop the hook or MCP entry; exit 0 for `memory-check` (never block a commit), exit 1 for the others. Mark it v0.3.x-only so the v0.4.0 bump drops it (minor-bump rule). Add one argv-shim flow assertion per exit code.

### R2. 5.3 MB build artifact committed

`scripts/docshistory/docshistory` (added in `f0448b11d`, ELF x86-64 with debug info, 5,362,115 bytes).

Failure: binary ships in the PR and, on a non-squash merge, in history forever; it is unreviewable and platform-specific. The same commit removed the `.commentsweep/` ignore block and left a double blank line, so nothing ignores the output.

Fix: `git rm --cached scripts/docshistory/docshistory`, add `scripts/docshistory/docshistory` to `.gitignore`.

### R3. Mobile ring can be nil under a live pump: nil dereference crashes the app

`apps/kira-space/internal/mobileterm/broker.go:141` (`SetRecording`), `broker.go:290-305` (`phoneConn.Snapshot/ReadFrom/End`), `serve.go:138` (`pump`).

`SetRecording(false)` nils `e.ring` for every entry with `e.conn == nil`. `toDesktop` (Release, Reclaim, ReleaseDevice) sets `e.conn = nil` while the `pump` goroutine still holds the `*phoneConn`. While output keeps flowing, `pump` stays in its `continue` path and calls `c.ReadFrom` without passing the `select` on `c.Done()`. `Ring.ReadFrom`/`End` dereference a nil receiver. `pump` runs in its own goroutine (`serve.go`, `go func() { ... b.pump(...) }`), not under net/http's handler recover, so the panic kills the desktop app.

Scenario: busy agent terminal held by a phone; user clicks Reclaim on desktop, then switches phone access off before the phone's socket closes. Window is short, consequence is a crash.

Fix: make the three `phoneConn` accessors nil-safe (empty data, offset 0, no overrun when `c.e.ring == nil`), or have `SetRecording(false)` skip entries whose `gen` still has a pump; nil-safe accessors are the smaller change.

### R4. Archive tag not on origin

`archive/memory-v0.3.18` points at `a08665c6a` locally; `git ls-remote --tags origin 'refs/tags/archive/*'` returns nothing (P690 Result records the HTTP 403). `docs/ARCHITECTURE.md:3112` already tells readers to restore from it.

Failure: PR squash-merges, chapter branch is deleted, `a08665c6a` becomes unreachable on origin; the documented restore path points at nothing.

Fix: push the tag (user action if the proxy keeps refusing) before the PR opens; hold the PR otherwise.

## Nit

### N1. Disable order keeps rings of phone-held terminals

`apps/kira-space/internal/bridge/mobile.go:279-281`, `apps/kira-space/internal/mobileweb/server.go:301`.

`SetEnabled(false)` calls `setRecording(false)` before `embedded.SetRunning(false)`, whose server stop runs `ReleaseAll`. At `SetRecording` time held terminals still have `e.conn != nil`, so their 1 MiB rings survive until the terminal exits, against P693 F3's stated goal. A concurrent `Output` that loaded `recording == true` just before the store can also recreate one ring after the drop.

Fix: call `setRecording(false)` after `SetRunning(false)`, or drop the ring in `toDesktop` when `!b.recording.Load()`.

### N2. Viewer drops a newer text that lands during the first Monaco load

`apps/kira-space/frontend/src/ade/v2/files/AdeFileViewer.vue:51-66`.

Two `found` answers before `loadMonaco()` resolves (Refresh during the first, cold Monaco import) start two `mountEditor` calls. The first creates the editor with the older text; the second returns at `if (... || editor) return` after the await, so the model never gets the newer text.

Fix: after the await, if `editor` exists, `editor.getModel()?.setValue(text)` instead of returning; or create from `viewer` text set last.

### N3. Release can race an in-flight search start and reopen the session

`apps/kira-space/frontend/src/ade/v2/files/AdeFilesTab.vue:88-97`, `apps/kira-space/frontend/src/ade/v2/files/state/fileSearch.ts:181-212`.

Enter then an immediate tab or branch switch: `release` sends cancel and close while `adeTaskFilesSearch` is still in flight. Bound calls run concurrently, so the backend can cancel and close first, then `TaskFilesSearch` reopens the session via `taskSession` and runs the full search for an unmounted tab; the session stays in `taskFiles` until archive or quit. On the client, `startSearch` then writes `searchId` into the dropped state object and leaves a `branchBySearchId` entry until `done`.

Fix: in `startSearch`, after the await, if `byBranch.get(branchId) !== state` (dropped), call `cancelSearch(branchId)` and `adeTaskFilesClose(branchId)` and skip the map insert.

### N4. Every path-validation error reports as "outside the worktree"

`apps/kira-space/internal/ade/taskfiles.go:78-80`, `:107-109`.

Both sites wrap any `pathsafe.ValidateRelPath` error in `ErrBadPath`, including `resolve root` and `resolve parent` I/O errors (worktree deleted on disk after `worktreeOf`). The UI then shows "This path is outside the task worktree."

Fix: wrap only `errors.Is(err, pathsafe.ErrPathEscapesRoot)`; pass other errors through (internal).

### N5. Dangling `op-owners/` in architecture doc

`docs/ARCHITECTURE.md:3620` still lists `op-owners/` among home-root entries. Its only writer (memory-mcp foreign op-log, `appwire/memoryops.go`, `oplog/foreign.go`) is gone.

Fix: drop `op-owners/` from that list.

## Dimensions with no finding

- editorlaunch: argv only, no shell, Windows never returns a `.cmd`, absolute paths only (no leading `-` flag injection), detached with reaped `Wait`, git env scrubbed, no credential read. `-tags server` passes nil launcher and revealer; bridge answers `E_UNSUPPORTED` and `TaskFilesLaunchers` reports none.
- Path safety: worktree root comes from the branch row; `pathsafe.ValidateRelPath` rejects absolute, `..`, symlink escape and dangling symlink; `ReadFileIf` opens regular files only, 8 MiB cap. Viewer is Monaco `readOnly` + `domReadOnly`; no write call exists on the task surface.
- Listing and search limits reuse `MaxListedFiles` (200,000), `MaxSearchMatches`, `MaxMatchesPerFile`; tree filter is debounced.
- Settings legacy keys: struct removal plus `TestSettingsLegacyMemoryKeys` and the UI spec cover load and write-through; `WindowModes` normalizes a stored `memory` mode; `panelTab` reads stored `files` as `repos`; llm-prompts overrides for removed ids are skipped in `rebuild`. All compat sits within the 0.3.18 to 0.3.19 step; no legacy from older minors added.
- Frontend rules: new components are `<script setup lang="ts">`, Tailwind only, shadcn-vue primitives, VueUse (`useElementSize`, `useDebounceFn`), TanStack for listing/read/launchers, one-concern Pinia stores (`adeFilesUi`, `adeFileSearch`; TanStack declined for the push stream with the reason stated). New copy keys present in `en.json`.
- `activateAdeStores`, `gcTime: 0` on candidates and file read, `usePaneResize` plain flag, debug hooks behind `__KIRA_DEBUG_HOOKS__` (defined `false` in mobile and unit builds): no defect found.
- RAM probe flow test is gated on `KIRA_RAM_PROBE=1`.
