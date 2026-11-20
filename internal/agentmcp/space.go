package agentmcp

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxDeclareRepos = 20
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
	// Mismatch is set when the branch is not checked out in the row's own worktree folder.
	Mismatch *WorktreeMismatch `json:"mismatch,omitempty"`
}

// WorktreeMismatch says what the row's own folder holds instead of the task branch. CheckedOut is
// "HEAD" for a detached commit.
type WorktreeMismatch struct {
	Folder     string `json:"folder" yaml:"folder"`
	CheckedOut string `json:"checkedOut" yaml:"checkedOut"`
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
	// Branch is set when the call named a repo and branch: that branch's worktree state.
	Branch *BranchInfo `json:"branch,omitempty"`
}

// BranchInfo is one branch of the task and the state of its worktree.
type BranchInfo struct {
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	// Base is the branch this one starts from; absent for the repo's main.
	Base     string    `json:"base,omitempty"`
	Worktree string    `json:"worktree"`
	Setup    SetupInfo `json:"setup"`
	Next     string    `json:"next"`
}

// Outcomes of one branch in a branch.request call.
const (
	BranchCreated  = "created"
	BranchExisting = "existing"
	// BranchCheckedOut: an existing branch was checked out in the task's own worktree.
	BranchCheckedOut = "checkedOut"
	BranchDenied     = "denied"
	BranchFailed     = "failed"
	// BranchPending: the call wait ended with the user's approval still open. The approval stays
	// open and applies on its own when the user answers.
	BranchPending = "pending"
	// BranchWaiting: the branch exists but its worktree setup was still running when the wait ended.
	BranchWaiting = "waiting"
)

// RequestedBranch is the answer for one branch of a branch.request call.
type RequestedBranch struct {
	BranchInfo
	Outcome string `json:"outcome"`
}

// RequestBranchesResult is branch.request's answer, one entry per branch asked for. Waited is how long
// the call waited, for a pending or waiting entry.
type RequestBranchesResult struct {
	Branches []RequestedBranch `json:"branches"`
	Waited   time.Duration     `json:"-"`
}

// Outcomes of branch.rename; denied, failed and pending reuse the branch.request words.
const BranchRenamed = "renamed"

// Remote branch states after branch.rename: renamed, kept (left as it was), untracked (the branch had no
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

// RenameBranchResult is branch.rename's answer.
type RenameBranchResult struct {
	Repo     string `json:"repo"`
	From     string `json:"from"`
	To       string `json:"to"`
	Outcome  string `json:"outcome"`
	Remote   string `json:"remote"`
	Worktree string `json:"worktree"`
	Next     string `json:"next"`
	// Waited is how long the call waited, for a pending outcome.
	Waited time.Duration `json:"-"`
}

// BranchChanged is branch.change's success outcome; denied, failed and pending reuse the branch.request words.
const BranchChanged = "changed"

// ChangeBranchArgs asks to re-point the task's branch From at the existing local branch To.
type ChangeBranchArgs struct {
	Repo string `json:"repo"`
	From string `json:"from"`
	To   string `json:"to"`
}

// ChangeBranchResult is branch.change's answer.
type ChangeBranchResult struct {
	Repo     string `json:"repo"`
	From     string `json:"from"`
	To       string `json:"to"`
	Outcome  string `json:"outcome"`
	Worktree string `json:"worktree"`
	Next     string `json:"next"`
	// Waited is how long the call waited, for a pending outcome.
	Waited time.Duration `json:"-"`
}

// OperationInfo is one running operation of the task that an agent may stop.
type OperationInfo struct {
	OpID      string `json:"opId"`
	Kind      string `json:"kind"`
	Subject   string `json:"subject,omitempty"`
	StartedAt string `json:"startedAt"`
	ElapsedMs int64  `json:"elapsedMs"`
}

// CancelOperationResult is the engine's answer to a cancel.
type CancelOperationResult struct {
	OpID      string
	Kind      string
	Cancelled bool
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
	// Orchestrator is true for the orchestrator on its own task.
	Orchestrator bool
}

