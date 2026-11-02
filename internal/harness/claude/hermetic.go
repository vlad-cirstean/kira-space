package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Hermetic launch facts, Claude 2.1.316 (P455, help text and the claude binary's own JS; no model
// call). TestHermeticSession checks the observable ones against the real CLI.
//   - C1: `--setting-sources user,project,local` gates every source.
//   - C2: ~/.claude/CLAUDE.md and rules load only with the user source.
//   - C3: user `mcpServers` of ~/.claude.json drop with the user source; project .mcp.json and
//     local-scope servers stay with project and local.
//   - C4: user skills, commands, agents, plugins, env, hooks, permissions, statusLine, model,
//     apiKeyHelper also gate on the user source.
//   - C5 (binary only): claude.ai connectors ignore setting sources; ENABLE_CLAUDEAI_MCP_SERVERS=false
//     turns them off.
//   - C6 (binary only): auto memory ignores setting sources; CLAUDE_CODE_DISABLE_AUTO_MEMORY=1 turns
//     it off.
//   - C7: OAuth, keychain, .credentials.json and ~/.claude.json login are no setting source and stay.
//     A sign-in kept in user settings (apiKeyHelper, provider env) drops with the user source.
//   - C8: --settings and managed settings load whatever --setting-sources says.
//   - C9 (TestHermeticSession): a repo under $HOME loads ~/.claude/CLAUDE.md as its ancestor's
//     project memory even without the user source; `claudeMdExcludes` in --settings drops it.

// hermeticEnv is what a hermetic launch sets next to --setting-sources (C5, C6).
var hermeticEnv = []string{"ENABLE_CLAUDEAI_MCP_SERVERS=false", "CLAUDE_CODE_DISABLE_AUTO_MEMORY=1"}

// hermeticSources is the --setting-sources value of a hermetic launch that loads repo config.
const hermeticSources = "project,local"

// hermeticSettings is the --settings value that drops ~/.claude/CLAUDE.md when it loads as an
// ancestor's project memory (C9); "" when the home folder is unknown.
func hermeticSettings() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	raw, err := json.Marshal(map[string][]string{"claudeMdExcludes": {filepath.Join(home, ".claude", "CLAUDE.md")}})
	if err != nil {
		return ""
	}
	return string(raw)
}
