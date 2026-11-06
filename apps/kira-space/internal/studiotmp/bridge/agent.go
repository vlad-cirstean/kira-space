package bridge

import "github.com/kirathecat/kira-studio/internal/harness/agentsvc"

// AgentService is this app's binding-name shim over internal/harness/agentsvc.Service.
type AgentService struct {
	*agentsvc.Service
}
