// Package claude is the Claude Code provider: the only package that knows the claude CLI's flags,
// output formats, config files and model names.
package claude

import (
	"os"
	"strings"

	"github.com/kirathecat/kira-studio/internal/harness"
)

// ID is the agent.provider value of this provider.
const ID = "claude"

// Catalog lists the model aliases the claude CLI resolves to the current model of that family.
var Catalog = harness.Catalog{Aliases: []string{"haiku", "sonnet", "opus"}, Default: "sonnet"}

// Provider implements harness.Provider for Claude Code.
type Provider struct {
	// lookPath and stat are test seams for the CLI lookup; nil means exec.LookPath and os.Stat.
	lookPath func(string) (string, error)
	stat     func(string) (os.FileInfo, error)

	mcpList listCache
}

// New returns the Claude Code provider.
func New() *Provider { return &Provider{} }

func (*Provider) ID() string { return ID }

func (*Provider) Label() string { return "Claude" }

// appendSystemPromptFileFlag is a hidden claude flag (absent from `claude --help`, named in the
// `--bare` help). A CLI that drops it fails the launch with "unknown option", never silently;
// tests/claude TestAgentContextHeadless guards it.
const appendSystemPromptFileFlag = "--append-system-prompt-file"

// KeepsContextOnResume: --system-prompt-snapshot is on by default, so a resumed conversation keeps
// its recorded prompt until it compacts.
func (*Provider) KeepsContextOnResume() bool { return true }

func (*Provider) Command() string { return "claude" }

func (*Provider) ProductLabel() string { return "Claude Code" }

func (*Provider) HeadlessLabel() string { return "claude -p" }

func (*Provider) ModelLabel(alias string) string {
	if alias == "" {
		return alias
	}
	return strings.ToUpper(alias[:1]) + alias[1:]
}

func (*Provider) Models() harness.Catalog { return Catalog }
