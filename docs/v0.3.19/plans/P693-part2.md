# P693 Part 2: agents module RAM, fixes and re-measure

Planner: Opus. Implementer: one sequential Sonnet subagent, cold start. Fixes F1 to F4 of
`plans/P693-audit.md` §11 in order, then one re-measure with the Part 1 method. No stream split:
F1, F2 and F4 all touch the ADE frontend and `tests/perf/heap-ram.spec.ts`; F3 is Go-only but its
re-measure shares the same runs. No ownership table proves independence, so one implementer.

Read first: `CLAUDE.md`, `docs/WORKING_AGREEMENT.md`, `docs/v0.3.19/SPEC.md` § P693,
`plans/P693-audit.md` (all of it; §3 commands, §6 record, §8 retained tree, §10 targets, §11 fix
list, §12 dropped), `plans/P693-part1.md` §2 (base, gate, repeats) and §6.3 (noise rule),
`docs/PERF.md` §1 and §2.15, `docs/ARCHITECTURE.md` Known open item "ADE first mount stays
retained after leaving ADE", `docs/DEV_ENVIRONMENT.md` "Perf probes" and "Agents module footprint
run".

CodeGraph: every discovery step below (F1 step 1, F2 step 1, F3 step 1, F4 analysis) goes through
`codegraph_explore` before Read or Grep (`ToolSearch` "codegraph" first if it is not loaded). The
index covers `/home/user/kira-space` and lags this worktree; re-check each hit in the worktree.
Grounding below came from `codegraph_explore` on that index, re-checked in the worktree at
`b34439d0d`. Line numbers are hints; re-grep before relying on one.

## 0. Base, state file, gate

- Product base: `b34439d0d` (chapter tip; the commit after the audit only removes unused code, so
  audit numbers stand as "before"). Probe base of the audit: `1276c7691` + `0107b563d`.
- State file: `docs/v0.3.19/plans/P693-part2-measure.md`. Every finding a later step needs goes there
  and is committed as it lands (`docs: P693 part 2 measurements`): F1 store list and retainer paths,
  F2 contract verdict, F4 breakdown, every re-measure table (median [spread], kept and discarded
  repeats with `RAMLOAD`). Never hold these for the final report.
- Gate, every measured run (unchanged from Part 1 §2): wait for `load1 <= 1.0` (10 s polls, max
  15 min) and no other `playwright|go test|vite|vitest` process; probe gate on; never set
  `KIRA_RAM_NOGATE`; discard and rerun any repeat whose `RAMLOAD` load1 > 1.0. 3 repeats for Pass A
  (both engines) and Go; Pass B once. Noise = max(spread, floor); floors: JS heap 2 MB, renderer or
  process PSS 10 MB, Go inuse 2 MB, goroutines 10.
- Smoke runs while implementing are 1 repeat and never audit data.

## 1. F1: retained ADE tree (row R6)

