package ade

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
)

// ErrInvalidInput marks a caller mistake (bad id, bad value); the bridge maps it to E_BAD_REQUEST.
var ErrInvalidInput = errors.New("ade: invalid input")

// ErrStale marks a write built from a board the caller saw before the task moved on; the bridge
// maps it to E_CONFLICT.
var ErrStale = errors.New("ade: task moved on")

// ErrWorkingChanged marks a working-review write built from file content that has since changed;
// the bridge maps it to E_CONFLICT.
var ErrWorkingChanged = errors.New("ade: working file changed")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidInput, fmt.Sprintf(format, args...))
}

// trimInvalid is err's text without the ErrInvalidInput prefix.
func trimInvalid(err error) string {
	return strings.TrimPrefix(err.Error(), ErrInvalidInput.Error()+": ")
}

func (b *TaskBoard) newID() string { return uuid.NewString() }

// --- helpers -----------------------------------------------------------------------------------

func (b *TaskBoard) wireTask(t model.AdeTask) (adewire.Task, error) {
	branches, err := b.deps.Tasks.BranchesLive()
	if err != nil {
		return adewire.Task{}, err
	}
	var ids []string
	for _, br := range branches {
		if br.TaskID == t.ID {
			ids = append(ids, br.ID)
		}
	}
	runs, err := b.deps.Tasks.RunsOfTask(t.ID)
	if err != nil {
		return adewire.Task{}, err
	}
	reviews, err := b.deps.Tasks.Reviews()
	if err != nil {
		return adewire.Task{}, err
	}
	out := b.toWireTask(t, ids, runs, reviews)
	decorateTask(&out, t, b.liveWorkflows())
	bh, err := b.deps.Tasks.BoardHandovers()
	if err != nil {
		return adewire.Task{}, err
	}
	applyHandovers(&out, bh[t.ID])
	b.badgeTasks([]adewire.Task{out})
	return out, nil
}

// firstStepID is the id of the step a workflow's new task enters, "" when every step is skipped.
func firstStepID(wf adewire.Workflow) string {
	if i, ok := firstStep(wf); ok {
		return wf.Steps[i].ID
	}
	return ""
}

func (b *TaskBoard) requireRepos(ids []string) error {
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			return invalid("codeRepoIds must be distinct, non-empty ids")
		}
		seen[id] = true
		rec, err := b.deps.CodeRepos.Get(id)
		if err != nil {
			return err
		}
		if rec == nil {
			return invalid("code repo %s not found", id)
		}
	}
	return nil
}

// mainShortName is the repo's main branch short name, "" when it cannot be read.
func (b *TaskBoard) mainShortName(ctx context.Context, codeRepoID string) string {
	entry, err := b.openRepo(ctx, codeRepoID)
	if err != nil {
		return ""
	}
	name, _, ok, err := entry.MainRef(ctx)
	if err != nil || !ok {
		return ""
	}
	short, _ := mainDisplay(name)
	return short
}

func jiraFields(j *adewire.Jira) (key, url string) {
	if j == nil {
		return "", ""
	}
	return j.Key, j.URL
}

// --- tasks -------------------------------------------------------------------------------------

