package memoryflow_test

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/memory"
	"github.com/kirathecat/kira-studio/internal/memory/importer"
	"github.com/kirathecat/kira-studio/internal/memory/memfmt"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const verifySystem = "cross-check facts extracted"

// ingestMask hides what differs per run on top of importMask: document group ids, which are generated.
var ingestMask = flowharness.Mask("roots", "base", "createdAt", "startedAt", "finishedAt", "costUsd", "groupId")

type gateItem struct {
	verdict   string
	fact      string
	questions []string
}

// answer writes a canned headless result and returns its path.
func answer(t *testing.T, structured any) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"type": "result", "subtype": "success", "is_error": false, "structured_output": structured})
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "answer.json")
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// extracted is one canned extract fact; unresolved is the model's own doubt.
type extracted struct{ fact, evidence, unresolved string }

func extractAnswer(t *testing.T, facts ...extracted) string {
	t.Helper()
	var out []map[string]string
	for _, f := range facts {
		out = append(out, map[string]string{"fact": f.fact, "evidence": f.evidence, "section": "", "unresolved": f.unresolved})
	}
	return answer(t, map[string]any{"facts": out})
}

func gateAnswer(t *testing.T, items ...gateItem) string {
	t.Helper()
	var out []map[string]any
	for i, it := range items {
		facts := []map[string]any{}
		if it.verdict == "accept" {
			facts = append(facts, map[string]any{"fact": it.fact, "reason": "Stated in the source.", "keywords": []string{"flow", "ingest", "source", fmt.Sprint("n", i)}})
		}
		out = append(out, map[string]any{"index": i, "verdict": it.verdict, "questions": append([]string{}, it.questions...), "facts": facts})
	}
	return answer(t, map[string]any{"items": out})
}

func reconcileAdds(t *testing.T, n int) string {
	t.Helper()
	var out []map[string]any
	for i := range n {
		out = append(out, map[string]any{"index": i, "action": "add", "target": "", "fact": "", "reason": "", "why": "New subject."})
	}
	return answer(t, map[string]any{"decisions": out})
}

// ingestApp boots the app with the given prompt rules; the finalize agent stores two facts through
// the real memory-mcp server, whose provenance comes from the job.
func ingestApp(t *testing.T, finalize string, rules ...fakeagent.PromptRule) *flowharness.App {
	t.Helper()
	return ingestAppReason(t, "document", finalize, rules...)
}

// ingestAppReason is ingestApp with the reason the finalize agent puts on both stored items.
func ingestAppReason(t *testing.T, reason, finalize string, rules ...fakeagent.PromptRule) *flowharness.App {
	t.Helper()
	app := flowharness.New(t)
	if finalize == "" {
		finalize = testdata(t, "finalize.json")
	}
	app.Scenario(fakeagent.Scenario{Prompts: rules, Claude: map[string][]fakeagent.Action{"*": {{
		MCP: []fakeagent.MCPCall{{Server: "kira-memory", Tool: "store_memory", Args: map[string]any{
			"items": []map[string]string{
				{"fact": "first", "reason": reason, "origin": "agent", "scope": "personal"},
				{"fact": "second", "reason": reason, "origin": "agent", "scope": "personal"},
			},
		}}},
		Emit: finalize,
	}}}})
	return app
}

// docRules answers extract, gate and reconcile for one document with two facts.
func docRules(t *testing.T, doc string, facts [2]extracted) []fakeagent.PromptRule {
	t.Helper()
	return []fakeagent.PromptRule{
		{System: extractSystem, Doc: doc, Emit: extractAnswer(t, facts[0], facts[1])},
		{System: gateSystem, Doc: doc, Emit: gateAnswer(t,
			gateItem{verdict: "accept", fact: facts[0].fact}, gateItem{verdict: "accept", fact: facts[1].fact})},
		{System: reconcileSystem, Doc: doc, Emit: reconcileAdds(t, 2)},
	}
}

