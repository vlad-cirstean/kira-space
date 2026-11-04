package adeflow_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/agentsandbox"
	"github.com/kirathecat/kira-studio/internal/agentsandbox/sandboxtest"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P287 agent sandbox. Contract ade-sandbox: tests/ui/ade-v2-sandbox.spec.ts reads these.

const sbScenario = "ade-sandbox"

var sbMask = flowharness.Mask("id", "taskId", "branchId", "sessionId", "startedAt", "finishedAt", "createdAt", "exitCode", "attempt", "at", "profile")

func newSandboxFx(t *testing.T, fake *sandboxtest.Fake, actions []fakeagent.Action, steps string) *runFixture {
	t.Helper()
	if fake.Dir == "" {
		fake.Dir = t.TempDir()
	}
	return newRunFixtureWith(t, actions, steps, flowharness.WithSandbox(fake))
}

func sandboxOf(t *testing.T, f *runFixture, runID string) adewire.RunSandboxDetail {
	t.Helper()
	d, err := f.app.W.AdeTask.RunSandbox(ctx, adewire.RunArgs{RunID: runID})
	if err != nil {
		t.Fatalf("RunSandbox: %v", err)
	}
	return d
}

func sbStep(id, sandbox string) string {
	return agentStep(id, sandbox)
}

func mustContain(t *testing.T, what, got string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Fatalf("%s lacks %q:\n%s", what, w, got)
		}
	}
}

func mustLack(t *testing.T, what, got string, nots ...string) {
	t.Helper()
	for _, n := range nots {
		if strings.Contains(got, n) {
			t.Fatalf("%s has %q:\n%s", what, n, got)
		}
	}
}

func TestSandboxStepDefault(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/tmp/kira-test-agent.sock")
	fake := &sandboxtest.Fake{}
	f := newSandboxFx(t, fake, []fakeagent.Action{{Name: "done"}}, sbStep("one", ""))
	f.start(t, "feat/api-sb")
	run := waitRun(t, f.app, f.taskID, "one", "done")
	if run.Sandbox == nil || run.Sandbox.Access != "write" || !run.Sandbox.Enforced || run.Sandbox.Denials != 0 {
		t.Fatalf("run badge = %+v, want an enforced write sandbox without denials", run.Sandbox)
	}
	br := branchOf(t, board(t, f.app), f.taskID, f.repoID)
	d := sandboxOf(t, f, run.ID)
	real, err := filepath.EvalSymlinks(br.Worktree)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Write) != 1 || d.Write[0] != real || !d.Network {
		t.Fatalf("detail = %+v, want the worktree %s, network on", d, real)
	}
	mustContain(t, "profile", d.Profile, "(version 1)", real)
	args := fakeFile(t, f.app, "api-1.args")
	mustContain(t, "argv", args, "--disallowedTools", "Bash(git branch:*)", "Bash(git worktree:*)", "--add-dir")
	env := fakeFile(t, f.app, "api-1.sandbox-env")
	mustContain(t, "env", env, "TMPDIR=")
	mustLack(t, "env", env, "SSH_AUTH_SOCK")
	mustContain(t, "prompt", fakeFile(t, f.app, "api-1.prompt"), "Sandbox: write only in "+real)
	if got := fake.Policies(); len(got) != 1 || got[0].Branch[real] != "feat/api-sb" {
		t.Fatalf("policies = %+v, want one that lets the worktree advance feat/api-sb", got)
	}
	f.app.Contract(t, sbScenario, "AdeTaskService.RunSandbox#policy", d, sbMask)
	f.app.Contract(t, sbScenario, "AdeTaskService.Run#policy", run, sbMask)
}

