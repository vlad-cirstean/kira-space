package bridge

import (
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

// LlmPromptsService is this app's binding-name shim over llmprompts.Bound.
type LlmPromptsService struct {
	*llmprompts.Bound
}

// ChannelLlmPromptsChanged broadcasts {revision} after the overrides file changed.
const ChannelLlmPromptsChanged = appevent.ChannelLlmPromptsChanged