// CreateTask creates a task with one not-created branch per repo, appended to the plan on `Day` (Later when nil).
func (b *TaskBoard) CreateTask(ctx context.Context, args adewire.CreateTaskArgs) (adewire.Task, error) {
	jiraKey, jiraURL := jiraFields(args.Jira)
	title := strings.TrimSpace(args.Title)
	if title == "" && jiraKey == "" {
		return adewire.Task{}, invalid("a task needs a title or a Jira key")
	}
	if err := b.requireRepos(args.CodeRepoIDs); err != nil {
		return adewire.Task{}, err
	}
	task := model.AdeTask{
		ID: b.newID(), Kind: model.AdeTaskKindTask, Title: title, JiraKey: jiraKey, JiraURL: jiraURL,
		HostURL: args.HostURL, Notes: args.Notes, CreatedAt: b.deps.Now().UnixMilli(),
	}
	if args.WorkflowID != "" {
		if b.deps.Workflows == nil {
			return adewire.Task{}, invalid("workflow %q not found", args.WorkflowID)
		}
		wf, ok := b.deps.Workflows.Get(args.WorkflowID)
		if !ok {
			return adewire.Task{}, invalid("workflow %q not found", args.WorkflowID)
		}
		task.WorkflowID, task.EntryStepID = wf.ID, firstStepID(wf)
	}
	for id := range args.Bases {
		if !slices.Contains(args.CodeRepoIDs, id) {
			return adewire.Task{}, invalid("bases: repo %s is not on the task", id)
		}
	}
	branches := make([]model.AdeTaskBranch, len(args.CodeRepoIDs))
	for i, repoID := range args.CodeRepoIDs {
		base, baseBranchID, err := b.storedBase(ctx, repoID, b.repoNickOf(repoID), args.Bases[repoID])
		if err != nil {
			return adewire.Task{}, err
		}
		branches[i] = model.AdeTaskBranch{
			ID: b.newID(), TaskID: task.ID, CodeRepoID: repoID, Kind: model.AdeBranchKindMine,
			Position: i, AddedAt: task.CreatedAt, Base: base, BaseBranchID: baseBranchID,
		}
	}
	stored, err := b.deps.Tasks.CreateTask(task, branches, args.Day)
	if err != nil {
		return adewire.Task{}, err
	}
	b.notifyBoard()
	return b.wireTask(stored)
}

// UpdateTask applies a patch; kind moves only between task and parked. A review task that becomes
// a task gets a worktree and prepare script on each branch, as an attached own branch does.
func (b *TaskBoard) UpdateTask(ctx context.Context, args adewire.UpdateTaskArgs) (adewire.Task, error) {
	p := args.Patch
	if p.Jira != nil && p.ClearJira {
		return adewire.Task{}, invalid("jira and clearJira are mutually exclusive")
	}
	patch := model.AdeTaskPatch{Title: p.Title, HostURL: p.HostURL, Est: p.Est, Notes: p.Notes, Color: p.Color, Kind: p.Kind, StatusID: p.StatusID}
	if p.Title != nil {
		trimmed := strings.TrimSpace(*p.Title)
		patch.Title = &trimmed
	}
	switch {
	case p.Jira != nil:
		patch.JiraKey, patch.JiraURL = &p.Jira.Key, &p.Jira.URL
	case p.ClearJira:
		empty := ""
		patch.JiraKey, patch.JiraURL = &empty, &empty
	}
	var converted []model.AdeTaskBranch
	if p.Kind != nil && *p.Kind == model.AdeTaskKindTask {
		prev, err := b.deps.Tasks.GetTask(args.TaskID)
		if err != nil {
			return adewire.Task{}, err
		}
		if prev.Kind == model.AdeTaskKindReview {
			live, err := b.deps.Tasks.BranchesLive()
			if err != nil {
				return adewire.Task{}, err
			}
			for _, sb := range live {
				if sb.TaskID == args.TaskID && sb.Kind == model.AdeBranchKindReview && sb.Name != "" {
					sb.Kind = model.AdeBranchKindMine
					converted = append(converted, sb)
				}
			}
		}
	}
	task, err := b.deps.Tasks.UpdateTask(args.TaskID, patch)
	if err != nil {
		return adewire.Task{}, err
	}
	b.notifyBoard()
	for _, sb := range converted {
		b.prepareAttached(ctx, sb)
	}
	return b.wireTask(task)
}

