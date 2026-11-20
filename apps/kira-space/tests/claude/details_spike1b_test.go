package claude

// P643 Part 1b: Part 1 repeated on sonnet with realistic fixtures and three files-arm variants.
//
//	FF  README.md + ../AGENTS.md as the planned generator writes them (full)
//	FL  the same facts cut to the minimum (lean)
//	FC  FL with one handovers.md and one notes.md instead of one file each (consolidated)
//
// Opt-in, spends real tokens (see the budget knobs):
//
//	KIRA_SPIKE=run1b go test -count=1 -run TestDetailsSpike1b -timeout 4h .
//	KIRA_SPIKE=run1b go test -count=1 -run TestDetailsSpikeContext1b -timeout 1h .
//
// Output files are required, never defaulted: KIRA_SPIKE1B_OUT (runs and report), KIRA_SPIKE1B_PROBE_OUT
// (context probes), KIRA_SPIKE1B_TOOLS_OUT (tool bytes); a test skips without its variable.
// Knobs: KIRA_SPIKE1B_REPS (default 3), KIRA_SPIKE1B_FLOWS, KIRA_SPIKE1B_ARMS (default T,FF,FL,FC),
// KIRA_SPIKE1B_NOFC (flows that skip FC), KIRA_SPIKE1B_BUDGET (USD, default 4),
// KIRA_SPIKE_DUMP (raw transcripts). Results: docs/HISTORY.md (v0.3.14 P643).

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const spikeRunCap1b = 0.5

// spikeFixtures1b is a var so the benchmark (details_bench_test.go) points the loaders at its own copy.
var spikeFixtures1b = "testdata/details-spike-1b"

// spike1bOn switches execute to the Part 1b fixtures and variants.
var spike1bOn bool

type libHandover struct{ name, from, to, body string }

type spikeLib struct {
	t1       []libHandover
	notes    []string // t-1 notes, file order
	noteName []string
	kids     map[string][]libHandover
	rows     [][]any
	flow     string
	explain  string
}

func loadHandovers(t *testing.T, dir string) []libHandover {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []libHandover
	for _, e := range ents {
		stem := strings.TrimSuffix(e.Name(), ".md")
		_, rest, _ := strings.Cut(stem, "-")
		from, to, _ := strings.Cut(rest, "-to-")
		out = append(out, libHandover{e.Name(), from, to, string(mustRead(t, filepath.Join(dir, e.Name())))})
	}
	return out
}

func loadSpikeLib(t *testing.T) *spikeLib {
	t.Helper()
	fx := spikeFixtures1b
	l := &spikeLib{kids: map[string][]libHandover{}}
	l.t1 = loadHandovers(t, filepath.Join(fx, "t1", "handovers"))
	ents, _ := os.ReadDir(filepath.Join(fx, "t1", "notes"))
	for _, e := range ents {
		l.noteName = append(l.noteName, e.Name())
		l.notes = append(l.notes, strings.TrimSpace(string(mustRead(t, filepath.Join(fx, "t1", "notes", e.Name())))))
	}
	for _, c := range []string{"c-billing", "c-settings"} {
		l.kids[c] = loadHandovers(t, filepath.Join(fx, "children", c, "handovers"))
	}
	var tbl struct {
		Rows [][]any `json:"rows"`
	}
	if err := json.Unmarshal(mustRead(t, filepath.Join(fx, "f3", "proof", "totals.json")), &tbl); err != nil {
		t.Fatal(err)
	}
	l.rows = tbl.Rows
	l.flow = string(mustRead(t, filepath.Join(fx, "f3", "proof", "flow.mmd")))
	var ex struct {
		Documents []struct {
			Body struct {
				Text string `json:"text"`
			} `json:"body"`
		} `json:"documents"`
	}
	if err := json.Unmarshal(mustRead(t, filepath.Join(fx, "f3", "proof", "explain.json")), &ex); err != nil {
		t.Fatal(err)
	}
	l.explain = ex.Documents[0].Body.Text
	return l
}

// capBytes500 mirrors the 500-byte list_tasks preview (ade.capBytes).
func capBytes500(s string) string {
	if len(s) <= 500 {
		return s
	}
	cut := s[:500]
	for !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}

func (l *spikeLib) children() []frozenChildTaskInfo {
	mk := func(id, title string) frozenChildTaskInfo {
		hs := l.kids[id]
		return frozenChildTaskInfo{ID: id, Title: title, Kind: "task", Workflow: "default", Status: "running", Step: "review", CreatedAt: "2026-11-01T09:00:00Z",
			LastHandover: capBytes500(hs[len(hs)-1].body)}
	}
	return []frozenChildTaskInfo{mk("c-billing", "Billing API"), mk("c-settings", "Settings UI")}
}

