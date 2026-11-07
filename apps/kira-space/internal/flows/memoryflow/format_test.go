package memoryflow_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/memory/memfmt"
)

// gitIn runs git in dir; a failure returns the combined output.
func gitIn(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitIn(dir, args...)
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Contract memory-settings. UI half: settings-memory.spec.ts "contract: check hook shows the
// install commands".
func TestCheckHook(t *testing.T) {
	app := flowharness.New(t)
	cc := app.W.Memory.CheckHook()
	exe := flowharness.Replace(cc.Executable, "<bin>/kira-space")
	app.Contract(t, "memory-settings", "MemoryService.CheckHook", cc, exe)
	if !filepath.IsAbs(cc.Executable) || !strings.Contains(cc.InstallCommand, "memory-check --install-hook") ||
		!strings.Contains(cc.HookLine, "memory-check --staged") || !strings.Contains(cc.InstallCommand, cc.Executable) {
		t.Fatalf("CheckHook = %+v, want an absolute executable in both commands", cc)
	}

	repo := t.TempDir()
	mustGit(t, repo, "init", "-q")
	install := exec.Command(cc.Executable, "memory-check", "--install-hook")
	install.Dir = repo
	if out, err := install.CombinedOutput(); err != nil {
		t.Fatalf("install-hook: %v\n%s", err, out)
	}
	hook, err := os.ReadFile(filepath.Join(repo, ".git", "hooks", "pre-commit"))
	if err != nil || !strings.Contains(string(hook), "# kira-memory-check") || !strings.Contains(string(hook), "memory-check --staged") {
		t.Fatalf("hook = %q, %v, want the marked pre-commit script", hook, err)
	}

	writeFile(t, repo, "bad.go", "package bad\n// kira-memory origin=user scope=company\n// km_7h3kq9x2mfa4 Port is 9090.\n")
	mustGit(t, repo, "add", "bad.go")
	out, err := gitIn(repo, "commit", "-q", "-m", "bad")
	if err == nil || !strings.Contains(out, "bad.go:2: ") || !strings.Contains(out, "retired (P296)") {
		t.Fatalf("commit of a retired block = %v\n%s, want it blocked with path:line", err, out)
	}

	writeFile(t, repo, "bad.go", "package bad\n// Ports are fixed by the manifest (km_7h3kq9x2mfa4).\n")
	mustGit(t, repo, "add", "bad.go")
	if out, err := gitIn(repo, "commit", "-q", "-m", "good"); err != nil {
		t.Fatalf("commit of a plain comment citing a fact id: %v\n%s", err, out)
	}

	foreign := t.TempDir()
	mustGit(t, foreign, "init", "-q")
	mine := filepath.Join(foreign, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(mine, []byte("#!/bin/sh\necho mine\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	clash := exec.Command(cc.Executable, "memory-check", "--install-hook")
	clash.Dir = foreign
	clashOut, err := clash.CombinedOutput()
	if err == nil || !strings.Contains(string(clashOut), cc.HookLine) {
		t.Fatalf("install over a foreign hook = %v\n%s, want exit 1 printing the hook line", err, clashOut)
	}
	if kept, _ := os.ReadFile(mine); string(kept) != "#!/bin/sh\necho mine\n" {
		t.Fatalf("foreign hook was rewritten: %q", kept)
	}
}

// memoryMCP connects a client to a real memory-mcp subprocess over the app's memory home.
func memoryMCP(t *testing.T, app *flowharness.App) *mcp.ClientSession {
	t.Helper()
	cmd := exec.Command(app.W.Memory.McpStatus().Executable, "memory-mcp")
	cmd.Dir = app.Work
	cmd.Env = append(os.Environ(), "CLAUDE_CODE_SESSION_ID="+flowSession)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "flow", Version: "1"}, nil).Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

type storeOut struct{ Status string }

func callStore(t *testing.T, cs *mcp.ClientSession, args map[string]any) (storeOut, string) {
	t.Helper()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "store_memory", Arguments: args})
	if err != nil || res.IsError {
		t.Fatalf("store_memory: %v %+v", err, res)
	}
	var out storeOut
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out, res.Content[0].(*mcp.TextContent).Text
}

// metaLine is a result's indented meta line; it starts with the version, kind and origin.
var metaLine = regexp.MustCompile(`^  v\d+ (fact|rule) origin=`)

func TestMcpSearchTextFormat(t *testing.T) {
	app := flowharness.New(t)
	gate(t, app, "gate-accept.json")
	if _, err := app.W.Memory.Store(ctx, twoItemsArgs()); err != nil {
		t.Fatal(err)
	}
	cs := memoryMCP(t, app)
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "search_memories", Arguments: map[string]any{"query": "postgresql"}})
	if err != nil || res.IsError {
		t.Fatalf("search_memories: %v %+v", err, res)
	}
	var lines []string
	for _, l := range strings.Split(res.Content[0].(*mcp.TextContent).Text, "\n") {
		if l != "" && !strings.HasPrefix(l, "Note: ") {
			lines = append(lines, l)
		}
	}
	if len(lines) != 3 || !regexp.MustCompile(`^`+memfmt.FactID+` \S`).MatchString(lines[0]) || !regexp.MustCompile(`^  \S`).MatchString(lines[1]) ||
		!metaLine.MatchString(lines[2]) || metaLine.MatchString(lines[1]) {
		t.Fatalf("search text = %q, want '<km_ id> <fact>', an indented reason and an indented meta line", lines)
	}
}
