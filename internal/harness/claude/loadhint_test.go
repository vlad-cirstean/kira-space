package claude

import (
	"testing"

	"github.com/kirathecat/kira-studio/internal/agentmcp"
)

// The Space prompt's load hint must name tools exactly as claude lists them.
func TestLoadHintNamesTools(t *testing.T) {
	p := New()
	got := p.LoadHint(agentmcp.State, agentmcp.CreateTask)
	want := `Load the tools first: ToolSearch with query "select:mcp__kira-space__state,mcp__kira-space__create_task".`
	if got != want {
		t.Fatalf("LoadHint = %q, want %q", got, want)
	}
	if p.LoadHint() != "" {
		t.Fatal("LoadHint with no tools is not empty")
	}
}
