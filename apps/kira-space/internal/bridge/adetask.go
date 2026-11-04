package bridge

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/ade"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/codeworkspace"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/appsettings"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/terminal"
)

const (
	adeTaskMaxColor    = 20
	adeTaskMaxRepoIDs  = 64
	adeTaskMaxPlanKeys = 10_000
	adeMaxYamlBytes    = 1 << 20
	adeMaxPathBytes    = 4096
)

// AdeTaskService is the v2 ADE task surface (P144): board snapshot, task/plan/backlog writes and
// workflow/repo reads. Run, session and workflow-edit methods land with later waves.
type AdeTaskService struct {
	Engine *ade.TaskBoard
	// Registry, Emit and FocusWindow serve FocusSession; nil in a fixture that never calls it.
	// FocusWindow is a func field, not a method, so it adds nothing to the bound surface.
	Registry    *terminal.Registry
	Emit        appcore.Emitter
	FocusWindow func(key string) bool
	// Hosts validates pasted issue and change-request links.
	Hosts *githost.Registry
	// OpenWindow, CloseWindow and SetWindowTitle serve review windows; func fields like FocusWindow.
	OpenWindow     func(rec shell.WindowRecord)
	CloseWindow    func(key string) bool
	SetWindowTitle func(key, title string)
}

// AdeTaskOpenSession is FocusSession's emit half: addressed to the one window just brought forward.
func AdeTaskOpenSession(e appcore.Emitter, windowKey string, payload adewire.OpenSessionEvent) {
	e.EmitTo(windowKey, adewire.ChannelOpenSession, payload)
}

// AdeTaskBoardChanged is TaskBoard.OnBoard's target: payload-free, every window re-fetches Board.
func AdeTaskBoardChanged(ev *Events) { ev.Broadcast(adewire.ChannelBoard) }

// AdeTaskRunsChanged is TaskBoard.OnRuns' target: the changed runs ride the event.
func AdeTaskRunsChanged(ev *Events, payload adewire.RunsChangedEvent) {
	ev.emit.Emit(adewire.ChannelRuns, payload)
}

// AdeTaskLogAppended is TaskBoard.OnLog's target: one stored batch of log chunks.
func AdeTaskLogAppended(ev *Events, payload adewire.LogEvent) {
	ev.emit.Emit(adewire.ChannelLog, payload)
}

// AdeTaskReviewChanged is TaskBoard.OnReview's target: the review of one branch changed.
func AdeTaskReviewChanged(ev *Events, payload adewire.ReviewEvent) {
	ev.emit.Emit(adewire.ChannelReview, payload)
}

// adeReviewNoteMax bounds the note of FinishReview, in runes.
const adeReviewNoteMax = 2000

// AdeTaskSessionsChanged is TaskBoard.OnSessions' target: payload-free.
func AdeTaskSessionsChanged(ev *Events) { ev.Broadcast(adewire.ChannelSessions) }

// AdeTaskWorkflowsChanged is TaskBoard.OnWorkflows' target: payload-free.
func AdeTaskWorkflowsChanged(ev *Events) { ev.Broadcast(adewire.ChannelWorkflows) }

// AdeTaskReposChanged is TaskBoard.OnRepos' target: payload-free.
func AdeTaskReposChanged(ev *Events) { ev.Broadcast(adewire.ChannelRepos) }

// adeTaskCatalog maps store and engine sentinels a person can meet to their wire code and catalog
// text. Sentinels that mean a caller bug are in adeTaskContract.
var adeTaskCatalog = []struct {
	err  error
	code ipcerr.Code
	key  string
}{
	{ade.ErrStale, ipcerr.CodeConflict, "backend.space.ade.taskMoved.body"},
	{gitsession.ErrRangedMarkOnNonText, ipcerr.CodeBadRequest, "backend.space.gitrpc.rangedMarkNeedsText.body"},
	{gitsession.ErrCommentNotText, ipcerr.CodeBadRequest, "backend.space.gitrpc.commentNeedsText.body"},
	{gitsession.ErrCommentRangeOutOfFile, ipcerr.CodeBadRequest, "backend.space.gitrpc.commentRangePastEnd.body"},
	{ade.ErrWorkingChanged, ipcerr.CodeConflict, "backend.space.ade.workingChanged.body"},
	{ade.ErrWorkflowChanged, ipcerr.CodeConflict, "backend.space.ade.workflowChanged.body"},
	{ade.ErrSetupPending, ipcerr.CodePreparing, "backend.space.ade.setupPending.body"},
	{repos.ErrBranchOnTask, ipcerr.CodeConflict, "backend.space.ade.branchOnTask.body"},
	{repos.ErrRepoOnTask, ipcerr.CodeConflict, "backend.space.ade.repoOnTask.body"},
	{ade.ErrSessionRunning, ipcerr.CodeConflict, "backend.space.ade.sessionRunning.body"},
	{ade.ErrSessionNotRunning, ipcerr.CodeConflict, "backend.space.ade.sessionNotRunning.body"},
	{ade.ErrResumeCwdNotDir, ipcerr.CodeConflict, "backend.space.ade.resumeCwdNotDir.body"},
	{repos.ErrTaskNotFound, ipcerr.CodeNotFound, "backend.space.ade.taskNotFound.body"},
	{repos.ErrBranchMissing, ipcerr.CodeNotFound, "backend.space.ade.branchMissing.body"},
	{repos.ErrRunNotFound, ipcerr.CodeNotFound, "backend.space.ade.runNotFound.body"},
	{ade.ErrSessionNotFound, ipcerr.CodeNotFound, "backend.space.ade.sessionNotFound.body"},
	{ade.ErrBranchRequestGone, ipcerr.CodeNotFound, "backend.space.ade.branchRequestGone.body"},
	{harness.ErrProviderGone, ipcerr.CodeConflict, "backend.core.agent.providerGone.body"},
}

var adeTaskContract = []error{repos.ErrReviewKind, ade.ErrSessionWrongTask, ade.ErrCommandMismatch}

// adeTaskError maps store and engine sentinels to wire codes.
func adeTaskError(err error) error {
	if err == nil {
		return nil
	}
	for _, c := range adeTaskCatalog {
		if errors.Is(err, c.err) {
			return ipcerr.T(c.code, c.key, nil)
		}
	}
	if errors.Is(err, repos.ErrTaskUnnamed) || errors.Is(err, repos.ErrStatusSet) || errors.Is(err, repos.ErrStatusUnknown) {
		return ipcerr.BadRequest(err.Error())
	}
	if errors.Is(err, ade.ErrInvalidInput) {
		// The detail is a refusal the person can act on (a cycle, a missing base), not a bug.
		return ipcerr.BadRequest(strings.TrimPrefix(err.Error(), ade.ErrInvalidInput.Error()+": "))
	}
	for _, c := range adeTaskContract {
		if errors.Is(err, c) {
			return ipcerr.Contract("%s", err.Error())
		}
	}
	return ipcerr.Wrap(err)
}

