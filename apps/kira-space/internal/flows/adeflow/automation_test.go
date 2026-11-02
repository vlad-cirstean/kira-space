package adeflow_test

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// autoFixture is a task on two started repos (api, web) plus a third not yet created (docs), so a
// script has two real branches and one draft to choose from.
type autoFixture struct {
	app              *flowharness.App
	task             string
	api, web, docs   string
	apiBr, webBr     adewire.Branch
	docsBr           adewire.Branch
	gateOne, gateTwo string
}

func (f *autoFixture) refresh(t *testing.T) {
	t.Helper()
	b := board(t, f.app)
	f.apiBr, f.webBr = branchOf(t, b, f.task, f.api), branchOf(t, b, f.task, f.web)
	for _, br := range b.Branches {
		if br.TaskID == f.task && br.CodeRepoID == f.docs {
			f.docsBr = br
		}
	}
}

// newAutoFixture builds the app. Attempt 1 of each repo finishes the first step; attempt 2 and 3 of
// api wait on the returned gate files, so a test controls when a script or a step ends.
func newAutoFixture(t *testing.T, extraStep string, draft bool, opts ...flowharness.Opt) *autoFixture {
	t.Helper()
	app := flowharness.New(t, opts...)
	gates := t.TempDir()
	f := &autoFixture{app: app, gateOne: filepath.Join(gates, "one"), gateTwo: filepath.Join(gates, "two")}
	app.Scenario(fakeagent.Scenario{Claude: map[string][]fakeagent.Action{
		"api": {{Name: "done"}, {Name: "done", WaitFile: f.gateOne}, {Name: "done", WaitFile: f.gateTwo}},
		"web": {{Name: "done"}},
		"*":   {{Name: "done"}},
	}})
	ids := map[string]string{}
	for _, name := range []string{"api", "web", "docs"} {
		repo, _ := cloneWithMain(app, filepath.Join(app.Work, name))
		ids[name] = importRepo(t, app, repo.Dir).ID
	}
	f.api, f.web, f.docs = ids["api"], ids["web"], ids["docs"]
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")+extraStep+manualStep))
	task := createTask(t, app, "Fix login", "flow", f.api, f.web)
	f.task = task.ID
	b := board(t, app)
	names := map[string]string{
		branchOf(t, b, f.task, f.api).ID: "feat/api-login",
		branchOf(t, b, f.task, f.web).ID: "feat/web-login",
	}
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.task, BranchNames: names}); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	waitRun(t, app, f.task, "one", "done")
	// waitRun sees one branch's run; the other branch must be done too, or it still counts as busy.
	testx.WaitUntil(t, waitFor, func() bool {
		n := 0
		for _, r := range taskOf(t, board(t, app), f.task).Runs {
			if r.StepID == "one" {
				if r.State != "done" {
					return false
				}
				n++
			}
		}
		return n == 2
	})
	if draft {
		// A draft branch makes step one pending on it again, so only the pickers test adds one.
		if _, err := app.W.AdeTask.AddTaskRepo(ctx, adewire.AddTaskRepoArgs{TaskID: f.task, CodeRepoID: f.docs}); err != nil {
			t.Fatalf("AddTaskRepo: %v", err)
		}
	}
	f.refresh(t)
	return f
}

func (f *autoFixture) script(t *testing.T, fields scripts.CustomScriptFields) scripts.CustomScript {
	t.Helper()
	if fields.Color == "" {
		fields.Color = "blue"
	}
	rec, err := f.app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: fields})
	if err != nil {
		t.Fatalf("create script %q: %v", fields.Name, err)
	}
	return rec
}

func (f *autoFixture) preview(t *testing.T, args scriptruns.RunArgs) scriptruns.Preview {
	t.Helper()
	pv, err := f.app.W.ScriptRuns.Preview(args)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	return pv
}

func (f *autoFixture) start(t *testing.T, args scriptruns.RunArgs) scriptruns.Started {
	t.Helper()
	started, err := f.app.W.ScriptRuns.Start(scriptruns.StartArgs{RunArgs: args, Hash: f.preview(t, args).Hash})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return started
}

