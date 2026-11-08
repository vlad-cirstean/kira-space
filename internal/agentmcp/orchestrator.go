package agentmcp

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

// orchestrator.go: the tools only an orchestrator task carries (P571): create_task, list_tasks and
// task_chain. Children never get them, so spawn depth is 1.

// Bounds on create_task's input and task_chain's page.
const (
	maxTaskTitleRunes  = 200
	maxTaskRepos       = 10
	defaultChainLimit  = 10
	maxChainLimit      = 50
	chainTotalMaxBytes = 64 << 10
)

// Outcomes of a create_task call.
const (
	TaskCreated = "created"
	TaskDenied  = "denied"
	TaskTimeout = "timeout"
	TaskFailed  = "failed"
)

// CreateTaskRepo is one repo of a child task: a nickname, name or code repo id, and optionally the ref the
// child's branch starts from.
type CreateTaskRepo struct {
	Repo string `json:"repo"`
	Base string `json:"base,omitempty"`
}

// CreateTaskArgs asks the user to create a child task. Workflow is an id or name. Brief becomes the child's
// first handover.
type CreateTaskArgs struct {
	Title    string           `json:"title"`
	Workflow string           `json:"workflow"`
	Repos    []CreateTaskRepo `json:"repos,omitempty"`
	Brief    string           `json:"brief"`
}

// CreateTaskResult is create_task's answer; TaskID is set when the child was created.
type CreateTaskResult struct {
	Outcome string `json:"outcome"`
	TaskID  string `json:"taskId,omitempty"`
	Title   string `json:"title"`
	Next    string `json:"next"`
}

// ListTasksArgs filters list_tasks: State is live, archived or all (default).
type ListTasksArgs struct {
	State string `json:"state,omitempty"`
}

// ChildTaskInfo is one child of the calling orchestrator. LastHandover is the first bytes of the child's
// newest handed handover.
type ChildTaskInfo struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Kind         string `json:"kind"`
	Workflow     string `json:"workflow"`
	Status       string `json:"status"`
	Step         string `json:"step"`
	Finished     bool   `json:"finished"`
	Archived     bool   `json:"archived"`
	CreatedAt    string `json:"createdAt"`
	LastHandover string `json:"lastHandover"`
}

// ListTasksResult is list_tasks' answer, newest first; More is set when the list was cut.
type ListTasksResult struct {
	Tasks []ChildTaskInfo `json:"tasks"`
	More  bool            `json:"more,omitempty"`
}

