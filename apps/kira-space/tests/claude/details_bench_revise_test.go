package claude

// P643 benchmark flows F1b and F3b: read an existing large artifact and revise a small part of it, twice.
// A run is two `claude -p` passes over the same task state; the cell is the sum, and the per-pass records
// (Flow "F1b#1", "F1b#2") feed the crossover table. Not gated: new flows, reported for information.
//
//	F1b handover revision. T: the handover is in the prompt and every pass resends it whole in finish_step's
//	    summary (today's only handover call). R: the handover is handovers/draft-implement.md; the model edits
//	    it, then submit and state step.finish.
//	F3b proof revision. T: the table rows are in the prompt (proof_list lists only ordinals), proof_save resends
//	    every step (keepOrdinal for the untouched ones). R: proof.md and its data files in the task folder,
//	    then submit.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// benchPasses is the number of passes of a revision flow.
const benchPasses = 2

// f3bRowCount is the size of the revised proof's table.
const f3bRowCount = 10

// benchReviseFlows are the multi-pass flows: reported, never gated.
var benchReviseFlows = []string{"F1b", "F3b"}

func benchMultiPass(flow string) bool { return slices.Contains(benchReviseFlows, flow) }

// benchFlow finds a benchmark flow: the revision flows first, then the spike's.
func benchFlow(id string) spikeFlow {
	for _, f := range benchRevisionFlows {
		if f.id == id {
			return f
		}
	}
	return flowByIDs(spikeFlows1b, id)
}

const f1bSeedLine = "seeds 400 orders across three days"

// f1bCheck verifies a revised handover after the given pass; base is the handover the task started with.
func f1bCheck(text, base string, pass int) (bool, string) {
	if len(text) < len(base)*9/10 {
		return false, fmt.Sprintf("handover shrank to %d bytes from %d", len(text), len(base))
	}
	for _, k := range []string{"## Decisions and why", "## How verified", "Start with the posting path in internal/ledger/post.go"} {
		if !strings.Contains(text, k) {
			return false, "handover lost " + k
		}
	}
	for _, c := range []struct{ has, not string }{
		{"seeds 600 orders across five days", f1bSeedLine},
		{"fine up to about two thousand mismatches", "fine up to a few hundred mismatches"},
	} {
		if !strings.Contains(text, c.has) || strings.Contains(text, c.not) {
			return false, "pass 1 change missing: " + c.has
		}
	}
	if !strings.Contains(text, "## Follow-up") || !strings.Contains(text, "--dry-run first for one week") {
		return false, "pass 1 follow-up paragraph missing"
	}
	if pass >= 2 && (!strings.Contains(text, "Keep the old numeric column for two releases") || strings.Contains(text, "Keep the old numeric column for one release")) {
		return false, "pass 2 change missing"
	}
	if pass < 2 && !strings.Contains(text, "Keep the old numeric column for one release") {
		return false, "pass 1 changed decision 6"
	}
	return true, ""
}

func f1bChanges(pass int) string {
	if pass == 1 {
		return "Make three changes and nothing else.\n" +
			"(1) In \"What changed\", the line for scripts/seed_orders.sh must say it seeds 600 orders across five days, not 400 across three days.\n" +
			"(2) In \"Open risks\", the admin table line must say it is fine up to about two thousand mismatches, not a few hundred.\n" +
			"(3) Add a new section \"## Follow-up\" at the end with one paragraph: \"The nightly job must run reconcile with --dry-run first for one week before it is wired into the schedule.\""
	}
	return "Make one change and nothing else, and keep your earlier changes: in \"Decisions and why\", decision 6 must say the old numeric column stays for two releases, not one."
}

// f1bText is the handover a pass starts from: the seed, then what the previous pass left.
func (r *spikeRun) f1bText() string {
	if r.pass > 1 && r.finishBase > 0 {
		if s := r.finishes[r.finishBase-1].Summary; strings.TrimSpace(s) != "" {
			return s
		}
	}
	return r.lib.latest().body
}

func f3bBaseRows(r *spikeRun) [][]any {
	out := make([][]any, f3bRowCount)
	for i := range out {
		out[i] = slices.Clone(r.lib.rows[i])
	}
	return out
}

