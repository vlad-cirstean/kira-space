package bridge

import (
	"context"

	"golang.org/x/sync/errgroup"

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

// Trackers lists the registered issue trackers by name, without probing any CLI.
func (s *IssueTrackerService) Trackers() TrackersResult {
	out := TrackersResult{Trackers: []TrackerInfo{}}
	for _, p := range s.Providers.All() {
		t := p.Terms()
		out.Trackers = append(out.Trackers, TrackerInfo{ID: p.ID(), Label: t.Label, Issue: t.Issue})
	}
	return out
}

// Tools probes every code host and issue tracker CLI, concurrently, and returns each with its setup
// steps. Refresh bypasses the cached probes.
func (s *IssueTrackerService) Tools(ctx context.Context, args ToolsArgs) (ToolsResult, error) {
	var tools []Tool
	var probes []func()
	for _, p := range s.Hosts.All() {
		cs, ok := p.(githost.CLISetup)
		if !ok {
			continue
		}
		t := p.Terms()
		i := len(tools)
		tools = append(tools, Tool{Kind: ToolKindHost, Provider: p.ID(), Label: t.Label, CLI: t.CLI, Setup: cs.Setup()})
		probes = append(probes, func() {
			st := cs.CLIStatus(ctx, args.Refresh)
			tools[i].Status = ToolStatus{
				Kind: st.Kind, Path: st.Path, Version: st.Version, Account: st.Account, Site: st.Site, Reason: st.Reason, Probed: nonNil(st.Probed),
			}
		})
	}
	for _, p := range s.Providers.All() {
		t := p.Terms()
		i := len(tools)
		tools = append(tools, Tool{Kind: ToolKindTracker, Provider: p.ID(), Label: t.Label, CLI: t.CLI, Setup: p.Setup()})
		probes = append(probes, func() {
			st := p.Status(ctx, args.Refresh)
			tools[i].Status = ToolStatus{
				Kind: st.Kind, Path: st.Path, Version: st.Version, Account: st.Account, Site: st.Site, Reason: st.Reason, Probed: nonNil(st.Probed),
			}
		})
	}
	var g errgroup.Group
	for _, probe := range probes {
		g.Go(func() error { probe(); return nil })
	}
	_ = g.Wait() // a probe reports failure in its Status
	if tools == nil {
		tools = []Tool{}
	}
	return ToolsResult{Tools: tools}, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