// AddTaskRepo adds a not-created branch in another repo to a live task.
func (b *TaskBoard) AddTaskRepo(ctx context.Context, args adewire.AddTaskRepoArgs) (adewire.Branch, error) {
	if err := b.requireRepos([]string{args.CodeRepoID}); err != nil {
		return adewire.Branch{}, err
	}
	var base, baseBranchID string
	if args.Base != nil {
		var err error
		if base, baseBranchID, err = b.storedBase(ctx, args.CodeRepoID, b.repoNickOf(args.CodeRepoID), *args.Base); err != nil {
			return adewire.Branch{}, err
		}
	}
	sb, err := b.deps.Tasks.AddBranch(model.AdeTaskBranch{
		ID: b.newID(), TaskID: args.TaskID, CodeRepoID: args.CodeRepoID, Kind: model.AdeBranchKindMine,
		AddedAt: b.deps.Now().UnixMilli(), Base: base, BaseBranchID: baseBranchID,
	})
	if err != nil {
		return adewire.Branch{}, err
	}
	b.notifyBoard()
	return zeroBranch(sb, cmpNonEmpty(sb.Base, b.mainShortName(ctx, sb.CodeRepoID)), nil), nil
}

// CandidateBranches lists local and remote-only branches that are not on a live task and not the
// repo's main, newest commit first. An empty codeRepoID covers every repo.
func (b *TaskBoard) CandidateBranches(ctx context.Context, codeRepoID string) (adewire.CandidateBranchesResult, error) {
	repoList, err := b.deps.CodeRepos.List()
	if err != nil {
		return adewire.CandidateBranchesResult{}, err
	}
	live, err := b.deps.Tasks.BranchesLive()
	if err != nil {
		return adewire.CandidateBranchesResult{}, err
	}
	taken := make(map[string]bool, len(live))
	for _, br := range live {
		taken[br.CodeRepoID+"\x00"+br.Name] = true
	}
	var mu sync.Mutex
	out := make([]adewire.CandidateBranch, 0)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(boardFanOut)
	for _, rec := range repoList {
		if codeRepoID != "" && rec.ID != codeRepoID {
			continue
		}
		g.Go(func() error {
			found, err := b.repoCandidates(gctx, rec.ID, taken)
			if err != nil {
				return nil // an unreadable repo contributes no candidates
			}
			mu.Lock()
			out = append(out, found...)
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait() // per-repo failures are skipped, never fatal
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].LastCommitAt != out[j].LastCommitAt {
			return out[i].LastCommitAt > out[j].LastCommitAt
		}
		if out[i].CodeRepoID != out[j].CodeRepoID {
			return out[i].CodeRepoID < out[j].CodeRepoID
		}
		return out[i].Name < out[j].Name
	})
	return adewire.CandidateBranchesResult{Branches: out}, nil
}

func (b *TaskBoard) repoCandidates(ctx context.Context, codeRepoID string, taken map[string]bool) ([]adewire.CandidateBranch, error) {
	entry, err := b.openRepo(ctx, codeRepoID)
	if err != nil {
		return nil, err
	}
	inv, err := entry.BranchInventory(ctx)
	if err != nil {
		return nil, err
	}
	userEmail, err := entry.ConfigValue(ctx, "user.email")
	if err != nil {
		return nil, err
	}
	mainName := ""
	if ref, _, ok, err := entry.MainRef(ctx); err == nil && ok {
		mainName, _ = mainDisplay(ref)
	}
	seen := map[string]bool{}
	out := make([]adewire.CandidateBranch, 0)
	for _, remoteOnly := range []bool{false, true} {
		for _, r := range inv {
			if (r.Remote != "") != remoteOnly || r.Short == mainName || seen[r.Short] || taken[codeRepoID+"\x00"+r.Short] {
				continue
			}
			seen[r.Short] = true
			out = append(out, adewire.CandidateBranch{
				CodeRepoID: codeRepoID, Name: r.Short, Author: r.AuthorName, LastCommitAt: r.CommitterUnix * 1000,
				RemoteOnly: remoteOnly, Mine: resolveKind(r, userEmail, "") == model.AdeBranchKindMine,
			})
		}
	}
	return out, nil
}

