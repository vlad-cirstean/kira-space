package harnessflow_test

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/harness/opencode"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

const ocSession = "ses_fake01"

func ocWorkflow(id, agent string) string {
	line := ""
	if agent != "" {
		line = "    agent: " + agent + "\n"
	}
	return "id: " + id + "\nname: " + id + "\nsteps:\n" +
		"  - id: work\n    name: Work\n    kind: manual\n    status: In progress\n    session: true\n" + line
}

func ocAgentFlow(id, extra string) string {
	return "id: " + id + "\nname: " + id + "\nsteps:\n" +
		"  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 1m\n    agent: opencode\n" + extra +
		"    prompt: work on {branch} in {repo}\n"
}

func fakeRead(t *testing.T, app *flowharness.App, name string) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(app.FakeDir, name))
	return string(b)
}

func waitFake(t *testing.T, app *flowharness.App, name string) string {
	t.Helper()
	var s string
	testx.WaitUntil(t, waitFor, func() bool { s = fakeRead(t, app, name); return s != "" })
	return s
}

// envLines returns the Go-quoted variables the fake recorded, by name.
func envLines(t *testing.T, raw string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, l := range strings.Split(raw, "\n") {
		if l == "" {
			continue
		}
		kv, err := strconv.Unquote(l)
		if err != nil {
			t.Fatalf("env record line %q: %v", l, err)
		}
		k, v, _ := strings.Cut(kv, "=")
		out[k] = v
	}
	return out
}

func openTUI(t *testing.T, app *flowharness.App, l adewire.Launch) {
	t.Helper()
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: l.TerminalID, Cwd: l.Cwd, Cols: 100, Rows: 30, WindowKey: window,
		Command: l.Command, LaunchKind: terminal.LaunchKindAgent,
	}); err != nil {
		t.Fatal(err)
	}
}

