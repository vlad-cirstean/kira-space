package dbmcpflow

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/notifysink"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func (f *fixture) dbmcpEntries(t *testing.T, n int) []prompts.Routed {
	t.Helper()
	var got []prompts.Routed
	testx.WaitUntil(t, 10*time.Second, func() bool {
		got = got[:0]
		all, err := f.app.W.PromptsSvc.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range all {
			if p.Kind == prompts.KindApproval {
				got = append(got, p)
			}
		}
		return len(got) == n
	})
	return got
}

func TestDbMcpApprovalRoutes(t *testing.T) {
	f := newFixture(t)
	sink := notifysink.New()
	f.app.W.Prompts.SetSink(sink)
	f.app.W.Windows.Add("w1", 0, nil, func() {})

	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	const write = "insert into notes (body) values ('three')"
	done := make(chan *mcp.CallToolResult, 1)
	go func() {
		res, _ := sess.CallTool(ctx, &mcp.CallToolParams{Name: "run_query", Arguments: map[string]any{"connectionId": f.connID, "sql": write, "path": "", "mode": "write"}})
		done <- res
	}()

	req := f.pending(t)
	e := f.dbmcpEntries(t, 1)[0]
	if e.Ref != req.Item.ID || e.Target != "w1" || e.Origin != "" {
		t.Fatalf("entry = %+v, want ref %s targeted at w1", e, req.Item.ID)
	}
	if strings.Contains(e.Title, "insert") || !strings.Contains(e.Title, "notes db") {
		t.Fatalf("title %q must name the connection and never the statement", e.Title)
	}
	f.app.StudioContract(t, "dbmcp-route", "PromptsService.List#approval", e, flowharness.Mask("createdAt", "id", "ref"))
	note, ok := sink.Shown("prompt:approval")
	if !ok || strings.Contains(note.Title+note.Body, "insert") {
		t.Fatalf("note = %+v, %v; want one without the statement", note, ok)
	}

	f.answerItems(t, false, req.Item.ID)
	if res := <-done; res == nil || !res.IsError {
		t.Fatalf("denied write = %+v, want a tool error", res)
	}
	f.dbmcpEntries(t, 0)
	testx.WaitUntil(t, 10*time.Second, func() bool { _, ok := sink.Shown("prompt:approval"); return !ok })

	// A request abandoned by its client withdraws the popup too.
	abandon, cancel := context.WithCancel(ctx)
	go func() {
		_, _ = sess.CallTool(abandon, &mcp.CallToolParams{Name: "run_query", Arguments: map[string]any{"connectionId": f.connID, "sql": write, "path": "", "mode": "write"}})
	}()
	f.pending(t)
	f.dbmcpEntries(t, 1)
	cancel()
	f.dbmcpEntries(t, 0)
}
