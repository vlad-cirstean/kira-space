package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/ade"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/sessionrestore"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// holdScript is a shell script that blocks until flag exists; the agent runs it as one Bash call, so
// a restart cuts the turn mid tool call.
func holdScript(t *testing.T, flag string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hold.sh")
	body := "#!/bin/sh\nwhile [ ! -e " + flag + " ]; do sleep 1; done\necho released\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func holdPrompt(script string) string {
	return "Run exactly one Bash command: sh " + script + " . It is safe to run again: if an earlier call was cut off, run it again and inspect nothing first. Wait for it to finish, then call state with action step.finish and args status done."
}

// startHeld starts the ADE run and returns once the agent has issued its Bash call.
func (f *adeFixture) startHeld(t *testing.T, script string) (adewire.Task, adewire.Run) {
	t.Helper()
	step := "  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 5m\n" +
		"    allowed_tools: [\"Bash(sh:*)\"]\n" +
		"    prompt: " + strconv.Quote(holdPrompt(script)) + "\n"
	f.saveWorkflow(t, "flow", step+manualStepYAML)
	task := f.createTask(t, "flow")
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
	if _, err := f.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{branchID: "feat/real-resume"}}); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	var run adewire.Run
	testx.WaitUntil(t, testTimeout, func() bool {
		run = f.stepRun(t, task.ID)
		return run.ID != "" && strings.Contains(f.runLog(t, run.ID), "Bash")
	})
	return task, run
}

// claudeSession returns Claude's session id of an ADE session row.
func (f *adeFixture) claudeSession(t *testing.T, sessionID string) string {
	t.Helper()
	res, err := f.W.AdeTask.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range res.Sessions {
		if s.ID == sessionID {
			return s.AgentSessionID
		}
	}
	t.Fatalf("no session %s in %+v", sessionID, res.Sessions)
	return ""
}

func (f *adeFixture) stepRun(t *testing.T, taskID string) adewire.Run {
	t.Helper()
	b, err := f.W.AdeTask.Board(ctx)
	if err != nil {
		return adewire.Run{}
	}
	var best adewire.Run
	for _, tk := range b.Tasks {
		if tk.ID != taskID {
			continue
		}
		for _, r := range tk.Runs {
			if r.StepID == "one" && r.Attempt >= best.Attempt {
				best = r
			}
		}
	}
	return best
}

// TestAdeResumeRun cuts a real agent off mid Bash call with an app restart, resumes it, releases the
// call and expects the same Claude session to finish through state step.finish. The CLI's interrupted tool_result
// line is not in the run log, so the check is the shared Claude session id.
func TestAdeResumeRun(t *testing.T) {
	f := newAdeFixture(t, nil)
	flag := filepath.Join(t.TempDir(), "flag")
	task, run := f.startHeld(t, holdScript(t, flag))

	f.Restart()
	var stuck adewire.Run
	testx.WaitUntil(t, testTimeout, func() bool {
		stuck = f.stepRun(t, task.ID)
		return stuck.ID == run.ID && stuck.State == "stuck" && stuck.Resumable
	})
	if err := f.W.AdeTask.ResumeRun(ctx, adewire.RunArgs{RunID: stuck.ID}); err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}
	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var resumed adewire.Run
	testx.WaitUntil(t, testTimeout, func() bool {
		resumed = f.stepRun(t, task.ID)
		return resumed.Attempt == 2 && resumed.State != "running"
	})
	log := f.runLog(t, resumed.ID)
	if resumed.State != "done" || resumed.Outcome == nil || !resumed.Outcome.Reported {
		t.Fatalf("resumed run = %+v, want done through state step.finish; log:\n%s", resumed, tail(log, 1500))
	}
	if got, want := f.claudeSession(t, resumed.SessionID), f.claudeSession(t, run.SessionID); got == "" || got != want {
		t.Fatalf("resumed run on claude session %q, want the interrupted run's %q", got, want)
	}
	f.requireCostUnder(t)
}

// removeTranscript deletes Claude's saved conversation for a session id under the test HOME.
func removeTranscript(t *testing.T, home, sessionID string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", sessionID+".jsonl"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no transcript for session %s under %s (%v)", sessionID, home, err)
	}
	for _, m := range matches {
		if err := os.Remove(m); err != nil {
			t.Fatal(err)
		}
	}
}

