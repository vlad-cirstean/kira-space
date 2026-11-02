package claudeflow_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func TestClaudeSettingsUntouched(t *testing.T) {
	app := flowharness.New(t)
	repo := app.NewRepo("proj")
	repo.Commit("base", map[string]string{"a.txt": "a\n"})
	claude(app, map[string][]fakeagent.Action{"*": {{}}})
	seedClaudeConfig(t, app.Home, repo.Dir)
	before := snapshot(t, app.Home, repo.Dir)

	check := func(t *testing.T, flow string) {
		t.Helper()
		if d := diff(before, snapshot(t, app.Home, repo.Dir)); len(d) > 0 {
			t.Fatalf("%s changed Claude Code config: %s", flow, strings.Join(d, "; "))
		}
	}
	hooksFile := func(t *testing.T, argv []string) {
		t.Helper()
		settings := flagValues(argv, "--settings")
		if len(settings) != 1 || !filepath.IsAbs(settings[0]) || strings.HasPrefix(settings[0], app.Home) {
			t.Fatalf("agent launch --settings = %q, want one absolute path outside the user's home", settings)
		}
	}

	t.Run("terminal_agent_session", func(t *testing.T) {
		argv := agentArgv(t, app, func() { openAgent(t, app, "agent-1", repo.Dir) })
		hooksFile(t, argv)
		check(t, "terminal_agent_session")
	})

	t.Run("ade_session_start", func(t *testing.T) {
		rec := importRepo(t, app, repo.Dir)
		saveWorkflow(t, app, "flow", "id: flow\nname: Flow\nsteps:\n  - id: work\n    name: Work\n    kind: manual\n    status: In progress\n    session: true\n")
		task, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Fix login", CodeRepoIDs: []string{rec.ID}, WorkflowID: "flow"})
		if err != nil {
			t.Fatal(err)
		}
		launch, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task.ID})
		if err != nil {
			t.Fatal(err)
		}
		argv := agentArgv(t, app, func() {
			if _, err := app.W.Terminal.Open(terminal.OpenArgs{
				TerminalID: launch.TerminalID, WindowKey: window, Cwd: launch.Cwd, Cols: 100, Rows: 30,
				Command: launch.Command, LaunchKind: terminal.LaunchKindAgent,
			}); err != nil {
				t.Fatal(err)
			}
		})
		hooksFile(t, argv)
		check(t, "ade_session_start")
	})

	isolated := func(t *testing.T, argv []string) {
		t.Helper()
		if got := flagValues(argv, "--setting-sources"); len(got) != 1 || got[0] != "" {
			t.Fatalf("--setting-sources = %q, want one empty value", got)
		}
		for _, p := range flagValues(argv, "--settings") {
			if strings.HasPrefix(p, app.Home) {
				t.Fatalf("--settings %q points into the user's home", p)
			}
		}
	}

	t.Run("smart_script_run", func(t *testing.T) {
		claude(app, map[string][]fakeagent.Action{"*": {{Name: "done"}}})
		rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
			Name: "ask", Kind: scripts.KindSmart, Command: "Reply ok.", Color: "blue",
			Smart: &scripts.Smart{Tools: []string{"Read"}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		args := scriptruns.RunArgs{ScriptID: rec.ID}
		pv, err := app.W.ScriptRuns.Preview(args)
		if err != nil {
			t.Fatal(err)
		}
		argv := agentArgv(t, app, func() {
			started, err := app.W.ScriptRuns.Start(scriptruns.StartArgs{RunArgs: args, Hash: pv.Hash})
			if err != nil {
				t.Fatal(err)
			}
			testx.WaitUntil(t, waitFor, func() bool {
				run, _ := app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: started.RunID})
				return run.State == "done"
			})
		})
		isolated(t, argv)
		check(t, "smart_script_run")
	})

	t.Run("ade_smart_step", func(t *testing.T) {
		claude(app, map[string][]fakeagent.Action{"*": {{Name: "done"}}})
		bare := app.NewBare("api-remote")
		seed := app.NewRepo("api-seed")
		seed.Commit("base", map[string]string{"base.txt": "base\n"})
		bare.PushFrom(seed)
		clone := bare.Clone(filepath.Join(app.Work, "api"))
		rec := importRepo(t, app, clone.Dir)
		if _, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
			Name: "lint", Kind: scripts.KindSmart, Command: "Lint {branch}.", Color: "blue",
			Smart: &scripts.Smart{Tools: []string{"Read"}}, UseAdeDir: true,
		}}); err != nil {
			t.Fatal(err)
		}
		saveWorkflow(t, app, "smartflow", "id: smartflow\nname: Flow\nsteps:\n  - id: s1\n    name: s1\n    kind: agent\n    runs_on: each repo\n    timeout: 2m\n    smart_script: lint\n")
		task, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Lint it", CodeRepoIDs: []string{rec.ID}, WorkflowID: "smartflow"})
		if err != nil {
			t.Fatal(err)
		}
		b, err := app.W.AdeTask.Board(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var branchID string
		for _, br := range b.Branches {
			if br.TaskID == task.ID {
				branchID = br.ID
			}
		}
		argv := agentArgv(t, app, func() {
			if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{branchID: "feat/lint"}}); err != nil {
				t.Fatal(err)
			}
			testx.WaitUntil(t, waitFor, func() bool {
				b, _ := app.W.AdeTask.Board(ctx)
				for _, tk := range b.Tasks {
					if tk.ID == task.ID {
						for _, r := range tk.Runs {
							if r.StepID == "s1" && r.State == "done" {
								return true
							}
						}
					}
				}
				return false
			})
		})
		isolated(t, argv)
		check(t, "ade_smart_step")
	})

	app.Restart()
	check(t, "app restart")
}