// f3bRows are the table rows the proof must hold after the given pass.
func f3bRows(r *spikeRun, pass int) [][]any {
	rows := f3bBaseRows(r)
	for _, row := range rows {
		switch id, _ := row[0].(float64); {
		case id == 7:
			row[1], row[2] = 900.0, 900.0
		case id == 3 && pass >= 2:
			row[1] = 2550.0
		}
	}
	return rows
}

func f3bCheck(r *spikeRun, p *spikeProof, pass int) (bool, string) {
	if strings.TrimSpace(p.Reason) == "" {
		return false, "no reason"
	}
	if len(p.Steps) != 4 || p.Steps[0].View != "table" || p.Steps[1].View != "diagram" || p.Steps[2].View != "document" || p.Steps[3].View != "document" {
		return false, "steps are not table, diagram, document, document"
	}
	if !rowsEqual(p.Steps[0].Rows, f3bRows(r, pass)) {
		return false, "table rows wrong"
	}
	if !strings.Contains(p.Steps[1].Text, "flowchart") || !strings.Contains(p.Steps[1].Text, "Mismatch grouped by cause") {
		return false, "diagram lost or altered"
	}
	if !strings.Contains(p.Steps[2].Text, "matched against their old total") {
		return false, "document lost or altered"
	}
	if !strings.Contains(p.Steps[3].Text, "corrected after close") {
		return false, "note step missing"
	}
	return true, ""
}

// f3bSeedT is the closed proof arm T starts with: the F3 proof with the first ten table rows.
func f3bSeedT(r *spikeRun) *spikeProof {
	return &spikeProof{Name: "Orders match ledger", Summary: f3bSummary, Steps: []spikeStep{
		{Description: f3bTableDesc, View: "table", Rows: f3bBaseRows(r)},
		{Description: "The matching path the reconcile command follows", View: "diagram", Text: r.lib.flow},
		{Description: "Why the match holds and what a late edit does", View: "document", Text: r.lib.explain},
	}}
}

const (
	f3bSummary   = "All 10 orders of the day match their ledger lines on id and total."
	f3bTableDesc = "Orders and ledger agree on all 10 rows"
)

var benchRevisionFlows = []spikeFlow{
	{
		id: "F1b", name: "handover revision, 2 passes",
		prompt: func(r *spikeRun) string {
			head := "You are the implement stage of a Kira Space task.\n\n"
			if r.arm == "T" {
				return head + "Your handover for the review stage so far:\n---\n" + r.f1bText() + "\n---\n\nYour assignment: revise this handover. " + f1bChanges(r.pass) +
					"\nKeep everything else as it is. Then finish the step with the complete revised handover as the summary."
			}
			return head + "Your assignment: revise your handover for the review stage, the draft handovers/draft-implement.md, in place. " + f1bChanges(r.pass) +
				"\nKeep everything else as it is. Then finish the step." + r.folderLine()
		},
		oracle: func(r *spikeRun, _ string) (bool, string) {
			if len(r.finishes) <= r.finishBase || r.finishes[len(r.finishes)-1].Status != "done" {
				return false, "finish_step done not called this pass"
			}
			text := r.draftText()
			if r.arm == "T" {
				text = r.finishes[len(r.finishes)-1].Summary
			}
			return f1bCheck(text, r.lib.latest().body, r.pass)
		},
	},
	{
		id: "F3b", name: "proof revision, 2 passes",
		seedT: f3bSeedT,
		prompt: func(r *spikeRun) string {
			var rows string
			if r.arm == "T" { // the old tools list ordinals only: the rows come with the prompt
				cur := r.studio.proofs["orders match ledger"]
				rows = "The proof's table step currently holds these rows (columns order_id, order_total, ledger_total):\n" + jsonRows(cur.Steps[0].Rows) + "\n\n"
			}
			change, reason := "order 7's total is 900, not 850, in both the order_total and ledger_total columns. Also add a fourth step, a document described \"Late correction note\", "+
				"with exactly this text: \"Order 7 was corrected after close; the reconcile command was rerun.\"", "order 7 corrected"
			if r.pass > 1 {
				change, reason = "order 3's order_total is 2550, not 2500 (change that one cell only). Keep the note step.", "order 3 corrected"
			}
			return "You are in a Kira Space task.\n\n" + rows +
				"Your assignment: the closed validation proof \"Orders match ledger\" needs a late correction: " + change +
				" Keep every other step as it is, and give the reason \"" + reason + "\". Then reply: done." + r.folderLine()
		},
		oracle: func(r *spikeRun, _ string) (bool, string) {
			var p *spikeProof
			if r.arm == "T" {
				if p = r.studio.edits["orders match ledger"]; p == nil {
					return false, "no pending edit this pass"
				}
			} else {
				var seen bool
				var why string
				if p, seen, why = r.finalProof(); p == nil {
					return false, why
				}
				if !seen {
					return false, "edit never submitted this pass"
				}
			}
			return f3bCheck(r, p, r.pass)
		},
	},
}

