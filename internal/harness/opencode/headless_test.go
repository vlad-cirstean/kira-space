package opencode

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/internal/harness"
)

// The sandbox read allow matches only `<run>-app.<role>.json` beside the grant.
func TestMergedMCPPathMatchesSandboxName(t *testing.T) {
	for _, last := range []string{"/c/run-1-3.mcp.json", "/c/run-1-app.mcp.json"} {
		if got, want := mergedMCPPath([]string{"/x.json", last}), "/c/run-1-app.merged.json"; got != want {
			t.Errorf("mergedMCPPath(%s) = %s, want %s", last, got, want)
		}
	}
}

func TestInteractiveSandbox(t *testing.T) {
	sb := &harness.SandboxExec{Profile: "/p/profile.sb", Env: []string{"TMPDIR=/s/tmp"}, Unset: []string{"SSH_AUTH_SOCK"}}
	l := (&Provider{}).InteractiveCommand(harness.InteractiveSpec{Bin: "opencode", Sandbox: sb, DisallowedTools: []string{"WebFetch"}})
	if want := "/usr/bin/env -u 'SSH_AUTH_SOCK' 'TMPDIR=/s/tmp' '/usr/bin/sandbox-exec' -f '/p/profile.sb' opencode"; l.Command != want {
		t.Fatalf("got %s want %s", l.Command, want)
	}
	perm := ""
	for _, kv := range l.Env {
		if v, ok := strings.CutPrefix(kv, "OPENCODE_PERMISSION="); ok {
			perm = v
		}
	}
	if !strings.Contains(perm, "webfetch") || !strings.Contains(perm, "deny") {
		t.Fatalf("permission %q lacks the webfetch denial", perm)
	}
}
