package claude

import (
	"testing"

	"github.com/kirathecat/kira-studio/internal/harness"
)

func TestScriptSessionShapes(t *testing.T) {
	base := harness.HeadlessSpec{Bin: "claude", SessionID: "s-1", MCPConfigs: []string{"/c/run.mcp.json"}, Config: harness.ConfigUser, AllowedTools: []string{"Write", "Bash(touch *)"}}
	fresh := "exec claude -p --output-format stream-json --verbose --session-id 's-1' --mcp-config '/c/run.mcp.json' --setting-sources 'user' --allowedTools 'Write' 'Bash(touch *)'"
	if got := Script(base); got != fresh {
		t.Fatalf("fresh:\n got %s\nwant %s", got, fresh)
	}
	base.Resume = "r-9"
	resumed := "exec claude -p --output-format stream-json --verbose --resume 'r-9' --mcp-config '/c/run.mcp.json' --setting-sources 'user' --allowedTools 'Write' 'Bash(touch *)'"
	if got := Script(base); got != resumed {
		t.Fatalf("resume:\n got %s\nwant %s", got, resumed)
	}
}

func TestScriptStageSettings(t *testing.T) {
	s := harness.HeadlessSpec{Bin: "claude", SessionID: "s-1", Config: harness.ConfigUser, AddDirs: []string{"/d"}, AllowedTools: []string{"Write"},
		Stage: harness.StageValues{SettingAutoCompactWindow: "200000"}}
	want := "exec claude -p --output-format stream-json --verbose --session-id 's-1' --autocompact '200000' --setting-sources 'user' --add-dir '/d' --allowedTools 'Write'"
	if got := Script(s); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	s.ContextFile = "/h/ctx/AGENTS.md"
	want = "exec claude -p --output-format stream-json --verbose --session-id 's-1' --autocompact '200000' --append-system-prompt-file '/h/ctx/AGENTS.md' --setting-sources 'user' --add-dir '/d' --allowedTools 'Write'"
	if got := Script(s); got != want {
		t.Fatalf("context:\n got %s\nwant %s", got, want)
	}
}

func TestScriptSandboxWrap(t *testing.T) {
	s := harness.HeadlessSpec{Bin: "claude", SessionID: "s-1", MCPConfigs: []string{"/c/run.mcp.json"}, Config: harness.ConfigUser,
		Sandbox: &harness.SandboxExec{Profile: "/p/it's.sb", Unset: []string{"SSH_AUTH_SOCK"}, Env: []string{"TMPDIR=/s/tmp", "GOFLAGS=-modcacherw"}}}
	want := `exec /usr/bin/env -u 'SSH_AUTH_SOCK' 'TMPDIR=/s/tmp' 'GOFLAGS=-modcacherw' '/usr/bin/sandbox-exec' -f '/p/it'\''s.sb' claude -p --output-format stream-json --verbose --session-id 's-1' --mcp-config '/c/run.mcp.json' --setting-sources 'user'`
	if got := Script(s); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestScriptIsolatedUserMCP(t *testing.T) {
	s := harness.HeadlessSpec{Bin: "claude", SessionID: "s-1", MCPConfigs: []string{"/c/run.mcp.json"}, Isolated: true, GrantMCP: []string{"kira-ade"}, UserMCP: []string{"fake", "b"}}
	want := `exec claude -p --output-format stream-json --verbose --session-id 's-1' --mcp-config '/c/run.mcp.json' --setting-sources 'user' --settings '{"disableAllHooks":true,"allowedMcpServers":[{"serverName":"kira-ade"},{"serverName":"fake"},{"serverName":"b"}],"enableAllProjectMcpServers":false,"enabledMcpjsonServers":[]}' --permission-prompts none`
	if got := Script(s); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	s.UserMCP = nil
	want = `exec claude -p --output-format stream-json --verbose --session-id 's-1' --mcp-config '/c/run.mcp.json' --setting-sources '' --strict-mcp-config --permission-prompts none`
	if got := Script(s); got != want {
		t.Fatalf("isolated:\n got %s\nwant %s", got, want)
	}
}

func TestScriptAppSettings(t *testing.T) {
	s := harness.HeadlessSpec{Bin: "claude", SessionID: "s-1", MCPConfigs: []string{"/c/run.mcp.json"}, Config: harness.ConfigAll,
		App: &harness.AppConfig{Files: map[string]string{"mcp": "/c/run-app.mcp.json", "settings": "/c/run-app.settings.json"}}}
	want := "exec claude -p --output-format stream-json --verbose --session-id 's-1' --mcp-config '/c/run.mcp.json' --mcp-config '/c/run-app.mcp.json' --setting-sources 'user,project,local' --settings '/c/run-app.settings.json'"
	if got := Script(s); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestScriptHermetic(t *testing.T) {
	base := harness.HeadlessSpec{Bin: "claude", SessionID: "s-1", Hermetic: true}
	for name, tc := range map[string]struct {
		cfg      harness.ConfigScope
		isolated bool
		want     string
	}{
		"all":      {harness.ConfigAll, false, "--setting-sources 'project,local'"},
		"user":     {harness.ConfigUser, false, "--setting-sources ''"},
		"isolated": {harness.ConfigAll, true, "--setting-sources '' --strict-mcp-config --permission-prompts none"},
	} {
		s := base
		s.Config, s.Isolated = tc.cfg, tc.isolated
		want := "exec claude -p --output-format stream-json --verbose --session-id 's-1' " + tc.want
		if got := Script(s); got != want {
			t.Fatalf("%s:\n got %s\nwant %s", name, got, want)
		}
	}
}

func TestInteractiveHermetic(t *testing.T) {
	l := (&Provider{}).InteractiveCommand(harness.InteractiveSpec{Bin: "claude", SessionID: "s-1", Hermetic: true})
	if want := "claude --session-id 's-1' '--setting-sources' 'project,local'"; l.Command != want {
		t.Fatalf("got %s want %s", l.Command, want)
	}
	if len(l.Env) != len(hermeticEnv) {
		t.Fatalf("env %v", l.Env)
	}
}