var adeWorkflowIDRe = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

// validateAdeFromStep checks the optional stale-guard step id.
func validateAdeFromStep(id string) error {
	if id == "" {
		return nil
	}
	return validateAdeItemID(id, "fromStepId")
}

func adeTaskInvalid(msg string) error { return ipcerr.Contract("%s", msg) }

func validateAdeTaskID(value, field string) error { return validateAdeItemID(value, field) }

func validateAdeTaskJira(j *adewire.Jira) error {
	if j == nil {
		return nil
	}
	return validateAdeJira(j.Key, j.URL)
}

func validateAdeTaskRepoIDs(ids []string) error {
	if len(ids) > adeTaskMaxRepoIDs {
		return adeTaskInvalid("codeRepoIds is too long")
	}
	for _, id := range ids {
		if err := validateAdeItemID(id, "codeRepoIds"); err != nil {
			return err
		}
	}
	return nil
}

func validateCreateTask(a adewire.CreateTaskArgs) error {
	if err := validateAdeName(a.Title, "title"); err != nil {
		return err
	}
	if err := validateAdeTaskJira(a.Jira); err != nil {
		return err
	}
	if err := validateAdeURL(a.HostURL, "hostUrl"); err != nil {
		return err
	}
	if err := validateAdeNotes(a.Notes); err != nil {
		return err
	}
	if len(a.WorkflowID) > adeMaxBranchBytes {
		return adeTaskInvalid("workflowId is too long")
	}
	if len(a.Bases) > adeTaskMaxRepoIDs {
		return adeTaskInvalid("bases is too long")
	}
	for id, c := range a.Bases {
		if err := validateAdeItemID(id, "bases"); err != nil {
			return err
		}
		if err := validateAdeBaseChoice(c, "bases"); err != nil {
			return err
		}
	}
	if a.Day != nil {
		if err := validateAdeISODate(*a.Day); err != nil {
			return err
		}
	}
	return validateAdeTaskRepoIDs(a.CodeRepoIDs)
}

// validateAdeBaseChoice checks a base choice: a branch name and a planner branch id, both optional.
func validateAdeBaseChoice(c adewire.BaseChoice, field string) error {
	if c.Ref != "" {
		if err := validateAdeBranchName(c.Ref, field+".ref"); err != nil {
			return err
		}
	}
	if c.BranchID != "" {
		return validateAdeItemID(c.BranchID, field+".branchId")
	}
	return nil
}

func validateAdeOnto(a adewire.OntoArgs) error {
	if err := validateAdeItemID(a.BranchID, "branchId"); err != nil {
		return err
	}
	if a.QueueWith != "" {
		if err := validateAdeItemID(a.QueueWith, "queueWith"); err != nil {
			return err
		}
		if a.Onto != nil {
			return adeTaskInvalid("onto and queueWith are mutually exclusive")
		}
	}
	if a.Onto != nil {
		return validateAdeBaseChoice(*a.Onto, "onto")
	}
	return nil
}

func validateUpdateTask(a adewire.UpdateTaskArgs) error {
	if err := validateAdeTaskID(a.TaskID, "taskId"); err != nil {
		return err
	}
	p := a.Patch
	if p.Title != nil {
		if err := validateAdeName(*p.Title, "title"); err != nil {
			return err
		}
	}
	if err := validateAdeTaskJira(p.Jira); err != nil {
		return err
	}
	if p.HostURL != nil {
		if err := validateAdeURL(*p.HostURL, "hostUrl"); err != nil {
			return err
		}
	}
	if p.Est != nil {
		if err := validateAdeEst(*p.Est); err != nil {
			return err
		}
	}
	if p.Notes != nil {
		if err := validateAdeNotes(*p.Notes); err != nil {
			return err
		}
	}
	if p.Color != nil && (*p.Color < 0 || *p.Color >= adeTaskMaxColor) {
		return adeTaskInvalid("color is out of range")
	}
	if p.Kind != nil && *p.Kind != "task" && *p.Kind != "parked" {
		return adeTaskInvalid("kind must be task or parked")
	}
	if p.StatusID != nil && !model.ValidAdeStatusID(*p.StatusID) {
		return adeTaskInvalid("statusId is not a status id")
	}
	return nil
}

func validateSaveStatuses(a adewire.SaveStatusesArgs) error {
	if len(a.Statuses) < model.AdeStatusMin || len(a.Statuses) > model.AdeStatusMax {
		return adeTaskInvalid("statuses must hold 1 to 20 entries")
	}
	for _, st := range a.Statuses {
		if err := (model.AdeTaskStatus{ID: st.ID, Name: strings.TrimSpace(st.Name), Color: st.Color}).Validate(); err != nil {
			return adeTaskInvalid(err.Error())
		}
	}
	for _, id := range []string{a.Defaults.New, a.Defaults.Step, a.Defaults.Review, a.Defaults.Finished} {
		if id != "" && !model.ValidAdeStatusID(id) {
			return adeTaskInvalid("defaults must name status ids")
		}
	}
	if a.Defaults.New == "" {
		return adeTaskInvalid("new tasks need a status")
	}
	if len(a.Reassign) > model.AdeStatusMax {
		return adeTaskInvalid("reassign is too long")
	}
	for from, to := range a.Reassign {
		if !model.ValidAdeStatusID(from) || !model.ValidAdeStatusID(to) {
			return adeTaskInvalid("reassign must map status ids to status ids")
		}
	}
	return nil
}

