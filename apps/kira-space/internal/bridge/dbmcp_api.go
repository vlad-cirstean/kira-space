package bridge

import (
	"errors"
	"log/slog"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentapi"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apiaccess"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/dbmcp"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
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

// AgentAPIAccess reads the user's API access rules, per plan. It fails closed: an unreadable settings store or
// unreadable stored rules refuse every request instead of falling back to allow-all.
func AgentAPIAccess(settings *repos.SettingsRepo) func() (apiaccess.Policy, error) {
	return func() (apiaccess.Policy, error) {
		all, err := settings.GetAll()
		if err != nil {
			slog.Warn("db mcp: read API access rules, refusing API requests", "scope", "dbmcp", "err", err)
			return apiaccess.Policy{}, errors.New("the API settings could not be read")
		}
		if all.DbMcp.APIRulesInvalid {
			return apiaccess.Policy{}, errors.New("the API access rules in Kira Space's database MCP settings could not be read; ask the user to replace them")
		}
		return apiaccess.Compile(all.DbMcp.APIRules)
	}
}
