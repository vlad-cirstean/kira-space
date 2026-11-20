package claude

// P643 benchmark harness (plan section 14.1). Opt-in, spends real tokens, never in hooks or CI:
//
//	KIRA_BENCH=run KIRA_BENCH_OUT=/tmp/bench.md bun run test:claude:details
//
// Knobs: KIRA_BENCH_FLOWS (comma list, default F1), KIRA_BENCH_ARMS (default: every registered arm),
// KIRA_BENCH_REPS (default 3), KIRA_BENCH_BUDGET (USD for the round, default 1.5). KIRA_BENCH_OUT is
// required: results never go to a default path. Arm T serves the frozen tool list in
// testdata/details-bench/tools-baseline.json over stub backends. Arm R is the real agentmcp.Server (state, the
// finish flush) and the files prompt fragment; its oracles are the spike flows' own.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/ade"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/harness/providers"
)

const (
	benchFixtures = "testdata/details-bench"
	benchRunCap   = 0.5
)

// benchSetup, when set, builds the tool surface of a run in place of the spike's own (see spikeRun.setup).
var benchSetup func(*testing.T, *spikeRun, spikeFlow) (string, []string, func())

// benchRunCapUSD is execute's per-run budget while the benchmark runs; zero keeps the spike cap.
var benchRunCapUSD float64

// benchRoles are the grant shapes whose tool lists the baseline freezes, by flow.
var benchRoles = []struct{ name, flow string }{
	{"run", "F5"}, {"run-proof", "F2"}, {"orchestrator", "F4"},
}

func benchRoleOf(flow string) string {
	switch flow {
	case "F2", "F3", "F3b":
		return "run-proof"
	case "F4", "F6":
		return "orchestrator"
	}
	return "run"
}

// benchBaseline is tools-baseline.json: per role, per MCP server, the tools as the client sees them,
// and today's finish_step acknowledgement.
type benchBaseline struct {
	Roles     map[string]map[string][]*mcp.Tool `json:"roles"`
	FinishAck string                            `json:"finishAck"`
}

func loadBenchBaseline(t *testing.T) benchBaseline {
	t.Helper()
	var b benchBaseline
	if err := json.Unmarshal(mustRead(t, filepath.Join(benchFixtures, "tools-baseline.json")), &b); err != nil {
		t.Fatal(err)
	}
	return b
}

// benchArmSetups are the registered arms. T serves the frozen tools; R serves the real agentmcp.Server
// (state, the finish flush) over the task folder the files prompt names.
var benchArmSetups = map[string]func(*testing.T, benchBaseline) func(*testing.T, *spikeRun, spikeFlow) (string, []string, func()){
	"T": benchSetupT,
	"R": benchSetupR,
}

// benchSpaceR backs arm R's server: step.finish reads the stage's draft in the task folder as
// the board does (the ade package is not importable from this module); branch.request answers once the
// branch's setup is done, as the board does after the user approves.
type benchSpaceR struct {
	*spikeSpace
	draft string
}

func (s *benchSpaceR) body() string {
	b, _ := os.ReadFile(s.draft)
	return strings.TrimSpace(string(b))
}

func (s *benchSpaceR) SubmitForFinish(_ context.Context, _ agentmcp.SpaceCall, _ string, ok bool) (agentmcp.SubmitFinish, error) {
	body := s.body()
	if body == "" && ok {
		return agentmcp.SubmitFinish{Refuse: "write handovers/draft-implement.md, then call state step.finish"}, nil
	}
	return agentmcp.SubmitFinish{Summary: body}, nil
}

func (s *benchSpaceR) RequestBranches(ctx context.Context, _ agentmcp.SpaceCall, specs []agentmcp.BranchSpec) (agentmcp.RequestBranchesResult, error) {
	if d := time.Until(s.br.ReadyAt); d > 0 {
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return agentmcp.RequestBranchesResult{}, ctx.Err()
		}
	}
	var out agentmcp.RequestBranchesResult
	for _, sp := range specs {
		out.Branches = append(out.Branches, agentmcp.RequestedBranch{
			BranchInfo: agentmcp.BranchInfo{Repo: sp.Repo, Branch: sp.Branch, Worktree: s.br.Worktree, Setup: agentmcp.SetupInfo{State: "done", Note: "setup done"},
				Next: "Ready. Work in " + s.br.Worktree + "."},
			Outcome: agentmcp.BranchExisting,
		})
	}
	return out, nil
}