func validateSetPlan(a adewire.SetPlanArgs) error {
	if len(a.Order) > adeTaskMaxPlanKeys || len(a.Days) > adeTaskMaxPlanKeys {
		return adeTaskInvalid("plan is too large")
	}
	for _, id := range a.Order {
		if err := validateAdeTaskID(id, "order"); err != nil {
			return err
		}
	}
	for id, day := range a.Days {
		if err := validateAdeTaskID(id, "days"); err != nil {
			return err
		}
		if day != nil {
			if err := validateAdeISODate(*day); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- reads -------------------------------------------------------------------------------------

func (s *AdeTaskService) Board(ctx context.Context) (adewire.Board, error) {
	ctx = adeUserCtx(ctx)
	b, err := s.Engine.Board(ctx)
	return b, adeTaskError(err)
}

func (s *AdeTaskService) ChangeRequests(ctx context.Context) (adewire.ChangeRequestsResult, error) {
	ctx = adeUserCtx(ctx)
	r, err := s.Engine.ChangeRequests(ctx)
	return r, adeTaskError(err)
}

func (s *AdeTaskService) Refresh(ctx context.Context, args adewire.RefreshArgs) (adewire.RefreshResult, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskRepoIDs(args.CodeRepoIDs); err != nil {
		return adewire.RefreshResult{}, err
	}
	r, err := s.Engine.Refresh(ctx, args.CodeRepoIDs)
	return r, adeTaskError(err)
}

func (s *AdeTaskService) ForcePush(ctx context.Context, args adewire.BranchArgs) (adewire.ForcePushResult, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.BranchID, "branchId"); err != nil {
		return adewire.ForcePushResult{}, err
	}
	r, err := s.Engine.ForcePush(ctx, args.BranchID)
	return r, adeTaskError(err)
}

func (s *AdeTaskService) CandidateBranches(ctx context.Context, args adewire.CandidateBranchesArgs) (adewire.CandidateBranchesResult, error) {
	ctx = adeUserCtx(ctx)
	if args.CodeRepoID != "" {
		if err := validateAdeItemID(args.CodeRepoID, "codeRepoId"); err != nil {
			return adewire.CandidateBranchesResult{}, err
		}
	}
	r, err := s.Engine.CandidateBranches(ctx, args.CodeRepoID)
	return r, adeTaskError(err)
}

func (s *AdeTaskService) Workflows(ctx context.Context) (adewire.WorkflowsResult, error) {
	ctx = adeUserCtx(ctx)
	r, err := s.Engine.Workflows(ctx)
	return r, adeTaskError(err)
}

func (s *AdeTaskService) Repos(ctx context.Context) (adewire.ReposResult, error) {
	ctx = adeUserCtx(ctx)
	r, err := s.Engine.Repos(ctx)
	return r, adeTaskError(err)
}

// HostProviders lists every registered code host provider; the phone reads it for its link labels.
func (s *AdeTaskService) HostProviders(context.Context) (githost.Providers, error) {
	return githost.Providers{Providers: s.Hosts.Infos()}, nil
}

// --- writes ------------------------------------------------------------------------------------

// validateHostLink refuses a non-empty value no registered provider recognises as an issue or
// change-request link.
func (s *AdeTaskService) validateHostLink(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if _, ok := s.Hosts.ParseLink(raw); ok {
		return nil
	}
	return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.gitHost.notLink.error", map[string]string{"links": s.Hosts.LinkLabels()})
}

func (s *AdeTaskService) CreateTask(ctx context.Context, args adewire.CreateTaskArgs) (adewire.Task, error) {
	ctx = adeUserCtx(ctx)
	if err := validateCreateTask(args); err != nil {
		return adewire.Task{}, err
	}
	if err := s.validateHostLink(args.HostURL); err != nil {
		return adewire.Task{}, err
	}
	t, err := s.Engine.CreateTask(ctx, args)
	return t, adeTaskError(err)
}

func (s *AdeTaskService) UpdateTask(ctx context.Context, args adewire.UpdateTaskArgs) (adewire.Task, error) {
	ctx = adeUserCtx(ctx)
	if err := validateUpdateTask(args); err != nil {
		return adewire.Task{}, err
	}
	if args.Patch.HostURL != nil {
		if err := s.validateHostLink(*args.Patch.HostURL); err != nil {
			return adewire.Task{}, err
		}
	}
	t, err := s.Engine.UpdateTask(ctx, args)
	return t, adeTaskError(err)
}

// SaveStatuses rewrites the task status set; tasks on a removed status move to its reassign target.
func (s *AdeTaskService) SaveStatuses(ctx context.Context, args adewire.SaveStatusesArgs) (adewire.SaveStatusesResult, error) {
	ctx = adeUserCtx(ctx)
	if err := validateSaveStatuses(args); err != nil {
		return adewire.SaveStatusesResult{}, err
	}
	res, err := s.Engine.SaveStatuses(ctx, args)
	return res, adeTaskError(err)
}

func (s *AdeTaskService) AddTaskRepo(ctx context.Context, args adewire.AddTaskRepoArgs) (adewire.Branch, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
		return adewire.Branch{}, err
	}
	if err := validateAdeTaskID(args.CodeRepoID, "codeRepoId"); err != nil {
		return adewire.Branch{}, err
	}
	if args.Base != nil {
		if err := validateAdeBaseChoice(*args.Base, "base"); err != nil {
			return adewire.Branch{}, err
		}
	}
	br, err := s.Engine.AddTaskRepo(ctx, args)
	return br, adeTaskError(err)
}

// RebasePreview returns the prompt Rebase would send, its stack and what blocks it.
func (s *AdeTaskService) RebasePreview(ctx context.Context, args adewire.OntoArgs) (adewire.RebasePreview, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeOnto(args); err != nil {
		return adewire.RebasePreview{}, err
	}
	p, err := s.Engine.RebasePreview(ctx, args)
	return p, adeTaskError(err)
}

// Rebase starts the background rebase of a branch (and the branches stacked on it).
func (s *AdeTaskService) Rebase(ctx context.Context, args adewire.RebaseArgs) (adewire.RebaseStart, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeOnto(adewire.OntoArgs{BranchID: args.BranchID, Onto: args.Onto, QueueWith: args.QueueWith}); err != nil {
		return adewire.RebaseStart{}, err
	}
	if len(args.Message) > adeMaxMessageBytes {
		return adewire.RebaseStart{}, adeMessageTooLong()
	}
	r, err := s.Engine.Rebase(ctx, args)
	return r, adeTaskError(err)
}

// AbortRebase aborts the rebase left in progress in a branch worktree.
func (s *AdeTaskService) AbortRebase(ctx context.Context, args adewire.BranchArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return err
	}
	return adeTaskError(s.Engine.AbortRebase(ctx, args))
}

// SetBranchBase sets the base of a branch that is not created yet.
func (s *AdeTaskService) SetBranchBase(ctx context.Context, args adewire.SetBranchBaseArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return err
	}
	if err := validateAdeBaseChoice(args.Base, "base"); err != nil {
		return err
	}
	return adeTaskError(s.Engine.SetBranchBase(ctx, args))
}

// SetTaskBranch gives a task row an existing git branch or creates a new one for it.
func (s *AdeTaskService) SetTaskBranch(ctx context.Context, args adewire.SetTaskBranchArgs) (adewire.Branch, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return adewire.Branch{}, err
	}
	if err := validateAdeBranchName(args.Name, "name"); err != nil {
		return adewire.Branch{}, err
	}
	br, err := s.Engine.SetTaskBranch(ctx, args)
	return br, adeTaskError(err)
}