func hookPost(t *testing.T, sock, token, terminalID, body string) int {
	t.Helper()
	cl := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", sock)
	}}}
	req, err := http.NewRequest(http.MethodPost, "http://kira/hook", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Kira-Terminal", terminalID)
	resp, err := cl.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

// TestOpenCodeNotFound: with no opencode CLI installed the provider is still listed, as not found.
func TestOpenCodeNotFound(t *testing.T) {
	app := flowharness.New(t)
	list, err := app.W.AgentSvc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[1].ID != "opencode" || list[1].Available || !list[0].Available {
		t.Fatalf("List = %+v, want claude available and opencode not", list)
	}
	app.Contract(t, "agent", "AgentService.List#no-opencode", list)
}

// TestOpenCodeProvider: OpenCode registered next to claude, picked per launch, per step and as the
// default, runs interactive and headless work, reports session events through its plugin, and
// refuses what it cannot do. Frontend half: agent-provider.spec.ts.
func TestOpenCodeProvider(t *testing.T) {
	app := flowharness.New(t, flowharness.WithOpenCode(), flowharness.WithTrackerGrace(time.Second))
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	repoRec, err := app.W.CodeWorkspace.ImportRepo(ctx, bridge.CodeWorkspaceImportArgs{Path: repo.Dir})
	if err != nil {
		t.Fatal(err)
	}
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: window})

	// (a) Both providers are listed, opencode found.
	list, err := app.W.AgentSvc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != "claude" || list[1].ID != "opencode" || !list[0].Available || !list[1].Available {
		t.Fatalf("List = %+v, want claude and opencode, both available", list)
	}
	app.Contract(t, "agent", "AgentService.List", list)

	// (b) "+" picks the provider per launch.
	tk := task(t, app, repoRec.ID, "plus", ocWorkflow("plus", ""))
	row := branchOf(t, app, tk.ID, repoRec.ID)
	br, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: row.ID, Name: "feat/api-oc"})
	if err != nil {
		t.Fatal(err)
	}
	launch, err := app.W.AdeTask.StartAdHoc(ctx, adewire.StartAdHocArgs{BranchID: br.ID, Provider: "opencode"})
	if err != nil {
		t.Fatalf("StartAdHoc opencode: %v", err)
	}
	if launch.Command != "opencode" {
		t.Fatalf("command = %q, want opencode", launch.Command)
	}
	openTUI(t, app, launch)
	testx.WaitUntil(t, waitFor, func() bool { return sessionByID(t, app, launch.SessionID).State == "running" })
	if s := sessionByID(t, app, launch.SessionID); s.Provider != "opencode" || s.AgentSessionID != "" {
		t.Fatalf("session = %+v, want an opencode row with no id yet", s)
	}
	env := envLines(t, waitFake(t, app, "opencode-1.env"))
	cfg := waitFake(t, app, "opencode-1.config")
	for _, want := range []string{`"mcp"`, `"type":"remote"`, `"Authorization":"Bearer `} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("OPENCODE_CONFIG file lacks %s:\n%s", want, cfg)
		}
	}
	if env["OPENCODE_CONFIG"] == "" || !strings.Contains(env["OPENCODE_CONFIG_CONTENT"], "plugin.js") {
		t.Fatalf("launch env = %v, want OPENCODE_CONFIG and the plugin in OPENCODE_CONFIG_CONTENT", env)
	}

	// (e) The plugin's events reach the listener: a bad token is refused, session.created fills the id.
	sock, token := env["KIRA_AGENT_HOOK_SOCKET"], env["KIRA_AGENT_HOOK_TOKEN"]
	body := `{"type":"session.created","sessionID":"` + ocSession + `"}`
	if code := hookPost(t, sock, "wrong", launch.TerminalID, body); code != http.StatusUnauthorized {
		t.Fatalf("hook with a bad token = %d, want 401", code)
	}
	if code := hookPost(t, sock, token, launch.TerminalID, body); code != http.StatusOK {
		t.Fatalf("hook = %d, want 200", code)
	}
	testx.WaitUntil(t, waitFor, func() bool { return sessionByID(t, app, launch.SessionID).AgentSessionID == ocSession })

	// Take over keeps the stored provider: claude is the default now, the resume is opencode's.
	if err := app.W.Terminal.Close(terminal.CloseArgs{TerminalID: launch.TerminalID}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool { return sessionByID(t, app, launch.SessionID).State == "stopped" })
	back, err := app.W.AdeTask.TakeOver(ctx, adewire.TakeOverArgs{SessionID: launch.SessionID})
	if err != nil || back.Command != "opencode -s '"+ocSession+"'" {
		t.Fatalf("TakeOver = %+v, %v, want opencode -s %s", back, err, ocSession)
	}

	// The same call with claude runs claude.
	tk2 := task(t, app, repoRec.ID, "plus2", ocWorkflow("plus2", ""))
	br2, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: branchOf(t, app, tk2.ID, repoRec.ID).ID, Name: "feat/api-claude"})
	if err != nil {
		t.Fatal(err)
	}
	cl, err := app.W.AdeTask.StartAdHoc(ctx, adewire.StartAdHocArgs{BranchID: br2.ID, Provider: "claude"})
	if err != nil || !strings.HasPrefix(cl.Command, "claude") {
		t.Fatalf("StartAdHoc claude = %+v, %v, want a claude command", cl, err)
	}
	if _, err := app.W.AdeTask.StartAdHoc(ctx, adewire.StartAdHocArgs{BranchID: br2.ID, Provider: "nope"}); err == nil {
		t.Fatal("StartAdHoc with an unknown provider succeeded")
	}

	// (c) As the default, opencode launches steps; a step's own agent wins.
	setProvider(t, app, "opencode")
	step, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task(t, app, repoRec.ID, "dflt", ocWorkflow("dflt", "")).ID})
	if err != nil || step.Command != "opencode" {
		t.Fatalf("LaunchStep on the default = %+v, %v, want opencode", step, err)
	}
	step, err = app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task(t, app, repoRec.ID, "pick", ocWorkflow("pick", "claude")).ID})
	if err != nil || !strings.HasPrefix(step.Command, "claude") {
		t.Fatalf("LaunchStep with agent: claude = %+v, %v, want claude", step, err)
	}
	setProvider(t, app, "claude")

	// (f) A smart script with a cost cap is blocked on opencode.
	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
		Name: "ask", Kind: scripts.KindSmart, Command: "Say hi", Color: "blue", Smart: &scripts.Smart{Model: "default", MaxBudgetUSD: 1},
	}})
	if err != nil {
		t.Fatal(err)
	}
	setProvider(t, app, "opencode")
	pv, err := app.W.ScriptRuns.Preview(scriptruns.RunArgs{ScriptID: rec.ID})
	if want := i18n.T("backend.core.agent.noCostCap.body", map[string]string{"agent": opencode.New().Label()}); err != nil || pv.Blocker != want {
		t.Fatalf("Preview blocker = %q, %v, want %q", pv.Blocker, err, want)
	}
	setProvider(t, app, "claude")

	// (g) A tool OpenCode has no name for fails the run, naming it.
	badTask := task(t, app, repoRec.ID, "bad", ocAgentFlow("bad", "    allowed_tools: [NotATool]\n"))
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: badTask.ID, BranchNames: map[string]string{branchOf(t, app, badTask.ID, repoRec.ID).ID: "feat/api-bad"}}); err != nil {
		t.Fatal(err)
	}
	var bad adewire.Run
	testx.WaitUntil(t, waitFor, func() bool {
		bad = latestRun(t, app, badTask.ID, "one")
		return bad.State == "failed" || bad.State == "stuck"
	})
	if bad.Outcome == nil || !strings.Contains(bad.Outcome.Summary+bad.Outcome.Reason+bad.Note, "NotATool") {
		t.Fatalf("run = %+v, want a failure naming NotATool", bad)
	}
}