func (f *autoFixture) run(t *testing.T, id, state string) scriptruns.Run {
	t.Helper()
	var r scriptruns.Run
	testx.WaitUntil(t, waitFor, func() bool {
		got, err := f.app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: id})
		r = got
		return err == nil && got.State == state
	})
	return r
}

func waitCall(t *testing.T, app *flowharness.App, prefix string, n int) {
	t.Helper()
	testx.WaitUntil(t, waitFor, func() bool {
		_, err := os.Stat(filepath.Join(app.FakeDir, fmt.Sprintf("%s-%d.args", prefix, n)))
		return err == nil
	})
}

// stepRunOn is the newest run of the step on one branch.
func stepRunOn(t *testing.T, app *flowharness.App, taskID, stepID, branchID string) (adewire.Run, bool) {
	t.Helper()
	var best adewire.Run
	found := false
	for _, r := range taskOf(t, board(t, app), taskID).Runs {
		if r.StepID == stepID && r.BranchID == branchID && (!found || r.Attempt >= best.Attempt) {
			best, found = r, true
		}
	}
	return best, found
}

func release(t *testing.T, gate string) {
	t.Helper()
	if err := os.WriteFile(gate, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func smartFields(name, prompt string) scripts.CustomScriptFields {
	return scripts.CustomScriptFields{Name: name, Kind: scripts.KindSmart, Command: prompt, UseAdeDir: true}
}

func partVars(pv scriptruns.Preview) map[string]string {
	vars := map[string]string{}
	for _, p := range pv.Prompt {
		if p.Var != "" {
			vars[p.Var] = p.Value
		}
	}
	return vars
}

func TestAutomationRun(t *testing.T) {
	t.Run("task and branch needs", func(t *testing.T) {
		f := newAutoFixture(t, "", true)
		rec := f.script(t, smartFields("audit", "Audit {branch} of {task}"))

		pv := f.preview(t, scriptruns.RunArgs{ScriptID: rec.ID})
		if len(pv.Needs.Tasks) != 1 || pv.Needs.Tasks[0].ID != f.task || len(pv.Missing) != 1 || pv.Missing[0] != "task" {
			t.Fatalf("without a task: needs %+v, missing %v", pv.Needs, pv.Missing)
		}

		pv = f.preview(t, scriptruns.RunArgs{ScriptID: rec.ID, TaskID: f.task})
		if len(pv.Needs.Branches) != 3 || len(pv.Missing) != 1 || pv.Missing[0] != "branch" {
			t.Fatalf("with a task: needs %+v, missing %v", pv.Needs, pv.Missing)
		}
		for _, b := range pv.Needs.Branches {
			if (b.ID == f.docsBr.ID) != b.Disabled || (b.Disabled && b.Why != "not created yet") {
				t.Errorf("branch choice %+v", b)
			}
		}

		pv = f.preview(t, scriptruns.RunArgs{ScriptID: rec.ID, TaskID: f.task, BranchID: f.apiBr.ID})
		if len(pv.Needs.Branches) != 0 || len(pv.Missing) != 0 || pv.Blocker != "" {
			t.Fatalf("with a branch: needs %+v, missing %v, blocker %q", pv.Needs, pv.Missing, pv.Blocker)
		}
		if _, err := f.app.W.ScriptRuns.Preview(scriptruns.RunArgs{ScriptID: rec.ID, TaskID: f.task, BranchID: f.docsBr.ID}); err == nil ||
			!strings.Contains(err.Error(), "That branch can’t run this script now") {
			t.Fatalf("a draft branch = %v", err)
		}
	})

	t.Run("variables, tools and folder", func(t *testing.T) {
		f := newAutoFixture(t, "", false)
		rec := f.script(t, smartFields("audit", "Audit {branch} on {base} in {worktree} for {task}"))
		args := scriptruns.RunArgs{ScriptID: rec.ID, TaskID: f.task, BranchID: f.apiBr.ID}
		pv := f.preview(t, args)

		vars := partVars(pv)
		if vars["task"] != "Fix login" || vars["branch"] != "feat/api-login" || vars["worktree"] != f.apiBr.Worktree || vars["base"] == "" {
			t.Fatalf("prompt vars = %v", vars)
		}
		env := map[string]string{}
		for _, e := range pv.Env {
			env[e.Name] = e.Value
		}
		if env["KIRA_TASK"] != "Fix login" || env["KIRA_BRANCH"] != "feat/api-login" || env["KIRA_BASE"] != vars["base"] {
			t.Fatalf("env = %v", env)
		}
		allowed := strings.Join(pv.Allowed, " ")
		for _, want := range append([]string{toolName(agentmcp.FinishStep), toolName(agentmcp.RunOutcome)}, spaceToolNames()...) {
			if !strings.Contains(allowed, want) {
				t.Errorf("allowed tools lack %q: %s", want, allowed)
			}
		}
		if !strings.HasPrefix(pv.Suffix, agentmcp.SpaceNote(llmprompts.Defaults())) {
			t.Errorf("suffix = %q, want it to start with the Space suffix", pv.Suffix)
		}
		if pv.Dir.Mode != scripts.DirModeWorktree || pv.Dir.Path != f.apiBr.Worktree || pv.Dir.Pending {
			t.Fatalf("dir = %+v, want the api worktree", pv.Dir)
		}

		// Contract ade-automation: tests/ui/ade-v2-automations.spec.ts "contract: ..." reads these.
		f.app.Contract(t, "ade-automation", "CustomScriptsService.Create", rec)
		f.app.Contract(t, "ade-automation", "ScriptRunsService.Preview", pv, flowharness.Mask("hash"))
		started := f.start(t, args)
		run := f.run(t, started.RunID, "running")
		f.app.Contract(t, "ade-automation", "ScriptRunsService.Get#running", run, flowharness.Mask("createdAt", "startedAt", "finishedAt", "hash", "sessionId", "costUsd"))
		if run.Trigger != scriptruns.TriggerADE || run.TaskID != f.task || run.BranchID != f.apiBr.ID || run.BranchLabel == "" || run.Cwd != f.apiBr.Worktree {
			t.Fatalf("run = %+v", run)
		}
		release(t, f.gateOne)
		finished := f.run(t, started.RunID, "done")
		f.app.Contract(t, "ade-automation", "ScriptRunsService.Get#done", finished, flowharness.Mask("createdAt", "startedAt", "finishedAt", "hash", "sessionId", "costUsd"))

		off := f.script(t, scripts.CustomScriptFields{Name: "elsewhere", Kind: scripts.KindSmart, Command: "Look at {branch}"})
		pv = f.preview(t, scriptruns.RunArgs{ScriptID: off.ID, TaskID: f.task, BranchID: f.apiBr.ID})
		if want := filepath.Join(f.app.SpaceHome, "automations", off.ID); pv.Dir.Mode == scripts.DirModeWorktree || pv.Dir.Path != want {
			t.Fatalf("useAdeDir off: dir = %+v, want %s", pv.Dir, want)
		}
	})

	t.Run("existing branch without a worktree", func(t *testing.T) {
		f := newAutoFixture(t, "", false)
		lib, _ := cloneWithMain(f.app, filepath.Join(f.app.Work, "lib"))
		libID := importRepo(t, f.app, lib.Dir).ID
		gitOut(t, lib.Dir, "branch", "feat/lib-existing")
		_, err := f.app.W.AdeTask.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: libID, Name: "feat/lib-existing", TaskID: f.task})
		if err != nil {
			t.Fatalf("AddExistingBranch: %v", err)
		}
		libBr := branchOf(t, board(t, f.app), f.task, libID)
		gitOut(t, lib.Dir, "worktree", "remove", "--force", libBr.Worktree)
		rec := f.script(t, smartFields("docs", "Check {branch}"))
		args := scriptruns.RunArgs{ScriptID: rec.ID, TaskID: f.task, BranchID: libBr.ID}
		pv := f.preview(t, args)
		if !pv.Dir.Pending || pv.Dir.Mode != scripts.DirModeWorktree {
			t.Fatalf("dir = %+v, want a pending worktree", pv.Dir)
		}
		started := f.start(t, args)
		run := f.run(t, started.RunID, "done")
		if st, err := os.Stat(run.Cwd); err != nil || !st.IsDir() {
			t.Fatalf("run cwd %q was not created: %v", run.Cwd, err)
		}
	})

	t.Run("gate both ways", func(t *testing.T) {
		f := newAutoFixture(t, agentStep("two", "    before: approval\n"), false)
		rec := f.script(t, smartFields("audit", "Audit {branch}"))
		args := scriptruns.RunArgs{ScriptID: rec.ID, TaskID: f.task, BranchID: f.apiBr.ID}

		started := f.start(t, args)
		waitCall(t, f.app, "api", 2)
		rp, err := f.app.W.AdeTask.RebasePreview(ctx, adewire.OntoArgs{BranchID: f.apiBr.ID})
		if err != nil {
			t.Fatalf("RebasePreview: %v", err)
		}
		if len(rp.Blockers) == 0 || !strings.Contains(rp.Blockers[0].Text, "script run audit is running in feat/api-login") {
			t.Fatalf("rebase blockers = %+v, want the automation", rp.Blockers)
		}
		if _, err := f.app.W.AdeTask.StartBranch(ctx, adewire.StartBranchArgs{BranchID: f.apiBr.ID}); err == nil {
			t.Fatal("an interactive session started in a worktree an automation holds")
		}

		step := adewire.StepArgs{TaskID: f.task, StepID: "two"}
		if err := f.app.W.AdeTask.Approve(ctx, step); err != nil {
			t.Fatalf("Approve: %v", err)
		}
		held, ok := stepRunOn(t, f.app, f.task, "two", f.apiBr.ID)
		if !ok || held.State != "pending" || held.Note != "waiting for script run audit" {
			t.Fatalf("step run while a script works = %+v, want pending behind the automation", held)
		}
		f.app.Contract(t, "ade-automation", "AdeTaskService.Run#waiting", held, flowharness.Mask("createdAt", "startedAt", "finishedAt", "sessionId"))

		release(t, f.gateOne)
		f.run(t, started.RunID, "done")
		testx.WaitUntil(t, waitFor, func() bool {
			r, ok := stepRunOn(t, f.app, f.task, "two", f.apiBr.ID)
			return ok && (r.State == "running" || r.State == "done")
		})

		// Reverse: the step now runs on api (attempt 3, held open); a script on that branch waits.
		waitCall(t, f.app, "api", 3)
		pv := f.preview(t, args)
		if pv.Blocker != "a run is working on feat/api-login" {
			t.Fatalf("preview blocker while a step runs = %q", pv.Blocker)
		}
		if _, err := f.app.W.ScriptRuns.Start(scriptruns.StartArgs{RunArgs: args, Hash: pv.Hash}); errCode(err) != "E_BAD_REQUEST" {
			t.Fatalf("Start on a busy branch = %v, want E_BAD_REQUEST", err)
		}
		release(t, f.gateTwo)
		testx.WaitUntil(t, waitFor, func() bool {
			r, ok := stepRunOn(t, f.app, f.task, "two", f.apiBr.ID)
			return ok && r.State == "done"
		})
	})

	t.Run("held step survives a restart", func(t *testing.T) {
		f := newAutoFixture(t, agentStep("two", "    before: approval\n"), false)
		rec := f.script(t, smartFields("audit", "Audit {branch}"))
		args := scriptruns.RunArgs{ScriptID: rec.ID, TaskID: f.task, BranchID: f.apiBr.ID}
		f.start(t, args)
		waitCall(t, f.app, "api", 2)
		step := adewire.StepArgs{TaskID: f.task, StepID: "two"}
		if err := f.app.W.AdeTask.Approve(ctx, step); err != nil {
			t.Fatal(err)
		}
		if r, _ := stepRunOn(t, f.app, f.task, "two", f.apiBr.ID); r.State != "pending" {
			t.Fatalf("step run = %+v, want pending", r)
		}
		release(t, f.gateTwo)
		release(t, f.gateOne)
		f.app.Restart()
		if _, err := f.app.W.AdeTask.Board(ctx); err != nil {
			t.Fatal(err)
		}
		testx.WaitUntil(t, waitFor, func() bool {
			r, ok := stepRunOn(t, f.app, f.task, "two", f.apiBr.ID)
			return ok && r.State != "pending"
		})
	})

	t.Run("a run held by an older version survives a restart", func(t *testing.T) {
		f := newAutoFixture(t, agentStep("two", "    before: approval\n"), false)
		rec := f.script(t, smartFields("audit", "Audit {branch}"))
		args := scriptruns.RunArgs{ScriptID: rec.ID, TaskID: f.task, BranchID: f.apiBr.ID}
		f.start(t, args)
		waitCall(t, f.app, "api", 2)
		step := adewire.StepArgs{TaskID: f.task, StepID: "two"}
		if err := f.app.W.AdeTask.Approve(ctx, step); err != nil {
			t.Fatal(err)
		}
		r, _ := stepRunOn(t, f.app, f.task, "two", f.apiBr.ID)
		if r.State != "pending" {
			t.Fatalf("step run = %+v, want pending", r)
		}
		old := "waiting for automation audit"
		if _, err := (&repos.AdeTaskRepo{DB: f.app.DB()}).UpdateRun(r.ID, model.AdeRunPatch{Note: &old}); err != nil {
			t.Fatal(err)
		}
		release(t, f.gateTwo)
		release(t, f.gateOne)
		f.app.Restart()
		if _, err := f.app.W.AdeTask.Board(ctx); err != nil {
			t.Fatal(err)
		}
		testx.WaitUntil(t, waitFor, func() bool {
			r, ok := stepRunOn(t, f.app, f.task, "two", f.apiBr.ID)
			return ok && r.State != "pending"
		})
	})

	t.Run("normal script from a task", func(t *testing.T) {
		f := newAutoFixture(t, "", false)
		rec := f.script(t, scripts.CustomScriptFields{Name: "show", Command: `echo "branch=$KIRA_BRANCH"; read x`, UseAdeDir: true})
		args := scriptruns.RunArgs{ScriptID: rec.ID, TaskID: f.task, BranchID: f.apiBr.ID}
		pv := f.preview(t, args)
		if pv.Kind != "script" || pv.Dir.Path != f.apiBr.Worktree {
			t.Fatalf("preview = %+v", pv)
		}
		started := f.start(t, args)
		if started.Terminal == nil {
			t.Fatalf("Start = %+v, want a terminal launch", started)
		}
		const win = "w-ade"
		if _, err := f.app.W.Terminal.Open(terminal.OpenArgs{
			TerminalID: "t1", WindowKey: win, Cols: 80, Rows: 24, LaunchKind: terminal.LaunchKindScript,
			ScriptID: rec.ID, ScriptLaunchToken: started.Terminal.Token,
		}); err != nil {
			t.Fatal(err)
		}
		testx.WaitUntil(t, waitFor, func() bool { return strings.Contains(terminalOutput(f.app, win, "t1"), "branch=feat/api-login") })
		runs, err := f.app.W.ScriptRuns.List(scriptruns.ListArgs{})
		if err != nil || len(runs) == 0 || runs[0].Trigger != scriptruns.TriggerADE || runs[0].TaskID != f.task {
			t.Fatalf("runs = %+v, %v", runs, err)
		}

		// A busy branch does not refuse a terminal script.
		smart := f.script(t, smartFields("audit", "Audit {branch}"))
		f.start(t, scriptruns.RunArgs{ScriptID: smart.ID, TaskID: f.task, BranchID: f.apiBr.ID})
		waitCall(t, f.app, "api", 2)
		if pv := f.preview(t, args); pv.Blocker != "" {
			t.Fatalf("a terminal script is blocked: %q", pv.Blocker)
		}
		release(t, f.gateOne)
	})

	t.Run("run_outcome automation and task filter", func(t *testing.T) {
		f := newAutoFixture(t, "", false)
		rec := f.script(t, smartFields("audit", "Audit {branch}"))
		args := scriptruns.RunArgs{ScriptID: rec.ID, TaskID: f.task, BranchID: f.apiBr.ID}
		f.start(t, args)
		waitCall(t, f.app, "api", 2)
		release(t, f.gateOne)
		testx.WaitUntil(t, waitFor, func() bool {
			runs, _ := f.app.W.ScriptRuns.List(scriptruns.ListArgs{TaskID: f.task})
			return len(runs) == 1 && runs[0].State == "done"
		})
		other := f.script(t, scripts.CustomScriptFields{Name: "elsewhere", Kind: scripts.KindSmart, Command: "hello"})
		f.start(t, scriptruns.RunArgs{ScriptID: other.ID})
		testx.WaitUntil(t, waitFor, func() bool {
			runs, _ := f.app.W.ScriptRuns.List(scriptruns.ListArgs{})
			return len(runs) == 2 && runs[0].State != "running" && runs[1].State != "running"
		})
		mine, err := f.app.W.ScriptRuns.List(scriptruns.ListArgs{TaskID: f.task})
		if err != nil || len(mine) != 1 || mine[0].TaskID != f.task {
			t.Fatalf("List(TaskID) = %+v, %v, want only the task's run", mine, err)
		}
	})
}

