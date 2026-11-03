package ade

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitcred"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/renameio/v2"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adeflow"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg/repocfgtest"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// The fake claude is this test binary re-executed: with KIRA_FAKE_CLAUDE=1 and "-p" first, TestMain
// plays one scripted agent run and exits. KIRA_FAKE_SCEN maps a repo name (the worktree's parent
// directory, "*" as fallback) to one action per attempt; the last action repeats.

const (
	fakeEnv  = "KIRA_FAKE_CLAUDE"
	fakeDir  = "KIRA_FAKE_DIR"
	fakeScen = "KIRA_FAKE_SCEN"
)

func TestMain(m *testing.M) {
	if os.Getenv(fakeEnv) == "1" && len(os.Args) > 1 && os.Args[1] == "-p" {
		os.Exit(runFakeClaude(os.Args[1:]))
	}
	os.Exit(testx.RunWithTempHomes(m))
}

func runFakeClaude(args []string) int {
	cwd, _ := os.Getwd()
	repo := filepath.Base(filepath.Dir(cwd))
	dir := os.Getenv(fakeDir)
	var scen map[string][]string
	_ = json.Unmarshal([]byte(os.Getenv(fakeScen)), &scen)
	actions := scen[repo]
	if len(actions) == 0 {
		actions = scen["*"]
	}
	counter := filepath.Join(dir, repo+".count")
	n := 0
	if raw, err := os.ReadFile(counter); err == nil {
		fmt.Sscanf(string(raw), "%d", &n)
	}
	n++
	_ = renameio.WriteFile(counter, []byte(fmt.Sprint(n)), 0o644)
	action := "done"
	if len(actions) > 0 {
		action = actions[min(n, len(actions))-1]
	}
	prompt, _ := readAll(os.Stdin)
	stem := fmt.Sprintf("%s-%d", repo, n)
	_ = os.WriteFile(filepath.Join(dir, stem+".args"), []byte(strings.Join(args, "\n")), 0o644)
	_ = os.WriteFile(filepath.Join(dir, stem+".prompt"), prompt, 0o644)

	emit := func(v any) {
		raw, _ := json.Marshal(v)
		fmt.Println(string(raw))
	}
	tool := func() {
		emit(map[string]any{"type": "assistant", "message": map[string]any{"content": []any{map[string]any{
			"type": "tool_use", "id": "t-bash", "name": "Bash", "input": map[string]any{"command": "git status"},
		}}}})
	}
	emit(map[string]any{"type": "system", "subtype": "init", "session_id": "fake"})
	finish := func(status string) int {
		cfg := argAfter(args, "--mcp-config")
		if err := fakeFinish(cfg, status, "summary-"+status); err != nil {
			fmt.Fprintln(os.Stderr, "finish_step:", err)
			return 3
		}
		emit(map[string]any{"type": "result", "subtype": "success", "num_turns": 2})
		return 0
	}
	switch action {
	case "done":
		tool()
		return finish("done")
	case "needs_input", "failed":
		return finish(action)
	case "sleep":
		time.Sleep(time.Hour)
	}
	emit(map[string]any{"type": "result", "subtype": "success", "num_turns": 1})
	return 0 // "nofinish"
}

func readAll(f *os.File) ([]byte, error) {
	var out []byte
	buf := make([]byte, 4096)
	for {
		n, err := f.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil {
			return out, nil
		}
	}
}

func argAfter(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

type fakeBearer struct{ token string }

func (b fakeBearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func fakeFinish(cfgPath, status, summary string) error {
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	var cfg struct {
		MCPServers map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	entry := cfg.MCPServers[agentmcp.ServerName]
	token := strings.TrimPrefix(entry.Headers["Authorization"], "Bearer ")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sess, err := mcp.NewClient(&mcp.Implementation{Name: "fake", Version: "1"}, nil).Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: entry.URL, HTTPClient: &http.Client{Transport: fakeBearer{token}}, DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		return err
	}
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "finish_step", Arguments: map[string]any{"status": status, "summary": summary, "reason": summary}})
	if err != nil {
		return err
	}
	if res.IsError {
		return fmt.Errorf("tool error: %v", res.Content)
	}
	return nil
}

