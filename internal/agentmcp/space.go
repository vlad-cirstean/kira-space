package agentmcp

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

const (
	maxDeclareRepos = 20
	maxWaitSeconds  = 600
)

// ToolError is a failure the agent can act on; its text is returned to the agent verbatim. Any other
// error reaches the agent as a generic failure.
type ToolError string

func (e ToolError) Error() string { return string(e) }

// SetupInfo is a worktree's prepare-script state: none (no worktree yet), running, ready or failed.
type SetupInfo struct {
	State string `json:"state"`
	Note  string `json:"note"`
}

// TaskRepo is a repo on the task and its branch, when created.
type TaskRepo struct {
	Repo       string    `json:"repo"`
	CodeRepoID string    `json:"codeRepoId"`
	Branch     *string   `json:"branch"`
	Worktree   string    `json:"worktree"`
	Setup      SetupInfo `json:"setup"`
}

// RegisteredRepo is a repo imported into Kira Space; only these can be declared.
type RegisteredRepo struct {
	Name       string `json:"name"`
	Nickname   string `json:"nickname"`
	CodeRepoID string `json:"codeRepoId"`
	Path       string `json:"path"`
}

// TaskInfo is the task as an agent sees it.
type TaskInfo struct {
	Title      string           `json:"title"`
	JiraKey    string           `json:"jiraKey"`
	Workflow   string           `json:"workflow"`
	Step       string           `json:"step"`
	Repos      []TaskRepo       `json:"repos"`
	Registered []RegisteredRepo `json:"registered"`
}

// BranchInfo is one branch of the task and the state of its worktree.
type BranchInfo struct {
	Repo     string    `json:"repo"`
	Branch   string    `json:"branch"`
	Worktree string    `json:"worktree"`
	Setup    SetupInfo `json:"setup"`
	Next     string    `json:"next"`
}

// Outcomes of one branch in a request_branches call.
const (
	BranchCreated  = "created"
	BranchExisting = "existing"
	BranchDenied   = "denied"
	BranchFailed   = "failed"
	BranchTimeout  = "timeout"
)

// RequestedBranch is the answer for one branch of a request_branches call.
type RequestedBranch struct {
	BranchInfo
	Outcome string `json:"outcome"`
}

// RequestBranchesResult is request_branches' answer, one entry per branch asked for.
type RequestBranchesResult struct {
	Branches []RequestedBranch `json:"branches"`
}

// Outcomes of rename_branch; renamed, denied, failed and timeout reuse the request_branches words.
const BranchRenamed = "renamed"

// Remote branch states after rename_branch: renamed, kept (left as it was), untracked (the branch had no
// remote branch) or failed (the local rename stands).
const (
	RemoteRenamed   = "renamed"
	RemoteKept      = "kept"
	RemoteUntracked = "untracked"
	RemoteFailed    = "failed"
)

// RenameBranchArgs asks to rename a branch of the task.
type RenameBranchArgs struct {
	Repo         string `json:"repo"`
	From         string `json:"from"`
	To           string `json:"to"`
	RenameRemote bool   `json:"renameRemote,omitempty"`
}

// RenameBranchResult is rename_branch's answer.
type RenameBranchResult struct {
	Repo     string `json:"repo"`
	From     string `json:"from"`
	To       string `json:"to"`
	Outcome  string `json:"outcome"`
	Remote   string `json:"remote"`
	Worktree string `json:"worktree"`
	Next     string `json:"next"`
}

// OperationInfo is one running operation of the task that an agent may stop.
type OperationInfo struct {
	OpID      string `json:"opId"`
	Kind      string `json:"kind"`
	Subject   string `json:"subject,omitempty"`
	StartedAt string `json:"startedAt"`
	ElapsedMs int64  `json:"elapsedMs"`
}

// ListOperationsResult is list_operations' answer.
type ListOperationsResult struct {
	Operations []OperationInfo `json:"operations"`
}

// CancelOperationArgs is cancel_operation's input.
type CancelOperationArgs struct {
	OpID string `json:"opId"`
}

// CancelOperationResult is cancel_operation's answer.
type CancelOperationResult struct {
	OpID      string `json:"opId"`
	Kind      string `json:"kind"`
	Cancelled bool   `json:"cancelled"`
}

