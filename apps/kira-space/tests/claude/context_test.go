package claude

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAgentContextHeadless checks the real claude receives the agent context through
// --append-system-prompt-file: a smart run answers with a marker only the context holds.
func TestAgentContextHeadless(t *testing.T) {
	app := newRealApp(t)
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	marker := "ctx-" + hex.EncodeToString(b)
	dir := app.ConfigDir("agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	text := "When asked for the context marker, answer " + marker + ".\n"
	if err := os.WriteFile(filepath.Join(dir, "context.md"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	run := smartRun(t, app, "What is the context marker? Reply with the marker only.", []string{"Read"})
	if run.State != "done" {
		t.Fatalf("run = %s, want done", run.State)
	}
	raw, _ := os.ReadFile(app.tee)
	if !strings.Contains(string(raw), marker) {
		t.Fatalf("reply lacks marker %s:\n%s", marker, raw)
	}
	app.requireCostUnder(t)
}
