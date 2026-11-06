// Package claude runs Kira Studio's Claude Code integrations against the real `claude` CLI. Own Go
// module, so no default command (`go test ./...`, lint, CI) compiles or runs it. Every test spends
// real tokens (haiku, tiny prompts, a budget cap). See docs/DEV_ENVIRONMENT.md "Real `claude`
// tests" for the area-to-command table.
package claude

import (
	"os"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
)

func TestMain(m *testing.M) { os.Exit(flowharness.Main(m)) }
