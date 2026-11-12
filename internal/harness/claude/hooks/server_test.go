package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/internal/harness"
)

// newTestServer starts a real Server (a real mkdtemp dir, a real unix socket, a real http.Server)
// and registers its own cleanup — every test in this file talks to it exactly as the shim would,
// over the socket, never by calling Go methods that skip the wire.
func newTestServer(t *testing.T, onEvent func(harness.AgentEvent)) *Server {
	t.Helper()
	s, err := New(harness.HookOptions{OnEvent: onEvent})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// clientFor dials s's own unix socket directly — §19.2's "over a real socket", bypassing the
// generated shim entirely (that script is covered by shim_test-shaped coverage of buildShim's own
// refusal rule, exercised below via ShellSingleQuote).
func clientFor(s *Server) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", s.ln.SockPath)
			},
		},
		Timeout: 2 * time.Second,
	}
}

func postHook(t *testing.T, client *http.Client, token, terminalID, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "http://localhost/hook", strings.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if terminalID != "" {
		req.Header.Set("X-Kira-Terminal", terminalID)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

// recordingEvents collects every Event OnEvent is called with, safe for concurrent use — the
// listener's own handler runs on net/http's per-request goroutine.
type recordingEvents struct {
	mu     sync.Mutex
	events []harness.AgentEvent
}

func (r *recordingEvents) record(e harness.AgentEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recordingEvents) all() []harness.AgentEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]harness.AgentEvent, len(r.events))
	copy(out, r.events)
	return out
}

// wait polls until n events arrived: OnEvent runs after the reply, so it can trail the response.
func (r *recordingEvents) wait(t *testing.T, n int) []harness.AgentEvent {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for len(r.all()) < n && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	return r.all()
}

