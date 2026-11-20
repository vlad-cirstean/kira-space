package agentmcp

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/internal/harness/claude"
	"github.com/kirathecat/kira-studio/internal/testx"
)

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func connect(t *testing.T, cfgPath, tokenOverride string) (*mcp.ClientSession, error) {
	t.Helper()
	return connectWith(t, cfgPath, tokenOverride, nil)
}

func connectWith(t *testing.T, cfgPath, tokenOverride string, opts *mcp.ClientOptions) (*mcp.ClientSession, error) {
	t.Helper()
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		MCPServers map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	entry := cfg.MCPServers[ServerName]
	token := entry.Headers["Authorization"][len("Bearer "):]
	if tokenOverride != "" {
		token = tokenOverride
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, opts)
	return client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: entry.URL, HTTPClient: &http.Client{Transport: bearerTransport{token}}, DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
}

func TestFinishStepServer(t *testing.T) {
	var mu sync.Mutex
	var got []string
	srv := NewServer(Options{Dir: t.TempDir(), OnFinish: func(runID string, f Finish) {
		mu.Lock()
		got = append(got, runID+"|"+f.Status)
		mu.Unlock()
	}})
	t.Cleanup(func() { _ = srv.Close() })
	cfgA, releaseA, err := srv.Register(Grant{Agent: claude.New(), RunID: "run-a"})
	if err != nil {
		t.Fatal(err)
	}
	cfgB, releaseB, err := srv.Register(Grant{Agent: claude.New(), RunID: "run-b"})
	if err != nil {
		t.Fatal(err)
	}
	defer releaseB()

	if info, err := os.Stat(cfgA); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config file mode = %v, %v; want 0600", info, err)
	}
	if raw, err := os.ReadFile(cfgA); err != nil || !strings.Contains(string(raw), `"timeout":2100000`) {
		t.Fatalf("grant config = %s (%v), want the 35 minute server timeout", raw, err)
	}
	if _, err := connect(t, cfgA, "not-a-valid-token"); err == nil {
		t.Fatal("a wrong bearer token connected")
	}

	sess, err := connect(t, cfgA, "")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	call := func(s *mcp.ClientSession, status string) *mcp.CallToolResult {
		res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "finish_step", Arguments: map[string]any{"status": status, "reason": "r"}})
		if err != nil { // the schema enum refuses an unknown status before the handler runs
			return &mcp.CallToolResult{IsError: true}
		}
		return res
	}
	if res := call(sess, "bogus"); !res.IsError {
		t.Fatal("invalid status accepted")
	}
	if res := call(sess, "done"); res.IsError {
		t.Fatalf("done refused: %v", res.Content)
	}
	sessB, err := connect(t, cfgB, "")
	if err != nil {
		t.Fatal(err)
	}
	call(sessB, "failed")
	mu.Lock()
	want := []string{"run-a|done", "run-b|failed"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("finish calls = %v, want %v", got, want)
	}
	mu.Unlock()

	releaseA()
	if _, err := os.Stat(cfgA); !os.IsNotExist(err) {
		t.Fatal("config file survives release")
	}
	if _, err := connect(t, cfgB, "run-a-token-forgotten"); err == nil {
		t.Fatal("forgotten token connected")
	}
}

type fakeSpace struct {
	mu     sync.Mutex
	calls  []string
	finish SubmitFinish
}

func (f *fakeSpace) rec(s string) {
	f.mu.Lock()
	f.calls = append(f.calls, s)
	f.mu.Unlock()
}

func (f *fakeSpace) DeclareRepos(_ context.Context, task string, repos []string) (TaskInfo, error) {
	f.rec("declare|" + task + "|" + repos[0])
	return TaskInfo{Title: "t-" + task, Repos: []TaskRepo{}, Registered: []RegisteredRepo{}}, nil
}

