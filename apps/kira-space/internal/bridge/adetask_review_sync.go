package bridge

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
)

// adetask_review_sync.go: freshness of a review's branch and base against their remotes (P581).

// ReviewSync reports how far the review's local branch and base are behind their remote twins.
func (s *AdeTaskService) ReviewSync(ctx context.Context, args adewire.BranchArgs) (adewire.ReviewSync, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.BranchID, "branchId"); err != nil {
		return adewire.ReviewSync{}, err
	}
	r, err := s.Engine.ReviewSync(ctx, args.BranchID)
	return r, adeTaskError(err)
}

// ReviewFetch fetches the remotes the review's branch and base depend on.
func (s *AdeTaskService) ReviewFetch(ctx context.Context, args adewire.BranchArgs) (adewire.ReviewFetchResult, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.BranchID, "branchId"); err != nil {
		return adewire.ReviewFetchResult{}, err
	}
	r, err := s.Engine.ReviewFetch(ctx, args.BranchID)
	return r, adeTaskError(err)
}

// ReviewUpdateLocal fast-forwards the review's local branch or base to its remote twin.
func (s *AdeTaskService) ReviewUpdateLocal(ctx context.Context, args adewire.ReviewUpdateLocalArgs) (adewire.ReviewSync, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.BranchID, "branchId"); err != nil {
		return adewire.ReviewSync{}, err
	}
	if args.Side != "branch" && args.Side != "base" {
		return adewire.ReviewSync{}, adeTaskInvalid("side must be branch or base")
	}
	r, err := s.Engine.ReviewUpdateLocal(ctx, args.BranchID, args.Side)
	return r, adeTaskError(err)
}