func TestSandboxReadStep(t *testing.T) {
	fake := &sandboxtest.Fake{}
	f := newSandboxFx(t, fake, []fakeagent.Action{{Name: "done"}}, sbStep("one", "    sandbox:\n      access: read\n"))
	f.start(t, "feat/api-sb")
	run := waitRun(t, f.app, f.taskID, "one", "done")
	d := sandboxOf(t, f, run.ID)
	if d.Access != "read" || len(d.Write) != 0 || len(d.Read) == 0 {
		t.Fatalf("detail = %+v, want read access that writes nothing", d)
	}
	mustContain(t, "argv", fakeFile(t, f.app, "api-1.args"), "Edit", "Write", "NotebookEdit")
	mustContain(t, "env", fakeFile(t, f.app, "api-1.sandbox-env"), "GIT_OPTIONAL_LOCKS=0")
	mustContain(t, "prompt", fakeFile(t, f.app, "api-1.prompt"), "read only")
}

func TestSandboxOff(t *testing.T) {
	fake := &sandboxtest.Fake{}
	f := newSandboxFx(t, fake, []fakeagent.Action{{Name: "done"}}, sbStep("one", "    sandbox:\n      access: off\n"))
	f.start(t, "feat/api-sb")
	run := waitRun(t, f.app, f.taskID, "one", "done")
	if run.Sandbox == nil || run.Sandbox.Access != "off" || run.Sandbox.Enforced {
		t.Fatalf("run badge = %+v, want access off, not enforced", run.Sandbox)
	}
	if n := len(fake.Policies()); n != 0 {
		t.Fatalf("Prepare ran %d times for an unconfined run", n)
	}
	mustLack(t, "argv", fakeFile(t, f.app, "api-1.args"), "--add-dir", "Bash(git branch:*)")
	mustLack(t, "prompt", fakeFile(t, f.app, "api-1.prompt"), "Sandbox")
	f.app.Contract(t, sbScenario, "AdeTaskService.RunSandbox#off", sandboxOf(t, f, run.ID), sbMask)
	f.app.Contract(t, sbScenario, "AdeTaskService.Run#off", run, sbMask)
}

