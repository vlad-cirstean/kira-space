# P489 Terminals: Cmd-click opens file paths in Kira Space

Plan for SPEC row P489. Planned against `7ab4cf4e` (P488 plan). One sequential implementer. Runs as
Stream B with P490 after it (8). Discovery used `codegraph_explore` over the terminal renderer
(`getOrCreateTerminal`, `TerminalRendererDeps`, `useTerminalMount`, `TerminalHostView`,
`createTerminalsStore.attachHeadlessTerminal`), the opened-files path (`openedfiles.Service`,
`OpenedFilesService`, `openPendingFiles`, `openRepoFileTab`, `openLocalFileTab`, `requestReveal`) and
the terminal registry (`terminal.Registry`, `Session`, `OpenNewWindowKeyed`).

## 1. Goal and invariants

- In a Kira Space desktop terminal, a file path in the output is a link. Cmd-click (Ctrl-click off
  mac) opens the file in Kira Space. Plain click never opens, same as web links today.
- `path`, `path:line`, `path:line:col` all link; line and col place the cursor.
- Relative paths resolve against the terminal's live cwd (after `cd`), not only its start cwd.
- A path that does not name an existing, openable file is not a link: no underline, no hint, click
  does nothing. A file deleted between hover and click announces an error.
- Open path is the one external opens already use: `openedfiles.Service` mints a keyed window and the
  page drains its targets. P489 adds a terminal entry to it and line/col to `Target`. What that
  window shows is P490's.
- Unchanged: web links and OSC 8 (`terminal-links.spec.ts` passes as is), Kira Studio terminals,
  Docker exec terminals, the phone terminal. None of them gets file links (4.4).

## 2. What exists

- `packages/workbench/src/terminal/terminalRenderer.ts:130` `getOrCreateTerminal`: one xterm per tab,
  `WebLinksAddon` plus `linkHandler` (OSC 8), both into `activate` (`:139`), which opens only
  `http(s)`. `createLinkHint` (`:53`) is the hover hint. Deps come from `TerminalRendererDeps`
  (`:29`); the deps object is app-wide, not per tab.
- Kira Space builds those deps in two places. A Kira Space terminal is created by whichever runs
  first:
  - `apps/kira-space/frontend/src/workbench/automationsModule.ts:47` `host.rendererDeps`: repo
    terminal tabs and ADE `AdeTuiPane.vue`.
  - `packages/workbench/src/state/createTerminalsStore.ts:158` `attachHeadlessTerminal`: ADE sessions
    opened with no view mounted. Options from `apps/kira-space/frontend/src/state/terminals.ts:15`.
- `apps/kira-space/internal/openedfiles/openedfiles.go`: `Resolve(path, repos)` (`:41`) requires an
  existing regular file with a `filetypes` extension, maps into an imported repo (longest root) or
  keeps the absolute path. `Service.Open` (`:122`) resolves a batch, stores targets under a fresh
  key, calls `OpenWindow(key)` (`appwire.go:521`, `OpenNewWindowKeyed`). `Pending` drains once.
- `apps/kira-space/frontend/src/state/openedFiles.ts:9` `openPendingFiles` opens each target:
  `openRepoFileTab(..., {preview: false})` or `openLocalFileTab(path)`. Neither passes a reveal.
  `openRepoFileTab` already takes `reveal: {line, column}` (`repoTabs.ts:26`); `openLocalFileTab`
  (`repoTabs.ts:89`) takes none, and `LocalFileView.vue` reads only persisted `revealLine`.
- `internal/terminal/session.go:59` `Session` keeps its start `cwd`, `cmd` and the pollable `ptmx`.
  Nothing tracks the live cwd. `Registry.get` (`:455`) is private.
- `ptmx_nonblocking.go`: `ptmx.Fd()` forces blocking mode and breaks `Close`; ioctls go through
  `SyscallConn` (`setWinsize` precedent, `:39`).

## 3. Path syntax (frontend parser)

`packages/workbench/src/terminal/filePaths.ts` (new), pure, no xterm import:
`findFilePaths(text: string): FilePathMatch[]`, `FilePathMatch = { start, end, path, line, col }`
(string indices into `text`, `end` exclusive; `line`/`col` 1-based or `null`).

