package opencode

import (
	"context"
	"encoding/json"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// MCPInstaller shows the config snippet to add by hand: no non-interactive `mcp add` form is
// known (U11), and Kira never writes the user's config.
func (p *Provider) MCPInstaller() harness.MCPInstaller { return installer{p} }

type installer struct{ p *Provider }

func (i installer) Status() harness.InstallStatus {
	path, probed, _ := i.p.find()
	return harness.InstallStatus{Path: path, Probed: probed}
}

func (i installer) Install(context.Context, string, string, string) harness.InstallResult {
	return i.manual()
}

func (i installer) InstallStdio(context.Context, string, string, []string) harness.InstallResult {
	return i.manual()
}

func (i installer) manual() harness.InstallResult {
	_, probed, found := i.p.find()
	if !found {
		return harness.InstallResult{Outcome: harness.OutcomeNotFound, Probed: probed}
	}
	return harness.InstallResult{
		Outcome: harness.OutcomeInstallFailed, Probed: probed,
		Detail: i18n.T("backend.core.agent.opencode.manualInstall.body", nil),
	}
}

// Command is the `mcp` block to paste into the user's opencode.json. The token lives behind the
// helper; the snippet carries a placeholder, never a secret.
func (installer) Command(name, url, helperPath string) string {
	return snippet(name, map[string]any{
		"type": "remote", "url": url, "enabled": true,
		"headers": map[string]string{"Authorization": "Bearer <token printed by " + helperPath + ">"},
	})
}

func (installer) StdioCommand(name, command string, args []string) string {
	return snippet(name, map[string]any{"type": "local", "command": append([]string{command}, args...), "enabled": true})
}

func snippet(name string, entry map[string]any) string {
	b, _ := json.MarshalIndent(map[string]any{"mcp": map[string]any{name: entry}}, "", "  ")
	return string(b)
}