func terminalOutput(app *flowharness.App, window, terminalID string) string {
	var sb strings.Builder
	for _, ev := range app.Events.Since(0, appevent.ChannelTerminal) {
		e, ok := ev.Data.(terminal.Event)
		if !ok || e.TerminalID != terminalID || ev.Window != window {
			continue
		}
		raw, _ := base64.StdEncoding.DecodeString(e.Data)
		sb.Write(raw)
	}
	return sb.String()
}

func errCode(err error) string {
	var ie *ipcerr.Error
	if errors.As(err, &ie) {
		return ie.Code
	}
	return ""
}

func smartStepYAML(script, extra string) string {
	return "  - id: s1\n    name: s1\n    kind: agent\n    runs_on: each repo\n    timeout: 2m\n    smart_script: " + script + "\n" + extra
}

// runOnly starts the run; a launch error is the run's own failure, which the board records.
func runOnly(t *testing.T, f *runFixture, branch string) adewire.Run {
	t.Helper()
	br := branchOf(t, board(t, f.app), f.taskID, f.repoID)
	_, _ = f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.taskID, BranchNames: map[string]string{br.ID: branch}})
	return waitRun(t, f.app, f.taskID, "s1", "done", "failed", "stuck")
}

func TestAutomationStep(t *testing.T) {
	smart := func(f *runFixture, name, body string, params []scripts.Param, mod func(*scripts.Smart)) {
		t.Helper()
		sm := &scripts.Smart{Model: "opus", MaxBudgetUSD: 0.5, Timeout: "5m", Tools: []string{"Read", "Grep"}}
		if mod != nil {
			mod(sm)
		}
		_, err := f.app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
			Name: name, Kind: scripts.KindSmart, Command: body, Color: "blue", Params: params, Smart: sm, UseAdeDir: true,
		}})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	level := []scripts.Param{{Name: "level", Type: scripts.ParamSelect, Options: []string{"low", "high"}, Required: true}}
	lintStep := func(extra string) string { return smartStepYAML("lint", extra) }

	t.Run("runs the script", func(t *testing.T) {
		f := newRunFixture(t, []fakeagent.Action{{Name: "done"}}, lintStep("    params:\n      level: high\n"))
		smart(f, "lint", "Lint {branch} at {level} for {task}", level, nil)
		r := runOnly(t, f, "feat/api-lint")
		if r.State != "done" {
			t.Fatalf("run = %+v", r)
		}
		if got, want := fakeFile(t, f.app, "api-1.prompt"), "Lint feat/api-lint at high for Fix login\n\n"+agentmcp.SpaceNote(llmprompts.Defaults())+"\n\n"+agentmcp.FinishInstruction(llmprompts.Defaults(), nil); !strings.HasPrefix(got, want) {
			t.Fatalf("prompt = %q, want it to start with %q", got, want)
		}
		args := fakeFile(t, f.app, "api-1.args")
		for _, want := range []string{"--model\nopus", "--max-budget-usd\n0.5", "--tools\nRead,Grep", "--strict-mcp-config", toolName(agentmcp.FinishStep), toolName(agentmcp.RunOutcome)} {
			if !strings.Contains(args, want) {
				t.Errorf("argv lacks %q:\n%s", want, args)
			}
		}
		if got := fakeFile(t, f.app, "api-1.env"); !strings.Contains(got, "KIRA_PARAM_LEVEL=high") || !strings.Contains(got, "KIRA_BRANCH=feat/api-lint") {
			t.Errorf("env = %q", got)
		}
	})

	t.Run("user mcp server", func(t *testing.T) {
		f := newRunFixture(t, []fakeagent.Action{{Name: "done", MCP: []fakeagent.MCPCall{{Server: "fake", Tool: "echo", Args: map[string]any{"text": "hi"}}}}}, lintStep("    params:\n      level: high\n"))
		f.app.SetHermetic(t, false) // ticks a server of the CLI's own config
		cfg := fmt.Sprintf(`{"mcpServers":{"fake":{"command":%q,"env":{"TOKEN":"SENTINEL-ENV"}}}}`, filepath.Join(f.app.BinDir, "fake-mcp"))
		if err := os.WriteFile(filepath.Join(f.app.Home, ".claude.json"), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
		smart(f, "lint", "Lint {level}", level, func(s *scripts.Smart) { s.MCP = []scripts.MCPChoice{{Server: "fake", AllTools: true}} })
		if r := runOnly(t, f, "feat/api-mcp"); r.State != "done" {
			t.Fatalf("run = %+v", r)
		}
		args := fakeFile(t, f.app, "api-1.args")
		if n := strings.Count(args, "--mcp-config"); n != 1 || strings.Contains(args, "--strict-mcp-config") || !strings.Contains(args, "--setting-sources\nuser") || !strings.Contains(args, "mcp__fake") {
			t.Errorf("argv:\n%s", args)
		}
		if got, want := fakeFile(t, f.app, "api-1.settings"), `{"disableAllHooks":true,"allowedMcpServers":[{"serverName":"kira-ade"},{"serverName":"fake"}],"enableAllProjectMcpServers":false,"enabledMcpjsonServers":[]}`; got != want {
			t.Errorf("--settings = %s, want %s", got, want)
		}
		if _, err := os.Stat(filepath.Join(f.app.FakeDir, "mcp-echo-1.json")); err != nil {
			t.Errorf("the echo call did not reach the user server: %v", err)
		}
		if strings.Contains(args, "SENTINEL") || strings.Contains(fakeFile(t, f.app, "api-1.mcp0"), "SENTINEL") {
			t.Error("a user credential reached the run's argv or config")
		}
		_ = filepath.WalkDir(f.app.SpaceHome, func(path string, d fs.DirEntry, err error) error {
			if err == nil && strings.HasSuffix(path, "-user.mcp.json") {
				t.Errorf("user config copy left: %s", path)
			}
			return nil
		})
	})

	t.Run("user mcp server removed fails the step", func(t *testing.T) {
		f := newRunFixture(t, []fakeagent.Action{{Name: "done"}}, lintStep("    params:\n      level: high\n"))
		f.app.SetHermetic(t, false)
		cfg := fmt.Sprintf(`{"mcpServers":{"other":{"command":%q}}}`, filepath.Join(f.app.BinDir, "fake-mcp"))
		if err := os.WriteFile(filepath.Join(f.app.Home, ".claude.json"), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
		smart(f, "lint", "Lint {level}", level, func(s *scripts.Smart) { s.MCP = []scripts.MCPChoice{{Server: "fake", AllTools: true}} })
		if r := runOnly(t, f, "feat/api-gone"); r.State != "failed" {
			t.Fatalf("run = %+v, want failed", r)
		}
	})

	t.Run("cannot start", func(t *testing.T) {
		for _, c := range []struct {
			name, extra, want string
			setup             func(f *runFixture)
		}{
			{"unknown script", "    params:\n      level: high\n", `smart script "lint": not found`, func(*runFixture) {}},
			{"duplicate name", "    params:\n      level: high\n", `smart script "lint": 2 scripts have that name`, func(f *runFixture) {
				smart(f, "lint", "a", level, nil)
				smart(f, "lint", "b", level, nil)
			}},
			{"missing param", "", `smart script "lint": needs a value for level`, func(f *runFixture) { smart(f, "lint", "x {level}", level, nil) }},
			{"not an option", "    params:\n      level: extreme\n", "is not an option of level", func(f *runFixture) { smart(f, "lint", "x {level}", level, nil) }},
		} {
			t.Run(c.name, func(t *testing.T) {
				f := newRunFixture(t, []fakeagent.Action{{Name: "done"}}, lintStep(c.extra))
				c.setup(f)
				r := runOnly(t, f, "feat/api-bad")
				if r.State != "failed" || !strings.HasPrefix(r.Note, "could not start: ") || !strings.Contains(r.Note, c.want) {
					t.Fatalf("run = %+v, want failed with %q", r, c.want)
				}
			})
		}
	})

	t.Run("budget stop", func(t *testing.T) {
		result := filepath.Join(t.TempDir(), "budget.jsonl")
		if err := os.WriteFile(result, []byte(`{"type":"result","subtype":"error_max_budget_usd","is_error":true,"total_cost_usd":0.5,"num_turns":3}`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		zero := 0
		f := newRunFixture(t, []fakeagent.Action{{Emit: result, Exit: &zero}}, lintStep("    params:\n      level: low\n"))
		smart(f, "lint", "x {level}", level, nil)
		r := runOnly(t, f, "feat/api-budget")
		o := r.Outcome
		if r.State == "done" || o == nil || o.Source != "budget" || o.Reason != "stopped at the budget of 0.5 USD" || o.CostUSD == nil || *o.CostUSD != 0.5 {
			t.Fatalf("run = %+v, outcome = %+v", r, o)
		}
	})

	t.Run("step timeout wins", func(t *testing.T) {
		f := newRunFixture(t, []fakeagent.Action{{Name: "sleep"}},
			"  - id: s1\n    name: s1\n    kind: agent\n    runs_on: each repo\n    timeout: 2s\n    smart_script: lint\n    params:\n      level: low\n")
		smart(f, "lint", "x {level}", level, func(s *scripts.Smart) { s.Timeout = "15m" })
		r := runOnly(t, f, "feat/api-timeout")
		if o := r.Outcome; o == nil || o.Source != "timeout" || !strings.Contains(o.Reason, "2s") {
			t.Fatalf("run = %+v", r)
		}
	})
}

func TestRunOutcomeAutomation(t *testing.T) {
	f := newAutoFixture(t, agentStep("two", "    before: approval\n"), false)
	call := func(kind string) fakeagent.MCPCall {
		return fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: "run_outcome", Args: map[string]any{"kind": kind}}
	}
	// Attempt 1 of api was step one; 2 is the failing script; 3 is step two asking for outcomes.
	f.app.Scenario(fakeagent.Scenario{Claude: map[string][]fakeagent.Action{
		"api": {{Name: "done"}, {Name: "failed"}, {Name: "done", MCP: []fakeagent.MCPCall{call("automation"), call("")}}},
		"*":   {{Name: "done"}},
	}})
	rec := f.script(t, smartFields("audit", "Audit {branch}"))
	started := f.start(t, scriptruns.RunArgs{ScriptID: rec.ID, TaskID: f.task, BranchID: f.apiBr.ID})
	failed := f.run(t, started.RunID, "failed")
	if failed.Outcome == nil || failed.Outcome.Reason == "" {
		t.Fatalf("script run = %+v, want a reason", failed)
	}

	if err := f.app.W.AdeTask.Approve(ctx, adewire.StepArgs{TaskID: f.task, StepID: "two"}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		r, ok := stepRunOn(t, f.app, f.task, "two", f.apiBr.ID)
		return ok && r.State == "done"
	})
	listed := fakeFile(t, f.app, "mcp-run_outcome-1.json")
	if !strings.Contains(listed, "audit") || !strings.Contains(listed, failed.Outcome.Reason) {
		t.Fatalf("kind automation result = %s, want the failed script run and its reason", listed)
	}
	if plain := fakeFile(t, f.app, "mcp-run_outcome-2.json"); strings.Contains(plain, "audit") {
		t.Fatalf("kind \"\" result = %s, must not list script runs", plain)
	}
}
