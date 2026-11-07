package ade

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/loginshell"
)

// P147 engine scenarios: send-back, stop, recovery, Take over, launches, archive, add-existing.

func (e *engine) allRuns(taskID string) []model.AdeRun {
	e.t.Helper()
	runs, err := e.repos.AdeTasks.RunsOfTask(taskID)
	if err != nil {
		e.t.Fatal(err)
	}
	return runs
}

// run finds one attempt of a step by its attempt number.
func (e *engine) attempt(taskID, step string, n int) model.AdeRun {
	e.t.Helper()
	for _, r := range e.allRuns(taskID) {
		if r.StepID == step && r.Attempt == n {
			return r
		}
	}
	e.t.Fatalf("no attempt %d of %s: %+v", n, step, e.allRuns(taskID))
	return model.AdeRun{}
}

func (e *engine) claudeID(r model.AdeRun) string {
	e.t.Helper()
	rec, err := e.repos.AdeSessions.Get(r.SessionID)
	if err != nil || rec == nil {
		e.t.Fatalf("session of run %s: %v, %v", r.ID, rec, err)
	}
	return rec.AgentSessionID
}

func TestRunEngine_sendBack(t *testing.T) {
	impl := agentStep("impl", "")
	tests := agentStep("tests", backTo("impl"))

	t.Run("round trip resumes the same Claude session", func(t *testing.T) {
		e := newEngine(t, map[string][]string{"*": {"done", "failed", "done", "done"}})
		e.repo("api")
		e.workflow(flowYAML(impl + tests))
		id := e.task("api")
		e.start(id)
		final := e.waitRun(id, "tests/api", model.AdeRunDone)
		if final.Attempt != 2 || final.Loops != 1 {
			t.Fatalf("final tests run = %+v; want attempt 2 loops 1", final)
		}
		back := e.attempt(id, "tests", 1)
		if back.State != model.AdeRunBack || back.Note != "summary-failed → back to impl (1 of 3)" {
			t.Fatalf("failed tests run = %+v", back)
		}
		fix := e.attempt(id, "impl", 2)
		if fix.State != model.AdeRunDone || fix.Loops != 1 {
			t.Fatalf("fix run = %+v", fix)
		}
		first := e.claudeID(e.attempt(id, "impl", 1))
		if got := e.claudeID(fix); got != first {
			t.Fatalf("fix session id = %s, want the first run's %s", got, first)
		}
		if args := e.fakeFile("api-1.args"); !strings.Contains(args, "--session-id\n"+first) {
			t.Fatalf("first run argv lacks --session-id %s:\n%s", first, args)
		}
		args := e.fakeFile("api-3.args")
		if !strings.Contains(args, "--resume\n"+first) || strings.Contains(args, "--session-id") {
			t.Fatalf("fix run argv = %s", args)
		}
		prompt := e.fakeFile("api-3.prompt")
		if !strings.Contains(prompt, "tests failed on feat/fix-login: summary-failed. Fix the implementation.") ||
			!strings.Contains(prompt, agentmcp.FinishInstruction(llmprompts.Defaults(), nil)) || strings.Contains(prompt, "Step 1/2") {
			t.Fatalf("fix prompt = %q", prompt)
		}
		rec, _ := e.repos.AdeSessions.Get(fix.SessionID)
		if rec.Resumes != first {
			t.Fatalf("fix session resumes %q, want %s", rec.Resumes, first)
		}
	})

	t.Run("three rounds then failed", func(t *testing.T) {
		e := newEngine(t, map[string][]string{"*": {"done", "failed", "done", "failed", "done", "failed", "done", "failed"}})
		e.repo("api")
		e.workflow(flowYAML(impl + tests))
		id := e.task("api")
		e.start(id)
		r := e.waitRun(id, "tests/api", model.AdeRunFailed)
		if r.Attempt != 4 || r.Note != "Stopped: tests reached its limit of 3 returns." {
			t.Fatalf("capped tests run = %+v", r)
		}
		if n, _ := e.repos.AdeTasks.CountRuns(id, "tests", r.BranchID, model.AdeRunBack); n != 3 {
			t.Fatalf("back runs = %d, want 3", n)
		}
		time.Sleep(300 * time.Millisecond)
		if latest := e.runs(id)["tests/api"]; latest.ID != r.ID || e.runs(id)["impl/api"].Attempt != 4 {
			t.Fatalf("engine moved on after the cap: %+v", e.runs(id))
		}
	})

	t.Run("steps between the target and the failer rerun", func(t *testing.T) {
		e := newEngine(t, map[string][]string{"*": {"done", "done", "failed", "done", "done", "done"}})
		e.repo("api")
		ci := agentStep("ci", backTo("impl"))
		e.workflow(flowYAML(impl + agentStep("tests", "") + ci))
		id := e.task("api")
		e.start(id)
		e.waitRun(id, "ci/api", model.AdeRunDone)
		if r := e.runs(id)["tests/api"]; r.Attempt != 2 || r.Loops != 1 || r.State != model.AdeRunDone {
			t.Fatalf("tests run = %+v; want attempt 2 loops 1 done", r)
		}
		if r := e.runs(id)["ci/api"]; r.Attempt != 2 || r.Loops != 1 {
			t.Fatalf("ci run = %+v", r)
		}
	})

	t.Run("target that did not run on the branch", func(t *testing.T) {
		e := newEngine(t, map[string][]string{"web": {"done", "done"}, "api": {"failed"}})
		e.repo("api")
		e.repo("web")
		webOnly := strings.Replace(impl, "each repo", "only web", 1)
		e.workflow(flowYAML(webOnly + tests))
		id := e.task("api", "web")
		e.start(id)
		r := e.waitRun(id, "tests/api", model.AdeRunFailed)
		if r.Note != "cannot send back: impl did not run on feat/fix-login" {
			t.Fatalf("note = %q", r.Note)
		}
	})
}

