package dbmcp

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestWithPanicRecoveryRecoversAndHidesPanicText: go-sdk runs every tool handler in its
// own goroutine with no recover() of its own, so an unguarded panic (nil deref, index out of
// range) would kill the whole desktop app. The wrapper must recover, log the stack, and return a
// generic IsError result that never repeats the panic value — on a masked connection that value
// could carry a row's own data (mirrors maskedToolError's posture).
func TestWithPanicRecoveryRecoversAndHidesPanicText(t *testing.T) {
	var logBuf bytes.Buffer
	s := &Server{log: slog.New(slog.NewTextHandler(&logBuf, nil))}

	type args struct{}
	panicky := func(context.Context, *mcp.CallToolRequest, args) (*mcp.CallToolResult, any, error) {
		var p *int
		_ = *p // nil deref — this string never appears in the panic value, only in this comment
		return nil, nil, nil
	}
	wrapped := withPanicRecovery(s, "fake_tool", panicky)

	res, out, err := wrapped(context.Background(), nil, args{})
	if err != nil {
		t.Fatalf("err = %v, want nil (a recovered panic surfaces as an IsError result, not a Go error)", err)
	}
	if out != nil {
		t.Fatalf("out = %v, want nil", out)
	}
	if res == nil || !res.IsError {
		t.Fatalf("res = %+v, want a non-nil IsError result", res)
	}
	if len(res.Content) == 0 {
		t.Fatal("res.Content is empty, want a generic error message")
	}
	text := ""
	if tc, ok := res.Content[0].(*mcp.TextContent); ok {
		text = tc.Text
	}
	if strings.Contains(text, "nil pointer") || strings.Contains(text, "runtime error") {
		t.Fatalf("result text = %q, want no panic-derived text reaching the client", text)
	}
	if text == "" {
		t.Fatal("result text is empty, want a generic internal-error message")
	}
	if !strings.Contains(logBuf.String(), "fake_tool") {
		t.Fatalf("log output = %q, want it to name the panicking tool", logBuf.String())
	}
}

// TestWithPanicRecoveryPassesThroughNormalResults confirms the wrapper is a no-op on a handler
// that returns normally — no swallowed result, no spurious recovery.
func TestWithPanicRecoveryPassesThroughNormalResults(t *testing.T) {
	s := &Server{log: slog.Default()}
	type args struct{}
	ok := func(context.Context, *mcp.CallToolRequest, args) (*mcp.CallToolResult, any, error) {
		return jsonResult(map[string]any{"ok": true})
	}
	wrapped := withPanicRecovery(s, "fake_tool", ok)

	res, _, err := wrapped(context.Background(), nil, args{})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("res = %+v, want a non-error result", res)
	}
	if len(res.Content) == 0 {
		t.Fatal("res.Content is empty, want the handler's own JSON result")
	}
	text := ""
	if tc, ok := res.Content[0].(*mcp.TextContent); ok {
		text = tc.Text
	}
	if !strings.Contains(text, `"ok":true`) {
		t.Fatalf("result text = %q, want the handler's own result to pass through unmodified", text)
	}
}
