package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/harness/providers"
	"github.com/kirathecat/kira-studio/internal/memory"
	"github.com/kirathecat/kira-studio/internal/terminal"
)

var ctx = context.Background()

const (
	// testTimeout bounds one test's real claude work.
	testTimeout = 3 * time.Minute
	// budget caps every claude -p argv this suite builds.
	budget = "0.05"
	// costLimit fails a test whose app-built headless run spent more (USD).
	costLimit = 0.10
	window    = "w-real"
)

// realApp is a flow-harness app whose `claude` is the real CLI, under a temp HOME.
type realApp struct {
	*flowharness.App
	Claude string
	// tee collects the stream-json stdout of every `claude -p` the app launches.
	tee string
}

// newRealApp boots the harness, then swaps the fake claude for the real one. Launches resolve
// `claude` through the login shell's PATH, whose first entry is BinDir (P150), so the real binary
// runs even from a ~/.local/bin install outside the temp HOME.
func newRealApp(t *testing.T, trustDirs ...string) *realApp {
	return newRealAppWith(t, nil, trustDirs...)
}

// newRealAppWith adds harness options, such as the real sandbox enforcer.
func newRealAppWith(t *testing.T, opts []flowharness.Opt, trustDirs ...string) *realApp {
	t.Helper()
	found, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("real claude tests: no claude on PATH")
	}
	real, err := filepath.EvalSymlinks(found)
	if err != nil {
		t.Fatal(err)
	}
	parentSession := os.Getenv("CLAUDE_CODE_SESSION_ID")
	if parentSession == "" {
		parentSession = os.Getenv("CLAUDE_CODE_REMOTE_SESSION_ID")
	}
	app := flowharness.New(t, opts...)
	ra := &realApp{App: app, Claude: real, tee: filepath.Join(app.Root, "claude-tee.jsonl")}

	// A wrapper, not a bare symlink: it copies the stdout of `claude -p` runs to ra.tee so a test
	// can read the result line (cost, model) the app's run log drops. Interactive launches exec
	// the real binary directly.
	wrapper := filepath.Join(app.BinDir, "claude")
	if err := os.Remove(wrapper); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf(`#!/bin/bash
for a in "$@"; do
  if [ "$a" = "-p" ]; then
    set -o pipefail
    %q "$@" | tee -a %q
    exit "${PIPESTATUS[0]}"
  fi
done
exec %q "$@"
`, real, ra.tee, real)
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	// Pin the model for app-composed launches (no --model on those argvs) and stop the agent
	// session running this suite from leaking its identity into the child (the sandbox's remote
	// session id does too: the child reuses it as its own session_id).
	t.Setenv("ANTHROPIC_MODEL", "haiku")
	for _, k := range []string{"CLAUDE_CODE_SESSION_ID", "CLAUDE_CODE_REMOTE_SESSION_ID", "CLAUDECODE"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	ra.seedClaudeJSON(t, trustDirs...)
	preflight(t, ra, parentSession)
	return ra
}

// seedClaudeJSON marks onboarding done and trusts each dir, keeping whatever else the CLI wrote.
// Without projects[dir] the TUI stops at the folder-trust dialog. Call before each launch.
func (a *realApp) seedClaudeJSON(t *testing.T, dirs ...string) {
	t.Helper()
	path := filepath.Join(a.Home, ".claude.json")
	doc := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &doc)
	}
	doc["hasCompletedOnboarding"] = true
	doc["theme"] = "dark"
	projects, _ := doc["projects"].(map[string]any)
	if projects == nil {
		projects = map[string]any{}
	}
	for _, d := range dirs {
		if resolved, err := filepath.EvalSymlinks(d); err == nil {
			d = resolved
		}
		entry, _ := projects[d].(map[string]any)
		if entry == nil {
			entry = map[string]any{}
		}
		entry["hasTrustDialogAccepted"] = true
		projects[d] = entry
	}
	doc["projects"] = projects
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

var (
	preflightOnce sync.Once
	preflightSkip string
	preflightFail string
)

