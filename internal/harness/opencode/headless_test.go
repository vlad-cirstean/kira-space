package opencode

import "testing"

// The sandbox read allow matches only `<run>-app.<role>.json` beside the grant.
func TestMergedMCPPathMatchesSandboxName(t *testing.T) {
	for _, last := range []string{"/c/run-1-3.mcp.json", "/c/run-1-app.mcp.json"} {
		if got, want := mergedMCPPath([]string{"/x.json", last}), "/c/run-1-app.merged.json"; got != want {
			t.Errorf("mergedMCPPath(%s) = %s, want %s", last, got, want)
		}
	}
}