// memorySession connects to a real memory-mcp subprocess as a Claude session would.
func memorySession(t *testing.T, app *flowharness.App) *mcp.ClientSession {
	t.Helper()
	cmd := exec.Command(app.W.Memory.McpStatus().Executable, "memory-mcp")
	cmd.Env = append(os.Environ(), "CLAUDE_CODE_SESSION_ID="+flowSession)
	sess, err := mcp.NewClient(&mcp.Implementation{Name: "flow", Version: "1"}, nil).Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

func callMCP(t *testing.T, sess *mcp.ClientSession, tool string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	if res.IsError {
		t.Fatalf("%s failed: %+v", tool, res.Content)
	}
	return res
}

func structuredAs[T any](t *testing.T, res *mcp.CallToolResult) T {
	t.Helper()
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return v
}

type ingested struct {
	JobID  string `json:"jobId"`
	State  string `json:"state"`
	Reason string `json:"reason"`
}

func ingestFiles(t *testing.T, sess *mcp.ClientSession, args map[string]any) ingested {
	t.Helper()
	return structuredAs[ingested](t, callMCP(t, sess, "ingest_files", args))
}

func waitIngest(t *testing.T, sess *mcp.ClientSession, id string, states ...string) importer.JobDetail {
	t.Helper()
	var got importer.JobDetail
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("last status of %s: %+v files %+v", id, got.Job, got.Files)
		}
	})
	testx.WaitUntil(t, waitFor, func() bool {
		got = structuredAs[importer.JobDetail](t, callMCP(t, sess, "ingest_status", map[string]any{"id": id}))
		return slices.Contains(states, got.Job.State)
	})
	return got
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// copyTestdata copies testdata files into a fresh directory under the app's work dir.
func copyTestdata(t *testing.T, app *flowharness.App, dir string, names ...string) string {
	t.Helper()
	dst := filepath.Join(app.Work, dir)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		raw, err := os.ReadFile(testdata(t, n))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, filepath.Base(n)), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dst
}

func TestMcpIngestExternal(t *testing.T) {
	var rules []fakeagent.PromptRule
	rules = append(rules, docRules(t, "slack.json", [2]extracted{
		{fact: "Billing runs on PostgreSQL 16 in production.", evidence: "Billing runs on PostgreSQL 16 in production"},
		{fact: "Invoices go out on the first day of each month.", evidence: "Invoices go out on the first day of each month"},
	})...)
	rules = append(rules, docRules(t, "notes.md", [2]extracted{
		{fact: "Releases ship every second Thursday.", evidence: "Releases ship every second Thursday"},
		{fact: "Hotfixes go through the on-call engineer.", evidence: "Hotfixes go through the on-call engineer"},
	})...)
	app := ingestApp(t, "", rules...)
	dir := copyTestdata(t, app, "export", "ingest/slack.json", "ingest/notes.md")
	sess := memorySession(t, app)

	started := ingestFiles(t, sess, map[string]any{
		"paths":  []string{filepath.Join(dir, "slack.json"), filepath.Join(dir, "notes.md")},
		"origin": "external", "sourceTag": "slack", "scope": "company",
		"groups": []map[string]string{{"kind": "thread", "key": "C123/1700.1", "title": "Billing thread"}},
	})
	if started.JobID == "" {
		t.Fatalf("ingest_files = %+v, want a job id", started)
	}
	done := waitIngest(t, sess, started.JobID, importer.JobDone, importer.JobFailed)
	if done.Job.State != importer.JobDone || done.Job.Totals.Added != 4 || done.Job.Totals.FailedFiles != 0 {
		t.Fatalf("job = %+v, want done with four added", done.Job)
	}
	for rel, kind := range map[string]string{"slack.json": importer.KindJSON, "notes.md": importer.KindMarkdown} {
		var f importer.File
		for _, c := range done.Files {
			if c.RelPath == rel {
				f = c
			}
		}
		if f.Kind != kind || f.State != importer.FileDone || !strings.HasPrefix(f.GroupID, "kg_") || len(f.FactIDs) != 2 {
			t.Fatalf("file %s = %+v, want a done %s file with its document group and two fact ids", rel, f, kind)
		}
	}

	recent, err := app.W.Memory.Recent(ctx)
	if err != nil || len(recent) != 4 {
		t.Fatalf("Recent = %d memories, %v, want 4", len(recent), err)
	}
	for _, m := range recent {
		if m.Origin != memory.OriginExternal || m.SourceTag == nil || *m.SourceTag != "slack" || m.Scope != memory.ScopeCompany ||
			m.SessionID == nil || *m.SessionID != flowSession {
			t.Fatalf("memory = %+v, want external/slack, company scope and session %q", m, flowSession)
		}
	}
	c, err := app.W.Memory.Context(ctx, bridge.MemoryContextArgs{ID: recent[0].LineageID})
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, g := range c.Groups {
		kinds = append(kinds, g.Kind)
	}
	for _, want := range []string{memory.GroupDocument, memory.GroupSession, memory.GroupThread} {
		if !slices.Contains(kinds, want) {
			t.Fatalf("group kinds = %v, want %s among them", kinds, want)
		}
	}

	jobs, err := app.W.MemoryImport.Jobs()
	if err != nil || len(jobs) != 1 || jobs[0].Channel != importer.ChannelMCP || jobs[0].ID != started.JobID {
		t.Fatalf("Jobs = %+v, %v, want the agent's job", jobs, err)
	}
	app.Contract(t, "memory-import", "MemoryImportService.Jobs#mcp", jobs, ingestMask)
	detail := jobDetail(t, app, started.JobID)
	app.Contract(t, "memory-import", "MemoryImportService.Job#mcp", detail, ingestMask)
}

