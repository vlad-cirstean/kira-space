package claude

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/kirathecat/kira-studio/internal/harness"
)

//go:embed settings.schema.json
var settingsSchema []byte

// defaultHookTimeout is the seconds a hook gets when its section names none.
const defaultHookTimeout = 60

// SettingsSchema is the schema of the claude section of settings.yaml.
func (*Provider) SettingsSchema() []byte { return settingsSchema }

// section is the claude section of settings.yaml, as its schema allows.
type section struct {
	Model          string `json:"model"`
	PermissionMode string `json:"permission_mode"`
	StatusLine     *struct {
		Command         string `json:"command"`
		Padding         *int   `json:"padding"`
		RefreshInterval *int   `json:"refresh_interval"`
	} `json:"status_line"`
	Hooks map[string]struct {
		Event   string `json:"event"`
		Matcher string `json:"matcher"`
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	} `json:"hooks"`
}

// settingsDoc is the document passed to --settings.
type settingsDoc struct {
	Model       string                      `json:"model,omitempty"`
	Permissions *permissionsDoc             `json:"permissions,omitempty"`
	Hooks       map[string][]hookMatcherDoc `json:"hooks,omitempty"`
	StatusLine  *statusLineDoc              `json:"statusLine,omitempty"`
}

type permissionsDoc struct {
	Allow       []string `json:"allow,omitempty"`
	Deny        []string `json:"deny,omitempty"`
	DefaultMode string   `json:"defaultMode,omitempty"`
}

type hookMatcherDoc struct {
	Matcher string           `json:"matcher,omitempty"`
	Hooks   []hookCommandDoc `json:"hooks"`
}

type hookCommandDoc struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

type statusLineDoc struct {
	Type            string `json:"type"`
	Command         string `json:"command"`
	Padding         *int   `json:"padding,omitempty"`
	RefreshInterval *int   `json:"refreshInterval,omitempty"`
}

// RenderSettings renders the MCP servers as a --mcp-config file and the rest as a --settings file;
// each is omitted when empty. A headless run gets neither permission_mode, allow nor status_line: an
// unattended run must not enter plan mode or widen the tools its step grants, and claude -p never
// runs a status line; deny still applies. No
// reference field renders into the settings file, so no secret can reach it.
func (p *Provider) RenderSettings(s harness.AgentSettings, kind harness.LaunchKind) (harness.NativeConfig, error) {
	var sec section
	if len(s.Section) > 0 {
		if err := json.Unmarshal(s.Section, &sec); err != nil {
			return harness.NativeConfig{}, fmt.Errorf("claude: settings section: %w", err)
		}
	}
	var out harness.NativeConfig
	if len(s.MCP) > 0 {
		b, err := p.MCPConfig(s.MCP)
		if err != nil {
			return out, err
		}
		out.Files = append(out.Files, harness.NativeFile{Role: "mcp", Name: "mcp.json", Data: b})
	}
	doc := buildSettings(s, sec, kind)
	if doc.Model != "" || doc.Permissions != nil || len(doc.Hooks) > 0 || doc.StatusLine != nil {
		b, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return out, fmt.Errorf("claude: encode settings: %w", err)
		}
		out.Files = append(out.Files, harness.NativeFile{Role: "settings", Name: "settings.json", Data: b})
	}
	keys := make([]string, 0, len(s.Env))
	for k := range s.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out.Env = append(out.Env, k+"="+s.Env[k])
	}
	return out, nil
}

func buildSettings(s harness.AgentSettings, sec section, kind harness.LaunchKind) settingsDoc {
	doc := settingsDoc{Model: sec.Model}
	perm := permissionsDoc{Deny: s.Deny}
	if kind == harness.LaunchInteractive {
		perm.Allow = s.Allow
		perm.DefaultMode = sec.PermissionMode
		if sl := sec.StatusLine; sl != nil {
			doc.StatusLine = &statusLineDoc{Type: "command", Command: sl.Command, Padding: sl.Padding, RefreshInterval: sl.RefreshInterval}
		}
	}
	if len(perm.Allow) > 0 || len(perm.Deny) > 0 || perm.DefaultMode != "" {
		doc.Permissions = &perm
	}
	add := func(event harness.EventKind, matcher, command string, timeout int) {
		if doc.Hooks == nil {
			doc.Hooks = map[string][]hookMatcherDoc{}
		}
		if timeout == 0 {
			timeout = defaultHookTimeout
		}
		doc.Hooks[string(event)] = append(doc.Hooks[string(event)], hookMatcherDoc{
			Matcher: matcher, Hooks: []hookCommandDoc{{Type: "command", Command: command, Timeout: timeout}},
		})
	}
	for _, h := range s.Hooks {
		add(h.Event, h.Matcher, h.Command, h.TimeoutSec)
	}
	names := make([]string, 0, len(sec.Hooks))
	for n := range sec.Hooks {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		h := sec.Hooks[n]
		add(harness.EventKind(h.Event), h.Matcher, h.Command, h.Timeout)
	}
	return doc
}