// TestOpenCodeHeadless: an agent step with agent: opencode runs headless, learns its session id from
// the event stream, resumes it with -s after a restart, and ends a lost conversation as such.
func TestOpenCodeHeadless(t *testing.T) {
	hold := filepath.Join(t.TempDir(), "hold")
	t.Cleanup(func() { _ = os.WriteFile(hold, nil, 0o644) })
	app := flowharness.New(t, flowharness.WithOpenCode(), flowharness.WithTrackerGrace(time.Second))
	app.Scenario(fakeagent.Scenario{OpenCode: map[string][]fakeagent.Action{"*": {{WaitFile: hold}, {Name: "lost"}, {Name: "done"}}}})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	repoRec, err := app.W.CodeWorkspace.ImportRepo(ctx, bridge.CodeWorkspaceImportArgs{Path: repo.Dir})
	if err != nil {
		t.Fatal(err)
	}
	tk := task(t, app, repoRec.ID, "head", ocAgentFlow("head", ""))
	br := branchOf(t, app, tk.ID, repoRec.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: tk.ID, BranchNames: map[string]string{br.ID: "feat/api-head"}}); err != nil {
		t.Fatal(err)
	}
	var run adewire.Run
	testx.WaitUntil(t, waitFor, func() bool { run = latestRun(t, app, tk.ID, "one"); return run.State == "running" })
	args := waitFake(t, app, "opencode-1.args")
	if !strings.HasPrefix(args, "run\n--format\njson") || strings.Contains(args, "\n-s\n") {
		t.Fatalf("first call args = %q, want a new run", args)
	}
	// The id arrives on the first event, while the run still works.
	testx.WaitUntil(t, waitFor, func() bool { return sessionByID(t, app, run.SessionID).AgentSessionID == ocSession })
	if s := sessionByID(t, app, run.SessionID); s.Provider != "opencode" || s.Mode != "headless" {
		t.Fatalf("session = %+v, want a headless opencode row", s)
	}

	app.Restart()
	testx.WaitUntil(t, waitFor, func() bool { run = latestRun(t, app, tk.ID, "one"); return run.State == "stuck" })
	if err := app.W.AdeTask.ResumeRun(ctx, adewire.RunArgs{RunID: run.ID}); err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		run = latestRun(t, app, tk.ID, "one")
		return run.Attempt == 2 && (run.State == "failed" || run.State == "stuck")
	})
	if second := waitFake(t, app, "opencode-2.args"); !strings.Contains(second, "\n-s\n"+ocSession) {
		t.Fatalf("resume args = %q, want -s %s", second, ocSession)
	}
	if run.Resumable || run.Outcome == nil || !strings.HasPrefix(run.Outcome.Reason, "could not resume") {
		t.Fatalf("lost run = %+v, want a failed could-not-resume outcome", run)
	}
	if err := app.W.AdeTask.RetryRun(ctx, adewire.RunArgs{RunID: run.ID}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool { run = latestRun(t, app, tk.ID, "one"); return run.Attempt == 3 && run.State == "done" })
}