Rules:

- A token is a run of chars other than whitespace and ``' " ` ( ) < > [ ] { } , ; |``.
- Drop trailing `.` `,` `:` `;` `!` `?` from the token (sentence end, `a.go:12:5:` compiler style).
- Suffix `:<line>` or `:<line>:<col>` at token end (digits only) splits off. `0` or more than 7
  digits means no position (the suffix stays part of nothing; the match is the bare path).
- The rest qualifies when all hold:
  - not empty, not only dots, not starting with `-`;
  - no `://` in the token, and the token is not inside a web link (a `scheme://` run earlier in the
    same whitespace-free span); web links stay `WebLinksAddon`'s;
  - contains `/`, or its last segment has an extension (`.` then 1 to 10 `[A-Za-z0-9]`);
  - path at most 1024 chars.
- `~` and `~/...` pass through as written; Go expands them.
- At most 20 matches per line; later ones are dropped.
- Not supported (stated, not half-built): paths with spaces (quoted or escaped), `path(line,col)`
  (tsc), `#L12` anchors, `a/`/`b/` diff prefixes, `file://` URLs, Windows paths.

Hover range covers path plus suffix, so `src/a.go:2:5` underlines whole.

## 4. Frontend design

### 4.1 Link provider (`packages/workbench/src/terminal/fileLinkProvider.ts`, new)

- `createFileLinkProvider(term, tabId, fileLinks, activate)` returns an xterm `ILinkProvider`
  (public API `term.registerLinkProvider`, not proposed API).
