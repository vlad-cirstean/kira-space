package dbmcpflow

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Backend half of tests/ui/studio/e2e-approvals-sessions.spec.ts (contract e2e-approvals-sessions): two agent
// sessions with two pending cards each. Every queue item names its session, so the dialog groups by it; denying
// one session's ids answers its agents and leaves the other session's cards pending.

const sessionsKey = "e2e-approvals-sessions"

func TestE2EApprovalsSessions(t *testing.T) {
	captureLog(t)
	f := newFixture(t)
	srv, _, envID := f.apiSetup(t, "127.0.0.1", false)
	a, _ := f.studioSession(t, "sess-a")
	b, _ := f.studioSession(t, "sess-b")
	opts := f.e2eOpts(srv.URL)
	req := func(n string) map[string]any { return apiArgs(envID, "GET", srv.URL+"/echo?n="+n) }

	calls := map[string]chan callOutcome{}
	for i, key := range []string{"a1", "a2", "b1", "b2"} {
		sess := a
		if strings.HasPrefix(key, "b") {
			sess = b
		}
		calls[key] = callToolAsync(sess, "api_request", req(key))
		f.items(t, i+1)
	}
	items := f.items(t, 4)
	bySession := map[string][]string{}
	for _, it := range items {
		if it.Origin.Session == "" {
			t.Fatalf("item %s has no origin.session, want the agent session", it.ID)
		}
		bySession[it.Origin.Session] = append(bySession[it.Origin.Session], it.ID)
	}
	if len(bySession) != 2 {
		t.Fatalf("sessions = %v, want two groups", bySession)
	}
	f.app.Contract(t, sessionsKey, "ApprovalsService.List", items, opts...)

	// The first two items belong to session A: they were raised first.
	idsA := []string{items[0].ID, items[1].ID}
	if got := bySession[items[0].Origin.Session]; !slices.Equal(got, idsA) {
		t.Fatalf("session of the first item holds %v, want %v", got, idsA)
	}
	f.app.Contract(t, sessionsKey, "args:ApprovalsService.Answer", approvals.AnswerArgs{IDs: idsA, Approve: false})
	f.answerItems(t, false, idsA...)
	for _, k := range []string{"a1", "a2"} {
		res := wait(t, calls[k])
		if !res.IsError || !strings.Contains(strings.ToLower(text(res)), "den") {
			t.Fatalf("%s result = %q, want the denial text", k, text(res))
		}
	}
	rest := f.items(t, 2)
	if rest[0].ID != items[2].ID || rest[1].ID != items[3].ID {
		t.Fatalf("pending = %v, want session B's two cards untouched", []string{rest[0].ID, rest[1].ID})
	}
	for _, k := range []string{"b1", "b2"} {
		select {
		case out := <-calls[k]:
			t.Fatalf("%s answered early: %v", k, out)
		case <-time.After(100 * time.Millisecond):
		}
	}
	f.answerItems(t, false, items[2].ID, items[3].ID)
	for _, k := range []string{"b1", "b2"} {
		wait(t, calls[k])
	}
	testx.WaitUntil(t, 10*time.Second, func() bool { return f.queueLen(t) == 0 })
}