// UpdateTaskRepo swaps the repo of a row whose branch is not created.
func (s *AdeTaskService) UpdateTaskRepo(ctx context.Context, args adewire.UpdateTaskRepoArgs) (adewire.Branch, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return adewire.Branch{}, err
	}
	if err := validateAdeItemID(args.CodeRepoID, "codeRepoId"); err != nil {
		return adewire.Branch{}, err
	}
	br, err := s.Engine.UpdateTaskRepo(ctx, args)
	return br, adeTaskError(err)
}

// RemoveTaskBranch detaches a row's branch; the repo stays on the task.
func (s *AdeTaskService) RemoveTaskBranch(ctx context.Context, args adewire.RemoveTaskRowArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return err
	}
	return adeTaskError(s.Engine.RemoveTaskBranch(ctx, args))
}

// RemoveTaskRepo detaches a row, its branch included.
func (s *AdeTaskService) RemoveTaskRepo(ctx context.Context, args adewire.RemoveTaskRowArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return err
	}
	return adeTaskError(s.Engine.RemoveTaskRepo(ctx, args))
}

// TaskBranchRisk reports what removing a row's branch would discard.
func (s *AdeTaskService) TaskBranchRisk(ctx context.Context, args adewire.BranchArgs) (adewire.BranchRisk, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return adewire.BranchRisk{}, err
	}
	r, err := s.Engine.TaskBranchRisk(ctx, args.BranchID)
	return r, adeTaskError(err)
}

// RepoBranches lists the bases the picker offers for a repo.
func (s *AdeTaskService) RepoBranches(ctx context.Context, args adewire.RepoBranchesArgs) (adewire.RepoBranches, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.CodeRepoID, "codeRepoId"); err != nil {
		return adewire.RepoBranches{}, err
	}
	if args.BranchID != "" {
		if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
			return adewire.RepoBranches{}, err
		}
	}
	r, err := s.Engine.RepoBranches(ctx, args)
	return r, adeTaskError(err)
}

func (s *AdeTaskService) AddExistingBranch(ctx context.Context, args adewire.AddExistingBranchArgs) (adewire.AddExistingBranchResult, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.CodeRepoID, "codeRepoId"); err != nil {
		return adewire.AddExistingBranchResult{}, err
	}
	if err := validateAdeBranchName(args.Name, "name"); err != nil {
		return adewire.AddExistingBranchResult{}, err
	}
	if args.TaskID != "" { // '' = new task (wire contract)
		if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
			return adewire.AddExistingBranchResult{}, err
		}
	}
	r, err := s.Engine.AddExistingBranch(ctx, args)
	return r, adeTaskError(err)
}

const adeMaxHostLinkBytes = 2048

func validateHostLinkArgs(args adewire.HostLinkArgs) (adewire.HostLinkArgs, error) {
	args.URL = strings.TrimSpace(args.URL)
	if args.URL == "" || len(args.URL) > adeMaxHostLinkBytes || strings.ContainsFunc(args.URL, unicode.IsControl) {
		return args, adeTaskInvalid("url must be a link of at most 2048 bytes")
	}
	if args.TaskID != "" {
		if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
			return args, err
		}
	}
	return args, nil
}

// ResolveHostLink names the branch a pasted PR or branch link points at; it changes nothing.
func (s *AdeTaskService) ResolveHostLink(ctx context.Context, args adewire.HostLinkArgs) (adewire.HostLinkTarget, error) {
	ctx = adeUserCtx(ctx)
	args, err := validateHostLinkArgs(args)
	if err != nil {
		return adewire.HostLinkTarget{}, err
	}
	r, err := s.Engine.ResolveHostLink(ctx, args)
	return r, adeTaskError(err)
}

// AddHostLink attaches the branch a pasted PR or branch link points at.
func (s *AdeTaskService) AddHostLink(ctx context.Context, args adewire.HostLinkArgs) (adewire.AddExistingBranchResult, error) {
	ctx = adeUserCtx(ctx)
	args, err := validateHostLinkArgs(args)
	if err != nil {
		return adewire.AddExistingBranchResult{}, err
	}
	r, err := s.Engine.AddHostLink(ctx, args)
	return r, adeTaskError(err)
}

func (s *AdeTaskService) SetPlan(ctx context.Context, args adewire.SetPlanArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateSetPlan(args); err != nil {
		return err
	}
	return adeTaskError(s.Engine.SetPlan(ctx, args))
}

// ShelveTask moves a task onto the Later or Backlog shelf.
func (s *AdeTaskService) ShelveTask(ctx context.Context, args adewire.ShelveTaskArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
		return err
	}
	if args.BeforeID != "" {
		if err := validateAdeTaskID(args.BeforeID, "beforeId"); err != nil {
			return err
		}
	}
	if args.Shelf != adewire.ShelfLater && args.Shelf != adewire.ShelfBacklog {
		return adeTaskInvalid("shelf must be later or backlog")
	}
	return adeTaskError(s.Engine.ShelveTask(ctx, args))
}

// CaptureTask adds a title-only task at the top of Later, or of args.Day.
func (s *AdeTaskService) CaptureTask(ctx context.Context, args adewire.CaptureTaskArgs) (adewire.Task, error) {
	ctx = adeUserCtx(ctx)
	if strings.TrimSpace(args.Title) == "" {
		return adewire.Task{}, adeTaskInvalid("title is required")
	}
	if err := validateAdeName(args.Title, "title"); err != nil {
		return adewire.Task{}, err
	}
	if args.Day != nil {
		if err := validateAdeISODate(*args.Day); err != nil {
			return adewire.Task{}, err
		}
	}
	t, err := s.Engine.CaptureTask(ctx, args)
	return t, adeTaskError(err)
}

func (s *AdeTaskService) SetQueuedAfter(ctx context.Context, args adewire.SetQueuedAfterArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.BranchID, "branchId"); err != nil {
		return err
	}
	if args.AfterBranchID != "" {
		if err := validateAdeTaskID(args.AfterBranchID, "afterBranchId"); err != nil {
			return err
		}
	}
	return adeTaskError(s.Engine.SetQueuedAfter(ctx, args))
}

// --- workflow files and repo config (P145) -------------------------------------------------------

func validateAdeYaml(src string) error {
	if len(src) > adeMaxYamlBytes {
		return adeYamlTooLarge()
	}
	return nil
}

func validateAdePath(path, field string) error {
	if path == "" {
		return adeTaskInvalid(field + " is required")
	}
	if len(path) > adeMaxPathBytes || strings.ContainsRune(path, 0) {
		return adeTaskInvalid(field + " is invalid")
	}
	return nil
}

