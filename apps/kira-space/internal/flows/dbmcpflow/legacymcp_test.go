package dbmcpflow

import (
	"slices"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P560: boot removes the retired user-scope registrations through the agent CLI, in the background.
func TestBootRemovesRetiredMcpRegistrations(t *testing.T) {
	app := flowharness.New(t)
	testx.WaitUntil(t, 10*time.Second, func() bool { return len(app.McpInstaller.RemoveCalls()) == 2 })
	got := app.McpInstaller.RemoveCalls()
	slices.Sort(got)
	if !slices.Equal(got, []string{"kira-ade", "kira-db"}) {
		t.Fatalf("removed %v, want kira-ade and kira-db", got)
	}
}
