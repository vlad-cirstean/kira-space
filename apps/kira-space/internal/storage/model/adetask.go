package model

import (
	"fmt"
	"regexp"
	"slices"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/internal/runoutcome"
)

// AdeTaskKind* are ade_tasks.kind's CHECK values; AdeBranchKind* (below) are the branch kinds.
const (
	AdeTaskKindTask   = "task"
	AdeTaskKindReview = "review"
	AdeTaskKindParked = "parked"
)

// ValidAdeTaskKind mirrors ade_tasks.kind's CHECK.
func ValidAdeTaskKind(v string) bool {
	return v == AdeTaskKindTask || v == AdeTaskKindReview || v == AdeTaskKindParked
}

// AdeRunState* mirror ade_runs.state's CHECK; AdeSetupState* mirror ade_worktree_setup.state's.
const (
	AdeSetupRunning = "running"
	AdeSetupReady   = "ready"
	AdeSetupFailed  = "failed"
)

// AdeTask is one row of ade_tasks. EntryStepID is the step the task last entered ("done" when
// finished, "" without a workflow). SinceJSON is the pass watermark, {"<stepId>/<branchId>":
// attempt}; "" stores NULL. WorkflowJSON is the whole workflow the task began with ("" = not
// started, follows the live file) and WorkflowHash its content hash.
type AdeTask struct {
	ID           string
	Kind         string
	Title        string
	Owner        string
	JiraKey      string
	JiraURL      string
	HostURL      string
	WorkflowID   string
	EntryStepID  string
	SinceJSON    string
	WorkflowJSON string
	WorkflowHash string
	Est          string
	Notes        string
	Color        int
	CreatedAt    int64
	ArchivedAt   *int64
	// StatusID is the task's status; StatusBy says who set it (AdeStatusBy*). StatusStep is the step key
	// whose status the engine last applied ("done" when finished, "" not started).
	StatusID   string
	StatusBy   string
	StatusStep string
}

// Validate asserts what no SQL constraint covers.
func (t AdeTask) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("model: ade task: id is required")
	}
	if !ValidAdeTaskKind(t.Kind) {
		return fmt.Errorf("model: ade task %q: invalid kind %q", t.ID, t.Kind)
	}
	return nil
}

// AdeTaskPatch holds only the leaves the caller changes; nil = unchanged.
type AdeTaskPatch struct {
	Title   *string
	JiraKey *string
	JiraURL *string
	HostURL *string
	Est     *string
	Notes   *string
	Color   *int
	Kind    *string
	// StatusID moves the task to that status by hand (status_by = user).
	StatusID *string
}

// AdeTaskBranch is one row of ade_task_branches. Name "" = branch not created yet.
type AdeTaskBranch struct {
	ID         string
	TaskID     string
	CodeRepoID string
	Name       string
	Kind       string
	Base       string
	// BaseBranchID is the live planner branch this one stacks on ("" = none); Base then holds that
	// branch's name ("" while it is a draft).
	BaseBranchID string
	// BasePendingFrom is the display name of the previous base while a Change base has no verified
	// rebase yet ("" otherwise).
	BasePendingFrom string
	QueuedAfter     string
	Position        int
	HadCommits      bool
	AddedAt         int64
	MergedAt        *int64
	ArchivedAt      *int64
	Origin          string
	// ChangeRequest is the PR recorded at archive; nil = none recorded.
	ChangeRequest *AdeChangeRequest
	// ReviewCommentsJSON holds the review comments moved off the branch at archive or detach; ""
	// = no snapshot.
	ReviewCommentsJSON string
}

// AdeChangeRequest is a pull or merge request stored on a branch row.
type AdeChangeRequest struct {
	Number int
	Title  string
	State  string
	URL    string
}

// AdeBranchOriginAgent marks a branch an agent declared or named.
const AdeBranchOriginAgent = "agent"

// Validate asserts what no SQL constraint covers.
func (b AdeTaskBranch) Validate() error {
	if b.ID == "" || b.TaskID == "" || b.CodeRepoID == "" {
		return fmt.Errorf("model: ade task branch: id, taskId and codeRepoId are required")
	}
	if !ValidAdeBranchKind(b.Kind) {
		return fmt.Errorf("model: ade task branch %q: invalid kind %q", b.ID, b.Kind)
	}
	return nil
}

