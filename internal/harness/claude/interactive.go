package claude

import (
	"strings"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/loginshell"
)

// InteractiveCommand picks the session id up front (`--session-id`) so no round trip learns it;
// a resume uses `--resume`. Each extra word is single-quoted, in a fixed order.
func (p *Provider) InteractiveCommand(s harness.InteractiveSpec) harness.Launch {
	bin := s.Bin
	if bin == "" {
		bin = p.Command()
	}
	flag := " --session-id "
	if s.Resume {
		flag = " --resume "
	}
	var b strings.Builder
	b.WriteString(bin + flag + loginshell.Quote(s.SessionID))
	word := func(w string) { b.WriteString(" " + loginshell.Quote(w)) }
	for _, w := range stageArgs(s.Stage, func(w string) string { return w }) {
		word(w)
	}
	if s.Hermetic {
		word("--setting-sources")
		word(hermeticSources)
	}
	if s.ContextFile != "" {
		word(appendSystemPromptFileFlag)
		word(s.ContextFile)
	}
	if len(s.AddDirs) > 0 {
		word("--add-dir")
		for _, d := range s.AddDirs {
			word(d)
		}
	}
	appMCP := ""
	if s.App != nil {
		appMCP = s.App.Files["mcp"]
	}
	if s.MCPConfig != "" || appMCP != "" {
		word("--mcp-config")
		for _, f := range []string{s.MCPConfig, appMCP} {
			if f != "" {
				word(f)
			}
		}
	}
	if s.MCPConfig != "" {
		word("--allowedTools")
		for _, t := range s.AllowedTools {
			word(t)
		}
	}
	launch := harness.Launch{SessionID: s.SessionID}
	if s.Hermetic {
		launch.Env = append(launch.Env, hermeticEnv...)
	}
	if s.App != nil {
		launch.Env = append(launch.Env, s.App.Env...)
		if f := s.App.Files["settings"]; f != "" {
			word("--settings")
			word(f)
			launch.ConfigFile = f
		}
	}
	launch.Command = b.String()
	return launch
}

// WithPrompt ends flag parsing with `--` so a trailing variadic flag (--add-dir a b) cannot take
// the prompt.
func (*Provider) WithPrompt(command, prompt string) string {
	return command + " -- " + loginshell.Quote(prompt)
}
