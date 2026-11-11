package notifyflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentnotify"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeclock"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// agentTab imports a repo "proj" and opens a Claude Code tab in it.
func agentTab(t *testing.T, app *flowharness.App, id string) (cwd string) {
	t.Helper()
	repo := app.NewRepo("proj")
	repo.Commit("base", map[string]string{"a.txt": "a\n"})
	importRepo(t, app, repo.Dir)
	openAgent(t, app, id, repo.Dir)
	return repo.Dir
}

func TestStopNotifies(t *testing.T) {
	app, sink := newApp(t)
	cwd := agentTab(t, app, "tab-1")
	stop(t, app, "tab-1", cwd, "Done.\nI fixed the login bug.")
	n := sink.only(t)
	if n.Kind != agentnotify.KindFinished || !strings.Contains(n.Title, "proj") {
		t.Fatalf("note = %+v, want finished naming proj", n)
	}
	if n.Body != "Done. I fixed the login bug." || n.TerminalID != "tab-1" || n.WindowKey != window {
		t.Fatalf("note = %+v, want the reply on one line and the terminal and window ids", n)
	}
}

func TestNeedsInputNotifies(t *testing.T) {
	app, sink := newApp(t)
	cwd := agentTab(t, app, "tab-1")
	hook(t, app, "tab-1", `{"hook_event_name":"Notification","notification_type":"permission_prompt","cwd":`+quote(cwd)+`,"message":"Claude needs your permission to use Bash"}`)
	n := sink.only(t)
	if n.Kind != agentnotify.KindNeedsInput || n.Body != "Claude needs your permission to use Bash" {
		t.Fatalf("note = %+v, want needs-input with the message", n)
	}
}

func TestIgnoredNotifications(t *testing.T) {
	app, sink := newApp(t)
	cwd := agentTab(t, app, "tab-1")
	for _, typ := range []string{"idle_prompt", "auth_success"} {
		hook(t, app, "tab-1", `{"hook_event_name":"Notification","notification_type":"`+typ+`","cwd":`+quote(cwd)+`,"message":"x"}`)
	}
	sink.none(t)
}

func TestWakeArmedStopSilent(t *testing.T) {
	app, sink := newApp(t)
	setCooldown(t, time.Millisecond)
	cwd := agentTab(t, app, "tab-1")
	hook(t, app, "tab-1", `{"hook_event_name":"PreToolUse","tool_name":"Monitor","tool_use_id":"u"}`)
	stop(t, app, "tab-1", cwd, "waiting")
	sink.none(t)
	event(t, app, "tab-1", "UserPromptSubmit")
	stop(t, app, "tab-1", cwd, "now done")
	if n := sink.only(t); n.Kind != agentnotify.KindFinished {
		t.Fatalf("note = %+v, want finished", n)
	}
}

