package bridge

import (
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/prompts"
)

// PromptsService is this app's binding-name shim over prompts.Bound.
type PromptsService struct {
	*prompts.Bound
}

// ChannelPromptsChanged broadcasts the full routed prompt list; ChannelPromptsReveal asks one window
// to show a prompt first.
const (
	ChannelPromptsChanged = appevent.ChannelPromptsChanged
	ChannelPromptsReveal  = appevent.ChannelPromptsReveal
)