func (s *AdeTaskService) WorkflowYaml(ctx context.Context, args adewire.FileNameArgs) (adewire.WorkflowYaml, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeBranchName(args.FileName, "fileName"); err != nil {
		return adewire.WorkflowYaml{}, err
	}
	r, err := s.Engine.WorkflowYaml(ctx, args.FileName)
	return r, adeTaskError(err)
}

func (s *AdeTaskService) ValidateWorkflowYaml(ctx context.Context, args adewire.ValidateWorkflowYamlArgs) (adewire.WorkflowValidation, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeYaml(args.Yaml); err != nil {
		return adewire.WorkflowValidation{}, err
	}
	return s.Engine.ValidateWorkflowYaml(ctx, args.Yaml), nil
}

func (s *AdeTaskService) SaveWorkflow(ctx context.Context, args adewire.SaveWorkflowArgs) (adewire.WorkflowEntry, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeBranchName(args.FileName, "fileName"); err != nil {
		return adewire.WorkflowEntry{}, err
	}
	r, err := s.Engine.SaveWorkflow(ctx, args)
	return r, adeTaskError(err)
}

func (s *AdeTaskService) SaveWorkflowYaml(ctx context.Context, args adewire.SaveWorkflowYamlArgs) (adewire.WorkflowEntry, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeBranchName(args.FileName, "fileName"); err != nil {
		return adewire.WorkflowEntry{}, err
	}
	if err := validateAdeYaml(args.Yaml); err != nil {
		return adewire.WorkflowEntry{}, err
	}
	r, err := s.Engine.SaveWorkflowYaml(ctx, args)
	return r, adeTaskError(err)
}

func (s *AdeTaskService) ImportWorkflow(ctx context.Context, args adewire.ImportWorkflowArgs) (adewire.WorkflowEntry, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdePath(args.Path, "path"); err != nil {
		return adewire.WorkflowEntry{}, err
	}
	r, err := s.Engine.ImportWorkflow(ctx, args)
	return r, adeTaskError(err)
}

func (s *AdeTaskService) NewWorkflow(ctx context.Context, args adewire.NewWorkflowArgs) (adewire.WorkflowEntry, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeName(args.Name, "name"); err != nil {
		return adewire.WorkflowEntry{}, err
	}
	r, err := s.Engine.NewWorkflow(ctx, args)
	return r, adeTaskError(err)
}

func (s *AdeTaskService) AddFolder(ctx context.Context, args adewire.FolderArgs) (adewire.FolderImportResult, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdePath(args.Path, "path"); err != nil {
		return adewire.FolderImportResult{}, err
	}
	r, err := s.Engine.AddFolder(ctx, args.Path, args.Watch)
	return r, adeTaskError(err)
}

func (s *AdeTaskService) SetFolderWatch(ctx context.Context, args adewire.FolderArgs) (adewire.Folder, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdePath(args.Path, "path"); err != nil {
		return adewire.Folder{}, err
	}
	r, err := s.Engine.SetFolderWatch(ctx, args.Path, args.Watch)
	return r, adeTaskError(err)
}

func (s *AdeTaskService) SetFolderHidden(ctx context.Context, args adewire.FolderHiddenArgs) (adewire.Folder, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdePath(args.Path, "path"); err != nil {
		return adewire.Folder{}, err
	}
	r, err := s.Engine.SetFolderHidden(ctx, args.Path, args.Hidden)
	return r, adeTaskError(err)
}

func (s *AdeTaskService) RemoveFolder(ctx context.Context, args adewire.PathArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdePath(args.Path, "path"); err != nil {
		return err
	}
	return adeTaskError(s.Engine.RemoveFolder(ctx, args.Path))
}

func (s *AdeTaskService) RecordMerge(ctx context.Context, args adewire.RecordMergeArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return err
	}
	if err := validateAdeBranchName(args.Target, "target"); err != nil {
		return err
	}
	return adeTaskError(s.Engine.RecordMerge(ctx, args.BranchID, args.Target))
}

// --- runs --------------------------------------------------------------------------------------

func (s *AdeTaskService) StartRun(ctx context.Context, args adewire.StartRunArgs) (adewire.StartRunResult, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
		return adewire.StartRunResult{}, err
	}
	if len(args.BranchNames) > adeTaskMaxRepoIDs {
		return adewire.StartRunResult{}, adeTaskInvalid("branchNames is too long")
	}
	for id, name := range args.BranchNames {
		if err := validateAdeItemID(id, "branchNames"); err != nil {
			return adewire.StartRunResult{}, err
		}
		if name == "" {
			continue
		}
		if err := validateAdeBranchName(name, "branchNames"); err != nil {
			return adewire.StartRunResult{}, err
		}
	}
	if len(args.Message) > adeMaxMessageBytes {
		return adewire.StartRunResult{}, adeMessageTooLong()
	}
	if err := validateAdeFromStep(args.FromStepID); err != nil {
		return adewire.StartRunResult{}, err
	}
	r, err := s.Engine.StartRun(ctx, args)
	return r, adeTaskError(err)
}

func (s *AdeTaskService) SetTaskWorkflow(ctx context.Context, args adewire.SetTaskWorkflowArgs) (adewire.Task, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
		return adewire.Task{}, err
	}
	if args.WorkflowID != "" && !adeWorkflowIDRe.MatchString(args.WorkflowID) {
		return adewire.Task{}, adeTaskInvalid("workflowId must be 1-64 chars of a-z, 0-9, _ or -")
	}
	t, err := s.Engine.SetTaskWorkflow(ctx, args)
	return t, adeTaskError(err)
}

func (s *AdeTaskService) Approve(ctx context.Context, args adewire.StepArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
		return err
	}
	if err := validateAdeItemID(args.StepID, "stepId"); err != nil {
		return err
	}
	return adeTaskError(s.Engine.Approve(ctx, args))
}

func (s *AdeTaskService) RetryRun(ctx context.Context, args adewire.RunArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.RunID, "runId"); err != nil {
		return err
	}
	return adeTaskError(s.Engine.RetryRun(ctx, args.RunID))
}

func (s *AdeTaskService) ResumeRun(ctx context.Context, args adewire.RunArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.RunID, "runId"); err != nil {
		return err
	}
	return adeTaskError(s.Engine.ResumeRun(ctx, args.RunID))
}

// ReviewStep describes the branch's review step for its review window.
func (s *AdeTaskService) ReviewStep(ctx context.Context, args adewire.BranchArgs) (adewire.ReviewStepState, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return adewire.ReviewStepState{}, err
	}
	st, err := s.Engine.ReviewStep(ctx, args.BranchID)
	return st, adeTaskError(err)
}

