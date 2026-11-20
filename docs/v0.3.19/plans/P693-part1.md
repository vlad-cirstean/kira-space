# P693 Part 1: agents module RAM, audit method

Planner: Opus. Implementer: one sequential Sonnet subagent, cold start. Output: probe extensions (one
commit) and `docs/v0.3.19/plans/P693-audit.md` (one commit). No product source change. No fix.

Read first: `CLAUDE.md`, `docs/WORKING_AGREEMENT.md`, `docs/v0.3.19/SPEC.md` § P693, `docs/PERF.md`
§2.15, §2.16, §3, `docs/DEV_ENVIRONMENT.md` "Perf probes" and "Agents module footprint run",
`docs/ARCHITECTURE.md` Known open item "ADE first mount stays retained after leaving ADE".

Grounding: CodeGraph (`codegraph_explore`) on the main checkout index (`/home/user/kira-space`, a few
commits behind this worktree; the worktree has no `.codegraph/`), re-checked in the worktree by grep.
Line numbers below are from the worktree at `1e3e82c4c`; re-grep before relying on one.

## 1. Decisions on the open questions

D1. Record figure and platform.
- macOS figures (`footprint`, status bar App/Agents/Scripts from `internal/metrics/sampler.go`
  `Sample`) cannot be taken here. This sandbox is Linux: Playwright Chromium (CDP: JS heap, DOM
  counters, heap snapshots) and Playwright WebKit (WPE: process PSS only, no GPU process).
- Figure of record for P693, both parts: **Kira-owned PSS** in the agents fixture, sum of
  (a) renderer: WebKit WebContent PSS, `ade-agents` world, checkpoint `final`, `heap-webkit` project;
  (b) Go app: app process PSS, Go probe (§4.4), checkpoint `idle60`;
  (c) Kira-started helpers: per-session login shell, `git cat-file` and other git children, the
  `js-worker` child, each from the Go probe process table at `idle60`.
  `claude` processes are excluded from the record (user's CLI). They get their own line, never a
  share of the record.
- Secondary figures, for attribution only: Chromium JS heap after two GCs (`RAM ... heapMb`) and Go
  `inuse_space` after `runtime.GC()` twice.
- Limits, written into the audit verbatim: PSS splits shared pages across processes, macOS
  `phys_footprint` counts private dirty plus compressed and excludes clean shared; V8 heap sizes are
  not JSC heap sizes; WPE has no GPU process and no IOSurface memory, macOS WebContent does; (a) and
  (b)+(c) come from two harnesses with the same fixture shape, not one live process set; the Go probe
  app is the test binary (harness code in its baseline, cancelled by checkpoint deltas).
- User's own number: the audit gets a "User figure" slot. The orchestrator asks the user once
  whether a Mac "Agents module footprint run" (`docs/DEV_ENVIRONMENT.md`) exists. If supplied before
  the audit is committed, record it, map each component (App to (b)+(c), WebContent to (a), Agents
  to `claude` plus shells) and state the proxy-to-Mac ratio per component. If not, the audit says
  "not supplied" and proceeds. Never block on it.

D2. ade20 retained tree: in scope for Part 1 attribution, fix gated. Part 1 runs §5.3 (WebKit check
plus Pass B retainer walk). Part 2 gets a fix item only if (1) WebKit WebContent PSS at `back` minus
`board` exceeds noise (§6.3), or the user's Mac figure implicates it, and (2) Pass B names the owning
component. Else Part 2 restates the Known open item with the new numbers. If WebKit shows no step,
the audit records the retention as Chromium-only (Blink `DocumentState` path) and drops it.

D3. `claude` share: out of scope for reduction. Kira cannot shrink the user's CLI. A session cap or
idle-stop policy is a behavior change the user has not asked for; the audit lists it as "user
decision, not in Part 2" with the measured per-session cost Kira adds around each `claude` (shell,
xterm buffer, Go buffers). Never run real `claude`: no token spend is justified here, since its
footprint is not Kira-fixable.