// promoteEdit accepts the pending edit, as the user does between two passes.
func (st *studioStub) promoteEdit() {
	st.mu.Lock()
	defer st.mu.Unlock()
	for k, e := range st.edits {
		e.Pending = false
		st.proofs[k] = e
		delete(st.edits, k)
	}
}

// executePasses runs a revision flow: setup once, then benchPasses claude runs over the same state.
// It returns the summed record and the per-pass records.
func executePasses(t *testing.T, home string, fl spikeFlow, arm, model string, rep int) (spikeRec, []spikeRec) {
	t.Helper()
	r := &spikeRun{t: t, flow: fl.id, arm: arm, lib: loadSpikeLib(t)}
	cfg, addDirs, done := r.setup(t, fl)
	defer done()
	if r.studio != nil && r.afterPass == nil { // arm T: the user accepts the pending edit between passes
		r.afterPass = r.studio.promoteEdit
	}
	var passes []spikeRec
	for p := 1; p <= benchPasses; p++ {
		r.pass, r.finishBase = p, len(r.finishes)
		rec := runClaude(t, home, r, fl, cfg, addDirs, arm, model, rep, fmt.Sprintf("-p%d", p))
		passes = append(passes, rec)
		if r.afterPass != nil {
			r.afterPass()
		}
	}
	return sumPasses(fl.id, passes), passes
}

// sumPasses is the cell record: costs and counts add up, the final context is the last pass's.
func sumPasses(flow string, passes []spikeRec) spikeRec {
	out := spikeRec{Flow: flow, Arm: passes[0].Arm, Model: passes[0].Model, Rep: passes[0].Rep, Pass: true, Tools: map[string]int{}, MCPTools: passes[0].MCPTools, CtxFirst: passes[0].CtxFirst}
	var why []string
	for i, p := range passes {
		out.Pass = out.Pass && p.Pass
		out.Cost += p.Cost
		out.Turns += p.Turns
		out.In += p.In
		out.CacheRead += p.CacheRead
		out.CacheWrite += p.CacheWrite
		out.Out += p.Out
		out.Wall += p.Wall
		out.Aborted = out.Aborted || p.Aborted
		out.CtxFinal, out.CtxGrowth = p.CtxFinal, p.CtxGrowth
		e, o := &out.spikeEditStats, p.spikeEditStats
		e.EditCalls += o.EditCalls
		e.ShellWrites += o.ShellWrites
		e.FailedEdits += o.FailedEdits
		e.Retried += o.Retried
		e.WholeRewrites += o.WholeRewrites
		e.WrongFile += o.WrongFile
		e.ReadCalls += o.ReadCalls
		e.ReadBytes += o.ReadBytes
		e.DocBytes += o.DocBytes
		e.HistReads += o.HistReads
		e.HistBytes += o.HistBytes
		e.SearchCalls += o.SearchCalls
		e.SearchCtx += o.SearchCtx
		for k, n := range p.Tools {
			out.Tools[k] += n
		}
		w := p.Why
		if w == "" {
			w = "ok"
		}
		why = append(why, fmt.Sprintf("p%d: %s", i+1, w))
	}
	out.Why = strings.Join(why, "; ")
	return out
}

