package appflow_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/metrics"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Backend half of the status bar metrics scenario: a running agent shows in Agents, totals are
// App + Agents + Scripts, and Agents empties once the run ends. The UI half is window-chrome.spec.ts
// (shared with TestAppMetricsCountsScripts and TestSQLitePreconnectCountsAsScript).
func TestAppMetricsCountsAgents(t *testing.T) {
	app := flowharness.New(t)
	hold := filepath.Join(t.TempDir(), "hold")
	claude(app, map[string][]fakeagent.Action{"*": {{WaitFile: hold}}})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)

	entry, err := app.W.AdeTask.SaveWorkflowYaml(ctx, adewire.SaveWorkflowYamlArgs{FileName: "flow.yaml", Yaml: agentFlow})
	if err != nil || entry.Error != nil {
		t.Fatalf("SaveWorkflowYaml: %v %+v", err, entry.Error)
	}
	task, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Fix login", CodeRepoIDs: []string{rec.ID}, WorkflowID: "flow"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := app.W.AdeTask.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, br := range b.Branches {
		if br.TaskID == task.ID {
			names[br.ID] = "feat/api-work"
		}
	}
	mark := app.Events.Mark()
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: names}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		raw, _ := os.ReadFile(filepath.Join(app.FakeDir, "api-1.args"))
		return len(raw) > 0
	})

	s := app.WaitMetrics(t, mark, func(s metrics.Sample) bool {
		return s.Agents.ProcessCount >= 1 && s.Agents.MemoryBytes > 0
	}, waitFor)
	flowharness.CheckMetricsTotals(t, s)
	if s.Scripts.ProcessCount != 0 {
		t.Fatalf("agent filed under Scripts: %+v", s)
	}

	if err := os.WriteFile(hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	mark = app.Events.Mark()
	app.WaitMetrics(t, mark, func(s metrics.Sample) bool { return s.Agents.ProcessCount == 0 }, waitFor)
}