// AddExistingBranch attaches an existing git branch to a live task, or (taskId "") to a new task:
// a review task when the tip author is not the user, else a plain task. A mine or parked branch that
// is not checked out anywhere gets a worktree and its prepare script (R19); a review branch gets
// none until Start or Take over.
func (b *TaskBoard) AddExistingBranch(ctx context.Context, args adewire.AddExistingBranchArgs) (adewire.AddExistingBranchResult, error) {
	var sb model.AdeTaskBranch
	res, err := b.attachExistingBranch(ctx, args, &sb, newTaskFields{})
	if err != nil {
		return res, err
	}
	if sb.Kind != model.AdeBranchKindReview {
		b.prepareAttached(ctx, sb)
	}
	return res, nil
}

// prepareAttached gives an attached branch its worktree and setup. A failure leaves the branch
// attached: the next Start or Take over retries through the launch gate.
func (b *TaskBoard) prepareAttached(ctx context.Context, sb model.AdeTaskBranch) {
	rec, err := b.deps.CodeRepos.Get(sb.CodeRepoID)
	if err != nil || rec == nil {
		slog.Warn("ade: attached branch worktree", "scope", "ade", "branch", sb.ID, "err", err)
		return
	}
	res, err := b.ensureWorktree(ctx, *rec, sb, newBranchName{})
	if err != nil {
		slog.Warn("ade: attached branch worktree", "scope", "ade", "branch", sb.ID, "err", err)
		b.recordSetupFailure(sb, "could not create the worktree: "+trimInvalid(err))
		return
	}
	if !res.Fresh {
		return
	}
	if err := b.startSetup(*rec, sb, res.Name, res.Path, b.onSetupReady); err != nil {
		slog.Warn("ade: attached branch setup", "scope", "ade", "branch", sb.ID, "err", err)
		b.recordSetupFailure(sb, "could not start the prepare script: "+trimInvalid(err))
	}
	b.notifyBoard()
}

// newTaskFields are the task fields an attach sets when it creates the task.
type newTaskFields struct {
	Title, HostURL string
	// ForceReview attaches the branch as review whatever its author email says; fork code is untrusted.
	ForceReview bool
}

// existingBranch is a git branch an attach resolved: its inventory row and the kind it attaches as.
type existingBranch struct {
	row  porcelain.InventoryRef
	kind string
}

// resolveExisting finds the branch in the repo's inventory and refuses one already on a live task.
// taskKind is the target task's kind ("" for a task not created yet). The caller holds the repo mutex.
func (b *TaskBoard) resolveExisting(ctx context.Context, codeRepoID, name, taskKind string, forceReview bool) (existingBranch, error) {
	entry, err := b.openRepo(ctx, codeRepoID)
	if err != nil {
		return existingBranch{}, err
	}
	inv, err := entry.BranchInventory(ctx)
	if err != nil {
		return existingBranch{}, err
	}
	remote, _ := entry.DefaultRemote(ctx)
	row, found := resolveQueuedRef(inv, name, remote)
	if !found {
		return existingBranch{}, invalid("branch %q not found", name)
	}
	userEmail, err := entry.ConfigValue(ctx, "user.email")
	if err != nil {
		return existingBranch{}, err
	}
	live, err := b.deps.Tasks.BranchesLive()
	if err != nil {
		return existingBranch{}, err
	}
	for _, br := range live {
		if br.CodeRepoID == codeRepoID && br.Name == name {
			return existingBranch{}, repos.ErrBranchOnTask
		}
	}
	kind := resolveKind(row, userEmail, "")
	if forceReview {
		kind = model.AdeBranchKindReview
	}
	if taskKind == model.AdeTaskKindParked {
		kind = model.AdeBranchKindParked
	}
	return existingBranch{row: row, kind: kind}, nil
}

// draftOf is the task's live not-created branch in the repo, nil when it has none.
func (b *TaskBoard) draftOf(taskID, codeRepoID string) (*model.AdeTaskBranch, error) {
	live, err := b.deps.Tasks.BranchesLive()
	if err != nil {
		return nil, err
	}
	for _, br := range live {
		if br.TaskID == taskID && br.CodeRepoID == codeRepoID && br.Name == "" {
			return &br, nil
		}
	}
	return nil, nil
}

