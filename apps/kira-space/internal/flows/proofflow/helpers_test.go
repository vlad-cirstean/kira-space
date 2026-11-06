package proofflow

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/mcpauth"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/proof"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/prompts"
)

var ctx = context.Background()

var urlRe = regexp.MustCompile(`"url":"([^"]+)"`)

type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

// fixture is an app with the MCP server on and a connected client.
type fixture struct {
	app  *flowharness.App
	sess *mcp.ClientSession
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	app := flowharness.New(t)
	st, err := app.W.DbMcp.SetEnabled(bridge.DbMcpSetEnabledArgs{Enabled: true})
	if err != nil || !st.Running {
		t.Fatalf("SetEnabled = %+v (%v), want running", st, err)
	}
	m := urlRe.FindStringSubmatch(st.Command)
	if m == nil {
		t.Fatalf("Status.Command has no server url: %q", st.Command)
	}
	plain, ok, err := mcpauth.LoadHelperToken(mcpauth.HelperTokenPathNamed(app.SpaceHome, "mcp-db"))
	if err != nil || !ok {
		t.Fatalf("helper token: ok=%v err=%v", ok, err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "flow-client", Version: "1"}, nil)
	sess, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: m[1], HTTPClient: &http.Client{Transport: bearer{plain}, Timeout: 30 * time.Second},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return &fixture{app: app, sess: sess}
}

func text(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// call runs a tool and decodes its JSON answer into out (nil to ignore it).
func (f *fixture) call(t *testing.T, name string, args map[string]any, out any) {
	t.Helper()
	res, err := f.sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("%s failed: %s", name, text(res))
	}
	if out != nil {
		if err := json.Unmarshal([]byte(text(res)), out); err != nil {
			t.Fatalf("%s answer %q: %v", name, text(res), err)
		}
	}
}

// fail runs a tool that must be refused and returns the message the agent sees.
func (f *fixture) fail(t *testing.T, name string, args map[string]any) string {
	t.Helper()
	res, err := f.sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return err.Error()
	}
	if !res.IsError {
		t.Fatalf("%s succeeded: %s", name, text(res))
	}
	return text(res)
}

func (f *fixture) start(t *testing.T, name string) string {
	t.Helper()
	var r proof.StartResult
	f.call(t, "proof_start_session", map[string]any{"name": name}, &r)
	return r.DraftID
}

func (f *fixture) addStep(t *testing.T, draft string, args map[string]any) int {
	t.Helper()
	args["draftId"] = draft
	var r struct{ Ordinal int }
	f.call(t, "proof_add_step", args, &r)
	return r.Ordinal
}

func (f *fixture) closeDraft(t *testing.T, draft string) proof.CloseResult {
	t.Helper()
	var r proof.CloseResult
	f.call(t, "proof_close_session", map[string]any{"draftId": draft}, &r)
	return r
}

func tableStep(desc string) map[string]any {
	return map[string]any{
		"description": desc, "view": "table", "query": "select id, name, tags from orders", "language": "sql", "source": "postgres",
		"data": map[string]any{
			"columns": []any{map[string]any{"name": "id"}, map[string]any{"name": "name"}, map[string]any{"name": "tags", "type": "json"}},
			"rows":    []any{[]any{1, "alpha", []any{"a", "b"}}, []any{2, nil, nil}, []any{9007199254740993, "<b>bold</b>", map[string]any{"k": 1}}},
		},
	}
}

func documentStep(desc string) map[string]any {
	return map[string]any{
		"description": desc, "view": "document", "source": "elasticsearch", "language": "json",
		"query": `{"query":{"match_all":{}}}`,
		"data": map[string]any{"documents": []any{
			map[string]any{"body": map[string]any{"z": 1, "a": map[string]any{"nested": true}}},
			map[string]any{"id": "doc-b", "body": map[string]any{"title": "Second"}},
		}},
	}
}

func keyValueStep(desc string) map[string]any {
	return map[string]any{
		"description": desc, "view": "keyvalue",
		"data": map[string]any{"entries": []any{
			map[string]any{"field": "status", "value": "ok"},
			map[string]any{"field": "latencyMs", "value": 12.5},
		}},
	}
}

func proofPrompts(t *testing.T, app *flowharness.App) []prompts.Routed {
	t.Helper()
	all, err := app.W.PromptsSvc.List()
	if err != nil {
		t.Fatal(err)
	}
	var out []prompts.Routed
	for _, p := range all {
		if p.Kind == prompts.KindProof {
			out = append(out, p)
		}
	}
	return out
}

