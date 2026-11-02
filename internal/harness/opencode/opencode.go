// Package opencode is the OpenCode provider: the only package that knows the opencode CLI's flags,
// output formats, config documents and tool names. Every fact not yet confirmed on the real binary
// lives in facts.go, marked with its probe id (docs/v0.2.26/plans/P449.md).
package opencode

import (
	"os"

	"github.com/kirathecat/kira-studio/internal/harness"
)

// ID is the agent.provider value of this provider.
const ID = "opencode"

// defaultModel omits -m, so the user's own OpenCode model applies. Model ids are provider/model
// and depend on the user's logins, which a static catalog cannot list.
const defaultModel = "default"

// Catalog is the single "default" alias.
var Catalog = harness.Catalog{Aliases: []string{defaultModel}, Default: defaultModel}

var _ harness.Provider = (*Provider)(nil)

// Provider implements harness.Provider for OpenCode.
type Provider struct {
	// lookPath and stat are test seams for the CLI lookup; nil means exec.LookPath and os.Stat.
	lookPath func(string) (string, error)
	stat     func(string) (os.FileInfo, error)

	mcpList listCache
}

// New returns the OpenCode provider.
func New() *Provider { return &Provider{} }

func (*Provider) ID() string { return ID }

func (*Provider) Label() string { return "OpenCode" }

func (*Provider) Command() string { return "opencode" }

func (*Provider) ProductLabel() string { return "OpenCode" }

func (*Provider) HeadlessLabel() string { return "opencode run" }

func (*Provider) ModelLabel(alias string) string {
	if alias == defaultModel {
		return "OpenCode default model"
	}
	return alias
}

func (*Provider) Models() harness.Catalog { return Catalog }

// StageSettings: OpenCode takes none.
func (*Provider) StageSettings() []harness.StageSetting { return nil }

// CapsCost: OpenCode has no cost cap flag or config (U17).
func (*Provider) CapsCost() bool { return capsCost }