// A Stop that leaves background tasks running is not finished; the last Stop, with none left, is.
func TestFinishedWaitsForBackground(t *testing.T) {
	app, sink := newApp(t)
	setCooldown(t, time.Millisecond)
	agentTab(t, app, "tab-1")
	raw, err := os.ReadFile(filepath.Join("testdata", "background-stops.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	for _, line := range lines[:len(lines)-1] {
		hook(t, app, "tab-1", line)
	}
	sink.none(t)
	hook(t, app, "tab-1", lines[len(lines)-1])
	if n := sink.only(t); n.Kind != agentnotify.KindFinished || n.Body != "all done" {
		t.Fatalf("note = %+v, want finished with the last reply", n)
	}
}

func TestFocusedAppSkipsFinished(t *testing.T) {
	app, sink := newApp(t)
	setCooldown(t, time.Millisecond)
	cwd := agentTab(t, app, "tab-1")

	app.WindowMgr.SetAppFocused(true)
	stop(t, app, "tab-1", cwd, "a")
	sink.none(t)

	time.Sleep(5 * time.Millisecond)
	app.WindowMgr.SetAppFocused(false)
	stop(t, app, "tab-1", cwd, "b")
	if len(sink.all()) != 1 {
		t.Fatalf("notes = %+v, want one once the app is blurred", sink.all())
	}
}

func TestFocusedTabSuppresses(t *testing.T) {
	app, sink := newApp(t)
	setCooldown(t, time.Millisecond)
	cwd := agentTab(t, app, "tab-1")
	needsInput := func() {
		hook(t, app, "tab-1", `{"hook_event_name":"Notification","notification_type":"permission_prompt","cwd":`+quote(cwd)+`,"message":"allow?"}`)
	}

	focus(t, app, bridge.ReportFocusArgs{Focused: true, Module: "terminal", ActiveTerminalID: "tab-1"})
	needsInput()
	sink.none(t)

	app.WindowMgr.SetAppFocused(true)
	focus(t, app, bridge.ReportFocusArgs{Focused: true, Module: "terminal", ActiveTerminalID: "tab-2"})
	needsInput()
	if len(sink.all()) != 1 {
		t.Fatalf("notes = %+v, want one while another tab is active, app focus aside", sink.all())
	}

	time.Sleep(5 * time.Millisecond)
	focus(t, app, bridge.ReportFocusArgs{Focused: false, Module: "terminal", ActiveTerminalID: "tab-1"})
	needsInput()
	if len(sink.all()) != 2 {
		t.Fatalf("notes = %+v, want a second one while the window is blurred", sink.all())
	}
}

func TestCooldown(t *testing.T) {
	app, sink := newApp(t)
	setCooldown(t, 300*time.Millisecond)
	cwd := agentTab(t, app, "tab-1")
	stop(t, app, "tab-1", cwd, "one")
	stop(t, app, "tab-1", cwd, "two")
	if len(sink.all()) != 1 {
		t.Fatalf("notes = %+v, want one inside the cooldown", sink.all())
	}
	time.Sleep(400 * time.Millisecond)
	stop(t, app, "tab-1", cwd, "three")
	if len(sink.all()) != 2 {
		t.Fatalf("notes = %+v, want two after the cooldown", sink.all())
	}
}

func TestPrefsToggles(t *testing.T) {
	app, sink := newApp(t)
	setCooldown(t, time.Millisecond)
	cwd := agentTab(t, app, "tab-1")
	needs := `{"hook_event_name":"Notification","notification_type":"elicitation_dialog","message":"pick one"}`
	pause := func() { time.Sleep(5 * time.Millisecond) }

	setNotify(t, app, model.AgentPatch{NotifyOnFinished: off()})
	stop(t, app, "tab-1", cwd, "x")
	sink.none(t)
	hook(t, app, "tab-1", needs)
	if n := sink.only(t); n.Kind != agentnotify.KindNeedsInput {
		t.Fatalf("note = %+v, want needs-input while finished is off", n)
	}

	on := true
	setNotify(t, app, model.AgentPatch{NotifyOnFinished: &on, NotifyOnNeedsInput: off()})
	pause()
	hook(t, app, "tab-1", needs)
	if len(sink.all()) != 1 {
		t.Fatalf("notes = %+v, want no new needs-input note", sink.all())
	}

	setNotify(t, app, model.AgentPatch{NotifyIncludeMessage: off()})
	stop(t, app, "tab-1", cwd, "secret reply")
	if got := sink.all(); len(got) != 2 || strings.Contains(got[1].Body, "secret") || got[1].Body == "" {
		t.Fatalf("notes = %+v, want the fixed body without the reply", got)
	}

	setNotify(t, app, model.AgentPatch{NotifyEnabled: off()})
	pause()
	stop(t, app, "tab-1", cwd, "x")
	hook(t, app, "tab-1", needs)
	if len(sink.all()) != 2 {
		t.Fatalf("notes = %+v, want nothing while the master switch is off", sink.all())
	}

	app.Restart()
	sink2 := &recSink{}
	app.W.AgentNotify.SetSink(sink2)
	cwd = agentTab2(t, app, "tab-2")
	stop(t, app, "tab-2", cwd, "x")
	sink2.none(t)
}

// agentTab2 opens a tab in an already-imported "proj" after a restart.
func agentTab2(t *testing.T, app *flowharness.App, id string) string {
	t.Helper()
	cwd := app.Work + "/proj"
	openAgent(t, app, id, cwd)
	return cwd
}

func TestAdeManualSessionNote(t *testing.T) {
	app, sink := newApp(t, flowharness.WithTrackerGrace(time.Second))
	claude(app, map[string][]fakeagent.Action{"*": {{Name: "done"}, {Name: "sleep"}}})
	setCooldown(t, time.Millisecond)
	task, rec := adeTask(t, app)
	startRun(t, app, task, rec.ID)
	waitRunDone(t, app, task.ID)
	launch, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	openAgent2(t, app, launch)
	testx.WaitUntil(t, waitFor, func() bool { _, ok := app.W.Tracker.RecordOf(launch.TerminalID); return ok })

	stop(t, app, launch.TerminalID, launch.Cwd, "step done")
	finished := sink.ofKind(agentnotify.KindFinished)
	if len(finished) != 1 {
		t.Fatalf("finished notes = %+v, want one", finished)
	}
	n := finished[0]
	if !strings.Contains(n.Title, "Fix login") || n.RecordID != launch.SessionID {
		t.Fatalf("note = %+v, want the task title and session record %s", n, launch.SessionID)
	}
}

func TestRunEndedNotifies(t *testing.T) {
	app, sink := newApp(t)
	claude(app, map[string][]fakeagent.Action{"*": {{Name: "done"}}})
	task, rec := adeTask(t, app)
	startRun(t, app, task, rec.ID)
	waitRunDone(t, app, task.ID)
	testx.WaitUntil(t, waitFor, func() bool { return len(sink.all()) > 0 })
	n := sink.only(t)
	if n.Kind != agentnotify.KindRunEnded || !strings.Contains(n.Title, "ADE run done") || !strings.Contains(n.Title, "Fix login") || n.TaskID != task.ID {
		t.Fatalf("note = %+v, want a run-ended note for the task", n)
	}

	app.Restart()
	sink2 := &recSink{}
	app.W.AgentNotify.SetSink(sink2)
	time.Sleep(time.Second)
	sink2.none(t)
}

func TestClickRevealsTerminal(t *testing.T) {
	app, sink := newApp(t)
	// The fake claude must outlive the click: once it exits, the terminal leaves the registry.
	claude(app, map[string][]fakeagent.Action{"*": {{WaitFile: filepath.Join(app.Root, "never")}}})
	cwd := agentTab(t, app, "tab-1")
	stop(t, app, "tab-1", cwd, "x")
	n := sink.only(t)
	mark := app.Events.Mark()
	app.W.AgentNotify.Click(map[string]any{"terminalId": n.TerminalID, "windowKey": n.WindowKey, "kind": string(n.Kind)})

	if got := app.WindowMgr.Focused; len(got) != 1 || got[0] != window {
		t.Fatalf("focused windows = %v, want [%s]", got, window)
	}
	evs := app.Events.Since(mark, bridge.ChannelAgentRevealTerminal)
	if len(evs) != 1 || evs[0].Window != window {
		t.Fatalf("reveal events = %+v, want one addressed to %s", evs, window)
	}
	var payload struct{ TerminalID string }
	evs[0].Decode(t, &payload)
	if payload.TerminalID != "tab-1" {
		t.Fatalf("reveal payload = %+v", payload)
	}
	app.ContractEvent(t, "notify-reveal", evs[0])
}

func TestClickRevealsAdeSession(t *testing.T) {
	app, sink := newApp(t, flowharness.WithTrackerGrace(time.Second))
	claude(app, map[string][]fakeagent.Action{"*": {{Name: "done"}, {Name: "sleep"}}})
	setCooldown(t, time.Millisecond)
	task, rec := adeTask(t, app)
	startRun(t, app, task, rec.ID)
	waitRunDone(t, app, task.ID)
	launch, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	openAgent2(t, app, launch)
	testx.WaitUntil(t, waitFor, func() bool { _, ok := app.W.Tracker.RecordOf(launch.TerminalID); return ok })
	stop(t, app, launch.TerminalID, launch.Cwd, "x")
	finished := sink.ofKind(agentnotify.KindFinished)
	if len(finished) != 1 {
		t.Fatalf("finished notes = %+v, want one", finished)
	}
	n := finished[0]

	mark := app.Events.Mark()
	app.W.AgentNotify.Click(map[string]any{"terminalId": n.TerminalID, "windowKey": n.WindowKey, "recordId": n.RecordID})
	evs := app.Events.Since(mark, adewire.ChannelOpenSession)
	if len(evs) != 1 || evs[0].Window != window {
		t.Fatalf("open-session events = %+v, want one addressed to %s", evs, window)
	}
	app.ContractEvent(t, "notify-reveal", evs[0])
}

func TestClickRevealsTask(t *testing.T) {
	app, sink := newApp(t)
	app.W.Windows.Add(window, 0, nil, func() {})
	claude(app, map[string][]fakeagent.Action{"*": {{Name: "done"}}})
	task, rec := adeTask(t, app)
	startRun(t, app, task, rec.ID)
	waitRunDone(t, app, task.ID)
	testx.WaitUntil(t, waitFor, func() bool { return len(sink.all()) > 0 })
	n := sink.only(t)
	mark := app.Events.Mark()
	app.W.AgentNotify.Click(map[string]any{"taskId": n.TaskID, "windowKey": n.WindowKey, "kind": string(n.Kind)})
	evs := app.Events.Since(mark, bridge.ChannelAgentRevealTask)
	if len(evs) != 1 || evs[0].Window != window {
		t.Fatalf("reveal events = %+v, want one addressed to %s", evs, window)
	}
	var payload struct{ TaskID string }
	evs[0].Decode(t, &payload)
	if payload.TaskID != task.ID {
		t.Fatalf("reveal payload = %+v, want task %s", payload, task.ID)
	}
	app.ContractEvent(t, "notify-reveal", evs[0])
}

func TestSendTest(t *testing.T) {
	app, sink := newApp(t)
	cwd := agentTab(t, app, "tab-1")
	_ = cwd
	focus(t, app, bridge.ReportFocusArgs{Focused: true, Module: "terminal", ActiveTerminalID: "tab-1"})
	if err := app.W.AgentNotifySvc.SendTest(); err != nil {
		t.Fatal(err)
	}
	if n := sink.only(t); n.Kind != agentnotify.KindTest {
		t.Fatalf("note = %+v, want a test note", n)
	}
	setNotify(t, app, model.AgentPatch{NotifyEnabled: off()})
	if err := app.W.AgentNotifySvc.SendTest(); err != nil {
		t.Fatal(err)
	}
	if len(sink.all()) != 1 {
		t.Fatalf("notes = %+v, want no test note while the master switch is off", sink.all())
	}
}

// smartRun starts a smart script "ask" and returns its run id.
func smartRun(t *testing.T, app *flowharness.App) string {
	t.Helper()
	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
		Name: "ask", Kind: scripts.KindSmart, Command: "say hi", Color: "blue",
	}})
	if err != nil {
		t.Fatal(err)
	}
	args := scriptruns.RunArgs{ScriptID: rec.ID}
	pv, err := app.W.ScriptRuns.Preview(args)
	if err != nil {
		t.Fatal(err)
	}
	st, err := app.W.ScriptRuns.Start(scriptruns.StartArgs{RunArgs: args, Hash: pv.Hash})
	if err != nil {
		t.Fatal(err)
	}
	return st.RunID
}

