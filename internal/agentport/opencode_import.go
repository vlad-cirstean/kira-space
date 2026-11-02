package agentport

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/kirathecat/kira-studio/internal/harness/opencode"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

var (
	openCodeModelRe = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._:-]+$`)
	filePluginRe    = regexp.MustCompile(`^file:///[^\s]+$`)
)

func openCodeRefIn(v string) (string, bool) { return opencode.RefsToKira(v) }

// importOpenCode reads the user's opencode config and AGENTS.md.
func (s *Service) importOpenCode(im *importer, files []target) {
	for _, t := range files {
		src := Source{Role: t.Role, Path: s.display(t.Path)}
		im.source = src.Path
		switch t.Role {
		case roleConfig:
			im.openCodeConfig(t, &src)
		case roleContext:
			im.importContext(t, src.Path, &src)
		}
		im.sources = append(im.sources, src)
	}
}

func (im *importer) openCodeConfig(t target, src *Source) {
	data, _, exists, err := readIfExists(t.Path, maxSettingsFile)
	src.Exists = exists
	if err != nil {
		src.Problem = readProblem(t.Path, err, maxSettingsFile)
		return
	}
	if !exists {
		return
	}
	found, others, err := topLevel(data, "mcp", "permission", "model", "plugin")
	if err != nil {
		src.Problem = i18n.T("backend.core.agentPort.notJSON.body", map[string]string{"file": baseName(t.Path)})
		return
	}
	im.openCodeServers(found["mcp"])
	im.openCodePermission(found["permission"])
	if raw := found["model"]; raw != nil {
		var m string
		if json.Unmarshal(raw, &m) != nil || len(m) > 200 || !openCodeModelRe.MatchString(m) {
			im.skip("opencode/model", "model", im.skipReason("badValue", ""))
		} else {
			im.add("opencode/model", "model", []op{{path: []string{"opencode", "model"}, value: m}}, nil, nil)
		}
	}
	im.openCodePlugins(found["plugin"])
	for _, k := range others {
		if k == "$schema" {
			continue
		}
		im.skipKey(k, "other", im.skipReason("noSetting", ""))
	}
}

type openCodeServer struct {
	Type        string            `json:"type"`
	Command     []string          `json:"command"`
	Environment map[string]string `json:"environment"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers"`
	Enabled     *bool             `json:"enabled"`
	OAuth       json.RawMessage   `json:"oauth"`
}

func (im *importer) openCodeServers(raw json.RawMessage) {
	if raw == nil {
		return
	}
	var servers map[string]json.RawMessage
	if json.Unmarshal(raw, &servers) != nil {
		return
	}
	for _, name := range sortedKeys(servers) {
		id := "mcp/" + name
		var s openCodeServer
		if json.Unmarshal(servers[name], &s) != nil {
			im.skip(id, "mcp", im.skipReason("badValue", ""))
			continue
		}
		ns := nativeServer{Disabled: s.Enabled != nil && !*s.Enabled}
		switch {
		case s.OAuth != nil && strings.TrimSpace(string(s.OAuth)) != "false":
			im.skip(id, "mcp", im.skipReason("serverAuth", ""))
			continue
		case s.Type == "remote" && s.URL != "":
			ns.URL, ns.Headers = s.URL, s.Headers
		case s.Type == "local" && len(s.Command) > 0:
			ns.Command, ns.Args, ns.Env = s.Command[0], s.Command[1:], s.Environment
		default:
			im.skip(id, "mcp", im.skipReason("badValue", ""))
			continue
		}
		im.addServer(name, ns)
	}
}

func (im *importer) openCodePermission(raw json.RawMessage) {
	if raw == nil {
		return
	}
	allow, deny, skipped, mixed := opencode.KiraPermissions(raw)
	for _, e := range allow {
		im.permission("allow", e)
	}
	for _, e := range deny {
		im.permission("deny", e)
	}
	for _, k := range skipped {
		im.skipKey(k, "other", im.skipReason("permissionRule", ""))
	}
	for _, k := range mixed {
		im.notes = append(im.notes, i18n.T("backend.core.agentPort.note.permissionOrder.body", map[string]string{"key": k}))
	}
}

func (im *importer) openCodePlugins(raw json.RawMessage) {
	if raw == nil {
		return
	}
	var plugins []string
	if json.Unmarshal(raw, &plugins) != nil {
		return
	}
	for _, p := range plugins {
		id := "opencode/plugin/" + p
		if !filePluginRe.MatchString(p) || len(p) > 2000 {
			im.skip(id, "plugin", im.skipReason("npmPlugin", ""))
			continue
		}
		im.add(id, "plugin", []op{{path: []string{"opencode", "plugins"}, append: true, value: p}}, nil, nil)
	}
}
