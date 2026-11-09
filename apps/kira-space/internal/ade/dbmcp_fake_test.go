package ade

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/dbmcp"
	"github.com/kirathecat/kira-studio/internal/harness"
)

// fakeDbMcp stands in for the session pool: db_mcp is on by default, so a board without one fails
// every launch.
type fakeDbMcp struct{}

func (fakeDbMcp) Grant(dbmcp.SessionIdentity) (harness.MCPServer, func(), error) {
	return harness.MCPServer{Name: "kira-studio-session", URL: "http://127.0.0.1:1/mcp"}, func() {}, nil
}
