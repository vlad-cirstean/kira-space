package ade

import (
	"context"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// issuetracker.go surfaces a task's linked issue (P417): an issue artifact and the issue line of the task tab.
// The tracker CLI owns the credentials; this file only asks.

const (
	// taskIssueTimeout bounds TaskIssue; the artifact sources keep their own, shorter budget.
	taskIssueTimeout = 10 * time.Second
	reasonNoTracker  = "noTracker"
)

// trackerRef is the issue the task links, parsed by p: the pasted URL first, else the bare key.
func trackerRef(p issuetracker.Provider, t model.AdeTask) (issuetracker.Ref, bool) {
	for _, raw := range []string{t.JiraURL, t.JiraKey} {
		if raw == "" {
			continue
		}
		if ref, ok := p.ParseRef(raw); ok {
			return ref, true
		}
	}
	return issuetracker.Ref{}, false
}

// staticIssueArtifact is the row a task shows while its tracker cannot answer: the stored link only.
func staticIssueArtifact(p issuetracker.Provider, t model.AdeTask, ref issuetracker.Ref) adewire.Artifact {
	return adewire.Artifact{
		Kind: model.AdeArtifactIssue, Title: ref.Key, URL: t.JiraURL, Ref: ref.Key, Source: p.ID(), CreatedAt: t.CreatedAt,
	}
}

type trackerSource struct {
	b *TaskBoard
	p issuetracker.Provider
}

func (s trackerSource) Name() string { return s.p.ID() }

// Artifacts never returns an error: a tracker failure keeps the static row and adds a gap naming why.
func (s trackerSource) Artifacts(ctx context.Context, sc ArtifactScope) ([]adewire.Artifact, []adewire.ArtifactGap, error) {
	ref, ok := trackerRef(s.p, sc.Task)
	if !ok {
		return nil, nil, nil
	}
	iss, err := s.p.Issue(ctx, ref)
	if err != nil {
		return []adewire.Artifact{staticIssueArtifact(s.p, sc.Task, ref)},
			[]adewire.ArtifactGap{{Source: s.p.ID(), Reason: issuetracker.Reason(err)}}, nil
	}
	id := s.p.ID()
	title := iss.Title
	if title == "" {
		title = iss.Ref.Key
	}
	out := []adewire.Artifact{{
		Kind: model.AdeArtifactIssue, Title: title, URL: iss.Ref.URL, Ref: iss.Ref.Key, Status: iss.Status,
		Source: id, CreatedAt: sc.Task.CreatedAt,
	}}
	return out, nil, nil
}

// TaskIssue reads the issue the task links. A tracker failure comes back as Reason, not as an error;
// the call fails only when the task is unreadable.
func (b *TaskBoard) TaskIssue(ctx context.Context, taskID string) (adewire.TaskIssue, error) {
	task, err := b.deps.Tasks.GetTask(taskID)
	if err != nil {
		return adewire.TaskIssue{}, err
	}
	for _, p := range b.deps.Trackers.All() {
		ref, ok := trackerRef(p, task)
		if !ok {
			continue
		}
		ictx, cancel := context.WithTimeout(ctx, taskIssueTimeout)
		iss, err := p.Issue(ictx, ref)
		cancel()
		out := adewire.TaskIssue{
			Provider: p.ID(), Label: p.Terms().Label, Key: ref.Key, URL: task.JiraURL,
			Summary: adewire.IssueSummary{Providers: []string{}},
		}
		if err != nil {
			out.Reason = issuetracker.Reason(err)
			return out, nil
		}
		out.Key, out.URL, out.Title = iss.Ref.Key, iss.Ref.URL, iss.Title
		out.Status, out.StatusCategory = iss.Status, iss.StatusCategory
		out.Summary = adewire.IssueSummary{
			Known: iss.Summary.Known, Count: iss.Summary.Count, State: iss.Summary.State, Providers: append([]string{}, iss.Summary.Providers...),
		}
		return out, nil
	}
	return adewire.TaskIssue{Reason: reasonNoTracker, Summary: adewire.IssueSummary{Providers: []string{}}}, nil
}