func reviewItems(t *testing.T, app *flowharness.App) []approvals.Item {
	t.Helper()
	all, err := app.W.ApprovalsSvc.List()
	if err != nil {
		t.Fatal(err)
	}
	var out []approvals.Item
	for _, it := range all {
		if it.Kind == approvals.KindProofReview {
			out = append(out, it)
		}
	}
	return out
}

func session(t *testing.T, app *flowharness.App, id string) proof.Detail {
	t.Helper()
	d, err := app.W.Proof.Session(bridge.ProofSessionArgs{SessionID: id})
	if err != nil {
		t.Fatalf("Session(%s): %v", id, err)
	}
	return d
}

func sessionID(t *testing.T, app *flowharness.App, name string) string {
	t.Helper()
	list, err := app.W.Proof.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range list {
		if s.Name == name {
			return s.ID
		}
	}
	t.Fatalf("no session named %q in %+v", name, list)
	return ""
}

func chartStep(desc, source string, data map[string]any) map[string]any {
	return map[string]any{"description": desc, "view": "chart", "source": source, "data": data}
}

func lineChartStep() map[string]any {
	step := chartStep("Errors per minute around the deploy", "elasticsearch", map[string]any{
		"type": "line",
		"x":    map[string]any{"type": "time", "name": "time"},
		"y":    map[string]any{"name": "errors", "unit": "req"},
		"series": []any{
			map[string]any{"name": "before", "points": []any{
				[]any{"2026-01-15T12:02:00Z", 4}, []any{"2026-01-15T12:00:00Z", 7}, []any{1768478460000, 5},
			}},
			map[string]any{"name": "after <b>fix</b>", "points": []any{
				[]any{"2026-01-15T13:03:00+01:00", 1}, []any{"2026-01-15T12:04:00.500Z", nil}, []any{"2026-01-15T12:05:00Z", 0},
			}},
		},
	})
	step["language"] = "json"
	step["query"] = `{"aggs":{"per_minute":{"date_histogram":{"field":"@timestamp","fixed_interval":"1m"}}}}`
	return step
}

func stackedBarStep(fiveXXWeb int) map[string]any {
	return chartStep("Responses by service", "elasticsearch", map[string]any{
		"type": "bar", "stacked": true,
		"x": map[string]any{"type": "category", "name": "service"},
		"series": []any{
			map[string]any{"name": "2xx", "points": []any{[]any{"api", 120}, []any{"web", 80}, []any{"worker", 30}}},
			map[string]any{"name": "5xx", "points": []any{[]any{"web", fiveXXWeb}, []any{"api", 2}}},
		},
	})
}

func histogramStep() map[string]any {
	return chartStep("Response time histogram", "elasticsearch", map[string]any{
		"type": "bar",
		"x":    map[string]any{"type": "value", "name": "ms"},
		"series": []any{
			map[string]any{"name": "requests", "points": []any{[]any{0, 12}, []any{50, 40}, []any{100, 22}, []any{150, 5}}},
		},
	})
}

func areaStep() map[string]any {
	minute := func(i, v int) []any { return []any{1768478400000 + i*60000, v} }
	return chartStep("Requests by region", "elasticsearch", map[string]any{
		"type": "area", "stacked": true,
		"x": map[string]any{"type": "time", "name": "time"},
		"series": []any{
			map[string]any{"name": "eu", "points": []any{minute(0, 10), minute(1, 14), minute(2, 9), minute(3, 12), minute(4, 11)}},
			map[string]any{"name": "us", "points": []any{minute(0, 20), minute(1, 18), minute(2, 25), minute(3, 22), minute(4, 19)}},
		},
	})
}

func scatterStep() map[string]any {
	return chartStep("Latency vs payload size", "elasticsearch", map[string]any{
		"type": "scatter",
		"x":    map[string]any{"type": "value", "name": "bytes"},
		"y":    map[string]any{"name": "ms", "unit": "ms"},
		"series": []any{
			map[string]any{"name": "requests", "points": []any{
				[]any{120, 8}, []any{340, 11}, []any{800, 15}, []any{1500, 22},
				[]any{2400, 31}, []any{3900, 38}, []any{6100, 60}, []any{9000, 95},
			}},
		},
	})
}

func pieStep() map[string]any {
	return chartStep("Share of 5xx by endpoint", "elasticsearch", map[string]any{
		"type": "pie", "unit": "req",
		"slices": []any{
			map[string]any{"name": "/checkout", "value": 5},
			map[string]any{"name": "/login", "value": 2},
			map[string]any{"name": "/search", "value": 1},
		},
	})
}
