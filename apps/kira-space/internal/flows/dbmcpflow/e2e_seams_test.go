package dbmcpflow

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

// TestE2ESeams proves the Wave 0 seams of P686 each work once: a Studio session, the approval timeout,
// the /forms route and the leak oracle.
func TestE2ESeams(t *testing.T) {
	const secret = "se&cret value/1"
	f := newFixture(t, flowharness.WithApprovalTimeout(300*time.Millisecond))
	captureLog(t)

	t.Run("studio session", func(t *testing.T) {
		sess, end := f.studioSession(t, "s1")
		res := callTool(t, sess, "list_connections", map[string]any{})
		if res.IsError {
			t.Fatalf("list_connections = %q", text(res))
		}
		f.keep(t, "session:s1", "k", `"v"`)
		if id := f.keptID(t, "session:s1", "k"); id == "" {
			t.Fatal("no kept id")
		}
		end()
		if n := len(f.app.W.Kept.List("session:s1")); n != 0 {
			t.Fatalf("%d kept variables after the session ended, want 0", n)
		}
	})

	t.Run("approval timeout", func(t *testing.T) {
		sess := f.externalSession(t)
		done := callAsync(sess, f.connID, "insert into notes (body) values ('late')")
		f.waitQueue(t, 1)
		res := wait(t, done)
		if !res.IsError || !strings.Contains(text(res), "got no answer within") {
			t.Fatalf("unanswered write = %q, want a refusal", text(res))
		}
		if n := f.queueLen(t); n != 0 {
			t.Fatalf("queue = %d after the timeout, want 0", n)
		}
	})

	t.Run("forms route", func(t *testing.T) {
		srv := flowharness.HTTP(t)
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/forms", nil)
		req.Header.Set("X-Echo", secret)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var got map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		forms := leakForms(t, secret)
		want := map[string]string{"raw": forms[0], "json": forms[1], "query": forms[3], "path": forms[4], "b64": forms[5], "b64url": forms[6]}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("forms[%s] = %q, want %q", k, got[k], v)
			}
		}
	})

	t.Run("leak oracle", func(t *testing.T) {
		f.noLeak(t, secret)
		if hits := f.leakHits(t, secret, "echoed "+secret); len(hits) != 1 {
			t.Fatalf("hits = %v, want exactly the agent text", hits)
		}
	})
}