// AdeTaskPlanRow is one row of ade_task_plan; Day nil = Later, or Backlog when Backlog is set.
type AdeTaskPlanRow struct {
	TaskID   string
	Day      *string
	Position int
	Backlog  bool
}

// AdeRun is one row of ade_runs.
type AdeRun struct {
	ID         string
	TaskID     string
	StepID     string
	BranchID   string
	Attempt    int
	State      string
	Loops      int
	Note       string
	Summary    string
	SessionID  string
	ExitCode   *int
	StartedAt  *int64
	FinishedAt *int64
	Outcome    *AdeRunOutcome
	// Purpose is AdeRunPurposeRebase for a rebase run (outside the workflow), AdeRunPurposeReview for
	// a review round, "" for a step run.
	Purpose string
	// SpecJSON is a rebase run's AdeRebaseSpec or a review round's AdeReviewSpec ("" otherwise).
	SpecJSON string
	Launch   AdeRunLaunch
}

// AdeRunPurposeRebase mirrors ade_runs.purpose's CHECK.
const AdeRunPurposeRebase = "rebase"

// AdeRunPurposeReview marks a review round: an agent run answering a review step's comments.
const AdeRunPurposeReview = "review"

// AdeReviewSpec is a review round's plan, stored in ade_runs.spec_json.
type AdeReviewSpec struct {
	ReviewRunID string  `json:"reviewRunId"`
	Round       int     `json:"round"`
	Route       string  `json:"route"`
	CommentIDs  []int64 `json:"commentIds"`
	HeadBefore  string  `json:"headBefore,omitempty"`
	// HeadAfter is the worktree tip when the round ended ("" while it runs).
	HeadAfter string `json:"headAfter,omitempty"`
	// GitRepoID and Branch key the comments in review.db.
	GitRepoID string `json:"gitRepoId"`
	Branch    string `json:"branch"`
	Fallback  bool   `json:"fallback,omitempty"`
	// AllowedTools are the target step's tools (implementer route); the reply and finish tools are added at launch.
	AllowedTools []string `json:"allowedTools,omitempty"`
}

// AdeReview is one row of ade_reviews: the review step run's send state.
type AdeReview struct {
	RunID             string
	Route             string
	TargetRunID       string
	DedicatedClaudeID string
	Rounds            int
}

// AdeRunOutcome is a run's stored end state: the shared outcome plus what the agent reported and
// what git showed.
type AdeRunOutcome struct {
	runoutcome.Outcome
	Report *AgentReport `json:"report,omitempty"`
	Rebase *RebaseFacts `json:"rebase,omitempty"`
	// Route is where the run's result sent the step: next, end, stop, a later step id, `back:<step id>`
	// for a loop to an earlier step or `retry` for one to the step itself.
	Route string `json:"route,omitempty"`
}

// AgentReport is the optional detail of a finish_step call.
type AgentReport struct {
	ConflictedFiles []string `json:"conflictedFiles,omitempty"`
	LastGitError    string   `json:"lastGitError,omitempty"`
	Tried           string   `json:"tried,omitempty"`
}

// RebaseFacts is what git showed after a rebase run ended.
type RebaseFacts struct {
	Verified        bool         `json:"verified"`
	InProgress      bool         `json:"inProgress"`
	Aborted         bool         `json:"aborted"`
	OntoRef         string       `json:"ontoRef"`
	OntoTip         string       `json:"ontoTip"`
	ConflictedFiles []string     `json:"conflictedFiles"`
	Branches        []BranchShas `json:"branches"`
	Pushed          *bool        `json:"pushed"`
}

// BranchShas is one rebased branch's tip before and after, and whether it sits on its base.
type BranchShas struct {
	BranchID string `json:"branchId"`
	Name     string `json:"name"`
	Before   string `json:"before"`
	After    string `json:"after"`
	OnBase   bool   `json:"onBase"`
}

