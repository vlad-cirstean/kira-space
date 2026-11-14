package adeflow_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func scriptStep(id, command string) string {
	return "  - id: " + id + "\n    name: " + id + "\n    kind: script\n    runs_on: each repo\n    timeout: 1m\n    command: " + command + "\n"
}

func TestRunOutcomes(t *testing.T) {
	t.Run("agent reports done", func(t *testing.T) {
		f := newRunFixture(t, []fakeagent.Action{{Name: "done"}}, agentStep("one", ""))
		f.start(t, "feat/api-done")
		r := waitRun(t, f.app, f.taskID, "one", "done")
		if o := r.Outcome; o == nil || o.Status != "done" || o.Source != "agent" || !o.Reported {
			t.Fatalf("outcome = %+v, want done/agent/reported", o)
		}
	})

	t.Run("agent ends without a report", func(t *testing.T) {
		zero := 0
		f := newRunFixture(t, []fakeagent.Action{{Exit: &zero}}, agentStep("one", ""))
		f.start(t, "feat/api-silent")
		r := waitRun(t, f.app, f.taskID, "one", "stuck", "failed")
		if o := r.Outcome; o == nil || o.Status != "failed" || o.Reported || o.Reason != "no report: Claude ended without calling finish_step" {
			t.Fatalf("outcome = %+v, want the no-report reason", o)
		}
	})

	t.Run("script exits non-zero", func(t *testing.T) {
		f := newRunFixture(t, nil, scriptStep("check", "'echo oops >&2; exit 2'"))
		f.start(t, "feat/api-script")
		r := waitRun(t, f.app, f.taskID, "check", "stuck", "failed")
		o := r.Outcome
		if o == nil || o.Source != "exit" || o.Reason != "exited with status 2" || o.ExitCode == nil || *o.ExitCode != 2 || !strings.Contains(o.LastError, "oops") {
			t.Fatalf("outcome = %+v, want exit 2 with stderr as the last error", o)
		}
	})

	t.Run("stop cancels", func(t *testing.T) {
		flag := filepath.Join(t.TempDir(), "release")
		f := newRunFixture(t, []fakeagent.Action{{Name: "done", WaitFile: flag}}, agentStep("one", ""))
		f.start(t, "feat/api-stop")
		running := waitRun(t, f.app, f.taskID, "one", "running")
		if err := f.app.W.AdeTask.StopRun(ctx, adewire.RunArgs{RunID: running.ID}); err != nil {
			t.Fatal(err)
		}
		r := waitRun(t, f.app, f.taskID, "one", "stuck")
		if o := r.Outcome; o == nil || o.Status != "cancelled" || o.Source != "user" || o.Reason != "stopped by you" {
			t.Fatalf("outcome = %+v, want cancelled by the user", o)
		}
	})

	t.Run("restart interrupts", func(t *testing.T) {
		hold := filepath.Join(t.TempDir(), "hold")
		f := newRunFixture(t, []fakeagent.Action{{WaitFile: hold}}, agentStep("one", ""))
		f.start(t, "feat/api-restart-outcome")
		waitRun(t, f.app, f.taskID, "one", "running")
		testx.WaitUntil(t, waitFor, func() bool { _, err := os.Stat(filepath.Join(f.app.FakeDir, "api-1.args")); return err == nil })
		f.app.Restart()
		if err := os.WriteFile(hold, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		r, _ := latestRun(t, f.app, f.taskID, "one")
		if o := r.Outcome; o == nil || o.Status != "failed" || o.Source != "restart" || !strings.Contains(o.Reason, "Kira Space") {
			t.Fatalf("run after restart = %+v, want failed by restart", r)
		}
	})
}

// J-RUN-021. A run's MCP config file goes and its token stops working once the run ends, whichever
// way it ends; a hung agent hits the step timeout.
func TestRunEndRevokesMCPAccess(t *testing.T) {
	read := func(t *testing.T, f *runFixture) (cfgPath, url, token string) {
		t.Helper()
		testx.WaitUntil(t, waitFor, func() bool { _, err := os.Stat(filepath.Join(f.app.FakeDir, "api-1.args")); return err == nil })
		cfgPath = argAfter(fakeFile(t, f.app, "api-1.args"), "--mcp-config")
		raw, err := os.ReadFile(cfgPath)
		if err != nil {
			t.Fatalf("mcp config %s: %v", cfgPath, err)
		}
		var cfg struct {
			Servers map[string]struct {
				URL     string            `json:"url"`
				Headers map[string]string `json:"headers"`
			} `json:"mcpServers"`
		}
		if err := json.Unmarshal(raw, &cfg); err != nil || len(cfg.Servers) == 0 {
			t.Fatalf("mcp config = %s, %v", raw, err)
		}
		for _, s := range cfg.Servers {
			if auth := s.Headers["Authorization"]; auth != "" {
				return cfgPath, s.URL, auth
			}
		}
		t.Fatalf("no authorised server in %s", raw)
		return
	}
	probe := func(t *testing.T, url, auth string) int {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		req.Header.Set("Authorization", auth)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0 // listener closed with the last registration: nothing accepts the token
		}
		defer res.Body.Close()
		return res.StatusCode
	}
	cases := []struct {
		name    string
		actions func(hold string) []fakeagent.Action
		steps   string
		state   string
	}{
		{"finished", func(h string) []fakeagent.Action { return []fakeagent.Action{{Name: "done", WaitFile: h}} }, agentStep("one", ""), "done"},
		{"crashed", func(h string) []fakeagent.Action { return []fakeagent.Action{{Name: "fail", WaitFile: h}} }, agentStep("one", ""), "failed"},
		{"timed out", func(h string) []fakeagent.Action { return []fakeagent.Action{{Name: "sleep"}} },
			"  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 2s\n    prompt: work\n", "failed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hold := filepath.Join(t.TempDir(), "hold")
			f := newRunFixture(t, c.actions(hold), c.steps)
			f.start(t, "feat/api-revoke")
			cfgPath, url, auth := read(t, f)
			if code := probe(t, url, auth); code != http.StatusOK {
				t.Fatalf("live token probe = %d, want 200", code)
			}
			if err := os.WriteFile(hold, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			r := waitRun(t, f.app, f.taskID, "one", c.state, "stuck")
			if c.name == "timed out" && (r.Outcome == nil || r.Outcome.Source != "timeout") {
				t.Fatalf("run = %+v, want a timeout outcome", r)
			}
			testx.WaitUntil(t, waitFor, func() bool { _, err := os.Stat(cfgPath); return os.IsNotExist(err) })
			if code := probe(t, url, auth); code != http.StatusUnauthorized && code != 0 {
				t.Fatalf("token probe after the run = %d, want 401 or a closed listener", code)
			}
		})
	}
}
