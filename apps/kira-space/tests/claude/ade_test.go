package claude

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// adeFixture is a real-claude app with one imported clone "api" holding one commit on main.
type adeFixture struct {
	*realApp
	repo   *flowharness.Repo
	repoID string
}

// newAdeFixture seeds files into the base commit, so a worktree off main carries them.
func newAdeFixture(t *testing.T, files map[string]string) *adeFixture {
	return newAdeFixtureWith(t, files)
}

func newAdeFixtureWith(t *testing.T, files map[string]string, opts ...flowharness.Opt) *adeFixture {
	t.Helper()
	app := newRealAppWith(t, opts)
	bare := app.NewBare("api-remote")
	seed := app.NewRepo("api-seed")
	seed.Commit("base", mergeFiles(map[string]string{"base.txt": "base\n"}, files))
	bare.PushFrom(seed)
	clone := bare.Clone(filepath.Join(app.Work, "api"))
	rec, err := app.W.CodeWorkspace.ImportRepo(ctx, bridge.CodeWorkspaceImportArgs{Path: clone.Dir})
	if err != nil {
		t.Fatalf("ImportRepo: %v", err)
	}
	return &adeFixture{realApp: app, repo: clone, repoID: rec.ID}
}

func mergeFiles(a, b map[string]string) map[string]string {
	for k, v := range b {
		a[k] = v
	}
	return a
}

func (f *adeFixture) saveWorkflow(t *testing.T, id, steps string) {
	t.Helper()
	entry, err := f.W.AdeTask.SaveWorkflowYaml(ctx, adewire.SaveWorkflowYamlArgs{FileName: id + ".yaml", Yaml: "id: " + id + "\nname: Flow\nsteps:\n" + steps})
	if err != nil || entry.Error != nil {
		t.Fatalf("SaveWorkflowYaml: %v %+v", err, entry.Error)
	}
}

func (f *adeFixture) createTask(t *testing.T, workflow string) adewire.Task {
	t.Helper()
	task, err := f.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Real claude", CodeRepoIDs: []string{f.repoID}, WorkflowID: workflow})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return task
}

const (
	agentStepYAML = "  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 2m\n" +
		"    prompt: This step has no work. Call finish_step with status done and summary ok.\n"
	manualStepYAML = "  - id: work\n    name: Work\n    kind: manual\n    status: In progress\n    session: true\n"
)

// startRun starts the task's agent step on branch and returns the run once it left running.
func (f *adeFixture) startRun(t *testing.T, task adewire.Task, branch string) adewire.Run {
	t.Helper()
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
	if _, err := f.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{branchID: branch}}); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	var run adewire.Run
	testx.WaitUntil(t, testTimeout, func() bool {
		b, err := f.W.AdeTask.Board(ctx)
		if err != nil {
			return false
		}
		for _, tk := range b.Tasks {
			if tk.ID != task.ID {
				continue
			}
			for _, r := range tk.Runs {
				if r.StepID == "one" && r.State != "running" && r.State != "queued" {
					run = r
					return true
				}
			}
		}
		return false
	})
	return run
}

func (f *adeFixture) runLog(t *testing.T, runID string) string {
	t.Helper()
	var sb strings.Builder
	after := 0
	for {
		page, err := f.W.AdeTask.ReadLog(ctx, adewire.ReadLogArgs{Kind: "run", ID: runID, AfterSeq: after})
		if err != nil {
			t.Fatalf("ReadLog: %v", err)
		}
		if len(page.Chunks) == 0 {
			return sb.String()
		}
		for _, c := range page.Chunks {
			sb.WriteString(c.Text)
			sb.WriteByte('\n')
		}
		after = page.NextSeq
	}
}

// poison is a project setting that breaks any run that loads it.
var poison = map[string]string{".claude/settings.json": `{"permissions":{"deny":["mcp__kira-ade__finish_step"]}}` + "\n"}