func TestRunEngine_stopRun(t *testing.T) {
	t.Run("running run becomes stuck and nothing advances", func(t *testing.T) {
		e := newEngine(t, map[string][]string{"*": {"sleep"}})
		e.repo("api")
		e.workflow(flowYAML(agentStep("first", "") + agentStep("second", "")))
		id := e.task("api")
		e.start(id)
		run := e.waitRun(id, "first/api", model.AdeRunRunning)
		if err := e.board.StopRun(context.Background(), run.ID); err != nil {
			t.Fatal(err)
		}
		got := e.runs(id)["first/api"]
		if got.State != model.AdeRunStuck || got.Note != "stopped by you" {
			t.Fatalf("stopped run = %+v", got)
		}
		time.Sleep(300 * time.Millisecond)
		if _, started := e.runs(id)["second/api"]; started {
			t.Fatal("a user stop advanced the task")
		}
		if err := e.board.StopRun(context.Background(), run.ID); err == nil {
			t.Fatal("StopRun accepted a stuck run")
		}
	})
	t.Run("pending run does not launch when its setup finishes", func(t *testing.T) {
		e := newEngine(t, map[string][]string{"*": {"done"}})
		e.repo("api")
		gate := filepath.Join(t.TempDir(), "gate")
		e.prepare("api", "while [ ! -f "+gate+" ]; do sleep 0.05; done")
		e.workflow(flowYAML(agentStep("first", "")))
		id := e.task("api")
		e.start(id)
		run := e.runs(id)["first/api"]
		if run.State != model.AdeRunPending {
			t.Fatalf("run = %+v", run)
		}
		if err := e.board.StopRun(context.Background(), run.ID); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(gate, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		branch := branchOf(t, e, id, "api")
		waitUntil(t, "setup ready", func() bool {
			s, _ := e.repos.AdeTasks.GetSetup(branch)
			return s != nil && s.State == model.AdeSetupReady
		})
		time.Sleep(300 * time.Millisecond)
		if got := e.runs(id)["first/api"]; got.State != model.AdeRunStuck || got.Note != "stopped by you" {
			t.Fatalf("stopped pending run = %+v", got)
		}
	})
}

func TestRunEngine_recover(t *testing.T) {
	e := newEngine(t, nil)
	e.repo("api")
	e.workflow(flowYAML(agentStep("first", "") + agentStep("second", "")))
	id := e.task("api")
	branch := branchOf(t, e, id, "api")
	now := time.Now().UnixMilli()
	seed := func(r model.AdeRun) {
		if err := e.repos.AdeTasks.InsertRun(r); err != nil {
			t.Fatal(err)
		}
	}
	seed(model.AdeRun{ID: "r-running", TaskID: id, StepID: "first", BranchID: branch, Attempt: 1, State: model.AdeRunRunning, StartedAt: &now})
	seed(model.AdeRun{ID: "r-pending", TaskID: id, StepID: "second", BranchID: branch, Attempt: 1, State: model.AdeRunPending, Note: noteWaitingSetup})
	if err := e.repos.AdeTasks.UpsertSetup(model.AdeWorktreeSetup{BranchID: branch, State: model.AdeSetupRunning, StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := e.repos.AdeLogs.Reset("setup", branch, id); err != nil {
		t.Fatal(err)
	}
	if err := e.repos.AdeSessions.InsertHeadless(model.AdeSession{
		ID: "s-head", Mode: model.AdeSessionModeHeadless, TaskID: id, BranchID: branch, StepID: "first",
		RunID: "r-running", AgentSessionID: "c1", Cwd: "/x", State: model.AdeSessionStateRunning, StartedAt: now, LastActiveAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.repos.AdeSessions.InsertTUI(model.AdeSession{
		ID: "s-tui", Mode: model.AdeSessionModeTUI, TaskID: id, BranchID: branch, AgentSessionID: "c2", Cwd: "/x",
		State: model.AdeSessionStateRunning, TerminalID: "term-1", StartedAt: now, LastActiveAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(e.board.deps.AgentDir, "r-running.mcp.json")
	if err := os.MkdirAll(e.board.deps.AgentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := e.board.Recover(); err != nil {
		t.Fatal(err)
	}
	if r, _ := e.repos.AdeTasks.GetRun("r-running"); r.State != model.AdeRunStuck || r.Note != restartOutcome.Reason || r.FinishedAt == nil {
		t.Fatalf("running run = %+v", r)
	}
	if r, _ := e.repos.AdeTasks.GetRun("r-pending"); r.State != model.AdeRunPending || r.Note != noteSetupFailed {
		t.Fatalf("pending run = %+v", r)
	}
	if s, _ := e.repos.AdeTasks.GetSetup(branch); s == nil || s.State != model.AdeSetupFailed {
		t.Fatalf("setup = %+v", s)
	}
	if !strings.Contains(e.logText("setup", branch), restartOutcome.Reason) {
		t.Fatal("setup log lacks the interruption line")
	}
	for _, sid := range []string{"s-head", "s-tui"} {
		if rec, _ := e.repos.AdeSessions.Get(sid); rec.State != model.AdeSessionStateStopped || rec.TerminalID != "" {
			t.Fatalf("session %s = %+v", sid, rec)
		}
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale MCP config survived: %v", err)
	}
}

// A held fix run keeps its send-back spec across Recover and launches as a resume (P156).
func TestRunEngine_recoverHeldFixRun(t *testing.T) {
	ctx := context.Background()
	e := newEngine(t, map[string][]string{"*": {"sleep", "done", "done"}})
	e.repo("api")
	e.workflow(flowYAML(agentStep("impl", "") + agentStep("tests", backTo("impl"))))
	id := e.task("api")
	e.start(id)
	first := e.waitRun(id, "impl/api", model.AdeRunRunning)
	waitUntil(t, "fake claude started", func() bool { _, err := os.Stat(filepath.Join(e.fake, "api.count")); return err == nil })
	if err := e.board.StopRun(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	claudeID := e.claudeID(first)
	branch := first.BranchID
	now := time.Now().UnixMilli()
	seed := func(r model.AdeRun) {
		if err := e.repos.AdeTasks.InsertRun(r); err != nil {
			t.Fatal(err)
		}
	}
	seed(model.AdeRun{ID: "r-tests1", TaskID: id, StepID: "tests", BranchID: branch, Attempt: 1,
		State: model.AdeRunBack, Note: "x → back to impl (1 of 3)", StartedAt: &now, FinishedAt: &now})
	spec := model.AdeRunLaunch{Note: "sent back by tests: x", ResumeID: claudeID, Prompt: "tests failed on feat/fix-login: x. Fix the implementation."}
	seed(model.AdeRun{ID: "r-fix", TaskID: id, StepID: "impl", BranchID: branch, Attempt: 2,
		State: model.AdeRunPending, Loops: 1, Note: noteWaitingSetup, Launch: spec})
	if err := e.repos.AdeTasks.UpsertSetup(model.AdeWorktreeSetup{BranchID: branch, State: model.AdeSetupRunning, StartedAt: now}); err != nil {
		t.Fatal(err)
	}

	if err := e.board.Recover(); err != nil {
		t.Fatal(err)
	}
	if r, _ := e.repos.AdeTasks.GetRun("r-fix"); r.State != model.AdeRunPending || r.Note != noteSetupFailed || r.Launch != spec {
		t.Fatalf("held fix run after Recover = %+v", r)
	}
	if err := e.board.RetrySetup(ctx, branch); err != nil {
		t.Fatal(err)
	}
	final := e.waitRun(id, "tests/api", model.AdeRunDone)
	if final.Attempt != 2 || final.Loops != 1 {
		t.Fatalf("final tests run = %+v; want attempt 2 loops 1", final)
	}
	fix, _ := e.repos.AdeTasks.GetRun("r-fix")
	if fix.State != model.AdeRunDone || fix.Launch != (model.AdeRunLaunch{}) {
		t.Fatalf("fix run = %+v; want done, spec cleared", fix)
	}
	if args := e.fakeFile("api-2.args"); !strings.Contains(args, "--resume\n"+claudeID) || strings.Contains(args, "--session-id") {
		t.Fatalf("fix run argv = %s", args)
	}
	if prompt := e.fakeFile("api-2.prompt"); !strings.Contains(prompt, spec.Prompt) ||
		!strings.Contains(prompt, agentmcp.FinishInstruction(llmprompts.Defaults(), nil)) || strings.Contains(prompt, "Step 1/2") {
		t.Fatalf("fix prompt = %q", prompt)
	}
	if rec, _ := e.repos.AdeSessions.Get(fix.SessionID); rec == nil || rec.Resumes != claudeID {
		t.Fatalf("fix session = %+v; want resumes %s", rec, claudeID)
	}
}

// quotedAfter reads the single-quoted word that follows a quoted flag in a composed command.
func quotedAfter(t *testing.T, command, flag string) string {
	t.Helper()
	marker := "'" + flag + "' '"
	i := strings.Index(command, marker)
	if i < 0 {
		t.Fatalf("command lacks %s: %s", flag, command)
	}
	rest := command[i+len(marker):]
	end := strings.Index(rest, "'")
	if end < 0 {
		t.Fatalf("unterminated word after %s: %s", flag, command)
	}
	return rest[:end]
}

// spawn composes a launch the way the terminal host would and registers it live.
func (e *engine) spawn(l adewire.Launch) string {
	e.t.Helper()
	composed, _, err := e.tracker.Compose(l.TerminalID, "", l.Command)
	if err != nil {
		e.t.Fatalf("Compose: %v", err)
	}
	e.live.add(l.TerminalID)
	return composed
}

func TestRunEngine_takeOver(t *testing.T) {
	ctx := context.Background()
	e := newEngine(t, map[string][]string{"*": {"sleep", "done"}})
	e.repo("api")
	e.workflow(flowYAML(agentStep("first", "") + agentStep("second", "")))
	id := e.task("api")
	e.start(id)
	run := e.waitRun(id, "first/api", model.AdeRunRunning)
	head, _ := e.repos.AdeSessions.Get(run.SessionID)
	waitUntil(t, "fake claude started", func() bool { _, err := os.Stat(filepath.Join(e.fake, "api.count")); return err == nil })

	if _, err := e.board.TakeOver(ctx, adewire.TakeOverArgs{SessionID: head.ID}); err == nil || !strings.Contains(err.Error(), "stop it first") {
		t.Fatalf("running without stop: %v", err)
	}
	l, err := e.board.TakeOver(ctx, adewire.TakeOverArgs{SessionID: head.ID, StopIfRunning: true})
	if err != nil {
		t.Fatal(err)
	}
	if r := e.runs(id)["first/api"]; r.State != model.AdeRunStuck || r.Note != "taken over in Claude Code" {
		t.Fatalf("taken-over run = %+v", r)
	}
	if l.Cwd != head.Cwd {
		t.Fatalf("cwd = %s, want the run's worktree %s", l.Cwd, head.Cwd)
	}
	composed := e.spawn(l)
	if !strings.HasPrefix(composed, e.tracker.deps.AgentBin+" --resume '"+head.AgentSessionID+"' ") ||
		!strings.Contains(composed, "'--allowedTools' '"+toolName(agentmcp.FinishStep)+"'") {
		t.Fatalf("command = %s", composed)
	}
	rec, _ := e.repos.AdeSessions.Get(l.SessionID)
	if rec == nil || rec.Mode != model.AdeSessionModeTUI || rec.Resumes != head.AgentSessionID || rec.AgentSessionID != head.AgentSessionID ||
		rec.RunID != "" || rec.TaskID != id || rec.BranchID != head.BranchID || rec.StepID != "first" {
		t.Fatalf("tui record = %+v", rec)
	}
	if _, err := e.board.TakeOver(ctx, adewire.TakeOverArgs{SessionID: head.ID}); err == nil || !strings.Contains(err.Error(), "already open") {
		t.Fatalf("second take over: %v", err)
	}
	if err := e.board.Send(ctx, adewire.SendArgs{SessionID: head.ID, Message: "x"}); err == nil || !strings.Contains(err.Error(), "Take over") {
		t.Fatalf("Send to a headless row: %v", err)
	}
	if err := e.board.Send(ctx, adewire.SendArgs{SessionID: l.SessionID, Message: "hello"}); err != nil {
		t.Fatal(err)
	}

	cfg := quotedAfter(t, composed, "--mcp-config")
	if err := fakeFinish(cfg, "done", "tui-done"); err != nil {
		t.Fatal(err)
	}
	if r := e.waitRun(id, "first/api", model.AdeRunDone); r.Summary != "tui-done" {
		t.Fatalf("finished run = %+v", r)
	}
	e.waitRun(id, "second/api", model.AdeRunDone)

	e.live.remove(l.TerminalID)
	waitUntil(t, "tui record stopped", func() bool {
		e.tracker.Reconcile()
		r, _ := e.repos.AdeSessions.Get(l.SessionID)
		return r.State == model.AdeSessionStateStopped
	})
	waitUntil(t, "mcp config released", func() bool { _, err := os.Stat(cfg); return os.IsNotExist(err) })
}

const manualSessionStep = "  - id: spec\n    name: Spec\n    kind: manual\n    status: todo\n    session: true\n    prompt: Write the spec for {task}\n"

// handoverLine ends every interactive launch message, after the Kira Space note.
var handoverLine = llmprompts.HeadlessSpaceSuffix.Text(llmprompts.Defaults(), llmprompts.NoVars{}) + "\n" +
	llmprompts.AdeHandoverLine.Text(llmprompts.Defaults(), llmprompts.NoVars{})

func TestRunEngine_launchStep(t *testing.T) {
	ctx := context.Background()
	e := newEngine(t, nil)
	e.repo("api")
	e.workflow(flowYAML(manualSessionStep + agentStep("one", "")))
	id := e.task("api")

	l, err := e.board.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: id})
	if err != nil {
		t.Fatal(err)
	}
	if l.Cwd != e.code["api"].Root {
		t.Fatalf("cwd = %s, want the repo root %s", l.Cwd, e.code["api"].Root)
	}
	composed := e.spawn(l)
	want := " -- " + loginshell.Quote("Spec: Fix login\n- Repo: api (read only, in "+e.code["api"].Root+")\nWrite the spec for Fix login\n"+handoverLine)
	if !strings.HasSuffix(composed, want) {
		t.Fatalf("command = %q\nwant suffix %q", composed, want)
	}
	if _, err := e.board.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: id}); err == nil || !strings.Contains(err.Error(), "Spec session is already running") {
		t.Fatalf("second LaunchStep: %v", err)
	}
	if _, err := e.board.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: "nope"}); err == nil {
		t.Fatal("LaunchStep accepted an unknown task")
	}
}

func TestRunEngine_startBranch(t *testing.T) {
	ctx := context.Background()
	t.Run("default message, one session per branch", func(t *testing.T) {
		e := newEngine(t, map[string][]string{"*": {"done"}})
		e.repo("api")
		e.workflow(flowYAML(agentStep("one", "")))
		id := e.task("api")
		if _, err := e.board.StartBranch(ctx, adewire.StartBranchArgs{BranchID: branchOf(t, e, id, "api")}); err == nil || !strings.Contains(err.Error(), "not created") {
			t.Fatalf("uncreated branch: %v", err)
		}
		e.start(id)
		e.waitRun(id, "one/api", model.AdeRunDone)
		l, err := e.board.StartBranch(ctx, adewire.StartBranchArgs{BranchID: branchOf(t, e, id, "api")})
		if err != nil {
			t.Fatal(err)
		}
		composed := e.spawn(l)
		if want := " -- " + loginshell.Quote("Task: Fix login\n- Repo: api · Branch: feat/fix-login · Worktree: "+l.Cwd+"\nHandover from one:\nsummary-done\n"+handoverLine); !strings.HasSuffix(composed, want) {
			t.Fatalf("command = %q\nwant suffix %q", composed, want)
		}
		if _, err := e.board.StartBranch(ctx, adewire.StartBranchArgs{BranchID: branchOf(t, e, id, "api")}); err == nil || !strings.Contains(err.Error(), "already open") {
			t.Fatalf("second StartBranch: %v", err)
		}
	})
	t.Run("a running background run blocks it", func(t *testing.T) {
		e := newEngine(t, map[string][]string{"*": {"sleep"}})
		e.repo("api")
		e.workflow(flowYAML(agentStep("one", "")))
		id := e.task("api")
		e.start(id)
		e.waitRun(id, "one/api", model.AdeRunRunning)
		if _, err := e.board.StartBranch(ctx, adewire.StartBranchArgs{BranchID: branchOf(t, e, id, "api")}); err == nil || !strings.Contains(err.Error(), "background run") {
			t.Fatalf("StartBranch beside a run: %v", err)
		}
	})
	t.Run("an unfinished setup blocks it", func(t *testing.T) {
		e := newEngine(t, map[string][]string{"*": {"done"}})
		e.repo("api")
		gate := filepath.Join(t.TempDir(), "gate")
		e.prepare("api", "while [ ! -f "+gate+" ]; do sleep 0.05; done")
		e.workflow(flowYAML(agentStep("one", "")))
		id := e.task("api")
		e.start(id)
		_, err := e.board.StartBranch(ctx, adewire.StartBranchArgs{BranchID: branchOf(t, e, id, "api")})
		if !errors.Is(err, ErrSetupPending) {
			t.Fatalf("StartBranch during setup: %v", err)
		}
		_ = os.WriteFile(gate, nil, 0o644)
	})
}

func TestRunEngine_archive(t *testing.T) {
	ctx := context.Background()
	e := newEngine(t, map[string][]string{"*": {"sleep"}})
	e.repo("api")
	e.workflow(flowYAML(agentStep("one", "")))
	id := e.task("api")
	run := (func() model.AdeRun { e.start(id); return e.waitRun(id, "one/api", model.AdeRunRunning) })()
	head, _ := e.repos.AdeSessions.Get(run.SessionID)
	wt := head.Cwd
	runGitQueue(t, wt, "config", "user.name", queueMineName)
	runGitQueue(t, wt, "config", "user.email", queueMineEmail)
	writeQueueFile(t, wt, "work.txt", "x\n")
	runGitQueue(t, wt, "add", "work.txt")
	runGitQueue(t, wt, "commit", "-q", "-m", "work")
	writeQueueFile(t, wt, "dirty.txt", "y\n")

	risk, err := e.board.ArchiveRisk(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(risk.Branches) != 1 {
		t.Fatalf("risk = %+v", risk)
	}
	br := risk.Branches[0]
	if br.Worktree != wt || br.Unmerged != 1 || len(br.Dirty) != 1 || br.Dirty[0].Path != "dirty.txt" || br.Blocked != "" {
		t.Fatalf("branch risk = %+v", br)
	}

	runGitQueue(t, e.code["api"].Root, "worktree", "lock", wt)
	if risk, _ = e.board.ArchiveRisk(ctx, id); risk.Branches[0].Blocked == "" {
		t.Fatal("a locked worktree is not reported blocked")
	}
	if err := e.board.ArchiveTask(ctx, id); err == nil || !strings.Contains(err.Error(), "cannot archive") {
		t.Fatalf("blocked archive: %v", err)
	}
	if got := e.runs(id)["one/api"]; got.State != model.AdeRunRunning {
		t.Fatalf("refused archive touched the run: %+v", got)
	}
	if task, _ := e.repos.AdeTasks.GetTask(id); task.ArchivedAt != nil {
		t.Fatal("refused archive archived the task")
	}
	runGitQueue(t, e.code["api"].Root, "worktree", "unlock", wt)

	if err := e.board.ArchiveTask(ctx, id); err != nil {
		t.Fatal(err)
	}
	if got := e.attempt(id, "one", 1); got.State != model.AdeRunFailed || got.Note != "stopped: task archived" {
		t.Fatalf("archived run = %+v", got)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("worktree survived: %v", err)
	}
	board, err := e.board.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range board.History {
		found = found || h.TaskID == id
	}
	if !found || len(board.Tasks) != 0 {
		t.Fatalf("history = %+v, live tasks = %d", board.History, len(board.Tasks))
	}
	if _, err := e.board.ArchiveRisk(ctx, id); err == nil {
		t.Fatal("ArchiveRisk accepted an archived task")
	}
}

func TestRunEngine_addExistingBranchWorktree(t *testing.T) {
	ctx := context.Background()
	e := newEngine(t, nil)
	e.repo("api")
	root := e.code["api"].Root
	runGitQueue(t, root, "branch", "feat/mine", "main")
	runGitQueue(t, root, "checkout", "-q", "-b", "feat/theirs", "main")
	writeQueueFile(t, root, "other.txt", "o\n")
	runGitQueue(t, root, "add", "other.txt")
	runGitQueueAs(t, root, queueOtherName, queueOtherEmail, "commit", "-q", "-m", "other")
	runGitQueue(t, root, "checkout", "-q", "main")

	mine, err := e.board.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: "api", Name: "feat/mine"})
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := e.repos.AdeTasks.GetSetup(mine.Branch.ID); s == nil || s.State != model.AdeSetupReady {
		t.Fatalf("mine setup = %+v", s)
	}
	sb, _ := e.repos.AdeTasks.GetBranch(mine.Branch.ID)
	if path, _ := e.board.worktreeOf(ctx, sb); path == "" {
		t.Fatal("mine branch has no worktree")
	}

	review, err := e.board.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: "api", Name: "feat/theirs"})
	if err != nil {
		t.Fatal(err)
	}
	if review.Task.Kind != model.AdeTaskKindReview {
		t.Fatalf("task kind = %s, want review", review.Task.Kind)
	}
	if s, _ := e.repos.AdeTasks.GetSetup(review.Branch.ID); s != nil {
		t.Fatalf("review branch got a setup: %+v", s)
	}
	rb, _ := e.repos.AdeTasks.GetBranch(review.Branch.ID)
	if path, _ := e.board.worktreeOf(ctx, rb); path != "" {
		t.Fatalf("review branch got a worktree: %s", path)
	}
}

