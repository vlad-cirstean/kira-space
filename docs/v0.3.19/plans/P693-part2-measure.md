# P693 Part 2 measurements

State file. Findings land here as they are found.

## F1: retained ADE tree

Method deviation: CodeGraph indexes `/home/user/kira-space`, not this worktree; used `explore` on the
main index plus worktree Grep. Heap walk: temp spec (unminified build, `ade20` shape, snapshot after
leave and 2 GC) with its own BFS from GC roots that skips DevTools console handles and WeakMap
ephemeron edges ("part of key"): WeakMap value-to-key cycles read as reachable otherwise.

Offending sources, four mechanisms (all named by walk, fixed in commit group 1):

| # | source | mechanism | fix |
|---|---|---|---|
| 1 | Pinia stores `adeBoardUi`, `adeDetailsList`, `adeOps`, `adeDialogs`, `adeStageDialog`, `adeLeases`, `adeTakeOver`, `adeRepoGate`, `adeTerminals` (code list) | store setup creates watcher, query or mutation under first consumer component; `job.i` pins it | `activateAdeStores` |
| 2 | Pinia store `repoLinks` (heap: `watch(codeReposStore.records)`, first consumer `AdeTaskCard`) | same, repo-mode store used by ADE tags | added to `activateAdeStores` |
| 3 | TanStack query `['adetask','candidates',id]` (`useCandidates`, first observer `AdeTaskBranchForm`) | cached 5 min after unmount; `queryFn` closes over the caller's getters, shares the V8 context chain of the component; held by the query gc `setTimeout` | `gcTime: 0` |
| 4 | `usePaneResize` `abort` (`onScopeDispose`) read `isDragging.value` | unmount runs inside the parent's render with tracking on: `WorkbenchShell` render effect subscribed to the ResizeHandle's `useDraggable` computed, which closes over the ADE `ResizeHandle` instance via `emit`. Stale link dropped only on the next `WorkbenchShell` render | plain `active` flag |

Not named: `repoVisibility` `showHidden` route (audit §8) never appeared; no further path at `left`.
`agentControlKey` was provided in `App.vue` setup, so a store set up before mount threw
`agentControlKey is not provided`, leaving `adeTakeOver` half built (`confirm` undefined opened a blank
"Take over run?" dialog). Provided in `main.ts` `mountApp` instead (same app-level provide).
Mobile launch: `installMobileLaunch` calls `useLaunchOpener` from an event callback; Pinia runs store
setup with `pinia._a.runWithContext`, so `inject` works there with `currentInstance` null. No change.

Trials: diagnosis trials were run as walk iterations (repoLinks, candidates gcTime, usePaneResize);
each moved the path, the third met the target. Without trial 3's fix the shell stays pinned until the
next `WorkbenchShell` render; without trial 2's, until the query gc timer fires (5 min).

Spec `ade-v2-retention.spec.ts` (`ui-heap`, Chromium), ade20 fixture, DOM nodes:

- Baseline is the `task` checkpoint (panel open): return restores the open task panel (+1338 nodes
  vs `board`), so `back - board` includes that panel by design.
- Pre-fix (all fixes reverted): `__adeShellRef.deref()` not undefined (assertion 1 fails). Earlier
  pre-fix run vs `board`: 7323 to 16764, delta 9441. Pre-fix duplicate vs `task`: 16764 - 8666 = 8098.
  Tolerance 810 (10%).
- Post-fix, 3 runs: task 8666, back 8861, delta 195 (all identical).
- Revert run: assertion 1 fails; assertion 2 would fail (16764 vs 8666+810).

## F2: headless terminals of stopped sessions

Verdict: branch A. Every path that sets a session `stopped` ends the pty or has none:

| path | pty |
|---|---|
| `Tracker.Reconcile` (`tracker.go` ~906) | runs only for a terminal no longer in `LiveAgents`: pty gone, exit event already sent |
| `Tracker.Stop` (~850), only caller `closeTUILocked` (`launches.go:219`) | marks stopped, then `CloseTerminal` blocks until the process exits; exit event follows |
| `Tracker.Abort` (~762) | terminal lost (compose failed) |
| `stopHeadlessSession` (`runs.go:1369`) | headless: `claude -p`, no pty and no frontend terminal |
| `StopAllTaskRunning` (`recover.go`) | boot recovery, no pty of the previous life |