func (f *fakeSpace) RequestBranches(ctx context.Context, call SpaceCall, specs []BranchSpec) (RequestBranchesResult, error) {
	f.rec("branch|" + call.TaskID + "|" + specs[0].Repo)
	if specs[0].Repo == "progress" {
		ReportProgress(ctx, "preparing one")
		ReportProgress(ctx, "preparing two")
	}
	if specs[0].Repo == "gone" {
		return RequestBranchesResult{}, ToolError("repo is no longer registered")
	}
	if specs[0].Repo == "block" {
		<-ctx.Done()
		return RequestBranchesResult{}, ctx.Err()
	}
	out := RequestBranchesResult{}
	for _, sp := range specs {
		out.Branches = append(out.Branches, RequestedBranch{BranchInfo: BranchInfo{Repo: sp.Repo, Branch: sp.Branch}, Outcome: BranchCreated})
	}
	return out, nil
}

func (f *fakeSpace) ChangeBranch(_ context.Context, call SpaceCall, args ChangeBranchArgs) (ChangeBranchResult, error) {
	f.rec("change|" + call.TaskID + "|" + args.From + ">" + args.To)
	return ChangeBranchResult{Repo: args.Repo, From: args.From, To: args.To, Outcome: BranchChanged}, nil
}

func (f *fakeSpace) RemoveWorktree(_ context.Context, call SpaceCall, args RemoveWorktreeArgs) (RemoveWorktreeResult, error) {
	f.rec("remove|" + call.TaskID + "|" + args.Repo + "/" + args.Branch)
	return RemoveWorktreeResult{Repo: args.Repo, Branch: args.Branch, Outcome: BranchRemoved, Next: "Removed worktree."}, nil
}

func (f *fakeSpace) AcquireLease(_ context.Context, call SpaceCall, args AcquireLeaseArgs) (AcquireLeaseResult, error) {
	f.rec("acquire|" + call.TaskID + "|" + call.SessionID + "|" + args.Queue)
	return AcquireLeaseResult{Queue: args.Queue, State: LeaseHeld}, nil
}

func (f *fakeSpace) ReleaseLease(_ context.Context, call SpaceCall, args ReleaseLeaseArgs) (ReleaseLeaseResult, error) {
	f.rec("release|" + call.TaskID + "|" + args.Queue)
	return ReleaseLeaseResult{}, nil
}

func (f *fakeSpace) RenameBranch(_ context.Context, call SpaceCall, args RenameBranchArgs) (RenameBranchResult, error) {
	f.rec("rename|" + call.TaskID + "|" + args.From + ">" + args.To)
	return RenameBranchResult{Repo: args.Repo, From: args.From, To: args.To, Outcome: BranchRenamed, Remote: RemoteKept}, nil
}

func (f *fakeSpace) CancelOperation(_ context.Context, task, opID string) (CancelOperationResult, error) {
	f.rec("cancelop|" + task + "|" + opID)
	return CancelOperationResult{OpID: opID, Cancelled: true}, nil
}

func (f *fakeSpace) SubmitForFinish(context.Context, SpaceCall, string, bool) (SubmitFinish, error) {
	return f.finish, nil
}

func (f *fakeSpace) FinishSession(_ context.Context, call SpaceCall) (string, error) {
	f.rec("finishsession|" + call.TaskID)
	return "Stage finished.", nil
}

func (f *fakeSpace) ListArtifacts(_ context.Context, task string) (TaskArtifacts, error) {
	f.rec("artifacts|" + task)
	return TaskArtifacts{Artifacts: []Artifact{{Kind: ArtifactDoc, Title: "d", Ref: "d.md", Source: "agent"}}}, nil
}

type fakeOrchestrator struct{}

func (fakeOrchestrator) SpawnTask(context.Context, SpaceCall, CreateTaskArgs) (CreateTaskResult, error) {
	return CreateTaskResult{}, nil
}

type fakeReviewer struct{}

func (fakeReviewer) Reply(string, string, string, bool) error { return nil }

type fakeSandbox struct{}

func (fakeSandbox) FetchRemote(context.Context, string, string) (RemoteResult, error) {
	return RemoteResult{}, nil
}

func (fakeSandbox) PushBranch(context.Context, string, bool) (RemoteResult, error) {
	return RemoteResult{}, nil
}

func toolNames(t *testing.T, s *mcp.ClientSession) map[string]bool {
	t.Helper()
	res, err := s.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, tool := range res.Tools {
		out[tool.Name] = true
	}
	return out
}

