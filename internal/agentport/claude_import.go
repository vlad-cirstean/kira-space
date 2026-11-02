package agentport

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

var (
	claudeModelRe = regexp.MustCompile(`^[A-Za-z0-9._:/\[\]-]{1,100}$`)
	permModes     = []string{"acceptEdits", "auto", "bypassPermissions", "manual", "dontAsk", "plan"}
	// loginKeys stay with Claude Code: never imported, never listed.
	loginKeys = []string{"apiKeyHelper", "awsAuthRefresh", "awsCredentialExport", "otelHeadersHelper", "forceLoginMethod"}
)

const (
	hookMaxTimeout = 600
	maxEntryLen    = 500
)

// skipReason is the i18n text of a skip; arg fills the one placeholder some reasons have.
func (im *importer) skipReason(key, arg string) string {
	switch key {
	case "badName":
		return i18n.T("backend.core.agentPort.skip.badName.body", nil)
	case "badValue":
		return i18n.T("backend.core.agentPort.skip.badValue.body", nil)
	case "hookEvent":
		return i18n.T("backend.core.agentPort.skip.hookEvent.body", map[string]string{"event": arg})
	case "hookType":
		return i18n.T("backend.core.agentPort.skip.hookType.body", map[string]string{"event": arg})
	case "kiraName":
		return i18n.T("backend.core.agentPort.skip.kiraName.body", map[string]string{"prefix": arg})
	case "noSetting":
		return i18n.T("backend.core.agentPort.skip.noSetting.body", nil)
	case "notText":
		return i18n.T("backend.core.agentPort.skip.notText.body", nil)
	case "npmPlugin":
		return i18n.T("backend.core.agentPort.skip.npmPlugin.body", nil)
	case "permissionRule":
		return i18n.T("backend.core.agentPort.skip.permissionRule.body", nil)
	case "serverAuth":
		return i18n.T("backend.core.agentPort.skip.serverAuth.body", nil)
	}
	return i18n.T("backend.core.agentPort.skip.transport.body", map[string]string{"transport": arg})
}

// importClaude reads ~/.claude/settings.json, CLAUDE.md and the user mcpServers of ~/.claude.json.
func (s *Service) importClaude(im *importer, files []target) {
	im.claudeEnum = s.claudeHookEvents()
	for _, t := range files {
		src := Source{Role: t.Role, Path: s.display(t.Path)}
		im.source = src.Path
		switch t.Role {
		case roleSettings:
			im.claudeSettings(t, &src)
		case roleContext:
			im.importContext(t, src.Path, &src)
		case roleMCP:
			im.claudeServers(t, &src)
		}
		im.sources = append(im.sources, src)
	}
}

// claudeHookEvents are the event names the claude section accepts, read from its schema.
func (s *Service) claudeHookEvents() []string {
	p, ok := s.o.Providers.Get(providerClaude)
	if !ok {
		return nil
	}
	host, ok := harness.SettingsHostOf(p)
	if !ok {
		return nil
	}
	var sch struct {
		Properties struct {
			Hooks struct {
				Items struct {
					Properties struct {
						Event struct {
							Enum []string `json:"enum"`
						} `json:"event"`
					} `json:"properties"`
				} `json:"additionalProperties"`
			} `json:"hooks"`
		} `json:"properties"`
	}
	_ = json.Unmarshal(host.SettingsSchema(), &sch)
	return sch.Properties.Hooks.Items.Properties.Event.Enum
}

func (im *importer) claudeSettings(t target, src *Source) {
	data, _, exists, err := readIfExists(t.Path, maxSettingsFile)
	src.Exists = exists
	if err != nil {
		src.Problem = readProblem(t.Path, err, maxSettingsFile)
		return
	}
	if !exists {
		return
	}
	found, others, err := topLevel(data, "env", "permissions", "model", "statusLine", "hooks")
	if err != nil {
		src.Problem = i18n.T("backend.core.agentPort.notJSON.body", map[string]string{"file": baseName(t.Path)})
		return
	}
	im.claudeEnv(found["env"])
	im.claudePermissions(found["permissions"])
	im.claudeModel(found["model"])
	im.claudeStatusLine(found["statusLine"])
	im.claudeHooks(found["hooks"])
	for _, k := range others {
		if slices.Contains(loginKeys, k) {
			continue
		}
		im.skipKey(k, "other", im.skipReason("noSetting", ""))
	}
}

