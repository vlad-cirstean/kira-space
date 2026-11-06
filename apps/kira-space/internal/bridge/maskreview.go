package bridge

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/maskreview"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// MaskReviewColumnsArgs asks for the live columns of one connection; Refresh bypasses the metadata cache.
type MaskReviewColumnsArgs struct {
	ID      string `json:"id"`
	Refresh bool   `json:"refresh"`
}

// ReviewColumns lists the live columns the review covers and refreshes the pending state. It works while MCP is off,
// so the user can prepare a review. Coverage is computed by the caller against its unsaved draft.
func (s *MaskRulesService) ReviewColumns(args MaskReviewColumnsArgs) (maskreview.Listing, error) {
	if args.ID == "" {
		return maskreview.Listing{}, ipcerr.Required("id")
	}
	l, err := s.Deps.MaskReview.Check(context.Background(), args.ID, args.Refresh)
	if err != nil {
		return maskreview.Listing{}, ipcerr.Wrap(err)
	}
	return l, nil
}

// MaskReviewPendingArgs names the connection whose pending count the review popup shows.
type MaskReviewPendingArgs struct {
	ID string `json:"id"`
}

// MaskReviewPending is the cached count of columns that need review.
type MaskReviewPending struct {
	Count int `json:"count"`
}

// ReviewPending reads the cached count; it runs no op.
func (s *MaskRulesService) ReviewPending(args MaskReviewPendingArgs) (MaskReviewPending, error) {
	if args.ID == "" {
		return MaskReviewPending{}, ipcerr.Required("id")
	}
	return MaskReviewPending{Count: s.Deps.MaskReview.PendingCount(args.ID)}, nil
}