Mechanism, confirmed in source. Vue 3.5.44 `doWatch` (`runtime-core.cjs.js` ~916-944) reads
`currentInstance` when a watcher is created: it closes over the instance in
`baseWatchOptions.call` and, for pre-flush, sets `job.i = instance`. A Pinia setup store runs its
setup inside the first consumer's `setup()`, so `currentInstance` is that component, while the
watcher's effect lives in the store's scope (Pinia's root scope, app lifetime). The store therefore
pins its first consumer, and through `instance.parent` and `subTree` the whole ADE tree, after
unmount. TanStack `useQuery`/`useMutation` (vue-query 5.105.1, `useBaseQuery.js:43,74`,
`useMutation.js:178,186`) call `watch` too, so a store that calls a query or mutation hook pins the
same way. `WorkbenchShell.vue:65` mounts the full view with `v-if` and no KeepAlive, so leaving ADE
unmounts the tree.

Candidates from code (worktree, `apps/kira-space/frontend/src`): stores whose setup creates a
watcher directly or through a hook, and that `main.ts` `mountShell` does not create before mount:
- `ade/v2/state/adeTerminals.ts`: `watch` + `useSessions` (query).
- `ade/v2/state/adeStageDialog.ts`: `usePlanModel`, `useSessions`, `useStepDone`,
  `useSetTaskStep`, `useNextPass`, `useDismissHandover`, `useMutationAction`.
- `ade/v2/state/adeDialogs.ts`: about 12 mutation hooks plus `useLaunchOpener` (creates
  `adeTerminals`).
- `ade/v2/state/adeLeases.ts`: `useLeases` (query) and 3 mutations.
- `ade/v2/state/adeTakeOver.ts`: `useSessionViews`, `useTakeOver`, `useRestoreTask`, `useSetupWait`,
  `useLaunchOpener`.
- `ade/v2/state/adeRepoGate.ts`: `useAddTaskRepo`, `useAsyncAction`.
- `ade/v2/state/adeBoardUi.ts`, `adeDetailsList.ts`: `usePersistedState` (check whether it wraps a
  VueUse storage watcher; `adeBoardUi` is also created by `installAdeSignals` callbacks).
- Repo-mode stores with `watch` (`repo/state/repoHeads.ts`, `repoLinks.ts`, `worktrees.ts`) and the
  audit's second route (`repoVisibility` `showHidden` `Dep.subs`): in scope only if the step 1 walk
  shows them on a path to an ADE component instance.

Step 1, name the stores (discovery, CodeGraph mandatory):
1. Enumerate with `codegraph_explore` every `defineStore` setup under `apps/kira-space/frontend/src`
   and `packages/workbench/src` that reaches `watch`, `watchEffect`, a VueUse watcher
   (`useStorage`, `watchDebounced`, `useEventListener`) or a vue-query hook, transitively through the
   composables and stores it calls. Mark each: created in `mountShell` before mount (safe), or first
   created by a component that can unmount (offender).
2. Confirm on the heap: Pass B build (audit §3), `KIRA_RAM_WORLD=ade` only, snapshots kept. In the
   `ade20` `left` snapshot, walk retainers from the `AdeShell` instance (shortest path, DevTools
   handles excluded, as audit §8). For each `ReactiveEffect` on a path, read its `fn` closure's script
   location (unminified build names the file and line) to name the store file. Repeat for every ADE
   component instance still present at `left`, not only `AdeShell`.
3. Write the union (code list plus heap list) to the state file with one line each: store, hook
   that creates the watcher, first consumer component.

Step 2, fix. Activate the offending stores outside any component instance, the first time the
`ade` full view becomes active, so their watchers carry no instance:
- New module `apps/kira-space/frontend/src/ade/v2/state/activate.ts`: `activateAdeStores(app: App)`,
  idempotent (module flag), runs `app.runWithContext(() => { useAdeTerminalsStore(); ... })` for
  every store from step 1. `runWithContext` gives `inject` (Pinia, `useQueryClient`) with
  `currentInstance` null. One comment line states why (watchers created under a component pin it).
- Trigger: `main.ts`, workbench windows only (`workbenchWindow`), after `provideShellServices` and
  before `app.mount`: a `watch(() => modeStore.active === 'ade', (on) => on && activateAdeStores(app),
  { immediate: true, flush: 'sync' })`. Sync flush runs before `AdeShell` setup on a mode switch;
  `immediate` covers a window restored into `ade`. Created in boot, outside any instance, app
  lifetime like `installAdeSignals`. `mountApp` gains the hook it needs (for example an
  `onBeforeMount(app)` argument); the proof window path stays unchanged.
- Mobile launch: `installMobileLaunch` (`ade/v2/dialog/mobileLaunch.ts`) calls `useLaunchOpener()`
  from an event callback, outside setup. Check with `codegraph_explore` whether vue-query 5.105.1
  `useQueryClient` throws there when `adeTerminals` was never created (no injection context). If it
  does, the callback calls `activateAdeStores` first (pass the app in through `installAdeSignals` or
  a module setter); same commit.
- Rejected: KeepAlive (keeps the tree alive on purpose, same RAM); `useXStore(pinia)` alone (does
  not clear `currentInstance`); moving `adeTerminals`' reap into a component (the reap must run app
  wide, and the query hooks in the other stores would still pin); eager activation at boot for every
  window (starts board, sessions and leases observers before the user opens ADE; after the first ADE
  visit behavior is identical either way, so the lazy trigger keeps today's startup).
- Library check: Vue `watch` plus `app.runWithContext` and Pinia's documented outside-component use;
  nothing hand-rolled.

Step 3, verify (Pass B, ade only): rerun step 1.2. Done when no retainer path runs from a Pinia store
scope to an unmounted ADE component instance at `left`, and Chromium `ade20` heap `back` - `board`
<= 2 MB in a smoke run. If another path remains (for example the `repoVisibility` route), name it in
the state file and fix it in this commit group if it is the same Vue/Pinia lifecycle mechanism. At
most 3 further diagnosis trials (uncommitted, reverted). Stop condition: target met, or 3 trials
spent. Then keep what moved R6 by more than its noise and restate the open item with numbers (§6).

Behavior: no user-visible change. Internal change: offending ADE stores and their observers start on
the first ADE entry; before, they started when a component first used them, usually within the same
entry.

Test half (frontend only, no IPC change, so no flow half):
- New UI spec `apps/kira-space/tests/ui/ade-v2-retention.spec.ts`. Board fixture from
  `tests/ui/support/adeV2.ts` with 20 live tasks (the ade20 shape). Steps: boot in `ade`; open one task
  panel (title input, capture box present, as the `task` checkpoint); store
  `window.__adeShellRef = new WeakRef(document.querySelector('[data-testid="ade-shell"]'))` in its own
  `page.evaluate`; switch to mode `git`; CDP `HeapProfiler.collectGarbage` twice; assert
  `__adeShellRef.deref() === undefined`. Then return to `ade`, GC twice, and assert
  `Memory.getDOMCounters().nodes` at `back` minus `board` is at most the tolerance. Tolerance: 10% of
  the pre-fix duplicate. Measure that duplicate once with step 2 reverted, in this spec's own fixture.
  The audit's ade20 figure was 16752 - 7323 = 9429 nodes, so expect about 943. Record pre-fix delta,
  tolerance and 3 post-fix deltas in the state file. No `page.clock.install()`; no ElementHandle kept.
- Run the spec once with step 2 reverted: both assertions must fail. Record it.
- CDP needs Chromium; `ui` is WebKit. Add project `ui-heap` to `apps/kira-space/playwright.config.ts`
  (`browserName: 'chromium'`, `testMatch: ['ade-v2-retention.spec.ts']`), add the file to `ui`'s
  `testIgnore`, add `--project=ui-heap` to `test:ui` in `package.json`, and add a step to
  `.github/workflows/pr.yml` next to the `ui-clipboard` step (line ~147), same shape.

Files: `ade/v2/state/activate.ts` (new), `main.ts`, possibly `ade/queries.ts` and
`ade/v2/dialog/mobileLaunch.ts`, any store a step 3 path names, the spec, `playwright.config.ts`,
`package.json`, `.github/workflows/pr.yml`.

Checks: `bun run typecheck`, `bun run lint`, `bun run build:test`, then
`bunx playwright test --config=apps/kira-space/playwright.config.ts --project=ui-heap`, and in
`--project=ui` the specs whose stores changed timing: `ade-v2-dialogs`, `ade-v2-launch`,
`ade-tui-takeover`, `ade-v2-leases`, `ade-terminal-lazy`, `ade-mobile-approval`, `ade-v2-handover`,
`ade-v2-norepo`, `ade-agents-tab`. `bun run test:unit` if a unit spec imports a changed store.

Commit: `fix(ade): activate ADE stores outside a component instance` (product, spec, project, CI).

## 2. F2: headless terminals of stopped sessions (row R9)

Finding from grounding: the reap exists and disposes the xterm. `adeTerminals` reaps a tracked
terminal once no running session holds its id and its local status is `exited`, `failed` or gone
(`adeTerminals.ts:29-50`). It calls `closeTerminalSession` and `cleanupTabRuntime`, and
`terminalRenderer.ts:140-145` disposes the xterm on that cleanup. The probe's `stopped4`
(`heap-ram.spec.ts:1113`) pushes `state: 'stopped', terminalId: ''` but never emits a terminal exit
event, so local status stays `running`, and the guard `local && local !== 'exited'` skips the reap.
In Go, `Tracker.Reconcile` (`internal/ade/tracker.go:875`) marks a record stopped only when its
terminal is no longer live. So in the real app a stop follows a pty exit, and the exit event fires.

Step 1, contract (discovery, CodeGraph mandatory): list every caller of
`AdeSessionsRepo.MarkStopped` and every other path that sets a session `stopped`. For each, confirm the
pty has exited or is closed (exit event sent through `BoundService`/`Registry`) before or with the
sessions push. Also confirm what a stopped session's viewer shows: stopped rows carry
`terminalId: ''`, so no pane can address the old buffer (check the TUI pane and `TerminalHostView`
consumers). Write the verdict to the state file.

Step 2, branch on the verdict:
- A (expected): every stop path ends the pty. F2 needs no product change. Fix the probe: `stopped4`
  first emits a terminal exit event for `term-ag0..3` on `IPC.terminal` (`exited: true`,
  `exitCode: 0`, the `TerminalEvent` shape `feedFrames` already uses), then the sessions push, then
  waits 2 s. That is the real order. Smoke run `KIRA_RAM_WORLD=ade-agents`, 1 repeat:
  `__kiraTerminalStats` empty at `stopped4`. In Pass B, ArrayBuffer retained at `stopped4` must be
  <= 0.5 MB. F2 is declined as a product change. Requirement named: the reap already frees
  everything on exit; the audit's 8.4 MB came from a probe that skipped the exit event. User-visible
  change: none.
- B: a path marks stopped while the pty lives. Fix that path in Go so it closes the terminal (exit
  event follows, the existing reap runs). Never reap a live terminal from the frontend: that kills a
  shell the user may still use. Test halves: flow test in `internal/flows/adeflow` (that stop path
  emits the terminal exit event and the session reads `stopped`) and a UI spec (exit event plus
  stopped push: `__kiraTerminalStats` has no entry for that id). User-visible change: the
  terminal of a session stopped that way ends with the stop; state it exactly in the Result.
- Either branch: if an existing UI spec already covers reap on exit (check `ade-terminal-lazy.spec.ts`
  and `ade-tui-takeover.spec.ts`), add none for A.

Checks: `bun run typecheck:tests`, `bun run lint`; B adds `go vet`, the new flow test, and
`go test -p 1 ./apps/kira-space/internal/flows/adeflow -run 'Session'`.

Commit: A: `test(perf): stopped4 emits terminal exits`. B: `fix(ade): close the terminal of a
stopped session` plus that probe commit.

## 3. F3: lazy mobile replay ring (row R8)

Grounding: `mobileterm.Broker.getOrCreate` (`broker.go:144`) builds each entry with
`newRing(ringCap)`, 1 MiB cap, growing with output. `Output` (`broker.go:162`) appends every chunk
whatever the phone access state. Only `Exited` frees the ring. Default `Mobile.Enabled` is false
(`model/settings.go:189`), so most users pay 1 MiB per agent terminal for nothing.

Step 1 (CodeGraph): confirm the boot path that reads `Mobile.Enabled`
(`NewMobileAccessService`, its `Build` caller, `StartIfEnabled` or supervisor start) and
`MobileAccessService.SetEnabled` (`bridge/mobile.go:259`). Confirm `TestPhoneTerminalAttach`'s
`pairedPhone` enables access before `startSession`.

Step 2, fix (no library; named requirement: replay is needed only for a phone that can attach,
which needs access on):
- `Broker` gains `recording atomic.Bool` and `SetRecording(on bool)`. Default off.
- `getOrCreate` builds the entry with no ring. `Output`: when recording is off, return before any
  append; when on, create the ring on first use (`if e.ring == nil`).
- `SetRecording(false)`: drop `e.ring` of every entry with no phone connection. Held entries are
  released by the server stop anyway.
- `Attach`: create an empty ring if the entry has none, so `Snapshot`, `ReadFrom` and `End` never see
  nil.
- Gate = the user's phone access setting `Mobile.Enabled`, not "server running". An untrusted
  network stops the server, and the phone may return, so replay stays. Wire it: the boot path calls
  `SetRecording(cfg.Mobile.Enabled)`; `SetEnabled` calls `s.Terminals.SetRecording(args.Enabled)`
  before it starts the supervisor on enable and after it stops it on disable. Add `SetRecording` to
  the `MobileTerminalBroker` interface (`bridge/mobile.go:99`).
- Existing `broker_test.go` cases that expect replay call `SetRecording(true)` first.

Edge change (user-facing, one line in the Result): a phone that attaches after phone access was
turned on mid-session sees only output since access was turned on, not the last 1 MiB from session
start. Turning access off and on again restarts replay at that moment.

Test halves: flow test, new `TestPhoneTerminalReplayStartsAtAccess` in
`internal/flows/mobileflow/terminal_test.go`, own fixture order: access off; `startSession` (the fake
agent writes its banner); enable access; produce output after enabling (a phone or desktop write that
the fake agent echoes, or a second `Emit`, whichever the fixture supports); pair and attach. Assert:
the replay holds the post-enable marker and not the banner. `TestPhoneTerminalAttach` still passes
(banner replay with access on before start). No frontend half: the phone page is unchanged and
renders the bytes the WebSocket sends. The WebSocket is the IPC boundary, asserted by the flow test.
No unit test: the lazy ring is one condition.

Checks: `go vet ./apps/kira-space/...`, `go test ./apps/kira-space/internal/mobileterm/...`,
`go test -p 1 ./apps/kira-space/internal/flows/mobileflow/...`, `bun run lint:go`, and one smoke
`TestRAMProbeAgents` (Part 1 §4.4 command, `KIRA_RAM_PROBE=1`): `mobileterm` absent or <= 0.3 MB in
`pprof -base heap-boot heap-idle60`.

Commit: `perf(mobileterm): keep the replay ring only while phone access is on`.

## 4. F4: native renderer growth after launch, gated (row R2)

Row: WebKit +120.3 MB `ade-agents` `board` to `final` (spreads up to 38 MB); Chromium PSS +38.3, of
which heap +10.0 and native +28.3. Steps: `launch4` +20.4, `switch10` +13.0 (Chromium PSS). No owner
named yet.

Step 1, probe (own commit, probe only): extend `heap-ram.spec.ts` `ade-agents` with an `ADEDOM`
line at `board`, `launch4`, `panes2`, `switch10`, `final`, printed after the sample so it never
perturbs it. Reuse the `ade` world's `ADEDOM` helper. Fields: DOM nodes per top-level region (shell
bar, board, panel, TUI pane, dialogs, rest), `.xterm` hosts opened, xterm row elements, `<canvas>`
count, `<style>` count and summed CSS text length, `document.fonts` loaded faces, and Chromium
`Memory.getDOMCounters` (documents, nodes, jsEventListeners). Pass B snapshots at `board`, `launch4`,
`switch10`, `final` (add `snapshot` to those steps): constructor deltas for `(compiled code)`,
`ExternalStringData`, ArrayBuffer backing stores, `Detached *`, `(system)`. Commit:
`test(perf): ADE DOM breakdown in ade-agents`.

