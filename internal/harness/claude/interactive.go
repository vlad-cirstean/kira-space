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
	if len(s.AddDirs) > 0 {
		word("--add-dir")
		for _, d := range s.AddDirs {
			word(d)
		}
	}
	if s.MCPConfig != "" {
		word("--mcp-config")
		word(s.MCPConfig)
		word("--allowedTools")
		for _, t := range s.AllowedTools {
			word(t)
		}
	}
	return harness.Launch{Command: b.String(), SessionID: s.SessionID}
}

// WithPrompt ends flag parsing with `--` so a trailing variadic flag (--add-dir a b) cannot take
// the prompt.
func (*Provider) WithPrompt(command, prompt string) string {
	return command + " -- " + loginshell.Quote(prompt)
}
