package harnessflow_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tailscale/hujson"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/agentcfg"
	"github.com/kirathecat/kira-studio/internal/agentport"
	"github.com/kirathecat/kira-studio/internal/flowtest"
	"github.com/kirathecat/kira-studio/internal/harness/claude"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// journeyLiterals are shape-free credential values (no sk- prefix, not hex): only the location
// (env, headers) and the name classify them, never the value's shape.
func journeyLiterals(names ...string) portLiterals {
	l := portLiterals{}
	for _, n := range names {
		l[n] = "p635-" + uuid.NewString()
	}
	return l
}

// jsonFile parses a native JSON file the way the providers do (JSONC tolerated).
func jsonFile(t *testing.T, path string) map[string]any {
	t.Helper()
	std, err := hujson.Standardize([]byte(readFile(t, path)))
	if err != nil {
		t.Fatalf("%s does not parse: %v", path, err)
	}
	var m map[string]any
	if err := json.Unmarshal(std, &m); err != nil {
		t.Fatalf("%s does not parse: %v", path, err)
	}
	return m
}

// jsonAt walks dotted keys of a decoded document.
func jsonAt(t *testing.T, doc map[string]any, path ...string) any {
	t.Helper()
	var cur any = doc
	for _, k := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("no %s in the document", strings.Join(path, "."))
		}
		cur = m[k]
	}
	return cur
}

func jsonEqual(t *testing.T, what string, got any, want string) {
	t.Helper()
	g, _ := json.Marshal(got)
	var w any
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatal(err)
	}
	wb, _ := json.Marshal(w)
	if string(g) != string(wb) {
		t.Errorf("%s = %s, want %s", what, g, wb)
	}
}

// editText replaces old with new once; a miss fails the test.
func editText(t *testing.T, text, old, new string) string {
	t.Helper()
	if strings.Count(text, old) != 1 {
		t.Fatalf("settings text holds %q %d times, want once:\n%s", old, strings.Count(text, old), text)
	}
	return strings.Replace(text, old, new, 1)
}

func wantBadRequest(t *testing.T, what string, err error) {
	t.Helper()
	var ie *ipcerr.Error
	if !errors.As(err, &ie) || ie.Code != string(ipcerr.CodeBadRequest) {
		t.Fatalf("%s = %v, want a bad request", what, err)
	}
}

var stampRe = regexp.MustCompile(`\d{8}T\d{6}Z(-\d+)?`)

// stable gives the contract run-independent backup stamps: entry i of n, newest first, is i seconds
// before a fixed time.
func stableBackups(list []agentport.Backup) []agentport.Backup {
	out := make([]agentport.Backup, len(list))
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i, b := range list {
		at := base.Add(-time.Duration(i) * time.Second)
		b.Taken = at.Format(time.RFC3339)
		b.File = stampRe.ReplaceAllString(b.File, at.Format("20060102T150405Z"))
		out[i] = b
	}
	return out
}

func stableResult(r agentport.PortResult) agentport.PortResult {
	for i, b := range r.Backups {
		r.Backups[i] = stampRe.ReplaceAllString(b, "20260101T120000Z")
	}
	return r
}

func backupBytes(t *testing.T, real string, b agentport.Backup) []byte {
	t.Helper()
	return []byte(readFile(t, filepath.Join(filepath.Dir(real), b.File)))
}

