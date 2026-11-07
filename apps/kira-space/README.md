# Kira Space

[![PR](https://github.com/vlad-cirstean/kira-studio/actions/workflows/pr.yml/badge.svg)](https://github.com/vlad-cirstean/kira-studio/actions/workflows/pr.yml)

Native macOS workbench: git client, code workspace, coding agents, memory store, terminal, phone
view, plus the database (Studio), Api, Docker and Kubernetes modules. Wails (Go) and Vue 3. Modes:
Agents, Automations, Memory, Git, Studio, Api, Docker, Kubernetes. Replaces the former Kira Studio
app.

[Presentation page](https://vlad-cirstean.github.io/kira-studio/) and
[screenshots](../../README.md#screenshots). Root [`README.md`](../../README.md) covers the Studio,
Api, Docker and Database MCP features.

## Status

- Beta. P537 merged Kira Studio into this app. Studio code lives in `frontend/src/studio` and the
  `studio*` Go packages.
- macOS 26+, Apple Silicon (`arm64`) only. Dark mode only.
- Packaged build is ad-hoc signed, not notarized. First launch after an install may show one
  keychain prompt; **Always Allow** answers it for that build.
- One data home, `~/.kira-space/`.

## Code workspace features

- **Repository import** — import a git repository; it opens as its own independent workspace,
  with its own isolated tab set.
- **Project tree and file viewer** — every tracked/untracked-but-not-ignored file, opened
  read-only into Monaco; refreshes on workspace open and on an explicit Refresh, not live.
- **Diff tabs** — worktree-vs-HEAD diffs in Monaco's own diff editor.
- **Search** — in-file via Monaco's own find widget; repository-wide in Go, with streamed results
  and no `ripgrep` subprocess.
- **Git graph, natively** — the `packages/git-ui` graph, mounted
  as each workspace's pinned first tab, with the code-review layer (inline AI-review gutter icons,
  PR-review threads) alongside it.
- **Inline git blame** — a per-line annotation over the file viewer, toggled by an Appearance
  setting. The status bar also shows the cursor line's author, relative date and commit summary
  regardless of that setting, and clicking it reveals the commit in the graph tab. A file tab
  pinned to a historical revision shows no blame at all — blame only ever reflects the working
  tree.
- **Markdown reading view** — a rendered-Markdown toggle beside the raw view, per tab.

## Agents, memory, terminal and phone

- **Agents** — a task planner with workflows, runs and interactive agent sessions. Claude Code or
  OpenCode, picked per launch, per step or in Settings. One shared context and settings file for every
  agent (`~/.kira-space/config/agent/`), secrets sealed in the keychain layer. Import and export of each agent's
  user config. Hermetic launches on by default: agents skip your user config. Kira writes your agent
  config only on Export.
  A task picks its base branch (the repo's main, any branch, or another planner branch to stack on); a
  headless run rebases it when the base changes. **Review code** opens the per-branch review in one step
  (Cmd/Ctrl+Shift+R). A workflow step declares its results and routes each to the next step, an earlier
  step (a bounded loop), the end of the stage or a stop; a graph editor draws and edits the routes.
  A desktop notification fires when a session finishes or needs input, or a headless run ends. The status
  bar shows Claude Code's 5-hour and weekly usage for the sessions Kira starts.
- **Memory** — an MCP server for Claude Code with keyword and semantic search. Semantic search needs
  an optional local model of about 35 MB, downloaded when you click the button in Settings > Memory.
  Add memories by hand, or import files or folders in bulk.
- **Runs** — scripts grouped in collections, each run in its own folder with a status and result,
  plus terminals. A smart script is a prompt run by headless Claude Code with typed parameters and a tool
  allowlist; run it on a task or branch from Agents (in the task worktree if you choose), as a workflow
  step, or on a cron schedule that asks you to confirm each due run. Secret params keep saved values.
  Smart scripts run on Claude Code.
- **Phone view** — Settings > Mobile access serves the Agents board to a phone. It uses plain HTTP and
  is meant for a trusted home network only: you pair by QR code and approve the code on the desktop,
  device tokens expire after 30 days, and traffic is not encrypted.

## Git features

The git backend runs inside this app — spawn discipline, porcelain parsing, the paged log walk,
pre-flight hazard analysis, every write — and serves this window's own native workspace (a pinned
native graph tab and a native code-review layer, both described under Code workspace above).

The native surface performs fetch/pull/push/force-push, undo, restack, stash and worktree
operations, with a native credential prompt for HTTPS remotes. Conflicts are resolved in your own
external editor, then Continue, Skip or Abort.

- **Commit graph** — a virtualized, lane-drawn log over the whole ref set, paged and streamed from
  the backend as binary FlatBuffers chunks; per-lane ref badges, a checked-out-HEAD indicator, and
  in-graph search.
- **Commit detail and diffs** — file tree, per-file and whole-commit diffs, blob reads, and a
  line-mapped **Go to file** that works on historical content not checked out on disk. Opening a
  commit's changes opens **every** changed file as one replaceable preview cohort rather than a
  dozen permanent tabs — a cohort tab promotes to permanent on the same double-click convention the
  file tree already uses.
- **Refs, checkout and history rewriting** — branches and tags (create/rename/delete, local and
  remote), checkout, revert, reset in all three modes, cherry-pick, plus an in-progress banner with
  Continue/Abort/Skip for a merge, rebase or cherry-pick left mid-flight.
- **Pre-flight, computed in Go** — every hazardous operation is classified server-side before it
  runs (uncommitted work, a detached `HEAD`, a protected branch, a conflicting pop) and the verdict
  crosses as data, so the editor never re-derives it. A single-slot **undo** covers the last
  undoable operation per repository, labelled with the window that ran it.
- **Remote operations** — fetch, push, force-with-lease, a decomposed pull with a strategy picker,
  background auto-fetch, and real cancellation. A credential prompt opens in this app's
  window, through an askpass broker that fails rather than hangs.
- **Stash** — the full stack (push/apply/pop/drop/branch-from-stash) with `merge-tree`-based pop
  prediction, plus branch-scoped extras: auto-stash on checkout tagged with the branch it came
  from, cross-branch apply, and a reusable global stash kept under this app's own `refs/kira/*`
  namespace, which never appears in your graph.
- **Branch review** — a base resolver and a ranged walk, with **incremental review state**: what
  you last reviewed is kept per file as a compressed content snapshot, not just a commit sha, so a
  rebase, squash or amend still diffs correctly. The file tree's mark-reviewed control is a real checkbox: a partially-reviewed file shows
  an indeterminate state, and a fully-reviewed file's row tint clears. A flat list of file/line
  **AI review comments** exports as plain text to paste into a chat — deliberately a copy-paste
  workflow, not a live integration.
- **Search** — a cancellable server-side `git log` tail scan paired with a client-side scan of
  already-loaded rows. Go's RE2 and JavaScript's `RegExp` are reconciled explicitly rather than
  approximated: a literal query runs no regex engine at all, a regex query is translated construct
  by construct, and what RE2 genuinely cannot express (lookaround, backreferences) is refused as a
  named result rather than silently mismatched.
- **Worktrees and stacked branches** — `git worktree` create/list/switch/remove with an optional
  per-repository prepare script you approve once, and stacked branches with restacking and stack
  navigation. **Create worktree** is also offered from the row context menu on a branch, a remote
  branch and a commit row, seeded from that row — the same create machinery the worktree list
  already used.
- **One tabbed picker for refs and working state** — branches, tags, stashes, worktrees and stacks
  fold into five tabs behind one filter box, with a live per-tab match count (so a query typed on
  one tab still hints a match on another), HEAD and any branch checked out in another worktree
  pinned to the top, the rest ranked by recency, a per-list "Show more" step, and
  arrow-key/Home/End/Enter roaming from the filter box down into the rows.
- **Code host change request links** (pull or merge requests) — resolved **per commit**, not per
  branch tip, so the indicator shows on a commit in the middle of a branch's history or in a
  detached `HEAD`, not only on a checked-out tip. A provider-neutral host layer picks the provider
  per repository (setting `Show status from`: detect from remote, off, or a provider). GitHub is
  the one provider shipped. Its authentication is delegated entirely to the `gh` CLI already on
  your machine: this app never holds a token, and never reads `gh`'s own stored credential. A
  commit's change request renders **inline in the detail panel** with its open/closed/merged
  state, and the link opens in the OS browser — never an embedded webview and never a real
  `<a href>`. **GitHub Enterprise hosts work** too: the host check accepts `github.com` plus any
  host the `gh` CLI has itself authenticated against.
Two limits worth knowing up front:

- **Git 2.38 or newer is required** — `git merge-tree --write-tree`, which conflict prediction
  needs. Below that the Git module shows a blocked state rather than degrading silently.

## Requirements

- macOS 26 or later, Apple Silicon (`arm64`).
- [Git](https://git-scm.com) 2.38 or newer on `PATH`.
- Optional: authenticated `claude` CLI for Agents, Memory and smart scripts; `opencode` for Agents.
- Optional: [GitHub CLI](https://cli.github.com) (`gh`), logged in, for PR links. Without it the
  PR indicator stays blank.
- Optional: Docker-compatible daemon for the Docker module and database tests.
- Toolchain (Go, Bun, Wails v3 CLI, Xcode command-line tools): see root
  [`README.md`](../../README.md) Requirements. One `bun install` or `bun run setup` covers
  everything.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/vlad-cirstean/kira-studio/main/scripts/install.sh | sh
```

Downloads the latest release's ad-hoc signed disk image, moves the app into `/Applications`, opens
it with no Gatekeeper prompt. Moves an installed Kira Studio to the Trash; first launch folds the old Studio home into `~/.kira-space/`.
The app checks for newer releases and offers to install.

Build from source:

```sh
git clone <repo-url>
cd kira-studio
bun run package
```

Output: `apps/kira-space/bin/Kira Space.app` and `apps/kira-space/bin/Kira Space.dmg`. A local build
skips the installer, so clear quarantine on first launch (right-click Open, or):

```sh
xattr -dr com.apple.quarantine "apps/kira-space/bin/Kira Space.app"
```

See [`docs/PACKAGING.md`](../../docs/PACKAGING.md) for bundle layout and the verification checklist.

## Development

```sh
bun run dev   # from the repo root: installs deps, then `wails3 task dev`
```

Run from the repo root. Shared scripts (`setup`, `lint`, `typecheck`, `test:go`) are in the root
[`README.md`](../../README.md) Development section.

| Script | What it does |
|---|---|
| `bun run dev` | `cd apps/kira-space && wails3 task dev`: native window, HMR |
| `bun run build` | Production Vue builds into `frontend/dist`, plus phone bundle `dist-mobile` |
| `bun run build:mobile` | Phone bundle only |
| `bun run typecheck:web` | `frontend/src`, `.vue` files included (`vue-tsc`) |
| `bun run typecheck:tests` | `tests/` tiers and Playwright configs (`tsgo`) |
| `bun run typecheck:unit` | `tests/unit` (`tsgo`) |
| `bun run typecheck:git` | `packages/git-ipc`, `packages/git-core` (`tsgo`), `packages/git-ui` (`vue-tsc`) |
| `bun run test:ui` | Build, then Playwright (WebKit): `ui`, `ui-clipboard` projects |
| `bun run test:ui:timing` | Wall-clock specs (`ui-timing`) |
| `bun run test:ui:mobile` | Phone bundle on an iOS (WebKit) profile, plus one Chromium project |
| `bun run test:e2e-real` | Playwright against a real `-tags server` build |
| `bun run test:flows` | Go flow tests on a temp home (`test:flows:complete` adds slow permutations) |
| `bun run test:claude` | Opt-in: real `claude` CLI, spends tokens |
| `bun run test:claude:db` | Opt-in: DB MCP, Kubernetes, proof and smart-script rows |
| `bun run test:visual` | Pixel-diff baselines |
| `bun run package` | Native Wails bundle and `.dmg`, ad-hoc signed: `apps/kira-space/bin/Kira Space.{app,dmg}` |

`bun run test:unit`, `bun run test:go` and unscoped `bun run typecheck` cover the whole workspace.

**App data:** `kira.db`, `logs/`, `review.db`, connections, Api collections and runs under
`~/.kira-space/` (`KIRA_SPACE_HOME`). Memories in `~/.kira-space/memory/`: `memory.db`
and downloaded `models/`. The home has one lock; two instances on different homes never contend.

## Tests

TypeScript suites under `apps/kira-space/tests/`: `unit/`, `ui/`, `ipc/`, `mobile/`, `e2e-real/`,
`visual/`, opt-in `perf/` and `proto/`, shared `contract/` fixtures. Each tier holds a `studio/`
subdirectory for the Studio module's specs, run as separate `-studio` Playwright projects. Opt-in
real-`claude` Go modules: `claude/` and `claude-studio/`. Go suite under `apps/kira-space/`,
flow tier included (`test:flows`). `packages/db-fixtures/` is the shared DB fixture corpus for the
Studio module.

- `bun run test:unit`: plain TypeScript modules with fakes.
- `bun run test:ui`: Playwright against the built bundle, real WebKit.
- `bun run test:go`: Go suite (`go test ./...` from the repo root). Git packages build real
  repositories under `t.TempDir()` and skip without `git`. Perf probes sit behind
  `KIRA_GIT_PERF=1` and assert nothing.

## Architecture

Native Wails (Go) app: one Go process handles windowing, IPC, the git backend and every database
driver. No sidecar, no second runtime. Vue 3 frontend in the OS WebView (WKWebView).

```
apps/kira-space/internal        the Go app: git backend (gitclient, gitops, gitstore, gitrpc, gitsession, ...), the native code workspace (codeworkspace), agents (ade), the phone server (mobileweb), wiring (appwire), flow tests (flowharness), the IPC bridge
apps/kira-space/frontend/src    the Vue 3 app (bindings + the built bundle live alongside it, both gitignored); Studio modules in src/studio
apps/kira-space/tests/unit      unit suite, no external resource (studio/ holds the Studio specs; same for ui, e2e-real, contract, visual)
apps/kira-space/tests/ui        Playwright against the built bundle, WebKit
apps/kira-space/tests/mobile    Playwright for the phone bundle
apps/kira-space/tests/e2e-real  Playwright against a real server build (only what cannot split at the IPC boundary)
apps/kira-space/tests/contract  JSON fixtures shared by the Go flow tests and the UI specs
apps/kira-space/tests/claude    opt-in tests against the real `claude` CLI (own Go module)
apps/kira-space/tests/visual    pixel-diff baselines
internal             repo-root Go shared by the app's packages: `shell`, `appevent`, `ipcerr`, `startupfail`, `appstorage`, `memory`, `harness`, and more
packages/workbench   the shared workbench shell (TitleBar/StatusBar/MainView/TabStrip), Pinia store factories, the Monaco editor bootstrap, test harnesses
packages/git-ipc     the git contract, RPC/codec/validation, the stream channel, the FlatBuffers schema
packages/git-core    client-side git logic: commit store, lane layout, the client half of search, settings schema
packages/git-ui      the git graph/review UI, hosted by the native Git module
packages/theme       shared design tokens/CSS, plus the shadcn-vue `components/ui/*` sets
```

See [`docs/ARCHITECTURE.md`](../../docs/ARCHITECTURE.md)'s Git module section for the full
current-state breakdown, and [`docs/HISTORY.md`](../../docs/HISTORY.md) for finished chapters.

## Not shipped

See the root [`README.md`](../../README.md) "Not shipped" section.

## Upgrading

The VS Code extension (`vladcirstean.kira-space-vscode`) was removed in v2.2. If you installed it, run
`code --uninstall-extension vladcirstean.kira-space-vscode`.

## License

[MIT](../../LICENSE)
