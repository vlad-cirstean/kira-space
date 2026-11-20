package adeflow_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Contract ade-required-services: tests/ui/ade-v2-required-services.spec.ts reads these.

func requiredServices(t *testing.T, f *autoFixture, taskID string) adewire.RequiredServices {
	t.Helper()
	got, err := f.app.W.AdeTask.RequiredServices(ctx, adewire.TaskArgs{TaskID: taskID})
	if err != nil {
		t.Fatalf("RequiredServices: %v", err)
	}
	return got
}

func serviceOf(t *testing.T, got adewire.RequiredServices, script, branchID string) adewire.RequiredService {
	t.Helper()
	for _, s := range got.Items {
		if s.Script == script && s.BranchID == branchID {
			return s
		}
	}
	t.Fatalf("no service %q on branch %q in %+v", script, branchID, got.Items)
	return adewire.RequiredService{}
}

// startInTerminal starts a normal script run and opens its terminal, the way the Run dialog does. The
// run row appears when the terminal opens.
func startInTerminal(t *testing.T, f *autoFixture, rec scripts.CustomScript, args scriptruns.RunArgs, win, term string) scriptruns.Run {
	t.Helper()
	started := f.start(t, args)
	if started.Terminal == nil {
		t.Fatalf("Start = %+v, want a terminal launch", started)
	}
	if _, err := f.app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: term, WindowKey: win, Cols: 80, Rows: 24, LaunchKind: terminal.LaunchKindScript,
		ScriptID: rec.ID, ScriptLaunchToken: started.Terminal.Token,
	}); err != nil {
		t.Fatal(err)
	}
	var run scriptruns.Run
	testx.WaitUntil(t, waitFor, func() bool {
		runs, err := f.app.W.ScriptRuns.List(scriptruns.ListArgs{})
		for _, r := range runs {
			if err == nil && r.TerminalID == term && r.State == "running" {
				run = r
				return true
			}
		}
		return false
	})
	return run
}

func TestRequiredServices(t *testing.T) {
	f := newAutoFixture(t, "", false)
	db := f.script(t, scripts.CustomScriptFields{
		Name: "Dev DB", Command: `echo "db up"; read x`,
		Params: []scripts.Param{{Name: "port", Type: scripts.ParamText}},
	})
	srv := f.script(t, scripts.CustomScriptFields{Name: "Dev server", Command: `echo "server up"; read x`, UseAdeDir: true})
	setRepoFile(t, f.app, f.api, func(rf *repocfg.File) {
		rf.Services = []model.ScriptRef{
			{Script: "Dev DB", Params: map[string]model.ParamValues{"port": {"5432"}}},
			{Script: "Dev server"},
			{Script: "Ghost"},
		}
	})

	stopped := requiredServices(t, f, f.task)
	if len(stopped.Items) != 3 {
		t.Fatalf("items = %+v, want 3", stopped.Items)
	}
	if s := serviceOf(t, stopped, "Dev DB", ""); s.State != adewire.ServiceStopped || s.ScriptID != db.ID || s.Params["port"][0] != "5432" {
		t.Fatalf("Dev DB = %+v, want stopped with its params", s)
	}
	if s := serviceOf(t, stopped, "Dev server", f.apiBr.ID); s.State != adewire.ServiceStopped || s.CodeRepoID != f.api {
		t.Fatalf("Dev server = %+v, want stopped on the api branch", s)
	}
	if s := serviceOf(t, stopped, "Ghost", ""); s.State != adewire.ServiceInvalid || s.Problem != "not found" || s.ScriptID != "" {
		t.Fatalf("Ghost = %+v, want invalid: not found", s)
	}
	f.app.Contract(t, "ade-required-services", "AdeTaskService.RequiredServices#stopped", stopped)

	// A run of the own-folder script from anywhere counts, the task does not matter.
	dbRun := startInTerminal(t, f, db, scriptruns.RunArgs{ScriptID: db.ID, Params: map[string][]string{"port": {"5432"}}}, "w-db", "t-db")
	if s := serviceOf(t, requiredServices(t, f, f.task), "Dev DB", ""); s.State != adewire.ServiceRunning || s.RunID != dbRun.ID {
		t.Fatalf("Dev DB = %+v, want running as %s", s, dbRun.ID)
	}
	// The worktree script counts per task and branch only.
	startInTerminal(t, f, srv, scriptruns.RunArgs{ScriptID: srv.ID, TaskID: f.task, BranchID: f.apiBr.ID}, "w-srv", "t-srv")
	setRepoFile(t, f.app, f.api, func(rf *repocfg.File) { rf.Services = rf.Services[:2] })
	running := requiredServices(t, f, f.task)
	if len(running.Items) != 2 || serviceOf(t, running, "Dev server", f.apiBr.ID).State != adewire.ServiceRunning {
		t.Fatalf("items = %+v, want both running", running.Items)
	}
	f.app.Contract(t, "ade-required-services", "AdeTaskService.RequiredServices#running", running)

	// Another task with a branch not created yet: the DB is shared, the server has no worktree.
	other := createTask(t, f.app, "Other work", "flow", f.api)
	var draft adewire.Branch
	for _, br := range board(t, f.app).Branches {
		if br.TaskID == other.ID && br.CodeRepoID == f.api {
			draft = br
		}
	}
	got := requiredServices(t, f, other.ID)
	if s := serviceOf(t, got, "Dev DB", ""); s.State != adewire.ServiceRunning || s.RunID != dbRun.ID {
		t.Fatalf("Dev DB for the other task = %+v, want the same run", s)
	}
	if s := serviceOf(t, got, "Dev server", draft.ID); s.State != adewire.ServiceNoWorktree {
		t.Fatalf("Dev server for the other task = %+v, want noWorktree", s)
	}
	f.app.Contract(t, "ade-required-services", "AdeTaskService.RequiredServices#noWorktree", got)

	if err := f.app.W.Terminal.Close(terminal.CloseArgs{TerminalID: "t-db"}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		return serviceOf(t, requiredServices(t, f, f.task), "Dev DB", "").State == adewire.ServiceStopped
	})
	if s := serviceOf(t, requiredServices(t, f, f.task), "Dev server", f.apiBr.ID); s.State != adewire.ServiceRunning {
		t.Fatalf("closing the DB terminal changed the server to %q", s.State)
	}
}
