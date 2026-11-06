package flowharness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Main is every flow package's TestMain body: os.Exit(flowharness.Main(m)). It captures the real
// Docker endpoint before any test swaps HOME, sweeps stale labelled resources, and points
// KIRA_HOME at a temp dir for the whole binary. The test binary is also the fake claude and the
// fake-mcp server (symlinked under those names), served before the test run starts.
func Main(m *testing.M) int {
	switch name := filepath.Base(os.Args[0]); name {
	case "claude", "fake-mcp":
		return fakeagent.Run(name, os.Args[1:])
	}
	dockerHost = resolveDockerHost()
	if dockerHost != "" {
		sweep(dockerHost)
	}
	return testx.RunWithTempHomes(m)
}
