package ade

import (
	"fmt"
	"slices"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/dbmcp"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/harness"
)

// DbMcpSessions mints the per-session Kira Studio server entry; *dbmcp.SessionPool implements it.
type DbMcpSessions interface {
	Grant(dbmcp.SessionIdentity) (harness.MCPServer, func(), error)
}

// grantDbMcp adds the Kira Studio tools to g when the step asks for them. The returned release is never nil.
func (b *TaskBoard) grantDbMcp(def stepDef, id dbmcp.SessionIdentity, g *agentmcp.Grant) (release func(), err error) {
	if !def.DbMcp {
		return func() {}, nil
	}
	if b.deps.DbMcp == nil {
		return nil, fmt.Errorf("Kira Studio tools could not start: the database server is not available")
	}
	id.StepID = def.ID
	entry, release, err := b.deps.DbMcp.Grant(id)
	if err != nil {
		return nil, fmt.Errorf("Kira Studio tools could not start: %w", err)
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