func TestImportProvenanceWritesDoc(t *testing.T) {
	// The finalize agent's reason is not a locator: the store pins the bare "Stated in <path>", so the
	// written doc has no heading and no reason line.
	writesDoc(t, "document", "Stated in billing.md", false)
}

// TestImportLocatorSectionWritesHeading: a section locator survives the gate and reconcile, and the
// written doc turns it into a heading with no reason line.
func TestImportLocatorSectionWritesHeading(t *testing.T) {
	writesDoc(t, "Stated in billing.md, Billing", "Stated in billing.md, Billing", true)
}

func writesDoc(t *testing.T, itemReason, wantReason string, heading bool) {
	t.Helper()
	app := ingestAppReason(t, itemReason, "",
		fakeagent.PromptRule{System: extractSystem, Doc: "billing.md", Emit: testdata(t, "extract-billing.md.json")},
		fakeagent.PromptRule{System: gateSystem, Doc: "billing.md", Emit: testdata(t, "gate-billing-rule.md.json")},
		fakeagent.PromptRule{System: reconcileSystem, Emit: testdata(t, "reconcile.json")})
	dir := copyTestdata(t, app, "docs", "import/billing.md")
	awaiting := createJob(t, app, dir)

	args := bridge.MemoryImportStartArgs{ID: awaiting.Job.ID, Origin: memory.OriginUser, Scope: memory.ScopeProject, Project: billingKey, WriteDoc: true}
	app.Contract(t, "memory-import", "args:MemoryImportService.Start#provenance", args, flowharness.Mask("id"))
	if err := app.W.MemoryImport.Start(args); err != nil {
		t.Fatal(err)
	}
	waitJob(t, app, awaiting.Job.ID, importer.JobDone, importer.JobFailed)
	// The doc is written after the file turns done, so the job can read done a moment before its path lands.
	var done importer.JobDetail
	var f importer.File
	testx.WaitUntil(t, waitFor, func() bool {
		done = jobDetail(t, app, awaiting.Job.ID)
		f = fileNamed(t, done, "billing.md")
		return f.DocPath != "" || f.DocError != ""
	})
	if done.Job.State != importer.JobDone || f.State != importer.FileDone || f.DocError != "" || f.DocPath != filepath.Join(dir, "billing.memory.md") {
		t.Fatalf("job = %+v file %+v, want done and a written doc beside the source", done.Job, f)
	}
	app.Contract(t, "memory-import", "MemoryImportService.Job#writeDoc", done, ingestMask)

	raw, err := os.ReadFile(f.DocPath)
	if err != nil {
		t.Fatal(err)
	}
	doc, inScope, problems := memfmt.ParseDoc(raw)
	if !inScope || len(problems) != 0 {
		t.Fatalf("written doc: in scope %v, problems %v", inScope, problems)
	}
	if doc.Origin != memory.OriginUser || doc.Scope != memory.ScopeProject || doc.Project != billingKey || doc.Group != f.GroupID {
		t.Fatalf("doc header = %+v, want user/project %s in group %s", doc.Provenance, billingKey, f.GroupID)
	}
	if doc.Version != 3 || doc.From != "billing.md" || strings.Contains(string(raw), " fact: ") || strings.Contains(string(raw), "\n  ") {
		t.Fatalf("written doc is not format v3 with from billing.md and no reason lines:\n%s", raw)
	}
	if !strings.Contains(string(raw), "\n## Facts\n") || !strings.Contains(string(raw), "\n## Rules\n") ||
		len(doc.Facts) != 2 || doc.Facts[0].Kind != memory.KindFact || doc.Facts[1].Kind != memory.KindRule {
		t.Fatalf("written doc has a fact under ## Facts and a rule under ## Rules:\n%s", raw)
	}
	if got := strings.Contains(string(raw), "\n### Billing\n"); got != heading {
		t.Fatalf("heading present = %v, want %v:\n%s", got, heading, raw)
	}
	recent, err := app.W.Memory.Recent(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var stored, written []string
	for _, m := range recent {
		stored = append(stored, m.LineageID)
	}
	for _, fact := range doc.Facts {
		written = append(written, fact.ID)
	}
	for _, m := range recent {
		if m.Reason != wantReason {
			t.Fatalf("stored reason %q, want the pinned locator %q", m.Reason, wantReason)
		}
	}
	slices.Sort(stored)
	slices.Sort(written)
	if len(stored) != 2 || !slices.Equal(stored, written) {
		t.Fatalf("stored ids %v, doc ids %v, want the same two", stored, written)
	}
	if _, err := os.Stat(filepath.Join(dir, "billing.md")); err != nil {
		t.Fatalf("source file gone: %v", err)
	}

	// Write, then re-import the written doc: every fact is a noop, whatever its reason was.
	again := createJob(t, app, dir)
	if err := app.W.MemoryImport.Start(bridge.MemoryImportStartArgs{ID: again.Job.ID}); err != nil {
		t.Fatal(err)
	}
	second := waitJob(t, app, again.Job.ID, importer.JobDone, importer.JobFailed)
	g := fileNamed(t, second, "billing.memory.md")
	if g.State != importer.FileDone || g.Added != 0 || g.Updated != 0 || g.Noop != 2 {
		t.Fatalf("re-imported doc file = %+v, want two noops", g)
	}
}

func TestImportFormattedDocParsesOnly(t *testing.T) {
	app := ingestApp(t, "") // no prompt rules: any model call would fail the file
	dir := copyTestdata(t, app, "docs", "memdoc/billing.memory.md")
	docFile := filepath.Join(dir, "billing.memory.md")

	run := func() importer.JobDetail {
		awaiting := createJob(t, app, dir)
		if err := app.W.MemoryImport.Start(bridge.MemoryImportStartArgs{ID: awaiting.Job.ID}); err != nil {
			t.Fatal(err)
		}
		return waitJob(t, app, awaiting.Job.ID, importer.JobDone, importer.JobFailed)
	}
	first := run()
	f := fileNamed(t, first, "billing.memory.md")
	if first.Job.State != importer.JobDone || first.Job.Calls != 0 || f.Kind != importer.KindMemdoc || f.GroupID != "kg_aaaaaaaaaaaa" || f.Added != 3 {
		t.Fatalf("job = %+v file %+v, want a parsed doc: three added, no model call, the header's group", first.Job, f)
	}
	app.Contract(t, "memory-import", "MemoryImportService.Job#memdoc", first, importMask)
	ids := func() []string {
		recent, err := app.W.Memory.Recent(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, m := range recent {
			out = append(out, m.LineageID)
		}
		slices.Sort(out)
		return out
	}
	if got := ids(); !slices.Equal(got, []string{"km_aaaaaaaaaaaa", "km_bbbbbbbbbbbb", "km_cccccccccccc"}) {
		t.Fatalf("stored ids = %v, want the doc's", got)
	}
	recent, _ := app.W.Memory.Recent(ctx)
	for _, m := range recent {
		if wantKind := map[string]string{"km_cccccccccccc": memory.KindRule}[m.LineageID]; m.Kind != cmp.Or(wantKind, memory.KindFact) {
			t.Fatalf("memory %s kind = %q, want the doc category's", m.LineageID, m.Kind)
		}
		if m.Origin != memory.OriginUser || m.Scope != memory.ScopeProject || m.Project == nil || *m.Project != billingKey {
			t.Fatalf("memory = %+v, want the header's user/project provenance", m)
		}
	}

	raw, err := os.ReadFile(docFile)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(raw), "  Stated in the billing runbook.", "  Confirmed by the platform team.", 1)
	if edited == string(raw) {
		t.Fatal("fixture lost its reason line")
	}
	if err := os.WriteFile(docFile, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	second := run()
	f = fileNamed(t, second, "billing.memory.md")
	if second.Job.State != importer.JobDone || second.Job.Calls != 0 || f.Updated != 1 || f.Noop != 2 || f.Added != 0 {
		t.Fatalf("second job = %+v file %+v, want one update and two noops", second.Job, f)
	}
	if got := ids(); !slices.Equal(got, []string{"km_aaaaaaaaaaaa", "km_bbbbbbbbbbbb", "km_cccccccccccc"}) {
		t.Fatalf("stored ids after re-ingest = %v, want the same three", got)
	}
	// The reason-less fact took the locator of its heading, the written one stayed.
	reasons := map[string]string{}
	recent, _ = app.W.Memory.Recent(ctx)
	for _, m := range recent {
		reasons[m.LineageID] = m.Reason
	}
	if reasons["km_aaaaaaaaaaaa"] != "Confirmed by the platform team." || reasons["km_bbbbbbbbbbbb"] != "Stated in billing.memory.md, Storage" || reasons["km_cccccccccccc"] != "Stated in billing.memory.md, Storage" {
		t.Fatalf("reasons = %v", reasons)
	}
}

// TestImportRetiredDocFails: a kira-memory: 1 doc has no compatibility reader; the file fails with the
// retired-format message and stores nothing.
func TestImportRetiredDocFails(t *testing.T) {
	app := ingestApp(t, "")
	dir := filepath.Join(app.Work, "retired")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "billing.memory.md", "---\nkira-memory: 1\ngroup: kg_aaaaaaaaaaaa\ntitle: Billing\norigin: user\nscope: personal\n---\n\n"+
		"km_aaaaaaaaaaaa fact: Invoices are monthly.\nkm_aaaaaaaaaaaa reason: Runbook.\n")
	awaiting := createJob(t, app, dir)
	if err := app.W.MemoryImport.Start(bridge.MemoryImportStartArgs{ID: awaiting.Job.ID}); err != nil {
		t.Fatal(err)
	}
	done := waitJob(t, app, awaiting.Job.ID, importer.JobDone, importer.JobFailed)
	f := fileNamed(t, done, "billing.memory.md")
	if f.State != importer.FileFailed || !strings.Contains(f.Reason, "kira-memory: 1 is the retired P273 format; rewrite it as kira-memory: 3") {
		t.Fatalf("file = %+v, want failed with the retired message", f)
	}
	if recent, _ := app.W.Memory.Recent(ctx); len(recent) != 0 {
		t.Fatalf("stored %d facts from a retired doc", len(recent))
	}
}

// TestImportRetiredV2DocFails: a kira-memory: 2 doc fails the file with the v2 message and stores nothing.
func TestImportRetiredV2DocFails(t *testing.T) {
	app := ingestApp(t, "")
	dir := filepath.Join(app.Work, "retired2")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "billing.memory.md", "---\nkira-memory: 2\ngroup: kg_aaaaaaaaaaaa\ntitle: Billing\norigin: user\nscope: personal\n---\n\n## Storage\n\n"+
		"- km_aaaaaaaaaaaa Invoices are monthly.\n")
	awaiting := createJob(t, app, dir)
	if err := app.W.MemoryImport.Start(bridge.MemoryImportStartArgs{ID: awaiting.Job.ID}); err != nil {
		t.Fatal(err)
	}
	done := waitJob(t, app, awaiting.Job.ID, importer.JobDone, importer.JobFailed)
	f := fileNamed(t, done, "billing.memory.md")
	if f.State != importer.FileFailed || !strings.Contains(f.Reason, "kira-memory: 2 is retired (P297)") {
		t.Fatalf("file = %+v, want failed with the v2 message", f)
	}
	if recent, _ := app.W.Memory.Recent(ctx); len(recent) != 0 {
		t.Fatalf("stored %d facts from a retired doc", len(recent))
	}
}

