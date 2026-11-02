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

// hookConfigDirEnv is the layer the hook plugin rides (Compose adds it after the command is built),
// so a hermetic launch never unsets it.
const hookConfigDirEnv = "OPENCODE_CONFIG_DIR"

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

// instructionsKey: `"instructions":["/abs/file.md"]` in OPENCODE_CONFIG_CONTENT is accepted, and
// the file's text reaches the model request (Q-OC1, Q-OC4, verified on 1.19.5). It layers with
// OPENCODE_CONFIG (mcp) and OPENCODE_CONFIG_DIR (plugin) (Q-OC3, verified).
const instructionsKey = "instructions"

// Hooks load through OPENCODE_CONFIG_DIR/opencode.json (Q-OC2, verified). Observed, not relied on
// (Q-OC4, stub provider, throwaway HOME): ~/.config/opencode/AGENTS.md and ~/.claude/CLAUDE.md did
// not reach the request; the config dir's AGENTS.md did.

// Resume re-reads the instructions file: an edit before `run -s <id>` reaches the model (Q-OC4,
// verified), so the provider does not implement harness.ContextKeeper.

// App settings ride OPENCODE_CONFIG_CONTENT next to `instructions`, except `mcp`, whose credentials
// go to a 0600 OPENCODE_CONFIG file instead of the environment. Verified on 1.19.5 (P453
// probes): `model` is kept and `plugin` merges across OPENCODE_CONFIG_DIR/opencode.json and the
// content (Q-OC5); `mcp` merges across the OPENCODE_CONFIG file and the content (Q-OC6);
// `permission` merges across OPENCODE_PERMISSION and the content (Q-OC8); a local server's
// `environment` reaches its process (Q-OC9).
//
// The content text is substituted (Q-OC7: {env:HOME} expands), so a value holding one of
// substitutionMarks is refused, not passed on.
var substitutionMarks = []string{"{env:", "{file:"}

// maxConfigContent caps the OPENCODE_CONFIG_CONTENT document the app settings render: Linux
// MAX_ARG_STRLEN is 128 KiB per environment string.
const maxConfigContent = 64 << 10

// User-level config (P454, probed on 1.19.5 by the orchestrator; internal/agentport holds the file
// names, G5 keeps them out of this package):
//   - Q-P454-OC1: in the user config dir opencode.json, opencode.jsonc and config.json all load; with
//     opencode.json and opencode.jsonc both present, opencode.jsonc wins. Export edits the winning
//     file, import reads it.
//   - Q-P454-OC2: JSONC (a comment, a trailing comma) is accepted.
//   - Q-P454-OC3: {env:X} is substituted in the user config (an mcp url); an unset variable becomes the
//     empty string, no error.
//   - Q-P454-OC4: {env:X} works in a local server's `environment` value and reaches its process.
//   - Q-P454-OC5: `permission` keeps edit, bash pattern maps and webfetch, in order.
//   - Q-P454-OC6: a run or `debug config` REWRITES the user's opencode.json: it inserts "$schema" as
//     the first key, keeps the rest minified, and creates package.json, package-lock.json and
//     node_modules in the config dir. Export must tolerate a "$schema" insert between preview and
//     apply.
//   - Q-P454-OC7: with a stub provider the global AGENTS.md in the user config dir reached the model
//     request, with and without `instructions` in opencode.json. Setting OPENCODE_CONFIG_DIR appears to
//     replace the global dir, so a launch that sets it does not load the global AGENTS.md (P455).
//   - Q-P454-OC8: an npm plugin name is accepted; import takes file:// plugins only.

// Hermetic launches (P455, orchestrator probes H-OC1..5 on 1.19.5, stub model, throwaway HOME):
//   - H-OC1: with XDG_CONFIG_HOME at a dir linking every ~/.config entry except `opencode`, `debug
//     config` drops the user's `model`, `mcp` and `plugin`; a project opencode.json `mcp` stays.
//     Without the redirect all of them load.
//   - H-OC2: ~/.opencode/opencode.json STILL loads under the redirect (its `mcp` is present). Only a
//     HOME redirect would drop it, and that moves git, ssh and tool config too: not done, a Known
//     open item.
//   - H-OC3: skills under the redirect: ~/.claude/skills, ~/.agents/skills, project .claude/skills and
//     project .opencode/skill load; ~/.config/opencode/skill drops. OPENCODE_DISABLE_CLAUDE_CODE_SKILLS=1
//     drops ~/.claude/skills and project .claude/skills; OPENCODE_DISABLE_EXTERNAL_SKILLS=1 also drops
//     ~/.agents/skills. No name drops only the user ones, so none is set (project skills kept) and the
//     user-skill leak is a Known open item.
//   - H-OC4: instructions. Baseline: global AGENTS.md, project AGENTS.md and Kira's file arrive;
//     ~/.claude/CLAUDE.md does not (AGENTS.md wins). Redirect alone: ~/.claude/CLAUDE.md arrives once
//     the global AGENTS.md is gone. Redirect plus OPENCODE_DISABLE_CLAUDE_CODE_PROMPT=1: only project
//     AGENTS.md and Kira's file arrive. The same name also drops a project CLAUDE.md, so a hermetic
//     launch does not load project CLAUDE.md (project AGENTS.md loads).
//   - H-OC5: the data dir holds log/, snapshot/, repos/ and opencode.db only (auth and sessions); the
//     redirect leaves it alone, login survives.
//
// hermeticEnv is the OPENCODE_DISABLE_* set of a hermetic launch that keeps project config.
var hermeticEnv = []string{"OPENCODE_DISABLE_CLAUDE_CODE_PROMPT=1"}
