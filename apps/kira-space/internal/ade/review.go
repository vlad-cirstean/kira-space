package ade

import (
	"context"
	"log/slog"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/codeworkspace"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/ids"
)

// review.go: review windows (P150). One ephemeral window per task branch reviews only that branch.

// ReviewWindowOpen is OpenReviewWindow's outcome: Existing means the window already was open and
// the caller only focuses it; otherwise the caller opens a native window with this record.
type ReviewWindowOpen struct {
	Key      string
	Title    string
	Order    int
	Existing bool
}

// refSpelling is how git names the row: the short name for a local branch, <remote>/<short> else.
func refSpelling(row porcelain.InventoryRef) string {
	if row.Remote == "" {
		return row.Short
	}
	return row.Remote + "/" + row.Short
}

// reviewSpellings resolves the branch and its base to ref spellings usable by review.* requests.
func (b *TaskBoard) reviewSpellings(ctx context.Context, entry *gitsession.RepoEntry, sb model.AdeTaskBranch) (branch, base string, err error) {
	inv, err := entry.BranchInventory(ctx)
	if err != nil {
		return "", "", err
	}
	remote, _ := entry.DefaultRemote(ctx)
	row, ok := resolveQueuedRef(inv, sb.Name, remote)
	if !ok {
		return "", "", invalid("branch %s not found in its repo", sb.Name)
	}
	branch = refSpelling(row)
	sc, err := b.baseCtxFor(ctx, entry, sb.CodeRepoID)
	if err != nil {
		return "", "", err
	}
	br := sc.resolveBase(sb)
	switch {
	case br.ok:
		return branch, br.ref, nil
	case br.reason == baseMissing:
		return branch, cmpNonEmpty(sb.Base, br.name), nil
	case br.reason == baseParentDraft:
		return "", "", invalid("the base of %s is not created yet", sb.Name)
	}
	return "", "", invalid("the repo has no main branch to review against")
}

func (b *TaskBoard) reviewTarget(ctx context.Context, tc *taskCtx, sb model.AdeTaskBranch) (adewire.ReviewWindowTarget, error) {
	entry, err := b.openRepo(ctx, sb.CodeRepoID)
	if err != nil {
		return adewire.ReviewWindowTarget{}, err
	}
	branch, base, err := b.reviewSpellings(ctx, entry, sb)
	if err != nil {
		return adewire.ReviewWindowTarget{}, err
	}
	worktree, err := b.worktreeOf(ctx, sb)
	if err != nil {
		return adewire.ReviewWindowTarget{}, err
	}
	return adewire.ReviewWindowTarget{
		TaskID: tc.task.ID, BranchID: sb.ID, CodeRepoID: sb.CodeRepoID, GitRepoID: entry.Summary.RepoID,
		Branch: branch, Base: base, Worktree: worktree,
		ChangeRequest: b.linkedChangeRequest(ctx, entry, tc.task.HostURL),
	}, nil
}

// linkedChangeRequest is the change request number the task's link names when that link points at
// entry's host repo; 0 otherwise. A task holds one branch per repo, so it is this branch's.
func (b *TaskBoard) linkedChangeRequest(ctx context.Context, entry *gitsession.RepoEntry, hostURL string) int {
	if hostURL == "" || b.deps.Registry == nil || b.deps.Registry.Hosts.Registry == nil {
		return 0
	}
	p, _, link, ok := b.deps.Registry.Hosts.Registry.ParseBranchLink(ctx, hostURL)
	if !ok || link.Issue || link.Number <= 0 {
		return 0
	}
	hp, hr, ok := entry.HostRepo(ctx)
	if !ok || hp.ID() != p.ID() {
		return 0
	}
	if strings.EqualFold(hr.Host, link.Repo.Host) && strings.EqualFold(hr.Owner, link.Repo.Owner) && strings.EqualFold(hr.Name, link.Repo.Name) {
		return link.Number
	}
	return 0
}

// changeRequestURL is the web link of the branch's change request: the task link's when it names
// this repo's, else the host's lookup by branch name. Empty when there is none; a host that is off
// or down never fails the caller.
func (b *TaskBoard) changeRequestURL(ctx context.Context, sb model.AdeTaskBranch, hostURL string) string {
	if sb.Name == "" || (sb.Kind != model.AdeBranchKindMine && sb.Kind != model.AdeBranchKindReview) {
		return ""
	}
	entry, err := b.openRepo(ctx, sb.CodeRepoID)
	if err != nil {
		return ""
	}
	if n := b.linkedChangeRequest(ctx, entry, hostURL); n > 0 {
		url, _ := entry.ChangeRequestURL(ctx, n)
		return url
	}
	if res := entry.ResolveBranchChangeRequest(ctx, sb.Name); res.Kind == "ok" && len(res.ChangeRequests) > 0 {
		return res.ChangeRequests[0].URL
	}
	return ""
}