D4. Budget row for `docs/PERF.md` §1: Part 1 proposes the row text in the audit (metric, world,
checkpoint, figure, threshold = Part 2 target). Part 2 adds it.

## 2. Base commit and host

- Base: the commit holding the probe extensions (§3). It changes no product source, so the numbers
  equal the product tip under it. Record both: `git rev-parse HEAD` after the probe commit, and its
  parent (product tip, expected after P692 and its guard-renumber commit). Never measure on a tree
  with uncommitted changes (`git status --porcelain` empty, except the audit file).
- Host record: `nproc`, `uname -r`, `go version`, Playwright browser revisions
  (`bunx playwright --version`, `ls /opt/pw-browsers`), `free -m`.
- Load gate, every run: before starting, loop `cat /proc/loadavg` every 10 s until `load1 <= 1.0`
  (max 15 min), and `pgrep -af 'playwright|go test|vite|vitest'` shows no process outside this run.
  The probe's own gate (`waitForQuietHost`, CPU busy <= 25 %) also runs and prints
  `RAMLOAD busy=.. load1=..`. Never set `KIRA_RAM_NOGATE`. Discard and rerun any repeat whose
  `RAMLOAD` reads `load1 > 1.0`; note the discard in the audit. Another agent shares this host: if
  the gate never passes in 15 min, wait and retry, never lower the gate.
- Repeats: 3 independent repeats per world and engine. Report median [spread], spread = max - min.

## 3. Probe extensions (one commit, before any measured run)

Allowed: extend existing probes and debug hooks; add one env-gated Go probe test. Not allowed: a
parallel tool, a product behavior change, a committed experiment. Debug hooks stay behind
`__KIRA_DEBUG_HOOKS__` (pattern: `frontend/src/studio/debugHooks.ts:215`), so a shipped build has
none. Commit: `test(perf): agents RAM probe checkpoints and Go probe`. Lint, typecheck, `go vet` and
the pre-commit hook pass clean.

E1. `apps/kira-space/tests/perf/heap-ram.spec.ts`, new world `ade-idle`: `agentsBoard()` (10 live,
200 archived), `IPC.adeTaskSessions` answering `{ sessions: [] }`, no launches. Checkpoints `board`,
`idle60` (`page.waitForTimeout(60_000)`), `left` (mode `git`), `back` (mode `ade`). Each with the
same `pss(...)` `after` hook as `ade-agents`.

E2. `ade` world (`ade20`, `ade200`): add the `pss(...)` `after` hook to every checkpoint, so the
`heap-webkit` project yields WebContent PSS at `board`, `task`, `left`, `back`. Move `pss`/`engine`
to a small helper in `tests/perf/support/ramWorlds.ts` shared by all three ADE worlds.

E3. `ade-agents` world, new checkpoints appended after `approval`, in this order:
- `log3m`: open the panel of a task with a run, emit 3 MB of run log chunks on the log push channel
  (find it from `installAdeReadSignals` `onLog` and the desktop signal source) in about 30 pushes, the
  run log visible (`[data-testid="ade-run-log"]`).
- `files1`: open the task Files tab and one file (P691; reuse the IPC mocks of the P691 UI spec under
  `tests/ui/`). Skip this checkpoint with a logged reason if no mock fits in one attempt.
- `stopped4`: emit a sessions push where the 4 running sessions are stopped (`state: 'stopped'`,
  `terminalId: ''`), wait 2 s: proves the `adeTerminals` reap frees xterm instances.
- `left` and `back`: mode `git`, then `ade`, as in the `ade` world.
`final` stays the record checkpoint; new checkpoints come after it so earlier numbers stay
comparable with P661.

E4. Introspection (`packages/workbench/src/testing/ui/memProbe.ts` `INTROSPECT_FN`), extend the
`RAM` record:
- `queryBytes`: per query key family (first three key parts joined by `/`), count and summed
  `JSON.stringify(state.data).length` (try/catch per query; a failure counts as `-1`). Families
  needed: `adetask/board`, `adetask/sessions`, `adetask/log/*`, `adetask/panelForward`,
  `adetask/archived`, everything else summed as `other`.
