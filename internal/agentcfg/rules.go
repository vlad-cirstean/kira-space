package agentcfg

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// problem builds a Problem at a JSON pointer; the loader adds its line.
func problem(path, key string, args map[string]string) *configfs.Problem {
	return &configfs.Problem{Path: path, Message: i18n.T(key, args)}
}

func pointer(parts ...string) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString("/" + strings.ReplaceAll(strings.ReplaceAll(p, "~", "~0"), "/", "~1"))
	}
	return b.String()
}

// rules are the cross-field checks run after schema validation. Each Doc renders into AGENTS.md.
func rules(providers []string) []configfs.Rule[Settings] {
	return []configfs.Rule[Settings]{
		{
			Doc:   "An MCP server has `command` or `url`, not both. `args` and `env` go with `command`, `headers` with `url`.",
			Check: func(s Settings, _ configfs.Locator) *configfs.Problem { return checkTransport(s) },
		},
		{
			Doc:   "An MCP server name never starts with `kira` (Kira's own servers). An environment variable name never starts with `KIRA_` (Kira's launch variables).",
			Check: func(s Settings, _ configfs.Locator) *configfs.Problem { return checkKiraNames(s) },
		},
		{
			Doc:   "`matcher` applies to `tool_start` and `tool_end` hooks only.",
			Check: func(s Settings, _ configfs.Locator) *configfs.Problem { return checkMatchers(s) },
		},
		{
			Doc:   "Every `skip_hooks` entry of a provider names a hook in the common `hooks`.",
			Check: func(s Settings, _ configfs.Locator) *configfs.Problem { return checkSkips(s, providers) },
		},
	}
}

func checkTransport(s Settings) *configfs.Problem {
	for _, name := range sortedKeys(s.MCPServers) {
		m := s.MCPServers[name]
		at := func(field string) string { return pointer("mcp_servers", name, field) }
		switch {
		case m.URL != "" && m.Command != "":
			return problem(at("command"), "backend.core.agentSettings.serverBoth.body", map[string]string{"server": name})
		case m.URL == "" && m.Command == "":
			return problem(pointer("mcp_servers", name), "backend.core.agentSettings.serverNone.body", map[string]string{"server": name})
		case m.URL != "" && (len(m.Args) > 0 || len(m.Env) > 0):
			field := "args"
			if len(m.Args) == 0 {
				field = "env"
			}
			return problem(at(field), "backend.core.agentSettings.serverField.body", map[string]string{"field": field, "server": name, "transport": "command"})
		case m.Command != "" && len(m.Headers) > 0:
			return problem(at("headers"), "backend.core.agentSettings.serverField.body", map[string]string{"field": "headers", "server": name, "transport": "url"})
		}
	}
	return nil
}

func checkKiraNames(s Settings) *configfs.Problem {
	for _, name := range sortedKeys(s.MCPServers) {
		if strings.HasPrefix(name, "kira") {
			return problem(pointer("mcp_servers", name), "backend.core.agentSettings.kiraServer.body", map[string]string{"server": name})
		}
	}
	check := func(env map[string]string, path ...string) *configfs.Problem {
		for _, k := range sortedKeys(env) {
			if strings.HasPrefix(k, "KIRA_") {
				return problem(pointer(append(path, k)...), "backend.core.agentSettings.kiraEnv.body", map[string]string{"name": k, "prefix": "KIRA_"})
			}
		}
		return nil
	}
	if p := check(s.Env, "env"); p != nil {
		return p
	}
	for _, name := range sortedKeys(s.MCPServers) {
		if p := check(s.MCPServers[name].Env, "mcp_servers", name, "env"); p != nil {
			return p
		}
	}
	return nil
}

func checkMatchers(s Settings) *configfs.Problem {
	for _, name := range sortedKeys(s.Hooks) {
		h := s.Hooks[name]
		if h.Matcher != "" && h.Event != "tool_start" && h.Event != "tool_end" {
			return problem(pointer("hooks", name, "matcher"), "backend.core.agentSettings.hookMatcher.body", map[string]string{"hook": name})
		}
	}
	return nil
}

func checkSkips(s Settings, providers []string) *configfs.Problem {
	for _, id := range providers {
		var sec struct {
			SkipHooks []string `json:"skip_hooks"`
		}
		if raw := s.Sections[id]; raw != nil {
			_ = json.Unmarshal(raw, &sec)
		}
		for i, n := range sec.SkipHooks {
			if _, ok := s.Hooks[n]; !ok {
				return problem(pointer(id, "skip_hooks", strconv.Itoa(i)), "backend.core.agentSettings.skipHook.body", map[string]string{"hook": n})
			}
		}
	}
	return nil
}
