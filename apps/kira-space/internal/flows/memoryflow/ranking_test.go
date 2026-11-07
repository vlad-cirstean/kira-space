package memoryflow_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/harness/claude"
	"github.com/kirathecat/kira-studio/internal/memory"
	"github.com/kirathecat/kira-studio/internal/memory/memfmt"
)

const webKey = "github.com/acme/web"

// acceptRunner accepts every item as one fact, keeping its text.
type acceptRunner struct{}

func (acceptRunner) Fail(kind error) error { return claude.New().Fail(kind) }

func (acceptRunner) Run(_ context.Context, c memory.Call) (json.RawMessage, error) {
	var in struct {
		Items []struct {
			Index        int
			Fact, Reason string
		}
	}
	if err := json.Unmarshal([]byte(c.Input), &in); err != nil {
		return nil, err
	}
	items := []map[string]any{}
	for _, it := range in.Items {
		items = append(items, map[string]any{"index": it.Index, "verdict": "accept", "questions": []string{},
			"facts": []map[string]any{{"fact": it.Fact, "reason": it.Reason, "keywords": []string{"invoice"}}}})
	}
	return json.Marshal(map[string]any{"items": items})
}

// seedRanked stores five facts on one topic through the real service with a fake runner, so each
// carries its own origin, scope and project.
func seedRanked(t *testing.T, app *flowharness.App) {
	t.Helper()
	st := memory.NewStore(filepath.Join(app.MemoryHome, "memory.db"))
	t.Cleanup(func() { _ = st.Close() })
	svc := memory.NewService(st, acceptRunner{}, memory.ServiceOptions{})
	t.Cleanup(svc.Close)
	res, err := svc.Store(ctx, memory.StoreRequest{Source: memory.SourceUI, Items: []memory.Item{
		{Fact: "Invoice retries use exponential backoff", Reason: "billing design", Origin: memory.OriginUser, Scope: memory.ScopeProject, Project: billingKey},
		{Fact: "Invoice retries are capped at five attempts", Reason: "billing code", Origin: memory.OriginAgent, Scope: memory.ScopeProject, Project: billingKey},
		{Fact: "Invoice retries show a banner at checkout", Reason: "web design", Origin: memory.OriginUser, Scope: memory.ScopeProject, Project: webKey},
		{Fact: "Invoice retries alert finance after failures", Reason: "ops runbook", Origin: memory.OriginExternal, SourceTag: "runbook", Scope: memory.ScopeCompany},
		{Fact: "Invoice retries are my weekly reminder", Reason: "personal note", Origin: memory.OriginUser, Scope: memory.ScopePersonal},
	}})
	if err != nil || res.Status != "stored" || len(res.Outcomes) != 5 {
		t.Fatalf("seed = %+v, %v, want five stored facts", res, err)
	}
}

func searchFacts(t *testing.T, hits []memory.Memory) []string {
	t.Helper()
	out := make([]string, len(hits))
	for i, m := range hits {
		out[i] = m.Fact
	}
	return out
}

func TestSearchRankedWithBoost(t *testing.T) {
	app := flowharness.New(t)
	seedRanked(t, app)
	args := bridge.MemorySearchArgs{Query: "invoice retries", Scopes: []string{memory.ScopeCompany, memory.ScopeProject}, Origins: []string{}, Project: billingKey}
	hits, err := app.W.Memory.Search(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Invoice retries use exponential backoff",
		"Invoice retries are capped at five attempts",
		"Invoice retries alert finance after failures",
		"Invoice retries show a banner at checkout",
	}
	if got := searchFacts(t, hits); !slices.Equal(got, want) {
		t.Fatalf("Search order = %q, want billing user, billing agent, company, web (the personal fact filtered out)", got)
	}
	for _, m := range hits {
		if m.Score == nil || m.Score.Total <= 0 || m.Score.Keyword <= 0 || m.Score.Boost == "" {
			t.Fatalf("score of %q = %+v, want total, keyword and a scope boost", m.Fact, m.Score)
		}
	}
	if b := hits[3].Score.Boost; b != "other-project" {
		t.Fatalf("web fact boost = %q, want other-project", b)
	}
	app.Contract(t, "memory", "args:MemoryService.Search#ranked", args)
	app.Contract(t, "memory", "MemoryService.Search#ranked", hits, flowharness.Mask("createdAt"))
}

func TestSearchOriginFilter(t *testing.T) {
	app := flowharness.New(t)
	seedRanked(t, app)
	args := bridge.MemorySearchArgs{Query: "invoice retries", Scopes: []string{}, Origins: []string{memory.OriginUser}, Project: billingKey}
	hits, err := app.W.Memory.Search(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 {
		t.Fatalf("Search = %q, want the three user facts", searchFacts(t, hits))
	}
	for _, m := range hits {
		if m.Origin != memory.OriginUser {
			t.Fatalf("hit %q has origin %q, want only user", m.Fact, m.Origin)
		}
	}
	app.Contract(t, "memory", "args:MemoryService.Search#user", args)
	app.Contract(t, "memory", "MemoryService.Search#user", hits, flowharness.Mask("createdAt"))
}

func TestMcpSearchScoreMeta(t *testing.T) {
	app := flowharness.New(t)
	gate(t, app, "gate-accept.json")
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "https://" + billingKey + ".git"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	cmd := exec.Command(app.W.Memory.McpStatus().Executable, "memory-mcp")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "CLAUDE_CODE_SESSION_ID="+flowSession)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "flow", Version: "1"}, nil).Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	out, _ := callStore(t, cs, map[string]any{"items": []map[string]string{
		{"fact": "billing runs postgres", "reason": "migration notes", "origin": "agent", "scope": "project"},
		{"fact": "staging deploys at 09:00", "reason": "release calendar", "origin": "agent", "scope": "company"},
	}})
	if out.Status != "stored" {
		t.Fatalf("store_memory = %+v, want stored", out)
	}
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "search_memories", Arguments: map[string]any{"query": "postgresql staging"}})
	if err != nil || res.IsError {
		t.Fatalf("search_memories: %v %+v", err, res)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	line := regexp.MustCompile(`^(` + memfmt.FactID + ` \S|  \S)`)
	var metas []string
	for _, l := range strings.Split(text, "\n") {
		if l == "" || strings.HasPrefix(l, "Note: ") {
			continue
		}
		if !line.MatchString(l) {
			t.Fatalf("line %q is not '<km_ id> <fact>' or an indented line", l)
		}
		if metaLine.MatchString(l) {
			metas = append(metas, l)
		}
	}
	if len(metas) != 2 {
		t.Fatalf("search text holds %d meta lines, want 2:\n%s", len(metas), text)
	}
	if first := metas[0]; !strings.Contains(first, "score=") || !strings.Contains(first, "kw=") || !strings.Contains(first, "scope=project:1.15") {
		t.Fatalf("first meta = %q, want the project fact with score, kw and scope=project:1.15", first)
	}
	var body struct{ Memories []memory.Memory }
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Memories) != 2 || body.Memories[0].Score == nil || body.Memories[0].Score.Boost != "project" {
		t.Fatalf("structured memories = %+v, want the project fact first with boost project", body.Memories)
	}
}