// SendReview sends the open review comments of a waiting review run to an agent.
func (s *AdeTaskService) SendReview(ctx context.Context, args adewire.SendReviewArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.RunID, "runId"); err != nil {
		return err
	}
	if args.TargetRunID != "" {
		if err := validateAdeItemID(args.TargetRunID, "targetRunId"); err != nil {
			return err
		}
	}
	if args.Route != "" && args.Route != adewire.ReviewSendImplementer && args.Route != adewire.ReviewSendNew {
		return adeTaskInvalid("route must be implementer or new")
	}
	return adeTaskError(s.Engine.SendReview(ctx, args))
}

// FinishReview ends a waiting review run on the result the user picked.
func (s *AdeTaskService) FinishReview(ctx context.Context, args adewire.FinishReviewArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.RunID, "runId"); err != nil {
		return err
	}
	if err := validateAdeItemID(args.Result, "result"); err != nil {
		return err
	}
	if utf8.RuneCountInString(args.Note) > adeReviewNoteMax {
		return adeTaskInvalid("note is too long")
	}
	return adeTaskError(s.Engine.FinishReview(ctx, args))
}

func (s *AdeTaskService) StepDone(ctx context.Context, args adewire.StepDoneArgs) (adewire.Task, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
		return adewire.Task{}, err
	}
	if err := validateAdeHandoverNotes(args.Notes); err != nil {
		return adewire.Task{}, err
	}
	t, err := s.Engine.StepDone(ctx, args.TaskID, args.Notes, args.CloseSessions)
	return t, adeTaskError(err)
}

// validateAdeHandoverNotes refuses handover text over the limit; nil is "keep the draft".
func validateAdeHandoverNotes(notes *string) error {
	if notes != nil && len(*notes) > agentmcp.MaxHandoverBytes {
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.ade.handoverTooLong.body", nil)
	}
	return nil
}

// Handovers lists a task's handover rows, newest first. Archived tasks work too.
func (s *AdeTaskService) Handovers(ctx context.Context, args adewire.TaskArgs) (adewire.HandoversResult, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
		return adewire.HandoversResult{}, err
	}
	rows, err := s.Engine.Handovers(ctx, args.TaskID)
	return adewire.HandoversResult{Handovers: rows}, adeTaskError(err)
}

// UpdateHandover replaces a handover row's body; an empty body deletes it.
func (s *AdeTaskService) UpdateHandover(ctx context.Context, args adewire.UpdateHandoverArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.ID, "id"); err != nil {
		return err
	}
	if err := validateAdeHandoverNotes(&args.Body); err != nil {
		return err
	}
	return adeTaskError(s.Engine.UpdateHandover(ctx, args.ID, args.Body))
}

// DismissHandover hides the task's "move to next stage" prompt; the draft stays.
func (s *AdeTaskService) DismissHandover(ctx context.Context, args adewire.TaskArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
		return err
	}
	return adeTaskError(s.Engine.DismissHandover(ctx, args.TaskID))
}

// Artifacts lists a task's artifacts: detected ones (task links, branches, change requests,
// deployments) and stored ones. Archived tasks work too; a source that fails shows up as a gap.
func (s *AdeTaskService) Artifacts(ctx context.Context, args adewire.TaskArgs) (adewire.ArtifactsResult, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
		return adewire.ArtifactsResult{}, err
	}
	res, err := s.Engine.TaskArtifacts(ctx, args.TaskID)
	return res, adeTaskError(err)
}

// validateLinkText checks one line of link text: non-empty, bounded, no control characters.
func validateLinkText(text string) error {
	text = strings.TrimSpace(text)
	if text == "" || len(text) > agentmcp.MaxArtifactURL+agentmcp.MaxArtifactTitle+16 || strings.ContainsFunc(text, unicode.IsControl) {
		return ipcerr.Contract("link text must be one line of at most %d bytes", agentmcp.MaxArtifactURL+agentmcp.MaxArtifactTitle+16)
	}
	return nil
}

// AddLink files one line of text under the right category of the task's links.
func (s *AdeTaskService) AddLink(ctx context.Context, args adewire.AddLinkArgs) (adewire.LinkResult, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
		return adewire.LinkResult{}, err
	}
	if err := validateLinkText(args.Text); err != nil {
		return adewire.LinkResult{}, err
	}
	res, err := s.Engine.AddLink(ctx, args)
	return res, adeTaskError(err)
}

// EditLink re-files a link row from new text, or recategorises a stored row.
func (s *AdeTaskService) EditLink(ctx context.Context, args adewire.EditLinkArgs) (adewire.LinkResult, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
		return adewire.LinkResult{}, err
	}
	switch args.Slot {
	case "jira", "host":
		if args.ID != "" || args.Kind != "" {
			return adewire.LinkResult{}, ipcerr.Contract("a %s link has neither an id nor a kind", args.Slot)
		}
	case "stored":
		if err := validateAdeItemID(args.ID, "id"); err != nil {
			return adewire.LinkResult{}, err
		}
	default:
		return adewire.LinkResult{}, ipcerr.Contract("slot %q is not jira, host or stored", args.Slot)
	}
	switch {
	case (args.Text == "") == (args.Kind == ""):
		return adewire.LinkResult{}, ipcerr.Contract("give exactly one of text and kind")
	case args.Kind != "" && !agentmcp.ReportableArtifactKind(args.Kind) && args.Kind != "branch":
		return adewire.LinkResult{}, ipcerr.Contract("kind %q is not a link kind", args.Kind)
	case args.Text != "":
		if err := validateLinkText(args.Text); err != nil {
			return adewire.LinkResult{}, err
		}
	}
	res, err := s.Engine.EditLink(ctx, args)
	return res, adeTaskError(err)
}

// RemoveArtifact deletes a stored artifact. Detected artifacts have no id and cannot be removed.
func (s *AdeTaskService) RemoveArtifact(ctx context.Context, args adewire.RemoveArtifactArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
		return err
	}
	if err := validateAdeItemID(args.ID, "id"); err != nil {
		return err
	}
	return adeTaskError(s.Engine.RemoveArtifact(ctx, args))
}

func (s *AdeTaskService) SetTaskStep(ctx context.Context, args adewire.SetTaskStepArgs) (adewire.Task, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
		return adewire.Task{}, err
	}
	if err := validateAdeItemID(args.StepID, "stepId"); err != nil {
		return adewire.Task{}, err
	}
	if err := validateAdeFromStep(args.FromStepID); err != nil {
		return adewire.Task{}, err
	}
	if err := validateAdeHandoverNotes(args.Notes); err != nil {
		return adewire.Task{}, err
	}
	t, err := s.Engine.SetTaskStep(ctx, args.TaskID, args.StepID, args.FromStepID, args.Notes, args.CloseSessions)
	return t, adeTaskError(err)
}