// stateCall calls the state tool and returns its structured reply.
func stateCall(t *testing.T, s *mcp.ClientSession, action string, args map[string]any, token ...string) (*mcp.CallToolResult, map[string]any) {
	t.Helper()
	params := &mcp.CallToolParams{Name: "state", Arguments: map[string]any{"action": action, "args": args}}
	if len(token) > 0 {
		params.SetProgressToken(token[0])
	}
	res, err := s.CallTool(context.Background(), params)
	if err != nil {
		t.Fatalf("state %s: %v", action, err)
	}
	reply, _ := res.StructuredContent.(map[string]any)
	return res, reply
}

// firstItem is the first item of a state reply.
func firstItem(t *testing.T, reply map[string]any) map[string]any {
	t.Helper()
	items, _ := reply["items"].([]any)
	if len(items) == 0 {
		t.Fatalf("reply has no items: %v", reply)
	}
	return items[0].(map[string]any)
}

// A grant exposes only its own tools and acts on its own task; a released token is dead.
func TestGrantScopes(t *testing.T) {
	space := &fakeSpace{}
	var finished []string
	srv := NewServer(Options{Dir: t.TempDir(), OnFinish: func(runID string, f Finish) { finished = append(finished, runID+"|"+f.Status) }, Space: space})
	t.Cleanup(func() { _ = srv.Close() })

	cfgRun, relRun, err := srv.Register(Grant{Agent: claude.New(), RunID: "run-1", TaskID: "task-1"})
	if err != nil {
		t.Fatal(err)
	}
	defer relRun()
	cfgSpace, relSpace, err := srv.Register(Grant{Agent: claude.New(), TaskID: "task-1", Space: true})
	if err != nil {
		t.Fatal(err)
	}
	cfgBoth, relBoth, err := srv.Register(Grant{Agent: claude.New(), RunID: "run-2", TaskID: "task-2", Space: true})
	if err != nil {
		t.Fatal(err)
	}
	defer relBoth()

	run, err := connect(t, cfgRun, "")
	if err != nil {
		t.Fatal(err)
	}
	if names := toolNames(t, run); len(names) != 1 || !names["finish_step"] {
		t.Fatalf("run-only grant tools = %v", names)
	}
	if res, err := run.CallTool(context.Background(), &mcp.CallToolParams{Name: "state", Arguments: map[string]any{"action": "branch.request"}}); err == nil && !res.IsError {
		t.Fatal("run-only grant called the state tool")
	}

	sp, err := connect(t, cfgSpace, "")
	if err != nil {
		t.Fatal(err)
	}
	if names := toolNames(t, sp); len(names) != 1 || !names["state"] {
		t.Fatalf("space-only grant tools = %v", names)
	}
	if res, err := sp.CallTool(context.Background(), &mcp.CallToolParams{Name: "finish_step", Arguments: map[string]any{"status": "done"}}); err == nil && !res.IsError {
		t.Fatal("space-only grant called finish_step")
	}
	if len(finished) != 0 {
		t.Fatalf("finish_step ran for a space grant: %v", finished)
	}

	both, err := connect(t, cfgBoth, "")
	if err != nil {
		t.Fatal(err)
	}
	if names := toolNames(t, both); len(names) != 1 || !names["state"] {
		t.Fatalf("a Space run lists %v, want state only: step.finish replaces finish_step", names)
	}
	if res, reply := stateCall(t, both, "branch.request", map[string]any{"repo": "api", "branch": "feat/x"}); res.IsError || firstItem(t, reply)["status"] != "done" {
		t.Fatalf("branch.request: %v", reply)
	}
	if res, reply := stateCall(t, sp, "lease.acquire", map[string]any{"queue": "q"}); res.IsError || firstItem(t, reply)["status"] != "done" {
		t.Fatalf("lease.acquire: %v", reply)
	}
	if res, reply := stateCall(t, sp, "worktree.remove", map[string]any{"repo": "api", "branch": "feat/x"}); res.IsError || firstItem(t, reply)["status"] != "done" {
		t.Fatalf("worktree.remove: %v", reply)
	}
	if res, reply := stateCall(t, both, "branch.request", map[string]any{"repo": "gone", "branch": "feat/x"}); res.IsError || firstItem(t, reply)["status"] != "invalid" {
		t.Fatalf("an agent-fixable refusal must be an invalid item, not an error: %v", reply)
	}
	space.mu.Lock()
	got := space.calls
	space.mu.Unlock()
	want := []string{"branch|task-2|api", "acquire|task-1||q", "remove|task-1|api/feat/x", "branch|task-2|gone"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}

	raw, err := os.ReadFile(cfgSpace)
	if err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(t.TempDir(), "kept.json")
	if err := os.WriteFile(kept, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	relSpace()
	if _, err := connect(t, kept, ""); err == nil {
		t.Fatal("a released grant connected")
	}
}

// A bad state call is an invalid item with a one-line fix, never an MCP error.
func TestStateInvalidCalls(t *testing.T) {
	srv := NewServer(Options{Dir: t.TempDir(), Space: &fakeSpace{}})
	t.Cleanup(func() { _ = srv.Close() })
	cfg, rel, err := srv.Register(Grant{Agent: claude.New(), TaskID: "task-1", Space: true})
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	sess, err := connect(t, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string]map[string]any{
		"flat":          {"action": "branch.request", "repo": "app", "branch": "x"},
		"stray field":   {"action": "branch.request", "args": map[string]any{"repo": "app", "branch": "x", "color": "red"}},
		"missing field": {"action": "branch.request", "args": map[string]any{"repo": "app"}},
		"args string":   {"action": "branch.request", "args": `{"repo": "app", "branch": "x"}`},
		"wrong type":    {"action": "branch.request", "args": map[string]any{"repo": "app", "branch": 7}},
		"remove blank":  {"action": "worktree.remove", "args": map[string]any{"repo": "", "branch": "x"}},
	} {
		res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "state", Arguments: raw})
		if err != nil || res.IsError {
			t.Fatalf("%s: %v %v, want an invalid item", name, err, res)
		}
		reply, _ := res.StructuredContent.(map[string]any)
		it := firstItem(t, reply)
		if it["status"] != "invalid" || it["fix"] == "" || it["fix"] == nil {
			t.Fatalf("%s: item = %v, want invalid with a fix", name, it)
		}
	}
}