Stopped rows carry `terminalId: ''`, so no pane addresses the old buffer. Reap guard
(`local && local !== 'exited' && local !== 'failed'`) waits for the exit event; in the real order the
push may precede the exit (Stop marks first), and the exit re-runs the watch (it reads local status).
The audit's 8.4 MB at `stopped4` came from a probe that skipped the exit event.

Probe fix: `stopped4` emits `IPC.terminal` exit (`exited: true`, `exitCode: 0`) for `term-ag0..3`
before the sessions push. Smoke (Chromium, `KIRA_RAM_WORLD=ade-agents`, 1 repeat, RAMLOAD load1 4.46
at start; smoke only, never audit data): `__kiraTerminalStats.terminals` = 4 entries at `files1`, empty
at `stopped4`, `left`, `back`. Pass B ArrayBuffer check follows in the re-measure.
No existing spec covers reap on exit for ADE; none added (plan: A adds no spec).

## F3: lazy mobile replay ring

Wiring confirmed: boot `StartMobileIfEnabled` reads `cfg.Mobile.Enabled`; `SetEnabled` is the toggle;
`TestPhoneTerminalAttach` `pairedPhone` runs `SetEnabled(true)` before `startSession`. Gate is
`Mobile.Enabled`, not "server running". `Broker.SetRecording` (default off), ring created on first
recorded output or on `Attach`, dropped on disable for terminals with no phone connection.
Flow test `TestPhoneTerminalReplayStartsAtAccess`: fails with the gate disabled (replay held the
banner), passes with it. Smoke (1 run, host load1 4.50 at start, smoke only): `mobileterm` absent from
`pprof -base heap-boot heap-idle60` top rows (harness base64 11.22 MB, next 0.13 MB).

## F4: native renderer growth after launch (row R2)

Probe: commit `2e23b2473` (`ADEDOM` JSON per checkpoint plus Pass B snapshots at `board`, `launch4`,
`switch10`, `final`). Data: Pass A Chromium ADEDOM lines, Pass B snapshot constructors (unminified).
Zero diagnosis trials used.

Step `board` to `launch4` (Chromium PSS +20.4 [3.0]; heap +3.9):

| owner | board | launch4 | delta |
|---|---|---|---|
| JSArrayBufferData (xterm buffers, 4 terminals, 4220 Uint32Array) | 0 | 7975 KB | +7.8 MB native |
| Snapshot native total | 26.1 MB | 36.7 MB | +10.6 |
| V8 heap | 37.2 | 41.2 | +4.0 |
| compiled code | 8589 KB | 10093 KB | +1.5 MB |
| ExternalStringData (lazy terminal chunk source) | 19330 KB | 20180 KB | +0.85 MB |
| blink GridSizingTrackCollection | 0 | 198 KB | +0.2 MB |
| DOM elements (shell) | 1119 | 1295 | +176 (panel 175, TUI pane 134, 1 `.xterm`, 53 rows, 3 `<style>` 55 KB, 0 canvas, fonts 1) |

Step `panes2` to `switch10` (Chromium PSS +12.8 [3.3]; heap +5.6): first task panel mount (`ade-panel`
0 to 281 elements). V8 41.2 to 46.9 (+5.7 MB), spread over many small owners: Vue instances +1417
(+338 KB), vnodes +3240 (+386 KB), Link +5.5k (+237 KB), Dep +6.3k (+270 KB), `(array)` +693 KB,
Object +5.3k (+100 KB), compiled code +1.1 MB, native +1.3 MB. ExternalStringData flat.

Fix gate verdict, stop condition: no owner reaches its floor with a no-loss mechanism.
- `launch4`: best owner is xterm scrollback buffers, 7.8 MB, under the 10 MB PSS floor. Only lever is
  scrollback depth (behavior loss). Buffers free on stop (R9, below). Compiled code and
  ExternalStringData are lazy-chunk cost (R5 class), 2.35 MB total.
- `switch10`: panel mount reactive graph, no single owner at the 2 MB heap floor (largest, compiled
  code 1.1 MB, is lazy chunk). Inherent to opening a panel.
No code change, no P694 row. Breakdown goes to PERF §2.17.

## Re-measure