// TestAdeResumeLost resumes after the transcript is gone: the CLI refuses without an API call, so the
// run fails with the could-not-resume reason and no cost.
func TestAdeResumeLost(t *testing.T) {
	f := newAdeFixture(t, nil)
	flag := filepath.Join(t.TempDir(), "flag")
	task, run := f.startHeld(t, holdScript(t, flag))

	f.Restart()
	var stuck adewire.Run
	testx.WaitUntil(t, testTimeout, func() bool {
		stuck = f.stepRun(t, task.ID)
		return stuck.ID == run.ID && stuck.State == "stuck" && stuck.Resumable
	})
	removeTranscript(t, f.Home, f.claudeSession(t, run.SessionID))
	if err := f.W.AdeTask.ResumeRun(ctx, adewire.RunArgs{RunID: stuck.ID}); err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}
	var lost adewire.Run
	testx.WaitUntil(t, testTimeout, func() bool {
		lost = f.stepRun(t, task.ID)
		return lost.Attempt == 2 && lost.State != "running"
	})
	o := lost.Outcome
	if lost.State == "done" || o == nil || o.Source != "start" || !strings.HasPrefix(o.Reason, "could not resume") {
		t.Fatalf("lost run = %+v, want a could-not-resume failure; log:\n%s", lost, tail(f.runLog(t, lost.ID), 1500))
	}
	if o.CostUSD != nil && *o.CostUSD > 0 {
		t.Fatalf("lost run cost %.4f USD, want none", *o.CostUSD)
	}
}

// TestSmartScriptResume does the same for a smart script run.
func TestSmartScriptResume(t *testing.T) {
	app := newRealApp(t)
	flag := filepath.Join(t.TempDir(), "flag")
	script := holdScript(t, flag)
	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
		Name: "real", Kind: scripts.KindSmart, Color: "blue",
		Command: "Run exactly one Bash command: sh " + script + " . It is safe to run again: if an earlier call was cut off, run it again and inspect nothing first. Wait for it to finish, then reply with the word ok.",
		Smart:   &scripts.Smart{Model: "haiku", MaxBudgetUSD: 0.2, Timeout: "5m", Tools: []string{"Bash"}, BashPatterns: []string{"sh:*"}},
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
	testx.WaitUntil(t, testTimeout, func() bool {
		page, err := app.W.ScriptRuns.ReadLog(scriptruns.ReadLogArgs{ID: started.RunID})
		if err != nil {
			return false
		}
		for _, c := range page.Chunks {
			if strings.Contains(c.Text, "Bash") {
				return true
			}
		}
		return false
	})

	app.Restart()
	var stuck scriptruns.Run
	testx.WaitUntil(t, testTimeout, func() bool {
		stuck, _ = app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: started.RunID})
		return stuck.Resumable
	})
	resumedStart, err := app.W.ScriptRuns.Resume(scriptruns.IDArgs{ID: started.RunID})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var resumed scriptruns.Run
	testx.WaitUntil(t, testTimeout, func() bool {
		resumed, _ = app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: resumedStart.RunID})
		return resumed.State != "running"
	})
	o := resumed.Outcome
	if resumed.State != "done" || o == nil || !o.Reported || resumed.ResumedFrom != started.RunID || resumed.SessionID != stuck.SessionID {
		t.Fatalf("resumed run = %+v (outcome %+v), want done from %s on session %s", resumed, o, started.RunID, stuck.SessionID)
	}
	app.requireCostUnder(t)
}