// Every grant's tools/list stays inside its token budget (bytes/4 of the tools/list JSON): the
// schema is paid on each turn of every run.
func TestToolSchemaBudget(t *testing.T) {
	srv := NewServer(Options{Dir: t.TempDir(), Space: &fakeSpace{}, Orchestrator: fakeOrchestrator{}, Review: fakeReviewer{}, SandboxGit: fakeSandbox{}})
	t.Cleanup(func() { _ = srv.Close() })
	review := &ReviewGrant{}
	for _, c := range []struct {
		name   string
		grant  Grant
		budget int
	}{
		{"run", Grant{RunID: "r", TaskID: "t", Space: true}, 900},
		{"session", Grant{TaskID: "t", SessionID: "s", Space: true}, 900},
		{"orchestrator", Grant{RunID: "r", TaskID: "t", Space: true, Orchestrator: true}, 1230},
		{"review", Grant{RunID: "r", TaskID: "t", Review: review}, 940},
		{"push", Grant{RunID: "r", TaskID: "t", Space: true, Sandbox: &SandboxGrant{Push: true}}, 960},
	} {
		g := c.grant
		g.Agent = claude.New()
		cfg, rel, err := srv.Register(g)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		sess, err := connect(t, cfg, "")
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		res, err := sess.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		raw, _ := json.Marshal(res.Tools)
		tokens := len(raw) / 4
		t.Logf("%s: %d tokens (budget %d)", c.name, tokens, c.budget)
		if tokens > c.budget {
			t.Errorf("%s: tools/list is %d tokens, budget %d", c.name, tokens, c.budget)
		}
		rel()
	}
}

func TestRegisterRefusesUselessGrants(t *testing.T) {
	srv := NewServer(Options{Dir: t.TempDir(), OnFinish: func(string, Finish) {}, Space: &fakeSpace{}})
	t.Cleanup(func() { _ = srv.Close() })
	for _, g := range []Grant{{Agent: claude.New()}, {Agent: claude.New(), Space: true}, {Agent: claude.New(), RunID: "r", Space: true}, {RunID: "r"}} {
		if _, _, err := srv.Register(g); err == nil {
			t.Errorf("Register(%+v) accepted", g)
		}
	}
	noTools := NewServer(Options{Dir: t.TempDir(), OnFinish: func(string, Finish) {}})
	t.Cleanup(func() { _ = noTools.Close() })
	if _, _, err := noTools.Register(Grant{Agent: claude.New(), TaskID: "t", Space: true}); err == nil {
		t.Error("a Space grant without tools accepted")
	}
}

