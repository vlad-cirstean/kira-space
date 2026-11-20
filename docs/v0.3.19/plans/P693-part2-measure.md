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