// SpaceTools is what the state and submit tools call; *ade.TaskBoard implements it. Every method acts
// on one task only. RequestBranches, RenameBranch, ChangeBranch and AcquireLease block until the user
// answers or the call wait ends; ctx ends when the caller is gone.
type SpaceTools interface {
	LeaseTools
	DeclareRepos(ctx context.Context, taskID string, repos []string) (TaskInfo, error)
	RequestBranches(ctx context.Context, call SpaceCall, branches []BranchSpec) (RequestBranchesResult, error)
	RenameBranch(ctx context.Context, call SpaceCall, args RenameBranchArgs) (RenameBranchResult, error)
	ChangeBranch(ctx context.Context, call SpaceCall, args ChangeBranchArgs) (ChangeBranchResult, error)
	// Submit reads the named task files (all when none) as the calling session; the reply is the count line then one line per file.
	Submit(ctx context.Context, call SpaceCall, in SubmitArgs) (string, error)
	// SubmitForFinish is a run's step.finish implicit submit; an ok finish with no draft is refused
	// while its result's route continues, except for a smart step.
	SubmitForFinish(ctx context.Context, call SpaceCall, status string, ok bool) (SubmitFinish, error)
	// FinishSession is a session's step.finish: it submits the task files, marks the stage's draft
	// finished and asks the user to move the task on. The reply is a note for the agent.
	FinishSession(ctx context.Context, call SpaceCall) (string, error)
	// ListArtifacts lists every artifact of the task; a failing source is a gap, not an error.
	ListArtifacts(ctx context.Context, taskID string) (TaskArtifacts, error)
	// CancelOperation stops one of them; an unknown or finished id is a ToolError.
	CancelOperation(ctx context.Context, taskID, opID string) (CancelOperationResult, error)
	// RemoveWorktree removes one of the task's own worktrees the way the UI's Remove branch does. It never
	// discards uncommitted work: a dirty or busy worktree is a failed result. The git branch stays.
	RemoveWorktree(ctx context.Context, call SpaceCall, args RemoveWorktreeArgs) (RemoveWorktreeResult, error)
}

// BranchSpec is one branch an agent asks for: a task or registered repo, the branch name the agent chose
// and an optional existing branch to start from. Existing asks for a branch that already exists instead
// of creating one; it takes no base.
type BranchSpec struct {
	Repo     string `json:"repo"`
	Branch   string `json:"branch"`
	Base     string `json:"base,omitempty"`
	Existing bool   `json:"existing,omitempty"`
}

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
	return SpaceCall{TaskID: g.TaskID, RunID: g.RunID, StepID: g.StepID, SessionID: g.SessionID, Orchestrator: g.Orchestrator}, nil
}

// cleanBranches trims and dedups specs by (repo, branch, base); it returns the refusal text when the list is
// empty, too long or holds a blank field.
func cleanBranches(specs []BranchSpec) ([]BranchSpec, string) {
	if n := len(specs); n == 0 || n > maxDeclareRepos {
		return nil, "branches needs 1 to 20 entries."
	}
	out := make([]BranchSpec, 0, len(specs))
	seen := map[BranchSpec]bool{}
	for _, sp := range specs {
		sp = BranchSpec{Repo: strings.TrimSpace(sp.Repo), Branch: strings.TrimSpace(sp.Branch), Base: strings.TrimSpace(sp.Base), Existing: sp.Existing}
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

// BranchRemoved is worktree.remove's success outcome; failed reuses the branch.request word.
const BranchRemoved = "removed"

// RemoveWorktreeArgs names the task branch whose worktree goes.
type RemoveWorktreeArgs struct {
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
}

// RemoveWorktreeResult is worktree.remove's answer.
type RemoveWorktreeResult struct {
	Repo     string `json:"repo"`
	Branch   string `json:"branch"`
	Outcome  string `json:"outcome"`
	Worktree string `json:"worktree"`
	Next     string `json:"next"`
}
