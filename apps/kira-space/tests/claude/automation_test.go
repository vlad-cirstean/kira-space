package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// smartRun creates a smart script with the given tools, runs it and waits for it to leave running.
func smartRun(t *testing.T, app *realApp, prompt string, tools []string, mcp ...scripts.MCPChoice) scriptruns.Run {
	t.Helper()
	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
		Name: "real", Kind: scripts.KindSmart, Command: prompt, Color: "blue",
		Smart: &scripts.Smart{Model: "haiku", MaxBudgetUSD: 0.05, Timeout: "3m", Tools: tools, MCP: mcp},
	}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	args := scriptruns.RunArgs{ScriptID: rec.ID}
	pv, err := app.W.ScriptRuns.Preview(args)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	started, err := app.W.ScriptRuns.Start(scriptruns.StartArgs{RunArgs: args, Hash: pv.Hash})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	var run scriptruns.Run
	testx.WaitUntil(t, testTimeout, func() bool {
		run, _ = app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: started.RunID})
		return run.State != "running"
	})
	return run
}

// TestSmartScriptRun runs smart scripts through the real claude: a reported success with a cost,
// and a prompt that asks for a tool the script was not given.
func TestSmartScriptRun(t *testing.T) {
	t.Run("reports done with a cost", func(t *testing.T) {
		run := smartRun(t, newRealApp(t), "Reply with the word ok.", []string{"Read", "Grep", "Glob"})
		o := run.Outcome
		if run.State != "done" || o == nil || !o.Reported || o.CostUSD == nil || *o.CostUSD <= 0 {
			t.Fatalf("run = %s, outcome = %+v, want done, reported, cost > 0", run.State, o)
		}
		t.Logf("PASS reports done: cost %.4f USD", *o.CostUSD)
	})

	t.Run("a tool outside the allowlist does not run", func(t *testing.T) {
		start := time.Now()
		run := smartRun(t, newRealApp(t), "Use the Bash tool to create a file named marker.txt containing hi. If you cannot, say so.", []string{"Read", "Grep", "Glob"})
		if run.Outcome == nil || !run.Outcome.Reported {
			t.Fatalf("outcome = %+v, want a reported status", run.Outcome)
		}
		if _, err := os.Stat(filepath.Join(run.Cwd, "marker.txt")); err == nil {
			t.Fatal("marker.txt exists: Bash ran without being allowed")
		}
		t.Logf("PASS no marker.txt: status %s in %s", run.Outcome.Status, time.Since(start).Round(time.Second))
	})

	t.Run("in task worktree", func(t *testing.T) {
		f := newAdeFixture(t, nil)
		f.saveWorkflow(t, "flow", agentStepYAML)
		task := f.createTask(t, "flow")
		if r := f.startRun(t, task, "feat/api-auto"); r.State != "done" {
			t.Fatalf("step run = %s, want done", r.State)
		}
		b, err := f.W.AdeTask.Board(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var branchID string
		for _, br := range b.Branches {
			if br.TaskID == task.ID && br.CodeRepoID == f.repoID {
				branchID = br.ID
			}
		}
		rec, err := f.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
			Name: "real", Kind: scripts.KindSmart, Color: "blue", UseAdeDir: true,
			Command: "Call task_info, then call finish_step with status done and a one-line summary.",
			Smart:   &scripts.Smart{Model: "haiku", MaxBudgetUSD: 0.05, Timeout: "3m", Tools: []string{"Read"}},
		}})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		args := scriptruns.RunArgs{ScriptID: rec.ID, TaskID: task.ID, BranchID: branchID}
		pv, err := f.W.ScriptRuns.Preview(args)
		if err != nil {
			t.Fatalf("Preview: %v", err)
		}
		started, err := f.W.ScriptRuns.Start(scriptruns.StartArgs{RunArgs: args, Hash: pv.Hash})
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		var run scriptruns.Run
		testx.WaitUntil(t, testTimeout, func() bool {
			run, _ = f.W.ScriptRuns.Get(scriptruns.IDArgs{ID: started.RunID})
			return run.State != "running"
		})
		o := run.Outcome
		if run.State != "done" || o == nil || !o.Reported || o.CostUSD == nil || *o.CostUSD <= 0 {
			t.Fatalf("run = %s, outcome = %+v, want done, reported, cost > 0", run.State, o)
		}
		if !strings.HasPrefix(run.Cwd, filepath.Join(f.Home, "wt")) || run.TaskID != task.ID || run.BranchID != branchID {
			t.Fatalf("run cwd %q task %q branch %q, want the task worktree", run.Cwd, run.TaskID, run.BranchID)
		}
		if !strings.Contains(strings.Join(run.Tools.AllowedTools, " "), "mcp__kira-space__task_info") {
			t.Fatalf("allowed tools = %v, want the Space tools", run.Tools.AllowedTools)
		}
		t.Logf("PASS in task worktree: cost %.4f USD in %s", *o.CostUSD, run.Cwd)
	})
	studioSmartScriptMCP(t)
}