// TestValidRequestDeliversEvent is §19.2 case 1: a valid POST with a token and X-Kira-Terminal
// delivers an Event carrying the header's terminal id and the body's hook_event_name/tool_name.
func TestValidRequestDeliversEvent(t *testing.T) {
	rec := &recordingEvents{}
	s := newTestServer(t, rec.record)
	client := clientFor(s)

	body := `{"hook_event_name":"PreToolUse","session_id":"sess-1","cwd":"/repo","tool_name":"Bash","tool_use_id":"tu-1","tool_input":{"command":"rm -rf /"}}`
	resp := postHook(t, client, s.ln.Token, "term-1", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	events := rec.wait(t, 1)
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	got := events[0]
	if got.TerminalID != "term-1" {
		t.Errorf("TerminalID = %q, want %q", got.TerminalID, "term-1")
	}
	if got.Event != "PreToolUse" {
		t.Errorf("Event = %q, want %q", got.Event, "PreToolUse")
	}
	if got.ToolName != "Bash" {
		t.Errorf("ToolName = %q, want %q", got.ToolName, "Bash")
	}
	if got.ToolUseID != "tu-1" {
		t.Errorf("ToolUseID = %q, want %q", got.ToolUseID, "tu-1")
	}
}

// TestWrongTokenRejected is §19.2 case 2: a wrong token is rejected and OnEvent never fires.
func TestWrongTokenRejected(t *testing.T) {
	rec := &recordingEvents{}
	s := newTestServer(t, rec.record)
	client := clientFor(s)

	resp := postHook(t, client, "not-the-token", "term-1", `{"hook_event_name":"Stop"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if len(rec.all()) != 0 {
		t.Fatalf("OnEvent fired on a wrong token")
	}
}

// TestMissingTerminalHeaderRejected is §19.2 case 3: a missing X-Kira-Terminal is a 400 with no
// event.
func TestMissingTerminalHeaderRejected(t *testing.T) {
	rec := &recordingEvents{}
	s := newTestServer(t, rec.record)
	client := clientFor(s)

	resp := postHook(t, client, s.ln.Token, "", `{"hook_event_name":"Stop"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if len(rec.all()) != 0 {
		t.Fatalf("OnEvent fired with no X-Kira-Terminal header")
	}
}

// TestOversizedBodyRejected is §19.2 case 4: a body over maxHookPayloadBytes is a 413 with no
// event.
func TestOversizedBodyRejected(t *testing.T) {
	rec := &recordingEvents{}
	s := newTestServer(t, rec.record)
	client := clientFor(s)

	oversized := `{"hook_event_name":"PreToolUse","message":"` + strings.Repeat("x", maxHookPayloadBytes+1) + `"}`
	resp := postHook(t, client, s.ln.Token, "term-1", oversized)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", resp.StatusCode)
	}
	if len(rec.all()) != 0 {
		t.Fatalf("OnEvent fired for an oversized body")
	}
}

// TestDroppedFieldsNeverReachEvent: tool_input/tool_response/transcript_path in the body never
// appear in the delivered Event — Event has no field for any of the three, so this documents the
// guarantee encoding/json's own unknown-field skip already provides, over a payload that carries
// all three.
func TestDroppedFieldsNeverReachEvent(t *testing.T) {
	rec := &recordingEvents{}
	s := newTestServer(t, rec.record)
	client := clientFor(s)

	body := `{
		"hook_event_name": "PostToolUse",
		"session_id": "sess-1",
		"cwd": "/repo",
		"tool_name": "Write",
		"tool_use_id": "tu-2",
		"transcript_path": "/Users/x/.claude/projects/foo/transcript.jsonl",
		"tool_input": {"file_path": "/repo/secret.txt", "content": "sensitive contents here"},
		"tool_response": {"filePath": "/repo/secret.txt", "content": "sensitive contents here"}
	}`
	resp := postHook(t, client, s.ln.Token, "term-1", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	events := rec.wait(t, 1)
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	got := events[0]
	if got.ToolName != "Write" || got.ToolUseID != "tu-2" {
		t.Fatalf("expected fields did not survive: %+v", got)
	}
	// Event has no TranscriptPath/ToolInput/ToolResponse field at all — this call over the real
	// struct is the guarantee itself, not a string search over the response.
}

// TestCloseRemovesDirectoryAndSocket is §19.2 case 6.
func TestCloseRemovesDirectoryAndSocket(t *testing.T) {
	s := newTestServer(t, nil)
	dir := s.ln.Dir
	sock := s.ln.SockPath
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("dir should exist before Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("dir still exists after Close: err=%v", err)
	}
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Fatalf("socket still exists after Close: err=%v", err)
	}
}

// TestShellSingleQuoteRefusesUnquotable is config.go's own non-HTTP case (§19.2's closing note):
// shellSingleQuote refuses a path holding a single quote or a newline, a hard error rather than a
// best-effort escape.
func TestShellSingleQuoteRefusesUnquotable(t *testing.T) {
	if _, err := shellSingleQuote("/tmp/kira-agent-abc/hook"); err != nil {
		t.Fatalf("ordinary path should not be refused: %v", err)
	}
	if _, err := shellSingleQuote("/tmp/kira-agent-'abc/hook"); err == nil {
		t.Fatalf("a path with a single quote should be refused")
	}
	if _, err := shellSingleQuote("/tmp/kira-agent-\nabc/hook"); err == nil {
		t.Fatalf("a path with a newline should be refused")
	}
}

// TestMessageTruncatedOnRuneBoundary guards §6.1's own bound: a Notification message over
// maxHookMessageBytes is truncated on a rune boundary, never splitting a multi-byte character.
func TestMessageTruncatedOnRuneBoundary(t *testing.T) {
	rec := &recordingEvents{}
	s := newTestServer(t, rec.record)
	client := clientFor(s)

	// A run of a 3-byte rune (€) well past maxHookMessageBytes — a byte-blind slice at exactly
	// maxHookMessageBytes would land mid-character on many offsets.
	var b strings.Builder
	for i := 0; i < maxHookMessageBytes; i++ {
		b.WriteRune('€')
	}
	msg := b.String()

	body := `{"hook_event_name":"Stop","message":` + jsonString(msg) + `,"last_assistant_message":` + jsonString(msg) + `}`
	resp := postHook(t, client, s.ln.Token, "term-1", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	events := rec.wait(t, 1)
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	got := events[0].Message
	if len(got) > maxHookMessageBytes {
		t.Fatalf("len(Message) = %d, want <= %d", len(got), maxHookMessageBytes)
	}
	if !isValidUTF8(got) {
		t.Fatalf("truncated message is not valid UTF-8: %q", got)
	}
	if last := events[0].LastAssistantMessage; last != got {
		t.Fatalf("LastAssistantMessage = %q, want same bound as Message %q", last, got)
	}
}

func jsonString(s string) string {
	var buf bytes.Buffer
	buf.WriteByte('"')
	for _, r := range s {
		if r == '"' || r == '\\' {
			buf.WriteByte('\\')
		}
		buf.WriteRune(r)
	}
	buf.WriteByte('"')
	return buf.String()
}

func isValidUTF8(s string) bool {
	return utf8.ValidString(s)
}

// TestShimInertWithoutSessionEnv guards that a loaded hooks.json outside a Kira session reaches
// nothing: without the session env the shim exits before any request; with it, one arrives.
func TestShimInertWithoutSessionEnv(t *testing.T) {
	curl, err := exec.LookPath("curl")
	if err != nil {
		t.Skip("no curl")
	}
	dir, err := os.MkdirTemp("", "ks")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	var hits atomic.Int32
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	body, err := buildShim(curl, sock)
	if err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(dir, "hook")
	if err := os.WriteFile(shim, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	run := func(env ...string) {
		t.Helper()
		cmd := exec.Command(shim)
		cmd.Env = append([]string{"PATH=" + os.Getenv("PATH")}, env...)
		cmd.Stdin = strings.NewReader(`{"hook_event_name":"Stop"}`)
		if err := cmd.Run(); err != nil {
			t.Fatalf("shim exited non-zero: %v", err)
		}
	}

	run()
	run("KIRA_AGENT_HOOK_TOKEN=tok")
	run("KIRA_TERMINAL_ID=t1")
	if n := hits.Load(); n != 0 {
		t.Fatalf("shim without session env reached the server %d time(s), want 0", n)
	}
	run("KIRA_AGENT_HOOK_TOKEN=tok", "KIRA_TERMINAL_ID=t1")
	if n := hits.Load(); n != 1 {
		t.Fatalf("shim with session env reached the server %d time(s), want 1", n)
	}
}

// TestReplyIsSentBeforeOnEventRuns: a slow OnEvent must not delay the answer the agent's hook waits
// for, and a Stop block still reaches the agent.
func TestReplyIsSentBeforeOnEventRuns(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{})
	s, err := New(harness.HookOptions{
		Reply:   func(harness.AgentEvent) harness.HookReply { return harness.HookReply{Block: "lease granted"} },
		OnEvent: func(harness.AgentEvent) { close(entered); <-release },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	resp := postHook(t, clientFor(s), s.ln.Token, "term-1", `{"hook_event_name":"Stop"}`)
	defer resp.Body.Close()
	var body struct{ Decision, Reason string }
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	if body.Decision != "block" || body.Reason != "lease granted" {
		t.Fatalf("reply = %+v, want the block", body)
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("OnEvent never ran")
	}
	close(release)
}