// SpaceCall identifies the caller of a Space tool: the task, and the run or stage it works for.
type SpaceCall struct {
	TaskID string
	// RunID is the headless run behind the call, "" for a session.
	RunID string
	// StepID is the stage a session was opened for, "" for a run.
	StepID string
	// SessionID is the Kira session record that made the call, "" when unknown.
	SessionID string
}

// SpaceTools is what the Kira Space tools call; *ade.TaskBoard implements it. Every method acts on
// one task only. RequestBranches blocks until the user answers; ctx ends when the caller is gone.
type SpaceTools interface {
	TaskInfo(ctx context.Context, taskID string) (TaskInfo, error)
	DeclareRepos(ctx context.Context, taskID string, repos []string) (TaskInfo, error)
	RequestBranches(ctx context.Context, call SpaceCall, branches []BranchSpec) (RequestBranchesResult, error)
	BranchStatus(ctx context.Context, taskID, repo, branch string, wait time.Duration) (BranchInfo, error)
	RenameBranch(ctx context.Context, call SpaceCall, args RenameBranchArgs) (RenameBranchResult, error)
	ReportArtifacts(ctx context.Context, call SpaceCall, in []ArtifactInput) (ReportArtifactsResult, error)
	// ListArtifacts lists every artifact of the task; a failing source is a gap, not an error.
	ListArtifacts(ctx context.Context, taskID string) (TaskArtifacts, error)
	// ListOperations lists the task's running operations that can be stopped; background rows stay out.
	ListOperations(ctx context.Context, taskID string) ([]OperationInfo, error)
	// CancelOperation stops one of them; an unknown or finished id is a ToolError.
	CancelOperation(ctx context.Context, taskID, opID string) (CancelOperationResult, error)
}

type declareArgs struct {
	Repos []string `json:"repos"`
}

// BranchSpec is one branch an agent asks for: a task or registered repo and the branch name the agent chose.
type BranchSpec struct {
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
}

type requestBranchesArgs struct {
	Branches []BranchSpec `json:"branches"`
}

type branchStatusArgs struct {
	Repo        string `json:"repo"`
	Branch      string `json:"branch"`
	WaitSeconds int    `json:"waitSeconds,omitempty"`
}

type reportArtifactsArgs struct {
	Artifacts []ArtifactInput `json:"artifacts"`
}

type noArgs struct{}

// spaceTask resolves the Space grant behind a call; every Space tool acts on that task alone.
func (s *Server) spaceTask(req *mcp.CallToolRequest) (SpaceCall, *mcp.CallToolResult) {
	var g Grant
	var ok bool
	if req.Extra != nil {
		g, ok = s.grant(req.Extra.TokenInfo)
	}
	if !ok || !g.Space || g.TaskID == "" {
		return SpaceCall{}, toolError("Kira Space tools are not available for this session.")
	}
	return SpaceCall{TaskID: g.TaskID, RunID: g.RunID, StepID: g.StepID, SessionID: g.SessionID}, nil
}

// cleanBranches trims and dedups specs by (repo, branch); it returns the refusal text when the list is
// empty, too long or holds a blank field.
func cleanBranches(specs []BranchSpec) ([]BranchSpec, string) {
	if n := len(specs); n == 0 || n > maxDeclareRepos {
		return nil, "branches needs 1 to 20 entries."
	}
	out := make([]BranchSpec, 0, len(specs))
	seen := map[BranchSpec]bool{}
	for _, sp := range specs {
		sp = BranchSpec{Repo: strings.TrimSpace(sp.Repo), Branch: strings.TrimSpace(sp.Branch)}
		if sp.Repo == "" || sp.Branch == "" {
			return nil, "Every branches entry needs a repo and a branch."
		}
		if !seen[sp] {
			seen[sp] = true
			out = append(out, sp)
		}
	}
	return out, ""
}

// fail turns an engine error into a tool result: a ToolError verbatim, anything else generic.
func fail(tool string, err error) *mcp.CallToolResult {
	var te ToolError
	if errors.As(err, &te) {
		return toolError(te.Error())
	}
	slog.Warn("agentmcp: space tool", "scope", "ade", "tool", tool, "err", err)
	return toolError("Kira Space could not complete that. Try again, or ask the user.")
}