func TestSandboxDenial(t *testing.T) {
	gate := filepath.Join(t.TempDir(), "gate")
	fake := &sandboxtest.Fake{}
	f := newSandboxFx(t, fake, []fakeagent.Action{{WaitFile: gate, Name: "failed"}}, sbStep("one", ""))
	f.start(t, "feat/api-sb")
	var runID string
	testx.WaitUntil(t, waitFor, func() bool { runID = fake.WatchedRun(); return runID != "" })
	d := agentsandbox.Denial{At: time.Unix(1_700_000_000, 0), Op: "file-write-create", Target: "/etc/kira-denied", Process: "git"}
	if !fake.Inject(runID, d) || !fake.Inject(runID, d) {
		t.Fatal("the run is not watched")
	}
	testx.WaitUntil(t, waitFor, func() bool {
		r, ok := latestRun(t, f.app, f.taskID, "one")
		return ok && r.Sandbox != nil && r.Sandbox.Denials == 2
	})
	live, _ := latestRun(t, f.app, f.taskID, "one")
	if live.Sandbox.Last != "file-write-create /etc/kira-denied" {
		t.Fatalf("badge = %+v, want the last denial", live.Sandbox)
	}
	if err := os.WriteFile(gate, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	run := waitRun(t, f.app, f.taskID, "one", "failed")
	if want := "Sandbox blocked file-write-create /etc/kira-denied."; !strings.HasPrefix(run.Note, want) {
		t.Fatalf("note = %q, want it to start with %q", run.Note, want)
	}
	detail := sandboxOf(t, f, run.ID)
	if detail.DenialCount != 2 || len(detail.Denials) != 1 || detail.Denials[0].Target != "/etc/kira-denied" {
		t.Fatalf("detail = %+v, want two denials, one kept", detail)
	}
	log := logText(t, f.app, "run", run.ID)
	if n := strings.Count(log, "sandbox denied file-write-create /etc/kira-denied (git)"); n != 1 {
		t.Fatalf("log has the denial line %d times, want 1:\n%s", n, log)
	}
	f.app.Contract(t, sbScenario, "AdeTaskService.RunSandbox#denied", detail, sbMask)
	f.app.Contract(t, sbScenario, "AdeTaskService.Run#denied", run, sbMask)
}

func TestSandboxNotEnforced(t *testing.T) {
	fake := &sandboxtest.Fake{NotEnforced: true}
	f := newSandboxFx(t, fake, []fakeagent.Action{{Name: "done"}}, sbStep("one", ""))
	f.start(t, "feat/api-sb")
	run := waitRun(t, f.app, f.taskID, "one", "done")
	if run.Sandbox == nil || run.Sandbox.Access != "write" || run.Sandbox.Enforced {
		t.Fatalf("run badge = %+v, want a declared write sandbox, not enforced", run.Sandbox)
	}
	d := sandboxOf(t, f, run.ID)
	mustContain(t, "reason", d.Reason, "not enforced")
	if _, err := os.Stat(filepath.Join(fake.Dir, "sandbox-exec-shim")); err == nil {
		t.Fatal("claude was wrapped although nothing is enforced")
	}
	mustContain(t, "argv", fakeFile(t, f.app, "api-1.args"), "Bash(git branch:*)")
	mustContain(t, "prompt", fakeFile(t, f.app, "api-1.prompt"), "(declared, not enforced)")
	mustContain(t, "log", logText(t, f.app, "run", run.ID), "not enforced")
	f.app.Contract(t, sbScenario, "AdeTaskService.RunSandbox#notEnforced", d, sbMask)
	f.app.Contract(t, sbScenario, "AdeTaskService.Run#notEnforced", run, sbMask)
}

func TestSandboxPrepareFails(t *testing.T) {
	fake := &sandboxtest.Fake{PrepareErr: errors.New("profile did not load")}
	f := newSandboxFx(t, fake, []fakeagent.Action{{Name: "done"}}, sbStep("one", ""))
	_ = f.start(t, "feat/api-sb")
	run := waitRun(t, f.app, f.taskID, "one", "failed")
	if !strings.Contains(run.Note, "could not start: profile did not load") {
		t.Fatalf("note = %q, want the launch failure", run.Note)
	}
	if _, err := os.Stat(filepath.Join(f.app.FakeDir, "api-1.args")); err == nil {
		t.Fatal("claude started although the sandbox failed to prepare")
	}
}

func TestSandboxTools(t *testing.T) {
	call := func(tool string, args map[string]any) fakeagent.MCPCall {
		return fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: tool, Args: args}
	}
	tolerated := func(c fakeagent.MCPCall) fakeagent.MCPCall { c.Tolerate = true; return c }
	result := func(f *runFixture, tool string) string {
		t.Helper()
		matches, _ := filepath.Glob(filepath.Join(f.app.FakeDir, "mcp-"+tool+"-*.json"))
		if len(matches) == 0 {
			t.Fatalf("the agent never got a %s result", tool)
		}
		raw, err := os.ReadFile(matches[0])
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}

	t.Run("push and fetch", func(t *testing.T) {
		f := newSandboxFx(t, &sandboxtest.Fake{}, []fakeagent.Action{{
			Sh:   "git commit -q --allow-empty -m sandboxed",
			MCP:  []fakeagent.MCPCall{tolerated(call("fetch_remote", map[string]any{"remote": "nope"})), call("fetch_remote", nil), call("push_branch", nil)},
			Name: "done",
		}}, sbStep("one", ""))
		f.start(t, "feat/api-sb")
		waitRun(t, f.app, f.taskID, "one", "done")
		mustContain(t, "bad remote", result(f, "fetch_remote"), "remote must be one of: origin")
		br := branchOf(t, board(t, f.app), f.taskID, f.repoID)
		if got, want := f.bare.Git("rev-parse", "refs/heads/feat/api-sb"), gitOut(t, br.Worktree, "rev-parse", "HEAD"); got != want {
			t.Fatalf("remote branch = %s, want the pushed commit %s", got, want)
		}
	})

	t.Run("read step cannot push", func(t *testing.T) {
		f := newSandboxFx(t, &sandboxtest.Fake{}, []fakeagent.Action{{MCP: []fakeagent.MCPCall{tolerated(call("push_branch", nil))}, Name: "done"}},
			sbStep("one", "    sandbox:\n      access: read\n"))
		f.start(t, "feat/api-sb")
		waitRun(t, f.app, f.taskID, "one", "done")
		mustContain(t, "push result", result(f, "push_branch"), "push is not allowed in a read-only step")
		if out := f.bare.Git("branch", "--list", "feat/api-sb"); out != "" {
			t.Fatalf("remote has %q after a refused push", out)
		}
	})
}