// AdeRebaseSpec is a rebase run's plan, stored in ade_runs.spec_json.
type AdeRebaseSpec struct {
	CodeRepoID    string           `json:"codeRepoId"`
	Repo          string           `json:"repo"`
	Remote        string           `json:"remote"`
	OntoRef       string           `json:"ontoRef"`
	OntoName      string           `json:"ontoName"`
	OntoTipBefore string           `json:"ontoTipBefore"`
	ChangeBase    bool             `json:"changeBase"`
	Stack         []AdeRebaseStack `json:"stack"`
	Push          bool             `json:"push"`
	Autostash     bool             `json:"autostash"`
	// Review is a review branch the rebase must leave alone (Queue after).
	Review string `json:"review,omitempty"`
}

// AdeRebaseStack is one branch of a rebase run, root first. ParentRef is the ref it goes onto;
// ParentBefore is the parent's tip before the run ("" for the root).
type AdeRebaseStack struct {
	BranchID     string `json:"branchId"`
	Name         string `json:"name"`
	Worktree     string `json:"worktree"`
	ParentRef    string `json:"parentRef"`
	ParentName   string `json:"parentName"`
	ParentBefore string `json:"parentBefore"`
	Before       string `json:"before"`
}

// AdeRunLaunch is a run's launch spec. Non-empty on a never-launched row (what the run needs once its
// worktree gate opens) and on a run a restart stopped (ResumeID only, for Resume); launch clears it.
type AdeRunLaunch struct {
	// Note stays on the run while it runs.
	Note string
	// ResumeID, when set, continues that Claude session with Prompt as the message.
	ResumeID string
	Prompt   string
	// Extra is added after a fresh run's composed prompt.
	Extra string
}

// AdeRunState* mirror ade_runs.state's CHECK.
const (
	AdeRunPending = "pending"
	AdeRunRunning = "running"
	AdeRunStuck   = "stuck"
	AdeRunFailed  = "failed"
	AdeRunBack    = "back"
	AdeRunDone    = "done"
	// AdeRunReview is a review step run waiting for the user; it has no process.
	AdeRunReview = "review"
)

// AdeRunPatch holds only the run leaves the caller changes; nil = unchanged.
type AdeRunPatch struct {
	State      *string
	Note       *string
	Summary    *string
	SessionID  *string
	ExitCode   *int
	StartedAt  *int64
	FinishedAt *int64
	Outcome    *AdeRunOutcome
	Launch     *AdeRunLaunch
	SpecJSON   *string
}

// AdeWorktreeSetup is one row of ade_worktree_setup.
type AdeWorktreeSetup struct {
	BranchID   string
	State      string
	StartedAt  int64
	FinishedAt *int64
	ExitCode   *int
	// Note is why the setup failed ("" otherwise).
	Note string
}

// AdeRepoEnv is one environment of a repo file.
type AdeRepoEnv struct {
	Name              string
	DeployedShaScript string
	// DeployedShaRun runs a Run-module script instead of DeployedShaScript.
	DeployedShaRun *ScriptRef
}

// Equal reports whether e and o configure the same environment.
func (e AdeRepoEnv) Equal(o AdeRepoEnv) bool {
	return e.Name == o.Name && e.DeployedShaScript == o.DeployedShaScript && e.DeployedShaRun.Equal(o.DeployedShaRun)
}

// AdeRepoConfig is a code repo joined with its import source and its repo file's nickname,
// integration branches and environments. A repo without a file carries the defaults.
type AdeRepoConfig struct {
	CodeRepoID          string
	RepoID              string // code_repos.repo_id: the key of its repo file
	Name                string
	Root                string
	Nickname            string
	Source              string
	IntegrationBranches []string
	Environments        []AdeRepoEnv
}

// AdeFolder is one row of ade_folders plus the number of repos imported from it.
type AdeFolder struct {
	Path      string
	Watch     bool
	Hidden    bool
	RepoCount int
	// HiddenCount is how many of the folder's imported repos are hidden.
	HiddenCount int
}

// AdeBranchMark is the last branch tip seen fully contained in a target (kind 'target') or an
// environment (kind 'env'). Recorded is set only by an explicit RecordMerge.
type AdeBranchMark struct {
	BranchID  string
	Kind      string
	Name      string
	MergedTip string
	Recorded  bool
	UpdatedAt int64
}

// AdeEnvState is one environment's last deploy-script result.
type AdeEnvState struct {
	CodeRepoID string
	Env        string
	Sha        string
	PrevSha    string
	Error      string
	CheckedAt  int64
}

