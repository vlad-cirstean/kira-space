package scriptruns

import (
	"context"
	"path/filepath"

	"github.com/kirathecat/kira-studio/internal/agentcfg"
)

// McpServers lists the MCP servers a smart run can tick: the app's agent settings when hermetic,
// else the agent CLI's own list from the app home.
func (s *Service) McpServers(ctx context.Context) (agentcfg.ServerList, error) {
	return s.Servers.List(ctx, s.Agent.Active(), s.Home)
}

// mcpDir is the folder a run's app MCP file is written to.
func (s *Service) mcpDir() string { return filepath.Join(s.Home, "automations-mcp") }
