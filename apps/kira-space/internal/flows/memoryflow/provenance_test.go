package memoryflow_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/memory"
)

const (
	billingKey  = "github.com/acme/billing"
	flowSession = "flow-session"
)

func factID(id string) bool { return memory.FactIDPattern.MatchString(id) && len(id) == 15 }

func TestStoreScopedFromModule(t *testing.T) {
	app := flowharness.New(t)
	gate(t, app, "gate-accept.json")
	args := bridge.MemoryStoreArgs{Items: twoItems, Scope: memory.ScopeProject, Project: billingKey}
	res, err := app.W.Memory.Store(ctx, args)
	if err != nil || res.Status != "stored" {
		t.Fatalf("Store = %+v, %v, want stored", res, err)
	}
	app.Contract(t, "memory", "args:MemoryService.Store#scoped", args)
	recent, err := app.W.Memory.Recent(ctx)
	if err != nil || len(recent) != 2 {
		t.Fatalf("Recent = %d memories, %v, want 2", len(recent), err)
	}
	app.Contract(t, "memory", "MemoryService.Recent#scoped", recent, flowharness.Mask("createdAt"))
	for _, m := range recent {
		if m.Origin != memory.OriginUser || m.Scope != memory.ScopeProject || m.Project == nil || *m.Project != billingKey ||
			m.SessionID != nil || !factID(m.LineageID) || m.ID != m.LineageID+".1" {
			t.Fatalf("stored memory = %+v, want a user fact in project %s with no session and a km_ id", m, billingKey)
		}
	}
	projects, err := app.W.Memory.Projects(ctx)
	if err != nil || !slices.Equal(projects, []string{billingKey}) {
		t.Fatalf("Projects = %v, %v, want [%s]", projects, err, billingKey)
	}
	app.Contract(t, "memory", "MemoryService.Projects#one", projects)
}

