package claude

import (
	"encoding/json"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/harness/claude/mcpinstall"
)

// MCPToolName is the name claude lists in --allowedTools for an MCP tool.
func (*Provider) MCPToolName(t harness.MCPTool) string {
	return "mcp__" + t.Server + "__" + t.Tool
}

// MCPConfig is the --mcp-config document: http servers carry type, url and headers; stdio servers
// carry command, args and env.
func (*Provider) MCPConfig(servers []harness.MCPServer) ([]byte, error) {
	m := make(map[string]any, len(servers))
	for _, s := range servers {
		if s.URL != "" {
			doc := map[string]any{"type": "http", "url": s.URL, "headers": s.Headers}
			setTimeout(doc, s)
			m[s.Name] = doc
			continue
		}
		// claude rejects null args and env, so each is left out when empty.
		doc := map[string]any{"command": s.Command}
		if len(s.Args) > 0 {
			doc["args"] = s.Args
		}
		if len(s.Env) > 0 {
			doc["env"] = s.Env
		}
		setTimeout(doc, s)
		m[s.Name] = doc
	}
	return json.Marshal(map[string]any{"mcpServers": m})
}

// setTimeout writes the per-server call timeout in ms; claude ignores values under 1000.
func setTimeout(doc map[string]any, s harness.MCPServer) {
	if s.Timeout > 0 {
		doc["timeout"] = s.Timeout.Milliseconds()
	}
}

// ToolArgs returns the --tools list (the ticked built-ins) and the exact --allowedTools list: the
// built-ins with Bash narrowed to its patterns, the ticked MCP tools, then extra, deduplicated.
func (p *Provider) ToolArgs(pol harness.ToolPolicy) (tools, allowed []string) {
	tools = append([]string{}, pol.Builtins...)
	seen := map[string]bool{}
	add := func(t string) {
		if !seen[t] {
			seen[t] = true
			allowed = append(allowed, t)
		}
	}
	for _, t := range pol.Builtins {
		if t == "Bash" && len(pol.BashPatterns) > 0 {
			for _, bp := range pol.BashPatterns {
				add("Bash(" + bp + ")")
			}
			continue
		}
		add(t)
	}
	for _, c := range pol.MCP {
		if c.AllTools {
			add("mcp__" + c.Server)
			continue
		}
		for _, t := range c.Tools {
			add(p.MCPToolName(harness.MCPTool{Server: c.Server, Tool: t}))
		}
	}
	for _, t := range pol.Extra {
		add(t)
	}
	if allowed == nil {
		allowed = []string{}
	}
	return tools, allowed
}

// MCPInstaller registers servers through `claude mcp add-json`.
func (p *Provider) MCPInstaller() harness.MCPInstaller {
	return mcpinstall.New(mcpinstall.Deps{LookPath: p.lookPath, Stat: p.stat, Candidates: Candidates})
}
