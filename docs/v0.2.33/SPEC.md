# v0.2.33

Branch `chapter/v0.2.33`. First `P` is 507 (highest in v0.2.32 is P506), last is P507.

Source: user request. One phase. It closes the chapter. No code review. Scope is closed.

| Phase | Title | Status |
|---|---|---|
| P507 | Deterministic removed-entry order | Done |

## Rows

### P507 Deterministic removed-entry order

- Cause: PR #27 (chapter v0.2.32) merged as `af368d446` before `c9fddf122` on `origin/chapter/v0.2.32` could ride it. `CaptureWorkingRemoved` and `captureCommittedRemoved` in `apps/kira-space/internal/gitsession/removed.go` ranged over Go maps (`review.WorkingRecords`, `review.Records`). Autoincrement ids in the removed table followed random path order. `TestReviewWorkingRemoved` (contract `tests/contract/git-review-removed-working.json`) failed about 5 in 40 runs.
- Fix: iterate `slices.Sorted(maps.Keys(records))`. Cherry-pick `c9fddf122`. Grep the rest of `gitsession`, `gitreview` for the same pattern.
- No review.
- Result: cause and fix as above; fix cherry-picked as `dbf845a0e`. Found by the P506 follow-up while chasing a pre-push failure. Grep of `gitsession` and `gitreview` for map ranges found no other order-sensitive instance (remaining map ranges only delete or dispose, or already sort, e.g. `working_review.go` promotion). `go vet` clean on `gitsession`. `go test -count=40 -run TestReviewWorkingRemoved ./apps/kira-space/internal/flows/adeflow/`: 0 failures. `gitsession/...` and `flows/reviewflow` pass. Not run: container tests (no Docker). No review.
