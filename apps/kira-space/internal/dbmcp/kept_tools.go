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
	// Put stores a value a script kept, after the response is final.
	Put(p keptvars.Put) (keptvars.Meta, error)
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
	Secret           bool           `json:"secret"`
	Source           keptListSource `json:"source"`
	ExpiresInSeconds int            `json:"expiresInSeconds"`
}

type keptListSource struct {
	Kind string `json:"kind"`
	MCP  string `json:"mcp"`
	Tool string `json:"tool"`
}

// listKeptVariables names what the caller can use. Never a value, a preview or the request that made it.
func (s *Server) listKeptVariables(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	list := s.cfg.Kept.List(keptScope(ctx))
	out := make([]keptListEntry, 0, len(list))
	for _, m := range list {
		out = append(out, keptListEntry{
			Name: m.Name, Type: m.Type, Items: m.Items, Secret: m.Secret,
			Source:           keptListSource{Kind: string(m.Source.Kind), MCP: m.Source.MCP, Tool: m.Source.Tool},
			ExpiresInSeconds: max(int(math.Ceil(time.Until(m.ExpiresAt).Seconds())), 0),
		})
	}
	return jsonResult(map[string]any{
		"variables": out,
		"note":      "Use a name as {{kept.name}} in api_request, or list it in params of run_query. Values are held by Kira and never shown to you.",
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
	// user marks a value the user supplied: the agent repeats the request and the user is asked again.
	user bool
}

func (e *keptGoneError) why() string {
	switch e.reason {
	case keptvars.ReasonCleared:
		return "was cleared by the user"
	case keptvars.ReasonReplaced:
		return "was replaced"
	case keptvars.ReasonEnded, keptvars.ReasonQuit:
		return "ended with the session"
	}
	return "expired"
}

func (e *keptGoneError) Error() string {
	if e.user {
		return fmt.Sprintf("kept variable %q %s while waiting for approval; nothing was sent. Repeat the request with the same inputs; the user will be asked again.", e.name, e.why())
	}
	return fmt.Sprintf("kept variable %q %s while waiting for approval; nothing was sent. Produce it again, then repeat the request.", e.name, e.why())
}

// queryText is the wording for run_query, where a statement ran or did not.
func (e *keptGoneError) queryText() string {
	return fmt.Sprintf("kept variable %q %s while waiting for approval; nothing ran. Produce it again, then repeat the query.", e.name, e.why())
}

// watchKept cancels the returned context when a kept variable of the plan leaves the store, so a card never
// offers Send for a value that is gone. stop must be called when the wait ends.
func (s *Server) watchKept(ctx context.Context, uses []agentapi.KeptUse) (context.Context, func()) {
	if len(uses) == 0 || s.cfg.Kept == nil {
		return ctx, func() {}
	}
	// A use with no id is a value the user has not supplied yet: nothing to watch.
	byID := make(map[string]agentapi.KeptUse, len(uses))
	ids := make([]string, 0, len(uses))
	for _, u := range uses {
		if u.ID != "" {
			byID[u.ID] = u
			ids = append(ids, u.ID)
		}
	}
	if len(ids) == 0 {
		return ctx, func() {}
	}
	ctx2, cancel := context.WithCancelCause(ctx)
	unwatch := s.cfg.Kept.OnRemove(ids, func(r keptvars.Removal) {
		u := byID[r.ID]
		cancel(&keptGoneError{name: u.Name, reason: r.Reason, user: u.Source.Kind == keptvars.SourceUser})
	})
	return ctx2, func() { unwatch(); cancel(nil) }
}