// chains lists each child's handovers newest first, like task_chain.
func (l *spikeLib) chains() map[string][]frozenChainRow {
	out := map[string][]frozenChainRow{}
	for id, hs := range l.kids {
		for i := len(hs) - 1; i >= 0; i-- {
			h := hs[i]
			out[id] = append(out[id], frozenChainRow{ID: fmt.Sprintf("h-%s-%d", id, i+1), From: h.from, To: h.to, Source: "agent", State: "handed",
				CreatedAt: fmt.Sprintf("2026-11-0%dT10:00:00Z", i+1), Body: h.body})
		}
	}
	return out
}

func (l *spikeLib) latest() libHandover { return l.t1[len(l.t1)-1] }

// inline is what a step prompt carries today (Q13): the incoming handover and the task's notes.
func (r *spikeRun) inline() string {
	if r.arm == "R" { // the files arm reads the handover and notes from the task folder
		return ""
	}
	h := r.lib.latest()
	var b strings.Builder
	fmt.Fprintf(&b, "Incoming handover (from %s):\n---\n%s---\n\nUser notes on this task:\n", h.from, h.body)
	for _, n := range r.lib.notes {
		b.WriteString("- " + n + "\n")
	}
	return b.String()
}

// ---- setup of the files arms ----

var chRe = regexp.MustCompile(`(?s)\[\[CH:(.*?)\]\]`)

func renderDoc(s string, orch bool) string {
	if orch {
		return chRe.ReplaceAllString(s, "$1")
	}
	return chRe.ReplaceAllString(s, "")
}

// seedHistory copies a folder's handovers and notes in the per-file layout, or folds them into
// handovers.md and notes.md; it returns the handed text of handovers.md in the folded layout.
func seedHistory(t *testing.T, src, dst string, cons bool) string {
	t.Helper()
	if !cons {
		copyTree(t, filepath.Join(src, "handovers"), filepath.Join(dst, "handovers"), nil)
		if _, err := os.Stat(filepath.Join(src, "notes")); err == nil {
			copyTree(t, filepath.Join(src, "notes"), filepath.Join(dst, "notes"), nil)
		} else {
			_ = os.MkdirAll(filepath.Join(dst, "notes"), 0o755)
		}
		return ""
	}
	var h strings.Builder
	for _, x := range loadHandovers(t, filepath.Join(src, "handovers")) {
		num, _, _ := strings.Cut(x.name, "-")
		fmt.Fprintf(&h, "## %s %s to %s\n\n", num, x.from, x.to)
		_, body, _ := strings.Cut(x.body, "\n")
		for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
			if strings.HasPrefix(line, "## ") {
				line = "#" + line
			}
			h.WriteString(line + "\n")
		}
		h.WriteString("\n")
	}
	writeFile(t, filepath.Join(dst, "handovers.md"), h.String())
	var n strings.Builder
	if ents, err := os.ReadDir(filepath.Join(src, "notes")); err == nil {
		for _, e := range ents {
			fmt.Fprintf(&n, "## %s\n\n%s\n", strings.TrimSuffix(e.Name(), ".md"), strings.TrimSpace(string(mustRead(t, filepath.Join(src, "notes", e.Name())))))
		}
	}
	writeFile(t, filepath.Join(dst, "notes.md"), n.String())
	return h.String()
}

func (r *spikeRun) setupFiles1b(t *testing.T, fl spikeFlow) (string, []string, func()) {
	fx := spikeFixtures1b
	orch := fl.id == "F4" || fl.id == "F6"
	cons := r.variant == "FC"
	state := map[string]string{"F4": "F4", "F5": "F5", "F6": "F6"}[fl.id]
	if state == "" {
		state = "default"
	}
	writeFile(t, filepath.Join(r.taskDir, "task.yaml"), string(mustRead(t, filepath.Join(fx, "t1", "task.yaml"))))
	writeFile(t, filepath.Join(r.taskDir, "state.yaml"), strings.ReplaceAll(string(mustRead(t, filepath.Join(fx, "states", state+".yaml"))), "__WT__", r.wt))
	vd := filepath.Join(fx, "variants", r.variant)
	proofIdx := "none yet."
	if fl.id == "F3" {
		proofIdx = `orders-match-ledger, "Orders match ledger", 3 steps: proofs/orders-match-ledger/proof.md`
	}
	writeFile(t, filepath.Join(r.taskDir, "README.md"), strings.ReplaceAll(renderDoc(string(mustRead(t, filepath.Join(vd, "README.md"))), orch), "__PROOFS__", proofIdx))
	agents := string(mustRead(t, filepath.Join(vd, "AGENTS.md")))
	if orch {
		agents += "\n" + string(mustRead(t, filepath.Join(vd, "AGENTS-children.md")))
	}
	writeFile(t, filepath.Join(r.tasks, "AGENTS.md"), agents)

	var handed string
	switch {
	case fl.id == "F4":
		for _, c := range []string{"c-billing", "c-settings"} {
			seedHistory(t, filepath.Join(fx, "children", c), filepath.Join(r.tasks, c), cons)
		}
	case !orch:
		handed = seedHistory(t, filepath.Join(fx, "t1"), r.taskDir, cons)
	}
	if fl.id == "F3" {
		pd := map[string]string{"FA": "proof-fa", "FP": "proof-fp"}[r.variant]
		if pd == "" {
			pd = "proof"
		}
		copyTree(t, filepath.Join(fx, "f3", pd), filepath.Join(r.taskDir, "proofs", "orders-match-ledger"), nil)
	}
	r.fa = newFilesArm(r.taskDir, "implement", r.wt, r.ready, fl.id == "F4")
	r.fa.cons, r.fa.handed = cons, handed
	cfg := filepath.Join(r.root, "mcp.json")
	raw, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"kira-space": map[string]any{"type": "http", "url": r.fa.srv.URL}}})
	if err := os.WriteFile(cfg, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg, []string{r.tasks}, r.fa.close
}