func (b *TaskBoard) attachExistingBranch(ctx context.Context, args adewire.AddExistingBranchArgs, attached *model.AdeTaskBranch, nf newTaskFields) (adewire.AddExistingBranchResult, error) {
	if err := b.requireRepos([]string{args.CodeRepoID}); err != nil {
		return adewire.AddExistingBranchResult{}, err
	}
	if args.TaskID != "" {
		// Task lock before repo lock, the order every task write takes them in.
		tmu := b.taskMu(args.TaskID)
		tmu.Lock()
		defer tmu.Unlock()
	}
	mu := b.repoMutex(args.CodeRepoID)
	mu.Lock()
	defer mu.Unlock()

	var task model.AdeTask
	var err error
	if args.TaskID != "" {
		if task, err = b.deps.Tasks.GetTask(args.TaskID); err != nil {
			return adewire.AddExistingBranchResult{}, err
		}
	}
	target, err := b.resolveExisting(ctx, args.CodeRepoID, args.Name, task.Kind, nf.ForceReview)
	if err != nil {
		return adewire.AddExistingBranchResult{}, err
	}

	now := b.deps.Now().UnixMilli()
	var sb model.AdeTaskBranch
	if args.TaskID == "" {
		task = model.AdeTask{ID: b.newID(), Kind: model.AdeTaskKindTask, CreatedAt: now, Title: nf.Title, HostURL: nf.HostURL}
		if target.kind == model.AdeBranchKindReview {
			task.Kind, task.Owner = model.AdeTaskKindReview, target.row.AuthorName
		}
		sb = model.AdeTaskBranch{ID: b.newID(), TaskID: task.ID, CodeRepoID: args.CodeRepoID, Name: args.Name, Kind: target.kind, AddedAt: now}
		if task, err = b.deps.Tasks.CreateTask(task, []model.AdeTaskBranch{sb}, nil); err != nil {
			return adewire.AddExistingBranchResult{}, err
		}
	} else {
		sb = model.AdeTaskBranch{ID: b.newID(), TaskID: task.ID, CodeRepoID: args.CodeRepoID, Name: args.Name, Kind: target.kind, AddedAt: now}
		draft, err := b.draftOf(task.ID, args.CodeRepoID)
		if err != nil {
			return adewire.AddExistingBranchResult{}, err
		}
		if draft != nil {
			// A repo added without a branch gets this one in the same row.
			if _, _, err := b.editableRow(ctx, draft.TaskID, draft.ID); err != nil {
				return adewire.AddExistingBranchResult{}, err
			}
			err = b.deps.Tasks.DetachBranch(draft.ID, &sb, now)
		} else {
			sb, err = b.deps.Tasks.AddBranch(sb)
		}
		if err != nil {
			return adewire.AddExistingBranchResult{}, err
		}
	}
	*attached = sb
	b.notifyBoard()
	facts := b.repoFacts(ctx, args.CodeRepoID, []model.AdeTaskBranch{sb}, nil)
	b.applyChecks(args.CodeRepoID, facts)
	wt, err := b.wireTask(task)
	if err != nil {
		return adewire.AddExistingBranchResult{}, err
	}
	return adewire.AddExistingBranchResult{Task: wt, Branch: facts.branches[sb.ID]}, nil
}

// SetPlan reorders tasks and sets their days; every id must be a live task.
func (b *TaskBoard) SetPlan(_ context.Context, args adewire.SetPlanArgs) error {
	tasks, err := b.deps.Tasks.ListLive()
	if err != nil {
		return err
	}
	live := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		live[t.ID] = true
	}
	seen := make(map[string]bool, len(args.Order))
	for _, id := range args.Order {
		if !live[id] || seen[id] {
			return invalid("order names %q, which is not a live task listed once", id)
		}
		seen[id] = true
	}
	for id := range args.Days {
		if !live[id] {
			return invalid("days names %q, which is not a live task", id)
		}
	}
	if err := b.deps.Tasks.SetPlan(args.Order, args.Days); err != nil {
		return err
	}
	b.notifyBoard()
	return nil
}

