package opencode

import (
	"cmp"
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
	l := harness.Launch{}
	resume := ""
	if s.Resume {
		resume = " -s " + loginshell.Quote(s.SessionID)
		l.SessionID = s.SessionID
	}
	if cfg := interactiveConfig(s); cfg != "" {
		l.Env = append(l.Env, "OPENCODE_CONFIG="+cfg)
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
	if s.Hermetic {
		// The words run after the login profile, so a profile export cannot bring user config back.
		words := hermeticWords(isolatedConfigHome(s.IsolationDir), ownLayers(l.Env)).quoted()
		l.Command = "/usr/bin/env " + strings.Join(words, " ") + " " + bin + resume
		return l
	}
	l.Command = bin + resume
	return l
}

// interactiveConfig is the OPENCODE_CONFIG path: the grant file, or the app's mcp file when there is
// one. With both, the grant joins the app file in place (the launch's own 0600 file, removed with it)
// since OPENCODE_CONFIG names one file. A merge that fails leaves the grant alone.
func interactiveConfig(s harness.InteractiveSpec) string {
	app := appMCPFile(s.App)
	if app == "" || s.MCPConfig == "" {
		return cmp.Or(s.MCPConfig, app)
	}
	if err := mergeMCP(app, []string{app, s.MCPConfig}); err != nil {
		slog.Warn("opencode: app MCP servers not applied", "scope", "opencode", "err", err)
		return s.MCPConfig
	}
	return app
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
