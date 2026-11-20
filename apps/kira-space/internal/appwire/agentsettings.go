package appwire

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/harness"
)

// agentProviderFrom reads agent.provider through the app's open settings handle.
func agentProviderFrom(r *repos.SettingsRepo) func() (string, error) {
	return func() (string, error) {
		s, err := r.GetAll()
		if err != nil {
			return "", err
		}
		return s.Agent.Provider, nil
	}
}

// agentSelector resolves the active provider from the app's settings.
func agentSelector(registry *harness.Registry, r *repos.SettingsRepo) harness.Selector {
	return harness.Selector{Registry: registry, ID: agentProviderFrom(r)}
}
