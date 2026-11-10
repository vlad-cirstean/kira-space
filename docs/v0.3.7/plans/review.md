# v0.3.7 code review, round 1

Base: `f16f8c264` (v0.3.6 merge, chapter branch point; no earlier review commit on this chapter). Scope:
`git diff f16f8c264..HEAD`, phases P590-P603. Reviewer: one Opus pass over architecture/security,
correctness and performance. Discovery through `codegraph_explore`, then targeted reads.

Covered with no finding: P590 probe (`adapterhost.Router.Probe`, `dbmcp.ensureReachable`), P592 Jira
cache, batch and watcher (`jira/client.go`, `jira/batch.go`, `ade/updates.go`), P593 threshold
(`thresholdFor`, file validation, migration 0095), P594 link parser, P595 policy, migration 0096 and
send recheck, P596 `maskreview.Covered` and its TS mirror, P597 shot folders (`bridge/proofshots.go`),
P599 `reviewChoice`, P603 migration 0097 (never widens: on only with some prompt/allow, allow only
when all four allow; drop of old columns has no index or constraint in the way), `kubeRun` gate and
post-approval re-check, `KubeClustersRepo`. P602 tests read for structure only; they never ran here.

## F1 (Medium) P598: a task whose only branch is not `mine` is refused as "no repo" and its repo cannot be picked

- Where: `apps/kira-space/internal/ade/steps.go:103-114` (`stepTargets` keys on `tc.mine` only),
  `apps/kira-space/internal/ade/runs.go:175-178` (`tc.mine` keeps `AdeBranchKindMine` only),
  `apps/kira-space/frontend/src/ade/v2/run/AdeRepoGateDialog.vue:36-40` (`taken` excludes every repo
  already on the task, any branch kind).
- Scenario: a plain task gets a colleague's existing branch through `AddExistingBranch`;
  `resolveKind` files it as `review` (`board_writes.go:458`). The task has repo A, no `mine` branch.
  Run on an automated step: `stepTargets` returns `ErrTaskNoRepo`, the bridge answers
  `E_TASK_NO_REPO` ("the task has no repo"), `stepNeedsRepo` (`progress.ts:287-292`) opens the repo
  gate. The picker hides repo A because a row on the task already uses it. The person can only add
  some other repo, so the agent then runs on a branch in the wrong repo, or Cancel and the step never
  starts. The message is also false: the task has a repo.
- Manual sessions on the same task do not hit this: `launchDirs` counts the non-mine branch as a
  read-only root, and `stepNeedsRepo` for manual steps checks `branchIds.length`. Automated and manual
  starts disagree on what "has a repo" means.
- Fix direction: decide what an automated step on a task with only non-mine branches does (offer a
  `mine` draft in the same repo, or a distinct refusal naming the branch kind), and let the gate offer
  repos whose only row is non-mine.

## F2 (Low) P590: `MarkLost` checks and acts without holding the state it checked

- Where: `apps/kira-space/internal/connections/service.go:891-904`.
- The guard (`connected`, no in-flight attempt, no teardown) is read under `s.mu`, the lock is
  dropped, then `beginTeardown` registers the teardown. Nothing aborts an attempt registered in that
  gap, unlike `Disconnect` (`abortInFlight` first).
- Scenario A: probe fails, `MarkLost` passes its check; the person clicks Connect (or Disconnect) at
  the same moment. `ConnectContext` sees no teardown yet and registers its attempt; `MarkLost` then
  calls `Backend.Disconnect` under the attempt. Final state depends on emit order: `connected` with
  the adapter torn down (the stale state P590 meant to remove), or `error` on a fresh live
  connection. With Disconnect the person ends on `error` instead of `disconnected`.
- Fix direction: take the teardown slot inside the same critical section as the check (or
  `abortInFlight` and re-check), so the check and the teardown are one step.

## F3 (Low) P595: with any rule stored and default `ask`, a redirect of an approved request is refused even to the same host

- Where: `apps/kira-space/internal/apiaccess/policy.go:186-209` (`CheckHop`),
  `apps/kira-space/internal/agentapi/resolve.go:438-453` (`redirectGuard`).
- `CheckHop` refuses every hop no allow rule matches unless the policy `AllowsAll` (zero rules and
  `ask`). The plan's reason (`plans/P595.md` "an allow rule skips the prompt") only covers requests
  that skipped approval. A request the user approved by hand gets the same treatment.
- Scenario: rules hold one unrelated deny rule (`deny DELETE *`), default `ask`. The agent sends
  `GET http://api.example.com/v1/items`; the user approves it. The server answers `301` to
  `https://api.example.com/v1/items` (or adds a trailing slash). The hop matches no allow rule and is
  refused: `redirect to api.example.com blocked: ... matches no allow rule`. With zero rules the same
  request follows a redirect to any host, so adding an unrelated deny rule breaks approved requests.
- Fix direction: for a request that went through approval (`!AllowedByRule`), let a hop to the
  approved scheme and host follow (deny rules still apply); keep the allow-only rule for requests an
  allow rule let through unprompted. Needs a decision, since the current behavior matches the row's
  Result wording.
