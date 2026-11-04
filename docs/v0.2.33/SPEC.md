# v0.2.33

Branch `chapter/v0.2.33`. First `P` is 507 (highest in v0.2.32 is P506), last is P507.

Source: user request. One phase. It closes the chapter. No code review. Scope is closed.

| Phase | Title | Status |
|---|---|---|
| P507 | Deterministic removed-entry order | Todo |

## Rows

### P507 Deterministic removed-entry order

- Cause: PR #27 (chapter v0.2.32) merged as `af368d446` before `c9fddf122` on `origin/chapter/v0.2.32` could ride it. `CaptureWorkingRemoved` and `captureCommittedRemoved` in `apps/kira-space/internal/gitsession/removed.go` ranged over Go maps (`review.WorkingRecords`, `review.Records`). Autoincrement ids in the removed table followed random path order. `TestReviewWorkingRemoved` (contract `tests/contract/git-review-removed-working.json`) failed about 5 in 40 runs.
- Fix: iterate `slices.Sorted(maps.Keys(records))`. Cherry-pick `c9fddf122`. Grep the rest of `gitsession`, `gitreview` for the same pattern.
- No review.