func (s *AdeTaskService) RetrySetup(ctx context.Context, args adewire.BranchArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return err
	}
	return adeTaskError(s.Engine.RetrySetup(ctx, args.BranchID))
}

func (s *AdeTaskService) ReadLog(ctx context.Context, args adewire.ReadLogArgs) (adewire.LogPage, error) {
	ctx = adeUserCtx(ctx)
	if args.Kind != "run" && args.Kind != "setup" {
		return adewire.LogPage{}, adeTaskInvalid("kind must be run or setup")
	}
	if err := validateAdeItemID(args.ID, "id"); err != nil {
		return adewire.LogPage{}, err
	}
	if args.AfterSeq < 0 {
		return adewire.LogPage{}, adeTaskInvalid("afterSeq must not be negative")
	}
	p, err := s.Engine.ReadLog(ctx, args)
	return p, adeTaskError(err)
}

// RunSandbox returns the sandbox a headless run started with.
func (s *AdeTaskService) RunSandbox(ctx context.Context, args adewire.RunArgs) (adewire.RunSandboxDetail, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.RunID, "runId"); err != nil {
		return adewire.RunSandboxDetail{}, err
	}
	d, err := s.Engine.RunSandbox(ctx, args.RunID)
	return d, adeTaskError(err)
}

// SessionSandbox returns the sandbox a manual step session started with.
func (s *AdeTaskService) SessionSandbox(ctx context.Context, args adewire.SessionSandboxArgs) (adewire.RunSandboxDetail, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.SessionID, "sessionId"); err != nil {
		return adewire.RunSandboxDetail{}, err
	}
	d, err := s.Engine.SessionSandbox(ctx, args.SessionID)
	return d, adeTaskError(err)
}

func (s *AdeTaskService) Sessions(ctx context.Context) (adewire.SessionsResult, error) {
	ctx = adeUserCtx(ctx)
	r, err := s.Engine.Sessions(ctx)
	return r, adeTaskError(err)
}

func (s *AdeTaskService) StopRun(ctx context.Context, args adewire.RunArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.RunID, "runId"); err != nil {
		return err
	}
	return adeTaskError(s.Engine.StopRun(ctx, args.RunID))
}

func (s *AdeTaskService) TakeOver(ctx context.Context, args adewire.TakeOverArgs) (adewire.Launch, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.SessionID, "sessionId"); err != nil {
		return adewire.Launch{}, err
	}
	l, err := s.Engine.TakeOver(ctx, args)
	return l, adeTaskError(err)
}

func (s *AdeTaskService) RestartSession(ctx context.Context, args adewire.RestartSessionArgs) (adewire.Launch, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.SessionID, "sessionId"); err != nil {
		return adewire.Launch{}, err
	}
	l, err := s.Engine.RestartSession(ctx, args)
	return l, adeTaskError(err)
}

func (s *AdeTaskService) CloseSession(ctx context.Context, args adewire.CloseSessionArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.SessionID, "sessionId"); err != nil {
		return err
	}
	return adeTaskError(s.Engine.CloseSession(ctx, args))
}

func (s *AdeTaskService) LaunchStep(ctx context.Context, args adewire.LaunchStepArgs) (adewire.Launch, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
		return adewire.Launch{}, err
	}
	if len(args.Message) > adeMaxMessageBytes {
		return adewire.Launch{}, adeMessageTooLong()
	}
	if err := validateAdeFromStep(args.FromStepID); err != nil {
		return adewire.Launch{}, err
	}
	l, err := s.Engine.LaunchStep(ctx, args)
	return l, adeTaskError(err)
}

func (s *AdeTaskService) StartBranch(ctx context.Context, args adewire.StartBranchArgs) (adewire.Launch, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return adewire.Launch{}, err
	}
	if len(args.Message) > adeMaxMessageBytes {
		return adewire.Launch{}, adeMessageTooLong()
	}
	l, err := s.Engine.StartBranch(ctx, args)
	return l, adeTaskError(err)
}

func (s *AdeTaskService) StartAdHoc(ctx context.Context, args adewire.StartAdHocArgs) (adewire.Launch, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return adewire.Launch{}, err
	}
	if args.Provider != "" && !appsettings.ValidProviderID(args.Provider) {
		return adewire.Launch{}, adeTaskInvalid("provider is not a valid agent id")
	}
	l, err := s.Engine.StartAdHoc(ctx, args)
	return l, adeTaskError(err)
}

func (s *AdeTaskService) Send(ctx context.Context, args adewire.SendArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.SessionID, "sessionId"); err != nil {
		return err
	}
	if args.Message == "" {
		return adeTaskInvalid("message is required")
	}
	if len(args.Message) > adeMaxMessageBytes {
		return adeMessageTooLong()
	}
	return adeTaskError(s.Engine.Send(ctx, args))
}

// FocusSession brings the window that owns a running session's terminal forward and tells it which
// task, branch and session to show. It returns false, never an error, when the session stopped or its
// window closed meanwhile.
func (s *AdeTaskService) FocusSession(_ context.Context, args adewire.FocusSessionArgs) (bool, error) {
	if err := validateAdeItemID(args.SessionID, "sessionId"); err != nil {
		return false, err
	}
	if args.TaskID != "" {
		if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
			return false, err
		}
	}
	rec, err := s.Engine.TUISession(args.SessionID)
	if err != nil {
		return false, adeTaskError(err)
	}
	if rec == nil {
		return false, adeTaskError(ade.ErrSessionNotFound)
	}
	if rec.Mode != model.AdeSessionModeTUI || rec.State != model.AdeSessionStateRunning || s.Registry == nil {
		return false, nil
	}
	windowKey, ok := s.Registry.WindowOf(rec.TerminalID)
	if !ok || s.FocusWindow == nil || !s.FocusWindow(windowKey) {
		return false, nil
	}
	taskID := args.TaskID
	if taskID == "" {
		taskID = rec.TaskID
	}
	AdeTaskOpenSession(s.Emit, windowKey, adewire.OpenSessionEvent{TaskID: taskID, BranchID: rec.BranchID, SessionID: rec.ID})
	return true, nil
}

func (s *AdeTaskService) ArchiveRisk(ctx context.Context, args adewire.TaskArgs) (adewire.ArchiveRisk, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
		return adewire.ArchiveRisk{}, err
	}
	r, err := s.Engine.ArchiveRisk(ctx, args.TaskID)
	return r, adeTaskError(err)
}

func (s *AdeTaskService) ArchiveTask(ctx context.Context, args adewire.TaskArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
		return err
	}
	return adeTaskError(s.Engine.ArchiveTask(ctx, args.TaskID))
}