// TestAdeHeadlessRun runs a one-step workflow through the real claude -p and its kira-ade MCP
// server. The second case poisons the repo's project settings and runs with
// ade.headlessSettingSources=user: the run passes only if --setting-sources user keeps them out.
func TestAdeHeadlessRun(t *testing.T) {
	cases := []struct {
		name, sources string
		files         map[string]string
		wantDone      bool
	}{
		{"default sources", "", nil, true},
		// Control: with every source loaded the poison bites, so the next case proves something.
		{"default sources load project settings", "", poison, false},
		{"user sources ignore project settings", "user", poison, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newAdeFixture(t, c.files)
			if c.sources != "" {
				if _, err := f.W.Settings.Set(bridge.SettingsSetArgs{Patch: model.SettingsPatch{Ade: &model.AdePatch{HeadlessSettingSources: &c.sources}}}); err != nil {
					t.Fatal(err)
				}
			}
			f.saveWorkflow(t, "flow", agentStepYAML+manualStepYAML)
			run := f.startRun(t, f.createTask(t, "flow"), "feat/real-claude")
			log := f.runLog(t, run.ID)
			if !c.wantDone {
				if run.State == "done" {
					t.Fatalf("run done despite the project setting denying finish_step; log:\n%s", tail(log, 1500))
				}
				return
			}
			if run.State != "done" {
				t.Fatalf("run = %+v, want done; log:\n%s", run, tail(log, 1500))
			}
			if !strings.Contains(log, "mcp__kira-ade__finish_step") {
				t.Fatalf("run log lacks the finish_step tool call:\n%s", tail(log, 1500))
			}
			f.requireCostUnder(t)
		})
	}
}

// TestAdeStepResults runs a step that declares its own results: the real claude must pick one of them
// from the finish_step schema, and the run keeps the chosen id.
func TestAdeStepResults(t *testing.T) {
	f := newAdeFixture(t, nil)
	step := "  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 2m\n" +
		"    prompt: This step has no work. Call finish_step with status pass and summary ok.\n" +
		"    results:\n" +
		"      - id: pass\n        ok: true\n        description: The step has nothing left to do.\n" +
		"      - id: retry_later\n        ok: false\n        description: Something is missing.\n"
	f.saveWorkflow(t, "flow", step+manualStepYAML)
	run := f.startRun(t, f.createTask(t, "flow"), "feat/real-claude")
	log := f.runLog(t, run.ID)
	if run.State != "done" || run.Outcome == nil || run.Outcome.Result != "pass" || run.Outcome.Route != "next" {
		t.Fatalf("run = %+v, want done with result pass routed next; log:\n%s", run, tail(log, 1500))
	}
	f.requireCostUnder(t)
}

// TestAdeManualSession opens a manual step's TUI session in the real claude, drives it with Send and
// follows it through the hooks.
func TestAdeManualSession(t *testing.T) {
	f := newAdeFixture(t, nil)
	f.saveWorkflow(t, "flow", manualStepYAML)
	task := f.createTask(t, "flow")
	launch, err := f.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	f.seedClaudeJSON(t, f.repo.Dir, launch.Cwd)
	mark := f.Events.Mark()
	if _, err := f.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: launch.TerminalID, WindowKey: window, Cwd: launch.Cwd, Cols: 120, Rows: 40,
		Command: launch.Command, LaunchKind: terminal.LaunchKindAgent,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.W.Terminal.Close(terminal.CloseArgs{TerminalID: launch.TerminalID}) })

	session := func() adewire.Session {
		res, err := f.W.AdeTask.Sessions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range res.Sessions {
			if s.ID == launch.SessionID {
				return s
			}
		}
		t.Fatalf("no session %s in %+v", launch.SessionID, res.Sessions)
		return adewire.Session{}
	}
	start := f.waitAgentEvent(t, mark, launch.TerminalID, "SessionStart", 60*time.Second)
	rec := session()
	if rec.AgentSessionID == "" || rec.AgentSessionID != start.SessionID {
		t.Fatalf("recorded claude session %q, SessionStart session %q, want equal", rec.AgentSessionID, start.SessionID)
	}

	stops := func() int {
		n := 0
		for _, e := range f.agentEvents(t, mark, launch.TerminalID) {
			if e.Event == "Stop" {
				n++
			}
		}
		return n
	}
	waitStops := func(want int) {
		t.Helper()
		testx.WaitUntil(t, 90*time.Second, func() bool { return stops() >= want })
	}
	// SessionStart fires before the input box accepts keys.
	time.Sleep(2 * time.Second)
	if err := f.W.AdeTask.Send(ctx, adewire.SendArgs{SessionID: launch.SessionID, Message: "Reply with the single word one."}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitStops(1)
	testx.WaitUntil(t, 10*time.Second, func() bool { return session().LastActiveAt > rec.LastActiveAt })

	if err := f.W.AdeTask.Send(ctx, adewire.SendArgs{SessionID: launch.SessionID, Message: "Reply with the single word two."}); err != nil {
		t.Fatalf("second Send: %v", err)
	}
	waitStops(2)
}