- `storeBytes`: per Pinia store id, `JSON.stringify` length of its state (try/catch, `-1` on cycle).
- `hooks.__kiraTerminalStats`: new debug hook registered from
  `packages/workbench/src/terminal/terminalRenderer.ts` (or the app's debug hook module if a package
  file cannot see `__KIRA_DEBUG_HOOKS__`; follow how `__kiraRetention` is reached): per entry of the
  renderer's `byTabId` map, `{ id, opened, cols, rows, bufferLines: term.buffer.normal.length,
  scrollback: term.options.scrollback }`, plus the terminals store drain bytes total.
These are byte estimates of retained data, not heap truth; Pass B gives heap truth.

E5. Go probe: `apps/kira-space/internal/flows/adeflow/ramprobe_test.go`, `TestRAMProbeAgents`,
`t.Skip` unless `KIRA_RAM_PROBE=1`, so normal flows and CI skip it. Built on the same harness setup
as `sessions_test.go` `TestInteractiveSessions` (`flowharness.New` with a fake agent scenario,
`openLaunch` to open each TUI terminal the way the renderer does). Steps:
- start: `runtime.MemProfileRate = 32 << 10` (finer heap samples; earlier allocations keep the
  default rate, a stated limit). If `KIRA_PPROF` is set, call `pprofserve.Start()` so a live run can
  be curled.
- `boot`: one repo, board built once (`Board`).
- `board200`: 10 live tasks, 200 archived tasks with branches (bound writes where a call exists, else
  the app's own repos, as other flow tests seed), then `Board` read.
- `sessions4`: 4 TUI agent sessions on 4 tasks. Fake `claude` scenario: an `Sh` action that writes
  about 2 MB of Claude-like full-screen redraw frames (ED2 plus 50 lines, the shape of
  `tests/perf/support/ramWorlds.ts` `agentFrames`) to stdout, then a `WaitFile` that never appears
  so the session stays running. Wait until all 4 rows read `running`.
- `idle60`: 60 s wall-clock wait. Figure-of-record checkpoint for the Go side.
- `pushes50`: 50 rounds of board invalidation plus `Board` and `Sessions` reads, 150 ms apart.
- `stopped4`: close the 4 sessions (`CloseSession` or terminal close as `TestCloseSession` does),
  wait for `stopped`.
- `final`: 10 s wait.
Per checkpoint: `runtime.GC()` twice; `runtime.ReadMemStats` (`HeapAlloc`, `HeapInuse`,
`StackInuse`, `Sys`, `NumGC`); `runtime.NumGoroutine()`; write `heap-<cp>.pb.gz`
(`pprof.Lookup("heap").WriteTo(f, 0)`) and `goroutine-<cp>.txt` (`debug=1`) to
`$KIRA_RAM_OUT` (default `apps/kira-space/perf-ram/go/<run>/`); process table: walk `/proc` for
every descendant of `os.Getpid()` plus self, read `Pss` from `/proc/<pid>/smaps_rollup` and
`comm`/`cmdline`, classify `app` (self), `shell` (direct child login shell of a terminal), `claude`
(fake agent; label it `claude-fake`), `git` (split `cat-file` from other subcommands), `js-worker`,
`other`. Print one line per checkpoint: `GORAM <cp> <json>` with MemStats, goroutines, and the
per-class `{count, pssMb}`; append the same JSON to `<out>/space-go.jsonl`. Git children are
transient: also sample the process table every 500 ms during `pushes50` and record the peak count
and peak PSS per class.
Docs: add `KIRA_RAM_PROBE` and `KIRA_RAM_OUT` rows to the `KIRA_*` table in
`docs/DEV_ENVIRONMENT.md`, and one bullet under "Perf probes" naming E1 to E5 and their commands.

`perf-ram/` output stays untracked; check `.gitignore` covers `apps/kira-space/perf-ram/`, add it in
the probe commit if not.

## 4. Runs

All commands from the worktree root. Before each pass move any old `apps/kira-space/perf-ram/` to
the scratchpad, so jsonl files hold this pass only. Tee every Playwright run to a log under the
scratchpad: `PSS`, `HEAPSNAP`, `HEAPDIFF`, `ADEDOM`, `RAMLOAD` lines live only on stdout.

### 4.1 Pass A, Chromium (minified)

```
bun run build:test
KIRA_RAM_WORLD=ade-agents,ade-idle,ade bunx playwright test \
  --config=apps/kira-space/playwright.perf.config.ts --project=heap heap-ram --repeat-each=3