// ---- flows ----

func flowByID(id string) spikeFlow {
	for _, f := range spikeFlows {
		if f.id == id {
			return f
		}
	}
	panic(id)
}

func (r *spikeRun) draftText() string {
	if r.variant == "FC" {
		return consDraftOf(string(mustReadOrEmpty(filepath.Join(r.taskDir, "handovers.md"))), "implement")
	}
	return string(mustReadOrEmpty(filepath.Join(r.taskDir, "handovers", "draft-implement.md")))
}

func mustReadOrEmpty(p string) []byte {
	b, _ := os.ReadFile(p)
	return b
}

func jsonRows(rows [][]any) string {
	raw, _ := json.Marshal(rows)
	return string(raw)
}

func (r *spikeRun) correctedRows() [][]any {
	out := make([][]any, len(r.lib.rows))
	for i, row := range r.lib.rows {
		out[i] = slices.Clone(row)
		if id, _ := row[0].(float64); id == 12 {
			out[i][1], out[i][2] = 1175.0, 1175.0
		}
	}
	return out
}

// proofChecks verifies the three steps of the Part 1b proof: table rows, diagram text, document text.
func proofChecks(r *spikeRun, p *spikeProof, rows [][]any) (bool, string) {
	if len(p.Steps) != 3 || p.Steps[0].View != "table" || p.Steps[1].View != "diagram" || p.Steps[2].View != "document" {
		return false, "steps are not table, diagram, document"
	}
	if !rowsEqual(p.Steps[0].Rows, rows) {
		return false, "table rows wrong"
	}
	if !strings.Contains(p.Steps[1].Text, "flowchart") || !strings.Contains(p.Steps[1].Text, "Mismatch grouped by cause") {
		return false, "diagram lost or altered"
	}
	if !strings.Contains(p.Steps[2].Text, "matched against their old total") {
		return false, "document lost or altered"
	}
	return true, ""
}

