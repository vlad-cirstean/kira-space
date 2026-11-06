package dbmcp

import (
	"context"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tree"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeConnsList is a minimal ConnectionsReader stub whose List() returns a fixed slice — just
// enough to drive resolveEnabled/modesOf for the schema-browsing tools below.
type fakeConnsList struct {
	summaries []model.ConnectionSummary
}

func (f *fakeConnsList) List() ([]model.ConnectionSummary, error) { return f.summaries, nil }
func (f *fakeConnsList) StateOf(string) model.ConnectionState     { return model.ConnectionState{} }
func (f *fakeConnsList) ConnectContext(context.Context, string) (model.ConnectionState, error) {
	return model.ConnectionState{}, nil
}

// fakeTree records whether any of its three methods was ever called — the schema-browsing tools'
// own gate (M7 finding #8) must refuse before reaching any of them, never after.
type fakeTree struct {
	calls int
}

func (f *fakeTree) Children(context.Context, string, string, bool) (tree.ChildrenResult, error) {
	f.calls++
	return tree.ChildrenResult{}, nil
}

func (f *fakeTree) Describe(context.Context, string, string, bool, *string) (tree.DescribeResult, error) {
	f.calls++
	return tree.DescribeResult{}, nil
}

func (f *fakeTree) SchemaColumns(context.Context, string, string, bool) (tree.SchemaColumnsResult, error) {
	f.calls++
	return tree.SchemaColumnsResult{}, nil
}

func mcpEnabledSummary(id, readMode string) model.ConnectionSummary {
	s := model.ConnectionSummary{ID: id, Name: "conn"}
	s.McpEnabled = true
	s.McpReadMode = readMode
	s.McpWriteMode = "deny"
	s.McpDdlMode = "deny"
	return s
}

func isErrorResult(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if res == nil || !res.IsError {
		t.Fatalf("result = %+v, want an error result", res)
	}
	if len(res.Content) == 0 {
		t.Fatal("error result has no content")
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] = %T, want *mcp.TextContent", res.Content[0])
	}
	return tc.Text
}

// TestSchemaBrowsingToolsRefuseWhenReadModeDeny: listChildren/describeTable/
// describeSchema previously checked only resolveEnabled (mcp_enabled) and never consulted the
// connection's own read permission mode at all — a connection whose read mode is "deny" still had
// its full schema browsable by an MCP client. Each must refuse before ever reaching the tree
// service.
func TestSchemaBrowsingToolsRefuseWhenReadModeDeny(t *testing.T) {
	conns := &fakeConnsList{summaries: []model.ConnectionSummary{mcpEnabledSummary("c1", "deny")}}
	tr := &fakeTree{}
	s := &Server{cfg: Config{Conns: conns, Tree: tr}}
	ctx := context.Background()

	t.Run("listChildren", func(t *testing.T) {
		res, _, err := s.listChildren(ctx, nil, listChildrenArgs{ConnectionID: "c1"})
		if err != nil {
			t.Fatalf("listChildren: %v", err)
		}
		if got := isErrorResult(t, res); !strings.Contains(got, "deny read statements") {
			t.Fatalf("listChildren error = %q, want it to mention denied read statements", got)
		}
	})

	t.Run("describeTable", func(t *testing.T) {
		res, _, err := s.describeTable(ctx, nil, describeTableArgs{ConnectionID: "c1", Path: "t"})
		if err != nil {
			t.Fatalf("describeTable: %v", err)
		}
		if got := isErrorResult(t, res); !strings.Contains(got, "deny read statements") {
			t.Fatalf("describeTable error = %q, want it to mention denied read statements", got)
		}
	})

	t.Run("describeSchema", func(t *testing.T) {
		res, _, err := s.describeSchema(ctx, nil, describeSchemaArgs{ConnectionID: "c1", Path: "s"})
		if err != nil {
			t.Fatalf("describeSchema: %v", err)
		}
		if got := isErrorResult(t, res); !strings.Contains(got, "deny read statements") {
			t.Fatalf("describeSchema error = %q, want it to mention denied read statements", got)
		}
	})

	if tr.calls != 0 {
		t.Fatalf("tree service called %d times, want 0 — the deny gate must refuse before reaching it", tr.calls)
	}
}

// TestSchemaBrowsingToolsAllowWhenReadModeAllow is the non-regression complement: a connection
// whose read mode is "allow" must still reach the tree service exactly as before this finding's
// fix — the new gate must not false-refuse the common case.
func TestSchemaBrowsingToolsAllowWhenReadModeAllow(t *testing.T) {
	conns := &fakeConnsList{summaries: []model.ConnectionSummary{mcpEnabledSummary("c1", "allow")}}
	tr := &fakeTree{}
	s := &Server{cfg: Config{Conns: conns, Tree: tr}}
	ctx := context.Background()

	if _, _, err := s.listChildren(ctx, nil, listChildrenArgs{ConnectionID: "c1"}); err != nil {
		t.Fatalf("listChildren: %v", err)
	}
	if _, _, err := s.describeTable(ctx, nil, describeTableArgs{ConnectionID: "c1", Path: "t"}); err != nil {
		t.Fatalf("describeTable: %v", err)
	}
	if _, _, err := s.describeSchema(ctx, nil, describeSchemaArgs{ConnectionID: "c1", Path: "s"}); err != nil {
		t.Fatalf("describeSchema: %v", err)
	}
	if tr.calls != 3 {
		t.Fatalf("tree service called %d times, want 3 (one per tool) when read mode is allow", tr.calls)
	}
}