func (s *benchSpaceR) SpawnTask(ctx context.Context, c agentmcp.SpaceCall, a agentmcp.CreateTaskArgs) (agentmcp.CreateTaskResult, error) {
	res, err := s.spikeSpace.SpawnTask(ctx, c, a)
	res.Next = "The child starts at its first stage with your brief as its first handover. Read its progress in state.yaml, chain."
	return res, err
}

// benchSetupR is the implementation arm: the real tool surface of a Space grant (state, and
// create_task for an orchestrator) and the task folder with its Kira-written files, granted with --add-dir.
func benchSetupR(*testing.T, benchBaseline) func(*testing.T, *spikeRun, spikeFlow) (string, []string, func()) {
	return func(t *testing.T, r *spikeRun, fl spikeFlow) (string, []string, func()) {
		if fl.id == "F2" || fl.id == "F3" || fl.id == "F3b" {
			return benchSetupRProof(t, r, fl)
		}
		orch := fl.id == "F4" || fl.id == "F6"
		in := taskfiles.ReadmeInput{TaskID: "t-1", Title: "Orders ledger", Step: "implement"}
		addDirs := []string{r.taskDir}
		if fl.id == "F4" {
			for _, c := range []struct{ id, title string }{{"c-billing", "Billing API"}, {"c-settings", "Settings UI"}} {
				dir := filepath.Join(r.tasks, c.id)
				seedHistory(t, filepath.Join(benchFixtures, "children", c.id), dir, false)
				in.Children = append(in.Children, taskfiles.ReadmeChild{ID: c.id, Title: c.title, LatestHandover: "handovers/0003-review-to-implement.md"})
				addDirs = append(addDirs, dir)
			}
		}
		writeFile(t, filepath.Join(r.taskDir, "README.md"), taskfiles.Readme(in))
		seedHistory(t, filepath.Join(benchFixtures, "t1"), r.taskDir, false)
		writeFile(t, filepath.Join(r.taskDir, "task.yaml"), string(mustRead(t, filepath.Join(benchFixtures, "t1", "task.yaml"))))
		stateID := strings.TrimSuffix(fl.id, "b") // F1b runs on F1's state
		stateSrc := filepath.Join(benchFixtures, "states-r", stateID+".yaml")
		stateYAML := func(done bool) string {
			b := strings.ReplaceAll(string(mustRead(t, stateSrc)), "__WT__", r.wt)
			if done {
				b = strings.Replace(strings.Replace(b, "status: waiting", "status: done", 1), "installing dependencies", "setup done", 1)
			}
			return taskfiles.Header(taskfiles.KindState) + b
		}
		statePath := filepath.Join(r.taskDir, "state.yaml")
		writeFile(t, statePath, stateYAML(false))
		if fl.id == "F5" { // Kira rewrites state.yaml when setup ends
			time.AfterFunc(time.Until(r.ready), func() { _ = os.WriteFile(statePath, []byte(stateYAML(true)), 0o644) })
		}
		space := &benchSpaceR{spikeSpace: r.space, draft: filepath.Join(r.taskDir, "handovers", "draft-implement.md")}
		if fl.id == "F1b" { // the task already holds the handover draft to revise
			writeFile(t, space.draft, r.lib.latest().body)
		}
		srv := agentmcp.NewServer(agentmcp.Options{
			Dir: filepath.Join(r.root, "mcp"), Space: space, Orchestrator: space,
			OnFinish: func(_ string, f agentmcp.Finish) { r.finishes = append(r.finishes, f) },
		})
		claudeProv, ok := providers.New().Get("claude")
		if !ok {
			t.Fatal("no claude provider")
		}
		cfg, release, err := srv.Register(agentmcp.Grant{RunID: "run-1", TaskID: "t-1", Space: true, Orchestrator: orch, Agent: claudeProv})
		if err != nil {
			t.Fatal(err)
		}
		return cfg, addDirs, func() { release(); _ = srv.Close() }
	}
}

func textResult(v any) (*mcp.CallToolResult, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	// The frozen tools declare an output schema: the client rejects a reply without structured content.
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}, StructuredContent: v}, nil
}

func errResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}

