package adeflow_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	_ "modernc.org/sqlite"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/mcpauth"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P548 Kira Studio tools in workflow steps. Contract ade-db-mcp-step: tests/ui/ade-v2-workflow-db-mcp.spec.ts reads it.

const (
	dbScenario  = "ade-db-mcp-step"
	dbSrvName   = "kira-studio-session"
	dbAllowRule = "mcp__kira-studio-session"
)

var dbURLRe = regexp.MustCompile(`"url":"([^"]+)"`)

type sessionEntry struct {
	URL     string            `json:"url"`
	Type    string            `json:"type"`
	Headers map[string]string `json:"headers"`
}

// mcpEntry reads one server of a --mcp-config file.
func mcpEntry(t *testing.T, raw, name string) (sessionEntry, bool) {
	t.Helper()
	var doc struct {
		Servers map[string]sessionEntry `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("mcp config %q: %v", raw, err)
	}
	e, ok := doc.Servers[name]
	return e, ok
}

func mcpServerNames(t *testing.T, raw string) []string {
	t.Helper()
	var doc struct {
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	var out []string
	for n := range doc.Servers {
		out = append(out, n)
	}
	return out
}

// postStatus POSTs an MCP initialize to url with the bearer token and returns the HTTP status.
func postStatus(t *testing.T, url, token string) int {
	t.Helper()
	got, err := tryPost(url, token)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return got
}

// tryPost is postStatus returning the transport error instead of failing.
func tryPost(url, token string) (int, error) {
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"flow","version":"1"}}}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	_ = res.Body.Close()
	return res.StatusCode, nil
}

type tokenTransport struct{ token string }

func (b tokenTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func sessionClient(t *testing.T, e sessionEntry) *mcp.ClientSession {
	t.Helper()
	token := strings.TrimPrefix(e.Headers["Authorization"], "Bearer ")
	client := mcp.NewClient(&mcp.Implementation{Name: "flow-session", Version: "1"}, nil)
	sess, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: e.URL, HTTPClient: &http.Client{Transport: tokenTransport{token}, Timeout: 30 * time.Second},
	}, nil)
	if err != nil {
		t.Fatalf("connect %s: %v", e.URL, err)
	}
	return sess
}

func TestDbMcpStep(t *testing.T) {
	hold := filepath.Join(t.TempDir(), "hold")
	app := flowharness.New(t, flowharness.WithTrackerGrace(time.Second))
	dbPath := filepath.Join(t.TempDir(), "step.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{"create table notes (id integer primary key, body text)", "insert into notes (body) values ('one')"} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	dbPtr := dbPath
	conn, err := app.W.Connections.Create(connections.Input{ConnectionFields: model.ConnectionFields{
		Name: "step db", Kind: "sqlite", Color: "teal", Mode: "fields", Database: &dbPtr, Options: map[string]any{},
		McpEnabled: true, McpDescription: "scratch", McpReadMode: "allow", McpWriteMode: "prompt", McpDdlMode: "deny",
	}})
	if err != nil {
		t.Fatal(err)
	}
	listConnections := fakeagent.MCPCall{Server: dbSrvName, Tool: "list_connections"}
	proofSave := fakeagent.MCPCall{Server: dbSrvName, Tool: "proof_save", Args: map[string]any{"name": "Db step proof", "steps": []any{map[string]any{
		"description": "Notes", "view": "keyvalue", "data": map[string]any{"entries": []any{map[string]any{"field": "rows", "value": 1}}},
	}}}}
	claude(app, map[string][]fakeagent.Action{"*": {
		{MCP: []fakeagent.MCPCall{listConnections, {Server: dbSrvName, Tool: "proof_list"}, proofSave}, Name: "done"},
		{MCP: []fakeagent.MCPCall{listConnections}, Name: "done"},
		{WaitFile: hold},
	}})
	t.Cleanup(func() { _ = os.WriteFile(hold, nil, 0o644) })
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)

	// 1. The step option saves, rereads and reaches the YAML; a script step refuses it.
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("withdb", "    allowed_tools: []\n")+agentStep("plain", "")))
	entry, _ := workflowEntry(t, app, "flow.yaml")
	wf := *entry.Workflow
	wf.Steps = append([]adewire.WorkflowStep(nil), wf.Steps...)
	wf.Steps[stepIndex(t, wf, "withdb")].DbMcp = true
	saved, err := app.W.AdeTask.SaveWorkflow(ctx, adewire.SaveWorkflowArgs{FileName: "flow.yaml", Workflow: wf, BaseHash: entry.Hash})
	if err != nil || saved.Error != nil {
		t.Fatalf("SaveWorkflow = %+v, %v", saved, err)
	}
	entry, list := workflowEntry(t, app, "flow.yaml")
	if !entry.Workflow.Steps[stepIndex(t, *entry.Workflow, "withdb")].DbMcp || entry.Workflow.Steps[stepIndex(t, *entry.Workflow, "plain")].DbMcp {
		t.Fatalf("steps = %+v, want db_mcp on withdb only", entry.Workflow.Steps)
	}
	raw, err := os.ReadFile(entry.Path)
	if err != nil || strings.Count(string(raw), "db_mcp: true") != 1 {
		t.Fatalf("workflow file (%v):\n%s\nwant one db_mcp: true", err, raw)
	}
	app.Contract(t, dbScenario, "AdeTaskService.Workflows", list)
	refused, err := app.W.AdeTask.ValidateWorkflowYaml(ctx, adewire.ValidateWorkflowYamlArgs{Yaml: flowYAML("bad", "  - id: s\n    name: s\n    kind: script\n    runs_on: each repo\n    timeout: 1m\n    command: echo\n    db_mcp: true\n")})
	if err != nil || refused.Workflow != nil || refused.Error == nil || !strings.Contains(refused.Error.Message, "db_mcp is not allowed on script steps") {
		t.Fatalf("script db_mcp = %+v / %v, want a refusal", refused.Workflow, refused.Error)
	}

	// 2. The Database MCP switch is off; the step still gets the tools, and the one without db_mcp does not.
	if app.W.DbMcp.Status().Running {
		t.Fatal("Database MCP server runs without the switch")
	}
	task := createTask(t, app, "Fix login", "flow", rec.ID)
	br := branchOf(t, board(t, app), task.ID, rec.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{br.ID: "feat/api-db"}}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, app, task.ID, "withdb", "done")
	if app.W.DbMcp.Status().Running {
		t.Fatal("a session grant started the external server")
	}
	if got := fakeFile(t, app, "mcp-list_connections-1.json"); !strings.Contains(got, "step db") {
		t.Fatalf("list_connections result = %s, want the connection", got)
	}
	fakeFile(t, app, "mcp-proof_list-1.json")
	// proof_save on the session server attaches the new proof to the step's task.
	if got := taskProofs(t, app, task.ID); len(got) != 1 || got[0].Name != "Db step proof" || got[0].Source != "agent" || got[0].StepID != "withdb" {
		t.Fatalf("task proofs = %+v, want the saved proof attached by the agent", got)
	}
	args1 := fakeFile(t, app, "api-1.args")
	mustContain(t, "withdb argv", args1, dbAllowRule)
	if !regexp.MustCompile(`(?m)^mcp__kira-studio$`).MatchString(args1) {
		t.Fatalf("withdb argv = %q, want the user-scope kira-studio denied beside %s", args1, dbSrvName)
	}
	if names := mcpServerNames(t, fakeFile(t, app, "api-1.mcp0")); len(names) != 2 {
		t.Fatalf("withdb servers = %v, want kira-space and %s", names, dbSrvName)
	}

	// 3. A step without db_mcp has only kira-space; calling the session server fails.
	waitRun(t, app, task.ID, "plain", "failed")
	mustLack(t, "plain argv", fakeFile(t, app, "api-2.args"), dbAllowRule)
	mustLack(t, "plain argv", fakeFile(t, app, "api-2.args"), "mcp__kira-studio\n")
	if names := mcpServerNames(t, fakeFile(t, app, "api-2.mcp0")); len(names) != 1 || names[0] != "kira-space" {
		t.Fatalf("plain servers = %v, want only kira-space", names)
	}
	mustContain(t, "plain run log", logText(t, app, "run", latestRunID(t, app, task.ID, "plain")), "mcp:")

	// 4. A manual session step: the token lives in the launch config, works only on the session server.
	saveWorkflow(t, app, "tui", flowYAML("tui", "  - id: work\n    name: Work\n    kind: manual\n    session: true\n    db_mcp: true\n"))
	tuiTask := createTask(t, app, "Session task", "tui", rec.ID)
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})
	launch, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: tuiTask.ID})
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := mcpConfigPath(t, launch.Command)
	cfgRaw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	e, ok := mcpEntry(t, string(cfgRaw), dbSrvName)
	if !ok || e.Type != "http" || e.Headers["Authorization"] == "" {
		t.Fatalf("launch config %s has no usable %s entry", cfgPath, dbSrvName)
	}
	token := strings.TrimPrefix(e.Headers["Authorization"], "Bearer ")
	if strings.Contains(launch.Command, token) || strings.Contains(logText(t, app, "run", latestRunID(t, app, task.ID, "withdb")), token) {
		t.Fatal("session token leaked into the launch command or a log")
	}
	st, err := app.W.DbMcp.SetEnabled(bridge.DbMcpSetEnabledArgs{Enabled: true})
	if err != nil || !st.Running {
		t.Fatalf("SetEnabled = %+v, %v", st, err)
	}
	m := dbURLRe.FindStringSubmatch(st.Command)
	if m == nil || m[1] == e.URL {
		t.Fatalf("external url %v vs session url %s, want two different servers", m, e.URL)
	}
	helper, ok, err := mcpauth.LoadHelperToken(mcpauth.HelperTokenPathNamed(app.SpaceHome, "mcp-db"))
	if err != nil || !ok {
		t.Fatalf("helper token: %v %v", ok, err)
	}
	if got := postStatus(t, e.URL, token); got != http.StatusOK {
		t.Fatalf("session token on the session server = %d, want 200", got)
	}
	if got := postStatus(t, e.URL, helper); got != http.StatusUnauthorized {
		t.Fatalf("external token on the session server = %d, want 401", got)
	}
	if got := postStatus(t, m[1], token); got != http.StatusUnauthorized {
		t.Fatalf("session token on the external server = %d, want 401", got)
	}
	if got := postStatus(t, m[1], helper); got != http.StatusOK {
		t.Fatalf("external token on the external server = %d, want 200", got)
	}

	// 5. A write parks for approval; turning the external server off leaves it, the session's end abandons it.
	openLaunch(t, app, launch)
	waitSession(t, app, launch.SessionID, "running")
	sess := sessionClient(t, e)
	defer sess.Close()
	call := make(chan *mcp.CallToolResult, 1)
	go func() {
		res, _ := sess.CallTool(ctx, &mcp.CallToolParams{Name: "run_query", Arguments: map[string]any{"connectionId": conn.ID, "sql": "insert into notes (body) values ('x')", "path": "", "mode": "write"}})
		call <- res
	}()
	testx.WaitUntil(t, waitFor, func() bool {
		items, _ := app.W.ApprovalsSvc.List()
		return len(items) == 1
	})
	if _, err := app.W.DbMcp.SetEnabled(bridge.DbMcpSetEnabledArgs{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if items, _ := app.W.ApprovalsSvc.List(); len(items) != 1 {
		t.Fatalf("queue = %d items after the switch went off, want the session's request kept", len(items))
	}
	if err := os.WriteFile(hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitSession(t, app, launch.SessionID, "stopped")
	select {
	case res := <-call:
		if res != nil && !res.IsError {
			t.Fatalf("parked write returned %+v after the session ended", res)
		}
	case <-time.After(waitFor):
		t.Fatal("parked write still waiting after the session ended")
	}
	if items, _ := app.W.ApprovalsSvc.List(); len(items) != 0 {
		t.Fatalf("queue = %d items after release, want none", len(items))
	}
	// The last release closes the server; a still-listening one must refuse the token.
	if got, err := tryPost(e.URL, token); err == nil && got != http.StatusUnauthorized {
		t.Fatalf("released session token = %d, want 401 or a closed server", got)
	}
}

var cfgFlag = regexp.MustCompile(`'?--mcp-config'?\s+'?([^'\s]+)'?`)

// mcpConfigPath is the --mcp-config file a launch command names.
func mcpConfigPath(t *testing.T, command string) string {
	t.Helper()
	m := cfgFlag.FindStringSubmatch(command)
	if m == nil {
		t.Fatalf("no --mcp-config in %q", command)
	}
	return m[1]
}

func latestRunID(t *testing.T, app *flowharness.App, taskID, stepID string) string {
	t.Helper()
	r, ok := latestRun(t, app, taskID, stepID)
	if !ok {
		t.Fatalf("no run of %s", stepID)
	}
	return r.ID
}