```
Per checkpoint: `heapMb` (two GCs), `dom.nodes`, `dom.jsEventListeners`, `rss.rendererMb`, Chromium
renderer PSS (`PSS ... renderer`), `suspects` (xterms, monaco editors), `intro.queries`,
`queryBytes`, `storeBytes`, `__kiraTerminalStats`.

### 4.2 Pass A, WebKit (minified, shipped engine family)

```
KIRA_RAM_WORLD=ade-agents,ade-idle,ade bunx playwright test \
  --config=apps/kira-space/playwright.perf.config.ts --project=heap-webkit heap-ram --repeat-each=3
```
Per checkpoint: `PSS ... webkit` `webcontent` and `network` MB. This is part (a) of the record.

### 4.3 Pass B, Chromium (unminified, attribution)

```
cd apps/kira-space/frontend && KIRA_DEBUG_HOOKS=1 bunx vite build --minify false && cd -
KIRA_RAM_SNAPSHOTS=1 KIRA_RAM_KEEP_SNAPSHOTS=1 KIRA_RAM_WORLD=ade-agents,ade-idle,ade \
  bunx playwright test --config=apps/kira-space/playwright.perf.config.ts --project=heap heap-ram
bun run build:test   # back to minified
```
One repeat (attribution, not statistics). Keep the `.heapsnapshot` files in the scratchpad, not the
repo. Analysis in §5.2.

### 4.4 Go side

```
for i in 1 2 3; do
  KIRA_RAM_PROBE=1 KIRA_RAM_OUT=apps/kira-space/perf-ram/go/r$i CGO_ENABLED=1 \
    go test ./apps/kira-space/internal/flows/adeflow -run '^TestRAMProbeAgents$' -count=1 -v -timeout 15m
