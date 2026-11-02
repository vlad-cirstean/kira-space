package flowharness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appwire"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Main is every flow package's TestMain body: os.Exit(flowharness.Main(m)). The test binary is also
// the fake claude, gh and kira-fake-agent (symlinked under those names), the askpass helper and memory-mcp /
// memory-embed, so those are served before the test run starts.
func Main(m *testing.M) int {
	if code, ok := appwire.RunArgvShim(os.Args); ok {
		return code
	}
	switch name := filepath.Base(os.Args[0]); name {
	case "claude", "opencode", "gh", "fake-mcp", fakeagent.Persona:
		return fakeagent.Run(name, os.Args[1:])
	}
	code := testx.RunWithTempHomes(m)
	removeTemplates()
	return code
}