func TestImportVerifiesQuestionableFacts(t *testing.T) {
	rewritten := "Deploys run from the main branch."
	app := ingestApp(t, "",
		fakeagent.PromptRule{System: extractSystem, Doc: "ops.md", Emit: extractAnswer(t,
			extracted{fact: "Deploys run from there.", evidence: "run from the main branch", unresolved: "Which place is meant by there?"},
			extracted{fact: "Pagers rotate every Monday.", evidence: "a quote the document never contains"})},
		fakeagent.PromptRule{System: verifySystem, Emit: answer(t, map[string]any{"verdicts": []map[string]string{
			{"id": "1.1", "verdict": "rewritten", "fact": rewritten, "evidence": "run from the main branch", "why": "The file names the branch."},
			{"id": "1.2", "verdict": "unsupported", "fact": "", "evidence": "", "why": "No pager rotation in the file."},
		}})},
		fakeagent.PromptRule{System: gateSystem, Doc: "ops.md", Emit: gateAnswer(t,
			gateItem{verdict: "accept", fact: rewritten}, gateItem{verdict: "accept", fact: "Nothing else is decided in ops.md."})},
		fakeagent.PromptRule{System: reconcileSystem, Doc: "ops.md", Emit: reconcileAdds(t, 2)})
	dir := filepath.Join(app.Work, "ops")
	writeFiles(t, dir, map[string]string{"ops.md": "# Ops\n\nDeploys run from the main branch. Nothing else is decided here.\n"})
	awaiting := createJob(t, app, dir)
	if err := app.W.MemoryImport.Start(bridge.MemoryImportStartArgs{ID: awaiting.Job.ID}); err != nil {
		t.Fatal(err)
	}
	done := waitJob(t, app, awaiting.Job.ID, importer.JobDone, importer.JobFailed)
	f := fileNamed(t, done, "ops.md")
	if done.Job.State != importer.JobDone || f.Verified != 2 || len(f.Dropped) != 1 || !strings.Contains(f.Dropped[0].Why, "Not supported by the source") ||
		f.Dropped[0].Fact != "Pagers rotate every Monday." {
		t.Fatalf("job = %+v file %+v, want two verified and the unsupported fact dropped with why", done.Job, f)
	}
	app.Contract(t, "memory-import", "MemoryImportService.Job#verified", done, ingestMask)

	prompts, _ := filepath.Glob(filepath.Join(app.FakeDir, "*.prompt"))
	var finalize string
	for _, p := range prompts {
		raw, _ := os.ReadFile(p)
		if strings.Contains(string(raw), `"provenance"`) {
			finalize = string(raw)
		}
	}
	if !strings.Contains(finalize, rewritten) || strings.Contains(finalize, "Pagers rotate") || strings.Contains(finalize, "Deploys run from there") {
		t.Fatalf("finalize input = %q, want the rewritten fact and neither the unsupported nor the original", finalize)
	}
}

