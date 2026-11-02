package claude

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/internal/agentcfg"
	"github.com/kirathecat/kira-studio/internal/agentport"
)

// TestAgentPortClaude: Import and Export against the real CLI's own user config. The real
// `claude mcp add-json -s user` seeds a server with a literal header; Import stores the literal as a
// secret; Export registers the server again through the CLI with an env reference, which the real
// `claude mcp list` then resolves. No model call.
func TestAgentPortClaude(t *testing.T) {
	app := newRealApp(t)
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	literal := "tok-" + hex.EncodeToString(b)
	home := app.Home
	claudeJSON := filepath.Join(home, ".claude.json")
	settingsJSON := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsJSON), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"theme":"dark","env":{"P454_PLAIN":"x"},"permissions":{"allow":["Read"]}}` + "\n"
	if err := os.WriteFile(settingsJSON, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	server := `{"type":"http","url":"https://api.example.com/mcp","headers":{"Authorization":"Bearer ` + literal + `"}}`
	if out, errOut, err := app.run(t, app.Work, nil, "mcp", "add-json", "-s", "user", "gh", server); err != nil {
		t.Fatalf("claude mcp add-json: %v\n%s%s", err, out, errOut)
	}

	svc := app.W.AgentPortSvc
	prev, err := svc.ImportPreview(ctx, agentport.ImportPreviewArgs{Provider: "claude"})
	if err != nil {
		t.Fatalf("ImportPreview: %v", err)
	}
	var gh *agentport.Item
	for i := range prev.Items {
		if prev.Items[i].ID == "mcp/gh" {
			gh = &prev.Items[i]
		}
	}
	if gh == nil || gh.State != agentport.StateNew || len(gh.Secrets) != 1 || strings.Contains(gh.Text, literal) {
		t.Fatalf("mcp/gh = %+v, want a new item with one secret slot and no literal", gh)
	}
	if _, err := svc.ImportApply(ctx, agentport.ImportApplyArgs{Token: prev.Token, Items: []agentport.ImportSelection{{ID: "mcp/gh"}, {ID: "env/P454_PLAIN"}}}); err != nil {
		t.Fatalf("ImportApply: %v", err)
	}
	settingsYAML, err := os.ReadFile(filepath.Join(app.SpaceHome, "agent", "settings.yaml"))
	if err != nil || strings.Contains(string(settingsYAML), literal) || !strings.Contains(string(settingsYAML), "${secret:gh-authorization}") {
		t.Fatalf("settings.yaml = %s, %v, want the reference and no literal", settingsYAML, err)
	}

	// A rule only Kira has, so the export changes settings.json.
	cur, err := app.W.AgentSettingsSvc.Get()
	if err != nil {
		t.Fatal(err)
	}
	if v, err := app.W.AgentSettingsSvc.Save(agentcfg.SaveArgs{Text: cur.Text + "permissions:\n  deny:\n    - Bash(rm:*)\n", BaseHash: cur.Hash}); err != nil || v.Problem != nil {
		t.Fatalf("Save = %+v, %v", v, err)
	}
	ep, err := svc.ExportPreview(ctx, agentport.ExportPreviewArgs{Provider: "claude"})
	if err != nil {
		t.Fatalf("ExportPreview: %v", err)
	}
	if ep.CLIMissing || len(ep.Servers) != 1 || ep.Servers[0].Name != "gh" || ep.Servers[0].Action != "replace" {
		t.Fatalf("export preview = %+v, want gh replace through the CLI", ep)
	}
	res, err := svc.ExportApply(ctx, agentport.ExportApplyArgs{Token: ep.Token, Files: []string{"settings"}, Servers: true})
	if err != nil {
		t.Fatalf("ExportApply: %v", err)
	}
	if len(res.Servers) != 1 || res.Servers[0].Outcome != "installed" {
		t.Fatalf("servers = %+v, want gh installed", res.Servers)
	}
	if m, _ := filepath.Glob(settingsJSON + ".kira-backup-*"); len(m) != 1 {
		t.Fatalf("settings.json backups = %v, want one", m)
	}
	if got, _ := os.ReadFile(settingsJSON); !strings.Contains(string(got), `"theme"`) || !strings.Contains(string(got), "P454_PLAIN") || !strings.Contains(string(got), "Bash(rm:*)") {
		t.Fatalf("settings.json = %s, want the merge", got)
	}
	cj, _ := os.ReadFile(claudeJSON)
	if strings.Contains(string(cj), literal) || !strings.Contains(string(cj), "${KIRA_SECRET_GH_AUTHORIZATION}") {
		t.Fatalf(".claude.json = %s, want the env reference and no literal", cj)
	}
	out, errOut, err := app.run(t, app.Work, []string{"KIRA_SECRET_GH_AUTHORIZATION=" + literal}, "mcp", "list")
	if err != nil || !strings.Contains(out, "gh:") {
		t.Fatalf("claude mcp list = %v\n%s%s", err, out, errOut)
	}
}