// benchPassTable is the crossover table: what each pass cost on each arm, with input tokens (fresh,
// cache read and cache write together) and the final context of the pass.
func benchPassTable(recs []spikeRec, flows, arms []string) string {
	var b strings.Builder
	b.WriteString("\n### Per pass (informational)\n\nInput tokens are fresh plus cache read plus cache write over the whole pass, summed over model calls.\n\n" +
		"| Flow | Arm | Pass | Cost USD (median) | Input tokens (median) | Output tokens (median) | Final context (median) | Turns (median) | Pass rate | Failed edit-path + retries (mean) |\n|---|---|---|---|---|---|---|---|---|---|\n")
	for _, f := range flows {
		if !benchMultiPass(f) {
			continue
		}
		for _, a := range arms {
			for _, p := range []string{"1", "2", "both"} {
				key := f + "#" + p
				if p == "both" {
					key = f
				}
				g := spikeAggregate(recs, key, a, "sonnet")
				if g.runs == 0 {
					continue
				}
				in := make([]float64, g.runs)
				fe := make([]float64, g.runs)
				for i := range in {
					in[i] = g.in[i] + g.cr[i] + g.cw[i]
					fe[i] = g.failed[i] + g.retr[i]
				}
				fmt.Fprintf(&b, "| %s | %s | %s | %.4f | %.0f | %.0f | %.0f | %.0f | %d of %d | %.2f |\n", f, a, p, median(g.cost), median(in), median(g.o), median(g.cf), median(g.turns), g.pass, g.runs, mean(fe))
			}
		}
		if !slices.Contains(arms, "T") || !slices.Contains(arms, "R") {
			continue
		}
		for _, p := range []string{"1", "2", "both"} {
			key := f + "#" + p
			if p == "both" {
				key = f
			}
			t, r := spikeAggregate(recs, key, "T", "sonnet"), spikeAggregate(recs, key, "R", "sonnet")
			if t.runs == 0 || r.runs == 0 {
				continue
			}
			tin, rin := make([]float64, t.runs), make([]float64, r.runs)
			for i := range tin {
				tin[i] = t.in[i] + t.cr[i] + t.cw[i]
			}
			for i := range rin {
				rin[i] = r.in[i] + r.cr[i] + r.cw[i]
			}
			fmt.Fprintf(&b, "| %s | R/T | %s | %.2f | %.2f | %.2f | %.2f | %.2f | | |\n", f, p, median(r.cost)/median(t.cost), median(rin)/median(tin), median(r.o)/median(t.o), median(r.cf)/median(t.cf), median(r.turns)/median(t.turns))
		}
	}
	return b.String()
}