func TestAutomationNotifies(t *testing.T) {
	cases := []struct {
		name   string
		action fakeagent.Action
		title  string
	}{
		{"failed", fakeagent.Action{Name: "failed"}, "Script run failed · ask"},
		{"needs input", fakeagent.Action{Name: "needs_input"}, "Script run needs you · ask"},
		{"done", fakeagent.Action{Name: "done"}, "Script run done · ask"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app, sink := newApp(t)
			claude(app, map[string][]fakeagent.Action{"*": {c.action}})
			id := smartRun(t, app)
			testx.WaitUntil(t, waitFor, func() bool { return len(sink.all()) > 0 })
			n := sink.only(t)
			if n.Kind != agentnotify.KindAutomation || n.Title != c.title || n.ScriptRunID != id {
				t.Fatalf("note = %+v, want %q for run %s", n, c.title, id)
			}
		})
	}
}

func TestAutomationQuietCases(t *testing.T) {
	t.Run("done off", func(t *testing.T) {
		app, sink := newApp(t)
		setNotify(t, app, model.AgentPatch{NotifyOnRunEnded: off()})
		claude(app, map[string][]fakeagent.Action{"*": {{Name: "done"}}})
		id := smartRun(t, app)
		waitScriptState(t, app, id, "done")
		time.Sleep(200 * time.Millisecond)
		sink.none(t)
	})
	t.Run("stopped", func(t *testing.T) {
		app, sink := newApp(t)
		claude(app, map[string][]fakeagent.Action{"*": {{Name: "sleep"}}})
		id := smartRun(t, app)
		testx.WaitUntil(t, waitFor, func() bool { return len(fakeEntries(app)) > 0 })
		if err := app.W.ScriptRuns.Stop(scriptruns.IDArgs{ID: id}); err != nil {
			t.Fatal(err)
		}
		waitScriptState(t, app, id, "cancelled")
		time.Sleep(200 * time.Millisecond)
		sink.none(t)
	})
	t.Run("focused run", func(t *testing.T) {
		app, sink := newApp(t)
		app.W.Windows.Add(window, 0, nil, func() {})
		gate := filepath.Join(app.Root, "gate")
		claude(app, map[string][]fakeagent.Action{"*": {{Name: "done", WaitFile: gate}}})
		id := smartRun(t, app)
		focus(t, app, bridge.ReportFocusArgs{Focused: true, Module: "automations", ActiveScriptRunID: id})
		if err := os.WriteFile(gate, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		waitScriptState(t, app, id, "done")
		time.Sleep(200 * time.Millisecond)
		sink.none(t)
	})
}

func fakeEntries(app *flowharness.App) []os.DirEntry {
	es, _ := os.ReadDir(app.FakeDir)
	return es
}

func waitScriptState(t *testing.T, app *flowharness.App, id, state string) {
	t.Helper()
	testx.WaitUntil(t, waitFor, func() bool {
		r, err := app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: id})
		return err == nil && r.State == state
	})
}

