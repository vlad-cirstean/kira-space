package harnessflow_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/agentcfg"
	"github.com/kirathecat/kira-studio/internal/agentport"
	"github.com/kirathecat/kira-studio/internal/flowtest"
	"github.com/kirathecat/kira-studio/internal/harness/claude"
	"github.com/kirathecat/kira-studio/internal/harness/opencode"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// portLiterals are unique credential values seeded into the native files; none may land anywhere
// Kira keeps in plaintext.
type portLiterals map[string]string

func newPortLiterals(names ...string) portLiterals {
	l := portLiterals{}
	for _, n := range names {
		l[n] = "sk-p454-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	return l
}

func (l portLiterals) fill(text string) string {
	for n, v := range l {
		text = strings.ReplaceAll(text, "@"+n+"@", v)
	}
	return text
}

// seed copies a testdata/port fixture to path with the literals filled in.
func (l portLiterals) seed(t *testing.T, fixture, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "port", fixture))
	if err != nil {
		t.Fatal(err)
	}
	b := []byte(l.fill(string(raw)))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return b
}

// scan fails when any literal sits in a file Kira keeps under root, apart from the seeded native files
// and the fake agent's recordings.
func (l portLiterals) scan(t *testing.T, app *flowharness.App, skip ...string) {
	t.Helper()
	_ = filepath.WalkDir(app.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		for _, s := range append([]string{app.FakeDir, app.Work, app.Home}, skip...) {
			if strings.HasPrefix(p, s) {
				return nil
			}
		}
		b, _ := os.ReadFile(p)
		for n, v := range l {
			if bytes.Contains(b, []byte(v)) {
				t.Errorf("%s holds the %s literal", p, n)
			}
		}
		return nil
	})
}

func (l portLiterals) absent(t *testing.T, what string, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	for n, lit := range l {
		if strings.Contains(string(b), lit) {
			t.Fatalf("%s holds the %s literal", what, n)
		}
	}
}

func portItem(t *testing.T, items []agentport.Item, id string) agentport.Item {
	t.Helper()
	for _, it := range items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("no item %q in %v", id, portIDs(items))
	return agentport.Item{}
}

func portHas(items []agentport.Item, id string) bool {
	for _, it := range items {
		if it.ID == id {
			return true
		}
	}
	return false
}

func portIDs(items []agentport.Item) []string {
	var ids []string
	for _, it := range items {
		ids = append(ids, it.ID+"="+it.State)
	}
	return ids
}

