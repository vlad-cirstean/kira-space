package agentmcp

import (
	"context"
	"github.com/kirathecat/kira-studio/internal/mcpcall"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolCall identifies one tools/call for Options.OnToolCall. Args is the raw arguments JSON.
type ToolCall struct {
	Tool, RunID, TaskID string
	// SessionID is the Kira session record behind the call, "" for a headless run.
	SessionID string
	Args      []byte
}

// recordCalls reports every tools/call to Options.OnToolCall with its grant's run and task.
func (s *Server) recordCalls(srv *mcp.Server) {
	if s.onCall == nil {
		return
	}
	mcpcall.Record(srv, func(ctx context.Context, c mcpcall.Call) (context.Context, func(bool, string, error)) {
		var g Grant
		if c.Extra != nil {
			g, _ = s.grant(c.Extra.TokenInfo)
		}
		return s.onCall(ctx, ToolCall{Tool: c.Tool, RunID: g.RunID, TaskID: g.TaskID, SessionID: g.SessionID, Args: c.Args})
	})
}