// --- harness -----------------------------------------------------------------------------------

type engine struct {
	t       *testing.T
	repos   *repos.Repos
	board   *TaskBoard
	home    string
	fake    string
	workdir string
	mu      sync.Mutex
	logs    []adewire.LogEvent
	code    map[string]model.CodeRepo
	tracker *Tracker
	live    *fakeLive
	closed  []string
	files   *repocfg.Store
}

func newEngine(t *testing.T, scen map[string][]string) *engine {
	t.Helper()
	skipWithoutGitQueue(t)
	db, err := storage.OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	r, err := repos.New(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	registry := gitsession.NewRegistry(gitclient.NewExecRunner())
	t.Cleanup(registry.Close)
	e := &engine{t: t, repos: r, home: t.TempDir(), fake: t.TempDir(), workdir: t.TempDir(), code: map[string]model.CodeRepo{}}
	e.files = repocfgtest.New(t, db.DB, filepath.Join(t.TempDir(), "repos"))
	r.AdeRepoConfig.Files = e.files
	raw, _ := json.Marshal(scen)
	t.Setenv(fakeEnv, "1")
	t.Setenv(fakeDir, e.fake)
	t.Setenv(fakeScen, string(raw))
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	wfDir := filepath.Join(t.TempDir(), "workflows")
	if err := os.MkdirAll(wfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	e.workdir = wfDir
	e.live = newFakeLive()
	e.tracker = NewTracker(TrackerDeps{
		Agent: testAgents,
		Store: r.AdeSessions, LiveAgents: e.live.AgentSessions, WriteTerminal: e.live.Write, Now: time.Now,
		Grace: 20 * time.Millisecond, PendingTTL: time.Minute, AgentBin: self,
	})
	e.board = NewTaskBoard(TaskBoardDeps{
		Agent:       testAgents,
		Credentials: gitcred.New(),
		Tracker:     e.tracker,
		CloseTerminal: func(id string) error {
			e.live.remove(id)
			e.mu.Lock()
			e.closed = append(e.closed, id)
			e.mu.Unlock()
			return nil
		},
		Tasks: r.AdeTasks, Statuses: r.AdeStatuses, RepoConfig: r.AdeRepoConfig, Facts: r.AdeFacts, CodeRepos: r.CodeRepos,
		GitRepoSettings: e.files.Settings, Runner: gitclient.NewExecRunner(), Registry: registry,
		GitStatus: func(context.Context) gitclient.GitStatus { return gitclient.GitStatus{Kind: "ok", Path: "git"} },
		Workflows: &adeflow.Reader{Dir: wfDir, Store: r.AdeTasks},
		Logs:      r.AdeLogs, Sessions: r.AdeSessions, AgentDir: filepath.Join(t.TempDir(), "agent"), AgentBin: self,
		OnLog:            func(ev adewire.LogEvent) { e.mu.Lock(); e.logs = append(e.logs, ev); e.mu.Unlock() },
		AutofetchMinutes: func() int { return 0 }, HomeDir: e.home,
	})
	e.tracker.SetStoppedHandler(e.board.OnTUIStopped)
	t.Cleanup(e.board.Close)
	return e
}

// repo imports a real checkout named name (the code repo's name decides its worktree directory).
func (e *engine) repo(name string) {
	e.t.Helper()
	_, dir := initQueueRepo(e.t)
	sum, err := gitclient.Identify(context.Background(), gitclient.NewExecRunner(), "git", dir)
	if err != nil {
		e.t.Fatal(err)
	}
	rec, err := e.repos.CodeRepos.Create(model.CodeRepo{ID: name, Name: name, Root: sum.Root, RepoID: sum.RepoID})
	if err != nil {
		e.t.Fatal(err)
	}
	e.code[name] = rec
}

func (e *engine) prepare(repo, script string) {
	e.t.Helper()
	repocfgtest.Set(e.t, e.files, e.code[repo].RepoID, func(f *repocfg.File) {
		f.Worktree = &repocfg.Worktree{PrepareScript: &script}
	})
}

func (e *engine) workflow(yaml string) {
	e.t.Helper()
	if err := os.WriteFile(filepath.Join(e.workdir, "flow.yaml"), []byte(yaml), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e *engine) task(repoNames ...string) string {
	e.t.Helper()
	task, err := e.board.CreateTask(context.Background(), adewire.CreateTaskArgs{Title: "Fix login", CodeRepoIDs: repoNames, WorkflowID: "flow"})
	if err != nil {
		e.t.Fatal(err)
	}
	return task.ID
}

func (e *engine) start(taskID string) []string {
	e.t.Helper()
	tc, err := e.board.loadTaskCtx(taskID)
	if err != nil {
		e.t.Fatal(err)
	}
	names := map[string]string{}
	for _, br := range tc.branches { // the user names each branch at Run; an agent step would leave it to the agent
		if br.Kind == model.AdeBranchKindMine && br.Name == "" {
			names[br.ID] = "feat/" + slug(taskTitle(tc.task, tc.branches))
		}
	}
	res, err := e.board.StartRun(context.Background(), adewire.StartRunArgs{TaskID: taskID, BranchNames: names})
	if err != nil {
		e.t.Fatalf("StartRun: %v", err)
	}
	return res.RunIDs
}

// runs returns the latest run per step id and repo name.
func (e *engine) runs(taskID string) map[string]model.AdeRun {
	e.t.Helper()
	latest, err := e.repos.AdeTasks.LatestRuns(taskID)
	if err != nil {
		e.t.Fatal(err)
	}
	branches, err := e.repos.AdeTasks.BranchesLive()
	if err != nil {
		e.t.Fatal(err)
	}
	repoOf := map[string]string{}
	for _, b := range branches {
		repoOf[b.ID] = b.CodeRepoID
	}
	out := map[string]model.AdeRun{}
	for _, r := range latest {
		out[r.StepID+"/"+repoOf[r.BranchID]] = r
	}
	return out
}

func (e *engine) waitRun(taskID, key string, states ...string) model.AdeRun {
	e.t.Helper()
	var got model.AdeRun
	waitUntil(e.t, key+" in "+strings.Join(states, "|"), func() bool {
		r, ok := e.runs(taskID)[key]
		got = r
		for _, s := range states {
			if ok && r.State == s {
				return true
			}
		}
		return false
	})
	return got
}

func (e *engine) logText(kind, id string) string {
	e.t.Helper()
	var sb strings.Builder
	after := 0
	for {
		page, err := e.board.ReadLog(context.Background(), adewire.ReadLogArgs{Kind: kind, ID: id, AfterSeq: after})
		if err != nil {
			e.t.Fatal(err)
		}
		if len(page.Chunks) == 0 {
			return sb.String()
		}
		for _, c := range page.Chunks {
			sb.WriteString(c.Text)
			sb.WriteByte('\n')
		}
		after = page.NextSeq
	}
}

func (e *engine) fakeFile(name string) string {
	e.t.Helper()
	raw, err := os.ReadFile(filepath.Join(e.fake, name))
	if err != nil {
		e.t.Fatalf("fake claude never ran (%s): %v", name, err)
	}
	return string(raw)
}

func flowYAML(steps string) string { return "id: flow\nname: Flow\nsteps:\n" + steps }

func agentStep(id, extra string) string {
	return "  - id: " + id + "\n    name: " + id + "\n    kind: agent\n    runs_on: each repo\n    timeout: 1m\n    prompt: work on {branch} in {repo}\n" + extra
}

// backTo routes the failed result to an earlier step.
func backTo(target string) string {
	return "    results:\n      - id: done\n        ok: true\n      - id: failed\n        ok: false\n        next: " + target + "\n"
}

// selfReturn retries the step on failure, up to limit returns.
func selfReturn(id string, limit int) string {
	return backTo(id) + "    max_returns: " + strconv.Itoa(limit) + "\n"
}

const manualStep = "  - id: review\n    name: Review\n    kind: manual\n    status: In review\n"

// --- tests -------------------------------------------------------------------------------------

func TestRunEngine_eachRepoAutoAdvanceApprovalAndStepDone(t *testing.T) {
	e := newEngine(t, map[string][]string{"*": {"done"}})
	e.repo("api")
	e.repo("web")
	e.prepare("api", "echo prepared")
	e.workflow(flowYAML(
		agentStep("one", "    allowed_tools: [\"Bash(git *)\"]\n") + agentStep("two", "    before: approval\n") + manualStep))
	id := e.task("api", "web")

	if _, err := e.board.StepDone(context.Background(), id, nil, false); err == nil {
		t.Fatal("StepDone accepted before the steps ran")
	}
	if runIDs := e.start(id); len(runIDs) != 2 {
		t.Fatalf("StartRun run ids = %v, want one per repo", runIDs)
	}
	if _, err := e.board.StartRun(context.Background(), adewire.StartRunArgs{TaskID: id}); err == nil {
		t.Fatal("second StartRun accepted")
	}
	for _, repo := range []string{"api", "web"} {
		r := e.waitRun(id, "one/"+repo, model.AdeRunDone)
		if r.Summary != "summary-done" {
			t.Fatalf("%s run = %+v; want the finish summary", repo, r)
		}
	}
	if _, queued := e.runs(id)["two/api"]; queued {
		t.Fatal("approval step started without Approve")
	}

	args := e.fakeFile("api-1.args")
	for _, want := range []string{"--output-format\nstream-json", "--setting-sources\nproject,local", toolName(agentmcp.FinishStep), "Bash(git *)"} {
		if !strings.Contains(args, want) {
			t.Fatalf("argv lacks %q:\n%s", want, args)
		}
	}
	cfg, _ := os.ReadFile(argAfterJoined(args, "--mcp-config"))
	if len(cfg) != 0 {
		t.Fatalf("MCP config %s outlives its run", argAfterJoined(args, "--mcp-config"))
	}
	if strings.Contains(args, "Bearer") {
		t.Fatal("bearer token on argv")
	}
	if prompt := e.fakeFile("api-1.prompt"); !strings.Contains(prompt, "Step 1/2: one") || !strings.Contains(prompt, agentmcp.FinishInstruction(llmprompts.Defaults(), nil)) {
		t.Fatalf("prompt = %q", prompt)
	}

	sessions, err := e.board.Sessions(context.Background())
	if err != nil || len(sessions.Sessions) != 2 {
		t.Fatalf("sessions = %+v, %v", sessions, err)
	}
	for _, s := range sessions.Sessions {
		if s.Mode != "headless" || s.State != "stopped" || s.TaskID != id {
			t.Fatalf("session = %+v", s)
		}
	}
	runLog := e.logText("run", e.runs(id)["one/api"].ID)
	if !strings.Contains(runLog, "▸ Bash git status") || !strings.Contains(runLog, "result: success") {
		t.Fatalf("run log = %q", runLog)
	}
	setupLog := e.logText("setup", branchOf(t, e, id, "api"))
	if !strings.Contains(setupLog, "prepared") {
		t.Fatalf("setup log = %q", setupLog)
	}

	if err := e.board.Approve(context.Background(), adewire.StepArgs{TaskID: id, StepID: "one"}); err == nil {
		t.Fatal("Approve accepted for a step that is not waiting")
	}
	if err := e.board.Approve(context.Background(), adewire.StepArgs{TaskID: id, StepID: "two"}); err != nil {
		t.Fatal(err)
	}
	e.waitRun(id, "two/api", model.AdeRunDone)
	e.waitRun(id, "two/web", model.AdeRunDone)

	task, err := e.board.StepDone(context.Background(), id, nil, false)
	if err != nil || task.EntryStepID != "done" {
		t.Fatalf("StepDone = %+v, %v; want done", task, err)
	}
}

func TestRunEngine_failureRules(t *testing.T) {
	cases := []struct {
		name  string
		scen  []string
		steps string
		key   string
		state string
		note  string
	}{
		{"needs_input is stuck", []string{"needs_input"}, agentStep("first", ""), "first/api", model.AdeRunStuck, "summary-needs_input"},
		{"stop keeps a failure", []string{"failed"}, agentStep("first", ""), "first/api", model.AdeRunFailed, "summary-failed"},
		{"no finish_step", []string{"nofinish"}, agentStep("first", ""), "first/api", model.AdeRunFailed, "no report: Claude ended without calling finish_step"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEngine(t, map[string][]string{"*": tc.scen})
			e.repo("api")
			e.workflow(flowYAML(tc.steps))
			id := e.task("api")
			e.start(id)
			r := e.waitRun(id, tc.key, tc.state)
			if r.Note != tc.note {
				t.Fatalf("run = %+v; want note %q", r, tc.note)
			}
			time.Sleep(300 * time.Millisecond)
			if latest := e.runs(id)[tc.key]; latest.ID != r.ID || latest.State != tc.state {
				t.Fatalf("engine moved on after %s: %+v", tc.state, latest)
			}
		})
	}
}

func TestRunEngine_retryAndTimeout(t *testing.T) {
	t.Run("RetryRun after needs_input", func(t *testing.T) {
		e := newEngine(t, map[string][]string{"*": {"needs_input", "done"}})
		e.repo("api")
		e.workflow(flowYAML(agentStep("first", "")))
		id := e.task("api")
		e.start(id)
		stuck := e.waitRun(id, "first/api", model.AdeRunStuck)
		if err := e.board.RetryRun(context.Background(), stuck.ID); err != nil {
			t.Fatal(err)
		}
		r := e.waitRun(id, "first/api", model.AdeRunDone)
		if r.Attempt != 2 {
			t.Fatalf("attempt = %d, want 2", r.Attempt)
		}
		if err := e.board.RetryRun(context.Background(), r.ID); err == nil {
			t.Fatal("RetryRun accepted a done run")
		}
	})
	t.Run("retry 1 then done", func(t *testing.T) {
		e := newEngine(t, map[string][]string{"*": {"failed", "done"}})
		e.repo("api")
		e.workflow(flowYAML(agentStep("first", selfReturn("first", 1))))
		id := e.task("api")
		e.start(id)
		if r := e.waitRun(id, "first/api", model.AdeRunDone); r.Attempt != 2 {
			t.Fatalf("attempt = %d, want 2", r.Attempt)
		}
	})
	t.Run("timeout fails the run", func(t *testing.T) {
		e := newEngine(t, map[string][]string{"*": {"sleep"}})
		e.repo("api")
		steps := strings.Replace(agentStep("first", ""), "timeout: 1m", "timeout: 1s", 1)
		e.workflow(flowYAML(steps))
		id := e.task("api")
		e.start(id)
		if r := e.waitRun(id, "first/api", model.AdeRunFailed); r.Note != "no report: timed out after 1s" {
			t.Fatalf("note = %q", r.Note)
		}
	})
}

func TestRunEngine_setupGate(t *testing.T) {
	e := newEngine(t, map[string][]string{"*": {"done"}})
	e.repo("api")
	gate := filepath.Join(t.TempDir(), "gate")
	e.prepare("api", "while [ ! -f "+gate+" ]; do sleep 0.05; done; exit 3")
	e.workflow(flowYAML(agentStep("first", "")))
	id := e.task("api")
	e.start(id)

	if r := e.runs(id)["first/api"]; r.State != model.AdeRunPending || r.Note != noteWaitingSetup {
		t.Fatalf("run behind a running setup = %+v", r)
	}
	branch := branchOf(t, e, id, "api")
	if err := e.board.RetrySetup(context.Background(), branch); err == nil {
		t.Fatal("RetrySetup accepted a running setup")
	}
	if err := os.WriteFile(gate, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "setup failed", func() bool {
		s, _ := e.repos.AdeTasks.GetSetup(branch)
		return s != nil && s.State == model.AdeSetupFailed
	})
	if r := e.runs(id)["first/api"]; r.State != model.AdeRunPending || r.Note != noteSetupFailed {
		t.Fatalf("run after a failed setup = %+v; want pending with %q", r, noteSetupFailed)
	}
	if !strings.Contains(e.logText("setup", branch), "exited with status 3") {
		t.Fatal("setup log does not name the failure")
	}
	e.prepare("api", "echo fixed")
	if err := e.board.RetrySetup(context.Background(), branch); err != nil {
		t.Fatal(err)
	}
	e.waitRun(id, "first/api", model.AdeRunDone)
}

func TestRunEngine_onceOnlyAndUnresolved(t *testing.T) {
	e := newEngine(t, map[string][]string{"*": {"done"}})
	e.repo("api")
	e.repo("web")
	e.workflow(flowYAML(strings.Replace(agentStep("solo", ""), "each repo", "once", 1)))
	id := e.task("api", "web")
	if ids := e.start(id); len(ids) != 1 {
		t.Fatalf("once step run ids = %v", ids)
	}
	r := e.waitRun(id, "solo/api", model.AdeRunDone)
	sessions, _ := e.board.Sessions(context.Background())
	if len(sessions.Sessions) != 1 || !strings.Contains(sessions.Sessions[0].Cwd, string(filepath.Separator)+"api"+string(filepath.Separator)) || r.BranchID == "" {
		t.Fatalf("once ran outside the first branch's worktree: %+v", sessions.Sessions)
	}

	e2 := newEngine(t, nil)
	e2.repo("api")
	e2.workflow(flowYAML(strings.Replace(agentStep("solo", ""), "each repo", "only nope", 1)))
	id2 := e2.task("api")
	if _, err := e2.board.StartRun(context.Background(), adewire.StartRunArgs{TaskID: id2}); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("unresolved only: %v", err)
	}
}

func TestRunEngine_scriptStep(t *testing.T) {
	script := func(command string) string {
		return flowYAML("  - id: ship\n    name: Ship\n    kind: script\n    status: In review\n    runs_on: each repo\n    timeout: 1m\n    command: |\n      " + command + "\n")
	}
	t.Run("quoted variables and done", func(t *testing.T) {
		e := newEngine(t, nil)
		e.repo("api")
		e.workflow(script("echo {branch} {repo}"))
		id := e.task("api")
		e.start(id)
		r := e.waitRun(id, "ship/api", model.AdeRunDone)
		if r.ExitCode == nil || *r.ExitCode != 0 {
			t.Fatalf("run = %+v", r)
		}
		if out := e.logText("run", r.ID); !strings.Contains(out, "feat/fix-login") {
			t.Fatalf("log = %q", out)
		}
		if sessions, _ := e.board.Sessions(context.Background()); len(sessions.Sessions) != 0 {
			t.Fatalf("script step created sessions: %+v", sessions.Sessions)
		}
	})
	t.Run("non-zero exit fails", func(t *testing.T) {
		e := newEngine(t, nil)
		e.repo("api")
		e.workflow(script("exit 3"))
		id := e.task("api")
		e.start(id)
		r := e.waitRun(id, "ship/api", model.AdeRunFailed)
		if r.Note != "exited with status 3" || r.ExitCode == nil || *r.ExitCode != 3 {
			t.Fatalf("run = %+v", r)
		}
	})
	t.Run("log tail cap and paging", func(t *testing.T) {
		e := newEngine(t, nil)
		e.repo("api")
		e.workflow(script("head -c 3000000 /dev/zero | tr '\\0' a | fold -w 1000"))
		id := e.task("api")
		e.start(id)
		r := e.waitRun(id, "ship/api", model.AdeRunDone)
		total, truncated, after, pages := 0, false, 0, 0
		for {
			page, err := e.board.ReadLog(context.Background(), adewire.ReadLogArgs{Kind: "run", ID: r.ID, AfterSeq: after})
			if err != nil {
				t.Fatal(err)
			}
			truncated = truncated || page.Truncated
			if len(page.Chunks) == 0 {
				break
			}
			if !page.Done {
				t.Fatal("finished run reports an unfinished log")
			}
			for _, c := range page.Chunks {
				total += len(c.Text)
			}
			after = page.NextSeq
			pages++
		}
		if !truncated || total > 2<<20+(64<<10) || total < 1<<20 {
			t.Fatalf("log: truncated=%v bytes=%d pages=%d; want a truncated tail near 2 MiB", truncated, total, pages)
		}
	})
}

// branchOf returns the task's branch id for a code repo.
func branchOf(t *testing.T, e *engine, taskID, repo string) string {
	t.Helper()
	branches, err := e.repos.AdeTasks.BranchesLive()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range branches {
		if b.TaskID == taskID && b.CodeRepoID == repo {
			return b.ID
		}
	}
	t.Fatalf("no branch of %s on %s", repo, taskID)
	return ""
}

// argAfterJoined reads a flag value out of the newline-joined argv the fake wrote.
func argAfterJoined(args, flag string) string {
	return argAfter(strings.Split(args, "\n"), flag)
}

func TestRunEngine_setTaskWorkflow(t *testing.T) {
	e := newEngine(t, map[string][]string{"*": {"done"}})
	e.repo("api")
	e.workflow(flowYAML(agentStep("first", "") + manualStep))
	other := "id: other\nname: Other\nsteps:\n" + manualStep
	if err := os.WriteFile(filepath.Join(e.workdir, "other.yaml"), []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	id := e.task("api")
	e.start(id)
	run := e.waitRun(id, "first/api", model.AdeRunDone)
	if _, err := e.board.SetTaskWorkflow(ctx, adewire.SetTaskWorkflowArgs{TaskID: id, WorkflowID: "flow"}); err != nil {
		t.Fatalf("same id: %v", err)
	}
	if len(e.runs(id)) != 1 {
		t.Fatal("same id deleted runs")
	}
	task, err := e.board.SetTaskWorkflow(ctx, adewire.SetTaskWorkflowArgs{TaskID: id, WorkflowID: "other"})
	if err != nil {
		t.Fatal(err)
	}
	if task.WorkflowID != "other" || len(e.runs(id)) != 0 {
		t.Fatalf("workflow = %q, runs = %d", task.WorkflowID, len(e.runs(id)))
	}
	if _, err := e.board.ReadLog(ctx, adewire.ReadLogArgs{Kind: "run", ID: run.ID}); err == nil {
		t.Fatal("run log survived")
	}
	if _, err := e.board.SetTaskWorkflow(ctx, adewire.SetTaskWorkflowArgs{TaskID: id, WorkflowID: "nope"}); err == nil {
		t.Fatal("unknown workflow accepted")
	}

	sleeper := newEngine(t, map[string][]string{"*": {"sleep"}})
	sleeper.repo("api")
	sleeper.workflow(flowYAML(agentStep("first", "")))
	sid := sleeper.task("api")
	sleeper.start(sid)
	sleeper.waitRun(sid, "first/api", model.AdeRunRunning)
	if _, err := sleeper.board.SetTaskWorkflow(ctx, adewire.SetTaskWorkflowArgs{TaskID: sid, WorkflowID: ""}); err == nil ||
		!strings.Contains(err.Error(), "stop its running agents first") {
		t.Fatalf("running run: err = %v", err)
	}
}
