package dbmcpflow

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/dbmcp"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/approvals"
)

// Shared seams of the e2e scenario tests (plan P686 section 3).

// safeBuf is a bytes.Buffer slog can write to while the test reads it.
type safeBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// logCapture is the buffer captureLog installed for the running test; noLeak reads it.
var logCapture *safeBuf

// captureLog sends slog's default logger to a Debug JSON handler over a buffer until the test ends.
func captureLog(t *testing.T) *safeBuf {
	t.Helper()
	buf := &safeBuf{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	logCapture = buf
	t.Cleanup(func() { slog.SetDefault(prev); logCapture = nil })
	return buf
}

// studioSession opens an MCP client on the per-session Studio server as session id. release ends the
// session: it clears the session's kept variables and abandons its parked approvals.
func (f *fixture) studioSession(t *testing.T, id string) (*mcp.ClientSession, func()) {
	t.Helper()
	entry, release, err := f.app.W.DbMcpSessions.Grant(dbmcp.SessionIdentity{SessionID: id, Cwd: f.app.Work})
	if err != nil {
		t.Fatalf("grant session %s: %v", id, err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "flow-studio-client", Version: "1"}, nil)
	sess, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   entry.URL,
		HTTPClient: &http.Client{Transport: headers(entry.Headers), Timeout: 30 * time.Second},
	}, nil)
	if err != nil {
		release()
		t.Fatalf("connect session %s: %v", id, err)
	}
	var closed sync.Once
	end := func() {
		closed.Do(func() {
			_ = sess.Close()
			release()
		})
	}
	t.Cleanup(end)
	return sess, end
}

type headers map[string]string

func (h headers) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for k, v := range h {
		r.Header.Set(k, v)
	}
	return http.DefaultTransport.RoundTrip(r)
}

// externalSession opens one more token-bearing client on the external server, closed with the test.
func (f *fixture) externalSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

// keptID is the id of the live kept variable name in scope.
func (f *fixture) keptID(t *testing.T, scope, name string) string {
	t.Helper()
	for _, m := range f.app.W.Kept.List(scope) {
		if m.Name == name {
			return m.ID
		}
	}
	t.Fatalf("no kept variable %q in scope %q", name, scope)
	return ""
}

// waitQueue waits for exactly n approval items, oldest first.
func (f *fixture) waitQueue(t *testing.T, n int) []approvals.Item { return f.items(t, n) }

// opsRecent is the last 500 audit rows, newest first.
func (f *fixture) opsRecent(t *testing.T) []oplog.Record {
	t.Helper()
	recs, err := f.app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	return recs
}

// leakForms are the shapes value takes on its way into a request, computed here and not by the code
// under test.
func leakForms(t testing.TB, value string) []string {
	t.Helper()
	if value == "" {
		t.Fatal("leak check needs a value")
	}
	j, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var plain bytes.Buffer
	enc := json.NewEncoder(&plain)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		t.Fatal(err)
	}
	noHTML := strings.TrimSuffix(plain.String(), "\n")
	return []string{
		value,
		string(j[1 : len(j)-1]),
		noHTML[1 : len(noHTML)-1],
		url.QueryEscape(value),
		url.PathEscape(value),
		base64.StdEncoding.EncodeToString([]byte(value)),
		base64.RawURLEncoding.EncodeToString([]byte(value)),
	}
}

// leakHits names every place value shows up in any form: the given agent texts, the audit rows, captured
// slog, emitted events, the approval queue, prompt entries, the tool list of both servers, kept
// variables and every file under the space home.
func (f *fixture) leakHits(t *testing.T, value string, agentTexts ...string) []string {
	t.Helper()
	forms := leakForms(t, value)
	var hits []string
	check := func(where, haystack string) {
		for _, form := range forms {
			if strings.Contains(haystack, form) {
				hits = append(hits, where+" holds "+form)
				return
			}
		}
	}
	for i, s := range agentTexts {
		check("agent text "+string(rune('0'+i%10)), s)
	}
	check("ops", mustJSON(t, f.opsRecent(t)))
	if logCapture != nil {
		check("slog", logCapture.String())
	}
	check("events", mustJSON(t, f.app.Events.Since(0, "")))
	check("approvals", mustJSON(t, f.list(t)))
	prompts, err := f.app.W.PromptsSvc.List()
	if err != nil {
		t.Fatal(err)
	}
	check("prompts", mustJSON(t, prompts))
	kept, err := f.app.W.KeptVars.List()
	if err != nil {
		t.Fatal(err)
	}
	check("kept list", mustJSON(t, kept))
	check("external tools", f.toolList(t, f.externalSession(t)))
	studio, end := f.studioSession(t, "leak-probe")
	check("studio tools", f.toolList(t, studio))
	end()
	err = filepath.WalkDir(f.app.SpaceHome, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		check("file "+strings.TrimPrefix(p, f.app.SpaceHome), string(b))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

// noLeak fails when value, in any form, shows up in an agent text or anywhere the app records state.
func (f *fixture) noLeak(t *testing.T, value string, agentTexts ...string) {
	t.Helper()
	for _, h := range f.leakHits(t, value, agentTexts...) {
		t.Errorf("leak: %s", h)
	}
}

func (f *fixture) toolList(t *testing.T, sess *mcp.ClientSession) string {
	t.Helper()
	list, err := sess.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	return mustJSON(t, list.Tools)
}