// BranchRequest reads an agent's open branch request; E_NOT_FOUND once it is answered.
func (s *AdeTaskService) BranchRequest(args adewire.BranchRequestArgs) (adewire.BranchRequest, error) {
	if err := validateAdeItemID(args.ID, "id"); err != nil {
		return adewire.BranchRequest{}, err
	}
	r, err := s.Engine.BranchRequest(args.ID)
	return r, adeTaskError(err)
}

// ApproveBranchRequest approves the checked repos of a request and denies the rest.
func (s *AdeTaskService) ApproveBranchRequest(args adewire.ApproveBranchRequestArgs) (adewire.BranchRequestAnswer, error) {
	if err := validateAdeItemID(args.ID, "id"); err != nil {
		return adewire.BranchRequestAnswer{}, err
	}
	if len(args.CodeRepoIDs) == 0 {
		return adewire.BranchRequestAnswer{}, adeTaskInvalid("codeRepoIds is empty")
	}
	if err := validateAdeTaskRepoIDs(args.CodeRepoIDs); err != nil {
		return adewire.BranchRequestAnswer{}, err
	}
	resolved, err := s.Engine.ApproveBranchRequest(args.ID, args.CodeRepoIDs)
	return adewire.BranchRequestAnswer{Resolved: resolved}, adeTaskError(err)
}

// DenyBranchRequest denies every repo of a request.
func (s *AdeTaskService) DenyBranchRequest(args adewire.BranchRequestArgs) (adewire.BranchRequestAnswer, error) {
	if err := validateAdeItemID(args.ID, "id"); err != nil {
		return adewire.BranchRequestAnswer{}, err
	}
	return adewire.BranchRequestAnswer{Resolved: s.Engine.DenyBranchRequest(args.ID)}, nil
}

// ArchivedTask reads an archived task with everything it kept.
func (s *AdeTaskService) ArchivedTask(ctx context.Context, args adewire.TaskArgs) (adewire.ArchivedTask, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
		return adewire.ArchivedTask{}, err
	}
	r, err := s.Engine.ArchivedTask(ctx, args.TaskID)
	return r, adeTaskError(err)
}

// RestoreTask unarchives a task into Later.
func (s *AdeTaskService) RestoreTask(ctx context.Context, args adewire.TaskArgs) (adewire.Task, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
		return adewire.Task{}, err
	}
	t, err := s.Engine.RestoreTask(ctx, args.TaskID)
	return t, adeTaskError(err)
}

// OpenReviewWindow opens the branch's review window, or focuses it when already open. true = opened.
func (s *AdeTaskService) OpenReviewWindow(ctx context.Context, args adewire.BranchArgs) (bool, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return false, err
	}
	if s.OpenWindow == nil || s.FocusWindow == nil {
		return false, adeTaskInvalid("review windows are not available")
	}
	res, err := s.Engine.OpenReviewWindow(ctx, args.BranchID)
	if err != nil {
		return false, adeTaskError(err)
	}
	if res.Existing {
		s.FocusWindow(res.Key)
		return false, nil
	}
	s.OpenWindow(shell.WindowRecord{Key: res.Key, Order: res.Order})
	if s.SetWindowTitle != nil {
		s.SetWindowTitle(res.Key, res.Title)
	}
	return true, nil
}

// ReviewWindowTarget returns what the window with this key reviews, null when it is no review window.
func (s *AdeTaskService) ReviewWindowTarget(ctx context.Context, args adewire.WindowKeyArgs) (*adewire.ReviewWindowTarget, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.WindowKey, "windowKey"); err != nil {
		return nil, err
	}
	t, err := s.Engine.ReviewWindowTarget(ctx, args.WindowKey)
	return t, adeTaskError(err)
}

// ReviewWorking lists the uncommitted changes of the branch's worktree against its HEAD.
func (s *AdeTaskService) ReviewWorking(ctx context.Context, args adewire.BranchArgs) (adewire.ReviewWorking, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return adewire.ReviewWorking{}, err
	}
	r, err := s.Engine.ReviewWorking(ctx, args.BranchID)
	return r, adeTaskError(err)
}

// ReviewWorkingDiff reads one uncommitted file of the branch's worktree for a diff tab.
func (s *AdeTaskService) ReviewWorkingDiff(ctx context.Context, args adewire.ReviewWorkingDiffArgs) (codeworkspace.DiffContent, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return codeworkspace.DiffContent{}, err
	}
	if args.Path == "" {
		return codeworkspace.DiffContent{}, adeTaskInvalid("path is required")
	}
	d, err := s.Engine.ReviewWorkingDiff(ctx, args.BranchID, args.Path)
	return d, adeTaskError(err)
}

// ReviewAgent returns the task's review agent session and, when it runs, the window hosting its terminal.
func (s *AdeTaskService) ReviewAgent(_ context.Context, args adewire.TaskArgs) (adewire.ReviewAgentState, error) {
	if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
		return adewire.ReviewAgentState{}, err
	}
	sess, err := s.Engine.ReviewAgent(args.TaskID)
	if err != nil {
		return adewire.ReviewAgentState{}, adeTaskError(err)
	}
	out := adewire.ReviewAgentState{Session: sess}
	if sess != nil && sess.State == model.AdeSessionStateRunning && sess.TerminalID != "" && s.Registry != nil {
		if key, ok := s.Registry.WindowOf(sess.TerminalID); ok {
			out.HostWindowKey = key
		}
	}
	return out, nil
}

// LaunchReviewAgent starts or resumes the task's review agent. E_CONFLICT when one already runs.
func (s *AdeTaskService) LaunchReviewAgent(ctx context.Context, args adewire.TaskArgs) (adewire.ReviewAgentLaunch, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeTaskID(args.TaskID, "taskId"); err != nil {
		return adewire.ReviewAgentLaunch{}, err
	}
	l, err := s.Engine.LaunchReviewAgent(ctx, args.TaskID)
	return l, adeTaskError(err)
}

// ForwardReviewComments pastes the branch's open review comments into its task's running review agent.
func (s *AdeTaskService) ForwardReviewComments(ctx context.Context, args adewire.BranchArgs) (adewire.ForwardReviewResult, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return adewire.ForwardReviewResult{}, err
	}
	n, err := s.Engine.ForwardReviewComments(ctx, args.BranchID)
	return adewire.ForwardReviewResult{Count: n}, adeTaskError(err)
}

// ReviewForwardText returns the message ForwardReviewComments would send. It sends and stamps nothing.
func (s *AdeTaskService) ReviewForwardText(ctx context.Context, args adewire.BranchArgs) (adewire.ReviewForwardText, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return adewire.ReviewForwardText{}, err
	}
	text, err := s.Engine.ReviewForwardText(ctx, args.BranchID)
	return adewire.ReviewForwardText{Text: text}, adeTaskError(err)
}
