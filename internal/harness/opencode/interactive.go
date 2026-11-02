package opencode

import (
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/loginshell"
)

// InteractiveCommand starts the TUI; a resume names the stored session with -s. The model is never
// passed: the TUI uses the user's own default. The MCP grant rides OPENCODE_CONFIG and extra
// directories and allowed tools ride OPENCODE_PERMISSION, the agent context rides
// OPENCODE_CONFIG_CONTENT as `instructions` next to the app's settings document, and the hook plugin rides OPENCODE_CONFIG_DIR (hooks
// Compose). OpenCode assigns a new session's id itself (FindSession).
func (p *Provider) InteractiveCommand(s harness.InteractiveSpec) harness.Launch {
	bin := s.Bin
	if bin == "" {
		bin = p.Command()
	}
	var b strings.Builder
	b.WriteString(bin)
	l := harness.Launch{}
	if s.Resume {
		b.WriteString(" -s " + loginshell.Quote(s.SessionID))
		l.SessionID = s.SessionID
	}
	l.Command = b.String()
	if s.MCPConfig != "" {
		l.Env = append(l.Env, "OPENCODE_CONFIG="+s.MCPConfig)
	}
	if perm := interactivePermission(s); perm != "" {
		l.Env = append(l.Env, "OPENCODE_PERMISSION="+perm)
	}
	if s.App != nil {
		l.Env = append(l.Env, s.App.Env...)
	}
	content, err := interactiveContent(s)
	if err != nil {
		slog.Warn("opencode: app settings not applied", "scope", "opencode", "err", err)
	}
	if content != "" {
		l.Env = append(l.Env, "OPENCODE_CONFIG_CONTENT="+content)
	}
	return l
}

// interactivePermission is the OPENCODE_PERMISSION document, "" when the launch needs none.
func interactivePermission(s harness.InteractiveSpec) string {
	doc := map[string]any{}
	if len(s.AddDirs) > 0 {
		dirs := ruleSet{}
		for _, d := range s.AddDirs {
			dirs.add(filepath.ToSlash(filepath.Clean(d))+"/**", "allow")
		}
		doc["external_directory"] = dirs
	}
	if interactiveNeedsPermission {
		rules, err := translateRules(s.AllowedTools, "allow")
		if err == nil {
			for k, v := range rules {
				doc[k] = v
			}
		}
	}
	if len(doc) == 0 {
		return ""
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return ""
	}
	return string(b)
}

// WithPrompt starts the TUI with its first prompt.
func (*Provider) WithPrompt(command, prompt string) string {
	return command + " --prompt " + loginshell.Quote(prompt)
}
