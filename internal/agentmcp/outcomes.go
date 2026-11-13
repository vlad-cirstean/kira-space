package agentmcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

const maxOutcomeRuns = 10

// OutcomeQuery filters the runs run_outcome returns. Kind is "", "rebase", "step" or "automation".
type OutcomeQuery struct {
	RunID  string
	Branch string
	Kind   string
	Limit  int
}

// RunOutcomeEntry is one run as an agent sees it. Outcome is the stored outcome as JSON, null while
// the run has none.
type RunOutcomeEntry struct {
	RunID      string `json:"runId"`
	Kind       string `json:"kind"`
	Step       string `json:"step"`
	Repo       string `json:"repo"`
	Branch     string `json:"branch"`
	State      string `json:"state"`
	FinishedAt *int64 `json:"finishedAt"`
	Outcome    any    `json:"outcome"`
}

// Outcomes is what run_outcome calls; *ade.TaskBoard implements it. It returns the task's runs only,
// newest first.
type Outcomes interface {
	RunOutcomes(ctx context.Context, taskID string, q OutcomeQuery) ([]RunOutcomeEntry, error)
}

type runOutcomeArgs struct {
	RunID  string `json:"runId,omitempty"`
	Branch string `json:"branch,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type runOutcomeResult struct {
	Runs []RunOutcomeEntry `json:"runs"`
}

func (s *Server) addOutcomeTool(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "run_outcome",
		Description: llmprompts.HeadlessToolRunOutcome.Text(s.prompts, llmprompts.NoVars{}),
		InputSchema: llmprompts.InputSchema[runOutcomeArgs](s.prompts, llmprompts.HeadlessToolRunOutcome.ID(), map[string]any{
			"limit": llmprompts.MaxVars{Max: maxOutcomeRuns},
		}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args runOutcomeArgs) (*mcp.CallToolResult, runOutcomeResult, error) {
		var g Grant
		var ok bool
		if req.Extra != nil {
			g, ok = s.grant(req.Extra.TokenInfo)
		}
		if !ok || g.TaskID == "" {
			return toolError("run_outcome is not available for this session."), runOutcomeResult{}, nil
		}
		switch args.Kind {
		case "", "rebase", "step", "automation":
		default:
			return toolError(`kind must be "rebase", "step" or "automation"`), runOutcomeResult{}, nil
		}
		if args.Limit < 0 || args.Limit > maxOutcomeRuns {
			return toolError("limit must be 1 to 10."), runOutcomeResult{}, nil
		}
		if args.Limit == 0 {
			args.Limit = 3
		}
		ctx, done := s.callCtx(ctx, req)
		defer done()
		runs, err := s.outcomes.RunOutcomes(ctx, g.TaskID, OutcomeQuery{RunID: args.RunID, Branch: args.Branch, Kind: args.Kind, Limit: args.Limit})
		if err != nil {
			return fail("run_outcome", err), runOutcomeResult{}, nil
		}
		if runs == nil {
			runs = []RunOutcomeEntry{}
		}
		return nil, runOutcomeResult{Runs: runs}, nil
	})
}
