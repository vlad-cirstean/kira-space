package agentport

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tailscale/hujson"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/harness/opencode"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// stripSchema drops the top-level "$schema" OpenCode inserts into the user's config on every run
// (Q-P454-OC6), so a staleness hash ignores it.
func stripSchema(b []byte) []byte {
	v, err := hujson.Parse(b)
	if err != nil || v.Find("/$schema") == nil {
		return b
	}
	if err := v.Patch([]byte(`[{"op":"remove","path":"/$schema"}]`)); err != nil {
		return b
	}
	return v.Pack()
}

func (s *Service) exportOpenCode(ex *exporter, sess *exportSession, files []target) error {
	as, _, err := s.o.Settings.ForProvider(providerOpenCode)
	if err != nil {
		return ipcerr.InternalErr(err)
	}
	var sec struct {
		Model   string   `json:"model"`
		Plugins []string `json:"plugins"`
	}
	if len(as.Section) > 0 {
		_ = json.Unmarshal(as.Section, &sec)
	}
	for _, t := range files {
		if t.Role != roleConfig {
			continue
		}
		f, err := ex.readFor(t, 0o600, maxSettingsFile)
		if err != nil {
			return err
		}
		f.norm = stripSchema
		if err := ex.openCodePatch(f, as, sec.Model, sec.Plugins); err != nil {
			return err
		}
		sess.files = append(sess.files, f)
	}
	return nil
}

func (ex *exporter) openCodePatch(f *exportFile, as harness.AgentSettings, model string, plugins []string) error {
	var found map[string]json.RawMessage
	var others []string
	if len(strings.TrimSpace(string(f.before))) > 0 {
		var err error
		found, others, err = topLevel(f.before, "mcp", "permission", "model", "plugin")
		if err != nil {
			return ipcerr.BadRequest(i18n.T("backend.core.agentPort.notJSON.body", map[string]string{"file": filepath.Base(f.path)}))
		}
	}
	p := newPatcher(found, others)
	if model != "" {
		p.set([]string{"model"}, model)
	}
	ex.openCodeMCP(p, as.MCP)
	if err := ex.openCodePermission(p, found["permission"], as); err != nil {
		return err
	}
	for _, pl := range plugins {
		p.appendTo([]string{"plugin"}, pl)
	}
	if len(as.Env) > 0 {
		ex.notes = append(ex.notes, i18n.T("backend.core.agentPort.export.opencodeEnv.body", map[string]string{"names": strings.Join(sortedKeys(as.Env), ", ")}))
	}
	if len(as.Hooks) > 0 {
		ex.notes = append(ex.notes, i18n.T("backend.core.agentPort.export.opencodeHooks.body", nil))
	}
	out, err := p.apply(f.before)
	if err != nil {
		return ipcerr.InternalErr(err)
	}
	f.after = out
	return nil
}

func (ex *exporter) openCodeMCP(p *patcher, servers []harness.MCPServer) {
	for _, srv := range servers {
		what := "mcp_servers." + srv.Name
		ok := true
		conv := func(field string, l loc, name, v string) string {
			out, good := ex.safe(what+"."+field, l, name, v, opencode.RefsFromKira, "OpenCode")
			ok = ok && good
			return out
		}
		mapped := func(field string, l loc, m map[string]string) map[string]string {
			if len(m) == 0 {
				return nil
			}
			out := make(map[string]string, len(m))
			for k, v := range m {
				out[k] = conv(field+"."+k, l, k, v)
			}
			return out
		}
		c := harness.MCPServer{Name: srv.Name}
		if srv.URL != "" {
			c.URL, c.Headers = conv("url", locURL, "", srv.URL), mapped("headers", locHeader, srv.Headers)
		} else {
			c.Command, c.Env = conv("command", locCommand, "", srv.Command), mapped("env", locEnv, srv.Env)
			prev := ""
			for i, a := range srv.Args {
				c.Args = append(c.Args, conv("args."+strconv.Itoa(i), locArg, prev, a))
				prev = a
			}
		}
		if ok {
			p.set([]string{"mcp", srv.Name}, opencode.MCPDoc([]harness.MCPServer{c})[srv.Name])
		}
	}
}

func (ex *exporter) openCodePermission(p *patcher, existing json.RawMessage, as harness.AgentSettings) error {
	doc, notes, err := opencode.PermissionDoc(as.Allow, as.Deny)
	if err != nil {
		return ipcerr.BadRequest(err.Error())
	}
	ex.notes = append(ex.notes, notes...)
	if len(doc) == 0 {
		return nil
	}
	var cur map[string]json.RawMessage
	if existing != nil && json.Unmarshal(existing, &cur) != nil {
		ex.skip("permission", i18n.T("backend.core.agentPort.export.permissionShape.body", nil))
		return nil
	}
	for _, key := range sortedKeys(doc) {
		merged := doc[key]
		if old, ok := cur[key]; ok {
			m, err := opencode.MergePermission(old, doc[key])
			if err != nil {
				ex.skip("permission."+key, i18n.T("backend.core.agentPort.export.permissionShape.body", nil))
				continue
			}
			merged = m
		}
		p.set([]string{"permission", key}, merged)
	}
	return nil
}
