package agentport

import (
	"context"
	"encoding/json"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// claudeServerDoc is `claude mcp add-json`'s input. Values are references, never secrets.
type claudeServerDoc struct {
	Type    string            `json:"type"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type hookCmd struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

type hookGroup struct {
	Matcher string    `json:"matcher,omitempty"`
	Hooks   []hookCmd `json:"hooks"`
}

// renderedClaude is the interactive launch's settings.json as Kira renders it.
type renderedClaude struct {
	Model       string `json:"model"`
	Permissions *struct {
		Allow       []string `json:"allow"`
		Deny        []string `json:"deny"`
		DefaultMode string   `json:"defaultMode"`
	} `json:"permissions"`
	Hooks map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Type    string `json:"type"`
			Command string `json:"command"`
			Timeout int    `json:"timeout"`
		} `json:"hooks"`
	} `json:"hooks"`
	StatusLine json.RawMessage `json:"statusLine"`
}

func (s *Service) exportClaude(ctx context.Context, ex *exporter, sess *exportSession, files []target, configDir, agentDir string) (cliMissing bool, err error) {
	p, _ := s.o.Providers.Get(providerClaude)
	host, ok := harness.SettingsHostOf(p)
	if !ok {
		return false, ipcerr.BadRequest(i18n.T("backend.core.agentPort.unknownProvider.body", map[string]string{"provider": providerClaude}))
	}
	as, _, err := s.o.Settings.ForProvider(providerClaude)
	if err != nil {
		return false, ipcerr.InternalErr(err)
	}
	cfg, err := host.RenderSettings(as, harness.LaunchInteractive)
	if err != nil {
		return false, ipcerr.InternalErr(err)
	}
	var rendered renderedClaude
	for _, f := range cfg.Files {
		if f.Role == "settings" {
			if err := json.Unmarshal(f.Data, &rendered); err != nil {
				return false, ipcerr.InternalErr(err)
			}
		}
	}
	for _, t := range files {
		switch t.Role {
		case roleSettings:
			f, err := ex.readFor(t, 0o600, maxSettingsFile)
			if err != nil {
				return false, err
			}
			if err := ex.claudeSettingsPatch(f, rendered, cfg.Env); err != nil {
				return false, err
			}
			sess.files = append(sess.files, f)
		case roleMCP:
			real, err := resolveTarget(t.Path)
			if err != nil {
				return false, ipcerr.InternalErr(err)
			}
			sess.claudeJSON = real
		}
	}
	if configDir != "" {
		sess.cliEnv = []string{"CLAUDE_CONFIG_DIR=" + configDir}
	}
	return s.claudeServers(ctx, ex, sess, p, as.MCP, agentDir), nil
}

// claudeSettingsPatch computes the merged settings.json: Kira's keys set, lists unioned, nothing of the
// user's removed.
func (ex *exporter) claudeSettingsPatch(f *exportFile, r renderedClaude, env []string) error {
	var found map[string]json.RawMessage
	var others []string
	if len(strings.TrimSpace(string(f.before))) > 0 {
		var err error
		found, others, err = topLevel(f.before, "env", "permissions", "model", "statusLine", "hooks")
		if err != nil {
			return ipcerr.BadRequest(i18n.T("backend.core.agentPort.notJSON.body", map[string]string{"file": filepath.Base(f.path)}))
		}
	}
	p := newPatcher(found, others)
	if r.Model != "" {
		p.set([]string{"model"}, r.Model)
	}
	if perm := r.Permissions; perm != nil {
		if perm.DefaultMode != "" {
			p.set([]string{"permissions", "defaultMode"}, perm.DefaultMode)
		}
		for _, e := range perm.Allow {
			p.appendTo([]string{"permissions", "allow"}, e)
		}
		for _, e := range perm.Deny {
			p.appendTo([]string{"permissions", "deny"}, e)
		}
	}
	for _, event := range sortedKeys(r.Hooks) {
		for _, g := range r.Hooks[event] {
			group := hookGroup{Matcher: g.Matcher}
			for _, h := range g.Hooks {
				group.Hooks = append(group.Hooks, hookCmd(h))
			}
			if !p.hasHook(event, g.Matcher, group.Hooks[0].Command) {
				p.appendTo([]string{"hooks", event}, group)
			}
		}
	}
	if len(r.StatusLine) > 0 {
		var v any
		_ = json.Unmarshal(r.StatusLine, &v)
		p.set([]string{"statusLine"}, v)
	}
	for _, kvp := range env {
		k, v, _ := strings.Cut(kvp, "=")
		what := "env." + k
		switch {
		case hasSecret(classify(locEnv, k, v)):
			ex.skip(what, i18n.T("backend.core.agentPort.export.plaintext.body", nil))
		case strings.Contains(strings.ReplaceAll(v, "$${", ""), "${"):
			ex.skip(what, i18n.T("backend.core.agentPort.export.envNoExpansion.body", map[string]string{"name": k}))
		default:
			p.set([]string{"env", k}, strings.ReplaceAll(v, "$${", "${"))
		}
	}
	out, err := p.apply(f.before)
	if err != nil {
		return ipcerr.InternalErr(err)
	}
	f.after = out
	return nil
}

// hasHook reports whether the file already has a hook of the event with this matcher and command.
func (p *patcher) hasHook(event, matcher, command string) bool {
	list, _ := p.existing[ptr("hooks", event)].([]any)
	for _, g := range list {
		gm, _ := g.(map[string]any)
		if m, _ := gm["matcher"].(string); m != matcher {
			continue
		}
		hooks, _ := gm["hooks"].([]any)
		for _, h := range hooks {
			if hm, _ := h.(map[string]any); hm != nil && hm["command"] == command {
				return true
			}
		}
	}
	return false
}

// claudeServers computes the add-json document of each enabled server and whether it replaces one. It
// reports whether the claude CLI is missing.
func (s *Service) claudeServers(ctx context.Context, ex *exporter, sess *exportSession, p harness.Provider, servers []harness.MCPServer, agentDir string) bool {
	missing := p.MCPInstaller().Status().Path == ""
	if len(servers) == 0 {
		return missing
	}
	var existing []string
	if !missing {
		if list, err := p.UserMCPServers(ctx, agentDir); err == nil {
			for _, u := range list {
				existing = append(existing, u.Name)
			}
		}
	}
	for _, srv := range servers {
		doc, row, ok := ex.claudeServerDoc(srv)
		if !ok {
			continue
		}
		row.Action = "add"
		if slices.Contains(existing, srv.Name) {
			row.Action = "replace"
		}
		raw, _ := json.Marshal(doc)
		sess.servers = append(sess.servers, exportServer{row: row, json: raw})
	}
	return missing
}

func (ex *exporter) claudeServerDoc(srv harness.MCPServer) (claudeServerDoc, ExportServer, bool) {
	what := "mcp_servers." + srv.Name
	ok := true
	conv := func(field string, l loc, name, v string) string {
		out, good := ex.safe(what+"."+field, l, name, v, claudeRefs, "Claude")
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
	var doc claudeServerDoc
	row := ExportServer{Name: srv.Name}
	if srv.URL != "" {
		doc = claudeServerDoc{Type: "http", URL: conv("url", locURL, "", srv.URL), Headers: mapped("headers", locHeader, srv.Headers)}
		row.Transport = "http"
		if u, err := url.Parse(srv.URL); err == nil {
			row.Target = u.Host
		}
	} else {
		doc = claudeServerDoc{Type: "stdio", Command: conv("command", locCommand, "", srv.Command), Env: mapped("env", locEnv, srv.Env)}
		prev := ""
		for i, a := range srv.Args {
			doc.Args = append(doc.Args, conv("args."+strconv.Itoa(i), locArg, prev, a))
			prev = a
		}
		row.Transport = "stdio"
		row.Target = filepath.Base(srv.Command)
	}
	return doc, row, ok
}

// applyServers registers each server through the Claude CLI, stopping at the first failure. The user's
// .claude.json is backed up before the first call: the CLI rewrites it.
func (s *Service) applyServers(ctx context.Context, exp *exportSession, res *PortResult, now time.Time) {
	p, _ := s.o.Providers.Get(providerClaude)
	inst, ok := p.MCPInstaller().(harness.JSONInstaller)
	if !ok {
		return
	}
	if exp.claudeJSON != "" {
		if data, mode, exists, err := readIfExists(exp.claudeJSON, maxClaudeJSON); err == nil && exists {
			b, err := backup(exp.claudeJSON, data, mode, now)
			if err != nil {
				res.Servers = append(res.Servers, ServerResult{Outcome: harness.OutcomeInstallFailed, Detail: err.Error()})
				return
			}
			res.Backups = append(res.Backups, b)
		}
	}
	for _, srv := range exp.servers {
		r := inst.InstallJSON(ctx, srv.row.Name, srv.json, exp.cliEnv)
		res.Servers = append(res.Servers, ServerResult{Name: srv.row.Name, Outcome: r.Outcome, Detail: r.Detail})
		if r.Outcome != harness.OutcomeInstalled {
			return
		}
	}
}