func TestSandboxReviewRound(t *testing.T) {
	gate := filepath.Join(t.TempDir(), "gate")
	steps := agentStep("implement", "") +
		"  - id: review\n    name: review\n    kind: review\n    runs_on: each repo\n    timeout: 1m\n" +
		"    sandbox:\n      network: false\n" +
		"    review:\n      send_to: implementer\n" +
		"    results:\n      - id: approved\n        ok: true\n"
	f := &reviewFx{runFixture: newRunFixtureWith(t, []fakeagent.Action{
		{Name: "done", Sh: rvImplement},
		{Sh: "echo fixed >> r.txt && git commit -q -am fix", MCP: []fakeagent.MCPCall{rvReply(1, "renamed")}, WaitFile: gate, Name: "done"},
	}, steps, flowharness.WithSandbox(&sandboxtest.Fake{Dir: t.TempDir()}))}
	f.toReview(t, "feat/api-sb")
	f.window(t)
	f.comment(t, 1, "rename this", "")
	if err := f.send("", ""); err != nil {
		t.Fatalf("SendReview: %v", err)
	}
	f.waitPhase(t, "working")
	if err := os.WriteFile(gate, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	f.waitPhase(t, "waiting")
	mustLack(t, "implement argv", fakeFile(t, f.app, "api-1.args"), "WebFetch")
	round := fakeFile(t, f.app, "api-2.args")
	mustContain(t, "round argv", round, "WebFetch", "WebSearch")
	mustContain(t, "round prompt", fakeFile(t, f.app, "api-2.prompt"), "Network: Claude API only.")
}

func TestSmartRunSandboxed(t *testing.T) {
	fake := &sandboxtest.Fake{Dir: t.TempDir()}
	f := newAutoFixture(t, "", false, flowharness.WithSandbox(fake))
	rec := f.script(t, smartFields("audit", "Audit {branch}"))
	started := f.start(t, scriptruns.RunArgs{ScriptID: rec.ID, TaskID: f.task, BranchID: f.apiBr.ID})
	f.run(t, started.RunID, "running")
	release(t, f.gateOne)
	f.run(t, started.RunID, "done")
	mustContain(t, "argv", fakeFile(t, f.app, "api-2.args"), "--disallowedTools", "--add-dir")
	mustContain(t, "env", fakeFile(t, f.app, "api-2.sandbox-env"), "TMPDIR=")
	if got := fake.Policies(); len(got) != 3 {
		t.Fatalf("policies = %d, want two step runs and the smart run", len(got))
	}
}

// P510 script steps take a sandbox block; without one they run unconfined. Contract ade-sandbox.
func TestSandboxScriptStep(t *testing.T) {
	const cmd = "'printenv TMPDIR > \"$PWD/.script-tmpdir\"'"
	t.Run("read access with the network off", func(t *testing.T) {
		fake := &sandboxtest.Fake{}
		steps := scriptStep("check", cmd) + "    sandbox:\n      access: read\n      network: false\n"
		f := newSandboxFx(t, fake, nil, steps)
		f.start(t, "feat/api-script")
		run := waitRun(t, f.app, f.taskID, "check", "done")
		br := branchOf(t, board(t, f.app), f.taskID, f.repoID)
		real, err := filepath.EvalSymlinks(br.Worktree)
		if err != nil {
			t.Fatal(err)
		}
		pols := fake.Policies()
		if len(pols) != 1 || len(pols[0].Write) != 0 || pols[0].Network || !slices.Contains(pols[0].Read, real) || pols[0].Branch[real] != "feat/api-script" {
			t.Fatalf("policies = %+v, want one read-only policy on %s, network off", pols, real)
		}
		raw, err := os.ReadFile(filepath.Join(br.Worktree, ".script-tmpdir"))
		if err != nil || !strings.Contains(string(raw), pols[0].Scratch) {
			t.Fatalf("script TMPDIR = %q (%v), want it under the scratch dir %s", raw, err, pols[0].Scratch)
		}
		d := sandboxOf(t, f, run.ID)
		if d.Access != "read" || d.Network || len(d.Write) != 0 {
			t.Fatalf("detail = %+v, want read access, network off", d)
		}
		if run.Sandbox == nil || run.Sandbox.Access != "read" || !run.Sandbox.Enforced {
			t.Fatalf("run badge = %+v, want an enforced read sandbox", run.Sandbox)
		}
		f.app.Contract(t, sbScenario, "AdeTaskService.RunSandbox#script", d, sbMask)
		f.app.Contract(t, sbScenario, "AdeTaskService.Run#script", run, sbMask)
	})

	t.Run("no block runs unconfined", func(t *testing.T) {
		fake := &sandboxtest.Fake{}
		f := newSandboxFx(t, fake, nil, scriptStep("check", "'echo ok'"))
		f.start(t, "feat/api-script")
		run := waitRun(t, f.app, f.taskID, "check", "done")
		if n := len(fake.Policies()); n != 0 {
			t.Fatalf("Prepare ran %d times for an unconfined script", n)
		}
		if run.Sandbox != nil {
			t.Fatalf("run badge = %+v, want none", run.Sandbox)
		}
	})
}

// P511 manual session steps take a sandbox. Contract ade-sandbox.

var sbSessionMask = flowharness.Mask("id", "taskId", "branchId", "agentSessionId", "terminalId", "startedAt", "lastActiveAt", "at", "profile")

func sessionStep(id, extra string) string {
	return "  - id: " + id + "\n    name: " + id + "\n    kind: manual\n    session: true\n" + extra
}

// manualSbFx is a task whose steps include a manual session step, with every interactive call held open.
type manualSbFx struct {
	*runFixture
	fake *sandboxtest.Fake
}

func newManualSbFx(t *testing.T, fake *sandboxtest.Fake, steps string) *manualSbFx {
	t.Helper()
	gate := filepath.Join(t.TempDir(), "gate")
	t.Cleanup(func() { _ = os.WriteFile(gate, nil, 0o644) })
	hold := fakeagent.Action{WaitFile: gate}
	f := newSandboxFx(t, fake, []fakeagent.Action{{Name: "done"}, hold, hold, hold, hold}, steps)
	f.app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})
	return &manualSbFx{runFixture: f, fake: fake}
}