// benchSetupT serves the frozen tools of the flow's role. Each tool the T flows call is answered by
// the spike stubs; any other tool replies with an error, as a flow that needed it is not in the set.
func benchSetupT(_ *testing.T, base benchBaseline) func(*testing.T, *spikeRun, spikeFlow) (string, []string, func()) {
	return func(t *testing.T, r *spikeRun, fl spikeFlow) (string, []string, func()) {
		role := base.Roles[benchRoleOf(fl.id)]
		if role == nil {
			t.Fatalf("baseline has no role for %s", fl.id)
		}
		handlers := map[string]mcp.ToolHandler{
			"task_info": func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				var a struct {
					Repo, Branch      string
					WaitSeconds       int
					IncludeRegistered bool
				}
				if err := json.Unmarshal(req.Params.Arguments, &a); err != nil {
					return errResult(err.Error()), nil
				}
				if (a.Repo == "") != (a.Branch == "") {
					return errResult("repo and branch go together; give both or neither."), nil
				}
				var br *agentmcp.BranchInfo
				if a.Repo != "" {
					st, _ := r.space.BranchStatus(ctx, "", a.Repo, a.Branch, time.Duration(a.WaitSeconds)*time.Second)
					br = &st
				}
				info, _ := r.space.TaskInfo(ctx, "", a.IncludeRegistered)
				info.Branch = br
				return textResult(info)
			},
			"finish_step": func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				var a struct {
					Status, Summary, Reason string
					ConflictedFiles         []string
					LastGitError, Tried     string
				}
				if err := json.Unmarshal(req.Params.Arguments, &a); err != nil {
					return errResult(err.Error()), nil
				}
				r.finishes = append(r.finishes, agentmcp.Finish{Status: a.Status, Summary: a.Summary, Reason: a.Reason,
					ConflictedFiles: a.ConflictedFiles, LastGitError: a.LastGitError, Tried: a.Tried})
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: base.FinishAck}}}, nil
			},
			"list_tasks": func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				res, _ := r.space.ListTasks(ctx, agentmcp.SpaceCall{}, frozenListTasksArgs{})
				return textResult(res)
			},
			"task_chain": func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				var a frozenTaskChainArgs
				if err := json.Unmarshal(req.Params.Arguments, &a); err != nil {
					return errResult(err.Error()), nil
				}
				res, _ := r.space.TaskChain(ctx, agentmcp.SpaceCall{}, a)
				return textResult(res)
			},
			"create_task": func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				var a agentmcp.CreateTaskArgs
				if err := json.Unmarshal(req.Params.Arguments, &a); err != nil {
					return errResult(err.Error()), nil
				}
				res, _ := r.space.SpawnTask(ctx, agentmcp.SpaceCall{}, a)
				return textResult(res)
			},
			"proof_save": func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				var a spikeSaveArgs
				if err := json.Unmarshal(req.Params.Arguments, &a); err != nil {
					return errResult(err.Error()), nil
				}
				res, out, err := r.studio.save(ctx, req, a)
				return studioReply(res, out, err)
			},
			"proof_list": func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				var a spikeListArgs
				if err := json.Unmarshal(req.Params.Arguments, &a); err != nil {
					return errResult(err.Error()), nil
				}
				res, out, err := r.studio.list(ctx, req, a)
				return studioReply(res, out, err)
			},
		}
		servers := map[string]any{}
		var closers []func()
		for name, tools := range role {
			srv := mcp.NewServer(&mcp.Implementation{Name: name, Version: "frozen"}, nil)
			for _, tl := range tools {
				h := handlers[tl.Name]
				if h == nil {
					tn := tl.Name
					h = func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
						return errResult(tn + " is not available in this benchmark run."), nil
					}
				}
				srv.AddTool(tl, h)
			}
			ts := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{Stateless: true}))
			closers = append(closers, ts.Close)
			servers[name] = map[string]any{"type": "http", "url": ts.URL}
		}
		cfg := filepath.Join(r.root, "mcp.json")
		raw, _ := json.Marshal(map[string]any{"mcpServers": servers})
		if err := os.WriteFile(cfg, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		return cfg, nil, func() {
			for _, c := range closers {
				c()
			}
			r.studio.close()
		}
	}
}

func studioReply(res *mcp.CallToolResult, out any, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return nil, err
	}
	if res != nil {
		return res, nil
	}
	return textResult(out)
}

