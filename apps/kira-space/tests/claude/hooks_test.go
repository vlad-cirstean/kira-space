package claude

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/internal/terminal"
)

// TestTerminalAgentHooks launches the real TUI through Terminal.Open and expects the injected
// --settings hooks to reach the listener.
func TestTerminalAgentHooks(t *testing.T) {
	repo := flowRepo(t)
	app := newRealApp(t, repo)
	const id = "agent-hooks"
	mark := app.Events.Mark()
	app.openAgent(t, id, repo, "claude --model haiku")

	app.waitAgentEvent(t, mark, id, "SessionStart", 60*time.Second)
	sessions := app.W.Terminal.AgentSessions().Sessions
	if len(sessions) != 1 || sessions[0].TerminalID != id {
		t.Fatalf("AgentSessions while live = %+v, want just %s", sessions, id)
	}
	// SessionStart fires before the input box accepts keys.
	time.Sleep(2 * time.Second)
	app.typeLine(t, id, "Reply with the single word pong.")
	app.waitAgentEvent(t, mark, id, "UserPromptSubmit", 30*time.Second)
	stop := app.waitAgentEvent(t, mark, id, "Stop", 90*time.Second)
	if stop.SessionID == "" || stop.Cwd == "" {
		t.Fatalf("Stop event lacks session or cwd: %+v", stop)
	}
	if start := app.waitAgentEvent(t, mark, id, "SessionStart", time.Second); start.SessionID != stop.SessionID {
		t.Fatalf("SessionStart session %q, Stop session %q, want one conversation", start.SessionID, stop.SessionID)
	}

	if err := app.W.Terminal.Close(terminal.CloseArgs{TerminalID: id}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for len(app.W.Terminal.AgentSessions().Sessions) != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("AgentSessions after Close = %+v, want none", app.W.Terminal.AgentSessions().Sessions)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// hookEvents copies internal/harness/claude/hooks/config.go's hookEvents for the same reason.
var hookEvents = []string{"SessionStart", "SessionEnd", "PreToolUse", "PostToolUse", "Notification", "Stop", "UserPromptSubmit", "SubagentStop"}

// captureSettings writes a hooks file whose every event appends its raw stdin payload as one
// line to capture.
func captureSettings(t *testing.T, dir, capture string) string {
	t.Helper()
	shim := filepath.Join(dir, "capture.sh")
	writeFile(t, shim, "#!/bin/sh\ncat >> '"+capture+"'\necho >> '"+capture+"'\n")
	if err := os.Chmod(shim, 0o755); err != nil {
		t.Fatal(err)
	}
	hooks := map[string]any{}
	for _, ev := range hookEvents {
		hooks[ev] = []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": shim, "timeout": 5}}}}
	}
	path := filepath.Join(dir, "hooks.json")
	writeFile(t, path, string(mustRaw(map[string]any{"hooks": hooks})))
	return path
}

func readPayloads(t *testing.T, capture string) []map[string]any {
	t.Helper()
	f, err := os.Open(capture)
	if err != nil {
		t.Fatalf("no hook ever fired: %v", err)
	}
	defer f.Close()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("hook payload is not JSON: %v: %s", err, sc.Text())
		}
		out = append(out, m)
	}
	return out
}

// TestHookPayloadContract catches CLI payload drift: the fields internal/harness/claude/hooks/http.go's
// hookRequest decodes (unexported; the always-present and per-event ones are listed below) must be
// in what the installed CLI sends, and the CLI must accept every event the hooks file names.
func TestHookPayloadContract(t *testing.T) {
	repo := flowRepo(t)
	app := newRealApp(t, repo)
	dir := t.TempDir()
	capture := filepath.Join(dir, "capture.jsonl")
	settings := captureSettings(t, dir, capture)

	_, stderr := app.claudeP(t, repo, nil,
		"Use the Bash tool to run exactly: true. Then reply with the word done.",
		"--settings", settings, "--allowedTools", "Bash(true)")
	for _, bad := range []string{"Invalid", "invalid", "settings error", "Unrecognized"} {
		if strings.Contains(stderr, bad) {
			t.Fatalf("CLI rejected the hooks file (stderr has %q):\n%s", bad, tail(stderr, 800))
		}
	}

	byEvent := map[string][]map[string]any{}
	for _, p := range readPayloads(t, capture) {
		name, _ := p["hook_event_name"].(string)
		byEvent[name] = append(byEvent[name], p)
		for _, f := range []string{"hook_event_name", "session_id", "cwd"} {
			if s, _ := p[f].(string); s == "" {
				t.Errorf("%s payload lacks %q: %v", name, f, p)
			}
		}
	}
	for _, ev := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"} {
		if len(byEvent[ev]) == 0 {
			t.Errorf("no %s payload captured; got events %v", ev, keys(byEvent))
		}
	}
	for _, ev := range []string{"PreToolUse", "PostToolUse"} {
		for _, p := range byEvent[ev] {
			for _, f := range []string{"tool_name", "tool_use_id"} {
				if s, _ := p[f].(string); s == "" {
					t.Errorf("%s payload lacks %q: %v", ev, f, p)
				}
			}
			if p["tool_name"] != "Bash" {
				t.Errorf("%s tool_name = %v, want Bash", ev, p["tool_name"])
			}
		}
	}
	for _, p := range byEvent["SessionStart"] {
		if s, _ := p["source"].(string); s == "" {
			t.Errorf("SessionStart payload lacks %q: %v", "source", p)
		}
	}
}

