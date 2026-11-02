package agentcfg

import (
	"encoding/json"
	"sort"
)

// Settings is the decoded settings.yaml. References stay verbatim: launches resolve them.
type Settings struct {
	Env         map[string]string    `json:"env,omitempty"`
	Permissions Permissions          `json:"permissions"`
	MCPServers  map[string]MCPServer `json:"mcp_servers,omitempty"`
	Hooks       map[string]HookDef   `json:"hooks,omitempty"`
	// Sections are the providers' own sections by provider id, validated by each provider's schema.
	Sections map[string]json.RawMessage `json:"-"`
}

// Permissions are tool permissions in Kira's tool vocabulary (Claude's tool names).
type Permissions struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// MCPServer is an http (url, headers) or stdio (command, args, env) server. Enabled defaults to true.
type MCPServer struct {
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Enabled *bool             `json:"enabled,omitempty"`
}

// HookDef is one common hook in the neutral vocabulary.
type HookDef struct {
	Event   string `json:"event"`
	Matcher string `json:"matcher,omitempty"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

// UnmarshalJSON reads the known keys into fields and every other key into Sections; the schema
// already refused keys that are neither.
func (s *Settings) UnmarshalJSON(b []byte) error {
	type plain Settings
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(b, &all); err != nil {
		return err
	}
	for _, k := range []string{"env", "permissions", "mcp_servers", "hooks"} {
		delete(all, k)
	}
	p.Sections = all
	if len(all) == 0 {
		p.Sections = nil
	}
	*s = Settings(p)
	return nil
}

// MarshalJSON writes the sections back at the top level, the shape the file has.
func (s Settings) MarshalJSON() ([]byte, error) {
	type plain Settings
	b, err := json.Marshal(plain(s))
	if err != nil {
		return nil, err
	}
	if len(s.Sections) == 0 {
		return b, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	for k, v := range s.Sections {
		m[k] = v
	}
	return json.Marshal(m)
}

// sortedKeys is m's keys in order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
