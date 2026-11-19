// Package claude runs Kira Space's Claude Code integrations against the real `claude` CLI. Own Go
// module, so no default command (`go test ./...`, lint, CI) compiles or runs it. Every test spends
// real tokens (haiku, tiny prompts, a budget cap). See docs/DEV_ENVIRONMENT.md "Real `claude`
// tests" for the area-to-command table.
package claude

import (
	"fmt"
	"os"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

// TestMain runs flowharness.Main (askpass, js-worker and the fake gh re-executions), then reports the spend.
func TestMain(m *testing.M) {
	code := flowharness.Main(m)
	// Re-executed helper processes (fake gh) spend nothing and must keep stdout clean.
	spendMu.Lock()
	if spent > 0 {
		fmt.Fprintf(os.Stderr, "real claude spend (claude -p results only, TUI sessions not counted): %.4f USD\n", spent)
	}
	spendMu.Unlock()
	os.Exit(code)
}