Step 2, analysis (CodeGraph for any owner named): attribute the `board` to `launch4` and `panes2` to
`switch10` steps. Write the table to the state file.

Fix gate: fix only if one named owner accounts for at least the noise of the metric it moves
(Chromium renderer PSS 10 MB, or JS heap 2 MB) and a code mechanism is named whose change loses no
behavior. At most 2 diagnosis trials (uncommitted, reverted).

Stop condition, any of: no owner reaches its floor; the owner is engine or lazy-chunk cost (code
pages, `ExternalStringData` of a loaded chunk, as R5); the 2 trials do not move the step past its
noise. Then F4 ends with the breakdown recorded in PERF and no fix. If an owner qualifies but the fix
leaves the agents module or needs a design decision, add a follow-up phase row `P694` at the end of
the `SPEC.md` table (numbering rule) and report it. Do not implement it here. A qualifying in-scope
fix gets commit group 4b (`perf(ade): ...`) with the test half its type needs (Part 1 §6.4).

## 5. Re-measure (after F1 to F4 land)

Exact Part 1 method, `plans/P693-audit.md` §3 commands verbatim, from the worktree root, gate as §0:
1. `bun run build:test`; Pass A Chromium, `KIRA_RAM_WORLD=ade-agents,ade-idle,ade`, `--project=heap`,
   `--repeat-each=3`, tee to scratchpad log.
