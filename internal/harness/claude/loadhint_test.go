package claude

import (
	"testing"

	"github.com/kirathecat/kira-studio/internal/agentmcp"
)

// The Space prompt's load hint must name tools exactly as claude lists them.
func TestLoadHintNamesSubmit(t *testing.T) {
	p := New()
	got := p.LoadHint(agentmcp.SpaceToolIDs[5], agentmcp.SpaceToolIDs[0])
	want := `Load the tools first: ToolSearch with query "select:mcp__kira-space__submit,mcp__kira-space__task_info".`
	if got != want {
		t.Fatalf("LoadHint = %q, want %q", got, want)
	}
	if p.LoadHint() != "" {
		t.Fatal("LoadHint with no tools is not empty")
	}
}