// A run grant's finish_step status enum is its step's result ids plus needs_input; any other id is refused.
func TestFinishStepStatusEnum(t *testing.T) {
	var got []string
	srv := NewServer(Options{Dir: t.TempDir(), OnFinish: func(_ string, f Finish) { got = append(got, f.Status) }})
	t.Cleanup(func() { _ = srv.Close() })
	enum := func(cfg string) []any {
		s, err := connect(t, cfg, "")
		if err != nil {
			t.Fatal(err)
		}
		res, err := s.ListTools(context.Background(), nil)
		if err != nil || len(res.Tools) != 1 {
			t.Fatalf("tools: %v %v", res, err)
		}
		sch, _ := json.Marshal(res.Tools[0].InputSchema)
		var parsed struct {
			Properties struct {
				Status struct {
					Enum []any `json:"enum"`
				} `json:"status"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(sch, &parsed); err != nil {
			t.Fatal(err)
		}
		return parsed.Properties.Status.Enum
	}
	implicit, relI, err := srv.Register(Grant{Agent: claude.New(), RunID: "run-i"})
	if err != nil {
		t.Fatal(err)
	}
	defer relI()
	if e := enum(implicit); !reflect.DeepEqual(e, []any{"done", "failed", "needs_input"}) {
		t.Fatalf("implicit enum = %v", e)
	}
	cfg, rel, err := srv.Register(Grant{Agent: claude.New(), RunID: "run-r", Results: []ResultSpec{{ID: "approved", OK: true}, {ID: "changes"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	if e := enum(cfg); !reflect.DeepEqual(e, []any{"approved", "changes", "needs_input"}) {
		t.Fatalf("declared enum = %v", e)
	}
	s, err := connect(t, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	call := func(status string) bool {
		res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "finish_step", Arguments: map[string]any{"status": status, "reason": "r"}})
		return err == nil && !res.IsError
	}
	if call("done") || call("bogus") {
		t.Fatal("a status outside the run's results was accepted")
	}
	if !call("changes") || !reflect.DeepEqual(got, []string{"changes"}) {
		t.Fatalf("declared result refused or misrecorded: %v", got)
	}
}

// A blocked Space call ends when its grant is released, even while the HTTP connection stays open.
func TestReleaseCancelsBlockedCall(t *testing.T) {
	srv := NewServer(Options{Dir: t.TempDir(), OnFinish: func(string, Finish) {}, Space: &fakeSpace{}})
	t.Cleanup(func() { _ = srv.Close() })
	cfg, release, err := srv.Register(Grant{Agent: claude.New(), TaskID: "task-1", Space: true})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := connect(t, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan *mcp.CallToolResult, 1)
	go func() {
		res, _ := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "state", Arguments: map[string]any{"action": "branch.request", "args": map[string]any{"repo": "block", "branch": "feat/x"}}})
		done <- res
	}()
	time.Sleep(200 * time.Millisecond)
	release()
	select {
	case res := <-done:
		if res != nil && !res.IsError {
			t.Fatalf("released call succeeded: %+v", res)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("blocked call outlived its release")
	}
}

// OnToolCall sees every tools/call with its grant's run and task, and the finish verdict.
func TestOnToolCallReportsEveryCall(t *testing.T) {
	type seen struct {
		call  ToolCall
		isErr bool
	}
	var mu sync.Mutex
	var got []seen
	srv := NewServer(Options{Dir: t.TempDir(), OnFinish: func(string, Finish) {},
		OnToolCall: func(ctx context.Context, c ToolCall) (context.Context, func(bool, string, error)) {
			return ctx, func(isErr bool, _ string, _ error) {
				mu.Lock()
				got = append(got, seen{c, isErr})
				mu.Unlock()
			}
		}})
	t.Cleanup(func() { _ = srv.Close() })
	cfg, release, err := srv.Register(Grant{Agent: claude.New(), RunID: "run-1", TaskID: "task-1"})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	sess, err := connect(t, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "finish_step", Arguments: map[string]any{"status": "done"}}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0].call.Tool != "finish_step" || got[0].call.RunID != "run-1" || got[0].call.TaskID != "task-1" || got[0].isErr {
		t.Fatalf("calls = %+v", got)
	}
}

// A state call reports progress to a client that sent a progress token, in order, before its result.
func TestStateReportsProgress(t *testing.T) {
	srv := NewServer(Options{Dir: t.TempDir(), Space: &fakeSpace{}})
	t.Cleanup(func() { _ = srv.Close() })
	cfg, rel, err := srv.Register(Grant{Agent: claude.New(), TaskID: "task-1", Space: true})
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	var mu sync.Mutex
	got := map[float64]string{}
	cs, err := connectWith(t, cfg, "", &mcp.ClientOptions{ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
		mu.Lock()
		got[req.Params.Progress] = req.Params.Message
		mu.Unlock()
	}})
	if err != nil {
		t.Fatal(err)
	}
	if res, _ := stateCall(t, cs, "branch.request", map[string]any{"repo": "progress", "branch": "feat/x"}, "tok"); res.IsError {
		t.Fatalf("state: %v", res)
	}
	// The client runs notification handlers on their own goroutines: wait, then compare by the
	// increasing progress value the server stamped.
	testx.WaitUntil(t, 5*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) == 2
	})
	mu.Lock()
	defer mu.Unlock()
	if got[1] != "preparing one" || got[2] != "preparing two" {
		t.Fatalf("progress = %v, want 1 then 2 with the two messages", got)
	}
}

// An ok step.finish the board refuses (no draft) records nothing and is an invalid item with the
// refusal; an accepted one goes through, carrying the submit reply.
func TestStepFinishImplicitSubmit(t *testing.T) {
	var mu sync.Mutex
	var got []string
	fs := &fakeSpace{finish: SubmitFinish{Refuse: "write handovers/draft-build.md, then call state step.finish"}}
	srv := NewServer(Options{Dir: t.TempDir(), Space: fs, OnFinish: func(runID string, f Finish) {
		mu.Lock()
		got = append(got, f.Status)
		mu.Unlock()
	}})
	t.Cleanup(func() { _ = srv.Close() })
	cfg, release, err := srv.Register(Grant{Agent: claude.New(), RunID: "run-1", TaskID: "task-1", Space: true})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	sess, err := connect(t, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	res, reply := stateCall(t, sess, "step.finish", map[string]any{"status": "done"})
	if it := firstItem(t, reply); res.IsError || it["status"] != "invalid" || !strings.Contains(it["message"].(string), "draft-build.md") {
		t.Fatalf("ok finish without a draft = %v; want the refusal", reply)
	}
	fs.finish = SubmitFinish{Reply: "accepted 1, pending user 0, invalid 0"}
	res, reply = stateCall(t, sess, "step.finish", map[string]any{"status": "done"})
	if it := firstItem(t, reply); res.IsError || it["status"] != "done" || !strings.Contains(it["message"].(string), "accepted 1, pending user 0, invalid 0") {
		t.Fatalf("ok finish with a draft = %v; want done with the reply", reply)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0] != "done" {
		t.Fatalf("finishes = %v, want one done", got)
	}
}

// A proof is a folder the watcher reads: no grant lists a proof tool.
func TestNoProofTool(t *testing.T) {
	srv := NewServer(Options{Dir: t.TempDir(), Space: &fakeSpace{}})
	t.Cleanup(func() { _ = srv.Close() })
	for _, g := range []Grant{
		{Agent: claude.New(), TaskID: "task-1", Space: true},
		{Agent: claude.New(), RunID: "run-1", TaskID: "task-1", Space: true},
	} {
		cfg, rel, err := srv.Register(g)
		if err != nil {
			t.Fatal(err)
		}
		defer rel()
		s, err := connect(t, cfg, "")
		if err != nil {
			t.Fatal(err)
		}
		for name := range toolNames(t, s) {
			if strings.Contains(name, "proof") || name == "create" {
				t.Errorf("grant %+v lists %q", g, name)
			}
		}
		_ = s.Close()
	}
}
