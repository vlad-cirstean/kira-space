package claude

// P643 Part 1 spike: does the planned file-based Details surface (arm F) cost about the same as
// today's tools (arm T) for five typical agent flows? Opt-in, spends real tokens:
//
//	KIRA_SPIKE=run go test -count=1 -run TestDetailsSpike -timeout 3h .
//
// Knobs: KIRA_SPIKE_REPS (haiku runs per flow and arm, default 3); flows F1..F6, KIRA_SPIKE_SONNET=0 (skip the
// sonnet round), KIRA_SPIKE_FLOWS (comma list, default F1..F5), KIRA_SPIKE_OUT (results file).
// Gate and thresholds: docs/HISTORY.md (v0.3.14 P643).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/harness/providers"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

const (
	spikeRunCap   = 0.25
	spikeTotalCap = 6.0
	spikeFixtures = "testdata/details-spike"
)

type spikeRec struct {
	Flow, Arm, Model string
	Rep              int
	Pass             bool
	Why              string
	Cost             float64
	Turns            int
	In, CacheRead    int
	CacheWrite, Out  int
	MCPTools         int
	CtxFirst         int     // context tokens (fresh + cache read + cache write) of the first model call
	CtxFinal         int     // same, last model call
	CtxGrowth        float64 // mean growth per further call
	spikeEditStats
	Tools   map[string]int
	Wall    float64
	Aborted bool
}

type spikeRun struct {
	t       *testing.T
	flow    string
	arm     string // "T" or "F"
	root    string // run scratch
	repo    string // claude cwd
	tasks   string // tasks root (arm F)
	taskDir string
	shot    string
	wt      string
	ready   time.Time
	variant string    // Part 1b files arm: FF, FL or FC; empty in Part 1
	lib     *spikeLib // Part 1b fixtures; nil in Part 1

	finishes []agentmcp.Finish
	// pass and finishBase serve multi-pass benchmark flows: the pass number (1 for single-pass flows) and
	// how many finishes earlier passes made.
	pass, finishBase int
	afterPass        func() // runs between passes: promotes the pending edit, resets what the oracle reads
	studio           *studioStub
	space            *spikeSpace
	fa               *filesArm
	// realProof, set by the real-app arm R, reads the proof Kira stored (the pending edit when there is one).
	realProof func() (*spikeProof, bool, string)
}

type spikeFlow struct {
	id, name string
	prompt   func(r *spikeRun) string
	// seedT returns the proof the studio stub starts with; fixtures for arm F come from testdata.
	seedT  func(r *spikeRun) *spikeProof
	oracle func(r *spikeRun, result string) (bool, string)
}

var orderRows = [][]any{{1.0, 400.0}, {2.0, 450.0}, {3.0, 400.0}}

func rowsEqual(got, want [][]any) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			return false
		}
		for j := range want[i] {
			g, _ := strconv.ParseFloat(fmt.Sprint(got[i][j]), 64)
			w, _ := strconv.ParseFloat(fmt.Sprint(want[i][j]), 64)
			if g != w {
				return false
			}
		}
	}
	return true
}

func (r *spikeRun) folderLine() string {
	if r.arm == "R" { // the files fragment Kira appends to a step prompt
		v := llmprompts.FilesVars{Folder: r.taskDir}
		if p, ok := providers.New().Get("claude"); ok {
			if l, ok := p.(harness.ToolLoader); ok {
				tools := []harness.MCPTool{agentmcp.Submit, agentmcp.State}
				if r.flow == "F4" || r.flow == "F6" {
					tools = append(tools, agentmcp.CreateTask)
				}
				v.Load = l.LoadHint(tools...)
			}
		}
		return "\n\n" + llmprompts.HeadlessSpaceFiles.Text(llmprompts.Defaults(), v)
	}
	if r.arm != "F" {
		return ""
	}
	if (r.variant == "FA" || r.variant == "FP") && r.flow == "F3" { // Part 1c A: Kira inlines the proof path
		return "\n\nTask folder: " + r.taskDir + " (README.md there). Proof file: proofs/orders-match-ledger/proof.md."
	}
	return "\n\nTask folder: " + r.taskDir + " (README.md there)."
}

