package agentmcp

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

// orchestrator.go: create_task, the tool only an orchestrator task carries (P571). Children never get
// it, so spawn depth is 1. Children are read through state.yaml and their folders.

// Bounds on create_task's input.
const (
	maxTaskTitleRunes = 200
	maxTaskRepos      = 10
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

// OrchestratorTools is what the orchestrator tools call; *ade.TaskBoard implements it. Each method acts for
// the calling task only. SpawnTask blocks until the user answers; ctx ends when the caller is gone.
type OrchestratorTools interface {
	SpawnTask(ctx context.Context, call SpaceCall, args CreateTaskArgs) (CreateTaskResult, error)
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