var spikeFlows1b = []spikeFlow{
	{
		id: "F1", name: "handover",
		prompt: func(r *spikeRun) string {
			return "You are the implement stage of a Kira Space task.\n\n" + r.inline() +
				"\nYour assignment: change greeting.txt in the working directory so it contains exactly: hello, world\n" +
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
			if !strings.Contains(strings.ToLower(r.draftText()), "greeting") {
				return false, "draft missing or does not name greeting.txt"
			}
			if r.arm == "R" { // the real server's finish_step
				if len(r.finishes) == 0 || r.finishes[len(r.finishes)-1].Status != "done" {
					return false, "finish_step done not accepted"
				}
				return true, ""
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
			return "You are in a Kira Space task.\n\n" + r.inline() +
				"\nYour assignment: save a validation proof named \"Orders match ledger\" with the summary \"All 20 orders of the day match their ledger lines on id and total.\" and three steps.\n" +
				"(1) A table, described \"Orders and ledger agree on all 20 rows\", columns order_id, order_total, ledger_total (all numbers), rows:\n" + jsonRows(r.lib.rows) + "\n" +
				"(2) A diagram, described \"The matching path the reconcile command follows\", Mermaid source:\n" + r.lib.flow + "\n" +
				"(3) A document, described \"Why the match holds and what a late edit does\", with this explanation text:\n" + r.lib.explain + "\n\nThen reply: done." + r.folderLine()
		},
		oracle: func(r *spikeRun, _ string) (bool, string) {
			var p *spikeProof
			if r.arm == "T" {
				if p = r.studio.proofs["orders match ledger"]; p == nil {
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
			}
			if !strings.EqualFold(p.Name, "Orders match ledger") || strings.TrimSpace(p.Summary) == "" {
				return false, "wrong name or no summary"
			}
			return proofChecks(r, p, r.lib.rows)
		},
	},
	{
		id: "F3", name: "proof edit",
		seedT: func(r *spikeRun) *spikeProof {
			return &spikeProof{Name: "Orders match ledger", Summary: "All 20 orders of the day match their ledger lines on id and total.", Steps: []spikeStep{
				{Description: "Orders and ledger agree on all 20 rows", View: "table", Rows: r.lib.rows},
				{Description: "The matching path the reconcile command follows", View: "diagram", Text: r.lib.flow},
				{Description: "Why the match holds and what a late edit does", View: "document", Text: r.lib.explain},
			}}
		},
		prompt: func(r *spikeRun) string {
			return "You are in a Kira Space task.\n\n" + r.inline() +
				"\nYour assignment: the closed validation proof \"Orders match ledger\" is wrong: order 12's total is 1175, not 1150, in both the order_total and ledger_total columns. " +
				"The table must then show these rows:\n" + jsonRows(r.correctedRows()) + "\n" +
				"Change only the table step, keep the diagram and document steps as they are" + r.reasonClause("order 12 corrected") + ". Then reply: done." + r.folderLine()
		},
		oracle: func(r *spikeRun, _ string) (bool, string) {
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
			if r.arm != "R" && strings.TrimSpace(p.Reason) == "" {
				return false, "no reason"
			}
			return proofChecks(r, p, r.correctedRows())
		},
	},
	{
		id: "F4", name: "orchestrator summary",
		prompt: func(r *spikeRun) string {
			return "You orchestrate two child tasks. For each child, name the blocking open risk that its latest handover reports. " +
				"Reply in two sentences, one per child, naming the child by its title." + r.folderLine()
		},
		oracle: func(_ *spikeRun, result string) (bool, string) {
			sent := strings.ToLower(result)
			if !strings.Contains(sent, "billing") || !strings.Contains(sent, "settings") {
				return false, "a child title is missing"
			}
			if !strings.Contains(sent, "batch") {
				return false, "billing risk (refund batch) missing"
			}
			if !strings.Contains(sent, "integrations") || !slices.ContainsFunc([]string{"focus is lost", "focus lost", "loses focus", "lost focus", "focus-lost"}, func(k string) bool { return strings.Contains(sent, k) }) {
				return false, "settings risk (focus lost on integrations) missing"
			}
			return true, ""
		},
	},
	{
		id: "F6", name: "create 2 child tasks",
		prompt: flowByID("F6").prompt, oracle: flowByID("F6").oracle,
	},
	{
		id: "F5", name: "branch wait",
		prompt: func(r *spikeRun) string {
			return "You are in a Kira Space task.\n\n" + r.inline() +
				"\nYour assignment: branch feature/x of repo app in this task is being set up. Wait until its setup is done, then reply with the branch's worktree path and nothing else." + r.folderLine()
		},
		oracle: flowByID("F5").oracle,
	},
}

// ---- report ----

func rng(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	return slices.Max(v) - slices.Min(v)
}

func rngTxt(v []float64, f string) string {
	if len(v) == 0 {
		return "-"
	}
	return fmt.Sprintf(f+" ["+f+"-"+f+"]", median(v), slices.Min(v), slices.Max(v))
}

func pick1b(recs []spikeRec, flow, arm string, f func(spikeRec) float64) []float64 {
	var out []float64
	for _, r := range recs {
		if r.Flow == flow && r.Arm == arm {
			out = append(out, f(r))
		}
	}
	return out
}

func passCount(recs []spikeRec, flow, arm string) (int, int) {
	p, n := 0, 0
	for _, r := range recs {
		if r.Flow == flow && r.Arm == arm {
			n++
			if r.Pass {
				p++
			}
		}
	}
	return p, n
}

func spike1bReport(recs []spikeRec, spent float64, note string, planned int, arms []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Runs: %d of %d planned, sonnet only. Spend: %.4f USD. %s\n\n", len(recs), planned, spent, note)
	names := map[string]string{}
	var order []string
	for _, f := range spikeFlows1b {
		names[f.id] = f.name
		order = append(order, f.id)
	}
	cost := func(r spikeRec) float64 { return r.Cost }
	ctxF := func(r spikeRec) float64 { return float64(r.CtxFinal) }
	fails := func(r spikeRec) float64 { return float64(r.FailedEdits + r.Retried) }
	b.WriteString("### Cost, context, edit ease\n\nCells are median [min-max] over the runs.\n\n| Flow | Arm | Pass | Cost USD | Final context | Turns | Failed edit-path + retries |\n|---|---|---|---|---|---|---|\n")
	for _, id := range order {
		for _, arm := range arms {
			p, n := passCount(recs, id, arm)
			if n == 0 {
				continue
			}
			fmt.Fprintf(&b, "| %s %s | %s | %d/%d | %s | %s | %s | %s |\n", id, names[id], arm, p, n,
				rngTxt(pick1b(recs, id, arm, cost), "%.4f"), rngTxt(pick1b(recs, id, arm, ctxF), "%.0f"),
				rngTxt(pick1b(recs, id, arm, func(r spikeRec) float64 { return float64(r.Turns) }), "%.0f"), rngTxt(pick1b(recs, id, arm, fails), "%.0f"))
		}
	}
	b.WriteString("\n### Attribution: what each files run read\n\nDoc = README.md and AGENTS.md result bytes read (tokens are bytes/4). History = reads that touched handovers or notes. Reads = Read, Glob, Grep and read-only Bash calls. Search = ToolSearch calls and the context they added. Medians.\n\n| Flow | Arm | Doc bytes | Doc tokens | Reads | Read bytes | History reads | History bytes | Search calls | Search ctx |\n|---|---|---|---|---|---|---|---|---|---|\n")
	for _, id := range order {
		for _, arm := range arms {
			if _, n := passCount(recs, id, arm); n == 0 {
				continue
			}
			md := func(f func(spikeRec) float64) float64 { return median(pick1b(recs, id, arm, f)) }
			fmt.Fprintf(&b, "| %s | %s | %.0f | %.0f | %.1f | %.0f | %.1f | %.0f | %.1f | %.0f |\n", id, arm,
				md(func(r spikeRec) float64 { return float64(r.DocBytes) }), md(func(r spikeRec) float64 { return float64(r.DocBytes) / 4 }),
				md(func(r spikeRec) float64 { return float64(r.ReadCalls) }), md(func(r spikeRec) float64 { return float64(r.ReadBytes) }),
				md(func(r spikeRec) float64 { return float64(r.HistReads) }), md(func(r spikeRec) float64 { return float64(r.HistBytes) }),
				md(func(r spikeRec) float64 { return float64(r.SearchCalls) }), md(func(r spikeRec) float64 { return float64(r.SearchCtx) }))
		}
	}
	b.WriteString("\n### Gate per files arm against T\n\nCost F/T median at most 1.2 (F6: 1.1); final context F/T median at most 1.0; pass count not lower; failed edit-path calls plus retries (mean) not higher than T's. `inconclusive` marks a metric where the gap between medians is smaller than the larger run-to-run range of the two arms. Turns are in the table above, never gated.\n\n| Flow | Arm | Cost F/T | Cost | Ctx F/T | Ctx | Pass | Pass | Edit F vs T | Edit |\n|---|---|---|---|---|---|---|---|---|---|\n")
	verdicts := map[string][]bool{}
	for _, id := range order {
		for _, arm := range arms {
			if arm == "T" {
				continue
			}
			if _, n := passCount(recs, id, arm); n == 0 {
				continue
			}
			tc, fc := pick1b(recs, id, "T", cost), pick1b(recs, id, arm, cost)
			tx, fx := pick1b(recs, id, "T", ctxF), pick1b(recs, id, arm, ctxF)
			te, fe := pick1b(recs, id, "T", fails), pick1b(recs, id, arm, fails)
			maxCost := 1.2
			if id == "F6" {
				maxCost = 1.1
			}
			incon := func(t, f []float64) string {
				if abs(median(f)-median(t)) < max(rng(t), rng(f)) {
					return " (inconclusive)"
				}
				return ""
			}
			cr, xr := safeDiv(median(fc), median(tc)), safeDiv(median(fx), median(tx))
			tp, tn := passCount(recs, id, "T")
			fp, fn := passCount(recs, id, arm)
			costOK, ctxOK, passOK, editOK := cr <= maxCost, xr <= 1.0, fp*tn >= tp*fn, mean(fe) <= mean(te)
			verdicts[arm] = append(verdicts[arm], costOK, ctxOK, passOK, editOK)
			fmt.Fprintf(&b, "| %s | %s | %.2f (<= %.1f) | %s%s | %.2f (<= 1.0) | %s%s | %d/%d vs %d/%d | %s | %.2f vs %.2f | %s%s |\n", id, arm, cr, maxCost, yn(costOK), incon(tc, fc), xr, yn(ctxOK), incon(tx, fx),
				fp, fn, tp, tn, yn(passOK), mean(fe), mean(te), yn(editOK), incon(te, fe))
		}
	}
	b.WriteString("\n## Runs\n\n| Flow | Arm | Rep | Pass | Cost | Turns | Final ctx | Doc bytes | Reads | Hist reads | Search | Failed+retr | Tool calls | Note |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	sorted := slices.Clone(recs)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Flow != sorted[j].Flow {
			return sorted[i].Flow < sorted[j].Flow
		}
		return sorted[i].Arm < sorted[j].Arm
	})
	for _, r := range sorted {
		var tools []string
		for k, n := range r.Tools {
			tools = append(tools, fmt.Sprintf("%s×%d", k, n))
		}
		slices.Sort(tools)
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %.4f | %d | %d | %d | %d | %d | %d | %d | %s | %s |\n", r.Flow, r.Arm, r.Rep, yn(r.Pass), r.Cost, r.Turns, r.CtxFinal,
			r.DocBytes, r.ReadCalls, r.HistReads, r.SearchCalls, r.FailedEdits+r.Retried, strings.Join(tools, " "), strings.ReplaceAll(r.Why, "|", "/"))
	}
	return b.String()
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// ---- tests ----