// finalProof reads the proof an arm-F run left in the task folder and whether a submit saw it.
func (r *spikeRun) finalProof() (*spikeProof, bool, string) {
	if r.realProof != nil {
		return r.realProof()
	}
	ents, _ := os.ReadDir(filepath.Join(r.taskDir, "proofs"))
	for _, e := range ents {
		dir := filepath.Join(r.taskDir, "proofs", e.Name())
		p, _, err := readProof(dir)
		if err != nil {
			return nil, false, err.Error()
		}
		rel := filepath.Join("proofs", e.Name(), "proof.md")
		r.fa.mu.Lock()
		seen := r.fa.seen[rel] == fileSHA(filepath.Join(dir, "proof.md"))
		r.fa.mu.Unlock()
		return p, seen, ""
	}
	return nil, false, "no proof folder"
}

func hasImage(p *spikeProof) bool {
	return slices.ContainsFunc(p.Steps, func(s spikeStep) bool { return s.View == "image" && s.ImageOK })
}

func tableRows(p *spikeProof) [][]any {
	for _, s := range p.Steps {
		if s.View == "table" {
			return s.Rows
		}
	}
	return nil
}

var spikeFlows = []spikeFlow{
	{
		id: "F1", name: "handover",
		prompt: func(r *spikeRun) string {
			return "You are the implement stage of a Kira Space task. Change greeting.txt in the working directory so it contains exactly: hello, world\n" +
				"Then hand over to the review stage (what you changed, what review should check) and finish the step." + r.folderLine()
		},
		oracle: func(r *spikeRun, _ string) (bool, string) {
			b, _ := os.ReadFile(filepath.Join(r.repo, "greeting.txt"))
			if strings.TrimSpace(string(b)) != "hello, world" {
				return false, "greeting.txt not changed"
			}
			if r.arm == "T" {
				if len(r.finishes) == 0 || r.finishes[len(r.finishes)-1].Status != "done" {
					return false, "finish_step done not called"
				}
				if !strings.Contains(strings.ToLower(r.finishes[len(r.finishes)-1].Summary), "greeting") {
					return false, "handover summary does not name greeting.txt"
				}
				return true, ""
			}
			draft := filepath.Join(r.taskDir, "handovers", "draft-implement.md")
			body, _ := os.ReadFile(draft)
			if !strings.Contains(strings.ToLower(string(body)), "greeting") {
				return false, "draft missing or does not name greeting.txt"
			}
			if len(r.fa.finish) == 0 || r.fa.finish[len(r.fa.finish)-1] != "done" {
				return false, "finish_step done not accepted"
			}
			return true, ""
		},
	},
	{
		id: "F2", name: "new proof",
		prompt: func(r *spikeRun) string {
			return `You are in a Kira Space task. Save a validation proof named "Orders match ledger" with a one-sentence summary and two steps: ` +
				`(1) a table of the three orders, columns id and total: 1 and 400, 2 and 450, 3 and 400, described "Orders and ledger agree: 3 rows, total 1250"; ` +
				`(2) the screenshot at ` + r.shot + ` as an image step, described "Ledger screenshot". Then reply: done.` + r.folderLine()
		},
		oracle: func(r *spikeRun, _ string) (bool, string) {
			var p *spikeProof
			if r.arm == "T" {
				p = r.studio.proofs["orders match ledger"]
				if p == nil {
					return false, "no proof saved"
				}
			} else {
				var seen bool
				var why string
				if p, seen, why = r.finalProof(); p == nil {
					return false, why
				}
				if !seen {
					return false, "proof never submitted"
				}
				if !strings.EqualFold(p.Name, "Orders match ledger") {
					return false, "wrong proof name"
				}
			}
			if strings.TrimSpace(p.Summary) == "" {
				return false, "no summary"
			}
			if !rowsEqual(tableRows(p), orderRows) {
				return false, "table rows wrong"
			}
			if !hasImage(p) {
				return false, "no valid image step"
			}
			return true, ""
		},
	},
	{
		id: "F3", name: "proof edit",
		seedT: func(r *spikeRun) *spikeProof {
			return &spikeProof{Name: "Orders match ledger", Summary: "Orders and ledger totals agree.", Steps: []spikeStep{
				{Description: "Orders and ledger agree: 3 rows, total 1250", View: "table", Rows: orderRows},
				{Description: "Ledger screenshot", View: "image", ImageOK: true},
			}}
		},
		prompt: func(r *spikeRun) string {
			return `The closed validation proof "Orders match ledger" is wrong: order 2's total is 475, not 450. ` +
				`The table must then show: 1 and 400, 2 and 475, 3 and 400. Change the table step, keep the screenshot step as it is, and give the reason "order 2 corrected". Then reply: done.` + r.folderLine()
		},
		oracle: func(r *spikeRun, _ string) (bool, string) {
			want := [][]any{{1.0, 400.0}, {2.0, 475.0}, {3.0, 400.0}}
			var p *spikeProof
			if r.arm == "T" {
				if p = r.studio.edits["orders match ledger"]; p == nil {
					return false, "no pending edit"
				}
			} else {
				var seen bool
				var why string
				if p, seen, why = r.finalProof(); p == nil {
					return false, why
				}
				if !seen {
					return false, "edit never submitted"
				}
			}
			if strings.TrimSpace(p.Reason) == "" {
				return false, "no reason"
			}
			if !rowsEqual(tableRows(p), want) {
				return false, "table rows wrong"
			}
			if !hasImage(p) {
				return false, "image step lost"
			}
			return true, ""
		},
	},
	{
		id: "F4", name: "orchestrator summary",
		prompt: func(r *spikeRun) string {
			return "You orchestrate two child tasks. From each child's latest handover, say where it stands: reply in two sentences, one per child, naming the child by its title." + r.folderLine()
		},
		oracle: func(_ *spikeRun, result string) (bool, string) {
			l := strings.ToLower(result)
			for _, w := range []string{"billing", "settings", "14", "audit"} {
				if !strings.Contains(l, w) {
					return false, "summary lacks " + w
				}
			}
			return true, ""
		},
	},
	{
		id: "F6", name: "create 2 child tasks",
		prompt: func(r *spikeRun) string {
			return "You orchestrate this work. Create two child tasks, each with workflow default and repo app. " +
				"Child 1, title \"Billing API v2\", its brief is exactly:\n\n" + briefBilling +
				"\n\nChild 2, title \"Settings UI refresh\", its brief is exactly:\n\n" + briefSettings +
				"\n\nThen reply with the two child ids." + r.folderLine()
		},
		oracle: func(r *spikeRun, _ string) (bool, string) {
			type kid struct {
				title, workflow, brief string
				repos                  []string
			}
			var kids []kid
			if r.fa == nil { // arms T and R record the calls on the stub space
				for _, a := range r.space.spawned {
					k := kid{a.Title, a.Workflow, a.Brief, nil}
					for _, rp := range a.Repos {
						k.repos = append(k.repos, rp.Repo)
					}
					kids = append(kids, k)
				}
			} else {
				for _, c := range r.fa.kids {
					kids = append(kids, kid{c.Title, c.Workflow, c.Brief, c.Repos})
				}
			}
			if len(kids) != 2 {
				return false, fmt.Sprintf("%d children created, want 2", len(kids))
			}
			want := map[string][2]string{"Billing API v2": {"REQ-B1", "REQ-B9"}, "Settings UI refresh": {"REQ-S1", "REQ-S9"}}
			for _, k := range kids {
				m, ok := want[k.title]
				if !ok {
					return false, "unexpected child title " + k.title
				}
				delete(want, k.title)
				if !strings.EqualFold(k.workflow, "default") || !slices.Contains(k.repos, "app") {
					return false, k.title + ": workflow or repo wrong"
				}
				if len(k.brief) < 1200 || !strings.Contains(k.brief, m[0]) || !strings.Contains(k.brief, m[1]) {
					return false, k.title + ": brief cut or altered"
				}
			}
			return true, ""
		},
	},
	{
		id: "F5", name: "branch wait",
		prompt: func(r *spikeRun) string {
			return "Branch feature/x of repo app in this task is being set up. Wait until its setup is done, then reply with the branch's worktree path and nothing else." + r.folderLine()
		},
		oracle: func(r *spikeRun, result string) (bool, string) {
			if !strings.Contains(result, r.wt) {
				return false, "result lacks worktree path"
			}
			if time.Now().Before(r.ready) {
				return false, "answered before setup ended"
			}
			return true, ""
		},
	},
}

