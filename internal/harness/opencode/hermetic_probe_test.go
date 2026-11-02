package opencode

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestHermeticDebugConfig checks the CLI fact the hermetic provider rests on (H-OC1): under the
// redirected config home `debug config` has no user model, mcp or plugin, and keeps project mcp.
// Opt-in: set KIRA_OPENCODE_BIN. No model call, no network.
func TestHermeticDebugConfig(t *testing.T) {
	bin := os.Getenv("KIRA_OPENCODE_BIN")
	if bin == "" {
		t.Skip("KIRA_OPENCODE_BIN not set")
	}
	home := t.TempDir()
	write := func(path, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(home, ".config", "opencode", "opencode.json"),
		`{"model":"anthropic/user-model","mcp":{"user-srv":{"type":"remote","url":"http://127.0.0.1:1/u"}},"plugin":["file:///tmp/user-plugin.js"]}`)
	write(filepath.Join(home, ".config", "git", "config"), "[user]\n\tname = t\n")
	repo := t.TempDir()
	write(filepath.Join(repo, "opencode.json"), `{"mcp":{"proj-srv":{"type":"local","command":["true"]}}}`)
	if err := exec.Command("git", "-C", repo, "init", "-q").Run(); err != nil {
		t.Fatal(err)
	}
	base := []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
	run := func(extra ...string) string {
		t.Helper()
		cmd := exec.Command(bin, "debug", "config")
		cmd.Dir = repo
		cmd.Env = append(append([]string{}, base...), extra...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("debug config: %v\n%s", err, out)
		}
		return string(out)
	}

	iso := t.TempDir()
	if err := (&Provider{}).Isolate(iso, base); err != nil {
		t.Fatal(err)
	}
	words := hermeticWords(isolatedConfigHome(iso), nil)
	hermetic := run(words.set...)
	for _, absent := range []string{"user-model", "user-srv", "user-plugin.js"} {
		if strings.Contains(hermetic, absent) {
			t.Errorf("hermetic config still has %q", absent)
		}
	}
	if !strings.Contains(hermetic, "proj-srv") {
		t.Error("hermetic config lost the project mcp server")
	}
	if control := run(); !strings.Contains(control, "user-srv") {
		t.Error("control without the redirect lost the user server: the probe proves nothing")
	}
}