// TestAdeAdHocSession opens an agent session with no first message: the agent starts idle, the user's
// first prompt arrives through Send, and the session holds the Space grant (handover).
func TestAdeAdHocSession(t *testing.T) {
	f := newAdeFixture(t, nil)
	f.saveWorkflow(t, "flow", manualStepYAML)
	task := f.createTask(t, "flow")
	b, err := f.W.AdeTask.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var row adewire.Branch
	for _, br := range b.Branches {
		if br.TaskID == task.ID && br.CodeRepoID == f.repoID {
			row = br
		}
	}
	br, err := f.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: row.ID, Name: "feat/ad-hoc"})
	if err != nil || br.Worktree == "" {
		t.Fatalf("SetTaskBranch = %+v, %v", br, err)
	}
	launch, err := f.W.AdeTask.StartAdHoc(ctx, adewire.StartAdHocArgs{BranchID: br.ID})
	if err != nil {
		t.Fatal(err)
	}
	f.seedClaudeJSON(t, f.repo.Dir, launch.Cwd)
	mark := f.Events.Mark()
	if _, err := f.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: launch.TerminalID, WindowKey: window, Cwd: launch.Cwd, Cols: 120, Rows: 40,
		Command: launch.Command, LaunchKind: terminal.LaunchKindAgent,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.W.Terminal.Close(terminal.CloseArgs{TerminalID: launch.TerminalID}) })

	f.waitAgentEvent(t, mark, launch.TerminalID, "SessionStart", 60*time.Second)
	// SessionStart fires before the input box accepts keys.
	time.Sleep(2 * time.Second)
	if err := f.W.AdeTask.Send(ctx, adewire.SendArgs{
		SessionID: launch.SessionID,
		Message:   "Call the handover tool with notes `adhoc ok`, then reply done.",
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	testx.WaitUntil(t, 90*time.Second, func() bool {
		for _, e := range f.agentEvents(t, mark, launch.TerminalID) {
			if e.Event == "Stop" {
				return true
			}
		}
		return false
	})
	res, err := f.W.AdeTask.Handovers(ctx, adewire.TaskArgs{TaskID: task.ID})
	if err != nil {
		t.Fatalf("Handovers: %v", err)
	}
	for _, h := range res.Handovers {
		if strings.Contains(h.Body, "adhoc ok") {
			f.requireCostUnder(t)
			return
		}
	}
	t.Fatalf("no handover with the notes in %+v", res.Handovers)
}

// TestAdeRebaseRun rebases a pushed task branch in a background run with the real claude: main
// moved on, the agent rebases and reports through finish_step, and git is the judge.
func TestAdeRebaseRun(t *testing.T) {
	cases := []struct {
		name         string
		mainFile     string // file main adds; the branch adds a.txt
		mainBody     string
		branchBody   string // body of mainFile on the branch; "" = no clash
		mayConflict  bool
		wantVerified bool
	}{
		{"clean", "m.txt", "m\n", "", false, true},
		{"one-line conflict", "clash.txt", "main\n", "branch\n", true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newAdeFixture(t, nil)
			f.saveWorkflow(t, "flow", agentStepYAML)
			f.repo.Git("checkout", "-q", "-b", "feat/a", "main")
			files := map[string]string{"a.txt": "a\n"}
			if c.branchBody != "" {
				files[c.mainFile] = c.branchBody
			}
			f.repo.Commit("work a", files)
			f.repo.Git("push", "-q", "origin", "feat/a")
			f.repo.Checkout("main")
			added, err := f.W.AdeTask.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: f.repoID, Name: "feat/a"})
			if err != nil {
				t.Fatalf("AddExistingBranch: %v", err)
			}
			f.repo.Commit("main moves", map[string]string{c.mainFile: c.mainBody})
			f.repo.Git("push", "-q", "origin", "main")

			branch := func() adewire.Branch {
				b, err := f.W.AdeTask.Board(ctx)
				if err != nil {
					t.Fatal(err)
				}
				for _, br := range b.Branches {
					if br.TaskID == added.Task.ID {
						return br
					}
				}
				t.Fatal("task branch missing")
				return adewire.Branch{}
			}
			br := branch()
			start, err := f.W.AdeTask.Rebase(ctx, adewire.RebaseArgs{BranchID: br.ID})
			if err != nil {
				t.Fatalf("Rebase: %v", err)
			}
			var run adewire.Run
			testx.WaitUntil(t, 4*time.Minute, func() bool {
				b, err := f.W.AdeTask.Board(ctx)
				if err != nil {
					return false
				}
				for _, tk := range b.Tasks {
					for _, r := range tk.Runs {
						if r.ID == start.RunID {
							run = r
							return r.State != "pending" && r.State != "running"
						}
					}
				}
				return false
			})
			o := run.Outcome
			if o == nil {
				t.Fatalf("run %+v has no outcome; log:\n%s", run, tail(f.runLog(t, run.ID), 1500))
			}
			t.Logf("state=%s status=%s source=%s reason=%q rebase=%+v report=%+v", run.State, o.Status, o.Source, o.Reason, o.Rebase, o.Report)
			switch {
			case run.State == "done":
				if o.Rebase == nil || !o.Rebase.Verified {
					t.Fatalf("done without a verified rebase: %+v", o.Rebase)
				}
			case c.wantVerified:
				t.Fatalf("clean rebase ended %s: %q; log:\n%s", run.State, o.Reason, tail(f.runLog(t, run.ID), 1500))
			case c.mayConflict:
				reported := (o.Report != nil && len(o.Report.ConflictedFiles) > 0) || (o.Rebase != nil && len(o.Rebase.ConflictedFiles) > 0)
				if !reported && !strings.HasPrefix(o.Reason, "no report") {
					t.Fatalf("conflict ended %s with neither conflicted files nor a no-report reason: %q", run.State, o.Reason)
				}
			}
			f.requireCostUnder(t)
		})
	}
}

