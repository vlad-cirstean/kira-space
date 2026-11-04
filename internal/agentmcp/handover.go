package agentmcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

// MaxHandoverBytes bounds a handover body, whoever writes it.
const MaxHandoverBytes = 16 << 10

// HandoverRecorder stores a session's handover notes; *ade.TaskBoard implements it. stepID is the stage
// the session was opened for ("" when unknown); the recorder refuses a stage the task has left.
type HandoverRecorder interface {
	RecordHandover(ctx context.Context, taskID, stepID, sessionID, notes string, finished bool) error
}

type handoverArgs struct {
	Notes    string `json:"notes"`
	Finished bool   `json:"finished,omitempty"`
}

// handoverGranted reports whether the grant lists the handover tool: a task session without a run. A run
// grant reports through finish_step, whose summary is that run's handover.
func (s *Server) handoverGranted(g Grant) bool {
	return g.TaskID != "" && g.RunID == "" && s.handover != nil
}

func (s *Server) addHandoverTool(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "handover",
		Description: llmprompts.HeadlessToolHandover.Text(s.prompts, llmprompts.NoVars{}),
		InputSchema: llmprompts.RequireNonEmpty(llmprompts.InputSchema[handoverArgs](s.prompts, llmprompts.HeadlessToolHandover.ID(), map[string]any{
			"notes": llmprompts.MaxVars{Max: MaxHandoverBytes},
		}), "notes"),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args handoverArgs) (*mcp.CallToolResult, any, error) {
		var g Grant
		var ok bool
		if req.Extra != nil {
			g, ok = s.grant(req.Extra.TokenInfo)
		}
		if !ok || !s.handoverGranted(g) {
			return toolError("handover is not available for this session."), nil, nil
		}
		if strings.TrimSpace(args.Notes) == "" {
			return toolError("notes must not be empty."), nil, nil
		}
		if len(args.Notes) > MaxHandoverBytes {
			return toolError(fmt.Sprintf("notes is too long: at most %d bytes.", MaxHandoverBytes)), nil, nil
		}
		if err := s.handover.RecordHandover(ctx, g.TaskID, g.StepID, g.SessionID, args.Notes, args.Finished); err != nil {
			return fail("handover", err), nil, nil
		}
		ack := llmprompts.HeadlessHandoverAck.Text(s.prompts, llmprompts.NoVars{})
		if !args.Finished {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: ack}}}, nil, nil
		}
		return &mcp.CallToolResult{Content: s.finishContent(ctx, req, ack, g)}, nil, nil
	})
}
