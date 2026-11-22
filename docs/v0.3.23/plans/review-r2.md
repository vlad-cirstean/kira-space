# v0.3.23 code review, round 2

Base commit: `c7534c9f0` (v0.3.22 squash merge, #55), same base as round 1. Diff `c7534c9f0..1e885ab3b`:
P716 to P727 plus round 1's fixes (`861ee60a4`, `736267f7b`, `0859f42c9`, `04eeb15f5`), all treated as
unreviewed. Round 1's fixes got special scrutiny: `UnlockVariables` behind the localauth gate, the
store thunk for typed secrets, undo in `editVariablesLocked`, `noteRouted`/`replayAdopted`, the
`prepareTerminalMove` exit wait, and `Registry.Open` pump tracking.

Counts: 1 important, 2 nit, 0 pre-existing.

## Important

### R2-F1 Unlocking an agent-shared secret leaves it shared with every host

- Severity: important. Dimension: (1) security.
- Where: `apps/kira-space/internal/apifiles/variables_edit.go:438-459` (`OpUnlock`). Consumers:
  `apps/kira-space/internal/agentapi/policy.go:31-50` (`availableOf`) and
  `apps/kira-space/internal/agentapi/resolve.go:246`.
- Failure: an environment secret shared with agents has `agent: true` and `agentHosts:
  [api.example.com]`. `availableOf` lets the agent insert it only into requests to those hosts.
  Unlock removes `secret` and `agentHosts` (rule `agentHostsMisplaced` forbids hosts on a plain value)
  but keeps `agent: true`. The value becomes a plain shared variable, and `availableOf` puts plain
  values into requests to any host. An agent can then send the former secret to a host the user
  never allowed, with no prompt when the environment auto-approves reads. The unlock confirmation
  says only that the value goes to the file in plain text, not that host binding is lost. Round 1's
  R1-F4 fixed the mirror case (`lock` now drops `agent` and `agentHosts`). Unlock was not changed,
  and no flow case unlocks a shared secret.
- Fix: in `OpUnlock`, also `removeKey(v, "agent")` when the variable had `agentHosts` (or always,
  same as `lock`), so the user shares it again on purpose. Add a step to `TestApiVariablesMatrix`:
  unlock a shared secret, then assert the file has no `agent` key and `AgentAccess` no longer lists it.

## Nit

### R2-F2 With OS auth available, one click writes a secret to the file in plain text

- Severity: nit. Dimension: (1) security and UX.
- Where: `apps/kira-space/frontend/src/studio/api/VariablesMatrix.vue:133-142`,
  `apps/kira-space/frontend/src/studio/api/state/variablesMatrix.ts:47-61`,
  `apps/kira-space/internal/bridge/apiconfig.go:178-188`.
- Failure: before R1-F1, the row lock button always opened the danger confirm ("{name} stops being
  a secret and its value is written to the file in plain text"). Now the confirm shows only on
  `confirmation-required`, that is only where the OS cannot authenticate. On a Mac inside the 5-minute
  grace window (any connection, variable or kept reveal opens it), `Authorize` grants with no prompt.
  So one click on the row's lock icon writes every holder's secret to its file as plain text and
  deletes the sealed value, with no question asked. Outside the grace window, the only text the user
  sees is the OS sheet's "unlock a saved variable value", which does not say plain text on disk. The
  gate authenticates the user. It does not confirm a deliberate destructive action, the job the
  confirm used to do.
- Fix: show the danger confirm first on every unlock (deliberate-action gate). After a yes, call
  `UnlockVariables` (the gate still requires OS auth where available). Keep the second confirm only
  for the `confirmation-required` path, or reuse the first answer as `confirmed`.

### R2-F3 `waitExited` misses an exit event that already landed, so the move waits the full 2 s

- Severity: nit. Dimension: (3) performance (latency).
- Where: `packages/workbench/src/state/createTerminalsStore.ts:170-181` (`waitExited`), caller
  `:395-402` (`prepareTerminalMove`).
- Failure: `Hold` answers `live: false` only after the pump is untracked. Untracking runs in
  `onDone`, which runs after the exit event was emitted (`internal/terminal/service.go` `flush`). So
  the exit event is always sent before the Hold reply, and it often reaches the renderer first.
  `applyTerminalStatus` then sets `sess.status = 'exited'` while `prepareTerminalMove` still awaits
  `terminalHold`. `waitExited` then registers a waiter for an event that already fired, and always
  runs into `TIMING.terminalMoveSettle` (2000 ms) before serializing. `waitRouted` avoids this with
  an early return; `waitExited` has none. This is the same class as R1-F5, in the path R1-F6 added.
- Fix: return at once when the session already ended, e.g. in `prepareTerminalMove` use
  `else if (sess.status === 'running') await waitExited(...)`, or check `byTabId.get(tabId)?.status`
  at the top of `waitExited`.

## Nothing real found

- R1-F1 gate: `UnlockVariables` calls the shared `*localauth.Authorizer` (`appwire/dbmodule.go:318`),
  sets the store's per-edit `Confirmed` only after a grant, and ignores the renderer's flag where OS
  auth is available. Grace semantics match every other reveal. `EditVariables` refuses an unlock op,
  and no other caller reaches `Store.EditVariables` (no agent, HTTP or phone path).
- R1-F2 thunk: the closure in `variablesMatrix.edit` captures only `held`, which it clears on first
  run. Mutations do not retry (desktop default), so no second run sends an empty batch. No
  `useMutation` keeps `secretValue`.
- R1-F3 undo: files are restored before references are re-read. Old values of overwritten secrets
  are put back. A new secret is deleted only when no file names it, and kept when references cannot
  be read. A failed restore leaves the reference, so its secret is kept. That is the safe side.
- R1-F5: `noteRouted` already runs for chunks queued while adopting, so `max(routed, result.end)`
  keeps the higher end.
- R1-F6 status: a not-live terminal is never sent as `running`. The only remaining issue is R2-F3.
- R1-F7: `Registry.Open` registers `p.out` under `r.mu` before `go sess.readLoop()`, so `onDone`
  cannot run before tracking. The doomed path never tracks. `untrackOutput` matches by pointer, so
  reusing an id is safe.
- Terminal move otherwise: lock order `Registry.mu` then coalescer `mu` holds. `Reown` keeps
  `sess.windowKey` so `remove` finds the right window. `ForgetMoves` runs on every window close and
  clears holds whose target or source closed. Held buffer capped at 1 MiB. Multi-tab prepare
  releases every hold it took on any failure.
- Tab move storage and inbox: one transaction with an ownership check, created window deleted on
  failure, inbox dropped on window close, snapshot budget enforced on both sides.
- Kube scripts: prepare cannot change cluster, tool or verb. The approval card, statement and
  namespace use the prepared request. Required args are checked after prepare. Kept reads are plain
  only. The transform failure text is masked. A withheld review drops staged values.
- Kept secrets: `kept.setSecret` values are masked from transform output, previews are suppressed,
  and staged reveal goes through the same gate. `onceRead` blocks derived keeps.
- Migration 0105 is additive with a default and CHECK. No legacy path from an older minor was added.
- G5 (CLI credentials): no change under `githost/`, `issuetracker/`, `internal/docker` or
  `internal/harness/claude`.
- `verify-packaging.sh` `source_grep`: its excluded dir names match no tracked source dir.
- Performance: beyond R2-F3, nothing real.
