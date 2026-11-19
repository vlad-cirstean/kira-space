package dbmcp

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentapi"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// KeptStore is dbmcp's view of the kept variable store: *keptvars.Store.
type KeptStore interface {
	agentapi.KeptReader
	List(scope string) []keptvars.Meta
	// OnRemove calls fn once, with the first removal among ids.
	OnRemove(ids []string, fn func(keptvars.Removal)) (stop func())
}

// externalKeptScope is the one scope of the external server: its transport is stateless, so it has no
// client identity to scope by.
const externalKeptScope = "external"

// KeptScopeOf is the kept-variable scope of an agent session.
func KeptScopeOf(sessionID string) string { return "session:" + sessionID }

// keptScope is the scope of the call in ctx: the agent session's, or the external one.
func keptScope(ctx context.Context) string {
	if id := originFrom(ctx).SessionID; id != "" {
		return KeptScopeOf(id)
	}
	return externalKeptScope
}

type keptListEntry struct {
	Name             string         `json:"name"`
	Type             string         `json:"type"`
	Items            int            `json:"items"`
	Source           keptListSource `json:"source"`
	ExpiresInSeconds int            `json:"expiresInSeconds"`
}

type keptListSource struct {
	MCP  string `json:"mcp"`
	Tool string `json:"tool"`
}

// listKeptVariables names what the caller can use. Never a value, a preview or the request that made it.
func (s *Server) listKeptVariables(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	list := s.cfg.Kept.List(keptScope(ctx))
	out := make([]keptListEntry, 0, len(list))
	for _, m := range list {
		out = append(out, keptListEntry{
			Name: m.Name, Type: m.Type, Items: m.Items, Source: keptListSource{MCP: m.Source.MCP, Tool: m.Source.Tool},
			ExpiresInSeconds: max(int(math.Ceil(time.Until(m.ExpiresAt).Seconds())), 0),
		})
	}
	return jsonResult(map[string]any{
		"variables": out,
		"note":      "Use a name as {{kept.name}} in api_request. Values are held by Kira and never shown to you.",
	})
}

func (s *Server) addKeptTools(srv *mcp.Server, src llmprompts.Source) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "list_kept_variables", Description: llmprompts.DbmcpToolListKept.Text(src, llmprompts.NoVars{}),
	}, withPanicRecovery(s, "list_kept_variables", s.listKeptVariables))
}

// keptGoneError ends an approval wait: a kept variable the request uses left the store.
type keptGoneError struct {
	name   string
	reason keptvars.Reason
}

func (e *keptGoneError) Error() string {
	why := "expired"
	switch e.reason {
	case keptvars.ReasonCleared:
		why = "was cleared by the user"
	case keptvars.ReasonReplaced:
		why = "was replaced"
	case keptvars.ReasonEnded, keptvars.ReasonQuit:
		why = "ended with the session"
	}
	return fmt.Sprintf("kept variable %q %s while waiting for approval; nothing was sent. Produce it again, then repeat the request.", e.name, why)
}

// watchKept cancels the returned context when a kept variable of the plan leaves the store, so a card never
// offers Send for a value that is gone. stop must be called when the wait ends.
func (s *Server) watchKept(ctx context.Context, uses []agentapi.KeptUse) (context.Context, func()) {
	if len(uses) == 0 || s.cfg.Kept == nil {
		return ctx, func() {}
	}
	ctx2, cancel := context.WithCancelCause(ctx)
	names := make(map[string]string, len(uses))
	ids := make([]string, len(uses))
	for i, u := range uses {
		names[u.ID], ids[i] = u.Name, u.ID
	}
	unwatch := s.cfg.Kept.OnRemove(ids, func(r keptvars.Removal) {
		cancel(&keptGoneError{name: names[r.ID], reason: r.Reason})
	})
	return ctx2, func() { unwatch(); cancel(nil) }
}
