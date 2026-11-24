package bridge

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/ade"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker"
)

// IssueTrackerService reads a task's linked issue and reports the local CLIs (code host and issue
// tracker) Settings explains. It is its own service so AdeTaskService stays as it is.
type IssueTrackerService struct {
	Engine    *ade.TaskBoard
	Hosts     *githost.Registry
	Providers *issuetracker.Registry
}

// TaskIssue reads the issue a task links: title, status and the pull requests that name it. A tracker
// failure comes back as Reason, not as an error. Without Fresh a kept copy answers at once, marked Stale.
func (s *IssueTrackerService) TaskIssue(ctx context.Context, args adewire.TaskIssueArgs) (adewire.TaskIssue, error) {
	if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
		return adewire.TaskIssue{}, err
	}
	res, err := s.Engine.TaskIssue(ctx, args.TaskID, args.Fresh)
	return res, adeTaskError(err)
}
