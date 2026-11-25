# Audit: slow task Details open

Details tab (`AdeDetailsTab.vue`) remounts on every tab switch (`AdeTaskPanel.vue:92-94` v-if/v-else). Queries on mount: useDiskChanges
(`details/useDetailsDisk.ts:12`, staleTime 0), useTaskArtifacts (`queries.ts:600`), useHandovers (:568), useTaskProofs (:576),
useTaskDbAudit (:584), usePanelForward (:680). `AdeLinkStrip.vue:286-291` shows a Skeleton until artifacts resolves; perceived open time =
Artifacts latency. Backend `bridge/adetask.go:1039` Artifacts -> `ade/artifacts.go:75` taskArtifacts (quick=false; sources taskLink, repo, stored, notes).

Causes, ranked (not caused by Jira removal; unmeasured on the user's machine):
1. Synchronous GitHub lookups in Artifacts: `resolveRepoHost` (`board.go:870`) -> `ResolveBranchChangeRequest` (`gitsession/changerequests.go:539`),
   cache miss or older than 1 h -> `fetchBranchChangeRequest` (:626) = `gh api` spawn plus network (0.5-2 s per branch, serialized, capped 5 s by
   artifactSourceTimeout). Cold start adds `Discovery.probe` (`githost/github/discovery.go:167-224`): `gh --version` + `gh auth status`. `ensureSnapshot` (:421) sync when cold.
2. Artifacts recomputes facts for every live branch of the repo (`repoFacts` `board_facts.go:189`, shared=nil), duplicating the cached board (`boardcache.go:108`):
   git spawns per branch, sync mark writes, single SQLite connection (`sqlitex.go:99`). Measured (fakehost): 13 ms at 1-8 branches, 30 ms at 24, about equal to a full board rebuild.
3. Refetch storm (mild P750 regression): `queries.ts:83-88` invalidates handovers/artifacts/disk/proofs on every board push; `boardEvent.ts:22` null taskIds invalidates all tasks;
   `notifyBoard()` allChanged from AddNote, RemoveArtifact, DismissFoundLink etc.; idlewatch rescan sends empty BoardChangedEvent. With the remount every return to Details re-shows the skeleton.
4. DiskChanges waits on `taskMu` (`detailssubmit.go:426`) during long task ops (`runs.go:342`).
5. Contention: board rebuilds compete for git spawns and the one DB connection.

Fixes (priority):
a. UI Artifacts non-blocking on host lookups: answer from `CachedBranchChangeRequest` (as `quickRepoArtifacts` `artifacts.go:298`), refresh in background, push scoped taskChanged.
b. `repoArtifacts` uses cached Board snapshot facts instead of `repoFacts`; at least only the task's own rows.
c. Render stored/task/notes links before the repo source; no skeleton for the whole strip.
d. Keep Details mounted (v-show/KeepAlive) and/or `placeholderData: keepPreviousData` on useTaskArtifacts.
e. Scope notifyBoard callers with taskChanged(ids); invalidate artifacts only on pushes that change links; scoped ids in idlewatch rescan.
f. DiskChanges TryLock, return last result marked stale.
Unknown: real gh latency and cache state, macOS git status cost, first open vs every reopen. Quick confirm: time the AdeTask.Artifacts bridge call in the app log on first open and on reopen.
