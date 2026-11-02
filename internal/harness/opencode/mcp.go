package opencode

import (
	"encoding/json"

	"github.com/kirathecat/kira-studio/internal/harness"
)

// MCPToolName is the name in Kira's tool vocabulary (Claude's `mcp__server__tool`), the form
// callers merge with a step's allowed_tools; permissionDoc translates it to OpenCode's key.
func (*Provider) MCPToolName(t harness.MCPTool) string {
	return "mcp__" + t.Server + "__" + t.Tool
}

// MCPConfig is the config document of the servers (V6, verified): remote servers carry url and
// headers, local ones a command array and environment.
func (*Provider) MCPConfig(servers []harness.MCPServer) ([]byte, error) {
	return json.Marshal(map[string]any{"mcp": mcpEntries(servers)})
}

func mcpEntries(servers []harness.MCPServer) map[string]any {
	m := make(map[string]any, len(servers))
	for _, s := range servers {
		if s.URL != "" {
			entry := map[string]any{"type": "remote", "url": s.URL, "enabled": true}
			if len(s.Headers) > 0 {
				entry["headers"] = s.Headers
			}
			m[s.Name] = entry
			continue
		}
		entry := map[string]any{"type": "local", "command": append([]string{s.Command}, s.Args...), "enabled": true}
		if len(s.Env) > 0 {
			entry["environment"] = s.Env
		}
		m[s.Name] = entry
	}
	return m
}