done
```
Three separate processes, never `-count=3` (one process shares heap state). Attribution per run:
```
go tool pprof -sample_index=inuse_space -top -nodecount=40 <out>/heap-idle60.pb.gz
go tool pprof -sample_index=inuse_space -top -nodecount=40 -base <out>/heap-board200.pb.gz <out>/heap-sessions4.pb.gz
go tool pprof -sample_index=inuse_space -top -nodecount=40 -base <out>/heap-sessions4.pb.gz <out>/heap-stopped4.pb.gz
go tool pprof -sample_index=inuse_space -top -cum -focus='internal/(ade|gitsession|mobileterm|terminal|oplog|agentnotify|mobileweb|harness)' <out>/heap-idle60.pb.gz
```
Group `inuse_space` by package (`-top -nodecount=200` then sum by package path prefix in a scratch
script). Goroutines: count per top frame from `goroutine-<cp>.txt`; per-session goroutines =
(`sessions4` - `board200`) / 4.

Never run real `claude`, never spend tokens (D3).

## 5. Analysis

### 5.1 Raw table

One table per world and engine: rows = checkpoints, columns = metrics of §4, each median [spread]
over the 3 repeats. Go: rows = checkpoints, columns `HeapInuse`, `Sys`, goroutines, PSS per process
class, git peak during `pushes50`.

### 5.2 Pass B attribution (renderer)

For `ade-agents` `final`, `approval`, `log3m`, `stopped4`, `back`, `ade-idle` `idle60`, `ade20`
`left`/`back`, attribute retained size by owner, using the `HEAPSNAP` report (named roots: each
`pinia:<id>`, each `query:<hash>`, `i18n:messages`; constructor rows) and `heapSnapshot.ts`
(`diffSnapshotFiles`, `retainerPath`). Owners, each a row:
- board query, sessions query, run log queries, other queries (from named roots);
- each ADE Pinia store over 0.5 MB, rest summed;
- xterm: `Terminal`, `BufferLine`, `CircularList` retained, per terminal from `__kiraTerminalStats`;
- Monaco (`StandaloneEditor`, `TextModel`, `monaco` module closures);
- Vue component instances and detached DOM (`Detached ...`, component count);
- i18n messages; framework and bundle baseline (what `ade-idle` `board` holds minus the above).
Renderer native = renderer PSS minus JS heap: one row, mostly not fixable (code pages, DOM native,
layout, fonts).

### 5.3 ade20 retained tree (D2)

1. WebKit: from §4.2, `ade20` WebContent PSS at `board`, `left`, `back`, `back3`. Step size =
   `back` - `board` median; compare with noise (§6.3).
2. Chromium Pass B: `HEAPDIFF ade20 left-back` and a `diffSnapshotFiles(left, back, /Detached|HTMLInputElement|VueComponent/)`
   focus run in a scratch script; list every detached `<input>` with its full node name (attributes)
   and its retainer path to `DocumentState`. Name the component that renders it (grep the
   attributes). Suspects from code: `AdeCaptureBox.vue` (`@keydown.enter` on the shadcn `Input`),
   `AdeTaskPanel.vue` title, `AdeEstimateField.vue`, `ade-task-host-input`.
3. Diagnosis-only trials allowed, at most 3, each on an uncommitted scratch edit reverted right
   after (`git stash` / `git checkout --`), on the unminified build: for example `v-if` unmount of the
   named input before the mode switch, or the input outside any `<form>`. Record each trial's `left`
   and `back` heap. Never commit a trial; the fix belongs to Part 2.

### 5.4 Go and process attribution

Rows: Go packages over 1 MB `inuse_space` at `idle60` (grouped as in §4.4); Go runtime overhead
(`Sys` - `HeapInuse`); app PSS minus Go `Sys` (binary text, cgo, SQLite page cache); per-session
cost = (`sessions4` - `board200`) / 4 for heap, goroutines and PSS; `stopped4` - `board200` as
retained-after-stop; shells; git `cat-file` count and PSS per repo or worktree; git peak during
pushes; `js-worker` (live or not).

## 6. Ranked table, targets, fix list

### 6.1 Ranked table

Columns: `id` (R1..Rn), owner, MB, share of record, fixable (yes, partly, no), why, source (world,
checkpoint, engine, pass). MB for renderer rows comes from Chromium Pass B; the audit says so per
row, and states WebKit corroboration where a WebKit PSS delta exists for the same checkpoint pair.
Share = MB / record total (D1). Sort by MB descending. `claude` gets a separate line below the table
with "not Kira-owned, not measured here" and, if supplied, the user's Mac figure.

### 6.2 Targets

- Per fixable row: target = row MB minus the saving its mechanism yields, computed from measured
  numbers (cap: bytes above cap at the checkpoint; removal of a duplicate: the duplicate's MB; lazy
  load: row MB at checkpoints where the data is not shown; reap: `stopped4` minus `board200`).
  Show the arithmetic. Never a round guess.
- Record target = record total minus the sum of kept savings. Also state the floor: record total
  minus every fixable row (best case).
- Each target names its row id. Part 2 re-measures the same rows.

### 6.3 Noise and the drop rule

Noise for a metric at a checkpoint = max(spread over the 3 repeats, floor). Floors: JS heap 2 MB
(P481 threshold), renderer or process PSS 10 MB, Go `inuse_space` 2 MB, goroutines 10. An item
whose expected saving is below the noise of the metric it moves is dropped with one line: item,
expected saving, noise, metric. A suspect cleared by the data (row under noise, or mechanism
absent) goes to the cleared-suspects table, not the fix list.

### 6.4 Part 2 fix list

Ordered by expected saving descending, ties broken by lower risk. Per item: id (F1..Fn), row(s)
moved, mechanism, library first (TanStack `gcTime`/`select`/separate query, VueUse, Pinia split per
`CLAUDE.md` Frontend) or the named requirement for hand-rolled, expected saving MB with arithmetic,
user-visible change (yes or no, what), test half (flow test for a Go cap, UI spec for a frontend
cache change, or "numbers in PERF only" when below a stable regression threshold), files touched.
Group related items (same module) as Part 2 commit groups.

## 7. Suspect shortlist (confirm or refute with numbers; never fix in Part 1)

Each suspect gets a verdict line in the audit: confirmed (row id), cleared (number), or not exercised
(why).

S1. Board history in every board read. `Board.history` (`frontend/src/ade/v2/wire.ts:776`) ships
every archived task; `useBoardHistory` (`readQueries.ts:37`) is its only reader; Go builds it on
every board build (`internal/ade/board.go:506`, `historyEntries`). Measure: `queryBytes`
`adetask/board` split (history JSON length vs rest), Go `inuse_space` under `historyEntries`,
`board200` minus `boot`.

S2. Board and sessions refetch per push. Every board push refetches the board through a gate, every
sessions push refetches the sessions list (300 stopped rows in the fixture) and invalidates the
`panelForward` family (`readSignals.ts:30-45`). Shallow queries replace whole values. Expect churn,
not retention: compare `push10` to `push50` heap; a rise above noise means retention.

S3. Run log cache. `useLog` `gcTime: 30_000` (`readQueries.ts:88-95`), `LOG_RETAIN_CHARS =
2_000_000` per log (`pushMerge.ts:20`, chars, so up to about 4 MB in V8 for two-byte text), one
`LogLine` object per chunk (`panel/AdeRunLog.vue:19-32`). Measure at `log3m`: log query bytes, line
count, heap delta.

S4. Headless xterm per running session. `attachHeadlessTerminal` (`createTerminalsStore.ts:165`)
creates an xterm for every running session, viewed or not; `AGENT_SCROLLBACK = 1000`
(`terminalRenderer.ts:152`), drain up to 256 KiB per tab (`createTerminalsStore.ts:38`). Reap of
stopped sessions: `adeTerminals.ts`. Measure: xterm retained per terminal at `final`, freed at
`stopped4`.

S5. Go mobile terminal ring. `mobileterm` keeps a 1 MiB replay ring per agent terminal
(`internal/mobileterm/ring.go:6`), created on first output by `getOrCreate`
(`broker.go:144-153`), possibly with phone access off. Measure: `mobileterm` `inuse_space` at
`sessions4`, `idle60`, `stopped4` (is the entry removed on close).

S6. ADE mounted tree retained after leave. ARCHITECTURE Known open item; §5.3.

S7. Monaco. Approval surfaces load Monaco only when a kind needs it (P661 F5); P691 added
`ade/v2/files/AdeFileViewer.vue`. Measure at `approval` and `files1`: monaco editors and models, and
whether the module and models stay after the dialog or tab closes (`left`).

S8. ADE Pinia stores (`ade/v2/state/*`, 24 files). Maps keyed by task id that may never shrink, for
example `adeBoardUi` `actionError`, `adeOps` `ops`. Measure: `storeBytes` and `pinia:*` named roots.

S9. Go board snapshot and git caches. Board snapshot (`internal/ade/boardcache.go:108`), per-repo
fact caches (`board_facts.go`), `gitsession` entry caches: diff cache cap 4 MiB per entry
(`internal/gitsession/cache.go:18`), detail, refs, stack, merge-base caches and one `git cat-file`
session per entry (`entry.go`, `detachCatFile`/`closeCatFile`). Measure: Go `inuse_space` under
`gitsession` and `internal/ade`; `cat-file` process count and PSS per repo or worktree.

S10. Go per-session state: op log in memory (`internal/oplog`, `Retention`), `agentnotify`, the
claude harness, terminal `Session` reader goroutines, log sinks (`internal/ade/logsink.go`).
Measure: per-session heap and goroutines from `sessions4` minus `board200`; retained after
`stopped4`.

S11. Mobile bridge duplicates. Desktop answers `mobileLaunchOpened`; Go `mobileweb` may hold board
or session copies for the phone. Measure: `mobileweb` `inuse_space` with phone access off (default).

S12. TanStack defaults. Client default `staleTime: Infinity`, `gcTime` 5 min unless a key sets
`KEEP_WHILE_OPEN` (`packages/workbench/src/state/queryClient.ts:17-35`). Per-task families
(`panelForward`, `archivedTask` with 200 archived rows) linger 5 min after their panel closes.
Measure: `intro.queries` `noObservers*` counts and `queryBytes` at `switch10`, `final`, `back`.

S13. Login shell per agent session. Each TUI session runs `<shell> -l -i -c claude`
(`internal/terminal/session.go:107-126`); the shell stays resident as parent of `claude`. Measure:
shell PSS per session. Fixable only if the shell can `exec` the command without losing the login
PATH; the audit states that condition, Part 2 decides.

S14. Renderer and bundle baseline. ADE chunks, i18n catalogs, Vue runtime. Measure: `ade-idle`
`board` heap and PSS. Mostly not fixable; listed for the share.

S15. Script runners and `js-worker`. Not started by the fixture; the audit marks them not exercised
unless the process table shows them.

## 8. Deliverable: `docs/v0.3.19/plans/P693-audit.md`

Committed as `docs: P693 agents RAM audit`. Terse style. Sections, in order:

1. Header: base commit (probe commit) and product tip under it; date; host record (§2); browser
   revisions; Go version.
2. Figure of record: definition (D1), limits (D1), user figure slot (value or "not supplied").
3. Commands: exact commands of §4, copy-pasteable, so the audit reruns from this file alone. Name the
   probe extension commit.
4. Runs: per world and engine, repeats kept and discarded (with `RAMLOAD` values).
5. Raw numbers: tables of §5.1.
6. Record baseline: (a), (b), (c) and total, median [spread]; `claude` line.
7. Ranked attribution: table of §6.1.
8. ade20 retained tree: §5.3 results, trials, verdict per D2.
9. Suspects: one verdict line each, S1 to S15 (§7).
10. Targets: per row and record target, with arithmetic (§6.2); proposed `docs/PERF.md` §1 budget row
    text (D4).
11. Part 2 fix list: ordered table (§6.4).
12. Dropped: one line per item under noise (§6.3).
13. Out of scope: `claude` reduction (D3), Chromium-only items, anything needing a user decision.

## 9. Order of work for the implementer

1. Read §1-§8 and the files named in §3. Confirm worktree clean and the concurrent guard-renumber
   commit landed (`git log -3`); never touch `scripts/`, `docs/ARCHITECTURE.md`,
   `docs/v0.3.19/SPEC.md` or `plans/P692*` for this phase.
2. E1-E5, docs rows, `.gitignore`. Run each new world, checkpoint and the Go probe once (1 repeat,
   gate on) to see it pass, then commit. Smoke numbers are not audit data.
3. Pass A Chromium, Pass A WebKit, Go probe (3 repeats each, gated). Pass B once.
4. Analysis (§5, §6), write the audit, commit.
5. Report: commits, record total, top 5 rows, fix list length, dropped count, user figure status.

Stop and report instead of guessing if: a probe extension needs a product behavior change; the
gate never passes; the Go probe cannot keep 4 fake TUI sessions running. Never narrow the method;
report what blocks it.
