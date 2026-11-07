package claude

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/internal/memory"
	"github.com/kirathecat/kira-studio/internal/memory/importer"
)

const slackExport = `[
  {"user": "ana", "ts": "1700000001.000100", "text": "The billing-api service listens on port 8080 in production."},
  {"user": "bo", "ts": "1700000002.000200", "text": "Invoices are archived to cold storage after ninety days."}
]
`

// TestMemoryIngestThroughClaude has the real claude call ingest_files on a repo's docs folder and a
// Slack export, with only the kira-memory server. The runner is the production process: this test
// binary re-executed as `memory-ingest` (flowharness.Main serves it). Claude calls ingest_status
// once; the test then waits for the job through the app, because a model polling loop costs tokens
// for nothing.
func TestMemoryIngestThroughClaude(t *testing.T) {
	app := newRealApp(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(app.Work, "billing")
	for _, args := range [][]string{
		{"init", "-q", repo},
		{"-C", repo, "remote", "add", "origin", "https://github.com/acme/billing.git"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	docs := filepath.Join(repo, "docs")
	for _, name := range []string{"billing.md", "releases.md"} {
		raw, err := os.ReadFile(filepath.Join("testdata", "smoke", name))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(docs, name), string(raw))
	}
	slack := filepath.Join(repo, "slack.json")
	writeFile(t, slack, slackExport)

	cfg := filepath.Join(t.TempDir(), "mcp.json")
	writeFile(t, cfg, string(mustRaw(map[string]any{"mcpServers": map[string]any{
		memoryServer: map[string]any{"command": exe, "args": []string{"memory-mcp"}, "env": map[string]string{
			"KIRA_SPACE_HOME": app.SpaceHome, "HOME": app.Home, "PATH": os.Getenv("PATH"),
		}},
	}})))
	tools := "mcp__" + memoryServer + "__ingest_files,mcp__" + memoryServer + "__ingest_status"
	out, _ := app.claudeP(t, repo, nil,
		"Call ingest_files once with paths ["+docs+", "+slack+"], origin external, sourceTag slack, scope project and no project argument. "+
			"Then call ingest_status once with the jobId from the result. Reply with the word done. Do not read the files yourself.",
		"--strict-mcp-config", "--mcp-config", cfg, "--allowedTools", tools,
		"--output-format", "stream-json", "--verbose")
	msgs := parseStream(out)
	uses := toolUses(msgs, "mcp__"+memoryServer+"__ingest_files")
	if len(uses) == 0 {
		t.Fatalf("no ingest_files tool use in the stream:\n%s", tail(out, 1500))
	}
	if text, isErr, ok := toolResult(msgs, uses[0].ID); !ok || isErr {
		t.Fatalf("ingest_files result missing or an error (ok=%v, text=%q)", ok, text)
	}
	if len(toolUses(msgs, "Read")) != 0 {
		t.Errorf("claude read the files itself:\n%s", tail(out, 1500))
	}

	jobs, err := app.W.MemoryImport.Jobs()
	if err != nil || len(jobs) != 1 || jobs[0].Channel != importer.ChannelMCP {
		t.Fatalf("Jobs = %+v, %v, want the agent's one job", jobs, err)
	}
	id := jobs[0].ID
	var d importer.JobDetail
	deadline := time.Now().Add(15 * time.Minute)
	for time.Now().Before(deadline) {
		if d, err = jobDetail(app, id); err != nil {
			t.Fatal(err)
		}
		if d.Job.State == importer.JobDone || d.Job.State == importer.JobFailed || d.Job.State == importer.JobPaused {
			break
		}
		time.Sleep(time.Second)
	}
	t.Logf("job %s: calls=%d cost=$%.3f totals=%+v", d.Job.State, d.Job.Calls, d.Job.CostUSD, d.Job.Totals)
	if d.Job.State != importer.JobDone {
		t.Fatalf("job = %+v, want done", d.Job)
	}
	for _, f := range d.Files {
		t.Logf("file %s: %s kind=%s added=%d verified=%d dropped=%+v", f.RelPath, f.State, f.Kind, f.Added, f.Verified, f.Dropped)
	}
	mems, err := app.W.Memory.Recent(ctx)
	if err != nil || len(mems) == 0 {
		t.Fatalf("Recent = %+v, %v, want the ingested facts", mems, err)
	}
	for _, m := range mems {
		if m.Origin != memory.OriginExternal || m.SourceTag == nil || *m.SourceTag != "slack" || m.Scope != memory.ScopeProject ||
			m.Project == nil || *m.Project != "github.com/acme/billing" {
			t.Errorf("memory %s = %+v, want external/slack in project github.com/acme/billing", m.ID, m)
		}
	}
}

func jobDetail(app *realApp, id string) (importer.JobDetail, error) {
	return app.W.MemoryImport.Job(bridge.MemoryImportJobArgs{ID: id})
}
