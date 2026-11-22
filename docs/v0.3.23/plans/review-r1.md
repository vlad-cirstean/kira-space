# v0.3.23 code review, round 1

Base commit: `c7534c9f0` (v0.3.22 squash merge, #55). v0.3.22's two review rounds and their fixes
live inside that squash; nothing reviewed lands after it. Diff `c7534c9f0..91a30b490`: 128 commits,
P716 to P727 (P721 cleanups included). Reviewed with focus on secret handling, terminal Hold/Adopt
and lock order, atomic tab move, EditVariables, held-result unification, migration 0105.

Counts: 2 important, 6 nit, 0 pre-existing.

## Important

### R1-F1 Unlock bypasses the fingerprint gate on API secrets

- Severity: important. Dimension: security.
- Where: `apps/kira-space/internal/apifiles/variables_edit.go:400-421` (`OpUnlock`),
  `apps/kira-space/internal/bridge/apiconfig.go:90-108`, caller
  `apps/kira-space/frontend/src/studio/api/VariablesMatrix.vue:126-141`.
- Failure: every other read of an API variable secret goes through `localauth.Gated`
  (`apivars.Service.Reveal`, `RevealHistory`); `Confirmed` there is honoured only when the OS cannot
  authenticate. `EditVariables` with `op: "unlock", confirmed: true` trusts the flag, calls
  `Secrets.Reveal` with no gate and writes the plaintext into the environment or collection file.
  The renderer then reads it back with `ApiConfigService.File` (YAML view). Any renderer call (or a
  compromised renderer) gets every API secret without a fingerprint. The UI only shows a plain
  confirm dialog. Plan `P720.md` line 191 and the field comment ("after the reveal gate") both
  promise the existing reveal gate; it is not there.
- Fix: run unlock behind the same `*localauth.Authorizer` as `apivars.Reveal` (a gated method that
  returns `confirmation-required` / `cancelled` outcomes, `Confirmed` honoured only where the OS
  cannot authenticate), or gate `EditVariables` when any edit is an unlock. Add a flow case: unlock
  with `confirmed: true` on a machine with local auth available is refused without a grant.

### R1-F2 Typed secret stays in the TanStack mutation for the app's lifetime

- Severity: important. Dimension: security.
- Where: `apps/kira-space/frontend/src/studio/api/state/variablesMatrix.ts:33-37`,
  `apps/kira-space/frontend/src/studio/api/state/apiQueries.ts:357-376`.
- Failure: `useEditVariables()` is created inside the `variablesMatrix` Pinia setup store, which
  never unmounts. Its `MutationObserver` stays subscribed to the last mutation, so that mutation
  and its `variables` (the edit list, `secretValue` plaintext included) stay in the mutation cache
  and in the observer's reactive state until the next edit. `gcTime: 0` only applies once a
  mutation has no observer, so it never triggers here. Same class as v0.3.22 round 1 F2 (revealed
  secret held in TanStack); `useSetApiSecret` relies on component unmount, this one has none.
- Fix: call `editMutation.reset()` in a `finally` after `mutateAsync` settles (or create the
  mutation in the component that owns the secret input), and keep `gcTime: 0`. Optionally a unit
  check that `queryClient.getMutationCache().getAll()` holds no `secretValue` after an edit.

## Nit

### R1-F3 EditVariables undo can delete a secret a written file references

- Severity: nit. Dimension: correctness.
- Where: `apps/kira-space/internal/apifiles/variables_edit.go:166-189`.
- Failure: secrets are sealed first, then files are saved one by one. If file 2's `saveLocked`
  fails (disk error, external edit conflict), file 1 is already written, but `undo` deletes every
  newly created secret, including ones file 1 now names: file 1 ends with an unset secret. A `set`
  on an existing secret overwrites its value and `undo` never restores it. The doc comment says a
  failed call writes nothing.
- Fix: on a mid-batch save failure, restore the files already saved (keep their pre-edit text and
  hash) before undoing secrets, or delete only secrets referenced by no saved file. Keep the old
  value of overwritten secrets and put it back on undo, or state the partial outcome in the comment.

### R1-F4 Locking an agent-shared plain variable is always refused

- Severity: nit. Dimension: correctness.
- Where: `apps/kira-space/internal/apifiles/variables_edit.go:389-399` (`OpLock`), rule
  `apps/kira-space/internal/apifiles/rules.go:135-136`.
- Failure: an environment variable with `agent: true` and a plain value becomes a secret with
  `agent: true` and no `agentHosts`. Validation then fails with `agentHostsRequired`, so the whole
  lock batch from the row's lock button (all holders) is refused with a rule message. No flow or
  UI case covers locking a shared variable.
- Fix: on lock, drop `agent` (the user re-shares with hosts) or route the UI to Agent access first,
  as share already does for a secret with no detectable host. Cover it in `TestApiVariablesMatrix`.

### R1-F5 Moving an adopted terminal again waits the full 2 s settle timeout

- Severity: nit. Dimension: performance (latency) and correctness.
- Where: `packages/workbench/src/state/createTerminalsStore.ts:406-428` (`replayAdopted`), wait at
  `:380`.
- Failure: `routedEnd` is set only by live events (`noteRouted`). After an adopt, the target window
  has no `routedEnd` for the tab, so a second move of an idle terminal calls `waitRouted(tabId,
  held.end)` with 0 routed and always runs into `TIMING.terminalMoveSettle` (2000 ms) before
  serializing.
- Fix: in `replayAdopted`, call `noteRouted(tabId, result.end)` after writing the held bytes (and
  for queued chunks newer than `result.end`, note their `end`).

### R1-F6 A terminal that exits during the move lands as "failed: terminal is not being moved"

- Severity: nit. Dimension: correctness.
- Where: `packages/workbench/src/state/createTerminalsStore.ts:376-385`,
  `internal/terminal/move.go:89-95`.
- Failure: the shell exits after the source window last saw it running. `Hold` returns
  `live: false` (pump untracked after the exit event was emitted), but the payload status is
  `live || sess.status === 'running' ? 'running'`, so `running`. The target calls `Adopt`, gets
  `ErrNotHeld` (E_CONTRACT) and marks the tab failed with that text; the real exit code is lost.
- Fix: when `live` is false, wait for the pending exit event (or ask Go for the recorded exit) and
  send `exited` with its code; at least never send `running` for a terminal Go reports not live.

### R1-F7 A session that exits during Open leaks its output pump in `Registry.outs`

- Severity: nit. Dimension: resource leak.
- Where: `internal/terminal/service.go:110-135`, `internal/terminal/move.go:23-30`.
- Failure: `Registry.Open` starts `readLoop` before it returns. A command that exits at once runs
  `onExit` (final flush, `onDone` untracks: no-op, nothing tracked yet) before `unregister`. If
  `trackOutput` runs in that window, `r.sessions[id]` still holds the session, so the finished
  coalescer is stored and never removed. A later `Hold` on that id answers `live: true` for a dead
  session and holds forever. Narrow window, small leak per hit.
- Fix: track the coalescer inside `Registry.Open` under the same lock that registers the session
  (pass it in `OpenParams`), or have `trackOutput` skip a coalescer whose final flush already ran.

### R1-F8 Oversized plain value or description gets the secret-value error text

- Severity: nit. Dimension: maintainability.
- Where: `apps/kira-space/internal/bridge/apiconfig.go:98-102`.
- Failure: `Value`, `SecretValue` and `Description` share one check and one message
  (`backend.core.agentSettings.secretValue.body`), so a long description or plain value is reported
  as a bad secret value.
- Fix: check each field with its own message, or a neutral "value too long or holds NUL" text.

## Nothing real found

- Held-result unification (`dbmcp/held.go`): DB, API and kube texts share one template; the old
  per-family copies are gone; recheck runs after approval on every family.
- Kept secrets and one-time values: `kept.setSecret` masks staged values in transform output,
  API-made secrets bind to the request origin, once values never reach `Store.Put`, the masker
  copies values before `wipeOnce`, `errDerivedOnce` blocks derived keeps, staged reveal is gated.
  DB and kube secrets binding to no origin is the user's P716/P718 decision.
- Kube scripts: prepare cannot change cluster, tool or verb; secret and user kept reads refused;
  Secret describe stays redacted before transform and review.
- Tab move storage (`TabsRepo.Move`): one transaction, ownership checked, created window record
  removed on failure; frontend releases holds on every failure path.
- Terminal lock order: `Registry.mu` then coalescer `mu` holds everywhere; `onDone` runs outside
  `mu`; held buffer capped at 1 MiB.
- Migration 0105: additive column with default and CHECK. Legacy `variable-set` restore mapping
  is marked for removal at v0.4.0, consistent with "Minor bumps drop legacy code".
- CLI credentials rule G5: no change under `githost/`, `issuetracker/`, `internal/docker` or
  `internal/harness/claude`.
- Performance: beyond R1-F5, nothing real (geometry snapshot, move payload caps, kube script
  input and output caps are bounded).
