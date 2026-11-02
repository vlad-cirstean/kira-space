package harnessflow_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/agentctx"
	"github.com/kirathecat/kira-studio/internal/agentsandbox/sandboxtest"
	"github.com/kirathecat/kira-studio/internal/configfs/configsvc"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeprovider"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const contextFlag = "--append-system-prompt-file"

// recorder reads the calls the fake agents recorded, each once.
type recorder struct {
	t    *testing.T
	app  *flowharness.App
	seen map[string]bool
}

func newRecorder(t *testing.T, app *flowharness.App) *recorder {
	return &recorder{t: t, app: app, seen: map[string]bool{}}
}

// next waits for a call not seen yet and returns its argv and the context file bytes it recorded.
func (r *recorder) next() (args, context string) {
	r.t.Helper()
	var stem string
	testx.WaitUntil(r.t, waitFor, func() bool {
		files, _ := filepath.Glob(filepath.Join(r.app.FakeDir, "*.args"))
		for _, f := range files {
			base := filepath.Base(f)
			if strings.HasPrefix(base, "gh-") || strings.HasPrefix(base, "mcp-list-") || r.seen[f] {
				continue
			}
			r.seen[f], stem = true, strings.TrimSuffix(f, ".args")
			return true
		}
		return false
	})
	a, _ := os.ReadFile(stem + ".args")
	c, _ := os.ReadFile(stem + ".context")
	return string(a), string(c)
}

func saveContext(t *testing.T, app *flowharness.App, text string) {
	t.Helper()
	cur, err := app.W.AgentContextSvc.Get()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.AgentContextSvc.Save(agentctx.SaveArgs{Text: text, BaseHash: cur.Hash}); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

func smartRun(t *testing.T, app *flowharness.App, name string) scriptruns.Run {
	t.Helper()
	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
		Name: name, Kind: scripts.KindSmart, Command: "Say hi", Color: "blue", Smart: &scripts.Smart{Model: "fast"},
	}})
	if err != nil {
		t.Fatal(err)
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
	testx.WaitUntil(t, waitFor, func() bool {
		run, _ = app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: started.RunID})
		return run.State == "done"
	})
	return run
}

func openAgent(t *testing.T, app *flowharness.App, l adewire.Launch) {
	t.Helper()
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: l.TerminalID, Cwd: l.Cwd, Cols: 100, Rows: 30, WindowKey: window,
		Command: l.Command, LaunchKind: terminal.LaunchKindAgent,
	}); err != nil {
		t.Fatalf("Terminal.Open: %v", err)
	}
}

