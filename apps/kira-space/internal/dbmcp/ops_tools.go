package dbmcp

import (
	"context"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OpsControl is dbmcp's consumer-declared view of *adapterhost.Router for the operation tools.
type OpsControl interface {
	RunningOps() []adapterhost.RunningOp
	CancelRunning(ctx context.Context, opID string) (bool, error)
}

// LiveRun is a running or waiting script run.
type LiveRun struct {
	ID, ScriptName, State string
	// Trigger is what started the run; "mcp" for an agent's.
	Trigger   string
	StartedAt *int64
}

// ScriptRunControl lists and stops runs of MCP-exposed scripts, which the Operations panel shows as run:<id> rows.
type ScriptRunControl interface {
	Live() ([]LiveRun, error)
	Stop(id string) error
}

const scriptRunPrefix = "run:"

type operationsArgs struct {
	Cancel string `json:"cancel,omitempty"`
}

type opEntry struct {
	OpID           string `json:"opId"`
	Type           string `json:"type"`
	Kind           string `json:"kind"`
	Source         string `json:"source"`
	StartedAt      string `json:"startedAt"`
	ElapsedMs      int64  `json:"elapsedMs"`
	ConnectionID   string `json:"connectionId,omitempty"`
	ConnectionName string `json:"connectionName,omitempty"`
	Name           string `json:"name,omitempty"`
	State          string `json:"state,omitempty"`
}

// exposure memoises resolveEnabled per connection id for one tool call.
type exposure struct {
	s    *Server
	seen map[string]string
	ok   map[string]bool
}

func (s *Server) newExposure() *exposure {
	return &exposure{s: s, seen: map[string]string{}, ok: map[string]bool{}}
}

// check returns the connection name when id is exposed to MCP.
func (e *exposure) check(id string) (string, bool) {
	if v, done := e.ok[id]; done {
		return e.seen[id], v
	}
	c, err := e.s.resolveEnabled(id)
	e.ok[id] = err == nil
	e.seen[id] = c.Name
	return c.Name, err == nil
}

func notFoundResult(opID string) (*mcp.CallToolResult, any, error) {
	return errResult("no running operation with id \"" + opID + "\"; it may have finished. Call operations for current ids.")
}

func (s *Server) listOperations() (*mcp.CallToolResult, any, error) {
	now := time.Now()
	ex := s.newExposure()
	out := make([]opEntry, 0)
	for _, op := range s.cfg.Ops.RunningOps() {
		e := opEntry{OpID: op.OpID, Type: "op", Kind: op.Kind, Source: op.Source, StartedAt: op.StartedAt}
		if t, err := time.Parse(time.RFC3339Nano, op.StartedAt); err == nil {
			e.ElapsedMs = now.Sub(t).Milliseconds()
		}
		if op.ConnectionID != nil {
			name, ok := ex.check(*op.ConnectionID)
			if !ok {
				continue
			}
			e.ConnectionID, e.ConnectionName = *op.ConnectionID, name
		}
		out = append(out, e)
	}
	runs, err := s.liveRuns()
	if err != nil {
		return nil, nil, err
	}
	for _, r := range runs {
		source := "user"
		if r.Trigger == "mcp" {
			source = "mcp"
		}
		e := opEntry{OpID: scriptRunPrefix + r.ID, Type: "scriptRun", Kind: "script", Source: source, Name: r.ScriptName, State: r.State}
		if r.StartedAt != nil {
			t := time.UnixMilli(*r.StartedAt)
			e.StartedAt, e.ElapsedMs = t.UTC().Format(time.RFC3339Nano), now.Sub(t).Milliseconds()
		}
		out = append(out, e)
	}
	return jsonResult(map[string]any{"operations": out})
}

func (s *Server) liveRuns() ([]LiveRun, error) {
	if s.cfg.Runs == nil {
		return nil, nil
	}
	return s.cfg.Runs.Live()
}

// operations lists the running operations, or with cancel stops one by id.
func (s *Server) operations(ctx context.Context, _ *mcp.CallToolRequest, a operationsArgs) (*mcp.CallToolResult, any, error) {
	if a.Cancel == "" {
		return s.listOperations()
	}
	return s.cancelOperation(ctx, a.Cancel)
}

func (s *Server) cancelOperation(ctx context.Context, opID string) (*mcp.CallToolResult, any, error) {
	if id, ok := strings.CutPrefix(opID, scriptRunPrefix); ok {
		return s.cancelScriptRun(opID, id)
	}
	ex := s.newExposure()
	for _, op := range s.cfg.Ops.RunningOps() {
		if op.OpID != opID {
			continue
		}
		if op.ConnectionID != nil {
			if _, ok := ex.check(*op.ConnectionID); !ok {
				break
			}
		}
		done, err := s.cfg.Ops.CancelRunning(ctx, opID)
		if err != nil {
			return nil, nil, err
		}
		if !done {
			break
		}
		return jsonResult(map[string]any{"opId": opID, "type": "op", "kind": op.Kind, "cancelled": true})
	}
	return notFoundResult(opID)
}

func (s *Server) cancelScriptRun(opID, runID string) (*mcp.CallToolResult, any, error) {
	runs, err := s.liveRuns()
	if err != nil {
		return nil, nil, err
	}
	for _, r := range runs {
		if r.ID != runID {
			continue
		}
		if err := s.cfg.Runs.Stop(runID); err != nil {
			return nil, nil, err
		}
		return jsonResult(map[string]any{"opId": opID, "type": "scriptRun", "stopped": true})
	}
	return notFoundResult(opID)
}

// addOpsTools registers operations.
func (s *Server) addOpsTools(srv *mcp.Server, src llmprompts.Source) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "operations",
		Description: llmprompts.DbmcpToolOperations.Text(src, llmprompts.NoVars{}),
		InputSchema: llmprompts.InputSchema[operationsArgs](src, llmprompts.DbmcpToolOperations.ID(), nil),
	}, withPanicRecovery(s, "operations", s.operations))
}
