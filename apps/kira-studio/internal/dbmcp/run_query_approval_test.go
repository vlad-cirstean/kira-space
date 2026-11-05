package dbmcp

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/mask"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/maskreview"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeConnsMutable is a ConnectionsReader whose one connection's modes can change mid-test —
// exactly what a real user flipping the connection's MCP tab while a prompt sits open would do.
type fakeConnsMutable struct {
	mu      sync.Mutex
	summary model.ConnectionSummary
}

func (f *fakeConnsMutable) List() ([]model.ConnectionSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return []model.ConnectionSummary{f.summary}, nil
}

func (f *fakeConnsMutable) StateOf(string) model.ConnectionState {
	return model.ConnectionState{Status: "connected"}
}

func (f *fakeConnsMutable) ConnectContext(context.Context, string) (model.ConnectionState, error) {
	return model.ConnectionState{Status: "connected"}, nil
}

func (f *fakeConnsMutable) setWriteMode(mode string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.summary.McpWriteMode = mode
}

func (f *fakeConnsMutable) setReadMode(mode string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.summary.McpReadMode = mode
}

// fakeQueryRunner always classifies as a fixed class and counts executions — the assertion this
// test cares about is whether Execute ever runs, not what it returns.
type fakeQueryRunner struct {
	class    adapters.OpClass
	executed atomic.Int32
	// lane counts ExecuteRead calls (the read lane); main counts Execute calls.
	lane atomic.Int32
	main atomic.Int32
}

func (f *fakeQueryRunner) ClassifyStatement(context.Context, string, string) (adapters.OpClass, error) {
	return f.class, nil
}

func (f *fakeQueryRunner) Execute(context.Context, adapterhost.ExecuteRequestWire) (adapterhost.ExecuteResponse, error) {
	f.executed.Add(1)
	f.main.Add(1)
	return adapterhost.ExecuteResponse{}, nil
}

func (f *fakeQueryRunner) ExecuteRead(context.Context, adapterhost.ExecuteRequestWire) (adapterhost.ExecuteResponse, error) {
	f.executed.Add(1)
	f.lane.Add(1)
	return adapterhost.ExecuteResponse{}, nil
}

type fakeColumnReviewNone struct{}

func (fakeColumnReviewNone) For(context.Context, model.ConnectionSummary) maskreview.Withheld {
	return maskreview.Withheld{}
}
func (fakeColumnReviewNone) Cached(string) (maskreview.Withheld, bool) {
	return maskreview.Withheld{}, false
}
func (fakeColumnReviewNone) MarkStale(string) {}

type fakeMaskRulesNone struct{}

func (fakeMaskRulesNone) MaskSetFor(string) (mask.Set, error)   { return mask.Set{}, nil }
func (fakeMaskRulesNone) List(string) ([]model.MaskRule, error) { return nil, nil }

// TestRunQueryRefusesApprovedWriteAfterPermissionRevokedMidWait: summary/verdict
// are resolved before ApprovalBroker.Request's own up-to-2-minute wait, and were never re-checked
// after Approve landed. A user reacting to an unexpected write prompt by tightening the
// connection's write mode to deny, then someone (or the same person) approving the now-stale
// prompt, must not run the statement anyway.
func TestRunQueryRefusesApprovedWriteAfterPermissionRevokedMidWait(t *testing.T) {
	conns := &fakeConnsMutable{summary: model.ConnectionSummary{
		ID: "c1", Name: "conn", Kind: "sqlite", McpEnabled: true,
		McpReadMode: "deny", McpWriteMode: "prompt", McpDdlMode: "deny",
	}}
	qr := &fakeQueryRunner{class: adapters.ClassWrite}
	approvals := NewApprovalBroker(time.Now, nil)
	s := &Server{cfg: Config{
		Conns: conns, Query: qr, Approvals: approvals, MaskRules: fakeMaskRulesNone{}, ColumnReview: fakeColumnReviewNone{},
		ExplainThreshold: func() int { return 1000 },
		PreviewResponses: func() bool { return false },
	}}

	type outcome struct {
		errText string
		isErr   bool
	}
	resCh := make(chan outcome, 1)
	go func() {
		res, _, _ := s.runQuery(context.Background(), nil, runQueryArgs{ConnectionID: "c1", SQL: "UPDATE t SET x = 1", Mode: modeWrite})
		if res == nil || !res.IsError || len(res.Content) == 0 {
			resCh <- outcome{isErr: res != nil && res.IsError}
			return
		}
		text := ""
		if tc, ok := res.Content[0].(*mcp.TextContent); ok {
			text = tc.Text
		}
		resCh <- outcome{errText: text, isErr: true}
	}()

	// Wait for the prompt to appear, then react to it exactly like a user would: tighten the mode
	// before answering.
	deadline := time.After(2 * time.Second)
	var requestID string
	for requestID == "" {
		select {
		case <-deadline:
			t.Fatal("run_query never raised an approval request")
		default:
		}
		if items := approvals.Queue.List(); len(items) > 0 {
			requestID = strings.TrimPrefix(items[0].ID, items[0].Kind+":")
		} else {
			time.Sleep(time.Millisecond)
		}
	}
	conns.setWriteMode("deny")
	if got := approvals.Approve(requestID); got != ApprovalActionResolved {
		t.Fatalf("Approve: got %v", got)
	}

	select {
	case out := <-resCh:
		if !out.isErr {
			t.Fatal("runQuery succeeded after the write mode was revoked mid-approval, want a refusal")
		}
		if !strings.Contains(out.errText, "now denies") {
			t.Fatalf("error text = %q, want it to mention the permissions now denying the statement", out.errText)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runQuery never returned after Approve")
	}

	if n := qr.executed.Load(); n != 0 {
		t.Fatalf("Execute was called %d times, want 0 — the statement must never run once permissions were revoked", n)
	}
}

// The declared mode picks the connection: read runs on the read lane whatever the write permission,
// write runs on the main connection.
func TestRunQueryModePicksConnection(t *testing.T) {
	for _, tc := range []struct {
		mode       string
		class      adapters.OpClass
		lane, main int32
	}{
		{modeRead, adapters.ClassRead, 1, 0},
		{modeWrite, adapters.ClassWrite, 0, 1},
		{modeWrite, adapters.ClassRead, 0, 1},
	} {
		conns := &fakeConnsMutable{summary: model.ConnectionSummary{
			ID: "c1", Name: "conn", Kind: "sqlite", McpEnabled: true,
			McpReadMode: "allow", McpWriteMode: "allow", McpDdlMode: "deny",
		}}
		qr := &fakeQueryRunner{class: tc.class}
		s := &Server{cfg: Config{
			Conns: conns, Query: qr, Approvals: NewApprovalBroker(time.Now, nil), MaskRules: fakeMaskRulesNone{}, ColumnReview: fakeColumnReviewNone{},
			ExplainThreshold: func() int { return 1000 },
			PreviewResponses: func() bool { return false },
		}}
		if _, _, err := s.runQuery(context.Background(), nil, runQueryArgs{ConnectionID: "c1", SQL: "SELECT 1", Mode: tc.mode}); err != nil {
			t.Fatalf("mode=%s class=%s: %v", tc.mode, tc.class, err)
		}
		if qr.lane.Load() != tc.lane || qr.main.Load() != tc.main {
			t.Errorf("mode=%s class=%s: lane=%d main=%d, want %d/%d", tc.mode, tc.class, qr.lane.Load(), qr.main.Load(), tc.lane, tc.main)
		}
	}
}