2. Pass A WebKit, same worlds, `--project=heap-webkit`, `--repeat-each=3`.
3. Pass B once (unminified `KIRA_DEBUG_HOOKS=1` build, `KIRA_RAM_SNAPSHOTS=1
   KIRA_RAM_KEEP_SNAPSHOTS=1`), then `bun run build:test`. Serves F1 verification (no store-scope path
   at `ade20` `left`), F2 (ArrayBuffer at `stopped4`) and F4 (breakdown at tip).
4. Go: three separate processes of `TestRAMProbeAgents`, as audit §3; `go tool pprof
   -sample_index=inuse_space -top -nodecount=40 -base heap-boot heap-idle60` per run. Re-derive the
   harness `encoding/base64.EncodeToString` MB from these runs. Never reuse the audit's 11.5.
Move any old `apps/kira-space/perf-ram/` to the scratchpad before each pass.

Changed checkpoint: `stopped4` now emits exit events, so the audit's `stopped4` is not comparable.
Base for it: a detached worktree at `b34439d0d` with only the stopped4 probe commit cherry-picked.
Run Pass A Chromium `KIRA_RAM_WORLD=ade-agents`, 3 repeats, and Pass B once. That is the F2 "before"
(in branch A it shows the reap already working on the base). Remove the worktree after. If the host
record (`nproc`, `uname -r`, browser revisions, `go version`) differs from audit §1, re-run the full
base the same way and say so.