func TestRunEngine_takeOverConcurrentOpensOnce(t *testing.T) {
	ctx := context.Background()
	e := newEngine(t, map[string][]string{"*": {"sleep", "done"}})
	e.repo("api")
	e.workflow(flowYAML(agentStep("first", "") + agentStep("second", "")))
	id := e.task("api")
	e.start(id)
	run := e.waitRun(id, "first/api", model.AdeRunRunning)
	head, _ := e.repos.AdeSessions.Get(run.SessionID)
	waitUntil(t, "fake claude started", func() bool { _, err := os.Stat(filepath.Join(e.fake, "api.count")); return err == nil })

	const callers = 4
	errs := make(chan error, callers)
	for range callers {
		go func() {
			_, err := e.board.TakeOver(ctx, adewire.TakeOverArgs{SessionID: head.ID, StopIfRunning: true})
			errs <- err
		}()
	}
	opened := 0
	for range callers {
		if err := <-errs; err == nil {
			opened++
		}
	}
	if opened != 1 {
		t.Fatalf("%d concurrent take overs opened the conversation, want 1", opened)
	}
}

func TestRunEngine_startBranchReleasesRunHeldForMissingWorktree(t *testing.T) {
	ctx := context.Background()
	e := newEngine(t, map[string][]string{"*": {"done"}})
	e.repo("api")
	e.workflow(flowYAML(agentStep("one", "") + agentStep("two", "    before: approval\n")))
	id := e.task("api")
	e.start(id)
	e.waitRun(id, "one/api", model.AdeRunDone)
	sb, err := e.repos.AdeTasks.GetBranch(branchOf(t, e, id, "api"))
	if err != nil {
		t.Fatal(err)
	}
	wt, err := e.board.worktreeOf(ctx, sb)
	if err != nil || wt == "" {
		t.Fatalf("worktree = %q, %v", wt, err)
	}
	runGitQueue(t, e.code["api"].Root, "worktree", "remove", "--force", wt)
	if err := e.board.Approve(ctx, adewire.StepArgs{TaskID: id, StepID: "two"}); err != nil {
		t.Fatal(err)
	}
	if got := e.runs(id)["two/api"]; got.State != model.AdeRunPending || got.Note != noteWorktreeGone {
		t.Fatalf("held run = %+v", got)
	}
	// Start recreates the worktree; the released run then holds the branch, so Start itself is refused.
	_, _ = e.board.StartBranch(ctx, adewire.StartBranchArgs{BranchID: sb.ID})
	e.waitRun(id, "two/api", model.AdeRunRunning, model.AdeRunDone)
}
