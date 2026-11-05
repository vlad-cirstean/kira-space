package bridge

import (
	"log/slog"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/agentapi"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/dbmcp"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/repos"
)

// apiSettings is dbmcp.Config.APISettings' backend, read fresh on every call. A read failure fails
// closed: the tools are not listed, and approval stays on.
func (s *DbMcpService) apiSettings() dbmcp.APISettings {
	settings, err := s.Deps.Repos.Settings.GetAll()
	if err != nil {
		slog.Warn("db mcp: read API settings, turning the API tools off", "scope", "dbmcp", "err", err)
		return dbmcp.APISettings{Approval: "always"}
	}
	return dbmcp.APISettings{Enabled: settings.DbMcp.APIRequests, Approval: settings.DbMcp.APIApproval}
}

// AgentAPIGlobals reads the user's API settings an agent request inherits, per send.
func AgentAPIGlobals(settings *repos.SettingsRepo) func() (agentapi.Globals, error) {
	return func() (agentapi.Globals, error) {
		all, err := settings.GetAll()
		if err != nil {
			return agentapi.Globals{}, err
		}
		return agentapi.Globals{HTTPVersion: all.Api.HTTPVersion, MaxResponseMb: all.Api.MaxResponseMb, MaxRedirects: all.Api.MaxRedirects}, nil
	}
}