// afterRun starts the task's branch and waits for the step before the session step.
func (f *manualSbFx) afterRun(t *testing.T) {
	t.Helper()
	f.start(t, "feat/api-sb")
	waitRun(t, f.app, f.taskID, "one", "done")
}

func (f *manualSbFx) policyOf(t *testing.T, sessionID string) agentsandbox.Policy {
	t.Helper()
	var got []agentsandbox.Policy
	for _, p := range f.fake.Policies() {
		if p.RunID == sessionID {
			got = append(got, p)
		}
	}
	if len(got) != 1 {
		t.Fatalf("policies for session %s = %+v, want exactly one", sessionID, got)
	}
	return got[0]
}

func (f *manualSbFx) launch(t *testing.T) adewire.Launch {
	t.Helper()
	l, err := f.app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: f.taskID})
	if err != nil {
		t.Fatalf("LaunchStep: %v", err)
	}
	return l
}

func (f *manualSbFx) sessionDetail(t *testing.T, id string) (adewire.RunSandboxDetail, error) {
	t.Helper()
	return f.app.W.AdeTask.SessionSandbox(ctx, adewire.SessionSandboxArgs{SessionID: id})
}

func sessionsOnly(s adewire.Session) adewire.SessionsResult {
	return adewire.SessionsResult{Sessions: []adewire.Session{s}}
}