// preflight runs one cheap `claude -p` in the temp HOME, once per package: it proves auth works
// there, that the haiku pin took, and that the child is not the parent agent session. An auth
// problem skips, because it is the environment's, not a regression.
func preflight(t *testing.T, a *realApp, parentSession string) {
	t.Helper()
	preflightOnce.Do(func() {
		out, errOut, err := a.run(t, a.Work, nil, "-p", "Reply: ok", "--model", "haiku", "--max-budget-usd", "0.02", "--output-format", "json")
		if err != nil {
			preflightSkip = fmt.Sprintf("claude -p does not run under a temp HOME (see docs/DEV_ENVIRONMENT.md \"Real `claude` tests\"): %v\n%s", err, tail(errOut+out, 600))
			return
		}
		recordSpend(out)
		var res struct {
			SessionID  string                    `json:"session_id"`
			ModelUsage map[string]map[string]any `json:"modelUsage"`
			IsError    bool                      `json:"is_error"`
			Result     string                    `json:"result"`
		}
		if err := json.Unmarshal([]byte(out), &res); err != nil {
			preflightFail = fmt.Sprintf("preflight output is not JSON: %v\n%s", err, tail(out, 600))
			return
		}
		if res.IsError {
			preflightSkip = "claude -p reports an error under a temp HOME (auth?): " + tail(res.Result, 300)
			return
		}
		haiku := false
		for k := range res.ModelUsage {
			haiku = haiku || strings.Contains(k, "haiku")
		}
		if !haiku {
			preflightFail = fmt.Sprintf("modelUsage has no haiku key: %v", res.ModelUsage)
		}
		if parentSession != "" && res.SessionID == parentSession {
			preflightFail = "child session_id equals the parent agent session's: env leaked"
		}
	})
	if preflightSkip != "" {
		t.Skip(preflightSkip)
	}
	if preflightFail != "" {
		t.Fatal(preflightFail)
	}
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

// run executes the real claude with the harness env (temp HOME). extraEnv entries override. The
// per-test timeout is testTimeout.
func (a *realApp) run(t *testing.T, dir string, extraEnv []string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	c, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	cmd := exec.CommandContext(c, a.Claude, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err = cmd.Run()
	return so.String(), se.String(), err
}

// claudeP runs `claude -p <prompt>` with the suite's guards: haiku and the budget cap.
func (a *realApp) claudeP(t *testing.T, dir string, extraEnv []string, prompt string, flags ...string) (stdout, stderr string) {
	t.Helper()
	args := append([]string{"-p", prompt, "--model", "haiku", "--max-budget-usd", budget}, flags...)
	if !slices.Contains(flags, "--output-format") {
		args = append(args, "--output-format", "json")
	}
	out, errOut, err := a.run(t, dir, extraEnv, args...)
	if err != nil {
		t.Fatalf("claude -p failed: %v\nstdout: %s\nstderr: %s", err, tail(out, 1500), tail(errOut, 1500))
	}
	recordSpend(out)
	return out, errOut
}

var (
	spendMu sync.Mutex
	spent   float64
)

// recordSpend adds the total_cost_usd of every result line in out (json or stream-json) to the
// package total TestMain prints. TUI sessions are not measurable this way.
func recordSpend(out string) {
	var total float64
	for _, m := range parseStream(out) {
		if m.Type == "result" {
			total += m.TotalCostUSD
		}
	}
	spendMu.Lock()
	spent += total
	spendMu.Unlock()
}

// streamMsg is the part of a stream-json line the tests read.
type streamMsg struct {
	Type         string  `json:"type"`
	Subtype      string  `json:"subtype"`
	SessionID    string  `json:"session_id"`
	IsError      bool    `json:"is_error"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	Message      struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type contentBlock struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Text      string          `json:"text"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// parseStream decodes stream-json lines, skipping any that are not JSON.
func parseStream(raw string) []streamMsg {
	var out []streamMsg
	sc := bufio.NewScanner(strings.NewReader(raw))
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var m streamMsg
		if json.Unmarshal(sc.Bytes(), &m) == nil && m.Type != "" {
			out = append(out, m)
		}
	}
	return out
}

func blocks(m streamMsg) []contentBlock {
	var bs []contentBlock
	_ = json.Unmarshal(m.Message.Content, &bs)
	return bs
}

// toolUses returns the tool_use blocks named name across the stream.
func toolUses(msgs []streamMsg, name string) []contentBlock {
	var out []contentBlock
	for _, m := range msgs {
		if m.Type != "assistant" {
			continue
		}
		for _, b := range blocks(m) {
			if b.Type == "tool_use" && b.Name == name {
				out = append(out, b)
			}
		}
	}
	return out
}

// toolResult returns the text and error flag of the result answering toolUseID.
func toolResult(msgs []streamMsg, toolUseID string) (text string, isError, ok bool) {
	for _, m := range msgs {
		if m.Type != "user" {
			continue
		}
		for _, b := range blocks(m) {
			if b.Type != "tool_result" || b.ToolUseID != toolUseID {
				continue
			}
			var s string
			if json.Unmarshal(b.Content, &s) == nil {
				return s, b.IsError, true
			}
			var parts []contentBlock
			var sb strings.Builder
			if json.Unmarshal(b.Content, &parts) == nil {
				for _, p := range parts {
					sb.WriteString(p.Text)
				}
			}
			return sb.String(), b.IsError, true
		}
	}
	return "", false, false
}

func mustRaw(v any) []byte {
	raw, _ := json.Marshal(v)
	return raw
}

// resultMsg returns the final result line of a stream.
func resultMsg(t *testing.T, msgs []streamMsg) streamMsg {
	t.Helper()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Type == "result" {
			return msgs[i]
		}
	}
	t.Fatal("stream has no result line")
	return streamMsg{}
}

// teeCost sums total_cost_usd over the result lines of every headless run the app made.
func (a *realApp) teeCost(t *testing.T) float64 {
	t.Helper()
	raw, _ := os.ReadFile(a.tee)
	var total float64
	for _, m := range parseStream(string(raw)) {
		if m.Type == "result" {
			total += m.TotalCostUSD
		}
	}
	return total
}

func (a *realApp) requireCostUnder(t *testing.T) {
	t.Helper()
	c := a.teeCost(t)
	spendMu.Lock()
	spent += c
	spendMu.Unlock()
	if c > costLimit {
		t.Fatalf("headless runs cost %.4f USD, limit %.2f", c, costLimit)
	}
}

// agentEvent is one ChannelAgentEvent payload (harness.AgentEvent on the wire).
type agentEvent struct {
	TerminalID string `json:"terminalId"`
	Event      string `json:"event"`
	SessionID  string `json:"sessionId"`
	Cwd        string `json:"cwd"`
	Source     string `json:"source"`
}

// agentEvents returns the hook events recorded since mark, optionally for one terminal.
func (a *realApp) agentEvents(t *testing.T, mark int, terminalID string) []agentEvent {
	t.Helper()
	var out []agentEvent
	for _, ev := range a.Events.Since(mark, bridge.ChannelAgentEvent) {
		var e agentEvent
		ev.Decode(t, &e)
		if terminalID == "" || e.TerminalID == terminalID {
			out = append(out, e)
		}
	}
	return out
}

// waitAgentEvent polls until a hook event named name arrives for terminalID after mark.
func (a *realApp) waitAgentEvent(t *testing.T, mark int, terminalID, name string, timeout time.Duration) agentEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, e := range a.agentEvents(t, mark, terminalID) {
			if e.Event == name {
				return e
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("no %s hook event for terminal %s within %s; saw %+v", name, terminalID, timeout, a.agentEvents(t, mark, terminalID))
	return agentEvent{}
}

// openAgent opens a agent terminal the way the renderer does.
func (a *realApp) openAgent(t *testing.T, id, cwd, command string) {
	t.Helper()
	if _, err := a.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: id, WindowKey: window, Cwd: cwd, Cols: 120, Rows: 40,
		Command: command, LaunchKind: terminal.LaunchKindAgent,
	}); err != nil {
		t.Fatalf("Terminal.Open %s: %v", id, err)
	}
	t.Cleanup(func() { _ = a.W.Terminal.Close(terminal.CloseArgs{TerminalID: id}) })
}

// typeLine types text into the terminal, then Enter after a beat so the TUI treats it as a submit.
func (a *realApp) typeLine(t *testing.T, id, text string) {
	t.Helper()
	write := func(s string) {
		if err := a.W.Terminal.Write(terminal.WriteArgs{TerminalID: id, Data: base64.StdEncoding.EncodeToString([]byte(s))}); err != nil {
			t.Fatalf("Terminal.Write: %v", err)
		}
	}
	write(text)
	time.Sleep(500 * time.Millisecond)
	write("\r")
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// flowRepo creates a committed repo under a temp dir, trusted by claude through newRealApp.
func flowRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	repo := filepath.Join(dir, "proj")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.name=T", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "base"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return repo
}

func agentSelector() harness.Selector { return harness.Selector{Registry: providers.New()} }

func newAgentRunner() *memory.AgentRunner { return memory.NewAgentRunner(agentSelector(), nil) }
