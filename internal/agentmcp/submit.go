package agentmcp

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

// MaxHandoverBytes bounds a handover body, whoever writes it.
const MaxHandoverBytes = 16 << 10

// maxSubmitPaths bounds the paths of one submit call.
const maxSubmitPaths = 50

// SubmitArgs is the submit tool's input: the files to read (all when none) and why, for a change that
// needs the user's approval. It is content only; state step.finish ends the step.
type SubmitArgs struct {
	Paths  []string `json:"paths,omitempty"`
	Reason string   `json:"reason,omitempty"`
}

// SubmitFinish is step.finish's implicit submit: the reply to append, or the refusal of an ok finish.
type SubmitFinish struct {
	Reply  string
	Refuse string
	// Summary is the step's draft handover: the run records it as its summary.
	Summary string
}

func (s *Server) addSubmitTool(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "submit",
		Description: llmprompts.HeadlessToolSubmit.Text(s.prompts, llmprompts.NoVars{}),
		InputSchema: llmprompts.InputSchema[SubmitArgs](s.prompts, llmprompts.HeadlessToolSubmit.ID(), map[string]any{
			"paths": llmprompts.MaxVars{Max: maxSubmitPaths},
		}),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SubmitArgs) (*mcp.CallToolResult, any, error) {
		call, bad := s.spaceTask(req)
		if bad != nil {
			return bad, nil, nil
		}
		if len(args.Paths) > maxSubmitPaths {
			return toolError("paths has too many entries."), nil, nil
		}
		args.Reason = strings.TrimSpace(args.Reason)
		ctx, done := s.callCtx(ctx, req)
		defer done()
		text, err := s.space.Submit(ctx, call, args)
		if err != nil {
			return fail("submit", err), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
	})
}