Host: Linux 6.18.44-fc-v726, 4 vCPU, 16087 MB, no swap, go1.27.3, Playwright 1.64.1, chromium-1243,
webkit-2370. Same as audit §1. Tip `2e23b2473`. Base for before: audit numbers (`b34439d0d`,
product unchanged since). Gate: load1 <= 1.0 and no competing process. Pass B load is not a gate
(snapshots burn CPU); attribution only, one repeat.

Runs. First Pass A Chromium attempt died at the 10 min background limit (log
`passA-chromium.killed.log`), discarded, not data. Rerun full.

| engine / world | kept | discarded |
|---|---|---|
| chromium ade20, ade200, ade-agents | 3 each, load1 0.08 to 0.77 | none |
| chromium ade-idle | 3 (load1 0.06, 0.39, rerun 0.27) | rep 1, load1 1.06; gated rerun 0.27 |
| webkit all four worlds | 3 each, load1 0.18 to 0.84 | none |
| Go | 3 processes, load1 0.75, 0.40, 0.12 | none |
| base worktree ade-agents chromium | 3 (load1 0.99, 0.41, 0.31) | none |

### Chromium Pass A, median [spread] (after; before from audit)

ade-agents (heap MB / renderer PSS MB / DOM counter nodes):

| cp | heap after | PSS after | PSS before | nodes after |
|---|---|---|---|---|
| board | 36.4 | 151.2 [3.6] | 149.8 | 8201 |
| launch4 | 40.3 | 171.6 [3.0] | 170.2 | 9288 |
| panes2 | 40.4 | 175.0 [3.6] | 173.1 | 9292 |
| switch10 | 46.0 | 187.8 [3.3] | 186.1 | 12224 |
| final | 46.3 | 189.3 [4.4] | 188.1 | 12224 |
| approval | 47.1 | 192.0 [3.6] | 191.3 | 12408 |
| log3m | 52.5 | 200.0 [4.3] | 200.0 | 13149 |
| files1 | 63.1 | 228.3 [3.9] | 227.5 | 13734 |
| stopped4 | 60.8 | 215.8 [5.4] | 227.3 (probe skipped exits) | 12534 |
| left | 57.3 | 204.3 [4.6] | 215.6 | 12536 |
| back | 57.2 | 214.5 [5.0] | 244.5 | 9060 |

`back` - `board`: heap +20.8 (was +39.8), PSS +63.3 (was +94.7), nodes +859 (was +10089). The world
stops 4 sessions and keeps a task panel plus Monaco chunk, so it is not the R6 row.

R6 rows, `back` - `board` (restored open task panel adds +3.1 heap, +1338 nodes, +9.6 PSS on ade20
and ade200; R6 baseline is `board`, ade-idle has no task):

| world | heap after [before] | PSS after [before] | nodes after [before] |
|---|---|---|---|
| ade-idle | +0.9 [0.0] (+16.2) | -5.5 [0.3] (+12.0) | +200 (+7452) |
| ade20 | +3.8 [0.0] (+20.0); vs `task` +0.8 | +5.0 [0.5] (+22.9); vs `task` -5.1 | +1531 (+9429); vs `task` +193 |
| ade200 | +3.9 [0.0]; vs `task` +0.7 | +3.2 [2.1]; vs `task` -6.2 | +1531; vs `task` +193 |

ade20 `back2`, `back3` heap 38.3, 38.6 (flat, before 54.5, 54.7). Nodes 8854 all three returns.

Targets: Chromium heap <= 2 MB met (ade-idle +0.9; ade20 and ade200 +0.7 to +0.8 net of the
restored panel); Chromium PSS <= 10 met. Spec `ade-v2-retention` asserts it.

### WebKit Pass A (WebContent PSS MB, median [spread])

| world | board | left | back | back - board (before) | back2, back3 - board |
|---|---|---|---|---|---|
| ade-idle | 422.6 [14.4] | 427.2 | 458.6 [7.1] | +33.4 [9.9] (+51.3 [6.4]) | n/a |
| ade20 | 427.9 [8.8] | 457.3 | 498.5 [2.1] | +70.8 [6.9] (+78.0 [8.5]); vs `task` +45.9 | +118.0 [76.4], +94.4 [46.9] |
| ade200 | 430.4 [16.1] | 454.3 | 470.5 [9.5] | +38.3 [23.8] (+58.4 [4.8]); vs `task` +16.4 | +37.5, +55.2 |
| ade-agents | 425.4 [10.4] | 703.5 | 759.0 [34.5] | +325.6 | n/a |

