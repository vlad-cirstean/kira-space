package mcpinstall

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/loginshell"
)

// TestQuoteRoundTrips guards the one property Command/Install's own headersHelper value
// depends on (F10): wrapping in single quotes must survive a POSIX shell splitting the resulting
// string on whitespace, including a path that itself contains a single quote.
func TestQuoteRoundTrips(t *testing.T) {
	cases := []string{
		"/plain/path/mcp-header-helper.sh",
		"/Users/vlad cirstean/.kira/mcp-header-helper.sh",
		"/tmp/weird's path/mcp-header-helper.sh",
	}
	for _, in := range cases {
		quoted := loginshell.Quote(in)
		if !strings.HasPrefix(quoted, "'") || !strings.HasSuffix(quoted, "'") {
			t.Fatalf("loginshell.Quote(%q) = %q, want a leading and trailing single quote", in, quoted)
		}
	}
}

// TestCommandRoundTripsThroughShell guards F10/P168 Part 6 F1: the copy-paste text, run through a
// real shell, must hand `claude` the same argv Install passes, including for a helper path with a
// space or a single quote.
func TestCommandRoundTripsThroughShell(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	for _, helperPath := range []string{
		"/Users/a/.kira/mcp-header-helper.sh",
		"/Users/a b/.kira/mcp-header-helper.sh",
		"/Users/o'n/.kira/mcp-header-helper.sh",
	} {
		cmd := New(Deps{}).Command("kira-studio", "http://127.0.0.1:8766/mcp", helperPath)
		script := "claude() { for a; do printf '%s\\n' \"$a\"; done; }\n" + cmd
		out, err := exec.Command("sh", "-c", script).Output()
		if err != nil {
			t.Fatalf("sh rejected %q: %v", cmd, err)
		}
		payload, _ := json.Marshal(httpServer("http://127.0.0.1:8766/mcp", helperPath))
		want := "mcp\nremove\n--scope\nuser\nkira-studio\nmcp\nadd-json\n--scope\nuser\nkira-studio\n" + string(payload) + "\n"
		if string(out) != want {
			t.Errorf("helper %q: shell argv = %q, want %q", helperPath, out, want)
		}
	}
}

// TestInstallRefusesRelativeHeaderHelperPath: a relative helperPath resolves against
// Claude Code's own cwd, not Kira's (reachable when os.UserHomeDir fails and kirapaths.Home falls
// back to a relative path) — Install must refuse outright rather than register a headersHelper
// that silently reads the wrong file.
func TestInstallRefusesRelativeHeaderHelperPath(t *testing.T) {
	ran := false
	installer := New(Deps{
		LookPath: func(string) (string, error) { return "/usr/local/bin/claude", nil },
		Stat:     func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		Run: func(context.Context, string, []string) error {
			ran = true
			return nil
		},
	})

	res := installer.Install(context.Background(), "kira-studio", "http://127.0.0.1:8766/mcp", "relative/path.sh")
	if res.Outcome != harness.OutcomeInstallFailed {
		t.Fatalf("Outcome = %q, want %q", res.Outcome, harness.OutcomeInstallFailed)
	}
	if ran {
		t.Fatal("Install spawned claude despite a relative helper path — must refuse before any spawn")
	}
}

// TestInstallQuotesHeaderHelperPathInPayload confirms Install sends the same quoted form Command
// displays, over a captured add-json payload.
func TestInstallQuotesHeaderHelperPathInPayload(t *testing.T) {
	helperPath := "/Users/vlad cirstean/.kira/mcp-header-helper.sh"
	var addPayload string
	installer := New(Deps{
		LookPath: func(string) (string, error) { return "/usr/local/bin/claude", nil },
		Run: func(_ context.Context, _ string, args []string) error {
			if len(args) > 0 && args[0] == "mcp" && len(args) > 1 && args[1] == "add-json" {
				addPayload = args[len(args)-1]
			}
			return nil
		},
	})

	res := installer.Install(context.Background(), "kira-studio", "http://127.0.0.1:8766/mcp", helperPath)
	if res.Outcome != harness.OutcomeInstalled {
		t.Fatalf("Outcome = %q, want %q (detail: %s)", res.Outcome, harness.OutcomeInstalled, res.Detail)
	}
	var decoded serverJSON
	if err := json.Unmarshal([]byte(addPayload), &decoded); err != nil {
		t.Fatalf("captured add-json payload is not valid JSON: %v (%s)", err, addPayload)
	}
	want := loginshell.Quote(helperPath)
	if decoded.HeadersHelper != want {
		t.Fatalf("HeadersHelper = %q, want %q (single-quoted)", decoded.HeadersHelper, want)
	}
}