func TestContextOfStoredFact(t *testing.T) {
	app := flowharness.New(t)
	gate(t, app, "gate-accept.json")
	if _, err := app.W.Memory.Store(ctx, twoItemsArgs()); err != nil {
		t.Fatal(err)
	}
	hits, err := app.W.Memory.Search(ctx, bridge.MemorySearchArgs{Query: "postgresql"})
	if err != nil || len(hits) != 1 {
		t.Fatalf("Search postgresql = %+v, %v, want the billing fact", hits, err)
	}
	c, err := app.W.Memory.Context(ctx, bridge.MemoryContextArgs{ID: hits[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if c.Memory.ID != hits[0].ID || len(c.Groups) != 1 || c.Groups[0].Kind != memory.GroupRequest ||
		c.Groups[0].Total != 1 || len(c.Groups[0].Facts) != 1 || !strings.Contains(c.Groups[0].Facts[0].Fact, "staging") {
		t.Fatalf("Context = %+v, want one request group holding the staging fact", c)
	}
	app.Contract(t, "memory", "MemoryService.Context#request", c, flowharness.Mask("createdAt"))
}

// runMemoryMCP plays a fake claude that calls store_memory on a real memory-mcp subprocess, started
// with the session env a real claude exports.
func runMemoryMCP(t *testing.T, app *flowharness.App, args map[string]any, prompts ...fakeagent.PromptRule) {
	t.Helper()
	app.Scenario(fakeagent.Scenario{
		Prompts: prompts,
		Claude: map[string][]fakeagent.Action{"*": {{
			MCP: []fakeagent.MCPCall{{Server: "kira-memory", Tool: "store_memory", Args: args}},
		}}},
	})
	cfg, err := json.Marshal(map[string]any{"mcpServers": map[string]any{"kira-memory": map[string]any{
		"command": app.W.Memory.McpStatus().Executable,
		"args":    []string{"memory-mcp"},
		"env":     map[string]string{"KIRA_SPACE_HOME": app.SpaceHome},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(cfgPath, cfg, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(app.BinDir, "claude"), "-p", "--mcp-config", cfgPath)
	cmd.Dir = app.Work
	cmd.Stdin = strings.NewReader("store")
	cmd.Env = append(os.Environ(), "CLAUDE_CODE_SESSION_ID="+flowSession)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fake claude: %v\n%s", err, out)
	}
}

func rule(t *testing.T, system, file string) fakeagent.PromptRule {
	t.Helper()
	return fakeagent.PromptRule{System: system, Emit: testdata(t, file)}
}

func TestMcpStoreProvenance(t *testing.T) {
	app := flowharness.New(t)
	runMemoryMCP(t, app, map[string]any{
		"items": []map[string]string{
			{"fact": "billing runs postgres", "reason": "thread", "origin": "external", "sourceTag": "slack", "scope": "company"},
			{"fact": "staging deploys at 09:00", "reason": "thread", "origin": "agent", "scope": "company"},
		},
		"groups": []map[string]string{{"kind": "thread", "key": "C123/1700.1", "title": "Billing thread"}},
	}, rule(t, gateSystem, "gate-accept.json"), rule(t, reconcileSystem, "reconcile.json"))

	recent, err := app.W.Memory.Recent(ctx)
	if err != nil || len(recent) != 2 {
		t.Fatalf("Recent = %d memories, %v, want 2", len(recent), err)
	}
	app.Contract(t, "memory", "MemoryService.Recent#mcp", recent, flowharness.Mask("createdAt"))
	var billing memory.Memory
	for _, m := range recent {
		if m.SessionID == nil || *m.SessionID != flowSession || m.Scope != memory.ScopeCompany || m.Project != nil {
			t.Fatalf("memory = %+v, want company scope, no project and session %q", m, flowSession)
		}
		if strings.Contains(m.Fact, "PostgreSQL") {
			billing = m
		}
	}
	if billing.Origin != memory.OriginExternal || billing.SourceTag == nil || *billing.SourceTag != "slack" {
		t.Fatalf("billing fact = %+v, want external from slack", billing)
	}
	c, err := app.W.Memory.Context(ctx, bridge.MemoryContextArgs{ID: billing.LineageID})
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, g := range c.Groups {
		kinds = append(kinds, g.Kind)
	}
	slices.Sort(kinds)
	if !slices.Equal(kinds, []string{memory.GroupRequest, memory.GroupSession, memory.GroupThread}) {
		t.Fatalf("group kinds = %v, want request, session and thread", kinds)
	}
	app.Contract(t, "memory", "MemoryService.Context#mcp", c, flowharness.Mask("createdAt"))
}

func TestAgentCannotSupersedeUser(t *testing.T) {
	app := flowharness.New(t)
	gate(t, app, "gate-accept.json")
	if _, err := app.W.Memory.Store(ctx, twoItemsArgs()); err != nil {
		t.Fatal(err)
	}
	runMemoryMCP(t, app, map[string]any{
		"items": []map[string]string{
			{"fact": "billing runs postgres 17", "reason": "deploy config", "origin": "agent", "scope": "personal"},
		},
	}, rule(t, gateSystem, "gate-agent-pg17.json"), rule(t, reconcileSystem, "reconcile-update-c1.json"))

	hits, err := app.W.Memory.Search(ctx, bridge.MemorySearchArgs{Query: "postgresql", IncludeHistory: true})
	if err != nil || len(hits) != 2 {
		t.Fatalf("Search postgresql = %+v, %v, want the user fact and the agent fact side by side", hits, err)
	}
	for _, m := range hits {
		if m.Historical || m.Version != 1 {
			t.Fatalf("memory = %+v, want every fact current at version 1: the agent add must not supersede", m)
		}
	}
	if !slices.ContainsFunc(hits, func(m memory.Memory) bool {
		return m.Origin == memory.OriginUser && m.Fact == "The billing service uses PostgreSQL 16."
	}) {
		t.Fatalf("hits = %+v, want the user fact untouched", hits)
	}
}
