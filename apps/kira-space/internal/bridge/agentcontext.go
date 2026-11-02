package bridge

import "github.com/kirathecat/kira-studio/internal/agentctx"

// ChannelAgentContextChanged is pushed, payload-free, when the agent context file changes on disk or
// is saved.
const ChannelAgentContextChanged = "kira:agentContext:changed"

// AgentContextService is Kira Space's own binding-name shim over agentctx.Service.
type AgentContextService struct {
	*agentctx.Service
}

// AgentContextChanged is the file watcher's target.
func AgentContextChanged(ev *Events) { ev.Broadcast(ChannelAgentContextChanged) }