WebKit R6 target (<= 10 MB) missed: ade-idle +33.4, ade20 +70.8. Cause not provable here: the probe
has no forced GC on WebKit (no `gc()`, no CDP), so JSC garbage and bmalloc freed pages are not
returned between `left` and `back`; `left` reads equal to or above `board`. Chromium (forced GC,
heap snapshot) shows the old tree gone. WebKit `back2`/`back3` do not step up per cycle in ade200
(+37.5, +55.2) and ade20 spreads 47 to 76, so a per-cycle leak is not evidenced.

WebKit other rows vs audit, all inside noise: ade-agents `board` 425.4 (435.7 [12.1]), `launch4`
566.1 (570.2 [14.3]), `stopped4` 702.9 [43.4] (714.3 [33.4]), `back` 759.0 (771.3 [2.5]); ade-idle
`idle60` 413.6 (418.5).

### Record

| part | after MB | before MB |
|---|---|---|
| (a) WebKit WebContent PSS ade-agents `final` | 572.4 [14.1] | 556.0 [24.6] |
| (b) Go anon `idle60` | 58.6 [2.9] | 66.0 [2.8] |
| harness base64 (3 runs 11.64, 11.55, 11.02) | 11.55 [0.62] | 11.5 |
| (b) minus base64 | 47.0 | 54.5 |
| (c) helpers | 0 | 0 |
| total | 619.4 | 610.5 |

Target 606.2 not reached as measured: (a) moved +16.4, inside its noise (24.6) and not touched by
any fix (F3 is Go-side); (b) moved -7.5 (heapInuse 29.0 [0.5] to 24.6 [1.2], -4.4, matches the ring
saving 4.3). Go portion alone beats its target.

Secondary rows: Kira-owned at `back` = 759.0 + (54.4 - 11.55) = 801.8 (before 771.3 + 43.1 = 814.4);
at `stopped4` = 702.9 + (57.8 - 11.55) = 749.2 (before 714.3 + 52.3 = 766.6). Both inside WebKit noise.

### Go probe, median [spread] (3 processes)

| cp | heapInuse MB | anon MB | goroutines |
|---|---|---|---|
| boot | 11.9 [1.4] | 36.8 [4.2] | 31 |
| board200 | 12.9 [1.2] | 38.2 [0.8] | 36 |
| sessions4 | 21.1 [2.8] | 45.3 [6.6] | 53 |
| idle60 | 24.6 [1.2] (was 29.0) | 58.6 [2.9] (was 66.0) | 53 |
| pushes50 | 26.5 [1.4] | 62.1 [0.9] | 53 |
| stopped4 | 26.5 [1.5] | 57.8 [3.4] | 41 |
| final | 26.5 [1.4] | 54.4 [2.0] | 41 |

R8: `pprof -base heap-boot heap-idle60`, nodecount 300, 3 runs: `mobileterm` rows 0 (before 4.3 MB
`Ring.Append`). Target <= 0.3 met. Total base delta 12.15, 12.26, 11.39 MB, base64 11.64, 11.55, 11.02.

### R9 and F2 base (Pass B)

Tip, `stopped4`: JSArrayBufferData 33 objects, 325 KB (before-probe 8.4 MB; at `final` 4251, 7975 KB).
Base worktree `b34439d0d` plus only `b1d6472d1`: `stopped4` 33, 325 KB; Chromium ade-agents PSS
`stopped4` 216.0 [1.4] (tip 215.8), `final` 189.7, `back` 233.9. Reap already worked on base: branch A
confirmed, the audit 8.4 MB was the probe. Target <= 0.5 MB met. Base worktree removed.

### R6 Pass B

Vue instances at `left` (before: 5461 board to 10750 back, duplicate tree): ade20 409, ade200 409,
ade-idle 374; at `back` 5875 / 4721 / 5567 (board 5112 / 3958 / 5461, ade20 and ade200 add the
restored panel). No duplicate. Pass B ran without `KIRA_RAM_KEEP_SNAPSHOTS` (disk 3.3 GB free, 941 MB
per pass); no store-scope path walk repeated here (F1 walk and `ade-v2-retention` cover it).

### Timing

`bun run test:ui:timing`, alone, gated: 4 passed (52.1 s). No ADE selection latency spec exists.