// TestAgentContextReachesEveryLaunch: the agent context saved through the editor's bound service
// reaches every kind of launch of both providers: interactive, Take over, Continue in terminal,
// headless ADE steps and smart scripts. Frontend half: ade-agent-context.spec.ts.
func TestAgentContextReachesEveryLaunch(t *testing.T) {
	const marker1, marker2 = "CONTEXT-MARKER-ONE\n", "CONTEXT-MARKER-TWO\n"
	fake := fakeprovider.New(map[string]json.RawMessage{})
	sbx := &sandboxtest.Fake{Dir: t.TempDir()}
	hold := filepath.Join(t.TempDir(), "hold")
	app := flowharness.New(t, flowharness.WithProvider(fake), flowharness.WithTrackerGrace(time.Second), flowharness.WithSandbox(sbx))
	t.Cleanup(func() { _ = os.WriteFile(hold, nil, 0o644) })
	app.Scenario(fakeagent.Scenario{Claude: map[string][]fakeagent.Action{"*": {{WaitFile: hold, Name: "done"}, {Name: "done"}}}})
	rec := newRecorder(t, app)

	// An empty context adds no flag.
	if v, err := app.W.AgentContextSvc.Get(); err != nil || v.Exists || v.MaxBytes != agentctx.MaxBytes || len(v.KeepsOnResume) != 1 || v.KeepsOnResume[0] != "Claude Code" {
		t.Fatalf("Get = %+v, %v, want no file, the cap and Claude keeping context on resume", v, err)
	}
	if path, err := app.W.AgentContext.ContextFile(); path != "" || err != nil {
		t.Fatalf("ContextFile with no file = %q, %v", path, err)
	}
	if err := os.WriteFile(hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	smartRun(t, app, "plain")
	if args, ctx := rec.next(); strings.Contains(args, contextFlag) || ctx != "" {
		t.Fatalf("a launch with no context got %q, %q", args, ctx)
	}
	if err := os.Remove(hold); err != nil {
		t.Fatal(err)
	}

	// The save is the IPC boundary; a stale base hash is a conflict.
	saveContext(t, app, marker1)
	if _, err := app.W.AgentContextSvc.Save(agentctx.SaveArgs{Text: "x", BaseHash: "stale"}); err == nil {
		t.Fatal("Save on a stale hash succeeded")
	}
	if err := app.W.ConfigFiles.Open(configsvc.FileArgs{Folder: agentctx.Folder, File: agentctx.File}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := app.FileOpener.OpenedFiles(); len(got) != 1 || got[0] != filepath.Join(app.SpaceHome, "agent", agentctx.File) {
		t.Fatalf("opened = %v, want the context file", got)
	}

	// Claude interactive: ad hoc session on a task branch.
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: window})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	repoRec, err := app.W.CodeWorkspace.ImportRepo(ctx, bridge.CodeWorkspaceImportArgs{Path: repo.Dir})
	if err != nil {
		t.Fatal(err)
	}
	tk := task(t, app, repoRec.ID, "flow", "id: flow\nname: Flow\nsteps:\n"+
		"  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 1m\n    prompt: work on {branch} in {repo}\n"+
		"  - id: work\n    name: Work\n    kind: manual\n    status: In progress\n    session: true\n")
	row := branchOf(t, app, tk.ID, repoRec.ID)
	br, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: row.ID, Name: "feat/api-ctx"})
	if err != nil {
		t.Fatal(err)
	}
	adhoc, err := app.W.AdeTask.StartAdHoc(ctx, adewire.BranchArgs{BranchID: br.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(adhoc.Command, contextFlag) || strings.Contains(adhoc.Command, marker1) {
		t.Fatalf("interactive command = %q, want the flag with a path, never the text", adhoc.Command)
	}
	openAgent(t, app, adhoc)
	if args, ctx := rec.next(); !strings.Contains(args, contextFlag) || ctx != marker1 {
		t.Fatalf("claude interactive got %q, %q, want the marker", args, ctx)
	}

	// An edit reaches the next launch (Take over); the running session keeps its own.
	saveContext(t, app, marker2)
	if err := os.WriteFile(hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool { return sessionByID(t, app, adhoc.SessionID).State == "stopped" })
	back, err := app.W.AdeTask.TakeOver(ctx, adewire.TakeOverArgs{SessionID: adhoc.SessionID})
	if err != nil || !strings.Contains(back.Command, "--resume") || !strings.Contains(back.Command, contextFlag) {
		t.Fatalf("TakeOver = %+v, %v, want a resume with the flag", back, err)
	}
	openAgent(t, app, back)
	if args, ctx := rec.next(); !strings.Contains(args, contextFlag) || ctx != marker2 {
		t.Fatalf("take over got %q, %q, want the edited marker", args, ctx)
	}

	// Claude headless: an ADE step, a smart script and Continue in terminal.
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: tk.ID}); err != nil {
		t.Fatal(err)
	}
	args, ctxText := rec.next()
	if !strings.Contains(args, "-p\n") || ctxText != marker2 {
		t.Fatalf("headless step got %q, %q, want the marker", args, ctxText)
	}
	testx.WaitUntil(t, waitFor, func() bool { return latestRun(t, app, tk.ID, "one").State == "done" })
	d, err := app.W.AdeTask.RunSandbox(ctx, adewire.RunArgs{RunID: latestRun(t, app, tk.ID, "one").ID})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := app.W.AgentContext.ContextFile()
	if err != nil || snap == "" {
		t.Fatalf("ContextFile = %q, %v", snap, err)
	}
	if real, err := filepath.EvalSymlinks(filepath.Dir(snap)); err != nil || !strings.Contains(d.Profile, real) {
		t.Fatalf("sandbox profile lacks the context dir %s:\n%s", filepath.Dir(snap), d.Profile)
	}
	smart := smartRun(t, app, "ask")
	if args, ctxText := rec.next(); !strings.Contains(args, contextFlag) || ctxText != marker2 {
		t.Fatalf("smart script got %q, %q, want the marker", args, ctxText)
	}
	resume, err := app.W.ScriptRuns.ResumeCommand(scriptruns.ResumeCommandArgs{RunID: smart.ID})
	if err != nil || !strings.Contains(resume.Command, "'"+contextFlag+"' '"+snap+"'") {
		t.Fatalf("ResumeCommand = %+v, %v, want the flag with the snapshot %s", resume, err, snap)
	}
	if got, _ := os.ReadFile(snap); string(got) != marker2 {
		t.Fatalf("snapshot = %q, want %q", got, marker2)
	}

	// The fake provider: interactive and headless runs get the same text.
	setProvider(t, app, fakeprovider.ID)
	step, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: tk.ID})
	if err != nil {
		t.Fatal(err)
	}
	openAgent(t, app, step)
	if args, ctxText := rec.next(); !strings.Contains(args, fakeagent.PersonaContextFlag) || ctxText != marker2 {
		t.Fatalf("fake interactive got %q, %q, want the marker", args, ctxText)
	}
	smartRun(t, app, "ask-fake")
	if h := fake.Headless(); len(h) == 0 || string(h[len(h)-1].Context) != marker2 {
		t.Fatalf("fake headless calls = %+v, want the marker", h)
	}
	setProvider(t, app, "claude")

	// A broken file keeps the last good text in use, across a restart.
	cfgPath := filepath.Join(app.SpaceHome, "agent", agentctx.File)
	mark := app.Events.Mark()
	if err := os.WriteFile(cfgPath, []byte("ok\n\xff\xfe"), 0o644); err != nil {
		t.Fatal(err)
	}
	app.Events.WaitAfter(t, mark, bridge.ChannelAgentContextChanged, nil, waitFor)
	for _, restarted := range []bool{false, true} {
		if restarted {
			app.Restart()
		}
		v, err := app.W.AgentContextSvc.Get()
		if err != nil || v.Problem == nil || v.Problem.Line != 2 || !v.LastGood || v.Editable {
			t.Fatalf("Get on a broken file (restarted %v) = %+v, %v, want line 2, last good, read-only", restarted, v, err)
		}
		if p, err := app.W.AgentContext.ContextFile(); err != nil {
			t.Fatal(err)
		} else if got, _ := os.ReadFile(p); string(got) != marker2 {
			t.Fatalf("last good text = %q, want %q", got, marker2)
		}
		if !restarted {
			var js string
			if err := app.DB().QueryRow(`SELECT value_json FROM config_last_valid WHERE folder = ? AND file = ?`, agentctx.Folder, agentctx.File).Scan(&js); err != nil || !strings.Contains(js, "CONTEXT-MARKER-TWO") {
				t.Fatalf("projection row = %q, %v", js, err)
			}
		}
	}

	// A blank file means no context again.
	if err := os.WriteFile(cfgPath, []byte(" \n\t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p, err := app.W.AgentContext.ContextFile(); p != "" || err != nil {
		t.Fatalf("ContextFile of a blank file = %q, %v", p, err)
	}
}
