package claude

// P643 benchmark harness (plan section 14.1). Opt-in, spends real tokens, never in hooks or CI:
//
//	KIRA_BENCH=run KIRA_BENCH_OUT=/tmp/bench.md bun run test:claude:details
//
// Knobs: KIRA_BENCH_FLOWS (comma list, default F1), KIRA_BENCH_ARMS (default: every registered arm),
// KIRA_BENCH_REPS (default 3), KIRA_BENCH_BUDGET (USD for the round, default 1.5). KIRA_BENCH_OUT is
// required: results never go to a default path. Arm T serves the frozen tool list in
// testdata/details-bench/tools-baseline.json over stub backends. Arm R, the implementation, registers
// in benchArmSetups when the Part that ships its tools lands; its oracles are the spike flows' own.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/internal/agentmcp"
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
	case "F2", "F3":
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

// benchArmSetups are the registered arms. T serves the frozen tools; R is added by Part 3.
var benchArmSetups = map[string]func(*testing.T, benchBaseline) func(*testing.T, *spikeRun, spikeFlow) (string, []string, func()){
	"T": benchSetupT,
}

func textResult(v any) (*mcp.CallToolResult, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil
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
				res, _ := r.space.ListTasks(ctx, agentmcp.SpaceCall{}, agentmcp.ListTasksArgs{})
				return textResult(res)
			},
			"task_chain": func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				var a agentmcp.TaskChainArgs
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
			fl := flowByIDs(spikeFlows1b, id)
			for _, arm := range arms {
				if spent+benchRunCap > budget {
					note = fmt.Sprintf("Budget guard: spent %.4f USD, next run could add %.2f, budget %.2f. Remaining runs skipped.", spent, benchRunCap, budget)
					break loop
				}
				benchSetup = benchArmSetups[arm](t, base)
				rec := execute(t, home, fl, arm, "sonnet", rep)
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
	fmt.Fprintf(&b, "# P643 benchmark\n\nRuns: %d of %d planned, sonnet. Spend: %.4f USD. %s\n\n", len(recs), planned, spent, note)
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
		b.WriteString("\n### Gate, R over T\n\n| Flow | Cost R/T | Context R/T | Pass R vs T | Verdict |\n|---|---|---|---|---|\n")
		for _, f := range flows {
			t, r := agg[f+"/T"], agg[f+"/R"]
			if t.runs == 0 || r.runs == 0 {
				continue
			}
			cost, ctx := median(r.cost)/median(t.cost), median(r.cf)/median(t.cf)
			ok := cost <= 1.2 && ctx <= 1.0 && r.pass >= t.pass
			verdict := "pass"
			if !ok {
				verdict = "fail"
				if inconclusive(t.cost, r.cost) || inconclusive(t.cf, r.cf) {
					verdict = "fail, inconclusive: Opus review of the raw runs"
				}
			}
			fmt.Fprintf(&b, "| %s | %.2f | %.2f | %d vs %d | %s |\n", f, cost, ctx, r.pass, t.pass, verdict)
		}
	}
	b.WriteString("\n### Runs\n\n| Flow | Arm | Rep | Pass | Cost | Turns | Why |\n|---|---|---|---|---|---|---|\n")
	for _, r := range recs {
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %.4f | %d | %s |\n", r.Flow, r.Arm, r.Rep, yn(r.Pass), r.Cost, r.Turns, strings.ReplaceAll(r.Why, "|", "/"))
	}
	return b.String()
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
		if res := call("task_info", map[string]any{}); res.IsError {
			t.Fatalf("task_info: %+v", res.Content)
		}
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