// AdeBranchKind* are ade_task_branches.kind's CHECK values: mine (the local git user's own),
// review (someone else's), parked (set aside by the user).
const (
	AdeBranchKindMine   = "mine"
	AdeBranchKindReview = "review"
	AdeBranchKindParked = "parked"
)

// ValidAdeBranchKind mirrors ade_task_branches.kind's CHECK constraint.
func ValidAdeBranchKind(v string) bool {
	switch v {
	case AdeBranchKindMine, AdeBranchKindReview, AdeBranchKindParked:
		return true
	default:
		return false
	}
}

// AdeStatusBy* are ade_tasks.status_by's CHECK values.
const (
	AdeStatusByAuto = "auto"
	AdeStatusByUser = "user"
)

// AdeStatusColors are ade_task_statuses.color's CHECK values, in display order.
var AdeStatusColors = []string{"grey", "brown", "red", "orange", "amber", "yellow", "lime", "green", "teal", "cyan", "sky", "blue", "indigo", "violet", "purple", "pink"}

// Slots of ade_task_status_defaults: the automatic status changes.
const (
	AdeStatusSlotNew      = "new"
	AdeStatusSlotStep     = "step"
	AdeStatusSlotReview   = "review"
	AdeStatusSlotFinished = "finished"
)

// Limits of the status set.
const (
	AdeStatusMin     = 1
	AdeStatusMax     = 20
	AdeStatusNameMax = 40
	AdeStatusIDMax   = 40
)

var adeStatusIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// ValidAdeStatusID reports a well-formed status id.
func ValidAdeStatusID(id string) bool {
	return len(id) <= AdeStatusIDMax && adeStatusIDPattern.MatchString(id)
}

// AdeTaskStatus is one row of ade_task_statuses.
type AdeTaskStatus struct {
	ID       string
	Name     string
	Color    string
	Position int
}

// Validate asserts one status; an empty ID is allowed (a new status gets a slug from its name).
func (s AdeTaskStatus) Validate() error {
	if s.ID != "" && !ValidAdeStatusID(s.ID) {
		return fmt.Errorf("model: ade task status: invalid id %q", s.ID)
	}
	if n := utf8.RuneCountInString(s.Name); n < 1 || n > AdeStatusNameMax {
		return fmt.Errorf("model: ade task status %q: name needs 1-%d characters", s.ID, AdeStatusNameMax)
	}
	if !slices.Contains(AdeStatusColors, s.Color) {
		return fmt.Errorf("model: ade task status %q: invalid color %q", s.ID, s.Color)
	}
	return nil
}

// DefaultAdeTaskStatuses is the seed set shipped with the app.
func DefaultAdeTaskStatuses() []AdeTaskStatus {
	return []AdeTaskStatus{
		{ID: "todo", Name: "To do", Color: "grey", Position: 0},
		{ID: "in-progress", Name: "In progress", Color: "amber", Position: 1},
		{ID: "in-review", Name: "In review", Color: "blue", Position: 2},
		{ID: "done", Name: "Done", Color: "green", Position: 3},
	}
}

// AdeStatusDefaults names the status of each automatic change; an empty field keeps the current status.
// New is required: a task always starts somewhere.
type AdeStatusDefaults struct {
	New      string
	Step     string
	Review   string
	Finished string
}

// DefaultAdeStatusDefaults is the seed of the defaults.
func DefaultAdeStatusDefaults() AdeStatusDefaults {
	return AdeStatusDefaults{New: "todo", Step: "in-progress", Review: "in-review", Finished: "done"}
}

// Validate asserts every named status is in set and New is named.
func (d AdeStatusDefaults) Validate(set []AdeTaskStatus) error {
	in := func(id string) bool {
		return slices.ContainsFunc(set, func(s AdeTaskStatus) bool { return s.ID == id })
	}
	if d.New == "" {
		return fmt.Errorf("model: ade status defaults: new tasks need a status")
	}
	named := [...]struct{ slot, id string }{{AdeStatusSlotNew, d.New}, {AdeStatusSlotStep, d.Step}, {AdeStatusSlotReview, d.Review}, {AdeStatusSlotFinished, d.Finished}}
	for _, n := range named {
		if n.id != "" && !in(n.id) {
			return fmt.Errorf("model: ade status defaults: %s names unknown status %q", n.slot, n.id)
		}
	}
	return nil
}