func backupsOf(t *testing.T, app *flowharness.App, provider, role string) []agentport.Backup {
	t.Helper()
	all, err := app.W.AgentPortSvc.Backups(ctx, agentport.BackupsArgs{Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	var out []agentport.Backup
	for _, b := range all {
		if b.Role == role {
			out = append(out, b)
		}
	}
	return out
}

func countBackups(t *testing.T, app *flowharness.App, provider string) int {
	t.Helper()
	all, err := app.W.AgentPortSvc.Backups(ctx, agentport.BackupsArgs{Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	return len(all)
}

// TestAgentPortClaudeJourney is J-HAR-002: import from a user Claude config (hooks, MCP servers with
// secrets, statusline), preview masked, apply, export back after edits, list the backups, undo
// through one. Frontend half: ade-agent-port.spec.ts, "journey".
func TestAgentPortClaudeJourney(t *testing.T) {
	defer claude.SetMCPListTTL(0)()
	lit := journeyLiterals("JIRA", "APIKEY", "OAUTH", "GH", "NOTES", "TEAM")
	app := flowharness.New(t, flowharness.WithOpenCode())
	svc := app.W.AgentPortSvc
	dotfiles := filepath.Join(app.Home, "dotfiles", "settings.json")
	settingsJSON := filepath.Join(app.Home, ".claude", "settings.json")
	claudeJSON := filepath.Join(app.Home, ".claude.json")
	claudeMD := filepath.Join(app.Home, ".claude", "CLAUDE.md")
	seedSettings := lit.seed(t, "journey-settings.json", dotfiles)
	if err := os.MkdirAll(filepath.Dir(settingsJSON), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dotfiles, settingsJSON); err != nil {
		t.Fatal(err)
	}
	seedClaude := lit.seed(t, "journey-claude.json", claudeJSON)
	seedMD := lit.seed(t, "journey-CLAUDE.md", claudeMD)
	yamlPath := filepath.Join(app.ConfigDir("agent"), agentcfg.File)

	// 1. Import preview: everything new, secrets in slots, none in the preview.
	prev, err := svc.ImportPreview(ctx, agentport.ImportPreviewArgs{Provider: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	lit.absent(t, "the import preview", prev)
	for _, id := range []string{
		"env/JIRA_TOKEN", "env/NODE_ENV", "claude/model", "claude/permission_mode", "claude/status_line",
		"allow/Bash(git status)", "allow/Read", "deny/Bash(rm:*)", "hook/notification-1", "hook/pretooluse-1",
		"claude.hook/sessionstart-1", "mcp/gh", "mcp/notes", "mcp/hdr", "context",
	} {
		if got := portItem(t, prev.Items, id); got.State != "new" {
			t.Errorf("item %s = %s, want new", id, got.State)
		}
	}
	if got := portItem(t, prev.Items, "hook-skip/Stop/prompt"); got.State != "skipped" {
		t.Errorf("Stop prompt hook = %s, want skipped", got.State)
	}
	for id, slot := range map[string]string{
		"env/JIRA_TOKEN": "env-jira_token", "mcp/gh": "gh-authorization", "mcp/notes": "notes-api_key", "mcp/hdr": "hdr-x-team",
	} {
		it := portItem(t, prev.Items, id)
		if len(it.Secrets) != 1 || it.Secrets[0].Name != slot || !strings.Contains(it.Text, "${secret:"+slot+"}") {
			t.Errorf("%s = %+v, want the single slot %s", id, it, slot)
		}
	}
	app.Contract(t, "agent-port-journey", "import", prev, flowtest.Mask("token"))

	// 2. Apply all: no literal in the result or settings.yaml, literals sealed, native files untouched.
	args := selectAll(prev)
	for i, sel := range args.Items {
		if sel.ID == "context" {
			args.Items[i].Mode = "replace"
		}
	}
	res, err := svc.ImportApply(ctx, args)
	if err != nil {
		t.Fatalf("ImportApply: %v", err)
	}
	lit.absent(t, "the import result", res)
	yaml := readFile(t, yamlPath)
	for n, v := range lit {
		if strings.Contains(yaml, v) {
			t.Errorf("settings.yaml holds the %s literal", n)
		}
	}
	for _, want := range []string{
		"${secret:env-jira_token}", "${secret:gh-authorization}", "${secret:notes-api_key}", "${secret:hdr-x-team}",
		"permission_mode: plan", "model: opus", "command: ~/bin/line.sh", "padding: 2", "refresh_interval: 5",
		"event: tool_start", "event: notification", "event: SessionStart",
	} {
		if !strings.Contains(yaml, want) {
			t.Errorf("settings.yaml lacks %q:\n%s", want, yaml)
		}
	}
	vals, _, err := app.W.AgentSecrets.Store.Open(secrets.ScopeAgentCredential, agentcfg.SecretOwner)
	if err != nil || vals["env-jira_token"] != lit["JIRA"] || vals["gh-authorization"] != "Bearer "+lit["GH"] && vals["gh-authorization"] != lit["GH"] ||
		vals["notes-api_key"] != lit["NOTES"] || vals["hdr-x-team"] != lit["TEAM"] {
		t.Fatalf("sealed secrets = %v, %v, want each seeded literal in its slot", vals, err)
	}
	for path, seed := range map[string][]byte{dotfiles: seedSettings, claudeJSON: seedClaude, claudeMD: seedMD} {
		if got, _ := os.ReadFile(path); !bytes.Equal(got, seed) {
			t.Errorf("import changed %s", filepath.Base(path))
		}
	}

	// 3. Edit in Kira.
	cur, err := app.W.AgentSettingsSvc.Get()
	if err != nil {
		t.Fatal(err)
	}
	text := cur.Text
	text = editText(t, text, "model: opus\n", "model: sonnet\n")
	text = editText(t, text, "    - Read\n", "    - Read\n    - Bash(go test:*)\n")
	text = editText(t, text, "padding: 2\n", "padding: 3\n")
	text = editText(t, text, "NODE_ENV: production\n", "NODE_ENV: development\n")
	text = editText(t, text, "command: echo notify\n", "command: echo notified\n")
	text = editText(t, text, "claude:\n  permission_mode", "  fmt:\n    event: tool_end\n    matcher: Edit\n    command: gofmt -w\nclaude:\n  permission_mode")
	text = editText(t, text, "hooks:\n  notification-1:", "  tool:\n    command: /usr/bin/tool\n    args: [\"--home\", \"${HOME}\"]\nhooks:\n  notification-1:")
	saveSettings(t, app, text)
	saveContext(t, app, "Be terse.\n")
	editedYAML := readFile(t, yamlPath)

	// 4. Export preview.
	ep, err := svc.ExportPreview(ctx, agentport.ExportPreviewArgs{Provider: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	lit.absent(t, "the export preview", ep)
	files := map[string]agentport.ExportFile{}
	for _, f := range ep.Files {
		files[f.Role] = f
	}
	sj := files["settings"]
	if !strings.Contains(sj.Before, "••••") {
		t.Errorf("settings Before = %s, want credentials masked", sj.Before)
	}
	for _, want := range []string{`"refreshInterval":5`, `"padding":3`, `"Bash(go test:*)"`, `"model": "sonnet"`} {
		if !strings.Contains(sj.After, want) {
			t.Errorf("settings After lacks %s:\n%s", want, sj.After)
		}
	}
	var skippedJira bool
	for _, s := range ep.Skipped {
		if s.What == "env.JIRA_TOKEN" && s.Reason != "" {
			skippedJira = true
		}
	}
	if !skippedJira {
		t.Errorf("skipped = %+v, want env.JIRA_TOKEN with a reason", ep.Skipped)
	}
	srv := map[string]string{}
	for _, s := range ep.Servers {
		srv[s.Name] = s.Action
	}
	if len(srv) != 4 || srv["gh"] != "replace" || srv["notes"] != "replace" || srv["hdr"] != "replace" || srv["tool"] != "add" {
		t.Errorf("servers = %v, want gh, notes and hdr replaced and tool added", srv)
	}
	for _, want := range []string{"HOME", "KIRA_SECRET_GH_AUTHORIZATION", "KIRA_SECRET_NOTES_API_KEY", "KIRA_SECRET_HDR_X_TEAM"} {
		if !slices.Contains(ep.EnvNames, want) {
			t.Errorf("envNames = %v, want %s", ep.EnvNames, want)
		}
	}
	app.Contract(t, "agent-port-journey", "export", ep, flowtest.Mask("token"))

	// 5. Export apply.
	if _, err := svc.ExportApply(ctx, agentport.ExportApplyArgs{Token: ep.Token, Files: []string{"settings", "context"}, Servers: true}); err != nil {
		t.Fatalf("ExportApply: %v", err)
	}
	if st, err := os.Lstat(settingsJSON); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("settings.json is no longer a symlink: %v", err)
	}
	exported := jsonFile(t, dotfiles)
	cj := jsonFile(t, claudeJSON)
	jsonEqual(t, "model", jsonAt(t, exported, "model"), `"sonnet"`)
	jsonEqual(t, "permissions.defaultMode", jsonAt(t, exported, "permissions", "defaultMode"), `"plan"`)
	jsonEqual(t, "permissions.allow", jsonAt(t, exported, "permissions", "allow"), `["Bash(git status)","Read","Bash(go test:*)"]`)
	jsonEqual(t, "permissions.deny", jsonAt(t, exported, "permissions", "deny"), `["Bash(rm:*)"]`)
	jsonEqual(t, "statusLine", jsonAt(t, exported, "statusLine"), `{"type":"command","command":"~/bin/line.sh","padding":3,"refreshInterval":5}`)
	jsonEqual(t, "env.NODE_ENV", jsonAt(t, exported, "env", "NODE_ENV"), `"development"`)
	hook := func(event, matcher, command string, timeout int) {
		t.Helper()
		inner := fmt.Sprintf(`{"type":"command","command":%q,"timeout":%d}`, command, timeout)
		if timeout == 0 {
			inner = fmt.Sprintf(`{"type":"command","command":%q}`, command)
		}
		group := fmt.Sprintf(`{"matcher":%q,"hooks":[%s]}`, matcher, inner)
		if matcher == "" {
			group = fmt.Sprintf(`{"hooks":[%s]}`, inner)
		}
		groups, _ := jsonAt(t, exported, "hooks", event).([]any)
		n := 0
		for _, g := range groups {
			gb, _ := json.Marshal(g)
			var a, b any
			_ = json.Unmarshal(gb, &a)
			_ = json.Unmarshal([]byte(group), &b)
			if fmt.Sprint(a) == fmt.Sprint(b) { // map printing sorts keys
				n++
			}
		}
		if n != 1 {
			t.Errorf("hooks.%s holds %s %d times, want once: %v", event, group, n, groups)
		}
	}
	hook("PreToolUse", "Bash", "echo pre", 15)
	hook("Notification", "", "echo notified", 60)
	// A hook the file already has (same matcher and command) is left as the user wrote it.
	hook("SessionStart", "startup", "echo started", 0)
	hook("PostToolUse", "Edit", "gofmt -w", 60)
	// Union rule: the old command stays in the user's file after an edit in Kira (designed, P454).
	hook("Notification", "", "echo notify", 0)
	jsonEqual(t, "theme", jsonAt(t, exported, "theme"), `"dark"`)
	if jsonAt(t, exported, "apiKeyHelper") != "echo "+lit["APIKEY"] || jsonAt(t, exported, "env", "JIRA_TOKEN") != lit["JIRA"] {
		t.Error("export dropped the user's own keys")
	}
	servers := jsonAt(t, cj, "mcpServers").(map[string]any)
	jsonEqual(t, "mcpServers.gh.headers", servers["gh"].(map[string]any)["headers"], `{"Authorization":"Bearer ${KIRA_SECRET_GH_AUTHORIZATION}"}`)
	jsonEqual(t, "mcpServers.notes.env", servers["notes"].(map[string]any)["env"], `{"API_KEY":"${KIRA_SECRET_NOTES_API_KEY}"}`)
	jsonEqual(t, "mcpServers.hdr.headers", servers["hdr"].(map[string]any)["headers"], `{"X-Team":"${KIRA_SECRET_HDR_X_TEAM}"}`)
	jsonEqual(t, "mcpServers.tool.args", servers["tool"].(map[string]any)["args"], `["--home","${HOME}"]`)
	if jsonAt(t, cj, "oauthAccount", "token") != lit["OAUTH"] {
		t.Error("export lost oauthAccount")
	}
	// A credential is never copied into a new key: a literal occurs in an exported file at most as
	// often as in its seed.
	for n, v := range lit {
		if got, was := strings.Count(readFile(t, dotfiles), v), bytes.Count(seedSettings, []byte(v)); got != was {
			t.Errorf("settings.json holds the %s literal %d times, seed %d", n, got, was)
		}
		if got, was := strings.Count(readFile(t, claudeJSON), v), bytes.Count(seedClaude, []byte(v)); got > was {
			t.Errorf(".claude.json holds the %s literal %d times, seed %d", n, got, was)
		}
	}
	if readFile(t, yamlPath) != editedYAML {
		t.Error("export changed settings.yaml")
	}
	entries, _ := os.ReadDir(app.FakeDir)
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(app.FakeDir, e.Name()))
		for n, v := range lit {
			if bytes.Contains(b, []byte(v)) {
				t.Errorf("fake claude recording %s holds the %s literal", e.Name(), n)
			}
		}
	}
	back, err := svc.ImportPreview(ctx, agentport.ImportPreviewArgs{Provider: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range back.Items {
		if it.State == "same" || it.State == "skipped" {
			continue
		}
		// The only leftover is the pre-edit hook the union keeps in the user's file.
		if it.ID != "hook/notification-2" || it.State != "new" || !strings.Contains(it.Text, "command: echo notify") {
			t.Errorf("round trip: %s = %s, want same\n%s", it.ID, it.State, it.Text)
		}
	}
	svc.Cancel(agentport.CancelArgs{Token: back.Token})

	// 6. Backups listed: one per file the export wrote or the CLI rewrote, holding the old bytes.
	list, err := svc.Backups(ctx, agentport.BackupsArgs{Provider: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 || list[0].Role != "settings" || list[1].Role != "context" || list[2].Role != "mcp" {
		t.Fatalf("backups = %+v, want settings, context and mcp", list)
	}
	for i, want := range []struct {
		real string
		seed []byte
	}{{dotfiles, seedSettings}, {claudeMD, seedMD}, {claudeJSON, seedClaude}} {
		if got := backupBytes(t, want.real, list[i]); !bytes.Equal(got, want.seed) {
			t.Errorf("%s backup differs from the seed", list[i].Role)
		}
		if list[i].Size != int64(len(want.seed)) {
			t.Errorf("%s backup size = %d, want %d", list[i].Role, list[i].Size, len(want.seed))
		}
	}
	if list[0].Target != "~/.claude/settings.json" || list[0].File == "" {
		t.Errorf("settings backup = %+v, want the link's display path", list[0])
	}
	if !list[0].Restorable || !list[1].Restorable || list[2].Restorable || list[2].Reason == "" {
		t.Errorf("backups = %+v, want only the .claude.json one not restorable, with a reason", list)
	}
	firstExport := []byte(readFile(t, dotfiles))

	// A second export in the same second: distinct names, newest first, nothing overwritten.
	cur, _ = app.W.AgentSettingsSvc.Get()
	saveSettings(t, app, editText(t, cur.Text, "model: sonnet\n", "model: haiku\n"))
	ep2, err := svc.ExportPreview(ctx, agentport.ExportPreviewArgs{Provider: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExportApply(ctx, agentport.ExportApplyArgs{Token: ep2.Token, Files: []string{"settings"}}); err != nil {
		t.Fatal(err)
	}
	sb := backupsOf(t, app, "claude", "settings")
	if len(sb) != 2 || sb[0].File == sb[1].File {
		t.Fatalf("settings backups = %+v, want two distinct", sb)
	}
	if !bytes.Equal(backupBytes(t, dotfiles, sb[0]), firstExport) || !bytes.Equal(backupBytes(t, dotfiles, sb[1]), seedSettings) {
		t.Error("settings backups: want the newest to hold the first export and the older the seed")
	}
	list, _ = svc.Backups(ctx, agentport.BackupsArgs{Provider: "claude"})
	app.Contract(t, "agent-port-journey", "backups", stableBackups(list))
	secondExport := []byte(readFile(t, dotfiles))

	// 7. Undo through a backup.
	res, err = svc.Restore(ctx, agentport.RestoreArgs{Provider: "claude", File: sb[1].File})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got, _ := os.ReadFile(dotfiles); !bytes.Equal(got, seedSettings) {
		t.Fatal("restore left settings.json different from the seed")
	}
	if st, err := os.Lstat(settingsJSON); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("restore replaced the settings.json symlink: %v", err)
	}
	if len(res.Written) != 1 || res.Written[0] != dotfiles || len(res.Backups) != 1 || !bytes.Equal([]byte(readFile(t, res.Backups[0])), secondExport) {
		t.Errorf("restore result = %+v, want the link target written and the pre-restore bytes backed up", res)
	}
	app.Contract(t, "agent-port-journey", "restore", stableResult(res))
	cb := backupsOf(t, app, "claude", "context")
	if _, err := svc.Restore(ctx, agentport.RestoreArgs{Provider: "claude", File: cb[0].File}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(claudeMD); !bytes.Equal(got, seedMD) {
		t.Error("restore left CLAUDE.md different from the seed")
	}
	n := countBackups(t, app, "claude")
	again, err := svc.Restore(ctx, agentport.RestoreArgs{Provider: "claude", File: cb[0].File})
	if err != nil || len(again.Written) != 0 || len(again.Backups) != 0 || len(again.Notes) != 1 || countBackups(t, app, "claude") != n {
		t.Errorf("second restore = %+v, %v, want a note and nothing written", again, err)
	}
	after, err := svc.ImportPreview(ctx, agentport.ImportPreviewArgs{Provider: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"claude/model", "claude/status_line"} {
		if got := portItem(t, after.Items, id); got.State != "conflict" {
			t.Errorf("after restore %s = %s, want conflict", id, got.State)
		}
	}
	svc.Cancel(agentport.CancelArgs{Token: after.Token})

	// Refusals write nothing and add no backup.
	dir := filepath.Dir(dotfiles)
	decoy := filepath.Join(dir, "settings.json.kira-backup-20200101T000000Z")
	if err := os.Symlink(claudeJSON, decoy); err != nil {
		t.Fatal(err)
	}
	mcp := backupsOf(t, app, "claude", "mcp")[0]
	n = countBackups(t, app, "claude")
	beforeRefusals := []byte(readFile(t, dotfiles))
	for what, args := range map[string]agentport.RestoreArgs{
		"the .claude.json backup": {Provider: "claude", File: mcp.File},
		"an unknown name":         {Provider: "claude", File: "settings.json.kira-backup-19990101T000000Z"},
		"a parent path":           {Provider: "claude", File: "../" + sb[0].File},
		"an absolute path":        {Provider: "claude", File: filepath.Join(dir, sb[0].File)},
		"another provider":        {Provider: "opencode", File: sb[0].File},
		"a symlink":               {Provider: "claude", File: filepath.Base(decoy)},
	} {
		_, err := svc.Restore(ctx, args)
		wantBadRequest(t, what, err)
		if got, _ := os.ReadFile(dotfiles); !bytes.Equal(got, beforeRefusals) {
			t.Errorf("%s changed settings.json", what)
		}
	}
	if got := countBackups(t, app, "claude"); got != n {
		t.Errorf("backups after refusals = %d, want %d (the decoy symlink is not listed)", got, n)
	}
	if got, _ := os.ReadFile(claudeJSON); bytes.Contains(got, []byte("kira-backup")) {
		t.Error(".claude.json changed")
	}

	// 8. No literal anywhere Kira keeps.
	lit.scan(t, app)
}

// TestAgentPortExportProbes: a status line with padding 0 survives import and export, and an export
// with no CLAUDE.md on disk takes no backup of it and does not fail.
func TestAgentPortExportProbes(t *testing.T) {
	defer claude.SetMCPListTTL(0)()
	app := flowharness.New(t)
	svc := app.W.AgentPortSvc
	settingsJSON := filepath.Join(app.Home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsJSON), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsJSON, []byte(`{"statusLine":{"type":"command","command":"echo line","padding":0}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	prev, err := svc.ImportPreview(ctx, agentport.ImportPreviewArgs{Provider: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if it := portItem(t, prev.Items, "claude/status_line"); !strings.Contains(it.Text, "padding: 0") {
		t.Fatalf("status_line = %q, want padding 0 kept", it.Text)
	}
	if _, err := svc.ImportApply(ctx, selectAll(prev)); err != nil {
		t.Fatal(err)
	}
	saveContext(t, app, "Be terse.\n")
	ep, err := svc.ExportPreview(ctx, agentport.ExportPreviewArgs{Provider: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExportApply(ctx, agentport.ExportApplyArgs{Token: ep.Token, Files: []string{"settings", "context"}}); err != nil {
		t.Fatal(err)
	}
	jsonEqual(t, "statusLine", jsonAt(t, jsonFile(t, settingsJSON), "statusLine"), `{"type":"command","command":"echo line","padding":0}`)
	if got := readFile(t, filepath.Join(app.Home, ".claude", "CLAUDE.md")); got != "Be terse.\n" {
		t.Errorf("CLAUDE.md = %q, want the context", got)
	}
	if got := backupsOf(t, app, "claude", "context"); len(got) != 0 {
		t.Errorf("context backups = %+v, want none for a new file", got)
	}
}
