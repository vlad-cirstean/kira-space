# Kira Studio

[![PR](https://github.com/vlad-cirstean/kira-studio/actions/workflows/pr.yml/badge.svg)](https://github.com/vlad-cirstean/kira-studio/actions/workflows/pr.yml)

Kira Studio is a native macOS workbench that combines a visual database client for ten engines
(DataGrip/DBeaver class) with an HTTP/gRPC API client (Postman/Insomnia class). It is built on
Wails (Go) and Vue 3, and you switch between its **Studio** (database), **Api**, **Runs** and
**Docker** modes with one button.

This repository also contains a sibling app, **Kira Space** — a native git client and code
workspace with coding agents (Claude Code or OpenCode, ADE), a memory store, a terminal and a phone view on your home
network. This README covers Kira Studio; see
[`apps/kira-space/README.md`](apps/kira-space/README.md) for Kira Space.

See both apps on the presentation page: [Kira Studio](https://vlad-cirstean.github.io/kira-studio/) and [Kira Space](https://vlad-cirstean.github.io/kira-studio/space/).

## Screenshots

### Kira Studio

![Kira Studio data grid showing an orders table with one row selected.](screenshots/framed/studio-data-grid.png)

The data grid: filter, sort, edit and commit rows.

![Kira Studio SQL console with a join query and its result grid.](screenshots/framed/studio-sql-console.png)

The SQL console: run a statement or the whole script.

![Kira Studio Api mode showing an HTTP request and its JSON response.](screenshots/framed/studio-api.png)

Api mode: HTTP and gRPC requests with a JSON response.

### Kira Space

![Kira Space commit graph with branches, tags and the commit detail pane open.](screenshots/framed/space-git-graph.png)

The git graph: branches, tags and commit detail.

![Kira Space agents board with tasks across stages and live runs.](screenshots/framed/space-agents-board.png)

The agents board: tasks across stages, with runs live.

![Kira Space memory list showing each fact with its origin and scope.](screenshots/framed/space-memory.png)

Memory: every fact shows its origin and scope.

## Status

- **Beta.** Expect bugs and breaking changes between builds.
- **macOS 26+ on Apple Silicon (`arm64`) only, dark mode only.**
- Builds are **ad-hoc signed, not notarized**. Because each new build counts as a different app to
  the Keychain, the first launch after an update may ask to use "confidential information stored
  in…" — choose **Always Allow** once for that build.
- Saved credentials, secret script parameters and agent secrets are encrypted at rest, with the key held in the macOS Keychain.

## Install

Run in Terminal. It installs a fresh copy or replaces an older one, then opens the app:

```sh
curl -fsSL https://raw.githubusercontent.com/vlad-cirstean/kira-studio/main/scripts/install.sh | sh -s -- --app=studio
```

The script downloads the latest release's disk image, moves the app into `/Applications` and opens
it, with no Gatekeeper prompt (unlike a DMG downloaded through a browser). Use `--app=space` to
install Kira Space instead. Once installed, the app checks GitHub for a newer release and offers to
update; nothing installs until you click **Update**.

### Building from source

```sh
git clone https://github.com/vlad-cirstean/kira-studio.git
cd kira-studio
bun run package:studio
```

This installs its own dependencies first (the Bun workspace, the Go module and the pinned `wails3`
CLI), so a fresh clone needs nothing else run beforehand. The app lands at
`apps/kira-studio/bin/Kira Studio.app` and its disk image at `apps/kira-studio/bin/Kira Studio.dmg`.

A locally built app has not gone through the installer, so its first launch needs a Gatekeeper
workaround — right-click it and choose Open, or run:

```sh
xattr -dr com.apple.quarantine "apps/kira-studio/bin/Kira Studio.app"
```

See [`docs/PACKAGING.md`](docs/PACKAGING.md) for the bundle layout, releases and the verification
checklist.

## Features

### Supported engines

| Engine | Default view | Query console | Writes |
|---|---|---|---|
| PostgreSQL | Grid | SQL | yes |
| MariaDB | Grid | SQL | yes |
| MySQL (8.0.16+) | Grid | SQL | yes |
| SQLite | Grid | SQL | yes |
| ClickHouse | Grid | SQL | insert only |
| MongoDB | Documents | shell-style | yes |
| Redis | Key/value | Redis commands | string keys (delete on any type) |
| Kafka | Stream | — | produce |
| SQS | Stream | — | send and delete |
| S3 | Object browser | — | yes, plus upload/download |

Stop cancels the operation itself (`pg_cancel_backend`, `KILL QUERY`, cursor abort, consumer stop),
not only its display. SQS never polls on its own: reads happen only when you press Poll, because
`ReceiveMessage` hides messages from real consumers. Pagination, counting, EXPLAIN and cancel
details per engine are in [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) (Per-database mapping and
Per-engine adapter facts).

### Studio (database client)

- **Project panel** — a lazy, cached connection tree with per-connection colors, live status,
  search and hide/show filters.
- **Connections** — fields or URI mode, test-connection, a per-connection **read-only guard**
  enforced in the backend, an optional **pre-connect script** (for example a port-forward),
  import from a JetBrains DataGrip project, and **paste credentials**: paste labelled text (key/value lines, `.env` lines or a `psql`/`mysql`-style
  command) into the dialog, review what was recognised, and apply. **Update credentials** on a saved
  connection changes only its username and password.
- **Tabs** — open the same table any number of times, each with its own paging, sort, filter and
  scroll; session restore reopens tabs without auto-connecting.
- **Data grid** — virtualized in both directions, page sizes from 10 to 10k, count-all on request,
  column projection, server-side `WHERE`/`ORDER BY` with history and saved filters, and in-page find.
- **Staged SQL edits** — add, delete and edit rows as a pending change set with an exact command
  preview; nothing reaches the database until you commit. Document, key/value and stream writes
  apply immediately.
- **Cell editor** — a Monaco panel that detects JSON, XML, SQL, base64, hex, epoch seconds and
  milliseconds, ISO-8601 and CSV, with beautify and compact.
- **Foreign-key navigation** — preview a referenced row in a popover, then open it in a pre-filtered
  tab or jump straight to editing it (PostgreSQL, MySQL, MariaDB, SQLite).
- **Query console** — per-connection consoles with run-statement/run-all, saved queries, a Format
  action, EXPLAIN plans for every SQL dialect, and optional auto-explain that flags expensive
  queries. SQL completions, diagnostics and hovers come from a DDL document you paste in.
- **Column masking (PII)** — mark a column as PII with one of six masks and preview the masked view
  live. Display-only: the data itself is never changed.
- **Generate data** — fill a SQL table with fake rows, with per-column recipes inferred from types.
- **Operations panel** — every database operation live, with duration, rows, command, cancel and
  re-run, plus a persisted operation log.
- **Also** — three-tier caching with no speculative prefetch
  ([`docs/PERF.md`](docs/PERF.md)), Touch ID (or system password) confirmation before revealing a saved password,
  multiple windows (`⇧⌘N`), a command palette (`⇧⌘P`), a Runs module (see below), and Settings for appearance, data, cache, Api, Database MCP and advanced
  options.

### Runs

Scripts grouped in collections. Each runs in its own folder under the app home (or a folder you choose)
and keeps a run history with status, exit code and result. A **smart script** is a prompt run by headless
Claude Code instead of a shell command: it takes typed parameters (text, select, multi-select, secret),
a model, a budget and timeout, and a tick list of the tools and MCP servers it may use. A script can also
**recur** on a cron schedule; a due run asks for confirmation by default and never replays a missed run.
A secret parameter can keep a saved value that never leaves the keychain layer, which lets a schedule run
without asking. Smart scripts use the agent context and agent settings shared with Kira Space
(`~/.kira-space/agent/`) and always launch hermetically, so your own Claude Code config is not loaded. MCP
servers come from those agent settings.

### Docker

A Docker mode manages the local engine or a saved context: containers, images, volumes and networks,
live container logs and stats, and exec terminals into a running container. A remote `tcp://` engine
without TLS works but is flagged as insecure in the UI.

### Api (HTTP/gRPC client)

- **Request builder** — method, URL, params, headers and body (raw, JSON/XML/HTML/JS,
  form-urlencoded, multipart, binary file), with a raw request editor alongside.
- **gRPC** — unary and streaming calls, sharing collections, environments and variables with HTTP.
- **Collections** — a tree of requests and folders with Postman-format import and export. Move a
  request or folder to another collection.
- **Environments and variables** — collection- and environment-scoped variables, color-coded
  environments, Faker-backed dynamic values, and secret masking in history and logs.
- **curl** — paste a curl command to build a request, or copy any request out as curl.
- **Response history and timeline** — the last 30 responses per request, and a DNS/connect/TLS/
  first-byte/download timing breakdown.
- **Incognito tabs** — a per-tab toggle; nothing from that tab is saved (no history, no log, no
  variable writes).

### Database MCP server

An optional local [MCP](https://modelcontextprotocol.io) server lets an AI client read your
databases under rules you set. It is off by default and enabled from Settings → Database MCP.

- Loopback only, on port 8766, with its own bearer token.
- Six tools: `list_connections`, `list_children`, `describe_table`, `describe_schema`, `run_query`,
  `explain_query`.
- **Deny by default** — a connection stays invisible to the AI client until you expose it.
- **Per-connection permissions** — read, write and DDL are each *allow*, *deny* or *prompt*
  (defaults: read allow, write prompt, DDL deny). *Prompt* waits for you to approve the exact
  statement. A statement that can't be classified gets the strictest mode.
- Queries estimated above the expensive-query threshold pause for approval when auto-explain is on.
- PII masking rules apply to results before the AI client sees them.

## Requirements (building from source)

- macOS 26 or later, Apple Silicon (`arm64`), with Xcode command-line tools.
- [Go](https://go.dev) 1.27+ and the [Wails v3](https://v3.wails.io) CLI (`wails3`; `bun run setup`
  installs the version pinned in `go.mod`).
- [Bun](https://bun.sh) — package manager, script runner and test runner. Build tooling only;
  nothing ships a Node runtime.
- Optional, for database tests and local fixture databases:
  [Colima](https://github.com/abiosoft/colima) or another Docker-compatible daemon.

## Development

Run from the repo root:

```sh
bun run dev:studio       # native window with hot reload (installs dependencies first)
bun run package:studio   # build and ad-hoc sign the .app and .dmg
```

Other scripts you'll use most (all but the last three cover Kira Space too):

- `bun run setup` — install dependencies without building or running anything.
- `bun run screenshots` — regenerate the images in `screenshots/` from the built frontends against
  the mocked runtime. Run it by hand when the UI changes, never beside a UI test run (it rebuilds
  both `frontend/dist`). Run it a second time and `git diff --exit-code -- screenshots/` must pass:
  the output is byte-identical.
- `bun run site:dev` and `bun run site:check` — serve the presentation page from `site/`, and build
  it and check its layout at phone and desktop widths.
- `bun run lint` and `bun run typecheck` — Biome plus the repo's style guards; TypeScript for every
  package.
- `bun run test:unit`, `bun run test:go` — the unit and Go suites (Go's real-engine cases need
  Docker and skip without it).
- `bun run test:ui:studio` — Playwright UI tests against the built frontend.
- `bun run test:flows:studio` and `bun run test:e2e-real:studio` — the Go flow tests, which run the
  real app wiring against real SQLite, HTTP and gRPC servers and Docker, and the full-stack
  Playwright tier.
- `bun run test:claude:studio` — opt-in tests against the real `claude` CLI. They need an
  authenticated `claude` and spend a few cents of tokens.

`package.json` lists every script. `bun install` also installs git hooks: `pre-commit` runs lint and
typecheck, `pre-push` runs the Go build and linters. The app stores its data in `~/.kira-studio/`
(override with `KIRA_HOME`). For local test databases, see
[`scripts/demo-dbs/README.md`](scripts/demo-dbs/README.md). Test tiers are described in
[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)'s Testing section, and sandbox-specific setup in
[`docs/DEV_ENVIRONMENT.md`](docs/DEV_ENVIRONMENT.md).

## Architecture

Kira Studio is a single native Go process (Wails v3) that handles windowing, IPC, SQLite storage
and every database driver in-process — no sidecar, no second runtime. The Vue 3 frontend runs in
the system WebView. Control calls go through Wails bindings; result pages travel over a binary
FlatBuffers data plane. Adapters are capability-driven, so adding an engine means adding one
package under `apps/kira-studio/internal/adapters/`, not changing the UI.

```
apps/kira-studio      this app: Go backend (internal/), Vue frontend (frontend/src), tests
apps/kira-space       Kira Space, the git client and code workspace
internal              Go shared by both apps
packages/             TypeScript/Vue shared by both apps (workbench shell, theme, wire protocol, Api logic, git UI)
docs                  architecture, packaging, performance and the per-chapter records
```

[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) is the full current-state reference.

## Documentation

- [Documentation site](https://vlad-cirstean.github.io/kira-studio/docs/) — user guides for both apps, and reference pages.
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — how the app works today, plus known open
  limitations.
- [`docs/PACKAGING.md`](docs/PACKAGING.md) — building, releasing, install and update.
- [`docs/DEV_ENVIRONMENT.md`](docs/DEV_ENVIRONMENT.md) — building and testing in a given sandbox.
- [`docs/HISTORY.md`](docs/HISTORY.md) distills every finished chapter (v1 to v2.6). An open chapter's
  spec and phase plans live in `docs/v<n>/`.
- [`CLAUDE.md`](CLAUDE.md) — the working agreement for changes to this repo.

## Not shipped

Light mode; Windows and Linux; DDL editing; exporting results to CSV/JSON files; connection folders;
split editor groups; SSH tunneling; code signing and notarization. Silent auto-update is
deliberately absent: the app only installs an update after you click **Update** (see
[`docs/PACKAGING.md`](docs/PACKAGING.md) §7).

## License

[MIT](LICENSE)