// TestDetailsBenchReviseServes checks, with no claude and no tokens, that both arms seed and serve F1b and
// F3b, that the oracles accept a correct revision of each pass and refuse the untouched start, and that a
// second pass works over the first pass's state.
func TestDetailsBenchReviseServes(t *testing.T) {
	base := loadBenchBaseline(t)
	spike1bOn, spikeFixtures1b = true, benchFixtures
	defer func() { spike1bOn, spikeFixtures1b, benchSetup = false, "testdata/details-spike-1b", nil }()
	for _, tc := range []struct{ flow, arm string }{{"F1b", "T"}, {"F3b", "T"}, {"F1b", "R"}, {"F3b", "R"}} {
		t.Run(tc.flow+tc.arm, func(t *testing.T) {
			benchSetup = benchArmSetups[tc.arm](t, base)
			r := &spikeRun{t: t, flow: tc.flow, arm: tc.arm, lib: loadSpikeLib(t)}
			fl := benchFlow(tc.flow)
			cfg, dirs, done := r.setup(t, fl)
			defer done()
			var conf struct {
				MCPServers map[string]struct {
					URL     string            `json:"url"`
					Headers map[string]string `json:"headers"`
				} `json:"mcpServers"`
			}
			if err := json.Unmarshal(mustRead(t, cfg), &conf); err != nil {
				t.Fatal(err)
			}
			conn := func(name string) *mcp.ClientSession {
				sv := conf.MCPServers[name]
				sess, err := mcp.NewClient(&mcp.Implementation{Name: "check", Version: "1"}, nil).Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: sv.URL, HTTPClient: httpWith(sv.Headers)}, nil)
				if err != nil {
					t.Fatal(err)
				}
				return sess
			}
			call := func(sess *mcp.ClientSession, tool string, args map[string]any) *mcp.CallToolResult {
				res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
				if err != nil {
					t.Fatalf("%s: %v", tool, err)
				}
				if res.IsError {
					raw, _ := json.Marshal(res.Content)
					t.Fatalf("%s: error %s", tool, raw)
				}
				return res
			}
			expect := func(pass int, want bool, what string) {
				t.Helper()
				ok, why := fl.oracle(r, "done")
				if ok != want {
					t.Fatalf("%s pass %d: oracle = %v (%s), want %v", what, pass, ok, why, want)
				}
			}
			r.pass, r.finishBase = 1, 0
			switch tc.flow + tc.arm {
			case "F1bT":
				if !strings.Contains(fl.prompt(r), r.lib.latest().body) {
					t.Fatal("T prompt does not carry the handover")
				}
				sess := conn("kira-space")
				text := r.lib.latest().body
				expect(1, false, "no finish")
				call(sess, "finish_step", map[string]any{"status": "done", "summary": text})
				expect(1, false, "untouched")
				text = f1bRevise(text, 1)
				call(sess, "finish_step", map[string]any{"status": "done", "summary": text})
				expect(1, true, "revised")
				r.pass, r.finishBase = 2, len(r.finishes)
				if !strings.Contains(fl.prompt(r), "## Follow-up") {
					t.Fatal("pass 2 prompt does not carry the pass 1 handover")
				}
				call(sess, "finish_step", map[string]any{"status": "done", "summary": text})
				expect(2, false, "pass 2 untouched")
				call(sess, "finish_step", map[string]any{"status": "done", "summary": f1bRevise(text, 2)})
				expect(2, true, "pass 2 revised")
				_ = sess.Close()
			case "F1bR":
				draft := filepath.Join(r.taskDir, "handovers", "draft-implement.md")
				if string(mustRead(t, draft)) != r.lib.latest().body {
					t.Fatal("draft is not seeded with the handover")
				}
				if len(dirs) != 1 {
					t.Fatalf("add-dirs = %v", dirs)
				}
				sess := conn("kira-space")
				call(sess, "state", map[string]any{"action": "step.finish", "args": map[string]any{"status": "done"}})
				expect(1, false, "untouched")
				text := f1bRevise(r.lib.latest().body, 1)
				writeFile(t, draft, text)
				call(sess, "state", map[string]any{"action": "step.finish", "args": map[string]any{"status": "done"}})
				expect(1, true, "revised")
				r.pass, r.finishBase = 2, len(r.finishes)
				call(sess, "state", map[string]any{"action": "step.finish", "args": map[string]any{"status": "done"}})
				expect(2, false, "pass 2 untouched")
				writeFile(t, draft, f1bRevise(text, 2))
				call(sess, "state", map[string]any{"action": "step.finish", "args": map[string]any{"status": "done"}})
				expect(2, true, "pass 2 revised")
				_ = sess.Close()
			case "F3bT":
				if len(r.studio.proofs["orders match ledger"].Steps[0].Rows) != f3bRowCount {
					t.Fatal("seed table is not 10 rows")
				}
				sess := conn("kira-studio")
				expect(1, false, "no edit")
				call(sess, "proof_list", map[string]any{"name": "Orders match ledger"})
				for pass := 1; pass <= benchPasses; pass++ {
					r.pass = pass
					if !strings.Contains(fl.prompt(r), jsonRows(r.studio.proofs["orders match ledger"].Steps[0].Rows)) {
						t.Fatalf("pass %d prompt does not carry the current rows", pass)
					}
					steps := []map[string]any{
						{"description": f3bTableDesc, "view": "table", "data": map[string]any{"columns": []map[string]any{{"name": "order_id", "type": "number"}, {"name": "order_total", "type": "number"}, {"name": "ledger_total", "type": "number"}}, "rows": f3bRows(r, pass)}},
						{"keepOrdinal": 2}, {"keepOrdinal": 3},
					}
					if pass == 1 {
						steps = append(steps, map[string]any{"description": "Late correction note", "view": "document", "data": map[string]any{"documents": []map[string]any{{"id": "note", "body": map[string]any{"text": "Order 7 was corrected after close; the reconcile command was rerun."}}}}})
					} else {
						steps = append(steps, map[string]any{"keepOrdinal": 4})
					}
					call(sess, "proof_save", map[string]any{"name": "Orders match ledger", "reason": "x", "steps": steps})
					expect(pass, true, "revised")
					r.afterPass = r.studio.promoteEdit
					r.afterPass()
					if _, ok := r.studio.edits["orders match ledger"]; ok {
						t.Fatal("edit not promoted")
					}
				}
				_ = sess.Close()
			case "F3bR":
				proofDir := filepath.Join(r.taskDir, "proofs", "orders-match-ledger")
				var tbl struct {
					Rows [][]any `json:"rows"`
				}
				if err := json.Unmarshal(mustRead(t, filepath.Join(proofDir, "totals.json")), &tbl); err != nil || len(tbl.Rows) != f3bRowCount {
					t.Fatalf("seed totals.json: %d rows, %v", len(tbl.Rows), err)
				}
				if !strings.Contains(string(mustRead(t, filepath.Join(proofDir, "proof.md"))), "all 10 rows") {
					t.Fatal("seed proof.md does not describe 10 rows")
				}
				sess := conn("kira-space")
				expect(1, false, "no submit")
				for pass := 1; pass <= benchPasses; pass++ {
					r.pass = pass
					f3bEditFiles(t, r, proofDir, pass)
					call(sess, "submit", map[string]any{"reason": "late correction"})
					expect(pass, true, "revised")
					if r.afterPass != nil {
						r.afterPass()
					}
				}
				_ = sess.Close()
			}
		})
	}
}

