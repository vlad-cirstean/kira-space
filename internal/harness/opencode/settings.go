package opencode

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

//go:embed settings.schema.json
var settingsSchema []byte

// configRole is the NativeFile role, and AppConfig.Inline key, of the OPENCODE_CONFIG_CONTENT document.
// mcpRole is the role, and AppConfig.Files key, of the 0600 file holding the resolved `mcp` servers:
// their headers and environment are credentials, which must not ride an environment variable every
// child of the agent inherits.
const (
	configRole = "config"
	mcpRole    = "mcp"
)

var _ harness.SettingsHost = (*Provider)(nil)

// SettingsSchema is the schema of the opencode section of settings.yaml.
func (*Provider) SettingsSchema() []byte { return settingsSchema }

// section is the opencode section of settings.yaml, as its schema allows.
type section struct {
	Model   string   `json:"model"`
	Plugins []string `json:"plugins"`
}

// appDoc is the app's part of OPENCODE_CONFIG_CONTENT. Kira's own part is `instructions`. It holds
// no credential: `mcp` goes to its own file.
type appDoc struct {
	Model      string              `json:"model,omitempty"`
	Permission map[string]*ruleSet `json:"permission,omitempty"`
	Plugin     []string            `json:"plugin,omitempty"`
}

// RenderSettings renders the non-secret keys as an inline OPENCODE_CONFIG_CONTENT document and the
// `mcp` servers, whose resolved headers and environment are credentials, as a 0600 file for
// OPENCODE_CONFIG. A headless run gets deny rules only: an unattended run must not widen the tools
// its step grants. An untranslatable deny entry fails the render (fail closed); an untranslatable
// allow entry and every common hook are notes.
func (p *Provider) RenderSettings(s harness.AgentSettings, kind harness.LaunchKind) (harness.NativeConfig, error) {
	var sec section
	if len(s.Section) > 0 {
		if err := json.Unmarshal(s.Section, &sec); err != nil {
			return harness.NativeConfig{}, fmt.Errorf("opencode: settings section: %w", err)
		}
	}
	var out harness.NativeConfig
	doc := appDoc{Model: sec.Model, Plugin: sec.Plugins}
	perm := map[string]*ruleSet{}
	if kind == harness.LaunchInteractive {
		for _, e := range s.Allow {
			rules, err := translateRules([]string{e}, "allow")
			if err != nil {
				out.Notes = append(out.Notes, i18n.T("backend.core.agentSettings.opencode.allowNotApplied.body", map[string]string{"entry": e}))
				continue
			}
			mergeRules(perm, rules)
		}
	}
	// Deny goes last: OpenCode's last matching rule wins.
	for _, e := range s.Deny {
		rules, err := translateRules([]string{e}, "deny")
		if err != nil {
			return out, err
		}
		mergeRules(perm, rules)
	}
	if len(perm) > 0 {
		doc.Permission = perm
	}
	for _, h := range s.Hooks {
		out.Notes = append(out.Notes, i18n.T("backend.core.agentSettings.opencode.hookNotApplied.body", map[string]string{"hook": h.Name}))
	}
	if doc.Model != "" || len(doc.Permission) > 0 || len(doc.Plugin) > 0 {
		b, err := json.Marshal(doc)
		if err != nil {
			return out, fmt.Errorf("opencode: encode settings: %w", err)
		}
		if err := checkMarks(b); err != nil {
			return out, err
		}
		if len(b) > maxConfigContent {
			return out, errors.New(i18n.T("backend.core.agentSettings.opencode.tooLarge.body", nil))
		}
		out.Files = append(out.Files, harness.NativeFile{Role: configRole, Name: "opencode config", Data: b, Inline: true})
		out.Notes = append(out.Notes, i18n.T("backend.core.agentSettings.opencode.envCarried.body", map[string]string{"name": "OPENCODE_CONFIG_CONTENT"}))
	}
	if len(s.MCP) > 0 {
		b, err := p.MCPConfig(s.MCP)
		if err != nil {
			return out, err
		}
		out.Files = append(out.Files, harness.NativeFile{Role: mcpRole, Name: "opencode mcp.json", Data: b})
		out.Notes = append(out.Notes, i18n.T("backend.core.agentSettings.opencode.fileCarried.body", map[string]string{"name": "OPENCODE_CONFIG"}))
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

// mergeRules appends src's entries to dst, key by key, in order.
func mergeRules(dst, src map[string]*ruleSet) {
	for k, r := range src {
		d := dst[k]
		if d == nil {
			d = &ruleSet{}
			dst[k] = d
		}
		for _, e := range r.entries {
			d.add(e.pattern, e.action)
		}
	}
}

// withAppConfig merges the app's inline document into doc, the headless OPENCODE_CONFIG_CONTENT
// document: app deny rules come after Kira's (deny wins), model and plugin are set.
func withAppConfig(doc map[string]any, app *harness.AppConfig) error {
	raw := app.Inline[configRole]
	if len(raw) == 0 {
		return nil
	}
	var in struct {
		Model      string              `json:"model"`
		Permission map[string]*ruleSet `json:"permission"`
		Plugin     []string            `json:"plugin"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return fmt.Errorf("opencode: app settings: %w", err)
	}
	if in.Model != "" {
		doc["model"] = in.Model
	}
	if len(in.Plugin) > 0 {
		doc["plugin"] = in.Plugin
	}
	if len(in.Permission) > 0 {
		perm, _ := doc["permission"].(map[string]*ruleSet)
		if perm == nil {
			perm = map[string]*ruleSet{}
		}
		mergeRules(perm, in.Permission)
		doc["permission"] = perm
	}
	return nil
}

// checkMarks refuses a document holding a substitution mark: OpenCode would expand it.
func checkMarks(b []byte) error {
	for _, mark := range substitutionMarks {
		if bytes.Contains(b, []byte(mark)) {
			return errors.New(i18n.T("backend.core.agentSettings.opencode.refSyntax.body", map[string]string{"marks": strings.Join(substitutionMarks, " or ")}))
		}
	}
	return nil
}

// appMCPFile is the path of the app's mcp file, "" when the launch has none.
func appMCPFile(app *harness.AppConfig) string {
	if app == nil {
		return ""
	}
	return app.Files[mcpRole]
}

// interactiveContent is the OPENCODE_CONFIG_CONTENT value of a TUI launch: the app document with
// Kira's `instructions` added; "" when there is neither.
func interactiveContent(s harness.InteractiveSpec) (string, error) {
	doc := map[string]json.RawMessage{}
	if s.App != nil && len(s.App.Inline[configRole]) > 0 {
		if err := json.Unmarshal(s.App.Inline[configRole], &doc); err != nil {
			return "", fmt.Errorf("opencode: app settings: %w", err)
		}
	}
	if s.ContextFile != "" {
		b, err := json.Marshal([]string{s.ContextFile})
		if err != nil {
			return "", err
		}
		doc[instructionsKey] = b
	}
	if len(doc) == 0 {
		return "", nil
	}
	b, err := json.Marshal(doc)
	return string(b), err
}

// KiraAdds lists Kira's own additions to an OpenCode launch.
func (*Provider) KiraAdds() []string {
	return []string{
		i18n.T("backend.core.agentSettings.kiraAdds.hooks.body", nil),
		i18n.T("backend.core.agentSettings.kiraAdds.mcp.body", nil),
	}
}
