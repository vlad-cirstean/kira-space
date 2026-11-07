package claude

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/internal/kirapaths"
	"github.com/kirathecat/kira-studio/internal/memory"
	"github.com/kirathecat/kira-studio/internal/memory/importer"
)

// noLauncher stands in for the runner process: the test runs its own runner in a goroutine.
type noLauncher struct{}

func (noLauncher) Launch() error { return nil }

// runRunner runs r until ctx ends. Engine.Create probes the runner lock for an instant; a runner
// that starts in that window gets ErrRunnerBusy, and noLauncher never starts another, so retry.
func runRunner(ctx context.Context, r *importer.Runner) {
	for {
		if err := r.Run(ctx); !errors.Is(err, importer.ErrRunnerBusy) || ctx.Err() != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// The finalize agent's MCP server is this test binary re-executed as `memory-mcp` (see TestMain).
// The runner runs in this process; the production runner process is covered by TestMemoryIngestThroughClaude.
func TestMemoryImport(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("real claude tests: no claude on PATH")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ms := memory.OpenDefault()
	defer ms.Close()
	svc := memory.NewService(ms, newAgentRunner(), memory.ServiceOptions{})
	defer svc.Close()
	agent := importer.ModelAgent{Runner: newAgentRunner(), Executable: exe, SpaceHome: kirapaths.SpaceHome(),
		Env: map[string]string{"KIRA_TEST_MEMORY_MCP": "1"}}
	// A small budget forces billing.md's second section into its own chunk, so "It restarts..."
	// reaches step 1 without the service name; releases.md's last sentence leaves its references open,
	// which sends its facts through the cross-check.
	runner, err := importer.NewRunner(ms, importer.RunnerOptions{Agent: agent, Docs: svc,
		Budget: importer.Budget{Target: 150, Max: 250, MinFill: 40, MinTail: 1, Context: 60}})
	if err != nil {
		t.Fatal(err)
	}
	runCtx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); runRunner(runCtx, runner) }()
	defer func() { stop(); <-done }()
	e, err := importer.Open(ms, importer.Options{Launcher: noLauncher{}})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	begin := time.Now()
	job, err := e.Create(importer.CreateArgs{Paths: []string{"testdata/smoke"}})
	if err != nil {
		t.Fatal(err)
	}
	waitFor := func(want string) importer.JobDetail {
		t.Helper()
		deadline := time.Now().Add(20 * time.Minute)
		for time.Now().Before(deadline) {
			d, err := e.Job(job.ID)
			if err != nil {
				t.Fatal(err)
			}
			if d.Job.State == want {
				return d
			}
			if d.Job.State == importer.JobFailed || d.Job.State == importer.JobPaused {
				t.Fatalf("job %s: %s", d.Job.State, d.Job.Reason)
			}
			time.Sleep(500 * time.Millisecond)
		}
		t.Fatalf("job never reached %s", want)
		return importer.JobDetail{}
	}
	d := waitFor(importer.JobAwaiting)
	t.Logf("estimate: %+v", d.Job.Estimate)
	if err := e.Start(job.ID, importer.StartArgs{}); err != nil {
		t.Fatal(err)
	}
	d = waitFor(importer.JobDone)
	verified := 0
	for _, f := range d.Files {
		verified += f.Verified
	}
	if verified < 1 {
		t.Errorf("verified = %d, want at least one fact cross-checked against its source file", verified)
	}
	t.Logf("done in %s: calls=%d cost=$%.3f totals=%+v", time.Since(begin).Round(time.Second), d.Job.Calls, d.Job.CostUSD, d.Job.Totals)
	for _, f := range d.Files {
		t.Logf("file %s: %s chunks=%d facts=%d added=%d updated=%d noop=%d cost=$%.3f unresolved=%+v dropped=%+v reason=%q",
			f.RelPath, f.State, f.ChunkCount, f.FactCount, f.Added, f.Updated, f.Noop, f.CostUSD, f.Unresolved, f.Dropped, f.Reason)
		if f.State != importer.FileDone {
			t.Errorf("%s is %s", f.RelPath, f.State)
		}
	}
	mems, err := svc.Recent(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	imported, restoreRule := 0, false
	for _, m := range mems {
		if m.Kind == memory.KindRule && strings.Contains(m.Fact, "backup") && strings.Contains(m.Fact, "production") {
			restoreRule = true
		}
		h, err := svc.History(context.Background(), m.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, ev := range h.Events {
			if ev.Source == memory.SourceImport && ev.Action == memory.ActionAdd {
				imported++
			}
		}
		if m.Origin != memory.OriginExternal || m.SourceTag == nil || *m.SourceTag != "doc" || m.Scope != memory.ScopePersonal {
			t.Errorf("memory %s = %+v, want external/doc in personal scope", m.ID, m)
		}
		c, err := svc.Context(context.Background(), m.ID, 0)
		if err != nil || !slices.ContainsFunc(c.Groups, func(g memory.GroupContext) bool { return g.Kind == memory.GroupDocument }) {
			t.Errorf("memory %s context = %+v, %v, want a document group", m.ID, c, err)
		}
		t.Logf("memory: %s\n    reason: %s", m.Fact, m.Reason)
	}
	if imported == 0 {
		t.Error("no add event with source import")
	}
	if !restoreRule {
		t.Error("the restore rule of billing.md is not stored with kind rule")
	}
}