// TestDetailsBench runs the requested flows on the registered arms and writes the comparison.
func TestDetailsBench(t *testing.T) {
	out := os.Getenv("KIRA_BENCH_OUT")
	if os.Getenv("KIRA_BENCH") != "run" || out == "" {
		t.Skip("P643 benchmark spends real tokens; set KIRA_BENCH=run and KIRA_BENCH_OUT (see the file comment)")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("no claude on PATH")
	}
	base := loadBenchBaseline(t)
	list := func(env, def string) []string {
		v := os.Getenv(env)
		if v == "" {
			v = def
		}
		var s []string
		for _, f := range strings.Split(v, ",") {
			if f = strings.TrimSpace(f); f != "" {
				s = append(s, f)
			}
		}
		return s
	}
	var registered []string
	for a := range benchArmSetups {
		registered = append(registered, a)
	}
	slices.Sort(registered)
	arms := list("KIRA_BENCH_ARMS", strings.Join(registered, ","))
	for _, a := range arms {
		if benchArmSetups[a] == nil {
			t.Fatalf("arm %q is not registered; registered: %v", a, registered)
		}
	}
	flows := list("KIRA_BENCH_FLOWS", "F1")
	reps := 3
	if v, err := strconv.Atoi(os.Getenv("KIRA_BENCH_REPS")); err == nil && v > 0 {
		reps = v
	}
	budget := 1.5
	if v, err := strconv.ParseFloat(os.Getenv("KIRA_BENCH_BUDGET"), 64); err == nil && v > 0 {
		budget = v
	}

	spike1bOn, spikeFixtures1b, benchRunCapUSD = true, benchFixtures, benchRunCap
	defer func() {
		spike1bOn, spikeFixtures1b, benchRunCapUSD, benchSetup = false, "testdata/details-spike-1b", 0, nil
	}()
	home := spikeHome(t)

	var recs []spikeRec
	var spent float64
	note := ""
	planned := len(flows) * len(arms) * reps
loop:
	for rep := 1; rep <= reps; rep++ {
		for _, id := range flows {
			fl := benchFlow(id)
			for _, arm := range arms {
				// The first run may cost the cap; later ones are bounded by the dearest run seen.
				next := benchRunCap
				if len(recs) > 0 {
					next = 0
					for _, r := range recs {
						next = max(next, r.Cost)
					}
				}
				if spent+next > budget {
					note = fmt.Sprintf("Budget guard: spent %.4f USD, next run could add %.2f, budget %.2f. Remaining runs skipped.", spent, next, budget)
					break loop
				}
				benchSetup = benchArmSetups[arm](t, base)
				var rec spikeRec
				if benchMultiPass(id) {
					var passes []spikeRec
					rec, passes = executePasses(t, home, fl, arm, "sonnet", rep)
					for i, p := range passes { // per-pass records feed the crossover table; their cost is already in rec
						p.Flow = fmt.Sprintf("%s#%d", id, i+1)
						recs = append(recs, p)
					}
				} else {
					rec = execute(t, home, fl, arm, "sonnet", rep)
				}
				spent += rec.Cost
				recs = append(recs, rec)
				t.Logf("%s %s #%d pass=%v cost=%.4f turns=%d spent=%.4f %s", rec.Flow, rec.Arm, rec.Rep, rec.Pass, rec.Cost, rec.Turns, spent, rec.Why)
				if err := os.WriteFile(out, []byte(benchReport(recs, flows, arms, spent, planned, note)), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if err := os.WriteFile(out, []byte(benchReport(recs, flows, arms, spent, planned, note)), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("benchmark spend %.4f USD over %d runs; results in %s", spent, len(recs), out)
}

// benchReport is the per-flow table and, where both T and R ran, the gate of plan 14.1: R over T
// cost at most 1.2, final context at most 1.0, pass count not lower, failed edit-path calls plus
// retries not higher. A metric whose median gap is below the larger run-to-run range is inconclusive.
func benchReport(recs []spikeRec, flows, arms []string, spent float64, planned int, note string) string {
	var b strings.Builder
	runs := 0
	for _, r := range recs {
		if !strings.Contains(r.Flow, "#") { // per-pass records are parts of a run
			runs++
		}
	}
	fmt.Fprintf(&b, "# P643 benchmark\n\nRuns: %d of %d planned, sonnet. Spend: %.4f USD. %s\n\n", runs, planned, spent, note)
	b.WriteString("| Flow | Arm | Pass | Cost USD (median) | Final context (median) | Turns (median) | Failed edit-path + retries (mean) |\n|---|---|---|---|---|---|---|\n")
	agg := map[string]spikeAgg{}
	for _, f := range flows {
		for _, a := range arms {
			g := spikeAggregate(recs, f, a, "sonnet")
			agg[f+"/"+a] = g
			if g.runs == 0 {
				continue
			}
			fe := make([]float64, len(g.failed))
			for i := range fe {
				fe[i] = g.failed[i] + g.retr[i]
			}
			fmt.Fprintf(&b, "| %s | %s | %d of %d | %.4f | %.0f | %.0f | %.2f |\n", f, a, g.pass, g.runs, median(g.cost), median(g.cf), median(g.turns), mean(fe))
		}
	}
	if slices.Contains(arms, "T") && slices.Contains(arms, "R") {
		b.WriteString("\n### Gate, R over T\n\n| Flow | Cost R/T | Context R/T | Pass R vs T | Failed + retries R vs T | Verdict |\n|---|---|---|---|---|---|\n")
		for _, f := range flows {
			t, r := agg[f+"/T"], agg[f+"/R"]
			if t.runs == 0 || r.runs == 0 {
				continue
			}
			cost, ctx := median(r.cost)/median(t.cost), median(r.cf)/median(t.cf)
			ok := cost <= 1.2 && ctx <= benchCtxBound(f) && r.pass >= t.pass && fails(r) <= fails(t)
			verdict := "pass"
			if benchMultiPass(f) { // new flows: the existing bounds are shown for information only
				verdict = "informational, not gated"
			} else if !ok {
				verdict = "fail"
				if inconclusive(t.cost, r.cost) || inconclusive(t.cf, r.cf) {
					verdict = "fail, inconclusive: Opus review of the raw runs"
				}
			}
			fmt.Fprintf(&b, "| %s | %.2f (max %.2f) | %.2f (max %.2f) | %d vs %d | %.2f vs %.2f | %s |\n", f, cost, 1.2, ctx, benchCtxBound(f), r.pass, t.pass, fails(r), fails(t), verdict)
		}
	}
	b.WriteString(benchPassTable(recs, flows, arms))
	b.WriteString("\n### Runs\n\n| Flow | Arm | Rep | Pass | Cost | Turns | Why |\n|---|---|---|---|---|---|---|\n")
	for _, r := range recs {
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %.4f | %d | %s |\n", r.Flow, r.Arm, r.Rep, yn(r.Pass), r.Cost, r.Turns, strings.ReplaceAll(r.Why, "|", "/"))
	}
	return b.String()
}

// benchCtxBound is the context gate of plan 14.1: 1.05 on the file-read flows F1 to F4, 1.0 on F5 and F6.
func benchCtxBound(flow string) float64 {
	if flow == "F5" || flow == "F6" {
		return 1.0
	}
	return 1.05
}

// fails is the mean of failed edit-path calls plus retries per run.
func fails(g spikeAgg) float64 {
	fe := make([]float64, len(g.failed))
	for i := range fe {
		fe[i] = g.failed[i] + g.retr[i]
	}
	return mean(fe)
}

// inconclusive is true when the gap between the medians is smaller than the larger run-to-run range.
func inconclusive(a, b []float64) bool {
	rng := func(v []float64) float64 {
		if len(v) == 0 {
			return 0
		}
		return slices.Max(v) - slices.Min(v)
	}
	gap := median(a) - median(b)
	if gap < 0 {
		gap = -gap
	}
	return gap < max(rng(a), rng(b))
}

// TestDetailsBenchFrozenServes checks, with no claude and no tokens, that arm T serves exactly the
// frozen tools and answers the calls its flows make.
func TestDetailsBenchFrozenServes(t *testing.T) {
	base := loadBenchBaseline(t)
	spike1bOn, spikeFixtures1b = true, benchFixtures
	benchSetup = benchSetupT(t, base)
	defer func() { spike1bOn, spikeFixtures1b, benchSetup = false, "testdata/details-spike-1b", nil }()
	for _, flow := range []string{"F5", "F2", "F4"} {
		r := &spikeRun{t: t, flow: flow, arm: "T", lib: loadSpikeLib(t)}
		fl := flowByIDs(spikeFlows1b, flow)
		cfg, _, done := r.setup(t, fl)
		var conf struct {
			MCPServers map[string]struct {
				URL string `json:"url"`
			} `json:"mcpServers"`
		}
		if err := json.Unmarshal(mustRead(t, cfg), &conf); err != nil {
			t.Fatal(err)
		}
		for name, want := range base.Roles[benchRoleOf(flow)] {
			tools, err := listTools(t, conf.MCPServers[name].URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(tools) != len(want) {
				t.Fatalf("%s %s: %d tools, baseline %d", flow, name, len(tools), len(want))
			}
			for _, tl := range tools {
				i := slices.IndexFunc(want, func(w *mcp.Tool) bool { return w.Name == tl.Name })
				if i < 0 {
					t.Fatalf("%s %s: tool %s is not in the baseline", flow, name, tl.Name)
				}
				got, _ := json.Marshal(tl)
				exp, _ := json.Marshal(want[i])
				if !bytes.Equal(got, exp) {
					t.Fatalf("%s %s: tool %s differs from the baseline", flow, name, tl.Name)
				}
			}
		}
		cl := mcp.NewClient(&mcp.Implementation{Name: "check", Version: "1"}, nil)
		sess, err := cl.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: conf.MCPServers["kira-space"].URL}, nil)
		if err != nil {
			t.Fatal(err)
		}
		call := func(tool string, args map[string]any) *mcp.CallToolResult {
			res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
			if err != nil {
				t.Fatalf("%s: %v", tool, err)
			}
			return res
		}
		// The CLI refuses a reply without structured content that fits the tool's output schema.
		fitsSchema := func(tool string, args map[string]any) {
			i := slices.IndexFunc(base.Roles[benchRoleOf(flow)]["kira-space"], func(w *mcp.Tool) bool { return w.Name == tool })
			if i < 0 {
				return
			}
			res := call(tool, args)
			raw, _ := json.Marshal(base.Roles[benchRoleOf(flow)]["kira-space"][i].OutputSchema)
			var sc jsonschema.Schema
			if err := json.Unmarshal(raw, &sc); err != nil {
				t.Fatal(err)
			}
			rs, err := sc.Resolve(nil)
			if err != nil {
				t.Fatal(err)
			}
			var inst any
			b, _ := json.Marshal(res.StructuredContent)
			_ = json.Unmarshal(b, &inst)
			if res.IsError || res.StructuredContent == nil || rs.Validate(inst) != nil {
				t.Fatalf("%s %s: reply %s does not fit the output schema: %v", flow, tool, b, rs.Validate(inst))
			}
		}
		fitsSchema("task_info", map[string]any{})
		fitsSchema("task_info", map[string]any{"repo": "app", "branch": "feature/x"})
		fitsSchema("list_tasks", map[string]any{})
		fitsSchema("task_chain", map[string]any{"taskId": "c-billing"})
		if res := call("finish_step", map[string]any{"status": "done", "summary": "x"}); res.IsError || len(r.finishes) != 1 || r.finishes[0].Status != "done" {
			t.Fatalf("finish_step: %+v %v", res, r.finishes)
		}
		if !call("acquire_lease", map[string]any{}).IsError {
			t.Fatal("unstubbed tool answered")
		}
		_ = sess.Close()
		done()
	}
}

// benchBoardR is the real task board with its finish flushes recorded: the proof oracles need what Kira answered.
type benchBoardR struct {
	*ade.TaskBoard
	mu      sync.Mutex
	replies []string
}

func (b *benchBoardR) SubmitForFinish(ctx context.Context, call agentmcp.SpaceCall, status string, ok bool) (agentmcp.SubmitFinish, error) {
	out, err := b.TaskBoard.SubmitForFinish(ctx, call, status, ok)
	b.mu.Lock()
	b.replies = append(b.replies, out.Reply)
	b.mu.Unlock()
	return out, err
}

// reset forgets what earlier passes saw, so a pass's oracle reads that pass's own flushes.
func (b *benchBoardR) reset() {
	b.mu.Lock()
	b.replies = nil
	b.mu.Unlock()
}

// seen reports whether a flush answered for the proof unit with the wanted line.
func (b *benchBoardR) seen(want string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, rp := range b.replies {
		if strings.Contains(rp, want) {
			return true
		}
	}
	return false
}

// benchSetupRProof runs F2 and F3 on a real app: a real task folder, the real finish flush with the proof
// unit, and the board as the grant's tools. F3 starts from an accepted proof.
func benchSetupRProof(t *testing.T, r *spikeRun, fl spikeFlow) (string, []string, func()) {
	f := newAdeFixture(t, nil)
	f.saveWorkflow(t, "flow", manualStepYAML)
	task := f.createTask(t, "flow")
	folder, err := f.W.AdeBoard.TaskFolder(task.ID)
	if err != nil || folder == "" {
		t.Fatalf("TaskFolder: %q %v", folder, err)
	}
	r.taskDir = folder
	board := &benchBoardR{TaskBoard: f.W.AdeBoard}
	want := "accepted, new proof"
	if fl.id == "F3" || fl.id == "F3b" {
		rows := 0 // F3: the fixture's 20 rows
		if fl.id == "F3b" {
			rows = f3bRowCount
		}
		seedProofR(t, filepath.Join(folder, "proofs", "orders-match-ledger"), rows)
		var rels []string
		_ = filepath.WalkDir(filepath.Join(folder, "proofs"), func(p string, d fs.DirEntry, err error) error {
			if rel, rerr := filepath.Rel(folder, p); err == nil && rerr == nil && !d.IsDir() {
				rels = append(rels, filepath.ToSlash(rel))
			}
			return nil
		})
		out, err := f.W.AdeBoard.SyncTaskFiles(ctx, agentmcp.SpaceCall{TaskID: task.ID, SessionID: "seed"}, rels)
		if err != nil || !strings.Contains(out, "accepted, new proof") {
			t.Fatalf("seed submit: %q %v", out, err)
		}
		want = "accepted, change to" // the task's only proof is its latest: the edit applies
	}
	r.afterPass = board.reset
	r.realProof = func() (*spikeProof, bool, string) { return storedProof(f, task.ID, board, want) }
	srv := agentmcp.NewServer(agentmcp.Options{
		Dir: filepath.Join(r.root, "mcp"), Space: board, Orchestrator: board,
		OnFinish: func(_ string, f agentmcp.Finish) { r.finishes = append(r.finishes, f) },
	})
	claudeProv, ok := providers.New().Get("claude")
	if !ok {
		t.Fatal("no claude provider")
	}
	cfg, release, err := srv.Register(agentmcp.Grant{RunID: "run-1", TaskID: task.ID, SessionID: "sess-1", Space: true, Agent: claudeProv})
	if err != nil {
		t.Fatal(err)
	}
	return cfg, []string{folder}, func() { release(); _ = srv.Close() }
}

// storedProof reads the proof Kira holds for the task: the pending revision when an edit waits, else the
// current one. seen is whether a flush answered for it with the wanted line.
func storedProof(f *adeFixture, taskID string, board *benchBoardR, want string) (*spikeProof, bool, string) {
	list, err := f.W.AdeTask.TaskProofs(ctx, adewire.TaskArgs{TaskID: taskID})
	if err != nil || len(list.Proofs) == 0 {
		return nil, false, "no proof stored"
	}
	d, err := f.W.Proof.Session(bridge.ProofSessionArgs{SessionID: list.Proofs[0].ProofID})
	if err != nil {
		return nil, false, err.Error()
	}
	rev := d.Current
	if d.Pending != nil {
		rev = *d.Pending
	}
	seen := board.seen(want)
	p := &spikeProof{Name: d.Name, Summary: d.Summary, Pending: d.Pending != nil}
	for _, m := range rev.Steps {
		st, err := f.W.Proof.Step(bridge.ProofStepArgs{SessionID: d.ID, Rev: rev.Rev, Ordinal: m.Ordinal})
		if err != nil {
			return nil, false, err.Error()
		}
		ss := spikeStep{View: m.View, Description: m.Description}
		switch m.View {
		case "table":
			ss.Rows, _ = decodeTable(st.Data)
		case "diagram":
			var dg struct {
				Source string `json:"source"`
			}
			_ = json.Unmarshal(st.Data, &dg)
			ss.Text = dg.Source
		default:
			ss.Text = string(st.Data)
		}
		p.Steps = append(p.Steps, ss)
	}
	return p, seen, ""
}

// seedProofR writes the F3 starting proof in the real proof.md format from the F3 fixtures: the table and
// document as data files, the diagram inline.
// rows trims the table to its first rows (0 keeps the fixture's 20).
func seedProofR(t *testing.T, dir string, rows int) {
	t.Helper()
	src := filepath.Join(benchFixtures, "f3", "proof")
	writeFile(t, filepath.Join(dir, "explain.json"), string(mustRead(t, filepath.Join(src, "explain.json"))))
	totals, n := mustRead(t, filepath.Join(src, "totals.json")), 20
	if rows > 0 {
		var tbl struct {
			Columns []any   `json:"columns"`
			Rows    [][]any `json:"rows"`
		}
		if err := json.Unmarshal(totals, &tbl); err != nil {
			t.Fatal(err)
		}
		tbl.Rows, n = tbl.Rows[:rows], rows
		totals, _ = json.MarshalIndent(tbl, "", "  ")
	}
	writeFile(t, filepath.Join(dir, "totals.json"), string(totals))
	var flow strings.Builder
	for _, l := range strings.Split(strings.TrimSpace(string(mustRead(t, filepath.Join(src, "flow.mmd")))), "\n") {
		flow.WriteString("      " + l + "\n")
	}
	writeFile(t, filepath.Join(dir, "proof.md"), "---\nname: Orders match ledger\nsummary: All "+strconv.Itoa(n)+" orders of the day match their ledger lines on id and total.\nsteps:\n"+
		"  - id: totals\n    description: Orders and ledger agree on all "+strconv.Itoa(n)+" rows\n    view: table\n    data: totals.json\n"+
		"  - id: flow\n    description: The matching path the reconcile command follows\n    view: diagram\n    inline: |\n"+flow.String()+
		"  - id: why\n    description: Why the match holds and what a late edit does\n    view: document\n    data: explain.json\n---\n")
}

// TestDetailsBenchArmRServes checks, with no claude and no tokens, that arm R serves the uniform Space
// surface over the task folder and answers the calls its flows make.
func TestDetailsBenchArmRServes(t *testing.T) {
	spike1bOn, spikeFixtures1b = true, benchFixtures
	benchSetup = benchSetupR(t, benchBaseline{})
	defer func() { spike1bOn, spikeFixtures1b, benchSetup = false, "testdata/details-spike-1b", nil }()
	for _, flow := range []string{"F1", "F4", "F5", "F6"} {
		r := &spikeRun{t: t, flow: flow, arm: "R", lib: loadSpikeLib(t)}
		fl := flowByIDs(spikeFlows1b, flow)
		cfg, dirs, done := r.setup(t, fl)
		var conf struct {
			MCPServers map[string]struct {
				URL     string            `json:"url"`
				Headers map[string]string `json:"headers"`
			} `json:"mcpServers"`
		}
		if err := json.Unmarshal(mustRead(t, cfg), &conf); err != nil {
			t.Fatal(err)
		}
		url, hdr := conf.MCPServers["kira-space"].URL, conf.MCPServers["kira-space"].Headers
		tools, err := listTools(t, url, hdr)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, tl := range tools {
			names = append(names, tl.Name)
		}
		slices.Sort(names)
		want := []string{"state"}
		if flow == "F4" || flow == "F6" {
			want = []string{"create_task", "state"}
		}
		if !slices.Equal(names, want) {
			t.Fatalf("%s tools = %v, want %v", flow, names, want)
		}
		for _, f := range []string{"README.md", "task.yaml", "state.yaml"} {
			if _, err := os.Stat(filepath.Join(r.taskDir, f)); err != nil {
				t.Fatalf("%s: %v", flow, err)
			}
		}
		if n := len(dirs); (flow == "F4") != (n == 3) {
			t.Fatalf("%s add-dirs = %v", flow, dirs)
		}
		sess, err := mcp.NewClient(&mcp.Implementation{Name: "check", Version: "1"}, nil).Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: url, HTTPClient: &http.Client{Transport: headerTransport(hdr)}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		call := func(tool string, args map[string]any) string {
			res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
			if err != nil {
				t.Fatalf("%s: %v", tool, err)
			}
			raw, _ := json.Marshal(res.Content)
			if res.IsError {
				t.Fatalf("%s %s: error %s", flow, tool, raw)
			}
			return string(raw)
		}
		switch flow {
		case "F5":
			out := call("state", map[string]any{"action": "branch.request", "args": map[string]any{"repo": "app", "branch": "feature/x", "existing": true}})
			if !strings.Contains(out, r.wt) || time.Now().Before(r.ready) {
				t.Fatalf("branch.request = %s, want the worktree after setup ends", out)
			}
		case "F6":
			call("create_task", map[string]any{"title": "x", "workflow": "default", "brief": "b", "repos": []map[string]any{{"repo": "app"}}})
			if len(r.space.spawned) != 1 {
				t.Fatalf("spawned = %d, want 1", len(r.space.spawned))
			}
		default:
			writeFile(t, filepath.Join(r.taskDir, "handovers", "draft-implement.md"), "done\n")
			if out := call("state", map[string]any{"action": "step.finish", "args": map[string]any{"status": "done"}}); !strings.Contains(out, "done") || len(r.finishes) != 1 {
				t.Fatalf("step.finish = %s, finishes %v", out, r.finishes)
			}
		}
		_ = sess.Close()
		done()
	}
}
