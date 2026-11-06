package dbmcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestExplainQueryRefusesApprovedReadAfterReadModeDeniedMidWait: explainQuery's own
// approval path resolves summary/verdict before ApprovalBroker.Request's own up-to-2-minute wait,
// mirroring runQuery's awaitApproval — an Approve landing after the user tightened read mode to
// deny mid-wait must not still run the composed EXPLAIN.
func TestExplainQueryRefusesApprovedReadAfterReadModeDeniedMidWait(t *testing.T) {
	conns := &fakeConnsMutable{summary: model.ConnectionSummary{
		ID: "c1", Name: "conn", Kind: "postgres", McpEnabled: true,
		McpReadMode: "prompt", McpWriteMode: "deny", McpDdlMode: "deny",
	}}
	qr := &fakeQueryRunner{class: adapters.ClassRead}
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
		res, _, _ := s.explainQuery(context.Background(), nil, explainQueryArgs{ConnectionID: "c1", SQL: "SELECT * FROM t", Path: "database:app"})
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

	deadline := time.After(2 * time.Second)
	var requestID string
	for requestID == "" {
		select {
		case <-deadline:
			t.Fatal("explain_query never raised an approval request")
		default:
		}
		if items := approvals.Queue.List(); len(items) > 0 {
			requestID = strings.TrimPrefix(items[0].ID, items[0].Kind+":")
		} else {
			time.Sleep(time.Millisecond)
		}
	}
	conns.setReadMode("deny")
	if got := approvals.Approve(requestID); got != ApprovalActionResolved {
		t.Fatalf("Approve: got %v", got)
	}

	select {
	case out := <-resCh:
		if !out.isErr {
			t.Fatal("explainQuery succeeded after read mode was denied mid-approval, want a refusal")
		}
		if !strings.Contains(out.errText, "now deny") {
			t.Fatalf("error text = %q, want it to mention the permissions now denying the statement", out.errText)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("explainQuery never returned after Approve")
	}

	if n := qr.executed.Load(); n != 0 {
		t.Fatalf("Execute was called %d times, want 0 — the statement must never run once read mode was denied", n)
	}
}