// TestAdeReportArtifacts runs a step that reports a doc by ref: the real claude must call
// report_artifacts with a doc, and the task's artifacts hold the row.
func TestAdeReportArtifacts(t *testing.T) {
	f := newAdeFixture(t, nil)
	step := "  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 3m\n" +
		"    prompt: Do not write any file. Call report_artifacts once with one artifact of kind doc, title Notes, " +
		"ref notes.md. Then call finish_step with status pass and summary ok.\n"
	f.saveWorkflow(t, "flow", step+manualStepYAML)
	task := f.createTask(t, "flow")
	run := f.startRun(t, task, "feat/real-claude")
	log := f.runLog(t, run.ID)
	if run.State != "done" {
		t.Fatalf("run = %+v, want done; log:\n%s", run, tail(log, 1500))
	}
	if !strings.Contains(log, "mcp__kira-ade__report_artifacts") {
		t.Fatalf("run log lacks the report_artifacts tool call:\n%s", tail(log, 1500))
	}
	res, err := f.W.AdeTask.Artifacts(ctx, adewire.TaskArgs{TaskID: task.ID})
	if err != nil {
		t.Fatalf("Artifacts: %v", err)
	}
	for _, a := range res.Artifacts {
		if a.Kind == "doc" && a.Ref == "notes.md" {
			f.requireCostUnder(t)
			return
		}
	}
	t.Fatalf("no doc artifact notes.md in %+v; log:\n%s", res.Artifacts, tail(log, 1500))
}