func TestClickRevealsScriptRun(t *testing.T) {
	app, sink := newApp(t)
	app.W.Windows.Add(window, 0, nil, func() {})
	claude(app, map[string][]fakeagent.Action{"*": {{Name: "failed"}}})
	id := smartRun(t, app)
	testx.WaitUntil(t, waitFor, func() bool { return len(sink.all()) > 0 })
	n := sink.only(t)
	mark := app.Events.Mark()
	app.W.AgentNotify.Click(map[string]any{"scriptRunId": n.ScriptRunID, "windowKey": n.WindowKey, "kind": string(n.Kind)})
	evs := app.Events.Since(mark, bridge.ChannelAgentRevealScriptRun)
	if len(evs) != 1 || evs[0].Window != window {
		t.Fatalf("reveal events = %+v, want one addressed to %s", evs, window)
	}
	var payload struct{ RunID string }
	evs[0].Decode(t, &payload)
	if payload.RunID != id {
		t.Fatalf("reveal payload = %+v, want run %s", payload, id)
	}
}

// recurring creates a normal recurring script "tick" and returns its id.
func recurring(t *testing.T, app *flowharness.App, command string, confirm bool) string {
	t.Helper()
	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
		Name: "tick", Kind: scripts.KindScript, Command: command, Color: "blue",
		Schedule: &scripts.Schedule{Cron: "* * * * *", Timezone: "UTC", Enabled: true, Confirm: confirm},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return rec.ID
}

