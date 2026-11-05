package agentmcp

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/internal/harness/claude"
)

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func connect(t *testing.T, cfgPath, tokenOverride string) (*mcp.ClientSession, error) {
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
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	return client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: entry.URL, HTTPClient: &http.Client{Transport: bearerTransport{token}}, DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
}

func TestFinishStepServer(t *testing.T) {
	var mu sync.Mutex
	var got []string
	srv := NewServer(Options{Dir: t.TempDir(), OnFinish: func(runID string, f Finish) {
		mu.Lock()
		got = append(got, runID+"|"+f.Status+"|"+f.Summary)
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
	if _, err := connect(t, cfgA, "not-a-valid-token"); err == nil {
		t.Fatal("a wrong bearer token connected")
	}

	sess, err := connect(t, cfgA, "")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	call := func(s *mcp.ClientSession, status string) *mcp.CallToolResult {
		res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "finish_step", Arguments: map[string]any{"status": status, "summary": "s-" + status, "reason": "r"}})
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
	want := []string{"run-a|done|s-done", "run-b|failed|s-failed"}
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
	mu    sync.Mutex
	calls []string
}

func (f *fakeSpace) rec(s string) {
	f.mu.Lock()
	f.calls = append(f.calls, s)
	f.mu.Unlock()
}

func (f *fakeSpace) TaskInfo(_ context.Context, task string) (TaskInfo, error) {
	f.rec("info|" + task)
	return TaskInfo{Title: "t-" + task, Repos: []TaskRepo{}, Registered: []RegisteredRepo{}}, nil
}

func (f *fakeSpace) DeclareRepos(_ context.Context, task string, repos []string) (TaskInfo, error) {
	f.rec("declare|" + task + "|" + repos[0])
	return TaskInfo{Title: "t-" + task, Repos: []TaskRepo{}, Registered: []RegisteredRepo{}}, nil
}

func (f *fakeSpace) RequestBranches(ctx context.Context, call SpaceCall, specs []BranchSpec) (RequestBranchesResult, error) {
	f.rec("branch|" + call.TaskID + "|" + specs[0].Repo)
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

func (f *fakeSpace) BranchStatus(_ context.Context, task, repo, _ string, _ time.Duration) (BranchInfo, error) {
	f.rec("status|" + task + "|" + repo)
	return BranchInfo{Repo: repo}, nil
}

func (f *fakeSpace) RenameBranch(_ context.Context, call SpaceCall, args RenameBranchArgs) (RenameBranchResult, error) {
	f.rec("rename|" + call.TaskID + "|" + args.From + ">" + args.To)
	return RenameBranchResult{Repo: args.Repo, From: args.From, To: args.To, Outcome: BranchRenamed, Remote: RemoteKept}, nil
}

func (f *fakeSpace) ReportArtifacts(_ context.Context, call SpaceCall, in []ArtifactInput) (ReportArtifactsResult, error) {
	f.rec("report|" + call.TaskID)
	return ReportArtifactsResult{Recorded: len(in)}, nil
}

func (f *fakeSpace) ListArtifacts(_ context.Context, task string) (TaskArtifacts, error) {
	f.rec("artifacts|" + task)
	return TaskArtifacts{Artifacts: []Artifact{{Kind: ArtifactDoc, Title: "d", Ref: "d.md", Source: "agent"}}}, nil
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
	if names := toolNames(t, run); !names["finish_step"] || names["request_branches"] || names["task_info"] {
		t.Fatalf("run-only grant tools = %v", names)
	}
	if res, err := run.CallTool(context.Background(), &mcp.CallToolParams{Name: "request_branches", Arguments: map[string]any{"branches": []map[string]string{{"repo": "r", "branch": "feat/x"}}}}); err == nil && !res.IsError {
		t.Fatal("run-only grant called a Space tool")
	}

	sp, err := connect(t, cfgSpace, "")
	if err != nil {
		t.Fatal(err)
	}
	if names := toolNames(t, sp); names["finish_step"] || !names["request_branches"] || !names["declare_repos"] || !names["task_info"] || !names["branch_status"] || !names["rename_branch"] || !names["report_artifacts"] {
		t.Fatalf("space-only grant tools = %v", names)
	}
	if res, err := sp.CallTool(context.Background(), &mcp.CallToolParams{Name: "finish_step", Arguments: map[string]any{"status": "done", "summary": "x"}}); err == nil && !res.IsError {
		t.Fatal("space-only grant called finish_step")
	}
	if len(finished) != 0 {
		t.Fatalf("finish_step ran for a space grant: %v", finished)
	}

	both, err := connect(t, cfgBoth, "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := both.CallTool(context.Background(), &mcp.CallToolParams{Name: "request_branches", Arguments: map[string]any{"branches": []map[string]string{{"repo": "api", "branch": "feat/x"}}}})
	if err != nil || res.IsError {
		t.Fatalf("request_branches: %v %v", err, res)
	}
	if _, err := sp.CallTool(context.Background(), &mcp.CallToolParams{Name: "task_info"}); err != nil {
		t.Fatal(err)
	}
	res, err = both.CallTool(context.Background(), &mcp.CallToolParams{Name: "request_branches", Arguments: map[string]any{"branches": []map[string]string{{"repo": "gone", "branch": "feat/x"}}}})
	if err != nil || !res.IsError {
		t.Fatalf("a ToolError must come back as an error result: %v %v", err, res)
	}
	space.mu.Lock()
	got := space.calls
	space.mu.Unlock()
	want := []string{"branch|task-2|api", "info|task-1", "branch|task-2|gone"}
	if len(got) != len(want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("calls = %v, want %v", got, want)
		}
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
		res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "finish_step", Arguments: map[string]any{"status": status, "summary": "x", "reason": "r"}})
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
		res, _ := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "request_branches", Arguments: map[string]any{"branches": []map[string]string{{"repo": "block", "branch": "feat/x"}}}})
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
	if _, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "finish_step", Arguments: map[string]any{"status": "done", "summary": "s"}}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0].call.Tool != "finish_step" || got[0].call.RunID != "run-1" || got[0].call.TaskID != "task-1" || got[0].isErr {
		t.Fatalf("calls = %+v", got)
	}
}
