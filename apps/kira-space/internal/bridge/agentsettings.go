package bridge

import "github.com/kirathecat/kira-studio/internal/agentcfg"

// ChannelAgentSettingsChanged is pushed, payload-free, when the agent settings file changes on disk or
// is saved.
const ChannelAgentSettingsChanged = "kira:agentSettings:changed"

// AgentSettingsService is Kira Space's own binding-name shim over agentcfg.Service.
type AgentSettingsService struct {
	*agentcfg.Service
}

// AgentSettingsChanged is the file watcher's target.
func AgentSettingsChanged(ev *Events) { ev.Broadcast(ChannelAgentSettingsChanged) }