Rows to compare, before (audit) and after, median [spread]:
- Record: (a) WebKit WebContent `ade-agents` `final`; (b) Go anon `idle60` minus harness base64;
  (c) helpers; total. Target 606.2 MB (610.5 - 4.3). Noise on (a) is 24.6, so state the delta
  against it.
- Secondary record rows (audit §10, §13): Kira-owned at `back` = WebKit `ade-agents` `back` + Go anon
  `final` minus base64; at `stopped4` = WebKit `stopped4` + Go anon `stopped4` minus base64.
- R6: WebKit `back` - `board` for `ade20`, `ade200`, `ade-idle`, plus `back2`, `back3` (the audit's
  second-return growth, +42.4 on ade20); Chromium heap, PSS, DOM nodes `back` - `board` for the same
  worlds; Pass B Vue instances at `left`. Targets: WebKit <= 10 MB, Chromium heap <= 2 MB, PSS <= 10.
- R9: Pass B ArrayBuffer retained at `stopped4` (target <= 0.5 MB); `__kiraTerminalStats` lines.
- R8: Go `mobileterm` inuse at `idle60` (target <= 0.3 MB); heapInuse and anon at `sessions4`,
  `idle60`.
- R2: F4 breakdown at tip.
- Regression check: every other checkpoint of every world within its noise of the audit; any move
  past noise is explained or fixed (same phase).