// TestHookBackgroundTasks pins the payload facts the background status relies on: a Stop lists
// running background tasks (id, type, status), a subagent's SubagentStop carries its agent_id, and
// the last Stop, after the tasks ended, lists none. claude -p stays alive until they end.
func TestHookBackgroundTasks(t *testing.T) {
	repo := flowRepo(t)
	app := newRealApp(t, repo)
	dir := t.TempDir()
	capture := filepath.Join(dir, "capture.jsonl")
	settings := captureSettings(t, dir, capture)

	app.claudeP(t, repo, nil,
		"Use the Bash tool with run_in_background true to run exactly: sleep 6. "+
			"Then use the Agent tool once with a general-purpose subagent that only replies with the word hi. "+
			"Then reply with the word done.",
		"--settings", settings, "--allowedTools", "Bash(sleep 6),Agent")

	var stops []map[string]any
	var subagentStops []map[string]any
	for _, p := range readPayloads(t, capture) {
		switch p["hook_event_name"] {
		case "Stop":
			stops = append(stops, p)
		case "SubagentStop":
			subagentStops = append(subagentStops, p)
		}
	}
	if len(stops) < 2 {
		t.Fatalf("captured %d Stop payloads, want the turn's Stop and the follow-up after the task ended", len(stops))
	}
	running := false
	for _, p := range stops {
		tasks, ok := p["background_tasks"].([]any)
		if !ok {
			t.Fatalf("Stop payload lacks background_tasks: %v", p)
		}
		for _, raw := range tasks {
			task, _ := raw.(map[string]any)
			for _, f := range []string{"id", "type", "status"} {
				if s, _ := task[f].(string); s == "" {
					t.Errorf("background task lacks %q: %v", f, task)
				}
			}
			running = running || task["status"] == "running"
		}
	}
	if !running {
		t.Errorf("no Stop listed a running background task: %v", stops)
	}
	if last, _ := stops[len(stops)-1]["background_tasks"].([]any); len(last) != 0 {
		t.Errorf("last Stop background_tasks = %v, want none", last)
	}
	if len(subagentStops) == 0 {
		t.Fatalf("no SubagentStop captured; the CLI no longer sends it or the subagent never ran")
	}
	for _, p := range subagentStops {
		if s, _ := p["agent_id"].(string); s == "" {
			t.Errorf("SubagentStop lacks agent_id: %v", p)
		}
	}
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// TestShimInertForPlainClaude: Kira's own hooks file, passed to a plain claude without the KIRA_*
// env a Kira terminal sets, reaches no listener and breaks nothing.
func TestShimInertForPlainClaude(t *testing.T) {
	repo := flowRepo(t)
	app := newRealApp(t, repo)
	status := app.W.AgentHooks.Status("claude")
	if !status.Running || status.SettingsPath == "" {
		t.Fatalf("agent hooks not running: %+v", status)
	}
	mark := app.Events.Mark()
	out, _ := app.claudeP(t, repo, []string{"KIRA_AGENT_HOOK_TOKEN=", "KIRA_AGENT_HOOK_SOCKET=", "KIRA_TERMINAL_ID="},
		"Reply with the single word pong.", "--settings", status.SettingsPath)
	if !strings.Contains(strings.ToLower(out), "pong") {
		t.Fatalf("claude output = %q, want pong", tail(out, 300))
	}
	time.Sleep(time.Second)
	if got := app.Events.Since(mark, "kira:agent:event"); len(got) != 0 {
		t.Fatalf("listener received %d hook events from a plain claude, want 0", len(got))
	}
}