func TestSandboxManualSession(t *testing.T) {
	const shim = "sandbox-exec-shim"

	t.Run("write, network off", func(t *testing.T) {
		fake := &sandboxtest.Fake{}
		f := newManualSbFx(t, fake, sbStep("one", "")+sessionStep("work", "    sandbox:\n      access: write\n      network: false\n"))
		f.afterRun(t)
		launch := f.launch(t)
		mustContain(t, "launch command", launch.Command, filepath.Join(fake.Dir, shim), " -f ", "'--disallowedTools'")
		br := branchOf(t, board(t, f.app), f.taskID, f.repoID)
		real, err := filepath.EvalSymlinks(br.Worktree)
		if err != nil {
			t.Fatal(err)
		}
		pol := f.policyOf(t, launch.SessionID)
		if len(pol.Write) != 1 || pol.Write[0] != real || pol.Network || pol.Branch[real] != "feat/api-sb" {
			t.Fatalf("policy = %+v, want the worktree %s writable on feat/api-sb, network off", pol, real)
		}
		if !slices.ContainsFunc(pol.ReadFiles, func(p string) bool { return strings.HasSuffix(p, ".mcp.json") }) {
			t.Fatalf("policy read files = %v, want the MCP config", pol.ReadFiles)
		}
		openLaunch(t, f.app, launch)
		waitSession(t, f.app, launch.SessionID, "running")
		testx.WaitUntil(t, waitFor, func() bool { return fake.WatchedRun() == launch.SessionID })
		row := sessionByID(t, f.app, launch.SessionID)
		if row.Sandbox == nil || row.Sandbox.Access != "write" || !row.Sandbox.Enforced {
			t.Fatalf("session badge = %+v, want an enforced write sandbox", row.Sandbox)
		}
		d := agentsandbox.Denial{At: time.Unix(1_700_000_000, 0), Op: "file-write-create", Target: "/etc/kira-denied", Process: "git"}
		if !fake.Inject(launch.SessionID, d) {
			t.Fatal("the session is not watched")
		}
		var detail adewire.RunSandboxDetail
		testx.WaitUntil(t, waitFor, func() bool {
			var err error
			detail, err = f.sessionDetail(t, launch.SessionID)
			return err == nil && detail.DenialCount == 1
		})
		if detail.Access != "write" || detail.Network || len(detail.Write) != 1 || detail.Write[0] != real {
			t.Fatalf("detail = %+v, want write access on %s, network off", detail, real)
		}
		row = sessionByID(t, f.app, launch.SessionID)
		f.app.Contract(t, sbScenario, "AdeTaskService.Sessions#manual-sandbox", sessionsOnly(row), sbSessionMask)
		f.app.Contract(t, sbScenario, "AdeTaskService.SessionSandbox#manual", detail, sbSessionMask)

		scratch := pol.Scratch
		if err := f.app.W.AdeTask.CloseSession(ctx, adewire.CloseSessionArgs{SessionID: launch.SessionID}); err != nil {
			t.Fatal(err)
		}
		waitSession(t, f.app, launch.SessionID, "stopped")
		testx.WaitUntil(t, waitFor, func() bool { _, err := os.Stat(scratch); return os.IsNotExist(err) })

		// Take over re-prepares the sandbox under the same record id.
		again, err := f.app.W.AdeTask.TakeOver(ctx, adewire.TakeOverArgs{SessionID: launch.SessionID})
		if err != nil {
			t.Fatalf("TakeOver: %v", err)
		}
		mustContain(t, "take over command", again.Command, filepath.Join(fake.Dir, shim), "--resume")
		n := 0
		for _, p := range fake.Policies() {
			if p.RunID == launch.SessionID {
				n++
			}
		}
		if n != 2 {
			t.Fatalf("%d policies for the record, want 2 (launch and take over)", n)
		}
	})

	t.Run("explicit off", func(t *testing.T) {
		fake := &sandboxtest.Fake{}
		f := newManualSbFx(t, fake, sessionStep("work", "    sandbox:\n      access: off\n"))
		launch := f.launch(t)
		mustLack(t, "launch command", launch.Command, filepath.Join(fake.Dir, shim), " -f ", "--disallowed", "--add-dir")
		openLaunch(t, f.app, launch)
		waitSession(t, f.app, launch.SessionID, "running")
		if n := len(fake.Policies()); n != 0 {
			t.Fatalf("Prepare ran %d times for an unconfined session", n)
		}
		if id := fake.WatchedRun(); id != "" {
			t.Fatalf("session %s is watched although the sandbox is off", id)
		}
		row := sessionByID(t, f.app, launch.SessionID)
		if row.Sandbox != nil {
			t.Fatalf("session badge = %+v, want none", row.Sandbox)
		}
		if _, err := f.sessionDetail(t, launch.SessionID); err == nil {
			t.Fatal("SessionSandbox returned a sandbox for an unconfined session")
		}
		f.app.Contract(t, sbScenario, "AdeTaskService.Sessions#manual-off", sessionsOnly(row), sbSessionMask)
	})

	t.Run("no block is off", func(t *testing.T) {
		fake := &sandboxtest.Fake{}
		f := newManualSbFx(t, fake, sessionStep("work", ""))
		entry, _ := workflowEntry(t, f.app, "flow.yaml")
		if got := entry.Workflow.Steps[0].Sandbox.Access; got != adewire.SandboxOff {
			t.Fatalf("manual step sandbox access = %q, want off", got)
		}
		launch := f.launch(t)
		mustLack(t, "launch command", launch.Command, filepath.Join(fake.Dir, shim), " -f ", "--disallowed")
		openLaunch(t, f.app, launch)
		waitSession(t, f.app, launch.SessionID, "running")
		if n := len(fake.Policies()); n != 0 {
			t.Fatalf("Prepare ran %d times for an unconfined session", n)
		}
		if row := sessionByID(t, f.app, launch.SessionID); row.Sandbox != nil {
			t.Fatalf("session badge = %+v, want none", row.Sandbox)
		}
	})

	t.Run("outside repo, read", func(t *testing.T) {
		fake := &sandboxtest.Fake{}
		f := newManualSbFx(t, fake, sessionStep("work", "    outside_repo: true\n    sandbox:\n      access: read\n"))
		launch := f.launch(t)
		real, err := filepath.EvalSymlinks(launch.Cwd)
		if err != nil {
			t.Fatal(err)
		}
		pol := f.policyOf(t, launch.SessionID)
		if len(pol.Write) != 0 || !slices.Contains(pol.Read, real) || len(pol.Branch) != 0 {
			t.Fatalf("policy = %+v, want the task folder %s read-only and no branch", pol, real)
		}
	})

	t.Run("not enforced", func(t *testing.T) {
		fake := &sandboxtest.Fake{NotEnforced: true}
		f := newManualSbFx(t, fake, sessionStep("work", "    outside_repo: true\n"+"    sandbox:\n      access: write\n"))
		launch := f.launch(t)
		mustLack(t, "launch command", launch.Command, filepath.Join(fake.Dir, shim), " -f ")
		mustContain(t, "launch command", launch.Command, "'--disallowedTools'")
		openLaunch(t, f.app, launch)
		waitSession(t, f.app, launch.SessionID, "running")
		var detail adewire.RunSandboxDetail
		testx.WaitUntil(t, waitFor, func() bool {
			var err error
			detail, err = f.sessionDetail(t, launch.SessionID)
			return err == nil
		})
		if detail.Enforced || detail.Access != "write" {
			t.Fatalf("detail = %+v, want a declared write sandbox, not enforced", detail)
		}
		mustContain(t, "reason", detail.Reason, "not enforced")
		if row := sessionByID(t, f.app, launch.SessionID); row.Sandbox == nil || row.Sandbox.Enforced {
			t.Fatalf("session badge = %+v, want not enforced", row.Sandbox)
		}
	})
}