Timing budgets: `bun run test:ui:timing` once, alone, gated (F1 changes boot wiring). No ADE
selection latency spec exists (grep for timing assertions under `tests/ui/ade*` finds none); say so in
the Result.

## 6. Docs

`docs/PERF.md`:
- New `### 2.17 P693 — agents module RAM`, §2.15 style: base and tip commits, host, gate, exact
  commands, the before/after tables of §5, targets met or missed with numbers, what was not reached
  and why (R1 floor, R2 per F4, R5, R7 dropped), the F2 probe correction, the F3 edge change.
- §1 budget rows. Threshold = after median + noise (max(spread, floor)). If the audit target was met,
  cap the threshold at that target. If it was missed, use after + noise and point to the explanation.
  Rows:
  - ADE leave and return, ade20 WebKit WebContent PSS, `back` - `board`. Where: `heap-ram.spec.ts`.
    Manual probe.
  - Same, ade-idle Chromium heap, `back` - `board`. Manual probe.
  - ADE shell collected after leave, and DOM nodes `back` - `board` <= tolerance. Where:
    `tests/ui/ade-v2-retention.spec.ts` (`ui-heap`). Asserted.
  - Stopped agent sessions, ade-agents Pass B ArrayBuffer retained at `stopped4`. Manual probe.
  - Agent terminal replay ring, Go probe `mobileterm` inuse at `idle60`. Manual probe.
  - Kira-owned PSS at `final` (record), `back` and `stopped4` (secondary). Manual probe.