// ShelveTask moves a task onto the Later or Backlog shelf, dropping its day. A review item stays on the Plan.
func (b *TaskBoard) ShelveTask(_ context.Context, args adewire.ShelveTaskArgs) error {
	if args.Shelf != adewire.ShelfLater && args.Shelf != adewire.ShelfBacklog {
		return invalid("shelf must be %q or %q", adewire.ShelfLater, adewire.ShelfBacklog)
	}
	if args.BeforeID == args.TaskID {
		return invalid("a task cannot go before itself")
	}
	mu := b.taskMu(args.TaskID)
	mu.Lock()
	defer mu.Unlock()
	t, err := b.deps.Tasks.GetTask(args.TaskID)
	if err != nil {
		return err
	}
	if t.ArchivedAt != nil {
		return repos.ErrTaskNotFound
	}
	if t.Kind == model.AdeTaskKindReview {
		return invalid("a review item stays on the Plan")
	}
	if err := b.deps.Tasks.Shelve(args.TaskID, args.Shelf == adewire.ShelfBacklog, args.BeforeID); err != nil {
		if errors.Is(err, repos.ErrShelfTarget) {
			return invalid("beforeId is not on the %s shelf", args.Shelf)
		}
		return err
	}
	b.notifyBoard()
	return nil
}

// CaptureTask adds a repo-less, workflow-less task at the top of Later, or of Day.
func (b *TaskBoard) CaptureTask(_ context.Context, args adewire.CaptureTaskArgs) (adewire.Task, error) {
	title := strings.TrimSpace(args.Title)
	if title == "" {
		return adewire.Task{}, invalid("title is required")
	}
	task, err := b.deps.Tasks.CaptureTask(model.AdeTask{
		ID: b.newID(), Kind: model.AdeTaskKindTask, Title: title, CreatedAt: b.deps.Now().UnixMilli(),
	}, args.Day)
	if err != nil {
		return adewire.Task{}, err
	}
	b.notifyBoard()
	return b.wireTask(task)
}

// SetQueuedAfter orders a branch after another branch of the same repo; "" clears it.
func (b *TaskBoard) SetQueuedAfter(_ context.Context, args adewire.SetQueuedAfterArgs) error {
	live, err := b.deps.Tasks.BranchesLive()
	if err != nil {
		return err
	}
	byID := make(map[string]model.AdeTaskBranch, len(live))
	for _, br := range live {
		byID[br.ID] = br
	}
	self, ok := byID[args.BranchID]
	if !ok {
		return repos.ErrBranchMissing
	}
	if args.AfterBranchID != "" {
		after, ok := byID[args.AfterBranchID]
		switch {
		case !ok:
			return invalid("afterBranchId %q is not a live branch", args.AfterBranchID)
		case after.ID == self.ID:
			return invalid("a branch cannot be queued after itself")
		case after.CodeRepoID != self.CodeRepoID:
			return invalid("a branch can only be queued after a branch of the same repo")
		}
		if queueCycle(byID, self.ID, after) {
			return invalid("queuing after that branch would form a cycle")
		}
	}
	if err := b.deps.Tasks.SetQueuedAfter(args.BranchID, args.AfterBranchID); err != nil {
		return err
	}
	b.notifyBoard()
	return nil
}

// --- workflows and repos -----------------------------------------------------------------------

// Workflows lists the workflow files with how many live tasks use each.
func (b *TaskBoard) Workflows(_ context.Context) (adewire.WorkflowsResult, error) {
	usedBy, err := b.workflowUsage()
	if err != nil {
		return adewire.WorkflowsResult{}, err
	}
	res := b.deps.Workflows.List(usedBy)
	res.Sandbox = b.sandboxSupport()
	res.StageSettings = stageCatalog()
	return res, nil
}