// restoreAgent returns the restore offer's agent row with id.
func (f *adeFixture) restoreAgent(t *testing.T, id string) ade.AgentItem {
	t.Helper()
	var got ade.AgentItem
	testx.WaitUntil(t, testTimeout, func() bool {
		st, err := f.W.SessionRestore.State(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range st.Agents {
			if a.ID == id {
				got = a
			}
		}
		return got.ID != ""
	})
	return got
}

// TestAdeRestoreRunResumeOnly cuts a headless run off with a restart and restores it with Resume
// only: the run's conversation reopens in a terminal and nothing is sent to it.
func TestAdeRestoreRunResumeOnly(t *testing.T) {
	f := newAdeFixture(t, nil)
	flag := filepath.Join(t.TempDir(), "flag")
	task, run := f.startHeld(t, holdScript(t, flag))
	claudeID := f.claudeSession(t, run.SessionID)

	f.Restart()
	testx.WaitUntil(t, testTimeout, func() bool {
		stuck := f.stepRun(t, task.ID)
		return stuck.ID == run.ID && stuck.State == "stuck" && stuck.Resumable
	})
	item := f.restoreAgent(t, "run:"+run.ID)
	if item.State != ade.AgentReady || item.Mode != ade.AgentModeHeadless || item.Continue != ade.AgentContinueOptional {
		t.Fatalf("offer = %+v, want a ready headless run with optional Continue", item)
	}
	res, err := f.W.SessionRestore.Restore(ctx, sessionrestore.Args{Agents: []ade.AgentChoice{{ID: item.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Agents) != 1 || res.Agents[0].Error != "" || res.Agents[0].Launch == nil {
		t.Fatalf("restore = %+v, want one terminal launch", res.Agents)
	}
	launch := res.Agents[0].Launch
	if !strings.Contains(launch.Command, claudeID) || !strings.Contains(launch.Command, "--resume") {
		t.Fatalf("command %q does not resume claude session %s", launch.Command, claudeID)
	}
	if strings.Contains(launch.Command, "Kira restarted") {
		t.Fatalf("command %q carries the continue line", launch.Command)
	}
	f.seedClaudeJSON(t, f.repo.Dir, launch.Cwd)
	mark := f.Events.Mark()
	f.openAgent(t, launch.TerminalID, launch.Cwd, launch.Command)
	t.Cleanup(func() { _ = f.W.Terminal.Close(terminal.CloseArgs{TerminalID: launch.TerminalID}) })
	time.Sleep(10 * time.Second)
	for _, e := range f.agentEvents(t, mark, launch.TerminalID) {
		if e.Event == "UserPromptSubmit" {
			t.Fatalf("a turn started with Resume only: %+v", f.agentEvents(t, mark, launch.TerminalID))
		}
	}
}

// TestAdeRestoreSessionContinue restores an idle TUI session after a restart with Continue: the
// terminal resumes the same Claude conversation and submits the continue line as a new turn.
func TestAdeRestoreSessionContinue(t *testing.T) {
	f := newAdeFixture(t, allowShell)
	f.saveWorkflow(t, "flow", manualStepYAML)
	task := f.createTask(t, "flow")
	launch, err := f.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	f.seedClaudeJSON(t, f.repo.Dir, launch.Cwd)
	mark := f.Events.Mark()
	f.openAgent(t, launch.TerminalID, launch.Cwd, launch.Command)
	f.waitAgentEvent(t, mark, launch.TerminalID, "SessionStart", 60*time.Second)
	// SessionStart fires before the input box accepts keys.
	time.Sleep(2 * time.Second)
	if err := f.W.AdeTask.Send(ctx, adewire.SendArgs{SessionID: launch.SessionID, Message: "Reply with the single word one."}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	f.waitAgentEvent(t, mark, launch.TerminalID, "Stop", 90*time.Second)
	claudeID := f.claudeSession(t, launch.SessionID)

	f.Restart()
	item := f.restoreAgent(t, "session:"+launch.SessionID)
	if item.State != ade.AgentReady || item.Mode != ade.AgentModeTUI {
		t.Fatalf("offer = %+v, want a ready TUI session", item)
	}
	res, err := f.W.SessionRestore.Restore(ctx, sessionrestore.Args{Agents: []ade.AgentChoice{{ID: item.ID, Continue: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Agents) != 1 || res.Agents[0].Error != "" || res.Agents[0].Launch == nil {
		t.Fatalf("restore = %+v, want one terminal launch", res.Agents)
	}
	next := res.Agents[0].Launch
	if !strings.Contains(next.Command, claudeID) {
		t.Fatalf("command %q does not resume claude session %s", next.Command, claudeID)
	}
	f.seedClaudeJSON(t, f.repo.Dir, next.Cwd)
	mark = f.Events.Mark()
	f.openAgent(t, next.TerminalID, next.Cwd, next.Command)
	t.Cleanup(func() { _ = f.W.Terminal.Close(terminal.CloseArgs{TerminalID: next.TerminalID}) })
	f.waitAgentEvent(t, mark, next.TerminalID, "UserPromptSubmit", 90*time.Second)
	f.waitAgentEvent(t, mark, next.TerminalID, "Stop", 90*time.Second)
	if got := f.claudeSession(t, next.SessionID); got != claudeID {
		t.Fatalf("restored session on claude session %q, want %q", got, claudeID)
	}
	f.requireCostUnder(t)
}

// recordedEnum returns the enum of property prop of the tool schema the CLI saved in the session's
// transcript when it first loaded that deferred tool (CLI 2.1.330: an attachment of type
// deferred_tools_record). --resume replays it and a later ToolSearch does not refresh it.
func recordedEnum(t *testing.T, home, sessionID, tool, prop string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", sessionID+".jsonl"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no transcript for session %s under %s (%v)", sessionID, home, err)
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	var find func(v any) []string
	find = func(v any) []string {
		switch x := v.(type) {
		case map[string]any:
			if x["name"] == tool {
				schema, _ := x["input_schema"].(map[string]any)
				props, _ := schema["properties"].(map[string]any)
				p, _ := props[prop].(map[string]any)
				if enum, ok := p["enum"].([]any); ok {
					out := make([]string, 0, len(enum))
					for _, e := range enum {
						s, _ := e.(string)
						out = append(out, s)
					}
					return out
				}
			}
			for _, c := range x {
				if out := find(c); out != nil {
					return out
				}
			}
		case []any:
			for _, c := range x {
				if out := find(c); out != nil {
					return out
				}
			}
		}
		return nil
	}
	for _, line := range strings.Split(string(raw), "\n") {
		var entry struct {
			Attachment map[string]any `json:"attachment"`
		}
		if json.Unmarshal([]byte(line), &entry) != nil || entry.Attachment["type"] != "deferred_tools_record" {
			continue
		}
		if enum := find(entry.Attachment); enum != nil {
			return enum
		}
	}
	t.Fatalf("session %s transcript has no deferred_tools_record entry for %s with a %q enum: CLI 2.1.330 format changed? Rerun the DEV_ENVIRONMENT hand probe", sessionID, tool, prop)
	return nil
}
