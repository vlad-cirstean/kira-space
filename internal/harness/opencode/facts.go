package opencode

// Every constant and table below is a fact about the opencode CLI. "verified" means seen on
// 1.19.5 (P449 plan, "Probe results"); "UNVERIFIED Un" means assumed, with the plan's probe id. A
// probe that disproves one changes only its entry here and the matching fake persona shape.

// sessionIDPrefix: ids look like ses_e6561b1c7ffeXywFgamxk8TJGD (U1, verified).
const sessionIDPrefix = "ses_"

// interactiveNeedsPermission: the default permission allows every tool (U9, verified), so an
// interactive session needs no allow list. external_directory still asks; AddDirs ride permission.
const interactiveNeedsPermission = false

// capsCost: no cost cap flag or config exists (U17, UNVERIFIED).
const capsCost = false

// resumeLostMarkers are lowercase substrings of the error text of `-s <unknown id>` (U5,
// UNVERIFIED: the probe hung on stdin).
var resumeLostMarkers = []string{"session not found", "session does not exist", "no session found"}

// sessionEnv are the variables that tie a child to an OpenCode session running this process
// (U13, partly verified: OPENCODE_PID, _CLIENT, _TERMINAL, _SERVER_*, _WORKSPACE_ID, _ROUTE exist).
var sessionEnv = []string{
	"OPENCODE", "OPENCODE_PID", "OPENCODE_CLIENT", "OPENCODE_TERMINAL", "OPENCODE_SERVER_PASSWORD",
	"OPENCODE_SERVER_USERNAME", "OPENCODE_WORKSPACE_ID", "OPENCODE_ROUTE", "OPENCODE_CALLER",
}

// layerEnv are the config layers a run that loads no user config must not inherit (verified names).
var layerEnv = []string{"OPENCODE_CONFIG", "OPENCODE_CONFIG_DIR", "OPENCODE_CONFIG_CONTENT", "OPENCODE_PERMISSION"}

// quietEnv is the environment of a run with the network off (U14, OPENCODE_DISABLE_AUTOUPDATE verified).
var quietEnv = []string{"OPENCODE_DISABLE_AUTOUPDATE=1", "OPENCODE_DISABLE_LSP_DOWNLOAD=1"}

// apiSuffixes are the model API hosts a run with the network off may reach (U16, UNVERIFIED).
var apiSuffixes = []string{
	"opencode.ai", "models.dev", "anthropic.com", "openai.com", "googleapis.com", "openrouter.ai",
	"githubcopilot.com", "x.ai", "groq.com", "mistral.ai", "deepseek.com",
}

// candidates are the install paths probed after PATH, relative to home or absolute (U15, UNVERIFIED).
var homeCandidates = []string{".opencode/bin/opencode", ".local/bin/opencode", ".bun/bin/opencode"}

// systemCandidates are the system-wide install paths (U15, UNVERIFIED).
var systemCandidates = []string{"/usr/local/bin/opencode", "/opt/homebrew/bin/opencode"}

// builtinTools maps a Kira (Claude) tool name to the OpenCode permission/tool ids it covers (U8:
// ids verified from `debug agent build`; websearch UNVERIFIED, permission key only).
var builtinTools = map[string][]string{
	"Read":         {"read"},
	"Glob":         {"glob"},
	"Grep":         {"grep"},
	"Edit":         {"edit"},
	"MultiEdit":    {"edit"},
	"NotebookEdit": {"edit"},
	"Write":        {"write"},
	"Bash":         {"bash"},
	"WebFetch":     {"webfetch"},
	"WebSearch":    {"websearch"},
	"TodoWrite":    {"todowrite"},
	"Task":         {"task"},
	"Skill":        {"skill"},
}

// builtinIDs are the built-in tool ids a run can turn off (verified, `debug agent build`).
var builtinIDs = []string{"bash", "read", "glob", "grep", "edit", "write", "task", "webfetch", "todowrite", "skill"}

// mcpToolKey is the permission/tool key of an MCP tool (U7, UNVERIFIED); a server's wildcard is
// `<server>_*`.
func mcpToolKey(server, tool string) string {
	if tool == "" {
		return server + "_*"
	}
	return server + "_" + tool
}