func (s *Server) addSpaceTools(srv *mcp.Server) {
	s.addTaskInfoTool(srv)
	s.addDeclareReposTool(srv)
	s.addRequestBranchesTool(srv)
	s.addBranchStatusTool(srv)
	s.addRenameBranchTool(srv)
	s.addReportArtifactsTool(srv)
	s.addOperationTools(srv)
}

func (s *Server) addOperationTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_operations",
		Description: llmprompts.HeadlessToolListOperations.Text(s.prompts, llmprompts.NoVars{}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, ListOperationsResult, error) {
		call, bad := s.spaceTask(req)
		if bad != nil {
			return bad, ListOperationsResult{}, nil
		}
		ctx, done := s.callCtx(ctx, req)
		defer done()
		ops, err := s.space.ListOperations(ctx, call.TaskID)
		if err != nil {
			return fail("list_operations", err), ListOperationsResult{}, nil
		}
		if ops == nil {
			ops = []OperationInfo{}
		}
		return nil, ListOperationsResult{Operations: ops}, nil
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "cancel_operation",
		Description: llmprompts.HeadlessToolCancelOperation.Text(s.prompts, llmprompts.NoVars{}),
		InputSchema: llmprompts.RequireNonEmpty(llmprompts.InputSchema[CancelOperationArgs](s.prompts, llmprompts.HeadlessToolCancelOperation.ID(), nil), "opId"),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CancelOperationArgs) (*mcp.CallToolResult, CancelOperationResult, error) {
		call, bad := s.spaceTask(req)
		if bad != nil {
			return bad, CancelOperationResult{}, nil
		}
		opID := strings.TrimSpace(args.OpID)
		if opID == "" {
			return toolError("opId is required."), CancelOperationResult{}, nil
		}
		ctx, done := s.callCtx(ctx, req)
		defer done()
		res, err := s.space.CancelOperation(ctx, call.TaskID, opID)
		if err != nil {
			return fail("cancel_operation", err), CancelOperationResult{}, nil
		}
		return nil, res, nil
	})
}

func (s *Server) addTaskInfoTool(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "task_info",
		Description: llmprompts.HeadlessToolTaskInfo.Text(s.prompts, llmprompts.NoVars{}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, TaskInfo, error) {
		call, bad := s.spaceTask(req)
		if bad != nil {
			return bad, TaskInfo{}, nil
		}
		ctx, done := s.callCtx(ctx, req)
		defer done()
		info, err := s.space.TaskInfo(ctx, call.TaskID)
		if err != nil {
			return fail("task_info", err), TaskInfo{}, nil
		}
		return nil, info, nil
	})
}

func (s *Server) addDeclareReposTool(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "declare_repos",
		Description: llmprompts.HeadlessToolDeclareRepos.Text(s.prompts, llmprompts.NoVars{}),
		InputSchema: llmprompts.RequireNonEmpty(llmprompts.InputSchema[declareArgs](s.prompts, llmprompts.HeadlessToolDeclareRepos.ID(), map[string]any{
			"repos": llmprompts.MaxVars{Max: maxDeclareRepos},
		}), "repos"),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args declareArgs) (*mcp.CallToolResult, TaskInfo, error) {
		call, bad := s.spaceTask(req)
		if bad != nil {
			return bad, TaskInfo{}, nil
		}
		if n := len(args.Repos); n == 0 || n > maxDeclareRepos {
			return toolError("repos needs 1 to 20 entries."), TaskInfo{}, nil
		}
		ctx, done := s.callCtx(ctx, req)
		defer done()
		info, err := s.space.DeclareRepos(ctx, call.TaskID, args.Repos)
		if err != nil {
			return fail("declare_repos", err), TaskInfo{}, nil
		}
		return nil, info, nil
	})
}