// reviewableBranch rejects a branch a review cannot target: archived, not mine or review, or not created.
func (b *TaskBoard) reviewableBranch(tc *taskCtx, sb model.AdeTaskBranch) error {
	if sb.ArchivedAt != nil {
		return invalid("the branch is archived")
	}
	if sb.Kind != model.AdeBranchKindMine && sb.Kind != model.AdeBranchKindReview {
		return invalid("only a mine or review branch can be reviewed")
	}
	if sb.Name == "" {
		return invalid("the branch of %s is not created yet", b.repoNick(tc, sb))
	}
	return nil
}

// OpenReviewWindow records the review window of a created branch and pins its review session.
func (b *TaskBoard) OpenReviewWindow(ctx context.Context, branchID string) (ReviewWindowOpen, error) {
	if b.deps.ReviewWindows == nil || b.deps.Windows == nil || b.deps.Registry == nil || b.deps.Registry.Review == nil {
		return ReviewWindowOpen{}, invalid("review windows are not available")
	}
	sb, err := b.deps.Tasks.GetBranch(branchID)
	if err != nil {
		return ReviewWindowOpen{}, err
	}
	mu := b.taskMu(sb.TaskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(sb.TaskID)
	if err != nil {
		return ReviewWindowOpen{}, err
	}
	if err := b.reviewableBranch(tc, sb); err != nil {
		return ReviewWindowOpen{}, err
	}
	if existing, err := b.deps.ReviewWindows.ByBranch(sb.ID); err != nil {
		return ReviewWindowOpen{}, err
	} else if existing != nil {
		return ReviewWindowOpen{Key: existing.WindowKey, Existing: true}, nil
	}
	tgt, err := b.reviewTarget(ctx, tc, sb)
	if err != nil {
		return ReviewWindowOpen{}, err
	}
	if err := b.promoteBranchReview(ctx, sb); err != nil {
		warnPromote(sb.ID, err)
	}
	if err := b.deps.Registry.Review.SetPinned(ctx, tgt.GitRepoID, tgt.Branch, true); err != nil {
		return ReviewWindowOpen{}, err
	}
	records, err := b.deps.Windows.List()
	if err != nil {
		return ReviewWindowOpen{}, err
	}
	order := 0
	for _, r := range records {
		if r.Order >= order {
			order = r.Order + 1
		}
	}
	key := ids.NewID()
	if err := b.deps.Windows.Create(model.WindowRecord{Key: key, Order: order}); err != nil {
		return ReviewWindowOpen{}, err
	}
	row := repos.AdeReviewWindow{WindowKey: key, TaskID: sb.TaskID, BranchID: sb.ID, CreatedAt: b.deps.Now().UnixMilli()}
	if err := b.deps.ReviewWindows.Insert(row); err != nil {
		if derr := b.deps.Windows.Delete(key); derr != nil {
			slog.Warn("ade: drop orphan review window row", "scope", "ade", "key", key, "err", derr)
		}
		return ReviewWindowOpen{}, err
	}
	title := "Review · " + b.repoNick(tc, sb) + " · " + sb.Name
	return ReviewWindowOpen{Key: key, Title: title, Order: order}, nil
}

// ReviewWindowTarget returns what a review window reviews, nil when key is not a review window or
// its branch is gone.
func (b *TaskBoard) ReviewWindowTarget(ctx context.Context, key string) (*adewire.ReviewWindowTarget, error) {
	if b.deps.ReviewWindows == nil {
		return nil, nil
	}
	row, err := b.deps.ReviewWindows.ByKey(key)
	if err != nil || row == nil {
		return nil, err
	}
	sb, err := b.deps.Tasks.GetBranch(row.BranchID)
	if err != nil {
		return nil, err
	}
	tc, err := b.loadTaskCtx(row.TaskID)
	if err != nil {
		return nil, err
	}
	if sb.Name == "" || sb.ArchivedAt != nil {
		return nil, nil
	}
	tgt, err := b.reviewTarget(ctx, tc, sb)
	if err != nil {
		return nil, err
	}
	return &tgt, nil
}

// reviewWorktree loads a reviewable branch and its worktree; worktree is "" when it has none.
func (b *TaskBoard) reviewWorktree(ctx context.Context, branchID string) (*gitsession.RepoEntry, model.AdeTaskBranch, string, error) {
	sb, err := b.deps.Tasks.GetBranch(branchID)
	if err != nil {
		return nil, sb, "", err
	}
	tc, err := b.loadTaskCtx(sb.TaskID)
	if err != nil {
		return nil, sb, "", err
	}
	if err := b.reviewableBranch(tc, sb); err != nil {
		return nil, sb, "", err
	}
	entry, err := b.openRepo(ctx, sb.CodeRepoID)
	if err != nil {
		return nil, sb, "", err
	}
	worktree, err := b.worktreeOf(ctx, sb)
	return entry, sb, worktree, err
}

// ReviewWorking lists the staged, unstaged and untracked changes of the branch's worktree with
// their uncommitted review status, after moving review state onto commits made since. The worktree
// path comes from the branch row, never from the caller.
func (b *TaskBoard) ReviewWorking(ctx context.Context, branchID string) (adewire.ReviewWorking, error) {
	entry, _, worktree, err := b.reviewWorktree(ctx, branchID)
	if err != nil {
		return adewire.ReviewWorking{}, err
	}
	if worktree == "" {
		return adewire.ReviewWorking{
			Files: []porcelain.FileChange{}, Reviews: map[string]gitsession.ReviewFileStatus{}, Promoted: []string{}, ContentHashes: map[string]string{},
		}, nil
	}
	w, err := b.workingReview(ctx, branchID)
	if err != nil {
		return adewire.ReviewWorking{}, err
	}
	promoted, err := w.promote(ctx)
	if err != nil {
		return adewire.ReviewWorking{}, err
	}
	head, files, err := entry.WorktreeChanges(ctx, worktree)
	if err != nil {
		return adewire.ReviewWorking{}, err
	}
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	// One read per file serves both the review states and the hashes a list mark echoes back.
	read := make(map[string]gitsession.WorkingContent, len(paths))
	current := func(path string) (gitsession.WorkingContent, error) {
		if c, ok := read[path]; ok {
			return c, nil
		}
		c, err := w.current(ctx, path)
		if err == nil {
			read[path] = c
		}
		return c, err
	}
	if err := entry.CaptureWorkingRemoved(ctx, w.branch, current); err != nil {
		return adewire.ReviewWorking{}, err
	}
	reviews, err := entry.WorkingReviewStates(ctx, w.branch, paths, current)
	if err != nil {
		return adewire.ReviewWorking{}, err
	}
	hashes := make(map[string]string, len(paths))
	for _, path := range paths {
		c, err := current(path)
		if err != nil {
			return adewire.ReviewWorking{}, err
		}
		hashes[path] = wireHash(c)
	}
	return adewire.ReviewWorking{
		Worktree: worktree, Head: head, Files: files, Reviews: reviews, Promoted: promoted, ContentHashes: hashes,
	}, nil
}

// ReviewWorkingDiff reads one uncommitted file: the worktree HEAD side against the file on disk.
func (b *TaskBoard) ReviewWorkingDiff(ctx context.Context, branchID, path string) (codeworkspace.DiffContent, error) {
	entry, _, worktree, err := b.reviewWorktree(ctx, branchID)
	if err != nil {
		return codeworkspace.DiffContent{}, err
	}
	if worktree == "" {
		return codeworkspace.DiffContent{}, invalid("the branch has no worktree")
	}
	if _, err := codeworkspace.ValidateRelPath(worktree, path); err != nil {
		return codeworkspace.DiffContent{}, invalid("bad path: %v", err)
	}
	_, head, err := entry.WorktreeHead(ctx, worktree)
	if err != nil {
		return codeworkspace.DiffContent{}, err
	}
	cf := entry.CatFile()
	if cf == nil {
		return codeworkspace.DiffContent{}, invalid("the repo is closing")
	}
	d, err := codeworkspace.ReadDiffAt(ctx, cf, worktree, head, path)
	if err != nil {
		return d, err
	}
	d.ContentHash = wireHash(workingSide(d.Worktree))
	return d, nil
}

// closeReviewWindows closes the task's review windows and drops their records (Archive). Review
// sessions stay pinned: archived work keeps its review comments.
func (b *TaskBoard) closeReviewWindows(tc *taskCtx) {
	if b.deps.ReviewWindows == nil {
		return
	}
	if b.deps.CloseReviewWindows != nil {
		b.deps.CloseReviewWindows(tc.task.ID)
	}
	if err := b.deps.ReviewWindows.DeleteByTask(tc.task.ID); err != nil {
		slog.Warn("ade: close review windows", "scope", "ade", "task", tc.task.ID, "err", err)
	}
}