func portItemByText(t *testing.T, items []agentport.Item, kind, contains string) agentport.Item {
	t.Helper()
	for _, it := range items {
		if it.Kind == kind && strings.Contains(it.Text, contains) {
			return it
		}
	}
	t.Fatalf("no %s item holding %q in %v", kind, contains, portIDs(items))
	return agentport.Item{}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func globOne(t *testing.T, pattern string) string {
	t.Helper()
	m, _ := filepath.Glob(pattern)
	if len(m) != 1 {
		t.Fatalf("glob %s = %v, want one match", pattern, m)
	}
	return m[0]
}

func selectAll(prev agentport.ImportPreview, skip ...string) agentport.ImportApplyArgs {
	args := agentport.ImportApplyArgs{Token: prev.Token}
	for _, it := range prev.Items {
		if it.State != agentport.StateNew && it.State != agentport.StateConflict {
			continue
		}
		if strings.Contains(strings.Join(skip, ","), it.ID) {
			continue
		}
		sel := agentport.ImportSelection{ID: it.ID}
		if it.Kind == "context" {
			sel.Mode = "append"
		}
		args.Items = append(args.Items, sel)
	}
	return args
}

// TestAgentPortClaude: Import reads Claude's user files into the shared settings, the secret store and
// the context; Export renders them back with a diff, backups and CLI-registered servers. Frontend
// half: ade-agent-port.spec.ts.
func TestAgentPortClaude(t *testing.T) {
	defer claude.SetMCPListTTL(0)()
	lit := newPortLiterals("JIRA", "APIKEY", "OAUTH", "GH", "NOTES")
	app := flowharness.New(t)
	svc, settings := app.W.AgentPortSvc, app.W.AgentSettingsSvc
	settingsJSON := filepath.Join(app.Home, ".claude", "settings.json")
	claudeJSON := filepath.Join(app.Home, ".claude.json")
	claudeMD := filepath.Join(app.Home, ".claude", "CLAUDE.md")
	seedSettings := lit.seed(t, "claude-settings.json", settingsJSON)
	seedClaude := lit.seed(t, "claude.json", claudeJSON)
	lit.seed(t, "CLAUDE.md", claudeMD)
	saveSettings(t, app, readFile(t, filepath.Join("testdata", "port", "settings.yaml")))
	saveContext(t, app, readFile(t, filepath.Join("testdata", "port", "context.md")))

	provs := svc.List(ctx)
	if len(provs) != 2 || provs[0].ID != "claude" || provs[1].ID != "opencode" || len(provs[0].Files) != 3 || !provs[0].Files[0].Exists {
		t.Fatalf("List = %+v, want claude with three files and opencode", provs)
	}

	// Import preview: states, slots, skips, and no literal anywhere in it.
	prev, err := svc.ImportPreview(ctx, agentport.ImportPreviewArgs{Provider: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	lit.absent(t, "the import preview", prev)
	want := map[string]string{
		"mcp/gh": "conflict", "mcp/notes": "new", "mcp/old": "skipped", "mcp/kira-db": "skipped",
		"env/NODE_ENV": "new", "env/JIRA_TOKEN": "new", "claude/model": "conflict", "claude/status_line": "new",
		"claude/permission_mode": "new", "context": "conflict", "allow/Read": "new", "deny/Bash(rm:*)": "new",
	}
	for id, state := range want {
		if got := portItem(t, prev.Items, id); got.State != state {
			t.Errorf("item %s state = %s, want %s", id, got.State, state)
		}
	}
	for _, absent := range []string{"mcp/proj", "env/apiKeyHelper", "skip/apiKeyHelper", "skip/oauthAccount", "skip/userID"} {
		if portHas(prev.Items, absent) {
			t.Errorf("item %s is listed, want it never read", absent)
		}
	}
	if !portHas(prev.Items, "skip/theme") || !portHas(prev.Items, "skip/permissions.ask") {
		t.Errorf("items = %v, want theme and permissions.ask skipped by name", portIDs(prev.Items))
	}
	if gh := portItem(t, prev.Items, "mcp/gh"); len(gh.Secrets) != 1 || gh.Secrets[0].Name != "gh-authorization" || !strings.Contains(gh.Text, "Bearer ${secret:gh-authorization}") || gh.Current == "" {
		t.Errorf("mcp/gh = %+v, want slot gh-authorization with the Bearer split and its current text", gh)
	}
	if n := portItem(t, prev.Items, "mcp/notes"); len(n.Secrets) != 1 || n.Secrets[0].Name != "notes-api_key" || !strings.Contains(n.Text, "${HOME}/notes") {
		t.Errorf("mcp/notes = %+v, want slot notes-api_key and the ${HOME} arg kept", n)
	}
	if j := portItem(t, prev.Items, "env/JIRA_TOKEN"); len(j.Secrets) != 1 || j.Secrets[0].Name != "env-jira_token" {
		t.Errorf("env/JIRA_TOKEN = %+v, want slot env-jira_token", j)
	}
	if ne := portItem(t, prev.Items, "env/NODE_ENV"); len(ne.Secrets) != 0 || !strings.Contains(ne.Text, "production") {
		t.Errorf("env/NODE_ENV = %+v, want plain text", ne)
	}
	if h := portItemByText(t, prev.Items, "hook", "event: tool_end"); !strings.Contains(h.Text, "matcher: Edit") {
		t.Errorf("PostToolUse hook = %+v, want the common tool_end hook", h)
	}
	if h := portItemByText(t, prev.Items, "native_hook", "event: SessionStart"); !strings.Contains(h.Text, "matcher: startup") {
		t.Errorf("SessionStart hook = %+v, want a claude hook", h)
	}
	if ctxItem := portItem(t, prev.Items, "context"); len(ctxItem.Modes) != 2 {
		t.Errorf("context = %+v, want modes replace and append", ctxItem)
	}
	app.Contract(t, "agent-port-import", "preview", prev, flowtest.Mask("token"))

	// Apply: gh replaced, model left, context appended.
	args := selectAll(prev, "claude/model")
	res, err := svc.ImportApply(ctx, args)
	if err != nil {
		t.Fatalf("ImportApply: %v", err)
	}
	lit.absent(t, "the import result", res)
	if _, err := svc.ImportApply(ctx, args); err == nil {
		t.Fatal("a second apply with the same token succeeded")
	}
	yaml := readFile(t, filepath.Join(app.SpaceHome, "agent", agentcfg.File))
	for _, want := range []string{"# kept by the import", "Bearer ${secret:gh-authorization}", "${secret:notes-api_key}", "${secret:env-jira_token}", "model: sonnet"} {
		if !strings.Contains(yaml, want) {
			t.Errorf("settings.yaml = %s, want %q", yaml, want)
		}
	}
	vals, _, err := app.W.AgentSecrets.Store.Open(secrets.ScopeAgentCredential, agentcfg.SecretOwner)
	if err != nil || vals["gh-authorization"] != lit["GH"] || vals["notes-api_key"] != lit["NOTES"] || vals["env-jira_token"] != lit["JIRA"] {
		t.Fatalf("saved secrets = %v, %v, want the seeded values", vals, err)
	}
	if cv, _ := app.W.AgentContextSvc.Get(); cv.Text != "Be brief.\n\nUse tabs.\n" {
		t.Fatalf("context.md = %q, want the append", cv.Text)
	}
	if got, _ := os.ReadFile(settingsJSON); !bytes.Equal(got, seedSettings) {
		t.Fatal("import changed ~/.claude/settings.json")
	}
	if got, _ := os.ReadFile(claudeJSON); !bytes.Equal(got, seedClaude) {
		t.Fatal("import changed ~/.claude.json")
	}
	again, err := svc.ImportPreview(ctx, agentport.ImportPreviewArgs{Provider: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"mcp/gh", "mcp/notes", "env/JIRA_TOKEN", "env/NODE_ENV", "claude/status_line", "allow/Read"} {
		if got := portItem(t, again.Items, id); got.State != "same" {
			t.Errorf("after apply %s = %s, want same", id, got.State)
		}
	}
	svc.Cancel(agentport.CancelArgs{Token: again.Token})
	lit.scan(t, app)

	// Export. Settings replaced by a small definition with a secret reference; settings.json is a symlink.
	if _, err := settings.SetSecret(agentcfg.SetSecretArgs{Name: "gh-token", Value: "hdr-" + lit["GH"]}); err != nil {
		t.Fatal(err)
	}
	saveSettings(t, app, `env:
  P454_E: ${secret:gh-token}
  PLAIN: x
permissions:
  allow:
    - Bash(ls)
mcp_servers:
  gh:
    url: https://api.example.com/mcp
    headers:
      Authorization: Bearer ${secret:gh-token}
  tool:
    command: /usr/bin/tool
    args: ["--home", "${HOME}"]
hooks:
  mark:
    event: tool_end
    matcher: Edit
    command: echo marked
claude:
  model: sonnet
`)
	dotfiles := filepath.Join(app.Home, "dotfiles", "settings.json")
	if err := os.MkdirAll(filepath.Dir(dotfiles), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(settingsJSON, dotfiles); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dotfiles, settingsJSON); err != nil {
		t.Fatal(err)
	}
	ep, err := svc.ExportPreview(ctx, agentport.ExportPreviewArgs{Provider: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	lit.absent(t, "the export preview", ep)
	if strings.Join(ep.EnvNames, ",") != "HOME,KIRA_SECRET_GH_TOKEN" {
		t.Errorf("envNames = %v, want HOME and KIRA_SECRET_GH_TOKEN", ep.EnvNames)
	}
	files := map[string]agentport.ExportFile{}
	for _, f := range ep.Files {
		files[f.Role] = f
	}
	sj := files["settings"]
	if !sj.Changed || !strings.Contains(sj.After, `"theme"`) || !strings.Contains(sj.After, "PLAIN") || strings.Contains(sj.After, "P454_E") ||
		!strings.Contains(sj.Before, "••••") || strings.Contains(sj.Before, lit["JIRA"]) || strings.Contains(sj.Before, lit["APIKEY"]) {
		t.Errorf("settings.json preview = %+v, want the merge with the user's keys, PLAIN added, P454_E skipped, credentials masked", sj)
	}
	if len(ep.Skipped) != 1 || ep.Skipped[0].What != "env.P454_E" {
		t.Errorf("skipped = %+v, want env.P454_E", ep.Skipped)
	}
	srv := map[string]string{}
	for _, s := range ep.Servers {
		srv[s.Name] = s.Action
	}
	if srv["gh"] != "replace" || srv["tool"] != "add" || len(srv) != 2 {
		t.Errorf("servers = %v, want gh replace and tool add", srv)
	}
	if !files["context"].Changed || files["context"].After != "Be brief.\n\nUse tabs.\n" {
		t.Errorf("context file = %+v, want CLAUDE.md replaced by context.md", files["context"])
	}
	app.Contract(t, "agent-port-export", "preview", ep, flowtest.Mask("token"))

	// A change between preview and apply is a conflict and writes nothing.
	if err := os.WriteFile(dotfiles, append(seedSettings, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = svc.ExportApply(ctx, agentport.ExportApplyArgs{Token: ep.Token, Files: []string{"settings", "context"}, Servers: true})
	var ie *ipcerr.Error
	if !errors.As(err, &ie) || ie.Code != string(ipcerr.CodeConflict) {
		t.Fatalf("ExportApply after an edit = %v, want a conflict", err)
	}
	if readFile(t, claudeMD) != "Use tabs.\n" {
		t.Fatal("a conflicting export wrote CLAUDE.md")
	}
	if err := os.WriteFile(dotfiles, seedSettings, 0o644); err != nil {
		t.Fatal(err)
	}
	ep, err = svc.ExportPreview(ctx, agentport.ExportPreviewArgs{Provider: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	res, err = svc.ExportApply(ctx, agentport.ExportApplyArgs{Token: ep.Token, Files: []string{"settings", "context"}, Servers: true})
	if err != nil {
		t.Fatalf("ExportApply: %v", err)
	}
	if st, err := os.Lstat(settingsJSON); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("settings.json is no longer a symlink: %v", err)
	}
	if got := readFile(t, dotfiles); !strings.Contains(got, `"theme"`) || !strings.Contains(got, "PLAIN") || !strings.Contains(got, `"Bash(ls)"`) {
		t.Errorf("settings.json = %s, want the merge", got)
	}
	if b := globOne(t, dotfiles+".kira-backup-*"); !bytes.Equal([]byte(readFile(t, b)), seedSettings) {
		t.Error("settings.json backup differs from the old bytes")
	}
	if b := globOne(t, claudeMD+".kira-backup-*"); readFile(t, b) != "Use tabs.\n" {
		t.Error("CLAUDE.md backup differs from the old bytes")
	}
	if b := globOne(t, claudeJSON+".kira-backup-*"); !bytes.Equal([]byte(readFile(t, b)), seedClaude) {
		t.Error(".claude.json backup differs from the old bytes")
	}
	if readFile(t, claudeMD) != "Be brief.\n\nUse tabs.\n" || len(res.Backups) != 3 || len(res.Servers) != 2 {
		t.Errorf("result = %+v, want CLAUDE.md replaced, three backups and two servers", res)
	}
	for _, name := range []string{"mcp-remove-1.args", "mcp-add-json-2.args"} {
		if _, err := os.Stat(filepath.Join(app.FakeDir, name)); err != nil {
			t.Errorf("the fake claude recorded no %s", name)
		}
	}
	add := fakeRead(t, app, "mcp-add-json-2.args")
	if !strings.Contains(add, "Bearer ${KIRA_SECRET_GH_TOKEN}") || strings.Contains(add, lit["GH"]) {
		t.Errorf("add-json argv = %q, want the env reference", add)
	}
	if !strings.Contains(readFile(t, claudeJSON), "${KIRA_SECRET_GH_TOKEN}") || !strings.Contains(readFile(t, claudeJSON), `"oauthAccount"`) {
		t.Error(".claude.json lacks the registered server or lost its other keys")
	}
	for _, f := range append(res.Written, res.Backups[0]) {
		if strings.Contains(readFile(t, f), "hdr-"+lit["GH"]) {
			t.Errorf("%s holds the secret value", f)
		}
	}

	// Round trip: the export's own output reads back as the same items.
	back, err := svc.ImportPreview(ctx, agentport.ImportPreviewArgs{Provider: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"mcp/gh", "mcp/tool", "env/PLAIN", "claude/model", "allow/Bash(ls)"} {
		if got := portItem(t, back.Items, id); got.State != "same" {
			t.Errorf("round trip %s = %s (%s), want same", id, got.State, got.Text)
		}
	}
	svc.Cancel(agentport.CancelArgs{Token: back.Token})
	lit.scan(t, app)
}

// TestAgentPortOpenCode: the same for OpenCode's JSONC config and AGENTS.md.
func TestAgentPortOpenCode(t *testing.T) {
	defer opencode.SetMCPListTTL(0)()
	lit := newPortLiterals("OCTOKEN", "OCHDR", "OCKEY")
	app := flowharness.New(t, flowharness.WithOpenCode())
	xdg := filepath.Join(app.Home, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	dir := filepath.Join(xdg, "opencode")
	cfg := filepath.Join(dir, "opencode.jsonc")
	agents := filepath.Join(dir, "AGENTS.md")
	seedCfg := lit.seed(t, "opencode.jsonc", cfg)
	lit.seed(t, "AGENTS.md", agents)
	svc := app.W.AgentPortSvc

	prev, err := svc.ImportPreview(ctx, agentport.ImportPreviewArgs{Provider: "opencode"})
	if err != nil {
		t.Fatal(err)
	}
	lit.absent(t, "the import preview", prev)
	want := map[string]string{
		"mcp/local1": "new", "mcp/remote1": "new", "opencode/model": "new", "opencode/plugin/file:///opt/plugins/a.js": "new",
		"opencode/plugin/opencode-foo": "skipped", "deny/Edit": "new", "deny/Bash(git push:*)": "new", "allow/Bash(git status)": "new",
		"skip/permission.webfetch.*": "skipped", "skip/provider": "skipped", "skip/instructions": "skipped", "context": "new",
	}
	for id, state := range want {
		if got := portItem(t, prev.Items, id); got.State != state {
			t.Errorf("item %s state = %s, want %s", id, got.State, state)
		}
	}
	if l := portItem(t, prev.Items, "mcp/local1"); len(l.Secrets) != 1 || l.Secrets[0].Name != "local1-token" || !strings.Contains(l.Text, "${HOME}") || !strings.Contains(l.Text, "command: npx") {
		t.Errorf("mcp/local1 = %+v, want slot local1-token, {env:HOME} as ${HOME} and the command split", l)
	}
	if r := portItem(t, prev.Items, "mcp/remote1"); len(r.Secrets) != 1 || !strings.Contains(r.Text, "enabled: false") {
		t.Errorf("mcp/remote1 = %+v, want a slot and enabled false", r)
	}
	if notes := strings.Join(prev.Notes, "\n"); !strings.Contains(notes, "P457_UNSET_ORG is not set") || strings.Contains(notes, "HOME is not set") {
		t.Errorf("import notes = %q, want one for the unset P457_UNSET_ORG only", notes)
	}
	app.Contract(t, "agent-port-import", "opencode", prev, flowtest.Mask("token"))
	if _, err := svc.ImportApply(ctx, selectAll(prev)); err != nil {
		t.Fatalf("ImportApply: %v", err)
	}
	if got, _ := os.ReadFile(cfg); !bytes.Equal(got, seedCfg) {
		t.Fatal("import changed opencode.jsonc")
	}
	vals, _, _ := app.W.AgentSecrets.Store.Open(secrets.ScopeAgentCredential, agentcfg.SecretOwner)
	if vals["local1-token"] != lit["OCTOKEN"] || vals["remote1-authorization"] != lit["OCHDR"] {
		t.Fatalf("saved secrets = %v, want the seeded values", vals)
	}
	lit.scan(t, app)

	// Export: the comment stays, the secret becomes {env:KIRA_SECRET_...}, permissions merge in order.
	if _, err := app.W.AgentSettingsSvc.SetSecret(agentcfg.SetSecretArgs{Name: "oc-token", Value: "tok-" + lit["OCTOKEN"]}); err != nil {
		t.Fatal(err)
	}
	saveSettings(t, app, `permissions:
  deny:
    - Bash(rm:*)
mcp_servers:
  fresh:
    url: https://fresh.example.com/mcp
    headers:
      Authorization: Bearer ${secret:oc-token}
opencode:
  model: anthropic/claude-opus-4-1
`)
	ep, err := svc.ExportPreview(ctx, agentport.ExportPreviewArgs{Provider: "opencode"})
	if err != nil {
		t.Fatal(err)
	}
	lit.absent(t, "the export preview", ep)
	var cf agentport.ExportFile
	for _, f := range ep.Files {
		if f.Role == "config" {
			cf = f
		}
	}
	for _, want := range []string{"// user comment", "{env:KIRA_SECRET_OC_TOKEN}", `"rm*"`, "claude-opus-4-1"} {
		if !strings.Contains(cf.After, want) {
			t.Errorf("config after = %s, want %q", cf.After, want)
		}
	}
	if strings.Contains(cf.Before, lit["OCTOKEN"]) || strings.Contains(cf.Before, lit["OCKEY"]) || !strings.Contains(cf.Before, "••••") {
		t.Errorf("config before = %s, want the credentials masked", cf.Before)
	}
	if notes := strings.Join(ep.Notes, "\n"); !strings.Contains(notes, "KIRA_SECRET_OC_TOKEN is not set") {
		t.Errorf("export notes = %q, want the unset secret variable named", notes)
	}
	app.Contract(t, "agent-port-export", "opencode", ep, flowtest.Mask("token"))
	res, err := svc.ExportApply(ctx, agentport.ExportApplyArgs{Token: ep.Token, Files: []string{"config", "context"}})
	if err != nil {
		t.Fatalf("ExportApply: %v", err)
	}
	if b := globOne(t, cfg+".kira-backup-*"); !bytes.Equal([]byte(readFile(t, b)), seedCfg) {
		t.Error("opencode.jsonc backup differs from the old bytes")
	}
	if got := readFile(t, cfg); !strings.Contains(got, "// user comment") || strings.Contains(got, "hdr-") || len(res.Written) == 0 {
		t.Errorf("opencode.jsonc = %s, want the comment kept", got)
	}
	lit.scan(t, app)
}
