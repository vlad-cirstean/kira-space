package agentcfg

import (
	"encoding/json"
	"fmt"

	"github.com/kirathecat/kira-studio/internal/harness"
)

// defaultHookTimeout is the seconds a hook gets when the file names none.
const defaultHookTimeout = 60

// hookEvents maps the neutral hook events to the harness kinds.
var hookEvents = map[string]harness.EventKind{
	"session_start": harness.EventSessionStart,
	"session_end":   harness.EventSessionEnd,
	"prompt":        harness.EventPrompt,
	"tool_start":    harness.EventToolStart,
	"tool_end":      harness.EventToolEnd,
	"notification":  harness.EventNotification,
	"turn_end":      harness.EventTurnEnd,
}

// enabled reports whether the server is on.
func (m MCPServer) enabled() bool { return m.Enabled == nil || *m.Enabled }

// skipSet is the provider section's skip_hooks as a set.
func skipSet(section json.RawMessage) map[string]bool {
	var sec struct {
		SkipHooks []string `json:"skip_hooks"`
	}
	_ = json.Unmarshal(section, &sec)
	out := make(map[string]bool, len(sec.SkipHooks))
	for _, n := range sec.SkipHooks {
		out[n] = true
	}
	return out
}

// agentSettings builds the provider's input from the settings with env and server fields already
// transformed by resolve (identity for the view). Disabled servers and skipped hooks drop out.
func (s Settings) agentSettings(provider string, resolve func(string) (string, error)) (harness.AgentSettings, error) {
	var out harness.AgentSettings
	var err error
	if out.Env, err = resolveMap(s.Env, resolve); err != nil {
		return out, err
	}
	for _, name := range sortedKeys(s.MCPServers) {
		m := s.MCPServers[name]
		if !m.enabled() {
			continue
		}
		srv := harness.MCPServer{Name: name}
		if srv.URL, err = resolve(m.URL); err != nil {
			return out, fmt.Errorf("mcp_servers.%s.url: %w", name, err)
		}
		if srv.Command, err = resolve(m.Command); err != nil {
			return out, fmt.Errorf("mcp_servers.%s.command: %w", name, err)
		}
		if srv.Headers, err = resolveMap(m.Headers, resolve); err != nil {
			return out, fmt.Errorf("mcp_servers.%s.headers: %w", name, err)
		}
		if srv.Env, err = resolveMap(m.Env, resolve); err != nil {
			return out, fmt.Errorf("mcp_servers.%s.env: %w", name, err)
		}
		for _, a := range m.Args {
			v, err := resolve(a)
			if err != nil {
				return out, fmt.Errorf("mcp_servers.%s.args: %w", name, err)
			}
			srv.Args = append(srv.Args, v)
		}
		out.MCP = append(out.MCP, srv)
	}
	out.Allow, out.Deny = s.Permissions.Allow, s.Permissions.Deny
	section := s.Sections[provider]
	skip := skipSet(section)
	for _, name := range sortedKeys(s.Hooks) {
		if skip[name] {
			continue
		}
		h := s.Hooks[name]
		timeout := h.Timeout
		if timeout == 0 {
			timeout = defaultHookTimeout
		}
		out.Hooks = append(out.Hooks, harness.Hook{Name: name, Matcher: h.Matcher, Command: h.Command, Event: hookEvents[h.Event], TimeoutSec: timeout})
	}
	out.Section = section
	return out, nil
}

// resolveMap transforms every value of m in key order; nil stays nil.
func resolveMap(m map[string]string, resolve func(string) (string, error)) (map[string]string, error) {
	if m == nil {
		return nil, nil
	}
	out := make(map[string]string, len(m))
	for _, k := range sortedKeys(m) {
		v, err := resolve(m[k])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		out[k] = v
	}
	return out, nil
}