`docs/ARCHITECTURE.md`:
- Known open item "ADE first mount stays retained after leaving ADE": delete it if the R6 targets are
  met (WebKit ade20 and ade-idle <= 10 MB, Chromium heap <= 2 MB). Otherwise restate it with the new
  numbers, the remaining retainer path and its delete condition.
- Add one rule line where frontend state is described: a Pinia store whose setup creates a watcher
  (directly, or through a vue-query or VueUse hook) pins the component that first created it. ADE
  stores therefore activate through `activateAdeStores`, and a new store of that kind joins its list.
- Mobile terminal section: the replay ring exists only while phone access is on (F3).
- Branch B of F2 only: the stop path that now closes its terminal.

`docs/DEV_ENVIRONMENT.md`: "Perf probes": `stopped4` emits terminal exits; `ADEDOM` in `ade-agents`;
the `ui-heap` project and its command. "Agents module footprint run": the secondary `back` and
`stopped4` figures.

`docs/v0.3.19/SPEC.md`: row P693 Part 2 to `Done`. Result paragraph: commits; per fix, landed or
declined with requirement; record before/after; R6, R9, R8 before/after against targets; F4 outcome;
budget rows added; timing run; open item deleted or restated. Include the one-line user-facing
mentions: F2 (branch A: no visible change; B: the exact change) and F3 (phone replay starts at the
moment access is turned on). Visible changes for the visual re-record: none expected (no rendered
UI change); confirm.

Commit: `docs: P693 part 2 results` (PERF, ARCHITECTURE, DEV_ENVIRONMENT, SPEC). The state-file
commits come before it.

## 7. Order of work

1. Read §0 files. `git status` clean, `git log -3` shows `b34439d0d` or later.
2. F1 steps 1 to 3, checks, commit. State file commit with the store list and paths.
3. F2 step 1, verdict to the state file; step 2 branch; checks; commit.
4. F3 steps 1 and 2, flow test, checks, commit.
5. F4 step 1 commit; Pass B (also the first F1 and F2 verification at tip); analysis to the state
   file; fix or stop per gate.
6. Re-measure §5, base worktree for `stopped4`, timing run. Tables to the state file, commit.
7. Docs §6, commit. Pre-commit hook passes on every commit; never `--no-verify` on a shipped commit.
8. Report: commits; per fix the verdict and before/after against its target; record total; F4
   outcome; anything restated or added to `SPEC.md`.

Stop and report instead of guessing if: activation needs a store change beyond its creation point;
the gate never passes in 15 min (wait and retry, never lower it); branch B needs a Go design change
beyond closing the terminal; a re-measure checkpoint regresses past noise with no named cause.
