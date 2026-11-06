package ade

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
)

// archived.go reads an archived task with everything it kept and restores it (P399).

// archivedHostTimeout bounds each repo's host lookups; a slow host falls back to the archive snapshot.
const archivedHostTimeout = 5 * time.Second

// ArchivedTask returns the archived task, its branch rows (detached ones included), their change
// requests (live when the host answers, else the snapshot taken at archive) and review comments.
func (b *TaskBoard) ArchivedTask(ctx context.Context, taskID string) (adewire.ArchivedTask, error) {
	t, err := b.deps.Tasks.GetTask(taskID)
	if err != nil {
		return adewire.ArchivedTask{}, err
	}
	if t.ArchivedAt == nil {
		return adewire.ArchivedTask{}, invalid("task is not archived")
	}
	rows, err := b.deps.Tasks.BranchesOfTask(taskID)
	if err != nil {
		return adewire.ArchivedTask{}, err
	}
	runs, err := b.deps.Tasks.RunsOfTask(taskID)
	if err != nil {
		return adewire.ArchivedTask{}, err
	}
	reviews, err := b.deps.Tasks.Reviews()
	if err != nil {
		return adewire.ArchivedTask{}, err
	}
	statuses, err := b.deps.Statuses.List()
	if err != nil {
		return adewire.ArchivedTask{}, err
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	wt := b.toWireTask(t, ids, runs, reviews)
	wt.Notes = b.userNotes(taskID)
	decorateTask(&wt, t, b.liveWorkflows())

	out := adewire.ArchivedTask{
		Task: wt, ArchivedAt: *t.ArchivedAt, Statuses: toWireStatuses(statuses),
		Branches: make([]adewire.ArchivedBranch, 0, len(rows)),
	}
	repoIDs, byRepo := branchesByRepo(rows)
	hosts := make(map[string]archivedRepo, len(repoIDs))
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(boardFanOut)
	for _, id := range repoIDs {
		g.Go(func() error {
			res := b.archivedRepo(gctx, id, byRepo[id])
			mu.Lock()
			hosts[id] = res
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait() // archivedRepo reports failures as kind "unavailable"
	for _, id := range repoIDs {
		out.Repos = append(out.Repos, hosts[id].host)
	}
	for _, r := range rows {
		ab := adewire.ArchivedBranch{
			ID: r.ID, CodeRepoID: r.CodeRepoID, Name: r.Name, Kind: r.Kind, Base: r.Base, Origin: r.Origin,
			AddedAt: r.AddedAt, MergedAt: r.MergedAt, Removed: r.ArchivedAt == nil || *r.ArchivedAt != *t.ArchivedAt,
			BranchURL: hosts[r.CodeRepoID].urls[r.ID], Comments: hosts[r.CodeRepoID].comments[r.ID],
		}
		if r.ArchivedAt != nil {
			ab.ArchivedAt = *r.ArchivedAt
		}
		if ab.Comments == nil {
			ab.Comments = []adewire.ArchivedComment{}
		}
		if cr, ok := hosts[r.CodeRepoID].found[r.ID]; ok {
			ab.ChangeRequest, ab.PRSource = &cr, "live"
		} else if r.ChangeRequest != nil {
			ab.ChangeRequest = &adewire.ChangeRequest{Number: r.ChangeRequest.Number, Title: r.ChangeRequest.Title, State: r.ChangeRequest.State, URL: r.ChangeRequest.URL}
			ab.PRSource = "archived"
		}
		out.Branches = append(out.Branches, ab)
	}
	return out, nil
}

// archivedRepo is one repo's share of an archived task: host availability, live change requests and
// branch URLs, and the stored review comments, each keyed by branch id.
type archivedRepo struct {
	host     adewire.RepoHost
	found    map[string]adewire.ChangeRequest
	urls     map[string]string
	comments map[string][]adewire.ArchivedComment
}

func (b *TaskBoard) archivedRepo(ctx context.Context, id string, rows []model.AdeTaskBranch) archivedRepo {
	res := archivedRepo{
		host:  adewire.RepoHost{CodeRepoID: id, Kind: "unavailable"},
		found: map[string]adewire.ChangeRequest{}, urls: map[string]string{}, comments: map[string][]adewire.ArchivedComment{},
	}
	entry, err := b.openRepo(ctx, id)
	if err != nil {
		slog.Warn("ade archived: open repo", "scope", "ade", "repo", id, "err", err)
		return res
	}
	hctx, cancel := context.WithTimeout(ctx, archivedHostTimeout)
	defer cancel()
	kind, host, found, urls := resolveRepoHost(hctx, entry, rows)
	res.host = adewire.RepoHost{CodeRepoID: id, Kind: kind, Host: host}
	res.found, res.urls = found, urls
	for _, r := range rows {
		if r.Name == "" {
			continue
		}
		if cs, ok := storedComments(r); ok {
			res.comments[r.ID] = archivedCommentsOf(cs)
			continue
		}
		res.comments[r.ID] = b.archivedComments(ctx, entry, r)
	}
	return res
}

func archivedCommentsOf(cs []storedComment) []adewire.ArchivedComment {
	out := make([]adewire.ArchivedComment, 0, len(cs))
	for _, c := range cs {
		out = append(out, adewire.ArchivedComment{
			Path: c.Path, Start: c.Start, End: c.End, Body: c.Body, Kind: c.Kind, State: c.State, Reply: c.Reply, CreatedAt: c.CreatedAt,
		})
	}
	return out
}

// archivedComments reads the live review comments of a row archived before snapshots existed,
// unanchored: the branch tip may be gone. A later task reusing the name shares that session, so
// comments made after the row was archived are left out.
func (b *TaskBoard) archivedComments(ctx context.Context, entry *gitsession.RepoEntry, sb model.AdeTaskBranch) []adewire.ArchivedComment {
	if b.deps.Registry == nil || b.deps.Registry.Review == nil {
		return nil
	}
	branch, _, err := b.reviewSpellings(ctx, entry, sb)
	if err != nil {
		branch = sb.Name
	}
	cs, err := b.deps.Registry.Review.Comments(ctx, entry.Summary.RepoID, branch)
	if err != nil {
		slog.Warn("ade archived: review comments", "scope", "ade", "branch", sb.ID, "err", err)
		return nil
	}
	out := make([]adewire.ArchivedComment, 0, len(cs))
	for _, c := range cs {
		if sb.ArchivedAt != nil && c.CreatedAt.UnixMilli() > *sb.ArchivedAt {
			continue
		}
		out = append(out, adewire.ArchivedComment{
			Path: c.Path, Start: c.Range.Start, End: c.Range.End, Body: c.Body, Kind: c.Kind, State: c.State,
			Reply: c.Reply, CreatedAt: c.CreatedAt.UnixMilli(),
		})
	}
	return out
}

// RestoreTask unarchives the task into Later. Worktrees come back on next launch (ensureWorktree).
func (b *TaskBoard) RestoreTask(ctx context.Context, taskID string) (adewire.Task, error) {
	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	if err := b.deps.Tasks.RestoreTask(taskID); err != nil {
		var held *repos.BranchHeldError
		switch {
		case errors.As(err, &held):
			return adewire.Task{}, invalid("cannot restore: branch %s is on task %q", held.Branch, held.TaskTitle)
		case errors.Is(err, repos.ErrTaskNotArchived):
			return adewire.Task{}, invalid("task is not archived")
		}
		return adewire.Task{}, err
	}
	b.notifyBoard()
	b.notifySessions()
	tc, err := b.loadTaskCtx(taskID)
	if err != nil {
		return adewire.Task{}, err
	}
	b.restoreReviewComments(ctx, tc.branches)
	runs, err := b.deps.Tasks.RunsOfTask(taskID)
	if err != nil {
		return adewire.Task{}, err
	}
	reviews, err := b.deps.Tasks.Reviews()
	if err != nil {
		return adewire.Task{}, err
	}
	ids := make([]string, 0, len(tc.branches))
	for _, br := range tc.branches {
		ids = append(ids, br.ID)
	}
	wt := b.toWireTask(tc.task, ids, runs, reviews)
	wt.Notes = b.userNotes(taskID)
	decorateTask(&wt, tc.task, b.liveWorkflows())
	return wt, nil
}
