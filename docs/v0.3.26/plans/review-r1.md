# v0.3.26 review round 1

Base: `f6887ffdf` (docs: drop review round 1 findings, v0.3.25 review). HEAD: `af1d39a46`. Scope:
`git diff f6887ffdf..HEAD`: P756-P760, full-run fixes, net effect of v0.3.25 CI fixes
787bbbc98/6b2a08eeb and close commits.

Count: important 3, pre_existing 0, nit 2.

## Findings

### R1-1 important: 0109 clears the had_commits latch with no way to recount it

`apps/kira-space/internal/storage/migrations/0109_p757_merged_tip.sql:11-12` sets `had_commits = 0`
on every live stacked or non-main-based row. The same file (:6-7) clears every live `merged_at`.
The comment says "recount from the resolved base on the next build". But the latch only rises when the
branch is ahead now: `board_facts.go:417` `hadCommits := sb.HadCommits || wb.Ahead > 0`.

Scenario: task B stacks on A. B's commits already landed in A or main before upgrade, so B is ahead 0
against its resolved base. After 0109, the latch is 0 and `merged_at` is NULL.
- `mergedFacts` never takes the ancestor path, because there are no commits.
- `checkPrMerges` (`board.go`) skips the row on `!br.HadCommits && wb.Ahead == 0`, so the PR-merge
  path never re-records it either.

B loses its merged state for good: no merged badge, and `branchRisk`/`archiveTips` (`archive.go`)
treat it as unmerged, so archive warns about commit loss. This is exactly the merged-into-main
case P757 is about.

Fix: drop the `had_commits` reset. Instead, recount in Go on first build, against the resolved
base, once per row. Use a `merge-base`/rev-list of the branch tip against the old main-based count,
or keep the latch when the tip is an ancestor of the resolved base. Add a migration test with a stacked,
already-merged row.

### R1-2 important: confirmArtifacts drops a later call's confirm while one runs

`apps/kira-space/internal/ade/artifacthost.go:82` (`begin`, :49-60). While a confirm job runs for a
task, every later `Artifacts` call's confirm is a no-op. The running job compares only against its
own call's `servedArts`/`servedGaps` (:96).

Scenario:
1. Call 1 serves S1 and starts the job.
2. A board push refetches. Call 2 reads an older `boardLatest` snapshot or empty caches and serves
   S2, which differs from the confirmed result C. Its confirm is dropped.
3. The job finds `sameShare(S1, C)`, so it does not push.

The client keeps showing stale S2 (wrong PR state, missing deployment) until an unrelated board
push. The doc comment's guarantee, "pushes when it differs from what the call served", does not
hold for call 2.

Fix: keep a per-task "dirty" flag. A busy `begin` stores the latest served share. `end` re-runs one
more confirm against that share, or the job compares against the latest served share recorded under
`h.mu` and not its own.

### R1-3 important: MonacoHost keeps dead code of the deleted Kibana QueryBar

`apps/kira-space/frontend/src/studio/editor/MonacoHost.vue`. The only caller of these was
`kibana/QueryBar.vue`, which P760 deleted. No caller sets them now:
- props `monacoLanguage` (:60), `triggerCharacters` (:62), `asyncCompletionSources` (:64),
  `singleLine` (:70)
- the `submit` emit (:93-94) and `SUBMIT_ACTION` (:142)
- their uses at :147, :255-258, :262, :278, :315-329, :459, :485-487, :581-591, :762, :783, :866,
  :876-877, and CSS :895-897

`EditorAsyncCompletionSource` (`studio/editor/completion.ts:57`) is used only there.

The owner asked for no dead code. P760 left a single-line, async-completion editor mode nothing uses.

Fix: remove those props, the emit, the action, the single-line branches and CSS, and
`EditorAsyncCompletionSource`. Then typecheck.

### R1-4 nit: hidden Details tab keeps refetching Artifacts on every board push

`apps/kira-space/frontend/src/ade/v2/panel/AdeTaskPanel.vue:212-218` keeps `AdeDetailsTab` mounted
(`v-show`) once seen. `AdeDetailsDiskBanner` pauses on `active`, but `useTaskArtifacts`
(`ade/v2/queries.ts:600-605`) has no `enabled`. Every board push invalidates it, so a hidden tab
calls `Artifacts`. Each call can start a confirm job: a host round trip plus a `Board` build when
facts are not current.

Scenario: a user opens Details once, then works in Main. Every push from any task costs one
Artifacts call plus a background confirm for this task.

Fix: thread `active` into the links query as `enabled` (TanStack refetches on re-enable), as the
disk banner does.

### R1-5 nit: DiskChanges answers a stale cache across external edits during contention

`apps/kira-space/internal/ade/detailssubmit.go:427-433`. On `TryLock` failure it returns
`lastDisk`. `dropDisk` runs only in mirror, Answer, SubmitFiles, SyncTaskFiles and resolveDisk. If
the user edits a task file outside Kira while a long task op holds the lock, the banner shows the
pre-edit change list. The comment's "its own push restats" covers only the op's own changes, until
the next poll after the op ends.

The effect is short-lived and self-heals on the next poll. Fix: either state this limit in the
comment, or also serve the cache only when its generation is newer than the op's start.

## Per dimension

- Architecture, structure, maintainability, security: R1-3. No security finding: the P760 removal
  leaves no unchecked IPC path, and no credential handling changed.
- Functional correctness, business logic: R1-1, R1-2, R1-5.
- Performance, resources: R1-4. Nothing else.

## Checked, no finding

- P756: revert matches `28780367d^`. The N18/U37 guards are coherent.
- P757:
  - `mergedFacts` record/byAncestor/stale cases are fine, and so is the compare-and-clear in
    `MarkBranchFacts` (`repos/adetask.go`).
  - The `checkPrMerges` head check (`HeadSha == tip` or ancestor) is fine.
  - `archive.go` and `rebase.go` use `MergedTip == tip`/`recordedMerge` correctly.
- P758:
  - `repoShare` pure builder, `cachedRepoHost` and `sameShare` (deployment `CreatedAt` ignored) are
    fine.
  - `DiskChanges` gen-guarded `storeDisk` is fine.
  - `finishChanged` includes the parent on `entryDone`, which matches `mirrorFinish`.
  - The `idlewatch` rescan is fine.
  - `taskChanged` scoped events are fine.
- P759: proof dialog removal has no leftovers. 0110 `DROP COLUMN seen_at` is safe (no index, view or
  trigger on it).
- P760:
  - 0111 kibana delete plus saved-queries rebuild is safe with FKs on, and `migrate_p760_test.go`
    covers it.
  - The Go removals are clean: bridge `QueriesService`, `dbmcp`, `connfiles` schema and validate,
    `maskreview`, `readlane`, `timeout`.
  - i18n, site and CI are clean.
  - Only MonacoHost is left over (R1-3).
- Full-run fixes: fine.
- `docs/ARCHITECTURE.md`: the P757 bullet (~2463), P758 bullets (~3044-3048) and P759 (~3609) match
  the code. "Known open items" (~4140) holds no stale entry.
- `TestAutomationRun` (`internal/flows/adeflow/automation_test.go`, "gate both ways" :292, "normal
  script from a task" :398): no race found. `releaseClaim` then `launchHeldLocked` in
  `automation.go` runs under one lock. The failures fit load timing against the 20 s `waitFor`
  (`helpers_test.go:24`). This is not a code defect, and there is no finding.