func scheduledStates(t *testing.T, app *flowharness.App, scriptID string) []string {
	t.Helper()
	runs, err := app.W.ScriptRuns.List(scriptruns.ListArgs{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range runs {
		if r.ScriptID == scriptID && r.Trigger == scriptruns.TriggerScheduled {
			out = append(out, r.State)
		}
	}
	return out
}

func TestRecurringScriptNotifies(t *testing.T) {
	start := time.Date(2030, 1, 7, 8, 59, 30, 0, time.UTC)
	fire := func(t *testing.T, clock *fakeclock.Clock) {
		t.Helper()
		if !clock.BlockUntil(1, waitFor) {
			t.Fatal("the scheduler never armed a timer")
		}
		clock.Advance(30 * time.Second)
	}
	t.Run("failed posts", func(t *testing.T) {
		clock := fakeclock.New(start)
		app, sink := newApp(t, flowharness.WithClock(clock))
		recurring(t, app, "exit 2", false)
		fire(t, clock)
		testx.WaitUntil(t, waitFor, func() bool { return len(sink.all()) > 0 })
		if n := sink.only(t); n.Title != "Recurring script failed · tick" {
			t.Fatalf("note = %+v", n)
		}
	})
	for _, c := range []struct {
		name, command string
		confirm       bool
		want          string
	}{
		{"done is quiet", "true", false, "done"},
		{"waiting is quiet", "true", true, "waiting"},
	} {
		t.Run(c.name, func(t *testing.T) {
			clock := fakeclock.New(start)
			app, sink := newApp(t, flowharness.WithClock(clock))
			id := recurring(t, app, c.command, c.confirm)
			fire(t, clock)
			testx.WaitUntil(t, waitFor, func() bool {
				s := scheduledStates(t, app, id)
				return len(s) == 1 && s[0] == c.want
			})
			time.Sleep(200 * time.Millisecond)
			sink.none(t)
		})
	}
	t.Run("skipped is quiet", func(t *testing.T) {
		clock := fakeclock.New(start)
		app, sink := newApp(t, flowharness.WithClock(clock))
		id := recurring(t, app, "sleep 30", false)
		fire(t, clock)
		testx.WaitUntil(t, waitFor, func() bool { return len(scheduledStates(t, app, id)) == 1 })
		if !clock.BlockUntil(1, waitFor) {
			t.Fatal("the scheduler never re-armed")
		}
		clock.Advance(time.Minute)
		testx.WaitUntil(t, waitFor, func() bool {
			s := scheduledStates(t, app, id)
			return len(s) == 2 && s[0] == "skipped"
		})
		time.Sleep(200 * time.Millisecond)
		sink.none(t)
	})
}
