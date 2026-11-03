package bridge

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
)

// GitGraphStatusArgs is GraphStatus's request.
type GitGraphStatusArgs struct {
	WindowKey string                   `json:"windowKey"`
	Params    gitrpc.GraphStatusParams `json:"params"`
}

// GraphStatus answers graph.status.
func (s *GitService) GraphStatus(ctx context.Context, a GitGraphStatusArgs) (gitrpc.GraphStatusResult, error) {
	return callGit[gitrpc.GraphStatusResult](ctx, s.windows, a.WindowKey, "graph.status", a.Params)
}

// GitGraphLoadMoreArgs is GraphLoadMore's request.
type GitGraphLoadMoreArgs struct {
	WindowKey string                     `json:"windowKey"`
	Params    gitrpc.GraphLoadMoreParams `json:"params"`
}

// GraphLoadMore answers graph.loadMore.
func (s *GitService) GraphLoadMore(ctx context.Context, a GitGraphLoadMoreArgs) (gitrpc.GraphLoadMoreResult, error) {
	return callGit[gitrpc.GraphLoadMoreResult](ctx, s.windows, a.WindowKey, "graph.loadMore", a.Params)
}

// GitGraphRefreshArgs is GraphRefresh's request.
type GitGraphRefreshArgs struct {
	WindowKey string                    `json:"windowKey"`
	Params    gitrpc.GraphRefreshParams `json:"params"`
}

// GraphRefresh answers graph.refresh.
func (s *GitService) GraphRefresh(ctx context.Context, a GitGraphRefreshArgs) (gitrpc.GraphRefreshResult, error) {
	return callGit[gitrpc.GraphRefreshResult](ctx, s.windows, a.WindowKey, "graph.refresh", a.Params)
}

// GitGraphReportFailureArgs is GraphReportFailure's request.
type GitGraphReportFailureArgs struct {
	WindowKey string                          `json:"windowKey"`
	Params    gitrpc.GraphReportFailureParams `json:"params"`
}

// GraphReportFailure answers graph.reportFailure.
func (s *GitService) GraphReportFailure(ctx context.Context, a GitGraphReportFailureArgs) (struct{}, error) {
	return callGit[struct{}](ctx, s.windows, a.WindowKey, "graph.reportFailure", a.Params)
}

// GitResetWindowArgs is ResetWindow's request.
type GitResetWindowArgs struct {
	WindowKey string `json:"windowKey"`
}

// ResetWindow drops the window's Conn and every hold, walk and subscription on it. A page calls it
// once per load, so a reload does not inherit the previous page's holds. It only touches the
// caller's own key.
func (s *GitService) ResetWindow(_ context.Context, a GitResetWindowArgs) (struct{}, error) {
	s.windows.Reset(a.WindowKey)
	return struct{}{}, nil
}
