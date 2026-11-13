package agentmcp

import (
	"context"
	"log/slog"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type progressKey struct{}

// progressReporter sends MCP progress notifications for one tool call.
type progressReporter struct {
	token   any
	session *mcp.ServerSession
	n       atomic.Int64
}

// withProgress returns ctx carrying a reporter when the client sent a progress token with the call;
// otherwise ctx unchanged. Stateless streamable HTTP delivers the notifications on that call's own
// response stream.
func withProgress(ctx context.Context, req *mcp.CallToolRequest) context.Context {
	if req == nil || req.Params == nil || req.Session == nil {
		return ctx
	}
	tok := req.Params.GetProgressToken()
	if tok == nil {
		return ctx
	}
	return context.WithValue(ctx, progressKey{}, &progressReporter{token: tok, session: req.Session})
}

// ReportProgress tells the caller of the running tool call how it is going. A no-op when the client
// asked for no progress; a delivery failure is logged at debug level and ignored.
func ReportProgress(ctx context.Context, msg string) {
	r, _ := ctx.Value(progressKey{}).(*progressReporter)
	if r == nil {
		return
	}
	err := r.session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
		ProgressToken: r.token,
		Progress:      float64(r.n.Add(1)),
		Message:       msg,
	})
	if err != nil {
		slog.Debug("agentmcp: progress notification", "scope", "ade", "err", err)
	}
}
