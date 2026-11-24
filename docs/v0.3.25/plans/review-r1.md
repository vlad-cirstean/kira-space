# v0.3.25 review round 1

Base: `07cca0cf3` (v0.3.24 on `main`). Scope: `git diff 07cca0cf3..HEAD`, P747 to P754. Three
dimensions: architecture/maintainability/security, functional correctness, performance.

Counts: 1 important, 6 nits. Groups are by root cause; fix and commit one group at a time.

## Group A: branch link marks change request seen (important)

### 1. Branch link click clears the unseen change request dot

- Severity: important.
- Where: `apps/kira-space/frontend/src/ade/v2/panel/AdeBranchPanel.vue:196-200`, used at `:428`
  (branch link) and `:454` (PR link).
- Defect: P751 Part 1 folded the removed eye button's `markItemSeen('cr:…')` into the shared
  `open()`. Branch link also calls `open(branchUrl, e)`, so opening the branch in the browser marks
  the change request update seen.
- Scenario: PR gets a new review or a push; row shows unseen dot. User clicks branch name to see the
  branch on GitHub. Dot clears, though user never opened the PR. Update is lost.
- Fix: mark `cr:` only in the PR link handler (`:454`). Branch link opens without marking (or marks
  `branch:` only, matching the watcher at `:192`). UI spec: branch link click keeps CR unseen, PR link
  click clears it.

## Group B: proof step invalidation rests on a wrong claim (nit)

### 2. Board push refetches every open proof step for nothing

- Severity: nit (performance).
- Where: `apps/kira-space/frontend/src/ade/queries.ts:91-92` (`proofStepPrefix` at `:33`).
- Defect: invalidation assumes an accepted edit keeps `rev`. It does not: `ProofCache.Propose` inserts
  rev `cur+1`, `Accept` (`internal/repos/proof.go:276-323`) sets `CurrentRev` to it, and
  `TaskProof.Rev` is `CurrentRev`. Flow test `detailsautoupdate_test.go:113` and `plans/P753.md:59`
  both say the rev bumps. Step keys `['proof','step',id,rev,ordinal]` are immutable; the refetched
  proofs list yields new keys for changed proofs.
- Scenario: Details panel with several expanded diagram steps, agent run active. Every board push
  (status sync, runs, handovers, any save) refetches every expanded step over IPC with a DB read,
  across all tasks, even ones untouched.
- Fix: drop the `proofStepPrefix` invalidation and the constant. Confirm UI spec
  `ade-v2-details-autoupdate.spec.ts` still passes through the rev change.

### 3. Docs repeat the wrong claim and a removed query

- Severity: nit (docs).
- Where: `docs/v0.3.25/SPEC.md:68-69` (P750 Result: "an accepted edit keeps `rev`, so steps are
  invalidated by prefix"); `docs/ARCHITECTURE.md:2476` ("disk changes, issue and proofs, and every
  open proof step").
- Defect: rev bumps on accept (see 2). `issuePrefix` invalidation was removed with the issue tracker
  in P751 Part 2.
- Fix: correct P750 Result wording; in ARCHITECTURE drop "issue" and, with 2, "every open proof step".

## Group C: idle watcher (nit)

All in `apps/kira-space/internal/ade/idlewatch.go`.

### 4. Agent-covered check reads `watches`, not `grants`

- Severity: nit (correctness, edge path).
- Where: `idlewatch.go:157-163` (`fire`).
- Defect: comment says agent-covered tasks are left out so an unapplied agent save never shows as a
  banner row. Check is `iw.b.watches[id] == nil`. When `newTaskWatch` fails for a task with a live
  grant, `watches[id]` stays nil while the agent runs.
- Scenario: watch setup fails (inotify limit hit, folder briefly missing) for a running agent. Agent
  saves a proof; idle watcher pushes; Details banner shows the agent's unapplied edit as an outside
  change, the exact case the comment rules out.
- Fix: skip when `b.grants[id] > 0` (or either check). Both read under `watchMu`.

### 5. Kira's own task-file writes trigger a redundant push

- Severity: nit (performance).
- Where: `idlewatch.go:93-117` (`handle`).
- Defect: any `keepTaskFile` path queues a push, including files Kira writes itself (notes,
  handovers, `links.yaml` mirror). Those writes already push the board.
- Scenario: add a note: panel push, then a second push about 300 ms later, and a second refetch of
  handovers, artifacts, disk changes and proofs.
- Fix (optional): drop events for paths with an open `task_file_intents` row or a hash equal to the
  stored one. Leave as is if the extra refetch is judged cheap; say so in the fixer's commit.

### 6. Archived task folders stay watched; initial walk holds `watchMu`

- Severity: nit (resources).
- Where: `idlewatch.go:37-62` (`startIdleWatch`), `skipDir` at `:74-84`.
- Defect: watcher walks all of `FilesDir` (`~/tasks`). `removeTaskDir` clears `TasksDir`
  (`~/ade/tasks`) only, so archived task folders stay and are watched for the app's life: one inotify
  watch per directory on Linux, growing with every archived task. The walk runs under `watchMu`, so an
  agent `watchTask` waits for it at startup.
- Scenario: long-lived install with hundreds of archived tasks with proof folders: thousands of
  inotify watches, closer to `max_user_watches`; first agent start after launch stalls behind the walk.
- Fix: skip folders of archived tasks in `skipDir` (or watch only live task folders and add/remove on
  task lifecycle); run the walk outside `watchMu`, taking it only to publish the watcher.

## Group D: stale "host view" comments after P751 (nit)

### 7. Comments name the removed host view

- Severity: nit (maintainability).
- Where: `apps/kira-space/frontend/src/ade/v2/pr/PrGlyph.vue:13`, `:43`;
  `apps/kira-space/frontend/src/ade/v2/pr/prGlyph.ts:7`, `:9`;
  `apps/kira-space/internal/flows/captureflow/doc.go:2`;
  `apps/kira-space/tests/ui/support/mockRuntime.ts:697`.
- Defect: comments describe readers and phrases of the deleted host view window. Misleads readers
  about who consumes `prGlyph` and what CaptureService flow drives.
- Fix: drop the host view mentions; reword `:9` to "the board wire".

## Nothing found

- Security: no new credential reads, shell-outs or unvalidated input; links open via `openExternal`.
- Migrations 0107 and 0108: correct. 0107 cascades tabs and layout through FK; 0108 table rebuild has
  no indexes or triggers to restore; `tracking.issue` setting and `tracker` op_log rows go.
- P747: timeout attribution (`ctx.Err() == nil`) and `RestoreLaunchTTL` correct.
- P749: `FitSize`, `cascadeSized`, proof bounds arithmetic correct and tested.
- P751 backend: `GitHostService.Tools` concurrent probes safe; `ParseJiraRef` matches old parser;
  `ArtifactGap.Reason` removal consistent across Go, agentmcp and TS; change request tracker intact.
- P752: header spacing fix sound.
- P753: version moves only on content writes; event emitted once per accepted edit, delete or dropped
  pending; marker frozen during capture; Refresh excluded from capture.
- P754: tokens and guard N18 consistent; keep awake dot uses existing `#badge` slot.
- i18n: no key used at base became orphaned at HEAD.
