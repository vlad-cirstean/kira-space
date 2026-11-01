package harnessflow_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/appsettings"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeprovider"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/memory/importer"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

var ctx = context.Background()

const (
	waitFor       = 20 * time.Second
	extractSystem = "extract facts and rules"
	mergeSystem   = "merge the facts extracted from one input"
	evidence      = "the worker keeps port"
	text          = "## Worker\n\nThe worker keeps port 8000 open and the queue never retries."
	window        = "w-main"
)

var mergedLines = []importer.Line{
	{Text: "Worker keeps port 8000 open.", Kind: "fact"},
	{Text: "Never retry the queue, because duplicates corrupt totals.", Kind: "rule"},
}

// extractOut and mergeOut are the structured answers both providers give.
func extractOut() map[string]any {
	return map[string]any{"facts": []map[string]string{
		{"fact": "Worker keeps port 8000 open.", "evidence": evidence, "section": "", "unresolved": ""},
		{"fact": "Queue never retries.", "evidence": evidence, "section": "", "unresolved": ""},
	}}
}

func mergeOut() map[string]any {
	return map[string]any{
		"lines":      mergedLines,
		"unresolved": []map[string]any{},
		"dropped":    []map[string]string{},
	}
}

func raw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// claudeAnswer writes the fake claude's canned reply to a structured call.
func claudeAnswer(t *testing.T, structured any) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "answer.json")
	body := raw(t, map[string]any{"type": "result", "subtype": "success", "is_error": false, "total_cost_usd": 0.01, "structured_output": structured})
	if err := os.WriteFile(p, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func setProvider(t *testing.T, app *flowharness.App, id string) {
	t.Helper()
	patch := model.SettingsPatch{Agent: &model.AgentPatch{AgentPatch: appsettings.AgentPatch{Provider: &id}}}
	if _, err := app.W.Settings.Set(bridge.SettingsSetArgs{Patch: patch}); err != nil {
		t.Fatalf("set agent.provider %s: %v", id, err)
	}
}

func distill(t *testing.T, app *flowharness.App, id string) []importer.Line {
	t.Helper()
	res, err := app.W.MemoryDistill.Distill(ctx, bridge.MemoryDistillArgs{RequestID: id, Text: text})
	if err != nil {
		t.Fatalf("Distill %s: %v", id, err)
	}
	return res.Lines
}

// claudeCalls counts the calls the fake claude recorded; the generic persona's own are not.
func claudeCalls(t *testing.T, app *flowharness.App) int {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(app.FakeDir, "*.args"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range files {
		if !strings.HasPrefix(filepath.Base(f), fakeagent.Persona) {
			n++
		}
	}
	return n
}

func wantGone(t *testing.T, err error) {
	t.Helper()
	var ie *ipcerr.Error
	if !errors.As(err, &ie) || ie.Code != string(ipcerr.CodeConflict) || ie.Message != i18n.T("backend.core.agent.providerGone.body", nil) {
		t.Fatalf("err = %v, want the provider gone conflict", err)
	}
}

// TestSecondProviderRegistered: a provider registered next to claude, and chosen through
// agent.provider, runs text distill, a smart script and the ADE with no caller change; a stored
// conversation keeps its provider; an unknown id falls back; an unregistered provider is reported.
// Frontend half: agent-provider.spec.ts.
func TestSecondProviderRegistered(t *testing.T) {
	fake := fakeprovider.New(map[string]json.RawMessage{})
	fake.Answers[extractSystem] = raw(t, extractOut())
	fake.Answers[mergeSystem] = raw(t, mergeOut())
	app := flowharness.New(t, flowharness.WithProvider(fake), flowharness.WithTrackerGrace(time.Second))
	usage := app.W.AgentUsage.Get()

	// 1. The provider's facts reach the display call.
	setProvider(t, app, fakeprovider.ID)
	info, err := app.W.AgentSvc.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "fake" || info.DefaultModel != "fast" || len(info.Models) != 2 || info.Models[1].ID != "deep" ||
		info.Command != fakeagent.Persona || info.Hooks || info.Usage {
		t.Fatalf("Info = %+v, want the fake provider with no hooks or usage", info)
	}

	app.Contract(t, "agent", "AgentService.Info#fake", info)

	// 2. Text distill runs on the fake at its default model.
	if got := distill(t, app, "d1"); !reflect.DeepEqual(got, mergedLines) {
		t.Fatalf("lines = %+v, want the fake's", got)
	}
	calls := fake.Structured()
	if len(calls) < 2 {
		t.Fatalf("fake saw %d structured calls, want extract and merge", len(calls))
	}
	for _, c := range calls {
		if c.Model != "fast" {
			t.Fatalf("structured model = %q, want fast", c.Model)
		}
	}

	// 3. A smart script runs headless on the fake with its own model; the run keeps the provider.
	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
		Name: "ask", Kind: scripts.KindSmart, Command: "Say hi", Color: "blue", Smart: &scripts.Smart{Model: "deep"},
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
	var smart scriptruns.Run
	testx.WaitUntil(t, waitFor, func() bool {
		smart, _ = app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: started.RunID})
		return smart.State == "done"
	})
	if smart.Provider != "fake" || smart.SessionID == "" || smart.Outcome == nil || smart.Outcome.Summary != "summary-fake" {
		t.Fatalf("smart run = %+v, want a done run of the fake", smart)
	}
	head := fake.Headless()
	if len(head) != 1 || head[0].Model != "deep" || !strings.Contains(head[0].Prompt, "Say hi") {
		t.Fatalf("fake headless calls = %+v, want one at model deep", head)
	}

	// A restart-interrupted run keeps its session as the resume point.
	if _, err := app.DB().Exec(`UPDATE script_runs SET resume_id = session_id WHERE id = ?`, smart.ID); err != nil {
		t.Fatal(err)
	}

	// 4. The ADE: a headless step run, then an interactive session, with no hooks to compose.
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	repoRec, err := app.W.CodeWorkspace.ImportRepo(ctx, bridge.CodeWorkspaceImportArgs{Path: repo.Dir})
	if err != nil {
		t.Fatal(err)
	}
	flow := "id: flow\nname: Flow\nsteps:\n" +
		"  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 1m\n    prompt: work on {branch} in {repo}\n" +
		"  - id: work\n    name: Work\n    kind: manual\n    status: In progress\n    session: true\n"
	if entry, err := app.W.AdeTask.SaveWorkflowYaml(ctx, adewire.SaveWorkflowYamlArgs{FileName: "flow.yaml", Yaml: flow}); err != nil || entry.Error != nil {
		t.Fatalf("SaveWorkflowYaml: %v %+v", err, entry.Error)
	}
	task, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Fix login", CodeRepoIDs: []string{repoRec.ID}, WorkflowID: "flow"})
	if err != nil {
		t.Fatal(err)
	}
	br := branchOf(t, app, task.ID, repoRec.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{br.ID: "feat/api-work"}}); err != nil {
		t.Fatal(err)
	}
	var run adewire.Run
	testx.WaitUntil(t, waitFor, func() bool {
		run = latestRun(t, app, task.ID, "one")
		return run.State == "done"
	})
	headless := sessionByID(t, app, run.SessionID)
	if headless.Provider != "fake" || headless.Mode != "headless" || headless.AgentSessionID == "" {
		t.Fatalf("headless session = %+v, want the fake's", headless)
	}

	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: window})
	step, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(step.Command, fakeagent.Persona+" --session ") || strings.Contains(step.Command, "--settings") {
		t.Fatalf("interactive command = %q, want the fake's, composed with no hook settings", step.Command)
	}
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: step.TerminalID, Cwd: step.Cwd, Cols: 100, Rows: 30, WindowKey: window,
		Command: step.Command, LaunchKind: terminal.LaunchKindAgent,
	}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool { return sessionByID(t, app, step.SessionID).State == "running" })
	if s := sessionByID(t, app, step.SessionID); s.Provider != "fake" {
		t.Fatalf("interactive session = %+v, want the fake's", s)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		b, _ := os.ReadFile(filepath.Join(app.FakeDir, fakeagent.Persona+"-1.args"))
		return strings.HasPrefix(string(b), "--session\n")
	})
	if got := app.W.AgentUsage.Get(); !reflect.DeepEqual(got, usage) {
		t.Fatalf("usage = %+v, want %+v: the fake feeds none", got, usage)
	}
	if n := claudeCalls(t, app); n != 0 {
		t.Fatalf("fake claude saw %d calls while the fake provider was active", n)
	}

	// 5. Back on claude, stored conversations still resume on the fake; new work runs on claude.
	setProvider(t, app, "claude")
	app.Scenario(fakeagent.Scenario{Prompts: []fakeagent.PromptRule{
		{System: extractSystem, Emit: claudeAnswer(t, extractOut())},
		{System: mergeSystem, Emit: claudeAnswer(t, mergeOut())},
	}})
	launch, err := app.W.AdeTask.TakeOver(ctx, adewire.TakeOverArgs{SessionID: headless.ID})
	if err != nil {
		t.Fatal(err)
	}
	if want := fakeagent.Persona + " --resume " + headless.AgentSessionID; launch.Command != want {
		t.Fatalf("take over command = %q, want %q", launch.Command, want)
	}
	resume, err := app.W.ScriptRuns.ResumeCommand(scriptruns.ResumeCommandArgs{RunID: smart.ID})
	if err != nil || resume.Command != fakeagent.Persona+" --resume "+smart.SessionID || resume.Token != "" {
		t.Fatalf("ResumeCommand = %+v, %v, want the fake's", resume, err)
	}
	// A resumed smart run keeps its provider: the grant config is rendered by the fake, not the active claude.
	renders := fake.MCPConfigs()
	resumed, err := app.W.ScriptRuns.Resume(scriptruns.IDArgs{ID: smart.ID})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		r, _ := app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: resumed.RunID})
		return r.State == "done"
	})
	if h := fake.Headless(); h[len(h)-1].Resume != smart.SessionID || fake.MCPConfigs() != renders+1 {
		t.Fatalf("headless = %+v, renders = %d (was %d), want the resume on the fake, rendered by it", h, fake.MCPConfigs(), renders)
	}
	structured := len(fake.Structured())
	distill(t, app, "d2")
	if n := claudeCalls(t, app); n == 0 {
		t.Fatal("a distill on claude made no fake claude call")
	}
	if len(fake.Structured()) != structured {
		t.Fatal("the fake saw a structured call while claude was active")
	}

	// 6. An unknown id falls back to claude; a stored provider that is gone is reported.
	before := claudeCalls(t, app)
	setProvider(t, app, "nope")
	distill(t, app, "d3")
	if claudeCalls(t, app) <= before || len(fake.Structured()) != structured {
		t.Fatal("an unknown provider id did not fall back to claude")
	}
	app.Unregister()
	app.Restart()
	info, err = app.W.AgentSvc.Info()
	if err != nil || info.ID != "claude" {
		t.Fatalf("Info after the provider is gone = %+v, %v, want claude", info, err)
	}
	_, err = app.W.ScriptRuns.ResumeCommand(scriptruns.ResumeCommandArgs{RunID: smart.ID})
	wantGone(t, err)
	_, err = app.W.AdeTask.TakeOver(ctx, adewire.TakeOverArgs{SessionID: headless.ID})
	wantGone(t, err)
}