func spikeHome(t *testing.T) string {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".claude.json"), `{"hasCompletedOnboarding":true,"theme":"dark"}`)
	return home
}

func TestDetailsSpike1b(t *testing.T) {
	if os.Getenv("KIRA_SPIKE") != "run1b" {
		t.Skip("P643 Part 1b spends real tokens; run with KIRA_SPIKE=run1b (see the file comment)")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("no claude on PATH")
	}
	spike1bOn = true
	defer func() { spike1bOn = false }()
	reps := 3
	if v, err := strconv.Atoi(os.Getenv("KIRA_SPIKE1B_REPS")); err == nil && v > 0 {
		reps = v
	}
	budget := 4.0
	if v, err := strconv.ParseFloat(os.Getenv("KIRA_SPIKE1B_BUDGET"), 64); err == nil && v > 0 {
		budget = v
	}
	set := func(env string) map[string]bool {
		m := map[string]bool{}
		for _, f := range strings.Split(os.Getenv(env), ",") {
			if f = strings.TrimSpace(f); f != "" {
				m[f] = true
			}
		}
		return m
	}
	flows, noFC := set("KIRA_SPIKE1B_FLOWS"), set("KIRA_SPIKE1B_NOFC")
	arms := []string{"T", "FF", "FL", "FC", "FA", "FP"}
	if a := set("KIRA_SPIKE1B_ARMS"); len(a) > 0 {
		arms = slices.DeleteFunc(arms, func(x string) bool { return !a[x] })
	}
	out := os.Getenv("KIRA_SPIKE1B_OUT")
	if out == "" {
		t.Skip("set KIRA_SPIKE1B_OUT")
	}
	home := spikeHome(t)

	type job struct {
		fl  spikeFlow
		arm string
		rep int
	}
	skip := set("KIRA_SPIKE1B_SKIP") // done runs as "F1-T-1"
	flowReps := map[string]int{}     // KIRA_SPIKE1B_FLOWREPS="F2=1,F5=1" overrides reps per flow
	for k := range set("KIRA_SPIKE1B_FLOWREPS") {
		id, n, _ := strings.Cut(k, "=")
		flowReps[id], _ = strconv.Atoi(n)
	}
	var jobs []job
	for rep := 1; rep <= reps; rep++ {
		for _, fl := range spikeFlows1b {
			if len(flows) > 0 && !flows[fl.id] {
				continue
			}
			if n, ok := flowReps[fl.id]; ok && rep > n {
				continue
			}
			for _, arm := range arms {
				if arm == "FC" && noFC[fl.id] || skip[fmt.Sprintf("%s-%s-%d", fl.id, arm, rep)] {
					continue
				}
				jobs = append(jobs, job{fl, arm, rep})
			}
		}
	}
	var recs []spikeRec
	var spent, maxRun float64
	if v, err := strconv.ParseFloat(os.Getenv("KIRA_SPIKE1B_FIRSTRUN"), 64); err == nil {
		maxRun = v / 1.5 // expected cost of the first run, when the budget left is below the per-run cap
	}
	note := ""
	for _, j := range jobs {
		reserve := spikeRunCap1b
		if maxRun > 0 {
			reserve = min(spikeRunCap1b, maxRun*1.5)
		}
		if spent+reserve > budget {
			note = fmt.Sprintf("Budget guard: spent %.4f USD, next run could add %.2f, budget %.2f. Remaining runs skipped.", spent, reserve, budget)
			break
		}
		rec := execute(t, home, j.fl, j.arm, "sonnet", j.rep)
		spent += rec.Cost
		maxRun = max(maxRun, rec.Cost)
		recs = append(recs, rec)
		t.Logf("%s %s #%d pass=%v cost=%.4f turns=%d spent=%.4f %s", rec.Flow, rec.Arm, rec.Rep, rec.Pass, rec.Cost, rec.Turns, spent, rec.Why)
		if err := os.WriteFile(out, []byte(spike1bReport(recs, spent, note, len(jobs), arms)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(out, []byte(spike1bReport(recs, spent, note, len(jobs), arms)), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("part 1b spend %.4f USD over %d runs; raw results in %s", spent, len(recs), out)
}

// TestDetailsSpikeContext1b measures the turn-1 context of single-turn probes to split the ~38k
// tokens into components. KIRA_SPIKE1B_PROBES picks probes (default: the cheap ones).
func TestDetailsSpikeContext1b(t *testing.T) {
	if os.Getenv("KIRA_SPIKE") != "run1b" || os.Getenv("KIRA_SPIKE1B_PROBE_OUT") == "" {
		t.Skip("P643 Part 1b spends real tokens; run with KIRA_SPIKE=run1b and KIRA_SPIKE1B_PROBE_OUT")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("no claude on PATH")
	}
	spike1bOn = true
	defer func() { spike1bOn = false }()
	home := spikeHome(t)
	type probe struct {
		name, flow, arm string // arm "" = no MCP at all
		tools           string // --tools value, "" = default
		noDefer         bool
	}
	all := []probe{
		{"bare (no tools, no MCP)", "", "", "none", false},
		{"built-in tools, no MCP", "", "", "", false},
		{"T run role, 12 tools", "F5", "T", "", false},
		{"T orchestrator, 15 tools", "F4", "T", "", false},
		{"F run role, 5 tools", "F5", "FL", "", false},
		{"F orchestrator, 6 tools", "F4", "FL", "", false},
		{"T run role, 12 tools, no deferral", "F5", "T", "", true},
		{"F run role, 5 tools, no deferral", "F5", "FL", "", true},
		{"T orchestrator, 15 tools, no deferral", "F4", "T", "", true},
		{"F orchestrator, 6 tools, no deferral", "F4", "FL", "", true},
	}
	want := map[string]bool{}
	for _, n := range strings.Split(os.Getenv("KIRA_SPIKE1B_PROBES"), ",") {
		if n = strings.TrimSpace(n); n != "" {
			want[n] = true
		}
	}
	var b strings.Builder
	b.WriteString("| Probe | MCP tools listed | Turn-1 context | Cost USD |\n|---|---|---|---|\n")
	for i, p := range all {
		if len(want) > 0 && !want[strconv.Itoa(i+1)] {
			continue
		}
		if len(want) == 0 && p.noDefer {
			continue
		}
		var cfg string
		var addDirs []string
		repoDir := t.TempDir()
		r := &spikeRun{t: t, flow: p.flow, arm: map[bool]string{true: "T", false: "F"}[p.arm == "T"], variant: p.arm, lib: loadSpikeLib(t)}
		if p.arm == "T" {
			r.variant = ""
		}
		if p.arm != "" {
			var done func()
			cfg, addDirs, done = r.setup(t, flowByIDs(spikeFlows1b, p.flow))
			defer done()
			repoDir = r.repo
		}
		args := []string{"-p", "Reply with the single word: ok", "--model", "sonnet", "--max-budget-usd", "0.3", "--output-format", "stream-json", "--verbose", "--strict-mcp-config"}
		if cfg != "" {
			args = append(args, "--mcp-config", cfg)
		}
		for _, d := range addDirs {
			args = append(args, "--add-dir", d)
		}
		if p.tools != "" {
			args = append(args, "--tools", "")
		}
		cmd := exec.Command("claude", args...)
		cmd.Dir = repoDir
		cmd.Env = append(filteredEnv(home), map[bool]string{true: "ENABLE_TOOL_SEARCH=false", false: "ENABLE_TOOL_SEARCH=true"}[p.noDefer])
		var so bytes.Buffer
		cmd.Stdout = &so
		_ = cmd.Run()
		rec := spikeRec{Tools: map[string]int{}}
		parseSpikeStream(so.String(), &rec)
		fmt.Fprintf(&b, "| %d %s | %d | %d | %.4f |\n", i+1, p.name, rec.MCPTools, rec.CtxFirst, rec.Cost)
		t.Logf("probe %d %s: mcp=%d ctx=%d cost=%.4f", i+1, p.name, rec.MCPTools, rec.CtxFirst, rec.Cost)
		if d := os.Getenv("KIRA_SPIKE_DUMP"); d != "" {
			_ = os.MkdirAll(d, 0o755)
			_ = os.WriteFile(filepath.Join(d, fmt.Sprintf("probe-%d.jsonl", i+1)), so.Bytes(), 0o644)
		}
	}
	out := os.Getenv("KIRA_SPIKE1B_PROBE_OUT")
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func flowByIDs(fs []spikeFlow, id string) spikeFlow {
	for _, f := range fs {
		if f.id == id {
			return f
		}
	}
	return spikeFlow{id: id}
}

func filteredEnv(home string) []string {
	env := []string{"HOME=" + home}
	for _, e := range os.Environ() {
		if k, _, _ := strings.Cut(e, "="); k == "HOME" || k == "CLAUDE_CODE_SESSION_ID" || k == "CLAUDE_CODE_REMOTE_SESSION_ID" || k == "CLAUDECODE" || k == "ENABLE_TOOL_SEARCH" {
			continue
		}
		env = append(env, e)
	}
	return env
}

// TestDetailsSpike1bReport rebuilds the report from transcripts of earlier runs (KIRA_SPIKE_REPORT_DIRS,
// comma list of dump dirs) and their verbose logs (KIRA_SPIKE_REPORT_LOGS); no tokens spent.
func TestDetailsSpike1bReport(t *testing.T) {
	dirs, logs, out := os.Getenv("KIRA_SPIKE_REPORT_DIRS"), os.Getenv("KIRA_SPIKE_REPORT_LOGS"), os.Getenv("KIRA_SPIKE1B_OUT")
	if dirs == "" || logs == "" || out == "" {
		t.Skip("set KIRA_SPIKE_REPORT_DIRS, KIRA_SPIKE_REPORT_LOGS and KIRA_SPIKE1B_OUT")
	}
	re := regexp.MustCompile(`(F\d) (T|FF|FL|FC|FA|FP) #(\d+) pass=(true|false) cost=[0-9.]+ turns=\d+ spent=[0-9.]+ ?(.*)$`)
	pass := map[string][2]string{}
	for _, lf := range strings.Split(logs, ",") {
		for _, l := range strings.Split(string(mustRead(t, lf)), "\n") {
			if m := re.FindStringSubmatch(l); m != nil {
				pass[m[1]+"-"+m[2]+"-"+m[3]] = [2]string{m[4], strings.TrimSpace(m[5])}
			}
		}
	}
	var recs []spikeRec
	var spent float64
	armSet := map[string]bool{}
	for _, d := range strings.Split(dirs, ",") {
		files, _ := filepath.Glob(filepath.Join(d, "F*-sonnet-*.jsonl"))
		for _, f := range files {
			parts := strings.Split(strings.TrimSuffix(filepath.Base(f), ".jsonl"), "-")
			if len(parts) != 4 {
				continue
			}
			rep, _ := strconv.Atoi(parts[3])
			rec := spikeRec{Flow: parts[0], Arm: parts[1], Model: "sonnet", Rep: rep}
			parseSpikeStream(string(mustRead(t, f)), &rec)
			p := pass[parts[0]+"-"+parts[1]+"-"+parts[3]]
			rec.Pass, rec.Why = p[0] == "true", p[1]
			spent += rec.Cost
			recs = append(recs, rec)
			armSet[parts[1]] = true
		}
	}
	var arms []string
	for _, a := range []string{"T", "FF", "FL", "FC", "FA", "FP"} {
		if armSet[a] {
			arms = append(arms, a)
		}
	}
	if err := os.WriteFile(out, []byte(spike1bReport(recs, spent, os.Getenv("KIRA_SPIKE1B_NOTE"), len(recs), arms)), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d runs, %.4f USD", len(recs), spent)
}

// TestDetailsSpikeToolBytes lists the tools each arm serves and sums their JSON size (name,
// description, input schema): the schema share of the context once loaded. Free, no claude.
func TestDetailsSpikeToolBytes(t *testing.T) {
	out := os.Getenv("KIRA_SPIKE1B_TOOLS_OUT")
	if os.Getenv("KIRA_SPIKE") != "run1b" || out == "" {
		t.Skip("set KIRA_SPIKE=run1b and KIRA_SPIKE1B_TOOLS_OUT")
	}
	spike1bOn = true
	defer func() { spike1bOn = false }()
	var b strings.Builder
	b.WriteString("| Arm and role | Tools | Bytes (name, description, schema) | Tokens at bytes/4 |\n|---|---|---|---|\n")
	for _, c := range []struct{ name, flow, arm string }{
		{"T run role (F5)", "F5", "T"}, {"T run role with proof tools (F2)", "F2", "T"}, {"T orchestrator (F4)", "F4", "T"},
		{"F run role", "F5", "FL"}, {"F orchestrator with create_task (F4)", "F4", "FL"}, {"F orchestrator without create_task (F6)", "F6", "FL"},
	} {
		r := &spikeRun{t: t, flow: c.flow, arm: map[bool]string{true: "T", false: "F"}[c.arm == "T"], variant: c.arm, lib: loadSpikeLib(t)}
		if c.arm == "T" {
			r.variant = ""
		}
		cfg, _, done := r.setup(t, flowByIDs(spikeFlows1b, c.flow))
		var conf struct {
			MCPServers map[string]struct {
				URL     string            `json:"url"`
				Headers map[string]string `json:"headers"`
			} `json:"mcpServers"`
		}
		if err := json.Unmarshal(mustRead(t, cfg), &conf); err != nil {
			t.Fatal(err)
		}
		n, total := 0, 0
		for _, sv := range conf.MCPServers {
			tools, err := listTools(t, sv.URL, sv.Headers)
			if err != nil {
				t.Fatal(err)
			}
			for _, tl := range tools {
				raw, _ := json.Marshal(tl)
				total += len(raw)
				n++
			}
		}
		done()
		fmt.Fprintf(&b, "| %s | %d | %d | %d |\n", c.name, n, total, total/4)
	}
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

type headerTransport map[string]string

func (h headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for k, v := range h {
		r.Header.Set(k, v)
	}
	return http.DefaultTransport.RoundTrip(r)
}

func listTools(t *testing.T, url string, headers map[string]string) ([]*mcp.Tool, error) {
	t.Helper()
	ctx := context.Background()
	cl := mcp.NewClient(&mcp.Implementation{Name: "spike", Version: "1"}, nil)
	sess, err := cl.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: url, HTTPClient: &http.Client{Transport: headerTransport(headers)}}, nil)
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	res, err := sess.ListTools(ctx, nil)
	if err != nil {
		return nil, err
	}
	return res.Tools, nil
}
