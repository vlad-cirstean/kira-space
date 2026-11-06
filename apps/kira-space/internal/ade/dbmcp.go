package ade

import (
	"fmt"
	"slices"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/dbmcp"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/harness"
)

// DbMcpSessions mints the per-session Kira DB server entry; *dbmcp.SessionPool implements it.
type DbMcpSessions interface {
	Grant(dbmcp.SessionIdentity) (harness.MCPServer, func(), error)
}

type dbMcpHolder struct{ sessions DbMcpSessions }

// SetDbMcp binds the session pool. The board is built before the Studio stage, so appwire calls this
// once that stage exists.
func (b *TaskBoard) SetDbMcp(s DbMcpSessions) { b.dbMcp.Store(&dbMcpHolder{sessions: s}) }

// grantDbMcp adds the Kira DB tools to g when the step asks for them. The returned release is never nil.
func (b *TaskBoard) grantDbMcp(def stepDef, id dbmcp.SessionIdentity, g *agentmcp.Grant) (release func(), err error) {
	if !def.DbMcp {
		return func() {}, nil
	}
	h := b.dbMcp.Load()
	if h == nil || h.sessions == nil {
		return nil, fmt.Errorf("Kira DB tools could not start: the database server is not available")
	}
	id.StepID = def.ID
	entry, release, err := h.sessions.Grant(id)
	if err != nil {
		return nil, fmt.Errorf("Kira DB tools could not start: %w", err)
	}
	g.Extra = append(slices.Clone(g.Extra), entry)
	return release, nil
}

// withDbMcpTool adds the session server's allow rule to tools for a step with db_mcp.
func withDbMcpTool(def stepDef, tools []string) []string {
	if !def.DbMcp || slices.Contains(tools, dbmcp.SessionAllowTool) {
		return tools
	}
	return append(slices.Clone(tools), dbmcp.SessionAllowTool)
}

// joinRelease runs a, then b, once.
func joinRelease(a, b func()) func() {
	return func() { a(); b() }
}

// tuiStepDef is the workflow step a TUI session opens for; the zero value when it has none.
func tuiStepDef(tc *taskCtx, stepID string) stepDef {
	if tc.wf == nil || stepID == "" {
		return stepDef{}
	}
	for _, d := range workflowSteps(*tc.wf) {
		if d.ID == stepID {
			return d
		}
	}
	return stepDef{}
}