const (
	briefBilling = `REQ-B1 Goal: move the billing endpoints to the v2 API without changing behavior for existing clients.

Scope: the 14 routes under /billing (invoices, refunds, proration, payment methods). Keep the v1 routes alive behind a deprecation header until the end of the quarter. Do not touch the ledger export job; another task owns it.

Constraints: every write endpoint takes an idempotency key and rejects a repeat with the stored response. Money is integer cents in every payload; reject floats at the edge. Errors use the shared problem+json shape from the platform package, with stable error codes the mobile apps already match on.

Testing: port the existing contract tests first and keep them green on both versions. Add a replay test per write endpoint. test_refund and test_proration are known to be flaky because they read the wall clock; inject a clock instead of sleeping.

Rollout: ship behind the billing-v2 flag, on for staff accounts first, then ten percent of traffic for two days, then everyone. Watch the refund error rate and p95 latency on the dashboard the platform team owns. Roll back by turning the flag off; no data migration is involved, so rollback needs no coordination.

Hand-off: finish with a handover that lists each migrated route, each deliberate behavior difference, and the open risks. Review will run the whole suite three times, so say what to watch for. REQ-B9 Done means all 14 routes serve v2 and v1 only redirects.`
	briefSettings = `REQ-S1 Goal: refresh the settings screen so it uses the new navigation component and passes the accessibility audit.

Scope: the settings route and its five sub-pages (profile, notifications, security, billing, integrations). Replace the old tab bar with the shared NavRail component; do not change the page contents. The billing page links into the billing API task's work, so leave its data calls as they are.

Constraints: keyboard focus must move through the rail in reading order and return to the rail after a sub-page loads. Every icon-only control gets an accessible name from the existing i18n catalog; add keys, never literals. Respect the reduced-motion setting for the rail's slide animation.

Testing: extend the component tests for the rail, add a keyboard-only end-to-end pass through all five sub-pages, and run the axe audit on each page. Record any violation you cannot fix with the reason.

Rollout: ship behind the nav-rail flag, on for internal users first, then a quarter of users for a week, then everyone. Compare the settings abandonment rate before and after, and collect the screen-reader feedback thread. Turning the flag off restores the old tab bar, so rollback needs no coordination.

Hand-off: finish with a handover that names the pages done, the audit result per page and anything left open. Review focuses on focus order. REQ-S9 Done means all five pages use NavRail and the audit reports zero serious violations.`
)