func TestExternalIngestNeverChallenges(t *testing.T) {
	asked := "Which service is meant by billing?"
	app := ingestApp(t, answer(t, map[string]any{
		"unresolved": []any{},
		"dropped":    []map[string]string{{"fact": "Billing is paused.", "why": "Not stored (nobody can answer): " + asked}},
	}),
		fakeagent.PromptRule{System: extractSystem, Doc: "export.md", Emit: extractAnswer(t,
			extracted{fact: "Billing is paused.", evidence: "Billing is paused"},
			extracted{fact: "Staging deploys at 09:00.", evidence: "Staging deploys at 09:00"})},
		fakeagent.PromptRule{System: gateSystem, Doc: "export.md", Emit: gateAnswer(t,
			gateItem{verdict: "challenge", questions: []string{asked}}, gateItem{verdict: "accept", fact: "Staging deploys at 09:00."})},
		fakeagent.PromptRule{System: reconcileSystem, Doc: "export.md", Emit: reconcileAdds(t, 1)})
	dir := filepath.Join(app.Work, "export")
	writeFiles(t, dir, map[string]string{"export.md": "# Export\n\nBilling is paused.\nStaging deploys at 09:00.\n"})
	sess := memorySession(t, app)
	started := ingestFiles(t, sess, map[string]any{"paths": []string{dir}, "origin": "external", "sourceTag": "slack", "scope": "personal"})
	done := waitIngest(t, sess, started.JobID, importer.JobDone, importer.JobFailed)
	f := fileNamed(t, done, "export.md")
	if done.Job.State != importer.JobDone || len(f.Unresolved) != 0 || len(f.Dropped) != 1 || f.Dropped[0].Fact != "Billing is paused." || done.Job.Totals.Dropped != 1 {
		t.Fatalf("job = %+v file %+v, want the challenged fact dropped and nothing unresolved", done.Job, f)
	}
	recent, err := app.W.Memory.Recent(ctx)
	if err != nil || len(recent) != 1 || recent[0].Fact != "Staging deploys at 09:00." {
		t.Fatalf("Recent = %+v, %v, want only the accepted fact", recent, err)
	}
	stored, err := os.ReadFile(filepath.Join(app.FakeDir, "mcp-store_memory-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Status     string `json:"status"`
		Challenges []struct {
			Questions []string `json:"questions"`
		} `json:"challenges"`
	}
	if err := json.Unmarshal(stored, &res); err != nil {
		t.Fatal(err)
	}
	if res.Status != "stored" || len(res.Challenges) != 1 || !slices.Equal(res.Challenges[0].Questions, []string{asked}) {
		t.Fatalf("store_memory result = %s, want status stored with the unanswered question reported", stored)
	}
}

func TestIngestAwaitsOverChunkCap(t *testing.T) {
	hold := filepath.Join(t.TempDir(), "never")
	app := ingestApp(t, "", fakeagent.PromptRule{System: extractSystem, WaitFile: hold, Emit: extractAnswer(t)})
	dir := filepath.Join(app.Work, "big")
	files := map[string]string{}
	for _, name := range []string{"big1.md", "big2.md"} {
		var b strings.Builder
		for i := range 170 {
			fmt.Fprintf(&b, "## %s %d\n\n%s\n\n", name, i, strings.Repeat(name+" ", 700))
		}
		files[name] = b.String()
	}
	writeFiles(t, dir, files)
	sess := memorySession(t, app)

	started := ingestFiles(t, sess, map[string]any{"paths": []string{dir}, "origin": "external", "sourceTag": "export", "scope": "personal"})
	waiting := waitIngest(t, sess, started.JobID, importer.JobAwaiting)
	if !strings.Contains(waiting.Job.Reason, "Needs confirmation") || waiting.Job.Estimate.Chunks <= 300 {
		t.Fatalf("job = %+v, want awaiting with the confirmation reason over 300 chunks", waiting.Job)
	}
	if err := app.W.MemoryImport.Start(bridge.MemoryImportStartArgs{ID: started.JobID}); err != nil {
		t.Fatal(err)
	}
	if got := waitJob(t, app, started.JobID, importer.JobRunning); got.Job.Reason != "" {
		t.Fatalf("job after Start = %+v, want running", got.Job)
	}
	if err := app.W.MemoryImport.Cancel(bridge.MemoryImportJobArgs{ID: started.JobID}); err != nil {
		t.Fatal(err)
	}
}

// A doc header claiming more than the agent's job holds must wait for the user: header provenance
// wins on apply, so an unattended start would let an agent raise its own facts to user grade.
func TestIngestAwaitsOnMemdocHeaderClaim(t *testing.T) {
	app := ingestApp(t, "")
	dir := copyTestdata(t, app, "docs", "memdoc/billing.memory.md")
	sess := memorySession(t, app)

	started := ingestFiles(t, sess, map[string]any{"paths": []string{dir}, "origin": "agent", "scope": "project", "project": billingKey})
	waiting := waitIngest(t, sess, started.JobID, importer.JobAwaiting)
	if !strings.Contains(waiting.Job.Reason, "Needs confirmation in Kira Space: 1 kira-memory docs claim") {
		t.Fatalf("job = %+v, want awaiting with the header-claim reason", waiting.Job)
	}
	app.Contract(t, "memory-import", "MemoryImportService.Job#claims", waiting, ingestMask)
	if recent, err := app.W.Memory.Recent(ctx); err != nil || len(recent) != 0 {
		t.Fatalf("Recent = %+v, %v, want nothing stored before the user confirms", recent, err)
	}
	if err := app.W.MemoryImport.Start(bridge.MemoryImportStartArgs{ID: started.JobID}); err != nil {
		t.Fatal(err)
	}
	if done := waitJob(t, app, started.JobID, importer.JobDone, importer.JobFailed); done.Job.State != importer.JobDone {
		t.Fatalf("job after Start = %+v, want done", done.Job)
	}
}

func TestIngestIdempotentCall(t *testing.T) {
	hold := filepath.Join(t.TempDir(), "release")
	app := ingestApp(t, "", fakeagent.PromptRule{System: extractSystem, Doc: "notes.md", WaitFile: hold, Emit: extractAnswer(t)})
	dir := copyTestdata(t, app, "notes", "ingest/notes.md")
	sess := memorySession(t, app)
	args := map[string]any{"paths": []string{dir}, "origin": "external", "sourceTag": "notes", "scope": "personal"}

	first := ingestFiles(t, sess, args)
	second := ingestFiles(t, sess, args)
	if first.JobID == "" || first.JobID != second.JobID {
		t.Fatalf("calls returned jobs %q and %q, want one", first.JobID, second.JobID)
	}
	if jobs, err := app.W.MemoryImport.Jobs(); err != nil || len(jobs) != 1 {
		t.Fatalf("Jobs = %+v, %v, want one", jobs, err)
	}
	waitJob(t, app, first.JobID, importer.JobRunning)
	if err := app.W.MemoryImport.Cancel(bridge.MemoryImportJobArgs{ID: first.JobID}); err != nil {
		t.Fatal(err)
	}
}

// The runner is its own process: closing the app's memory service leaves a running job alone.
func TestRunnerOutlivesApp(t *testing.T) {
	hold := filepath.Join(t.TempDir(), "release")
	app := ingestApp(t, "",
		fakeagent.PromptRule{System: extractSystem, Doc: "billing.md", WaitFile: hold, Emit: testdata(t, "extract-billing.md.json")},
		fakeagent.PromptRule{System: gateSystem, Doc: "billing.md", Emit: testdata(t, "gate-billing.md.json")},
		fakeagent.PromptRule{System: reconcileSystem, Emit: testdata(t, "reconcile.json")})
	dir := copyTestdata(t, app, "docs", "import/billing.md")
	awaiting := createJob(t, app, dir)
	if err := app.W.MemoryImport.Start(bridge.MemoryImportStartArgs{ID: awaiting.Job.ID}); err != nil {
		t.Fatal(err)
	}
	waitJob(t, app, awaiting.Job.ID, importer.JobRunning)

	app.Restart() // closes the memory service and the engine
	if err := os.WriteFile(hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	done := waitJob(t, app, awaiting.Job.ID, importer.JobDone)
	if done.Job.Totals.Added != 2 {
		t.Fatalf("job = %+v, want two added by the runner that outlived the app", done.Job)
	}
}