func (s *Server) addRequestBranchesTool(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "request_branches",
		Description: llmprompts.HeadlessToolRequestBranches.Text(s.prompts, llmprompts.NoVars{}),
		InputSchema: llmprompts.RequireNonEmpty(llmprompts.InputSchema[requestBranchesArgs](s.prompts, llmprompts.HeadlessToolRequestBranches.ID(), map[string]any{
			"branches": llmprompts.MaxVars{Max: maxDeclareRepos},
		}), "branches"),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args requestBranchesArgs) (*mcp.CallToolResult, RequestBranchesResult, error) {
		call, bad := s.spaceTask(req)
		if bad != nil {
			return bad, RequestBranchesResult{}, nil
		}
		specs, msg := cleanBranches(args.Branches)
		if msg != "" {
			return toolError(msg), RequestBranchesResult{}, nil
		}
		ctx, done := s.callCtx(ctx, req)
		defer done()
		res, err := s.space.RequestBranches(ctx, call, specs)
		if err != nil {
			return fail("request_branches", err), RequestBranchesResult{}, nil
		}
		return nil, res, nil
	})
}

func (s *Server) addBranchStatusTool(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "branch_status",
		Description: llmprompts.HeadlessToolBranchStatus.Text(s.prompts, llmprompts.NoVars{}),
		InputSchema: llmprompts.RequireNonEmpty(llmprompts.InputSchema[branchStatusArgs](s.prompts, llmprompts.HeadlessToolBranchStatus.ID(), map[string]any{
			"waitSeconds": llmprompts.MaxVars{Max: maxWaitSeconds},
		}), "repo", "branch"),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args branchStatusArgs) (*mcp.CallToolResult, BranchInfo, error) {
		call, bad := s.spaceTask(req)
		if bad != nil {
			return bad, BranchInfo{}, nil
		}
		if args.WaitSeconds < 0 || args.WaitSeconds > maxWaitSeconds {
			return toolError("waitSeconds must be 0 to 600."), BranchInfo{}, nil
		}
		ctx, done := s.callCtx(ctx, req)
		defer done()
		info, err := s.space.BranchStatus(ctx, call.TaskID, args.Repo, args.Branch, time.Duration(args.WaitSeconds)*time.Second)
		if err != nil {
			return fail("branch_status", err), BranchInfo{}, nil
		}
		return nil, info, nil
	})
}

func (s *Server) addRenameBranchTool(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "rename_branch",
		Description: llmprompts.HeadlessToolRenameBranch.Text(s.prompts, llmprompts.NoVars{}),
		InputSchema: llmprompts.RequireNonEmpty(llmprompts.InputSchema[RenameBranchArgs](s.prompts, llmprompts.HeadlessToolRenameBranch.ID(), nil), "repo", "from", "to"),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args RenameBranchArgs) (*mcp.CallToolResult, RenameBranchResult, error) {
		call, bad := s.spaceTask(req)
		if bad != nil {
			return bad, RenameBranchResult{}, nil
		}
		args.From, args.To = strings.TrimSpace(args.From), strings.TrimSpace(args.To)
		if args.From == "" || args.To == "" || args.From == args.To {
			return toolError("from and to must be two different branch names."), RenameBranchResult{}, nil
		}
		ctx, done := s.callCtx(ctx, req)
		defer done()
		res, err := s.space.RenameBranch(ctx, call, args)
		if err != nil {
			return fail("rename_branch", err), RenameBranchResult{}, nil
		}
		return nil, res, nil
	})
}

func (s *Server) addReportArtifactsTool(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "report_artifacts",
		Description: llmprompts.HeadlessToolReportArtifacts.Text(s.prompts, llmprompts.NoVars{}),
		InputSchema: llmprompts.RequireNonEmpty(llmprompts.InputSchema[reportArtifactsArgs](s.prompts, llmprompts.HeadlessToolReportArtifacts.ID(), map[string]any{
			"artifacts": llmprompts.MaxVars{Max: MaxArtifactsPerCall},
		}), "artifacts"),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args reportArtifactsArgs) (*mcp.CallToolResult, ReportArtifactsResult, error) {
		call, bad := s.spaceTask(req)
		if bad != nil {
			return bad, ReportArtifactsResult{}, nil
		}
		ctx, done := s.callCtx(ctx, req)
		defer done()
		res, err := s.space.ReportArtifacts(ctx, call, args.Artifacts)
		if err != nil {
			return fail("report_artifacts", err), ReportArtifactsResult{}, nil
		}
		return nil, res, nil
	})
}