// cloneWithMain returns a clone at dir of a fresh bare remote holding one commit on main.
func cloneWithMain(app *flowharness.App, dir string) (*flowharness.Repo, *flowharness.Bare) {
	name := filepath.Base(dir)
	bare := app.NewBare(name + "-remote")
	seed := app.NewRepo(name + "-seed")
	seed.Commit("base", map[string]string{"base.txt": "base\n"})
	bare.PushFrom(seed)
	return bare.Clone(dir), bare
}

func branchOf(t *testing.T, app *flowharness.App, taskID, repoID string) adewire.Branch {
	t.Helper()
	b, err := app.W.AdeTask.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, br := range b.Branches {
		if br.TaskID == taskID && br.CodeRepoID == repoID {
			return br
		}
	}
	t.Fatalf("board has no branch of task %s in repo %s", taskID, repoID)
	return adewire.Branch{}
}

// latestRun is the newest run of the task's step, from the board.
func latestRun(t *testing.T, app *flowharness.App, taskID, stepID string) adewire.Run {
	t.Helper()
	b, err := app.W.AdeTask.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var best adewire.Run
	for _, tk := range b.Tasks {
		if tk.ID != taskID {
			continue
		}
		for _, r := range tk.Runs {
			if r.StepID == stepID && r.Attempt >= best.Attempt {
				best = r
			}
		}
	}
	return best
}

func sessionByID(t *testing.T, app *flowharness.App, id string) adewire.Session {
	t.Helper()
	res, err := app.W.AdeTask.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range res.Sessions {
		if s.ID == id {
			return s
		}
	}
	return adewire.Session{}
}