func copyTree(t *testing.T, src, dst string, repl map[string]string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for k, v := range repl {
			b = bytes.ReplaceAll(b, []byte(k), []byte(v))
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func writeShot(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 40))
	for x := 0; x < 64; x++ {
		for y := 0; y < 40; y++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 6), 120, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setup builds the run's directories and its arm's servers; the returned func tears them down and
// the string is the --mcp-config file.
func (r *spikeRun) setup(t *testing.T, fl spikeFlow) (mcpConfig string, addDirs []string, done func()) {
	r.root = t.TempDir()
	if res, err := filepath.EvalSymlinks(r.root); err == nil {
		r.root = res
	}
	r.repo = filepath.Join(r.root, "proj")
	r.tasks = filepath.Join(r.root, "tasks")
	r.taskDir = filepath.Join(r.tasks, "t-1")
	r.wt = filepath.Join(r.root, "wt", "app-feature-x")
	r.shot = filepath.Join(r.repo, "shot.jpg")
	r.ready = time.Now().Add(4 * time.Second)
	for _, d := range []string{r.repo, r.taskDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(r.repo, "greeting.txt"), "hello\n")
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "."}, {"-c", "user.name=T", "-c", "user.email=t@example.com", "commit", "-q", "-m", "base"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = r.repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	writeShot(t, r.shot)

	if r.arm == "F" && r.lib != nil {
		return r.setupFiles1b(t, fl)
	}
	if r.arm == "F" {
		repl := map[string]string{"__WT__": r.wt}
		copyTree(t, filepath.Join(spikeFixtures, "task"), r.taskDir, repl)
		for _, d := range []string{"notes", "handovers"} {
			_ = os.MkdirAll(filepath.Join(r.taskDir, d), 0o755)
		}
		flowDir := filepath.Join(spikeFixtures, "flows", fl.id)
		if ents, err := os.ReadDir(flowDir); err == nil {
			for _, e := range ents {
				copyTree(t, filepath.Join(flowDir, e.Name()), filepath.Join(r.tasks, e.Name()), repl)
			}
		}
		if fl.id == "F3" {
			if err := os.WriteFile(filepath.Join(r.taskDir, "proofs", "orders-match-ledger", "shot.jpg"), mustRead(t, r.shot), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		r.fa = newFilesArm(r.taskDir, "implement", r.wt, r.ready, fl.id == "F4")
		cfg := filepath.Join(r.root, "mcp.json")
		raw, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"kira-space": map[string]any{"type": "http", "url": r.fa.srv.URL}}})
		if err := os.WriteFile(cfg, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		addDirs = []string{r.taskDir}
		if fl.id == "F4" {
			addDirs = append(addDirs, filepath.Join(r.tasks, "c-billing"), filepath.Join(r.tasks, "c-settings"))
		}
		return cfg, addDirs, r.fa.close
	}

	// Arm T: the real agentmcp.Server over stub backends, and the kira-studio proof stub.
	var seed *spikeProof
	if fl.seedT != nil {
		seed = fl.seedT(r)
	}
	r.studio = newStudioStub(seed)
	space := &spikeSpace{br: spikeBranch{Repo: "app", Branch: "feature/x", Worktree: r.wt, ReadyAt: r.ready}}
	if fl.id == "F4" && r.lib != nil {
		space.children, space.chain = r.lib.children(), r.lib.chains()
	} else if fl.id == "F4" {
		space.children = []frozenChildTaskInfo{
			{ID: "c-billing", Title: "Billing API", Kind: "task", Workflow: "default", Status: "running", Step: "review", CreatedAt: "2026-11-01T09:00:00Z",
				LastHandover: "Migrated 14 endpoints to v2. Two tests still flaky: test_refund, test_proration."},
			{ID: "c-settings", Title: "Settings UI", Kind: "task", Workflow: "default", Status: "running", Step: "review", CreatedAt: "2026-11-01T09:05:00Z",
				LastHandover: "Replaced the nav bar with the new component. Accessibility audit still open."},
		}
		space.chain = map[string][]frozenChainRow{
			"c-billing": {
				{ID: "h2", From: "implement", To: "review", Source: "agent", State: "handed", CreatedAt: "2026-11-02T10:00:00Z",
					Body: "Migrated 14 endpoints to v2. Two tests still flaky: test_refund, test_proration. Review should run the suite three times."},
				{ID: "h1", From: "plan", To: "implement", Source: "agent", State: "handed", CreatedAt: "2026-11-01T10:00:00Z", Body: "Plan: migrate the billing endpoints to v2 one router at a time."},
			},
			"c-settings": {
				{ID: "h4", From: "implement", To: "review", Source: "agent", State: "handed", CreatedAt: "2026-11-02T10:05:00Z",
					Body: "Replaced the nav bar with the new component. Accessibility audit still open: keyboard focus order unchecked."},
				{ID: "h3", From: "plan", To: "implement", Source: "agent", State: "handed", CreatedAt: "2026-11-01T10:05:00Z", Body: "Plan: swap the settings nav bar, then audit accessibility."},
			},
		}
	}
	r.space = space
	if benchSetup != nil {
		return benchSetup(t, r, fl)
	}
	srv := agentmcp.NewServer(agentmcp.Options{
		Dir: filepath.Join(r.root, "mcp"), Space: space, Orchestrator: space,
		OnFinish: func(_ string, f agentmcp.Finish) { r.finishes = append(r.finishes, f) },
	})
	claudeProv, ok := providers.New().Get("claude")
	if !ok {
		t.Fatal("no claude provider")
	}
	grant := agentmcp.Grant{RunID: "run-1", TaskID: "t-1", Space: true, Orchestrator: fl.id == "F4" || fl.id == "F6", Agent: claudeProv}
	if fl.id == "F2" || fl.id == "F3" { // the proof tools exist only where the flow is about proofs
		grant.Extra = []harness.MCPServer{{Name: "kira-studio", URL: r.studio.srv.URL, Headers: map[string]string{"X-Spike": "1"}}}
	}
	cfg, release, err := srv.Register(grant)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, nil, func() { release(); _ = srv.Close(); r.studio.close() }
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type spikeResultLine struct {
	Type         string  `json:"type"`
	Subtype      string  `json:"subtype"`
	IsError      bool    `json:"is_error"`
	Result       string  `json:"result"`
	NumTurns     int     `json:"num_turns"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	Usage        struct {
		In         int `json:"input_tokens"`
		Out        int `json:"output_tokens"`
		CacheRead  int `json:"cache_read_input_tokens"`
		CacheWrite int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
	Tools []string `json:"tools"`
}

// execute runs one real claude -p and scores it.
func execute(t *testing.T, home string, fl spikeFlow, arm, model string, rep int) spikeRec {
	t.Helper()
	label := arm
	r := &spikeRun{t: t, flow: fl.id, arm: arm}
	if spike1bOn { // Part 1b: realistic fixtures; files arms are the variants FF, FL, FC
		r.lib = loadSpikeLib(t)
		if arm != "T" && benchSetup == nil {
			r.arm, r.variant = "F", arm
		}
	}
	arm = label
	cfg, addDirs, done := r.setup(t, fl)
	defer done()
	return runClaude(t, home, r, fl, cfg, addDirs, arm, model, rep, "")
}

// runClaude runs one claude -p over an already built run and scores it; tag names the dump file of a
// later pass of a multi-pass flow.
func runClaude(t *testing.T, home string, r *spikeRun, fl spikeFlow, cfg string, addDirs []string, arm, model string, rep int, tag string) spikeRec {
	t.Helper()
	args := []string{"-p", fl.prompt(r), "--model", model, "--max-budget-usd", fmt.Sprint(max(spikeRunCap, benchRunCapUSD)),
		"--output-format", "stream-json", "--verbose", "--strict-mcp-config", "--mcp-config", cfg,
		"--allowedTools", "mcp__kira-space,mcp__kira-studio,Read,Edit,Write,Glob,Grep,Bash"}
	for _, d := range addDirs {
		args = append(args, "--add-dir", d)
	}
	cmd := exec.Command("claude", args...)
	cmd.Dir = r.repo
	env := []string{"HOME=" + home}
	for _, e := range os.Environ() {
		if k, _, _ := strings.Cut(e, "="); k == "HOME" || k == "CLAUDE_CODE_SESSION_ID" || k == "CLAUDE_CODE_REMOTE_SESSION_ID" || k == "CLAUDECODE" {
			continue
		}
		env = append(env, e)
	}
	cmd.Env = env
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	start := time.Now()
	timer := time.AfterFunc(6*time.Minute, func() { _ = cmd.Process.Kill() })
	runErr := cmd.Run()
	timer.Stop()
	if d := os.Getenv("KIRA_SPIKE_DUMP"); d != "" {
		_ = os.MkdirAll(d, 0o755)
		_ = os.WriteFile(filepath.Join(d, fmt.Sprintf("%s-%s-%s-%d%s.jsonl", fl.id, arm, model, rep, tag)), []byte(so.String()+"\n"+se.String()), 0o644)
	}
	rec := spikeRec{Flow: fl.id, Arm: arm, Model: model, Rep: rep, Wall: time.Since(start).Seconds(), Tools: map[string]int{}}

	res := parseSpikeStream(so.String(), &rec)
	switch {
	case res.Type != "result":
		rec.Why = fmt.Sprintf("no result line: %v %s", runErr, tail(se.String(), 200))
	case res.IsError:
		rec.Why = "claude error: " + res.Subtype + " " + tail(res.Result, 160)
	default:
		rec.Pass, rec.Why = fl.oracle(r, res.Result)
	}
	return rec
}

func TestDetailsSpike(t *testing.T) {
	if os.Getenv("KIRA_SPIKE") != "run" {
		t.Skip("P643 spike spends up to 6 USD; run with KIRA_SPIKE=run (see the file comment)")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("no claude on PATH")
	}
	reps := 3
	if v, err := strconv.Atoi(os.Getenv("KIRA_SPIKE_REPS")); err == nil && v > 0 {
		reps = v
	}
	wanted := map[string]bool{}
	for _, f := range strings.Split(os.Getenv("KIRA_SPIKE_FLOWS"), ",") {
		if f = strings.TrimSpace(f); f != "" {
			wanted[f] = true
		}
	}
	out := os.Getenv("KIRA_SPIKE_OUT")
	if out == "" {
		out = filepath.Join(t.TempDir(), "P643-spike-results.md") // set KIRA_SPIKE_OUT to keep it
	}

	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".claude.json"), `{"hasCompletedOnboarding":true,"theme":"dark"}`)

	type job struct {
		fl         spikeFlow
		arm, model string
		rep        int
	}
	var jobs []job
	for rep := 1; rep <= reps; rep++ {
		for _, fl := range spikeFlows {
			if len(wanted) > 0 && !wanted[fl.id] {
				continue
			}
			for _, arm := range []string{"T", "F"} {
				jobs = append(jobs, job{fl, arm, "haiku", rep})
			}
		}
	}
	if os.Getenv("KIRA_SPIKE_SONNET") != "0" {
		for _, fl := range spikeFlows {
			if len(wanted) > 0 && !wanted[fl.id] {
				continue
			}
			for _, arm := range []string{"T", "F"} {
				jobs = append(jobs, job{fl, arm, "sonnet", 1})
			}
		}
	}

	var recs []spikeRec
	var spent float64
	aborted := ""
	for _, j := range jobs {
		if spent+spikeRunCap > spikeTotalCap {
			aborted = fmt.Sprintf("budget guard: spent %.4f USD, next run could add %.2f, total cap %.2f. Remaining runs skipped.", spent, spikeRunCap, spikeTotalCap)
			break
		}
		rec := execute(t, home, j.fl, j.arm, j.model, j.rep)
		spent += rec.Cost
		recs = append(recs, rec)
		t.Logf("%s %s %s #%d pass=%v cost=%.4f turns=%d %s", rec.Flow, rec.Arm, rec.Model, rec.Rep, rec.Pass, rec.Cost, rec.Turns, rec.Why)
		if err := os.WriteFile(out, []byte(spikeReport(recs, spent, aborted, len(jobs))), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(out, []byte(spikeReport(recs, spent, aborted, len(jobs))), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("spike spend %.4f USD over %d runs; results in %s", spent, len(recs), out)
}
