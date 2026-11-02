# Dev environment

How to build, run and test this repo in whatever sandbox/container a session happens to be in —
credential and tooling constraints, container quirks, per-subsystem run instructions. Not app
facts: those live in `docs/ARCHITECTURE.md`. Not team process: that lives in `CLAUDE.md`.

## Check the OS at session start

`uname -s`. A real Mac session carries full capabilities and permissions — a push credential with
the `workflow` scope (the `.github/workflows/` restriction below doesn't apply), the real macOS
Keychain, Colima for containers. A Linux sandbox/container session is the constrained case: every
restriction in this file assumes Linux unless its own section says otherwise. Check once, before
assuming a constraint below applies to the current session.

## The user cannot run anything in the sandbox

No shell, commands, kill, manual cleanup, tests or scripts in this container. An agent runs it, or
hands it off to the user's own machine/VM. Never ask the user to run a command in the sandbox.

## Git push: `.github/workflows/` changes can't be pushed from a Linux sandbox

On a Linux sandbox session, the git push credential is an OAuth App token without the `workflow`
scope, so GitHub rejects *any* push — including unrelated commits stacked on top — once a commit
touches a file under `.github/workflows/`. GitHub enforces this regardless of what the diff does;
no way to grant the scope from inside such a session. Confirmed not to apply on a real Mac session
(2026-09-12): a Mac session's push credential carries the `workflow` scope, so a commit touching
`.github/workflows/*` pushes normally there — skip the workaround below entirely.

On a Linux sandbox: never commit a change to `.github/workflows/*` directly. Instead:

- **Modifying an existing workflow file**: write the intended diff as a new file under
  `docs/pending-changes/` (create the directory if it doesn't exist) — the target workflow file's
  path as the filename with `.patch` appended (e.g.
  `docs/pending-changes/.github__workflows__release.yml.patch`), containing a normal `git
  diff`-style patch plus a one-line note of why.
- **Adding a whole new workflow file**: write it complete and ready-to-use under
  `docs/pending-workflows/` (create the directory if it doesn't exist).

Commit and push either like anything else (neither path is under `.github/workflows/`, so neither
trips the scope check). The user applies it to the real workflow file from an environment with
proper push credentials, and pushes it — then whoever applies it deletes the corresponding
`docs/pending-changes/`/`docs/pending-workflows/` file, in the same commit. An entry still there
means it hasn't been applied yet; don't recreate one that already exists for the same target file,
and don't let this workaround become an excuse to touch workflow files more often than the task
needs.

`.github/workflows/pages.yml` (P280, GitHub Pages deploy) is live. Remaining steps to go live:

1. In GitHub: Settings > Pages > Build and deployment > Source = GitHub Actions.
2. Pages deploys only via the dispatch `release-on-merge.yml` sends after each chapter merge, or
   by hand: run the "Pages" workflow on `main`. The `github-pages` environment accepts deployments
   from `main` only, by default. A run from another branch fails unless that branch is added under
   Settings > Environments > github-pages.
3. Open `https://vlad-cirstean.github.io/kira-studio/`.

## Branches, chapter PR, release (P330)

- `.githooks/pre-push` runs `scripts/check-push-refs.sh` first. It refuses a push to any branch but
  `main` and `chapter/v<major>.<minor>.<patch>`. Tags and deletes pass. A session handed a
  `claude/*` working branch pushes to the chapter branch instead: `git push origin HEAD:chapter/vX.Y.Z`.
- Next version when the user names none: `git fetch --tags origin && git tag -l 'v*' --sort=-v:refname | head -1`, patch plus 1.
- `gh pr create` uses GraphQL, which returns 403 in cloud sessions. Open the chapter PR over REST:
  `gh api repos/vlad-cirstean/kira-studio/pulls -f head=chapter/vX.Y.Z -f base=main -f title=vX.Y.Z -f body='…'`.
- After that, nothing is manual: `pr-auto-merge.yml` squash-merges once the required checks pass, and
  `release-on-merge.yml` tags and releases. Flow and recovery: `docs/PACKAGING.md` §7.

## Presentation screenshots and site check (P280)

- `bun run screenshots` rebuilds both `frontend/dist` (test bundles), then captures six shots in
  WebKit into `screenshots/`. For Studio only: `bun run build:test:studio && bunx playwright test
  --config=apps/kira-studio/playwright.config.ts --project=screenshots`; same pattern for Space. Never run it beside a UI suite in the same checkout. Not in hooks, CI or
  the Pages Action. Run it by hand when a shown UI changes, then commit the PNGs.
- Rerun it and check `git status --porcelain screenshots/` is empty. A diff means a nondeterministic
  input; fix the cause, never add tolerance.
- Shot flows: `apps/*/tests/screenshots/*.shot.ts`. A shot fails on a console error, so a mocked
  channel without a fixture (`E_FIXTURE_MISS`, HTTP 422) shows up as one; add the snapshot.
- `bun run site:check` builds the site, then runs the 375 px and 1280 px WebKit checks.
  `bun run site:dev` serves it locally.
- Site visual baselines (`site/tests/visual.spec.ts-snapshots/`) are recorded on Linux in this
  sandbox and skipped elsewhere. Refresh only for an intended change:
  `bunx playwright test --config=site/playwright.config.ts visual --update-snapshots` (after
  `bun run site:build`).
- `bun run site:build` runs `astro check`, then the build. Root `lint` runs `site/scripts/check-docs.ts`
  (docs style lint); run it alone with `bun site/scripts/check-docs.ts`. Its spec runs in `test:unit`.
  A `sources:` path that does not exist fails lint.

## Docker (for `packages/db-fixtures/`'s container fixtures, used directly by `apps/kira-studio/tests/e2e-real/`, and Docker flow tests)

- **Claude Code on the web's Linux containers**: `docker` is preinstalled but the daemon isn't
  running and there's no systemd. Start it directly, as root: `nohup dockerd > /tmp/dockerd.log 2>&1 &
  disown`, then check `docker info` / `/tmp/dockerd.log` for `"API listen on
  /var/run/docker.sock"`. Once per fresh container.
- **The Docker module's real-engine test needs the daemon up** (P200): `go test -race -v
  ./internal/docker/... ./internal/shell/...`; `TestEngine` skips without `dockerd`, so confirm it
  shows `--- PASS`, not `SKIP`.
- **Studio Docker flow tests need the daemon up too**; see "Kira Studio flow
  suites (P232)" below.
- **The other dev environment (macOS) uses Colima** (`colima start`) — not `dockerd` directly, and
  no systemd there either.
- **Docker Hub blob downloads are blocked here.** `production.cloudfront.docker.com` (the CDN every
  Hub blob redirects to) and `quay.io` both 403 through the outbound proxy, so a direct pull
  resolves the manifest but never fetches a layer. **`mirror.gcr.io` is not blocked** —
  the test fixtures use plain Hub names. Run `bun run test:images` once per session: it pulls every
  pinned image from the mirror in parallel (60s cold, 7.5 GB) and re-tags it to the plain name. The list
  comes from `testsupport.Images()`. `alpine:3.20` and `bash:5.2` keep only their `mirror.gcr.io/library/`
  name: a second plain tag on the same image fails `dockerflow` `TestImageRegistryURL`.
  Rule: a Docker Hub *official* image (no namespace — `mariadb`, `mysql`, `postgres`, `redis`,
  `mongo`) lives under `library/` on the real registry, so the mirror path needs that prefix. An
  already-namespaced image (`clickhouse/clickhouse-server`, `confluentinc/cp-kafka`,
  `localstack/localstack`) mirrors at the same path with no prefix. Confirmed for every image this
  repo uses. ClickHouse needs one further workaround on top — see the ClickHouse section below.
- **Docker Hub `429 toomanyrequests`** fails Go and `e2e-real` tests that pull `confluentinc/cp-kafka`, kibana or postgres.
  Not a code fault. Run `bun run test:images` (mirror pull) first, then rerun later if it persists.
- **Bun's `testcontainers` integration hangs indefinitely in this sandbox** on images that pull and
  run fine under plain Node with the identical image (confirmed fine on real hardware — a Bun-here
  quirk, not a real bug). Postgres is a confirmed instance. Workaround, already applied in
  `apps/kira-studio/tests/e2e-real/` (`support/postgres.ts`, `support/mariadb.ts`): invoke
  Playwright via its plain Node CLI entrypoint (`node node_modules/.bin/playwright test
  --project=e2e-real`), never `bunx playwright test`. **No vendored Node runtime exists**
  to bundle a standalone capture script against. Capturing a
  genuinely new `tests/ui/` fixture shape needs a one-off capture mode in
  `apps/kira-studio/internal/ipcfixture`'s Go generator, not yet built (only the six committed
  per-adapter fixture generators exist, see below).

## `apps/kira-studio/tests/ipc/` — building and testing in this environment (P50, updated P57, backend moved to Go P58f)

See `docs/ARCHITECTURE.md`'s Testing section for what this tier is and why its fixture is generated
rather than hand-written. This section is only about running it here.

- **Neither half needs `xvfb`** — no Electron, no native window anywhere in the repo. The frontend
  half (`bun run test:ipc:fe:studio`) drives headless Chromium against a static file server, the same way
  `tests/ui/` does.
- **The backend half is Go, not a bundled TypeScript spec** — no esbuild step, no vendored Node
  runtime. `apps/kira-studio/internal/ipcfixture`'s per-adapter Go test (`clickhouse_test.go`,
  `kafka_test.go`, `mariadb_test.go`, `mysql_test.go`, `redis_test.go`, `sqs_test.go` — postgres and
  sqlite generate no fixture of their own, see `docs/ARCHITECTURE.md`) drives the real
  `adapterhost`/`adapters` stack against a real container and writes
  `apps/kira-studio/tests/ipc/<adapter>/<adapter>.fixture.ts`.
- **Regenerating a fixture**: `KIRA_IPC_FIXTURES=write go test ./apps/kira-studio/internal/ipcfixture/...`
  writes each `<adapter>.fixture.ts` via `write.go`'s `mustMarshalNoEscape` (`SetEscapeHTML(false)` +
  typed structs, never maps, since Go's `encoding/json` otherwise escapes HTML and sorts map keys).
  Run `bunx biome check --write` on the result afterward — formatting only, content unchanged.
- **All six adapters' fixture generators run for real here** against containers pulled via
  `mirror.gcr.io`. ClickHouse needs no extra work this time, unlike the old TypeScript tier:
  `testcontainers-go/modules/clickhouse` doesn't hardcode a restrictive `Ulimits`. Kafka's capture
  has one quirk of its own: a read fans across both partitions and interleaves by arrival, not
  key/offset, so the fixture sorts the captured page by key, and the consumer group's coordinator
  host:port is frozen the same way ClickHouse's `.inner_id.<uuid>` is.
- **There is no one-off capture mode yet** (P58f D15's own gap, see the Docker section above):
  `KIRA_IPC_FIXTURES=write` only regenerates the six fixtures already committed under
  `apps/kira-studio/tests/ipc/`. Capturing a fresh shape for a new `tests/ui/` fixture needs that
  mode written first.

## Kibana and Elasticsearch — testing in this environment (P424)

- Fixture starts Elasticsearch plus Kibana (`kibana:9.5.3`, `elasticsearch:9.5.3`, plain Hub names; `bun run test:images` pulls both) on one Docker network. Needs about 1.5 GB of memory. Cold start takes 1 to 3 minutes; seeding adds seconds. Never run two heavy jobs beside it: a concurrent `vue-tsc` run OOM-killed Kibana (exit 137) on a 16 GB box.
- Run from `apps/kira-studio`: `CGO_ENABLED=1 go test ./internal/adapters/kibana/ -count=1 -timeout 40m` (conformance, search, KQL, interval). Complete suite: `KIRA_TEST_MATRIX=1 ... -run TestKibana_AuthMatrix`. Flow test: `go test ./internal/flows/dbflow -run TestKibanaDiscoverJourney`; `KIRA_CONTRACT=write` regenerates `tests/contract/kibana-discover.json`.
- Compat overrides: `KIRA_COMPAT_IMAGE_KIBANA` and `KIRA_COMPAT_IMAGE_ELASTICSEARCH`. Keep both on the same version.
- Elasticsearch runs with `cluster.routing.allocation.disk.threshold_enabled=false`. On a near-full disk the watermark otherwise leaves every shard unassigned and Kibana hangs on startup.
- A low-privilege Kibana user on 9.x gets no version from `/api/status`; tests do not assert one for that user.

## ClickHouse — testing in this environment (P36, its workaround resolved P58f)

See `docs/ARCHITECTURE.md`'s ClickHouse section for the adapter's own design facts.

- This sandbox's `ulimit -Hn` is fixed at 20000 and can't be raised even as root (a plain `docker
  run` with no custom ulimits works fine), which broke the old JS tier: `@testcontainers/clickhouse`
  hardcoded `.withUlimits({nofile: {hard: 262144, soft: 262144}})`. The Go side
  (`apps/kira-studio/internal/adapters/testsupport/clickhouse.go`, used by both
  `internal/adapters/clickhouse` and `internal/ipcfixture`) needs no equivalent workaround:
  `testcontainers-go/modules/clickhouse` doesn't hardcode a restrictive `Ulimits`, so this sandbox's
  ceiling never becomes a problem. If a future container image or module version reintroduces one,
  `testcontainers-go`'s own `WithHostConfigModifier` on the generic container option is the fix.

## SQLite — testing in this environment (P35)

- Coverage lives in `apps/kira-studio/internal/adapters/sqlite/*_test.go` (`bun run test:go`), no
  Docker: `modernc.org/sqlite` (pure Go, no cgo) against a real `t.TempDir()` file.
  `packages/db-fixtures/support/sqlite.ts` survives regardless, since `apps/kira-studio/tests/e2e-real/`
  still reads it directly.
- `apps/kira-studio/tests/e2e-real/sqlite-real.spec.ts` runs unconditionally, Docker-free by design
  — a real `-tags server` Go binary, every adapter served in-process, and a real temp-file database
  driven by a plain Playwright tab. Prerequisites (`scripts/setup.sh`, `bun run build:studio`, and
  `go build -tags server`) are memoized per worker process by
  `apps/kira-studio/tests/e2e-real/fixtures.ts`'s `buildPrerequisites()`.

## Secrets / `KIRA_INSECURE_SECRETS` (P25, moved to Go in P52/P57)

See `docs/ARCHITECTURE.md`'s Storage section for the cipher, the key and the envelope. Here:

- **There is no Linux keychain backend at all** (no `gnome-keyring`/`kwallet` probing) — set
  `KIRA_INSECURE_SECRETS=1` before launching the app (`bun run dev:studio`, the Go binary directly, or
  `t.Setenv("KIRA_INSECURE_SECRETS", "1")` in a Go test) on any Linux box to opt into the dev-only
  fallback. `apps/kira-studio/tests/e2e-real/`'s fixture sets it for every real-backend test. Kira Space needs it on Linux too (P447: saved script secrets); its flow harness sets it.
- Without it, Linux secret storage is **unavailable** — a password-bearing save fails visibly
  rather than silently falling back to plaintext. Deliberate, not a bug to work around.
- **On macOS the app ignores the variable outright** — it uses the real Keychain, so leaving it set
  accidentally can never weaken it. `apps/kira-studio/tests/ui/secrets.spec.ts`'s "keychain
  available" scenario guards this.

## The git module — running and testing it here (G1-G34, updated P79; moved to Kira Space, P100)

See `docs/ARCHITECTURE.md`'s Git module section for what it is and why. This section is only about
running it here. **As of v1.9 P100, the whole module lives under `apps/kira-space`, not
`apps/kira-studio`** — every path and env var below was retargeted at Part 1/Part 2, not left as
historical prose.

- **`go test ./apps/kira-space/internal/git...` needs a real `git` on `PATH` and nothing else** —
  no Docker, no container, no display. Each test builds its own repository under
  `t.TempDir()`, and cases that need `git` self-skip without it. The app's own floor is **git
  2.38** (`merge-tree --write-tree`), so an older `git` skips more than it runs rather than failing
  informatively.
- **`KIRA_SPACE_HOME` scopes the instance lock, not just the database.** The single-instance flock
  is `${KIRA_SPACE_HOME}/app.lock`, so two `KIRA_SPACE_HOME`s are two fully independent backends
  and a test never contends with a `bun run dev:space` session. Anything needing a server builds
  one over its own temp `KIRA_SPACE_HOME` — never the fixed path.
  Never `KIRA_HOME` (Kira Studio's own env var) — the two apps' homes are fully separate as of
  P100, so setting the wrong one silently talks to the wrong app's storage, or none at all.
- **A test binary panics if it resolves an app home with `KIRA_HOME`/`KIRA_SPACE_HOME` unset** (`kirapaths.Home`, non-`production` builds). A package whose tests reach one calls `testx.RunWithTempHomes` from `TestMain`.
- **`bun run dev:space` runs Kira Space's own dev loop** (`cd apps/kira-space && wails3 task dev`)
  — a separate native window/process from `bun run dev:studio`'s Kira Studio, on its own Vite dev-server
  port (9246, beside Kira Studio's 9245) so both can run at once without colliding.
- **The perf probes are opt-in and assert nothing.**
  `KIRA_GIT_PERF=1 go test -run 'TestGraphStreamPerf|TestG8PerfBaseline' ./apps/kira-space/internal/flows/gitflow/ -v`
  prints one `key=value` line per probe. No threshold assertion, deliberately: this container's
  numbers and a real Mac's aren't comparable, so a hard bound would be flaky in exactly the way
  it's meant to guard against. Record numbers in the commit message and, when they
  answer a stated budget, in `docs/PERF.md`.
- **`bun run perf:http:studio` probes the HTTP response viewer** (P160, `tests/perf/`, Playwright
  project `perf`, in no suite script). Builds the production bundle (`build:studio` overwrites
  `frontend/dist`; `test:ui:studio` rebuilds its own test bundle). `KIRA_PERF_RUNS` (default 3) runs
  per case, `KIRA_PERF_CASES=json,text-1line` narrows. 12 cases: 4 body shapes x 2.4/5/12 MB. Gate:
  run only at `load1 <= 1.0` (`/proc/loadavg`); each run line prints `load1` (includes the probe's own browser, ~1.0 extra, so a reading up to ~2 during a run is normal; check the gate before starting).
  Output is `key=value` per run plus a median `summary` per case. RSS is Linux-only and sums every
  browser process under the test worker.
- **`bun run perf:parse:studio` probes every large-input parse caller** (P163,
  `tests/perf/parse-callers.spec.ts`, same `perf` project and gate). Cases: `req-body-json`,
  `req-body-xml`, `grpc-json` (0.5/5 MB, `KIRA_PERF_SIZES_MB`), `console-format` (64/236/1024 KB),
  `doc-fieldnames`, `console-copy-all-json|shell`, `pure-fns` (pure callers timed in-page via a
  `bun build` bundle served at `/__pure.js`; WebKit timers are 1 ms), `worker-costs`.
  `KIRA_PERF_CASES`, `KIRA_PERF_RUNS` as above. Editors are seeded in-page by a synthetic paste:
  the Playwright WebKit transport breaks on any message over 512 KB. A before run needs a second
  worktree at the older commit with the probe files copied over. This sandbox's `load1` swings 1-8
  with nothing of ours running (host steal): read the per-run `load1`, rerun a case that matters.
- **`bun run perf:documents:studio` probes the Documents list scroll** (P161, `tests/perf/documents-scroll.spec.ts`,
  same `perf` project). 5 000 Mongo documents, ~160 px expanded rows, real wheel input. Cases `ladder`
  (momentum), `flick` (80 x 9 600 px), `flick-up`; `KIRA_PERF_CASES`, `KIRA_PERF_RUNS` as above. Prints
  frame p50/p95/max, frames over 50 ms and rendered-band coverage per run. Same gate. Run through the
  repo's own Playwright (`node node_modules/.bin/playwright`): the global `/opt/node22/bin/playwright`
  reports "No tests found".
- **The FSEvents watcher is `darwin && cgo`** (`apps/kira-space/internal/gitclient/watcher_fsevents_darwin.go`), so
  a Linux run exercises the `fsnotify` companion instead. Both satisfy the same seam and both are
  covered by `watcher_test.go`; only the darwin backend's own behaviour needs real hardware.
- **The scoped Go race run** for a git-chapter phase is the git packages in play plus
  `internal` itself for the layering test, not the whole tree — `git show bd0c224688d9:docs/v1.3/SPEC.md`'s own "Full
  verification scope" note fixes the list and the reason. The unscoped tree stays worth running
  occasionally as a backstop, not per phase.
- **A fresh worktree fails `bun run typecheck`/the pre-commit hook on any change**, even after
  `bun install` — `typecheck:web:studio`/`typecheck:space-web`/`typecheck:tests:studio`/
  `typecheck:space-tests`/`typecheck:unit:studio`/`typecheck:space-unit` all resolve one of the two
  apps' frontends' Wails-generated `@bindings/*` modules, and `go build ./...` (pre-push) fails
  separately on `//go:embed all:frontend/dist` finding no built frontend. **Run
  `sh scripts/prepare-worktree.sh` once per fresh worktree** (below) — it closes both gaps plus the
  Linux-only `wails3` CLI build deps, idempotently. `scripts/prepare-dev-environment.sh` runs it
  together with `scripts/codegraph-setup.sh` (CodeGraph section, below) — the one command for a
  worktree that also needs the code index. For a change confined to
  `packages/git-core`/`git-ipc`/`git-ui` where running the full script is disproportionate, verify
  instead with `bun run typecheck:git` (or the five `tsgo`/`vue-tsc` invocations it chains, run
  separately) plus each touched package's own `bun test`; the pre-commit hook itself still runs the
  unscoped `bun run typecheck`, so it fails regardless — its own header comments a `--no-verify`
  bypass for exactly that narrower case.

- **A fresh `git worktree` in this container can check out an orphaned "Initial commit" scaffold
  instead of the real branch tip.** Hit by 5 of P79's 6 fix batches. It is a provisioning race, not
  data loss — the affected worktree holds nothing of value, confirmed by `git status` and an
  ancestry check *before* any remediation. Recovery inside the isolated worktree is
  `git reset --hard`/`git merge --ff-only`/a fresh branch off the real tip; check what you actually
  have before choosing, and prefer stopping and reporting over self-remediating when the worktree
  might hold real work.
- **Rebasing this chapter's branch onto an advanced base needs `git rebase --rebase-merges`.** A
  plain `git rebase` flattens merge commits and replays every merged batch's individual commits
  linearly, reproducing spurious conflicts against content the branch's own merge commits already
  integrated. With `--rebase-merges` the same rebase replayed exactly one real conflict, once.
  State the shape that makes this apply (a feature branch that carries merge commits of its own),
  not just the flag.
- **`CodeWorkspaceService.ImportRepo` works in the `-tags server` build** (P231): `locator_server.go`
  uses `gitclient.NewHostLocator`, which finds `git` on `PATH`. The darwin locator stays native-only.
  Set `git.path` via `SettingsService.Set` (`{"git":{"gitPath":"/usr/bin/git"}}`) or the `settings`
  row `git.path`; the default `""` fails `exec: no command` in ADE git calls. The `/wails/runtime` POST body for a bound call, driven with a plain `curl` (no
  browser, no `wails3` client): `{"object":0,"method":0,"args":{"call-id":"<uuid>","methodName":
  "github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge.<Service>.<Method>",
  "methodID":0,"args":[<positional JSON args>]}}` — `object:0`/`method:0` select Wails v3's own
  `Call`/`CallBinding` request kind (`pkg/application/messageprocessor.go`'s `callRequest`/
  `messageprocessor_call.go`'s `CallBinding`, both `0`); the response body is the bound method's
  own return value marshaled directly, no envelope.
- **The session's own auto-mode permission classifier blocks `git remote set-url`/`git remote add`
  outright, even against a disposable scratch repo built for a live check** (P129 Part 6's own
  `§6.1`) — flagged as a "Remote Repoint" regardless of intent. A live check that needs a
  GitHub-shaped `origin` to exercise a GitHub-gated code path (`githost/github`'s `ParseRemote`
  and `Claims`) cannot repoint an existing scratch remote to fake that shape in this environment; plan
  around it (seed the scratch repo with the right remote URL from the start, before it exists as a
  git remote at all) or fall back to reading the gate's own source plus its existing unit coverage,
  rather than spending a retry loop on the same denied action.

## Wails v3 / Go — building and testing in this environment (P51, P52, P55)

- **None of this toolchain persists across sessions.** Re-run at the start of any fresh container:
  `apt-get update -qq && apt-get install -y libgtk-4-dev libwebkitgtk-6.0-dev pkg-config` (needed even though the product
  targets macOS — `wails3`'s own Linux build fails at `internal/operatingsystem` with a
  `pkg-config` error without it), then `go install github.com/wailsapp/wails/v3/cmd/wails3@<the
  version go.mod pins>` and `export PATH=$PATH:$(go env GOPATH)/bin`. **Pin that version, never
  `@latest`** (which once resolved a beta ahead of `go.mod` and silently skewed the bindings
  generator against the runtime library). **Pin `GOTOOLCHAIN` to go.mod's own `go` directive for
  that install too** (`GOTOOLCHAIN=go<directive> go install …`) — `auto` resolves from *Wails'* own
  floor rather than this repo's, which once degraded the bindings generator into 52 spurious
  "requires newer Go version" warnings on this exact repo. `scripts/setup.sh` does all of this
  automatically.
- **`wails.io`/`v3.wails.io` are 403-blocked**, on real macOS hardware too — organizational proxy
  policy, not a sandbox artifact. `proxy.golang.org` is reachable, which is all the Go toolchain
  needs. **Read the installed module source under
  `$(go env GOPATH)/pkg/mod/github.com/wailsapp/wails/v3@<version>/` instead of the docs site** —
  it's the real source for the exact pinned version.
- **No package under either app's `internal/` needs a C compiler on Linux any more** (v1.9 P97
  removed `internal/codeparse`, this repo's one unconditionally-cgo package). Every remaining cgo
  call either app makes (a handful of darwin-only files in repo-root `internal/secrets`, Kira Studio's own
  `internal/localauth`, repo-root `internal/metrics` (P116 H4: shared between both apps' own
  metrics tickers), and Kira Space's own `internal/gitclient`'s FSEvents watcher) is behind a
  `darwin && cgo` build tag with a real, working `!darwin || !cgo` companion, invisible to a Linux
  build; `modernc.org/sqlite` (the sqlite adapter and each app's own storage) stays cgo-free on
  every platform. `go test ./apps/kira-studio/internal/...` /
  `go build ./apps/kira-studio/internal/...` (and the `apps/kira-space` equivalents) need no C
  compiler in this container as a result. Only each app's own `main` package imports Wails and
  needs the GTK/WebKit headers, so prefer `./apps/kira-studio/internal/...` or
  `./apps/kira-space/internal/...` for a fast loop.
- **`GOOS=darwin` cross-compiles here only with `CGO_ENABLED=0`.** A pure-Go package builds and
  vets for `darwin/arm64` from this container (`GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build
  ./…`, exit 0); a cgo one cannot be built for darwin here at all (`CGO_ENABLED=1 GOOS=darwin`
  fails inside `runtime/cgo` with `clang: error: unsupported option '-arch'`), while a real macOS
  build is `CGO_ENABLED=1`. So a `darwin && cgo` file is compiled, vetted and tested by nobody
  until a human builds on a Mac. Treat that as a **design** constraint, not just a testing gap: for
  code whose whole purpose is to work on a path nobody exercises interactively, prefer a pure-Go
  implementation this container can build and test — `internal/startupfail` chose an argv-only
  `osascript` spawn over a cgo `NSAlert` shim for exactly this reason, and it's checkable in CI as
  a result. Wails 3.1's build guard rejects `CGO_ENABLED=0` on darwin without `-tags purego` (or
  `server`): add `-tags purego` to the vet/build line above.
- **Regenerate bindings via `wails3 task common:generate:bindings`** (or `scripts/setup.sh`, which
  calls it) — never a hand-typed `wails3 generate bindings` flag list, which has already drifted
  from the task's real flags once. `scripts/setup.sh` now calls the task **unconditionally on every
  run**; a CLI/toolchain-identity stamp only decides whether Task's own checksum cache gets wiped
  first, so a plain `bun run setup` always spends the task's own up-to-date check (~0.2s when
  nothing changed, ~7s on a real source change) rather than skipping it. Each app's `frontend/bindings/**` are real Vite import
  targets, so a missing one fails the build with an unresolvable import rather than a stale-bindings
  surprise; regenerate whenever a bridge service's method set changes, and before any frontend
  build. **`-names` is load-bearing, not cosmetic**: without it, every generated call site emits
  `$Call.ByID(<numeric-id>, ...)` instead of `$Call.ByName("<pkg>.<Service>.<Method>", ...)`, and
  `tests/ui/support/mockRuntime.ts`'s request-interception layer is keyed on the `ByName` string FQN
  (`CHANNEL_TO_FQN`) — a `-names`-less regeneration silently breaks every `tests/ui/` spec at the
  very first bound call of the boot sequence (`layoutGetAll`), surfacing as
  `page.waitForSelector('[data-testid="status-bar"]')` timing out with a page-level `Error: no
  CHANNEL_TO_FQN entry for undefined` — nothing about the failure points at bindings at all.
- **A first `wails3 task dev` build takes ~60s** (native compile, icon/binding generation, Vite cold
  start). Giving up after 15-25s looks exactly like a sandbox limitation and isn't.
- **A background process started in one shell invocation cannot be signalled from a later, separate
  one** — it still shows up in the later call's `ps aux` (the process table is shared) but the
  signal doesn't land, leaving an unreapable zombie squatting on its port for the rest of the
  session. Do everything — start, poll, test, kill — inside **one** invocation, polling a log file
  or a port in a loop instead of a fixed `sleep`, with a correspondingly long timeout. Relatedly,
  `export FOO=bar && long-command &` backgrounds the whole `&&` chain, so `FOO` never reaches the
  parent shell — put `export` on its own line.
- **This container's minimal init reaps slowly.** After killing a process group, its members can
  answer `kill(pid, 0)` as alive for a second or two: a reparented orphan becomes a zombie the
  moment it exits, and `kill(pid, 0)` succeeds against a zombie until something `wait()`s it.
  `internal/preconnect`'s and `internal/connections`' process-group-kill tests poll for `ESRCH` with
  a multi-second timeout for exactly this reason — don't tighten them based on a fast macOS run.
- **On Linux, `/wails/runtime` and `/wails/stream/*` are unreachable over plain HTTP from a desktop
  build**, dev or packaged: `pkg/application/linux_cgo.go` registers `wails://` as a custom URI
  scheme intercepted *inside* the native process, so `curl` or a plain browser tab can never
  exercise real bindings there. **The `//go:build server` platform is the way around it**
  (`go build -tags server`, zero source changes): it serves the whole bound-call surface and the
  data-plane stream over a real TCP listener with no webview and no scheme registration at all —
  this repo's established substitute for GUI-driven boot proofs in a sandbox with no display,
  preferred over `xvfb`/`xdotool`/screenshot techniques. `tests/e2e-real/` is built on it.
- **Debugging a rendered bug (real CSS/layout) with an actual display**: both apps'
  `build/config.yml` build their `wails3 dev` run with `EXTRA_TAGS=mcp`, which compiles in Wails
  v3's own MCP server (`js_eval`, `dom_query`, `mouse_click`, `screenshot_dom`, etc. — see
  `pkg/application/mcp_*.go`). Scoped to `dev_mode`, so production packaging
  (`wails3 task darwin:package:dmg`) never sees it. Reach it at `http://127.0.0.1:9099/mcp`,
  either via `claude mcp add --transport http <name> http://127.0.0.1:9099/mcp` (new session
  needed before the tools show up) or plain `curl` (JSON-RPC, no handshake needed: `tools/list`,
  `tools/call`).

## Kira Space real backend in a sandbox — server-tag recipe (P126)

Build: `go build -tags server ./apps/kira-space/...` (Wails v3/Go's own `server` platform, above),
temp `KIRA_SPACE_HOME`, `WAILS_SERVER_HOST=127.0.0.1`. Run `bun run build:space-mobile` first: `main.go` embeds `frontend/dist-mobile`.

- **Page boots with `/?window=<uuid>`**: the renderer calls `WindowsService.Ensure`, which creates
  the `windows` row. No seeding. Import repos via `CodeWorkspaceService.ImportRepo`.
- **Git discovery works here** (P231): the server build uses `gitclient.NewHostLocator`. No
  throwaway `Locate` patch.
- **A fresh or reused worktree needs `bun run setup` before typecheck, unit tests, knip or a push.**
  Bindings are gitignored; a stale set (e.g. a removed `SetAgentAware`, a new `DockerService`) breaks
  `typecheck`, `test:unit` and the pre-push `lint:dead` (knip reports unlisted `@bindings/*`). Run
  `bash scripts/setup.sh` in the worktree you push from, in a fresh worktree and after adding a bound
  service, then push again if the hook failed on missing bindings.
- **The ADE v2 flows need no `Locate` patch** (P148 live run): seed `code_repos`, `windows('main')` and
  `git.path` rows, and the board, runs, Take over, archive and dialogs all work.
- **`claude -p` blocks a standalone `sleep N` and backgrounds it**, so the run ends `ended without
  finish_step`. A live-run step that must stay running prompts `until [ -f <flag> ]; do sleep 2; done`.
  Kill leftover `claude` TUI processes after a run.
- **A reused server-tag harness home keeps persisted tabs.** A stale repo-graph tab in `tabs` makes a
  fresh run open the old graph; delete the row before reuse (P203).
- **An interactive `claude` runs here** (P237, 2.1.295). Under a temp `HOME` it needs `.claude.json`
  `{"hasCompletedOnboarding":true,"theme":"dark","projects":{"<cwd>":{"hasTrustDialogAccepted":true}}}`.
  Without `projects[cwd]` it stops at the folder-trust dialog. Auth comes from env, so a temp `HOME`
  still signs in. Hooks fire in the TUI as in `claude -p`. See "Real `claude` tests".
- The server build's `EmitTo` broadcasts to every page (`internal/shell/emitto_server.go`, P231). Payloads without
  a window key reach all pages: close-flush request, `AdeTaskOpenSession`, `ChannelMobileOpenLaunch`. With two pages
  in `e2e-real`, closing one window flushes all; focusing an ADE session opens it in all. Use one page per such test.
- **A fake `gh` or `claude` needs a `HOME` override** (P150). PTY login shells reorder `PATH`, so a prefix
  set in the server's env is lost. Point `HOME` at a dir whose `.bash_profile` prepends the fake bin dir,
  then confirm with `/proc/<pid>/cmdline`. Discovery uses `exec.LookPath` for `gh`.
- **Review smoke seeding** (P150): `code_repos.repo_id` is the repo path; copy sample workflows into
  `<home>/workflows`; task rows need `entry_step_id`. To patch git discovery without touching source,
  build with a `-overlay` JSON replacing the discovery file.
- **In harness scripts never `pkill -f` or `pgrep -f` a pattern that appears in your own command line**
  (e.g. `claude --session-id`): it kills the calling shell. Anchor it (`pgrep -f '^claude --session-id'`).

## Kira Studio flow suites (P232)

- Go flows: `bun run test:flows:studio`. Complete suite: `bun run test:flows:studio:complete`
  (`KIRA_FLOW_COMPLETE=1 KIRA_FLOW_DOCKER=require`, 20 min timeout). Real-backend Playwright:
  `bun run test:e2e-real:studio`, which builds `apps/kira-studio/bin/flowservers` (gitignored), the
  HTTP, HTTPS and gRPC servers the Go flows also use.
- Run `go test` with `CGO_ENABLED=1`.
- Fake `claude` also plays `claude mcp list` (reads `$HOME/.claude.json`, records `mcp-list-<n>.args`) and, for `--setting-sources user`, finds tools on those servers within `--settings` `allowedMcpServers`. A test that changes the config sets `claude.SetMCPListTTL(0)`.
- Docker flows need a daemon; if `docker ps` fails, start one with
  `setsid nohup dockerd > /tmp/dockerd.log 2>&1 < /dev/null &`. Without one they skip, or fail under
  `KIRA_FLOW_DOCKER=require`. They pull `mirror.gcr.io/library/alpine:3.20`; labelled leftovers older
  than an hour are swept at the next run. The `not-installed` status class needs a host without `/var/run/docker.sock`
  (run it on a Mac with Docker stopped and no `~/.docker`).
- `docker-real.spec.ts` execs into `mirror.gcr.io/library/bash:5.2`: with alpine, busybox ash can swallow an
  Enter that follows a cursor-position reply. Test-side workaround, not a product bug.
- `terminal-real.spec.ts` uses a temp `HOME` removed at process exit, not in `afterAll`: with `fullyParallel`
  `afterAll` runs between tests of one worker.

## Kubernetes flow tests (P439)

- `go test ./apps/kira-studio/internal/flows/kubeflow` needs no cluster and no kubectl. `flowharness.KubeFake`
  is an HTTPS API server; `KubeFake.WriteKubeconfig` writes the kubeconfig and the exec-plugin script.
- The harness reads PATH and KUBECONFIG from the process environment at call time (`kubeEnv`), so a test sets them with `t.Setenv`.
- Rewrite `apps/kira-studio/tests/contract/kubernetes.json` with `KIRA_CONTRACT=write`, then rerun without it; the UI spec `kubernetes-module.spec.ts` reads the same file.

## Contract fixtures (P249)

- Fixtures live in `apps/<app>/tests/contract/*.json`, written by Go flow tests and read by UI specs.
- Regenerate: `KIRA_CONTRACT=write go test -p 2 ./internal/flows/<pkg> -run <Test>` from the app
  dir, then rerun without the variable with `-count=2` to confirm stability. Write merges; delete
  stale keys by hand. Inspect the diff for secrets and machine paths.
- Run one UI half: `npx playwright test --config=apps/<app>/playwright.config.ts --project=ui <spec> -g "contract:"`.
- Biome ignores the directories; never hand-format them.
- Phone specs set `server.state.replies.{me,pair,expired,backlog,backlogItem}` from a contract. Studio
  specs push an engine `cache:stats` event with `pushCacheStats` (`tests/ui/support/mockStream.ts`).
- A `bun run ... &` job dies with its tool call; start long suites with `setsid nohup`.
- Complete-gated flow tests that write contracts need `KIRA_FLOW_COMPLETE=1 KIRA_FLOW_DOCKER=require`.

## acli fixtures (P417)

- `apps/kira-space/internal/issuetracker/jira/testdata/*.json` are hand-written from acli's documented output. Running a downloaded `acli` binary was refused in the sandbox, so no real output was captured.
- Refresh by hand against a real site: `acli jira workitem view KEY --json --fields '*all,-comment,-attachment,-worklog,-description'`, then redact. Keep both Development field forms (string and object).
- P428 adds `issue-detail.json`, `issue-detail-plain.json` and `comment-list.json`. Refresh with `acli jira workitem view KEY --json --fields <jira.DetailFields>` and `acli jira workitem comment list --key KEY --json --limit 50 --order -created`, then redact.
- P429: no `comment create` fixture exists. `fakeacli` records the call and reads the `--body-file` ADF. Check by hand: `acli jira workitem comment create --key KEY --body-file body.json` with an ADF document.
- Flow tests use `flowharness/fakeacli`; no test needs `acli` installed.

## `az` fixtures (P423)

- Install for captures: `python3 -m venv /tmp/azenv && /tmp/azenv/bin/pip install azure-cli && AZURE_CONFIG_DIR=/tmp/azcfg /tmp/azenv/bin/az extension add --name azure-devops`. Keep `AZURE_CONFIG_DIR` in scratch.
- `githost/azure/testdata`: version, configure-list and stderr files are captured from real `az`. PR, policy, query and thread JSON are hand-written from the documented shape and await confirmation against a real org (`testdata/README`). No org was reachable in the sandbox.
- P427 detail fixtures (`pr_detail.json`, `policy_detail.json`, `threads_get.json`) are hand-written too. Check the invoke GET projection with `az devops invoke --area git --resource pullRequestThreads --route-parameters project=<p> repositoryId=<r> pullRequestId=<n> --http-method GET --query "<ThreadsQuery>" --org https://dev.azure.com/<org> --detect false -o json` (`--query` is a global argument; `--help` lists it).
- GitHub `githost/github/testdata/pr_detail.json` is hand-written from the GraphQL schema. Optional check: `gh api graphql -f query=@<detailQuery text> -f owner=<o> -f name=<r> -F number=<n>` against a real PR.
- Refresh PR JSON: `az repos pr list --org https://dev.azure.com/<org> --project <p> --repository <r> --detect false -o json`, then redact.
- Flow tests use `flowharness/fakeaz`; no test needs `az` installed.

## Kira Space flow suites (P231)

- Go flows: `bun run test:flows:space`. Complete suite: `bun run test:flows:space:complete`
  (`KIRA_FLOW_COMPLETE=1`, 20 min timeout). Real-backend Playwright: `bun run test:e2e-real:space`
  (builds `apps/kira-space/bin/kira-space-server-test` and `bin/fakeclaude`, gitignored).
- Fake `claude`/`gh`: the test binary or `fakeclaude` symlinked as `claude`/`gh`. Scenario JSON
  path in `KIRA_FAKE_SCEN`, call records (`.args`, `.cwd`, `.prompt`) in `KIRA_FAKE_DIR`.
- Second code host: `flowharness.WithGitHost(fakehost.New())` registers an in-process `forge.test` host
  (merge requests) after GitHub. No CLI, no PATH. Azure registers in every harness through an in-process `fakeaz`; `WithAzure` seeds PRs and auth state, and the default harness has no az. Use `WithoutGh()` to prove no step reaches `gh`. It records reviews and comments (`SetAuthor`, `Reviews`, `Comments`) and refuses approving your own merge request.
  Flow: `flows/githostflow`; UI half
  `tests/ui/git-host-provider.spec.ts`.
- Harness roots use a short `os.MkdirTemp("", "ksf")` path: unix-socket paths cap near 108 bytes.
- Many watchers per parallel test can hit the inotify limit; raise `fs.inotify.max_user_instances`.
- Run `go test` with `CGO_ENABLED=1`; lint with `PATH=$HOME/go/bin:$PATH`.
- Coverage gate (P236): `apps/<app>/internal/flows/coverage/` fails on a bound method with no flow call.
  Exempt one with a reason in `exempt.txt`.

## e2e-real: a plain page is not the main window (P246)

Prompts route to the main window key. A Chromium page opened by the fixture has a random key, so a
popup never shows there. Call `openMainWindow(kira)` (`@workbench/testing/e2eReal`) first: it reads
`PromptsService.MainWindow` and reloads the page under that key. Open a second window with
`browser.newPage()` and `?window=<key>`, not `page.context().newPage()`.

## Real `claude` tests

Opt-in tests that run the real `claude` CLI and spend tokens. Never in CI, pre-commit or pre-push.

- Own Go module per app: `apps/kira-space/tests/claude/`, `apps/kira-studio/tests/claude/`. Opt-in by absence:
  `go test ./...`, `lint:go`, CI and hooks stop at the nested `go.mod`; pre-push lints a changed nested module
  by `cd`-ing into it (`nested_go_mods` in `scripts/hook-scope.sh`), never tests it. No build tag, no env flag. Never add
  a root `go.work`: workspace mode pulls the modules into `./...`. After a Wails or Go bump run
  `go mod tidy` in both modules.
- Run: `bun run test:claude` (both), `test:claude:space`, `test:claude:studio`. Narrow with `go test` flags
  appended: `bun run test:claude:space -run 'TestAdeHeadlessRun|TestAdeRebaseRun'`.
- Compile check without spending: `bun run lint:claude` (golangci-lint in both modules).
- Needs an authenticated `claude` on `PATH`. Tests use haiku (`--model haiku`, `ANTHROPIC_MODEL=haiku` for
  app-built launches), a temp `HOME`, `--max-budget-usd 0.05` and one-sentence prompts. A package preflight
  skips, not fails, when `claude` is missing or `claude -p` cannot run under the temp `HOME`.
- Rule: after changing code in an area below, run that row before calling the change done. Append `-count=1`: `go test` caches a passing run and prints the old spend.
- Resume rows (P279, CLI 2.1.296): a held `hold.sh` Bash call, app restart, Resume. Prompts say the call is safe to rerun and to inspect nothing first; haiku otherwise runs `ps`/`ls`, gets denied (no approval surface) and stops as blocked. The CLI's "interrupted" `tool_result` is not in the run log; the check is the shared Claude session id. Hand probe recipe: temp `HOME` with an onboarding-done `.claude.json`, parent `CLAUDE_CODE_SESSION_ID`, `CLAUDE_CODE_REMOTE_SESSION_ID`, `CLAUDECODE` unset, SIGKILL the child's process group mid tool call, then `claude -p --resume <id>`.
- P444 (CLI 2.1.316): `TestSmartScriptRun/user_MCP_server_loads_through_claude` runs real `claude mcp list` plus one haiku run, a few cents.
- Spend, measured on 2026-10-09 (CLI 2.1.295, haiku): `claude -p` results of the Space suite 0.014 USD, TUI sessions (about 4 at 1 to 2 haiku prompts) an estimated 0.02 USD, Studio suite under 0.01 USD, memory gate smoke a few cents, import smoke 0.14 USD (P211), memory docs test 0.01 USD (P276). Space suite about 75 s, Studio about 12 s. The Space run prints its measured `claude -p` spend. P279 (CLI 2.1.296): resume rows 0.0067 USD per run of the three (about 55 s), whole Space ADE and smart set 0.020 USD (about 156 s). P296: `comment` subtest of the memory docs test 0.006 USD (about 16 s), prompt-registry row of the Space suite 0.083 USD (about 427 s). P297 (CLI 2.1.302): `TestMemoryDistill` 0.03 USD (about 21 s), `TestMemoryDistillBridge` 0.03 USD (about 13 s), `TestMemoryMcpFollowsDocs` 0.022 USD (about 85 s), prompt-registry row 0.082 USD (about 444 s); `TestMemoryGate` now also checks rule kind (about 32 s). Eval subset round `round-p297-01` 1.50 USD (ledger `spend-p297.jsonl`, needs `KIRA_EVAL_PHASE_CAP_USD` above the run cap plus 2); a shallow clone must first `git fetch --depth=1 origin <corpus commit>`.
- `scripts/docshistory` (docs chapter fold, P299): needs the logged-in `claude` CLI, builds `kira-space` itself
  (or `--binary`), gives each unit temp `KIRA_MEMORY_HOME`/`KIRA_SPACE_HOME` and never touches your memory DB.
  Defaults (also `doc`): `--runs docs/history-runs`, `--out -`, `--jobs 1`. Cost per phase 0.02 to 0.25 USD (P299 pilot, mean 0.07);
  v2.5 (P300): 27 units, 1.45 USD real, 3.20 USD charged (failed tries charge a 0.25 USD estimate).
  v2.6 (P305): 12 units (13 tries), 0.78 USD real, 0.78 charged. `--max-usd` stops new
  units, resume reruns the command. `repoint` skips `scripts/docshistory/` and both `testdata/` roots (eval data lives in
  `apps/kira-space/tests/claude/testdata/memory-eval/`, `internal/llmprompts/testdata/`).

| Changed area (paths) | Command |
|---|---|
| Hook injection, shim, listener: `internal/harness/claude/hooks/**`, `apps/kira-space/internal/ade/tracker.go`, `internal/terminal/bound.go` | `bun run test:claude:space -run 'TestTerminalAgentHooks\|TestHookPayloadContract\|TestShimInertForPlainClaude'` |
| Claude config isolation: anything that composes a `claude` argv or env (`internal/harness/claude/**`), `apps/kira-space/internal/appwire/wire.go` | `bun run test:claude:space -run TestRealClaudeSettingsUntouched` |
| ADE runs and sessions: `apps/kira-space/internal/{ade,adeflow}/**, internal/agentmcp/**, internal/mcpcall/**, internal/harness/claude/headless.go` | `bun run test:claude:space -run 'TestAdeHeadlessRun\|TestAdeStepResults\|TestAdeManualSession\|TestAdeAdHocSession\|TestAdeRebaseRun\|TestAdeResumeRun\|TestAdeResumeLost\|TestAdeReviewStep\|TestAdeReviewLost\|TestAdeSandbox\|TestAdeSandboxReadOnly\|TestAdeBranchRequest\|TestAdeReportArtifacts'` (`TestAdeBranchRequest` holds the request 65 s, about 90 s, 0.009 USD: the CLI cuts an MCP call at 60 s unless `MCP_TOOL_TIMEOUT` is set, which Kira does for every launch) |
| Agent sandbox: `internal/agentsandbox/**`, `apps/kira-space/internal/ade/sandbox.go` | `bun run test:claude:space -run 'TestAdeSandbox\|TestAdeSandboxReadOnly' -count=1`. Linux checks the second layer (flags, note, `enforced 0`); the macOS half is in the Mac checklist below. |
| Smart scripts: `internal/{agentmcp,mcpcall,scriptruns,scripts}/**` (incl. `scheduler.go`, `confirm.go`), `internal/harness/claude/headless.go`, `apps/kira-space/internal/ade/automation.go` | `bun run test:claude:space -run 'TestSmartScriptRun\|TestSmartScriptResume'` and `bun run test:claude:studio -run TestSmartScriptRun` |
| Memory MCP and install: `internal/memory/memorycli/**`, `internal/memory/mcpserver/**`, `internal/memory/memfmt/**`, `internal/mcpcall/**`, `apps/kira-space/internal/appwire/memoryops.go`, `internal/harness/claude/mcpinstall/**`, `apps/kira-space/internal/bridge/memory.go`, `apps/kira-space/internal/bridge/memorydistill.go` | `bun run test:claude:space -run 'TestMemoryMcpThroughClaude\|TestMemoryMcpFollowsDocs\|TestMemoryInstallRegisters\|TestMemoryDistill\|TestMemoryDistillBridge'` (`TestMemoryDistill` covers `ingest_files` with `distill` true, P297; `TestMemoryDistillBridge` covers the UI bridge call, P285 Part 4, P297; `TestMemoryDistillRewrite` covers mode rewrite, P315, about 0.01 USD) |
| Memory gate: `internal/memory/{agentrunner.go,gate*.go,service.go}`, `internal/harness/claude/structured.go` | `bun run test:claude:space -run TestMemoryGate` |
| Memory import and ingest: `internal/memory/importer/**`, `internal/harness/claude/structured.go` | `bun run test:claude:space -run 'TestMemoryImport\|TestMemoryIngest\|TestMemoryDistill\|TestMemoryDistillBridge\|TestMemoryDistillTrivia'` (import about 50 s, 0.14 USD, P211; ingest P274; the `TestMemoryDistill` prefix also runs `TestMemoryDistillRewrite`, P315) |
| Memory ingest eval (P285): `internal/llmprompts` memory.import.* prompts, `internal/memory/importer/{chunk,extract,finalize}*.go` quality | `bun run eval:memory` with `KIRA_EVAL_MODE=run KIRA_EVAL_ROUND=<name>` (see ARCHITECTURE "Memory ingest eval"). Opt-in, never in hooks or CI. Costs about 4 to 5 USD per tuning run (8 to 10 min), judge included; P285 spent 57.9 USD in all; Part 2 spent 28.5 USD (ledger `spend-p2.jsonl`). Part 3 spent about 5.4 USD on the eval (ledger `spend-p3.jsonl`). P289 spent about 8.9 USD on the 5-source subset (ledger `spend-p289.jsonl`; `KIRA_EVAL_ONLY=plan-p257,spec-chunk-go,doc-arch-prompts,cmt-runner-go,json-ade-errors`, about 1.5 USD per round); run the test binary from `apps/kira-space/tests/claude` (`go test -c`) to keep a tree edit from changing a round midway. `KIRA_EVAL_MODE=footprint` with `KIRA_EVAL_ROUNDS=<r1,r2>` re-renders stored facts of past rounds and costs nothing. P291 (retrieval and telegraph eval) spent 14.04 USD of 20 (ledger `P291-runs/spend-p291.jsonl`, set `KIRA_EVAL_RUNS_DIR=P291-runs KIRA_EVAL_LEDGER=spend-p291.jsonl`); modes `questions`, `keys`, `telegraph`, `score` (with `KIRA_EVAL_STYLE=tele`) are paid, `retrieval` and `compare` are free and need `KIRA_ORT_LIB` (`sh scripts/fetch-onnxruntime.sh linux-x64`). Not a pass/fail row: it scores quality. |
| DB MCP: `apps/kira-studio/internal/dbmcp/**`, `apps/kira-studio/internal/kubernetes/**`, `apps/kira-studio/internal/bridge/dbmcp.go`, `apps/kira-studio/internal/proof/**`, `apps/kira-studio/internal/bridge/proof.go` | `bun run test:claude:studio` (`TestKubeMcpThroughClaude` covers the kube tools, P440) |
| Prompt registry, store, templates: `internal/llmprompts/**` (after a default rewrite run the whole row; regenerate goldens with `go test ./internal/llmprompts -run TestGolden -update` and contracts with `KIRA_CONTRACT=write`) | memory gate, memory MCP, memory import, ADE, smart scripts and DB MCP rows above: `test:claude:space -run 'TestMemoryGate\|TestMemoryMcpThroughClaude\|TestMemoryMcpFollowsDocs\|TestMemoryInstallRegisters\|TestMemoryImport\|TestMemoryIngest\|TestMemoryDistill\|TestMemoryDistillBridge\|TestMemoryDistillTrivia\|TestAdeHeadlessRun\|TestAdeStepResults\|TestAdeManualSession\|TestAdeAdHocSession\|TestAdeRebaseRun\|TestAdeResumeRun\|TestAdeResumeLost\|TestAdeSandbox\|TestSmartScriptRun\|TestSmartScriptResume'` and `bun run test:claude:studio` |
| Agent context: `internal/agentctx/**`, `ContextFile` in `internal/harness/claude/{interactive,headless}.go`, `apps/kira-space/internal/ade/{tracker,runs}.go`, `internal/scriptruns/{smart_run,resume}.go` | `bun run test:claude:space -run TestAgentContextHeadless -count=1` (one haiku smart run, about 10 s, a cent) |
| Any other `internal/harness/**` change | `bun run test:claude` |
| Claude CLI upgrade | `bun run test:claude` |

Sandbox notes:

- Inside an agent session the child `claude` reuses the parent's session id. The suite unsets
  `CLAUDE_CODE_SESSION_ID`, `CLAUDE_CODE_REMOTE_SESSION_ID` and `CLAUDECODE`; the preflight asserts the ids differ.
- The TUI tests seed `$HOME/.claude.json` with onboarding done and `projects[cwd]` trust (see the server-tag section).
- The Space suite replaces the harness's fake `claude` with a wrapper script in `BinDir`: it copies stdout of
  `claude -p` runs to a file (the app's run log drops the cost) and execs the real binary otherwise.
- Studio's harness fakes the `claude` MCP installer. `TestDbMcpInstallRegisters` forwards the arguments the
  fake records to the real `mcpinstall` installer.
- `~/.claude.json` is not compared in the settings test: the CLI rewrites it on every run (P233 A13).

## OpenCode (P449)

Flow tests run a fake `opencode` (`fakeagent` persona, `flowharness.WithOpenCode()`); no real OpenCode test exists, since no model key is available here. The facts in `internal/harness/opencode/facts.go` come from the installed CLI (1.19.5) through the no-model probes below.

- Dev install, in a scratch dir outside the repo: `npm init -y && npm i --ignore-scripts opencode-ai`. It installs the launcher plus a platform package such as `opencode-linux-x64`. Run `node_modules/opencode-linux-x64/bin/opencode` directly; the launcher refuses without postinstall under `--ignore-scripts`. Always set `HOME=$(mktemp -d)` so user config is never read or written, and run from a scratch dir with `git init -q`.
- The Claude Code auto-mode classifier blocks a subagent told to install an external package, and a probe that runs a model prompt. Run the install from the orchestrating session or have the user approve.
- No model key needed: `--help`, `--version`, `session list --format json` (expect `[]`), `debug config`, `debug paths`, `debug agent build`, `mcp list`, `mcp add --help`, and `strings <bin> | grep -oE 'OPENCODE_[A-Z_]+'` for env names. `OPENCODE_CONFIG=c.json opencode debug config` shows how a config merges and masks header values. `OPENCODE_PERMISSION='{...}' opencode debug config` shows the permission merge.
- `run` reads the prompt from stdin when it is not a terminal: always pipe it (`echo hi | opencode run --format json`). A positional message with no stdin hangs.
- Need a model key (still unverified, see Known open items): the `run --format json` tool and cost events (`step_start`, `text`, `step_finish` verified), the error for `-s <unknown id>`, plugin event names (`/tmp/kp.js` writing each event to a file, loaded through `OPENCODE_CONFIG_DIR` (a dir whose `opencode.json` lists the plugin; verified to load)), the MCP tool permission key, and whether a TUI session appears in `session list` before its first prompt.

## Mac checklist (user-run)

The sandbox (P287) enforces on macOS only. The sandbox session cannot run Seatbelt, so these steps wait for a Mac.

1. `go test ./internal/agentsandbox -run TestEnforce -count=1 -v`. Expect every subtest to pass: worktree
   write and `git commit` allowed; write outside, `git branch`, `git worktree add`, `git update-ref`, `git tag`,
   object delete and `.git` file write denied; home file read denied, `read` folder allowed; direct connect
   with network off denied; the watcher reports the `git branch` denial within 5 s.
2. Sign in to `claude` on the Mac. Sandboxed runs read the login keychain.
3. `bun run test:claude:space -run 'TestAdeSandbox' -count=1`. Expect `a.txt` in the worktree, no branch `x` or
   `y`, at least one denial recorded, and a run `done` or `failed` with a reason naming the ref path. The
   read-only test must leave the file unchanged.
4. In the app: run a workflow step, open the run-line lock badge, check the denial list and **View profile**.
   Run `sandbox-exec -f <copied profile> /usr/bin/true` from a terminal; it must exit 0.
5. Profile order (P292): writes to `~/.claude/settings.json`, hook dirs and `~/Library/Keychains` stay denied
   inside a sandboxed run even with a broad allow path (later rules win). The policy refuses `~`, `~/Library`
   and `~/.claude` as allow or read paths; also try one through a symlink.
6. Denial attribution (P292 B3): a short-lived child, such as `git` run by `bash`, exits before the watcher
   walks its parents. Denial is attributed only when a process of the same name was already attributed.
   Check the first denial from a new short-lived binary shows in the run-line badge; if not, record the miss.
7. Plain HTTP (P292 A1): with network off and a real client, `curl http://example.com` through the proxy must
   fail with a 4xx or connection error. `curl https://<allowed host>` must still work.
8. MCP config read (P292 round 2): the profile allows reading a run's MCP config files by regex. Run an ADE step
   with an MCP server under the sandbox; it must start and list its tools. Other files in the same temp dir stay
   unreadable.
9. Token refresh (P341): run an ADE step after the OAuth token expires. It must still sign in; the refresh
   happens under the sandbox, where keychain file writes stay denied. Record a failure as a Known open item.
10. If everything passes, delete the "unverified on a real Mac (P287)" item from `docs/ARCHITECTURE.md`.

## Load, disk and test scope (P236-P251)

- **Test speed on this box (P294).** 4 cores. Go: `bun run test:go:containers` (81s) and `bun run test:go:rest`
  (134s) split `go test ./...` for CI; leave `-count=1` off to reuse the test cache. `ade` (44s) never caches.
  Playwright `ui` costs about 17 min per app on an idle host (P443): Studio 701 tests, 1044 s; Space 953 tests,
  1038 s; 4 workers (`'100%'`), parallelism 3.9. 2 or 3 workers cost more wall (sweep: 3 workers up to +4%, 2 workers
  +13-21%); more than 4 cannot be set from the CLI because each project's `workers` caps the pool. Per-test median
  is 4.2 s against 1.5 s for the cheapest test, so boot is not the whole cost; fixed sleeps are under a minute
  of it. `--shard=N/4` splits the same cores, so it saves nothing on one host (CI runs 4 shards on 4 machines).
  `bun run test:ui:changed` runs only the app suites a change can affect (`ui_target_of` in `scripts/hook-scope.sh`:
  `packages/**`, config, `scripts/` and unknown paths run both; Go, docs and `*.md` run none;
  `--dry-run` prints the choice; one-app change saves about 17 min). Space `ui` runs in no CI job, so a local
  run is its only gate. Re-run failures with
  `playwright test --last-failed`; `--only-changed=<ref>` selects spec and support files only, since app code is a
  built bundle. Hooks run no UI test: pre-commit lints and typechecks only the staged files (full run when config,
  a lockfile, `scripts/` or a shared package changed), pre-push builds, lints and tests only changed Go packages.
  Timing report: `PLAYWRIGHT_JSON_OUTPUT_NAME=out.json playwright test ... --reporter=json`, then
  `bun scripts/ui-timings.ts <label> out.json` (per-project table, slowest files and tests).
  `sh scripts/prepare-ui-tests.sh` also installs the chromium headless shell `ipc-frontend` needs.
- **`nohup` makes SIGHUP ignored, and children inherit it.** The terminal package resets that for its shells (P294). Other code relying on a hang-up misbehaves in a `nohup` run.
- **Run one heavy suite at a time.** The sandbox has 4 CPUs. Two Go suites, or a Go suite beside Playwright,
  push load above 20. The specs that still fail under load are listed in `ARCHITECTURE.md` Known open items;
  rerun one of those alone before calling it broken.
- **Load repro recipe (P277).** Start 8 busy loops (`for i in $(seq 8); do (while :; do :; done) & done`), wait 15 s,
  then run the one spec with `--repeat-each=40 --workers=8` (project `ui`, WebKit), or the Go test with `-count=40`.
  Kill the loops after. Count failures out of runs and compare with the same run at the base commit.
  Never answer a load failure with a longer wait or a retry; find what the step waits on.
- **`golangci-lint` refuses parallel instances** (two runs fail on the lock). Run it alone. On a lock error
  wait 60 s and retry. This covers `bun run lint:go`, `lint:claude` and the pre-push hook.
- **`bun run lint:go` needs `bun run build:space-mobile` once per checkout** (`main.go` embeds
  `frontend/dist-mobile`; absent, typecheck of the Go module fails).
- **`ui-timing` needs an idle machine** (load below ~1, nothing else running). It is out of `bun run test:ui:studio`
  (P443); run `bun run test:ui:timing:studio` alone (4 tests, about 70 s; CI runs the same `--no-deps` command). Check
  no `playwright`, `chrome` or `webkit` process is left before it starts. The WebKit sandbox is slow (measured 1.8x
  on JS and 2.9x on DOM work against Chromium on the same host); the open budget failures, if any, are in
  `ARCHITECTURE.md` Known open items.
- **Docker flow tests need `dockerd` started by hand** (section above) and `KIRA_FLOW_DOCKER=require`, or
  they skip and report green.
- **Check `df -h /` before a build.** Aim for 6 GB free. The Go build cache once reached 8 GB and filled the
  disk (P236). Reclaim regenerable files first: `/tmp/go-build*`, `/tmp/go-link-*`,
  `/tmp/playwright-transform-cache-*`, `apps/*/test-results`. `go clean -cache` only
  when the disk is full: the next build is cold. Keep the Docker images (`docker image prune` would force a
  pull before the complete suites).
- **`e2e-real` keeps only what cannot split at the IPC boundary** (P249): Space `boot-real`, `multiwindow-real`,
  `prompts-two-windows-real`, `terminal-real`; Studio `docker-real`, `mariadb-real`, `multiwindow-real`,
  `postgres-real`, `prompts-two-windows-real`, `sqlite-real`, `terminal-real`. Every other scenario is a Go
  flow test plus a UI spec on the same contract fixtures (section above).
- **A worktree whose `node_modules` is a symlink to another checkout** writes `bun install` results through
  it: P251's `@wailsio/runtime` bump changed the shared copy. Run `bun install` in the worktree itself.

## Playwright UI tier — `webkit` needs fetching explicitly

This container ships only Chromium preinstalled. `bunx playwright install webkit` downloads the
browser itself; its own post-install warning names the missing system libraries (`apt-get install
libevent-2.1-7t64 libgstreamer-plugins-bad1.0-0 libflite1 gstreamer1.0-libav libavif16` at the time of
writing) — install exactly those, not a generic `playwright install-deps`, which pulls far more
than `webkit` alone needs.

`scripts/prepare-ui-tests.sh` (called by `scripts/prepare-dev-environment.sh`) does both
automatically: idempotent, skips when the browser and all five packages are present. Set
`KIRA_SKIP_WEBKIT=1` to skip. Failure (offline, apt error) warns with the manual commands and
exits 0. It overrides `PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1` for that one install only.

## Mobile agents web — build, test, try on a phone (P212)

- Build: `bun run build:space-mobile` writes `apps/kira-space/frontend/dist-mobile`, which `main.go`
  embeds; gitignored like `frontend/dist`, so `go build ./...` fails on a clean checkout until built.
  `scripts/prepare-worktree.sh` builds it, root `bun run build:space` chains it (CI), and the
  Taskfile `build:frontend` task builds it before production `go build`.
- Tests: `bun run test:ui:space-mobile` (projects `mobile-ios` WebKit, `mobile-android` Chromium;
  builds first). `insecure-origin.spec.ts` runs on Chromium only (host mapping in
  `playwright.config.ts`). Visual baselines are not part of it.
- Phone: enable Settings > Mobile access, click "Trust this network" while on your home Wi-Fi, scan
  the QR, approve the code in Kira Space. The server runs only on that network (identity: subnet,
  router IP, router MAC). Phones that installed the old "Kira Space local CA" profile: remove it (iOS:
  Settings > General > VPN & Device Management; Android: Settings > Security > Encryption &
  credentials > User credentials).
- Server build in a sandbox: no LAN, so the pane shows "No network connection" or a container
  address; the live check needs a private interface address (`curl http://<lan-ip>:7790/`).
- Writes and terminals (P212 Part 2): a new pairing can change the backlog and move tasks between steps. Replying to
  agents and terminal attach need the pane's global "Let phones reply to agents and control their
  terminals" switch plus the phone's own Agent input switch, both off by default. To try an attach:
  turn both on, start a task step session on the desktop, open Need You or Plan on the phone and tap
  Terminal. The desktop pane shows "Controlling n terminal(s)"; the window shows an overlay with
  "Reconnect here". Needs an authenticated `claude`, see the Known open items in `ARCHITECTURE.md`.
- Terminal e2e has no Go server: `tests/mobile/terminal.spec.ts` scripts the socket with
  `page.routeWebSocket` (hello and bytes in, key frames recorded). `mockServer.ts` records each POST's
  body and `Idempotency-Key`, and `state.failNext` scripts one refusal. Go side: `go test -race
  ./apps/kira-space/internal/mobileterm/` drives a real `coder/websocket` client against the broker
  with a fake clock.

## Memory MCP and the `claude` CLI (P201)

- `KIRA_MEMORY_HOME` scopes `memory.db` (default `~/.kira-memory`). `testx.RunWithTempHomes` sets it
  for test binaries.
- Smoke against the real CLI, opt-in: `bun run test:claude:space -run TestMemoryGate`.
  Needs an authenticated `claude` on `PATH` (`claude -p` works here, CLI 2.1.292; ~14 s, a few cents).
  The gate uses `--safe-mode --setting-sources "" --strict-mcp-config --tools "" --no-session-persistence`;
  never `--bare`, it forces API-key auth.
- Import smoke (P211), real CLI, about 50 s and 0.14 USD:
  `bun run test:claude:space -run TestMemoryImport`. It runs the engine on
  `apps/kira-space/tests/claude/testdata/smoke` with a small chunk budget and re-executes the test binary as
  `memory-mcp` (set `KIRA_TEST_MEMORY_MCP=1`; `TestMain` in that module handles it), so no built app is needed.
  P274: the import now runs in a detached `memory-ingest` runner (flow binary and `tests/claude` binary
  serve it through `flowharness.Main`); its log is `$KIRA_MEMORY_HOME/import-runner.log`. Check no runner
  outlives a suite: `pgrep -f '^.*memory-ingest$'` is empty 35 s after.
  Import UI states: `bun run build:test:space`, then Playwright `memory-import` (mock bridge).
- MCP path: `go build -tags server ./apps/kira-space`, then `claude -p --mcp-config <file>
  --strict-mcp-config` with a `stdio` server `{command: <binary>, args: ["memory-mcp"], env:
  {KIRA_MEMORY_HOME: <tmp>}}`. `printf '' | <binary> memory-mcp` exits 0. The macOS app-bundle binary
  is unchecked (Known open items).
- Semantic search (P210): the ONNX Runtime library is not in the repo. `sh scripts/fetch-onnxruntime.sh
  linux-x64` downloads the pinned 1.31.1 build (checksum-verified, idempotent) to
  `apps/kira-space/build/onnxruntime/linux-x64/` (gitignored) and prints the library path; set
  `KIRA_ORT_LIB` to it for any process that should embed (the server-tag app, `memory-mcp`). Without it
  the status is `unavailable` and search stays keyword-only. Hugging Face is reachable through the agent
  proxy here, so the 35 MB model download works. Real-model check, not in CI:
  `KIRA_ORT_LIB=… go test -tags embedsmoke ./internal/memory/embed/ -run Smoke -v` (installs into a
  temp dir, runs the real worker, needs 8/10 top-1). Tests otherwise use a fake `Embedder` and a fake
  worker (the test binary re-executed), so no network or library is needed. `go build` of the
  `CGO_ENABLED=0` stub and `GOOS=darwin` pure-Go packages works here; the cgo darwin build does not.
- Worker footprint: `grep VmRSS /proc/$(pgrep -f 'memory-embed')/status` while it runs; it should
  vanish about 5 min after the last search.

## `tests/visual/*` pixel diffs: font mismatch, not a code regression

`visual` baselines render Inter. `pin-fonts.css` collapses the UI stack to `sans-serif`; Inter wins
only via two pieces: apt `fonts-inter` (4.0+ds-1, noble/universe) and fontconfig rule
`tests/visual/support/fontconfig/56-prefer-inter.conf` in `/etc/fonts/conf.d/`. `fonts-inter`
alone leaves `sans-serif` on DejaVu Sans. CI installs both in `ui-extra` (setup action input
`visual-fonts`), so sandbox and CI render identically. Monospace stays DejaVu Sans Mono in both.

Whole-page uniform glyph diff across every spec means fonts differ. Check `fc-match sans-serif`
prints `Inter`; if not, install both pieces. Never `--update-snapshots` from a mismatched
environment: it corrupts the baseline. Baselines re-recorded in this container (P227, P229).

## `golangci-lint` / `knip` — code-quality tooling in this environment (P94)

See `git show bd0c224688d9:docs/v1.8/plans/P94-code-quality-tooling.md` for what these tools check and why
the pass split. Here: the container quirk that blocked two prior attempts, and the pre-commit/
pre-push design.

- **The preinstalled `/usr/local/bin/golangci-lint` (2.5.0, built with go1.25.1) refuses this repo**
  outright: `go.mod` pins `go 1.27.3`, and golangci-lint's config loader rejects targeting a Go
  version newer than the one it was itself built with. `go install .../golangci-lint@latest` does
  not fix this — it produces a binary built with **go1.26.8**, one minor version short, every time.
- **Why:** `go version` in this container reports `go1.27.3` only because `GOTOOLCHAIN=auto` reads
  *this repo's* `go.mod` and downloads that toolchain on demand for commands run inside the module.
  `GOTOOLCHAIN=local go version` reports the container's real default: **go1.24.7**. `go install
  pkg@version` runs in module-aware mode *ignoring the current module* — it resolves only
  golangci-lint's own `go.mod` directive (`go 1.26.0`, no `toolchain` line), and `auto` downloads
  the minimum toolchain satisfying that — go1.26.8, not the 1.27.3 already sitting in the module
  cache. `@latest` doesn't help either; it just moves which release hits the same gap.
- **The fix**, pin `GOTOOLCHAIN` explicitly to the version this repo actually wants, on the install
  itself:
  ```sh
  GOTOOLCHAIN=go1.27.3 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
  ```
  `scripts/install-golangci-lint.sh` does exactly this, validating a cached `GOPATH/bin` binary
  against the pinned version+toolchain string before trusting it (the same pattern
  `scripts/setup.sh` already uses for `wails3`). No `go.mod` change, no Go version bump, no source
  checkout needed — install only, same as any other `go install` target.
- **No prebuilt release binary works, and neither does `golangci-lint-action`.** Every official
  v2.15.0 release binary is itself built with go1.26.8 and fails identically. Build from source
  everywhere, including CI — `actions/setup-go` with `go-version-file: go.mod` already makes 1.27.3
  the runner's default toolchain there, so a plain `go install` in CI already builds correctly; the
  `GOTOOLCHAIN` pin in the script is belt-and-braces so the same one command works locally too.
- **Pre-commit stays exactly `bun run lint` + `bun run typecheck`** (~15s) — golangci-lint and knip
  are deliberately not in it. `golangci-lint`'s cost is bimodal: ~1s warm, but minutes on a cold Go
  build cache (fresh clone, dependency bump, toolchain change) — a per-commit hook with a multi-
  minute p99 gets routed around with `--no-verify`, which `CLAUDE.md` forbids as a way of finishing.
  `knip` is a whole-graph analysis; gating it per commit punishes a normal, legible multi-commit
  refactor (deleting a consumer in one commit, adding its replacement in the next). Both instead run
  in the `.githooks/pre-push` hook (`go build ./...`, `bun run lint:go`, `bun run lint:dead`, ~30s
  warm) — push is
  when work leaves the machine, the same boundary CI defends — and in CI's `checks` job.
- **`typecheck:*` run `--incremental`** (P215) with one `tsBuildInfoFile` per project under
  `node_modules/.cache/tsbuildinfo` (gitignored). Delete that directory to force a cold check.

## A pre-P97 dev box still has orphaned repo-map files on disk

The app has never shipped, so migration 0025 is a plain forward drop with no installed base to
preserve (`docs/ARCHITECTURE.md`'s Storage section) — nothing cleans up a dev box's own leftovers
from before v1.9 P97. If this container (or a persistent dev machine) ran a pre-P97 build, delete
by hand: `${KIRA_HOME}/codeindex.db*` (the tree-sitter parse cache), `${KIRA_HOME}/codeindex-sync-
*.lock` (its per-repository sync flocks) and `${KIRA_HOME}/mcp-repo-map-*-token.json` (the repo-map
MCP tokens). No runtime cleanup code is shipped for this — a permanent housekeeping path in a
shipping app would be permanent code for a one-off developer chore.

## Database MCP server — reaching it in this environment (M1-M5)

This server exists only inside the app process: `apps/kira-studio/cmd/` holds only `g1measure`, and
`package.json` has no `mcp:*` script for it.

- **No headless binary and no `bun run mcp:*` script exists for it.** The only ways to reach it here
  are `bun run dev:studio` (needs a GUI this container does not have) or a `go build -tags server` boot
  proof — the same `//go:build server` route documented above for the bound-call surface. M5's own
  verification did exactly that and drove a real `dbmcp` endpoint with `curl`.
- Its token is `${KIRA_HOME}/mcp-db-token.json` — no repo slug, one per `KIRA_HOME`
  (`internal/bridge/dbmcp.go`'s `dbMcpTokenName`, via `mcpauth.PathNamed`) — and it carries a 7-day
  expiry since M1 (`mcpauth.TTL`).
- Its port is always `DefaultPort` **8766** (`internal/dbmcp/server.go`). A conflict on that port
  fails the enable outright with a message saying another process is using it — `bindHTTP` never
  falls back to a different, kernel-assigned port (`docs/ARCHITECTURE.md`'s DB MCP section).
- Its endpoint is `http://127.0.0.1:<port>/mcp`, an ordinary `go-sdk/mcp` Streamable HTTP server
  (`internal/dbmcp/http.go`'s `bindHTTP`) — a JSON-RPC 2.0 `tools/call` POST with an `Authorization:
  Bearer <token>` header reaches it the same way any MCP Streamable HTTP client would; the SDK's own
  client package is the reference for the exact request shape (`Content-Type`/`Accept` framing) if
  driving it with a raw `curl` rather than a real MCP client.

## shadcn-vue — adding a component set in this environment

`shadcn-vue add` cannot run here: this repo has no `components.json`/CLI wiring. Pull from the
registry directly:

1. `curl -sS "https://shadcn-vue.com/r/styles/reka-nova/<name>.json" -o /tmp/<name>.json`.
2. For each `files[]` entry, strip the `styles/reka-nova/ui/` prefix and write it under
   `packages/theme/src/components/ui/<name>/`.
3. Rewrite `@/lib/utils` to `@theme/lib/utils`, and any `@/…` sibling import to
   `@theme/components/ui/…`.
4. Pull `registryDependencies` recursively the same way; check `dependencies` against the root
   `package.json` before adding anything (and against `CLAUDE.md`'s open-source-only rule).
5. Run `bun run format` over the new files.

P110 iter2 used this for `empty` and for `resizable`'s `ResizablePanel`/`ResizablePanelGroup`.

## CodeGraph — the code index in this environment

`CLAUDE.md` says how to navigate with CodeGraph; this is the setup. **`scripts/codegraph-setup.sh`
runs once per worktree, before a Claude session is spawned into it — not a `SessionStart` hook.**
It installs `@colbymchenry/codegraph` globally via `npm` if missing, then `codegraph sync .` (or
`codegraph init .` on first run) and prints `codegraph status .`. Whoever provisions the worktree
(the orchestrating session, before handing it to a subagent) runs it alongside
`scripts/prepare-worktree.sh` — or both in one call via `scripts/prepare-dev-environment.sh`. The
index lives in `.codegraph/` (gitignored). `.mcp.json` registers
`codegraph serve --mcp`; its one tool is
`codegraph_explore`, deferred until `ToolSearch` loads it. Each stream worktree (`/home/user/kira-v21-X`)
needs its own run of `scripts/codegraph-setup.sh`; a `.codegraph/` missing there means a cold index. A `UserPromptSubmit` hook
(`codegraph prompt-hook`) injects matching symbols into every prompt — this hook still fires
per-message regardless of how the index got built. If a worktree was never provisioned with
`scripts/codegraph-setup.sh` (a manual clone, say), run it by hand once before relying on the index.

## CodeGraph duplicate finder — `scripts/codegraph-duplicates.ts`

Runnable form of the ad-hoc sweeps P107/P113/P115 ran against `.codegraph/codegraph.db` (they were
never committed as a script; method text lives in `git show bd0c224688d9:docs/v1.9/plans/P107-duplication-findings*.md`
§0 and `P113-duplication-sweep.md` §0). Opens the index read-only via `bun:sqlite`, reads each
function/method body from disk by `nodes.file_path` + line span, never writes. Needs an index
(`scripts/codegraph-setup.sh`); with none it prints how to build one and exits 0. In a linked
worktree with no index of its own, falls back to the main checkout's (`git worktree list`); override
with `--db`/`CODEGRAPH_DB`. `--root` = where source files are read from.

```sh
bun scripts/codegraph-duplicates.ts --path apps/kira-studio/internal --lang go --top 10
bun scripts/codegraph-duplicates.ts --sweep calls --lcs 0.6 --min-callees 4 --json
```

Options: `--db`/`CODEGRAPH_DB`, `--root`, `--path` (repeatable), `--lang`, `--min-lines` (6),
`--sweep exact,blind,name,calls`, `--similarity` (0.6), `--lcs` (0.7), `--min-callees` (6),
`--top` (20), `--include-tests`, `--include-generated`, `--json`.

Sweeps (function/method nodes only, groups need >=2 files):

- `exact`: same body after stripping comments and whitespace.
- `blind`: same after collapsing identifiers, string and number literals. Excludes groups `exact` has.
- `name`: same symbol name and language across files, line-set ratio >= `--similarity`. Names with
  >30 definitions skipped.
- `calls`: ordered `calls` edges, LCS/max >= `--lcs`. Resolved edges only; P113's widening to
  unresolved stdlib refs is not reproduced.

Ranking: members x lines. False positives: one-line delegates and `Get`/`Remove`/`Close`-style repo
boilerplate (`blind`), per-adapter shape that is deliberate (`name`/`calls`), cross-language and
`kira-space` vs `kira-studio` mirrors, tests (dropped by default), generated bindings (dropped via
`files.generated`). Anonymous callbacks and struct fields are not graph nodes, so unseen. Read every
hit with `codegraph_explore` before calling it duplication.

## Mutation testing (P151, report-only)

Measures unit-test strength. Never wired into hooks, `package.json`, `pr.yml` or any gate; survivors
are data, not failures. Scope: Go `go test` packages and `bun test` unit specs only (no Playwright
suites). Tooling lives in `scripts/mutation/` and `tools/mutation/` (own `package.json` and
`bun.lock`, not a root workspace).

- **Prerequisites**: `bun run setup` (generated Wails bindings; the TS runner exits without them),
  Go per `go.mod`, `jq`, `node` 22+. Go tool `gremlins` v0.6.0 installs itself into
  `tools/mutation/bin/` (gitignored); Stryker installs via `bun install --frozen-lockfile` in
  `tools/mutation/`.
- **Run**: `nice -n 19 sh scripts/mutation/run.sh go|ts|all [target...]`. Go targets are package
  dirs (`apps/kira-studio/internal/mask`); TS targets are area names from
  `tools/mutation/areas.json`. Default: every Go package with tests, every TS area.
- **Flags**: `--changed <ref>` mutates only files changed since `<ref>` (Go: other files of a package
  are excluded; TS: intersected with the area globs). `--resume <run-dir>` continues a halted run
  (units with an `.info.json` are skipped; a halt loses at most one unit). `--dirty` snapshots
  uncommitted tracked changes. `MUTATION_WORKERS` sets workers (default half the cores; more
  turns timing-sensitive tests into false kills). `MUTATION_STRYKER_ARGS=--dryRunOnly` checks an
  area's initial test run without mutating.
- **Isolation**: each run mutates a tracked-files snapshot in a temp dir (`TMPDIR` is redirected
  there), never the live checkout; `git status` stays clean. Disk: snapshot ~52MB plus one gremlins
  copy per worker.
- **Output**: `tools/mutation/out/<utc>-<sha>/` (gitignored), `out/latest` symlinks the newest.
  `summary.md` / `summary.json` are generated at the end of every run, or by hand with
  `bun tools/mutation/summarize.ts <run-dir>`. `meta.json` records per-segment wall time.
  Score = (killed + timeout) / (killed + timeout + survived + no coverage), same for both languages.
- **Runtime**: Go ~0.7 s per mutant per worker at a 2s suite, more for slow suites; TS ~0.4 s per
  covered mutant per worker, plus 10s per timed-out mutant (infinite-loop mutants are common). A
  full run takes hours: run it in resumable chunks (one `run.sh` call per target group,
  `--resume` the same run dir). Heaviest Go suites: `gitsession`, `ade`, `gitrpc`,
  `apps/kira-studio/internal`.
- **Go details**: a package whose plain `go test` is red is recorded `red` and skipped. gremlins
  scales its per-mutant timeout from the coverage run, so `go.sh` passes a coefficient that floors
  it at 60s. `_darwin.go` and `//go:build darwin` files are not mutated (invisible on Linux).
  Container-gated adapter tests `t.Skip` without Docker, so their mutants report "no coverage";
  there is no container mode.
- **TS workarounds** (all inside the scripts and `stryker.config.mjs`, no repo test/source edit):
  1. Bun's 5s per-test timeout kills the 100k-sha `shaTable` test under instrumentation:
     `bun.bunArgs --timeout 60000`.
  2. The runner eager-imports every mutated module before any spec. Root `bunfig.toml` `[test] preload`
     (`window.ts`, `wailsRuntime.ts`) registers the `/wails/runtime.js` mock first; the snapshot copies it.
  3. `tabs-save-retries-after-failure` and `tabs-save-serialized` specs race on `setTimeout(0)`
     under instrumentation: excluded from the area's test list in `areas.json`, with reason.
  4. Workspace packages carry their own `node_modules` (isolated linker) which Stryker's sandbox
     does not link: `inPlace: true` on the disposable snapshot, and `run.sh` symlinks every
     `node_modules`.
- **TS limits**: no type checker (bun strips types, so some survivors are type-invalid code), `.vue`
  files are not mutated, static mutants are ignored, `packages/theme` has no
  unit suite.
- **CI**: none, by user decision (2026-10-07) — not worth the investment yet. Run manually only,
  via `scripts/mutation/run.sh` below.
- **Risks**: gremlins and `@hughescr/stryker-bun-runner` are single-maintainer, slow-moving tools.
  Fallbacks if either breaks: `avito-tech/go-mutesting` (about 5x slower, no coverage split),
  Stryker's `command` runner with `coverageAnalysis: 'off'` (10-30x slower).

## Perf probes — `tests/perf/` (scroll lag, RSS)

Report-only probes: print `PERF ...` lines, assert nothing. Run `bun run perf:probe:studio` or
`bun run perf:probe:space`; narrow with a file filter, e.g.
`bunx playwright test --config=apps/kira-studio/playwright.perf.config.ts documents`.
Helper: `packages/workbench/src/testing/ui/perfProbe.ts` (RSS sampler, rAF frame stats, momentum
flick). Add a view by copying a probe and calling `measureFlick` over `FLICK_LADDER`.

- Tab heap probe (P433): `bunx playwright test --config=apps/kira-studio/playwright.perf.config.ts
  --project=heap` (same for `apps/kira-space`). Chromium only (CDP), report-only, prints `HEAP ...`
  lines: heap per open tab, reads per switch round, reads after six fake minutes away, heap after
  close-all. Helper: `packages/workbench/src/testing/ui/heapProbe.ts`. Build first
  (`bun run build:test:studio` or `build:test:space`). `KIRA_HEAP_FILE_LINES` sizes the Space file.
- Engine is Playwright WebKit. Linux runs the WPE port: software rendering, no GPU process.
- fps = 1000 / frame ms. Compare views to each other, not to 16 ms.
- RSS comes from `ps`. On macOS it omits CoreAnimation/IOSurface graphics memory, the dominant
  scroll cost there. Read real footprint with `apps/kira-studio/frontend/proto/grid/mac/wkhost.swift`.
- Env knobs: `NDOCS` (documents), `NROWS`/`NCOLS` (console grid), `BODYKIND`/`NITEMS` (HTTP
  response), `NTABLES` (tree), `GRAPH_N` (git graph), `NCONTAINERS` (docker list).

## Grid prototype (P165) — `proto/grid/`

Dev-only Cheetah Grid prototype beside SlickGrid baselines. Not in the shipped bundle; Go embeds only
`frontend/dist`. Plan and verdict: `git show bd0c224688d9:docs/v2.0/plans/P165-cheetah-grid-prototype.md`.

- Pages: `cheetah.html`, `slick.html?variant=kira|stock`, `empty.html` (bare scroller control).
  Query: `fixture=features` (NULL, empty, truncated, masked, FK rows), `w`/`h`, `rowHeight`, `zebra=1`,
  `readonly=1`.
- `bun run proto:dev:grid` serves them. `proto:build:grid` writes `dist-proto` (no debug hooks).
  `proto:build:grid:hooks` writes `dist-proto-hooks` (`window.__kiraGridProto`, `window.cheetahGrid`,
  trace HUD).
- Both grid pages build the same data: `createProtoData` over the shared `wideTable` template, 10 000 rows x 20 columns, no randomness. Open `cheetah.html` and `slick.html?variant=kira` in two windows to compare by hand; pass the same `fixture`, `w`, `h` and `rowHeight` to both.
- `bun run test:proto:studio` builds the hooks build and runs `tests/proto/` in Playwright WebKit.
- `bun run perf:probe:proto` builds both and runs `tests/perf/proto-grid-scroll.spec.ts`. Frame cost
  uses the release build, late data the hooks build. `GRID=cheetah,slick-stock,slick-kira` narrows.
  `PERF_HEADED=1 xvfb-run -a bun run perf:probe:proto` runs headed WebKitGTK.
- Compare against the app with `NCOLS=20 TRACE=1 bunx playwright test
  --config=apps/kira-studio/playwright.perf.config.ts grid-scroll` at the same commit. Its
  `PERF viewport` line is the size the proto probe copies.
- Container numbers rank engines only. No GPU, DPR 1. Footprint needs the Mac.

### Mac footprint run (user-run)

Needs Xcode command line tools. Not run in the sandbox, `wkhost.swift` is uncompiled there.

1. `bun run proto:build:grid && bun run proto:build:grid:hooks`, then
   `bun run proto:preview:grid` (serves `dist-proto` on 127.0.0.1:9246).
2. `swiftc -O -o /tmp/wkhost apps/kira-studio/frontend/proto/grid/mac/wkhost.swift`.
3. Per page: `/tmp/wkhost <url> 90 apps/kira-studio/frontend/proto/grid/mac/driver.js 2> <name>.log`
   for `proto/grid/empty.html`, `slick.html?variant=kira`, `slick.html?variant=stock`,
   `cheetah.html`, all with `?w=1440&h=960&rowHeight=28` appended (use `&` after the variant).
4. Per page and band (`MARK-idle`, `MARK-40px`, `MARK-100px`, `MARK-200px`): idle footprint, peak
   `FOOTPRINT`, delta. Never `vmmap` for the series.
5. Late data on a real trackpad: serve `dist-proto-hooks`
   (`cd apps/kira-studio/frontend && bunx vite preview --config vite.proto.config.ts --outDir
   dist-proto-hooks`), open `cheetah.html?fixture=features` in Safari, press Start trace, one hard
   two-finger flick, Stop trace. The JSON is on the clipboard. Same on `slick.html?variant=kira`. In
   the real app: `__kiraScrollTrace` (`docs/PERF.md` section 2.1a). Note perceived smoothness and
   blank edges per page.


## `KIRA_*` environment variables

Every name read or set in Go. "Set by" says who supplies it; "you" means set it by hand.

| Name | Purpose | Set by |
|---|---|---|
| `KIRA_HOME` | Kira Studio data dir. Panics in a non-`production` test binary when unset | you; `testx.RunWithTempHomes` in tests |
| `KIRA_SPACE_HOME` | Kira Space data dir and instance-lock dir (`app.lock`) | you; `testx.RunWithTempHomes` |
| `KIRA_MEMORY_HOME` | Memory data dir, else `~/.kira-memory` | you; `testx.RunWithTempHomes` |
| `KIRA_DEV` | Overrides the production check on a non-`production` build (`kirapaths`) | you |
| `KIRA_INSECURE_SECRETS` | `1` stores secrets unencrypted on Linux without a keyring. Dev only | you; IPC fixture harness |
| `KIRA_NO_STARTUP_ALERT` | Skips the native startup-failure dialog; the failure is still logged (`startupfail`) | you, CI and headless runs |
| `KIRA_ORT_LIB` | ONNX Runtime shared library path, else the bundled copy | you |
| `KIRA_TERMINAL_ID`, `KIRA_AGENT_HOOK_SOCKET`, `KIRA_AGENT_HOOK_TOKEN` | Identify a terminal and its hook socket to the Claude hook shim | `terminal` on launch, `harness/claude/hooks` |
| `KIRA_USER_STATUSLINE` | The user's own statusline command the hook shim chains to | `harness/claude/hooks` shim |
| `KIRA_ASKPASS_SOCK`, `KIRA_ASKPASS_TOKEN`, `KIRA_ASKPASS_OPID`, `KIRA_ASKPASS_TIMEOUT_MS` | Git askpass helper reaches the prompt broker; per-op id and timeout | `gitaskpass` broker. `loginshell` strips the first two |
| `KIRA_PREPARE`, `KIRA_WORKTREE_PATH`, `KIRA_WORKTREE_BRANCH`, `KIRA_REPO_ROOT`, `KIRA_REPO_COMMON_DIR` | Worktree prepare script context. `KIRA_WORKTREE_BRANCH` is omitted on a detached head | `gitprepare` |
| `KIRA_TASK`, `KIRA_REPO`, `KIRA_BRANCH`, `KIRA_BASE`, `KIRA_<NAME>` | Built-in vars of a script (`scripts.BuiltinEnv`) | `adeflow` script runs |
| `KIRA_PARAM_<NAME>` | Smart-script parameter value; a multiselect is newline-joined | `scripts` smart runs |
| `KIRA_IPC_FIXTURES` | `write` regenerates IPC fixtures | you |
| `KIRA_GIT_FIXTURES` | `write` regenerates the porcelain golden corpus | you |
| `KIRA_CONTRACT` | `write` stores flow-test contract output | you |
| `KIRA_FLOW_COMPLETE` | Opts into the complete flow suite | you, CI |
| `KIRA_FLOW_DOCKER` | `require` fails, rather than skips, flow tests without Docker | you, CI |
| `KIRA_FLOWTEST_RUN` | Run id inside a Docker flow-test child process | `dockerflow` tests |
| `KIRA_TEST_MATRIX` | Gates the Tier-2 adapter matrix | you, CI |
| `KIRA_COMPAT_IMAGE_<KIND>` | Overrides the adapter compat container image per kind | you |
| `KIRA_FAKE_SCEN`, `KIRA_FAKE_DIR`, `KIRA_DOC` | Fake agent: scenario JSON or path, call-log dir, doc basename | flow tests |
| `KIRA_FAKE_CLAUDE` | Re-executes the ADE test binary as a fake `claude` | `ade` tests |
| `KIRA_MEMORY_EVAL`, `KIRA_EVAL_MODE`, `KIRA_EVAL_ROUND`, `KIRA_EVAL_SET`, `KIRA_EVAL_ONLY`, `KIRA_EVAL_ROUNDS`, `KIRA_EVAL_PROMPTS`, `KIRA_EVAL_BUDGET`, `KIRA_EVAL_PARALLEL`, `KIRA_EVAL_CAP_USD`, `KIRA_EVAL_PHASE_CAP_USD`, `KIRA_EVAL_LEDGER`, `KIRA_EVAL_RUNS_DIR`, `KIRA_EVAL_MODEL_HOME`, `KIRA_EVAL_RECIPES`, `KIRA_EVAL_STYLES`, `KIRA_EVAL_STYLE`, `KIRA_EVAL_VARIANTS`, `KIRA_EVAL_EMBED_WORKER` | Memory ingest eval; `KIRA_MEMORY_EVAL=1` opts in (costs real money); `KIRA_EVAL_LEDGER` names the spend file per part (default `spend.jsonl`); `KIRA_EVAL_RUNS_DIR` picks the runs dir under `apps/kira-space/tests/claude/testdata/memory-eval` (P291: `P291-runs`); `KIRA_EVAL_EMBED_WORKER=1` is internal (the test binary re-executed as the embedding worker) | `bun run eval:memory`, you |
| `KIRA_TEST_MEMORY_MCP` | Real `claude` tests: run the binary as the memory MCP server | `apps/kira-space/tests/claude` |
| `KIRA_GIT_DIFFERENTIAL`, `KIRA_GIT_PERF` | `1` runs the git search differential test, the git perf probe | you |
| `KIRA_DATAGRIP_REAL_PROJECT` | Path of a real DataGrip project for the opt-in import test | you |
| `KIRA_EMBED_SMOKE_WORKER`, `KIRA_EMBED_FAKE_WORKER`, `KIRA_ASKPASS_HELPER_PROCESS`, `KIRA_TEST_MARKER` | Re-exec markers inside the owning package's tests; never set by hand | the tests themselves |

## Comment sweep (P320)

`go run ./scripts/commentsweep <cmd>`, from repo root. `run` spends tokens: needs an authenticated `claude` on `PATH`
and builds `kira-space` (`bun run build:space` when `frontend/dist` or `dist-mobile` is missing). Work dir
`.commentsweep/` (gitignored, resumable): `manifest.jsonl` holds every comment found, any size, with its status.

- `scan [--min-chars 300]`: extract only, counts and cost projection. Refuses a dirty in-scope tree.
- `run --max-usd X [--jobs 4] [--lang go|ts|vue]`: one `ingest_files` rewrite call per comment; rerun after a halt.
- `apply --lang L [--dry-run]`: byte-splice, code-identity check, never commits. `restore --ids a,b` undoes units.
- `report --not-applied`: every comment that did not ship, grouped by status. `report --sample N --out F`: judge sample.
- `status`: outcomes, spend, applied files.