// f1bRevise applies a pass's changes to a handover the way a correct model would.
func f1bRevise(text string, pass int) string {
	if pass == 1 {
		text = strings.Replace(text, f1bSeedLine, "seeds 600 orders across five days", 1)
		text = strings.Replace(text, "fine up to a few hundred mismatches", "fine up to about two thousand mismatches", 1)
		return strings.TrimRight(text, "\n") + "\n\n## Follow-up\n\nThe nightly job must run reconcile with --dry-run first for one week before it is wired into the schedule.\n"
	}
	return strings.Replace(text, "Keep the old numeric column for one release", "Keep the old numeric column for two releases", 1)
}

// f3bEditFiles edits the proof files in the task folder as a model would in the given pass.
func f3bEditFiles(t *testing.T, r *spikeRun, dir string, pass int) {
	t.Helper()
	tbl := string(mustRead(t, filepath.Join(dir, "totals.json")))
	var doc struct {
		Columns []any   `json:"columns"`
		Rows    [][]any `json:"rows"`
	}
	if err := json.Unmarshal([]byte(tbl), &doc); err != nil {
		t.Fatal(err)
	}
	doc.Rows = f3bRows(r, pass)
	raw, _ := json.MarshalIndent(doc, "", "  ")
	writeFile(t, filepath.Join(dir, "totals.json"), string(raw))
	if pass == 1 {
		writeFile(t, filepath.Join(dir, "note.json"), `{"documents":[{"id":"note","body":{"text":"Order 7 was corrected after close; the reconcile command was rerun."}}]}`)
		pm := string(mustRead(t, filepath.Join(dir, "proof.md")))
		pm = strings.TrimSuffix(pm, "---\n") + "  - id: note\n    description: Late correction note\n    view: document\n    data: note.json\n---\n"
		writeFile(t, filepath.Join(dir, "proof.md"), pm)
	}
}

func httpWith(h map[string]string) *http.Client { return &http.Client{Transport: headerTransport(h)} }
