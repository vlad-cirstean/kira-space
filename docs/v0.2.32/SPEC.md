# v0.2.32

Branch `chapter/v0.2.32`. First `P` is 506 (highest in v0.2.31 is P505), last is P506.

Source: user request. One phase. It closes the chapter. No code review. Scope is closed.

| Phase | Title | Status |
|---|---|---|
| P506 | Fix repoless workflow test race | Done |

## Rows

### P506 Fix repoless workflow test race

- Cause: `TestRepolessWorkflow` snapshots `Board#started` once the fake agent's prompt file appears. The agent keeps running. Its `declare_repos` MCP call can create a branch before the snapshot on a slow runner, so the snapshot drifts from contract `ade-norepo.json`. CI failure: `go (Linux)` on PR #26.
- Fix: commit `594a2932` (merged branch, landed after PR #26 merged). Add `Action.WaitBefore` to `internal/flowtest/fakeagent/fakeagent.go`. Gate the first stage's MCP calls on a hold file in `apps/kira-space/internal/flows/adeflow/norepo_test.go`. The test releases the hold after the snapshot.
- No review.
- Result: cause and fix as above; fix cherry-picked as `fb4ee2f66`. `go vet` clean on both packages. `GOMAXPROCS=1 go test -count=3 -run TestRepolessWorkflow` passes. Full adeflow package plus `./internal/flowtest/...`: one run failed (failing test not captured), two reruns of adeflow with `-count=1` passed in about 160s. Not run: container tests (no Docker), review and full suite (test-only fix).
