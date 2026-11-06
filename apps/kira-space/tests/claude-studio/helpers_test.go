package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
)

var ctx = context.Background()

const (
	// testTimeout bounds one real claude call.
	testTimeout = 3 * time.Minute
	// budget caps every claude -p argv this suite builds.
	budget = "0.05"
)

type realApp struct {
	*studioflowharness.App
	Claude string
}

// newRealApp boots the harness over a temp HOME and checks the real claude runs in it. The
// harness's installer fake stays: tests forward its recorded arguments to the real installer.
func newRealApp(t *testing.T) *realApp {
	t.Helper()
	found, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("real claude tests: no claude on PATH")
	}
	parentSession := os.Getenv("CLAUDE_CODE_SESSION_ID")
	if parentSession == "" {
		parentSession = os.Getenv("CLAUDE_CODE_REMOTE_SESSION_ID")
	}
	app := studioflowharness.New(t)
	// Stop the agent session running this suite from leaking its identity into the child (the
	// sandbox's remote session id does too: the child reuses it as its own session_id).
	for _, k := range []string{"CLAUDE_CODE_SESSION_ID", "CLAUDE_CODE_REMOTE_SESSION_ID", "CLAUDECODE"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	ra := &realApp{App: app, Claude: found}
	preflight(t, ra, parentSession)
	return ra
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
		out, errOut, err := a.run(t, t.TempDir(), "-p", "Reply: ok", "--model", "haiku", "--max-budget-usd", "0.02", "--output-format", "json")
		if err != nil {
			preflightSkip = fmt.Sprintf("claude -p does not run under a temp HOME (see docs/DEV_ENVIRONMENT.md \"Real `claude` tests\"): %v\n%s", err, tail(errOut+out, 600))
			return
		}
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

// run executes the real claude with the harness env (temp HOME).
func (a *realApp) run(t *testing.T, dir string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	c, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	cmd := exec.CommandContext(c, a.Claude, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err = cmd.Run()
	return so.String(), se.String(), err
}

// claudeP runs `claude -p <prompt>` with the suite's guards: haiku and the budget cap.
func (a *realApp) claudeP(t *testing.T, dir, prompt string, flags ...string) string {
	t.Helper()
	args := append([]string{"-p", prompt, "--model", "haiku", "--max-budget-usd", budget}, flags...)
	out, errOut, err := a.run(t, dir, args...)
	if err != nil {
		t.Fatalf("claude -p failed: %v\nstdout: %s\nstderr: %s", err, tail(out, 1500), tail(errOut, 1500))
	}
	return out
}

type streamMsg struct {
	Type    string `json:"type"`
	IsError bool   `json:"is_error"`
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type contentBlock struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
	Input     json.RawMessage `json:"input"`
}

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
			var parts []struct {
				Text string `json:"text"`
			}
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