func (im *importer) claudeEnv(raw json.RawMessage) {
	if raw == nil {
		return
	}
	var env map[string]json.RawMessage
	if json.Unmarshal(raw, &env) != nil {
		return
	}
	for _, k := range sortedKeys(env) {
		id := "env/" + k
		var v string
		switch {
		case json.Unmarshal(env[k], &v) != nil:
			im.skip(id, "env", im.skipReason("notText", ""))
		case !envNameRe.MatchString(k):
			im.skip(id, "env", im.skipReason("badName", ""))
		case strings.HasPrefix(k, "KIRA_"):
			im.skip(id, "env", im.skipReason("kiraName", "KIRA_"))
		default:
			text, slots, pend, why := im.value(locEnv, k, v, "env-"+k, "env."+k)
			if why != "" {
				im.skip(id, "env", why)
				continue
			}
			im.add(id, "env", []op{{path: []string{"env", k}, value: text}}, slots, pend)
		}
	}
}

func (im *importer) claudePermissions(raw json.RawMessage) {
	if raw == nil {
		return
	}
	var p struct {
		Allow       []string        `json:"allow"`
		Deny        []string        `json:"deny"`
		Ask         json.RawMessage `json:"ask"`
		Dirs        json.RawMessage `json:"additionalDirectories"`
		DefaultMode string          `json:"defaultMode"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return
	}
	for _, e := range p.Allow {
		im.permission("allow", e)
	}
	for _, e := range p.Deny {
		im.permission("deny", e)
	}
	if p.Ask != nil {
		im.skipKey("permissions.ask", "other", im.skipReason("noSetting", ""))
	}
	if p.Dirs != nil {
		im.skipKey("permissions.additionalDirectories", "other", im.skipReason("noSetting", ""))
	}
	if p.DefaultMode != "" {
		if slices.Contains(permModes, p.DefaultMode) {
			im.add("claude/permission_mode", "permission_mode", []op{{path: []string{"claude", "permission_mode"}, value: p.DefaultMode}}, nil, nil)
		} else {
			im.skip("claude/permission_mode", "permission_mode", im.skipReason("badValue", ""))
		}
	}
}

// permission adds one allow or deny entry.
func (im *importer) permission(kind, entry string) {
	id := kind + "/" + entry
	if entry == "" || len(entry) > maxEntryLen {
		im.skip(id, kind, im.skipReason("badValue", ""))
		return
	}
	im.add(id, kind, []op{{path: []string{"permissions", kind}, append: true, value: entry}}, nil, nil)
}

func (im *importer) claudeModel(raw json.RawMessage) {
	if raw == nil {
		return
	}
	var m string
	if json.Unmarshal(raw, &m) != nil || !claudeModelRe.MatchString(m) {
		im.skip("claude/model", "model", im.skipReason("badValue", ""))
		return
	}
	im.add("claude/model", "model", []op{{path: []string{"claude", "model"}, value: m}}, nil, nil)
}

func (im *importer) claudeStatusLine(raw json.RawMessage) {
	if raw == nil {
		return
	}
	var sl struct {
		Type            string `json:"type"`
		Command         string `json:"command"`
		Padding         *int   `json:"padding"`
		RefreshInterval *int   `json:"refreshInterval"`
	}
	if json.Unmarshal(raw, &sl) != nil || sl.Type != "command" || sl.Command == "" || len(sl.Command) > 8192 ||
		(sl.Padding != nil && (*sl.Padding < 0 || *sl.Padding > 10)) ||
		(sl.RefreshInterval != nil && (*sl.RefreshInterval < 1 || *sl.RefreshInterval > 3600)) {
		im.skip("claude/status_line", "status_line", im.skipReason("badValue", ""))
		return
	}
	v := omap{{"command", sl.Command}}
	if sl.Padding != nil {
		v = append(v, kv{"padding", *sl.Padding})
	}
	if sl.RefreshInterval != nil {
		v = append(v, kv{"refresh_interval", *sl.RefreshInterval})
	}
	im.add("claude/status_line", "status_line", []op{{path: []string{"claude", "status_line"}, value: v}}, nil, nil)
}

type claudeHookGroup struct {
	Matcher string `json:"matcher"`
	Hooks   []struct {
		Type    string `json:"type"`
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	} `json:"hooks"`
}

func (im *importer) claudeHooks(raw json.RawMessage) {
	if raw == nil {
		return
	}
	var byEvent map[string][]claudeHookGroup
	if json.Unmarshal(raw, &byEvent) != nil {
		return
	}
	for _, event := range sortedKeys(byEvent) {
		for _, g := range byEvent[event] {
			for _, h := range g.Hooks {
				im.claudeHook(event, g.Matcher, h.Type, h.Command, h.Timeout)
			}
		}
	}
}

func (im *importer) claudeHook(event, matcher, typ, command string, timeout int) {
	label := event
	switch {
	case typ != "command":
		im.skip("hook-skip/"+event+"/"+typ, "hook", im.skipReason("hookType", label))
		return
	case command == "" || len(command) > 8192 || len(matcher) > maxEntryLen || timeout < 0 || timeout > hookMaxTimeout:
		im.skip("hook-skip/"+event+"/bad", "hook", im.skipReason("badValue", ""))
		return
	}
	neutral, mapped := neutralOf(event)
	common := mapped && (matcher == "" || neutral == "tool_start" || neutral == "tool_end")
	if !common && !slices.Contains(im.claudeEnum, event) {
		im.skip("hook-skip/"+event+"/event", "hook", im.skipReason("hookEvent", label))
		return
	}
	h := omap{{"event", event}}
	prefix, idPrefix, kind := "claude", "claude.hook/", "native_hook"
	path := []string{"claude", "hooks"}
	if common {
		h = omap{{"event", neutral}}
		prefix, idPrefix, kind, path = "", "hook/", "hook", []string{"hooks"}
	}
	if matcher != "" {
		h = append(h, kv{"matcher", matcher})
	}
	h = append(h, kv{"command", command})
	if timeout > 0 {
		h = append(h, kv{"timeout", timeout})
	}
	name := im.hookName(prefix, event)
	it := im.add(idPrefix+name, kind, []op{{path: append(slices.Clone(path), name), value: h}}, nil, nil)
	if im.sameHook(providerClaude, event, matcher, command) {
		it.same = true
		it.resolveState(im.cur)
	}
}

// neutralOf is the settings.yaml event name of a Claude event.
func neutralOf(event string) (string, bool) {
	return neutralEvent(harness.EventKind(event))
}

// claudeServers reads the user mcpServers of ~/.claude.json and nothing else in that file.
func (im *importer) claudeServers(t target, src *Source) {
	data, _, exists, err := readIfExists(t.Path, maxClaudeJSON)
	src.Exists = exists
	if err != nil {
		src.Problem = readProblem(t.Path, err, maxClaudeJSON>>20<<20)
		return
	}
	if !exists {
		return
	}
	found, _, err := topLevel(data, "mcpServers")
	if err != nil {
		src.Problem = i18n.T("backend.core.agentPort.notJSON.body", map[string]string{"file": baseName(t.Path)})
		return
	}
	var servers map[string]json.RawMessage
	if json.Unmarshal(found["mcpServers"], &servers) != nil {
		return
	}
	for _, name := range sortedKeys(servers) {
		im.claudeServer(name, servers[name])
	}
}

type claudeServer struct {
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	OAuth   json.RawMessage   `json:"oauth"`
	Helper  json.RawMessage   `json:"headersHelper"`
}

func (im *importer) claudeServer(name string, raw json.RawMessage) {
	id := "mcp/" + name
	var s claudeServer
	if json.Unmarshal(raw, &s) != nil {
		im.skip(id, "mcp", im.skipReason("badValue", ""))
		return
	}
	switch {
	case s.Type == "sse" || s.Type == "ws":
		im.skip(id, "mcp", im.skipReason("transport", strings.ToUpper(s.Type)))
	case s.OAuth != nil || s.Helper != nil:
		im.skip(id, "mcp", im.skipReason("serverAuth", ""))
	case s.Type == "http" && s.URL == "", s.Type != "http" && s.Command == "":
		im.skip(id, "mcp", im.skipReason("badValue", ""))
	default:
		ns := nativeServer{Command: s.Command, Args: s.Args, Env: s.Env}
		if s.Type == "http" {
			ns = nativeServer{URL: s.URL, Headers: s.Headers}
		}
		im.addServer(name, ns)
	}
}