// TaskChainArgs reads a handover chain: the caller's own (TaskID empty) or one of its children's. Before
// is a row id from an earlier page's Next.
type TaskChainArgs struct {
	TaskID string `json:"taskId,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Before string `json:"before,omitempty"`
}

// ChainRow is one handover row, drafts included. Body is cut at MaxHandoverBytes with a marker.
type ChainRow struct {
	ID         string `json:"id"`
	From       string `json:"from"`
	To         string `json:"to"`
	Source     string `json:"source"`
	State      string `json:"state"`
	Body       string `json:"body"`
	CreatedAt  string `json:"createdAt"`
	FromTaskID string `json:"fromTaskId,omitempty"`
}

// TaskChainResult is task_chain's answer, newest first. Next is the cursor for the page after it, set when
// More is.
type TaskChainResult struct {
	Rows []ChainRow `json:"rows"`
	More bool       `json:"more,omitempty"`
	Next string     `json:"next,omitempty"`
}

// OrchestratorTools is what the orchestrator tools call; *ade.TaskBoard implements it. Each method acts for
// the calling task only. SpawnTask blocks until the user answers; ctx ends when the caller is gone.
type OrchestratorTools interface {
	SpawnTask(ctx context.Context, call SpaceCall, args CreateTaskArgs) (CreateTaskResult, error)
	ListTasks(ctx context.Context, call SpaceCall, args ListTasksArgs) (ListTasksResult, error)
	TaskChain(ctx context.Context, call SpaceCall, args TaskChainArgs) (TaskChainResult, error)
}

// ChainTotalMaxBytes is the most body text one task_chain answer carries; the engine stops a page there.
const ChainTotalMaxBytes = chainTotalMaxBytes

// ChainBodyCap cuts body to MaxHandoverBytes on a rune boundary, with a marker when it cut.
func ChainBodyCap(body string) string {
	if len(body) <= MaxHandoverBytes {
		return body
	}
	cut := body[:MaxHandoverBytes]
	for !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + "\n[cut]"
}

// orchestratorCall resolves the orchestrator grant behind a call.
func (s *Server) orchestratorCall(req *mcp.CallToolRequest) (SpaceCall, *mcp.CallToolResult) {
	var g Grant
	var ok bool
	if req.Extra != nil {
		g, ok = s.grant(req.Extra.TokenInfo)
	}
	if !ok || !g.Space || !g.Orchestrator || g.TaskID == "" {
		return SpaceCall{}, toolError("This tool is only available to an orchestrator task.")
	}
	return SpaceCall{TaskID: g.TaskID, RunID: g.RunID, StepID: g.StepID, SessionID: g.SessionID}, nil
}

func (s *Server) addOrchestratorTools(srv *mcp.Server) {
	s.addCreateTaskTool(srv)
	s.addListTasksTool(srv)
	s.addTaskChainTool(srv)
}

// cleanCreateTask trims args and returns the refusal text when one field is out of bounds.
func cleanCreateTask(a CreateTaskArgs) (CreateTaskArgs, string) {
	a.Title, a.Workflow, a.Brief = strings.TrimSpace(a.Title), strings.TrimSpace(a.Workflow), strings.TrimSpace(a.Brief)
	switch {
	case a.Title == "" || utf8.RuneCountInString(a.Title) > maxTaskTitleRunes:
		return a, "title needs 1 to 200 characters."
	case a.Workflow == "":
		return a, "workflow is required: a workflow id or name."
	case a.Brief == "":
		return a, "brief is required: what the child task must do."
	case len(a.Brief) > MaxHandoverBytes:
		return a, "brief is too long: at most 16384 bytes."
	case len(a.Repos) > maxTaskRepos:
		return a, "repos has at most 10 entries."
	}
	repos := make([]CreateTaskRepo, len(a.Repos))
	for i, r := range a.Repos {
		repos[i] = CreateTaskRepo{Repo: strings.TrimSpace(r.Repo), Base: strings.TrimSpace(r.Base)}
		if repos[i].Repo == "" {
			return a, "Every repos entry needs a repo."
		}
	}
	a.Repos = repos
	return a, ""
}

func (s *Server) addCreateTaskTool(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_task",
		Description: llmprompts.HeadlessToolCreateTask.Text(s.prompts, llmprompts.NoVars{}),
		InputSchema: llmprompts.RequireNonEmpty(llmprompts.InputSchema[CreateTaskArgs](s.prompts, llmprompts.HeadlessToolCreateTask.ID(), map[string]any{
			"repos": llmprompts.MaxVars{Max: maxTaskRepos},
			"brief": llmprompts.MaxVars{Max: MaxHandoverBytes},
		}), "title", "workflow", "brief"),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CreateTaskArgs) (*mcp.CallToolResult, CreateTaskResult, error) {
		call, bad := s.orchestratorCall(req)
		if bad != nil {
			return bad, CreateTaskResult{}, nil
		}
		args, msg := cleanCreateTask(args)
		if msg != "" {
			return toolError(msg), CreateTaskResult{}, nil
		}
		ctx, done := s.callCtx(ctx, req)
		defer done()
		res, err := s.orchestrator.SpawnTask(ctx, call, args)
		if err != nil {
			return fail("create_task", err), CreateTaskResult{}, nil
		}
		return nil, res, nil
	})
}

func (s *Server) addListTasksTool(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_tasks",
		Description: llmprompts.HeadlessToolListTasks.Text(s.prompts, llmprompts.NoVars{}),
		InputSchema: llmprompts.InputSchema[ListTasksArgs](s.prompts, llmprompts.HeadlessToolListTasks.ID(), nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ListTasksArgs) (*mcp.CallToolResult, ListTasksResult, error) {
		call, bad := s.orchestratorCall(req)
		if bad != nil {
			return bad, ListTasksResult{}, nil
		}
		switch args.State = strings.TrimSpace(args.State); args.State {
		case "", "live", "archived", "all":
		default:
			return toolError("state is live, archived or all."), ListTasksResult{}, nil
		}
		ctx, done := s.callCtx(ctx, req)
		defer done()
		res, err := s.orchestrator.ListTasks(ctx, call, args)
		if err != nil {
			return fail("list_tasks", err), ListTasksResult{}, nil
		}
		if res.Tasks == nil {
			res.Tasks = []ChildTaskInfo{}
		}
		return nil, res, nil
	})
}

func (s *Server) addTaskChainTool(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "task_chain",
		Description: llmprompts.HeadlessToolTaskChain.Text(s.prompts, llmprompts.NoVars{}),
		InputSchema: llmprompts.InputSchema[TaskChainArgs](s.prompts, llmprompts.HeadlessToolTaskChain.ID(), map[string]any{
			"limit": llmprompts.MaxVars{Max: maxChainLimit},
		}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args TaskChainArgs) (*mcp.CallToolResult, TaskChainResult, error) {
		call, bad := s.orchestratorCall(req)
		if bad != nil {
			return bad, TaskChainResult{}, nil
		}
		if args.Limit < 0 || args.Limit > maxChainLimit {
			return toolError("limit is 1 to 50."), TaskChainResult{}, nil
		}
		if args.Limit == 0 {
			args.Limit = defaultChainLimit
		}
		args.TaskID, args.Before = strings.TrimSpace(args.TaskID), strings.TrimSpace(args.Before)
		ctx, done := s.callCtx(ctx, req)
		defer done()
		res, err := s.orchestrator.TaskChain(ctx, call, args)
		if err != nil {
			return fail("task_chain", err), TaskChainResult{}, nil
		}
		if res.Rows == nil {
			res.Rows = []ChainRow{}
		}
		return nil, res, nil
	})
}
