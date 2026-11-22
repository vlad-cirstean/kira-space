package claude

// Realistic fixtures (testdata/details-bench), seeding helpers and the F1b..F6 flows the P714 details bench
// runs on both arms (details_bench_test.go); born as the P643 Part 1b spike.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
	fx := benchFixtures
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
			return false, "unknown arm " + r.arm
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

// ---- tests ----

func spikeHome(t *testing.T) string {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".claude.json"), `{"hasCompletedOnboarding":true,"theme":"dark"}`)
	return home
}

func flowByIDs(fs []spikeFlow, id string) spikeFlow {
	for _, f := range fs {
		if f.id == id {
			return f
		}
	}
	return spikeFlow{id: id}
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