// Repos lists every code repo with its ade config and the folders it was imported from.
func (b *TaskBoard) Repos(_ context.Context) (adewire.ReposResult, error) {
	configs, err := b.deps.RepoConfig.List()
	if err != nil {
		return adewire.ReposResult{}, err
	}
	folders, err := b.deps.RepoConfig.Folders()
	if err != nil {
		return adewire.ReposResult{}, err
	}
	live, err := b.deps.Tasks.BranchesLive()
	if err != nil {
		return adewire.ReposResult{}, err
	}
	used := make(map[string]bool)
	for _, br := range live {
		used[br.CodeRepoID] = true
	}
	out := adewire.ReposResult{Repos: make([]adewire.Repo, 0, len(configs)), Folders: make([]adewire.Folder, 0, len(folders))}
	for _, c := range configs {
		prepare, timeout, basePath := "", model.DefaultPrepareTimeout, ""
		if b.deps.GitRepoSettings != nil {
			if s, err := b.deps.GitRepoSettings(c.RepoID); err == nil {
				prepare, timeout, basePath = s.WorktreePrepareScript, s.WorktreePrepareTimeout, s.WorktreeBasePath
			}
		}
		stem, problem := "", (*adewire.RepoProblem)(nil)
		if b.deps.RepoFile != nil {
			stem, problem = b.deps.RepoFile(c.RepoID)
		}
		r := adewire.Repo{
			File: stem, Problem: problem,
			CodeRepoID: c.CodeRepoID, Name: c.Name, Nickname: c.Nickname, Path: c.Root, Source: c.Source,
			UsedByTasks: used[c.CodeRepoID], IntegrationBranches: nonNil(c.IntegrationBranches),
			PrepareScript: prepare, PrepareTimeout: timeout, WorktreeBasePath: basePath, Environments: make([]adewire.Environment, len(c.Environments)),
		}
		for i, e := range c.Environments {
			r.Environments[i] = adewire.Environment{Name: e.Name, DeployedShaScript: e.DeployedShaScript}
		}
		out.Repos = append(out.Repos, r)
	}
	for _, f := range folders {
		out.Folders = append(out.Folders, wireFolder(f))
	}
	return out, nil
}

// queueCycle reports whether walking parents from start reaches selfID. A branch's parent is its
// base branch, else its queue link, else the live branch of its repo that its base names.
func queueCycle(byID map[string]model.AdeTaskBranch, selfID string, start model.AdeTaskBranch) bool {
	byName := make(map[[2]string]string, len(byID))
	for _, br := range byID {
		if br.Name != "" {
			byName[[2]string{br.CodeRepoID, br.Name}] = br.ID
		}
	}
	cur := start
	for range len(byID) + 1 {
		parent := cur.BaseBranchID
		if parent == "" {
			parent = cur.QueuedAfter
		}
		if parent == "" && cur.Base != "" {
			parent = byName[[2]string{cur.CodeRepoID, cur.Base}]
		}
		if parent == "" || parent == cur.ID {
			return false
		}
		if parent == selfID {
			return true
		}
		next, ok := byID[parent]
		if !ok {
			return false
		}
		cur = next
	}
	return true
}

// hostRegistry is the code host registry, nil when none is wired.
func (b *TaskBoard) hostRegistry() *githost.Registry {
	if b.deps.Registry == nil {
		return nil
	}
	return b.deps.Registry.Hosts.Registry
}

// hostLink is the issue or change-request link a pasted URL names, nil when none matches.
func (b *TaskBoard) hostLink(raw string) *githost.Link {
	if raw == "" || b.deps.Registry == nil {
		return nil
	}
	l, ok := b.deps.Registry.Hosts.Registry.ParseLink(raw)
	if !ok {
		return nil
	}
	return &l
}