- `provideLinks(y, callback)`:
  1. Build the logical line: walk back from row `y` while `isWrapped`, forward while the next row
     `isWrapped`. Text from `translateToString(true)` per row; a string-index to cell map built from
     `getCell(x).getWidth()` per row so wide chars and wrapped rows map back to `{x, y}` ranges
     (same approach as `@xterm/addon-web-links`' link computer). The map itself is the pure
     `toCellRange` in `filePaths.ts` (7.3); this file only feeds it the buffer's widths.
  2. `findFilePaths(text)`. None: `callback(undefined)`, no IPC.
  3. `fileLinks.resolve(tabId, paths)` (one IPC per hovered row). Keep matches whose answer is
     `true`; build `ILink { range, text, activate, hover, leave, decorations: {underline: true,
     pointerCursor: true} }`. Only ranges touching row `y` are returned.
  4. A rejected resolve logs (`logWarn('terminalFileLinks', ...)`) and returns no links.
- `activate(event, text)`: modifier rule as web links (`linkActivation === 'modifier' &&
  !isPrimaryModifier(event)` returns). Then `hint.hide()`, `fileLinks.open(tabId, {path, line,
  col})`.
- Hover hint: existing `createLinkHint`, text = the matched text (path plus suffix), same
  `workbench.terminal.link.tooltip` second line. No new key.

### 4.2 Renderer seam (`terminalRenderer.ts`)

- `TerminalRendererDeps` gains optional `fileLinks?: TerminalFileLinks`:

```ts
export interface FileLinkTarget { path: string; line: number | null; col: number | null }
export interface TerminalFileLinks {
  /** One answer per path: does it name an openable file, against tabId's live cwd. */
  resolve(tabId: string, paths: string[]): Promise<boolean[]>;
  /** Opens one target in Kira Space; never throws (announces a failure). */
  open(tabId: string, target: FileLinkTarget): void;
}
```

- `getOrCreateTerminal`: when `deps.fileLinks` is set, `term.registerLinkProvider(
  createFileLinkProvider(term, tabId, deps.fileLinks, ...))` after `term.open(host)` (needs the hint,
  which exists only after open). Disposed with the terminal (xterm disposes its providers).
- `filePaths.ts` and `fileLinkProvider.ts` are imported only by `terminalRenderer.ts`, so they ride
  its lazy chunk (file header rule: xterm stays out of the boot bundle).
- Types live in `terminalRenderer.ts` beside `TerminalRendererDeps`; `createTerminalsStore.ts`
  imports the type only (`import type`), so no xterm reaches the boot bundle.

### 4.3 Headless terminals (`createTerminalsStore.ts`)

- `TerminalsStoreOptions` gains optional `fileLinks?: TerminalFileLinks`; `attachHeadlessTerminal`
  passes it into the deps it builds (`:162`). Without this an ADE session first created headless
  never gets file links, since `byTabId` keeps the first deps forever.

### 4.4 Kira Space wiring

- `apps/kira-space/frontend/src/state/terminalFileLinks.ts` (new):
  `createTerminalFileLinks(fallbackCwd: (tabId: string) => string): TerminalFileLinks`.
  - `resolve`: `control.openedFilesTerminalResolve({terminalId: tabId, fallbackCwd:
    fallbackCwd(tabId), paths})` returns `openable`.
  - `open`: `control.openedFilesTerminalOpen({terminalId, fallbackCwd, path, line: line ?? 0, col:
    col ?? 0})`; a rejection announces through `announceApp(errorText(err))`
    (`@workbench/behaviour/liveRegion`, `@workbench/bridge/codedError`) and logs. Never throws.
- `state/terminals.ts`: build one instance with `fallbackCwd = (id) =>
  useTerminalsStore().terminalSession(id)?.cwd ?? ''` (read lazily inside the call, so no
  module-init cycle); pass it as the store's `fileLinks` option and export it as `terminalFileLinks`.
- `workbench/automationsModule.ts`: `host.rendererDeps.fileLinks = terminalFileLinks`.
- Not wired: Kira Studio (`apps/kira-studio/frontend/src/state/terminals.ts`, its
  `automationsModule.ts`): Kira Studio cannot open a file in Kira Space. Docker exec
  (`packages/docker-ui/src/context.ts`): paths name files inside a container. Phone
  (`mobile/screens/TerminalScreen.vue`): no window to open into. All omit `fileLinks`, so behaviour
  there is byte-identical.
- `bridge/index.ts`: two control calls next to `openedFilesRead`. `packages/shared/domain/repo.ts`:
  `OpenedFileTarget` gains `line?: number; col?: number` and the two arg/result types. Regenerate
  bindings (`wails3 task common:generate:bindings`; gitignored).

### 4.5 Opened window applies line/col

- `state/openedFiles.ts`: target with `line` passes `reveal: {line, column: col}` (column omitted
  when `col` is absent) to `openRepoFileTab`, and to `openLocalFileTab`. Move the per-target branch
  into one exported `openTarget(target)` so P490 reroutes it in one place.
- `state/repoTabs.ts` `openLocalFileTab(path, reveal?)`: a new tab gets `revealLine` in its state; a
  reused tab gets `patchLocalFileTabState({revealLine})`; both call `requestReveal(id, reveal)`
  (as `openRepoFileTab` does).
- `views/repo/LocalFileView.vue`: on mount `consumeReveal(tab.id)` wins over persisted
  `revealLine`, same block as `RepoFileView.vue:172-195`. Extract that block into one
  `applyMountReveal(editor, pending, restored, revealLine)` in `views/repo/reveal.ts` and call it
  from both views, rather than copy it. Update the "No blame, no reveal requests" header comment.
- Why now: P489 opens a fresh window per click, so reuse never happens yet; P490's window reuse
  (8) then needs no reveal work.

## 5. Backend design

### 5.1 Live cwd (`internal/terminal`, shared by both apps)

- `internal/terminal/cwd.go` (new): `func (s *Session) liveCwd() string`:
  1. Foreground process group of the pty: `ptmx.SyscallConn()` then `Control` with
     `unix.IoctlGetInt(int(fd), unix.TIOCGPGRP)`. Never `ptmx.Fd()` (`ptmx_nonblocking.go`).
  2. That pgid's cwd, else the shell's (`cmd.Process.Pid`), through gopsutil
     `process.NewProcess(pid)` then `Cwd()` (`github.com/shirou/gopsutil/v4`, already in `go.mod`;
     implemented on darwin and linux). Library, not hand-rolled `proc_pidinfo`/`/proc` code.
  3. Else the session's start `cwd`.
- `func (r *Registry) Cwd(id string) (string, bool)`: `false` only when no live session has `id`.
- Foreground first: a running `claude` or `npm` prints paths relative to its own cwd. For a launched
  agent the shell `exec`s it, so both answers agree.
- Kira Studio compiles it and never calls it.

### 5.2 Terminal path resolution (`apps/kira-space/internal/openedfiles/terminal.go`, new)

- `func TerminalPath(raw, cwd, home string) (string, bool)`: `~` and `~/x` expand against `home`;
  an absolute path stays; a relative path joins `cwd` (empty `cwd` gives `false`); result
  `filepath.Clean`. `~user` forms give `false`.
- `func (s *Service) Openable(cwd, home string, paths []string) []bool`: lists repos once, then per
  path `TerminalPath` then `Resolve`; `true` only when both succeed (existing regular file, supported
  extension; directories and unsupported files are `false`).
- `func (s *Service) OpenAt(abs string, line, col int) error`: `Resolve` (error wrapped as
  `ErrNotOpenable`); `t.Line, t.Col = line, col`; `s.deliver([]Target{t})`.
- `Target` gains `Line int \`json:"line,omitempty"\``, `Col int \`json:"col,omitempty"\``. Zero means
  none, so `tests/contract/opened-files.json` stays byte-identical.
- `Service.Open` keeps resolving its batch, then calls the extracted `deliver(targets)` (mint key,
  store, `OpenWindow(key)`). `deliver` is the one place that picks a window: P490's seam.

### 5.3 Bound surface (`apps/kira-space/internal/bridge/openedfiles.go`)

`OpenedFilesService` gains `Cwd func(terminalID string) (string, bool)` (wired in
`appwire.go` to the terminal registry's `Cwd` once the registry exists; a package function field,
not an exported method, so not bound) and two methods:

```go
type OpenedFilesTerminalResolveArgs struct {
	TerminalID  string   `json:"terminalId"`
	FallbackCwd string   `json:"fallbackCwd"`
	Paths       []string `json:"paths"`
}
type OpenedFilesTerminalResolveResult struct {
	Openable []bool `json:"openable"`
}
type OpenedFilesTerminalOpenArgs struct {
	TerminalID  string `json:"terminalId"`
	FallbackCwd string `json:"fallbackCwd"`
	Path        string `json:"path"`
	Line        int    `json:"line"`
	Col         int    `json:"col"`
}
func (s *OpenedFilesService) TerminalResolve(args OpenedFilesTerminalResolveArgs) (OpenedFilesTerminalResolveResult, error)
func (s *OpenedFilesService) TerminalOpen(args OpenedFilesTerminalOpenArgs) error
```

- Validation (`ipcerr.Required` / `ipcerr.Contract`): `terminalId` required; `paths` 1 to 20 (the
  parser cap); each path 1 to 4096 bytes, no NUL; `line`, `col` 0 to 10 000 000; `col > 0` needs
  `line > 0`; `fallbackCwd` empty or absolute.
- cwd: `Cwd(terminalID)` when live, else `FallbackCwd` (an exited terminal's scrollback still links).
  Home from `os.UserHomeDir()`.
- `TerminalOpen` on a path that is not openable returns `ipcerr.T(ipcerr.CodeNotFound,
  "backend.space.openedFiles.notOpenable.body", {"path": args.Path})`, text "Can't open {path}: no
  such file, or Kira Space can't show this type." No window opens.
- Trust: same reasoning as `Service.Read`'s comment. Bound calls come only from this app's webviews,
  which already run shells; resolution is limited to the `filetypes` vocabulary and only reports a
  boolean.
- `OpenedFilesService` header comment: "Open and Add stay off the bound surface" stays true
  (`TerminalOpen` goes through `OpenAt`, not `Open`/`Add`).

### 5.4 Wiring

- `appwire.go:301`: set `Cwd` after the terminal registry is built (`:388` area); the flow harness
  boots through the same `Build`, so flows get it.
- ARCHITECTURE "bound services" count is unchanged (methods on an existing service).

## 6. Library rules

- xterm: `registerLinkProvider` is xterm's own link API. `@xterm/addon-web-links` only matches URLs
  (its regex is fixed to `https?://`), so file paths need a provider. No maintained package detects
  terminal file paths with `:line:col`; VS Code's detector is internal to VS Code. Requirement named:
  the `path:line:col` grammar plus a cwd-relative existence check, which no library offers.
  `filePaths.ts` is that hand-rolled part, kept pure and tested (7.3).
- gopsutil for process cwd (5.1); `golang.org/x/sys/unix` for the ioctl (already used).
- TanStack Query: declined for `resolve`. It runs inside xterm's `provideLinks` callback in a module
  outside any component, renders no loading or error state, and must not cache (files and cwd change
  between hovers). Requirement named: an uncached validation call whose consumer is an xterm
  callback.
- Tailwind, shadcn-vue: no new UI surface; the hint reuses `createLinkHint` and
  `tooltipContentVariants`. VueUse: nothing applies (no DOM listener the provider owns). Pinia: no new
  store; `useTerminalsStore` supplies the fallback cwd.
- No new dependency.

## 7. Tests

One scenario on both sides of the IPC boundary. Same names for the same steps.

### 7.1 Backend flow: `apps/kira-space/internal/flows/windowflow/terminallinks_test.go` (new) `TestTerminalFileLinks`

Setup: import repo `proj` with `src/a.go` (3 lines); `notes/b.json` under `app.Work`; `k.json` under
`app.Home`. Open a real shell terminal `t1`, window `w1`, cwd `repo.Dir`, via `app.W.Terminal.Open`.

1. Resolve paths `["src/a.go", "./src/a.go", "missing.go", "src", "<abs notes/b.json>", "x.txt",
   "~/k.json"]`: `[true, true, false, false, true, false, true]`. Contract
   `args:OpenedFilesService.TerminalResolve` and `OpenedFilesService.TerminalResolve` in
   `tests/contract/terminal-file-links.json`. Args contract masks `terminalId`.
2. Live cwd: write `cd src\r` to `t1`; poll Resolve `["a.go"]` until `[true]` (`testx` eventually
   helper, bounded). Then `["src/a.go"]` is `[false]`.
3. Open in repo: `TerminalOpen {t1, path: "a.go", line: 2, col: 5}`: one new keyed window;
   `Pending(key)` = `[{repoId, relPath: "src/a.go", line: 2, col: 5}]`. Contract
   `args:OpenedFilesService.TerminalOpen` and `OpenedFilesService.Pending:terminal`.
4. Open outside repos: absolute `notes/b.json`, line 1, col 0: `Pending` = `[{path, line: 1}]`
   (no `col` key).
5. Not openable: `TerminalOpen` `missing.go`: `E_NOT_FOUND`, key
   `backend.space.openedFiles.notOpenable.body`, no new window. Contract
   `error:OpenedFilesService.TerminalOpen`.
6. Exited terminal: close `t1`. Resolve `["src/a.go"]` with `fallbackCwd: repo.Dir` is `[true]`;
   with `fallbackCwd: ""` is `[false]`; absolute `b.json` still `[true]`.
7. Validation: empty `terminalId`, 21 paths, relative `fallbackCwd`, `col` without `line`: each
   `E_BAD_REQUEST`, no window.

Existing `TestOpenedFilesNewWindow` passes unchanged (its contract keeps no `line`/`col`).

### 7.2 UI spec: `apps/kira-space/tests/ui/terminal-file-links.spec.ts` (new), same scenario

Setup as `terminal-links.spec.ts` (repo row, `tab-strip-new`, `emitWailsEvent` on `IPC.terminal`).
The printed line is built from the contract's resolve `paths`, with `:2:5` on the first, so the
parser sends exactly the contract args. Mock answers `TerminalResolve` with the contract result.

1. `contract: a file path opens in Kira Space only on Cmd/Ctrl-click, with a hover hint`: hover
   `src/a.go:2:5`: hint shows the text and the click hint; plain click: no `TerminalOpen`; Meta
   click: one `TerminalOpen`, args `toMatchObject` the contract args minus `terminalId`
   (`{path: "src/a.go", line: 2, col: 5}`), `terminalId` equals the active tab id.
2. `contract: a path that is not a file is not a link`: hover `missing.go` and `src`: hint hidden;
   Meta click: no `TerminalOpen`, no `LinkService.OpenExternal`.
3. `contract: a web link on the same line still opens in the browser`: line carries
   `https://example.com/x.go` too: Meta click there calls `LinkService.OpenExternal` once, never
   `TerminalResolve` with a URL fragment in `paths`.
4. `contract: a file gone by click time announces the error`: `TerminalOpen` answers the contract
   error; the app live region shows its message.
5. `contract: the opened window places the cursor`: boot with `openedFilesPending` answering
   `OpenedFilesService.Pending:terminal` (repo target, line 2, col 5) plus the repo read stubs
   (`opened-files` / `repo-workspace.spec.ts` precedents): the repo-file tab is active and its saved
   `revealLine` is 2 (the tab-save control call in the log, as `repo-workspace.spec.ts` reads tab state), and
   Monaco's selection starts at 2:5 (the cursor's `.view-line` index). Same for a local-file target
   with `line: 1`.

`terminal-links.spec.ts` and `opened-files.spec.ts` pass unchanged, in both apps.

### 7.3 Unit: `apps/kira-space/tests/unit/terminal-file-paths.spec.ts` (new)

`findFilePaths` meets CLAUDE.md's bar (a parser with interacting rules). Table cases: bare relative,
`./` `../` `~/`, absolute, `:line`, `:line:col`, trailing `:` and `.`, zero and 8-digit line, URL
excluded, path inside a URL excluded, `-flag` excluded, extensionless word excluded, `Makefile`-style
no-extension with `/` included, wide chars before a path (indices), 21 candidates capped to 20,
quotes and parens around a path. The index-to-cell map from 4.1 step 1 is a pure exported function
in `filePaths.ts` (`toCellRange(rows: {widths: number[]}[], start, end)`, no xterm types); two more
cases cover it: a path wrapped across two rows, and a wide char before a path.

Fast checks per commit: Go build, vet, lint, typecheck, biome. Run only `internal/terminal`,
`apps/kira-space/internal/openedfiles`, `flows/windowflow` Go tests, the new unit spec, and
`terminal-file-links`, `terminal-links`, `opened-files`, `repo-file-edit` UI specs (Kira Space) plus
Kira Studio `terminal-links`. Full suite runs once at chapter end.

## 8. Seam for P490 (same stream, next)

P490: the opened-file window shows no repos or sidebar, only ad hoc file tabs. P489 leaves:

- One window choice: `openedfiles.Service.deliver(targets)`. P490 changes it (for example, reuse a
  live ad hoc window and push the target by event instead of minting a key per click). `OpenAt`,
  `Open` and the bound methods stay as P489 leaves them.
- One page-side open: `state/openedFiles.ts` `openTarget(target)`. P490 reroutes repo targets into
  its ad hoc tab set instead of `openRepoWorkspace` if its plan says so.
- Live reveal already works on a reused local-file or repo-file tab (4.5).
- `tests/contract/terminal-file-links.json` and `OpenedFilesService.Pending:terminal` are inputs
  P490's UI spec can boot an ad hoc window from.
- P489 touches no window chrome, sidebar, workspace store or `main.ts` boot gating; those are P490's.
  Overlap with P490 is only `openedfiles.go`, `bridge/openedfiles.go`, `openedFiles.ts`,
  `repoTabs.ts`, `LocalFileView.vue`, `opened-files.spec.ts`, windowflow tests. Sequential in one
  stream, so no conflict; P490 plans against P489's commits.

## 9. Docs

- `docs/ARCHITECTURE.md` terminal links bullet (`:854`): replace "No file-path links." with: Kira
  Space desktop terminals also link file paths (`filePaths.ts` grammar; `fileLinkProvider.ts` over
  `registerLinkProvider`); each hovered row asks `OpenedFilesService.TerminalResolve` against the
  live cwd (foreground process group, then shell, then start cwd; `fallbackCwd` once exited); only
  openable files link; Cmd/Ctrl-click opens through `TerminalOpen` into the opened-files window.
  Kira Studio, Docker exec and phone terminals have no file links.
- "External file opens and default app (P332)": one bullet. Terminal opens enter at
  `Service.OpenAt`; `Target.line`/`col` place the cursor; `deliver` is the one window choice.
- `docs/DEV_ENVIRONMENT.md`: nothing.

## 10. Commits

1. `feat: terminal live cwd` (5.1).
2. `feat: resolve and open terminal file paths in Kira Space` (5.2, 5.3, 5.4; bindings).
3. `test: terminal file links flow and contract` (7.1).
4. `feat: terminal file path links` (3, 4.1 to 4.4; unit spec 7.3 with it).
5. `feat: opened files place the cursor` (4.5).
6. `test: terminal file links UI spec` (7.2).
7. `docs: terminal file links` (9).

## 11. Stream B file ownership

Stream B runs P489 then P490 in its own worktree off the chapter branch. Stream A is P484 to P487
(Kubernetes, `apps/kira-studio/internal/kubernetes/**`, `apps/kira-studio/frontend/src/kubernetes/**`,
`dbmcp`, `bridge/kubernetes.go`, op log). Stream C is P488 (install text) and P491/P492 (ADE,
`apps/kira-space/frontend/src/ade/**`).

Owned by Stream B (no other stream edits these):

- `internal/terminal/cwd.go` (new), `internal/terminal/session.go`
- `apps/kira-space/internal/openedfiles/**`
- `apps/kira-space/internal/bridge/openedfiles.go`
- `apps/kira-space/internal/appwire/appwire.go`
- `apps/kira-space/internal/flows/windowflow/**`
- `apps/kira-space/tests/contract/opened-files.json`, `terminal-file-links.json` (new)
- `packages/workbench/src/terminal/**` (`terminalRenderer.ts`, `filePaths.ts`, `fileLinkProvider.ts`)
- `packages/workbench/src/state/createTerminalsStore.ts`
- `packages/shared/domain/repo.ts`
- `apps/kira-space/frontend/src/state/terminals.ts`, `state/terminalFileLinks.ts` (new),
  `state/openedFiles.ts`, `state/repoTabs.ts`
- `apps/kira-space/frontend/src/workbench/automationsModule.ts`
- `apps/kira-space/frontend/src/views/repo/LocalFileView.vue`, `RepoFileView.vue`, `reveal.ts`
- `apps/kira-space/tests/ui/terminal-file-links.spec.ts` (new), `opened-files.spec.ts`
- `apps/kira-space/tests/unit/terminal-file-paths.spec.ts` (new)
- P490 additions, planned there: Kira Space window shell, sidebar and workspace files
  (`apps/kira-space/frontend/src/main.ts`, `state/workspace.ts`, the app shell component), and
  `apps/kira-space/main.go` if window wiring moves. P490's plan restates its list; it must stay
  outside `ade/**` and Stream A's paths.

Shared files (edited by more than one stream) and how conflicts resolve:

- `docs/v0.2.29/SPEC.md`: each stream edits only its own rows (Status cell, Result). Rebase
  conflicts keep both sides line by line; never reorder or renumber.
- `packages/theme/src/i18n/en.json`: Stream B adds only
  `backend.space.openedFiles.notOpenable.body`. Other streams add under their own subtrees
  (`kubernetes`, `backend.studio.*`, `space.ade.*`, install text keys). A conflict keeps every key
  from both sides in the file's sorted nesting; rerun the i18n check.
- `apps/kira-space/frontend/src/bridge/index.ts`: Stream B adds two calls next to
  `openedFilesRead`. Stream C adds only if its plan needs a call, in the ADE block. Keep both.
- `apps/kira-space/tests/ui/support/mockRuntime.ts`, `support/ipcChannels.ts`: Stream B adds two
  entries next to `openedFiles*`. Keep both sides' entries.
- `docs/ARCHITECTURE.md`: Stream B edits the terminal links bullet and the P332 section only. Stream A
  edits "Kubernetes module"; Stream C its ADE text. Different hunks; a conflict keeps both.
- Generated Wails bindings are gitignored: regenerate after rebase, never merged.
- `go.mod`/`go.sum`, `package.json`, lockfiles: Stream B adds no dependency. If another stream does,
  take theirs.

No ordering dependency on Stream A or C: no shared symbol, no shared contract file, no shared spec.
Landing per CLAUDE.md: rebase onto `chapter/v0.2.29` in any order; a real conflict outside the shared
list above means the ownership boundary was wrong, so stop and report.
